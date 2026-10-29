package selfupdate

import (
	"fmt"
	"path/filepath"
	"strings"
)

func cleanupReceiptName(base string) string {
	return "." + base + ".selfupdate.cleanup"
}

// backupPrefix is the name prefix of every backup backupFile makes.
func backupPrefix(base string) string {
	return "." + base + ".selfupdate-bak-"
}

// validateReceiptBackup accepts only what writeCleanupReceipt records: the
// bare basename of a backup of this target, never the target, the lock, a
// path, or a traversal (0003-MADR B6). It is portable so every OS tests it.
func validateReceiptBackup(target Target, name string) error {
	if name == "" || !filepath.IsLocal(name) || filepath.Base(name) != name || strings.ContainsAny(name, `/\`) {
		return fmt.Errorf("selfupdate: cleanup receipt backup is not a basename")
	}
	if name == target.Base || !strings.HasPrefix(name, backupPrefix(target.Base)) || len(name) == len(backupPrefix(target.Base)) {
		return fmt.Errorf("selfupdate: cleanup receipt backup is not a backup of %s", target.Base)
	}
	return nil
}

func cleanupReceiptPath(target Target) string {
	return filepath.Join(target.Dir, cleanupReceiptName(target.Base))
}
