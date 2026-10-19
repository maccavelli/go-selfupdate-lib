---
status: accepted
date: 2026-10-19
decision-makers: go-selfupdate-lib maintainers
consulted: "GitHub issue #1, \"releasespec: add a before_uninstall installer hook\" (2026-10-19), from the owner for magic-cli-remote's 0054-MADR-standardize-self-update-on-go-selfupdate-lib.md D11; a read of the spec, the two installer templates, their renderer, the library-floor check and their tests at 902808d (v1.16.0 plus 0025's closing)"
informed: the fleet's programs whose release spec turns on the installers, magic-cli-remote first
---
# Add a `before_uninstall` installer hook that runs the installed copy before `--uninstall` removes it, with the install hooks' rules, a `--force` to remove past a failed hook, and a library floor so an older program cannot embed it

## Context and Problem Statement

GitHub issue #1 asks for a third installer hook time, `before_uninstall`.
With it, `install.sh --uninstall` and `install.ps1 -Uninstall` can run a
product command before they delete the binaries. A program that sets up a
background service at install time can use it to take that service down
before its binary goes. Today `after_install` gives a product the seam to
set a service up, and nothing gives it one to take the service down. An
operator who runs only the installer's uninstall leaves a systemd unit, a
LaunchAgent or a scheduled task pointing at a binary that is gone.

The consumer is magic-cli-remote. That repository's
`0054-MADR-standardize-self-update-on-go-selfupdate-lib.md` D11 would add:

```json
{"when": "before_uninstall", "product": "mcremote", "args": ["setup-service", "--uninstall"]}
```

Until then it documents a two-command uninstall.

This record decides whether to add the hook, how it behaves, the issue's
two open questions, and two things the issue does not raise: the library
floor, and where the Unix installer reads its hook list during an
uninstall.

**Facts [observed 2026-10-19, `main` at `902808d`]:**

* **F1. The spec has two hook times.**
  * `selfupdate/releasespec/installer.go:20-28` defines `HookBeforeInstall`
    and `HookAfterInstall`, and `validateInstaller` refuses any other
    `when` (`:84`).
  * `installer_test.go:53` pins the refusal of `after_uninstall`.
  * A hook names a product the spec lists, with 1 to 16 arguments matching
    `installerArgRe` (`:87-97`), and the installer holds at most 8 hooks
    (`maxHooks`).
* **F2. `install.sh` runs no hook on uninstall.**
  * `uninstall()` (`install.sh:289-300`) removes `<dir>/<product>` and
    `<dir>/<product>.prev` for each selected product, or prints "would
    remove" under `--dry-run`.
  * `main` calls it (`:395-398`) after the option, `--product` and root
    checks, and before anything else, `$WORK` included.
  * `run_hooks WHEN DIR` (`:202-230`) reads the hook list from
    `$WORK/hooks`. It runs a hook only when its product is selected and
    `<dir>/<product>` is an executable regular file, with `</dev/null`. It
    skips everything under `NO_HOOKS=1`, and stops at the first failure only
    for `before_install` (`:225`).
  * `$WORK` is made, with `mktemp -d` in `$DIR`, only on the install path
    (`:435`).
* **F3. `install.ps1` runs no hook on uninstall.**
  * The `-Uninstall` branch (`install.ps1:487-499`) removes `<dir>\<product>.exe`
    and its `.prev`, clears the folder's PATH entry with the last program,
    and returns 0.
  * `Invoke-InstallHook WHEN DIR SELECTED SKIP` (`:410-427`) has the same
    rules as `run_hooks`. It reads `$Hooks`, a value in the generated
    script, so it needs no work file, and it stops at the first failure only
    for `before_install` (`:424`).
* **F4. The renderer passes any hook time through.** `render.go:120-122`
  writes each hook as `<when> <product> <args>` into `install.sh`'s
  `HOOKS`, and `:166-170` writes it into `install.ps1`'s `$Hooks`. A new
  time needs no renderer change.
* **F5. `--no-hooks` already removes without running a hook.** It, and
  `<PREFIX>_NO_HOOKS=1`, skip every hook (`install.sh:205`; `install.ps1:476`,
  `:411`). With a `before_uninstall` hook, `--uninstall --no-hooks` is the
  operator's way to remove the binaries regardless.
* **F6. A program parses the spec it embeds.**
  * Its `releaseConfig` calls `releasespec.Parse` on the embedded bytes
    (`docs/guides/building-releases.md`, step 2). A program built against
    `v1.16.0` would refuse a spec with `before_uninstall` at start-up, and
    so could not update itself.
  * The build tool already guards the same case for the `installer` field:
    `specFloors` (`internal/cmd/selfupdate-release/floor.go:27-30`) refuses
    a module that requires a release older than the first one that reads a
    field the spec uses, naming the `go get` that fixes it.
* **F7. The tests to extend.**
  * `installer_sh_test.go` has the cases "before_install runs the installed
    copy, and only that" (`:361`), "a failed before_install changes
    nothing" (`:372`) and "--uninstall" (`:398`).
  * `installer_ps_test.go` has the same three (`:752`, `:775`, `:844`), and
    runs on Windows under both PowerShells in CI.
  * The fixture spec's hooks are in `installer_harness_test.go:88`.
* **F8. The docs to extend.** `building-releases.md`'s `### Options`,
  `### Hooks` and `### Exit codes` (`:474-520`). `install.sh`'s usage line
  "skip the release's install hooks" (`:63`), and `install.ps1`'s help.
  0014-MADR's hooks description (`:265-274`).

## Decision Drivers

* No exported API breaks: a new constant and a new accepted value are
  additions within v1, and `make apicheck` stays compatible with
  `v1.16.0`.
* Least surprise: the new hook behaves as `before_install` does in every
  rule that can carry over (selection, the installed copy, no standard
  input, `--no-hooks`, stopping at the first failure, exit 1 with nothing
  changed).
* An uninstall that fails part-way must not leave a service torn down
  with its binary still present, or the reverse, without saying so.
* A spec must not break the program that embeds it (F6).
* Both installers behave alike, and each new behaviour is tested on its
  shell, seen failing first.

## Considered Options

### D1: add the hook

* **D1-A.** Add `HookBeforeUninstall = "before_uninstall"` to
  `releasespec`, accepted by `validateInstaller` under the existing
  product, count and argument rules. Both installers run it on uninstall,
  as D2 says.
* **D1-B.** No new hook. Document the two-command uninstall as the pattern.
* **D1-C.** A general `uninstall` command on each product, which the
  installers would always call, without the spec naming it.

### D2: how it runs

* **D2-A.** As issue #1 proposes, with `before_install`'s rules:
  * on `--uninstall` / `-Uninstall`, each `before_uninstall` hook whose
    product is selected runs the installed copy, `<dir>/<product>`, with
    its arguments and no standard input, before anything is removed;
  * a product that is not installed is skipped;
  * hooks run in the spec's order, and the first failure stops the
    uninstall with nothing removed, exiting 1 with "a before_uninstall
    hook failed; nothing was removed";
  * `--no-hooks` / `-NoHooks` and `<PREFIX>_NO_HOOKS=1` skip them;
  * under `--dry-run` each prints `would run <product> <args>`, and
    nothing runs.
* **D2-B.** As D2-A, but every hook runs, and a failure is reported after
  the binaries are removed (as `after_install` does), exiting 3.

### D3: which products' hooks run (the issue's first question)

* **D3-A.** Only the selected products', as `before_install`: with
  `--product`, only the hooks naming a product being removed run.
* **D3-B.** Every `before_uninstall` hook, whatever was selected.

### D4: overriding a failed hook (the issue's second question)

* **D4-A.** No new option. `--no-hooks` already removes the binaries
  without running any hook (F5); the guide says so beside the exit code.
* **D4-B.** A new `--force` / `-Force` that runs the hooks and removes the
  binaries even when one fails.

### D5: the library floor (not in the issue)

* **D5-A.** `specFloors` gains a floor at the release that adds the hook:
  a spec with a `before_uninstall` hook in a module that requires an older
  library fails the build workflow's plan step, naming the `go get` to
  run.
* **D5-B.** No floor; the release notes say to update the library first.

### D6: where `install.sh` reads its hooks during an uninstall (not in the issue)

* **D6-A.** The uninstall path makes its own work directory with `mktemp
  -d` under `${TMPDIR:-/tmp}`, removed by the same `cleanup` trap, so
  `run_hooks` is unchanged. `$DIR` may not exist on an uninstall, and an
  uninstall writes nothing into it.
* **D6-B.** `run_hooks` reads `$HOOKS` from a here-document instead of a
  file, on both paths.

## Decision Outcome

Chosen options (the owner, 2026-10-19, answering each question):
"D1-A", "D2-A", "D3-A", "D4-B", "D5-A" and "D6-A", released as
`v1.17.0`. The owner chose D4-B, a `--force` option, over the proposed
D4-A. Its behaviour, which D4-B left open, is set here:

* **`--force` / `-Force`** applies only with `--uninstall` /
  `-Uninstall`; given without it, it is a usage error (exit 1). It has
  no environment variable, so it cannot be left on by accident.
* **With it,** every selected `before_uninstall` hook runs. A failure is
  reported and does not stop the others. The binaries are then removed,
  and a run with a failed hook exits 3, "removed, but a
  before_uninstall hook failed": the uninstall counterpart of
  `after_install`'s 3.
* **With `--no-hooks`** it changes nothing, since no hook runs. **With
  `--dry-run`** it prints what would run and changes nothing.

The reasons, with D4-B's in place of D4-A's:

* **D1-A** gives the consumer the seam the issue asks for, in the spec
  field that already holds `before_install` and `after_install`. D1-B
  leaves the two-command uninstall the issue reports. D1-C invents a
  command contract every product would have to meet.
* **D2-A** keeps the uninstall atomic in the only sense an installer can:
  either the service teardown succeeded and the binaries go, or nothing is
  removed and the operator sees which hook failed. D2-B would remove a
  binary whose service teardown failed, which is the broken state the
  issue describes.
* **D3-A** matches `before_install`'s selection exactly (F2, F3). A hook
  that reaches beyond its product is the consumer's to scope; the issue
  agrees.
* **D4-B** is the owner's choice. `--no-hooks` removes without trying
  the teardown; `--force` tries it and removes regardless, so an
  operator whose service is already half gone still gets the teardown
  attempted and the binaries removed, with exit 3 saying a hook failed.
* **D5-A** protects a program from its own spec (F6), as the `installer`
  field already is. D5-B leaves a program that cannot start.
* **D6-A** changes only the uninstall path. D6-B touches the install
  path, which is tested and shipped, for no gain.

### Consequences

* Good, because a program that installs a service can remove it in the
  same uninstall, and magic-cli-remote's two-command step goes.
* Good, because the new hook has no rule of its own to learn: every rule
  is `before_install`'s.
* Neutral, because an uninstall can now fail (exit 1) where it never
  could, when a hook fails; `--force` or `--no-hooks` gets past it.
* Neutral, because `--force` is a new option on both installers, for
  uninstall only.
* Neutral, because a program that adds the hook must require
  `v1.17.0`, which the plan step enforces.
* Bad, because both templates and their tests change, and the Windows
  tests must run on Windows.

### Confirmation

* `releasespec` accepts `before_uninstall`, still refuses `after_uninstall`,
  and `make apicheck` is compatible with `v1.16.0`.
* Each installer's new tests are seen failing on a plant of today's
  template:
  * a hook that runs before removal, and runs the installed copy;
  * a failing hook that leaves every binary in place, with exit 1 and the
    message;
  * a product not installed;
  * `--no-hooks` and `<PREFIX>_NO_HOOKS=1`;
  * `--force`, which removes past a failed hook and exits 3, and is
    refused without `--uninstall`;
  * `--dry-run`;
  * `--product` selecting another product.
* The floor test refuses a spec with the hook in a module that requires
  `v1.16.0`.
* `scripts/check-installers.sh` (shellcheck, `dash -n`,
  PSScriptAnalyzer) passes on both templates.
* `make gate` ends `overall=0`; `v1.17.0`'s CI passes, with the installer
  containers and the Windows installer tests.

## Pros and Cons of the Options

### D1-A. A `before_uninstall` hook

* Good, because it is one constant and one accepted value in an existing
  field.
* Bad, because both installers change.

### D1-B. Document two commands

* Good, because nothing changes.
* Bad, because the broken state the issue reports remains the default.

### D1-C. A standard `uninstall` command

* Bad, because it imposes a command on every product, and the spec could
  no longer say which product does what.

### D2-A. Before, all or nothing, exit 1

* Good, because a failed teardown leaves the binaries where the service
  expects them, and says so.
* Neutral, because an operator who wants the binaries gone anyway uses
  `--no-hooks`.

### D2-B. Continue, report after, exit 3

* Bad, because it can remove the binary of a service it failed to
  remove.

### D3-A. Selected products only

* Good, because it is `before_install`'s rule.

### D3-B. Every hook

* Bad, because `--uninstall --product a` would run b's teardown and leave
  b installed.

### D4-A. `--no-hooks` is the override

* Good, because it adds no option, and the existing one already does it.

### D4-B. `--force`

* Bad, because it adds an option whose only effect is to run a hook and
  ignore it.

### D5-A. A floor

* Good, because a program cannot be built with a spec it cannot parse.

### D5-B. A release note

* Bad, because the failure is at the program's start-up, after it ships.

### D6-A. A work directory for uninstall

* Good, because the install path is untouched.

### D6-B. A here-document

* Bad, because it rewrites a shipped, tested path for no behaviour change.

## More Information

* Issue: GitHub issue #1 of this repository. It is closed when
  `v1.17.0` ships, on the owner's ask.
* [0014-MADR-shared-installer-templates.md](0014-MADR-shared-installer-templates.md)
  decided the hooks, and its `## Amendments` gains a pointer here once
  this record is accepted.
* [0015-MADR-remediate-third-debugging-pass-findings.md](0015-MADR-remediate-third-debugging-pass-findings.md)
  F3 introduced the library floor that D5-A extends.
* The consumer's record is in another repository: magic-cli-remote,
  `docs/decisions/0054-MADR-standardize-self-update-on-go-selfupdate-lib.md`,
  D11.
* Out of scope: listing the install hooks under an install's `--dry-run`,
  which today prints only the downloads, and an `after_uninstall` time,
  which would run a product whose binary is gone.
