package selfupdate

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// orderLife and orderRec record lifecycle and reconciler calls into the log
// the custom session writes to, so one slice holds the whole call order.
type orderLife struct {
	log       *[]string
	healthErr error
}

func (l *orderLife) Installed(context.Context, string) (bool, error) { return true, nil }
func (l *orderLife) Running(context.Context, string) (bool, error)   { return true, nil }
func (l *orderLife) Stop(context.Context, string) error {
	*l.log = append(*l.log, "Stop")
	return nil
}
func (l *orderLife) Start(context.Context, string) error {
	*l.log = append(*l.log, "Start")
	return nil
}
func (l *orderLife) WaitHealthy(context.Context, string) error {
	*l.log = append(*l.log, "WaitHealthy")
	return l.healthErr
}

type orderRec struct{ log *[]string }

func (r *orderRec) Reconcile(context.Context, string, string) (ReconcileResult, error) {
	*r.log = append(*r.log, "Reconcile")
	return ReconcileResult{Changed: true, State: "unit"}, nil
}
func (r *orderRec) Restore(context.Context, string, ReconcileResult) error {
	*r.log = append(*r.log, "Restore")
	return nil
}

// customTwoPhase is a consumer's own TwoPhaseSession, with no access to the
// package's session internals.
type customTwoPhase struct {
	log    *[]string
	dir    string
	closes int
	// got is the replacement handed to Commit or Rollback.
	got AppliedReplacement
}

const customState = "custom-state"

func (s *customTwoPhase) Target() Target {
	return Target{Path: filepath.Join(s.dir, "demo"), Dir: s.dir, Base: "demo"}
}
func (s *customTwoPhase) CreateStaging(context.Context) (*os.File, string, error) {
	f, err := os.CreateTemp(s.dir, ".demo.custom-")
	if err != nil {
		return nil, "", err
	}
	return f, f.Name(), nil
}
func (s *customTwoPhase) Install(context.Context, InstallRequest) (InstallResult, error) {
	*s.log = append(*s.log, "Install")
	return InstallResult{Applied: true}, nil
}
func (s *customTwoPhase) Close() error {
	s.closes++
	return nil
}
func (s *customTwoPhase) Owns(path string) bool { return filepath.Dir(path) == s.dir }
func (s *customTwoPhase) Apply(context.Context, InstallRequest) (AppliedReplacement, error) {
	*s.log = append(*s.log, "Apply")
	return AppliedReplacement{Target: s.Target().Path, Backup: s.Target().Path + ".bak", State: customState}, nil
}
func (s *customTwoPhase) Commit(_ context.Context, a AppliedReplacement) (InstallResult, error) {
	*s.log = append(*s.log, "Commit")
	s.got = a
	return InstallResult{Target: a.Target, Applied: true}, nil
}
func (s *customTwoPhase) Rollback(_ context.Context, a AppliedReplacement) error {
	*s.log = append(*s.log, "Rollback")
	s.got = a
	return nil
}

// customInstaller hands out one session. sess is an InstallSession so a
// test can offer a session that is not two-phase.
type customInstaller struct{ sess InstallSession }

func (c customInstaller) ResolveTarget(context.Context) (Target, error) { return c.sess.Target(), nil }
func (c customInstaller) Begin(context.Context, Target) (InstallSession, error) {
	return c.sess, nil
}

func customManaged(t *testing.T, sess InstallSession, life Lifecycle, rec Reconciler) (InstallSession, error) {
	t.Helper()
	m, err := NewManagedInstallerFor(customInstaller{sess: sess}, life, rec)
	if err != nil {
		t.Fatal(err)
	}
	return m.Begin(context.Background(), sess.Target())
}

func TestManagedDrivesCustomTwoPhaseSession(t *testing.T) {
	log := &[]string{}
	custom := &customTwoPhase{log: log, dir: t.TempDir()}
	sess, err := customManaged(t, custom, &orderLife{log: log}, &orderRec{log: log})
	if err != nil {
		t.Fatal(err)
	}
	res, err := sess.Install(context.Background(), InstallRequest{Product: "demo"})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"Stop", "Apply", "Reconcile", "Start", "WaitHealthy", "Commit"}
	if !slices.Equal(*log, want) {
		t.Fatalf("calls = %v, want %v", *log, want)
	}
	if custom.got.State != customState {
		t.Fatalf("Commit got State %v, want the value Apply returned", custom.got.State)
	}
	if !res.Applied || !res.ServiceInstalled || !res.ServiceWasRunning {
		t.Fatalf("res = %+v, want an applied install of a running service", res)
	}
}

func TestManagedRollsBackCustomSession(t *testing.T) {
	log := &[]string{}
	custom := &customTwoPhase{log: log, dir: t.TempDir()}
	unhealthy := errors.New("unhealthy")
	sess, err := customManaged(t, custom, &orderLife{log: log, healthErr: unhealthy}, &orderRec{log: log})
	if err != nil {
		t.Fatal(err)
	}
	res, err := sess.Install(context.Background(), InstallRequest{Product: "demo"})
	if !errors.Is(err, ErrManagedInstall) || !errors.Is(err, unhealthy) {
		t.Fatalf("err = %v, want ErrManagedInstall joined with the health failure", err)
	}
	// The started, unhealthy binary is stopped before the old one is
	// restored (0010-MADR B4).
	want := []string{"Stop", "Apply", "Reconcile", "Start", "WaitHealthy", "Stop", "Restore", "Rollback", "Start", "WaitHealthy"}
	if !slices.Equal(*log, want) {
		t.Fatalf("calls = %v, want %v", *log, want)
	}
	if custom.got.State != customState {
		t.Fatalf("Rollback got State %v, want the value Apply returned", custom.got.State)
	}
	if !res.RolledBack || res.Applied {
		t.Fatalf("res = %+v, want a rolled-back install", res)
	}
}

func TestManagedRejectsSingleStepSession(t *testing.T) {
	single := &customTwoPhase{log: &[]string{}, dir: t.TempDir()}
	// Only the InstallSession methods are visible through this value.
	var plain InstallSession = struct{ InstallSession }{single}
	sess, err := customManaged(t, plain, &orderLife{log: &[]string{}}, &orderRec{log: &[]string{}})
	if err == nil || !strings.Contains(err.Error(), "managed installer requires a two-phase session") {
		t.Fatalf("Begin = %v, %v; want the two-phase refusal", sess, err)
	}
	if single.closes != 1 {
		t.Fatalf("the refused session was closed %d times, want 1", single.closes)
	}
}

func TestNewManagedInstallerForRejectsNil(t *testing.T) {
	var typedNil *StandaloneInstaller
	for name, inner := range map[string]Installer{"nil": nil, "typed nil": typedNil} {
		if _, err := NewManagedInstallerFor(inner, &fakeLife{}, &fakeRec{}); err == nil {
			t.Errorf("%s installer accepted", name)
		}
	}
}

type ownerFunc func(string) bool

func (f ownerFunc) Owns(path string) bool { return f(path) }

func TestSessOwnsFailsClosed(t *testing.T) {
	plain := pendingSession{}
	if sessOwns(plain, "/any/path") {
		t.Fatal("a session that is not a StagingOwner owns a path")
	}
	yes := struct {
		pendingSession
		ownerFunc
	}{ownerFunc: func(string) bool { return true }}
	no := struct {
		pendingSession
		ownerFunc
	}{ownerFunc: func(string) bool { return false }}
	if !sessOwns(yes, "/any/path") || sessOwns(no, "/any/path") {
		t.Fatal("sessOwns did not ask the StagingOwner")
	}

	_, exe := withTempHome(t)
	inst, err := NewStandaloneInstaller(InstallOptions{TargetPolicy: TargetPolicy{ExecutablePath: exe}})
	if err != nil {
		t.Fatal(err)
	}
	target, err := inst.ResolveTarget(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	sess, err := inst.Begin(context.Background(), target)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = sess.Close() }()
	path := stageNew(t, sess)
	if !sessOwns(sess, path) || sessOwns(sess, exe) {
		t.Fatal("the standalone session misreports its staging")
	}
}

// unownedInstaller hides logSession's Owns, as a custom installer written
// before StagingOwner existed would.
type unownedInstaller struct{ *logInstaller }

func (u unownedInstaller) Begin(ctx context.Context, t Target) (InstallSession, error) {
	sess, err := u.logInstaller.Begin(ctx, t)
	if err != nil {
		return nil, err
	}
	return struct{ InstallSession }{sess}, nil
}

// TestTransformRequiresStagingOwner: the behaviour change in the release
// notes. A custom session paired with a Transformer must say which staging
// it owns, or the run stops before Install.
func TestTransformRequiresStagingOwner(t *testing.T) {
	env := newContractEnv(t)
	_, _, plats := fixtureRelease(t, "demo")
	sel, err := NewExactAssetSelector(plats)
	if err != nil {
		t.Fatal(err)
	}
	u, err := New(Config{
		Source: env.src, Versions: NewStrictVersionPolicy(), Assets: sel,
		Transformer: growTransformer{extra: []byte("-signed")},
		Installer:   unownedInstaller{env.inst},
		Reporter:    env.rep, Confirmer: env.conf, Limits: env.lim,
	})
	if err != nil {
		t.Fatal(err)
	}
	req := applyReq()
	req.Yes = true
	_, err = u.Run(context.Background(), req)
	if err == nil || !strings.Contains(err.Error(), "transformed staging is not owned by the session") {
		t.Fatalf("Run = %v, want the ownership refusal", err)
	}
	if slices.Contains(*env.log, "Install") {
		t.Fatalf("Install ran: %v", *env.log)
	}
}

func TestCommitRejectsForeignState(t *testing.T) {
	_, exe := withTempHome(t)
	before, err := os.ReadFile(exe)
	if err != nil {
		t.Fatal(err)
	}
	inst, err := NewStandaloneInstaller(InstallOptions{TargetPolicy: TargetPolicy{ExecutablePath: exe}})
	if err != nil {
		t.Fatal(err)
	}
	target, err := inst.ResolveTarget(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	sess, err := inst.Begin(context.Background(), target)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = sess.Close() }()
	two, ok := sess.(TwoPhaseSession)
	if !ok {
		t.Fatal("the standalone session is not a TwoPhaseSession")
	}
	const want = "selfupdate: replacement was not applied by this session"
	for name, state := range map[string]any{"nil": nil, "foreign": customState} {
		a := AppliedReplacement{Target: exe, Backup: exe + ".bak", State: state}
		if _, err := two.Commit(context.Background(), a); err == nil || err.Error() != want {
			t.Errorf("Commit(%s) = %v, want %q", name, err, want)
		}
		if err := two.Rollback(context.Background(), a); err == nil || err.Error() != want {
			t.Errorf("Rollback(%s) = %v, want %q", name, err, want)
		}
	}
	after, err := os.ReadFile(exe)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(before) {
		t.Fatal("a foreign replacement changed the target")
	}
}

// TestStandaloneApplyCommit: the standalone session's own two-phase path
// replaces the target, keeps a backup until Commit, and removes it then.
func TestStandaloneApplyCommit(t *testing.T) {
	_, exe := withTempHome(t)
	inst, err := NewStandaloneInstaller(InstallOptions{TargetPolicy: TargetPolicy{ExecutablePath: exe}})
	if err != nil {
		t.Fatal(err)
	}
	target, err := inst.ResolveTarget(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	sess, err := inst.Begin(context.Background(), target)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = sess.Close() }()
	two := sess.(TwoPhaseSession)
	path := stageNew(t, sess)
	applied, err := two.Apply(context.Background(), InstallRequest{Product: "demo", Artifact: StagedArtifact{Path: path}})
	if err != nil {
		t.Fatal(err)
	}
	if applied.Target != target.Path || applied.Backup == "" {
		t.Fatalf("applied = %+v, want the target and a backup", applied)
	}
	if _, err := os.Lstat(applied.Backup); err != nil {
		t.Fatalf("backup before Commit: %v", err)
	}
	res, err := two.Commit(context.Background(), applied)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Applied || res.Target != target.Path {
		t.Fatalf("res = %+v", res)
	}
	if _, err := os.Lstat(applied.Backup); !errors.Is(err, os.ErrNotExist) && res.PendingBackup == "" {
		t.Fatalf("backup after Commit: %v, want it removed", err)
	}
	got, err := os.ReadFile(exe)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "new-bytes" {
		t.Fatalf("target = %q, want the new bytes", got)
	}
}
