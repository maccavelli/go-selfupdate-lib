package selfupdate

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"testing"
)

// Tests for docs/decisions/0010-PLAN-v1-6-0-owner-contracts.md S2 (Q2, B3):
// a stopped service is started after the update only when it was running,
// or is configured to start. These use only the v1.5.1 API, so they run
// against the unfixed code too.

// seqLife logs the service actions, not the probes.
type seqLife struct {
	running   bool
	healthErr error
	log       []string
}

func (l *seqLife) Installed(context.Context, string) (bool, error) { return true, nil }
func (l *seqLife) Running(context.Context, string) (bool, error)   { return l.running, nil }
func (l *seqLife) Stop(context.Context, string) error {
	l.log = append(l.log, "stop")
	return nil
}
func (l *seqLife) Start(context.Context, string) error {
	l.log = append(l.log, "start")
	return nil
}
func (l *seqLife) WaitHealthy(context.Context, string) error {
	l.log = append(l.log, "health")
	err := l.healthErr
	l.healthErr = nil // only the new binary is unhealthy
	return err
}

// seqEnabledLife can also say whether the service is configured to start.
type seqEnabledLife struct {
	seqLife
	enabled      bool
	enabledErr   error
	enabledCalls int
}

func (l *seqEnabledLife) Enabled(context.Context, string) (bool, error) {
	l.enabledCalls++
	return l.enabled, l.enabledErr
}

// installWith runs one managed install of "new-bytes" with life.
func installWith(t *testing.T, life Lifecycle, rec *fakeRec) (InstallResult, string, error) {
	t.Helper()
	_, exe := withTempHome(t)
	inner, err := NewStandaloneInstaller(InstallOptions{TargetPolicy: TargetPolicy{ExecutablePath: exe}})
	if err != nil {
		t.Fatal(err)
	}
	m, err := NewManagedInstaller(inner, life, rec)
	if err != nil {
		t.Fatal(err)
	}
	target, err := m.ResolveTarget(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	sess, err := m.Begin(context.Background(), target)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = sess.Close() }()
	res, err := sess.Install(context.Background(), InstallRequest{Product: "demo", Artifact: StagedArtifact{Path: stageNew(t, sess)}})
	return res, readString(t, exe), err
}

func TestManagedStartRule(t *testing.T) {
	for _, tc := range []struct {
		name string
		life func() (Lifecycle, *seqLife)
		want []string
	}{
		{"stopped, not enabled", func() (Lifecycle, *seqLife) {
			l := &seqEnabledLife{}
			return l, &l.seqLife
		}, nil},
		{"stopped, enabled", func() (Lifecycle, *seqLife) {
			l := &seqEnabledLife{enabled: true}
			return l, &l.seqLife
		}, []string{"start", "health"}},
		{"running, not enabled", func() (Lifecycle, *seqLife) {
			l := &seqEnabledLife{seqLife: seqLife{running: true}}
			return l, &l.seqLife
		}, []string{"stop", "start", "health"}},
		{"running, Enabled fails", func() (Lifecycle, *seqLife) {
			l := &seqEnabledLife{seqLife: seqLife{running: true}, enabledErr: errors.New("fixture: unit unreadable")}
			return l, &l.seqLife
		}, []string{"stop", "start", "health"}},
		{"stopped, no Enabled", func() (Lifecycle, *seqLife) {
			l := &seqLife{}
			return l, l
		}, nil},
		{"running, no Enabled", func() (Lifecycle, *seqLife) {
			l := &seqLife{running: true}
			return l, l
		}, []string{"stop", "start", "health"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			life, seq := tc.life()
			res, got, err := installWith(t, life, &fakeRec{})
			if err != nil || !res.Applied || !res.ServiceInstalled || got != "new-bytes" {
				t.Fatalf("res = %+v, err = %v, target %q", res, err, got)
			}
			if !slices.Equal(seq.log, tc.want) {
				t.Fatalf("lifecycle %v, want %v", seq.log, tc.want)
			}
		})
	}
}

// TestManagedEnabledErrorReplacesNothing: a stopped service whose start
// configuration cannot be read fails the install before anything changes.
func TestManagedEnabledErrorReplacesNothing(t *testing.T) {
	life := &seqEnabledLife{enabledErr: errors.New("fixture: unit unreadable")}
	res, got, err := installWith(t, life, &fakeRec{})
	if !errors.Is(err, ErrManagedInstall) || res.Applied || got != "old-bytes" || len(life.log) != 0 {
		t.Fatalf("res = %+v, err = %v, target %q, lifecycle %v", res, err, got, life.log)
	}
}

// swapRec moves the target's directory aside during Reconcile, so Commit is
// refused with ErrConcurrentUpdate.
type swapRec struct {
	fakeRec
	dir, moved string
	swapErr    error
}

func (r *swapRec) Reconcile(ctx context.Context, product, path string) (ReconcileResult, error) {
	if r.swapErr = os.Rename(r.dir, r.moved); r.swapErr == nil {
		r.swapErr = os.Mkdir(r.dir, 0o755)
	}
	return r.fakeRec.Reconcile(ctx, product, path)
}

// TestManagedCommitRefusedStartRule: a commit refused after the update
// recovers by the same rule. A service the update did not start is not
// stopped or started; one it started is stopped, and started on the old
// binary.
func TestManagedCommitRefusedStartRule(t *testing.T) {
	for _, tc := range []struct {
		name    string
		enabled bool
		want    []string
	}{
		{"stopped, not enabled", false, nil},
		{"stopped, enabled", true, []string{"start", "health", "stop", "start", "health"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, exe := withTempHome(t)
			dir := filepath.Dir(exe)
			rec := &swapRec{dir: dir, moved: dir + ".moved"}
			t.Cleanup(func() {
				if _, err := os.Lstat(rec.moved); err == nil {
					_ = os.RemoveAll(dir)
					_ = os.Rename(rec.moved, dir)
				}
			})
			inner, err := NewStandaloneInstaller(InstallOptions{TargetPolicy: TargetPolicy{ExecutablePath: exe}})
			if err != nil {
				t.Fatal(err)
			}
			life := &seqEnabledLife{enabled: tc.enabled}
			m, err := NewManagedInstaller(inner, life, rec)
			if err != nil {
				t.Fatal(err)
			}
			target, err := m.ResolveTarget(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			sess, err := m.Begin(context.Background(), target)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = sess.Close() }()
			res, err := sess.Install(context.Background(), InstallRequest{Product: "demo", Artifact: StagedArtifact{Path: stageNew(t, sess)}})
			if rec.swapErr != nil {
				if runtime.GOOS != goosWindows {
					t.Fatalf("directory swap failed: %v", rec.swapErr)
				}
				t.Skipf("the OS refused the swap: %v", rec.swapErr)
			}
			if !errors.Is(err, ErrConcurrentUpdate) || res.Applied || !res.RolledBack {
				t.Fatalf("res = %+v, err = %v; want the refused commit rolled back", res, err)
			}
			if !slices.Equal(life.log, tc.want) {
				t.Fatalf("lifecycle %v, want %v", life.log, tc.want)
			}
		})
	}
}

// TestManagedRecoveryRestartsOnlyWhatRan: recovery starts the old binary
// again only when the service was running, or this install started it.
func TestManagedRecoveryRestartsOnlyWhatRan(t *testing.T) {
	unhealthy := errors.New("fixture: unhealthy")
	for _, tc := range []struct {
		name string
		life func() (Lifecycle, *seqLife)
		rec  *fakeRec
		want []string
	}{
		{"stopped, not enabled, reconcile fails", func() (Lifecycle, *seqLife) {
			l := &seqEnabledLife{}
			return l, &l.seqLife
		}, &fakeRec{err: errors.New("fixture: reconcile")}, nil},
		{"stopped, no Enabled, reconcile fails", func() (Lifecycle, *seqLife) {
			l := &seqLife{}
			return l, l
		}, &fakeRec{err: errors.New("fixture: reconcile")}, nil},
		{"stopped, enabled, unhealthy", func() (Lifecycle, *seqLife) {
			l := &seqEnabledLife{enabled: true, seqLife: seqLife{healthErr: unhealthy}}
			return l, &l.seqLife
		}, &fakeRec{}, []string{"start", "health", "stop", "start", "health"}},
		{"running, reconcile fails", func() (Lifecycle, *seqLife) {
			l := &seqLife{running: true}
			return l, l
		}, &fakeRec{err: errors.New("fixture: reconcile")}, []string{"stop", "start", "health"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			life, seq := tc.life()
			res, got, err := installWith(t, life, tc.rec)
			if !errors.Is(err, ErrManagedInstall) || res.Applied || !res.RolledBack || got != "old-bytes" {
				t.Fatalf("res = %+v, err = %v, target %q", res, err, got)
			}
			if !slices.Equal(seq.log, tc.want) {
				t.Fatalf("lifecycle %v, want %v", seq.log, tc.want)
			}
		})
	}
}
