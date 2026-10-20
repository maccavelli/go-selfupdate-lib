// Package selfupdatetest provides test doubles for programs built on
// selfupdate: release fixtures, an in-memory selfupdate.ReleaseSource, a recording
// Reporter, a scripted Confirmer, and a fake GitHub API whose asset
// downloads redirect to a second TLS origin (0004-MADR H3). Its
// VerifyReleaseSource and SourceCase check a ReleaseSource of another host
// against the contract the updater relies on (0025-MADR D11).
//
// It uses only the standard library and selfupdate, and is meant for
// tests; nothing in it is hardened for production use.
package selfupdatetest

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"sync"

	"github.com/maccavelli/go-selfupdate-lib/selfupdate"
)

// manifestName is the checksum manifest's asset name.
const manifestName = "SHA256SUMS"

// AssetSpec is one release asset.
type AssetSpec struct {
	// Name is the asset filename.
	Name string
	// Body is the asset's bytes. Size and digest are computed from it.
	Body []byte
	// State is the GitHub asset state. Empty means
	// selfupdate.AssetStateUploaded.
	State string
	// OmitDigest leaves Asset.Digest empty, as older GitHub releases do.
	OmitDigest bool
}

// ReleaseSpec is one release.
type ReleaseSpec struct {
	// Tag is the release tag.
	Tag string
	// Immutable, Draft and Prerelease are the GitHub release flags.
	Immutable, Draft, Prerelease bool
	// Assets are the release's assets, in order.
	Assets []AssetSpec
}

// NewRelease returns an immutable release with one binary per platform,
// named with selfupdate.ExactAssetName, and a SHA256SUMS manifest listing
// each binary's digest in platform order.
func NewRelease(product, tag string, platforms []selfupdate.Platform, body func(selfupdate.Platform) []byte) ReleaseSpec {
	spec := ReleaseSpec{Tag: tag, Immutable: true}
	var manifest bytes.Buffer
	for _, p := range platforms {
		name := selfupdate.ExactAssetName(product, p)
		b := bytes.Clone(body(p))
		sum := sha256.Sum256(b)
		fmt.Fprintf(&manifest, "%s  %s\n", hex.EncodeToString(sum[:]), name)
		spec.Assets = append(spec.Assets, AssetSpec{Name: name, Body: b})
	}
	spec.Assets = append(spec.Assets, AssetSpec{Name: manifestName, Body: manifest.Bytes()})
	return spec
}

// builtRelease is a ReleaseSpec with IDs assigned.
type builtRelease struct {
	release selfupdate.Release
	bodies  map[int64][]byte
}

// build assigns the release ID and asset IDs from next, which it advances.
func build(spec ReleaseSpec, next *int64, url string) builtRelease {
	*next++
	b := builtRelease{
		release: selfupdate.Release{
			ID: *next, Tag: spec.Tag, URL: url,
			Draft: spec.Draft, Prerelease: spec.Prerelease, Immutable: spec.Immutable,
		},
		bodies: make(map[int64][]byte, len(spec.Assets)),
	}
	for _, a := range spec.Assets {
		*next++
		state := a.State
		if state == "" {
			state = selfupdate.AssetStateUploaded
		}
		asset := selfupdate.Asset{ID: *next, Name: a.Name, State: state, Size: int64(len(a.Body))}
		if !a.OmitDigest {
			sum := sha256.Sum256(a.Body)
			asset.Digest = "sha256:" + hex.EncodeToString(sum[:])
		}
		b.release.Assets = append(b.release.Assets, asset)
		b.bodies[asset.ID] = bytes.Clone(a.Body)
	}
	return b
}

// FakeSource is an in-memory selfupdate.ReleaseSource. It is safe for
// concurrent use.
type FakeSource struct {
	mu       sync.Mutex
	latest   string
	releases map[string]builtRelease
	order    []string // tags, as declared
	calls    []string
}

// NewFakeSource returns a source whose Latest is the release tagged latest.
// An empty latest makes Latest fail. A missing release is an error that
// wraps selfupdate.ErrNoRelease, as GitHubSource's is, so a Checker's
// CheckCached records it as selfupdate.CheckNoRelease (0024-MADR D4-A).
func NewFakeSource(latest string, releases ...ReleaseSpec) *FakeSource {
	s := &FakeSource{latest: latest, releases: make(map[string]builtRelease, len(releases))}
	var next int64
	for _, spec := range releases {
		s.releases[spec.Tag] = build(spec, &next, "https://github.invalid/releases/tag/"+spec.Tag)
		s.order = append(s.order, spec.Tag)
	}
	return s
}

func (s *FakeSource) record(call string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls = append(s.calls, call)
}

func (s *FakeSource) lookup(tag string) (selfupdate.Release, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	b, ok := s.releases[tag]
	if !ok {
		return selfupdate.Release{}, fmt.Errorf("selfupdatetest: no release %q: %w", tag, selfupdate.ErrNoRelease)
	}
	rel := b.release
	rel.Assets = append([]selfupdate.Asset(nil), rel.Assets...)
	return rel, nil
}

// Latest implements selfupdate.ReleaseSource.
func (s *FakeSource) Latest(ctx context.Context) (selfupdate.Release, error) {
	s.record("Latest")
	if err := ctx.Err(); err != nil {
		return selfupdate.Release{}, err
	}
	if s.latest == "" {
		return selfupdate.Release{}, fmt.Errorf("selfupdatetest: no latest release: %w", selfupdate.ErrNoRelease)
	}
	return s.lookup(s.latest)
}

// ByTag implements selfupdate.ReleaseSource.
func (s *FakeSource) ByTag(ctx context.Context, tag string) (selfupdate.Release, error) {
	s.record("ByTag " + tag)
	if err := ctx.Err(); err != nil {
		return selfupdate.Release{}, err
	}
	return s.lookup(tag)
}

// OpenAsset implements selfupdate.ReleaseSource.
func (s *FakeSource) OpenAsset(ctx context.Context, rel selfupdate.Release, asset selfupdate.Asset) (io.ReadCloser, error) {
	s.record("OpenAsset " + asset.Name)
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	b, ok := s.releases[rel.Tag]
	if !ok {
		return nil, fmt.Errorf("selfupdatetest: no release %q", rel.Tag)
	}
	body, ok := b.bodies[asset.ID]
	if !ok {
		return nil, fmt.Errorf("selfupdatetest: no asset %d in release %q", asset.ID, rel.Tag)
	}
	return io.NopCloser(bytes.NewReader(body)), nil
}

// ListReleases implements selfupdate.ReleaseLister: every release, drafts
// and prereleases included, in the order NewFakeSource was given them, up to
// o.Limit when it is positive. It applies no other limit.
func (s *FakeSource) ListReleases(ctx context.Context, o selfupdate.ListOptions) ([]selfupdate.Release, error) {
	s.record("ListReleases")
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.mu.Lock()
	order := append([]string(nil), s.order...)
	s.mu.Unlock()
	var out []selfupdate.Release
	for _, tag := range order {
		if o.Limit > 0 && len(out) == o.Limit {
			break
		}
		rel, err := s.lookup(tag)
		if err != nil {
			return nil, err
		}
		out = append(out, rel)
	}
	return out, nil
}

// Calls returns the calls made so far, in order: "Latest", "ByTag <tag>",
// "ListReleases" and "OpenAsset <name>".
func (s *FakeSource) Calls() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.calls...)
}

// RecordingReporter records every event. Its zero value is ready to use,
// and it is safe for concurrent use.
type RecordingReporter struct {
	mu     sync.Mutex
	events []selfupdate.Event
}

// Report implements selfupdate.Reporter.
func (r *RecordingReporter) Report(_ context.Context, ev selfupdate.Event) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, ev)
	return nil
}

// Events returns the events reported so far, in order.
func (r *RecordingReporter) Events() []selfupdate.Event {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]selfupdate.Event(nil), r.events...)
}

// Kinds returns the kinds of the events reported so far, in order.
func (r *RecordingReporter) Kinds() []selfupdate.EventKind {
	r.mu.Lock()
	defer r.mu.Unlock()
	kinds := make([]selfupdate.EventKind, len(r.events))
	for i, ev := range r.events {
		kinds[i] = ev.Kind
	}
	return kinds
}

// ScriptedConfirmer answers prompts from Answers, in order. When Err is set
// every prompt fails with it; when the answers run out, a prompt fails.
// It is safe for concurrent use.
type ScriptedConfirmer struct {
	// Answers are returned one per prompt.
	Answers []bool
	// Err, when set, is returned for every prompt.
	Err error

	mu      sync.Mutex
	prompts []selfupdate.Prompt
}

// Confirm implements selfupdate.Confirmer.
func (c *ScriptedConfirmer) Confirm(_ context.Context, p selfupdate.Prompt) (bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.prompts = append(c.prompts, p)
	if c.Err != nil {
		return false, c.Err
	}
	i := len(c.prompts) - 1
	if i >= len(c.Answers) {
		return false, fmt.Errorf("selfupdatetest: no scripted answer for prompt %d", i+1)
	}
	return c.Answers[i], nil
}

// Prompts returns the prompts received so far, in order.
func (c *ScriptedConfirmer) Prompts() []selfupdate.Prompt {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]selfupdate.Prompt(nil), c.prompts...)
}
