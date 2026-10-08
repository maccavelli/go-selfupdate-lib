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

func fileSHA256(path string) (digest string, err error) {
	f, err := openAbsFile(path, os.O_RDONLY, 0)
	if err != nil {
		return "", err
	}
	defer func() {
		err = joinClose(err, f)
	}()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// rootFileSHA256 hashes name inside root, refusing a file other than the one
// want describes.
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
		return "", fmt.Errorf("selfupdate: pending backup changed while it was checked: %w", ErrConcurrentUpdate)
	}
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
