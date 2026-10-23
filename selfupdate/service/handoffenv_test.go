package service

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"slices"
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

// setArgs sets os.Args for the test, and puts the old value back after.
func setArgs(t *testing.T, args ...string) {
	t.Helper()
	old := os.Args
	t.Cleanup(func() { os.Args = old })
	os.Args = args
}

// unsetEnv unsets name for the test, and puts the old value back after.
func unsetEnv(t *testing.T, name string) {
	t.Helper()
	t.Setenv(name, "")
	if err := os.Unsetenv(name); err != nil {
		t.Fatal(err)
	}
}

// TestLoadHandOffEnvArg: a Task Scheduler handoff's first argument names
// the file, which is applied before the hop check and removed from
// os.Args; the argument anywhere else is the program's own
// (docs/decisions/0032-MADR-task-scheduler-registration-and-one-shot-handoff.md
// D6-A).
func TestLoadHandOffEnvArg(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "env")
	for _, name := range []string{"SU_TEST_ARG", EnvHandOffEnv, EnvHandOffHopParent} {
		unsetEnv(t, name)
	}

	t.Run("applied and removed", func(t *testing.T) {
		path, err := WriteHandOffEnv(dir, "arg", []string{"SU_TEST_ARG=from-file"})
		if err != nil {
			t.Fatal(err)
		}
		setArgs(t, "prog", ArgHandOffEnv+path, "update", "a b")
		if err := LoadHandOffEnv(); err != nil {
			t.Fatal(err)
		}
		if want := []string{"prog", "update", "a b"}; !slices.Equal(os.Args, want) {
			t.Fatalf("os.Args = %q, want %q", os.Args, want)
		}
		if got := os.Getenv("SU_TEST_ARG"); got != "from-file" {
			t.Fatalf("SU_TEST_ARG = %q", got)
		}
		if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("the file was left: %v", err)
		}
	})

	t.Run("anywhere else, left alone", func(t *testing.T) {
		unsetEnv(t, "SU_TEST_ARG")
		path, err := WriteHandOffEnv(dir, "later", []string{"SU_TEST_ARG=from-file"})
		if err != nil {
			t.Fatal(err)
		}
		setArgs(t, "prog", "update", ArgHandOffEnv+path)
		if err := LoadHandOffEnv(); err != nil {
			t.Fatal(err)
		}
		if want := []string{"prog", "update", ArgHandOffEnv + path}; !slices.Equal(os.Args, want) {
			t.Fatalf("os.Args = %q, want %q", os.Args, want)
		}
		if got, set := os.LookupEnv("SU_TEST_ARG"); set {
			t.Fatalf("SU_TEST_ARG = %q, want unset", got)
		}
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("the file was consumed: %v", err)
		}
	})

	t.Run("before the hop check", func(t *testing.T) {
		// The hop parent's process does not exist, so the check does not
		// wait; it unsets the variable only if the file was applied first.
		path, err := WriteHandOffEnv(dir, "hop", []string{EnvHandOffHopParent + "=1073741823"})
		if err != nil {
			t.Fatal(err)
		}
		setArgs(t, "prog", ArgHandOffEnv+path)
		if err := LoadHandOffEnv(); err != nil {
			t.Fatal(err)
		}
		if v, set := os.LookupEnv(EnvHandOffHopParent); set {
			t.Fatalf("%s = %q after the load: the hop check ran before the file", EnvHandOffHopParent, v)
		}
	})

	t.Run("a relative path refused", func(t *testing.T) {
		setArgs(t, "prog", ArgHandOffEnv+"rel.env", "update")
		if err := LoadHandOffEnv(); err == nil {
			t.Fatal("a relative path was loaded")
		}
	})

	t.Run("no argument", func(t *testing.T) {
		setArgs(t, "prog", "update")
		if err := LoadHandOffEnv(); err != nil || !slices.Equal(os.Args, []string{"prog", "update"}) {
			t.Fatalf("os.Args = %q, %v", os.Args, err)
		}
	})
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
