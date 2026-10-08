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
// The unpacker refuses any archive that another tool, such as bsdtar or
// Info-ZIP's unzip, could extract differently: names outside printable
// ASCII or with an element ending in a dot or a space, a zip local header
// that disagrees with its central record or an Info-ZIP Unicode Path
// field, a directory that is not named as one or that holds data, and
// encrypted entries
// (docs/decisions/0015-MADR-remediate-third-debugging-pass-findings.md
// E1-E4). A PAX global header, such as git archive writes, is skipped,
// unless it sets an entry's path, link, size or sparse map (E8). A zip's
// records must account for every byte of it: no bytes before its first
// entry, between entries or after its end record, no slack in its central
// directory, and each data descriptor reading one way only, as its
// central record says, so that a streaming reader, such as macOS's ditto
// or Java's ZipInputStream, sees the entries archive/zip does
// (docs/decisions/0017-MADR-verify-build-provenance-and-close-0015-open-items.md
// 2B). On every host, not only on Windows, it refuses a name whose element
// holds one of < > : " | ? *, which Win32 refuses and Windows extractors
// rewrite to "_", or names a device: CON, PRN, AUX, NUL, CONIN$, CONOUT$,
// COM0-COM9 or LPT0-LPT9, before its first dot and in any case (4C).
//
// The package uses only the standard library and compiles on every OS.
// This module's release workflows build and publish archives from a
// release spec with "packaging": "archive" (selfupdate/releasespec;
// docs/decisions/0013-MADR-build-and-stage-release-workflow.md). Archives
// from other tooling, such as GoReleaser, work too, published as immutable
// GitHub releases with a SHA-256 checksum file.
package archive
