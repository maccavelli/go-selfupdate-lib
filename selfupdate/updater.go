package selfupdate

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"time"
)

// errNotCommitted reports an Installer that returned neither a committed
// replacement nor an error (0003-MADR C2).
var errNotCommitted = errors.New("selfupdate: installer reported no committed replacement")

// New constructs an Updater. Source, Versions, Assets, Installer, Reporter,
// and Confirmer are required, and none may be a typed nil. An empty
// Verifiers slice is valid, but no element may be nil. A nil Transformer is
// a no-op; a typed-nil Transformer is rejected.
func New(cfg Config) (*Updater, error) {
	if isNil(cfg.Source) {
		return nil, fmt.Errorf("selfupdate: source is required")
	}
	if isNil(cfg.Versions) {
		return nil, fmt.Errorf("selfupdate: version policy is required")
	}
	if isNil(cfg.Assets) {
		return nil, fmt.Errorf("selfupdate: asset selector is required")
	}
	if isNil(cfg.Installer) {
		return nil, fmt.Errorf("selfupdate: installer is required")
	}
	if isNil(cfg.Reporter) {
		return nil, fmt.Errorf("selfupdate: reporter is required")
	}
	if isNil(cfg.Confirmer) {
		return nil, fmt.Errorf("selfupdate: confirmer is required")
	}
	for i, v := range cfg.Verifiers {
		if isNil(v) {
			return nil, fmt.Errorf("selfupdate: verifier %d is nil", i)
		}
	}
	for i, v := range cfg.ManifestVerifiers {
		if isNil(v) {
			return nil, fmt.Errorf("selfupdate: manifest verifier %d is nil", i)
		}
	}
	for i, p := range cfg.Probes {
		if isNil(p) {
			return nil, fmt.Errorf("selfupdate: probe %d is nil", i)
		}
	}
	if cfg.Transformer != nil && isNil(cfg.Transformer) {
		return nil, fmt.Errorf("selfupdate: transformer is a typed nil")
	}
	if err := cfg.Limits.valid(); err != nil {
		return nil, err
	}
	if cfg.ProgressInterval < 0 {
		return nil, fmt.Errorf("selfupdate: progress interval must not be negative")
	}
	transformer := cfg.Transformer
	if transformer == nil {
		transformer = noopTransformer{}
	}
	verifiers := append([]Verifier(nil), cfg.Verifiers...)
	return &Updater{
		source:      cfg.Source,
		versions:    cfg.Versions,
		assets:      cfg.Assets,
		verifiers:   verifiers,
		transformer: transformer,
		installer:   cfg.Installer,
		reporter:    cfg.Reporter,
		confirmer:   cfg.Confirmer,
		limits:      cfg.Limits,
		progress:    cfg.ProgressInterval,
		manifestVfy: append([]ManifestVerifier(nil), cfg.ManifestVerifiers...),
		probes:      append([]Prober(nil), cfg.Probes...),
	}, nil
}

// Run executes one self-update request. The library never calls os.Exit.
// It is RunWith with no options.
func (u *Updater) Run(ctx context.Context, req Request) (Result, error) {
	return u.RunWith(ctx, req)
}

func (u *run) execute(ctx context.Context, req Request) (res Result, err error) {
	// Each run resolves its own negative credential outcome (0010-MADR A2).
	ctx = withRunMark(ctx)
	// A failed run ends with EventFailed, except when the product name is
	// not safe to report or check mode found an update (0004-MADR G5, A7).
	defer func() {
		if err == nil || errors.Is(err, ErrUpdateAvailable) || validateProduct(req.Product) != nil {
			return
		}
		u.reportOutcome(ctx, Event{
			Kind: EventFailed, Product: req.Product, Current: req.CurrentVersion,
			Target: res.TargetVersion, Asset: res.AssetName, Detail: failureClass(err),
		})
	}()
	if err := validateRequest(req, u.versions); err != nil {
		if validateProduct(req.Product) != nil {
			// An invalid product name is not safe to put in the prefix.
			return Result{}, err
		}
		return Result{}, wrapRun(req, err)
	}
	req.Platform = normalizePlatform(req.Platform)
	if err := u.report(ctx, Event{Kind: EventResolvingTarget, Product: req.Product, Current: req.CurrentVersion}); err != nil {
		return Result{}, wrapRun(req, err)
	}
	target, err := u.installer.ResolveTarget(ctx)
	if err != nil {
		return Result{}, wrapRun(req, err)
	}
	if err := u.report(ctx, Event{Kind: EventFetchingRelease, Product: req.Product, Current: req.CurrentVersion, Target: req.TargetVersion}); err != nil {
		return Result{}, wrapRun(req, err)
	}
	// Discovery is shared with Checker.Check, so Run and Check cannot
	// disagree (0004-MADR G3).
	rel, sel, op, err := u.checker().discover(ctx, req)
	if err != nil {
		return Result{}, wrapRun(req, err)
	}
	result := Result{
		Product:        req.Product,
		CurrentVersion: req.CurrentVersion,
		TargetVersion:  rel.Tag,
		ReleaseURL:     rel.URL,
		AssetName:      sel.Binary.Name,
		Operation:      op,
		DryRun:         req.DryRun,
	}
	selected := Event{
		Kind: EventSelected, Product: req.Product, Current: req.CurrentVersion,
		Target: rel.Tag, Asset: sel.Binary.Name,
	}
	if req.CheckOnly && op == OperationReplaceLocal {
		selected.Detail = "local build: apply requires --force"
	}
	if err := u.report(ctx, selected); err != nil {
		return Result{}, wrapRun(req, err)
	}
	if req.CheckOnly {
		result.Checked = true
		if op == OperationNone {
			return result, nil
		}
		return result, ErrUpdateAvailable
	}
	if op == OperationNone {
		return result, nil
	}
	// A dry run changes nothing, so it needs no approval.
	if !req.Yes && !req.DryRun {
		ok, err := u.confirmer.Confirm(ctx, Prompt{
			Product: req.Product, Current: req.CurrentVersion, Target: rel.Tag, Operation: op,
		})
		if err != nil {
			return result, wrapRun(req, err)
		}
		if !ok {
			result.Declined = true
			u.reportOutcome(ctx, Event{
				Kind: EventDeclined, Product: req.Product, Current: req.CurrentVersion,
				Target: rel.Tag, Asset: sel.Binary.Name,
			})
			return result, nil
		}
	}
	return u.apply(ctx, req, result, target, rel, sel)
}

func (u *run) apply(ctx context.Context, req Request, result Result, target Target, rel Release, sel Selection) (resultOut Result, err error) {
	resultOut = result
	sess, err := u.installer.Begin(ctx, target)
	if err != nil {
		return resultOut, wrapRun(req, err)
	}
	// Close runs exactly once: explicitly before the terminal event on the
	// success path (PLAN §4.6 step 15), otherwise from this safety net.
	closed := false
	closeSession := func() error {
		if closed {
			return nil
		}
		closed = true
		return sess.Close()
	}
	defer func() {
		if cerr := closeSession(); cerr != nil {
			err = errors.Join(err, wrapRun(req, cerr))
		}
	}()
	if rerr := u.report(ctx, Event{Kind: EventDownloadingManifest, Product: req.Product, Target: rel.Tag, Asset: sel.Manifest.Name}); rerr != nil {
		return resultOut, wrapRun(req, rerr)
	}
	var manifestBuf bytes.Buffer
	if _, err = downloadAsset(ctx, u.source, rel, sel.Manifest, &manifestBuf, u.limits.Manifest); err != nil {
		return resultOut, wrapRun(req, err)
	}
	// Parse the manifest and find the one required entry before any staging
	// or binary download (PLAN §4.6 step 9).
	entries, err := ParseSHA256SUMS(manifestBuf.Bytes())
	if err != nil {
		return resultOut, wrapRun(req, err)
	}
	manifestDigest, err := checksumFor(entries, sel.ManifestName)
	if err != nil {
		return resultOut, wrapRun(req, err)
	}
	// Manifest verifiers run before staging exists and before any binary
	// byte is fetched (0004-MADR G9).
	if err = u.runManifestVerifiers(ctx, req, rel, sel, manifestBuf.Bytes()); err != nil {
		return resultOut, wrapRun(req, err)
	}
	if rerr := u.report(ctx, Event{Kind: EventDownloadingBinary, Product: req.Product, Target: rel.Tag, Asset: sel.Binary.Name, Bytes: sel.Binary.Size}); rerr != nil {
		return resultOut, wrapRun(req, rerr)
	}
	f, stagedPath, err := sess.CreateStaging(ctx)
	if err != nil {
		return resultOut, wrapRun(req, err)
	}
	progress := u.newProgress(ctx, req, rel, sel)
	var staging io.Writer = f
	if progress != nil {
		staging = &progressWriter{w: f, done: progress.written}
	}
	releaseDigest, err := downloadAsset(ctx, u.source, rel, sel.Binary, staging, u.limits.Executable)
	closeErr := f.Close()
	if err != nil {
		return resultOut, wrapRun(req, errors.Join(err, closeErr))
	}
	if closeErr != nil {
		return resultOut, wrapRun(req, closeErr)
	}
	if progress != nil {
		progress.finish()
	}
	ghDigest := ""
	if sel.Binary.Digest != "" {
		ghDigest, err = parseGitHubDigest(sel.Binary.Digest)
		if err != nil {
			return resultOut, wrapRun(req, err)
		}
	}
	if err = verifyIntegrity(Verification{
		Product: req.Product, Release: rel, Selection: sel,
		Size: sel.Binary.Size, SHA256: releaseDigest, ManifestSHA256: manifestDigest, GitHubSHA256: ghDigest,
	}); err != nil {
		return resultOut, wrapRun(req, err)
	}
	if err = u.runVerifiers(ctx, req, rel, sel, stagedPath, releaseDigest, manifestDigest, ghDigest); err != nil {
		return resultOut, wrapRun(req, err)
	}
	// Verified precedes the optional transform (the EventKind order, and
	// PLAN §4.6 step 12); the transformed staging is revalidated below.
	if rerr := u.report(ctx, Event{Kind: EventVerified, Product: req.Product, Target: rel.Tag, Asset: sel.Binary.Name}); rerr != nil {
		return resultOut, wrapRun(req, rerr)
	}
	installedDigest := releaseDigest
	// The staged size is the advertised one, which downloadAsset enforced,
	// until a transform changes it (0004-MADR G8).
	installedSize := sel.Binary.Size
	if _, isNoop := u.transformer.(noopTransformer); !isNoop {
		if rerr := u.report(ctx, Event{Kind: EventTransforming, Product: req.Product, Asset: sel.Binary.Name}); rerr != nil {
			return resultOut, wrapRun(req, rerr)
		}
		if err = u.transformer.Transform(ctx, TransformRequest{
			Product: req.Product, Platform: req.Platform, Path: stagedPath, ReleaseDigest: releaseDigest,
		}); err != nil {
			return resultOut, wrapRun(req, err)
		}
		installedDigest, installedSize, err = hashAndValidateStaging(sess, stagedPath, u.limits.Executable)
		if err != nil {
			return resultOut, wrapRun(req, err)
		}
	}
	if err = u.runProbes(ctx, req, rel, stagedPath); err != nil {
		return resultOut, wrapRun(req, err)
	}
	if req.DryRun {
		// Everything short of Install has run. Closing the session
		// removes the staging file (0004-MADR G11).
		resultOut.ReleaseDigest = releaseDigest
		resultOut.InstalledDigest = installedDigest
		closeErr = closeSession()
		repErr := u.report(ctx, Event{
			Kind: EventComplete, Product: req.Product, Current: req.CurrentVersion,
			Target: rel.Tag, Asset: sel.Binary.Name, Detail: "dry run: verified, nothing installed",
		})
		return resultOut, wrapRun(req, errors.Join(closeErr, repErr))
	}
	if rerr := u.report(ctx, Event{Kind: EventInstalling, Product: req.Product, Target: rel.Tag, Asset: sel.Binary.Name}); rerr != nil {
		return resultOut, wrapRun(req, rerr)
	}
	installed, instErr := sess.Install(ctx, InstallRequest{
		Product:       req.Product,
		TargetVersion: rel.Tag,
		Artifact: StagedArtifact{
			Path: stagedPath, Size: installedSize,
			ReleaseDigest: releaseDigest, InstalledDigest: installedDigest,
		},
	})
	resultOut.ReleaseDigest = releaseDigest
	resultOut.InstalledDigest = installedDigest
	resultOut.ServiceInstalled = installed.ServiceInstalled
	resultOut.ServiceWasRunning = installed.ServiceWasRunning
	resultOut.ServiceStarted = installed.ServiceStarted
	resultOut.PendingBackup = installed.PendingBackup
	resultOut.Previous = installed.Previous
	if installed.RolledBack {
		u.reportOutcome(ctx, Event{Kind: EventRolledBack, Product: req.Product, Target: rel.Tag, Asset: sel.Binary.Name})
	}
	if !installed.Applied {
		if instErr == nil {
			instErr = errNotCommitted
		}
		if installed.Backup != "" {
			// The replacement is live and restoring the previous binary
			// failed: the backup is the only copy of it, so the caller
			// must learn where it is (0004-MADR R1).
			resultOut.PendingBackup = installed.Backup
			instErr = errors.Join(instErr, fmt.Errorf(
				"selfupdate: the previous binary was kept at %s because restoring it failed; restore or remove it",
				sanitizeText(retainedPath(target, installed.Backup))))
		}
		return resultOut, wrapRun(req, instErr)
	}
	resultOut.Applied = true
	if instErr != nil && resultOut.PendingBackup == "" && installed.Backup != "" {
		// A failed cleanup after the commit can leave the backup beside the
		// new binary: report it while it is there (0010-MADR B8).
		if _, err := os.Lstat(retainedPath(target, installed.Backup)); err == nil {
			resultOut.PendingBackup = installed.Backup
		}
	}
	// Release the session (and its lock) before the terminal event, so a
	// Close failure is joined with the committed result rather than surfacing
	// after "complete" has been reported.
	closeErr = closeSession()
	detail := "release asset integrity verified"
	if resultOut.PendingBackup != "" {
		detail = "pending backup " + sanitizeText(retainedPath(target, resultOut.PendingBackup)) +
			" will be validated and removed before the next download"
	}
	repErr := u.report(ctx, Event{
		Kind: EventComplete, Product: req.Product, Current: req.CurrentVersion,
		Target: rel.Tag, Asset: sel.Binary.Name, Detail: detail,
	})
	return resultOut, wrapRun(req, errors.Join(instErr, closeErr, repErr))
}

// retainedPath makes an installer-reported backup path absolute. The
// standalone installer reports an absolute path; a custom Installer may
// report a name relative to the target directory (0003-PLAN deviation D2).
func retainedPath(target Target, p string) string {
	if filepath.IsAbs(p) {
		return p
	}
	return filepath.Join(target.Dir, p)
}

// runProbes runs Config.Probes on the staging file, first making it
// runnable: CreateTemp creates it 0600 (0004-MADR G9).
func (u *Updater) runProbes(ctx context.Context, req Request, rel Release, stagedPath string) error {
	if len(u.probes) == 0 {
		return nil
	}
	if err := os.Chmod(stagedPath, 0o700); err != nil { //nolint:gosec // G302: the staged binary must be executable to probe it
		return fmt.Errorf("selfupdate: make staging runnable: %w", err)
	}
	for _, p := range u.probes {
		if err := p.Probe(ctx, ProbeRequest{
			Product: req.Product, TargetVersion: rel.Tag, Path: stagedPath, Phase: ProbeStaged,
		}); err != nil {
			return fmt.Errorf("selfupdate: staged binary failed a probe: %w", err)
		}
	}
	return nil
}

func (u *run) runVerifiers(ctx context.Context, req Request, rel Release, sel Selection, path, digest, manifestDigest, ghDigest string) error {
	for _, v := range u.verifiers {
		err := v.Verify(ctx, Verification{
			Product: req.Product, Release: rel, Selection: sel,
			Size: sel.Binary.Size, SHA256: digest, ManifestSHA256: manifestDigest, GitHubSHA256: ghDigest,
			Open: func() (io.ReadCloser, error) {
				return openAbsFile(path, os.O_RDONLY, 0)
			},
			OpenAsset: u.openAsset(rel),
		})
		if err != nil {
			// As runManifestVerifiers does: a verifier's refusal is an
			// integrity failure (0010-MADR A8).
			return errors.Join(ErrIntegrity, err)
		}
	}
	return nil
}

func (u *run) report(ctx context.Context, ev Event) error {
	return u.reporter.Report(ctx, ev)
}

// reportOutcome delivers an advisory event: one that reports something
// that has already happened. It is delivered even after the caller's
// cancellation, and a reporter error on it is ignored (0004-MADR A7).
func (u *run) reportOutcome(ctx context.Context, ev Event) {
	advisory(u.reporter.Report(context.WithoutCancel(ctx), ev))
}

// advisory is where an error goes that must not change the outcome: a
// reporter error on an advisory event (0004-MADR amendments A1 and A7),
// or the close of a response discarded for a retry. It is dropped here,
// on purpose.
func advisory(_ error) {}

// failureClasses maps a run's error to EventFailed's Detail. The first
// match wins, so cancellation outranks whatever it interrupted.
var failureClasses = []struct {
	err   error
	class string
}{
	{context.Canceled, "canceled"},
	{context.DeadlineExceeded, "deadline-exceeded"},
	{ErrConfirmationRequired, "confirmation-required"},
	{ErrForceRequired, "force-required"},
	{ErrLatestOlder, "latest-older"},
	{ErrMutableRelease, "mutable-release"},
	{ErrRateLimited, "rate-limited"},
	{ErrUnsupportedPlatform, "unsupported-platform"},
	{ErrConcurrentUpdate, "concurrent-update"},
	{ErrManagedInstall, "managed-install"},
	{ErrIntegrity, "integrity"},
}

func failureClass(err error) string {
	for _, c := range failureClasses {
		if errors.Is(err, c.err) {
			return c.class
		}
	}
	return "error"
}

// downloadProgress throttles EventProgress for one binary download
// (0004-MADR G5, amendment A1).
type downloadProgress struct {
	u        *run
	ctx      context.Context
	template Event
	last     time.Time
}

// newProgress returns nil when progress is disabled. Otherwise it reports
// the first event, at zero bytes.
func (u *run) newProgress(ctx context.Context, req Request, rel Release, sel Selection) *downloadProgress {
	if u.progress <= 0 {
		return nil
	}
	p := &downloadProgress{u: u, ctx: ctx, template: Event{
		Kind: EventProgress, Product: req.Product, Target: rel.Tag, Asset: sel.Binary.Name, Total: sel.Binary.Size,
	}}
	p.emit(0)
	p.last = timeNow()
	return p
}

func (p *downloadProgress) emit(done int64) {
	ev := p.template
	ev.Bytes = done
	// Progress is advisory: a reporter error never fails the download.
	advisory(p.u.reporter.Report(p.ctx, ev))
}

// written reports the running total, at most once per interval. The
// total itself is left to finish.
func (p *downloadProgress) written(done int64) {
	if done >= p.template.Total {
		return
	}
	if now := timeNow(); now.Sub(p.last) >= p.u.progress {
		p.last = now
		p.emit(done)
	}
}

func (p *downloadProgress) finish() {
	p.emit(p.template.Total)
}

// progressWriter counts bytes written to the staging file.
type progressWriter struct {
	w    io.Writer
	n    int64
	done func(int64)
}

func (p *progressWriter) Write(b []byte) (int, error) {
	n, err := p.w.Write(b)
	p.n += int64(n)
	p.done(p.n)
	return n, err
}

// hashAndValidateStaging returns the transformed staging file's digest and
// size.
func hashAndValidateStaging(sess InstallSession, path string, limit int64) (string, int64, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return "", 0, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return "", 0, fmt.Errorf("selfupdate: transformed staging is not a regular file")
	}
	if info.Size() > limit {
		return "", 0, fmt.Errorf("selfupdate: transformed staging exceeds executable limit")
	}
	if !sessOwns(sess, path) {
		return "", 0, fmt.Errorf("selfupdate: transformed staging is not owned by the session")
	}
	sum, err := hashFile(path)
	if err != nil {
		return "", 0, err
	}
	return sum, info.Size(), nil
}

// sessOwns fails closed: a session that is not a StagingOwner owns no
// staging path (0004-MADR G7).
func sessOwns(sess InstallSession, path string) bool {
	o, ok := sess.(StagingOwner)
	return ok && o.Owns(path)
}

func hashFile(path string) (digest string, err error) {
	f, err := openAbsFile(path, os.O_RDONLY, 0)
	if err != nil {
		return "", err
	}
	defer func() {
		err = joinClose(err, f)
	}()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// isNil reports an untyped nil or an interface holding a nil pointer, map,
// slice, func, channel or interface (0003-MADR C3).
func isNil(v any) bool {
	if v == nil {
		return true
	}
	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Pointer, reflect.Map, reflect.Slice, reflect.Func, reflect.Chan, reflect.Interface:
		return rv.IsNil()
	default:
		return false
	}
}

func normalizePlatform(p Platform) Platform {
	if p.OS == "" && p.Arch == "" {
		return Platform{OS: runtime.GOOS, Arch: runtime.GOARCH}
	}
	return p
}

func wrapRun(req Request, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("selfupdate: %s: %w", req.Product, err)
}
