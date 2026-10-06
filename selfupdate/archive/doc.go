// Package archive selects a release asset that is an archive holding the
// program, a .tar.gz, .zip or .gz, and extracts the program from it during
// an update (docs/decisions/0012-MADR-archive-assets-and-macos-codesign.md).
//
// NewSelector chooses the archive and the checksum asset, and marks the
// selection Packed; the archive's own digest is what SHA256SUMS and the
// Verifiers check. NewUnpacker is the selfupdate.Unpacker that then writes
// the program into the session's staging. The two go together in a
// selfupdate.Config.
//
// The package uses only the standard library and compiles on every OS.
// This module's release workflows build and publish archives from a
// release spec with "packaging": "archive" (selfupdate/releasespec;
// docs/decisions/0013-MADR-build-and-stage-release-workflow.md). Archives
// from other tooling, such as GoReleaser, work too, published as immutable
// GitHub releases with a SHA-256 checksum file.
package archive
