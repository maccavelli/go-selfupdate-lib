package errprefix

import (
	"errors"
	"fmt"
	"testing"
)

var errSentinel = errors.New("selfupdate: integrity check failed")

// codeError is an error type of the test's own, for errors.As.
type codeError struct{ code int }

func (e *codeError) Error() string { return fmt.Sprintf("selfupdate: code %d", e.code) }

func TestWrapAndStrip(t *testing.T) {
	t.Parallel()
	plain := errors.New("open x: no such file or directory")
	coded := &codeError{code: 7}
	for _, c := range []struct {
		name       string
		err        error
		wrap, trip string // want: Wrap("selfupdate: op", err), Strip(err)
		is         error
	}{
		{"prefixed sentinel", errSentinel,
			"selfupdate: op: integrity check failed", "integrity check failed", errSentinel},
		{"wrapped sentinel", fmt.Errorf("selfupdate: missing digest: %w", errSentinel),
			"selfupdate: op: missing digest: selfupdate: integrity check failed",
			"missing digest: selfupdate: integrity check failed", errSentinel},
		{"no prefix", plain,
			"selfupdate: op: open x: no such file or directory", "open x: no such file or directory", plain},
		{"prefix later only", fmt.Errorf("read x: %w", errSentinel),
			"selfupdate: op: read x: selfupdate: integrity check failed",
			"read x: selfupdate: integrity check failed", errSentinel},
		{"join, both prefixed", errors.Join(errSentinel, errors.New("selfupdate: close failed")),
			"selfupdate: op: integrity check failed\nselfupdate: close failed",
			"integrity check failed\nselfupdate: close failed", errSentinel},
		{"own type", coded, "selfupdate: op: code 7", "code 7", coded},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			w := Wrap("selfupdate: op", c.err)
			if got := w.Error(); got != c.wrap {
				t.Errorf("Wrap text = %q, want %q", got, c.wrap)
			}
			s := Strip(c.err)
			if got := s.Error(); got != c.trip {
				t.Errorf("Strip text = %q, want %q", got, c.trip)
			}
			for _, e := range []error{w, s} {
				if !errors.Is(e, c.is) {
					t.Errorf("%q: errors.Is(%v) = false", e, c.is)
				}
				// Identity, compared as interfaces: the case's own value.
				var ce *codeError
				if got := errors.As(e, &ce); got != (any(c.err) == any(coded)) || got && ce != coded {
					t.Errorf("%q: errors.As(*codeError) = %t, %v", e, got, ce)
				}
			}
			var pe *Error
			if !errors.As(w, &pe) || pe.Op != "selfupdate: op" || any(pe.Err) != any(c.err) {
				t.Errorf("Wrap is not an *Error with Op and Err: %#v", w)
			}
		})
	}
}

func TestStripReturnsUnprefixedErrorItself(t *testing.T) {
	t.Parallel()
	plain := errors.New("open x: no such file or directory")
	// Identity, compared as interfaces: the same value, not a wrapper.
	if got := Strip(plain); any(got) != any(plain) {
		t.Errorf("Strip of an unprefixed error = %#v, want the error itself", got)
	}
}

func TestNil(t *testing.T) {
	t.Parallel()
	if Wrap("selfupdate: op", nil) != nil {
		t.Error("Wrap(nil) is not nil")
	}
	if Strip(nil) != nil {
		t.Error("Strip(nil) is not nil")
	}
}
