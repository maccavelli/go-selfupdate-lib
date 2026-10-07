---
status: in-progress
date: 2026-10-06
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
