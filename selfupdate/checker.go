package selfupdate

import (
	"context"
	"fmt"

	"golang.org/x/mod/semver"
)

// CheckerConfig composes a Checker: the discovery half of Config, with no
// Installer, Reporter or Confirmer
// (docs/decisions/0004-MADR-evolve-selfupdate-api-and-tui-support.md G3).
type CheckerConfig struct {
	// Source discovers releases.
	Source ReleaseSource
	// Versions validates and compares tags.
	Versions VersionPolicy
	// Assets selects exact raw-binary names.
	Assets AssetSelector
	// Limits bound the selected assets' advertised sizes.
	Limits Limits
}

// Checker answers "is there an update?" without resolving, locking or
// touching a target, prompting, reporting, or downloading an asset body. It
// holds no mutable state and is safe for concurrent use.
type Checker struct {
	source   ReleaseSource
	versions VersionPolicy
	assets   AssetSelector
	limits   Limits
	// unpacker and fromUpdater: a Checker an Updater made checks the
	// selection against the Updater's Unpacker, as Run does (0015-MADR C4).
	unpacker    Unpacker
	fromUpdater bool
}

// CheckRequest is one availability question.
type CheckRequest struct {
	// Product is the executable basename used in exact asset names.
	Product string
	// CurrentVersion is the running identity. For ReleaseBuild it must
	// satisfy the configured VersionPolicy.
	CurrentVersion string
	// CurrentBuild distinguishes release and local binaries.
	CurrentBuild BuildKind
	// TargetVersion selects an exact tag. Empty means the latest stable
	// release.
	TargetVersion string
	// Platform selects the asset matrix entry. Zero means runtime GOOS/GOARCH.
	Platform Platform
	// Channel selects a release channel, as Request.Channel does. It is part
	// of CheckCached's key, so each channel keeps its own answer.
	Channel string
}

// Availability is the answer to a CheckRequest. Its names match Result
// (0004-MADR amendment A3).
type Availability struct {
	// Product is the requested product name.
	Product string
	// CurrentVersion is the running identity supplied in the request.
	CurrentVersion string
	// TargetVersion is the selected release tag.
	TargetVersion string
	// ReleaseURL is the selected release's HTML URL when known.
	ReleaseURL string
	// AssetName is the exact selected executable asset name.
	AssetName string
	// Operation is the action an apply would take.
	Operation Operation
	// Available is true when Operation is not OperationNone.
	Available bool
	// ForceRequired is true when applying needs Request.Force: the running
	// binary is a local build.
	ForceRequired bool
}

// NewChecker constructs a Checker. Source, Versions and Assets are required,
// and none may be a typed nil; Limits must be valid.
func NewChecker(cfg CheckerConfig) (*Checker, error) {
	if isNil(cfg.Source) {
		return nil, fmt.Errorf("selfupdate: source is required")
	}
	if isNil(cfg.Versions) {
		return nil, fmt.Errorf("selfupdate: version policy is required")
	}
	if isNil(cfg.Assets) {
		return nil, fmt.Errorf("selfupdate: asset selector is required")
	}
	if err := cfg.Limits.valid(); err != nil {
		return nil, err
	}
	return &Checker{source: cfg.Source, versions: cfg.Versions, assets: cfg.Assets, limits: cfg.Limits}, nil
}

// Checker returns a Checker that shares the Updater's source, version
// policy, asset selector, unpacker and limits: it fails a selection the
// Updater's Unpacker cannot handle, as Run does.
func (u *Updater) Checker() *Checker {
	return &Checker{source: u.source, versions: u.versions, assets: u.assets, limits: u.limits,
		unpacker: u.unpacker, fromUpdater: true}
}

// Check reports whether an update is available. Unlike Run with CheckOnly,
// it resolves no target and needs no Installer, Confirmer or Reporter, and
// it reports availability as a value rather than as ErrUpdateAvailable. A
// latest release older than the running one is ErrLatestOlder, as in Run.
func (c *Checker) Check(ctx context.Context, cr CheckRequest) (Availability, error) {
	req, err := c.prepare(cr)
	if err != nil {
		return Availability{}, err
	}
	return c.checkPrepared(withRunMark(ctx), req)
}

// prepare validates a CheckRequest as a check-only Request and normalizes
// its platform. Errors are wrapped as Run wraps them.
func (c *Checker) prepare(cr CheckRequest) (Request, error) {
	req := Request{
		Product:        cr.Product,
		CurrentVersion: cr.CurrentVersion,
		CurrentBuild:   cr.CurrentBuild,
		TargetVersion:  cr.TargetVersion,
		Platform:       cr.Platform,
		CheckOnly:      true,
		Channel:        cr.Channel,
	}
	if err := validateRequest(req, c.versions); err != nil {
		if validateProduct(req.Product) != nil {
			// An invalid product name is not safe to put in the prefix.
			return Request{}, err
		}
		return Request{}, wrapRun(req, err)
	}
	req.Platform = normalizePlatform(req.Platform)
	return req, nil
}

func (c *Checker) checkPrepared(ctx context.Context, req Request) (Availability, error) {
	rel, sel, op, err := c.discover(ctx, req)
	if err != nil {
		return Availability{}, wrapRun(req, err)
	}
	available := op != OperationNone
	return Availability{
		Product:        req.Product,
		CurrentVersion: req.CurrentVersion,
		TargetVersion:  rel.Tag,
		ReleaseURL:     rel.URL,
		AssetName:      sel.Binary.Name,
		Operation:      op,
		Available:      available,
		ForceRequired:  available && req.CurrentBuild == LocalBuild,
	}, nil
}

// discover fetches the release for a validated request with a normalized
// platform, applies every release and asset check, and classifies the
// operation. Run and Check share it, so they cannot disagree. Errors are
// returned unwrapped.
func (c *Checker) discover(ctx context.Context, req Request) (Release, Selection, Operation, error) {
	rel, fromLatest, err := c.fetchRelease(ctx, req)
	if err != nil {
		return Release{}, Selection{}, OperationNone, err
	}
	// PLAN §4.6 step 4 order: immutable, then state, then tag. The tag is
	// untrusted until Validate passes, so it is always quoted.
	if !rel.Immutable {
		return Release{}, Selection{}, OperationNone, fmt.Errorf("selfupdate: release %q is not immutable: %w", rel.Tag, ErrMutableRelease)
	}
	if rel.Draft || (rel.Prerelease && req.Channel == "") {
		return Release{}, Selection{}, OperationNone, fmt.Errorf("selfupdate: release %q is not a stable published release", rel.Tag)
	}
	if err := c.versions.Validate(rel.Tag); err != nil {
		return Release{}, Selection{}, OperationNone, err
	}
	if err := c.checkChannel(req.Channel, rel); err != nil {
		return Release{}, Selection{}, OperationNone, err
	}
	sel, err := c.assets.Select(rel, req.Product, req.Platform)
	if err != nil {
		return Release{}, Selection{}, OperationNone, err
	}
	// Only the selected binary and manifest are checked for state, size and
	// digest syntax, each against its own limit, and before check mode can
	// report anything (PLAN §4.6 step 4; 0003-MADR A1 and A5).
	if err := validateAssetMetadata(sel.Binary, c.limits.Executable); err != nil {
		return Release{}, Selection{}, OperationNone, err
	}
	if err := validateAssetMetadata(sel.Manifest, c.limits.Manifest); err != nil {
		return Release{}, Selection{}, OperationNone, err
	}
	op, err := classifyOperation(c.versions, req, rel.Tag, fromLatest)
	if err != nil {
		return Release{}, Selection{}, OperationNone, err
	}
	// A selector and an unpacker that disagree fail here, before any asset
	// is downloaded, on a check too (0012-MADR §2), and so in an Updater's
	// Checker as in its Run (0015-MADR C4).
	if c.fromUpdater {
		if err := c.matchUnpacker(sel); err != nil {
			return Release{}, Selection{}, OperationNone, err
		}
	}
	return rel, sel, op, nil
}

// matchUnpacker fails a selection the configured Unpacker cannot handle: an
// archive with no unpacker, or an unpacker with a selection that is not an
// archive.
func (c *Checker) matchUnpacker(sel Selection) error {
	switch {
	case sel.Packed && c.unpacker == nil:
		return fmt.Errorf("selfupdate: asset %q is an archive and no Unpacker is configured", sanitizeText(sel.Binary.Name))
	case !sel.Packed && c.unpacker != nil:
		return fmt.Errorf("selfupdate: an Unpacker is configured, but asset %q is not marked as an archive", sanitizeText(sel.Binary.Name))
	}
	return nil
}

// checkChannel applies a ChannelPolicy's checks to a release, on every
// channel, the stable one included. Its prerelease flag must agree with its
// tag: an immutable release's flag can still be edited, and its tag cannot,
// so a disagreement means the flag was changed after publication
// (0005-MADR §3). A named channel must admit the tag. A plain policy has no
// channels and decides for itself (0004-MADR G2; 0005-PLAN deviation D4).
func (c *Checker) checkChannel(channel string, rel Release) error {
	cp, ok := c.versions.(ChannelPolicy)
	if !ok {
		if channel != "" {
			return fmt.Errorf("selfupdate: release %q is not on channel %q", rel.Tag, channel)
		}
		return nil
	}
	if (semver.Prerelease(rel.Tag) != "") != rel.Prerelease {
		return fmt.Errorf("selfupdate: release %q has a prerelease flag that disagrees with its tag", rel.Tag)
	}
	if channel != "" && !cp.Admits(channel, rel.Tag) {
		return fmt.Errorf("selfupdate: release %q is not on channel %q", rel.Tag, channel)
	}
	return nil
}

// channelRelease is discovery on a channel: the highest admissible release
// the source lists, chosen on its tag and flags alone, then checked in full.
// A chosen release that fails a check is an error, never a reason to
// install an older one (0005-MADR §3, amendment E5).
func (c *Checker) channelRelease(ctx context.Context, channel string) (Release, error) {
	lister, ok := c.source.(ReleaseLister)
	if !ok {
		return Release{}, fmt.Errorf("selfupdate: channel %q needs a source that lists releases", channel)
	}
	cp, ok := c.versions.(ChannelPolicy)
	if !ok {
		return Release{}, fmt.Errorf("selfupdate: channel %q is not offered by the version policy", channel)
	}
	rels, err := lister.ListReleases(ctx, ListOptions{})
	if err != nil {
		return Release{}, err
	}
	best := -1
	for i, r := range rels {
		if r.Draft || cp.Validate(r.Tag) != nil || c.checkChannel(channel, r) != nil {
			continue
		}
		if best >= 0 {
			if cmp, err := cp.Compare(r.Tag, rels[best].Tag); err != nil || cmp <= 0 {
				continue
			}
		}
		best = i
	}
	if best < 0 {
		return Release{}, fmt.Errorf("selfupdate: no release on channel %q", channel)
	}
	if err := validateReleaseStructure(rels[best]); err != nil {
		return Release{}, fmt.Errorf("selfupdate: release %q: %w", rels[best].Tag, err)
	}
	return rels[best], nil
}

func (c *Checker) fetchRelease(ctx context.Context, req Request) (Release, bool, error) {
	if req.Channel != "" && req.TargetVersion == "" {
		rel, err := c.channelRelease(ctx, req.Channel)
		return rel, true, err
	}
	if req.TargetVersion == "" {
		rel, err := c.source.Latest(ctx)
		return rel, true, err
	}
	rel, err := c.source.ByTag(ctx, req.TargetVersion)
	if err != nil {
		return Release{}, false, err
	}
	if rel.Tag != req.TargetVersion {
		return Release{}, false, fmt.Errorf("selfupdate: source returned release %q for requested %q: %w",
			rel.Tag, req.TargetVersion, ErrIntegrity)
	}
	return rel, false, nil
}
