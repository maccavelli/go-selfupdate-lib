# Migrating from `mcplib/selfupdate` to go-selfupdate-lib

For a program that imports `github.com/maccavelli/mcplib/selfupdate`, or that
publishes its releases through `mcplib`'s `publish-selfupdate-release.yml`.
The package and workflow here are `mcplib` `v1.6.0`'s, with the same Go API
and the fixes of a debugging pass, so the move is mechanical. Why they moved
is in [0002-MADR](../decisions/0002-MADR-rehome-selfupdate-from-mcplib.md);
the fixes, and the behaviour they change, are in
[0003-MADR](../decisions/0003-MADR-remediate-debugging-pass-findings.md).

Do the migration under your own repository's records. This guide is the
checklist, not the authorization.

## 1. Go 1.27.1

go-selfupdate-lib requires `go 1.27.1`, so `go get` raises your `go` directive to
at least that. Move it deliberately first, and run your full test suite at
1.27.1 before changing anything else.

## 2. The Go import

```bash
go get github.com/maccavelli/go-selfupdate-lib@v1.12.0
```

`v1.12.0` is the current release. `v1.6.0` changed a few behaviours that
`v1.5.x` had; they are listed in [6. From v1.5 to v1.6](#6-from-v15-to-v16).
`v1.7.0`, `v1.8.0`, `v1.9.0` and `v1.10.0` only add to the API: see
[7. From v1.6 to v1.7](#7-from-v16-to-v17),
[8. From v1.7 to v1.8](#8-from-v17-to-v18),
[9. From v1.8 to v1.9](#9-from-v18-to-v19) and
[10. From v1.9 to v1.10](#10-from-v19-to-v110). `v1.10.1` changes no API;
its fixes are in [From v1.10.0 to v1.10.1](#from-v1100-to-v1101).
`v1.11.0` adds to the API and changes a few behaviours: see
[11. From v1.10 to v1.11](#11-from-v110-to-v111). `v1.11.1` changes no
API; its fixes are in [From v1.11.0 to v1.11.1](#from-v1110-to-v1111).
`v1.12.0` only adds to the API: see
[12. From v1.11 to v1.12](#12-from-v111-to-v112).

Replace every `github.com/maccavelli/mcplib/selfupdate` import with
`github.com/maccavelli/go-selfupdate-lib/selfupdate`. No identifier or signature
changes, and the package name is still `selfupdate`, so no call site
changes. Then:

```bash
go mod tidy
```

If `selfupdate` was your last `mcplib` import, `mcplib` and everything it
brought in (the MCP go-sdk among them) leave your `go.mod`.

### Behaviour you may notice

Each of these is a fix. None needs a code change unless your program relied
on the old behaviour.

- **Updating a running program on Windows works reliably.** A replacement
  that Windows refused with "Access is denied" while the old image was
  running is now retried.
- **`--version` is pinned.** If the release source returns a different tag
  than the one requested, `Run` fails with `ErrIntegrity` instead of
  installing it.
- **End of input at the `[y/N]` prompt is "no".** It used to be an `io.EOF`
  error and exit 1; now it is a decline, `Declined` with a nil error, exit
  0.
- **An installer that commits nothing is an error.** A custom `Installer`
  returning `Applied: false` with no error used to count as success.
- **`New` rejects nil and typed-nil collaborators,** including a nil element
  in `Verifiers`, which used to panic mid-update.
- **Unrelated release assets no longer matter.** Only the selected binary
  and `SHA256SUMS` are checked, so an extra asset that is large,
  still uploading, or zero-sized no longer blocks updates.
- **Redirects must stay on HTTPS,** except to a loopback host.
- **Events arrive in order:** `verified` before `transforming`, and
  `complete` only after the session (and its lock) is released. A local
  build's check mode says that apply needs `--force`.
- **Every error names the product** (`selfupdate: <product>: …`), and
  `errors.Is` still matches the sentinels.
- **`PendingBackup` is a path.** On Windows it was always the backup's full
  path; the documentation said "basename" and now says "path".

## 3. The release workflow

In the job that publishes your release, change the `uses:` line and delete
`bridge-release`:

```diff
-    uses: maccavelli/mcplib/.github/workflows/publish-selfupdate-release.yml@<mcplib SHA> # mcplib v1.x.y
+    uses: maccavelli/go-selfupdate-lib/.github/workflows/publish-selfupdate-release.yml@247a2b644b0594a16041e091e4769e43e4f6e639 # v1.12.0
     with:
       artifact-name: …
       products-json: …
       platforms-json: …
       extra-assets-json: …
-      bridge-release: false
```

- **Pin the full commit SHA of a release tag, never the tag name.** The
  workflow is unchanged from `v1.3.0` to `v1.5.0`. `v1.5.1` refuses an
  empty binary, keeps a backport from becoming the latest release, and
  accepts only ASCII digits in a tag. `v1.6.0`, `v1.7.0` and `v1.8.0`
  change nothing in it. `v1.9.0` adds an optional `format` on the
  platform objects, for releases of archives, and changes nothing a
  current call passes. `v1.10.0` reads each tar.gz and gz asset to its
  end before publishing, and changes nothing a call passes either.
  `v1.10.1` runs one publish at a time per repository and names the
  immutable releases setting on a timeout. `v1.11.0`, `v1.11.1` and
  `v1.12.0` change nothing in it, though from `v1.11.1` its release tools
  build with Go 1.27.2, `go.mod`'s `toolchain` line; the example pins
  `v1.12.0`.
  Tags are annotated, so the tag ref names a tag object, not the commit
  `uses:` needs. Resolve the commit with the peeled ref:
  `git ls-remote https://github.com/maccavelli/go-selfupdate-lib 'refs/tags/v1.12.0^{}'`.
- **`bridge-release` must go,** even when it is `false`. The workflow no
  longer declares it, and GitHub rejects an input the called workflow does
  not define. It only ever permitted `magic-cli-remote` `v0.16.0`, which is
  already published.
- `artifact-name`, `products-json`, `platforms-json` and
  `extra-assets-json` mean what they did. The job still needs
  `contents: write`, `id-token: write` and `attestations: write`. Two
  checks are stricter:
  - every extra asset name must match `^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`.
    `install.sh`, `install.ps1` and `<product>-vX.Y.Z-arm64.apk` do; a name
    with a space, a glob character or a leading `-` does not;
  - `SHA256SUMS` must parse exactly as the client parses it: `\n` or
    `\r\n` line endings, and each line under 4096 bytes. `sha256sum`
    output always does.
- The staging artifact must hold regular files only; a symlink is refused.
- **`prerelease-channels-json` is new and optional.** Its default, `[]`,
  publishes strict `vX.Y.Z` tags only, as before. To publish prereleases,
  see [Offer a beta channel](extending-selfupdate.md#offer-a-beta-channel).

## 4. Check

- `go build ./...`, `go vet ./...` and your full `go test ./...` pass.
- `grep -rn 'mcplib/selfupdate' --include='*.go' .` finds nothing.
- `grep -rn 'maccavelli/mcplib/.github/workflows' .github` finds nothing.
- Your release job's `with:` block has no `bridge-release`.
- The first tag you push after the change produces a complete, immutable
  release. A tag-only job cannot be tested before a tag, so plan that tag
  as the check.

## 5. Adopt the canonical update command

Two packages, added in `v1.4.0` (when the module was `go-core-lib`), replace
each program's own update command
([0004-PLAN-v1-4-0-command-surface.md](../decisions/0004-PLAN-v1-4-0-command-surface.md)):

- `buildinfo` owns the build stamps, and decides release or local from them
  alone;
- `selfupdate/cli` is the command: the flags, the stream rule, the exit
  codes, signals and the timeout, in one call to `cli.Command`.

The steps below are prepare-commit-msg's, proven on a scratch copy of it at
`cfada6e`. Its `update --check` then printed this repository's
`selfupdate/cli/testdata/migration` files byte for byte, with exit codes 0,
10 and 1.

**`go.mod`.** Require go-selfupdate-lib `v1.12.0`, with `go 1.27.1`. The steps
were proven at `v1.5.0`; every later release only adds to the API, so
they still build, and §6 lists the behaviour `v1.6.0` changed. `mcplib` stays
only if something else still imports it; in prepare-commit-msg,
`llmprovider` and `wizard` do.

**`update.go`.** The stamp variables, the flag parsing, the contradiction
checks and `runUpdate` all go. What is left builds the updater:

```go
package main

import (
    "net/http"
    "time"

    "github.com/maccavelli/go-selfupdate-lib/buildinfo"
    "github.com/maccavelli/go-selfupdate-lib/selfupdate"
    "github.com/maccavelli/go-selfupdate-lib/selfupdate/cli"
)

const (
    archAMD64 = "amd64"
    archARM64 = "arm64"
)

// updateTimeout bounds each GitHub request. cli.Run bounds the whole run.
const updateTimeout = 15 * time.Minute

// newUpdateUpdater builds the updater; tests replace it.
var newUpdateUpdater = defaultNewUpdateUpdater

// updateOptions are the update command's streams; tests replace them.
var updateOptions = cli.StdioOptions

func defaultNewUpdateUpdater() (*selfupdate.Updater, error) {
    src, err := selfupdate.NewGitHubSource(selfupdate.GitHubOptions{
        Repository: selfupdate.Repository{Owner: "maccavelli", Name: "prepare-commit-msg"},
        Client:     &http.Client{Timeout: updateTimeout},
        UserAgent:  selfupdate.UserAgent(AppTitle, buildinfo.Identity().Current()),
        Limits:     selfupdate.DefaultLimits(),
    })
    if err != nil {
        return nil, err
    }
    selector, err := selfupdate.NewExactAssetSelector([]selfupdate.Platform{
        {OS: "linux", Arch: archAMD64},
        {OS: "linux", Arch: archARM64},
        {OS: "darwin", Arch: archAMD64},
        {OS: "darwin", Arch: archARM64},
        {OS: "windows", Arch: archAMD64},
        {OS: "windows", Arch: archARM64},
    })
    if err != nil {
        return nil, err
    }
    installer, err := selfupdate.NewStandaloneInstaller(selfupdate.InstallOptions{})
    if err != nil {
        return nil, err
    }
    // cli.Run supplies the reporter and the confirmer, on the right streams.
    return selfupdate.New(selfupdate.Config{
        Source:    src,
        Versions:  selfupdate.NewStrictVersionPolicy(),
        Assets:    selector,
        Installer: installer,
        Reporter:  selfupdate.DiscardReporter(),
        Confirmer: selfupdate.NonInteractiveConfirmer(),
        Limits:    selfupdate.DefaultLimits(),
    })
}
```

**`main.go`** and the **`Makefile`**:

```diff
diff --git a/Makefile b/Makefile
--- a/Makefile
+++ b/Makefile
@@ -22,3 +22,3 @@ build: ## Compiles the Go application for the local OS/Arch
     @mkdir -p $(DIST_DIR)
-    @CGO_ENABLED=0 go build -trimpath -tags netgo -ldflags "-extldflags '-static' -s -w -X main.Version=$(VERSION)" -o $(DIST_DIR)/$(BINARY_NAME)-$(shell go env GOOS)-$(shell go env GOARCH)$(if $(filter windows,$(shell go env GOOS)),.exe,) .
+    @CGO_ENABLED=0 go build -trimpath -tags netgo -ldflags "-extldflags '-static' -s -w -X github.com/maccavelli/go-selfupdate-lib/buildinfo.version=$(VERSION)" -o $(DIST_DIR)/$(BINARY_NAME)-$(shell go env GOOS)-$(shell go env GOARCH)$(if $(filter windows,$(shell go env GOOS)),.exe,) .
 
@@ -28,3 +28,3 @@ linux: linux-amd64 ## Alias for linux-amd64
 
-RELEASE_LDFLAGS := -s -w -X main.Version=$(VERSION) -X main.RawVersion=$(VERSION) -X main.RawBuildKind=release
+RELEASE_LDFLAGS := -s -w -X github.com/maccavelli/go-selfupdate-lib/buildinfo.version=$(VERSION) -X github.com/maccavelli/go-selfupdate-lib/buildinfo.kind=release
 
diff --git a/main.go b/main.go
--- a/main.go
+++ b/main.go
@@ -9,6 +9,4 @@ import (
     "os"
-    "os/signal"
     "slices"
     "strings"
-    "syscall"
     "time"
@@ -21,4 +19,5 @@ import (
 
+    "github.com/maccavelli/go-selfupdate-lib/buildinfo"
+    "github.com/maccavelli/go-selfupdate-lib/selfupdate/cli"
     "github.com/maccavelli/mcplib/llmprovider"
-    "github.com/maccavelli/mcplib/selfupdate"
 )
@@ -30,5 +29,2 @@ var osGetenv = os.Getenv
 
-// Version is overwritten by build flags during the compilation process.
-var Version = localVersionIdentity
-
 const (
@@ -89,3 +85,3 @@ func main() {
     case "version", "--version", "-V":
-        fmt.Printf("%s version %s (%s)\n", AppTitle, strings.TrimPrefix(displayVersion(), "v"), RawBuildKind)
+        fmt.Printf("%s version %s\n", AppTitle, buildinfo.Identity())
         return
@@ -101,11 +97,3 @@ func main() {
     case "update":
-        ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
-        defer stop()
-        ctx, cancel := context.WithTimeout(ctx, updateTimeout)
-        defer cancel()
-        res, err := runUpdate(ctx, args[1:])
-        if err != nil && !errors.Is(err, selfupdate.ErrUpdateAvailable) {
-            fmt.Fprintf(os.Stderr, "Update failed: %v\n", err)
-        }
-        osExit(selfupdate.ExitCode(res, err))
+        osExit(cli.Command(context.Background(), args[1:], AppTitle, buildinfo.Identity(), newUpdateUpdater, updateOptions()))
         return
@@ -117,9 +105,2 @@ func main() {
 
-func displayVersion() string {
-    if RawVersion != "" && RawVersion != localVersionIdentity {
-        return RawVersion
-    }
-    return Version
-}
-
 func runConfigure(args []string) error {
```

The `Makefile` writes out the two `-X` names that `buildinfo.VersionVar` and
`buildinfo.KindVar` hold, because make cannot read a Go constant. A test
that runs `make -n build-all` and counts both names keeps them right.

### What a user of the program will notice

- **Progress, prompts and the outcome go to stderr.** Stdout is empty
  unless `--json` is given (0004-MADR, owner decision 1). A script that read
  the old progress from stdout must read stderr.
- **The outcome is one line,** such as `prepare-commit-msg: update available:
  v1.2.0 -> v1.3.0`.
- **A failure is one line,** `update failed: <message>`. The old
  `Update failed:` is gone.
- **New flags:** `--dry-run`, `--json` and `--channel`.
- **Usage errors exit 1,** and print the flag list. `-h` and `--help` print
  help and exit 0.
- **`version`** prints `buildinfo.Identity()`, such as
  `v1.3.0 (release) 0123456789ab`. The revision is there when the build
  recorded one.

### With cobra

`cli.Command` uses the standard `flag` package. A cobra program binds the
same flags on its own command and calls `cli.Run` and `cli.Exit`, which fixes
C3 (a spurious usage dump on exit 10) and C4 (ctrl+c not cancelling):

```go
var flags cli.Flags
cmd := &cobra.Command{
    Use:  "update",
    Args: cobra.NoArgs,
    RunE: func(cmd *cobra.Command, _ []string) error {
        cmd.SilenceErrors, cmd.SilenceUsage = true, true
        req, err := flags.Request(product, buildinfo.Identity())
        o := cli.StdioOptions()
        o.JSON = flags.JSON
        if err != nil {
            os.Exit(cli.Exit(o.Stderr, selfupdate.Result{}, err))
        }
        u, err := newUpdater()
        if err != nil {
            os.Exit(cli.Exit(o.Stderr, selfupdate.Result{}, err))
        }
        ctx := cmd.Context()
        if ctx == nil { // not run with ExecuteContext
            ctx = context.Background()
        }
        res, err := cli.Run(ctx, u, req, o)
        if code := cli.Exit(o.Stderr, res, err); code != 0 {
            os.Exit(code)
        }
        return nil
    },
}
flags.Bind(cmd.Flags()) // pflag's flag set has BoolVarP, so -y works
```

Unlike `cli.Command`, this recipe writes no `--json` result object when the
request is refused or the updater cannot be built. The exit code and the
stderr line are the same. cobra checks `Args` before `RunE` runs, so a
positional argument still prints cobra's usage.

An MCP server that sends `os.Stdout` to stderr to protect JSON-RPC passes
the real stdout in `o.Stdout` when `--json` is given.

### Check

- `go build ./...`, `go vet ./...` and `go test ./...` pass.
- `<prog> update --check > /dev/null` still shows the outcome, on stderr.
- `<prog> update --check --json 2> /dev/null` prints only JSON lines, the
  last with `"kind":"result"`.
- `make -n build-all` names `buildinfo.version` and `buildinfo.kind`, once
  for each platform.

## 6. From v1.5 to v1.6

```bash
go get github.com/maccavelli/go-selfupdate-lib@v1.12.0
```

This takes the current release; what `v1.7.0`, `v1.8.0`, `v1.9.0` and
`v1.10.0` add is in [7. From v1.6 to v1.7](#7-from-v16-to-v17),
[8. From v1.7 to v1.8](#8-from-v17-to-v18),
[9. From v1.8 to v1.9](#9-from-v18-to-v19) and
[10. From v1.9 to v1.10](#10-from-v19-to-v110). Every exported change is an
addition, so a program that built on `v1.5.x` builds unchanged. These are
the behaviours that change, and what to do about each. Why, and how, is in
[0010-MADR](../decisions/0010-MADR-remediate-second-debugging-pass-findings.md)
§3 and its amendments A2 to A5.

### An error after the update is a warning

Once a run has reported `complete`, a later error no longer fails it. Such an
error is a failed unlock, a failed report of `complete`, or an installer that
committed and still returned an error. Each is an `EventWarning` after
`complete`, and an entry in `Result.Warnings`, and `Run` returns no error.
The exit code is 0, where it was 1 with the binary already replaced.

- `Result.Warnings` is of type `Warnings`: `List()` returns the entries.
  `NewWarnings` builds one, for a test.
- `cli` prints `warning: <text>` on stderr for each one, in either mode.
- A reporter that switches on the event kind sees `warning` after
  `complete`. Treat it as advisory, as `rolled-back` is.

### The JSON result is schema 2

`Result.Document`, and so the `--json` result object, has `schema_version`
2. It adds `service_started`, and `warnings`, an array that is omitted when
there are none. A program that reads the document checks `schema_version`.

A dry run's `complete` event has the `detail` `dry-run`. It was the sentence
"dry run: verified, nothing installed".

### An apply for another platform is refused

`Request.Platform` set to anything but the running platform is refused with
`ErrUnsupportedPlatform`, before any network call, unless the request is a
check or a dry run. Leave `Platform` zero to update the running program.
To ask about another platform, use `CheckOnly` or `DryRun`.

### A stopped service stays stopped

`ManagedInstaller` used to start a service after the update even when it had
been stopped. Now it starts one only when it was running, or when your
`Lifecycle` also implements `EnabledLifecycle` and `Enabled` reports it
configured to start. To keep starting a stopped service that is enabled,
implement `Enabled`: systemd `is-enabled`, launchd `RunAtLoad` or
`KeepAlive`, a Windows automatic start type. `InstallResult.ServiceStarted`,
`Result.ServiceStarted` and `service_started` say what happened.

### A setuid or setgid binary needs the policy

A target with the setuid or setgid bit is refused, where it used to be
replaced and lose the bit. Set `TargetPolicy.AllowSpecialModeBits` if the
program is installed that way: the new binary keeps the bits. The sticky
bit is always kept. On Unix the new binary also gets the old one's owner
and group, when the updater may give them.

### Smaller changes

- `CheckCached` also caches `ErrLatestOlder`, `ErrUnsupportedPlatform` and
  `ErrMutableRelease`, for `maxAge`, as `CheckRecord.Outcome`. A cached one
  returns an error that matches its sentinel. A check file written before
  `v1.6.0` loads as a miss: the first check after the upgrade asks the
  network once.
- `Begin` and `CleanupPending` remove what a crashed update left beside the
  target: staging files and backups that nothing else owns under the lock.
- A zero `ConfirmNeeded` or `CredentialNeeded`, such as one a UI test
  builds, can be answered: the call returns at once.
- `selfupdatetest.GitHubServer.RequireCredential(header, value)` requires a
  custom-header credential, and `RecordedRequest.CredentialHeaders` names
  the credential headers each request carried.

### Check

- `go build ./...`, `go vet ./...` and `go test ./...` pass.
- A test or a script that reads the `--json` result accepts
  `schema_version` 2.
- A managed service that is stopped but enabled is started after an update
  only if your `Lifecycle` implements `Enabled`.

## 7. From v1.6 to v1.7

```bash
go get github.com/maccavelli/go-selfupdate-lib@v1.12.0
```

`v1.7.0` only adds: `make apicheck` reports it compatible with `v1.6.0`, it
requires the same three modules, and the release workflow is unchanged. A
program that does not use the new packages behaves as it did. Why, and how,
is in
[0011-MADR](../decisions/0011-MADR-reference-service-lifecycles.md).

### What is new

- **`selfupdate/service`:** what the backends share: `PollHealthy`,
  `NewExecReconciler` for a program that owns its service definition, the
  typed errors, and the handoff (`HandOffFunc`, `ReportFunc`,
  `ReadHandOffResult`, `LoadHandOffEnv`).
- **`selfupdate/service/systemd`, `launchd` and `scm`:** a
  `selfupdate.Lifecycle`, `EnabledLifecycle`, `Reconciler` and
  `service.Detacher` each, for a systemd unit, a launchd job and a Windows
  service, and `systemd.Notify` with its helpers. Each recognises its binary
  by file identity too, so a definition that names it through a symlinked
  directory or by a Windows 8.3 short name is not a mismatch.
- **`cli.Options.HandOff`:** an update started from inside the service, such
  as by an agent the service spawned, runs detached from it. The command
  prints `update handed off: …` and exits 0; under `--json` the result
  object gains `handed_off`.

### Adopting it

- If your program hand-wrote a `Lifecycle` for one of these managers,
  replace it with the backend's `New`, and pass the result to
  `NewManagedInstaller` as both the `Lifecycle` and the `Reconciler`.
- To update from inside the service, call `service.ReportFunc()` first in
  `main`, and set `Options.HandOff` from `service.HandOffFunc` and that
  function. See
  [Run as a service](extending-selfupdate.md#run-as-a-service).

### Check

- `go build ./...`, `go vet ./...` and `go test ./...` pass.
- `go list -m github.com/maccavelli/go-selfupdate-lib` gives `v1.12.0`.

## 8. From v1.7 to v1.8

```bash
go get github.com/maccavelli/go-selfupdate-lib@v1.12.0
```

`v1.8.0` only adds. `make apicheck` reports it compatible with `v1.7.0`, it
requires the same three modules, and the release workflow is unchanged. A
program that does not use the new pieces behaves as it did: nothing new
runs unless you configure it. Why, and how, is in
[0012-MADR](../decisions/0012-MADR-archive-assets-and-macos-codesign.md).

### What is new

- **In `selfupdate`:**
  - an extract stage, `Config.Unpacker`, for a selection the selector marks
    `Packed`. It runs after the `Verifiers` and before the transform, and
    reports the new event `unpacking`;
  - `CheckImage`, the image check `NewImageVerifier` makes, on any reader;
  - `ChainTransformers`, to run several transforms in order.
- **`selfupdate/archive`:** `NewSelector` and `NewUnpacker`, to update
  from a release whose asset is a `.tar.gz`, `.zip` or `.gz` holding the
  program, in this module's naming or GoReleaser's.
- **`selfupdate/codesign`,** for a publisher who signs macOS binaries:
  - `NewSigner` re-signs the staged binary with your identity under a fixed
    identifier;
  - `NewChecker` refuses a binary whose signature is missing, invalid, or
    does not meet your requirement.

  It is opt-in, and nothing runs it by default.

### Adopting it

- **To ship archives,** set `Config.Assets` to `archive.NewSelector` and
  `Config.Unpacker` to `archive.NewUnpacker`. See
  [Ship an archive](extending-selfupdate.md#ship-an-archive). Since
  `v1.9.0` this repository's workflows build and publish archives too,
  from a spec whose platforms set `format`: see
  [the building guide, §7](building-releases.md#7-archives).
- **If you re-sign on update** with your own `Transformer`, as
  magic-cli-remote's `codesignTransformer` does, replace it with
  `codesign.NewSigner`, with an `Identifier`. That fixes two defects: the
  identifier taken from the staging file's name, and a `codesign` found on
  `PATH`. See [Sign on macOS](extending-selfupdate.md#sign-on-macos).
- **A JSON or event consumer** that switches on event kinds sees
  `unpacking` only from a run with an `Unpacker`.

### Check

- `go build ./...`, `go vet ./...` and `go test ./...` pass.
- `go list -m github.com/maccavelli/go-selfupdate-lib` gives `v1.12.0`.

## 9. From v1.8 to v1.9

```bash
go get github.com/maccavelli/go-selfupdate-lib@v1.12.0
```

`v1.9.0` only adds. `make apicheck` reports it compatible with `v1.8.0`,
and it requires the same three modules. A program and a release workflow
that use none of the new pieces behave as they did. Why, and how, is in
[0013-MADR](../decisions/0013-MADR-build-and-stage-release-workflow.md).

### What is new

- **`selfupdate/releasespec`:** a release spec, `selfupdate-release.json`,
  that your program embeds and the build workflow reads: products,
  platforms, packaging, extras and prerelease channels, in one file. It
  gives your updater its asset selector and unpacker, and refuses a
  product name the spec does not list.
- **`build-selfupdate-release.yml`,** a new reusable workflow: it builds
  every product for every platform with a fixed recipe, checks each binary,
  runs your identity command on each platform's runner, packs archives,
  writes `SHA256SUMS` and stages the release. Off a tag it rehearses.
- **`publish-selfupdate-release.yml` publishes archives:** a `format` on
  every `platforms-json` object, which the build workflow writes for you,
  makes the release one of archives, checked with the client's own
  unpacker before anything is published. Without a `format`, nothing
  changes.

### Adopting it

- **Nothing is required.** Your current pin of the publish workflow keeps
  working, and so does moving the pin to `v1.9.0` with the same inputs.
- **To build with the new workflow,** follow
  [Building releases](building-releases.md): write the spec, embed it,
  configure the updater from it, and replace your build, checksum and
  staging jobs with the build workflow. If you stamp your own variables,
  move to `buildinfo` first.
- **To ship archives,** set `"packaging": "archive"` in the spec.

### Check

- `go build ./...`, `go vet ./...` and `go test ./...` pass.
- `go list -m github.com/maccavelli/go-selfupdate-lib` gives `v1.12.0`.
- A pull request runs the build workflow as a rehearsal, and it passes.

## 10. From v1.9 to v1.10

```bash
go get github.com/maccavelli/go-selfupdate-lib@v1.12.0
```

`v1.10.0` only adds. `make apicheck` reports it compatible with `v1.9.0`,
and it requires the same three modules. A spec without `installer`
stages exactly what `v1.9.0` staged. Why, and how, is in
[0014-MADR](../decisions/0014-MADR-shared-installer-templates.md).

### What is new

- **The spec's `installer` field:** present, even empty, it makes the
  build workflow render `install.sh` and `install.ps1` into every release
  from this repository's templates, with the program's products,
  platforms, formats, channels, repository and tag. Each installer
  installs its own release, checks every file against `SHA256SUMS`, checks
  the identity, keeps the previous binary, and can run hooks of your
  products before and after the install. `releasespec` gains the
  `Spec.Installer` field, the `Installer` and `Hook` types,
  `InstallerScripts`, `InstallerName` and `InstallerEnvPrefix`, and the
  constants `InstallerScript`, `InstallerPowerShell`, `HookBeforeInstall`
  and `HookAfterInstall`.
- **With `installer` present,** `install.sh` and `install.ps1` are
  reserved extra names, and hook arguments and `identity_args` keep to
  `[A-Za-z0-9._:=/,+@%-]`, because the installers embed them.
- **The publish workflow's archive check** also reads each tar.gz and gz
  asset to its end, and refuses one that is cut, has a wrong checksum or
  has data after it, as `gzip` and `tar` would. The build workflow never
  writes one; an archive you stage yourself must be whole.

### Adopting it

- **Nothing is required.** A spec without `installer`, and your
  hand-written installers, keep working.
- **To use the generated installers,** follow
  [step 12 of the building guide](building-releases.md#12-installers):
  add `installer`, with `name` and `env_prefix` set to what your scripts
  used; delete your scripts and their extras; move any product-specific
  step into a subcommand and a hook. The one-liners' URLs stay the same.
- **Move both to `v1.10.0` together:** your program's module, so that
  `releasespec.Parse` reads the spec it embeds, and the build workflow's
  pin, so that it renders the installers. `v1.9.0` refuses a spec with
  `installer` as an unknown field, in your program and in the workflow.
  Until `v1.11.0`, the build workflow does not check this; since then its
  plan step refuses a module whose requirement is too old for the spec.

### From v1.10.0 to v1.10.1

```bash
go get github.com/maccavelli/go-selfupdate-lib@v1.10.1
```

`v1.10.1` fixes the third debugging pass's findings and changes no API:
`make apicheck` reports it compatible with `v1.10.0`. What changed, and
why, is in
[0015-MADR](../decisions/0015-MADR-remediate-third-debugging-pass-findings.md).

**Behaviour changes:**

- **A backup the run could not restore** is kept as
  `.<base>.selfupdate-kept-<n>`, and no later session's sweep removes it;
  restore or remove it yourself, as `Result.PendingBackup` says. A dry
  run sweeps nothing.
- **A second `Commit` or `Rollback`** of one replacement is refused.
- **A target replaced during the update,** by a package manager or with a
  symlink, fails the install with `ErrConcurrentUpdate` instead of being
  overwritten.
- **A release without the platform's asset** fails as
  `unsupported-platform`, and `CheckCached` caches the answer.
- **A stop that fails after the service stopped,** such as a wait that
  timed out, starts the service again. The stop waits by the service
  manager's own kill bound, whatever your context does.
- **The archive unpacker** also refuses names outside printable ASCII or
  with an element ending in a dot or a space, a zip local header that
  disagrees with its central record, an Info-ZIP Unicode Path field, a
  directory that is not named as one or that holds data, and encrypted
  zip entries. `releasespec.Parse` refuses, under archive packaging, an
  archive name over 128 characters and a tar.gz program name over 100.
- **A codesign `Requirement`** is checked in its own `codesign --verify`
  run, after the identifier's.
- **The generated installers:** a product named twice installs once; an
  empty `--product` is refused; hooks and identity commands run with no
  standard input; a relative install folder is made absolute; checksums
  verify in a folder whose name holds `\`; a `--version` holding a newline
  is refused; a failed identity check restores only what this run moved
  aside; and a directory where a program goes is refused.
- **The publish workflow** runs one publish at a time per repository
  (group `go-selfupdate-lib-publish-<owner>/<repo>`), names the immutable
  releases setting when it times out, and accepts a repository whose
  current latest release is not `vX.Y.Z`.

**Check:** `go list -m github.com/maccavelli/go-selfupdate-lib` gives
`v1.10.1`.

### Check

- `go build ./...`, `go vet ./...` and `go test ./...` pass.
- `go list -m github.com/maccavelli/go-selfupdate-lib` gives `v1.12.0`.
- A pull request's rehearsal stages `install.sh` and `install.ps1`, and a
  dry run of the released one-liner (`… | sh -s -- --dry-run`) names the
  assets you expect.

## 11. From v1.10 to v1.11

```bash
go get github.com/maccavelli/go-selfupdate-lib@v1.12.0
```

`v1.11.0` settles the contracts the third debugging pass left open.
`make apicheck` reports it compatible with `v1.10.1`: every exported change
is an addition. Some behaviours change, and a program that relies on them
may need a change. Why, and how, is in
[0015-MADR](../decisions/0015-MADR-remediate-third-debugging-pass-findings.md).

### What changes

- **Zero values:** an `Updater` or a `Checker` that its constructor did
  not make, such as a zero value or a nil pointer, returns
  `ErrNotConstructed` instead of panicking; a zero `Stream`'s `Cancel`
  does nothing.
- **No such release:** a 404 for the latest release, or for a tag, is
  `ErrNoRelease`. `CheckCached` caches it as `CheckNoRelease`
  (`no-release`) for `maxAge`, so a repository with no stable release yet
  is asked once per interval, not on every start.
- **The JSON result is schema 3:** `rolled_back` says the previous binary
  was restored after the new one was installed, and `probes_skipped` that
  a dry run for another platform did not run the probes. `Result` has
  `RolledBack` and `ProbesSkipped`. A reader that checks `schema_version`
  must accept 3.
- **Handoffs:** an update from inside a service without `--yes` is refused
  with `ErrConfirmationRequired` (exit 1), since the detached run cannot
  ask; and a run with nothing to install is not handed off.
- **Reconcile warnings:** `ReconcileResult.Warnings` reaches
  `Result.Warnings` and a `warning` event after `complete`, on an update
  that applied. `ExecReconciler` fills it from the receipt, and warns
  when a definition was rewritten without the service manager being
  reloaded.
- **Poll options:** the service backends' `New` refuses `Options.Poll`
  whose settle window is not shorter than its timeout, launchd's settle
  at least 10 s (`PollOptions.Validate`).
- **Dependents:** with `StopDependents`, the SCM backend starts the
  dependents it stopped again after the service.
- **Special mode bits:** a previous binary kept beside the target, by
  `KeepPrevious` or after a failed restore, loses setuid and setgid.
- **Back-off:** a rate limit with requests left waits its `Retry-After`,
  not the primary window's reset.
- **The spec:** `null` for any value is refused: leave the field out.
- **Archives:** a tar.gz from `git archive` unpacks: its PAX global header
  is skipped, unless it sets an entry's path, link, size or sparse map.
- **The build workflow:** its plan step checks that the module at
  `module-dir` requires a release of this library that reads every field
  the spec uses, such as `installer` (`v1.10.0`).

### Adopting it

- **A zero `Updater` or `Checker`** that used to panic now returns an
  error: build them with `New` and `NewChecker`.
- **Read `schema_version` 3,** and the two new keys, if you parse the
  result object.
- **Pass `--yes`** to an update an agent runs inside a service; it already
  had to, since the detached run never asked.
- **Check `Options.Poll`:** a short `Timeout` needs a shorter `Settle`, or
  a negative one for no window.
- **Restore setuid or setgid yourself** when you put a kept `.previous`
  back.
- **A spec with `null`** fails to parse: remove the key.
- **Move both to `v1.11.0` together:** your program's module and the
  workflows' pins. The build workflow now refuses a module whose
  requirement is too old for the spec.

### From v1.11.0 to v1.11.1

```bash
go get github.com/maccavelli/go-selfupdate-lib@v1.11.1
```

`v1.11.1` closes the last two archive differentials and changes no API:
`make apicheck` reports it compatible with `v1.11.0`. What changed, and
why, is in
[0017-MADR](../decisions/0017-MADR-verify-build-provenance-and-close-0015-open-items.md).

**Behaviour changes:**

- **A zip whose records do not account for every byte** is refused:
  bytes before its first entry, between entries or after its end record;
  slack in its central directory; a zip64 end record that disagrees with
  the end record; and a data descriptor that reads two ways, or not as
  its central record says. A streaming reader, such as macOS's `ditto`,
  would extract an entry no central record names. Archives this module's
  build workflow packs, and those Go, Python and Info-ZIP write, pass.
- **A name Windows reserves is refused on every host,** not only on
  Windows: an element holding one of `< > : " | ? *`, or naming a device
  (`CON`, `PRN`, `AUX`, `NUL`, `CONIN$`, `CONOUT$`, `COM0`–`COM9`,
  `LPT0`–`LPT9`, before its first dot, in any case). `UnpackOptions.Member`
  follows the same rules, so `NewUnpacker` refuses one no entry could
  have. A product named after a device, such as `aux`, cannot be packed
  as an archive.
- **Go 1.27.2:** `go.mod` gains `toolchain go1.27.2`, and keeps `go 1.27.1`
  as your floor. CI and the publish workflow's release tools build with
  1.27.2, which fixes ten advisories in 1.27.1's standard library. Build
  your program with 1.27.2 too: your `go.mod`'s `toolchain` line, or your
  hosts' Go
  ([0018-MADR](../decisions/0018-MADR-move-toolchain-to-go-1-27-2.md)).

**Check:** `go list -m github.com/maccavelli/go-selfupdate-lib` gives
`v1.11.1`.

### Check

- `go build ./...`, `go vet ./...` and `go test ./...` pass.
- `go list -m github.com/maccavelli/go-selfupdate-lib` gives `v1.12.0`.
- A pull request's rehearsal passes the plan step's module check.

## 12. From v1.11 to v1.12

```bash
go get github.com/maccavelli/go-selfupdate-lib@v1.12.0
```

`v1.12.0` adds a build-provenance check, and keeps the previous binary of
an update that was interrupted. `make apicheck` reports it compatible with
`v1.11.1`: every exported change is an addition. Why, and how, is in
[0017-MADR](../decisions/0017-MADR-verify-build-provenance-and-close-0015-open-items.md).

### What is new

- **`selfupdate/verify/ghattest`:** an opt-in check that a release was
  built by your release workflow. `NewManifestVerifier` runs `gh attestation
  verify` on the release's `SHA256SUMS`, before any binary is downloaded:
  the attestation must name your repository and the release's tag, and be
  signed by this module's publish workflow on a GitHub-hosted runner.
  `NewVerifier` checks the asset itself instead. Every failure fails the
  update with `ErrIntegrity`, a missing or logged-out `gh` included.
- **An interrupted update keeps the previous binary:** an update writes
  `.<base>.selfupdate.pending` before it replaces the target, and removes
  it once the replacement is committed or rolled back. The next session,
  such as `CleanupPending` at startup, keeps the backup it names as
  `.<base>.selfupdate-kept-<n>` instead of removing it. An update refuses
  to start while a journal it could not resolve is still there, and names
  it.
- **`KeptBackups`:** `StandaloneInstaller.KeptBackups` and
  `ManagedInstaller.KeptBackups` list the kept backups beside the target,
  each with its path, size and time.

### Adopting it

- **To check provenance,** add the manifest verifier, with `gh`'s absolute
  path and your own repository:

  ```go
  v, err := ghattest.NewManifestVerifier(ghattest.Options{
      Policy: ghattest.Policy{Repository: selfupdate.Repository{Owner: "example", Name: "relay"}},
      GH:     "/usr/local/bin/gh",
  })
  if err != nil {
      return err
  }
  cfg.ManifestVerifiers = append(cfg.ManifestVerifiers, v)
  ```

  `gh` must be logged in on the machine that updates, even for a public
  repository, so the check suits a developer's machine more than a
  service. Restrict who can create `v*` tags in your repository (the
  building guide's step 4): the check does not stop a malicious tag pushed
  through your workflow.
- **After `CleanupPending`, look for kept backups,** and tell the user, or
  restore or remove them:

  ```go
  kept, err := inst.KeptBackups(ctx)
  ```

- **Nothing else changes:** the journal is written whatever you adopt, and
  a program that adopts neither keeps working.

### Check

- `go build ./...`, `go vet ./...` and `go test ./...` pass.
- `go list -m github.com/maccavelli/go-selfupdate-lib` gives `v1.12.0`.
