//go:build windows

package service

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"unsafe"

	"golang.org/x/sys/windows"
)

// fileAllAccess is FILE_ALL_ACCESS, STANDARD_RIGHTS_REQUIRED | SYNCHRONIZE
// | 0x1FF, the right SDDL's FA grants; x/sys/windows does not define it.
const fileAllAccess = windows.STANDARD_RIGHTS_REQUIRED | windows.SYNCHRONIZE | 0x1FF

// aceEntry is one entry of a DACL.
type aceEntry struct {
	typ   byte
	flags byte
	mask  windows.ACCESS_MASK
	sid   *windows.SID
}

// dacl reads path's DACL: whether it is protected, and its entries. The
// SIDs are compared as SIDs: SDDL text writes some, such as the built-in
// Administrator account's, as an alias.
func dacl(t *testing.T, path string) (bool, []aceEntry) {
	t.Helper()
	sd, err := windows.GetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		t.Fatal(err)
	}
	control, _, err := sd.Control()
	if err != nil {
		t.Fatal(err)
	}
	acl, _, err := sd.DACL()
	if err != nil {
		t.Fatal(err)
	}
	var out []aceEntry
	for i := range uint32(acl.AceCount) {
		var ace *windows.ACCESS_ALLOWED_ACE
		if err := windows.GetAce(acl, i, &ace); err != nil {
			t.Fatal(err)
		}
		sid := (*windows.SID)(unsafe.Pointer(&ace.SidStart)) //nolint:gosec // the SID starts at SidStart
		out = append(out, aceEntry{typ: ace.Header.AceType, flags: ace.Header.AceFlags, mask: ace.Mask, sid: sid})
	}
	return control&windows.SE_DACL_PROTECTED != 0, out
}

// TestWriteHandOffEnvProtectsDir: WriteHandOffEnv gives the directory a
// protected DACL of the user, SYSTEM and Administrators, with full control
// each, though its parent grants Everyone access, and the file inherits
// it
// (docs/decisions/0032-MADR-task-scheduler-registration-and-one-shot-handoff.md
// Amendment A1, P1-8 and P1-9; 0032-PLAN Deviation D7).
func TestWriteHandOffEnvProtectsDir(t *testing.T) {
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		t.Fatal(err)
	}
	system, err := windows.CreateWellKnownSid(windows.WinLocalSystemSid)
	if err != nil {
		t.Fatal(err)
	}
	admins, err := windows.CreateWellKnownSid(windows.WinBuiltinAdministratorsSid)
	if err != nil {
		t.Fatal(err)
	}
	want := []*windows.SID{user.User.Sid, system, admins}

	parent := t.TempDir()
	open, err := windows.SecurityDescriptorFromString("D:(A;OICI;FA;;;" + user.User.Sid.String() + ")(A;OICI;FA;;;SY)(A;OICI;FA;;;BA)(A;OICI;GR;;;WD)")
	if err != nil {
		t.Fatal(err)
	}
	openACL, _, err := open.DACL()
	if err != nil {
		t.Fatal(err)
	}
	if err := windows.SetNamedSecurityInfo(parent, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION, nil, nil, openACL, nil); err != nil {
		t.Fatal(err)
	}
	everyone, err := windows.CreateWellKnownSid(windows.WinWorldSid)
	if err != nil {
		t.Fatal(err)
	}
	_, inherit := dacl(t, parent)
	if !slices.ContainsFunc(inherit, func(e aceEntry) bool { return windows.EqualSid(e.sid, everyone) }) {
		t.Fatalf("the parent's %d entries do not grant Everyone access", len(inherit))
	}
	dir := filepath.Join(parent, "handoff")
	path, err := WriteHandOffEnv(dir, "acl", []string{"A=1"})
	if err != nil {
		t.Fatal(err)
	}
	check := func(what string, wantProtected, inherited bool) {
		t.Helper()
		protected, entries := dacl(t, map[bool]string{false: dir, true: path}[inherited])
		var got []string
		for _, e := range entries {
			got = append(got, e.sid.String())
		}
		if protected != wantProtected || len(entries) != len(want) {
			t.Fatalf("%s: protected %t, entries %s; want protected %t and %d entries", what, protected, strings.Join(got, " "), wantProtected, len(want))
		}
		for i, e := range entries {
			if e.typ != windows.ACCESS_ALLOWED_ACE_TYPE || e.mask != fileAllAccess || !windows.EqualSid(e.sid, want[i]) ||
				(e.flags&windows.INHERITED_ACE != 0) != inherited {
				t.Fatalf("%s: entry %d is type %d, mask %#x, flags %#x, SID %s; want full control for %s, inherited %t",
					what, i, e.typ, e.mask, e.flags, e.sid, want[i], inherited)
			}
		}
	}
	check("the directory", true, false)
	check("the file", false, true)
	if _, err := os.ReadFile(path); err != nil { //nolint:gosec // the test's own file
		t.Fatalf("the user cannot read the file: %v", err)
	}
}
