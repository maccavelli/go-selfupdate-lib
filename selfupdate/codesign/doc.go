// Package codesign re-signs a staged binary on macOS, and checks a staged
// binary's signature, with Apple's /usr/bin/codesign
// (docs/decisions/0012-MADR-archive-assets-and-macos-codesign.md §5, §6).
//
// It is opt-in, for a publisher who signs. No default of this module runs
// it: a program that builds neither a signer nor a checker updates an
// unsigned or linker-signed binary exactly as it would without this
// package (0012-MADR §10).
//
//   - NewSigner returns a selfupdate.Transformer that signs the staged file
//     with a configured identity and identifier, and verifies the result.
//     Signing with a certificate needs the identity in the user's
//     keychains; it is expected to need Xcode or the Command Line Tools,
//     whose codesign_allocate /usr/bin holds only a shim for.
//   - NewChecker returns a selfupdate.Prober that requires the staged file's
//     signature to be valid and, when given, to meet a code requirement,
//     such as a Developer ID requirement naming a team. A binary with no
//     signature fails it.
//
// The package compiles on every OS. Off macOS, both constructors return
// service.ErrUnsupported. The tool runs through a service.Runner, by
// absolute path, with an environment the package builds.
package codesign
