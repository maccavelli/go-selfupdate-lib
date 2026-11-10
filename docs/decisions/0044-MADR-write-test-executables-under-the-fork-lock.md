---
status: accepted
date: 2026-11-10
decision-makers: go-selfupdate-lib maintainers
consulted: "CI run 42922736233, the v1.26.2 tag's, attempt 1; every _test.go file at e62d77c; Go 1.27.3's syscall/exec_unix.go, syscall/forkpipe2.go and cmd/go/internal/test/test.go; go.dev/issue/22315"
informed: the owner, who chose on 2026-11-10 to re-run the tag's failed job and fix the race under this record (0043-PLAN Deviation 3)
---
<!-- markdownlint-disable MD004 MD013 MD024 MD060 -->

# Write a test's executable under `syscall.ForkLock`, through one `internal/testproc` helper, held by a source scan

## Context and Problem Statement

The `v1.26.2` tag's CI failed one job. On ubuntu-24.04, `go test -shuffle=on -count=2 ./...` failed `TestResolveTargetRealExecutableThroughSymlink/resolve` with `fork/exec …/link/demo: text file busy`. The tagged Go code had passed the same job on `a9f7c7d` (F2).

The cause is a race between Linux and Go that any test hits when it writes an executable and then runs it while another goroutine of the same test binary starts a process (F3). The test suite writes 75 files with an execute bit (F4). None of them guards against the race.

This record decides how a test writes an executable (D1), which writes change (D2), how the rule is held (D3), and whether the change is released (D4).

Facts are tagged by their source:

* **[read]:** the tree at `e62d77c`, the `v1.26.2` tag, and Go 1.27.3's source.
* **[observed]:** CI run 42922736233.
* **[derived]:** follows from the above.

### Facts

* **F1 [observed]. The failure.** Attempt 1 of CI run 42922736233, on the `v1.26.2` tag, failed `validate (ubuntu-24.04)` and passed its 17 other jobs. Its log ends:

  ```text
  -test.shuffle 1794353207770373151
  --- FAIL: TestResolveTargetRealExecutableThroughSymlink (0.00s)
      --- FAIL: TestResolveTargetRealExecutableThroughSymlink/resolve (0.01s)
          target_test.go:263: helper: fork/exec /tmp/TestResolveTargetRealExecutableThroughSymlinkresolve536157657/001/link/demo: text file busy:
  FAIL	github.com/maccavelli/go-selfupdate-lib/selfupdate	12.741s
  ```

* **F2 [read]. Not this release's code.**
  * `git diff --name-only a9f7c7d e62d77c` lists `CHANGELOG.md`, the migration guide and 0043-PLAN only. CI run 42916757677 on `a9f7c7d` passed `validate (ubuntu-24.04)`.
  * The test came in `df9f405` (`v1.26.0`; 0041-MADR Amendment A1). Of the CI runs since then, run 42922736233 is the only one that failed.
* **F3 [read]. The race (go.dev/issue/22315).**
  * `symlinkedCopy` (`selfupdate/target_test.go:285-306`) writes a copy of the test binary with `os.WriteFile(c.exe, in, 0o755)` (`:301`). Each of the test's three parallel subtests then starts it four times (`runResolveHelper`, `:310-327`).
  * `os.WriteFile` opens the file with `O_CLOEXEC`. A process that another goroutine forks while the file is open inherits the write descriptor. The descriptor closes only when that child calls `exec`.
  * Until then, Linux refuses to run the file with `ETXTBSY`, which Go reports as `text file busy`.
  * Go's `syscall.ForkLock` is the documented guard (`syscall/exec_unix.go:19-65`). Code that creates a descriptor holds it for reading, and "the fork itself is done holding ForkLock for writing". `acquireForkLock` (`syscall/forkpipe2.go:39-80`) keeps that promise: the first of a group of forks takes `ForkLock.Lock()`, and a waiting reader is let in between forks.
  * `syscall.ForkLock` is declared on every platform this module tests: `exec_unix.go:65` and `exec_windows.go:19`.
  * `cmd/go` meets the same race when it runs a test binary, and retries on `ETXTBSY` (`cmd/go/internal/test/test.go:1655-1715`).
* **F4 [read]. The writes.** A scratchpad scan of every `_test.go` file, parsing each call's arguments, found 75 `os.WriteFile` calls whose mode literal sets an execute bit, and no `os.OpenFile` call that does:

  | Where | Calls |
  | :--- | ---: |
  | files with no OS in their name | 47 |
  | `_linux_test.go` | 9 |
  | `_darwin_test.go` | 8 |
  | `_windows_test.go` | 11 |

  * Some write a file that a test then runs. Examples are `target_test.go:301`, `replace_native_test.go:117`, `e2e_running_test.go:152` and `internal/cmd/selfupdate-release/build_test.go:169`.
  * Some write a script that a shell the test starts runs in turn, such as the stubs at `installer_sh_test.go:81`, `:135` and `:162`. A retry at the test's own `exec` cannot reach that run.
  * Most write a target that is never run, such as `"old-bytes"`.
* **F5 [read]. Where a helper would live.**
  * `internal/testproc` is test support that only `_test.go` files import (0023-MADR). It already holds `GoBuild` and `Start`.
  * Its depguard rule, `internal-testproc`, allows the standard library only (`.golangci.yml:86-92`), and `syscall` is part of it.
  * `TestNoDirectFixtureBuild` and 0043's `TestLiteralPrefixWrapsUseWrap` are source scans in `internal/testproc` that walk the module's files.
* **F6 [read]. The example files.** Four of the 75 writes are in `example_test.go` files:
  * three are in helpers with no `testing.TB`, which return an error or panic: `selfupdate/example_test.go:434` and `selfupdate/cli/example_test.go:35` in `exampleUpdater`, and `selfupdate/verify/ghattest/example_test.go:57` in a test;
  * one is in the body of `ExampleNewFakeSource` (`selfupdate/selfupdatetest/example_test.go:30`). `go doc` and pkg.go.dev print that body as the example a reader copies, and a reader cannot import `internal/testproc`. The file it writes holds `demo v1.0.0\n`, which nothing runs.
* **F7 [read]. Production code is not in this record.** `selfupdate` runs a file it wrote in one place: a post-install probe runs the installed target (`probe.go:138`), after the session closed and renamed the staged file. A host program that forks in another goroutine while the staged file is still open could meet the same race. That window, and its remedy, belong to the library's API and its programs, not to the test suite, so this record leaves them out.

## Decision Drivers

* **CI is a release gate.** A tag's run must not fail on a race in the tests (F1).
* **Fix the cause, not each symptom.** A retry must reach every run, including one a shell makes (F4).
* **One place decides how a test writes an executable,** and a new test cannot bring the race back unnoticed.
* **Standard library only,** within `internal/testproc`'s import rule (F5).

## Considered Options

### D1: how a test writes an executable (F3, F4)

* **D1-A.** `testproc.WriteExecutable(name string, data []byte, perm os.FileMode) error` has `os.WriteFile`'s signature and result. It writes the file while holding `syscall.ForkLock.RLock()` from the open to the close. No process can fork while the descriptor is open, so none inherits it. A site changes only the function it calls (F6).
* **D1-B.** Each place that runs a written file retries on `ETXTBSY` until the test's deadline, as `cmd/go` does.
* **D1-C.** The test process never opens the file: a child process (`cp`, or a Go helper) writes it.

### D2: which writes (F4)

* **D2-A.** All 75 writes with an execute bit, on every platform, but the one in the body of a godoc `Example` function (F6).
* **D2-B.** Only the writes of a file that a test, or a process it starts, runs.
* **D2-C.** `target_test.go:301` alone.

### D3: how the rule is held (F5)

* **D3-A.** A source scan in `internal/testproc`, `TestExecutableWritesUseWriteExecutable`. It refuses an `os.WriteFile` or `os.OpenFile` in a `_test.go` file whose mode literal sets an execute bit, outside `internal/testproc` itself and outside the body of a function whose name begins `Example`. It names each file and line, and fails when it reads fewer files than a floor.
* **D3-B.** No scan: review alone.
* **D3-C.** A `forbidigo` rule refusing `os.WriteFile` in every test file.

### D4: the release

* **D4-A.** No release. Only `_test.go` files and `internal/testproc` change, so no program's build changes. The change reaches the next tag.
* **D4-B.** `v1.26.3`.

## Decision Outcome

Chosen options: "D1-A", "D2-A", "D3-A" and "D4-A", by the owner on 2026-11-10, as proposed ("Accept the 0044 madr approved to execute the plan").

* **Why D1-A.**
  * It removes the cause: the lock is the guard Go documents for exactly this window (F3).
  * It covers a run made by a shell the test starts, which D1-B cannot reach (F4).
  * It costs one lock held across a write. A fork waits for that write, at most the time to write one test binary.
  * D1-C needs a writer program on each platform, and a process start for each write.
* **Why D2-A.**
  * The godoc example keeps the standard call, because its body is documentation, and the file it writes is never run (F6).
  * The scan can hold a rule about the mode, but not one about whether some later code runs the file. D2-B needs a judgement at each site, which a new test can get wrong silently.
  * Holding the lock to write a file that is never run is harmless.
  * D2-C leaves the other writes of F4 open to the same race.
* **Why D3-A.** It follows 0043's `TestLiteralPrefixWrapsUseWrap`, and fails at once on a new site. D3-B lets a new test bring the race back. D3-C refuses the many writes that set no execute bit.
* **Why D4-A.** A program that requires this module does not compile its tests. `v1.26.2`'s tag stays as it is. Its failed job was re-run (0043-PLAN Deviation 3).

### Consequences

* Good, because no test write of an executable can race a fork in the same test binary.
* Good, because the scan refuses a new `os.WriteFile` of an executable in a test.
* Neutral, because a fork in a test waits while another test writes an executable.
* Neutral, because the 11 writes in `_windows_test.go` files take a lock that guards nothing there today. Windows has no `ETXTBSY`, but `syscall.ForkLock` exists there, so the code is the same on every platform.
* Bad, because 74 sites move from the standard `os.WriteFile` to a helper specific to this project.

### Confirmation

* **The race seen and removed.** A scratch Go program outside the tree is run on the Linux test host. It starts processes from several goroutines in a loop while others write a copy of a binary and run it. It counts `ETXTBSY`:
  * with `os.WriteFile`, it must report some within its budget;
  * with `WriteExecutable`'s locked write, it must report none in the same budget.
* **`WriteExecutable`'s own test** checks the content and the mode, and that it returns the error `os.WriteFile` would, for a missing directory.
* **The scan, seen failing:**
  * on the tree before the sites change, where it must name the 74 calls;
  * on a copy with one site turned back into `os.WriteFile`, where it must name that site alone.
* **The tests pass** on the macOS development host, the Linux test host and the Windows test host, and in CI, where `go test -shuffle=on -count=2` runs on Linux.
* **`make gate`** ends `overall=0`, and `make apicheck` lists no addition.

## Pros and Cons of the Options

### D1-A

* Good, because it closes the window the race needs.
* Good, because it covers a file a grandchild runs.
* Neutral, because a fork waits for a write.

### D1-B

* Good, because it is the approach `cmd/go` takes.
* Bad, because it cannot reach a file that a shell started by the test runs.
* Bad, because every run site needs it.

### D1-C

* Good, because the test process never holds the descriptor.
* Bad, because it needs a writer program, and a process start, for every write.

### D2-A

* Good, because a scan can hold the rule.
* Bad, because it changes 74 sites.

### D2-B

* Good, because it changes fewer sites.
* Bad, because each site is a judgement that no scan holds.

### D2-C

* Good, because it changes one line.
* Bad, because every other write in F4 stays open to the race.

### D3-A

* Good, because a new site fails at once, naming its line.

### D3-B

* Good, because there is no machinery.
* Bad, because the race can come back.

### D3-C

* Good, because it runs in lint.
* Bad, because it refuses every `os.WriteFile` in a test, not only an executable's.

### D4-A

* Good, because no program's build changes, so there is nothing to release.

### D4-B

* Bad, because a release would carry no change a program can see.

## More Information

* Plan: [0044-PLAN-write-test-executables-under-the-fork-lock.md](0044-PLAN-write-test-executables-under-the-fork-lock.md).
* Related:
  * [0043-PLAN-name-the-package-once-in-wrapped-errors.md](0043-PLAN-name-the-package-once-in-wrapped-errors.md) Deviation 3, which re-ran the tag's failed job and opened this record;
  * [0023-MADR-robust-test-fixtures-and-process-waits.md](0023-MADR-robust-test-fixtures-and-process-waits.md), which made `internal/testproc` the home of test fixtures;
  * [0043-MADR-name-the-package-once-in-wrapped-errors.md](0043-MADR-name-the-package-once-in-wrapped-errors.md) D6-A, whose scan D3-A follows;
  * [0041-MADR-refuse-symlink-invocation-on-every-platform.md](0041-MADR-refuse-symlink-invocation-on-every-platform.md) Amendment A1, which added the failing test.
* go.dev/issue/22315, the race in Go's issue tracker.
* **Out of scope:** the production window of F7, and any test write that sets no execute bit.
