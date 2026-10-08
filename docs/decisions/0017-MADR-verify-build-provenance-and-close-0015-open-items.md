---
status: accepted
date: 2026-10-08
decision-makers: go-selfupdate-lib maintainers
consulted: the 0004 roadmap's open-work table; 0012, 0013, 0014 and 0015's records; the research of 2026-10-08 under Method
informed: the fleet's programs that adopt the build workflow and installers
---
# Check build provenance at update time with an opt-in `selfupdate/verify/ghattest`, and close the four items 0015 left open

## Context and Problem Statement

`v1.11.0` (2026-10-08) closed
[0015-MADR-remediate-third-debugging-pass-findings.md](0015-MADR-remediate-third-debugging-pass-findings.md).
[0004-MADR-evolve-selfupdate-api-and-tui-support.md](0004-MADR-evolve-selfupdate-api-and-tui-support.md)
keeps the module's open work in one table (P3, `:1143-1155`). The owner
asked, on 2026-10-08, for one record covering five of its items:

1. **`selfupdate/verify/ghattest`, the runtime attestation check**
   (0004-MADR P3 row `:1147`; §6 `:784-786`).
2. **Unreferenced local entries in a zip**
   ([0015-PLAN-remediate-third-debugging-pass-findings.md](0015-PLAN-remediate-third-debugging-pass-findings.md)
   `:60-62`, decision N4 `:105`).
3. **A crash between `Apply` and `Commit`** (0015-PLAN `:63-65`, N5
   `:106`).
4. **Refusing `:` in archive entry names on every host** (0015-PLAN
   `:66`).
5. **`queue: max` on the publish workflow's concurrency** (0015-PLAN
   `:67-68`, N9 `:110`).

The question is what to do about each: build it, and in which form and
release; or record it closed with a reason.

### Method

Three read-only investigations on 2026-10-08:

* the code and records behind each item, cited as `file:line` at
  `6dcdd8a` (`v1.11.0` plus records);
* GitHub artifact attestations, immutable-release attestations,
  `gh attestation verify`, `sigstore-go` and prior art, from the sources
  under More Information;
* the four leftovers, with experiments.

Every experiment ran in the session's scratch directory, against crafted
files and live public endpoints. Nothing in this repository was changed,
and no probe code is committed. Labels:

* **[read]**: read in this repository or at the cited URL;
* **[ran]**: observed in a scratch experiment on 2026-10-08, on macOS
  26.6.2 with Go 1.27.1;
* **[inferred]**: the investigation's reasoning, not observed.

### Item 1: runtime provenance (`verify/ghattest`)

**What 0004 already decided [read].**

* §1's table row (`:261`): "`selfupdate/verify/ghattest` | runtime
  attestation check | the exec variant is dependency-free; the sigstore
  variant is a new module | yes".
* §6 (`:784-786`): "**Runtime provenance** (`verify/ghattest`) still
  needs its own record: its sigstore variant is a nested module with a
  large dependency tree, and its exec variant needs `gh` at run time."
* Drivers (`:197-207`): additive within v1; "Core stays on the standard
  library plus the current `golang.org/x/{mod,sys,term}`. Any new module
  needs a MADR"; "The security posture does not erode by default … Each
  relaxation is an explicit opt-in".
* Option D (`:1004-1006`, rejected) argued against nested modules here:
  one "brings Charm into its CI, its vulnerability-scan scope and its
  dependency policy, and needs path-prefixed tags". The same argument
  applies to a sigstore module.
* [0004-REPORT-release-signing-research.md](../reports/0004-REPORT-release-signing-research.md)
  (`:40-46`) records the owner's requirements for signing:
  * idempotent: "verification does not depend on a clock";
  * "pure Go, with `CGO_ENABLED=0` and no external binary at run time";
  * "an option with no new module is strongly preferred".

  Its table (`:69-70`) scores `sigstore-go` at "19 direct and 60 indirect
  requirements" and `gh attestation verify` as "needs `gh` installed". It
  concludes (`:102-105`): "Sigstore adds provenance, not idempotency …
  It costs about 79 module requirements and a trusted-root refresh, so it
  would be an opt-in nested module."

**What the pipeline offers today [read].**

* **The order of `apply`** (`selfupdate/updater.go:215-412`):
  1. the manifest download;
  2. `ParseSHA256SUMS`;
  3. `Config.ManifestVerifiers`, "before staging exists and before any
     binary byte is fetched" (`:258-262`);
  4. the binary download;
  5. `verifyIntegrity`, which checks the SHA256SUMS entry and GitHub's
     asset digest;
  6. `Config.Verifiers`, on the asset as published;
  7. unpack, transform, probes, install.

  A check (`CheckOnly`) runs no verifier (`:184-190`).
* **The seams** (`selfupdate/types.go:324-351`,
  `selfupdate/manifestverify.go:18-44`):
  * `Verifier.Verify(ctx, Verification)` and
    `ManifestVerifier.VerifyManifest(ctx, ManifestVerification)`;
  * `Verification` carries `Product`, `Release`, `Selection`, `Size`,
    `SHA256`, `ManifestSHA256`, `GitHubSHA256`, `Open` and `OpenAsset`;
  * `ManifestVerification` carries the `Manifest` bytes;
  * neither carries an HTTP client, a credential or the repository;
  * a refusal is joined with `ErrIntegrity` (`updater.go:496-516`,
    `manifestverify.go:57-58`).
* **The release metadata** has `Tag`, `URL` and `Immutable`, but no
  commit: `githubReleaseJSON` decodes no `target_commitish`
  (`github.go:289-297`).
* **The GitHub source's client is not reachable.** Its client, token,
  rate-limit mapping and redirect policy (`github.go:218-251`, `:577-637`,
  `:818-873`) are unexported. A verifier needs its own client, but can
  reuse the exported `CredentialProvider` chain and `RateLimitError`.
* **The precedent for running a tool** is `selfupdate/codesign`. It runs
  `/usr/bin/codesign` through `service.Runner`, whose `Command.Path`
  "must be absolute: a runner never looks a tool up on PATH"
  (`service/runner.go:14-39`), with a built environment
  (`codesign.go:22`).

**What the releases carry [read].**

* The publish workflow attests every staged file in one step,
  `actions/attest-build-provenance@96278af… # v3.2.0` with
  `subject-path: staging/*`
  (`.github/workflows/publish-selfupdate-release.yml:224-227`).
* Staging always holds `SHA256SUMS` (`scripts/verify-selfupdate-release.sh:184-185`),
  so **the manifest is itself attested**, beside every binary, archive,
  extra and installer.
* The workflow then waits for immutability and runs `gh release verify`
  (`:251-281`).
* The installers verify only on request: `--verify-attestation` /
  `-VerifyAttestation` refuses without an authenticated `gh`
  (`install.sh:406-409`, `install.ps1:364-371`). They then run
  `gh attestation verify <asset> --repo <repository> --signer-workflow
  maccavelli/go-selfupdate-lib/.github/workflows/publish-selfupdate-release.yml`
  on each product asset (`install.sh:321`, `install.ps1:405`), but not on
  `SHA256SUMS`. 0014-MADR Q2 (`:482-486`) made this opt-in because "`gh`
  exits 4 unauthenticated".
* The guide documents the same command (`docs/guides/building-releases.md:314-322`).

**What an attestation is [read, ran].**

* **The format:** a Sigstore bundle,
  `application/vnd.dev.sigstore.bundle.v0.3+json`, holding a DSSE
  envelope over an in-toto Statement v1. The predicate is
  `https://slsa.dev/provenance/v1`, with `buildType
  https://actions.github.io/buildtypes/workflow/v1`. One statement names
  many subjects (21 in `cli/cli` v2.102.0's) [ran].
* **For a public repository** the leaf certificate comes from the
  Sigstore public-good Fulcio [read: [artifact attestations](https://docs.github.com/en/actions/concepts/security/artifact-attestations)]:
  * it is valid for 10 minutes;
  * its SAN is the signing workflow's URI at its ref;
  * the bundle carries one Rekor v1 entry, with an inclusion promise and
    an inclusion proof [ran].
* **For a private repository** the signer is GitHub's own Sigstore
  instance: no transparency log, an RFC 3161 timestamp instead, and it
  needs GitHub Enterprise Cloud. GitHub Enterprise Server does not
  support artifact attestations [read: [actions/attest](https://github.com/actions/attest)].
* **The certificate's extensions** (Fulcio OIDs `1.3.6.1.4.1.57264.1.*`,
  [oid-info](https://github.com/sigstore/fulcio/blob/main/docs/oid-info.md))
  carry:
  * the issuer (`.8`, `https://token.actions.githubusercontent.com`);
  * the Build Signer URI and digest (`.9`, `.10`);
  * the runner environment (`.11`);
  * the source repository's URI, commit, ref and numeric ID (`.12`–`.15`);
  * the owner's URI and ID (`.16`, `.17`);
  * the Build Config URI and digest (`.18`, `.19`).
* **A reusable workflow, which is this module's case** [ran on
  `aquaproj/aqua` v2.64.0, whose release calls another repository's
  reusable workflow]:
  * the SAN and Build Signer URI are the *reusable* workflow at the
    caller's pinned ref, and the Build Signer Digest is that commit;
  * the Source Repository URI, digest and ref are the *caller's*
    (`refs/tags/<tag>`);
  * the Build Config URI is the caller's top-level workflow;
  * the attestation is stored under the caller's repository.

  0013-MADR (`:210-215`) records the same for this module's publish
  workflow.

**Fetching one [read, ran].**

* **The API call:** `GET /repos/{owner}/{repo}/attestations/sha256:<hex>`.
  It answers anonymously for a public repository (HTTP 200 [ran]),
  charged to the core limit of 60 requests per hour per IP. Callers need
  read access, and a fine-grained token needs `attestations:read`
  [read: [REST attestations](https://docs.github.com/en/rest/repos/attestations)].
* **No inline bundle:** API version `2026-03-10` removed the `bundle`
  field from list responses
  [read: [breaking changes](https://docs.github.com/en/rest/about-the-rest-api/breaking-changes)].
  This module already sends that version (`github.go:22`). Each entry now
  has a `bundle_url`:
  * a blob-storage URL whose signature expires after one hour;
  * no GitHub credential, and no API rate-limit charge;
  * the bundle is Snappy *block*-compressed (`.json.sn`) [ran];
  * `gh` moved to it in [cli/cli#10185](https://github.com/cli/cli/pull/10185).
* **The change broke other native verifiers.** mise broke in February
  2026, and a third-party scanner in July 2026 (sources under More
  Information).
* **Sizes** [ran]: 4.8 KB (release attestation) and 12 KB (provenance)
  compressed, each fetched in about 0.2 s.

**Immutable releases' own attestation [read, ran].**

* GitHub signs a release attestation for every immutable release
  [read: [immutable releases GA, 2025-10-28](https://github.blog/changelog/2025-10-28-immutable-releases-are-now-generally-available/)].
* Its predicate is `https://in-toto.io/attestation/release/v0.2` [ran].
  Its subjects are `pkg:github/<owner>/<repo>@<tag>` with the commit's
  SHA-1, then every asset's SHA-256.
* It is signed by GitHub's own instance, even for a public repository:
  * the certificate subject is `CN=Attester`, with SAN
    `https://dotcom.releases.github.com`, valid for a year;
  * there is an RFC 3161 timestamp and no log entry [ran].
* `gh release verify` and `verify-asset` (gh 2.81.0) check it.
* **It proves nothing about the build** [inferred]: GitHub attests
  whatever an immutable release contains, so a release made with a stolen
  `contents: write` token is attested like any other. aqua's registry
  generator deliberately ignores it ([aqua#5247](https://github.com/aquaproj/aqua/pull/5247)).

**Verifying without `gh` [read, ran].**

* **`github.com/sigstore/sigstore-go`** v1.3.0 (2026-07-30, Apache-2.0,
  `go 1.25.8`) verified both kinds of bundle with `CGO_ENABLED=0` [ran]:
  * provenance against the public-good root, and the release attestation
    against GitHub's root;
  * a wrong digest and a wrong signer workflow each failed with a clear
    error;
  * each verify took 5–8 ms, and a cold TUF refresh of both roots
    0.97 s;
  * gh's embedded GitHub `root.json` v3 (expired 2025-04-11) still
    chained forward to v9.
* **The cost of a probe module** that imports `verify`, `root`, `tuf` and
  `bundle` [ran]:
  * 70 `go.mod` requirements (1 direct, 69 indirect);
  * 367 modules in `go list -m all`;
  * 71 modules and 555 packages linked, among them grpc, genproto,
    four OpenTelemetry modules, 22 go-openapi modules, `k8s.io/klog`,
    `go-containerregistry`, `certificate-transparency-go`, `rekor`,
    `go-tuf/v2`, `in-toto-golang` and protobuf;
  * a binary 18 MB larger than a stdlib baseline: 26.7 MB against 8.5 MB
    on darwin/arm64.
* **Its trusted roots come from TUF** [ran]:
  * `tuf-repo-cdn.sigstore.dev` for public good, and `tuf-repo.github.com`
    for GitHub's instance;
  * both `timestamp.json` files expire weekly;
  * GitHub's CA and TSA rotated six times between 2023-10 and 2026-06,
    each generation lasting about six months.
* **The hosts a run needs** are therefore four: `api.github.com`, the
  blob host, and the two TUF hosts, plus a writable cache.
* **A snapshot of the root embedded in the binary** fails on the first
  release signed after a rotation, and so strands the binaries in the
  field [inferred].
* **A stdlib-only verifier** would implement [read:
  [client spec](https://github.com/sigstore/architecture-docs/blob/main/client-spec.md);
  sizes inferred]:
  * DSSE PAE;
  * x509 path validation at the signing time;
  * embedded SCTs (RFC 6962 precertificate reconstruction);
  * Rekor v1 SETs over RFC 8785 JSON, inclusion proofs and checkpoints;
  * Rekor v2 shards, which rotate yearly
    ([Rekor v2 GA](https://blog.sigstore.dev/rekor-v2-ga/));
  * RFC 3161 CMS (no CMS in the stdlib);
  * a TUF client.

  That is about 2,000–3,000 lines of security-critical code chasing a
  moving specification.
* **`gh`'s own verifier** (`github.com/cli/cli/v2/pkg/cmd/attestation/verification`)
  is `sigstore-go` plus `golang/snappy`, inside the whole `gh` module. It
  is not a supported library API.

**`gh attestation verify`** [read: [manual](https://cli.github.com/manual/gh_attestation_verify),
[policy.go](https://raw.githubusercontent.com/cli/cli/trunk/pkg/cmd/attestation/verify/policy.go)]:

* **Identity flags:** `--repo`, `--signer-workflow`, `--signer-digest`,
  `--cert-oidc-issuer` (default the Actions issuer), and the source checks
  `--source-ref` and `--source-digest`.
* **Policy flags:** `--predicate-type` (default
  `https://slsa.dev/provenance/v1`), `--deny-self-hosted-runners`.
* **Offline inputs:** `--bundle`, `--custom-trusted-root`.
* **A reusable workflow needs `--signer-workflow` or `--signer-repo`.**
* `gh` refuses without a token even for a public repository
  ([aqua#3157](https://github.com/aquaproj/aqua/issues/3157); Homebrew's
  `attestation.rb`).
* It caches the TUF roots for a day under its own cache directory.

**Prior art [read].**

* **Homebrew** runs `gh attestation verify`:
  * it needs credentials, and retries on missing attestations;
  * it was turned on by default, then made opt-in again in 4.6.0
    (2025-08-05: `HOMEBREW_VERIFY_ATTESTATIONS` "must be explicitly set
    again");
  * its code notes a native Ruby verifier as the eventual fix.
* **aqua** runs a `gh` it installs itself, with an opt-out.
* **mise** verifies natively, by default. It has had a run of breakages:
  * the `bundle_url` change;
  * a new Rekor key type in the trusted root;
  * a hard-coded issuer;
  * repository transfers;
  * missing signer pinning;
  * a fail-open report of 2026-10-06, where a blocked TUF host still
    printed "verified" because only the release attestation was checked.
* **The Go self-update libraries** checked (`creativeprojects/go-selfupdate`,
  `minio/selfupdate`, `rhysd/go-github-selfupdate`) have no attestation
  support; each offers a checksum and a signature-verifier hook.

**What provenance adds to this pipeline [inferred from the above].**
Today an installed binary is bound by:

* HTTPS to `api.github.com`;
* an immutable release;
* GitHub's asset digest;
* the `SHA256SUMS` entry;
* the strict version policy.

What provenance adds, and what it does not:

* **It moves a release through the workflow.** It proves the binary, or
  the `SHA256SUMS` that pins it, came out of
  `publish-selfupdate-release.yml`, on a GitHub-hosted runner, for this
  program's repository and for `refs/tags/<tag>`.
* **It stops a stolen token from releasing directly.** A token with
  `contents: write` can create and publish an immutable release with
  assets it built itself, and the updater installs those today. With
  provenance checked, such a release fails, because only a workflow run
  can obtain the Fulcio certificate.
* **It does not stop a malicious tag.** The same token can push a commit
  and a `v*` tag, and the workflow then attests the attacker's build. The
  difference is that the malicious code is then in the repository's
  history and in a workflow run.

  0004-MADR's hardening list (`:826-829`) already names the control that
  closes this: "a tag ruleset that restricts creating `v*` tags to the
  owner". Provenance plus that ruleset turns "steal a token" into "steal
  the owner's account". Provenance alone turns it into "steal a token and
  leave a trail".

### Item 2: unreferenced local entries in a zip (N4)

**The rule and the code [read].**

* 0012-MADR §4 refuses an archive "when two tools read differently".
  0015's E1 enforced that for local and central names that differ.
* The library reads a zip through `archive/zip` over the downloaded file
  (`selfupdate/archive/unpack.go:136-153`, `:418`).
* `headerRecorder` (`:401`) captures each local header's offset, which
  `archive/zip` does not export. `checkLocal` (`:511`) compares that
  header with its central record.
* Nothing checks that the entries tile the file from offset 0 to the
  central directory:
  * a local entry that no central record names, in a gap or prepended,
    is invisible to the library;
  * Go accepts prepended data as `baseOffset` since Go 1.19
    ([golang/go df57592](https://github.com/golang/go/commit/df57592276bc26e2eb4e4ca5e77e4e2e422c7c6b));
  * trailing bytes after the EOCD comment are tolerated
    ([reader.go@go1.27.1](https://github.com/golang/go/blob/go1.27.1/src/archive/zip/reader.go)).

**The experiment [ran].** A zip whose central directory names one
`relay` (GOOD), with a second local `relay` (EVIL) that no central record
names. In `hidden-gap.zip` EVIL sits before the central directory; in
`hidden-prefix.zip` it is prepended.

| Reader | gap | prefix |
| :--- | :--- | :--- |
| this library's `Unpacker` (`v1.11.0`) | accepted, installs GOOD | accepted, installs GOOD |
| `cat file \| bsdtar -xvf -` (streaming) | lists `relay` twice, leaves EVIL | `relay` twice, leaves GOOD |
| `ditto -x -k` (macOS) | **EVIL**, exit 0 | GOOD |
| `jar -t < file` (`ZipInputStream`) | `relay` listed twice | not run |
| `bsdtar -xf file`, `unzip`, Python `zipfile`, `jar -tf` | GOOD | GOOD (`unzip` warns, exit 1) |

The library's own extractors read the central directory:

* the `Unpacker`;
* `install.ps1`, through `[IO.Compression.ZipFile]::OpenRead`
  (`install.ps1:153`);
* `install.sh`, through `unzip` on a file (`install.sh:334`).

The exposure is review: macOS's default extractor, and any streaming
reader, shows a different program from the one the updater installs.
Because the archive must still match `SHA256SUMS`, the attacker must
already control the build or the release [inferred].

**The research [read].**

* You et al., "My ZIP isn't your ZIP", USENIX Security 2025
  ([paper](https://www.usenix.org/conference/usenixsecurity25/presentation/you)),
  found 1,221 of 1,225 parser pairs affected. For the streaming class:
  "Detection requires checking for such holes and overlaps."
* uv's CVE-2025-54368 and PyPI's rules of 2025-08-07 now refuse local
  and central lists that do not match, trailing data and multiple EOCDs
  ([uv advisory](https://astral.sh/blog/uv-security-advisory-cve-2025-54368),
  [PyPI](https://blog.pypi.org/posts/2025-08-07-wheel-archive-confusion-attacks/)).

**A stdlib check closes it [ran].** A scratch prototype of about 90
lines:

* parses the EOCD and the zip64 locator by hand, requiring the comment
  to end at EOF;
* requires `cdOff + cdSize == eocdStart`, which refuses prepended data
  and junk;
* computes each entry's span as the recorded header up to the data plus
  the data descriptor;
* requires the sorted spans to tile `[0, cdOff)` exactly.

Its results:

* Go-written (`pack.go`'s shape) and Python-written zips pass;
* `hidden-gap.zip` gives "unreferenced bytes [1431876, 1431930) before
  the central directory";
* `hidden-prefix.zip` gives "data between the central directory and its
  end (or prepended data)".

The data descriptor's width (12 or 20 bytes, plus an optional
`PK\7\8`) is ambiguous for hostile input. It has to be read and checked
against the central record's CRC and sizes, not guessed [inferred, after
the USENIX paper's "self-consistent in both modes" construction].

### Item 3: a crash between `Apply` and `Commit` (N5)

**The code [read].**

* `Apply` (`selfupdate/session.go:229`):
  1. hard-links the target to `.<base>.selfupdate-bak-<n>` (`replace.go:72-77`);
  2. renames staging over the target and syncs the directory
     (`replace_unix.go:60-67`, `replace_windows.go:42-49`);
  3. runs the post-install probe.
* The replacement is held only in memory: `*replacement`
  (`session.go:134-138`). No journal or marker exists on disk.
* `Commit` (`:342`, `commitLocked` `:203`) removes the backup, or with
  `KeepPrevious` renames it to `.<base>.previous`.
* The standalone `Install` (`:150`) runs rename, probe and commit in one
  call: the same window, but short.
* A managed run (`managed.go:106-133`) runs `Stop`, `Apply`, `Reconcile`,
  `Start`, `WaitHealthy`, then `Commit`. The window spans the service's
  whole health wait. The reconcile receipt is memory-only too.
* Every non-dry-run session calls `removeLeftovers` (`leftovers.go:77`)
  from `beginSession`. `CleanupPending` (`standalone.go:58`) is the call
  a program makes at start. The sweep removes every regular
  `.<base>.selfupdate-bak-<digits>` (`isLeftover`, `:24`).
* `.selfupdate-kept-<n>` (`keptName`, `:50`) survives the sweep, but only
  `retainLocked` (`session.go:410`) gives that name, after a failed
  restore.

**What a crash leaves, by point [inferred from the code].**

| Crash point | On disk | What happens next |
| :--- | :--- | :--- |
| after the rename, before `Commit` (standalone) | the new binary at the target; the backup | The next session, often the *new* binary's own `CleanupPending`, deletes the backup. The previous binary is lost with no report. With `KeepPrevious`, `.previous` holds the release before that. |
| managed, after `Apply`, before `Start` | the same; the service stopped; the definition rewritten if `Reconcile` ran | The service returns at reboot or login on the new binary, whose service health was never checked. The definition's `Restore` never runs. |
| managed, during `WaitHealthy` | the new binary running | The service manager restarts the new binary if it fails. Its start-up sweep deletes the backup, so no rollback remains. |

The lock (`flock`, `LockFileEx`) is released by the kernel when the
process dies, so nothing blocks the sweep.

**How others handle the window [read].**

* **Mender:** "ArtifactCommit makes the update persistent". A failure or
  "spontaneous reboot" before it runs `ArtifactRollback` from a persisted
  state store ([state scripts](https://docs.mender.io/artifacts/state-scripts)).
* **Android A/B** marks a boot successful after verification
  ([A/B](https://source.android.com/docs/core/ota/ab)).
* **systemd boot counting** keeps the counter in the file name and
  changes it by atomic renames
  ([automatic boot assessment](https://systemd.io/AUTOMATIC_BOOT_ASSESSMENT/)).
* **RAUC** marks the slot good after verification
  ([integration](https://rauc.readthedocs.io/en/latest/integration.html)).
* **Chromium on Windows** persists a pending marker, and the next launch
  *completes* the update
  ([install_worker.cc](https://chromium.googlesource.com/chromium/src/+/main/chrome/installer/setup/install_worker.cc)).
* **Squirrel.Windows** keeps the previous version's directory, with "no
  built-in support for rolling back".
* **Velopack's** rollback is commented out.
* **No Go self-update library** has a confirm phase or recovery at the
  next start: go-update, minio, fynelabs, creativeprojects, rhysd.

The pattern among those that recover is a durable marker, written before
the irreversible step, that the next start reads.

### Item 4: `:` and other names Windows folds

**The code [read].**

* `checkName` (`selfupdate/archive/unpack.go:264`) refuses `\`, NUL, a
  leading `/` and `..` everywhere. It then requires
  `filepath.IsLocal(filepath.FromSlash(clean))` *on the host it runs on*.
  On Windows, Go's `IsLocal` refuses any `:` and the reserved device
  names; on Linux and macOS it accepts them.
* Go's own comment: "Colons are only valid when marking a drive letter
  … Rejecting any path with a colon is conservative but safe"
  (`internal/filepathlite/path_windows.go`). Whether a reserved name with
  an extension is reserved depends on the Windows version.
* `checkPortable` (`:282`, 0015 E2/N1) allows every byte 0x20–0x7e, so
  `< > " | ? *` pass on every host.
* `releasespec`'s `validatePath` already refuses `\`, `:` and NUL in an
  extra's source path on every host (`releasespec/validate.go:132`).
* Product names match `^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`, so the
  program itself never has a `:`. The build writes one entry, the
  product (`internal/cmd/selfupdate-release/pack.go:35-63`).
* `UnpackOptions.Member` is checked only with `fs.ValidPath`
  (`unpack.go:68`), which allows `:`.

**The platforms [read].**

* Windows reserves `< > : " / \ | ? *` and the device names CON, PRN,
  AUX, NUL, COM1–9 and LPT1–9; "NUL.txt and NUL.tar.gz are both
  equivalent to NUL"
  ([naming a file](https://learn.microsoft.com/en-us/windows/win32/fileio/naming-a-file)).
* `name:stream` names an NTFS alternate data stream, and
  `name::$DATA` the main stream
  ([file streams](https://learn.microsoft.com/en-us/windows/win32/fileio/file-streams)).
* Windows `tar.exe` (libarchive) rewrites `: * ? " < > |` to `_`
  ([archive_write_disk_windows.c](https://raw.githubusercontent.com/libarchive/libarchive/master/libarchive/archive_write_disk_windows.c)).
  .NET's `ExtractToDirectory` does the same
  ([Archiving.Utils.Windows.cs](https://raw.githubusercontent.com/dotnet/runtime/main/src/libraries/Common/src/System/IO/Archiving.Utils.Windows.cs)).
* `install.ps1` uses `ExtractToFile` to a fixed path, so it is not
  affected.

**The differential [inferred].** Take an archive holding `my_tool.exe`
and `my:tool.exe`, for the product `my_tool`:

* the `Unpacker` accepts it on Linux and macOS, for example in a dry run
  for another platform;
* it refuses it on Windows;
* a reviewer's `tar.exe` on Windows writes one `my_tool.exe`, the last
  one in the archive.

So the verdict depends on the host, against 0012 §4's "two tools" rule,
which N1 already applied to case and to a trailing dot or space. No
archive the build makes contains any of these names. A third-party
archive with such a name in an entry that is not the program would
become refused.

### Item 5: `queue: max` (N9)

**The code [read].**

* The publish job's concurrency
  (`.github/workflows/publish-selfupdate-release.yml:42-48`) is group
  `go-selfupdate-lib-publish-${{ github.repository }}`,
  `cancel-in-progress: false`, with the default queue.
* `scripts/workflow-shape_test.sh:93-95` asserts the group and that
  `cancel-in-progress` is false.
* CI runs `actionlint@v1.7.12` (`.github/workflows/ci.yml:189`).

**The platform [read].**

* GitHub's changelog of 2026-05-07, "concurrency groups now allow larger
  queues":
  * "Previously, a concurrency group could have one run in progress and
    one pending run. If another run entered the group, the pending run
    was canceled and replaced";
  * `queue: max` allows "up to 100 queued jobs or workflow runs per
    concurrency group", only with `cancel-in-progress` false
    ([changelog](https://github.blog/changelog/2026-05-07-github-actions-concurrency-groups-now-allow-larger-queues/)).
* The same key is documented for `jobs.<job_id>.concurrency`
  ([workflow syntax](https://docs.github.com/en/actions/reference/workflows-and-actions/workflow-syntax#jobsjob_idconcurrency)).
* The order is "not guaranteed". GitHub Enterprise Server's documentation
  (3.18–3.22) has no `queue`.
* **No released actionlint accepts `queue`.** The latest is v1.7.12
  (2026-03-30), whose parser allows only `group` and
  `cancel-in-progress`. [rhysd/actionlint#654](https://github.com/rhysd/actionlint/pull/654),
  which adds it, has been open and unmerged since 2026-04-22.
* GitHub creates no tag events at all when more than three tags are
  pushed at once
  ([events](https://docs.github.com/en/actions/reference/workflows-and-actions/events-that-trigger-workflows)).

**What three quick tags do today [inferred from the documentation].**

1. Tag 1's publish job runs, and tag 2's waits.
2. Tag 3's arrives, so tag 2's pending job is cancelled.
3. Tag 2 then has no release, not even a draft, and nothing fails loudly.
4. The recovery is to re-run the cancelled job.

The records overstate the guarantee:

* `docs/architecture.md:373-375`: "so two tags of one repository publish
  one after the other";
* `docs/guides/building-releases.md:195-198`: "so two tags never race the
  latest flag".

Both are true only for two tags.

## Decision Drivers

* **The security posture does not erode by default, and a new module
  needs a record naming it** (0004-MADR drivers; `AGENTS.md`). Item 1
  must be opt-in, and its form decides whether this module's dependency
  floor moves.
* **The owner's verification requirements** (0004-REPORT `:40-46`): pure
  Go, cgo off, no external binary at run time, clock-free, three OSes.
  No form of item 1 meets all of them (Item 1 above), so the record must
  say which one it gives up.
* **A configured check fails closed.** mise's fail-open report shows the
  cost of a check that skips itself silently.
* **The fleet's programs are developer command-line tools first,** with
  some services. An exec check suits a developer's machine with `gh`
  logged in, and does not suit an unattended service.
* **A refusal 0012 §4 already requires belongs in a patch release,** as
  0015's E1–E4 did in `v1.10.1`.
* **Consumers pin the workflows and installers by a release tag's
  commit,** so a workflow change reaches them only through a tag.
* **Every new check is seen failing on a planted break** first.
* **The owner prefers a complete, extensible surface** to cuts made only
  for scope.

## Considered Options

**Item 1, runtime provenance:**

* **1A. Record only.** Provenance stays out of band: the installers'
  `--verify-attestation` and the guide's step 10.
* **1B. A `Policy` and an exec verifier in this module.**
  `selfupdate/verify/ghattest`, stdlib only, runs `gh attestation verify`
  through `service.Runner`.
* **1C. `sigstore-go` in this module,** in its main `go.mod`.
* **1D. `sigstore-go` in a nested module in this repository.**
* **1E. `sigstore-go` in a separate repository** implementing
  `selfupdate.ManifestVerifier`.
* **1F. A stdlib-only Sigstore verifier.**
* **1G. GitHub's release attestation only.**

**Item 2, unreferenced zip entries:**

* **2A. Record only.**
* **2B. Refuse a zip whose entries do not tile the file.**
* **2C. 2B, and refuse data descriptors.** The build would write sizes up
  front.

**Item 3, a crash between `Apply` and `Commit`:**

* **3A. Record only.**
* **3B. A durable pending journal; the next session keeps the backup.**
* **3C. 3B, and the next session rolls back an uncommitted update.**
* **3D. Never sweep backups.**

**Item 4, names Windows folds:**

* **4A. Record only.**
* **4B. Refuse `:` on every host.**
* **4C. Refuse Windows' reserved characters and device names on every
  host.**

**Item 5, `queue: max`:**

* **5A. Correct the documentation now, and add `queue: max` when a
  released actionlint accepts it.**
* **5B. Add `queue: max` now, with an actionlint suppression.**
* **5C. Move CI to an actionlint fork that accepts `queue`.**

## Decision Outcome

Chosen option: **1B, 2B, 3B, 4C and 5A**, in three tracks, because
together they close all five open items with no new module and no
loosened check. 1E is left as a named follow-up for its own records, if
a program needs unattended provenance checks. The owner answered the
questions below on 2026-10-08 (Owner questions); 3B reports a kept
backup through a new listing method, as question 4's answer chose.

* **1B** gives the opt-in runtime check 0004 §1 anticipated, with no new
  module. Its `Policy` is shaped so that an in-process verifier (1E) can
  take it unchanged.
* **2B and 4C** make the archive verdict the same for every reader and
  every host, which 0012 §4 already requires. Each is a refusal with a
  scratch-proven red case.
* **3B** stops a crash from silently destroying the only previous binary.
  It makes no automatic decision at the next start.
* **5A** fixes what the records claim, and leaves the lint gate whole.

### 1. On `main`, with no release

* **5A's documentation:**
  * `docs/architecture.md:373-375` and `docs/guides/building-releases.md:195-198`
    say what a third pending tag does, and how to recover: re-run the
    cancelled publish job;
  * the guide adds GitHub's "more than three tags" limit.
* **The trigger to add `queue: max`:** an actionlint release that
  accepts `queue`. The pin bump, the key and a `workflow-shape_test.sh`
  assertion that `queue` is `max` then land in one change, in the next
  release, since the workflow is pinned by tag.
* **The 0004-MADR P3 table:** this record's rows replace the five items.
  The 0015-PLAN row gains its line numbers (`:57-68`).
* **The tag ruleset (question 3):** the building guide's step 4
  recommends a ruleset that restricts creating `v*` tags to the owner,
  and says what provenance does not stop without one: a malicious tag
  pushed through the workflow is attested like any other.

### 2. `v1.11.1`: refusals the documented rule already requires

* **2B, in `selfupdate/archive`, stdlib only.** A zip is refused, joined
  with `ErrIntegrity` like E1, unless:
  * the EOCD's comment ends at the end of the file;
  * the central directory ends where the EOCD (or zip64 record) begins,
    so `baseOffset` is 0;
  * the local entries, each from its recorded header to the end of its
    data and data descriptor, tile `[0, cdOff)` with no gap or overlap.

  A data descriptor is read and must match the central record's CRC and
  sizes. Its width is decided by that match, never guessed.

  **Tests and records:** `hidden-gap.zip` and `hidden-prefix.zip` become
  table cases and `FuzzUnpackZip` seeds, red on `v1.11.0`. 0012-MADR §4
  gets an amendment naming the rule.
* **4C, in `checkPortable`:**
  * an element containing `< > : " | ? *`, or whose name before its
    first dot is a reserved device name, is refused on every host;
  * the device names are CON, PRN, AUX, NUL, COM0–9, LPT0–9, CONIN$ and
    CONOUT$, compared case-insensitively;
  * `NewUnpacker` applies the same rule to `UnpackOptions.Member`.

  **Records:** 0012-MADR §4 gets an amendment beside N1's.
* **Release:** `make apicheck` must report `compatible with v1.11.0`.
  The migration guide gains "From v1.11.0 to v1.11.1".

### 3. `v1.12.0`: the journal and the provenance verifier

* **3B: the journal.**
  * **Before** `Apply`'s rename, `replaceLocked` writes
    `.<base>.selfupdate.pending` beside the target: temp file, fsync,
    rename, directory fsync. It holds:
    * a schema number;
    * the backup's base name;
    * the old and new digests;
    * the phase;
    * for a managed install, the product.
  * `Commit` and `Rollback` remove the journal last.
  * A session that finds a journal it does not own, under the lock,
    renames the named backup to the existing `.selfupdate-kept-<n>` name
    (`keptName`, the name `retainLocked` already gives a backup that is
    the only copy, `session.go:410`), so the sweep never removes it. It
    then removes the journal.
  * **Reporting it needs a new surface.** `Result.PendingBackup` is
    filled only within the run that kept a backup (`types.go:156-164`),
    and `CleanupPending` returns only an `error` (`standalone.go:58`).
    Decided (question 4): **a new method that lists the kept backups**
    beside the target: every `.<base>.selfupdate-kept-<n>`, whether a
    crash or a failed restore left it. `CleanupPending` keeps its
    contract and returns no new error. The method's name, receiver and
    result type are settled in the PLAN; it is an addition, so
    `make apicheck` stays compatible.
  * The journal's base names are validated as the Windows cleanup receipt's
    are (`cleanup_windows.go:19-60`).
  * No automatic rollback (3C) and no replay of a reconcile receipt; those
    wait for their own record (question 4).
* **1B: `selfupdate/verify/ghattest`**, a new package, stdlib plus this
  module.
  * **`Policy`:**
    * `Repository`: the program's own repository, required;
    * `SignerWorkflow`: default the publish workflow's path,
      `maccavelli/go-selfupdate-lib/.github/workflows/publish-selfupdate-release.yml`,
      exported as a constant;
    * `SignerDigests`: an optional allow-list of that workflow's commits;
    * `AllowSelfHostedRunners`: false by default.
  * **Each check also requires:**
    * the predicate `https://slsa.dev/provenance/v1`;
    * the issuer `https://token.actions.githubusercontent.com`;
    * the source ref `refs/tags/<Release.Tag>`.
  * **The exec verifier** implements `selfupdate.ManifestVerifier` (question
    2). It writes the downloaded `SHA256SUMS` to a private temporary file,
    then runs:

    ```text
    gh attestation verify <file> --repo <Repository>
       --signer-workflow <SignerWorkflow> [--signer-digest <d>]
       --source-ref refs/tags/<tag> --predicate-type https://slsa.dev/provenance/v1
       --cert-oidc-issuer https://token.actions.githubusercontent.com
       [--deny-self-hosted-runners]
    ```

    It runs this through a `service.Runner`, with an absolute `gh` path
    and an environment the caller supplies; nothing is looked up on
    `PATH`.

    Checking the manifest binds every asset through the digest the
    updater already checks, costs one small file, and refuses before any
    binary byte is fetched.
  * **A `Verifier` on the asset itself** is offered too, for a caller who
    wants it.
  * **Any failure fails the run,** joined with `ErrIntegrity`. That covers
    a non-zero exit, an exit 4 (not logged in), a timeout and a missing
    `gh`. The error names the `gh` exit code and a bounded excerpt of its
    stderr. Nothing is skipped.
  * **No default anywhere enables it.** A program adds it to
    `Config.ManifestVerifiers` itself.
  * **Records:**
    * a depguard rule `selfupdate-verify-ghattest`
      (`$gostd`, `…/selfupdate$`, `…/selfupdate/service$`), and its
      exclusion from `other-packages`;
    * the `AGENTS.md` scope paragraph;
    * `docs/architecture.md`'s rule count;
    * `selfupdate/doc.go`'s Integrity section;
    * a recipe in `docs/guides/extending-selfupdate.md`;
    * the migration guide's "From v1.11 to v1.12".
* **`make apicheck`** must report `compatible with v1.11.1`: every change
  is an addition.

### Not decided here

* **1E**, the in-process `sigstore-go` verifier, in its own repository
  with its own records, implementing this package's `Policy`.
* **3C**, automatic rollback, and persisting a reconcile receipt.
* **A build-side attestation, and the move to `actions/attest`**
  (0013-MADR §10).
* **A tag ruleset in each consumer repository.** That is each
  repository's own setting. 0004-MADR's hardening list names it
  (`:826-829`), but the building guide does not mention it. Question 3
  asks whether the guide should.

### Consequences

* Good, because a program can refuse a release that its own workflow did
  not attest, with no new module in this repository's graph.
* Good, because the archive verdict no longer depends on the reader or
  the host. That closes the last two differentials 0015 recorded open.
* Good, because a crash during an update no longer deletes the previous
  binary without a trace.
* Good, because the documentation stops promising more than the publish
  workflow's default queue gives.
* Neutral, because 1B adds a run-time dependency on an authenticated
  `gh`, but only for a program that opts in. It is the installers'
  existing rule.
* Bad, because 1B does not suit an unattended service, where `gh` is
  rarely installed or logged in. Such a program has to wait for 1E, or
  rely on immutability and checksums as today.
* Bad, because 1B's check depends on `gh`'s flags and output, which
  change between `gh` versions. The PLAN pins a minimum `gh` version
  after probing it.
* Bad, because 2B and 4C refuse archives a third-party tool might write:
  a self-extracting or prepended zip, a trailing signature block, or an
  entry named `aux.txt`. None of these is produced by this module's
  build.
* Bad, because 3B adds an on-disk format that must stay readable across
  releases.
* Bad, because 5A leaves a three-tag burst able to lose a release until
  actionlint accepts `queue`. The mitigation is documented, not
  automatic.

### Confirmation

* **Each new check has a red case on `v1.11.0`, and a planted break in
  a scratch copy that makes it fail:**
  * 2B: `hidden-gap.zip`, `hidden-prefix.zip`, a trailing-bytes zip, and a
    data descriptor whose CRC disagrees;
  * 4C: `my:tool.exe` beside `my_tool.exe`, `aux.txt`, `x?y`, and a
    `Member` of `a:b`;
  * 3B: a test that stops after `Apply` and starts a new session, then
    expects the backup kept under its `-kept-` name, `CleanupPending`
    returning nil, and the listing method naming it;
  * 1B: a stub `gh`, a Runner fake, that asserts the exact argument list
    and fails closed on exits 1 and 4, a timeout, and a missing binary.
* **A live test of 1B** runs the real `gh` against a real attested
  `SHA256SUMS`, both one that passes and one with the wrong signer
  workflow.
* **`make gate` ends `overall=0`,** and `make apicheck` reports
  compatible, on each release commit.
* **`go mod tidy -diff` is clean,** and `go.mod` is unchanged by this
  record.

## Pros and Cons of the Options

### 1A. Record only

* Good, because it costs nothing and adds no run-time dependency.
* Good, because the threat it leaves open (a release made directly with a
  stolen token) is narrowed by immutable releases and a tag ruleset.
* Bad, because 0004 §1 marked `verify/ghattest` "yes" and G9 named the
  gap: "The release workflow already produces build attestations that
  nothing verifies at runtime."
* Bad, because an update in place stays weaker than an install with
  `--verify-attestation`.

### 1B. A `Policy` and an exec verifier in this module

* Good, because it is stdlib only. The dependency floor and depguard's
  `module` rule are unchanged.
* Good, because GitHub maintains the Sigstore verification, the trusted
  roots, TUF, the `bundle_url` storage and the Rekor changes inside `gh`.
  Each of those broke a native verifier in 2026 (mise).
* Good, because it reuses a proven pattern: `codesign`'s absolute-path
  `service.Runner`.
* Good, because checking `SHA256SUMS` costs one small file and refuses
  before any binary is fetched.
* Neutral, because it is the same rule as the installers' opt-in.
* Bad, because it needs `gh` installed and logged in, even for a public
  repository. That breaks the owner's "no external binary at run time"
  requirement for this check.
* Bad, because `gh`'s output and flags are a moving interface.

### 1C. `sigstore-go` in this module

* Good, because it is in process, works on all three OSes with cgo off,
  and verifies in milliseconds [ran].
* Bad, because it adds about 70 requirements to this module's `go.mod`,
  for every consumer's module graph, against 0004's dependency floor.
* Bad, because a linking program grows by about 18 MB.
* Bad, because it needs four egress hosts and a writable TUF cache.

### 1D. `sigstore-go` in a nested module here

* Good, because programs that do not import it pay nothing.
* Bad, because 0004 Option D's objections apply:
  * CI, vulnerability-scan scope and path-prefixed tags;
  * lint does not reach nested modules today (0013-PLAN D4;
    `docs/architecture.md:431-434`).

### 1E. `sigstore-go` in a separate repository

* Good, because this module's floor, CI and tags are untouched.
* Good, because it is the only in-process option that suits a service.
* Neutral, because it can implement 1B's `Policy` unchanged.
* Bad, because a second repository needs its own records and releases,
  and must track `sigstore-go` and GitHub's trust-root changes.

### 1F. A stdlib-only Sigstore verifier

* Good, because it adds no module.
* Bad, because it is about 2,000–3,000 lines of security-critical
  cryptography: SCTs, Rekor v1 and v2, RFC 3161 CMS and TUF.
* Bad, because it chases a moving specification, which is exactly where
  native verifiers broke in 2026.

### 1G. GitHub's release attestation only

* Good, because every immutable release has one, with no workflow change.
* Bad, because GitHub signs whatever the release contains, so it proves
  nothing about the build, and a stolen-token release passes.
* Bad, because GitHub's own roots rotate about every six months, which
  still needs TUF.

### 2A. Record only

* Good, because it costs nothing.
* Bad, because macOS's default extractor shows a different program from
  the one the updater installs [ran], against 0012 §4.

### 2B. Refuse a zip whose entries do not tile the file

* Good, because it closes the gap the experiment shows, with the stdlib,
  inside `archive/unpack.go`.
* Good, because it is the rule set uv and PyPI adopted in 2025, plus the
  hole check the USENIX paper recommends.
* Bad, because it refuses self-extracting and prepended zips, which this
  module's build never writes.

### 2C. 2B, and refuse data descriptors

* Good, because it removes the descriptor-width ambiguity entirely.
* Bad, because the build's `pack.go` writes descriptors today, so the
  build must change to `CreateRaw`, and existing releases would be
  refused.

### 3A. Record only

* Good, because it costs nothing.
* Bad, because a crash deletes the only previous binary with no report.
  That is the outcome 0015's B1 forbade for every other path.

### 3B. A durable pending journal; the next session keeps the backup

* Good, because the backup survives under an existing name, which no
  sweep removes.
* Neutral, because reporting it at the next start needs a new sentinel
  or method: no existing field reaches `CleanupPending`'s caller.
* Good, because the next start makes no automatic decision. The new
  binary, which may be the faulty one, does not roll itself back.
* Neutral, because it reuses the Windows cleanup receipt's pattern.
* Bad, because it adds an on-disk format, and a write and fsync to every
  update.

### 3C. Roll back an uncommitted update at the next session

* Good, because it is Mender's rule: an uncommitted update is rolled
  back.
* Bad, because the code that decides is the *new* binary's start-up. It
  would roll back the binary that is running it, and a managed rollback
  needs the reconcile receipt (`ReconcileResult.State` is `any`)
  persisted.

### 3D. Never sweep backups

* Good, because it adds no format.
* Bad, because the sweep cannot tell a crash's backup from an aborted
  staging copy, so leftovers each as large as the binary accumulate.
  That was 0010's B11.

### 4A. Record only

* Bad, because the verdict depends on the host.

### 4B. Refuse `:` everywhere

* Good, because it is one line, and matches `releasespec.validatePath`.
* Bad, because `* ? " < > |` and the device names still fold on Windows.

### 4C. Refuse the Windows-reserved set everywhere

* Good, because every host returns the same verdict, completing N1.
* Bad, because it is about 20 lines, and could refuse a third-party
  archive with such a name in a non-program entry.

### 5A. Correct the documentation now; add `queue: max` later

* Good, because the records stop overstating, and the lint gate stays
  whole.
* Bad, because a three-tag burst can still lose a release until
  actionlint is released with `queue`.

### 5B. Add `queue: max` now with a suppression

* Good, because the burst no longer loses a release.
* Bad, because it loosens a check (an actionlint `ignore:` pattern).
* Bad, because GitHub Enterprise Server callers could not parse the
  reusable workflow [inferred].

### 5C. Move CI to an actionlint fork

* Good, because the key is accepted.
* Bad, because it is a new tool and a supply-chain decision, and the
  fork is a different module.

## Owner questions

**Answered (2026-10-08).** Asked one by one, the owner chose:

| # | Question | Answer |
| :--- | :--- | :--- |
| 1 | Item 1's form | 1B now, 1E left to its own records (recommended) |
| 2 | What 1B checks | `SHA256SUMS`, through a `ManifestVerifier` (recommended) |
| 3 | The tag ruleset | the guide recommends it (recommended) |
| 4 | Item 3 at the next session | keep the backup, and report it through a new method that lists kept backups |
| 5 | Item 4's set | every Windows-reserved character and device name (recommended) |
| 6 | Item 2's strictness | tiling, no prepended or trailing data, descriptors checked (recommended) |
| 7 | Item 5 | documentation now, `queue: max` when actionlint accepts it (recommended) |
| 8 | Tracks | `v1.11.1` for 2B and 4C, `v1.12.0` for 3B and 1B (recommended) |

The questions as asked follow, recommended answers first.

1. **Item 1's form.**
   * 1B now, with 1E left to its own records when a service needs it
     (recommended).
   * 1E now, with this repository adding only the `Policy`.
   * 1A, record only.
2. **What 1B checks.**
   * `SHA256SUMS`, through a `ManifestVerifier` (recommended): one small
     file, before any binary download, and it binds every asset.
   * Each asset, through a `Verifier`.
   * Both.

   Both forms are built either way. This chooses the one the
   documentation recommends.
3. **The tag ruleset.** Provenance's value depends on who can push a `v*`
   tag.
   * The building guide's step 4 recommends a ruleset restricting `v*`
     creation, and says what provenance does not stop without one
     (recommended).
   * Leave the guide as it is.
4. **Item 3 at the next session.**
   * Keep the backup as `-kept-<n>`, and report it with
     `ErrInterruptedUpdate` from `CleanupPending` plus a warning in the
     next run's `Result.Warnings` (recommended).
   * Keep it, and report it through a new method that lists kept
     backups.
   * Roll back an uncommitted update (3C), as its own record.
   * Record only (3A).
5. **Item 4's set.**
   * Every Windows-reserved character and device name (recommended).
   * `:` only (4B).
6. **Item 2's strictness.**
   * Tiling, no prepended or trailing data, and descriptors checked
     against the central record (recommended).
   * 2C, refusing descriptors.
7. **Item 5.**
   * Documentation now, and `queue: max` when actionlint accepts it
     (recommended).
   * 5B, `queue: max` with a suppression now.
8. **Tracks.**
   * `v1.11.1` for 2B and 4C, and `v1.12.0` for 3B and 1B (recommended).
   * One `v1.12.0` for everything.

## Amendments

### A1 (2026-10-08): decisions and corrections from planning

*Status: accepted (2026-10-08), with
[0017-PLAN-verify-build-provenance-and-close-0015-open-items.md](0017-PLAN-verify-build-provenance-and-close-0015-open-items.md)
("Approved to proceed").* The PLAN's decisions made while planning, and
its corrections to this record, copied here as the PLAN states them.

**Decisions N1–N14:**

| # | Item | Decision | Phase |
| :--- | :--- | :--- | :--- |
| N1 | 2B | The local header's data-descriptor flag (0x8) must equal the central record's. Streaming readers use the local one. | P1 |
| N2 | 2B | Exactly one of a descriptor's four readings may match the central record's CRC and sizes: 12 or 20 bytes, each with or without `PK\x07\x08`. None is "does not match"; two is "reads two ways". | P1 |
| N3 | 2B | A zip64 end record, when its locator is present, is always checked. It must end at its locator and agree with every end-record field that is not saturated. Python and Info-ZIP read it even when nothing is saturated. | P1 |
| N4 | 2B | The central records must fill the central directory exactly: the sum over entries of `46 + len(Name) + len(Extra) + len(Comment)` equals the directory's size. archive/zip reads records by count, and ignores slack after them. | P1 |
| N5 | 4C | A device stem is the element before its first dot, with trailing spaces trimmed, as Go's `isReservedName` does (`internal/filepathlite/path_windows.go:98`). COM0 and LPT0 are included, as the MADR lists them. | P2 |
| N6 | 4C | `UnpackOptions.Member` must pass the whole of `checkPortable`: E2's rules and 4C's. | P2 |
| N7 | 3B | The journal is written inside `replaceTarget`, after `backupFile` and before the rename, through a `beforeRename` callback. The backup's name exists only there (`replace_unix.go:60-66`, `replace_windows.go:38-48`). | Q1 |
| N8 | 3B | One phase value, `"applying"`. Every recovery decision comes from what is on disk, not from the phase. An unknown phase is read as `"applying"`. | Q1 |
| N9 | 3B | `Apply` and `Install` refuse to start while an earlier journal is still pending (one recovery could not resolve). The error names the journal's path. | Q1 |
| N10 | 3B | `KeptBackups` lives on `*StandaloneInstaller`, the type that has `CleanupPending`, and returns `[]KeptBackup{Path, Size, ModTime}` with JSON tags. `*ManagedInstaller` delegates to its inner installer when that has the method, and otherwise returns an error. | Q1 |
| N11 | 1B | `gh` runs with `--format json`. The verifier re-checks every policy field in the certificate gh reports, and the subject digest. That is how `SignerDigests` with more than one entry works, since `--signer-digest` takes one value, and it guards against a `gh` that ignores a flag. | Q2 |
| N12 | 1B | The arguments include `--hostname github.com`, so `GH_HOST` cannot redirect the check. | Q2 |
| N13 | 1B | `Options.Env` nil means this process's environment. The package always appends `GH_PROMPT_DISABLED=1`, `GH_NO_UPDATE_NOTIFIER=1`, `GH_SPINNER_DISABLED=1` and `NO_COLOR=1`. `gh` needs `HOME` or `GH_CONFIG_DIR`, or `GH_TOKEN`, to find its credentials, so a fixed environment like codesign's (`codesign.go:22`) would break its login. | Q2 |
| N14 | 1B | `Options.Timeout` defaults to 2 minutes; a negative value is refused. One check took 3.5–4.1 s in the probe below. `gh`'s version is not checked: a `gh` without a flag exits 1, which fails closed. | Q2 |

**Corrections:**

1. **The journal's name check** is the portable `validateReceiptBackup`
   (`cleanup.go:21-29`), plus `keptName`'s digits rule (`leftovers.go:50`).
   The MADR cites only the receipt's struct and its version checks
   (`cleanup_windows.go:19-60`).
2. **2B is about 150 lines of code, not about 90.** That is the
   prototype's count, which covers N1–N3. N4 is not in the prototype; P1
   adds it and its test. Go's zip reader keeps each central record's raw
   name, extra field and comment (`reader.go:386-388` at go1.27.1), so
   the sum N4 uses can be computed from `zip.File`.
3. **`gh`'s flags were probed** (the MADR's "Not verified", first item),
   with `gh` 2.102.0 on 2026-10-08. The probe used `aquaproj/aqua`
   v2.64.0's `aqua_2.64.0_checksums.txt`, which a reusable workflow in
   another repository attests: `suzuki-shunsuke/go-release-workflow`, at
   `cf03c29d97518871efb36bbb80bcc01a19645b49`. That is this module's
   shape.

   | Case | Exit | gh's message |
   | :--- | :--- | :--- |
   | every flag 1B passes, matching | 0 | — |
   | `--source-ref refs/tags/v2.63.0` | 1 | "expected SourceRepositoryRef to be refs/tags/v2.63.0, got refs/tags/v2.64.0" |
   | `--source-ref v2.64.0` (no `refs/tags/`) | 1 | "expected SourceRepositoryRef to be v2.64.0, got refs/tags/v2.64.0" |
   | `--signer-digest` wrong | 1 | "expected BuildSignerDigest to be 0000…, got cf03c29d…" |
   | another signer workflow, or none | 1 | "verifying with issuer \"sigstore.dev\"" |
   | another `--repo` | 1 | "HTTP 404: Not Found (…/attestations/sha256:…)" |
   | `--cert-oidc-issuer` wrong | 1 | "expected Issuer to be https://example.com, got https://token.actions.githubusercontent.com" |
   | another `--predicate-type` | 1 | "HTTP 404: Not Found" |
   | a changed file | 1 | "HTTP 404: Not Found" |
   | no file | 1 | "failed to open local artifact" |
   | no credentials (`GH_CONFIG_DIR` empty, no token) | 4 | "To get started with GitHub CLI, please run: gh auth login" |

   * **The JSON output** (`--format json`, 17.8 KB) is an array of
     `{attestation, verificationResult}`.
     * `verificationResult.signature.certificate` carries:
       * `buildSignerURI`, `buildSignerDigest`;
       * `sourceRepositoryURI`, `sourceRepositoryRef`,
         `sourceRepositoryDigest`;
       * `runnerEnvironment`, `issuer`.
     * `verificationResult.statement` carries `predicateType` and
       `subject[].digest.sha256`.
   * **The values seen:**
     * `buildSignerURI` is the reusable workflow `@<sha>`;
     * `sourceRepositoryURI` is the caller, `https://github.com/aquaproj/aqua`;
     * `sourceRepositoryRef` is `refs/tags/v2.64.0`;
     * `runnerEnvironment` is `github-hosted`.

   The minimum `gh` the documentation names is the one probed, 2.102.0
   (N14).
4. **`SignerDigests` cannot map onto `--signer-digest`,** which takes one
   value. N11 resolves it.
5. **The listing method** is `KeptBackups` on `*StandaloneInstaller`
   (N10). `Installer` and `TwoPhaseSession` (`types.go:494-539`) take no
   new method, by 0004's compatibility rule.
6. **`docs/README.md:59`** still says "§6 to §10", though §11 exists. D1
   corrects it.

The "Not verified" section's first item is answered by correction 3.

## More Information

### Related records

* [0004-MADR-evolve-selfupdate-api-and-tui-support.md](0004-MADR-evolve-selfupdate-api-and-tui-support.md):
  §1, §6, G9, Option D, P3's open-work table.
* [0004-REPORT-release-signing-research.md](../reports/0004-REPORT-release-signing-research.md):
  the owner's verification requirements, and the earlier dependency
  count.
* [0012-MADR-archive-assets-and-macos-codesign.md](0012-MADR-archive-assets-and-macos-codesign.md):
  §4's refusals, which items 2 and 4 amend.
* [0013-MADR-build-and-stage-release-workflow.md](0013-MADR-build-and-stage-release-workflow.md):
  the publish workflow as the attestation's signer (`:210-215`), and §10.
* [0014-MADR-shared-installer-templates.md](0014-MADR-shared-installer-templates.md):
  Q2, the installers' opt-in attestation check.
* [0015-MADR-remediate-third-debugging-pass-findings.md](0015-MADR-remediate-third-debugging-pass-findings.md)
  and [0015-PLAN-remediate-third-debugging-pass-findings.md](0015-PLAN-remediate-third-debugging-pass-findings.md):
  E1, B1, F9 and decisions N4, N5, N9.

### Sources

**GitHub:**

* [Artifact attestations](https://docs.github.com/en/actions/concepts/security/artifact-attestations)
* [REST: repository attestations](https://docs.github.com/en/rest/repos/attestations)
* [REST: breaking changes, 2026-03-10](https://docs.github.com/en/rest/about-the-rest-api/breaking-changes)
* [Verifying attestations offline](https://docs.github.com/en/actions/how-tos/secure-your-work/use-artifact-attestations/verify-attestations-offline)
* [Immutable releases GA, 2025-10-28](https://github.blog/changelog/2025-10-28-immutable-releases-are-now-generally-available/)
* [Immutable releases](https://docs.github.com/en/code-security/supply-chain-security/understanding-your-software-supply-chain/immutable-releases)
* [OIDC with reusable workflows](https://docs.github.com/en/actions/how-tos/secure-your-work/security-harden-deployments/oidc-with-reusable-workflows)
* [Concurrency queues, 2026-05-07](https://github.blog/changelog/2026-05-07-github-actions-concurrency-groups-now-allow-larger-queues/)
* [Workflow syntax: `jobs.<job_id>.concurrency`](https://docs.github.com/en/actions/reference/workflows-and-actions/workflow-syntax#jobsjob_idconcurrency)
* [Events that trigger workflows](https://docs.github.com/en/actions/reference/workflows-and-actions/events-that-trigger-workflows)

**`gh`:**

* [`gh attestation verify`](https://cli.github.com/manual/gh_attestation_verify)
* [`gh release verify-asset`](https://cli.github.com/manual/gh_release_verify-asset)
* [policy.go](https://raw.githubusercontent.com/cli/cli/trunk/pkg/cmd/attestation/verify/policy.go)
* [sigstore.go](https://raw.githubusercontent.com/cli/cli/trunk/pkg/cmd/attestation/verification/sigstore.go)
* [tuf.go](https://raw.githubusercontent.com/cli/cli/trunk/pkg/cmd/attestation/verification/tuf.go)
* [cli/cli#10185](https://github.com/cli/cli/pull/10185)

**Sigstore:**

* [sigstore-go](https://github.com/sigstore/sigstore-go)
* [Fulcio OID info](https://github.com/sigstore/fulcio/blob/main/docs/oid-info.md)
* [Client specification](https://github.com/sigstore/architecture-docs/blob/main/client-spec.md)
* [Rekor v2 GA](https://blog.sigstore.dev/rekor-v2-ga/)
* [actions/attest](https://github.com/actions/attest)
* [actions/attest-build-provenance](https://github.com/actions/attest-build-provenance)

**Prior art:**

* [Homebrew attestation.rb](https://raw.githubusercontent.com/Homebrew/brew/main/Library/Homebrew/attestation.rb)
* [Homebrew 4.6.0](https://brew.sh/2025/08/05/homebrew-4.6.0/)
* [aqua#3157](https://github.com/aquaproj/aqua/issues/3157)
* [aqua#5247](https://github.com/aquaproj/aqua/pull/5247)
* [mise v2026.2.13](https://newreleases.io/project/github/jdx/mise/release/v2026.2.13)
* [mise discussion 7577](https://github.com/jdx/mise/discussions/7577)
* [mise#12766](https://github.com/jdx/mise/pull/12766)
* [mise#13875](https://github.com/jdx/mise/pull/13875)
* [The fail-open report](https://github.com/technicalpickles/dotfiles/pull/47)

**Zip:**

* [You et al., USENIX Security 2025](https://www.usenix.org/conference/usenixsecurity25/presentation/you)
* [uv CVE-2025-54368](https://astral.sh/blog/uv-security-advisory-cve-2025-54368)
* [PyPI, 2025-08-07](https://blog.pypi.org/posts/2025-08-07-wheel-archive-confusion-attacks/)
* [golang/go df57592](https://github.com/golang/go/commit/df57592276bc26e2eb4e4ca5e77e4e2e422c7c6b)
* [archive/zip reader.go at go1.27.1](https://github.com/golang/go/blob/go1.27.1/src/archive/zip/reader.go)

**Windows names:**

* [Naming a file](https://learn.microsoft.com/en-us/windows/win32/fileio/naming-a-file)
* [File streams](https://learn.microsoft.com/en-us/windows/win32/fileio/file-streams)
* [libarchive archive_write_disk_windows.c](https://raw.githubusercontent.com/libarchive/libarchive/master/libarchive/archive_write_disk_windows.c)
* [.NET Archiving.Utils.Windows.cs](https://raw.githubusercontent.com/dotnet/runtime/main/src/libraries/Common/src/System/IO/Archiving.Utils.Windows.cs)

**Crash recovery:**

* [Mender state scripts](https://docs.mender.io/artifacts/state-scripts)
* [Android A/B](https://source.android.com/docs/core/ota/ab)
* [systemd automatic boot assessment](https://systemd.io/AUTOMATIC_BOOT_ASSESSMENT/)
* [RAUC integration](https://rauc.readthedocs.io/en/latest/integration.html)
* [Chromium install_worker.cc](https://chromium.googlesource.com/chromium/src/+/main/chrome/installer/setup/install_worker.cc)

**actionlint:**

* [rhysd/actionlint#654](https://github.com/rhysd/actionlint/pull/654)

### Not verified

* **Whether `gh`'s `--source-ref` and `--signer-digest` behave as
  documented** for this module's reusable-workflow case. The PLAN probes
  them on a real release before the code is written, and names the
  minimum `gh` version. *Answered by amendment A1, correction 3.*
* **Whether a release attestation exists for private repositories, and
  on GitHub Enterprise Server.** It is not needed under the chosen
  outcome.
* **Item 5's three-tag cancellation** is read from GitHub's
  documentation. It was not run.
