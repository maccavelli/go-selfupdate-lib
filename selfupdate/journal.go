package selfupdate

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// The interrupted-update journal
// (docs/decisions/0017-MADR-verify-build-provenance-and-close-0015-open-items.md
// 3B). An update writes .<base>.selfupdate.pending beside the target before
// it replaces it, and removes it once the replacement is committed or
// rolled back. A session that finds one, under the lock, knows an update
// was interrupted in between, and keeps the backup it names instead of
// sweeping it: it may be the only copy of the previous binary.

// pendingJournal is .<base>.selfupdate.pending. Every schema keeps Backup,
// so an older reader can still keep it.
type pendingJournal struct {
	Schema    int    `json:"schema"`
	Backup    string `json:"backup"`
	OldDigest string `json:"old_digest"`
	NewDigest string `json:"new_digest"`
	Phase     string `json:"phase"`
	Product   string `json:"product,omitempty"`
}

const (
	journalSchema = 1
	// journalApplying is the one phase written: before the rename, with
	// Commit or Rollback not yet finished. Recovery decides from what is on
	// disk, not from the phase (0017-PLAN N8).
	journalApplying = "applying"
)

func journalName(base string) string {
	return "." + base + ".selfupdate.pending"
}

// journalTmpPrefix names the journal's temporary file before its rename;
// isLeftover matches it.
func journalTmpPrefix(base string) string {
	return "." + base + ".selfupdate.pending-tmp-"
}

var (
	// writeJournalFn is writeJournal, replaced in tests.
	writeJournalFn = writeJournal
	// keptRenameFn is (*os.Root).Rename for recovery's kept rename,
	// replaced in tests.
	keptRenameFn = (*os.Root).Rename
)

// writeJournal writes j as target's journal through root: a private
// temporary file, synced and renamed over the journal's name, then the
// directory synced, so the journal is on disk before the target is
// replaced.
func writeJournal(root *os.Root, target Target, j pendingJournal) error {
	data, err := json.Marshal(j)
	if err != nil {
		return err
	}
	tmpPath, err := randomSibling(target.Dir, journalTmpPrefix(target.Base))
	if err != nil {
		return err
	}
	tmp := filepath.Base(tmpPath)
	f, err := root.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	if _, err := f.Write(data); err != nil {
		return errors.Join(err, f.Close(), root.Remove(tmp))
	}
	if err := f.Sync(); err != nil {
		return errors.Join(err, f.Close(), root.Remove(tmp))
	}
	if err := f.Close(); err != nil {
		return errors.Join(err, root.Remove(tmp))
	}
	if err := restrictJournal(filepath.Join(target.Dir, tmp)); err != nil {
		return errors.Join(err, root.Remove(tmp))
	}
	if err := root.Rename(tmp, journalName(target.Base)); err != nil {
		return errors.Join(err, root.Remove(tmp))
	}
	return syncRootFn(root)
}

// removeJournal removes target's journal through root, if there is one. It
// is advisory: a journal whose backup is gone is dropped by the next
// session.
func removeJournal(root *os.Root, target Target) {
	if err := root.Remove(journalName(target.Base)); err == nil {
		advisory(syncRootFn(root))
	}
}

// recoverJournal handles a journal an earlier, interrupted update left, and
// returns the leftover sweep's keep list and backup switch. It never fails:
// CleanupPending's contract has no error for it (0017-MADR, question 4). The
// rules apply in order, and the first that holds decides
// (0017-PLAN Q1 step 2):
//
//  1. no journal: nothing;
//  2. not a regular file, or a reparse point: left, and no backup swept;
//  3. unreadable, or naming no backup of this target: the same;
//  4. its backup is gone: the journal is removed;
//  5. its backup is on the cleanup receipt: the journal is removed;
//  6. its backup is not a regular file: left, and no backup swept;
//  7. the target is still the previous binary (the backup's own file, or
//     its digest): the backup is redundant, and the journal is removed;
//  8. otherwise the backup is the only copy of the previous binary: it is
//     renamed to its kept name, or a fresh kept name when that is taken,
//     and the journal is removed. When the rename fails, the backup is kept
//     where it is, and the journal stays for the next session.
func recoverJournal(target Target, root *os.Root, keep map[string]bool, sweep bool) (map[string]bool, bool) {
	name := journalName(target.Base)
	info, err := root.Lstat(name)
	if errors.Is(err, fs.ErrNotExist) {
		return keep, sweep
	}
	if err != nil || !info.Mode().IsRegular() || journalReparse(filepath.Join(target.Dir, name)) {
		return keep, false
	}
	j, ok := readJournal(root, target, name)
	if !ok {
		return keep, false
	}
	binfo, err := root.Lstat(j.Backup)
	switch {
	case errors.Is(err, fs.ErrNotExist), err == nil && keep[j.Backup]:
		removeJournal(root, target)
		return keep, sweep
	case err != nil || !binfo.Mode().IsRegular() || journalReparse(filepath.Join(target.Dir, j.Backup)):
		return keep, false
	}
	if previousStillLive(root, target, binfo, j.OldDigest) {
		removeJournal(root, target)
		return keep, sweep
	}
	if !keepInterrupted(root, target, j.Backup) {
		if keep == nil {
			keep = map[string]bool{}
		}
		keep[j.Backup] = true
		return keep, sweep
	}
	removeJournal(root, target)
	return keep, sweep
}

// readJournal reads and checks the journal: a schema of 1 or later, and a
// backup this package names, with digits a kept name can take.
func readJournal(root *os.Root, target Target, name string) (pendingJournal, bool) {
	data, err := root.ReadFile(name)
	if err != nil {
		return pendingJournal{}, false
	}
	var j pendingJournal
	if err := json.Unmarshal(data, &j); err != nil || j.Schema < 1 {
		return pendingJournal{}, false
	}
	if validateReceiptBackup(target, j.Backup) != nil {
		return pendingJournal{}, false
	}
	if _, ok := keptName(target.Base, j.Backup); !ok {
		return pendingJournal{}, false
	}
	return j, true
}

// previousStillLive reports whether the target is still the previous binary:
// the backup's own file, or a file with the digest it had.
func previousStillLive(root *os.Root, target Target, backup os.FileInfo, oldDigest string) bool {
	tinfo, err := root.Lstat(target.Base)
	if err != nil || !tinfo.Mode().IsRegular() {
		return false
	}
	if os.SameFile(backup, tinfo) {
		return true
	}
	digest, err := rootFileSHA256(root, target.Base, tinfo)
	return err == nil && oldDigest != "" && digest == oldDigest
}

// keepInterrupted renames an interrupted update's backup to its kept name,
// or to a fresh kept name when that is taken, never over another file, and
// drops its setuid and setgid bits as retainLocked does. It reports whether
// the backup now has a kept name.
func keepInterrupted(root *os.Root, target Target, backup string) bool {
	kept, _ := keptName(target.Base, backup)
	if _, err := root.Lstat(kept); err == nil {
		fresh, err := randomSibling(target.Dir, "."+target.Base+".selfupdate-kept-")
		if err != nil {
			return false
		}
		kept = filepath.Base(fresh)
	}
	if err := keptRenameFn(root, backup, kept); err != nil {
		return false
	}
	advisory(syncRootFn(root))
	advisory(clearSpecialBits(filepath.Join(target.Dir, kept)))
	return true
}

// errJournalPending refuses an update while an earlier one's journal is
// still there: recovery could not resolve it, and replacing it would let the
// sweep take the backup it protects (0017-PLAN N9).
func errJournalPending(target Target) error {
	return fmt.Errorf("selfupdate: an earlier update's journal %s is pending; resolve it, then retry",
		filepath.Join(target.Dir, journalName(target.Base)))
}
