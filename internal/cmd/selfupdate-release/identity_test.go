package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/maccavelli/go-selfupdate-lib/selfupdate"
	"github.com/maccavelli/go-selfupdate-lib/selfupdate/archive"
)

func TestIdentityPattern(t *testing.T) {
	sha := "0123456789abcdef0123456789abcdef01234567"
	re := identityPattern("v1.2.3", "release", sha)
	for line, want := range map[string]bool{
		"v1.2.3 (release) 0123456789ab":         true,
		"v1.2.3 (release)":                      true,
		"v1.2.3 (release) 0123456789ab-dirty":   false,
		"v1.2.3 (local) 0123456789ab":           false,
		"v1.2.30 (release) 0123456789ab":        false,
		"v1.2.3 (release) 0123456789ab extra":   false,
		"relay v1.2.3 (release) 0123456789ab":   false,
		"v1.2.3 (release) ffffffffffff":         false,
		"v1x2x3 (release) 0123456789ab":         false,
		"v1.2.3 (release) 0123456789ab\nsecond": false,
	} {
		if re.MatchString(line) != want {
			t.Errorf("%q matched %v, want %v", line, !want, want)
		}
	}
}

func TestIdentity(t *testing.T) {
	b := sharedBuild(t)
	staging := t.TempDir()
	prog := hostProgram(t)
	raw := selfupdate.ExactAssetName("relay", host)
	if err := os.WriteFile(filepath.Join(staging, raw), prog, 0o644); err != nil {
		t.Fatal(err)
	}
	packedName, _ := archive.FleetName("relay", "", host, archive.TarGz)
	if err := os.WriteFile(filepath.Join(staging, packedName), packed(t, archive.TarGz, programName("relay", host), prog), 0o644); err != nil {
		t.Fatal(err)
	}
	in := func(asset, args, version string) identityInput {
		return identityInput{Staging: staging, Asset: asset, Product: "relay", Platform: host,
			ArgsJSON: args, WantVersion: version, WantKind: "release", SHA: b.sha, Timeout: 5 * time.Second}
	}
	for _, asset := range []string{raw, packedName} {
		first, err := identity(context.Background(), in(asset, `["version"]`, fixtureTag))
		if err != nil {
			t.Fatalf("%s: %v", asset, err)
		}
		if first != fixtureTag+" (release) "+b.sha[:12] {
			t.Fatalf("%s printed %q", asset, first)
		}
	}
	for _, tc := range []struct {
		name string
		in   identityInput
		want string
	}{
		{"another version", in(raw, `["version"]`, "v1.2.4"), `printed "v1.2.3 (release) `},
		{"no output", in(raw, `["silent"]`, fixtureTag), `printed ""`},
		{"the wrong output", in(raw, `["other"]`, fixtureTag), `printed "relay"`},
		{"a hang", func() identityInput { i := in(raw, `["hang"]`, fixtureTag); i.Timeout = time.Second; return i }(), "did not finish within 1s"},
		{"another platform", func() identityInput {
			i := in(raw, `["version"]`, fixtureTag)
			i.Platform = selfupdate.Platform{OS: "plan9", Arch: "386"}
			return i
		}(), "this runner is"},
		{"no arguments", in(raw, `[]`, fixtureTag), "-args-json"},
		{"a path as the asset", in("../"+raw, `["version"]`, fixtureTag), "must be a file name"},
		{"a local kind", func() identityInput { i := in(raw, `["version"]`, fixtureTag); i.WantKind = "local"; return i }(), `want "v1.2.3 (local)`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := identity(context.Background(), tc.in)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("identity: %v; want %q", err, tc.want)
			}
		})
	}
}
