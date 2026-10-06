package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"regexp"
	"strconv"
	"strings"

	"github.com/maccavelli/go-selfupdate-lib/selfupdate"
	"github.com/maccavelli/go-selfupdate-lib/selfupdate/releasespec"
)

// runners maps each os/arch with a GitHub-hosted runner to its label
// (0013-MADR §5). darwin/amd64 has none.
var runners = map[string]string{
	"linux/amd64":   "ubuntu-24.04",
	"linux/arm64":   "ubuntu-24.04-arm",
	"darwin/arm64":  "macos-15",
	"windows/amd64": "windows-2025",
	"windows/arm64": "windows-11-arm",
}

var (
	shaRe = regexp.MustCompile(`^[0-9a-f]{40}$`)
	// nameRe is the release workflow's asset name rule, for an artifact
	// name and a stamped version.
	nameRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`)
)

func loadSpec(path string) (releasespec.Spec, error) {
	data, err := os.ReadFile(path) //nolint:gosec // the caller's own spec
	if err != nil {
		return releasespec.Spec{}, err
	}
	return releasespec.Parse(data)
}

type planInput struct {
	SpecPath     string
	RefType      string // "tag", or anything else for a rehearsal
	RefName      string
	SHA          string
	RunAttempt   string
	ArtifactName string
}

// identityLeg is one leg of the identity job's matrix.
type identityLeg struct {
	Product string `json:"product"`
	OS      string `json:"os"`
	Arch    string `json:"arch"`
	Runner  string `json:"runner"`
	Asset   string `json:"asset"`
	Args    string `json:"args"` // a JSON array, as a string
}

type planResult struct {
	Tag            string
	Rehearsal      bool
	StampVersion   string
	StampKind      string
	ArtifactName   string
	ProductsJSON   string
	PlatformsJSON  string
	ExtrasJSON     string
	ChannelsJSON   string
	IdentityMatrix []identityLeg
	ToolTargets    []string
	Rows           []planRow
}

// planRow is one product on one platform, for the summary.
type planRow struct {
	Product, Platform, Asset, Identity string
}

func plan(in planInput) (planResult, error) {
	spec, err := loadSpec(in.SpecPath)
	if err != nil {
		return planResult{}, err
	}
	if !shaRe.MatchString(in.SHA) {
		return planResult{}, usagef("-sha %q is not a full commit SHA", in.SHA)
	}
	attempt, err := strconv.Atoi(in.RunAttempt)
	if err != nil || attempt < 1 {
		return planResult{}, usagef("-run-attempt %q is not a positive number", in.RunAttempt)
	}
	r := planResult{StampKind: kindLocal, Rehearsal: true, StampVersion: "rehearsal-" + in.SHA[:12]}
	if in.RefType == "tag" {
		if err := checkTag(spec, in.RefName); err != nil {
			return planResult{}, err
		}
		r.Tag, r.Rehearsal, r.StampVersion, r.StampKind = in.RefName, false, in.RefName, kindRelease
	}
	r.ArtifactName = in.ArtifactName
	if r.ArtifactName == "" {
		r.ArtifactName = fmt.Sprintf("selfupdate-release-%s-%d", r.StampVersion, attempt)
		if r.Rehearsal {
			r.ArtifactName = fmt.Sprintf("selfupdate-rehearsal-%s-%d", in.SHA[:12], attempt)
		}
	}
	if !nameRe.MatchString(r.ArtifactName) {
		return planResult{}, usagef("artifact name %q must match %s", r.ArtifactName, nameRe)
	}
	r.ProductsJSON, r.PlatformsJSON, r.ChannelsJSON = spec.ProductsJSON(), spec.PlatformsJSON(), spec.ChannelsJSON()
	if r.ExtrasJSON, err = spec.ExtrasJSON(r.StampVersion); err != nil {
		return planResult{}, err
	}
	if err := r.planIdentity(spec); err != nil {
		return planResult{}, err
	}
	return r, nil
}

// checkTag applies the client's tag rule with the spec's channels, the rule
// check-release-tag.sh mirrors, so a release stamp is always one buildinfo
// reports as a release.
func checkTag(spec releasespec.Spec, tag string) error {
	o := selfupdate.SemverOptions{}
	if len(spec.PrereleaseChannels) > 0 {
		o = selfupdate.SemverOptions{AllowPrerelease: true, Channels: spec.PrereleaseChannels}
	}
	policy, err := selfupdate.NewSemverPolicy(o)
	if err != nil {
		return err
	}
	if err := policy.Validate(tag); err != nil {
		return fmt.Errorf("tag %q is not an admitted release tag: %w", tag, err)
	}
	return nil
}

func (r *planResult) planIdentity(spec releasespec.Spec) error {
	seen := map[string]bool{}
	r.IdentityMatrix = []identityLeg{}
	r.ToolTargets = []string{}
	for _, prod := range spec.Products {
		for _, p := range spec.Targets() {
			asset, err := spec.AssetName(prod.Name, p)
			if err != nil {
				return err
			}
			row := planRow{Product: prod.Name, Platform: p.OS + "/" + p.Arch, Asset: asset}
			runner, ok := runners[row.Platform]
			switch {
			case len(prod.IdentityArgs) == 0:
				row.Identity = "not run: no identity_args"
			case !ok:
				row.Identity = "not run: no runner for " + row.Platform
			default:
				args, err := json.Marshal(prod.IdentityArgs)
				if err != nil {
					return err
				}
				row.Identity = runner
				r.IdentityMatrix = append(r.IdentityMatrix, identityLeg{
					Product: prod.Name, OS: p.OS, Arch: p.Arch, Runner: runner, Asset: asset, Args: string(args),
				})
				if !seen[row.Platform] {
					seen[row.Platform] = true
					r.ToolTargets = append(r.ToolTargets, row.Platform)
				}
			}
			r.Rows = append(r.Rows, row)
		}
	}
	return nil
}

func (r planResult) outputs() ([]output, error) {
	matrix, err := json.Marshal(r.IdentityMatrix)
	if err != nil {
		return nil, err
	}
	targets, err := json.Marshal(r.ToolTargets)
	if err != nil {
		return nil, err
	}
	return []output{
		{"tag", r.Tag},
		{"rehearsal", strconv.FormatBool(r.Rehearsal)},
		{"stamp-version", r.StampVersion},
		{"stamp-kind", r.StampKind},
		{"artifact-name", r.ArtifactName},
		{"products-json", r.ProductsJSON},
		{"platforms-json", r.PlatformsJSON},
		{"extra-assets-json", r.ExtrasJSON},
		{"prerelease-channels-json", r.ChannelsJSON},
		{"identity-matrix", string(matrix)},
		{"tool-targets", string(targets)},
	}, nil
}

func (r planResult) summary() string {
	var b strings.Builder
	mode := "release " + r.Tag
	if r.Rehearsal {
		mode = "rehearsal (" + r.StampVersion + ", stamped local)"
	}
	fmt.Fprintf(&b, "## Self-update release plan: %s\n\n| Product | Platform | Asset | Identity run |\n| :--- | :--- | :--- | :--- |\n", mode)
	for _, row := range r.Rows {
		fmt.Fprintf(&b, "| %s | %s | `%s` | %s |\n", row.Product, row.Platform, row.Asset, row.Identity)
	}
	return b.String() + "\n"
}

func runPlan(_ context.Context, args []string, stdout io.Writer) error {
	f := newFlags("plan")
	spec := f.str("spec", true)
	refType := f.str("ref-type", true)
	refName := f.str("ref-name", true)
	sha := f.str("sha", true)
	attempt := f.str("run-attempt", true)
	artifact := f.str("artifact-name", false)
	ghOut := f.str("github-output", false)
	summary := f.str("summary", false)
	if err := f.parse(args); err != nil {
		return err
	}
	r, err := plan(planInput{SpecPath: *spec, RefType: *refType, RefName: *refName, SHA: *sha, RunAttempt: *attempt, ArtifactName: *artifact})
	if err != nil {
		return err
	}
	outs, err := r.outputs()
	if err != nil {
		return err
	}
	for _, o := range outs {
		if err := printf(stdout, "%s=%s\n", o.name, o.value); err != nil {
			return err
		}
	}
	if err := writeOutputs(*ghOut, outs); err != nil {
		return err
	}
	return appendSummary(*summary, r.summary())
}
