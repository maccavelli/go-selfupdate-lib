package selfupdate_test

import (
	"context"
	"errors"
	"path"
	"slices"
	"strings"
	"testing"

	"github.com/maccavelli/go-selfupdate-lib/selfupdate"
	"github.com/maccavelli/go-selfupdate-lib/selfupdate/selfupdatetest"
)

// Tests for docs/decisions/0005-PLAN-opt-in-prerelease-channels.md Step 5:
// channel discovery, through FakeSource and through GitHubServer.

var channelPlats = []selfupdate.Platform{goldenPlatform}

// channelRelease is an immutable release flagged as its tag says.
func channelRelease(tag string) selfupdatetest.ReleaseSpec {
	spec := selfupdatetest.NewRelease("demo", tag, channelPlats, releaseBody(tag))
	spec.Prerelease = strings.Contains(tag, "-")
	return spec
}

func withFlags(spec selfupdatetest.ReleaseSpec, f func(*selfupdatetest.ReleaseSpec)) selfupdatetest.ReleaseSpec {
	f(&spec)
	return spec
}

// channelBackend opens a source over specs, and reports the source calls
// made so far as FakeSource.Calls names them.
type channelBackend struct {
	name string
	open func(t *testing.T, specs ...selfupdatetest.ReleaseSpec) (selfupdate.ReleaseSource, func() []string)
}

var channelBackends = []channelBackend{
	{"FakeSource", func(_ *testing.T, specs ...selfupdatetest.ReleaseSpec) (selfupdate.ReleaseSource, func() []string) {
		// Latest is chosen as GitHubServer chooses it.
		latest := ""
		for _, s := range specs {
			if !s.Draft && !s.Prerelease {
				latest = s.Tag
			}
		}
		src := selfupdatetest.NewFakeSource(latest, specs...)
		return src, src.Calls
	}},
	{"GitHubServer", func(t *testing.T, specs ...selfupdatetest.ReleaseSpec) (selfupdate.ReleaseSource, func() []string) {
		gh := selfupdatetest.NewGitHubServer(t, "owner", "demo", specs...)
		src, err := selfupdate.NewGitHubSource(selfupdate.GitHubOptions{
			Repository: selfupdate.Repository{Owner: "owner", Name: "demo"},
			Client:     gh.Client, APIBaseURL: gh.APIBase, UserAgent: "demo/v1.0.0",
			Limits: selfupdate.DefaultLimits(),
		})
		if err != nil {
			t.Fatal(err)
		}
		calls := func() []string {
			var out []string
			for _, r := range gh.Requests() {
				switch {
				case strings.HasSuffix(r.Path, "/releases/latest"):
					out = append(out, "Latest")
				case strings.HasSuffix(r.Path, "/releases"):
					out = append(out, "ListReleases")
				case strings.Contains(r.Path, "/releases/tags/"):
					out = append(out, "ByTag "+path.Base(r.Path))
				}
			}
			return out
		}
		return src, calls
	}},
}

func channelCheck(t *testing.T, src selfupdate.ReleaseSource, current, channel, target string) (selfupdate.Availability, error) {
	t.Helper()
	policy, err := selfupdate.NewSemverPolicy(selfupdate.SemverOptions{AllowPrerelease: true, Channels: []string{"rc", "beta", "alpha"}})
	if err != nil {
		t.Fatal(err)
	}
	sel, err := selfupdate.NewExactAssetSelector(channelPlats)
	if err != nil {
		t.Fatal(err)
	}
	c, err := selfupdate.NewChecker(selfupdate.CheckerConfig{Source: src, Versions: policy, Assets: sel, Limits: selfupdate.DefaultLimits()})
	if err != nil {
		t.Fatal(err)
	}
	return c.Check(context.Background(), selfupdate.CheckRequest{
		Product: "demo", CurrentVersion: current, CurrentBuild: selfupdate.ReleaseBuild,
		Platform: goldenPlatform, Channel: channel, TargetVersion: target,
	})
}

func TestChannelDiscovery(t *testing.T) {
	t.Parallel()
	mutable := withFlags(channelRelease("v1.3.0-rc.1"), func(s *selfupdatetest.ReleaseSpec) { s.Immutable = false })
	malformed := func(tag string) selfupdatetest.ReleaseSpec {
		return withFlags(channelRelease(tag), func(s *selfupdatetest.ReleaseSpec) {
			s.Assets = append(s.Assets, selfupdatetest.AssetSpec{Name: "bad/name", Body: []byte("x")})
		})
	}
	unflaggedRC := withFlags(channelRelease("v1.4.0-rc.1"), func(s *selfupdatetest.ReleaseSpec) { s.Prerelease = false })
	flaggedStable := withFlags(channelRelease("v1.5.0"), func(s *selfupdatetest.ReleaseSpec) { s.Prerelease = true })
	draft := withFlags(channelRelease("v1.6.0-rc.1"), func(s *selfupdatetest.ReleaseSpec) { s.Draft = true })

	for _, c := range []struct {
		name                     string
		specs                    []selfupdatetest.ReleaseSpec
		current, channel, target string
		want                     string // the selected tag, when there is no error
		wantOp                   selfupdate.Operation
		wantErr                  error  // matched with errors.Is when set
		wantText                 string // matched as a substring when set
	}{
		{name: "out of order: the highest admissible wins", current: "v1.0.0", channel: "beta",
			specs: []selfupdatetest.ReleaseSpec{channelRelease("v1.1.0"), channelRelease("v1.3.0-beta.1"),
				channelRelease("v1.2.0"), channelRelease("v1.3.0-alpha.2"), channelRelease("v1.2.5-beta.1")},
			want: "v1.3.0-beta.1", wantOp: selfupdate.OperationUpgrade},
		{name: "rc admits stable but not beta", current: "v1.0.0", channel: "rc",
			specs: []selfupdatetest.ReleaseSpec{channelRelease("v1.1.0"), channelRelease("v1.3.0-beta.1"), channelRelease("v1.2.0")},
			want:  "v1.2.0", wantOp: selfupdate.OperationUpgrade},
		{name: "beta takes rc.1 over beta.3 of the same core", current: "v1.0.0", channel: "beta",
			specs: []selfupdatetest.ReleaseSpec{channelRelease("v1.3.0-rc.1"), channelRelease("v1.3.0-beta.3")},
			want:  "v1.3.0-rc.1", wantOp: selfupdate.OperationUpgrade},
		{name: "a newer stable beats both", current: "v1.0.0", channel: "beta",
			specs: []selfupdatetest.ReleaseSpec{channelRelease("v1.3.0-rc.1"), channelRelease("v1.3.0"), channelRelease("v1.3.0-beta.3")},
			want:  "v1.3.0", wantOp: selfupdate.OperationUpgrade},
		{name: "a draft is dropped", current: "v1.0.0", channel: "rc",
			specs: []selfupdatetest.ReleaseSpec{channelRelease("v1.2.0"), draft},
			want:  "v1.2.0", wantOp: selfupdate.OperationUpgrade},
		{name: "a mutable winner is an error, not a fallback", current: "v1.0.0", channel: "rc",
			specs:   []selfupdatetest.ReleaseSpec{channelRelease("v1.2.0"), mutable},
			wantErr: selfupdate.ErrMutableRelease},
		{name: "a malformed winner is an error", current: "v1.0.0", channel: "rc",
			specs:    []selfupdatetest.ReleaseSpec{channelRelease("v1.2.0"), malformed("v1.3.0-rc.1")},
			wantText: `asset name "bad/name" is not a basename`},
		{name: "a malformed older release is harmless", current: "v1.0.0", channel: "rc",
			specs: []selfupdatetest.ReleaseSpec{malformed("v1.1.0-rc.1"), channelRelease("v1.2.0")},
			want:  "v1.2.0", wantOp: selfupdate.OperationUpgrade},
		{name: "an unflagged prerelease tag is dropped", current: "v1.0.0", channel: "rc",
			specs: []selfupdatetest.ReleaseSpec{channelRelease("v1.2.0"), unflaggedRC},
			want:  "v1.2.0", wantOp: selfupdate.OperationUpgrade},
		{name: "a flagged stable tag is dropped", current: "v1.0.0", channel: "rc",
			specs: []selfupdatetest.ReleaseSpec{channelRelease("v1.2.0"), flaggedStable},
			want:  "v1.2.0", wantOp: selfupdate.OperationUpgrade},
		{name: "nothing on the channel", current: "v1.0.0", channel: "rc",
			specs:    []selfupdatetest.ReleaseSpec{channelRelease("v1.3.0-beta.1")},
			wantText: `no release on channel "rc"`},
		// MADR §4.
		{name: "rc to stable upgrades, on a channel", current: "v1.3.0-rc.2", channel: "rc",
			specs: []selfupdatetest.ReleaseSpec{channelRelease("v1.3.0-rc.2"), channelRelease("v1.3.0")},
			want:  "v1.3.0", wantOp: selfupdate.OperationUpgrade},
		{name: "rc to stable upgrades, on stable", current: "v1.3.0-rc.2",
			specs: []selfupdatetest.ReleaseSpec{channelRelease("v1.3.0-rc.2"), channelRelease("v1.3.0")},
			want:  "v1.3.0", wantOp: selfupdate.OperationUpgrade},
		{name: "leaving a channel never downgrades", current: "v1.3.0-rc.2",
			specs:   []selfupdatetest.ReleaseSpec{channelRelease("v1.2.0"), channelRelease("v1.3.0-rc.2")},
			wantErr: selfupdate.ErrLatestOlder},
		{name: "a channel never downgrades", current: "v1.4.0", channel: "rc",
			specs:   []selfupdatetest.ReleaseSpec{channelRelease("v1.2.0"), channelRelease("v1.4.0-rc.1")},
			wantErr: selfupdate.ErrLatestOlder},
		// Pinned versions.
		{name: "a pinned prerelease without a channel", current: "v1.0.0", target: "v1.3.0-rc.1",
			specs:    []selfupdatetest.ReleaseSpec{channelRelease("v1.3.0-rc.1")},
			wantText: "request a channel that admits it"},
		{name: "a pinned prerelease on a channel that admits it", current: "v1.0.0", channel: "beta", target: "v1.3.0-rc.1",
			specs: []selfupdatetest.ReleaseSpec{channelRelease("v1.3.0-rc.1"), channelRelease("v1.4.0-rc.1")},
			want:  "v1.3.0-rc.1", wantOp: selfupdate.OperationUpgrade},
		{name: "a pinned older prerelease is a rollback", current: "v1.4.0", channel: "rc", target: "v1.3.0-rc.1",
			specs: []selfupdatetest.ReleaseSpec{channelRelease("v1.3.0-rc.1")},
			want:  "v1.3.0-rc.1", wantOp: selfupdate.OperationRollback},
		{name: "a pinned release whose flag disagrees", current: "v1.0.0", channel: "rc", target: "v1.4.0-rc.1",
			specs:    []selfupdatetest.ReleaseSpec{unflaggedRC},
			wantText: "prerelease flag that disagrees with its tag"},
		// Deviation D4: the stable channel of a ChannelPolicy checks the flag.
		{name: "stable refuses an unflagged prerelease from Latest", current: "v1.0.0",
			specs:    []selfupdatetest.ReleaseSpec{channelRelease("v1.2.0"), unflaggedRC},
			wantText: "prerelease flag that disagrees with its tag"},
	} {
		for _, b := range channelBackends {
			t.Run(c.name+"/"+b.name, func(t *testing.T) {
				src, _ := b.open(t, c.specs...)
				got, err := channelCheck(t, src, c.current, c.channel, c.target)
				switch {
				case c.wantErr != nil:
					if !errors.Is(err, c.wantErr) {
						t.Fatalf("err = %v, want %v", err, c.wantErr)
					}
				case c.wantText != "":
					if err == nil || !strings.Contains(err.Error(), c.wantText) {
						t.Fatalf("err = %v, want one containing %q", err, c.wantText)
					}
				case err != nil:
					t.Fatal(err)
				case got.TargetVersion != c.want || got.Operation != c.wantOp:
					t.Fatalf("selected %s (%v), want %s (%v)", got.TargetVersion, got.Operation, c.want, c.wantOp)
				}
			})
		}
	}
}

func TestChannelSourceCalls(t *testing.T) {
	t.Parallel()
	specs := []selfupdatetest.ReleaseSpec{channelRelease("v1.2.0"), channelRelease("v1.3.0-rc.1")}
	for _, b := range channelBackends {
		t.Run(b.name, func(t *testing.T) {
			src, calls := b.open(t, specs...)
			got, err := channelCheck(t, src, "v1.0.0", "", "")
			if err != nil || got.TargetVersion != "v1.2.0" {
				t.Fatalf("stable = %s, %v; want v1.2.0", got.TargetVersion, err)
			}
			if c := calls(); !slices.Contains(c, "Latest") || slices.Contains(c, "ListReleases") {
				t.Fatalf("a stable request made %v; want Latest and no list", c)
			}

			src, calls = b.open(t, specs...)
			if _, err := channelCheck(t, src, "v1.0.0", "rc", ""); err != nil {
				t.Fatal(err)
			}
			if c := calls(); !slices.Contains(c, "ListReleases") || slices.Contains(c, "Latest") {
				t.Fatalf("a channel request made %v; want the list and no Latest", c)
			}
		})
	}
}

// latestOnly hides ListReleases.
type latestOnly struct{ selfupdate.ReleaseSource }

func TestChannelNeedsLister(t *testing.T) {
	t.Parallel()
	src := selfupdatetest.NewFakeSource("v1.2.0", channelRelease("v1.2.0"), channelRelease("v1.3.0-rc.1"))
	_, err := channelCheck(t, latestOnly{src}, "v1.0.0", "rc", "")
	if err == nil || !strings.Contains(err.Error(), `channel "rc" needs a source that lists releases`) {
		t.Fatalf("err = %v, want the missing capability named", err)
	}
	if slices.Contains(src.Calls(), "Latest") {
		t.Fatalf("calls = %v; a channel must not fall back to Latest", src.Calls())
	}
}
