package service

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// SameExecutable reports whether a and b name the same binary: the same
// path once cleaned, case-insensitively on Windows; or, failing that, the
// same file, by os.SameFile. The second matches a Windows 8.3 short name, a
// junction, or a path through a symlinked directory. A path that cannot be
// read matches only by its text. Each backend's Reconcile uses it to
// recognise the binary a definition runs (0011-MADR amendment A5).
func SameExecutable(a, b string) bool {
	ca, cb := filepath.Clean(a), filepath.Clean(b)
	if ca == cb || (runtime.GOOS == "windows" && strings.EqualFold(ca, cb)) {
		return true
	}
	ia, err := os.Stat(ca)
	if err != nil {
		return false
	}
	ib, err := os.Stat(cb)
	if err != nil {
		return false
	}
	return os.SameFile(ia, ib)
}
