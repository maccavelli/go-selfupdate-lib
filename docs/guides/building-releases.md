# Building releases

How a program builds, checks, stages and publishes its self-update release
with this repository's two reusable workflows, from one release spec that
the program also embeds. Why it works as it does is in
[0013-MADR](../decisions/0013-MADR-build-and-stage-release-workflow.md).

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

Both workflows are pinned to the same commit of this repository. Until
`v1.9.0` is tagged, the examples below show it as `<v1.9.0-commit>`.

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
    {"name": "install.sh", "path": "scripts/install.sh"}
  ],
  "prerelease_channels": ["rc", "beta"]
}
```

- **`products`:** each `name` is the asset prefix and the product your
  updater asks for. `package` is the main package, relative to the module.
  `tags` adds build tags. `identity_args` is optional; see step 3.
- **`platforms`:** GOOS and GOARCH pairs. Each must be a target of your
  Go toolchain.
- **`packaging`:** `binary` (the default) ships
  `<product>-<os>-<arch>[.exe]`; `archive` ships archives (step 7).
- **`extras`:** further assets, such as installers. `{tag}` in a name is
  replaced by the release tag. With `path`, the file comes from your
  repository; without it, from an artifact you upload (step 6).
- **`prerelease_channels`:** the channels the publish workflow may release
  `vX.Y.Z-NAME.N` tags for, most stable first; see
  [Offer a beta channel](extending-selfupdate.md#offer-a-beta-channel).

Unknown fields, a misspelled key and a duplicate key are errors, so a typo
fails loudly. `releasespec.Parse`'s errors name the field, such as
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
and the job summary says so.

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
    uses: maccavelli/go-selfupdate-lib/.github/workflows/build-selfupdate-release.yml@<v1.9.0-commit> # v1.9.0
    with:
      spec-path: internal/updateclient/selfupdate-release.json

  release:
    needs: build
    if: needs.build.outputs.rehearsal == 'false'
    permissions:
      contents: write
      id-token: write
      attestations: write
    uses: maccavelli/go-selfupdate-lib/.github/workflows/publish-selfupdate-release.yml@<v1.9.0-commit> # v1.9.0
    with:
      artifact-name: ${{ needs.build.outputs.artifact-name }}
      products-json: ${{ needs.build.outputs.products-json }}
      platforms-json: ${{ needs.build.outputs.platforms-json }}
      extra-assets-json: ${{ needs.build.outputs.extra-assets-json }}
      prerelease-channels-json: ${{ needs.build.outputs.prerelease-channels-json }}
```

- **Pin both to the full commit SHA of a release tag,** as the publish
  workflow has always required. Tags are annotated; resolve the commit with
  `git ls-remote https://github.com/maccavelli/go-selfupdate-lib 'refs/tags/v1.9.0^{}'`.
- **The build job needs only `contents: read`.** A called workflow cannot
  raise its token beyond what the calling workflow grants, so grant at
  least that.
- **`module-dir`** names the Go module when it is not the repository root.
  The Go version is that module's `go` line.
- **`artifact-name`** overrides the staged artifact's name. Set it when one
  run calls the build workflow twice.

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
    uses: maccavelli/go-selfupdate-lib/.github/workflows/build-selfupdate-release.yml@<v1.9.0-commit> # v1.9.0
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
  refuse is never published.
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
| `… printed "…" first, want "<tag> (release) …"` | the identity command does not print `buildinfo.Identity()` |
| `… did not finish within 30s` | the identity command waits for input or the network |
| `the extras directory holds …, which the spec does not list` | the extras artifact holds a file the spec does not name |
| `verify-selfupdate-release: file set mismatch …` | the staged set and the spec disagree; a bug to report |

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
  installers can read the spec later.
- **Delete** the CI steps that build, check cgo, write `SHA256SUMS`,
  re-verify and stage: the build workflow does each of them.
- **Keep** your test, lint and smoke jobs. The build job can `need` them.
- **An APK or other non-Go asset** stays in its own job, as in step 6.
