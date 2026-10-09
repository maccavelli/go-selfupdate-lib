# Building releases

How a program builds, checks, stages and publishes its self-update release
with this repository's two reusable workflows, from one release spec that
the program also embeds, and gives its users installers generated from the
same spec. Why it works as it does is in
[0013-MADR](../decisions/0013-MADR-build-and-stage-release-workflow.md) and
[0014-MADR](../decisions/0014-MADR-shared-installer-templates.md).

The pieces:

- **`selfupdate-release.json`,** the release spec, in your repository:
  products, platforms, packaging, extra assets, prerelease channels.
- **`selfupdate/releasespec`,** which your program uses to read the spec
  it embeds, and which the workflow uses to read the same file.
- **`build-selfupdate-release.yml`,** which builds every product for every
  platform with a fixed recipe, checks each binary, packs, writes
  `SHA256SUMS`, and uploads the staged set. Off a tag it rehearses.
- **`publish-selfupdate-release.yml`,** unchanged in what it takes, which
  validates, publishes, attests and waits for the release to be immutable.
- **`install.sh` and `install.ps1`,** which the build workflow renders into
  the release from this repository's templates when the spec asks (step
  12).

Both workflows are pinned to the same commit of this repository; the
examples below pin `v1.12.0`'s.

## 1. Write the spec

Put `selfupdate-release.json` in the directory of the Go package that
configures your updater. `go:embed` cannot reach a parent directory, so it
cannot live at the repository root unless that package does.

```json
{
  "schema": 1,
  "products": [
    {"name": "relay", "package": "./cmd/relay", "identity_args": ["version"]}
  ],
  "platforms": [
    {"os": "linux", "arch": "amd64"},
    {"os": "linux", "arch": "arm64"},
    {"os": "darwin", "arch": "arm64"},
    {"os": "windows", "arch": "amd64"}
  ],
  "packaging": "binary",
  "extras": [
    {"name": "relay-{tag}.spdx.json", "path": "dist/sbom.spdx.json"}
  ],
  "prerelease_channels": ["rc", "beta"],
  "installer": {}
}
```

- **`products`:** each `name` is the asset prefix and the product your
  updater asks for. `package` is the main package, relative to the module.
  `tags` adds build tags. `identity_args` is optional; see step 3.
- **`platforms`:** GOOS and GOARCH pairs. Each must be a target of your
  Go toolchain.
- **`packaging`:** `binary` (the default) ships
  `<product>-<os>-<arch>[.exe]`; `archive` ships archives (step 7). Under
  `archive`, each archive's name must be at most 128 characters, and a
  tar.gz program's name at most 100.
- **`extras`:** further assets, such as an SBOM. `{tag}` in a name is
  replaced by the release tag. With `path`, the file comes from your
  repository; without it, from an artifact you upload (step 6).
- **`prerelease_channels`:** the channels the publish workflow may release
  `vX.Y.Z-NAME.N` tags for, most stable first; see
  [Offer a beta channel](extending-selfupdate.md#offer-a-beta-channel).
- **`installer`:** present, even empty, it adds `install.sh` and
  `install.ps1` to every release; see step 12.

Unknown fields, a misspelled key, a duplicate key and `null` are errors,
so a typo fails loudly: leave a field out rather than set it to `null`. `releasespec.Parse`'s errors name the field, such as
`releasespec: products[0].name: "-relay" must match …`.

## 2. Embed it, and configure the updater from it

```go
package updateclient

import (
    _ "embed"

    "github.com/maccavelli/go-selfupdate-lib/selfupdate"
    "github.com/maccavelli/go-selfupdate-lib/selfupdate/releasespec"
)

//go:embed selfupdate-release.json
var releaseSpec []byte

const product = "relay"

// releaseConfig is the part of the updater's Config the spec decides.
func releaseConfig() (selfupdate.Config, error) {
    spec, err := releasespec.Parse(releaseSpec)
    if err != nil {
        return selfupdate.Config{}, err
    }
    // Fails at startup, and in your tests, if the product name and the
    // assets the workflow publishes ever disagree.
    if _, err := spec.Product(product); err != nil {
        return selfupdate.Config{}, err
    }
    assets, err := spec.AssetSelector()
    if err != nil {
        return selfupdate.Config{}, err
    }
    unpacker, err := spec.Unpacker()
    if err != nil {
        return selfupdate.Config{}, err
    }
    return selfupdate.Config{Assets: assets, Unpacker: unpacker}, nil
}
```

Fill in the rest of the `Config` (source, version policy, installer,
reporter, confirmer) as before. `AssetSelector` and `Unpacker` always
match: an exact selector and no unpacker for `binary`, the archive pair for
`archive`. A unit test that calls `releaseConfig` catches a spec your
program cannot use before any release does.

The spec also replaces the platform list your Makefile, CI and installers
each kept. `spec.Targets()` returns it.

## 3. Add an identity command (recommended)

The workflow sets the release stamp itself, but it cannot tell from the
binary alone that your program *reports* it: the linker silently drops a
stamp for a variable the program does not link, and a program can print
its own variables instead. The identity run settles it. Give the product a
command whose first line of output is `buildinfo.Identity()`:

```go
case "version":
    fmt.Println(buildinfo.Identity()) // "v1.2.3 (release) 0123456789ab"
```

and name it in the spec: `"identity_args": ["version"]`. The workflow runs
the staged program on each platform that has a GitHub-hosted runner
(linux/amd64, linux/arm64, darwin/arm64, windows/amd64 and windows/arm64)
and requires exactly `<tag> (release)`, optionally followed by the commit.
A product without `identity_args` is built and checked without being run,
and the job summary says so. The installers run the same command after an
install, with no standard input.

## 4. Call the workflows

```yaml
on:
  push:
    branches: [main]
    tags: ['v*']
  pull_request:

permissions:
  contents: read

jobs:
  build:
    uses: maccavelli/go-selfupdate-lib/.github/workflows/build-selfupdate-release.yml@247a2b644b0594a16041e091e4769e43e4f6e639 # v1.12.0
    with:
      spec-path: internal/updateclient/selfupdate-release.json

  release:
    needs: build
    if: needs.build.outputs.rehearsal == 'false'
    permissions:
      contents: write
      id-token: write
      attestations: write
    uses: maccavelli/go-selfupdate-lib/.github/workflows/publish-selfupdate-release.yml@247a2b644b0594a16041e091e4769e43e4f6e639 # v1.12.0
    with:
      artifact-name: ${{ needs.build.outputs.artifact-name }}
      products-json: ${{ needs.build.outputs.products-json }}
      platforms-json: ${{ needs.build.outputs.platforms-json }}
      extra-assets-json: ${{ needs.build.outputs.extra-assets-json }}
      prerelease-channels-json: ${{ needs.build.outputs.prerelease-channels-json }}
```

- **Pin both to the full commit SHA of a release tag,** as the publish
  workflow has always required. Tags are annotated; resolve the commit with
  `git ls-remote https://github.com/maccavelli/go-selfupdate-lib 'refs/tags/v1.12.0^{}'`.
- **The build job needs only `contents: read`.** A called workflow cannot
  raise its token beyond what the calling workflow grants, so grant at
  least that.
- **`module-dir`** names the Go module when it is not the repository root.
  The Go version is that module's `go` line. Since `v1.11.0` the plan
  step also checks that module's requirement of this library: a spec
  that uses a field an older release refuses, such as `installer`
  (`v1.10.0`), needs a module that requires a release that reads it. A
  `replace` with a directory is not checked, and the job summary says so.
- **`artifact-name`** overrides the staged artifact's name. Set it when one
  run calls the build workflow twice.
- **One publish at a time:** the publish job runs in the concurrency group
  `go-selfupdate-lib-publish-<owner>/<repo>`, without cancelling one in
  progress, so two tags never race the latest flag. Do not give another
  job that group's name. GitHub keeps one waiting run per group: a third
  tag pushed while one publish runs and another waits cancels the waiting
  one, and that tag gets no release. Re-run its cancelled publish job
  from the Actions page. GitHub creates no tag events at all for a push
  of more than three tags, so push tags one at a time, or at most three
  together.

**Before the first release,** set up the repository:

- **Turn on immutable releases:** Settings → General → Releases → "Enable
  release immutability", or the organization's release policy. It applies
  only to releases published after it is on. Without it the publish job
  publishes the release, waits 120 s for GitHub to mark it immutable, and
  fails, leaving a live, mutable release that clients refuse and the
  installers would still install from. Delete it, turn the setting on, and
  re-run the jobs.
- **Attestations** need a public repository, or GitHub Enterprise Cloud.
- **Restrict who can create `v*` tags:** Settings → Rules → Rulesets → New
  tag ruleset, targeting `v*`, with creations restricted to the owner or
  the release managers. The publish workflow attests whatever a `v*` tag
  builds. A check of that attestation (`selfupdate/verify/ghattest` from
  `v1.12.0`, or the installers' `--verify-attestation`) stops a release
  made with a stolen token outside the workflow, but not a malicious tag
  pushed through it
  ([0017-MADR](../decisions/0017-MADR-verify-build-provenance-and-close-0015-open-items.md)
  item 1).

## 5. Rehearsals

On any ref that is not a tag (a branch push, a pull request), the build
workflow runs the same recipe and every check, stamped
`rehearsal-<commit> (local)` instead of as a release, and the `release` job
is skipped. The release recipe is then tried on every change, not first on
the tag. A rehearsal never produces a release-stamped binary.

## 6. Extras from another job

An extra without a `path` comes from an artifact of the same run. Upload it
in its own job, and name the artifact:

```yaml
  apk:
    runs-on: ubuntu-24.04
    steps:
      # … build magic-cli-remote-<tag>-arm64.apk …
      - uses: actions/upload-artifact@043fb46d1a93c77aae656e7c1c64a875d1fc6a0a # v7.0.1
        with:
          name: extras
          path: dist/extras/

  build:
    needs: apk
    uses: maccavelli/go-selfupdate-lib/.github/workflows/build-selfupdate-release.yml@247a2b644b0594a16041e091e4769e43e4f6e639 # v1.12.0
    with:
      spec-path: internal/updateclient/selfupdate-release.json
      extras-artifact-name: extras
```

The artifact must hold exactly the extras without a `path`, under their
names after `{tag}` is replaced, as regular files.

## 7. Archives

Set `"packaging": "archive"`. Each platform ships an archive holding the
one program at its top level: zip on Windows and tar.gz elsewhere, unless a
platform names its `format` (`tar.gz`, `zip` or `gz`). A release is all
archives or all binaries.

- The archives are packed with fixed metadata (mode 0755, owner 0, the
  commit's time), so the same tag packs the same bytes.
- The build workflow unpacks each with the library's own unpacker and
  compares the program byte for byte. The publish workflow unpacks each
  again before it creates the release, so a release the client would
  refuse is never published. Since `v1.10.1` it also runs the client's
  archive selector on each archive's name, and `releasespec.Parse`
  refuses a spec whose composed names the selector, or a tar.gz header,
  could not hold: an archive name over 128 characters, or a tar.gz
  program name over 100.
- Your program's `spec.AssetSelector()` and `spec.Unpacker()` already
  select and extract them; see
  [Ship an archive](extending-selfupdate.md#ship-an-archive) for what the
  unpacker refuses.

## 8. The build recipe

Every binary is built on one `ubuntu-24.04` runner by cross-compiling:

```sh
CGO_ENABLED=0 GOOS=<os> GOARCH=<arch> GOFLAGS=-mod=readonly GOTOOLCHAIN=local GOENV=off GOWORK=off \
  go build -trimpath -buildvcs=true [-tags <tags>] \
    -ldflags "-s -w -X …/buildinfo.version=<tag> -X …/buildinfo.kind=release" \
    -o <asset> <package>
```

- **The stamp is always `buildinfo`'s,** with the tag as written,
  including its `v`.
- **Nothing else is yours to set:** no extra `-ldflags`, no `GOFLAGS`, no
  `GOAMD64` or `GOARM64` level, no environment. Build tags are the one
  per-product choice. With cgo off, `netgo`, `osusergo` and
  `-extldflags -static` change nothing.
- **The Go cache is off,** so a release build restores nothing another
  ref wrote.
- **Each binary is then checked** from its own build information: the
  platform, the tags, cgo off, `-trimpath`, the commit, a clean tree, the
  toolchain, the module and package, and, for a module at the repository
  root, the tag as the main module's version.

## 9. When a check fails

| Message | What it means |
| :--- | :--- |
| `releasespec: <field>: …` | the spec breaks a rule; the field is named |
| `tag "…" is not an admitted release tag` | a tag that is not `vX.Y.Z`, or a prerelease on a channel the spec does not list |
| `… is not a target of this toolchain (go tool dist list)` | a platform your Go version cannot build |
| `product … does not import …/buildinfo; its release stamp would be lost` | the main package does not link `buildinfo`, so `-X` would set nothing |
| `vcs.modified is "true", want "false"` | something wrote into the checkout before the build |
| `vcs.revision is "…", want "…"` | the checkout is not the commit the workflow ran for |
| `the main module version is "…", want "<tag>"` | the checkout did not hold the tag at its commit |
| `… the client's unpacker refuses it` | a packed archive the client could not read; a bug to report |
| `… the gzip stream is not whole: …` | a tar.gz or gz asset that is cut, has a wrong checksum, or has data after it, which `gzip` and `tar` would refuse; a bug to report |
| `… printed "…" first, want "<tag> (release) …"` | the identity command does not print `buildinfo.Identity()` |
| `… did not finish within 30s` | the identity command waits for input or the network |
| `the extras directory holds …, which the spec does not list` | the extras artifact holds a file the spec does not name |
| `verify-selfupdate-release: file set mismatch …` | the staged set and the spec disagree; a bug to report |
| `releasespec: extras[…].name: "install.sh" is already an installer's name` | the spec has `installer` and still lists a hand-written installer; delete the extra |
| `releasespec: …args[…]: "…" must match …, as the installers embed it` | a hook argument or `identity_args` entry with a space, quote or `$`; with `installer`, both keep to `[A-Za-z0-9._:=/,+@%-]` |
| `releasespec: installer.env_prefix: the prefix made from …; set installer.env_prefix` | the repository's name makes no valid variable prefix, such as one that starts with a digit |
| `releasespec: "…": null is not allowed; leave the field out` | a field set to `null`, which the spec does not read as absent |
| `module … requires go-selfupdate-lib …; this spec's "installer" needs v1.10.0 or later …` | your program's `go.mod` requires a release that cannot parse the spec it embeds; move it to the version the workflow is pinned to |

## 10. Verify an attestation

The publish workflow attests every file it uploads, and is the attestation's
signer:

```sh
gh attestation verify relay-linux-amd64 --repo <owner>/<repo> \
  --signer-workflow maccavelli/go-selfupdate-lib/.github/workflows/publish-selfupdate-release.yml
```

## 11. Moving from a Makefile build

- **Stamps:** link `github.com/maccavelli/go-selfupdate-lib/buildinfo` and
  report `buildinfo.Identity()`. Drop your own `main.version`,
  `main.buildKind`, `RawVersion` and the like, and the `BUILD_KIND`
  variable: the workflow always stamps a tag build as a release. A commit
  and a build time are in `Identity()`'s `Revision` and `Time`.
- **Versions** carry their `v`: the stamp is the tag as written.
- **The platform list** moves into the spec, and your Go code reads it
  from there. Delete the copies in the Makefile, CI and verify scripts;
  the installers come from the spec too (step 12).
- **Delete** the CI steps that build, check cgo, write `SHA256SUMS`,
  re-verify and stage: the build workflow does each of them.
- **Keep** your test, lint and smoke jobs. The build job can `need` them.
- **An APK or other non-Go asset** stays in its own job, as in step 6.

## 12. Installers

Add `"installer": {}` to the spec, and every release carries `install.sh`,
when the spec lists a platform other than Windows, and `install.ps1`, when
it lists a Windows one. The build workflow renders them from this
repository's templates with your products, platforms, formats, channels,
repository and tag. They are extras: published and attested with the
release, and not listed in `SHA256SUMS`.

Your program's module must require the version the workflow is pinned
to, at least `v1.10.0`, and since `v1.11.0` the plan step refuses an
older one: an older `releasespec.Parse` refuses the spec's
`installer` field, and the shipped program cannot update itself. Move
`go.mod` and both pins together.

```json
"installer": {
  "name": "relay",
  "env_prefix": "RELAY",
  "hooks": [
    {"when": "after_install", "product": "relay", "args": ["configure", "--defaults"]}
  ]
}
```

- **`name`** names the Windows install folder,
  `%LOCALAPPDATA%\Programs\<name>`. The default is the repository's name.
- **`env_prefix`** prefixes the installers' variables, such as
  `RELAY_VERSION`. The default is `name` in upper case, with every other
  character as `_`.
- **`hooks`,** at most eight, run one of your products around the install
  (below).

### The one-liners

```sh
curl -fsSL https://github.com/<owner>/<repo>/releases/latest/download/install.sh | sh
curl -fsSL https://github.com/<owner>/<repo>/releases/download/v1.2.3/install.sh | sh -s -- --dir ~/bin
```

```powershell
irm https://github.com/<owner>/<repo>/releases/latest/download/install.ps1 | iex
& ([scriptblock]::Create((irm https://github.com/<owner>/<repo>/releases/latest/download/install.ps1))) -Version v1.2.3
```

Each installer installs its own release: the `latest/download` URL the
latest one, and a tag's URL that tag. It asks no API for "latest", so there
is no race and no rate limit.

### What they do

- **Download** every file of the release by its tag, over HTTPS only:
  `curl --proto '=https'`, or a `wget` that has `--https-only` (BusyBox's
  does not, and is refused); `Invoke-WebRequest` on Windows, with TLS 1.2
  added on Windows PowerShell 5.1.
- **Check** each file against the release's `SHA256SUMS`, strictly, with
  `sha256sum`, `shasum` or `openssl` (or `Get-FileHash`), before anything
  is installed. A host with none of them gets an error, not an unchecked
  install.
- **Pick the platform:** `uname`, with Rosetta read as arm64; on Windows,
  the machine's own architecture from the registry. A platform the release
  does not ship is an error that lists the ones it does.
- **Install** into `~/.local/bin`, never with `sudo`, and refuse to run as
  root without `--allow-root`; on Windows into
  `%LOCALAPPDATA%\Programs\<name>`. The previous binary stays as
  `<product>.prev` (`<product>.exe.prev`); a running `.exe` is renamed, not
  overwritten.
- **Check the identity** of each product with `identity_args`: it must
  print `<tag> (release)` first. If it does not, the previous binaries are
  put back.
- **PATH:** advice on Unix. On Windows the user PATH is updated, editing
  the registry value so `%VAR%` entries and its type are kept.

### Options

| `install.sh` | `install.ps1` | Variable | What it does |
| :--- | :--- | :--- | :--- |
| `--version TAG` | `-Version TAG` | `<PREFIX>_VERSION` | install another release, with or without its `v`: a stable tag, or a prerelease on a channel the spec lists. A release whose asset names differ, such as a raw release from an archive release's installer, is refused (exit 1) with its own installer's URL |
| `--dir DIR` | `-InstallDir DIR` | `<PREFIX>_INSTALL_DIR` | install there; a relative folder is made absolute first |
| `--product NAME` | `-Product NAME` | | install only that product; repeat it for more (a name given twice counts once) |
| `--verify-attestation` | `-VerifyAttestation` | | also verify each download's attestation with `gh` (below) |
| `--no-hooks` | `-NoHooks` | `<PREFIX>_NO_HOOKS=1` | skip the hooks |
| | `-NoPathUpdate` | `<PREFIX>_NO_PATH_UPDATE=1` | print PATH advice instead of updating it |
| `--allow-root` | | | allow running as root |
| `--dry-run` | `-DryRun` | | print what would be downloaded and where it would go, and change nothing |
| `--uninstall` | `-Uninstall` | | remove the binaries and `.prev` files, and on Windows the folder's PATH entry with the last program; configuration is left |

`irm … | iex` cannot pass options: use the variables, or the scriptblock
form. A switch takes no value: `-DryRun`, not `-DryRun:$true`, which is
refused.

### Hooks

- **`before_install`** runs the installed copy, when there is one, before
  the new binary takes its place: for example, to stop a service. If it
  fails, nothing is changed and the installer exits 1. A first install
  skips it.
- **`after_install`** runs the new copy after the swap and the identity
  check: for example, to write a default configuration. If it fails, the
  program stays installed and the installer exits 3.
- **Arguments** keep to `[A-Za-z0-9._:=/,+@%-]`, with no space, quote or
  `$`, because the installers embed them in shell and PowerShell code. With
  `installer` present, so do `identity_args`. A step that needs more
  belongs in a subcommand of your program.
- They run with no standard input, and only a regular file is run.
- Their output is shown; `--no-hooks` skips them.

### Exit codes

| Code | Meaning |
| :--- | :--- |
| 0 | installed (or removed, or a dry run) |
| 1 | a usage or environment error: an unknown option, an empty or unsafe `--product`, an unsupported platform, root, a missing tool, a refused version, a directory where a program or its `.prev` goes, a failed `before_install` hook |
| 2 | a verification failure: a download, `SHA256SUMS`, a checksum, an attestation or the identity; nothing was changed, or the previous binaries were put back |
| 3 | installed, but an `after_install` hook failed |

Through `iex` or the scriptblock form, `install.ps1` never exits your
session: a failure is the error `install failed (exit N)`.

### Attestations

`--verify-attestation` also runs, for each download, the check in step
10, and fails (exit 2) on any that does not verify. It needs `gh`, logged
in: `gh attestation verify` refuses to run without authentication, so a
missing or logged-out `gh` is an error (exit 1), not a skipped check.

### Moving from a hand-written installer

- **Set `name` and `env_prefix`** to what your script used, so the Windows
  folder and the variables your users set stay the same.
- **Delete the scripts,** and their `extras` entries: with `installer`
  present, both names are the installers' own.
- **Move product-specific steps,** such as restarting a service or writing
  a configuration, into a subcommand of your program and name it in a
  hook. Shell completion and package managers stay outside the
  installers.
- **Your README's one-liners keep their URLs:** the asset names are the
  same.
- **Testing a release locally:** `SELFUPDATE_INSTALL_BASE_URL` replaces
  `https://github.com` with another `https://` origin, or a loopback
  `http://127.0.0.1:<port>` or `http://localhost:<port>`, and nothing else.
