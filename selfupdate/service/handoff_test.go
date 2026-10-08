package service

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/maccavelli/go-selfupdate-lib/selfupdate"
)

// Tests for docs/decisions/0011-PLAN-reference-service-lifecycles.md V1
// step 5: the handoff core.

type fakeDetacher struct {
	inside    bool
	insideErr error
	got       *HandOff
}

func (f *fakeDetacher) Inside(context.Context) (bool, error) { return f.inside, f.insideErr }

func (f *fakeDetacher) Detach(_ context.Context, spec HandOff) (Detached, error) {
	f.got = &spec
	return Detached{ID: spec.ID, Where: "fake", ResultPath: spec.ResultPath}, nil
}

func TestHandOffIfInside(t *testing.T) {
	t.Setenv(EnvHandOff, "")
	exe := selfExe(t)

	outside := &fakeDetacher{}
	if _, handed, err := HandOffIfInside(context.Background(), outside, HandOff{}); err != nil || handed || outside.got != nil {
		t.Fatalf("outside: handed %t, err %v", handed, err)
	}

	inside := &fakeDetacher{inside: true}
	det, handed, err := HandOffIfInside(context.Background(), inside, HandOff{Args: []string{"update", "-y"}, Env: []string{"KEEP=1"}})
	if err != nil || !handed {
		t.Fatalf("inside: handed %t, err %v", handed, err)
	}
	spec := *inside.got
	want := DefaultResultPath(exe)
	if spec.Executable != exe || spec.ResultPath != want || det.ResultPath != want || len(spec.ID) != 16 {
		t.Fatalf("spec %+v, detached %+v", spec, det)
	}
	if !slices.Equal(spec.Args, []string{"update", "-y"}) ||
		!slices.Equal(spec.Env, []string{EnvHandOff + "=" + spec.ID, EnvHandOffResult + "=" + want, "KEEP=1"}) {
		t.Fatalf("args %q, env %q", spec.Args, spec.Env)
	}
	if filepath.Base(want) != "."+filepath.Base(exe)+".selfupdate.handoff" {
		t.Fatalf("default result path %s", want)
	}
}

// TestHandOffIfInsideFailsClosed: an unanswerable Inside is an error, not
// "outside": proceeding could stop the caller's own service.
func TestHandOffIfInsideFailsClosed(t *testing.T) {
	t.Setenv(EnvHandOff, "")
	probe := errors.New("fixture: cgroup unreadable")
	d := &fakeDetacher{insideErr: probe}
	if _, handed, err := HandOffIfInside(context.Background(), d, HandOff{}); !errors.Is(err, probe) || handed {
		t.Fatalf("handed %t, err %v", handed, err)
	}
	if _, _, err := HandOffIfInside(context.Background(), nil, HandOff{}); err == nil {
		t.Fatal("a nil detacher was accepted")
	}
	if _, _, err := HandOffIfInside(context.Background(), &fakeDetacher{inside: true}, HandOff{ID: "a/b"}); err == nil {
		t.Fatal("an ID with a slash was accepted")
	}
	if _, _, err := HandOffIfInside(context.Background(), &fakeDetacher{inside: true}, HandOff{ResultPath: "rel/result"}); err == nil {
		t.Fatal("a relative result path was accepted")
	}
}

// TestWriteHandOffResultNeedsCleanAbsolutePath: the path comes from the
// environment, so a relative or unclean one is refused.
func TestWriteHandOffResultNeedsCleanAbsolutePath(t *testing.T) {
	dir := t.TempDir()
	// Built by hand: filepath.Join would clean the second one.
	sep := string(filepath.Separator)
	for _, p := range []string{"result", dir + sep + "x" + sep + ".." + sep + "result"} {
		if err := WriteHandOffResult(p, HandOffResult{}); err == nil {
			t.Errorf("%q was written", p)
		}
	}
	if err := WriteHandOffResult(filepath.Join(dir, "result"), HandOffResult{ID: "a"}); err != nil {
		t.Fatal(err)
	}
	if entries, err := os.ReadDir(dir); err != nil || len(entries) != 1 {
		t.Fatalf("left %v, %v; want only the result", entries, err)
	}
}

// TestDetachedRunNeverHandsOffAgain: with SELFUPDATE_HANDOFF set, nothing
// is asked or started.
func TestDetachedRunNeverHandsOffAgain(t *testing.T) {
	t.Setenv(EnvHandOff, "abc")
	d := &fakeDetacher{inside: true}
	if _, handed, err := HandOffIfInside(context.Background(), d, HandOff{}); err != nil || handed || d.got != nil {
		t.Fatalf("handed %t, err %v", handed, err)
	}
}

func TestHandOffFunc(t *testing.T) {
	t.Setenv(EnvHandOff, "")
	result := filepath.Join(t.TempDir(), "x")
	f := HandOffFunc(&fakeDetacher{inside: true}, HandOff{ID: "abc", ResultPath: result})
	handed, detail, err := f(context.Background(), selfupdate.Request{Yes: true})
	if err != nil || !handed || detail != "abc (fake); result in "+result {
		t.Fatalf("handed %t, detail %q, err %v", handed, detail, err)
	}
	// Inside, without --yes: the detached run could not ask
	// (docs/decisions/0015-MADR-remediate-third-debugging-pass-findings.md C3).
	handed, _, err = f(context.Background(), selfupdate.Request{})
	if handed || !errors.Is(err, selfupdate.ErrConfirmationRequired) || !strings.Contains(err.Error(), "pass --yes") {
		t.Fatalf("inside without --yes: handed %t, err %v", handed, err)
	}
	handed, detail, err = HandOffFunc(&fakeDetacher{}, HandOff{})(context.Background(), selfupdate.Request{})
	if err != nil || handed || detail != "" {
		t.Fatalf("outside: handed %t, detail %q, err %v", handed, detail, err)
	}
}

func TestReportFunc(t *testing.T) {
	path := filepath.Join(t.TempDir(), "result")
	t.Setenv(EnvHandOff, "")
	t.Setenv(EnvHandOffResult, path)
	report := ReportFunc()
	if err := report(selfupdate.Result{}, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadHandOffResult(path); err == nil {
		t.Fatal("a run that was not handed off wrote a result")
	}

	t.Setenv(EnvHandOff, "abc")
	res := selfupdate.Result{Product: "demo", CurrentVersion: "v1.0.0", TargetVersion: "v1.1.0", Applied: true}
	if err := report(res, nil); err != nil {
		t.Fatal(err)
	}
	got, err := ReadHandOffResult(path)
	if err != nil || got.ID != "abc" || got.ExitCode != 0 || got.Error != "" || !got.Result.Applied ||
		got.SchemaVersion != HandOffResultSchema || got.FinishedAt.Before(got.StartedAt) {
		t.Fatalf("result %+v, %v", got, err)
	}

	if err := report(selfupdate.Result{Product: "demo"}, errors.New("fixture: unhealthy")); err != nil {
		t.Fatal(err)
	}
	got, err = ReadHandOffResult(path)
	if err != nil || got.ExitCode != 1 || got.Error != "fixture: unhealthy" {
		t.Fatalf("failed run's result %+v, %v", got, err)
	}
}

func TestReadHandOffResultRefuses(t *testing.T) {
	dir := t.TempDir()
	for name, body := range map[string]string{"malformed": "not json", "huge": strings.Repeat(" ", maxHandOffResult+2)} {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := ReadHandOffResult(p); err == nil {
			t.Errorf("%s was read", name)
		}
	}
	if _, err := ReadHandOffResult(filepath.Join(dir, "missing")); err == nil {
		t.Error("a missing file was read")
	}
}

func TestProcessDetacherNeedsInside(t *testing.T) {
	if _, err := ProcessDetacher(nil).Inside(context.Background()); err == nil {
		t.Fatal("no inside check was accepted")
	}
	inside, err := ProcessDetacher(func(context.Context) (bool, error) { return true, nil }).Inside(context.Background())
	if err != nil || !inside {
		t.Fatalf("inside %t, err %v", inside, err)
	}
}
