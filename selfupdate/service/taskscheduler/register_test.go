package taskscheduler

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"testing/synctest"

	"github.com/maccavelli/go-selfupdate-lib/selfupdate/service"
)

// Tests for Register, Definition and Delete (0032-MADR D2-A,
// 0032-PLAN-task-scheduler-registration-and-one-shot-handoff.md J3).

const regXML = `<?xml version="1.0" encoding="UTF-8"?>
<Task version="1.2" xmlns="http://schemas.microsoft.com/windows/2004/02/mit/task">
  <Principals><Principal id="Author"><LogonType>InteractiveToken</LogonType></Principal></Principals>
  <Actions Context="Author"><Exec><Command>C:\Program Files\Demo\demo.exe</Command></Exec></Actions>
</Task>`

func withPrincipal(inner string) string {
	return `<Task xmlns="http://schemas.microsoft.com/windows/2004/02/mit/task"><Principals><Principal id="Author">` +
		inner + `</Principal></Principals><Actions><Exec><Command>x.exe</Command></Exec></Actions></Task>`
}

func TestRegisterCreatesAndUpdates(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := newFake()
		task := testTask(t, f, Options{})
		ctx := context.Background()
		if err := task.Register(ctx, "demo", regXML); err != nil {
			t.Fatal(err)
		}
		ft := f.tasks[`\demo`]
		if ft == nil || ft.logon != logonInteractiveToken {
			t.Fatalf("registered %+v", ft)
		}
		if strings.Contains(ft.def, "<?xml") || !strings.HasPrefix(ft.def, "<Task") {
			t.Fatalf("the declaration was not removed: %q", ft.def[:40])
		}
		if v := f.verbs(); !slices.Contains(v, `register \demo flags=6 logon=3`) {
			t.Fatalf("verbs %q", v)
		}
		updated := strings.Replace(regXML, "demo.exe", "demo2.exe", 1)
		if err := task.Register(ctx, "demo", updated); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(f.tasks[`\demo`].def, "demo2.exe") {
			t.Fatal("the definition was not replaced")
		}
		if got, err := task.Definition(ctx, "demo"); err != nil || !strings.Contains(got, "demo2.exe") {
			t.Fatalf("Definition = %q, %v", got, err)
		}
	})
}

func TestRegisterCreatesTheFolder(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := newFake()
		task := testTask(t, f, Options{Folder: `\Vendor\Product`})
		if err := task.Register(context.Background(), "demo", regXML); err != nil {
			t.Fatal(err)
		}
		want := []string{`register \Vendor\Product\demo flags=6 logon=3`, `mkfolder \Vendor\Product`, `register \Vendor\Product\demo flags=6 logon=3`}
		if v := f.verbs(); !slices.Equal(v, want) {
			t.Fatalf("verbs %q, want %q", v, want)
		}
		if !f.folders[`\Vendor`] || f.tasks[`\Vendor\Product\demo`] == nil {
			t.Fatalf("folders %v, tasks %v", f.folders, f.tasks)
		}
	})
}

func TestRegisterErrors(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx := context.Background()
		f := newFake()
		f.fail["register"] = hrAccessDenied
		task := testTask(t, f, Options{})
		if err := task.Register(ctx, "demo", regXML); !errors.Is(err, service.ErrPermission) {
			t.Fatalf("denied: %v, want ErrPermission", err)
		}

		f = newFake()
		f.fail["mkfolder"] = hrAccessDenied
		task = testTask(t, f, Options{Folder: `\Vendor`})
		if err := task.Register(ctx, "demo", regXML); !errors.Is(err, service.ErrPermission) {
			t.Fatalf("folder denied: %v, want ErrPermission", err)
		}

		cancelled, cancel := context.WithCancel(ctx)
		cancel()
		f = newFake()
		task = testTask(t, f, Options{})
		if err := task.Register(cancelled, "demo", regXML); !errors.Is(err, context.Canceled) || len(f.verbs()) != 0 {
			t.Fatalf("cancelled: %v, verbs %q", err, f.verbs())
		}
	})
}

// TestRegisterRefuses: a principal a user cannot register for themselves
// without credentials is refused before Task Scheduler is called
// (0032-MADR Amendment A1, P1-2).
func TestRegisterRefuses(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		for name, body := range map[string]string{
			"S4U":                        withPrincipal(`<LogonType>S4U</LogonType>`),
			"Password":                   withPrincipal(`<LogonType>Password</LogonType>`),
			"InteractiveTokenOrPassword": withPrincipal(`<LogonType>InteractiveTokenOrPassword</LogonType>`),
			"a group":                    withPrincipal(`<GroupId>S-1-5-32-545</GroupId>`),
			"SYSTEM by SID":              withPrincipal(`<UserId>S-1-5-18</UserId>`),
			"LocalService by name":       withPrincipal(`<UserId>NT AUTHORITY\LOCAL SERVICE</UserId>`),
			"not XML":                    `<Task><Principals>`,
		} {
			f := newFake()
			task := testTask(t, f, Options{})
			if err := task.Register(context.Background(), "demo", body); err == nil {
				t.Errorf("%s: registered", name)
			}
			if v := f.verbs(); len(v) != 0 {
				t.Errorf("%s: Task Scheduler was called: %q", name, v)
			}
		}
		f := newFake()
		task := testTask(t, f, Options{})
		if err := task.Register(context.Background(), "demo", withPrincipal(`<UserId>user</UserId>`)); err != nil {
			t.Fatalf("an InteractiveToken principal with no logon type: %v", err)
		}
	})
}

func TestDefinitionAndDelete(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx := context.Background()
		f := newFake()
		task := testTask(t, f, Options{})
		if _, err := task.Definition(ctx, "demo"); !errors.Is(err, service.ErrNotInstalled) {
			t.Fatalf("missing: %v, want ErrNotInstalled", err)
		}
		f.add(`\demo`, 0)
		if got, err := task.Definition(ctx, "demo"); err != nil || got != demoXML {
			t.Fatalf("Definition = %q, %v", got, err)
		}
		for i := range 2 {
			if err := task.Delete(ctx, "demo"); err != nil {
				t.Fatalf("delete %d: %v", i+1, err)
			}
		}
		if f.tasks[`\demo`] != nil {
			t.Fatal("not deleted")
		}
		f.fail["delete"] = hrAccessDenied
		if err := task.Delete(ctx, "demo"); !errors.Is(err, service.ErrPermission) {
			t.Fatalf("denied: %v, want ErrPermission", err)
		}
		task.o.Name = ""
		if err := task.Delete(ctx, `a\b`); err == nil {
			t.Fatal("an invalid name was deleted")
		}
	})
}

func TestStripDeclaration(t *testing.T) {
	for in, want := range map[string]string{
		`<?xml version="1.0" encoding="UTF-16"?>` + "\r\n<Task/>": "<Task/>",
		"\ufeff  <?xml version=\"1.0\"?><Task/>":                  "<Task/>",
		"<Task/>":                                                 "<Task/>",
		"<?xml unterminated":                                      "<?xml unterminated",
	} {
		if got := stripDeclaration(in); got != want {
			t.Errorf("stripDeclaration(%q) = %q, want %q", in, got, want)
		}
	}
}
