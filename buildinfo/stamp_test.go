package buildinfo_test

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/maccavelli/go-selfupdate-lib/buildinfo"
	"github.com/maccavelli/go-selfupdate-lib/internal/testproc"
)

// stampMain prints Identity as JSON.
const stampMain = `package main

import (
	"encoding/json"
	"os"

	"github.com/maccavelli/go-selfupdate-lib/buildinfo"
)

func main() {
	_ = json.NewEncoder(os.Stdout).Encode(buildinfo.Identity())
}
`

// TestStampedBinary proves that VersionVar, KindVar and LDFlags stamp a real
// linked binary (0004-PLAN-v1-4-0-command-surface.md, A5).
func TestStampedBinary(t *testing.T) {
	if testing.Short() {
		t.Skip("builds three binaries")
	}
	root, err := filepath.Abs("..")
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	gomod := "module example.com/stamp\n\ngo 1.27.1\n\nrequire github.com/maccavelli/go-selfupdate-lib v0.0.0\n\nreplace github.com/maccavelli/go-selfupdate-lib => " + filepath.ToSlash(root) + "\n"
	sum, err := os.ReadFile(filepath.Join(root, "go.sum"))
	if err != nil {
		t.Fatal(err)
	}
	for name, body := range map[string]string{"go.mod": gomod, "main.go": stampMain, "go.sum": string(sum)} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	for _, tc := range []struct {
		name, ldflags string
		want          buildinfo.Kind
		version       string
		reason        bool
	}{
		{"release", buildinfo.LDFlags("v1.2.3"), buildinfo.KindRelease, "v1.2.3", false},
		{"unstamped", "", buildinfo.KindLocal, "", false},
		{"bad-tag", "-X " + buildinfo.KindVar + "=release -X " + buildinfo.VersionVar + "=v1.2", buildinfo.KindLocal, "v1.2", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			bin := filepath.Join(dir, tc.name)
			if runtime.GOOS == "windows" {
				bin += ".exe"
			}
			testproc.GoBuild(t, dir, bin, []string{"GOFLAGS=-mod=mod", "GOPROXY=off", "GOWORK=off"}, "-ldflags", tc.ldflags)
			out, err := exec.Command(bin).Output()
			if err != nil {
				t.Fatalf("run %s: %v", bin, err)
			}
			var got buildinfo.Info
			if err := json.Unmarshal(out, &got); err != nil {
				t.Fatalf("decode %q: %v", out, err)
			}
			if got.Kind != tc.want || got.Version != tc.version || (got.Reason != "") != tc.reason {
				t.Fatalf("got kind=%s version=%q reason=%q; want kind=%s version=%q reason set=%t",
					got.Kind, got.Version, got.Reason, tc.want, tc.version, tc.reason)
			}
		})
	}
}
