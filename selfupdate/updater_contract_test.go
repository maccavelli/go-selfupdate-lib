package selfupdate

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// Tests for the coordinator contract of mcplib
// docs/0005-PLAN-canonicalize-cli-self-update-in-mcplib.md §4.6, added by
// docs/decisions/0003-PLAN-remediate-debugging-pass-findings.md Phase 1.

// logInstaller and logSession record every call in a shared log, and can be
// told to fail at one of them.
type logInstaller struct {
	log        *[]string
	dir        string
	failAt     string
	applied    bool
	installErr error
	closeErr   error
	pending    string
}

func (i *logInstaller) fail(call string) error {
	*i.log = append(*i.log, call)
	if i.failAt == call {
		return errors.New("injected " + call + " failure")
	}
	return nil
}

func (i *logInstaller) target() Target {
	return Target{Path: filepath.Join(i.dir, "demo"), Dir: i.dir, Base: "demo"}
}

func (i *logInstaller) ResolveTarget(context.Context) (Target, error) {
	if err := i.fail("ResolveTarget"); err != nil {
		return Target{}, err
	}
	return i.target(), nil
}

func (i *logInstaller) Begin(context.Context, Target) (InstallSession, error) {
	if err := i.fail("Begin"); err != nil {
		return nil, err
	}
	return &logSession{inst: i}, nil
}

type logSession struct{ inst *logInstaller }

func (s *logSession) Target() Target { return s.inst.target() }

func (s *logSession) CreateStaging(context.Context) (*os.File, string, error) {
	if err := s.inst.fail("CreateStaging"); err != nil {
		return nil, "", err
	}
	f, err := os.CreateTemp(s.inst.dir, ".demo.staging-")
	if err != nil {
		return nil, "", err
	}
	return f, f.Name(), nil
}

func (s *logSession) Install(context.Context, InstallRequest) (InstallResult, error) {
	if err := s.inst.fail("Install"); err != nil {
		return InstallResult{}, err
	}
	return InstallResult{Applied: s.inst.applied, PendingBackup: s.inst.pending}, s.inst.installErr
}

// Owns implements StagingOwner for the staging files CreateStaging makes.
func (s *logSession) Owns(path string) bool {
	return filepath.Dir(path) == s.inst.dir && strings.HasPrefix(filepath.Base(path), ".demo.staging-")
}

func (s *logSession) Close() error {
	*s.inst.log = append(*s.inst.log, "Close")
	return s.inst.closeErr
}

type logVerifier struct {
	log    *[]string
	failAt bool
}

func (v *logVerifier) Verify(context.Context, Verification) error {
	*v.log = append(*v.log, "Verify")
	if v.failAt {
		return errors.New("injected Verify failure")
	}
	return nil
}

type logConfirmer struct {
	log *[]string
	err error
}

func (c *logConfirmer) Confirm(context.Context, Prompt) (bool, error) {
	*c.log = append(*c.log, "Confirm")
	return c.err == nil, c.err
}

type contractEnv struct {
	log  *[]string
	src  *scriptSource
	inst *logInstaller
	rep  *recReporter
	ver  *logVerifier
	conf *logConfirmer
	xf   Transformer
	lim  Limits
	u    *Updater
}

func newContractEnv(t *testing.T) *contractEnv {
	t.Helper()
	rel, bodies, _ := fixtureRelease(t, "demo")
	log := &[]string{}
	env := &contractEnv{
		log:  log,
		src:  &scriptSource{rel: rel, bodies: bodies, log: log},
		inst: &logInstaller{log: log, dir: t.TempDir(), applied: true},
		rep:  &recReporter{log: log},
		ver:  &logVerifier{log: log},
		conf: &logConfirmer{log: log},
		lim:  DefaultLimits(),
	}
	return env
}

func (e *contractEnv) build(t *testing.T) *Updater {
	t.Helper()
	_, _, plats := fixtureRelease(t, "demo")
	sel, err := NewExactAssetSelector(plats)
	if err != nil {
		t.Fatal(err)
	}
	u, err := New(Config{
		Source: e.src, Versions: NewStrictVersionPolicy(), Assets: sel,
		Verifiers: []Verifier{e.ver}, Transformer: e.xf,
		Installer: e.inst, Reporter: e.rep, Confirmer: e.conf, Limits: e.lim,
	})
	if err != nil {
		t.Fatal(err)
	}
	e.u = u
	return u
}

func applyReq() Request {
	return Request{Product: "demo", CurrentVersion: "v1.0.0", CurrentBuild: ReleaseBuild}
}

func calls(log []string) []string {
	var out []string
	for _, c := range log {
		if !strings.HasPrefix(c, "event:") {
			out = append(out, c)
		}
	}
	return out
}

// TestRunCallOrderFailureBoundaries injects a failure at every collaborator
// call and asserts that no later call happens, except the session Close
// that Begin's safety net always runs (0003-MADR C13).
func TestRunCallOrderFailureBoundaries(t *testing.T) {
	order := []string{"ResolveTarget", "Latest", "Confirm", "Begin", "OpenAsset:3", "CreateStaging", "OpenAsset:2", "Verify", "Transform", "Install"}
	for i, boundary := range order {
		t.Run(boundary, func(t *testing.T) {
			env := newContractEnv(t)
			env.xf = transformerFunc(func(context.Context, TransformRequest) error {
				*env.log = append(*env.log, "Transform")
				if boundary == "Transform" {
					return errors.New("injected Transform failure")
				}
				return nil
			})
			switch boundary {
			case "ResolveTarget", "Begin", "CreateStaging", "Install":
				env.inst.failAt = boundary
			case "Latest":
				env.src.err = errors.New("injected Latest failure")
			case "Confirm":
				env.conf.err = errors.New("injected Confirm failure")
			case "OpenAsset:3":
				env.src.openErr = map[int64]error{3: errors.New("injected manifest failure")}
			case "OpenAsset:2":
				env.src.openErr = map[int64]error{2: errors.New("injected binary failure")}
			case "Verify":
				env.ver.failAt = true
			}
			u := env.build(t)
			res, err := u.Run(context.Background(), applyReq())
			if err == nil || res.Applied {
				t.Fatalf("failure at %s: res=%+v err=%v", boundary, res, err)
			}
			got := calls(*env.log)
			for _, later := range order[i+1:] {
				if slices.Contains(got, later) {
					t.Fatalf("failure at %s, yet %s was called: %v", boundary, later, got)
				}
			}
			begun := slices.Contains(got, "Begin") && boundary != "Begin"
			if begun != slices.Contains(got, "Close") {
				t.Fatalf("failure at %s: Begin/Close mismatch: %v", boundary, got)
			}
		})
	}
}

// TestRunOperationsMatrix exercises every Operation through Run
// (0003-MADR C13).
func TestRunOperationsMatrix(t *testing.T) {
	cases := []struct {
		name    string
		req     func() Request
		tag     string
		wantOp  Operation
		wantErr bool
	}{
		{"upgrade latest", applyReq, "v1.1.0", OperationUpgrade, false},
		{"exact upgrade", func() Request { r := applyReq(); r.TargetVersion = "v1.1.0"; return r }, "v1.1.0", OperationUpgrade, false},
		{"exact rollback", func() Request { r := applyReq(); r.CurrentVersion = "v1.2.0"; r.TargetVersion = "v1.1.0"; return r }, "v1.1.0", OperationRollback, false},
		{"reinstall with force", func() Request { r := applyReq(); r.CurrentVersion = "v1.1.0"; r.Force = true; return r }, "v1.1.0", OperationReinstall, false},
		{"same version without force", func() Request { r := applyReq(); r.CurrentVersion = "v1.1.0"; return r }, "v1.1.0", OperationNone, false},
		{"local build with force", func() Request {
			r := applyReq()
			r.CurrentBuild, r.CurrentVersion, r.Force = LocalBuild, "dev", true
			return r
		}, "v1.1.0", OperationReplaceLocal, false},
		{"local build without force", func() Request { r := applyReq(); r.CurrentBuild, r.CurrentVersion = LocalBuild, "dev"; return r }, "v1.1.0", OperationNone, true},
		{"latest older than running", func() Request { r := applyReq(); r.CurrentVersion = "v1.2.0"; return r }, "v1.1.0", OperationNone, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			env := newContractEnv(t)
			env.src.rel.Tag = tc.tag
			u := env.build(t)
			req := tc.req()
			req.Yes = true
			res, err := u.Run(context.Background(), req)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("want error, got %+v", res)
				}
				if slices.Contains(calls(*env.log), "Begin") {
					t.Fatalf("began despite error: %v", *env.log)
				}
				return
			}
			if err != nil || res.Operation != tc.wantOp {
				t.Fatalf("op=%s err=%v, want %s", res.Operation, err, tc.wantOp)
			}
			wantApplied := tc.wantOp != OperationNone
			if res.Applied != wantApplied {
				t.Fatalf("applied=%v want %v", res.Applied, wantApplied)
			}
		})
	}
}

// TestRunRejectsTagMismatch: an exact --version must return that tag
// (0003-MADR C1).
func TestRunRejectsTagMismatch(t *testing.T) {
	env := newContractEnv(t)
	env.src.rel.Tag = "v1.1.0"
	u := env.build(t)
	req := applyReq()
	req.TargetVersion, req.Yes = "v1.0.5", true
	res, err := u.Run(context.Background(), req)
	if !errors.Is(err, ErrIntegrity) || res.Applied {
		t.Fatalf("res=%+v err=%v", res, err)
	}
	if !strings.Contains(err.Error(), `"v1.1.0"`) || !strings.Contains(err.Error(), `"v1.0.5"`) {
		t.Fatalf("error does not name both tags: %v", err)
	}
	if slices.Contains(calls(*env.log), "Begin") {
		t.Fatalf("began on a mismatched tag: %v", *env.log)
	}
}

// TestRunValidatesSelectedAssets: the selected binary and manifest are
// validated by the Updater, each against its own limit, before check mode
// reports (0003-MADR A1 and A5).
func TestRunValidatesSelectedAssets(t *testing.T) {
	cases := map[string]func(*contractEnv){
		"binary not uploaded": func(e *contractEnv) { e.src.rel.Assets[0].State = "open" },
		"binary zero size":    func(e *contractEnv) { e.src.rel.Assets[0].Size = 0 },
		"binary bad digest":   func(e *contractEnv) { e.src.rel.Assets[0].Digest = "sha512:abc" },
		"manifest over its own limit": func(e *contractEnv) {
			e.lim.Manifest = 8
		},
		"binary over executable limit": func(e *contractEnv) {
			e.lim.Executable = 4
		},
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			env := newContractEnv(t)
			mutate(env)
			u := env.build(t)
			req := applyReq()
			req.CheckOnly = true
			res, err := u.Run(context.Background(), req)
			// A failed check still says it was a check: Checked echoes the
			// request (0015-MADR C6; 0015-PLAN deviation D2).
			if err == nil || errors.Is(err, ErrUpdateAvailable) || !res.Checked {
				t.Fatalf("res=%+v err=%v", res, err)
			}
		})
	}
}

// TestRunEscapesUntrustedTag: a control-character tag never reaches error
// text raw (0003-MADR C4).
func TestRunEscapesUntrustedTag(t *testing.T) {
	for _, mutable := range []bool{true, false} {
		env := newContractEnv(t)
		env.src.rel.Tag = "\x1b]0;x\x07v9.9.9\n::error::x"
		env.src.rel.Immutable = !mutable
		env.src.rel.Draft = !mutable
		u := env.build(t)
		_, err := u.Run(context.Background(), applyReq())
		if err == nil {
			t.Fatal("accepted a control-character tag")
		}
		if strings.ContainsAny(err.Error(), "\x1b\x07\n") {
			t.Fatalf("raw control characters in error: %q", err.Error())
		}
	}
}

// TestRunRejectsManifestBeforeStaging: a manifest without the entry fails
// before any staging or binary download (0003-MADR C9).
func TestRunRejectsManifestBeforeStaging(t *testing.T) {
	env := newContractEnv(t)
	env.src.bodies[3] = []byte(strings.Repeat("0", 64) + "  some-other-asset\n")
	env.src.rel.Assets[1].Size = int64(len(env.src.bodies[3]))
	u := env.build(t)
	req := applyReq()
	req.Yes = true
	if _, err := u.Run(context.Background(), req); err == nil {
		t.Fatal("accepted a manifest without the selected entry")
	}
	got := calls(*env.log)
	if slices.Contains(got, "CreateStaging") || slices.Contains(got, "OpenAsset:2") {
		t.Fatalf("staged or downloaded the binary first: %v", got)
	}
}

// TestRunEventOrderWithTransformer: verified precedes transforming, and the
// transformed staging is rehashed (0003-MADR C8; covers hashFile, sessOwns
// and hashAndValidateStaging).
func TestRunEventOrderWithTransformer(t *testing.T) {
	rel, bodies, plats := fixtureRelease(t, "demo")
	src := &scriptSource{rel: rel, bodies: bodies}
	_, exe := withTempHome(t)
	inst, err := NewStandaloneInstaller(InstallOptions{TargetPolicy: TargetPolicy{ExecutablePath: exe}})
	if err != nil {
		t.Fatal(err)
	}
	sel, _ := NewExactAssetSelector(plats)
	rep := &recReporter{}
	xf := transformerFunc(func(_ context.Context, req TransformRequest) error {
		f, err := os.OpenFile(req.Path, os.O_WRONLY|os.O_APPEND, 0)
		if err != nil {
			return err
		}
		if _, err := f.WriteString("-signed"); err != nil {
			return errors.Join(err, f.Close())
		}
		return f.Close()
	})
	u, err := New(Config{
		Source: src, Versions: NewStrictVersionPolicy(), Assets: sel, Transformer: xf,
		Installer: inst, Reporter: rep, Confirmer: &recConfirmer{ok: true}, Limits: DefaultLimits(),
	})
	if err != nil {
		t.Fatal(err)
	}
	res, err := u.Run(context.Background(), Request{Product: "demo", CurrentVersion: "v1.0.0", CurrentBuild: ReleaseBuild, Yes: true})
	if err != nil || !res.Applied {
		t.Fatalf("res=%+v err=%v", res, err)
	}
	want := []EventKind{
		EventResolvingTarget, EventFetchingRelease, EventSelected, EventDownloadingManifest,
		EventDownloadingBinary, EventVerified, EventTransforming, EventInstalling, EventComplete,
	}
	if !slices.Equal(rep.kinds, want) {
		t.Fatalf("events = %v\nwant     %v", rep.kinds, want)
	}
	sum := sha256.Sum256([]byte("hello-bin-signed"))
	if res.InstalledDigest != hex.EncodeToString(sum[:]) || res.InstalledDigest == res.ReleaseDigest {
		t.Fatalf("installed digest %s release digest %s", res.InstalledDigest, res.ReleaseDigest)
	}
	got, _ := os.ReadFile(exe)
	if string(got) != "hello-bin-signed" {
		t.Fatalf("installed %q", got)
	}
}

// TestRunTransformerSizeLimit: a transform that grows staging past the
// executable limit is refused.
func TestRunTransformerSizeLimit(t *testing.T) {
	rel, bodies, plats := fixtureRelease(t, "demo")
	src := &scriptSource{rel: rel, bodies: bodies}
	_, exe := withTempHome(t)
	inst, _ := NewStandaloneInstaller(InstallOptions{TargetPolicy: TargetPolicy{ExecutablePath: exe}})
	sel, _ := NewExactAssetSelector(plats)
	lim := DefaultLimits()
	lim.Executable = 64
	xf := transformerFunc(func(_ context.Context, req TransformRequest) error {
		return os.WriteFile(req.Path, bytes.Repeat([]byte("x"), 128), 0o600)
	})
	u, err := New(Config{
		Source: src, Versions: NewStrictVersionPolicy(), Assets: sel, Transformer: xf,
		Installer: inst, Reporter: &recReporter{}, Confirmer: &recConfirmer{ok: true}, Limits: lim,
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = u.Run(context.Background(), Request{Product: "demo", CurrentVersion: "v1.0.0", CurrentBuild: ReleaseBuild, Yes: true})
	if err == nil || !strings.Contains(err.Error(), "exceeds executable limit") {
		t.Fatalf("err = %v", err)
	}
}

// TestRunInstallerNoCommitIsError: Applied false with a nil error is a
// failure, not success (0003-MADR C2).
func TestRunInstallerNoCommitIsError(t *testing.T) {
	env := newContractEnv(t)
	env.inst.applied = false
	u := env.build(t)
	req := applyReq()
	req.Yes = true
	res, err := u.Run(context.Background(), req)
	if !errors.Is(err, errNotCommitted) || res.Applied || ExitCode(res, err) != 1 {
		t.Fatalf("res=%+v err=%v exit=%d", res, err, ExitCode(res, err))
	}
	if containsKind(env.rep.kinds, EventComplete) {
		t.Fatal("reported complete without a commit")
	}
}

// TestRunCompleteAfterClose: the session is closed before EventComplete
// (0003-MADR C14).
func TestRunCompleteAfterClose(t *testing.T) {
	env := newContractEnv(t)
	u := env.build(t)
	req := applyReq()
	req.Yes = true
	if _, err := u.Run(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	closeAt := slices.Index(*env.log, "Close")
	completeAt := slices.Index(*env.log, "event:"+EventComplete.String())
	if closeAt < 0 || completeAt < 0 || closeAt > completeAt {
		t.Fatalf("Close at %d, complete at %d: %v", closeAt, completeAt, *env.log)
	}
	if n := strings.Count(strings.Join(*env.log, ","), "Close"); n != 1 {
		t.Fatalf("Close called %d times: %v", n, *env.log)
	}
}

// TestRunCloseErrorIsAWarning: a Close failure after a committed install
// keeps Applied true, and is a warning, not an error. It was
// TestRunCloseErrorJoinedAfterCommit, which expected the error: the owner's
// answer to 0010-MADR Q3 reversed that.
func TestRunCloseErrorIsAWarning(t *testing.T) {
	env := newContractEnv(t)
	env.inst.closeErr = errors.New("unlock failed")
	u := env.build(t)
	req := applyReq()
	req.Yes = true
	res, err := u.Run(context.Background(), req)
	if !res.Applied || err != nil || !slices.Equal(res.Warnings.List(), []string{"unlock failed"}) {
		t.Fatalf("res=%+v err=%v", res, err)
	}
}

// TestRunErrorsNameProduct: every error return is prefixed with the product
// and keeps errors.Is (0003-MADR C10).
func TestRunErrorsNameProduct(t *testing.T) {
	sentinel := errors.New("report failed")
	cases := map[string]func(*contractEnv, *Request){
		"invalid request": func(_ *contractEnv, r *Request) { r.CurrentVersion = "1.0" },
		"report resolving": func(e *contractEnv, _ *Request) {
			e.rep.errAt, e.rep.fail = EventResolvingTarget, sentinel
		},
		"report fetching": func(e *contractEnv, _ *Request) {
			e.rep.errAt, e.rep.fail = EventFetchingRelease, sentinel
		},
		"report selected": func(e *contractEnv, _ *Request) {
			e.rep.errAt, e.rep.fail = EventSelected, sentinel
		},
		"report manifest": func(e *contractEnv, _ *Request) {
			e.rep.errAt, e.rep.fail = EventDownloadingManifest, sentinel
		},
		"report binary": func(e *contractEnv, _ *Request) {
			e.rep.errAt, e.rep.fail = EventDownloadingBinary, sentinel
		},
		"report verified": func(e *contractEnv, _ *Request) {
			e.rep.errAt, e.rep.fail = EventVerified, sentinel
		},
		"report installing": func(e *contractEnv, _ *Request) {
			e.rep.errAt, e.rep.fail = EventInstalling, sentinel
		},
		// A failed report of complete is a warning since v1.6.0, not an
		// error: TestLateErrorIsNotAFailure (0010-MADR Q3).
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			env := newContractEnv(t)
			req := applyReq()
			req.Yes = true
			mutate(env, &req)
			u := env.build(t)
			_, err := u.Run(context.Background(), req)
			if err == nil {
				t.Fatal("no error")
			}
			if !strings.HasPrefix(err.Error(), "selfupdate: demo: ") {
				t.Fatalf("error not prefixed with the product: %q", err.Error())
			}
			if env.rep.fail != nil && !errors.Is(err, sentinel) {
				t.Fatalf("errors.Is lost: %v", err)
			}
		})
	}
}

// TestRunLocalBuildCheckHintsForce: check mode on a local build says apply
// needs --force (0003-MADR C5).
func TestRunLocalBuildCheckHintsForce(t *testing.T) {
	env := newContractEnv(t)
	u := env.build(t)
	req := applyReq()
	req.CurrentBuild, req.CurrentVersion, req.CheckOnly = LocalBuild, "dev", true
	res, err := u.Run(context.Background(), req)
	if !errors.Is(err, ErrUpdateAvailable) || res.Operation != OperationReplaceLocal {
		t.Fatalf("res=%+v err=%v", res, err)
	}
	for _, ev := range env.rep.events {
		if ev.Kind == EventSelected && strings.Contains(ev.Detail, "--force") {
			return
		}
	}
	t.Fatalf("no --force hint in %+v", env.rep.events)
}

// TestRunPendingBackupDetail: the pending-backup detail names the retained
// path, not only its basename (0003-MADR C12).
func TestRunPendingBackupDetail(t *testing.T) {
	env := newContractEnv(t)
	env.inst.pending = ".demo.selfupdate-bak-1"
	u := env.build(t)
	req := applyReq()
	req.Yes = true
	res, err := u.Run(context.Background(), req)
	if err != nil || res.PendingBackup != ".demo.selfupdate-bak-1" {
		t.Fatalf("res=%+v err=%v", res, err)
	}
	want := filepath.Join(env.inst.dir, ".demo.selfupdate-bak-1")
	for _, ev := range env.rep.events {
		if ev.Kind == EventComplete {
			if !strings.Contains(ev.Detail, want) {
				t.Fatalf("detail %q does not name %q", ev.Detail, want)
			}
			return
		}
	}
	t.Fatal("no complete event")
}

// TestRunPendingBackupAbsolutePath: the standalone installer reports an
// absolute path; the detail uses it as is, not joined onto the directory
// again (0003-PLAN deviation D2).
func TestRunPendingBackupAbsolutePath(t *testing.T) {
	env := newContractEnv(t)
	abs := filepath.Join(env.inst.dir, ".demo.selfupdate-bak-2")
	env.inst.pending = abs
	u := env.build(t)
	req := applyReq()
	req.Yes = true
	if _, err := u.Run(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	for _, ev := range env.rep.events {
		if ev.Kind == EventComplete {
			if !strings.Contains(ev.Detail, abs) || strings.Count(ev.Detail, env.inst.dir) != 1 {
				t.Fatalf("detail %q, want %q exactly once", ev.Detail, abs)
			}
			return
		}
	}
	t.Fatal("no complete event")
}

// TestDiscoveryFailureResultNamesRun: a run that fails in discovery still
// returns the product, the running version and what was asked (a check, a
// dry run), as the events and an early failure do
// (docs/decisions/0015-MADR-remediate-third-debugging-pass-findings.md C6).
func TestDiscoveryFailureResultNamesRun(t *testing.T) {
	for _, req := range []Request{
		{Product: "demo", CurrentVersion: "v1.0.0", CurrentBuild: ReleaseBuild, CheckOnly: true},
		{Product: "demo", CurrentVersion: "v1.0.0", CurrentBuild: ReleaseBuild, DryRun: true},
	} {
		env := newContractEnv(t)
		env.src.err = errors.New("fixture: the API is down")
		res, err := env.build(t).Run(context.Background(), req)
		if err == nil {
			t.Fatal("a failed discovery returned no error")
		}
		if res.Product != "demo" || res.CurrentVersion != "v1.0.0" || res.Checked != req.CheckOnly || res.DryRun != req.DryRun {
			t.Fatalf("Product=%q CurrentVersion=%q Checked=%t DryRun=%t; want the run named", res.Product, res.CurrentVersion, res.Checked, res.DryRun)
		}
	}
}
