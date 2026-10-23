//go:build !windows

package taskscheduler

import "strings"

// sameAccount compares two accounts as text, case-insensitively: off
// Windows a SID cannot be resolved to the account it names.
func sameAccount(a, b string) bool {
	return strings.EqualFold(strings.TrimSpace(a), strings.TrimSpace(b))
}
