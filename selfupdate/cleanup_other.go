//go:build !windows

package selfupdate

import (
	"os"
)

func processCleanupReceipt(target Target, root *os.Root) error {
	name := cleanupReceiptName(target.Base)
	_, err := root.Lstat(name)
	if err == nil {
		// Remove through the root that was checked, not the path
		// (0004-MADR R5).
		return root.Remove(name)
	}
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

// listedBackups names the backups a cleanup receipt still lists. Only
// Windows writes receipts.
func listedBackups(Target) (map[string]bool, bool) {
	return nil, true
}
