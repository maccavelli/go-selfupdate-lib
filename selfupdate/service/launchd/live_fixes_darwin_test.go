//go:build darwin

package launchd

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Live tests for docs/decisions/0015-MADR-remediate-third-debugging-pass-findings.md
// D3 and D7, against this Mac's launchd, as the other live tests run.

// oneShotLive loads a one-shot job under liveLabel whose program is a
// script that appends word to marker, with runAtLoad as the plist's
// RunAtLoad value. It returns the plist, the marker, and the directory
// holding a second script that appends "new".
func oneShotLive(t *testing.T, runAtLoad string) (plist, marker, dir string) {
	t.Helper()
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	marker = filepath.Join(dir, "marker")
	for _, word := range []string{"old", "new"} {
		script := "#!/bin/sh\necho " + word + " >> \"$FAKE_MARKER\"\n"
		if err := os.WriteFile(filepath.Join(dir, word+".sh"), []byte(script), 0o755); err != nil { //nolint:gosec // an executable fixture
			t.Fatal(err)
		}
	}
	body := string(oneShotPlist(liveLabel, []string{filepath.Join(dir, "old.sh")}, map[string]string{"FAKE_MARKER": marker}))
	body = strings.Replace(body, "<key>RunAtLoad</key>\n\t<true/>", "<key>RunAtLoad</key>\n\t"+runAtLoad, 1)
	plist = filepath.Join(dir, liveLabel+".plist")
	if err := os.WriteFile(plist, []byte(body), 0o644); err != nil { //nolint:gosec // a LaunchAgent plist
		t.Fatal(err)
	}
	target := liveDomain() + "/" + liveLabel
	_ = exec.Command("/bin/launchctl", "bootout", target).Run()
	if out, err := exec.Command("/bin/launchctl", "enable", target).CombinedOutput(); err != nil {
		t.Fatalf("enable: %v: %s", err, out)
	}
	t.Cleanup(func() { _ = exec.Command("/bin/launchctl", "bootout", target).Run() })
	if out, err := exec.Command("/bin/launchctl", "bootstrap", liveDomain(), plist).CombinedOutput(); err != nil {
		t.Fatalf("bootstrap: %v: %s", err, out)
	}
	return plist, marker, dir
}

// markerLines reads the marker's lines; a missing marker has none.
func markerLines(t *testing.T, marker string) []string {
	t.Helper()
	body, err := os.ReadFile(marker) //nolint:gosec // the live test's own file
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		t.Fatal(err)
	}
	return strings.Fields(string(body))
}

// waitMarker waits until the marker has more than n lines, the last of
// them want, and returns the lines.
func waitMarker(t *testing.T, marker string, n int, want, what string) []string {
	t.Helper()
	for deadline := time.Now().Add(20 * time.Second); ; time.Sleep(100 * time.Millisecond) {
		lines := markerLines(t, marker)
		if len(lines) > n && lines[len(lines)-1] == want {
			return lines
		}
		if time.Now().After(deadline) {
			t.Fatalf("%s: the marker reads %q; want a run of the %s script after line %d", what, lines, want, n)
		}
	}
}

// TestLiveRewriteLoadedIdleJob: a one-shot job that ran and exited stays
// loaded, and launchd keeps the definition it loaded. A rewrite reloads
// it, so the job runs the new program from then on, Start's kickstart
// included (D3).
func TestLiveRewriteLoadedIdleJob(t *testing.T) {
	requireLive(t)
	plist, marker, dir := oneShotLive(t, "<true/>")
	waitMarker(t, marker, 0, "old", "the load")
	j, err := New(Options{Label: liveLabel, Domain: GUI(os.Getuid()), Plist: plist, RewritePath: true})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if s, err := j.probe(ctx); err != nil || !s.loaded || s.running {
		t.Fatalf("probe = %+v, %v; want the job loaded and idle", s, err)
	}
	res, err := j.Reconcile(ctx, "demo", filepath.Join(dir, "new.sh"))
	if err != nil || !res.Changed {
		t.Fatalf("Reconcile = %+v, %v", res, err)
	}
	lines := waitMarker(t, marker, 1, "new", "the reload")
	if err := j.Start(ctx, "demo"); err != nil {
		t.Fatal(err)
	}
	waitMarker(t, marker, len(lines), "new", "Start")
	out, err := exec.Command("/bin/launchctl", "print", liveDomain()+"/"+liveLabel).Output()
	if want := "program = " + filepath.Join(dir, "new.sh") + "\n"; err != nil || !strings.Contains(string(out), want) {
		t.Fatalf("launchctl print lacks %q: %v\n%s", want, err, out)
	}
}

// TestLiveRunAtLoadInteger: launchd runs a job at load only for
// RunAtLoad <true/>; an integer 1 does not, and Enabled says so (D7).
func TestLiveRunAtLoadInteger(t *testing.T) {
	requireLive(t)
	plist, marker, _ := oneShotLive(t, "<integer>1</integer>")
	j, err := New(Options{Label: liveLabel, Domain: GUI(os.Getuid()), Plist: plist})
	if err != nil {
		t.Fatal(err)
	}
	if enabled, err := j.Enabled(context.Background(), "demo"); err != nil || enabled {
		t.Fatalf("Enabled = %t, %v; RunAtLoad 1 does not run the job at load", enabled, err)
	}
	time.Sleep(2 * time.Second)
	if lines := markerLines(t, marker); len(lines) != 0 {
		t.Fatalf("launchd ran the job at load: %q", lines)
	}
}
