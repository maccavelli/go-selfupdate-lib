//go:build darwin

package launchd

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/maccavelli/go-selfupdate-lib/selfupdate/service"
)

// TestEnabledRealPlutil runs Enabled's plist reads through the real
// /usr/bin/plutil, launchctl still faked: the fake plutil cannot show that a
// format refuses a value (0011-PLAN V3 step 2).
func TestEnabledRealPlutil(t *testing.T) {
	const head = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>Label</key>
	<string>com.example.demo</string>
`
	for _, c := range []struct {
		name, keys string
		want       bool
	}{
		{"RunAtLoad", "<key>RunAtLoad</key><true/>", true},
		{"KeepAlive", "<key>KeepAlive</key><true/>", true},
		{"KeepAlive dict", "<key>KeepAlive</key><dict><key>SuccessfulExit</key><false/></dict>", true},
		{"KeepAlive empty dict", "<key>KeepAlive</key><dict/>", true},
		{"both false", "<key>RunAtLoad</key><false/><key>KeepAlive</key><false/>", false},
		{"neither", "", false},
		// launchd runs a job at load for <true/> only (0015-MADR D7).
		{"RunAtLoad integer 1", "<key>RunAtLoad</key><integer>1</integer>", false},
		{"RunAtLoad integer 0", "<key>RunAtLoad</key><integer>0</integer>", false},
	} {
		plist := filepath.Join(t.TempDir(), "com.example.demo.plist")
		if err := os.WriteFile(plist, []byte(head+c.keys+"\n</dict>\n</plist>\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		f := newFake()
		j := testJob(t, f, Options{Plist: plist})
		j.o.Runner = service.RunnerFunc(func(ctx context.Context, cmd service.Command) (service.Output, error) {
			if strings.HasSuffix(cmd.Path, "/plutil") {
				return service.ExecRunner().Run(ctx, cmd)
			}
			return f.Run(ctx, cmd)
		})
		if got, err := j.Enabled(context.Background(), "demo"); err != nil || got != c.want {
			t.Errorf("%s: enabled %t, %v; want %t", c.name, got, err, c.want)
		}
	}
}

// TestPlistValueRealPlutil runs plistValue through the real
// /usr/bin/plutil: an integer, a missing key, a file that is no dictionary
// (a bare word lints as a valid old-style property list), and an unreadable
// file (0015-MADR B3, and amendment A1's D7 probe evidence).
func TestPlistValueRealPlutil(t *testing.T) {
	const head = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>ExitTimeOut</key>
	<integer>90</integer>
</dict>
</plist>
`
	job := func(t *testing.T, body string, mode os.FileMode) *Job {
		t.Helper()
		plist := filepath.Join(t.TempDir(), "com.example.demo.plist")
		if err := os.WriteFile(plist, []byte(body), mode); err != nil {
			t.Fatal(err)
		}
		f := newFake()
		j := testJob(t, f, Options{Plist: plist})
		j.o.Runner = service.RunnerFunc(func(ctx context.Context, cmd service.Command) (service.Output, error) {
			if strings.HasSuffix(cmd.Path, "/plutil") {
				return service.ExecRunner().Run(ctx, cmd)
			}
			return f.Run(ctx, cmd)
		})
		return j
	}
	ctx := context.Background()
	j := job(t, head, 0o600)
	if typ, raw, present, err := j.plistValue(ctx, "ExitTimeOut"); err != nil || typ != "integer" || raw != "90" || !present {
		t.Fatalf("ExitTimeOut: %q %q %t %v", typ, raw, present, err)
	}
	if typ, _, present, err := j.plistValue(ctx, "Missing"); err != nil || present {
		t.Fatalf("Missing: %q %t %v", typ, present, err)
	}
	if _, _, _, err := job(t, "garbage\n", 0o600).plistValue(ctx, "ExitTimeOut"); err == nil {
		t.Fatal("a plist holding a bare word read as a dictionary")
	}
	if os.Geteuid() != 0 {
		if _, _, _, err := job(t, head, 0).plistValue(ctx, "ExitTimeOut"); err == nil {
			t.Fatal("an unreadable plist read without an error")
		}
	}
}

// TestEnabledRealPlutilUnreadable: through the real /usr/bin/plutil, a plist
// holding a bare word (which plutil -lint passes) and one that cannot be
// read are errors, not "not enabled" (0015-MADR D7).
func TestEnabledRealPlutilUnreadable(t *testing.T) {
	for _, c := range []struct {
		name string
		body string
		mode os.FileMode
	}{
		{"a bare word", "garbage\n", 0o600},
		{"mode 0000", "<plist version=\"1.0\"><dict/></plist>\n", 0},
	} {
		if c.mode == 0 && os.Geteuid() == 0 {
			continue // root reads any file
		}
		plist := filepath.Join(t.TempDir(), "com.example.demo.plist")
		if err := os.WriteFile(plist, []byte(c.body), c.mode); err != nil {
			t.Fatal(err)
		}
		f := newFake()
		j := testJob(t, f, Options{Plist: plist})
		j.o.Runner = service.RunnerFunc(func(ctx context.Context, cmd service.Command) (service.Output, error) {
			if strings.HasSuffix(cmd.Path, "/plutil") {
				return service.ExecRunner().Run(ctx, cmd)
			}
			return f.Run(ctx, cmd)
		})
		if got, err := j.Enabled(context.Background(), "demo"); err == nil {
			t.Errorf("%s: Enabled = %t, <nil>; want an error", c.name, got)
		}
	}
}
