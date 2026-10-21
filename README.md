# go-selfupdate-lib

[![Go Reference](https://pkg.go.dev/badge/github.com/maccavelli/go-selfupdate-lib.svg)](https://pkg.go.dev/github.com/maccavelli/go-selfupdate-lib)

> **Formerly `go-core-lib`.** Up to `v1.4.1` this module was
> `github.com/maccavelli/go-core-lib`, which is now deprecated in favour of this
> path. Releases from `v1.5.0` use this path; the earlier tags are not
> valid versions of it
> ([0009-MADR](docs/decisions/0009-MADR-rename-to-go-selfupdate-lib.md)).

The fleet's self-update library: release discovery, exact assets, SHA-256
integrity and locked replacement of the running binary, with the shared
`update` command and the build stamps it reads. It ships no binary, and it
depends on none of the fleet's other libraries: only the standard library and
`golang.org/x/mod`, `golang.org/x/sys` and `golang.org/x/term`. Code that is
not self-update belongs in another module.

Module: `github.com/maccavelli/go-selfupdate-lib`

**Documentation:** [docs/](docs/README.md) ·
**Changes:** [CHANGELOG.md](CHANGELOG.md) ·
**Security:** [SECURITY.md](SECURITY.md)

## Status

From `v1.17.2` the module requires Go 1.26.10; `v1.16.0` to `v1.17.1`
require 1.26.9, and `v1.15.2` and earlier 1.27.1. It is built and tested
with Go 1.27.3 (its `toolchain` line), and CI also tests it with 1.26.10;
neither standard library had a published advisory on 2026-10-21
([0029-MADR](docs/decisions/0029-MADR-move-toolchain-to-go-1-27-3-and-floor-to-1-26-10.md)). Build your program with one of them. The current release is `v1.17.2`. `v1.5.0` was
the first under this path:

```bash
go get github.com/maccavelli/go-selfupdate-lib@v1.17.2
```

| Package | What it does |
| :--- | :--- |
| [`selfupdate`](selfupdate/) | GitHub Releases discovery, exact assets, SHA-256 integrity, locked replacement of the running binary |
| [`selfupdate/cli`](selfupdate/cli/) | the canonical `update` command: flags, stdout for protocol output only, exit codes 0, 10 and 1 |
| [`buildinfo`](buildinfo/) | the build stamps that say whether a binary is a release |
| [`selfupdate/selfupdatetest`](selfupdate/selfupdatetest/) | test doubles: release fixtures, a fake source, a fake GitHub API; and `VerifyReleaseSource`, the conformance check for a release source of another host |
| [`selfupdate/archive`](selfupdate/archive/) | updating from a release asset that is a tar.gz, zip or gz holding the program |
| [`selfupdate/codesign`](selfupdate/codesign/) | opt-in macOS re-signing of the staged binary, and signature checks, with `/usr/bin/codesign` |
| [`selfupdate/verify/ghattest`](selfupdate/verify/ghattest/) | opt-in check that a release was built by its workflow, with `gh attestation verify` |
| [`selfupdate/verify/signednote`](selfupdate/verify/signednote/) | opt-in check that a release's `SHA256SUMS` was signed by a key the program embeds, as a signed note |
| [`selfupdate/releasespec`](selfupdate/releasespec/) | the release spec a program embeds and the build workflow reads: products, platforms, packaging, extras, channels, installers |
| [`selfupdate/service`](selfupdate/service/) | what the service lifecycles share: health polling, typed errors, and the handoff of an update started inside the service |
| [`selfupdate/service/systemd`](selfupdate/service/systemd/), [`launchd`](selfupdate/service/launchd/), [`scm`](selfupdate/service/scm/), [`taskscheduler`](selfupdate/service/taskscheduler/) | the managed-update lifecycle for a systemd unit, a launchd job, a Windows service and a per-user Task Scheduler task |

`selfupdate` and its release workflow come from `mcplib` `v1.6.0`, with the
same API
([0002-MADR](docs/decisions/0002-MADR-rehome-selfupdate-from-mcplib.md)).
Before the first release, a debugging pass fixed 42 findings in both
([0003-MADR](docs/decisions/0003-MADR-remediate-debugging-pass-findings.md)).
Among them: updating a running Windows program no longer fails with
"Access is denied", and a failed rollback is no longer reported as a clean
failure. The behaviour a consumer can see is listed in the
[migration guide](docs/guides/migrating-from-mcplib-selfupdate.md#behaviour-you-may-notice).

## Self-update

`selfupdate` discovers GitHub Releases, selects the exact asset
`<product>-<goos>-<goarch>[.exe]`, checks it against `SHA256SUMS` (and the
GitHub digest when present), and replaces the running binary under a lock.
It proves release-asset integrity, not publisher signature authenticity,
unless the program opts in to `selfupdate/verify/signednote`, which checks
a signed release statement over `SHA256SUMS`.

`selfupdate` never reads flags or calls `os.Exit`. Only
`service.HandOffHop` and `service.HandOffHopContext` exit, and only in a
hand-off hop; `LoadHandOffEnv`, `LoadHandOffEnvContext` and `ReportFunc`
call one of them first, so they exit in a hop too. The program binds:

- a **source** (`NewGitHubSource`, with a required product/version
  `User-Agent`; `GH_TOKEN` or `GITHUB_TOKEN` is used when set),
- a **version policy** (`NewStrictVersionPolicy`: strict `vMAJOR.MINOR.PATCH`;
  `NewSemverPolicy` adds opt-in prerelease channels),
- an **asset selector** (`NewExactAssetSelector`),
- an **installer** (`StandaloneInstaller` for a plain binary;
  `ManagedInstaller` when a service must stop and start around the replace),
- a **reporter** and a **confirmer** (`NewTextReporter`,
  `NewTerminalConfirmer`).

### Quickstart

`NewGitHubUpdater` binds all of those, with their safe defaults, for a
standalone program released on GitHub with exact asset names:

```go
func update(ctx context.Context) int {
    id := buildinfo.Identity()
    u, err := selfupdate.NewGitHubUpdater(selfupdate.GitHubUpdaterOptions{
        Repository: selfupdate.Repository{Owner: "<owner>", Name: "<repo>"},
        Product:    "<product>",
        Version:    id.Current(),
    })
    if err != nil {
        fmt.Fprintln(os.Stderr, err)
        return 1
    }
    build := selfupdate.LocalBuild
    if id.Kind == buildinfo.KindRelease {
        build = selfupdate.ReleaseBuild
    }
    res, err := u.Run(ctx, selfupdate.Request{Product: "<product>", CurrentVersion: id.Current(), CurrentBuild: build})
    return selfupdate.ExitCode(res, err)
}
```

For the full `update` command, with its flags and `--json`, use
`selfupdate/cli`'s `Command` instead.

`ExitCode` maps a run to the process status: 0 when there is no error
(already current, declined, or applied), 10 when a check finds an update,
and 1 for any other error. `selfupdate/example_test.go` shows a complete
standalone binding, a managed-service binding, and `ExitCode`.

An exact `--version` is pinned: a source that returns another tag is an
`ErrIntegrity` failure. Redirects must stay on HTTPS; a plain-http hop is
allowed only to a loopback host, from an API on loopback. End of input at
the confirmation prompt is a decline. On Windows the library never starts
a `.bat` or `.cmd` program, in any spelling: the version probe, the
default runner (and so `ghattest` and the exec reconciler) and
the service handoff refuse it with an error that wraps
`selfupdate.ErrBatchFile`.

### Building and publishing releases

Two reusable workflows, both pinned to the full SHA of a tag commit of
this repository:

- **`build-selfupdate-release.yml`** builds every product for every
  platform from the program's release spec (`selfupdate-release.json`,
  read by `selfupdate/releasespec`) with a fixed recipe. It checks each
  binary from its own build information, runs it on its platform's runner
  when the spec names an identity command, packs archives when the spec
  says so, writes `SHA256SUMS`, and uploads the staged set. When the spec
  has `installer`, it renders `install.sh` and `install.ps1` into the
  release from this repository's tested templates. Off a tag it
  rehearses, stamped local. Its outputs are the publish workflow's inputs.
  See [the building guide](docs/guides/building-releases.md).
- **`publish-selfupdate-release.yml`** accepts only a complete staged set
  that matches the declared products, platforms and extras, refuses a tag
  that already has a release (drafts included), attests the files, and
  publishes an immutable release. It never `--clobber`s. It is the only
  supported publication path for the asset contract. Before the first
  release, turn on immutable releases for the repository (Settings →
  General → Releases → "Enable release immutability"); attestations need
  a public repository or GitHub Enterprise Cloud.

The publish workflow's `SHA256SUMS` check is the client's own parser,
ported, so it never publishes a manifest the client cannot read. Extra asset
names must match `^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`, and staged entries
must be regular files. A `format` (`tar.gz`, `zip` or `gz`) on every
platform object makes the release one of archives, which it unpacks with the
client's own unpacker before publishing. The optional
`prerelease-channels-json` names the channels it may publish
`vX.Y.Z-NAME.N` prereleases for, which never become the latest release; the
default `[]` publishes stable tags only. A stable tag lower than the
current latest, such as a backport, is published without becoming the
latest release either, so clients keep seeing the newest one.

With `sign: true`, `note-verifier-key` and the `SELFUPDATE_NOTE_KEY`
secret, the publish workflow also signs `SHA256SUMS` with a release
statement, `SHA256SUMS.note`, which `selfupdate/verify/signednote`
checks; signing is opt-in
([the building guide, §13](docs/guides/building-releases.md#13-sign-a-release-opt-in)).
`SHA256SUMS`, `SHA256SUMS-*` and `SHA256SUMS.note` are reserved, and no
extra may take one of those names.

A program that stages its own release calls the publish workflow alone:

```yaml
release:
  permissions:
    contents: write
    id-token: write
    attestations: write
  uses: maccavelli/go-selfupdate-lib/.github/workflows/publish-selfupdate-release.yml@036289ecc701d84c7749f4950a57a154c1af2cf4 # v1.17.2
  with:
    artifact-name: <the uploaded artifact holding the staged release>
    products-json: '["<product>"]'
    platforms-json: '[{"os":"linux","arch":"amd64"},{"os":"windows","arch":"amd64"}]'
    extra-assets-json: '[]'
    # Optional; for prereleases tagged vX.Y.Z-rc.N or vX.Y.Z-beta.N:
    # prerelease-channels-json: '["rc","beta"]'
```

#### When a publish fails

A re-run is refused while any release for the tag exists, drafts included.

- **Failed after "Create a draft release" and before "Publish the draft"**
  (in the upload or the attestation): the release is still a draft. Delete
  it, then re-run the job. A failure before "Create a draft release" leaves
  nothing to delete.

  ```sh
  gh release delete <tag> --repo <owner>/<repo> --yes
  ```

- **Failed after "Publish the draft"** (waiting for immutability): the
  release is published, and immutable once GitHub marks it so. With
  immutable releases on, its tag can never be reused, even if the release
  is deleted. Fix forward with a new patch tag.
- **Timed out waiting for immutability, with the setting off:** the release
  is live and mutable. Delete it, turn on immutable releases, and re-run
  all jobs.

  ```sh
  gh release delete <tag> --repo <owner>/<repo> --yes
  ```

## I want to…

| I want to… | Start here |
| :--- | :--- |
| see what is in this repository today | [architecture.md](docs/architecture.md) |
| move a program from `mcplib/selfupdate` to this module | [the migration guide](docs/guides/migrating-from-mcplib-selfupdate.md) |
| offer a beta or rc channel | [the extending guide](docs/guides/extending-selfupdate.md#offer-a-beta-channel) |
| build and publish my program's releases from one spec | [the building guide](docs/guides/building-releases.md) |
| know why `selfupdate` moved here, and what changed on the way | [0002-MADR](docs/decisions/0002-MADR-rehome-selfupdate-from-mcplib.md) |
| know why the repository is set up the way it is | [0001-MADR](docs/decisions/0001-MADR-scaffold-shared-go-library.md) |
| know why it was renamed from `go-core-lib` | [0009-MADR](docs/decisions/0009-MADR-rename-to-go-selfupdate-lib.md) |
| contribute: checks, records and commit rules | [AGENTS.md](AGENTS.md) |

## License

Licensed under the Apache License, Version 2.0. See [LICENSE](LICENSE).
