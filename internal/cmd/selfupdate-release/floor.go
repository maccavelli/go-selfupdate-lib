package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/maccavelli/go-selfupdate-lib/selfupdate/releasespec"
)

// libraryPath is this library's module path.
const libraryPath = "github.com/maccavelli/go-selfupdate-lib"

// specFloor is a spec field that older releases of this library refuse:
// a program whose module requires one of them cannot parse the spec it
// embeds, and cannot update itself
// (docs/decisions/0015-MADR-remediate-third-debugging-pass-findings.md F3).
type specFloor struct {
	field string // the JSON path
	min   string // the first release that reads it
	why   string
	set   func(releasespec.Spec) bool
}

// specFloors lists every field a release after v1.9.0 added.
var specFloors = []specFloor{
	{"installer", "v1.10.0", "releasespec.Parse in v1.9.0 refuses it as an unknown field",
		func(s releasespec.Spec) bool { return s.Installer != nil }},
}

// goModEdit is the part of `go mod edit -json` this reads.
type goModEdit struct {
	Module  struct{ Path string }
	Require []struct{ Path, Version string }
	Replace []struct {
		Old struct{ Path, Version string }
		New struct{ Path, Version string }
	}
}

// checkLibraryFloor checks that the module in dir requires a release of
// this library that reads every field spec uses. A module that is the
// library is not checked; a directory replace is not either, and the note
// says so. It reads go.mod with `go mod edit -json`, and compares versions
// by semver precedence.
func checkLibraryFloor(ctx context.Context, g goTool, dir string, spec releasespec.Spec) (note string, err error) {
	out, err := g.run(ctx, dir, nil, "mod", "edit", "-json")
	if err != nil {
		return "", err
	}
	var mod goModEdit
	if err := json.Unmarshal([]byte(out), &mod); err != nil {
		return "", fmt.Errorf("go mod edit -json: %w", err)
	}
	if mod.Module.Path == libraryPath {
		return "", nil
	}
	version := ""
	for _, r := range mod.Require {
		if r.Path == libraryPath {
			version = r.Version
		}
	}
	if version == "" {
		return "", fmt.Errorf("module %s does not require %s, whose releasespec reads the spec", mod.Module.Path, libraryPath)
	}
	for _, r := range mod.Replace {
		if r.Old.Path != libraryPath || r.Old.Version != "" && r.Old.Version != version {
			continue
		}
		if r.New.Version == "" {
			return fmt.Sprintf("%s is replaced by the directory %s: its version is not checked against the spec.", libraryPath, r.New.Path), nil
		}
		version = r.New.Version
	}
	for _, f := range specFloors {
		if f.set(spec) && semverLess(version, f.min) {
			return "", fmt.Errorf("module %s requires go-selfupdate-lib %s; this spec's %q needs %s or later, or the program cannot parse the spec it embeds (%s; go get %s@%s)",
				mod.Module.Path, version, f.field, f.min, f.why, libraryPath, f.min)
		}
	}
	return "", nil
}

// semverLess reports whether a precedes b, by semver precedence: a
// prerelease, such as a pseudo-version, precedes its release. A version
// that does not parse precedes every other.
func semverLess(a, b string) bool {
	return semverCompare(a, b) < 0
}

func semverCompare(a, b string) int {
	pa, okA := parseSemver(a)
	pb, okB := parseSemver(b)
	if !okA || !okB {
		if okA == okB {
			return strings.Compare(a, b)
		}
		if !okA {
			return -1
		}
		return 1
	}
	for i := range 3 {
		if pa.core[i] != pb.core[i] {
			if pa.core[i] < pb.core[i] {
				return -1
			}
			return 1
		}
	}
	switch {
	case pa.pre == "" && pb.pre == "":
		return 0
	case pa.pre == "":
		return 1
	case pb.pre == "":
		return -1
	}
	ia, ib := strings.Split(pa.pre, "."), strings.Split(pb.pre, ".")
	for i := 0; i < len(ia) && i < len(ib); i++ {
		if c := compareIdent(ia[i], ib[i]); c != 0 {
			return c
		}
	}
	switch {
	case len(ia) < len(ib):
		return -1
	case len(ia) > len(ib):
		return 1
	}
	return 0
}

type semver struct {
	core [3]uint64
	pre  string
}

// parseSemver reads vMAJOR.MINOR.PATCH[-PRE][+BUILD].
func parseSemver(v string) (semver, bool) {
	rest, ok := strings.CutPrefix(v, "v")
	if !ok {
		return semver{}, false
	}
	rest, _, _ = strings.Cut(rest, "+")
	core, pre, _ := strings.Cut(rest, "-")
	parts := strings.Split(core, ".")
	if len(parts) != 3 {
		return semver{}, false
	}
	var s semver
	for i, p := range parts {
		n, err := strconv.ParseUint(p, 10, 64)
		if err != nil || p == "" || len(p) > 1 && p[0] == '0' {
			return semver{}, false
		}
		s.core[i] = n
	}
	s.pre = pre
	return s, true
}

// compareIdent compares two prerelease identifiers: numeric ones by value,
// and before alphanumeric ones, which compare as text.
func compareIdent(a, b string) int {
	na, errA := strconv.ParseUint(a, 10, 64)
	nb, errB := strconv.ParseUint(b, 10, 64)
	switch {
	case errA == nil && errB == nil:
		switch {
		case na < nb:
			return -1
		case na > nb:
			return 1
		}
		return 0
	case errA == nil:
		return -1
	case errB == nil:
		return 1
	}
	return strings.Compare(a, b)
}
