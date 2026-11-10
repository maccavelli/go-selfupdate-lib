---
status: accepted
date: 2026-11-10
decision-makers: go-selfupdate-lib maintainers
consulted: "issue #11, opened 2026-11-10; every non-test Go file under selfupdate/ and internal/ at 8b19f1d, whose Go code is v1.26.0's; the cli golden files; .golangci.yml's depguard rules; internal/testproc's source scan; probe P1, run on 2026-11-10 on the macOS development host"
informed: the fleet's programs, which require this module and print its errors
---
<!-- markdownlint-disable MD004 MD013 MD024 MD060 -->

# Name the package once in a wrapped error, through one `internal/errprefix` helper that every literal-prefix wrap and `wrapRun` use, held by a source scan

## Context and Problem Statement

Issue #11 reports that the error an `Updater` run returns names the package twice. `wrapRun` puts `selfupdate: <product>: ` in front of an error that already begins with `selfupdate: `. A CLI's user reads `update failed: selfupdate: prepare-commit-msg: selfupdate: refusing to update through a symlink: …`. The issue proposes, as its Option A, that `wrapRun` drop one leading `selfupdate: ` from what it wraps.

Reading the tree found the same shape far beyond `wrapRun`. Calls of the form `fmt.Errorf("selfupdate: <literal text>: %w", err)` occur 136 times. When `err` is one of the package's sentinels, which all begin with `selfupdate: `, the package is named again. A run that fails then names it three times (F2).

This record decides:

* which wrap sites drop the wrapped error's leading prefix (D1);
* how they do it (D2);
* what happens to an `errors.Join` (D3);
* where the product sits in the line (D4);
* what is left out (D5);
* how the rule is held (D6);
* how the change ships (D7).

Facts are tagged by their source:

* **[read]:** the tree at `8b19f1d`. Its Go code is `v1.26.0`'s: `git diff --name-only v1.26.0 HEAD -- '*.go'` prints nothing.
* **[probed]:** probe P1, described in F2.
* **[derived]:** follows from the above.

### Facts

* **F1 [read]. `wrapRun`.** `selfupdate/updater.go:768-773` returns `fmt.Errorf("selfupdate: %s: %w", req.Product, err)` whatever `err` begins with. It has 36 non-test callers:
  * `checkcache.go`: 1;
  * `checker.go`: 2;
  * `plan.go`: 5;
  * `updater.go`: 28.

  `plan.go:394` builds the error it wraps inline, with the prefix: `errors.New("selfupdate: a plan is for an update; use Checker.Check for a check")`.
* **F2 [probed]. Probe P1: the text a run returns today.** A scratch module outside the tree required this one through a `replace` directive. It built an `Updater` over `selfupdatetest.NewFakeSource` with `NonInteractiveConfirmer`, and ran it twice with go1.27.3 on darwin/arm64. Each line is printed as `cli.Exit` prints it. `git status --short` was empty afterwards.

  | Case | Text |
  | :--- | :--- |
  | no `Yes` | `update failed: selfupdate: demo: selfupdate: pass --yes to apply without prompting: selfupdate: confirmation required` |
  | binary body changed after the manifest was written | `update failed: selfupdate: demo: selfupdate: staged digest does not match SHA256SUMS: selfupdate: integrity check failed` |

  The package is named three times. `wrapRun` adds the first repeat. The `fmt.Errorf` that wraps a sentinel adds the second.
* **F3 [read]. The literal-prefix wraps.** A scan of every non-test `.go` file, outside `internal/cmd/` and `internal/testproc/`, found 136 calls in 43 files whose format is a single literal that begins `selfupdate: `, ends `: %w`, and has no other verb:

  | Directory | Calls |
  | :--- | ---: |
  | `selfupdate/` | 93 |
  | `selfupdate/service/` | 14 |
  | `selfupdate/service/taskscheduler/` | 7 |
  | `selfupdate/archive/` | 4 |
  | `selfupdate/verify/ghattest/` | 4 |
  | `internal/procfs/` | 4 |
  | `selfupdate/service/scm/` | 3 |
  | `selfupdate/verify/signednote/` | 3 |
  | `selfupdate/service/systemd/` | 2 |
  | `selfupdate/codesign/` | 1 |
  | `selfupdate/service/launchd/` | 1 |

  * At 25 of them, the wrapped value is a sentinel, or an `errors.Join` that starts with one. Twenty-four are in `selfupdate/`, and one is `service/handoff.go:198`. The sentinels are `ErrIntegrity`, `ErrConfirmationRequired`, `ErrConcurrentUpdate`, `ErrManagedInstall` and `ErrRateLimited`. Each one's text begins `selfupdate: ` (`selfupdate/errors.go:17-46`).
  * Some wrap an error from another of this module's packages, which carries its own prefix:
    * `archive/select.go:78` wraps `NewExactAssetSelector`'s error, such as `selfupdate: asset selector requires at least one platform` (`selfupdate/assets.go:28`);
    * the runner's error that `codesign.go:100` wraps can be `winproc.ErrBatchFile`, whose text begins `selfupdate: ` (`internal/winproc/winproc.go:29`).
  * Not every error this module builds carries the prefix. `PollOptions.Validate`, which `launchd/job.go:173`, `scm/service.go:95`, `systemd/unit.go:121`, `taskscheduler/task.go:186` and `supervised.go:164` wrap, begins `service: ` (`selfupdate/service/poll.go:73`).
  * The rest wrap an error from the standard library, which carries no prefix.
  * One of the 136 is an exported sentinel. `checkcache.go:129` defines `ErrCheckDeferred = fmt.Errorf("selfupdate: check deferred by an earlier rate limit: %w", ErrRateLimited)`.
* **F4 [read]. What is not a literal-prefix wrap.** A `%w` that follows a verb is mid-line. Its wrapped text comes after text the call computes. Examples:
  * the four backends' binary-mismatch refusals, such as `"selfupdate: systemd: %s runs %s, not %s: %w"` (`systemd/reconcile.go:57-58`);
  * `codesign.go:268-270`, which wraps `ErrIntegrity` after the tool's output;
  * `checkcache.go:189`, `"cached check from %s: %w"`.

  Their tests pin the repeated sentinel text, for example `systemd/reconcile_test.go:37`.
* **F5 [read]. What pins today's text.**
  * `selfupdate/cli/flags_test.go:128` expects exactly `"selfupdate: demo: " + reqErr.Error()`, where `reqErr` begins `selfupdate: `.
  * Six golden files under `selfupdate/cli/testdata/golden/` hold the repeated text: `contradiction.{json.stderr,json.stdout,text.stderr}` and `no-confirm.{json.stderr,json.stdout,text.stderr}`. The `.json.stdout` files hold it in the `--json` result's `error` field. `helpers_test.go:16` rewrites them with `-update`.
  * `updater_contract_test.go:545` checks only that the text begins `selfupdate: demo: `.
  * Two records quote the repeated form as output of their time. They are history:
    * `0025-PLAN-align-with-go-sdk-design-practices.md:2097`;
    * `0041-PLAN-refuse-symlink-invocation-on-every-platform.md:169`.
* **F6 [read]. Joined errors.** `updater.go:277` joins errors that are each already wrapped. `updater.go:302` wraps `errors.Join(err, closeErr)`. Five `managed.go` calls wrap `errors.Join(ErrManagedInstall, …)`. A join's text is its parts separated by newlines. `cli`'s `oneLine` (`cli/run.go:372-379`) folds them into one line.
* **F7 [read]. The import rules.**
  * A package under `internal/` with no rule of its own falls under `other-packages`, which allows this module (`.golangci.yml:292-320`). AGENTS.md requires a new package to get its own rule and be excluded from `other-packages`.
  * `internal-procfs` allows the standard library only (`.golangci.yml:105-111`; `0035-MADR-serve-supervisors-through-a-supervised-adapter.md` D13-C).
  * Ten package rules would need to allow a new helper: `selfupdate`, `service`, `service-launchd`, `service-scm`, `service-systemd`, `service-taskscheduler`, `selfupdate-archive`, `selfupdate-codesign`, `selfupdate-verify-ghattest` and `selfupdate-verify-signednote`.
* **F8 [read]. The 0042 scan.** `TestRunTimeMessagesNameNoGoIdentifier` (`internal/testproc/testproc_test.go:401`) joins the string literals of each `fmt.Errorf` and `errors.New` call (`errorMessages`, `:478-512`). It checks them against 0042-MADR D1-A's rule. It fails when it reads fewer than 100 files or 500 calls.
* **F9 [read]. What the guide promises.** The migration guide says "Every error names the product (`selfupdate: <product>: …`), and `errors.Is` still matches the sentinels" (`docs/guides/migrating-from-mcplib-selfupdate.md:141-142`).
* **F10 [read]. No caller unwraps by hand.** `git grep 'errors.Unwrap('` finds nothing. Every check in the tree goes through `errors.Is` or `errors.As`, which walk a chain of any length.

## Decision Drivers

* **A CLI's user reads the line.** 0042-MADR made run-time messages something a user is meant to read. The first thing on the line should be the fact, not a repeat (issue #11).
* **`errors.Is` and `errors.As` keep matching.** Programs match sentinels and `*SymlinkInvocationError`, not text (F9, F10).
* **One place decides how a wrapped error renders,** so a later change does not touch every site again (the owner's priorities, 2026-11-10: extensibility, flexibility, future-proofing).
* **The rule holds by itself.** A new wrap written later must not bring the repeat back unnoticed.
* **No exported API.** A text fix needs no new exported identifier, and `make apicheck` stays clean.

## Considered Options

### D1: which sites (F1, F3, F4)

* **D1-A.** `wrapRun` and every literal-prefix wrap of F3.
* **D1-B.** `wrapRun` and the 17 literal-prefix wraps whose first segment is a package name.
* **D1-C.** `wrapRun` alone, as issue #11's Option A.

### D2: how (F7, F8)

* **D2-A.** `internal/errprefix` offers two functions:
  * `Wrap(op string, err error) error`, which owns the whole composition;
  * `Strip(err error) error`, the primitive under it, for any shape `Wrap` does not fit.

  The literal-prefix wraps and `wrapRun` call `Wrap`.
* **D2-B.** `internal/errprefix.Wrap` alone, at every site.
* **D2-C.** `internal/errprefix.Strip` alone. Each site keeps its `fmt.Errorf` and wraps its argument: `fmt.Errorf("selfupdate: stat target: %w", errprefix.Strip(err))`.
* **D2-D.** An unexported copy of the helper in each of the ten packages, and no new package.

### D3: joined errors (F6)

* **D3-A.** Only the text's leading prefix is dropped, which is the first part's.
* **D3-B.** Each line of a joined text drops its own leading prefix.
* **D3-C.** A join is unwrapped, and each part gets its own `selfupdate: <product>: `.

### D4: the line (F9)

* **D4-A.** `update failed: selfupdate: <product>: <reason>`. The product follows the package, and `cli.Exit`'s `update failed: ` stays outermost.
* **D4-B.** `update failed: selfupdate: <reason> (<product>)`.
* **D4-C.** `selfupdate: <product>: <reason>`, with `cli.Exit` printing no `update failed: `.

### D5: what is left out (F3, F4, F7)

* **D5-A.** The mid-line wraps of F4, and `internal/procfs`'s four calls.
* **D5-B.** The mid-line wraps only. `procfs` imports the helper, and 0035-MADR D13-C is amended.
* **D5-C.** The mid-line wraps only. `procfs` keeps its own unexported copy.

### D6: how the rule is held (F8)

* **D6-A.** A source scan in `internal/testproc`, beside 0042's.
* **D6-B.** Run-time tests only: `errprefix`'s table test, the golden files and P1's two cases.
* **D6-C.** A `golangci-lint` rule.

### D7: the release

* **D7-A.** `v1.26.1`.
* **D7-B.** `v1.27.0`.
* **D7-C.** Unreleased on `main` until the next feature.

## Decision Outcome

The owner chose every option on 2026-11-10, as AskUserQuestion answers, each from three or four options with the recommended one first.

* **D1: "D1-A".** `wrapRun` and the 132 literal-prefix wraps outside `procfs` (D5-A).
  * The owner was first shown an undercount, "106 sites in 32 files", from a pattern that missed formats with more than one literal segment, such as `"selfupdate: service: hand the update off: %w"`. The criterion did not change. The count was corrected to 136 in 43 files before D2 was asked, and the owner chose with that count.
  * D1-B leaves P1's second repeat: `selfupdate: demo: staged digest does not match SHA256SUMS: selfupdate: integrity check failed`.
  * D1-C leaves both inner repeats.
* **D2: "D2-A".** The owner asked for D2-B and D2-C's pros and cons, weighed for extensibility, flexibility and future-proofing, and then chose the hybrid.
  * **`errprefix.Error`** is `struct{ Op string; Err error }`.
    * `Error()` returns `Op + ": " + ` the text of `Err` with one leading `selfupdate: ` removed.
    * `Unwrap()` returns `Err`.
  * **`Wrap(op, err)`** returns nil for a nil `err`, and otherwise `&Error{Op: op, Err: err}`.
  * **`Strip(err)`** returns `err` itself when its text does not begin `selfupdate: `. Otherwise it returns a value whose `Error()` drops that prefix and whose `Unwrap()` returns `err`. `Error.Error` uses the same removal.
  * **At the sites.** Each literal-prefix wrap becomes `errprefix.Wrap("selfupdate: <literal text>", x)`, keeping the literal without its trailing `: %w`. `wrapRun` becomes `errprefix.Wrap("selfupdate: "+req.Product, err)`.
  * **Why the hybrid.**
    * The operation and the cause are fields, so a later change can be made in one type with no site touched again. It might add a `log/slog` value, an `Is` rule, or the removal of a nested repeat.
    * `Strip` keeps D2-C's flexibility for a shape `Wrap` does not fit, with no `Wrapf`.
  * **Rejected.**
    * D2-B: a later shape would need a new helper then.
    * D2-C: the composition rule would be split across 136 sites, and adding structure later would touch each one again.
    * D2-D: ten copies to keep in step.
* **D3: "D3-A".**
  * It is no worse than today. The later parts of a join already carry neither the product nor a second prefix.
  * It is the simplest rule to state and test.
  * D3-B leaves the later parts naming nothing. D3-C changes what `Unwrap` returns.
* **D4: "D4-A".**
  * It keeps F9's documented shape and every prefix check (F5).
  * D4-B reorders a line the guide documents.
  * D4-C changes `cli` beyond the issue.
  * This answers issue #11's review questions 1 and 2.
* **D5: "D5-A".**
  * `procfs` wraps only `os` errors read from `/proc`, which cannot begin `selfupdate: `. 0035-MADR D13-C keeps it free of this module's imports.
  * The mid-line wraps of F4 compose text the call computes, so "the leading prefix" of the wrapped part is not where the line begins.
  * D5-B amends a decision for calls where the helper would never act. D5-C adds a second copy.
* **D6: "D6-A".** `TestLiteralPrefixWrapsUseWrap`:
  * It refuses any `fmt.Errorf` whose format is a literal that begins `selfupdate: `, ends `: %w`, and has no other verb, in the files 0042's scan reads, outside `internal/procfs/`.
  * It names each file and line it refuses.
  * It fails when it finds fewer than 130 `errprefix.Wrap` calls, which catches a wrong root.
  * 0042's scan also reads the literal of each `errprefix.Wrap` call, so the 136 literals stay under its rule and its 500-call floor.
  * D6-B misses a new site no test triggers. D6-C needs a custom analyzer, since `forbidigo` matches call names and not formats.
* **D7: "D7-A".**
  * Only error text changes, and no exported identifier is added.
  * `v1.26.0` released its message changes in a minor because it also added API. D7-B would signal a change of API that is not there.
  * D7-C leaves the fleet's programs, `prepare-commit-msg` among them, with the repeated text.

### Consequences

* Good, because P1's lines read `update failed: selfupdate: demo: pass --yes to apply without prompting: confirmation required` and `update failed: selfupdate: demo: staged digest does not match SHA256SUMS: integrity check failed`.
* Good, because issue #11's line reads `update failed: selfupdate: prepare-commit-msg: refusing to update through a symlink: <link> links to <target>; start <target> itself to update it`.
* Good, because `errors.Is` and `errors.As` match as before. Each chain gains one link, and no caller unwraps by hand (F10).
* Good, because a sentinel printed alone keeps its prefix, and so still names its package.
* Good, because one type, `errprefix.Error`, decides how every literal-prefix wrap renders.
* Good, because `TestLiteralPrefixWrapsUseWrap` refuses a new literal-prefix `fmt.Errorf`.
* Neutral, because `ErrCheckDeferred`'s dynamic type changes from `fmt`'s wrapper to `*errprefix.Error`. Its declared type is `error`, so `make apicheck` is unaffected. Its text drops the inner prefix: `selfupdate: check deferred by an earlier rate limit: rate limited`.
* Neutral, because the mid-line wraps of F4 keep their repeat: for example `…: selfupdate: service: binary mismatch`. A later record can apply `Strip` there.
* Neutral, because a joined error's later parts keep their own prefix (D3-A).
* Bad, because a program that matches the full text sees it change. The CHANGELOG and the migration guide list the change, and the sentinels are the documented way to match.
* Bad, because ten depguard rules, `other-packages`, AGENTS.md and `docs/architecture.md` change for one helper.
* Bad, because the 132 sites use a call specific to this project, in place of the standard `fmt.Errorf`.

### Confirmation

* **`errprefix`'s table test** covers each case below. Each checks the text, and `errors.Is` or `errors.As` against the original:
  * a prefixed sentinel;
  * a wrapped sentinel whose text begins with the prefix;
  * an error with no prefix, which `Strip` returns as itself;
  * a prefix later in the text but not at the start, which stays;
  * a nil error;
  * a join whose two parts are both prefixed, where only the first loses it;
  * `errors.As` to an error type of the test's own, through `Wrap`.
* **P1's two cases become tests** in `selfupdate`, asserting the exact text and `errors.Is`. The same test checks `errors.As` to `*SymlinkInvocationError` through `wrapRun`, since `errprefix` cannot import `selfupdate`.
* **The golden files are rewritten,** and their diff is exactly the six lines of F5.
* **`flags_test.go:128`** expects one prefix.
* **Seen failing,** each in a scratch copy and never in the tree:
  * `TestLiteralPrefixWrapsUseWrap` on the tree before the sites change: it names the 132 sites;
  * one site turned back into `fmt.Errorf`: the scan names it;
  * the scan without its `internal/procfs/` exclusion: it names exactly `procfs`'s four calls;
  * `Strip` returning `err` unchanged: the table test and the golden files fail;
  * `Strip` removing every occurrence and not only the leading one: the "later in the text" case fails;
  * a `Wrap` literal given a Go identifier: 0042's scan names it.
* **`make gate`** ends `overall=0`, `make apicheck` is compatible with `v1.26.0` and lists no addition, and the four documentation checks are clean.

## Pros and Cons of the Options

### D1-A

* Good, because every repeat the issue and P1 show is gone.
* Bad, because it changes 43 files.

### D1-B

* Good, because it changes about 12 files.
* Bad, because the sentinel repeats that integrity, confirmation and concurrency failures show stay.

### D1-C

* Good, because one function changes.
* Bad, because both of P1's inner repeats stay.

### D2-A

* Good, because the composition lives in one type with named fields.
* Good, because `Strip` serves a shape `Wrap` does not.
* Bad, because there are two functions to learn.

### D2-B

* Good, because there is one idiom.
* Bad, because a later shape needs a new helper.

### D2-C

* Good, because the standard idiom, and `go vet`'s printf check, stay at every site.
* Bad, because adding structure later touches every site again.

### D2-D

* Good, because no import rule changes.
* Bad, because there are ten copies of one function and its tests.

### D3-A

* Good, because the rule is simple, and no worse than today.
* Neutral, because a join's later parts keep their prefix.

### D3-B

* Good, because the whole folded line reads once.
* Bad, because later parts then name neither package nor product.

### D3-C

* Good, because each part says where it came from.
* Bad, because the text grows and `Unwrap`'s shape changes.

### D4-A

* Good, because it keeps the documented shape and every prefix check.

### D4-B

* Good, because the reason comes first.
* Bad, because it changes the documented shape.

### D4-C

* Good, because the line is shorter.
* Bad, because the line no longer says what failed.

### D5-A

* Good, because 0035-MADR D13-C stands, and `procfs` cannot carry the prefix.
* Neutral, because the scan names one exclusion.

### D5-B

* Good, because there is no exception.
* Bad, because it amends a decision for calls where the helper never acts.

### D5-C

* Good, because there is no exception, and D13-C stands.
* Bad, because there is a second copy of the helper.

### D6-A

* Good, because a new site that forgets the rule fails at once, naming its line.
* Neutral, because it is a second scan beside 0042's.

### D6-B

* Good, because there is less machinery.
* Bad, because a site no test triggers passes.

### D6-C

* Good, because it runs in lint.
* Bad, because it needs a custom analyzer.

### D7-A

* Good, because the release matches the change: text only.

### D7-B

* Neutral, because it signals the change more loudly.
* Bad, because there is no API to justify a minor release.

### D7-C

* Good, because there is no tag work now.
* Bad, because the fleet keeps the repeat until the next feature.

## More Information

* Issue #11, its Options A to C, and its review questions 1 and 2, which D4-A answers.
* Plan: [0043-PLAN-name-the-package-once-in-wrapped-errors.md](0043-PLAN-name-the-package-once-in-wrapped-errors.md).
* Related:
  * [0042-MADR-keep-go-identifiers-out-of-run-time-messages.md](0042-MADR-keep-go-identifiers-out-of-run-time-messages.md), whose rule the `Wrap` literals stay under, and whose scan D6-A extends;
  * [0041-MADR-refuse-symlink-invocation-on-every-platform.md](0041-MADR-refuse-symlink-invocation-on-every-platform.md) Amendment A1, which gave the symlink refusal the text issue #11 quotes;
  * [0035-MADR-serve-supervisors-through-a-supervised-adapter.md](0035-MADR-serve-supervisors-through-a-supervised-adapter.md) D13-C, which D5-A keeps;
  * [0040-MADR-close-four-open-items-after-v1-23-1.md](0040-MADR-close-four-open-items-after-v1-23-1.md) D2-A, which added `internal/redact` the way D2-A adds `internal/errprefix`.
* **Out of scope:**
  * the mid-line wraps of F4;
  * `internal/cmd/`'s release tools, `selfupdatetest` and `servicetest`, as in 0042-MADR;
  * `cli`'s `update failed: ` (D4-A);
  * any exported error type.
