package selfupdate

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// Tests for docs/decisions/0004-PLAN-v1-1-0-core-api.md Step 3.

type checkRow struct {
	name      string
	mutate    func(*Release)
	req       CheckRequest
	policy    VersionPolicy
	wantOp    Operation
	wantAvail bool
	wantForce bool
	wantErr   error  // matched with errors.Is when set
	wantText  string // matched as a substring when set
}

func checkRows() []checkRow {
	release := CheckRequest{Product: "demo", CurrentVersion: "v1.0.0", CurrentBuild: ReleaseBuild}
	with := func(f func(*CheckRequest)) CheckRequest {
		r := release
		f(&r)
		return r
	}
	return []checkRow{
		{name: "upgrade", req: release, wantOp: OperationUpgrade, wantAvail: true},
		{name: "up to date", req: with(func(r *CheckRequest) { r.CurrentVersion = "v1.1.0" }), wantOp: OperationNone},
		{name: "local build", req: with(func(r *CheckRequest) { r.CurrentVersion = "dev"; r.CurrentBuild = LocalBuild }),
			wantOp: OperationReplaceLocal, wantAvail: true, wantForce: true},
		{name: "exact rollback", req: with(func(r *CheckRequest) { r.CurrentVersion = "v1.2.0"; r.TargetVersion = "v1.1.0" }),
			wantOp: OperationRollback, wantAvail: true},
		{name: "latest older", req: with(func(r *CheckRequest) { r.CurrentVersion = "v2.0.0" }), wantErr: ErrLatestOlder},
		{name: "mutable release", mutate: func(r *Release) { r.Immutable = false }, req: release, wantErr: ErrMutableRelease},
		{name: "prerelease", mutate: func(r *Release) { r.Prerelease = true }, req: release,
			wantText: "not a stable published release"},
		// A custom source may return a draft; only discover refuses it
		// (0010-MADR A13).
		{name: "draft", mutate: func(r *Release) { r.Draft = true }, req: release,
			wantText: "not a stable published release"},
		{name: "draft by tag", mutate: func(r *Release) { r.Draft = true },
			req:      with(func(r *CheckRequest) { r.CurrentVersion = "v1.2.0"; r.TargetVersion = "v1.1.0" }),
			wantText: "not a stable published release"},
		{name: "unsupported platform", req: with(func(r *CheckRequest) { r.Platform = Platform{OS: "plan9", Arch: "amd64"} }),
			wantErr: ErrUnsupportedPlatform},
		{name: "bad digest syntax", mutate: func(r *Release) { r.Assets[0].Digest = "sha256:zz" }, req: release,
			wantErr: ErrIntegrity},
		{name: "configured policy", policy: prereleasePolicy{},
			mutate: func(r *Release) { r.Tag = "v1.2.3-rc.1" },
			req:    with(func(r *CheckRequest) { r.TargetVersion = "v1.2.3-rc.1" }),
			wantOp: OperationUpgrade, wantAvail: true},
	}
}

func checkerFor(t *testing.T, row checkRow) (*Checker, *scriptSource, AssetSelector) {
	t.Helper()
	rel, bodies, plats := fixtureRelease(t, "demo")
	if row.mutate != nil {
		row.mutate(&rel)
	}
	src := &scriptSource{rel: rel, bodies: bodies}
	sel, err := NewExactAssetSelector(plats)
	if err != nil {
		t.Fatal(err)
	}
	policy := row.policy
	if policy == nil {
		policy = NewStrictVersionPolicy()
	}
	c, err := NewChecker(CheckerConfig{Source: src, Versions: policy, Assets: sel, Limits: DefaultLimits()})
	if err != nil {
		t.Fatal(err)
	}
	return c, src, sel
}

func checkOutcome(t *testing.T, row checkRow, err error) {
	t.Helper()
	switch {
	case row.wantErr != nil:
		if !errors.Is(err, row.wantErr) {
			t.Fatalf("err = %v, want %v", err, row.wantErr)
		}
	case row.wantText != "":
		if err == nil || !strings.Contains(err.Error(), row.wantText) {
			t.Fatalf("err = %v, want %q", err, row.wantText)
		}
	}
}

// TestCheckerAvailability: one row per outcome the query API reports.
func TestCheckerAvailability(t *testing.T) {
	for _, row := range checkRows() {
		t.Run(row.name, func(t *testing.T) {
			c, _, _ := checkerFor(t, row)
			got, err := c.Check(context.Background(), row.req)
			if row.wantErr != nil || row.wantText != "" {
				checkOutcome(t, row, err)
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got.Operation != row.wantOp || got.Available != row.wantAvail || got.ForceRequired != row.wantForce {
				t.Fatalf("got %+v, want op=%v available=%v force=%v", got, row.wantOp, row.wantAvail, row.wantForce)
			}
			if got.Product != "demo" || got.CurrentVersion != row.req.CurrentVersion || got.AssetName == "" ||
				got.TargetVersion == "" || got.ReleaseURL == "" {
				t.Fatalf("incomplete availability %+v", got)
			}
		})
	}
}

// TestCheckerAgreesWithRunCheck: Run with CheckOnly and Check reach the same
// selection and the same error class for every row (0004-MADR G3).
func TestCheckerAgreesWithRunCheck(t *testing.T) {
	for _, row := range checkRows() {
		t.Run(row.name, func(t *testing.T) {
			c, src, sel := checkerFor(t, row)
			avail, cerr := c.Check(context.Background(), row.req)
			env := newContractEnv(t)
			policy := row.policy
			if policy == nil {
				policy = NewStrictVersionPolicy()
			}
			u, err := New(Config{Source: src, Versions: policy, Assets: sel, Installer: env.inst,
				Reporter: env.rep, Confirmer: env.conf, Limits: DefaultLimits()})
			if err != nil {
				t.Fatal(err)
			}
			res, rerr := u.Run(context.Background(), Request{
				Product: row.req.Product, CurrentVersion: row.req.CurrentVersion, CurrentBuild: row.req.CurrentBuild,
				TargetVersion: row.req.TargetVersion, Platform: row.req.Platform, CheckOnly: true,
			})
			if cerr != nil {
				if rerr == nil || errors.Is(rerr, ErrUpdateAvailable) {
					t.Fatalf("Check failed (%v) but Run did not (%v)", cerr, rerr)
				}
				for _, s := range []error{ErrLatestOlder, ErrMutableRelease, ErrUnsupportedPlatform, ErrIntegrity} {
					if errors.Is(cerr, s) != errors.Is(rerr, s) {
						t.Fatalf("error classes differ for %v: Check %v, Run %v", s, cerr, rerr)
					}
				}
				return
			}
			if avail.Available != errors.Is(rerr, ErrUpdateAvailable) {
				t.Fatalf("Available=%v but Run returned %v", avail.Available, rerr)
			}
			if avail.TargetVersion != res.TargetVersion || avail.ReleaseURL != res.ReleaseURL ||
				avail.AssetName != res.AssetName || avail.Operation != res.Operation {
				t.Fatalf("Check %+v disagrees with Run %+v", avail, res)
			}
		})
	}
}

// TestCheckerNeedsNoInstaller: a Checker is built from discovery parts
// alone, and works where no target could be resolved.
func TestCheckerNeedsNoInstaller(t *testing.T) {
	setSeam(t, &userHomeDir, func() (string, error) { return "", errors.New("no home") })
	setSeam(t, &osExecutable, func() (string, error) { return "", errors.New("no executable") })
	c, _, _ := checkerFor(t, checkRows()[0])
	got, err := c.Check(context.Background(), checkRows()[0].req)
	if err != nil || !got.Available {
		t.Fatalf("got %+v err=%v", got, err)
	}
	if _, err := NewChecker(CheckerConfig{Versions: NewStrictVersionPolicy(), Limits: DefaultLimits()}); err == nil {
		t.Fatal("NewChecker accepted a missing source")
	}
	var typedNil *scriptSource
	if _, err := NewChecker(CheckerConfig{Source: typedNil, Versions: NewStrictVersionPolicy(),
		Assets: &exactAssetSelector{}, Limits: DefaultLimits()}); err == nil {
		t.Fatal("NewChecker accepted a typed-nil source")
	}
}

// TestCheckerDownloadsNothing: a check never opens an asset body.
func TestCheckerDownloadsNothing(t *testing.T) {
	row := checkRows()[0]
	c, src, _ := checkerFor(t, row)
	if _, err := c.Check(context.Background(), row.req); err != nil {
		t.Fatal(err)
	}
	for _, call := range src.calls {
		if strings.HasPrefix(call, "OpenAsset") {
			t.Fatalf("Check downloaded an asset: %v", src.calls)
		}
	}
}

// TestUpdaterChecker: the Updater's Checker uses the Updater's own parts.
func TestUpdaterChecker(t *testing.T) {
	env := newContractEnv(t)
	u := env.build(t)
	got, err := u.Checker().Check(context.Background(), CheckRequest{Product: "demo", CurrentVersion: "v1.0.0", CurrentBuild: ReleaseBuild})
	if err != nil || !got.Available {
		t.Fatalf("got %+v err=%v", got, err)
	}
	if len(env.src.calls) == 0 || env.src.calls[0] != "Latest" {
		t.Fatalf("the updater's source was not used: %v", env.src.calls)
	}
}
