---
status: complete
date: 2026-10-07
associated-madr: "0014-MADR-shared-installer-templates.md"
---
# Implement the shared installer templates: the spec's `installer` field, `install.sh` and `install.ps1` rendered and staged by the build workflow, and their tests (`v1.10.0`)

Associated MADR: [0014-MADR-shared-installer-templates.md](0014-MADR-shared-installer-templates.md)

## Goal

* A program that adds `"installer": {}` to its release spec gets an
  `install.sh` and an `install.ps1` in every release, rendered from this
  repository's templates with its products, platforms, formats,
  channels, repository and tag (MADR §2–§4).
* The installers meet MADR §4's contract, and fix D1–D22 for every
  program that adopts them.
* They are tested as written and as rendered: under `sh`, dash, bash and
  BusyBox, under Windows PowerShell 5.1 and PowerShell 7, as a file,
  through `irm | iex` and through `[scriptblock]::Create`.
* `v1.10.0` is tagged. `make apicheck` reports it compatible with
  `v1.9.0`.

## Scope

### In scope

| Phase | Work |
| :--- | :--- |
| I0 | Records: the MADR's answers and acceptance, this PLAN, `docs/README.md` rows. No code. |
| I1 | `selfupdate/releasespec`: the `installer` field, its rules, `ExtraNames` with the installers, the fuzz target and parity |
| I2 | The templates as valid static scripts, the renderer, `selfupdate-release installer`, and rendering in `stage` |
| I3 | `install.sh`'s behaviour tests, and a BusyBox and dash container job |
| I4 | `install.ps1`'s behaviour tests, under 5.1 and 7, with `iex` and scriptblock forms |
| I5 | CI: template linting, the fixture spec's installer, the rehearsal staging the installers |
| I6 | Docs, guide sections, migration guide §10, release notes |
| I7 | The release: the owner's push and tag, the pin commit, the live rehearsal in a throwaway repository, its removal |

### Out of scope

* Moving any program onto the installers, and moving magic-cli-remote's
  service management or recall's `configure` into hooks. Each repository
  does that under its own records (MADR §6).
* Package managers, signing the installers, system-wide installs, shell
  completion and MCP client registration (MADR §7).
* darwin/amd64: the templates do not refuse it if a spec lists it, and no
  test or runner targets it (0012-MADR §10).
* Push, tags, and the throwaway repository of I7, which need the owner's
  ask.

## Rules for every phase

The rules of
[0013-PLAN-build-and-stage-release-workflow.md](0013-PLAN-build-and-stage-release-workflow.md)
apply: tests first, each seen to fail on a planted break in a scratch
copy; `gate.sh` before each commit; `make pre-add-check` on staged Go
files; markdownlint and `doccheck.py` on documents; one commit per
phase, staged by the agent and committed by the owner with
`git commit --no-edit`; a dated deviation entry, and a MADR amendment
where a decision or fact changes, before continuing past a surprise.

In addition:

* **API:** `make apicheck` reports `compatible with v1.9.0` at every
  phase. `go.mod` and `go.sum` do not change.
* **Scripts:** the templates pass shellcheck 0.11.0 (`-s sh`), `dash -n`
  and PSScriptAnalyzer (no warnings or errors) at every phase they
  exist. `install.sh` uses no `local`; `install.ps1` is saved without a
  BOM, with CRLF-safe parsing and LF line endings in the repository.
* **Hosts:** the Linux and Windows test hosts run I3's and I4's tests as
  well as CI.

*(2026-10-07)* The session tools these records cite are now in the
repository: `gate.sh` as `scripts/gate.sh` (`make gate`), `doccheck.py` as
`scripts/check-docs.sh`, and `plantcopy.py` as `scripts/plant-copy.sh`
([0015-PLAN-remediate-third-debugging-pass-findings.md](0015-PLAN-remediate-third-debugging-pass-findings.md)
R1).

## Implementation Steps

### Phase I0: records

1. The MADR gains "### 10. Owner answers (2026-10-06)" with Q1–Q4
   verbatim, and the status `accepted`. Every answer follows its
   recommendation, so no section changes.
2. This PLAN, amended to the answers, and its row in `docs/README.md`
   beside the MADR's.
3. The owner approves this PLAN before I1.

### Phase I1: `selfupdate/releasespec` learns the installer

**Files:** `selfupdate/releasespec/spec.go`, `validate.go`, and their
tests and `testdata/`.

1. **Tests first:**
   * accepted: `"installer": {}`; with `name`, `env_prefix`, and hooks of
     both kinds; `installer` absent (nothing changes);
   * refused, each naming its field: a bad `name`; `env_prefix` with
     lower case or over 32 characters; 9 hooks; a hook `when` other than
     `before_install` or `after_install`; a hook naming an unlisted
     product; empty, over-long or NUL hook arguments; an extra named
     `install.sh` or `INSTALL.PS1` while `installer` is present;
   * `ExtraNames`: the installers appended after the spec's extras,
     `install.sh` only with a non-Windows platform and `install.ps1` only
     with a Windows one; nothing appended without `installer`;
   * `EnvPrefix(repoName)` and `InstallerName(repoName)` apply the
     defaults of MADR §2;
   * `TestVerifierParity` gains a spec with `installer`: the staged
     release, installers included, passes the publish verifier;
   * `FuzzParse` seeds a spec with `installer`; its round trip holds.
2. **The code:** types `Installer{Name, EnvPrefix string; Hooks []Hook}`
   and `Hook{When, Product string; Args []string}` with JSON tags;
   `Spec.Installer *Installer` (`json:"installer,omitempty"`); the
   validation; `ExtraNames` and the two default helpers.
3. **Plants:** the reserved-name check removed; a hook on an unlisted
   product accepted; `install.ps1` listed for a spec without Windows;
   the env-prefix default keeping a hyphen.

### Phase I2: the templates and the renderer

**Files:** `internal/cmd/selfupdate-release/installer/install.sh`,
`install.ps1` (embedded with `go:embed`); `render.go`, `installer.go`
(the subcommand); `stage.go`; tests.

1. **The templates,** each a complete script with sample values in the
   marked block, implementing MADR §4. Written to pass the script rules
   above from the first commit.
2. **The renderer:**
   * finds exactly one block between the markers, or fails;
   * builds the values from the spec, the repository and the tag:
     repository, tag, channels, products, each product's identity and
     hook arguments, each platform's asset name and format, the folder
     name and the environment prefix;
   * validates every value against its rule, and the repository against
     `^[A-Za-z0-9-]+/[A-Za-z0-9._-]+$`, and single-quotes it (sh) or
     single-quotes and doubles `'` (PowerShell, though no rule admits
     one);
   * writes LF line endings and no BOM.
3. **`selfupdate-release installer -spec … -repository … -tag … -out
   DIR`** renders both into `DIR`.
4. **`stage`** renders them into the staging directory when the spec has
   `installer`, before `SHA256SUMS` (which they are not in) and the
   extras. `plan`'s `extra-assets-json` already lists them through
   `ExtraNames`. The build workflow passes `-repository
   "$GITHUB_REPOSITORY"`; `stage` gains the flag.
5. **Tests:** block replacement; a template without the block, or with
   two, refused; every refused value; the rendered scripts parse
   (`sh -n`, and `pwsh -NoProfile -Command` with the parser API when
   `pwsh` is present); `stage` with the fixture's installer spec stages
   both, and the publish verifier accepts the set.
6. **Plants:** a value written unquoted; the block searched for only
   once with a second left in place; `stage` dropping `install.ps1`; a
   repository with a quote accepted.

### Phase I3: `install.sh`'s behaviour

**Files:** `internal/cmd/selfupdate-release/installer_sh_test.go`, test
helpers, and a CI job.

1. **The harness:** a Go test renders the installers for the fixture
   spec, builds and stages a fixture release (the B2 helpers), and serves
   it from an `httptest` server at `SELFUPDATE_INSTALL_BASE_URL`, at the
   path `/<repository>/releases/download/<tag>/<name>`. It runs the
   rendered `install.sh` under each shell found on `PATH` (`sh`, `dash`,
   `bash`, `busybox sh`) with `HOME` and the install directory in
   `t.TempDir()`.
2. **Cases,** each asserting the exit code, the stdout or stderr line,
   and the files under the install directory:
   * a fresh install of every product; the installed bytes equal the
     staged; mode 0755; `--product` a subset;
   * a reinstall keeps `<name>.prev`;
   * `--version` with and without `v`, and `<PREFIX>_VERSION`; a version
     the channels refuse (exit 1); a version whose release lacks the asset
     names this installer expects, such as a raw release asked for by an
     archive release's installer (exit 1, that release's own installer URL
     printed);
   * a checksum mismatch (exit 2, nothing changed); a `SHA256SUMS` line
     with CR, with 63 hex characters, duplicated, or missing (exit 2);
   * an identity mismatch (a fixture flag makes `version` lie): exit 2,
     the previous binary restored;
   * archives: tar.gz and gz from the archive fixture;
   * hooks: `before_install` runs the old copy and a failure stops the
     install; `after_install` runs the new copy and a failure exits 3;
     `--no-hooks`;
   * `--uninstall` removes binaries and `.prev`;
   * `--dry-run` changes nothing;
   * an unsupported platform (a `uname` stub on `PATH`) lists the
     supported ones; a Rosetta stub (`uname -m` x86_64,
     `sysctl.proc_translated` 1) installs darwin/arm64;
   * root refused without `--allow-root` (an `id -u` stub);
   * the template uses no `local` (a static check on its text);
   * `SELFUPDATE_INSTALL_BASE_URL` with a non-loopback `http://` refused;
   * Ctrl-C: SIGINT during a slowed download leaves no temporary file and
     exits non-zero;
   * a truncated script (the rendered file cut in half) runs nothing.
3. **The container job** (Linux CI): the same test binary in `alpine`
   (BusyBox `ash`, musl) and `debian` (dash), cross-compiled with
   `CGO_ENABLED=0`, so the static Go binary is also run under musl.
4. **Plants:** the checksum step skipped; CR accepted; the Rosetta
   correction removed; `.prev` not restored on an identity failure; the
   INT trap without `exit`; a `local` added (the static check); `wget`
   without `--https-only`.

### Phase I4: `install.ps1`'s behaviour

**Files:** `internal/cmd/selfupdate-release/installer_ps_test.go`.

1. **The harness,** as I3's, runs the rendered `install.ps1` under
   `powershell.exe` (5.1) and `pwsh` (7) where present, three ways: `-File`,
   `irm <url> | iex` with arguments in environment variables, and
   `& ([scriptblock]::Create((irm <url>))) -Version …`. `LOCALAPPDATA`
   points into `t.TempDir()`; PATH updates go to a scratch registry key
   named by a test-only variable that the script honours only when
   `SELFUPDATE_INSTALL_BASE_URL` is a loopback URL.
2. **Cases:** I3's, for Windows assets (zip, `.exe`), plus:
   * the user PATH updated once, `REG_EXPAND_SZ` kept, a `%VAR%` entry
     not expanded, and nothing written with `-NoPathUpdate`;
   * a running copy replaced: the old `.exe` held open by a running
     process, renamed, the new one in place;
   * after `iex`, the caller's session has no new variables or
     functions, and strict mode and `$ErrorActionPreference` are unchanged;
     the one lasting change, on 5.1, is TLS 1.2 added to the protocol set,
     which the case asserts by name;
   * `-Uninstall` removes the binaries, `.prev` and the PATH entry.
3. **CI:** windows-2025 runs the Go tests (they find both PowerShells);
   PSScriptAnalyzer runs on the template under `pwsh` on Linux.
4. **Plants:** `-UseBasicParsing` removed (the 5.1 run hangs, so the test
   has a timeout); the PATH written through `SetEnvironmentVariable`
   (the `%VAR%` case fails); state leaking from the scriptblock; the
   `$PSCmdlet` guard removed; a BOM written by the renderer.

### Phase I5: CI and the rehearsal

1. **The fixture spec** gains `"installer": {"hooks": [...]}`, with an
   `after_install` hook running a fixture subcommand that writes a
   marker file next to the binary.
2. **`ci.yml`:** shellcheck and `dash -n` on the `install.sh` template;
   PSScriptAnalyzer on the `install.ps1` template; the container job of
   I3; the existing rehearsal now stages and verifies both installers.
3. **`workflow-shape_test.sh`:** `stage` in the build workflow passes
   `-repository`.
4. **Plants:** a planted shellcheck finding in the template fails the
   lint step; the fixture spec without `installer` makes the rehearsal
   check's installer assertion fail.

### Phase I6: docs and release notes

1. **`docs/guides/building-releases.md`:** a section "Installers": the
   field, the one-liners (latest and pinned), the flags and variables,
   hooks, the exit codes, `--verify-attestation`, and moving from a
   hand-written installer, including `env_prefix`.
2. **`README.md`, `docs/architecture.md`, `docs/README.md`:** the field,
   the templates, the tree, the counts, the rows.
3. **The migration guide:** "10. From v1.9 to v1.10".
4. **Release notes for `v1.10.0`** in this PLAN.

### Phase I7: the release

1. **The owner** pushes, waits for CI, tags `v1.10.0` and pushes the tag.
2. **The pin commit:** pins, `go get` lines, checks and
   `architecture.md`'s commit move to `v1.10.0`.
3. **The live rehearsal,** on the owner's ask, in a throwaway public
   repository with immutable releases:
   * a program with two products, `"installer"` with an `after_install`
     hook, released raw (`v0.0.1`, `v0.0.2`) then as archives
     (`v0.0.3`);
   * on the Mac and the Linux host:
     `curl -fsSL https://github.com/<repo>/releases/latest/download/install.sh | sh`,
     then the pinned `v0.0.1` URL, then `--uninstall`;
   * on the Windows host, under 5.1 and 7:
     `irm https://github.com/<repo>/releases/latest/download/install.ps1 | iex`,
     the scriptblock form with `-Version v0.0.1`, then `-Uninstall`;
   * `--verify-attestation` with an authenticated `gh` on the Mac;
   * then the repository is deleted.
4. **The agent's checks:** CI on the tag; the proxy.

## Verification

* **V1.** Every new test, rule and gate was seen to fail on a planted
  break, recorded per phase with the failure text.
* **V2.** `make apicheck`: `compatible with v1.9.0`; `go.mod` and
  `go.sum` unchanged.
* **V3.** A spec without `installer` stages exactly what `v1.9.0`
  staged: the 0013 fixtures and parity cases pass unmodified.
* **V4.** Each of D1–D22 that the templates address has a test that
  would fail with the defect present; the record lists each against its
  test.
* **V5.** The templates pass shellcheck, `dash -n` and PSScriptAnalyzer
  as written and as rendered.
* **V6.** The behaviour tests pass under `sh` (macOS), dash, bash and
  BusyBox, and under PowerShell 5.1 and 7 in all three invocation forms.
* **V7.** The live rehearsal passes every check of I7.
* **V8.** The MADR's "Not verified" entries are each restated with what
  this PLAN established: Alpine and BusyBox (I3), TLS 1.2 on 5.1 (I4).
* **V9.** CI is green on `main` and on `v1.10.0`.

## Rollout and Rollback

* **Rollout.** Opt-in: nothing changes for a spec without `installer`.
  A program adopts by adding the field and deleting its own scripts and
  their staging; its README one-liners keep their URLs, since the asset
  names are the same.
* **Rollback.** Before the tag, phases revert with their dependants (I2
  needs I1; I3 and I4 need I2; I5 needs I2–I4). After the tag, fix
  forward in `v1.10.x`. A program that meets a defect removes the field
  and stages its previous scripts as extras again; its published
  releases are immutable and keep their installers.

## Execution Record

### Phase I0: records (2026-10-06)

* **Steps 1 and 2.** The owner answered Q1–Q4: "1-4, follow
  recommendations." The answers are in MADR §10, the MADR is `accepted`,
  and `docs/README.md` shows it so.
* **Step 3.** The owner approved this PLAN: "approved, proceed". It is
  `in-progress`.
* **Checks:** markdownlint on copies of the records and on
  `docs/README.md`; `doccheck.py` on links and identifiers. No code
  changes, so `gate.sh` is not run.

### Phase I1: `selfupdate/releasespec` learns the installer (2026-10-06)

* **The code** (`installer.go`, new; `spec.go`; `validate.go`):
  * `Installer{Name, EnvPrefix, Hooks}`, `Hook{When, Product, Args}`,
    `Spec.Installer` (a pointer, so `{}` turns the installers on), the
    constants `InstallerScript`, `InstallerPowerShell`,
    `HookBeforeInstall` and `HookAfterInstall`;
  * the rules of MADR §2, checked after the platforms and before the
    extras;
  * `InstallerScripts()`: `install.sh` for a platform that is not
    Windows, then `install.ps1` for a Windows one;
  * `ExtraNames` appends them, and reserves both names, compared ignoring
    case, whenever `installer` is present;
  * the default helpers are `InstallerName(repo)` and
    `InstallerEnvPrefix(repo)`, the PLAN's `InstallerName` and
    `EnvPrefix` under those names. A made prefix that is not valid (a
    repository name starting with a digit, or longer than 32) is an error
    naming `installer.env_prefix`.
* **Tests** (`installer_test.go`, `testdata/installer.json`): the fixture
  and `{}` accepted, and no installers without the field; 12 refusals,
  each naming its field; the installer names as ordinary extras without
  the field; `InstallerScripts` and `ExtraNames` for mixed, Unix-only and
  Windows-only platforms; the defaults for four repository names and
  their refusals.
* **`TestVerifierParity`** gains `installer.json`: a release staged with
  both installers passes the publish verifier, on macOS and on the
  Windows test host.
* **`FuzzParse` found a defect** in its first gate run: `"hooks": []`
  parsed to an empty list, which encoding drops, so the spec changed in a
  round trip. `Parse` now normalizes an empty hook list to nil, as it
  does extras and channels. The failing input is kept as a regression
  seed, `testdata/fuzz/FuzzParse/60baf1860bb34233`, which passes with the
  fix and fails without it ("round trip changed the spec").
* **Plants,** each in a scratch copy, each caught:

  | Plant | Caught by |
  | :--- | :--- |
  | the installer names not reserved | `TestInstallerRefused`: Parse accepted the spec; want "is already an installer's name" |
  | a hook on an unlisted product accepted | `TestInstallerRefused` |
  | a hook at any time accepted | `TestInstallerRefused` |
  | `install.ps1` without a Windows platform | `TestInstallerScriptsAndExtraNames`: `InstallerScripts = [install.sh install.ps1], want [install.sh]` |
  | `ExtraNames` without the installers | `TestInstallerScriptsAndExtraNames` |
  | the made prefix keeping hyphens | `TestInstallerDefaults` |
  | the empty-hooks normalization removed | the fuzz seed |

* **Checks:** `gate.sh`, every step rc 0, run twice (the second after the
  fuzz fix); `make apicheck`: `compatible with v1.9.0`; `make
  pre-add-check` on the five Go files: "5 file(s) clean".

### Phase I2: the templates and the renderer (2026-10-06)

* **`installer/install.sh`** (POSIX `sh`, 430 lines) and
  **`installer/install.ps1`** (PowerShell 5.1 and 7, 429 lines), each a
  complete script with sample values in its marked block, implementing
  MADR §4. Decisions made while writing them:
  * **`wget`.** BusyBox `wget` has no `--https-only`. `install.sh` uses
    `curl` first, then a `wget` that lists `--https-only` in its help, and
    refuses any other `wget` ("install curl") rather than risk plain HTTP.
  * **Exit codes under `iex`.** `exit` there would close the user's
    session. `install.ps1` exits with its code only when the scriptblock
    was defined in a file (`$MyInvocation.MyCommand.ScriptBlock.File`);
    under `iex` or `[scriptblock]::Create` it prints the message and
    throws `install failed (exit N)`.
  * **No `$PSCmdlet`, no `-WhatIf`.** `-DryRun` does that job, so D2's
    unguarded call has nothing to guard.
  * **`Install-Release` takes its options as parameters.** Reading the
    outer scriptblock's parameters through dynamic scope left
    PSScriptAnalyzer reporting 8 `PSReviewUnusedParameter` findings, and
    an early `$script:NoHooks` would have leaked a variable into the
    caller's scope under `iex`; both are gone.
  * **The broadcast** of `WM_SETTINGCHANGE` defines one type,
    `SelfupdateInstall.NativeMethods`, in the process, as Bun's installer
    does; it is skipped when a test redirects PATH updates.
* **The renderer** (`render.go`): `newInstallerValues` builds the values
  from the spec, the repository and the tag; `check` refuses any value
  outside `^[A-Za-z0-9._:=/,+@%-]+$`, whatever validated it before;
  `replaceBlock` replaces the one block, keeps the markers, indents as
  the start marker is, and refuses no block, two starts, two ends and CR
  endings. Output has LF endings and no BOM.
* **`selfupdate-release installer`** renders both into a directory;
  **`stage`** renders them into the staging directory when the spec asks
  (`-repository` required then), and the build workflow passes
  `-repository "$REPOSITORY"` from `github.repository`.
* **D1** added the argument charset to `releasespec` (see Deviations).
* **Checks of the scripts as written and as rendered:**
  * shellcheck 0.11.0 (`-s sh`): clean, after replacing `tr 'A-Z' 'a-z'`
    with `[:upper:]`/`[:lower:]` (SC2018, SC2019);
  * `sh -n` (macOS), `dash -n`, `bash -n`: clean, in the tests;
  * PSScriptAnalyzer 1.25.0 on the Windows test host, warnings and
    errors: "0 findings" for the template and a render;
  * the PowerShell parser on this Mac: 0 errors;
  * `-DryRun` of a render on the Windows test host, under 5.1 and 7:
    exit 0, the windows/amd64 assets and
    `%LOCALAPPDATA%\Programs\relay-suite`;
  * on this Mac under pwsh 7.6.6: as a file, exit 1 with "install.ps1 is
    for Windows"; under `iex`, the same message, a thrown error, the
    session kept, and no new variable or function from the script.
* **Tests** (`render_test.go`): the values in each script, nothing
  changed outside the block, no CR or BOM; the scripts a spec's
  platforms produce; refusals of repository, tag, unsafe values and a
  digit-led prefix; `replaceBlock` cases each with its message; the
  templates end by calling `main` and the scriptblock, use no `local` as
  a command, and parse in every shell present; `stage` writes both, not
  in `SHA256SUMS`, and needs `-repository`; the subcommand. They pass on
  macOS, the Linux test host and the Windows test host.
* **Plants,** each in a scratch copy, each caught at an assertion:

  | Plant | Caught by |
  | :--- | :--- |
  | a value written unquoted | `TestRenderInstallers`: `install.sh lacks "TAG='v1.2.3-rc.1'"` |
  | a second start marker accepted | `TestReplaceBlock`: `two starts, one end: replaceBlock: <nil>` |
  | `stage` dropping `install.ps1` | `TestStageWritesInstallers` |
  | a quoted repository accepted | `TestRenderRefuses` |
  | the renderer's own check bypassed | `TestRenderRefuses`: `check: <nil>` |
  | the end marker dropped | `TestRenderInstallers` |
  | `local` in `install.sh` | `TestTemplatesAsWritten`: `install.sh uses local` |
  | D1: `identity_args` unchecked | `TestInstallerArgumentCharset` |
  | D1: a quote in a hook argument | `TestInstallerRefused` |

  The second plant was first missed: the "two blocks" case had two
  starts and two ends, so the end check refused it anyway. A case with
  two starts and one end, and a message asserted per case, now catch it.
* **Checks:** `gate.sh`, every step rc 0; actionlint, `check-workflows.sh`
  and the shape test on the changed workflow; `make pre-add-check` on the
  seven Go files: "7 file(s) clean".

### Phase I3: `install.sh`'s behaviour (2026-10-06)

* **The harness** (`installer_harness_test.go`, shared with I4): one
  build of the fixture with two products (`relay`, which reports its
  identity, and `relayctl`, which does not) for linux/amd64, linux/arm64,
  darwin/arm64, windows/amd64 and the host, staged three times at
  `v1.2.3` with installers: raw (`fixture/relay`), tar.gz
  (`fixture/relay-tgz`) and gz (`fixture/relay-gz`). An `httptest`
  server serves `/<owner>/<repo>/releases/download/<tag>/<name>` from
  them. A test can route another tag to a release, replace a file,
  replace an asset with its `SHA256SUMS` line updated, or stall a
  response halfway. `SELFUPDATE_INSTALL_TEST_RELEASES` names a directory
  for the staged releases: a run that finds it complete reuses it,
  otherwise it builds into it and keeps it. It is built for `unix`
  only until I4's tests use it on Windows: on `GOOS=windows` the lint's
  `unused` check refused it with no caller.
* **The tests** (`installer_sh_test.go`, `unix` only) run the rendered
  script as `curl … | sh -s -- …` runs it, on standard input, with
  `HOME` and the install directory in a temporary directory and stubs
  ahead of `PATH`, under each of `sh`, `dash`, `bash` and `busybox sh`
  found once by its real path. Each case checks the exit code, a line
  of output, and the install directory byte for byte and mode 0755.
  * `TestInstallSh`, 27 cases: the PLAN's list, plus `--dir` and
    `RELAY_INSTALL_DIR`, an upper-case hash, a fresh install skipping
    `before_install`, `RELAY_NO_HOOKS`, `--uninstall --product` and
    `--uninstall --dry-run`, `aarch64`, an Intel Mac (the sysctl
    failing), and base URLs carrying user information. A stand-in
    script, with its line in `SHA256SUMS`, plays the copy a hook or
    the identity check runs.
  * `TestInstallShInterrupt`: SIGINT to the process group while the
    asset stalls halfway; the temporary directory exists before, and
    after the script exits 130 the install directory is empty.
  * `TestInstallShTruncated`: the script cut at each of its 435 line
    ends and at half its bytes; no request reaches the server and
    nothing is written under `HOME`.
  * `TestInstallShFetchers`: with no `curl` on `PATH`, no `wget` either
    (exit 1); a `wget` without `--https-only` refused before it runs; one
    with it run as `wget --https-only -q -O …`.
  * `TestInstallShHashTools`: no `sha256sum`, `shasum` or `openssl`:
    exit 1, nothing installed.
  * `SELFUPDATE_INSTALL_REQUIRE_SHELLS` lists shell binaries a run must
    reach with working stubs, so a container job cannot pass without its
    shell.
* **D2** fixed two defects in the templates; **D3** skips six cases
  under a shell that runs its own applets ahead of `PATH` (see
  Deviations).
* **The container job** (`ci.yml`, `installer-containers`): the runner
  stages the releases and cross-compiles the test binary with
  `CGO_ENABLED=0`, then runs it as the runner's user in
  `alpine:3.24.2` (BusyBox ash and wget, musl, no curl; required shell
  `busybox`) and `buildpack-deps:trixie-curl` (dash and curl; required
  `dash,bash`), each pinned by index digest. Its first run is CI's.
* **Runs:**
  * this Mac (`/bin/sh`, `/bin/dash`, Homebrew bash): the package's
    tests pass;
  * the Linux test host (dash as `sh`, bash, Ubuntu's `busybox-static`):
    "pass=95 skip=6 fail=0", the six being D3's;
  * an Alpine 3.24.2 minirootfs (sha256 checked) in a chroot on the
    Linux test host, not as root, `curl` absent, required shell
    `busybox`: "pass=37 skip=0 fail=0". This settles the MADR's first
    two "Not verified" entries.
* **Plants,** each in a scratch copy, each caught at an assertion:

  | Plant | Caught by |
  | :--- | :--- |
  | the checksum step skipped | "an altered asset": `exit 0, want 2` |
  | CR accepted (`sub(/\r$/, "")` in the parser) | "SHA256SUMS with CR line endings": `exit 0, want 2` |
  | the Rosetta correction removed | "Rosetta is corrected to arm64": `exit 1, want 0` |
  | `.prev` not restored | "an identity mismatch restores the previous copy": the directory's contents |
  | the INT trap without `exit` | `TestInstallShInterrupt`: `exit 2 after SIGINT, want 130` |
  | a `local` added | `TestTemplatesAsWritten`: `install.sh uses local` |
  | `wget` without `--https-only` | `TestInstallShFetchers`: `wget ran as "wget -q -O …"` |
  | D2: the asset fetched before `SHA256SUMS` is read | "… with other asset names": `exit 2, want 1` |
  | D2: any text after the loopback colon | the base URL case: `exit 2, want 1` |
  | `hash_of` hashing nothing | `TestInstallShHashTools`: `exit 2, want 1` |
  | a command at the top level | `TestInstallShTruncated`: `a truncated script wrote to its home: [.local/ stubs/]` |

  `SELFUPDATE_INSTALL_REQUIRE_SHELLS=busybox` on this Mac, which has no
  BusyBox, failed the run with "no busybox that honours PATH".
* **`install.ps1`** (D2): PowerShell's parser reports 0 errors here and
  on the Windows test host, and PSScriptAnalyzer 0 findings there.
  I4's tests cover the change.
* **Checks:** `gate.sh`; actionlint 1.7.12; `check-workflows.sh` (all,
  and `expressions` and `pins` on `ci.yml`); `workflow-shape_test.sh`;
  shellcheck 0.11.0 on the template; `make pre-add-check` on the three
  Go files.

### Phase I4: `install.ps1`'s behaviour (2026-10-06)

* **The harness** is shared now, as I3's record said it would be: its
  `unix` constraint is gone, `exitCode`, `filesIn`, `expectFiles` (mode
  0755 checked outside Windows only), `runResult`, `rawRepo` and
  `SELFUPDATE_INSTALL_REQUIRE_SHELLS` moved into it, the server offers
  the script under test at a path it does not count as a request, and
  `TestMain` removes any directory registered with `keepUntilExit`.
* **The tests** (`installer_ps_test.go`, Windows only) run the rendered
  script under `powershell.exe` (5.1) and `pwsh.exe` (7), with
  `LOCALAPPDATA` and the install folder in a temporary directory, PATH
  updates sent to a scratch key `HKCU\Software\SelfupdateInstallTest\<pid>-<n>`
  (deleted after each case), and the environment cleaned of `RELAY_*`,
  `SELFUPDATE_INSTALL_*` and `PSModulePath`. Three forms:
  * **file:** `-File install.ps1 <options>`;
  * **iex:** a driver runs `Invoke-RestMethod <url> | Invoke-Expression`,
    options in environment variables only;
  * **scriptblock:** the driver runs
    `& ([scriptblock]::Create((Invoke-RestMethod <url>))) <options>`, the
    options as typed text.

  After each iex and scriptblock run the driver reports what the session
  kept; each run must show no new variable or function,
  `$ErrorActionPreference` and strict mode unchanged, and the TLS set
  unchanged on 7 and with only Tls12 added on 5.1 (0 -> 3072 on the test
  host).
* **`TestInstallPs1`, 33 cases,** each in the forms its options allow:
  146 runs, 33 + 9 + 31 per PowerShell. I3's cases for Windows assets
  (`.exe`, and zip from the archive release), and:
  * the user PATH: created as `REG_EXPAND_SZ`; added once to an existing
    value with its `%USERPROFILE%` entry unexpanded and its kind kept,
    `REG_SZ` included; an entry already there, with a trailing backslash,
    left alone; nothing written with `-NoPathUpdate` or
    `RELAY_NO_PATH_UPDATE`; removed with the last program on
    `-Uninstall`, and kept while another remains;
  * a running copy: a held stand-in renamed to `relay.exe.prev` and the
    new binary in place; a second install sets the still-running `.prev`
    aside as `relay.exe.old-<guid>`;
  * an unknown option and a switch given a value: exit 1, nothing done.

  Stand-ins are a small Go program built at test time, so a hook or an
  identity check runs a real `.exe` that prints, logs and exits as told.
* **`TestInstallPs1Truncated`:** one process per PowerShell runs the
  script cut at each of its 451 line ends through `Invoke-Expression` and
  through `[scriptblock]::Create`; then half the script runs as a file.
  No request, nothing under `LOCALAPPDATA`, no PATH value.
* **`TestInstallPs1Interrupt`** (D5): Ctrl+Break to PowerShell 7 during a
  stalled download; it exits non-zero and the install folder is empty.
  The test gives itself a console first if it has none, as on a CI
  runner; the test host's ssh session already had one, so that path has
  not run yet.
* **I3 cases with no Windows counterpart:** root (`--allow-root` has
  none, MADR §4); the `uname`, `sysctl` and `aarch64` stubs (the script
  reads the architecture from `HKLM`, which a test cannot replace; the
  unsupported-platform case uses a release for the other architecture
  instead); missing `curl`, `wget` or hash tools (built-ins on Windows).
* **Not tested:** that `SELFUPDATE_INSTALL_TEST_ENV_KEY` is ignored with
  an `https://` base URL. A test of it would write the real user PATH.
* **D4** fixed four defects in `install.ps1`; **D5** decided Ctrl-C;
  **D6** added a static check (see Deviations). With the template as
  committed in I3 the same tests failed 43 of 158; with the fixes, none.
* **PLAN step 3, CI:** the `go test ./...` step now sets
  `SELFUPDATE_INSTALL_REQUIRE_SHELLS` (`powershell,pwsh` on Windows,
  `dash,bash` on Linux, `sh,bash` on macOS), so windows-2025 runs the
  tests under both PowerShells or fails. PSScriptAnalyzer in CI is I5's
  step 2, which names it too; it lands there with shellcheck.
* **Runs:** the Windows test host, the whole package with both
  PowerShells required: "pass=247 fail=0 skip=0", `TestInstallPs1Interrupt/pwsh`
  included, and `TestInstallPs1*` three times over: "pass=480 fail=0
  skip=0"; this Mac, the package with `sh,dash,bash` required; the Linux
  test host, `TestInstallSh*`: "pass=95 skip=6 fail=0", the six being D3's.
* **Plants,** each in a scratch copy, the Windows ones run on the
  Windows test host:

  | Plant | Caught by |
  | :--- | :--- |
  | `-UseBasicParsing` removed | not by a run (D6); `TestTemplatesAsWritten`: `"Invoke-WebRequest -Uri $Url -OutFile $Path -TimeoutSec 60" lacks -UseBasicParsing` |
  | PATH written expanded, as `REG_SZ` | the `%VAR%` case: `PATH is REG_SZ "C:\\Users\\<user>\\bin;…"` |
  | a function leaking from the scriptblock (`function global:Write-Line`) | the iex session check: `no "driver: new functions []"` |
  | TLS 1.2 not added on 5.1 | `Windows PowerShell ran the script without adding TLS 1.2 (0 -> 0)` |
  | D4: hook output returned | "a failed before_install changes nothing": `exit 0, want 1` |
  | D4: the rejected binary renamed, not deleted (`if ($true)` for `Clear-SettledFile`) | the identity case: the folder holds `relay.exe.bad-…` |
  | D4: PATH cleared by a partial uninstall | `-Uninstall`: `PATH is REG_EXPAND_SZ ""` |
  | D4: positional binding allowed | "a switch given a value": `exit 0, want 1` |
  | D4: a binding error not a usage error | "an unknown option": `exit 0, want 1` |
  | D5: the temporary folder not removed | `TestInstallPs1Interrupt`: the folder holds `.relay-install.…` |
  | a statement at the top level | `TestInstallPs1Truncated`: `a cut script wrote to LOCALAPPDATA: [Programs/]` |
  | a BOM written by the renderer | `TestRenderInstallers`: `install.ps1: CR or BOM in the output` |

  The PLAN's "write PATH through `SetEnvironmentVariable`" plant is
  replaced by writing the scratch key as that call would, expanded and
  `REG_SZ`: the call itself writes the real user PATH. Its "`$PSCmdlet`
  guard removed" plant has no site: the template has no `$PSCmdlet`
  (I2).
* **Checks:** PowerShell's parser (0 errors) here and on the Windows
  test host, and PSScriptAnalyzer 1.25.0 there (0 findings) on the
  template and a render; shellcheck on `install.sh`; actionlint 1.7.12;
  `check-workflows.sh` (all, and `expressions`, `permissions` and `pins` on
  `ci.yml`); `workflow-shape_test.sh`; `gate.sh`; `make pre-add-check` on
  the changed Go files.

### Phase I5: CI and the rehearsal (2026-10-06)

* **The fixture** asks for installers: both fixture specs gain
  `"installer"` with one `after_install` hook, `relay mark-installed`.
  The fixture program's new `mark-installed` writes its identity to
  `relay.installed` beside itself; `TestFixtureMarksInstalled` runs the
  built program and compares the file with `relay version`. The live
  rehearsal of I7 runs the hook through the real one-liners.
* **`scripts/check-installers.sh`** lints installer files: shellcheck
  (`-s sh`) and `dash -n` on `.sh`, PowerShell's parser and
  PSScriptAnalyzer (warnings and errors) on `.ps1`, through `pwsh` or
  `$PWSH`. With `--staged DIR --repository R --tag T` it first checks a
  staged set: both installers present, rendered for `R` and `T`, and not
  in `SHA256SUMS`. Exit 1 for a finding, 2 for a usage error or a missing
  tool. `scripts/check-installers_test.sh` (17 cases) uses the templates'
  sample values as a staged set and a stub `pwsh` for the analyzer's three
  outcomes.
* **`ci.yml`:**
  * the Linux lint step runs the test, then the script on both
    templates, with shellcheck 0.11.0 and the runner's pwsh and
    PSScriptAnalyzer (I4 step 3's PSScriptAnalyzer lands here);
  * `release-rehearsal-check` gains "Check the staged installers": the
    script with `--staged` on the raw and the archive sets, for
    `github.repository` and the build's stamp (the tag, or
    `rehearsal-<sha12>` off a tag). A rehearsal's stamp is not a release
    tag, so the installers are checked as rendered and not run; I7 runs
    them;
  * the I3 container job is already in place.
* **`workflow-shape_test.sh`:** `stage` in the build workflow passes
  `-repository "$REPOSITORY"`, with `REPOSITORY` from
  `github.repository`.
* **A local rehearsal** (a scratch script running the workflow's
  sequence with the built tool on a committed copy of the fixture,
  laid out as in this repository): for each fixture spec, `plan` lists
  `install.sh` and `install.ps1` among the extras, `stage` writes them,
  the publish verifier passes ("verify-selfupdate-release: ok", "ok
  (packed)"), and the installer check is clean. With `installer` removed
  from the spec the check fails: "staging has no install.sh", "staging
  has no install.ps1".
* **The real analyzer,** through the script on the Windows test host:
  "check-installers: clean" for the template and for the `install.ps1`
  each fixture spec stages.
* **Plants,** each in a scratch copy:

  | Plant | Caught by |
  | :--- | :--- |
  | a shellcheck finding in the template (`$*` unquoted in `say`) | the CI lint command: `install.sh: shellcheck` |
  | the fixture spec without `installer` | the rehearsal's installer check (local rehearsal): `has no install.sh`, `has no install.ps1` |
  | `-repository` dropped from `stage` in the build workflow | `workflow-shape_test.sh`: `FAIL stage renders the installers for the calling repository` |
  | a PSScriptAnalyzer finding (an unused variable) | the script on the Windows test host: `PSUseDeclaredVarsMoreThanAssignments`, exit 1 |
  | the staged TAG check removed | `check-installers_test.sh`: "another tag" |
  | the `SHA256SUMS` check removed | "an installer listed in SHA256SUMS": `exit 0, want 1` |
  | a missing analyzer taken as a finding | "no PSScriptAnalyzer": `exit 1, want 2` |
  | an absent `install.ps1` not noticed | "a staged set without install.ps1": `exit 0, want 1` |
  | the fixture's marker misnamed | `TestFixtureMarksInstalled`: no `relay.installed` |
* **Checks:** shellcheck on `scripts/*.sh`; actionlint 1.7.12;
  `check-workflows.sh` (all, and `expressions`, `permissions` and `pins`
  on `ci.yml`); `workflow-shape_test.sh`; `gate.sh`; `make
  pre-add-check` on `build_test.go`; `go vet` and `gofmt` on the fixture
  module.

### Phase I6: docs and release notes (2026-10-06)

* **`docs/guides/building-releases.md`:** the introduction names the
  installers and 0014-MADR; step 1's example drops its hand-written
  `install.sh` extra for an SBOM and adds `"installer": {}`, with a bullet
  for the field; step 9's table gains the three spec errors the field
  brings (a reserved name, an argument outside the character set, a prefix
  that cannot be made); step 11 points at the new step. **Step 12,
  "Installers"**, is new, after the others so no anchor moves: the field,
  the one-liners (latest and pinned, both shells), what the installers
  do, the options with their variables, hooks, the exit codes,
  `--verify-attestation`, and moving from a hand-written installer,
  `name` and `env_prefix` included, with the test seam.
* **The migration guide:** "10. From v1.9 to v1.10", and §2's list of
  additive releases. It says to move the program's module and the
  workflow pin together: `v1.9.0`'s `Parse` sets `DisallowUnknownFields`
  (`spec.go:113` at the tag) and has no `Installer`, so it refuses the
  field in either place. The `go get` lines and "the current release"
  move in I7's pin commit, as in 0013's B6.
* **`README.md`:** the `releasespec` row names installers; the build
  workflow's paragraph says it renders them.
* **`docs/architecture.md`:** the tree gains `installer/` and
  `check-installers.sh`, and the tool's `installer` subcommand; the table's
  counts (`releasespec` 4 and 5 files and 4 specs; the tool 11 and 10, plus
  the 2 templates); the build workflow's `stage` step renders the
  installers; CI's required shells, the installer lint, the rehearsal's
  installer check and the container job.
* **`docs/README.md`:** an "I want to…" row for the installers; the
  migration row runs to §10.
* **Checked against the code,** claim by claim: the options, variables,
  defaults and exit codes against both templates; the error messages
  against `releasespec`; the exported names against `installer.go` and
  `spec.go`. One correction before staging: on Windows the kept copy is
  `<product>.exe.prev`.
* **Checks:** markdownlint-cli2 0.23.2 over the repository, "0 issues";
  `doccheck.py` for links and identifiers on every changed file.

### Release notes for `v1.10.0` (2026-10-06)

Additions only: `make apicheck` reports `compatible with v1.9.0`, and
`go.mod` is unchanged. A spec without `installer` stages exactly what
`v1.9.0` staged.

* **Generated installers.** A release spec's new `installer` field makes
  the build workflow render `install.sh` and `install.ps1` into every
  release from this repository's templates, with the program's
  products, platforms, formats, channels, repository and tag. Each
  installs its own release by tag over HTTPS only; checks every file
  against `SHA256SUMS` before installing; picks the platform (Rosetta
  read as arm64, the native architecture on Windows); keeps the previous
  binary and puts it back if the new one does not report the release;
  updates the Windows user PATH without expanding `%VAR%` entries; runs
  optional `before_install` and `after_install` hooks of the program's
  own products; supports `--version`, `--product`, `--dry-run`,
  `--uninstall` and `--verify-attestation`; and exits 0, 1, 2 or 3 as
  documented. `install.ps1` runs as a file, through `irm | iex` and
  through `[scriptblock]::Create`, under Windows PowerShell 5.1 and
  PowerShell 7, and leaves nothing in the caller's session but TLS 1.2 on
  5.1.
* **`selfupdate/releasespec`:** `Spec.Installer`, the `Installer` and
  `Hook` types, `InstallerScripts`, `InstallerName`,
  `InstallerEnvPrefix`, and the constants `InstallerScript`,
  `InstallerPowerShell`, `HookBeforeInstall` and `HookAfterInstall`.
  With `installer` present, `install.sh` and `install.ps1` are reserved
  extra names, and hook arguments and `identity_args` keep to
  `[A-Za-z0-9._:=/,+@%-]`.
* **`build-selfupdate-release.yml`:** `stage` renders the installers for
  the calling repository; nothing changes for a spec without the field,
  and the workflow's inputs and outputs are the same.
* **`publish-selfupdate-release.yml`'s archive check** (D8) also reads
  every tar.gz and gz asset to its end with gzip, and refuses one that is
  cut, has a wrong checksum, or has data after its one member, before the
  client's unpacker runs. The build workflow never writes such an asset;
  the client's own unpacker is unchanged.
* **Tooling** (no release surface): `selfupdate-release installer`;
  `scripts/check-installers.sh`; behaviour tests of both installers
  under sh, dash, bash, BusyBox, Windows PowerShell 5.1 and PowerShell
  7; a CI job running `install.sh`'s tests in Alpine and Debian; the
  launchd backend's tests no longer ask the system about a fixture PID
  (D9).

### Phase I7: the release (2026-10-07)

* **Step 1, the tag.** D7 to D9 landed as `a0a26b6`, pushed by the owner;
  its CI run, 37571411162, passed all 17 jobs, the Windows leg's
  Ctrl+Break case and the Linux lint step included. On the owner's ask
  ("Tag v1.10.0 and push it") the agent tagged `v1.10.0` on `a0a26b6`,
  annotated with the message `v1.10.0` as `v1.9.0` is, after
  `scripts/check-release-tag.sh v1.10.0` exited 0, and pushed the tag.
  `git ls-remote origin 'refs/tags/v1.10.0^{}'` gives
  `a0a26b6ecf66f51c19e9fea0f665c76ca5e99e4c`.
* **The tag's CI** (run 37577710330) passed all 17 jobs, the first run to
  build the fixture as a release with installers: the ten identity legs,
  raw and archive on the five runners, each printed
  `v1.10.0 (release) a0a26b6ecf66`, and "Check the staged installers"
  reported "check-installers: clean" for both sets, rendered for the
  tag.
* **Step 2, the pin commit:** `README.md` (the status, `go get`, the
  publish example's pin), `docs/architecture.md` (the current release and
  its commit), the building guide (both workflow pins in step 4, the
  extras example's pin, the `ls-remote` example) and the migration guide
  (every `go get` line and `go list` check, §2's current release, §3's
  pin and what `v1.10.0` changes in the publish workflow, §5's `go.mod`
  step and the list of additive releases) move to `v1.10.0` at
  `a0a26b6ecf66f51c19e9fea0f665c76ca5e99e4c`. What a version section says
  of its own release stays.
* **Step 3, the live rehearsal** (2026-10-07), on the owner's ask
  ("Proceed then run the rehearsal").
  * **The repository:** a throwaway public repository, created with
    `gh repo create`; immutable releases turned on with
    `PUT /repos/{owner}/{repo}/immutable-releases` (204), read back as
    `"enabled":true` some seconds later. The agent's token has no
    `delete_repo` scope, so the owner removes it, as for 0013.
  * **The program:** a module requiring `v1.10.0` from
    `proxy.golang.org`, with two products: `rehearsal`, with
    `identity_args: ["version"]`, a `spec` command that parses the
    embedded spec with that release's `releasespec` ("installers:
    [install.sh install.ps1]") and a `mark-installed` command; and
    `rehearsalctl`, with no identity command. The spec lists five
    platforms and `"installer"` with an `after_install` hook,
    `rehearsal mark-installed`. The caller is the building guide's,
    both workflows pinned to `a0a26b6`, on a `feature/rehearsal` branch
    as in 0013's rehearsal.
  * **Runs,** each green: the branch push, a rehearsal with five identity
    legs and `release` skipped; `v0.0.1` and `v0.0.2`, raw, and `v0.0.3`,
    archives (tar.gz and zip). Each release is immutable, with
    `install.sh`, `install.ps1`, ten assets and `SHA256SUMS`; `v0.0.2`
    became latest, then `v0.0.3`.
  * **On the development Mac and the Linux test host,** with `HOME` a
    scratch directory, so neither host's own `~/.local/bin` was touched;
    the same result on both:

    | One-liner | Exit | Result |
    | :--- | :--- | :--- |
    | `curl -fsSL …/releases/latest/download/install.sh \| sh` | 0 | both products at `v0.0.3 (release) f0d73ad1d2d9`, from tar.gz; the hook ran; `rehearsal.installed` holds that identity |
    | the same, `-s -- --version v0.0.1` | 1 | "release v0.0.1 has no rehearsal-<os>-<arch>.tar.gz; its own installer is …/releases/download/v0.0.1/install.sh" |
    | `…/download/v0.0.2/install.sh \| sh -s -- --version v0.0.1` | 0 | `v0.0.1 (release) 8df73fd36066`; `rehearsal.prev` reports `v0.0.3` |
    | `…/download/v0.0.1/install.sh \| sh` | 0 | `v0.0.1` again, the hook's marker rewritten |
    | the latest, `--uninstall` | 0 | both binaries and their `.prev` removed; "configuration, if any, is left in place" |

    On the Mac, with its own `HOME`, so `gh` was logged in, the latest
    one-liner with `--verify-attestation --dir <scratch> --no-hooks`
    exited 0, installing `v0.0.3`.
  * **On the Windows test host,** under Windows PowerShell 5.1.26100 and
    PowerShell 7.6.6, with `LOCALAPPDATA` a scratch folder and the real
    user PATH, its raw value and kind saved first:

    | Form | Result, both PowerShells |
    | :--- | :--- |
    | `irm …/latest/download/install.ps1 \| iex` | both products at `v0.0.3 (release) f0d73ad1d2d9`, from zip; the hook ran; "added … to your user PATH"; the user PATH has the folder, still `REG_SZ` |
    | `& ([scriptblock]::Create((irm …/latest/…))) -Version v0.0.1` | "release v0.0.1 has no rehearsal-windows-amd64.zip; its own installer is …/download/v0.0.1/install.ps1", then the error `install failed (exit 1)`, the session kept |
    | `& ([scriptblock]::Create((irm …/download/v0.0.2/install.ps1))) -Version v0.0.1` | `v0.0.1 (release) 8df73fd36066`; `rehearsal.exe.prev` and `rehearsalctl.exe.prev` kept |
    | the latest, `-Uninstall` | both binaries removed, and the folder off the user PATH |

    After each run the user PATH was byte for byte as before ("user PATH
    as before: True"), so the saved copy was not needed. The test
    script's own attempt to run `rehearsal.exe.prev` failed, as Windows
    runs no program without an `.exe` name; that is the script's, not
    the installer's.
  * **Removal:** the owner deleted the repository, as asked; the API then
    returned "Not Found" for it, by name, through both GraphQL and REST.
* **Step 4, the agent's checks:** CI on the tag, above; on
  `proxy.golang.org`, `go list -m …@v1.10.0` gives `v1.10.0` at
  2026-10-07T04:25:29Z, its origin `a0a26b6` and `refs/tags/v1.10.0`, and
  `@latest` already resolves to `v1.10.0`.

### Verification (2026-10-07)

* **V1.** Every new test, rule and gate was seen to fail on a planted
  break, in a scratch copy, with the failure in its phase's table: 7 in
  I1, 9 in I2, 11 in I3, 12 in I4 and a thirteenth for D4's wait, 9 in
  I5, 3 for D8, 1 for D9 and 7 for D10. D7's fix was seen against the
  failure it fixes, reproduced on the Windows test host.
* **V2.** `make apicheck` reported `compatible with v1.9.0` at every
  phase's gate. `go.mod` and `go.sum` are unchanged since `v1.9.0`.
* **V3.** A spec without `installer` stages what `v1.9.0` staged: `stage`
  renders nothing without the field, and the verifier, its fixtures,
  `stage_test.go` and `releasespec`'s parity cases are unchanged since
  `v1.9.0` (its test data only gained `installer.json` and a fuzz seed).
  One publish-side check changed by decision: D8's whole-gzip read, for
  archive releases.
* **V4.** Each of MADR's D1–D22 the templates address, against its test:

  | Defect | Test that fails with it present |
  | :--- | :--- |
  | D1, checksum lookup by a legacy name | every install case of `TestInstallPs1` and `TestInstallSh`: `SHA256SUMS` lists the canonical names only |
  | D2, an unguarded `$PSCmdlet` | none to guard: the template has no `$PSCmdlet` (I2); the `iex` and scriptblock forms run every case under strict mode |
  | D3, the asset name reported as the version | `installed <dir>/relay (v1.2.3)`, asserted in each install case |
  | D4, no version through `iex` | "RELAY_VERSION installs another release", in the `iex` form |
  | D5, `v` and validation | "-Version installs another release" (`1.2.3` and `v1.2.3`); "-Version outside the tag rule or the channels"; `install.sh`'s equivalents |
  | D6, no Windows uninstall | "-Uninstall" |
  | D7, backups and checks | "a reinstall keeps the previous copy"; "an identity mismatch restores the previous copy" (I3's plant) |
  | D8, four Windows folders | every `install.ps1` case asserts `%LOCALAPPDATA%\Programs\relay` |
  | D9, Machine entries copied to the user PATH | "the user PATH: added once…": the exact value and kind (I4's plant) |
  | D10, no real Windows coverage | `TestInstallPs1` itself: real downloads, three forms, both PowerShells, on CI's `windows-2025` |
  | D11, no shellcheck in CI | `check-installers.sh` in the lint step (I5's plant) |
  | D12, comments on versioned manifests | not testable: documentation; neither template nor guide describes them |
  | D14, test seams in production | "SELFUPDATE_INSTALL_BASE_URL is https or loopback", both scripts (D2's plant). The PATH key's loopback rule is untested, as a test would write the real PATH (I4) |
  | D15, transport | `TestInstallShFetchers` (I3's `wget` plant); curl's flags in `TestTemplatesAsWritten` (D10's plant); the 5.1 TLS session check (I4's plant) |
  | D16, two `latest` redirects | the harness serves `releases/download/<tag>/` only; "--version of a release with other asset names" asserts the one request, by tag |
  | D17, Rosetta | "Rosetta is corrected to arm64" (I3's plant) |
  | D18, x64 PowerShell on Arm64 | `TestTemplatesAsWritten`: the registry, never `OSArchitecture` (D10's plant). A run on Arm64 is not verified |
  | D19, `iex` scope | the session check after every `iex` and scriptblock run (I4's plant) |
  | D20, Ctrl-C | `TestInstallShInterrupt` (I3's plant); `TestInstallPs1Interrupt` under PowerShell 7 (D5, I4's plant). 5.1 is not verified |
  | D21, loose `SHA256SUMS` parsing | the CR, 63-character, upper-case, duplicate and missing cases, both scripts (I3's plant) |
  | D22, archives, channels, products, attestation | the tar.gz, gz and zip cases; the channel cases; `--product`; D10's attestation cases |
* **V5.** The templates pass shellcheck 0.11.0, `dash -n` and
  PSScriptAnalyzer as written and as rendered (I2, I4, I5), in CI's lint
  step and in the rehearsal's staged-installer check.
* **V6.** The behaviour tests pass under `sh` (macOS), dash, bash,
  Ubuntu's busybox-static (D3's six cases skipped, saying why) and
  Alpine's BusyBox (none skipped, in CI's container job), and under
  Windows PowerShell 5.1 and PowerShell 7 as a file, through `iex` and
  through `[scriptblock]::Create`.
* **V7.** The live rehearsal passed every check of I7 step 3, and the
  repository is gone.
* **V8.** MADR's "Not verified" entries for Alpine and BusyBox (I3) and
  TLS 1.2 on 5.1 (I4) are restated; the Arm64 entry stays, with D18.
* **V9.** CI is green on `main`, at `a0a26b6` (run 37571411162) and the
  pin commit `18e0575`, and on `v1.10.0` (run 37577710330).

### Deviations

* **D1 (2026-10-06), I2: arguments the installers embed.**
  * **Found:** MADR §3 says no rendered value can contain a quote, "each
    is checked against the spec's rules". Hook arguments and
    `identity_args` may be any non-empty string without NUL, so a quote,
    a space or a `$` could reach the shell and PowerShell code.
  * **Decision (the owner):** restrict the character set. With
    `installer` present, `releasespec` requires every hook argument and
    every product's `identity_args` to match `^[A-Za-z0-9._:=/,+@%-]+$`,
    at `Parse`. MADR §3 is amended.
  * **Files:** `selfupdate/releasespec/installer.go` and
    `installer_test.go` join I2.
* **D2 (2026-10-06), I3: two template defects the tests found.**
  * **Found:** `install.sh`'s `fetch_verified` downloaded the asset
    before reading its line in `SHA256SUMS`. For a release without the
    asset GitHub answers 404, so `--version` of a release with other
    asset names exited 2 with "download of … failed", not 1 with that
    release's installer URL as MADR §4 requires. `install.ps1` had the
    same order. And `set_base` accepted `http://localhost:@example.invalid`
    as a loopback URL; curl reads `localhost:` as a user name and went to
    the other host over plain HTTP. `install.ps1` anchors a numeric port,
    and was not affected.
  * **Decision (the owner):** fix both scripts in I3. Each reads the
    asset's line first and downloads only a listed asset; `install.sh`
    accepts only digits after the loopback colon. The MADR's contract is
    unchanged; the scripts now meet it.
  * **Files:** `installer/install.sh` and `installer/install.ps1` join
    I3. I4's tests cover the PowerShell change.
* **D3 (2026-10-06), I3: a BusyBox that runs its applets first.**
  * **Found:** on the Linux test host, `busybox` is Ubuntu's
    `busybox-static`, built with `FEATURE_SH_STANDALONE`: its ash runs its
    own `uname`, `id`, `wget` and `sha256sum` whatever `PATH` holds (a
    `uname` stub ahead of `PATH` still printed `Linux`). Six cases fake
    their conditions with stubs: the unsupported platform, `aarch64`,
    Rosetta, root, and the fetcher and hash-tool tests. Alpine builds
    BusyBox without that option ("`# CONFIG_FEATURE_SH_STANDALONE is not
    set`" in aports' `main/busybox/busyboxconfig`).
  * **Decision (the owner):** detect it and skip with the reason. The
    harness runs a `uname` stub under each shell; where the stub does not
    run, those six cases skip, saying why, and the rest run. The Alpine
    job runs all of them, and `SELFUPDATE_INSTALL_REQUIRE_SHELLS` fails it
    if they cannot.
  * **Files:** none beyond I3's.
* **D4 (2026-10-06), I4: four `install.ps1` defects the tests found.**
  * **Found,** with the template as committed in I3:
    1. a hook's output became part of `Invoke-InstallHook`'s result, an
       array, which is true however the hook ended: a failing
       `before_install` hook that printed anything let the install go
       on, and a failing `after_install` hook exited 0, not 3;
    2. an identity failure renamed the rejected binary to
       `relay.exe.bad-<guid>` and left it in the folder;
    3. `-Uninstall -Product relayctl` took the folder off PATH while
       `relay.exe` stayed;
    4. options were bound by `& { … } @args` itself: as a file, `-Bogus`
       exited 0; under the scriptblock form it was a bare binding error;
       and `@args` passes `-Uninstall:$false` on as `-Uninstall` and a
       stray `False`, which became `-InstallDir`:
       `-Uninstall:$false -Version v1.2.3` printed "removed
       False\relay.exe".
  * **Decision (the owner):** fix as tested. Hook output is written to
    the console, not returned; a rejected binary is deleted, and set
    aside only if it cannot be; the PATH entry goes with the last
    program; the outer scriptblock takes no parameters and runs the
    `[CmdletBinding(PositionalBinding = $false)]` block inside a `try`,
    so a wrong option is a usage error and a switch given a value is
    refused. MADR §4 is amended.
  * **Then found,** in the final run on the Windows test host: two of six
    identity-mismatch runs under PowerShell 7 still left
    `relay.exe.bad-<guid>`. The delete had met a handle still closing,
    from the identity check's process or a scanner, and the script set
    the file aside as for a running one. `Clear-SettledFile` now retries the
    delete for up to 2 s before a file is set aside, for the rejected
    binary and for an old `.prev` alike. Three passes of
    `TestInstallPs1` then ran 480 runs without a failure. This keeps the
    owner's decision; "cannot be deleted" now means after that wait.
    Named `Remove-Settled` first, it drew PSScriptAnalyzer's
    `PSUseShouldProcessForStateChangingFunctions`; it is
    `Clear-SettledFile`, a verb the rule leaves alone, as
    `Clear-UserPathEntry` is.
  * **Files:** `installer/install.ps1`, and `render_test.go`, whose
    expected PowerShell lines are one level deeper.
* **D5 (2026-10-06), I4: Ctrl-C on Windows.**
  * **Found:** Ctrl+Break sent to a new process group stops PowerShell 7,
    which runs its `finally` blocks; Windows PowerShell 5.1 ignored it
    for 20 s. A real Ctrl-C (`CTRL_C_EVENT` to the console, from a sender
    in a new console that ignored it itself) stopped neither within 20 s.
  * **Decision (the owner):** a Ctrl+Break case under PowerShell 7 only.
    How 5.1 cleans up after Ctrl-C is not verified.
  * **Files:** none beyond I4's.
* **D6 (2026-10-06), I4: `-UseBasicParsing` cannot fail a run.**
  * **Found:** the plant removing it passed. On the Windows test host
    (5.1.26100.9444, September 2026 updates), non-interactively,
    `Invoke-WebRequest` without the flag failed with "Windows PowerShell
    is in NonInteractive mode. Read and Prompt functionality is not
    available." for HTML, plain-text and octet-stream responses kept in
    memory, and downloaded all three without a prompt given `-OutFile`,
    which the template always uses. `Invoke-RestMethod`, as in
    `irm … | iex`, ran under 5.1 in every iex case.
  * **Decision (the owner):** a static check. `TestTemplatesAsWritten`
    requires `-UseBasicParsing` on every line that calls
    `Invoke-WebRequest` or one of its aliases outside a comment. MADR's
    fact is qualified.
  * **Files:** `render_test.go`.
* **D7 (2026-10-07), after I6's push: CI's Windows leg.**
  * **Found:** CI runs 37565767526, 37567660285 and 37568546818 failed on
    `windows-2025` at `TestInstallPs1Interrupt`: "AllocConsole: Access is
    denied." The runner starts its steps with a console but no console
    window, so `GetConsoleWindow` returned 0 and `AllocConsole`, finding a
    console already there, failed. I4's record named this path as not yet
    run. Launched with `CREATE_NO_WINDOW` on the Windows test host, the
    committed test binary failed the same way.
  * **Decision (the owner):** fix as reproduced: `ensureConsole` calls
    `AllocConsole` and takes `ERROR_ACCESS_DENIED` as a console being
    there. Under the same launch the fixed binary passed:
    `--- PASS: TestInstallPs1Interrupt/pwsh`.
  * **Files:** `installer_ps_test.go`.
* **D8 (2026-10-07), after I6's push: a truncated tar.gz accepted.**
  * **Found:** run 37568546818 failed on `macos-15` at
    `TestCheck/a_truncated_gzip`: "check: <nil>". The client's tar.gz
    unpacker stops at tar's end and never reads the gzip trailer, as
    0012-MADR §4 decided ("a gzip CRC adds nothing" once `SHA256SUMS` has
    checked the archive). Whether the case's 12-byte cut reaches the
    program depends on how the fixture binary compresses, and the binary
    changes with every fixture commit. I5's `mark-installed` moved it: on
    this Mac the case failed 4 of 12 runs with fresh fixtures, against 12
    of 12 passes with the fixture before I5.
  * **What it matters for:** the installers unpack with system tools. A
    tar.gz cut in its trailer, with a wrong CRC, and with data after it:
    GNU tar 1.35 on the Linux test host refused all three (rc 2); macOS
    bsdtar 3.5.3 refused the cut one only; `gzip -t` refused all three on
    both. A release the client accepts could fail `install.sh`.
  * **Decision (the owner),** after a first answer of "harden the
    client's unpacker", which would have reversed 0012-MADR §4 and was
    withdrawn when that was put to the owner: the publish check reads
    every tar.gz and gz asset to its end with gzip, one member and
    nothing after it, before the client's unpacker runs; the client is
    unchanged. The first route's edit to `selfupdate/archive` was
    reverted before anything was staged. MADR §5 is amended.
  * **Evidence:** `TestCheck` gains six cases (no trailer, a wrong
    checksum, data after the member, a second member, a `.gz` without its
    trailer, a cut in half); "a truncated gzip" now fails the same way
    every time. Twelve runs with fresh fixtures: 12 passes. Plants: the
    check removed (six cases fail, two with the unpacker's own refusal),
    the trailing-data test removed, and `Multistream(true)`, each caught.
  * **Files:** `check.go`, `pack_test.go`; the building guide's table of
    failures, `docs/architecture.md`, the migration guide's §10 and the
    release notes.
* **D9 (2026-10-07), after I6's push: a launchd test that read the
  system.**
  * **Found:** run 37568546818 failed on `ubuntu-24.04` at
    `TestInsideByAncestry`: "an unrelated job: inside true". `Inside`
    asks the kernel for the job PID's process group
    (`syscall.Getpgid`), and the test's fake does not answer it: when a
    process of the same `go test` run held the fixture PID 4242, the
    "unrelated" job was in this process's group. The code is
    0011-MADR's (commit `d207bd5`, 2026-10-04); 0014 did not cause it.
    Made deterministic in a scratch copy, with the job's PID set to the
    test's parent, `go test`: "an unrelated job: inside true, <nil>".
  * **Decision (the owner):** fix it here, because `v1.10.0` needs green
    CI. The `Job` holds its group lookup, `processGroups` by default;
    the tests' fake answers it from its own table, never the system.
    `TestInsideByAncestry` gains the case it covered only by accident: a
    job in this process's group, and one in another. The deterministic
    repro passes with the fix; the group branch removed fails the new
    case: "a job in this process's group: inside false".
  * **Files:** `selfupdate/service/launchd/job.go`, `detach.go`,
    `fake_test.go`, `launchd_test.go`.
* **CI after the pushes (2026-10-07).** Run 37567660285 (I5) ran the
  steps I3 and I5 had not seen run: the Linux lint step, the script's
  test and "check-installers: clean" on both templates under the
  runner's PSScriptAnalyzer; `installer-containers`, with
  `TestInstallSh/sh_(busybox)` in Alpine and `sh_(dash)` and `bash` in
  Debian, each passing; and "Check the staged installers", "clean" for
  both sets. Its one failure, and run 37568546818's three, are D7 to D9.
* **D10 (2026-10-07), closing: V4's untested defects.**
  * **Found:** checking V4 before closing, each of MADR's D1–D22 against
    the tests, four had none: D22's attestation verification
    (`--verify-attestation` passed only in the live rehearsal; no test
    covered a missing `gh`, a logged-out one or a failing verification);
    D15's curl retries and timeout (`wget --https-only` and TLS 1.2 on
    5.1 are tested); D18, the native architecture on Arm64 Windows (no
    Arm64 host); and D12, comments describing versioned manifests
    (documentation).
  * **Decision (the owner):** add the tests, then close. A stub `gh`
    ahead of `PATH` tests `--verify-attestation` and `-VerifyAttestation`
    in both installers: each download verified with this release's
    repository and the publish workflow as signer; a failing
    verification (exit 2, nothing changed); `gh` logged out (exit 1); no
    `gh` (exit 1). `TestTemplatesAsWritten` pins curl's `--proto`,
    `--tlsv1.2`, `--fail`, `--retry 3` and `--connect-timeout 20`, and that
    `install.ps1` reads the architecture from the registry and never from
    `OSArchitecture` or `$env:PROCESSOR_ARCHITECTURE`. D18's run on Arm64
    and D12 stay recorded as not testable here.
  * **Files:** `installer_sh_test.go`, `installer_ps_test.go`,
    `render_test.go`.
  * **Evidence:** three cases in `TestInstallSh` and three in
    `TestInstallPs1` (file and scriptblock forms): each download verified,
    named with `--repo fixture/relay --signer-workflow
    maccavelli/go-selfupdate-lib/.github/workflows/publish-selfupdate-release.yml`;
    a failing verification, exit 2 and the earlier binary kept; logged
    out, exit 1; no `gh`, exit 1 with nothing downloaded. They pass on
    this Mac (sh, dash, bash), on the Linux test host ("pass=104 skip=6
    fail=0", busybox-static included, as `gh` is no applet) and on the
    Windows test host, the whole package "pass=266 fail=0 skip=0".
    Plants, each caught: attestation never verified, in each script (`gh
    calls "" lack …`); the login check removed, in each (`exit 0, want
    1`); the signer workflow dropped from `install.sh`'s call; curl
    without `--retry` (`the curl call lacks --retry 3`); the architecture
    from `OSArchitecture` (`install.ps1 reads the architecture from
    OSArchitecture`).
