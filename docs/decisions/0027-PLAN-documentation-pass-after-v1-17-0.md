---
status: in-progress
date: 2026-10-20
associated-madr: "0027-MADR-documentation-pass-after-v1-17-0.md"
---
# Implement the documentation pass after v1.17.0, the Examples, the three defect fixes and the two CI gaps, as v1.17.1

Associated MADR: [0027-MADR-documentation-pass-after-v1-17-0.md](0027-MADR-documentation-pass-after-v1-17-0.md)
(D1-A, D2-A, D3-A, D4-A, D5-A, as the owner chose).

## Goal

Every document and doc comment a reader of the module meets is true of
the code `v1.17.1` ships, and complete for v1.16.0's and v1.17.0's API.
The three defects (C1–C3) and the two CI gaps (G1, G2) are fixed, each
with a check seen failing first, and `v1.17.1` is released.

## Scope

### In scope

| Phase | What | Commit |
| :--- | :--- | :--- |
| X0 | this pair, its index rows, and 0025-PLAN's "After closing" note | records only |
| X1 | C1: `install.ps1`'s uninstall checks its removals; `install.sh` parity | code and tests |
| X2 | C2 and C3: a nil `*Updater`'s `Checker`; the managed session's lock | code and tests |
| X3 | G1 and G2: govulncheck in `go126`; signednote's fuzz corpus | CI |
| X4 | Go doc comments, citations, script and workflow headers (A5) | comments |
| X5 | the Examples (D4-A) | tests |
| X6 | the Markdown documents (A1–A4) | docs |
| X7 | `v1.17.1` | release |
| X8 | closing | records |

### Out of scope

* The records' bodies: they are history. Only the index and the records'
  front matter are checked.
* The repository settings (0025-MADR Amendment A5): the owner keeps them
  deferred.
* An `ExampleNewExecReconciler` (A5-36): 0025-MADR D14 did not ask for
  one, and the guide describes the receipt protocol. Recorded as not
  added.
* The fleet's two other agent pointers that a workspace standard expects
  (outside A1): not a statement these documents make.

## Rules

* The gate and the citation rule are AGENTS.md's. A Go comment cites per
  0024-MADR D2-A; Markdown cites by full filename and links.
* Each new test, assertion and CI step is seen failing on a
  `scripts/plant-copy.sh` copy or a scratch run, never on the tree, and
  the failure is quoted in the phase's record.
* `make pre-add-check` on every Go file a phase changes; `make gate`
  ends `overall=0` before each push; markdownlint on changed Markdown;
  `actionlint` on a changed workflow.
* `apicheck` stays compatible with `v1.17.0`.
* Each phase is one commit, on the owner's word; each push on the owner's
  word.
* A finding that turns out wrong in execution is recorded as not changed,
  with the reason, not silently dropped.

## Findings

IDs are the audits' own. "Fix" is the change this pass makes, under
D1-A unless another decision is named.

### README.md, AGENTS.md, SECURITY.md, CHANGELOG, index (X6, A1)

| ID | Where | Finding | Fix |
| :--- | :--- | :--- | :--- |
| A1-01 | SECURITY.md:39-40 | govulncheck "on its 1.26.9 floor"; only the Linux `validate` job runs it (G1) | true after X3 (D3-A); reword to name both jobs |
| A1-02 | docs/README.md:100 | the crasher row omits `selfupdate/verify/signednote`; CI's upload omits its corpus (G2) | add signednote; X3 fixes the upload |
| A1-03 | AGENTS.md:39-40, 226-230 | `CheckNoLeak` and its rule for `service` tests are absent | one sentence, citing 0025-MADR D4 and A6 |
| A1-04 | README.md:69 | "only `service.HandOffHop` … exits" | name `HandOffHopContext`, and `LoadHandOffEnv`/`…Context`/`ReportFunc`, which call it |
| A1-05 | SECURITY.md:26-27 | "and its GitHub digest" | "and, when GitHub gives one, its digest" |
| A1-06 | README.md:41 | selfupdatetest's row omits `VerifyReleaseSource` | add it |
| A1-07 | README.md:139-158 | no word on `sign: true`, `note-verifier-key`, the secret, or the reserved extra names | one sentence each, linking building-releases §13 |
| A1-08 | docs/README.md:75-124 | no rows for `NewGitHubUpdater`, hooks and `--force`, `VerifyReleaseSource`, `NewSlogReporter`/`Clock` | add rows pointing at the X6 sections |
| A1-09 | docs/README.md:98 vs AGENTS.md:206-208 | the by-hand lists differ: the stray-`.git` run | add it to docs/README.md |
| A1-10 | .golangci.yml:71-73; AGENTS.md:33-34 | winproc "builds every process the library's packages start"; the release tool and testproc start their own | "every process a package under `selfupdate/` starts", as winproc's doc says |
| A1-11 | Makefile:4, :7 | "Prefer GOBIN, then GOPATH, then PATH"; `GOLANGCI_LINT` looks only in GOPATH/bin | give `GOLANGCI_LINT` the same lookup as the other two tools |
| A1-12 | CHANGELOG.md:9-11 | no entry for the S16 fix on `main` | the `v1.17.1` entry (X7) names it |

### `docs/architecture.md` (X6, A2)

| ID | Where | Finding | Fix |
| :--- | :--- | :--- | :--- |
| A2-01 | :586-590 | `make fuzz` names three packages; signednote is a fourth | add it: "five, three, one and one" |
| A2-02 | :692-693 | the corpus upload covers three packages (G2) | true after X3 |
| A2-03 | :658-723, :660 | no `go126` bullet; "Go version read from `go.mod`" | add the job, with its govulncheck (X3); qualify the sentence |
| A2-04 | :718-721 | the Debian run "(dash, curl)"; it requires bash too | "dash and bash, curl" |
| A2-05 | :304-313, tree :103-104 | testproc: no `CheckNoLeak`, no `Proc.Wait` | add both; cite 0025-MADR D4 and A6 |
| A2-06 | :419-421 | the comparable-struct example is `Config.Clock`; `Config` has slices and never was comparable | use `CheckerConfig.Clock`, as A4 says |
| A2-07 | :176-185 | v1.16.0's `HTTPError`, `CommandError`, the `…Context` hand-off calls and redaction absent | add sub-bullets with their files |
| A2-08 | :237 | "the six typed errors" | "the six sentinel errors and `CommandError`" |
| A2-09 | :329-332 | no bullet for signednote or `internal/cmd/selfupdate-note` | add a release-signing bullet (0025-MADR D10) |
| A2-10 | :333-338, :451-454, tree :109-110 | no installers paragraph: hook times, options, exit codes | add one, linking building-releases §12 |
| A2-11 | :437-439 | the floor check: one rule, "every field" | the floors table, "field or value", the highest-floor rule, the module exemptions |
| A2-12 | (none) | no release procedure for this module | a short section that links the 0017-PLAN procedure |
| A2-13 | :34-119 | the tree omits CHANGELOG.md and SECURITY.md | add them |
| A2-14 | :9-30 | no word on the `retract` of `v1.0.0`–`v1.4.1` | one sentence (0025-MADR D7) |
| A2-15 | :25 | 1.26.9 cited to "0025-MADR D8"; D8 chose 1.26.8, A1 made it 1.26.9 | "D8, Amendment A1" |
| A2-16 | :299-301, :421, :731, :736 | records cited by number alone; 0022-MADR's headings are "A2", "A3" | link each by filename, with the record's own anchor |
| A2-17 | :126, :129, :133 | tracked fuzz seeds not counted | "plus N fuzz seeds" |
| A2-18 | :556-559, :573-574 | signednote's `RequiredFrom` gap and the key-custody caveat absent | add both |
| A2-19 | :458-462 | identity: no optional commit, no skipped legs | mention both |
| A2-20 | :217-227 | buildinfo's `LDFlags`; cli's `Help`, `HelpText`, `DefaultTimeout`, `FlagSet` absent | add them |
| A2-21 | :396-398 | User-Agent and credential order predate `NewGitHubUpdater` | name its default, and Token, Credentials, `GH_TOKEN`, `GITHUB_TOKEN` |
| A2-22 | :111-112 | the fixture's `nostamp` program unnamed | name both programs |
| A2-23 | :504-511 | the note steps nested under "archives only" | their own numbered item |

### `docs/guides/extending-selfupdate.md` (X6, A3)

| ID | Where | Finding | Fix |
| :--- | :--- | :--- | :--- |
| A3-01 | :3-9 | `NewGitHubUpdater`, the default the guide extends, is never named | open with it and its Examples; each seam composes `New` |
| A3-02 | :11-15 | "what `v1.1.0` added … are in 0004-PLAN" | point at CHANGELOG.md and the migration guide |
| A3-03 | :24-40 | no `CheckerConfig.Clock` for record ages | one sentence |
| A3-04 | :24-40 | `*HTTPError` absent | one sentence |
| A3-05 | :78-88 | publish's channels written by hand; the spec's `prerelease_channels` and the build output carry them | rewrite, linking building-releases §1 and §4 |
| A3-06 | :95-128 | `NewSlogReporter` absent | one sentence beside `MultiReporter` |
| A3-07 | :97-99, :122-124 | the JSON reporter's omission rule and the final `result` object | state both; link the cli package doc |
| A3-08 | :134-153 | the redaction rule and its JSON caveat | one bullet |
| A3-09 | :291-322 | signednote in prose only; "`SHA256SUMS.sig`" | a snippet, `RequiredFrom`, freshness, "since v1.16.0", the Example; `.note` |
| A3-10 | :365-379 | `NewVersionProberWith` absent; `ExampleChainTransformers` not pointed to | add both |
| A3-11 | :427-433 | a dry run for another platform skips the probes | one sentence |
| A3-12 | :437-439, :464-473 | a kept previous binary loses setuid and setgid | one sentence |
| A3-13 | :424-425 | the ad-hoc codesign tests need `SELFUPDATE_REQUIRE_CODESIGN=1` | name it |
| A3-14 | :494 | launchd's `Enabled` also needs the label not disabled | add it |
| A3-15 | :538-545 | `*service.CommandError` absent; `ExamplePollHealthy` not pointed to | add both |
| A3-16 | :831-834 | `LoadHandOffEnvContext` and `HandOffHopContext` absent | name them |
| A3-17 | :886-905 | no section on another release host; `VerifyReleaseSource` absent | add "Use another release host" |
| A3-18 | :632-638, :333-337 | snippets overwrite `err` before checking it | check each |
| A3-19 | :196-199 | one run at a time per Updater | add `ErrConcurrentUpdate` |

### The migration and building guides (X6, A4)

| ID | Where | Finding | Fix |
| :--- | :--- | :--- | :--- |
| A4-01 | migrating:411-412 | "§7 to §15" | "§7 to §17" (and "§18" once X7 adds it, if a section is added) |
| A4-02 | migrating:31-58 | no index line for `v1.5.1` | add it |
| A4-03 | building:386 | "a build time" in `Identity()`'s `Time`; it is the commit's time | "the commit and its time" |
| A4-04 | building:359 | the identity timeout "waits for input"; its standard input is the null device | name the network, not input |
| A4-05 | building:427-428, :364 | the prefix rule: letters and digits, a leading letter, 32 characters, from `installer.name` | state the rule |
| A4-06 | building:71-73 | channels must also be in strictly descending ASCII order | add it |
| A4-07 | building:220-225, §13 | the sign job's `<artifact-name>-note`, kept 1 day, and its reserved name | name them in §13 |
| A4-08 | building:323-337 | `GOFLAGS=-mod=readonly`, no `GOPRIVATE`: private modules cannot build | state it |
| A4-09 | building:200-201 | "as the publish workflow has always required"; nothing checks the pin | "as this repository tells callers; neither workflow checks it" |
| A4-10 | building:345-368 | missing messages: refuse-existing, `sign` without a key, the note key, the gh checks; no link to README's recovery | add rows and the link |
| A4-11 | building:479-490 | `-h`/`--help` absent; `--uninstall` acts on the `--dir` given | add both |
| A4-12 | building:489, :525 | a running Windows program is reported removed (C1) | document the new behaviour after X1 |
| A4-13 | building:215-219 | `artifact-name`'s pattern | add it |

### Go doc comments, Examples, headers (X4, X5, A5)

| ID | Where | Finding | Fix |
| :--- | :--- | :--- | :--- |
| A5-01 | buildinfo.go:6-7; cli/doc.go:34-35 | "exactly one -ldflags string, which LDFlags returns"; the recipe adds `-s -w` | the two `-X` stamps `LDFlags` returns |
| A5-02 | errors.go:18-20; checker.go:124-127 | a nil `*Updater`'s `Checker` panics (C2) | X2 |
| A5-03 | types.go:541-543 | managed sessions do not serialize (C3) | X2 |
| A5-04 | types.go:160-163 | `PendingBackup` is set on any OS | "a backup the commit could not remove: on Windows, …" |
| A5-05 | types.go:224 | the source never reads `Limits.Manifest`; the Updater does | say which applies which |
| A5-06 | types.go:227-229 | "Nil keeps the v1.0 behaviour" | name Token, `GH_TOKEN`, `GITHUB_TOKEN` |
| A5-07 | types.go:667, :770, :812-813 | the event, `Asset` and `Executable` name the archive too | "the release asset (the executable, or the archive)" |
| A5-08 | types.go:586-593 | `Lifecycle`'s contract unstated | a short paragraph |
| A5-09 | errors.go:29-31 | `ErrUnsupportedPlatform`'s other two cases | list them |
| A5-10 | checkcache.go:153-154 | argument errors; a save error with a valid record | add both |
| A5-11 | doc.go:92-94 | `NewSlogReporter` absent | add it |
| A5-12 | doc.go:176-177 | selfupdatetest's `VerifyReleaseSource` absent | add it |
| A5-13 | example_test.go:27 | `http.Client{Timeout: 15 * time.Minute}`, which the Client doc advises against | ResponseHeaderTimeout, or a context deadline |
| A5-14 | document.go:12-37 | `ResultDocument`'s fields bare | one sentence on the type |
| A5-15 | example_test.go | no Example for `NewSlogReporter`, `NewVersionProberWith`, `CleanupPending` | X5 (D4-A) |
| A5-16 | checker.go:206, :226; updater.go:242, :264, :318 | "PLAN §4.6 step N", mcplib's | cite 0003-MADR's anchors |
| A5-17 | github.go:234-236; winproc_windows.go:24-26 | another repository's record beside this one's | drop the other repository's pointer (0024-MADR E10) |
| A5-18 | cli/command.go:19-22 | options `Run` refuses also report as a failed run | add it |
| A5-19 | selfupdatetest.go:1-4 | package doc omits `VerifyReleaseSource`, `SourceCase` | add them |
| A5-20 | selfupdatetest.go:6 | "standard library only"; it imports selfupdate | "the standard library and selfupdate" |
| A5-21 | conformance.go:28 | no Example | X5 |
| A5-22 | archive/doc.go:32 | "only the standard library" | "the standard library and selfupdate" |
| A5-23 | archive/unpack.go:59 | a negative `MaxEntries` is refused | add it |
| A5-24 | releasespec/installer.go:46 | hooks "around the install" | "and before --uninstall" |
| A5-25 | releasespec/installer.go:56 | Args "without NUL"; the enforced class is narrower | state the class and why |
| A5-26 | releasespec/spec.go:74-78 | `IdentityArgs`' two rules | add both |
| A5-27 | releasespec/spec.go:332-335 | `SHA256SUMS-*` and `SHA256SUMS.note` also refused | name both |
| A5-28 | releasespec/validate.go:44-45 | the `before_uninstall` rule's record unnamed | add 0026-MADR |
| A5-29 | releasespec/installer.go:34-35; spec.go:59-60 | only the scripts the platforms need are generated | "as InstallerScripts decides" |
| A5-30 | releasespec/doc.go:19-20 | a subdirectory works | "in that directory or below it" |
| A5-31 | signednote/doc.go:37 | "The build workflow's publish step signs" | the publish workflow signs |
| A5-32 | signednote/example_test.go:10 | names `releaseKeys` | X5 |
| A5-33 | signednote/example_test.go:14-31 | placeholder keys fail `New` | X5: real public keys, `// Output:` |
| A5-34 | service/handoff.go:70-79 | `HandOffResult`'s fields bare | document them |
| A5-35 | service/errors.go:46, :51 | `CommandError`'s bound and Name | state both bounds; "what was acted on" |
| A5-36 | service/example_test.go | no ExecReconciler Example | not added (Out of scope) |
| A5-37 | launchd/example_test.go:17 | no `Plist`; `New` refuses it | X5 |
| A5-38 | launchd/lifecycle.go:190-191 | Start omits `ErrTimeout` and ctx | add both |
| A5-39 | launchd/lifecycle.go:92-96 | Stop names `ErrNotInstalled`, never returned | drop it; a job not loaded counts as stopped |
| A5-40 | launchd/job.go:6 | it also runs `/bin/ps` | add it |
| A5-41 | systemd/name.go:14-15 | a bare template is refused | say so |
| A5-42 | systemd/lifecycle.go:134-135 | Start has no ctx sentence | add it |
| A5-43 | scm/lifecycle.go:205-206 | Start omits `ErrUnhealthy`, `ErrTimeout`, ctx | add them |
| A5-44 | scm/detach.go:54-55 | names only `ReportFunc`, `LoadHandOffEnv` | add `LoadHandOffEnvContext` |
| A5-45 | taskscheduler/lifecycle.go:127-131 | Stop's Errors omit `ErrTaskDisabled`, which step 1 names | add it |
| A5-46 | taskscheduler/detach.go:66-67 | as A5-44 | as A5-44 |
| A5-47 | taskscheduler/scheduler.go:39-40, :79-80 | "(Amendment A1, probe N)" with no record | "0021-MADR Amendment A1" |
| A5-48 | taskscheduler and scm methods taking `_ context.Context` | none says the context is ignored; `Task.Start` ignores it | one sentence per type |
| A5-49 | internal/testproc/testproc.go:1-11 | package doc omits `CheckNoLeak` | add it |
| A5-50 | testproc.go:216 | "Every package's tests share it" | name the packages |
| A5-51 | check-docs.sh `--arch`; gate.sh header | "tracked"; it counts untracked files too | "the tree's files" |
| A5-52 | check-api-compat.sh header; ci.yml:31-32 | "the newest v1.* tag"; it is the nearest reachable | "the nearest v1.* tag reachable from HEAD" |
| A5-53 | refuse-existing-release.sh header | a usage error also exits 1 | add it |
| A5-54 | verify-selfupdate-release.sh:2-10 | no usage line or exit codes | add both |
| A5-55 | publish-selfupdate-release.yml:1 | no header comment | add one like the build workflow's |
| A5-56 | ci.yml:123-125 | `SELFUPDATE_REQUIRE_PYTHON` also makes the releasespec parity test mandatory | name it |

## Implementation Steps

### Phase X0: records

This pair, `docs/README.md`'s two rows, and 0025-PLAN's "After closing
(2026-10-20)" note, already in the tree. One commit, records only.

### Phase X1: C1, the uninstall's removals (D2-A)

1. **Probe first,** on the Windows test host: does `Remove-Item -Force`
   on a running `.exe` fail, and does `Test-Path` then find it? If not,
   C1 is not a defect as stated: stop and amend this pair.
2. **`install.ps1`:** after removing a program and its `.prev`, check
   that the program is gone. If it is not, write `install: could not
   remove <exe>; stop the program and run the uninstall again` to
   standard error, go on with the others, and return 3 at the end. The
   header's exit codes and the PATH rule stay as they are, with 3's
   meaning widened to "removed only in part".
3. **`install.sh`:** the same check after `rm -f`, for parity.
4. **Tests:** `installer_ps_test.go` gains a case with a program held
   running during `-Uninstall`: exit 3, the message, the other program
   removed, the folder still on PATH. `installer_sh_test.go` gains the
   parity case with a removal that fails (a directory where the program
   is, or a read-only directory).
5. **Seen failing:** the check removed, on plant copies; the Windows case
   on the Windows test host under both PowerShells and both forms.

### Phase X2: C2 and C3 (D2-A)

1. **C2:** `(*Updater).Checker` returns `&Checker{}` for a nil receiver;
   a test calls `Check` on it and wants `ErrNotConstructed`.
2. **C3:** the managed session gets a `sync.Mutex` held by each method,
   as `session.go` does. A `-race` test runs `Install` and `Close` at
   once on one managed session over fakes.
3. **Seen failing:** each fix removed on a plant copy: the panic; the race
   report.
4. Their doc comments (A5-02, A5-03) stay as written, which is now true.

### Phase X3: G1 and G2 (D3-A)

1. **G1:** the `go126` job gains a govulncheck step, installed as the
   `validate` job installs it, and run with that job's toolchain. Seen
   failing: the same command under an older 1.26 patch with a published
   standard-library advisory, in a scratch run, with the advisory quoted.
2. **G2:** the corpus upload gains
   `selfupdate/verify/signednote/testdata/fuzz/`.
3. `actionlint`, and the workflow checker's rules
   (`scripts/check-workflows.sh`).

### Phase X4: Go doc comments, citations, headers (D1-A)

Every A5 row except those X2 and X5 own. Each comment is read against
its code first. `go vet` for linux, darwin and windows, and `go doc` of
each changed identifier.

### Phase X5: the Examples (D4-A)

1. launchd's `ExampleNew` gains a `Plist`; its comment says what it shows.
2. signednote's `ExampleNew` uses two real verifier keys generated for an
   example name, fixes the `releaseKeys` comment, and gains `// Output:`.
3. New: `ExampleNewSlogReporter` and `ExampleNewVersionProberWith` in
   `selfupdate`, `ExampleVerifyReleaseSource` in `selfupdatetest`, each
   with `// Output:`; `ExampleCleanupPending`, compile-only.
4. A5-13's client is replaced as its row says.
5. **Seen failing:** each new `// Output:` with a planted wrong line.

### Phase X6: the Markdown documents (D1-A)

Every A1–A4 row, by file. Each new snippet compiles in a scratch module
against the tree; each spec example parses. The links, ids and arch
checks, and markdownlint, pass.

### Phase X7: `v1.17.1` (D5-A)

The 0017-PLAN procedure, as 0026-PLAN U5 ran it: the pre-tag change
(CHANGELOG `## v1.17.1`, naming C1–C3, G1, G2 and 0025's A6; the
migration guide's "From v1.17.0 to v1.17.1" part and index line), `make
gate`, the push and CI on the owner's word, the tag on the owner's word,
the tag's CI and identity legs, the pin commit, and the proxy.

### Phase X8: closing

`### Verification, at closing`, `status: complete`, the index row, and
the MADR's status.

## Verification

* **V1 (findings):** every row above is closed, or recorded as not
  changed with the reason.
* **V2 (defects):** C1's, C2's and C3's tests passed, and failed on their
  plants.
* **V3 (CI):** the `go126` job runs govulncheck and passes; the corpus
  path list holds four directories.
* **V4 (Examples):** `go test -run '^Example' ./...` passes, with the new
  ones running.
* **V5 (checks):** links, ids, arch, markdownlint and actionlint clean;
  `make gate` `overall=0`; `apicheck` compatible with `v1.17.0`.
* **V6 (release):** `v1.17.1` is tagged, its CI passes with the identity
  legs, and the proxy serves it.

## Rollout and Rollback

* **Rollout:** one commit per phase on `main`, pushed on the owner's
  word; `v1.17.1` at X7. A caller gets C1's fix by pinning the workflows
  to `v1.17.1`, and C2 and C3 by requiring it.
* **Rollback:** before the tag, `git revert` of a phase; after it, fix
  forward. A script that needs the old exit 0 for a partial Windows
  uninstall can treat 3 as success, as it already may for `--force`.

## Execution record

### Approval (2026-10-20)

The owner chose every recommendation: "Fix all three" (D2-A), "Close
both in CI" (D3-A), "Release v1.17.1" (D5-A), and "All findings,
fix+add Examples" (D1-A, D4-A), which also approved the pair and asked
to start with X0. The MADR became `accepted`, and this plan
`in-progress`.

### X0 (2026-10-20)

1. This pair, and `docs/README.md`'s rows for it: the MADR `accepted`,
   this plan `in-progress`.
2. 0025-PLAN's "After closing (2026-10-20)" note: S16 ships with the
   next release, now `v1.17.1`; the throwaway repository is deleted;
   the settings stay deferred.
3. Links, ids and arch clean; markdownlint skips the records by its
   configuration.

### X1 (2026-10-20)

1. **The probe (step 1),** on the Windows test host, under Windows
   PowerShell 5.1.26100 and PowerShell 7.6.6. A copy of `PING.EXE` named
   `relay.exe`, with a `.prev`, was started, and the installer's own line
   run on both:
   * `Remove-Item -LiteralPath $exe, "$exe.prev" -Force -ErrorAction
     SilentlyContinue` left `relay.exe` (`exe=True`) and removed the
     `.prev` (`prev=False`);
   * with `-ErrorAction Stop`: `System.UnauthorizedAccessException: Access
     to the path '…\relay.exe' is denied.`;
   * a rename of the running file succeeded. The plan reports instead, so
     the rename is noted and not used.

   C1 is a defect as stated, so the phase went on.
2. **`install.ps1`:** after `Remove-Item`, a program still there is
   reported, `install: could not remove <exe>; stop the program and run
   the uninstall again`, and the loop goes on. The uninstall returns 3 when
   a hook failed or a program was not removed. The header's exit 3 gains
   "or removed only in part: a program could not be removed".
   * **Caught before any run:** the first draft named its flag `$left`,
     which the PATH rule below already uses for the programs still
     installed (`$left = @($Products | …)`). The reassignment would have
     made a `-Product` uninstall that leaves another program installed
     exit 3. The flag is `$notRemoved`, and `install.sh`'s is
     `not_removed`.
3. **`install.sh`:** `rm -f … || :`, then a program still there (`-e` or
   `-L`) is reported, `install: could not remove <path>`, and the exit is
   3 the same way. Without `|| :`, `set -eu` would have ended the
   uninstall at a failing `rm`. The message leaves out "stop the program",
   since Unix unlinks a running program's file. The header changes as
   `install.ps1`'s does.
4. **Tests:**
   * `installer_ps_test.go`: "-Uninstall reports a running program it
     cannot remove". The stand-in `relay.exe` (with a `.prev`) and
     `relayctl.exe` are placed, PATH is set to the folder, and `relay.exe
     hold` is started. `-Uninstall` exits 3 with the message and `removed
     …relayctl.exe`, does not claim `relay.exe` was removed, leaves only
     `relay.exe`, and keeps the folder on PATH. With the program stopped, a
     second `-Uninstall` exits 0, removes it, and clears PATH.
   * `installer_sh_test.go`: "--uninstall reports a program it cannot
     remove". A non-empty directory stands where `relay` goes, which no
     `rm -f` removes, even as root; `relayctl` is a stand-in.
     `--uninstall --no-hooks` exits 3 with the message and `removed
     …relayctl`, and leaves only `relay/`.
5. **Seen failing,** each on a `scripts/plant-copy.sh` copy:

   | Plant | Where | Failure |
   | :--- | :--- | :--- |
   | the `-e`/`-L` check → `if false; then` | `install.sh`, dash, sh, bash | 3 of 3: `installer_sh_test.go:405: exit 0, want 3` |
   | exit 3 only for a hook → `if [ "$uninstall_failed" = 1 ]; then` | `install.sh`, dash, sh, bash | 3 of 3: `installer_sh_test.go:405: exit 0, want 3` |
   | `if (Test-Path -LiteralPath $exe) {` → `if ($false) {` | `install.ps1`, both PowerShells, both forms | 4 of 4: `installer_ps_test.go:879: exit 0, want 3` |
   | `-or $notRemoved` dropped from the return | `install.ps1`, both PowerShells, both forms | 4 of 4: `installer_ps_test.go:879: exit 0, want 3` |

   The first `install.ps1` bundle failed to build: `if (Test-Path
   -LiteralPath $exe) {` occurs more than once in the template, so
   `plant-copy.sh` refused it. The plant takes the next line too, and each
   copy was checked to hold its plant before it was sent.
6. **Checks:**
   * here: `go test -run 'TestInstallSh'` `ok`; `go vet` for darwin and
     windows; `scripts/check-installers.sh` on `install.sh` clean;
     `make pre-add-check` on the two test files clean;
   * on the Windows test host, with
     `SELFUPDATE_INSTALL_REQUIRE_SHELLS=powershell,pwsh`: `go test -run
     '^TestInstallPs1'` `ok`, 238 subtests passing, the new case in each
     PowerShell and form; `check-installers.sh` on `install.ps1` clean.
   * The building guide's exit table (A4-12) changes in X6.

### X2 (2026-10-20)

1. **C2:** `(*Updater).Checker` returns `&Checker{}` for a nil receiver,
   and its doc says so. A zero `Checker` already fails `constructed()`, so
   `Check` and `CheckCached` return `ErrNotConstructed`.
   `TestZeroValues` adds `(*Updater)(nil).Checker()` to its list of
   unconstructed Checkers.
2. **C3:** `managedSession` gains `mu sync.Mutex`. `Target`,
   `CreateStaging`, `Close`, `Owns` and `Install` each hold it, and the
   type's comment says why. Two things were checked first:
   * nothing in `managed.go` calls the session's own methods while
     holding it, so there is no re-entry;
   * the Updater closes its session only in its own goroutine, from a
     `defer` (`updater.go:244-252`), so no library path waits on the new
     lock.
3. **A correction to step 2's test:** the step named a `-race` test of
   `Install` and `Close` at once. `managedSession` has no mutable field,
   so the race detector has nothing to report. The defect is one of
   order: `Close` ran while `Install` was between its calls.
   `TestManagedSessionSerializes` tests that instead:
   * `Install` runs over a `blockingStop` lifecycle, whose `Stop` waits;
   * `Close` is called once `Install` is inside it, and must not return
     within 200 ms;
   * after the release, `Install` must succeed, then `Close`, and the
     target holds the new bytes.

   The 200 ms bounds the look for an early return and orders nothing, as
   the test's comment says. With the lock, `Close` cannot return early, so
   the test cannot fail spuriously. It passed 20 times in a row under
   `-race`. No decision changes.
4. **Seen failing,** each on a `scripts/plant-copy.sh` copy, under `-race`:

   | Plant | Failure |
   | :--- | :--- |
   | the nil guard removed | `TestZeroValues`: `panic: runtime error: invalid memory address or nil pointer dereference`, today's behaviour |
   | `Install` without the lock | `managed_test.go:343: Close returned (<nil>) while Install was inside Stop` |
   | `Close` without the lock | the same |

   The plant script's check that each plant landed failed on the first
   one: it read the first line of the empty replacement. The panic shows
   that plant landed.
5. **Checks:**
   * `go test -race -run 'TestZeroValues|TestManaged' ./selfupdate/` is
     `ok`;
   * `make pre-add-check` on the four files is clean;
   * `apicheck` is compatible with `v1.17.0`, and ids and arch are clean;
   * `go doc ./selfupdate Updater.Checker` shows the new sentence.
   * A5-02's and A5-03's doc comments stand as written; both are now
     true.

### X3 (2026-10-20)

1. **G1:** the `go126` job gains "govulncheck, the floor toolchain",
   after its `go test`. It installs `golang.org/x/vuln/cmd/govulncheck@v1.8.0`
   as the `validate` job does, and runs it under that job's
   `GOTOOLCHAIN: local` and Go 1.26.9. A comment cites G1.
   * Could the floor build it? `govulncheck` v1.8.0's `go.mod` says `go
     1.26.0`, read from the module proxy.
2. **G2:** the `fuzz corpus` step's paths gain
   `selfupdate/verify/signednote/testdata/fuzz/`, four directories in all,
   one per fuzzed package (`Makefile:77-80`).
3. **Seen failing (G1),** as the job runs it, with toolchains and modules
   in a scratch module cache:
   * **Control,** the tree under `GOTOOLCHAIN=go1.26.9`: `No
     vulnerabilities found.`, exit 0.
   * **Plant,** a `scripts/plant-copy.sh` copy whose `go.mod` says `go
     1.26.0`, under `GOTOOLCHAIN=go1.26.0`: exit 3, `Your code is affected
     by 30 vulnerabilities from the Go standard library`. The first is
     `GO-2026-6617`, "HTTP/2 server crash due to HPACK encoder race in
     net/http". The copy's `go` line was lowered because Go refuses a
     module that requires a newer toolchain than the one running.

   G2 adds no check, so nothing is planted for it. The step's path list
   and `actionlint` confirm it, as the MADR's Confirmation says.
4. **Checks:** `actionlint .github/workflows/ci.yml`;
   `scripts/check-workflows.sh` (all rules, 2 workflows) and `--rule pins
   .github/workflows/ci.yml`; `scripts/check-workflows_test.sh`; and
   `scripts/workflow-shape_test.sh`. All pass. The step itself runs on CI
   after the push, and its log is recorded then (V3).

### X4 (2026-10-20)

47 of the 56 A5 rows, in 41 files. The other nine are elsewhere: A5-02
and A5-03 in X2; A5-13, A5-15, A5-21, A5-32, A5-33 and A5-37 in X5;
A5-36 out of scope. Each comment was read against its code before it
changed. Below, a row is "as its fix" unless what was found differs.

1. **`buildinfo`, `selfupdate`, `cli`:**
   * A5-01: the two `-X` stamps `LDFlags` returns, with `-s -w` added by
     the workflow (`build.go:27-28`). The cli doc says "the stamps
     buildinfo.LDFlags returns".
   * A5-04, A5-06, A5-07, A5-09, A5-11, A5-12 and A5-18, as their fixes.
   * A5-05: the source applies ReleaseJSON, ErrorBody, and Executable to
     every asset's declared size (`github.go:334`, `:346`, `:567`). The
     Updater applies Manifest and Executable to the downloads
     (`updater.go:261`, `:291`).
   * A5-08: `Lifecycle` gains a paragraph:
     * the string is `Request.Product`, and Installed false means a plain
       install;
     * Running is asked again after a Stop error;
     * recovery runs under a 2-minute context the caller's cancellation
       does not end (`managed.go:288`, `recoveryTimeout` at `:306`).
   * A5-10: CheckCached's comment already said that a record with a
     non-zero CheckedAt holds a real answer whatever the error. So only
     the argument refusals were added, with a pointer to that sentence for
     a save error.
   * A5-14: "Apart from SchemaVersion": SchemaVersion has no Result field.
     The other 21 do, checked against `go doc ./selfupdate Result`.
   * A5-16: the five "PLAN §4.6" anchors now cite 0003-MADR's own rows:
     * step 4's order → C4;
     * step 4's selected assets → A1 and A5, already cited;
     * step 9 → C9;
     * step 12 → C8, the Verified-before-Transforming order;
     * step 15 → C14.
   * A5-17: both other-repository pointers dropped. 0003-MADR A3 restates
     the HTTPS rule, and 0021-MADR D7 the ConPTY one.
2. **`selfupdatetest`, `archive`, `releasespec`, `signednote`:**
   * A5-19, A5-20, A5-22, A5-23, A5-24, A5-27, A5-29, A5-30 and A5-31, as
     their fixes.
   * A5-25: the class `^[A-Za-z0-9._:=/,+@%-]+$` (`installer.go:69`), and
     why.
   * A5-26: the two rules, from `validate.go:101-104` and
     `installer.go:103-109`.
   * A5-28: "the before_uninstall hook of 0026-MADR D1".
3. **`service` and its backends:**
   * A5-34: each `HandOffResult` field documented from `ReportFunc`
     (`handoff.go:180-200`) and the hop's failure write
     (`handoffhop.go:91-99`).
   * A5-35: Name lists what launchd names (label, plist, domain). Message
     states both bounds: the Runner's 1 MiB per stream
     (`runner.go:52`) and the exec reconciler's 4 KiB
     (`execreconciler.go:210`).
   * A5-38: launchd `Start` gains `ErrTimeout`, from the bootstrap's bound
     (`lifecycle.go:259`), and the ctx sentence.
   * A5-39: launchd `Stop` never returns `ErrNotInstalled`. The probe maps
     launchctl's not-found exit to "not loaded" (`job.go:261`, `:274`),
     and Stop returns nil for a job not loaded. The comment says so, and
     the sentinel is dropped.
   * A5-40, A5-41, A5-42, A5-44, A5-45, A5-46 and A5-47, as their fixes.
   * A5-43: scm `Start` gains `ErrUnhealthy` (`lifecycle.go:260`) and
     `ErrTimeout`, for a hung pending state or a ctx that ends in
     `waitPending` (`:321-341`).
   * A5-48: one sentence on each type, `Task` and `Service`, naming the
     methods whose parameter is `_ context.Context`:
     * `Task`: Installed, Running, Enabled, Start, Reconcile and Restore;
     * `Service`: the same less Start.

     The two `detach.go` lines the row listed pass ctx to
     `service.DetachProcess`, so they are not named. `Task.Start` was
     read: it runs the task and does not wait.
4. **`internal/testproc`, scripts, workflows:**
   * A5-49 and A5-50, as their fixes. A5-50 names the packages that call
     the check (`git grep -l CheckNoLeak -- '*_test.go'`).
   * A5-51: "no .go file" in `check-docs.sh`, and "the tree's files" in
     `gate.sh`.
   * A5-52: "the nearest v1.* tag reachable from HEAD", in
     `check-api-compat.sh` and `ci.yml`.
   * A5-53, as its fix.
   * A5-54: Usage and Exit lines, with `--extras` optional and defaulting
     to `[]` (`:27`). Exit 1 covers a finding or a missing directory
     (`:72-75`), and 2 a usage error (`:21`).
   * A5-55: a header for the publish workflow in the build workflow's
     style: what it publishes, tag only, the publish job's order, and the
     sign job.
   * A5-56: the parity test named beside the manifest differential.
5. **Checks:**
   * `gofmt` clean; `actionlint` on the three workflows;
     `scripts/check-workflows.sh`, all rules and `--rule pins` on
     `ci.yml`;
   * the workflow-shape, check-workflows, check-docs, check-api-compat,
     refuse-existing-release and verify-selfupdate-release script tests;
   * `make gate` with `SELFUPDATE_REQUIRE_PYTHON=1`: `overall=0`, 15 steps
     passing. `apicheck` compatible with `v1.17.0`; links 642 in 77
     files, 0 broken; ids 42 files, 0 findings.
6. **After the gate,** A5-54's `usage()` string in
   `verify-selfupdate-release.sh` still showed `--extras JSON` as
   required. It reads `[--extras JSON]`, as the header now does.
   `verify-selfupdate-release_test.sh`, shellcheck and the
   workflow-shape test pass again.
