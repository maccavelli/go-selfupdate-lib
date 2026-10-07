package selfupdate

import (
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"
)

// Tests for the network and integrity path, added by
// docs/decisions/0003-PLAN-remediate-debugging-pass-findings.md Phase 2.

func uploaded(id int64, name string, size int64) githubAssetJSON {
	return githubAssetJSON{ID: id, Name: name, State: "uploaded", Size: size}
}

// TestGitHubExtraAssetsDoNotPoisonRelease: an unrelated extra asset with a
// bad state, size or digest does not make the release unusable (A1).
func TestGitHubExtraAssetsDoNotPoisonRelease(t *testing.T) {
	extras := map[string]githubAssetJSON{
		"zero-size":   uploaded(3, "README.txt", 0),
		"open-state":  {ID: 3, Name: "install.sh", State: "open", Size: 10},
		"oversize":    uploaded(3, "big.apk", 1<<30),
		"sha512":      {ID: 3, Name: "notes.txt", State: "uploaded", Size: 10, Digest: "sha512:" + strings.Repeat("a", 128)},
		"new-starter": {ID: 3, Name: "pending.bin", State: "starter", Size: 10},
	}
	for name, extra := range extras {
		t.Run(name, func(t *testing.T) {
			body := validReleaseJSON(t, "v1.0.0", uploaded(1, "demo-linux-amd64", 10), uploaded(2, "SHA256SUMS", 10), extra)
			env := newGitHubEnv(t, func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write(body) }, "")
			rel, err := env.src.Latest(context.Background())
			if err != nil {
				t.Fatalf("extra %s poisoned the release: %v", name, err)
			}
			if len(rel.Assets) != 3 {
				t.Fatalf("assets = %+v", rel.Assets)
			}
		})
	}
	// Structure is still enforced for every asset.
	body := validReleaseJSON(t, "v1.0.0", uploaded(1, "demo-linux-amd64", 10), uploaded(2, "../SHA256SUMS", 10))
	env := newGitHubEnv(t, func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write(body) }, "")
	if _, err := env.src.Latest(context.Background()); err == nil {
		t.Fatal("accepted a non-basename asset name")
	}
}

// TestGitHubByTagRejectsMismatchedTag (A2).
func TestGitHubByTagRejectsMismatchedTag(t *testing.T) {
	body := validReleaseJSON(t, "v0.1.0", uploaded(1, "x", 1))
	env := newGitHubEnv(t, func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write(body) }, "")
	rel, err := env.src.ByTag(context.Background(), "v1.2.3")
	if !errors.Is(err, ErrIntegrity) || rel.Tag != "" {
		t.Fatalf("rel=%+v err=%v", rel, err)
	}
}

// TestGitHubRedirectRequiresHTTPS (A3).
func TestGitHubRedirectRequiresHTTPS(t *testing.T) {
	env := newGitHubEnv(t, func(http.ResponseWriter, *http.Request) {}, "")
	prev := &http.Request{URL: mustURL(t, "https://api.github.com/repos/o/r/releases/assets/1"), Header: http.Header{}}
	cases := map[string]bool{
		"http://objects.example.com/blob":  false,
		"HTTP://objects.example.com/blob":  false,
		"ftp://objects.example.com/blob":   false,
		"https://objects.example.com/blob": true,
		"http://127.0.0.1:8080/blob":       true,
		"http://localhost/blob":            true,
		"http://[::1]/blob":                true,
	}
	for raw, allowed := range cases {
		req := &http.Request{URL: mustURL(t, raw), Header: http.Header{}}
		err := env.src.checkRedirect(req, []*http.Request{prev})
		if (err == nil) != allowed {
			t.Errorf("redirect to %s: err=%v, allowed=%v", raw, err, allowed)
		}
	}
}

func mustURL(t *testing.T, raw string) *url.URL {
	t.Helper()
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	return u
}

// TestDecodeJSONRejectsTrailingDelimiters (A6).
func TestDecodeJSONRejectsTrailingDelimiters(t *testing.T) {
	for _, in := range []string{`{"id":1}}`, `{"id":1}]`, `{"id":1}]]]}}`, `{"id":1} x`, `{"id":1}{"id":2}`} {
		var v map[string]any
		if err := decodeJSON([]byte(in), &v); err == nil {
			t.Errorf("accepted %q", in)
		}
	}
	for _, in := range []string{`{"id":1}`, "{\"id\":1}\n", " {\"id\":1} \r\n\t"} {
		var v map[string]any
		if err := decodeJSON([]byte(in), &v); err != nil {
			t.Errorf("rejected %q: %v", in, err)
		}
	}
}

// TestRateLimitRetryAfterOverflow (A7).
func TestRateLimitRetryAfterOverflow(t *testing.T) {
	for _, v := range []string{"9999999999", "9223372037", "92233720368547758070"} {
		resp := &http.Response{StatusCode: http.StatusTooManyRequests, Header: http.Header{"Retry-After": {v}}}
		var rl *RateLimitError
		if err := parseRateLimit(resp, nil); !errors.As(err, &rl) || rl.RetryAfter != 0 {
			t.Errorf("Retry-After %s: err=%v retry=%v", v, err, rl.RetryAfter)
		}
	}
	resp := &http.Response{StatusCode: http.StatusTooManyRequests, Header: http.Header{"Retry-After": {"9223372036"}}}
	var rl *RateLimitError
	if err := parseRateLimit(resp, nil); !errors.As(err, &rl) || rl.RetryAfter <= 0 {
		t.Fatalf("largest representable Retry-After: err=%v retry=%v", err, rl.RetryAfter)
	}
}

// TestGitHubLargeErrorBodyKeepsRateLimit: an error body over the limit is
// truncated, not fatal, so the status still maps (A8).
func TestGitHubLargeErrorBodyKeepsRateLimit(t *testing.T) {
	big := strings.Repeat("x", 8<<10)
	env := newGitHubEnv(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Retry-After", "5")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(big))
	}, "")
	_, err := env.src.Latest(context.Background())
	var rl *RateLimitError
	if !errors.As(err, &rl) || rl.RetryAfter != 5*time.Second {
		t.Fatalf("err = %v", err)
	}
	env = newGitHubEnv(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(big))
	}, "")
	rel := Release{ID: 1, Tag: "v1.0.0", Assets: []Asset{{ID: 7, Name: "demo", State: "uploaded", Size: 4}}}
	if _, err := env.src.OpenAsset(context.Background(), rel, rel.Assets[0]); err == nil || !strings.Contains(err.Error(), "github http 404") {
		t.Fatalf("OpenAsset err = %v", err)
	}
}

// TestNewGitHubSourceRejectsDotNames (A10).
func TestNewGitHubSourceRejectsDotNames(t *testing.T) {
	for _, repo := range []Repository{{Owner: ".", Name: "r"}, {Owner: "..", Name: "r"}, {Owner: "o", Name: "."}, {Owner: "o", Name: ".."}} {
		_, err := NewGitHubSource(GitHubOptions{Repository: repo, Client: http.DefaultClient, UserAgent: "demo/v1.0.0", Limits: testGitHubLimits()})
		if err == nil {
			t.Errorf("accepted %+v", repo)
		}
	}
	if _, err := NewGitHubSource(GitHubOptions{Repository: Repository{Owner: "o.k", Name: "r.name"}, Client: http.DefaultClient, UserAgent: "demo/v1.0.0", Limits: testGitHubLimits()}); err != nil {
		t.Fatalf("rejected dotted but valid names: %v", err)
	}
}

// TestSanitizeRemovesFormatControls (A11).
func TestSanitizeRemovesFormatControls(t *testing.T) {
	in := "ok\u202egnp.exe\u2066x\u200by\u2028z\u2029w\ufeffv"
	for name, got := range map[string]string{
		"diagnostic": sanitizeDiagnostic(in, 1024),
		"text":       sanitizeText(in),
	} {
		for _, r := range []rune{'\u202e', '\u2066', '\u200b', '\u2028', '\u2029', '\ufeff'} {
			if strings.ContainsRune(got, r) {
				t.Errorf("%s kept U+%04X: %q", name, r, got)
			}
		}
		if !strings.Contains(got, "ok") || !strings.Contains(got, "gnp.exe") {
			t.Errorf("%s lost printable text: %q", name, got)
		}
	}
}

// --- A4: source-level checks that had no failing test -------------------

// TestGitHubForbiddenRemainingZeroIsRateLimit: GitHub's primary rate-limit
// shape, 403 with X-RateLimit-Remaining 0 and no Retry-After.
func TestGitHubForbiddenRemainingZeroIsRateLimit(t *testing.T) {
	env := newGitHubEnv(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("X-RateLimit-Remaining", "0")
		w.Header().Set("X-RateLimit-Reset", "1900000000")
		w.WriteHeader(http.StatusForbidden)
	}, "")
	_, err := env.src.Latest(context.Background())
	var rl *RateLimitError
	if !errors.As(err, &rl) || rl.StatusCode != http.StatusForbidden || rl.Remaining != 0 || rl.Reset.Unix() != 1900000000 {
		t.Fatalf("err = %v", err)
	}
	// A plain 403 is not a rate limit.
	env = newGitHubEnv(t, func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusForbidden) }, "")
	if _, err := env.src.Latest(context.Background()); err == nil || errors.Is(err, ErrRateLimited) {
		t.Fatalf("plain 403 err = %v", err)
	}
}

// TestGitHubOpenAssetStatuses: non-2xx asset responses are errors, and 429
// is a RateLimitError.
func TestGitHubOpenAssetStatuses(t *testing.T) {
	rel := Release{ID: 1, Tag: "v1.0.0", Assets: []Asset{{ID: 7, Name: "demo", State: "uploaded", Size: 4}}}
	for status, rateLimited := range map[int]bool{http.StatusNotFound: false, http.StatusTooManyRequests: true, http.StatusInternalServerError: false} {
		env := newGitHubEnv(t, func(w http.ResponseWriter, _ *http.Request) {
			if status == http.StatusTooManyRequests {
				w.Header().Set("Retry-After", "1")
			}
			w.WriteHeader(status)
		}, "")
		rc, err := env.src.OpenAsset(context.Background(), rel, rel.Assets[0])
		if err == nil {
			_ = rc.Close()
			t.Fatalf("status %d: no error", status)
		}
		if errors.Is(err, ErrRateLimited) != rateLimited {
			t.Fatalf("status %d: err = %v", status, err)
		}
	}
}

// TestGitHubOpenAssetValidatesAsset: state, size and release membership are
// checked before any request is sent.
func TestGitHubOpenAssetValidatesAsset(t *testing.T) {
	var requests int
	env := newGitHubEnv(t, func(w http.ResponseWriter, _ *http.Request) {
		requests++
		_, _ = w.Write([]byte("body"))
	}, "")
	good := Asset{ID: 7, Name: "demo", State: "uploaded", Size: 4}
	rel := Release{ID: 1, Tag: "v1.0.0", Assets: []Asset{good}}
	cases := map[string]Asset{
		"open state": {ID: 7, Name: "demo", State: "open", Size: 4},
		"zero size":  {ID: 7, Name: "demo", State: "uploaded", Size: 0},
		"oversize":   {ID: 7, Name: "demo", State: "uploaded", Size: testGitHubLimits().Executable + 1},
		"foreign id": {ID: 99, Name: "demo", State: "uploaded", Size: 4},
	}
	for name, a := range cases {
		if rc, err := env.src.OpenAsset(context.Background(), rel, a); err == nil {
			_ = rc.Close()
			t.Errorf("%s: accepted", name)
		}
	}
	if requests != 0 {
		t.Fatalf("%d request(s) sent for invalid assets", requests)
	}
	rc, err := env.src.OpenAsset(context.Background(), rel, good)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(rc)
	_ = rc.Close()
	if string(b) != "body" {
		t.Fatalf("body %q", b)
	}
}

// TestRateLimitErrorText covers RateLimitError.Error and Unwrap.
func TestRateLimitErrorText(t *testing.T) {
	e := &RateLimitError{StatusCode: 429, Remaining: 3, RetryAfter: 2 * time.Second, Reset: time.Unix(1900000000, 0)}
	got := e.Error()
	for _, want := range []string{"status 429", "remaining 3", "retry-after 2s", "2030-03-17T17:46:40Z"} {
		if !strings.Contains(got, want) {
			t.Errorf("%q lacks %q", got, want)
		}
	}
	if !errors.Is(e, ErrRateLimited) {
		t.Fatal("does not unwrap to ErrRateLimited")
	}
	var nilErr *RateLimitError
	if nilErr.Error() != ErrRateLimited.Error() {
		t.Fatalf("nil receiver text %q", nilErr.Error())
	}
}

// TestReleaseIdentityRequired: a release with no ID or no tag is refused by
// both the fetched-release check and the listed-release check
// (0010-MADR A13: deleting either check survived the suite).
func TestReleaseIdentityRequired(t *testing.T) {
	for name, rel := range map[string]Release{
		"no id":  {ID: 0, Tag: "v1.0.0", Immutable: true},
		"no tag": {ID: 7, Tag: "", Immutable: true},
	} {
		if err := validateFetchedRelease(rel); err == nil || !strings.Contains(err.Error(), "incomplete") {
			t.Errorf("validateFetchedRelease, %s: err = %v", name, err)
		}
		if err := validateReleaseStructure(rel); err == nil || !strings.Contains(err.Error(), "incomplete") {
			t.Errorf("validateReleaseStructure, %s: err = %v", name, err)
		}
	}
}

// TestRedirectCap: the source's redirect policy stops at maxRedirects
// (0010-MADR A13: disabling the cap survived the suite).
func TestRedirectCap(t *testing.T) {
	src, err := NewGitHubSource(GitHubOptions{Repository: Repository{Owner: "owner", Name: "demo"},
		Client: &http.Client{}, UserAgent: "demo/v1.0.0", Limits: DefaultLimits()})
	if err != nil {
		t.Fatal(err)
	}
	req, err := http.NewRequest(http.MethodGet, "https://api.github.com/repos/owner/demo", nil)
	if err != nil {
		t.Fatal(err)
	}
	via := make([]*http.Request, maxRedirects)
	for i := range via {
		via[i] = req
	}
	if err := src.checkRedirect(req, via[:maxRedirects-1]); err != nil {
		t.Fatalf("redirect %d refused: %v", maxRedirects-1, err)
	}
	if err := src.checkRedirect(req, via); err == nil || !strings.Contains(err.Error(), "too many redirects") {
		t.Fatalf("redirect %d: err = %v, want too many redirects", maxRedirects, err)
	}
}

// TestGitHubSecondaryRateLimitWithoutHeaders: a 403 whose body names a
// secondary rate limit is a RateLimitError even with neither Retry-After
// nor X-RateLimit-Remaining, so CheckCached backs off
// (docs/decisions/0015-MADR-remediate-third-debugging-pass-findings.md A4).
func TestGitHubSecondaryRateLimitWithoutHeaders(t *testing.T) {
	noEnv(t, nil)
	requests := 0
	env := newGitHubEnv(t, func(w http.ResponseWriter, _ *http.Request) {
		requests++
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"message":"You have exceeded a secondary rate limit. Please wait a few minutes before you try again."}`))
	}, "")
	_, err := env.src.Latest(context.Background())
	var rl *RateLimitError
	if !errors.As(err, &rl) || rl.StatusCode != http.StatusForbidden {
		t.Fatalf("err = %v; isRateLimited=%t", err, errors.Is(err, ErrRateLimited))
	}
	setSeam(t, &timeNow, func() time.Time { return cacheNow })
	plat := Platform{OS: "linux", Arch: "amd64"}
	sel, err := NewExactAssetSelector([]Platform{plat})
	if err != nil {
		t.Fatal(err)
	}
	c, err := NewChecker(CheckerConfig{Source: env.src, Versions: NewStrictVersionPolicy(), Assets: sel, Limits: DefaultLimits()})
	if err != nil {
		t.Fatal(err)
	}
	req := CheckRequest{Product: "demo", CurrentVersion: "v1.0.0", CurrentBuild: ReleaseBuild, Platform: plat}
	store := &memStore{}
	requests = 0
	for range 3 {
		_, _ = c.CheckCached(context.Background(), req, store, time.Hour)
	}
	if requests != 1 {
		t.Fatalf("calls=%d over three CheckCached; want one, then the deferral", requests)
	}
}

// TestOpenAssetKeepsContentEncodedBytes: an asset is read as served. The
// request asks for no encoding, and a response a CDN encodes anyway is not
// decoded, so the size and digest checks see the published bytes
// (docs/decisions/0015-MADR-remediate-third-debugging-pass-findings.md A6).
func TestOpenAssetKeepsContentEncodedBytes(t *testing.T) {
	noEnv(t, nil)
	var asked string
	var gz bytes.Buffer
	zw := gzip.NewWriter(&gz)
	if _, err := zw.Write([]byte("the published asset bytes")); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	body := gz.Bytes()
	env := newGitHubEnv(t, func(w http.ResponseWriter, r *http.Request) {
		asked = r.Header.Get("Accept-Encoding")
		w.Header().Set("Content-Encoding", "gzip")
		_, _ = w.Write(body)
	}, "")
	asset := Asset{ID: 7, Name: "demo.gz", State: "uploaded", Size: int64(len(body))}
	rel := Release{ID: 1, Tag: "v1.0.0", Assets: []Asset{asset}}
	rc, err := env.src.OpenAsset(context.Background(), rel, asset)
	if err != nil {
		t.Fatal(err)
	}
	got, err := io.ReadAll(rc)
	_ = rc.Close()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, body) || asked != "identity" {
		t.Fatalf("received %d bytes (decoded=%t), Accept-Encoding %q; want the %d bytes as served and identity",
			len(got), !bytes.Equal(got, body), asked, len(body))
	}
}
