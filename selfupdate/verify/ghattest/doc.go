// Package ghattest checks, during an update, that a release was built by
// the expected GitHub Actions workflow: it verifies the release's build
// provenance attestation with gh attestation verify
// (docs/decisions/0017-MADR-verify-build-provenance-and-close-0015-open-items.md
// 1B).
//
// NewManifestVerifier is the check to use: a selfupdate.ManifestVerifier
// that verifies the release's SHA256SUMS, before any binary is downloaded.
// SHA256SUMS pins every asset's digest, which the updater checks anyway, so
// one check covers them all. This module's publish workflow attests every
// file it publishes, SHA256SUMS included. NewVerifier checks the asset
// itself instead.
//
// What it proves: the file came out of Policy.SignerWorkflow, this
// module's publish workflow by default, run for Policy.Repository at the
// release's tag, on a GitHub-hosted runner. A release made with a stolen
// token outside the workflow fails it. What it does not prove: anything
// about a malicious commit and v* tag pushed through the workflow, which
// it attests like any other. Restrict who can create v* tags with a tag
// ruleset; the building guide's step 4 says how.
//
// The check is opt-in: nothing enables it by default. It runs gh, by the
// absolute path Options.GH gives, through a service.Runner, and gh must be
// logged in even for a public repository. Every failure fails the update:
// gh missing, not logged in (exit 4), not verifying (exit 1), timing out,
// or reporting an attestation that does not meet the whole Policy, which
// the package checks again in gh's JSON output. The updater joins the error
// with selfupdate.ErrIntegrity. It suits a developer's machine, where gh
// is installed and logged in, more than an unattended service. gh 2.102.0
// was the version probed.
package ghattest
