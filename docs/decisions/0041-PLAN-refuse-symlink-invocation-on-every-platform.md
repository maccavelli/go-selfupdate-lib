---
status: complete
date: 2026-11-09
associated-madr: "0041-MADR-refuse-symlink-invocation-on-every-platform.md"
---
<!-- markdownlint-disable MD004 MD013 MD024 MD060 -->

# Implement the refusal of an update started through a symlink on every platform

Associated MADR: [0041-MADR-refuse-symlink-invocation-on-every-platform.md](0041-MADR-refuse-symlink-invocation-on-every-platform.md)
(D1-A and D2-A, which the owner accepted as proposed, and this plan approved, on 2026-11-09).

## Goal

A program started through a symlink to its binary is refused with `ErrSymlinkInvocation` on Linux, as it already is on macOS and Windows. A test that uses the real `os.Executable` holds this, and the refusal is documented. Issue #9 is closed by the commit that fixes it.

## Scope

In scope:

* `selfupdate/target.go`: the invoked-path check (MADR D1-A).
* `selfupdate/replace.go`: the `osArgs0` and `lookPath` fields of `fsOps`, bound in `realFS`.
* `selfupdate/errors.go`: `ErrSymlinkInvocation`.
* `selfupdate/updater.go`: the `symlink-invocation` failure class.
* `selfupdate/types.go`, `selfupdate/standalone.go`, `selfupdate/doc.go`: documentation of the refusal and of `ExecutablePath` as the override.
* `selfupdate/target_test.go`, `selfupdate/replace_native_test.go` (its `TestMain`), and `selfupdate/events_test.go` (the failure classes).
* `docs/architecture.md` (install path, and the `failed` event's class list), `docs/guides/extending-selfupdate.md` (beside `AllowSpecialModeBits`).
* Phase 3 only: the `v1.25.0` release documents.

Out of scope:

* D1-B (`AT_EXECFN`), and a symlinked directory or a hard link in the path (MADR More Information).
* The managed installer: `NewManagedInstaller` wraps a `*StandaloneInstaller` (`selfupdate/managed.go:43-47`), and `StandaloneInstaller` reaches its target only through `resolveTarget` (`standalone.go:56`, `:82`, `:115`), so it gets the fix without a change of its own. Phase 1 step 6 confirms this on the changed tree.
* Pushing, tagging and closing the issue, which need the owner's ask in the same turn.

## Implementation Steps

### Phase 1: the check, the API and the documentation

1. **`errors.go`.** Add `ErrSymlinkInvocation = errors.New("selfupdate: refusing to update through a symlink")`, with a comment that names the override and cites `0041-MADR D2-A`.
2. **`replace.go`.** Add to `fsOps`, beside `osExecutable`:
   * `osArgs0 func() string`, bound in `realFS` to a function that returns `os.Args[0]`, or `""` when `os.Args` is empty;
   * `lookPath func(string) (string, error)`, bound to `exec.LookPath`.

   Update the field comment at `:37-38`.
3. **`target.go`.**
   * The existing refusal at `:20-22` returns `fmt.Errorf("%w: %s links to %s; run %s, or set TargetPolicy.ExecutablePath to it", ErrSymlinkInvocation, raw, resolved, resolved)`. `resolved` comes from `filepath.EvalSymlinks(raw)`; when that fails, the message names `raw` alone.
   * After `resolvedInfo` is read (`:34-40`), and only when `policy.ExecutablePath` is empty, call a new `fx.invokedSymlink(resolvedInfo) (string, bool)`. It returns the invoked path and true when, as MADR D1-A says:
     * `fx.osArgs0()` is not empty;
     * it is made absolute against the working directory when it contains a separator (`strings.ContainsRune(a0, os.PathSeparator)`, or `'/'` on Windows too), or found with `fx.lookPath` otherwise, where any error means false;
     * `os.Lstat` of it reports `ModeSymlink`;
     * `os.Stat` of it succeeds and `os.SameFile` with `resolvedInfo`.

     When it returns true, `resolveTarget` refuses with the same wrapped message, naming the invoked path and `resolved`.
   * The function's comment cites `0041-MADR D1-A` and says why `os.Args[0]` can only add a refusal.
4. **`updater.go`.** Add `{ErrSymlinkInvocation, "symlink-invocation"}` to `failureClasses` after `ErrManagedInstall`.
5. **Documentation in code.**
   * `TargetPolicy` (`types.go:457`): a paragraph saying that an update started through a symlink to the binary is refused with `ErrSymlinkInvocation`, on every platform, and that setting `ExecutablePath` to the resolved path updates it on purpose. `ExecutablePath`'s comment says that when it is set, `os.Args[0]` is not read.
   * `NewStandaloneInstaller` (`standalone.go:29-30`) and the `# Installers` section of `doc.go` (`:155-175`): one sentence each, pointing at `TargetPolicy`.
6. **The managed installer.** Re-read the callers of `resolveTarget` on the changed tree, and confirm they are still `standalone.go`'s three and `revalidateTarget`. If another path to the target has appeared, stop: that is a deviation, recorded before going on.
7. **Unit tests, `target_test.go`.** One table test, `TestResolveTargetInvokedSymlink`, with `homeFS(home)` given an `osExecutable` that returns the real binary (as Linux does), and the `osArgs0` and `lookPath` of each case:

   | Case | `osArgs0` | `lookPath` | `ExecutablePath` | Want |
   | :--- | :--- | :--- | :--- | :--- |
   | absolute link | the link | unused | empty | `ErrSymlinkInvocation` |
   | relative link | `./alias`, from a test-local working directory (see below) | unused | empty | `ErrSymlinkInvocation` |
   | bare name | `alias` | returns the link | empty | `ErrSymlinkInvocation` |
   | bare name, not found | `alias` | `exec.ErrNotFound` | empty | resolved |
   | bare name, `ErrDot` | `alias` | `exec.ErrDot` | empty | resolved |
   | link to another file | a link to a second binary | unused | empty | resolved |
   | missing path | a path that does not exist | unused | empty | resolved |
   | empty | `""` | unused | empty | resolved |
   | real path | the real binary | unused | empty | resolved |
   | override | the link | unused | the real binary | resolved |

   The relative case changes the working directory, so it is not `t.Parallel()`; it uses `t.Chdir`. Change `TestResolveTargetRejectsSymlink` to want `errors.Is(err, ErrSymlinkInvocation)`, not any error.
8. **Process test.** In `replace_native_test.go`'s `TestMain`, add a mode on `SELFUPDATE_TEST_RESOLVE_TARGET=1`: it calls `realFS().resolveTarget(TargetPolicy{AllowedRoots: []string{os.Getenv("SELFUPDATE_TEST_ROOT")}})` and prints `ok` or `symlink` (when `errors.Is(err, ErrSymlinkInvocation)`) or `err <message>`, then exits 0. In `target_test.go`, `TestResolveTargetRealExecutableThroughSymlink` copies the test binary into `<tmp>/real/demo[.exe]`, links `<tmp>/link/demo[.exe]` to it (skipping when `os.Symlink` fails), and runs:
   * the real path: want `ok`;
   * the link by absolute path: want `symlink`;
   * not on Windows (MADR F3): the link as `./demo` with `cmd.Dir` the link's directory, and as `demo` with `PATH` the link's directory first: want `symlink`.

   Each run uses `testproc.Budget` for its context. The helper runs to completion and is waited on with `cmd.Output`, so `testproc.Start`, which waits for a ready line from a long-lived helper, does not apply. Phase 1's handoff confirms that against `TestNoDirectFixtureBuild` and `internal/testproc`'s rules.
9. **Failure class test.** Add `fmt.Errorf("x: %w", ErrSymlinkInvocation): "symlink-invocation"` to the table in `selfupdate/events_test.go:168-185`.
10. **`docs/architecture.md`.** In the install path (`:488-530`), a bullet: the target is refused when the program was started through a symlink to it, checked on `os.Executable`'s result and on `os.Args[0]`. Add `symlink-invocation` to the `failed` event's class list (`:480-487`), after `managed-install`.
11. **`docs/guides/extending-selfupdate.md`.** Beside the `AllowSpecialModeBits` paragraph (`:590-600`), a paragraph on the refusal and the `ExecutablePath` override.
12. **Checks and commit.** `gofmt`, then `make pre-add-check FILES="…"` on every changed `.go` file, `make gate` (`overall=0`), `make markdownlint`, `make check-records` and `make docs-check`. `make apicheck` must list `ErrSymlinkInvocation` as the only change. Commit with `git commit --no-edit`. The PLAN's execution record and its status change to `in-progress` go in the same commit.

### Phase 2: live verification and the planted failure

1. On the Linux test host, run `go test -count=1 -run 'TestResolveTarget' ./selfupdate/` on a bundle of the Phase 1 tree. Both new tests must pass.
2. On the Linux test host, make a copy with `scripts/plant-copy.sh selfupdate/target.go` that removes the call to `invokedSymlink`, and run the same tests there. Both must fail: the unit test's three link cases, and the process test's link runs, with `ok` where `symlink` was wanted. Record the output.
3. On macOS and the Windows test host, run the same tests on the Phase 1 tree. They must pass, with the process test's Windows run either passing or skipping because `os.Symlink` needs a privilege. Record which.
4. Reproduce issue #9 end to end on the Linux test host. Build a scratch consumer, outside the tree, that composes an `Updater` from `selfupdatetest.NewFakeSource` with a newer release and `NewStandaloneInstaller(InstallOptions{})`, stamped as an older version. Start it through a link under a temporary home, with `HOME` set to it. On the Phase 1 tree it must fail with `ErrSymlinkInvocation` and leave the target's bytes unchanged; on `v1.24.0` it must update, as the issue reports. If the scratch consumer needs a change to this module, record that as a deviation and stop.
5. Record every result in this PLAN's execution record, with the output rather than a summary. No commit is made in this phase except the record, which is documentation.

### Phase 3: prepare `v1.25.0`

1. `CHANGELOG.md`: a `v1.25.0` entry. Added: `ErrSymlinkInvocation`, and the `symlink-invocation` class. Changed: the refusal on Linux and on any platform whose `os.Executable` resolves links, its message, and the `failed` event's detail for it. Fixed: issue #9.
2. `docs/guides/migrating-from-mcplib-selfupdate.md`: section `25. From v1.24 to v1.25`, in the form of section 24. It says that a Linux program started through a symlink is now refused, and how to update it: run the target, or set `TargetPolicy.ExecutablePath`. Update the guide's other version references, as the `v1.24.0` preparation did (`d752e65`).
3. `README.md`, `docs/architecture.md` and `docs/guides/building-releases.md`: the version references, as `d752e65` changed them.
4. Run Phase 1 step 12's checks, then commit with `git commit --no-edit`. Set this PLAN to `complete` in that commit. (The MADR was set to `accepted` when the owner accepted it, before Phase 1.)
5. Run the pre-push disclosure guard (AGENTS.md "Identifiers") over the outgoing commits, and report the result. Do not push or tag unless the owner asks in the same turn.

*Deviation 4 split steps 2 to 4 in two. Phase 3a, step 1 and step 2's section, was the commit the owner tagged. Phase 3b, after the tag and its green CI, is step 2's other references, step 3, and step 4's status change.*

## Verification

* Unit tests: every row of Phase 1 step 7's table gives the result it names.
* Process test: through the link it refuses with the real `os.Executable`, on the Linux test host and Linux CI. It was seen to fail on the Linux test host without the fix (Phase 2 step 2).
* End to end: issue #9's reproduction fails with `ErrSymlinkInvocation` and leaves the target unchanged (Phase 2 step 4).
* `make gate` ends `overall=0`; `make markdownlint`, `make check-records` and `make docs-check` pass; `make apicheck` reports the one addition.
* CI, after the owner pushes: the Linux, macOS, Windows and `go126` jobs pass.

## Rollout and Rollback

* Rollout is the `v1.25.0` release. A program gets the change when it moves to that version. Its Linux users started through a link see the refusal and the advice in its message.
* Rollback, before a tag: revert Phase 1's commit. After a tag: a `v1.25.1` that reverts the check and keeps `ErrSymlinkInvocation` and the class, since an exported name cannot be removed within `v1`. A program can also stay on `v1.24.0`, or set `TargetPolicy.ExecutablePath` to `os.Executable`'s result, which bypasses the new check.

## Execution Record

The owner accepted the MADR and approved this plan on 2026-11-09. Phase 1 and Phase 2 ran that day, on a tree at `c0bfb54` plus the changes below, before any commit.

### Phase 1

* Steps 1 to 5 landed as written: `ErrSymlinkInvocation` (`errors.go`); `fsOps.osArgs0` and `fsOps.lookPath`, bound to `os.Args[0]` and `exec.LookPath` (`replace.go`); `invokedSymlink`, `symlinkRefusal` and `symlinkRefusalTo` (`target.go`), with the check after the resolved target's regular-file check and only when `ExecutablePath` is empty; the `symlink-invocation` class after `managed-install` (`updater.go`); and the documentation in `types.go`, `standalone.go` and `doc.go`.
* Step 6: the callers of `resolveTarget` on the changed tree are `standalone.go:58`, `:84`, `:117` and `revalidateTarget` (`target.go:303`). No other path to the target.
* Steps 7 to 9: `TestResolveTargetInvokedSymlink` (ten cases), `TestResolveTargetRealExecutableThroughSymlink` (four runs), the `SELFUPDATE_TEST_RESOLVE_TARGET` mode in `TestMain`, and the `symlink-invocation` row in `TestFailureClass`. `TestResolveTargetRejectsSymlink` now wants `ErrSymlinkInvocation`. The tests reuse `install_hardening_test.go`'s `symlinkOrSkip`, which skips only on Windows when a link cannot be made.
* Steps 10 and 11: `docs/architecture.md` gets an install-path bullet and the class; `docs/guides/extending-selfupdate.md` gets the section "Update a program started through a symlink", after the setuid section.
* Step 12: `make gate` ended `overall=0`. Each step: gofmt, lint (`0 issues.`), vet, race, shuffle, tidy, apicheck (`compatible with v1.24.0`), fuzz, vuln (`No vulnerabilities found.`), scripts, shellcheck, crossvet, links (`935 links in 108 files, 0 broken`), arch (`21 rows … 0 without a row`), ids (`15 files, 20 deny-list rules, 0 findings`). `make apicheck` prints compatibility, not a list of additions. The diff's only new exported identifier is `ErrSymlinkInvocation`; `osArgs0`, `lookPath` and the three functions are unexported.
* The owner commits and pushes Phase 1 to `main`; see Deviation 3.

### Phase 2

* Step 1, the Linux test host (go1.27.3 linux/amd64): every `TestResolveTarget*` test and `TestFailureClass` passed, all ten `TestResolveTargetInvokedSymlink` cases and all four `TestResolveTargetRealExecutableThroughSymlink` runs included.
* Step 2, the planted failure: `scripts/plant-copy.sh selfupdate/target.go` replaced `if policy.ExecutablePath == "" {` before the `invokedSymlink` call with `if false {`. The copy ran on the Linux test host:

  ```text
  --- FAIL: TestResolveTargetInvokedSymlink (0.00s)
      --- FAIL: TestResolveTargetInvokedSymlink/absolute_link (0.00s)
          target_test.go:106: err = <nil>, want ErrSymlinkInvocation
      --- FAIL: TestResolveTargetInvokedSymlink/relative_link (0.00s)
          target_test.go:106: err = <nil>, want ErrSymlinkInvocation
      --- FAIL: TestResolveTargetInvokedSymlink/bare_name (0.00s)
          target_test.go:106: err = <nil>, want ErrSymlinkInvocation
  --- FAIL: TestResolveTargetRealExecutableThroughSymlink (0.02s)
      --- FAIL: TestResolveTargetRealExecutableThroughSymlink/bare_name_on_PATH (0.01s)
          target_test.go:182: helper printed "ok", want "symlink"
      --- FAIL: TestResolveTargetRealExecutableThroughSymlink/absolute_link (0.01s)
          target_test.go:182: helper printed "ok", want "symlink"
      --- FAIL: TestResolveTargetRealExecutableThroughSymlink/relative_link (0.01s)
          target_test.go:182: helper printed "ok", want "symlink"
  FAIL
  ```

  Only the link cases failed; the seven cases that must resolve still passed. The copy was removed.
* Step 3: on macOS (go1.27.3 darwin/arm64) and the Windows test host (go1.27.3 windows/amd64) the same tests passed. On Windows the links were made, so nothing skipped, and all four process runs ran (Deviation 2).
* Step 4, issue #9 end to end on the Linux test host: a scratch consumer outside the tree composed `selfupdatetest.NewFakeSource` (a `v1.1.0` release), `NewStandaloneInstaller(InstallOptions{})` and a request from `v1.0.0` with `Yes`. It was built into `<home>/.local/bin/demo`, linked from `<home>/link/demo`, and run through the link with `HOME` set to `<home>`, a new temporary directory:

  ```text
  == v1.24.0: run through the link, exit 0
      selfupdate: installing product=demo target=v1.1.0 asset=demo-linux-amd64
      selfupdate: complete product=demo current=v1.0.0 target=v1.1.0 asset=demo-linux-amd64 release asset integrity verified
      run-error: <nil>
     target sha256 before=e3e5955d00fe4e88 after=1569788e57870102 unchanged=False
     target begins b'demo v1.1.0 linux/amd64\n'
  == fixed: run through the link, exit 0
      selfupdate: resolving-target product=demo current=v1.0.0
      selfupdate: failed product=demo current=v1.0.0 symlink-invocation
      run-error: selfupdate: demo: selfupdate: refusing to update through a symlink: <home>/link/demo links to <home>/.local/bin/demo; run <home>/.local/bin/demo, or set TargetPolicy.ExecutablePath to it
      is-symlink-refusal: true
     target sha256 before=bb7f415d4a71c964 after=bb7f415d4a71c964 unchanged=True
  ```

  `v1.24.0` reproduces the issue. The fixed tree refuses it, leaves the target's bytes unchanged, and reports the class `symlink-invocation`. The scratch consumer needed no change to this module.

### Phase 3

* The owner committed Phase 1 as `b8a5bd0` and the record of Deviation 3 as `6201686`, then said "committed, proceed".
* Step 1: `CHANGELOG.md` has `## v1.25.0 - 2026-11-09`, with Added, Changed and Fixed, linking §25, the MADR and this PLAN. The date moves if the tag is cut on another day.
* Step 2, its first half: the guide has `## 25. From v1.24 to v1.25`, in §24's form, with "What is new", "Behaviour you may notice", "Adopting it" and "Check". Its second half, the guide's other version references and the release-history line, waits for the pin commit (Deviation 4).
* Step 3 waits for the pin commit (Deviation 4).
* Checks on the tree: `scripts/check-release-tag.sh v1.25.0` exit 0; `git tag -l v1.25.0` prints nothing; `make markdownlint` 0 issues; `make docs-check` `943 links in 110 files, 0 broken`, the new `#25-from-v124-to-v125` anchor included; `scripts/check-docs.sh --ids` on the changelog and the guide, 0 findings.
* Phase 3a: the owner committed and pushed it as `875dec3`. Its Docs run (42671188899) failed on attempts 1 and 2 with no job started. The run page's one annotation was "Internal server error" with a GitHub correlation ID, and the same workflow had passed on `b8a5bd0` at 17:53Z. So the cause was GitHub's, not the tree's. Attempt 3, re-run at the owner's ask, passed at 19:50:39Z: markdownlint, records and index, links and architecture table, home paths.
* The owner tagged `v1.25.0` on `875dec3c9d475ca47c4b012ca43742de832ef626` (annotated, 2026-11-09, the changelog's date), pushed it, and approved Deviation 4. Tag CI run 42687360803 on `875dec3`: `success` at 20:06:21Z, with all 18 jobs passing. They include `validate` on ubuntu-24.04, macos-15 and windows-2025, `go126`, `installer-containers`, and both release rehearsals.

### Phase 3b

* `git diff --name-only v1.24.0 v1.25.0` lists nothing under `.github`, `internal/cmd`, or `scripts/*release*` (the four release scripts and their tests), so the `v1.24.0` guide's statements about the workflows hold for `v1.25.0`.
* Pins, moved from `v1.24.0` and `74ab5dd` to `v1.25.0` and `875dec3c9d475ca47c4b012ca43742de832ef626` by a script that asserts each count before it replaces:
  * `README.md`: the current release, the `go get` line, and the publish pin (1 each).
  * `docs/architecture.md`: the current release and its commit.
  * `docs/guides/building-releases.md`: "examples below pin", the three workflow pins, and the `ls-remote` ref.
  * `docs/guides/migrating-from-mcplib-selfupdate.md`: §2's `go get` line and "current release", §3's publish pin, "`v1.17.2` to `v1.25.0` nothing" and its example pin, the `ls-remote` ref, §5's `go.mod` requirement, the `go get` lines of §6 to §24 (20 in all with §2's) and the `go list` checks of §7 to §24 (18).
  * The guide's release history gains a `v1.25.0` line linking §25.
* Kept as history: `docs/architecture.md`'s "Since `v1.24.0`" (redaction), the guide's `v1.24.0` release-history line, §24's prose, and §25's three mentions of `v1.24.0`.
* Checks on the tree: `make markdownlint` 0 issues; `make check-records` 97 records, 0 findings; `make docs-check` 944 links in 110 files, 0 broken, 21 architecture rows, 0 differ; `make disclosure-check` 739 files, 0 findings; `scripts/check-docs.sh --ids` on the four changed guides and pages, 0 findings.
* Step 5: no commit existed for the guard to scan, since the owner commits Phase 3b and the guard runs on their push. `scripts/check-docs.sh --ids`, which applies the guard's deny list, over the six changed files, including this PLAN and `docs/README.md`'s index row, which now reads `complete`: 0 findings.
* The commit follows Deviation 3: the owner commits and pushes.

### Deviations

* **Deviation 1 (2026-11-09), Phase 1 step 7, the relative case.** The step said the relative case would use `t.Chdir`, and so could not be parallel. Instead it takes `filepath.Rel` from the working directory to the link. The code path is the same, a relative `os.Args[0]` made absolute against the working directory, and the test stays parallel. No decision changes.
* **Deviation 2 (2026-11-09), Phase 1 step 8, Windows.** The step left the relative and bare-name runs out on Windows, citing MADR F3, where Python's `CreateProcess` could not start them. The test sets `cmd.Path` to the link and `cmd.Args[0]` to the relative path or bare name, so the start does not depend on how a name is searched. All four runs passed on the Windows test host. MADR F3 describes the probe and stays true.
* **Deviation 3 (2026-11-09), Phase 1 step 12, the commit.** Step 12 had the agent commit with `git commit --no-edit`. The `enforcing-git-branch-gates` policy forbids an agent commit on `main` without a `feature/` branch and a Jira key. The owner chose the `sysadmin-git-branch-gates` policy instead, which needs neither, and chose to commit and push Phase 1 to `main` themselves. The agent ran `make pre-add-check` on the changed Go files and left the tree uncommitted. Phase 3's commit follows the same arrangement unless the owner says otherwise.
* **Deviation 4 (2026-11-09), Phase 3 steps 2 to 4, the pins. Approved by the owner on 2026-11-09, after they tagged `v1.25.0`.** Steps 2 and 3 put the version references in the same commit as the changelog. Those references pin the full SHA of the commit the tag is on (`README.md`'s publish pin, `docs/architecture.md`'s "annotated tag on commit", `docs/guides/building-releases.md`'s three workflow pins, the guide's §3 pin). A commit cannot name its own SHA. The `v1.24.0` release split the work in two (0040-PLAN-redact-env-in-formatted-values.md E1 and E2): the changelog and the guide's section went into the commit that was tagged (`74ab5dd`), and a pin commit after the tag (`d752e65`) moved the references and added the release-history line. The proposed resolution is the same split. Phase 3a is steps 1 and 2's section, in the commit to be tagged. Phase 3b, after the owner's tag and its green CI, checks that `.github`, `internal/cmd` and the four release scripts are unchanged since `v1.24.0`, moves every pin to `v1.25.0` and the tag's commit, adds the release-history line, and sets this PLAN to `complete`.

### Follow-up

* **Issue #10 (2026-11-09), after this PLAN was complete.** It found that the message this PLAN shipped names `TargetPolicy.ExecutablePath` to a CLI's user. The message also says "run `<target>`" when `ExecutablePath` is itself the link, and `CleanupPending` and `KeptBackups` fail with it through a link. On Linux, that last failure came with `v1.25.0`. This PLAN stays `complete`: it did what it set out to do, and its checks passed. The fixes are proposed in 0041-MADR's Amendment A1 and planned in [0041-PLAN-type-the-symlink-refusal-and-spare-startup-maintenance.md](0041-PLAN-type-the-symlink-refusal-and-spare-startup-maintenance.md).
