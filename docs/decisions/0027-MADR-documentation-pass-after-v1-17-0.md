---
status: accepted
date: 2026-10-20
decision-makers: go-selfupdate-lib maintainers
consulted: "a read-only audit, in five parts, of every document a reader of the module meets, against the code at c491312 (v1.17.0 plus records and the 0025-PLAN S16 test-support fix), 2026-10-20; the earlier pass, 0024-MADR-documentation-pass-after-v1-15-1.md"
informed: the fleet's programs that read this module's guides and go doc, magic-cli-remote first
---
# Correct the documentation against v1.17.0, fill the gaps v1.16.0 and v1.17.0 left, fix the three defects and two CI gaps the audit found, and release v1.17.1

## Context and Problem Statement

The owner asked, on 2026-10-20, for a full audit of the documentation:
"ensure they are all up-to-date, accurate, grounded in the current codebase
and project state, and complete". The last pass,
[0024-MADR-documentation-pass-after-v1-15-1.md](0024-MADR-documentation-pass-after-v1-15-1.md),
was made against `v1.15.1`. Since then three releases have shipped:

* `v1.15.2`, from 0024's pass;
* `v1.16.0`, from
  [0025-MADR-align-with-go-sdk-design-practices.md](0025-MADR-align-with-go-sdk-design-practices.md),
  the largest change since the rename;
* `v1.17.0`, from
  [0026-MADR-before-uninstall-installer-hook.md](0026-MADR-before-uninstall-installer-hook.md).

0025's Amendment A6 (the leak check) is on `main` and not yet released.

The audit found 123 items. Most are gaps: v1.16.0 added API that the
guides and the architecture page do not describe. Three are defects in
code that the documents state correctly or not at all, and two are CI
gaps that make a document's claim untrue. A pass that only edited prose
would write those down as behaviour. This record decides five things:

* D1: what the pass corrects;
* D2: what is done about the three defects;
* D3: what is done about the two CI gaps;
* D4: which Examples are fixed or added;
* D5: whether a release follows.

**Facts [observed 2026-10-20, `main` at `c491312`]:**

* **The audit.** Five read-only audits, one per area, as 0024's were:
  1. README.md, AGENTS.md, SECURITY.md, CHANGELOG.md, `docs/README.md`,
     `docs/reports/`, the per-agent pointers, and the comments in
     `.golangci.yml` and the Makefile;
  2. `docs/architecture.md`;
  3. `docs/guides/extending-selfupdate.md`;
  4. the migration guide and `docs/guides/building-releases.md`;
  5. every package's doc comment, exported doc comments and Examples, and
     the header comments of `internal/`, the scripts and the workflows.

  Each audit checked every claim against the code, `go doc`, the
  workflows, the git tags and the records. They compiled each guide's
  snippets in a scratch module, parsed every spec example with
  `releasespec.Parse`, rendered the installers into a scratch directory,
  and ran the Examples. Each finding cites its evidence. The findings that
  change code, and a sample of the others, were checked again before this
  record: A1-01, A1-02, A2-06, A3-05, A4-01, A4-04, A4-12, A5-02, A5-03,
  A5-05, A5-31 and A5-37. The PLAN's "Findings" table lists all 123:

  | Where | Findings | Kind |
  | :--- | :--- | :--- |
  | README.md, AGENTS.md, SECURITY.md, CHANGELOG, index | 12 | SECURITY.md's govulncheck claim; the fuzz row; `CheckNoLeak` absent from AGENTS.md; who exits in a hop; the digest "when present"; `VerifyReleaseSource`, signing and the reserved extras absent from README; index rows for the v1.16.0 and v1.17.0 features; winproc's scope; the Makefile's lookup comment |
  | `docs/architecture.md` | 23 | no installer or hooks paragraph; signednote's fuzz target and corpus; no `go126` job in the CI section; `CheckNoLeak`; the wrong comparable-struct example; v1.16.0's errors, contexts and redaction absent; no signednote bullet; the floors; no release procedure; records cited by number alone |
  | `extending-selfupdate.md` | 19 | none wrong; the v1.16.0 API absent (`NewGitHubUpdater`, `HTTPError`, `CommandError`, `Clock`, `NewSlogReporter`, `NewVersionProberWith`, the `…Context` hand-off calls, `VerifyReleaseSource`, the redaction caveat); a stale channel-publishing instruction; no section on another release host |
  | migration and building guides | 13 | a stale "§7 to §15"; `v1.5.1` absent from the index; "build time"; the identity's standard input; the prefix and channel-order rules; the `-note` artifact; private modules unsupported; missing messages and options |
  | Go doc comments, Examples, headers | 56 | six comments wrong about the code; two Examples that fail when run; backends' "Errors" paragraphs that leave out errors the code returns; stale package docs; three citations with no record or another repository's; script and workflow headers |

* **The three defects:**
  * **C1, `install.ps1`'s uninstall reports a removal that failed.** It
    removes each program with `Remove-Item … -ErrorAction
    SilentlyContinue` and then prints `removed <exe>` unconditionally
    (`internal/cmd/selfupdate-release/installer/install.ps1:506-507`).
    Windows does not delete the image of a running program, so such a
    program stays installed while the installer says it was removed and
    exits 0. Its folder then stays on PATH (`:510-511`), which is right,
    and unexplained. Windows' refusal is [unverified on the Windows test
    host]; the code path was read. `install.sh` removes with `rm -f`
    (`install.sh:323`), which unlinks a running program's file on Unix.
  * **C2, `(*Updater).Checker` on a nil `*Updater` panics.**
    `ErrNotConstructed`'s doc says an Updater or a Checker "that its
    constructor did not make, such as a zero value or a nil pointer",
    returns it (`selfupdate/errors.go:18-20`). `Checker` reads `u.source`
    with no nil check (`selfupdate/checker.go:124-127`); a scratch run
    panicked with a nil pointer dereference.
  * **C3, managed install sessions do not serialize their methods.**
    `InstallSession`'s doc says "The sessions this package returns
    serialize their methods" (`selfupdate/types.go:541-543`), a contract
    0025-MADR D4 wrote and `v1.16.0` published. Only the standalone
    session holds a mutex (`selfupdate/session.go:21`);
    `selfupdate/managed.go` has none, and its `Install` runs Stop, the
    inner apply and Start unlocked.
* **The two CI gaps:**
  * **G1, govulncheck on the floor.** SECURITY.md says "`govulncheck`
    runs in CI and in `make gate`, on the Go 1.27.2 toolchain the module
    builds with and on its 1.26.9 floor" (`SECURITY.md:39-40`). It runs
    only in the Linux `validate` job, with the toolchain from `go.mod`
    (`.github/workflows/ci.yml:197-201`). The `go126` job builds, vets and
    tests with 1.26.9 and runs no govulncheck (`ci.yml:298-323`). The
    1.26.9 run was a one-time check recorded in 0025-PLAN.
  * **G2, signednote's fuzz corpus.** `make fuzz` fuzzes
    `selfupdate/verify/signednote` (`Makefile:80`, since `v1.16.0`). CI's
    `fuzz corpus` step uploads only the `testdata/fuzz/` directories of
    `selfupdate`, `selfupdate/archive` and `selfupdate/releasespec`
    (`ci.yml:157-161`), so a signednote crasher found on CI is lost.
* **Two Examples fail when run.** Both are compile-only, so `go test`
  never shows it:
  * launchd's `ExampleNew` passes no `Plist`, which `New` requires
    (`selfupdate/service/launchd/example_test.go:17`, `job.go:157-159`);
  * signednote's `ExampleNew` uses placeholder keys that `New` refuses
    ("verifier key: malformed verifier id"), and its comment names a
    `releaseKeys` that does not exist
    (`selfupdate/verify/signednote/example_test.go:10-31`).
* **Examples a reader needs and lacks:** `NewSlogReporter`,
  `NewVersionProberWith`, `CleanupPending` and
  `selfupdatetest.VerifyReleaseSource`. `NewVersionProberWith` exists to
  be used without a process (0025-MADR D12), and `VerifyReleaseSource` is
  the entry point for anyone writing another source.
* **What is released.** Since `v1.17.0`, `main` changed only docs,
  records and `internal/testproc` (0025-PLAN, "After closing"). C1 is in
  a template the build workflow renders at the caller's pinned commit, so
  a caller gets its fix only from a tag. C2 and C3 are in `selfupdate`.
* **The audit's own footprint.** Nothing in the repository was written,
  and nothing was installed under the home directory. The audits wrote
  only to the session's scratch directory and the Go caches.

## Decision Drivers

* A program author reads the guides and `go doc` as the contract. A
  document that is wrong costs that author more than a gap does, and a
  gap in v1.16.0's API leaves that API unused.
* A defect is not documentation: writing C1, C2 or C3 down as behaviour
  would make the guide accurate and the library wrong.
* A security document states what the repository does, not what it did
  once.
* No exported API breaks; `apicheck` stays compatible with `v1.17.0`.
* Every new test, assertion or gate is seen failing first (AGENTS.md).

## Considered Options

### D1: what the pass corrects

* **D1-A.** Every finding: wrong, stale, broken, contradictory, missing
  and unclear, in every document and comment the audit covered.
* **D1-B.** Only what is wrong, stale, broken or contradictory.
* **D1-C.** The Markdown only; Go comments wait for another pass.

### D2: the three defects

* **D2-A.** Fix each in code, with a test that fails on today's code:
  * C1: after removing a program, `install.ps1` checks that it is gone.
    One that is not is reported, `install: could not remove <exe>; stop
    the program and run the uninstall again`, and the uninstall exits 3,
    whose meaning widens to "removed only in part". The rest are still
    removed, and the folder stays on PATH while a program is left in it.
    `install.sh` gets the same check after `rm -f`, for parity.
  * C2: `Checker` on a nil `*Updater` returns a zero `Checker`, whose
    methods already return `ErrNotConstructed`.
  * C3: the managed session takes a mutex in each method, as the
    standalone session does, so the published contract holds.
* **D2-B.** Correct the documents to state today's behaviour, and fix
  later.
* **D2-C.** Fix C1 only, and narrow the C2 and C3 docs.

### D3: the two CI gaps

* **D3-A.** Close both in CI: add a govulncheck step to the `go126` job
  (G1), and add `selfupdate/verify/signednote/testdata/fuzz/` to the
  corpus upload (G2).
* **D3-B.** Correct SECURITY.md to say the 1.26.9 check was run once, and
  close G2 only.

### D4: the Examples

* **D4-A.** Fix the two broken Examples, and make signednote's run with
  `// Output:`, since it needs no OS. Add `ExampleNewSlogReporter`,
  `ExampleNewVersionProberWith` and `ExampleVerifyReleaseSource` with
  `// Output:`, and a compile-only `ExampleCleanupPending`.
* **D4-B.** Fix the two broken Examples only.

### D5: a release

* **D5-A.** Release `v1.17.1` after the pass, carrying C1–C3 and 0025's
  Amendment A6, with the usual pins, changelog and migration-guide part.
* **D5-B.** Leave everything on `main` until the next feature release.

## Decision Outcome

Chosen options (the owner, 2026-10-20: "Fix all three", "Close both in
CI", "Release v1.17.1" and "All findings, fix+add Examples"): "D1-A",
"D2-A", "D3-A", "D4-A" and "D5-A", because:

* D1-A is what the owner asked for: "complete" covers the gaps, and most
  of the 123 items are gaps a reader of v1.16.0's API meets.
* D2-A: each defect is small, local and testable, and each contradicts a
  sentence the module already publishes. C1 misreports the one thing an
  uninstall reports; C2 and C3 break contracts `v1.16.0`'s docs promised.
* D3-A makes SECURITY.md's claim true rather than smaller, for the cost of
  one CI step, and keeps a crasher CI finds.
* D4-A: an Example that `go test` runs cannot drift from the code, and
  each new one shows a v1.16.0 entry point no Example shows.
* D5-A: C1 reaches a caller only through a tag, and C2 and C3 change what
  a program sees.

### Consequences

* Good, because the guides, the architecture page and `go doc` describe
  the module as `v1.17.1` ships it, including the API v1.16.0 added.
* Good, because an uninstall on Windows says when a program could not be
  removed, instead of reporting success.
* Good, because SECURITY.md's vulnerability claim is checked on every CI
  run, on both toolchains.
* Neutral, because exit 3 for a partial removal widens a code that
  already means "removed, but something failed"; a script that treats 3
  as a warning keeps working.
* Bad, because the pass touches most documents and many Go files, so its
  commits are large; the PLAN splits them by area.
* Bad, because `v1.17.1` needs a release, its pins and the guide's part.

### Confirmation

* Every finding in the PLAN's table is closed with the line changed, or
  recorded as not changed with the reason.
* Each code fix's test fails on a plant of today's code: C1 in
  `installer_ps_test.go` on the Windows test host, with a program held
  open, and its `install.sh` parity case; C2 and C3 in `selfupdate`'s
  tests, C3 under `-race`.
* G1's step is seen failing on an older toolchain with a known advisory;
  G2's path is confirmed by `actionlint` and by the step's file list.
* `scripts/check-docs.sh --links`, `--ids` and `--arch`, markdownlint,
  `go vet` for three targets, and `make gate` pass; every Example passes
  `go test -run '^Example'`.
* `apicheck` reports compatibility with `v1.17.0`; `v1.17.1`'s CI passes.

## Pros and Cons of the Options

### D1-A. Every finding

* Good, because it is what the owner asked for, and it closes v1.16.0's
  undocumented API.
* Bad, because it is the largest pass yet by line count.

### D1-B. Wrong, stale and broken only

* Good, because it is smaller.
* Bad, because the gaps are most of the findings, and "complete" was the
  request.

### D1-C. Markdown only

* Bad, because six doc comments are wrong about the code, and `go doc` is
  the first thing a program author reads.

### D2-A. Fix all three

* Good, because each published sentence stays true.
* Bad, because C1 needs the Windows test host, and C3 adds a lock.

### D2-B. Document now, fix later

* Bad, because it writes a false success (C1) and a panic (C2) down as
  behaviour.

### D2-C. C1 only

* Good, because it is the only one a user meets without writing code.
* Bad, because narrowing C3's doc withdraws a contract `v1.16.0`
  published.

### D3-A. Close both gaps

* Good, because SECURITY.md's claim then holds on every run.
* Bad, because the `go126` job runs a little longer.

### D3-B. Correct the claim

* Good, because it costs nothing in CI.
* Bad, because a standard-library advisory in 1.26.9 would go unseen
  until someone reran the check by hand.

### D4-A. Fix and add

* Good, because the fixed and new Examples run under `go test`, so they
  stay right.
* Bad, because the backends' Examples stay compile-only, since their
  output depends on the OS.

### D4-B. Fix only

* Bad, because v1.16.0's entry points keep no Example.

### D5-A. Release `v1.17.1`

* Good, because C1 reaches callers only through a tag.
* Bad, because a release costs the pin commit and a guide part.

### D5-B. Wait

* Bad, because a Windows uninstall keeps reporting false success.

## More Information

* The PLAN:
  [0027-PLAN-documentation-pass-after-v1-17-0.md](0027-PLAN-documentation-pass-after-v1-17-0.md).
* The comment citation rule this pass applies is 0024-MADR D2-A, as
  AGENTS.md "Records" states it.
* The release procedure is
  [0017-PLAN-verify-build-provenance-and-close-0015-open-items.md](0017-PLAN-verify-build-provenance-and-close-0015-open-items.md)'s,
  as 0025-PLAN S14 and 0026-PLAN U5 ran it.
