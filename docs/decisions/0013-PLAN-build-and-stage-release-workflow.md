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
     *(Deviation D1, 2026-10-05: B1 covers `binary` specs. The `archive`
     specs join this test in B3, when the verifier learns `format`.)*
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
   * ~~the verifier parity input dropping `format`~~ (moved to B3, D1);
   * ~~an `os/exec` import planted in the package, which `make lint` must
     refuse with the rule's message~~ (D2): an import of
     `selfupdate/service`, used, planted in the package, which
     golangci-lint must refuse with the rule's message.

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
   * `continue-on-error: true` on the archive check;
   * from B1 (D1): `PlatformsJSON` dropping `format`, which the archive
     cases of `TestVerifierParity` must catch.

   Each must fail the script tests, the shape test or the parity test.
6. **`TestVerifierParity` gains the `archive` specs** (D1).

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

### Phase B1: `selfupdate/releasespec` (2026-10-05)

* **The package** (`doc.go`, `spec.go`, `validate.go`), as MADR §3 with
  the four publish-input methods. Beyond the MADR's text:
  * `Platform.Target()` converts an entry to a `selfupdate.Platform`;
    `MaxSize` and `TagPlaceholder` are exported constants.
  * `Parse` also walks the JSON and refuses a duplicate key, or a key
    that is not lowercase letters and underscores. `encoding/json`
    matches keys case-insensitively and keeps the last of two, so
    `"Schema"` or a repeated key would otherwise pass silently.
  * Product names, extras and the extras' clash with canonical assets
    are compared ignoring case, as the archive unpacker compares member
    names. "Unique" in MADR §2 is read that way; no rule is looser.
  * An empty optional list is normalized to nil, so an accepted spec
    re-encodes to an equal value.
* **Tests** (`spec_test.go`, `parity_test.go`): 3 accepted fixtures and
  62 refused cases, one per rule, each asserting the field in the error;
  the methods; the selector and unpacker matching on every platform;
  the publish inputs, exactly.
* **`TestVerifierParity`** runs `verify-selfupdate-release.sh` on a
  release staged from each `binary` fixture (D1), and on the input each
  of 7 refused specs would have produced. Each refusal must carry the
  verifier's own message, not a usage error.
  * **On the Windows test host** it first failed: Git's `sh.exe` parses
    a command line by other rules than the ones Go quotes it by. A probe
    showed `["relay"]` arrive as `[\relay"] --extras [] --dir C:tmpa`.
    The test now passes the inputs in the environment, and `sh -c`
    expands them into the verifier's arguments.
  * The refused cases had passed there on the usage error, a false
    pass, which is why each now requires its message.
  * Windows: 96 passes, no skips. macOS: all pass, with
    `SELFUPDATE_REQUIRE_PYTHON=1`.
* **`FuzzParse`** (round trip, publish inputs, selector): 20 s ran
  3,164,214 executions clean. `make fuzz` runs it (`-m 1`), and CI's
  fuzz-corpus artifact includes its directory.
* **`ExampleParse`** embeds `testdata/archive.json` and prints each
  target's asset name and `packed: true`.
* **depguard:** `selfupdate-releasespec` allows `$gostd`, `selfupdate`
  and `selfupdate/archive`; `other-packages` excludes the directory.
* **Plants,** each in a scratch copy, each caught:

  | Plant | Caught by |
  | :--- | :--- |
  | unknown fields accepted | `TestParseRefuses` |
  | trailing data accepted | `TestParseRefuses` |
  | duplicate key accepted | `TestParseRefuses` |
  | schema 2 accepted | `TestParseRefuses` |
  | duplicate platform accepted | `TestParseRefuses` |
  | product names compared with case | `TestParseRefuses` |
  | empty `identity_args` accepted | `TestParseRefuses` |
  | `format` under `binary` accepted | `TestParseRefuses` |
  | Windows defaulting to tar.gz | `TestFormatFor` |
  | `{tag}` not expanded | `TestParseAccepts` |
  | `AssetSelector` always exact | `TestSelectorAndUnpackerMatch` |
  | `Unpacker` always set | `TestSelectorAndUnpackerMatch` |
  | extras allowing `SHA256SUMS` | `TestVerifierParity` |
  | `PlatformsJSON` uppercasing `os` | `TestVerifierParity` |
  | a usage error in the verifier call | `TestVerifierParity`: `the verifier failed, but not with "invalid product": exit status 2` |
  | a used `selfupdate/service` import (D2) | depguard: "is not allowed from list 'selfupdate-releasespec'" |

* **Checks:**
  * `gate.sh`: every step rc 0: gofmt, lint on three GOOS, vet, race,
    shuffle, tidy, apicheck, fuzz (9 targets), vuln, the script tests,
    cross vet. It ran twice; the second run followed the parity test's
    Windows fix.
  * `make pre-add-check` on the seven Go files: "7 file(s) clean".
  * `make apicheck`: `compatible with v1.8.0`. `go.mod` is unchanged.

### Phase B2: `internal/cmd/selfupdate-release` (2026-10-05)

* **The command,** one file per subcommand, each a flag-parsing shim over
  a function the tests call: `plan.go`, `build.go`, `stage.go`,
  `pack.go`, `check.go`, `identity.go`; `gotool.go` runs the go command;
  `gha.go` writes step outputs and the summary. Beyond the steps as
  written:
  * **The go command's environment** is built, not inherited: the
    recipe's `GOENV=off`, `GOTOOLCHAIN=local`, `GOFLAGS=-mod=readonly`,
    `CGO_ENABLED=0` and `GOWORK=off`, then only `PATH`, the home, temp
    and cache locations, and the module proxy settings. `GOENV=off`
    keeps a user's go env file out: it could otherwise set `GOAMD64` or
    `GOARM64`, which the recipe leaves at their defaults.
  * **`stage` takes `-stamp-version`,** which replaces `{tag}` in the
    extras' names, so a rehearsal stages them too.
  * **The tag check at the root** compares the module and source
    directories with `os.SameFile`.
  * **Every write is checked:** `printf` returns the write's error, and
    `run` returns the exit code with the stderr line.
* **The fixture** (`testdata/fixture/`): `cmd/relay` prints
  `buildinfo.Identity()` for `version`, prints nothing for `silent`, and
  sleeps for `hang`; `cmd/nostamp` imports no `buildinfo`; the two specs
  name relay only; `notes.txt` is the binary spec's extra. It has no
  `go.sum` (D3).
* **Tests** (80 passes on each OS): the fixture is copied into a new
  git repository, tagged `v1.2.3` (annotated), and built once for
  linux/amd64, darwin/arm64, windows/amd64 and the host; tests copy its
  output before changing it.
  * `build`: every target named and stamped, the main module version the
    tag; `nostamp` refused before anything is written; an unknown
    target, kind, a version with a space, an occupied output refused;
    `GOFLAGS`, `CGO_ENABLED` and a go env file ignored, with a check that
    the planted env file does take effect for a go command that reads
    it; build tags recorded and accepted by `stage`.
  * `stage`: raw and archive releases; byte-identical on a second run;
    `SHA256SUMS` sorted and parsed back; refused: a wrong revision, a tag
    the build was not made at, a swapped platform, an empty file, a file
    that is not a Go binary, a missing binary, a bad SHA, a dirty tree;
    extras from the repository and the extras directory, and refused
    when missing, stray or a symlink.
  * `checkBuildInfo`, on edited build information: cgo, no `-trimpath`,
    build tags, modified, another arch, toolchain, module or package, a
    dirty version.
  * `pack`: the tar, zip and gzip headers field by field; repeatable;
    an unknown format refused; a truncated archive and a changed byte
    caught by the round trip.
  * `check`: good tar.gz, zip and gz; refused: the program twice, the
    program as a symlink, a truncated gzip, a script; a raw release is a
    no-op.
  * `identity`: the raw and the packed program report
    `v1.2.3 (release) <sha12>`; refused: another version, no output, the
    wrong output, a hang (1 s timeout), another platform, no arguments,
    a path as the asset, the wrong kind. The pattern refuses `-dirty`,
    a prefix, a suffix, another revision.
  * `plan`: tag and rehearsal modes, names, extras, the identity matrix
    and tool targets, the summary rows; refused: an unlisted channel, a
    loose tag, a short SHA, attempt 0, a bad artifact name. Outputs are
    parsed back as the runner parses them, including a value holding
    `EOF`, a delimiter prefix and a `name<<x` line.
  * Exit codes: 2 for usage errors, 1 for a failed check.
* **The host runs found two test defects,** both fixed in the tests:
  * **Windows:** Git for Windows cannot open `NUL` as
    `GIT_CONFIG_GLOBAL` ("unable to access 'NUL'"); the helper now gives
    git an empty configuration file.
  * **Linux:** flipping an ELF binary's last byte broke its section
    header table, so the unpacker's image check refused it before the
    byte comparison the test meant to reach; the test now flips a byte
    in the middle.
* **Lint** asked for constants for the stamp kinds and `windows`, a
  string key for the runner table, and every `fmt.Fprintf` checked. One
  `nolint:gosec` per file read or command run, each with its reason.
* **Plants,** each in a scratch copy, each caught by its test:

  | Plant | Caught by |
  | :--- | :--- |
  | layer 1 skipped | `TestBuildRefusesAProgramWithoutBuildinfo` |
  | the `vcs.modified` check removed | `TestStageRefusesADirtyTree` |
  | the `-trimpath` check removed | `TestCheckBuildInfo` |
  | the revision check removed | `TestStageRefuses` |
  | the GOOS check removed | `TestStageRefuses` (the image check then refuses it, with another message) |
  | the tag check at the root removed | `TestStageRefuses` |
  | a tar packer that changes one byte | `TestStageArchivesRoundTripAndRepeat`: "the client's unpacker refuses it" |
  | `SHA256SUMS` sorted in reverse | `TestStageRaw` |
  | the zip mode 0644 | `TestPackMetadata` |
  | the identity pattern a prefix match | `TestIdentityPattern` |
  | a fixed output delimiter, `EOF` | `TestWriteOutputs`: "output tricky holds its delimiter" |
  | `GOENV=off` dropped | `TestBuildIgnoresTheCallersGoEnvironment`: `GOAMD64="v3"` |
  | `GOFLAGS` inherited | `TestBuildIgnoresTheCallersGoEnvironment`: `-tags="evil"` |
  | stray extras accepted | `TestStageExtras` |
  | a symlinked extra accepted | `TestStageExtras` |
  | identity on another platform | `TestIdentity` |
  | `check` skipping archives | `TestCheck` |
  | the tag rule skipped | `TestPlanRefuses` |
  | identity legs for products without `identity_args` | `TestPlanTag` |
  | usage errors exiting 1 | `TestRunExitCodes` |

  Three plants first failed to compile and one was missed; none of
  those counted. The compile failures were rewritten to compile. The
  miss, `GOENV=off` dropped, showed that the test set `GOENV` to its
  file, which the tool never inherits, so the plant changed nothing it
  could see; the test now places the file where a go command looks by
  default, under a new home, and the plant is caught.
* **Checks:**
  * the tool's tests on macOS, the Linux test host and the Windows test
    host: 80 passes each, no skips;
  * `gate.sh`: every step rc 0: gofmt, lint on three GOOS, vet, race,
    shuffle, tidy, apicheck (`compatible with v1.8.0`), fuzz (9
    targets), vuln, the script tests, cross vet;
  * after D4, every script test and shellcheck again;
  * `make pre-add-check` on the 17 Go files, the fixture's two included:
    "17 file(s) clean"; and with no arguments: "257 file(s) clean".
  * The 20 plants above were run again after the host fixes: all
    caught, each at a test assertion.

### Phase B3: the publish workflow learns archives (2026-10-05)

* **`verify-selfupdate-release.sh`:**
  * a platform object has `os` and `arch`, and may have `format`
    (`tar.gz`, `zip` or `gz`), on every object or none; an unknown
    format or another key is refused with its own message;
  * in a packed release the canonical asset is
    `<product>-<os>-<arch>.<format>`, and `SHA256SUMS` must list exactly
    those archives ("…exactly the canonical archives");
  * in a packed release an extra may not take a canonical asset's name,
    in any case (D5: packed releases only);
  * `--github-output FILE` appends `packed=true` or `packed=false` after
    a successful verification, and nothing on a failure; an empty file
    name is a usage error.
* **`publish-selfupdate-release.yml`:** "Validate the staged file set"
  has `id: verify` and passes `--github-output "$GITHUB_OUTPUT"`. Then,
  under `if: steps.verify.outputs.packed == 'true'`, "Set up Go for the
  archive check" (setup-go v7.0.0 from the tools' `go.mod`, cache off)
  and "Check archived programs" (`go run
  ./internal/cmd/selfupdate-release check` in the tools checkout, with
  `GOTOOLCHAIN=local` and the inputs from `env:`). The `platforms-json`
  input's description names `format`. actionlint and
  `check-workflows.sh` (all rules) pass.
* **`scripts/workflow-shape_test.sh`** (new; PyYAML), run by CI's
  "Verify the workflow contract" step: the tag check first after the
  tools checkout; the verifier's id and output flag; both Go steps after
  the verifier and before the draft, guarded by exactly the packed
  output, without `continue-on-error`; the check step runs
  `selfupdate-release check`.
* **Tests:**
  * `verify-selfupdate-release_test.sh` gains 16 checks (69 in all): a packed
    release in all three formats; a format on some platforms only,
    staged to match so only that rule can refuse it; an unknown format;
    another key; a raw binary where an archive is declared; raw names in
    a packed `SHA256SUMS`; an archive name, and the same in another
    case, as an extra; a raw release with an extra named like a binary,
    accepted as before (D5); the output flag for a packed, a raw and a
    failed run; the flag without a file. The new refusals assert the
    verifier's own message (`run_refused`), not just a failure.
  * **The verifier fixtures from before 0013** (53 cases, the test file
    at `HEAD`) pass unchanged against the new verifier, in a scratch
    copy: a caller passing today's inputs gets today's behaviour (V3).
  * `TestVerifierParity` gains `archive.json` (D1), and two refusals
    both sides make: an unknown archive format, and an extra named like
    an archive. It passes on macOS and on the Windows test host.
* **Plants,** each in a scratch copy, each caught:

  | Plant | Caught by |
  | :--- | :--- |
  | mixed formats accepted | `a format on some platforms only (expected exit 1 …, got 0 …)` |
  | a Windows zip keeping `.exe` | `packed release: tar.gz, zip and gz` |
  | the packed output always `false` | `packed output is [packed=false]` |
  | the packed output written before the checks | `packed output is [packed=True …` |
  | the packed output written on the failure path | `a failed verification wrote [packed=true]` |
  | extras allowed a canonical name | `an archive name listed as an extra` |
  | the extra rule applied to raw releases (D5) | `raw release: an extra named like a binary is accepted, as before` |
  | the archive check moved after the draft | shape: `the archive check runs after the verifier and before the draft` |
  | `continue-on-error` on the archive check | shape: `the archive check has no continue-on-error` |
  | the archive check guarded by `always()` | shape: `the archive check runs exactly when the release is packed` |
  | the verifier called without the output flag | shape: `the verifier writes its packed output` |
  | `PlatformsJSON` dropping `format` (D1) | `TestVerifierParity`: `the verifier refused a release built from archive.json` |

  The first draft of the new fixtures used `run_fail`, which passes on
  any failure; two of them failed for a reason other than the rule they
  named (the mixed case's file set did not match). They now assert the
  message, and the mixed case is staged to match.
* **Checks:**
  * `gate.sh`: every step rc 0: gofmt, lint on three GOOS, vet, race,
    shuffle, tidy, apicheck, fuzz, vuln, the script tests, cross vet;
    the script tests again after D5;
  * shellcheck on the three scripts; actionlint; `check-workflows.sh`;
  * `make pre-add-check` on `parity_test.go`: "1 file(s) clean".

### Deviations

* **D1 (2026-10-05), B1: verifier parity for archive specs.**
  * **Found:** B1 step 1's `TestVerifierParity` feeds each accepted
    spec's publish inputs to `verify-selfupdate-release.sh`. The verifier
    refuses any platform object with a `format` key ("platform objects
    must have only os and arch", `scripts/verify-selfupdate-release.sh:113`).
    That is by design until B3, so B1 depended on B3.
  * **Decision (the owner):** split the parity test. B1 covers `binary`
    specs; B3 adds the `archive` specs to the same test, and the plant
    "`PlatformsJSON` dropping `format`" moves to B3. Nothing is skipped.
  * **Files:** none added; B3's step list gains step 6.
* **D2 (2026-10-05), B1: the depguard plant.**
  * **Found:** B1 step 6 planted an `os/exec` import to prove the
    `selfupdate-releasespec` rule. The rule allows `$gostd`, as every
    per-package rule here does, so `os/exec` is allowed. A blank import
    was caught by revive's `blank-imports`, and a used one passed lint
    with "0 issues": neither tests the rule.
  * **Decision:** the plant imports a module package the rule does not
    allow, `selfupdate/service`, and uses it. golangci-lint then fails
    with "import 'github.com/maccavelli/go-selfupdate-lib/selfupdate/service'
    is not allowed from list 'selfupdate-releasespec' (depguard)". The
    rule and the MADR are unchanged; only the plant was wrong.
  * **Files:** none.
* **D3 (2026-10-05), B2: the fixture has no `go.sum`.**
  * **Found:** B2's file list names a fixture `go.sum`. `go mod tidy`
    writes none: the fixture imports only `buildinfo`, which imports the
    standard library only, and the module graph is pruned, so no
    checksum is needed. It builds with `GOFLAGS=-mod=readonly`.
  * **Decision:** no `go.sum`. A fixture that later imports a package
    with dependencies gets one from `go mod tidy`, and `-mod=readonly`
    fails loudly until it does.
  * **Files:** one fewer than listed.
* **D4 (2026-10-05), B2: the pre-add check and nested modules.**
  * **Found:** `make pre-add-check` on B2's files passed gofmt and lint,
    then failed go vet and go test on the fixture's two Go files: "main
    module (github.com/maccavelli/go-selfupdate-lib) does not contain
    package …/testdata/fixture/cmd/relay". `scripts/go-precheck.sh` ran
    every package in the main module. The limit was there before;
    0013 is the first change to commit Go files in a nested module.
  * **Decision (the owner):** teach the check. go vet and go test now run
    in the module that owns each package, the nearest `go.mod` above it,
    with `go -C <module>` and `GOWORK=off`. With no arguments, `./...` runs
    in the main module and in every nested module holding a tracked Go
    file. A deleted package widens its own module's run to `./...`. Lint
    still runs `./...` in the main module only, which does not reach a
    nested module; the header says so.
  * **Tests** (`go-precheck_test.sh`, 4 new cases, 11 in all): a nested
    module's files are vetted and tested in it; its failing test fails
    the check; the no-argument mode covers every module; a deleted nested
    package falls back to its module's `./...`.
  * **Plants,** each caught by those cases: `module_of` ignoring
    `go.mod`; test failures dropped; the no-argument mode per package; a
    nested run without `-C`.
  * **Files:** `scripts/go-precheck.sh` and `scripts/go-precheck_test.sh`
    join B2.
* **D5 (2026-10-05), B3: the extra-name rule, packed releases only.**
  * **Found:** B3 step 1 lists "an archive name listed as an extra" as
    refused. The first implementation refused an extra named like any
    canonical asset, in raw releases too. For a raw release that changes
    today's behaviour, where such an extra is merged into the binary's
    entry, and MADR §8 promises "A caller that passes today's inputs gets
    today's behaviour".
  * **Decision (the owner):** packed releases only. The MADR's promise
    holds; `releasespec` still refuses the name for both packagings, so
    a release built by the build workflow never has one.
  * **Files:** none added; a raw-release fixture shows the behaviour is
    unchanged.
