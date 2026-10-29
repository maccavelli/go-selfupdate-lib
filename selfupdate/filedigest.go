package selfupdate

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
)

// The digests of files beside the target. The Windows cleanup receipt and
// the interrupted-update journal both record and check them
// (docs/decisions/0017-PLAN-verify-build-provenance-and-close-0015-open-items.md
// Q1).

// rootFileSHA256 hashes name inside root, refusing a file other than the one
// want describes: the target, staging, or a pending backup, each read
// through the session's root (0038-MADR F8).
func rootFileSHA256(root *os.Root, name string, want os.FileInfo) (digest string, err error) {
	f, err := root.Open(name)
	if err != nil {
		return "", err
	}
	defer func() {
		err = joinClose(err, f)
	}()
	got, err := f.Stat()
	if err != nil {
		return "", err
	}
	if !os.SameFile(want, got) {
		return "", fmt.Errorf("selfupdate: %s changed while it was checked: %w", name, ErrConcurrentUpdate)
	}
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
