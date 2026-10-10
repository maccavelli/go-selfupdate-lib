package selfupdate

import (
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
	var found []string
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
		calls, err := directCalls(path)
		found = append(found, calls...)
		return err
	})
	if err != nil {
		t.Fatal(err)
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
