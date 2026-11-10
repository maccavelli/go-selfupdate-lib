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
	"regexp"
	"runtime"
	"slices"
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

// recordingTB records what CheckNoLeak reports instead of failing the
// test that runs it.
type recordingTB struct {
	testing.TB
	errs []string
}

func (r *recordingTB) Helper()        {}
func (r *recordingTB) Cleanup(func()) {}
func (r *recordingTB) Errorf(format string, args ...any) {
	r.errs = append(r.errs, fmt.Sprintf(format, args...))
}

// TestCheckNoLeakSeesLeak: a goroutine still blocked when the check runs
// is reported, also when a goroutine that ran before the check began ends
// during the test, and a test that leaks nothing is not (0025-MADR D4,
// Amendment A6).
func TestCheckNoLeakSeesLeak(t *testing.T) {
	leaky := &recordingTB{TB: t}
	check := CheckNoLeak(leaky)
	stop := make(chan struct{})
	go func() { <-stop }()
	check()
	close(stop)
	if len(leaky.errs) != 1 || !strings.Contains(leaky.errs[0], "goroutines:") {
		t.Fatalf("a blocked goroutine reported %q, want one goroutines: error", leaky.errs)
	}

	release := make(chan struct{})
	go func() { <-release }()
	masked := &recordingTB{TB: t}
	check = CheckNoLeak(masked)
	hold := make(chan struct{})
	go func() { <-hold }()
	n := runtime.NumGoroutine()
	close(release)
	for deadline := time.Now().Add(Budget(t)); runtime.NumGoroutine() >= n; time.Sleep(10 * time.Millisecond) {
		if time.Now().After(deadline) {
			t.Fatal("the goroutine that ran before the check did not end")
		}
	}
	check()
	close(hold)
	if len(masked.errs) != 1 || !strings.Contains(masked.errs[0], "goroutines: 1 started") {
		t.Fatalf("a leak beside an ending goroutine reported %q, want one goroutines: error", masked.errs)
	}

	clean := &recordingTB{TB: t}
	CheckNoLeak(clean)()
	if len(clean.errs) != 0 {
		t.Fatalf("no leak reported %q", clean.errs)
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
		"go.mod":  "module fixture\n\ngo 1.26.10\n",
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

// goIdentifier matches a Go identifier in a message, pkg.Name or
// Type.Field; recordCitation matches a record number
// (docs/decisions/0042-MADR-keep-go-identifiers-out-of-run-time-messages.md
// F1).
var (
	goIdentifier   = regexp.MustCompile(`\b[A-Za-z][A-Za-z0-9]*\.[A-Z][A-Za-z0-9]*\b`)
	recordCitation = regexp.MustCompile(`\b[0-9]{4}-(MADR|PLAN|REPORT|GATES)\b`)
)

// allowedIdentifiers are the configuration messages, which the program's
// author meets on its first run, and which may name the option at fault,
// by the file's slash path (0042-MADR D1-A, F1).
var allowedIdentifiers = map[string][]string{
	// cli.Run's checks of its Options.
	"selfupdate/cli/run.go": {"Options.Stderr", "Options.Stdout", "Options.Timeout"},
	// NewGitHubSource with no client.
	"selfupdate/github.go": {"selfupdate.DefaultHTTPClient"},
	// Updater.Plan asked for a check.
	"selfupdate/plan.go": {"Checker.Check"},
	// launchd.New's Options.
	"selfupdate/service/launchd/job.go": {"Options.Domain"},
	// PollOptions' validation.
	"selfupdate/service/poll.go": {"Poll.Settle", "Poll.Timeout"},
	// The handoff with no service name or unit.
	"selfupdate/service/scm/detach.go":            {"Options.Name"},
	"selfupdate/service/systemd/detach.go":        {"Options.Unit"},
	"selfupdate/service/taskscheduler/detach.go":  {"Options.Name"},
	"selfupdate/service/taskscheduler/handoff.go": {"Options.Name"},
	// NewSupervised's SupervisedOptions.
	"selfupdate/service/supervised.go": {
		"SupervisedOptions.State", "SupervisedOptions.Up", "SupervisedOptions.Down",
		"SupervisedOptions.KillAfter", "Poll.Timeout",
	},
	// The unit's name, from Options.Unit or the program's product.
	"selfupdate/service/systemd/unit.go": {"Options.Unit"},
	// taskscheduler.New's Options.
	"selfupdate/service/taskscheduler/task.go": {"Options.StopGrace", "Options.HandOffDir", "Options.Poll"},
}

// TestRunTimeMessagesNameNoGoIdentifier: no fmt.Errorf or errors.New in the
// module's non-test code names a Go identifier, except the configuration
// messages allowedIdentifiers lists, and none cites a record. A run-time
// message reaches a person who cannot change the program's configuration,
// such as a CLI's user, so it states the fact; the remedy for the program's
// author is in godoc (0042-MADR D1-A, D4-A). Release tools, test support
// and test data are not scanned.
func TestRunTimeMessagesNameNoGoIdentifier(t *testing.T) {
	root := filepath.Join("..", "..")
	if _, err := os.Stat(filepath.Join(root, "go.mod")); err != nil {
		t.Fatalf("no module root at %s: run the test from its package directory: %v", root, err)
	}
	seen := map[string]bool{}
	var found []string
	calls := 0
	files, err := walkScanned(root, nil, func(rel, path string) error {
		msgs, err := errorMessages(path)
		if err != nil {
			return err
		}
		for _, m := range msgs {
			calls++
			for _, c := range recordCitation.FindAllString(m.text, -1) {
				found = append(found, fmt.Sprintf("%s:%d: cites %s: %q", rel, m.line, c, m.text))
			}
			for _, id := range goIdentifier.FindAllString(m.text, -1) {
				if slices.Contains(allowedIdentifiers[rel], id) {
					seen[rel+" "+id] = true
					continue
				}
				found = append(found, fmt.Sprintf("%s:%d: names %s: %q", rel, m.line, id, m.text))
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if files < 100 || calls < 500 {
		t.Fatalf("read %d files and %d calls; the module has more: is the root wrong?", files, calls)
	}
	for _, f := range found {
		t.Errorf("%s: a run-time message names no Go identifier and cites no record; state the fact, and put the remedy in godoc (0042-MADR D1-A)", f)
	}
	for rel, ids := range allowedIdentifiers {
		for _, id := range ids {
			if !seen[rel+" "+id] {
				t.Errorf("%s: allowedIdentifiers lists %s, which no message there names: remove the entry", rel, id)
			}
		}
	}
}

// scanSkip are the trees the source scans do not read: release tools, and
// test support, whose messages a workflow's author or a program's tests
// read (0042-MADR More Information).
var scanSkip = []string{"internal/cmd/", "internal/testproc/", "selfupdate/selfupdatetest/", "selfupdate/service/servicetest/"}

// walkScanned calls fn for each non-test .go file under root that a source
// scan reads: outside .git, testdata, scanSkip and extraSkip. It returns
// the number of files it called fn for.
func walkScanned(root string, extraSkip []string, fn func(rel, path string) error) (int, error) {
	skip := slices.Concat(scanSkip, extraSkip)
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
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if !strings.HasSuffix(rel, ".go") || strings.HasSuffix(rel, "_test.go") {
			return nil
		}
		for _, p := range skip {
			if strings.HasPrefix(rel, p) {
				return nil
			}
		}
		files++
		return fn(rel, path)
	})
	return files, err
}

// TestLiteralPrefixWrapsUseWrap: no fmt.Errorf in the scanned code wraps
// an error behind a literal prefix, a format that begins "selfupdate: ",
// ends ": %w" and has no other verb. Such a wrap uses errprefix.Wrap,
// which drops the wrapped error's own leading "selfupdate: ", so the line
// names the package once
// (docs/decisions/0043-MADR-name-the-package-once-in-wrapped-errors.md
// D1-A, D6-A). internal/procfs wraps only os errors from /proc, which
// carry no prefix, and keeps its standard-library-only rule (0043-MADR
// D5-A; 0035-MADR D13-C), so it is not read.
func TestLiteralPrefixWrapsUseWrap(t *testing.T) {
	root := filepath.Join("..", "..")
	if _, err := os.Stat(filepath.Join(root, "go.mod")); err != nil {
		t.Fatalf("no module root at %s: run the test from its package directory: %v", root, err)
	}
	var found []string
	wraps := 0
	files, err := walkScanned(root, []string{"internal/procfs/"}, func(rel, path string) error {
		fset := token.NewFileSet()
		f, err := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
		if err != nil {
			return err
		}
		ast.Inspect(f, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok || len(call.Args) == 0 {
				return true
			}
			switch {
			case isCall(call, "errprefix", "Wrap"):
				wraps++
			case isCall(call, "fmt", "Errorf"):
				lit, ok := call.Args[0].(*ast.BasicLit)
				if !ok || lit.Kind != token.STRING {
					return true
				}
				format, err := strconv.Unquote(lit.Value)
				if err == nil && literalPrefixWrap(format) {
					found = append(found, fmt.Sprintf("%s:%d: %q", rel, fset.Position(call.Pos()).Line, format))
				}
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range found {
		t.Errorf("%s: a wrap behind a literal prefix uses errprefix.Wrap, so the line names the package once (0043-MADR D6-A)", f)
	}
	if files < 100 || wraps < 130 {
		t.Errorf("read %d files and %d errprefix.Wrap calls; the module has more: is the root wrong?", files, wraps)
	}
}

// literalPrefixWrap reports whether format is a literal prefix before one
// wrapped error: it begins "selfupdate: ", ends ": %w", and its only verb
// is that %w.
func literalPrefixWrap(format string) bool {
	return strings.HasPrefix(format, "selfupdate: ") && strings.HasSuffix(format, ": %w") && strings.Count(format, "%") == 1
}

// isCall reports whether call's function is the selector pkg.name.
func isCall(call *ast.CallExpr, pkg, name string) bool {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	id, ok := sel.X.(*ast.Ident)
	return ok && id.Name == pkg && sel.Sel.Name == name
}

// message is the joined string literals of one fmt.Errorf or errors.New
// call, and its line.
type message struct {
	text string
	line int
}

// errorMessages returns the text of each fmt.Errorf and errors.New call in
// path, and of each errprefix.Wrap call's operation, its first argument,
// which is the literal text a fmt.Errorf wrap held before
// (docs/decisions/0043-MADR-name-the-package-once-in-wrapped-errors.md
// D6-A).
func errorMessages(path string) ([]message, error) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
	if err != nil {
		return nil, err
	}
	var out []message
	ast.Inspect(f, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		var text ast.Node
		switch {
		case isCall(call, "fmt", "Errorf"), isCall(call, "errors", "New"):
			text = call
		case isCall(call, "errprefix", "Wrap") && len(call.Args) > 0:
			text = call.Args[0]
		default:
			return true
		}
		var b strings.Builder
		ast.Inspect(text, func(m ast.Node) bool {
			if lit, ok := m.(*ast.BasicLit); ok && lit.Kind == token.STRING {
				if s, err := strconv.Unquote(lit.Value); err == nil {
					b.WriteString(s)
				}
			}
			return true
		})
		out = append(out, message{text: b.String(), line: fset.Position(call.Pos()).Line})
		return true
	})
	return out, nil
}
