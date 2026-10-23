---
status: proposed
date: 2026-10-23
decision-makers: go-selfupdate-lib maintainers
consulted: "GitHub issues #5, \"selfupdate: export the default HTTP client\", and #4, \"service/launchd: report whether a job is loaded\" (both 2026-10-22), from the owner for magic-cli-remote; a read of `selfupdate` and `selfupdate/service/launchd` at f07d3fd (v1.17.2 plus three records commits)"
informed: the fleet's programs that compose `selfupdate.New` or run a launchd job, magic-cli-remote first
---
# Export the library's recommended HTTP client as `DefaultHTTPClient` and keep `GitHubOptions.Client` required, and give `launchd.Job` a `Loaded` method and a one-call `Status` snapshot, released as `v1.18.0`

## Context and Problem Statement

Two small issues from the same consumer each ask the library to export
something it already computes privately.

**Issue #5.** `NewGitHubUpdater` builds a client whose transport bounds the
response headers but not the whole request, so that a large download is
bounded by the caller's context. A program that composes
`selfupdate.New` itself, for example to build a `ManagedInstaller`, must
pass `GitHubOptions.Client`, and cannot get that client. The field's own
documentation warns against `Client.Timeout`, and the migration guide's
example sets exactly that. magic-cli-remote followed the guide: a GitHub
request that stalls before its headers waits 15 minutes, where the
library's default client gives up after 30 seconds.

**Issue #4.** `launchd.Job` exposes `Installed` (the plist exists),
`Enabled` and `Running`. Whether launchd has the job loaded in its domain
is a fourth state, which the backend computes and does not export. A
status command must tell "plist present, not loaded" from "loaded, not
running". magic-cli-remote runs `launchctl print` itself for it, in two
places, and treats any failure as "not loaded".

This record decides how each is exported, answers the issues' open
questions, and releases both as `v1.18.0`.

**Facts [observed 2026-10-23, `main` at `f07d3fd`]:**

* **F1. The default client is private.**
  `selfupdate/githubupdater.go:101-121`, `defaultHTTPClient`, clones
  `http.DefaultTransport` (or rebuilds Go's defaults when a program
  replaced it with another `RoundTripper`), sets `ResponseHeaderTimeout`
  to 30 s, and sets no `Client.Timeout`. Its callers are
  `NewGitHubUpdater` (`:59`), which uses it when
  `GitHubUpdaterOptions.Client` is nil, and
  `githubupdater_internal_test.go:13`, `TestDefaultClientTimeouts`. Its
  doc comment still says "Go 1.27.2's" settings; the toolchain is 1.27.3
  (0029-MADR).
* **F2. `GitHubOptions.Client` is required, on purpose.**
  `NewGitHubSource` refuses a nil client with "selfupdate: github client
  is required" (`github.go:118-120`). `GitHubOptions`' doc says "every
  field is explicit" (`types.go:202`), and its `Client` doc ends
  "NewGitHubUpdater's default client does that" (`types.go:213-216`),
  naming a client the caller cannot reach.
  [0025-MADR-align-with-go-sdk-design-practices.md](0025-MADR-align-with-go-sdk-design-practices.md)
  D5 added `NewGitHubUpdater` as the defaulting path and rejected D5-B,
  zero `Limits` meaning `DefaultLimits()`, "because it turns a documented
  error (zero limits) into silent success".
* **F3. The library's own documents teach the trap.**
  * `docs/guides/migrating-from-mcplib-selfupdate.md:233-238`, the
    worked `update` command, passes `Client: &http.Client{Timeout:
    updateTimeout}` with a 15-minute `updateTimeout`, beside the comment
    "cli.Run bounds the whole run".
  * `selfupdate/example_test.go:31` builds its transport by hand, with
    the 30 s header timeout and the environment's proxy, and without
    Go's dial, TLS and idle defaults.
* **F4. `launchd` computes "loaded" and keeps it.**
  * `selfupdate/service/launchd/job.go:216-220`: the private `state` has
    `loaded`, `pid` and `running`.
  * `job.go:251-277`, `probe`: `launchctl list <label>` when that sees
    the job's domain (`listSees`, `:235`), otherwise `launchctl print
    <domain>/<label>`. Exit 0 is loaded, exit 113 is not loaded, and any
    other exit is an error naming the command.
  * `lifecycle.go:33-36`: `Running` returns only `running`. `Stop`,
    `Start` and `WaitHealthy` branch on `loaded` internally.
* **F5. The other states' costs.** `Installed` stats the plist
  (`lifecycle.go:22`). `Enabled` (`:44`) reads `RunAtLoad` and
  `KeepAlive` through `plutil` and, when either is set, runs `launchctl
  print-disabled`; with the plist missing it returns an error wrapping
  `service.ErrNotInstalled` (`plist.go:31-33`). A status command that
  calls `Installed`, `Enabled`, `Running` and a `Loaded` probes launchd
  twice for one state.
* **F6. The consumer's copies are looser than `probe`.**
  magic-cli-remote at `6db2792`: `internal/cli/service/control.go:150-156`
  and `internal/cli/service/launchd_wait.go:54-57` treat any failure of
  `launchctl print` as "not loaded", and never use `launchctl list`.
  Its `0054-PLAN-standardize-self-update-on-go-selfupdate-lib.md` S1,
  Deviation 2, records this as the one launchd probe it could not move.
* **F7. The existing tests.** `launchd_test.go:59`,
  `TestProbeUsesListWhenItSees`, covers `probe`'s `list` and `print`
  choice through `Running` and `probe`; `:92`, `TestInstalledAndRunning`,
  covers a missing plist and a job that is not loaded. The fake runner
  (`fake_test.go:78`) answers `list` and `print` with exit 113 when its
  `loaded` is false.
* **F8. API policy.** Within v1 every exported change is an addition,
  and `make apicheck` checks it against the newest `v1.*` tag, `v1.17.2`
  (0025-MADR D6). An exported result type has named fields with JSON
  tags, as `PlistBackup` (`json_test.go:11`) and `service.ExecState` do.

## Decision Drivers

* Additive only: `make apicheck` stays compatible with `v1.17.2`.
* One way to get the recommended client, so a consumer cannot drift
  from it, and the library's own examples use it.
* Keep `GitHubOptions`' "every field is explicit" contract, which 0025
  decided.
* `launchd` reports a state with `probe`'s rules, so that a consumer can
  drop its own `launchctl print`.
* Exported results are fields with JSON tags, and the API may be
  speculative where it is coherent and cheap.

## Considered Options

### D1: how the recommended client is exported (issue #5)

* **D1-A.** `func DefaultHTTPClient() *http.Client` in `selfupdate`. Each
  call returns a new client with a new `*http.Transport`, built as
  `defaultHTTPClient` is now, which it replaces. `NewGitHubUpdater` calls
  it. The doc says the transport is a `*http.Transport` the caller owns
  and may adjust.
* **D1-B.** D1-A, plus `func NewHTTPClient(HTTPClientOptions)
  *http.Client` with a `ResponseHeaderTimeout` field (zero meaning 30 s),
  `DefaultHTTPClient` being `NewHTTPClient(HTTPClientOptions{})`.
* **D1-C.** No API. Document the recipe beside `GitHubOptions.Client`.

### D2: what a nil `GitHubOptions.Client` means (issue #5's first question)

* **D2-A.** It stays an error. The field's doc and the error name
  `DefaultHTTPClient`: "selfupdate: github client is required; use
  selfupdate.DefaultHTTPClient()".
* **D2-B.** Nil means `DefaultHTTPClient()`.

### D3: the library's examples (F3)

* **D3-A.** The migration guide's worked command and
  `selfupdate/example_test.go` use `DefaultHTTPClient()`, the guide's
  `updateTimeout` constant goes, and its comment says the context bounds
  the run.
* **D3-B.** Leave them; the doc comment is enough.

### D4: how `launchd` exports "loaded" (issue #4 and its question)

* **D4-A.** `func (j *Job) Loaded(ctx context.Context, product string)
  (bool, error)`, as the issue proposes: `probe`'s `loaded`, with
  `Running`'s arguments and errors.
* **D4-B.** `func (j *Job) Status(ctx context.Context, product string)
  (Status, error)` only, returning every state from one probe.
* **D4-C.** Both: D4-A, and D4-B with this type:

  ```go
  // Status is a launchd job's state at one moment.
  type Status struct {
      Installed bool `json:"installed"` // the plist exists
      Loaded    bool `json:"loaded"`    // launchd knows the job in its domain
      Enabled   bool `json:"enabled"`   // as Enabled reports it
      Running   bool `json:"running"`   // the job has a running process
      PID       int  `json:"pid"`       // its process ID; 0 when not running
  }
  ```

  `Status` stats the plist, runs `probe` once, and runs `Enabled`'s
  checks only when the plist exists (`Enabled` is false, not an error,
  when it does not). `probe` runs whatever the plist's state, since a
  loaded job can outlive its plist. Any error is returned with a zero
  `Status`.

## Decision Outcome

Proposed, for the owner to decide: "D1-A", "D2-A", "D3-A" and "D4-C",
released together as `v1.18.0`, a minor release because both add API.

* **D1-A** is the issue's proposal, named after `DefaultLimits`, and it
  removes the copy-or-misuse choice. A fresh transport per call is
  already tunable: a caller that wants another header timeout sets it on
  the returned `*http.Transport`. D1-B adds a second constructor for the
  one field a caller can already set, and stays available as an addition
  if a second knob appears. D1-C leaves each consumer a copy to drift.
* **D2-A** keeps 0025's decision that `GitHubOptions` defaults nothing
  (F2), and turns the error into the pointer to the fix. D2-B is 0025's
  rejected D5-B for a different field: a source built with no client
  would succeed silently, and `NewGitHubUpdater` remains the defaulting
  path.
* **D3-A** removes the trap at its source (F3); the consumer copied the
  guide. D3-B leaves the guide contradicting the field's doc.
* **D4-C** gives the issue's `Loaded`, in the shape of `Installed`,
  `Enabled` and `Running`, and also answers its open question: a status
  command takes one snapshot, with one launchd probe and exported,
  JSON-tagged fields (F8). D4-A alone leaves the double probe (F5); D4-B
  alone breaks the per-state pattern every backend has.

### Consequences

* Good, because a program that composes `New` gets the recommended
  client in one call, and the guide no longer teaches `Client.Timeout`.
* Good, because a launchd status command can drop its own `launchctl
  print`, and gets `probe`'s `list`-or-`print` choice and its exit-113
  rule (F6).
* Neutral, because `NewGitHubSource`'s nil-client error text changes; it
  is not a sentinel, and no test outside this module can match it
  reliably.
* Neutral, because `Status` is launchd's alone. A cross-backend status
  type is not decided here.
* Bad, because `Status` and `Loaded` overlap with `Running`, so there
  are two ways to read the same state.

### Confirmation

* `make apicheck` is compatible with `v1.17.2`, and reports the added
  identifiers only.
* `DefaultHTTPClient`'s test: no `Client.Timeout`, a 30 s
  `ResponseHeaderTimeout`, Go's TLS-handshake timeout, a proxy function,
  and two calls returning different clients and different transports,
  neither of them `http.DefaultTransport`.
* `NewGitHubSource` with no client fails, naming `DefaultHTTPClient`.
* `Loaded`'s tests cover every `probe` branch: `list` sees the domain and
  the job is loaded; `list` exits 113; `print` reports loaded; `print`
  exits 113; another exit is an error.
* `Status`'s tests: every field for a running job; a loaded job with no
  process; a job not loaded; a missing plist, with `Enabled` false and no
  `plutil` call; an error from `probe`; one `list` or `print` per call;
  and its JSON keys pinned.
* Each new test is seen failing on a `scripts/plant-copy.sh` copy.
* `make gate` ends `overall=0`; the `v1.18.0` tag's CI passes.

## Pros and Cons of the Options

### D1-A. `DefaultHTTPClient()`

* Good, because it is one function, and the client it returns is the
  library's own.
* Neutral, because tuning means a type assertion on `Transport`.

### D1-B. `NewHTTPClient(HTTPClientOptions)` as well

* Good, because a tuned client needs no type assertion.
* Bad, because its one option duplicates a field the caller already
  owns.

### D1-C. A documented recipe

* Bad, because every consumer keeps a copy that drifts.

### D2-A. Required, with the pointer

* Good, because it keeps 0025's explicit-options contract.
* Neutral, because a caller still writes one more line than with D2-B.

### D2-B. Nil means the default

* Good, because the trap goes for every caller.
* Bad, because it reverses 0025 D5 for one field of a type documented as
  having no defaults.

### D3-A. Fix the examples

* Good, because the library stops teaching the trap.

### D3-B. Leave them

* Bad, because the guide and the field's doc disagree.

### D4-A. `Loaded` only

* Good, because it is the issue's exact ask.
* Bad, because a status command still probes launchd twice.

### D4-B. `Status` only

* Good, because it is one call.
* Bad, because the other states are per-method on every backend.

### D4-C. Both

* Good, because it serves the per-state caller and the status command.
* Bad, because the API has two ways to read `loaded` and `running`.

## More Information

* Issues #5 and #4 of this repository. Each is closed when `v1.18.0`
  ships, on the owner's ask.
* [0025-MADR-align-with-go-sdk-design-practices.md](0025-MADR-align-with-go-sdk-design-practices.md)
  D5 (the defaulting path, and the rejected D5-B) and D6 (additive API).
* [0011-MADR-reference-service-lifecycles.md](0011-MADR-reference-service-lifecycles.md)
  §7 decided `launchd`'s `list`-or-`print` probe that `Loaded` and
  `Status` expose.
* The consumer's records are in another repository: magic-cli-remote,
  `docs/decisions/0054-PLAN-standardize-self-update-on-go-selfupdate-lib.md`
  S1, Deviation 2.
* Out of scope: a status snapshot for `systemd`, `scm` or
  `taskscheduler`, or one interface over all four; a record of its own
  may add them.
