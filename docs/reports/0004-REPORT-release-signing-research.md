# Release signing for `selfupdate`: research and a design kept for later

Associated record:
[0004-MADR-evolve-selfupdate-api-and-tui-support.md](../decisions/0004-MADR-evolve-selfupdate-api-and-tui-support.md)

Date: 2026-09-30

This report records research. It decides nothing. The decision it
informed is in the associated MADR: release signing is **deferred**, and
only the `ManifestVerifier` hook is built now.

## Why the research was done

The first draft of the MADR listed release signing as a later item and
asked the owner where a signing key would live. The owner answered:

> Research this. It will need to be idempotent and work across windows,
> linux, and macos. An idiomatic go solution would be ideal, although web
> research for existing solutions, ideas, and best practices being used by
> similar go projects use cases.

## Method

- **The research.** One read-only review covered:
  - how comparable Go projects sign releases and verify them at update
    time;
  - how each approach fits `selfupdate`'s seams (`updater.go`, `types.go`,
    `doc.go` and `.github/workflows/publish-selfupdate-release.yml`).
- **Local checks.** The author re-checked the load-bearing claims:
  - the import list and API of `golang.org/x/mod@v0.40.0/sumdb/note`, in
    the module cache;
  - a determinism and rotation probe, a small Go program run twice on a
    scratch copy;
  - the visibility of the seven repositories that publish with the
    workflow (all public, checked 2026-09-30).
- No repository was modified.

## Requirements the owner set

- **Idempotent:** the same input gives the same signature, and
  verification does not depend on a clock.
- **Cross-platform:** verification works on Windows, Linux and macOS in
  pure Go, with `CGO_ENABLED=0` and no external binary at run time.
- **Idiomatic Go**, informed by what similar projects do.
- **The dependency floor** from `AGENTS.md`: a new module needs its own
  MADR, so an option with no new module is strongly preferred.

## What `selfupdate` already guarantees

| Threat | Covered today by |
| :--- | :--- |
| Tampering in transit | HTTPS to the API origin; the https-only redirect rule for asset downloads |
| Release files swapped after publication | GitHub immutable releases. The Updater refuses a release that is not immutable. |
| A corrupted or substituted binary | `SHA256SUMS` and GitHub's per-asset digests, both checked before install |
| An older release replayed as newer | the strict version policy; a downgrade needs an exact `--version` |
| A re-run overwriting a release | `refuse-existing-release.sh` fails closed; `--clobber` is forbidden |
| Proof of which workflow built a binary | build-provenance attestations, produced by the workflow but not checked at run time |

## Options compared

| Option | New modules | Pure-Go verify, no binary | Byte-idempotent signing | Clock-free verify | Adopted by |
| :--- | :--- | :--- | :--- | :--- | :--- |
| **C2SP signed note** (`golang.org/x/mod/sumdb/note`) | **none**: x/mod is already required | yes | **yes** (Ed25519) | yes | Go checksum database, Sigsum, transparency-dev witnesses |
| Raw Ed25519 detached signature | none | yes | yes | yes | Tailscale `distsign` |
| minisign (`aead.dev/minisign`) | `golang.org/x/crypto` | yes | only with a fixed trusted comment; the default one carries a timestamp | yes | minio/selfupdate |
| SSH signatures (sshsig) | `golang.org/x/crypto`, or hand-parsing | yes | yes with Ed25519 (the tool's own output unconfirmed) | yes | Git commit signing; release use is ad hoc |
| OpenPGP (`ProtonMail/go-crypto`) | circl and x/crypto | yes | **no**: the signature embeds a creation time | **no**: keys expire | HashiCorp, Syncthing, goreleaser's default |
| ECDSA | none | yes | **no**: `SignASN1` is randomized | yes | Syncthing auto-upgrade, creativeprojects/go-selfupdate |
| Sigstore keyless / GitHub attestations (`sigstore-go`) | 19 direct and 60 indirect requirements | yes, but the trusted root needs refreshing | **no**: a new certificate and log entry per run | **no** | Caddy, gh CLI, goreleaser |
| `gh attestation verify` as a subprocess | none | **no**: needs `gh` installed | n/a | no | gh CLI |

## Findings

- **The signed note is the only option that meets all four requirements
  with no new module.**
  - It is the Go project's own signed format (C2SP `signed-note`).
  - Its package imports only the standard library, and x/mod is already a
    requirement.
  - Its signatures are Ed25519, which is deterministic (RFC 8032; Go
    documents that "rand is ignored").
  - The probe confirmed that signing the same text twice gives
    byte-identical output.
- **Rotation is built into the format.**
  - `note.Sign` accepts several signers.
  - `note.Open` ignores signatures from unknown keys, and fails when a
    known key's signature is invalid (`InvalidSignatureError`, or "no
    verifiable signatures" when no known key signed).
  - The probe confirmed four cases:
    - a note signed by an old and a new key opens with the new key alone;
    - a note signed by the old key alone is rejected by the new key;
    - a note with a changed tag is rejected;
    - CRLF line endings are rejected.
  - The probe built for all three operating systems with `CGO_ENABLED=0`.
- **The Go toolchain embeds its checksum-database verifier key as a Go
  string constant** (`cmd/go/internal/modfetch/key.go`). That is the idiom
  for embedding a key.
- **Today's `Verifier` seam cannot check a manifest signature.**
  `Verification` has neither the manifest bytes nor a way to open another
  asset. The MADR's `ManifestVerifier` seam, which runs after the manifest
  download and before the binary download, is what a signature check
  needs.
- **Sigstore adds provenance, not idempotency.** Runtime provenance would
  prove that a binary was built by this workflow at a given commit. It
  costs about 79 module requirements and a trusted-root refresh, so it
  would be an opt-in nested module.

## What signing would add today

The owner decided on 2026-09-30 that signing would be auto-approved, with
no required reviewers on the signing environment. Under that decision, and
with the key held as an Actions secret, each threat looks like this:

| Threat | Does a signature help? |
| :--- | :--- |
| Someone gains write access to a repository (stolen token or account) | **No.** They push a `v*` tag, and the workflow signs automatically. Anyone who can push a tag can also change the workflow at that tag and read the secret, so repository write access is effectively key access. |
| GitHub's storage or asset servers serve altered bytes | **Marginally.** The key sits in GitHub's own secret store. |
| Tampering in transit | **No.** HTTPS already covers it. |
| Releases served from a host that cannot guarantee immutability (GitLab, mirrors, S3, plain HTTP) | **Yes.** Without a signature, nothing binds a tag to its files. |
| A human check on each release | **No longer**, because signing is auto-approved. |

A signature earns its cost only when the key lives outside the thing it
protects, or when the release host cannot be trusted to keep files
unchanged. Neither is true while every release is on GitHub with immutable
releases on.

**The realistic threat is repository and account compromise.** These
defences cover it, and cost minutes per repository:

- a tag ruleset that restricts creating `v*` tags to the owner;
- two-factor authentication, and fine-grained, short-lived tokens;
- immutable releases turned on in every consumer repository (the Updater
  already refuses releases that are not immutable).

## Design kept for when signing is needed

The MADR makes signing a requirement of the future record on releases that
are not immutable. This section keeps the design ready for that record. It
decides nothing now.

### Verification

```go
// package selfupdate/verify/signednote
type Config struct {
    Repository   string   // "github.com/<owner>/<repo>"; must match the statement
    Keys         []string // note verifier keys, "name+hash+base64"
    RequiredFrom string   // tags >= this must carry a valid note; "" = every tag
}
func New(Config) (selfupdate.ManifestVerifier, error)
```

`VerifyManifest` does the following:

1. Opens `SHA256SUMS.note` through `OpenAsset`, bounded at 16 KiB.
2. Calls `note.Open` with `note.VerifierList` of the configured keys.
3. Parses the statement strictly.
4. Requires all of these:
   - the repository equals `Config.Repository`;
   - the tag equals `Release.Tag`;
   - the product appears in `products`;
   - the SHA-256 of the manifest bytes equals the `manifest` line.
5. Wraps every failure in `ErrIntegrity`.

### The signed statement

A release gains `SHA256SUMS.note`. `SHA256SUMS` itself is unchanged, so
existing binaries are unaffected. The name avoids `.sig`, which suggests a
raw signify, minisign or cosign signature.

The statement has LF line endings, a fixed line order, and no timestamps or
run identifiers, so it is a pure function of its inputs:

```text
selfupdate-release/v1
repository github.com/<owner>/<repo>
tag v1.2.3
products <p1> <p2>
manifest SHA256SUMS sha256:<hex of the exact asset bytes>

— <key name> <base64 signature>
```

| Line | Stops |
| :--- | :--- |
| Header | a signature made for another purpose being accepted as a release statement |
| `repository` | another repository's release being substituted under a shared key |
| `tag` | an older signed release being replayed as a newer one |
| `products` | one product's binary being substituted for another in a multi-product release |

A "freeze" attack, in which a stale release is served as the latest, is
not solved by signing. Only HTTPS to the API origin mitigates it.

### Signing in the workflow

- `publish-selfupdate-release.yml` gains an input `sign`. When it is
  `true`, the job signs after validation and before the draft release is
  created. It fails closed when the environment or its secret is missing.
- The signing job runs in the caller's environment `selfupdate-signing`:
  - The private key is that environment's secret `SELFUPDATE_NOTE_KEY`, in
    the one-line `PRIVATE+KEY+…` form, which GitHub masks in logs.
  - Environment secrets cannot be passed through `secrets:` to a reusable
    workflow. The environment is resolved in the caller's repository.
  - The environment's deployment rule allows only tags matching `v*`.
- The signer is a small Go `main`, for example
  `internal/cmd/selfupdate-note`. It is run with `go run` from the
  go-core-lib checkout at `workflow_sha`, and it shares the statement
  builder with the verifier's parser, so the format is defined once.
- Before upload, the job opens its own note with the public key, held in a
  repository variable. This catches a mismatch between the secret and the
  variable.
- `verify-selfupdate-release.sh` reserves the name `SHA256SUMS.note`, so a
  caller cannot stage its own copy.

### Embedding the verifying key

Each consumer embeds its keys as a Go `const` beside its `selfupdate`
configuration:

- not with `-ldflags -X`, because a build without the flag gets an empty
  key and would fail open;
- not with `go:embed`, because a Windows checkout with `autocrlf` can add
  `\r`.

The library holds no product keys.

### Keys, rotation and revocation

- **Each repository trusts two keys from the first signed release:**
  - a CI key, held in the environment secret;
  - an offline backup key that never enters Actions.
- **Planned rotation:**
  1. Sign with both the old and the new key.
  2. Release a version that trusts both.
  3. Sign with the new key alone.
  4. Drop the old key in the next release.
- **A lost or compromised CI key:**
  1. Sign the next release with the backup key alone.
  2. That release trusts the backup key and a new CI key.
- **The limit:** an installed binary can never be told remotely to stop
  trusting a key. Tailscale's embedded root keys have the same limit.

### Rollout, per consumer

1. The workflow publishes `SHA256SUMS.note`. Existing binaries ignore it.
2. The consumer releases a version whose verifier has `RequiredFrom` set to
   its first signed tag. The hop from unsigned to signed is
   trust-on-upgrade: it rests on HTTPS, immutable releases and the
   checksum, as today.
3. Every later release requires a valid note. Installing an older, unsigned
   tag with `--version` keeps working.
4. `doc.go` stops saying there is no publisher signature.

### What "idempotent" guarantees

- The same manifest bytes, keys, repository, tag and product list always
  give a byte-identical `SHA256SUMS.note`.
- Verification is a pure function of the bytes and the embedded keys. It
  has no clock and no expiry, so there are no time-dependent failures.
- It does not make builds reproducible. The note is idempotent relative to
  the staged manifest.
- A re-run still stops at `refuse-existing-release`, which fails closed.
  Re-running after a draft is deleted republishes an identical note.

### Where the offline backup key could be kept

The backup key is one line of about 100 characters. It is used only when
the CI key is lost or compromised, perhaps never. What matters is that it
survives for years and never touches day-to-day machines or accounts.

| Option | Survives device loss | Exposure if an online account is breached | Cost and effort | Drawback |
| :--- | :--- | :--- | :--- | :--- |
| Password-manager secure note | yes (synced) | exposed along with the vault | none | not offline |
| Passphrase-encrypted file (`age`) on two offline USB drives in two places, passphrase in the password manager | yes | none: the vault holds only the passphrase | two drives | drives degrade; check yearly |
| Paper copy (text or QR) in a safe | yes, fire aside | none | minutes | whoever opens the safe has the key, unless the paper holds the encrypted file |
| Hardware key (non-exportable) | only with two devices, each a separately trusted key | none | per device, plus a custom `note.Signer` | more code; Ed25519 support depends on the device firmware (unconfirmed) |
| Cloud KMS | yes | the cloud account becomes the target | monthly per key | not offline; Ed25519 support unconfirmed |
| Key split among trusted people (Shamir) | yes | none | coordination | overkill for one owner, unless succession matters |

Practices that apply to any choice:

- Generate the key on the owner's machine, never in CI or a chat.
- Keep two copies in two physical places.
- Encrypt the key, and keep the passphrase elsewhere.
- Keep it out of Actions, repositories, and unencrypted laptop disks.
- Prove the restore at creation and yearly: decrypt, sign a test note, and
  verify it against the embedded key.
- Keep a recovery guide that holds no key material.
- Label each copy with its key name and hash.

The option favoured during the discussion was the `age`-encrypted file on
two drives plus a paper copy of the encrypted file, with the passphrase in
the password manager.

### Questions for the record that adopts signing

1. **A signer command in a library-only repository.** `AGENTS.md` says
   "no packaged binary". An unreleased `internal/cmd` tool, run only by
   `go run` in the workflow, packages nothing. The recommendation was to
   allow it, and to say so in `AGENTS.md`.
2. **One key per repository, or one shared key?** The statement's
   `repository` and `products` lines make a shared key safe against
   substitution, but a shared key widens the damage of a compromise. The
   recommendation was one key pair per repository.
3. **Key names.** A name may not contain spaces or `+`. The
   recommendation was `github.com/<owner>/<repo>/selfupdate`.
4. **Required reviewers on `selfupdate-signing`.** The owner decided
   auto-approve on 2026-09-30. That record should revisit the choice,
   because a human gate is most of what a CI-held key could add.
5. **The backup-key location.** Options and practices are listed above.

## Since (2026-10-12)

This report is kept as written. Two of its statements have since been
settled by later records:

- **Provenance is checked at run time, opt-in, in this module.** The
  threat table's "produced by the workflow but not checked at run time"
  and the finding that runtime provenance "would be an opt-in nested
  module" predate `selfupdate/verify/ghattest`, which `v1.12.0` added: a
  `ManifestVerifier` that runs `gh attestation verify`, with no new module
  requirement
  ([0017-MADR-verify-build-provenance-and-close-0015-open-items.md](../decisions/0017-MADR-verify-build-provenance-and-close-0015-open-items.md)).
- **Question 1, a tool under `internal/cmd`:** settled. AGENTS.md says
  "Commands under `internal/cmd/` are build and release tools that the
  workflows run from source; they are never released", and
  `internal/cmd/selfupdate-release` is one
  ([0013-MADR-build-and-stage-release-workflow.md](../decisions/0013-MADR-build-and-stage-release-workflow.md)).

## Since (2026-10-19)

- **A correction to "Signing in the workflow".** The bullet "Environment
  secrets cannot be passed through `secrets:` to a reusable workflow"
  is contradicted by GitHub's "Reuse workflows" page, read on 2026-10-19.
  The page says that the caller "must still pass the secret … You can pass
  a secret by name even if it only exists in the environment", and that
  if the caller "doesn't pass an environment secret, the secret resolves
  to an empty string in the reusable workflow. The workflow run doesn't
  show an error." A design built on the bullet as written would sign with
  an empty key, or fail without a clear cause.
- **The design is built.**
  [0025-MADR-align-with-go-sdk-design-practices.md](../decisions/0025-MADR-align-with-go-sdk-design-practices.md)
  D10-A builds it opt-in, with the refinements of its Amendment A1:
  - signing runs in a separate `sign` job, which has the environment
    `selfupdate-signing`;
  - the caller passes `SELFUPDATE_NOTE_KEY` by name, and the verifier key
    as an input;
  - the signer refuses an empty key and checks its own note before it is
    uploaded.

  The rest of this report is kept as written.

## Sources

- C2SP signed-note specification,
  <https://github.com/C2SP/C2SP/blob/main/signed-note.md>
- `golang.org/x/mod/sumdb/note`, <https://pkg.go.dev/golang.org/x/mod/sumdb/note>
- RFC 8032 (deterministic EdDSA), <https://www.rfc-editor.org/info/rfc8032/>
- Go `crypto/ed25519`, <https://pkg.go.dev/crypto/ed25519>
- Go `crypto/ecdsa`, <https://pkg.go.dev/crypto/ecdsa>
- Sigsum checkpoints, <https://pkg.go.dev/sigsum.org/sigsum-go/pkg/checkpoint>
- transparency-dev formats,
  <https://github.com/transparency-dev/formats/blob/main/log/README.md>
- Tailscale `distsign`, <https://pkg.go.dev/tailscale.com/clientupdate/distsign>
- minio/selfupdate's minisign verifier,
  <https://github.com/minio/selfupdate/blob/master/minisign.go>
- minisign format, <https://jedisct1.github.io/minisign/>
- SSH signatures, <https://www.agwa.name/blog/post/ssh_signatures>
- goreleaser signing, <https://goreleaser.com/customization/sign/>
- HashiCorp binary verification,
  <https://developer.hashicorp.com/well-architected-framework/verify-hashicorp-binary>
- k6 expired signing key, <https://github.com/grafana/k6/issues/2973>
- Syncthing release signing, <https://docs.syncthing.net/dev/release-signing.html>
- creativeprojects/go-selfupdate,
  <https://pkg.go.dev/github.com/creativeprojects/go-selfupdate>
- sigstore-go requirements, <https://github.com/sigstore/sigstore-go/blob/main/go.mod>
- Caddy signature verification, <https://caddyserver.com/docs/signature-verification>
- Verifying attestations offline,
  <https://docs.github.com/en/actions/security-for-github-actions/using-artifact-attestations/verifying-attestations-offline>
- `gh release verify`, <https://cli.github.com/manual/gh_release_verify>
- Reusable workflows,
  <https://docs.github.com/en/actions/how-tos/reuse-automations/reuse-workflows>
- Deployments and environments,
  <https://docs.github.com/en/actions/reference/workflows-and-actions/deployments-and-environments>
- Immutable releases,
  <https://github.blog/changelog/2025-10-28-immutable-releases-are-now-generally-available/>
