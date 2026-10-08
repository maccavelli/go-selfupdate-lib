package selfupdate

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"
)

const (
	gitHubAPIVersion  = "2026-03-10"
	gitHubAcceptJSON  = "application/vnd.github+json"
	gitHubAcceptAsset = "application/octet-stream"
	defaultAPIOrigin  = "https://api.github.com"
	maxRedirects      = 10
)

var (
	lookupEnv = os.Getenv
	timeNow   = time.Now
)

// GitHubSource is the canonical GitHub Releases implementation of ReleaseSource.
type GitHubSource struct {
	repo      Repository
	client    *http.Client
	apiBase   *url.URL
	userAgent string
	token     string // the explicit Token, else GH_TOKEN, else GITHUB_TOKEN
	explicit  bool   // token came from GitHubOptions.Token
	envName   string // the variable token came from, when not explicit
	provider  CredentialProvider
	observer  CredentialObserver
	limits    Limits
	now       func() time.Time
	cred      credentialState
}

// credentialState is the source's resolved credential. A credential, once
// resolved, is shared by every later request. Anonymity, and the one retry
// after a 401, are kept only for the run that decided them: a later run, or
// a later check, resolves again (0010-MADR A2). A source used outside any
// run keeps them for its lifetime, as before.
type credentialState struct {
	mu           sync.Mutex
	resolved     bool
	has          bool
	cred         Credential
	fromProvider bool
	// perRun marks an environment token the source fell back to because the
	// provider declined: it holds for the run that resolved it only, so a
	// later run asks the provider again (0015-MADR A2).
	perRun     bool
	anonRun    *runMark // the run that resolved anonymity, or a perRun token
	retried    bool
	retriedRun *runMark // the run that used the one retry
	accepted   bool
	// inflight is closed when a resolution in progress ends. The provider
	// runs with no lock held, so a request waiting for it can honour its
	// own context (0010-MADR A7).
	inflight chan struct{}
}

// runKey marks one run, or one check, in its context, so the source can
// tell one run's negative credential outcome from the next's.
type runKey struct{}

// runMark identifies one run; only its address matters.
type runMark struct{ _ byte }

// withRunMark returns ctx marked as a new run.
func withRunMark(ctx context.Context) context.Context {
	return context.WithValue(ctx, runKey{}, &runMark{})
}

// runMarkOf is ctx's run, or nil outside any run.
func runMarkOf(ctx context.Context) *runMark {
	if m, ok := ctx.Value(runKey{}).(*runMark); ok {
		return m
	}
	return nil
}

// inStream reports whether ctx belongs to a run started with Start, where a
// provider may prompt (0010-MADR C10).
func inStream(ctx context.Context) bool {
	_, ok := ctx.Value(streamKey{}).(*Stream)
	return ok
}

// NewGitHubSource validates options, clones the supplied client, and resolves
// the token once. It never mutates the caller's client or URL. The clone's
// CheckRedirect is the source's own; the caller's is never called.
func NewGitHubSource(opts GitHubOptions) (*GitHubSource, error) {
	if opts.Client == nil {
		return nil, fmt.Errorf("selfupdate: github client is required")
	}
	if err := opts.Limits.valid(); err != nil {
		return nil, err
	}
	if err := validateGitHubName("owner", opts.Repository.Owner); err != nil {
		return nil, err
	}
	if err := validateGitHubName("repository", opts.Repository.Name); err != nil {
		return nil, err
	}
	if err := validateUserAgent(opts.UserAgent); err != nil {
		return nil, err
	}
	base, err := normalizeAPIBase(opts.APIBaseURL)
	if err != nil {
		return nil, err
	}
	cloned := *opts.Client
	token, envName := resolveToken(opts.Token)
	src := &GitHubSource{
		repo:      opts.Repository,
		client:    &cloned,
		apiBase:   base,
		userAgent: opts.UserAgent,
		token:     token,
		explicit:  opts.Token != "",
		envName:   envName,
		limits:    opts.Limits,
		now:       timeNow,
	}
	if !isNil(opts.Credentials) {
		src.provider = opts.Credentials
	}
	if !isNil(opts.Observer) {
		src.observer = opts.Observer
	}
	src.client.CheckRedirect = src.checkRedirect
	return src, nil
}

func validateGitHubName(kind, name string) error {
	if name == "" {
		return fmt.Errorf("selfupdate: github %s is required", kind)
	}
	if strings.ContainsAny(name, `/\:?`) || strings.IndexFunc(name, unicode.IsControl) >= 0 {
		return fmt.Errorf("selfupdate: github %s contains illegal characters", kind)
	}
	if name == "." || name == ".." {
		return fmt.Errorf("selfupdate: github %s must not be a dot segment", kind)
	}
	return nil
}

func validateUserAgent(ua string) error {
	if strings.TrimSpace(ua) == "" {
		return fmt.Errorf("selfupdate: user-agent is required")
	}
	if strings.IndexFunc(ua, unicode.IsControl) >= 0 {
		return fmt.Errorf("selfupdate: user-agent contains illegal characters")
	}
	return nil
}

// resolveToken returns the explicit token, else GH_TOKEN, else
// GITHUB_TOKEN, with the name of the variable it came from.
func resolveToken(explicit string) (token, envName string) {
	if explicit != "" {
		return explicit, ""
	}
	for _, name := range []string{"GH_TOKEN", "GITHUB_TOKEN"} {
		if v := lookupEnv(name); v != "" {
			return v, name
		}
	}
	return "", ""
}

func normalizeAPIBase(raw *url.URL) (*url.URL, error) {
	var base *url.URL
	if raw == nil {
		parsed, err := url.Parse(defaultAPIOrigin)
		if err != nil {
			return nil, err
		}
		base = parsed
	} else {
		cloned := *raw
		base = &cloned
	}
	if base.User != nil || base.RawQuery != "" || base.Fragment != "" {
		return nil, fmt.Errorf("selfupdate: github API base must not include user, query, or fragment")
	}
	if base.Scheme == "" || base.Host == "" {
		return nil, fmt.Errorf("selfupdate: github API base is incomplete")
	}
	if !isLoopbackHost(base.Hostname()) && !strings.EqualFold(base.Scheme, "https") {
		return nil, fmt.Errorf("selfupdate: github API base must be https")
	}
	base.Path = strings.TrimSuffix(base.Path, "/")
	return base, nil
}

func isLoopbackHost(host string) bool {
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func (s *GitHubSource) checkRedirect(req *http.Request, via []*http.Request) error {
	if len(via) >= maxRedirects {
		return fmt.Errorf("selfupdate: too many redirects")
	}
	// Any non-loopback hop must stay on HTTPS (mcplib 0005-PLAN §4.2;
	// 0003-MADR A3), and a plain-http loopback hop is allowed only from an
	// API that is itself on loopback (0010-MADR A10). The URL is not
	// echoed: a redirect target can carry signed query parameters.
	if !strings.EqualFold(req.URL.Scheme, "https") &&
		(!isLoopbackHost(req.URL.Hostname()) || !isLoopbackHost(s.apiBase.Hostname())) {
		return fmt.Errorf("selfupdate: refusing redirect to a non-https location")
	}
	if !sameOrigin(req.URL, s.apiBase) {
		// Another origin gets only the source's fixed headers. A credential
		// goes only to the origin it was requested for (0004-MADR G10),
		// whichever header it was sent in, even one the source's credential
		// has since moved from (0015-MADR A3).
		for k := range req.Header {
			if !forwardedHeaders[http.CanonicalHeaderKey(k)] {
				req.Header.Del(k)
			}
		}
	}
	return nil
}

// forwardedHeaders are the headers a request keeps on a hop to another
// origin: the ones newRequest sets that carry no credential.
var forwardedHeaders = map[string]bool{
	"Accept":               true,
	"Accept-Encoding":      true,
	"User-Agent":           true,
	"X-Github-Api-Version": true,
}

func sameOrigin(u, base *url.URL) bool {
	return strings.EqualFold(u.Scheme, base.Scheme) && strings.EqualFold(u.Host, base.Host)
}

func (s *GitHubSource) apiURL(elem ...string) string {
	var b strings.Builder
	b.WriteString(strings.TrimRight(s.apiBase.String(), "/"))
	for _, e := range elem {
		b.WriteByte('/')
		b.WriteString(url.PathEscape(e))
	}
	return b.String()
}

// Latest implements ReleaseSource.
func (s *GitHubSource) Latest(ctx context.Context) (Release, error) {
	return s.getRelease(ctx, s.apiURL("repos", s.repo.Owner, s.repo.Name, "releases", "latest"))
}

// ByTag implements ReleaseSource.
func (s *GitHubSource) ByTag(ctx context.Context, tag string) (Release, error) {
	// "." and ".." are refused as owner and repository dot names are: a
	// normalising proxy would resolve them to another path (0010-MADR A11).
	if tag == "" || tag == "." || tag == ".." || strings.ContainsAny(tag, `/\:`) || strings.IndexFunc(tag, unicode.IsControl) >= 0 {
		return Release{}, fmt.Errorf("selfupdate: invalid release tag %q", tag)
	}
	rel, err := s.getRelease(ctx, s.apiURL("repos", s.repo.Owner, s.repo.Name, "releases", "tags", tag))
	if err != nil {
		return Release{}, err
	}
	if rel.Tag != tag {
		return Release{}, fmt.Errorf("selfupdate: github returned release %q for tag %q: %w", rel.Tag, tag, ErrIntegrity)
	}
	return rel, nil
}

type githubReleaseJSON struct {
	ID         int64             `json:"id"`
	TagName    string            `json:"tag_name"`
	HTMLURL    string            `json:"html_url"`
	Draft      bool              `json:"draft"`
	Prerelease bool              `json:"prerelease"`
	Immutable  bool              `json:"immutable"`
	Assets     []githubAssetJSON `json:"assets"`
}

type githubAssetJSON struct {
	ID     int64  `json:"id"`
	Name   string `json:"name"`
	State  string `json:"state"`
	Size   int64  `json:"size"`
	Digest string `json:"digest"`
}

func (s *GitHubSource) getRelease(ctx context.Context, rawURL string) (rel Release, err error) {
	resp, err := s.send(ctx, rawURL, gitHubAcceptJSON)
	if err != nil {
		return Release{}, err
	}
	defer func() {
		if cerr := resp.Body.Close(); cerr != nil && err == nil {
			err = cerr
		}
	}()
	// Look at the status before the body: an error body is read only up to
	// ErrorBody, truncated rather than refused, so an oversized 429 still
	// maps to RateLimitError (0003-MADR A8).
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		errBody, rerr := readTruncated(resp.Body, s.limits.ErrorBody)
		if rerr != nil {
			return Release{}, rerr
		}
		statusErr := s.mapStatus(resp, errBody)
		if resp.StatusCode == http.StatusNotFound {
			// No stable release yet, or no such tag: the same answer until
			// one is published, so CheckCached may cache it (0015-MADR A1).
			return Release{}, fmt.Errorf("%w: %w", ErrNoRelease, statusErr)
		}
		return Release{}, statusErr
	}
	body, err := readBounded(resp.Body, s.limits.ReleaseJSON)
	if err != nil {
		return Release{}, err
	}
	var raw githubReleaseJSON
	if err := decodeJSON(body, &raw); err != nil {
		return Release{}, err
	}
	rel, err = mapRelease(raw)
	if err != nil {
		return Release{}, err
	}
	if err := validateFetchedRelease(rel); err != nil {
		return Release{}, err
	}
	return rel, nil
}

const (
	// listPageSize keeps one page of release entries, which carry full
	// release notes, well inside Limits.ReleaseJSON (0005-MADR E3).
	listPageSize     = 30
	defaultListLimit = 90
	maxListLimit     = 300
)

func listLimit(o ListOptions) (int, error) {
	switch {
	case o.Limit == 0:
		return defaultListLimit, nil
	case o.Limit < 0 || o.Limit > maxListLimit:
		return 0, fmt.Errorf("selfupdate: list limit %d is outside 1..%d", o.Limit, maxListLimit)
	default:
		return o.Limit, nil
	}
}

// ListReleases implements ReleaseLister. It pages
// GET /repos/{owner}/{repo}/releases, listPageSize at a time, through the
// same credential, redirect and rate-limit handling as Latest, and stops
// at the limit or a short page. Each page is bounded by
// Limits.ReleaseJSON. Entries are not structure-checked: discovery checks
// the release it chooses (0005-MADR amendment E5).
func (s *GitHubSource) ListReleases(ctx context.Context, o ListOptions) ([]Release, error) {
	limit, err := listLimit(o)
	if err != nil {
		return nil, err
	}
	var out []Release
	for page := 1; len(out) < limit; page++ {
		raws, err := s.getReleasePage(ctx, page)
		if err != nil {
			return nil, err
		}
		for _, raw := range raws {
			if len(out) == limit {
				break
			}
			out = append(out, mapReleaseUnchecked(raw))
		}
		if len(raws) < listPageSize {
			break
		}
	}
	return out, nil
}

func (s *GitHubSource) getReleasePage(ctx context.Context, page int) (raws []githubReleaseJSON, err error) {
	rawURL := s.apiURL("repos", s.repo.Owner, s.repo.Name, "releases") +
		"?per_page=" + strconv.Itoa(listPageSize) + "&page=" + strconv.Itoa(page)
	resp, err := s.send(ctx, rawURL, gitHubAcceptJSON)
	if err != nil {
		return nil, err
	}
	defer func() {
		if cerr := resp.Body.Close(); cerr != nil && err == nil {
			err = cerr
		}
	}()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		errBody, rerr := readTruncated(resp.Body, s.limits.ErrorBody)
		if rerr != nil {
			return nil, rerr
		}
		return nil, s.mapStatus(resp, errBody)
	}
	body, err := readBounded(resp.Body, s.limits.ReleaseJSON)
	if err != nil {
		return nil, err
	}
	if err := decodeJSON(body, &raws); err != nil {
		return nil, err
	}
	return raws, nil
}

// mapReleaseUnchecked maps a listed release without checking its assets'
// structure.
func mapReleaseUnchecked(raw githubReleaseJSON) Release {
	rel := Release{
		ID:         raw.ID,
		Tag:        raw.TagName,
		URL:        raw.HTMLURL,
		Draft:      raw.Draft,
		Prerelease: raw.Prerelease,
		Immutable:  raw.Immutable,
		Assets:     make([]Asset, 0, len(raw.Assets)),
	}
	for _, a := range raw.Assets {
		rel.Assets = append(rel.Assets, Asset(a))
	}
	return rel
}

// validateReleaseStructure is the structure check mapRelease applies, plus
// the identity validateFetchedRelease requires, for a listed release that
// discovery has chosen.
func validateReleaseStructure(rel Release) error {
	if rel.ID <= 0 || rel.Tag == "" {
		return fmt.Errorf("selfupdate: release metadata is incomplete")
	}
	for _, a := range rel.Assets {
		if err := validateAssetStructure(a); err != nil {
			return err
		}
	}
	return nil
}

// mapRelease checks only the structure of every asset. State, size and digest
// are validated for the selected binary and manifest alone, by the Updater,
// so an unrelated extra asset cannot make a release unusable (0003-MADR A1).
func mapRelease(raw githubReleaseJSON) (Release, error) {
	rel := Release{
		ID:         raw.ID,
		Tag:        raw.TagName,
		URL:        raw.HTMLURL,
		Draft:      raw.Draft,
		Prerelease: raw.Prerelease,
		Immutable:  raw.Immutable,
		Assets:     make([]Asset, 0, len(raw.Assets)),
	}
	for _, a := range raw.Assets {
		asset := Asset(a)
		if err := validateAssetStructure(asset); err != nil {
			return Release{}, err
		}
		rel.Assets = append(rel.Assets, asset)
	}
	return rel, nil
}

// validateAssetStructure checks the identity every asset must have: an ID,
// and a name that is a basename without control characters.
func validateAssetStructure(a Asset) error {
	if a.ID <= 0 || a.Name == "" {
		return fmt.Errorf("selfupdate: asset metadata is incomplete")
	}
	// "." and ".." contain no separator but are not names of files
	// (0004-MADR R9).
	if a.Name == "." || a.Name == ".." || strings.ContainsAny(a.Name, `/\`) || strings.IndexFunc(a.Name, unicode.IsControl) >= 0 {
		return fmt.Errorf("selfupdate: asset name %q is not a basename", a.Name)
	}
	return nil
}

func validateFetchedRelease(rel Release) error {
	if rel.ID <= 0 || rel.Tag == "" {
		return fmt.Errorf("selfupdate: release metadata is incomplete")
	}
	if rel.Draft {
		return fmt.Errorf("selfupdate: release %q is a draft", rel.Tag)
	}
	if !rel.Immutable {
		return fmt.Errorf("selfupdate: release %q is not immutable: %w", rel.Tag, ErrMutableRelease)
	}
	return nil
}

func assetBelongsToRelease(rel Release, asset Asset) error {
	if len(rel.Assets) == 0 {
		return nil
	}
	for _, a := range rel.Assets {
		if a.ID == asset.ID {
			return nil
		}
	}
	return fmt.Errorf("selfupdate: asset %d is not part of release %q", asset.ID, rel.Tag)
}

// validateAssetMetadata is the full check for an asset about to be used:
// structure, uploaded state, a positive size within maxSize, and digest
// syntax.
func validateAssetMetadata(a Asset, maxSize int64) error {
	if err := validateAssetStructure(a); err != nil {
		return err
	}
	if a.State != AssetStateUploaded {
		return fmt.Errorf("selfupdate: asset %s is not uploaded", a.Name)
	}
	if a.Size <= 0 {
		return fmt.Errorf("selfupdate: asset %s has non-positive size: %w", a.Name, ErrIntegrity)
	}
	if a.Size > maxSize {
		return fmt.Errorf("selfupdate: asset %s size %d exceeds limit %d: %w", a.Name, a.Size, maxSize, ErrIntegrity)
	}
	if a.Digest != "" {
		if _, err := parseGitHubDigest(a.Digest); err != nil {
			return err
		}
	}
	return nil
}

// OpenAsset implements ReleaseSource. The body is fetched from the asset API
// path derived from owner, repository, and asset ID.
func (s *GitHubSource) OpenAsset(ctx context.Context, rel Release, asset Asset) (io.ReadCloser, error) {
	if err := assetBelongsToRelease(rel, asset); err != nil {
		return nil, err
	}
	if err := validateAssetMetadata(asset, s.limits.Executable); err != nil {
		return nil, err
	}
	rawURL := s.apiURL("repos", s.repo.Owner, s.repo.Name, "releases", "assets", strconv.FormatInt(asset.ID, 10))
	resp, err := s.send(ctx, rawURL, gitHubAcceptAsset)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		body, readErr := readTruncated(resp.Body, s.limits.ErrorBody)
		closeErr := resp.Body.Close()
		if readErr != nil {
			return nil, errors.Join(readErr, closeErr)
		}
		statusErr := s.mapStatus(resp, body)
		if closeErr != nil {
			return nil, errors.Join(statusErr, closeErr)
		}
		return nil, statusErr
	}
	return resp.Body, nil
}

func (s *GitHubSource) newRequest(ctx context.Context, method, rawURL, accept string, cred *Credential) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, method, rawURL, http.NoBody)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", accept)
	req.Header.Set("X-GitHub-Api-Version", gitHubAPIVersion)
	req.Header.Set("User-Agent", s.userAgent)
	if accept == gitHubAcceptAsset {
		// An asset is read as published. Asking for no encoding also stops
		// the transport from decoding one a CDN applies anyway, so the size
		// and digest checks see the published bytes (0015-MADR A6).
		req.Header.Set("Accept-Encoding", "identity")
	}
	if cred != nil && sameOrigin(req.URL, s.apiBase) {
		if cred.Header == "" {
			req.Header.Set("Authorization", "Bearer "+string(cred.Value))
		} else {
			req.Header.Set(cred.Header, string(cred.Value))
		}
	}
	return req, nil
}

// send issues one GET with the source's credential attached when the URL
// is on the API origin. A 401 from the API origin to a provider's
// credential asks the provider once more and retries once with a different
// credential; this happens at most once per run, so a provider that prompts
// is never asked in a loop. A 401 from another origin, which never saw the
// credential, is returned as it is (0010-MADR A6). The first 2xx to a
// credentialed request tells the Observer (0004-MADR G10).
func (s *GitHubSource) send(ctx context.Context, rawURL, accept string) (*http.Response, error) {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return nil, err
	}
	var cred *Credential
	if sameOrigin(parsed, s.apiBase) {
		if cred, err = s.credential(ctx); err != nil {
			return nil, err
		}
	}
	resp, err := s.do(ctx, rawURL, accept, cred)
	if err != nil {
		return nil, err
	}
	if cred != nil && resp.StatusCode == http.StatusUnauthorized &&
		resp.Request != nil && sameOrigin(resp.Request.URL, s.apiBase) {
		if next := s.refresh(ctx, cred); next != nil {
			drainClose(resp)
			cred = next
			if resp, err = s.do(ctx, rawURL, accept, cred); err != nil {
				return nil, err
			}
		}
	}
	if cred != nil && resp.StatusCode >= 200 && resp.StatusCode <= 299 {
		s.accept(ctx, *cred)
	}
	return resp, nil
}

func (s *GitHubSource) do(ctx context.Context, rawURL, accept string, cred *Credential) (*http.Response, error) {
	req, err := s.newRequest(ctx, http.MethodGet, rawURL, accept, cred)
	if err != nil {
		return nil, err
	}
	resp, err := s.client.Do(req)
	if err != nil {
		// A refused redirect, or a failure on the redirected hop, names the
		// redirect URL, whose query can be a signature (0010-MADR A5).
		var ue *url.Error
		if errors.As(err, &ue) {
			ue.URL = redactURL(ue.URL)
		}
		return nil, err
	}
	return resp, nil
}

// redactURL is raw without its user information, query and fragment.
func redactURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return "<unparsable URL>"
	}
	u.User, u.RawQuery, u.ForceQuery, u.Fragment, u.RawFragment = nil, "", false, "", ""
	return u.String()
}

// drainClose discards a refused response before its retry. Nothing depends
// on it, so its errors must not change the outcome.
func drainClose(resp *http.Response) {
	_, cerr := io.Copy(io.Discard, io.LimitReader(resp.Body, 64<<10))
	advisory(errors.Join(cerr, resp.Body.Close()))
}

// WithCredentials implements CredentialedSource. The copy shares the
// repository, API base, user agent, explicit and environment tokens,
// observer and limits. Its provider is p (none when p is nil), its
// credential state is new, and its client is its own copy. A redirect to
// another origin carries only Accept, Accept-Encoding, User-Agent and
// X-GitHub-Api-Version, so no credential follows it.
func (s *GitHubSource) WithCredentials(p CredentialProvider) ReleaseSource {
	client := *s.client
	c := &GitHubSource{
		repo:      s.repo,
		client:    &client,
		apiBase:   s.apiBase,
		userAgent: s.userAgent,
		token:     s.token,
		explicit:  s.explicit,
		envName:   s.envName,
		observer:  s.observer,
		limits:    s.limits,
		now:       s.now,
	}
	if !isNil(p) {
		c.provider = p
	}
	c.client.CheckRedirect = c.checkRedirect
	return c
}

// credential returns the source's credential, resolving it when nothing
// usable is known: the explicit Token, else the provider, else the
// environment token. Nil means anonymous. One resolution runs at a time,
// with no lock held; a request that waits for it honours its own context.
func (s *GitHubSource) credential(ctx context.Context) (*Credential, error) {
	mark := runMarkOf(ctx)
	for {
		s.cred.mu.Lock()
		if s.cred.resolved && ((s.cred.has && !s.cred.perRun) || s.cred.anonRun == mark) {
			has, c := s.cred.has, s.cred.cred
			s.cred.mu.Unlock()
			if !has {
				return nil, nil
			}
			return &c, nil
		}
		if wait := s.cred.inflight; wait != nil {
			s.cred.mu.Unlock()
			select {
			case <-wait:
				continue
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}
		done := make(chan struct{})
		s.cred.inflight = done
		s.cred.mu.Unlock()

		c, has, fromProvider, err := s.resolve(ctx)

		s.cred.mu.Lock()
		s.cred.inflight = nil
		close(done)
		if err == nil {
			s.cred.resolved, s.cred.has, s.cred.cred, s.cred.fromProvider = true, has, c, fromProvider
			s.cred.perRun = has && !fromProvider && !s.explicit && s.provider != nil
			s.cred.anonRun = mark
		}
		s.cred.mu.Unlock()
		if err != nil {
			return nil, err
		}
		if !has {
			return nil, nil
		}
		return &c, nil
	}
}

// resolve finds the credential. It holds no lock: a provider may block on
// the host.
func (s *GitHubSource) resolve(ctx context.Context) (c Credential, has, fromProvider bool, err error) {
	if s.explicit {
		return Credential{Value: []byte(s.token), Source: "token"}, true, false, nil
	}
	if s.provider != nil {
		pc, perr := s.provider.Credential(ctx, CredentialRequest{Origin: s.origin(), Interactive: inStream(ctx)})
		switch {
		case perr == nil:
			if verr := validateCredential(pc); verr != nil {
				return Credential{}, false, false, verr
			}
			return pc, true, true, nil
		case !errors.Is(perr, ErrNoCredential):
			return Credential{}, false, false, perr
		}
	}
	if s.token != "" {
		return Credential{Value: []byte(s.token), Source: "env:" + s.envName}, true, false, nil
	}
	return Credential{}, false, false, nil
}

// refresh asks the provider once more after a 401, at most once per run. It
// returns the new credential when it differs from the refused one, and nil
// otherwise: the caller then returns the 401. The provider runs with no
// lock held.
func (s *GitHubSource) refresh(ctx context.Context, refused *Credential) *Credential {
	mark := runMarkOf(ctx)
	s.cred.mu.Lock()
	if !s.cred.fromProvider || (s.cred.retried && s.cred.retriedRun == mark) {
		s.cred.mu.Unlock()
		return nil
	}
	s.cred.retried, s.cred.retriedRun = true, mark
	s.cred.mu.Unlock()
	c, err := s.provider.Credential(ctx, CredentialRequest{
		Origin:      s.origin(),
		Cause:       fmt.Errorf("selfupdate: github http %d: credential from %s refused", http.StatusUnauthorized, sanitizeText(refused.Source)),
		Interactive: inStream(ctx),
	})
	if err != nil || validateCredential(c) != nil || sameCredential(c, *refused) {
		return nil
	}
	s.cred.mu.Lock()
	s.cred.cred = c
	s.cred.mu.Unlock()
	return &c
}

func (s *GitHubSource) accept(ctx context.Context, c Credential) {
	s.cred.mu.Lock()
	first := !s.cred.accepted
	s.cred.accepted = true
	s.cred.mu.Unlock()
	if first && s.observer != nil {
		s.observer.Accepted(ctx, c)
	}
}

func (s *GitHubSource) origin() *url.URL {
	u := *s.apiBase
	return &u
}

// mapStatus maps a non-2xx response; every caller has filtered 2xx.
func (s *GitHubSource) mapStatus(resp *http.Response, body []byte) error {
	if resp.StatusCode == http.StatusTooManyRequests || rateLimitedForbidden(resp, body) {
		return parseRateLimit(resp, s.now)
	}
	diag := sanitizeDiagnostic(string(body), s.limits.ErrorBody)
	return fmt.Errorf("selfupdate: github http %d: %s", resp.StatusCode, diag)
}

// maxRetryAfterSeconds is the largest Retry-After, in seconds, that fits in a
// time.Duration.
const maxRetryAfterSeconds = math.MaxInt64 / int64(time.Second)

// rateLimitedForbidden reports a 403 that is a rate limit: one with
// Retry-After, with no requests remaining, or whose body names a secondary
// rate limit, which GitHub may send without either header (0015-MADR A4).
func rateLimitedForbidden(resp *http.Response, body []byte) bool {
	if resp.StatusCode != http.StatusForbidden {
		return false
	}
	if resp.Header.Get("Retry-After") != "" || resp.Header.Get("X-RateLimit-Remaining") == "0" {
		return true
	}
	return bytes.Contains(bytes.ToLower(body), []byte("secondary rate limit"))
}

func parseRateLimit(resp *http.Response, now func() time.Time) error {
	if now == nil {
		now = timeNow
	}
	err := &RateLimitError{StatusCode: resp.StatusCode}
	if v := strings.TrimSpace(resp.Header.Get("X-RateLimit-Remaining")); v != "" {
		if n, perr := strconv.ParseInt(v, 10, 64); perr == nil {
			err.Remaining = n
		}
	}
	if v := strings.TrimSpace(resp.Header.Get("X-RateLimit-Reset")); v != "" {
		if n, perr := strconv.ParseInt(v, 10, 64); perr == nil {
			err.Reset = time.Unix(n, 0).UTC()
		}
	}
	if v := strings.TrimSpace(resp.Header.Get("Retry-After")); v != "" {
		if secs, perr := strconv.ParseInt(v, 10, 64); perr == nil {
			// Seconds beyond what a Duration holds would overflow into a
			// negative value; treat them as malformed (0003-MADR A7).
			if secs > 0 && secs <= maxRetryAfterSeconds {
				err.RetryAfter = time.Duration(secs) * time.Second
			}
		} else if when, perr := http.ParseTime(v); perr == nil {
			d := when.Sub(now())
			if d > 0 {
				err.RetryAfter = d
			}
		}
	}
	return err
}
