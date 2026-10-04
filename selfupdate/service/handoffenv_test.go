package service

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/maccavelli/go-selfupdate-lib/selfupdate"
)

// Tests for docs/decisions/0011-MADR-reference-service-lifecycles.md
// amendment A3: the private environment file.

func TestHandOffEnvRoundTrip(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "env")
	value := `say "hi" $HOME \n` + "`x`\nsecond line"
	path, err := WriteHandOffEnv(dir, "abc", []string{"SU_TEST_SECRET=" + value, "SU_TEST_EMPTY=", "junk"})
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" {
		fi, err := os.Stat(path)
		if err != nil || fi.Mode().Perm() != 0o600 {
			t.Fatalf("file mode %v, %v", fi.Mode(), err)
		}
		di, err := os.Stat(dir)
		if err != nil || di.Mode().Perm() != 0o700 {
			t.Fatalf("directory mode %v, %v", di.Mode(), err)
		}
	}
	t.Setenv(EnvHandOffEnv, path)
	t.Setenv("SU_TEST_SECRET", "")
	if err := LoadHandOffEnv(); err != nil {
		t.Fatal(err)
	}
	if got := os.Getenv("SU_TEST_SECRET"); got != value {
		t.Fatalf("SU_TEST_SECRET = %q", got)
	}
	if _, set := os.LookupEnv(EnvHandOffEnv); set {
		t.Fatal("the variable was left set")
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("the file was left: %v", err)
	}
	if err := LoadHandOffEnv(); err != nil {
		t.Fatalf("a second load, with nothing to load: %v", err)
	}
}

func TestLoadHandOffEnvRefuses(t *testing.T) {
	dir := t.TempDir()
	open := filepath.Join(dir, "open.env")
	if err := os.WriteFile(open, []byte(`["X=1"]`), 0o644); err != nil { //nolint:gosec // the world-readable file the test refuses
		t.Fatal(err)
	}
	cases := map[string]string{"relative": "rel.env", "a directory": dir}
	if runtime.GOOS != "windows" {
		cases["world-readable"] = open
	}
	for name, path := range cases {
		t.Run(name, func(t *testing.T) {
			t.Setenv(EnvHandOffEnv, path)
			if err := LoadHandOffEnv(); err == nil {
				t.Fatalf("%s was loaded", path)
			}
		})
	}
	if _, err := WriteHandOffEnv("rel", "abc", nil); err == nil {
		t.Fatal("a relative directory was accepted")
	}
	if _, err := WriteHandOffEnv(dir, "a/b", nil); err == nil {
		t.Fatal("an ID with a slash was accepted")
	}
}

// TestReportFuncReportsLoadFailure: a detached run whose environment did
// not load says so in its result.
func TestReportFuncReportsLoadFailure(t *testing.T) {
	result := filepath.Join(t.TempDir(), "result")
	t.Setenv(EnvHandOff, "abc")
	t.Setenv(EnvHandOffResult, result)
	t.Setenv(EnvHandOffEnv, filepath.Join(t.TempDir(), "missing.env"))
	report := ReportFunc()
	if err := report(selfupdate.Result{Product: "demo"}, nil); err == nil {
		t.Fatal("the load failure was not returned")
	}
	got, err := ReadHandOffResult(result)
	if err != nil || got.ExitCode != 1 || !strings.Contains(got.Error, "missing.env") {
		t.Fatalf("result %+v, %v", got, err)
	}
}
