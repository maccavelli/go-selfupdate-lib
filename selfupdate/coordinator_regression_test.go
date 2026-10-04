package selfupdate

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// Regression tests for docs/decisions/0010-MADR-remediate-second-debugging-pass-findings.md
// C6 (docs/decisions/0010-PLAN-v1-5-1-contract-preserving-fixes.md P6).

// sameBaseSession stages under the target's own basename, in another
// directory, and hides any Owns: it is not a StagingOwner.
type sameBaseSession struct {
	InstallSession
	dir string
}

func (s sameBaseSession) CreateStaging(context.Context) (*os.File, string, error) {
	path := filepath.Join(s.dir, s.Target().Base)
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0o600)
	return f, path, err
}

type sameBaseInstaller struct {
	*logInstaller
	dir string
}

func (i sameBaseInstaller) Begin(ctx context.Context, t Target) (InstallSession, error) {
	sess, err := i.logInstaller.Begin(ctx, t)
	if err != nil {
		return nil, err
	}
	return sameBaseSession{InstallSession: sess, dir: i.dir}, nil
}

// TestTransformSameBasenameRequiresStagingOwner: a session that is not a
// StagingOwner fails closed with a Transformer even when its staging has
// the target's basename (0010-MADR C6, 0004-MADR G7).
func TestTransformSameBasenameRequiresStagingOwner(t *testing.T) {
	env := newContractEnv(t)
	_, _, plats := fixtureRelease(t, "demo")
	sel, err := NewExactAssetSelector(plats)
	if err != nil {
		t.Fatal(err)
	}
	u, err := New(Config{
		Source: env.src, Versions: NewStrictVersionPolicy(), Assets: sel,
		Transformer: growTransformer{extra: []byte("-signed")},
		Installer:   sameBaseInstaller{logInstaller: env.inst, dir: t.TempDir()},
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
