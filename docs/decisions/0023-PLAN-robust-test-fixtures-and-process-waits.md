---
status: proposed
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

*(Empty until approval.)*
