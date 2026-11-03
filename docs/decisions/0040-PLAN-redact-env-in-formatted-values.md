---
status: in-progress
date: 2026-11-02
associated-madr: "0040-MADR-close-four-open-items-after-v1-23-1.md"
---
<!-- markdownlint-disable MD004 MD013 MD024 MD060 -->

# Implement D2: redact `Env` when `service.Command`, `service.HandOff`, `service.ExecOptions` and `ghattest.Options` are formatted, and release `v1.24.0`

Associated MADR: [0040-MADR-close-four-open-items-after-v1-23-1.md](0040-MADR-close-four-open-items-after-v1-23-1.md)
(D2-A, as the owner chose on 2026-11-02, with the helpers in a new `internal/redact`).

## Goal

* **No `fmt` verb and no log/slog handler given one of the four values prints an `Env` value.** Each type gains `String`, `GoString`, `Format` and `LogValue` on a value receiver, as 0025-MADR D1-A gave `Credential` and `GitHubOptions` (MADR F2.6). Today each prints a secret 6 or 7 times out of 7 (F2.5).
* **The names stay.** Each `Env` entry prints as its name and `<redacted>`, so the output still serves a diagnosis.
* **Each new test is seen failing first,** on scratch copies with one method or one rule planted out.
* **`v1.24.0` is tagged from `main`.** `make apicheck` reports it compatible with `v1.23.1`, its tag's CI passes every job, and the proxy serves it as `@latest`.

## Scope

### In scope

| Phase | What | Commit |
| :--- | :--- | :--- |
| E0 | the records: the 0040 MADR, its three PLANs and their index rows | records only, shared with the D3 and D4 PLANs |
| E1 | the methods in `runner.go`, `handoff.go`, `execreconciler.go` and `ghattest.go`; the new `internal/redact` package and its test; two new test files; the depguard rules in `.golangci.yml`; `AGENTS.md`; `CHANGELOG.md`, the migration guide's §24, the extending guide, `docs/architecture.md`; this PLAN's record | one commit; its push runs full CI |
| E2 | `v1.24.0`: the tag, its CI, the pin commit, the proxy | the owner's tag, then a pin commit |
| E3 | closing | records only |

### Out of scope

* **`encoding/json`.** It still prints `Env` whole, and slog's JSON handler still prints it for a value nested in another, as 0025-MADR Amendment A3 records for `Credential`. Each type's doc says so (MADR, Consequences).
* **Changing a field's type** (D2-D), **redacting by name pattern** (D2-B), and an **exported redaction helper** (D2-C). The MADR rejects each.
* **Other fields that could hold a secret.** `HandOff.Args`, `Command.Args` and the paths print as they are (F2.7).
* **`ghattest.Policy` on its own.** It holds no secret and gains no method. `Options` prints it inside its own `String`.
* **`selfupdate`'s own `redacted` and `setOrNil`** (`credentials.go:64-77`). They stay where they are, and `selfupdate` does not import `internal/redact`. The owner chose the package for the four new types only (2026-11-02).
* **The architecture table check itself.** At `3acd87f` it compares only the rows the table has: the pre-check's plant A1 removed the new row and the check still exited 0. [0040-PLAN-require-architecture-table-rows.md](0040-PLAN-require-architecture-table-rows.md) (D5, the MADR's Amendment A1) makes it require a row for every directory Go builds, and runs before this plan. If D5 is not in the tree when E1 starts, that is a deviation, and E1 step 8 asserts the row by name until it is.
* **Programs that adopt `v1.24.0`,** which do so in their own repositories.
* **Pushes and the tag,** which happen only on the owner's word, in the same turn.

## Rules

* E1 ends in one commit, `git commit --no-edit`, made by the owner, after `make pre-add-check FILES="<the eight .go files>"` exits 0. `actionlint` is not needed, since no workflow changes.
* Every edit to an existing file is a Python script under the scratchpad that asserts each old string occurs exactly once before replacing it.
* Every plant is made with `scripts/plant-copy.sh` on a scratch copy, never in the tree. Each copy is removed after its run.
* Gate and tool output goes to the scratchpad, with its exit status captured before any filter.
* Before each push the owner is asked for, the disclosure guard runs over the outgoing commits.
* Nothing staged names a host, an account or a real-machine path.
* **Deviations.** A step that finds anything outside its files, or a check that fails, stops. It is recorded here, dated, and in the MADR as an amendment when a fact or decision changes.

## The output

What each method prints. The same rules apply to all four types:

* a string field is quoted, with `strconv.Quote`;
* a `[]string` other than `Env` is printed as `%q` prints it, `[]` when empty or nil;
* a function or interface field is printed as `<set>` or `<nil>`;
* every `Env` entry is redacted, by `redact.Env` (E1 step 3).

| `Env` entry | Prints as |
| :--- | :--- |
| `NAME=value` | `"NAME=<redacted>"` |
| `NAME=` (empty value) | `"NAME=<empty>"`, as `GitHubOptions.Token` prints `<empty>` |
| an entry with no `=` | `"<redacted>"`; the whole entry may be the secret |
| a nil `Env` | `<nil>`, since a nil `Env` means "this process's environment" for `Command` and `ghattest.Options` |
| an empty, non-nil `Env` | `[]` |

| Type | `String`, for the fixtures shown |
| :--- | :--- |
| `service.Command` | `Command{Path:"/usr/bin/gh" Args:["attestation"] Env:["GH_TOKEN=<redacted>" "EMPTY=<empty>" "<redacted>"] Stdin:<2 bytes>}`. `Stdin` is `<nil>` when nil, else `<N bytes>`. The zero value is `Command{Path:"" Args:[] Env:<nil> Stdin:<nil>}`. |
| `service.HandOff` | `HandOff{ID:"h1" Executable:"/opt/demo" Args:["update"] Env:["A=<redacted>"] ResultPath:"/opt/.demo.selfupdate.handoff"}` |
| `service.ExecOptions` | `ExecOptions{Reconcile:["setup"] Restore:[] RestoreFunc:<set> Env:<nil> Runner:<nil>}` |
| `ghattest.Options` | `Options{Policy:{Repository:"o/r" SignerWorkflow:"" SignerDigests:[] AllowSelfHostedRunners:false} GH:"/usr/bin/gh" Env:["GH_TOKEN=<redacted>" "EMPTY=<empty>" "<redacted>"] Runner:<set> Timeout:0s TempDir:""}`. `Timeout` prints as `time.Duration` does. |

`GoString` is `String`. `Format` writes `String` for every verb, so `%x`, `%d` and `%q` reach no field. `LogValue` is a group with the same content, keyed in lower snake case, as `GitHubOptions.LogValue` is:

| Type | `LogValue` keys |
| :--- | :--- |
| `Command` | `path`, `args`, `env`, `stdin` |
| `HandOff` | `id`, `executable`, `args`, `env`, `result_path` |
| `ExecOptions` | `reconcile`, `restore`, `restore_func`, `env`, `runner` |
| `ghattest.Options` | `policy` (a group: `repository`, `signer_workflow`, `signer_digests`, `allow_self_hosted_runners`), `gh`, `env`, `runner`, `timeout`, `temp_dir` |

`env` is the redacted entries, or the string `<nil>`. A nil pointer to one of the types prints `<nil>` through `fmt`, which catches the nil receiver.

## Implementation Steps

### Phase E0: records

As [0040-PLAN-record-host-cleanup-in-0038.md](0040-PLAN-record-host-cleanup-in-0038.md) H0. One records commit holds the MADR and the three PLANs.

### Phase E1: the methods, the tests and the documents

Files:

* `selfupdate/service/runner.go`, `handoff.go` and `execreconciler.go`;
* `selfupdate/verify/ghattest/ghattest.go`;
* `internal/redact/redact.go` and `internal/redact/redact_test.go`, both new;
* `selfupdate/service/redact_test.go` and `selfupdate/verify/ghattest/redact_test.go`, both new;
* `.golangci.yml` and `AGENTS.md`;
* `CHANGELOG.md`, `docs/guides/migrating-from-mcplib-selfupdate.md`, `docs/guides/extending-selfupdate.md` and `docs/architecture.md`;
* this PLAN, and `docs/README.md` (this plan's row).

1. **Baseline.**
   * `git status --short` is clean.
   * `git diff --stat 4a9ead5 HEAD -- selfupdate/service/runner.go selfupdate/service/handoff.go selfupdate/service/execreconciler.go selfupdate/verify/ghattest .golangci.yml` prints nothing, so the four types and the rules are as the MADR read them.
   * `internal/redact` does not exist, and `git grep -n 'fixtureSecret\|secretForms' -- selfupdate/service selfupdate/verify` prints nothing, so the new names are free.
2. **The methods.** Each type's four methods follow its declaration, in the order `String`, `GoString`, `Format`, `LogValue`, as `Credential`'s do in `credentials.go`. `Command`'s are, verbatim:

   ```go
   // String implements fmt.Stringer. It prints each Env entry as its name and
   // <redacted>, or <empty> for an empty value, an entry with no "=" as
   // <redacted>, a nil Env as <nil>, and Stdin only as its length.
   func (c Command) String() string {
   	return "Command{Path:" + strconv.Quote(c.Path) + " Args:" + redact.Quoted(c.Args) +
   		" Env:" + redact.EnvText(c.Env) + " Stdin:" + redact.Length(c.Stdin) + "}"
   }

   // GoString implements fmt.GoStringer. It is String.
   func (c Command) GoString() string { return c.String() }

   // Format implements fmt.Formatter: every verb prints String, so no
   // directive reaches an Env value or Stdin.
   func (c Command) Format(f fmt.State, _ rune) {
   	_, _ = io.WriteString(f, c.String()) //nolint:errcheck // fmt.Formatter has no error to return; fmt drops write errors
   }

   // LogValue implements slog.LogValuer, with Env and Stdin as String
   // prints them.
   func (c Command) LogValue() slog.Value {
   	return slog.GroupValue(slog.String("path", c.Path), slog.Any("args", c.Args), redact.EnvAttr(c.Env),
   		slog.String("stdin", redact.Length(c.Stdin)))
   }
   ```

   `HandOff`, `ExecOptions` and `ghattest.Options` follow the same shape, with the fields and keys of "The output" above. `ghattest.Options` prints `SignerDigests` with `redact.Quoted`. Each file's imports gain `github.com/maccavelli/go-selfupdate-lib/internal/redact` and what else it uses:
   * `runner.go`: `io`, `log/slog` and `strconv`;
   * `handoff.go`: `io`, `log/slog` and `strconv`;
   * `execreconciler.go`: `io` and `log/slog`;
   * `ghattest.go`: `io`, `log/slog` and `strconv`.
3. **The package `internal/redact`.** This is the owner's answer of 2026-11-02: one copy of the helpers, which `service` and `ghattest` both import. It is not an exported API: the `internal/` directory keeps it within the module, so `apidiff` does not report it. `internal/redact/redact.go`, verbatim:

   ```go
   // Package redact is what the formatters of service.Command,
   // service.HandOff, service.ExecOptions and ghattest.Options share: an
   // environment with its values hidden, and stand-ins for what a formatter
   // does not print (docs/decisions/0040-MADR-close-four-open-items-after-v1-23-1.md
   // D2-A).
   package redact

   import (
   	"fmt"
   	"log/slog"
   	"strconv"
   	"strings"
   )

   // Nil stands in for a nil value a formatter does not print.
   const Nil = "<nil>"

   // Env is env with every value hidden: NAME=<redacted>, or NAME=<empty> for
   // an empty value, and <redacted> for an entry with no "=", which may be the
   // secret itself. A nil env stays nil.
   func Env(env []string) []string {
   	if env == nil {
   		return nil
   	}
   	out := make([]string, len(env))
   	for i, e := range env {
   		name, value, ok := strings.Cut(e, "=")
   		switch {
   		case !ok:
   			out[i] = "<redacted>"
   		case value == "":
   			out[i] = name + "=<empty>"
   		default:
   			out[i] = name + "=<redacted>"
   		}
   	}
   	return out
   }

   // EnvText is env as a String method prints it: Nil, or Env's entries,
   // quoted.
   func EnvText(env []string) string {
   	if env == nil {
   		return Nil
   	}
   	return Quoted(Env(env))
   }

   // EnvAttr is env as a LogValue method gives it, under the key "env".
   func EnvAttr(env []string) slog.Attr {
   	if env == nil {
   		return slog.String("env", Nil)
   	}
   	return slog.Any("env", Env(env))
   }

   // Quoted is s as %q prints it, each element quoted.
   func Quoted(s []string) string { return fmt.Sprintf("%q", s) }

   // Length stands in for bytes a formatter does not print: Nil, or
   // <N bytes>.
   func Length(b []byte) string {
   	if b == nil {
   		return Nil
   	}
   	return "<" + strconv.Itoa(len(b)) + " bytes>"
   }

   // SetOrNil stands in for a value a formatter does not print: <set>, or
   // Nil.
   func SetOrNil(set bool) string {
   	if set {
   		return "<set>"
   	}
   	return Nil
   }
   ```

   `Nil` is a constant because `golangci-lint`'s `goconst` refused three `"<nil>"` literals in one file in the first pre-check.

   `internal/redact/redact_test.go` tests the package on its own (`t.Parallel()`).
   * `TestEnv`: `Env` maps `GH_TOKEN=secret`, `EMPTY=`, `secret`, `=C:=C:\x` (a Windows drive entry) and `A=b=c` to `GH_TOKEN=<redacted>`, `EMPTY=<empty>`, `<redacted>`, `=<redacted>` and `A=<redacted>`. A nil env stays nil, and an empty one stays empty and non-nil.
   * `TestText`: `EnvText`, `Quoted`, `Length` and `SetOrNil` for nil, empty and set inputs, and `EnvAttr`'s key and kind.

   **The depguard rules** in `.golangci.yml`, as `AGENTS.md` ("Dependencies") requires of a new package:
   * a rule `internal-redact`, which sorts after `banned`, beside the other `internal-*` rules: files `**/internal/redact/*.go` and `!$test`, allow `$gostd` only, with a comment citing 0040-MADR D2-A;
   * `service` and `selfupdate-verify-ghattest` each gain the allow `github.com/maccavelli/go-selfupdate-lib/internal/redact$`;
   * `other-packages` gains the exclusion `"!**/internal/redact/*.go"`.
4. **The doc comments.** Each type's comment gains a paragraph after its first, after a `//` line, as `Credential`'s says what it redacts. In order, `Command`, `HandOff`, `ExecOptions` and `ghattest.Options`:

   ```go
   // Formatting a Command with fmt or log/slog prints each Env entry's name,
   // never its value, and Stdin only as its length. encoding/json prints
   // both, and slog's JSON handler encodes a value that holds a Command, such
   // as a struct field, with encoding/json too, so log the Command itself
   // (0040-MADR D2-A).

   // Formatting a HandOff with fmt or log/slog prints each Env entry's name,
   // never its value; encoding/json prints it, as for Command (0040-MADR
   // D2-A).

   // Formatting ExecOptions with fmt or log/slog prints each Env entry's
   // name, never its value, and RestoreFunc and Runner only as <set> or <nil>.
   // encoding/json cannot encode them, since they hold a function
   // (0040-MADR D2-A).

   // Formatting Options with fmt or log/slog prints each Env entry's name,
   // never its value, and Runner only as <set> or <nil>. encoding/json prints
   // Env, and slog's JSON handler encodes a value that holds Options, such as
   // a struct field, with encoding/json too, so log the Options themselves
   // (0040-MADR D2-A).
   ```

5. **The tests.** These are in the style of `selfupdate/redact_test.go`, with its `fixtureSecret` (`"fixture-secret-0123"`) and `secretForms`: the text, lower and upper hex, base64, and the decimal bytes. The fixture `Env` is `{"GH_TOKEN=" + fixtureSecret, "EMPTY=", fixtureSecret}`.
   * **`selfupdate/service/redact_test.go`.** Each test calls `testproc.CheckNoLeak(t)` first, as the package's tests do.
     * **`TestEnvNeverFormats`** formats each of these values through `%v`, `%+v`, `%#v`, `%s`, `%q`, `%x`, `%X` and `%d`, and through slog's text and JSON handlers:
       * `Command` (with `Stdin` set to the secret), `*Command`, `HandOff`, `*HandOff`, `ExecOptions` (with `RestoreFunc` and `Runner` set) and `*ExecOptions`;
       * a slice `[]any{cmd, ho}`;
       * a struct field `struct{ C Command }`.

       No form of the secret may appear, and `GH_TOKEN=<redacted>` must. The slice and the struct field through slog's JSON handler are the exception, where encoding/json's output, secret included, is required (Amendment A3).
     * **`TestEnvFormatText`** requires `String` to be exactly the `Command` (with `Stdin: []byte("{}")`), zero `Command`, `HandOff` and `ExecOptions` lines of "The output". It also requires `Command{Env: []string{}}` to print `Env:[]`, and a nil `*Command` to print `<nil>`.
   * **`selfupdate/verify/ghattest/redact_test.go`.** **`TestOptionsEnvNeverFormats`** (`t.Parallel()`, as the package's tests are) formats these values with the same verbs and handlers:
     * `Options`, with `Runner: service.ExecRunner()`, and `*Options`;
     * a slice;
     * a struct field.

     The slice and struct field hold a copy with `Runner` nil, since encoding/json cannot encode a `Runner`, and through slog's JSON handler they must show the secret, as above. The test also requires the `ghattest.Options` line of "The output" exactly, and `Env:<nil> Runner:<nil> Timeout:1m0s` for `Options{Timeout: time.Minute}`.
6. **Seen failing.** Each plant is made in a copy from `scripts/plant-copy.sh`, which holds the new code and tests. The test plants run `go test -count=1 -run '^(TestEnvNeverFormats|TestEnvFormatText|TestOptionsEnvNeverFormats|TestEnv|TestText)$' ./selfupdate/service/ ./selfupdate/verify/ghattest/ ./internal/redact/`. The rule plants run `golangci-lint run -c .golangci.yml` on the same three packages, with `GOOS=linux` and `CGO_ENABLED=0`. Each must exit 1, and the unplanted tree must exit 0.

   | Plant | File | OLD → NEW | Fails, as the pre-check of 2026-11-02 saw |
   | :--- | :--- | :--- | :--- |
   | F1 | `runner.go` | `'func (c Command) Format('` → `'func (c Command) unusedFormat('` | `slice with %d prints the secret as "fixture-secret-0123"` |
   | F2 | `handoff.go` | `'func (h HandOff) Format('` → `'func (h HandOff) unusedFormat('` | `HandOff with %x does not name GH_TOKEN` (hex of `String`) |
   | F3 | `execreconciler.go` | `'func (o ExecOptions) Format('` → `'func (o ExecOptions) unusedFormat('` | as F2, for `ExecOptions` |
   | F4 | `ghattest.go` | `'func (o Options) Format('` → `'func (o Options) unusedFormat('` | as F2, for `Options` |
   | L1 | `runner.go` | `'func (c Command) LogValue('` → `'func (c Command) unusedLogValue('` | `Command through slog's json handler prints the secret as "fixture-secret-0123"` |
   | L2 | `ghattest.go` | `'func (o Options) LogValue('` → `'func (o Options) unusedLogValue('` | `Options through slog's json handler does not name GH_TOKEN`, since encoding/json fails on the `Runner` |
   | R1 | `internal/redact/redact.go` | `'out[i] = name + "=<redacted>"'` → `'out[i] = e'` | `slice with %v prints the secret as "fixture-secret-0123"` |
   | N1 | `internal/redact/redact.go` | `'out[i] = name + "=<redacted>"'` → `'out[i] = "<redacted>"'` | `HandOff with %v does not name GH_TOKEN` |
   | E1 | `internal/redact/redact.go` | `'out[i] = name + "=<empty>"'` → `'out[i] = name + "=<redacted>"'` | `FAIL` in all three packages: `TestEnvFormatText`, `TestOptionsEnvNeverFormats`'s exact line, and `TestEnv` |
   | G1 | `internal/redact/redact.go` | `'\t"strings"\n)'` → an import of `internal/testproc` and `var _ = testproc.CheckNoLeak` | `import 'github.com/maccavelli/go-selfupdate-lib/internal/testproc' is not allowed from list 'internal-redact' (depguard)` |
   | G2 | `.golangci.yml` | `service`'s `internal/redact$` allow removed, OLD being its `internal/procfs$` line and the allow after it | `import 'github.com/maccavelli/go-selfupdate-lib/internal/redact' is not allowed from list 'service' (depguard)`, in `execreconciler.go`, `handoff.go` and `runner.go` |

   F1 to F4 are 0025-PLAN S2 step 3's plant, one per type. G1 and G2 show that the new rule and the new allow are live (`AGENTS.md`, "Checks").
7. **The documents.**
   * **`CHANGELOG.md`:** `## v1.24.0 - <the tag's date>`.
     * Under Added: `String`, `GoString`, `Format` and `LogValue` on `service.Command`, `service.HandOff`, `service.ExecOptions` and `ghattest.Options`.
     * Under Changed: `fmt` and `log/slog` print those four with each `Env` value as `<redacted>`, `Command.Stdin` as its length, and functions and runners as `<set>` or `<nil>`. `encoding/json` is unchanged.
     * If the D3 PLAN's commit is in the tag, also under Changed: `taskscheduler.Options.RequestStop`'s doc says its run is spent inside `Poll`'s timeout.
     * It links the guide's new section, the MADR and this PLAN. E2 moves the date if the tag is cut on another day.
   * **The migration guide** gains `## 24. From v1.23 to v1.24` after §23, which ends the guide today. It holds:
     * its own `go get …@v1.24.0` block;
     * an opening: one exported change, all additions, and `make apicheck` "compatible with v1.23.1";
     * "What is new", the methods;
     * "Behaviour you may notice", the new `fmt` and slog output, and that JSON is unchanged;
     * "Adopting it": nothing is required, and a program that matched the old `%v` output of these types, which no document offered, matches the new;
     * "Check".

     It cites the MADR.
   * **`docs/guides/extending-selfupdate.md`** gains one bullet in "Run as a service", after the `ExecOptions.Env` sentence (`:734`), and one in "Check build provenance". Each says "Since `v1.24.0`, `fmt` and `log/slog` print … with each `Env` value as `<redacted>`", and that `encoding/json` does not.
   * **`docs/architecture.md`:**
     * the tree (`:121-128`) gains `internal/redact/`, after `internal/procfs/`: "the formatters' redaction: an environment's values hidden";
     * the redaction bullet (`:221-224`) gains the four types, their files, and `internal/redact`;
     * a bullet after `internal/procfs`'s (`:397`) says what `internal/redact` holds and who imports it;
     * the package table:
       * `selfupdate/service/` goes from 22 / 29 to 22 / 30, and its imports gain `internal/redact`;
       * `selfupdate/verify/ghattest/` goes from 2 / 3 to 2 / 4, and its imports gain `internal/redact`;
       * a new row after `internal/procfs/`: `` `internal/redact/` | `redact` | 1 | 1 | none ``.
   * **`AGENTS.md`:**
     * the scope paragraph names `internal/redact` after `internal/procfs`: what it hides and stands in for, for the formatters of the four types, citing 0040-MADR D2-A;
     * "Dependencies" lists the rule `internal-redact` with the other `internal-*` rules.
8. **Checks.**
   * `make gate`: every step rc=0, `overall=0`. That covers `lint` 0 issues on three GOOS, `tidy` clean, `apicheck` "compatible with v1.23.1", `arch` "21 rows in docs/architecture.md, 0 differ, 0 without a row" (D5's check), and `ids` 0 findings.
   * D5's check covers `internal/redact/`'s row. Seen failing: on a scratch copy of E1's tree with that row removed, `--arch` must exit 1 with `internal/redact: 1 non-test and 1 test .go files, and no row in the table`.
   * `apidiff` between a `git archive` of `v1.23.1` and the tree lists exactly the 16 methods, each "added", and nothing incompatible.
   * Under `GOTOOLCHAIN=go1.26.10`, `go vet ./...` and `go test -count=1 ./...` exit 0.
   * `go test -race -count=1 ./selfupdate/service/ ./selfupdate/verify/ghattest/ ./internal/redact/`, with `CGO_ENABLED=1`, on a copy of the tree on the Linux test host and on the Windows test host: `ok` each. Each copy is removed after its run.
   * `make markdownlint`, `make check-records`, `make docs-check`, and `scripts/check-docs.sh --ids` on the changed files: exit 0 each.
9. The owner commits. The push runs full CI, which must pass every job.

**Pre-check, 2026-11-01.** E1 was first prototyped with the helpers unexported in each package, on a scratch copy of `4a9ead5`, not in the tree (scratchpad scripts `p40d2.py`, then `p40d2b.py`). Results:

* the three type tests passed;
* `golangci-lint` reported 0 issues on three GOOS once `"<nil>"` was a constant. The first run reported `goconst`'s `string "<nil>" has 3 occurrences`;
* `apidiff` listed the 16 methods as "added", and nothing as incompatible;
* nine plants each exited 1.

**Pre-check, 2026-11-02, as written above.** After the owner's answer, E1 steps 2 to 6 were prototyped with `internal/redact` on a scratch copy of `4a9ead5`, not in the tree (scratchpad script `p40d2c.py`). The `.golangci.yml` changes and the three package-table changes were made too. Results:

* `gofmt -l` printed nothing, and `go vet` exited 0 on the three packages;
* the five new tests passed, and so did the three packages' other tests;
* `golangci-lint run ./...` reported 0 issues for `GOOS=linux`, `darwin` and `windows`, depguard included;
* `scripts/check-docs.sh --arch` reported 21 rows and 0 differ, in a git index of the copy;
* `apidiff` from `v1.23.1` listed 16 changes, the 16 methods, each "added". `-incompatible` listed nothing, and `internal/redact` is not reported;
* each of the eleven plants in step 6's table exited 1 with its failure;
* plant A1, the new table row removed, left `--arch` at exit 0 with 20 rows: the gap D5 closes. D5's pre-check saw its check refuse the same case, an untracked `internal/redact/redact.go` with no row (exit 1).

### Phase E2: `v1.24.0`

As [0039-PLAN-refresh-golang-x-requirements.md](0039-PLAN-refresh-golang-x-requirements.md) R2, for `v1.24.0`:

1. **Before the tag.**
   * `make apicheck` reports compatibility with `v1.23.1`.
   * `apidiff` between a `git archive` of `v1.23.1` and the tree lists E1 step 8's 16 additions and nothing else.
   * `scripts/check-release-tag.sh v1.24.0` exits 0.
   * With `main` level with `origin`, the disclosure guard passes.
2. **The tag:** the owner makes the annotated tag `v1.24.0`, message `v1.24.0` as the earlier tags. It goes on E1's commit, or on a later commit that changes only records or D3's or D4's files, and the owner pushes it.
3. **The tag's CI** completes `success`, every job. Its identity legs print `v1.24.0 (release) <12-hex>`.
4. **The pin commit.** ~~`git diff --stat v1.23.1 v1.24.0 -- .github internal/cmd scripts` must be empty~~ `git diff --stat v1.23.1 v1.24.0 -- .github internal/cmd` and the four scripts the release workflows run (`check-release-tag.sh`, `verify-selfupdate-release.sh`, `refuse-existing-release.sh`, `release-latest-flag.sh`) must be empty (deviation of 2026-11-02, below), so the workflows are unchanged and only the pins move. They move to `v1.24.0` and its commit, as 0039-PLAN R2 step 4 moved them:
   * `README.md`'s current release, `go get` line and publish pin;
   * `docs/architecture.md`'s current release and commit;
   * `docs/guides/building-releases.md`'s "examples below pin", three workflow pins and `git ls-remote` example;
   * in the migration guide:
     * every `go get github.com/maccavelli/go-selfupdate-lib@v1.23.1` line and every "`go list -m` … gives `v1.23.1`" line;
     * "is the current release", §3's workflow pin, "the example pins" and its range, `git ls-remote`, and §5's "Require go-selfupdate-lib";
     * §2's release list, which gains `v1.24.0` and links §24.

     The `go get` and `go list -m` lines are counted before the edit, and the counts are recorded. On 2026-11-01 there are 19 `go get …@v1.23.1` lines and 17 `go list -m` lines naming `v1.23.1`. 0039's count deviation came from a fixed number; counting first avoids that.

   Lines that name `v1.23.1` as history stay. `make markdownlint`, `make docs-check` and `--ids` on the changed files exit 0. The owner commits and pushes; the push runs Docs.
5. **The proxy:**
   * `go list -m -json github.com/maccavelli/go-selfupdate-lib@v1.24.0` gives its commit and `Ref refs/tags/v1.24.0`;
   * `@latest` resolves to `v1.24.0`;
   * `go doc github.com/maccavelli/go-selfupdate-lib/selfupdate/service Command.String` resolves in a scratch module that requires it.

### Phase E3: closing

1. The execution record complete, the Verification below answered with output, this PLAN `complete`, the MADR's status as decided, and the index rows to match.
2. No copy of the tree left on a test host, by a read-only look. `git status --short` is clean but for the owner's commits.

## Verification

* **V1 (redaction):** `TestEnvNeverFormats`, `TestEnvFormatText`, `TestOptionsEnvNeverFormats`, and `internal/redact`'s `TestEnv` and `TestText` pass at E1's commit, on every CI job and on the two test hosts. Probe E1's seven forms (MADR F2.5) are among them, and none prints the secret.
* **V2 (seen failing):** the eleven plants each fail as E1 step 6's table says, the two depguard plants among them.
* **V2a (the package):** `internal/redact` is in `.golangci.yml` (its rule, two allows and the `other-packages` exclusion), in `AGENTS.md`, and in `docs/architecture.md`'s tree and table, by name.
* **V3 (the API):** `apidiff` `v1.23.1` → `v1.24.0` lists the 16 methods, each "added", and nothing else. `make apicheck` reports compatibility with `v1.23.1` before the tag.
* **V4 (the gates):** `make gate` `overall=0`, the documentation checks exit 0, and the floor's `vet` and `test` pass under `go1.26.10`.
* **V5 (the release):** the tag's CI is green, its identity legs name `v1.24.0`, and the proxy serves it as `@latest`.

## Rollout and Rollback

* **Rollout:** E1 is one commit on `main`, pushed on the owner's word. The tag follows a green CI run. A program gets the new output when it requires `v1.24.0`.
* **Rollback:** before the tag, `git revert` of E1's commit. After it, the methods are API and stay within `v1`. A fault is fixed forward in `v1.24.1`, and a format that must change is changed there. No document offers the format as stable.

## Execution record

### The owner's answers (2026-11-02)

Asked four questions, with three options each:

* **Decisions:** "Accept D2-A, D3-A, D4-A (Recommended)". D2-A is the owner's decision.
* **D2 helpers:** "New internal/redact package", not the recommended copy into `ghattest`. E1 steps 2, 3, 6, 7 and 8 were rewritten for it before any execution, and proved again on a scratch copy (E1's pre-check of 2026-11-02). Because nothing had been executed, this is not a deviation. `selfupdate`'s own helpers stay where they are.
* **Order:** "Records, then D4, D3, D2 (Recommended)". This plan runs last, and its tag carries D3's doc sentence. With the MADR's Amendment A1, D5's plan runs just before it, if the owner accepts D5.
* **Host tests:** "As written (Recommended)": E1 step 8's Linux and Windows runs.

This plan stayed `proposed` until the owner approved its execution.

### Approval (2026-11-02)

The owner committed and pushed D5's phase as `0db3575`, and said "committed, puashed, proceed". This plan ran last, as the order says, and is `in-progress`.

### E1: the methods, the tests and the documents (2026-11-02)

Files: as E1 lists them, and no others.

1. **Baseline.**
   * `git status --short` was clean, with `main` level with `origin` at `0db3575`.
   * `git diff --stat 4a9ead5 HEAD` of the four types' files and `.golangci.yml` printed nothing (exit 0).
   * `internal/redact` did not exist, and the `git grep` for `fixtureSecret` and `secretForms` exited 1, with no match.
2. **Steps 2 to 5: the code, the package, the rules and the tests.** These were written by the pre-check's own `prototype()` (scratchpad `p40e1apply2.py`, reading `p40d2c.py`).
   * After it, `cmp` found these nine files byte-identical to the copies the pre-check of 2026-11-02 linted, diffed and planted: `runner.go`, `handoff.go`, `execreconciler.go`, `ghattest.go`, both new `redact_test.go` files, `internal/redact/redact.go` and its test, and `.golangci.yml`.
   * `gofmt` ran on the eight `.go` files.
3. **Step 6: seen failing.** Each plant was made in a `scripts/plant-copy.sh` copy, and each copy was removed (scratchpad `p40e1plants.py`). All eleven exited 1:
   * F1: `slice with %d prints the secret as "fixture-secret-0123"`;
   * F2: `HandOff with %x does not name GH_TOKEN`, and `HandOff with %d prints the secret`;
   * F3: `*ExecOptions with %x does not name GH_TOKEN`, and `%d prints the secret`;
   * F4: `*Options with %x does not name GH_TOKEN`, and `%d prints the secret`;
   * L1: `Command through slog's json handler prints the secret as "fixture-secret-0123"`;
   * L2: `Options through slog's json handler does not name GH_TOKEN` (`!ERROR:json: unsupported type`);
   * R1: `*Command with %v prints the secret as "fixture-secret-0123"`;
   * N1: `Command with %v does not name GH_TOKEN`;
   * E1: `FAIL` in all three packages, `internal/redact`'s line being `Env = [… "EMPTY=<redacted>" …], want [… "EMPTY=<empty>" …]`;
   * G1: `import 'github.com/maccavelli/go-selfupdate-lib/internal/testproc' is not allowed from list 'internal-redact' (depguard)`;
   * G2: `import 'github.com/maccavelli/go-selfupdate-lib/internal/redact' is not allowed from list 'service' (depguard)`, in `execreconciler.go`, `handoff.go` and `runner.go`.

   The step-8 row plant, `internal/redact/`'s row removed, exited 1 under D5's check: `docs/architecture.md: internal/redact: 1 non-test and 1 test .go files, and no row in the table`.
4. **Step 7: the documents,** written by the same script, each old string asserted once:
   * `CHANGELOG.md`: `## v1.24.0 - 2026-11-02`, with Added, and Changed. Changed includes D3's `RequestStop` sentence, since D3's commit, `cb1234f`, is in the tree. It links §24, the MADR and this PLAN.
   * The migration guide: `## 24. From v1.23 to v1.24` after §23, with its `go get …@v1.24.0` block, "What is new", "Behaviour you may notice", "Adopting it" and "Check".
   * `docs/guides/extending-selfupdate.md`: one sentence in "Run as a service", after the `ExecOptions.Env` sentence, and one in "Check build provenance", after `Options.Timeout`'s.
   * `docs/architecture.md`:
     * the tree's `internal/redact/` entry;
     * the redaction bullet;
     * a bullet after `internal/procfs`'s;
     * the package table: `selfupdate/service/` 22 / 30 with `internal/redact` among its imports, `selfupdate/verify/ghattest/` 2 / 4 with `internal/redact`, and the new `internal/redact/` row.
   * `AGENTS.md`: `internal/redact` in the scope paragraph, and `internal-redact` in "Dependencies".
5. **Step 8: checks.**
   * **`make gate`:** `overall=0`. Every step was rc=0:
     * `lint` 0 issues, on three GOOS;
     * `tidy` clean;
     * `apicheck` "compatible with v1.23.1";
     * `fuzz` 11 targets clean;
     * `vuln` "No vulnerabilities found.";
     * `scripts` "all script tests passed";
     * `links` 930 in 108 files, 0 broken;
     * `arch` "21 rows in docs/architecture.md, 0 differ, 0 without a row";
     * `ids` 16 files, 0 findings.
   * **`make pre-add-check`** on the eight `.go` files: "8 file(s) clean (gofmt, golangci-lint, go vet, go test, govulncheck)".
   * **`apidiff`** from a `git archive` of `v1.23.1` to the tree (scratchpad `p40e1api.py`): 16 changes, each "added", the four methods on each of the four types. `-incompatible` listed nothing, and `internal/redact` is not reported.
   * **The floor:** under `GOTOOLCHAIN=go1.26.10` (`go version go1.26.10 darwin/arm64`), `go vet ./...` exit 0, and `go test -count=1 ./...` exit 0.
   * **The test hosts:** `go test -race -count=1 ./selfupdate/service/ ./selfupdate/verify/ghattest/ ./internal/redact/`, with `CGO_ENABLED=1`, on a copy of the tree. The Linux test host: `ok` for the three, `go-test-exit=0`. The Windows test host: `ok` for the three, `go-test-exit=0`. Afterwards each holds 0 `sulib-*` copies.
   * **Documents:**
     * `make markdownlint`: 0 issues;
     * `make check-records`: 95 records, 0 findings;
     * `make docs-check`: 930 links, 0 broken; `arch` "21 rows …, 0 without a row";
     * `--ids` on the 16 changed and new files: 0 findings.
6. **The push.** The owner committed E1 as `74ab5dd` and pushed it.
   * Asked how CI should be confirmed, the owner allowed read-only `gh` calls for this step ("You may read it with gh"), and said "i will tag and push".
   * CI run 41540437098 was `success`, 18 of 18 jobs, with no `--- FAIL` line in its log.
   * Its live tests:
     * `validate (ubuntu-24.04)`: 24 PASS, `TestLiveConformance` twice and `TestLiveSupervisedS6/conformance` among them;
     * `validate (macos-15)`: 14 PASS and 1 SKIP, `TestLiveSignIdentity`, which needs a signing identity the runner lacks;
     * `validate (windows-2025)`: 18 PASS, the SCM's `TestLiveConformance` and `TestLiveTaskConformance` among them.
   * `go126` ran `go1.26.10`.
   * Docs run 41540437040: `success`.

E1 ends here, with its commit on `main` and its CI green. E2 waited for the owner's tag.

### E2: `v1.24.0` (2026-11-02)

1. **Before the tag,** at `74ab5dd`, with `main` level with `origin`:
   * `make apicheck`: "compatible with v1.23.1";
   * `apidiff` from a `git archive` of `v1.23.1` to the tree: 16 changes, each "added", and `-incompatible` lists nothing (scratchpad `p40e1api.py`, run again). The floor's `go vet ./...` and `go test ./...` exit 0 under `go1.26.10`;
   * `scripts/check-release-tag.sh v1.24.0`: exit 0;
   * `git tag -l v1.24.0` prints nothing, so the tag is new;
   * the disclosure guard: exit 0.

   The tag is the owner's to make and push, on `74ab5dd`, as step 2 says.

2. **The tag:** the owner made the annotated tag `v1.24.0` on `74ab5dd`, message `v1.24.0` as the earlier tags, tagged 2026-11-02 17:25 −0600, and pushed it. The changelog's date, 2026-11-02, stands.
3. **The tag's CI,** run 41545281504: `success`, 18 of 18 jobs, with no `--- FAIL` in its log.
   * The ten identity legs each print `"v1.24.0 (release) 74ab5dd376a4"`.
   * Live tests:
     * `validate (ubuntu-24.04)`: 24 PASS, `TestLiveConformance` twice and `TestLiveSupervisedS6` among them;
     * `validate (macos-15)`: 14 PASS and 1 SKIP, `TestLiveSignIdentity`;
     * `validate (windows-2025)`: 18 PASS, `TestLiveConformance` and `TestLiveTaskConformance` among them.
   * `go126` ran `go1.26.10`.
4. **The pin commit.** The narrowed check of the deviation below, `git diff --stat v1.23.1 v1.24.0 -- .github internal/cmd` and the four workflow scripts, printed nothing (exit 0). The pins moved to `v1.24.0` and `74ab5dd…` (scratchpad `p40pin.py`, each old string asserted at its counted number):
   * `README.md`: the current release, the `go get` line and the publish pin;
   * `docs/architecture.md`: the current release and its commit;
   * `docs/guides/building-releases.md`: "examples below pin", the three workflow pins and the `git ls-remote` example;
   * the migration guide:
     * the 19 `go get …@v1.23.1` and 17 `go list -m` … `v1.23.1` lines, counted before the edit;
     * "is the current release", §3's workflow pin, "the example pins" and its range, now `v1.17.2` to `v1.24.0`, `git ls-remote`, and §5's "Require go-selfupdate-lib";
     * §2's release list, which gains `v1.24.0` and links §24.

   After the edit, no file holds `c24f591…`, and the four hold the new commit 1, 1, 3 and 1 times. Lines that name `v1.23.1` as history stay: §2's entry for it, "From v1.23.0 to v1.23.1", and §24's "compatible with `v1.23.1`" and "as in `v1.23.1`".

   Checks:
   * `make markdownlint`: 0 issues;
   * `make docs-check`: 931 links in 108 files, 0 broken, and 21 rows, 0 differ, 0 without a row;
   * `--ids` on the five changed files: 0 findings;
   * `make check-records`: 95 records, 0 findings.
5. **The proxy** (scratchpad `p40proxy.py`, which reads `proxy.golang.org`):
   * `go list -m -json github.com/maccavelli/go-selfupdate-lib@v1.24.0` gives `Version v1.24.0`, `GoVersion 1.26.10` and an `Origin` of `Hash 74ab5dd376a4c864debb32b2d8abfc7856059ba3`, `Ref refs/tags/v1.24.0`;
   * `@latest` resolves to `v1.24.0`;
   * in a scratch module that requires it, `go doc` finds `func (c Command) String() string` in `selfupdate/service` and `func (o Options) LogValue() slog.Value` in `selfupdate/verify/ghattest`.
**Waiting:** the owner's commit and push of this change, whose Docs run must pass. E3 closes the plan on the owner's word.

### Deviation: E2 step 4's diff check (2026-11-02)

* **Found.** After the owner tagged `v1.24.0` on `74ab5dd`, `git diff --stat v1.23.1 v1.24.0 -- .github internal/cmd scripts` printed three files: `scripts/check-docs.sh`, `scripts/check-docs_test.sh` and `scripts/gate.sh`, 39 insertions and 12 deletions. D5's commit, `0db3575`, changed them. D5 was added after this step was written, which named `scripts` whole.
* **Why it does not touch the pins.**
  * `.github` and `internal/cmd` are unchanged (the same diff, exit 0, empty).
  * The build and publish workflows run four scripts from the pinned tag: `check-release-tag.sh`, `verify-selfupdate-release.sh`, `refuse-existing-release.sh` and `release-latest-flag.sh`. None of the three changed scripts appears in either workflow.
  * So a workflow pinned to `v1.24.0` runs exactly what one pinned to `v1.23.1` runs, which is what the check is for.
* **Decision.** The owner chose "Narrow the check, record it":
  * step 4 checks `.github`, `internal/cmd` and those four scripts;
  * every pin moves;
  * the migration guide's range reads `v1.17.2` to `v1.24.0`.

  No file is added to the phase. The MADR asserts nothing about this check, so it is unchanged.
* **Also allowed.** Read-only `gh` calls for the tag's CI run ("Yes, read-only gh").
