// Package releasespec reads a program's release spec, the
// selfupdate-release.json file that describes its products, platforms,
// packaging, extra assets and prerelease channels in one place
// (docs/decisions/0013-MADR-build-and-stage-release-workflow.md §2, §3).
//
// The program embeds the file and configures its updater from it:
//
//	//go:embed selfupdate-release.json
//	var releaseSpec []byte
//
//	spec, err := releasespec.Parse(releaseSpec)
//	// spec.Product(product), spec.AssetSelector(), spec.Unpacker()
//
// build-selfupdate-release.yml reads the same file with the same parser, so
// the platform list, the product names and the asset names the program
// selects cannot drift from what the workflow builds and publishes.
//
// The file must sit in the directory of the package that embeds it, since
// an embed pattern cannot reach a parent directory. Parse refuses unknown
// fields, trailing data and a schema other than SchemaVersion.
//
// The package imports the standard library, selfupdate and
// selfupdate/archive only.
package releasespec
