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
