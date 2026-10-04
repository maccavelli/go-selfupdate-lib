package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"golang.org/x/term"

	"github.com/maccavelli/go-selfupdate-lib/buildinfo"
	"github.com/maccavelli/go-selfupdate-lib/selfupdate"
	"github.com/maccavelli/go-selfupdate-lib/selfupdate/selfupdatetest"
)

// failSource fails discovery with a fixed error.
type failSource struct{ *selfupdatetest.FakeSource }

func (failSource) Latest(context.Context) (selfupdate.Release, error) {
	return selfupdate.Release{}, errors.New("fixture: source unavailable")
}

// blockSource blocks in Latest or OpenAsset until the run's context ends.
type blockSource struct {
	*selfupdatetest.FakeSource
	inLatest bool
	ready    func()
	deadline chan time.Time
}

func (s blockSource) Latest(ctx context.Context) (selfupdate.Release, error) {
	if s.deadline != nil {
		d, _ := ctx.Deadline()
		s.deadline <- d
	}
	if s.inLatest {
		<-ctx.Done()
		return selfupdate.Release{}, ctx.Err()
	}
	return s.FakeSource.Latest(ctx)
}

func (s blockSource) OpenAsset(ctx context.Context, rel selfupdate.Release, a selfupdate.Asset) (io.ReadCloser, error) {
	if s.ready != nil {
		s.ready()
	}
	<-ctx.Done()
	return nil, ctx.Err()
}

var releaseID = buildinfo.Info{Version: "v1.0.0", Kind: buildinfo.KindRelease}

type scenario struct {
	name        string
	latest      string // the fixture's latest release
	src         func(*selfupdatetest.FakeSource) selfupdate.ReleaseSource
	id          buildinfo.Info
	flags       Flags
	req         *selfupdate.Request // when set, used as is, bypassing Flags.Request
	stdin       string
	interactive bool
	closeErr    error // when set, the session's Close fails after the install
}

var scenarios = []scenario{
	{name: "up-to-date", latest: "v1.0.0", id: releaseID, flags: Flags{Check: true}},
	{name: "available", latest: "v1.1.0", id: releaseID, flags: Flags{Check: true}},
	{name: "local-available", latest: "v1.1.0", id: buildinfo.Info{Kind: buildinfo.KindLocal}, flags: Flags{Check: true}},
	{name: "applied", latest: "v1.1.0", id: releaseID, flags: Flags{Yes: true}},
	{name: "declined", latest: "v1.1.0", id: releaseID, stdin: "n\n", interactive: true},
	{name: "dry-run", latest: "v1.1.0", id: releaseID, flags: Flags{DryRun: true}},
	{name: "no-confirm", latest: "v1.1.0", id: releaseID},
	{name: "failed", latest: "v1.1.0", id: releaseID, flags: Flags{Check: true},
		src: func(f *selfupdatetest.FakeSource) selfupdate.ReleaseSource { return failSource{f} }},
	// A late error is a warning: exit 0 (0010-MADR Q3).
	{name: "warning", latest: "v1.1.0", id: releaseID, flags: Flags{Yes: true}, closeErr: errors.New("unlock failed")},
	{name: "contradiction", latest: "v1.1.0", id: releaseID,
		req: &selfupdate.Request{Product: "demo", CurrentVersion: "v1.0.0", CurrentBuild: selfupdate.ReleaseBuild, CheckOnly: true, Yes: true}},
}

// outcome is what one invocation wrote, with the asset name normalized
// (rule 7).
type outcome struct {
	stdout, stderr string
	code           int
	tg             target
	res            selfupdate.Result
	err            error
}

// fixture builds the scenario's source and target.
func (sc scenario) fixture(t *testing.T, product string) (selfupdate.ReleaseSource, target) {
	t.Helper()
	fake := selfupdatetest.NewFakeSource(sc.latest, release(product, "v1.0.0"), release(product, "v1.1.0"))
	var src selfupdate.ReleaseSource = fake
	if sc.src != nil {
		src = sc.src(fake)
	}
	return src, newTarget(t)
}

// run runs the scenario through Run and Exit, as a program would.
func (sc scenario) run(t *testing.T, asJSON bool) outcome {
	t.Helper()
	src, tg := sc.fixture(t, "demo")
	u, err := buildUpdaterClosing(src, tg, sc.closeErr)
	if err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	req, err := sc.flags.Request("demo", sc.id)
	if sc.req != nil {
		req, err = *sc.req, nil
	}
	if err != nil {
		t.Fatal(err)
	}
	res, err := Run(context.Background(), u, req, Options{
		Stdout: &stdout, Stderr: &stderr, Stdin: strings.NewReader(sc.stdin),
		Interactive: sc.interactive, JSON: asJSON, Signals: []os.Signal{},
	})
	code := Exit(&stderr, res, err)
	return outcome{stdout: normalize(stdout.String(), "demo"), stderr: normalize(stderr.String(), "demo"),
		code: code, tg: tg, res: res, err: err}
}

// normalize replaces the running platform's asset name with {{asset}}.
func normalize(s, product string) string {
	return strings.ReplaceAll(s, selfupdate.ExactAssetName(product, here), "{{asset}}")
}

func TestGolden(t *testing.T) {
	for _, sc := range scenarios {
		t.Run(sc.name, func(t *testing.T) {
			text := sc.run(t, false)
			js := sc.run(t, true)
			if text.code != js.code {
				t.Fatalf("exit %d in text mode, %d in JSON mode", text.code, js.code)
			}
			golden(t, sc.name+".text.stderr", text.stderr)
			golden(t, sc.name+".json.stdout", js.stdout)
			golden(t, sc.name+".json.stderr", js.stderr)
			golden(t, sc.name+".code", strconv.Itoa(text.code)+"\n")
		})
	}
}

func TestStdoutEmptyWithoutJSON(t *testing.T) {
	for _, sc := range scenarios {
		if out := sc.run(t, false); out.stdout != "" {
			t.Errorf("%s: stdout %q without --json", sc.name, out.stdout)
		}
	}
}

func TestJSONStream(t *testing.T) {
	for _, sc := range scenarios {
		out := sc.run(t, true)
		lines := strings.Split(strings.TrimSuffix(out.stdout, "\n"), "\n")
		if !strings.HasSuffix(out.stdout, "\n") || len(lines) == 0 {
			t.Fatalf("%s: stdout %q", sc.name, out.stdout)
		}
		for i, l := range lines {
			var obj map[string]any
			if err := json.Unmarshal([]byte(l), &obj); err != nil {
				t.Fatalf("%s line %d %q: %v", sc.name, i, l, err)
			}
			isResult := obj["kind"] == "result"
			if isResult != (i == len(lines)-1) {
				t.Fatalf("%s line %d of %d has kind %v", sc.name, i, len(lines), obj["kind"])
			}
		}
	}
}

func TestConfirmation(t *testing.T) {
	yes := scenario{name: "yes", latest: "v1.1.0", id: releaseID, stdin: "y\n", interactive: true}.run(t, false)
	if yes.code != 0 || !yes.res.Applied || yes.stdout != "" {
		t.Fatalf("y: code %d applied %t stdout %q", yes.code, yes.res.Applied, yes.stdout)
	}
	if !strings.Contains(yes.stderr, "? [y/N] ") {
		t.Fatalf("y: no prompt on stderr: %q", yes.stderr)
	}
	no := scenario{name: "no", latest: "v1.1.0", id: releaseID, stdin: "n\n", interactive: true}.run(t, false)
	if no.code != 0 || !no.res.Declined || no.stdout != "" {
		t.Fatalf("n: code %d declined %t stdout %q", no.code, no.res.Declined, no.stdout)
	}
	no.tg.unchanged(t)
	off := scenario{name: "off", latest: "v1.1.0", id: releaseID}.run(t, false)
	if off.code != 1 || !errors.Is(off.err, selfupdate.ErrConfirmationRequired) {
		t.Fatalf("not interactive: code %d err %v", off.code, off.err)
	}
	off.tg.unchanged(t)
}

// setNotify replaces notifyContext for one test.
func setNotify(t *testing.T, f func(context.Context, ...os.Signal) (context.Context, context.CancelFunc)) {
	t.Helper()
	old := notifyContext
	notifyContext = f
	t.Cleanup(func() { notifyContext = old })
}

func TestSignalsWired(t *testing.T) {
	var got [][]os.Signal
	setNotify(t, func(ctx context.Context, sigs ...os.Signal) (context.Context, context.CancelFunc) {
		got = append(got, slices.Clone(sigs))
		ctx, cancel := context.WithCancel(ctx)
		cancel() // as if a signal arrived at once
		return ctx, cancel
	})
	for _, tc := range []struct {
		signals []os.Signal
		want    [][]os.Signal
		check   bool  // with no signal there is no cancel, so only check
		err     error // the run's outcome
	}{
		{nil, [][]os.Signal{{os.Interrupt, syscall.SIGTERM}}, false, context.Canceled},
		{[]os.Signal{}, nil, true, selfupdate.ErrUpdateAvailable},
	} {
		got = nil
		src, tg := scenario{latest: "v1.1.0"}.fixture(t, "demo")
		u := newUpdater(t, src, tg)
		var stderr bytes.Buffer
		_, err := Run(context.Background(), u, selfupdate.Request{Product: "demo", CurrentVersion: "v1.0.0",
			CurrentBuild: selfupdate.ReleaseBuild, Yes: !tc.check, CheckOnly: tc.check}, Options{Stderr: &stderr, Signals: tc.signals})
		if !slices.EqualFunc(got, tc.want, slices.Equal[[]os.Signal]) {
			t.Fatalf("signals %v: notifyContext got %v, want %v", tc.signals, got, tc.want)
		}
		if !errors.Is(err, tc.err) {
			t.Fatalf("signals %v: run returned %v, want %v", tc.signals, err, tc.err)
		}
		tg.unchanged(t)
	}
}

func TestTimeout(t *testing.T) {
	fake := selfupdatetest.NewFakeSource("v1.1.0", release("demo", "v1.1.0"))
	req := selfupdate.Request{Product: "demo", CurrentVersion: "v1.0.0", CurrentBuild: selfupdate.ReleaseBuild, CheckOnly: true}

	// Zero means DefaultTimeout.
	seen := make(chan time.Time, 1)
	tg := newTarget(t)
	start := time.Now()
	_, _ = Run(context.Background(), newUpdater(t, blockSource{FakeSource: fake, deadline: seen}, tg), req,
		Options{Stderr: &bytes.Buffer{}, Signals: []os.Signal{}})
	if d := (<-seen).Sub(start); d < DefaultTimeout-time.Second || d > DefaultTimeout+time.Second {
		t.Fatalf("deadline %v after the call, want %v", d, DefaultTimeout)
	}

	// Negative is refused.
	if _, err := Run(context.Background(), newUpdater(t, fake, tg), req, Options{Stderr: &bytes.Buffer{}, Timeout: -time.Nanosecond}); err == nil {
		t.Fatal("a negative timeout was accepted")
	}

	// An expired one fails the run, and leaves the target alone.
	var stderr bytes.Buffer
	res, err := Run(context.Background(), newUpdater(t, blockSource{FakeSource: fake, inLatest: true}, tg),
		selfupdate.Request{Product: "demo", CurrentVersion: "v1.0.0", CurrentBuild: selfupdate.ReleaseBuild, Yes: true},
		Options{Stderr: &stderr, Timeout: time.Nanosecond, Signals: []os.Signal{}})
	if !errors.Is(err, context.DeadlineExceeded) || Exit(&stderr, res, err) != 1 {
		t.Fatalf("expired timeout: %v", err)
	}
	tg.unchanged(t)
}

// onlyWriter fails the test if anything but the writer under test is
// written to.
type onlyWriter struct{ bytes.Buffer }

func TestExit(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
		code int
		line string
	}{
		{"nil", nil, 0, ""},
		{"available", selfupdate.ErrUpdateAvailable, 10, ""},
		{"wrapped available", errors.Join(errors.New("x"), selfupdate.ErrUpdateAvailable), 10, ""},
		{"wrapped", errors.Join(errors.New("first"), selfupdate.ErrIntegrity), 1, "update failed: first selfupdate: integrity check failed\n"},
		{"two lines", errors.New("one\ntwo\r\n\tthree"), 1, "update failed: one two three\n"},
	} {
		var w onlyWriter
		if code := Exit(&w, selfupdate.Result{}, tc.err); code != tc.code || w.String() != tc.line {
			t.Errorf("%s: Exit = %d, %q; want %d, %q", tc.name, code, w.String(), tc.code, tc.line)
		}
	}
}

func TestOptionsRefused(t *testing.T) {
	tg := newTarget(t)
	u := newUpdater(t, selfupdatetest.NewFakeSource("v1.0.0", release("demo", "v1.0.0")), tg)
	for _, tc := range []struct {
		name string
		u    *selfupdate.Updater
		o    Options
	}{
		{"nil updater", nil, Options{Stderr: &bytes.Buffer{}}},
		{"nil stderr", u, Options{}},
		{"json without stdout", u, Options{Stderr: &bytes.Buffer{}, JSON: true}},
		{"negative timeout", u, Options{Stderr: &bytes.Buffer{}, Timeout: -1}},
	} {
		if _, err := Run(context.Background(), tc.u, selfupdate.Request{}, tc.o); err == nil || !strings.HasPrefix(err.Error(), "cli: ") {
			t.Errorf("%s: err %v", tc.name, err)
		}
	}
}

// TestNoStdoutOutsideStdio scans the package's non-test files: os.Stdout
// appears only in StdioOptions, and nothing prints with fmt.Print*, print
// or println (A22).
func TestNoStdoutOutsideStdio(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	scanned := 0
	for _, name := range files {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, name, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		scanned++
		for _, decl := range f.Decls {
			fn, _ := decl.(*ast.FuncDecl)
			ast.Inspect(decl, func(n ast.Node) bool {
				switch v := n.(type) {
				case *ast.SelectorExpr:
					if x, ok := v.X.(*ast.Ident); ok && x.Name == "os" && v.Sel.Name == "Stdout" &&
						(fn == nil || fn.Name.Name != "StdioOptions") {
						t.Errorf("%s: os.Stdout outside StdioOptions", fset.Position(v.Pos()))
					}
					if x, ok := v.X.(*ast.Ident); ok && x.Name == "fmt" && strings.HasPrefix(v.Sel.Name, "Print") {
						t.Errorf("%s: fmt.%s", fset.Position(v.Pos()), v.Sel.Name)
					}
				case *ast.CallExpr:
					if id, ok := v.Fun.(*ast.Ident); ok && (id.Name == "print" || id.Name == "println") {
						t.Errorf("%s: %s", fset.Position(v.Pos()), id.Name)
					}
				}
				return true
			})
		}
	}
	if scanned < 3 {
		t.Fatalf("scanned %d files", scanned)
	}
}

// checkNoLeak fails the test when more goroutines run once it has cleaned
// up than when it was called. It polls for up to 2 s, because a goroutine
// that is ending is still counted for a moment (0004-MADR H6). The sleep
// only paces that poll; it orders nothing. Call it first.
func checkNoLeak(t testing.TB) {
	t.Helper()
	base := runtime.NumGoroutine()
	t.Cleanup(func() {
		deadline := time.Now().Add(2 * time.Second)
		for runtime.NumGoroutine() > base {
			if time.Now().After(deadline) {
				t.Errorf("goroutines: %d now, %d when the test began", runtime.NumGoroutine(), base)
				return
			}
			runtime.Gosched()
			time.Sleep(10 * time.Millisecond)
		}
	})
}

// TestRunsLeakNothing: the runs that drive a prompt confirmer, a timeout
// and a cancellation end every goroutine they start (0010-MADR C12).
func TestRunsLeakNothing(t *testing.T) {
	checkNoLeak(t)
	for _, sc := range []scenario{
		{name: "prompt yes", latest: "v1.1.0", id: releaseID, stdin: "y\n", interactive: true},
		{name: "prompt no", latest: "v1.1.0", id: releaseID, stdin: "n\n", interactive: true},
		{name: "no confirmer", latest: "v1.1.0", id: releaseID},
	} {
		sc.run(t, false)
		sc.run(t, true)
	}
	fake := selfupdatetest.NewFakeSource("v1.1.0", release("demo", "v1.1.0"))
	_, err := Run(context.Background(), newUpdater(t, blockSource{FakeSource: fake, inLatest: true}, newTarget(t)),
		selfupdate.Request{Product: "demo", CurrentVersion: "v1.0.0", CurrentBuild: selfupdate.ReleaseBuild, Yes: true},
		Options{Stderr: &bytes.Buffer{}, Timeout: time.Millisecond, Signals: []os.Signal{}})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("timed-out run: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	ready := make(chan struct{})
	done := make(chan error, 1)
	blocked := newUpdater(t, blockSource{FakeSource: fake, ready: func() { close(ready) }}, newTarget(t))
	go func() {
		_, err := Run(ctx, blocked,
			selfupdate.Request{Product: "demo", CurrentVersion: "v1.0.0", CurrentBuild: selfupdate.ReleaseBuild, Yes: true},
			Options{Stderr: &bytes.Buffer{}, Signals: []os.Signal{}})
		done <- err
	}()
	<-ready
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled run: %v", err)
	}
}

// failOn fails the one Write that contains marker, and keeps the rest. It
// does not embed its buffer: io.WriteString would find the buffer's
// WriteString and bypass Write.
type failOn struct {
	buf    bytes.Buffer
	marker string
}

var errWriteRefused = errors.New("fixture: write refused")

func (w *failOn) Write(p []byte) (int, error) {
	if w.marker != "" && bytes.Contains(p, []byte(w.marker)) {
		return 0, errWriteRefused
	}
	return w.buf.Write(p)
}

func (w *failOn) String() string { return w.buf.String() }

// TestCheckResultWriteFails: a check whose result object or summary
// cannot be written fails with the write error and exit 1, and Exit says
// so; it is not reported as an available update (0010-MADR C3).
func TestCheckResultWriteFails(t *testing.T) {
	req := selfupdate.Request{Product: "demo", CurrentVersion: "v1.0.0", CurrentBuild: selfupdate.ReleaseBuild, CheckOnly: true}
	for _, tc := range []struct {
		name           string
		json           bool
		stdout, stderr *failOn
	}{
		{"json", true, &failOn{marker: `"kind":"result"`}, &failOn{}},
		{"text", false, &failOn{}, &failOn{marker: "demo: update available: "}},
	} {
		src, tg := scenario{latest: "v1.1.0"}.fixture(t, "demo")
		res, err := Run(context.Background(), newUpdater(t, src, tg), req,
			Options{Stdout: tc.stdout, Stderr: tc.stderr, JSON: tc.json, Signals: []os.Signal{}})
		if !errors.Is(err, errWriteRefused) || errors.Is(err, selfupdate.ErrUpdateAvailable) {
			t.Errorf("%s: err %v, want the write error alone", tc.name, err)
		}
		if code := Exit(tc.stderr, res, err); code != 1 {
			t.Errorf("%s: exit %d, want 1", tc.name, code)
		}
		if !strings.Contains(tc.stderr.String(), "update failed: fixture: write refused\n") {
			t.Errorf("%s: stderr %q, want the failure reported", tc.name, tc.stderr.String())
		}
		tg.unchanged(t)
	}
}

// TestStdioOptions: the process's own streams, interactive only on a
// terminal, and every other option at its default (0010-MADR C12).
func TestStdioOptions(t *testing.T) {
	o := StdioOptions()
	if o.Stdout != os.Stdout || o.Stderr != os.Stderr || o.Stdin != os.Stdin {
		t.Fatalf("streams %v %v %v, want the process's own", o.Stdout, o.Stderr, o.Stdin)
	}
	if want := term.IsTerminal(int(os.Stdin.Fd())); o.Interactive != want {
		t.Fatalf("Interactive = %t, want %t", o.Interactive, want)
	}
	if o.JSON || o.Confirmer != nil || o.Timeout != 0 || o.Signals != nil {
		t.Fatalf("options %+v, want the defaults", o)
	}
}
