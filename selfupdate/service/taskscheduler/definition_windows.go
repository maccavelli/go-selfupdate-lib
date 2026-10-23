//go:build windows

package taskscheduler

import (
	"strings"

	"golang.org/x/sys/windows"
)

// sameAccount compares two accounts case-insensitively, and resolves a
// SID to the account it names, so that the SID Task Scheduler stores
// equals the DOMAIN\name, or name, a definition gives (0032-MADR D3-A).
func sameAccount(a, b string) bool {
	a, b = strings.TrimSpace(a), strings.TrimSpace(b)
	if strings.EqualFold(a, b) {
		return true
	}
	return sidNames(a, b) || sidNames(b, a)
}

// sidNames reports whether sid is a SID that names account, given as
// DOMAIN\name or name.
func sidNames(sid, account string) bool {
	s, err := windows.StringToSid(sid)
	if err != nil {
		return false
	}
	name, domain, _, err := s.LookupAccount("")
	if err != nil {
		return false
	}
	return strings.EqualFold(account, name) || strings.EqualFold(account, domain+`\`+name)
}
