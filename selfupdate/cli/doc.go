// Package cli is the canonical self-update command for every program built on
// selfupdate: one set of flags, one stream rule, one set of exit codes
// (docs/decisions/0004-MADR-evolve-selfupdate-api-and-tui-support.md §5).
//
//	<prog> update [--check] [--yes|-y] [--force] [--dry-run] [--json]
//	              [--version vX.Y.Z] [--channel NAME]
//
// Stdout carries machine-readable protocol output and nothing else: without
// --json it stays empty, and under --json it is JSON Lines, one event per
// line, then exactly one {"kind":"result",…} object. Everything else goes to
// stderr. The exit status is 0 when up to date, declined or installed, 10
// when --check finds an update, and 1 on any error.
//
// An error after the run did its work, such as a failed unlock once the
// binary is replaced, is a warning, not a failure: the exit status stays 0,
// stderr gets one "warning: …" line for each in either mode, and under
// --json the result object's "result" carries them as its "warnings" array
// (schema_version 2).
//
// A program with the standard flag package calls Command once:
//
//	case "update":
//		os.Exit(cli.Command(ctx, args, product, buildinfo.Identity(), newUpdater, cli.StdioOptions()))
//
// Command builds the updater only after the flags and the request are
// valid. A program that parses its own flags, such as one using cobra,
// binds Flags on its flag set and calls Flags.Request, Run and Exit itself.
// The version comes from package buildinfo, stamped at link time with
// buildinfo.LDFlags.
package cli
