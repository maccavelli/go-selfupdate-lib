---
status: in-progress
date: 2026-10-05
associated-madr: "0012-MADR-archive-assets-and-macos-codesign.md"
---
# Implement `selfupdate/archive` and `selfupdate/codesign`: the extract stage, tar.gz, zip and gz assets, and opt-in macOS re-signing and signature checks (`v1.8.0`)

Associated MADR: [0012-MADR-archive-assets-and-macos-codesign.md](0012-MADR-archive-assets-and-macos-codesign.md)

## Goal

* `selfupdate` gains the extract stage, as MADR §2 and §7 describe:
  `Unpacker`, `UnpackerFunc`, `UnpackRequest`, `Config.Unpacker`,
  `Selection.Packed`, `EventUnpacking`, `CheckImage` and
  `ChainTransformers`.
* `selfupdate/archive` selects and extracts the program from a tar.gz, zip
  or gz release asset, and refuses every archive MADR §4 lists.
* `selfupdate/codesign` re-signs (`NewSigner`) and checks (`NewChecker`)
  on macOS, through `service.Runner` and `/usr/bin/codesign` (MADR §5,
  §6).
* **Nothing changes for a program that configures none of it.** No
  default runs `codesign` or inspects a signature (MADR §10). An unsigned
  or linker-signed binary updates exactly as in `v1.7.0`.
* `v1.8.0` is tagged. `make apicheck` reports it compatible with `v1.7.0`.

## Scope

### In scope

| Phase | Work |
| :--- | :--- |
| U0 | Records: this PLAN, the MADR's answers and acceptance, the `docs/README.md` rows, AGENTS.md's stale package and rule lists. No code. |
| U1 | `selfupdate`: the extract stage, `CheckImage`, `ChainTransformers` |
| U2 | `selfupdate/archive`: selection, and its depguard rule |
| U3 | `selfupdate/archive`: extraction, fuzzing, end to end |
| U4 | `selfupdate/codesign`, its depguard rule, its live tests and CI step |
| U5 | Docs, examples and release notes |
| U6 | The release: the owner's push and tag, the pin commit, the agent's checks |

### Out of scope

* **Publishing archives through `publish-selfupdate-release.yml`.** Its
  `SHA256SUMS` rule stays (MADR §8). That is the build-and-stage
  workflow's record.
* **xz, zstd and bzip2,** several programs per archive, and app bundles
  (MADR §8).
* **Notarization, `spctl` and quarantine handling** (MADR §8).
* **darwin/amd64** (MADR §10). No fixture, test, example, guide text
  or release check builds or targets it. The code does not refuse it
  either.
* **Turning anything on by default.** No `cli` flag, environment variable
  or helper builds a signer or checker (MADR §10).
* **Moving magic-cli-remote onto `codesign.NewSigner`.** That is its own
  repository's work, after it moves from `mcplib v1.4.1` to this module.
* **Push and tags,** which need the owner's ask.

## Rules for every phase

The v1.7.0 PLAN's rules apply
([0011-PLAN-reference-service-lifecycles.md](0011-PLAN-reference-service-lifecycles.md)):

* Tests first, each seen to fail on a deliberately broken input in a
  scratch copy, never in the tree.
* The full gate before each commit: gofmt, `make lint` on three GOOS,
  vet, race, shuffle, `make fuzz`, `make vuln`, `go mod tidy -diff`, the
  script tests, cross vet.
* `make pre-add-check FILES=…` on the staged Go files; markdownlint on the
  records.
* One commit per phase, staged by the agent and committed by the owner.
* A dated deviation entry, and a MADR amendment where a decision or an
  asserted fact changes, before continuing past any surprise.

In addition:

* **API:** `make apicheck` reports `compatible with v1.7.0` at every phase.
  `Selection`, `Request`, `Result`, `Event` and `TransformRequest` stay
  comparable.
* **Modules:** `go.mod` does not change.
* **Live tests** are gated by an environment variable. They skip with
  their reason when it is unset, and fail when it is set and they cannot
  run.
* **The existing suite** passes unmodified at every phase, apart from
  tests that gain cases. This is the proof that a default run is
  unchanged.
* **macOS means darwin/arm64** in every fixture, test and check (MADR
  §10).
* **The Windows test host** runs U1 and U3, for `.exe` names, zip and PE
  images.

## Implementation Steps

### Phase U0: records

1. The MADR, with the owner's answers in §10 (2026-10-05), and status
   `accepted`.
2. This PLAN, with its row in `docs/README.md`.
3. AGENTS.md's module description and depguard list name the four
   `service` packages and their rules, missing since `v1.7.0`. Its
   statement that each package lives in its own top-level directory is
   corrected to match the tree.
4. The owner approves this PLAN before U1.

### Phase U1: the extract stage in `selfupdate`

1. **Types.**
   * `UnpackRequest{Product string; Platform Platform; AssetName string;
     Archive string; Program string; Limit int64}`.
   * `Unpacker` and `UnpackerFunc`, beside `TransformerFunc`.
   * `Config.Unpacker`: nil means none; a typed nil is refused, as for
     `Transformer`.
   * `Selection.Packed bool`, documented as in MADR §2.
2. **`EventUnpacking`.** It goes in a new block of kinds added in v1.8.0,
   after `EventWarning`, so every earlier value keeps its number.
   `EventKind.String` gives `unpacking`.
3. **Matching selector and unpacker.** After discovery, before the
   `CheckOnly` return, `execute` fails when `Selection.Packed` and
   `Config.Unpacker` disagree.
   * The error names both, so a misconfigured program fails on its first
     `--check`.
   * `Checker`, which never installs, does not check this.
4. **`New` refuses known-bad combinations:**
   * `Config.Unpacker` with the selector `NewExactAssetSelector` returns;
   * `Config.Unpacker` with an image verifier from `NewImageVerifier` in
     `Verifiers`.

   Both are recognised by the module's own unexported types.
5. **The stage, in `apply`,** after `runVerifiers` and `EventVerified`,
   when `Selection.Packed`:
   * emit `EventUnpacking{Product, Asset: sel.Binary.Name}`;
   * `sess.CreateStaging` a second time, close the file, and call `Unpack`
     with `Archive` set to the asset's staging path, `Program` to the new
     path, and `Limit` to `Limits.Executable`;
   * apply `hashAndValidateStaging` to `Program`;
   * continue with `Program` as the staging path, its size as the
     installed size and its digest as the installed digest.

   An `Unpack` error fails the run as a transform error does. A dry run
   unpacks.
6. **`CheckImage(r io.ReaderAt, p Platform) error`** is the body of
   `imageVerifier.Verify`, exported, and `NewImageVerifier` calls it. A
   platform it does not cover fails with `ErrUnsupportedPlatform`.
7. **`ChainTransformers(ts ...Transformer) (Transformer, error)`:**
   * runs each in order on the same request;
   * stops at the first error;
   * refuses an empty list, a nil and a typed nil.
8. **Doc comments,** reworded where an archive changes what they mean:
   * `Selection.Binary`: an archive when `Packed`;
   * `Config.Assets`: no longer "exact raw-binary names" only;
   * `Verification`: the staged bytes are the release asset;
   * `TransformRequest.ReleaseDigest`, `StagedArtifact`,
     `Result.ReleaseDigest` and `Result.InstalledDigest`: as MADR §2
     states;
   * the package doc's pipeline order.
9. **Tests:**
   * **Event order:** `TestRunEventOrderWithUnpacker`, with an unpacker
     alone and with a transformer too, giving `… Verified, Unpacking,
     Transforming, Installing, Complete`. The existing event-order tests
     keep their lists.
   * **Mismatches:**
     * `Packed` with no unpacker, and an unpacker with an unpacked
       selection, each fail before any asset body is opened (the fake
       source counts opens);
     * `--check` fails the same way;
     * `New` refuses both known-bad combinations.
   * **Staging:**
     * `Program` must be a regular file, not a symlink, within the limit
       and owned;
     * a session that is not a `StagingOwner` fails closed;
     * both staging files are gone after `Close`;
     * the leftover sweep removes both names after a simulated crash.
   * **Digests:** `ReleaseDigest` is the archive's; `InstalledDigest` is
     the program's after the unpack and a transform.
   * **Dry run:** a dry run unpacks and installs nothing.
   * **`CheckImage`:** the `TestImageVerifier` cases through both entry
     points.
   * **`ChainTransformers`:** order, the first error stops the chain, the
     refusals, and the coordinator rehashes after the chain.
   * **API:** `apicheck` is compatible, and `Selection` stays comparable
     (a compile-time `==`).
10. **Plants,** in a scratch copy:
    * the stage skipped;
    * `Program` not validated;
    * the mismatch check removed;
    * `EventUnpacking` not emitted;
    * the chain run in reverse.

### Phase U2: `selfupdate/archive`, selection

1. **The package.** Its doc says it compiles on every OS, uses only the
   standard library, and serves releases published by other tooling
   until the publish workflow accepts archives (MADR §8).
2. **`Format`:** `TarGz`, `Zip` and `Gz`, with the extensions `.tar.gz`
   (`.tgz` accepted when reading), `.zip` and `.gz`.
3. **`SelectorOptions` and `NewSelector`,** as MADR §3 declares.
   * `Platforms` is validated by building `NewExactAssetSelector` over the
     same list, so both selectors accept exactly the same lists and no new
     API is needed.
   * Defaults: `Format` gives zip on windows and tar.gz elsewhere; `Name`
     is `FleetName`; `Manifest` is `"SHA256SUMS"`.
4. **`Select`:**
   * refuses a platform outside the list (`ErrUnsupportedPlatform`);
   * builds both names and refuses any that fails the name check;
   * requires exactly one asset of each name;
   * returns `Selection{Binary, Manifest, ManifestName: <archive name>,
     Packed: true}`.
5. **The name check** is the release workflow's regular expression,
   `^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`, plus "not `SHA256SUMS` and not a
   path". A test reads the expression out of
   `scripts/verify-selfupdate-release.sh` and requires it to equal the
   package's, so the two cannot drift.
6. **The naming helpers:**
   * `FleetName`: `<product>-<os>-<arch>.<ext>`;
   * `GoReleaserName`: `<product>_<tag without "v">_<os>_<arch>.<ext>`. It
     refuses `arm`, `mips`, `mipsle`, `mips64` and `mips64le`, whose
     default GoReleaser names carry a variant `Platform` does not hold;
   * `GoReleaserChecksums`: `<product>_<tag without "v">_checksums.txt`.

   The doc of `GoReleaserName` says GoReleaser's default format is tar.gz
   on every OS, so a GoReleaser consumer sets `Format` to return `TarGz`
   always.
7. **depguard:**
   * rule `selfupdate-archive`: `**/selfupdate/archive/*.go`, `!$test`,
     allowing `$gostd` and `…/selfupdate$`;
   * excluded from `other-packages`.

   AGENTS.md names the package and the rule.
8. **Tests:**
   * the default names on linux, darwin and windows;
   * `GoReleaserName` for `v1.2.3` (`relay_1.2.3_linux_amd64.tar.gz`) and
     its refusals;
   * a custom `Name` returning a bad name is an error, not a miss;
   * a missing or duplicate archive, or manifest, is refused;
   * `Packed` and `ManifestName`;
   * the regular-expression differential;
   * invalid `Platforms` are refused as the exact selector refuses them;
   * depguard: a planted `x/term` import fails `make lint`.
9. **A real GoReleaser `checksums.txt`.** One is fetched once from a public
   GoReleaser release, kept under `selfupdate/archive/testdata/` with its
   source URL in a comment file, and parsed with `ParseSHA256SUMS`. This
   settles the MADR's "Not verified" entry, and if it does not parse, that
   is a deviation.

### Phase U3: `selfupdate/archive`, extraction

1. **`UnpackOptions` and `NewUnpacker`,** as MADR §4 declares.
   * `Member`, when set, must satisfy `fs.ValidPath`.
   * `MaxEntries` of zero means 4096; a negative value is refused.
2. **Format detection:**
   * by the suffix of `UnpackRequest.AssetName`;
   * then the first bytes of `Archive`: gzip `1f 8b`, zip `PK\x03\x04` or
     `PK\x05\x06`;
   * an unknown suffix or a mismatch is refused.
3. **tar.gz,** in one pass over `tar.NewReader(gzip.NewReader(…))`:
   * count entries against `MaxEntries`;
   * check every name (MADR §4): not empty or absolute, no `..`, no
     backslash, no NUL, `filepath.IsLocal`;
   * check duplicates after `path.Clean`, and ignoring case;
   * refuse `TypeLink`, `TypeSymlink`, `TypeChar`, `TypeBlock`,
     `TypeFifo`, `TypeGNUSparse` and PAX sparse headers;
   * sum the declared sizes of regular files against `Limit`;
   * stream the program, when it is found, into `Program` through a reader
     capped at `Limit`+1;
   * keep reading to the end, so a later duplicate or bad entry still
     refuses the archive.

   The decompressed stream as a whole is capped at `Limit` plus an 8 MiB
   allowance for headers. Any limit is reached as a refusal, never as a
   truncated program.
4. **zip,** with `zip.NewReader` over the archive's staging file:
   * before reading any data, the same entry-count, name, duplicate and
     declared-size checks, over the central directory;
   * a mode that is not a regular file or a directory is refused;
   * a method other than store or deflate is refused;
   * data ranges are checked: each entry's `DataOffset()` and
     `CompressedSize64` must lie inside the file, and sorted ranges must
     not overlap;
   * the program is read to EOF through a reader capped at `Limit`+1, so
     the standard library checks its size and CRC.
5. **gz:**
   * one member, with `Multistream(false)`;
   * the whole stream is the program, capped at `Limit`+1;
   * bytes after the member are refused.
6. **The program:**
   * named by `Member`, or else the one regular file whose base name is
     the product (plus `.exe` on windows) at depth 0 or 1;
   * none, or more than one, is refused.
7. **After writing,** the program file is closed and `CheckImage` runs on
   it for `UnpackRequest.Platform`.
8. **Errors:** refusals wrap `selfupdate.ErrIntegrity` with the prefix
   `selfupdate: archive:`, and name the entry, sanitized. The context is
   checked between entries.
9. **Tests.** Fixtures are built in the tests with `archive/tar`,
   `archive/zip` and `compress/gzip`, around real Go executables
   cross-built as `imageverify_test.go`'s `buildFixture` builds them.
   Malformed archives are hand-assembled.
   * **Accepted:** each format; the program at depth 0 and at depth 1;
     beside a README and a LICENSE; with directory entries; through
     `Member`; with a `.tgz` name; `.exe` in a zip on windows.
   * **Refused, one test for each MADR §4 rule:**
     * an absolute name, a `..`, a backslash, a NUL;
     * duplicate names, and names that differ only in case;
     * a symlink, and a hard link, named as the program;
     * a device, a FIFO, a GNU sparse entry and a PAX sparse entry;
     * `MaxEntries`+1 entries;
     * a declared size over `Limit`, and a gzip bomb over `Limit` that
       declares less;
     * overlapping zip entries;
     * an unknown zip method;
     * a suffix that does not match the magic;
     * no program, two programs, and the program two directories down;
     * a wrong-architecture image;
     * trailing bytes after a gz member;
     * a cancelled context.
   * **End to end:**
     * `selfupdatetest` releases whose assets are archives and whose
       `SHA256SUMS` lists them run through `Updater` with `NewSelector` and
       `NewUnpacker`, in fleet and GoReleaser naming, applied and dry-run,
       into a temporary target;
     * the installed program's bytes equal the fixture;
     * `Result.AssetName` is the archive;
     * the same releases run through `cli.Command`, in text and `--json`,
       show the `unpacking` event.
10. **Fuzzing:**
    * `FuzzUnpackTarGz`, `FuzzUnpackZip` and `FuzzUnpackGz` call the
      unexported extraction with the image check off. Invariants: no
      panic, at most `Limit` bytes written, and every error a refusal or
      the context's.
    * Seeds: the accepted fixtures and each refused one.
    * `make fuzz` runs `scripts/go-fuzz.sh -t $(FUZZTIME) -m 3
      ./selfupdate/archive` after the `selfupdate` run.
    * CI's fuzz-corpus artifact adds `selfupdate/archive/testdata/fuzz/`.
11. **Plants,** in a scratch copy, one for each refusal class, each caught
    by its test: a name check removed, the duplicate check removed, links
    accepted, the size cap removed, the overlap check removed, the depth
    rule widened, the image check removed.

### Phase U4: `selfupdate/codesign`

1. **The package:**
   * its doc says it is opt-in, for a publisher who signs, and that no
     default runs it (MADR §10);
   * it compiles on every OS;
   * its constructors take the OS through an unexported seam, as
     `launchd.newJob` does, and return `service.ErrUnsupported`, wrapped,
     off darwin.
2. **`SignOptions` and `NewSigner`** (MADR §5).
   * **`Identity`:** required. It must not begin with `-` unless it is
     exactly `-`.
   * **`Identifier`:** required, matching `^[A-Za-z0-9][A-Za-z0-9.-]*$`.
   * **`Keychain` and `Codesign`:** absolute when set; `Codesign` defaults
     to `/usr/bin/codesign`.
   * **`Requirement`:** no newline or NUL, and no leading `=`.
   * **`Runner`:** defaults to `service.ExecRunner()`.
3. **`Transform`:**
   * refuses a platform that is not darwin;
   * runs the two commands of MADR §5 with the environment
     `PATH=/usr/bin:/bin:/usr/sbin:/sbin` and `LC_ALL=C`;
   * passes the requirement as one argument:
     `-R=identifier "<Identifier>"`, followed by `and (<Requirement>)`
     after a space when a requirement is set;
   * maps exit codes as MADR §5 does;
   * puts the tool's output in the error, capped at 1 KiB, sanitized and
     on one line.
4. **`CheckOptions` and `NewChecker`** (MADR §6):
   * at `ProbeStaged` it runs
     `codesign --verify --strict [-R=<Requirement>] <path>`;
   * at `ProbeInstalled` it does nothing;
   * exits 1 and 3 wrap `selfupdate.ErrIntegrity`.
5. **depguard:**
   * rule `selfupdate-codesign`: `**/selfupdate/codesign/*.go`, `!$test`,
     allowing `$gostd`, `…/selfupdate$` and `…/selfupdate/service$`;
   * excluded from `other-packages`.

   AGENTS.md names the package and the rule.
6. **No default reaches it:** `go list -f '{{.ImportPath}}:
   {{join .Imports " "}}' ./...` shows no package outside
   `selfupdate/codesign` importing it. The output is recorded. The strict
   allow lists of every other rule already deny it, apart from
   `other-packages`, whose packages are internal helpers.
7. **Tests with a fake `Runner`** (as `launchd/fake_test.go`):
   * the exact argv for each option combination, and the environment;
   * each exit code's error, and that `errors.Is(err, ErrIntegrity)` holds
     only for the checker's exits 1 and 3;
   * every constructor refusal;
   * `service.ErrUnsupported` for linux and windows;
   * a non-darwin platform refused before the runner is called;
   * the checker's no-op at `ProbeInstalled`.
8. **Live tests** (`live_darwin_test.go`, with a `live_other_test.go`
   stub):
   * **Gated by `SELFUPDATE_REQUIRE_CODESIGN=1`, run in CI:**
     * `TestLiveSignAdHoc`: a copy of a cross-built darwin/arm64 fixture,
       signed with `Identity: "-"` and an identifier. `codesign -d -v`
       shows that identifier, and the checker accepts it.
     * `TestLiveCheckerExitCodes`, on the checker alone:
       * a linker-signed arm64 build is accepted;
       * a copy with its signature removed (`codesign --remove-signature`)
         is refused (exit 1);
       * a copy with one byte changed is refused (exit 1);
       * `Requirement: "anchor apple generic"` on an ad-hoc signature is
         refused (exit 3).
   * **Gated by `SELFUPDATE_CODESIGN_IDENTITY`, never set in CI:**
     `TestLiveSignIdentity` signs with that identity (and
     `SELFUPDATE_CODESIGN_KEYCHAIN` when set). It checks:
     * `Authority=` lines and the identifier in `codesign -d -v`;
     * the checker with `Requirement: "anchor apple generic"`.

     It is the test for when the owner has an identity (MADR §10), and
     skips with that reason otherwise.
9. **CI:** a macos-15 step, "codesign live test", with
   `SELFUPDATE_REQUIRE_CODESIGN: "1"`, runs
   `go test -count=1 -v -run '^TestLive' ./selfupdate/codesign`.
   `scripts/check-workflows.sh` passes it.
10. **Plants,** in a scratch copy:
    * `--identifier` dropped;
    * the identifier left out of `-R`;
    * the darwin platform check removed;
    * exit 3 treated as success;
    * the runner given a relative path;
    * live, the checker treating exit 1 as success.

### Phase U5: docs, examples and release notes

1. **`docs/guides/extending-selfupdate.md`:**
   * a new "Ship an archive" section: the selector and the unpacker
     together; fleet and GoReleaser naming; `Format` for GoReleaser;
     immutable releases; why this repository's workflow cannot publish one
     yet;
   * a new "Sign on macOS" section: opt-in, for a publisher who signs;
     `Identity`, `Identifier`, `Requirement`; the checker, which a binary
     with no signature fails; signing needs Xcode or the Command Line
     Tools, not verified;
   * "Verify a signature later" and "Probe the new binary" state where the
     extract stage falls.
2. **Other docs:**
   * `docs/architecture.md`: the tree, the package table, and what each
     package holds;
   * `README.md`: the package table;
   * `docs/README.md`: "I want to…" rows for archives and signing;
   * AGENTS.md, if U2 and U4 left anything.
3. **The migration guide,** under the standing rule that it follows the
   current release: a new "8. From v1.7 to v1.8" section with the
   additions and how to adopt them. Every other version reference moves
   to `v1.8.0` in U6's pin commit.
4. **Examples** that compile on every OS, with no `// Output:` where they
   would run a tool: `ExampleNewSelector` and `ExampleNewUnpacker`
   (wiring a `Config`), `ExampleNewSigner`, `ExampleNewChecker` and
   `ExampleChainTransformers`.
5. **Release notes** in this PLAN's execution record: the additions only;
   `compatible with v1.7.0`; `go.mod` unchanged; nothing changes for a
   program that does not configure the new pieces.

### Phase U6: the release

1. **The owner** pushes, waits for CI, and tags `v1.8.0` (annotated).
2. **After the tag,** one commit:
   * moves the workflow pins in `README.md` and the migration guide to the
     tag's commit;
   * moves the migration guide's `go get` commands and §5's `go.mod` step
     to `v1.8.0`;
   * gives `architecture.md` the tag's commit;
   * marks this PLAN `complete`;
   * states whether `publish-selfupdate-release.yml` and `scripts/`
     changed between `v1.7.0` and `v1.8.0` (expected: no).
3. **The agent's checks:**
   * CI on the tag, and the proxy's `@latest`;
   * a scratch consumer requiring `v1.8.0` from the proxy, built for
     linux, darwin and windows. On each test host it updates from a fake
     source in each format and both namings, and shows a misconfigured
     selector and unpacker refused on `--check`;
   * on the development Mac, the consumer re-signs ad-hoc with
     `NewSigner`, and `NewChecker` refuses a darwin/arm64 build with its
     signature removed.

     A default `Config` updates a linker-signed darwin/arm64 build, the
     shape the fleet ships, and also that signature-stripped copy. Nothing
     in a default run looks at a signature. That is the owner's condition
     (MADR §10), shown on the release itself.

## Verification

* **V1.** Every new test and gate was seen to fail on a deliberately
  broken input, recorded per phase.
* **V2.** `make apicheck`: `compatible with v1.7.0`. `go.mod` is
  unchanged.
* **V3.** A default run is unchanged:
  * the suite of `v1.7.0` passes unmodified, apart from added cases;
  * no package outside `selfupdate/codesign` imports it;
  * a default `Config` updates a linker-signed darwin/arm64 build, and a
    copy with its signature removed (U6).
* **V4.** Every refusal in MADR §4 has a passing test on all three CI
  runners, and the archive fuzz targets run clean in `make fuzz`.
* **V5.** The `codesign` live tests pass in CI on macos-15, none skipped,
  and `TestLiveSignIdentity` skips there with its reason.
* **V6.** The MADR's "Not verified" entries are each restated with what
  this PLAN established, or left open with the reason.
* **V7.** CI is green on `main` and on `v1.8.0`.

## Rollout and Rollback

* **Rollout.** Every addition is opt-in. A program changes behaviour only
  when it sets `Config.Unpacker`, uses the archive selector, or builds a
  signer or checker.
* **Rollback.** Before the tag, any phase reverts alone. U2 to U4 depend
  on U1, and U3 on U2. After the tag, fix forward in `v1.8.x`. A defect in
  `archive` or `codesign` affects only programs that configured it.

## Execution Record

### Phase U0: records (2026-10-05)

* **The MADR** carries the owner's answers in §10, and is `accepted`:
  * Q1, "build it";
  * on `codesign`, nothing that blocks a run on macOS, since the owner's
    binaries are not signed: the package is opt-in and for later;
  * on darwin/amd64, "i do not want, nor need to support amd64 darwin".
* **Added while planning,** before acceptance: §4 also refuses entry
  names that match ignoring case, and the probe evidence records an
  arm64 copy with its signature removed failing `codesign --verify`
  with exit 1.
* **The owner approved this PLAN** ("proceed", 2026-10-05).
* **AGENTS.md:**
  * names the four `service` packages and their depguard rules, missing
    since `v1.7.0`;
  * no longer says every package has its own top-level directory: only
    `buildinfo` and `selfupdate` do, and the subpackages live under
    `selfupdate/`.
* **`docs/README.md`** indexes the MADR (`accepted`) and this PLAN
  (`in-progress`).
* **No code.** Checks: markdownlint on the records, `AGENTS.md` and
  the index; the link and anchor check; the identifier scan.

### Phase U1: the extract stage in `selfupdate` (2026-10-05)

* **Built,** as steps 1 to 8 say:
  * `UnpackRequest`, `Unpacker`, `UnpackerFunc` and `Config.Unpacker`;
  * `Selection.Packed`;
  * `EventUnpacking`, in a block of kinds added in v1.8.0, numbered
    `EventWarning`+1;
  * `CheckImage`;
  * `ChainTransformers`.
* **`New`'s refusals.** `validUnpacker` refuses a typed nil, the selector
  that `NewExactAssetSelector` returns, and an image verifier among the
  `Verifiers`.
* **The match check.** `matchUnpacker` runs in `execute` right after
  discovery, so `--check` fails too.
* **The stage.** `unpack` creates the second staging file, closes it, calls
  the unpacker with `Limit` set to `Limits.Executable`, and checks the
  result with `hashAndValidateStaging`. That function now takes a noun for
  its errors: "transformed staging", as before, or "unpacked program".
  An unpacker's error is passed through unwrapped; `archive` wraps its own
  refusals in `ErrIntegrity` (MADR §4).
* **Doc comments** follow step 8: `Selection`, `AssetSelector`,
  `Verification`, `TransformRequest.ReleaseDigest`, `StagedArtifact`,
  `Result.AssetName` and both `Result` digests, `Config.Assets`,
  `Config.Verifiers`, `New`, and the package doc's pipeline.
* **Tests:**
  * New, in `selfupdate/unpack_test.go`. They use a stand-in archive
    format, a fixed prefix before the program, with a test selector and
    unpacker:
    * `TestRunEventOrderWithUnpacker`, with and without a transform;
    * `TestUnpackRequest`: the request's fields, the archive on disk, and
      both staging names matched by `isLeftover`, so a crash leaves
      nothing the sweep misses;
    * `TestDryRunUnpacks`;
    * `TestUnpackFailures`: an unpacker error, a symlink, over the limit;
    * `TestUnpackRequiresStagingOwner`;
    * `TestPackedSelectionNeedsUnpacker`, for both mismatches, in an
      apply and a check, with no asset opened;
    * `TestNewRefusesUnpackerCombinations`;
    * `TestEventUnpacking`, `TestUnpackerFunc`, `TestChainTransformers`
      and `TestRunWithChainedTransformers`;
    * a compile-time `==` on `Selection`.
  * `TestImageVerifier` gained a `CheckImage` assertion for every row
    except those of darwin/amd64, which this work does not target (MADR
    §10). `TestCheckImageUnsupportedPlatform` is new.
  * No other test changed.
* **Plants,** each in a scratch copy, each caught:

  | Plant | Caught by |
  | --- | --- |
  | the stage skipped | `TestRunEventOrderWithUnpacker`: no `unpacking` event |
  | the program not validated | `TestUnpackFailures` |
  | the match check removed | `TestPackedSelectionNeedsUnpacker`: a nil `Unpacker` panicked, the failure the check exists to prevent |
  | `EventUnpacking` not emitted | `TestRunEventOrderWithUnpacker` |
  | the chain run in reverse | `TestChainTransformers`: `order [b:/p a:/p]` |
  | `New`'s refusals removed | `TestNewRefusesUnpackerCombinations` |
  | `CheckImage` accepting anything | `TestImageVerifier`: "CheckImage linux-amd64 as linux/arm64: <nil>" |

* **Fixed on the way, all in my own new code:**
  * `TestUnpackRequest` compared a staging directory with the target's
    unresolved path; it now resolves symlinks first.
  * `TestPackedSelectionNeedsUnpacker` sent `--check` with `--yes`, which
    the run rightly refuses as contradictory.
  * The ownership test first used `sameBaseSession`. That session creates
    one fixed path, so a second staging file fails with "file exists"
    before ownership is ever checked. It now uses a session that hides
    `Owns` and nothing else.
  * The shared helper `newImageVerifier` tripped revive's
    `confusing-naming`, beside `NewImageVerifier`, and is now
    `imageCheckFor`.
* **The Windows test host:** `go test ./selfupdate/ ./selfupdate/cli/`
  passes.
* **Checks** (`gate.sh`, every one rc 0):
  * gofmt; `make lint`, 0 issues on linux, darwin and windows; vet;
  * race and shuffle;
  * `go mod tidy -diff`; `make apicheck`: `compatible with v1.7.0`;
  * `make fuzz`: 5 targets clean; `make vuln`: no vulnerabilities;
  * every script test; cross vet.

  `make pre-add-check` passes on the eight Go files.

### Phase U2: `selfupdate/archive`, selection (2026-10-05)

* **CI on U1** (`6d1c80a`, run 37333388476) is green on all three
  runners.
* **Built,** in `selfupdate/archive/doc.go` and `select.go`:
  * `Format`, with `TarGz`, `Zip` and `Gz`, and `Format.Extension`;
  * `SelectorOptions` and `NewSelector`;
  * `FleetName`, `GoReleaserName` and `GoReleaserChecksums`.
* **Select:**
  * The product, the platform's fields and the platform list are checked
    by asking the exact selector, built from the same list, about a
    release holding only the names it wants. Both selectors therefore
    refuse alike, with no new API. The exact selector's checks are
    unexported.
  * Asset state and digest syntax need no check here: discovery's
    `validateAssetMetadata` checks every selected asset.
* **`Format.Extension`** is exported beyond MADR §3's sketch, so a
  consumer's own `Name` can build names. It is additive.
* **depguard:** the rule `selfupdate-archive` (`$gostd` and
  `…/selfupdate$`), excluded from `other-packages`. AGENTS.md names the
  package and the rule.
* **Tests,** in `selfupdate/archive/select_test.go`:
  * `TestFormatExtension`, `TestFleetName` and `TestGoReleaserNames`,
    with the refusal of arm and the mips architectures;
  * `TestNewSelectorRefusesPlatforms`: each refusal contains the exact
    selector's own error;
  * `TestSelectDefaults` and `TestSelectGoReleaser`;
  * `TestSelectRefusals`, 14 cases: a platform outside the list
    (`ErrUnsupportedPlatform`), an invalid product, a missing or
    duplicate archive or manifest, names with a slash, `..`, empty,
    129 characters, or naming the manifest, a bad manifest name, a
    `Name` error, and an unknown format;
  * `TestNameCheckMatchesReleaseWorkflow`, which reads `product_re` out of
    `scripts/verify-selfupdate-release.sh`;
  * `TestSelectorWithUnpacker`: `selfupdate.New` accepts this selector
    beside an `Unpacker`.
* **Step 9, a real GoReleaser checksum file.**
  `testdata/goreleaser-v2.18.2-checksums.txt` is `checksums.txt` from the
  immutable release v2.18.2 of goreleaser/goreleaser, fetched unchanged
  (SHA-256 `0818c962…0a47`). `TestGoReleaserChecksumsFile` checks that:
  * it parses with `ParseSHA256SUMS`: 53 entries, two fields per line;
  * a consumer-written `Name` selects GoReleaser's own archives from it.

  This settles the MADR's "Not verified" entry about GoReleaser's
  checksum format. The release also shows the uname style in use:
  `goreleaser_Darwin_arm64.tar.gz`, `goreleaser_Linux_x86_64.tar.gz`,
  `goreleaser_Windows_x86_64.zip`. GoReleaser's own configuration is not
  its default template, so the MADR's choice to leave that style to a
  consumer's `Name` stands.
* **Plants,** each in a scratch copy, each caught:

  | Plant | Caught by |
  | --- | --- |
  | the platform check skipped | `TestSelectRefusals`: "no asset … riscv64", want "not in the product platform matrix" |
  | the name check removed | `TestSelectRefusals`: "no asset \"dir/relay.tar.gz\"" |
  | duplicates accepted | `TestSelectRefusals`: `<nil>`, want "duplicate asset" |
  | not marked `Packed` | `TestSelectDefaults` |
  | GoReleaser variants accepted | `TestGoReleaserNames`: arm accepted |
  | the name check drifting from the workflow's | `TestNameCheckMatchesReleaseWorkflow` |
  | a `golang.org/x/term` import | depguard: "not allowed from list 'selfupdate-archive'" |

* **Checks** (`gate.sh`, every one rc 0):
  * gofmt; `make lint`, 0 issues; vet; race and shuffle;
  * tidy; `make apicheck`: `compatible with v1.7.0`;
  * fuzz; vuln; the script tests; cross vet.

  `make pre-add-check` passes on the three Go files.

### Phase U3: `selfupdate/archive`, extraction (2026-10-05)

* **CI on U2** (run 37334879799) is green on all three runners.
* **Built,** in `selfupdate/archive/unpack.go`:
  * `UnpackOptions` and `NewUnpacker`;
  * the format from the suffix, then the magic bytes;
  * tar.gz, zip and gz extraction, each with the rules of MADR §4;
  * the image check through `selfupdate.CheckImage`.
* **The rules** are enforced as steps 3 to 7 say:
  * entry names: empty, absolute, a backslash or a NUL, and anything
    `path.Clean` leaves non-local by `filepath.IsLocal`;
  * duplicates after cleaning, and ignoring case;
  * link, device, FIFO and sparse entries anywhere;
  * the entry count;
  * the sum of declared sizes against `Limit`, and the decompressed tar
    stream against `Limit` plus 8 MiB for headers;
  * zip methods other than store and deflate, and data ranges outside the
    file or overlapping;
  * the program at depth 0 or 1, or `Member`;
  * exactly one program, read through a cap of `Limit`+1;
  * a gz member followed by anything.
* **A sparse entry in PAX records** is caught by its `GNU.sparse.*` keys.
  A probe showed that `archive/tar` returns such an entry as a regular
  file named for the real file, and that its writer drops those keys, so
  the fixture is hand-built. A GNU sparse header (`'S'`) is refused by the
  reader itself ("invalid tar header"), which the unpacker reports.
* **Deviations from the steps as written,** none of which changes a
  decision:
  * **Order.** `unpack.go` was written before its tests, against the
    "tests first" rule. Each rule's test was then seen to fail on a plant
    (below), which is the evidence that rule asks for.
  * **The fuzz targets call the unexported `extract`,** which has no image
    check. The test-only switch to turn the check off was not needed and
    was removed.
  * **`Member` does not apply to a `.gz` asset,** which has no members.
    It is ignored there, and the doc says so.
* **Tests:**
  * `main_test.go` cross-builds three real executables once: this host's,
    a linux one on another architecture, and windows/amd64. None is
    darwin/amd64. It also holds the archive builders and the hand-built
    sparse fixtures.
  * `unpack_test.go`:
    * `TestNewUnpackerRefuses`;
    * `TestUnpackAccepts`, 7 cases: tar.gz beside other files with `./`
      and directory entries, one directory down, `.tgz`, `Member` two
      directories down, zip, a windows `.exe` in a zip, and gz;
    * `TestUnpackRefuses`, 41 cases, one or more for each rule of MADR §4;
    * `TestUnpackContextAndLimit`: a cancelled context, a zero limit, and a
      platform with no image check.
  * `e2e_test.go`:
    * `TestUpdaterInstallsFromArchive`, 12 cases: tar.gz, zip and gz, in
      fleet and GoReleaser naming, applied and dry-run, through `Updater`
      with `selfupdatetest`'s fake source. Each checks the installed
      bytes, `AssetName`, and both digests;
    * `TestCommandShowsUnpacking`: through `cli.Command`, `unpacking` in
      text and in `--json`.
* **Fuzzing:**
  * `FuzzUnpackTarGz`, `FuzzUnpackZip` and `FuzzUnpackGz`, seeded with
    accepted and refused archives;
  * `make fuzz` runs them after `selfupdate`'s (`-m 3`): "3 fuzz targets
    ran clean in ./selfupdate/archive";
  * CI's fuzz-corpus artifact keeps `selfupdate/archive/testdata/fuzz/`.
* **Plants,** each in a scratch copy, all 15 caught on the final code:

  | Plant | Caught by (`TestUnpackRefuses/…`) |
  | --- | --- |
  | the name check removed | backslash, NUL |
  | the `IsLocal` check removed | dot-dot, dot-dot inside, zip dot-dot |
  | the duplicate check removed | the four duplicate cases |
  | case folding removed | differs only in case |
  | links accepted | both links named as the program, a symlink elsewhere |
  | PAX sparse accepted | PAX sparse |
  | the declared size unchecked | declared over the limit, entries together over the limit |
  | the program's size uncapped | gz bomb |
  | the overlap check removed | overlapping entries |
  | the zip method unchecked | unknown method |
  | the entry count unchecked | too many entries |
  | the depth rule widened | two directories down |
  | the image check removed | wrong architecture, not an executable |
  | the magic check removed | zip named tar.gz |
  | gz trailing data accepted | data after the member, a second member |

  **The first round missed one, which was dead code.** An explicit
  check for `..` components changed nothing when removed, because
  `path.Clean` folds every `..` it can and `IsLocal` refuses the rest. The
  loop is gone, and the `IsLocal` plant shows the `..` cases depend on
  `IsLocal`.
* **Fixed on the way:**
  * **The Windows test host failed eight cases.** My fixtures named the
    program `relay`, where on windows it is `relay.exe`, so four refusal
    cases failed for the wrong reason ("holds no program"). The tests now
    name the program for the host. The end-to-end tests already did.
  * **Lint:**
    * `ReadAt`'s error is checked;
    * a parameter named `max` is renamed;
    * the test helper `newUnpacker` is `mustUnpacker`, beside
      `NewUnpacker`;
    * the end-to-end test's body is a helper, for gocognit;
    * gosec G115 flagged four integer conversions in the zip checks. They
      are gone: one bounds-checked `uint64` to `int64` conversion, and
      every comparison in `int64`.

    The plants above were rerun after these changes.
* **Not tested:** a tar entry that holds fewer bytes than it declares.
  `archive/tar` reports such an entry as an unexpected EOF, which reaches
  the unpacker as a read error and a refusal. The check that the program
  holds what it declares is defence beyond that.
* **The Windows test host:** `go test ./selfupdate/archive/ ./selfupdate/`
  passes.
* **Checks** (`gate.sh`, every one rc 0):
  * gofmt; `make lint`, 0 issues; vet; race and shuffle;
  * tidy; `make apicheck`: `compatible with v1.7.0`;
  * `make fuzz`, both packages; vuln; the script tests; cross vet.

  `make pre-add-check` passes on the five Go files.
