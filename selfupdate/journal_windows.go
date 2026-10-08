//go:build windows

package selfupdate

// restrictJournal gives the journal an owner-only DACL, as the cleanup
// receipt has (0017-PLAN Q1).
func restrictJournal(path string) error {
	return restrictToCurrentUser(path)
}

// journalReparse reports a reparse point, which recovery never follows.
func journalReparse(path string) bool {
	return isReparsePoint(path)
}
