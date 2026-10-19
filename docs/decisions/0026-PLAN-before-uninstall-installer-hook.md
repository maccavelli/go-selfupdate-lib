---
status: in-progress
date: 2026-10-19
associated-madr: "0026-MADR-before-uninstall-installer-hook.md"
---
# Implement the `before_uninstall` installer hook as `v1.17.0`

Associated MADR: [0026-MADR-before-uninstall-installer-hook.md](0026-MADR-before-uninstall-installer-hook.md)
(D1-A, D2-A, D3-A, D4-B, D5-A, D6-A, as the owner chose).

## Goal

* A release spec can name a `before_uninstall` hook. `install.sh
  --uninstall` and `install.ps1 -Uninstall` run it, with D2-A's rules,
  before they remove anything.
* A module that requires a library older than `v1.17.0` cannot build a
  spec that uses it (D5-A).
* Every new behaviour is tested on its shell, seen failing first, and
  released as `v1.17.0`. Issue #1 is closed on the owner's ask.

## Scope

### In scope

| Phase | What | Commit |
| :--- | :--- | :--- |
| U0 | this pair, its index rows | records only |
| U1 | `releasespec`: the constant, the validation, the floor | code and tests |
| U2 | `install.sh`: the uninstall hooks | template and tests |
| U3 | `install.ps1`: the uninstall hooks | template and tests |
| U4 | the guides, 0014's pointer, the changelog, the migration guide's §17 | docs |
| U5 | `v1.17.0` | per 0017's release procedure |
| U6 | closing | records only |

### Out of scope

* An `after_uninstall` time (MADR, More Information), and listing the
  install hooks under an install's `--dry-run`.
* Any change to the install path of either template.

## Rules

* **`main`.** Under `sysadmin-git-branch-gates`, the agent commits only
  on "commit to main" in that turn, with `git commit --no-edit`. A push or
  a tag needs its own word.
* **Each phase stops at a deviation**, recorded in this plan, and in the
  MADR when it changes a decision or a fact.
* **Checks before each commit:**
  * `make pre-add-check` on the changed Go files;
  * `scripts/check-installers.sh` on a changed template;
  * `scripts/check-docs.sh --links`, `--ids` and markdownlint on changed
    Markdown;
  * `--arch` when a `.go` file is added;
  * `make apicheck`, compatible with `v1.16.0`.

  `make gate` must end `overall=0` before the push.
* **Seen failing first:** every new test fails on a `scripts/plant-copy.sh`
  copy with one planted break, never on a dirtied tree, and the record
  quotes it.
* **Windows:** `install.ps1`'s tests run on the Windows test host, under
  both PowerShells (`SELFUPDATE_INSTALL_REQUIRE_SHELLS=powershell,pwsh`),
  from a bundle with no AppleDouble files.

## Implementation Steps

### Phase U0: records

1. `docs/README.md` gains this pair's rows after 0025's.
2. On approval, the MADR records the owner's decisions, this plan becomes
   `in-progress` with `### Approval`, and 0014-MADR's `## Amendments`
   gains a dated pointer to 0026.
3. Checks; commit on the owner's ask.

### Phase U1: `releasespec` (D1-A, D5-A)

1. `selfupdate/releasespec/installer.go`:
   * `HookBeforeUninstall = "before_uninstall"`, documented as "runs the
     installed copy, when there is one, before --uninstall removes it";
   * `validateInstaller` (`:84`) accepts it, and its error names all three
     times;
   * `Hook.When`'s doc names the three constants.
2. `installer_test.go`: a spec with a `before_uninstall` hook parses, and
   `InstallerScripts` is unchanged by it. The existing `after_uninstall`
   refusal (`:53`) stays, its message updated to the three-time wording.
3. **The floor** (`internal/cmd/selfupdate-release/floor.go:27-30`): a
   `specFloors` entry, `{"installer.hooks[].when: before_uninstall",
   "v1.17.0", "releasespec.Parse in v1.16.0 refuses it", <any hook with
   When == HookBeforeUninstall>}`. A test beside the existing floor tests
   checks:
   * a spec with the hook in a module requiring `v1.16.0` is refused,
     naming `go get …@v1.17.0`;
   * the same module requiring `v1.17.0` passes.
4. **Seen failing:** a plant of `validateInstaller` without the new time
   fails the parse test, and a plant without the floor entry fails the
   floor test.
5. `make apicheck` is compatible with `v1.16.0`. Checks; commit.

### Phase U2: `install.sh` (D2-A, D3-A, D4-A, D6-A)

1. **`run_hooks`** (`install.sh:202-230`):
   * the first failure stops it for any `before_*` time (`case $1 in
     before_*) break ;; esac` in place of `:225`);
   * under `DRY_RUN=1` a hook that would run prints `would run <product>
     <args>` and is not run. Only the uninstall path reaches this, because
     the install path's dry run returns before its hooks (`:410-416`).
2. **`uninstall()`** (`:289-300`), before its loop, when hooks are not
   skipped and `$HOOKS` is not empty:
   * it sets the `cleanup` traps;
   * it makes `WORK=$(mktemp -d "${TMPDIR:-/tmp}/.$INSTALLER_NAME-uninstall.XXXXXX")`;
   * without `--force`, it runs `run_hooks before_uninstall "$DIR" || die
     1 "a before_uninstall hook failed; nothing was removed"`;
   * with `--force` (`FORCE=1`), `run_hooks` does not stop at a failure
     (`before_uninstall` breaks only when `FORCE` is 0). A failure sets a
     flag, the binaries are removed, and `uninstall` then exits 3 with
     "removed, but a before_uninstall hook failed".
   * `main` parses `--force`, and refuses it without `--uninstall`: "--force
     applies only to --uninstall", exit 1.
3. **Text:** the usage line `--no-hooks` reads "skip the release's hooks";
   the header's exit codes add that a failed `before_uninstall` hook removes
   nothing and exits 1.
4. **The fixture** (`installer_harness_test.go:88`) gains `{"when":
   "before_uninstall", "product": "relay", "args": ["hook-uninstall"]}`.
   The fixture program records its hook arguments as it records
   `hook-before`.
5. **Tests** (`installer_sh_test.go`, beside "--uninstall" at `:398`), in
   each shell the test runs:
   * the hook runs the installed `relay` before `relay` and `relay.prev`
     are removed;
   * a failing hook exits 1 with "before_uninstall hook failed: relay
     hook-uninstall" and "a before_uninstall hook failed; nothing was
     removed", and leaves both binaries and `.prev` files;
   * with `relay` not installed, no hook runs and `relayctl` is removed;
   * `--no-hooks` and `<PREFIX>_NO_HOOKS=1` remove with no hook run;
   * `--dry-run` prints "would run relay hook-uninstall" and "would
     remove", runs nothing and removes nothing;
   * `--product relayctl` runs no `relay` hook;
   * `--force` with a failing hook removes both binaries, exits 3 with the
     message, and still runs a second `before_uninstall` hook;
   * `--force` without `--uninstall` exits 1.
6. **Seen failing:** a plant of `uninstall()` without the hook call fails
   the "runs before removal" case; a plant that keeps going after a
   failed `before_uninstall` without `--force` fails the "leaves the
   binaries" case; a plant where `--force` still stops fails the
   `--force` case.
7. `scripts/check-installers.sh internal/cmd/selfupdate-release/installer/install.sh`
   passes (shellcheck `-s sh`, `dash -n`). The installer container job's
   shells (BusyBox ash, dash) run the tests in CI. Checks; commit.

### Phase U3: `install.ps1` (D2-A, D3-A, D4-A)

1. **`Invoke-InstallHook`** (`install.ps1:410-427`) gains a `[bool]$DryRun
   = $false` parameter. Under it, a hook that would run prints `would run
   <product> <args>` and is not run. The first failure stops it for any
   `before_*` time (`$When -clike 'before_*'` in place of `:424`).
2. **The `-Uninstall` branch** (`:487-499`) first runs the
   `before_uninstall` hooks:
   * without `-Force`, `Invoke-InstallHook` stops at the first failure, and
     the branch `Invoke-Fail`s 1 with 'a before_uninstall hook failed;
     nothing was removed';
   * with `-Force` (a new `[switch]$Force`), every hook runs and the
     binaries are removed; a failed hook then gives exit 3, 'removed, but
     a before_uninstall hook failed';
   * `-Force` without `-Uninstall` is refused, exit 1.
3. **Text:** `-NoHooks`'s help and the header's exit codes, as U2 step 3.
4. **Tests** (`installer_ps_test.go`, beside "-Uninstall" at `:844`): U2
   step 5's eight cases, in `argModes` and both PowerShells.
5. **Seen failing:** the same three plants as U2 step 6, on the Windows
   test host.
6. `scripts/check-installers.sh …/install.ps1` passes (the parser and
   PSScriptAnalyzer). The Windows test host runs `go test -run
   '^TestInstallPs1' ./internal/cmd/selfupdate-release/` under both
   PowerShells. Checks; commit.

### Phase U4: the documents

1. **`building-releases.md`:**
   * `### Hooks` gains `before_uninstall`;
   * `### Options`: `--no-hooks` "skip the hooks"; `--uninstall` "after
     the `before_uninstall` hooks"; and a new row, `--force` / `-Force`,
     "with --uninstall only: remove even when a before_uninstall hook
     fails";
   * `### Exit codes`: row 1 gains "a failed `before_uninstall` hook
     (nothing removed)", and row 3 gains "removed under `--force`, but a
     `before_uninstall` hook failed";
   * §1's spec example gains a `before_uninstall` hook.
2. **0014-MADR's pointer** is added on approval (U0 step 2).
   `docs/architecture.md`'s installer paragraph names the third time.
3. **`CHANGELOG.md`:** `## Unreleased` gains Added (`HookBeforeUninstall`
   and the uninstall hooks) and Changed (the floor).
4. **The migration guide:** `## 17. From v1.16 to v1.17`. To adopt it,
   require `v1.17.0`, then add the hook; an uninstall that fails a hook
   exits 1.
5. Snippets: the JSON example parses with `releasespec.Parse` in a scratch
   module. Checks; commit.

### Phase U5: `v1.17.0`

[0017-PLAN-verify-build-provenance-and-close-0015-open-items.md](0017-PLAN-verify-build-provenance-and-close-0015-open-items.md)'s
release procedure, as 0025-PLAN S14 ran it:

1. `make gate` ends `overall=0`, then the push on the owner's word, and
   CI green on `main`.
2. The disclosure guard and `check-release-tag.sh v1.17.0`, then the tag
   on the owner's word.
3. The tag's CI, with its identity legs.
4. The pin commit: every current-release pointer moves to `v1.17.0`.
5. The proxy and `@latest`.
6. Release notes.
7. Issue #1 is closed with a comment naming `v1.17.0`, on the owner's
   ask.

### Phase U6: closing

`### Verification, at closing`, `status: complete`, and the index row.
The MADR stays `accepted`.

## Verification

* **V1 (spec):** `before_uninstall` parses, `after_uninstall` is
  refused, and `apicheck` is compatible with `v1.16.0`.
* **V2 (floor):** a `v1.16.0` module with the hook is refused, naming
  the fix.
* **V3 (installers):** the eight cases pass in each shell of both
  templates, and each new test failed on its plant.
* **V4 (lint):** `check-installers.sh` passes on both templates.
* **V5 (release):** `v1.17.0` is tagged, and its CI and the proxy serve
  it.
* **V6 (issue):** issue #1 is closed with the release named.

## Rollout and Rollback

* **Rollout:** one commit per phase on `main`, pushed on the owner's word;
  `v1.17.0` at U5. A consumer opts in by requiring `v1.17.0` and adding the
  hook. The installers carry it only when the consumer's workflow pins
  `v1.17.0`.
* **Rollback:** before the tag, `git revert` of a phase; after it, fix
  forward. A consumer drops the hook from its spec to go back.

## Execution record

### Approval (2026-10-19)

The owner answered the MADR's questions: "Stop, plus a --force flag"
(D2-A with D4-B), "Only the selected products'" (D3-A), and "Approve
and proceed". `--force`'s behaviour, which D4-B left open (the exit
code, its scope, no environment variable), is set in the MADR's
Decision Outcome, and U2, U3 and U4 carry it. The MADR became
`accepted`, and this plan `in-progress`.

### U0 (2026-10-19)

1. `docs/README.md` holds the pair's rows: the MADR, with its title naming
   `--force`, `accepted`; this plan `in-progress`.
2. The MADR records the owner's choices and `--force`'s behaviour.
3. 0014-MADR gains `## Amendments` with `A1 (2026-10-19)`, pointing
   here.
4. The owner committed U0 as `27519b7` ("committed, proceed").

### U1 (2026-10-19)

1. **`releasespec`:**
   * `HookBeforeUninstall = "before_uninstall"`;
   * `validateInstaller` accepts it, and its refusal reads `… is not
     "before_install", "after_install" or "before_uninstall"`;
   * `Hook.When`'s doc names the three.
   * The `after_uninstall` refusal (`installer_test.go:53`) still holds: it
     matches `installer.hooks[0].when`, which the new wording keeps.
2. **The floor:**
   * `specFloors` gains `{"installer.hooks[].when before_uninstall",
     "v1.17.0", …}`, set when any hook's `When` is `HookBeforeUninstall`.
   * **A refinement step 3 did not name:** `checkLibraryFloor` returned the
     first floor a module failed. A `v1.9.0` module with the new hook would
     have been told `go get …@v1.10.0`, which is not enough. It now reports
     the highest floor the module misses. No decision changes; D5-A's "naming
     the `go get` to run" needs it.
3. **Tests:**
   * `TestInstallerBeforeUninstall`: the hook parses, and `InstallerScripts`
     equals an empty installer's.
   * `TestPlanRefusesAnOldLibrary` gains four cases:
     * `v1.16.0` with the hook is refused, naming the floor;
     * `v1.17.0` with it passes;
     * `v1.9.0` with it names `go get …@v1.17.0`;
     * `v1.16.0` with only the install hooks passes.
   * `TestSpecFloorsCoverEveryField` still passes: the new floor is a value,
     not a new field.
4. **Seen failing:**

   | Plant | Failure |
   | :--- | :--- |
   | `validateInstaller` without the new time | `installer_test.go:191: Parse: releasespec: installer.hooks[0].when: "before_uninstall" is not "before_install", "after_install" or "before_uninstall"` |
   | the floor at `v0.0.0` | `plan_test.go:269: err = <nil>; want "this spec's \"installer.hooks[].when before_uninstall\" needs v1.17.0 or later"` |
   | the first miss wins again | `plan_test.go:269: err = … this spec's "installer" needs v1.10.0 or later …` (for the `v1.9.0` case) |

5. **Checks:**
   * `go test ./selfupdate/releasespec/ ./internal/cmd/selfupdate-release/`
     `ok`, with `SELFUPDATE_REQUIRE_PYTHON=1`;
   * `pre-add-check` 4 files clean;
   * `make apicheck` compatible with `v1.16.0`;
   * links, markdownlint, ids and arch clean.

### U2 (2026-10-19)

1. **`install.sh`:**
   * `run_hooks` (`:205-243`): under `--dry-run`, a hook that would run prints
     `would run <product> <args>` and is not run. A failure stops a
     `before_*` time, except `before_uninstall` under `--force` (`:235-238`).
   * `uninstall()` (`:302-331`): when hooks are not skipped and the release
     has any, it sets the `cleanup` traps and makes its work directory under
     `${TMPDIR:-/tmp}` (`:312`). It then runs the `before_uninstall` hooks.
     Without `--force`, a failure ends the uninstall: exit 1, "a
     before_uninstall hook failed; nothing was removed". With `--force`, the
     binaries are removed and the exit is 3, "removed, but a
     before_uninstall hook failed".
   * `main`: `FORCE=0`, and `--force` parses. Without `--uninstall` it is
     refused: exit 1, "--force applies only to --uninstall" (`:409-411`).
   * **Text:**
     * the usage line `--no-hooks` reads "skip the release's hooks";
     * a `--force` line is added;
     * the header's exit codes name both `before_*` failures under 1, and
       `--force`'s removal under 3. The old header left out the
       `before_install` failure's exit 1.
2. **The fixture:** `installSpecJSON` gains `{"when": "before_uninstall",
   "product": "relay", "args": ["hook-uninstall"]}`.
   * **A correction to step 4:** the fixture program does not record
     `hook-before`. Its `main` prints `relay` and exits 0 for any argument it
     does not know, and the stand-ins (`standIn`) do the recording. So
     `hook-uninstall` needs no change to the program, just as `hook-before`
     needed none. The existing "--uninstall" case runs that hook through the
     real `relay` and still passes.
3. **Tests:** `installer_sh_test.go` (`:409-494`) gains step 5's eight cases.
   They run under `sh`, `bash` and `dash` on this host, and all 24 pass:
   * "before_uninstall runs the installed copy before removal": the
     stand-in logs only when it finds itself on disk, so its log line shows
     that it ran before `relay` was removed. `relay`, `relay.prev` and
     `relayctl` are all removed.
   * "a failed before_uninstall removes nothing";
   * "before_uninstall skips a product not installed";
   * "--no-hooks and RELAY_NO_HOOKS skip before_uninstall";
   * "--uninstall --dry-run shows the hook and runs nothing";
   * "--uninstall --product runs only that product's hooks";
   * "--force removes past a failed before_uninstall and exits 3": a second
     hook, on `relayctl`, is added with `renderWith`; both hooks run.
   * "--force needs --uninstall".
4. **Seen failing:**
   * Step 6 named three plants. Nine were run, one or more for each case,
     each on a `scripts/plant-copy.sh` copy of `install.sh` under `dash`.
   * The session's `plant/u2.py` made the copies and ran them; the tree
     was not touched.
   * The table quotes each plant's first failure, with the temporary paths
     elided.

   | Plant | Failure |
   | :--- | :--- |
   | `uninstall()` without the hook call | `installer_sh_test.go:416: the output lacks "running relay hook-uninstall"`; the failure, `--dry-run` and `--force` cases fail too |
   | `[ "$FORCE" = 1 ] \|\| die 1 …` replaced by `:` | `installer_sh_test.go:428: exit 3, want 1` |
   | `run_hooks` without the installed-copy check | `installer_sh_test.go:434: exit 1, want 0` (`…/relay: not found`), and twelve install cases |
   | `--no-hooks` not setting `NO_HOOKS` | `installer_sh_test.go:443: exit 1, want 0` |
   | `<PREFIX>_NO_HOOKS` not read | `installer_sh_test.go:447: exit 1, want 0` |
   | the dry-run branch removed | `installer_sh_test.go:457: the output lacks "would run relay hook-uninstall"` |
   | `run_hooks` without `selected` | `installer_sh_test.go:469: a hook of an unselected product ran` |
   | `before_uninstall) break ;;` (`--force` still stops) | `installer_sh_test.go:488: hook log "old hook-uninstall\n", want both before_uninstall hooks` |
   | the `--force` refusal replaced by `:` | `installer_sh_test.go:492: exit 0, want 1` (it installed) |

5. **Checks:**
   * `go test ./internal/cmd/selfupdate-release/` is `ok`, with
     `SELFUPDATE_REQUIRE_PYTHON=1`.
   * `scripts/check-installers.sh …/install.sh` is clean.
   * `installer_ps_test.go` is `//go:build windows`, so its cases do not
     build on this host. `Invoke-InstallHook` (`install.ps1:410`) skips a
     hook whose time is not its own, so the new fixture hook is inert there
     until U3. U3 runs those cases on the Windows test host.

### U3 (2026-10-19)

1. **`install.ps1`:**
   * `Invoke-InstallHook` (`:410-438`) gains `[bool]$DryRun = $false` and
     `[bool]$Force = $false`. Under `-DryRun`, a hook that would run prints
     `would run <product> <args>` and is not run. A failure stops a
     `before_*` time, except `before_uninstall` under `-Force` (`:434`).
     * Step 1 named only `$DryRun`. `$Force` is how step 2's "every hook
       runs" reaches the loop, as `FORCE` does in `install.sh`'s
       `run_hooks`.
   * The `-Uninstall` branch (`:498-518`) runs the `before_uninstall` hooks
     first. Without `-Force`, a failure ends it: `Invoke-Fail` 1, 'a
     before_uninstall hook failed; nothing was removed'. With `-Force`, the
     binaries are removed, the PATH entry is cleared as before, and the
     return is 3, 'removed, but a before_uninstall hook failed'.
   * `[switch]$Force` (`:44`) reaches `Install-Release` as `[bool]$Force`.
     Without `-Uninstall` it is refused: `Invoke-Fail` 1, '-Force applies
     only to -Uninstall' (`:472`).
   * **Text:** the header's exit codes, as in `install.sh`. **A correction
     to step 3:** `install.ps1` has no help text, so there is no `-NoHooks`
     help to change. The header is the only text.
2. **Tests:**
   * **A helper move:** `renderWith` moves from `installer_sh_test.go`
     (`//go:build unix`) to `installer_harness_test.go`, which has no build
     tag. The `-Force` case needs it on Windows. Its body is unchanged.
   * `installer_ps_test.go` (`:859-944`) gains step 4's eight cases in
     `argModes`. The stand-ins are `old` for `relay.exe`, and `new` for
     `relayctl.exe` and for the `-Force` case's second hook (`hook-two`).
3. **Fixed during the phase: the TLS order.**
   * **First run:** 31 of the 32 new runs passed (eight cases, two forms,
     two PowerShells). `powershell/scriptblock/-Force needs -Uninstall`
     failed with `installer_ps_test.go:941: Windows PowerShell ran the script
     without adding TLS 1.2 (0 -> 0)`.
   * **Cause:** the `-Force` check stood before the line that adds TLS 1.2
     on Windows PowerShell 5.1. The header promises that addition, and
     `session` requires it for every refusal except a parameter-binding
     error. The other option refusals, such as an unknown `-Product`, come
     after the TLS line.
   * **Fix:** the check moves below the TLS line (`:472`). No decision or
     test changes.
4. **Seen failing:**
   * Nine plants, the same as U2's, on `scripts/plant-copy.sh` copies of
     `install.ps1`.
   * They ran on the Windows test host under `pwsh` in the file form, from
     a bundle with no AppleDouble files.
   * The first eight ran on copies made before the fix. The fix does not
     touch the lines those eight plant or the cases they fail. The `-Force`
     refusal plant was run again after the fix, under both PowerShells and
     both forms.
   * The table quotes each plant's first failure, with the temporary paths
     elided.

   | Plant | Failure |
   | :--- | :--- |
   | the `-Uninstall` branch without the hook call | `installer_ps_test.go:933: exit 0, want 3`; the failure, `-DryRun` and "before removal" cases fail too |
   | the no-`-Force` `Invoke-Fail` removed | `installer_ps_test.go:876: exit 3, want 1` |
   | `Invoke-InstallHook` without the installed-copy check | `installer_ps_test.go:639: exit 1, want 0` (`before_install hook failed`), and sixteen other cases |
   | `$skipHooks` without `-NoHooks` | `installer_ps_test.go:891: exit 1, want 0` |
   | `$skipHooks` without `<PREFIX>_NO_HOOKS` | `installer_ps_test.go:895: exit 1, want 0` |
   | the dry-run branch removed | `installer_ps_test.go:905: the output lacks "would run relay hook-uninstall"` |
   | `Invoke-InstallHook` without `$Selected` | `installer_ps_test.go:918: a hook of an unselected product ran` |
   | `if ($When -clike 'before_*') { break }` (`-Force` still stops) | `installer_ps_test.go:937: hook log "old hook-uninstall\n", want both before_uninstall hooks` |
   | the `-Force` refusal removed | `installer_ps_test.go:941: exit 0, want 1` (it installed), in each PowerShell and form |

5. **Checks:**
   * On the Windows test host, with
     `SELFUPDATE_INSTALL_REQUIRE_SHELLS=powershell,pwsh`:
     * after the fix, `go test -run '^TestInstallPs1'` was `ok`, and all 32 new runs passed;
     * `scripts/check-installers.sh …/install.ps1` (the parser and
       PSScriptAnalyzer) was clean.
   * On this host:
     * PSScriptAnalyzer is not installed, so `check-installers.sh` on
       `install.ps1` refuses to run here;
     * `make pre-add-check` on the three test files was clean, its lint
       covering `GOOS=windows`;
     * `go test ./internal/cmd/selfupdate-release/` was `ok`, with the
       sh cases still using the moved `renderWith`.
   * **A watched risk that did not occur:** a hook's image can stay mapped
     for a moment after it exits, and the uninstall's `Remove-Item` is
     `-ErrorAction SilentlyContinue`. In every run of "before_uninstall
     runs the installed copy before removal", `relay.exe` was gone right
     after its hook. That held in each PowerShell and form, so no change was
     made for it.
