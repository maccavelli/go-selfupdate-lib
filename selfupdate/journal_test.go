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

// Tests for docs/decisions/0017-PLAN-verify-build-provenance-and-close-0015-open-items.md
// Q1: an update interrupted between Apply and Commit leaves its journal,
// and the next session keeps the backup it names (0017-MADR 3B).

// interruptedApply applies an update and closes the session without Commit
// or Rollback, as a process killed in between would leave it.
func interruptedApply(t *testing.T, inst Installer) {
	t.Helper()
	target, err := inst.ResolveTarget(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	sess, err := inst.Begin(context.Background(), target)
	if err != nil {
		t.Fatal(err)
	}
	two, ok := sess.(TwoPhaseSession)
	if !ok {
		if m, isManaged := sess.(*managedSession); isManaged {
			two = m.inner
		} else {
			t.Fatalf("%T is not two-phase", sess)
		}
	}
	if _, err := two.Apply(context.Background(), InstallRequest{Product: "demo", Artifact: StagedArtifact{Path: stageNew(t, sess)}}); err != nil {
		t.Fatal(err)
	}
	if err := sess.Close(); err != nil {
		t.Fatal(err)
	}
}

// keptIn is the paths of the kept backups of demo in dir.
func keptIn(t *testing.T, dir string) []string {
	t.Helper()
	kept, err := filepath.Glob(filepath.Join(dir, ".demo.selfupdate-kept-*"))
	if err != nil {
		t.Fatal(err)
	}
	return kept
}

// wantKept requires exactly one kept backup of demo holding the previous
// binary, beside a target holding the new one, and returns its path.
func wantKept(t *testing.T, exe string) string {
	t.Helper()
	kept := keptIn(t, filepath.Dir(exe))
	if len(kept) != 1 || readString(t, kept[0]) != "old-bytes" {
		t.Fatalf("want one .demo.selfupdate-kept-<n> holding old-bytes, found %q", kept)
	}
	if got := readString(t, exe); got != "new-bytes" {
		t.Fatalf("the target holds %q, want the new binary", got)
	}
	return kept[0]
}

// noJournalIn requires no journal of demo, and no journal temporary file,
// in dir.
func noJournalIn(t *testing.T, dir string) {
	t.Helper()
	left, err := filepath.Glob(filepath.Join(dir, ".demo.selfupdate.pending*"))
	if err != nil {
		t.Fatal(err)
	}
	if len(left) != 0 {
		t.Fatalf("journal files left: %q", left)
	}
}

// samePath reports whether a and b name the same file: KeptBackups gives
// the target directory's resolved path, which may differ from the one a
// test built through a symlink (/var and /private/var on macOS).
func samePath(t *testing.T, a, b string) bool {
	t.Helper()
	ai, aerr := os.Stat(a)
	bi, berr := os.Stat(b)
	return aerr == nil && berr == nil && os.SameFile(ai, bi)
}

func sha256Hex(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

// TestInterruptedApplyKeepsBackup: whatever session comes next, a backup an
// interrupted update left is kept, KeptBackups lists it, and the journal
// is gone.
func TestInterruptedApplyKeepsBackup(t *testing.T) {
	standalone := func(t *testing.T) (*StandaloneInstaller, string) {
		t.Helper()
		_, exe := withTempHome(t)
		inst, err := NewStandaloneInstaller(InstallOptions{TargetPolicy: TargetPolicy{ExecutablePath: exe}})
		if err != nil {
			t.Fatal(err)
		}
		interruptedApply(t, inst)
		return inst, exe
	}
	listed := func(t *testing.T, inst interface {
		KeptBackups(context.Context) ([]KeptBackup, error)
	}, kept string) {
		t.Helper()
		got, err := inst.KeptBackups(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != 1 || !samePath(t, got[0].Path, kept) || got[0].Size != int64(len("old-bytes")) {
			t.Fatalf("KeptBackups = %+v, want %s", got, kept)
		}
	}
	t.Run("CleanupPending", func(t *testing.T) {
		inst, exe := standalone(t)
		if err := inst.CleanupPending(context.Background()); err != nil {
			t.Fatal(err)
		}
		listed(t, inst, wantKept(t, exe))
		noJournalIn(t, filepath.Dir(exe))
	})
	t.Run("Run", func(t *testing.T) {
		inst, exe := standalone(t)
		target, err := inst.ResolveTarget(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		sess, err := inst.Begin(context.Background(), target)
		if err != nil {
			t.Fatal(err)
		}
		if err := sess.Close(); err != nil {
			t.Fatal(err)
		}
		listed(t, inst, wantKept(t, exe))
		noJournalIn(t, filepath.Dir(exe))
	})
	t.Run("Managed", func(t *testing.T) {
		m, _, sess, exe := managedEnv(t, &fakeLife{installed: true, running: true}, &fakeRec{})
		if err := sess.Close(); err != nil {
			t.Fatal(err)
		}
		interruptedApply(t, m)
		if err := m.inner.(*StandaloneInstaller).CleanupPending(context.Background()); err != nil {
			t.Fatal(err)
		}
		listed(t, m, wantKept(t, exe))
		noJournalIn(t, filepath.Dir(exe))
	})
	t.Run("DryRun", func(t *testing.T) {
		inst, exe := standalone(t)
		req := applyReq()
		req.DryRun = true
		if _, err := dryRunUpdater(t, inst).Run(context.Background(), req); err != nil {
			t.Fatal(err)
		}
		// A dry run changes nothing beside the target: the backup and the
		// journal are still there for the next real session.
		backups, err := filepath.Glob(filepath.Join(filepath.Dir(exe), ".demo.selfupdate-bak-*"))
		if err != nil {
			t.Fatal(err)
		}
		if len(backups) != 1 || readString(t, backups[0]) != "old-bytes" {
			t.Fatalf("after a dry run, backups %q; want the one the interrupted update left", backups)
		}
		if _, err := os.Lstat(filepath.Join(filepath.Dir(exe), ".demo.selfupdate.pending")); err != nil {
			t.Fatalf("after a dry run, the journal: %v", err)
		}
		if err := inst.CleanupPending(context.Background()); err != nil {
			t.Fatal(err)
		}
		listed(t, inst, wantKept(t, exe))
		noJournalIn(t, filepath.Dir(exe))
	})
}

// readJournalFile reads the journal beside exe.
func readJournalFile(t *testing.T, exe string) pendingJournal {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(filepath.Dir(exe), ".demo.selfupdate.pending"))
	if err != nil {
		t.Fatalf("the journal: %v", err)
	}
	var j pendingJournal
	if err := json.Unmarshal(b, &j); err != nil {
		t.Fatal(err)
	}
	return j
}

// checkJournal requires the journal an update of demo writes, and its
// backup to hold the previous binary.
func checkJournal(t *testing.T, j pendingJournal, product, backup string) {
	t.Helper()
	if j.Schema != 1 || j.Phase != "applying" || j.Product != product ||
		j.OldDigest != sha256Hex("old-bytes") || j.NewDigest != sha256Hex("new-bytes") ||
		!strings.HasPrefix(j.Backup, ".demo.selfupdate-bak-") {
		t.Fatalf("journal %+v", j)
	}
	if backup != "old-bytes" {
		t.Fatalf("the journal's backup holds %q", backup)
	}
}

// TestJournalDuringInstall: while the installed binary's probe runs, the
// journal names the backup and both digests, with no product; once
// Install returns, it is gone.
func TestJournalDuringInstall(t *testing.T) {
	_, exe := withTempHome(t)
	var seen *pendingJournal
	var backup string
	inst, err := NewStandaloneInstaller(InstallOptions{
		TargetPolicy: TargetPolicy{ExecutablePath: exe},
		PostInstall: ProberFunc(func(context.Context, ProbeRequest) error {
			j := readJournalFile(t, exe)
			seen = &j
			backup = readString(t, filepath.Join(filepath.Dir(exe), j.Backup))
			return nil
		}),
	})
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
	if _, err := sess.Install(context.Background(), InstallRequest{Product: "demo", Artifact: StagedArtifact{Path: stageNew(t, sess)}}); err != nil {
		t.Fatal(err)
	}
	if seen == nil {
		t.Fatal("the probe did not run")
	}
	checkJournal(t, *seen, "", backup)
	noJournalIn(t, filepath.Dir(exe))
}

// TestJournalDuringApply: after Apply the journal names the product, as a
// managed install's would; Commit removes it.
func TestJournalDuringApply(t *testing.T) {
	sess, exe := standaloneSession(t)
	two := sess.(TwoPhaseSession)
	applied, err := two.Apply(context.Background(), InstallRequest{Product: "demo", Artifact: StagedArtifact{Path: stageNew(t, sess)}})
	if err != nil {
		t.Fatal(err)
	}
	j := readJournalFile(t, exe)
	checkJournal(t, j, "demo", readString(t, filepath.Join(filepath.Dir(exe), j.Backup)))
	if _, err := two.Commit(context.Background(), applied); err != nil {
		t.Fatal(err)
	}
	noJournalIn(t, filepath.Dir(exe))
}

// TestJournalWriteFailure: an update whose journal cannot be written fails
// before the target is replaced, and leaves no backup and no journal.
func TestJournalWriteFailure(t *testing.T) {
	sess, exe := standaloneSession(t)
	failing := errors.New("injected journal failure")
	setSeam(t, &writeJournalFn, func(*os.Root, Target, pendingJournal) error { return failing })
	res, err := sess.Install(context.Background(), InstallRequest{Product: "demo", Artifact: StagedArtifact{Path: stageNew(t, sess)}})
	if !errors.Is(err, failing) || res.Applied {
		t.Fatalf("Applied=%v err=%v; want the journal failure", res.Applied, err)
	}
	if got := readString(t, exe); got != "old-bytes" {
		t.Fatalf("target %q", got)
	}
	if left := backupsIn(t, filepath.Dir(exe)); len(left) != 0 {
		t.Fatalf("backups left: %v", left)
	}
	noJournalIn(t, filepath.Dir(exe))
}

// TestApplyRefusesPendingJournal: a journal recovery left (here, one it
// cannot read) blocks the next update, which names it (0017-PLAN N9).
func TestApplyRefusesPendingJournal(t *testing.T) {
	sess, exe := standaloneSession(t)
	journal := filepath.Join(filepath.Dir(exe), ".demo.selfupdate.pending")
	if err := os.WriteFile(journal, []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := sess.(TwoPhaseSession).Apply(context.Background(), InstallRequest{Product: "demo", Artifact: StagedArtifact{Path: stageNew(t, sess)}})
	// The error names the journal by the target directory's resolved path,
	// which may differ from the one built here (an 8.3 TEMP on Windows).
	if err == nil || !strings.Contains(err.Error(), "is pending") || !strings.Contains(err.Error(), string(filepath.Separator)+".demo.selfupdate.pending ") {
		t.Fatalf("err = %v; want the pending journal named", err)
	}
	if got := readString(t, exe); got != "old-bytes" {
		t.Fatalf("target %q", got)
	}
}

// recoveryBackup is the backup name TestJournalRecoveryRules plants.
const recoveryBackup = ".demo.selfupdate-bak-7"

// recoveryPlant is what TestJournalRecoveryRules puts beside the target.
type recoveryPlant struct {
	journal    string // the journal's content; "" none, "dir" a directory
	backup     string // the backup's content; "" none, "dir" a directory, "link" a hard link of the target
	target     string
	takenKept  bool
	failRename bool
}

// recoveryWant is what it requires after CleanupPending.
type recoveryWant struct {
	journal bool   // the journal is still there
	backup  bool   // the backup is still there, under its own name
	kept    int    // kept backups holding old-bytes
	target  string // the target's content
}

// plantFile writes body at path; "" plants nothing and "dir" a directory.
func plantFile(t *testing.T, path, body string, mode os.FileMode) {
	t.Helper()
	switch body {
	case "":
	case "dir":
		if err := os.Mkdir(path, 0o700); err != nil {
			t.Fatal(err)
		}
	default:
		if err := os.WriteFile(path, []byte(body), mode); err != nil {
			t.Fatal(err)
		}
	}
}

// plantRecovery plants p beside exe.
func plantRecovery(t *testing.T, exe string, p recoveryPlant) {
	t.Helper()
	dir := filepath.Dir(exe)
	if err := os.WriteFile(exe, []byte(p.target), 0o755); err != nil {
		t.Fatal(err)
	}
	plantFile(t, filepath.Join(dir, ".demo.selfupdate.pending"), p.journal, 0o600)
	if p.backup == "link" {
		if err := os.Link(exe, filepath.Join(dir, recoveryBackup)); err != nil {
			t.Fatal(err)
		}
	} else {
		plantFile(t, filepath.Join(dir, recoveryBackup), p.backup, 0o755)
	}
	if p.takenKept {
		plantFile(t, filepath.Join(dir, ".demo.selfupdate-kept-7"), "old-bytes", 0o755)
	}
	if p.failRename {
		setSeam(t, &keptRenameFn, func(*os.Root, string, string) error { return errors.New("injected rename failure") })
	}
}

// checkRecovery requires w beside exe.
func checkRecovery(t *testing.T, exe string, p recoveryPlant, w recoveryWant) {
	t.Helper()
	dir := filepath.Dir(exe)
	_, jerr := os.Lstat(filepath.Join(dir, ".demo.selfupdate.pending"))
	_, berr := os.Lstat(filepath.Join(dir, recoveryBackup))
	kept := 0
	for _, k := range keptIn(t, dir) {
		if readString(t, k) == "old-bytes" {
			kept++
		}
	}
	if (jerr == nil) != w.journal || (berr == nil) != w.backup || kept != w.kept {
		t.Fatalf("journal %v (want %v), backup %v (want %v), kept %d (want %d)", jerr == nil, w.journal, berr == nil, w.backup, kept, w.kept)
	}
	if got := readString(t, exe); got != w.target {
		t.Fatalf("target %q, want %q", got, w.target)
	}
	if p.takenKept && readString(t, filepath.Join(dir, ".demo.selfupdate-kept-7")) != "old-bytes" {
		t.Fatal("the earlier kept backup was changed")
	}
}

// TestJournalRecoveryRules: each rule of recoverJournal, from files planted
// as an interrupted update, or something else, would leave them, then
// CleanupPending (0017-PLAN Q1 step 2).
func TestJournalRecoveryRules(t *testing.T) {
	const backup = recoveryBackup
	journalJSON := func(j pendingJournal) string {
		b, err := json.Marshal(j)
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}
	good := pendingJournal{Schema: 1, Backup: backup, OldDigest: sha256Hex("old-bytes"), NewDigest: sha256Hex("new-bytes"), Phase: "applying"}
	type plant = recoveryPlant
	type want = recoveryWant
	cases := []struct {
		name string
		p    plant
		w    want
	}{
		{"1 no journal: the sweep takes the backup", plant{backup: "old-bytes", target: "new-bytes"}, want{target: "new-bytes"}},
		{"2 a journal that is a directory", plant{journal: "dir", backup: "old-bytes", target: "new-bytes"}, want{journal: true, backup: true, target: "new-bytes"}},
		{"3 an unreadable journal", plant{journal: "{", backup: "old-bytes", target: "new-bytes"}, want{journal: true, backup: true, target: "new-bytes"}},
		{"3 a journal naming the target", plant{journal: journalJSON(pendingJournal{Schema: 1, Backup: "demo"}), backup: "old-bytes", target: "new-bytes"}, want{journal: true, backup: true, target: "new-bytes"}},
		{"3 schema 0", plant{journal: journalJSON(pendingJournal{Backup: backup}), backup: "old-bytes", target: "new-bytes"}, want{journal: true, backup: true, target: "new-bytes"}},
		{"4 the backup is gone", plant{journal: journalJSON(good), target: "new-bytes"}, want{target: "new-bytes"}},
		{"6 a backup that is a directory", plant{journal: journalJSON(good), backup: "dir", target: "new-bytes"}, want{journal: true, backup: true, target: "new-bytes"}},
		{"7 the backup is the target's own file", plant{journal: journalJSON(good), backup: "link", target: "old-bytes"}, want{target: "old-bytes"}},
		{"7 the target is still the previous binary", plant{journal: journalJSON(good), backup: "old-bytes", target: "old-bytes"}, want{target: "old-bytes"}},
		{"8 the new binary is live", plant{journal: journalJSON(good), backup: "old-bytes", target: "new-bytes"}, want{kept: 1, target: "new-bytes"}},
		{"8 a third binary is live", plant{journal: journalJSON(good), backup: "old-bytes", target: "other"}, want{kept: 1, target: "other"}},
		{"8 the kept name is taken", plant{journal: journalJSON(good), backup: "old-bytes", target: "new-bytes", takenKept: true}, want{kept: 2, target: "new-bytes"}},
		{"8 the kept rename fails", plant{journal: journalJSON(good), backup: "old-bytes", target: "new-bytes", failRename: true}, want{journal: true, backup: true, target: "new-bytes"}},
		{"8 a later schema with a backup", plant{journal: journalJSON(pendingJournal{Schema: 2, Backup: backup, OldDigest: sha256Hex("old-bytes"), Phase: "committing"}), backup: "old-bytes", target: "new-bytes"}, want{kept: 1, target: "new-bytes"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, exe := withTempHome(t)
			plantRecovery(t, exe, c.p)
			inst, err := NewStandaloneInstaller(InstallOptions{TargetPolicy: TargetPolicy{ExecutablePath: exe}})
			if err != nil {
				t.Fatal(err)
			}
			if err := inst.CleanupPending(context.Background()); err != nil {
				t.Fatalf("CleanupPending: %v", err)
			}
			checkRecovery(t, exe, c.p, c.w)
		})
	}
}

// TestJournalRecoveryReceiptListed: a backup the Windows cleanup receipt
// lists is the receipt's to remove: recovery drops the journal and leaves
// the backup to it (rule 5). The receipt exists on Windows only, so the
// keep list is given directly.
func TestJournalRecoveryReceiptListed(t *testing.T) {
	_, exe := withTempHome(t)
	dir := filepath.Dir(exe)
	target, err := resolveTarget(TargetPolicy{ExecutablePath: exe})
	if err != nil {
		t.Fatal(err)
	}
	const backup = ".demo.selfupdate-bak-7"
	b, err := json.Marshal(pendingJournal{Schema: 1, Backup: backup, OldDigest: sha256Hex("old-bytes"), Phase: "applying"})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".demo.selfupdate.pending"), b, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, backup), []byte("old-bytes"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(exe, []byte("new-bytes"), 0o755); err != nil {
		t.Fatal(err)
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = root.Close() }()
	keep, sweep := recoverJournal(target, root, map[string]bool{backup: true}, true)
	if !keep[backup] || !sweep {
		t.Fatalf("keep %v sweep %v", keep, sweep)
	}
	noJournalIn(t, dir)
	if len(keptIn(t, dir)) != 0 || readString(t, filepath.Join(dir, backup)) != "old-bytes" {
		t.Fatal("the receipt's backup was renamed or changed")
	}
}
