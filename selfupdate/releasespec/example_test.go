package releasespec_test

import (
	_ "embed"
	"fmt"

	"github.com/maccavelli/go-selfupdate-lib/selfupdate/releasespec"
)

// A program embeds the spec beside its update code. Here the spec is
// testdata/archive.json.
//
//go:embed testdata/archive.json
var releaseSpec []byte

func ExampleParse() {
	spec, err := releasespec.Parse(releaseSpec)
	if err != nil {
		fmt.Println(err)
		return
	}
	product, err := spec.Product("relay")
	if err != nil {
		fmt.Println(err)
		return
	}
	for _, p := range spec.Targets() {
		name, _ := spec.AssetName(product.Name, p)
		fmt.Printf("%s/%s: %s\n", p.OS, p.Arch, name)
	}
	unpacker, _ := spec.Unpacker()
	fmt.Println("packed:", unpacker != nil)
	// Output:
	// linux/amd64: relay-linux-amd64.tar.gz
	// linux/arm64: relay-linux-arm64.gz
	// darwin/arm64: relay-darwin-arm64.tar.gz
	// windows/amd64: relay-windows-amd64.zip
	// windows/arm64: relay-windows-arm64.tar.gz
	// packed: true
}
