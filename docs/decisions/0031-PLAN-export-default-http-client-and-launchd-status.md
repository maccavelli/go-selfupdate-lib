---
status: in-progress
date: 2026-10-23
associated-madr: "0031-MADR-export-default-http-client-and-launchd-status.md"
---
# Implement `DefaultHTTPClient` and `launchd.Job`'s `Loaded` and `Status`, and release `v1.18.0`

Associated MADR: [0031-MADR-export-default-http-client-and-launchd-status.md](0031-MADR-export-default-http-client-and-launchd-status.md)
(D1-A, D2-A, D3-A and D4-C, as the owner chose).

## Goal

* `selfupdate.DefaultHTTPClient()` returns the library's recommended
  client; `NewGitHubUpdater` uses it, and a nil `GitHubOptions.Client`
  is still refused, with an error that names it (issue #5).
* `(*launchd.Job).Loaded` and `(*launchd.Job).Status` report `probe`'s
  state (issue #4).
* The library's own examples stop passing `Client.Timeout`.
* Every new test is seen failing first, and both ship as `v1.18.0`.
  Issues #5 and #4 are closed on the owner's ask.

## Scope

### In scope

| Phase | What | Commit |
| :--- | :--- | :--- |
| H0 | this pair, its index rows | records only |
| H1 | `selfupdate`: `DefaultHTTPClient`, the nil-client error, the example | code and tests |
| H2 | `launchd`: `Loaded`, `Status` | code and tests |
| H3 | the guide's example, the migration guide's §18, the changelog, the architecture page | docs |
| H4 | `v1.18.0` | per the architecture page's "Releasing this module" |
| H5 | closing | records only |

### Out of scope

* `NewHTTPClient(HTTPClientOptions)` (MADR D1-B), and a nil
  `GitHubOptions.Client` meaning a default (D2-B).
* A status snapshot for `systemd`, `scm` or `taskscheduler`, or a shared
  interface (MADR, More Information).
* Any change to `probe`'s behaviour.
* No `.go` file is added or removed, so the architecture page's package
  table is unchanged.

## Rules

* **`main`.** Every phase commits on `main` with `git commit --no-edit`
  (the global hook writes the message). A push or a tag needs the
  owner's word in that turn.
* **Each phase stops at a deviation**, recorded here, and in the MADR
  when it changes a decision or a fact.
* **Checks before each commit:**
  * `make pre-add-check FILES="…"` on the changed Go files;
  * `scripts/check-docs.sh --links` and `--ids`, and
    `npx --yes markdownlint-cli2@0.23.2`, on changed Markdown;
  * `make apicheck`, compatible with `v1.17.2`.

  `make gate` must end `overall=0` before the push.
* **Seen failing first:** every new test fails on a
  `scripts/plant-copy.sh` copy with one planted break, never on a dirtied
  tree, and this plan quotes the failure.

## Implementation Steps

### Phase H0: records

1. `docs/README.md` gains this pair's rows after 0030's.
2. Checks; commit on the owner's word.
3. On approval, the MADR records the owner's choices and becomes
   `accepted`, and this plan becomes `in-progress` with an
   `### Approval` entry, in the first commit of H1 or a records commit
   before it.

### Phase H1: `selfupdate` (D1-A, D2-A, D3-A's example)

1. **`selfupdate/githubupdater.go`:**
   * `defaultHTTPClient` becomes exported `DefaultHTTPClient`, with the
     same body. Its doc says: a new client and a new `*http.Transport`
     on each call, owned by the caller and safe to adjust;
     `http.DefaultTransport`'s settings when it is a `*http.Transport`,
     otherwise Go 1.27.3's defaults; a 30 s `ResponseHeaderTimeout`;
     and no `Client.Timeout`, so the caller's context bounds a download
     (0025-MADR D14).
   * `NewGitHubUpdater` (`:59`) calls it, and `GitHubUpdaterOptions.Client`'s
     doc names it.
2. **`selfupdate/types.go:213-216`:** `GitHubOptions.Client`'s doc ends
   "DefaultHTTPClient returns the recommended client" in place of
   "NewGitHubUpdater's default client does that".
3. **`selfupdate/github.go:118-120`:** the nil-client error reads
   "selfupdate: github client is required; use
   selfupdate.DefaultHTTPClient()".
4. **`selfupdate/example_test.go:31`** passes `selfupdate.DefaultHTTPClient()`.
5. **Tests:**
   * `githubupdater_internal_test.go`'s `TestDefaultClientTimeouts`
     stays in its file, calls `DefaultHTTPClient`, and gains: `tr.Proxy`
     is set; two calls return different clients and different
     transports. The file stays, so the architecture table is unchanged.
   * A `NewGitHubSource` test with a nil client checks the error names
     `DefaultHTTPClient`.
6. **Seen failing:** a plant where `DefaultHTTPClient` returns a shared
   package-level client fails "different clients"; a plant that sets
   `Timeout: time.Minute` fails "no Client.Timeout"; a plant of the old
   error text fails the nil-client test.
7. `make apicheck` lists `DefaultHTTPClient` as compatible. Checks;
   commit.

### Phase H2: `launchd` (D4-C)

1. **`selfupdate/service/launchd/lifecycle.go`**, after `Running`:
   * `Loaded(ctx context.Context, _ string) (bool, error)`: `probe`'s
     `loaded`. Its doc: launchd knows the job in its domain, read by
     `launchctl list` when that sees the domain, otherwise `launchctl
     print`; exit 113 is `false, nil`; any other failure is an error,
     matched as `Running`'s are.
   * `type Status struct` with MADR D4-C's fields and JSON tags, and
     `Status(ctx context.Context, product string) (Status, error)`:
     `Installed`; one `probe`; `Enabled` only when installed. Any error
     returns `Status{}` and the error.
2. **Tests** (`launchd_test.go`, beside `TestInstalledAndRunning`, with
   `testproc.CheckNoLeak` first, as the file's tests do):
   * `TestLoaded`: `list` seen and loaded; `list` exit 113; `print`
     (another user's domain) loaded; `print` exit 113; a `list` exit 1
     is an error that `errors.As` finds as a `*service.CommandError`.
   * `TestStatus`: a running job (every field, `PID` 100); loaded with
     no process (`Running` false, `PID` 0); not loaded; a missing plist
     (`Installed` and `Enabled` false, no `plutil` in `f.verbs()`, and
     `probe` still runs); a `probe` error returning `Status{}`; exactly
     one `list` or `print` per call.
   * `json_test.go`: `TestStatusJSON` pins
     `{"installed":true,"loaded":true,"enabled":true,"running":true,"pid":100}`
     and round-trips it.
3. **Live:** a new `TestLiveStatus` in `live_darwin_test.go` builds its
   job as `TestLiveStopWaitsForSlowExit` (`:446`) does (`newSlowLiveJob`,
   `waitRunning`, `liveJobFor`). It checks `Status` while the job runs
   (`Installed`, `Loaded`, `Running`, and `PID` equal to `waitRunning`'s
   PID), and after `Stop` (`Loaded` and `Running` false, `PID` 0), with
   `Loaded` agreeing each time. Run on this Mac with
   `SELFUPDATE_REQUIRE_LAUNCHD=1`; CI's macOS job runs it, as it runs
   the other live tests.
4. **Seen failing:** a plant where `Loaded` returns `running` fails the
   loaded-not-running case; a plant where `Status` calls `Enabled`
   without checking `Installed` fails the missing-plist case; a plant
   that renames a JSON tag fails `TestStatusJSON`.
5. `make apicheck` lists `Job.Loaded`, `Job.Status` and `Status` as
   compatible. Checks; commit.

### Phase H3: the documents (D3-A)

1. **The migration guide's worked command**
   (`docs/guides/migrating-from-mcplib-selfupdate.md:222-238`): the
   `updateTimeout` constant and its comment go; `Client:` becomes
   `selfupdate.DefaultHTTPClient()`; a comment says `cli.Run`'s context
   bounds the run. The snippet compiles in a scratch module against the
   working tree (`replace`), outside the repository.
2. **The migration guide's §18, "From v1.17 to v1.18":** the two
   additions; a program that passes `&http.Client{Timeout: …}` moves to
   `DefaultHTTPClient()`; a launchd status command can use `Status`.
   The guide's current-release pointers move in H4's pin commit, not
   here.
3. **`CHANGELOG.md`:** `## Unreleased` with Added (`DefaultHTTPClient`,
   `Job.Loaded`, `Job.Status`, `launchd.Status`) and Changed (the
   nil-client error text), linking §18 and this pair.
4. **`docs/architecture.md:190-191`:** "its default client" becomes
   "`DefaultHTTPClient`"; the `launchd` paragraph (`:282`) names
   `Loaded` and `Status`.
5. Checks; commit.

### Phase H4: `v1.18.0`

The architecture page's "Releasing this module", steps 1 to 7, with
`vX = v1.18.0`:

1. The pre-tag change: `CHANGELOG.md`'s `## Unreleased` becomes
   `## v1.18.0 - <date>`.
2. `make gate` ends `overall=0`.
3. The push on the owner's word, and CI green on `main`.
4. `scripts/check-release-tag.sh v1.18.0` and the disclosure guard over
   the outgoing tag, then the annotated tag and its push on the owner's
   word.
5. The tag's CI run: every job passes, and the identity legs print
   `v1.18.0 (release) <12-hex>`.
6. The pin commit: `git grep -n 'v1\.17\.2'` lists the current-release
   pointers; each one that names the current release moves to
   `v1.18.0`, including the migration guide's `go get` lines, §5 and the
   workflow pin. Lines that name `v1.17.2` as history stay.
7. The proxy: `go list -m -json github.com/maccavelli/go-selfupdate-lib@v1.18.0`
   gives the tagged commit, and `@latest` resolves to it.
8. Issues #5 and #4 are closed with a comment naming `v1.18.0`, on the
   owner's ask.

### Phase H5: closing

`### Verification, at closing` with the output, `status: complete`, and
the index row. The MADR stays `accepted`.

## Verification

* **V1 (API):** `make apicheck` against `v1.17.2` reports only
  additions: `DefaultHTTPClient`, `launchd.Status`, `Job.Loaded`,
  `Job.Status`.
* **V2 (client):** the client tests pass, and each failed on its plant.
* **V3 (launchd):** `TestLoaded`, `TestStatus` and `TestStatusJSON` pass,
  and each failed on its plant; the live test passes on this Mac with
  `SELFUPDATE_REQUIRE_LAUNCHD=1`.
* **V4 (docs):** the guide's snippet compiles; markdownlint, the link
  check and the identifier check pass.
* **V5 (release):** `v1.18.0` is tagged, its CI passes, and the proxy
  serves it as `@latest`.
* **V6 (issues):** #5 and #4 are closed with the release named.

## Rollout and Rollback

* **Rollout:** one commit per phase on `main`, pushed on the owner's
  word; `v1.18.0` at H4. A consumer adopts it by requiring `v1.18.0`;
  nothing changes for a caller that does not call the new API, except
  the nil-client error's text.
* **Rollback:** before the tag, `git revert` of a phase; after it, fix
  forward, since exported API cannot be removed within v1.

## Execution record

### Approval

* 2026-10-23: the owner committed H0 (`bf400dd`) and said "committed,
  proceed", approving the MADR's proposed options and this plan as
  written. The MADR becomes `accepted` and this plan `in-progress`, in
  H1's commit.

### H1: `selfupdate` (2026-10-23)

* `defaultHTTPClient` is now exported `DefaultHTTPClient`, with the same
  body and the doc of step 1; `NewGitHubUpdater` calls it, and
  `GitHubUpdaterOptions.Client` and `GitHubOptions.Client` name it. The
  nil-client error reads "selfupdate: github client is required; use
  selfupdate.DefaultHTTPClient()". `ExampleNew_standalone` passes
  `selfupdate.DefaultHTTPClient()`, and its `net/http` import goes.
* `TestDefaultClientTimeouts` checks the proxy function and that two
  calls share neither client nor transport; `TestNewGitHubSource` checks
  the nil-client error names `DefaultHTTPClient()`.
* **Seen failing** (`scripts/plant-copy.sh`, each copy removed after):

  | Plant | Failure |
  | :--- | :--- |
  | `DefaultHTTPClient` returns one package-level client | `githubupdater_internal_test.go:33: two calls share a client or a transport` |
  | the client sets `Timeout: time.Minute` | `githubupdater_internal_test.go:16: Client.Timeout = 1m0s, want none` |
  | the old error text | `github_test.go:103: nil client: selfupdate: github client is required; want a refusal naming DefaultHTTPClient()` |

* `make apicheck`: "check-api-compat: compatible with v1.17.2". It runs
  apidiff for incompatible changes only, so it does not list additions,
  as step 7 and V1 assumed; `go doc ./selfupdate DefaultHTTPClient`
  shows the new function instead. This is a wording correction to the
  plan's expected output, not a change to what was built.
* `make pre-add-check` on the six Go files: "6 file(s) clean (gofmt,
  golangci-lint, go vet, go test, govulncheck)".


### H2: `launchd` (2026-10-23)

* `lifecycle.go` gains `Job.Loaded`, `type Status` with MADR D4-C's
  fields and JSON tags, and `Job.Status`: `Installed`, one `probe`, and
  `Enabled` only when the plist exists. `PID` is set only when the job
  runs.
* Tests: `TestLoaded` (`list` and `print` each loaded, loaded with no
  process, and exit 113; a `list` exit 1 found as a
  `*service.CommandError`); `TestStatus` (running, loaded with no
  process, not loaded, disabled, a missing plist while still loaded, and
  a probe error giving a zero `Status`; one probe each, and no `plutil`
  without a plist); `TestStatusJSON`.
* `TestLiveStatus` uses `newLiveJob`, which is `newSlowLiveJob(t, 0)`,
  rather than the slow job step 3 named: the test needs a job that stops
  promptly, and the constructor is the same. It passed on this Mac with
  `SELFUPDATE_REQUIRE_LAUNCHD=1`: `--- PASS: TestLiveStatus (0.99s)`.
  CI's macOS job runs it through `-run '^TestLive'`.
* **Seen failing** (`scripts/plant-copy.sh`, each copy removed after):

  | Plant | Failure |
  | :--- | :--- |
  | `Loaded` returns `s.running` | `launchd_test.go:145: no process: loaded false, <nil>; want loaded`, for `list` and `print` |
  | `Status` runs `Enabled` without the `installed` check | `launchd_test.go:189: Status = {… Loaded:false …}, selfupdate: service: not installed: <temp>/missing.plist; want {… Loaded:true … Running:true PID:100}` |
  | the `loaded` tag renamed `isLoaded` | `json_test.go:30: {"installed":true,"isLoaded":true,…}` |
  | `Status` reads `Running` again | `launchd_test.go:192: 2 probes (verbs ["managername" "list" "list" "print-disabled"]), want 1` |
  | `PID` always 0 (live, on this Mac) | `live_darwin_test.go:501: running: Status = {… Running:true PID:0}, <nil>; want {… PID:<pid>}` |

* `make apicheck`: "compatible with v1.17.2" (additions are not listed;
  H1's note). `go doc ./selfupdate/service/launchd Status` shows the
  type.
* `make pre-add-check` on the four Go files: "4 file(s) clean (gofmt,
  golangci-lint, go vet, go test, govulncheck)".
