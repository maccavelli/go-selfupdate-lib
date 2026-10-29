package selfupdatetest_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/maccavelli/go-selfupdate-lib/selfupdate"
	"github.com/maccavelli/go-selfupdate-lib/selfupdate/selfupdatetest"
)

// Tests for VerifyReleaseSource (docs/decisions/0025-MADR-align-with-go-sdk-design-practices.md
// D11).

var conformancePlatforms = []selfupdate.Platform{{OS: "linux", Arch: "amd64"}, {OS: "windows", Arch: "amd64"}}

func conformanceReleases() []selfupdatetest.ReleaseSpec {
	body := func(tag string) func(selfupdate.Platform) []byte {
		return func(p selfupdate.Platform) []byte { return []byte("demo " + tag + " " + p.OS) }
	}
	return []selfupdatetest.ReleaseSpec{
		selfupdatetest.NewRelease("demo", "v1.0.0", conformancePlatforms, body("v1.0.0")),
		selfupdatetest.NewRelease("demo", "v1.1.0", conformancePlatforms, body("v1.1.0")),
	}
}

var conformanceCase = selfupdatetest.SourceCase{LatestTag: "v1.1.0", MissingTag: "v9.9.9"}

func TestVerifyReleaseSourcePasses(t *testing.T) {
	ctx := context.Background()
	if err := selfupdatetest.VerifyReleaseSource(ctx, selfupdatetest.NewFakeSource("v1.1.0", conformanceReleases()...), conformanceCase); err != nil {
		t.Errorf("FakeSource: %v", err)
	}
	gh := selfupdatetest.NewGitHubServer(t, "owner", "demo", conformanceReleases()...)
	src, err := selfupdate.NewGitHubSource(selfupdate.GitHubOptions{
		Repository: selfupdate.Repository{Owner: "owner", Name: "demo"},
		Client:     gh.Client, APIBaseURL: gh.APIBase, UserAgent: "demo/v1.0.0", Limits: selfupdate.DefaultLimits(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := selfupdatetest.VerifyReleaseSource(ctx, src, conformanceCase); err != nil {
		t.Errorf("GitHubSource: %v", err)
	}
}

// broken wraps a source and breaks one rule.
type broken struct {
	selfupdate.ReleaseSource
	edit    func(*selfupdate.Release)
	missing error
	body    func([]byte) []byte
}

func (b broken) fix(r selfupdate.Release, err error) (selfupdate.Release, error) {
	if err == nil && b.edit != nil {
		r.Assets = append([]selfupdate.Asset(nil), r.Assets...)
		b.edit(&r)
	}
	return r, err
}

func (b broken) Latest(ctx context.Context) (selfupdate.Release, error) {
	return b.fix(b.ReleaseSource.Latest(ctx))
}

func (b broken) ByTag(ctx context.Context, tag string) (selfupdate.Release, error) {
	if tag == conformanceCase.MissingTag && b.missing != nil {
		return selfupdate.Release{}, b.missing
	}
	return b.fix(b.ReleaseSource.ByTag(ctx, tag))
}

func (b broken) OpenAsset(ctx context.Context, rel selfupdate.Release, a selfupdate.Asset) (io.ReadCloser, error) {
	rc, err := b.ReleaseSource.OpenAsset(ctx, rel, a)
	if err != nil || b.body == nil {
		return rc, err
	}
	data, err := io.ReadAll(rc)
	if cerr := rc.Close(); err == nil {
		err = cerr
	}
	return io.NopCloser(bytes.NewReader(b.body(data))), err
}

func TestVerifyReleaseSourceFails(t *testing.T) {
	for _, c := range []struct {
		name string
		src  broken
		sc   selfupdatetest.SourceCase
		want string
	}{
		{"unwrapped missing release", broken{missing: errors.New("no such tag")}, conformanceCase, "wraps selfupdate.ErrNoRelease"},
		{"mutable release", broken{edit: func(r *selfupdate.Release) { r.Immutable = false }}, conformanceCase, "is not Immutable"},
		{"asset ID 0", broken{edit: func(r *selfupdate.Release) { r.Assets[0].ID = 0 }}, conformanceCase, "requires a positive ID"},
		{"asset not uploaded", broken{edit: func(r *selfupdate.Release) { r.Assets[0].State = "new" }}, conformanceCase, `has State "new"`},
		{"short body", broken{body: func(b []byte) []byte { return b[:len(b)-1] }}, conformanceCase, "bytes, want Size"},
		{"wrong digest", broken{edit: func(r *selfupdate.Release) { r.Assets[0].Digest = "sha256:" + strings.Repeat("0", 64) }}, conformanceCase, "does not match its Digest"},
		{"wrong latest", broken{}, selfupdatetest.SourceCase{LatestTag: "v2.0.0", MissingTag: "v9.9.9"}, `Latest returned "v1.1.0", want "v2.0.0"`},
		// A plan's Apply compares the release ID
		// (docs/decisions/0038-MADR-pinned-plans-lifecycle-conformance-and-bubble-tests.md
		// D1-A).
		{"IDs that change per call", broken{edit: idsPerCall()}, conformanceCase, "needs the same IDs each time"},
	} {
		c.src.ReleaseSource = selfupdatetest.NewFakeSource("v1.1.0", conformanceReleases()...)
		err := selfupdatetest.VerifyReleaseSource(context.Background(), c.src, c.sc)
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: err = %v; want it to name %q", c.name, err, c.want)
		}
	}
	if err := selfupdatetest.VerifyReleaseSource(context.Background(), nil, conformanceCase); err == nil {
		t.Error("a nil source passed")
	}
	if err := selfupdatetest.VerifyReleaseSource(context.Background(), selfupdatetest.NewFakeSource("v1.1.0"), selfupdatetest.SourceCase{}); err == nil {
		t.Error("an empty SourceCase passed")
	}
}

// idsPerCall gives the release a new ID on each call, as a source that
// numbers its answers rather than its releases would.
func idsPerCall() func(*selfupdate.Release) {
	var n int64
	return func(r *selfupdate.Release) {
		n++
		r.ID += 1000 * n
	}
}
