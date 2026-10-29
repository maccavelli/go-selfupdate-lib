package selfupdate

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"strings"
)

// Plans (docs/decisions/0038-MADR-pinned-plans-lifecycle-conformance-and-bubble-tests.md
// D1-A). A Plan names one release, its assets and its SHA256SUMS entry for
// one request. Updater.Plan makes it; Updater.Apply installs exactly that
// release, or refuses with ErrPlanStale, however long after and whatever
// has been published since. A plan carries no approval.

// PlanSchema is the schema_version of PlanDocument, the only one ParsePlan
// reads.
const PlanSchema = 1

// maxPlanSize is the largest document ParsePlan reads, in bytes.
const maxPlanSize = 64 << 10

// Plan is a resolved update for one request: the request, the release it
// resolved to, the selection, and the binary's SHA256SUMS entry. It is
// comparable. Its fields are what Updater.Plan found; a Plan built or
// changed by hand is checked again by Updater.Apply against the source.
type Plan struct {
	// Product, CurrentVersion, CurrentBuild, Channel and Force are the
	// request's.
	Product        string
	CurrentVersion string
	CurrentBuild   BuildKind
	// Platform is the request's, normalized: the running platform when the
	// request named none.
	Platform Platform
	Channel  string
	// RequestedVersion is the request's TargetVersion: empty when the
	// request asked for the latest release.
	RequestedVersion string
	Force            bool
	// TargetVersion is the resolved release's tag, which Apply fetches.
	TargetVersion string
	// ReleaseID is the resolved release's Release.ID.
	ReleaseID int64
	// ReleaseURL is the resolved release's page.
	ReleaseURL string
	// Latest reports that the release was resolved as the latest, stable
	// or on Channel, so an older one is ErrLatestOlder, not a rollback.
	Latest bool
	// Operation is the classified action.
	Operation Operation
	// Selection is the selected binary, or archive, and SHA256SUMS.
	Selection Selection
	// ManifestSHA256 is the binary's SHA256SUMS entry, lowercase hex. It is
	// empty when Operation is OperationNone: nothing is downloaded.
	ManifestSHA256 string
}

// Plan's equality is what a caller's comparison relies on: a map key must
// be comparable, so a field that is not fails the build here
// (docs/decisions/0038-PLAN-pinned-plans-lifecycle-conformance-and-bubble-tests.md
// Deviation D14).
var _ map[Plan]struct{}

// PlanDocument is the JSON form of a Plan, with stable snake_case keys,
// and the build kind and operation as strings, in ResultDocument's style.
// It carries no Yes and no DryRun, so approval never travels with a plan.
type PlanDocument struct {
	SchemaVersion    int           `json:"schema_version"`
	Product          string        `json:"product"`
	CurrentVersion   string        `json:"current_version"`
	CurrentBuild     string        `json:"current_build"`
	OS               string        `json:"os"`
	Arch             string        `json:"arch"`
	Channel          string        `json:"channel,omitempty"`
	RequestedVersion string        `json:"requested_version,omitempty"`
	Force            bool          `json:"force"`
	TargetVersion    string        `json:"target_version"`
	ReleaseID        int64         `json:"release_id"`
	ReleaseURL       string        `json:"release_url,omitempty"`
	Latest           bool          `json:"latest"`
	Operation        string        `json:"operation"`
	Binary           AssetDocument `json:"binary"`
	Manifest         AssetDocument `json:"manifest"`
	ManifestName     string        `json:"manifest_name"`
	Packed           bool          `json:"packed"`
	ManifestSHA256   string        `json:"manifest_sha256,omitempty"`
}

// AssetDocument is the JSON form of an Asset in a PlanDocument.
type AssetDocument struct {
	ID     int64  `json:"id"`
	Name   string `json:"name"`
	State  string `json:"state"`
	Size   int64  `json:"size"`
	Digest string `json:"digest,omitempty"`
}

// Document returns the JSON form of p. It has no side effects.
func (p Plan) Document() PlanDocument {
	return PlanDocument{
		SchemaVersion:    PlanSchema,
		Product:          p.Product,
		CurrentVersion:   p.CurrentVersion,
		CurrentBuild:     p.CurrentBuild.String(),
		OS:               p.Platform.OS,
		Arch:             p.Platform.Arch,
		Channel:          p.Channel,
		RequestedVersion: p.RequestedVersion,
		Force:            p.Force,
		TargetVersion:    p.TargetVersion,
		ReleaseID:        p.ReleaseID,
		ReleaseURL:       p.ReleaseURL,
		Latest:           p.Latest,
		Operation:        p.Operation.String(),
		Binary:           assetDocument(p.Selection.Binary),
		Manifest:         assetDocument(p.Selection.Manifest),
		ManifestName:     p.Selection.ManifestName,
		Packed:           p.Selection.Packed,
		ManifestSHA256:   p.ManifestSHA256,
	}
}

func assetDocument(a Asset) AssetDocument {
	return AssetDocument(a)
}

// Request returns the request p was made for, without Yes, DryRun or
// CheckOnly: the request a host passes to Apply, with Yes set once it has
// shown the plan and has the user's approval.
func (p Plan) Request() Request {
	return Request{
		Product:        p.Product,
		CurrentVersion: p.CurrentVersion,
		CurrentBuild:   p.CurrentBuild,
		TargetVersion:  p.RequestedVersion,
		Platform:       p.Platform,
		Force:          p.Force,
		Channel:        p.Channel,
	}
}

// ParsePlan reads a PlanDocument and returns its Plan. It refuses more
// than 64 KiB, a field it does not know, anything after the one JSON
// value, a schema_version other than PlanSchema, and a plan that is not
// valid: a product name the Updater refuses, an unknown build kind or
// operation, a missing platform or tag, an ID or a size that is not
// positive, an asset name that is not a basename, or a digest that is not
// SHA-256 hex. It does not consult a source: Apply checks the plan against
// the release.
func ParsePlan(data []byte) (Plan, error) {
	if len(data) > maxPlanSize {
		return Plan{}, fmt.Errorf("selfupdate: plan: the document is %d bytes, more than %d", len(data), maxPlanSize)
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	var d PlanDocument
	if err := dec.Decode(&d); err != nil {
		return Plan{}, fmt.Errorf("selfupdate: plan: %w", err)
	}
	if err := dec.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return Plan{}, errors.New("selfupdate: plan: data after the plan")
	}
	if d.SchemaVersion != PlanSchema {
		return Plan{}, fmt.Errorf("selfupdate: plan: schema_version: %d is not supported; this library reads schema %d", d.SchemaVersion, PlanSchema)
	}
	build, err := parseBuildKind(d.CurrentBuild)
	if err != nil {
		return Plan{}, fmt.Errorf("selfupdate: plan: current_build: %w", err)
	}
	op, err := parseOperation(d.Operation)
	if err != nil {
		return Plan{}, fmt.Errorf("selfupdate: plan: operation: %w", err)
	}
	p := Plan{
		Product:          d.Product,
		CurrentVersion:   d.CurrentVersion,
		CurrentBuild:     build,
		Platform:         Platform{OS: d.OS, Arch: d.Arch},
		Channel:          d.Channel,
		RequestedVersion: d.RequestedVersion,
		Force:            d.Force,
		TargetVersion:    d.TargetVersion,
		ReleaseID:        d.ReleaseID,
		ReleaseURL:       d.ReleaseURL,
		Latest:           d.Latest,
		Operation:        op,
		Selection: Selection{
			Binary:       Asset(d.Binary),
			Manifest:     Asset(d.Manifest),
			ManifestName: d.ManifestName,
			Packed:       d.Packed,
		},
		ManifestSHA256: d.ManifestSHA256,
	}
	if err := p.validate(); err != nil {
		return Plan{}, err
	}
	return p, nil
}

// validate checks what a plan must hold whatever the source says, for
// ParsePlan and for Apply. Each error names the field.
func (p Plan) validate() error {
	invalid := func(field, reason string) error {
		return fmt.Errorf("selfupdate: plan: %s: %s", field, reason)
	}
	if err := validateProduct(p.Product); err != nil {
		return fmt.Errorf("selfupdate: plan: product: %w", err)
	}
	switch {
	case p.CurrentVersion == "":
		return invalid("current_version", "empty")
	case p.CurrentBuild != ReleaseBuild && p.CurrentBuild != LocalBuild:
		return invalid("current_build", "not release or local")
	case p.Platform.OS == "" || p.Platform.Arch == "":
		return invalid("os, arch", "both are required")
	case p.TargetVersion == "":
		return invalid("target_version", "empty")
	case p.RequestedVersion != "" && p.RequestedVersion != p.TargetVersion:
		return invalid("requested_version", "not target_version")
	case p.Latest != (p.RequestedVersion == ""):
		return invalid("latest", "a plan is the latest exactly when no version was requested")
	case p.ReleaseID <= 0:
		return invalid("release_id", "not positive")
	case p.Operation > OperationReplaceLocal:
		return invalid("operation", "unknown")
	}
	for _, a := range []struct {
		field string
		asset Asset
	}{{"binary", p.Selection.Binary}, {"manifest", p.Selection.Manifest}} {
		if err := validateAssetMetadata(a.asset, math.MaxInt64); err != nil {
			return fmt.Errorf("selfupdate: plan: %s: %w", a.field, err)
		}
	}
	if err := validateChecksumName(p.Selection.ManifestName); err != nil {
		return fmt.Errorf("selfupdate: plan: manifest_name: %w", err)
	}
	if p.Operation == OperationNone {
		if p.ManifestSHA256 != "" {
			return invalid("manifest_sha256", "set for operation none, which downloads nothing")
		}
		return nil
	}
	if d, err := parseHexDigest(p.ManifestSHA256); err != nil || d != p.ManifestSHA256 {
		return invalid("manifest_sha256", "not 64 lowercase hex characters")
	}
	return nil
}

// matches refuses a request that is not the one p was made for, naming
// each field that differs. req's platform is normalized.
func (p Plan) matches(req Request) error {
	var diff []string
	for _, f := range []struct {
		name string
		same bool
	}{
		{"product", req.Product == p.Product},
		{"current version", req.CurrentVersion == p.CurrentVersion},
		{"current build", req.CurrentBuild == p.CurrentBuild},
		{"platform", req.Platform == p.Platform},
		{"channel", req.Channel == p.Channel},
		{"requested version", req.TargetVersion == p.RequestedVersion},
		{"force", req.Force == p.Force},
	} {
		if !f.same {
			diff = append(diff, f.name)
		}
	}
	if len(diff) > 0 {
		return fmt.Errorf("selfupdate: the request is not the plan's: its %s differ: %w", strings.Join(diff, ", "), ErrPlanStale)
	}
	return nil
}

// sameRelease refuses a vetted release, selection and operation that are
// not the ones p pinned, naming each difference.
func (p Plan) sameRelease(rel Release, sel Selection, op Operation) error {
	var diff []string
	if rel.ID != p.ReleaseID {
		diff = append(diff, fmt.Sprintf("release ID %d, not %d", rel.ID, p.ReleaseID))
	}
	diff = append(diff, assetDiff("binary", sel.Binary, p.Selection.Binary)...)
	diff = append(diff, assetDiff("manifest", sel.Manifest, p.Selection.Manifest)...)
	if sel.ManifestName != p.Selection.ManifestName {
		diff = append(diff, fmt.Sprintf("manifest name %q, not %q", sanitizeText(sel.ManifestName), sanitizeText(p.Selection.ManifestName)))
	}
	if sel.Packed != p.Selection.Packed {
		diff = append(diff, fmt.Sprintf("packed %t, not %t", sel.Packed, p.Selection.Packed))
	}
	if op != p.Operation {
		diff = append(diff, fmt.Sprintf("operation %s, not %s", op, p.Operation))
	}
	if len(diff) > 0 {
		return fmt.Errorf("selfupdate: release %q no longer matches the plan: %s: %w", rel.Tag, strings.Join(diff, "; "), ErrPlanStale)
	}
	return nil
}

// assetDiff names how got differs from the asset a plan pinned: its ID,
// name and size, and its digest when the plan has one.
func assetDiff(what string, got, want Asset) []string {
	var diff []string
	if got.ID != want.ID {
		diff = append(diff, fmt.Sprintf("%s ID %d, not %d", what, got.ID, want.ID))
	}
	if got.Name != want.Name {
		diff = append(diff, fmt.Sprintf("%s name %q, not %q", what, sanitizeText(got.Name), sanitizeText(want.Name)))
	}
	if got.Size != want.Size {
		diff = append(diff, fmt.Sprintf("%s size %d, not %d", what, got.Size, want.Size))
	}
	if want.Digest != "" && got.Digest != want.Digest {
		diff = append(diff, fmt.Sprintf("%s digest %q, not %q", what, sanitizeText(got.Digest), sanitizeText(want.Digest)))
	}
	return diff
}

// sameEntry refuses a SHA256SUMS entry for name other than the one p
// pinned. It is stale and an integrity failure both: the release is
// immutable, so its manifest should not have changed.
func (p Plan) sameEntry(name, digest string) error {
	if digest == p.ManifestSHA256 {
		return nil
	}
	return fmt.Errorf("selfupdate: SHA256SUMS entry for %s is %s, not the plan's %s: %w",
		sanitizeText(name), digest, p.ManifestSHA256, errors.Join(ErrPlanStale, ErrIntegrity))
}

// pinned is discovery for Apply: the release tagged p.TargetVersion,
// vetted as Plan vetted it, with p.Latest, then compared with p. Errors
// are returned unwrapped.
func (c *Checker) pinned(ctx context.Context, req Request, p Plan) (Release, Selection, Operation, error) {
	rel, err := c.fetchTag(ctx, p.TargetVersion)
	if err != nil {
		return Release{}, Selection{}, OperationNone, err
	}
	rel, sel, op, err := c.vet(req, rel, p.Latest)
	if err != nil {
		return Release{}, Selection{}, OperationNone, err
	}
	if err := p.sameRelease(rel, sel, op); err != nil {
		return Release{}, Selection{}, OperationNone, err
	}
	return rel, sel, op, nil
}

// Plan resolves req to a Plan without installing anything: the checks Run
// makes before its confirmation, then, unless the operation is
// OperationNone, the SHA256SUMS download within Limits.Manifest, the
// binary's entry and the ManifestVerifiers, which Run makes after it. It
// reports no events, takes no lock, resolves no target and downloads no
// binary, and it does not share Run's guard. opts apply as they do to
// RunWith; WithCredentials reaches a private repository, and the
// reporter, confirmer and progress options have nothing to act on. A
// request with CheckOnly is refused: Checker.Check is the check. Yes and
// DryRun are not recorded
// (docs/decisions/0038-MADR-pinned-plans-lifecycle-conformance-and-bubble-tests.md
// D1-A).
//
// Errors: those of Run before its confirmation, and its manifest and
// manifest verifier errors; an invalid option's, which comes first; and
// ErrNotConstructed.
func (u *Updater) Plan(ctx context.Context, req Request, opts ...RunOption) (Plan, error) {
	if !u.constructed() {
		return Plan{}, ErrNotConstructed
	}
	s, err := scope(opts)
	if err != nil {
		return Plan{}, err
	}
	r, err := u.newRun(s)
	if err != nil {
		return Plan{}, err
	}
	return r.plan(withRunMark(ctx), req)
}

func (u *run) plan(ctx context.Context, req Request) (Plan, error) {
	if err := validateRequest(req, u.versions, u.runningPlatform()); err != nil {
		if validateProduct(req.Product) != nil {
			// An invalid product name is not safe to put in the prefix.
			return Plan{}, err
		}
		return Plan{}, wrapRun(req, err)
	}
	if req.CheckOnly {
		return Plan{}, wrapRun(req, errors.New("selfupdate: a plan is for an update; use Checker.Check for a check"))
	}
	req.Platform = normalizePlatform(req.Platform, u.runningPlatform())
	c := u.checker()
	rel, fromLatest, err := c.fetchRelease(ctx, req)
	if err != nil {
		return Plan{}, wrapRun(req, err)
	}
	rel, sel, op, err := c.vet(req, rel, fromLatest)
	if err != nil {
		return Plan{}, wrapRun(req, err)
	}
	p := Plan{
		Product:          req.Product,
		CurrentVersion:   req.CurrentVersion,
		CurrentBuild:     req.CurrentBuild,
		Platform:         req.Platform,
		Channel:          req.Channel,
		RequestedVersion: req.TargetVersion,
		Force:            req.Force,
		TargetVersion:    rel.Tag,
		ReleaseID:        rel.ID,
		ReleaseURL:       rel.URL,
		Latest:           fromLatest,
		Operation:        op,
		Selection:        sel,
	}
	if op == OperationNone {
		return p, nil
	}
	if p.ManifestSHA256, err = u.manifestEntry(ctx, req, rel, sel, nil); err != nil {
		return Plan{}, wrapRun(req, err)
	}
	return p, nil
}
