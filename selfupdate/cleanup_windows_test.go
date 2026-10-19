//go:build windows

package selfupdate

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWindowsCleanupReceiptRoundTrip(t *testing.T) {
	_, exe := withTempHome(t)
	target, err := resolveTarget(TargetPolicy{ExecutablePath: exe})
	if err != nil {
		t.Fatal(err)
	}
	backup := filepath.Join(target.Dir, "."+target.Base+".selfupdate-bak-test")
	if err := os.WriteFile(backup, []byte("old-bytes"), 0o600); err != nil {
		t.Fatal(err)
	}
	digest, err := fileSHA256(backup)
	if err != nil {
		t.Fatal(err)
	}
	if err := writeCleanupReceipt(context.Background(), target, applyResult{backup: backup, oldDigest: digest}); err != nil {
		t.Fatal(err)
	}
	root, err := os.OpenRoot(target.Dir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = root.Close() }()
	if err := processCleanupReceipt(context.Background(), target, root); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(backup); !os.IsNotExist(err) {
		t.Fatalf("backup remained: %v", err)
	}
}

func TestWindowsCleanupReceiptDigestMismatch(t *testing.T) {
	_, exe := withTempHome(t)
	target, err := resolveTarget(TargetPolicy{ExecutablePath: exe})
	if err != nil {
		t.Fatal(err)
	}
	// A valid backup name, so the digest check (not the name check) is what
	// refuses it (0003-MADR B6 made "bak" invalid).
	bakName := backupPrefix(target.Base) + "test"
	backup := filepath.Join(target.Dir, bakName)
	if err := os.WriteFile(backup, []byte("old-bytes"), 0o600); err != nil {
		t.Fatal(err)
	}
	rec := cleanupReceipt{Version: 1, Backup: bakName, Digest: "00"}
	data, _ := json.Marshal(rec)
	if err := os.WriteFile(cleanupReceiptPath(target), data, 0o600); err != nil {
		t.Fatal(err)
	}
	root, err := os.OpenRoot(target.Dir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = root.Close() }()
	if err := processCleanupReceipt(context.Background(), target, root); !errors.Is(err, ErrIntegrity) {
		t.Fatalf("err = %v, want the digest mismatch (ErrIntegrity)", err)
	}
}

// receiptEnv writes a backup and a receipt for it, and opens the root.
func receiptEnv(t *testing.T, backupName string, rec cleanupReceipt, writeBackup bool) (Target, *os.Root) {
	t.Helper()
	_, exe := withTempHome(t)
	target, err := resolveTarget(TargetPolicy{ExecutablePath: exe})
	if err != nil {
		t.Fatal(err)
	}
	if writeBackup {
		if err := os.WriteFile(filepath.Join(target.Dir, backupName), []byte("old-bytes"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	data, err := json.Marshal(rec)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cleanupReceiptPath(target), data, 0o600); err != nil {
		t.Fatal(err)
	}
	root, err := os.OpenRoot(target.Dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = root.Close() })
	return target, root
}

func oldBytesDigest(t *testing.T) string {
	t.Helper()
	sum := sha256.Sum256([]byte("old-bytes"))
	return hex.EncodeToString(sum[:])
}

// TestWindowsCleanupReceiptMissingBackup: a receipt whose backup is gone is
// removed instead of blocking every later update (0003-MADR B7).
func TestWindowsCleanupReceiptMissingBackup(t *testing.T) {
	name := ".demo.selfupdate-bak-gone"
	target, root := receiptEnv(t, name, cleanupReceipt{Version: 1, Backup: name, Digest: oldBytesDigest(t)}, false)
	if err := processCleanupReceipt(context.Background(), target, root); err != nil {
		t.Fatalf("stale receipt blocked the update: %v", err)
	}
	if _, err := os.Lstat(cleanupReceiptPath(target)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("stale receipt kept: %v", err)
	}
}

// TestWindowsCleanupReceiptMalformed: malformed receipts, and receipts that
// name anything but a backup of this target, fail closed and are kept
// (0003-MADR B6).
func TestWindowsCleanupReceiptMalformed(t *testing.T) {
	digest := oldBytesDigest(t)
	cases := map[string]cleanupReceipt{
		"version 2":       {Version: 2, Backup: ".demo.selfupdate-bak-1", Digest: digest},
		"empty backup":    {Version: 1, Backup: "", Digest: digest},
		"empty digest":    {Version: 1, Backup: ".demo.selfupdate-bak-1", Digest: ""},
		"names target":    {Version: 1, Backup: "demo", Digest: digest},
		"names lock":      {Version: 1, Backup: ".demo.selfupdate.lock", Digest: digest},
		"absolute path":   {Version: 1, Backup: `C:\Windows\.demo.selfupdate-bak-1`, Digest: digest},
		"traversal":       {Version: 1, Backup: `..\.demo.selfupdate-bak-1`, Digest: digest},
		"other product":   {Version: 1, Backup: ".other.selfupdate-bak-1", Digest: digest},
		"bare prefix":     {Version: 1, Backup: ".demo.selfupdate-bak-", Digest: digest},
		"digest mismatch": {Version: 1, Backup: ".demo.selfupdate-bak-1", Digest: strings.Repeat("0", 64)},
	}
	for name, rec := range cases {
		t.Run(name, func(t *testing.T) {
			backupName := rec.Backup
			if backupName == "" || strings.ContainsAny(backupName, `\/`) {
				backupName = ".demo.selfupdate-bak-1"
			}
			target, root := receiptEnv(t, backupName, rec, true)
			if err := processCleanupReceipt(context.Background(), target, root); err == nil {
				t.Fatal("accepted")
			}
			if _, err := os.Lstat(cleanupReceiptPath(target)); err != nil {
				t.Fatalf("receipt removed after a refusal: %v", err)
			}
			if _, err := os.Lstat(target.Path); err != nil {
				t.Fatalf("target removed: %v", err)
			}
		})
	}
	// Not JSON at all.
	_, exe := withTempHome(t)
	target, err := resolveTarget(TargetPolicy{ExecutablePath: exe})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cleanupReceiptPath(target), []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	root, err := os.OpenRoot(target.Dir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = root.Close() }()
	if err := processCleanupReceipt(context.Background(), target, root); err == nil {
		t.Fatal("accepted a non-JSON receipt")
	}
}

// TestWindowsCleanupReceiptReparseBackup: a backup that is a symlink is not
// followed or removed.
func TestWindowsCleanupReceiptReparseBackup(t *testing.T) {
	name := ".demo.selfupdate-bak-link"
	target, root := receiptEnv(t, name, cleanupReceipt{Version: 1, Backup: name, Digest: oldBytesDigest(t)}, false)
	victim := filepath.Join(target.Dir, "victim")
	if err := os.WriteFile(victim, []byte("old-bytes"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("victim", filepath.Join(target.Dir, name)); err != nil {
		t.Fatalf("symlink (Developer Mode is required on the Windows test host): %v", err)
	}
	if err := processCleanupReceipt(context.Background(), target, root); err == nil {
		t.Fatal("accepted a symlinked backup")
	}
	if _, err := os.Lstat(victim); err != nil {
		t.Fatalf("symlink target removed: %v", err)
	}
}

// TestWindowsReceiptConsumedByBegin: Begin consumes a valid receipt and its
// backup before anything else happens.
func TestWindowsReceiptConsumedByBegin(t *testing.T) {
	_, exe := withTempHome(t)
	inst, err := NewStandaloneInstaller(InstallOptions{TargetPolicy: TargetPolicy{ExecutablePath: exe}})
	if err != nil {
		t.Fatal(err)
	}
	target, err := inst.ResolveTarget(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	name := backupPrefix(target.Base) + "begin"
	backup := filepath.Join(target.Dir, name)
	if err := os.WriteFile(backup, []byte("old-bytes"), 0o600); err != nil {
		t.Fatal(err)
	}
	data, _ := json.Marshal(cleanupReceipt{Version: 1, Backup: name, Digest: oldBytesDigest(t)})
	if err := os.WriteFile(cleanupReceiptPath(target), data, 0o600); err != nil {
		t.Fatal(err)
	}
	sess, err := inst.Begin(context.Background(), target)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = sess.Close() }()
	for _, p := range []string{backup, cleanupReceiptPath(target)} {
		if _, err := os.Lstat(p); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("%s not consumed by Begin: %v", filepath.Base(p), err)
		}
	}
}
