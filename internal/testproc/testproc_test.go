package testproc

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"
)

// helperEnv makes the test binary act as a helper instead of running
// tests: "ready" writes TESTPROC_READY after 300 ms and waits for
// TESTPROC_DONE; "exit3" prints a line and exits 3; "never" sleeps and
// never writes its ready file.
const helperEnv = "TESTPROC_HELPER"

func TestMain(m *testing.M) {
	switch os.Getenv(helperEnv) {
	case "":
		os.Exit(m.Run())
	case "ready":
		time.Sleep(300 * time.Millisecond)
		fmt.Println("helper: ready")
		if err := os.WriteFile(os.Getenv("TESTPROC_READY"), nil, 0o600); err != nil {
			os.Exit(2)
		}
		for deadline := time.Now().Add(10 * time.Second); time.Now().Before(deadline); time.Sleep(10 * time.Millisecond) {
			if _, err := os.Stat(os.Getenv("TESTPROC_DONE")); err == nil {
				os.Exit(0)
			}
		}
		os.Exit(4)
	case "exit3":
		fmt.Println("helper: boom")
		os.Exit(3)
	case "never":
		time.Sleep(5 * time.Second)
		os.Exit(0)
	}
	os.Exit(2)
}

// helper returns a command that runs this test binary in mode, and its
// ready and done paths.
func helper(t *testing.T, mode string) (*exec.Cmd, string, string) {
	t.Helper()
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	ready, done := filepath.Join(dir, "ready"), filepath.Join(dir, "done")
	cmd := exec.Command(self) //nolint:gosec // this test binary, as a helper
	cmd.Env = append(os.Environ(), helperEnv+"="+mode, "TESTPROC_READY="+ready, "TESTPROC_DONE="+done)
	return cmd, ready, done
}

// TestStartReady: a helper that is ready after 300 ms is started, and
// Wait returns its exit.
func TestStartReady(t *testing.T) {
	cmd, ready, done := helper(t, "ready")
	p := Start(t, cmd, ready)
	if err := os.WriteFile(done, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := p.Wait(); err != nil {
		t.Fatalf("Wait: %v", err)
	}
	if err := p.Wait(); err != nil {
		t.Fatalf("a second Wait: %v", err)
	}
}

// TestStartExitsFirst: a helper that exits before it is ready is reported
// at once, with its exit status and its output, not at the end of the
// budget (0023-MADR D2-A).
func TestStartExitsFirst(t *testing.T) {
	cmd, ready, _ := helper(t, "exit3")
	began := time.Now()
	_, err := startWithin(cmd, ready, 30*time.Second)
	took := time.Since(began)
	if err == nil || !strings.Contains(err.Error(), "exit status 3") || !strings.Contains(err.Error(), "helper: boom") {
		t.Fatalf("start = %v, want the exit status and the output", err)
	}
	if took > 2*time.Second {
		t.Fatalf("the exit was reported after %s, want at once", took)
	}
}

// TestStartBudget: a helper that never gets ready fails when the budget
// runs out, and is ended.
func TestStartBudget(t *testing.T) {
	cmd, ready, _ := helper(t, "never")
	began := time.Now()
	_, err := startWithin(cmd, ready, 200*time.Millisecond)
	if err == nil || !strings.Contains(err.Error(), "not ready after") {
		t.Fatalf("start = %v, want not ready", err)
	}
	if took := time.Since(began); took > 3*time.Second {
		t.Fatalf("the budget ran %s, want about 200 ms", took)
	}
	if cmd.ProcessState == nil {
		t.Fatal("the helper was not ended")
	}
}

// TestBudget: the wait runs to the test's deadline, less 5 % of the time
// left and at least 1 s; without a deadline, 5 minutes.
func TestBudget(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	for _, c := range []struct {
		left time.Duration
		set  bool
		want time.Duration
	}{
		{100 * time.Second, true, 95 * time.Second},
		{10 * time.Second, true, 9 * time.Second},
		{500 * time.Millisecond, true, 0},
		{0, false, 5 * time.Minute},
	} {
		if got := budgetAt(now.Add(c.left), c.set, now); got != c.want {
			t.Errorf("budgetAt(%s left, deadline %t) = %s, want %s", c.left, c.set, got, c.want)
		}
	}
	if b := Budget(t); b <= 0 {
		t.Errorf("Budget(t) = %s, want a positive budget", b)
	}
}

// TestGoBuildIgnoresParentGit: a fixture builds in a directory whose
// parent holds a .git that git cannot read, and runs (0023-MADR D1-A).
func TestGoBuildIgnoresParentGit(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, ".git"), 0o700); err != nil {
		t.Fatal(err)
	}
	mod := filepath.Join(root, "fixture")
	if err := os.Mkdir(mod, 0o700); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		"go.mod":  "module fixture\n\ngo 1.27\n",
		"main.go": "package main\n\nfunc main() { println(\"fixture\") }\n",
	}
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(mod, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	out := filepath.Join(root, "fixture.bin")
	if runtime.GOOS == "windows" {
		out += ".exe"
	}
	GoBuild(t, mod, out, []string{"CGO_ENABLED=0", "GOWORK=off", "GOFLAGS="})
	got, err := exec.Command(out).CombinedOutput() //nolint:gosec // the fixture just built
	if err != nil || !strings.Contains(string(got), "fixture") {
		t.Fatalf("the fixture: %v, %q", err, got)
	}
}

// TestNoDirectFixtureBuild: no _test.go file in the module runs
// "go build" itself; each builds its fixture through GoBuild or Build,
// which pass -buildvcs=false (0023-MADR D1-A). A call counts when its
// program is the literal "go", or a variable assigned from
// exec.LookPath("go") in the same file, and an argument is the literal
// "build".
func TestNoDirectFixtureBuild(t *testing.T) {
	root := filepath.Join("..", "..")
	if _, err := os.Stat(filepath.Join(root, "go.mod")); err != nil {
		t.Fatalf("no module root at %s: run the test from its package directory: %v", root, err)
	}
	var found []string
	files := 0
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if name := d.Name(); name == "testdata" || name == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, "_test.go") {
			return nil
		}
		files++
		calls, err := directBuilds(path)
		found = append(found, calls...)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if files == 0 {
		t.Fatal("no _test.go file found: the test reads the module's source")
	}
	sort.Strings(found)
	for _, f := range found {
		t.Errorf("%s: build the fixture with testproc.GoBuild or testproc.Build (0023-MADR D1-A)", f)
	}
}

// directBuilds returns "file:line" for each direct go build in path.
func directBuilds(path string) ([]string, error) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
	if err != nil {
		return nil, err
	}
	execName := ""
	for _, imp := range f.Imports {
		if p, _ := strconv.Unquote(imp.Path.Value); p == "os/exec" {
			execName = "exec"
			if imp.Name != nil {
				execName = imp.Name.Name
			}
		}
	}
	if execName == "" {
		return nil, nil
	}
	isExec := func(e ast.Expr, names ...string) bool {
		sel, ok := e.(*ast.SelectorExpr)
		if !ok {
			return false
		}
		id, ok := sel.X.(*ast.Ident)
		if !ok || id.Name != execName {
			return false
		}
		for _, n := range names {
			if sel.Sel.Name == n {
				return true
			}
		}
		return false
	}
	isLit := func(e ast.Expr, s string) bool {
		lit, ok := e.(*ast.BasicLit)
		if !ok || lit.Kind != token.STRING {
			return false
		}
		v, err := strconv.Unquote(lit.Value)
		return err == nil && v == s
	}
	goVars := map[string]bool{}
	ast.Inspect(f, func(n ast.Node) bool {
		as, ok := n.(*ast.AssignStmt)
		if !ok || len(as.Rhs) != 1 {
			return true
		}
		call, ok := as.Rhs[0].(*ast.CallExpr)
		if !ok || !isExec(call.Fun, "LookPath") || len(call.Args) != 1 || !isLit(call.Args[0], "go") {
			return true
		}
		if id, ok := as.Lhs[0].(*ast.Ident); ok {
			goVars[id.Name] = true
		}
		return true
	})
	var found []string
	ast.Inspect(f, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok || !isExec(call.Fun, "Command", "CommandContext") {
			return true
		}
		args := call.Args
		if isExec(call.Fun, "CommandContext") && len(args) > 0 {
			args = args[1:]
		}
		if len(args) < 2 {
			return true
		}
		prog, ok := args[0].(*ast.Ident)
		if !isLit(args[0], "go") && (!ok || !goVars[prog.Name]) {
			return true
		}
		for _, a := range args[1:] {
			if isLit(a, "build") {
				found = append(found, fset.Position(call.Pos()).String())
				break
			}
		}
		return true
	})
	return found, nil
}
