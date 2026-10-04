//go:build !unix && !windows

package service

// startDetached is unsupported where there is neither a Unix session nor a
// Windows process group.
func startDetached(string, []string, []string) (int, error) {
	return 0, ErrUnsupported
}
