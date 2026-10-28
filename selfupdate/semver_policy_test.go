package selfupdate

import (
	"strings"
	"testing"
)

// Tests for docs/decisions/0005-PLAN-opt-in-prerelease-channels.md Step 2.

func channelPolicy(t *testing.T, channels ...string) ChannelPolicy {
	t.Helper()
	p, err := NewSemverPolicy(SemverOptions{AllowPrerelease: true, Channels: channels})
	if err != nil {
		t.Fatal(err)
	}
	cp, ok := p.(ChannelPolicy)
	if !ok {
		t.Fatalf("NewSemverPolicy returned %T, not a ChannelPolicy", p)
	}
	return cp
}

func TestNewSemverPolicyOptions(t *testing.T) {
	t.Parallel()
	if _, err := NewSemverPolicy(SemverOptions{}); err != nil {
		t.Fatalf("the zero options: %v", err)
	}
	if _, err := NewSemverPolicy(SemverOptions{AllowPrerelease: true, Channels: []string{"rc", "beta", "alpha"}}); err != nil {
		t.Fatalf("rc, beta, alpha: %v", err)
	}
	for _, c := range []struct {
		name string
		o    SemverOptions
		want string
	}{
		{"prerelease with no channel", SemverOptions{AllowPrerelease: true}, "needs at least one channel"},
		{"channels without prerelease", SemverOptions{Channels: []string{"rc"}}, "channels need AllowPrerelease"},
		{"stability not in ASCII order", SemverOptions{AllowPrerelease: true, Channels: []string{"beta", "nightly"}}, `channel "beta" must come after "nightly"`},
		{"a duplicate", SemverOptions{AllowPrerelease: true, Channels: []string{"rc", "rc"}}, `channel "rc" is listed twice`},
		{"an uppercase name", SemverOptions{AllowPrerelease: true, Channels: []string{"RC"}}, `channel name "RC" must match`},
		{"a hyphenated name", SemverOptions{AllowPrerelease: true, Channels: []string{"pre-view"}}, `channel name "pre-view" must match`},
		{"a numeric start", SemverOptions{AllowPrerelease: true, Channels: []string{"1rc"}}, `channel name "1rc" must match`},
		{"too long", SemverOptions{AllowPrerelease: true, Channels: []string{strings.Repeat("a", 17)}}, "must match"},
	} {
		if _, err := NewSemverPolicy(c.o); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: err = %v, want one containing %q", c.name, err, c.want)
		}
	}
}

func TestSemverPolicyDoesNotShareChannels(t *testing.T) {
	t.Parallel()
	channels := []string{"rc", "beta"}
	p, err := NewSemverPolicy(SemverOptions{AllowPrerelease: true, Channels: channels})
	if err != nil {
		t.Fatal(err)
	}
	channels[0] = "zz"
	if err := p.Validate("v1.0.0-rc.1"); err != nil {
		t.Fatalf("changing the caller's slice changed the policy: %v", err)
	}
	if err := p.Validate("v1.0.0-zz.1"); err == nil {
		t.Fatal("changing the caller's slice admitted a new name")
	}
}

func TestSemverPolicyValidate(t *testing.T) {
	t.Parallel()
	p := channelPolicy(t, "rc", "beta", "alpha")
	stable, err := NewSemverPolicy(SemverOptions{})
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		tag        string
		pre, plain bool // accepted by the channel policy, by the stable one
	}{
		{"v1.2.3", true, true},
		{"v0.0.0", true, true},
		{"v1.2.3-rc.1", true, false},
		{"v1.2.3-beta.10", true, false},
		{"v1.2.3-alpha.0", true, false},
		{"v1.2.3+meta", false, false},
		{"v1.2.3-rc.1+meta", false, false},
		{"v1.2", false, false},
		{"v01.2.3", false, false},
		{"1.2.3", false, false},
		{"v1.2.3-rc", false, false},
		{"v1.2.3-rc.01", false, false},
		{"v1.2.3-RC.1", false, false},
		{"v1.2.3-gamma.1", false, false},
		{"v1.2.3-rc.1.2", false, false},
		{"v1.2.3-rc.x", false, false},
		{"v1.2.3-pre-view.1", false, false},
		{"v1.2.3-rc.1\n", false, false},
		{"", false, false},
	} {
		if got := p.Validate(c.tag) == nil; got != c.pre {
			t.Errorf("channel policy Validate(%q) accepted=%v, want %v", c.tag, got, c.pre)
		}
		if got := stable.Validate(c.tag) == nil; got != c.plain {
			t.Errorf("stable policy Validate(%q) accepted=%v, want %v", c.tag, got, c.plain)
		}
	}
}

func TestSemverPolicyCompare(t *testing.T) {
	t.Parallel()
	p := channelPolicy(t, "rc", "beta", "alpha")
	for _, c := range []struct{ lo, hi string }{
		{"v1.3.0-rc.2", "v1.3.0-rc.10"},
		{"v1.3.0-beta.9", "v1.3.0-rc.1"},
		{"v1.3.0-alpha.9", "v1.3.0-beta.1"},
		{"v1.3.0-rc.9", "v1.3.0"},
		{"v1.2.9", "v1.3.0-alpha.0"},
	} {
		if got, err := p.Compare(c.lo, c.hi); err != nil || got != -1 {
			t.Errorf("Compare(%q, %q) = %d, %v; want -1", c.lo, c.hi, got, err)
		}
		if got, err := p.Compare(c.hi, c.lo); err != nil || got != 1 {
			t.Errorf("Compare(%q, %q) = %d, %v; want 1", c.hi, c.lo, got, err)
		}
	}
	if _, err := p.Compare("v1.0.0", "v1.0.0+meta"); err == nil {
		t.Error("Compare accepted build metadata")
	}
}

func TestSemverPolicyChannels(t *testing.T) {
	t.Parallel()
	p := channelPolicy(t, "rc", "beta", "alpha")
	for _, name := range []string{"", "rc", "beta", "alpha"} {
		if err := p.ValidChannel(name); err != nil {
			t.Errorf("ValidChannel(%q) = %v", name, err)
		}
	}
	if err := p.ValidChannel("nightly"); err == nil || !strings.Contains(err.Error(), "not offered") {
		t.Errorf("ValidChannel(nightly) = %v", err)
	}
	admits := map[string][]string{
		"":      {"v1.0.0"},
		"rc":    {"v1.0.0", "v1.0.0-rc.1"},
		"beta":  {"v1.0.0", "v1.0.0-rc.1", "v1.0.0-beta.1"},
		"alpha": {"v1.0.0", "v1.0.0-rc.1", "v1.0.0-beta.1", "v1.0.0-alpha.1"},
	}
	all := []string{"v1.0.0", "v1.0.0-rc.1", "v1.0.0-beta.1", "v1.0.0-alpha.1"}
	for channel, want := range admits {
		for _, tag := range all {
			wanted := false
			for _, w := range want {
				wanted = wanted || w == tag
			}
			if got := p.Admits(channel, tag); got != wanted {
				t.Errorf("Admits(%q, %q) = %v, want %v", channel, tag, got, wanted)
			}
		}
	}
	stable, err := NewSemverPolicy(SemverOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if err := stable.(ChannelPolicy).ValidChannel("rc"); err == nil {
		t.Error("a policy with no channels offered rc")
	}
}
