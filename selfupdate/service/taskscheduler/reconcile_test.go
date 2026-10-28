package taskscheduler

import (
	"context"
	"slices"
	"strings"
	"testing"
	"testing/synctest"

	"github.com/maccavelli/go-selfupdate-lib/selfupdate"
)

// TestReconcileMatch: a task that already runs the executable, by any case,
// is left alone.
func TestReconcileMatch(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		for _, path := range []string{demoExe, `c:\program files\demo\DEMO.EXE`, `"` + demoExe + `"`} {
			f := newFake()
			f.add(`\demo`, 0).acts[0].Path = path
			task := testTask(t, f, Options{RewritePath: true})
			res, err := task.Reconcile(context.Background(), "demo", demoExe)
			if err != nil || res.Changed || len(f.verbs()) != 0 {
				t.Errorf("%s: %+v, %v, calls %q", path, res, err, f.verbs())
			}
		}
	})
}

// TestReconcileMoved: a task running another binary is an error without
// RewritePath, and is rewritten with it: the Path alone changes, and the
// receipt holds the definition as it was.
func TestReconcileMoved(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		const moved = `C:\Users\<user>\AppData\Local\Demo\demo.exe`
		f := newFake()
		f.add(`\demo`, 0)
		task := testTask(t, f, Options{})
		if _, err := task.Reconcile(context.Background(), "demo", moved); err == nil || !strings.Contains(err.Error(), "Options.RewritePath") {
			t.Fatalf("without RewritePath: %v", err)
		}

		task = testTask(t, f, Options{RewritePath: true})
		res, err := task.Reconcile(context.Background(), "demo", moved)
		if err != nil || !res.Changed {
			t.Fatalf("Reconcile: %+v, %v", res, err)
		}
		if want := []string{"setpath " + moved}; !slices.Equal(f.verbs(), want) {
			t.Fatalf("calls %q, want %q", f.verbs(), want)
		}
		want := DefinitionBackup{Folder: `\`, Name: "demo", XML: demoXML}
		if b, ok := res.State.(DefinitionBackup); !ok || b != want {
			t.Fatalf("receipt %#v, want %#v", res.State, want)
		}
		if def := f.tasks[`\demo`].def; def != strings.Replace(demoXML, demoExe, moved, 1) {
			t.Fatalf("definition after the rewrite: %s", def)
		}
	})
}

// TestReconcileQuoted: a quoted Path stays quoted.
func TestReconcileQuoted(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		const moved = `C:\New Place\demo.exe`
		f := newFake()
		f.add(`\demo`, 0).acts[0].Path = `"` + demoExe + `"`
		task := testTask(t, f, Options{RewritePath: true})
		if _, err := task.Reconcile(context.Background(), "demo", moved); err != nil {
			t.Fatal(err)
		}
		if want := []string{`setpath "` + moved + `"`}; !slices.Equal(f.verbs(), want) {
			t.Fatalf("calls %q, want %q", f.verbs(), want)
		}
	})
}

// TestReconcileRefused: anything but exactly one Exec action, another
// logon type than an interactive token (Amendment A1, Q6), and a path
// holding a quote are refused, and nothing is changed.
func TestReconcileRefused(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		const moved = `C:\New\demo.exe`
		for _, c := range []struct {
			name  string
			edit  func(*fakeTask)
			exe   string
			about string
		}{
			{"two actions", func(ft *fakeTask) { ft.acts = append(ft.acts, action{Type: actionExec, Path: `C:\x.exe`}) }, moved, "2 actions"},
			{"no action", func(ft *fakeTask) { ft.acts = nil }, moved, "no action"},
			{"not Exec", func(ft *fakeTask) { ft.acts = []action{{Type: 5}} }, moved, "type 5"},
			{"S4U", func(ft *fakeTask) { ft.logon = logonS4U }, moved, "S4U"},
			{"stored password", func(ft *fakeTask) { ft.logon = logonPassword }, moved, "a stored password"},
			{"quote", func(*fakeTask) {}, `C:\New\de"mo.exe`, "quote"},
		} {
			f := newFake()
			c.edit(f.add(`\demo`, 0))
			task := testTask(t, f, Options{RewritePath: true})
			_, err := task.Reconcile(context.Background(), "demo", c.exe)
			if err == nil || !strings.Contains(err.Error(), c.about) || len(f.verbs()) != 0 {
				t.Errorf("%s: %v, calls %q; want an error about %q and no change", c.name, err, f.verbs(), c.about)
			}
		}
	})
}

// TestRestore: Restore registers exactly the saved definition; a receipt
// that changed nothing is a no-op, and a foreign receipt is refused.
func TestRestore(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx := context.Background()
		f := newFake()
		ft := f.add(`\Vendor\demo`, 0)
		ft.def = "changed"
		task := testTask(t, f, Options{Folder: `\Vendor`})
		receipt := selfupdate.ReconcileResult{Changed: true, State: DefinitionBackup{Folder: `\Vendor`, Name: "demo", XML: demoXML}}
		if err := task.Restore(ctx, "demo", receipt); err != nil {
			t.Fatal(err)
		}
		if ft.def != demoXML || !slices.Equal(f.verbs(), []string{"restore"}) {
			t.Fatalf("definition %q, calls %q", ft.def, f.verbs())
		}
		if err := task.Restore(ctx, "demo", selfupdate.ReconcileResult{}); err != nil || len(f.verbs()) != 1 {
			t.Fatalf("unchanged receipt: %v, calls %q", err, f.verbs())
		}
		if err := task.Restore(ctx, "demo", selfupdate.ReconcileResult{Changed: true, State: "other"}); err == nil {
			t.Fatal("a foreign receipt was accepted")
		}
	})
}
