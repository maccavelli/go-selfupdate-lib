# runit, s6 and OpenRC: full backends in the library, or a seam the consumer fills

Associated record:
[0035-MADR-serve-supervisors-through-a-supervised-adapter.md](../decisions/0035-MADR-serve-supervisors-through-a-supervised-adapter.md),
proposed on 2026-10-24 and not yet decided.

Date: 2026-10-24

This report records findings. It decides nothing. The owner asked:

- what opened issue #2;
- whether the library should leave this functionality to the consumer
  and provide an API for it, or carry full backends;
- whether full backends are a good forward-looking posture.

The decision belongs to 0035-MADR, which this report asks to be revised
before it is decided.

## Summary

- **What opened the issue.** magic-cli-remote, the only program that uses
  `selfupdate/service`, moved its runit, s6 and OpenRC support into Go and
  wrote its own backend for them, because the library had none. Measured
  against the library's contract, that backend had three gaps.
- **Since then the consumer has acted on them.**
  - It closed the inside gap itself, with a check that matches the
    library's.
  - It has planned the context and sentinel gap as its own work.
  - It chose not to wire the third piece, the handoff. It is waiting for
    the library's `Detacher` instead, though `service.ProcessDetacher`
    already offers one.
- **Full backends would hold the library below its own founding bar.**
  0011-MADR's drivers include "Stable interfaces only. Parse what each
  platform promises to keep". Two of the three supervisors document no
  machine form for their state, so a library backend would rest on
  observed output.
- **The consumer keeps most of its code under either answer.** The
  library never provisions, so the scripts, their rendering and the
  session supervisors stay the consumer's whatever is decided. That is
  about 650 lines in `supervisor.go` alone.
- **Recommendation, for review: a hybrid.**
  - **Now:** one generic, supervisor-neutral adapter in `service`. The
    library owns the part that is hard to get right: stop and start
    under `ctx`, the typed errors, the health wait, the inside check by
    descent, the handoff, and verify-only reconcile. A caller supplies
    small functions that read state and send commands.
  - **Later:** first-party backends built on that adapter, only where a
    supervisor documents a stable interface (s6's `s6-svstat -o`) and a
    second consumer or the owner asks.

  This unblocks the consumer's handoff in one release, and extends to
  supervisors nobody has asked for yet.

## What opened the issue

The evidence is the consumer's records and code at `be41ef9` (its
`master`, read on 2026-10-24), and the issue itself.

1. **2026-10-13 and 2026-10-19.** magic-cli-remote's
   `0054-MADR-standardize-self-update-on-go-selfupdate-lib.md`, in its
   amendment of 2026-10-13, chose O2: "The supervisors move into
   `setup-service`. runit, s6, OpenRC … become Go code". Its amendment of
   2026-10-19 (S5), at S5-3, adds product backends for `update`,
   "because the library has none for these supervisors".
2. **2026-10-22.** Issue #2 asks for library backends. It names three
   gaps in the consumer's `scriptBackend`: it ignores its context; its
   errors carry no library sentinel; it has no inside check. It also
   notes that the backend looks tools up on `PATH`.
3. **2026-10-23.** The consumer's
   `0210-MADR-hand-off-agent-run-updates-outside-the-service.md`:
   - **F2:** the consumer reproduced an agent-run update leaving the
     service down on runit, s6 and OpenRC 0.62.6.
   - **D4:** it adds the inside check to its own backend. That is
     `scriptBackend.Inside`, a walk up `/proc/<pid>/stat` to the
     supervised PID (`scriptbackend.go:124-155`;
     `scriptbackend_inside_linux.go`, 32 lines).
   - **Option D:** "hand off on runit, s6 and OpenRC through
     `service.ProcessDetacher` with Magic's own inside check". It was
     declined, in part "because it is Magic writing a backend's
     handoff, which upstream issue #2 asks the library to own". The
     owner's answer reads "Guard now, handoff via #2".
   - The comment on issue #2 came from this record.
4. **Proposed, dated 2026-10-23.** The consumer's
   `0207-MADR-move-to-current-library-releases-and-close-the-adoption-gaps.md`
   D20: "The script backend follows the backend contract … `Stop`,
   `Start` and `WaitHealthy` honour their context, and their errors wrap
   `service.ErrTimeout`, `ErrUnhealthy` and `ErrNotInstalled`." At
   `be41ef9` this is not yet in the code. `Stop` still polls to its own
   15 s with `time.Sleep` (`scriptbackend.go:181-191`), and
   `WaitHealthy` returns plain errors (`:234-247`).

So of the issue's three gaps:

- the consumer has closed one (inside);
- it plans to close one itself (context and sentinels);
- what remains is the handoff, which the consumer chose to wait for.

## The consumer's code today [read, `be41ef9`]

| Part | Where | Would it move into the library? |
| :--- | :--- | :--- |
| Render, set up, refresh and remove the run and init scripts | `supervisor.go`, 648 lines | No, under any option. The library never provisions (issue #2, Proposed behaviour 2; 0035-MADR) |
| Start a `runsvdir`, `s6-svscan` or `openrc --user` for the session when none runs | `supervisor_unix.go`, 80 lines; `scriptBackend.Start`, `:194-230` | No. 0035-MADR D6-A returns a sentinel, and the consumer still starts the supervisor |
| Where the PID file is (`${XDG_RUNTIME_DIR:-/tmp}/<name>.pid`), which its own script writes | `scriptBackend.supervisedPID`, `:91-122` | No. 0035-MADR D3-A takes it as `Options.PIDFile` |
| Read state: `sv status` and `s6-svstat` by regular expression, and `rc-service --user <name> status` by its text | `settle.go:198-258` | Yes, under full backends |
| Send commands: `sv down`/`up`, `s6-svc -d`/`-u`, `rc-service stop`/`start` | `scriptBackend.Stop`/`Start` | Yes, under full backends |
| Inside check by descent | `scriptBackend.Inside`, `procParent` | Yes, under either option |
| Stop and health waits under `ctx`, typed errors | planned by its 0207-MADR D20 | Yes, under either option |
| The handoff | not written; waits on #2 | Yes, under either option |

The last three rows, the parts the issue calls "the parts of the
contract the library's backends get right", move under either option.
The middle two rows move only under full backends. The first three
rows never move.

## Who would use library backends [read]

- Three of the owner's repositories depend on the library:
  gobble-cli (`v1.17.0`), prepare-commit-msg (`v1.17.2`) and
  magic-cli-remote (`v1.19.0`). This was read from each repository's
  `go.mod` on 2026-10-24.
- Only magic-cli-remote imports `selfupdate/service` or a backend. The
  other two import `selfupdate`, `selfupdate/cli`, `selfupdate/releasespec`
  and `selfupdate/selfupdatetest` only, read from shallow clones of their
  default branches.

So there is one consumer, and it already owns the supervisor-specific
parts that are coupled to its own scripts.

## What the library offers a lifecycle it has no backend for [read]

- **The seams:** `selfupdate.Lifecycle`, `EnabledLifecycle` and
  `Reconciler`.
- **The typed errors and the wait:**
  - the six `service` sentinels and `CommandError`;
  - `PollHealthy` over a `HealthProbe`, with `Health.Instance` and
    `Previous` to prove a new start (`service/poll.go`).
- **The reconcile and the handoff:**
  - `NewExecReconciler`;
  - `ProcessDetacher(inside)`, `DetachProcess` and `ScrubManagerEnv`.
- **The guidance.** The extending guide says, for "A service manager
  with no backend here": use "`service.ProcessDetacher(inside)`, with
  your own check of whether this process is inside. On Unix it is no
  defence against a cgroup kill or a launchd job's reap"
  (`docs/guides/extending-selfupdate.md:1085-1088`).

What a consumer must still assemble itself, each time, is the
combination: refuse inside, signal, poll to a bound under `ctx`, map
each outcome to a sentinel, prove a new instance, and detach safely.
This module exports no descent walk. `internal/winproc.Descends` is
internal, and is fed only on Windows. So a consumer writes its own, as
this one did in 32 lines.

## The library's own bar [read]

0011-MADR, which created `selfupdate/service`, sets these drivers:

- "**Correct by default.** A reference lifecycle that leaves a service
  stopped, or reports a draining process as healthy, is worse than
  none."
- "**Stable interfaces only.** Parse what each platform promises to keep:
  `systemctl show` properties, `launchctl` exit codes and `list`, Win32
  status and configuration structures."
- "**Testable without a real service manager,** and proven against a
  real one in CI."

It also rejected option D, "Build no reference lifecycles … keep each
program's own lifecycle", because "the defects in the two programs
stay, and every new service program re-implements the seams".

What 0035-MADR found in the manuals, set against that bar:

| Supervisor | A documented machine form for state | Status codes documented | Under 0011's bar |
| :--- | :--- | :--- | :--- |
| s6 | yes: `s6-svstat -o`, "programmatically parsable" | yes, for `s6-svstat` and `s6-svc` | meets it |
| runit | no: the text of `sv status` is not documented; the FAQ shows one example | `sv` exits 0, or 1 to 99 by count, or 100 | below it: a backend parses observed text |
| OpenRC | no: `rc-status -f ini` lists states, and no command prints a PID | only "started returning 0" | below it: a backend relies on observed codes |

0035-MADR's D4-A answers that gap with captured fixtures and live tests.
That makes a change detectable, not stable. The consumer already parses
the same forms, and pins them with its own live runs (its 0210-MADR R1
reproduced on all three).

## The options

### A. Full backends (0035-MADR as drafted)

Three packages own the state reads and the commands, as well as the
contract.

- Good, because a new program on these supervisors writes nothing but
  its scripts.
- Good, because it matches the shape of the existing four backends.
- Bad, because two of the three rest on undocumented forms, below
  0011-MADR's bar.
- Bad, because the cost recurs. Three packages need lint rules, probes,
  fixtures, and CI live setups, one of them an Alpine container. Every
  supervisor release can change a form.
- Bad, because the consumer keeps its scripts, its PID-file layout and
  its session-supervisor start (the table above). So it still carries
  supervisor knowledge, now split across two repositories.
- Bad, because the handoff, the only blocked piece, arrives only as each
  backend ships. 0035-MADR D14-A plans s6, then runit, then OpenRC, in
  three releases.

### B. An API only: a generic adapter in `service`

The library owns the contract. The caller supplies the facts about its
supervisor. A sketch, for review, not decided:

```go
// SupervisedOptions describe a service under a supervisor the library has
// no backend for. Each function is the caller's.
type SupervisedOptions struct {
    State     func(ctx context.Context) (SupervisedState, error) // up, PID, failed, normally up
    Up, Down  func(ctx context.Context) error                    // send the command; do not wait
    Installed func(ctx context.Context) (bool, error)
    Poll      PollOptions
    Probe     HealthProbe
}

// NewSupervised returns a Lifecycle, EnabledLifecycle, Reconciler and
// Detacher built on o.
func NewSupervised(o SupervisedOptions) (*Supervised, error)
```

The library would implement:

- `Stop`, which refuses inside by descent from `State().PID`, calls
  `Down`, and polls `State` under `ctx` and `Poll` to `ErrTimeout`;
- `Start`, and `WaitHealthy` as `PollHealthy`, with the PID as
  `Instance` and `Failed` as `ErrUnhealthy`;
- `Running`, `Enabled` and `Inside`;
- `Detach` through `DetachProcess`;
- a verify-only `Reconcile`, which compares the binary recorded at
  `Stop` from `/proc/<pid>/exe`.

`ErrNoSupervisor` could be a sentinel that `State` returns.

- Good, because the library parses no undocumented output, which keeps
  0011-MADR's bar.
- Good, because it closes all three of the issue's gaps, and the
  handoff, in one release. The consumer passes its existing, live-tested
  samplers and commands.
- Good, because it serves any supervisor, such as dinit, daemontools,
  supervisord, or a container's init, with no further library release.
- Good, because the tests are small: fake `State`, `Up` and `Down`
  functions, beside the existing `DetachProcess` tests.
- Bad, because each program still writes its supervisor's parsing, and
  its correctness is the program's.
- Bad, because it is a fifth way to get a lifecycle, beside four
  concrete backends, and has to be documented as such.
- **Open:** the OpenRC cgroup case. With `rc_cgroup_cleanup=YES`, a
  stop kills a detached run in the service's cgroup. No rule that is
  generic across supervisors detects that without false positives. So
  the adapter would need a caller-supplied guard, such as
  `DetachCheck func(ctx) error`, or a documented limit.

### C. A hybrid: B now, first-party backends on top later

- B ships first.
- A first-party backend is added later only where a supervisor
  documents a stable interface, today s6. Even then, only when a second
  consumer, or the owner, asks. It is built on the adapter: its own
  `State`, `Up` and `Down` over `s6-svstat -o` and `s6-svc`.
- runit and OpenRC stay with the program, through the adapter, until
  their forms are documented or demand appears.

- Good, because it has every advantage of B. A later first-party
  backend is small, since the contract is already the adapter's.
- Good, because the library's reference backends stay at 0011-MADR's
  bar.
- Neutral, because a program on runit or OpenRC writes its state
  function, which the only such program already has.

## Evaluation

| Criterion | A. Full backends | B. Adapter | C. Hybrid |
| :--- | :--- | :--- | :--- |
| Closes the issue's three gaps | yes, per backend release | yes, one release | yes, one release |
| Unblocks the consumer's handoff | after each backend's release | one release | one release |
| 0011-MADR "stable interfaces only" | met for s6 only | met: the library parses nothing undocumented | met |
| Demand served | one consumer | one consumer, and any future supervisor | as B |
| Library maintenance | three packages, three CI live setups, undocumented forms | one type, fake-function tests | as B, with s6 optional |
| Consumer's remaining code | scripts, session start, PID layout | as A, plus its samplers and commands, which it already has | as B |
| Extends to an unasked supervisor | needs a new backend | no library change | no library change |

## Recommendation, for review

- **Revise 0035-MADR before it is decided.**
  - Add a first decision: who owns the supervisor-specific part. Its
    options are A, B and C above.
  - Recommend C.
  - Re-scope the remaining decisions to the adapter: naming,
    `ErrNoSupervisor`, the inside rule, the OpenRC detach guard, and
    verify-only reconcile.
  - Keep D2-A (Linux) and D7-A (descent), which apply unchanged.
  - Move the s6 backend to a later decision.
- **For the consumer**, under C, its 0207-MADR D20 and its handoff wait
  both resolve onto the adapter. It passes its samplers and commands,
  and deletes its own inside walk and wait loops.
- **If the owner prefers A**, 0035-MADR stands as drafted. This report's
  findings still apply as risks: the undocumented forms, the three
  release cycles, and the consumer's code that stays.

## Sources

- The issue and its comment:
  <https://github.com/maccavelli/go-selfupdate-lib/issues/2>.
- magic-cli-remote at `be41ef9`, read through the GitHub API:
  - records: `docs/decisions/0054-MADR-standardize-self-update-on-go-selfupdate-lib.md`
    (O2, amendment S5-2 and S5-3),
    `0207-MADR-move-to-current-library-releases-and-close-the-adoption-gaps.md`
    (D4 as amended, D20), and
    `0210-MADR-hand-off-agent-run-updates-outside-the-service.md` (F2,
    D4, option D, question 2);
  - code: `internal/cli/service/scriptbackend.go`,
    `scriptbackend_inside_linux.go`, `backend.go`, `settle.go`,
    `supervisor.go` and `supervisor_unix.go`.
- This repository at `009838f`:
  [0011-MADR](../decisions/0011-MADR-reference-service-lifecycles.md)
  (Decision Drivers, option D),
  [0035-MADR](../decisions/0035-MADR-serve-supervisors-through-a-supervised-adapter.md)
  (F12 to F25), `selfupdate/service`, and
  `docs/guides/extending-selfupdate.md`.
- The other dependents: gobble-cli and prepare-commit-msg, from their
  `go.mod` files and shallow clones of their default branches.

## Decided (2026-10-24)

The owner chose option C. 0035-MADR was rewritten before any decision
on it, and renamed to match:
[0035-MADR-serve-supervisors-through-a-supervised-adapter.md](../decisions/0035-MADR-serve-supervisors-through-a-supervised-adapter.md).
Its D0 records the choice, and its D1 to D14 propose the adapter. The
text above is as first written, except for the record's file name.

## Integration assessment: option C against the issue as written

This compares what magic-cli-remote must do, and what it gains, under
the issue's full backends (A) and under the adapter (C). The consumer is
read at `be41ef9`. The adapter is as 0035-MADR proposes it.

| Concern | Under A, full backends | Under C, the adapter |
| :--- | :--- | :--- |
| **Starting a session supervisor**, which the consumer's `Start` does when none runs | A library `Start` returns `ErrNoSupervisor`. The managed installer calls `Start` itself, so the consumer must wrap the library backend in a lifecycle of its own, which starts the supervisor and retries. That wrapper must also re-export `Inside`, `Detach` and `Enabled`, or the handoff's type assertion fails | The consumer's `Up` starts the supervisor, then runs `sv up`, `s6-svc -u` or `rc-service start`. No wrapper |
| **State parsing** | The library's. The consumer's live-tested parses are replaced by the library's, which rest on undocumented forms for runit and OpenRC | The consumer's, moved behind `State`. s6 moves to `s6-svstat -o`, its documented form |
| **Its script layout**: service directories under `$XDG_DATA_HOME`, the PID file at `${XDG_RUNTIME_DIR:-/tmp}/<name>.pid` | Translated into each backend's options: a directory, or a service name, a scope and a PID file | Unchanged. `State` already knows it |
| **The handoff** | Arrives as each backend ships: s6, then runit, then OpenRC | Arrives in one release. The update's existing type assertion picks it up |
| **Context, sentinels, the stop bound, a failed stop, PID reuse, restart detection** | The library's | The library's |
| **The application probe** on these supervisors, which the script backend lacks today | Through `Options.Probe` | Through `SupervisedOptions.Probe`, from the same `HealthOptions` |
| **OpenRC under cgroup cleanup** | The library guesses from the cgroup name | The consumer's `DetachCheck` decides from what it knows of its own setup |
| **Its reconciler** | Unchanged: its own `ExecReconciler` | Unchanged |
| **Code it deletes** | `scriptBackend`; its samplers and parses; its settle for script kinds | `scriptBackend`'s inside walk, stop poll and `WaitHealthy`; its settle for script kinds |
| **Code it keeps** | Scripts, PID layout, session start, plus the wrapper above | Scripts, PID layout, session start, plus small `State`, `Up` and `Down` functions per kind |
| **Releases it waits on** | Three | One |

Findings from the comparison:

- **C is the more compatible plug for this consumer.** The one part A
  cannot absorb, starting a session supervisor inside `Start`, is part
  of C's shape by design.
- **C needs work the consumer has already planned.** Its 0207-MADR D20
  asks for `ctx` and the sentinels, and C delivers them. The rest is
  moving existing code behind three functions.
- **C needs two fixes the consumer needs anyway.** Today its commands
  run through `PATH` with their own 30 s bound, and print to its
  standard output during an update. Behind `State`, `Up` and `Down`
  they must take `ctx`, and they should use a `service.Runner` with
  absolute paths.
- **What C does not give the consumer** is the parsing itself. That
  stays its code, as under its own records today.
