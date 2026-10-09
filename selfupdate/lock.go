package selfupdate

import (
	"errors"
	"fmt"
	"os"
)

// lockOpenHook runs at each stage of openLockFile. Tests use it to change
// the lock between the Lstat and the open; it does nothing otherwise.
var lockOpenHook = func(stage string) {}

// openLockFile opens the lock file name inside root without following a
// symlink. os.Root follows a final-component symlink that stays inside the
// root, whatever O_NOFOLLOW says, so the name is checked with Lstat first,
// created only with O_EXCL (which never follows), and compared with
// os.SameFile after opening (0003-MADR B5). lockOpenFlags, set per OS, are
// ORed into the open flags.
func openLockFile(root *os.Root, name string) (*os.File, error) {
	for range 2 {
		lockOpenHook("before-lstat")
		pre, err := root.Lstat(name)
		lockOpenHook("after-lstat")
		if errors.Is(err, os.ErrNotExist) {
			f, cerr := root.OpenFile(name, os.O_RDWR|os.O_CREATE|os.O_EXCL|lockOpenFlags, 0o600)
			if errors.Is(cerr, os.ErrExist) {
				continue // created concurrently: examine what is there now
			}
			if cerr != nil {
				return nil, fmt.Errorf("selfupdate: create lock: %w", cerr)
			}
			return f, nil
		}
		if err != nil {
			return nil, fmt.Errorf("selfupdate: stat lock: %w", err)
		}
		if !pre.Mode().IsRegular() {
			return nil, fmt.Errorf("selfupdate: lock is not a regular file")
		}
		f, err := root.OpenFile(name, os.O_RDWR|lockOpenFlags, 0)
		if err != nil {
			return nil, fmt.Errorf("selfupdate: open lock: %w", err)
		}
		post, err := f.Stat()
		if err != nil {
			return nil, joinClose(err, f)
		}
		if !os.SameFile(pre, post) {
			return nil, joinClose(fmt.Errorf("selfupdate: lock changed while opening"), f)
		}
		return f, nil
	}
	return nil, fmt.Errorf("selfupdate: lock kept changing while opening")
}
