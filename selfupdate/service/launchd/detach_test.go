package launchd

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/maccavelli/go-selfupdate-lib/selfupdate/service"
)

// Tests for docs/decisions/0011-PLAN-reference-service-lifecycles.md V3
// step 7 and MADR amendment A3: the one-shot job and its cleanup.

func handOffSpec() service.HandOff {
	return service.HandOff{
		ID: "abc123", Executable: "/opt/demo/demo", Args: []string{"update", "-y", "<&>"},
		Env: []string{service.EnvHandOff + "=abc123", service.EnvHandOffResult + "=/opt/demo/.demo.selfupdate.handoff",
			"SECRET_TOKEN=s3cret"},
		ResultPath: "/opt/demo/.demo.selfupdate.handoff",
	}
}

func TestDetach(t *testing.T) {
	f := newFake()
	j := testJob(t, f, Options{})
	det, err := j.Detach(context.Background(), handOffSpec())
	if err != nil {
		t.Fatal(err)
	}
	label := "com.example.demo.selfupdate.abc123"
	plist := filepath.Join(j.o.JobDir, label+".plist")
	envFile := filepath.Join(j.o.JobDir, "handoff-abc123.env")
	body := readString(t, plist)
	for _, want := range []string{
		"<key>Label</key>\n\t<string>" + label + "</string>",
		"<string>/opt/demo/demo</string>", "<string>&lt;&amp;&gt;</string>",
		"<key>RunAtLoad</key>\n\t<true/>", "<key>AbandonProcessGroup</key>\n\t<true/>",
		"<key>" + service.EnvHandOffEnv + "</key>\n\t\t<string>" + envFile + "</string>",
		"<key>" + service.EnvHandOff + "</key>\n\t\t<string>abc123</string>",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("plist lacks %q:\n%s", want, body)
		}
	}
	if strings.Contains(body, "s3cret") || strings.Contains(body, "KeepAlive") {
		t.Fatalf("the plist holds a secret, or KeepAlive:\n%s", body)
	}
	if !strings.Contains(readString(t, envFile), `"SECRET_TOKEN=s3cret"`) {
		t.Fatal("the environment file lacks the caller's environment")
	}
	if runtime.GOOS != "windows" {
		for path, mode := range map[string]os.FileMode{plist: 0o600, envFile: 0o600, j.o.JobDir: 0o700} {
			if info, err := os.Stat(path); err != nil || info.Mode().Perm() != mode {
				t.Fatalf("%s mode %v, %v; want %v", path, info.Mode(), err, mode)
			}
		}
	}
	if runtime.GOOS == "darwin" {
		if out, err := exec.Command("/usr/bin/plutil", "-lint", plist).CombinedOutput(); err != nil {
			t.Fatalf("plutil -lint: %v: %s", err, out)
		}
	}
	last := f.calls[len(f.calls)-1]
	if !slices.Equal(last[1:], []string{"bootstrap", "gui/503", plist}) {
		t.Fatalf("bootstrap call %q", last)
	}
	if det.Where != "launchd job "+label {
		t.Fatalf("detached %+v", det)
	}
}

func TestDetachFailureRemovesFiles(t *testing.T) {
	f := newFake()
	f.bootstrapCodes = []int{exitIO}
	j := testJob(t, f, Options{})
	if _, err := j.Detach(context.Background(), handOffSpec()); err == nil {
		t.Fatal("a refused bootstrap succeeded")
	}
	if entries, _ := os.ReadDir(j.o.JobDir); len(entries) != 0 {
		t.Fatalf("a failed handoff left %v", entries)
	}
}

func TestCleanupHandOffs(t *testing.T) {
	f := newFake()
	j := testJob(t, f, Options{})
	if err := os.MkdirAll(j.o.JobDir, 0o700); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"running", "spawning", "finished", "unloaded"} {
		for _, name := range []string{"com.example.demo.selfupdate." + id + ".plist", "handoff-" + id + ".env"} {
			if err := os.WriteFile(filepath.Join(j.o.JobDir, name), nil, 0o600); err != nil {
				t.Fatal(err)
			}
		}
	}
	f.handOffs = map[string]service.Output{
		"gui/503/com.example.demo.selfupdate.running": {Stdout: []byte(printOutput(321, "running"))},
		// launchd is still spawning it (testdata/print-xpcproxy.txt).
		"gui/503/com.example.demo.selfupdate.spawning": {Stdout: []byte(printOutput(322, "xpcproxy"))},
		"gui/503/com.example.demo.selfupdate.finished": {Stdout: []byte(printOutput(0, "not running"))},
		"gui/503/com.example.demo.selfupdate.unloaded": {ExitCode: exitNotFound},
	}
	if err := j.CleanupHandOffs(context.Background()); err != nil {
		t.Fatal(err)
	}
	var left []string
	entries, _ := os.ReadDir(j.o.JobDir)
	for _, e := range entries {
		left = append(left, e.Name())
	}
	if !slices.Equal(left, []string{
		"com.example.demo.selfupdate.running.plist", "com.example.demo.selfupdate.spawning.plist",
		"handoff-running.env", "handoff-spawning.env",
	}) {
		t.Fatalf("left %q; want only the running and spawning jobs' files", left)
	}
	bootouts := 0
	for _, c := range f.calls {
		if len(c) > 2 && c[1] == "bootout" {
			bootouts++
			if c[2] != "gui/503/com.example.demo.selfupdate.finished" {
				t.Fatalf("booted out %s", c[2])
			}
		}
	}
	if bootouts != 1 {
		t.Fatalf("%d bootouts, want 1", bootouts)
	}
}
