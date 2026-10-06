package main

import (
	"context"
	"debug/buildinfo"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/maccavelli/go-selfupdate-lib/selfupdate"
)

func TestBuildRefusesAProgramWithoutBuildinfo(t *testing.T) {
	repo, _ := fixtureRepo(t, "")
	data, err := json.Marshal(map[string]any{
		"schema":    1,
		"products":  []any{map[string]any{"name": "nostamp", "package": "./cmd/nostamp"}},
		"platforms": platformObjects([]selfupdate.Platform{host}, nil),
	})
	if err != nil {
		t.Fatal(err)
	}
	spec := filepath.Join(t.TempDir(), "spec.json")
	if err := os.WriteFile(spec, data, 0o644); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(t.TempDir(), "bin")
	err = build(context.Background(), buildInput{SpecPath: spec, ModuleDir: repo, StampVersion: fixtureTag, StampKind: "release", Out: out}, io.Discard)
	if err == nil || !strings.Contains(err.Error(), "does not import github.com/maccavelli/go-selfupdate-lib/buildinfo; its release stamp would be lost") {
		t.Fatalf("build: %v", err)
	}
	if entries, _ := os.ReadDir(out); len(entries) != 0 {
		t.Fatalf("build wrote %d files before refusing", len(entries))
	}
}

func TestBuildRefuses(t *testing.T) {
	repo, _ := fixtureRepo(t, "")
	hostSpec := writeSpec(t, "binary", platformObjects([]selfupdate.Platform{host}, nil))
	occupied := t.TempDir()
	if err := os.WriteFile(filepath.Join(occupied, "old"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name string
		in   buildInput
		want string
	}{
		{"an unknown target", buildInput{SpecPath: writeSpec(t, "binary", []map[string]string{{"os": "plan10", "arch": "amd64"}}),
			StampVersion: fixtureTag, StampKind: "release"}, "plan10/amd64 is not a target of this toolchain"},
		{"an unknown kind", buildInput{SpecPath: hostSpec, StampVersion: fixtureTag, StampKind: "beta"}, `-stamp-kind "beta"`},
		{"a version with a space", buildInput{SpecPath: hostSpec, StampVersion: "v1.2.3 -X x=y", StampKind: "release"}, "-stamp-version"},
		{"an occupied output", buildInput{SpecPath: hostSpec, StampVersion: fixtureTag, StampKind: "release", Out: occupied}, "is not empty"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tc.in.ModuleDir = repo
			if tc.in.Out == "" {
				tc.in.Out = filepath.Join(t.TempDir(), "bin")
			}
			err := build(context.Background(), tc.in, io.Discard)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("build: %v; want %q", err, tc.want)
			}
		})
	}
}

// TestBuildIgnoresTheCallersGoEnvironment: GOFLAGS, GOAMD64 and
// CGO_ENABLED in the environment, and a go env file, never reach the build
// (0013-MADR §4).
func TestBuildIgnoresTheCallersGoEnvironment(t *testing.T) {
	repo, sha := fixtureRepo(t, "")
	// A user's go env file, at its default place under a new home. It sets
	// what the recipe leaves at its default, so only GOENV=off keeps it
	// out. The caches stay where they are, so nothing rebuilds from cold.
	for _, name := range []string{"GOCACHE", "GOMODCACHE"} {
		out, err := exec.Command("go", "env", name).Output()
		if err != nil {
			t.Fatal(err)
		}
		t.Setenv(name, strings.TrimSpace(string(out)))
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("APPDATA", filepath.Join(home, "AppData"))
	t.Setenv("GOENV", "")
	cfg, err := os.UserConfigDir()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(cfg, "go"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cfg, "go", "env"), []byte("GOAMD64=v3\nGOARM64=v9.0\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// The file takes effect for a go command that reads it.
	if out, err := exec.Command("go", "env", "GOAMD64").Output(); err != nil || strings.TrimSpace(string(out)) != "v3" {
		t.Fatalf("the planted go env file is not read: %q, %v", out, err)
	}
	t.Setenv("GOFLAGS", "-tags=evil")
	t.Setenv("CGO_ENABLED", "1")
	amd64 := selfupdate.Platform{OS: "linux", Arch: "amd64"}
	arm64 := selfupdate.Platform{OS: "linux", Arch: "arm64"}
	spec := writeSpec(t, "binary", platformObjects([]selfupdate.Platform{amd64, arm64}, nil))
	out := filepath.Join(t.TempDir(), "bin")
	if err := build(context.Background(), buildInput{SpecPath: spec, ModuleDir: repo, StampVersion: "rehearsal-" + sha[:12], StampKind: "local", Out: out}, io.Discard); err != nil {
		t.Fatal(err)
	}
	for p, level := range map[selfupdate.Platform][2]string{amd64: {"GOAMD64", "v1"}, arm64: {"GOARM64", "v8.0"}} {
		bi, err := buildinfo.ReadFile(filepath.Join(out, selfupdate.ExactAssetName("relay", p)))
		if err != nil {
			t.Fatal(err)
		}
		got := map[string]string{}
		for _, s := range bi.Settings {
			got[s.Key] = s.Value
		}
		if got["-tags"] != "" || got[level[0]] != level[1] || got["CGO_ENABLED"] != "0" {
			t.Errorf("%v: -tags=%q %s=%q CGO_ENABLED=%q", p, got["-tags"], level[0], got[level[0]], got["CGO_ENABLED"])
		}
	}
}

func TestBuildWithTags(t *testing.T) {
	repo, sha := fixtureRepo(t, "")
	data, err := json.Marshal(map[string]any{
		"schema":    1,
		"products":  []any{map[string]any{"name": "relay", "package": "./cmd/relay", "tags": []string{"netgo", "osusergo"}}},
		"platforms": platformObjects([]selfupdate.Platform{host}, nil),
	})
	if err != nil {
		t.Fatal(err)
	}
	spec := filepath.Join(t.TempDir(), "spec.json")
	if err := os.WriteFile(spec, data, 0o644); err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(t.TempDir(), "bin")
	if err := build(context.Background(), buildInput{SpecPath: spec, ModuleDir: repo, StampVersion: "rehearsal-" + sha[:12], StampKind: "local", Out: bin}, io.Discard); err != nil {
		t.Fatal(err)
	}
	_, err = stage(context.Background(), stageInput{SpecPath: spec, ModuleDir: repo, Src: repo, Bin: bin,
		Out: filepath.Join(t.TempDir(), "out"), SHA: sha, StampVersion: "rehearsal-" + sha[:12]}, io.Discard)
	if err != nil {
		t.Fatalf("stage, with the tags the spec names: %v", err)
	}
}
