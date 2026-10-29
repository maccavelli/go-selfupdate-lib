package selfupdate

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// Tests that the install path resolves every name through the session's
// root, so a directory swapped in during an update is never written to
// (docs/decisions/0038-MADR-pinned-plans-lifecycle-conformance-and-bubble-tests.md
// F8, P2, D3-A).

// testRoot opens dir as a root, closed when the test ends.
func testRoot(t *testing.T, dir string) *os.Root {
	t.Helper()
	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = root.Close() })
	return root
}

// skipSwapOnWindows skips a test that swaps a directory: Windows refuses to
// rename one while the session holds handles inside it.
func skipSwapOnWindows(t *testing.T) {
	t.Helper()
	if runtime.GOOS == goosWindows {
		t.Skip("a directory swap is refused on Windows")
	}
}

// decoy is a directory swapped in for a locked one.
type decoy struct {
	dir, moved string
	before     map[string]string
}

// swapToDecoy renames dir aside and makes a decoy directory in its place,
// holding demo and a file at each of names, so a path-based call finds
// something to act on. It returns the decoy with what it holds.
func swapToDecoy(t *testing.T, dir string, names ...string) *decoy {
	t.Helper()
	d := &decoy{dir: dir, moved: dir + ".moved"}
	if err := os.Rename(dir, d.moved); err != nil {
		t.Fatalf("swap: %v", err)
	}
	t.Cleanup(func() {
		_ = os.RemoveAll(dir)
		_ = os.Rename(d.moved, dir)
	})
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range append([]string{"demo"}, names...) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("decoy "+name), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	d.before = snapshot(t, dir)
	return d
}

// readOnly makes the decoy read-only, so a call by path that would create
// or remove a name in it fails, unless the test runs as root.
func (d *decoy) readOnly(t *testing.T) {
	t.Helper()
	if err := os.Chmod(d.dir, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(d.dir, 0o755) })
}

// untouched requires the decoy to hold what it was made with.
func (d *decoy) untouched(t *testing.T) {
	t.Helper()
	after := snapshot(t, d.dir)
	for name, was := range d.before {
		if after[name] != was {
			t.Errorf("decoy %s: %q, want %q", name, after[name], was)
		}
	}
	for name, is := range after {
		if _, ok := d.before[name]; !ok {
			t.Errorf("decoy gained %s: %q", name, is)
		}
	}
}

// snapshot is each name in dir with its mode and content.
func snapshot(t *testing.T, dir string) map[string]string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	out := make(map[string]string, len(entries))
	for _, e := range entries {
		info, err := e.Info()
		if err != nil {
			t.Fatal(err)
		}
		b, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			t.Fatal(err)
		}
		out[e.Name()] = info.Mode().String() + " " + string(b)
	}
	return out
}

// TestInstallRefusesDirectorySwappedBeforeRename is probe P2: the directory
// is swapped after the target is checked and before the rename. The rename
// goes into the locked directory, where the session sees the swap and
// rolls back; the decoy is untouched, and no backup is left.
func TestInstallRefusesDirectorySwappedBeforeRename(t *testing.T) {
	t.Parallel()
	skipSwapOnWindows(t)
	sess, exe := standaloneSession(t)
	path := stageNew(t, sess)
	fx := fsOf(t, sess)
	real := fx.writeJournalFn
	var d *decoy
	fx.writeJournalFn = func(root *os.Root, target Target, j pendingJournal) error {
		if err := real(root, target, j); err != nil {
			return err
		}
		d = swapToDecoy(t, filepath.Dir(exe), filepath.Base(path))
		return nil
	}
	res, err := sess.Install(context.Background(), InstallRequest{Product: "demo", Artifact: StagedArtifact{Path: path}})
	if d == nil {
		t.Fatalf("the journal was never written: %+v, %v", res, err)
	}
	if !errors.Is(err, ErrConcurrentUpdate) || res.Applied || !res.RolledBack {
		t.Fatalf("Install = %+v, %v; want rolled back with ErrConcurrentUpdate", res, err)
	}
	d.untouched(t)
	if got := readString(t, filepath.Join(d.moved, "demo")); got != "old-bytes" {
		t.Errorf("locked directory's target holds %q", got)
	}
	if left := backupsIn(t, d.moved); len(left) != 0 {
		t.Errorf("backups left in the locked directory: %v", left)
	}
	noJournalIn(t, d.moved)
}

// swapCase is one row of TestInstallSwapAtEachStep.
type swapCase struct {
	name string
	// swap is when: "begin", right after Begin and before staging;
	// "check", right after the replacement checks the directory;
	// "probe", during the post-install probe.
	swap         string
	probeErr     error
	keepPrevious bool
	// noLink refuses the hard link, so the backup is a copy.
	noLink     bool
	wantErr    error
	applied    bool
	rolledBack bool
	want       string
}

// swapRun is what one swapCase's install did.
type swapRun struct {
	res      InstallResult
	err      error
	closeErr error
	d        *decoy
	journal  pendingJournal
	original os.FileInfo
}

// TestInstallSwapAtEachStep swaps the directory at each step of an
// install for a read-only decoy holding only demo. Every name resolved
// after the swap is the locked directory's: the install ends there as it
// would without the swap, or is refused by the check after it, and a call
// by path would find nothing in the decoy, or be refused writing to it.
func TestInstallSwapAtEachStep(t *testing.T) {
	t.Parallel()
	skipSwapOnWindows(t)
	unhealthy := errors.New("injected probe failure")
	for _, tc := range []swapCase{
		{name: "after Begin", swap: "begin", wantErr: ErrConcurrentUpdate, want: "old-bytes"},
		{name: "after the check", swap: "check", wantErr: ErrConcurrentUpdate, rolledBack: true, want: "old-bytes"},
		{name: "after the check, with no hard links", swap: "check", noLink: true, wantErr: ErrConcurrentUpdate, rolledBack: true, want: "old-bytes"},
		{name: "in a healthy probe", swap: "probe", applied: true, want: "new-bytes"},
		{name: "in a healthy probe, keeping the previous", swap: "probe", keepPrevious: true, applied: true, want: "new-bytes"},
		{name: "in a failed probe", swap: "probe", probeErr: unhealthy, wantErr: unhealthy, rolledBack: true, want: "old-bytes"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			tc.check(t, tc.run(t))
		})
	}
}

// run installs over a fresh target, swapping its directory when tc says.
func (tc swapCase) run(t *testing.T) swapRun {
	t.Helper()
	_, exe := tempTarget(t)
	var r swapRun
	var err error
	if r.original, err = os.Lstat(exe); err != nil {
		t.Fatal(err)
	}
	swap := func() {
		r.d = swapToDecoy(t, filepath.Dir(exe))
		r.d.readOnly(t)
	}
	opts := InstallOptions{TargetPolicy: targetPolicy(exe), KeepPrevious: tc.keepPrevious}
	if tc.swap == "probe" {
		opts.PostInstall = ProberFunc(func(context.Context, ProbeRequest) error {
			swap()
			return tc.probeErr
		})
	}
	inst, err := NewStandaloneInstaller(opts)
	if err != nil {
		t.Fatal(err)
	}
	fx := fsFor(inst)
	if tc.swap == "check" {
		fx.afterCheckHook = swap
	}
	if tc.noLink {
		fx.rootLink = func(*os.Root, string, string) error { return errors.New("injected link failure") }
	}
	realJournal := fx.writeJournalFn
	fx.writeJournalFn = func(root *os.Root, target Target, j pendingJournal) error {
		r.journal = j
		return realJournal(root, target, j)
	}
	target, err := inst.ResolveTarget(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	sess, err := inst.Begin(context.Background(), target)
	if err != nil {
		t.Fatal(err)
	}
	if tc.swap == "begin" {
		swap()
	}
	path := stageNew(t, sess)
	r.res, r.err = sess.Install(context.Background(), InstallRequest{Product: "demo", Artifact: StagedArtifact{Path: path}})
	r.closeErr = sess.Close()
	if r.d == nil {
		t.Fatalf("no swap happened: %+v, %v", r.res, r.err)
	}
	return r
}

// check requires the result tc wants, the decoy untouched, and the locked
// directory as the install left it.
func (tc swapCase) check(t *testing.T, r swapRun) {
	t.Helper()
	if tc.wantErr == nil && r.err != nil || tc.wantErr != nil && !errors.Is(r.err, tc.wantErr) {
		t.Fatalf("Install = %+v, %v; want %v", r.res, r.err, tc.wantErr)
	}
	if r.res.Applied != tc.applied || r.res.RolledBack != tc.rolledBack {
		t.Fatalf("Install = %+v; want Applied=%t RolledBack=%t", r.res, tc.applied, tc.rolledBack)
	}
	if r.closeErr != nil {
		t.Errorf("Close: %v", r.closeErr)
	}
	r.d.untouched(t)
	locked := filepath.Join(r.d.moved, "demo")
	if got := readString(t, locked); got != tc.want {
		t.Errorf("locked directory's target holds %q, want %q", got, tc.want)
	}
	if left := stagingLeft(t, r.d.moved, "demo"); len(left) != 0 {
		t.Errorf("left in the locked directory: %v", left)
	}
	noJournalIn(t, r.d.moved)
	if tc.keepPrevious {
		if got := readString(t, filepath.Join(r.d.moved, ".demo.previous")); got != "old-bytes" {
			t.Errorf("the previous binary holds %q", got)
		}
	}
	if tc.swap != "check" {
		return
	}
	// The digests the journal recorded are the locked directory's files,
	// and the previous binary came back as its own file: the backup was a
	// hard link made there.
	if r.journal.OldDigest != sha256Hex("old-bytes") || r.journal.NewDigest != sha256Hex("new-bytes") {
		t.Errorf("journal digests %s, %s; want old-bytes', new-bytes'", r.journal.OldDigest, r.journal.NewDigest)
	}
	if now, err := os.Lstat(locked); !tc.noLink && (err != nil || !os.SameFile(r.original, now)) {
		t.Errorf("the restored target is not the original file: %v", err)
	}
}

// TestClearSpecialBitsThroughRoot: clearSpecialBits drops setuid from the
// file in the locked directory, through a descriptor opened from the root,
// and not from the decoy's file of the same name.
func TestClearSpecialBitsThroughRoot(t *testing.T) {
	t.Parallel()
	skipSwapOnWindows(t)
	dir := t.TempDir()
	kept := filepath.Join(dir, ".demo.selfupdate-kept-1")
	if err := os.WriteFile(kept, []byte("old-bytes"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(kept, os.ModeSetuid|0o755); err != nil {
		t.Skipf("chmod setuid: %v", err)
	}
	root := testRoot(t, dir)
	d := swapToDecoy(t, dir)
	decoyKept := filepath.Join(dir, filepath.Base(kept))
	if err := os.WriteFile(decoyKept, []byte("decoy"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(decoyKept, os.ModeSetuid|0o755); err != nil {
		t.Fatal(err)
	}
	d.before = snapshot(t, dir)
	if err := realFS().clearSpecialBits(root, filepath.Base(kept)); err != nil {
		t.Fatal(err)
	}
	d.untouched(t)
	info, err := os.Lstat(filepath.Join(d.moved, filepath.Base(kept)))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode()&os.ModeSetuid != 0 || info.Mode().Perm() != 0o755 {
		t.Fatalf("locked kept backup mode %v; want 0755 without setuid", info.Mode())
	}
}

// TestKeepInterruptedThroughRoot: an interrupted update's backup whose kept
// name is taken gets a fresh kept name, reserved in the locked directory
// through the root.
func TestKeepInterruptedThroughRoot(t *testing.T) {
	t.Parallel()
	skipSwapOnWindows(t)
	_, exe := tempTarget(t)
	dir := filepath.Dir(exe)
	backup := backupPrefix("demo") + "7"
	for _, name := range []string{backup, ".demo.selfupdate-kept-7"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(name), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	root := testRoot(t, dir)
	target := Target{Path: exe, Dir: dir, Base: "demo"}
	d := swapToDecoy(t, dir)
	d.readOnly(t)
	if !realFS().keepInterrupted(root, target, backup) {
		t.Fatal("the backup was not kept")
	}
	d.untouched(t)
	kept := keptIn(t, d.moved)
	if len(kept) != 2 {
		t.Fatalf("kept names in the locked directory: %q, want the taken one and a fresh one", kept)
	}
}

// TestRollbackRemovesSameFileBackup: a rollback whose backup is the
// target's own file, so its rename does nothing, removes the backup
// (0038-MADR P2).
func TestRollbackRemovesSameFileBackup(t *testing.T) {
	t.Parallel()
	for name, rollback := range map[string]func(root *os.Root, target Target, a applyResult) error{
		"rollbackReplacement": func(root *os.Root, target Target, a applyResult) error {
			return realFS().rollbackReplacement(context.Background(), root, target, a)
		},
		"rollbackInRoot": func(root *os.Root, target Target, a applyResult) error {
			s := &installSession{fs: realFS(), root: root, target: target}
			return s.rollbackInRoot(a)
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			_, exe := tempTarget(t)
			dir := filepath.Dir(exe)
			backup := filepath.Join(dir, backupPrefix("demo")+"1")
			if err := os.Link(exe, backup); err != nil {
				t.Skipf("hard link: %v", err)
			}
			target := Target{Path: exe, Dir: dir, Base: "demo"}
			if err := rollback(testRoot(t, dir), target, applyResult{backup: backup}); err != nil {
				t.Fatal(err)
			}
			if got := readString(t, exe); got != "old-bytes" {
				t.Fatalf("target holds %q", got)
			}
			if left := backupsIn(t, dir); len(left) != 0 {
				t.Fatalf("backups left: %v", left)
			}
		})
	}
}

// TestRootNamesAreLeftovers: the names the install path makes in the root
// are the ones the leftover sweep and keptName parse.
func TestRootNamesAreLeftovers(t *testing.T) {
	t.Parallel()
	_, exe := tempTarget(t)
	root := testRoot(t, filepath.Dir(exe))
	target := Target{Path: exe, Dir: filepath.Dir(exe), Base: "demo"}
	var names []string
	for _, prefix := range []string{".demo.selfupdate-", journalTmpPrefix("demo"), ".demo.selfupdate.cleanup-tmp-"} {
		suffix := ""
		if prefix == ".demo.selfupdate-" {
			suffix = stagingSuffix()
		}
		f, name, err := rootCreateTemp(root, prefix, suffix)
		if err != nil {
			t.Fatal(err)
		}
		if err := f.Close(); err != nil {
			t.Fatal(err)
		}
		names = append(names, name)
	}
	backup, err := realFS().backupFile(root, target)
	if err != nil {
		t.Fatal(err)
	}
	names = append(names, backup)
	for _, name := range names {
		if !isLeftover("demo", name) {
			t.Errorf("%s is not a leftover", name)
		}
	}
	if _, ok := keptName("demo", backup); !ok {
		t.Errorf("backup %s has no kept name", backup)
	}
	fresh, err := rootReserveName(root, ".demo.selfupdate-kept-")
	if err != nil {
		t.Fatal(err)
	}
	if isLeftover("demo", fresh) {
		t.Errorf("fresh kept name %s is a leftover", fresh)
	}
	if _, err := root.Lstat(fresh); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("reserved name %s exists: %v", fresh, err)
	}
}
