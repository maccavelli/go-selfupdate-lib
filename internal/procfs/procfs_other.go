//go:build !linux

package procfs

// ReadStat returns ErrUnsupported off Linux.
func ReadStat(int) (Stat, error) { return Stat{}, ErrUnsupported }

// Exe returns ErrUnsupported off Linux.
func Exe(int) (string, error) { return "", ErrUnsupported }
