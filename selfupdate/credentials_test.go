package selfupdate

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

// Tests for docs/decisions/0004-PLAN-v1-1-0-core-api.md Step 6. Two
// loopback servers are two origins: the API, and an asset host it
// redirects to.

type seenRequest struct {
	path, auth, custom string
}

type credServer struct {
	api, foreign *httptest.Server
	mu           sync.Mutex
	seen         []seenRequest
	foreignSeen  []seenRequest
	refuse       map[string]bool // Authorization or X-Api-Key values answered with 401
}

func newCredServer(t *testing.T) *credServer {
	t.Helper()
	cs := &credServer{refuse: map[string]bool{}}
	body := validReleaseJSON(t, "v1.0.0", uploaded(1, "demo-linux-amd64", 5), uploaded(2, "SHA256SUMS", 5))
	cs.foreign = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cs.mu.Lock()
		cs.foreignSeen = append(cs.foreignSeen, seenRequest{r.URL.Path, r.Header.Get("Authorization"), r.Header.Get("X-Api-Key")})
		cs.mu.Unlock()
		_, _ = w.Write([]byte("hello"))
	}))
	t.Cleanup(cs.foreign.Close)
	cs.api = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		req := seenRequest{r.URL.Path, r.Header.Get("Authorization"), r.Header.Get("X-Api-Key")}
		cs.mu.Lock()
		cs.seen = append(cs.seen, req)
		refused := cs.refuse[req.auth] || cs.refuse[req.custom]
		cs.mu.Unlock()
		if refused {
			http.Error(w, "bad credentials", http.StatusUnauthorized)
			return
		}
		switch r.URL.Path {
		case "/repos/maccavelli/demo/releases/latest":
			_, _ = w.Write(body)
		case "/repos/maccavelli/demo/releases/assets/1":
			http.Redirect(w, r, cs.foreign.URL+"/blob", http.StatusFound)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(cs.api.Close)
	return cs
}

func (cs *credServer) source(t *testing.T, edit func(*GitHubOptions)) *GitHubSource {
	t.Helper()
	base, err := url.Parse(cs.api.URL)
	if err != nil {
		t.Fatal(err)
	}
	opts := GitHubOptions{
		Repository: Repository{Owner: "maccavelli", Name: "demo"},
		Client:     cs.api.Client(),
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

func (cs *credServer) requests() []seenRequest {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	return append([]seenRequest(nil), cs.seen...)
}

// scripted returns each credential in turn, then repeats the last one.
type scripted struct {
	mu     sync.Mutex
	values []Credential
	err    error
	calls  int
	causes []error
}

func (p *scripted) Credential(_ context.Context, r CredentialRequest) (Credential, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.calls++
	p.causes = append(p.causes, r.Cause)
	if p.err != nil {
		return Credential{}, p.err
	}
	i := p.calls - 1
	if i >= len(p.values) {
		i = len(p.values) - 1
	}
	return p.values[i], nil
}

func bearer(v string) Credential { return Credential{Value: []byte(v), Source: "test"} }

func noEnv(t *testing.T, env map[string]string) {
	t.Helper()
	setSeam(t, &lookupEnv, func(k string) string { return env[k] })
}

func TestCredentialOrder(t *testing.T) {
	ctx := context.Background()
	cs := newCredServer(t)
	noEnv(t, map[string]string{"GH_TOKEN": "envtok"})
	prov := &scripted{values: []Credential{bearer("prov")}}

	explicit := cs.source(t, func(o *GitHubOptions) { o.Token = "explicit"; o.Credentials = prov })
	if _, err := explicit.Latest(ctx); err != nil {
		t.Fatal(err)
	}
	withProvider := cs.source(t, func(o *GitHubOptions) { o.Credentials = prov })
	if _, err := withProvider.Latest(ctx); err != nil {
		t.Fatal(err)
	}
	none := &scripted{err: ErrNoCredential}
	fallThrough := cs.source(t, func(o *GitHubOptions) { o.Credentials = none })
	if _, err := fallThrough.Latest(ctx); err != nil {
		t.Fatal(err)
	}
	got := cs.requests()
	want := []string{"Bearer explicit", "Bearer prov", "Bearer envtok"}
	for i, w := range want {
		if got[i].auth != w {
			t.Errorf("request %d: Authorization %q, want %q", i, got[i].auth, w)
		}
	}
	if prov.calls != 1 {
		t.Errorf("the provider was asked %d times; the explicit token must come first", prov.calls)
	}
}

func TestCredentialLazy(t *testing.T) {
	cs := newCredServer(t)
	noEnv(t, nil)
	prov := &scripted{values: []Credential{bearer("prov")}}
	src := cs.source(t, func(o *GitHubOptions) { o.Credentials = prov })
	if prov.calls != 0 {
		t.Fatal("the constructor asked the provider")
	}
	for range 2 {
		if _, err := src.Latest(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	if prov.calls != 1 {
		t.Fatalf("provider asked %d times for two requests", prov.calls)
	}
}

func TestCredentialFallThrough(t *testing.T) {
	cs := newCredServer(t)
	noEnv(t, nil)
	src := cs.source(t, func(o *GitHubOptions) {
		o.Credentials = ChainCredentials(&scripted{err: ErrNoCredential}, nil, EnvCredential("", "UNSET"))
	})
	if _, err := src.Latest(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := cs.requests()[0]; got.auth != "" || got.custom != "" {
		t.Fatalf("an anonymous request carried %+v", got)
	}

	second := cs.source(t, func(o *GitHubOptions) {
		o.Credentials = ChainCredentials(&scripted{err: ErrNoCredential}, &scripted{values: []Credential{bearer("second")}})
	})
	if _, err := second.Latest(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := cs.requests()[1]; got.auth != "Bearer second" {
		t.Fatalf("the chain did not fall through to its second link: %+v", got)
	}
}

func TestCredentialProviderErrorFailsRequest(t *testing.T) {
	cs := newCredServer(t)
	noEnv(t, nil)
	boom := errors.New("keychain locked")
	src := cs.source(t, func(o *GitHubOptions) { o.Credentials = &scripted{err: boom} })
	if _, err := src.Latest(context.Background()); !errors.Is(err, boom) {
		t.Fatalf("err = %v", err)
	}
	if n := len(cs.requests()); n != 0 {
		t.Fatalf("%d requests sent after the provider failed", n)
	}
}

func TestCredentialHeaderModes(t *testing.T) {
	cs := newCredServer(t)
	noEnv(t, nil)
	custom := cs.source(t, func(o *GitHubOptions) {
		o.Credentials = &scripted{values: []Credential{{Header: "X-Api-Key", Value: []byte("k1"), Source: "test"}}}
	})
	if _, err := custom.Latest(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := cs.requests()[0]; got.custom != "k1" || got.auth != "" {
		t.Fatalf("custom header mode sent %+v", got)
	}
}

func TestCredentialInvalidRefused(t *testing.T) {
	cs := newCredServer(t)
	noEnv(t, nil)
	for _, bad := range []Credential{
		{Header: "Bad Header", Value: []byte("secret-one"), Source: "t"},
		{Value: []byte("secret\r\ntwo"), Source: "t"},
		{Value: nil, Source: "t"},
	} {
		src := cs.source(t, func(o *GitHubOptions) { o.Credentials = &scripted{values: []Credential{bad}} })
		_, err := src.Latest(context.Background())
		if err == nil || !strings.Contains(err.Error(), "invalid credential") || strings.Contains(err.Error(), "secret") {
			t.Errorf("credential %q: err = %v", bad.Header, err)
		}
	}
}

func TestCredentialStrippedCrossOrigin(t *testing.T) {
	noEnv(t, nil)
	for _, cred := range []Credential{bearer("tok"), {Header: "X-Api-Key", Value: []byte("k1"), Source: "t"}} {
		cs := newCredServer(t)
		src := cs.source(t, func(o *GitHubOptions) { o.Credentials = &scripted{values: []Credential{cred}} })
		rel := Release{ID: 11, Tag: "v1.0.0"}
		rc, err := src.OpenAsset(context.Background(), rel, Asset{ID: 1, Name: "demo-linux-amd64", State: AssetStateUploaded, Size: 5})
		if err != nil {
			t.Fatal(err)
		}
		_, _ = io.ReadAll(rc)
		_ = rc.Close()
		if first := cs.requests()[0]; first.auth == "" && first.custom == "" {
			t.Fatalf("the API request carried no credential: %+v", first)
		}
		cs.mu.Lock()
		foreign := cs.foreignSeen
		cs.mu.Unlock()
		if len(foreign) != 1 || foreign[0].auth != "" || foreign[0].custom != "" {
			t.Fatalf("credential %q reached the asset host: %+v", cred.Header, foreign)
		}
	}
}

type countingObserver struct{ n atomic.Int32 }

func (o *countingObserver) Accepted(context.Context, Credential) { o.n.Add(1) }

func TestCredentialAcceptedOnce(t *testing.T) {
	cs := newCredServer(t)
	noEnv(t, nil)
	obs := &countingObserver{}
	src := cs.source(t, func(o *GitHubOptions) {
		o.Credentials = &scripted{values: []Credential{bearer("good")}}
		o.Observer = obs
	})
	for range 3 {
		if _, err := src.Latest(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	if got := obs.n.Load(); got != 1 {
		t.Fatalf("Accepted called %d times", got)
	}

	cs.refuse["Bearer bad"] = true
	refusedObs := &countingObserver{}
	refused := cs.source(t, func(o *GitHubOptions) {
		o.Credentials = &scripted{values: []Credential{bearer("bad")}}
		o.Observer = refusedObs
	})
	if _, err := refused.Latest(context.Background()); err == nil {
		t.Fatal("want the 401")
	}
	if refusedObs.n.Load() != 0 {
		t.Fatal("Accepted called for a refused credential")
	}
}

func TestCredential401RetryOnce(t *testing.T) {
	cs := newCredServer(t)
	noEnv(t, nil)
	cs.refuse["Bearer bad"] = true
	prov := &scripted{values: []Credential{bearer("bad"), bearer("good")}}
	src := cs.source(t, func(o *GitHubOptions) { o.Credentials = prov })
	if _, err := src.Latest(context.Background()); err != nil {
		t.Fatalf("the retry with a new credential failed: %v", err)
	}
	if prov.calls != 2 || prov.causes[0] != nil || prov.causes[1] == nil {
		t.Fatalf("calls=%d causes=%v", prov.calls, prov.causes)
	}
	if n := len(cs.requests()); n != 2 {
		t.Fatalf("%d requests, want the refused one and one retry", n)
	}

	cs2 := newCredServer(t)
	cs2.refuse["Bearer bad"] = true
	cs2.refuse["Bearer worse"] = true
	twice := &scripted{values: []Credential{bearer("bad"), bearer("worse"), bearer("good")}}
	src2 := cs2.source(t, func(o *GitHubOptions) { o.Credentials = twice })
	if _, err := src2.Latest(context.Background()); err == nil {
		t.Fatal("want the second 401")
	}
	if n := len(cs2.requests()); n != 2 || twice.calls != 2 {
		t.Fatalf("requests=%d calls=%d; only one retry is allowed", n, twice.calls)
	}

	// Once per source: after the retry succeeded, a later 401 does not ask
	// the provider again, so a provider that prompts is never looped.
	cs.mu.Lock()
	cs.refuse["Bearer good"] = true
	cs.mu.Unlock()
	if _, err := src.Latest(context.Background()); err == nil {
		t.Fatal("want the later 401")
	}
	if prov.calls != 2 {
		t.Fatalf("the provider was asked %d times; the retry is once per source", prov.calls)
	}
}

func TestCredential401SameValueNoRetry(t *testing.T) {
	cs := newCredServer(t)
	noEnv(t, nil)
	cs.refuse["Bearer bad"] = true
	src := cs.source(t, func(o *GitHubOptions) { o.Credentials = &scripted{values: []Credential{bearer("bad")}} })
	if _, err := src.Latest(context.Background()); err == nil {
		t.Fatal("want the 401")
	}
	if n := len(cs.requests()); n != 1 {
		t.Fatalf("%d requests; the same credential must not be resent", n)
	}

	fixed := cs.source(t, func(o *GitHubOptions) { o.Token = "bad" })
	if _, err := fixed.Latest(context.Background()); err == nil {
		t.Fatal("want the 401")
	}
	if n := len(cs.requests()); n != 2 {
		t.Fatalf("an explicit token was retried: %d requests", n)
	}
}

func TestEnvCredentialSource(t *testing.T) {
	noEnv(t, map[string]string{"B": "value-b"})
	got, err := EnvCredential("X-Key", "A", "B").Credential(context.Background(), CredentialRequest{})
	if err != nil || got.Source != "env:B" || got.Header != "X-Key" || string(got.Value) != "value-b" {
		t.Fatalf("got %+v err=%v", got, err)
	}
	if _, err := EnvCredential("", "A").Credential(context.Background(), CredentialRequest{}); !errors.Is(err, ErrNoCredential) {
		t.Fatalf("unset: %v", err)
	}
}

// slowProvider counts calls; concurrent first requests must share one.
type slowProvider struct{ calls atomic.Int32 }

func (p *slowProvider) Credential(context.Context, CredentialRequest) (Credential, error) {
	p.calls.Add(1)
	return bearer("tok"), nil
}

func TestCredentialConcurrent(t *testing.T) {
	cs := newCredServer(t)
	noEnv(t, nil)
	prov := &slowProvider{}
	src := cs.source(t, func(o *GitHubOptions) { o.Credentials = prov })
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			if _, err := src.Latest(context.Background()); err != nil {
				t.Error(err)
			}
		})
	}
	wg.Wait()
	if got := prov.calls.Load(); got != 1 {
		t.Fatalf("provider asked %d times by concurrent requests", got)
	}
}

// TestGitHubSourceWithCredentials: the copy has its own provider and
// state, and the receiver is unchanged
// (0004-PLAN-v1-2-0-interaction-stream.md Step 3).
func TestGitHubSourceWithCredentials(t *testing.T) {
	ctx := context.Background()
	noEnv(t, nil)
	cs := newCredServer(t)
	src := cs.source(t, nil)
	// The receiver resolves first, to anonymous: a copy that inherited that
	// state would never ask its own provider.
	if _, err := src.Latest(ctx); err != nil {
		t.Fatal(err)
	}
	prov := &scripted{values: []Credential{bearer("run")}}
	copied, ok := src.WithCredentials(prov).(*GitHubSource)
	if !ok || copied == src {
		t.Fatalf("WithCredentials returned %T %p, want a new *GitHubSource", copied, copied)
	}
	if _, err := copied.Latest(ctx); err != nil {
		t.Fatal(err)
	}
	if got := cs.requests()[1].auth; got != "Bearer run" {
		t.Fatalf("the copy sent %q, want the run's provider", got)
	}
	if src.provider != nil {
		t.Fatalf("the receiver's provider changed: %v", src.provider)
	}
	if _, err := src.Latest(ctx); err != nil {
		t.Fatal(err)
	}
	if got := cs.requests()[2].auth; got != "" {
		t.Fatalf("the receiver sent %q, want anonymous", got)
	}

	explicit := cs.source(t, func(o *GitHubOptions) { o.Token = "explicit" })
	if _, err := explicit.WithCredentials(prov).Latest(ctx); err != nil {
		t.Fatal(err)
	}
	if got := cs.requests()[3].auth; got != "Bearer explicit" {
		t.Fatalf("an explicit token lost to the run's provider: %q", got)
	}

	withProvider := cs.source(t, func(o *GitHubOptions) { o.Credentials = prov })
	bare, _ := withProvider.WithCredentials(nil).(*GitHubSource)
	if bare.provider != nil {
		t.Fatal("WithCredentials(nil) kept the receiver's provider")
	}
}

// TestGitHubSourceWithCredentialsStripsCrossOrigin: the copy's redirect
// check scrubs the copy's own credential header, not the receiver's.
func TestGitHubSourceWithCredentialsStripsCrossOrigin(t *testing.T) {
	noEnv(t, nil)
	cs := newCredServer(t)
	src := cs.source(t, nil)
	custom := &scripted{values: []Credential{{Header: "X-Api-Key", Value: []byte("k1"), Source: "t"}}}
	copied := src.WithCredentials(custom)
	rel := Release{ID: 11, Tag: "v1.0.0"}
	rc, err := copied.OpenAsset(context.Background(), rel, Asset{ID: 1, Name: "demo-linux-amd64", State: AssetStateUploaded, Size: 5})
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.ReadAll(rc)
	_ = rc.Close()
	if first := cs.requests()[0]; first.custom != "k1" {
		t.Fatalf("the API request carried %+v, want the custom header", first)
	}
	cs.mu.Lock()
	foreign := cs.foreignSeen
	cs.mu.Unlock()
	if len(foreign) != 1 || foreign[0].custom != "" || foreign[0].auth != "" {
		t.Fatalf("the copy's credential reached the asset host: %+v", foreign)
	}
}
