---
status: accepted
date: 2026-10-06
decision-makers: go-selfupdate-lib maintainers (the owner)
consulted: the installers of rustup, uv (cargo-dist), GoReleaser (godownloader, run), golangci-lint, Homebrew, starship, tailscale, k3s, Deno, Bun, mise and fnm; the POSIX shell standard, ShellCheck, Debian Policy; Microsoft's PowerShell, .NET and Windows documentation; Apple's Rosetta and Gatekeeper documentation; GitHub's releases, immutable releases, rate-limit and attestation documentation; the GitHub-hosted runner image readmes; the installers and CI of every fleet repository
informed: magic-cli-remote, mcp-server-magictools, mcp-server-recall, mcp-server-socratic-thinker, mcp-server-duckduckgo, prepare-commit-msg
---
# Generate each program's `install.sh` and `install.ps1` from its release spec in the build workflow, from one tested template each

## Context and Problem Statement

[0004-MADR-evolve-selfupdate-api-and-tui-support.md](0004-MADR-evolve-selfupdate-api-and-tui-support.md)
§7 (Phase 4) lists:

> **Installer templates** (`install.sh`, `install.ps1` and their tests),
> parametrised by product, repository and default directory. They replace
> about 1,500 near-duplicate lines across four programs. The legacy
> versioned-manifest branch is dropped.

[0013-MADR-build-and-stage-release-workflow.md](0013-MADR-build-and-stage-release-workflow.md)
§10 left them out of `v1.9.0` and noted "They can read the spec later".
This record decides how. Research for it found four things the 0004
sentence does not settle:

1. **Three of the four programs' Windows installers cannot install their
   current releases** (D1 below), and a second defect sits behind the
   first (D2). Near-duplicates did not stay duplicates: fixes made in one
   copy never reached the others.
2. **The duplicated code is 2,804 lines, not 1,500,** and about 620 of
   them, magic-cli-remote's service management, are not generic at all.
   A template must leave a seam for product-specific steps rather than
   absorb them.
3. **The release spec already holds most of what a template needs:**
   products, platforms, packaging, prerelease channels and each product's
   identity command. The build workflow already knows the repository and
   the tag. A generated installer can be exact where the hand-written ones
   are approximate.
4. **No installer handles what the fleet now publishes:** archives,
   prerelease channels, or attestations.

### What the fleet does today

A read-only survey of every repository beside this one (2026-10-06).
Product installers exist in four repositories; no other program has one.

| Repository | `install.sh` | `install.ps1` | Tests | Published |
| :--- | :--- | :--- | :--- | :--- |
| magic-cli-remote | 1,118 lines, POSIX sh; about 290 generic, the rest service management for systemd, launchd, runit, s6 and OpenRC, and advisories | 371, PowerShell 5.1+; about 85% generic | a 980-line sh test; a PowerShell unit test (AST-loaded functions) and a fixture test with real `irm \| iex` runs, under 5.1 and 7, gating the release | release extra |
| mcp-server-magictools | 247 | 153 | sh test and shellcheck; PowerShell `-WhatIf` only | release extra |
| mcp-server-recall | 315 (adds `configure --encrypt-db`) | 193 | sh test and shellcheck; **no PowerShell test** | release extra |
| mcp-server-socratic-thinker | 254 (magictools, renamed) | 153 | as magictools | release extra; no one-liner in its README |

The eight scripts total 2,804 lines, and their tests 2,349 (`wc -l`,
2026-10-06). The MCP scripts are derived from magic-cli-remote's core;
their PowerShell scripts are its version from before its own fixes.

**What they share:** POSIX `sh` with `set -eu`; the body in `main`,
called on the last line; `curl -fsSL --proto '=https' --tlsv1.2`, falling
back to `wget`; `releases/latest/download/<asset>` or
`releases/download/v<pin>/<asset>`, never the API; `SHA256SUMS` checked
with `sha256sum`, `shasum -a 256` or `openssl dgst`; a temporary directory
in the install directory, renamed into place; `~/.local/bin` by default,
never `sudo`; PATH advice on Unix; `%LOCALAPPDATA%\Programs\…` on Windows.

**Defects** (V: verified in the source; I: inferred):

| ID | Finding |
| :--- | :--- |
| D1 (V) | magictools, recall and socratic `install.ps1` look up only the legacy versioned line `"$Product-windows-$Arch-"` (`install.ps1:65-66`, recall `:79-80`), but their releases list canonical names only. Every current release fails with "no checksum entry". magic-cli-remote fixed this (its MADR 0156 F7, `install.ps1:97-170`). |
| D2 (V) | The same three call `$PSCmdlet.ShouldProcess` unguarded (`:139`, recall `:162`). Under `irm \| iex` with strict mode that throws; magic-cli-remote measured and fixed it (its MADR 0156 D12). It surfaces once D1 is fixed. |
| D3 (V) | magictools `install.sh:108-109` sets the reported version to the asset name on a canonical manifest, and prints "mcp-server-magictools mcp-server-magictools-linux-amd64 installed". |
| D4 (V) | No `install.ps1` reads a version variable, so `irm \| iex` cannot pin a version. |
| D5 (V) | `v` is not stripped in two `install.ps1` (`-Version v1.2.3` builds `vv1.2.3`); three scripts do not validate the version. |
| D6 (V) | No `install.ps1` can uninstall. |
| D7 (V) | Backups and checks differ: magic-cli-remote `install.sh` keeps no `.prev`, the others do; two never run the installed binary, two do. |
| D8 (V) | Four Windows directory conventions: `Programs\magictools`, `Programs\socratic-thinker`, `Programs\mcp-server-recall`, and a folder per product whose `-InstallDir` names the parent. |
| D9 (V) | The MCP `install.ps1` PATH advice appends the merged `$env:Path`, copying Machine entries into the User PATH; magic-cli-remote fixed it (its MADR 0159 F20). |
| D10 (V) | Windows coverage is a `-WhatIf` that returns before any download, or nothing: how D1 and D2 shipped. |
| D11 (V) | magic-cli-remote CI never runs shellcheck on `install.sh`, which its own guide requires. |
| D12 (V) | Comments and docs still describe versioned manifests. |
| D14 (V) | Test seams (`MC_TEST_BASE_URL`, `ST_TEST_BASE_URL`, `-BaseUrl`) are honoured in production, accept a local path or `http://`. |
| D15 (V) | `wget` without `--https-only`; no `curl` retries or timeouts; no TLS 1.2 on PowerShell 5.1. |
| D16 (I) | The binary and `SHA256SUMS` come from two separate `latest` redirects. A release published between them fails the install, and an unpinned install reports "unknown". |
| D17 (I) | A Rosetta-translated shell on Apple silicon reports `x86_64`, and every `install.sh` rejects it as darwin/amd64. |
| D19 (I) | `irm \| iex` runs in the caller's scope: strict mode, `$ErrorActionPreference` and the helper functions stay set in the user's session. |
| D20 (I) | `trap cleanup EXIT INT TERM` without an `exit` in the handler: Ctrl-C does not stop the script. |
| D21 (V) | `SHA256SUMS` parsing is loose: first match wins, no 64-hex check, an unanchored legacy pattern. |
| D22 (V) | No installer supports archives, prerelease channels, product selection or attestation verification. |

D13, D23 and D24 (a README without its one-liner, uninstall leaving
configuration silently, a configure step run after a declined install)
are product-level and fall away with the template. D18 (x64 PowerShell on
Arm64 Windows installing the amd64 build) is addressed by §4's
architecture rule.

### What the platforms and tools require

From the research (sources under "More Information"; probes on macOS
26.6.2 arm64 and PowerShell 7.6.6):

* **A truncated download.** A flat script cut after its second step ran
  both steps and exited 0 under `dash` and `/bin/sh`; wrapped in a
  function called on the last line, it was a syntax error and ran
  nothing. Through `Invoke-Expression` the same: flat, both lines ran;
  wrapped, a parse error and nothing ran. The whole body must be a
  function, in both scripts.
* **HTTPS only.** `curl --proto '=https'` refuses plain HTTP
  (`curl: (1) Protocol "http" disabled`). `wget` needs `--https-only`.
* **POSIX `sh`.** `local` works in macOS `/bin/sh` (bash 3.2 in POSIX
  mode), dash, zsh and bash, but not ksh93 (`local: not found`, and the
  variable leaks). `mktemp` is not POSIX; BusyBox's needs a template
  ending in `XXXXXX`. `uname -m` is `aarch64` on Linux arm64 and `arm64` on
  macOS.
* **Rosetta.** Under `arch -x86_64`, `uname -m` is `x86_64` and
  `sysctl -n sysctl.proc_translated` is `1`; the sysctl does not exist on
  Intel Macs and exits 1.
* **Hash tools.** This Mac has `/sbin/sha256sum` (since an unverified
  macOS version), `shasum` (a Perl script; Apple has warned Perl may go)
  and LibreSSL `openssl`. BusyBox `sha256sum` supports only `-c`, `-s` and
  `-w`. An installer should find its line itself, hash with the first tool
  present, and fail if there is none.
* **Static Go binaries.** `CGO_ENABLED=0` Linux builds are statically
  linked and use Go's own resolver and `os/user`, so no musl or glibc
  detection is needed (not run on Alpine; see "Not verified").
* **macOS quarantine.** `curl` and `wget` set no `com.apple.quarantine`;
  the linker's ad-hoc signature runs from Terminal. A quarantined
  download is killed (`Killed: 9`) until `xattr -d`; macOS `tar` carries
  quarantine to extracted files. An installer that downloads with `curl`
  needs no `xattr`.
* **GitHub downloads.** `releases/latest/download/<asset>` is documented,
  redirects through `releases/download/<tag>/…`, and carried no rate-limit
  headers; the REST API allows 60 unauthenticated requests an hour. On an
  immutable release, assets and tags are locked but the latest flag can
  move, so an installer should resolve the tag once and fetch every file
  by it.
* **Attestations.** `gh attestation verify` and `gh release verify-asset`
  exit 4 without authentication. Verification can only be optional, and
  gated on `gh auth status`.
* **PowerShell 5.1.** Since the 2025-12-09 fix for CVE-2025-54100,
  `Invoke-WebRequest` prompts without `-UseBasicParsing` and automation
  can hang (KB5074596). TLS 1.2 is added to the protocol set on 5.1.
  `Get-FileHash` exists on 5.1. Before .NET 7, `OSArchitecture` reports
  X64 inside an emulated process on Arm64; the native architecture is in
  `HKLM\…\Session Manager\Environment` (Bun's approach).
* **PowerShell arguments and scope.** `iex` runs in the caller's scope.
  Arguments pass as `& ([scriptblock]::Create((irm url))) -Version x`, or
  through environment variables. The script is published without a BOM.
* **The user PATH.** Writing it through
  `[Environment]::SetEnvironmentVariable(…, 'User')` expands `%VAR%`
  entries; Bun and cargo-dist edit the raw registry value, keep
  `REG_EXPAND_SZ`, and broadcast `WM_SETTINGCHANGE`, as magic-cli-remote
  does. `%LOCALAPPDATA%\Programs` is `FOLDERID_UserProgramFiles`.
* **Peers.** cargo-dist generates one installer per release with the
  version and URLs baked in (its `.ps1` checks no checksum; its `.sh`
  skips the check without `sha256sum`). godownloader generated scripts
  from `.goreleaser.yml` and is archived. golangci-lint's script fails
  closed on a missing checksum. Homebrew refuses root and checks `sudo`
  non-interactively. Deno, Bun, starship and fnm verify nothing.
* **Runners.** ubuntu-24.04 has shellcheck 0.9.0 (this repository's CI
  pins 0.11.0), dash 0.5.12, Docker, pwsh 7.6.6, Pester 5.9.0 and
  PSScriptAnalyzer 1.25.0. windows-2025 has Windows PowerShell 5.1, pwsh
  7.6.6, and Pester 3.4.0 and 5.9.0. No image has BusyBox or bats-core;
  a container job does.

### What the library has

* **The release spec** (0013) has products with their packages and
  optional `identity_args`, platforms with formats, packaging, extras with
  a `{tag}` placeholder, and prerelease channels. `Parse` refuses unknown
  fields: a field added in a later library version is refused by an older
  one, loudly, at the program's startup and in its tests.
* **The build workflow** stages the release from the spec with the
  internal tool `selfupdate-release`, which already writes files, runs the
  publish verifier on them and lists the extras for publication. It knows
  `github.repository` and the tag.
* **The publish workflow** attests every staged file, the installers
  included.
* **Identity.** A product with `identity_args` prints
  `<tag> (release) <sha12>` as its first line, the line the workflow
  already checks on each platform's runner.

## Decision Drivers

* **One installer implementation per shell,** tested once, so a fix
  reaches every program.
* **No hand-maintained product data in the scripts.** Product names,
  platforms, formats and the repository come from the spec and the
  workflow, as the binaries' do.
* **Integrity first:** every download checked against `SHA256SUMS` from
  the same tag, failing closed; attestation verification where the user
  can do it.
* **Safe to pipe:** truncation-proof, HTTPS-only, no `sudo`, no state
  left in the caller's PowerShell session.
* **Product-specific steps stay in the product,** as subcommands the
  installer runs, not as shell in a shared template.
* **Tested where it breaks:** every shell the installers claim, PowerShell
  5.1 and 7, the `irm | iex` form, and a real release.
* **Backward compatibility for programs that do not opt in,** as in
  0013: no change unless the spec asks for installers.

## Considered Options

* **A. Two static template scripts in this repository, with a generated
  values block, rendered from the spec by the build workflow and staged
  as `install.sh` and `install.ps1`**
* **B. One generic script per shell that reads a published manifest at
  run time**
* **C. A generator command that writes the scripts into each repository**
* **D. A minimal bootstrap script, with the installation done by the
  program itself in Go**
* **E. Keep each repository's scripts, and fix their defects in place**
* **F. Package managers (Homebrew, Scoop, winget) instead of scripts**

## Decision Outcome

Chosen option: **"A"**, because:

* it is the only option with one tested implementation per shell *and*
  no product data maintained by hand;
* the values come from the same spec the binaries and the update path
  use;
* the result is an ordinary release asset, attested like the binaries;
* a program opts in with one spec field.

### 1. The pieces

| Piece | Kind | What it does |
| :--- | :--- | :--- |
| `installer` in the release spec | optional spec field | turns the installers on; names the install folder and the environment prefix; lists product hooks |
| `internal/cmd/selfupdate-release/installer/install.sh` | template, a valid POSIX `sh` script | the Unix installer, with a generated values block |
| `internal/cmd/selfupdate-release/installer/install.ps1` | template, a valid PowerShell script | the Windows installer, with a generated values block |
| the `stage` step | changed | renders both into the staged release when the spec asks for them |
| `selfupdate-release installer` | new subcommand | renders them to a directory, for a local look or a test |

### 2. The spec field

```json
"installer": {
  "name": "magic-cli-remote",
  "env_prefix": "MCREMOTE",
  "hooks": [
    {"when": "before_install", "product": "mcremote", "args": ["service", "stop", "--if-running"]},
    {"when": "after_install", "product": "mcremote", "args": ["setup-service", "--refresh"]}
  ]
}
```

* **Present, even as `{}`, turns the installers on.** Absent, nothing
  changes.
* **`name`**, optional, matches the product rule. It names the Windows
  folder, `%LOCALAPPDATA%\Programs\<name>`, and the default environment
  prefix. Empty means the repository's name, which the workflow supplies.
* **`env_prefix`**, optional, `^[A-Z][A-Z0-9_]{0,31}$`. Empty means `name`
  in upper case with every other character `_`. It lets a program keep
  the variables its users already set (`MCP_RECALL_*`).
* **`hooks`**, optional, at most 8. Each runs one product of the release
  with fixed arguments (`args`, at most 16, as `identity_args`):
  * `before_install` runs the *installed* copy, when there is one, before
    the swap: to stop a service holding the binary;
  * `after_install` runs the new copy after the swap and the identity
    check: to configure, or to set up a service.

  Hooks are skipped with `--no-hooks` or `<PREFIX>_NO_HOOKS=1`. A failing
  `before_install` hook stops the install before anything changes; a
  failing `after_install` hook leaves the new binary installed and exits 3.
* **Only release products.** A hook names a product the spec lists;
  nothing else can be run.
* **`install.sh` and `install.ps1` become reserved extra names** when the
  field is present: the spec refuses an extra that takes them.
  `ExtraNames` lists them, so the publish inputs include them. `install.sh`
  is generated only when a non-Windows platform is listed, and
  `install.ps1` only when a Windows one is.
* **Compatibility.** The field needs the library version that adds it.
  An older library's `Parse` refuses it as an unknown field, at the
  program's startup and in its tests, as 0013 §2 intends. The build
  workflow and the library are released together, and the guide pins a
  program's workflow calls and its `go.mod` to the same release.

### 3. The templates

Each template is a complete, valid script with sample values in one
block between marker lines:

```sh
# >>> selfupdate-release values
REPOSITORY='maccavelli/relay'
TAG='v1.2.3'
...
# <<< selfupdate-release values
```

The renderer replaces only that block, with values it has validated and
single-quoted. The rest of the script is static, so the template itself is
linted (shellcheck, `sh -n` under dash; PSScriptAnalyzer), and tested
against a fixture release, as written. No value can contain a quote: each
is checked against the spec's rules and the repository rule
`^[A-Za-z0-9-]+/[A-Za-z0-9._-]+$`.

The generated values: the repository; the release's own tag; the
prerelease channels; the products, their identity arguments and hooks;
each platform's asset name and format; the folder name and environment
prefix.

### 4. What the installers do

Both scripts share one contract.

* **The version.** By default the installer's own release: the tag baked
  in. `releases/latest/download/install.sh` therefore installs the latest
  release, and `releases/download/v1.2.3/install.sh` installs `v1.2.3`.
  `--version` (`-Version`), or `<PREFIX>_VERSION`, installs another tag,
  with or without its `v`, admitted only by the tag rule and the release's
  channels. There is no API call, no `latest` lookup at run time, and so
  no race (D16) and no rate limit.
  * An installer carries its own release's asset names. With
    `--version`, it fetches that release's `SHA256SUMS` and installs only
    if the names it expects are there; otherwise it fails closed (exit 1)
    and prints that release's own installer URL,
    `…/releases/download/<tag>/install.sh`.
* **Every file by tag:** `https://github.com/<repository>/releases/download/<tag>/<name>`.
  `curl --proto '=https' --tlsv1.2 --fail --retry 3 --connect-timeout 20`,
  or `wget --https-only`; on Windows `Invoke-WebRequest -UseBasicParsing`,
  TLS 1.2 added on 5.1.
* **The platform:** `uname -s`/`-m`, with Rosetta corrected to arm64
  (`sysctl.proc_translated`), and `aarch64` read as `arm64`; on Windows
  the native architecture from the registry. A platform the release does
  not list fails with the list it does.
* **Integrity, failing closed (exit 2):**
  * `SHA256SUMS` from the same tag, parsed strictly: exactly one line
    whose second field is the asset's name, 64 lower-case hex characters,
    two spaces; a trailing CR refused, as the build never writes one;
  * the hash from `sha256sum`, `shasum -a 256` or `openssl dgst -sha256`,
    and a failure if none exists; `Get-FileHash` on Windows;
  * with `--verify-attestation`, `gh attestation verify --repo
    <repository> --signer-workflow <publish workflow>` on the asset; a
    missing or unauthenticated `gh` is then a failure, not a skip.
* **Archives:** the program, and nothing else, extracted from tar.gz
  (`tar`), zip (`unzip`, or Windows' `tar.exe`, never Git Bash's) or gz
  (`gzip -dc`).
* **Placement:** `~/.local/bin` (`--dir`, `<PREFIX>_INSTALL_DIR`), never
  `sudo`, refused as root unless `--allow-root`; on Windows
  `%LOCALAPPDATA%\Programs\<name>`. Staged in a temporary file in the
  install directory, mode 0755, the old binary renamed to `<name>.prev`,
  then renamed into place; on Windows the running `.exe` is renamed, as
  `selfupdate` does.
* **The identity check:** for a product with `identity_args`, the new
  binary must print `<tag> (release)` first. If it does not, the previous
  binary is restored and the installer exits 2.
* **PATH:** on Unix, advice only. On Windows, the user PATH is updated by
  default (§9, Q1), editing the raw registry value, keeping
  `REG_EXPAND_SZ`, checking User and Machine first, and broadcasting
  `WM_SETTINGCHANGE`; `--no-path-update` or `<PREFIX>_NO_PATH_UPDATE=1`
  prints advice instead.
* **Products:** every product by default; `--product` (repeatable)
  installs a subset.
* **`--uninstall`:** removes the products' binaries and `.prev` files,
  and, on Windows, the folder's PATH entry; says that configuration is
  left.
* **`--dry-run`:** prints what it would download and where it would
  install, and changes nothing.
* **Exit codes:** 0 installed; 1 a usage or environment error; 2 a
  verification failure, with nothing changed or the previous binary
  restored; 3 installed, but an `after_install` hook failed.
* **Script hygiene:**
  * the whole body in a function called on the last line, in both
    scripts;
  * `install.sh` in POSIX `sh` without `local`, `set -eu`, `umask 022`,
    a `trap` that cleans up and exits on INT and TERM;
  * `install.ps1` in one scriptblock with `[CmdletBinding()]`, run as
    `& { … }` from the last line so nothing stays in the caller's scope,
    strict mode set inside, PowerShell 5.1 and 7, no BOM, a guarded
    `$PSCmdlet`. The one process-wide change is TLS 1.2 added to the
    protocol set on 5.1, which the script says;
  * arguments through flags or environment variables; for PowerShell
    also `& ([scriptblock]::Create((irm <url>))) -Version vX.Y.Z`.
    `install.ps1` takes the same options as parameters: `-Version`,
    `-InstallDir`, `-Product`, `-NoPathUpdate`, `-NoHooks`,
    `-VerifyAttestation`, `-Uninstall` and `-DryRun`. `--allow-root` has no
    Windows counterpart.
* **The test seams:** `SELFUPDATE_INSTALL_BASE_URL` replaces
  `https://github.com` only with another `https://` origin or a loopback
  `http://127.0.0.1:<port>` or `http://localhost:<port>`. It is the same
  in every program, and it cannot downgrade a real download to plain
  HTTP. On Windows, a second test variable redirects PATH updates to a
  scratch registry key, honoured only while the base URL is a loopback
  one, so a test never edits the real user PATH.

### 5. What is checked, and where

* **The templates,** as written: shellcheck 0.11.0, `sh -n` under dash,
  and PSScriptAnalyzer.
* **The renderer:** Go tests for the block replacement, quoting, refused
  values, and the files a spec produces.
* **The behaviour:** Go tests in the tool's package that render the
  templates for the fixture, serve a staged fixture release from a
  loopback `httptest` server, and run:
  * `install.sh` under every shell present: `sh`, `dash`, `bash`, and
    BusyBox `ash` in a container job;
  * `install.ps1` under Windows PowerShell 5.1 and PowerShell 7, as a
    file, with `irm | iex`, and through `[scriptblock]::Create`.

  Each run asserts the installed bytes, the exit code and what changed on
  disk: checksum and identity failures change nothing or restore the
  previous binary; archives; hooks; uninstall; PATH updates against a
  scratch key.
* **CI** runs them on all three OSes, and the rehearsal stages the
  installers with the fixture.
* **A live rehearsal** runs the real one-liners against a real release on
  the three test hosts.

### 6. Product-specific steps

magic-cli-remote's service management and recall's `configure` stay in
those programs, as subcommands their hooks call. The templates carry no
service or product code. A program whose current installer does more
moves that logic into Go first. `selfupdate/service` already holds the
lifecycles its self-update uses; the installer then calls the same code.

### 7. What stays out

* **Package managers** (Homebrew, Scoop, winget). They are complementary,
  and each needs its own record.
* **Signing or notarizing the installers** themselves. They are attested
  by the publish workflow like every other asset, and the one-liners
  trust TLS to GitHub for the script, as every peer does.
* **System-wide installs and `sudo`.**
* **Shell completion, MCP client registration and service setup** in the
  templates; a hook can run a product's own command for each.
* **Moving each program onto the installers.** Each repository does it
  under its own records; the building guide describes it.

### 8. Pieces of the 0013 machinery reused

`releasespec` validates the field; `stage` renders and stages;
`ExtraNames` lists the installers, so `plan`, the publish verifier and the
publish workflow need no change; the identity arguments are the ones the
identity run uses.

### 9. Questions for the owner

* **Q1. Does `install.ps1` update the user PATH by default?**
  * Recommendation: yes, with `--no-path-update` to opt out, as
    magic-cli-remote does today. Editing a Windows PATH by hand is a
    registry or settings task most users will not do.
  * `install.sh` only prints advice: editing shell startup files is
    invasive, and `~/.local/bin` is on PATH on many systems already.
* **Q2. Is attestation verification opt-in?**
  * Recommendation: opt-in, `--verify-attestation`, which then fails
    without an authenticated `gh`. `gh` exits 4 unauthenticated, so an
    automatic check would either be skipped silently for most users or
    fail installs that `SHA256SUMS` already protects.
* **Q3. Do hooks exist, and in both directions?**
  * Recommendation: yes, `before_install` and `after_install`, run only
    on release products. Without `before_install`, magic-cli-remote cannot
    stop its service before the swap (its `install.sh:1097-1103` does it
    to avoid `ETXTBSY` on Darwin); without `after_install`, recall cannot
    configure. Without hooks those programs keep their own scripts.
* **Q4. Does an installer install its own release by default?**
  * Recommendation: yes. The `latest` URL then installs the latest
    release, a pinned URL installs that release, and no run-time lookup
    can race or hit a rate limit. The alternative, resolving `latest`
    when the script runs, makes a pinned installer URL install something
    else.

### 10. Owner answers (2026-10-06)

The owner answered:

> 1-4, follow recommendations.

* **Q1:** `install.ps1` updates the user PATH by default, with
  `--no-path-update` (`-NoPathUpdate`) and `<PREFIX>_NO_PATH_UPDATE=1` to
  opt out; `install.sh` prints advice only.
* **Q2:** attestation verification is opt-in, `--verify-attestation`
  (`-VerifyAttestation`), and then fails without an authenticated `gh`.
* **Q3:** hooks exist in both directions, `before_install` and
  `after_install`, on release products only.
* **Q4:** an installer installs its own release by default.

No section changes: §2 and §4 already state the recommendations.

### Consequences

* Good, because D1–D22 are fixed once, for every program that opts in,
  and stay fixed: one implementation per shell, tested on three OSes,
  five shells and two PowerShells.
* Good, because the installers know exactly what the release holds: the
  products, platforms, formats and channels come from the spec.
* Good, because the installer is an attested release asset, version-
  locked to its release, with no run-time API call.
* Good, because product-specific code moves into the products, where the
  self-update path can share it.
* Neutral, because the spec gains a field, so the release is `v1.10.0`,
  compatible with `v1.9.0` (`make apicheck`), and programs must move to it
  to use the field.
* Bad, because programs with custom installer logic must move it into Go
  subcommands before adopting the templates: magic-cli-remote's about
  620 lines of service management first.
* Bad, because environment variable names change for programs that do
  not set `env_prefix` to their old prefix; the field exists so they need
  not.
* Bad, because the repository takes on PowerShell and shell test
  infrastructure: a loopback release server, a BusyBox container job and
  two PowerShell versions in CI.

### Confirmation

* The templates pass shellcheck, `dash -n` and PSScriptAnalyzer as
  written, and a planted defect in each fails them.
* The behaviour tests pass on macOS, Linux (dash, bash, BusyBox) and
  Windows (5.1, 7, `irm | iex`), each seen to fail on a planted break:
  a skipped checksum, an accepted CR, a missed Rosetta correction, a
  leaked variable after `iex`, an unrestored binary after an identity
  failure.
* The CI rehearsal stages both installers for the fixture, and they pass
  the publish verifier.
* A live rehearsal, in a throwaway repository with immutable releases,
  runs `curl … | sh` and `irm … | iex` on the three test hosts, with a
  pinned and a latest URL, a raw and an archive release, and a hook.

## Pros and Cons of the Options

### A. Static templates with a generated values block, rendered by the build workflow

* Good, because the templates are real scripts: linted and tested as
  written, not only after rendering.
* Good, because the values are validated data, not text substitution in
  code.
* Good, because the installer is pinned to its release: no race, no API.
* Neutral, because each release carries its own installer; an installer
  fix reaches users with the program's next release.
* Bad, because the spec gains a field and the library a minor version.

### B. A generic script that reads a published manifest at run time

* Good, because one script would serve every program unchanged.
* Bad, because POSIX `sh` has no JSON parser; the manifest would need a
  second, line-based format, or `jq`, which is often absent.
* Bad, because it needs one more download and one more trust step before
  anything is checked.

### C. A generator that writes the scripts into each repository

* Good, because each repository can read its installer in review.
* Bad, because copies return, and so does drift: a fix needs a
  regeneration in every repository, the failure mode that left D1 in
  three of four.

### D. A bootstrap script, with installation done by the program

* Good, because PATH, hooks and placement would be Go, tested once.
* Bad, because the bootstrap still needs platform detection, download and
  checksum verification: the larger part of the script.
* Bad, because every program would have to add and keep an install
  subcommand, and a broken one cannot be reinstalled by itself.

### E. Fix each repository's scripts in place

* Good, because it needs no change here.
* Bad, because it is how the fleet got here: four copies, fixes that did
  not travel, and 2,804 lines of duplicated scripts and 2,349 of tests.

### F. Package managers

* Good, because they are what many users prefer.
* Bad, because each is its own channel with its own review and
  credentials, and none covers every platform the fleet ships.
  Complementary, out of scope (§7).

## More Information

### Probe evidence

On macOS 26.6.2 arm64 (`/bin/sh` is bash 3.2.57), dash and PowerShell
7.6.6, from the research:

* A script truncated after its second step: flat, `step1 ran` and
  `step2 ran`, exit 0; wrapped in a function called on the last line, a
  syntax error, exit 2, nothing run. Through `Invoke-Expression`: both
  lines ran flat, a parse error and nothing run wrapped.
* `curl --proto '=https' http://…`: `curl: (1) Protocol "http" disabled`,
  exit 1.
* `local x` in a function: works in `/bin/sh`, dash, zsh, bash; ksh93u+
  prints `local: not found` and the variable leaks.
* `arch -x86_64 /bin/sh -c 'uname -m; sysctl -n sysctl.proc_translated'`:
  `x86_64`, `1`.
* `command -v sha256sum shasum openssl`: `/sbin/sha256sum`,
  `/usr/bin/shasum`, `/usr/bin/openssl` (LibreSSL 3.3.6).
* Files fetched with `curl` and `wget`: no `com.apple.quarantine`. A Go
  arm64 binary with a Safari-style quarantine flag: `Killed: 9`; after
  `xattr -d com.apple.quarantine`, it runs. With its signature removed:
  `Killed: 9`.
* `gh attestation verify` and `gh release verify-asset` without
  authentication: exit 4.

### Sources

* curl | sh safety: <https://sandstorm.io/news/2015-09-24-is-curl-bash-insecure-pgp-verified-install>
* rustup: <https://rust-lang.github.io/rustup/installation/other.html>
* uv and cargo-dist: <https://docs.astral.sh/uv/getting-started/installation/>,
  <https://axodotdev.github.io/cargo-dist/book/installers/shell.html>
* godownloader: <https://github.com/goreleaser/godownloader>
* golangci-lint: <https://golangci-lint.run/docs/welcome/install/local/>
* gh CLI: <https://github.com/cli/cli/blob/trunk/docs/install_linux.md>
* POSIX shell: <https://pubs.opengroup.org/onlinepubs/9799919799/utilities/V3_chap02.html>
* Debian Policy, scripts: <https://www.debian.org/doc/debian-policy/ch-files.html#scripts>
* ShellCheck SC3043: <https://www.shellcheck.net/wiki/SC3043>
* Rosetta: <https://developer.apple.com/documentation/apple-silicon/about-the-rosetta-translation-environment>
* `Invoke-WebRequest` on 5.1: <https://learn.microsoft.com/en-us/powershell/module/microsoft.powershell.utility/invoke-webrequest?view=powershell-5.1>;
  KB5074596: <https://support.microsoft.com/KB/5074596>
* `OSArchitecture` under emulation: <https://learn.microsoft.com/en-us/dotnet/core/compatibility/interop/7.0/osarchitecture-emulation>
* `SetEnvironmentVariable`: <https://learn.microsoft.com/en-us/dotnet/api/system.environment.setenvironmentvariable>
* Execution policies: <https://learn.microsoft.com/en-us/powershell/module/microsoft.powershell.core/about/about_execution_policies>
* Known folders: <https://learn.microsoft.com/en-us/windows/win32/shell/knownfolderid>
* Arm64 emulation: <https://learn.microsoft.com/en-us/windows/arm/apps-on-arm-x86-emulation>
* Release links: <https://docs.github.com/en/repositories/releasing-projects-on-github/linking-to-releases>
* Latest release: <https://docs.github.com/en/rest/releases/releases#get-the-latest-release>
* Rate limits: <https://docs.github.com/en/rest/using-the-rest-api/rate-limits-for-the-rest-api>
* Immutable releases: <https://docs.github.com/en/code-security/supply-chain-security/understanding-your-software-supply-chain/immutable-releases>
* Asset digests: <https://github.blog/changelog/2025-06-03-releases-now-expose-digests-for-release-assets/>
* `gh attestation verify`: <https://cli.github.com/manual/gh_attestation_verify>
* Runner images: <https://github.com/actions/runner-images>

### Not verified

* A `CGO_ENABLED=0` binary run on Alpine or under BusyBox. The PLAN's
  container job runs one.
* BusyBox `ash` and `local`; the templates do not use `local`.
* Which macOS version added `/sbin/sha256sum`; the templates fall back to
  `shasum` and `openssl`.
* Whether Windows PowerShell 5.1 negotiates TLS 1.2 by default; the
  template adds it.
* `OSArchitecture` under 5.1 on Windows 11; the template reads the
  registry.
* How GitHub rate-limits the web `releases/download` URLs; undocumented.
* Whether a Terminal "Developer Tools" exemption affected the Gatekeeper
  probes.

### Related

* [0004-MADR-evolve-selfupdate-api-and-tui-support.md](0004-MADR-evolve-selfupdate-api-and-tui-support.md):
  §7 Phase 4.
* [0013-MADR-build-and-stage-release-workflow.md](0013-MADR-build-and-stage-release-workflow.md):
  the spec, the build workflow and the internal tool this record extends.
* [0011-MADR-reference-service-lifecycles.md](0011-MADR-reference-service-lifecycles.md):
  the service lifecycles a hook can call into.
* [0014-PLAN-shared-installer-templates.md](0014-PLAN-shared-installer-templates.md).
* magic-cli-remote: `scripts/install.sh`, `scripts/install.ps1` and their
  tests; its records 0156 (F7, D12) and 0159 (F20).
