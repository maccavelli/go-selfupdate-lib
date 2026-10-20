// Package buildinfo owns the build stamps every program links in, and decides
// from them alone whether the running binary is a published release
// (docs/decisions/0004-MADR-evolve-selfupdate-api-and-tui-support.md §5,
// amendments F1 and F2).
//
// A release build carries two -X stamps, which LDFlags returns as one
// -ldflags fragment; the build workflow adds -s -w to them:
//
//	-X github.com/maccavelli/go-selfupdate-lib/buildinfo.version=v1.2.3
//	-X github.com/maccavelli/go-selfupdate-lib/buildinfo.kind=release
//
// Identity reports KindRelease only when the stamped kind is exactly
// "release" and the stamped version is a release tag, vMAJOR.MINOR.PATCH or
// vMAJOR.MINOR.PATCH-NAME.N. Anything else is a local build. The module
// version and VCS details from runtime/debug.ReadBuildInfo are reported for
// display, and never decide the kind.
//
// The package imports only the standard library, and nothing from
// selfupdate, so any program can read its stamps.
package buildinfo

import (
	"regexp"
	"runtime/debug"
	"strconv"
	"strings"
)

// The stamp variables. They are set only by the linker.
var version, kind string

const (
	// VersionVar is the -X symbol that stamps the version.
	VersionVar = "github.com/maccavelli/go-selfupdate-lib/buildinfo.version"
	// KindVar is the -X symbol that stamps the build kind.
	KindVar = "github.com/maccavelli/go-selfupdate-lib/buildinfo.kind"
)

// releaseKind is the only stamped kind that can make a release.
const releaseKind = "release"

// releaseTag is a release version: vMAJOR.MINOR.PATCH, or the prerelease
// form vMAJOR.MINOR.PATCH-NAME.N of
// docs/decisions/0005-MADR-opt-in-prerelease-channels.md.
var releaseTag = regexp.MustCompile(`^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(-[a-z][a-z0-9]{0,15}\.(0|[1-9][0-9]*))?$`)

// readBuildInfo is debug.ReadBuildInfo, replaced in tests.
var readBuildInfo = debug.ReadBuildInfo

// Kind is the kind of the running build.
type Kind uint8

const (
	// KindUnknown is the zero value. Identity never returns it.
	KindUnknown Kind = iota
	// KindRelease is a binary stamped as a release with a release tag.
	KindRelease
	// KindLocal is any other binary: unstamped, stamped local, or stamped
	// release with a version that is not a release tag.
	KindLocal
)

// String returns "unknown", "release" or "local".
func (k Kind) String() string {
	switch k {
	case KindRelease:
		return "release"
	case KindLocal:
		return "local"
	default:
		return "unknown"
	}
}

// Info is the running binary's identity.
type Info struct {
	// Version is the stamped version, verbatim. It may be empty.
	Version string
	// Kind is KindRelease or KindLocal.
	Kind Kind
	// Reason says why a "release" stamp was refused. It is empty otherwise.
	Reason string
	// Module is the main module's path, from debug.ReadBuildInfo.
	Module string
	// ModuleVersion is the main module's version, such as "(devel)" or a
	// tag from go install.
	ModuleVersion string
	// GoVersion is the toolchain that built the binary.
	GoVersion string
	// Revision is the vcs.revision build setting.
	Revision string
	// Time is the vcs.time build setting, verbatim.
	Time string
	// Modified is true when the vcs.modified build setting is "true".
	Modified bool
}

// Identity returns the running binary's identity.
func Identity() Info {
	info := decide(version, kind)
	if bi, ok := readBuildInfo(); ok && bi != nil {
		info.Module = bi.Main.Path
		info.ModuleVersion = bi.Main.Version
		info.GoVersion = bi.GoVersion
		for _, s := range bi.Settings {
			switch s.Key {
			case "vcs.revision":
				info.Revision = s.Value
			case "vcs.time":
				info.Time = s.Value
			case "vcs.modified":
				info.Modified = s.Value == "true"
			}
		}
	}
	return info
}

// decide applies the release rule to the two stamps.
func decide(v, k string) Info {
	info := Info{Version: v, Kind: KindLocal}
	if k != releaseKind {
		return info
	}
	if !releaseTag.MatchString(v) {
		info.Reason = "buildinfo: stamped version " + strconv.Quote(v) + " is not a release tag"
		return info
	}
	info.Kind = KindRelease
	return info
}

// Current is the version to report as the running identity: the stamped
// version; else the module version, unless it is "(devel)"; else "dev".
func (i Info) Current() string {
	switch {
	case i.Version != "":
		return i.Version
	case i.ModuleVersion != "" && i.ModuleVersion != "(devel)":
		return i.ModuleVersion
	default:
		return "dev"
	}
}

// String returns "<Current> (<kind>)", followed by the first 12 characters
// of the revision, and "-dirty" when modified, when the revision is known.
func (i Info) String() string {
	var b strings.Builder
	b.WriteString(i.Current())
	b.WriteString(" (")
	b.WriteString(i.Kind.String())
	b.WriteString(")")
	if i.Revision != "" {
		b.WriteString(" ")
		b.WriteString(i.Revision[:min(12, len(i.Revision))])
		if i.Modified {
			b.WriteString("-dirty")
		}
	}
	return b.String()
}

// LDFlags returns the -ldflags fragment that stamps tag as a release.
func LDFlags(tag string) string {
	return "-X " + VersionVar + "=" + tag + " -X " + KindVar + "=" + releaseKind
}
