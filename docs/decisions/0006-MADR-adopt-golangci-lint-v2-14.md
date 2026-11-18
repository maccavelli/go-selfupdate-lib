---
status: accepted
date: 2026-10-01
decision-makers: owner
---
# Adopt golangci-lint v2.14.0, clearing its one finding at the source

## Context and Problem Statement

CI pins `golangci-lint` at `v2.13.2` (`.github/workflows/ci.yml`; the pin's
origin is
[0003-PLAN-remediate-debugging-pass-findings.md](0003-PLAN-remediate-debugging-pass-findings.md)).
`scripts/go-precheck.sh` names the same version in its install hint.

On 2026-10-01 the local binary was upgraded outside this repository's work,
to `v2.14.0`. That release is dated 2026-09-24, and it bumps the bundled
gosec from 2.28.0 to 2.29.0 (golangci-lint commit `f0a15fd`).

Since the upgrade, the pre-add gate (`make pre-add-check`, and the
machine-wide agent gate before a commit that stages Go files) refuses every
Go commit. A clean clone of `HEAD` fails the same way, on all three lint
targets, with one finding:

```text
selfupdate/target.go:127:23: G703: Path traversal via taint analysis (gosec)
```

### What G703 is, and where this finding comes from

* **The rule.** G703 is gosec 2.29's taint rule for path traversal
  (`analyzers/pathtraversal.go` at gosec `v2.29.0`).
  * **Sources:** `os.Args`, `os.Getenv`, `os.ReadFile`, and
    `*http.Request`, `*url.URL`, `*bufio.Reader` and `*bufio.Scanner`
    parameters.
  * **Sinks:** among others, `os.Lstat` and `os.Stat`.
  * **Sanitizers:** only `filepath.Base`, `filepath.Rel` and `path.Base`.
    `filepath.Clean` and `filepath.Abs` are "deliberately NOT sanitizers"
    (gosec issue #1721).
* **The sink** is `os.Lstat(abs)` in `canonicalizeRoot`, which validates a
  `TargetPolicy.AllowedRoots` entry. That entry is supplied by the embedding
  program, and the function is itself the validator: it requires a non-empty
  absolute path, stats it, resolves symlinks, re-checks the directory and
  refuses a filesystem root.
* **The source is a test,** not production code:
  `TestCanonicalizeRootRejectsFilesystemRoot` (`selfupdate/target_test.go:70`)
  builds the Windows root as `os.Getenv("SystemDrive") + "\"`. The repository
  lints with `tests: true`.
* **Evidence:**
  * linting `HEAD` with `--tests=false` reports `0 issues`;
  * the only `os.Getenv` in production code, `lookupEnv` in `github.go`,
    feeds tokens, not paths;
  * on a scratch clone, building that root from
    `filepath.VolumeName(t.TempDir())` instead clears the finding on `linux`,
    `darwin` and `windows`, with `0 issues`;
  * adding `filepath.Clean`, or a `..` check, before the sink does not
    clear it, as the rule's sanitizer list predicts.

## Decision Drivers

* **Local gates and CI must run the same linter,** or a green local check
  means nothing for CI (0002-MADR §5).
* **No exemption for a false positive that has a real fix.** The repository's
  only `nolint`s are two line-scoped G204/G302 exemptions, approved because
  the code must run and chmod a binary (0004-PLAN Phase 1, deviation D2).
  This finding has no such need.
* **Keep current tooling.** Newer analysers find real defects, so the pin
  should move forward, not stay behind the developer machines.

## Considered Options

* **A. Bump the pin to v2.14.0 and remove the taint source from the test.**
* **B. Keep v2.13.2,** and gate locally with a copy of the pinned binary.
* **C. Bump, and exempt `target.go:127` with a line-scoped `nolint:gosec`.**
* **D. Bump, and exclude G703 in `.golangci.yml`.**

## Decision Outcome

Chosen option: **"A"** (owner, 2026-10-01: "Pin CI to v2.14 too"), because
it is the only one that leaves no exemption and no version gap:

* the test builds its root without an environment read, and keeps its
  purpose;
* CI, the pre-add script's install hint and the docs move to `v2.14.0`
  together.

### Consequences

* Good, because local and CI lint agree again, on the newer gosec.
* Good, because G703 stays fully enabled for production code, with no
  exemption.
* Neutral, because a later G703 in a test file can again surface at a
  production sink. The response is the same: fix the source in the test.
* Bad, because a future local upgrade can drift again. This record is the
  pattern to follow: bump the pin with the fix, in one record.

### Confirmation

* On a scratch copy, the old test line makes v2.14 report G703 at
  `target.go:127`, and the new line clears it.
* `TestCanonicalizeRootRejectsFilesystemRoot` still fails when
  `canonicalizeRoot` accepts a filesystem root, on Unix and on the Windows
  test host.
* `make lint` with v2.14.0 is clean on all three targets.
* CI's lint step runs v2.14.0 and is green.

## Pros and Cons of the Options

### A. Bump and fix the source

* Good, because there is no exemption and no gap.
* Neutral, because it is a one-line test change plus the pin.

### B. Keep v2.13.2

* Good, because nothing changes.
* Bad, because every developer machine on the newer linter needs a
  side-installed binary, and the newer gosec's checks are lost.

### C. A line-scoped `nolint`

* Good, because it is quick.
* Bad, because it exempts production code from a finding whose cause is a
  test, and it hides any later real taint at that sink.

### D. Exclude G703

* Bad, because it turns off the rule for the whole repository.

## More Information

* The golangci-lint release list (`gh api repos/golangci/golangci-lint/releases`):
  `v2.14.0` was published 2026-09-24, and `v2.13.2` on 2026-08-27.
* gosec `v2.29.0`, `analyzers/pathtraversal.go`: the sources, sinks and
  sanitizers quoted above.
* [0004-PLAN-v1-1-0-core-api.md](0004-PLAN-v1-1-0-core-api.md), Step 8,
  deviation D2: the repository's only `nolint`s.

* Update 2026-11-18: [0046-MADR-move-golangci-lint-to-v2-15-0.md](0046-MADR-move-golangci-lint-to-v2-15-0.md) supersedes the pin of v2.14.0 for the version only.
