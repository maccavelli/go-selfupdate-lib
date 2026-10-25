package selfupdate

import (
	"path/filepath"
	"testing"
)

func TestCleanupReceiptName(t *testing.T) {
	got := cleanupReceiptName("demo")
	if got != ".demo.selfupdate.cleanup" {
		t.Fatalf("%q", got)
	}
	path := cleanupReceiptPath(Target{Dir: filepath.FromSlash("/tmp/x"), Base: "demo"})
	if filepath.Base(path) != got {
		t.Fatalf("%q", path)
	}
}

// TestValidateReceiptBackup: a receipt may name only a backup of its own
// target, as a bare basename (0003-MADR B6). Portable, so every OS runs it.
func TestValidateReceiptBackup(t *testing.T) {
	target := Target{Dir: filepath.FromSlash("/home/<user>/bin"), Base: "demo.exe", Path: filepath.FromSlash("/home/<user>/bin/demo.exe")}
	good := []string{".demo.exe.selfupdate-bak-123", ".demo.exe.selfupdate-bak-x"}
	bad := []string{
		"",
		"demo.exe",                     // the target itself
		".demo.exe.selfupdate.lock",    // the lock
		".demo.exe.selfupdate.cleanup", // the receipt
		".demo.exe.selfupdate-bak-",    // the bare prefix
		".other.exe.selfupdate-bak-1",  // another product's backup
		"victim",                       // any other file
		"/home/<user>/bin/.demo.exe.selfupdate-bak-1", // an absolute path
		`C:\bin\.demo.exe.selfupdate-bak-1`,
		"../.demo.exe.selfupdate-bak-1",
		"sub/.demo.exe.selfupdate-bak-1",
		`sub\.demo.exe.selfupdate-bak-1`,
		".",
		"..",
	}
	for _, name := range good {
		if err := validateReceiptBackup(target, name); err != nil {
			t.Errorf("rejected %q: %v", name, err)
		}
	}
	for _, name := range bad {
		if err := validateReceiptBackup(target, name); err == nil {
			t.Errorf("accepted %q", name)
		}
	}
}
