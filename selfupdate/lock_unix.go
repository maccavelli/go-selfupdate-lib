//go:build unix

package selfupdate

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"golang.org/x/sys/unix"
)

type lockHandle struct {
	file *os.File
}

func lockBasename(base string) string {
	return "." + base + ".selfupdate.lock"
}

func acquireLock(ctx context.Context, root *os.Root, base string, timeout time.Duration) (lockHandle, error) {
	name := lockBasename(base)
	deadline := time.Now().Add(timeout)
	for {
		if err := ctx.Err(); err != nil {
			return lockHandle{}, err
		}
		h, err := tryAcquireLock(root, name)
		if err == nil {
			return h, nil
		}
		if !isLockBusy(err) {
			return lockHandle{}, err
		}
		if time.Now().After(deadline) {
			return lockHandle{}, ErrConcurrentUpdate
		}
		timer := time.NewTimer(10 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return lockHandle{}, ctx.Err()
		case <-timer.C:
		}
	}
}

// lockOpenFlags keeps a final-component symlink from being followed.
const lockOpenFlags = unix.O_NOFOLLOW

func tryAcquireLock(root *os.Root, name string) (lockHandle, error) {
	f, err := openLockFile(root, name)
	if err != nil {
		return lockHandle{}, err
	}
	info, err := f.Stat()
	if err != nil {
		return lockHandle{}, joinClose(err, f)
	}
	if !info.Mode().IsRegular() {
		return lockHandle{}, joinClose(fmt.Errorf("selfupdate: lock is not a regular file"), f)
	}
	if err := unix.Flock(int(f.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
		return lockHandle{}, joinClose(err, f)
	}
	return lockHandle{file: f}, nil
}

func isLockBusy(err error) bool {
	return errors.Is(err, unix.EWOULDBLOCK) || errors.Is(err, unix.EAGAIN)
}

func (h lockHandle) release() error {
	if h.file == nil {
		return nil
	}
	err := unix.Flock(int(h.file.Fd()), unix.LOCK_UN)
	if cerr := h.file.Close(); err == nil {
		err = cerr
	}
	return err
}
