---
status: in-progress
date: 2026-10-08
associated-madr: "0018-MADR-move-toolchain-to-go-1-27-2.md"
---
# Implement the Go 1.27.2 toolchain: the hosts, a `toolchain` line, and `v1.11.1` from a release branch

Associated MADR: [0018-MADR-move-toolchain-to-go-1-27-2.md](0018-MADR-move-toolchain-to-go-1-27-2.md)

## Goal

* **Every build this module controls uses Go 1.27.2:** this Mac, the
  Linux test host, the Windows test host and its default WSL
  distribution, CI, and the publish workflow's release tools.
* **`go.mod` keeps `go 1.27.1`** as consumers' floor.
* **`v1.11.1` is tagged on a `release/v1.11.1` branch commit** holding
  `50eafb6` and the toolchain change only. Its CI and its tag's CI pass.
* **[0017-PLAN-verify-build-provenance-and-close-0015-open-items.md](0017-PLAN-verify-build-provenance-and-close-0015-open-items.md)
  resumes at Q2,** with `make gate` green.

## Scope

### In scope

| Phase | What | Where |
| :--- | :--- | :--- |
| T0 | the records: this pair, the index, 0017's Deviation D4 | `main`, records only |
| T1 | Go 1.27.2 and the two tools on the four hosts | outside the tree |
| T2 | `go.mod` `toolchain go1.27.2`; `ci.yml` runs on `release/**` | `main`, one commit |
| T3 | the docs that name the toolchain | `main` |
| T4 | `release/v1.11.1`, its CI, and 0017's release procedure for `v1.11.1` | the branch, the tag, the pins on `main` |

### Out of scope

* **0169 D2 in magic-cli-remote,** and the fleet's standard file (0169
  D11). The owner amends them there.
* **Other repositories' `go.mod` files.**
* **Removing Go 1.27.1** from any host: nothing is deleted.
* **Push and tags,** which happen only on the owner's ask in the same
  turn.

## Rules

0017-PLAN's "Rules for every phase" and its "Phase procedure" apply. Its
rule 7 changes here: `go.mod` gains a `toolchain` line in T2, and
`go mod tidy -diff` stays clean. Host changes are recorded:
* before and after;
* by role ("this Mac", "the Linux test host", "the Windows test host",
  "its WSL distribution");
* with `<user>` paths.

## Implementation Steps

### Phase T0: records

**Files:**
* `docs/decisions/0018-MADR-move-toolchain-to-go-1-27-2.md`
* `docs/decisions/0018-PLAN-move-toolchain-to-go-1-27-2.md`
* `docs/README.md`
* `docs/decisions/0017-PLAN-verify-build-provenance-and-close-0015-open-items.md`

1. **0017-PLAN gains `### Deviation D4 (2026-10-08)`:**
   * Q2's gate failed `vuln`, with the ten advisories;
   * the owner chose this pair's option A and the release branch;
   * 0017's release procedure for `v1.11.1` applies to the branch
     commit;
   * 0017 Q2 resumes after this PLAN's T2.
2. **`docs/README.md`** gains the two 0018 rows.
3. **This PLAN:** `status: in-progress`, and an Approval entry.

### Phase T1: the hosts

*The hosts are moved by the owner's dotfiles repository, under its own
records: `docs/decisions/0011-MADR-upgrade-homedir-go-toolchain-to-1.27.2.md`
and its PLAN (P2 for this Mac, P3 for the Linux test host, P4 for the
Windows test host). That plan also sets the Linux host's `PATH` line and
reinstalls eleven Go tools. This Mac and the WSL distribution were on
1.27.2 on 2026-10-08. The agent first answered "Yes, move them" for the
two test hosts, then found that plan under way, and the owner chose that
it moves them ("Dotfiles plan moves them"). Steps 1–6 below are therefore
not run by this PLAN. The agent runs step 7's checks, read-only, on each
host once the owner says P3 and P4 are done.*

For each host, in this order: this Mac, the Linux test host, the Windows
test host, its default WSL distribution.

1. **Inventory**, recorded:
   * `go version`;
   * `go env GOROOT GOENV GOTOOLCHAIN`;
   * how `go` is found: `command -v go` and `readlink -f`, the symlink or
     the `PATH` entry and the file that sets it;
   * each `golangci-lint` and `govulncheck` on `PATH`, with `version`.
2. **Download** `go1.27.2.<os>-<arch>` (`.tar.gz`, or `.zip` on Windows)
   from `https://go.dev/dl/`.
   * Its SHA-256 must equal the `sha256` that
     `https://go.dev/dl/?mode=json` lists for that file.
   * A mismatch stops the phase.
3. **Install it beside 1.27.1,** under the same parent:
   * `~/.local/go1.27.2` on this Mac;
   * `~/sdk/go1.27.2` on the others.

   Nothing is removed.
4. **Point `go` at it** by the same mechanism step 1 found:
   * the symlink (`~/.local/bin/go`, and `gofmt` beside it) is repointed;
   * or the `PATH` entry naming `go1.27.1/bin` is changed to
     `go1.27.2/bin`. On Windows that is the user `PATH`; its raw value and
     kind are saved first, as the 0014 rehearsal did.
5. **Set the toolchain:** `go env -w GOTOOLCHAIN=go1.27.2`.
6. **Rebuild the tools** into the directories step 1 found, with
   `GOBIN=<dir> go install`:
   * `github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.14.0`;
   * `golang.org/x/vuln/cmd/govulncheck@v1.8.0`.
7. **Check:**
   * `go version` gives `go1.27.2`;
   * `golangci-lint version` names `go1.27.2`;
   * `govulncheck -version` gives `Scanner: govulncheck@v1.8.0`;
   * on a scratch copy of the tree at `main`, `govulncheck ./...` reports
     `No vulnerabilities found.`

### Phase T2: the `toolchain` line, and CI on release branches

**Files:**
* `go.mod`
* `.github/workflows/ci.yml`

1. **`go.mod`:** after `go 1.27.1`, the line `toolchain go1.27.2`.
2. **`ci.yml`:** `on.push.branches` becomes `[main, 'release/**']`.
3. **Red:** 0017 Q2's gate log, `vuln rc=2`, with the ten advisories, on
   Go 1.27.1. A pin cannot be shown failing on a broken input the way a
   test can (0007-PLAN step 1, check 3). Its check is CI's setup-go log,
   which names `1.27.2`, and CI's `govulncheck` step.
4. **Checks:**
   * `make gate` ends `overall=0` with `vuln rc=0`. 0017 Q2's files are
     still uncommitted in the tree, and are not staged;
   * `go mod tidy -diff` is clean;
   * actionlint v1.7.12 and `scripts/check-workflows.sh` pass on `ci.yml`.
5. **Commit** on `main`, with the two files only, so that T4 can
   cherry-pick it whole.

### Phase T3: the docs that name the toolchain

**Files:**
* `AGENTS.md`
* `docs/architecture.md`
* `README.md`

1. **`AGENTS.md`'s "Requires Go 1.27.1."** becomes: "Requires Go 1.27.1;
   built and tested with Go 1.27.2 (the `toolchain` line, 0018-MADR)."
2. **`docs/architecture.md` and `README.md`** say the same where they name
   the Go version. `git grep -n '1\.27\.1' -- AGENTS.md README.md
   docs/architecture.md docs/guides` lists the candidates, and each
   changed line is named in the record.

   A guide's statement about a consumer's `go.mod` (`go 1.27.1` as the
   floor) stays.
3. **Checks:** links, ids, markdownlint, and `make gate`.

### Phase T4: `v1.11.1` from `release/v1.11.1`

1. **The branch,** in a separate worktree, so the main tree is not
   touched: `git worktree add <scratch> -b release/v1.11.1 50eafb6`, then
   `git cherry-pick <T2's commit>` there.

   The branch's commit holds `50eafb6` plus `go.mod`'s `toolchain` line
   and `ci.yml`'s trigger.
2. **On the owner's ask:** `git push origin release/v1.11.1`. CI runs on
   the branch, and must pass all jobs; its setup-go step names `1.27.2`.
3. **0017's release procedure, steps 3–6,** with `vX = v1.11.1` and
   `<commit>` the branch's commit:
   * the tag and the disclosure guard;
   * `ls-remote` and the tag's CI identity legs;
   * the pin commit on `main`, pinning the branch's commit;
   * the proxy check.
4. **After the tag:** the branch is kept. Deleting it is the owner's call,
   and nothing depends on it once the tag exists. The worktree is
   removed.
5. **The record** names each commit, run and line.

Then 0017 resumes at Q2: its gate runs again, and Q2 commits.

## Verification

* **V1.** `go version` is `go1.27.2` on all four hosts.
  `golangci-lint version` and `govulncheck -version` are as T1 step 7
  says.
* **V2.** `make gate` ends `overall=0`, with `vuln rc=0`, on this Mac at
  T2 and T3.
* **V3.** CI passes on `main` at T2's commit and on the
  `release/v1.11.1` commit. Its setup-go log names 1.27.2.
* **V4.** The `v1.11.1` tag's CI passes all jobs. Its identity legs print
  `v1.11.1 (release) <12-hex>` of the branch commit.
* **V5.** `git diff 50eafb6 v1.11.1` shows only `go.mod`'s
  `toolchain` line and `ci.yml`'s trigger.

## Rollout and Rollback

* **The hosts keep Go 1.27.1.** Pointing the symlink, or the `PATH`
  entry, and `GOTOOLCHAIN` back restores them.
* **A consumer is unaffected** until it moves its own toolchain.
* **Releases are immutable.** A fix is a new patch release.

## Execution Record

### Approval (2026-10-08)

The owner approved execution: "Update to 1.27.2". Asked who moves the two
test hosts that were still on `go1.27.1`, the owner answered "Yes, move
them". On that day:
* this Mac and the WSL distribution reported `go1.27.2`, with
  `GOTOOLCHAIN=go1.27.2`;
* the Linux test host and the Windows test host reported `go1.27.1`.

### Phase T0: records (2026-10-08)

* This pair, and the two rows in `docs/README.md`.
* 0017-PLAN gains its Q2 record and Deviation D4.

### Phase T1: the hosts (2026-10-08)

* **This Mac:** `go version go1.27.2 darwin/arm64`, `GOTOOLCHAIN` `go1.27.2`,
  `~/.local/bin/go` → `~/.local/go1.27.2/bin/go`. The owner moved it,
  under the dotfiles plan's P2.
* **The WSL distribution:** `go1.27.2`, `GOTOOLCHAIN` `go1.27.2`.
* **The Linux test host and the Windows test host:** still `go1.27.1` on
  2026-10-08. The dotfiles plan's P3 and P4 move them, and step 7's checks
  wait for the owner to say they are done.

### Phase T2: the `toolchain` line, and CI on release branches (2026-10-08)

* **Commit:** `355dd3d`, holding the two files only:
  * `go.mod` gains `toolchain go1.27.2`, after `go 1.27.1`;
  * `ci.yml`'s `on.push.branches` is `[main, 'release/**']`, with a
    comment citing 0018-MADR.
* **Red:** 0017 Q2's gate, `vuln rc=2`, on Go 1.27.1.
* **Checks,** on this Mac with Go 1.27.2:
  * `go mod tidy -diff`: clean;
  * actionlint v1.7.12 on `ci.yml`: clean;
  * `scripts/check-workflows.sh`, as CI runs it: with no arguments, and
    with `--rule expressions`, `--rule permissions` and `--rule pins` on
    `ci.yml`, each `rc=0`;
  * `make gate`: every step `rc=0`, `vuln` "No vulnerabilities found.",
    `overall=0`.

### Phase T3: the docs that name the toolchain (2026-10-08)

* `git grep -n '1\.27\.1' -- AGENTS.md README.md docs/architecture.md docs/guides`
  listed seven lines. These changed:
  * `AGENTS.md:28`: "Requires Go 1.27.1; built and tested with Go 1.27.2,
    the `toolchain` line in `go.mod`", citing 0018-MADR;
  * `README.md:22`: the same, and "build your program with 1.27.2 too";
  * `docs/architecture.md:17`: the `toolchain` line, what it moves, and
    the ten advisories, linking 0018-MADR.
* **Kept:** the migration guide's four lines (`:14`, `:16`, `:18`, `:155`),
  which state a consumer's `go 1.27.1` floor.
