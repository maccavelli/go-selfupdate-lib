//go:build linux

package procfs

import (
	"fmt"
	"os"
	"strconv"
)

// ReadStat reads /proc/<pid>/stat once. A process that does not exist is an
// error wrapping fs.ErrNotExist.
func ReadStat(pid int) (Stat, error) {
	if pid <= 0 {
		return Stat{}, fmt.Errorf("selfupdate: procfs: invalid pid %d", pid)
	}
	raw, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/stat") //nolint:gosec // a positive integer PID under /proc, checked above
	if err != nil {
		return Stat{}, fmt.Errorf("selfupdate: procfs: %w", err)
	}
	s, err := parseStat(string(raw))
	if err != nil {
		return Stat{}, fmt.Errorf("%w (pid %d)", err, pid)
	}
	return s, nil
}

// Exe reads the /proc/<pid>/exe link: the binary the process runs. After
// the binary is replaced by rename it reads "<path> (deleted)" (0035-MADR
// Amendment A1, P1). A process that does not exist is an error wrapping
// fs.ErrNotExist.
func Exe(pid int) (string, error) {
	if pid <= 0 {
		return "", fmt.Errorf("selfupdate: procfs: invalid pid %d", pid)
	}
	path, err := os.Readlink("/proc/" + strconv.Itoa(pid) + "/exe")
	if err != nil {
		return "", fmt.Errorf("selfupdate: procfs: %w", err)
	}
	return path, nil
}
