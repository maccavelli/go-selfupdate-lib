package selfupdate

import (
	"os"
	"strings"
)

// Crash leftovers (docs/decisions/0010-MADR-remediate-second-debugging-pass-findings.md
// Q6, B11). An update killed between creating its staging file or its
// backup and removing it leaves the file beside the target. The next
// session, under the target's lock, removes it: no other update of this
// target can be running, so nothing else owns such a file. A backup an
// update reported as the only copy of the previous binary is renamed to
// keptName first, and is not a leftover (0015-MADR B1); so is the backup an
// interrupted update's journal names, before the sweep (0017-MADR 3B).

// leftoverRemove is (*os.Root).Remove, replaced in tests.
var leftoverRemove = (*os.Root).Remove

// isLeftover reports whether name is one this package creates for the
// target base and removes on success: a staging file, as CreateStaging
// names it, or a backup, as randomSibling names it. Both end in the
// decimal number os.CreateTemp chooses, so no other name matches,
// including another target's whose name starts with base.
func isLeftover(base, name string) bool {
	// The Windows cleanup receipt's temporary file, before its rename
	// (0015-MADR B8).
	if digits, ok := strings.CutPrefix(name, "."+base+".selfupdate.cleanup-tmp-"); ok {
		return allDigits(digits)
	}
	// The journal's temporary file, before its rename. The journal itself,
	// .<base>.selfupdate.pending, is not a leftover (0017-MADR 3B).
	if digits, ok := strings.CutPrefix(name, journalTmpPrefix(base)); ok {
		return allDigits(digits)
	}
	rest, ok := strings.CutPrefix(name, "."+base+".selfupdate-")
	if !ok {
		return false
	}
	if digits, isBackup := strings.CutPrefix(rest, "bak-"); isBackup {
		return allDigits(digits)
	}
	if suffix := stagingSuffix(); suffix != "" {
		if rest, ok = strings.CutSuffix(rest, suffix); !ok {
			return false
		}
	}
	return allDigits(rest)
}

// keptName is the name a backup takes when it is the only copy of the
// previous binary, because restoring it failed or because the update was
// interrupted (0017-MADR 3B):
// .<base>.selfupdate-kept-<n> for .<base>.selfupdate-bak-<n>. isLeftover
// matches neither it nor a cleanup receipt's list, so no later session
// removes it; the caller restores or removes it (0015-MADR B1).
func keptName(base, name string) (string, bool) {
	digits, ok := strings.CutPrefix(name, backupPrefix(base))
	if !ok || !allDigits(digits) {
		return "", false
	}
	return "." + base + ".selfupdate-kept-" + digits, true
}

func allDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// removeLeftovers removes the target's leftovers through root, except the
// names in keep: the backups a cleanup receipt still lists. With
// sweepBackups false, because the receipt could not be read, it removes
// staging files only. A name that is not a regular file is not one this
// package wrote, and is left alone, so a symlink is never followed.
// Removal is best-effort: a leftover is harmless, and must never block an
// update.
func removeLeftovers(target Target, root *os.Root, keep map[string]bool, sweepBackups bool) {
	dir, err := root.Open(".")
	if err != nil {
		return
	}
	names, err := dir.Readdirnames(-1)
	advisory(dir.Close())
	if err != nil {
		return
	}
	for _, name := range names {
		if keep[name] || !isLeftover(target.Base, name) {
			continue
		}
		if !sweepBackups && strings.HasPrefix(name, backupPrefix(target.Base)) {
			continue
		}
		info, err := root.Lstat(name)
		if err != nil || !info.Mode().IsRegular() {
			continue
		}
		advisory(leftoverRemove(root, name))
	}
}
