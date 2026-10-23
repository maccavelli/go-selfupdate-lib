//go:build windows

package taskscheduler

import (
	"testing"

	"golang.org/x/sys/windows"
)

// TestSameAccountResolvesSID: the SID Task Scheduler stores equals the
// account a definition names, as DOMAIN\name or name (0032-MADR D3-A).
func TestSameAccountResolvesSID(t *testing.T) {
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		t.Fatal(err)
	}
	sid := user.User.Sid.String()
	name, domain, _, err := user.User.Sid.LookupAccount("")
	if err != nil {
		t.Fatal(err)
	}
	for _, account := range []string{name, domain + `\` + name} {
		if !sameAccount(sid, account) || !sameAccount(account, sid) {
			t.Errorf("the SID and %q differ", account)
		}
	}
	if sameAccount(sid, "nobody-"+name) || sameAccount("S-1-5-18", name) {
		t.Error("a SID matched another account")
	}
	stored := `<Task><Principals><Principal><UserId>` + sid + `</UserId></Principal></Principals><Actions><Exec><Command>x</Command></Exec></Actions></Task>`
	wanted := `<Task><Principals><Principal><UserId>` + domain + `\` + name + `</UserId></Principal></Principals><Actions><Exec><Command>x</Command></Exec></Actions></Task>`
	if d, err := DiffDefinitions(stored, wanted); err != nil || len(d) != 0 {
		t.Fatalf("a stored SID against its account: %+v, %v", d, err)
	}
}
