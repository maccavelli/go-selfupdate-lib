package selfupdate

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"
)

// Regression tests for docs/decisions/0010-MADR-remediate-second-debugging-pass-findings.md
// A4–A7, A9–A11 (docs/decisions/0010-PLAN-v1-5-1-contract-preserving-fixes.md P2).

func hardeningSource(t *testing.T, api *httptest.Server, edit func(*GitHubOptions)) *GitHubSource {
	t.Helper()
	base, err := url.Parse(api.URL)
	if err != nil {
		t.Fatal(err)
	}
	opts := GitHubOptions{
		Repository: Repository{Owner: "maccavelli", Name: "demo"},
		Client:     api.Client(),
		APIBaseURL: base,
		UserAgent:  "demo/v1.0.0",
		Limits:     testGitHubLimits(),
	}
	if edit != nil {
		edit(&opts)
	}
	src, err := NewGitHubSource(opts)
	if err != nil {
		t.Fatal(err)
	}
	return src
}

// TestNotBeforeClamped: a rate limit defers the next check by at least a
// minute and at most an hour, whatever the headers say, and a stored
// deferral beyond the cap is a miss (A4).
func TestNotBeforeClamped(t *testing.T) {
	for name, tc := range map[string]struct {
		rl   *RateLimitError
		want time.Duration
	}{
		"reset in 2200":     {&RateLimitError{StatusCode: 403, Reset: time.Date(2200, 1, 1, 0, 0, 0, 0, time.UTC)}, time.Hour},
		"reset in the past": {&RateLimitError{StatusCode: 403, Reset: cacheNow.Add(-2 * time.Minute)}, time.Minute},
		"no headers":        {&RateLimitError{StatusCode: 429}, time.Minute},
		"retry-after 10m":   {&RateLimitError{StatusCode: 429, RetryAfter: 10 * time.Minute}, 10 * time.Minute},
		// A secondary limit with quota left waits its Retry-After, not the
		// primary window's reset; an exhausted quota waits for the reset
		// (docs/decisions/0015-MADR-remediate-third-debugging-pass-findings.md A5).
		"retry-after, quota left": {&RateLimitError{StatusCode: 403, RetryAfter: time.Minute, Remaining: 5,
			Reset: cacheNow.Add(50 * time.Minute)}, time.Minute},
		"quota spent, reset in 30m": {&RateLimitError{StatusCode: 403, Reset: cacheNow.Add(30 * time.Minute)}, 30 * time.Minute},
	} {
		if got := notBefore(cacheNow, tc.rl).Sub(cacheNow); got != tc.want {
			t.Errorf("%s: deferred %v, want %v", name, got, tc.want)
		}
	}

	// The stored record must match the request as CheckCached keys it, the
	// platform normalised, or the cap is never what makes it a miss
	// (0015-MADR A7).
	c, src, req, key := cacheEnv(t)
	store := &memStore{rec: CheckRecord{Request: key, NotBefore: cacheNow.Add(48 * time.Hour)}}
	before := networkCalls(src)
	if _, err := c.CheckCached(context.Background(), req, store, time.Hour); err != nil {
		t.Fatalf("a stored deferral beyond the cap: %v, want a fresh check", err)
	}
	if networkCalls(src) == before {
		t.Fatal("a stored deferral beyond the cap still deferred the check")
	}
}

// TestRedirectErrorHidesQuery: a refused redirect, and a failure on the
// redirected hop, never echo the signed query of the redirect URL (A5).
func TestRedirectErrorHidesQuery(t *testing.T) {
	noEnv(t, nil)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	closedAddr := ln.Addr().String()
	if err := ln.Close(); err != nil {
		t.Fatal(err)
	}
	var target string
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target, http.StatusFound)
	}))
	defer api.Close()
	src := hardeningSource(t, api, nil)
	asset := Asset{ID: 1, Name: "demo-linux-amd64", State: AssetStateUploaded, Size: 5}
	for _, tgt := range []string{
		"http://downloads.example.invalid/blob?X-Amz-Signature=SECRET1#frag",
		"http://" + closedAddr + "/blob?X-Amz-Signature=SECRET2",
	} {
		target = tgt
		_, err := src.OpenAsset(context.Background(), Release{ID: 1, Tag: "v1.0.0"}, asset)
		if err == nil {
			t.Fatalf("redirect to %s succeeded", tgt)
		}
		if strings.Contains(err.Error(), "SECRET") || strings.Contains(err.Error(), "frag") {
			t.Errorf("error echoes the redirect query: %v", err)
		}
		if !strings.Contains(err.Error(), "/blob") {
			t.Errorf("error lost the redirect path: %v", err)
		}
	}
}

type countingProvider struct {
	mu    sync.Mutex
	calls int
}

func (p *countingProvider) Credential(context.Context, CredentialRequest) (Credential, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.calls++
	return Credential{Value: []byte("tok" + string(rune('0'+p.calls))), Source: "probe"}, nil
}

// TestForeign401DoesNotRefresh: a 401 from the download origin, which never
// saw the credential, does not ask the provider again (A6).
func TestForeign401DoesNotRefresh(t *testing.T) {
	noEnv(t, nil)
	foreign := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "nope", http.StatusUnauthorized)
	}))
	defer foreign.Close()
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, foreign.URL+"/blob", http.StatusFound)
	}))
	defer api.Close()
	p := &countingProvider{}
	src := hardeningSource(t, api, func(o *GitHubOptions) { o.Credentials = p })
	_, err := src.OpenAsset(context.Background(), Release{ID: 1, Tag: "v1.0.0"},
		Asset{ID: 1, Name: "demo-linux-amd64", State: AssetStateUploaded, Size: 5})
	if err == nil || !strings.Contains(err.Error(), "401") {
		t.Fatalf("err = %v, want the download origin's 401", err)
	}
	if p.calls != 1 {
		t.Fatalf("the provider was asked %d times, want 1", p.calls)
	}
}

type blockingProvider struct {
	entered chan struct{}
	release chan struct{}
	once    sync.Once
}

func (b *blockingProvider) Credential(context.Context, CredentialRequest) (Credential, error) {
	b.once.Do(func() { close(b.entered) })
	<-b.release
	return Credential{}, ErrNoCredential
}

// TestCredentialWaitHonoursContext: while one request waits on a blocking
// provider, another request's context still ends it (A7).
func TestCredentialWaitHonoursContext(t *testing.T) {
	noEnv(t, nil)
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.NotFound(w, r) }))
	defer api.Close()
	bp := &blockingProvider{entered: make(chan struct{}), release: make(chan struct{})}
	src := hardeningSource(t, api, func(o *GitHubOptions) { o.Credentials = bp })
	first := make(chan struct{})
	go func() {
		defer close(first)
		_, _ = src.Latest(context.Background())
	}()
	<-bp.entered
	// The provider is released after 2s whatever happens, so a source that
	// ignores the waiter's context fails this test instead of hanging it.
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(bp.release) }) }
	safety := time.AfterFunc(2*time.Second, release)
	defer safety.Stop()
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, err := src.Latest(ctx)
	elapsed := time.Since(start)
	release()
	<-first
	if !errors.Is(err, context.DeadlineExceeded) || elapsed > time.Second {
		t.Fatalf("second Latest returned after %v with %v, want its own deadline", elapsed, err)
	}
}

// TestCredentialRefusesControlBytes: every control byte but HTAB is refused
// in a credential value, as net/http would refuse it on the wire (A9).
func TestCredentialRefusesControlBytes(t *testing.T) {
	for _, v := range []string{"tok\x01en", "tok\x7fen", "tok\x1ben", "tok\ren", "tok\x00"} {
		if err := validateCredential(Credential{Value: []byte(v), Source: "test"}); err == nil {
			t.Errorf("%q accepted", v)
		}
	}
	if err := validateCredential(Credential{Value: []byte("tok\ten"), Source: "test"}); err != nil {
		t.Errorf("a tab was refused: %v", err)
	}
}

// TestLoopbackRedirectOnlyFromLoopback: a plain-http loopback hop is
// allowed only when the API itself is on loopback (A10).
func TestLoopbackRedirectOnlyFromLoopback(t *testing.T) {
	noEnv(t, nil)
	remote, err := NewGitHubSource(GitHubOptions{Repository: Repository{Owner: "o", Name: "r"}, Client: &http.Client{},
		UserAgent: "demo/v1.0.0", Limits: testGitHubLimits()})
	if err != nil {
		t.Fatal(err)
	}
	prev, err := http.NewRequest(http.MethodGet, "https://api.github.com/repos/o/r/releases/assets/1", nil)
	if err != nil {
		t.Fatal(err)
	}
	next, err := http.NewRequest(http.MethodGet, "http://127.0.0.1:8080/admin", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := remote.checkRedirect(next, []*http.Request{prev}); err == nil {
		t.Fatal("a remote API's redirect to plain-http loopback was followed")
	}
	api := httptest.NewServer(http.NotFoundHandler())
	defer api.Close()
	local := hardeningSource(t, api, nil)
	if err := local.checkRedirect(next, []*http.Request{prev}); err != nil {
		t.Fatalf("a loopback API's redirect to loopback was refused: %v", err)
	}
}

// TestByTagRefusesDotSegments: "." and ".." are refused before any request
// (A11).
func TestByTagRefusesDotSegments(t *testing.T) {
	noEnv(t, nil)
	var hits int
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		http.NotFound(w, r)
	}))
	defer api.Close()
	src := hardeningSource(t, api, nil)
	for _, tag := range []string{"..", "."} {
		if _, err := src.ByTag(context.Background(), tag); err == nil || !strings.Contains(err.Error(), "invalid release tag") {
			t.Errorf("ByTag(%q) = %v", tag, err)
		}
	}
	if hits != 0 {
		t.Fatalf("%d requests were sent", hits)
	}
}

// TestRedirectKeepsOnlyFixedHeaders: a hop to another origin carries only
// the source's fixed headers. A credential header the request still holds,
// though the source's credential has since moved to another header, does
// not cross (docs/decisions/0015-MADR-remediate-third-debugging-pass-findings.md A3).
func TestRedirectKeepsOnlyFixedHeaders(t *testing.T) {
	noEnv(t, nil)
	api := httptest.NewTLSServer(http.NotFoundHandler())
	t.Cleanup(api.Close)
	src := hardeningSource(t, api, nil)
	src.cred.mu.Lock()
	src.cred.cred = Credential{Header: "X-New-Key", Value: []byte("new-secret"), Source: "provider"}
	src.cred.mu.Unlock()
	first, err := http.NewRequest(http.MethodGet, api.URL+"/repos/maccavelli/demo/releases/assets/7", http.NoBody)
	if err != nil {
		t.Fatal(err)
	}
	hop, err := http.NewRequest(http.MethodGet, "https://objects.example.invalid/blob", http.NoBody)
	if err != nil {
		t.Fatal(err)
	}
	for k, v := range map[string]string{
		"Accept": gitHubAcceptAsset, "Accept-Encoding": "identity", "User-Agent": "demo/v1.0.0",
		"X-GitHub-Api-Version": gitHubAPIVersion, "X-Old-Key": "old-secret", "Authorization": "Bearer old",
		"Cookie": "session=1",
	} {
		hop.Header.Set(k, v)
	}
	if err := src.checkRedirect(hop, []*http.Request{first}); err != nil {
		t.Fatal(err)
	}
	var kept []string
	for k := range hop.Header {
		kept = append(kept, k)
	}
	slices.Sort(kept)
	want := []string{"Accept", "Accept-Encoding", "User-Agent", "X-Github-Api-Version"}
	if !slices.Equal(kept, want) {
		t.Fatalf("headers on the foreign hop %v, want only %v", kept, want)
	}
}
