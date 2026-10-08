package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// TestUsageListsEveryFlag: each subcommand's usage line in the package
// comment names every flag its run function registers, a required one as
// -name and an optional one inside […]
// (docs/decisions/0015-MADR-remediate-third-debugging-pass-findings.md G7).
func TestUsageListsEveryFlag(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	var doc string
	flags := map[string]map[string]bool{} // subcommand: flag: required
	for _, name := range files {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, name, nil, parser.ParseComments)
		if err != nil {
			t.Fatal(err)
		}
		if f.Doc != nil && name == "main.go" {
			doc = f.Doc.Text()
		}
		for _, d := range f.Decls {
			fn, ok := d.(*ast.FuncDecl)
			if !ok || fn.Body == nil || !strings.HasPrefix(fn.Name.Name, "run") {
				continue
			}
			collect(t, fn, flags)
		}
	}
	if doc == "" || len(flags) == 0 {
		t.Fatalf("found no usage comment (%d bytes) or no subcommands (%v)", len(doc), flags)
	}
	lines := map[string]string{}
	for _, line := range strings.Split(doc, "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 2 && fields[0] == "selfupdate-release" {
			lines[fields[1]] = " " + strings.Join(fields[2:], " ") + " "
		}
	}
	for cmd, fs := range flags {
		line, ok := lines[cmd]
		if !ok {
			t.Errorf("%s has no usage line", cmd)
			continue
		}
		for name, required := range fs {
			plain := strings.Contains(line, " -"+name+" ")
			optional := strings.Contains(line, "[-"+name+" ") || strings.Contains(line, "[-"+name+"]")
			switch {
			case required && !plain:
				t.Errorf("%s: -%s is required, and not in the usage line as -%s", cmd, name, name)
			case !required && !optional:
				t.Errorf("%s: -%s is not in the usage line", cmd, name)
			}
		}
	}
}

// collect records, for a run function, the subcommand newFlags names and
// each f.str(name, required) it registers.
func collect(t *testing.T, fn *ast.FuncDecl, flags map[string]map[string]bool) {
	t.Helper()
	var cmd string
	reg := map[string]bool{}
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		switch f := call.Fun.(type) {
		case *ast.Ident:
			if f.Name == "newFlags" && len(call.Args) == 1 {
				cmd = stringLit(t, call.Args[0])
			}
		case *ast.SelectorExpr:
			if f.Sel.Name == "str" && len(call.Args) == 2 {
				req, ok := call.Args[1].(*ast.Ident)
				if !ok {
					t.Fatalf("%s: f.str's second argument is not true or false", fn.Name.Name)
				}
				reg[stringLit(t, call.Args[0])] = req.Name == "true"
			}
		}
		return true
	})
	if cmd != "" {
		flags[cmd] = reg
	}
}

func stringLit(t *testing.T, e ast.Expr) string {
	t.Helper()
	lit, ok := e.(*ast.BasicLit)
	if !ok || lit.Kind != token.STRING {
		t.Fatalf("not a string literal: %T", e)
	}
	s, err := strconv.Unquote(lit.Value)
	if err != nil {
		t.Fatal(err)
	}
	return s
}
