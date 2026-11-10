---
status: in-progress
date: 2026-11-09
associated-madr: "0041-MADR-refuse-symlink-invocation-on-every-platform.md"
---
<!-- markdownlint-disable MD004 MD013 MD024 MD060 -->

# Implement Amendment A1: a typed symlink refusal, a message for each source, and startup maintenance through a link

Associated MADR: [0041-MADR-refuse-symlink-invocation-on-every-platform.md](0041-MADR-refuse-symlink-invocation-on-every-platform.md)
(Amendment A1, D3-A, D4-A and D5-A, chosen by the owner on 2026-11-09).

This is the MADR's second PLAN. The first, [0041-PLAN-refuse-symlink-invocation-on-every-platform.md](0041-PLAN-refuse-symlink-invocation-on-every-platform.md), is complete and shipped `v1.25.0`. This one answers issue #10's proposals P1 to P3 and P5's symlink part. P4, and P5's rule test, are [0042-PLAN-keep-go-identifiers-out-of-run-time-messages.md](0042-PLAN-keep-go-identifiers-out-of-run-time-messages.md), which runs after this one.

## Goal

* **The refusal is a typed error.** `*SymlinkInvocationError` has `Link`, `Target` and `Source`. `errors.Is(err, ErrSymlinkInvocation)` and the `symlink-invocation` class are unchanged (D3-A).
* **Its message suits each source and names no Go identifier** (D4-A).
* **`CleanupPending` and `KeptBackups` no longer fail when the program was started through a link.** That covers a link found by `os.Executable` or `os.Args[0]`, but not one in `TargetPolicy.ExecutablePath`. `CleanupPending` writes nothing then, and `KeptBackups` lists the target's backups (D5-A).
* **Each new assertion is seen to fail** on a planted copy. The changed tests pass on the macOS development host, a Linux test host and the Windows test host.
* **`v1.26.0` carries these changes and 0042's together,** as the owner chose on 2026-11-09. That release is made by [0042-PLAN-keep-go-identifiers-out-of-run-time-messages.md](0042-PLAN-keep-go-identifiers-out-of-run-time-messages.md) Phases B3a and B3b, which replace this PLAN's Phases A3a and A3b.

## Scope

### In scope

| Phase | What | Commit |
| :--- | :--- | :--- |
| A0 | the records: the MADR's Amendment A1, this PLAN, a follow-up line in the first PLAN, the 0042 pair, and their rows in `docs/README.md` | records only |
| A1 | the code, the tests, the godoc, `docs/architecture.md`, `docs/guides/extending-selfupdate.md`, this PLAN's record | one commit |
| A2 | live runs, planted failures, the probe again; this PLAN's record | records only |
| ~~A3a~~ | moved to 0042's PLAN, Phase B3a, which writes one `v1.26.0` entry for both (owner, 2026-11-09) | — |
| ~~A3b~~ | moved to 0042's PLAN, Phase B3b, which also sets this PLAN to `complete` | — |

Files that Phase A1 changes:

* `selfupdate/errors.go`: `SymlinkSource` and its constants, `SymlinkInvocationError`, and `ErrSymlinkInvocation`'s comment.
* `selfupdate/target.go`: the two refusals build the typed error; `symlinkRefusal` and `symlinkRefusalTo` are removed.
* `selfupdate/standalone.go`: `CleanupPending`, `KeptBackups` and their comments, and a helper ~~`linkStart`~~ `linkTarget` (Deviation 1).
* `selfupdate/types.go`: `TargetPolicy`'s and `ExecutablePath`'s comments.
* `selfupdate/doc.go`: the Installers section's sentence.
* Tests: `selfupdate/target_test.go`, `selfupdate/keptbackups_test.go`, `selfupdate/events_test.go`, and `selfupdate/replace_native_test.go` (its `TestMain` helper).
* Docs: `docs/architecture.md` (the install path), and `docs/guides/extending-selfupdate.md` ("Keep the previous binary" and "Update a program started through a symlink").

No `.go` file is added or removed, so `docs/architecture.md`'s package table keeps its counts.

### Out of scope

* **0042-MADR's rule** for the other run-time messages, and its source-scan test.
* **The `--json` result document.** Its `schema_version` stays. The `error` string carries both paths (MADR Amendment A1, D3 options).
* **`cli`.** It prints the new text with no change of its own (FA1.2).
* **`ManagedInstaller`.** Its `KeptBackups` delegates (FA1.3), and it has no `CleanupPending`.
* **Pushes, tags and closing issue #10,** which happen only on the owner's word in the same turn.

## Rules

* Mutation starts only after the owner accepts the amendment and approves this PLAN.
* The owner commits and pushes (0041-PLAN-refuse-symlink-invocation-on-every-platform.md Deviation 3). The agent runs the checks and leaves the tree for them.
* A deviation stops the work: it is recorded here, dated, and the owner decides before it goes on.
* Nothing committed names a host, an account or a real-machine path. Probe output uses `<tmp>`.

## Implementation Steps

### Phase A0: the records

1. The records listed in the Scope, as written on 2026-11-09. `make markdownlint`, `make check-records`, `make docs-check` and `make disclosure-check` pass.

### Phase A1: the code, the tests and the documentation

1. **`selfupdate/errors.go`.** After `HTTPError`, add:

   ```go
   // SymlinkSource says how a refused symlink was found.
   type SymlinkSource string

   const (
       // SymlinkSourceExecutable: os.Executable returned the link (macOS, Windows).
       SymlinkSourceExecutable SymlinkSource = "executable"
       // SymlinkSourceInvocation: os.Args[0] named the link, and os.Executable
       // had resolved it (Linux, and any OS whose os.Executable resolves links).
       SymlinkSourceInvocation SymlinkSource = "invocation"
       // SymlinkSourcePolicy: TargetPolicy.ExecutablePath is the link.
       SymlinkSourcePolicy SymlinkSource = "policy"
   )

   // SymlinkInvocationError is the refusal to update through a symlink.
   // errors.Is(err, ErrSymlinkInvocation) holds for it.
   type SymlinkInvocationError struct {
       Link   string        `json:"link"`             // the symlink, absolute
       Target string        `json:"target,omitempty"` // what it resolves to; empty when it does not resolve
       Source SymlinkSource `json:"source"`
   }
   ```

   * `Error()` returns `ErrSymlinkInvocation.Error()`, followed by one of three texts:
     * when `Target` is empty, `: <Link> is a symlink`;
     * when `Source` is `SymlinkSourcePolicy`, `: the configured executable path <Link> links to <Target>`;
     * otherwise `: <Link> links to <Target>; start <Target> itself to update it`.

     A nil receiver returns the sentinel's text, as `RateLimitError` does (FA1.7).
   * `Unwrap()` returns `ErrSymlinkInvocation`.
   * Each declaration's comment says what the MADR does, and cites `0041-MADR Amendment A1 D3-A, D4-A`.
   * The type's comment carries the remedy for the program's author. To update through a link on purpose, set `TargetPolicy.ExecutablePath` to `Target`. With `SymlinkSourcePolicy`, the configured path is the link.
   * `ErrSymlinkInvocation`'s comment says it is returned as a `*SymlinkInvocationError`, and names `CleanupPending` and `KeptBackups`'s behaviour (D5-A).
2. **`selfupdate/target.go`.**
   * At `:20-21`, the source is `SymlinkSourcePolicy` when `policy.ExecutablePath` is set, and `SymlinkSourceExecutable` otherwise. Return `symlinkError(raw, source)`.
   * At `:42-43`, return `&SymlinkInvocationError{Link: invoked, Target: resolved, Source: SymlinkSourceInvocation}`.
   * Replace `symlinkRefusal` and `symlinkRefusalTo` (`:117-135`) with `symlinkError(link string, source SymlinkSource) *SymlinkInvocationError`. It sets `Target` to `filepath.Abs(filepath.EvalSymlinks(link))`, or to "" when either fails.
3. **`selfupdate/standalone.go`.**
   * Add ~~`linkStart(err error) (*SymlinkInvocationError, bool)`: true when~~ `linkTarget(err error) (string, bool)` (Deviation 1): the refusal's `Target` and true when `errors.As` finds a `*SymlinkInvocationError` whose `Source` is not `SymlinkSourcePolicy`, and "", false otherwise. Its comment cites `0041-MADR Amendment A1 D5-A`.
   * `CleanupPending`: when `resolveTarget` fails and `linkTarget` reports true, return nil before `beginSession`.
   * `KeptBackups`: when `resolveTarget` fails, ~~`linkStart` is true and `Target` is~~ `linkTarget` reports true and the target it returns is not empty, copy `s.policy`, set its `ExecutablePath` to that target, and resolve again. That second resolution runs every other check. Then list. Any other failure is returned as before.
   * Both comments say what happens through a link, and why `CleanupPending` writes nothing there (FA1.5, FA1.6).
4. **`selfupdate/types.go`.**
   * `TargetPolicy`'s paragraph (`:459-465`) names `*SymlinkInvocationError`.
   * `ExecutablePath`'s comment says that a link given here is refused under every call, `CleanupPending` and `KeptBackups` included.
5. **`selfupdate/doc.go`.** The Installers sentence (`:166-168`) says that `CleanupPending` and `KeptBackups` do not fail for a program started through a link.
6. **`selfupdate/target_test.go`.**
   * `TestResolveTargetInvokedSymlink`: the refused rows check `errors.As` with `Source == SymlinkSourceInvocation`, `Link` the invoked path and `Target` the real binary, and `Error()` exactly.
   * `TestResolveTargetRejectsSymlink`: `Source == SymlinkSourcePolicy`, and `Error()` exactly, with no "start".
   * New `TestResolveTargetExecutableSymlink`: `homeFS` with `osExecutable` returning the link, as macOS does, gives `SymlinkSourceExecutable`.
   * New `TestSymlinkInvocationErrorText`:
     * a dangling link, given as `ExecutablePath` and as `os.Executable`'s result, gives the unresolved text and an empty `Target`;
     * no text in any case contains `TargetPolicy`;
     * a nil receiver gives the sentinel's text, and `errors.Is` holds through `Unwrap`;
     * `json.Marshal` gives `{"link":…,"target":…,"source":"policy"}`, and omits `target` when it is empty.
7. **`selfupdate/keptbackups_test.go`.** New `TestMaintenanceThroughSymlink`, using `tempTarget`, a link in a second temporary directory, and `fsFor(inst)`:
   * the executable source (`osExecutable` returns the link) and the invocation source (`osExecutable` returns the target, `osArgs0` the link): `CleanupPending` returns nil, and the target's directory listing is the same before and after, with no `.demo.selfupdate.lock`. `KeptBackups` lists a planted `.demo.selfupdate-kept-1`.
   * the policy source (`ExecutablePath` the link): both return `errors.Is(err, ErrSymlinkInvocation)`, with `Source == SymlinkSourcePolicy`.
   * a dangling link from `os.Executable`: `CleanupPending` returns nil, and `KeptBackups` returns the refusal.
8. **`selfupdate/events_test.go`.** Add the row `fmt.Errorf("resolve: %w", &SymlinkInvocationError{…}): "symlink-invocation"` to `TestFailureClass`.
9. **The process test.**
   * `replace_native_test.go`: `resolveTargetHelper` reads `SELFUPDATE_TEST_OP`, where empty means `resolve`. `cleanup` and `kept` call `CleanupPending` and `KeptBackups` on `NewStandaloneInstaller` with the same policy. It prints `ok`, `ok <n>` for `kept`, `symlink <source>`, or `err <message>`.
   * `target_test.go`: factor the copy-and-link setup of `TestResolveTargetRealExecutableThroughSymlink` into a helper. Each subtest gets its own directory. It runs `resolve`, `cleanup` and `kept`, by the real path and through the link by absolute path, relative path and bare name.
   * Through the link the results are:
     * `resolve`: `symlink invocation` on Linux, and `symlink executable` on macOS and Windows, by `runtime.GOOS`;
     * `cleanup`: `ok`, with the target's directory unchanged;
     * `kept`: `ok 1`, with a kept backup planted.
10. **Docs.**
    * `docs/architecture.md`'s install-path bullet (`:490-494`) names `*SymlinkInvocationError` and its source. It says that `CleanupPending` and `KeptBackups` do not fail for a link start.
    * `docs/guides/extending-selfupdate.md`:
      * "Update a program started through a symlink" (`:603-617`) gives the three texts, `errors.As`, and the remedy;
      * "Keep the previous binary" (`:561-566`) says what `CleanupPending` and `KeptBackups` do through a link.
11. **Checks.**
    * `make pre-add-check FILES="<every changed .go file>"`.
    * `make gate`, ending `overall=0`.
    * `make markdownlint`, `make check-records`, `make docs-check` and `make disclosure-check`.
    * `make apicheck` lists exactly `SymlinkInvocationError`, its methods, `SymlinkSource` and its three constants, all compatible.
    * The record of this phase goes in this PLAN, in the same commit. The owner commits.

### Phase A2: live runs and planted failures

1. On the Linux test host and the Windows test host, run `go test -count=1 -run 'TestResolveTarget|TestSymlinkInvocationError|TestMaintenanceThroughSymlink|TestKeptBackups|TestFailureClass' ./selfupdate/` on a bundle of the Phase A1 tree, and the same on the macOS development host. Record each result, and which process subtests ran or skipped. On Windows a subtest may skip when `os.Symlink` needs a privilege.
2. With `scripts/plant-copy.sh`, make four copies, each with one change. Run the tests from step 1 in each, on the Linux test host. Each copy must fail the named test:
   * `Error()`'s default text returned to `v1.25.0`'s: `TestResolveTargetInvokedSymlink` and `TestSymlinkInvocationErrorText`;
   * `CleanupPending` without its ~~`linkStart`~~ `linkTarget` branch: `TestMaintenanceThroughSymlink` and the process test's `cleanup` cases;
   * `KeptBackups` without its branch: the same tests' `kept` cases;
   * ~~`linkStart`~~ `linkTarget` true for `SymlinkSourcePolicy`: `TestMaintenanceThroughSymlink`'s policy case.

   The exact OLD and NEW strings come from the Phase A1 code, and each must occur once (the script's header). Record each failure's output.
3. Run probe A1-P1 (MADR FA1.4) on the Phase A1 tree, on the macOS development host and the Linux test host, with each case in a fresh directory. Through the link, cases 2 and 3 must give D4-A's texts. Case 2 must give `KeptBackups` 1 and `CleanupPending` nil, and create no lock file. Case 3 must still refuse all three calls.
4. Record every result in this PLAN, with output, not a summary. No commit but the record.

### Phase A3a: prepare `v1.26.0`

*Not run here. On 2026-11-09 the owner chose one `v1.26.0` for this PLAN and 0042's, so 0042-PLAN-keep-go-identifiers-out-of-run-time-messages.md Phase B3a does this phase's work for both. The steps stay below as written, and B3a covers each of them.*

1. `CHANGELOG.md`: a `## v1.26.0 - <date>` entry.
   * Added: `SymlinkInvocationError`, `SymlinkSource` and its constants.
   * Changed: the refusal's text, by source.
   * Fixed: issue #10's points, startup maintenance through a link included. It notes that this was new on Linux in `v1.25.0`.

   It links §26, the MADR's Amendment A1 and this PLAN.
2. `docs/guides/migrating-from-mcplib-selfupdate.md`: `## 26. From v1.25 to v1.26`, in §25's form, with "What is new", "Behaviour you may notice", "Adopting it" and "Check".
3. `scripts/check-release-tag.sh v1.26.0` exits 0, and `git tag -l v1.26.0` prints nothing. Run the checks of Phase A1 step 11 that apply to Markdown. The owner commits, pushes, tags `v1.26.0` and pushes the tag.

### Phase A3b: the pins, after the tag's CI is green

*Not run here, for the same reason. 0042-PLAN-keep-go-identifiers-out-of-run-time-messages.md Phase B3b does it, and sets this PLAN to `complete` in the same commit. That is once Phases A1 and A2 here are recorded, and the tag's CI is green.*

1. Check that `git diff --name-only v1.25.0 v1.26.0` lists nothing under `.github`, `internal/cmd` or `scripts/*release*`. If it does, stop: the guide's statements about the workflows need re-reading. That is a deviation.
2. Move the version references from `v1.25.0` and its commit to `v1.26.0` and its commit, with a script that asserts each count, as 0041-PLAN-refuse-symlink-invocation-on-every-platform.md Phase 3b did, in the same files:
   * `README.md`;
   * `docs/architecture.md`;
   * `docs/guides/building-releases.md`;
   * the migration guide's pins, `go get` lines and `go list` checks.

   Add a `v1.26.0` release-history line, and keep history lines as they are.
3. Set this PLAN to `complete`, and its row in `docs/README.md`. Run the four documentation checks and `scripts/check-docs.sh --ids` on the changed files. The owner commits and pushes. Closing issue #10 is the owner's, or the agent's when asked in that turn, and waits for 0042's PLAN if the owner wants one comment for both.

## Verification

* Every Phase A1 test passes on the three hosts, or skips for the recorded reason (Phase A2 step 1).
* Each planted copy fails the test it names (Phase A2 step 2).
* Probe A1-P1 shows D4-A's texts and D5-A's behaviour on macOS and Linux (Phase A2 step 3).
* `make gate` ends `overall=0`. `make apicheck` reports only the additions.
* CI, after the owner pushes: the Linux, macOS, Windows and `go126` jobs pass, and the tag's run passes.

## Rollout and Rollback

* **Rollout** is `v1.26.0`, which also carries 0042's change.
  * A program gets the new text and the new maintenance behaviour when it moves to that version. `gobble-cli`'s test of `<link> links to ` keeps passing (MADR FA1.8).
  * A program that matched `, or set TargetPolicy.ExecutablePath to it` loses the match. The `v1.25.0` changelog says to match the sentinel.
* **Rollback, before the tag:** revert Phase A1's commit.
* **Rollback, after the tag:** a `v1.26.1` that restores `CleanupPending` and `KeptBackups`'s refusal and the old text. It keeps `SymlinkInvocationError` and its constants, since an exported name stays within `v1`.

## Execution Record

The owner chose Amendment A1's options on 2026-11-09. They committed Phase A0 and 0042's Phase B0 as `b7fdb3e`, and approved execution the same day ("committed, proceed"). Phases A1 and A2 ran on a tree at `b7fdb3e` plus the changes below, before any commit.

### Phase A1

* **Steps 1 to 10,** as written, except step 3's helper (Deviation 1).
  * `selfupdate/errors.go` adds `SymlinkSource`, its three constants and `SymlinkInvocationError`, after `HTTPError`, and rewrites `ErrSymlinkInvocation`'s comment.
  * `selfupdate/target.go` builds the typed error at both refusals. `symlinkError(link, source)` replaces `symlinkRefusal` and `symlinkRefusalTo`.
  * `selfupdate/standalone.go` adds `linkTarget`. `CleanupPending` checks it before `if err != nil`, and `KeptBackups` resolves again with the returned target.
  * Godoc changes in `selfupdate/types.go` and `selfupdate/doc.go`.
  * `docs/architecture.md`'s install-path bullet changes.
  * `docs/guides/extending-selfupdate.md` gains a table of the three sources' texts, and the maintenance behaviour in two sections.
* **Tests.**
  * `target_test.go`:
    * `TestResolveTargetRejectsSymlink` and `TestResolveTargetInvokedSymlink` check the fields and the exact text;
    * new `TestResolveTargetExecutableSymlink` and `TestSymlinkInvocationErrorText`;
    * the helpers `wantSymlinkError`, `startText` and `realPath`;
    * the process test runs `resolve`, `cleanup` and `kept`, each in its own copy, through `symlinkedCopy`, `runResolveHelper` and `dirNames`. Within each copy the link starts run before the real-path start.
  * `keptbackups_test.go`: new `TestMaintenanceThroughSymlink`.
  * `events_test.go`: a row for the typed error. `gofmt -w` realigned the table.
  * `replace_native_test.go`: `resolveTargetHelper` reads `SELFUPDATE_TEST_OP`.
* **No `.go` file was added or removed.**
* **Step 11, checks.**
  * `make pre-add-check FILES="<the 9 changed .go files>"` failed twice before passing, both times on `linkTarget`'s first two forms (Deviation 1). Its third run printed `go-precheck: 9 file(s) clean (gofmt, golangci-lint, go vet, go test, govulncheck).`
  * `make markdownlint` 0 issues.
  * `make check-records` 100 records, 0 findings.
  * `make docs-check` 964 links in 113 files, 0 broken, 21 architecture rows, 0 differ, 0 without a row.
  * `make disclosure-check` 742 files, 0 findings.
  * `make gate` ended `overall=0`. Each of its steps exited 0: gofmt, lint (0 issues), vet, race, shuffle, tidy, apicheck (`compatible with v1.25.0`), fuzz, vuln (`No vulnerabilities found.`), scripts, shellcheck, crossvet, links, arch and ids.
  * `make apicheck` reports incompatible changes only. So the additions were listed with the scratchpad's `apiadds.py`, which runs the same apidiff pin without `-incompatible` against a `git archive` of `v1.25.0`. It printed "Compatible changes:", then `SymlinkInvocationError`, `SymlinkSource`, `SymlinkSourceExecutable`, `SymlinkSourceInvocation` and `SymlinkSourcePolicy`, each `added`, and nothing else. apidiff lists a type's methods with the type.

### Phase A2

* **Step 1, the three hosts.** `go test -count=1 -v -run 'TestResolveTarget|TestSymlinkInvocationError|TestMaintenanceThroughSymlink|TestKeptBackups|TestFailureClass|TestManagedKeptBackups' ./selfupdate/` passed on all three, and no subtest skipped:
  * the macOS development host;
  * a Linux test host (`go1.27.3 linux/amd64`);
  * the Windows test host (`go1.27.3 windows/amd64`, Git Bash). Its `os.Symlink` was permitted.

  Every case of `TestResolveTargetInvokedSymlink`, `TestMaintenanceThroughSymlink` (`executable`, `invocation`, `policy`, `dangling`) and `TestResolveTargetRealExecutableThroughSymlink` (`resolve`, `cleanup`, `kept`) passed, with the rest of the set.
* **Step 2, four planted copies on the Linux test host.** The scratchpad's `a2plants.py` made each copy with `scripts/plant-copy.sh`, ran step 1's tests in it, and removed it. The remote command ended in `| head`, so its exit was 0. The failures were read from the output, which ends `FAIL` for every copy. Paths are shown as `<tmp>`.
  * **P1.** In `errors.go`, `"; start " + e.Target + " itself to update it"` became `"; run " + e.Target + ", or set TargetPolicy.ExecutablePath to it"`. Failed:
    * `TestResolveTargetExecutableSymlink`;
    * `TestSymlinkInvocationErrorText`, with `"…; run /b/real, or set TargetPolicy.ExecutablePath to it" names a Go identifier` for the executable and invocation sources;
    * `TestResolveTargetInvokedSymlink`'s `absolute_link`, `relative_link` and `bare_name`.
  * **P2.** In `standalone.go`, `if _, ok := linkTarget(err); ok {` became `linkTarget(nil)`. Failed:
    * `TestMaintenanceThroughSymlink`'s `executable`, `invocation` and `dangling`, with `CleanupPending = selfupdate: refusing to update through a symlink: <tmp>/002/demo links to <tmp>/001/demo; start <tmp>/001/demo itself to update it, want nil`;
    * `TestResolveTargetRealExecutableThroughSymlink/cleanup`, with `absolute link: helper printed "symlink invocation", want "ok"`.
  * **P3.** `resolved, ok := linkTarget(err)` became `linkTarget(nil)`. Failed:
    * `TestMaintenanceThroughSymlink`'s `executable` and `invocation`, with `KeptBackups = [], selfupdate: refusing …; want the one kept backup`;
    * `…/kept`, with `helper printed "symlink invocation", want "ok 1"`.
  * **P4.** `link.Source != SymlinkSourcePolicy` became `link.Source != ""`. Failed `TestMaintenanceThroughSymlink/policy`, with `CleanupPending = <nil>, want the policy's refusal`.
* **Step 3, probe A1-P1 on the changed tree.** The scratchpad's `a1probe/run2.py` gave each case a fresh directory. The macOS development host and a Linux test host printed the same:

  | Case | `ResolveTarget` | `KeptBackups` | `CleanupPending` | `real/` after |
  | :--- | :--- | :--- | :--- | :--- |
  | 1, real path | ok | ok, 1 | ok | `.msgprobe.selfupdate.lock` added |
  | 2, link | `…: <tmp>/link/msgprobe links to <tmp>/real/msgprobe; start <tmp>/real/msgprobe itself to update it` | ok, 1 | ok | unchanged |
  | 3, `ExecutablePath` = link | `…: the configured executable path <tmp>/link/msgprobe links to <tmp>/real/msgprobe` | the same refusal | the same refusal | unchanged |
  | 4, link, `ExecutablePath` = real | ok | ok, 1 | ok | `.msgprobe.selfupdate.lock` added |

  Before the change, case 2 refused all three calls with the `v1.25.0` text (MADR FA1.4). Now it gives D4-A's text and D5-A's behaviour, and writes nothing in `real/`.

### Phase A3a, done as 0042's Phase B3a

* 0042-PLAN-keep-go-identifiers-out-of-run-time-messages.md Phase B3a wrote one `## v1.26.0 - 2026-11-09` entry in `CHANGELOG.md`, and the migration guide's `## 26. From v1.25 to v1.26`, for both PLANs. Its record holds the checks.
* Each step of Phase A3a is covered:
  * Added: `SymlinkInvocationError`, `SymlinkSource` and its three constants.
  * Changed: the refusal's text by source, with each old and new text in §26.
  * Fixed: issue #10's points, startup maintenance through a link included, noting that it was new on Linux in `v1.25.0`.
  * The entry links §26, the MADR's Amendment A1 and this PLAN.
  * `scripts/check-release-tag.sh v1.26.0` exit 0, `git tag -l v1.26.0` empty, and the Markdown checks pass.

### Owner decisions before execution

* **2026-11-09.** Three options were put to the owner for each decision, with the recommended one first.
  * They chose D3-A (a string-backed typed error), D4-A (a text for each source, and only the fact for the policy case) and D5-A (`CleanupPending` does nothing, and `KeptBackups` follows the link). The options they were not given are D3-C, D4-D and D5-C.
  * For the release, they chose one `v1.26.0` with 0042's change, over `v1.26.0` and then `v1.27.0`, and over `v1.26.0` with 0042 left unreleased. Phases A3a and A3b move to 0042's PLAN, as annotated above. No step of Phases A0 to A2 changes.

### Deviations

* **Deviation 1 (2026-11-09), Phase A1 step 3, `linkStart`'s signature.**
  * **What failed.** The first run of `make pre-add-check` on the changed Go files failed, on all three GOOS runs, with one finding: `selfupdate/standalone.go:97:6: Error return value is not checked (errcheck)` on `if _, ok := linkStart(err); ok {`. The signature this step gave, `(*SymlinkInvocationError, bool)`, returns an error type, and `CleanupPending` discarded it with `_`. The finding was new code's, not a pre-existing one.
  * **The options put to the owner.**
    * Return only the pointer, which was recommended.
    * Keep the signature and add a redundant nil check in `CleanupPending`.
    * Use a bool helper for `CleanupPending`, and a second `errors.As` in `KeptBackups`.

    A `//nolint` was not offered, since it is a workaround.
  * **The owner's first choice.** On 2026-11-09 they chose to return only the pointer, as `linkStart(err error) *SymlinkInvocationError`.
  * **That choice failed too.** The next `make pre-add-check` failed with `selfupdate/standalone.go:98:4: error is not nil (line 97) but it returns nil (nilerr)`, on all three GOOS runs. `*SymlinkInvocationError` implements `error`, so `if linkStart(err) != nil { return nil }` reads as returning nil for a non-nil error. Checking `linkStart(err)` before `err != nil` failed the same way (`:99:3`, "line 96").
  * **What was tried.** On a scratch copy of the tree, `linkTarget(err error) (string, bool)` returns the refusal's `Target` and whether it is a link start. It gave `golangci-lint` 0 issues for linux, darwin and windows, and `TestMaintenanceThroughSymlink`, `TestResolveTargetRealExecutableThroughSymlink` and `TestKeptBackups` passed.
  * **The options put to the owner, a second time.**
    * That form, which was recommended.
    * A bool helper, with a second `errors.As` in `KeptBackups`.
    * Inline `errors.As` in both methods.

    A `//nolint` was not offered.
  * **The owner's final choice.** On 2026-11-09 they chose `linkTarget(err error) (string, bool)`.
    * `CleanupPending` returns nil when it reports true. The check comes before `if err != nil`.
    * `KeptBackups` resolves again with the returned target when it reports true and the target is not empty.
    * Behaviour and tests are unchanged.
  * **What does not change.** The MADR: the helper is unexported, and D5-A's behaviour is the same.
