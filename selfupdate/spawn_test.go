package selfupdate

import (
	"context"
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/maccavelli/go-selfupdate-lib/internal/winproc"
)

// directStarts are the calls that start a process without
// internal/winproc, by import path.
var directStarts = map[string][]string{
	"os/exec": {"Command", "CommandContext"},
	"os":      {"StartProcess"},
	"syscall": {"StartProcess", "ForkExec"},
}

// spawnAllowlist names a file and call that may start a process directly,
// as "path/file.go: importpath.Func" ("probe.go: os/exec.Command"), with
// the reason. It is empty: every start under selfupdate/ goes through
// internal/winproc. An entry that no longer matches a call fails the test.
var spawnAllowlist = map[string]string{}

// TestNoDirectProcessStart: no non-test file under selfupdate/ starts a
// process except through internal/winproc, which gives a console-less
// caller's children a windowless console and refuses a batch file
// (docs/decisions/0021-MADR-task-scheduler-lifecycle-and-windowless-children.md
// D7). os.StartProcess and syscall.StartProcess are banned too: they are
// the bypass one layer down.
func TestNoDirectProcessStart(t *testing.T) {
	t.Parallel()
	var found []string
	read := 0
	err := filepath.WalkDir(".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == "testdata" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		read++
		calls, err := directCalls(path)
		found = append(found, calls...)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if read == 0 {
		t.Fatal("no source file under selfupdate/: the test reads the source, so run it from its package directory")
	}
	used := map[string]bool{}
	for _, f := range found {
		key := f[strings.Index(f, " ")+1:]
		if _, ok := spawnAllowlist[key]; ok {
			used[key] = true
			continue
		}
		t.Errorf("%s: start the process through internal/winproc (0021-MADR D7)", f)
	}
	for key := range spawnAllowlist {
		if !used[key] {
			t.Errorf("spawnAllowlist entry %q matches no call: remove it", key)
		}
	}
}

// directCalls returns "file:line file: pkg.Func" for each banned call in
// path, resolving each import's local name, so an aliased import is
// caught. A dot import of a banned package is reported as such.
func directCalls(path string) ([]string, error) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
	if err != nil {
		return nil, err
	}
	slash := filepath.ToSlash(path)
	local := map[string]string{} // local name -> import path
	var found []string
	for _, imp := range f.Imports {
		p, err := strconv.Unquote(imp.Path.Value)
		if err != nil {
			return nil, err
		}
		if _, banned := directStarts[p]; !banned {
			continue
		}
		name := p[strings.LastIndex(p, "/")+1:]
		if imp.Name != nil {
			name = imp.Name.Name
		}
		if name == "." {
			found = append(found, fmt.Sprintf("%s: %s: dot import of %s", fset.Position(imp.Pos()), slash, p))
			continue
		}
		local[name] = p
	}
	ast.Inspect(f, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		id, ok := sel.X.(*ast.Ident)
		if !ok {
			return true
		}
		p, ok := local[id.Name]
		if !ok {
			return true
		}
		for _, fn := range directStarts[p] {
			if sel.Sel.Name == fn {
				found = append(found, fmt.Sprintf("%s: %s: %s.%s", fset.Position(call.Pos()), slash, p, fn))
			}
		}
		return true
	})
	sort.Strings(found)
	return found, nil
}

// TestErrBatchFileIsWinproc: the exported sentinel is the very value
// winproc returns, so errors.Is finds it on every OS (0022-MADR H1). The
// operands are compared as interfaces, for identity: a wrapper around
// winproc's value would satisfy errors.Is one way and still not be it.
func TestErrBatchFileIsWinproc(t *testing.T) {
	t.Parallel()
	if any(ErrBatchFile) != any(winproc.ErrBatchFile) {
		t.Fatalf("selfupdate.ErrBatchFile %q is not winproc.ErrBatchFile %q", ErrBatchFile, winproc.ErrBatchFile)
	}
	if !errors.Is(fmt.Errorf("site: %w", winproc.ErrBatchFile), ErrBatchFile) {
		t.Fatal("errors.Is does not find ErrBatchFile in a wrapped winproc error")
	}
}

// chainModule is this module's path; a package under selfupdate/ is
// chainModule + "/selfupdate" + its directory.
const chainModule = "github.com/maccavelli/go-selfupdate-lib"

// chainFunc is a function by import path and name.
type chainFunc struct{ pkg, name string }

// chainFile is a parsed non-test file and its package's import path.
type chainFile struct {
	pkg  string
	file *ast.File
}

// chainWant are functions the walk must reach, so that a walk that
// follows nothing fails rather than passes.
var chainWant = []chainFunc{
	{chainModule + "/selfupdate/service", "runExec"},
	{chainModule + "/selfupdate/service", "startWith"},
	{chainModule + "/selfupdate/service", "startDetached"},
	{chainModule + "/selfupdate/service", "DetachProcess"},
}

// TestBatchErrorChain: every error from winproc.Command and
// winproc.Detached in a non-test file under selfupdate/ reaches its
// function's caller unchanged or wrapped with %w, and so does every error
// from a plain function that returns one, across packages, so that
// errors.Is(err, ErrBatchFile) holds at the exported entry points
// (0022-MADR H1). It is a static check, so it holds on every OS, not only
// where the refusal can happen. A call site must read
// "x, err := f(…); if err != nil { return …, err }" (or wrap with %w), or
// "return f(…)"; a form the walk cannot follow fails with its line.
//
// The walk stops at a method or a function literal, and every method on
// the chain is reached through an interface (Prober, Runner, Detacher),
// which no syntax walk can follow. TestBatchErrorReachesCallers, and its
// cases in the service, ghattest, codesign and cli packages, hold the
// chain past that boundary, at the entry points a program calls
// (0022-MADR Amendment A2).
func TestBatchErrorChain(t *testing.T) {
	t.Parallel()
	w := chainWalk{
		fset: token.NewFileSet(),
		sources: map[chainFunc]bool{
			{chainModule + "/internal/winproc", "Command"}:  true,
			{chainModule + "/internal/winproc", "Detached"}: true,
		},
		checked: map[token.Pos]bool{},
	}
	files, err := chainLoad(w.fset)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) == 0 {
		t.Fatal("no source file under selfupdate/: the test reads the source, so run it from its package directory")
	}
	for grew := true; grew; {
		grew = w.pass(files)
	}
	for _, p := range w.problems {
		t.Errorf("%s (0022-MADR H1: errors.Is(err, ErrBatchFile) must hold at the caller)", p)
	}
	for _, f := range chainWant {
		if !w.sources[f] {
			t.Errorf("the walk did not reach %s.%s: it follows less than it should", f.pkg, f.name)
		}
	}
}

// chainLoad parses every non-test Go file under the working directory,
// outside testdata.
func chainLoad(fset *token.FileSet) ([]chainFile, error) {
	var files []chainFile
	err := filepath.WalkDir(".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == "testdata" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		f, err := parser.ParseFile(fset, filepath.ToSlash(path), nil, parser.SkipObjectResolution)
		if err != nil {
			return err
		}
		pkg := chainModule + "/selfupdate"
		if dir := filepath.ToSlash(filepath.Dir(path)); dir != "." {
			pkg += "/" + dir
		}
		files = append(files, chainFile{pkg: pkg, file: f})
		return nil
	})
	return files, err
}

// chainWalk is the state of TestBatchErrorChain's walk: the functions
// whose error result carries winproc's, and the calls already judged.
type chainWalk struct {
	fset     *token.FileSet
	sources  map[chainFunc]bool
	checked  map[token.Pos]bool
	problems []string
}

// pass judges every call of a source not judged yet, and reports whether
// it found a new source, which needs another pass.
func (w *chainWalk) pass(files []chainFile) bool {
	grew := false
	for _, cf := range files {
		imports := chainImports(cf.file)
		for _, decl := range cf.file.Decls {
			if fd, ok := decl.(*ast.FuncDecl); ok && fd.Body != nil && w.function(cf.pkg, imports, fd) {
				grew = true
			}
		}
	}
	return grew
}

// function judges the source calls in fd, and reports whether fd became a
// source: a plain function that returns one's error.
func (w *chainWalk) function(pkg string, imports map[string]string, fd *ast.FuncDecl) bool {
	isSource := func(c *ast.CallExpr) bool {
		fn, ok := chainCallee(c, pkg, imports)
		return ok && w.sources[fn]
	}
	grew := false
	var stack []ast.Node
	ast.Inspect(fd.Body, func(n ast.Node) bool {
		if n == nil {
			stack = stack[:len(stack)-1]
			return true
		}
		stack = append(stack, n)
		call, ok := n.(*ast.CallExpr)
		if !ok || w.checked[call.Pos()] || !isSource(call) {
			return true
		}
		w.checked[call.Pos()] = true
		returned, problem := chainFollow(stack, imports, isSource)
		if problem != "" {
			w.problems = append(w.problems, fmt.Sprintf("%s: %s", w.fset.Position(call.Pos()), problem))
			return true
		}
		fn := chainFunc{pkg, fd.Name.Name}
		if returned && fd.Recv == nil && !chainInLiteral(stack) && !w.sources[fn] {
			w.sources[fn] = true
			grew = true
		}
		return true
	})
	return grew
}

// chainFollow reports whether the error of the call at the top of stack
// is returned, unchanged or wrapped with %w, or a problem.
func chainFollow(stack []ast.Node, imports map[string]string, isSource func(*ast.CallExpr) bool) (bool, string) {
	call := stack[len(stack)-1].(*ast.CallExpr)
	switch parent := stack[len(stack)-2].(type) {
	case *ast.ReturnStmt:
		return true, ""
	case *ast.AssignStmt:
		if len(parent.Rhs) != 1 || parent.Rhs[0] != call {
			return false, "the call is one of several values assigned; assign it alone"
		}
		id, ok := parent.Lhs[len(parent.Lhs)-1].(*ast.Ident)
		if !ok || id.Name == "_" {
			return false, "the call's error is discarded"
		}
		// Scan the statements after the assignment, then after each
		// enclosing statement, out to the function or literal's body.
		for i := len(stack) - 2; i > 0; i-- {
			if _, ok := stack[i].(*ast.FuncLit); ok {
				break
			}
			block, ok := stack[i-1].(*ast.BlockStmt)
			if !ok {
				continue
			}
			at := -1
			for j, s := range block.List {
				if s == stack[i] {
					at = j
				}
			}
			for _, s := range block.List[at+1:] {
				done, problem := chainStmt(s, id.Name, imports, isSource)
				if problem != "" || done {
					return done, problem
				}
			}
		}
		return false, "the call's error is never returned"
	default:
		return false, "the call's error is neither assigned nor returned"
	}
}

// chainStmt judges one statement after the error e was assigned: done
// when it returns e as the caller needs it, a problem when it loses e or
// uses it in a way the walk cannot follow, neither when it does not touch e.
func chainStmt(s ast.Stmt, e string, imports map[string]string, isSource func(*ast.CallExpr) bool) (bool, string) {
	if !chainMentions(s, e) {
		return false, ""
	}
	switch s := s.(type) {
	case *ast.ReturnStmt:
		return chainReturn(s, e, imports)
	case *ast.IfStmt:
		if s.Init != nil && chainMentions(s.Init, e) {
			return false, "err is used in an if statement's init"
		}
		if s.Else != nil && chainMentions(s.Else, e) {
			return false, "err is used in an else branch the walk does not follow"
		}
		returns := false
		for _, b := range s.Body.List {
			if !chainMentions(b, e) {
				continue
			}
			switch b := b.(type) {
			case *ast.ReturnStmt:
				if _, problem := chainReturn(b, e, imports); problem != "" {
					return false, problem
				}
				returns = true
			case *ast.AssignStmt:
				// Re-assigned from another source call, which is
				// checked on its own: startDetached's retry.
				if c, ok := b.Rhs[0].(*ast.CallExpr); !ok || len(b.Rhs) != 1 || !isSource(c) {
					return false, "err is overwritten in a branch"
				}
			default:
				return false, "err is used in a branch the walk does not follow"
			}
		}
		// "if err != nil { return …err… }" returns every non-nil error.
		if cond, ok := s.Cond.(*ast.BinaryExpr); ok && returns && cond.Op == token.NEQ &&
			chainIsIdent(cond.X, e) && chainIsIdent(cond.Y, "nil") {
			return true, ""
		}
		return false, ""
	default:
		return false, "err is used in a statement the walk does not follow; return it or wrap it with %w"
	}
}

// chainReturn judges a return statement that mentions e.
func chainReturn(r *ast.ReturnStmt, e string, imports map[string]string) (bool, string) {
	for _, x := range r.Results {
		if chainIsIdent(x, e) {
			return true, ""
		}
		c, ok := x.(*ast.CallExpr)
		if !ok || !chainMentions(c, e) {
			continue
		}
		fn, _ := chainCallee(c, "", imports)
		switch fn {
		case chainFunc{"errors", "Join"}:
			return true, ""
		case chainFunc{"fmt", "Errorf"}:
			return chainVerb(c, e)
		}
		return false, "err is returned through a call the walk does not follow"
	}
	return false, "err is returned in a form the walk does not follow"
}

// chainVerb checks that every argument e of an fmt.Errorf call is
// formatted with %w.
func chainVerb(c *ast.CallExpr, e string) (bool, string) {
	lit, ok := c.Args[0].(*ast.BasicLit)
	if !ok || lit.Kind != token.STRING {
		return false, "fmt.Errorf's format is not a literal"
	}
	format, err := strconv.Unquote(lit.Value)
	if err != nil {
		return false, "fmt.Errorf's format does not unquote"
	}
	var verbs []byte
	for i := 0; i < len(format); i++ {
		if format[i] != '%' {
			continue
		}
		i++
		if i < len(format) && format[i] == '%' {
			continue
		}
		for i < len(format) && strings.IndexByte("+-# 0123456789.", format[i]) >= 0 {
			i++
		}
		if i < len(format) && (format[i] == '[' || format[i] == '*') {
			return false, "fmt.Errorf uses an argument index or a * width"
		}
		if i < len(format) {
			verbs = append(verbs, format[i])
		}
	}
	found := false
	for k, a := range c.Args[1:] {
		if !chainIsIdent(a, e) {
			if chainMentions(a, e) {
				return false, "fmt.Errorf formats an expression of err, not err"
			}
			continue
		}
		if k >= len(verbs) || verbs[k] != 'w' {
			return false, "fmt.Errorf formats err without %w, so errors.Is cannot reach it"
		}
		found = true
	}
	return found, ""
}

// chainImports maps each import's local name to its path.
func chainImports(f *ast.File) map[string]string {
	m := map[string]string{}
	for _, imp := range f.Imports {
		p, err := strconv.Unquote(imp.Path.Value)
		if err != nil {
			continue
		}
		name := p[strings.LastIndex(p, "/")+1:]
		if imp.Name != nil {
			name = imp.Name.Name
		}
		m[name] = p
	}
	return m
}

// chainCallee names the function a call calls: a plain name in pkg, or a
// package-qualified one. A method call is not named.
func chainCallee(c *ast.CallExpr, pkg string, imports map[string]string) (chainFunc, bool) {
	switch fun := c.Fun.(type) {
	case *ast.Ident:
		return chainFunc{pkg, fun.Name}, true
	case *ast.SelectorExpr:
		if id, ok := fun.X.(*ast.Ident); ok {
			if p, ok := imports[id.Name]; ok {
				return chainFunc{p, fun.Sel.Name}, true
			}
		}
	}
	return chainFunc{}, false
}

func chainIsIdent(x ast.Expr, name string) bool {
	id, ok := x.(*ast.Ident)
	return ok && id.Name == name
}

func chainMentions(n ast.Node, name string) bool {
	found := false
	ast.Inspect(n, func(n ast.Node) bool {
		if id, ok := n.(*ast.Ident); ok && id.Name == name {
			found = true
		}
		return !found
	})
	return found
}

func chainInLiteral(stack []ast.Node) bool {
	for _, n := range stack {
		if _, ok := n.(*ast.FuncLit); ok {
			return true
		}
	}
	return false
}

// TestBatchErrorReachesCallers: a batch-file refusal from behind an
// interface reaches the program as ErrBatchFile, through Updater.Run and
// through an install session, managed or not. Each fake returns it
// wrapped, as winproc's callers do, and must have run (0022-MADR
// Amendment A2, D5-A).
func TestBatchErrorReachesCallers(t *testing.T) {
	t.Parallel()
	refused := fmt.Errorf("fake: %w", ErrBatchFile)
	is := func(t *testing.T, ran bool, err error) {
		t.Helper()
		if !ran {
			t.Fatal("the fake never ran, so the case proves nothing")
		}
		if !errors.Is(err, ErrBatchFile) {
			t.Fatalf("err = %v, want it to wrap ErrBatchFile", err)
		}
	}
	req := applyReq()
	req.Yes = true

	t.Run("staged probe", func(t *testing.T) {
		ran := false
		u := newContractEnv(t).build(t)
		u.probes = []Prober{ProberFunc(func(context.Context, ProbeRequest) error { ran = true; return refused })}
		_, err := u.Run(context.Background(), req)
		is(t, ran, err)
	})
	t.Run("manifest verifier", func(t *testing.T) {
		ran := false
		_, exe := tempTarget(t)
		inst, err := NewStandaloneInstaller(InstallOptions{TargetPolicy: targetPolicy(exe)})
		if err != nil {
			t.Fatal(err)
		}
		rel, bodies, plats := fixtureRelease(t, "demo")
		sel, _ := NewExactAssetSelector(plats)
		u, err := New(Config{Source: &scriptSource{rel: rel, bodies: bodies}, Versions: NewStrictVersionPolicy(), Assets: sel,
			Installer: inst, Reporter: &recReporter{}, Confirmer: &recConfirmer{}, Limits: DefaultLimits(),
			ManifestVerifiers: []ManifestVerifier{ManifestVerifierFunc(func(context.Context, ManifestVerification) error {
				ran = true
				return refused
			})}})
		if err != nil {
			t.Fatal(err)
		}
		_, err = u.Run(context.Background(), req)
		is(t, ran, err)
	})
	t.Run("post-install probe", func(t *testing.T) {
		ran := false
		sess, _ := postInstallSession(t, ProberFunc(func(context.Context, ProbeRequest) error { ran = true; return refused }))
		_, err := sess.Install(context.Background(), InstallRequest{Product: "demo", Artifact: StagedArtifact{Path: stageNew(t, sess)}})
		is(t, ran, err)
	})
	t.Run("managed reconcile", func(t *testing.T) {
		_, exe := tempTarget(t)
		inner, err := NewStandaloneInstaller(InstallOptions{TargetPolicy: targetPolicy(exe)})
		if err != nil {
			t.Fatal(err)
		}
		rec := &ranRec{fakeRec: fakeRec{err: refused}}
		m, err := NewManagedInstaller(inner, &fakeLife{installed: true, running: true}, rec)
		if err != nil {
			t.Fatal(err)
		}
		target, err := m.ResolveTarget(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		sess, err := m.Begin(context.Background(), target)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = sess.Close() })
		_, err = sess.Install(context.Background(), InstallRequest{Product: "demo", Artifact: StagedArtifact{Path: stageNew(t, sess)}})
		is(t, rec.ran, err)
	})
}

// ranRec is fakeRec that records that Reconcile ran.
type ranRec struct {
	fakeRec
	ran bool
}

func (r *ranRec) Reconcile(ctx context.Context, product, exe string) (ReconcileResult, error) {
	r.ran = true
	return r.fakeRec.Reconcile(ctx, product, exe)
}
