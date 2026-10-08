---
status: accepted
date: 2026-10-05
decision-makers: go-selfupdate-lib maintainers (the owner)
consulted: GitHub Actions documentation and changelogs, the READMEs and source of actions/upload-artifact, actions/download-artifact, actions/attest-build-provenance and the actions toolkit, the ubuntu-24.04 runner image readme, the Go 1.27.1 toolchain and its documentation (cmd/go, cmd/link, debug/buildinfo, embed), go.dev/blog/rebuild, the GNU tar manual, GoReleaser's documentation, slsa-github-generator's specification; the release pipelines of every fleet repository
informed: magic-cli-remote, mcp-server-magictools, mcp-server-recall, mcp-server-socratic-thinker, mcp-server-duckduckgo, prepare-commit-msg, gobble-cli
---
# Build, check and stage a self-update release in a reusable workflow, from one release spec that the program embeds

## Context and Problem Statement

[0004-MADR-evolve-selfupdate-api-and-tui-support.md](0004-MADR-evolve-selfupdate-api-and-tui-support.md)
§7 (Phase 4) lists, first among its shared tooling:

> **A build-and-stage reusable workflow**, plus one platform-matrix file
> read by Go (`go:embed`) and by CI. It builds with the canonical ldflags,
> asserts that the built binary reports `build_kind=release`, then
> generates `SHA256SUMS` and uploads. This would have caught C1 and C2.
> It changes the workflow contract, so it needs a record.

[0010-MADR-remediate-second-debugging-pass-findings.md](0010-MADR-remediate-second-debugging-pass-findings.md)
lists it as "absent; it needs its own record".
[0012-MADR-archive-assets-and-macos-codesign.md](0012-MADR-archive-assets-and-macos-codesign.md)
§8 makes it the gate for fleet archive releases: "A fleet product can
adopt an archive release only when the build-and-stage workflow's record
changes that contract."

This is that record. Research for it found four things the 0004 sentence
does not settle:

1. **A built binary cannot be read for its stamps.** With `-trimpath`,
   which every fleet release build uses, Go leaves `-ldflags` out of the
   binary's build information. With `-s`, which most use, there is no
   symbol table to find the variable by. And the linker silently ignores
   a `-X` for a symbol the binary does not contain. "Asserts that the
   built binary reports `build_kind=release`" needs a different method
   (§5).
2. **`go:embed` cannot reach a file outside the package directory.** The
   file has to live where the program's update code is, not at the
   repository root (§2).
3. **The publish workflow refuses an archive release.** Its verifier
   requires `SHA256SUMS` to list exactly the raw binaries (§8).
4. **The fleet's release pipelines have drifted further than 0004
   recorded.** Among the six programs that call the publish workflow
   there are three platform sets, three sets of stamp symbols and three
   `SHA256SUMS` generators. C1 is still live in one program and C2 in
   three ("What the fleet does today").

### What the release path does today

`publish-selfupdate-release.yml` is a `workflow_call` workflow with four
required inputs (`artifact-name`, `products-json`, `platforms-json`,
`extra-assets-json`) and one optional (`prerelease-channels-json`,
default `'[]'`). It needs `contents: write`, `id-token: write` and
`attestations: write` (`:32-35`). On a tag ref it:

* checks out its own commit at `.core-lib-release-tools`, through
  `job.workflow_repository` and `job.workflow_sha` (`:69-78`);
* requires an admitted tag (`check-release-tag.sh`) and refuses an
  existing release, drafts included (`refuse-existing-release.sh`);
* downloads the caller's artifact into `staging/` (`:123-127`);
* validates it with `scripts/verify-selfupdate-release.sh`;
* creates a draft, uploads one argument per file, attests `staging/*`
  with `actions/attest-build-provenance` v3.2.0 (`:183-186`), publishes,
  and waits for immutability and `gh release verify`.

It builds nothing, and does not generate `SHA256SUMS`: the caller stages
both.

`verify-selfupdate-release.sh`:

* accepts platform objects with "only os and arch" (`:113`);
* names each binary `<product>-<os>-<arch>`, plus `.exe` on Windows
  (`:135-139`), as `selfupdate.ExactAssetName` does
  (`selfupdate/assets.go:59`);
* requires the staged set to equal those binaries, `SHA256SUMS` and the
  extras, regular files only (`:146-164`);
* refuses an empty binary (`:186`), and parses `SHA256SUMS` with
  `selfupdate_manifest.py`, the client's parser ported and held to it by
  `TestManifestDifferential`;
* fails "SHA256SUMS must contain exactly the canonical binaries"
  (`:173`). An archive can be published only as an extra, which
  `SHA256SUMS` may not list, so a client cannot verify it.

`buildinfo` owns the stamps. `LDFlags(tag)` returns
`-X <VersionVar>=<tag> -X <KindVar>=release`
(`buildinfo/buildinfo.go:34-36, 165`). `Identity()` reports `KindRelease`
only when the kind is exactly `release` and the version matches
`releaseTag` (`:45`). That pattern admits every tag `check-release-tag.sh`
admits, and more: it accepts any prerelease name, where the script
accepts only the listed channels. A tag the workflows admit therefore
always stamps as a release.
`Info.String()` is `<Current> (<kind>)`, then the first 12 characters of
`vcs.revision`, then `-dirty` when modified. `selfupdate/cli` imports
`buildinfo` (`command.go:9`, `flags.go:8`), so any program that uses
`cli` links `buildinfo`, whether or not it reports `Identity()`.

`selfupdate/archive` (`v1.8.0`) names archives with `FleetName`,
`<product>-<os>-<arch>.tar.gz`, `.zip` or `.gz`, and by default picks zip
on Windows and tar.gz elsewhere (`select.go:52-54, 160`). Its unpacker
finds the one regular file named `<product>` (`.exe` on Windows) at the
top level or one directory down (`unpack.go:34-38`). `Unpack` reads
`UnpackRequest.Archive` and writes `UnpackRequest.Program`, both plain
paths (`:106, 123, 132`), so it runs outside an update session.
`selfupdate.CheckImage` checks that a file is an executable image for a
platform, and `selfupdate.ParseSHA256SUMS` is the client's manifest
parser. Both are exported.

### What the fleet does today

A read-only survey of every repository beside this one (2026-10-05). It
cites paths in those repositories.

| Repository | Self-update module | Platforms | Stamp symbols | Kind on a tag build | `SHA256SUMS` from | Publish workflow |
| :--- | :--- | :--- | :--- | :--- | :--- | :--- |
| prepare-commit-msg | go-selfupdate-lib v1.5.0 | linux, darwin, windows × amd64, arm64 (6) | `buildinfo.version`, `buildinfo.kind` | release (hard-coded in `build-all`) | `scripts/verify-release.sh` | this repository's, v1.5.0 |
| magic-cli-remote | mcplib v1.4.1 | linux/amd64, linux/arm64, darwin/arm64, windows/amd64 | `main.version`, `main.commit`, `main.date`, `main.buildKind` | release | `sha256sum mcremote-* mcrelay-*` | mcplib v1.4.1, with `bridge-release` |
| mcp-server-magictools | mcplib v1.4.1 | linux/amd64, darwin/arm64, windows/amd64 | `main.RawVersion`, `main.RawBuildKind` | **local** (C2) | a glob into `sha256sum` | mcplib v1.4.1 |
| mcp-server-recall | mcplib v1.4.1 | the four of magic-cli-remote | as magictools | **local** (C2) | as magictools | mcplib v1.4.1 |
| mcp-server-socratic-thinker | mcplib v1.4.1 | as magictools | as magictools | **local** (C2) | as magictools | mcplib v1.4.1 |
| mcp-server-duckduckgo | mcplib v1.4.1 | as magictools | as magictools | release | as magictools | mcplib v1.4.1 |

* **Every one builds on a single ubuntu runner by cross-compiling.** None
  builds natively. Native runners appear only in test and smoke jobs.
* **The platform list is written three to six times per repository:**
  the Makefile's `build-all`, CI's `platforms-json`, the Go list passed to
  `NewExactAssetSelector`, the installers' architecture rules, and
  verify scripts (`prepare-commit-msg/scripts/verify-release.sh:58-67`,
  `magic-cli-remote/scripts/verify-build-metadata.sh:20-23`). Nothing ties
  them together.
* **C1 is still live.** mcp-server-socratic-thinker passes
  `Product: cliName`, `"socratic-thinker"`
  (`cmd/mcp-server-socratic-thinker/update.go:109`, `constants.go:5`),
  and publishes `mcp-server-socratic-thinker-<os>-<arch>`.
* **C2 is still live** in magictools, recall and socratic: the tag build
  runs `make build-all VERSION="$TAG"` with no `BUILD_KIND`, and each
  Makefile defaults to `local`.
* **The version format differs:** `vX.Y.Z` in most, `X.Y.Z` in
  magic-cli-remote (`ci.yml:198, 220`).
* **Flags differ:** `-tags netgo -extldflags '-static'` on Linux in some;
  `netgo,osusergo` in magic-cli-remote; `-s -w` in all but gobble-cli,
  which has no `-trimpath` either; `CGO_ENABLED=0` explicit on some
  targets only.
* **Extras:** `install.sh` and `install.ps1` in four repositories, and in
  magic-cli-remote an APK built by another job, named with the tag
  (`magic-cli-remote-${TAG}-arm64.apk`).
* **Seven GitLab-era servers** publish with an unpinned
  `softprops/action-gh-release@v3` and do not self-update. ocp-login and
  ocp-login-macos release to GitLab with their own updater. None of them
  is a caller of this workflow.
* **No repository uses GoReleaser,** and pi-go's
  `docs/decisions/0004-MADR-go-module-architecture.md` states "no
  goreleaser" as fleet policy.

### What the platforms and tools require

**Go** (Go 1.27.1, probed on darwin/arm64; outputs under "Probe
evidence"):

* **`-trimpath` drops `-ldflags` from the build information.**
  `cmd/go/internal/load/pkg.go` records `-ldflags` only
  `if !cfg.BuildTrimpath` (go.dev/issue/52372). Without `-trimpath` the
  line is `build -ldflags="-s -w -X main.v=v1.2.3"`; with it, absent.
* **The linker ignores `-X` for a symbol the binary lacks:**
  `-X main.nonexistent=v1 -X example.com/other.kind=release` builds and
  exits 0.
* **The build information records what a release check needs:**
  `-trimpath=true`, `CGO_ENABLED`, `GOOS`, `GOARCH`, `GOAMD64` /
  `GOARM64`, `-tags`, `vcs.revision`, `vcs.time` and `vcs.modified`.
  `debug/buildinfo.ReadFile` reads ELF, PE and Mach-O on any host.
* **Since Go 1.24 the main module's version comes from the VCS tag.** A
  clean checkout at tag `v1.2.3` builds with `mod example.com/p v1.2.3`;
  with an untracked file present it is `v1.2.3+dirty` and
  `vcs.modified=true`. An untracked file makes the build dirty, so no
  tool checkout or output may sit inside the source tree.
* **`go:embed` patterns are relative to the package directory and "may
  not contain '.' or '..'"** (pkg.go.dev/embed).
* **Builds are reproducible** for the same source, toolchain and flags
  with `-trimpath` and `CGO_ENABLED=0` (go.dev/blog/rebuild). Two
  `-trimpath` builds from different directories were byte-identical.
* **The linker ad-hoc signs darwin/arm64 binaries** by target, not host
  (`flags=0x20002(adhoc,linker-signed)`; golang/go#42684). A
  cross-compiled macOS build from Linux is therefore runnable on Apple
  silicon. This is verified from source and on a local build, not from a
  Linux host.
* **`-buildvcs=true` fails without `git`;** the default `auto` silently
  skips stamping. A shallow clone stamps correctly. actions/checkout sets
  `safe.directory` by default.

**GitHub Actions:**

* **`workflow_call` inputs are `boolean`, `number` or `string`.** A list
  travels as a JSON string and is read with `fromJSON`.
* **A called workflow may use `strategy.matrix` from `fromJSON` of an
  input or of a prior job's output,** up to 256 jobs. The `job` context
  is not available in `strategy` or `runs-on`.
* **Nesting** is up to 10 levels, and 50 unique reusable workflows per
  run (raised from 4 and 20, November 2025 changelog).
* **A called workflow can keep or reduce the caller's token permissions,
  never raise them.** Artifact actions within a run need no token scope.
* **`job.workflow_repository` and `job.workflow_sha`** (2026-09-03
  changelog) name the called workflow's own commit. Inside a called
  workflow, `github.*` is the caller's.
* **Outputs** pass from one called workflow to another through the
  caller's `needs.<job>.outputs`.
* **Artifacts** (upload v7.0.1, download v8.0.1): names are unique per
  run and immutable; default retention is 90 days, settable 1–90; a zip
  artifact loses file modes; download v8 fails on a digest mismatch. A
  called workflow's jobs are in the caller's run.
* **Attestations:** the attesting workflow is the signer.
  `builder.id` is the reusable workflow's `job_workflow_ref`;
  `externalParameters.workflow` is the top-level caller. `gh attestation
  verify` needs `--signer-workflow` or `--signer-repo` for a reusable
  workflow. attest-build-provenance v4.2.2 exists and is a wrapper on
  `actions/attest`; this repository pins v3.2.0.
* **The ubuntu-24.04 image** (20260927) has GNU tar 1.35, zip 3.0,
  coreutils 9.4, git 2.55.0, Python 3.12.3 and gh 2.101.0, and caches Go
  1.24, 1.25 and 1.26 only: setup-go downloads Go 1.27.1.
* **The six fleet repositories in the table above are public,** as is
  this one (`gh repo view`, 2026-10-05). GitHub announced its arm64
  runners for public repositories (see "Not verified").

**Peers:**

* **GoReleaser** names archives `{{ .ProjectName }}_{{ .Version }}_{{ .Os
  }}_{{ .Arch }}`, packs tar.gz with a zip override for Windows, forces
  mode 0755, and names the checksum file
  `{{ .ProjectName }}_{{ .Version }}_checksums.txt`. It builds, packs and
  publishes as one tool.
* **slsa-github-generator** reaches SLSA Build L3 by running the build
  inside a reusable workflow that the caller can influence only through
  inputs. Artifacts built outside that workflow are protected only by
  their hashes.
* **Deterministic packing:** GNU tar needs `--sort=name --mtime
  --owner=0 --group=0 --numeric-owner --format=posix` and `gzip -n`.
  Go's `archive/tar`, `archive/zip` and `compress/gzip` and Python's
  stdlib produced byte-identical repeats in a probe.

## Decision Drivers

* **One source for the platform list and the product names,** read by
  the program and by CI. This removes C1 and the drift.
* **The release recipe belongs to the workflow, not the caller,** so a
  forgotten `BUILD_KIND` (C2) cannot happen.
* **Check what can be checked from the binary itself,** and say plainly
  what static reading cannot prove.
* **The publish workflow keeps its contract and guarantees.** Existing
  callers pinned to an earlier commit are unaffected. A release built by
  other means still publishes as before.
* **Archives become publishable,** with the client's own unpacker as the
  judge, as `SHA256SUMS` already has the client's own parser.
* **One implementation of each rule:** Go code that the library already
  tests, not a second copy in shell.
* **No new module.** AGENTS.md requires a record for one; none is
  needed.
* **Reproducible:** the same tag builds the same bytes.
* **Testable in this repository's CI,** on every push, without
  publishing.

## Considered Options

* **A. A reusable `build-selfupdate-release.yml`, a release spec file
  embedded by the program, a `selfupdate/releasespec` package, and an
  internal Go tool that builds, checks and stages; the publish workflow
  learns archives**
* **B. As A, but one reusable workflow that builds and then calls the
  publish workflow nested**
* **C. A composite action instead of a reusable workflow**
* **D. GoReleaser**
* **E. A Makefile and workflow template copied into each repository**
* **F. Build each platform natively on a runner matrix**

## Decision Outcome

Chosen option: **"A"**, because:

* it is the only option that puts one platform file in front of both the
  Go program and CI;
* it takes the build recipe away from the caller;
* it judges archives and stamps with the library's own code;
* it leaves `publish-selfupdate-release.yml` callable exactly as today.

B is kept as a later addition (§11, Q2). How a nested call resolves is
not verified, and A keeps the publish workflow pinned and tested on its
own.

### 1. The pieces

| Piece | Kind | What it does |
| :--- | :--- | :--- |
| `selfupdate-release.json` | a file in the calling repository | the release spec: products, their packages, platforms, packaging, extras, prerelease channels |
| `selfupdate/releasespec` | new package (exported API) | parses and validates the spec; gives the program its platforms, asset selector, unpacker and product check |
| `internal/cmd/selfupdate-release` | new internal command, never released | `plan`, `build`, `stage`, `check` and `identity`: the workflow's logic, in Go, on the library's own code |
| `.github/workflows/build-selfupdate-release.yml` | new reusable workflow | builds, checks, packs and stages from the spec; uploads the artifact; outputs the publish inputs |
| `.github/workflows/publish-selfupdate-release.yml` | changed, additively | accepts a `format` per platform; checks archives with the library's unpacker |
| `scripts/verify-selfupdate-release.sh` | changed, additively | the archive names in the file set and in `SHA256SUMS` |

A program adopts it in three steps:

1. It adds the spec.
2. It embeds the spec and configures its updater from it.
3. It replaces its build, checksum and staging jobs with one call to the
   build workflow, and passes that workflow's outputs to the publish
   workflow.

### 2. The release spec

One JSON file, conventionally `selfupdate-release.json`, in the directory
of the Go package that configures the updater, because `go:embed` cannot
reach a parent directory. JSON because Go reads it with the standard
library and the workflow's tool is Go. YAML would need a module.

```json
{
  "schema": 1,
  "products": [
    {"name": "mcremote", "package": "./cmd/mcremote", "identity_args": ["version"]},
    {"name": "mcrelay", "package": "./cmd/mcrelay"}
  ],
  "platforms": [
    {"os": "linux", "arch": "amd64"},
    {"os": "linux", "arch": "arm64"},
    {"os": "darwin", "arch": "arm64"},
    {"os": "windows", "arch": "amd64"}
  ],
  "packaging": "binary",
  "extras": [
    {"name": "install.sh", "path": "scripts/install.sh"},
    {"name": "install.ps1", "path": "scripts/install.ps1"},
    {"name": "magic-cli-remote-{tag}-arm64.apk"}
  ],
  "prerelease_channels": ["rc", "beta"]
}
```

Rules. Every one is enforced by `releasespec.Parse`, and the workflow
calls nothing else to read the file.

* **One JSON value, unknown fields refused,** at most 64 KiB. `schema`
  must be `1`. A later schema is a new number, and an older library
  refuses it with an error naming the version.
* **`products`:** one to 16. Each `name` matches the publish verifier's
  product rule, `^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`, and is unique.
  * `package` is the main package, relative to the module directory:
    `.` or `./` followed by a valid slash path (`fs.ValidPath`).
  * `tags`, optional, are build tags, each `^[A-Za-z0-9_.]+$`, at most
    16.
  * `identity_args`, optional (§12, Q1), at most 16 non-empty strings
    without NUL, makes the program print `buildinfo.Identity().String()`
    as the first line of its stdout (§5). When present it has at least
    one entry.
* **`platforms`:** one to 32, unique. `os` and `arch` match
  `^[a-z0-9][a-z0-9_]*$`, as `NewExactAssetSelector` and the verifier
  require. `format`, optional, is `tar.gz`, `zip` or `gz`, only with
  `"packaging": "archive"`. Whether the pair is a real Go target is
  checked at build time against `go tool dist list`, by the toolchain
  that builds it.
* **`packaging`:** `binary` (the default) or `archive`. With `archive`,
  every platform is packed: as its `format`, else zip on Windows and
  tar.gz elsewhere, which is `archive.NewSelector`'s default. Mixing raw
  binaries and archives in one release is refused, because
  one selector cannot select both.
* **`extras`:** at most 32, unique. `name` matches the product rule after
  `{tag}`, the only placeholder, is replaced by the release tag. `name`
  is never `SHA256SUMS`, `SHA256SUMS-…` or a canonical asset's name.
  `path`, optional, is a valid slash path to a regular file in the
  calling repository. Without it, the file comes from the caller's extras
  artifact.
* **`prerelease_channels`:** as `prerelease-channels-json` today: each
  `^[a-z][a-z0-9]{0,15}$`, most stable first, strictly descending ASCII
  order.

### 3. `selfupdate/releasespec`

Exported fields with JSON tags, as the module's other documents use:

```go
const SchemaVersion = 1

type Spec struct {
    Schema             int        `json:"schema"`
    Products           []Product  `json:"products"`
    Platforms          []Platform `json:"platforms"`
    Packaging          Packaging  `json:"packaging,omitempty"`
    Extras             []Extra    `json:"extras,omitempty"`
    PrereleaseChannels []string   `json:"prerelease_channels,omitempty"`
}
type Product struct {
    Name         string   `json:"name"`
    Package      string   `json:"package"`
    Tags         []string `json:"tags,omitempty"`
    IdentityArgs []string `json:"identity_args,omitempty"`
}
type Platform struct {
    OS     string         `json:"os"`
    Arch   string         `json:"arch"`
    Format archive.Format `json:"format,omitempty"`
}
type Packaging string // PackagingBinary "binary", PackagingArchive "archive"
type Extra struct {
    Name string `json:"name"`
    Path string `json:"path,omitempty"`
}

func Parse(data []byte) (Spec, error)
func (s Spec) Validate() error
func (s Spec) Product(name string) (Product, error)
func (s Spec) Targets() []selfupdate.Platform
func (s Spec) FormatFor(p selfupdate.Platform) (archive.Format, bool)
func (s Spec) AssetName(product string, p selfupdate.Platform) (string, error)
func (s Spec) AssetSelector() (selfupdate.AssetSelector, error)
func (s Spec) Unpacker() (selfupdate.Unpacker, error)
func (s Spec) ExtraNames(tag string) ([]string, error)

// The publish workflow's inputs, as JSON strings.
func (s Spec) ProductsJSON() string
func (s Spec) PlatformsJSON() string          // with "format" when packed
func (s Spec) ExtrasJSON(tag string) (string, error)
func (s Spec) ChannelsJSON() string
```

* **`Product` fails for a name the spec does not list.** A program that
  calls it with its `Product` turns C1 into a startup error, which its
  tests catch.
* **`AssetSelector` returns `NewExactAssetSelector` or
  `archive.NewSelector`, and `Unpacker` returns nil or
  `archive.NewUnpacker`,** so the selector and unpacker always match. A
  mismatch is the misconfiguration the `v1.8.0` coordinator refuses
  before download.
* **`AssetName`** is `ExactAssetName` or `FleetName` for the platform's
  format: the one function the tool, the selector and the tests share.
* **Imports:** the standard library, `selfupdate` and
  `selfupdate/archive`. It gets a `depguard` rule of its own,
  `selfupdate-releasespec`, which sorts after `banned`, and is excluded
  from `other-packages`.

A program uses it so:

```go
//go:embed selfupdate-release.json
var releaseSpec []byte

spec, err := releasespec.Parse(releaseSpec)
// spec.Product(product); spec.AssetSelector(); spec.Unpacker()
```

### 4. The build recipe

The workflow builds every product for every platform on one
`ubuntu-24.04` runner by cross-compiling, as the whole fleet does today:

```sh
CGO_ENABLED=0 GOOS=<os> GOARCH=<arch> GOFLAGS=-mod=readonly GOTOOLCHAIN=local \
  go build -trimpath -buildvcs=true [-tags <tags>] \
    -ldflags "-s -w -X <buildinfo.VersionVar>=<tag> -X <buildinfo.KindVar>=release" \
    -o <out>/<asset> <package>
```

* **The stamp is always `buildinfo`'s,** with the full tag including
  `v`. A program that stamps its own variables must move to `buildinfo`
  to use the workflow. That has been the canonical stamp since `v1.4.0`
  (0004 §5, F1, F2), under this module path since `v1.5.0`.
* **No caller-supplied ldflags, `GOFLAGS`, environment or `GOAMD64` /
  `GOARM64` level.** Every input to the bytes is the tag's tree and the
  toolchain. Build tags are the one choice per product.
  * `CGO_ENABLED=0` uses Go's own resolver, so the fleet's `netgo` and
    `osusergo` tags and `-extldflags -static` change nothing. They stay
    expressible as `tags`.
* **The toolchain** is the calling module's `go` line, installed by
  `actions/setup-go` with its cache off, and pinned with
  `GOTOOLCHAIN=local`. The tool checks that every binary's `GoVersion`
  is that toolchain's. The cache is off because a release build should
  not restore a cache that another ref wrote.
* **Off a tag ref** the workflow runs as a *rehearsal*. The same recipe
  builds with `-X <VersionVar>=rehearsal-<sha12> -X <KindVar>=local`. A
  rehearsal never produces a release-stamped binary, so a rehearsal
  artifact can never be published as a release.
* **The source** is checked out at `github.sha` into `src/`, the tools
  at `job.workflow_sha` into `tools/`, and the output goes to
  `$RUNNER_TEMP`. None of them sits inside another, so nothing makes
  `src/` dirty.

### 5. What is checked, and how

Static reading cannot prove that a program *reports* `release`:

* the `-X` values are not in the build information under `-trimpath`;
* `-s` removes the symbols;
* the linker ignores a stamp for a missing symbol;
* `cli` links `buildinfo` into any program that uses it, even one that
  reports its own variables, as magic-cli-remote does today.

So the check is in three layers. The first two always run. The third
runs for each product that declares `identity_args`, on every platform
that has a runner (§12, Q1). The job summary names every product without
it, so the gap stays visible.

1. **Before the build, per product and platform:**
   * `go list -deps` for that GOOS/GOARCH must include
     `github.com/maccavelli/go-selfupdate-lib/buildinfo`. Without it the
     stamp would vanish silently.
   * The pair must be in `go tool dist list`.
2. **After the build, per binary, from `debug/buildinfo.ReadFile`:**
   * `GOOS`, `GOARCH` and `-tags` are the requested ones;
   * `CGO_ENABLED=0` and `-trimpath=true`;
   * `vcs.revision` is `github.sha` and `vcs.modified=false`;
   * `GoVersion` is the toolchain's;
   * the main package path is the module path joined with `package`;
   * on a tag build whose module is at the repository root, the main
     module version is the tag, which Go 1.24+ derives from VCS.
   * In addition, `selfupdate.CheckImage` accepts the file as an
     executable for the platform, and it is not empty.
3. **The identity run, on a native runner.** The workflow runs the
   staged asset with `identity_args`, taken from the artifact and
   unpacked first when packed, with a 30-second timeout and stdin closed.
   * The first line of stdout must match
     `^<tag> \(release\)( <sha12>)?$` (`rehearsal-<sha12> (local)` in a
     rehearsal). That is `Info.String()` for this build.
   * This is the only layer that proves 0004's "reports
     `build_kind=release`".
   * Runners: linux/amd64 `ubuntu-24.04`, linux/arm64
     `ubuntu-24.04-arm`, darwin/arm64 `macos-15`, windows/amd64
     `windows-2025`, windows/arm64 `windows-11-arm`.
   * Any other platform is not run, and the job summary says so. darwin
     on amd64 is among them.

Layers 1 and 2 catch C2 and any build that ignored the recipe. Layer 3
also catches a program whose `version` output ignores `buildinfo`. C1 is
caught by `Spec.Product` in the program and by the asset names the spec
dictates.

### 6. Packing and `SHA256SUMS`

The `stage` step, in Go:

* **A raw binary** is copied under its `ExactAssetName`.
* **An archive** holds the one program, `<product>` (`.exe` on Windows),
  at the top level, the unpacker's default lookup:
  * tar.gz: a USTAR regular file, mode 0755, uid and gid 0, no user or
    group names, modification time `vcs.time`. gzip with no name, no
    modification time and OS byte 255.
  * zip: deflate, Unix creator, mode 0755, modification time
    `vcs.time`.
  * gz: the binary alone, gzip as above.

  Each archive is then **unpacked with `archive.NewUnpacker`**, and the
  result must equal the binary byte for byte. The client's own refusals
  are the check, not a port of them.
* **`SHA256SUMS`** lists exactly the canonical assets (binaries or
  archives), as `<hex>  <name>`, LF, sorted by name in byte order. It is
  parsed back with `selfupdate.ParseSHA256SUMS` and compared. Extras are
  not listed, as today.
* **Extras** are copied from `path` in `src/`, or from the extras
  artifact. Only regular files are accepted, never a symlink. The staged
  set must then be exactly what `verify-selfupdate-release.sh` expects,
  and the workflow runs that verifier on it before upload, as publish
  will.

The same tag on the same toolchain packs the same bytes, so a re-run
produces identical digests.

### 7. `build-selfupdate-release.yml`

**Inputs** (strings unless noted):

| Input | Required | Meaning |
| :--- | :--- | :--- |
| `spec-path` | yes | the spec, relative to the repository root |
| `module-dir` | no, `.` | the Go module, relative to the repository root |
| `extras-artifact-name` | no, `''` | an artifact of this run holding the extras that have no `path` |
| `artifact-name` | no, `''` | overrides the default staged artifact name |
| `retention-days` | no, number, `7` | the staged artifact's retention |

**Outputs:**

* `artifact-name`;
* `tag`, which is empty in a rehearsal;
* `products-json`, `platforms-json` (with `format` when packed),
  `extra-assets-json` and `prerelease-channels-json`, the publish
  workflow's inputs, each derived from the spec;
* `rehearsal`: `true` off a tag.

**Jobs.** Permissions are `contents: read` and nothing else:

1. **`build`** (ubuntu-24.04):
   1. Check out the tools, then `src/`, without persisted credentials.
   2. Set up Go.
   3. Build the tool from `tools/`.
   4. On a tag, require an admitted tag with `check-release-tag.sh` and
      the spec's channels.
   5. `plan`, then `build`, then `stage`.
   6. Run `verify-selfupdate-release.sh` on the staging directory.
   7. Upload the staged artifact, and an artifact of the tool
      cross-compiled for each identity runner.
   8. Write a job summary listing the assets and their SHA-256.
2. **`identity`:** a matrix from `plan`'s output, one leg per product
   with `identity_args` and runnable platform. It downloads both
   artifacts and runs `selfupdate-release identity`. It is skipped when
   the matrix is empty.

**The default artifact name** is
`selfupdate-release-<tag>-<run_attempt>`, or
`selfupdate-rehearsal-<sha12>-<run_attempt>`, so a re-run does not
collide with an earlier attempt's immutable artifact.

**The workflow publishes nothing and needs no write permission.** It
attests nothing either: the publish workflow attests the files it
uploads, as today.

### 8. Changes to `publish-selfupdate-release.yml`

Both are additive. A caller that passes today's inputs gets today's
behaviour.

* **`platforms-json` objects may carry `format`**
  (`tar.gz`, `zip` or `gz`), on every object or none.
  * With it, the canonical asset is `FleetName`'s, and `SHA256SUMS` must
    list exactly those archives.
  * Without it, nothing changes.
  * `verify-selfupdate-release.sh` and its fixtures gain the archive
    names.
* **A new step, after "Validate the staged file set" and before "Create
  a draft release",** runs only when the verifier reports a packed
  release (a `format` is present):
  1. It sets up Go from the tools' `go.mod`, with the cache off.
  2. It runs `selfupdate-release check` on `staging/`.
  3. For each archive, the library's `archive.NewUnpacker` must extract
     a program that passes `CheckImage` for its platform.

  A release that the client's unpacker would refuse is never published,
  whoever built it.

No input is removed, and none changes meaning.
`actions/attest-build-provenance` stays at v3.2.0. Moving to v4 is
separate work (§10).

### 9. The caller

```yaml
jobs:
  build:
    uses: maccavelli/go-selfupdate-lib/.github/workflows/build-selfupdate-release.yml@<v1.9.0 commit> # v1.9.0
    with:
      spec-path: internal/updateclient/selfupdate-release.json
  release:
    needs: build
    if: needs.build.outputs.rehearsal == 'false'
    permissions:
      contents: write
      id-token: write
      attestations: write
    uses: maccavelli/go-selfupdate-lib/.github/workflows/publish-selfupdate-release.yml@<v1.9.0 commit> # v1.9.0
    with:
      artifact-name: ${{ needs.build.outputs.artifact-name }}
      products-json: ${{ needs.build.outputs.products-json }}
      platforms-json: ${{ needs.build.outputs.platforms-json }}
      extra-assets-json: ${{ needs.build.outputs.extra-assets-json }}
      prerelease-channels-json: ${{ needs.build.outputs.prerelease-channels-json }}
```

On a pull request or a branch push, the same `build` call rehearses. The
release recipe is then exercised on every change, not first on the tag.

### 10. What stays out

* **The installer templates** (`install.sh`, `install.ps1`), 0004 §7's
  second item. They are their own record. They can read the spec later.
* **Signing or notarizing macOS builds in CI.** No Apple identity exists
  (0012-MADR §10). The linker's ad-hoc signature is what ships.
* **An attestation from the build workflow,** and the move to
  attest-build-provenance v4 or `actions/attest`. Today the publish
  workflow is the signer; a verifier uses `--signer-workflow` with it.
* **GoReleaser naming in the fleet.** `FleetName` only. A GoReleaser
  release is still readable by the client (0012).
* **Files beside the program in an archive,** such as `LICENSE`. The
  schema can add a field later, additively.
* **Non-GitHub hosts** (ocp-login, ocp-login-macos), and the seven
  servers that do not self-update.
* **Moving each program onto the workflow.** That is each repository's
  own work, under its own records. This record's guide describes it.

### 11. Questions for the owner

* **Q1. Is the identity run required, or opt-in per product?**
  * Recommendation: opt-in in schema 1. The job summary names every
    product without it.
  * Requiring it would make each program add a command that prints
    `Info.String()`. magic-cli-remote prints its own variables today, so
    it could not adopt the workflow until it changes that command.
* **Q2. Two workflows that the caller chains (§9), or one that also
  publishes?**
  * Recommendation: two, now.
    * The publish contract stays as it is, tested and pinned.
    * A rehearsal is simply the build call with no publish job; a combined
      workflow would need a mode of its own to skip publishing.
    * A nested call's resolution of a relative path to the called
      repository's commit is not verified ("Not verified").
  * A combined `release-selfupdate.yml` can be added later without
    changing either.
* **Q3. Is the build recipe fixed, with build tags as the only choice per
  product?**
  * Recommendation: yes. Extra `-X` symbols, `GOFLAGS` or a microarchitecture
    level would bring back per-repository drift and the risk of a release
    built from inputs the tag does not hold.
  * Programs that stamp a commit and date get both from `buildinfo`'s
    `Revision` and `Time`.

### 12. Owner answers (2026-10-05)

The owner answered:

> 1. yes. we will address magic-cli-remote. 2. 2 workflows. 3. yes.

* **Q1: opt-in per product, as recommended.**
  * The owner first answered "yes", making the identity run required,
    and that answer was recorded, amending §2, §3, §5, §7 and the
    Consequences.
  * The same day the owner revised it: "change q1 to opt-in". Those
    amendments were undone.
  * §2 keeps `identity_args` optional; when present it has at least one
    entry.
  * §5 runs the third layer for each product that declares it, and the
    job summary names every product that does not.
  * magic-cli-remote can adopt the workflow before its `version` command
    reports `buildinfo`. The owner will address that command in that
    repository.

  A platform with no runner, darwin/amd64 among them, is reported and
  not run, as §5 says.
* **Q2: two workflows,** chained by the caller as §9 shows. Option B
  stays out.
* **Q3: the recipe is fixed,** as §4 states. Build tags are the only
  per-product choice.

### Consequences

* Good, because a program's platforms, product names, packaging and
  extras live in one file that the program embeds and the workflow reads
  with the same parser. C1 becomes a startup error, and the platform list
  cannot drift.
* Good, because the workflow, not the caller, sets the stamp. C2 cannot
  recur for a program built by it.
* Good, because every check uses the library's own code: the
  `SHA256SUMS` parser, `CheckImage` and the archive unpacker.
* Good, because fleet programs can publish archives for the first time,
  and the publish workflow refuses an archive the client would refuse.
* Good, because the same recipe runs on every pull request as a
  rehearsal, and this repository's CI rehearses it on a fixture on every
  push.
* Good, because the bytes are reproducible: no caller flags, no cache,
  fixed archive metadata.
* Neutral, because `selfupdate/releasespec` is new exported API, so the
  release is `v1.9.0`. `make apicheck` must stay compatible with
  `v1.8.0`.
* Neutral, because the internal command is a binary in the tree. It is
  never released, and AGENTS.md's "library only" is amended to say so.
* Neutral, because `-s -w` stays: binaries keep today's size, and the
  stamp is proved by the identity run rather than by symbols.
* Bad, because a program must move its stamps to `buildinfo`, and its
  spec next to its update code, before it can adopt the workflow.
  magic-cli-remote also has to move off `mcplib`.
* Bad, because the static layers cannot prove that a program reports its
  stamp. Only an opt-in identity run does, and only on platforms that
  have a runner. A product without one is named in every job summary.
* Bad, because the publish workflow gains a Go setup step for archive
  releases, about the cost of one module download.
* Bad, because the build and the attestation stay in two workflows that
  the caller joins. That falls short of SLSA Build L3's isolated builder
  (inferred from slsa-github-generator's specification). The staged
  files are bound by their digests, and download v8 refuses a mismatch.

### Confirmation

* **`selfupdate/releasespec`:**
  * table tests for every rule of §2, each seen to fail on a planted
    break;
  * a fuzz target for `Parse` in `make fuzz` and CI;
  * an example that embeds a spec.
* **The internal command:** tests that build a fixture program for
  linux/amd64, darwin/arm64 and windows/amd64, then:
  * stage it raw and as each archive format, and unpack the result with
    the library;
  * refuse a missing `buildinfo` dependency, a dirty tree, a wrong
    GOOS/GOARCH, cgo, a missing `-trimpath`, a wrong revision, a
    corrupted archive and a stray file.
* **The verifier's script tests** gain archive cases: a good release, a
  binary listed where an archive should be, mixed formats, and an
  unknown format.
* **`check-workflows.sh`** and actionlint pass on the new workflow and
  the changed one.
* **A CI job in this repository** calls `build-selfupdate-release.yml`
  through its local path on a fixture module under `testdata/`. It
  rehearses on every push and builds as a release on a `v*` tag, with an
  identity run on each of the five runners. The staged artifact is
  checked as publish would check it.
* **A live publish**, in a throwaway repository the owner provides,
  with immutable releases on:
  * a raw and an archive release;
  * `gh attestation verify` with `--signer-workflow`;
  * a scratch consumer that updates from one to the other through a
    `releasespec` configuration.

## Pros and Cons of the Options

### A. Reusable build workflow, embedded spec, `releasespec`, internal tool

* Good, because it is the only option where the program and CI read one
  file with one parser.
* Good, because the recipe is fixed in a workflow pinned by commit, which
  a caller cannot change.
* Good, because the tool reuses the library's tested parser, image check
  and unpacker, and can be run locally against a checkout.
* Good, because the publish contract is unchanged for existing callers.
* Neutral, because a caller writes two `uses:` jobs instead of one.
* Bad, because it adds exported API and an internal command to
  maintain.

### B. One workflow that builds and calls publish nested

* Good, because the caller cannot interpose between building and
  publishing, which is closer to an isolated builder.
* Good, because a caller writes one job.
* Neutral, because the attestation's signer would still be the nested
  publish workflow.
* Bad, because a relative nested `uses:` resolving to the called
  repository's commit is documented for same-repository calls and
  supported only by third-party reports for this case. It is not
  verified.
* Neutral, because a caller's own extra asset (an APK built in its own
  job) arrives as an input artifact, as it does in A.
* Bad, because a rehearsal needs a mode inside the combined workflow
  that skips publishing.

### C. A composite action

* Good, because it runs in the caller's job, with no artifact hop.
* Bad, because it runs with the caller's environment, `GOFLAGS`,
  toolchain and prior steps. The recipe is no longer the workflow's.
* Bad, because it cannot set its own permissions or runner, and cannot
  run a native identity matrix.

### D. GoReleaser

* Good, because it builds, packs, checksums and publishes, and is widely
  used.
* Bad, because the fleet's policy is "no goreleaser" (pi-go
  `0004-MADR-go-module-architecture.md`).
* Bad, because it publishes itself. It would bypass this repository's
  tag rule, existing-release refusal, file-set verification and wait for
  immutability.
* Bad, because its configuration is per repository YAML that the Go
  program cannot embed and that no library code validates. The drift
  moves rather than goes.

### E. A template copied into each repository

* Good, because it needs no change here.
* Bad, because it is the status quo. The magictools, recall and
  socratic pipelines are near-verbatim copies of one another, and all
  three still carry C2. The fleet's `ci-server.yml` template, copied into
  seven servers, uses unpinned actions.

### F. Native builds on a runner matrix

* Good, because each binary could run in place right after it is built.
* Neutral, because Go cross-compiles every fleet target without cgo, and
  the fleet already does.
* Bad, because the toolchain and runner image then differ per platform,
  and the artifacts must be merged from many jobs.
* Bad, because darwin/amd64 and some targets have no runner. Option A
  runs binaries natively only for the identity check, where it adds
  something.

## Amendments

### A1 (2026-10-07): §2's archive-name limits

*Status: accepted (2026-10-07). The finding and its fix are in [0015-MADR-remediate-third-debugging-pass-findings.md](0015-MADR-remediate-third-debugging-pass-findings.md)
(E5); they were built in [0015-PLAN-remediate-third-debugging-pass-findings.md](0015-PLAN-remediate-third-debugging-pass-findings.md), Phase P6, and ship in `v1.10.1`.*

* **§2:** under `"packaging": "archive"`, `Validate` refuses a spec whose
  composed asset name the client's archive selector would refuse (over
  128 characters), and a tar.gz program name over 100 characters, which
  the USTAR header `pack` writes cannot hold.
* **§8:** the publish workflow's archive check also selects each staged
  archive with the client's own selector, so a release the client could
  not select is never published.

### A2 (2026-10-08): null in the spec, and the module's library version

*Status: accepted (2026-10-08). The findings are in [0015-MADR-remediate-third-debugging-pass-findings.md](0015-MADR-remediate-third-debugging-pass-findings.md) (E7, F3); they
were built in [0015-PLAN-remediate-third-debugging-pass-findings.md](0015-PLAN-remediate-third-debugging-pass-findings.md), Phase Q4, and ship in `v1.11.0`.*

* **§2 (E7):** `Parse` refuses `null` for any value, which
  `encoding/json` would read as an absent field: `"installer": null` is
  not `"installer": {}`.
* **§7 (F3):** the plan step takes `-module-dir`, and checks that the
  module requires a release of this library that reads every field the
  spec uses (`installer` needs `v1.10.0`): a program built against an
  older one cannot parse the spec it embeds. A module that is the library,
  and a directory `replace`, are not checked; the job summary notes the
  latter.

## More Information

### Probe evidence

Go 1.27.1 on darwin/arm64, 2026-10-05, in a scratch module
`example.com/p` whose `main` prints `var v string`.
`go version -m` separates fields with tabs; they are shown here as
spaces.

`-trimpath` and `-ldflags`, building for linux/amd64 with
`-ldflags "-s -w -X main.v=v1.2.3"`:

```text
== without -trimpath
    build    -ldflags="-s -w -X main.v=v1.2.3"
    build    CGO_ENABLED=0
    build    GOOS=linux
== with -trimpath
    build    -trimpath=true
    build    CGO_ENABLED=0
    build    GOOS=linux
```

The module version from VCS, building for windows/amd64 with
`-trimpath -buildvcs=true` in a git repository tagged `v1.2.3`:

```text
== clean at tag
    mod    example.com/p    v1.2.3
    build    vcs=git
    build    vcs.revision=<40 hex>
    build    vcs.time=<RFC 3339>
    build    vcs.modified=false
== untracked file present
    mod    example.com/p    v1.2.3+dirty
    build    vcs.modified=true
```

A `-X` for a missing symbol,
`-trimpath -ldflags "-X main.nonexistent=v1 -X example.com/other.kind=release"`,
exits 0, and the program prints an empty `v`.

The research probes also built windows/amd64, darwin/arm64 and
linux/amd64 binaries and read `-trimpath=true`, `CGO_ENABLED=0`,
`GOOS`, `GOARCH`, `GOAMD64=v1` and `GOARM64=v8.0` from each. Two
`-trimpath` builds from different directories were identical. They
packed tar.gz, zip and gz twice each with the standard library, with
byte-identical results and modes kept. `codesign -dv` on a darwin/arm64
build showed `flags=0x20002(adhoc,linker-signed)`.

### Sources

* GitHub Actions:
  * Reusing workflow configurations:
    <https://docs.github.com/en/actions/reference/workflows-and-actions/reusing-workflow-configurations>
  * Reuse workflows:
    <https://docs.github.com/en/actions/how-tos/reuse-automations/reuse-workflows>
  * Workflow syntax:
    <https://docs.github.com/en/actions/reference/workflows-and-actions/workflow-syntax>
  * Contexts:
    <https://docs.github.com/en/actions/reference/workflows-and-actions/contexts>
  * Limits: <https://docs.github.com/en/actions/reference/limits>
  * Running variations of jobs:
    <https://docs.github.com/en/actions/how-tos/write-workflows/choose-what-workflows-do/run-job-variations>
  * Changelog, November 2025 (nesting 10, 50 workflows):
    <https://github.blog/changelog/2025-11-06-new-releases-for-github-actions-november-2025/>
  * Changelog, early September 2026 (`job.workflow_*`):
    <https://github.blog/changelog/2026-09-03-github-actions-early-september-2026-updates/>
  * Artifact attestations and reusable workflows:
    <https://docs.github.com/en/actions/how-tos/secure-your-work/use-artifact-attestations/increase-security-rating>
  * ubuntu-24.04 image:
    <https://github.com/actions/runner-images/blob/main/images/ubuntu/Ubuntu2404-Readme.md>
* Actions:
  * <https://github.com/actions/upload-artifact/blob/v7.0.1/README.md>
  * <https://github.com/actions/download-artifact/blob/v8.0.1/README.md>
  * <https://github.com/actions/attest-build-provenance/releases/tag/v4.0.0>
  * <https://github.com/actions/toolkit/blob/main/packages/attest/src/provenance.ts>
  * <https://github.com/actions/toolkit/blob/main/packages/artifact/src/internal/shared/config.ts>
* Go:
  * go.dev/issue/52372 (`-ldflags` omitted under `-trimpath`)
  * <https://pkg.go.dev/debug/buildinfo>
  * <https://pkg.go.dev/embed>
  * <https://go.dev/blog/rebuild>
  * <https://go.dev/doc/go1.24> (the main module version from VCS)
  * <https://github.com/golang/go/issues/42684> (linker ad-hoc signing)
  * <https://github.com/golang/go/issues/64275> (`-buildvcs` and git
    ownership)
* Packing:
  * <https://www.gnu.org/software/tar/manual/html_section/Reproducibility.html>
  * <https://goreleaser.com/customization/archive/>
  * <https://goreleaser.com/customization/checksum/>
* SLSA:
  * <https://github.com/slsa-framework/slsa-github-generator/blob/main/SPECIFICATIONS.md>

### Not verified

* **That actions/checkout on a tag push leaves the tag in the shallow
  clone,** so that Go stamps the tag as the main module version. Expected
  from checkout's tag fetch. The PLAN's tag rehearsal shows it. If not,
  §5's version check falls back to `vcs.revision` alone, by amendment.
* **That an annotated tag stamps like the lightweight one probed.**
* **That `ubuntu-24.04-arm` and `windows-11-arm` run jobs for these
  public repositories.** GitHub announced both for public repositories.
  The PLAN's CI job runs on each.
* **That a nested reusable call by relative path resolves to the called
  repository's commit** (Q2, option B). Third-party reports only.
* **That a re-run of a job can upload an artifact name that an earlier
  attempt uploaded.** The name includes `run_attempt` so it never has to.
* **GoReleaser's checksum line format.** Not needed; it is not
  generated.
* **SLSA level.** The "falls short of L3" judgement is inferred, not
  quoted.

### Related

* [0004-MADR-evolve-selfupdate-api-and-tui-support.md](0004-MADR-evolve-selfupdate-api-and-tui-support.md):
  §4 (C1, C2), §5 (`buildinfo`), §7 Phase 4.
* [0002-MADR-rehome-selfupdate-from-mcplib.md](0002-MADR-rehome-selfupdate-from-mcplib.md):
  the publish workflow's contract.
* [0003-MADR-remediate-debugging-pass-findings.md](0003-MADR-remediate-debugging-pass-findings.md):
  D5 (one argument per file), D6 (regular files), D8 (no `${{ }}` in
  `run:`).
* [0005-MADR-opt-in-prerelease-channels.md](0005-MADR-opt-in-prerelease-channels.md):
  the tag rule.
* [0008-MADR-enforce-import-rules-with-depguard.md](0008-MADR-enforce-import-rules-with-depguard.md):
  the per-package rule.
* [0010-MADR-remediate-second-debugging-pass-findings.md](0010-MADR-remediate-second-debugging-pass-findings.md):
  the roadmap table, D7 (top-level permissions).
* [0012-MADR-archive-assets-and-macos-codesign.md](0012-MADR-archive-assets-and-macos-codesign.md):
  §3, §4 and §8.
* [0013-PLAN-build-and-stage-release-workflow.md](0013-PLAN-build-and-stage-release-workflow.md).
* pi-go: `docs/decisions/0004-MADR-go-module-architecture.md` ("no
  goreleaser").
