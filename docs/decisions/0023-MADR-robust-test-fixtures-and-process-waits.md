---
status: accepted
date: 2026-10-11
decision-makers: go-selfupdate-lib maintainers
consulted: "the tree at 773dd0b; make gate runs on this Mac and on a Linux test host, 2026-10-11 (0022-PLAN-error-chain-and-process-liveness.md, Q5's checks); a reproduction in the session scratchpad; Go 1.27.2's src/internal/testenv/exec.go"
informed: whoever runs make gate or CI for this module
---
# Make the tests' fixture builds independent of the directory's git state, and their helper-process waits independent of machine load

## Context and Problem Statement

On 2026-10-11, `make gate` could not pass for a change that touched only
Markdown, on two machines, for two reasons that have nothing to do with
the code under test
([0022-PLAN-error-chain-and-process-liveness.md](0022-PLAN-error-chain-and-process-liveness.md),
"Checks on the pin commit"):

* on this Mac, under load, tests that start a freshly built helper ran
  out of fixed time budgets;
* on a Linux test host whose temporary directory holds an empty `.git`,
  every test that builds a fixture there failed to stamp VCS
  information.

The owner asked for a detailed, comprehensive record and plan to fix and
improve these tests. A gate that fails for the machine instead of the code
costs a release a detour, and teaches whoever runs it to distrust a red
step.

**Facts [observed 2026-10-11, `main` at `773dd0b`]:**

* **Six tests build a Go fixture with `go build` in a temporary
  directory**, with Go's default `-buildvcs=auto`:

  | Site | Builds | Directory |
  | :--- | :--- | :--- |
  | `selfupdate/archive/main_test.go:90` (`TestMain`'s `buildFixture`) | three fixture programs | `os.MkdirTemp` |
  | `selfupdate/imageverify_test.go:27` (`buildFixture`) | image fixtures per platform | a test's temporary directory |
  | `selfupdate/e2e_running_test.go:81` (`buildHelper`) | the running-copy helper, two versions | `t.TempDir()` |
  | `buildinfo/stamp_test.go:65` | three stamped programs | a test's temporary directory |
  | `selfupdate/codesign/live_darwin_test.go:59` | the signing fixture (live, macOS) | a test's temporary directory |
  | `internal/cmd/selfupdate-release/installer_ps_test.go:535` | the PowerShell stand-ins (Windows) | `os.MkdirTemp` |

  The release tool's own fixture (`helpers_test.go`, `buildFixture`) is
  built by the tool's production `build` inside a git repository the test
  creates (`makeFixtureRepo`); its VCS stamp is the point, and git finds
  that repository first.
* **`-buildvcs=auto` reads the nearest `.git` above the build directory.**
  A `.git` that `git status` cannot read fails the build with "error
  obtaining VCS status: exit status 128" [measured]. Reproduced with a
  `TMPDIR` holding an empty `.git`, in the session scratchpad:
  * `selfupdate/archive`: `TestMain` failed, so no test ran;
  * `selfupdate`, the image-verifier tests: 1 failure;
  * `buildinfo`, the stamp tests: 4 failures;
  * `selfupdate`, `TestE2E*`: 3 failures;
  * `internal/cmd/selfupdate-release`, `TestIdentity` and `TestBuild*`:
    pass, since they build in their own repository.

  On the Linux test host only `selfupdate/archive` failed, and the fuzz
  step that lists its targets; why the others passed there is
  [unverified]. None of these fixtures needs VCS information: no test
  reads it from them.
* **Six waits start a helper process and poll for a "ready" file until
  a fixed deadline,** and none of them notices the helper exiting first:

  | Site | Budget | Failure text |
  | :--- | :--- | :--- |
  | `selfupdate/e2e_running_test.go:112` (`startCopy`) | 10 s | "the running copy never signalled ready" |
  | `selfupdate/lifecycle_windows_test.go:99` | 10 s | "helper did not start" |
  | `selfupdate/lifecycle_windows_test.go:181` | 10 s | "helper did not start" |
  | `selfupdate/replace_native_test.go:84` | 5 s | "helper did not start" |
  | `selfupdate/journal_windows_test.go:45` | 10 s | "helper did not start" |
  | `selfupdate/cleanup_busy_windows_test.go:219` | 10 s | "helper did not start" |

  A helper that crashes before it is ready costs the whole budget, then
  reports "never signalled ready" instead of its exit status. Each helper
  bounds its own life (`e2e_running_test.go:46`, 60 s;
  `replace_native_test.go:40`, 15 s), so none outlives the test.
* **`TestIdentity` budgets 5 s for each run that should succeed**
  (`identity_test.go:50`), where the code under test defaults to
  `identityTimeout`, 30 s (`identity.go:22`); its "a hang" case sets 1 s
  on purpose. The 5 s is the test's choice, not the tool's.
* **A first start is slow under load.** On this Mac at load average 54 to
  77 (10 cores; another workload's tests, a management daemon and
  Spotlight), a fresh hello-world binary took 0.86 s and 1.52 s to start
  the first time, and 0.05 s after [measured]. The three tests above
  failed on their budgets there, alone as well as in the gate, at code
  that CI had passed twice.
* **The Go project bounds a test's subprocesses by the test's deadline:**
  `internal/testenv.CommandContext` uses `t.Deadline()`, keeps a grace
  period of 5 % of the remaining time, and scales it by
  `GO_TEST_TIMEOUT_SCALE` [read: Go 1.27.2,
  `src/internal/testenv/exec.go`].
* **Out of this record:** in-process waits on goroutines and channels
  (`stream_test.go`, `confirmer_test.go` and others), where a slow machine
  only delays a scheduler; the live tests' budgets, which wait on real
  service managers and are tuned there; `writeSettled`'s 2 s, which
  mirrors the installer's own bound; production timeouts.

## Decision Drivers

* **A check fails for the code, not for the machine.** A red step is a
  defect to find, not weather.
* **No check is loosened.** What a test asserts stays asserted; a budget
  that guards a setup step is not the property under test.
* **A failure says what happened:** a helper that exited says so, with its
  status and output, at once.
* **One way, written once,** and held, so a new test does not reintroduce
  either fault.
* **No new module.** Test support uses the standard library.

## Considered Options

### D1: fixture builds and the directory's git state

* **D1-A. A test-support package, `internal/testproc`, whose `GoBuild`
  always passes `-buildvcs=false`; every fixture build uses it; a static
  test refuses a direct `go build` in a test file.**
* **D1-B. `-buildvcs=false` added at each of the six sites, and nothing
  more.**
* **D1-C. `GOFLAGS=-buildvcs=false` set by `make gate` and CI.**
* **D1-D. Build each fixture inside this repository's tree.**

### D2: waiting for a helper process

* **D2-A. `testproc.Start(t, cmd, ready)`: start the process, wait for
  the ready file, fail at once if the process exits first (with its exit
  status and the tail of its output), and bound the wait by the test's
  deadline less a grace period, as `internal/testenv` does; every helper
  wait uses it, and the process's `Wait` goes through it.**
* **D2-B. Raise each fixed budget, to 60 s say.**
* **D2-C. A scale factor, `SELFUPDATE_TEST_TIMEOUT_SCALE`, on the fixed
  budgets.**
* **D2-D. Start each fresh binary once, unmeasured, before the timed
  start.**

### D3: `TestIdentity`'s budget

* **D3-A. The runs that should succeed use the tool's own default,
  `identityTimeout`; "a hang" keeps its 1 s.**
* **D3-B. Keep 5 s.**

### D4: holding D1 in CI

* **D4-A. CI's Linux job runs the packages that build fixtures with a
  `TMPDIR` that holds an empty `.git`.**
* **D4-B. The static test of D1-A alone.**

## Decision Outcome

Chosen on 2026-10-11 by the owner ("committed, proceed"): **D1-A, D2-A,
D3-A and D4-A.**

* **D1-A,** because a fixture build that cannot read a stray `.git` is
  correct on any machine, and none of the fixtures uses VCS information.
  A helper puts the flag in one place, and the static test keeps the next
  fixture build from forgetting it. D1-C fixes only the gate, not
  `go test`; D1-D changes six fixture modules to dodge a flag.
* **D2-A,** because the readiness wait guards setup, not the behaviour
  under test: a slow first start is not a defect, and a crash should be
  reported as one at once. Bounding by the test's deadline follows the Go
  project's own practice, so `go test -timeout` stays the one knob. D2-B
  only moves the cliff; D2-C needs whoever runs the gate to know the
  machine is slow; D2-D still guesses a budget.
* **D3-A,** because the test then checks the tool with the budget it
  ships, and still checks the timeout path with "a hang".
* **D4-A,** because it checks the outcome (the build works under a stray
  `.git`), where the static test checks the form, and costs one step in a
  job that already builds these packages.

**The decisions in detail:**

1. **`internal/testproc`**, a non-test package that only `_test.go` files
   import, standard library only:
   * `GoBuild(t testing.TB, dir, out string, env []string, args ...string)`
     runs `go build -buildvcs=false args… -o out .` in `dir`, with `env`
     appended to the environment, and fails `t` with the output on error;
   * `Start(t testing.TB, cmd *exec.Cmd, ready string) *Proc` starts
     `cmd`, captures its combined output, and waits for `ready`. It fails
     `t` when the process exits first ("exited (status) before it was
     ready", with the output's last lines), or when the budget runs out
     ("not ready after <elapsed>");
   * `(*Proc).Wait() error` waits for the process, once, for every
     caller;
   * `Budget(t testing.TB) time.Duration`: with a test deadline, the time
     left less a grace period of 5 % of it, at least 1 s; without one,
     5 minutes.
2. **depguard:** a rule `internal-testproc`, `$gostd` only, sorted after
   `banned`; `other-packages` excludes it. AGENTS.md's list and
   `docs/architecture.md`'s rule count, tree and package table follow.
3. **The six fixture builds** call `GoBuild`. A test in
   `internal/testproc` parses every `_test.go` file in the module and
   fails on a call of `exec.Command` or `exec.CommandContext` whose
   arguments hold the literal `"build"` with `go` as the program, naming
   the file and line; it reaches the six sites through the fixture build
   helpers, and its allowlist is empty.
4. **The six helper waits** call `Start`, and their cleanups call
   `Proc.Wait`. The failure texts become `Start`'s.
5. **`TestIdentity`** leaves `Timeout` unset for its successful runs.
6. **CI's Linux job** runs `go test` for `./selfupdate/...`, `./buildinfo/`
   and `./internal/...` with `TMPDIR` set to a directory holding an empty
   `.git`.
7. **No release.** Nothing exported or shipped changes.

### Consequences

* Good, because `make gate` and CI fail for the code, on a loaded Mac, a
  host with a stray `.git`, or a runner.
* Good, because a crashed helper is reported at once, with its status and
  output.
* Good, because the next fixture build or helper wait gets both fixes by
  using the helper, and the static test catches one that does not.
* Neutral, because a helper that hangs while alive now waits until the
  test's deadline, less the grace; each helper's own life bound ends it
  sooner in practice.
* Bad, because a new package, its depguard rule and its row add a little
  to the records and the gate.

### Confirmation

* Before the change, the reproduction (`TMPDIR` with an empty `.git`)
  fails the four packages named above; after it, they pass.
* `internal/testproc`'s own tests fail on a plant of each behaviour:
  `GoBuild` without `-buildvcs=false` under a stray `.git`; `Start`
  without its exit check (a helper that exits at once is reported only at
  the budget); `Budget` without the grace period. The static test fails
  on a planted direct `go build`.
* `make gate` passes on the Linux test host with its own `TMPDIR` and
  `BASH_ENV` untouched, and on this Mac; the record states the Mac's load.
* The Windows-only sites pass on the Windows test host; CI passes,
  including the new step.

## Pros and Cons of the Options

### D1-A. A helper, and a static test

* Good, because the flag lives in one place, and a new site is caught.
* Bad, because it adds a package.

### D1-B. The flag at each site

* Good, because it is six one-line changes.
* Bad, because the seventh fixture build forgets it.

### D1-C. `GOFLAGS` in the gate and CI

* Good, because no test changes.
* Bad, because `go test` run directly still fails, and the setting hides
  the dependency rather than removing it.

### D1-D. Build inside the repository

* Good, because a repository the build can read is always found.
* Bad, because the fixtures then carry this repository's VCS stamp,
  dirty or not, which nothing wants, and six fixture modules move.

### D2-A. Start, bounded by the test's deadline

* Good, because a slow start passes, a crash fails at once, and the budget
  follows `go test -timeout`, as in the Go project.
* Neutral, because a live hang runs to the deadline, less the grace.
* Bad, because six sites change shape, their `Wait` included.

### D2-B. Larger fixed budgets

* Good, because each change is one number.
* Bad, because it moves the failure to a slower machine, and a crash
  still waits the whole budget.

### D2-C. A scale factor

* Good, because a slow machine can be told so.
* Bad, because the person running the gate must know to set it; the
  failure of 2026-10-11 would have happened all the same.

### D2-D. A warm-up start

* Good, because the timed start is then fast.
* Bad, because the warm-up itself needs a budget, and it hides a start
  that is slow for a reason.

### D3-A. The tool's own budget

* Good, because the test checks what ships.
* Neutral, because a wedged success case waits up to 30 s.

### D3-B. 5 s

* Good, because a regression in start time shows sooner.
* Bad, because start time is not what the test is for, and the machine's
  load decides it.

### D4-A. A CI step with a stray `.git`

* Good, because it checks the outcome on every push.
* Bad, because it adds about a minute to the Linux job.

### D4-B. The static test alone

* Good, because it costs nothing at run time.
* Bad, because it checks the form: a fixture built another way, or a new
  path to VCS stamping, passes it.

## More Information

* [0022-PLAN-error-chain-and-process-liveness.md](0022-PLAN-error-chain-and-process-liveness.md),
  "Checks on the pin commit": the two failed gate runs and their
  evidence.
* [0008-MADR-enforce-import-rules-with-depguard.md](0008-MADR-enforce-import-rules-with-depguard.md):
  a new package gets its own rule.
* `go help build`, `-buildvcs`: by default ("auto"), VCS information is
  stamped "if the main package, the main module containing it, and the
  current directory are all in the same repository"; `-buildvcs=false`
  always omits it.
