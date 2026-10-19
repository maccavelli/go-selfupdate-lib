// Package signednote checks, during an update, that a release's SHA256SUMS
// was signed by a key the program trusts: a C2SP signed note over a release
// statement, published beside the manifest as SHA256SUMS.note
// (docs/decisions/0025-MADR-align-with-go-sdk-design-practices.md D10;
// docs/reports/0004-REPORT-release-signing-research.md, "Design kept for
// when signing is needed").
//
// New returns a selfupdate.ManifestVerifier. It reads the note through the
// release's OpenAsset, at most MaxNoteSize bytes, opens it with
// golang.org/x/mod/sumdb/note against Config.Keys, parses the statement
// strictly, and requires its repository to be Config.Repository, its tag the
// release's, its products to include the one being updated, and its manifest
// line to name the manifest asset with the SHA-256 of the exact bytes the
// updater downloaded. The statement is five LF lines with no timestamp, so
// the same release and key always give the same note:
//
//	selfupdate-release/v1
//	repository github.com/<owner>/<repo>
//	tag v1.2.3
//	products <p1> <p2>
//	manifest SHA256SUMS sha256:<hex>
//
// What it proves: a holder of a key in Config.Keys signed this manifest for
// this repository, tag and product, so a substituted manifest, another
// repository's or another product's release, and a replayed older tag all
// fail. What it does not prove: freshness, since a stale release served as
// the latest still verifies; or how the signing key was kept. A key held by
// an auto-approved CI job is as safe as the repository's tag rules.
//
// The check is opt-in. Config.RequiredFrom names the first signed tag:
// a release below it may have no note, so a program can still install an
// older, unsigned release with an exact version. A note on such a release is
// verified all the same. A prerelease of RequiredFrom sorts below it. Every
// failure fails the update, and the updater joins the error with
// selfupdate.ErrIntegrity.
//
// The build workflow's publish step signs when it is called with sign: true;
// docs/guides/building-releases.md, "Sign a release", says how. Keys are
// embedded in the program as Go constants, never set with -ldflags -X: a
// build without the flag would trust no key.
package signednote
