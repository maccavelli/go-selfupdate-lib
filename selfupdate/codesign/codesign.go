package codesign

import (
	"context"
	"errors"
	"fmt"
	"path"
	"regexp"
	"runtime"
	"strings"
	"unicode/utf8"

	"github.com/maccavelli/go-selfupdate-lib/selfupdate"
	"github.com/maccavelli/go-selfupdate-lib/selfupdate/service"
)

// defaultCodesign is the tool's path on every macOS: it is on the sealed
// system volume, not a Command Line Tools shim.
const defaultCodesign = "/usr/bin/codesign"

// toolEnv is the environment the tool runs with, built, never inherited, as
// the launchd backend builds launchctl's (0012-MADR §5).
var toolEnv = []string{"PATH=/usr/bin:/bin:/usr/sbin:/sbin", "LC_ALL=C"}

// maxDetail bounds the tool output an error carries.
const maxDetail = 1024

// identifierRe is what an identifier may be: never a flag, never a quote.
var identifierRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9.-]*$`)

// SignOptions configure NewSigner.
type SignOptions struct {
	// Identity is a signing identity codesign accepts: a certificate's
	// common name or SHA-1 hash, or "-" for an ad-hoc signature. Required.
	Identity string
	// Identifier is the signature's identifier, such as
	// "com.example.relay". Required: codesign's default comes from the
	// file name, which for a staging file is ".<product>".
	Identifier string
	// Keychain, when set, is the absolute path of a keychain to search for
	// Identity.
	Keychain string
	// Runtime signs with the hardened runtime (--options runtime).
	Runtime bool
	// Timestamp asks Apple's timestamp server for a secure timestamp. False
	// passes --timestamp=none, so an update never needs that server.
	Timestamp bool
	// Requirement, when set, is checked after signing, in addition to the
	// identifier, as with codesign -R. It has no leading "=".
	Requirement string
	// Codesign is the tool's absolute path. Empty means /usr/bin/codesign.
	Codesign string
	// Runner runs the tool. Nil means service.ExecRunner.
	Runner service.Runner
}

// CheckOptions configure NewChecker.
type CheckOptions struct {
	// Requirement, when set, must be met, as with codesign -R: for example
	// a Developer ID requirement naming a team. Empty checks only that the
	// signature is valid and meets its own designated requirement. It has
	// no leading "=".
	Requirement string
	// Codesign is the tool's absolute path. Empty means /usr/bin/codesign.
	Codesign string
	// Runner runs the tool. Nil means service.ExecRunner.
	Runner service.Runner
}

// tool is the codesign command and its runner.
type tool struct {
	path   string
	runner service.Runner
}

// newTool checks the tool's path as a macOS path, whatever this host is:
// the tool only ever runs there.
func newTool(goos, toolPath string, r service.Runner) (tool, error) {
	if goos != "darwin" {
		return tool{}, fmt.Errorf("%w: codesign runs on macOS, not %s", service.ErrUnsupported, goos)
	}
	if toolPath == "" {
		toolPath = defaultCodesign
	}
	if !path.IsAbs(toolPath) {
		return tool{}, fmt.Errorf("selfupdate: codesign: the tool path %q is not absolute", toolPath)
	}
	if r == nil {
		r = service.ExecRunner()
	}
	return tool{path: toolPath, runner: r}, nil
}

// run runs the tool with args, and returns its exit code and output.
func (t tool) run(ctx context.Context, args ...string) (service.Output, error) {
	out, err := t.runner.Run(ctx, service.Command{Path: t.path, Args: args, Env: append([]string(nil), toolEnv...)})
	if err != nil {
		return service.Output{}, fmt.Errorf("selfupdate: codesign: %w", err)
	}
	return out, nil
}

// detail is the tool's output for an error: stderr then stdout, on one
// line, without control characters, at most maxDetail bytes.
func detail(out service.Output) string {
	s := strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f {
			return ' '
		}
		return r
	}, string(out.Stderr)+" "+string(out.Stdout))
	s = strings.Join(strings.Fields(s), " ")
	if len(s) > maxDetail {
		s = strings.ToValidUTF8(s[:maxDetail], "") + "…"
	}
	if !utf8.ValidString(s) {
		s = strings.ToValidUTF8(s, "?")
	}
	return s
}

// checkRequirement refuses a requirement that is not one argument's worth
// of requirement text.
func checkRequirement(req string) error {
	if strings.ContainsAny(req, "\n\r\x00") || strings.HasPrefix(req, "=") {
		return fmt.Errorf("selfupdate: codesign: the requirement %q must be one line, with no leading \"=\"", req)
	}
	return nil
}

// verifyArgs are codesign's arguments to verify path against requirement.
func verifyArgs(requirement, path string) []string {
	args := []string{"--verify", "--strict"}
	if requirement != "" {
		args = append(args, "-R="+requirement)
	}
	return append(args, path)
}

type signer struct {
	tool
	o SignOptions
}

// NewSigner returns a Transformer that signs the staged file with
// o.Identity under o.Identifier, then verifies that the signature is valid
// and names that identifier, and meets o.Requirement when it is set
// (0012-MADR §5). Off macOS it returns service.ErrUnsupported.
func NewSigner(o SignOptions) (selfupdate.Transformer, error) {
	return newSigner(o, runtime.GOOS)
}

func newSigner(o SignOptions, goos string) (selfupdate.Transformer, error) {
	t, err := newTool(goos, o.Codesign, o.Runner)
	if err != nil {
		return nil, err
	}
	switch {
	case o.Identity == "":
		return nil, errors.New("selfupdate: codesign: Identity is required")
	case o.Identity != "-" && strings.HasPrefix(o.Identity, "-"), strings.ContainsAny(o.Identity, "\n\r\x00"):
		return nil, fmt.Errorf("selfupdate: codesign: invalid identity %q", o.Identity)
	case !identifierRe.MatchString(o.Identifier):
		return nil, fmt.Errorf("selfupdate: codesign: invalid identifier %q", o.Identifier)
	case o.Keychain != "" && !path.IsAbs(o.Keychain):
		return nil, fmt.Errorf("selfupdate: codesign: the keychain path %q is not absolute", o.Keychain)
	}
	if err := checkRequirement(o.Requirement); err != nil {
		return nil, err
	}
	return &signer{tool: t, o: o}, nil
}

// requirement is the identifier, and the configured requirement when set.
func (s *signer) requirement() string {
	r := `identifier "` + s.o.Identifier + `"`
	if s.o.Requirement != "" {
		r += " and (" + s.o.Requirement + ")"
	}
	return r
}

// Transform implements selfupdate.Transformer.
func (s *signer) Transform(ctx context.Context, req selfupdate.TransformRequest) error {
	if req.Platform.OS != "darwin" {
		return fmt.Errorf("selfupdate: codesign: refusing to sign a %q binary", req.Platform.OS)
	}
	args := []string{"--force", "--sign", s.o.Identity, "--identifier", s.o.Identifier}
	if s.o.Keychain != "" {
		args = append(args, "--keychain", s.o.Keychain)
	}
	if s.o.Runtime {
		args = append(args, "--options", "runtime")
	}
	if s.o.Timestamp {
		args = append(args, "--timestamp")
	} else {
		args = append(args, "--timestamp=none")
	}
	out, err := s.run(ctx, append(args, req.Path)...)
	if err != nil {
		return err
	}
	if out.ExitCode != 0 {
		return fmt.Errorf("selfupdate: codesign: signing failed (exit %d): %s", out.ExitCode, detail(out))
	}
	out, err = s.run(ctx, verifyArgs(s.requirement(), req.Path)...)
	if err != nil {
		return err
	}
	switch out.ExitCode {
	case 0:
		return nil
	case 1:
		return fmt.Errorf("selfupdate: codesign: the new signature is not valid (exit 1): %s", detail(out))
	case 3:
		return fmt.Errorf("selfupdate: codesign: the new signature does not meet %s (exit 3): %s", s.requirement(), detail(out))
	}
	return fmt.Errorf("selfupdate: codesign: verifying failed (exit %d): %s", out.ExitCode, detail(out))
}

type checker struct {
	tool
	requirement string
}

// NewChecker returns a Prober that, on the staged file, requires a valid
// signature that meets o.Requirement when it is set (0012-MADR §6). A
// missing or invalid signature, and an unmet requirement, wrap
// selfupdate.ErrIntegrity. It does nothing on the installed file, which is
// the same file. Off macOS it returns service.ErrUnsupported.
func NewChecker(o CheckOptions) (selfupdate.Prober, error) {
	return newChecker(o, runtime.GOOS)
}

func newChecker(o CheckOptions, goos string) (selfupdate.Prober, error) {
	t, err := newTool(goos, o.Codesign, o.Runner)
	if err != nil {
		return nil, err
	}
	if err := checkRequirement(o.Requirement); err != nil {
		return nil, err
	}
	return &checker{tool: t, requirement: o.Requirement}, nil
}

// Probe implements selfupdate.Prober.
func (c *checker) Probe(ctx context.Context, req selfupdate.ProbeRequest) error {
	if req.Phase != selfupdate.ProbeStaged {
		return nil
	}
	out, err := c.run(ctx, verifyArgs(c.requirement, req.Path)...)
	if err != nil {
		return err
	}
	switch out.ExitCode {
	case 0:
		return nil
	case 1:
		return fmt.Errorf("selfupdate: codesign: the binary's signature is missing or invalid (exit 1): %s: %w", detail(out), selfupdate.ErrIntegrity)
	case 3:
		return fmt.Errorf("selfupdate: codesign: the binary's signature does not meet %s (exit 3): %s: %w", c.requirement, detail(out), selfupdate.ErrIntegrity)
	}
	return fmt.Errorf("selfupdate: codesign: verifying failed (exit %d): %s", out.ExitCode, detail(out))
}
