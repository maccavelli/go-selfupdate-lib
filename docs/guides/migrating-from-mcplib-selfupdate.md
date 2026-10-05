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
go get github.com/maccavelli/go-selfupdate-lib@v1.7.0
```

`v1.7.0` is the current release. `v1.6.0` changed a few behaviours that
`v1.5.x` had; they are listed in [6. From v1.5 to v1.6](#6-from-v15-to-v16).
`v1.7.0` only adds to the API: see [7. From v1.6 to v1.7](#7-from-v16-to-v17).

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
+    uses: maccavelli/go-selfupdate-lib/.github/workflows/publish-selfupdate-release.yml@e825cdafd332df7e532f0c54da9d69a53e27d70b # v1.7.0
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
  accepts only ASCII digits in a tag. `v1.6.0` and `v1.7.0` change nothing
  in it; the example pins `v1.7.0`.
  Tags are annotated, so the tag ref names a tag object, not the commit
  `uses:` needs. Resolve the commit with the peeled ref:
  `git ls-remote https://github.com/maccavelli/go-selfupdate-lib 'refs/tags/v1.7.0^{}'`.
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

**`go.mod`.** Require go-selfupdate-lib `v1.7.0`, with `go 1.27.1`. The steps
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
go get github.com/maccavelli/go-selfupdate-lib@v1.7.0
```

This takes the current release; what `v1.7.0` adds is in
[7. From v1.6 to v1.7](#7-from-v16-to-v17). Every exported change is an
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
go get github.com/maccavelli/go-selfupdate-lib@v1.7.0
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
- `go list -m github.com/maccavelli/go-selfupdate-lib` gives `v1.7.0`.
