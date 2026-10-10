//go:build windows

package winproc

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// getConsoleProcessList is not in x/sys/windows.
var getConsoleProcessList = windows.NewLazySystemDLL("kernel32.dll").NewProc("GetConsoleProcessList")

// hasConsole reports whether this process is attached to a console:
// GetConsoleProcessList returns how many processes share it, and 0 when
// there is none. GetConsoleWindow is not used, because it returns 0 under
// ConPTY for a process that has a console (0021-MADR D7, from
// magic-cli-remote's 0159 D17). The answer is read once.
var hasConsole = sync.OnceValue(func() bool {
	var pids [1]uint32
	n, _, _ := getConsoleProcessList.Call(uintptr(unsafe.Pointer(&pids[0])), uintptr(len(pids))) //nolint:errcheck,gosec // a fixed buffer for a Win32 call; 0 on any failure already means no console to inherit
	return n != 0
})

// checkProgram refuses a .bat or .cmd program. os/exec resolves an
// absolute path's PATHEXT extension privately and leaves cmd.Path as given
// (go.dev/issue/62596), so the path is resolved here with exec.LookPath,
// the same search Start makes, and C:\dir\shim with only shim.cmd beside
// it is refused too (0021-MADR D8, Amendment A2). Every caller passes an
// absolute path.
func checkProgram(cmd *exec.Cmd) error {
	path := cmd.Path
	if filepath.IsAbs(path) {
		if lp, err := exec.LookPath(path); err == nil {
			path = lp
		}
	}
	switch strings.ToLower(filepath.Ext(path)) {
	case ".bat", ".cmd":
		return fmt.Errorf("%w: %s", ErrBatchFile, filepath.Base(path))
	}
	return nil
}

// attachedAttr gives a child CREATE_NO_WINDOW when this process has no
// console. Without it, the child would get a new console with a visible
// window; with a console, the child inherits it, as it always has.
// CREATE_NO_WINDOW is ignored with DETACHED_PROCESS, so the two are never
// combined (process-creation flags).
func attachedAttr() *syscall.SysProcAttr {
	if hasConsole() {
		return nil
	}
	return &syscall.SysProcAttr{CreationFlags: windows.CREATE_NO_WINDOW}
}

// detachedAttr starts the handoff's run with a console of its own and no
// window, in place of DETACHED_PROCESS, under which every console child the
// run started would open a visible window of its own.
func detachedAttr(breakaway bool) *syscall.SysProcAttr {
	flags := uint32(windows.CREATE_NO_WINDOW | windows.CREATE_NEW_PROCESS_GROUP)
	if breakaway {
		flags |= windows.CREATE_BREAKAWAY_FROM_JOB
	}
	return &syscall.SysProcAttr{CreationFlags: flags}
}
