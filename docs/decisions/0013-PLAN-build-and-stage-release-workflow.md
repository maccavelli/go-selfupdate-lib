---
status: in-progress
date: 2026-10-05
associated-madr: "0013-MADR-build-and-stage-release-workflow.md"
---
# Implement the build-and-stage release workflow: `selfupdate/releasespec`, the internal release tool, `build-selfupdate-release.yml`, and archive releases through the publish workflow (`v1.9.0`)

Associated MADR: [0013-MADR-build-and-stage-release-workflow.md](0013-MADR-build-and-stage-release-workflow.md)

## Goal

* A program's release is described once, in `selfupdate-release.json`,
  which the program embeds and the workflow reads with the same parser
  (MADR §2, §3).
* `build-selfupdate-release.yml` builds every product for every platform
  with the fixed recipe (MADR §4) and checks each binary in the three
  layers of MADR §5. It packs, writes `SHA256SUMS`, stages and uploads
  (MADR §6, §7). Off a tag it rehearses.
* `publish-selfupdate-release.yml` publishes archive releases, and checks
  them with the library's own unpacker (MADR §8). Every existing call
  behaves as before.
* This repository's CI rehearses the workflow on a fixture on every push,
  and builds it as a release on every `v*` tag.
* `v1.9.0` is tagged. `make apicheck` reports it compatible with
  `v1.8.0`.

## Scope

### In scope

| Phase | Work |
| :--- | :--- |
| B0 | Records: the MADR's answers and acceptance, this PLAN, `docs/README.md` rows, AGENTS.md. No code. |
| B1 | `selfupdate/releasespec`, its `depguard` rule, fuzz target and example |
| B2 | `internal/cmd/selfupdate-release`: `plan`, `build`, `stage`, `check`, `identity`, and the fixture module |
| B3 | `publish-selfupdate-release.yml` and `verify-selfupdate-release.sh`: the `format` key and the archive check |
| B4 | `build-selfupdate-release.yml`, the `pins` rule in `check-workflows.sh`, and the CI rehearsal job |
| B5 | Docs, guide, migration guide §9 and release notes |
| B6 | The release: the owner's push and tag, the pin commit, the live publish rehearsal, the agent's checks |

### Out of scope

* The installer templates (MADR §10). They get their own record.
* Moving any fleet program onto the workflow. That happens in each
  repository, under its own records. B5's guide says how.
* Signing or notarizing macOS builds, an attestation from the build
  workflow, and attest-build-provenance v4 (MADR §10).
* Option B's combined workflow (MADR §12, Q2).
* darwin/amd64. No fixture, test or runner builds or runs it
  (0012-MADR §10). The spec does not refuse it, and the identity run
  reports it as having no runner.
* Push, tags and the throwaway repository of B6, which need the owner's
  ask.

## Rules for every phase

The rules of
[0012-PLAN-archive-assets-and-macos-codesign.md](0012-PLAN-archive-assets-and-macos-codesign.md)
apply:

* Tests first, each seen to fail on a deliberately broken input in a
  scratch copy (`plantcopy.py`), never in the tree.
* The full gate before each commit, `gate.sh`:
  * gofmt, `make lint` on three GOOS, vet, race, shuffle and cross vet;
  * `make fuzz`, `make vuln` and `go mod tidy -diff`;
  * the script tests and `make apicheck`.
* `make pre-add-check FILES=…` on staged Go files. markdownlint on a copy
  of each record with the repository's configuration; MD004 (`*`
  markers) is expected there. The link, anchor and identifier check
  (`doccheck.py`).
* One commit per phase, staged by the agent and committed by the owner
  with `git commit --no-edit`.
* A dated deviation entry, and a MADR amendment where a decision or an
  asserted fact changes, before continuing past any surprise.

In addition:

* **API:** `make apicheck` reports `compatible with v1.8.0` at every
  phase. Nothing exported changes outside the new package.
* **Modules:** `go.mod` and `go.sum` do not change. The fixture module
  has its own `go.mod` and `go.sum` under `testdata/`, outside the main
  module.
* **The existing suite** passes unmodified, apart from tests that gain
  cases.
* **Workflows:**
  * every `uses:` of an action is pinned to a full commit SHA, with the
    version in a comment. They are the SHAs this repository already
    pins: checkout `3d3c42e…` v7.0.1, setup-go `b7ad1da…` v7.0.0,
    upload-artifact `043fb46…` v7.0.1, download-artifact `3e5f45b…`
    v8.0.1;
  * no `${{ }}` inside a `run:` block;
  * top-level `permissions:`;
  * actionlint v1.7.12 and `check-workflows.sh` are clean.
* **Tools in tests:** B2's tests need `go` and `git`, which every CI
  runner and test host has. They fail, not skip, without them.
* **The Windows test host** runs B2's tests, for `.exe` names, zip and PE
  images, and a Linux host runs them for ELF.

## Implementation Steps

### Phase B0: records

1. The MADR gains "### 12. Owner answers (2026-10-05)" with the answers
   to Q1–Q3 verbatim, and the status `accepted`. Q1 is opt-in, after the
   owner's revision; §12 records both answers.
2. This PLAN, amended to the answers, and its row in `docs/README.md`
   beside the MADR's.
3. AGENTS.md:
   * the module description names `selfupdate/releasespec`, and the
     depguard list names `selfupdate-releasespec`;
   * "It is a library only: no packaged binary" gains: "Commands under
     `internal/cmd/` are build and release tools that the workflows run
     from source; they are never released."
4. The owner approves this PLAN before B1.

### Phase B1: `selfupdate/releasespec`

**Files:** `selfupdate/releasespec/{doc.go,spec.go,validate.go}`, and
tests `spec_test.go`, `fuzz_test.go`, `example_test.go`,
`testdata/*.json`; `.golangci.yml`; `Makefile`;
`.github/workflows/ci.yml` (fuzz corpus path).

1. **Tests first** (`spec_test.go`), table-driven over `testdata/` files
   and inline JSON:
   * **accepted:**
     * the MADR §2 example;
     * a single product and platform;
     * `packaging: archive` with and without `format`;
     * `"package": "."`;
     * an extra with `{tag}`;
     * the empty optional fields.
   * **refused,** one case per rule, each asserting the error names the
     field:
     * the JSON shape: unknown field; trailing data; two values; over
       64 KiB;
     * `schema`: 0, 2 or missing;
     * `products`: none, more than 16, a duplicate, a bad name (leading
       `-`, space, `/`, 129 characters);
     * `package`: not `.`/`./`-prefixed; `./../x`; `./a//b`; absolute;
     * `tags` and `identity_args`: a bad tag; more than 16 of either;
       `identity_args` present but empty; an empty or NUL
       `identity_args` entry. A spec without `identity_args` is accepted
       (MADR §12, Q1);
     * `platforms`: none, more than 32, a duplicate, uppercase or a bad
       character in `os`/`arch`;
     * `format`: one under `binary`; an unknown one;
     * `packaging`: an unknown value;
     * `extras`: more than 32; `SHA256SUMS` or `SHA256SUMS-x`; a name
       equal to a canonical asset; a bad name after `{tag}`; a duplicate
       after `{tag}`; a `path` with `..`, absolute or empty segments;
     * `prerelease_channels`: as `check-release-tag.sh` refuses them:
       bad name, not strictly descending, duplicate.
   * **methods:**
     * `Product` for a listed and an unlisted name;
     * `Targets` in spec order;
     * `FormatFor` defaults: zip on windows, tar.gz elsewhere, an
       explicit `gz`;
     * `AssetName` equals `selfupdate.ExactAssetName` or
       `archive.FleetName` for every case;
     * `AssetSelector` and `Unpacker` are of the matching kind: a
       selection from a `selfupdatetest` release is `Packed` exactly when
       `Unpacker` is non-nil;
     * `ExtraNames("v1.2.3")` expands `{tag}`.
   * **Parity with the publish verifier** (`TestVerifierParity`): for
     every accepted fixture, the inputs `ProductsJSON`, `PlatformsJSON`,
     `ExtrasJSON(tag)` and `ChannelsJSON` produce are accepted by
     `verify-selfupdate-release.sh` on a staged directory the test
     builds. For selected refused fixtures, the matching verifier input
     is refused. It needs `python3`, and is mandatory under
     `SELFUPDATE_REQUIRE_PYTHON=1`, as `TestManifestDifferential` is.
2. **The package**, as MADR §3, including the four publish-input
   methods. `Parse` uses `json.Decoder` with `DisallowUnknownFields` and
   refuses a second value. Errors are
   `releasespec: <field path>: <reason>`. The JSON methods emit compact
   JSON, with keys in a fixed order, so the outputs are stable.
3. **`FuzzParse`:** `Parse` never panics; an accepted spec re-encodes
   and re-parses to an equal value; `Validate` agrees with `Parse`. Seeds
   are the `testdata/` files. `Makefile` adds
   `./scripts/go-fuzz.sh -t $(FUZZTIME) -m 1 ./selfupdate/releasespec`,
   and CI's fuzz-corpus artifact adds `selfupdate/releasespec/testdata/fuzz/`.
4. **`ExampleParse`** embeds a spec from `testdata/` and prints the
   targets and asset names (with `// Output:`).
5. **depguard:** a rule `selfupdate-releasespec` for
   `**/selfupdate/releasespec/*.go`, not tests, allowing `$gostd`,
   `selfupdate` and `selfupdate/archive`. `other-packages` excludes the
   directory.
6. **Plants** (each in a scratch copy, each must fail its test):
   * unknown fields accepted;
   * `schema` 2 accepted;
   * a duplicate platform accepted;
   * `format` under `binary` accepted;
   * windows defaulting to tar.gz;
   * `{tag}` not expanded;
   * `AssetSelector` always exact;
   * the extras rule allowing `SHA256SUMS`;
   * the verifier parity input dropping `format`;
   * an `os/exec` import planted in the package, which `make lint` must
     refuse with the rule's message.

### Phase B2: `internal/cmd/selfupdate-release`

**Files:**

* `internal/cmd/selfupdate-release/`:
  * `main.go` (subcommand dispatch, `flag` per subcommand);
  * `plan.go`, `build.go`, `stage.go`, `pack.go`, `check.go`,
    `identity.go`, `gha.go` (writing `GITHUB_OUTPUT` and the step
    summary);
  * tests: `*_test.go`;
* the fixture `testdata/fixture/`:
  * `go.mod`, module `example.com/relay`, requiring this module through
    `replace github.com/maccavelli/go-selfupdate-lib => ../../../../..`;
  * `go.sum`;
  * `cmd/relay/main.go`, which prints `buildinfo.Identity().String()`
    for the argument `version`;
  * `cmd/nostamp/main.go`, which imports no `buildinfo`. No committed
    spec names it; the tests write specs that do;
  * `selfupdate-release.json` (relay only, `binary`) and
    `selfupdate-release.archive.json` (relay only, `archive`), used by B4's
    CI job.

**Subcommands.** Each validates its flags, writes errors as
`selfupdate-release <subcommand>: …`, and exits 0, 1 (a check failed) or
2 (usage).

* **`plan`:**
  * Flags: `-spec`, `-ref-type`, `-ref-name`, `-sha`, `-run-attempt`,
    `-artifact-name`, `-github-output`, `-summary`.
  * Parses the spec, and picks tag or rehearsal mode from `-ref-type`.
  * Writes the outputs of MADR §7:
    * `tag`, `rehearsal`;
    * `stamp-version`, `stamp-kind`;
    * `artifact-name`;
    * `products-json`, `platforms-json`, `extra-assets-json`,
      `prerelease-channels-json`;
    * `identity-matrix` (objects `{product, os, arch, runner, asset}`);
    * `tool-targets`.
  * Uses the runner table of MADR §5. Writes `name<<EOF` blocks with a
    random delimiter, never the raw value on one line.
* **`build`:**
  * Flags: `-spec`, `-module-dir`, `-stamp-version`, `-stamp-kind`,
    `-out`.
  * Checks `go env GOVERSION` and `go tool dist list`.
  * Then, per product and platform:
    1. runs `go list -deps` with the platform's GOOS and GOARCH, and
       requires `…/buildinfo`;
    2. runs `go build` exactly as MADR §4, through `os/exec` with an
       explicit environment: the inherited `PATH`, `HOME`, `GOPATH`,
       `GOCACHE`, `GOMODCACHE` and `TMPDIR`, plus the recipe's
       variables. `GOFLAGS` is set, never inherited.
  * The ldflags come from `buildinfo.VersionVar` and `buildinfo.KindVar`.
* **`stage`:**
  * Flags: `-spec`, `-module-dir`, `-src`, `-bin`, `-out`, `-sha`,
    `-tag`, `-extras-dir`, `-summary`.
  * Runs MADR §5 layer 2 on each binary. The tag-version check runs only
    when the module directory is the repository root.
  * Packs as MADR §6, unpacks each archive with `archive.NewUnpacker`
    and compares the bytes.
  * Writes `SHA256SUMS`, parses it back with `ParseSHA256SUMS`, then
    copies the extras.
  * Refuses a non-regular extra, a missing one and a stray file in the
    extras directory.
* **`check`:**
  * Flags: `-dir`, `-products-json`, `-platforms-json`.
  * For each archive in a `format` release, unpacks it with
    `archive.NewUnpacker` and passes the program to `CheckImage` for its
    platform.
  * Exits 0 with nothing to do when no platform has a `format`.
* **`identity`:**
  * Flags: `-staging`, `-asset`, `-product`, `-os`, `-arch`,
    `-args-json`, `-want-version`, `-want-kind`, `-sha`.
  * Copies or unpacks the asset into a temporary directory and sets mode
    0755.
  * Runs it with a 30-second context, stdin from the null device, and an
    environment of `PATH` plus, on Windows, `SystemRoot`.
  * Matches the first stdout line against MADR §5's pattern. On a
    mismatch, prints the first 4 KiB of stdout and stderr.

**Steps:**

1. **Tests first.** Each test copies the fixture into `t.TempDir()`,
   runs `git init`, commits, and tags when the case needs it.
   * The `replace` path is rewritten to the absolute repository root in
     the copy.
   * A helper builds through the `build` code path; nothing calls the
     workflow.
   * Cases:
     * **`plan`:**
       * a tag and a rehearsal ref;
       * the default and an overridden artifact name;
       * `identity-matrix` has a leg for every product with
         `identity_args` on every platform with a runner. It has none
         for darwin/amd64 or for a product without `identity_args`, and
         the summary names both as not run;
       * the output-file format, with a value that contains a newline.
     * **`build`:**
       * the fixture for linux/amd64, darwin/arm64 and windows/amd64;
       * `nostamp` refused: "does not import …/buildinfo";
       * an unknown pair refused by `dist list`;
       * `GOFLAGS=-mod=mod` in the parent environment has no effect.
     * **`stage`:**
       * raw, then each archive format, round-tripped;
       * byte-identical on a second run;
       * `SHA256SUMS` sorted and parsed back;
       * refused: a dirty tree (an untracked file); a binary built
         without `-trimpath`; one with `CGO_ENABLED=1` (built by the
         test, with the build-info read stubbed where cgo is
         unavailable); a revision that is not `-sha`; a swapped
         GOOS/GOARCH (a windows binary under a linux name); an empty
         file; a symlinked extra; a missing extra; the tag version
         mismatch at the root.
     * **`check`:** good archives; a tar with the program twice; a zip
       with the program as a symlink; a truncated gzip; a raw-format
       release, a no-op.
     * **`identity`:**
       * runs the host-platform fixture: `version` matches, a wrong tag
         does not;
       * a program that prints nothing fails;
       * a hanging program (a fixture flag) times out.
2. **The command,** as above.
3. **Lint:** gosec G204 on `exec.Command` with a variable path is
   expected. The `go` binary is resolved once with `exec.LookPath("go")`
   and the arguments are built from validated values. Any site the
   linter still flags carries a `//nolint:gosec // …` that says so.
4. **Plants:**
   * layer 1 skipped, so `nostamp` must be caught by a test;
   * the `vcs.modified` check removed;
   * the `-trimpath` check removed;
   * the revision check removed;
   * the GOOS check removed;
   * the archive round trip skipped, with a corrupted packer;
   * `SHA256SUMS` unsorted;
   * the zip mode left at 0644 (the unpacker's image check still passes,
     so the planted check is the mode assertion);
   * the identity pattern loosened to a prefix match on the version;
   * the `GITHUB_OUTPUT` writer using a fixed delimiter, with a value
     that contains it.

### Phase B3: the publish workflow learns archives

**Files:** `scripts/verify-selfupdate-release.sh`,
`scripts/verify-selfupdate-release_test.sh`,
`.github/workflows/publish-selfupdate-release.yml`, the new
`scripts/workflow-shape_test.sh`, and `.github/workflows/ci.yml` (which
runs it).

1. **Tests first** (`verify-selfupdate-release_test.sh`):
   * accepted:
     * a `format` release with tar.gz, zip and gz platforms, and
       `SHA256SUMS` listing the archives;
     * every existing fixture, unchanged.
   * refused:
     * `format` on some platforms only;
     * an unknown format;
     * an extra key;
     * a raw binary staged where an archive is declared;
     * `SHA256SUMS` listing the raw binary names in a `format` release;
     * an archive name listed as an extra.
2. **The verifier:**
   * keys ⊆ `{os, arch, format}`, with `os` and `arch` required;
   * `format` ∈ {`tar.gz`, `zip`, `gz`}, on all objects or none;
   * `asset_name` appends the format's extension instead of `.exe`;
   * a new optional `--github-output FILE` appends `packed=true` or
     `packed=false` after a successful verification. The script tests
     cover it for a raw and a packed release, and it writes nothing on a
     failure.
3. **The workflow.** In "Validate the staged file set" (`id: verify`),
   the verifier gains `--github-output "$GITHUB_OUTPUT"`. Then:
   1. **"Set up Go for the archive check,"** under
      `if: steps.verify.outputs.packed == 'true'`: setup-go with
      `go-version-file: .core-lib-release-tools/go.mod` and
      `cache: false`.
   2. **"Check archived programs,"** under the same `if:`: from
      `.core-lib-release-tools`,
      `go run ./internal/cmd/selfupdate-release check -dir "$GITHUB_WORKSPACE/staging" …`,
      with the inputs from `env:`.
4. **A shape test** (`scripts/workflow-shape_test.sh`, Python with
   PyYAML, run in the "Verify the workflow contract" CI step). For the
   publish workflow:
   * both Go steps come after "Validate the staged file set" and before
     "Create a draft release";
   * both are guarded by exactly `steps.verify.outputs.packed == 'true'`;
   * neither has `continue-on-error`;
   * the tag check is still the first check after the tools checkout.
5. **Plants:**
   * mixed `format` accepted;
   * `asset_name` keeping `.exe` for a Windows zip;
   * `--github-output` always writing `packed=false`;
   * `--github-output` writing on a failed verification;
   * the archive check moved after "Create a draft release";
   * `continue-on-error: true` on the archive check.

   Each must fail the script tests or the shape test.

### Phase B4: `build-selfupdate-release.yml` and the CI rehearsal

**Files:** `.github/workflows/build-selfupdate-release.yml`,
`.github/workflows/ci.yml`, `scripts/check-workflows.sh`,
`scripts/check-workflows_test.sh`, `scripts/workflow-shape_test.sh`.

1. **`check-workflows.sh` gains a rule, `pins`:** every step `uses:`
   that names an action (not `./…` and not `docker://`) ends in `@` and
   40 hex characters. A job-level `uses:` of a reusable workflow is
   exempt when it is a local `./` path. `all` includes it.
   * Tests (`check-workflows_test.sh`): a tag pin, a short SHA and a
     branch, each refused; a full SHA and a local path accepted.
   * Plants: the rule disabled; the regex accepting 39 characters.
2. **The workflow,** as MADR §7:
   * **Inputs and outputs:** as the MADR's table.
   * **Permissions:** top-level `contents: read`.
   * **`build`** (`ubuntu-24.04`, `timeout-minutes: 30`):
     1. Tools checkout to `tools/` at `job.workflow_sha`, as the publish
        workflow does.
     2. Source checkout to `src/` at `github.sha`, `fetch-depth: 1`,
        `persist-credentials: false`.
     3. setup-go from `src/<module-dir>/go.mod`, `cache: false`.
     4. `go build -o "$RUNNER_TEMP/bin/selfupdate-release" ./internal/cmd/selfupdate-release`
        in `tools/`, with `GOTOOLCHAIN=local`.
     5. `plan`.
     6. On a tag, `sh tools/scripts/check-release-tag.sh "$TAG" "$CHANNELS_JSON"`.
     7. When `extras-artifact-name` is set, download it to
        `$RUNNER_TEMP/extras`.
     8. `build` to `$RUNNER_TEMP/bin-out`.
     9. `stage` to `$RUNNER_TEMP/staging`.
     10. `verify-selfupdate-release.sh` on the staging directory, with
         `--tag` on a tag.
     11. Cross-compile the tool for each `tool-targets` entry into
         `$RUNNER_TEMP/tools-out`.
     12. Upload `$RUNNER_TEMP/staging` as `artifact-name`, with
         `if-no-files-found: error` and the input retention.
     13. Upload the tools as `<artifact-name>-tools`, with retention 1.
   * **`identity`:**
     * `needs: build` and `if: needs.build.outputs.identity-matrix != '[]'`;
     * `strategy.fail-fast: false`, with a matrix `include` from
       `fromJSON(needs.build.outputs.identity-matrix)`;
     * `runs-on: ${{ matrix.runner }}`, `timeout-minutes: 10`;
     * steps: download both artifacts, mark the tool executable outside
       Windows, run `identity`. Every value reaches `run:` through
       `env:`.
   * Every `run:` uses `shell: bash`, `set -euo pipefail`, and reads
     values from `env:` only.
3. **The CI job, `release-rehearsal`,** in `ci.yml`:
   * `uses: ./.github/workflows/build-selfupdate-release.yml`, with
     `spec-path` and `module-dir` naming the B2 fixture.
   * On a branch or pull request it rehearses. On a `v*` tag it builds
     the fixture as that release, and its identity runs prove
     `v1.9.0 (release) <sha12>` on all five runners.
   * A following job, `release-rehearsal-check` (`needs`, ubuntu), runs
     the publish verifier on the downloaded artifact with the outputs.
     That proves the hand-off without publishing.
   * The fixture spec lists linux/amd64, linux/arm64, darwin/arm64,
     windows/amd64 and windows/arm64, with
     `identity_args: ["version"]`, as `binary`. A second fixture spec,
     `selfupdate-release.archive.json`, gives the same platforms as
     `archive` with a gz platform, and a second call of the workflow
     runs it.
4. **CI's existing checks** cover the new file:
   * actionlint;
   * `check-workflows.sh` (all rules) on the build and publish
     workflows;
   * `--rule expressions`, `permissions` and `pins` on `ci.yml`.
5. **Plants** (in a scratch clone, run through actionlint and
   `check-workflows.sh`, and one through a real CI run on a branch the
   owner pushes, if the owner agrees):
   * a `${{ inputs.spec-path }}` inside `run:` (expressions);
   * no top-level `permissions:` (permissions);
   * a checkout pinned to `@v7` (pins);
   * the source checked out into `tools/src`. B2's dirty-tree test
     covers the behaviour. Here `workflow-shape_test.sh` gains, for the
     build workflow, that the two checkout paths are siblings, that
     setup-go has `cache: false`, and that the identity job `needs` the
     build job, and the plant must fail it;
   * on the branch run, the fixture's `version` printing the wrong
     version. The identity job must fail.

### Phase B5: docs, guide and release notes

1. **A new guide, `docs/guides/building-releases.md`:**
   * writing the spec;
   * embedding it and configuring the updater from it;
   * the caller jobs (MADR §9);
   * rehearsals on pull requests;
   * the identity command, opt-in per product and recommended (MADR
     §12, Q1), with `fmt.Println(buildinfo.Identity())`;
   * adopting archives;
   * the failure modes and their messages;
   * verifying an attestation with
     `gh attestation verify --signer-workflow maccavelli/go-selfupdate-lib/.github/workflows/publish-selfupdate-release.yml`;
   * moving from a Makefile build, including from `main.*` stamps to
     `buildinfo`, and from the version without `v`.
2. **`README.md`:**
   * "Publishing releases" becomes "Building and publishing releases",
     with the two-job caller;
   * the "I want to…" row;
   * the package row for `releasespec`.
3. **`docs/architecture.md`:**
   * the tree;
   * the Go code table with its counts;
   * a "Build workflow" section beside "Release workflow";
   * the publish workflow's new step.
4. **`docs/README.md`:** "I want to…" rows for describing a release once
   and for building it in CI.
5. **The migration guide:** a new "9. From v1.8 to v1.9". Its other
   version references move in B6's pin commit. This follows the standing
   rule that every release bumps the guide.
6. **Release notes for `v1.9.0`,** in this PLAN's execution record.
7. **Checks:** markdownlint, `doccheck.py`, the identifier scan, and
   `gate.sh`.

### Phase B6: the release

1. **The owner** pushes and waits for CI, including `release-rehearsal`
   on all five runners. The owner then tags `v1.9.0` (annotated) and
   pushes it. The tag's CI builds the fixture as `v1.9.0` and runs its
   identity checks.
2. **After the tag,** one commit:
   * moves the workflow pins in `README.md`, the guide and the migration
     guide to the tag's commit;
   * moves the migration guide's `go get` commands and §5's `go.mod`
     step to `v1.9.0`;
   * gives `architecture.md` the tag's commit;
   * marks this PLAN `complete`, if every Verification item holds.
3. **The live publish rehearsal.** The owner creates a throwaway
   *public* repository with immutable releases on, or asks the agent to.
   It holds a small program, written for this step and not committed
   here:
   * a `version` command that prints `buildinfo.Identity()`;
   * an `update` command on `cli.Command`, configured from an embedded
     spec through `releasespec`;
   * MADR §9's caller as its workflow, pinned to `v1.9.0`.
   * The owner tags `v0.0.1` and `v0.0.2` with `packaging: binary`, then
     `v0.0.3` and `v0.0.4` with `packaging: archive`.
   * The agent then checks:
     * each release's asset set, `SHA256SUMS` and immutability;
     * `gh attestation verify <asset> --repo <owner>/<repo> --signer-workflow maccavelli/go-selfupdate-lib/.github/workflows/publish-selfupdate-release.yml`
       on one asset of each release;
     * the downloaded `v0.0.1` binary on each test host:
       * `version` prints `v0.0.1 (release) <sha12>`;
       * `update --check` exits 10, which C2 would have broken;
       * `update --yes` installs `v0.0.2`.
     * the same with `v0.0.3` to `v0.0.4` through the archive selector
       and unpacker;
     * a re-run of the `v0.0.2` build job gives the same SHA-256 for
       every asset.
   * The owner deletes the repository afterwards, or keeps it as a
     standing rehearsal. Either way it is named in the execution record
     only as "the throwaway repository".
4. **The agent's checks:**
   * CI on the tag;
   * the proxy's `@latest`;
   * a scratch consumer requiring `v1.9.0` from the proxy that parses a
     spec and builds for linux, windows and darwin/arm64.

## Verification

* **V1.** Every new test, rule and gate was seen to fail on a
  deliberately broken input, recorded per phase with the failure text.
* **V2.** `make apicheck`: `compatible with v1.8.0`. `go.mod` and
  `go.sum` are unchanged.
* **V3.** The publish workflow is unchanged for existing callers:
  * every existing verifier fixture passes unmodified;
  * a raw release takes no new step (the verifier reports
    `packed=false`, and the Go steps are skipped).
* **V4.** Every rule in MADR §2 has a refusing test, and `FuzzParse`
  runs clean in `make fuzz` and CI.
* **V5.** Every check in MADR §5 layers 1 and 2 has a refusing test.
  Layer 3 failed on a planted wrong version in a real CI run, and passed
  on all five runners for the tag.
* **V6.** Packing is reproducible:
  * two `stage` runs are byte-identical in the tests;
  * a re-run of a live build gives the same digests (B6).
* **V7.** The live publish rehearsal (B6) passes every check listed
  there.
* **V8.** The MADR's "Not verified" entries are each restated with what
  this PLAN established, or left open with the reason:
  * the tag in the shallow clone;
  * annotated tags;
  * the arm64 runners;
  * the re-run artifact name.
* **V9.** CI is green on `main` and on `v1.9.0`.

## Rollout and Rollback

* **Rollout.**
  * The build workflow and `releasespec` are new and opt-in.
  * The publish workflow's changes take effect only for a caller that
    passes `format`.
  * Existing callers pin earlier commits, and see nothing until they
    move their pin. If they then pass today's inputs, they get today's
    behaviour.
* **Rollback.**
  * Before the tag, a phase reverts together with the phases that
    depend on it: B2 depends on B1, B4 on B2 and B3, and B5 on all of
    them. B3 reverts alone.
  * After the tag, fix forward in `v1.9.x`.
  * A caller that meets a defect in the build workflow pins its previous
    publish commit and its own build again. Nothing it published needs
    changing: the releases are immutable, and the client reads them as
    before.

## Execution Record

### Phase B0: records (2026-10-05)

* **Steps 1 and 2.** The owner answered Q1–Q3:
  * the answers are in MADR §12, and the MADR is `accepted`;
  * Q1 was first answered "required", and those amendments were made.
    The owner then revised it: "change q1 to opt-in". MADR §2, §3, §5,
    §7, the Consequences, and this PLAN's B1 refusals, B2 `plan` cases
    and B5 guide are back to opt-in. §5 now states that the job summary
    names every product without an identity run;
  * `docs/README.md` shows the MADR `accepted`.
* **Step 4.** The owner approved this PLAN: "approved. proceed." It is
  `in-progress`.
* **Step 3, AGENTS.md:**
  * the module description names `selfupdate/releasespec` and this
    MADR;
  * "It is a library only: no packaged binary" gains the sentence on
    `internal/cmd/`;
  * the depguard list names `selfupdate-releasespec`.

  The rule itself lands with the package in B1, as each earlier package's
  did.
* **Checks:** markdownlint on copies of the records and on `AGENTS.md`,
  and `doccheck.py` (links and identifiers) on the four files. No code
  changes, so `gate.sh` is not run.
