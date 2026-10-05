package codesign

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/maccavelli/go-selfupdate-lib/selfupdate"
	"github.com/maccavelli/go-selfupdate-lib/selfupdate/service"
)

// Tests for docs/decisions/0012-PLAN-archive-assets-and-macos-codesign.md
// U4 step 7, against a fake runner.

// fakeTool records each command and answers with the next scripted output.
type fakeTool struct {
	calls   []service.Command
	outputs []service.Output
	err     error
}

func (f *fakeTool) Run(_ context.Context, c service.Command) (service.Output, error) {
	f.calls = append(f.calls, c)
	if f.err != nil {
		return service.Output{}, f.err
	}
	if len(f.outputs) == 0 {
		return service.Output{}, nil
	}
	out := f.outputs[0]
	f.outputs = f.outputs[1:]
	return out, nil
}

func (f *fakeTool) argv() [][]string {
	var out [][]string
	for _, c := range f.calls {
		out = append(out, append([]string{c.Path}, c.Args...))
	}
	return out
}

var darwin = selfupdate.Platform{OS: "darwin", Arch: "arm64"}

func signReq() selfupdate.TransformRequest {
	return selfupdate.TransformRequest{Product: "relay", Platform: darwin, Path: "/opt/relay/.relay.selfupdate-1"}
}

func mustSigner(t *testing.T, o SignOptions) (selfupdate.Transformer, *fakeTool) {
	t.Helper()
	f := &fakeTool{}
	if o.Runner == nil {
		o.Runner = f
	}
	s, err := signerFor(o, "darwin")
	if err != nil {
		t.Fatal(err)
	}
	return s, f
}

func TestSignerArguments(t *testing.T) {
	const path = "/opt/relay/.relay.selfupdate-1"
	for _, c := range []struct {
		name string
		o    SignOptions
		sign []string
		req  string
	}{
		{"ad-hoc", SignOptions{Identity: "-", Identifier: "com.example.relay"},
			[]string{"--force", "--sign", "-", "--identifier", "com.example.relay", "--timestamp=none", path},
			`identifier "com.example.relay"`},
		{"every option", SignOptions{
			Identity: "Apple Development: A (TEAMID)", Identifier: "com.example.relay", Keychain: "/k/login.keychain-db",
			Runtime: true, Timestamp: true, Requirement: "anchor apple generic", Codesign: "/opt/bin/codesign",
		},
			[]string{"--force", "--sign", "Apple Development: A (TEAMID)", "--identifier", "com.example.relay",
				"--keychain", "/k/login.keychain-db", "--options", "runtime", "--timestamp", path},
			`identifier "com.example.relay" and (anchor apple generic)`},
	} {
		t.Run(c.name, func(t *testing.T) {
			s, f := mustSigner(t, c.o)
			if err := s.Transform(context.Background(), signReq()); err != nil {
				t.Fatal(err)
			}
			tool := c.o.Codesign
			if tool == "" {
				tool = "/usr/bin/codesign"
			}
			want := [][]string{
				append([]string{tool}, c.sign...),
				{tool, "--verify", "--strict", "-R=" + c.req, path},
			}
			if got := f.argv(); !slices.EqualFunc(got, want, slices.Equal) {
				t.Fatalf("argv\n%q\nwant\n%q", got, want)
			}
			for _, call := range f.calls {
				if !slices.Equal(call.Env, []string{"PATH=/usr/bin:/bin:/usr/sbin:/sbin", "LC_ALL=C"}) {
					t.Fatalf("environment %q", call.Env)
				}
			}
		})
	}
}

func TestSignerErrors(t *testing.T) {
	for _, c := range []struct {
		name    string
		outputs []service.Output
		runErr  error
		want    string
	}{
		{"sign fails", []service.Output{{ExitCode: 1, Stderr: []byte("no identity found\n")}}, nil, "signing failed (exit 1): no identity found"},
		{"invalid", []service.Output{{}, {ExitCode: 1, Stderr: []byte("invalid signature")}}, nil, "is not valid (exit 1)"},
		{"requirement unmet", []service.Output{{}, {ExitCode: 3}}, nil, "does not meet identifier"},
		{"bad arguments", []service.Output{{}, {ExitCode: 2}}, nil, "verifying failed (exit 2)"},
		{"runner", nil, errors.New("cannot run"), "cannot run"},
	} {
		t.Run(c.name, func(t *testing.T) {
			s, f := mustSigner(t, SignOptions{Identity: "-", Identifier: "com.example.relay"})
			f.outputs, f.err = c.outputs, c.runErr
			err := s.Transform(context.Background(), signReq())
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("Transform = %v, want %q", err, c.want)
			}
			if errors.Is(err, selfupdate.ErrIntegrity) {
				t.Fatalf("a signer error wraps ErrIntegrity: %v", err)
			}
		})
	}
}

// TestSignerOutputDetail: the tool's output is one line, without control
// characters, and capped.
func TestSignerOutputDetail(t *testing.T) {
	s, f := mustSigner(t, SignOptions{Identity: "-", Identifier: "com.example.relay"})
	f.outputs = []service.Output{{ExitCode: 1, Stderr: []byte("line one\nline\x1b[31m two\n" + strings.Repeat("x", 4000))}}
	err := s.Transform(context.Background(), signReq())
	if err == nil {
		t.Fatal("accepted")
	}
	msg := err.Error()
	if strings.ContainsAny(msg, "\n\x1b") || !strings.Contains(msg, "line one line [31m two") || len(msg) > maxDetail+200 {
		t.Fatalf("detail %q (%d bytes)", msg[:min(len(msg), 120)], len(msg))
	}
}

func TestSignerRefusesForeignPlatform(t *testing.T) {
	s, f := mustSigner(t, SignOptions{Identity: "-", Identifier: "com.example.relay"})
	for _, os := range []string{"linux", "windows", ""} {
		req := signReq()
		req.Platform.OS = os
		if err := s.Transform(context.Background(), req); err == nil {
			t.Errorf("signed a %q binary", os)
		}
	}
	if len(f.calls) != 0 {
		t.Fatalf("ran the tool: %q", f.argv())
	}
}

func TestNewSignerRefuses(t *testing.T) {
	ok := SignOptions{Identity: "-", Identifier: "com.example.relay"}
	for name, edit := range map[string]func(*SignOptions){
		"no identity":          func(o *SignOptions) { o.Identity = "" },
		"identity as a flag":   func(o *SignOptions) { o.Identity = "--deep" },
		"identity with a line": func(o *SignOptions) { o.Identity = "A\nB" },
		"no identifier":        func(o *SignOptions) { o.Identifier = "" },
		"identifier as a flag": func(o *SignOptions) { o.Identifier = "-x" },
		"identifier quoted":    func(o *SignOptions) { o.Identifier = `a"b` },
		"relative keychain":    func(o *SignOptions) { o.Keychain = "login.keychain" },
		"relative tool":        func(o *SignOptions) { o.Codesign = "codesign" },
		"requirement with =":   func(o *SignOptions) { o.Requirement = "=anchor apple" },
		"requirement lines":    func(o *SignOptions) { o.Requirement = "anchor apple\nor true" },
	} {
		o := ok
		edit(&o)
		if _, err := signerFor(o, "darwin"); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	for _, goos := range []string{"linux", "windows"} {
		if _, err := signerFor(ok, goos); !errors.Is(err, service.ErrUnsupported) {
			t.Errorf("%s: %v, want ErrUnsupported", goos, err)
		}
	}
	if _, err := signerFor(ok, "darwin"); err != nil {
		t.Fatalf("a valid signer: %v", err)
	}
}

func mustChecker(t *testing.T, o CheckOptions) (selfupdate.Prober, *fakeTool) {
	t.Helper()
	f := &fakeTool{}
	o.Runner = f
	c, err := checkerFor(o, "darwin")
	if err != nil {
		t.Fatal(err)
	}
	return c, f
}

func staged() selfupdate.ProbeRequest {
	return selfupdate.ProbeRequest{Product: "relay", Path: "/opt/relay/.relay.selfupdate-1", Phase: selfupdate.ProbeStaged}
}

func TestCheckerArguments(t *testing.T) {
	for _, c := range []struct {
		req  string
		want []string
	}{
		{"", []string{"/usr/bin/codesign", "--verify", "--strict", "/opt/relay/.relay.selfupdate-1"}},
		{"anchor apple generic", []string{"/usr/bin/codesign", "--verify", "--strict", "-R=anchor apple generic", "/opt/relay/.relay.selfupdate-1"}},
	} {
		ch, f := mustChecker(t, CheckOptions{Requirement: c.req})
		if err := ch.Probe(context.Background(), staged()); err != nil {
			t.Fatal(err)
		}
		if got := f.argv(); len(got) != 1 || !slices.Equal(got[0], c.want) {
			t.Fatalf("argv %q, want %q", got, c.want)
		}
	}
}

// TestCheckerErrors: exits 1 and 3 are integrity failures; exit 2 and a
// runner failure are not.
func TestCheckerErrors(t *testing.T) {
	for _, c := range []struct {
		out       service.Output
		runErr    error
		want      string
		integrity bool
	}{
		{service.Output{ExitCode: 1, Stderr: []byte("code object is not signed at all")}, nil, "missing or invalid (exit 1): code object is not signed at all", true},
		{service.Output{ExitCode: 3}, nil, "does not meet anchor apple generic (exit 3)", true},
		{service.Output{ExitCode: 2}, nil, "verifying failed (exit 2)", false},
		{service.Output{}, errors.New("cannot run"), "cannot run", false},
	} {
		ch, f := mustChecker(t, CheckOptions{Requirement: "anchor apple generic"})
		f.outputs, f.err = []service.Output{c.out}, c.runErr
		err := ch.Probe(context.Background(), staged())
		if err == nil || !strings.Contains(err.Error(), c.want) || errors.Is(err, selfupdate.ErrIntegrity) != c.integrity {
			t.Errorf("Probe = %v; want %q, integrity %v", err, c.want, c.integrity)
		}
	}
}

func TestCheckerSkipsInstalled(t *testing.T) {
	ch, f := mustChecker(t, CheckOptions{})
	req := staged()
	req.Phase = selfupdate.ProbeInstalled
	if err := ch.Probe(context.Background(), req); err != nil || len(f.calls) != 0 {
		t.Fatalf("Probe = %v, calls %q", err, f.argv())
	}
}

func TestNewCheckerRefuses(t *testing.T) {
	for name, o := range map[string]CheckOptions{
		"relative tool":      {Codesign: "codesign"},
		"requirement with =": {Requirement: "=anchor apple"},
		"requirement NUL":    {Requirement: "anchor\x00apple"},
	} {
		if _, err := checkerFor(o, "darwin"); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	for _, goos := range []string{"linux", "windows"} {
		if _, err := checkerFor(CheckOptions{}, goos); !errors.Is(err, service.ErrUnsupported) {
			t.Errorf("%s: %v, want ErrUnsupported", goos, err)
		}
	}
}

// TestPublicConstructorsFollowTheOS: on this OS, the exported constructors
// give what signerFor and checkerFor give for it.
func TestPublicConstructorsFollowTheOS(t *testing.T) {
	_, sErr := NewSigner(SignOptions{Identity: "-", Identifier: "com.example.relay"})
	_, cErr := NewChecker(CheckOptions{})
	onMac := errors.Is(sErr, service.ErrUnsupported) || errors.Is(cErr, service.ErrUnsupported)
	if wantUnsupported := !isDarwin(); onMac != wantUnsupported {
		t.Fatalf("NewSigner %v, NewChecker %v", sErr, cErr)
	}
}
