package selfupdate

import (
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/maccavelli/go-selfupdate-lib/internal/winproc"
)

var (
	// ErrUpdateAvailable is returned from Run in check mode when the selected
	// target is a different actionable release. ExitCode maps it to 10.
	ErrUpdateAvailable = errors.New("selfupdate: update available")
	// ErrConfirmationRequired is returned when apply needs a TTY or --yes.
	ErrConfirmationRequired = errors.New("selfupdate: confirmation required")
	// ErrNotConstructed is returned by an Updater or a Checker that its
	// constructor did not make, such as a zero value or a nil pointer
	// (0015-MADR C8).
	ErrNotConstructed = errors.New("selfupdate: not constructed by its constructor")
	// ErrConcurrentUpdate is returned when another update holds the target
	// lock, an overlapping Run is already in progress on the same Updater,
	// or the target, or its directory, was replaced during the update.
	ErrConcurrentUpdate = errors.New("selfupdate: concurrent update")
	// ErrManagedInstall is returned when managed stop, reconcile, start, or
	// health fails. Recovery errors are joined with it.
	ErrManagedInstall = errors.New("selfupdate: managed install failed")
	// ErrUnsupportedPlatform is returned when the requested platform is not
	// in the selector's caller-supplied matrix, when the release has no
	// asset for a platform that is, when an install names a platform other
	// than the running one, and when there is no executable image check
	// for the platform.
	ErrUnsupportedPlatform = errors.New("selfupdate: unsupported platform")
	// ErrIntegrity is returned when size, digest, or checksum verification
	// fails.
	ErrIntegrity = errors.New("selfupdate: integrity check failed")
	// ErrMutableRelease is returned when selected release metadata is not
	// immutable.
	ErrMutableRelease = errors.New("selfupdate: release is not immutable")
	// ErrNoRelease is returned when the release asked for does not exist:
	// a repository with no stable release yet, or a tag never published
	// (0015-MADR A1).
	ErrNoRelease = errors.New("selfupdate: no such release")
	// ErrRateLimited is the sentinel unwrapped by RateLimitError.
	ErrRateLimited = errors.New("selfupdate: rate limited")
	// ErrForceRequired is returned when applying over a local build without
	// Request.Force (0004-MADR G4).
	ErrForceRequired = errors.New("selfupdate: replacing a local build requires --force")
	// ErrLatestOlder is returned when the latest release, stable or on the
	// request's channel, is older than the running release build
	// (0004-MADR G4).
	ErrLatestOlder = errors.New("selfupdate: latest release is older than the running version")
	// ErrPlanStale is returned by Updater.Apply when the request is not the
	// one the plan was made for, or when the release no longer matches the
	// plan: another release ID, other assets or digests, another
	// operation, or another SHA256SUMS entry. A changed entry also matches
	// ErrIntegrity
	// (docs/decisions/0038-MADR-pinned-plans-lifecycle-conformance-and-bubble-tests.md
	// D1-A).
	ErrPlanStale = errors.New("selfupdate: the plan is stale")
	// ErrSymlinkInvocation is returned, as a *SymlinkInvocationError, when
	// the program was started through a symlink to its binary: the path
	// os.Executable returns is one, or os.Args[0] names one that leads to
	// the running binary. It is also returned when
	// TargetPolicy.ExecutablePath is a symlink. The update is refused on
	// every platform, so a link into a tree another tool manages is never
	// followed and overwritten. Set TargetPolicy.ExecutablePath to the
	// resolved path to update it on purpose.
	//
	// StandaloneInstaller.CleanupPending and KeptBackups do not fail with
	// it for a program started through a link: CleanupPending does
	// nothing, and KeptBackups lists the backups beside the link's target.
	// With an ExecutablePath that is a link they fail with it, as
	// ResolveTarget does
	// (docs/decisions/0041-MADR-refuse-symlink-invocation-on-every-platform.md
	// D1-A, D2-A, Amendment A1 D3-A, D5-A).
	ErrSymlinkInvocation = errors.New("selfupdate: refusing to update through a symlink")
	// ErrSpecialModeBits is returned, wrapped, when the target has the
	// setuid or setgid bit and TargetPolicy.AllowSpecialModeBits is not
	// set, so an update never silently grants or drops privileges. Set
	// AllowSpecialModeBits to replace such a target; the new binary keeps
	// the bits. The message names the target, and no option
	// (docs/decisions/0042-MADR-keep-go-identifiers-out-of-run-time-messages.md
	// D1-A, D2-A).
	ErrSpecialModeBits = errors.New("selfupdate: refusing to replace a setuid or setgid target")
	// ErrNoAllowedRoot is returned, wrapped, when no directory may hold the
	// target: the home directory is unavailable (unset, the filesystem
	// root, or missing), and TargetPolicy.AllowedRoots is empty. Add an
	// AllowedRoots entry that covers the target, as a service account with
	// no home needs (0042-MADR D1-A, D2-A).
	ErrNoAllowedRoot = errors.New("selfupdate: no allowed self-update root")
	// ErrBatchFile is returned, wrapped, on Windows when the program to
	// start is a .bat or .cmd file, in any spelling Windows resolves to
	// one: a trailing dot or space, an alternate data stream, or no
	// extension at all. The version probe, service.ExecRunner and
	// service.DetachProcess, and so the handoff, refuse it, because
	// cmd.exe would parse its arguments (BatBadBut, CVE-2024-24576;
	// 0021-MADR D8, 0022-MADR H2). It is never returned elsewhere.
	ErrBatchFile = winproc.ErrBatchFile
)

// RateLimitError retains GitHub rate-limit guidance. Missing or malformed
// optional headers yield zero values rather than guessed guidance.
type RateLimitError struct {
	// StatusCode is the HTTP status, typically 429 or 403.
	StatusCode int
	// RetryAfter is parsed from Retry-After when present.
	RetryAfter time.Duration
	// Reset is parsed from X-RateLimit-Reset when present.
	Reset time.Time
	// Remaining is parsed from X-RateLimit-Remaining when present.
	Remaining int64
}

// Error implements the error interface.
func (e *RateLimitError) Error() string {
	if e == nil {
		return ErrRateLimited.Error()
	}
	return fmt.Sprintf("selfupdate: rate limited: status %d remaining %d retry-after %s reset %s",
		e.StatusCode, e.Remaining, e.RetryAfter, e.Reset.UTC().Format(time.RFC3339))
}

// Unwrap reports ErrRateLimited.
func (e *RateLimitError) Unwrap() error {
	return ErrRateLimited
}

// HTTPError is a non-2xx HTTP response that no sentinel describes, from a
// ReleaseSource that speaks HTTP, such as GitHubSource. A missing release
// also wraps ErrNoRelease around it, and a rate limit is a *RateLimitError
// instead (docs/decisions/0025-MADR-align-with-go-sdk-design-practices.md D2).
type HTTPError struct {
	// Source names the source, such as "github".
	Source string
	// StatusCode is the HTTP status.
	StatusCode int
	// Diagnostic is the sanitized, bounded response body, or the reason.
	Diagnostic string
}

// Error implements the error interface:
// "selfupdate: <source> http <status>: <diagnostic>".
func (e *HTTPError) Error() string {
	if e == nil {
		return "selfupdate: http error"
	}
	if e.Source == "" {
		return fmt.Sprintf("selfupdate: http %d: %s", e.StatusCode, e.Diagnostic)
	}
	return fmt.Sprintf("selfupdate: %s http %d: %s", e.Source, e.StatusCode, e.Diagnostic)
}

// SymlinkSource says how a refused symlink was found
// (docs/decisions/0041-MADR-refuse-symlink-invocation-on-every-platform.md
// Amendment A1 D3-A).
type SymlinkSource string

// The ways a refused symlink is found.
const (
	// SymlinkSourceExecutable: os.Executable returned the link, as it does
	// on macOS and Windows.
	SymlinkSourceExecutable SymlinkSource = "executable"
	// SymlinkSourceInvocation: os.Args[0] named the link, and os.Executable
	// had already resolved it, as it has on Linux and on any OS whose
	// os.Executable resolves links.
	SymlinkSourceInvocation SymlinkSource = "invocation"
	// SymlinkSourcePolicy: TargetPolicy.ExecutablePath is the link.
	SymlinkSourcePolicy SymlinkSource = "policy"
)

// SymlinkInvocationError is the refusal to update through a symlink.
// errors.Is(err, ErrSymlinkInvocation) holds for it, and errors.As gives
// the link, what it resolves to, and how it was found.
//
// Its message names no Go identifier, since a CLI prints it to a person who
// cannot change the program's configuration. The remedy for the program's
// author is here instead: to update through a link on purpose, set
// TargetPolicy.ExecutablePath to Target. With SymlinkSourcePolicy the
// configured ExecutablePath is itself the link, and the program, or
// whoever set that path, must name the target instead
// (docs/decisions/0041-MADR-refuse-symlink-invocation-on-every-platform.md
// Amendment A1 D3-A, D4-A).
type SymlinkInvocationError struct {
	// Link is the symlink, as an absolute path.
	Link string `json:"link"`
	// Target is the file Link resolves to, as an absolute path, or empty
	// when it does not resolve.
	Target string `json:"target,omitempty"`
	// Source is how Link was found.
	Source SymlinkSource `json:"source"`
}

// Error implements the error interface. After ErrSymlinkInvocation's text it
// is ": <link> is a symlink" when Target is empty; ": the configured
// executable path <link> links to <target>" for SymlinkSourcePolicy; and
// otherwise ": <link> links to <target>; start <target> itself to update
// it".
func (e *SymlinkInvocationError) Error() string {
	prefix := ErrSymlinkInvocation.Error()
	switch {
	case e == nil:
		return prefix
	case e.Target == "":
		return prefix + ": " + e.Link + " is a symlink"
	case e.Source == SymlinkSourcePolicy:
		return prefix + ": the configured executable path " + e.Link + " links to " + e.Target
	default:
		return prefix + ": " + e.Link + " links to " + e.Target + "; start " + e.Target + " itself to update it"
	}
}

// Unwrap reports ErrSymlinkInvocation.
func (e *SymlinkInvocationError) Unwrap() error {
	return ErrSymlinkInvocation
}

// ExitCode maps a Run outcome to the canonical process status. It returns 10
// only when ErrUpdateAvailable is in the error chain, 1 for any other error,
// and 0 otherwise. Package selfupdate never calls os.Exit.
func ExitCode(result Result, err error) int {
	_ = result
	if err == nil {
		return 0
	}
	if errors.Is(err, ErrUpdateAvailable) {
		return 10
	}
	return 1
}

func itoa(v uint64) string {
	return strconv.FormatUint(v, 10)
}
