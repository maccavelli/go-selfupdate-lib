---
status: accepted
date: 2026-10-19
decision-makers: go-selfupdate-lib maintainers
consulted: "the owner's list of Go SDK design practices (2026-10-19); four read-only audits of the code at 5c0ab0d (v1.15.2 plus records), 2026-10-19, with the load-bearing findings re-checked; Effective Go, Go Code Review Comments, the Go blog's \"Keeping Your Modules Compatible\", the Go release policy, the TUF attack catalogue; magic-cli-remote's 0169-MADR (the fleet toolchain rule); 0004-REPORT-release-signing-research.md"
informed: the fleet's programs that import this module, magic-cli-remote first
---
# Align go-selfupdate-lib with Go SDK design practices: redact secrets, type and document errors, write the concurrency and evolution contracts, add a one-call quickstart, retract the pre-rename tags, lower the Go floor to 1.26.9, and ship release signing as an opt-in verifier

## Context and Problem Statement

On 2026-10-19 the owner supplied a list of practices for designing a Go
SDK. It draws on Effective Go, Go Code Review Comments, Google's Go style
guide and the module compatibility rules. The owner asked:

> assess the current codebase, evaluate the provided list of recommended
> best practices and standards for designing and building go
> SDK/libraries and determine what we need to do in order to more closely
> align with the recommendations to enhance, improve, and expand
> compatibility, flexibility, adaptability, and extensibility, as well as
> hardening and stability.

The list has ten sections: API design, construction, context, errors,
versioning, documentation, dependencies, concurrency, security and a
publishing checklist. The security section has bullets for an
authentication SDK and a self-update SDK, and this module is the latter.

This record does three things:

* scores the module at `v1.15.2` against every practice on the list, with
  evidence (the scorecard below);
* names each place where the module falls short, and says whether the
  shortfall is a defect, a gap, or a choice an earlier record made on
  purpose;
* proposes one decision per shortfall (D1–D14), every one of them
  compatible with v1.

It was written `proposed`, and the owner decided each of D1–D14 and the
release split on 2026-10-19 (Decision Outcome). Nothing in it is
implemented yet. Two of its decisions reopen choices the owner made
earlier, and they say so: D10 reopens 0004's deferral of release signing,
and D8 moves the `go` line below the version 0001 and 0018 chose.

### Method

* **Four read-only audits**, one per group of practices:
  1. API design, construction and injectability (§1, §2, §7);
  2. context, errors, secrets and concurrency (§3, §4, §8, and the
     credential bullets of §9);
  3. the self-update security model (§9);
  4. versioning, documentation, packaging, tests and the checklist (§5,
     §6, §7, §10).

  Each audit read the code, `go doc` and the records, and wrote its own
  probes in a scratch directory outside the repository:
  * a `go/ast` inventory of exported identifiers, interfaces,
    package-level variables and context parameters;
  * a doc-comment checker, seen failing on three planted declarations;
  * a token-formatting probe;
  * an `apidiff` experiment on open and sealed interfaces;
  * a GitHub Enterprise base-URL probe;
  * `go test -cover ./...`, and `go test -race ./selfupdate/`.

  Commands that would write the tree ran in a scratch clone, including
  `make apicheck` against `v1.15.2`, `v1.5.0` and `v1.0.0`.
* **Re-checks by this record's author.** Every finding that drives a
  decision was checked again from the source, by reading the cited lines
  or re-running the probe:
  * the token leak (F1);
  * the untyped HTTP status error (F2);
  * `HandOffHop` and the `context.Background` retry (F3);
  * `lastCoInit` and the Updater's run guard (F4);
  * the "no convenience constructor" policy (F5);
  * the version list and the old path's `@latest` (F7);
  * the Go floor (F8);
  * the repository's security settings (F9);
  * the `Immutable` gate and the GitHub-shaped fields (F11).
* **Two probes added by this record** (F7, F8), each seen to fail or
  differ on a planted input.
* **External sources**, listed under More Information.

### Scorecard

Verdicts: **Aligned**, **Partial**, **Gap**, **N/A**. "§" is the section of
the owner's list. Line numbers are at `5c0ab0d`.

| § | Practice | Verdict | Evidence |
| :--- | :--- | :--- | :--- |
| 1 | Accept interfaces, return concrete types | Partial | Every seam in `Config` is an interface (`selfupdate/types.go:806-850`). `New`, `NewChecker`, `NewGitHubSource`, the installers and the four backends' `New` return concrete types. 16 constructors return interfaces, for example `NewExactAssetSelector`, `NewStrictVersionPolicy`, `NewTextReporter`, `archive.NewSelector`, `codesign.NewSigner` and `ghattest.NewManifestVerifier`. |
| 1 | Small interfaces; growth without breakage | Partial | 25 interfaces in `selfupdate`; most have one method and a `…Func` adapter. Callers implement `Lifecycle` (5 methods, `types.go:560`), `InstallSession` (4, `:519`), `TwoPhaseSession` (8 with embedded, `:551`) and `ReleaseSource` (3, `:281`), and none can gain a method: `apidiff` at the pinned version reports `Open.B: added` as incompatible, and accepts a method added to a sealed interface. The module already grows by optional interfaces: `EnabledLifecycle` (`managed.go:139`), `ReleaseLister` (`checker.go:264`), `ChannelPolicy`, `CredentialedSource`, `StagingOwner`. The policy is not written down. |
| 1 | Minimize the public surface | Partial | 458 exported identifiers in 13 public packages; `selfupdate` holds 215 of them. GitHub's JSON types and the state machines are unexported (`github.go:290`, `:300`; `managed.go:104`; `checkcache.go:259`). `ParseSHA256SUMS` is used outside `selfupdate` only by `internal/cmd/selfupdate-release/stage.go:235`, and its doc offers it to callers ("exactly as the Updater does", `checksums.go:46`). |
| 1 | Expose answers, not internals | Aligned | `Updater.Run` and `RunWith`, `Request.CheckOnly` and `DryRun`, `Checker.Check` and `CheckCached`, `Start` and `Stream`, `cli.Command`. `Availability` (`checker.go:63`) answers with `Available`, `ForceRequired` and `Operation`. |
| 1 | Least surprise: safe defaults, one-line common path | Partial | The defaults that exist are safe: strict version policy, prereleases opt-in, no credential to another origin, `InstallOptions{}` valid, and zero `cli.Options` fields defaulted. There is no common-path one-liner. The minimal standalone binding is five fallible calls (`example_test.go:24-65`), and `DefaultLimits()` has to be passed twice, because zero limits are refused (`types.go:777`, `:799-803`). See F5. |
| 1 | No global mutable state | Aligned (exported); Partial (internal) | Every exported package-level variable is an `errors.New` sentinel. About 35 unexported function variables are test seams (`github.go:30-31`, `target.go:12-14`, `replace.go:13-21`, …). They are why `selfupdate`'s tests never call `t.Parallel()` (0 calls; 11 in the repository). One production write is unsynchronised: `lastCoInit` (F4). |
| 2 | Required values as parameters, optional behaviour as options | Aligned (by another idiom) | Construction is a validated config struct: `New(Config)`, `NewChecker(CheckerConfig)`, `NewGitHubSource(GitHubOptions)`, and each backend's `New(Options)`, which refuse nil, typed-nil, invalid limits and negative intervals (`updater.go:28-73`, `checker.go:86-99`, `github.go:105-125`). Per-run behaviour uses sealed functional options (`RunOption`, `runoptions.go:12`). |
| 2 | `NewClient(required)` works out of the box | Gap | `GitHubOptions.Client` is required (`github.go:106`), and `Limits` has no zero default. Two doc comments state the policy: "There is no convenience constructor that fills security-relevant defaults" (`types.go:806-807`) and "There is no convenience constructor that fills a client, token, or limits" (`types.go:200-201`). F5. |
| 2 | Exported config struct where configuration is serialized | Aligned | `releasespec.Spec` is the JSON form, with exported tagged fields, and `Parse` refuses unknown fields and nulls (`releasespec/spec.go:44+`). `Config` holds interfaces, so it cannot be serialized. `ResultDocument`, the check record and `ExecState` are tagged. |
| 2 | Options order-independent; conflicts documented | Aligned | "When an option is given twice, the last one wins" (`runoptions.go:11`). A nil or invalid option is an error before anything runs (`runoptions.go:98-110`). |
| 3 | `context.Context` first on every blocking or I/O operation; none stored | Partial | 80 of 221 exported functions and methods take ctx, and every I/O method of every exported interface does. Exceptions: `service.HandOffHop`, which waits up to 10 s with no ctx (`handoffhop.go:36-42`) and is called by `LoadHandOffEnv` and `ReportFunc`; and `cleanup_windows.go:245`, which calls a cancellable retry with `context.Background()`. The others are short local operations: `HandOffFunc`, `WriteHandOffResult`, `SameExecutable`, the `systemd` notify functions over a local socket, `systemd.Available`, `InstallSession.Close`. The one stored context is unexported and lives for one download (`updater.go:566`). F3. |
| 3 | Respect cancellation; document timeouts | Aligned | The single request constructor is `http.NewRequestWithContext` (`github.go:579`). Every child process uses `exec.CommandContext` through `internal/winproc` (`winproc.go:37`), except the deliberately detached child (`winproc.go:51`). Wait loops select on `ctx.Done()`. The defaults are documented: `cli.DefaultTimeout` 15 min, `DefaultLockTimeout` 5 s, poll 250 ms / 60 s / 10 s, `ghattest` 2 min. The `GitHubOptions.Client` doc says nothing about a timeout (`types.go:206-211`). |
| 4 | Error for expected failures; panic only on programmer error | Aligned | One `panic` in library code, `releasespec/spec.go:430` `mustJSON`, whose inputs (`[]string` and a struct of strings) cannot fail to marshal. No exported `Must*`. |
| 4 | Wrap with `%w` | Aligned | 568 `fmt.Errorf` calls in non-test files, 245 with `%w`. Three searches (two regular expressions and a verb-pairing parser) found no error passed through `%v`, `%s` or `%q`. `err.Error()` appears only at process or JSON boundaries (`cli/run.go:248`, `:291`; `service/handoff.go:196`). |
| 4 | Document the errors; sentinels or typed errors to distinguish cases | Partial | 21 sentinels: 13 in `selfupdate/errors.go:15-59`, `ErrNoCheckRecord`, `ErrCheckDeferred`, `ErrNoCredential`, six in `service/errors.go:11-27`, and `taskscheduler.ErrTaskDisabled`. One typed error, `*RateLimitError`, with `Unwrap` (`errors.go:64-85`). Other non-2xx statuses are an untyped string (F2), and a failed service command exposes only its sentinel class (`systemd/unit.go:232-237`). Errors are documented per sentinel, not per function. |
| 4 | No secrets in errors or logs | Partial | Error text names only the credential's source (`credentials.go:146-168`), and redirect URLs are redacted (`github.go:647-665`). But formatting a `Credential`, a `GitHubOptions` or a `*GitHubSource` prints the token (F1). |
| 5 | Semantic versioning; additive changes only | Aligned | 25 annotated tags, `v1.0.0` to `v1.15.2`. `make apicheck` (`scripts/check-api-compat.sh`, pinned `apidiff`) runs in the gate and CI, and in a scratch clone it reports the tree compatible with `v1.15.2`, `v1.5.0` and `v1.0.0`. The behaviour changes inside v1 (exit codes, `schema_version`, refusals) are listed per release in the migration guide. |
| 5 | Deprecate before removal; v2 on a new path | Aligned (N/A so far) | Nothing has been removed, and `grep -rn 'Deprecated:' --include=*.go` finds 0. No v2 exists. The policy is not written down (D6). |
| 5 | CHANGELOG; proper tags | Partial | Tags yes; no `CHANGELOG*` file, and no GitHub release for any of this module's tags. Release notes live in PLAN execution records (for example `0013-PLAN-build-and-stage-release-workflow.md`, "Release notes for v1.9.0") and in the migration guide's "From vX to vY" sections. |
| 5 | Design for extensibility | Aligned | Config structs that gain fields, optional interfaces, sealed `RunOption` and `Interaction`, and `apicheck`. |
| 5 | (found) A clean version list | Gap | `go list -m -versions` lists `v1.0.0` to `v1.4.1` under this path, though none resolves under it (F7). |
| 5 | (found) A floor adopters can meet | Gap | `go 1.27.1` shuts out the other supported Go line, 1.26, though the library code needs nothing from 1.27 (F8). |
| 6 | Godoc on every export, starting with its name | Aligned | 458 exported identifiers, none missing a doc comment or starting with another word. Checked by a `go/ast` program that was seen to fail on three planted declarations, and enforced by revive's `exported` rule (`.golangci.yml`). |
| 6 | Godoc covers behaviour, side effects, concurrency and errors | Partial | Concurrency is stated in four files only (`checker.go:27`, `stream.go:217`, `:268`, `selfupdatetest.go:99`, `:214`, `:248`). `Updater` refuses overlapping runs with `ErrConcurrentUpdate` (`runoptions.go:157-160`), and neither `Updater`'s doc nor `Run`'s says so (`types.go:854-856`). `ExitCode`'s doc says "The library never calls os.Exit" (`errors.go:91`), but `service.HandOffHop` calls it (`handoffhop.go:44`). |
| 6 | Runnable Examples | Partial | 36 `Example` functions. All 18 in `selfupdate` have `// Output:`. The backends', `codesign`'s, `ghattest`'s and `cli.ExampleHandOff` are compile-only, reasonably, since they touch the system. `selfupdate/service`, with 54 exported identifiers, has none. |
| 6 | Package comment with the simplest happy path | Partial | `selfupdate/doc.go` is sectioned prose with no code. The happy path is in `example_test.go`. |
| 6 | Common case obvious; advanced knobs separate | Partial | The advanced features are subpackages, opt-in. The common case needs the five-call composition (F5). |
| 7 | Prefer the standard library | Aligned | Three requirements, all `golang.org/x` (`mod` for `semver`, `sys`, `term`), with no indirect ones; `go mod tidy -diff` is clean; `depguard` enforces the floor (`docs/decisions/0008-MADR-enforce-import-rules-with-depguard.md`). |
| 7 | Small core; sub-packages for optional features | Partial | `selfupdate` is 8,813 of 19,441 public source lines (45%). Every extra is a subpackage: archives, codesign, provenance, the release spec, services and the CLI. |
| 7 | Easy to test (HTTP client, token source, filesystem, clock) | Partial | Public seams: `GitHubOptions.Client` and `APIBaseURL`, `selfupdatetest.NewGitHubServer` and `FakeSource`, `CredentialProvider`, `CheckStore`, `service.Runner`. No public clock: `CheckCached`'s ages use the internal `timeNow` (`checkcache.go:168`). `NewVersionProber` takes no runner. |
| 7 | High coverage of public behaviour; table tests; mock servers | Aligned | `go test -count=1 -cover ./...` exits 0: buildinfo 100.0%, codesign 97.7%, releasespec 97.5%, cli 94.9%, ghattest 94.7%, selfupdate 91.6%, archive 90.5%, selfupdatetest 90.0%, service 86.7%, the backends 81.6–84.2% (darwin; scm and taskscheduler mostly their non-Windows files). 9 fuzz targets, a Python manifest differential, live suites against real service managers. |
| 8 | Document concurrency safety | Partial | As above. `GitHubSource`, the installers and the four backends are guarded by internal mutexes, and their docs do not say so. |
| 8 | Immutable configuration after construction | Aligned | `Updater` fields are set by `New` and immutable (`types.go:854-856`); `NewGitHubSource` clones the client (`github.go:126`). |
| 8 | No locks across I/O | Partial | `GitHubSource` calls the credential provider and the network with no lock held (`github.go:69-71`, `:750-751`). `installSession` holds its mutex across renames and fsync by design (`session.go:93-112`, `:158`, `:243`, `:406`, `:441`), bounded by `LockTimeout`. |
| 8 | Cancellation for long operations | Aligned | As in §3. `go test -race -count=1 ./selfupdate/` passes in 25 s. |
| 9 | Verify signatures and checksums before applying; pin the key | Partial | Checksums are mandatory: `SHA256SUMS` must exist (`assets.go:113-114`), the staged digest must equal the manifest entry and the GitHub digest when present, compared in constant time (`verify.go:8-25`), and the release must be immutable (`checker.go:183`). No publisher signature is verified, by decision (`selfupdate/doc.go:100-103`; 0004-MADR "Release signing: deferred, with the hook built"). The hook, `ManifestVerifier`, runs before any binary byte is fetched (`updater.go:260`), and `ghattest` is an opt-in provenance check that needs `gh`. F10. |
| 9 | Atomic replacement with rollback | Aligned | Unix: hard-link backup, journal, `rename`, directory fsync, restore on a failed sync (`replace_unix.go:51-97`). Windows: `MoveFileEx` with `REPLACE_EXISTING` and `WRITE_THROUGH`, retried while the image is busy (`replace_windows.go:34-125`). `PostInstall` probes before commit and rolls back on failure (`session.go:284-305`). Two-phase sessions roll back a managed install. An interrupted update's journal is recovered (`journal.go`). After a commit, the way back is an exact lower `TargetVersion`, `KeepPrevious` or `KeptBackups`. |
| 9 | Platform and architecture selection | Aligned; Partial on variants | `NewExactAssetSelector` (`assets.go:59-65`), and `archive.NewSelector` with `FleetName` and `GoReleaserName`. `Platform` is `{OS, Arch}` only (`types.go:45-50`), so GOARM, GOAMD64, musl and universal darwin builds need a custom name function or selector. F12. |
| 9 | Check-only and apply modes; explicit progress and failure | Aligned | `CheckOnly`, `Checker`, `DryRun`; typed events with `EventFailed` sent from a defer on every failure (`updater.go:128-137`); text, JSON Lines, multi and stream reporters. Byte progress is opt-in through `Config.ProgressInterval` (0004-MADR amendment A1). |
| 9 | Source abstraction for a later GitLab | Partial | `ReleaseSource` is small (3 methods), and `FakeSource` proves the seam. The contract is GitHub-shaped: `Release.ID`, `Asset.ID`, `Asset.State == "uploaded"`, GitHub's `sha256:<hex>` digest, and docs that say "one GitHub release" (`types.go:230-260`). The real blocker is the `Immutable` gate (`checker.go:183`), which the deferred §6 record of 0004 owns. GitHub Enterprise works: a probe with base `<srv>/api/v3` requested `/api/v3/repos/o/r/releases/latest`, though no test covers a path-prefixed base. F11. |
| 9 | Trust model documented | Partial | `selfupdate/doc.go` "# Integrity" and README say what is proven. `ghattest/doc.go` has "What it proves / What it does not prove". There is no single user-facing page, no `SECURITY.md`, and the freeze-attack limit appears only in `0004-REPORT-release-signing-research.md`. F9. |
| 9 | Credentials as token sources, from env, files or stores; never logged | Aligned; Partial on logging | `CredentialProvider` takes ctx and a `CredentialRequest`; `EnvCredential`, `ChainCredentials`, `PromptCredential`, a refresh after 401, and a `CredentialObserver` (`credentials.go:29-135`). There are no file or keychain providers, which is consistent with the dependency floor, and callers implement the interface. The formatting leak is F1. |
| 9 | Common auth flows (API key, OAuth2, workload identity) | N/A | The module authenticates to GitHub with a token only. |
| 10 | gofmt; `go test`; `go vet` | Aligned | `gofmt -l` on every tracked file is empty; `go vet` with `CGO_ENABLED=0` for linux, darwin and windows is empty; `go test ./...` exits 0. |
| 10 | Public API reviewed for accidental exports | Partial | No review has been recorded. The candidates are listed in the §1 row above. |
| 10 | Examples and a 30-second README quickstart | Partial | README has a `go get` line and a package table, but no Go code block and no pkg.go.dev badge; it points to `example_test.go`. |
| 10 | CHANGELOG entry and tag | Partial | As in §5. |
| 10 | Minimal, justified dependencies | Aligned | As in §7. |

### Facts that drive the decisions [observed 2026-10-19, `main` at `5c0ab0d`]

* **F1. A token prints when its holder is formatted.** No type that holds
  a secret has `String`, `GoString`, `Format` or `LogValue`. The audit's
  probe, re-run for this record, printed (addresses elided):

  ```text
  Credential %s : { ghp_SECRET env:GH_TOKEN}
  Credential %v : { [103 104 112 95 83 69 67 82 69 84] env:GH_TOKEN}
  Credential json: {"Header":"","Value":"Z2hwX1NFQ1JFVA==","Source":"env:GH_TOKEN"}
  slog: c="{Header: Value:[103 104 112 95 83 69 67 82 69 84] Source:env:GH_TOKEN}"
  GitHubOptions %+v: {… Token:ghp_OPTSECRET …}
  *GitHubSource %v: &{{o r} 0x… 0x… x/1.0 ghp_OPTSECRET true …}
  ```

  `%v` on the source pointer, the value a program is most likely to log,
  prints the token held in its unexported `token` field. The JSON form is
  not a leak in the same sense. `CredentialObserver`'s doc says "A program
  uses it to save a token only once it is known to work"
  (`credentials.go:47-48`), so a program may marshal a `Credential` on
  purpose.
* **F2. HTTP failures other than a rate limit are untyped.**
  `mapStatus` returns `fmt.Errorf("selfupdate: github http %d: %s",
  resp.StatusCode, diag)` (`github.go:818-824`). A caller cannot tell 401,
  403, 404 or 5xx apart without parsing text. `*RateLimitError` is the
  only typed error.
* **F3. Two waits ignore the caller's context.**
  * `service.HandOffHop()` takes no ctx, and in the real run "waits up to
    10 s for the hop to exit" (`handoffhop.go:36-42`) in a
    `time.Sleep(5 * time.Millisecond)` loop (`hopwait_unix.go:17`).
  * `writeCleanupEntries` calls `moveFileReplace(context.Background(), …)`
    (`cleanup_windows.go:245`), a retry that `retryBudget(ctx)` bounds
    (`replace_windows.go:102`). It therefore takes the default budget
    whatever the caller's deadline says.
* **F4. Concurrency contracts exist in code, not in docs, and one write
  races.**
  * `execRun` refuses an overlapping run: `if
    !u.running.CompareAndSwap(false, true) { return Result{},
    ErrConcurrentUpdate }` (`runoptions.go:157-160`). `ErrConcurrentUpdate`'s
    doc says so; `Updater`'s and `Run`'s do not.
  * `taskscheduler/com_windows.go:49` declares `var lastCoInit hresult`
    "for TestCOMLayout", and `withCOM` writes it with no synchronisation
    (`:152`). `withCOM` runs for each COM operation. Two `Task` values used
    from two goroutines would write it concurrently. This is [unverified]
    under `-race`, which CI does not run on Windows.
* **F5. "No convenience constructor" is a stated policy with no
  recorded rationale.** Both doc comments (`types.go:200-201`, `:806-807`)
  say it. `grep -rni 'convenience' docs/` finds no record that decided
  it. Every default it withholds is either safe and constant
  (`DefaultLimits`, the strict policy, the exact selector, the standalone
  installer) or a choice the program must make (the repository, the
  product, the User-Agent).
* **F6. Callers implement the large interfaces, so those interfaces are
  frozen.** The `apidiff` experiment at the version `check-api-compat.sh`
  pins gave:

  ```text
  Incompatible changes:
  - Open.B: added
  Compatible changes:
  - Cfg.Y: added
  - New: added
  - Opt: added
  - Sealed.B: added
  ```

  Sealing an existing interface now would itself break every external
  implementer. Changing any of the 16 constructors to return a concrete
  type is an incompatible result-type change under `apidiff`.
* **F7. The pre-rename tags pollute the version list, and `retract`
  removes them.**
  * `go list -m -versions github.com/maccavelli/go-selfupdate-lib` lists
    `v1.0.0 … v1.4.1` before `v1.5.0`. Each declares
    `github.com/maccavelli/go-core-lib`, so none resolves under this path
    (`0009-PLAN-rename-to-go-selfupdate-lib.md`, live verification).
  * `go list -m -json github.com/maccavelli/go-core-lib@latest` answers
    `"Version": "v1.15.2"` with no `Deprecated` field, so the old path's
    deprecation notice at `v1.4.1` is never shown. 0009-MADR §7 named this
    risk, and 0009's PLAN recorded that it happened. No change in this
    repository can fix the old path.
  * **Probe (this record).** A file `GOPROXY` held `v1.0.0`, declaring
    another path, then `v1.1.0` and `v1.2.0`. The probe ran `go list -m
    -versions`, first without and then with `retract v1.0.0 // declared
    another module path` in `v1.2.0`'s `go.mod`:

    ```text
    == retract=0
    example.com/m v1.0.0 v1.1.0 v1.2.0
    == retract=1
    example.com/m v1.1.0 v1.2.0
    ```

    `-retracted` still lists all three. A `retract [v1.0.0, v1.4.1]` in
    the next release's `go.mod` therefore hides the pre-rename tags from
    `go list -m -versions` and `-u`.
* **F8. The library code needs nothing from Go 1.27.**
  * Go's release policy supports each major release "until there are two
    newer major releases". Go 1.27 shipped in August 2026, so 1.26 and
    1.27 are supported today. `go 1.27.1` requires the newer one. A 1.26
    program with `GOTOOLCHAIN=local` cannot build; otherwise the go
    command downloads a newer toolchain for it. All three `x/` requirements
    declare `go 1.25.0`.
  * **Probe (this record).** In a scratch clone, `go mod edit -go=1.26.0
    -toolchain=none`, then `GOTOOLCHAIN=local` with Go 1.27.2:
    * `go build ./...` exits 0 for linux, darwin and windows;
    * `go vet ./...` fails only in the test file
      `selfupdate/managed_stopped_test.go:95`, `:99` and `:226`: "use of
      promoted field … in struct literal … requires go1.27 or later", a
      language change;
    * with every `_test.go` removed, `go vet ./...`, including the
      `stdversion` analyzer, exits 0 with no output for linux, darwin,
      windows and freebsd.

    The control: a planted `strings.CutLast`, new in Go 1.27 per
    `api/go1.27.txt`, made `go vet ./buildinfo/` fail with
    "strings.CutLast requires go1.27 or later (file is go1.26)", and
    removing it made vet pass.
  * **The fleet rule.** magic-cli-remote's
    `docs/decisions/0169-MADR-standardize-toolchains-on-current-supported-advisory-free-releases.md`
    D2 moves each module's `go` directive to 1.27 "repository by
    repository after each passes its own gates", and says that until a
    repository has passed, "the floor is 1.26.8 … Go 1.26 remains an
    acceptable fallback until 1.28 ships (~2027-02)". This repository chose
    1.27.1 under that rule
    (`0001-MADR-scaffold-shared-go-library.md`,
    `0018-MADR-move-toolchain-to-go-1-27-2.md`). Whether 1.26.8 has an
    advisory today is [unverified]. D8's PLAN checks it with `govulncheck`
    before the line moves.
* **F9. The repository's own defences are not set.** Read through the
  GitHub API on 2026-10-19:
  * private vulnerability reporting is `{"enabled":false}`;
  * the repository has no rulesets (`[]`), so creating a `v*` tag is not
    restricted;
  * Dependabot security updates are disabled, and vulnerability alerts
    answer 404 (off);
  * `main` is not protected;
  * secret scanning and push protection are enabled;
  * there is no `SECURITY.md` anywhere in the tree.

  0004-REPORT names a tag ruleset restricting `v*` to the owner as part of
  the realistic defence against repository compromise ("What signing
  would add today").
* **F10. Signing was deferred on evidence that still holds, and its design
  is ready.**
  * 0004-MADR decided on 2026-09-30: "defer signing. Build and test the
    hook now". Its reasons: with a CI-held, auto-approved key, "repository
    write access is effectively key access", while HTTPS, immutable
    releases and `SHA256SUMS` cover the other threats.
  * 0004-REPORT keeps a full design: a C2SP signed note over `SHA256SUMS`
    through `golang.org/x/mod/sumdb/note`, a module already required, with
    Ed25519, deterministic output, no clock, rotation built in, and keys
    embedded as Go constants. It lists `selfupdate/verify/signednote` as
    open work.
  * Signing becomes necessary on a host that cannot keep files unchanged
    (F11).
* **F11. Non-GitHub hosts wait on one gate.** `checker.go:183` refuses
  any release whose `Immutable` is false, for every source. A GitLab
  source would have to claim immutability falsely. 0004-MADR §6 reserves
  the remedy, `Release.Integrity` and `Config.AcceptMutableReleases` gated
  on a `ManifestVerifier`, to its own record, which the owner deferred
  along with ocp-login's migration. The rest of the contract is
  GitHub-worded but host-neutral in use.
* **F12. Variant builds go through the name function.**
  `archive.SelectorOptions.Name` maps a `Platform` to any asset name, so a
  universal darwin asset works today. GOARM, musl and GOAMD64 need a
  selector that reads beyond `runtime.GOOS` and `runtime.GOARCH`, and
  `GoReleaserName` refuses `arm` and `mips*` (`archive/select.go:177-195`).
  No consumer has asked for them.

### What is already aligned and stays

The module already does most of the list: validated construction,
order-independent sealed run options, safe defaults, `%w` throughout, 21
sentinels, complete godoc, 36 Examples, three `x/` requirements under
`depguard`, `apicheck` on every commit, coverage of 81.6–100%, fuzzing,
race tests, mandatory checksums, an immutable-release gate, atomic and
journalled replacement with rollback, check-only and dry-run modes, typed
events, and a small source seam. No decision below changes any of that.

## Decision Drivers

* **No exported API breaks.** `make apicheck` stays compatible with
  `v1.15.2`. A change that `apidiff` accepts but that changes behaviour a
  program can see is named as such, and is listed in the migration guide.
* **Security defaults never weaken** (the owner's standing rule, 0004-MADR
  §6: "each needs its own record").
* **No new module requirement.** AGENTS.md: "No module may be required
  without a MADR in this repository that names it." Every decision below
  is satisfied by the standard library and the three requirements.
* **Speculative API is built when it is coherent** (the owner,
  2026-10-01). It is cut when it is incoherent, adds a dependency, or
  weakens a default.
* **Exported, JSON-tagged fields over accessors** where a type is data
  (the owner, 0010-PLAN S3 deviation D2).
* **Each fix is seen to fail first** (AGENTS.md, "Pre-add checks").
* **Earlier owner decisions are reopened explicitly, never silently.**

## Considered Options

### D1: secrets when a value is formatted

* **D1-A.** Add redacting formatters, without touching JSON:
  * `Credential` gains `String`, `GoString`, `Format` and `LogValue`,
    printing `Header` and `Source` and `Value:<redacted>`;
  * `*GitHubSource` gains `String`, `Format` and `LogValue`, printing the
    repository and API origin;
  * `GitHubOptions` gains the same, with `Token:<redacted>`.

  Each goes on the receiver that does not copy a lock.
* **D1-B.** As D1-A, plus `MarshalJSON` on `Credential` that omits
  `Value`.
* **D1-C.** Document "do not format these" and change nothing.

### D2: errors a caller can distinguish

* **D2-A.** An exported `*HTTPError{StatusCode int; Diagnostic string}`
  from `GitHubSource` for every non-2xx it does not already map, with the
  same message text as today. An exported
  `*service.CommandError{Step, Name string; ExitCode int; Stderr string}`
  that unwraps to today's sentinel. An "Errors" paragraph in the docs of
  `Updater.Run`, `RunWith`, `Start`, `Checker.Check`, `CheckCached`, and
  each `Lifecycle` method of the four backends.
* **D2-B.** The docs paragraphs only.
* **D2-C.** No change.

### D3: context on every wait

* **D3-A.** Add `service.HandOffHopContext(ctx) error`, with
  `HandOffHop()` calling it with `context.Background()`, and a
  ctx-taking `LoadHandOffEnvContext`. Thread the caller's ctx into
  `writeCleanupEntries`, which is unexported. Leave the local socket and
  stat calls as they are, with a sentence saying why.
* **D3-B.** Thread the ctx into the unexported code only.
* **D3-C.** No change.

### D4: concurrency contracts

* **D4-A.** One sentence of contract on each exported type with state:
  * `Updater`: "Run, RunWith and Start may be called from any goroutine;
    an overlapping run returns ErrConcurrentUpdate";
  * `GitHubSource`, `StandaloneInstaller` and `ManagedInstaller`;
  * the four backends' types;
  * `InstallSession`: its methods serialize.

  Also make `lastCoInit` an `atomic.Uint32`, and run `checkNoLeak` in
  `selfupdate/service` and its backends' tests.
* **D4-B.** The doc sentences only.
* **D4-C.** No change.

### D5: a one-call common path

* **D5-A.** An additive constructor that composes today's parts with
  their safe defaults, beside `New`, which is unchanged. In outline:

  ```go
  // NewGitHubUpdater returns an Updater for a standalone program released
  // on GitHub with the fleet's asset names.
  func NewGitHubUpdater(o GitHubUpdaterOptions) (*Updater, error)

  type GitHubUpdaterOptions struct {
      Repository Repository  // required
      Product    string      // required: the asset prefix
      Version    string      // required: the running version, for UserAgent
      Platforms  []Platform  // nil: the running platform only
      Client     *http.Client // nil: a client with transport timeouts and no whole-request timeout
      Stdin      io.Reader   // nil: os.Stdin
      Stderr     io.Writer   // nil: os.Stderr
  }
  ```

  It uses `DefaultLimits`, the strict version policy, the exact selector,
  `StandaloneInstaller` with `InstallOptions{}`, a text reporter and a
  terminal confirmer. The two "no convenience constructor" sentences are
  reworded to "`New` fills no defaults; `NewGitHubUpdater` composes the
  safe ones". README gains a quickstart with it.
* **D5-B.** Treat zero `Limits` as `DefaultLimits()` in `Config` and
  `GitHubOptions`.
* **D5-C.** A README quickstart with today's five calls, and no new API.

### D6: how the API evolves

* **D6-A.** Write the policy the module already follows into
  `docs/architecture.md` and the extending guide:
  * additive only within v1, enforced by `apicheck`;
  * an interface callers implement never gains a method; a new capability
    is a new optional interface, checked by type assertion, as
    `EnabledLifecycle` and `ReleaseLister` are;
  * an interface only the library implements is sealed from its first
    release;
  * a new constructor returns a concrete type unless the value is a pure
    strategy;
  * a field or function to be retired gets `// Deprecated:` naming its
    replacement, and is removed only in a `/v2` module;
  * every behaviour change a program can see is listed in the migration
    guide and the changelog (D7).
* **D6-B.** Seal `Lifecycle`, `InstallSession`, `TwoPhaseSession` and
  `ReleaseSource` now.
* **D6-C.** No written policy.

### D7: the version list and the changelog

* **D7-A.** `go.mod` gains `retract [v1.0.0, v1.4.1] // declared
  github.com/maccavelli/go-core-lib; use v1.5.0 or later` in the next
  release. A root `CHANGELOG.md` in Keep a Changelog form gets one entry
  per release from `v1.5.0`, plus a line for the rename. Each entry links
  the migration guide's section and the release's records. Every later
  release adds its entry in the same commit as its migration-guide
  section.
* **D7-B.** As D7-A, without the changelog: the migration guide is the
  changelog, and README links it as such.
* **D7-C.** No change.

### D8: the Go floor

* **D8-A.** Set `go 1.26.8`, which the fleet rule allows as the floor, and
  keep `toolchain go1.27.2`, so this repository's own builds and gates
  stay on 1.27.2. Rewrite the three test lines that use the 1.27 literal
  form. Add a CI job that builds, vets and tests with the newest 1.26
  patch (`GOTOOLCHAIN=local`). The line moves to 1.27 when 1.28 ships, or
  when the library first needs a 1.27 feature, in a record.
* **D8-B.** Keep `go 1.27.1`.
* **D8-C.** `go 1.26.0`, below the fleet rule's floor.

### D9: the trust model and the repository's defences

* **D9-A.** A root `SECURITY.md`, linked from README, with:
  * how to report a vulnerability (GitHub private vulnerability
    reporting);
  * the supported versions (the newest v1 minor);
  * the trust model in short.

  A full "Trust model" section in `docs/architecture.md` that states:
  * what each run proves: TLS to the API origin, the immutable-release
    gate, `SHA256SUMS` and the GitHub digest, the exact tag;
  * what it does not prove: a publisher signature unless a verifier is
    configured; that `SHA256SUMS` shares the binary's origin; freshness,
    since a freeze attack is mitigated only by TLS to the API origin;
    the redirect hosts, which the checksum guards rather than an
    allow-list;
  * what the opt-in verifiers add.

  Five repository settings are recommended to the owner, who alone can
  change them:
  * private vulnerability reporting on;
  * Dependabot alerts on;
  * a tag ruleset that limits creating `v*` tags to the owner;
  * a rule on `main` that blocks force-push and deletion and requires no
    pull request, so commits straight to `main` keep working;
  * Dependabot security-update pull requests left off, since a
    requirement changes only with a record here.
* **D9-B.** `SECURITY.md` and the trust-model section, with no settings
  recommended.
* **D9-C.** No change.

### D10: release signing (reopens 0004's deferral)

* **D10-A.** Build 0004-REPORT's design, opt-in end to end:
  * `selfupdate/verify/signednote`, a `ManifestVerifier` over
    `SHA256SUMS.note` with `Config{Repository, Keys, RequiredFrom}`;
  * an unreleased signer, `internal/cmd/selfupdate-note`;
  * a `sign` input on `publish-selfupdate-release.yml`, default `false`;
  * `SHA256SUMS.note` reserved in `verify-selfupdate-release.sh`.

  A program turns it on when it embeds keys. 0004-REPORT's questions 2–5
  (key per repository, key names, required reviewers, the backup key's
  place) are settled per consumer at adoption. 0004-MADR gains an
  amendment pointing here.
* **D10-B.** The verifier package only; no signer and no workflow input.
* **D10-C.** Keep the deferral; D9 states the gap plainly.
* **D10-D.** Make a signature mandatory.

### D11: the source seam for other hosts

* **D11-A.** Make the contract host-neutral in its docs, and give
  implementers a conformance test, without touching the `Immutable`
  gate:
  * `Release`, `Asset` and `ReleaseSource` docs say what each field means
    to the Updater, not to GitHub: `ID` is opaque to the Updater;
    `State` must be `"uploaded"` when the asset is complete;
    `Digest` is `sha256:<hex>` or empty; `URL` is the release's page;
  * `selfupdatetest.CheckReleaseSource(t, src, fixtures)` checks a
    `ReleaseSource` against the contract the Updater relies on: the
    `ErrNoRelease` wrapping, the `OpenAsset` bounds, and the field rules;
  * a test covers a path-prefixed GitHub Enterprise base (`/api/v3`).

  The §6 record of 0004 (`Release.Integrity`,
  `AcceptMutableReleases`, a GitLab source) stays deferred.
* **D11-B.** Write the §6 record now, with a `selfupdate/gitlab` source
  that requires D10's verifier.
* **D11-C.** No change.

### D12: testability and observability seams

* **D12-A.**
  * `CheckerConfig.Now` and `Config.Now`, of type `func() time.Time`,
    with nil meaning `time.Now`. They set the clock for check-record ages
    and the progress throttle.
  * `NewVersionProberWith(VersionProbeOptions)`, whose `Run` field is a
    func type, since `selfupdate` cannot import `service`.
  * `NewSlogReporter(*slog.Logger) Reporter`, a standard-library adapter
    that logs each event at a level its kind decides.

  The internal seams stay internal.
* **D12-B.** `NewSlogReporter` only.
* **D12-C.** No change.

### D13: platform variants

* **D13-A.** Document the universal-darwin and musl recipes (a `Name`
  function, or a selector), and record `Platform.Variant` as future work
  for when a consumer needs it, with the release spec and workflows
  together.
* **D13-B.** Add `Platform.Variant` now.

### D14: documentation gaps not covered above

* **D14-A.** Close them:
  * an `Example` for `selfupdate/service` (`PollHealthy` over a func,
    with `// Output:`);
  * a code block with the happy path in `selfupdate/doc.go`;
  * a pkg.go.dev badge in README;
  * `ExitCode`'s doc reworded to "package selfupdate never calls
    os.Exit";
  * `GitHubOptions.Client`'s doc says that `Client.Timeout` caps the whole
    asset download, and recommends a context deadline or a transport's
    `ResponseHeaderTimeout` instead;
  * a recorded review of the exported surface, which marks nothing
    deprecated unless D6's policy calls for it.
* **D14-B.** No change.

## Decision Outcome

Chosen options (the owner, 2026-10-19, answering each decision in turn):
"D1-A", "D2-A", "D3-A", "D4-A", "D5-A", "D6-A", "D7-A", "D8-A", "D9-A"
without its rule on `main`, "D10-A", "D11-A", "D12-A", "D13-A" and
"D14-A", all released together as `v1.16.0`.

The owner's answers changed two things from the proposal:

* **D9-A without the `main` rule.** The owner chose D9-A "but do not
  block operations on main". The record recommends four settings:
  * private vulnerability reporting on;
  * Dependabot alerts on;
  * a tag ruleset limiting `v*` tag creation to the owner;
  * Dependabot security-update pull requests left off.

  It recommends no rule, protection or ruleset on `main`.
* **One release.** The owner chose a single `v1.16.0` for every
  decision, signing included, in place of the proposed split into four
  units over `v1.16.0` and `v1.17.0`.

D8 was then amended to `go 1.26.9`; see "Amendment A1" below.

The reasons for each choice:

* **D1-A** removes the leak the owner's list names first under security,
  at the formatting entry points a program uses: `fmt`, `%v` on a pointer,
  and `slog`. It leaves JSON alone, because `CredentialObserver` exists so
  a program can save a token (F1), and a `MarshalJSON` that drops `Value`
  would silently break that.
* **D2-A** lets a caller act on 401 versus 5xx and on a failed service
  command without parsing text. Same message text, new types:
  `errors.As` starts to work, and nothing that matched before stops
  matching.
* **D3-A** brings the last two waits under the caller's context. The
  `…Context` names follow the standard library (`net.DialContext`,
  `signal.NotifyContext`), and the existing functions keep their
  signatures.
* **D4-A** writes down guarantees the code already keeps, and fixes the
  one unsynchronised write. Without D4-A's leak check, a backend goroutine
  leak would go unseen.
* **D5-A** gives the list's "NewClient(required) works out of the box"
  without weakening anything, because every default it fills is the value
  the docs and Examples already prescribe. D5-B was rejected because it
  turns a documented error (zero limits) into silent success. D5-C leaves
  the five calls.
* **D6-A** is the module's practice, so writing it costs nothing and
  binds future records. D6-B breaks every external `Lifecycle` and source
  (F6).
* **D7-A** cleans the version list with a mechanism the probe proved
  (F7). A changelog is the list's explicit ask, and the migration guide is
  too long to serve as one. The changelog is an index of releases that
  links each migration-guide section, rather than a second copy of it.
* **D8-A** opens the module to the supported 1.26 line, which F8 proves
  costs nothing in library code. It sits inside the fleet rule's stated
  floor. The repository's own builds stay on 1.27.2, so the advisory
  posture 0018 bought is unchanged for everything this repository builds.
* **D9-A** puts the trust model where adopters look, and gives
  `SECURITY.md` a working channel. The tag ruleset closes the part of
  the threat that 0004-REPORT ranks first, repository compromise, that
  creates a release tag, which F9 shows is open today. Leaving `main`
  unruled is the owner's choice, and it keeps commits straight to `main`
  and every other operation on it as they are.
* **D10-A** answers the list's "always verify signatures … pin the public
  key" in the only way v1 allows, as an opt-in. It is the design 0004
  already chose for the day it is needed, at no dependency cost (F10).
  Its value is stated honestly:
  * Under an auto-approved CI key it adds little against repository
    compromise (0004-REPORT).
  * It adds more with required reviewers on the signing environment.
  * It is a prerequisite for any host other than GitHub (F11).

  D10-D was rejected: every existing release is unsigned, so a mandatory
  check breaks every program on its next update.
* **D11-A** delivers what the list asks of the source seam, that a later
  host can be added without breaking callers, and adds no new policy. The
  policy, accepting mutable releases, keeps its own record (0004 §6) and
  stays the owner's to schedule.
* **D12-A** adds the injectable clock and runner the list names, plus a
  `slog` bridge that makes the event stream usable as logs. Each is a nil
  default on an existing struct or a new function.
* **D13-A** answers the variant question for today's consumers with no
  schema change, and keeps `Variant` for a record that can move the
  spec, the workflows and the installers together.
* **D14-A** closes the remaining documentation rows of the scorecard.

The PLAN, `0025-PLAN-align-with-go-sdk-design-practices.md`, implements
every decision for one release, `v1.16.0`. It may stage the work in
phases with a commit each, and it tags once, after the last phase. The
signing workflow input (D10) is rehearsed live on a scratch repository
before the tag, as the proposed separate signing release would have been.

### Amendment A1 (2026-10-19): the floor is `go 1.26.9`, and three facts are corrected

While the PLAN was being written, the owner said: "set the floor to go
1.26.9". D8-A is kept with that one change.

* **The floor.**
  * `go.mod`'s line becomes `go 1.26.9`, not `go 1.26.8`.
  * `toolchain go1.27.2`, the 1.26 CI job, the three test-line rewrites,
    and the move to 1.27 when 1.28 ships are as D8-A says.
* **The release list.**
  * `https://go.dev/dl/?mode=json&include=all`, read on 2026-10-19,
    lists `go1.26.9` as the newest 1.26 patch: `go1.27.2`, `go1.27.1`,
    `go1.27.0`, then `go1.26.9`, `go1.26.8`.
  * 1.26.9 is above the fleet rule's 1.26.8 floor, so D8-A still sits
    inside magic-cli-remote's 0169 D2.
  * F8's probe used `-go=1.26.0`, below either floor, so its result
    holds for 1.26.9.
  * The CI job's "newest 1.26 patch" is `1.26.9` today.
  * **The advisory check.** `govulncheck` v1.8.0 (database of
    2026-10-08), in a scratch clone with its `go` line at 1.26.x:
    * `GOTOOLCHAIN=go1.26.8` exits 3: "Your code is affected by 10
      vulnerabilities from the Go standard library" (GO-2026-6603, -6604,
      -6605, -6607, -6608, -6610 to -6613, -6617), each "Fixed in
      …@go1.26.9";
    * `GOTOOLCHAIN=go1.26.9` exits 0: "No vulnerabilities found";
    * Go 1.27.2 also exits 0.

    So 1.26.8 would have admitted ten advisories that 1.26.9 fixes. The
    owner's floor is the advisory-free one.
  * **A 1.26.9 toolchain passes.** In the same clone, with the three test
    lines and ten test fixtures' `go.mod` lines moved to 1.26.9,
    `GOTOOLCHAIN=go1.26.9 go vet ./...` (linux, darwin, windows, freebsd)
    and `go test -count=1 ./...` exit 0. The fixtures are listed in the
    PLAN.
* **Corrections found while the PLAN was written.**
  * The `GitHubOptions` sentence is at `types.go:201-202`, not
    `:200-201` (F5 and the scorecard).
  * "The library never calls os.Exit" appears three times: in
    `ExitCode`'s doc (`errors.go:91`), `Run`'s (`updater.go:119`) and
    `Result`'s (`types.go:125`). D14-A's rewording covers all three.
  * 0004-REPORT's claim that "Environment secrets cannot be passed through
    `secrets:` to a reusable workflow" (its "Signing in the workflow") is
    contradicted by GitHub's reusable-workflow documentation. That
    documentation says the caller must pass the secret, by name or with
    `secrets: inherit`, and that a secret the caller does not pass
    "resolves to an empty string … The workflow run doesn't show an
    error." D10-A's workflow is built on the documented behaviour, and the
    report gains a correction under its "Since" heading. The PLAN has the
    details.
* **Refinements of the outlines, from the code.** The PLAN's facts fix
  shapes that the options above gave only in outline. No choice changes.
  * **D5-A.**
    * `GitHubUpdaterOptions.Stdin` is an `*os.File`, because
      `NewTerminalConfirmer` takes one (`confirmer.go:43`).
    * The struct gains `APIBaseURL *url.URL` (nil means
      `https://api.github.com`). That is how GitHub Enterprise and
      `selfupdatetest.NewGitHubServer`, which serves at its own `APIBase`,
      reach it.
    * An empty `Version` is refused, since `UserAgent` never fails
      (`useragent.go:15-21`).
  * **D2-A.** `HTTPError` carries a `Source` field (`"github"`), so its
    text stays "selfupdate: github http N: …" and a later source can reuse
    the type. `service.CommandError` carries `Backend`, `Step`, `Name`,
    `ExitCode` and `Message`. Its text is the format the three command
    backends already share, "selfupdate: <backend>: <step> <name>: exit N:
    <message>" (`systemd/unit.go:233`, `launchd/job.go:323`,
    `service/execreconciler.go:212-221`). The sentinels keep wrapping it
    as they wrap the string today.
  * **D11-A.** The conformance check returns an error,
    `selfupdatetest.VerifyReleaseSource(ctx, src, SourceCase) error`, in
    place of taking a `testing.TB`. That way its own tests can assert that
    it fails on a broken source.
  * **D10-A.**
    * Signing runs in its own job, `sign`, with a fixed `environment:
      selfupdate-signing`. The caller passes the secret
      `SELFUPDATE_NOTE_KEY` by name, and the verifier key as the input
      `note-verifier-key`. The publish job never sees the private key, and
      its existing steps keep their guards.
    * The statement's `repository` is `<host>/<owner>/<name>`, taken from
      `github.server_url` and `github.repository`.
    * The note is `<manifest name>.note`, which is `SHA256SUMS.note` for
      this fleet's releases.
    * `releasespec` reserves that name as `verify-selfupdate-release.sh`
      does, with a parity case.

### Amendment A2 (2026-10-19): the receipt write after a commit is bounded, not cancelled

F3's second bullet was right about the defect and wrong about the fix it
implied. The receipt write at `cleanup_windows.go:245` is reached from
two places:

* from session start, before anything changes (`session.go:605`);
* from `commitLocked`, after the new binary is in place
  (`session.go:213-228`).

Honouring cancellation in the second would leave a busy backup off the
receipt after a committed update. The owner chose, during the PLAN's S1
(deviation D1):

* session start passes the caller's cancellable context, with the
  session's timeout as the retry budget;
* the commit path passes `context.WithoutCancel`, with the session's
  `LockTimeout` as the budget, as the rollback already does
  (`session.go:297`).

The fixed 5 s default is gone from both.

### Amendment A3 (2026-10-19): what D1-A redacts, and what it cannot

slog calls `LogValue` only on the value it is given. Its JSON handler
encodes anything else with `encoding/json`, and D1-A leaves that alone so
a `CredentialObserver` can save the accepted credential. A `Credential`
held inside another value, such as a struct field, is therefore printed
by slog's JSON handler. Every fmt verb, slog's text handler, and both
handlers given the `Credential` itself redact it.

The owner kept D1-A (the PLAN's deviation D2). `Credential`'s doc says
this, and a test pins it. The Consequences' first bullet holds for a
credential logged directly or through fmt.

### Amendment A4 (2026-10-19): the clock is a `Clock` interface

D12-A's `CheckerConfig.Now` and `Config.Now`, of type `func() time.Time`,
would make `Checker` and `CheckerConfig` non-comparable, and `apicheck`
reports that as incompatible with `v1.15.2` (the PLAN's deviation D4).
The owner chose an interface instead:

* `type Clock interface{ Now() time.Time }`, with the adapter
  `ClockFunc`;
* fields `Config.Clock` and `CheckerConfig.Clock`, nil meaning the real
  time.

D12-A is otherwise unchanged.

### Amendment A5 (2026-10-19): the repository settings wait

D9-A recommended four repository settings, among them private
vulnerability reporting and a tag ruleset limiting `v*` to the owner. At
closing, the owner chose to keep them off for now (the PLAN's deviation
D6). `SECURITY.md` says reporting is not yet on. The Consequences' and
D9-A's claim, that the tag ruleset closes the repository-compromise
threat, therefore does not hold until the owner applies it; the trust
model's advice to restrict `v*` tags stands.

### Amendment A6 (2026-10-19): the leak check compares goroutines, not a count

Accepted by the owner ("approved, proceed"), and built in the PLAN's
S16 (deviation D7).

D4-A's leak check, `internal/testproc.CheckNoLeak`, notes
`runtime.NumGoroutine()` when a test calls it. It fails the test only
while more goroutines run than that. A goroutine that already ran when
the count was taken, and ends during the test, therefore hides one that
the test leaked. The count is back where it started, and the check
passes at once.

The evidence:

* **CI on `main`** at `b6e325d`, a commit of two documents (run
  `39352949466`, `validate (ubuntu-24.04)`, `go test -race`):
  `TestCheckNoLeakSeesLeak (0.01s)`, `testproc_test.go:162: a blocked
  goroutine reported [], want one goroutines: error`. The same code
  passed in three runs earlier the same day.
* **This Mac,** `go test -race -count=200 -run TestCheckNoLeakSeesLeak
  ./internal/testproc/`: 4 of 200 runs failed, each the CI failure. 100
  runs after `TestStart*` and `TestBudget` failed none. The ending
  goroutine does not need those tests: it can be the previous run's
  released goroutine, or the `testing` package's goroutine for the test
  before.
* **0.01 s** is the masked path: a leak that is seen waits out the
  check's 2 s poll before it is reported.

This also holds for the 140 calls in the `selfupdate`, `selfupdate/cli`
and `selfupdate/service` tests: such a test can pass with a leak. The
check's doc already says that "a goroutine that is ending is still
counted for a moment" (0004-MADR H6). That is the same effect, read only
in the direction that delays a report.

Decided: `CheckNoLeak` records the IDs of the goroutines that run when
it is called, from `runtime.Stack(buf, true)`. It reports any goroutine
that started after that and still runs, with the first line of its
stack. It keeps the 2 s poll, since a new goroutine that is ending is
still listed for a moment, and it keeps the `goroutines:` prefix. An
older goroutine that ends can then hide nothing. It uses only the
standard library, which the `internal-testproc` depguard rule already
allows. No exported API changes, and `internal/testproc` is imported only
by `_test.go` files, so no release is needed.

Rejected:

* re-running the failed job, which turns `main` green and leaves the
  flake and the masked leaks;
* waiting in the test for the goroutine count to settle before the check,
  which fixes this one test, not the check its 140 callers rely on;
* goleak, a module this repository does not require. The rule is that no
  module is required without a record that names it, and the standard
  library does the same here.

### Amendment A7 (2026-10-21): the floor is `go 1.26.10`

Accepted by the owner ("option a, approved to proceed"), as
[0029-MADR](0029-MADR-move-toolchain-to-go-1-27-3-and-floor-to-1-26-10.md).

GO-2026-6809, in `crypto/x509`, was published on 2026-10-21.
`govulncheck` reports it in Go 1.26.9 and in 1.27.2, and in neither
1.26.10 nor 1.27.3. Amendment A1's reason for 1.26.9, that it was
advisory-free, therefore no longer holds.

Decided:

* `go.mod`'s line becomes `go 1.26.10`, and the toolchain line
  `toolchain go1.27.3`;
* the `go126` job builds, vets, tests and runs `govulncheck` with
  1.26.10;
* the test fixtures' `go` lines follow, as the PLAN's S11 moved them.

D8's reason for a 1.26 floor stands: the library needs nothing from
1.27, and Go 1.26 is still supported. The rest of this record is kept
as written.

### Consequences

* Good, because a program that logs its updater, its options or a
  credential no longer prints a token.
* Good, because callers can branch on HTTP status and service-command
  failures with `errors.As`, and every operation's docs say what it
  returns.
* Good, because a standalone program updates with one constructor call,
  and README shows it.
* Good, because Go 1.26 programs can adopt the module, and `go list -m
  -versions` stops offering eight tags that cannot resolve.
* Good, because the evolution policy, the concurrency contracts and the
  trust model are written where adopters and future records read them.
* Good, because a signed release becomes possible for any consumer that
  wants it, and that unblocks the §6 record when the owner schedules it.
* Neutral, because `%v` and `%s` of `Credential`, `GitHubOptions` and
  `*GitHubSource` print differently. `apidiff` accepts the new methods,
  but the output is a visible change, and the migration guide lists it.
* Neutral, because the trust model gets stated, not changed: freshness
  stays unproven, and signatures stay off by default.
* Bad, because D8-A adds a CI job, and pins library code below Go 1.27
  features until the line moves, which is expected when 1.28 ships.
* Bad, because D10-A adds a package, a signer and a workflow input to
  maintain, and their value depends on each consumer's key handling.
* Bad, because each new export grows the surface (D5, D12) that `apicheck`
  then freezes for v1.
* Bad, because one release, `v1.16.0`, carries every change, signing
  included, so its change set, review and migration-guide section are
  large, and a defect in any part holds back the rest.
* Neutral, because `main` stays unruled by the owner's choice, so a
  force-push or deletion of `main` remains possible for anyone with
  write access.

### Confirmation

* `make apicheck` reports compatibility with `v1.15.2` at every commit.
* Each new test is seen failing on a planted copy of today's code
  (`scripts/plant-copy.sh`), and the handoff states what the failure
  looked like:
  * the token probe as a test, for each formatted type;
  * `errors.As` for `*HTTPError` and `*CommandError`;
  * a cancelled `HandOffHopContext`;
  * `-race` on Windows for `lastCoInit`, run on the Windows test host;
  * `NewGitHubUpdater` against `selfupdatetest.NewGitHubServer`;
  * the conformance helper against a source that breaks each rule;
  * the GitHub Enterprise path test;
  * the Go 1.26 CI job, on a planted 1.27 symbol;
  * the `signednote` verifier against a wrong key, a wrong tag, a changed
    manifest and a missing note.
* After `v1.16.0` is published, `go list -m -versions` on a fresh module
  cache lists nothing below `v1.5.0`.
* `make gate`, `scripts/check-docs.sh --links --ids --arch`, markdownlint
  on the changed Markdown, and `actionlint` on a changed workflow all
  pass.
* The scorecard is re-run after the last unit. Every row this record
  chose to change reads Aligned. Any row that does not is named with its
  reason in the PLAN's execution record.

## Pros and Cons of the Options

### D1-A. Redacting formatters, JSON untouched

* Good, because it covers `fmt`, `%v` on the pointer, and `slog`, which
  are where tokens leak in practice.
* Good, because it keeps the `CredentialObserver` save path working.
* Neutral, because `apidiff` accepts the added methods, while the visible
  output changes.
* Bad, because `fmt.Sprintf("%+v", *src)` on a dereferenced source would
  still bypass a pointer method. `go vet`'s copylocks check flags that
  copy.

### D1-B. Plus a redacting `MarshalJSON`

* Good, because a credential marshalled by accident does not leak.
* Bad, because it breaks a program that saves the accepted credential as
  JSON, the use `CredentialObserver` names.

### D1-C. Document only

* Bad, because a documented leak is still a leak, and the list says
  "never log tokens".

### D2-A. Typed errors and per-function docs

* Good, because it is additive: the same text, and `errors.Is` on every
  sentinel as before.
* Bad, because it adds two exported types to keep stable.

### D2-B. Docs only

* Good, because it adds nothing to the surface.
* Bad, because a caller still parses text to tell 401 from 503.

### D2-C. No change

* Bad, because the list's "typed errors for common cases" stays unmet.

### D3-A. `…Context` variants and an internal ctx

* Good, because every wait honours the caller.
* Bad, because there are two names for each operation.

### D3-B. Internal only

* Bad, because `HandOffHop`'s 10 s wait stays outside the caller's
  control.

### D3-C. No change

* Bad, because the cleanup retry keeps its default budget whatever the
  caller's deadline.

### D4-A. Contracts, race fix and leak check

* Good, because adopters learn what is safe from the docs, not from
  reading `runoptions.go`.
* Bad, because the leak check adds a little time to each backend's test
  run.

### D4-B. Docs only

* Bad, because it leaves the `lastCoInit` race.

### D4-C. No change

* Bad, because the concurrency rows of the scorecard stay Partial.

### D5-A. `NewGitHubUpdater`

* Good, because the common case is one call, and every advanced knob
  stays in `Config`.
* Good, because it composes public parts only, so it adds no second code
  path.
* Bad, because it is a second constructor to keep compatible, and the
  default client's transport timeouts become a documented value.

### D5-B. Zero limits mean defaults

* Good, because it needs no new export.
* Bad, because it turns a documented refusal into success, and a zero
  that came from a bug is no longer caught.

### D5-C. Quickstart only

* Bad, because the quickstart is five calls long.

### D6-A. Written evolution policy

* Good, because it costs nothing and binds future records.
* Neutral, because existing open interfaces stay open and frozen.

### D6-B. Seal now

* Bad, because it breaks every external implementer, in v1 (F6).

### D6-C. No written policy

* Bad, because each future record re-derives the policy.

### D7-A. `retract` and `CHANGELOG.md`

* Good, because the probe shows `retract` works on these versions (F7).
* Good, because the changelog is where adopters look, and each entry links
  the guide.
* Bad, because a release step is added, and the entries for `v1.5.0` to
  `v1.15.2` are written once, after the fact.

### D7-B. `retract`, with the guide as the changelog

* Good, because nothing is duplicated.
* Bad, because the guide's sections are long how-tos, not a release index.

### D7-C. No change

* Bad, because the version list keeps eight dead versions.

### D8-A. `go 1.26.8` with `toolchain go1.27.2`

* Good, because both supported Go lines can build the module, as F8
  proves.
* Good, because it stays inside the fleet rule's floor, and the
  repository's own builds keep 1.27.2.
* Bad, because library code waits for 1.27 features until the next move,
  and CI gains a job.

### D8-B. Keep 1.27.1

* Good, because nothing changes.
* Bad, because adopters on 1.26 are shut out for no technical reason.

### D8-C. `go 1.26.0`

* Bad, because it sits below the fleet rule's floor, and admits 1.26
  patches the fleet rule considers too old.

### D9-A. `SECURITY.md`, the trust model and the settings

* Good, because the settings close the realistic threat F9 shows is open.
* Bad, because the settings are the owner's to change, outside the tree,
  so the PLAN can only verify them.

### D9-B. Documents only

* Bad, because the reporting channel `SECURITY.md` names stays off.

### D9-C. No change

* Bad, because adopters have no single statement of the trust model.

### D10-A. `signednote`, opt-in end to end

* Good, because it needs no new module, and its design is complete
  (0004-REPORT).
* Good, because a consumer can turn it on, and other hosts become
  possible.
* Bad, because it reopens a decision on grounds that 0004-REPORT judged
  thin under an auto-approved key.
* Bad, because there is more to maintain, including a workflow input.

### D10-B. Verifier only

* Bad, because no release can be signed until a second record adds the
  signer.

### D10-C. Keep deferred

* Good, because it matches 0004's evidence for today's GitHub-only
  releases.
* Bad, because the list's first self-update bullet stays Partial.

### D10-D. Mandatory

* Bad, because it breaks every unsigned release in v1.

### D11-A. Host-neutral docs and a conformance helper

* Good, because a third-party source can prove itself against the
  contract.
* Neutral, because GitLab still waits on the §6 record.

### D11-B. The §6 record now

* Good, because it gives real multi-host support.
* Bad, because the owner deferred it, and it depends on D10.

### D11-C. No change

* Bad, because the seam stays GitHub-worded.

### D12-A. Clock, prober runner and slog reporter

* Good, because each is a nil-default field or a new function, and each
  is something the list names.
* Bad, because there are three more exports to freeze.

### D12-B. slog only

* Bad, because a program still cannot test its cache freshness without
  real time.

### D12-C. No change

* Bad, because the testability row stays Partial.

### D13-A. Recipes now, `Variant` later

* Good, because there is no schema change.
* Neutral, because the arm and musl rows stay Partial until a consumer
  needs them.

### D13-B. `Variant` now

* Bad, because the release spec, the workflows and the installers would
  all have to move together, for no current consumer.

### D14-A. Close the gaps

* Good, because the remaining Partial documentation rows become Aligned.

### D14-B. No change

* Bad, because `selfupdate/service` keeps no Example, and `ExitCode`'s
  doc stays wrong.

## More Information

### Relation to earlier records

* **0004-MADR-evolve-selfupdate-api-and-tui-support.md.** D10 reopens its
  "Release signing: deferred". D11 leaves its §6 record deferred. If D10
  is chosen, 0004-MADR gains a dated amendment that points here. It is not
  rewritten.
* **0004-REPORT-release-signing-research.md.** D10 builds its design and
  inherits its open questions 2–5.
* **0009-MADR-rename-to-go-selfupdate-lib.md.** D7's `retract` addresses
  the version-list side of its §7 risk. The old path's `@latest` stays
  beyond this repository's reach.
* **0001-MADR-scaffold-shared-go-library.md** and
  **0018-MADR-move-toolchain-to-go-1-27-2.md.** D8 lowers the `go` line
  they set and keeps 0018's `toolchain` line. If D8 is chosen, 0018 gains
  an amendment.
* **magic-cli-remote's
  `docs/decisions/0169-MADR-standardize-toolchains-on-current-supported-advisory-free-releases.md`**
  (another repository). D8 stays within its D2 floor and needs no change
  there.

### Out of scope

* Changing any constructor's result type, sealing an existing interface,
  or removing an export. These are v2 changes, and no v2 is proposed.
* Non-GitHub sources and mutable releases (0004 §6), the `sigstore-go`
  variant of `ghattest` (0017), a D-Bus systemd backend (0011), and the
  other open items 0004-MADR lists.
* Converting the internal test seams so `selfupdate`'s tests can run in
  parallel. It changes no behaviour a program sees, and it would be its
  own record.
* Signed git tags. The Go checksum database already pins every module
  version's content for consumers, so a tag signature adds little for
  them. A record can revisit this.
* The authentication-SDK bullets of the owner's list beyond token
  handling. That SDK is go-llmprovider-sdk, another repository.

### The audits' footprint

Nothing in the repository was written. The scratch clones, probes and
outputs live outside the tree. The go command's module cache gained the
versions the proxy probes fetched. One copy of a source file was made by
this record's author in the system temporary directory, outside the
scratchpad, and deleted at once.

### Sources

* Effective Go, <https://go.dev/doc/effective_go>
* Go Code Review Comments ("Contexts", "Interfaces"),
  <https://go.dev/wiki/CodeReviewComments>
* Keeping Your Modules Compatible (Go blog, 2020-07-07),
  <https://go.dev/blog/module-compatibility>
* Go Modules: v2 and Beyond, <https://go.dev/blog/v2-go-modules>
* Go release history and policy, <https://go.dev/doc/devel/release>
* Go 1.27 release notes, <https://go.dev/doc/go1.27>
* `retract` directive, <https://go.dev/ref/mod#go-mod-file-retract>
* TUF, "Security" (rollback, freeze and fast-forward attacks),
  <https://theupdateframework.io/security/>
* Keep a Changelog 1.1.0, <https://keepachangelog.com/en/1.1.0/>
* `log/slog` `LogValuer`, <https://pkg.go.dev/log/slog#LogValuer>
* `fmt.Formatter` and `fmt.GoStringer`, <https://pkg.go.dev/fmt>
* `apidiff`, <https://pkg.go.dev/golang.org/x/exp/apidiff>
* minio/selfupdate's minisign verifier and creativeprojects/go-selfupdate's
  ECDSA validator, as compared in
  [0004-REPORT-release-signing-research.md](../reports/0004-REPORT-release-signing-research.md)
* GitHub private vulnerability reporting,
  <https://docs.github.com/en/code-security/security-advisories/working-with-repository-security-advisories/configuring-private-vulnerability-reporting-for-a-repository>
* GitHub rulesets,
  <https://docs.github.com/en/repositories/configuring-branches-and-merges-in-your-repository/managing-rulesets/about-rulesets>
