//go:build unix

package launchd

import (
	"context"
	"os"
	"testing"

	"github.com/maccavelli/go-selfupdate-lib/selfupdate"
)

// The rewrite keeps the plist's owner and mode, which are Unix's; the
// package runs on macOS.

func TestReconcileRewriteAndRestore(t *testing.T) {
	f := newFake()
	j := testJob(t, f, Options{RewritePath: true})
	if err := os.Chmod(j.o.Plist, 0o640); err != nil {
		t.Fatal(err)
	}
	res, err := j.Reconcile(context.Background(), "demo", "/opt/new/demo")
	if err != nil || !res.Changed {
		t.Fatalf("%+v, %v", res, err)
	}
	if got := readString(t, j.o.Plist); got != "replaced ProgramArguments.0 with /opt/new/demo" {
		t.Fatalf("plist %q", got)
	}
	if info, err := os.Stat(j.o.Plist); err != nil || info.Mode().Perm() != 0o640 {
		t.Fatalf("mode %v, %v", info.Mode(), err)
	}
	if err := j.Restore(context.Background(), "demo", res); err != nil {
		t.Fatal(err)
	}
	if got := readString(t, j.o.Plist); got != "<plist/>" {
		t.Fatalf("restored %q", got)
	}
	if err := j.Restore(context.Background(), "demo", selfupdate.ReconcileResult{Changed: true, State: "x"}); err == nil {
		t.Fatal("a foreign receipt was restored")
	}
}
