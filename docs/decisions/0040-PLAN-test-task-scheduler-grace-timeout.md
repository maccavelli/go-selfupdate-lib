---
status: complete
date: 2026-11-02
associated-madr: "0040-MADR-close-four-open-items-after-v1-23-1.md"
---
<!-- markdownlint-disable MD004 MD013 MD024 MD060 -->

# Implement D3: test Task Scheduler's `Stop` when its bound ends within the `RequestStop` grace

Associated MADR: [0040-MADR-close-four-open-items-after-v1-23-1.md](0040-MADR-close-four-open-items-after-v1-23-1.md)
(D3-A, as the owner chose on 2026-11-02).

## Goal

* **A fake test reaches `Stop`'s grace path and pins today's behaviour.** The path is `taskscheduler/lifecycle.go:197-200`, where the bound runs out within the grace and `Stop` returns `stillRuns(path)` without ending the task (MADR F3.1). No test reaches it at `4a9ead5` (F3.4).
* **The test is seen failing** with that path planted out, and with the re-enable after it planted out.
* **`Options.RequestStop`'s doc says its run is spent inside `Poll`'s timeout** (F3.3).
* **0037-PLAN says why the path was untested and that it now is.** Its two lines (`:191`, `:234`) are annotated, not rewritten.
* **No behaviour changes, and no release.** `Stop` is unchanged. The doc sentence ships with the next tag.

## Scope

### In scope

| Phase | What | Commit |
| :--- | :--- | :--- |
| T0 | the records: the 0040 MADR, its three PLANs and their index rows | records only, shared with the D2 and D4 PLANs |
| T1 | `selfupdate/service/taskscheduler/lifecycle_test.go` (one test), `selfupdate/service/taskscheduler/task.go` (one doc comment), 0037-PLAN's two annotations and its `date`, this PLAN's record, its row in `docs/README.md` | one commit; its push runs full CI |

### Out of scope

* **Any change to `Stop` or to another non-test line.** Ending the task anyway is D3-B, which the MADR rejects here. It needs a decision of its own.
* **Another fake read or seam** (D3-C). The test drives the path through `RequestStop`, which a program supplies.
* **`docs/architecture.md`.** No file is added or removed, so its package table is unchanged.
* **The live tests.** Their `RequestStop` returns at once (F3.4), and this plan does not change them.
* **A release.** The doc sentence reaches pkg.go.dev with the next tag, `v1.24.0` if the D2 PLAN follows.
* **Pushes,** which happen only on the owner's word, in the same turn.

## Rules

* T1 ends in one commit, `git commit --no-edit`, made by the owner, after `make pre-add-check FILES="selfupdate/service/taskscheduler/lifecycle_test.go selfupdate/service/taskscheduler/task.go"` exits 0.
* Every edit is a Python script under the scratchpad that asserts each old string occurs exactly once before replacing it.
* Every plant is made with `scripts/plant-copy.sh` on a scratch copy, never in the tree. Each copy is removed after its run.
* Gate and tool output goes to the scratchpad, with its exit status captured before any filter.
* Nothing staged names a host, an account or a real-machine path.
* **Deviations.** A step that finds anything outside its files, or a check that fails, stops. It is recorded here, dated, and in the MADR as an amendment when a fact or decision changes.

## Implementation Steps

### Phase T0: records

As [0040-PLAN-record-host-cleanup-in-0038.md](0040-PLAN-record-host-cleanup-in-0038.md) H0. One records commit holds the MADR and the three PLANs.

### Phase T1: the test, the doc and the annotations

Files: `selfupdate/service/taskscheduler/lifecycle_test.go`, `selfupdate/service/taskscheduler/task.go`, `docs/decisions/0037-PLAN-honour-contexts-in-backend-fakes-and-stop-commands.md`, this PLAN, and `docs/README.md` (this plan's row).

1. **Baseline.**
   * `git status --short` is clean.
   * `git diff --stat 4a9ead5 HEAD -- selfupdate/service/taskscheduler` prints nothing, so the code is what the MADR read.
   * `go test -count=1 -coverprofile=<scratch>/t1-before.out ./selfupdate/service/taskscheduler/` exits 0. Its profile has the block `lifecycle.go:199.5,200.1` with a count of 0, as probe G2 found.
2. **The test.** In `lifecycle_test.go`, after `TestStopTimeoutAtDefaultBound`. It needs no new import:

   ```go
   // TestStopBoundEndsInGrace: RequestStop runs inside Poll's timeout, so a
   // request that spends most of it leaves the grace to outlast the stop's
   // bound. Stop then reports that the task still runs, does not end it,
   // and enables it again
   // (docs/decisions/0040-MADR-close-four-open-items-after-v1-23-1.md
   // D3-A).
   func TestStopBoundEndsInGrace(t *testing.T) {
   	testproc.CheckNoLeak(t)
   	synctest.Test(t, func(t *testing.T) {
   		f := newFake()
   		ft := f.add(`\demo`, 4242)
   		ft.exitIn = -1 // never exits
   		const timeout, grace = 300 * time.Millisecond, 100 * time.Millisecond
   		var bound time.Time
   		task := testTask(t, f, Options{
   			Poll:      service.PollOptions{Timeout: timeout},
   			StopGrace: grace,
   			RequestStop: func(ctx context.Context, _ uint32) error {
   				bound, _ = ctx.Deadline()
   				time.Sleep(timeout - grace/2) // a request that takes most of the bound
   				return nil
   			},
   		})
   		start := time.Now()
   		err := task.Stop(context.Background(), "demo")
   		if !errors.Is(err, service.ErrTimeout) || !strings.Contains(err.Error(), `\demo still runs after it was stopped`) {
   			t.Fatalf("Stop = %v, want ErrTimeout saying the task still runs", err)
   		}
   		if want := start.Add(timeout); !bound.Equal(want) {
   			t.Errorf("RequestStop's deadline %s, want Poll's timeout from the start, %s", bound, want)
   		}
   		if took := time.Since(start); took != timeout {
   			t.Errorf("Stop took %s, want the bound, %s", took, timeout)
   		}
   		if want := []string{"enable false", "enable true"}; !slices.Equal(f.verbs(), want) {
   			t.Errorf("calls %q, want %q: the task is not ended, and is enabled again", f.verbs(), want)
   		}
   		if !ft.on || !f.alive[4242] || task.disabled[`\demo`] {
   			t.Errorf("enabled %t, program alive %t, recorded as disabled %t; want true, true, false",
   				ft.on, f.alive[4242], task.disabled[`\demo`])
   		}
   	})
   }
   ```

   This is probe G1's test (MADR F3.5). It adds the assertion on `RequestStop`'s deadline, which shows the request runs inside the bound that `Stop` set before it. It also drops the probe's logging.
3. **The doc.** `Options.RequestStop`'s comment in `task.go` (`:69-74`) gains, after "(0021-MADR D4).":

   ```go
   	// Its run is spent inside Poll's timeout, as StopGrace is: when a slow
   	// RequestStop leaves the grace to outlast that timeout, Stop returns
   	// service.ErrTimeout without ending the task, and enables the task
   	// again (0040-MADR D3-A).
   ```

   The comment is re-wrapped to the file's width only if `gofmt` or the linter asks.
4. **0037-PLAN.** Two annotations and the front matter:
   * **`:191`.** The reason is struck through, and a correction follows:

     ```markdown
       * That path is not covered by a test: ~~it needs the reads before the grace to spend the timeout, and `New` refuses a grace that is not shorter than it.~~ *Corrected on <the commit's date>: a `RequestStop` that spends most of the timeout reaches it too, since it runs inside the same bound ([0040-MADR-close-four-open-items-after-v1-23-1.md](0040-MADR-close-four-open-items-after-v1-23-1.md) F3.3). `TestStopBoundEndsInGrace` covers it ([0040-PLAN-test-task-scheduler-grace-timeout.md](0040-PLAN-test-task-scheduler-grace-timeout.md)).*
     ```

   * **`:234`.** The line keeps its text and gains, after one space:

     ```markdown
     *Covered since <the commit's date> by `TestStopBoundEndsInGrace` ([0040-PLAN-test-task-scheduler-grace-timeout.md](0040-PLAN-test-task-scheduler-grace-timeout.md)).*
     ```

   * **Front matter.** `date:` moves to the commit's date. `status:` stays `complete`.

   `<the commit's date>` is written as `YYYY-MM-DD`. A script asserts that `git diff -U0` of 0037-PLAN removes only the `date:` line and lines 191 and 234, and that the new line 234 starts with the old line's whole text.
5. **Seen failing.** Each plant runs `go test -count=1 -run '^TestStopBoundEndsInGrace$' ./selfupdate/service/taskscheduler/` in a copy from `scripts/plant-copy.sh`, which holds the new test:

   | Plant | File | OLD → NEW | Expected failure |
   | :--- | :--- | :--- | :--- |
   | P1, the grace path out | `lifecycle.go` | `'// The bound ran out within the grace: the task is not ended.\n\t\t\t\treturn stillRuns(path)'` → `'// planted'` | `Stop = <nil>, want ErrTimeout saying the task still runs` |
   | P2, the re-enable out | `lifecycle.go` | `'err = errors.Join(err, t.enableAgain(r, path))'` → `'_ = path'` | `calls ["enable false"], want ["enable false" "enable true"]: …` and `enabled false, program alive true, recorded as disabled true; want true, true, false` |

   Each must exit 1 with its failure. The unplanted tree must exit 0.

   **Pre-check, 2026-11-01.** The test above, with the plants as written, was run on scratch copies of `4a9ead5`, not in the tree (scratchpad script `p40d3.py`). Each OLD occurs once in `lifecycle.go`. HEAD exited 0. P1 and P2 each exited 1 with exactly the failures in the table. If `plant-copy.sh` refuses an OLD because it does not occur exactly once, that is a deviation. The plant is not reshaped to fit.
6. **Coverage after.** `go test -count=1 -coverprofile=<scratch>/t1-after.out ./selfupdate/service/taskscheduler/` exits 0. The block `lifecycle.go:199.5,200.1` has a count of at least 1, and the package's statement coverage is at least step 1's. In the pre-check, the block's line was `lifecycle.go:199.5,200.1 1 1`, and coverage went from 89.0% (probe G2) to 89.1%.
7. **Checks.**
   * `make gate`: every step rc=0, `overall=0`. `apicheck` says "compatible with v1.23.1", since only a comment changed in a non-test file.
   * `go test -race -count=1 ./selfupdate/service/taskscheduler/`, with `CGO_ENABLED=1`, on a copy of the tree on the Windows test host: `ok`. Windows is the backend's own platform, where the package builds its COM scheduler beside the fake. The copy is removed after the run.
   * `make markdownlint`, `make check-records`, `make docs-check`, and `scripts/check-docs.sh --ids` on the changed files: exit 0 each.
8. This PLAN's execution record, the plan `complete`, and its row to match. The owner commits. Before the owner is asked to push, the disclosure guard runs over the outgoing commits. The push runs full CI, which must pass every job.

## Verification

* **V1 (the path):** `TestStopBoundEndsInGrace` passes at the commit, under `-race` on this Mac and the Windows test host, and on every CI job.
* **V2 (seen failing):** P1 and P2 each fail as step 5's table says.
* **V3 (coverage):** `lifecycle.go:199.5,200.1` goes from 0 hits (step 1) to at least 1 (step 6).
* **V4 (no behaviour change):** `git diff` of the commit touches no non-test `.go` line but the comment of step 3. `make apicheck` reports compatibility with `v1.23.1`.
* **V5 (the records):** 0037-PLAN's two lines carry their annotations, by step 4's assertion. `make check-records` 0 findings.

## Rollout and Rollback

* **Rollout:** one commit on `main`, pushed on the owner's word. A program sees the doc sentence once a later tag carries it.
* **Rollback:** `git revert` of the commit. Nothing depends on the test.

## Execution record

### The owner's answers (2026-11-02)

* **Decisions:** "Accept D2-A, D3-A, D4-A (Recommended)". D3-A is the owner's decision.
* **Order:** "Records, then D4, D3, D2 (Recommended)". This plan runs second, after D4's, and before D2's release, which then carries T1 step 3's doc sentence.
* **Host tests:** "As written (Recommended)": T1 step 7's Windows run.

This plan stayed `proposed` until the owner approved its execution.

### Approval (2026-11-02)

The owner committed and pushed D4's phase as `b34bcd1`, and said "committed and pushed, proceed". This plan ran second, as the order says.

### T1: the test, the doc and the annotations (2026-11-02)

Files: `lifecycle_test.go`, `task.go`, 0037-PLAN, this PLAN and `docs/README.md` (this plan's row), as listed.

1. **Baseline.**
   * `git status --short` was clean, with `main` level with `origin` at `b34bcd1`.
   * `git diff --stat 4a9ead5 HEAD -- selfupdate/service/taskscheduler` printed nothing (exit 0).
   * `go test -coverprofile` exited 0: `coverage: 89.0% of statements`, and `lifecycle.go:199.5,200.1 1 0`.
2. **The test** went in before `TestStopRequestFails`'s comment, so after `TestStopTimeoutAtDefaultBound`, as step 2 gives it. It needed no new import.
3. **The doc sentence** went in after "(0021-MADR D4)." in `Options.RequestStop`'s comment, as step 3 gives it. `gofmt -l` printed nothing, and no re-wrap was needed.
4. **0037-PLAN:**
   * line 191's reason is struck through and corrected;
   * line 234 gains its pointer;
   * `date:` moved from 2026-10-27 to 2026-11-02, and `status:` stays `complete`.

   `<the commit's date>` became `2026-11-02`. `git diff -U0` of the file shows three one-line hunks, at lines 3, 191 and 234, and nothing else. The new line 234 starts with the old line's whole text.

   The test, the sentence and both annotations were read from this PLAN's own code blocks by the edit script (scratchpad `p40t1.py`). The script asserted each anchor and each old line before writing.
5. **Seen failing,** through `scripts/plant-copy.sh` (scratchpad `p40t1plant.py`), each copy removed after its run:
   * the tree: `--- PASS: TestStopBoundEndsInGrace`, exit 0;
   * P1: exit 1, `lifecycle_test.go:263: Stop = <nil>, want ErrTimeout saying the task still runs`;
   * P2: exit 1, `lifecycle_test.go:272: calls ["enable false"], want ["enable false" "enable true"]: the task is not ended, and is enabled again` and `lifecycle_test.go:275: enabled false, program alive true, recorded as disabled true; want true, true, false`.
6. **Coverage after:** exit 0, `coverage: 89.1% of statements`, and `lifecycle.go:199.5,200.1 1 1`.
7. **Checks:**
   * **`make gate`:** `overall=0`. Every step was rc=0:
     * `lint` 0 issues, on three GOOS;
     * `apicheck` "compatible with v1.23.1";
     * `fuzz` 11 targets clean;
     * `vuln` "No vulnerabilities found.";
     * `scripts` "all script tests passed";
     * `links` 921 in 108 files, 0 broken;
     * `arch` 20 rows, 0 differ;
     * `ids` 3 files, 0 findings.
   * **`make pre-add-check`** on the two `.go` files: "2 file(s) clean (gofmt, golangci-lint, go vet, go test, govulncheck)".
   * **The Windows test host:** `go test -race -count=1 ./selfupdate/service/taskscheduler/`, with `CGO_ENABLED=1`, on a copy of the tree: `ok … 1.720s`, `go-test-exit=0`. A first run narrowed to `-run TestStop` also passed, 15 `TestStop*` tests, `TestStopBoundEndsInGrace` among them. The whole package was then run as written. Afterwards the host holds 0 `sulib-*` copies.
   * **Documents:**
     * `make markdownlint`: 0 issues;
     * `make check-records`: 95 records, 0 findings;
     * `--links` on 0037-PLAN: 4 links, 0 broken;
     * `--ids` on the three changed files, with the deny list: 0 findings.

### Verification, at closing (2026-11-02)

* **V1 (the path):** `TestStopBoundEndsInGrace` passes in the tree (step 5), in the gate's race and shuffle steps on this Mac (step 7), and under `-race` on the Windows test host (step 7). The push's CI is the owner's to report.
* **V2 (seen failing):** P1 and P2 each exit 1 with the failures in step 5, as the pre-check of 2026-11-01 saw.
* **V3 (coverage):** `lifecycle.go:199.5,200.1` went from 0 hits to 1 (steps 1 and 6). Statement coverage went from 89.0% to 89.1%.
* **V4 (no behaviour change):** the only non-test `.go` change is the four comment lines in `task.go`. `make apicheck`: "compatible with v1.23.1".
* **V5 (the records):** 0037-PLAN's lines 191 and 234 carry their annotations (step 4). `make check-records`: 0 findings.

Not done here, as the plan's scope says: `Stop` is unchanged, no live test was run or changed, and there is no release. The doc sentence ships with the next tag.
