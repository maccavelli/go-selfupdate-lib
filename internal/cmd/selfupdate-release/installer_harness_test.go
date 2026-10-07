package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/maccavelli/go-selfupdate-lib/selfupdate"
	"github.com/maccavelli/go-selfupdate-lib/selfupdate/releasespec"
)

// The harness of the installer behaviour tests
// (docs/decisions/0014-PLAN-shared-installer-templates.md I3 and I4): the
// fixture's releases, staged with their installers, and a loopback server
// that serves them as github.com does, at SELFUPDATE_INSTALL_BASE_URL
// (0014-MADR §5).

// requireShellsEnv lists, comma-separated, the shell binaries a job must
// run the tests under: "busybox" in the Alpine job, "dash" in the Debian
// one, "powershell" and "pwsh" on Windows. A Unix shell counts only where
// stubs ahead of PATH work in it.
const requireShellsEnv = "SELFUPDATE_INSTALL_REQUIRE_SHELLS"

// releasesEnv names a directory for the staged releases. A run that finds
// it complete uses it without building; otherwise it builds into it and
// keeps it. The container job builds on the runner, where go and git are,
// and runs the test binary in images that have neither.
const releasesEnv = "SELFUPDATE_INSTALL_TEST_RELEASES"

// releaseKinds are the fixture's releases, each staged at fixtureTag in a
// repository of its own: raw binaries, and archives whose Unix assets are
// tar.gz or gz.
var releaseKinds = map[string]string{"raw": "fixture/relay", "tgz": "fixture/relay-tgz", "gz": "fixture/relay-gz"}

func installPlatforms() []selfupdate.Platform {
	out := []selfupdate.Platform{{OS: "linux", Arch: "amd64"}, {OS: "linux", Arch: "arm64"}, {OS: "darwin", Arch: "arm64"}, {OS: "windows", Arch: "amd64"}}
	if !slices.Contains(out, host) {
		out = append(out, host)
	}
	return out
}

// installSpecJSON is one kind's spec: relay, which reports its identity,
// and relayctl, which does not; the rc channel; and a hook of each kind.
func installSpecJSON(kind string) ([]byte, error) {
	format := map[string]string{"tgz": "tar.gz", "gz": "gz"}[kind]
	packaging := "binary"
	if format != "" {
		packaging = "archive"
	}
	var platforms []map[string]string
	for _, p := range installPlatforms() {
		m := map[string]string{"os": p.OS, "arch": p.Arch}
		if format != "" {
			m["format"] = format
			if p.OS == goosWindows {
				m["format"] = "zip"
			}
		}
		platforms = append(platforms, m)
	}
	return json.Marshal(map[string]any{
		"schema": 1,
		"products": []any{
			map[string]any{"name": "relay", "package": "./cmd/relay", "identity_args": []string{"version"}},
			map[string]any{"name": "relayctl", "package": "./cmd/relay"},
		},
		"platforms":           platforms,
		"packaging":           packaging,
		"prerelease_channels": []string{"rc"},
		"installer": map[string]any{"name": "relay", "env_prefix": "RELAY", "hooks": []any{
			map[string]any{"when": "before_install", "product": "relay", "args": []string{"hook-before"}},
			map[string]any{"when": "after_install", "product": "relay", "args": []string{"hook-after", "--mark=1"}},
		}},
	})
}

// installReleases are the staged releases, at <dir>/<kind>, with their
// specs at <dir>/<kind>.json.
type installReleases struct{ dir string }

var (
	releasesOnce sync.Once
	releases     installReleases
	releasesErr  error
)

// sharedReleases builds and stages the releases once per test binary.
func sharedReleases(t *testing.T) installReleases {
	t.Helper()
	releasesOnce.Do(func() { releases, releasesErr = prepareReleases() })
	if releasesErr != nil {
		t.Fatalf("staging the installer releases: %v", releasesErr)
	}
	return releases
}

func prepareReleases() (installReleases, error) {
	dir := os.Getenv(releasesEnv)
	if dir == "" {
		d, err := os.MkdirTemp("", "selfupdate-install-test-*")
		if err != nil {
			return installReleases{}, err
		}
		keepUntilExit(d)
		dir = d
	} else if _, err := os.Stat(filepath.Join(dir, "complete")); err == nil {
		return installReleases{dir}, nil
	}
	base := filepath.Join(dir, "build")
	if err := os.MkdirAll(base, 0o755); err != nil {
		return installReleases{}, err
	}
	repo, sha, err := makeFixtureRepo(base, fixtureTag)
	if err != nil {
		return installReleases{}, err
	}
	for kind := range releaseKinds {
		data, err := installSpecJSON(kind)
		if err != nil {
			return installReleases{}, err
		}
		if err := os.WriteFile(filepath.Join(dir, kind+".json"), data, 0o644); err != nil {
			return installReleases{}, err
		}
	}
	ctx := context.Background()
	bin := filepath.Join(base, "bin")
	if err := build(ctx, buildInput{SpecPath: filepath.Join(dir, "raw.json"), ModuleDir: repo, StampVersion: fixtureTag, StampKind: "release", Out: bin}, io.Discard); err != nil {
		return installReleases{}, err
	}
	for kind, repository := range releaseKinds {
		in := stageInput{SpecPath: filepath.Join(dir, kind+".json"), ModuleDir: repo, Src: repo, Bin: bin, Out: filepath.Join(dir, kind),
			SHA: sha, Tag: fixtureTag, StampVersion: fixtureTag, Repository: repository}
		if _, err := stage(ctx, in, io.Discard); err != nil {
			return installReleases{}, err
		}
	}
	if err := os.RemoveAll(base); err != nil {
		return installReleases{}, err
	}
	return installReleases{dir}, os.WriteFile(filepath.Join(dir, "complete"), nil, 0o644)
}

func (r installReleases) staged(kind string) string { return filepath.Join(r.dir, kind) }

func (r installReleases) spec(t *testing.T, kind string) releasespec.Spec {
	t.Helper()
	s, err := loadSpec(filepath.Join(r.dir, kind+".json"))
	if err != nil {
		t.Fatal(err)
	}
	return s
}

// assetName is a product's asset for a platform in one kind of release.
func (r installReleases) assetName(t *testing.T, kind, product string, p selfupdate.Platform) string {
	t.Helper()
	name, err := r.spec(t, kind).AssetName(product, p)
	if err != nil {
		t.Fatal(err)
	}
	return name
}

func (r installReleases) file(t *testing.T, kind, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(r.staged(kind), name))
	if err != nil {
		t.Fatal(err)
	}
	return data
}

// program is a product's binary for this host, as built.
func (r installReleases) program(t *testing.T, product string) []byte {
	t.Helper()
	return r.file(t, "raw", r.assetName(t, "raw", product, host))
}

// render is one kind's installer for another tag.
func (r installReleases) render(t *testing.T, kind, name, tag string) []byte {
	t.Helper()
	files, err := renderInstallers(r.spec(t, kind), releaseKinds[kind], tag)
	if err != nil {
		t.Fatal(err)
	}
	return files[name]
}

// rawRepo is the raw release's repository.
const rawRepo = "fixture/relay"

// runResult is an installer run's exit code and combined output.
type runResult struct {
	code int
	out  string
}

// releaseServer serves /<owner>/<repo>/releases/download/<tag>/<name>
// from the staged directories, with a test's replacements.
type releaseServer struct {
	*httptest.Server
	mu       sync.Mutex
	routes   map[string]string        // "<owner>/<repo>@<tag>": a directory
	replaced map[string][]byte        // "<owner>/<repo>@<tag>/<name>": a body
	stalls   map[string]chan struct{} // name: closed when the stalled response starts
	requests []string
	script   []byte // served at scriptPath, and not listed in requests
}

// scriptPath is where the server offers the installer under test, for the
// forms that fetch it themselves.
const scriptPath = "/installer-under-test"

func newReleaseServer(t *testing.T, r installReleases) *releaseServer {
	t.Helper()
	s := &releaseServer{routes: map[string]string{}, replaced: map[string][]byte{}, stalls: map[string]chan struct{}{}}
	for kind, repository := range releaseKinds {
		s.routes[repository+"@"+fixtureTag] = r.staged(kind)
	}
	s.Server = httptest.NewServer(http.HandlerFunc(s.serve))
	t.Cleanup(s.Close)
	return s
}

func (s *releaseServer) serve(w http.ResponseWriter, req *http.Request) {
	parts := strings.Split(req.URL.Path, "/")
	s.mu.Lock()
	if req.URL.Path == scriptPath {
		script := s.script
		s.mu.Unlock()
		_, _ = w.Write(script)
		return
	}
	s.requests = append(s.requests, req.URL.Path)
	if len(parts) != 7 || parts[0] != "" || parts[3] != "releases" || parts[4] != "download" || parts[6] == "." || parts[6] == ".." {
		s.mu.Unlock()
		http.NotFound(w, req)
		return
	}
	key, name := parts[1]+"/"+parts[2]+"@"+parts[5], parts[6]
	body, ok := s.replaced[key+"/"+name]
	dir, routed := s.routes[key]
	stall := s.stalls[name]
	delete(s.stalls, name)
	s.mu.Unlock()
	if !ok {
		data, err := os.ReadFile(filepath.Join(dir, name))
		if !routed || err != nil {
			http.NotFound(w, req)
			return
		}
		body = data
	}
	if stall == nil {
		_, _ = w.Write(body)
		return
	}
	// Half the body, then nothing until the client goes.
	w.Header().Set("Content-Length", strconv.Itoa(len(body)))
	_, _ = w.Write(body[:len(body)/2])
	w.(http.Flusher).Flush()
	close(stall)
	select {
	case <-req.Context().Done():
	case <-time.After(2 * time.Minute):
	}
}

// got is the release paths requested so far.
func (s *releaseServer) got() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.requests)
}

func (s *releaseServer) route(repository, tag, dir string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.routes[repository+"@"+tag] = dir
}

func (s *releaseServer) replace(repository, tag, name string, body []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.replaced[repository+"@"+tag+"/"+name] = body
}

// stall makes the next request for name stop halfway, and returns a
// channel closed when it has.
func (s *releaseServer) stall(name string) <-chan struct{} {
	s.mu.Lock()
	defer s.mu.Unlock()
	ch := make(chan struct{})
	s.stalls[name] = ch
	return ch
}

// body is what the server would send for name.
func (s *releaseServer) body(t *testing.T, repository, tag, name string) []byte {
	t.Helper()
	s.mu.Lock()
	data, ok := s.replaced[repository+"@"+tag+"/"+name]
	dir := s.routes[repository+"@"+tag]
	s.mu.Unlock()
	if ok {
		return data
	}
	data, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		t.Fatal(err)
	}
	return data
}

// editSums replaces SHA256SUMS with edit's result. edit gets the lines
// without their newlines, and the index of name's line.
func (s *releaseServer) editSums(t *testing.T, repository, tag, name string, edit func(lines []string, i int) []string) {
	t.Helper()
	lines := strings.Split(strings.TrimSuffix(string(s.body(t, repository, tag, "SHA256SUMS")), "\n"), "\n")
	i := slices.IndexFunc(lines, func(l string) bool { return strings.HasSuffix(l, "  "+name) })
	if i < 0 {
		t.Fatalf("SHA256SUMS has no line for %s", name)
	}
	s.replace(repository, tag, "SHA256SUMS", []byte(strings.Join(edit(lines, i), "\n")+"\n"))
}

// replaceAsset serves body as name, and lists its hash in SHA256SUMS, so
// it passes the checksum.
func (s *releaseServer) replaceAsset(t *testing.T, repository, tag, name string, body []byte) {
	t.Helper()
	sum := sha256.Sum256(body)
	s.editSums(t, repository, tag, name, func(lines []string, i int) []string {
		lines[i] = hex.EncodeToString(sum[:]) + "  " + name
		return lines
	})
	s.replace(repository, tag, name, body)
}

func exitCode(t *testing.T, err error) int {
	t.Helper()
	var ee *exec.ExitError
	switch {
	case err == nil:
		return 0
	case errors.As(err, &ee):
		return ee.ExitCode()
	}
	t.Fatal(err)
	return 0
}

// filesIn is a directory's entries and their contents; a directory, such
// as a temporary one left behind, has a nil body.
func filesIn(t *testing.T, dir string) map[string][]byte {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return map[string][]byte{}
	}
	if err != nil {
		t.Fatal(err)
	}
	out := map[string][]byte{}
	for _, e := range entries {
		if e.IsDir() {
			out[e.Name()+"/"] = nil
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			t.Fatal(err)
		}
		out[e.Name()] = data
	}
	return out
}

// expectFiles checks that dir holds exactly want, byte for byte, and,
// outside Windows, that each file is mode 0755.
func expectFiles(t *testing.T, dir string, want map[string][]byte) {
	t.Helper()
	got := filesIn(t, dir)
	var gotNames, wantNames []string
	for n := range got {
		gotNames = append(gotNames, n)
	}
	for n := range want {
		wantNames = append(wantNames, n)
	}
	slices.Sort(gotNames)
	slices.Sort(wantNames)
	if !slices.Equal(gotNames, wantNames) {
		t.Fatalf("%s holds %v, want %v", dir, gotNames, wantNames)
	}
	for n, data := range want {
		if !bytes.Equal(got[n], data) {
			t.Errorf("%s: not the expected bytes (%d, want %d)", n, len(got[n]), len(data))
		}
		info, err := os.Stat(filepath.Join(dir, n))
		if err != nil {
			t.Fatal(err)
		}
		if runtime.GOOS != goosWindows && info.Mode().Perm() != 0o755 {
			t.Errorf("%s: mode %v, want 0755", n, info.Mode().Perm())
		}
	}
}
