package selfupdate

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"testing"
	"time"

	"github.com/maccavelli/go-selfupdate-lib/internal/testproc"
)

func TestMain(m *testing.M) {
	// The probe tests run this binary as the "new release": it prints a
	// version, optionally after a delay or with a failing exit code
	// (docs/decisions/0004-PLAN-v1-1-0-core-api.md Step 8).
	if v, ok := os.LookupEnv("SELFUPDATE_TEST_PRINT_VERSION"); ok {
		if d, err := time.ParseDuration(os.Getenv("SELFUPDATE_TEST_SLEEP")); err == nil {
			time.Sleep(d)
		}
		fmt.Println(v)
		code, _ := strconv.Atoi(os.Getenv("SELFUPDATE_TEST_EXIT"))
		os.Exit(code)
	}
	if os.Getenv("SELFUPDATE_NATIVE_HELPER") == "1" {
		nativeHelper()
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func nativeHelper() {
	ready := os.Getenv("SELFUPDATE_READY")
	done := os.Getenv("SELFUPDATE_DONE")
	if ready != "" {
		_ = os.WriteFile(ready, []byte("ready"), 0o600)
	}
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		if done != "" {
			if _, err := os.Stat(done); err == nil {
				return
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestNativeReplaceRunningCopy(t *testing.T) {
	t.Parallel()
	home, _ := tempTarget(t)
	src, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	helperName := "helper"
	if runtime.GOOS == goosWindows {
		helperName = "helper.exe"
	}
	exe := filepath.Join(home, helperName)
	in, err := os.ReadFile(src)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(exe, in, 0o755); err != nil {
		t.Fatal(err)
	}
	ready := filepath.Join(home, "ready")
	done := filepath.Join(home, "done")
	cmd := exec.Command(exe)
	cmd.Env = append(os.Environ(),
		"SELFUPDATE_NATIVE_HELPER=1",
		"SELFUPDATE_READY="+ready,
		"SELFUPDATE_DONE="+done,
	)
	proc := testproc.Start(t, cmd, ready)
	defer func() {
		_ = os.WriteFile(done, []byte("x"), 0o600)
		_ = proc.Wait()
	}()

	inst, err := NewStandaloneInstaller(InstallOptions{TargetPolicy: targetPolicy(exe)})
	if err != nil {
		t.Fatal(err)
	}
	target, err := inst.ResolveTarget(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	sess, err := inst.Begin(context.Background(), target)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = sess.Close() }()
	f, path, err := sess.CreateStaging(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	replacement := append([]byte{}, in...)
	replacement = append(replacement, []byte("REPLACED")...)
	if _, err := f.Write(replacement); err != nil {
		t.Fatal(err)
	}
	if err := f.Sync(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	res, installErr := sess.Install(context.Background(), InstallRequest{
		Product:  "helper",
		Artifact: StagedArtifact{Path: path, Size: int64(len(replacement))},
	})
	if installErr != nil && res.PendingBackup == "" {
		t.Fatal(installErr)
	}
	got, err := os.ReadFile(exe)
	if err != nil {
		t.Fatal(err)
	}
	// A clean Install must leave exactly the new bytes. The old bytes are
	// acceptable only on the pending-backup path (0003-PLAN Phase 0b).
	if installErr == nil && res.PendingBackup == "" && string(got) != string(replacement) {
		t.Fatalf("Install succeeded but the target does not hold the new bytes (%d bytes)", len(got))
	}
	if string(got) != string(replacement) && string(got) != string(in) {
		t.Fatalf("target is neither old nor new (%d bytes)", len(got))
	}
	_ = os.WriteFile(done, []byte("x"), 0o600)
	waitErr := proc.Wait()
	if waitErr != nil {
		t.Logf("helper exit: %v (allowed while replacing a running image)", waitErr)
	}
	final, err := os.ReadFile(exe)
	if err != nil {
		t.Fatal(err)
	}
	if string(final) != string(replacement) && string(final) != string(in) {
		t.Fatal("lost both old and new executable bytes")
	}
}
