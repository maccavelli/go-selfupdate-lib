package taskscheduler

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/maccavelli/go-selfupdate-lib/selfupdate"
	"github.com/maccavelli/go-selfupdate-lib/selfupdate/service"
)

// Tests for the one-shot handoff (0032-MADR D5-A to D8-A,
// 0032-PLAN-task-scheduler-registration-and-one-shot-handoff.md J7).

// TestOneShotGolden: the definition oneShotXML renders equals the one Task
// Scheduler stored for the same inputs in 0032-MADR Amendment A1's probe
// (testdata/oneshot.stored.xml, redacted as testdata/definitions/README.md
// says), and composeArgs quotes the arguments as that run received them.
func TestOneShotGolden(t *testing.T) {
	body, err := os.ReadFile(filepath.Join("testdata", "oneshot.stored.xml"))
	if err != nil {
		t.Fatal(err)
	}
	stored := string(body)
	const dir = `C:\Users\user\AppData\Local\Temp\tsprobe`
	args := composeArgs([]string{
		service.ArgHandOffEnv + dir + `\run1\handoff\handoff-probe980f36aa.env`,
		"oneshotrun", dir + `\run1`, "a b", `"q"`, `back\slash\`, "ü 任务",
	})
	got, err := flatten(stored)
	if err != nil {
		t.Fatal(err)
	}
	if want := got["Actions/Exec[1]/Arguments"]; args != want {
		t.Fatalf("composeArgs = %s\nstored     = %s", args, want)
	}
	p := principal{UserID: "S-1-5-21-1111111111-2222222222-3333333333-1001", LogonType: logonInteractiveToken}
	def := oneShotXML(p, "selfupdate handoff", dir+`\tsprobe.exe`, args, `C:\Users\user\AppData\Local\Temp`)
	if d, err := DiffDefinitions(stored, def); err != nil || len(d) != 0 {
		t.Fatalf("stored against rendered: %+v, %v", d, err)
	}
}

func TestEscapeArg(t *testing.T) {
	for in, want := range map[string]string{
		"":             `""`,
		"plain":        "plain",
		"a b":          `"a b"`,
		`"q"`:          `\"q\"`,
		`back\slash\`:  `back\slash\`,
		`a\ b\`:        `"a\ b\\"`,
		`say "hi" now`: `"say \"hi\" now"`,
		`x\"y`:         `x\\\"y`,
		"tab\there":    "\"tab\there\"",
	} {
		if got := escapeArg(in); got != want {
			t.Errorf("escapeArg(%q) = %s, want %s", in, got, want)
		}
	}
}

// oneShotTask is a Task over f with a service task \demo for the user, and
// a handoff directory of its own.
func oneShotTask(t *testing.T, f *fakeSched, o Options) (*Task, string) {
	t.Helper()
	svc := f.add(`\demo`, 0)
	svc.user, svc.runLevel = "user", 1
	dir := filepath.Join(t.TempDir(), "handoff")
	o.HandOffDir = dir
	task := testTask(t, f, o)
	return task, dir
}

var oneShotSpec = service.HandOff{
	ID: "abc123", Executable: `C:\Program Files\Demo\demo.exe`,
	Args: []string{"update", "--yes", "a b"}, Env: []string{"SU_SECRET=hunter2"}, ResultPath: `C:\Program Files\Demo\.demo.exe.selfupdate.handoff`,
}

func TestDetachOneShot(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := newFake()
		task, dir := oneShotTask(t, f, Options{})
		det, err := task.Detach(context.Background(), oneShotSpec)
		if err != nil {
			t.Fatal(err)
		}
		if want := (service.Detached{ID: "abc123", Where: `task \demo.selfupdate.abc123`, ResultPath: oneShotSpec.ResultPath}); det != want {
			t.Fatalf("Detached %+v, want %+v", det, want)
		}
		if v := f.verbs(); !slices.Contains(v, `register \demo.selfupdate.abc123 flags=2 logon=3`) || !slices.Contains(v, "run") {
			t.Fatalf("verbs %q", v)
		}
		ft := f.tasks[`\demo.selfupdate.abc123`]
		if ft == nil {
			t.Fatal("no one-shot task")
		}
		leaves, err := flatten(ft.def)
		if err != nil {
			t.Fatal(err)
		}
		for path, want := range map[string]string{
			principalPath + "UserId":              "user",
			principalPath + "RunLevel":            "HighestAvailable",
			principalPath + "LogonType":           "InteractiveToken",
			"Actions/Exec[1]/Command":             oneShotSpec.Executable,
			"Settings/DisallowStartIfOnBatteries": "false",
			"Settings/StopIfGoingOnBatteries":     "false",
			"Settings/ExecutionTimeLimit":         "PT0S",
		} {
			if leaves[path] != want {
				t.Errorf("%s = %q, want %q", path, leaves[path], want)
			}
		}
		if _, ok := leaves["Triggers"]; ok {
			t.Error("the one-shot task has a trigger")
		}
		args := leaves["Actions/Exec[1]/Arguments"]
		file := filepath.Join(dir, "handoff-abc123.env")
		if want := composeArgs([]string{service.ArgHandOffEnv + file, "update", "--yes", "a b"}); args != want {
			t.Fatalf("Arguments %s, want %s", args, want)
		}
		if strings.Contains(ft.def, "hunter2") {
			t.Fatal("the definition holds an environment value")
		}
		body, err := os.ReadFile(file) //nolint:gosec // the test's own file
		if err != nil {
			t.Fatal(err)
		}
		var vars []string
		if err := json.Unmarshal(body, &vars); err != nil {
			t.Fatal(err)
		}
		for _, want := range []string{"SU_SECRET=hunter2", service.EnvHandOffHop + "=1"} {
			if !slices.Contains(vars, want) {
				t.Errorf("the environment file lacks %s", want)
			}
		}
	})
}

// TestDetachOneShotFailures: a failure after the file is written leaves no
// task and no file.
func TestDetachOneShotFailures(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		never := int32(hasNotRun)
		for _, c := range []struct {
			name string
			edit func(f *fakeSched)
			want error
		}{
			{"register denied", func(f *fakeSched) { f.fail["register"] = hrAccessDenied }, service.ErrPermission},
			{"run denied", func(f *fakeSched) { f.onRegister = func(ft *fakeTask) { ft.runErr = hrAccessDenied } }, service.ErrPermission},
			{"never starts", func(f *fakeSched) { f.onRegister = func(ft *fakeTask) { ft.runExits = &never } }, service.ErrTimeout},
		} {
			f := newFake()
			c.edit(f)
			task, dir := oneShotTask(t, f, Options{})
			start := time.Now()
			if _, err := task.Detach(context.Background(), oneShotSpec); !errors.Is(err, c.want) {
				t.Errorf("%s: %v, want %v", c.name, err, c.want)
			}
			// A run that never starts is given Poll's timeout, the 60 s
			// default, and no more.
			if took := time.Since(start); errors.Is(c.want, service.ErrTimeout) && took != service.DefaultPollTimeout {
				t.Errorf("%s: gave up after %s, want %s", c.name, took, service.DefaultPollTimeout)
			}
			if f.tasks[`\demo.selfupdate.abc123`] != nil {
				t.Errorf("%s: the one-shot task was left", c.name)
			}
			if ents, _ := os.ReadDir(dir); len(ents) != 0 {
				t.Errorf("%s: files were left: %v", c.name, ents)
			}
		}
	})
}

func TestDetachOneShotRefuses(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx := context.Background()
		f := newFake()
		task, dir := oneShotTask(t, f, Options{})
		f.tasks[`\demo`].logon = logonS4U
		if _, err := task.Detach(ctx, oneShotSpec); err == nil || !strings.Contains(err.Error(), "HopDetach") {
			t.Fatalf("S4U: %v, want a refusal naming HopDetach", err)
		}
		f.tasks[`\demo`].logon = logonInteractiveToken
		quoted := oneShotSpec
		quoted.Executable = `C:\a"b.exe`
		if _, err := task.Detach(ctx, quoted); err == nil {
			t.Fatal("a quoted path was registered")
		}
		for _, v := range f.verbs() {
			if strings.HasPrefix(v, "register") {
				t.Fatalf("registered: %q", f.verbs())
			}
		}
		if ents, _ := os.ReadDir(dir); len(ents) != 0 {
			t.Fatalf("files were written: %v", ents)
		}
		task.o.Name = ""
		if _, err := task.Detach(ctx, oneShotSpec); err == nil {
			t.Fatal("Detach without Options.Name")
		}
	})
}

// TestDetachHop: with Options.HopDetach, Detach starts the hop and
// registers nothing; the program here does not exist, so the start fails.
func TestDetachHop(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := newFake()
		task, _ := oneShotTask(t, f, Options{HopDetach: true})
		spec := oneShotSpec
		spec.Executable = filepath.Join(t.TempDir(), "no-such-program")
		_, err := task.Detach(context.Background(), spec)
		if err == nil || !strings.Contains(err.Error(), "start the detached run") {
			t.Fatalf("Detach = %v, want the hop's start error", err)
		}
		if v := f.verbs(); len(v) != 0 {
			t.Fatalf("Task Scheduler was called: %q", v)
		}
	})
}

func TestCleanupHandOffs(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx := context.Background()
		f := newFake()
		task, dir := oneShotTask(t, f, Options{})
		f.add(`\demo.selfupdate.done`, 0)
		f.add(`\demo.selfupdate.run`, 4242)
		f.add(`\demo.selfupdate.new`, 0).last.Result = hasNotRun
		f.add(`\other.selfupdate.x`, 0)
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
		for _, id := range []string{"done", "run"} {
			if err := os.WriteFile(filepath.Join(dir, "handoff-"+id+".env"), []byte("[]"), 0o600); err != nil {
				t.Fatal(err)
			}
		}
		if err := task.CleanupHandOffs(ctx); err != nil {
			t.Fatal(err)
		}
		for path, kept := range map[string]bool{
			`\demo`: true, `\demo.selfupdate.done`: false, `\demo.selfupdate.run`: true,
			`\demo.selfupdate.new`: true, `\other.selfupdate.x`: true,
		} {
			if (f.tasks[path] != nil) != kept {
				t.Errorf("%s kept %t, want %t", path, f.tasks[path] != nil, kept)
			}
		}
		if _, err := os.Stat(filepath.Join(dir, "handoff-done.env")); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("the finished task's file: %v", err)
		}
		if _, err := os.Stat(filepath.Join(dir, "handoff-run.env")); err != nil {
			t.Errorf("the running task's file: %v", err)
		}

		f.add(`\demo.selfupdate.again`, 0)
		f.fail["delete"] = hrAccessDenied
		if err := task.CleanupHandOffs(ctx); !errors.Is(err, service.ErrPermission) {
			t.Fatalf("delete denied: %v, want ErrPermission", err)
		}
		missing := testTask(t, newFake(), Options{Folder: `\Missing`, HandOffDir: dir})
		if err := missing.CleanupHandOffs(ctx); err != nil {
			t.Fatalf("a missing folder: %v", err)
		}
	})
}

// TestDetachOneShotScrubsManagerEnv: the environment file leaves out the
// manager's per-unit variables, keeps the rest and one spec.Env names, and
// ends with the hop variable (0033-MADR D3-A, D5-A).
func TestDetachOneShotScrubsManagerEnv(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		t.Setenv("NOTIFY_SOCKET", "/run/caller/notify")
		t.Setenv("INVOCATION_ID", "caller0000")
		t.Setenv("RUNTIME_DIRECTORY", "/run/caller")
		t.Setenv("STATE_DIRECTORY", "/var/lib/demo")
		f := newFake()
		task, dir := oneShotTask(t, f, Options{})
		spec := oneShotSpec
		spec.Env = append(slices.Clone(spec.Env), "NOTIFY_SOCKET=/run/explicit")
		if _, err := task.Detach(context.Background(), spec); err != nil {
			t.Fatal(err)
		}
		body, err := os.ReadFile(filepath.Join(dir, "handoff-abc123.env")) //nolint:gosec // the test's own file
		if err != nil {
			t.Fatal(err)
		}
		var vars []string
		if err := json.Unmarshal(body, &vars); err != nil {
			t.Fatal(err)
		}
		for _, want := range []string{"STATE_DIRECTORY=/var/lib/demo", "NOTIFY_SOCKET=/run/explicit"} {
			if !slices.Contains(vars, want) {
				t.Errorf("the environment file lacks %s: %q", want, vars)
			}
		}
		for _, kv := range vars {
			for _, gone := range []string{"INVOCATION_ID=", "RUNTIME_DIRECTORY=", "NOTIFY_SOCKET=/run/caller"} {
				if strings.HasPrefix(kv, gone) {
					t.Errorf("the environment file holds the caller's %s", kv)
				}
			}
		}
		if last := vars[len(vars)-1]; last != service.EnvHandOffHop+"=1" {
			t.Errorf("the last variable is %s, want the hop variable", last)
		}
	})
}

// carriedPlan is a plan whose document holds quotes, a dollar sign and an
// ampersand, as a carrier must keep them
// (docs/decisions/0038-PLAN-pinned-plans-lifecycle-conformance-and-bubble-tests.md
// Deviation D15).
func carriedPlan() selfupdate.Plan {
	digest := strings.Repeat("ab", 32)
	return selfupdate.Plan{
		Product: "demo", CurrentVersion: "v1.0.0", CurrentBuild: selfupdate.ReleaseBuild,
		Platform: selfupdate.Platform{OS: "linux", Arch: "amd64"}, TargetVersion: "v1.1.0", ReleaseID: 7,
		ReleaseURL: `https://example.invalid/r?a="b"&c=$d\e`, Latest: true, Operation: selfupdate.OperationUpgrade,
		Selection: selfupdate.Selection{
			Binary:       selfupdate.Asset{ID: 8, Name: "demo-linux-amd64", State: selfupdate.AssetStateUploaded, Size: 12, Digest: "sha256:" + digest},
			Manifest:     selfupdate.Asset{ID: 9, Name: "SHA256SUMS", State: selfupdate.AssetStateUploaded, Size: 83},
			ManifestName: "demo-linux-amd64",
		},
		ManifestSHA256: digest,
	}
}

// planEnv is the HandOff.Env entry service.HandOffPlanFunc adds for p.
func planEnv(t *testing.T, p selfupdate.Plan) string {
	t.Helper()
	doc, err := json.Marshal(p.Document())
	if err != nil {
		t.Fatal(err)
	}
	return service.EnvHandOffPlan + "=" + string(doc)
}

// planFrom finds the plan's entry in vars and parses it.
func planFrom(t *testing.T, vars []string) selfupdate.Plan {
	t.Helper()
	for _, kv := range vars {
		if doc, ok := strings.CutPrefix(kv, service.EnvHandOffPlan+"="); ok {
			p, err := selfupdate.ParsePlan([]byte(doc))
			if err != nil {
				t.Fatalf("the carried plan: %v", err)
			}
			return p
		}
	}
	t.Fatalf("no %s among %d variables", service.EnvHandOffPlan, len(vars))
	return selfupdate.Plan{}
}

// TestDetachOneShotCarriesPlan: the plan's entry reaches the private
// environment file, not the task's definition, and reads back as the same
// plan.
func TestDetachOneShotCarriesPlan(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := newFake()
		task, dir := oneShotTask(t, f, Options{})
		spec := oneShotSpec
		p := carriedPlan()
		spec.Env = append(slices.Clone(spec.Env), planEnv(t, p))
		if _, err := task.Detach(context.Background(), spec); err != nil {
			t.Fatal(err)
		}
		if ft := f.tasks[`\demo.selfupdate.abc123`]; ft == nil || strings.Contains(ft.def, "SHA256SUMS") {
			t.Fatal("no one-shot task, or the plan reached its definition")
		}
		body, err := os.ReadFile(filepath.Join(dir, "handoff-abc123.env")) //nolint:gosec // the test's own file
		if err != nil {
			t.Fatal(err)
		}
		var vars []string
		if err := json.Unmarshal(body, &vars); err != nil {
			t.Fatal(err)
		}
		if got := planFrom(t, vars); got != p {
			t.Fatalf("the environment file carries %+v, want %+v", got, p)
		}
	})
}
