# AGENTS.md

Instructions for AI coding agents working in this repository. All agents read
this file. A repository-local `CLAUDE.md` / `.claude/rules/` / `.grok/rules/` /
`.opencode/rules.md` wins only where it is more specific than this file.

`go-selfupdate-lib` is the fleet's self-update library
(`github.com/maccavelli/go-selfupdate-lib`). It was `go-core-lib` up to `v1.4.1`
(`docs/decisions/0009-MADR-rename-to-go-selfupdate-lib.md`). Its scope is
self-update only: `selfupdate`; its `cli` and `selfupdatetest`
subpackages; `selfupdate/service` and its `systemd`, `launchd` and `scm`
backends (`docs/decisions/0011-MADR-reference-service-lifecycles.md`);
`selfupdate/archive`
(`docs/decisions/0012-MADR-archive-assets-and-macos-codesign.md`); and
`buildinfo`, which stamps the identity `selfupdate` decides on. A package
that does not serve self-update belongs in another module. It is a library
only: no packaged binary. `buildinfo` and `selfupdate` are top-level
directories, and each `selfupdate` subpackage lives in a directory under
`selfupdate/`, named after it; there is no root package. Unexported helpers
shared between packages live under `internal/`. Requires Go 1.27.1.

## Dependencies

No module may be required without a MADR in this repository that names it.
Never import `github.com/maccavelli/mcplib`, the MCP go-sdk
(`github.com/modelcontextprotocol/go-sdk`) or
`github.com/maccavelli/go-llmprovider-sdk`: this module sits below them in the
dependency graph, and callers use it to avoid them. Charm
(`charm.land/…`, `github.com/charmbracelet/…`) stays out too; it lives in
go-tui-lib.

`depguard` in `.golangci.yml` enforces these rules, so `make lint`, the
pre-add check and CI fail on a breach
(`docs/decisions/0008-MADR-enforce-import-rules-with-depguard.md`):

- `banned` refuses the modules above, in every file, with the reason;
- `module` allows only the standard library, this module and the
  required modules, in every file;
- `buildinfo`, `selfupdate`, `selfupdate-cli`, `selfupdatetest`,
  `service`, `service-launchd`, `service-scm`, `service-systemd` and
  `selfupdate-archive` allow each package, outside its tests, only the
  imports its record names;
- `other-packages` allows any other package, an `internal/` helper
  included, only the standard library and this module
  (`docs/decisions/0010-MADR-remediate-second-debugging-pass-findings.md`
  D13).

A record that adds a module or a package amends those rules in the same
commit. A new package gets its own rule, and is excluded from
`other-packages`. A new rule's name must sort after `banned`.

`go.mod` and `go.sum` change with the code that needs them: a requirement is
added in the commit that adds its first import, and removed in the commit that
removes its last. `go mod tidy -diff` is clean at every commit.

## MADR and PLAN before mutating work

**Whenever the user asks for an MADR and a plan, load the
`madr-and-plan-writing` skill first** and follow it for authoring, naming and
review. This applies both to writing a fresh pair and to amending an existing
one.

The name is exact — it is the `name:` field of the skill. A mistyped call
returns `Unknown skill`, and an agent that proceeds without the skill writes
something shaped like a MADR while missing the required headings and the
directory rule below. Verify against the filesystem rather than memory:

```bash
ls -d ~/.claude/skills/*madr* && grep '^name:' ~/.claude/skills/*madr*/SKILL.md
```

**Read-only investigation is allowed with no pair.** Reading, searching,
`git log` / `git show` / `git diff`, and existing tests or diagnostics that
do not write the tree do not need a MADR.

**Mutating work is not.** Before the first write, name the
`docs/decisions/NNNN-MADR-*` / `docs/decisions/NNNN-PLAN-*` pair being
executed, or stop and write one.

Mutating means: creating, editing, or deleting files; staging or committing
(except the bootstrap exception below); dependency or lockfile changes;
CI / config / hook changes; builds or installers that write the tree,
`$HOME`, or a live service; generating committed artifacts.

Order:

1. Investigate (read-only).
2. Write or amend the MADR (`status: proposed` unless the owner already
   decided). Present it. Do not implement.
3. Write or amend the PLAN. Present it.
4. Mutate **only after** the owner explicitly approves execution
   (`proceed`, `execute the plan`, `do phase N`). Stay inside that PLAN.
5. Anything discovered mid-execution that is out of scope waits: amend the
   pair, re-approve, then continue. Completing a phase is not permission to
   invent the next unwritten one.

Follow-up vs greenfield:

- **Same topic** (debug, leftover phase, bug found in that plan's live
  run): amend that number. Add a PLAN phase or an amendment in the MADR. Do
  not silently rewrite historical rationale.
- **Greenfield**: next unused `NNNN`, new MADR, new PLAN, same slug. No
  mutation until that PLAN is approved.

Bootstrap exception: authoring `docs/decisions/NNNN-MADR-*`,
`docs/decisions/NNNN-PLAN-*`, this file, and the per-agent pointers
(`.claude/rules/`, `.grok/rules/`, `.opencode/rules.md`) does not require a
*prior* pair. Putting source, tests, CI, or product config in that same commit
is a violation.

`git push` and tags still need an explicit ask in the same turn.

## Records

```text
docs/decisions/NNNN-MADR-short-slug.md
docs/decisions/NNNN-PLAN-short-slug.md
docs/reports/NNNN-REPORT-short-slug.md
docs/reports/NNNN-GATES-short-slug.md
```

- `NNNN` is a zero-padded 4-digit number, one sequence across
  `docs/decisions/` and `docs/reports/`. A MADR and its PLAN share the same
  number and the same slug.
- **Next number** is the highest `NNNN` among all four kinds anywhere under
  `docs/`, plus one. Never reuse a number, never renumber an existing record,
  never leave a gap deliberately.
- Cite records by full filename, never by number alone. Cite another
  repository's record by repository and filename; a relative link cannot reach
  it.
- `docs/README.md` indexes every record. Update it in the same change.

## Pre-add checks

Before staging Go files, run:

```bash
make pre-add-check                 # every tracked Go file
make pre-add-check FILES="a.go b.go"
```

It runs `scripts/go-precheck.sh`: `gofmt` on the files;
`golangci-lint run -c .golangci.yml ./...` once each for `GOOS=linux`,
`darwin` and `windows` with `CGO_ENABLED=0`, the same runs as `make lint` and
CI (`docs/decisions/0002-MADR-rehome-selfupdate-from-mcplib.md` §5);
`go vet` and `go test` on the packages the files belong to; and
`govulncheck ./...` (`GO_PRECHECK_SKIP_VULN=1` skips it offline). The
`selfupdate` tests include `TestManifestDifferential`, which needs
`python3` and skips without it; set `SELFUPDATE_REQUIRE_PYTHON=1` to make it
mandatory, as CI does on Linux and macOS. `golint` is
not used: its checks are `revive`'s `exported`, `package-comments` and
`var-naming` rules in `.golangci.yml`. A file that fails is not committed.

The machine-wide agent gate runs the same script before every agent
`git commit` that stages Go files, and denies the commit when it fails. There
is no `git add` hook on every host; do not rely on one.

`make lint` and `make vuln` must be clean before a release-shaped change.

Cross-target commands set `CGO_ENABLED=0` explicitly: a host `go env` may
set `CGO_ENABLED=1`, and cgo cannot cross-compile to another OS here.

## Identifiers

Nothing committed carries a hostname, account name, org-internal path, or a
real-machine absolute path. Use placeholders (`<user>`, `/home/<user>/...`).

- When recording an identifier scan in a record or a commit, describe what was
  scanned for ("the local account name", "the hostname domain"); never quote
  it.
- Pushes to GitHub pass through a global pre-push disclosure guard. Before
  asking the owner to push, run the guard itself over the outgoing commits:

  ```bash
  echo "refs/heads/main $(git rev-parse HEAD) refs/heads/main $(git rev-parse origin/main)" |
    python3 ~/.global-git-hooks/github-disclosure.py pre-push origin "$(git remote get-url origin)"
  ```

- Never bypass it with `--no-verify`.

## Commits

`git commit --no-edit`. Never pass `-m` / `--message` / `-F`. The global
`prepare-commit-msg` hook writes the message from the staged diff. This
repository sets a local `user.name` / `user.email`; do not override it.
