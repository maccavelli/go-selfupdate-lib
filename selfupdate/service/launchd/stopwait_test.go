package launchd

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/maccavelli/go-selfupdate-lib/selfupdate"
	"github.com/maccavelli/go-selfupdate-lib/selfupdate/service"
)

// Tests for docs/decisions/0015-MADR-remediate-third-debugging-pass-findings.md
// B3: once bootout is issued, Stop waits for the job to go whatever the
// caller's context does, bounded by launchd's own kill bound; and a managed
// update whose stop failed after the job went starts it again.

// TestStopFinishesAfterCancel: a caller's context that ends after bootout
// does not cut the wait short. Stop waits until the job is gone, then
// reports the cancellation.
func TestStopFinishesAfterCancel(t *testing.T) {
	f := newFake()
	f.pid = 99999999 // a PID no process has
	f.goneAfter = 3
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	f.onBootout = func(*fakeLaunchd) { cancel() }
	j := testJob(t, f, Options{})
	err := j.Stop(ctx, "demo")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Stop = %v, want the caller's cancellation", err)
	}
	v := f.verbs()
	bootout := slices.Index(v, "bootout")
	if bootout < 0 || strings.Count(strings.Join(v[bootout:], " "), "print") < 3 || f.loaded {
		t.Fatalf("verbs %q, loaded %t: Stop returned while the job was still loaded", v, f.loaded)
	}
}

// TestManagedStopTimeoutRestartsJob: the job leaves launchd's list after
// bootout, but the stop wait times out first. The managed update then
// finds the job not running and starts it again.
func TestManagedStopTimeoutRestartsJob(t *testing.T) {
	f := newFake()
	f.pid = 99999999
	f.goneAfter = 1 << 30
	// ExitTimeOut 0 is no kill bound, so the wait is Poll.Timeout's.
	f.keys["ExitTimeOut"] = "0"
	j := testJob(t, f, Options{Poll: service.PollOptions{Timeout: 100 * time.Millisecond}})
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(dir, "demo")
	if err := os.WriteFile(target, []byte("old-bytes"), 0o755); err != nil { //nolint:gosec // an executable fixture
		t.Fatal(err)
	}
	res, err := fakeManagedInstall(t, j, target)
	if !errors.Is(err, selfupdate.ErrManagedInstall) || res.Applied {
		t.Fatalf("Install = %+v, %v; want a failed update", res, err)
	}
	v := f.verbs()
	bootout := slices.Index(v, "bootout")
	if bootout < 0 || !slices.Contains(v[bootout:], "kickstart") {
		t.Fatalf("verbs %q; want the job started again after the failed stop", v)
	}
	if got, _ := os.ReadFile(target); string(got) != "old-bytes" { //nolint:gosec // the test's own file
		t.Fatalf("target holds %q", got)
	}
}

// fakeManagedInstall installs "new-bytes" over target through a managed
// installer with j as its lifecycle and reconciler.
func fakeManagedInstall(t *testing.T, j *Job, target string) (selfupdate.InstallResult, error) {
	t.Helper()
	ctx := context.Background()
	inner, err := selfupdate.NewStandaloneInstaller(selfupdate.InstallOptions{
		TargetPolicy: selfupdate.TargetPolicy{ExecutablePath: target, AllowedRoots: []string{filepath.Dir(target)}},
	})
	if err != nil {
		t.Fatal(err)
	}
	m, err := selfupdate.NewManagedInstaller(inner, j, j)
	if err != nil {
		t.Fatal(err)
	}
	tgt, err := m.ResolveTarget(ctx)
	if err != nil {
		t.Fatal(err)
	}
	sess, err := m.Begin(ctx, tgt)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sess.Close() })
	fh, path, err := sess.CreateStaging(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fh.Write([]byte("new-bytes")); err != nil {
		t.Fatal(err)
	}
	if err := fh.Close(); err != nil {
		t.Fatal(err)
	}
	return sess.Install(ctx, selfupdate.InstallRequest{Product: "demo", Artifact: selfupdate.StagedArtifact{Path: path}})
}

// TestStopBound: the stop wait is the longer of the poll timeout and
// ExitTimeOut plus stopGrace, 5 s plus stopGrace when ExitTimeOut is
// absent. An ExitTimeOut of 0, or one that is not an integer, leaves the
// poll timeout, and so does an unreadable plist.
func TestStopBound(t *testing.T) {
	for _, c := range []struct {
		name    string
		key     string
		typ     string
		corrupt bool
		poll    time.Duration
		want    time.Duration
	}{
		{"absent", "", "", false, 0, service.DefaultPollTimeout},
		{"absent, short poll", "", "", false, time.Second, defaultExitTimeOut + stopGrace},
		{"ExitTimeOut 90", "90", "", false, 0, 90*time.Second + stopGrace},
		{"ExitTimeOut 0", "0", "", false, time.Second, time.Second},
		{"a string", "soon", "string", false, time.Second, time.Second},
		{"unreadable", "90", "", true, time.Second, time.Second},
	} {
		f := newFake()
		if c.key != "" {
			f.keys["ExitTimeOut"] = c.key
		}
		if c.typ != "" {
			f.types = map[string]string{"ExitTimeOut": c.typ}
		}
		f.corrupt = c.corrupt
		j := testJob(t, f, Options{})
		j.o.Poll.Timeout = c.poll
		if got := j.stopBound(context.Background()); got != c.want {
			t.Errorf("%s: bound %v, want %v", c.name, got, c.want)
		}
	}
}

// TestStopWaitFollowsExitTimeOut: the wait after bootout runs to the
// bound, not to the poll timeout, so a job that takes longer than the poll
// timeout to exit is waited for.
func TestStopWaitFollowsExitTimeOut(t *testing.T) {
	f := newFake()
	f.pid = 99999999
	f.goneAfter = 3
	f.keys["ExitTimeOut"] = "90"
	j := testJob(t, f, Options{Poll: service.PollOptions{Timeout: time.Millisecond}})
	if err := j.Stop(context.Background(), "demo"); err != nil {
		t.Fatalf("Stop = %v; a 1 ms poll timeout must not cut a 120 s bound short", err)
	}
}
