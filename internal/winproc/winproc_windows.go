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
// magic-cli-remote's
// docs/decisions/0159-MADR-the-windows-service-path-is-half-wired.md D17).
// The answer is read once.
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
//
// The extension is read from the name Windows will open, not the name as
// written: run.bat. and run.cmd. put an argument under cmd.exe's parser,
// and run.bat with a trailing space runs the script (measured, 0022-PLAN
// P1). runName normalizes those spellings first.
func checkProgram(cmd *exec.Cmd) error {
	path := cmd.Path
	if filepath.IsAbs(path) {
		if lp, err := exec.LookPath(path); err == nil {
			path = lp
		}
	}
	name := runName(path)
	switch strings.ToLower(filepath.Ext(name)) {
	case ".bat", ".cmd":
		return fmt.Errorf("%w: %s", ErrBatchFile, name)
	}
	return nil
}

// runName is the final component of path as the Win32 layer reads it:
// GetFullPathName drops its trailing dots and spaces, and an alternate data
// stream suffix (run.bat::$DATA) is cut at the first colon (0022-MADR H2).
// When GetFullPathName fails, the path is read as written.
func runName(path string) string {
	if full, err := fullPathName(path); err == nil {
		path = full
	}
	name := filepath.Base(path)
	if i := strings.IndexByte(name, ':'); i >= 0 {
		name = name[:i]
	}
	return name
}

// fullPathName is windows.GetFullPathName, with a buffer as long as the
// call asks for.
func fullPathName(path string) (string, error) {
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return "", err
	}
	buf := make([]uint16, windows.MAX_PATH)
	for {
		n, err := windows.GetFullPathName(p, uint32(len(buf)), &buf[0], nil) //nolint:gosec // len(buf) is at most the length the call asked for
		if err != nil {
			return "", err
		}
		if int(n) < len(buf) {
			return windows.UTF16ToString(buf[:n]), nil
		}
		buf = make([]uint16, n)
	}
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
