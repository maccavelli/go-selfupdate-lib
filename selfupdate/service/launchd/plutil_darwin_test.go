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
