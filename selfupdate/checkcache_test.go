package selfupdate

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// Tests for docs/decisions/0004-PLAN-v1-1-0-core-api.md Step 4.

var cacheNow = time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)

type memStore struct {
	rec     CheckRecord
	loadErr error
	saveErr error
	saves   []CheckRecord
}

func (m *memStore) Load(context.Context) (CheckRecord, error) {
	if m.loadErr != nil {
		return CheckRecord{}, m.loadErr
	}
	return m.rec, nil
}

func (m *memStore) Save(_ context.Context, r CheckRecord) error {
	m.saves = append(m.saves, r)
	if m.saveErr != nil {
		return m.saveErr
	}
	m.rec = r
	return nil
}

// cacheEnv is a checker at a fixed clock, and the normalized key for the
// default upgrade request.
func cacheEnv(t *testing.T) (*Checker, *scriptSource, CheckRequest, CheckRequest) {
	t.Helper()
	setSeam(t, &timeNow, func() time.Time { return cacheNow })
	c, src, _ := checkerFor(t, checkRows()[0])
	req := checkRows()[0].req
	key := req
	key.Platform = normalizePlatform(Platform{})
	return c, src, req, key
}

func networkCalls(src *scriptSource) int {
	n := 0
	for _, c := range src.calls {
		if c == "Latest" || c == "ByTag" || c == "ListReleases" {
			n++
		}
	}
	return n
}

func TestCheckCachedFreshHit(t *testing.T) {
	c, src, req, key := cacheEnv(t)
	store := &memStore{rec: CheckRecord{Request: key, Availability: Availability{Available: true, TargetVersion: "v9.9.9"},
		CheckedAt: cacheNow.Add(-time.Minute)}}
	rec, err := c.CheckCached(context.Background(), req, store, time.Hour)
	if err != nil || rec.Availability.TargetVersion != "v9.9.9" || networkCalls(src) != 0 {
		t.Fatalf("rec=%+v err=%v calls=%v", rec, err, src.calls)
	}
}

func TestCheckCachedStaleRefresh(t *testing.T) {
	c, src, req, key := cacheEnv(t)
	store := &memStore{rec: CheckRecord{Request: key, CheckedAt: cacheNow.Add(-2 * time.Hour)}}
	rec, err := c.CheckCached(context.Background(), req, store, time.Hour)
	if err != nil || networkCalls(src) != 1 || !rec.CheckedAt.Equal(cacheNow) || rec.Availability.TargetVersion != "v1.1.0" {
		t.Fatalf("rec=%+v err=%v calls=%v", rec, err, src.calls)
	}
	if len(store.saves) != 1 || store.saves[0] != rec {
		t.Fatalf("saves = %+v", store.saves)
	}
}

func TestCheckCachedKeyChange(t *testing.T) {
	c, src, req, key := cacheEnv(t)
	key.CurrentVersion = "v0.9.0"
	store := &memStore{rec: CheckRecord{Request: key, CheckedAt: cacheNow.Add(-time.Minute)}}
	if _, err := c.CheckCached(context.Background(), req, store, time.Hour); err != nil || networkCalls(src) != 1 {
		t.Fatalf("a record for another running version was reused: err=%v calls=%v", err, src.calls)
	}
}

func TestCheckCachedClockSkew(t *testing.T) {
	c, src, req, key := cacheEnv(t)
	store := &memStore{rec: CheckRecord{Request: key, CheckedAt: cacheNow.Add(time.Hour)}}
	if _, err := c.CheckCached(context.Background(), req, store, 2*time.Hour); err != nil || networkCalls(src) != 1 {
		t.Fatalf("a record from the future was reused: err=%v calls=%v", err, src.calls)
	}
}

func rateLimited(t *testing.T, rl *RateLimitError) (CheckRecord, *memStore, error) {
	t.Helper()
	c, src, req, _ := cacheEnv(t)
	src.err = rl
	store := &memStore{loadErr: ErrNoCheckRecord}
	rec, err := c.CheckCached(context.Background(), req, store, time.Hour)
	return rec, store, err
}

func TestCheckCachedRateLimitReset(t *testing.T) {
	rec, store, err := rateLimited(t, &RateLimitError{StatusCode: 403, Reset: cacheNow.Add(10 * time.Minute)})
	var rl *RateLimitError
	if !errors.As(err, &rl) || !rec.NotBefore.Equal(cacheNow.Add(10*time.Minute)) || len(store.saves) != 1 {
		t.Fatalf("rec=%+v err=%v saves=%d", rec, err, len(store.saves))
	}
}

func TestCheckCachedRateLimitRetryAfter(t *testing.T) {
	rec, _, _ := rateLimited(t, &RateLimitError{StatusCode: 429, RetryAfter: 30 * time.Second})
	if !rec.NotBefore.Equal(cacheNow.Add(30 * time.Second)) {
		t.Fatalf("NotBefore = %v", rec.NotBefore)
	}
	rec, _, _ = rateLimited(t, &RateLimitError{StatusCode: 429, RetryAfter: time.Minute, Reset: cacheNow.Add(10 * time.Second)})
	if !rec.NotBefore.Equal(cacheNow.Add(time.Minute)) {
		t.Fatalf("NotBefore = %v, want the later of reset and retry-after", rec.NotBefore)
	}
}

func TestCheckCachedRateLimitDefaultMinute(t *testing.T) {
	rec, _, _ := rateLimited(t, &RateLimitError{StatusCode: 403})
	if !rec.NotBefore.Equal(cacheNow.Add(time.Minute)) {
		t.Fatalf("NotBefore = %v, want one minute ahead", rec.NotBefore)
	}
}

func TestCheckCachedDeferredNoNetwork(t *testing.T) {
	c, src, req, key := cacheEnv(t)
	prior := CheckRecord{Request: key, Availability: Availability{Available: true}, CheckedAt: cacheNow.Add(-3 * time.Hour),
		NotBefore: cacheNow.Add(5 * time.Minute)}
	store := &memStore{rec: prior}
	rec, err := c.CheckCached(context.Background(), req, store, time.Hour)
	if !errors.Is(err, ErrCheckDeferred) || !errors.Is(err, ErrRateLimited) || rec != prior || networkCalls(src) != 0 {
		t.Fatalf("rec=%+v err=%v calls=%v", rec, err, src.calls)
	}
}

func TestCheckCachedSaveError(t *testing.T) {
	c, _, req, _ := cacheEnv(t)
	store := &memStore{loadErr: ErrNoCheckRecord, saveErr: errors.New("disk full")}
	rec, err := c.CheckCached(context.Background(), req, store, time.Hour)
	if err == nil || !strings.Contains(err.Error(), "save check record") || rec.CheckedAt.IsZero() || !rec.Availability.Available {
		t.Fatalf("rec=%+v err=%v; want the fresh answer with the save error", rec, err)
	}
}

func TestCheckCachedArguments(t *testing.T) {
	c, _, req, _ := cacheEnv(t)
	if _, err := c.CheckCached(context.Background(), req, nil, time.Hour); err == nil {
		t.Fatal("nil store accepted")
	}
	if _, err := c.CheckCached(context.Background(), req, &memStore{}, 0); err == nil {
		t.Fatal("zero max age accepted")
	}
}

const wantCheckRecordJSON = `{"schema_version":3,"product":"demo","current_version":"v1.0.0","current_build":"release",` +
	`"target_version":"","platform":{"os":"linux","arch":"amd64"},"channel":"","outcome":"answered","available":true,"force_required":false,` +
	`"operation":"upgrade","selected_version":"v1.1.0","release_url":"https://example.invalid/v1.1.0",` +
	`"asset_name":"demo-linux-amd64","checked_at":"2026-09-30T12:00:00Z","not_before":""}` + "\n"

func sampleRecord() CheckRecord {
	req := CheckRequest{Product: "demo", CurrentVersion: "v1.0.0", CurrentBuild: ReleaseBuild,
		Platform: Platform{OS: "linux", Arch: "amd64"}}
	return CheckRecord{
		Request: req,
		Availability: Availability{Product: "demo", CurrentVersion: "v1.0.0", TargetVersion: "v1.1.0",
			ReleaseURL: "https://example.invalid/v1.1.0", AssetName: "demo-linux-amd64",
			Operation: OperationUpgrade, Available: true},
		CheckedAt: cacheNow,
	}
}

func fileStore(t *testing.T) (CheckStore, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "cache", "demo-check.json")
	s, err := NewFileCheckStore(path)
	if err != nil {
		t.Fatal(err)
	}
	return s, path
}

func TestFileCheckStoreRoundTrip(t *testing.T) {
	s, path := fileStore(t)
	ctx := context.Background()
	if _, err := s.Load(ctx); !errors.Is(err, ErrNoCheckRecord) {
		t.Fatalf("missing file: %v", err)
	}
	want := sampleRecord()
	if err := s.Save(ctx, want); err != nil {
		t.Fatal(err)
	}
	if got := readString(t, path); got != wantCheckRecordJSON {
		t.Fatalf("document:\n%s\nwant:\n%s", got, wantCheckRecordJSON)
	}
	ents, err := os.ReadDir(filepath.Dir(path))
	if err != nil {
		t.Fatal(err)
	}
	if len(ents) != 1 {
		t.Fatalf("temporary files left behind: %v", ents)
	}
	got, err := s.Load(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if got.Request != want.Request || got.Availability != want.Availability ||
		!got.CheckedAt.Equal(want.CheckedAt) || !got.NotBefore.IsZero() {
		t.Fatalf("round trip: %+v, want %+v", got, want)
	}
}

func TestFileCheckStoreCorruptIsMiss(t *testing.T) {
	s, path := fileStore(t)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	for _, body := range []string{"not json", `{"schema_version":3,"outcome":"answered","operation":"sideways"}`, `{"schema_version":3,"outcome":"sideways"}`, ""} {
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := s.Load(context.Background()); !errors.Is(err, ErrNoCheckRecord) {
			t.Errorf("body %q: %v", body, err)
		}
	}
}

func TestFileCheckStoreSchemaMismatch(t *testing.T) {
	s, path := fileStore(t)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	for _, schema := range []string{`"schema_version":4`, `"schema_version":0`} {
		if err := os.WriteFile(path, []byte(strings.Replace(wantCheckRecordJSON, `"schema_version":3`, schema, 1)), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := s.Load(context.Background()); !errors.Is(err, ErrNoCheckRecord) {
			t.Fatalf("a record with %s loaded: %v", schema, err)
		}
	}
}

func TestFileCheckStoreMode(t *testing.T) {
	s, path := fileStore(t)
	if err := s.Save(context.Background(), sampleRecord()); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != goosWindows && info.Mode().Perm() != 0o600 {
		t.Fatalf("mode = %v, want 0600", info.Mode().Perm())
	}
}

func TestFileCheckStoreReplaces(t *testing.T) {
	s, _ := fileStore(t)
	ctx := context.Background()
	first := sampleRecord()
	second := sampleRecord()
	second.Availability.TargetVersion = "v1.2.0"
	for _, r := range []CheckRecord{first, second} {
		if err := s.Save(ctx, r); err != nil {
			t.Fatal(err)
		}
	}
	got, err := s.Load(ctx)
	if err != nil || got.Availability.TargetVersion != "v1.2.0" {
		t.Fatalf("got %+v err=%v", got, err)
	}
}

func TestNewFileCheckStoreNeedsAbsolutePath(t *testing.T) {
	for _, p := range []string{"", "relative/check.json"} {
		if _, err := NewFileCheckStore(p); err == nil {
			t.Errorf("%q accepted", p)
		}
	}
}

// Tests for docs/decisions/0005-PLAN-opt-in-prerelease-channels.md Step 3.

// schema1CheckRecordJSON is a record as v1.2.0 wrote it: no channel. Since
// schema 3 it loads as a miss (TestFileCheckStoreOlderSchemaIsMiss).
const schema1CheckRecordJSON = `{"schema_version":1,"product":"demo","current_version":"v1.0.0","current_build":"release",` +
	`"target_version":"","platform":{"os":"linux","arch":"amd64"},"available":true,"force_required":false,` +
	`"operation":"upgrade","selected_version":"v1.1.0","release_url":"https://example.invalid/v1.1.0",` +
	`"asset_name":"demo-linux-amd64","checked_at":"2026-09-30T12:00:00Z","not_before":""}` + "\n"

func TestFileCheckStoreKeepsChannel(t *testing.T) {
	s, _ := fileStore(t)
	rec := sampleRecord()
	rec.Request.Channel = "rc"
	if err := s.Save(context.Background(), rec); err != nil {
		t.Fatal(err)
	}
	got, err := s.Load(context.Background())
	if err != nil || got.Request.Channel != "rc" {
		t.Fatalf("channel after a round trip = %q, %v", got.Request.Channel, err)
	}
}

// TestCheckCachedChannelIsolation: an answer saved for one channel is never
// served for another.
func TestCheckCachedChannelIsolation(t *testing.T) {
	setSeam(t, &timeNow, func() time.Time { return cacheNow })
	policy, err := NewSemverPolicy(SemverOptions{AllowPrerelease: true, Channels: []string{"rc", "beta"}})
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct{ saved, asked string }{{"rc", ""}, {"", "rc"}, {"rc", "beta"}} {
		ch, src, _ := checkerFor(t, checkRow{policy: policy})
		req := checkRows()[0].req
		key := req
		key.Platform = normalizePlatform(Platform{})
		key.Channel = c.saved
		store := &memStore{rec: CheckRecord{Request: key, Availability: Availability{Available: true, TargetVersion: "v9.9.9"},
			CheckedAt: cacheNow.Add(-time.Minute)}}
		req.Channel = c.asked
		rec, err := ch.CheckCached(context.Background(), req, store, time.Hour)
		if err != nil || rec.Availability.TargetVersion == "v9.9.9" || networkCalls(src) != 1 {
			t.Errorf("saved on %q, asked on %q: rec=%+v err=%v calls=%v; want a fresh check", c.saved, c.asked, rec, err, src.calls)
		}
	}
}
