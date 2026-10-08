---
status: in-progress
date: 2026-10-08
associated-madr: "0017-MADR-verify-build-provenance-and-close-0015-open-items.md"
---
# Implement 0017: the documentation on main, `v1.11.1` (the zip tiling and Windows-name refusals), and `v1.12.0` (the interrupted-update journal and `selfupdate/verify/ghattest`)

Associated MADR: [0017-MADR-verify-build-provenance-and-close-0015-open-items.md](0017-MADR-verify-build-provenance-and-close-0015-open-items.md)

## Goal

* **Every item the MADR chose ends built, tested and documented,** in the
  track the owner chose (question 8):
  * 5A and question 3's guide text on `main`;
  * 2B and 4C in `v1.11.1`;
  * 3B and 1B in `v1.12.0`.
* **Each new check is seen failing first:** on `v1.11.0`'s code, then on a
  planted break in a scratch copy, before it is committed.
* **Each release stays API-compatible:**
  * `v1.11.1` changes no exported identifier;
  * `v1.12.0` only adds, and its full apidiff report lists exactly the
    additions this PLAN names.
* **`go.mod` and `go.sum` do not change.** Depguard gains exactly one rule,
  `selfupdate-verify-ghattest`.
* **The records stop overstating.** That covers the publish queue, the
  crash leftovers and the open-work table.

Every fact this PLAN rests on was read at `70ad0be` (`v1.11.0` plus
records), unless a step names a scratch experiment of 2026-10-08. Line
numbers are at `70ad0be`. Bare Go file names are in `selfupdate/`;
`archive/` is `selfupdate/archive/`, and `ghattest/` is
`selfupdate/verify/ghattest/`.

## Scope

### In scope

| Phase | Track | MADR item | Commit |
| :--- | :--- | :--- | :--- |
| R0 | records | the MADR's amendment A1 (this PLAN's corrections), the index | records only (bootstrap exception) |
| D1 | `main` | 5A; question 3's tag ruleset; the 0004 open-work table; one stale index row | documentation |
| P1 | `v1.11.1` | 2B: zip entries tile the file | `archive/` |
| P2 | `v1.11.1` | 4C: names Windows reserves, on every host | `archive/` |
| P3 | `v1.11.1` | the release commit, then the release procedure | documentation, then the tag and pins |
| Q1 | `v1.12.0` | 3B: the interrupted-update journal and `KeptBackups` | `selfupdate/` |
| Q2 | `v1.12.0` | 1B: `selfupdate/verify/ghattest` | new package, depguard, scope docs |
| Q3 | `v1.12.0` | the release commit, then the release procedure | documentation, then the tag and pins |

### Out of scope

* **1E,** the in-process `sigstore-go` verifier, which gets its own
  repository and records (MADR, "Not decided here").
* **3C,** automatic rollback and a persisted reconcile receipt.
* **`queue: max` itself.** D1 records the trigger: an actionlint release
  that accepts `queue`. The key, the pin bump and the shape assertion land
  together in the release after that.
* **A product named after a Windows device** (`aux`, `con`, `com1`).
  `releasespec` allows such a name
  (`releasespec/validate.go:31`). After P2, pack's round trip
  (`internal/cmd/selfupdate-release/pack.go:84-119`) refuses its archive at
  build time with P2's message. Refusing it earlier in `releasespec` is a
  separate decision.
* **A build-side attestation and `actions/attest`** (0013-MADR §10).
* **A tag ruleset in any repository.** D1 only documents it.
* **Push and tags,** which happen only on the owner's ask in the same turn.

## Decisions this PLAN assumes

**The owner's answers of 2026-10-08** (MADR, Owner questions):

| # | Decision | Phase |
| :--- | :--- | :--- |
| Q1 | 1B: a `Policy` and an exec verifier here; 1E later, in its own records | Q2 |
| Q2 | the documented check is `SHA256SUMS`, through a `ManifestVerifier`; a `Verifier` on the asset is built too | Q2 |
| Q3 | the building guide's step 4 recommends a `v*` tag ruleset | D1 |
| Q4 | an interrupted update's backup is kept as `-kept-<n>`, and is reported by a new method that lists kept backups; `CleanupPending` gains no error | Q1 |
| Q5 | every Windows-reserved character and device name, on every host | P2 |
| Q6 | tiling, no prepended or trailing data, and descriptors checked against the central record | P1 |
| Q7 | documentation now, and `queue: max` once actionlint accepts it | D1 |
| Q8 | `v1.11.1` for 2B and 4C; `v1.12.0` for 3B and 1B | all |

**Decisions made while planning (N1–N14).** They are accepted when this
PLAN is approved, and R0 copies them into the MADR as amendment A1.

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

## Corrections to the MADR found while planning

R0 records these in the MADR as amendment A1, with N1–N14.

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

## Rules for every phase

1. **Stay inside the phase.** A step changes only the phase's **Files**.
   A finding outside them stops the phase. It is resolved as the owner's
   global rules say (stop, evidence, resolutions with the recommended one
   first, the consequence of doing nothing), and is recorded as a dated
   deviation in this PLAN before work goes on.
2. **One commit per phase,** with `git commit --no-edit`. The global
   `prepare-commit-msg` hook writes the message. The agent commits when
   the owner's approval covers commits, as 0015's did ("You may commit to
   main and proceed"). Otherwise it stages and stops.
3. **Test first.** A new test fails on the unfixed code with the phase's
   **Red** text, or is marked **pin** and passes before the change.
4. **Plant after.** With the fix in place, each **Plant** is applied with
   `scripts/plant-copy.sh FILE OLD NEW` in a scratch copy. The named test
   must fail there; its failing line is recorded, and the copy is
   removed. The tree is never dirtied for a plant.
5. **Checks before staging,** each with exit status 0:
   * `make gate`, ending `overall=0`;
   * `make pre-add-check FILES="<every staged .go file>"`;
   * `scripts/check-docs.sh --links` on every changed Markdown file, and
     `--ids` on every changed file;
   * `npx --yes markdownlint-cli2@0.23.2`.

   **Each check's output goes to a scratch file,** and `$?` is captured
   before any filter.
6. **API.**
   * **Track 2:** `make apicheck` reports `compatible with v1.11.0`, and
     the full report (Verification V2) lists no exported change.
   * **Track 3:** compatible with `v1.11.1`; the report lists only the
     phase's additions.
7. **No new module.** `go.mod` and `go.sum` are unchanged, and
   `go mod tidy -diff` is clean. Depguard changes only in Q2.
8. **Identifiers.** Nothing committed names a host, an account, or a
   real home path. The test hosts are "the Linux test host" and "the
   Windows test host".

## Phase procedure

1. **Red.** Write the phase's tests. Run the phase's **Run** command and
   save its output to `$GATE_OUT/red.txt`, capturing `$?` before any
   filter. Each new test fails with its **Red** text, or a pin passes.
2. **Fix.** Make the changes the steps name, and only those.
3. **Green.** The same **Run** command passes; save it to
   `$GATE_OUT/green.txt`.
4. **Plants.** For each plant, `scripts/plant-copy.sh FILE OLD NEW`, with
   OLD and NEW as Python string literals. In the printed copy, run
   `go test -count=1 -run '^<Test>$' ./<package>`. It must fail. Record
   the failing line, then `rm -rf` the copy.
5. **Docs** named by the steps are edited.
6. **Checks** of rule 5, and rule 6's API check.
7. **Record.** This PLAN's execution record gains the phase's dated
   section, with:
   * each test's red line and its plant's failing line;
   * the gate's `name rc=N` lines;
   * apicheck's last line;
   * any deviation.
8. **Stage** exactly the **Files** list with `git add -- <files>`.
   `git diff --cached --name-only` must equal the list, and
   `git status --short` must show nothing else modified. Then commit
   (rule 2).

`GATE_OUT` is the gate's output directory, a `mktemp -d` unless it is
set.

## Live tests

| Phase | Test | Where |
| :--- | :--- | :--- |
| Q1 | `TestWindowsInterruptedApplyRunningImage` | CI's `windows-2025` leg (it is an ordinary Windows test), and the Windows test host with `TMP` and `TEMP` at an 8.3 path, 30 runs |
| Q2 | `TestLiveVerify` with `SELFUPDATE_REQUIRE_GHATTEST=1` | this Mac, with `gh` logged in; the Windows test host, with its `gh` |

* **CI has no `GH_TOKEN`** (`.github/workflows/ci.yml`), so CI skips Q2's
  live test. That is the codesign identity test's precedent
  (`codesign/live_darwin_test.go`).
* **Q2's fixture is aqua's release.** No durable release from this
  module's publish workflow exists: the rehearsal repositories of 0013,
  0014 and 0015 were deleted. Q2's live test therefore uses the probed
  release by default. `SELFUPDATE_GHATTEST_REPOSITORY`,
  `SELFUPDATE_GHATTEST_TAG` and `SELFUPDATE_GHATTEST_ASSET` point it at a
  release from this module's workflow once one exists.

## Release procedure

The same for `v1.11.1` (P3) and `v1.12.0` (Q3); `vX` is the release.

1. **CI** on `main` at the release commit is green: all jobs of `ci.yml`.
2. **The track's Live tests** pass, recorded.
3. **The tag,** on the owner's ask in that turn:
   * `scripts/check-release-tag.sh vX` exits 0;
   * `git tag -a vX -m vX <commit>`;
   * the disclosure guard runs over the outgoing tag:

     ```bash
     echo "refs/tags/vX $(git rev-parse vX) refs/tags/vX 0000000000000000000000000000000000000000" |
       python3 ~/.global-git-hooks/github-disclosure.py pre-push origin "$(git remote get-url origin)"
     ```

   * `git push origin vX`.
4. **The tag's state.** `git ls-remote origin 'refs/tags/vX^{}'` gives the
   commit. The tag's CI run passes all jobs, and its identity legs print
   `vX (release) <12-hex>`.
5. **The pin commit,** staged by the agent:
   * `README.md`: the status, the `go get` line, and the publish
     example's pin;
   * `docs/architecture.md`: the current release and its commit;
   * the building guide: both workflow pins in step 4, the extras
     example's pin, and the `ls-remote` example;
   * the migration guide: every `go get` line and `go list` check outside
     a version section's own; §2's current release; §3's pin; §5's
     `go.mod` step.

   **How the lines are found.** The pin is `<40-hex> # vX`. The candidates
   are listed with `git grep -n '<previous>\|<previous 7-hex>' --
   README.md docs/architecture.md docs/guides`:
   * for `v1.11.1`: `'v1\.11\.0\|a0612da'`;
   * for `v1.12.0`: `'v1\.11\.1\|<v1.11.1's 7-hex>'`.

   Each changed line is named in the record.
6. **Proxy.** `GOPROXY=https://proxy.golang.org GOFLAGS=-mod=mod go list
   -m -json github.com/maccavelli/go-selfupdate-lib@vX` gives `vX`, with
   `Origin.Hash` the commit, and `@latest` resolves to `vX`.
7. **A live installer rehearsal,** only on the owner's ask. The
   throwaway repository must use tags that no earlier rehearsal
   published (0015-PLAN Deviation D9), for example `v0.2.1` to
   `v0.2.3`.

## Implementation Steps

### Phase R0: records

**Files:**
* `docs/decisions/0017-MADR-verify-build-provenance-and-close-0015-open-items.md`
* `docs/decisions/0017-PLAN-verify-build-provenance-and-close-0015-open-items.md`
* `docs/README.md`

1. **The MADR** gains `## Amendments`, before `## More Information`, with
   `### A1 (<approval date>): decisions and corrections from planning`:
   * N1–N14, as rows;
   * Corrections 1–6;
   * its "Not verified" first item is answered by Correction 3.
2. **This PLAN:** front matter `status: in-progress`; the Execution
   Record gains `### Approval (<date>)`, quoting the owner's approval.
3. **`docs/README.md`:** the 0017 PLAN row reads `in-progress`.
4. **Checks:** `scripts/check-docs.sh --links` and `--ids` on the three
   files, and markdownlint. No Go file changes, so `make gate` need not
   run (bootstrap exception).

### Phase D1: documentation on `main`

**Files:**
* `docs/architecture.md`
* `docs/guides/building-releases.md`
* `docs/decisions/0004-MADR-evolve-selfupdate-api-and-tui-support.md`
* `docs/README.md`

1. **`docs/architecture.md:373-375`** becomes:

   > The job runs in the concurrency group
   > `go-selfupdate-lib-publish-<repository>`, never cancelling a run in
   > progress. GitHub keeps one pending run per group: a third tag that
   > arrives while one publish runs and another waits cancels the waiting
   > one, whose tag then has no release until its publish job is re-run
   > (0017-MADR item 5). `queue: max` would keep it, once actionlint
   > accepts the key.

2. **`docs/guides/building-releases.md:195-198`,** the "One publish at a
   time" bullet, says the same in the guide's voice:
   * a third tag cancels the pending second;
   * re-run that tag's cancelled publish job from the Actions page;
   * GitHub creates no tag events at all for a push of more than three
     tags, so push tags one at a time or at most three together.
3. **The same guide, "Before the first release"** (after `:209`), gains
   a third bullet:
   * **Restrict who can create `v*` tags.** Use a tag ruleset that allows
     only the owner, or the release managers, to create tags matching
     `v*` (Settings → Rules → Rulesets → New tag ruleset → target
     `v*`, restrict creations).
   * **Why:** the publish workflow attests whatever a `v*` tag builds. A
     check of that attestation (`selfupdate/verify/ghattest`, from
     `v1.12.0`, or `--verify-attestation`) stops a release made with a
     stolen token outside the workflow, but not a malicious tag pushed
     through it.
4. **0004-MADR P3's open-work table:**
   * row `:1147` (ghattest) becomes `selfupdate/verify/ghattest`'s exec
     verifier | [0017-MADR] track 3, `v1.12.0`; its `sigstore-go`
     variant | 0017-MADR, "Not decided here";
   * row `:1155` becomes three rows:
     * unreferenced zip entries and names Windows reserves | 0017-MADR
       track 2, `v1.11.1`;
     * a crash between `Apply` and `Commit` | 0017-MADR track 3,
       `v1.12.0`;
     * `queue: max` | 0017-MADR item 5: waits for an actionlint release
       that accepts `queue`, after `v1.7.12`.

   0015-PLAN's citation gains `:57-68`.
5. **`docs/README.md:59`:** "§6 to §10" becomes "§6 to §11", with the
   same link.
6. **Checks:** rule 5's documentation checks, and `make gate`, which
   covers links and identifiers.

### Phase P1: zip entries tile the file (2B)

**Files:**
* `selfupdate/archive/unpack.go`
* `selfupdate/archive/unpack_test.go`
* `selfupdate/archive/main_test.go`
* `selfupdate/archive/fuzz_test.go`
* `selfupdate/archive/tiling_test.go` (new)
* `selfupdate/archive/doc.go`

1. **Fix, `unpack.go`.** Add the three functions of the 2026-10-08
   prototype (scratch `p17a/proto/tiling.go`), with N4 added:
   * **Constants:** `eocdLen = 22`, `zip64LocLen = 20`,
     `zip64EndMinLen = 56`, `eocdSearch = 65 * 1024`, each commented with
     its APPNOTE section.
   * **`directoryBounds(ra io.ReaderAt, size int64) (cdOff, cdSize int64,
     err error)`.**
     * It finds the last `PK\x05\x06` in the last `eocdSearch` bytes, as
       `archive/zip`'s `readDirectoryEnd` does, using `bytes.LastIndex`.
     * It refuses "data follows the end of the central directory" unless
       the comment ends at `size`.
     * When `PK\x06\x07` is at `eocd-20`: the zip64 record must start
       with `PK\x06\x06`, end at the locator, and agree with each EOCD
       field not saturated (N3). It refuses "the zip64 end record
       disagrees with the end record", "… does not end at its locator",
       or "the zip64 locator is not valid".
     * It requires `cdOff + cdSize == end`, else "the central directory
       does not end where its end record begins (prepended or trailing
       data)".
   * **`descriptorLen(ra io.ReaderAt, zf *zip.File, off, limit int64)
     (int64, error)`.**
     * It tries the four readings (N2) within `[off, limit)`.
     * One match returns its length. None is "entry %q: its data
       descriptor does not match its central record"; two is "entry %q:
       its data descriptor reads two ways".
   * **`checkTiling(ra io.ReaderAt, size int64, zr *zip.Reader, locals
     []local) error`.**
     * **N4:** the sum of `46 + len(zf.Name) + len(zf.Extra) +
       len(zf.Comment)` over `zr.File` must equal `cdSize`, else "the
       central records do not fill the central directory".
     * **Sorting:** `locals` is sorted by `hdr`. A header before the
       running end is "two entries overlap"; one after it is
       "unreferenced bytes [%d, %d) before the central directory".
     * **N1:** the local flag bit 0x8 must equal `zf.Flags&0x8`, else
       "entry %q: its local header and central record disagree on a
       data descriptor".
     * **Data:** data past `cdOff` is "entry %q runs into the central
       directory".
     * **The descriptor** follows the data when 0x8 is set.
     * **The end:** the running end must equal `cdOff`.
   * **The call:** in `extractZip`, after the `checkLocal` loop (`:487-491`)
     and before `program == nil`:

     ```go
     if err := checkTiling(ra, size, zr, locals); err != nil {
     	return err
     }
     ```

     The existing data-overlap check (`:481-486`) still fires first, so
     its row keeps its message.

   Every refusal uses `refuse`, so it wraps `selfupdate.ErrIntegrity`.
   No import is added.
2. **Test helpers, `main_test.go`,** all taking `testing.TB` so they can
   seed the fuzzer:
   * `hiddenGapZip(tb)`: `zipBytes` of `zfile(prog, hostProgram)`, then a
     second local `prog` entry inserted before the central directory,
     with the EOCD's offset moved;
   * `hiddenPrefixZip(tb)`: the same entry prepended;
   * `trailingZip(tb)`: 16 bytes after the EOCD;
   * `badDescriptorZip(tb)`: README's descriptor CRC changed;
   * `twoWayDescriptorZip(tb)`: an empty entry whose 16-byte signed
     descriptor is followed by 8 zero bytes, then the next header;
   * `clearedFlagZip(tb)`: the local 0x8 cleared;
   * `zip64DisagreeZip(tb)`: a zip64 record and locator whose record
     count disagrees with the EOCD's;
   * `slackZip(tb)`: 4 bytes of slack inside the central directory, with
     `cdSize` increased.

   Each is the prototype's byte surgery, against offsets read from the
   buffer, never hard-coded.
3. **Tests.**
   * **`TestUnpackRefuses` gains eight rows,** each on `a.zip` (the
     second column is the `want` substring):

     | Row | Want |
     | :--- | :--- |
     | `zip hidden entry in a gap` | `unreferenced bytes` |
     | `zip prepended entry` | `does not end where its end record begins` |
     | `zip trailing bytes` | `data follows the end of the central directory` |
     | `zip descriptor CRC` | `does not match its central record` |
     | `zip descriptor two ways` | `reads two ways` |
     | `zip descriptor flag` | `disagree on a data descriptor` |
     | `zip64 record disagrees` | `disagrees with the end record` |
     | `zip central directory slack` | `do not fill the central directory` |

   * **`TestUnpackAccepts` gains:**
     * `zip with a comment`;
     * `zip stored without a descriptor` (`raw` entries, `CreateRaw`);
     * `zip with a directory`;
     * `zip written by Info-ZIP` (bytes from `zip -X`, kept as a Go
       string constant, generated once by the step and recorded).
   * **`tiling_test.go`** (package `archive`):
     * `TestDirectoryBoundsZip64` writes 70,000 empty entries with
       `zip.Writer`, which forces zip64, and requires `directoryBounds`
       to accept it. It calls the function directly, since 70,000
       entries exceed `MaxEntries`.
     * `TestDescriptorLenWidths` covers 12, 16, 20 and 24 bytes.
   * **`fuzz_test.go`:** `FuzzUnpackZip` gains `f.Add(hiddenGapZip(f))`
     and `f.Add(hiddenPrefixZip(f))`.
4. **Red.**
   * On `v1.11.0`'s `unpack.go`, the eight `TestUnpackRefuses` rows fail
     with "Unpack = <nil>". The prototype's red run saw this for the gap,
     prefix, trailing, CRC and zip64 cases.
   * `tiling_test.go` fails to build, since `directoryBounds` is
     undefined.
   * The accept rows are pins.
5. **Plants,** in `selfupdate/archive/unpack.go`:

   | OLD | NEW | Fails |
   | :--- | :--- | :--- |
   | `'if err := checkTiling(ra, size, zr, locals); err != nil {'` | `'if err := checkTiling(ra, size, zr, locals); false && err != nil {'` | every new refusal row |
   | `'if l.hdr > at {'` | `'if false && l.hdr > at {'` | `zip hidden entry in a gap` |
   | `'if found != 0 {'` | `'if false {'` | `zip descriptor two ways` |
   | `'if localDD != (l.zf.Flags&0x8 != 0) {'` | `'if false {'` | `zip descriptor flag` |
   | the N4 comparison `!=` | `==` | `zip central directory slack` (and every accept row) |

   *Deviation D1 (2026-10-08): the second plant survived against `zip
   hidden entry in a gap`. A row, `zip hidden entry between entries`, was
   added for the in-loop check, and a plant on the end check (`if at !=
   cdOff {`) runs against the gap row. The Info-ZIP case runs in
   `tiling_test.go`, as `TestOtherWritersTile`, not in
   `TestUnpackAccepts`.*

6. **Docs:** `archive/doc.go:11-19` adds:
   * zip local entries that do not tile the file: a gap, prepended or
     trailing data, or a data descriptor that disagrees with its central
     record;
   * a citation of 0017-MADR 2B.
7. **Run:** `go test -count=1 ./selfupdate/archive/ ./internal/cmd/selfupdate-release/ ./selfupdate/releasespec/`.
   pack's round trip unpacks every built archive, so the second package
   proves that pack's output still passes.
8. **Fuzz:** `go test -run '^$' -fuzz '^FuzzUnpackZip$' -fuzztime 60s
   ./selfupdate/archive/` finds no failure; the gate's fuzz step also runs.

### Phase P2: names Windows reserves, on every host (4C)

**Files:**
* `selfupdate/archive/unpack.go`
* `selfupdate/archive/unpack_test.go`
* `selfupdate/archive/names_test.go` (new)
* `selfupdate/archive/doc.go`

1. **Fix, `unpack.go`:**
   * **`windowsFolded`:**

     ```go
     // windowsFolded are the printable ASCII characters Win32 refuses in a
     // name, and that Windows extractors (tar.exe, .NET) rewrite to "_".
     const windowsFolded = `<>:"|?*`
     ```

   * **`windowsReserved(el string) bool`:**
     * the stem is `strings.Cut(el, ".")`'s first part, with trailing
       spaces trimmed (N5);
     * it is true for CON, PRN, AUX, NUL, CONIN$ and CONOUT$
       (case-insensitive);
     * and for `COM` or `LPT` followed by one ASCII digit, 0–9.
   * **In `checkPortable`'s element loop (`:288-292`),** after the
     dot/space check:

     ```go
     if strings.ContainsAny(el, windowsFolded) || windowsReserved(el) {
     	return refuse("entry name %q has an element Windows reserves", name)
     }
     ```

     Tar and zip both pass through `entries.add` (`:214-236`), so both
     formats get the rule.
   * **`NewUnpacker`'s Member check (`:68`) (N6):**

     ```go
     if o.Member != "" && (!fs.ValidPath(o.Member) || checkPortable(o.Member, o.Member) != nil) {
     ```

     This keeps its plain error, `"selfupdate: archive: invalid member
     path %q"`.
   * **Comments:** the `checkPortable` comment (`:277-281`) and the
     `UnpackOptions.Member` comment (`:48-51`) name the rule and 0017-MADR
     4C.
2. **Tests.**
   * **`TestUnpackRefuses` gains:**
     * `zip colon beside the program`: `zfile("my:tool", small)`;
     * `tar aux.txt`;
     * `tar x?y`;
     * `zip com1.log in a folder`: `zfile("logs/com1.log", small)`;
     * `tar CONOUT$`.

     Each beside `prog`, wanting "an element Windows reserves".
   * **`TestNewUnpackerRefuses`** gains Members `a:b`, `bin/aux.txt`,
     `x?y` and `relay.` (E2, through N6).
   * **`TestUnpackAccepts` gains** `names Windows allows`: `relay_1.2.3`,
     `a b/readme`, `auxiliary`, `com10`, beside `prog`. This is a pin.
   * **`names_test.go`:** `TestWindowsReserved` is the prototype's table.
     * **Reserved:** `aux`, `AUX`, `aux.txt`, `AUX.tar.gz`, `con`, `prn`,
       `nul`, `NUL.txt`, `com0`, `com1`, `COM9`, `com1.tar.gz`, `lpt0`,
       `LPT9.log`, `conin$`, `CONOUT$`, `conout$.x`, `aux .txt`.
     * **Not reserved:** `com10`, `lpt`, `com`, `comx`, `auxiliary`,
       `aux_`, `xaux`, `con1`, `conin`, `relay`, `relay.exe`, `""`,
       `.aux`.
3. **Red.**
   * On P1's code, the five refusal rows fail with "Unpack = <nil>" on
     Linux and macOS. On Windows, Go's `IsLocal` already refuses most of
     them, which is the differential.
   * The four Members fail with "accepted".
   * `names_test.go` fails to build.
4. **Plants,** in `unpack.go`:

   | OLD | NEW | Fails |
   | :--- | :--- | :--- |
   | `'if strings.ContainsAny(el, windowsFolded) \|\| windowsReserved(el) {'` | `'if false {'` | the five rows |
   | `'case "CON", "PRN", "AUX", "NUL", "CONIN$", "CONOUT$":'` | `'case "CON", "PRN", "NUL", "CONIN$", "CONOUT$":'` | `tar aux.txt`, `TestWindowsReserved` |
   | `' \|\| checkPortable(o.Member, o.Member) != nil'` | `''` | the four Members |
   | `"'0' <= stem[3] && stem[3] <= '9'"` | `"'1' <= stem[3] && stem[3] <= '9'"` | `TestWindowsReserved` (`com0`, `lpt0`) |

5. **Docs:** `archive/doc.go` adds the characters `< > : " | ? *` and
   the device names, citing 0017-MADR 4C.
6. **Run:** as in P1.

### Phase P3: the `v1.11.1` release

**Files** (the release commit):
* `docs/decisions/0012-MADR-archive-assets-and-macos-codesign.md`
* `docs/guides/migrating-from-mcplib-selfupdate.md`
* `docs/guides/extending-selfupdate.md`
* `docs/decisions/0017-PLAN-verify-build-provenance-and-close-0015-open-items.md`

1. **0012-MADR** gains `### A3 (<date>): no unreferenced bytes, and no
   name Windows reserves`, before `## More Information` (`:929`). It
   follows A1's shape (`:895`): an italic status line naming 0017-MADR
   2B and 4C, this PLAN's P1 and P2, and `v1.11.1`, then the two rules
   as bullets.
2. **The migration guide:**
   * a new `### From v1.11.0 to v1.11.1` inside §11, before its
     `### Check` (`:770`), in the shape of `### From v1.10.0 to v1.10.1`
     (`:646-692`):
     * the `go get …@v1.11.1` line;
     * "changes no API: `make apicheck` reports it compatible with
       `v1.11.0`";
     * **Behaviour changes:** the two refusals;
     * **Check:** gives `v1.11.1`.
   * the intro at `:26-35` adds a sentence: `v1.11.1` changes no API, and
     its fixes are in a link to `#from-v1110-to-v1111`.
3. **The extending guide's "Ship an archive" list** (`:227-246`) gains a
   "Since `v1.11.1`" bullet with the two rules.
4. **This PLAN** gets the release notes for `v1.11.1`, and the phase
   record.
5. **Then the Release procedure,** as `vX = v1.11.1`, with the pin commit
   as its own commit.

### Phase Q1: the interrupted-update journal and `KeptBackups` (3B)

**Files:**
* `selfupdate/journal.go` (new)
* `selfupdate/journal_windows.go` (new)
* `selfupdate/journal_other.go` (new)
* `selfupdate/filedigest.go` (new)
* `selfupdate/session.go`
* `selfupdate/replace_unix.go`
* `selfupdate/replace_windows.go`
* `selfupdate/cleanup_windows.go`
* `selfupdate/leftovers.go`
* `selfupdate/standalone.go`
* `selfupdate/managed.go`
* `selfupdate/types.go`
* `selfupdate/doc.go`
* `selfupdate/journal_test.go` (new)
* `selfupdate/keptbackups_test.go` (new)
* `selfupdate/journal_windows_test.go` (new)
* `selfupdate/modepolicy_test.go`
* `selfupdate/twophase_test.go`
* `selfupdate/recovery_test.go`
* `selfupdate/install_hardening_test.go`
* `selfupdate/e2e_running_test.go`

1. **`filedigest.go`:** move `fileSHA256` and `rootFileSHA256` out of
   `cleanup_windows.go` (`:87-124`), unchanged, so every OS has them.
   `cleanup_windows.go` drops its copies.
2. **`journal.go`** (portable):
   * **The record:**

     ```go
     // pendingJournal is .<base>.selfupdate.pending: written before an
     // update's rename and removed once it is committed or rolled back,
     // so a session that finds it knows an update was interrupted
     // (docs/decisions/0017-MADR-verify-build-provenance-and-close-0015-open-items.md 3B).
     // Every schema keeps Backup, so an older reader can still keep it.
     type pendingJournal struct {
     	Schema    int    `json:"schema"`
     	Backup    string `json:"backup"`
     	OldDigest string `json:"old_digest"`
     	NewDigest string `json:"new_digest"`
     	Phase     string `json:"phase"`
     	Product   string `json:"product,omitempty"`
     }
     const (
     	journalSchema   = 1
     	journalApplying = "applying"
     )
     ```

   * **The names:** `journalName(base)` is `"." + base +
     ".selfupdate.pending"`; `journalTmpPrefix(base)` is `"." + base +
     ".selfupdate.pending-tmp-"`.
   * **`writeJournal(root *os.Root, target Target, j pendingJournal) error`**,
     through `root` throughout:
     1. `randomSibling(target.Dir, journalTmpPrefix)`;
     2. `root.OpenFile(tmp, O_WRONLY|O_CREATE|O_EXCL, 0o600)`;
     3. write `json.Marshal(j)`, `Sync`, `Close`;
     4. `restrictJournal(filepath.Join(target.Dir, tmp))`;
     5. `root.Rename(tmp, journalName)`;
     6. `syncRootFn(root)`, whose error is returned.

     Any failure removes `tmp`. A package seam, `writeJournalFn =
     writeJournal`, lets tests fail it.
   * **`restrictJournal(path string) error`** is per OS:
     * `journal_windows.go` (`//go:build windows`) calls
       `restrictToCurrentUser` (`cleanup_windows.go:293`), which exists
       only on Windows;
     * `journal_other.go` (`//go:build !windows`) returns nil, since the
       0600 mode already restricts it.
   * **`keptRenameFn = (*os.Root).Rename`** is the seam row 8's rename
     goes through, so a test can fail it.
   * **`removeJournal(root, target)`** is advisory: `root.Remove`, then
     `syncRootFn`.
   * **`recoverJournal(target Target, root *os.Root, keep map[string]bool,
     sweep bool) (map[string]bool, bool)`** returns no error, so
     `CleanupPending` keeps its contract. The rules are applied in this
     order, and the first that holds decides:

     | # | Found | Action |
     | :--- | :--- | :--- |
     | 1 | no journal | nothing |
     | 2 | the journal is not a regular file, or is a reparse point (Windows) | leave it; `sweep = false` |
     | 3 | it does not parse; `Backup` fails `validateReceiptBackup` or has no `keptName`; or `Schema < 1` | leave it; `sweep = false` |
     | 4 | the backup is absent | remove the journal |
     | 5 | the backup is in `keep` (a Windows cleanup receipt lists it) | remove the journal |
     | 6 | the backup is not a regular file | leave the journal; `sweep = false` |
     | 7 | `os.SameFile(backup, target)`, or the target's SHA-256 equals `OldDigest` | the backup is redundant: remove the journal, and let the sweep remove the backup |
     | 8 | otherwise (the new binary, or a third, is live) | keep the backup: rename it to `keptName`, or, when that name is taken, to a fresh `.<base>.selfupdate-kept-<digits>` from `randomSibling`; clear its special bits as `retainLocked` does; sync; remove the journal. If the rename fails, add the backup to `keep` and leave the journal |

     A schema above 1, with a valid `Backup`, follows rows 4–8 (N8). The
     target's digest is computed only in row 7, and only when `SameFile`
     is false.
3. **`replace_unix.go` and `replace_windows.go`:** `replaceTarget` gains
   a final parameter, `beforeRename func(backup, oldDigest string) error`
   (N7).
   * **Unix:** compute `oldDigest, err := fileSHA256(target.Path)` before
     `randomSibling`, as Windows already does (`:38`).
   * **Both:** after `backupFile` succeeds, call `beforeRename(backup,
     oldDigest)` when it is not nil. On error, return `applyResult{},
     joinRemove(fmt.Errorf("selfupdate: write the journal: %w", err),
     backup)`. The target is untouched.
4. **`session.go`:**
   * `installSession` gains `journal bool`.
   * **`replaceLocked(ctx, path)` becomes `replaceLocked(ctx, req
     InstallRequest, product string)`:**
     * it refuses with `fmt.Errorf("selfupdate: an earlier update's
       journal %s is pending; resolve it, then retry", path)` when
       `root.Lstat(journalName)` finds one (N9);
     * its `beforeRename` writes `pendingJournal{journalSchema, base(backup),
       oldDigest, newDigest, journalApplying, product}`;
     * `newDigest` is `req.Artifact.InstalledDigest` when that is 64
       lower-case hex, and otherwise `fileSHA256(req.Artifact.Path)`;
     * `s.journal = true` on success.
   * **The callers:** `Install` passes `product = ""`, and `Apply` passes
     `req.Product`.
   * **`s.journal` is cleared** (`removeJournal` and `s.journal = false`)
     in exactly these places:
     * after `replaceLocked` returns an error with nothing renamed;
     * after `errRolledBack` from the replace or the probe;
     * after `rollbackInRoot` succeeds or reports `errRestoredUnsynced`;
     * after `commitLocked` runs, whatever its result;
     * after a successful `Rollback`;
     * after `retainLocked` returns a path other than its input.

     `Close` never removes the journal.
   * **The comments:** `previousPath` (`:35-37`) and `retainLocked`
     (`:405-409`) name the journal.
   * **`beginSession`,** inside `!isDryRun` (`:538-548`):

     ```go
     keep, sweepBackups := listedBackups(original)
     keep, sweepBackups = recoverJournal(original, root, keep, sweepBackups)
     removeLeftovers(original, root, keep, sweepBackups)
     ```

5. **`leftovers.go`:**
   * `isLeftover` (`:24-43`) also matches `journalTmpPrefix(base)` +
     digits, as it matches `cleanup-tmp-`. The journal itself
     (`.selfupdate.pending`) does not match `".selfupdate-"`, so it is
     never swept.
   * The comments at `:8-14` and `:45-49` say a kept backup may also come
     from an interrupted update.
6. **`KeptBackups`** (N10):

   ```go
   // KeptBackup is a copy of a previous binary that an update left beside
   // the target and that no session removes: one whose restore failed, or
   // one an interrupted update left (0017-MADR 3B). Restore or remove it.
   type KeptBackup struct {
   	Path    string    `json:"path"`
   	Size    int64     `json:"size"`
   	ModTime time.Time `json:"mod_time"`
   }
   ```

   * **`(*StandaloneInstaller).KeptBackups(ctx) ([]KeptBackup, error)`,**
     in `standalone.go`:
     * it checks `ctx.Err()`, then `resolveTarget(s.policy)`, then
       `os.OpenRoot(target.Dir)` and `Readdirnames`;
     * it keeps the names exactly `.<base>.selfupdate-kept-<digits>` that
       `root.Lstat` reports regular, skipping reparse points on Windows;
     * it returns them sorted by name, with absolute paths; none is
       `nil, nil`;
     * it takes no lock, so it never fails with `ErrConcurrentUpdate`.
   * **`(*ManagedInstaller).KeptBackups(ctx)`,** in `managed.go`,
     delegates through `interface{ KeptBackups(context.Context)
     ([]KeptBackup, error) }` on `m.inner`. Otherwise it returns
     `fmt.Errorf("selfupdate: the inner installer does not list kept
     backups")`.
7. **Docs in code:**
   * `standalone.go:51-57` (`CleanupPending`): an interrupted update's
     backup is kept, and `KeptBackups` lists it;
   * `types.go:156-164` (`Result.PendingBackup`) and `:467-471`
     (`InstallResult.Backup`): point to `KeptBackups`;
   * `doc.go:121-124` (Installers): the journal, and `KeptBackups`.
8. **Tests.**
   * **`journal_test.go`:**
     * **`TestInterruptedApplyKeepsBackup`:** `standaloneSession`, then
       `Apply` (no Commit), then `Close`. Its subtests:
       * `CleanupPending`: `CleanupPending` returns nil;
         `.demo.selfupdate-kept-<n>` holds `old-bytes`; the target holds
         `new-bytes`; no journal; `KeptBackups` names the kept file.
       * `Run`: the next update run keeps it too.
       * `Managed`: the same through `NewManagedInstaller`.
       * `DryRun`: a dry run leaves the journal and the backup.

       This mirrors `TestKeptBackupSurvivesLaterSessions`
       (`recovery_test.go:292`).
     * **`TestJournalDuringInstall`:** a `postInstall` prober reads the
       journal while the probe runs. It holds schema 1, phase
       `applying`, the backup's name, `old_digest` equal to
       SHA-256(`old-bytes`), `new_digest` equal to SHA-256(`new-bytes`),
       and an empty product. After `Install`, no journal remains.
     * **`TestJournalDuringApply`:** the same through `Apply`, with
       `product` `demo`.
     * **`TestJournalRecoveryRules`:** one row per row of step 2's table.
       Each plants the files by hand under `withTempHome`, runs
       `CleanupPending`, and checks the files after. Row 8 has two
       cases: the kept name free, and taken (the earlier kept file
       untouched, a fresh one made). The rename-failure case sets the
       `keptRenameFn` seam to fail, and expects the journal left, the
       backup in place, and no error.
     * **`TestJournalWriteFailure`:** with `writeJournalFn` failing,
       `Install` returns the error. The target holds `old-bytes`, and no
       backup or journal remains.
     * **`TestApplyRefusesPendingJournal`:** a planted malformed journal
       makes `Apply` fail with "is pending" (N9).
   * **`keptbackups_test.go`:** `TestKeptBackups` lists only regular
     `-kept-<digits>` files, sorted. It ignores:
     * a symlink, and a directory;
     * `.demo.previous`, the journal, and `-kept-x`.

     It also runs while another session holds the lock, and returns
     `nil, nil` for none. **`TestManagedKeptBackups`** covers delegation,
     and the error for an inner installer without the method.
   * **`journal_windows_test.go`** (`//go:build windows`):
     `TestWindowsInterruptedApplyRunningImage`. The old binary runs as
     the native helper (`TestWindowsKeepPreviousRunningPrevious`'s
     pattern, `cleanup_busy_windows_test.go:190`). The test runs `Apply`,
     then `Close`, then `CleanupPending`; the backup is renamed to its
     kept name while the image runs.
   * **`TestIsLeftover`** (`modepolicy_test.go:34`) gains:
     * `.demo.selfupdate.pending-tmp-123`: true;
     * `.demo.selfupdate.pending`: false;
     * `.demo.selfupdate.pending-tmp-x`: false.
   * **Journal-absent assertions** are added after success in
     `TestStandaloneApplyCommit` and `TestSecondCommitOrRollbackRefused`,
     and after the rollback in
     `TestInstallRollsBackWhenDirectoryMovesAfterRename`, in the moved
     directory. They are added to each case of
     `TestInstallInjectedFailures`, and to the e2e test's `leftovers`
     (`e2e_running_test.go:266`), with the name written out, since that
     is the external package. The existing helpers filter on
     `".demo.selfupdate-"` and would not see it.
9. **Red.**
   * `TestInterruptedApplyKeepsBackup/CleanupPending` fails on `v1.11.1`
     with its own message, "want one .demo.selfupdate-kept-<n> holding
     old-bytes, found none", because the sweep removed the backup. The
     test is written first without `KeptBackups`, so that it builds on
     `v1.11.1`; the `KeptBackups` assertion is added with the fix.
   * The new files fail to build, since `KeptBackups` and the journal
     are undefined.
   * The `TestIsLeftover` rows fail.
   * The journal-absent assertions pass as pins.
10. **Plants:**

    | File | OLD | NEW | Fails |
    | :--- | :--- | :--- | :--- |
    | `session.go` | `'keep, sweepBackups = recoverJournal(original, root, keep, sweepBackups)'` | `''` | `TestInterruptedApplyKeepsBackup` |
    | `journal.go` | the `root.Rename(tmp, journalName…)` line | the same line commented out | `TestJournalDuringInstall` |
    | `standalone.go` | the digits check in `KeptBackups` | a check that accepts any suffix | `TestKeptBackups` (`-kept-x`) |
    | `journal.go` | the taken-name `Lstat` check in row 8 | `if false {` | `TestJournalRecoveryRules/taken` |
    | `session.go` | the N9 `Lstat` refusal | `if false {` | `TestApplyRefusesPendingJournal` |

11. **API** (rule 6): `KeptBackup` and its three fields;
    `(*StandaloneInstaller).KeptBackups`; `(*ManagedInstaller).KeptBackups`.
12. **Run:** `go test -count=1 ./selfupdate/`. The Windows test runs on
    CI and on the Windows test host (Live tests).

### Phase Q2: `selfupdate/verify/ghattest` (1B)

**Files:**
* `selfupdate/verify/ghattest/doc.go`
* `selfupdate/verify/ghattest/ghattest.go`
* `selfupdate/verify/ghattest/ghattest_test.go`
* `selfupdate/verify/ghattest/example_test.go`
* `selfupdate/verify/ghattest/live_test.go`
* `selfupdate/verify/ghattest/testdata/verify-aqua-v2.64.0.json`
* `.golangci.yml`
* `AGENTS.md`

1. **The API, `ghattest.go`:**

   ```go
   // PublishWorkflow is this module's publish workflow, the signer of every
   // release it publishes (0013-MADR :210-215).
   const PublishWorkflow = "maccavelli/go-selfupdate-lib/.github/workflows/publish-selfupdate-release.yml"

   // PredicateType and Issuer are what every check requires.
   const (
   	PredicateType = "https://slsa.dev/provenance/v1"
   	Issuer        = "https://token.actions.githubusercontent.com"
   )

   // Policy is what an attestation must say. A sigstore-go verifier (0017-MADR
   // 1E) takes the same Policy.
   type Policy struct {
   	// Repository is the program's own repository, which the attestation's
   	// source must be. Required.
   	Repository selfupdate.Repository
   	// SignerWorkflow is owner/repo/path of the workflow that signed.
   	// Empty means PublishWorkflow.
   	SignerWorkflow string
   	// SignerDigests, when set, are the signer workflow's commits accepted:
   	// 40 lower-case hex each.
   	SignerDigests []string
   	// AllowSelfHostedRunners accepts an attestation made on a
   	// self-hosted runner.
   	AllowSelfHostedRunners bool
   }

   // Options configure a verifier. GH is required.
   type Options struct {
   	Policy Policy
   	// GH is gh's absolute path; nothing is looked up on PATH.
   	GH string
   	// Env is gh's environment; nil means this process's (N13).
   	Env []string
   	// Runner runs gh. Nil means service.ExecRunner.
   	Runner service.Runner
   	// Timeout bounds one gh run. Zero means 2 minutes.
   	Timeout time.Duration
   	// TempDir holds the file gh reads. Empty means os.TempDir.
   	TempDir string
   }

   func NewManifestVerifier(o Options) (selfupdate.ManifestVerifier, error)
   func NewVerifier(o Options) (selfupdate.Verifier, error)
   ```

2. **Construction refuses:**
   * a `Repository` whose owner or name is empty, contains `/ \ : ?`, a
     control byte or a space, is `.` or `..`, or starts with `-`;
   * a `SignerWorkflow` not of the form
     `<owner>/<repo>/.github/workflows/<file>.yml` or `.yaml`, or one
     with a space, a control byte or a leading `-`;
   * a `SignerDigests` entry that is not 40 lower-case hex;
   * a `GH` that is not `filepath.IsAbs`. That is not `path.IsAbs`, so a
     Windows `gh` is allowed;
   * a negative `Timeout`.
3. **Each check:**
   1. **The tag:** `Release.Tag` must be non-empty, printable ASCII
      without a space, and must not start with `-`. The ref is
      `refs/tags/<tag>`.
   2. **The file.** The manifest verifier writes `ManifestVerification.Manifest`.
      The asset verifier copies `Verification.Open()`, at most
      `Verification.Size` bytes, hashing as it copies, and refuses before
      running `gh` unless the hash is `Verification.SHA256`. The file is
      `os.CreateTemp(TempDir, "selfupdate-ghattest-*")` (0600 on Unix),
      closed before `gh` runs and removed by a `defer`.
   3. **The arguments, in this order:**

      ```text
      attestation verify <file> --hostname github.com
        --repo <owner>/<name> --signer-workflow <SignerWorkflow>
        --source-ref refs/tags/<tag> --predicate-type <PredicateType>
        --cert-oidc-issuer <Issuer> --format json
        [--deny-self-hosted-runners]        unless AllowSelfHostedRunners
        [--signer-digest <d>]               only when len(SignerDigests) == 1
      ```

   4. **The run:** `Runner.Run`, under `context.WithTimeout`, with
      `Command{Path: GH, Args, Env}` (N13).
   5. **The result:**
      * a Runner error is "selfupdate: ghattest: running gh: %w";
      * exit 4 is "selfupdate: ghattest: gh is not logged in (exit 4):
        <detail>";
      * any other non-zero exit is "selfupdate: ghattest: attestation
        verification failed (exit N): <detail>";
      * exit 0 decodes stdout as `[]struct{VerificationResult …}` (N11).
        Some entry must have:
        * `statement.predicateType == PredicateType`;
        * `certificate.issuer == Issuer`;
        * `certificate.sourceRepositoryURI` equal, ignoring case, to
          `https://github.com/<owner>/<name>`;
        * `certificate.sourceRepositoryRef == "refs/tags/<tag>"`;
        * `certificate.buildSignerURI` starting, ignoring case, with
          `https://github.com/<SignerWorkflow>@`;
        * `certificate.buildSignerDigest` in `SignerDigests`, when that
          is set;
        * `certificate.runnerEnvironment == "github-hosted"`, unless
          self-hosted runners are allowed;
        * a `statement.subject[].digest.sha256` equal to the file's
          SHA-256.

        Otherwise the error is "selfupdate: ghattest: gh reported
        success, but no attestation matches the policy: <first failing
        field>". Undecodable output is "selfupdate: ghattest: gh's output
        is not the expected JSON: %v".
   6. **`<detail>`** is codesign's `detail` (`codesign.go:105-122`):
      control characters folded, 1024 bytes at most. It is copied, since
      depguard gives the two packages no shared `internal/` helper.

   The updater joins every error with `ErrIntegrity`
   (`manifestverify.go:57-58`, `updater.go:496-516`).
4. **`doc.go`:**
   * the package comment: what the check proves, and what it does not
     (MADR item 1);
   * that it needs `gh` logged in, even for a public repository;
   * that it is opt-in, and fails closed;
   * that `gh` 2.102.0 was the version probed;
   * a pointer to the tag ruleset (D1).
5. **Depguard, `.golangci.yml`:**
   * a new rule after `selfupdate-releasespec` (`:176`):

     ```yaml
     # selfupdate/verify/ghattest: the standard library, selfupdate, and
     # selfupdate/service for its Runner
     # (docs/decisions/0017-MADR-verify-build-provenance-and-close-0015-open-items.md 1B).
     selfupdate-verify-ghattest:
       list-mode: strict
       files:
         - "**/selfupdate/verify/ghattest/*.go"
         - "!$test"
       allow:
         - $gostd
         - github.com/maccavelli/go-selfupdate-lib/selfupdate$
         - github.com/maccavelli/go-selfupdate-lib/selfupdate/service$
     ```

   * `other-packages` gains `- "!**/selfupdate/verify/ghattest/*.go"`
     after `:197`.

   The rule name sorts after `banned`.
6. **`AGENTS.md`:**
   * the scope paragraph (`:9-25`) adds "`selfupdate/verify/ghattest`, an
     opt-in provenance check
     (`docs/decisions/0017-MADR-verify-build-provenance-and-close-0015-open-items.md`)";
   * the sentence on directories (`:24`) allows a directory under
     `selfupdate/verify/`;
   * the depguard list (`:44-47`) adds `selfupdate-verify-ghattest`.
7. **Tests, `ghattest_test.go`** (package `ghattest`; the fake runner is
   codesign's `fakeTool`, copied, `codesign_test.go:17-43`):
   * **`TestNewRefuses`:** one row per refusal in step 2.
   * **`TestArguments`:** the exact argv for no digest, one digest, two
     digests (no flag) and self-hosted allowed; `Path` is `GH`; `Env`
     ends with N13's four variables, after this process's or the given
     environment.
   * **`TestExitCodes`:** exit 1 gives "verification failed (exit 1)";
     exit 4 gives "not logged in (exit 4)"; a Runner error gives "running
     gh"; a runner that sleeps past `Timeout: time.Millisecond` gives
     `context.DeadlineExceeded`. Each error carries the fake's stderr,
     bounded and without `\n`.
   * **`TestPolicyRecheck`:** exit 0 with
     `testdata/verify-aqua-v2.64.0.json`, the probe's output, against
     aqua's policy, passes. One mutated copy per field of step 3.5 is
     refused with that field's name. So are an empty array and garbage.
   * **`TestTagRefused`:** `""`, `-x`, `v1 2`, `v1\n`.
   * **`TestManifestFile`:** the fake runner reads the file named in
     argv. It holds exactly the manifest, has mode 0600 on Unix, and is
     gone after the check, in success and in failure.
   * **`TestVerifierChecksDigest`:** `Open` returns bytes whose SHA-256
     is not `Verification.SHA256`. The check is refused, and the fake
     runner is never called.
   * **`TestUpdaterRefusesUnattested`,** in package `ghattest_test`: an
     `Updater` on `selfupdatetest`'s fake GitHub server, with the
     manifest verifier and a fake runner exiting 1. `Run` fails, and
     `errors.Is(err, selfupdate.ErrIntegrity)` holds. No binary request
     reaches the server.
8. **`example_test.go`:** `ExampleNewManifestVerifier`, compile-only, as
   codesign's examples are.
9. **`live_test.go`:** `TestLiveVerify`, gated by
   `SELFUPDATE_REQUIRE_GHATTEST=1` (skip otherwise, as
   `codesign/live_darwin_test.go:31-37` does).
   * **`gh`** is `SELFUPDATE_GHATTEST_GH`, or `exec.LookPath("gh")` made
     absolute.
   * **The fixture** (Correction 3) is fetched over HTTPS from
     `https://github.com/aquaproj/aqua/releases/download/v2.64.0/aqua_2.64.0_checksums.txt`.
     Its SHA-256 must be
     `651d4378614e8506c5814d3bed0c5643eea74b957e491601427ad8ebf241fa02`,
     else the test fails, naming the changed fixture.
   * **The cases:**
     * the policy `aquaproj/aqua`, `suzuki-shunsuke/go-release-workflow/.github/workflows/release.yaml`,
       tag `v2.64.0` passes;
     * so does the same with `SignerDigests` of `cf03c29d…` alone, and of
       it plus a second digest (the JSON path, N11);
     * another digest alone is refused;
     * another signer workflow is refused (exit 1);
     * tag `v2.63.0` is refused (exit 1).
   * The three `SELFUPDATE_GHATTEST_*` variables (Live tests) replace the
     fixture when set.
10. **Red.**
    * The package does not exist on `v1.11.1`, so the tests fail to
      build. That is the red, as 0015's G3 recorded "the build failed on
      `RolledBack`".
    * `TestUpdaterRefusesUnattested` is the behavioural red. It is first
      run with `Config.ManifestVerifiers` empty, which is `v1.11.1`'s
      behaviour: the update succeeds, and the test fails with "Run =
      <nil>, want an ErrIntegrity". The verifier is then added.
11. **Plants,** in `ghattest.go`:

    | OLD | NEW | Fails |
    | :--- | :--- | :--- |
    | the `--source-ref` pair | removed | `TestArguments` |
    | the `sourceRepositoryRef` comparison | `true` | `TestPolicyRecheck/ref` |
    | `case 4:` | `case 99:` | `TestExitCodes/4` |
    | the asset digest check | `if false {` | `TestVerifierChecksDigest` |
    | the `defer os.Remove(` | removed | `TestManifestFile` |
    | the negative-`Timeout` check | `if false {` | `TestNewRefuses/negative-timeout` |
    | the `-` check on a repository owner | `if false {` | `TestNewRefuses/owner-dash` |

12. **API:**
    * package `ghattest`: `PublishWorkflow`, `PredicateType`, `Issuer`;
    * `Policy` and its four fields; `Options` and its six fields;
    * `NewManifestVerifier`, `NewVerifier`.
13. **Run:** `go test -count=1 ./selfupdate/verify/ghattest/`. Then the
    live test: on this Mac, `SELFUPDATE_REQUIRE_GHATTEST=1 go test
    -count=1 -v -run '^TestLive' ./selfupdate/verify/ghattest/`, and the
    same on the Windows test host.

### Phase Q3: the `v1.12.0` release

**Files** (the release commit):
* `docs/guides/migrating-from-mcplib-selfupdate.md`
* `docs/guides/extending-selfupdate.md`
* `docs/architecture.md`
* `README.md`
* `docs/README.md`
* `selfupdate/doc.go`
* `docs/decisions/0004-MADR-evolve-selfupdate-api-and-tui-support.md`
* `docs/decisions/0017-PLAN-verify-build-provenance-and-close-0015-open-items.md`

1. **The migration guide** gains `## 12. From v1.11 to v1.12`, in §11's
   shape (`:702-774`):
   * the `go get …@v1.12.0` line;
   * "`make apicheck` reports it compatible with `v1.11.1`: every
     exported change is an addition";
   * **What is new:** `ghattest`; `KeptBackups`; the journal;
   * **Adopting it:**
     * adding the manifest verifier: a Go snippet with `gh`'s path, the
       `Policy` and the environment;
     * calling `KeptBackups` after `CleanupPending`;
   * **Check.**

   The intro (`:26-35`) adds `v1.12.0` to the releases that only add.
2. **The extending guide:**
   * a new `## Check build provenance`, after "Verify a signature later"
     (`:262`), in "Sign on macOS"'s shape (`:296-340`): what it checks,
     the snippet, that it fails closed, what it does not stop (the tag
     ruleset), and "Pointers:" naming `ExampleNewManifestVerifier` and
     `TestLiveVerify` with its variable;
   * "Verify a signature later" points to it;
   * "Keep the previous binary" (`:350-365`): an interrupted update's
     backup is kept, and `KeptBackups` lists it.
3. **`docs/architecture.md`:**
   * the tree (`:76-81`) gains `verify/ghattest/`;
   * the package table (`:103-116`) gains its row;
   * the prose bullets (`:229-241`) gain a `ghattest` bullet;
   * the installers paragraph (`:142-145`, `:262-274`) names the journal
     and `KeptBackups`;
   * the import rules (`:418-430`) become 15, with
     `selfupdate-verify-ghattest`;
   * the live tests (`:462-469`) gain `TestLiveVerify`, not run in CI.
4. **`README.md`:** the package table (`:36`) gains `ghattest`.
   **`docs/README.md`:** "I want to…" gains "check that a release was
   built by my workflow", pointing to the extending guide's new section.
5. **`selfupdate/doc.go` Integrity (`:98-109`):** a sentence naming
   `selfupdate/verify/ghattest` as the opt-in provenance check on
   `SHA256SUMS`.
6. **0004-MADR P3:** the rows D1 wrote for `v1.11.1` and `v1.12.0` move
   to the list of done items (`:1157-1160`). The §1 as-built table
   (`:1131-1138`) gains `verify/ghattest`.
7. **This PLAN:** the release notes for `v1.12.0`.
8. **Then the Release procedure,** as `vX = v1.12.0`.

## Verification

* **V1.** Every new test failed first on the code before its phase, and
  every plant made its named test fail in a scratch copy. Each is
  recorded with its failing line.
* **V2.** The full apidiff report (0015-PLAN V3's command):
  * against `v1.11.0`, at `v1.11.1`, lists no change;
  * against `v1.11.1`, at `v1.12.0`, lists exactly Q1's and Q2's API
    steps, all compatible.
* **V3.** `git diff v1.11.0 -- go.mod go.sum` is empty, and
  `go mod tidy -diff` is clean.
* **V4.** `.golangci.yml` has 15 depguard rules. `make lint` is clean
  for linux, darwin and windows. A scratch copy in which `ghattest.go`
  imports `selfupdate/archive` fails lint with
  `selfupdate-verify-ghattest`.
* **V5.** `make gate` ended `overall=0` at every phase.
* **V6.** CI is green on `main` at each phase's commit, and on both tags.
* **V7.** The Live tests passed: Q1 on the Windows test host (30 runs,
  8.3 `TEMP`); Q2 on this Mac and on the Windows test host.
* **V8.** The hidden-gap and hidden-prefix zips:
  * `v1.11.0`'s Unpacker installs GOOD from them;
  * `v1.11.1`'s refuses them;
  * the same bytes are the fuzz seeds.
* **V9.** Links and identifiers are clean on every changed file. The
  migration guide has `### From v1.11.0 to v1.11.1` and
  `## 12. From v1.11 to v1.12`.

## Rollout and Rollback

* **`v1.11.1`** only refuses archives that 0012 §4's rule already
  forbade. A program that relied on one, a third-party zip with
  prepended data or a reserved name, stays on `v1.11.0` while its
  archive is fixed. A wrongly refused archive is fixed forward in a
  `v1.11.2`, with its test.
* **`v1.12.0` is additive.** Nothing enables `ghattest` by default, so
  adopting the release changes no update's verification.
* **The journal is new on disk.** A program that goes back to `v1.11.x`
  after a crash:
  * finds a `.selfupdate.pending` that `v1.11.x` ignores, since it
    matches no leftover pattern;
  * sweeps the backup, as before;
  * keeps any `.selfupdate.pending-tmp-<n>` file, as `v1.11.x` does not
    know the name.

  Such a file is under 1 KB, and `v1.12.x` sweeps it again.
* **Releases are immutable, and tags are never moved.** A fix is a new
  patch release.

## Execution Record

### Approval (2026-10-08)

The owner approved this PLAN on 2026-10-08: "Approved to proceed". That
accepts N1–N14 and the corrections, which R0 copies into the MADR as
amendment A1.

### Phase R0: records (2026-10-08)

* **The MADR** gains `## Amendments` with A1: rows N1–N14 and
  corrections 1–6, copied from this PLAN. Its "Not verified" first item
  points to correction 3.
* **This PLAN:** `status: in-progress`, and this record.
* **`docs/README.md`:** the 0017 PLAN row reads `in-progress`.

### Phase D1: documentation on `main` (2026-10-08)

* **`docs/architecture.md`** (the concurrency paragraph after the
  release-workflow list): GitHub keeps one pending run per group; a third
  tag cancels the waiting one, whose tag has no release until its publish
  job is re-run; `queue: max` waits for actionlint.
* **`docs/guides/building-releases.md`**, step 4:
  * the "One publish at a time" bullet adds the third-tag cancellation,
    the re-run, and GitHub's no-events limit for a push of more than three
    tags;
  * "Before the first release" gains "Restrict who can create `v*`
    tags", with what provenance does not stop without it (question 3).
* **0004-MADR P3's open-work table:**
  * the ghattest row splits into its exec verifier (0017 track 3) and its
    `sigstore-go` variant (0017, "Not decided here");
  * the 0015 row splits into the zip and names row (track 2), the crash
    row (track 3), and the `queue: max` row (item 5);
  * each cites 0015-PLAN `:57-68`.
* **`docs/README.md`:** "§6 to §10" becomes "§6 to §11" (correction 6).
* **This PLAN** is staged with each phase, for its record (Phase
  procedure, step 7), though the phase's **Files** list does not name it.
* **Checks:**
  * `make gate`: every step `rc=0`, `overall=0`; apicheck "compatible
    with v1.11.0"; links "373 links in 52 files, 0 broken";
  * `scripts/check-docs.sh` on the four files: "122 links in 4 files, 0
    broken", "0 findings";
  * markdownlint: "0 issues".

### Deviation D1 (2026-10-08): P1's gap plant, and where the Info-ZIP case runs

* **Found:**
  * **The plan's plant for the in-loop gap check survived.** It changed
    `if l.hdr > at {` to `if false && l.hdr > at {`, run against
    `zip hidden entry in a gap`. `hiddenGapZip` puts the hidden entry
    after the last named entry, so the end check, `if at != cdOff {`,
    refuses it with the same "unreferenced bytes" message, and the
    in-loop check is never reached.
  * **`TestUnpackAccepts` runs the image check,** so an Info-ZIP fixture
    there would have to hold a real executable for the host, a 1–2 MB
    constant.
* **Resolutions offered:**
  1. a row, `zip hidden entry between entries`, whose hidden entry lies
     between two named entries, with a plant on each check (recommended);
  2. the plan's row only, which leaves the in-loop check unproven.

  And for the Info-ZIP case:
  1. a small Info-ZIP zip run through `extract`, which applies every zip
     check but the image check (recommended);
  2. a zip built at test time with the `zip` tool, skipped where there
     is none.
* **Decision:** the owner chose both recommended resolutions.
* **Added to P1:**
  * `hiddenBetweenZip` in `main_test.go`, and its row in
    `TestUnpackRefuses`;
  * `TestOtherWritersTile` in `tiling_test.go`, with the 412-byte zip
    Info-ZIP's zip 3.0 wrote with `-X` (`hello.txt`, `docs/`,
    `docs/readme`, `relay`), as a hex constant.
* **Signatures:** the helpers take the program's bytes and the hidden
  bytes as arguments, for example `hiddenGapZip(tb, good, evil)`, so the
  fuzz seeds stay small.

### Phase P1: zip entries tile the file (2026-10-08)

* **Fix (`unpack.go`):**
  * `directoryBounds`, `descriptorLen` and `checkTiling` from the
    prototype, with the `*zip.Reader` parameter;
  * N4's check, that the central records' lengths sum to the directory's
    size, with the constant `centralRecordLen = 46`;
  * the call after the `checkLocal` loop.

  No import was added.
* **Red,** on `v1.11.0`'s `unpack.go`: the eight planned refusal rows
  each failed, `unpack_test.go:280: Unpack = <nil>, want "<message>"`,
  for:
  * `unreferenced bytes`;
  * `does not end where its end record begins`;
  * `data follows the end of the central directory`;
  * `does not match its central record`;
  * `reads two ways`;
  * `disagree on a data descriptor`;
  * `disagrees with the end record`;
  * `do not fill the central directory`.

  D1's row failed the same way, "Unpack = <nil>, want "unreferenced
  bytes"", in a copy with the tiling call off. `tiling_test.go` failed to
  build: `undefined: directoryBounds`, `undefined: descriptorLen`. The
  three new accept rows passed (pins).
* **Green:** `go test -count=1 ./selfupdate/archive/
  ./internal/cmd/selfupdate-release/ ./selfupdate/releasespec/` passed.
  The release tool's round trip unpacks every archive it builds, so it
  also passed on pack's output.
* **Plants,** each in a scratch copy:

  | Plant | Result |
  | :--- | :--- |
  | the tiling call off | failed all nine zip rows; first: `Unpack = <nil>, want "unreferenced bytes"` |
  | the in-loop gap check off | the plan's run survived (D1). Against `zip hidden entry between entries`, it fails: `Unpack = <nil>, want "unreferenced bytes"` |
  | the end check (`if at != cdOff {`) off | failed `zip hidden entry in a gap`: `Unpack = <nil>, want "unreferenced bytes"` |
  | the two-ways check off | failed `zip descriptor two ways`: `Unpack = <nil>, want "reads two ways"` |
  | the descriptor-flag check off | failed `zip descriptor flag`: the archive was refused as `unreferenced bytes [1117538, 1117554) …` instead |
  | N4's comparison flipped | failed `zip central directory slack`: `Unpack = <nil>, want "do not fill the central directory"` |

* **Fuzz:** `FuzzUnpackZip` for 60 s, with the two new seeds: 9,253,495
  executions, no failure. No corpus file was written to the tree.
* **Docs:** `archive/doc.go` names the rule, citing 0017-MADR 2B.
* **Checks:**
  * `make gate`: every step `rc=0`, `overall=0`; apicheck "compatible
    with v1.11.0";
  * `make pre-add-check` on the six files: "6 file(s) clean (gofmt,
    golangci-lint, go vet, go test, govulncheck)".
