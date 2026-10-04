package scm

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/maccavelli/go-selfupdate-lib/selfupdate/service"
)

// fakeEnv makes the test binary the detached run of TestDetach: it writes
// "<ppid> <hop marker seen> <FAKE_SECRET>" to FAKE_OUT.
const fakeEnv = "SELFUPDATE_SCM_FAKE"

func fakeRun() int {
	out := os.Getenv("FAKE_OUT")
	_, marked := os.LookupEnv(service.EnvHandOffHop)
	body := strconv.Itoa(os.Getppid()) + " " + strconv.FormatBool(marked) + " " + os.Getenv("FAKE_SECRET")
	if err := os.WriteFile(out+".tmp", []byte(body), 0o600); err != nil {
		return 3
	}
	if err := os.Rename(out+".tmp", out); err != nil {
		return 4
	}
	return 0
}

// TestDetach: Detach starts a hop, and the hop the real run, which is not
// this process's child, does not see the marker, and has the handoff's
// environment (0011-MADR amendment A4).
func TestDetach(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(t.TempDir(), "run")
	s, _ := testService(t, newFake(), Options{})
	det, err := s.Detach(context.Background(), service.HandOff{
		ID: "abc", Executable: exe, ResultPath: filepath.Join(t.TempDir(), "result"),
		Env: []string{fakeEnv + "=1", "FAKE_OUT=" + out, "FAKE_SECRET=s3cret"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(det.Where, "hop") || det.ID != "abc" {
		t.Fatalf("detached %+v", det)
	}
	var body []byte
	for deadline := time.Now().Add(30 * time.Second); ; {
		if body, err = os.ReadFile(out); err == nil { //nolint:gosec // the test's own file, written by rename
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the real run never reported")
		}
		time.Sleep(20 * time.Millisecond)
	}
	f := strings.Fields(string(body))
	if len(f) != 3 || f[0] == strconv.Itoa(os.Getpid()) || f[1] != "false" || f[2] != "s3cret" {
		t.Fatalf("the real run reported %q", body)
	}
}
