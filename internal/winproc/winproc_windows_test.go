//go:build windows

package winproc

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"unsafe"

	"golang.org/x/sys/windows"
)

// helperArg makes the test binary act as one generation of the three-deep
// test instead of running tests.
const helperArg = "winproc-helper"

var (
	kernel32         = windows.NewLazySystemDLL("kernel32.dll")
	procFreeConsole  = kernel32.NewProc("FreeConsole")
	procConsoleWin   = kernel32.NewProc("GetConsoleWindow")
	procIsWinVisible = windows.NewLazySystemDLL("user32.dll").NewProc("IsWindowVisible")
)

func TestMain(m *testing.M) {
	if len(os.Args) > 3 && os.Args[1] == helperArg {
		os.Exit(helper(os.Args[2], os.Args[3]))
	}
	os.Exit(m.Run())
}

// helper runs one generation:
//   - parent-command and parent-detached free this process's console, then
//     start a child through Command or Detached, and wait for it;
//   - child reports its console, then starts a grandchild with no flags at
//     all, as a program's own children are started, and waits for it;
//   - grandchild reports its console.
func helper(mode, dir string) int {
	self, err := os.Executable()
	if err != nil {
		return 3
	}
	child := []string{helperArg, "child", dir}
	switch mode {
	case "parent-command", "parent-detached":
		if r, _, err := procFreeConsole.Call(); r == 0 {
			fmt.Fprintln(os.Stderr, "FreeConsole:", err)
			return 4
		}
		var cmd *exec.Cmd
		if mode == "parent-command" {
			cmd, err = Command(context.Background(), self, child...)
		} else {
			cmd, err = Detached(self, child, false)
		}
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 5
		}
		if err := cmd.Run(); err != nil {
			fmt.Fprintln(os.Stderr, "child:", err)
			return 6
		}
	case "child":
		if err := report(dir, "child"); err != nil {
			return 7
		}
		if err := exec.Command(self, helperArg, "grandchild", dir).Run(); err != nil {
			return 8
		}
	case "grandchild":
		if err := report(dir, "grandchild"); err != nil {
			return 9
		}
	default:
		return 2
	}
	return 0
}

// report writes how many processes share this process's console, its
// console window, and whether that window is visible.
func report(dir, tag string) error {
	var pids [8]uint32
	n, _, _ := getConsoleProcessList.Call(uintptr(unsafe.Pointer(&pids[0])), uintptr(len(pids)))
	hwnd, _, _ := procConsoleWin.Call()
	visible := uintptr(0)
	if hwnd != 0 {
		visible, _, _ = procIsWinVisible.Call(hwnd)
	}
	line := fmt.Sprintf("procs=%d hwnd=%#x visible=%t", n, hwnd, visible != 0)
	return os.WriteFile(filepath.Join(dir, tag), []byte(line), 0o600)
}

// TestNoConsoleWindowThreeDeep: a console-less parent starts a child
// through Command, or through Detached as the handoff does; the child
// starts a grandchild with no flags. Neither gets a console window, and
// both have a console, so a console program among them still runs
// (0021-MADR D7; magic-cli-remote 0159 probe 8).
func TestNoConsoleWindowThreeDeep(t *testing.T) {
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"parent-command", "parent-detached"} {
		t.Run(mode, func(t *testing.T) {
			dir := t.TempDir()
			if out, err := exec.Command(self, helperArg, mode, dir).CombinedOutput(); err != nil {
				t.Fatalf("%s: %v\n%s", mode, err, out)
			}
			for _, gen := range []string{"child", "grandchild"} {
				b, err := os.ReadFile(filepath.Join(dir, gen))
				if err != nil {
					t.Fatalf("%s: %v", gen, err)
				}
				got := string(b)
				if strings.HasPrefix(got, "procs=0 ") {
					t.Errorf("%s has no console: %s", gen, got)
				}
				if !strings.Contains(got, " hwnd=0x0 ") {
					t.Errorf("%s has a console window: %s", gen, got)
				}
			}
		})
	}
}

// TestBatchFileRefused: Command and Detached refuse a .bat or .cmd
// program, whatever its case, and an extensionless path that os/exec
// resolves to one through PATHEXT (0021-MADR D8). They refuse it too in
// each spelling the Win32 layer reads as one: a trailing dot or space,
// which 0022-PLAN P1 measured reaching cmd.exe, and an alternate data
// stream (0022-MADR H2).
func TestBatchFileRefused(t *testing.T) {
	dir := t.TempDir()
	for _, f := range []string{"shim.cmd", "run.bat", "run.cmd"} {
		if err := os.WriteFile(filepath.Join(dir, f), []byte("@echo off\r\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	for _, c := range []struct {
		name    string
		refused bool
	}{
		{"x.CMD", true},
		{"x.bat", true},
		{"x.Bat", true},
		{"x.exe", false},
		{"shim", true},
		{"run.bat.", true},
		{"run.bat..", true},
		{"run.bat ", true},
		{"run.bat. .", true},
		{"run.cmd.", true},
		{"run.cmd ", true},
		{"RUN.BAT.", true},
		{"run.bat::$DATA", true},
		{"run.bat:s", true},
		{"x.exe.", false},
		{"x.exe ", false},
		{"x.bat.exe", false},
		{"x.batch", false},
	} {
		// Joined by hand: the spelling must reach the guard as written.
		path := dir + `\` + c.name
		_, err := Command(context.Background(), path)
		if got := errors.Is(err, ErrBatchFile); got != c.refused {
			t.Errorf("Command(%s): refused %t (%v), want %t", c.name, got, err, c.refused)
		}
		_, err = Detached(path, nil, false)
		if got := errors.Is(err, ErrBatchFile); got != c.refused {
			t.Errorf("Detached(%s): refused %t (%v), want %t", c.name, got, err, c.refused)
		}
	}
}
