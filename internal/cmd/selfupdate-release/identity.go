package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"

	"github.com/maccavelli/go-selfupdate-lib/selfupdate"
	"github.com/maccavelli/go-selfupdate-lib/selfupdate/archive"
)

// identityTimeout bounds one identity run (0013-MADR §5).
const identityTimeout = 30 * time.Second

type identityInput struct {
	Staging     string
	Asset       string
	Product     string
	Platform    selfupdate.Platform
	ArgsJSON    string
	WantVersion string
	WantKind    string
	SHA         string
	Timeout     time.Duration
}

// identityPattern is the first stdout line the program must print:
// buildinfo.Info.String() for this build, "<version> (<kind>)" then,
// optionally, the first 12 characters of the revision.
func identityPattern(version, kind, sha string) *regexp.Regexp {
	return regexp.MustCompile(`^` + regexp.QuoteMeta(version) + ` \(` + regexp.QuoteMeta(kind) + `\)( ` + regexp.QuoteMeta(sha[:12]) + `)?$`)
}

// identity is layer 3 of 0013-MADR §5: run the staged program, on its own
// platform, and require it to report the stamp it was built with.
func identity(ctx context.Context, in identityInput) (string, error) {
	if in.Platform.OS != runtime.GOOS || in.Platform.Arch != runtime.GOARCH {
		return "", fmt.Errorf("this runner is %s/%s, not %s/%s", runtime.GOOS, runtime.GOARCH, in.Platform.OS, in.Platform.Arch)
	}
	if !shaRe.MatchString(in.SHA) {
		return "", usagef("-sha %q is not a full commit SHA", in.SHA)
	}
	if in.WantKind != kindRelease && in.WantKind != kindLocal {
		return "", usagef("-want-kind %q is not release or local", in.WantKind)
	}
	var args []string
	if err := json.Unmarshal([]byte(in.ArgsJSON), &args); err != nil || len(args) == 0 {
		return "", usagef("-args-json %q is not a non-empty JSON array of strings", in.ArgsJSON)
	}
	if filepath.Base(in.Asset) != in.Asset || in.Asset == "" {
		return "", usagef("-asset %q must be a file name", in.Asset)
	}
	prog, err := stagedProgram(ctx, filepath.Join(in.Staging, in.Asset), in.Product, in.Platform)
	if err != nil {
		return "", err
	}
	dir, err := os.MkdirTemp("", "selfupdate-release-identity-*")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(dir) //nolint:errcheck // a temporary directory
	exe := filepath.Join(dir, programName(in.Product, in.Platform))
	if err := os.WriteFile(exe, prog, 0o755); err != nil { //nolint:gosec // the program must be executable to run it
		return "", err
	}
	timeout := in.Timeout
	if timeout == 0 {
		timeout = identityTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, exe, args...) //nolint:gosec // G204: runs the staged release under test by design
	cmd.Env = identityEnv()
	cmd.WaitDelay = time.Second
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	runErr := cmd.Run()
	if ctx.Err() != nil {
		return "", fmt.Errorf("%s %s did not finish within %s", in.Asset, strings.Join(args, " "), timeout)
	}
	first, _, _ := strings.Cut(stdout.String(), "\n")
	first = strings.TrimSuffix(first, "\r")
	if runErr != nil || !identityPattern(in.WantVersion, in.WantKind, in.SHA).MatchString(first) {
		return "", fmt.Errorf("%s %s printed %q first, want \"%s (%s) %s\" (exit: %w)\nstdout:\n%s\nstderr:\n%s",
			in.Asset, strings.Join(args, " "), first, in.WantVersion, in.WantKind, in.SHA[:12], runErr,
			limit(stdout.Bytes()), limit(stderr.Bytes()))
	}
	return first, nil
}

// stagedProgram returns the program in a staged asset: the asset itself,
// or the program the client's unpacker extracts from it.
func stagedProgram(ctx context.Context, path, product string, p selfupdate.Platform) ([]byte, error) {
	if isArchive(path) {
		return unpackProgram(ctx, path, product, p)
	}
	return os.ReadFile(path) //nolint:gosec // the staged asset this run downloaded
}

func isArchive(path string) bool {
	for _, f := range []archive.Format{archive.TarGz, archive.Zip, archive.Gz} {
		if strings.HasSuffix(path, f.Extension()) {
			return true
		}
	}
	return false
}

// identityEnv is PATH, and SystemRoot on Windows, which a Windows program
// needs to start.
func identityEnv() []string {
	env := []string{"PATH=" + os.Getenv("PATH")}
	if v, ok := os.LookupEnv("SystemRoot"); ok {
		env = append(env, "SystemRoot="+v)
	}
	return env
}

// limit returns the first 4 KiB of b, as text.
func limit(b []byte) string {
	const keep = 4 << 10
	if len(b) > keep {
		return string(b[:keep]) + "\n[…]"
	}
	return string(b)
}

func runIdentity(ctx context.Context, args []string, stdout io.Writer) error {
	f := newFlags("identity")
	staging := f.str("staging", true)
	asset := f.str("asset", true)
	product := f.str("product", true)
	goos := f.str("os", true)
	goarch := f.str("arch", true)
	argsJSON := f.str("args-json", true)
	version := f.str("want-version", true)
	kind := f.str("want-kind", true)
	sha := f.str("sha", true)
	if err := f.parse(args); err != nil {
		return err
	}
	first, err := identity(ctx, identityInput{
		Staging: *staging, Asset: *asset, Product: *product, Platform: selfupdate.Platform{OS: *goos, Arch: *goarch},
		ArgsJSON: *argsJSON, WantVersion: *version, WantKind: *kind, SHA: *sha,
	})
	if err != nil {
		return err
	}
	return printf(stdout, "selfupdate-release identity: %s reports %q\n", *asset, first)
}
