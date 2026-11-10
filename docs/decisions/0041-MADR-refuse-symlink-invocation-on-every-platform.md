---
status: accepted
date: 2026-11-09
decision-makers: go-selfupdate-lib maintainers
consulted: "issue #9, opened 2026-11-09; selfupdate/target.go, replace.go, types.go, standalone.go, doc.go, updater.go, spawn_test.go and target_test.go at c0bfb54; Go 1.27.3's os/executable*.go; probe P1, run on 2026-11-09 on the macOS development host, a Linux test host and the Windows test host; for Amendment A1: issue #10, opened 2026-11-09; selfupdate/target.go, errors.go, standalone.go, managed.go, session.go, updater.go, doc.go, types.go and cli/run.go at 01e7c13, whose Go code is v1.25.0's; the guides and example_test.go at 01e7c13; the fleet checkouts on the development host; probe A1-P1, run on 2026-11-09 on the macOS development host and a Linux test host"
informed: the fleet's programs, which require this module and call its workflows
---
<!-- markdownlint-disable MD004 MD013 MD024 MD060 -->

# Refuse an update started through a symlink on every platform, by checking the invoked path as well as `os.Executable`

## Context and Problem Statement

`StandaloneInstaller` refuses to replace its target when the program was started through a symlink. The check reads only the path `os.Executable` returns. Go resolves that path on Linux and leaves it alone on macOS and Windows. So the refusal works on macOS and Windows and never fires on Linux.

Issue #9 reports it. On linux/amd64 at `v1.24.0`, a program started as `$HOME/link/<product>` → `$HOME/.local/bin/<product>` updates and replaces the link's target. On darwin/arm64 the same run fails with `selfupdate: refusing to update through a symlink`.

This record decides how to make the refusal hold on every platform (D1), and what API the refusal gets (D2).

Facts are tagged by their source:

* **[read]:** the tree at `c0bfb54`, or Go 1.27.3's source.
* **[probed]:** probe P1, described in F3.
* **[derived]:** follows from the above.
* **[not established]:** stated as unknown.

### Facts

* **F1 [read]. Where the check is.** `fsOps.resolveTarget` (`selfupdate/target.go:11-25`) takes `raw` from `rawExecutablePath` (`:70-90`). `raw` is `TargetPolicy.ExecutablePath` when it is set. Otherwise it is `fx.osExecutable()`, which `realFS` binds to `os.Executable` (`selfupdate/replace.go:64`). Only `raw` goes through `os.Lstat`, and `ModeSymlink` there returns `selfupdate: refusing to update through a symlink` (`target.go:20-22`). Then `filepath.EvalSymlinks(raw)` gives the target that is checked and replaced (`:26-56`). `revalidateTarget` runs `resolveTarget` again before the replace (`:235-251`).
* **F2 [read]. What `os.Executable` returns.** Its documentation (`$GOROOT/src/os/executable.go`) says: "If a symlink was used to start the process, depending on the operating system, the result might be the symlink or the path it pointed to." On Linux it is `Readlink("/proc/self/exe")` (`executable_procfs.go:21-23`). The kernel has already resolved that path. On darwin it is the `executable_path` the kernel passed, made absolute against the starting directory (`executable_darwin.go:17-29`). On Windows it is `GetModuleFileName(0)` (`executable_windows.go`).
* **F3 [probed]. Probe P1.** A Python script built a Go program that prints `os.Args[0]`, `os.Executable()`, and whether `os.Lstat` reports each as a symlink. It ran the program by its real path and through a symlink in a separate directory, in a new temporary directory that it removed afterwards:

  | Host | Started as | `os.Args[0]` | `os.Executable()` | `Lstat(exe)` is a symlink |
  | :--- | :--- | :--- | :--- | :--- |
  | macOS | link, absolute | `<tmp>/link/prog` | `<tmp>/link/prog` | yes |
  | macOS | link, `./prog` from its directory | `./prog` | `<tmp>/link/prog` | yes |
  | macOS | link, bare `prog` found on `PATH` | `prog` | `<tmp>/link/prog` | yes |
  | Linux | link, absolute | `<tmp>/link/prog` | `<tmp>/real/prog` | **no** |
  | Linux | link, `./prog` from its directory | `./prog` | `<tmp>/real/prog` | **no** |
  | Linux | link, bare `prog` found on `PATH` | `prog` | `<tmp>/real/prog` | **no** |
  | Windows | link, absolute | `<tmp>\link\prog.exe` | `<tmp>\link\prog.exe` | yes |

  Each host's run by its real path returned the real path, not a symlink. On Windows the relative and bare-name runs did not start, because `CreateProcess` searches differently from `execvp`. A bare `os.Args[0]` (`prog`) names no file until it is searched on `PATH`, so `os.Lstat` on it, made absolute against the working directory, fails.
* **F4 [read]. Why the tests miss it.** `TestResolveTargetRejectsSymlink` (`selfupdate/target_test.go:28-38`) passes the link as `TargetPolicy.ExecutablePath`. That skips `os.Executable`, so the test passes on Linux. No test runs the real `os.Executable` through a symlink.
* **F5 [read]. The refusal is not documented.** The issue says `NewStandaloneInstaller` documents the refusal. It does not. Nothing in `standalone.go:13-31`, `TargetPolicy` (`types.go:457-472`), the package documentation (`doc.go:155-175`), `docs/architecture.md`'s install path (`:488-530`) or the guides mentions it. Only the error text does. The check came with the package when it moved from mcplib (`2b7fdae`).
* **F6 [read]. The refusal has no sentinel.** It is a plain `fmt.Errorf` (`target.go:21`). `failureClass` (`selfupdate/updater.go:596-620`) gives the `failed` event the detail `error` for it. A sentinel such as `ErrManagedInstall` gets a class of its own (`managed-install`).
* **F7 [read]. What the tree permits for a `PATH` search.** `TestNoDirectProcessStart` (`selfupdate/spawn_test.go:20-26`) bans `exec.Command`, `exec.CommandContext`, `os.StartProcess` and `syscall.StartProcess`/`ForkExec` in `selfupdate/`. It does not ban `exec.LookPath`, which starts no process. The `selfupdate` depguard rule allows the standard library (`.golangci.yml:112-122`).
* **F8 [derived]. Who is affected.** On Linux, a program that a shell, a unit file or a launcher starts through a symlink gets its symlink's target replaced. That target is a regular file under the home directory or an allowed root, not setuid or setgid, so every other check passes (F1). The issue names one case: a link into a tree another tool manages under the home directory, such as Homebrew on Linux under `~/.linuxbrew`. On macOS and Windows that run is refused.
* **F9 [not established].** How many fleet programs are started through a symlink on Linux today, and so how many updates now succeeding there would be refused after the change.

## Decision Drivers

* The same run should get the same answer on every platform.
* The fix may refuse more updates, never fewer. A path a parent process chose (`os.Args[0]`) must not be able to make an update that is refused today succeed.
* An update started directly must not be refused because of something else in the environment, such as a stale `PATH` or a working directory changed since start.
* `TargetPolicy.ExecutablePath` stays the explicit override, and the way to update deliberately through a link: pass the resolved path.
* No new module, and no unsafe code in `selfupdate`.
* A test must hold the fix with the real `os.Executable`, so it fails on Linux when the fix is removed.

## Considered Options

D1, how to detect a symlink start:

* D1-A: check the path the program was started by, recovered from `os.Args[0]`, as well as `os.Executable`'s result, and refuse when it is a symlink to the same file
* D1-B: on Linux, read the path given to `execve` from the auxiliary vector (`AT_EXECFN`)
* D1-C: document that the refusal holds only where `os.Executable` returns the invoked path, and leave callers to pass it as `TargetPolicy.ExecutablePath`
* D1-D: drop the refusal on every platform, and update the link's target

D2, the refusal's API:

* D2-A: an exported sentinel `ErrSymlinkInvocation`, a message that names the link and its target, and a failure class `symlink-invocation`
* D2-B: the sentinel and message, with no failure class
* D2-C: no API change; the message stays as it is

## Decision Outcome

The owner accepted D1-A and D2-A as proposed on 2026-11-09.

Chosen option for D1: "D1-A", because it works the same way on every platform with the standard library alone. Its identity check means `os.Args[0]` can only add a refusal for a link to the running binary, never remove one.

Chosen option for D2: "D2-A", because a caller or a CLI can then tell this refusal from any other failure without matching text, as it already can for `ErrManagedInstall`. The class is additive, as this module's API rule needs.

### D1-A in detail

When `TargetPolicy.ExecutablePath` is empty, `resolveTarget` keeps today's check of `os.Executable`'s result. It then works out an invoked path from `os.Args[0]`:

1. empty: there is none;
2. with a path separator: it is made absolute against the working directory;
3. without one: it is searched on `PATH` with `exec.LookPath`. A failure, including `exec.ErrDot`, means there is none.

When that path exists, `os.Lstat` reports it as a symlink, and `os.Stat` through it is `os.SameFile` as `os.Lstat` of the target that `os.Executable` resolved to, the update is refused as through a symlink. In every other case the invoked path is ignored, and the result is what it is today.

The check is not limited to Linux. On macOS and Windows `os.Executable`'s check already refuses first, so the second one changes nothing there (F3). On another Unix whose `os.Executable` resolves the link, it closes the same gap.

`os.Args[0]` and `exec.LookPath` are reached through two new `fsOps` fields, `osArgs0` and `lookPath`, so a unit test can supply them, as it supplies `osExecutable` today (F1).

When `TargetPolicy.ExecutablePath` is set, `os.Args[0]` is not read: the caller has named the path, and that path's own `Lstat` check applies (F1).

### D2-A in detail

* `selfupdate/errors.go` adds `ErrSymlinkInvocation = errors.New("selfupdate: refusing to update through a symlink")`. The message keeps today's text as its prefix.
* Both refusals, of `raw` and of the invoked path, wrap it: `%w: <link> links to <target>; run <target>, or set TargetPolicy.ExecutablePath to it`. Shipped in `v1.25.0`. Amendment A1 (D3-A, D4-A, chosen 2026-11-09) replaces this text and makes the refusal a typed error, keeping the sentinel and the class.
* `failureClasses` gains `{ErrSymlinkInvocation, "symlink-invocation"}` after `managed-install`. The `failed` event's detail for this refusal changes from `error` to `symlink-invocation`.

### Consequences

* Good, because a Linux program started through a symlink is refused, as on macOS and Windows, and a link into a tree another tool manages is no longer overwritten there (F8).
* Good, because the refusal is documented for the first time, with the way to update through a link on purpose (F5).
* Good, because a test runs the real `os.Executable` through a symlink, which the existing test cannot (F4).
* Neutral, because macOS and Windows behave as they do today, except for the longer message and the new class.
* Bad, because a Linux program that updates today through a symlink will be refused after the upgrade. Its user must run the target, or the program must set `TargetPolicy.ExecutablePath`. How many programs this reaches is not established (F9). The changelog and the migration guide say so.
* Bad, because the refusal depends on `os.Args[0]`, which a parent can set to anything. A parent that hides the link, with `exec -a` or a launcher that passes the resolved path, gets today's Linux result. D1-B would close that; D1-A does not.
* Bad, because the `failed` event's detail for this case changes from `error` to `symlink-invocation`. A consumer that matched `error` for it sees the new value.

### Confirmation

* Unit tests in `selfupdate/target_test.go` replace `osExecutable`, `osArgs0` and `lookPath` on an `fsOps`. Each case is a temporary home with the real binary and a link to it:
  * `os.Args[0]` as an absolute link, a relative link, and a bare name that `lookPath` finds as the link: refused, `errors.Is(err, ErrSymlinkInvocation)`;
  * `os.Args[0]` as a link to another file, a path that does not exist, a bare name `lookPath` cannot find, and empty: resolved, as today;
  * `os.Args[0]` as a link and `TargetPolicy.ExecutablePath` set to the real binary: resolved, so `os.Args[0]` was not read.
* A process test runs a copy of the test binary through a symlink in a temporary home. The copy calls `realFS().resolveTarget` with the real `os.Executable` and `os.Args[0]`. Through the link it must report the refusal; by its real path it must resolve. It runs the link by absolute path on every platform, and by relative path and by bare name on `PATH` where `exec` starts them (F3). It skips where `os.Symlink` fails, which on Windows needs a privilege.
* That process test is seen to fail on the Linux test host on a copy with the second check removed, using `scripts/plant-copy.sh`. It passes there without the fix only on macOS and Windows, which is the bug.
* `failureClass(ErrSymlinkInvocation wrapped)` is `symlink-invocation` in `updater_test.go`'s class table.
* `make gate` ends `overall=0`; `make apicheck` reports one addition, `ErrSymlinkInvocation`.

## Pros and Cons of the Options

### D1-A: also check the invoked path from `os.Args[0]`

* Good, because it uses the standard library only, with no unsafe code and no platform file.
* Good, because the `os.SameFile` check ties `os.Args[0]` to the running binary. A stale `PATH`, a changed working directory or a spoofed `os.Args[0]` that names another file is ignored, not refused.
* Good, because it covers every platform whose `os.Executable` resolves links, not Linux alone.
* Neutral, because on macOS and Windows it repeats a refusal that `os.Executable` already makes.
* Bad, because a parent that sets `os.Args[0]` to the resolved path hides the link.
* Bad, because a bare name is searched on `PATH` as it is now, not as it was when the program started.

### D1-B: read `AT_EXECFN` on Linux

* Good, because `AT_EXECFN` is the path the kernel was given, which a parent cannot set apart from the file it runs. A shell's `PATH` search hands `execve` the full path.
* Bad, because the auxiliary vector holds `AT_EXECFN` as an address in the process's own stack. Reading it needs `unsafe` in `selfupdate`, or `/proc/self/mem`, and Go exposes no API for the string.
* Bad, because it is Linux only, and a relative `execve` path is still relative to the starting directory, which may have changed.

### D1-C: document the gap, and leave callers to pass the path

* Good, because no behaviour changes.
* Bad, because the default stays inconsistent: every caller on Linux must know to do it, and a caller that does not gets no refusal.
* Bad, because a caller passing the invoked path would have to recover it from `os.Args[0]` itself, which is D1-A in every program.

### D1-D: drop the refusal, and update the link's target

* Good, because every platform would then agree, and a program started through a link would update.
* Bad, because a link into a tree another tool manages would be overwritten on every platform, the harm F8 names.
* Bad, because it removes a refusal that macOS and Windows callers get today, which weakens a security default within `v1`.

### D2-A: a sentinel, a message naming both paths, and a failure class

* Good, because a caller can test `errors.Is(err, ErrSymlinkInvocation)` and give its own advice, and a CLI's `failed` event says what happened.
* Good, because the message tells the user what to run instead.
* Neutral, because the message keeps today's prefix, so a match on it still holds.
* Bad, because the `failed` event's detail changes from `error` for this case.

### D2-B: a sentinel and a message, without a class

* Good, because the `failed` event stays `error`.
* Bad, because the event then hides a cause that `managed-install` and `concurrent-update` show for theirs.

### D2-C: no API change

* Good, because nothing changes beyond D1.
* Bad, because a caller can only match the message text.

## More Information

* Issue #9: "StandaloneInstaller: a symlink invocation is not refused on Linux, because os.Executable resolves it".
* Plan: [0041-PLAN-refuse-symlink-invocation-on-every-platform.md](0041-PLAN-refuse-symlink-invocation-on-every-platform.md), released as `v1.25.0`.
* Amendment A1's plan: [0041-PLAN-type-the-symlink-refusal-and-spare-startup-maintenance.md](0041-PLAN-type-the-symlink-refusal-and-spare-startup-maintenance.md).
* Issue #10: "ErrSymlinkInvocation: the refusal names a Go field to CLI users, gives wrong advice for an explicit ExecutablePath, and fails CleanupPending/KeptBackups". Its proposal P4, the same rule for the other run-time messages, is decided separately in [0042-MADR-keep-go-identifiers-out-of-run-time-messages.md](0042-MADR-keep-go-identifiers-out-of-run-time-messages.md).
* A directory symlink in the path (`~/bin` → `~/.local/bin`) is not refused today on any platform: `os.Lstat` reports only the final component. D1-A keeps that.
* A hard link cannot be told from the file it names, and is not refused on any platform. This record does not change that.

## Amendment A1 (2026-11-09): D3 to D5, the refusal's type, its message and startup maintenance

Issue #10, opened on 2026-11-09 against `v1.25.0`, found three flaws in what D2-A shipped:

* the message names a Go field to the person running a CLI;
* it gives the wrong advice when `TargetPolicy.ExecutablePath` is the link;
* `CleanupPending` and `KeptBackups` fail with it, although neither updates anything.

It also asked for the link and the target as fields. On 2026-11-09 the owner asked for the records to be amended. This amendment adds D3 (the type), D4 (the message) and D5 (startup maintenance). It was proposed, and the owner chose D3-A, D4-A and D5-A the same day (Amendment A1 outcome). D1-A, the sentinel and the `symlink-invocation` class stay as decided. Only D2-A's message text is replaced (D4).

The issue's proposal P4, the same rule for the library's other run-time messages, is not decided here. It reaches four packages beyond the symlink refusal, so it has its own record: [0042-MADR-keep-go-identifiers-out-of-run-time-messages.md](0042-MADR-keep-go-identifiers-out-of-run-time-messages.md).

Facts are tagged as above. **[probed]** here means probe A1-P1 (FA1.4). The tree read is `01e7c13`, whose Go code is `v1.25.0`'s: no `.go` file has changed since `b8a5bd0`.

### Amendment A1 facts

* **FA1.1 [read]. One message for three sources.**
  * `symlinkRefusalTo` (`selfupdate/target.go:132-135`) builds `%w: <link> links to <target>; run <target>, or set TargetPolicy.ExecutablePath to it`.
  * It is reached in two places:
    * from `symlinkRefusal` (`:119-128`), for the path `rawExecutablePath` gave (`:20-21`). That path is `TargetPolicy.ExecutablePath` when it is set, and `os.Executable()` otherwise (`:137-157`). When `filepath.EvalSymlinks` fails, the message is `%w: <link>` (`:122`).
    * directly, for the path `invokedSymlink` found from `os.Args[0]` (`:41-44`). That runs only when `ExecutablePath` is empty.
  * So the link is found in one of three ways: `os.Executable` returned it (macOS, Windows); `TargetPolicy.ExecutablePath` is it; or `os.Args[0]` named it (Linux, and any OS whose `os.Executable` resolves links). The message cannot tell them apart.
* **FA1.2 [read]. Who reads the message.**
  * `cli.Exit` prints any error as `update failed: <message>` (`selfupdate/cli/run.go:357-363`). The `--json` result's `error` carries the same text.
  * The library's other run-time hints speak in the canonical command's flags: `pass --yes to apply without prompting` (`selfupdate/adapters.go:98`, `confirmer.go:89`), `pass --yes to apply without a TTY` (`confirmer.go:62`), and `ErrForceRequired`'s `replacing a local build requires --force` (`errors.go:49`).
  * The person at a terminal cannot set `TargetPolicy.ExecutablePath`. Only the program's author can.
* **FA1.3 [read]. Who calls `CleanupPending` and `KeptBackups`.**
  * Both begin with `resolveTarget` (`selfupdate/standalone.go:83-93`, `:113-122`). `ManagedInstaller.KeptBackups` delegates to its inner installer's (`managed.go:86-94`).
  * The package documentation says `CleanupPending` is "called at startup" (`doc.go:159`). The extending guide says the same (`docs/guides/extending-selfupdate.md:561-563`), and to call `KeptBackups` after it (`:578-579`). The migration guide does too (`:929`, `:958`). `ExampleStandaloneInstaller_CleanupPending` (`selfupdate/example_test.go:630-639`) prints any error that is not `ErrConcurrentUpdate`.
  * Nothing in the library calls either method. Only programs do.
* **FA1.4 [probed]. Probe A1-P1.**
  * **What it was.** A program built against the tree calls `ResolveTarget`, `KeptBackups` and `CleanupPending` on `NewStandaloneInstaller` with `TargetPolicy{ExecutablePath: argv[1], AllowedRoots: [<tmp>]}`. A Python script runs it by its real path and through a symlink in a sibling directory, in a new temporary directory that it removes afterwards. A kept backup, `.msgprobe.selfupdate-kept-1`, is planted beside the real binary. The script lists the real binary's directory before and after each run.
  * **Results.** The macOS development host and a Linux test host printed the same:

    | Case | Started as | `ExecutablePath` | `ResolveTarget` | `KeptBackups` | `CleanupPending` |
    | :--- | :--- | :--- | :--- | :--- | :--- |
    | 1 | `<tmp>/real/msgprobe` | empty | ok | ok, 1 backup | ok; `.msgprobe.selfupdate.lock` created |
    | 2 | `<tmp>/link/msgprobe` → real | empty | refused | refused | refused; nothing changed |
    | 3 | `<tmp>/real/msgprobe` | `<tmp>/link/msgprobe` | refused | refused | refused; nothing changed |
    | 4 | `<tmp>/link/msgprobe` | `<tmp>/real/msgprobe` | ok | ok, 1 backup | ok |

  * **What the refusals said.** Every refusal was `errors.Is(err, ErrSymlinkInvocation)`, with the text `selfupdate: refusing to update through a symlink: <tmp>/link/msgprobe links to <tmp>/real/msgprobe; run <tmp>/real/msgprobe, or set TargetPolicy.ExecutablePath to it`.
    * In case 3 that text tells a process that is already running `<tmp>/real/msgprobe` to run it.
    * Case 2 is found through `os.Executable` on macOS, and through `os.Args[0]` on Linux (F3).
  * **What this confirms.** It matches issue #10's macOS table, and extends it to Linux and to the success path.
* **FA1.5 [read]. What `CleanupPending` writes, and what `KeptBackups` reads.**
  * `CleanupPending` runs `beginSession`. That takes the lock, `.<base>.selfupdate.lock`, in the target's directory (`session.go:592`). Under the lock it:
    * processes a cleanup receipt;
    * recovers an interrupted update's journal, keeping its backup (`journal.go:88-96`);
    * sweeps the leftovers of a crashed update.

    Then it releases the lock. The lock file stays (FA1.4, case 1). Every write is in the target's directory.
  * `KeptBackups` takes no lock and writes nothing. It opens the target's directory with `os.OpenRoot`, and lists regular files named `.<base>.selfupdate-kept-<digits>`, never through a symlink (`standalone.go:109-112`, `:124-127`).
* **FA1.6 [derived]. Where a leftover beside the target can come from.**
  * Since `v1.25.0` an update through a link is refused on every platform (D1-A). So a leftover beside a link's target comes from a run that resolved the target itself:
    * the program started by the target's own path;
    * the program with `ExecutablePath` set to the target;
    * on Linux, an update through the link by `v1.24.0` or earlier, which F8 describes.
  * On Windows, a receipt is left for the old image, which the running process holds open after a direct update. The next `CleanupPending` removes it once that process has exited (`docs/architecture.md:830-833`).
* **FA1.7 [read]. Precedents for the shape.**
  * `RateLimitError` (`selfupdate/errors.go:84-108`) is a pointer-receiver error whose `Unwrap` returns its sentinel, and whose `Error` handles a nil receiver.
  * `HTTPError` (`:113-131`) and `service.CommandError` are typed errors with exported fields. None of the three has JSON tags.
  * The module's enums are string-backed, such as `archive.Format` and `releasespec.Packaging`.
  * `failureClass` matches with `errors.Is` (`selfupdate/updater.go:615-619`). So an error that unwraps to `ErrSymlinkInvocation` keeps the class `symlink-invocation` (`:611`).
* **FA1.8 [read]. What matches the text today.**
  * No test in this module asserts the message after its prefix.
  * Of the fleet checkouts on the development host, `gobble-cli` requires `v1.25.0`. Its `internal/cli/update_test.go:347-348` asserts `refusing to update through a symlink` and `<link> links to ` in its stderr. Its `internal/cli/update.go:198-205` calls `CleanupPending` only after `ResolveTarget` succeeds.
  * `prepare-commit-msg` requires `v1.25.0` and calls neither method.
  * `magic-cli-remote` requires `v1.24.0`. It neither matches the refusal nor calls either method.
* **FA1.9 [not established].** Whether any program calls `CleanupPending` or `KeptBackups` at startup and is started through a link. The development host's checkouts do not (FA1.8), and fleet programs outside them were not read.

### Amendment A1 options

D3, the refusal's type:

* **D3-A.** `*SymlinkInvocationError`, with exported fields `Link`, `Target` and `Source`, each with a JSON tag. It unwraps to `ErrSymlinkInvocation`. `Source` is a string-backed `SymlinkSource`, whose three constants are `SymlinkSourceExecutable` (`"executable"`), `SymlinkSourceInvocation` (`"invocation"`) and `SymlinkSourcePolicy` (`"policy"`).
* **D3-B.** The same type, with `SymlinkSource` an `int` from `iota + 1`, as issue #10 proposed.
* **D3-C.** No type. A function `SymlinkPaths(err error) (link, target string, ok bool)` reads the paths from the error.
* **D3-D.** Three sentinels, one for each source, each wrapping `ErrSymlinkInvocation`.

D4, the message:

* **D4-A.** One message for each case. Each keeps today's prefix, `selfupdate: refusing to update through a symlink`, and names no Go identifier:

  | Case | Text after the prefix |
  | :--- | :--- |
  | `SymlinkSourceExecutable` or `SymlinkSourceInvocation`, target resolved | `: <link> links to <target>; start <target> itself to update it` |
  | `SymlinkSourcePolicy`, target resolved | `: the configured executable path <link> links to <target>` |
  | any source, target not resolved | `: <link> is a symlink` |

  The author's remedy moves to godoc: set `TargetPolicy.ExecutablePath` to the resolved path to update through a link on purpose. It goes on `SymlinkInvocationError`, `ErrSymlinkInvocation` and `TargetPolicy`, and in the extending guide.
* **D4-B.** D4-A, with `; fix the configured path` after the `SymlinkSourcePolicy` text.
* **D4-C.** Drop only `, or set TargetPolicy.ExecutablePath to it` from today's text.
* **D4-D.** Leave the text, and have `cli.Exit` and `cli.Run` reword it when `errors.Is(err, ErrSymlinkInvocation)`.

D5, `CleanupPending` and `KeptBackups` when the program was started through a link:

* **D5-A.** For `SymlinkSourceExecutable` and `SymlinkSourceInvocation` only:
  * `CleanupPending` returns nil, without taking the lock or touching any file.
  * `KeptBackups` resolves the target again with `ExecutablePath` set to the error's `Target`, so the regular-file, setuid and allowed-root checks still apply. It then lists the backups beside that target. When `Target` is empty it returns the refusal.

  With `SymlinkSourcePolicy`, both are refused as today: an explicit `ExecutablePath` that is a link is a configuration error. `ResolveTarget`, `Begin` and the update path refuse exactly as today.
* **D5-B.** Both follow the link. `CleanupPending` takes the lock and processes the leftovers beside the resolved target.
* **D5-C.** Both return nothing. `CleanupPending` returns nil, and `KeptBackups` returns nil, nil.
* **D5-D.** No change. The documentation tells programs to ignore `ErrSymlinkInvocation` from either method.

### Amendment A1 outcome

Chosen options: "D3-A", "D4-A" and "D5-A", by the owner on 2026-11-09, as proposed, each from three options put to them: D3-A, D3-B and D3-D; D4-A, D4-B and D4-C; D5-A, D5-B and D5-D.

* **Why D3-A.**
  * A program with its own UI gets both paths, and the way the link was found, with `errors.As`, and does not parse the string (FA1.1). `errors.Is` and the class still work (FA1.7).
  * The shape follows `RateLimitError`'s (FA1.7). Its fields carry JSON tags, and its `Source` marshals as a name, so the type can appear in a JSON document, as the module prefers for exported types.
  * The source tells D4 and D5 which case they are in, without a second error path through `resolveTarget`.
* **Why D4-A.**
  * "Start" fits a shell, a unit file and a launcher, which "run" does not for the second two.
  * The `SymlinkSourcePolicy` text states a fact, because the remedy belongs to whoever configured the path, and only the program knows who that is (FA1.4, case 3). A program that reads the path from its own setting can wrap the error with that setting's name.
  * The text keeps `<link> links to `, which is the part a fleet test matches (FA1.8).
* **Why D5-A.**
  * A program started through a link stops failing its startup maintenance (FA1.3, FA1.4).
  * Nothing is written in the link target's tree, which D1-A keeps out of reach (F8, FA1.5).
  * `KeptBackups` is read-only and takes no lock (FA1.5), so following the link to list them writes nothing.
  * An explicit `ExecutablePath` link stays an error under every call. A misconfiguration is then reported where the program can see it.
* **Rejected.**
  * D3-B marshals as a number, which says nothing to a reader of JSON.
  * D3-C hides a typed value behind a parser of the module's own string.
  * D3-D multiplies sentinels for one refusal, and gives no paths.
  * D4-B gives an instruction that only some programs' users can follow.
  * D4-C leaves FA1.4's case 3 wrong, and the paths in the string only.
  * D4-D fixes `cli` users alone, and the two texts can drift.
  * D5-B writes a lock file and removes files in a tree that another tool may manage, which is the harm D1-A refuses for an update (F8). The only gain is a cleanup that the next direct start or update does anyway.
  * D5-C hides kept backups that exist, which `KeptBackups` exists to report.
  * D5-D pushes the same check into every program, and the example and guides would have to teach it.
* **Consequences.**
  * Good, because a CLI's user reads an instruction they can follow, and the program's author finds the override in godoc.
  * Good, because a program with an `ExecutablePath` that is a link is told the fact, not given an instruction that cannot work.
  * Good, because a program started through a link no longer fails `CleanupPending` and `KeptBackups` at every start. That was new on Linux in `v1.25.0`, and older on macOS and Windows (FA1.4).
  * Good, because a program can read `Link`, `Target` and `Source` without parsing.
  * Neutral, because `errors.Is(err, ErrSymlinkInvocation)`, the class and the prefix are unchanged.
  * Bad, because the text after the prefix changes again, one release after `v1.25.0` changed it. The `v1.25.0` changelog already says to match the sentinel, not the text.
  * Bad, because `CleanupPending`'s nil does not say it skipped. A leftover beside the target waits for the next start by the target's own path, or the next update. On Windows that includes the old image a direct update left (FA1.6). `KeptBackups` still reports a kept backup meanwhile.
  * Bad, because `KeptBackups` now succeeds through a link where it failed. A program that relied on the failure to detect a link start gets a listing. `ResolveTarget` still reports it.
* **Release.** The type and constants are additions, so the release is a minor one. On 2026-11-09 the owner chose one `v1.26.0` for this amendment and [0042-MADR-keep-go-identifiers-out-of-run-time-messages.md](0042-MADR-keep-go-identifiers-out-of-run-time-messages.md) together, over `v1.26.0` and then `v1.27.0`. So the release phases are in 0042's PLAN. `make apicheck` must report additions only.

### Amendment A1 confirmation

* `selfupdate/target_test.go`:
  * for each source, `errors.As` gives `Link`, `Target` and `Source`, and `Error()` is exactly D4-A's text;
  * no message contains `TargetPolicy`;
  * a dangling link gives the unresolved text;
  * FA1.4's case 3 gives `SymlinkSourcePolicy` and no "start".
* An error test:
  * a nil `*SymlinkInvocationError` prints the sentinel's text and unwraps to it;
  * `json.Marshal` of a value gives the keys `link`, `target` and `source`, with `source` a name.
* `selfupdate/events_test.go`'s class table: a wrapped `*SymlinkInvocationError` is `symlink-invocation`.
* Unit tests of `CleanupPending` and `KeptBackups`, using `fsOps` fakes for the executable and invocation sources:
  * `CleanupPending` returns nil, and the target's directory listing is unchanged: no lock file;
  * `KeptBackups` lists a planted kept backup;
  * with `SymlinkSourcePolicy`, both are refused.
* The process test that 0041-PLAN added runs `CleanupPending` and `KeptBackups` too, with the real `os.Executable` and `os.Args[0]`. Those come from Linux's `os.Args[0]` and from macOS's and Windows's `os.Executable`.
* Each new assertion is seen to fail on a copy with one change planted by `scripts/plant-copy.sh`:
  * the old message;
  * `CleanupPending` without D5-A;
  * `KeptBackups` without D5-A;
  * `SymlinkSourcePolicy` treated as followable.
* The changed tests pass on the macOS development host, a Linux test host and the Windows test host.
* `make gate` ends `overall=0`. `make apicheck` reports `SymlinkInvocationError`, `SymlinkSource` and its three constants as additions, and nothing incompatible.

### Amendment A1 pros and cons

* **D3-A:** Good, because it follows `RateLimitError` and the module's JSON preference. Bad, because it adds a type and three constants to the API.
* **D3-B:** Good, because a compact enum. Bad, because a JSON reader sees a number.
* **D3-C:** Good, because no new type. Bad, because it parses the module's own text, and breaks when the text changes.
* **D3-D:** Good, because `errors.Is` alone tells the source. Bad, because three more sentinels, and no paths.
* **D4-A:** Good, because each reader gets a true statement, and the user an instruction they can follow. Bad, because the text changes again after `v1.25.0`.
* **D4-B:** Good, because the policy case gets a direction. Bad, because "the configured path" may be one that the reader cannot change.
* **D4-C:** Good, because it is the smallest change. Bad, because case 3 stays wrong.
* **D4-D:** Good, because the core text is unchanged. Bad, because only `cli` users are helped, and the texts drift.
* **D5-A:** Good, because startup stops failing, and nothing is written through the link. Bad, because a skipped cleanup is silent.
* **D5-B:** Good, because leftovers are cleaned at once. Bad, because it writes in the link target's tree.
* **D5-C:** Good, because it is the simplest. Bad, because kept backups go unreported.
* **D5-D:** Good, because no library change. Bad, because every program must handle it, and the docs must teach it.
