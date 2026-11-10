// Package errprefix composes a wrapped error's text so that it names the
// package once. A wrap such as "selfupdate: stat target" around an error
// whose own text begins "selfupdate: " drops that inner prefix
// (docs/decisions/0043-MADR-name-the-package-once-in-wrapped-errors.md
// D2-A).
package errprefix

import "strings"

// prefix is the text every error of this module begins with.
const prefix = "selfupdate: "

// Error is an error with the operation that failed. Its text is Op, then
// the text of Err with one leading "selfupdate: " removed; only the
// text's start is touched, so the later parts of an errors.Join keep
// their own (0043-MADR D3-A).
type Error struct {
	// Op names where the error happened, such as "selfupdate: stat
	// target" or "selfupdate: demo".
	Op string
	// Err is the error that happened.
	Err error
}

// Error returns Op, ": ", and Err's text without its leading prefix.
func (e *Error) Error() string {
	return e.Op + ": " + trim(e.Err.Error())
}

// Unwrap returns Err, so errors.Is and errors.As see through the wrap.
func (e *Error) Unwrap() error { return e.Err }

// Wrap returns err wrapped with op, as an *Error, or nil when err is nil.
func Wrap(op string, err error) error {
	if err == nil {
		return nil
	}
	return &Error{Op: op, Err: err}
}

// Strip returns err with one leading "selfupdate: " dropped from its
// text, for a composition Wrap does not fit. An error whose text does not
// begin with it is returned as itself, and nil stays nil. The result
// unwraps to err.
func Strip(err error) error {
	if err == nil || !strings.HasPrefix(err.Error(), prefix) {
		return err
	}
	return stripped{err}
}

// stripped is an error whose text is its cause's, without the prefix.
type stripped struct{ err error }

func (s stripped) Error() string { return trim(s.err.Error()) }

func (s stripped) Unwrap() error { return s.err }

// trim removes one leading prefix from s.
func trim(s string) string { return strings.TrimPrefix(s, prefix) }
