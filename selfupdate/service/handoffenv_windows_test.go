//go:build windows

package service

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/sys/windows"
)

// sddl reads path's DACL as SDDL.
func sddl(t *testing.T, path string) string {
	t.Helper()
	sd, err := windows.GetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		t.Fatal(err)
	}
	return sd.String()
}

// TestWriteHandOffEnvProtectsDir: WriteHandOffEnv gives the directory a
// protected DACL of the user, SYSTEM and Administrators, though its
// parent grants Everyone access, and the file inherits it
// (docs/decisions/0032-MADR-task-scheduler-registration-and-one-shot-handoff.md
// Amendment A1, P1-8 and P1-9).
func TestWriteHandOffEnvProtectsDir(t *testing.T) {
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		t.Fatal(err)
	}
	sid := user.User.Sid.String()
	parent := t.TempDir()
	open, err := windows.SecurityDescriptorFromString("D:(A;OICI;FA;;;" + sid + ")(A;OICI;FA;;;SY)(A;OICI;FA;;;BA)(A;OICI;GR;;;WD)")
	if err != nil {
		t.Fatal(err)
	}
	dacl, _, err := open.DACL()
	if err != nil {
		t.Fatal(err)
	}
	if err := windows.SetNamedSecurityInfo(parent, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION, nil, nil, dacl, nil); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(sddl(t, parent), ";;;WD)") {
		t.Fatalf("the parent does not grant Everyone access: %s", sddl(t, parent))
	}
	dir := filepath.Join(parent, "handoff")
	path, err := WriteHandOffEnv(dir, "acl", []string{"A=1"})
	if err != nil {
		t.Fatal(err)
	}
	d := sddl(t, dir)
	if !strings.HasPrefix(d, "D:P") || strings.Count(d, "(A;") != 3 || strings.Contains(d, ";;;WD)") ||
		!strings.Contains(d, ";;;"+sid+")") || !strings.Contains(d, ";;;SY)") || !strings.Contains(d, ";;;BA)") {
		t.Fatalf("the directory's DACL: %s; want protected, the user, SYSTEM and Administrators only", d)
	}
	f := sddl(t, path)
	if strings.Contains(f, ";;;WD)") || !strings.Contains(f, ";;;"+sid+")") {
		t.Fatalf("the file's DACL: %s; want the directory's", f)
	}
	if _, err := os.ReadFile(path); err != nil { //nolint:gosec // the test's own file
		t.Fatalf("the user cannot read the file: %v", err)
	}
}
