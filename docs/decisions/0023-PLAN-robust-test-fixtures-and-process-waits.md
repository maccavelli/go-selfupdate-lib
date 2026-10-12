---
status: in-progress
date: 2026-10-11
associated-madr: "0023-MADR-robust-test-fixtures-and-process-waits.md"
---
# Implement fixture builds independent of git state and helper waits independent of machine load

Associated MADR: [0023-MADR-robust-test-fixtures-and-process-waits.md](0023-MADR-robust-test-fixtures-and-process-waits.md)

## Goal

* **No fixture build reads the git state around it** (MADR D1-A), and
  CI holds that under a stray `.git` (D4-A).
* **No helper wait fails for a slow machine, and none hides a crash**
  (D2-A).
* **`TestIdentity` checks the tool with the budget it ships** (D3-A).
* **`make gate` passes** on the Linux test host as it is configured, and
  on this Mac, for the code alone.

## Scope

### In scope

| Phase | What | Commit |
| :--- | :--- | :--- |
| S0 | this pair and its two `docs/README.md` rows | records only |
| S1 | measurements: the failure before the change, and first-start times | records only |
| S2 | `internal/testproc`, its tests, its depguard rule, its row | code, tests, config, docs |
| S3 | the six fixture builds through `GoBuild`, and the static test | tests |
| S4 | the six helper waits through `Start`, and `TestIdentity` | tests |
| S5 | CI's stray-`.git` step, and the documents | CI, docs |
| S6 | the runs that prove it, on the three hosts | records only |
| S7 | closing | records only |

### Out of scope

* **In-process waits, live tests' budgets, `writeSettled`, production
  timeouts** (MADR, "Out of this record").
* **The release tool's own fixture,** which is built in a repository on
  purpose.
* **The Linux test host's `.git` in its `TMPDIR`, and its `BASH_ENV`:**
  they are its owner's settings; the tests stop depending on them.
* **A release:** nothing exported or shipped changes.
* **Push,** which happens only on the owner's ask in the same turn.

## Rules

* **`main`, no branch.** The owner commits and pushes on `main`. An agent
  commits only on an explicit "commit to main" in that turn, with
  `git commit --no-edit`.
* **Each phase stops at a deviation** (AGENTS.md, Plan deviations).
* **Checks before each commit:** `make pre-add-check FILES="…"` on every
  changed Go file; `scripts/check-docs.sh --links` and `--ids`, and
  markdownlint, on every changed Markdown file; `make gate` ending
  `overall=0`. Where this Mac's load makes the gate fail on the timeouts
  this plan removes, until S4 lands, the gate runs on the Linux test host
  as in 0022's second plan (Q5), and the record says so.
* **Seen failing first:** every new test and check fails on a
  `scripts/plant-copy.sh` copy with one planted break, never by dirtying
  the tree; the record quotes the failing line.
* **Hosts:** this Mac; the Linux test host over ssh, in a new directory
  under `/var/tmp`, removed afterwards; the Windows test host for the
  Windows-only sites, as in 0021 and 0022. Nothing committed names a host,
  an account or a real absolute path.

## Implementation Steps

### Phase S0: records

1. `HEAD` is `773dd0b` or a records-only descendant, on `main`.
2. `docs/README.md` gains the two 0023 rows after 0022's.
3. On approval: this plan becomes `in-progress` with `### Approval`
   quoting the owner; the MADR becomes `accepted` with the chosen options.
4. Checks; commit.

### Phase S1: measurements

Nothing in the tree changes.

1. **The stray `.git`, before the change,** on this Mac, with `TMPDIR`
   set to a scratch directory holding an empty `.git`: `go test` of
   `./selfupdate/archive/`, `./selfupdate/` (`-run
   'Image|TestE2E'`), `./buildinfo/` and `./internal/cmd/selfupdate-release/`.
   The record keeps each package's result and its "error obtaining VCS
   status" lines. The same on the Linux test host, with its own `TMPDIR`
   and `BASH_ENV` as they are.
2. **First-start time** of a freshly built hello-world binary, three
   fresh copies each, first start and second, on this Mac (with its load
   average at the time), on the Linux test host and on the Windows test
   host.
3. **The helpers' own life bounds** read and recorded: that each helper
   behind the six waits exits on its own within its bound when nothing
   signals it.

### Phase S2: `internal/testproc`

1. `internal/testproc/testproc.go`: `GoBuild`, `Start`, `Proc` and its
   `Wait`, `Budget`, as MADR decision 1 says. Its package doc says it is
   imported only by tests, and why it exists. Standard library only.
   * `Start` runs `cmd.Wait` in one goroutine and keeps its result for
     `Proc.Wait`; it captures combined output in a bounded buffer
     (64 KiB), and a failure quotes the last 20 lines.
   * `Budget` takes `t.Deadline()`; the grace is 5 % of what is left, at
     least 1 s; without a deadline, 5 minutes.
2. `internal/testproc/testproc_test.go`, its own tests, using the test
   binary as the helper (a `TestMain` mode):
   * `TestStartReady`: a helper that writes the ready file after 300 ms
     is started, and `Wait` returns its exit;
   * `TestStartExitsFirst`: a helper that exits 3 before writing it is
     reported within 2 s, with "exit status 3" and its last output line;
   * `TestStartBudget`: an internal form of the wait with a 200 ms budget
     and a helper that never writes it: "not ready after";
   * `TestBudget`: the grace arithmetic for a deadline 100 s, 10 s and
     0.5 s away, and without one;
   * `TestGoBuildIgnoresParentGit`: a module in a directory whose parent
     holds an empty `.git` builds, and the binary runs.
3. **depguard:** `internal-testproc` (`$gostd`), after `banned`;
   `other-packages` excludes the package. AGENTS.md names the rule and
   says tests that build a fixture or wait on a helper use the package.
   `docs/architecture.md`: the tree, a table row (the `arch` step holds
   its counts), the rule count (18) and a bullet.
4. **Seen failing,** each a plant in a copy:
   * `GoBuild` without `-buildvcs=false`: `TestGoBuildIgnoresParentGit`
     fails with the VCS error;
   * `Start` without its exit check: `TestStartExitsFirst` fails on time
     (its own 2 s check), not on the message;
   * `Budget` without the grace: `TestBudget` fails;
   * a planted import of a non-standard package into `testproc.go`:
     depguard names `internal-testproc`.
5. Checks; commit.

### Phase S3: the fixture builds

1. The six sites of the MADR's first table call `testproc.GoBuild`, each
   keeping its environment (`GOOS`, `GOARCH`, `CGO_ENABLED`, `GOWORK`,
   `GOFLAGS`, `GOPROXY`) and flags (`-ldflags`, `-trimpath`).
2. `internal/testproc`'s `TestNoDirectFixtureBuild` parses every
   `_test.go` file under the module root, skipping `testdata`, and fails
   on an `exec.Command` or `exec.CommandContext` whose program is the
   literal `"go"` or a variable assigned from `exec.LookPath("go")`, and
   whose arguments hold the literal `"build"`, naming the file and line.
   It must find the module's `_test.go` files (a count above zero) and,
   before S3's step 1 lands, the six sites, so it is not vacuous.
3. **Seen failing:**
   * the reproduction of S1 step 1, now passing on this Mac and on the
     Linux test host with its own `TMPDIR`; and, on a copy with one site
     reverted to a direct build, that site's package fails again under
     the stray `.git`;
   * `TestNoDirectFixtureBuild` on that same copy names the site.
4. The macOS live site (`codesign`) runs with
   `SELFUPDATE_REQUIRE_CODESIGN=1` on this Mac; the Windows site
   (`installer_ps_test.go`) on the Windows test host.
5. Checks; commit.

### Phase S4: the helper waits and `TestIdentity`

1. The six sites of the MADR's second table call `testproc.Start`; their
   cleanups call `Proc.Wait` in place of `cmd.Wait`. The helpers'
   messages and their own life bounds are unchanged.
2. `TestIdentity`'s successful runs leave `Timeout` unset; "a hang"
   keeps `time.Second`.
3. **Seen failing:**
   * a copy where the E2E helper exits 9 before writing its ready file:
     `TestE2E*` fails at once with "exit status 9", where today's code
     waited 10 s and said "never signalled ready" (both timed);
   * a copy where `TestIdentity`'s hang case loses its own 1 s budget:
     the case then takes the 30 s default, which the record times, so the
     case is shown to rely on its explicit budget.
4. The Windows sites run on the Windows test host; `replace_native_test.go`
   and the E2E tests on all three hosts.
5. Checks; commit.

### Phase S5: CI and documents

1. `.github/workflows/ci.yml`, the Linux job: a step "fixture builds
   under a stray .git" after `go test`:
   `mkdir -p "$RUNNER_TEMP/stray/.git"` then
   `TMPDIR="$RUNNER_TEMP/stray" go test -count=1 ./selfupdate/... ./buildinfo/ ./internal/...`.
   `scripts/check-workflows.sh` and actionlint pass on it.
2. `docs/architecture.md`'s CI bullet names the step; AGENTS.md's
   pre-add section says how to run it by hand.
3. Checks; commit.

### Phase S6: the proof runs

1. **The Linux test host:** a clone at the S5 commit, `make gate` with
   the host's `TMPDIR` and `BASH_ENV` as they are, and `GATE_SKIP=ids`
   only (no deny list there; the step passes on this Mac): `overall=0`.
2. **This Mac:** `make gate`, with the load average recorded before and
   after. If it fails, the record names the step and the test, and a
   failure that is not one of the waits this plan changed is a deviation.
3. **The Windows test host:** the packages with Windows-only sites,
   `selfupdate`, `internal/cmd/selfupdate-release` and `internal/testproc`,
   cross-compiled and run.
4. **CI** on the pushed S5 commit, the new step included, after the
   owner's push.

### Phase S7: closing

1. `### Verification, at closing`, V1 to V6 with their evidence.
2. `### Closing`, `status: complete`, the index row. Commit, then push on
   the owner's ask.

## Verification

* **V1 (reproduced).** S1's record shows the four packages failing under
  a stray `.git`, and first-start times on three hosts.
* **V2 (helper).** Each `internal/testproc` test failed on its plant and
  passes on the three hosts.
* **V3 (builds).** The reproduction passes after S3; a reverted site fails
  it again; `TestNoDirectFixtureBuild` names that site.
* **V4 (waits).** A helper that exits first is reported at once with its
  status, timed against today's behaviour; `TestIdentity` uses the
  shipped budget.
* **V5 (gate).** `make gate` passes on the Linux test host as configured,
  and on this Mac, with the load stated.
* **V6 (CI).** CI passes on the pushed commits, the stray-`.git` step
  included.

## Rollout and Rollback

* **Rollout:** each phase is one commit on `main`, pushed on the owner's
  ask. No release follows.
* **Rollback:** `git revert` of any phase; the phases are tests, CI and
  records, so nothing a program uses changes.
* **A deviation in any phase** stops it.

## Execution record

### Approval (2026-10-11)

The owner committed the pair and its index rows, and said "committed,
proceed". The MADR is `accepted` with D1-A, D2-A, D3-A and D4-A.

### S0 (2026-10-11)

The owner committed the pair and its rows as `745328e`. This plan is
`in-progress`, the MADR `accepted`, here and in the index.

### S1 (2026-10-11)

**Step 1, a stray `.git`, before the change.**

On this Mac, `TMPDIR` set to a scratch directory holding an empty `.git`,
`go test -count=1`, at `745328e` (the tests are `773dd0b`'s):

```text
./selfupdate/archive/ -run '.': rc=1 fails=0 vcs_errors=1 last=FAIL
./selfupdate/ -run 'Image|TestE2E': rc=1 fails=4 vcs_errors=4 last=FAIL
./buildinfo/ -run '.': rc=1 fails=4 vcs_errors=3 last=FAIL
./internal/cmd/selfupdate-release/ -run '.': rc=0 fails=0 vcs_errors=0 last=ok  	github.com/maccavelli/go-selfupdate-lib/internal/cmd/selfupdate-release	181.565s
```

(`fails` counts `--- FAIL` lines, `vcs_errors` "error obtaining VCS
status" lines. `archive`'s failure is in `TestMain`, so no test line is
printed.)

On the Linux test host, a clone at `773dd0b`, run from a login shell with
the host's own `TMPDIR` (which holds an empty `.git`) and `BASH_ENV`:

```text
head=773dd0b tmpdir_has_git=yes bash_env_set=yes
./selfupdate/archive/ -run '.': rc=1 fails=0 vcs_errors=1
./selfupdate/ -run 'Image|TestE2E': rc=0 fails=0 vcs_errors=0
./buildinfo/ -run '.': rc=0 fails=0 vcs_errors=0
./internal/cmd/selfupdate-release/ -run '.': rc=0 fails=0 vcs_errors=0
```

Only `archive` fails there, as in 0022's gate runs; why the image,
E2E and `buildinfo` builds do not is [unverified]. Every fix of S3 covers
both behaviours.

**Step 2, first start of a fresh binary** (a hello-world, `-buildvcs=false`,
three fresh copies, first start then second, bash's `time`):

| Host | Load average | First starts | Second starts |
| :--- | :--- | :--- | :--- |
| this Mac | 44.24, 38.72, 27.40 | 4.284 s, 1.499 s, 0.808 s | 0.010 s, 0.067 s, 0.055 s |
| the Linux test host | 0.29, 0.23, 0.40 | 0.003 s, 0.003 s, 0.003 s | 0.002 s, 0.003 s, 0.002 s |
| the Windows test host | — | 0.026 s, 0.022 s, 0.021 s | 0.036 s, 0.019 s, 0.037 s |

A first start on this Mac, under that load, takes up to 4.3 s of a 10 s
budget before the helper runs a line; the second start of the same file
does not. The helper waits each start a freshly copied binary.

**Step 3, the helpers' own life bounds,** read:

* the E2E helper (`serve`, `e2e_running_test.go:40-52`) returns after 60 s
  without its done file;
* the other five waits start a copy of the test binary with
  `SELFUPDATE_NATIVE_HELPER=1`, whose `nativeHelper`
  (`replace_native_test.go:34-49`) returns after 15 s without its done
  file.

So a helper that hangs alive ends on its own, well before a test's
deadline.

**Host cleanup.** The Linux run's directory and the timing copies on both
hosts were removed; 0 `selfupdate-s1` directories remain there.

**Checks (S0 and S1).** On the three records: `scripts/check-docs.sh
--links` `98 links in 3 files, 0 broken`; `--ids` `3 files, 16 deny-list
rules, 0 findings`; markdownlint `0 issues` on `docs/README.md`. With this
Mac's load average above 40, `make gate` ran on the Linux test host as the
Rules allow: a clone at `773dd0b` with the diff applied (three files,
SHA-256 matched), `env -u BASH_ENV`, a clean `TMPDIR`, `GATE_SKIP=ids`;
every other step `rc=0`, `overall=0`.

### S2 (2026-10-11)

The owner committed S0 and S1 as `d396052`, and said "proceed".

**Files.** New: `internal/testproc/testproc.go`,
`internal/testproc/testproc_test.go`. Changed: `.golangci.yml`,
`AGENTS.md`, `docs/architecture.md`.

**As built:**

* `GoBuild`, `Start`, `Proc.Wait` and `Budget`, as step 1 says. The
  unexported forms are `startWithin` (Start with an explicit budget) and
  `budgetAt` (the arithmetic, with the clock passed in): revive's
  `confusing-naming` refused `start` and `budget` beside `Start` and
  `Budget`. `Start` kills a helper whose budget runs out, and waits for
  it, before failing the test.
* The tests use the test binary as the helper, through
  `TESTPROC_HELPER`'s three modes. `TestBudget`'s cases: 100 s left gives
  95 s; 10 s gives 9 s (the 1 s floor); 0.5 s gives 0; no deadline gives
  5 minutes.
* depguard: `internal-testproc`, `$gostd`, after `internal-winproc`;
  `other-packages` excludes the package. AGENTS.md's rule list, and a
  paragraph after the gate's that says when a test uses the package.
  `docs/architecture.md`: the tree, a table row (1 and 1), the rule count
  (18) and a bullet.

**Seen failing, on this Mac,** each a `scripts/plant-copy.sh` copy of
`testproc.go`:

| Plant | Failing line |
| :--- | :--- |
| `GoBuild` without `-buildvcs=false` | `testproc_test.go:157: go build -o …/fixture.bin . in …: exit status 1`, with `error obtaining VCS status: exit status 128` |
| `Start` without its exit check | `testproc_test.go:86: start = testproc.test not ready after 30.001s` (the test took 30 s, where the tree reports the exit in 0.02 s) |
| `Budget` without the grace | `testproc_test.go:125: budgetAt(1m40s left, deadline true) = 1m40s, want 1m35s`, and the 10 s and 0.5 s cases |
| `golang.org/x/mod/semver` imported into `testproc.go` | `internal/testproc/testproc.go:17:2: import 'golang.org/x/mod/semver' is not allowed from list 'internal-testproc' (depguard)` |

**Checks so far.** On the tree: the package's five tests pass (`ok …
1.909s`). `make pre-add-check FILES="<the 2 Go files>"`: its first run
failed on the two revive findings above; after the rename, `go-precheck:
2 file(s) clean`.

### Deviation D1 (2026-10-11): the table check cannot see an uncommitted package

* **Found.** `scripts/check-docs.sh --arch docs/architecture.md`, with the
  new row in place: `docs/architecture.md:130: internal/testproc: no
  tracked .go file in that directory`, exit 2. `--arch` counts
  `git ls-files` (`check-docs.sh`, `tracked_go`), which lists tracked
  files only, so the gate's `arch` step fails for any change that adds a
  package, until its files are committed or staged. The `ids` step, by
  contrast, already reads the changed and the untracked files
  (`gate.sh:21`). The gap is 0022's check's, not new code's; S2 is the
  first phase since 0022 to add a package.
* **Recommended:** `--arch` counts tracked files and untracked ones that
  `.gitignore` does not exclude (`git ls-files --cached --others
  --exclude-standard`), so the table describes the tree as it will be
  committed. Cost: one line in `check-docs.sh`, a `check-docs_test.sh`
  case (an untracked file counts, an ignored one does not), and its
  fail-first run. Files added to S2: `scripts/check-docs.sh`,
  `scripts/check-docs_test.sh`.
* **Alternative:** stage the new files (`git add`) before each gate run.
  Cost: no code; but the gate's verdict then depends on what is staged,
  and every later phase that adds a package meets the same failure.
* **Doing nothing:** S2's gate fails on `arch` until the package is
  committed, so the rule "gate before commit" cannot be met for it.

### Deviation D1, resolution (2026-10-11)

* **Decision (the owner, "option 1"):** `--arch` counts tracked files and
  untracked ones `.gitignore` does not exclude.
* **As built:** `tracked_go` lists `git ls-files -z --cached --others
  --exclude-standard -- '*.go'`; the header comment says so; the error
  for an empty directory reads "no .go file in that directory".
  `check-docs_test.sh`'s fixture repository gains a committed
  `.gitignore` naming `pkg/ignored.go`, that file, and an untracked
  `pkg/sub/new.go`: the matching table now counts `pkg/sub` as 2 and 0, and
  a new case, `untracked.md`, which leaves the untracked file out, fails
  with `docs/untracked.md:8: pkg/sub: table 1/0, tree 2/0`. `17 passed, 0
  failed`.
* **Seen failing:** a `plant-copy.sh` copy of `check-docs.sh` with the old
  tracked-only listing, run through the test's `SCRIPT`: `FAIL arch good:
  … pkg/sub: table 2/0, tree 1/0`, `FAIL arch drift`, and `FAIL arch
  untracked: rc=0 … 0 differ`; `14 passed, 3 failed`.
* **On the tree:** `check-docs: 16 rows in docs/architecture.md, 0 differ`,
  with `internal/testproc` still uncommitted. `shellcheck` on both
  scripts: clean.
* **Records:** 0022-MADR D2 carries an annotation; its decision stands.

### S2, checks (2026-10-11)

* `internal/testproc`'s five tests, cross-compiled: `PASS` on the Windows
  test host, and on the Linux test host from a login shell with its own
  `TMPDIR` (which holds an empty `.git`) and `BASH_ENV`.
  `TestGoBuildIgnoresParentGit` passes on both.
* `make pre-add-check FILES="<the 2 Go files>"`: `go-precheck: 2 file(s)
  clean`.
* On the changed Markdown: links and identifiers clean.
* `make gate` on this Mac, load average 12.32 before and 50.90 after:
  every step `rc=0`, `apicheck` `compatible with v1.15.1`, `arch` `16
  rows in docs/architecture.md, 0 differ` with the package still
  uncommitted, `overall=0`.

### S3 (2026-10-11)

The owner committed S2 and D1 as `599ae94`, and said "proceed".

**Files.** Changed: `internal/testproc/testproc.go`,
`internal/testproc/testproc_test.go`, `selfupdate/archive/main_test.go`,
`selfupdate/imageverify_test.go`, `selfupdate/e2e_running_test.go`,
`buildinfo/stamp_test.go`, `selfupdate/codesign/live_darwin_test.go`,
`internal/cmd/selfupdate-release/installer_ps_test.go`. No file is added.

**As built:**

* **`testproc.Build`**, added: `GoBuild` for a caller with no
  `testing.TB`, returning the error. `archive`'s fixtures are built in
  `TestMain`, and the PowerShell stand-ins under a `sync.Once`, so neither
  has a `t` to fail. `GoBuild` wraps it. MADR decision 1 named `GoBuild`
  alone.
* **The six sites** keep their environments and flags: `archive` and the
  stand-ins call `Build`; the image verifier, the E2E helper (with
  `-ldflags`), `buildinfo`'s stamp test (with `GOFLAGS=-mod=mod`,
  `GOPROXY=off` and `-ldflags`) and the `codesign` live fixture call
  `GoBuild`. The stand-ins' `-o` is now an absolute path in their
  directory, where it was relative to it. `os/exec` imports that no
  longer serve a build were removed from `archive` and the image
  verifier's tests.
* **`TestNoDirectFixtureBuild`**, in `testproc_test.go`: it walks the
  module from `../..`, skipping `testdata` and `.git`, parses each
  `_test.go` file, and reports an `exec.Command` or `exec.CommandContext`
  whose program is the literal `"go"`, or a variable assigned from
  `exec.LookPath("go")` in that file, with a literal `"build"` argument.
  It fails when it reads no `_test.go` file.
* **Before step 1, on the tree at `599ae94`,** it reported exactly the six
  sites, so it is not vacuous:

```text
buildinfo/stamp_test.go:65:13
internal/cmd/selfupdate-release/installer_ps_test.go:535:11
selfupdate/archive/main_test.go:90:9
selfupdate/codesign/live_darwin_test.go:59:9
selfupdate/e2e_running_test.go:81:9
selfupdate/imageverify_test.go:27:9
```

**The stray `.git`, after the change** (S1 step 1's reproduction, on the
tree):

On this Mac:

```text
./selfupdate/archive/ -run '.': rc=0 fails=0 vcs_errors=0
./selfupdate/ -run 'Image|TestE2E': rc=0 fails=0 vcs_errors=0
./buildinfo/ -run '.': rc=0 fails=0 vcs_errors=0
./internal/cmd/selfupdate-release/ -run '.': rc=0 fails=0 vcs_errors=0
```

On the Linux test host, a clone at `d396052` with the diff applied (15
files, SHA-256 matched), from a login shell with its own `TMPDIR` (which
holds an empty `.git`) and `BASH_ENV`: every package `rc=0`, 0 VCS
errors, `internal/testproc` included; before the change, `archive` failed
there.

**Seen failing (step 3),** a `scripts/plant-copy.sh` copy with
`buildinfo/stamp_test.go`'s build put back to a direct `go build`:

* under the stray `.git` on this Mac: `--- FAIL: TestStampedBinary`,
  `TestStampedBinary/release` and `/unstamped`, each with `error obtaining
  VCS status: exit status 128`;
* `TestNoDirectFixtureBuild`: `testproc_test.go:209:
  buildinfo/stamp_test.go:67:13: build the fixture with testproc.GoBuild
  or testproc.Build (0023-MADR D1-A)`.

**Step 4, the platform-only sites:**

* `codesign`, on this Mac, `SELFUPDATE_REQUIRE_CODESIGN=1 -run TestLive`:
  `TestLiveSignAdHoc` and `TestLiveCheckerExitCodes` `PASS`, plainly and
  under the stray `.git`; `TestLiveSignIdentity` skips, as before, with no
  signing identity set.
* The Windows test host, a clone at `d396052` with the diff applied (15
  files, SHA-256 matched), Go 1.27.2: `-run '^TestInstallPs1$'
  ./internal/cmd/selfupdate-release/` `ok` in 24.488 s (the stand-ins
  built and used); `-run 'Image|TestE2E' ./selfupdate/` `ok`;
  `./internal/testproc/` `ok`.

**Checks.** Vet for darwin, linux and windows: clean. `make
pre-add-check FILES="<the 8 Go files>"`: `go-precheck: 8 file(s) clean`.
`make gate` on this Mac, load average 11.93 before and 60.76 after: every
step `rc=0`, `arch` `0 differ`, `overall=0`.

**Host cleanup.** Both hosts' run directories were removed.

### S4 (2026-10-11)

The owner committed S3 as `014eec1`, and said "proceed".

**Files.** Changed: `selfupdate/e2e_running_test.go`,
`selfupdate/lifecycle_windows_test.go` (two sites),
`selfupdate/replace_native_test.go`, `selfupdate/journal_windows_test.go`,
`selfupdate/cleanup_busy_windows_test.go`,
`internal/cmd/selfupdate-release/identity_test.go`. No file is added.

**As built:**

* Each of the six waits calls `testproc.Start(t, cmd, ready)` in place of
  `cmd.Start` and its polling loop; each cleanup, and `helperService.Stop`
  and `runningCopy.stop`, call `Proc.Wait`. `helperService` and
  `runningCopy` hold a `*testproc.Proc` where they held the `*exec.Cmd`.
  `TestNativeReplaceRunningCopy`, which waited twice (explicitly, then in
  its defer), now gets the same result from both.
* The helpers, their ready and done files, and their own life bounds are
  unchanged. `lifecycle_windows_test.go` no longer imports `time`.
* `TestIdentity`'s shared input leaves `Timeout` unset, with a comment;
  "a hang" sets its own 1 s, as before.

**Seen failing (step 3), on this Mac:**

| Plant | Run | Result |
| :--- | :--- | :--- |
| the E2E helper's `serve` exits 9 before its ready file, on the tree | `-run '^TestE2EUpdateRunningCopy$'` | `--- FAIL (0.80s)`: `e2e_running_test.go:281: demo exited (exit status 9) before it was ready, after 417ms` |
| the same plant on `014eec1` (`git archive`, before this phase) | the same | `--- FAIL (10.36s)`: `e2e_running_test.go:293: the running copy never signalled ready` |
| `TestIdentity`'s "a hang" without its own 1 s | `-run '^TestIdentity$'` | `--- FAIL: TestIdentity/a_hang (30.00s)`: `identity: relay-darwin-arm64 hang did not finish within 30s; want "did not finish within 1s"` |

The first two rows are V4: a crash is reported in under half a second,
with its status, where the old wait spent its whole budget and named the
wrong cause. The third shows "a hang" depends on its own budget, so the
shared default does not weaken it.

**The hosts (step 4),** a clone at `d396052` with the diff applied (20
files, SHA-256 matched):

* the Windows test host: `-run 'TestReplaceBeforeStopRunningImage|TestKeepPreviousRunningImage|TestNativeReplaceRunningCopy|TestWindows|TestE2E'
  ./selfupdate/`: 19 tests `PASS`, among them all five Windows-only
  sites' tests and the E2E tests; `TestIdentity` `ok`;
* the Linux test host, its own `TMPDIR` and `BASH_ENV`:
  `TestNativeReplaceRunningCopy` and the four `TestE2E*` `PASS`;
  `TestIdentity` `ok`;
* this Mac: the same `selfupdate` tests and `TestIdentity` (4.35 s) `ok`.

**Checks.** Vet for darwin, linux and windows: clean. `make
pre-add-check FILES="<the 6 Go files>"`: `go-precheck: 6 file(s) clean`.
`make gate` on this Mac, load average 45.69 before and 53.87 (1 min),
63.51 (5 min) after, the conditions in which it failed on 2026-10-11:
every step `rc=0`, `overall=0`.

**Host cleanup.** Both hosts' run directories were removed.

### S5 (2026-10-11)

The owner committed S4 as `b5670d5`, and said "proceed".

**Files.** Changed: `.github/workflows/ci.yml`, `docs/architecture.md`,
`AGENTS.md`.

* **CI:** a step "fixture builds under a stray .git" in the `validate`
  job, after `go test -shuffle`, `if: runner.os == 'Linux'`, under bash:
  `mkdir -p "$RUNNER_TEMP/stray/.git"`, then `TMPDIR="$RUNNER_TEMP/stray"
  go test -count=1 ./selfupdate/... ./buildinfo/ ./internal/...`. Its
  comment cites MADR D4-A.
* `docs/architecture.md`'s "Linux also" CI bullet names the step.
  AGENTS.md's `internal/testproc` paragraph names
  `TestNoDirectFixtureBuild` and gives the step's command to run by hand.

**The step's command, on the Linux test host:**

* on this tree (a clone at `d396052` with the diff applied, 21 files,
  SHA-256 matched), with a `RUNNER_TEMP` of its own: all 16 packages
  `ok`, `step_rc=0`;
* **seen failing,** on the tree before S3 (a clone at `d396052`, no
  diff): `FAIL github.com/maccavelli/go-selfupdate-lib/selfupdate/archive`
  with `error obtaining VCS status: exit status 128`, `step_rc=1`. As in
  S1, only `archive` fails on that host; the step fails all the same.

**Checks.** `scripts/check-workflows.sh` with `expressions`,
`permissions` and `pins` on `ci.yml`: each exit 0; `actionlint` v1.7.12:
clean. On the two documents: links, identifiers and markdownlint clean.
`make gate` on this Mac, load average 5.11 before and 38.46 after: every
step `rc=0`, `overall=0`.
