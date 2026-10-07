package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"

	"github.com/maccavelli/go-selfupdate-lib/selfupdate"
)

// Tests for docs/decisions/0013-PLAN-build-and-stage-release-workflow.md
// B2: the internal release tool. They need go and git, and fail without
// them.

const fixtureTag = "v1.2.3"

var host = selfupdate.Platform{OS: runtime.GOOS, Arch: runtime.GOARCH}

// testPlatforms are the fixture's platforms: the three CI runners' and the
// host's.
func testPlatforms() []selfupdate.Platform {
	out := []selfupdate.Platform{{OS: "linux", Arch: "amd64"}, {OS: "darwin", Arch: "arm64"}, {OS: "windows", Arch: "amd64"}}
	for _, p := range out {
		if p == host {
			return out
		}
	}
	return append(out, host)
}

// repoRoot is this repository's root, which the fixture's replace names.
func repoRoot() (string, error) {
	return filepath.Abs(filepath.Join("..", "..", ".."))
}

// gitIn runs git in dir with no system configuration, and the empty file
// global as its global one. Git for Windows cannot open NUL as a
// configuration file, so it is a real file.
func gitIn(dir, global string, args ...string) (string, error) {
	cmd := exec.Command("git", append([]string{"-c", "user.name=fixture", "-c", "user.email=fixture@example.com",
		"-c", "commit.gpgsign=false", "-c", "tag.gpgsign=false"}, args...)...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL="+global)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git %s: %w\n%s", strings.Join(args, " "), err, out)
	}
	return strings.TrimSpace(string(out)), nil
}

// makeFixtureRepo copies the fixture module into base/repo as a new git
// repository, with its replace pointing at this repository, commits it,
// and tags it (annotated) when tag is set. It returns the repository and
// the commit.
func makeFixtureRepo(base, tag string) (string, string, error) {
	root, err := repoRoot()
	if err != nil {
		return "", "", err
	}
	dir := filepath.Join(base, "repo")
	src := filepath.Join("testdata", "fixture")
	err = filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		if d.IsDir() {
			return os.MkdirAll(filepath.Join(dir, rel), 0o755)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if rel == "go.mod" {
			data = bytes.Replace(data, []byte("=> ../../../../.."), []byte("=> "+filepath.ToSlash(root)), 1)
		}
		return os.WriteFile(filepath.Join(dir, rel), data, 0o644)
	})
	if err != nil {
		return "", "", err
	}
	global := filepath.Join(base, "gitconfig")
	if err := os.WriteFile(global, nil, 0o644); err != nil {
		return "", "", err
	}
	steps := [][]string{{"init", "-q"}, {"add", "-A"}, {"commit", "-q", "-m", "fixture"}}
	if tag != "" {
		steps = append(steps, []string{"tag", "-a", "-m", tag, tag})
	}
	for _, args := range steps {
		if _, err := gitIn(dir, global, args...); err != nil {
			return "", "", err
		}
	}
	sha, err := gitIn(dir, global, "rev-parse", "HEAD")
	return dir, sha, err
}

// fixtureRepo is makeFixtureRepo in a test's temporary directory.
func fixtureRepo(t *testing.T, tag string) (string, string) {
	t.Helper()
	dir, sha, err := makeFixtureRepo(t.TempDir(), tag)
	if err != nil {
		t.Fatal(err)
	}
	return dir, sha
}

// specJSON is a spec for the relay fixture.
func specJSON(packaging string, platforms []map[string]string, extras []map[string]string) ([]byte, error) {
	spec := map[string]any{
		"schema":    1,
		"products":  []any{map[string]any{"name": "relay", "package": "./cmd/relay", "identity_args": []string{"version"}}},
		"platforms": platforms,
		"packaging": packaging,
	}
	if len(extras) > 0 {
		spec["extras"] = extras
	}
	return json.Marshal(spec)
}

// writeSpec writes a spec for the relay fixture outside any repository,
// and returns its path.
func writeSpec(t *testing.T, packaging string, platforms []map[string]string, extras ...map[string]string) string {
	t.Helper()
	data, err := specJSON(packaging, platforms, extras)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "selfupdate-release.json")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func platformObjects(ps []selfupdate.Platform, format func(selfupdate.Platform) string) []map[string]string {
	out := make([]map[string]string, len(ps))
	for i, p := range ps {
		out[i] = map[string]string{"os": p.OS, "arch": p.Arch}
		if format != nil {
			if f := format(p); f != "" {
				out[i]["format"] = f
			}
		}
	}
	return out
}

// built is the fixture built once, tagged fixtureTag, for testPlatforms.
type built struct {
	base, repo, sha, spec, bin string
}

var (
	buildOnce sync.Once
	shared    built
	sharedErr error
	// testTemps are directories the installer tests made outside any
	// test's own (installer_harness_test.go), removed by TestMain.
	testTemps   []string
	testTempsMu sync.Mutex
)

// keepUntilExit has TestMain remove dir.
func keepUntilExit(dir string) {
	testTempsMu.Lock()
	defer testTempsMu.Unlock()
	testTemps = append(testTemps, dir)
}

// sharedBuild builds the tagged fixture once per test binary. Tests read
// its output, and copy it before changing anything.
func sharedBuild(t *testing.T) built {
	t.Helper()
	buildOnce.Do(func() { shared, sharedErr = buildFixture() })
	if sharedErr != nil {
		t.Fatalf("building the fixture: %v", sharedErr)
	}
	return shared
}

func buildFixture() (built, error) {
	base, err := os.MkdirTemp("", "selfupdate-release-test-*")
	if err != nil {
		return built{}, err
	}
	b := built{base: base}
	if b.repo, b.sha, err = makeFixtureRepo(base, fixtureTag); err != nil {
		return b, err
	}
	data, err := specJSON("binary", platformObjects(testPlatforms(), nil), nil)
	if err != nil {
		return b, err
	}
	b.spec = filepath.Join(base, "spec.json")
	if err := os.WriteFile(b.spec, data, 0o644); err != nil {
		return b, err
	}
	b.bin = filepath.Join(base, "bin")
	var log bytes.Buffer
	err = build(context.Background(), buildInput{SpecPath: b.spec, ModuleDir: b.repo, StampVersion: fixtureTag, StampKind: "release", Out: b.bin}, &log)
	return b, err
}

// copyDir copies the regular files of src into a new directory.
func copyDir(t *testing.T, src string) string {
	t.Helper()
	dst := t.TempDir()
	entries, err := os.ReadDir(src)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		data, err := os.ReadFile(filepath.Join(src, e.Name()))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dst, e.Name()), data, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return dst
}

func TestMain(m *testing.M) {
	code := m.Run()
	if shared.base != "" {
		os.RemoveAll(shared.base)
	}
	for _, dir := range testTemps {
		os.RemoveAll(dir)
	}
	os.Exit(code)
}
