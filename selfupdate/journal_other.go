//go:build !windows

package selfupdate

// restrictJournal has nothing to add here: the journal is created 0600
// (0017-PLAN Q1).
func restrictJournal(string) error {
	return nil
}

// journalReparse is false here: a symlink is not a regular file, which
// recovery already refuses.
func journalReparse(string) bool {
	return false
}
