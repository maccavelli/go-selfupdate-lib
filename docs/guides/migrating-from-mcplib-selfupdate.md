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
go get github.com/maccavelli/go-selfupdate-lib@v1.5.0
```

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
+    uses: maccavelli/go-selfupdate-lib/.github/workflows/publish-selfupdate-release.yml@c7a8b4ca8045bdb26b0908206b775192358c8253 # v1.5.1
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
  accepts only ASCII digits in a tag; the example pins `v1.5.1`.
  Tags are annotated, so the tag ref names a tag object, not the commit
  `uses:` needs. Resolve the commit with the peeled ref:
  `git ls-remote https://github.com/maccavelli/go-selfupdate-lib 'refs/tags/v1.5.1^{}'`.
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

**`go.mod`.** Require go-selfupdate-lib `v1.5.0`, with `go 1.27.1`. `mcplib` stays
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
