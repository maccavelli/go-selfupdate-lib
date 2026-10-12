---
status: accepted
date: 2026-10-12
decision-makers: go-selfupdate-lib maintainers
consulted: "a read-only audit, in five parts, of every document a reader of the module meets, against the code at c2eded4 (v1.15.1 plus records, CI and tests), 2026-10-12; the earlier pass, 0022-MADR-documentation-pass-after-v1-14-0.md"
informed: the fleet's programs that read this module's guides and go doc, magic-cli-remote first
---
# Correct the documentation against v1.15.1, settle how Go comments cite records, add the missing Examples, and fix the four defects the audit found

## Context and Problem Statement

The owner asked, on 2026-10-12, for a documentation pass: "ensure all
docs are accurate, comprehensive, and up-to-date". The last pass,
[0022-MADR-documentation-pass-after-v1-14-0.md](0022-MADR-documentation-pass-after-v1-14-0.md),
was made against `v1.14.0`. Since then `v1.15.0` and `v1.15.1` shipped,
and three more records landed:

* [0022-PLAN-liveness-open-at-medium-integrity.md](0022-PLAN-liveness-open-at-medium-integrity.md);
* [0023-MADR-robust-test-fixtures-and-process-waits.md](0023-MADR-robust-test-fixtures-and-process-waits.md);
* [0021-PLAN-instance-race-and-ci-live-tests.md](0021-PLAN-instance-race-and-ci-live-tests.md).

magic-cli-remote is assessing a move onto `v1.15.1`, so its authors are
the next close readers.

The audit found 91 items. Four of them are defects in code that the
documents describe correctly or not at all, so a pass that only edited
prose would either write a defect down as behaviour or leave it unsaid.
This record decides four things:

* D1: what the pass corrects;
* D2: how Go comments cite records;
* D3: which Examples are added;
* D4: what is done about the four defects.

**Facts [observed 2026-10-12, `main` at `c2eded4`]:**

* **The audit.** Five read-only audits, one per area:
  1. README.md, AGENTS.md, `docs/README.md`, `docs/reports/` and the
     per-agent pointers;
  2. `docs/architecture.md`;
  3. `docs/guides/extending-selfupdate.md`;
  4. the migration guide and `docs/guides/building-releases.md`;
  5. every package's doc comment, exported doc comments and Examples.

  Each audit checked every claim against the code, `go doc`, the
  workflows and the records, compiled each guide's snippets in a scratch
  module, and ran the Examples. Each finding cites its evidence; the
  findings that change code, and a sample of the others, were checked
  again before this record. The PLAN's "Findings" table lists all 91:

  | Where | Findings | Kind |
  | :--- | :--- | :--- |
  | README.md, AGENTS.md, `docs/README.md`, reports | 10 | the loopback-redirect rule; two stale report claims; what `make gate` leaves out; `plant-copy.sh`'s argument form; the live-test switches; `internal/testproc` and `winproc`'s liveness check absent; four guide sections with no index row; the batch-file refusal absent from README |
  | `docs/architecture.md` | 19 | `winproc`'s scope and `Alive`'s OS; `ErrBatchFile`, `ErrTaskDisabled`, the hop wait, `SameExecutable`, `releasespec` absent; `ghattest` under the wrong record; CI's triggers, the session probe, codesign's identity test; `other-packages`; the publish workflow's first step and token; `apicheck`'s baseline |
  | `extending-selfupdate.md` | 19 | Task Scheduler's stop bound and a disabled task after a failed stop (C1, C2); version attributions; the handoff's refusal and `ErrInsideService`; the liveness fix; live-test switches; the service errors and poll defaults; the exec reconciler's receipt; one snippet that does not compile |
  | migration and building guides | 13 | the toolchain; `install.ps1`'s standard input (D2); schema 4; missing `v1.5.x` sections; arm64 runners on private repositories; setup failures; extras' paths; the `-tools` artifact; limits and timeouts; uninstall's leftovers |
  | Go doc comments and Examples | 25 | four stale statements from before channels, archives and reconcile warnings; four docs naming unexported identifiers or another repository; the receipt protocol; `FakeSource` and `ErrNoRelease` (E14); records cited by number (E21); seven packages without an Example (E22); an Example that hard-codes what `buildinfo` supplies |
  | seen outside the five scopes | 5 | `.golangci.yml`'s backend count and two number-only citations; two script headers that miscount their modes |

* **The four defects:**
  * **C1, Task Scheduler's grace.** `Task.Stop` sets one deadline from
    `Options.Poll.Timeout` (`selfupdate/service/taskscheduler/lifecycle.go:138`)
    and spends `StopGrace` inside it (`:173`, `:232`). When `StopGrace`
    is at least `Poll.Timeout`, the grace wait fails with `ErrTimeout`
    and `Stop` returns (`:174-175`) without calling
    `IRegisteredTask::Stop`. `New` does not refuse that combination
    (`task.go:132-139`).
  * **C2, a task left disabled.** `Stop` disables the task (`:163`)
    before it ends the program, and returns any later error without
    enabling it again. The managed install's `recoverStop`
    (`selfupdate/managed.go:261-275`) calls `Start` only when the service
    is down. A stop that fails with the program still running therefore
    leaves the program running and its task disabled, so its logon and
    watchdog triggers no longer start it. The guide says only a crash
    between `Stop` and `Start` does that.
  * **D2, `install.ps1`'s standard input.** `install.sh` runs hooks and
    the identity command with `</dev/null`
    (`internal/cmd/selfupdate-release/installer/install.sh:222`, `:257`).
    `install.ps1` runs both with no redirection (`install.ps1:273`,
    `:290`). Both guides say the installers give hooks no standard input
    (`building-releases.md:147-148`, `:478`; the migration guide `:700`).
    What a hook then reads under PowerShell is [unverified].
  * **E14, `FakeSource` and `ErrNoRelease`.** `GitHubSource` wraps
    `ErrNoRelease` for a missing release (`selfupdate/github.go:330`), and
    `CheckCached` caches it as `CheckNoRelease`. `selfupdatetest.FakeSource`
    returns an unwrapped error (`selfupdatetest.go:131`, `:145`), so a
    program's test with the fake never takes either path. The
    `ReleaseSource` doc does not state the contract.
* **Records cited by number in Go comments (E21).** About 450 comment
  lines in 98 non-test files cite records as "0021-MADR D7"; 178 of them
  reach `go doc`. AGENTS.md says "Cite records by full filename, never by
  number alone". A doc comment cannot hold a link, and since a number may
  now name several PLANs ("0022-PLAN P1" fits three), some citations are
  ambiguous; a few anchors name no record at all ("(amendment F4)").
  0022's pass corrected only cross-repository citations.
* **Packages without an Example (E22):** `buildinfo`,
  `selfupdate/service`, its four backends, and `selfupdatetest`.
  `selfupdate`'s `exampleService` says "A real program binds a
  selfupdate/service backend here", and no backend shows it.
* **The audit's own footprint.** Nothing in the repository was written.
  One audit installed the `apidiff` version `scripts/check-api-compat.sh`
  pins into the Go bin directory under the home directory, beyond its
  read-only brief; nothing else outside the scratchpad was written.

## Decision Drivers

* A program author reads the guides and `go doc` as the contract. A
  document that is wrong costs that author more than a gap does.
* A defect is not documentation: describing C1 or C2 as intended
  behaviour would make the guide accurate and the library wrong.
* No exported API breaks; `apicheck` stays compatible with `v1.15.1`.
* The citation rule exists so a reader can find the record. Churn that
  does not help a reader is a cost.
* Every new test or assertion is seen failing first (AGENTS.md).

## Considered Options

### D1: what the pass corrects

* **D1-A.** Every confirmed finding in the PLAN's table, prose and Go
  doc alike.
* **D1-B.** Only what is wrong, stale or broken; gaps wait.
* **D1-C.** The Markdown documents only; Go doc comments wait.

### D2: how Go comments cite records

* **D2-A.** Markdown cites by full filename, as AGENTS.md says (the
  pass fixes each exception it found). A Go comment may cite
  "NNNN-MADR <anchor>" when that number has one MADR, since a comment
  cannot link. It names the full filename for a PLAN whose number has
  more than one PLAN, and for a report. Every anchor names its record. A
  fact from another repository is restated and cited to this
  repository's record. AGENTS.md's "Records" says so.
* **D2-B.** Full filenames everywhere, about 450 comment lines.
* **D2-C.** Leave Go comments as they are.

### D3: which Examples are added

* **D3-A.** `buildinfo`: `ExampleIdentity`. `selfupdatetest`: an
  Example with `FakeSource` and an `Updater`, with `// Output:`. Each
  backend: a compile-only `ExampleNew` that wires it into
  `NewManagedInstaller`. `selfupdate`: `ExampleNew_standalone` builds its
  User-Agent with `UserAgent` and `buildinfo`, and names `releasespec`
  for the asset selector.
* **D3-B.** Only the stale `ExampleNew_standalone` (E23).
* **D3-C.** No Example changes.

### D4: the four defects

* **D4-A. Fix them in this record, test each first, and release
  `v1.15.2`:**
  * C1: `New` refuses a `StopGrace` that is not shorter than the
    effective `Poll.Timeout`, with an error naming both; *(amended
    2026-10-12, below: only a grace the program set, with `RequestStop`
    set; a defaulted grace is shortened instead)*
  * C2: when `Stop` fails after it disabled the task and the recorded
    program still runs, it enables the task again before it returns, so
    a failed stop leaves the task as it found it;
  * D2: `install.ps1` gives hooks and the identity command empty
    standard input, measured on the Windows test host before and after
    through the installer's Windows test;
  * E14: `FakeSource` wraps `selfupdate.ErrNoRelease` for a missing
    release, and `ReleaseSource`'s doc states the contract.

  The guides then describe the fixed behaviour, and the migration guide
  gains its `v1.15.2` section.
* **D4-B. Document the current behaviour now,** and decide the fixes in
  later records.
* **D4-C. Fix C1 and C2 only,** and document D2 and E14 as they are.

## Decision Outcome

Chosen options (the owner, 2026-10-12, "follow recommendations and
proceed"): "D1-A", "D2-A", "D3-A" and "D4-A", because:

* D1-A is what the owner asked for. Every finding is confirmed, and a
  gap a reader needs (the live-test switches, the receipt protocol, the
  service errors) costs as much as a wrong line.
* D2-A fixes every citation a reader cannot resolve: the ambiguous
  PLANs, the anchors with no record, and the facts in another
  repository. It leaves the rest, which resolve today, and makes the
  rule match what a comment can do.
* D3-A shows the wiring the package docs promise and no Example shows.
  The backends' Examples compile on every OS, as their constructors do.
* D4-A: each of the four is small, local and testable. Writing C1 or C2
  down as behaviour would mislead the reader the pass is for. Together
  they make `v1.15.2` a release with changes a program sees.

### Consequences

* Good, because the guides and `go doc` match the code at `v1.15.2`, and
  the two Task Scheduler defects no longer leave a program stopped or a
  task disabled.
* Good, because a program's test with `FakeSource` takes the same
  `ErrNoRelease` and `CheckNoRelease` paths as one with GitHub.
* Neutral, because C1's refusal rejects a configuration that could never
  stop a task, so no working program changes behaviour. *(Not so as first
  decided: see the amendment. As amended, a program on the defaults is
  not refused.)*
* Bad, because the pass touches most documents and many Go files, so
  its commits are large; the PLAN splits them by area.
* Bad, because `v1.15.2` needs a release, its pins, and the guide's
  section.

### Confirmation

* Every finding in the PLAN's table is closed with the line changed, or
  recorded as not changed with the reason.
* Each code fix's test fails on a plant of today's code: C1 and C2 over
  the fake transport and in the live suite on the Windows test host; D2
  in the installer's Windows test, with a hook that reads its standard
  input; E14 in `selfupdatetest`'s tests.
* `scripts/check-docs.sh --links`, `--ids` and `--arch`, markdownlint,
  `go vet` for three targets, and `make gate` pass; every Example passes
  `go test -run '^Example'`.
* `apicheck` reports compatibility with `v1.15.1`; `v1.15.2`'s CI passes.

## Pros and Cons of the Options

### D1-A. Every finding

* Good, because the result is what the owner asked for.
* Bad, because it is the largest of the three.

### D1-B. Wrong, stale and broken only

* Good, because it is smaller.
* Bad, because the gaps are what a new reader trips on.

### D1-C. Markdown only

* Good, because no Go file changes.
* Bad, because `go doc` is where a program author reads the contract.

### D2-A. Fix what cannot be resolved, and write the rule down

* Good, because every citation then resolves, changing the sites the
  audit names (about 15) and any more the PLAN's search finds.
* Bad, because Go comments keep a shorter form than Markdown.

### D2-B. Full filenames everywhere

* Good, because one rule covers every file.
* Bad, because about 450 lines change, most of them already
  unambiguous, and doc comments grow long.

### D2-C. Leave them

* Bad, because the ambiguous and unanchored citations stay.

### D3-A. The Examples above

* Good, because each package a program uses shows its wiring.
* Bad, because new test files change `docs/architecture.md`'s counts,
  and the backend Examples are compile-only.

### D3-B and D3-C

* Good, because they are smaller.
* Bad, because the backends keep no Example.

### D4-A. Fix and release

* Good, because the documents can describe correct behaviour.
* Bad, because it needs a release, and the D2 fix needs the Windows test
  host.

### D4-B. Document now, fix later

* Good, because the pass stays prose only.
* Bad, because the guides would document two defects as behaviour.

### D4-C. Task Scheduler only

* Good, because it is smaller than D4-A.
* Bad, because the installer guide would stay wrong for `install.ps1`,
  or document a gap between the two installers.

## Amendments

### A1 (2026-10-12): C1 refuses only a grace the program set

* **Found in the PLAN's W1,** before any code changed. `DefaultStopGrace`
  is 10 s (`selfupdate/service/taskscheduler/task.go:32`). A program that
  sets `RequestStop` and a `Poll.Timeout` of 10 s or less, and leaves
  `StopGrace` at zero, would be refused by `New` under D4-A as first
  written. That program works today whenever its program exits on
  request before the timeout; only a program that ignores the request
  meets C1. D4-A's claim that the refused configuration "could never stop
  a task" was wrong. `StopGrace` is used only when `RequestStop` is set.
* **Decision (the owner, 2026-10-12, "go with recommendations.
  proceed."):** with `RequestStop` set, `New` refuses a `StopGrace` the
  program set that is not shorter than the effective `Poll.Timeout`,
  naming both. A zero `StopGrace` becomes the shorter of
  `DefaultStopGrace` and half the effective timeout. Without
  `RequestStop`, `StopGrace` is not checked.
* **Not chosen:** `Stop` capping every grace at half the timeout, which
  silently shortens a long grace a program set; documenting C1 only,
  which leaves the trap.

### A2 (2026-10-12, proposed): `install.ps1` gives hooks an empty file as standard input

* **Found in the PLAN's Deviations D3 and D4.** W3's `@() | & $exe` and
  D3's BOM-less `$OutputEncoding` both passed on the Windows test host and
  both failed on CI's `windows-2025` (runs `38306877278`, `38310786338`):
  in Windows PowerShell 5.1, every hook and the identity check read 3
  bytes. `main` is red.
* **Measured on the Windows test host** (Windows PowerShell 5.1.26100,
  PowerShell 7.6.6; a program that reports what it reads):

  | How the program is started | 5.1 | 7.6 |
  | :--- | :--- | :--- |
  | `@() \| & prog`, BOM-less `$OutputEncoding`, `[Console]::InputEncoding` UTF-8 with a BOM | 3 bytes, `efbbbf` | 0 |
  | `@() \| & prog`, both UTF-8 with a BOM | 6 bytes, two BOMs | 3 bytes |
  | `Start-Process -RedirectStandardInput <empty file>`, both UTF-8 with a BOM | 0 | 0 |

  The host's console input encoding is IBM437, which has no BOM, so W3 and
  D3 passed there; CI's is UTF-8 [derived from the failure].
* **Why, from the sources:**
  * Data piped to a native command is encoded with `$OutputEncoding`,
    and in Windows PowerShell "any Unicode encoding, except UTF7, always
    creates a BOM"; PowerShell 6 and later default to UTF-8 without one
    ([about_Character_Encoding][ace]).
  * Windows PowerShell 5.1 runs on .NET Framework, whose `Process` writes
    a redirected standard input through `Console.InputEncoding`: a
    UTF-8 console input encoding puts a BOM on the child's input before
    anything is written. CliWrap reproduced it with a bare `Process`
    under Windows' UTF-8 setting, and its maintainer declined to work
    around it because `Console.InputEncoding` "is a global variable"
    ([CliWrap#215][cw]); .NET recorded the Desktop and Core difference
    for PowerShell ([dotnet/runtime#16174][rt]). PowerShell's own
    discussions treat the console encodings as session-wide state
    ([PowerShell#4681][p4681], [#10907][p10907]).
  * On Windows, `Start-Process -RedirectStandardInput` opens the file and
    hands its handle to `CreateProcess` as `hStdInput`; no writer and no
    encoding is involved (PowerShell's `Process.cs`, `SetStartupInfo`).
  * Its pitfalls, each measured or documented: `-ArgumentList` joins the
    arguments with spaces and quotes nothing (`'b c'` arrived as `b`,
    `c`); `-Wait` waits for the whole process tree
    ([Start-Process][sp]), so a hook that starts a long-lived program
    would hang the installer; and Windows PowerShell reports no
    `ExitCode` unless the process handle is read while it runs (measured:
    empty on 5.1, `0` on 7.6).
  * **Public practice:** Microsoft's `microsoft/WindowsDeveloperConfig`
    runs `wsl.exe` with no input as `Start-Process -NoNewWindow
    -RedirectStandardInput ([IO.Path]::GetTempFileName())`, with
    `-PassThru`, no `-Wait`, the handle read first ("so Windows
    PowerShell can still report ExitCode"), and a poll on `HasExited`
    (`src/windows-dev-config/steps/_environment.ps1`,
    `Invoke-DevConfigProcess`). `ggml-org/llama-install.sh`'s `install.ps1`
    redirects a tool's input from a file the same way.
* **Options:**
  * **A2-1. `Start-Process` with an empty temporary file as standard
    input, in both editions** (recommended). Each hook and the identity
    command start with `-NoNewWindow -PassThru -RedirectStandardInput
    <an empty file from GetTempFileName>`; the handle is read at once; the
    installer polls `HasExited`, so Ctrl+C still stops it, then reads
    `ExitCode`. A hook's output goes straight to the installer's standard
    output, as `Write-Line` sent it. The identity command's output goes to
    a temporary file, whose first line is read, and its error output to
    another, discarded as `2>$null` did. The arguments are joined by
    `Start-Process`, which is safe because the renderer allows only
    `^[A-Za-z0-9._:=/,+@%-]+$` in any value it writes (`render.go:94-103`).
    The temporary files are removed in `finally`. D3's `$OutputEncoding`
    lines go, since nothing is piped.
  * **A2-2. Set a BOM-less `[Console]::InputEncoding` for the call, and
    restore it** (the recommendation before this research). It works, but
    it changes the console's state for every program attached to it while
    the hook runs, against the guidance above, and it fixes only the .NET
    Framework path.
  * **A2-3. Revert W3,** and document that `install.ps1`'s hooks share
    the console's input.
* **Recommended: A2-1,** because it removes the cause rather than
  working around an encoding: the child gets an empty file's handle, in
  both editions, whatever either encoding is; it uses documented
  parameters and Microsoft's own pattern; and it changes no session or
  console state.

[ace]: https://learn.microsoft.com/powershell/module/microsoft.powershell.core/about/about_character_encoding
[cw]: https://github.com/Tyrrrz/CliWrap/issues/215
[rt]: https://github.com/dotnet/runtime/issues/16174
[p4681]: https://github.com/PowerShell/PowerShell/issues/4681
[p10907]: https://github.com/PowerShell/PowerShell/issues/10907
[sp]: https://learn.microsoft.com/powershell/module/microsoft.powershell.management/start-process

## More Information

* The PLAN, [0024-PLAN-documentation-pass-after-v1-15-1.md](0024-PLAN-documentation-pass-after-v1-15-1.md),
  holds the findings table and the phases.
* The audit's scratch files lived in the session scratchpad, outside the
  repository.
