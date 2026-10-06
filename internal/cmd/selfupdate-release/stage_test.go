package main

import (
	"bytes"
	"context"
	"debug/buildinfo"
	"io"
	"os"
	"path/filepath"
	"runtime/debug"
	"sort"
	"strings"
	"testing"

	"github.com/maccavelli/go-selfupdate-lib/selfupdate"
	"github.com/maccavelli/go-selfupdate-lib/selfupdate/archive"
)

func stageWith(t *testing.T, b built, spec, bin string, mutate func(*stageInput)) (map[string]string, string, error) {
	t.Helper()
	out := filepath.Join(t.TempDir(), "staging")
	in := stageInput{SpecPath: spec, ModuleDir: b.repo, Src: b.repo, Bin: bin, Out: out,
		SHA: b.sha, Tag: fixtureTag, StampVersion: fixtureTag}
	if mutate != nil {
		mutate(&in)
	}
	sums, err := stage(context.Background(), in, io.Discard)
	return sums, out, err
}

func listDir(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	sort.Strings(names)
	return names
}

func TestBuildStampsAndNamesEveryTarget(t *testing.T) {
	b := sharedBuild(t)
	var want []string
	for _, p := range testPlatforms() {
		want = append(want, selfupdate.ExactAssetName("relay", p))
	}
	sort.Strings(want)
	if got := listDir(t, b.bin); strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("build wrote %v, want %v", got, want)
	}
	for _, name := range want {
		bi, err := buildinfo.ReadFile(filepath.Join(b.bin, name))
		if err != nil {
			t.Fatal(err)
		}
		if bi.Main.Version != fixtureTag {
			t.Errorf("%s: main module version %q, want the tag", name, bi.Main.Version)
		}
	}
}

func TestStageRaw(t *testing.T) {
	b := sharedBuild(t)
	notes := filepath.Join(b.repo, "notes.txt")
	if _, err := os.Stat(notes); err != nil {
		t.Fatal(err)
	}
	spec := writeSpec(t, "binary", platformObjects(testPlatforms(), nil),
		map[string]string{"name": "relay-notes-{tag}.txt", "path": "notes.txt"})
	sums, out, err := stageWith(t, b, spec, b.bin, nil)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"SHA256SUMS", "relay-notes-v1.2.3.txt"}
	for _, p := range testPlatforms() {
		name := selfupdate.ExactAssetName("relay", p)
		want = append(want, name)
		raw, _ := os.ReadFile(filepath.Join(b.bin, name))
		staged, _ := os.ReadFile(filepath.Join(out, name))
		if !bytes.Equal(raw, staged) {
			t.Errorf("%s is not the built binary", name)
		}
	}
	sort.Strings(want)
	if got := listDir(t, out); strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("staged %v, want %v", got, want)
	}
	data, err := os.ReadFile(filepath.Join(out, "SHA256SUMS"))
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := selfupdate.ParseSHA256SUMS(data)
	if err != nil || len(parsed) != len(testPlatforms()) {
		t.Fatalf("SHA256SUMS: %v, %d entries", err, len(parsed))
	}
	var names []string
	for _, line := range strings.Split(strings.TrimSuffix(string(data), "\n"), "\n") {
		names = append(names, line[66:])
	}
	if !sort.StringsAreSorted(names) {
		t.Fatalf("SHA256SUMS is not sorted by name: %v", names)
	}
	for n, s := range sums {
		if parsed[n] != s {
			t.Errorf("SHA256SUMS %s = %s, want %s", n, parsed[n], s)
		}
	}
}

// archiveFormats packs linux tar.gz (the default), darwin gz and windows
// zip (the default).
func archiveFormats(p selfupdate.Platform) string {
	if p.OS == "darwin" {
		return "gz"
	}
	return ""
}

func TestStageArchivesRoundTripAndRepeat(t *testing.T) {
	b := sharedBuild(t)
	spec := writeSpec(t, "archive", platformObjects(testPlatforms(), archiveFormats))
	sums1, out1, err := stageWith(t, b, spec, b.bin, nil)
	if err != nil {
		t.Fatal(err)
	}
	sums2, _, err := stageWith(t, b, spec, b.bin, nil)
	if err != nil {
		t.Fatal(err)
	}
	for n, s := range sums1 {
		if sums2[n] != s {
			t.Errorf("%s packed to different bytes on a second run", n)
		}
	}
	for _, p := range testPlatforms() {
		f := archive.TarGz
		switch p.OS {
		case "windows":
			f = archive.Zip
		case "darwin":
			f = archive.Gz
		}
		name, _ := archive.FleetName("relay", "", p, f)
		if _, ok := sums1[name]; !ok {
			t.Fatalf("no %s staged: %v", name, sums1)
		}
		raw, _ := os.ReadFile(filepath.Join(b.bin, selfupdate.ExactAssetName("relay", p)))
		got, err := unpackProgram(context.Background(), filepath.Join(out1, name), "relay", p)
		if err != nil || !bytes.Equal(got, raw) {
			t.Fatalf("%s does not unpack to the binary: %v", name, err)
		}
	}
	checked, err := check(context.Background(), out1, `["relay"]`, mustPlatformsJSON(t, spec))
	if err != nil || len(checked) != len(testPlatforms()) {
		t.Fatalf("check: %v, %v", checked, err)
	}
}

func mustPlatformsJSON(t *testing.T, specPath string) string {
	t.Helper()
	s, err := loadSpec(specPath)
	if err != nil {
		t.Fatal(err)
	}
	return s.PlatformsJSON()
}

func TestStageRefuses(t *testing.T) {
	b := sharedBuild(t)
	spec := writeSpec(t, "binary", platformObjects(testPlatforms(), nil))
	linux := selfupdate.ExactAssetName("relay", selfupdate.Platform{OS: "linux", Arch: "amd64"})
	windows := selfupdate.ExactAssetName("relay", selfupdate.Platform{OS: "windows", Arch: "amd64"})
	cases := []struct {
		name   string
		bin    func(t *testing.T, dir string)
		mutate func(*stageInput)
		want   string
	}{
		{"a wrong revision", nil, func(in *stageInput) { in.SHA = strings.Repeat("a", 40) }, "vcs.revision is"},
		{"a tag the build was not made at", nil, func(in *stageInput) { in.Tag = "v9.9.9" }, `the main module version is "v1.2.3", want "v9.9.9"`},
		{"a swapped platform", func(t *testing.T, dir string) {
			data, err := os.ReadFile(filepath.Join(dir, windows))
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(dir, linux), data, 0o755); err != nil {
				t.Fatal(err)
			}
		}, nil, `GOOS is "windows", want "linux"`},
		{"an empty binary", func(t *testing.T, dir string) {
			if err := os.WriteFile(filepath.Join(dir, linux), nil, 0o755); err != nil {
				t.Fatal(err)
			}
		}, nil, "is empty"},
		{"a file that is not a Go binary", func(t *testing.T, dir string) {
			if err := os.WriteFile(filepath.Join(dir, linux), []byte("#!/bin/sh\necho relay\n"), 0o755); err != nil {
				t.Fatal(err)
			}
		}, nil, "no Go build information"},
		{"a missing binary", func(t *testing.T, dir string) {
			if err := os.Remove(filepath.Join(dir, linux)); err != nil {
				t.Fatal(err)
			}
		}, nil, linux},
		{"a bad SHA", nil, func(in *stageInput) { in.SHA = "HEAD" }, "is not a full commit SHA"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			bin := b.bin
			if tc.bin != nil {
				bin = copyDir(t, b.bin)
				tc.bin(t, bin)
			}
			_, _, err := stageWith(t, b, spec, bin, tc.mutate)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("stage: %v; want an error containing %q", err, tc.want)
			}
		})
	}
}

func TestStageRefusesADirtyTree(t *testing.T) {
	repo, sha := fixtureRepo(t, "")
	if err := os.WriteFile(filepath.Join(repo, "untracked.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	spec := writeSpec(t, "binary", platformObjects([]selfupdate.Platform{host}, nil))
	bin := filepath.Join(t.TempDir(), "bin")
	if err := build(context.Background(), buildInput{SpecPath: spec, ModuleDir: repo, StampVersion: "rehearsal-" + sha[:12], StampKind: "local", Out: bin}, io.Discard); err != nil {
		t.Fatal(err)
	}
	_, err := stage(context.Background(), stageInput{SpecPath: spec, ModuleDir: repo, Src: repo, Bin: bin,
		Out: filepath.Join(t.TempDir(), "out"), SHA: sha, StampVersion: "rehearsal-" + sha[:12]}, io.Discard)
	if err == nil || !strings.Contains(err.Error(), `vcs.modified is "true", want "false"`) {
		t.Fatalf("stage: %v", err)
	}
}

func TestStageExtras(t *testing.T) {
	b := sharedBuild(t)
	host1 := []selfupdate.Platform{host}
	hostBin := t.TempDir()
	name := selfupdate.ExactAssetName("relay", host)
	data, err := os.ReadFile(filepath.Join(b.bin, name))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(hostBin, name), data, 0o755); err != nil {
		t.Fatal(err)
	}
	fromDir := map[string]string{"name": "relay-{tag}.apk"}
	spec := writeSpec(t, "binary", platformObjects(host1, nil), fromDir)

	extras := t.TempDir()
	if err := os.WriteFile(filepath.Join(extras, "relay-v1.2.3.apk"), []byte("apk"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, out, err := stageWith(t, b, spec, hostBin, func(in *stageInput) { in.ExtrasDir = extras }); err != nil {
		t.Fatalf("extras from the directory: %v", err)
	} else if got, _ := os.ReadFile(filepath.Join(out, "relay-v1.2.3.apk")); string(got) != "apk" {
		t.Fatalf("the extra is %q", got)
	}

	stray := t.TempDir()
	for _, n := range []string{"relay-v1.2.3.apk", "stray.txt"} {
		if err := os.WriteFile(filepath.Join(stray, n), []byte(n), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	for _, tc := range []struct {
		name, extras, want string
	}{
		{"no extras directory", "", "no extras directory was given"},
		{"a missing extra", t.TempDir(), "extra relay-v1.2.3.apk"},
		{"a stray file", stray, "holds stray.txt, which the spec does not list"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, _, err := stageWith(t, b, spec, hostBin, func(in *stageInput) { in.ExtrasDir = tc.extras })
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("stage: %v; want %q", err, tc.want)
			}
		})
	}

	t.Run("a symlinked extra", func(t *testing.T) {
		link := t.TempDir()
		if err := os.Symlink(filepath.Join(extras, "relay-v1.2.3.apk"), filepath.Join(link, "relay-v1.2.3.apk")); err != nil {
			t.Skipf("this host cannot make a symlink: %v", err)
		}
		_, _, err := stageWith(t, b, spec, hostBin, func(in *stageInput) { in.ExtrasDir = link })
		if err == nil || !strings.Contains(err.Error(), "is not a regular file") {
			t.Fatalf("stage: %v", err)
		}
	})
}

func TestCheckBuildInfo(t *testing.T) {
	good := func() *buildinfo.BuildInfo {
		return &buildinfo.BuildInfo{
			GoVersion: "go1.27.1", Path: "example.com/relay/cmd/relay",
			Main: debug.Module{Path: "example.com/relay", Version: "v1.2.3"},
			Settings: []debug.BuildSetting{
				{Key: "-trimpath", Value: "true"}, {Key: "CGO_ENABLED", Value: "0"},
				{Key: "GOOS", Value: "linux"}, {Key: "GOARCH", Value: "amd64"},
				{Key: "vcs.revision", Value: strings.Repeat("b", 40)}, {Key: "vcs.modified", Value: "false"},
			},
		}
	}
	want := binaryWant{Platform: selfupdate.Platform{OS: "linux", Arch: "amd64"}, SHA: strings.Repeat("b", 40),
		GoVersion: "go1.27.1", Module: "example.com/relay", Package: "example.com/relay/cmd/relay", Tag: "v1.2.3"}
	if err := checkBuildInfo(good(), want); err != nil {
		t.Fatalf("a good binary: %v", err)
	}
	set := func(key, value string) func(*buildinfo.BuildInfo) {
		return func(bi *buildinfo.BuildInfo) {
			for i := range bi.Settings {
				if bi.Settings[i].Key == key {
					bi.Settings[i].Value = value
					return
				}
			}
			bi.Settings = append(bi.Settings, debug.BuildSetting{Key: key, Value: value})
		}
	}
	for _, tc := range []struct {
		name string
		edit func(*buildinfo.BuildInfo)
		want string
	}{
		{"cgo", set("CGO_ENABLED", "1"), "CGO_ENABLED"},
		{"no -trimpath", set("-trimpath", "false"), "-trimpath"},
		{"build tags", set("-tags", "evil"), "-tags"},
		{"modified", set("vcs.modified", "true"), "vcs.modified"},
		{"another arch", set("GOARCH", "arm64"), "GOARCH"},
		{"another toolchain", func(bi *buildinfo.BuildInfo) { bi.GoVersion = "go1.26.6" }, "the Go version"},
		{"another module", func(bi *buildinfo.BuildInfo) { bi.Main.Path = "example.com/other" }, "the main module"},
		{"another package", func(bi *buildinfo.BuildInfo) { bi.Path = "example.com/relay/cmd/other" }, "the main package"},
		{"a dirty version", func(bi *buildinfo.BuildInfo) { bi.Main.Version = "v1.2.3+dirty" }, "the main module version"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			bi := good()
			tc.edit(bi)
			if err := checkBuildInfo(bi, want); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("checkBuildInfo: %v; want %q", err, tc.want)
			}
		})
	}
}
