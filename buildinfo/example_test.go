package buildinfo_test

import (
	"fmt"

	"github.com/maccavelli/go-selfupdate-lib/buildinfo"
)

// A release build links in the stamps LDFlags returns, as the build
// workflow does.
func ExampleLDFlags() {
	fmt.Println(buildinfo.LDFlags("v1.2.3"))
	// Output: -X github.com/maccavelli/go-selfupdate-lib/buildinfo.version=v1.2.3 -X github.com/maccavelli/go-selfupdate-lib/buildinfo.kind=release
}

// Identity decides whether the running binary is a release from its stamps
// alone. A test binary is not stamped, so it is a local build; Current is
// the version a program reports, and passes to selfupdate.UserAgent.
func ExampleIdentity() {
	id := buildinfo.Identity()
	fmt.Println(id.Kind)
	fmt.Println(id.Current() != "")
	// Output:
	// local
	// true
}
