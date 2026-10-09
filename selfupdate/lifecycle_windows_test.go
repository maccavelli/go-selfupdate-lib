//go:build windows

package selfupdate

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// plantPendingCleanup leaves a backup under a cleanup receipt, as a commit
// whose running image kept the backup open does, and returns the paths
// CleanupPending must remove.
func plantPendingCleanup(t *testing.T, target Target) []string {
	t.Helper()
	backup := filepath.Join(target.Dir, backupPrefix(target.Base)+"pending")
	if err := os.WriteFile(backup, []byte("old-bytes"), 0o600); err != nil {
		t.Fatal(err)
	}
	digest, err := fileSHA256(backup)
	if err != nil {
		t.Fatal(err)
	}
	if err := writeCleanupReceipt(target, applyResult{backup: backup, oldDigest: digest}); err != nil {
		t.Fatal(err)
	}
	return []string{backup, cleanupReceiptPath(target)}
}

// helperService is a "service" whose process is a running copy of the
// target: Stop ends it and waits, and records what the target held then.
type helperService struct {
	cmd       *exec.Cmd
	done      string
	target    string
	running   bool
	atStop    []byte
	ranAtStop bool
	log       []string
}

func (h *helperService) Installed(context.Context, string) (bool, error) { return true, nil }
func (h *helperService) Running(context.Context, string) (bool, error)   { return h.running, nil }
func (h *helperService) Stop(context.Context, string) error {
	h.log = append(h.log, "stop")
	h.atStop, _ = os.ReadFile(h.target)
	h.ranAtStop = h.running
	if err := os.WriteFile(h.done, []byte("x"), 0o600); err != nil {
		return err
	}
	err := h.cmd.Wait()
	h.running = false
	return err
}
func (h *helperService) Start(context.Context, string) error {
	h.log = append(h.log, "start")
	return nil
}
func (h *helperService) WaitHealthy(context.Context, string) error {
	h.log = append(h.log, "health")
	return nil
}

// TestReplaceBeforeStopRunningImage: with ReplaceBeforeStop, the binary a
// running service holds is replaced before the stop, and the commit after
// the stop removes the backup, which no running image holds by then
// (docs/decisions/0020-MADR-precheck-gofmt-errors-and-replace-before-stop.md
// 4B).
func TestReplaceBeforeStopRunningImage(t *testing.T) {
	home, _ := withTempHome(t)
	src, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	in, err := os.ReadFile(src)
	if err != nil {
		t.Fatal(err)
	}
	exe := filepath.Join(home, "svc.exe")
	if err := os.WriteFile(exe, in, 0o755); err != nil {
		t.Fatal(err)
	}
	ready, done := filepath.Join(home, "ready"), filepath.Join(home, "done")
	cmd := exec.Command(exe)
	cmd.Env = append(os.Environ(), "SELFUPDATE_NATIVE_HELPER=1", "SELFUPDATE_READY="+ready, "SELFUPDATE_DONE="+done)
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	svc := &helperService{cmd: cmd, done: done, target: exe, running: true}
	defer func() {
		if svc.running {
			_ = os.WriteFile(done, []byte("x"), 0o600)
			_ = cmd.Wait()
		}
	}()
	deadline := time.Now().Add(10 * time.Second)
	for {
		if _, err := os.Stat(ready); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("helper did not start")
		}
		time.Sleep(10 * time.Millisecond)
	}

	inner, err := NewStandaloneInstaller(InstallOptions{TargetPolicy: TargetPolicy{ExecutablePath: exe}})
	if err != nil {
		t.Fatal(err)
	}
	m, err := NewManagedInstallerWith(inner, svc, &fakeRec{}, ManagedOptions{ReplaceBeforeStop: true})
	if err != nil {
		t.Fatal(err)
	}
	target, err := m.ResolveTarget(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	sess, err := m.Begin(context.Background(), target)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = sess.Close() }()
	f, path, err := sess.CreateStaging(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	replacement := append(append([]byte{}, in...), "REPLACED"...)
	if _, err := f.Write(replacement); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	res, err := sess.Install(context.Background(), InstallRequest{Product: "svc", Artifact: StagedArtifact{Path: path, Size: int64(len(replacement))}})
	if err != nil || !res.Applied || !res.ReplacedBeforeStop || res.PendingBackup != "" {
		t.Fatalf("res = %+v, err = %v; want applied before the stop, with no pending backup", res, err)
	}
	if !svc.ranAtStop || string(svc.atStop) != string(replacement) {
		t.Fatalf("at Stop: running %v, target %d bytes; want the replacement under the running image", svc.ranAtStop, len(svc.atStop))
	}
	if got, err := os.ReadFile(exe); err != nil || string(got) != string(replacement) {
		t.Fatalf("target: %d bytes, err %v; want the replacement", len(got), err)
	}
	if _, err := os.Lstat(cleanupReceiptPath(target)); !os.IsNotExist(err) {
		t.Fatalf("a cleanup receipt was written: %v", err)
	}
}

// TestKeepPreviousRunningImage: on Windows the backup is a hard link to
// the running image, and KeepPrevious renames it while that image runs,
// so no cleanup receipt is needed (0004-PLAN-v1-1-0-core-api.md Step 10
// rule 3).
func TestKeepPreviousRunningImage(t *testing.T) {
	home, _ := withTempHome(t)
	src, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	in, err := os.ReadFile(src)
	if err != nil {
		t.Fatal(err)
	}
	exe := filepath.Join(home, "helper.exe")
	if err := os.WriteFile(exe, in, 0o755); err != nil {
		t.Fatal(err)
	}
	ready, done := filepath.Join(home, "ready"), filepath.Join(home, "done")
	cmd := exec.Command(exe)
	cmd.Env = append(os.Environ(), "SELFUPDATE_NATIVE_HELPER=1", "SELFUPDATE_READY="+ready, "SELFUPDATE_DONE="+done)
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = os.WriteFile(done, []byte("x"), 0o600)
		_ = cmd.Wait()
	}()
	deadline := time.Now().Add(10 * time.Second)
	for {
		if _, err := os.Stat(ready); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("helper did not start")
		}
		time.Sleep(10 * time.Millisecond)
	}
	running, err := os.Stat(exe)
	if err != nil {
		t.Fatal(err)
	}
	// On Windows, os.Stat loads the file ID lazily, from the path, at the
	// first SameFile: load it now, while exe still names the running image.
	_ = os.SameFile(running, running)

	inst, err := NewStandaloneInstaller(InstallOptions{TargetPolicy: TargetPolicy{ExecutablePath: exe}, KeepPrevious: true})
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
	replacement := append(append([]byte{}, in...), "REPLACED"...)
	if _, err := f.Write(replacement); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	res, err := sess.Install(context.Background(), InstallRequest{Product: "helper", Artifact: StagedArtifact{Path: path, Size: int64(len(replacement))}})
	if err != nil {
		t.Fatalf("res = %+v, err = %v", res, err)
	}
	if !res.Applied || res.PendingBackup != "" || filepath.Base(res.Previous) != ".helper.exe.previous" {
		t.Fatalf("res = %+v, want Previous and no pending backup", res)
	}
	prev, err := os.Stat(res.Previous)
	if err != nil {
		t.Fatal(err)
	}
	if !os.SameFile(running, prev) {
		t.Fatal("previous is not the running image")
	}
	if _, err := os.Lstat(cleanupReceiptPath(target)); !os.IsNotExist(err) {
		t.Fatalf("a cleanup receipt was written: %v", err)
	}
	if got, err := os.ReadFile(exe); err != nil || string(got) != string(replacement) {
		t.Fatalf("target: %d bytes, err %v; want the replacement", len(got), err)
	}
}
