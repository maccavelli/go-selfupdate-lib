# The pre-add check passes a file gofmt cannot read

Associated record: none in this repository. The same gap was found in
go-tui-lib, whose `scripts/go-precheck.sh` was taken from this one. It is
decided there in go-tui-lib's
`docs/decisions/0015-MADR-precheck-gofmt-errors.md` (proposed on
2026-10-08), with its plan in
`docs/decisions/0015-PLAN-precheck-gofmt-errors.md`.

Date: 2026-10-08

This report records a finding. It decides nothing. Whether and how to fix
it here is this repository's own decision, by its own MADR and PLAN.

## The finding

`scripts/go-precheck.sh` runs gofmt on the Go files it checks
(`scripts/go-precheck.sh:95-105`):

```bash
unformatted="$(gofmt -l "${files[@]}")"
if [ -n "$unformatted" ]; then
```

It reads gofmt's standard output, the list of unformatted files, and not
gofmt's exit status. gofmt fails without printing a file name in two
cases. A probe on 2026-10-08 with the Go toolchain's gofmt:

| `gofmt -l` on | Exit status | Standard output | Standard error |
| :--- | ---: | :--- | :--- |
| a formatted file | 0 | nothing | nothing |
| an unformatted file | 0 | the file's name | nothing |
| a missing file | 2 | nothing | `lstat missing.go: no such file or directory` |
| a file that does not parse | 2 | nothing | `broken.go:3:9: expected ')', found '{'` |

So the gofmt step passes a file gofmt could not read or parse.

The two paths collect their files differently (`scripts/go-precheck.sh:53-70`):

- **With a file list,** only arguments that exist go to gofmt
  (`[ -f "$f" ]`, line 61), and every Go argument names a package
  directory for `go vet` and `go test`.
- **With no list,** every `git ls-files '*.go'` entry goes to gofmt, with
  no existence check (lines 66-69), including a tracked file deleted from
  the work tree but not yet staged.

The script's test, `scripts/go-precheck_test.sh`, stubs gofmt with a
script that records its arguments and exits 0 (lines 34-37). It cannot
observe either failure.

## What it does here

The tree's own `scripts/go-precheck.sh`, run on a scratch clone at
`6dcdd8a` on 2026-10-08, with `GO_PRECHECK_SKIP_VULN=1`:

| Case | Exit | What reported it |
| :--- | ---: | :--- |
| G0, the control: `buildinfo/buildinfo.go` as it is | 0 | "1 file(s) clean (gofmt, golangci-lint, go vet, go test)" |
| G1, a file in `buildinfo` that does not parse, in a file list | 1 | golangci-lint (`typecheck`), `go vet` and `go test`; gofmt's own message printed, but did not fail the step |
| G2, a file in the `testdata/fixture` module that does not parse, in a file list | 1 | `go vet` and `go test`; gofmt's message printed, not counted |
| G3, `buildinfo/stamp_test.go` deleted from the work tree, no list | 0 | gofmt printed "lstat buildinfo/stamp_test.go: no such file or directory"; the run reported "280 file(s) clean" |

- **A file that does not parse** fails the precheck here: the later
  steps catch it, because this script vets and tests every package
  directory a file is in, `testdata` included. The gap only moves the
  failure from gofmt to the later steps.
- **A tracked file deleted from the work tree** passes the no-list path,
  and is counted among the clean files. The precheck reports success
  while gofmt reported an error.

## How it differs from go-tui-lib

In go-tui-lib, a Go file under `testdata` is formatted but not vetted or
tested: its framework examples import modules the library does not
require (go-tui-lib's `docs/decisions/0014-PLAN-api-policy-gates.md`,
deviation D5). There, a file that does not parse can pass the precheck
outright, which is why go-tui-lib decided to fix the step. Here, only the
deleted-file case passes, and only on the no-list path:
`make pre-add-check` without `FILES`, and `make release-check`.

## What a fix would look like

go-tui-lib's proposed fix, recorded for reference:

- the gofmt step captures gofmt's standard error and exit status, and a
  non-zero status fails the step, showing gofmt's message;
- the no-list path keeps a tracked file only if the work tree has it
  (`[ -f "$f" ]`), as the file-list path does;
- a test that runs the real gofmt, not a stub that always exits 0, on a
  file that does not parse and on a deleted tracked file.

Here the gofmt stub in `scripts/go-precheck_test.sh` would have to fail
on demand (say, through an environment variable, as the `go` and
`govulncheck` stubs do), or one case would run the real gofmt.
