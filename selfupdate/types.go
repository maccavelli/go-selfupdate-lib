package selfupdate

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"sync/atomic"
	"time"
)

// BuildKind states whether the running binary is a published release or a
// local development build. Release comparison never infers this from version
// text.
type BuildKind uint8

const (
	// BuildUnknown is the zero value and is not a valid Request.CurrentBuild.
	BuildUnknown BuildKind = iota
	// ReleaseBuild is a binary stamped from a strict vMAJOR.MINOR.PATCH tag.
	ReleaseBuild
	// LocalBuild is any non-release identity, including dirty VCS trees.
	LocalBuild
)

// String implements fmt.Stringer.
func (k BuildKind) String() string {
	switch k {
	case BuildUnknown:
		return "unknown"
	case ReleaseBuild:
		return "release"
	case LocalBuild:
		return "local"
	default:
		return "buildkind(" + itoa(uint64(k)) + ")"
	}
}

// Platform is a GOOS/GOARCH pair. The zero value means the runtime platform.
type Platform struct {
	// OS is a GOOS value such as "linux". Empty only when Arch is also empty.
	OS string
	// Arch is a GOARCH value such as "amd64". Empty only when OS is also empty.
	Arch string
}

// Request is one self-update invocation. The library never reads global flag
// state or process arguments.
type Request struct {
	// Product is the executable basename used in exact asset names.
	Product string
	// CurrentVersion is the running identity. For ReleaseBuild it must be a
	// tag the configured VersionPolicy validates: under
	// NewStrictVersionPolicy, a strict vMAJOR.MINOR.PATCH. For LocalBuild it
	// is not ordered.
	CurrentVersion string
	// CurrentBuild distinguishes release and local binaries.
	CurrentBuild BuildKind
	// TargetVersion selects an exact tag, which the VersionPolicy must
	// validate. Under a ChannelPolicy, a prerelease tag also needs a Channel
	// that admits it. Empty means the latest release on Channel: with no
	// Channel, the latest stable release.
	TargetVersion string
	// Platform selects the asset matrix entry. Zero means runtime GOOS/GOARCH.
	Platform Platform
	// CheckOnly performs discovery and policy evaluation without download.
	CheckOnly bool
	// Force permits replacing a local build or reinstalling the selected
	// version. It never bypasses validation, integrity, or target policy.
	Force bool
	// Yes approves the already-selected operation without prompting.
	Yes bool
	// DryRun downloads, verifies, transforms and probes the release, then
	// discards it without prompting: nothing is installed.
	DryRun bool
	// Channel selects a release channel offered by a ChannelPolicy, such as
	// "rc" or "beta". Empty is the stable channel, which never installs a
	// prerelease (0005-MADR §2).
	Channel string
}

// Operation is the classified action for a request and selected release.
type Operation uint8

const (
	// OperationNone means the running identity already equals the selection.
	OperationNone Operation = iota
	// OperationUpgrade installs a higher stable release.
	OperationUpgrade
	// OperationReinstall replaces the same release version under --force.
	OperationReinstall
	// OperationRollback installs an exact lower stable tag.
	OperationRollback
	// OperationReplaceLocal replaces a local build with a stable release.
	OperationReplaceLocal
)

// String implements fmt.Stringer.
func (o Operation) String() string {
	switch o {
	case OperationNone:
		return "none"
	case OperationUpgrade:
		return "upgrade"
	case OperationReinstall:
		return "reinstall"
	case OperationRollback:
		return "rollback"
	case OperationReplaceLocal:
		return "replace-local"
	default:
		return "operation(" + itoa(uint64(o)) + ")"
	}
}

// Result is the typed outcome of Updater.Run. The library never calls os.Exit.
type Result struct {
	// Product is the requested product name.
	Product string
	// CurrentVersion is the running identity supplied in the request.
	CurrentVersion string
	// TargetVersion is the selected release tag.
	TargetVersion string
	// ReleaseURL is the selected GitHub release HTML URL when known.
	ReleaseURL string
	// AssetName is the exact selected executable asset name.
	AssetName string
	// Operation is the classified action.
	Operation Operation
	// Checked is true when the run was check-only.
	Checked bool
	// Applied is true when a healthy installation committed.
	Applied bool
	// Declined is true when the user declined an interactive apply.
	Declined bool
	// ReleaseDigest is the verified SHA-256 hex of the release bytes.
	ReleaseDigest string
	// InstalledDigest is the SHA-256 hex of the bytes after any transform.
	InstalledDigest string
	// ServiceInstalled reports whether a managed definition existed.
	ServiceInstalled bool
	// ServiceWasRunning reports whether that definition's process was active.
	ServiceWasRunning bool
	// ServiceStarted reports whether the update started the service, as
	// InstallResult.ServiceStarted does.
	ServiceStarted bool
	// PendingBackup is a backup of the previous binary left beside the
	// target. With Applied true, it is the Windows running-image backup the
	// active image kept open after commit; it is validated and removed
	// before the next download. With Applied false, on any OS, the new
	// binary is live and restoring the previous one failed; the backup is
	// the only copy of the previous binary, and the caller must restore or
	// remove it.
	PendingBackup string
	// DryRun echoes Request.DryRun: the release was checked and nothing
	// was installed.
	DryRun bool
	// Previous is where InstallOptions.KeepPrevious kept the previous
	// binary.
	Previous string
	// Warnings lists the errors that arrived after EventComplete. None of
	// them failed the run.
	Warnings Warnings
}

// Repository is a GitHub owner/name pair.
type Repository struct {
	// Owner is the GitHub account or organization.
	Owner string
	// Name is the repository name.
	Name string
}

// GitHubOptions construct a GitHubSource. There is no convenience constructor
// that fills a client, token, or limits.
type GitHubOptions struct {
	// Repository is the GitHub owner/name pair.
	Repository Repository
	// Client is required. NewGitHubSource clones it and never mutates the
	// caller's value.
	Client *http.Client
	// APIBaseURL is the GitHub API origin. Nil means https://api.github.com.
	APIBaseURL *url.URL
	// UserAgent is the required product/version User-Agent.
	UserAgent string
	// Token is an explicit GitHub token. Empty falls back to GH_TOKEN then
	// GITHUB_TOKEN.
	Token string
	// Limits bound JSON, error, manifest, and executable bodies.
	Limits Limits
	// Credentials is asked lazily, on the first API request, after Token
	// and before GH_TOKEN and GITHUB_TOKEN (0004-MADR G10). Nil keeps the
	// v1.0 behaviour.
	Credentials CredentialProvider
	// Observer learns when a credential was accepted by a successful
	// API response. It is called once per source.
	Observer CredentialObserver
}

// Release is one GitHub release after JSON decoding and field validation.
type Release struct {
	// ID is the GitHub release identifier.
	ID int64
	// Tag is the release tag name.
	Tag string
	// URL is the HTML URL for the release.
	URL string
	// Draft reports whether the release is still a draft.
	Draft bool
	// Prerelease reports whether GitHub marked the release as a prerelease.
	Prerelease bool
	// Immutable reports whether GitHub marked the release immutable.
	Immutable bool
	// Assets are the release assets as returned by the API.
	Assets []Asset
}

// Asset is one GitHub release asset.
type Asset struct {
	// ID is the GitHub asset identifier used for API downloads.
	ID int64
	// Name is the exact asset filename.
	Name string
	// State is the GitHub asset state; uploaded is required to install.
	State string
	// Size is the advertised byte length.
	Size int64
	// Digest is GitHub's sha256:<hex> field when supplied.
	Digest string
}

// Selection is the exact executable and checksum manifest chosen from a release.
type Selection struct {
	// Binary is the exact platform executable asset.
	Binary Asset
	// Manifest is the exact SHA256SUMS asset.
	Manifest Asset
	// ManifestName is the binary basename looked up in SHA256SUMS.
	ManifestName string
}

// ReleaseSource discovers releases and opens asset bodies.
type ReleaseSource interface {
	Latest(context.Context) (Release, error)
	ByTag(context.Context, string) (Release, error)
	OpenAsset(context.Context, Release, Asset) (io.ReadCloser, error)
}

// AssetSelector chooses exactly one platform binary and the SHA256SUMS asset.
type AssetSelector interface {
	Select(Release, string, Platform) (Selection, error)
}

// VersionPolicy validates and compares strict stable release tags.
type VersionPolicy interface {
	Validate(string) error
	Compare(string, string) (int, error)
}

// ListOptions bound ReleaseLister.ListReleases.
type ListOptions struct {
	// Limit is how many releases to consider: zero means 90, and more than
	// 300 is refused. GitHub lists newest first in practice but does not
	// document an order, so a release beyond the limit may be missed; that
	// costs an update, never a wrong one, because discovery checks its
	// choice in full (0005-MADR §3, amendments E3 and E5).
	Limit int
}

// ReleaseLister is a ReleaseSource that can list releases, prereleases
// included, which discovery needs for Request.Channel: GitHub's latest
// release is never a prerelease. Entries may be drafts or malformed;
// discovery decides (0005-MADR §3).
type ReleaseLister interface {
	ListReleases(ctx context.Context, o ListOptions) ([]Release, error)
}

// ChannelPolicy is a VersionPolicy that offers release channels. The
// empty channel is stable. ValidChannel reports whether a request may name
// a channel, and Admits whether a tag the policy validated belongs on it.
// Discovery uses it for Request.Channel; with any other policy every
// non-empty channel is refused (0005-MADR, amendment E1).
type ChannelPolicy interface {
	VersionPolicy
	ValidChannel(name string) error
	Admits(channel, tag string) bool
}

// Verification is the input to a Verifier. Open returns a fresh read-only
// descriptor of the staged bytes; it is not a writable staging path.
type Verification struct {
	// Product is the requested product name.
	Product string
	// Release is the selected release metadata.
	Release Release
	// Selection is the selected binary and manifest.
	Selection Selection
	// Size is the staged byte length.
	Size int64
	// SHA256 is the staged content digest.
	SHA256 string
	// ManifestSHA256 is the digest from the SHA256SUMS entry.
	ManifestSHA256 string
	// GitHubSHA256 is the digest from GitHub asset metadata when supplied.
	GitHubSHA256 string
	// Open returns a new reader over the staged bytes.
	Open func() (io.ReadCloser, error)
	// OpenAsset opens another asset of the same release by exact name,
	// within limit bytes, enforcing its advertised size and digest. A
	// signature verifier uses it to fetch a detached signature
	// (0004-MADR G9).
	OpenAsset func(ctx context.Context, name string, limit int64) (io.ReadCloser, error)
}

// Verifier is an additional integrity or authenticity check.
type Verifier interface {
	Verify(context.Context, Verification) error
}

// TransformRequest describes a consumer-authorized post-verification change
// such as macOS codesigning.
type TransformRequest struct {
	// Product is the requested product name.
	Product string
	// Platform is the selected platform.
	Platform Platform
	// Path is the locked staging path.
	Path string
	// ReleaseDigest is the verified pre-transform digest.
	ReleaseDigest string
}

// Transformer applies an authorized post-verification change. The coordinator
// recomputes InstalledDigest itself after Transform returns.
type Transformer interface {
	Transform(context.Context, TransformRequest) error
}

// StagedArtifact is a verified staging file owned by one InstallSession.
type StagedArtifact struct {
	// Path is the absolute staging path.
	Path string
	// Size is the staged byte length after any transform.
	Size int64
	// ReleaseDigest is the verified pre-transform digest.
	ReleaseDigest string
	// InstalledDigest is the post-transform digest.
	InstalledDigest string
}

// TargetPolicy controls which executable path may be replaced.
type TargetPolicy struct {
	// ExecutablePath overrides os.Executable when non-empty.
	ExecutablePath string
	// AllowedRoots are additive canonical directories besides the user's
	// home directory. An empty slice adds nothing.
	AllowedRoots []string
}

// Target is a resolved executable path. Callers treat values as opaque and
// must not construct or modify them.
type Target struct {
	// Path is the absolute resolved executable path.
	Path string
	// Dir is the absolute resolved parent directory.
	Dir string
	// Base is the executable basename.
	Base string

	identity fileIdentity
}

type fileIdentity struct {
	info  os.FileInfo
	size  int64
	mtime int64
}

// InstallRequest is the payload for InstallSession.Install.
type InstallRequest struct {
	// Product is the requested product name.
	Product string
	// Artifact is the session-owned staged binary.
	Artifact StagedArtifact
	// TargetVersion is the release tag being installed.
	TargetVersion string
}

// InstallResult is the outcome of InstallSession.Install.
type InstallResult struct {
	// Target is the replaced executable path.
	Target string
	// Backup is the retained rollback path before commit cleanup.
	Backup string
	// Applied is true when replacement committed healthily.
	Applied bool
	// ServiceInstalled reports whether a managed definition existed.
	ServiceInstalled bool
	// ServiceWasRunning reports whether that definition's process was active.
	ServiceWasRunning bool
	// ServiceStarted reports whether the update started the service: it was
	// running, or it was stopped and EnabledLifecycle reported it
	// configured to start. A stopped service that is not stays stopped.
	ServiceStarted bool
	// PendingBackup is the path of the Windows running-image backup when it
	// could not be removed after commit.
	PendingBackup string
	// RolledBack reports that the installer restored the previous binary
	// itself after a failure.
	RolledBack bool
	// Previous is where the previous binary was kept at commit, when the
	// installer keeps it.
	Previous string
}

// Installer resolves the target and begins a locked install session.
type Installer interface {
	ResolveTarget(context.Context) (Target, error)
	Begin(context.Context, Target) (InstallSession, error)
}

// InstallSession owns the per-target lock, anchored directory, staging names,
// and cleanup through Close.
type InstallSession interface {
	Target() Target
	CreateStaging(context.Context) (*os.File, string, error)
	Install(context.Context, InstallRequest) (InstallResult, error)
	Close() error
}

// StagingOwner reports whether a session created a staging path. The
// coordinator asks it before hashing a transformed staging file; a session
// that does not implement it owns nothing (0004-MADR G7).
type StagingOwner interface {
	Owns(path string) bool
}

// AppliedReplacement is a replacement that Apply made live and that
// Commit or Rollback has not yet finished.
type AppliedReplacement struct {
	// Target is the replaced executable path.
	Target string
	// Backup is the previous binary's path, or "" when no backup exists.
	Backup string
	// State is private to the session that made the replacement; it is
	// returned to that session's Commit or Rollback unchanged.
	State any
}

// TwoPhaseSession is an InstallSession that a ManagedInstaller can drive:
// Apply makes the replacement live with a backup, and Commit or Rollback
// finishes it once the service has been reconciled, restarted and checked
// (0004-MADR G7).
type TwoPhaseSession interface {
	InstallSession
	StagingOwner
	Apply(context.Context, InstallRequest) (AppliedReplacement, error)
	Commit(context.Context, AppliedReplacement) (InstallResult, error)
	Rollback(context.Context, AppliedReplacement) error
}

// Lifecycle is the consumer-owned service control seam.
type Lifecycle interface {
	Installed(context.Context, string) (bool, error)
	Running(context.Context, string) (bool, error)
	Stop(context.Context, string) error
	Start(context.Context, string) error
	WaitHealthy(context.Context, string) error
}

// EnabledLifecycle is a Lifecycle that can report whether a service is
// configured to start: systemd is-enabled, launchd RunAtLoad or KeepAlive,
// a Windows SCM automatic start type. A managed update starts a service
// that was stopped only when Enabled reports true; without this interface,
// it starts only a service that was running (0010-MADR Q2).
type EnabledLifecycle interface {
	Enabled(context.Context, string) (bool, error)
}

// ReconcileResult is the restoration receipt for a definition change.
type ReconcileResult struct {
	// Changed reports whether the definition was rewritten.
	Changed bool
	// Detail is a short human-readable description.
	Detail string
	// State holds consumer restoration data.
	State any
}

// Reconciler rewrites an existing managed definition and can restore it.
type Reconciler interface {
	Reconcile(ctx context.Context, product, executable string) (ReconcileResult, error)
	Restore(ctx context.Context, product string, receipt ReconcileResult) error
}

// InstallOptions configure standalone target resolution and locking.
type InstallOptions struct {
	// TargetPolicy selects the executable and allowed roots.
	TargetPolicy TargetPolicy
	// LockTimeout bounds lock acquisition and, on Windows, the retry of a
	// replacement the running image refuses while it is busy. Zero selects
	// DefaultLockTimeout.
	LockTimeout time.Duration
	// PostInstall, when set, runs the installed binary after the
	// replacement and before it is committed; a failure rolls the
	// replacement back (0004-MADR G9).
	PostInstall Prober
	// KeepPrevious keeps the previous binary at .<base>.previous beside
	// the target at commit, replacing an older one, instead of removing
	// it (0004-MADR G11).
	KeepPrevious bool
}

// DefaultLockTimeout is the lock acquisition bound when InstallOptions leaves
// LockTimeout unset.
const DefaultLockTimeout time.Duration = 5 * time.Second

const goosWindows = "windows"

// EventKind is a structured progress event.
type EventKind uint8

const (
	// EventUnknown is the zero value and is never emitted.
	EventUnknown EventKind = iota
	// EventResolvingTarget is emitted before network work.
	EventResolvingTarget
	// EventFetchingRelease is emitted before release discovery.
	EventFetchingRelease
	// EventSelected is emitted after exact asset selection.
	EventSelected
	// EventDownloadingManifest is emitted before the SHA256SUMS body.
	EventDownloadingManifest
	// EventDownloadingBinary is emitted before the executable body.
	EventDownloadingBinary
	// EventVerified is emitted after integrity checks succeed.
	EventVerified
	// EventTransforming is emitted before an authorized transform.
	EventTransforming
	// EventInstalling is emitted immediately before Installer.Install.
	EventInstalling
	// EventComplete is emitted after a healthy committed installation, and
	// at the end of a dry run, whose Detail is "dry-run". It is the run's
	// one terminal event: a later error is an EventWarning, not a failure.
	EventComplete
)

// Event kinds added in v1.1.0, appended so every earlier value keeps its
// number (0004-MADR G5).
const (
	// EventProgress reports download progress: Bytes so far of Total. It
	// is emitted only when Config.ProgressInterval is positive, and a
	// reporter error on it is ignored.
	EventProgress EventKind = iota + EventComplete + 1
	// EventDeclined is emitted when the Confirmer declines. A reporter
	// error on it is ignored.
	EventDeclined
	// EventFailed is the last event of a failed run; Detail is the error
	// class. A reporter error on it is ignored.
	EventFailed
	// EventRolledBack is emitted when the installer restored the previous
	// binary itself. A reporter error on it is ignored.
	EventRolledBack
)

// Event kinds added in v1.6.0, appended so every earlier value keeps its
// number (0010-MADR Q3).
const (
	// EventWarning follows EventComplete, once for each error that arrived
	// after the run had done its work: a failed Close, a failed report of
	// complete, or an installer that committed and still returned an
	// error. Detail is the sanitized error, which Result.Warnings also
	// lists. The run does not fail. A reporter error on it is ignored.
	EventWarning EventKind = iota + EventRolledBack + 1
)

// String implements fmt.Stringer.
func (k EventKind) String() string {
	switch k {
	case EventUnknown:
		return "unknown"
	case EventResolvingTarget:
		return "resolving-target"
	case EventFetchingRelease:
		return "fetching-release"
	case EventSelected:
		return "selected"
	case EventDownloadingManifest:
		return "downloading-manifest"
	case EventDownloadingBinary:
		return "downloading-binary"
	case EventVerified:
		return "verified"
	case EventTransforming:
		return "transforming"
	case EventInstalling:
		return "installing"
	case EventComplete:
		return "complete"
	case EventProgress:
		return "progress"
	case EventDeclined:
		return "declined"
	case EventFailed:
		return "failed"
	case EventRolledBack:
		return "rolled-back"
	case EventWarning:
		return "warning"
	default:
		return "eventkind(" + itoa(uint64(k)) + ")"
	}
}

// Event is one structured progress report.
type Event struct {
	// Kind identifies the stage.
	Kind EventKind
	// Product is the requested product name.
	Product string
	// Current is the running identity.
	Current string
	// Target is the selected release tag.
	Target string
	// Asset is the selected executable name when known.
	Asset string
	// Bytes is a size or progress count when meaningful.
	Bytes int64
	// Detail is additional sanitized text.
	Detail string
	// Total is the advertised byte length for EventProgress, and zero
	// otherwise.
	Total int64
}

// Reporter receives structured progress. Implementations must not read or
// write global standard streams unless the consumer injected them.
type Reporter interface {
	Report(context.Context, Event) error
}

// Prompt is the confirmation request for an already-selected operation.
type Prompt struct {
	// Product is the requested product name.
	Product string
	// Current is the running identity.
	Current string
	// Target is the selected release tag.
	Target string
	// Operation is the classified action.
	Operation Operation
}

// Confirmer approves or declines an already-selected apply.
type Confirmer interface {
	Confirm(context.Context, Prompt) (bool, error)
}

// Limits bound remote bodies. Zero values are invalid at Updater construction.
type Limits struct {
	// ReleaseJSON is the maximum GitHub release JSON body in bytes.
	ReleaseJSON int64
	// ErrorBody is the maximum error diagnostic body in bytes.
	ErrorBody int64
	// Manifest is the maximum SHA256SUMS body in bytes.
	Manifest int64
	// Executable is the maximum executable body in bytes.
	Executable int64
}

// DefaultLimits returns the canonical v1 body limits.
func DefaultLimits() Limits {
	return Limits{
		ReleaseJSON: 2 << 20,
		ErrorBody:   64 << 10,
		Manifest:    1 << 20,
		Executable:  512 << 20,
	}
}

func (l Limits) valid() error {
	if l.ReleaseJSON <= 0 || l.ErrorBody <= 0 || l.Manifest <= 0 || l.Executable <= 0 {
		return fmt.Errorf("selfupdate: limits must be positive")
	}
	return nil
}

// Config is the explicit Updater composition. There is no convenience
// constructor that fills security-relevant defaults.
type Config struct {
	// Source discovers releases.
	Source ReleaseSource
	// Versions validates and compares tags.
	Versions VersionPolicy
	// Assets selects exact raw-binary names.
	Assets AssetSelector
	// Verifiers run after built-in integrity checks. An empty slice is valid.
	Verifiers []Verifier
	// Transformer is an optional post-verification change. Nil is a no-op.
	Transformer Transformer
	// Installer owns target resolution and replacement.
	Installer Installer
	// Reporter receives structured progress.
	Reporter Reporter
	// Confirmer approves interactive applies.
	Confirmer Confirmer
	// Limits bound remote bodies. The Updater applies Manifest and
	// Executable. ReleaseJSON and ErrorBody must be valid here too, but they
	// take effect only through the ReleaseSource's own limits, for example
	// GitHubOptions.Limits.
	Limits Limits
	// ProgressInterval is the minimum time between EventProgress reports
	// during the binary download. Zero reports no progress (0004-MADR
	// amendment A1); a negative value is invalid.
	ProgressInterval time.Duration
	// ManifestVerifiers run in order after SHA256SUMS is downloaded and
	// parsed, before the binary is downloaded. A failure aborts the run
	// and matches ErrIntegrity (0004-MADR G9). No element may be nil.
	ManifestVerifiers []ManifestVerifier
	// Probes run the verified, runnable staging file, in order, after any
	// transform and before anything is replaced (0004-MADR G9). No element
	// may be nil.
	Probes []Prober
}

// Updater is the coordinator. Unexported collaborator fields are populated
// by New and are immutable afterwards.
type Updater struct {
	source      ReleaseSource
	versions    VersionPolicy
	assets      AssetSelector
	verifiers   []Verifier
	transformer Transformer
	installer   Installer
	reporter    Reporter
	confirmer   Confirmer
	limits      Limits
	running     atomic.Bool
	progress    time.Duration
	manifestVfy []ManifestVerifier
	probes      []Prober
}
