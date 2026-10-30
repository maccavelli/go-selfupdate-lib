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
- **`publish-selfupdate-release.yml`,** which validates, publishes,
  attests and waits for the release to be immutable, and, when you ask,
  signs `SHA256SUMS` first (step 13).
- **`install.sh` and `install.ps1`,** which the build workflow renders into
  the release from this repository's templates when the spec asks (step
  12).

Both workflows are pinned to the same commit of this repository; the
examples below pin `v1.23.0`'s.

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
    {"name": "relay-{tag}.notice.txt", "path": "NOTICE"}
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
- **`extras`:** further assets, such as a notice or an SBOM. `{tag}` in a
  name is replaced by the release tag. With `path`, the file comes from
  your repository: the path is relative to the repository root, and the
  file must be committed at the tagged commit, since the workflow runs
  none of your steps before it stages. A generated file, such as an SBOM,
  comes without `path`, from an artifact you upload (step 6).
- **`prerelease_channels`:** the channels the publish workflow may release
  `vX.Y.Z-NAME.N` tags for, most stable first, in strictly descending
  ASCII order (`rc` before `beta`); a list in another order, or with a
  name twice, is refused. See
  [Offer a beta channel](extending-selfupdate.md#offer-a-beta-channel).
- **`installer`:** present, even empty, it adds `install.sh` and
  `install.ps1` to every release; see step 12.

**Limits:** 1 to 16 products and 1 to 32 platforms; at most 32 extras;
at most 16 `tags` and 16 `identity_args` for a product; at most 8 hooks,
each with 1 to 16 arguments; and a spec of at most 64 KiB
(`releasespec.MaxSize`).

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
and requires exactly `<tag> (release)`, optionally followed by the commit,
as the first line, and an exit status of 0. The run's environment holds
`PATH` alone, and `SystemRoot` on Windows, so a command that reads `HOME`
or a configuration file fails there, with its output in the error.
A product without `identity_args` is built and checked without being run,
and the job summary says so. The installers run the same command after an
install, with no standard input (`install.ps1` since `v1.15.2`).

The linux/arm64 and windows/arm64 runners, `ubuntu-24.04-arm` and
`windows-11-arm`, were announced for public repositories. In a private
repository that ships those platforms with `identity_args`, their identity
jobs may find no runner, and wait.

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
    uses: maccavelli/go-selfupdate-lib/.github/workflows/build-selfupdate-release.yml@67520be8af1ad1b6228c29e4b1aa3e61e1ca7ae4 # v1.23.0
    with:
      spec-path: internal/updateclient/selfupdate-release.json

  release:
    needs: build
    if: needs.build.outputs.rehearsal == 'false'
    permissions:
      contents: write
      id-token: write
      attestations: write
    uses: maccavelli/go-selfupdate-lib/.github/workflows/publish-selfupdate-release.yml@67520be8af1ad1b6228c29e4b1aa3e61e1ca7ae4 # v1.23.0
    with:
      artifact-name: ${{ needs.build.outputs.artifact-name }}
      products-json: ${{ needs.build.outputs.products-json }}
      platforms-json: ${{ needs.build.outputs.platforms-json }}
      extra-assets-json: ${{ needs.build.outputs.extra-assets-json }}
      prerelease-channels-json: ${{ needs.build.outputs.prerelease-channels-json }}
```

- **Signing is opt-in:** `sign`, `note-verifier-key` and the
  `SELFUPDATE_NOTE_KEY` secret are left out above; step 13 adds them.
- **Pin both to the full commit SHA of a release tag,** as this
  repository tells callers; neither workflow checks it. Tags are
  annotated; resolve the commit with
  `git ls-remote https://github.com/maccavelli/go-selfupdate-lib 'refs/tags/v1.23.0^{}'`.
- **The build job needs only `contents: read`.** A called workflow cannot
  raise its token beyond what the calling workflow grants, so grant at
  least that.
- **`module-dir`** names the Go module when it is not the repository root.
  That module must require this library, whose `releasespec` reads the
  spec. The Go version is its `toolchain` line, or its `go` line when it
  has none (`setup-go`'s `go-version-file`). Since `v1.11.0` the plan
  step also checks that module's requirement of this library: a spec
  that uses a field or value an older release refuses, such as
  `installer` (`v1.10.0`) or a `before_uninstall` hook (`v1.17.0`), needs
  a module that requires a release that reads it. A
  `replace` with a directory is not checked, and the job summary says so.
- **`artifact-name`** overrides the staged artifact's name. Set it when one
  run calls the build workflow twice. Without it, the name is
  `selfupdate-release-<tag>-<attempt>`, or
  `selfupdate-rehearsal-<first 12 hex of the commit>-<attempt>` in a
  rehearsal. A name must match `^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`, or
  the plan step stops with a usage error,
  `artifact name "…" must match …`.
- **`retention-days`** keeps the staged artifact that many days, 7 by
  default. The build job also uploads `<artifact-name>-tools`, the built
  release tool the identity jobs use, kept for 1 day: re-run failed
  identity jobs within a day, and a cancelled publish within the staged
  artifact's retention. Do not upload an artifact of your own with that
  name. With `sign: true` the sign job uploads a third,
  `<artifact-name>-note` (step 13).
- **The jobs' limits:** the build job times out after 30 minutes, and
  each identity job after 10.
- **Its outputs** are `artifact-name`; `tag`, empty in a rehearsal;
  `rehearsal`, `'true'` or `'false'`; and the four publish inputs.
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
`rehearsal-<first 12 hex of the commit> (local)` instead of as a release,
and the `release` job
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
    uses: maccavelli/go-selfupdate-lib/.github/workflows/build-selfupdate-release.yml@67520be8af1ad1b6228c29e4b1aa3e61e1ca7ae4 # v1.23.0
    with:
      spec-path: internal/updateclient/selfupdate-release.json
      extras-artifact-name: extras
```

The artifact must hold exactly the extras without a `path`, under their
names after `{tag}` is replaced, as regular files. In a rehearsal `{tag}`
is the stamp, `rehearsal-<first 12 hex of the commit>`, so the extras job
names its files from `github.ref_name` on a tag and from the first 12
characters of `github.sha` otherwise; the build workflow does not output
the stamp.

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
- **Modules come from the module proxy:** `GOFLAGS=-mod=readonly`
  ignores a `vendor/` directory, and the workflow takes no input or
  secret for `GOPRIVATE` or for credentials, so a program that depends
  on a private module cannot be built with it.
- **setup-go's cache is off,** so a release build restores nothing
  another ref wrote.
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
| `… did not finish within 30s` | the identity command waits on the network, or on something else; its standard input is the null device, so a read of it ends at once |
| `the extras directory holds …, which the spec does not list` | the extras artifact holds a file the spec does not name |
| `verify-selfupdate-release: file set mismatch …` | the staged set and the spec disagree; a bug to report |
| `releasespec: extras[…].name: "install.sh" is already an installer's name` | the spec has `installer` and still lists a hand-written installer; delete the extra |
| `releasespec: …args[…]: "…" must match …, as the installers embed it` | a hook argument or `identity_args` entry with a space, quote or `$`; with `installer`, both keep to `[A-Za-z0-9._:=/,+@%-]` |
| `releasespec: installer.env_prefix: the prefix made from …; set installer.env_prefix` | the prefix made from `installer.name`, or from the repository's name when it is not set, does not match `^[A-Z][A-Z0-9_]{0,31}$`: it starts with a digit or `_`, or is longer than 32 characters (step 12) |
| `releasespec: "…": null is not allowed; leave the field out` | a field set to `null`, which the spec does not read as absent |
| `module … requires go-selfupdate-lib …; this spec's "…" needs v… or later …` | your program's `go.mod` requires a release that cannot parse the spec it embeds: `"installer"` needs `v1.10.0`, and `"installer.hooks[].when before_uninstall"` `v1.17.0`. Move it to the version the workflow is pinned to; the message's `go get` names the highest floor the module misses |
| `module … does not require github.com/maccavelli/go-selfupdate-lib, whose releasespec reads the spec` | the module `module-dir` names does not require this library; add it |
| `extra … has no path, and no extras directory was given` | an extra without `path`, and no `extras-artifact-name`; give one or the other (step 6) |
| `refuse-existing-release: release <tag> already exists (including drafts)` | the tag already has a release, a draft left by a failed run included; see below |
| `publish-selfupdate-release: sign needs note-verifier-key` | `sign: true` without `note-verifier-key`, or with it set from an empty variable (step 13) |
| `selfupdate-note: SELFUPDATE_NOTE_KEY is empty: the caller must pass the secret to the workflow` | `sign: true`, but the caller's `secrets:` does not pass `SELFUPDATE_NOTE_KEY`, or the secret is empty (step 13) |
| `selfupdate-note: the note does not open with -verifier-key: …` | `note-verifier-key` is not the verifier key of the signer key in `SELFUPDATE_NOTE_KEY` (step 13) |
| `publish-selfupdate-release: this gh has no 'release verify'`, `… this gh's release view has no isImmutable` or `… gh <version> is older than 2.81.0` | the runner's `gh` is older than 2.81.0, which the publish job needs to check that the release is immutable; nothing was published |

A failed `sign` publishes nothing. A publish that failed after it
created the draft, or after it published it, is recovered as the
README's [When a publish fails](../../README.md#when-a-publish-fails)
says.

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
  variable: the workflow always stamps a tag build as a release. The
  commit and its time are in `Identity()`'s `Revision` and `Time`.
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

Your program's module should require the version the workflow is pinned
to, and must require at least `v1.10.0`: an older `releasespec.Parse`
refuses the spec's `installer` field, and the shipped program cannot
update itself. A `before_uninstall` hook raises that floor to `v1.17.0`,
for the same reason. Since `v1.11.0` the plan step refuses a requirement
below the floor; it does not check that the requirement matches the pin.
Move `go.mod` and both pins together.

```json
"installer": {
  "name": "relay",
  "env_prefix": "RELAY",
  "hooks": [
    {"when": "after_install", "product": "relay", "args": ["configure", "--defaults"]},
    {"when": "before_uninstall", "product": "relay", "args": ["service", "uninstall"]}
  ]
}
```

- **`name`** names the Windows install folder,
  `%LOCALAPPDATA%\Programs\<name>`. The default is the repository's name.
- **`env_prefix`** prefixes the installers' variables, such as
  `RELAY_VERSION`. The default is made from `name`, so from the
  repository's name when `name` is not set: in upper case, with each
  letter `A` to `Z` and each digit kept, and every other character as
  `_`. A prefix must match `^[A-Z][A-Z0-9_]{0,31}$`, a leading letter
  and at most 32 characters; when the one made from the name does not,
  set `env_prefix`.
- **`hooks`,** at most eight, run one of your products around the install
  and before an uninstall (below).

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

- **Download** `SHA256SUMS` and this platform's asset of each selected
  product, from the release's tag, over HTTPS only:
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
  put back, and a new binary with no earlier copy is removed. On Windows,
  one that the identity check's process still holds after 2 s is renamed
  `<product>.exe.bad-<guid>`, so it never keeps its name (since
  `v1.12.1`). A `.prev` that a running program still holds is renamed
  `<product>.exe.old-<guid>` in the same way.
- **PATH:** advice on Unix. On Windows the user PATH is updated, editing
  the registry value so `%VAR%` entries and its type are kept.

### Options

| `install.sh` | `install.ps1` | Variable | What it does |
| :--- | :--- | :--- | :--- |
| `--version TAG` | `-Version TAG` | `<PREFIX>_VERSION` | install another release, with or without its `v`: a stable tag, or a prerelease on a channel the spec lists. A release whose asset names differ, such as a raw release from an archive release's installer, is refused (exit 1) with its own installer's URL |
| `--dir DIR` | `-InstallDir DIR` | `<PREFIX>_INSTALL_DIR` | install there; a relative folder is made absolute first |
| `--product NAME` | `-Product NAME` | | install only that product. For more, repeat `--product`, or give `install.ps1` a list, `-Product a,b`: PowerShell refuses a parameter given twice. A name given twice counts once |
| `--verify-attestation` | `-VerifyAttestation` | | also verify each download's attestation with `gh` (below) |
| `--no-hooks` | `-NoHooks` | `<PREFIX>_NO_HOOKS=1` | skip the hooks |
| | `-NoPathUpdate` | `<PREFIX>_NO_PATH_UPDATE=1` | print PATH advice instead of updating it |
| `--allow-root` | | | allow running as root |
| `-h`, `--help` | | | print the options and exit 0 |
| `--dry-run` | `-DryRun` | | print what would be downloaded and where it would go, and change nothing; with `--uninstall`, the `before_uninstall` hooks it would run and the files it would remove |
| `--uninstall` | `-Uninstall` | | after the `before_uninstall` hooks, remove the binaries and `.prev` files from `--dir` (`-InstallDir`), `<PREFIX>_INSTALL_DIR` or the default folder, so a program installed with `--dir` is uninstalled with the same `--dir`; and on Windows remove the folder's PATH entry with the last program. Configuration is left, and so are Windows' `.old-` and `.bad-` files. Since `v1.17.1`, a program that cannot be removed is reported, `install: could not remove <path>`, the others are still removed, and the exit is 3 (below) |
| `--force` | `-Force` | | with `--uninstall` only: remove even when a `before_uninstall` hook fails, and exit 3; refused (exit 1) without it |

`irm … | iex` cannot pass options: use the variables, or the scriptblock
form. A switch takes no value: `-DryRun`, not `-DryRun:$true`, which is
refused.

On Windows a running program cannot be removed. Since `v1.17.1`,
`-Uninstall` then reports
`install: could not remove <exe>; stop the program and run the uninstall again`,
removes the others, keeps the folder on PATH while a program remains,
and exits 3; before, it reported the program removed and exited 0. A
`before_uninstall` hook can stop the program first.

### Hooks

- **`before_install`** runs the installed copy, when there is one, before
  the new binary takes its place: for example, to stop a service. If it
  fails, nothing is changed and the installer exits 1. A first install
  skips it.
- **`after_install`** runs the new copy after the swap and the identity
  check: for example, to write a default configuration. If it fails, the
  program stays installed and the installer exits 3.
- **`before_uninstall`** runs the installed copy, when there is one,
  before `--uninstall` removes it: for example, to remove the service it
  registered. Only the selected products' hooks run, so `--product`
  narrows them as it narrows the removal. If one fails, the rest are
  skipped, nothing is removed and the installer exits 1. With `--force`,
  every hook runs, the binaries are removed anyway, and a failure gives
  exit 3. Since `v1.17.0`.
- **Arguments** keep to `[A-Za-z0-9._:=/,+@%-]`, with no space, quote or
  `$`, because the installers embed them in shell and PowerShell code. With
  `installer` present, so do `identity_args`. A step that needs more
  belongs in a subcommand of your program.
- They run with no standard input, and only a regular file is run.
  `install.ps1` gives them the `NUL` device since `v1.15.2`; before, they
  shared the console's input.
- Their output is shown; `--no-hooks` skips them.

### Exit codes

| Code | Meaning |
| :--- | :--- |
| 0 | installed (or removed, or a dry run) |
| 1 | a usage or environment error: an unknown option, an empty or unsafe `--product`, `--force` without `--uninstall`, an unsupported platform, root, a missing tool, a refused version, on install a directory where a program or its `.prev` goes, a failed `before_install` hook (nothing changed) or `before_uninstall` hook (nothing removed) |
| 2 | a verification failure: a download, `SHA256SUMS`, a checksum, an attestation or the identity; nothing was changed, or the previous binaries were put back |
| 3 | installed, but an `after_install` hook failed; or removed under `--force`, but a `before_uninstall` hook failed; or, since `v1.17.1`, removed only in part: a program could not be removed |

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

## 13. Sign a release (opt-in)

The publish workflow can sign `SHA256SUMS` with a release statement,
`SHA256SUMS.note`, which your program checks with
`selfupdate/verify/signednote` before it downloads a binary
([0025-MADR](../decisions/0025-MADR-align-with-go-sdk-design-practices.md)
D10). Nothing signs until you turn it on, and existing binaries ignore the
note.

1. **Make the keys on your own machine,** never in CI, from a checkout of
   this repository at your pinned commit: a CI key, and an offline backup
   key.

   ```sh
   go run ./internal/cmd/selfupdate-note keygen -name github.com/<owner>/<repo>/selfupdate -out ci.key
   go run ./internal/cmd/selfupdate-note keygen -name github.com/<owner>/<repo>/selfupdate-backup -out backup.key
   ```

   Each prints its verifier key. Keep the signer keys as the research
   report advises: encrypted, in two places, out of any repository
   ([0004-REPORT](../reports/0004-REPORT-release-signing-research.md),
   "Keys, rotation and revocation" and "Where the offline backup key could
   be kept").
2. **Create the environment `selfupdate-signing`** in your repository,
   allowed to deploy from `v*` tags only. Its secret `SELFUPDATE_NOTE_KEY`
   is the CI signer key, and the repository variable
   `SELFUPDATE_NOTE_VKEY` holds its verifier key.
3. **Call the publish workflow with signing,** passing the secret by name.
   A secret you do not pass is an empty string in the called workflow,
   and the sign job then fails, so nothing unsigned is published by
   mistake:

   ```yaml
     release:
       # … as in step 4
       with:
         # … as in step 4
         sign: true
         note-verifier-key: ${{ vars.SELFUPDATE_NOTE_VKEY }}
       secrets:
         SELFUPDATE_NOTE_KEY: ${{ secrets.SELFUPDATE_NOTE_KEY }}
   ```

   The `sign` job runs in that environment, with a read-only token. It
   checks the staged set, signs the exact `SHA256SUMS` for
   `github.com/<owner>/<repo>`, the tag and your products, opens its own
   note with `note-verifier-key`, and hands it to `publish`. `publish`
   adds it to the release, and attests it with the rest. A failed `sign`
   publishes nothing.

   The note goes from `sign` to `publish` as the artifact
   `<artifact-name>-note`, kept for 1 day: re-run a cancelled or failed
   `publish` job within a day, since after that it finds no note and
   fails. Do not upload an artifact of your own with that name.
4. **Embed the verifier keys** in your program as Go constants, and add
   the verifier, with `RequiredFrom` at your first signed tag:

   ```go
   const (
       ciKey     = "github.com/<owner>/<repo>/selfupdate+<hash>+<key>"
       backupKey = "github.com/<owner>/<repo>/selfupdate-backup+<hash>+<key>"
   )

   v, err := signednote.New(signednote.Config{
       Repository:   "github.com/<owner>/<repo>",
       Keys:         []string{ciKey, backupKey},
       RequiredFrom: "v1.4.0",
   })
   // Config.ManifestVerifiers: []selfupdate.ManifestVerifier{v}
   ```

   Not with `-ldflags -X`: a build without the flag would trust no key.
   An older, unsigned release still installs with an exact `--version`.

To rotate, sign with the old and the new key, release a version that
trusts both, then sign with the new key alone (0004-REPORT, "Planned
rotation").
