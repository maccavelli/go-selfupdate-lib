//go:build windows

package service

import "golang.org/x/sys/windows"

// protectDir gives dir a protected access list: full control for the
// current user, SYSTEM and Administrators, inherited by what it holds, and
// none inherited from its parent. Windows does not enforce WriteHandOffEnv's
// mode 0700, and a directory under %LOCALAPPDATA% can inherit entries for
// others
// (docs/decisions/0032-MADR-task-scheduler-registration-and-one-shot-handoff.md
// Amendment A1, P1-8; a medium-integrity process sets this list, P1-9).
func protectDir(dir string) error {
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return err
	}
	sd, err := windows.SecurityDescriptorFromString(
		"D:P(A;OICI;FA;;;" + user.User.Sid.String() + ")(A;OICI;FA;;;SY)(A;OICI;FA;;;BA)")
	if err != nil {
		return err
	}
	dacl, _, err := sd.DACL()
	if err != nil {
		return err
	}
	return windows.SetNamedSecurityInfo(dir, windows.SE_FILE_OBJECT,
		windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION, nil, nil, dacl, nil)
}
