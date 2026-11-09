---
status: accepted
date: 2026-11-09
decision-makers: go-selfupdate-lib maintainers
consulted: "issue #9, opened 2026-11-09; selfupdate/target.go, replace.go, types.go, standalone.go, doc.go, updater.go, spawn_test.go and target_test.go at c0bfb54; Go 1.27.3's os/executable*.go; probe P1, run on 2026-11-09 on the macOS development host, a Linux test host and the Windows test host"
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
* Both refusals, of `raw` and of the invoked path, wrap it: `%w: <link> links to <target>; run <target>, or set TargetPolicy.ExecutablePath to it`.
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
* Plan: [0041-PLAN-refuse-symlink-invocation-on-every-platform.md](0041-PLAN-refuse-symlink-invocation-on-every-platform.md).
* A directory symlink in the path (`~/bin` → `~/.local/bin`) is not refused today on any platform: `os.Lstat` reports only the final component. D1-A keeps that.
* A hard link cannot be told from the file it names, and is not refused on any platform. This record does not change that.
