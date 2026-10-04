package selfupdate

import (
	"fmt"
	"regexp"
	"strings"

	"golang.org/x/mod/semver"
)

var (
	productRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`)
	versionRe = regexp.MustCompile(`^v(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)$`)
	// channelNameRe and prereleaseNumRe are the two halves of a -NAME.N
	// prerelease suffix (0005-MADR §1, amendment E2).
	channelNameRe   = regexp.MustCompile(`^[a-z][a-z0-9]{0,15}$`)
	prereleaseNumRe = regexp.MustCompile(`^(0|[1-9]\d*)$`)
)

type strictVersionPolicy struct{}

// NewStrictVersionPolicy returns the canonical stable-tag policy. Tags must
// match vMAJOR.MINOR.PATCH with no leading zeroes, prerelease, or build
// metadata.
func NewStrictVersionPolicy() VersionPolicy {
	return strictVersionPolicy{}
}

// Validate implements VersionPolicy.
func (strictVersionPolicy) Validate(tag string) error {
	if !versionRe.MatchString(tag) {
		return fmt.Errorf("selfupdate: %q is not a strict stable tag", tag)
	}
	if !semver.IsValid(tag) {
		return fmt.Errorf("selfupdate: %q is not a strict stable tag", tag)
	}
	return nil
}

// Compare implements VersionPolicy. Both arguments are validated first.
func (p strictVersionPolicy) Compare(a, b string) (int, error) {
	if err := p.Validate(a); err != nil {
		return 0, err
	}
	if err := p.Validate(b); err != nil {
		return 0, err
	}
	return semver.Compare(a, b), nil
}

// SemverOptions configure NewSemverPolicy.
type SemverOptions struct {
	// AllowPrerelease admits vMAJOR.MINOR.PATCH-NAME.N tags, where NAME is
	// one of Channels and N is a decimal with no leading zero.
	AllowPrerelease bool
	// Channels lists the prerelease names, most stable first, in strictly
	// descending ASCII order, for example {"rc", "beta", "alpha"}. The
	// order makes SemVer precedence, which compares names lexically, agree
	// with stability. Each name matches ^[a-z][a-z0-9]{0,15}$.
	Channels []string
}

type semverPolicy struct {
	allow bool
	rank  map[string]int // channel name to its index, most stable first
}

// NewSemverPolicy returns a policy for strict vMAJOR.MINOR.PATCH tags and,
// with AllowPrerelease, -NAME.N prereleases on the named channels. Build
// metadata is never accepted: it does not take part in precedence, so two
// tags that differ only in it would compare equal. The returned policy is a
// ChannelPolicy (0005-MADR §1, amendments E1 and E2).
func NewSemverPolicy(o SemverOptions) (VersionPolicy, error) {
	if !o.AllowPrerelease {
		if len(o.Channels) > 0 {
			return nil, fmt.Errorf("selfupdate: semver policy: channels need AllowPrerelease")
		}
		return &semverPolicy{}, nil
	}
	if len(o.Channels) == 0 {
		return nil, fmt.Errorf("selfupdate: semver policy: AllowPrerelease needs at least one channel")
	}
	p := &semverPolicy{allow: true, rank: make(map[string]int, len(o.Channels))}
	for i, name := range o.Channels {
		if !channelNameRe.MatchString(name) {
			return nil, fmt.Errorf("selfupdate: semver policy: channel name %q must match %s", name, channelNameRe)
		}
		if _, dup := p.rank[name]; dup {
			return nil, fmt.Errorf("selfupdate: semver policy: channel %q is listed twice", name)
		}
		if i > 0 && o.Channels[i-1] <= name {
			return nil, fmt.Errorf("selfupdate: semver policy: channel %q must come after %q: list channels in descending ASCII order, most stable first, so version precedence agrees with stability",
				o.Channels[i-1], name)
		}
		p.rank[name] = i
	}
	return p, nil
}

// Validate implements VersionPolicy.
func (p *semverPolicy) Validate(tag string) error {
	core, pre, hasPre := strings.Cut(tag, "-")
	if !versionRe.MatchString(core) {
		return fmt.Errorf("selfupdate: %q is not a strict release tag", tag)
	}
	if hasPre {
		if !p.allow {
			return fmt.Errorf("selfupdate: %q is not a strict stable tag", tag)
		}
		name, num, ok := strings.Cut(pre, ".")
		if _, known := p.rank[name]; !ok || !known || !prereleaseNumRe.MatchString(num) {
			return fmt.Errorf("selfupdate: %q is not an admitted prerelease tag", tag)
		}
	}
	if !semver.IsValid(tag) {
		return fmt.Errorf("selfupdate: %q is not a valid release tag", tag)
	}
	return nil
}

// Compare implements VersionPolicy. Both arguments are validated first.
func (p *semverPolicy) Compare(a, b string) (int, error) {
	if err := p.Validate(a); err != nil {
		return 0, err
	}
	if err := p.Validate(b); err != nil {
		return 0, err
	}
	return semver.Compare(a, b), nil
}

// ValidChannel implements ChannelPolicy.
func (p *semverPolicy) ValidChannel(name string) error {
	if name == "" {
		return nil
	}
	if _, ok := p.rank[name]; !ok {
		return fmt.Errorf("selfupdate: channel %q is not offered by the version policy", name)
	}
	return nil
}

// Admits implements ChannelPolicy. A stable tag is on every channel; a
// prerelease is on a channel at most as stable as its own name, so "beta"
// admits rc and beta builds but not alpha. The stable channel admits no
// prerelease.
func (p *semverPolicy) Admits(channel, tag string) bool {
	pre := semver.Prerelease(tag)
	if pre == "" {
		return true
	}
	name, _, _ := strings.Cut(strings.TrimPrefix(pre, "-"), ".")
	tagRank, tagKnown := p.rank[name]
	chRank, chKnown := p.rank[channel]
	return tagKnown && chKnown && tagRank <= chRank
}

func validateProduct(product string) error {
	if !productRe.MatchString(product) {
		return fmt.Errorf("selfupdate: invalid product name %q", product)
	}
	return nil
}

// validateRequest checks a request's shape, and validates its versions with
// the configured policy rather than a hardcoded strict one (0004-MADR G2).
func validateRequest(req Request, versions VersionPolicy) error {
	if err := validateProduct(req.Product); err != nil {
		return err
	}
	switch req.CurrentBuild {
	case ReleaseBuild, LocalBuild:
	default:
		return fmt.Errorf("selfupdate: unknown build kind %s", req.CurrentBuild)
	}
	if (req.Platform.OS == "") != (req.Platform.Arch == "") {
		return fmt.Errorf("selfupdate: platform OS and architecture must both be set or both empty")
	}
	if req.CheckOnly && req.Yes {
		return fmt.Errorf("selfupdate: --check and --yes are contradictory")
	}
	if req.CheckOnly && req.Force {
		return fmt.Errorf("selfupdate: --check and --force are contradictory")
	}
	if req.CheckOnly && req.DryRun {
		return fmt.Errorf("selfupdate: --check and --dry-run are contradictory")
	}
	// An apply replaces the running binary, so it must be built for the
	// running platform. A check or a dry run may ask about another one
	// (0010-MADR Q4).
	if !req.CheckOnly && !req.DryRun && req.Platform != (Platform{}) {
		if running := runningPlatform(); req.Platform != running {
			return fmt.Errorf("selfupdate: cannot apply a %s/%s release on %s/%s: %w",
				sanitizeText(req.Platform.OS), sanitizeText(req.Platform.Arch), running.OS, running.Arch, ErrUnsupportedPlatform)
		}
	}
	if req.CurrentBuild == ReleaseBuild {
		if err := versions.Validate(req.CurrentVersion); err != nil {
			return err
		}
	}
	if req.TargetVersion != "" {
		if err := versions.Validate(req.TargetVersion); err != nil {
			return err
		}
	}
	return validateChannel(req, versions)
}

// validateChannel checks Request.Channel against the policy. Under a
// ChannelPolicy it also refuses a pinned prerelease unless the request names
// a channel that admits it (0005-MADR §2, Q4). A plain VersionPolicy keeps
// deciding which tags it accepts, as 0004-MADR G2 promises (0005-PLAN
// deviation D3).
func validateChannel(req Request, versions VersionPolicy) error {
	cp, isChannelPolicy := versions.(ChannelPolicy)
	if req.Channel != "" {
		if !isChannelPolicy {
			return fmt.Errorf("selfupdate: channel %q is not offered by the version policy", req.Channel)
		}
		if err := cp.ValidChannel(req.Channel); err != nil {
			return err
		}
	}
	if isChannelPolicy && req.TargetVersion != "" && semver.Prerelease(req.TargetVersion) != "" {
		if req.Channel == "" || !cp.Admits(req.Channel, req.TargetVersion) {
			return fmt.Errorf("selfupdate: %s is a prerelease; request a channel that admits it", req.TargetVersion)
		}
	}
	return nil
}

// classifyOperation reports the action for a validated request and already
// validated selected tag. fromLatest is true when the selection was the
// latest stable release rather than an exact --version.
func classifyOperation(versions VersionPolicy, req Request, selected string, fromLatest bool) (Operation, error) {
	if err := versions.Validate(selected); err != nil {
		return OperationNone, err
	}
	if req.CurrentBuild == LocalBuild {
		if req.CheckOnly {
			return OperationReplaceLocal, nil
		}
		if !req.Force {
			return OperationNone, fmt.Errorf("%w to install %s", ErrForceRequired, selected)
		}
		return OperationReplaceLocal, nil
	}
	cmp, err := versions.Compare(req.CurrentVersion, selected)
	if err != nil {
		return OperationNone, err
	}
	switch {
	case cmp < 0:
		return OperationUpgrade, nil
	case cmp == 0:
		if req.Force && !req.CheckOnly {
			return OperationReinstall, nil
		}
		return OperationNone, nil
	default:
		if fromLatest {
			return OperationNone, fmt.Errorf("selfupdate: latest release %s is older than running %s: %w",
				selected, req.CurrentVersion, ErrLatestOlder)
		}
		return OperationRollback, nil
	}
}
