---
status: accepted
date: 2026-10-05
decision-makers: go-selfupdate-lib maintainers (the owner)
consulted: Go 1.27.1 source and release notes, the Go vulnerability database, Apple's code-signing documentation and technotes, codesign(1) on macOS 26.6.2, the source of creativeprojects/go-selfupdate, rhysd/go-github-selfupdate, minio/selfupdate, fynelabs/selfupdate, Tailscale's clientupdate, GoReleaser, blacktop/go-macho, anchore/quill and apple-codesign; magic-cli-remote's updater and records
informed: magic-cli-remote, mcp-server-magictools, mcp-server-recall, mcp-server-socratic-thinker, mcp-server-duckduckgo, prepare-commit-msg
---
# Build `selfupdate/archive` and `selfupdate/codesign`: extract the program from a verified tar.gz or zip asset in a new stage before the transform, and re-sign or check it on macOS with `/usr/bin/codesign`

## Context and Problem Statement

[0004-MADR-evolve-selfupdate-api-and-tui-support.md](0004-MADR-evolve-selfupdate-api-and-tui-support.md)
lists both packages in its §1 target-shape table as "covered here":

* `selfupdate/archive`: "tar.gz and zip selector plus extract
  transformer", standard library only;
* `selfupdate/codesign`: "darwin re-sign transformer, moved from
  magic-cli-remote", standard library (`os/exec`).

Its §7 schedules `codesign` in Phase 4. Its amendment P2 (2026-10-03)
schedules `archive` there too, and says each "needs a PLAN before work
starts" and no record of its own. This record exists because research for
that PLAN found four things the 0004 table does not settle:

1. **An archive does not fit the pipeline as an extract transformer.**
   The pipeline has one transformer slot, and its Verifiers run before it,
   on the downloaded bytes.
2. **The re-sign transformer was rebuilt badly.** It lost its stable
   signing identifier when magic-cli-remote rebuilt its updater on mcplib's
   pipeline, and it looks `codesign` up on `PATH`.
3. **No fleet program publishes archives,** and this repository's publish
   workflow cannot publish one as a self-update asset.
4. **Verifying a macOS signature is not the same as re-signing one.**
   0004 names only re-signing.

This record decides how both packages are built. It does not reopen
whether they are built: P2 decided that. Section 9 asks the owner the one
scope question the research raises.

### What the pipeline does today

`Updater.apply` (`selfupdate/updater.go`) runs these steps in order:

1. The selector picks the binary and the `SHA256SUMS` asset. The run
   checks the binary's advertised size against `Limits.Executable` and the
   manifest's against `Limits.Manifest` (`checker.go:176-187`).
2. It downloads `SHA256SUMS` within `Limits.Manifest`, parses it, and
   finds the entry for `Selection.ManifestName` (`updater.go:204-217`).
3. `ManifestVerifier`s run on the manifest, before any binary byte is
   fetched (`:223`).
4. `InstallSession.CreateStaging` makes the staging file, and the binary
   streams into it within `Limits.Executable`, hashed as it arrives
   (`:229-239`). Staging is `.<base>.selfupdate-<random>` (plus `.exe` on
   Windows), mode 0600, in the target's directory (`session.go:67-92`).
5. `verifyIntegrity` compares the staged digest with the `SHA256SUMS`
   entry and, when GitHub supplies one, the asset's digest (`:256-261`).
6. `Config.Verifiers` run on the staged bytes; an error is joined with
   `ErrIntegrity` (`:262`). `NewImageVerifier` is one: it parses the bytes
   as ELF, Mach-O or PE for the platform (`imageverify.go:31-37`).
7. `EventVerified`, then, when a `Transformer` is set:
   * `EventTransforming`;
   * `Transform(ctx, TransformRequest{Product, Platform, Path, ReleaseDigest})`;
   * `hashAndValidateStaging`: a regular file, not a symlink, within
     `Limits.Executable`, and owned by the session through `StagingOwner`,
     failing closed; then rehash (`:274-287`, `:537-563`).
8. Probes run on the staged file, after `chmod 0700` (`:288`).
9. A dry run stops here and removes staging (`:291-302`); otherwise
   `EventInstalling` and `Install` (`:303-313`).

The installer replaces the target by renaming the staging file over it
(`replace_unix.go:43-45, 67-69`), so the running binary's inode is never
written. The backup is a hard link to the old inode. A crash leaves names
that `leftovers.go:22-36` sweeps: `.<base>.selfupdate-<digits>[.exe]` and
`-bak-<digits>`.

The seams this record touches, as declared in `selfupdate/types.go`:

* `Selection{Binary Asset; Manifest Asset; ManifestName string}`
  (`:238-246`): `Binary` is "the exact platform executable asset".
* `Verification` (`:298-322`): `SHA256` is "the staged content digest";
  `Open` reads the staged bytes.
* `TransformRequest` and `Transformer` (`:329-346`). `Config.Transformer`
  holds one `Transformer` (`:715-716`). There is no way to chain them; the
  module's only combinators are `ChainCredentials` and `MultiReporter`.
* `Limits` (`:675-702`): `ReleaseJSON` 2 MiB, `ErrorBody` 64 KiB,
  `Manifest` 1 MiB, `Executable` 512 MiB by default. `Executable` bounds
  the downloaded asset and the transformed file alike.
* `InstallSession.CreateStaging` may be called more than once; the
  built-in session registers every path it creates (`session.go:83-87`),
  and `Close` removes them all.
* `EventKind` is a `uint8`, and each release appends its new kinds in a
  new block, so every earlier value keeps its number (`:536-592`).

`NewExactAssetSelector` matches exactly `<product>-<os>-<arch>` (plus
`.exe` on Windows) and `SHA256SUMS` (`assets.go:8, 55-119`). Only that
selector requires the manifest's name; the coordinator accepts any
`Selection.Manifest` that parses as `SHA256SUMS` lines.

The release workflow, `publish-selfupdate-release.yml`, validates the
staged files with `scripts/verify-selfupdate-release.sh`:

* the staged set must equal the canonical binaries, `SHA256SUMS` and the
  declared extras (`:146-164`);
* "SHA256SUMS must contain exactly the canonical binaries" (`:172-173`);
* extras pass the name check `^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`
  (`:82, 129`), so an archive can be published, but only as an extra that
  `SHA256SUMS` may not list.

The workflow does not generate `SHA256SUMS`; the caller stages it. The
build-and-stage workflow that would generate it is a separate Phase 4 item
that 0004 §7 says "changes the workflow contract, so it needs a record".

Dependencies follow
[0008-MADR-enforce-import-rules-with-depguard.md](0008-MADR-enforce-import-rules-with-depguard.md).
Each package gets its own `depguard` rule naming exactly its imports, and
is excluded from `other-packages`. A rule's name must sort after `banned`
(amendment D1). `archive` sorts before `banned`, so the rule needs another
name.

The service packages of
[0011-MADR-reference-service-lifecycles.md](0011-MADR-reference-service-lifecycles.md)
set the pattern for running a platform tool:

* `service.Runner` runs a `service.Command` whose `Path` "must be
  absolute: a runner never looks a tool up on PATH" (`service/runner.go:13-14`);
  `ExecRunner` rejects a relative path (`:64-66`).
* Each package compiles on every OS and returns `service.ErrUnsupported`
  from `New` on the wrong one (0011-MADR §1).
* Tests drive a fake `Runner`; live tests run the real tool behind a
  `SELFUPDATE_REQUIRE_*` variable.

The launchd live tests already run `/usr/bin/codesign --force --sign -
--identifier <marker>` to tell builds apart (`launchd/live_darwin_test.go:188-210`).
They pass on CI's macos-15 runner.

### What the fleet does today

A read-only survey of the fleet's repositories (2026-10-05) found:

* **No program publishes archives.** Each publishes bare
  `<product>-<os>-<arch>[.exe]` binaries and `SHA256SUMS`. No repository
  imports `archive/tar`, `archive/zip` or `compress/gzip`, and none has a
  GoReleaser configuration. No installer extracts anything.
* **`selfupdate/archive` would be new capability, not a consolidation.**
  mcplib's
  `0005-MADR-canonicalize-cli-self-update-in-mcplib.md` rejected a peer
  library partly because "these repositories publish raw binaries from one
  source and do not need archive, multi-forge, ARM fallback, or naming-rule
  breadth".
* **No release signs with a Developer ID.**
  * Every darwin binary carries only what the Go linker gives it.
  * ocp-login-macos has a manual Makefile target that signs with a
    Developer ID and submits for notarization; no CI job runs it.
  * ocp-login and ocp-login-macos assert the ad-hoc signature in tests.
* **magic-cli-remote re-signs,** through `codesignTransformer` in
  `internal/updateclient/codesign_darwin.go` (75 lines; a 14-line
  non-darwin stub returns nil; 95 lines of darwin-only tests).
  * The identity comes from `MC_CODESIGN_IDENTITY` (`client.go:203`).
  * `Transform` runs `codesign --force --sign <identity> <path>`, then
    `codesign --verify --strict <path>`, and refuses a non-darwin platform.
  * It runs the bare name `codesign`, a `PATH` lookup.
  * It exists so that "the installed image keeps its TCC grants across an
    update" (its records 0060, 0069 and 0065).
  * magic-cli-remote requires `mcplib v1.4.1`, not this module.
* **Adopting mcplib's pipeline dropped the stable identifier.**
  * The code before that commit (`45f82032`), at
    `45f82032^:internal/update/swap.go:58-68`,
    signed with `-i com.magiccliremote.<product>`, as the Makefile still
    does (`Makefile:207-208`).
  * The transformer does not.
  * The probe below shows the result: an identifier taken from the staging
    file's name, `.<product>`. The first update after a Makefile install
    therefore changes the identifier. A certificate signature's designated
    requirement names the identifier, so grants tied to the old one would
    not carry over, which magic-cli-remote's 0069 D6 meant to prevent. That
    the grants are lost is inferred, not observed (see "Not verified").

### What the platforms and peers require

**Archives, in Go 1.27.1:**

* `archive/tar` and `archive/zip` reject unsafe names only under
  `GODEBUG=tarinsecurepath=0` / `zipinsecurepath=0`.
  * Both default to `1` (accept) in 1.27.1, unchanged since Go 1.20 added
    them; tracking issue #55356 is open.
  * tar checks names, never link targets.
  * zip also treats a backslash as unsafe.
  * Neither check is on by default, so a package must make its own.
* Nothing in either reader stops:
  * symlink or hard-link entries;
  * duplicate names when iterating `Reader.File`;
  * overlapping zip entries, the basis of the non-recursive zip bomb
    (Fifield, WOOT 2019);
  * a declared size the attacker chose.

  zip does enforce the declared uncompressed size and the CRC. The gzip
  trailer's size is modulo 2^32, so it is no bound.
* Recent CVEs in these readers, all fixed in 1.27.1:
  * CVE-2022-2879: tar header memory;
  * CVE-2025-58183 and CVE-2026-32288: tar sparse maps;
  * CVE-2025-61728: zip name indexing;
  * CVE-2024-24789: zip parser differential.
* The standard library decompresses gzip, flate, zlib, bzip2 and lzw. It
  has no xz and no public zstd: `internal/zstd` serves `debug/elf` only,
  and proposal #62513 is accepted but not built.
* `os.Root` (Go 1.24, extended in 1.25) confines file operations to a
  directory.

**Peer libraries:**

* **creativeprojects/go-selfupdate** (v1.6.0):
  * extracts `.zip`, `.tar.gz`, `.tgz`, `.gz`, `.tar.xz`, `.xz` and
    `.bz2`, with the third-party `ulikunitz/xz`;
  * picks the first entry whose base name matches a pattern, without
    checking the entry type;
  * sets no size limits.
* **rhysd/go-github-selfupdate** does much the same, and is unmaintained
  since 2021.
* **minio/selfupdate** and **fynelabs/selfupdate** have no archive support.
* **Tailscale's clientupdate:**
  * on Linux, selects tarball entries by base name, fails on "missing or
    duplicate files", and renames only after extracting all of them;
  * on macOS, it does not replace binaries at all.
* **GoReleaser** (v2.18.2) archives:
  * format: `tar.gz` by default;
  * name: `{{ .ProjectName }}_{{ .Version }}_{{ .Os }}_{{ .Arch }}`, with
    suffixes for ARM, MIPS and amd64 levels other than v1;
  * contents: license, readme and changelog files beside the binary;
  * checksums: in `{{ .ProjectName }}_{{ .Version }}_checksums.txt`.

  `goreleaser init` writes a different, uname-style template.
* None of these libraries checks or re-signs a macOS signature after an
  update.

**macOS code signing:**

* **On Apple silicon, every executable must be signed; an ad-hoc
  signature is enough** (Big Sur 11.0.1 release notes).
  * The Go linker ad-hoc signs `darwin/arm64` output, and only that
    (`NeedCodeSign`, since Go issue 42684).
  * A `darwin/amd64` build is not signed at all.
* **`codesign --verify --strict -R=<requirement>`:**
  * exits 0 when the code is valid and meets the requirement;
  * exits 1 when it is unsigned or invalid;
  * exits 2 on bad arguments;
  * exits 3 when it is "properly signed" but fails `-R`.

  A literal requirement takes a leading `=`. Apple's TN3127 gives a
  Developer ID designated requirement and advises starting from one dumped
  from signed code rather than writing one by hand.
* **Apple says not to parse signatures yourself.** TN3126: the structure
  "may well change again… To get information or validate a code
  signature, use the `codesign` tool or the Code Signing Services API".
  * blacktop/go-macho parses signatures but does not verify them.
  * anchore/quill verifies only the CMS signature, and depends on the AWS
    SDK and Charm, which this module bans.
  * apple-codesign warns that its own verification "will vary from what
    Apple tools do".
* **`/usr/bin/codesign` is on the sealed system volume,** not a Command
  Line Tools shim.
  * `/usr/bin/codesign_allocate`, which signing uses to make room for a
    signature, is the `xcode-select` shim: the same inode as
    `/usr/bin/git` on the development Mac.
* **"Updating Mac Software":** the kernel caches signature information
  and does not flush it when a file changes, so an update must write a
  new file and rename it over the old one. This module already does.
* **Quarantine:**
  * Downloads by a Go `net/http` client get no `com.apple.quarantine`;
    quarantine is opt-in per app (`LSFileQuarantineEnabled`).
  * `spctl --assess --type execute` rejects every bare command-line tool,
    including Apple's own.
  * Notarization needs a Developer ID, the hardened runtime and a secure
    timestamp; a ticket cannot be stapled to a bare Mach-O.

## Decision Drivers

* **Integrity is unchanged.** `SHA256SUMS`, the GitHub digest, the
  `ManifestVerifier`s and the `Verifier`s keep checking exactly the bytes
  the release published. Extraction is a deterministic function of bytes
  already verified.
* **Fail closed on ambiguity.** If two readers of one archive could
  disagree about which file is the program, the updater refuses it. A
  reviewer who unpacks a release with `tar` or `unzip` must see the binary
  the updater installs.
* **Bounded work.** Every byte read and every entry counted has a limit,
  derived from `Limits`, never from the archive's own headers.
* **No new modules.** The standard library and the module's existing
  requirements only (AGENTS.md; 0008-MADR).
* **Additive and v1-compatible.** `make apicheck` against `v1.7.0` reports
  only compatible changes. Every type whose values are compared today
  stays comparable.
* **One way to run a tool.** `codesign` runs through `service.Runner`, by
  absolute path, with an environment it builds, as the launchd backend
  runs `launchctl`.
* **Apple's tool is the authority on Apple signatures** (TN3126).
* **Every package compiles on every OS,** and on the wrong OS its
  constructors return `service.ErrUnsupported` (0011-MADR §1).
* **The owner's standing preference:** build coherent, extensible API now,
  even without a consumer yet, as long as it adds no dependency and weakens
  no default.

## Considered Options

* A. A core extract stage (`Config.Unpacker`), `selfupdate/archive` on the
  standard library, and `selfupdate/codesign` running `/usr/bin/codesign`
  through `service.Runner`.
* B. Extract as a `Transformer`, with a new `ChainTransformers` to put
  `codesign` after it.
* C. Verify signatures in Go instead of running `codesign`.
* D. Move magic-cli-remote's transformer unchanged.
* E. Build `codesign` only, and withdraw `archive` from Phase 4.

## Decision Outcome

Chosen option: "A. A core extract stage, `selfupdate/archive` on the
standard library, and `selfupdate/codesign` running `/usr/bin/codesign`
through `service.Runner`", because:

* it keeps every integrity check on the published bytes;
* it gives the extracted program the same staging ownership, limits and
  crash cleanup as a downloaded one;
* it leaves the one `Transformer` slot for `codesign`;
* it lets Apple's own tool judge signatures.

Section 9 asks whether to build the archive packages now.

The release that ships them is `v1.8.0`. The plan for it is a separate
`0012-PLAN-*` record, not written yet.

### 1. Packages

| Package | Contents | Imports | depguard rule |
| --- | --- | --- | --- |
| `selfupdate` (additions) | `Unpacker`, `UnpackerFunc`, `UnpackRequest`, `Config.Unpacker`, `Selection.Packed`, `EventUnpacking`, `CheckImage`, `ChainTransformers` | unchanged | `selfupdate`, unchanged |
| `selfupdate/archive` | `NewSelector`, `SelectorOptions`, `Format`, the naming helpers, `NewUnpacker`, `UnpackOptions` | standard library; `…/selfupdate$` | `selfupdate-archive` |
| `selfupdate/codesign` | `NewSigner`, `SignOptions`, `NewChecker`, `CheckOptions` | standard library; `…/selfupdate$`; `…/selfupdate/service$` | `selfupdate-codesign` |

* **Rule names:** both rules sort after `banned`, unlike a rule named
  `archive`. Each allows exactly the imports above and is excluded from
  `other-packages`.
* **`codesign` imports `selfupdate/service`** for `Runner`, `Command`,
  `Output`, `ExecRunner` and `ErrUnsupported`. A second, private runner
  would duplicate the contract and its fakes.
  * `service` adds no requirement: its only extra import,
    `golang.org/x/sys/windows`, is already required, and only its Windows
    files use it.
* **AGENTS.md** gains both packages in its package list and its depguard
  list.

### 2. The extract stage in `selfupdate`

```go
// UnpackRequest is the input to an Unpacker.
type UnpackRequest struct {
    Product   string   // the requested product name
    Platform  Platform // the selected platform
    AssetName string   // the verified asset's name, such as "relay-linux-amd64.tar.gz"
    Archive   string   // the verified asset's staging path; read it, never write it
    Program   string   // a second, empty staging path; write the program here
    Limit     int64    // the most bytes the program and the whole extraction may take
}

// An Unpacker writes the program inside a verified release asset to
// Program. The coordinator then checks Program as it checks a
// transformed file.
type Unpacker interface {
    Unpack(context.Context, UnpackRequest) error
}
```

* **`Selection.Packed bool`:** "the binary asset is an archive, and an
  `Unpacker` extracts the program from it". `archive.NewSelector` sets it;
  `NewExactAssetSelector` never does. `Selection` stays comparable.
* **Matching selector and unpacker.** Right after selection, before any
  download, the run fails when:
  * `Selection.Packed` is set and `Config.Unpacker` is nil;
  * `Config.Unpacker` is set and `Selection.Packed` is not.

  Without this, an archive would be installed as the program, or a bare
  binary handed to an extractor.
* **`New` refuses known-bad combinations** before any run:
  * an `Unpacker` with `NewExactAssetSelector`;
  * an `Unpacker` with a `NewImageVerifier` among the `Verifiers`. That
    verifier parses the release asset, which is now an archive. The archive
    unpacker checks the program's image instead (section 4).

  Both checks recognise the module's own unexported types; a consumer's
  own selector or verifier is the consumer's to match.
* **Where it runs:** after `EventVerified` and before the transform. The
  run:
  1. emits `EventUnpacking`, whose `Asset` is the archive's name;
  2. calls `CreateStaging` a second time;
  3. closes that file and passes its path as `Program`, with `Limit` set to
     `Limits.Executable`;
  4. on success, applies `hashAndValidateStaging`'s checks to `Program`:
     a regular file, not a symlink, within `Limits.Executable`, owned by
     the session through `StagingOwner`, failing closed as for a
     transform;
  5. uses `Program` as the staging path from then on: the transform, the
     probes and the install see the program.
* **Both staging files are the session's:**
  * `Close` removes the archive's;
  * the install renames the program's;
  * after a crash, both match the sweep pattern.

  A custom session that does not implement `StagingOwner` cannot unpack,
  as it cannot transform (0004-MADR G7).
* **What the checks see:** `SHA256SUMS`, the GitHub digest and the
  `Verifier`s keep checking the asset. `Verification` keeps its meaning:
  the staged bytes are the published asset.
* **Digests:**
  * `Result.ReleaseDigest` and `TransformRequest.ReleaseDigest` are the
    archive's digest, "the verified SHA-256 hex of the release bytes";
  * `Result.InstalledDigest` is the program's, after the unpack and any
    transform.

  Their doc comments say so.
* **A dry run unpacks,** as it transforms, so `--dry-run` proves the
  archive.
* **`EventUnpacking`** is appended in a block for kinds added in v1.8.0,
  after `EventWarning`. A reporter sees it only from a run with an
  `Unpacker`. `EventKind.String` names it `unpacking`, as it names
  `EventTransforming` `transforming` (`types.go:611`). That name is the
  event's `kind` in the JSON reporter (`jsonreporter.go:46`), and so in
  `cli`'s `--json` stream.
* **`CheckImage(r io.ReaderAt, p Platform) error`** exports the check
  behind `NewImageVerifier`: ELF, Mach-O (thin or fat) or PE, for the
  platform's architecture. It fails with `ErrUnsupportedPlatform` on a
  platform that check does not cover. `NewImageVerifier` keeps its
  behaviour.

### 3. `selfupdate/archive`: selection

```go
type Format string

const (
    TarGz Format = "tar.gz" // also matched as .tgz
    Zip   Format = "zip"
    Gz    Format = "gz"     // one gzip-compressed binary, GoReleaser's "gz"
)

type SelectorOptions struct {
    Platforms []selfupdate.Platform
    // Format picks the format for a platform. Nil means Zip on windows,
    // TarGz elsewhere.
    Format func(selfupdate.Platform) Format
    // Name builds the asset name. Nil means FleetName.
    Name func(product, tag string, p selfupdate.Platform, f Format) (string, error)
    // Manifest names the checksum asset. Nil means "SHA256SUMS".
    Manifest func(product, tag string) (string, error)
}

func NewSelector(o SelectorOptions) (selfupdate.AssetSelector, error)
```

* **`Platforms`:** validated as `NewExactAssetSelector` validates them:
  unique, lowercase, and matching `^[a-z0-9][a-z0-9_]*$`.
* **`Select`** requires exactly one asset named by `Name` and one named by
  `Manifest`. A duplicate, or a platform outside the list, fails as it
  does for the exact selector. It returns:
  * `Binary`: the archive;
  * `Manifest`: the checksum asset;
  * `ManifestName`: the archive's name;
  * `Packed`: true.
* **Name validation:** a name from `Name` or `Manifest` must be a
  basename that matches the release workflow's name check,
  `^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`. A name that fails is an error, not
  a miss.
* **`FleetName`** is this module's convention with the format's
  extension: `<product>-<os>-<arch>.tar.gz`, `.zip` or `.gz`.
* **`GoReleaserName`** is GoReleaser's default template,
  `<project>_<version>_<os>_<arch>.<ext>`:
  * `<version>` is the tag without its leading `v`;
  * `<project>` is the product;
  * it refuses `arm`, `mips*` and any other architecture whose default
    name needs a variant `Platform` does not carry.
* **`GoReleaserChecksums`** names `<project>_<version>_checksums.txt`.
* **Other schemes,** such as `goreleaser init`'s uname-style names, are a
  `Name` function the consumer writes.

### 4. `selfupdate/archive`: extraction

```go
type UnpackOptions struct {
    // Member is the program's path inside the archive. Empty means the one
    // regular file whose base name is the product (plus ".exe" on windows),
    // at the top level or one directory down.
    Member string
    // MaxEntries bounds the entries read. Zero means 4096.
    MaxEntries int
}

func NewUnpacker(o UnpackOptions) (selfupdate.Unpacker, error)
```

* **Format:** taken from the asset name's suffix: `.tar.gz` or `.tgz`,
  `.zip`, `.gz`.
  * The first bytes must match: gzip `1f 8b`, or zip `PK\x03\x04` (or
    `PK\x05\x06` for an empty archive, which then has no program).
  * A suffix the unpacker does not know, or a mismatch, is refused.
* **The archive is refused, wrapping `ErrIntegrity`, when any entry:**
  * has a name that is empty, absolute, contains `..`, a backslash or a
    NUL, or is not `filepath.IsLocal` on the host. This is enforced
    whatever `GODEBUG` says;
  * repeats a name, after `path.Clean`, or matches another ignoring
    case: on a case-insensitive file system, such as macOS's default,
    `tar` or `unzip` writes one over the other, so a reviewer could see
    a different program from the one the updater installs;
  * is a link, device, FIFO or sparse entry (tar `TypeLink`,
    `TypeSymlink`, `TypeChar`, `TypeBlock`, `TypeFifo`, `TypeGNUSparse`,
    or a PAX sparse header) anywhere in the archive. Directories, and
    regular files other than the program, are read past;
  * would take the archive past `MaxEntries`;
  * declares, or reads, more bytes than `Limit`, counted over every entry
    read, not just the program;
  * (zip) uses a compression method other than store and deflate;
  * (zip) overlaps another entry's data, judged by data offset and
    compressed size (Fifield's check).
* **The program:**
  * exactly one regular file named by `Member`, or, when `Member` is
    empty, one with the product's base name at depth 0 or 1. None or more
    than one is refused, wrapping `ErrIntegrity`;
  * for `.gz`, the whole decompressed stream, single member
    (`Multistream(false)`), with trailing data refused.
* **Writing:** the program streams into `Program` through a reader capped
  at `Limit`+1 bytes, so an overrun is seen rather than truncated.
  * A zip entry is read to EOF, so the standard library checks its size
    and CRC.
  * Nothing is written by an entry's name; there is no directory to
    escape.
  * The unpacker sets no mode or owner; the installer does, from the old
    target.
* **Image check:** after writing, `CheckImage` on the program for the
  request's platform. A mismatch is refused, wrapping `ErrIntegrity`. A
  platform `CheckImage` does not cover fails with
  `ErrUnsupportedPlatform`.
* **No parallelism, no temporary files,** and memory bounded by the
  standard library's header limits. zip needs `io.ReaderAt`, so it
  reads the archive's staging file.
* **The rest of the archive's bytes are not read for their checksums.**
  The archive is already verified against `SHA256SUMS`, so a gzip CRC
  adds nothing.

### 5. `selfupdate/codesign`: re-signing

```go
type SignOptions struct {
    // Identity is a signing identity codesign accepts: a certificate's
    // common name or SHA-1 hash, or "-" for an ad-hoc signature. Required.
    Identity string
    // Identifier is the signature's identifier, such as
    // "com.example.relay". Required: codesign's default comes from the
    // file name, which for a staging file is ".<product>".
    Identifier string
    // Keychain, when set, is an absolute path to search for Identity.
    Keychain string
    // Runtime adds --options runtime (the hardened runtime).
    Runtime bool
    // Timestamp asks Apple's timestamp server; false passes
    // --timestamp=none, so an update never needs that server.
    Timestamp bool
    // Requirement, when set, is checked after signing, in addition to
    // the identifier, as with codesign -R. No leading "=".
    Requirement string
    // Codesign is the tool's absolute path. Empty means /usr/bin/codesign.
    Codesign string
    // Runner runs the tool. Nil means service.ExecRunner.
    Runner service.Runner
}

func NewSigner(o SignOptions) (selfupdate.Transformer, error)
```

* **`NewSigner`:**
  * returns `service.ErrUnsupported` off macOS;
  * refuses an empty `Identity` or `Identifier`, a relative `Keychain` or
    `Codesign`, and an `Identifier` outside `[A-Za-z0-9.-]`, so it can
    never be read as a flag.
* **`Transform`** refuses a platform that is not darwin, then runs:

  ```text
  codesign --force --sign <Identity> --identifier <Identifier>
           [--keychain <Keychain>] [--options runtime]
           --timestamp=none | --timestamp
           <staging path>
  codesign --verify --strict -R=identifier "<Identifier>"[ and (<Requirement>)] <staging path>
  ```

  * The environment is built, never inherited:
    `PATH=/usr/bin:/bin:/usr/sbin:/sbin` and `LC_ALL=C`, as in the launchd
    backend.
  * A failed sign fails the transform, with the tool's output capped and
    on one line.
  * Verification exit codes: 1 means the signature it just made is
    invalid; 2 means a bad argument; 3 means it does not meet the
    requirement. Each fails the transform.
* **The verify step pins the identifier,** where magic-cli-remote's checks
  only that some signature is valid. A run therefore cannot install a
  binary whose identifier differs from the one `Identifier` names.
* **`codesign --force` writes a new file** (the probe below). The staging
  file is never executed before it is signed, so the kernel cache cannot
  hold a stale signature for it.
* **For magic-cli-remote, this restores the pre-move behaviour:**
  `Identity` from `MC_CODESIGN_IDENTITY`, `Identifier`
  `com.magiccliremote.<product>`. The move is that repository's own work.

### 6. `selfupdate/codesign`: checking

```go
type CheckOptions struct {
    // Requirement, when set, must be met, as with codesign -R: for
    // example a Developer ID requirement naming a team. Empty checks
    // only that the signature is valid and meets its own designated
    // requirement.
    Requirement string
    Codesign    string         // as SignOptions
    Runner      service.Runner // as SignOptions
}

func NewChecker(o CheckOptions) (selfupdate.Prober, error)
```

* **What it is:** a `Prober`, not a `Verifier`. A `Verifier` gets a
  reader, while `codesign` needs a path. Probes also run after the
  transform, so they see the bytes that will be installed.
* **When it runs:** at `ProbeStaged` it runs
  `codesign --verify --strict [-R=<Requirement>] <path>`. At
  `ProbeInstalled` it does nothing, because the installed file is the
  same inode.
* **Errors:**
  * exit 1 (unsigned or invalid) and exit 3 (requirement unmet) wrap
    `ErrIntegrity`;
  * exit 2 and a runner failure do not.
* **Off macOS** it returns `service.ErrUnsupported`.
* **It is opt-in** (§10). Its doc comment says that a binary with no
  signature fails it.
* **With a Developer ID requirement,** it lets a publisher who signs
  releases refuse an update signed by anyone else, independently of
  `SHA256SUMS`. With no requirement, it catches a darwin/arm64 binary whose
  signature is missing or broken before the swap, rather than after it,
  when the kernel kills it.

### 7. Order of the stages, and `ChainTransformers`

With every stage configured, a run goes:

1. select;
2. download `SHA256SUMS`;
3. `ManifestVerifier`s;
4. download the asset;
5. digest checks;
6. `Verifier`s, on the asset;
7. `EventVerified`;
8. `EventUnpacking` and `Unpacker`;
9. `EventTransforming` and `Transformer`, such as `codesign.NewSigner`;
10. `Prober`s, such as `codesign.NewChecker`;
11. dry run or install.

`ChainTransformers(ts ...Transformer) (Transformer, error)`:

* runs each transformer in order, on the same path, with the same request;
* stops at the first error;
* refuses an empty list, a nil and a typed nil;
* lets the coordinator rehash once, after the chain.

It lets a consumer put a transform of its own beside `NewSigner`. The
archive does not need it: the unpacker has its own stage.

### 8. What stays out

* **xz, zstd and bzip2 archives:**
  * xz and zstd need a module, so a record of their own (AGENTS.md);
  * zstd waits for a public `compress/zstd`;
  * bzip2 is in the standard library but has no demand; adding it later
    is additive.
* **Publishing archives through `publish-selfupdate-release.yml`.** Its
  rule that `SHA256SUMS` lists exactly the canonical binaries stays. A
  fleet product can adopt an archive release only when the build-and-stage
  workflow's record changes that contract. Until then, `selfupdate/archive`
  serves releases published by other tooling, such as GoReleaser, that are
  immutable GitHub releases with a SHA-256 checksum file. The updater
  requires immutability (`ErrMutableRelease`), and a GoReleaser release is
  immutable only when its repository turns immutable releases on.
* **Several programs from one archive,** and app bundles. A bundle's seal
  covers more than the executable, and replacing only the executable
  breaks it.
* **Notarization, `spctl` and quarantine handling.**
  * `spctl` rejects every bare tool.
  * A ticket cannot be stapled to one, so checking it needs the network:
    `--check-notarization`.
  * The updater's downloads are not quarantined.

  A consumer that wants the online check passes
  `Requirement: "notarized"` to `NewChecker`. Section 9 does not adopt
  that as a default.
* **Pure-Go signature verification** (option C).

### 9. Question for the owner

**Q1. Build `selfupdate/archive` now, with no fleet publisher?**

*Recommended: yes, as P2 decided, under the standing preference for
coherent API with no consumer yet.*

* It adds no module and changes no default.
* It covers the release shape most Go projects publish, GoReleaser's.
* The core stage is the part that is hard to add later without breaking
  ordering: `Selection.Packed`, `Unpacker` and `EventUnpacking`.

The alternative is option E: build `codesign` and the core
`ChainTransformers` only, and amend 0004 to withdraw `archive` until a
publisher wants it.

### 10. Owner answers (2026-10-05)

* **Q1: "build it."** `selfupdate/archive` and the core stage are built
  in `v1.8.0`. Option E is not taken.
* **On `codesign`:** "i do not need anything that will block the runtime
  on macos since for now i am not signing my binaries. this should be
  forward, future-proofing functionality for when I have an apple ID."
  So:
  * **Nothing runs `codesign` unless a program configures it.**
    `NewSigner` and `NewChecker` are reached only through `Config`; no
    default, no `cli` flag, no helper and no other package of this module
    builds either.
  * **Nothing in a default run inspects a macOS signature.** The core
    stage acts only when a selector marks an archive. The archive
    unpacker's image check reads the Mach-O header, as `NewImageVerifier`
    does, never the signature. An unsigned or linker-signed binary updates
    exactly as in `v1.7.0`.
  * **The package is built and tested now, for later.** Ad-hoc signing
    and requirement checks are proven live in CI. Signing with a
    certificate identity has a live test that runs only when the owner
    names an identity on a Mac that has one, and is skipped in CI.
  * **The identity is the owner's later choice.** `SignOptions.Identity`
    takes whatever `codesign --sign` accepts. Which certificate an Apple
    account provides is not decided or verified here.
  * The docs present the package as opt-in, for a publisher who signs.
* **On darwin/amd64:** "i do not want, nor need to support amd64 darwin."
  So:
  * darwin/arm64 is the only macOS target this work builds, tests,
    documents or checks;
  * no fixture, live test, guide example or release check uses
    darwin/amd64;
  * the code singles out no architecture. `archive`, `codesign` and
    `CheckImage` neither add darwin/amd64 handling nor refuse it, so a
    consumer's own platform list decides, as it does for every other
    pair;
  * the facts about darwin/amd64 above, that Go leaves it unsigned and how
    `codesign` reports that, stay as research context. No decision rests
    on them.

### Consequences

* Good, because the archive packages add no dependency, change no default
  and give the updater the archive shape GoReleaser publishes.
* Good, because `SHA256SUMS`, digests and `Verifier`s keep one meaning,
  the published bytes, whatever the asset's shape.
* Good, because the extracted program inherits staging ownership, the
  executable limit, crash cleanup and the image check.
* Good, because an archive that two tools could read differently is
  refused: duplicate names, overlapping entries, link entries and unsafe
  names.
* Good, because re-signing pins the identifier and runs the tool by
  absolute path. That fixes the two defects in the transformer it
  replaces.
* Good, because a publisher who signs with a Developer ID can make the
  updater refuse anything else, with Apple's own tool as judge.
* Neutral, because a matching selector and unpacker must both be
  configured. The run refuses a mismatch before downloading.
* Neutral, because `selfupdate/codesign` depends on `selfupdate/service`
  for the runner contract.
* Bad, because no fleet program can publish an archive through this
  repository's workflow until the build-and-stage record changes its
  contract.
* Bad, because an archive costs a second staging file of up to
  `Limits.Executable` bytes until `Close`.
* Bad, because re-signing on a Mac without Xcode or the Command Line
  Tools is expected to fail: `codesign_allocate` is a shim there.
  Verification does not need it. This is not verified; see "Not verified".
* Bad, because the Developer ID paths cannot run in CI, which holds no
  identity. Only ad-hoc signing and requirement checks are tested live.

### Confirmation

* **`make apicheck`:** "compatible with v1.7.0"; `go.mod` unchanged;
  `go mod tidy -diff` clean.
* **depguard:**
  * the two new rules are in place, named after `banned`, and excluded
    from `other-packages`;
  * a planted import outside each allow list fails `make lint`.
* **The core stage:**
  * event-order tests with and without an `Unpacker`;
  * a selector and unpacker mismatch is refused before any download;
  * `New` refuses an `Unpacker` with `NewExactAssetSelector`, and with
    `NewImageVerifier`;
  * `Owns` fails closed;
  * a dry run unpacks;
  * the leftover sweep removes both staging names after a simulated
    crash;
  * the digests are as section 2 states.
* **End to end:** `selfupdatetest` releases in each format and both
  naming schemes run through the `Updater` and the `cli`, and the
  installed program carries the right bytes.
* **One refusal test for each rule in section 4,** each seen to fail on
  the unfixed code or a plant: unsafe names (absolute, `..`, backslash,
  NUL), duplicates, a symlink or hard link named as the program, devices
  and FIFOs, sparse entries, too many entries, the size limit by
  declaration and by reading, a gzip bomb, overlapping zip entries, an
  unknown method, a suffix and magic mismatch, a missing or duplicate
  program, a program two directories down, and a wrong-architecture image.
* **Fuzz targets** for tar.gz, zip and gz extraction. `make fuzz` today
  fuzzes `./selfupdate` only, so the PLAN extends it.
* **`codesign` against a fake `Runner`:**
  * the exact arguments;
  * the built environment;
  * each exit code's error;
  * the refusals in `NewSigner` and `NewChecker`;
  * `service.ErrUnsupported` off macOS.
* **`codesign` live,** on the macos-15 runner behind
  `SELFUPDATE_REQUIRE_CODESIGN=1`:
  * an ad-hoc `NewSigner` gives the configured identifier and a valid
    signature;
  * `NewChecker` accepts a linker-signed arm64 build, and refuses a copy
    with its signature removed (exit 1), a tampered one (exit 1) and an
    unmet requirement (exit 3).
* **A scratch consumer** after the tag, as for `v1.7.0`.

## Pros and Cons of the Options

### A. A core extract stage, `selfupdate/archive`, and `selfupdate/codesign` through `service.Runner`

* Good, because integrity checks keep checking the published asset, and
  nothing about `Verification` changes meaning.
* Good, because the program gets the session's ownership, limits and
  cleanup without a transformer making files of its own.
* Good, because the single `Transformer` stays free for `codesign`.
* Good, because `codesign` follows the module's tool rules: absolute path,
  built environment, fake-able runner.
* Neutral, because it adds a stage and an event kind to `selfupdate`.
* Bad, because it is the largest change to the core of the five.

### B. Extract as a `Transformer`

* Good, because it needs no new stage: `ChainTransformers` alone lets
  extraction and `codesign` share the slot.
* Bad, because `NewImageVerifier` and any digest-checking `Verifier`
  would see the archive, with no way to check the program's image.
* Bad, because the transformer would have to make its own scratch file in
  the target directory:
  * the session would not own it;
  * `Close` would not remove it;
  * the crash sweep would miss it unless its name copied the session's
    pattern.

  Writing the program over the archive in place needs the whole archive
  in memory.
* Bad, because a selector that marks the asset as an archive could not
  tell the core that an extractor is required, so a misconfigured run
  would install an archive as the program.

### C. Verify signatures in Go

* Good, because it would run on any OS, so tests of signing policy would
  not need a Mac.
* Bad, because Apple says not to: the format changes, and the `codesign`
  tool is the supported interface (TN3126).
* Bad, because the standard library has no CMS/PKCS#7 verifier and no
  requirement-language evaluator. The libraries that have them either do
  not verify (go-macho), verify only part of it with banned dependencies
  (quill), or disclaim matching Apple (apple-codesign).
* Bad, because signing itself would still need `codesign`.

### D. Move magic-cli-remote's transformer unchanged

* Good, because it is a 75-line move with tests.
* Bad, because it keeps both defects: no `--identifier`, so a staging
  name decides the identifier, and a `PATH` lookup of `codesign`.
* Bad, because it offers no way to check a signature, which is the
  safety a Developer ID publisher would want.

### E. Build `codesign` only; withdraw `archive`

* Good, because it builds only what a fleet program uses, and leaves the
  core pipeline as it is.
* Good, because mcplib's 0005-MADR made this argument for raw binaries,
  and the fleet has not changed shape since.
* Bad, because it reverses P2 against the owner's standing preference.
* Bad, because adding the stage later re-opens the ordering of
  `Verifier`s, transforms and events after consumers depend on it.

## Amendments

### A1 (2026-10-07): §4's further refusals, and §5's requirement in its own run

*Status: accepted (2026-10-07). The findings and their fixes are in
[0015-MADR-remediate-third-debugging-pass-findings.md](0015-MADR-remediate-third-debugging-pass-findings.md) (E1–E4, E6); they were built in [0015-PLAN-remediate-third-debugging-pass-findings.md](0015-PLAN-remediate-third-debugging-pass-findings.md), Phase P6, and ship in
`v1.10.1`.*

* **§4, what another tool would read differently** (E1–E4). The unpacker
  also refuses:
  * a zip entry whose local header disagrees with its central-directory
    record (signature, method, name, lengths), and an Info-ZIP Unicode
    Path extra field (0x7075), which bsdtar and Info-ZIP use in place of
    the header's name;
  * a name with a byte outside printable ASCII, or an element ending in a
    dot or a space, which APFS, NTFS or Win32 would fold into another
    name; with both refused, the "differs only in case" rule is exact;
  * a zip directory attribute on a name without a trailing `/`, a zip
    directory entry that holds data, and a tar regular file whose name
    ends in `/`;
  * an encrypted zip entry (flags `0x2041`), in either header.
* **§5, the requirement** (E6): `SignOptions.Requirement` is checked in a
  `codesign --verify` run of its own, after the identifier's, never
  joined to it: a requirement such as `anchor apple) or (always` could
  otherwise cancel the identifier pin.

### A2 (2026-10-08): a PAX global header

*Status: accepted (2026-10-08). The finding is in [0015-MADR-remediate-third-debugging-pass-findings.md](0015-MADR-remediate-third-debugging-pass-findings.md) (E8); it was built
in [0015-PLAN-remediate-third-debugging-pass-findings.md](0015-PLAN-remediate-third-debugging-pass-findings.md), Phase Q4, and ships in `v1.11.0`.*

* **§4:** a PAX global header, such as `git archive` writes with the
  commit as a comment, is skipped, and counts toward the entry limit. One
  whose records set `path`, `linkpath`, `size` or a `GNU.sparse.` key is
  refused: it would change how another tool reads every entry.

## More Information

### Probe evidence

On the development Mac (macOS 26.6.2, Apple silicon, Go 1.27.1),
2026-10-05, in a scratch directory:

* **Go's linker signature:**
  * `CGO_ENABLED=0 go build` for darwin/arm64 gives `Identifier=a.out`,
    `flags=0x20002(adhoc,linker-signed)`, `Signature=adhoc` and
    `TeamIdentifier=not set`;
  * for darwin/amd64 it gives "code object is not signed at all", and
    `codesign --verify --strict` exits 1.
* **The identifier without `--identifier`:**
  `codesign --force --sign - .demo.selfupdate-a1b2c3` gives
  `Identifier=.demo`, and keeps it after a rename. Its designated
  requirement is `cdhash H"…"`, which changes with every build.
* **With `--identifier com.example.demo`:** `Identifier=com.example.demo`.
* **Inode:** `codesign --force --sign` replaced the file with a new one.
* **Requirement checks:**
  * `-R='identifier "com.example.demo"'` exits 0;
  * `-R='identifier "com.other"'` and `-R='anchor apple generic'` exit 3
    on the ad-hoc signature;
  * `-R=…` on the unsigned amd64 build exits 1.
  * `codesign --remove-signature` on a copy of the arm64 build succeeds,
    and `codesign --verify --strict` on that copy exits 1, "code object is
    not signed at all". This is how the live tests make an unsigned
    darwin/arm64 file (§10).
* **`/usr/bin/codesign_allocate`** has the same inode as `/usr/bin/git`
  (78 links): the `xcode-select` shim. `/usr/bin/codesign` is a separate
  binary.
* **`man codesign`:**
  * on `--identifier`: "If this option is omitted, the identifier is
    derived from either the Info.plist (if present), or the filename of
    the executable being signed";
  * on `--timestamp`: "If this option is not given at all, a
    system-specific default behavior is invoked… The special value none
    explicitly disables the use of timestamp services."
* **During the web research** (same host):
  * a signed binary overwritten in place by `cp` while it ran was killed
    (`Killed: 9`) on every later launch, and write-then-rename never was;
  * an arm64 binary with its signature removed, or one byte changed, was
    killed;
  * files downloaded by `curl` and by a Go `net/http` client carried no
    `com.apple.quarantine`;
  * `spctl --assess --type execute` rejected bare tools, Apple's included.

### Sources

Go:

* Go 1.20 release notes (`tarinsecurepath`, `zipinsecurepath`):
  <https://go.dev/doc/go1.20>; defaults at 1.27.1:
  <https://github.com/golang/go/blob/go1.27.1/doc/godebug.md>; issue
  #55356: <https://github.com/golang/go/issues/55356>
* `archive/tar` and `archive/zip` readers at go1.27.1:
  <https://github.com/golang/go/blob/go1.27.1/src/archive/tar/reader.go>,
  <https://github.com/golang/go/blob/go1.27.1/src/archive/zip/reader.go>;
  zip entry-count limit, issue #78367:
  <https://github.com/golang/go/issues/78367>
* `os.Root`: <https://go.dev/doc/go1.24>, <https://go.dev/doc/go1.25>
* `compress/` at go1.27.1:
  <https://github.com/golang/go/tree/go1.27.1/src/compress>; zstd
  proposal #62513: <https://github.com/golang/go/issues/62513>
* Vulnerabilities: <https://pkg.go.dev/vuln/GO-2022-1037> (CVE-2022-2879),
  <https://pkg.go.dev/vuln/GO-2025-4014> (CVE-2025-58183),
  <https://pkg.go.dev/vuln/GO-2026-4869> (CVE-2026-32288),
  <https://pkg.go.dev/vuln/GO-2026-4342> (CVE-2025-61728),
  <https://pkg.go.dev/vuln/GO-2024-2888> (CVE-2024-24789)
* The linker's ad-hoc signature: issue #42684,
  <https://github.com/golang/go/issues/42684>;
  <https://github.com/golang/go/blob/go1.27.1/src/cmd/link/internal/ld/lib.go>

Archives elsewhere:

* Fifield, "A better zip bomb" (WOOT 2019):
  <https://www.bamsoftware.com/hacks/zipbomb/>
* Zip Slip (secondary, vendor research):
  <https://security.snyk.io/research/zip-slip-vulnerability>
* creativeprojects/go-selfupdate:
  <https://github.com/creativeprojects/go-selfupdate/blob/main/decompress.go>
* rhysd/go-github-selfupdate:
  <https://github.com/rhysd/go-github-selfupdate/blob/master/selfupdate/uncompress.go>
* minio/selfupdate: <https://github.com/minio/selfupdate>;
  fynelabs/selfupdate: <https://github.com/fynelabs/selfupdate>
* Tailscale clientupdate:
  <https://github.com/tailscale/tailscale/blob/main/clientupdate/clientupdate.go>
* GoReleaser archives and checksums:
  <https://github.com/goreleaser/goreleaser/blob/main/internal/pipe/archive/archive.go>,
  <https://github.com/goreleaser/goreleaser/blob/main/www/content/customization/package/archives.md>,
  <https://github.com/goreleaser/goreleaser/blob/main/internal/pipe/checksums/checksums.go>

Apple:

* codesign(1), read on the development Mac; mirror (secondary):
  <https://keith.github.io/xcode-man-pages/codesign.1.html>
* Code Signing Requirement Language:
  <https://developer.apple.com/library/archive/documentation/Security/Conceptual/CodeSigningGuide/RequirementLang/RequirementLang.html>
* TN3127, Inside Code Signing: Requirements:
  <https://developer.apple.com/documentation/technotes/tn3127-inside-code-signing-requirements>
* TN3126, Inside Code Signing: Hashes:
  <https://developer.apple.com/documentation/technotes/tn3126-inside-code-signing-hashes>
* Updating Mac Software:
  <https://developer.apple.com/documentation/security/updating-mac-software>
* Big Sur 11.0.1 Universal Apps release notes:
  <https://developer.apple.com/documentation/macos-release-notes/macos-big-sur-11_0_1-universal-apps-release-notes>
* Notarizing macOS software before distribution:
  <https://developer.apple.com/documentation/security/notarizing-macos-software-before-distribution>;
  Customizing the notarization workflow:
  <https://developer.apple.com/documentation/security/customizing-the-notarization-workflow>
* `LSFileQuarantineEnabled`:
  <https://developer.apple.com/documentation/bundleresources/information-property-list/lsfilequarantineenabled>
* Hardened Runtime:
  <https://developer.apple.com/documentation/security/hardened-runtime>
* Apple Developer Forums, testing notarization (thread 130560) and a Go
  self-updater killed after update (thread 758098):
  <https://developer.apple.com/forums/thread/130560>,
  <https://developer.apple.com/forums/thread/758098>

Signature tooling considered for option C:

* blacktop/go-macho:
  <https://github.com/blacktop/go-macho/blob/master/pkg/codesign/codesign.go>
* anchore/quill:
  <https://github.com/anchore/quill/blob/main/quill/extract/signature.go>,
  <https://github.com/anchore/quill/blob/main/go.mod>
* apple-codesign:
  <https://github.com/indygreg/apple-platform-rs/blob/main/apple-codesign/src/verify.rs>

### Not verified

* **Re-signing on a Mac without Xcode or the Command Line Tools.** The
  development Mac has Xcode. That `codesign --sign` fails there, because
  `codesign_allocate` is a shim, is inferred, not observed.
* **Signing with a certificate identity** (Apple Development or
  Developer ID), and whether TCC grants then survive an update. magic-cli-remote's
  0069-MADR D6 asserts it; no identity was available here.
* **GoReleaser's `checksums.txt`** is expected to parse with
  `ParseSHA256SUMS`. The PLAN tests a real one.
* **`goreleaser init`'s uname-style template.** Its exact text was not
  read, which is why it is not built in.
* **`com.apple.provenance`,** which macOS put on the downloads in the
  quarantine probe. Apple does not document it, and nothing here depends
  on it.
* **Whether the hardened runtime (`Runtime: true`) changes anything** for
  a Go program that re-execs itself after an update. No exception
  entitlement is expected, but none was tested.

### Related

* [0004-MADR-evolve-selfupdate-api-and-tui-support.md](0004-MADR-evolve-selfupdate-api-and-tui-support.md):
  §1, §7 Phase 4, amendment P2, G7 (staging ownership), G9 (the image
  check).
* [0008-MADR-enforce-import-rules-with-depguard.md](0008-MADR-enforce-import-rules-with-depguard.md):
  the rules and amendment D1.
* [0009-MADR-rename-to-go-selfupdate-lib.md](0009-MADR-rename-to-go-selfupdate-lib.md):
  the module's scope.
* [0010-MADR-remediate-second-debugging-pass-findings.md](0010-MADR-remediate-second-debugging-pass-findings.md):
  D14 and Q8, which scheduled `archive`.
* [0011-MADR-reference-service-lifecycles.md](0011-MADR-reference-service-lifecycles.md):
  `service.Runner`, `ErrUnsupported`, and replacing by rename.
* magic-cli-remote: `internal/updateclient/codesign_darwin.go`; records
  `0060-MADR-local-unsigned-build-and-install.md`,
  `0069-MADR-macos-permissions-and-sandbox-parity.md` and
  `0065-PLAN-update-automation.md`.
* mcplib: `0005-MADR-canonicalize-cli-self-update-in-mcplib.md`, F8 (a
  re-sign is a transform, not verification) and its argument against
  archive breadth.
