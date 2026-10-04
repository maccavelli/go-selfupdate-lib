package selfupdatetest

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/maccavelli/go-selfupdate-lib/selfupdate"
)

// RecordedRequest is one request either GitHubServer origin received.
type RecordedRequest struct {
	// Host is the request's host:port. The API and asset origins differ.
	Host string
	// Path is the request path.
	Path string
	// Authorization reports whether an Authorization header was present.
	Authorization bool
	// CredentialHeaders names the credential headers the request carried:
	// Authorization, and the header RequireCredential named. It never holds
	// a value (0010-MADR A14).
	CredentialHeaders HeaderNames
}

// HeaderNames lists canonical HTTP header names, joined by ", " as HTTP
// joins a list. It is a string, so RecordedRequest stays comparable.
type HeaderNames string

// authorizationHeader carries a bearer token, and is always watched.
const authorizationHeader = "Authorization"

// List returns the names, or nil when there are none.
func (h HeaderNames) List() []string {
	if h == "" {
		return nil
	}
	return strings.Split(string(h), ", ")
}

// Has reports whether name, in any case, is in the list.
func (h HeaderNames) Has(name string) bool {
	return slices.Contains(h.List(), http.CanonicalHeaderKey(name))
}

// GitHubServer is a fake GitHub REST API on one TLS origin. It serves
// releases/latest, releases/tags/{tag}, releases/assets/{id} and the paged
// release list for one repository. Each asset request answers 302 to a second TLS origin, which
// serves the bytes, as GitHub redirects to its download host. Both servers
// close when the test ends.
type GitHubServer struct {
	// APIBase is the API origin, for selfupdate.GitHubOptions.APIBaseURL.
	APIBase *url.URL
	// Client trusts both servers' certificates.
	Client *http.Client

	api, assets *httptest.Server
	prefix      string

	mu        sync.Mutex
	releases  []builtRelease
	latest    int // index into releases, or -1
	requests  []RecordedRequest
	limited   int
	limitHdr  http.Header
	truncated bool
	// credHeader and credValue are what RequireCredential requires.
	credHeader string
	credValue  string
}

// NewGitHubServer serves releases for owner/repo. Latest is the last
// release that is neither a draft nor a prerelease, as on GitHub.
func NewGitHubServer(t testing.TB, owner, repo string, releases ...ReleaseSpec) *GitHubServer {
	t.Helper()
	g := &GitHubServer{prefix: "/repos/" + owner + "/" + repo + "/releases/", latest: -1}
	var next int64
	for i, spec := range releases {
		g.releases = append(g.releases, build(spec, &next, "https://github.invalid/"+owner+"/"+repo+"/releases/tag/"+spec.Tag))
		if !spec.Draft && !spec.Prerelease {
			g.latest = i
		}
	}
	g.assets = httptest.NewTLSServer(http.HandlerFunc(g.serveAsset))
	t.Cleanup(g.assets.Close)
	g.api = httptest.NewTLSServer(http.HandlerFunc(g.serveAPI))
	t.Cleanup(g.api.Close)

	base, err := url.Parse(g.api.URL)
	if err != nil {
		t.Fatal(err)
	}
	g.APIBase = base
	pool := x509.NewCertPool()
	pool.AddCert(g.api.Certificate())
	pool.AddCert(g.assets.Certificate())
	transport := &http.Transport{
		TLSClientConfig:   &tls.Config{RootCAs: pool, MinVersion: tls.VersionTLS12},
		ForceAttemptHTTP2: true,
	}
	g.Client = &http.Client{Transport: transport}
	t.Cleanup(transport.CloseIdleConnections)
	return g
}

// RateLimit makes every later API request fail with status and header,
// such as 403 with X-RateLimit-Remaining: 0, or 429 with Retry-After. A
// zero status serves normally again. Asset downloads are not limited.
func (g *GitHubServer) RateLimit(status int, header http.Header) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.limited = status
	g.limitHdr = header.Clone()
}

// RequireToken makes every later API request without the header
// "Authorization: Bearer <token>" fail with 401, as GitHub does for a
// missing or bad token on a private repository. An empty token serves
// anonymous requests again. Asset downloads are not affected. It is
// RequireCredential("", token).
func (g *GitHubServer) RequireToken(token string) {
	g.RequireCredential("", token)
}

// RequireCredential makes every later API request without the credential
// fail with 401. It requires what selfupdate sends for a Credential with
// this Header and Value: an empty header means "Authorization: Bearer
// <value>", and a named header, Authorization included, carries the value
// as it is. An empty value serves anonymous requests again. Asset
// downloads are not affected, and Requests records the header's name on
// either origin, so a test can prove it never reaches the asset one.
func (g *GitHubServer) RequireCredential(header, value string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.credHeader, g.credValue = header, value
}

// TruncateAssets makes the asset origin advertise each body's full length
// and send only its first half, so the download ends early.
func (g *GitHubServer) TruncateAssets(truncate bool) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.truncated = truncate
}

// Requests returns every request either origin received, in order.
func (g *GitHubServer) Requests() []RecordedRequest {
	g.mu.Lock()
	defer g.mu.Unlock()
	return append([]RecordedRequest(nil), g.requests...)
}

func (g *GitHubServer) record(r *http.Request) {
	g.mu.Lock()
	defer g.mu.Unlock()
	watched := []string{authorizationHeader}
	if h := http.CanonicalHeaderKey(g.credHeader); h != "" && h != authorizationHeader {
		watched = append(watched, h)
	}
	var present []string
	for _, h := range watched {
		if r.Header.Get(h) != "" {
			present = append(present, h)
		}
	}
	g.requests = append(g.requests, RecordedRequest{
		Host: r.Host, Path: r.URL.Path, Authorization: r.Header.Get(authorizationHeader) != "",
		CredentialHeaders: HeaderNames(strings.Join(present, ", ")),
	})
}

type releaseJSON struct {
	ID         int64       `json:"id"`
	TagName    string      `json:"tag_name"`
	HTMLURL    string      `json:"html_url"`
	Draft      bool        `json:"draft"`
	Prerelease bool        `json:"prerelease"`
	Immutable  bool        `json:"immutable"`
	Assets     []assetJSON `json:"assets"`
}

type assetJSON struct {
	ID     int64  `json:"id"`
	Name   string `json:"name"`
	State  string `json:"state"`
	Size   int64  `json:"size"`
	Digest string `json:"digest,omitempty"`
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	body, err := json.Marshal(v)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if _, err := w.Write(body); err != nil {
		return
	}
}

// writeMessage answers with GitHub's error shape, {"message": msg}.
func writeMessage(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"message": msg})
}

func notFound(w http.ResponseWriter) {
	writeMessage(w, http.StatusNotFound, "Not Found")
}

func (g *GitHubServer) serveAPI(w http.ResponseWriter, r *http.Request) {
	g.record(r)
	g.mu.Lock()
	limited, header, credHeader, credValue := g.limited, g.limitHdr, g.credHeader, g.credValue
	g.mu.Unlock()
	if limited != 0 {
		for k, v := range header {
			w.Header()[k] = append([]string(nil), v...)
		}
		writeMessage(w, limited, "API rate limit exceeded")
		return
	}
	if credValue != "" {
		name, want := credHeader, credValue
		if name == "" {
			name, want = authorizationHeader, "Bearer "+credValue
		}
		if r.Header.Get(name) != want {
			writeMessage(w, http.StatusUnauthorized, "Bad credentials")
			return
		}
	}
	if r.Method == http.MethodGet && r.URL.Path == strings.TrimSuffix(g.prefix, "/") {
		g.serveList(w, r)
		return
	}
	rest, ok := strings.CutPrefix(r.URL.Path, g.prefix)
	if !ok || r.Method != http.MethodGet {
		notFound(w)
		return
	}
	switch {
	case rest == "latest":
		g.mu.Lock()
		i := g.latest
		g.mu.Unlock()
		if i < 0 {
			notFound(w)
			return
		}
		g.serveRelease(w, i)
	case strings.HasPrefix(rest, "tags/"):
		tag := strings.TrimPrefix(rest, "tags/")
		for i := range g.releases {
			if g.releases[i].release.Tag == tag {
				g.serveRelease(w, i)
				return
			}
		}
		notFound(w)
	case strings.HasPrefix(rest, "assets/"):
		id, err := strconv.ParseInt(strings.TrimPrefix(rest, "assets/"), 10, 64)
		if err != nil {
			notFound(w)
			return
		}
		if _, ok := g.body(id); !ok {
			notFound(w)
			return
		}
		http.Redirect(w, r, g.assets.URL+"/download/"+strconv.FormatInt(id, 10), http.StatusFound)
	default:
		notFound(w)
	}
}

// serveList serves the releases in declared order, paged by page and
// per_page (default 30, at most 100), as GitHub does.
func (g *GitHubServer) serveList(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	page, perPage := 1, 30
	if v, err := strconv.Atoi(q.Get("page")); err == nil && v > 0 {
		page = v
	}
	if v, err := strconv.Atoi(q.Get("per_page")); err == nil && v > 0 {
		perPage = min(v, 100)
	}
	out := []releaseJSON{}
	for i := (page - 1) * perPage; i < len(g.releases) && i < page*perPage; i++ {
		out = append(out, releaseDoc(g.releases[i].release))
	}
	writeJSON(w, http.StatusOK, out)
}

func releaseDoc(rel selfupdate.Release) releaseJSON {
	out := releaseJSON{
		ID: rel.ID, TagName: rel.Tag, HTMLURL: rel.URL,
		Draft: rel.Draft, Prerelease: rel.Prerelease, Immutable: rel.Immutable,
		Assets: make([]assetJSON, 0, len(rel.Assets)),
	}
	for _, a := range rel.Assets {
		out.Assets = append(out.Assets, assetJSON(a))
	}
	return out
}

func (g *GitHubServer) serveRelease(w http.ResponseWriter, i int) {
	writeJSON(w, http.StatusOK, releaseDoc(g.releases[i].release))
}

func (g *GitHubServer) body(id int64) ([]byte, bool) {
	for _, b := range g.releases {
		if body, ok := b.bodies[id]; ok {
			return body, true
		}
	}
	return nil, false
}

func (g *GitHubServer) serveAsset(w http.ResponseWriter, r *http.Request) {
	g.record(r)
	id, err := strconv.ParseInt(strings.TrimPrefix(r.URL.Path, "/download/"), 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	body, ok := g.body(id)
	if !ok {
		http.NotFound(w, r)
		return
	}
	g.mu.Lock()
	truncated := g.truncated
	g.mu.Unlock()
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Length", strconv.Itoa(len(body)))
	if truncated {
		body = body[:len(body)/2]
	}
	if _, err := w.Write(body); err != nil {
		return
	}
}
