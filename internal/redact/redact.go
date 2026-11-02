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
