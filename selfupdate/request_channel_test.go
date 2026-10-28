package selfupdate

import (
	"strings"
	"testing"
)

// Tests for docs/decisions/0005-PLAN-opt-in-prerelease-channels.md Step 3:
// Request.Channel validation.

func TestValidateRequestChannel(t *testing.T) {
	t.Parallel()
	channels, err := NewSemverPolicy(SemverOptions{AllowPrerelease: true, Channels: []string{"rc", "beta", "alpha"}})
	if err != nil {
		t.Fatal(err)
	}
	req := func(channel, target, current string) Request {
		return Request{Product: "demo", CurrentVersion: current, CurrentBuild: ReleaseBuild,
			TargetVersion: target, Channel: channel}
	}
	for _, c := range []struct {
		name   string
		policy VersionPolicy
		req    Request
		want   string // "" means valid
	}{
		{"stable on the channel policy", channels, req("", "", "v1.0.0"), ""},
		{"a known channel", channels, req("beta", "", "v1.0.0"), ""},
		{"an unknown channel", channels, req("nightly", "", "v1.0.0"), `channel "nightly" is not offered`},
		{"a channel on the strict policy", NewStrictVersionPolicy(), req("rc", "", "v1.0.0"), `channel "rc" is not offered`},
		{"a channel on a plain custom policy", prereleasePolicy{}, req("rc", "", "v1.0.0"), `channel "rc" is not offered`},
		{"a pinned prerelease with no channel", channels, req("", "v1.1.0-rc.1", "v1.0.0"), "is a prerelease; request a channel that admits it"},
		{"a pinned prerelease on a channel that admits it", channels, req("beta", "v1.1.0-rc.1", "v1.0.0"), ""},
		{"a pinned prerelease on a stricter channel", channels, req("rc", "v1.1.0-beta.1", "v1.0.0"), "is a prerelease; request a channel that admits it"},
		{"a pinned stable tag on a channel", channels, req("rc", "v1.1.0", "v1.0.0"), ""},
		{"running an rc, on the stable channel", channels, req("", "", "v1.1.0-rc.2"), ""},
		// D3: a plain policy decides for itself (0004-MADR G2).
		{"a plain custom policy pins a prerelease", prereleasePolicy{}, req("", "v1.1.0-rc.1", "v1.0.0"), ""},
	} {
		err := validateRequest(c.req, c.policy, runtimePlatform())
		switch {
		case c.want == "" && err != nil:
			t.Errorf("%s: %v", c.name, err)
		case c.want != "" && (err == nil || !strings.Contains(err.Error(), c.want)):
			t.Errorf("%s: err = %v, want one containing %q", c.name, err, c.want)
		}
	}
}

func TestCheckerPrepareKeepsChannel(t *testing.T) {
	t.Parallel()
	policy, err := NewSemverPolicy(SemverOptions{AllowPrerelease: true, Channels: []string{"rc"}})
	if err != nil {
		t.Fatal(err)
	}
	c, _, _ := checkerFor(t, checkRow{policy: policy})
	cr := checkRows()[0].req
	cr.Channel = "rc"
	req, err := c.prepare(cr)
	if err != nil || req.Channel != "rc" {
		t.Fatalf("prepare = %+v, %v; want the channel carried", req, err)
	}
}
