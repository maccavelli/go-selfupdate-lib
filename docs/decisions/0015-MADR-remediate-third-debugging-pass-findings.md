---
status: accepted
date: 2026-10-07
decision-makers: go-selfupdate-lib maintainers
consulted: the 0004 roadmap's records (0011–0014)
informed: the fleet's programs that will adopt the build workflow and installers
---
# Fix the third debugging pass's findings: a v1.10.1 of contract-preserving fixes, record and tooling fixes on main, and a v1.11.0 for the contracts the owner decides

## Context and Problem Statement

On 2026-10-07, after `v1.10.0` closed the 0004 roadmap's Phase 4
([0014-PLAN-shared-installer-templates.md](0014-PLAN-shared-installer-templates.md)),
the owner asked: "run a debugging pass across the codebase. find bugs. find
gaps. find missing or incomplete wiring or functionality. write findings to
an madr for review. be thorough."

This record lists what the pass found and proposes how to fix it. It is the
third such pass, after
[0003-MADR-remediate-debugging-pass-findings.md](0003-MADR-remediate-debugging-pass-findings.md)
(before `v1.0.0`) and
[0010-MADR-remediate-second-debugging-pass-findings.md](0010-MADR-remediate-second-debugging-pass-findings.md)
(at `v1.5.0`). Since 0010 the module has grown by `selfupdate/service`
and its three backends (`v1.7.0`), `selfupdate/archive` and
`selfupdate/codesign` (`v1.8.0`), `selfupdate/releasespec`, the release
tool and the build workflow (`v1.9.0`), and the generated installers
(`v1.10.0`). Most findings are in that new code.

### Method

* Seven read-only reviews ran in parallel, one per area, against `HEAD`
  `18e0575` (`v1.10.0` plus its pin commit):
  * **A**, network and integrity: `github.go`, downloads, `SHA256SUMS`
    and `manifestverify.go`, asset selection, versions and channels,
    `checkcache.go`, credentials;
  * **B**, filesystem, locking and install: `target.go`, `lock_*`,
    `replace*`, `cleanup*`, `leftovers.go`, `session.go`,
    `standalone.go`, `managed.go`, `probe.go`, `imageverify.go`;
  * **C**, the coordinator, the public API and the command surface:
    `updater.go`, `checker.go`, `stream.go`, reporters, confirmers,
    `types.go`, `selfupdate/cli`, `buildinfo`, `selfupdatetest`;
  * **D**, service lifecycles: `selfupdate/service`, `systemd`,
    `launchd`, `scm`, and their wiring to `ManagedInstaller`;
  * **E**, `selfupdate/archive`, `selfupdate/codesign` and
    `selfupdate/releasespec`;
  * **F**, release tooling, installers and CI:
    `internal/cmd/selfupdate-release`, the installer templates, the three
    workflows, `scripts/*`, the Makefile;
  * **G**, documentation, wiring and the roadmap: every guide, package
    doc and record against the code.
* Each review measured the code against its own documentation, the guides,
  `docs/architecture.md` and the records (0003, 0004, 0005, 0010–0014),
  re-checked the fixes 0003 and 0010 made in its area, and reproduced each
  finding with a probe test or a planted break where it could.
* Every experiment ran in a scratch clone. The repository was not
  modified; each reviewer and the author checked `git status` before and
  after.
* The author re-ran the headline evidence in a scratch clone of the
  author's own
  (B1, C1, C2, C3), read the cited code for the High and Medium findings,
  and checked the one external fact a finding rests on (D2, systemd's
  restart counter) in systemd's source.

**Evidence** column:

* **R:** reproduced in the author's re-run;
* **R\*:** reproduced by the reviewer only;
* **C:** confirmed by the author reading the cited code;
* **C\*:** the reviewer's code read only;
* **—:** reasoning only.

A finding two reviewers raised keeps one ID and names the other. IDs are
this record's own: cite them as "0015-MADR B1", since 0010 used the same
letters.

**Baseline:**

* `go test -race ./...` passes in all 12 packages, with total coverage
  88.4 %: `buildinfo` 100 %, `codesign` 97.7 %, `releasespec` 96.9 %,
  `cli` 95.2 %, `archive` 92.1 %, `selfupdate` 91.4 %, `selfupdatetest`
  90.7 %, `service` 86.4 %, `systemd` 83.6 %, the release tool 81.3 %,
  `launchd` 81.3 %, `scm` 80.6 %.
* CI was green on `18e0575` (run 37634065621).
* `make lint` on three targets, `go vet`, cross `go vet`, `govulncheck`,
  `go mod tidy -diff`, `make apicheck` (compatible with `v1.10.0`), every
  `scripts/*_test.sh`, shellcheck, markdownlint and actionlint are clean.
* Fuzzing: each of the four archive and releasespec targets ran 30 s
  clean (2.3–3.8 million executions each).

Two findings are rated High. Neither installs an unverified binary: the
download, `SHA256SUMS` and digest checks held in every review. B1 loses
the only copy of the previous binary after a failed restore; B3 leaves a
service stopped after a failed stop. The Medium findings break a
documented contract or fail a supported setup.

### High

| ID | Where | Finding | Evidence |
| :--- | :--- | :--- | :--- |
| B1 | `leftovers.go:57-79`, `session.go:403-406`, `cleanup_other.go:25-27`, `types.go:156-163` | When a restore fails after the new binary went live (a failed directory sync, or a failed post-install probe whose rollback fails), the run reports `Result.PendingBackup` as "the only copy of the previous binary … the caller must restore or remove it". The next session's leftover sweep (0010 Q6) removes every `.<base>.selfupdate-bak-<n>` on Unix, because only Windows writes a receipt that lists backups to keep. That next session comes from the startup `CleanupPending` the docs ask for, any later `Run`, or a dry run, which doc.go says "leaves the target untouched". The previous binary is lost silently, and a known-bad binary stays live. A crash between `Apply` and `Commit` ends the same way. | R: install `applied=false backup=.demo.selfupdate-bak-835335262 holds "old-bytes"`; after `CleanupPending`: the backup `no such file or directory`, the target `"new-bytes"` |
| B3 (= D1) | `managed.go:104-108`; `service/launchd/lifecycle.go:121-148`; `service/systemd/lifecycle.go:77-88` | A `Stop` error ends `Install` with no recovery: nothing calls `Start`. The backends can fail after the stop happened: launchd's `bootout` runs before `waitGone` times out (its wait is `Poll.Timeout`, 60 s by default, not the job's `ExitTimeOut` plus 30 s that 0011 §7 states); systemd's and SCM's stop waits can time out or lose their context after the unit stopped. The service stays down. 0011's Decision Drivers: "a reference lifecycle that leaves a service stopped … is worse than none". | C; R\*: fake lifecycle `stops=1 starts=0 healths=0`; fake launchd job: `bootout … timed out`, `job loaded after the failed update: false`, no `bootstrap` or `kickstart` |

### Medium

| ID | Where | Finding | Evidence |
| :--- | :--- | :--- | :--- |
| A1 | `assets.go:108-113`, `archive/select.go:151-153` vs `checkcache.go:132-136`, doc.go | 0010's fix caches `ErrUnsupportedPlatform`, and the guide says such a program "no longer asks GitHub on every start". But both selectors return a plain error when the platform is in their matrix and the release lacks its asset, the realistic case of a release that dropped a platform. `CheckCached` saves nothing and asks GitHub on every start. A 404 from `releases/latest` (no stable release yet) is not cached either. | C; R\*: five `CheckCached` calls, `apiRequests=5`, `release v1.1.0 has no exact asset "demo-linux-arm64"` |
| A2 | `github.go:681-709,724-744` | The residual of 0010 A2. Outside a Stream, `PromptCredential` returns `ErrNoCredential`, `resolve` falls back to `GH_TOKEN`/`GITHUB_TOKEN`, and the source caches that token for its lifetime; only anonymity is re-resolved per run. A startup `Check` therefore stops every later `Start` from asking the provider, and a stale env token's 401 cannot refresh. | R\*: with a startup check, `Start prompts=0 … github http 401`; without it, `prompts=1 applied=true` |
| B2 | `session.go:237-261`, `replace_unix.go:47-60` | The target is validated once, in `beginSession`. After the downloads, verifiers, unpack and probes, `replaceLocked` re-checks only the directory, and `replaceTarget` takes the owner and mode from whatever `Lstat` finds. A target replaced in that window, by a package manager or with a symlink, is overwritten with no `ErrConcurrentUpdate`; a Linux symlink's 0777 mode makes the new binary world-writable, bypassing `resolveTarget`'s refusal to update through a symlink. | C; R\*: `symlink: applied=true … target now "new-bytes" mode -rwxrwxrwx` |
| C1 | `cli/run.go:143-155,173-176` | Under `--json` the result object is written before two steps that can still fail the run: writing the `warning:` lines (its error never reaches `finish`) and `HandOff.Report`. The object says `"exit_code":0` with no error while the process exits 1. 0004 F8: the result object carries the run's exit code. cli/doc.go also says an error after the run did its work leaves the status 0, while `HandOff.Report` is documented to fail the run. | R: `{"kind":"result","exit_code":0,…,"applied":true…}`, process exit 1 |
| C2 | `cli/command.go:39-57` | A detached handoff run that fails before `Run` (the updater cannot be built, or the options are refused) ends through `Options.report` and never calls `HandOff.Report`. In a detached run that is what writes the result file, so the agent that handed off finds none. 0011 §9: "Report runs after the update with its outcome, in every run". | R: `newUpdater fails: exit=1 Report calls=0` |
| C4 | `checker.go:95-97,134-150` vs `updater.go:157-167` | `Run` refuses a selector and unpacker that disagree (`matchUnpacker`, 0012 §2); `Updater.Checker()` carries no unpacker, so `Check` and `CheckCached` report an update `Run` refuses. The comment two lines above says "Run and Check cannot disagree (0004-MADR G3)". A startup banner can advertise an update that can never apply. | C; R\*: `Run --check err=… no Unpacker is configured`, `Checker.Check available=true` |
| D2 | `service/systemd/lifecycle.go:99-104,138` | `Start` reads `NRestarts` while the unit is stopped; `WaitHealthy` fails on any difference. systemd zeroes the counter on a start that is not an automatic restart (`src/core/service.c:3623-3625` on `main`, read 2026-10-07: "This is not an automatic restart? Flush the restart counter then."). A unit that auto-restarted since its last manual start has its first update rolled back as unhealthy. 0011 §7 says it fails "on an `NRestarts` increase". | C; R\*: baseline 3, after start 0: `not healthy: … NRestarts=0` |
| D3 | `service/launchd/reconcile.go:29-34`, `lifecycle.go:171-176`, `managed.go:96-108` | `RewritePath` assumes `Stop` booted the job out and `Start` bootstraps it. A job that is loaded but not running (`RunAtLoad`, process exited) is not stopped, so after the plist is rewritten `Start` sees it loaded and `kickstart` runs launchd's cached definition: the old path. The update reports success with the old binary running. 0011 §5: "A changed plist needs `bootout`, the wait, then `bootstrap`." | R\*: launchctl verbs `[managername list print-disabled list enable kickstart]`, no `bootout` or `bootstrap` |
| D4 | `service/systemd/reconcile.go:17,88-102,130-146` | systemd applies drop-ins sorted by file name across directories, so `90-selfupdate.conf` loads before `override.conf` (`systemctl edit`). An override that resets `ExecStart=` keeps the old binary, and `reload` checks only `NeedDaemonReload`, never the resulting `ExecStart`. `Reconcile` reports success. | R\*: `changed=true err=<nil>`, effective `ExecStart` `/opt/old/demo run --flag` |
| D5 | `service/scm/reconcile.go:48-61,92-108` | An unquoted `BinaryPathName` containing a space, naming a moved binary (the T1574.009 case 0011 cites), is split at the first space. `RewritePath` writes the tail of the old path as arguments; without it the error names the wrong path. | R\*: `C:\Program Files\Old\demo.exe run` → `"C:\Program Files\New\demo.exe" Files\Old\demo.exe run` |
| D6 | `service/scm/lifecycle.go:120-134,181-211` | `Options.StopDependents` stops the active dependents; nothing records them and nothing starts them again. After a successful update they stay down, the outcome 0011's drivers reject; §7 says only that stopping them is an option. | R\*: after Stop, Start, WaitHealthy: `demo` running, `child` stopped |
| E1 | `archive/unpack.go:339-372` | The zip path trusts only the central directory's name. It never compares the local header's name, and ignores the Info-ZIP Unicode Path extra field (0x7075). bsdtar, which is macOS `tar` and Windows `tar.exe`, uses both. Two tools extract different programs from one archive, which 0012 §4 says is refused. The archive is still checked against `SHA256SUMS`; what fails is that a reviewer extracts what the updater installs. | C; R\*: names swapped between local and central headers: Go installs `GOOD`, `bsdtar -xf` writes `relay = EVIL`; the 0x7075 case the same |
| E2 | `archive/unpack.go:209` | "Differs only in case" uses `strings.ToLower`, not the folding APFS and NTFS apply (`ſ` U+017F folds to `s`), and accepts `relay.exe` beside `relay.exe.` or `relay.exe ` (Win32 strips both). 0012 §4 adopted the rule because a case-insensitive file system lets `tar` write one entry over another. | R\*: `relays` and `relayſ` accepted, Go installs `GOOD`; on APFS `tar -xzf` leaves one file, `EVIL` |
| F1 | `installer/install.sh:342,402-417`; `install.ps1:323-326,399-410` | `--product relay --product relay` lists `relay` twice. The second pass of the swap moves the new binary over `relay.prev`, destroying the old one, then fails on the missing `.new` and exits 1 with nothing restored. The guide says "repeat it for more"; the exit codes promise "nothing was changed, or the previous binaries were put back". | C; R\*: exit 1, `mv: … relay.new … No such file or directory`; only `relay.prev`, holding the new binary, remains |
| F2 | `installer/install.sh:206-220,239-251` | `run_hooks` and `check_identity` read their lists with `done <file`, and the program each runs inherits that file as stdin. A hook or identity command that reads stdin swallows the remaining lines: later products are never identity-checked and later hooks never run, and the install exits 0. | C; R\*: under sh, dash and bash, `relayctl` reporting `v0.0.1 (release)` passed, and its hook did not run |
| F3 | `build-selfupdate-release.yml:110-142`, `plan.go:34-40`, building guide step 12 | The workflow parses the spec with its own `releasespec`; the program parses its embedded copy with the version its `go.mod` requires. A caller on the `v1.10.0` pin with `"installer": {}` and `go.mod` still at `v1.9.0` builds and publishes cleanly, and the shipped program's `releasespec.Parse` fails, taking self-update with it. The migration guide's §10 says to move both together; the building guide's step 12 does not, and nothing checks. | R\*: `v1.9.0`'s `Parse`: `releasespec: json: unknown field "installer"` |
| F4 | `publish-selfupdate-release.yml:222-263`, `README.md` ("publishes an immutable release"), building guide §4 | The publish workflow publishes, then waits 120 s for `isImmutable`. In a repository without immutable releases the release stays published, mutable and latest, and only the job fails. The client refuses it, but the generated installers install from it. Neither the guide's step 4 nor the README names the setting, and the timeout message does not either. Attestations likewise need a public repository or GitHub Enterprise Cloud. | C\* |

### Low

| ID | Where | Finding | Evidence |
| :--- | :--- | :--- | :--- |
| A3 | `github.go:224-234` | `checkRedirect` scrubs the header of the source's current credential, not the one the redirected request carries. If a concurrent request's 401 refresh switches the header, an in-flight request's cross-origin redirect keeps the old custom header. Needs concurrent use of one source, which `Checker` allows. | R\*: `foreign origin saw: X-Old-Key=old-secret` |
| A4 | `github.go:801-809` | A 403 "secondary rate limit" with neither `Retry-After` nor `X-RateLimit-Reset` maps to a plain `github http 403`, so `CheckCached` has no back-off and calls on every start. | R\*: `isRateLimited=false calls=3` |
| A5 | `checkcache.go:226-240` | A secondary limit's `Retry-After: 60` is stretched to the primary window's `X-RateLimit-Reset` even with `Remaining > 0`. Documented (0010-PLAN-v1-5-1 D1): a design question. | R\*: `NotBefore - now = 49m59s` |
| A6 | `github.go:536-537,556-572` | Asset requests let Go's transport ask for gzip and decode it transparently. A `.gz` or `.tar.gz` asset served with `Content-Encoding: gzip` by a CDN fails the size and digest checks: closed, but uninstallable. | R\*: `advertised=47 received=1400 (decoded=true)` |
| A9 | `github.go:138,669` | `NewGitHubSource` and `WithCredentials` replace the caller's `Client.CheckRedirect` in their clone; `GitHubOptions.Client` says only that it is cloned. | C\* |
| B4 | `managed.go:170-186`, `session.go:195-199,329-337` | Managed recovery reports `RolledBack=false` when the old binary is back: after `Apply` undid a directory swap, or `Rollback` restored and only its sync failed. `rollbackInRoot` also returns the backup's name after a successful rename, so `Run` could say "kept at" a path that no longer exists. 0010 B9's class, fixed only on the standalone path. | R\*: `target="old-bytes" RolledBack=false Backup=""` |
| B5 | `session.go:164-180` | With `AllowSpecialModeBits` and `KeepPrevious`, `.<base>.previous` keeps setuid or setgid at a predictable path: the vulnerable copy an update replaces stays privileged. Undocumented. | R\*: `previous=.demo.previous mode=urwxr-xr-x` |
| B6 | `probe.go:101-107` | When the run's deadline or cancellation ends a probe, the error wraps neither context error: it reads as the prober's own timeout, and `EventFailed.Detail` is `error`. | R\*: parent deadline 100 ms: `… timed out after 10s isDeadline=false` |
| B7 | `session.go:279-304` | `TwoPhaseSession.Commit` after `Rollback` of the same state reports `Applied=true` with the old binary in place. | R\*: `commit after rollback: target="old-bytes" Applied=true` |
| B8 | `cleanup_windows.go:267`, `leftovers.go:22-36` | The Windows receipt's temporary file, `.<base>.selfupdate.cleanup-tmp-<n>`, matches no leftover pattern; a crash between create and rename leaves it forever. | C\* |
| B9 | `imageverify.go:139-140` | The PE check requires `IMAGE_FILE_EXECUTABLE_IMAGE`, which DLLs also set, and not `IMAGE_FILE_DLL` clear. Mach-O requires `MH_EXECUTE`. | C\* |
| C3 (= D9) | `cli/run.go:123-132`, extending guide (handoff) | `Detach` runs for any apply, before discovery and whatever `req.Yes` says. An interactive `prog update` inside the service hands off and exits 0 without asking; the detached copy cannot prompt and fails with `ErrConfirmationRequired`. An up-to-date `--yes` run also hands off. The guide says the update "must have `--yes`", but nothing enforces it. | R: `Detach called=true yes=false exit=0 stderr="update handed off: abc"` |
| C5 | `updater.go:187-196`, extending guide, doc.go | The guide says every run has one terminal event, `complete`, `failed` or `declined`. An up-to-date apply, an up-to-date check and a check that finds an update end at `selected`. | R\*: `kinds=[resolving-target fetching-release selected]` |
| C6 | `updater.go:139-162`, `cli/testdata/golden/failed.json.stdout` | A run that fails in discovery returns `Result{}`, so the `--json` object says `"product":""` and `"checked":false` for a `--check` run, while the events name the product and the early-failure path fills it. | C\*: the committed golden |
| C7 | `reporter.go:27`, `jsonreporter.go:39`, `confirmer.go:138` | `NewTextReporter`, `NewJSONReporter` and `NewPromptConfirmer`'s writer accept a typed nil and panic later, including through `cli.Options.Stdout` and `Stderr`. 0003 C3's class. | R\*: `panic: runtime error: invalid memory address` |
| C8 | `runoptions.go:110`, `checker.go:123`, `stream.go:266` | Zero values of the exported `Updater`, `Checker` and `Stream` panic; `Stream.Cancel` is documented as callable any number of times from any goroutine. 0010 C9 fixed this for the reply types only. | R\* |
| C9 | `updater.go:334` | A dry run for another platform (0010 Q4) still runs staged probes, which execute the foreign binary and fail. | — |
| D7 | `service/launchd/lifecycle.go:63-75` | `Enabled` reads any non-zero `plutil` exit as "key missing", so an unreadable or corrupt plist is "not enabled" with no error, and a stopped, enabled job is left stopped. `RunAtLoad` `<integer>0</integer>` counts as true. | R\*: mode-0000 plist: `Enabled=false,<nil>` |
| D8 | `service/systemd/detach.go:182-189` | `writeEnvFile` deletes every `handoff-*.env` in the shared `/run/selfupdate`, including another unit's that is still starting; on systemd 236–239 a concurrent handoff of another product can fail. | C\* |
| D10 | `service/poll.go`, the backends | Nothing refuses a settle window longer than the timeout; with a short `Poll.Timeout` every update rolls back on `ErrTimeout`. The stop wait's timeout says "not healthy within …". | C\* |
| D11 | `service/systemd/unit.go:237` | Any message containing "not found" maps to `ErrNotInstalled`, including a start that fails on a missing dependency. | C\* |
| E3 | `archive/unpack.go:293-296,348-351` | A zip entry with the MS-DOS directory attribute and data, without a trailing `/`, is skipped as a directory; Info-ZIP `unzip` extracts it as a file. A tar regular entry `relay/` is accepted as the program; bsdtar makes a directory. Both also slip past "more than one program". | C; R\* |
| E4 | `archive/unpack.go:352-354` | Encrypted zip entries (`Flags` bit 0 or 6) are not refused; a stored one with CRC 0 installs its ciphertext. | C; R\*: `ciphertext-that-go-installs-as-is err=nil`; `unzip` exit 82 |
| E5 | `releasespec/validate.go:61-99`, `spec.go:257-272` vs `archive/select.go:122-126`; `check.go:46-56` | `Parse` caps the product name at 128 but never the composed asset name; the archive selector refuses names over 128, and the publish check never runs `Select`. A valid spec can publish a release the client refuses, against the guide's §7. | R\*: a 120-character product: `Select` fails, `invalid asset name` (139 characters) |
| E6 | `codesign/codesign.go:124-129,175-181` | `SignOptions.Requirement` is pasted into `identifier "X" and (<Requirement>)` with no check that its parentheses balance: `anchor apple) or (always` cancels the identifier pin 0012 §5 promises. The value is the program's own configuration. | R\*: `csreq … -t`: `identifier "com.example.relay" and anchor apple or always` |
| E7 | `releasespec/installer.go:30-32`, `spec.go:108-129` | `"installer": null`, `"extras": null` and similar parse as absent, while `Installer`'s godoc says "Present, even empty, it turns them on". | R\* |
| E8 | `archive/unpack.go:293-298` | A PAX global header (`git archive` output) is refused as "not a regular file or a directory". Safe, but not in the documented refusal list, while the package doc says other tooling's archives work. | R\* |
| F5 | `install.ps1:116,153,168-171,229-235,318-320,369-371` | A relative `-InstallDir` is never made absolute: .NET calls resolve it against the process's directory and cmdlets against `$PWD`, and `Add-UserPath` can write the relative entry. `install.sh --dir bin` advises `export PATH="bin:$PATH"`. | R\* (the mechanism, with pwsh) |
| F6 | `installer/install.sh:123-133` | `sha256sum` and `shasum` escape a file name with a backslash, printing `\<hash>`, so an install directory containing `\` always fails verification (exit 2). | R\* |
| F7 | `installer/install.sh:153,209-211,258-265,358-367,412-414` | Edge cases: a `--version` with a newline passes the tag rule line by line; `--product ''` installs nothing and exits 0; a failed identity check "restores" a stale `.prev` from an earlier run; a directory at the target is executed as a hook. | R\* |
| F8 | `scripts/release-latest-flag.sh:53-62` | A repository whose current latest release is not `vX.Y.Z` (before migrating) cannot publish, and the error does not say what to do. | R\* |
| F9 | `publish-selfupdate-release.yml:39-41` | No `concurrency` group: two tag runs can race the latest-flag rule, and two runs of one tag can each pass `refuse-existing-release` and create two drafts. | — |
| G4 | `service/execreconciler.go:45,47` | `Receipt.Reloaded` and `Receipt.Warnings` are decoded and never read: a child's warnings never reach `Result.Warnings`. | C\* |
| G5 | `types.go:598` | godoc names `Installer.Install`; the method is `InstallSession.Install`. | C\* |
| G6 | `doc.go` (Integrity), `releasespec/doc.go:1-4` | The package doc says verifiers run on the staged binary and `NewImageVerifier` checks it; since `v1.8.0`, with an Unpacker they see the archive and `New` refuses `NewImageVerifier`. The releasespec doc omits `installer`. | C\* |
| G7 | `internal/cmd/selfupdate-release/main.go:13` | `stage`'s usage line omits `-repository`, which a spec with `installer` requires. | C\* |
| G8 | migration guide :531-532, extending guide :116, `docs/README.md:67` | "This repository's release workflow still publishes bare binaries only" (false since `v1.9.0`); "six whole runs" (seven); crashers go to `selfupdate/testdata/fuzz` (three packages fuzz now). | C\* |
| G9 | `docs/architecture.md:40-61,156-157,286-288,333-336,410-424` | Stale or incomplete: the cache file is "schema 2 … reads schema 1" (it is 3, read only); `release-latest-flag.sh`, its test and the "ownership as root" step are missing; build step 4 omits `GOWORK=off`; the publish step omits the backport latest-flag rule. (Merges A8.) | C |
| G10 | `archive`, `cli`, `codesign` Examples | 0010 recorded "every `Example` has an output check". Four do not: `ExampleNewUnpacker` could; `ExampleHandOff` and the two codesign Examples cannot by design. | C\* |

### Gaps

| ID | Where | Gap | Evidence |
| :--- | :--- | :--- | :--- |
| A7 | `checkcache.go:167-169` | 0010 A4's "a stored `NotBefore` beyond the cap is a miss" has no killing test: disabling it leaves the package green. Of 21 planted breaks in area A, it is the only survivor. | R\*: `stored-deferral-cap: SURVIVED` |
| B10 | `target.go:79-93`, `managed.go:69` | Two of 0010 B14's coverage gaps are open, though 0010-PLAN-v1-5-1 P6 says they were closed: the default `os.Executable` path and a relative `ExecutablePath` (35.7 %), and `managedSession.Target` (0 %). | R\*: module-wide coverage |
| G1 | `0004-MADR…:255-263,1088-1094`, `docs/README.md:83` | 0004 still says "the rest of Phase 4 stays planned, and none of it is built", and its §1 table misdescribes `archive`, `cli` and `codesign`. Phase 4 shipped in 0011–0014; the work still open (`verify/signednote`, `verify/ghattest`, `gitlab`, `httpmanifest`, `go-tui-lib/updatetea`, 0011's "replace before stop", 0012 §8's other formats) is listed only inside single records. | C\* |
| G2 | 0010–0014 PLANs | The PLANs' rules and execution records rest on `gate.sh`, `doccheck.py`, `plantcopy.py` and `simulate_build.sh`, which exist only in the agent's session scratchpad. About 40 "`gate.sh`, every step rc 0" entries cannot be re-run from the repository. | C: none of the four is in the tree |
| G3 | `0011-MADR…:555-558`, `0011-PLAN…:408-410` | 0011's Confirmation promises a live handoff test in which "a second run with a failing health check reports the rollback in its result". None exists, the PLAN's V4 dropped it with no deviation entry, and neither `ResultDocument` nor `Result` has a rolled-back field to report. | C\* |

### Checked and fine

* **0003's and 0010's fixes, re-checked in each area** by code, test and,
  in area A, 21 planted breaks (20 killed; A7 is the survivor): tag
  pinning, https-only redirects and their loopback rule, credential and
  custom-header scrubbing, per-run anonymity, digest-at-size, the
  foreign-origin 401 rule, the `NotBefore` clamp, verifier errors wrapping
  `ErrIntegrity`; the lock's `Lstat`/`O_EXCL`/`SameFile`, receipt
  validation, staging deregistered after the rename, the copy fallback's
  mode, the bounded busy-image retry, leftovers swept by strict name, the
  mode policy; typed-nil collaborators, `errNotCommitted`, EOF as a
  decline, event order, `Close` before `complete`, errors after
  `complete` as warnings, `Command`'s result object for refused options.
* **Integrity:** `SHA256SUMS` parsing (the Python differential passes),
  constant-time digest comparison, `copyLimited`, exact selection with
  duplicate refusal, the channel grammar and `Admits`.
* **Stream and options:** four concurrent `Next` consumers plus `Cancel`
  under `-race` delivered `Finished` once; options apply in order and
  refuse typed nils.
* **Service backends:** unit and label grammar, cgroup v1 and v2, the
  version gates, `EnvironmentFile` quoting, `sd_notify`, `launchctl` exit
  mapping and bootstrap retry, SCM least-privilege opens and the
  wait-hint loop, the handoff file modes and size caps. The timing-heavy
  tests ran 10–15 times in parallel with no flake; no test depends on host
  PIDs beyond the one fixed in 0014 D9.
* **Archives:** path traversal, links, devices, sparse entries, bombs, the
  zip overlap check and single-member `.gz` are refused; each fuzz target
  ran 30 s clean and does what its comment says.
* **Release tooling:** no `${{ }}` reaches a `run:` block; tag-rule parity
  with `NewSemverPolicy` on 20 edge tags; the publish order and its
  latest-flag rules; the build recipe against its checks; the installers'
  HTTPS-only fetches, strict `SHA256SUMS`, spaces and symlinks in the
  install directory, and a read-only directory.
* **Documentation:** every Go snippet in the three guides compiles for
  three targets (including the cobra recipe); every backticked API name
  resolves; all 27 Examples pass; every link and anchor resolves; the
  package table, depguard's 14 rules (planted imports fail the expected
  rule), the Makefile, the pins and the workflow inputs match.

## Decision Drivers

* **Two High findings lose a binary or leave a service down.** They should
  ship at once, in a patch release.
* **Most Medium findings are a fix that keeps a documented contract.** A
  patch release must stay `compatible with v1.10.0` under `make apicheck`.
* **Some findings need the owner to pick a contract first:** C3, C5, C8,
  C9, A5, B5, B7, D6, D10, E7, E8, F3, F4's pre-check, G2, G3.
* **Consumers pin the workflows and the installers by the commit of a
  release tag.** A fix to the templates, the release tool or a workflow
  reaches them only through a tagged release, so those fixes ride the
  patch release, not `main` alone.
* **Records and documentation ship with no tag** and change on `main`.
* **Every fix comes with the test that would have caught it,** seen to
  fail on `HEAD` first (0003's and 0010's practice).
* **The owner prefers a complete, extensible surface** to cuts made only
  for scope.

## Considered Options

* **A. Three tracks:** records, documentation and the in-repository gate
  on `main`, with no release; `v1.10.1` for every fix that keeps the
  documented contract, the High findings first; `v1.11.0` for the
  contracts the owner decides.
* **B. Everything in one `v1.11.0`.**
* **C. High and Medium only;** Low findings and gaps recorded and left.
* **D. Record the findings, and fix nothing now.**

## Decision Outcome

Chosen option: **"A. Three tracks"**, because it ships the two High fixes and
every contract-preserving fix at once, without waiting on the contract
decisions, and keeps `v1.10.1` provably compatible so a program can take
it with no code change.

### 1. On `main`, with no release

* **G1:** an amendment to
  [0004-MADR-evolve-selfupdate-api-and-tui-support.md](0004-MADR-evolve-selfupdate-api-and-tui-support.md)
  that closes Phase 4 against 0011–0014, corrects its §1 rows, and lists
  the open work in one place; `docs/README.md` points there.
* **G3:** an amendment to 0011 restating its Confirmation item, with the
  `v1.11.0` decision below.
* **G8, G9, G10, A9's documentation, F4's documentation** (the immutable
  releases setting and the attestation prerequisite in the building
  guide's step 4 and the README), and **F3's documentation** (step 12
  names the `v1.10.0` floor for the program's module).
* **G2:** the gate in the repository, per the owner's answer below.

### 2. `v1.10.1`: every fix that keeps the documented contract

* **High:** B1 (a kept backup leaves the leftover pattern, or the sweep
  honours a keep-list on every OS, and a dry run sweeps nothing); B3 (a
  `Stop` error after the stop was issued recovers with `Start` and
  `WaitHealthy`, and launchd's stop wait follows `ExitTimeOut` plus 30 s).
* **Network:** A1 (both selectors wrap `ErrUnsupportedPlatform`), A2 (an
  env-token fallback reached after the provider declined is per run), A3
  (a cross-origin hop keeps only the source's fixed headers), A4, A6.
* **Install:** B2 (`replaceLocked` requires the locked target's identity
  and a regular file), B4, B6, B8, B9; the A7 and B10 tests.
* **API and CLI:** C1, C2, C4, C6, C7.
* **Services:** D2 (fail only on an increase after the start), D3, D4
  (verify the effective `ExecStart` after the reload), D5, D7, D8, D11.
* **Archives and spec:** E1, E2, E3, E4 (each a refusal the documented
  rule already requires), E5 (validate composed names, or `check` runs
  `Select`), E6.
* **Installers and tooling:** F1, F2, F5, F6, F7, F8, F9, G4 (surface
  receipt warnings), G5, G6, G7.

### 3. `v1.11.0`: the contracts the owner decides

The questions below. Each answer is recorded as an amendment here before
its code is written, and lands in `v1.11.0` (or `v1.10.1` where the answer
keeps every documented contract).

### 4. Owner answers

Answered 2026-10-07: every recommended answer below. Track 3, `v1.11.0`,
is therefore C3, C8, C9, G3, A5, B5, D6, D10, E7, E8 and F3's check,
plus two items amendment A1 below moves or adds: G4, and A1's release
that does not exist yet. C5's answer is documentation, on `main`. F4's is
documentation and the timeout message, in tracks 1 and 2. B7 moves to
`v1.10.1` (amendment A1). The PLAN,
[0015-PLAN-remediate-third-debugging-pass-findings.md](0015-PLAN-remediate-third-debugging-pass-findings.md),
states each answer as a row of its "Decisions this PLAN assumes".

### Consequences

* Good, because the two High findings and every contract-preserving fix
  ship in one patch, with the test that would have caught each.
* Good, because the workflow and installer fixes reach consumers through
  the tag they pin.
* Neutral, because several fixes make the library refuse more: archives
  two tools read differently (E1–E4), a spec whose asset names the client
  refuses (E5), a requirement that unbalances the identifier pin (E6).
  Each refusal is one the documentation already promises.
* Bad, because `v1.10.1` is broad: 43 fixes across every package,
  each with its own test, a PLAN of several phases.
* Bad, because the contract questions hold back `v1.11.0` until answered.

### Confirmation

* Each fix's test is seen to fail on `HEAD` `18e0575` and pass after the
  fix, recorded per phase with its failure text.
* `make apicheck` reports `v1.10.1` compatible with `v1.10.0`.
* B1: a kept backup survives `CleanupPending`, a later `Run` and a dry
  run, on Unix and Windows. B3: a `Stop` that fails after stopping leaves
  the service running, under each backend's fake and the live tests.
* The live tests of 0011 and the rehearsal of 0014 pass on the three test
  hosts and CI before the tag.
* A re-run of this pass's probes, kept with the PLAN, shows each finding
  fixed or decided.

## Pros and Cons of the Options

### A. Three tracks

* Good, because the High findings ship without waiting on decisions.
* Good, because a patch release that keeps every contract needs no
  consumer change.
* Bad, because three tracks are more release work than one.

### B. Everything in one `v1.11.0`

* Good, because one release, one migration section.
* Bad, because the High fixes wait on every contract question.

### C. High and Medium only

* Good, because it is smaller.
* Bad, because the Low findings include cheap fixes to security-flavoured
  edges (A3, B5, E4, E6) and to installers users run directly (F5–F7).
* Bad, because it repeats the gaps a fourth pass would find again.

### D. Record only

* Good, because it costs nothing now.
* Bad, because B1 and B3 stay live in every program on `v1.10.0`.

## Owner questions

**Answered (2026-10-07).** Asked one by one, the owner chose the
recommended answer to each of the 15 questions, then approved the PLAN
("proceed"). The answers, as the PLAN applies them, are rows Q1–Q15 of
its "Decisions this PLAN assumes"; approving the PLAN also accepted its
rows N1–N9. Q7 lands in `v1.10.1`, Q11 refuses `size` and `GNU.sparse.*`
records too (N3), and Q12's check is the spec's per-field floor
(amendment A1).

Recommended answers first. The answers decide the `v1.11.0` scope.

1. **C3: a handoff without `--yes`.** Refuse it before detaching, with
   `ErrConfirmationRequired` and a message to add `--yes` (recommended);
   or ask here first, then hand off with `Yes`; or document only. Also:
   hand off only after discovery finds an operation (recommended), so an
   up-to-date run never detaches.
2. **C5: a run that ends at `selected`.** Document the exception, that an
   up-to-date or check-only run's last event is `selected` with its
   operation (recommended); or emit `complete` with `Detail`
   `up-to-date` or `update-available`, a new event a consumer may not
   expect.
3. **C8: zero values.** Return a "not constructed" error from `Run`,
   `Check` and `Start`, and make a zero `Stream.Cancel` a no-op
   (recommended); or document that the types must be constructed.
4. **C9: probes on a dry run for another platform.** Skip them and say so
   in the result (recommended); or refuse such a dry run when probes are
   configured.
5. **A5: a secondary limit's back-off.** Use `X-RateLimit-Reset` only when
   `Remaining` is 0 (recommended), changing 0010-PLAN-v1-5-1 D1; or keep
   it.
6. **B5: a privileged `.previous`.** Clear setuid and setgid on
   `.previous` and on a kept backup (recommended); or document it.
7. **B7: a second `Commit` or `Rollback` of one replacement.** Refuse it
   (recommended); or document that the state is single-use.
8. **D6: `StopDependents`.** Record the dependents `Stop` stopped and
   start them again in `Start` and in recovery (recommended); or document
   that the caller restarts them.
9. **D10: a settle window longer than the timeout.** Refuse the options
   (recommended); or clamp the settle window.
10. **E7: `null` in a spec.** Refuse `null` for every field (recommended,
    so a typo cannot turn the installers off); or document that `null`
    means absent.
11. **E8: PAX global headers.** Skip a global header that carries no
    `path` or `linkpath` record (recommended, so `git archive` tarballs
    work); or refuse it on purpose and document it.
12. **F3: the caller's module version.** `plan` refuses a module whose
    go-selfupdate-lib requirement is older than the tool's
    (recommended); or document the floor only (track 1 does that either
    way).
13. **F4: immutable releases.** Name the setting in the timeout error and
    the docs (recommended, in track 1 and `v1.10.1`); a check before
    publishing needs Administration read, which `GITHUB_TOKEN` likely
    lacks (not confirmed).
14. **G2: the gate.** Commit `gate.sh`, the document checker and the
    plant helper under `scripts/`, with tests, and have the PLANs name
    them (recommended); or reword the PLANs to name `make pre-add-check`,
    `make lint` and CI only.
15. **G3: a rolled-back field.** Add `RolledBack` to `Result` and its
    document (schema 3), and the live handoff test 0011 promised
    (recommended, in `v1.11.0`); or amend 0011 to drop the item.

## Amendments

### A1 (2026-10-07): corrections found while planning

*Status: accepted with the PLAN (2026-10-07).* Planning read every finding
against the code at `d6a570f`. These corrections change no finding's
severity; each is applied in the PLAN, and the finding tables above are
left as written.

* **B7 moves to `v1.10.1`.** B1's kept backup needs the replacement's state
  in `Rollback` and in managed recovery, which B7's state provides. B7 adds
  no exported identifier.
* **G4 moves to `v1.11.0`:** reporting a reconciler's warnings needs a new
  exported field, `ReconcileResult.Warnings`.
* **A1 splits.** The selectors wrap `ErrUnsupportedPlatform` in `v1.10.1`.
  Caching a release that does not exist yet needs a new sentinel,
  `ErrNoRelease`, and outcome, `CheckNoRelease`, in `v1.11.0`.
* **C9 is reproduced** (the table's "—"): a staged probe ran on another
  platform's binary and failed with `exec format error`.
* **F3's check is the spec's per-field floor** (`installer` needs
  `v1.10.0`), not the tool's own version. The tool is built from a
  tagless checkout, so its version reads as a pseudo-version or
  `(devel)`. CI's rehearsal fixture replaces the library with a
  directory, which is not checked.
* **F7(a) and F7(c) also hold in `install.ps1`.** `Test-ReleaseTag`
  (`:129`) ends its pattern with `$`, which .NET matches before a final
  newline. The identity failure's restore (`:411-423`) moves a stale
  `<name>.exe.prev` into place when no `<name>.exe` existed.
* **F7(d) needs two rules:** a hook runs only a regular file, and the
  install refuses a directory at a product's path before changing
  anything.
* **Citations:** G7 is `main.go:11`, not `:13`; G5 is `types.go:598`; C1 is
  `cli/run.go:142-157`. The cli goldens hold 13 `*.json.stdout` files.
* **F4:** the README's "Its tag can never be reused" is false when
  immutable releases are off.
* **E5:** `pack` writes tar.gz as USTAR (`pack.go:43`), so a program name
  over 100 characters cannot be packed at all. `Validate` refuses it.
* **D2's external fact** is pinned by the fix's rule, not relied on: the
  baseline is read after the start, whatever systemd does with the
  counter.
* **D7, probe evidence** (macOS 26.6.2, `/usr/bin/plutil`, 2026-10-07).
  * `plutil -lint` passes a file holding only `garbage`: a bare word is a
    valid old-style property list.
  * `plutil -convert xml1 -o -` prints its root as
    `<string>garbage</string>`.
  * `-type <key>` prints `integer` for `<integer>0</integer>`, and exits 1
    with "No value at that key path" for a missing key or a non-dictionary
    root.
  * `-extract <key> raw -expect bool` exits 1 for an integer.
  * A missing file and a mode-0000 file each exit 1, with a message.

  The readable-plist check therefore converts to XML and requires a
  dictionary root. A live test pins launchd's own reading of an integer
  `RunAtLoad`.
* **B3's bound** is the longer of the poll timeout and the service
  manager's own kill bound: launchd `ExitTimeOut`, 5 s when absent (0011's
  probe evidence), plus 30 s; systemd `TimeoutStopUSec` plus 30 s. A stop
  never waits less than it does today.

## More Information

* Earlier passes:
  [0003-MADR-remediate-debugging-pass-findings.md](0003-MADR-remediate-debugging-pass-findings.md)
  and
  [0010-MADR-remediate-second-debugging-pass-findings.md](0010-MADR-remediate-second-debugging-pass-findings.md).
* The records whose code most findings concern:
  [0011-MADR-reference-service-lifecycles.md](0011-MADR-reference-service-lifecycles.md),
  [0012-MADR-archive-assets-and-macos-codesign.md](0012-MADR-archive-assets-and-macos-codesign.md),
  [0013-MADR-build-and-stage-release-workflow.md](0013-MADR-build-and-stage-release-workflow.md),
  [0014-MADR-shared-installer-templates.md](0014-MADR-shared-installer-templates.md).
* External fact: systemd `src/core/service.c:3623-3625` (`main`, read
  2026-10-07) resets `n_restarts` on a start that is not an automatic
  restart (D2).
* The reviewers' probes were not committed: the PLAN that follows this
  record turns each into the test its fix needs.
* The PLAN:
  [0015-PLAN-remediate-third-debugging-pass-findings.md](0015-PLAN-remediate-third-debugging-pass-findings.md).
