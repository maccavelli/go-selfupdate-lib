package selfupdate

import (
	"encoding/json"
	"strings"
)

// Warnings lists, sanitized and in order, the errors that arrived after a
// run's EventComplete: each was also reported as an EventWarning, and none
// failed the run (0010-MADR Q3).
//
// Its JSON form is an array of strings. It is a string, its entries joined
// by newlines, so Result and ResultDocument stay comparable and == compares
// warnings by value (0010-MADR amendment A4). An entry never holds a
// newline: NewWarnings and Add sanitize each one, as event text is, and
// drop one that is empty after sanitizing. The zero value has no entries.
type Warnings string

// NewWarnings returns the given warnings, sanitized.
func NewWarnings(warnings ...string) Warnings {
	return Warnings("").Add(warnings...)
}

// Add returns w with each warning, sanitized, after the entries it has.
func (w Warnings) Add(warnings ...string) Warnings {
	entries := w.List()
	for _, s := range warnings {
		if s = sanitizeText(s); s != "" {
			entries = append(entries, s)
		}
	}
	return Warnings(strings.Join(entries, "\n"))
}

// List returns the entries, or nil when there are none.
func (w Warnings) List() []string {
	if w == "" {
		return nil
	}
	return strings.Split(string(w), "\n")
}

// Len returns the number of entries.
func (w Warnings) Len() int {
	if w == "" {
		return 0
	}
	return strings.Count(string(w), "\n") + 1
}

// MarshalJSON implements json.Marshaler: the entries as an array, which is
// [] when there are none.
func (w Warnings) MarshalJSON() ([]byte, error) {
	list := w.List()
	if list == nil {
		list = []string{}
	}
	return json.Marshal(list)
}

// UnmarshalJSON implements json.Unmarshaler. It reads an array of strings,
// or null, and sanitizes each entry as NewWarnings does.
func (w *Warnings) UnmarshalJSON(b []byte) error {
	var list []string
	if err := json.Unmarshal(b, &list); err != nil {
		return err
	}
	*w = NewWarnings(list...)
	return nil
}
