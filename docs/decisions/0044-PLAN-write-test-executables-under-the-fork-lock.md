---
status: complete
date: 2026-11-10
associated-madr: "0044-MADR-write-test-executables-under-the-fork-lock.md"
---
<!-- markdownlint-disable MD004 MD013 MD024 MD060 -->

# Implement test executables written under the fork lock: `testproc.WriteExecutable`, 74 sites, a source scan, and the race seen and removed

Associated MADR: [0044-MADR-write-test-executables-under-the-fork-lock.md](0044-MADR-write-test-executables-under-the-fork-lock.md)
(D1-A, D2-A, D3-A and D4-A, chosen by the owner on 2026-11-10).

## Goal

* **`testproc.WriteExecutable`** writes a file under `syscall.ForkLock`, with `os.WriteFile`'s signature (MADR D1-A).
* **The 74 test writes of MADR F4 outside a godoc example body use it** (D2-A), so no test write of an executable can race a fork.
* **`TestExecutableWritesUseWriteExecutable` holds the rule** (D3-A). It is seen to fail on the tree before the sites change, and on a planted copy.
* **The race is seen and removed** on the Linux test host, by a scratch program run with each writer (MADR Confirmation).
* **No release** (D4-A). The change reaches the next tag.

## Scope

### Preconditions

* The owner has accepted the MADR, with its options as proposed or as changed, and approved this PLAN.
* `HEAD` is `e62d77c`, or a later commit that touches no `_test.go` file and nothing in `internal/testproc/`.

### In scope

| Phase | What | Commit |
| :--- | :--- | :--- |
| E0 | this PLAN, its MADR, and their rows in `docs/README.md` | records only |
| E1 | `WriteExecutable` and its test, the scan, the 74 sites, `AGENTS.md`, `docs/architecture.md`, this PLAN's record | one commit |
| E2 | the race experiment, the runs on each host, the plants; this PLAN set to `complete` | records only |

Files that Phase E1 changes:

* `internal/testproc/testproc.go` and `internal/testproc/testproc_test.go`.
* The `_test.go` files of MADR F4 that hold the 74 sites. The scan of Phase E1 step 3 lists them.
* `AGENTS.md` and `docs/architecture.md`.

No `.go` file is added or removed, so the architecture table does not change. No import rule changes: `syscall` is in the standard library, and test files fall under the `module` rule, which allows this module.

### Out of scope

* MADR F7's production window, and any test write that sets no execute bit.
* The godoc example body of MADR F6, `ExampleNewFakeSource`.
* `v1.26.2`'s tag and CI run, which 0043-PLAN Deviation 3 handles.
* Any exported identifier. `make apicheck` must list no addition.
* Pushes and tags. These happen only on the owner's word in the same turn.

## Rules

* Mutation starts only after the owner approves this PLAN.
* The owner commits and pushes. The agent runs the checks and leaves the tree.
* A deviation stops the work: it is recorded here, dated, and the owner decides before it goes on.
* Every planted failure runs in a scratch copy (`scripts/plant-copy.sh`), never in the tree. The race experiment runs in a scratch directory outside the tree.
* Nothing committed names a host, an account or a real-machine path.

## Implementation Steps

### Phase E0: the records

1. This PLAN, its MADR and their two rows in `docs/README.md`. Run `make markdownlint`, `make check-records`, `make docs-check`, and `scripts/check-docs.sh --ids` on the three files. The owner commits.

### Phase E1: the helper, the scan and the sites

1. **`WriteExecutable`** in `internal/testproc/testproc.go`:

   ```go
   func WriteExecutable(name string, data []byte, perm os.FileMode) error {
       syscall.ForkLock.RLock()
       defer syscall.ForkLock.RUnlock()
       return os.WriteFile(name, data, perm)
   }
   ```

   * Its godoc says what it guards: a process forked while the file is open would inherit the write descriptor, and Linux would refuse to run the file with `ETXTBSY` until that child calls `exec` (go.dev/issue/22315). It cites `0044-MADR D1-A`.
   * The package comment names it beside `GoBuild` and `Start`.
2. **Its test,** `TestWriteExecutable`, in `testproc_test.go`:
   * the file holds the data, and on a platform other than Windows its mode is the one asked for;
   * a path in a missing directory returns an error that `errors.Is` matches to `fs.ErrNotExist`, as `os.WriteFile`'s does.
3. **Write the scan first, and see it fail on the tree.** `TestExecutableWritesUseWriteExecutable`, after `TestNoDirectFixtureBuild`:
   * **Which files.** Every `_test.go` file in the module, as `TestNoDirectFixtureBuild` walks them, skipping `testdata`, `.git` and `internal/testproc/`.
   * **What it refuses.** A call to `os.WriteFile` or `os.OpenFile` whose last argument is an integer literal that sets an execute bit, or an expression that contains one, such as `os.ModeSetuid|0o755`. It parses each file with `go/parser` and skips a call inside a function whose name begins `Example` (MADR F6).
   * **What it counts.** Calls to `testproc.WriteExecutable`, and to `WriteExecutable` inside package `testproc`. Fewer than 70 fails, and so do fewer than 100 files read, which catches a wrong root.
   * **What a failure says.** `path:line`, and the rule with its record (`0044-MADR D3-A`).
   * **Seeing it fail.** Run it before step 4. It must name exactly MADR F4's 74 sites, with `selfupdate/selfupdatetest/example_test.go:30` not among them. Record the count and the first and last lines of the output.
4. **The sites.** A Python script in the scratchpad takes the 74 `path:line` pairs from step 3's output:
   * it asserts each file's count, and that each line holds `os.WriteFile(` once;
   * it rewrites `os.WriteFile(` to `testproc.WriteExecutable(` on that line;
   * it drops a trailing `//nolint:gosec // an executable fixture` on a rewritten line, since the call it excused is gone;
   * it adds the import `github.com/maccavelli/go-selfupdate-lib/internal/testproc` where it is missing, and drops `os` where no use is left.

   It runs first on a scratch copy of the tree, where `gofmt -l` is empty after `gofmt -w` and `go vet ./...` passes for `GOOS=linux`, `darwin` and `windows` with `CGO_ENABLED=0`. Then it runs on the tree, and each changed file must be byte-identical to the copy's.
5. **Docs.**
   * `AGENTS.md`: the paragraph on `internal/testproc` (`:270-275`) gains one sentence. A test that writes an executable uses `testproc.WriteExecutable`, and `TestExecutableWritesUseWriteExecutable` holds it (`0044-MADR`).
   * `docs/architecture.md`: the `internal/testproc` bullet (from `:425`) names `WriteExecutable` and the scan.
6. **Checks.**
   * `make pre-add-check FILES="<every changed .go file>"`.
   * `make gate`, ending `overall=0`.
   * `make apicheck`: compatible with `v1.26.2`, and no addition.
   * `make markdownlint`, `make check-records`, `make docs-check` and `make disclosure-check`.
   * Record the output in this PLAN, in the same commit. The owner commits.

### Phase E2: the race, the hosts and the plants

1. **The race, seen and removed.** A scratch Go module outside the tree, copied to a scratch directory on the Linux test host and run there with go1.27.3:
   * `starters` goroutines run `/bin/true` in a loop;
   * `writers` goroutines, in a loop, write a copy of `/bin/true` to a new temporary file and run it, counting runs that fail with `syscall.ETXTBSY`;
   * a flag picks the writer: `os.WriteFile`, or the same write under `syscall.ForkLock.RLock()`, as step 1 of Phase E1 writes it;
   * it stops after a fixed number of runs, and prints the runs and the `ETXTBSY` count.

   With `os.WriteFile`, it must report `ETXTBSY` at least once. If it does not, the counts are raised and the run repeated, up to three times. With the lock, it must report none, for the same counts. Record both outputs. The scratch directory is removed afterwards.
2. **The hosts.** On a bundle of the Phase E1 tree:
   * `go test -count=1 ./...` on the macOS development host and the Linux test host;
   * `go test -count=1 -shuffle=on ./selfupdate/...` on the Linux test host, as CI runs it;
   * `go test -count=1 ./selfupdate/... ./internal/...` on the Windows test host.

   Record each result. The clones are removed afterwards.
3. **The plants.** With `scripts/plant-copy.sh`:

   | Plant | Test | Must fail with |
   | :--- | :--- | :--- |
   | `target_test.go:301`'s `testproc.WriteExecutable(` back to `os.WriteFile(` | `TestExecutableWritesUseWriteExecutable` | that line alone |
   | the scan's `Example` exemption removed | `TestExecutableWritesUseWriteExecutable` | `selfupdate/selfupdatetest/example_test.go:30` alone |
   | `WriteExecutable` writing with perm `0o600` | `TestWriteExecutable` | the mode check |

4. Set this PLAN to `complete`, with its row in `docs/README.md`. Run the checks, record every result here, and the owner commits.

## Verification

* `TestExecutableWritesUseWriteExecutable` passes on the changed tree. It fails on the tree before Phase E1 step 4, naming the 74 sites, and on Phase E2's two scan plants.
* `TestWriteExecutable` passes, and fails on its plant.
* Phase E2 step 1 reports `ETXTBSY` with `os.WriteFile` and none with the lock.
* `make gate` ends `overall=0`. `make apicheck` lists no addition. The four documentation checks are clean.
* Phase E2's host runs pass on macOS, Linux and Windows, and the push's CI passes.

## Rollout and Rollback

* **Rollout.** Nothing ships: only test code and `internal/testproc` change (D4-A). The next tag carries it.
* **Rollback.** Revert Phase E1's commit. No program is affected either way.

## Execution record

The owner accepted the MADR and approved this PLAN on 2026-11-10 ("Accept the 0044 madr approved to execute the plan"). In the same turn they approved the agent committing and pushing to `main` ("You are approved to commit and push to main"), so this PLAN's phases are committed and pushed by the agent, each after its checks pass.

### Phase E0

* The MADR is `accepted` and this PLAN `in-progress`, with their rows in `docs/README.md`. The records were committed with 0043-PLAN's Phase C5b as `c4bf7ca`. The pre-push guard over it exited 0, and it was pushed.

### Phase E1

* **Precondition.** `HEAD` was `c4bf7ca`, which changes only Markdown since `e62d77c`.
* **Step 1.** `WriteExecutable` is in `internal/testproc/testproc.go`, after `Build`, as the PLAN gives it, and the file imports `syscall`. Its godoc cites `0044-MADR D1-A`, and the package comment names it.
* **Step 2.** `TestWriteExecutable` passed. It checks the content, the mode `-rwxr-xr-x` where the platform is not Windows, and `fs.ErrNotExist` for a missing directory.
* **Step 3: the scan, seen failing.**
  * `TestExecutableWritesUseWriteExecutable` and its helper, `executableWrites`, are after `TestNoDirectFixtureBuild`'s helpers.
  * It skips `internal/testproc/` as a whole, so it counts `testproc.WriteExecutable` calls only. The PLAN's count of `WriteExecutable` inside package `testproc` would never apply.
  * Its first run stopped at the floor before it listed a site: `read 254 files and 0 testproc.WriteExecutable calls; …`. It now lists the sites first and then checks the floor, as 0043's scan does.
  * On the tree before step 4 it failed. It named 74 sites, and `selfupdate/selfupdatetest/example_test.go:30` was not among them. A comparison with the scratchpad inventory of MADR F4 differed by that line alone. The floor line read `read 254 files and 0 testproc.WriteExecutable calls; the module has more: is the root wrong?`. The first and last sites were:

    ```text
    internal/cmd/selfupdate-release/build_test.go:169: write an executable with testproc.WriteExecutable, so no fork inherits its descriptor (0044-MADR D3-A)
    selfupdate/verify/signednote/signednote_test.go:163: write an executable with testproc.WriteExecutable, so no fork inherits its descriptor (0044-MADR D3-A)
    ```

* **Step 4: the sites.**
  * The scratchpad's `convert_writes.py` took the 74 sites from step 3's output, in 39 files, and asserted each line before it wrote.
  * Its first two runs on the scratch copy stopped on an assertion before writing. Both cases were already in the PLAN's step:
    * four rewritten lines carry a `//nolint:gosec` with another reason, so the script drops only the exact `an executable fixture` comment, as the step says;
    * `signednote_test.go` used `os` only for its write, so the script drops that import, as the step says.

    gosec does not run on `_test.go` files (`.golangci.yml:395-401`), so the four kept comments change nothing.
  * On the scratch copy it gave `files 39 sites 74 nolint-dropped 27 imports-added 31 os-dropped ['selfupdate/verify/signednote/signednote_test.go']`.
    * `gofmt -w` re-sorted the new import in 19 files, and `gofmt -l` was empty after.
    * `go vet ./...` passed for `GOOS=linux`, `darwin` and `windows` with `CGO_ENABLED=0`.
    * The scan and `TestWriteExecutable` passed.
  * On the tree it gave the same line. After `gofmt -w` on the same 39 files, each was byte-identical to the copy's.
* **Step 5: docs.** `AGENTS.md`'s `internal/testproc` paragraph and `docs/architecture.md`'s `internal/testproc` bullet name `WriteExecutable` and the scan.
* **Step 6: checks.**
  * `make pre-add-check` on the 41 changed `.go` files: `go-precheck: 41 file(s) clean (gofmt, golangci-lint, go vet, go test, govulncheck).`
  * `make gate` gave `overall=0`, with every step `rc=0`:
    * gofmt, lint (`0 issues.`), vet, race, shuffle and tidy;
    * apicheck: `check-api-compat: compatible with v1.26.2`, and no line but the internal packages it ignores;
    * fuzz, vuln (`No vulnerabilities found.`), scripts, shellcheck and crossvet;
    * links: `1013 links in 117 files, 0 broken`;
    * arch: `22 rows … 0 differ, 0 without a row`;
    * ids: `43 files, 20 deny-list rules, 0 findings`.
  * `make markdownlint`: `0 issues`. `make check-records`: `104 records, 0 findings`. `make docs-check`: `1013 links in 117 files, 0 broken`, with 22 architecture rows and 0 differing. `make disclosure-check`: `748 files, 20 deny-list rules, 0 findings`.
* Committed as `cc21690` and pushed, after the pre-push guard over it exited 0. The push's CI run 42931313430 passed on its first attempt, with all 18 jobs green: `validate` on ubuntu-24.04, macos-15 and windows-2025, `go126`, `installer-containers`, and both release rehearsals with their identity legs. Docs run 42931313378 passed too.

### Phase E2

* **Step 1: the race, seen and removed.** The scratchpad's `etxtbsy` module was copied to a scratch directory on the Linux test host, a 4-CPU linux/amd64 machine. It was built and run there with go1.27.3. Each writer writes a copy of `/bin/true` with mode `0o755` and runs it, while 8 starter goroutines run `/bin/true` in a loop:

  ```text
  locked=false starters=8 writers=8 runs=4000 etxtbsy=402 other=0
  locked=false starters=8 writers=8 runs=16000 etxtbsy=1641 other=0
  locked=true starters=8 writers=8 runs=4000 etxtbsy=0 other=0
  locked=true starters=8 writers=8 runs=16000 etxtbsy=0 other=0
  ```

  With `os.WriteFile`, about one run in ten failed with `ETXTBSY`. Under `syscall.ForkLock.RLock()`, as `WriteExecutable` writes, none did, for the same counts, and no run failed any other way. The counts did not need raising.
* **Step 2: the hosts.** A bundle of `cc21690` was cloned on each test host, with the temporary directory inside the clone's directory:
  * the macOS development host, `go test -count=1 ./...` on the tree: `ok` for all 22 packages;
  * the Linux test host, `go test -count=1 ./...`: `ok` for all 22 packages. `go test -count=1 -shuffle=on ./selfupdate/...`: `ok` for all 14 packages;
  * the Windows test host, `go test -count=1 ./selfupdate/... ./internal/...`: `ok` for all 21 packages.

  The scratch directories on both test hosts were removed afterwards, the experiment's included, and each removal was confirmed.
* **Step 3: the plants.** The scratchpad's `e2_plants.py` made each copy with `scripts/plant-copy.sh`, ran the named test in it, and removed it. `git status --short` was empty afterwards.

  | Plant | Result |
  | :--- | :--- |
  | `target_test.go:301`'s `testproc.WriteExecutable(` back to `os.WriteFile(` | `TestExecutableWritesUseWriteExecutable` failed with that line alone: `selfupdate/target_test.go:301: write an executable with testproc.WriteExecutable, so no fork inherits its descriptor (0044-MADR D3-A)` |
  | the scan's `Example` prefix changed to one no function has | it failed with `selfupdate/selfupdatetest/example_test.go:30` alone |
  | `WriteExecutable` writing with `0o600` | `TestWriteExecutable` failed: `mode = -rw-------, want -rwxr-xr-x` |

* **Step 4.** This PLAN is `complete`, with its row in `docs/README.md`.
  * `make markdownlint`: `0 issues`. `make check-records`: `104 records, 0 findings`.
  * `make docs-check`: `1013 links in 117 files, 0 broken`, with 22 architecture rows and 0 differing.
  * `make disclosure-check`: `748 files, 20 deny-list rules, 0 findings`.
  * `scripts/check-docs.sh --ids` on this PLAN and `docs/README.md`: `2 files, 20 deny-list rules, 0 findings`.
* Committed and pushed after the pre-push guard over it exited 0.
