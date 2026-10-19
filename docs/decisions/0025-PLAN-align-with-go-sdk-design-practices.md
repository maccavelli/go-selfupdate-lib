---
status: in-progress
date: 2026-10-19
associated-madr: "0025-MADR-align-with-go-sdk-design-practices.md"
---
# Implement the Go SDK practices alignment as `v1.16.0`: redaction, typed errors, contexts, contracts, a quickstart, seams, a source conformance check, opt-in release signing, the Go 1.26.9 floor, `retract`, a changelog and a trust model

Associated MADR: [0025-MADR-align-with-go-sdk-design-practices.md](0025-MADR-align-with-go-sdk-design-practices.md)
(D1-A to D14-A; D9-A without its rule on `main`; one release; Amendment
A1).

## Goal

* Every decision of the MADR is built, tested and documented, and
  released together as `v1.16.0`. A test or assertion that is new is seen
  failing first.
* `make apicheck` reports compatibility with `v1.15.2` at every phase.
* The MADR's scorecard, re-run at closing, reads Aligned on every row the
  MADR chose to change. A row that does not is recorded with its reason.

## Scope

### In scope

| Phase | What | Decisions | Commit |
| :--- | :--- | :--- | :--- |
| S0 | this pair, the index, three record amendments | records | records only |
| S1 | internal hardening: the cleanup retry's context, `lastCoInit`, the shared leak check | D3 (internal), D4 | code and tests |
| S2 | redacting formatters | D1 | code and tests |
| S3 | `HTTPError`, `service.CommandError`, the "Errors" paragraphs | D2 | code, tests, Go doc |
| S4 | `HandOffHopContext`, `LoadHandOffEnvContext` | D3 | code and tests |
| S5 | concurrency contracts and the doc corrections | D4, D14 | Go doc |
| S6 | `NewGitHubUpdater`, its Example, the README quickstart, `doc.go`'s code block | D5, D14 | code, tests, docs |
| S7 | `Config.Now`, `CheckerConfig.Now`, `NewVersionProberWith`, `NewSlogReporter` | D12 | code and tests |
| S8 | host-neutral source docs, `VerifyReleaseSource`, the GitHub Enterprise test | D11 | code, tests, Go doc |
| S9 | `selfupdate/verify/signednote`, `internal/cmd/selfupdate-note`, the reserved name | D10 | code, tests, lint config |
| S10 | the publish workflow's `sign` job, its tests and docs | D10 | workflow, scripts, docs |
| S11 | the Go floor, `go 1.26.9` | D8 | `go.mod`, tests, CI, docs |
| S12 | `retract`, `CHANGELOG.md`, the evolution policy, the trust model, `SECURITY.md`, the variant recipes, the remaining docs | D6, D7, D9, D13, D14 | `go.mod`, docs, one Example |
| S13 | the signing rehearsal on a throwaway repository | D10 | none (execution record only) |
| S14 | `v1.16.0`, and the owner's repository settings | all; D9 | per 0017's release procedure |
| S15 | closing | — | records only |

### Out of scope

* Everything the MADR lists under "Out of scope": result-type changes,
  sealing interfaces, removals, non-GitHub sources and mutable releases
  (0004 §6), `ghattest`'s `sigstore-go` variant, a D-Bus systemd backend,
  converting the internal test seams, signed git tags, and the
  authentication-SDK bullets.
* Any rule, protection or ruleset on `main` (the owner, D9).
* A formatter for `ghattest.Options.Env` or `service.Command.Env`. Either
  can hold `GH_TOKEN=…`, and neither is a type D1 names. S2 records them
  as seen, and a later record may add them.
* Typed errors from `scm` and `taskscheduler`, which fail through
  Win32 or HRESULT errors rather than commands
  (`scm/service.go:139-148`, `taskscheduler/task.go:205-217`), and from
  `codesign`, `ghattest` and `launchd`'s `plutil` calls. D2 names "a failed
  service command", meaning `systemctl`, `systemd-run`, `launchctl` and the
  exec reconciler's child.
* A clock on `GitHubSource`'s own rate-limit arithmetic (`github.go:136`,
  `:845`), which D12 does not name.

## Rules

* **`main`, no branch.** The owner commits and pushes on `main`. An agent
  commits only on an explicit ask in that turn, with
  `git commit --no-edit`, after the phase's checks pass.
  *(Approval, 2026-10-19: the owner's "accept/approved to proceed", with
  the owner's standing rule "Commit at the end of each approved plan phase
  after pre-add checks pass", authorizes the agent to commit at the end of
  each phase. Pushes, tags and every other outward-facing step still need
  an ask in that turn.)*
* **Each phase stops at a deviation** (AGENTS.md, Plan deviations). The
  deviation is recorded in this plan, and in the MADR when it changes a
  decision or a fact the MADR asserts, before the phase continues.
* **Checks before each commit:**
  * `make pre-add-check FILES="…"` on every changed Go file;
  * `scripts/check-docs.sh --links` and `--ids`, and `npx --yes
    markdownlint-cli2@0.23.2`, on every changed Markdown file;
  * `scripts/check-docs.sh --arch docs/architecture.md` when a `.go` file
    is added;
  * `actionlint` on a changed workflow;
  * `make apicheck` (compatible with `v1.15.2`).

  `make gate` must end `overall=0` once before each push (0024-PLAN's
  deviation D2).
* **Seen failing first.** Every new test or assertion fails on a
  `scripts/plant-copy.sh FILE OLD NEW` copy with one planted break, never
  on a dirtied tree. The execution record quotes the failing line. A plant
  needs OLD to occur exactly once in FILE; when it does not, the phase
  names a larger OLD.
* **The arch table.** Every phase that adds a `.go` file updates
  `docs/architecture.md`'s "Go code" table in the same commit. The
  expected counts are given in each phase, starting from today's
  `selfupdate` 45/83, `service` 16/14, `selfupdatetest` 2/2, and
  `internal/testproc` 1/1.
* **The Windows test host.** Binaries are cross-compiled here, copied to a
  new directory under its temporary directory, run from its ssh shell, and
  cleaned up.
* **Snippets.** Every Go snippet added to a Markdown file is built in a
  scratch module outside the repository for linux, darwin and windows with
  `CGO_ENABLED=0`.
* **Identifiers.** Nothing committed names a host, an account or a real
  absolute path. Command examples use `<owner>` and `<repo>`.
* **Outward-facing steps** (a push, a tag, creating or deleting a
  repository, a repository setting, a secret) happen only on the owner's
  ask in that turn. The agent prepares the commands and verifies the
  result with read-only calls.

## Implementation Steps

### Phase S0: records

1. `HEAD` is `5c0ab0d` or a records-only descendant, on `main`, and the
   tree is clean apart from this pair.
2. `docs/README.md` has the MADR's row (`accepted`) and this PLAN's row
   (`proposed`) after 0024's.
3. **0004-MADR.** `## Amendments` gains `### P4 (2026-10-19): release
   signing is built, opt-in, by 0025`. It says:
   * that 0025-MADR D10-A reopens "Release signing: deferred, with the
     hook built";
   * what is built: `selfupdate/verify/signednote`,
     `internal/cmd/selfupdate-note`, and the publish workflow's `sign` job;
   * that §6 (mutable releases, other hosts) stays deferred.

   It links 0025-MADR. The open-work table's `signednote` row is struck
   through and annotated "built by 0025 (`v1.16.0`)". Nothing else in
   0004-MADR changes.
4. **0004-REPORT.** A `## Since (2026-10-19)` section is added after `##
   Since (2026-10-12)`. It holds two bullets:
   * The secret claim is corrected. The sentence "Environment secrets
     cannot be passed through `secrets:` to a reusable workflow" is
     contradicted by GitHub's "Reuse workflows" page. The page is quoted:
     the caller "must still pass the secret … by name even if it only
     exists in the environment", and an unpassed secret "resolves to an
     empty string in the reusable workflow. The workflow run doesn't show
     an error". The report is otherwise kept as written.
   * The design is built by 0025, with the refinements of 0025-MADR
     Amendment A1.
5. **0018-MADR.** A `## Amendments` section is added before `## More
   Information`, holding `### A1 (2026-10-19): the floor moves to
   1.26.9`. It says that 0025-MADR D8-A (Amendment A1) sets `go 1.26.9`
   and keeps `toolchain go1.27.2`, and gives the reason: the code needs
   nothing from 1.27, and 1.26.9 is advisory-free.
6. Checks (links, ids, markdownlint on `docs/README.md`). Commit on the
   owner's ask: records only.
7. On approval, this plan becomes `in-progress` with `### Approval` in its
   execution record.

### Phase S1: internal hardening (D3 internal, D4)

1. **The cleanup retry takes the caller's context** (F3,
   `cleanup_windows.go:245`).
   * `writeCleanupEntries(target Target, entries []cleanupEntry) error`
     becomes `writeCleanupEntries(ctx context.Context, target Target,
     entries []cleanupEntry) error`. It passes `ctx` to `moveFileReplace`
     in place of `context.Background()`.
   * The chain gains a leading `ctx context.Context`, with the call sites
     updated:
     * `writeCleanupReceipt` (`cleanup_windows.go:172-179`);
     * `keepAsPending` and `commitReplacement`
       (`replace_windows.go:147-170`; Unix stubs `replace_unix.go:101`,
       `:105`);
     * `processCleanupReceipt` (`cleanup_windows.go:84`; stub
       `cleanup_other.go:9`).
   * Callers: `commitLocked` (`session.go:213`, `:215`, `:228`) passes
     `withRetryBudget(ctx, s.lockTimeout)`, as it already does for
     `replacePath` (`:223`). `beginSession` (`session.go:572`, `:605`)
     passes `withRetryBudget(ctx, timeout)`.
   * Tests: update the call sites in `cleanup_windows_test.go`,
     `cleanup_busy_windows_test.go` and `lifecycle_windows_test.go`. Add
     `TestCleanupReceiptHonoursContext` to `cleanup_busy_windows_test.go`.
     With the receipt's destination held open (the file's existing busy
     helper) and an already-cancelled context, `writeCleanupEntries`
     returns within 1 s, with `errors.Is(err, context.Canceled)`.
   * **Seen failing:** a plant restoring `context.Background()` at `:245`
     makes the new test fail (it waits out `DefaultLockTimeout`, 5 s). Run
     on the Windows test host and in Windows CI.
2. **`lastCoInit`** (F4, `taskscheduler/com_windows.go:48-49`, `:152`).
   * It becomes `var lastCoInit atomic.Uint32`. `withCOM` stores
     `uint32(hr)`, and `com_windows_test.go:28`, `:30` read
     `lastCoInit.Load()`.
   * New test `TestWithCOMConcurrent` in `com_windows_test.go`: 8
     goroutines × 50 calls of `withCOM(func() error { return nil })`, with
     every call returning nil.
   * **Seen failing:** on the Windows test host, run `go test -race -run
     '^TestWithCOMConcurrent$'` on a plant restoring the plain `hresult`
     variable. It reports `WARNING: DATA RACE`, and the fix passes.
     `-race` on Windows needs cgo and a C compiler. When the host has
     neither (`go env CC`, `gcc --version`), the phase stops as a
     deviation; it does not drop the check.
3. **One leak check, everywhere.**
   * `internal/testproc` gains `func CheckNoLeak(t testing.TB) (checkNow
     func())`, which is `selfupdate/leak_test.go:9-35` verbatim, in
     `testproc.go`. That adds no file, and the `internal-testproc`
     depguard rule (`$gostd`) already allows it.
   * `selfupdate/leak_test.go` becomes `var checkNoLeak =
     testproc.CheckNoLeak`. `export_test.go:8-9` is unchanged.
     `selfupdate/cli/run_test.go:384-402` is deleted, and its caller
     (`:407`) uses `testproc.CheckNoLeak`.
   * `testproc.CheckNoLeak(t)` is the first statement of every top-level
     `Test…` function in:
     * `selfupdate/service`: `poll_test.go`, `handoff_test.go`,
       `execreconciler_test.go`, `runner_test.go`;
     * the four backends: `systemd_test.go`, `launchd_test.go`,
       `scm_test.go`, `taskscheduler/lifecycle_test.go`.

     None of those packages calls `t.Parallel()` (`git grep` found
     none). A test that the check fails because a helper process
     legitimately outlives it is listed in the record with the reason,
     and gets no check.
   * **Seen failing:**
     * a plant of `go func() { time.Sleep(time.Hour) }()` at the start of
       `PollHealthy` (`service/poll.go:97`) makes `poll_test.go` report
       `goroutines: N now, M when the test began`;
     * a plant in `testproc.CheckNoLeak` that never compares fails its
       own new test, `TestCheckNoLeakSeesLeak`. That test leaks a blocked
       goroutine in a subtest it runs under a fake `testing.TB` that
       records `Errorf`. The fake TB embeds `testing.TB` to satisfy the
       private method.
4. Arch: `internal/testproc` 1/1, unchanged. Checks; commit.

### Phase S2: redacting formatters (D1)

1. **`Credential`** (`credentials.go:19-26`, value receivers):
   * `func (c Credential) String() string` returns
     `Credential{Header:<q>, Source:<q>, Value:<redacted>}`. `<q>` is
     `strconv.Quote` of the field. `Value:` reads `<redacted>` when the
     value is non-empty and `<empty>` otherwise.
   * `GoString` returns the same text.
   * `Format(f fmt.State, verb rune)` writes `c.String()` for every verb.
     Its doc says that `%#v`, `%x` and `%q` included.
   * `LogValue() slog.Value` returns a group of `header`, `source` and
     `value` (`<redacted>` or `<empty>`).
   * The type's doc gains: "Formatting a Credential with fmt or log/slog
     never prints Value; encoding/json does, so a CredentialObserver can
     save it."
2. **`GitHubOptions`** (`types.go:203-228`, value receivers), with the
   same four methods:
   * `GitHubOptions{Repository:<owner>/<name> APIBaseURL:<url|default>
     UserAgent:<q> Token:<redacted|empty> Credentials:<set|nil>
     Observer:<set|nil> Client:<set|nil> Limits:{…}}`;
   * `APIBaseURL` through `redactURL` (`github.go:658`);
   * `LogValue` as a group of the same keys.
3. **`*GitHubSource`** (`github.go:34-48`, pointer receivers, because
   `credentialState` holds a `sync.Mutex` by value):
   * `String`, `Format` and `LogValue` print `GitHubSource{Repository:<o>/<n>
     API:<origin>}`, with the origin from `apiBase`;
   * a nil receiver prints `GitHubSource(nil)`.
4. **Tests**, in the new `selfupdate/redact_test.go`:
   * `TestSecretsNeverFormat`, a table of `Credential`, `GitHubOptions`,
     `*GitHubSource` and `[]any{…}` holding each. For each, the verbs `%v`,
     `%+v`, `%#v`, `%s`, `%q`, `%x` and `%X`, plus `slog.NewTextHandler`
     and `slog.NewJSONHandler` output, must not contain the fixture secret
     `fixture-secret-0123`, nor its hex or base64, and must contain
     `<redacted>`.
   * `TestCredentialJSONKeepsValue` pins the decision to leave JSON alone:
     `json.Marshal(Credential{Value: []byte("v")})` holds
     `"Value":"dg=="`.
   * **Seen failing:** three plants, each removing one type's `Format`
     method, fail `TestSecretsNeverFormat` with the secret quoted.
5. Arch: `selfupdate` 45/84. Checks; commit.
6. Seen and left (Out of scope): `ghattest.Options.Env` and
   `service.Command.Env`, recorded.

### Phase S3: typed errors and the "Errors" paragraphs (D2)

1. **`HTTPError`**, in `selfupdate/errors.go` after `RateLimitError`:

   ```go
   // HTTPError is a non-2xx response that no sentinel describes, from a
   // ReleaseSource that speaks HTTP. A 404 for a release also wraps
   // ErrNoRelease, and a rate limit is a *RateLimitError instead.
   type HTTPError struct {
       // Source names the source, such as "github".
       Source string
       // StatusCode is the HTTP status.
       StatusCode int
       // Diagnostic is the sanitized, bounded response body or reason.
       Diagnostic string
   }
   ```

   `Error()` returns `"selfupdate: " + src + " http %d: %s"`, where `src`
   is `Source`, or `"http"` when `Source` is empty. A nil receiver gives
   `"selfupdate: http error"`. There is no `Unwrap`.
2. `mapStatus` (`github.go:817-824`) returns `&HTTPError{Source:
   "github", StatusCode: resp.StatusCode, Diagnostic: diag}`. `refresh`'s
   `Cause` (`github.go:790`) becomes `&HTTPError{Source: "github",
   StatusCode: http.StatusUnauthorized, Diagnostic: "credential from " +
   sanitizeText(refused.Source) + " refused"}`. Every message is
   byte-identical to today's.
3. **`service.CommandError`**, in `selfupdate/service/errors.go`:

   ```go
   // CommandError is a service manager command that exited non-zero. A
   // classified failure wraps it with ErrPermission or ErrNotInstalled.
   type CommandError struct {
       Backend  string // "systemd", "launchd" or "service"
       Step     string // what was run, such as "stop" or "bootout"
       Name     string // the unit, label or program
       ExitCode int
       Message  string // standard error, or standard output when that is empty, trimmed and capped
   }
   ```

   `Error()` returns `fmt.Sprintf("selfupdate: %s: %s %s: exit %d: %s",
   …)`.
   * `systemd.commandError` (`unit.go:230-246`),
     `launchd.launchctlError` (`job.go:317-333`) and
     `service.childError` (`execreconciler.go:212-221`) build a
     `*CommandError` where they build the string today. Their wrapping
     (`fmt.Errorf("%w: %w", service.ErrPermission, err)`, the
     disabled-job suffix) is unchanged, and `childError`'s 4 KiB cap stays
     in `Message`.
4. **Tests:**
   * new `selfupdate/httperror_test.go`:
     * `TestHTTPErrorAs`: a 500, a 401 after refresh and an asset 404
       from `newGitHubEnv` are each `errors.As` an `*HTTPError` with the
       status;
     * a release 404 is also `errors.Is(err, ErrNoRelease)`;
     * every message equals today's, and the existing text assertions
       (`github_test.go:533`, `github_hardening_test.go:146`,
       `e2e_running_test.go:446`, `stream_run_test.go:401`, `:415`) pass
       unchanged;
   * new `selfupdate/service/commanderror_test.go`: `TestCommandErrorAs`
     drives `commandError`, `launchctlError` and `childError` through the
     backends' fake runners. Each case checks `errors.As`,
     `errors.Is(err, ErrPermission)` or `ErrNotInstalled` where
     classified, and the message unchanged, including
     `launchd_test.go:248`. Cases for `systemd` and `launchd` live in
     their packages' tests if `service`'s test cannot reach them; the
     record names the files.
   * **Seen failing:** a plant of `mapStatus` back to `fmt.Errorf`, and
     one of `commandError` back to its string, each fail its test.
5. **"Errors" paragraphs.** Each doc comment gains a paragraph beginning
   `Errors:` that names what the function returns and how to match it.
   Each entry is traced to a code path. An entry that no test reaches gets
   a test in this phase, or is dropped:
   * `Updater.Run` and `RunWith`:
     * `ErrNotConstructed`, `ErrConcurrentUpdate`, `ErrUpdateAvailable`
       (CheckOnly), `ErrConfirmationRequired`, `ErrForceRequired`,
       `ErrLatestOlder`, `ErrNoRelease`, `ErrMutableRelease`,
       `ErrUnsupportedPlatform`, `ErrIntegrity`, `ErrManagedInstall`,
       `ErrBatchFile`;
     * `ErrRateLimited` through `*RateLimitError`, and `*HTTPError`;
     * the context's error.
   * `Start`: the same, delivered as `Finished.Err`.
   * `Checker.Check`: the discovery subset (`ErrLatestOlder`,
     `ErrNoRelease`, `ErrMutableRelease`, `ErrUnsupportedPlatform`,
     `ErrIntegrity`, `ErrRateLimited`, `*HTTPError`).
   * `CheckCached` keeps its list (`checkcache.go:132-152`) and adds
     `*HTTPError`.
   * Each backend's `Stop`, `Start` and `WaitHealthy` (systemd, launchd,
     scm, taskscheduler) get the lists the facts trace:
     * `ErrPermission`, `ErrNotInstalled`, `ErrInsideService`,
       `ErrTimeout`, `ErrUnhealthy`, `ErrUnsupported`;
     * `ErrTaskDisabled` for taskscheduler;
     * `*service.CommandError` for systemd and launchd.
6. Arch: `selfupdate` 45/85; `service` 16/15. Checks; commit.

### Phase S4: contexts on the hand-off waits (D3)

1. `selfupdate/service/handoffhop.go`:
   * `func HandOffHopContext(ctx context.Context) error` holds today's
     body. `HandOffHop()` becomes `return
     HandOffHopContext(context.Background())`.
   * In the real run, a context that ends during the wait stops the wait,
     unsets `EnvHandOffHopParent`, and returns `ctx.Err()`.
   * The doc says both, and keeps "waits up to 10 s".
2. `waitHopExit(pid int, d time.Duration)` becomes `waitHopExit(ctx
   context.Context, pid int, d time.Duration) error`:
   * Unix (`hopwait_unix.go:15-19`): the 5 ms sleep becomes a `select` on
     `ctx.Done()` and a 5 ms timer.
   * Windows (`hopwait_windows.go:15-25`): `WaitForSingleObject` in
     slices of at most 50 ms, checking `ctx.Done()` between slices.
   * Other: returns nil.
   * `service/main_test.go:88` passes `context.Background()`.
3. `handoffenv.go`: `func LoadHandOffEnvContext(ctx context.Context)
   error` holds today's body and calls `HandOffHopContext(ctx)`.
   `LoadHandOffEnv()` calls it with `context.Background()`. `ReportFunc`
   is unchanged.
4. **Tests** (`handoffhop_test.go`): `TestHandOffHopContextCancelled`
   uses the existing `fakeParent`/"hopchild" machinery. With the parent
   still alive and a context cancelled after 50 ms, `HandOffHopContext`
   returns `context.Canceled` in under 1 s and has unset the variable.
   `TestWaitHopExit` (`:79`) is updated to the new signature.
   * **Seen failing:** a plant of the Unix loop back to `time.Sleep` makes
     the new test take the full 10 s and fail its 1 s bound. It runs here
     and on the Windows test host, where the Windows plant restores the
     single `WaitForSingleObject`.
5. `service/doc.go:23` names both forms. Checks; commit.

### Phase S5: concurrency contracts and doc corrections (D4, D14)

1. **Concurrency sentences** (doc comments only):
   * `Updater` (`types.go:854-856`): "An Updater is safe for concurrent
     use. Run, RunWith and Start may be called from any goroutine; a run
     that overlaps another on the same Updater returns
     ErrConcurrentUpdate." `Run` (`updater.go:119`) and `RunWith`
     (`runoptions.go:130`) say the same in one sentence.
   * `GitHubSource` (`github.go:34`): "safe for concurrent use; it
     resolves a credential at most once at a time, with no lock held
     during network I/O."
   * `StandaloneInstaller` (`standalone.go:13`) and `ManagedInstaller`
     (`managed.go:12`): "safe for concurrent use; concurrent updates of
     one target are serialized by the target's lock file, and the loser
     returns ErrConcurrentUpdate."
   * `InstallSession` (`types.go:517`): "The sessions this package
     returns serialize their methods; a session of your own need not."
   * `systemd.Unit`, `launchd.Job`, `scm.Service`, `taskscheduler.Task`:
     "safe for concurrent use; the Updater drives one product's Stop,
     Start and WaitHealthy in order, and a program that calls them itself
     must do the same."
2. **"The library never calls os.Exit"** becomes "Package selfupdate
   never calls os.Exit" at `errors.go:91`, `updater.go:119` and
   `types.go:125`.
3. **`GitHubOptions.Client`** (`types.go:206-210`) gains: "Client.Timeout
   bounds each request including the whole asset download; prefer a
   context deadline, or a Transport with ResponseHeaderTimeout, so a slow
   large download is not cut off. NewGitHubUpdater's default client does
   that."
4. `go doc` for each changed type and package shows the text; vet for
   three targets. Checks; commit.

### Phase S6: `NewGitHubUpdater` (D5, D14)

1. New `selfupdate/githubupdater.go`:

   ```go
   // GitHubUpdaterOptions configure NewGitHubUpdater. Repository, Product
   // and Version are required; every other field has a default.
   type GitHubUpdaterOptions struct {
       Repository Repository   // required
       Product    string       // required: the asset prefix, as Request.Product
       Version    string       // required: the running version, for the User-Agent
       Platforms  []Platform   // nil: the running platform
       APIBaseURL *url.URL     // nil: https://api.github.com
       Client     *http.Client // nil: a client with transport timeouts and no Client.Timeout
       Stdin      *os.File     // nil: os.Stdin, for the terminal confirmer
       Stderr     io.Writer    // nil: os.Stderr, for the text reporter and the prompt
   }

   // NewGitHubUpdater returns an Updater for a standalone program released
   // on GitHub with exact asset names, composed from the package's own
   // parts with their safe defaults.
   func NewGitHubUpdater(o GitHubUpdaterOptions) (*Updater, error)
   ```

   The composition, in order (each error returned with the prefix
   `selfupdate: github updater: `):
   1. `validateProduct(o.Product)` (`version.go:158`), and an empty
      `o.Version` refused;
   2. `NewGitHubSource(GitHubOptions{Repository, Client, APIBaseURL,
      UserAgent: UserAgent(o.Product, o.Version), Limits:
      DefaultLimits()})`;
   3. `NewExactAssetSelector(platforms)`, with platforms `[]Platform{{OS:
      runtime.GOOS, Arch: runtime.GOARCH}}` when nil;
   4. `NewStandaloneInstaller(InstallOptions{})`;
   5. `New(Config{Source, Versions: NewStrictVersionPolicy(), Assets,
      Installer, Reporter: NewTextReporter(stderr), Confirmer:
      NewTerminalConfirmer(stdin, stderr), Limits: DefaultLimits()})`.

   **The default client:** clone `http.DefaultTransport` when it is an
   `*http.Transport`. Otherwise build `&http.Transport{Proxy:
   http.ProxyFromEnvironment, DialContext: (&net.Dialer{Timeout: 30 *
   time.Second, KeepAlive: 30 * time.Second}).DialContext,
   ForceAttemptHTTP2: true, MaxIdleConns: 100, IdleConnTimeout: 90 *
   time.Second, TLSHandshakeTimeout: 10 * time.Second,
   ExpectContinueTimeout: time.Second}` (Go 1.27.2's
   `net/http/transport.go:47-58`). Then set `ResponseHeaderTimeout: 30 *
   time.Second`, and return `&http.Client{Transport: t}` with no
   `Timeout`. The doc states these values.
2. The two sentences are reworded:
   * `types.go:201-202`: "GitHubOptions construct a GitHubSource; every
     field is explicit. NewGitHubUpdater composes a source with safe
     defaults."
   * `types.go:806-807`: "Config is the explicit Updater composition. New
     fills no defaults; NewGitHubUpdater composes the safe ones for a
     standalone program."
3. **Tests**, new `selfupdate/githubupdater_test.go` (package
   `selfupdate_test`):
   * `TestNewGitHubUpdaterRefuses`: an empty owner, an empty name, an
     empty or invalid product (`"a b"`), an empty version, and an invalid
     platform, each an error.
   * `TestNewGitHubUpdaterChecks`: against `selfupdatetest.NewGitHubServer`,
     with its `APIBase` and `Client`, a `Run` with `CheckOnly` for the
     running platform finds `v1.1.0` over `v1.0.0` (`ErrUpdateAvailable`).
     The request's `User-Agent` equals `UserAgent(product, version)`.
   * `TestDefaultClientTimeouts` (internal test, in
     `githubupdater_internal_test.go`; arch counts it): the default client
     has no `Timeout`, and `ResponseHeaderTimeout` is 30 s.
   * `ExampleNewGitHubUpdater` in `example_test.go`, with `// Output:
     updater ready`.
   * **Seen failing:** a plant that skips the empty-version refusal fails
     `TestNewGitHubUpdaterRefuses`; one that sets `Client.Timeout` fails
     `TestDefaultClientTimeouts`.
4. **README** `## Self-update`, after the bullet list (`:63-72`): a
   "Quickstart" paragraph and a Go block of about 15 lines. It calls
   `NewGitHubUpdater` and `Run`, then `os.Exit(selfupdate.ExitCode(res,
   err))`. The block is built as Rules say.
5. **`selfupdate/doc.go`**, `# Updating` (`:10-18`), gains a tab-indented
   code block with the same happy path, as `cli/doc.go:28-29` does.
6. Arch: `selfupdate` 46/87 (`githubupdater.go`; `githubupdater_test.go`,
   `githubupdater_internal_test.go`). `docs/architecture.md`'s "by file"
   list names `githubupdater.go`. Checks; commit.

### Phase S7: clock, prober runner, slog reporter (D12)

1. **Clock.**
   * `Config.Now` and `CheckerConfig.Now`, of type `func() time.Time`.
     The doc: "Now is the clock for check-record ages and the progress
     throttle. Nil means the real time."
   * `Updater` and `Checker` gain an unexported `now func() time.Time`,
     copied by `New`, `NewChecker` and `Updater.Checker()`
     (`checker.go:105-108`).
   * The reads at `checkcache.go:168` and `updater.go:581`, `:598` call a
     method `clock()` that returns `now()` when set, else `timeNow()`.
     The read happens at the call, so the existing `setSeam(&timeNow, …)`
     tests keep working.
2. **Prober.**

   ```go
   // VersionProbeOptions configure NewVersionProberWith.
   type VersionProbeOptions struct {
       Args    []string                 // required
       Want    func(tag string) string  // nil: the tag itself
       Timeout time.Duration            // required, > 0
       // Run starts path with args, writes its standard output to
       // stdout, and returns when it exits; a non-zero exit is an error.
       // Nil runs it with the package's own windowless runner.
       Run func(ctx context.Context, path string, args []string, stdout io.Writer) error
   }
   func NewVersionProberWith(o VersionProbeOptions) (Prober, error)
   ```

   `NewVersionProber(args, want, timeout)` (`probe.go:86`) returns
   `NewVersionProberWith(VersionProbeOptions{Args: args, Want: want,
   Timeout: timeout})`. Only `probe.go:107-113` is behind `Run`. The
   64 KiB cap, the timeout and the context classification (`:114-129`)
   stay around it.
3. **`NewSlogReporter`**, new `selfupdate/slogreporter.go`:
   * `func NewSlogReporter(l *slog.Logger) Reporter`. A nil logger makes
     `Report` return `selfupdate: slog reporter logger is nil`, as
     `textReporter` does for a nil writer (`reporter.go:21`, `:32`).
   * Each event is logged with `l.Log(ctx, level, kind.String(), attrs…)`.
     The attrs are `product`, `current`, `target`, `asset`, `bytes`,
     `total` and `detail`, each omitted when empty or zero, and each
     string through `sanitizeText`.
   * Levels: `EventFailed` is Error; `EventRolledBack` and `EventWarning`
     are Warn; `EventProgress` is Debug; every other kind is Info.
4. **Tests:**
   * `checkcache_test.go` gains `TestCheckCachedUsesConfigNow`: a fixed
     `Now` decides fresh versus stale with no seam.
   * `probe_test.go` gains `TestVersionProberWithRun`:
     * a fake `Run` that writes `v1.2.3` passes;
     * one that writes `v1.2.4` fails the want;
     * one that blocks hits the timeout, with the existing timeout text.
   * New `slogreporter_test.go`: the kinds, levels and attrs over a
     `slog.NewJSONHandler` buffer; a nil logger; sanitizing of a control
     character.
   * **Seen failing:** a plant of `clock()` that ignores `now`; a plant of
     the prober that ignores `Run`; a plant mapping `EventFailed` to
     Info. Each fails its test.
5. Arch: `selfupdate` 47/88. The "by file" list names `slogreporter.go`.
   Checks; commit.

### Phase S8: the source seam (D11)

1. **Docs** (comments only, `types.go:230-318`):
   * `Release` and `Asset` say what each field means to the Updater:
     * `ID` is the source's identifier, and the Updater requires
       `Asset.ID > 0` (`github.go:489-497`);
     * `State` is `AssetStateUploaded` for a complete asset;
     * `Digest` is `sha256:<64 hex>` or empty;
     * `URL` is the release's page, copied to `Result.ReleaseURL`;
     * `Immutable` must be true, or the run fails with `ErrMutableRelease`.
   * Their first lines drop "GitHub": "Release is one release from a
     ReleaseSource …", and "Asset is one release asset …". GitHub-specific
     remarks move into `GitHubSource`'s doc.
   * `ReleaseSource` gains a paragraph that names
     `selfupdatetest.VerifyReleaseSource`.
2. **`VerifyReleaseSource`**, new `selfupdate/selfupdatetest/conformance.go`:

   ```go
   // SourceCase names what VerifyReleaseSource expects of a source.
   type SourceCase struct {
       LatestTag  string // the tag Latest must return
       MissingTag string // a tag ByTag must report as missing
   }

   // VerifyReleaseSource checks src against the contract the Updater
   // relies on and returns every violation joined, or nil.
   func VerifyReleaseSource(ctx context.Context, src selfupdate.ReleaseSource, c SourceCase) error
   ```

   It checks:
   * `Latest` returns `LatestTag`, `Immutable`, and not a draft;
   * `ByTag(LatestTag)` returns the same release;
   * `ByTag(MissingTag)` matches `selfupdate.ErrNoRelease`;
   * each asset has `ID > 0`, a basename `Name`, `State ==
     AssetStateUploaded`, `Size > 0`, and a valid or empty `Digest`;
   * each asset's `OpenAsset` body is exactly `Size` bytes, with a
     SHA-256 equal to `Digest` when set.
3. **Tests**, new `conformance_test.go`:
   * `VerifyReleaseSource` passes for `NewFakeSource` and for a
     `GitHubSource` over `NewGitHubServer`;
   * it fails, naming the rule, for seven broken sources (wrapping a
     `FakeSource`): an unwrapped missing release, a mutable release, an
     asset with ID 0, the state "new", a short body, a wrong digest, and a
     wrong latest tag.
   * **Seen failing:** a plant dropping the `ErrNoRelease` check passes a
     broken source, and its case fails.
4. **GitHub Enterprise:** `TestGitHubAPIBasePathPrefix` in
   `github_test.go`. A server under `/api/v3` must see
   `/api/v3/repos/o/r/releases/latest` and the asset path under the same
   prefix. **Seen failing:** a plant of `normalizeAPIBase` that drops the
   path (`github.go:206`).
5. Arch: `selfupdatetest` 3/3. Checks; commit.

### Phase S9: `signednote` and its signer (D10, library side)

1. **New package `selfupdate/verify/signednote`.** It has three files:
   `doc.go`, `statement.go` and `signednote.go`.

   ```go
   const (
       Header      = "selfupdate-release/v1" // the statement's first line
       Suffix      = ".note"                 // the note is <manifest name> + Suffix
       MaxNoteSize = 16 << 10                // the OpenAsset bound
   )

   // Statement is the signed release statement.
   type Statement struct {
       Repository     string   `json:"repository"`      // <host>/<owner>/<name>, as github.server_url and github.repository give it
       Tag            string   `json:"tag"`             // a semantic version tag
       Products       []string `json:"products"`        // ascending, unique, at least one
       Manifest       string   `json:"manifest"`        // the manifest's asset name, such as SHA256SUMS
       ManifestSHA256 string   `json:"manifest_sha256"` // 64 lowercase hex
   }
   func (s Statement) Text() (string, error)       // validates, then the five LF lines with a final LF
   func ParseStatement(text string) (Statement, error) // strict: exactly the five lines, in order, LF only
   func Open(note []byte, keys []string) (Statement, error) // note.Open with the keys, then ParseStatement

   type Config struct {
       Repository   string   // required: <host>/<owner>/<name>
       Keys         []string // required: note verifier keys, name+hash+base64; no duplicates
       RequiredFrom string   // "" means every tag; else tags below it may be unsigned
   }
   func New(c Config) (selfupdate.ManifestVerifier, error)
   ```

   The validation:
   * `Repository` matches
     `^[a-z0-9][a-z0-9.-]*/[A-Za-z0-9._-]+/[A-Za-z0-9._-]+$`;
   * `Tag` and `RequiredFrom` pass `semver.IsValid` with no build
     metadata;
   * each product, and `Manifest`, match
     `^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`;
   * `ParseStatement` refuses CR, a blank line, a missing final LF, a
     duplicate or unsorted product, and any trailing line;
   * each key passes `note.NewVerifier`, and `New` refuses a duplicate
     key.

   `VerifyManifest(ctx, m)`:
   1. The note name is `m.Selection.ManifestName + Suffix`.
   2. When `m.Release.Assets` has no asset of that name, and `RequiredFrom`
      is set with `semver.Compare(m.Release.Tag, RequiredFrom) < 0`, it
      returns nil. Otherwise a missing note is an error naming the tag.
   3. It reads the note through `m.OpenAsset(ctx, name, MaxNoteSize)`.
   4. It calls `Open(note, c.Keys)`.
   5. It requires `Repository == c.Repository`, `Tag == m.Release.Tag`,
      `m.Product` in `Products`, `Manifest == m.Selection.ManifestName`,
      and `ManifestSHA256 == hex(sha256(m.Manifest))`.

   The Updater joins every failure with `ErrIntegrity`
   (`manifestverify.go:48-62`). A note present on a tag below
   `RequiredFrom` is verified all the same. The doc says so, and says
   that a prerelease of `RequiredFrom` sorts below it.

   `doc.go` follows `ghattest/doc.go`: what it checks, what it proves
   (that the holder of a trusted key signed this manifest for this
   repository, tag and product), what it does not prove (freshness, or
   key custody), opt-in, and the failure modes.
2. **depguard** (`.golangci.yml`):
   * `selfupdate-verify-signednote` allows `$gostd`,
     `golang.org/x/mod/semver$`, `golang.org/x/mod/sumdb/note$` and
     `github.com/maccavelli/go-selfupdate-lib/selfupdate$`, with the
     shape of `selfupdate-verify-ghattest` (`:214-225`);
   * `internal-cmd-selfupdate-note` allows `$gostd`,
     `golang.org/x/mod/sumdb/note$` and
     `github.com/maccavelli/go-selfupdate-lib/selfupdate/verify/signednote$`;
   * `other-packages` excludes both directories.

   Both names sort after `banned`. `go.mod` is unchanged, because
   `golang.org/x/mod` is required and `module` allows it by prefix.
3. **New `internal/cmd/selfupdate-note/main.go`:**
   * `keygen -name NAME -out FILE` writes the signer key (mode 0600,
     refusing an existing FILE) and prints the verifier key on standard
     output. A key name may not contain a space or `+`.
   * `sign -dir DIR -manifest SHA256SUMS -repository R -tag T
     -products-json J -verifier-key V -out FILE`:
     1. It reads the signer key from the environment variable
       `SELFUPDATE_NOTE_KEY`, never a flag, and an empty value is exit 1.
     2. It hashes `DIR/<manifest>` and builds the `Statement`.
     3. It signs with `note.Sign`.
     4. It opens the result with `signednote.Open(out, []string{V})`, so
        a key that does not match V is exit 1.
     5. It writes FILE, refusing an existing one.
   * Exit codes are 0, 1 on a failed check, and 2 on usage, as
     `selfupdate-release` (`main.go:1-18`). The tool never prints the
     signer key.
4. **The reserved name.**
   * `scripts/verify-selfupdate-release.sh:152-158` adds `extra ==
     "SHA256SUMS.note"` to the refused extras.
   * `selfupdate/releasespec/spec.go:368` refuses the same name as
     "reserved for the release statement".
   * `releasespec/parity_test.go:140-141` gains the case, and
     `spec_test.go` (beside `:245`) its case.
   * `verify-selfupdate-release_test.sh` gains `run_refused "extra named
     SHA256SUMS.note" "invalid or duplicate extra asset" …`, after
     `:259-263`.
5. **Tests:**
   * `statement_test.go`: round trips; every refusal in item 1; and
     byte-identical `Text()` for equal statements.
   * `signednote_test.go`: a key pair from `note.GenerateKey` with a fixed
     `rand` reader; then through `selfupdatetest.NewFakeSource` and an
     Updater, the cases below:
     * valid, which passes;
     * a wrong key;
     * a changed tag, repository, product, manifest name or manifest
       byte;
     * a missing note at or above `RequiredFrom`;
     * a missing note below it, which passes;
     * a note over 16 KiB;
     * CRLF.

     Each failing case matches `selfupdate.ErrIntegrity`.
   * `example_test.go`: `ExampleNew`, compile-only.
   * `fuzz_test.go`: `FuzzParseStatement`, which never panics and whose
     accepted text round-trips. The Makefile's `fuzz` target gains
     `./scripts/go-fuzz.sh -t $(FUZZTIME) -m 1
     ./selfupdate/verify/signednote`.
   * `internal/cmd/selfupdate-note/main_test.go` covers:
     * `keygen` writes mode 0600 and prints a key `note.NewVerifier`
       accepts;
     * `sign` with an empty `SELFUPDATE_NOTE_KEY` exits 1;
     * a mismatched `-verifier-key` exits 1;
     * the output opens and equals a second run's bytes;
     * the signer key never appears on standard output or standard
       error.
   * **Seen failing:** a plant skipping the tag comparison; one skipping
     the manifest hash; one in `verify-selfupdate-release.sh` without the
     new name; and one in `sign` that skips the self-check. Each fails its
     test.
6. **Docs:**
   * AGENTS.md's scope paragraph names `selfupdate/verify/signednote`
     (0025-MADR D10). Its depguard list names both rules, and the
     `other-packages` sentence names `internal/cmd/selfupdate-note`.
   * `docs/architecture.md` gets the tree lines, two table rows
     (`selfupdate/verify/signednote/`: 3 / 4, including a fuzz target,
     non-standard imports `x/mod/semver`, `x/mod/sumdb/note`;
     `internal/cmd/selfupdate-note/`: 1 / 1, `x/mod/sumdb/note`), the
     depguard list (`:512-524`), and the opening paragraph's tools.
   * README's package table gets a row.
7. Checks, including `GOOS=windows` and `darwin` vet and lint; commit.

### Phase S10: the publish workflow's `sign` job (D10, workflow side)

1. **`publish-selfupdate-release.yml`.**
   * `on.workflow_call` gains:
     * the input `sign` (`type: boolean`, `required: false`, `default:
       false`);
     * the input `note-verifier-key` (`type: string`, `required: false`,
       `default: ''`), the verifier key the new note must open with;
     * `secrets: SELFUPDATE_NOTE_KEY: {required: false}`.
   * A new job, `sign`, before `publish`:
     * `if: inputs.sign`, `runs-on: ubuntu-24.04`, `environment:
       selfupdate-signing`, job `permissions: contents: read`.
     * Its steps are copies of `publish`'s steps:
       1. the tools checkout (`:78-87`);
       2. "Require an admitted tag" (`:97-109`);
       3. the artifact download (`:132-136`);
       4. "Validate the staged file set" (`:141-160`);
       5. `actions/setup-go` at the pinned SHA, with `go-version-file:
          .core-lib-release-tools/go.mod` and `cache: false`.
     * Step "Sign the manifest": `working-directory:
       .core-lib-release-tools`, with `env` holding:
       * `SELFUPDATE_NOTE_KEY: ${{ secrets.SELFUPDATE_NOTE_KEY }}`;
       * `VERIFIER_KEY: ${{ inputs.note-verifier-key }}`;
       * `SERVER_URL: ${{ github.server_url }}`;
       * `REPOSITORY: ${{ github.repository }}`;
       * `TAG: ${{ github.ref_name }}`;
       * `PRODUCTS_JSON: ${{ inputs.products-json }}`;
       * `GOTOOLCHAIN: local`.

       It runs `go run ./internal/cmd/selfupdate-note sign -dir
       "$GITHUB_WORKSPACE/staging" -manifest SHA256SUMS -repository
       "${SERVER_URL#https://}/$REPOSITORY" -tag "$TAG" -products-json
       "$PRODUCTS_JSON" -verifier-key "$VERIFIER_KEY" -out
       "$RUNNER_TEMP/note/SHA256SUMS.note"`. A preceding `test -n
       "$VERIFIER_KEY"` fails closed. The tool itself refuses an empty
       secret, so a caller that forgets to pass the secret (the silent
       empty string) fails here.
     * `actions/upload-artifact` at the pin `build-selfupdate-release.yml:256`
       uses, with name `${{ inputs.artifact-name }}-note`, path
       `${{ runner.temp }}/note/SHA256SUMS.note`, `if-no-files-found:
       error` and `retention-days: 1`.
   * `publish` gains `needs: sign` and `if: ${{ !cancelled() &&
     (needs.sign.result == 'success' || (needs.sign.result == 'skipped' &&
     !inputs.sign)) }}`.
   * `publish` gains a step after "Check archived programs" and before
     "Create a draft release": "Add the release statement", with `if:
     inputs.sign`. It is `actions/download-artifact` at the pinned SHA,
     name `${{ inputs.artifact-name }}-note`, path `staging`. A following
     step with the same guard runs `test -f staging/SHA256SUMS.note`.
   * The upload loop (`:218-222`) and `subject-path: staging/*` (`:224`)
     then carry the note unchanged. The existing steps and their guards
     are unchanged, so `workflow-shape_test.sh:84` still holds.
2. **`scripts/workflow-shape_test.sh`** gains these assertions:
   * the `sign` input is boolean with default `false`;
   * the secret is declared and not required;
   * `jobs.sign` has `if: inputs.sign` and `environment:
     selfupdate-signing`;
   * its "Sign the manifest" step reads the secret only through `env` and
     has no `continue-on-error`;
   * `jobs.publish.needs` is `sign`, and the `if` is the exact string
     above;
   * "Add the release statement" sits after `archives` and before
     `draft`, guarded by `inputs.sign`.

   **Seen failing:** a plant of the workflow with `continue-on-error:
   true` on the sign step, and one with the add step after `draft`.
   `scripts/check-workflows.sh` and `actionlint` pass.
3. **Docs:**
   * `docs/guides/building-releases.md` gets a new `## 13. Sign a release
     (opt-in)` after §12. It shows:
     * the caller YAML (`sign: true`, `note-verifier-key: ${{
       vars.SELFUPDATE_NOTE_VKEY }}`, `secrets: SELFUPDATE_NOTE_KEY: ${{
       secrets.SELFUPDATE_NOTE_KEY }}`);
     * the environment `selfupdate-signing` with a `v*` tag rule;
     * `go run ./internal/cmd/selfupdate-note keygen` on the owner's
       machine;
     * the program's `signednote.Config` with `RequiredFrom`;
     * 0004-REPORT's rotation and backup-key practice, by link.

     §4's caller example notes the two optional inputs.
   * `docs/guides/extending-selfupdate.md`'s `## Verify a signature
     later` becomes `## Verify a release signature`, built on
     `signednote`. Every link to `#verify-a-signature-later` is updated
     (`git grep`), and `--links` passes.
   * `docs/architecture.md`'s "Release workflow" (`:420-466`) describes
     the `sign` job. "What is not here" (`:618-619`) drops "Release
     signing" and says that signatures are opt-in.
   * `selfupdate/doc.go:102-103` and `README.md:59` keep "no publisher
     signature is verified by default", and add a pointer to
     `signednote`.
4. Checks; commit. The live run is S13.

### Phase S11: the Go floor, `go 1.26.9` (D8)

1. `go.mod`: `go 1.27.1` becomes `go 1.26.9`. `toolchain go1.27.2` stays.
2. `selfupdate/managed_stopped_test.go`, each old string asserted to
   occur once:
   * `:95` becomes `l := &seqEnabledLife{seqLife: seqLife{running: true}}`;
   * `:99` becomes `l := &seqEnabledLife{seqLife: seqLife{running: true},
     enabledErr: errors.New("fixture: unit unreadable")}`;
   * `:226` becomes `l := &seqEnabledLife{seqLife: seqLife{healthErr:
     unhealthy}, enabled: true}`.
3. The test fixtures' `go` lines become `go 1.26.9`:
   * from `go 1.27`: `internal/testproc/testproc_test.go:151`,
     `selfupdate/archive/main_test.go:79`,
     `selfupdate/codesign/live_darwin_test.go:49` and
     `selfupdate/imageverify_test.go:84`;
   * from `go 1.27.1`: `buildinfo/stamp_test.go:41`,
     `internal/cmd/selfupdate-release/installer_ps_test.go:574`,
     `plan_test.go:214`, `testdata/fixture/go.mod:3`,
     `selfupdate/e2e_running_test.go:71` and `scripts/go-fuzz_test.sh:30`.

   `stage_test.go:339`'s `"go1.26.6"` (the "another toolchain" case) and
   `buildinfo_test.go:62`, `:76` are data, and stay.
4. **CI**, `.github/workflows/ci.yml`: a new job `go126`.
   * Its comment is `# The module's floor, go 1.26.9; GOTOOLCHAIN=local
     stops the toolchain line switching to 1.27.2 (0025-MADR D8)`.
   * `runs-on: ubuntu-24.04`, and job `env: GOTOOLCHAIN: local`.
   * Its steps:
     1. checkout at `3d3c42e5…` with `persist-credentials: false`;
     2. setup-go at `b7ad1dad…` with `go-version: '1.26.9'`;
     3. `test "$(go env GOVERSION)" = go1.26.9`;
     4. `go build ./...`;
     5. `CGO_ENABLED=0 GOOS=windows go vet ./...` and `GOOS=darwin`;
     6. `go vet ./...`;
     7. `go test -count=1 ./...`.

   The release-rehearsal jobs, which read `testdata/fixture/go.mod`
   (`ci.yml:336-337`), now build with 1.26.9. That is how the floor
   reaches the build workflow's tool.
5. **Docs:**
   * AGENTS.md `:38-40` becomes "Requires Go 1.26.9; built and tested with
     Go 1.27.2, the `toolchain` line in `go.mod`
     (`0018-MADR-…`; `0025-MADR-…` D8)".
   * README.md `:22-24` says the module requires 1.26.9, and advises
     building with 1.26.9 or 1.27.2, both advisory-free on 2026-10-19.
   * `docs/architecture.md:17` and the advisories note at `:19-21`.
   * The migration guide: `## 1. Go 1.27.1` becomes `## 1. Go 1.26.9 or
     later` (no link targets `#1-go-1271`; checked again with `git grep`
     before the change), and `:16` and `:179` follow.
6. **Checks:**
   * in a scratch clone, `GOTOOLCHAIN=go1.26.9 go vet ./...` (linux,
     darwin, windows, freebsd) and `go test -count=1 ./...` exit 0;
   * `GOTOOLCHAIN=go1.26.9 govulncheck ./...` exits 0;
   * `go mod tidy -diff` is clean;
   * `make gate` passes with 1.27.2.
7. **Seen failing:** a plant adding `strings.CutLast` to `buildinfo`
   fails the `go126` job's commands run locally under `GOTOOLCHAIN=go1.26.9`
   ("undefined: strings.CutLast"), and fails the main job's vet
   ("requires go1.27 or later"). Commit.

### Phase S12: `retract`, the changelog, the policy, the trust model, the remaining docs (D6, D7, D9, D13, D14)

1. **`retract`** (D7): `go.mod` gains, after the `toolchain` line, the
   single line `retract [v1.0.0, v1.4.1] // declared
   github.com/maccavelli/go-core-lib; use v1.5.0 or later`. No comment
   goes above it, since such a comment would join the rationale. `go mod
   tidy -diff` is clean, and `go mod edit -json` shows the range.
2. **`CHANGELOG.md`** at the root (D7):
   * Keep a Changelog 1.1.0 form. The header states the module's semver
     policy and links the migration guide.
   * `## [Unreleased]` holds S1 to S11's changes, grouped Added, Changed,
     Fixed and Security.
   * One `## [vX.Y.Z] - YYYY-MM-DD` per tag from `v1.15.2` down to
     `v1.5.0`. Each has its one-to-two-line summary, its migration-guide
     anchor and its records, from this plan's "Changelog source" table.
   * A final line for `v1.0.0`–`v1.4.1`: published as `go-core-lib`, and
     retracted here.
   * README.md links it next to `**Documentation:**`. `docs/README.md`'s
     "I want to…" table gains "see what changed in each release".
3. **The evolution policy** (D6):
   * a `## API evolution` section in `docs/architecture.md`, after `## Go
     code`, with the MADR's six rules;
   * a short `## Evolve with the library` section in
     `extending-selfupdate.md` before `## Test a program that
     self-updates`, telling implementers that a new capability arrives as
     an optional interface (`EnabledLifecycle` at `:456-471` as the
     example).
4. **The trust model and `SECURITY.md`** (D9):
   * A `## Trust model` section in `docs/architecture.md`, after `##
     Release workflow`, covering:
     * what a run proves: TLS to the API origin, the immutable-release
       gate, `SHA256SUMS` and the GitHub digest, the exact tag;
     * what it does not prove: a publisher signature without
       `signednote`; that `SHA256SUMS` shares the binary's origin;
       freshness, with the freeze attack named and linked to TUF; and the
       redirect hosts, which the checksum guards;
     * what `signednote`, `ghattest` and `codesign` add.
   * Root `SECURITY.md` holds:
     * reporting, through GitHub private vulnerability reporting on this
       repository;
     * supported versions (the newest `v1` minor);
     * a five-line trust-model summary linking the section;
     * the `govulncheck` and `make gate` practice.
   * README links it. `docs/README.md` gains "report a vulnerability" and
     "know what an update proves".
5. **Variant recipes** (D13): `extending-selfupdate.md`'s `## Ship an
   archive` (`:206`) gains `### Universal, musl and arm variants`. It
   covers:
   * a `Name` function mapping both darwin pairs to one universal asset;
   * a custom `AssetSelector` for raw variant binaries;
   * that `GoReleaserName` refuses `arm` and `mips*`.

   `docs/architecture.md`'s "What is not here" gains `Platform.Variant`
   as future work, which needs the release spec, the workflows and the
   installers together.
6. **The remaining D14 items:**
   * new `selfupdate/service/example_test.go`, `ExamplePollHealthy`: a
     probe that returns `Health{Ready: true, Instance: "42"}`,
     `PollOptions{Settle: -1}`, and `// Output: healthy`. **Seen
     failing:** a planted wrong output;
   * README's pkg.go.dev badge under the H1: `[![Go
     Reference](https://pkg.go.dev/badge/github.com/maccavelli/go-selfupdate-lib.svg)](https://pkg.go.dev/github.com/maccavelli/go-selfupdate-lib)`;
   * the surface review, recorded in the execution record. The facts'
     32-name heuristic list, each checked: enum members, sentinels, `…Func`
     adapters, or types reachable from exported signatures. No accidental
     export, and nothing deprecated.
7. Arch: `service` 16/16. Snippets built; checks; commit.

### Phase S13: the signing rehearsal (D10)

On the owner's ask, after S12 is pushed to `main`, following 0013-PLAN
step 3's precedent:

1. **Setup:**
   * A throwaway public repository is created; immutable releases are
     turned on (`PUT …/immutable-releases`, 204).
   * It holds a small program (`version`, `update`) from an embedded spec,
     and a caller workflow pinned to S12's commit, with `sign: true`.
   * Keys come from `selfupdate-note keygen`, run on this machine in a
     scratch directory, and are never committed.
   * The environment is `selfupdate-signing`, with a deployment rule for
     `v*` tags (`gh api -X PUT
     repos/<owner>/<repo>/environments/selfupdate-signing`). The secret is
     set with `gh secret set SELFUPDATE_NOTE_KEY --env selfupdate-signing`,
     and the variable `SELFUPDATE_NOTE_VKEY` holds the verifier key.
2. **Runs, each recorded with its run ID and outcome:**
   * `v0.0.1` without `sign`: published, no note.
   * `v0.0.2` signed: the release has `SHA256SUMS.note`, and the note opens
     with the verifier key.
   * The program at `v0.0.1`, with `RequiredFrom: v0.0.2` and the verifier
     key embedded, then:
     * `update --check` and `update --yes` reach `v0.0.2` on this Mac, the
       Linux test host and the Windows test host;
     * a build with a wrong key refuses `v0.0.2` with `ErrIntegrity`;
     * `update --version v0.0.1`, below `RequiredFrom`, installs.
   * `v0.0.3` signed with the secret unset in the caller: the `sign` job
     fails at the empty-secret check, and nothing is published.
   * `v0.0.4` signed with a verifier key that does not match the secret:
     the `sign` job fails at the self-check, and nothing is published.
3. **The unverified points**, settled and recorded:
   * which repository's environment the reusable job resolves;
   * whether a `deployments` permission is needed.

   Either one failing is a deviation that stops the phase.
4. **Teardown:** the repository is deleted, and the record names it only
   as "the throwaway repository". The keys are deleted.

### Phase S14: `v1.16.0` and the repository settings

1. The migration guide gains `## 16. From v1.15 to v1.16`, with what
   changed and how to adopt each part:
   * the visible formatter output;
   * `HTTPError` and `CommandError`;
   * the `…Context` functions;
   * `NewGitHubUpdater`, `Now`, `NewVersionProberWith`, `NewSlogReporter`
     and `VerifyReleaseSource`;
   * signing;
   * the 1.26.9 floor;
   * `retract`.

   It moves the guide's current version to `v1.16.0`. `CHANGELOG.md`'s
   `[Unreleased]` becomes `[v1.16.0] - <date>`. `docs/README.md` links §16.
2. [0017-PLAN-verify-build-provenance-and-close-0015-open-items.md](0017-PLAN-verify-build-provenance-and-close-0015-open-items.md)'s
   "Release procedure", as 0024-PLAN W10 ran it:
   1. CI green on `main`. Expect 18 jobs, with `go126` added.
   2. The live suites recorded: the service backends' live tests on their
      hosts as CI runs them, and S13.
   3. The tag on the owner's ask. `scripts/check-release-tag.sh v1.16.0`
      exits 0, the disclosure guard runs over the outgoing tag, and the
      tag is pushed.
   4. The tag's CI, with its identity legs printing `v1.16.0 (release)
      <12-hex>`.
   5. The pin commit. Candidates come from `git grep -n 'v1\.15\.2\|<7-hex
      of v1.15.2>' -- README.md docs/architecture.md docs/guides
      CHANGELOG.md`, and each changed or kept line is named.
   6. The proxy: `go list -m -json …@v1.16.0` gives the commit, and
      `@latest` resolves to `v1.16.0`.
   7. **`retract` confirmed:** with `GOMODCACHE` and `GOPATH` in a new
      temporary directory, `GOFLAGS=-modcacherw go list -m -versions
      github.com/maccavelli/go-selfupdate-lib` starts at `v1.5.0`, and
      `-retracted` lists `v1.0.0` too.
   8. Release notes in the execution record.
3. **The repository settings (D9).** The agent prepares and the owner
   runs:

   ```bash
   R=<owner>/go-selfupdate-lib
   gh api -X PUT repos/$R/private-vulnerability-reporting   # expect 204
   gh api -X PUT repos/$R/vulnerability-alerts              # expect 204
   gh api -X POST repos/$R/rulesets --input ruleset.json     # tag ruleset below
   ```

   `ruleset.json`:

   ```json
   {
     "name": "release tags",
     "target": "tag",
     "enforcement": "active",
     "conditions": { "ref_name": { "include": ["refs/tags/v*"], "exclude": [] } },
     "rules": [
       { "type": "creation" },
       { "type": "update", "parameters": { "update_allows_fetch_and_merge": false } },
       { "type": "deletion" }
     ],
     "bypass_actors": [ { "actor_id": 5, "actor_type": "RepositoryRole", "bypass_mode": "always" } ]
   }
   ```

   **Verification (read-only):**
   * `private-vulnerability-reporting` gives `{"enabled":true}`;
   * `vulnerability-alerts` gives 204;
   * `automated-security-fixes` stays `{"enabled":false,…}`;
   * `rulesets/<id>` has `current_user_can_bypass` equal to `always`;
   * `branches/main/protection` is still 404, so nothing touches `main`.

   That `actor_id` 5 is the repository admin role is [unverified]. If
   `current_user_can_bypass` is not `always`, the ruleset is deleted at
   once and the phase stops as a deviation. The likely fix is a `User`
   bypass actor with the owner's numeric ID.

   The ruleset is created **after** the `v1.16.0` tag is pushed, so the
   release cannot be blocked by it. The next release confirms that the
   owner can still push a `v*` tag.

### Phase S15: closing

* The scorecard of 0025-MADR is re-run against the tree, with the four
  audits' commands where they apply, and recorded row by row.
* `### Verification, at closing` is written, and this plan becomes
  `status: complete`, with its index row updated. The MADR stays
  `accepted`.

## Changelog source

From the migration guide's sections and the records index. Used by S12
step 2.

| Tag | Date | Guide anchor | Records | Summary |
| :--- | :--- | :--- | :--- | :--- |
| `v1.15.2` | 2026-10-12 | `#from-v1151-to-v1152` | 0024 pair; 0021-PLAN-instance-race-and-ci-live-tests.md | Task Scheduler's `StopGrace` checked against the timeout; a failed stop leaves the task enabled; `FakeSource` wraps `ErrNoRelease`; `install.ps1` gives hooks no input |
| `v1.15.1` | 2026-10-11 | `#from-v1150-to-v1151` | 0022-PLAN-error-chain-and-process-liveness.md | Task Scheduler reads an exited program as gone; no API change |
| `v1.15.0` | 2026-10-11 | `#15-from-v114-to-v115` | 0022-MADR; 0022-PLAN-documentation-pass-after-v1-14-0.md | `ErrBatchFile` exported; a disabled task it cannot restart is refused |
| `v1.14.0` | 2026-10-10 | `#14-from-v113-to-v114` | 0021 pair | the Task Scheduler backend; windowless Windows children; batch files refused |
| `v1.13.0` | 2026-10-09 | `#13-from-v112-to-v113` | 0020 pair | `ManagedOptions.ReplaceBeforeStop` |
| `v1.12.1` | 2026-10-08 | `#from-v1120-to-v1121` | 0017 pair | the Windows installer fixed; no API change |
| `v1.12.0` | 2026-10-08 | `#12-from-v111-to-v112` | 0017 pair | `selfupdate/verify/ghattest`; an interrupted update's backup kept |
| `v1.11.1` | 2026-10-08 | `#from-v1110-to-v1111` | 0017 pair; 0018 pair | the last archive differentials closed; `toolchain go1.27.2` |
| `v1.11.0` | 2026-10-08 | `#11-from-v110-to-v111` | 0015 pair | `ErrNotConstructed`; a 404 is `ErrNoRelease`, cached as `no-release` |
| `v1.10.1` | 2026-10-07 | `#from-v1100-to-v1101` | 0015 pair | the third debugging pass's fixes; no API change |
| `v1.10.0` | 2026-10-07 | `#10-from-v19-to-v110` | 0014 pair | the spec's `installer`: `install.sh` and `install.ps1` |
| `v1.9.0` | 2026-10-06 | `#9-from-v18-to-v19` | 0013 pair | `selfupdate/releasespec` and the build workflow |
| `v1.8.0` | 2026-10-05 | `#8-from-v17-to-v18` | 0012 pair | `Config.Unpacker`; `selfupdate/archive` and `selfupdate/codesign` |
| `v1.7.0` | 2026-10-05 | `#7-from-v16-to-v17` | 0011 pair | `selfupdate/service` and the systemd, launchd and SCM backends |
| `v1.6.0` | 2026-10-04 | `#6-from-v15-to-v16` | 0010-MADR; 0010-PLAN-v1-6-0-owner-contracts.md | post-update errors are warnings; JSON schema 2; a stopped service stays stopped |
| `v1.5.1` | 2026-10-03 | `#from-v150-to-v151` | 0010-PLAN-v1-5-1-contract-preserving-fixes.md | every failure writes the `--json` result; no API change |
| `v1.5.0` | 2026-10-02 | `#2-the-go-import` | 0009 pair | the first release under this path |

## Verification

* **V1 (decisions).** D1–D14 are built as the phases say, and every
  deviation is recorded.
* **V2 (seen failing).** Every new test and assertion has its plant and
  quoted failure in the record.
* **V3 (compatibility).** `make apicheck` is compatible with `v1.15.2` at
  every phase and at the tag.
* **V4 (the floor).** The `go126` job passes, and `govulncheck` with
  1.26.9 and 1.27.2 reports nothing.
* **V5 (signing).** S13's runs match their expected outcomes.
* **V6 (versions).** `go list -m -versions` on a fresh cache starts at
  `v1.5.0`.
* **V7 (checks).** Links, identifiers, the arch table, markdownlint and
  actionlint are clean. `make gate` ends `overall=0` before each push.
* **V8 (release).** `v1.16.0` is tagged, its CI passed, and the proxy
  serves it.
* **V9 (settings).** S14 step 3's GETs give the expected values, and
  `main` is untouched.
* **V10 (scorecard).** S15's re-run is recorded.

## Rollout and Rollback

* **Rollout.** One commit per phase S0 to S12, on `main`, pushed on the
  owner's ask. S13 tests the pushed commit. S14 tags `v1.16.0` once.
* **Rollback.** Before the tag, `git revert` of a phase's commit. After
  it, fix forward in `v1.16.1`.
  * The formatter output change cannot be undone without a release.
  * `retract` cannot be withdrawn, only narrowed by a later `go.mod`.
  * The ruleset is deleted with `gh api -X DELETE
    repos/<owner>/go-selfupdate-lib/rulesets/<id>`.
* **Signing** stays off for every consumer until it sets `sign: true` and
  embeds keys. Turning it off is `sign: false`, plus a release whose
  `RequiredFrom` is removed.
* **A deviation in any phase** stops it.

## Execution record

### Approval (2026-10-19)

The owner answered the MADR's decisions on 2026-10-19 (D9-A without a
rule on `main`; one release), set the floor to `go 1.26.9` (MADR
Amendment A1), and then approved this plan: "accept/approved to proceed".
The plan became `in-progress`.

### S0 (2026-10-19)

1. `HEAD` was `5c0ab0d` on `main`. The tree was clean apart from this
   pair and `docs/README.md`. Hooks resolve to the global hooks
   directory.
2. `docs/README.md` holds the MADR's row (`accepted`) and this plan's
   row.
3. 0004-MADR gains amendment `P4 (2026-10-19)`, and its open-work row for
   `signednote` is struck through and annotated.
4. 0004-REPORT gains `## Since (2026-10-19)`: the correction of the
   secrets bullet, and the build.
5. 0018-MADR gains `## Amendments` with `A1 (2026-10-19)`.
