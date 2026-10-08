//go:build windows

package selfupdate

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"golang.org/x/sys/windows"
)

// cleanupEntry is one pending backup: its basename and the digest it had.
type cleanupEntry struct {
	Backup string `json:"backup"`
	Digest string `json:"digest"`
}

// cleanupReceipt lists the backups a commit could not remove because a
// running image held them. Version 1 holds one entry in Backup and Digest;
// version 2 holds several in Backups. A receipt with one entry is written
// as version 1, which every earlier release reads (0010-MADR amendment A1).
type cleanupReceipt struct {
	Version int            `json:"version"`
	Backup  string         `json:"backup,omitempty"`
	Digest  string         `json:"digest,omitempty"`
	Backups []cleanupEntry `json:"backups,omitempty"`
}

var errMalformedReceipt = errors.New("selfupdate: malformed cleanup receipt")

// entries returns the receipt's pending backups, refusing a receipt that
// mixes the two versions' fields or names an incomplete entry.
func (r cleanupReceipt) entries() ([]cleanupEntry, error) {
	switch r.Version {
	case 1:
		if r.Backup == "" || r.Digest == "" || len(r.Backups) != 0 {
			return nil, errMalformedReceipt
		}
		return []cleanupEntry{{Backup: r.Backup, Digest: r.Digest}}, nil
	case 2:
		if len(r.Backups) == 0 || r.Backup != "" || r.Digest != "" {
			return nil, errMalformedReceipt
		}
		for _, e := range r.Backups {
			if e.Backup == "" || e.Digest == "" {
				return nil, errMalformedReceipt
			}
		}
		return r.Backups, nil
	}
	return nil, errMalformedReceipt
}

// receiptFor is the receipt that lists entries: version 1 for one entry.
func receiptFor(entries []cleanupEntry) cleanupReceipt {
	if len(entries) == 1 {
		return cleanupReceipt{Version: 1, Backup: entries[0].Backup, Digest: entries[0].Digest}
	}
	return cleanupReceipt{Version: 2, Backups: entries}
}

// parseCleanupReceipt decodes a receipt and validates every backup name.
func parseCleanupReceipt(target Target, data []byte) ([]cleanupEntry, error) {
	var rec cleanupReceipt
	if err := json.Unmarshal(data, &rec); err != nil {
		return nil, fmt.Errorf("selfupdate: malformed cleanup receipt: %w", err)
	}
	entries, err := rec.entries()
	if err != nil {
		return nil, err
	}
	for _, e := range entries {
		if err := validateReceiptBackup(target, e.Backup); err != nil {
			return nil, err
		}
	}
	return entries, nil
}

func processCleanupReceipt(target Target, root *os.Root) error {
	name := cleanupReceiptName(target.Base)
	info, err := root.Lstat(name)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("selfupdate: stat cleanup receipt: %w", err)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("selfupdate: cleanup receipt is not a regular file")
	}
	if isReparsePoint(filepath.Join(target.Dir, name)) {
		return fmt.Errorf("selfupdate: cleanup receipt is a reparse point")
	}
	data, err := root.ReadFile(name)
	if err != nil {
		return fmt.Errorf("selfupdate: read cleanup receipt: %w", err)
	}
	entries, err := parseCleanupReceipt(target, data)
	if err != nil {
		return err
	}
	var kept []cleanupEntry
	for _, e := range entries {
		keep, err := removePendingBackup(target, root, e)
		if err != nil {
			return err
		}
		if keep {
			kept = append(kept, e)
		}
	}
	switch {
	case len(kept) == len(entries):
		// Every backup is still held by a running image: nothing changed,
		// and the session continues (0010-MADR B5).
		return nil
	case len(kept) == 0:
		if err := root.Remove(name); err != nil {
			return fmt.Errorf("selfupdate: remove cleanup receipt: %w", err)
		}
	default:
		if err := writeCleanupEntries(target, kept); err != nil {
			return fmt.Errorf("selfupdate: rewrite cleanup receipt: %w", err)
		}
	}
	return syncDirFn(target.Dir)
}

// removePendingBackup removes one pending backup through the root. It
// reports keep when a running image still holds the backup: that is not an
// error, and the entry stays for a later session (0010-MADR B5).
func removePendingBackup(target Target, root *os.Root, e cleanupEntry) (keep bool, err error) {
	backupPath := filepath.Join(target.Dir, e.Backup)
	binfo, err := root.Lstat(e.Backup)
	if errors.Is(err, os.ErrNotExist) {
		// The backup is already gone (a crash between the two removals, or
		// a manual delete): the entry has nothing left to protect, and
		// keeping it would block every later update (0003-MADR B7).
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("selfupdate: stat pending backup: %w", err)
	}
	if !binfo.Mode().IsRegular() || isReparsePoint(backupPath) {
		return false, fmt.Errorf("selfupdate: pending backup is not a regular file")
	}
	// Hash through the root, the same directory the removal below goes
	// through, and only the file that was just checked (0004-MADR R5).
	got, err := rootFileSHA256(root, e.Backup, binfo)
	if err != nil {
		return false, err
	}
	if got != e.Digest {
		return false, fmt.Errorf("selfupdate: pending backup digest mismatch: %w", ErrIntegrity)
	}
	if err := root.Remove(e.Backup); err != nil {
		if isBusyRunningImage(err) {
			return true, nil
		}
		return false, fmt.Errorf("selfupdate: remove pending backup: %w", err)
	}
	return false, nil
}

// writeCleanupReceipt adds result's backup to the target's receipt,
// creating it when there is none. The caller holds the session's lock.
func writeCleanupReceipt(target Target, result applyResult) error {
	entries, err := readCleanupEntries(target)
	if err != nil {
		return err
	}
	entries = append(entries, cleanupEntry{Backup: filepath.Base(result.backup), Digest: result.oldDigest})
	return writeCleanupEntries(target, entries)
}

// readCleanupEntries reads the target's receipt by path, under the lock:
// none when there is no receipt.
func readCleanupEntries(target Target) ([]cleanupEntry, error) {
	path := cleanupReceiptPath(target)
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("selfupdate: stat cleanup receipt: %w", err)
	}
	if !info.Mode().IsRegular() || isReparsePoint(path) {
		return nil, fmt.Errorf("selfupdate: cleanup receipt is not a regular file")
	}
	data, err := os.ReadFile(path) //nolint:gosec // the target's own receipt, under its lock
	if err != nil {
		return nil, fmt.Errorf("selfupdate: read cleanup receipt: %w", err)
	}
	return parseCleanupReceipt(target, data)
}

// listedBackups names the backups the receipt still lists, which a
// running image holds: the leftover sweep keeps them. ok is false when the
// receipt cannot be read, and then no backup may be swept.
func listedBackups(target Target) (names map[string]bool, ok bool) {
	entries, err := readCleanupEntries(target)
	if err != nil {
		return nil, false
	}
	names = make(map[string]bool, len(entries))
	for _, e := range entries {
		names[e.Backup] = true
	}
	return names, true
}

// writeCleanupEntries replaces the target's receipt with one listing
// entries: a new file, restricted to the current user, then a replace, so a
// reader never sees a partial receipt.
func writeCleanupEntries(target Target, entries []cleanupEntry) error {
	data, err := json.Marshal(receiptFor(entries))
	if err != nil {
		return err
	}
	tmp, err := randomSibling(target.Dir, "."+target.Base+".selfupdate.cleanup-tmp-")
	if err != nil {
		return err
	}
	f, err := openAbsFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	if _, err := f.Write(data); err != nil {
		return joinRemove(joinClose(err, f), tmp)
	}
	if err := f.Sync(); err != nil {
		return joinRemove(joinClose(err, f), tmp)
	}
	if err := f.Close(); err != nil {
		return joinRemove(err, tmp)
	}
	if err := restrictToCurrentUser(tmp); err != nil {
		return joinRemove(err, tmp)
	}
	if err := moveFileReplace(context.Background(), tmp, cleanupReceiptPath(target)); err != nil {
		return joinRemove(err, tmp)
	}
	return nil
}

func restrictToCurrentUser(path string) (err error) {
	var token windows.Token
	if err := windows.OpenProcessToken(windows.CurrentProcess(), windows.TOKEN_QUERY, &token); err != nil {
		return err
	}
	defer func() {
		err = errors.Join(err, token.Close())
	}()
	user, err := token.GetTokenUser()
	if err != nil {
		return err
	}
	sid := user.User.Sid
	dacl, err := windows.ACLFromEntries([]windows.EXPLICIT_ACCESS{{
		AccessPermissions: windows.GENERIC_ALL,
		AccessMode:        windows.GRANT_ACCESS,
		Inheritance:       windows.NO_INHERITANCE,
		Trustee: windows.TRUSTEE{
			TrusteeForm:  windows.TRUSTEE_IS_SID,
			TrusteeType:  windows.TRUSTEE_IS_USER,
			TrusteeValue: windows.TrusteeValueFromSID(sid),
		},
	}}, nil)
	if err != nil {
		return err
	}
	sec, err := windows.NewSecurityDescriptor()
	if err != nil {
		return err
	}
	if err := sec.SetDACL(dacl, true, false); err != nil {
		return err
	}
	if err := sec.SetOwner(sid, false); err != nil {
		return err
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	return windows.SetNamedSecurityInfo(
		abs,
		windows.SE_FILE_OBJECT,
		windows.OWNER_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION,
		sid,
		nil,
		dacl,
		nil,
	)
}
