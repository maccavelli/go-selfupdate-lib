---
status: accepted
date: 2026-11-09
decision-makers: go-selfupdate-lib maintainers
consulted: "issue #10, opened 2026-11-09, its proposals P4 and P5; every non-test Go file under selfupdate/, buildinfo/ and internal/ at 01e7c13, whose Go code is v1.25.0's; AGENTS.md, docs/architecture.md and the guides at 01e7c13; .golangci.yml's depguard rules; the fleet checkouts on the development host; probe M1, an AST scan run on 2026-11-09 on the macOS development host"
informed: the fleet's programs, which require this module and print its errors
---
<!-- markdownlint-disable MD004 MD013 MD024 MD060 -->

# Keep Go identifiers and record citations out of run-time error messages, give each refusal whose remedy leaves the message a sentinel, and hold the rule with a source scan

## Context and Problem Statement

Issue #10 found that the symlink refusal tells the person running a CLI to "set TargetPolicy.ExecutablePath". That person cannot. Only the program's author can. [0041-MADR-refuse-symlink-invocation-on-every-platform.md](0041-MADR-refuse-symlink-invocation-on-every-platform.md) Amendment A1 fixes that message. The issue's proposal P4 asks for the same rule everywhere: a run-time error that a CLI prints names no Go identifier. It states the fact, and the remedy when a user can act on it. Its P5 asks for a test that holds the rule.

Other run-time messages do what the symlink one did. They name `TargetPolicy` fields, backend `Options` fields and a constructor, and three cite a record by number. A program's user meets them in `update failed: …`, in the `--json` result's `error`, and in logs. None of them has a sentinel, so a program that wants to reword one can only match its text.

This record decides:

* the rule for a message's text (D1);
* whether a refusal whose remedy leaves the message gets a sentinel (D2);
* whether those sentinels get failure classes (D3);
* how the rule is held (D4).

Facts are tagged by their source:

* **[read]:** the tree at `01e7c13`, whose Go code is `v1.25.0`'s (no `.go` file has changed since `b8a5bd0`), or the fleet checkouts on the development host.
* **[probed]:** probe M1, described in F1.
* **[derived]:** follows from the above.
* **[not established]:** stated as unknown.

### Facts

* **F1 [probed]. Probe M1, the inventory.**
  * **What it was.** A Go program in a scratch directory parsed, with `go/parser`, every non-test `.go` file under the module, all build tags included. It left out `internal/cmd/`, `internal/testproc/`, `selfupdate/selfupdatetest/`, `selfupdate/service/servicetest/` and every `testdata/`.
  * **What it matched.** In each call to `fmt.Errorf` or `errors.New`, it joined the string literals and matched two patterns:
    * `\b[A-Za-z][A-Za-z0-9]*\.[A-Z][A-Za-z0-9]*\b`: a Go identifier, `pkg.Name` or `Type.Field`;
    * `\b[0-9]{4}-(MADR|PLAN|REPORT|GATES)\b`: a record citation.
  * **What it found.** 136 files and 689 calls, of which 35 matched. Each match is one of two kinds:
    * **Run-time.** The condition depends on the host: a file's mode, the home directory, a service definition, a task's principal, the process a service ran. A program's user can meet it on any run.
    * **Configuration.** The condition depends only on the program's own options or calls. The author meets it on the first run, before any user does.

  The 12 run-time matches, and the one configuration match that cites a record:

  | # | Site | Kind | Today's text, after its backend prefix |
  | :--- | :--- | :--- | :--- |
  | M1 | `selfupdate/target.go:47-48` | run-time | `target <p> has the setuid or setgid bit; TargetPolicy.AllowSpecialModeBits permits replacing it` |
  | M2 | `selfupdate/target.go:180` | run-time | `no allowed self-update root: the home directory is unavailable, and TargetPolicy.AllowedRoots is empty` |
  | M3 | `selfupdate/service/launchd/reconcile.go:46-47` | run-time | `<plist> runs <current>, not <executable>; Options.RewritePath allows rewriting it` |
  | M4 | `selfupdate/service/systemd/reconcile.go:55-56` | run-time | `<unit> runs <current>, not <executable>; Options.RewritePath allows a drop-in` |
  | M5 | `selfupdate/service/scm/reconcile.go:62` | run-time | `<name> runs <current>, not <executable>; Options.RewritePath allows rewriting it` |
  | M6 | `selfupdate/service/taskscheduler/reconcile.go:63` | run-time | `<path> runs <current>, not <executable>; Options.RewritePath allows rewriting it` |
  | M7 | `selfupdate/service/supervised_lifecycle.go:306` | run-time | `the service ran <a>, not <b>; a program whose service runs another binary passes service.NewExecReconciler` |
  | M8 | `selfupdate/service/taskscheduler/handoff.go:72` | run-time | `<path> logs on with <type>; the one-shot handoff is measured only for an interactive token, and Options.HopDetach starts a hop instead (0032-MADR D8-A)` |
  | M9 | `selfupdate/service/taskscheduler/reconcile.go:70` | run-time | `<path> logs on with <type>; a rewrite is measured only for an interactive token (0021-MADR Amendment A1, Q6)` |
  | M10 | `selfupdate/service/taskscheduler/lifecycle.go:251` | run-time | `Options.RequestStop: <the program's error>` |
  | M11 | `selfupdate/target.go:133` | run-time | the symlink refusal, which 0041-MADR Amendment A1 D4-A replaces |
  | M12 | `selfupdate/service/taskscheduler/register.go:164` | configuration | `logon type S4U is refused to a user who is not an administrator (0032-MADR Amendment A1, P1-2)` |

  The other 23 matches are configuration checks that name the option at fault. Some are fields of the options a constructor validates:
  * `cli.Options` (`selfupdate/cli/run.go:111`, `:113`, `:115`);
  * `SupervisedOptions` (`selfupdate/service/supervised.go:157`, `:159`, `:161`, `:169`, `:171`, `:173`);
  * `PollOptions` (`selfupdate/service/poll.go:73`);
  * launchd's `Options.Domain` (`launchd/job.go:160`);
  * taskscheduler's `Options.StopGrace` and `Options.HandOffDir` (`task.go:164`, `:167`, `:180`).

  Others are an option the program left empty, or an API used the wrong way:
  * the handoff's `Options.Name` or `Options.Unit` (`scm/detach.go:19`, `systemd/detach.go:27`, `:89`, `taskscheduler/detach.go:17`, `handoff.go:50`, `:160`);
  * the systemd unit's name, from `Options.Unit` or the program's product (`systemd/unit.go:185`);
  * `selfupdate.DefaultHTTPClient` (`selfupdate/github.go:120`);
  * `Checker.Check` (`selfupdate/plan.go:394`).
* **F2 [read]. How a message reaches the person running the program.**
  * `cli.Exit` prints any error as `update failed: <message>` (`selfupdate/cli/run.go:357-363`), and the `--json` result's `error` holds the same text.
  * A managed update wraps a lifecycle or reconcile failure with `ErrManagedInstall` (`selfupdate/managed.go:357`). So M3 to M10 reach the CLI too.
  * `Supervised` prefixes its errors with `selfupdate: service: <step> <name>:` (`supervised.go:188-190`).
* **F3 [read]. Each remedy is already in godoc.**
  * `TargetPolicy.AllowSpecialModeBits` (`selfupdate/types.go:474-481`).
  * `RewritePath` on each backend's `Options`: `launchd/job.go:101-104`, `systemd/unit.go:57-61`, `scm/service.go:39-43` and `taskscheduler/task.go:64-68`.
  * `Supervised.Reconcile`, which names `NewExecReconciler` (`supervised_lifecycle.go:284-290`).
  * `HopDetach` (`taskscheduler/task.go:86-93`).

  Two are missing:
  * `AllowedRoots` (`types.go:471-473`) does not say that, with no usable home directory, it must cover the target;
  * `HopDetach` does not say that a non-interactive principal refuses the one-shot handoff.
* **F4 [read]. Sentinel precedents.**
  * `selfupdate/errors.go` holds 15 sentinels, `ErrManagedInstall` and `ErrSymlinkInvocation` among them. `service/errors.go` holds seven, such as `ErrNotInstalled` and `ErrNoSupervisor`.
  * `taskscheduler` exports one of its own, `ErrTaskDisabled` (`task.go:40-47`).
  * A message wraps a sentinel in one of two ways:
    * as a prefix, `%w: <detail>`, as `ErrSymlinkInvocation` does;
    * as a suffix, as `target directory changed while locking: %w` does with `ErrConcurrentUpdate`.
* **F5 [read]. Where a sentinel can live.**
  * depguard's `selfupdate` rule allows `selfupdate` only the standard library, `x/mod/semver`, `x/sys`, `x/term` and `internal/winproc` (`.golangci.yml:112-122`). So `selfupdate` cannot import `service`.
  * The backends and `Supervised` already import `service`.
  * `failureClass` matches `failureClasses` in order, with `errors.Is` (`selfupdate/updater.go:591-620`). `managed-install` comes before `symlink-invocation`.
  * `ManagedInstaller.ResolveTarget` returns the inner installer's error unwrapped (`managed.go:77-79`). So a target refusal reaches the class list as itself, managed or not.
* **F6 [read]. Record citations.**
  * `AGENTS.md`'s citation rule is for Go comments: "A Go comment cannot link, so it may cite `NNNN-MADR <anchor>`". It says nothing of messages.
  * Three error messages cite a record (M8, M9, M12). No other message does, by F1's pattern.
  * A fourth citation sits in a file the library writes, not an error. The systemd drop-in's comment line is `# Written by go-selfupdate-lib: the binary moved (0011-MADR §5).` (`systemd/reconcile.go:96`).
* **F7 [read]. The library already words hints for a CLI's user.**
  * `pass --yes to apply without prompting` (`selfupdate/adapters.go:98`, `confirmer.go:89`).
  * `pass --yes to apply without a TTY` (`confirmer.go:62`).
  * `replacing a local build requires --force` (`errors.go:49`).

  These name flags of the canonical command, not Go identifiers.
* **F8 [read]. Tests that assert today's text.**
  * Six tests assert an identifier that D1 removes:
    * `launchd/launchd_test.go:533` (`RewritePath`);
    * `scm/scm_test.go:576` (`RewritePath`);
    * `systemd/reconcile_test.go:34` (`RewritePath`);
    * `taskscheduler/reconcile_test.go:38` (`Options.RewritePath`);
    * `service/supervised_health_test.go:142` (`NewExecReconciler`);
    * `taskscheduler/handoff_test.go:190` (`HopDetach`).
  * A seventh, `selfupdate/modebits_unix_test.go:48`, asserts `setuid or setgid`.
  * No test asserts M2's, M9's or M10's text.
* **F9 [read]. A source scan has a precedent.** `TestNoDirectFixtureBuild` (`internal/testproc/testproc_test.go:230-265`) walks the module from `../..`, parses each test file with `go/parser`, and fails on a direct `go build`. `AGENTS.md:271` and `docs/architecture.md:433-434` describe it. `TestNoDirectProcessStart` (`selfupdate/spawn_test.go:40`) scans `selfupdate/`'s own files.
* **F10 [read]. The fleet.**
  * Of the checkouts on the development host, `gobble-cli` and `prepare-commit-msg` require `v1.25.0`, and `magic-cli-remote` `v1.24.0`.
  * None matches M1 to M10's or M12's text. The only hits for `Options.RequestStop:` in `magic-cli-remote` are struct fields in its tests.
  * `magic-cli-remote` sets taskscheduler's `RequestStop`, so M10 can reach its users.
* **F11 [not established].** Whether a fleet program outside the development host's checkouts matches any of these texts.

## Decision Drivers

* The person who reads a run-time message is often not the program's author. A CLI prints it to its user. A service writes it to an operator's log.
* A message must stay true and complete as a statement of fact: the paths, the unit, the logon type.
* A program that wants to reword a refusal, or give the remedy in its own terms, must be able to recognise it without matching text.
* The change is additive within `v1`. `make apicheck` reports no incompatible change.
* The rule must be held by a check that fails when a new message breaks it, not by memory.
* A configuration error stays as direct as it is: naming the field is the fastest fix for an author.

## Considered Options

D1, the rule:

* **D1-A.** A run-time message names no Go identifier and cites no record. It states the fact, and when the person running the program can act, says how in their terms, such as a flag of the canonical command (F7). A configuration message may name the field at fault. No error message, of either kind, cites a record.
* **D1-B.** Keep the remedies, marked for the author, such as `(for the program: TargetPolicy.AllowSpecialModeBits)`.
* **D1-C.** No rule. Fix messages one by one as issues report them.

D2, sentinels for the refusals whose remedy leaves the message:

* **D2-A.** One sentinel for each kind of refusal:
  * `selfupdate.ErrSpecialModeBits` (M1);
  * `selfupdate.ErrNoAllowedRoot` (M2);
  * `service.ErrBinaryMismatch` (M3 to M7);
  * `taskscheduler.ErrLogonType` (M8, M9).

  M10 wraps the program's own error, which the program can already match, and gets none.
* **D2-B.** No sentinels. Only the texts change.
* **D2-C.** One generic sentinel, `selfupdate.ErrRefused`, for every refusal.

D3, failure classes:

* **D3-A.** `special-mode-bits` and `no-allowed-root` for the two `selfupdate` sentinels, after `symlink-invocation`. The service sentinels get none.
* **D3-B.** No new classes.
* **D3-C.** Classes for all four sentinels, `binary-mismatch` and `logon-type` as well.
  * It needs `ErrBinaryMismatch` and `ErrLogonType` defined in `selfupdate`, and re-exported by `service` and `taskscheduler`, as `ErrBatchFile` re-exports `internal/winproc`'s (F4, F5).
  * Inside a managed update, `managed-install` still comes first. Unless the two classes were placed ahead of it, which would change existing events' classes, they would show only outside a managed update.

  This option was added on 2026-11-09, when the options were put to the owner.

D4, how the rule is held:

* **D4-A.** A source scan, `TestRunTimeMessagesNameNoGoIdentifier`, beside `TestNoDirectFixtureBuild` in `internal/testproc/testproc_test.go`. It applies F1's scope and patterns to every `fmt.Errorf` and `errors.New`.
  * It allows a Go identifier only where an allowlist names the file and the identifier. The allowlist holds the 23 configuration matches of F1.
  * It allows no record citation at all.
  * An allowlist entry that no longer matches fails too.
* **D4-B.** Table tests of each message's text, and no scan.
* **D4-C.** Review only.

## Decision Outcome

Chosen options: "D1-A", "D2-A", "D3-A" and "D4-A", by the owner on 2026-11-09, as proposed. Each was chosen from three options put to them: D1-A to D1-C, D2-A to D2-C, D3-A to D3-C and D4-A to D4-C.

### The messages under D1-A and D2-A

| # | New text, after the backend prefix where there is one | Sentinel |
| :--- | :--- | :--- |
| M1 | `selfupdate: refusing to replace a setuid or setgid target: <p>` | `ErrSpecialModeBits`, text `selfupdate: refusing to replace a setuid or setgid target` |
| M2 | `selfupdate: no allowed self-update root: the home directory is unavailable` | `ErrNoAllowedRoot`, text `selfupdate: no allowed self-update root` |
| M3 to M6 | `<name> runs <current>, not <executable>: selfupdate: service: binary mismatch` | `service.ErrBinaryMismatch`, text `selfupdate: service: binary mismatch` |
| M7 | `the service ran <a>, not <b>: selfupdate: service: binary mismatch` | `service.ErrBinaryMismatch` |
| M8 | `<path> logs on with <type>, and the one-shot handoff is supported only for an interactive token: selfupdate: taskscheduler: unsupported logon type` | `taskscheduler.ErrLogonType`, text `selfupdate: taskscheduler: unsupported logon type` |
| M9 | `<path> logs on with <type>, and a rewrite is supported only for an interactive token: selfupdate: taskscheduler: unsupported logon type` | `taskscheduler.ErrLogonType` |
| M10 | `the program's stop request: <the program's error>` | none |
| M11 | 0041-MADR Amendment A1 D4-A | `ErrSymlinkInvocation`, as decided there |
| M12 | `logon type S4U is refused to a user who is not an administrator` | none |

* **Prefix or suffix.**
  * M1 and M2 put the sentinel first, as `ErrSymlinkInvocation` does. M2's text is unchanged up to its last clause.
  * M3 to M9 keep the backend prefix, and put the sentinel last, as the lock's `ErrConcurrentUpdate` does (F4).
* **The remedies move to godoc,** where F3 shows they already are, filling its two gaps:
  * `AllowedRoots` says it must cover the target when no home directory is usable;
  * `HopDetach` says that a non-interactive principal refuses the one-shot handoff.

  Each sentinel's comment names its remedy.
* **The citations** move to comments beside the code. The drop-in comment line of F6 is not an error and stays.

### Why

* **D1-A.**
  * It gives every reader a true statement (F1, F2), and keeps the flags-in-hints convention, which already speaks to the user (F7).
  * Configuration errors keep naming the field, because the author is the only one who meets them.
  * A record number means nothing outside this repository, and the comments already carry it (F6).
* **D2-A.**
  * When a remedy leaves the message, a program needs a way to recognise the case and restate the remedy in its own terms. That is why 0041 gave the symlink refusal a sentinel.
  * Four sentinels cover nine messages, one for each kind of fact, and they live where the depguard rules allow (F5). `taskscheduler` already exports a sentinel of its own (F4).
* **D3-A.**
  * A target refusal reaches the class list unwrapped, managed or not (F5). So its class tells a JSON consumer what happened, as `symlink-invocation` does.
  * A service sentinel cannot be classed. `selfupdate` cannot import `service` (F5), and a managed failure is `managed-install` first, which is unchanged.
* **D4-A.**
  * It fails on a new message that breaks the rule, which D4-B cannot see.
  * It follows a scan that already runs in every test run (F9).
  * The allowlist is the record of which messages are for the author. A stale entry fails, so the list cannot rot.
* **Rejected.**
  * D1-B still shows a Go name to a user, and makes every message longer.
  * D1-C repeats issue #10 for every message.
  * D2-B leaves programs matching text.
  * D2-C cannot tell a setuid target from a missing root.
  * D3-B leaves `error` for two refusals a JSON consumer could tell apart.
  * D3-C moves two sentinels out of the packages that return them. It adds classes that a managed update, the usual way a service updates, would rarely show.
  * D4-B and D4-C let the next message break the rule unseen.

### Consequences

* Good, because a CLI's user and a service's operator read facts, not Go names. `cli` needs no change (F2).
* Good, because a program can match `ErrSpecialModeBits`, `ErrNoAllowedRoot`, `ErrBinaryMismatch` and `ErrLogonType`, and word the remedy for its own users.
* Good, because the `failed` event names two more refusals.
* Good, because the next message that names a Go identifier, or cites a record, fails a test.
* Neutral, because configuration errors are unchanged.
* Bad, because nine messages' texts change after their prefixes, and M8's and M9's shape changes. A program that matched them loses the match. No fleet checkout on the development host does (F10). Others are not established (F11).
* Bad, because two failure classes change from `error` to `special-mode-bits` and `no-allowed-root`. A JSON consumer that matched `error` for them sees the new values.
* Bad, because six tests change from text to `errors.Is` (F8).
* Bad, because the scan sees only literal text in `fmt.Errorf` and `errors.New`. A message built elsewhere, such as through `fmt.Sprintf` or `service.CommandError`'s formatter, is not checked.

### Confirmation

* The scan, `TestRunTimeMessagesNameNoGoIdentifier`:
  * passes on the changed tree, with its allowlist exactly F1's 23 configuration matches;
  * reads at least one file, and at least one call, so a wrong root fails it;
  * is seen to fail on three copies, each planted with `scripts/plant-copy.sh`:
    * one message given `Options.RewritePath` back;
    * one given a record citation;
    * one allowlist entry whose message has gone.
* The changed refusals, each tested with `errors.Is` and its exact text:
  * M1 in `modebits_unix_test.go`;
  * M2 in `install_regression_test.go`'s `TestHomeUnavailableRoot`;
  * M3 to M6 in each backend's reconcile test;
  * M7 in `supervised_health_test.go`;
  * M8 in `taskscheduler/handoff_test.go`;
  * M9 in `taskscheduler/reconcile_test.go`;
  * M10 by a test of `requestError`;
  * M12 in `register_test.go`.
* `events_test.go`'s class table gains the two classes.
* `make gate` ends `overall=0`. `make apicheck` reports the four sentinels as additions, and nothing incompatible.
* CI's Linux, macOS, Windows and `go126` jobs pass, which run the Windows-only and Linux-only tests.

## Pros and Cons of the Options

### D1-A: facts for run-time messages, fields for configuration, no citations

* Good, because each reader gets what they can use, and the author's remedy stays one click away in godoc.
* Good, because it matches the hints that already speak to a CLI's user (F7).
* Neutral, because it needs a line between run-time and configuration. F1's line, whether the condition depends on the host, decides all 35 matches.
* Bad, because nine texts change.

### D1-B: keep the remedies, marked for the author

* Good, because the author sees the remedy in the log.
* Bad, because the user still sees a Go name, and longer text.

### D1-C: no rule

* Good, because nothing changes now.
* Bad, because each message is found by its users, one issue at a time.

### D2-A: one sentinel for each kind of refusal

* Good, because a program can tell each case and reword it. It follows `ErrManagedInstall` and `ErrSymlinkInvocation`.
* Bad, because four names are added to the API.

### D2-B: no sentinels

* Good, because no API is added.
* Bad, because the remedy leaves the message, and the program cannot find the case to restate it.

### D2-C: one generic sentinel

* Good, because one name.
* Bad, because a program cannot tell one refusal from another, which is the point.

### D3-A: classes for the two target refusals

* Good, because a JSON consumer can tell them apart from `error`.
* Bad, because their class changes from `error`.

### D3-B: no classes

* Good, because nothing changes in the events.
* Bad, because the events hide two causes the library can name.

### D3-C: classes for all four sentinels

* Good, because every new sentinel has a class.
* Bad, because two sentinels live away from their packages, and inside a managed update their classes do not show without reordering the existing ones.

### D4-A: a source scan with an allowlist

* Good, because it catches the next message, and the allowlist documents the exceptions.
* Bad, because it sees only literal text in two calls, and its pattern could match a non-Go `x.Y` in a message. None does today (F1), and one that did would be fixed or allowlisted by name.

### D4-B: table tests only

* Good, because each text is pinned exactly.
* Bad, because a new message is not seen.

### D4-C: review only

* Good, because no code.
* Bad, because the messages F1 lists were all reviewed when they were written, and still broke the rule.

## More Information

* Issue #10, proposals P4 and P5.
* The symlink refusal: [0041-MADR-refuse-symlink-invocation-on-every-platform.md](0041-MADR-refuse-symlink-invocation-on-every-platform.md) Amendment A1, planned in [0041-PLAN-type-the-symlink-refusal-and-spare-startup-maintenance.md](0041-PLAN-type-the-symlink-refusal-and-spare-startup-maintenance.md).
* Plan: [0042-PLAN-keep-go-identifiers-out-of-run-time-messages.md](0042-PLAN-keep-go-identifiers-out-of-run-time-messages.md). It runs after 0041's second PLAN, so that M11 is already fixed when the scan first runs.
* **Release.** The sentinels are additions, so the release is a minor one. Three routes were put to the owner on 2026-11-09:
  * `v1.26.0` and then `v1.27.0`, which was recommended;
  * one `v1.26.0` with both changes;
  * `v1.26.0` with 0042 left unreleased.

  They chose one `v1.26.0` with this change and 0041's Amendment A1. The 0041 PLAN's release phases move to this record's PLAN, Phases B3a and B3b.
* **Out of scope:**
  * the hints in CLI terms (F7), which already speak to the user;
  * the systemd drop-in's comment line (F6);
  * `internal/cmd/`'s release tools, whose messages a workflow's author reads;
  * the test-support packages `selfupdatetest` and `servicetest`, whose messages a program's tests read.
