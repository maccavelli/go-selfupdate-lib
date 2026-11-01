---
status: proposed
date: 2026-11-01
associated-madr: "0040-MADR-close-four-open-items-after-v1-23-1.md"
---
<!-- markdownlint-disable MD004 MD013 MD024 MD060 -->

# Implement D2: redact `Env` when `service.Command`, `service.HandOff`, `service.ExecOptions` and `ghattest.Options` are formatted, and release `v1.24.0`

Associated MADR: [0040-MADR-close-four-open-items-after-v1-23-1.md](0040-MADR-close-four-open-items-after-v1-23-1.md)
(D2-A, proposed).

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
| E1 | the methods and helpers in `runner.go`, `handoff.go`, `execreconciler.go` and `ghattest.go`; two new test files; `CHANGELOG.md`, the migration guide's §24, the extending guide, `docs/architecture.md`; this PLAN's record | one commit; its push runs full CI |
| E2 | `v1.24.0`: the tag, its CI, the pin commit, the proxy | the owner's tag, then a pin commit |
| E3 | closing | records only |

### Out of scope

* **`encoding/json`.** It still prints `Env` whole, and slog's JSON handler still prints it for a value nested in another, as 0025-MADR Amendment A3 records for `Credential`. Each type's doc says so (MADR, Consequences).
* **Changing a field's type** (D2-D), **redacting by name pattern** (D2-B), and an **exported redaction helper** (D2-C). The MADR rejects each.
* **Other fields that could hold a secret.** `HandOff.Args`, `Command.Args` and the paths print as they are (F2.7).
* **`ghattest.Policy` on its own.** It holds no secret and gains no method. `Options` prints it inside its own `String`.
* **A shared internal package for the helpers.** See E1 step 3.
* **Programs that adopt `v1.24.0`,** which do so in their own repositories.
* **Pushes and the tag,** which happen only on the owner's word, in the same turn.

## Rules

* E1 ends in one commit, `git commit --no-edit`, made by the owner, after `make pre-add-check FILES="<the six .go files>"` exits 0.
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
* every `Env` entry is redacted, by `redactEnv` (E1 step 3).

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
* `selfupdate/service/redact_test.go` and `selfupdate/verify/ghattest/redact_test.go`, both new;
* `CHANGELOG.md`, `docs/guides/migrating-from-mcplib-selfupdate.md`, `docs/guides/extending-selfupdate.md` and `docs/architecture.md`;
* this PLAN, and `docs/README.md` (this plan's row).

1. **Baseline.**
   * `git status --short` is clean.
   * `git diff --stat 4a9ead5 HEAD -- selfupdate/service/runner.go selfupdate/service/handoff.go selfupdate/service/execreconciler.go selfupdate/verify/ghattest` prints nothing, so the four types are as the MADR read them.
   * `git grep -n 'fixtureSecret\|secretForms' -- selfupdate/service selfupdate/verify` prints nothing, so the new tests' names are free.
2. **The methods.** Each type's four methods follow its declaration, in the order `String`, `GoString`, `Format`, `LogValue`, as `Credential`'s do in `credentials.go`. `Command`'s are, verbatim:

   ```go
   // String implements fmt.Stringer. It prints each Env entry as its name and
   // <redacted>, or <empty> for an empty value, an entry with no "=" as
   // <redacted>, a nil Env as <nil>, and Stdin only as its length.
   func (c Command) String() string {
   	return "Command{Path:" + strconv.Quote(c.Path) + " Args:" + quoted(c.Args) +
   		" Env:" + envText(c.Env) + " Stdin:" + lengthText(c.Stdin) + "}"
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
   	return slog.GroupValue(slog.String("path", c.Path), slog.Any("args", c.Args), envAttr(c.Env),
   		slog.String("stdin", lengthText(c.Stdin)))
   }
   ```

   `HandOff`, `ExecOptions` and `ghattest.Options` follow the same shape, with the fields and keys of "The output" above. Each file's imports gain what it uses:
   * `runner.go`: `io`, `log/slog`, `strconv` and `strings`;
   * `handoff.go`: `io`, `log/slog` and `strconv`;
   * `execreconciler.go`: `io` and `log/slog`;
   * `ghattest.go`: `io`, `log/slog` and `strconv`.

   All are standard library, so no depguard rule changes.
3. **The helpers.** These are unexported, at the end of `runner.go`, verbatim:

   ```go
   // nilText stands in for a nil value a formatter does not print.
   const nilText = "<nil>"

   // redactEnv is env with every value hidden: NAME=<redacted>, or
   // NAME=<empty> for an empty value, and <redacted> for an entry with no "="
   // (0040-MADR D2-A). A nil env stays nil.
   func redactEnv(env []string) []string {
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

   // envText is env as String prints it: <nil>, or its redacted entries,
   // quoted.
   func envText(env []string) string {
   	if env == nil {
   		return nilText
   	}
   	return quoted(redactEnv(env))
   }

   // envAttr is env as LogValue gives it.
   func envAttr(env []string) slog.Attr {
   	if env == nil {
   		return slog.String("env", nilText)
   	}
   	return slog.Any("env", redactEnv(env))
   }

   // quoted is s as %q prints it, each element quoted.
   func quoted(s []string) string { return fmt.Sprintf("%q", s) }

   // lengthText stands in for bytes a formatter does not print: <nil>, or
   // their length.
   func lengthText(b []byte) string {
   	if b == nil {
   		return nilText
   	}
   	return "<" + strconv.Itoa(len(b)) + " bytes>"
   }

   // setOrNil stands in for a value a formatter does not print.
   func setOrNil(set bool) string {
   	if set {
   		return "<set>"
   	}
   	return nilText
   }
   ```

   `ghattest.go` ends with a copy of `nilText`, `redactEnv`, `envText`, `envAttr`, `quoted` and `setOrNil`. It needs no `lengthText`. The copy's first comment names `service`'s as the original, and says why it is copied: `service`'s helpers are unexported, and `ghattest` may import only the standard library, `selfupdate` and `service` (`.golangci.yml`, `selfupdate-verify-ghattest`).

   Sharing them instead would take either of two routes. One is an exported helper, which is D2-C's shape and which the MADR rejects. The other is a new `internal/` package, which needs its own depguard rule, a row in `docs/architecture.md` and a line in `AGENTS.md`, for the 61 lines the helpers take in the pre-check. The copy is the smaller change. If the owner prefers the internal package, it is a deviation: this PLAN gains the three files, and the MADR is unchanged.

   `nilText` is a constant because `golangci-lint`'s `goconst` refused three `"<nil>"` literals in `runner.go`, in the pre-check below.
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
6. **Seen failing.** Each plant runs `go test -count=1 -run '^(TestEnvNeverFormats|TestEnvFormatText|TestOptionsEnvNeverFormats)$' ./selfupdate/service/ ./selfupdate/verify/ghattest/` in a copy from `scripts/plant-copy.sh`, which holds the new code and tests. Each must exit 1, and the unplanted tree must exit 0.

   | Plant | File | OLD → NEW | Fails, as the pre-check saw |
   | :--- | :--- | :--- | :--- |
   | F1 | `runner.go` | `'func (c Command) Format('` → `'func (c Command) unusedFormat('` | `Command with %d prints the secret as "fixture-secret-0123"` |
   | F2 | `handoff.go` | `'func (h HandOff) Format('` → `'func (h HandOff) unusedFormat('` | `HandOff with %x does not name GH_TOKEN` (hex of `String`), and `%d` prints the secret |
   | F3 | `execreconciler.go` | `'func (o ExecOptions) Format('` → `'func (o ExecOptions) unusedFormat('` | as F2, for `ExecOptions` |
   | F4 | `ghattest.go` | `'func (o Options) Format('` → `'func (o Options) unusedFormat('` | as F2, for `Options` |
   | L1 | `runner.go` | `'func (c Command) LogValue('` → `'func (c Command) unusedLogValue('` | `Command through slog's json handler prints the secret` |
   | L2 | `ghattest.go` | `'func (o Options) LogValue('` → `'func (o Options) unusedLogValue('` | `Options through slog's json handler does not name GH_TOKEN`, since encoding/json fails on the `Runner` |
   | R1 | `runner.go` | `'out[i] = name + "=<redacted>"'` → `'out[i] = e'` | `*Command with %v prints the secret` |
   | R2 | `ghattest.go` | `'out[i] = name + "=<redacted>"'` → `'out[i] = e'` | `Options with %v prints the secret` |
   | N1 | `runner.go` | `'out[i] = name + "=<redacted>"'` → `'out[i] = "<redacted>"'` | `HandOff with %v does not name GH_TOKEN` |

   F1 to F4 are 0025-PLAN S2 step 3's plant, one per type.
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
     * the redaction bullet (`:221-224`) gains the four types and their files;
     * the package table's `selfupdate/service/` row goes from 22 / 29 to 22 / 30;
     * the `selfupdate/verify/ghattest/` row goes from 2 / 3 to 2 / 4.
8. **Checks.**
   * `make gate`: every step rc=0, `overall=0`. That covers `lint` 0 issues on three GOOS, `tidy` clean, `apicheck` "compatible with v1.23.1", `arch` 0 differ, and `ids` 0 findings.
   * `apidiff` between a `git archive` of `v1.23.1` and the tree lists exactly the 16 methods, each "added", and nothing incompatible.
   * Under `GOTOOLCHAIN=go1.26.10`, `go vet ./...` and `go test -count=1 ./...` exit 0.
   * `go test -race -count=1 ./selfupdate/service/ ./selfupdate/verify/ghattest/`, with `CGO_ENABLED=1`, on a copy of the tree on the Linux test host and on the Windows test host: `ok` each. Each copy is removed after its run.
   * `make markdownlint`, `make check-records`, `make docs-check`, and `scripts/check-docs.sh --ids` on the changed files: exit 0 each.
9. The owner commits. The push runs full CI, which must pass every job.

**Pre-check, 2026-11-01.** E1 steps 2 to 6 were prototyped on a scratch copy of `4a9ead5`, not in the tree (scratchpad scripts `p40d2.py`, then `p40d2b.py` with `nilText`). The code and tests were as written above. Results:

* `gofmt -l` printed nothing, and `go vet` exited 0 on both packages;
* the three new tests passed, and so did both packages' other tests;
* `golangci-lint` reported 0 issues for `GOOS=linux`, `darwin` and `windows`. The first run, without `nilText`, reported one `goconst` issue, `string "<nil>" has 3 occurrences`, which step 3's constant answers;
* `apidiff` from `v1.23.1` listed the 16 methods as "added", and `-incompatible` listed nothing;
* each of the nine plants exited 1 with the failure in step 6's table.

### Phase E2: `v1.24.0`

As [0039-PLAN-refresh-golang-x-requirements.md](0039-PLAN-refresh-golang-x-requirements.md) R2, for `v1.24.0`:

1. **Before the tag.**
   * `make apicheck` reports compatibility with `v1.23.1`.
   * `apidiff` between a `git archive` of `v1.23.1` and the tree lists E1 step 8's 16 additions and nothing else.
   * `scripts/check-release-tag.sh v1.24.0` exits 0.
   * With `main` level with `origin`, the disclosure guard passes.
2. **The tag:** the owner makes the annotated tag `v1.24.0`, message `v1.24.0` as the earlier tags. It goes on E1's commit, or on a later commit that changes only records or D3's or D4's files, and the owner pushes it.
3. **The tag's CI** completes `success`, every job. Its identity legs print `v1.24.0 (release) <12-hex>`.
4. **The pin commit.** `git diff --stat v1.23.1 v1.24.0 -- .github internal/cmd scripts` must be empty, so the workflows are unchanged and only the pins move. They move to `v1.24.0` and its commit, as 0039-PLAN R2 step 4 moved them:
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

* **V1 (redaction):** `TestEnvNeverFormats`, `TestEnvFormatText` and `TestOptionsEnvNeverFormats` pass at E1's commit, on every CI job and on the two test hosts. Probe E1's seven forms (MADR F2.5) are among them, and none prints the secret.
* **V2 (seen failing):** the nine plants each fail as E1 step 6's table says.
* **V3 (the API):** `apidiff` `v1.23.1` → `v1.24.0` lists the 16 methods, each "added", and nothing else. `make apicheck` reports compatibility with `v1.23.1` before the tag.
* **V4 (the gates):** `make gate` `overall=0`, the documentation checks exit 0, and the floor's `vet` and `test` pass under `go1.26.10`.
* **V5 (the release):** the tag's CI is green, its identity legs name `v1.24.0`, and the proxy serves it as `@latest`.

## Rollout and Rollback

* **Rollout:** E1 is one commit on `main`, pushed on the owner's word. The tag follows a green CI run. A program gets the new output when it requires `v1.24.0`.
* **Rollback:** before the tag, `git revert` of E1's commit. After it, the methods are API and stay within `v1`. A fault is fixed forward in `v1.24.1`, and a format that must change is changed there. No document offers the format as stable.

## Execution record

Not started. This plan is `proposed`.
