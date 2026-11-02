---
status: proposed
date: 2026-11-02
decision-makers: go-selfupdate-lib maintainers
consulted: "the records' open items, read at 4a9ead5 on 2026-11-01; selfupdate/service, selfupdate/service/taskscheduler and selfupdate/verify/ghattest at 4a9ead5; probes E1, G1 and G2 on scratch copies of 4a9ead5; probe S1, read-only GETs of this repository's settings through the GitHub API, and probe S2, on 2026-11-01; GitHub's documentation on rulesets, private vulnerability reporting, Dependabot alerts and Actions settings, read 2026-11-01; the D2 to D4 citations re-read at 4a9ead5, and the D2 and D3 PLANs' code pre-checked on scratch copies of 4a9ead5, on 2026-11-01; the D2 PLAN's internal/redact form pre-checked the same way on 2026-11-02"
informed: the fleet's programs, which require this module and call its workflows
---
<!-- markdownlint-disable MD004 MD013 MD024 MD060 -->

# Apply the deferred repository settings, redact `Env` in four formatted types, test Task Scheduler's stop when its bound ends within the grace, and record the host cleanup in 0038

## Context and Problem Statement

After `v1.23.1` an assessment of every record found no plan in progress and no open issue. It found items that earlier records set aside, deferred or left stale. The owner chose four to specify on 2026-11-01:

1. **The repository settings** that [0025-MADR-align-with-go-sdk-design-practices.md](0025-MADR-align-with-go-sdk-design-practices.md) D9-A recommended and Amendment A5 deferred.
2. **A redacting formatter for the `Env` fields.** [0025-PLAN-align-with-go-sdk-design-practices.md](0025-PLAN-align-with-go-sdk-design-practices.md) left two out of scope (`:52-54`) and recorded them as "Seen and left" (`:1414-1415`).
3. **A test for Task Scheduler's stop when its bound ends within the `RequestStop` grace.** [0037-PLAN-honour-contexts-in-backend-fakes-and-stop-commands.md](0037-PLAN-honour-contexts-in-backend-fakes-and-stop-commands.md) left that path untested (`:191`, `:234`).
4. **A note in [0038-PLAN-pinned-plans-lifecycle-conformance-and-bubble-tests.md](0038-PLAN-pinned-plans-lifecycle-conformance-and-bubble-tests.md).** It still says host leftovers "stay on the hosts" (`:920-923`), though they were removed on 2026-10-30.

The four are independent. This record decides each, D1 to D4. Each gets its own PLAN (More Information).

Amendment A1 (2026-11-02), at the end of this record, adds a fifth, D5. It makes the architecture table check require a row for every directory Go builds, a gap the D2 PLAN's pre-check found. D2's verification relies on it. The slug keeps "four", since a record is never renamed.

Facts are tagged by their source:

* **[read]:** the tree at `4a9ead5`.
* **[probed]:** a probe run for this record; each probe is described under its decision.
* **[docs]:** GitHub's documentation, quoted.
* **[derived]:** follows from the above.
* **[not established]:** stated as unknown.

### D1 facts: the repository settings

* **F1.1 [read]. What 0025 recommended.** D9-A (`0025-MADR:479-487`) recommended five settings to the owner, "who alone can change them":
  * private vulnerability reporting on;
  * Dependabot alerts on;
  * a tag ruleset that limits creating `v*` tags to the owner;
  * a rule on `main` that blocks force-push and deletion and requires no pull request;
  * Dependabot security-update pull requests left off.
* **F1.2 [read]. What was prepared, and the deferral.** 0025-PLAN S14 step 3 (`:1074-1110`) prepared three of the five as API calls for the owner to run:
  * private vulnerability reporting;
  * Dependabot alerts;
  * one tag ruleset on `refs/tags/v*`, with `creation`, `update` and `deletion` rules and bypass actor `RepositoryRole` 5, "[unverified]" to be the admin role.

  It did not prepare the `main` rule. The owner chose to "keep them off for now" (0025-PLAN Deviation D6, `:2293-2309`), and 0025-MADR Amendment A5 (`:794-802`) records it.
* **F1.3 [read]. What the documents say now.**
  * `SECURITY.md:5-8` says private reporting "is not yet turned on", and names the owner's profile contact until it is.
  * `docs/architecture.md:752-755`'s trust model tells a publisher to "Restrict `v*` tag creation with a tag ruleset, and keep two-factor authentication on".
  * `docs/guides/building-releases.md:256-262` gives callers the same advice.
  * A5 says D9-A's claim, that the tag ruleset closes the repository-compromise threat, "does not hold until the owner applies it".
* **F1.4 [probed]. The settings as they stand.** Probe S1, on 2026-11-01, made GET requests only, through `gh api` with no method, field or input flag. It found:

  | Setting | State |
  | :--- | :--- |
  | owner, visibility, default branch | a user account; public; `main` |
  | collaborators | one, with admin |
  | private vulnerability reporting | `{"enabled": false}` |
  | Dependabot alerts (`vulnerability-alerts`) | 404, "Vulnerability alerts are disabled." |
  | Dependabot security updates | `{"enabled": false, "paused": false}` |
  | secret scanning; push protection | enabled; enabled |
  | secret scanning's non-provider patterns; validity checks | disabled; disabled |
  | rulesets | 0 |
  | `main` | "Branch not protected"; 0 rules apply |
  | deploy keys; webhooks; environments | 0; 0; 0 |
  | releases; `v*` tags | 0; 39 |
  | immutable releases | `{"enabled": false, "enforced_by_owner": false}` |
  | Actions | enabled; `allowed_actions` `all`; `sha_pinning_required` false |
  | Actions' default workflow permissions | `read`; workflows cannot approve pull requests |

* **F1.5 [not established]. Two things the token cannot read** (probe S2, GET only):
  * the account's two-factor flag, which came back empty for the token's scopes;
  * the GitHub Apps installed with access to this repository. Listing installations needs a token "authorized to a GitHub App" (HTTP 403).

  So no App's `contents` permission on this repository is known.
* **F1.6 [read]. No workflow here can write a ref.**
  * `ci.yml:21-22` and `docs.yml:20-21` hold `contents: read`.
  * `build-selfupdate-release.yml` and `publish-selfupdate-release.yml` trigger on `workflow_call` alone (`:10-11` each), so they run in a caller's repository, with the caller's token.
  * The publish workflow's `contents: write` (`:64-67`) is that caller's.
* **F1.7 [read]. Every external action is pinned to a 40-hex commit.** The only `uses:` lines without one are `ci.yml:395` and `:403`, which call the local `./.github/workflows/build-selfupdate-release.yml`.
* **F1.8 [read]. Tags here never move, and this module publishes no GitHub release.**
  * [0004-PLAN-v1-3-1-ocp-login-pattern-rewording.md](0004-PLAN-v1-3-1-ocp-login-pattern-rewording.md) (`:53`) puts "Rewriting history, or moving any tag" out of scope, under 0004-MADR Amendment P1.
  * [0030-MADR-publish-github-release-on-tag.md](0030-MADR-publish-github-release-on-tag.md) was rejected: "This module publishes no GitHub releases" (`:103-105`).
  * So a `v*` tag here makes a module version, which the module proxy serves and the checksum database records.
  * It also names the commit a caller resolves when it pins this repository's workflows: `building-releases.md` resolves the pin with `git ls-remote … 'refs/tags/vX^{}'`.
* **F1.9 [docs]. Rulesets.**
  * **Availability:** "Rulesets are available in public repositories with {% data variables.product.prodname_free_user %} and {% data variables.product.prodname_free_team %} for organizations, and in public and private repositories with {% data variables.product.prodname_pro %}, …". This is the docs' gated-features text, `data/reusables/gated-features/repo-rules.md` in `github/docs`, which the About rulesets page shows as its `product` line. A public repository of a user account on the free plan is the first case.
  * **Targets:** "You can use rulesets to target branches or tags", with "`fnmatch` syntax", in which "the `*` wildcard does not match directory separators (`/`)" (About rulesets; Creating rulesets).
  * **Rules:**
    * "Restrict creations: … only users with bypass permissions can create branches or tags whose name matches the pattern";
    * "Restrict updates: … only users with bypass permissions can push to branches or tags whose name matches";
    * "Restrict deletions: … only users with bypass permissions can delete …";
    * "Block force pushes: You can prevent users from force pushing to the targeted branches or tags" (Available rules).
  * **Bypass actors:** "Repository admins, organization owners, and enterprise owners", the write roles, teams, "GitHub Apps" and "Dependabot" may be granted bypass (Creating rulesets).
  * **Bypass modes:** the REST field `bypass_mode` is `always`, `pull_request` or `exempt`, and "pull_request is only applicable to branch rulesets".
  * **Enforcement:** "evaluate is only available with GitHub Enterprise" (REST, Create a repository ruleset).
  * **Layering:** "if multiple rulesets target the same branch or tag … the rules in each of these rulesets are aggregated. If the same rule is defined in different ways … the most restrictive version of the rule applies" (About rulesets).
  * **Not stated in the pages read:**
    * the REST `actor_id` of the admin role. GitHub's own provisioning script in `github/docs` (`src/ghes-releases/scripts/deprecate/create-docs-ghes-version-repo.sh:54`) uses `{ "actor_id": 5, "actor_type": "RepositoryRole", "bypass_mode": "always" }` without labelling it;
    * a tag example of `conditions.ref_name.include`.
* **F1.10 [docs]. The other settings.**
  * **Private vulnerability reporting:** "Owners and administrators of public repositories can allow security researchers to report vulnerabilities securely … by enabling private vulnerability reporting"; researchers then "see a Report a vulnerability button on the repository's 'Advisories' page".
  * **Dependabot alerts:** `PUT /repos/{owner}/{repo}/vulnerability-alerts` "Enables dependency alerts and the dependency graph", with admin access.
  * **Actions SHA pinning:** the setting requires "all actions must be pinned to a full-length commit SHA … Reusable workflows can still be referenced by tag". The pages read do not say whether a local `./` reference is exempt.
* **F1.11 [derived]. What a tag ruleset changes here.** With one admin collaborator (F1.4) and read-only workflow tokens (F1.6), a `v*` creation rule with an admin bypass refuses:
  * a GitHub App with `contents` write, of which none is known and none can be ruled out (F1.5);
  * a deploy key added later;
  * a collaborator with write access added later.

  It does not refuse anything that acts with the owner's own credentials, since those are the admin's. That is two-factor authentication's and short-lived tokens' work (0004-MADR `:825-830`). Rules on `update` and `deletion` keep the 39 existing tags and every later one where F1.8 says they stay.
* **F1.12 [read]. The owner's word.** On 2026-11-01 the owner stated that nothing is done to this repository's GitHub settings without their approval. Probe S1's reads were made before that statement. No call to GitHub, read or write, is part of this record's decision without the owner's word in that turn.

### D2 facts: `Env` when a value is formatted

* **F2.1 [read]. Four exported option types hold an environment as `Env []string`:**
  * `service.Command` (`selfupdate/service/runner.go:22-24`);
  * `service.HandOff` (`handoff.go:48-50`);
  * `service.ExecOptions` (`execreconciler.go:76-77`);
  * `ghattest.Options` (`selfupdate/verify/ghattest/ghattest.go:76-79`).

  0025-PLAN named the first and the last.
* **F2.2 [read]. What they hold.**
  * `ExecReconciler` runs its child with `passOn(o.Env)` (`execreconciler.go:116`, `:165`, `:175-176`): this process's whole environment, scrubbed of service-manager variables, then `ExecOptions.Env` (`managerenv.go:84-87`).
  * `ghattest` runs `gh` with `Options.Env`, or `os.Environ()` when that is nil, then its own settings (`ghattest.go:273-279`). Its doc says `gh` "finds its login through HOME, GH_CONFIG_DIR or GH_TOKEN" (`:76-78`).
  * `systemd`, `launchd` and `codesign` build fixed environments (`systemd/unit.go:134-142`, `launchd/job.go:212`, `codesign/codesign.go:23`).

  So a `Command` that `ExecReconciler` or `ghattest` builds can carry any secret in this process's environment, `GH_TOKEN` among them.
* **F2.3 [read]. Where a `Command` goes.** Each is passed to a `service.Runner`, and a program can supply its own through:
  * `ExecOptions.Runner`;
  * `ghattest.Options.Runner`;
  * the backends' `Options.Runner`.

  `ExecRunner`'s errors name only the program's base name (`runner.go:76`, `:88`, `:95`), or quote a `Path` that is not absolute (`:69`). None prints `Env`.
* **F2.4 [read]. The library never formats these values itself.**
  * A scan of every non-test `Errorf`, `Sprintf`, `Printf`, `Fprintf`, `Sprint` and `Println` call under `selfupdate`, `buildinfo` and `internal`, for an argument named `spec`, `o`, `c`, `opts`, `cmd`, `command`, `options`, `h` or `ho`, found four calls: `service/poll.go:109` (a `Health`), `taskscheduler/api.go:49` (an `hresult`) and `systemd/name.go:62`, `:66` (a byte). None is one of the four types.
  * The only non-test files that import `log/slog` are `credentials.go`, `slogreporter.go` and `github.go`. `NewSlogReporter` logs an event's fields and detail.

  The exposure is a program's own formatting: a `Runner` that logs its `Command`, or a log line holding an option value.
* **F2.5 [probed]. Each type prints a secret today.** Probe E1 ran a test in a scratch copy of `4a9ead5`, with `Env: []string{"GH_TOKEN=fixture-secret-0123"}`, through `%v`, `%+v`, `%#v`, `%s` and `%q`, and through log/slog's text and JSON handlers:

  | Type | `Stringer`, `Formatter`, `LogValuer` | Printed the secret |
  | :--- | :--- | :--- |
  | `service.Command` | none | 7 of 7 |
  | `service.HandOff` | none | 7 of 7 |
  | `service.ExecOptions` | none | 6 of 7 |
  | `ghattest.Options` | none | 7 of 7 |

  * `%+v` of the `Command` printed `{Path:/usr/bin/gh Args:[attestation] Env:[GH_TOKEN=fixture-secret-0123] Stdin:[]}`.
  * slog's JSON handler printed `"v":"!ERROR:json: unsupported type: func(context.Context, string, service.Receipt) error"` for `ExecOptions`: `encoding/json` refuses its `RestoreFunc` field, so the secret did not show.
* **F2.6 [read]. The pattern already in use.** 0025-MADR D1-A gave `Credential` and `GitHubOptions` `String`, `GoString`, `Format` and `LogValue`, and gave `*GitHubSource` `String`, `Format` and `LogValue` (`selfupdate/credentials.go:24-65`, `github.go:828-890`).
  * `Format` prints `String` for every verb, "so no directive reaches" the secret.
  * `encoding/json` is left alone.
  * Amendment A3 (`0025-MADR:767-777`) records what that leaves: slog's JSON handler encodes a value without `LogValue` with `encoding/json`, so a secret holder nested in a struct still prints through it.
  * `selfupdate/redact_test.go` tests every verb and both handlers.
  * 0025-PLAN S2 step 4 recorded `make apicheck` "compatible with v1.15.2" after adding those methods.
* **F2.7 [read]. Nothing blocks the methods.**
  * No struct embeds the four types, so their methods promote nowhere.
  * No test formats one: the `%v`, `%+v` and `%#v` calls in test files are on other types.
  * The library encodes none of the four with `encoding/json`. The `json.Marshal(vars)` at `handoffenv.go:62` encodes a `[]string` into the private handoff file, by design.
  * `selfupdate/cli` takes no token flag, so `HandOff.Args`, "such as os.Args[1:]" (`handoff.go:46`), holds none of the library's own.

### D3 facts: Task Scheduler's stop when its bound ends within the grace

* **F3.1 [read]. The path.**
  * `Task.Stop` bounds its context by `Options.Poll`'s timeout before anything is stopped, on `context.WithoutCancel(ctx)`, so the caller's deadline does not shorten it (`taskscheduler/lifecycle.go:153-156`, `task.go:245-251`).
  * It calls `Options.RequestStop` with that bounded context (`lifecycle.go:193`), then waits up to `StopGrace` (`:194`).
  * When the wait ends because the context's deadline passed, `Stop` returns `stillRuns(path)`, `service.ErrTimeout`, without ending the task (`:197-200`).
  * The deferred restore then enables the task again, because the program still runs (`:186-190`).
* **F3.2 [read]. Why 0037 left it untested.** 0037-PLAN N5 (`:191`) says: "That path is not covered by a test: it needs the reads before the grace to spend the timeout, and `New` refuses a grace that is not shorter than it". The refusal is `task.go:175-176`.
* **F3.3 [read]. What 0037 missed.** `RequestStop` runs on the bounded context and is the program's own function (F3.1). Its run is spent inside `Poll`'s timeout, so a `RequestStop` that takes most of the timeout leaves the grace to outlast it.
  * `Options.StopGrace`'s doc says it "is spent inside Poll's timeout" (`task.go:76-81`).
  * `Options.RequestStop`'s doc says nothing of its own time (`:69-75`).
  * The tests already run in synctest bubbles, where `time.Sleep` advances the bubble's clock (`lifecycle_test.go:9`; `fake_test.go:238-243`).
* **F3.4 [probed]. Nothing reaches it today.** Probe G2 ran `go test -coverprofile` on a scratch copy of `4a9ead5`, with go1.27.3 on darwin/arm64.
  * The package reports 89.0% of statements covered.
  * In `Stop`'s tail, every block has at least one hit except `lifecycle.go:199.5-200.1`, the `return stillRuns(path)` of F3.1, which has 0.
  * The live tests do not reach it either: their `RequestStop` writes a file and returns at once, with a grace of 5 s or 15 s inside a 30 s or 90 s timeout (`live_windows_test.go:358-362`, `:905-908`, `:1244-1247`).
* **F3.5 [probed]. A fake test reaches it.** Probe G1 added one test to a scratch copy of `4a9ead5`, inside `synctest.Test`:
  * a task whose program never exits (`exitIn = -1`);
  * `Poll.Timeout` of 300 ms and a `StopGrace` of 100 ms;
  * a `RequestStop` that sleeps 250 ms and returns nil.

  | Copy | Result |
  | :--- | :--- |
  | HEAD | PASS. `Stop = selfupdate: service: timed out: \demo still runs after it was stopped`; took 300ms; calls `["enable false" "enable true"]`; enabled `true`; program alive `true`; recorded as disabled `false` |
  | `:197-200` planted out | FAIL. `Stop = <nil>`; calls `["enable false" "stop"]`; enabled `false`; alive `false`; recorded as disabled `true` |

  The planted copy ends the task, and the fake ends it at once. That is what D3-B below would do.

### D4 facts: the record of the host leftovers

* **F4.1 [read]. What 0038-PLAN says.** Its closing "Not done here" (`:916-925`) says leftovers "this plan's runs cannot be shown to have made stay on the hosts":
  * on the Windows test host's temporary directory: 509 empty `selfupdate-artifact-*` directories, four test directories and 34 `sulib-*` remote-run copies;
  * one `selfupdate-live-run-*` directory in the WSL;
  * nine fixture directories in this Mac's temporary directory.

  The plan is `complete`, committed as `833f49f`.
* **F4.2 [read, from the session's scratch record]. What happened after.** On 2026-10-30 at 12:52 UTC, seven minutes after `833f49f` (07:45 −0500), the owner said "proceed to item 1, cleanup."
  * A look-only inventory first recounted each set: 509 artifact directories, none of them non-empty; 34 copies, 242 MB in all, the newest from 2026-10-24; the four test directories; the WSL directory, which held a test binary and its run script from 2026-10-04; and 9 entries on this Mac.
  * The removal stopped on any difference. It first checked again the counts of 509 artifact directories and 34 copies, that the WSL directory's two files were still there, and the count of nine on this Mac. Then it removed:
    * the artifact directories with `rmdir`, so only empty ones could go;
    * the copies, and the four test directories by name;
    * the WSL directory;
    * the nine Mac entries.
  * Every step exited 0, and the counts after were 0 on each host.

  No record says so. The scratch record names hosts and accounts, so it is described here, not quoted.
* **F4.3 [read]. The rules for a correction.** A plan "keeps an execution record", and history is not silently rewritten: an original line is struck through or annotated (the `madr-and-plan-writing` skill; `AGENTS.md`, "MADR and PLAN before mutating work"). A record names no hostname, account or real-machine path (`AGENTS.md`, "Identifiers").

## Decision Drivers

* **The owner alone changes GitHub settings** (F1.12, F1.1). An agent prepares them and reads them back only on the owner's word.
* **No new requirement, and no exported change without its release.** An added method is an addition under `apidiff`, so a minor release. Every other change here is a test, a comment or a document.
* **Proof first.** Every new test or check is seen failing on a planted copy before it lands. Every decision that rests on GitHub's behaviour is read back after it is applied.
* **One idiom.** A new redaction follows 0025-MADR D1-A, so a reader meets the same `String`, `GoString`, `Format` and `LogValue` everywhere.
* **Records that match the hosts and the code.** A reader of 0038 or 0037 is not misled about what remains.
* **No identifier in a committed file** (F4.3).

## Considered Options

### D1: the repository settings (F1.1 to F1.12)

* **D1-A.** D9-A as written, applied by the owner:
  * private vulnerability reporting on;
  * Dependabot alerts on;
  * one tag ruleset on `v*` restricting creations, updates and deletions, with the repository admin role as an `always` bypass;
  * one branch ruleset on `main` with "Restrict deletions" and "Block force pushes", no other rule and no bypass;
  * Dependabot security updates left off.

  `SECURITY.md` and 0025-MADR (a new amendment) then say what is on.
* **D1-B.** As D1-A, but the tag rules split into two rulesets:
  * "release tags: create", restricting creations, with the admin role as an `always` bypass;
  * "release tags: keep", restricting updates and deletions and blocking force pushes, with no bypass.

  An existing tag can then not be moved or deleted by anyone, the owner included, until the owner disables "keep", which the audit log records.
* **D1-C.** Private vulnerability reporting and Dependabot alerts only; no ruleset.
* **D1-D.** Keep A5's deferral, and leave the documents as they are.
* **D1-E.** Any of the above, plus "Require actions to be pinned to a full-length commit SHA" (F1.7, F1.10).

### D2: `Env` in the four types (F2.1 to F2.7)

* **D2-A.** Each of `service.Command`, `service.HandOff`, `service.ExecOptions` and `ghattest.Options` gains, on a value receiver, `String`, `GoString`, `Format` (every verb prints `String`) and `LogValue`, as 0025-MADR D1-A:
  * **`Env`:** each entry prints as its name and `=<redacted>`, or `=<empty>` for an empty value, as `GitHubOptions.Token` prints `<empty>`. An entry with no `=` prints as `<redacted>`. A nil `Env` prints as `<nil>`, since it means "this process's environment" for `Command` and `ghattest.Options`, and an empty one as `[]`.
  * **Function and interface fields** (`RestoreFunc`, `Runner`): `<set>` or `<nil>`.
  * **`Command.Stdin`:** its length in bytes, or `<nil>`.
  * **Every other field** prints as it is, strings quoted: paths, arguments, IDs, the `ghattest` policy, the timeout.

  `encoding/json` is left alone, and each type's doc says what Amendment A3 says for `Credential`. A minor release, `v1.24.0`.
* **D2-B.** As D2-A, but redact only entries whose name matches a pattern such as `TOKEN`, `SECRET`, `PASSWORD` or `KEY`.
* **D2-C.** An exported `service.RedactEnv([]string) []string` and a doc sentence on each field; no formatter.
* **D2-D.** A named type, `service.Env []string`, with the formatter, as each field's type.
* **D2-E.** A doc sentence on each field ("do not log this"), and nothing else.

### D3: the grace path (F3.1 to F3.5)

* **D3-A.** Pin today's behaviour with a fake test in a synctest bubble, whose `RequestStop` spends most of `Poll`'s timeout (probe G1's shape). The test asserts:
  * `Stop` is `ErrTimeout` saying the task still runs;
  * the calls are the disable and the re-enable, with no `IRegisteredTask::Stop`;
  * the program still runs, and the task is enabled and not recorded as disabled;
  * `Stop` took exactly the bound.

  `Options.RequestStop`'s doc gains that its run is spent inside `Poll`'s timeout, as `StopGrace`'s is. Two lines of 0037-PLAN are annotated, not rewritten. `:191`'s reason is struck through and corrected (F3.3), and `:234` gains a pointer here. No release: a test and a comment.
* **D3-B.** Change the behaviour: when the bound ends within the grace, end the task anyway (`IRegisteredTask::Stop`), then report `ErrTimeout` only if the program has not gone. Test it as in D3-A, with the new expectations. A patch release.
* **D3-C.** Reach the path through a slow fake read, such as `instances`, instead of `RequestStop`.
* **D3-D.** Leave it untested; correct 0037-PLAN's reason only.

### D4: the record (F4.1 to F4.3)

* **D4-A.** Append a dated entry to 0038-PLAN's execution record, after "The plan is complete.":
  * what was removed, by kind and count, from each host;
  * the owner's word, the re-check before each removal, and the counts after;
  * that it was done after the plan closed, outside its phases.

  The "stay on the hosts" bullet (`:920-923`) is annotated "removed 2026-10-30, see below", not rewritten. The plan stays `complete`.
* **D4-B.** A `0040-REPORT-…` holding the same, with 0038 untouched.
* **D4-C.** Leave 0038 as it is.

## Decision Outcome

Proposed: "D1-B, D2-A, D3-A and D4-A".

**The owner's answers (2026-11-02).** Asked with three options each, the owner chose:

* **"Accept D2-A, D3-A, D4-A".** Chosen options: "D2-A", "D3-A" and "D4-A". D1 is not decided: D1-B stays proposed until its PLAN is asked for. The record keeps `status: proposed` because its front matter takes one status and D1 is open. It becomes `accepted` when D1 is decided.
* **The helpers in a new `internal/redact` package,** in place of the recommended copy of them into `ghattest`. D2-A's formatters share one copy there, which no program can import, under its own depguard rule (D2-A below).
* **The order "Records, then D4, D3, D2"**, and **the host tests as each PLAN writes them.**

* **D1-B.**
  * **Each setting answers a fact here.**
    * Private vulnerability reporting gives `SECURITY.md` the channel it names (F1.3).
    * Dependabot alerts watch the three modules this module requires.
    * The creation rule closes the paths F1.11 lists, as D9-A meant.
    * The `main` rule keeps the owner's direct commits working (F1.9 lists no rule that refuses a push here) and refuses a force push or a deletion, which the records already never do.
  * **Why "keep" is a ruleset of its own.**
    * A bypass covers a whole ruleset (F1.9), so one ruleset cannot let the admin create a tag and refuse the admin moving one.
    * Tags never move here (F1.8). A moved tag would change the commit a caller resolves for a workflow pin, and would contradict the checksum database's record of the version.
    * Two rulesets cost one more line in the settings.
  * **What D1-B does not prove.** It does not stop a thief of the owner's credentials (F1.11); the trust model already says so, and keeps its advice on two-factor authentication.
  * **Rejected.**
    * D1-A leaves tags movable by the admin.
    * D1-C leaves the threat A5 names open.
    * D1-D keeps a `SECURITY.md` that names a channel that does not exist.
  * **D1-E waits.** Every external action is already pinned (F1.7). Whether the setting accepts `ci.yml`'s local workflow calls is not established (F1.10), and a wrong guess fails every CI run. A record of its own can take it up with a probe.
* **D2-A.**
  * **Why.**
    * It closes the four types that print a secret today (F2.5) with the idiom 0025 already uses (F2.6).
    * Keeping the names keeps the output useful for a diagnosis.
    * Printing `Stdin` as a length keeps a receipt body out of a log line.
  * **Rejected.**
    * D2-B leaks whatever its pattern does not name, such as a password inside a `DATABASE_URL`.
    * D2-C protects only a program that calls it.
    * D2-D changes four fields' types, which `apidiff` reports as incompatible, and that is a v2 change (0025-MADR "Out of scope").
    * D2-E is D1-C's "a documented leak is still a leak".
  * **Where the helpers live (the owner's answer, 2026-11-02).** `service`'s and `ghattest`'s formatters share `internal/redact`: `Env`, `EnvText`, `EnvAttr`, `Quoted`, `Length`, `SetOrNil` and the constant `Nil`. A copy per package was the smaller change. The owner chose one copy, which a later formatter can import too.
    * As `AGENTS.md` ("Dependencies") requires of a new package, it gets its own depguard rule, `internal-redact` (the standard library only). `service` and `selfupdate-verify-ghattest` each gain an allow for it, and `other-packages` excludes it.
    * `AGENTS.md` and `docs/architecture.md` name it.
    * `internal/` keeps it out of the exported API, so `apidiff` does not report it.
    * `selfupdate`'s own `redacted` and `setOrNil` (`credentials.go:64-77`) stay where they are.
  * **Release.** The methods are exported additions, so `v1.24.0`. The D2 PLAN's pre-check bears this out: `apidiff` from `v1.23.1` to a prototype of D2-A lists the 16 methods as "added", and nothing as incompatible. Its changelog and the migration guide name the changed `fmt` output, as `v1.16.0`'s did.
* **D3-A.**
  * **Why.**
    * It covers the one block of `Stop` no test reaches (F3.4), by the route F3.3 found, and probe G1 shows the test failing when the path is planted out (F3.5).
    * The doc sentence states a bound a program can already hit.
  * **Rejected.**
    * D3-B changes what 0037-PLAN N5 deliberately kept ("as before") and needs a decision of its own, with a release. This record pins the behaviour, so such a decision changes a test that states it.
    * D3-C reaches the path through an internal that a program cannot influence.
    * D3-D leaves the gap.
* **D4-A.**
  * **Why.** A reader of 0038 meets the correction where the stale line is, and the history stays as written (F4.3).
  * **Rejected.**
    * D4-B puts it where a reader of 0038 would not look.
    * D4-C leaves a false statement.

### Consequences

* Good, because private vulnerability reporting gives a reporter a channel inside GitHub, and `SECURITY.md` names it.
* Good, because no App, deploy key or later collaborator can create a `v*` tag, so no one but the owner can make a module version. And no one at all can move a release tag, or force-push or delete `main`, without first turning a ruleset off.
* Good, because a program that logs a `Command`, `HandOff`, `ExecOptions` or `ghattest.Options` through `fmt` or slog's handlers given the value no longer prints a variable's value.
* Good, because every block of Task Scheduler's `Stop` is covered, and `RequestStop`'s cost is documented.
* Good, because 0038 matches the hosts.
* Neutral, because D2-A changes what `fmt` prints for these four types. A program that parsed that output, which no documentation offered, sees a new form.
* Neutral, because the module gains a package, `internal/redact`, with a depguard rule, a row in `docs/architecture.md` and a line in `AGENTS.md`. A program cannot import it.
* Neutral, because `encoding/json`, and slog's JSON handler for a value nested in a struct, still print `Env`. Each type's doc says so, as `Credential`'s does.
* Bad, because Dependabot alerts report a vulnerable module whether or not this module reaches it, where `govulncheck` reports reachability. An alert may need triage that CI's `govulncheck` would not ask for.
* Bad, because deleting or moving a `v*` tag, which no record has ever needed, first needs the owner to disable "release tags: keep".
* Bad, because the new ruleset settings live outside the tree. They are recorded in the D1 PLAN's execution record and read back there, but a later change in the settings page would not show in a diff.
* Bad, because `v1.24.0` is a release whose only exported change is formatting.

### Confirmation

* **D1.**
  * The owner applies each setting. The D1 PLAN gives the settings-page steps, with the matching API calls as an alternative.
  * On the owner's word, read-only GETs read the result back:
    * private vulnerability reporting `{"enabled": true}`;
    * `vulnerability-alerts` 204;
    * `automated-security-fixes` still `{"enabled": false, …}`;
    * the three rulesets as specified;
    * `current_user_can_bypass` on "release tags: create" `always`;
    * no rule on `main` beyond deletion and force push.
  * The read-back also records the admin role's `actor_id` (F1.9). The owner's next ordinary push to `main` is the proof that direct commits still work. `SECURITY.md` and 0025-MADR's amendment land with `make markdownlint`, `make docs-check` and the identifier check clean.
* **D2.**
  * A test in the style of `selfupdate/redact_test.go` formats each type through `%v`, `%+v`, `%#v`, `%s`, `%q`, `%x`, `%X` and `%d`, through slog's text and JSON handlers, and as an element of a slice. No form of the fixture secret may appear, and the variable's name must.
  * Each type's methods are seen failing first: a planted copy renames each type's `Format`, and the test fails, as in 0025-PLAN S2 step 3. Further plants rename `LogValue`, and keep or hide an entry's value. The D2 PLAN lists the nine plants, and its pre-check saw each fail.
  * `make apicheck` reports compatibility with `v1.23.1`. `make gate` is `overall=0`. The tag's CI is green.
* **D3.**
  * The test passes at HEAD and fails with `lifecycle.go:197-200` planted out, as probe G1 did.
  * A coverage profile shows the block covered.
  * `make gate` is `overall=0`.
* **D4.**
  * The entry names no host, account or path. `scripts/check-docs.sh --ids` and `make markdownlint` are clean.
  * The 0038 bullet is annotated, not deleted.

## Pros and Cons of the Options

### D1-A

* Good, because it is D9-A exactly, as the owner reviewed it in 0025.
* Bad, because the admin bypass that creation needs lets the admin move or delete a tag too.

### D1-B

* Good, because creation stays the owner's, and no one moves a tag.
* Neutral, because the owner can still disable "keep", deliberately and in the audit log.
* Bad, because it is one more ruleset to read back and keep.

### D1-C

* Good, because it changes nothing about who may push.
* Bad, because the threat Amendment A5 names stays open.

### D1-D

* Good, because nothing changes.
* Bad, because `SECURITY.md`'s channel and the trust model's advice remain unapplied here.

### D1-E

* Good, because it would enforce what F1.7 shows is already true.
* Bad, because whether it accepts local workflow calls is not established, and the cost of a wrong guess is every CI run.

### D2-A

* Good, because it matches D1-A's formatters, test and documentation.
* Good, because names stay visible for diagnosis.
* Neutral, because `fmt` output changes.
* Bad, because JSON still prints `Env`, as for `Credential`.

### D2-B

* Good, because ordinary variables stay readable.
* Bad, because a pattern misses secrets it does not name.

### D2-C

* Good, because nothing changes for a program that does not call it.
* Bad, because only a program that knows to call it is protected.

### D2-D

* Good, because the redaction would travel with every `[]string` the type holds.
* Bad, because `apidiff` reports a changed field type as incompatible.

### D2-E

* Good, because it costs a sentence.
* Bad, because a documented leak is still a leak.

### D3-A

* Good, because it covers the last untested block of `Stop` with a route a program can take.
* Good, because the doc then states what probe G1 measured.
* Neutral, because it pins a behaviour a later record may change.

### D3-B

* Good, because a hung `RequestStop` would no longer leave the program running past the stop.
* Bad, because it reverses a choice 0037 made, without a decision of its own.

### D3-C

* Good, because it needs no sleeping `RequestStop`.
* Bad, because it tests a timing no program controls.

### D3-D

* Good, because nothing changes.
* Bad, because the gap stays.

### D4-A

* Good, because the correction sits beside the stale line.
* Neutral, because a complete plan gains an entry after its closing.

### D4-B

* Good, because 0038 stays untouched.
* Bad, because a reader of 0038 is not told.

### D4-C

* Bad, because 0038 states what is no longer true.

## More Information

* **Plans.** One per decision, each with its own slug and all sharing this number:
  * `0040-PLAN-apply-repository-settings.md` (D1), not written yet. It follows when the owner asks for it, and no GitHub call, read or write, is made for it without the owner's word in that turn (F1.12);
  * [0040-PLAN-redact-env-in-formatted-values.md](0040-PLAN-redact-env-in-formatted-values.md) (D2);
  * [0040-PLAN-test-task-scheduler-grace-timeout.md](0040-PLAN-test-task-scheduler-grace-timeout.md) (D3);
  * [0040-PLAN-record-host-cleanup-in-0038.md](0040-PLAN-record-host-cleanup-in-0038.md) (D4);
  * [0040-PLAN-require-architecture-table-rows.md](0040-PLAN-require-architecture-table-rows.md) (D5, Amendment A1).

  `make check-records` requires a lone PLAN to copy its MADR's slug (`scripts/check_records.py:121-122`). The three written PLANs are added in one change, so none is alone, and D1's joins them later under its own slug. They are independent. D4, then D3, then D2 with its release, then D1 when the owner chooses to apply it, is one order that keeps each commit small. It also puts D3's doc sentence into `v1.24.0`.
* **The probes**, each on a scratch copy or by GET only. The tree was not written.
  * **S1:** the settings table, F1.4.
  * **S2:** the two-factor flag and the app installations, F1.5.
  * **E1:** the four types formatted, F2.5.
  * **G1:** the grace-path test at HEAD and planted, F3.5.
  * **G2:** the coverage profile, F3.4.
  * **The D3 PLAN's pre-check:** its exact test at HEAD and with its two plants.
  * **The D2 PLAN's pre-checks:**
    * a prototype of D2-A with the helpers in each package (2026-11-01), with its tests, lint on three GOOS, `apidiff` from `v1.23.1`, and nine plants;
    * then the `internal/redact` form the owner chose (2026-11-02). Its lint and depguard ran clean on three GOOS, and `apidiff` again listed the 16 methods alone. Eleven plants failed, two of them depguard refusals of the new rule and allow.
* **GitHub documentation read on 2026-11-01:**
  * the About rulesets, Available rules for rulesets and Creating rulesets for a repository pages;
  * the REST pages for repository rules and repositories;
  * Configuring private vulnerability reporting for a repository;
  * Managing GitHub Actions settings for a repository;
  * in `github/docs`, `data/reusables/gated-features/repo-rules.md` and `src/ghes-releases/scripts/deprecate/create-docs-ghes-version-repo.sh`.
* **Related records:**
  * [0025-MADR-align-with-go-sdk-design-practices.md](0025-MADR-align-with-go-sdk-design-practices.md): D1-A, D9-A, Amendments A3 and A5;
  * [0025-PLAN-align-with-go-sdk-design-practices.md](0025-PLAN-align-with-go-sdk-design-practices.md): S2, S14 and Deviation D6;
  * [0037-MADR-honour-contexts-in-backend-fakes-and-stop-commands.md](0037-MADR-honour-contexts-in-backend-fakes-and-stop-commands.md) D5-A, and its PLAN's N5;
  * [0038-PLAN-pinned-plans-lifecycle-conformance-and-bubble-tests.md](0038-PLAN-pinned-plans-lifecycle-conformance-and-bubble-tests.md), Q19;
  * [0004-MADR-evolve-selfupdate-api-and-tui-support.md](0004-MADR-evolve-selfupdate-api-and-tui-support.md), "Release signing", the hardening list.
* **Out of scope:**
  * the other set-aside items the assessment listed;
  * D1-E;
  * secret scanning's non-provider patterns and validity checks, seen off in F1.4;
  * immutable releases, which apply to releases and this module publishes none (F1.8);
  * every setting of another repository.

## Amendment A1 (2026-11-02): D5, the architecture table check

The D2 PLAN's pre-check removed the new `internal/redact/` row from `docs/architecture.md`'s package table, and the check still passed. On 2026-11-02 the owner asked to fix that ("fix this"). This amendment adds D5. It is `proposed`, and the owner's decisions of the same day (Decision Outcome) do not cover it.

### D5 facts

* **F5.1 [read]. What the check does.** `scripts/check-docs.sh --arch` (`docs/decisions/0022-MADR-documentation-pass-after-v1-14-0.md` D2):
  * reads the table under "## Go code";
  * compares each row's first integers with the `.go` files in that directory, tracked or untracked and not ignored (`check_arch`, `:239-290`; `tracked_go`, `:220-236`);
  * reports a row for a directory with no `.go` file (exit 2).

  It never looks at a directory the table lacks. Its header says only that the table "states each directory's number of … files" (`:30-40`).
* **F5.2 [probed]. The gap, and the tree today.**
  * The D2 PLAN's pre-check plant A1 removed the `internal/redact/` row and `--arch` exited 0, with `20 rows … 0 differ`.
  * Probe A2 (scratchpad `p40arch.py`, read-only) listed every directory that holds a `.go` file, by the check's own `git ls-files` rule: 22 directories. 20 of them have rows, and the 20 rows are exactly `go list ./...`'s 20 packages.
  * The other two are `internal/cmd/selfupdate-release/testdata/fixture/cmd/nostamp` and `…/relay`. They are fixture programs that `go` does not build, and the `internal/cmd/selfupdate-release/` row's "plus the `testdata/fixture/` module" describes them.
* **F5.3 [docs, read]. What `./...` leaves out.**
  * `go help packages` (go1.27.3): patterns with "..." exclude "Directories named "vendor"", "Directories named "testdata"", "Files and directories with names beginning with "_" or ".", "Directories that contain a go.mod file", and "Directories matching an ignore directive in a module's go.mod file".
  * The tree has no `vendor` directory, and `go.mod` has no `ignore` directive. Its only other `go.mod` is `internal/cmd/selfupdate-release/testdata/fixture/go.mod`, under `testdata`.
* **F5.4 [read]. Who runs the check.**
  * It runs in `make docs-check`, which full CI and `docs.yml` run (`Makefile:115-118`), and in `make gate`'s `arch` step (`scripts/gate.sh:139-143`).
  * `docs.yml` installs no Go: it has no `setup-go` step.
  * Nothing parses the summary line: a scan of every tracked file outside the records, for its text, finds only the script and its test.
  * `scripts/check-docs_test.sh` tests the mode in a throwaway repository, and runs another script when `SCRIPT` names one (`:9`).
* **F5.5 [probed]. The fix, tried.** The D5 PLAN's pre-check ran D5-A on a scratch copy of `3acd87f`. Its own tests passed, 20 of 20. Against HEAD's script, the `good` and `missing` cases failed. The real page passed with `0 without a row`. A removed row, and an untracked `internal/redact/redact.go` with no row, each exited 1, where HEAD's script passes the second. With the exemption planted out, the fixture's exempt directories were reported.

### D5 options

* **D5-A.** `--arch` also fails, exit 1, on a directory that holds a `.go` file and has no row. The exception is a path with a part named `testdata` or `vendor`, or starting with `.` or `_`: what `./...` leaves out by name (F5.3).
  * The finding names the page, the directory and its counts.
  * The summary gains "`<K>` without a row".
  * The script's tests gain a missing-row case and exempt directories.
  * Nested modules and `ignore` directives are not exempted. None exists outside `testdata`, and one would fail loudly.
* **D5-B.** The same, with the package set from `go list ./...` in place of the `.go` files' directories.
* **D5-C.** No change to the check. Each plan that adds a package asserts its row by name, as the D2 PLAN did.

### D5 outcome

Proposed: "D5-A".

* **Why.**
  * It closes the gap the plant found (F5.2) with the file list the check already reads, so it needs no Go and runs the same in `docs.yml` (F5.4).
  * The rule is `go`'s own, by name (F5.3), and today it reports nothing on the real page (F5.5).
* **Rejected.**
  * D5-B needs Go in `docs.yml`, and its answer depends on the build constraints of the host it runs on.
  * D5-C leaves the next package to whoever remembers.
* **Consequences.**
  * Good, because a package added without its row fails `make docs-check`, `make gate` and CI before it is committed. The `internal/redact` row D2 adds is then checked like every other.
  * Neutral, because the summary line changes. Nothing reads it.
  * Bad, because a future nested module or `ignore` directive outside `testdata` needs the check amended. It fails loudly until then.
* **Confirmation.** The script's tests pass, and the new cases fail against HEAD's script. A removed row and an untracked package with no row each exit 1. The exemption, planted out, fails the tests. `make docs-check` and `make gate` exit 0.
* **Plan and order.** [0040-PLAN-require-architecture-table-rows.md](0040-PLAN-require-architecture-table-rows.md). It runs after D3's and before D2's, so D2's new row is checked by the gate itself. The owner's order "Records, then D4, D3, D2" becomes D4, D3, D5, D2, if the owner accepts D5. No release: no released file changes.

### D5 pros and cons

* **D5-A:** Good, because it needs only git and the tree's own file list. Bad, because it encodes `./...`'s name rules, and not its `go.mod` rules.
* **D5-B:** Good, because `go list` is the authority on packages. Bad, because it needs Go where the Docs workflow has none, and it varies with the host's GOOS.
* **D5-C:** Good, because it costs nothing now. Bad, because the gap stays for every later package.
