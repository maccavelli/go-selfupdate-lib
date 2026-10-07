package selfupdate

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Tests for docs/decisions/0010-PLAN-v1-6-0-owner-contracts.md S1 (Q1, A3):
// CheckCached caches deterministic outcomes for maxAge. These use only the
// v1.5.1 API, so they run against the unfixed code too.

// outcomeRows are the checks whose error is deterministic: the same
// question gets the same answer until a release changes.
func outcomeRows(t *testing.T) []checkRow {
	t.Helper()
	var rows []checkRow
	for _, r := range checkRows() {
		if r.wantErr != nil && (errors.Is(r.wantErr, ErrLatestOlder) || errors.Is(r.wantErr, ErrMutableRelease) ||
			errors.Is(r.wantErr, ErrUnsupportedPlatform)) {
			rows = append(rows, r)
		}
	}
	if len(rows) != 4 {
		t.Fatalf("found %d deterministic rows, want 4", len(rows))
	}
	return rows
}

// TestCheckCachedDeterministicErrorCached: five calls within maxAge make
// one network call, and each returns the error's class. Once maxAge has
// passed, the next call checks again.
func TestCheckCachedDeterministicErrorCached(t *testing.T) {
	for _, row := range outcomeRows(t) {
		t.Run(row.name, func(t *testing.T) {
			now := cacheNow
			setSeam(t, &timeNow, func() time.Time { return now })
			c, src, _ := checkerFor(t, row)
			store := &memStore{}
			for i := range 5 {
				if _, err := c.CheckCached(context.Background(), row.req, store, time.Hour); !errors.Is(err, row.wantErr) {
					t.Fatalf("call %d: err = %v, want %v", i, err, row.wantErr)
				}
				now = now.Add(time.Minute)
			}
			if n := networkCalls(src); n != 1 || len(store.saves) != 1 {
				t.Fatalf("network calls %d, saves %d; want 1 and 1", n, len(store.saves))
			}
			now = cacheNow.Add(time.Hour)
			if _, err := c.CheckCached(context.Background(), row.req, store, time.Hour); !errors.Is(err, row.wantErr) {
				t.Fatalf("after maxAge: err = %v", err)
			}
			if n := networkCalls(src); n != 2 {
				t.Fatalf("after maxAge: network calls %d, want 2", n)
			}
		})
	}
}

// TestCheckCachedTransientErrorNotCached: an error that may not recur is
// never saved, and every call checks again. A cancellation or a rate limit
// is transient even when it also carries a deterministic sentinel.
func TestCheckCachedTransientErrorNotCached(t *testing.T) {
	for _, transient := range []error{
		errors.New("fixture: connection reset"),
		context.DeadlineExceeded,
		errors.Join(context.Canceled, ErrMutableRelease),
		errors.Join(context.DeadlineExceeded, ErrLatestOlder),
		errors.Join(ErrRateLimited, ErrUnsupportedPlatform),
	} {
		c, src, req, _ := cacheEnv(t)
		src.err = transient
		store := &memStore{}
		for range 3 {
			if _, err := c.CheckCached(context.Background(), req, store, time.Hour); !errors.Is(err, transient) {
				t.Fatalf("%v: err = %v", transient, err)
			}
		}
		if n := networkCalls(src); n != 3 || len(store.saves) != 0 {
			t.Fatalf("%v: network calls %d, saves %d; want 3 and 0", transient, n, len(store.saves))
		}
	}
}

// TestCheckCachedOutcomeRecorded: the saved record names the outcome, says
// nothing is available, and is what a cached call returns, with an error
// that matches the sentinel.
func TestCheckCachedOutcomeRecorded(t *testing.T) {
	want := map[error]CheckOutcome{
		ErrLatestOlder:         CheckLatestOlder,
		ErrMutableRelease:      CheckMutableRelease,
		ErrUnsupportedPlatform: CheckUnsupportedPlatform,
	}
	for _, row := range outcomeRows(t) {
		t.Run(row.name, func(t *testing.T) {
			setSeam(t, &timeNow, func() time.Time { return cacheNow })
			c, _, _ := checkerFor(t, row)
			store := &memStore{}
			fresh, err := c.CheckCached(context.Background(), row.req, store, time.Hour)
			if !errors.Is(err, row.wantErr) || fresh.Outcome != want[row.wantErr] || !fresh.CheckedAt.Equal(cacheNow) ||
				fresh.Availability.Available || fresh.Availability.Product != "demo" || store.rec != fresh {
				t.Fatalf("fresh = %+v, err = %v; saved %+v", fresh, err, store.rec)
			}
			cached, err := c.CheckCached(context.Background(), row.req, store, time.Hour)
			if cached != fresh || !errors.Is(err, row.wantErr) || errors.Is(err, ErrCheckDeferred) ||
				!strings.Contains(err.Error(), "selfupdate: demo: cached check from 2026-09-30T12:00:00Z: ") {
				t.Fatalf("cached = %+v, err = %v", cached, err)
			}
		})
	}
}

// TestCheckOutcomeNames: every outcome has a stable name and its sentinel,
// and an unknown value has neither.
func TestCheckOutcomeNames(t *testing.T) {
	for _, c := range []struct {
		o    CheckOutcome
		name string
		err  error
	}{
		{CheckAnswered, "answered", nil},
		{CheckLatestOlder, "latest-older", ErrLatestOlder},
		{CheckUnsupportedPlatform, "unsupported-platform", ErrUnsupportedPlatform},
		{CheckMutableRelease, "mutable-release", ErrMutableRelease},
		{CheckOutcome(200), "CheckOutcome(200)", nil},
	} {
		// errors.Is with a nil target reports whether the error is nil.
		if c.o.String() != c.name || !errors.Is(c.o.Err(), c.err) {
			t.Errorf("%d: %q, %v; want %q, %v", c.o, c.o.String(), c.o.Err(), c.name, c.err)
		}
		got, err := parseCheckOutcome(c.name)
		if known := c.o.valid(); known != (err == nil) || (known && got != c.o) {
			t.Errorf("parse %q = %d, %v", c.name, got, err)
		}
	}
}

// TestCheckCachedUnknownOutcomeIsMiss: a custom store's record with an
// outcome this package does not know is checked again, not trusted.
func TestCheckCachedUnknownOutcomeIsMiss(t *testing.T) {
	c, src, req, key := cacheEnv(t)
	store := &memStore{rec: CheckRecord{Request: key, Outcome: CheckOutcome(200), CheckedAt: cacheNow.Add(-time.Minute)}}
	rec, err := c.CheckCached(context.Background(), req, store, time.Hour)
	if err != nil || networkCalls(src) != 1 || rec.Outcome != CheckAnswered || !rec.Availability.Available {
		t.Fatalf("rec = %+v, err = %v, calls = %v", rec, err, src.calls)
	}
}

// TestFileCheckStoreKeepsOutcome: the outcome survives a round trip.
func TestFileCheckStoreKeepsOutcome(t *testing.T) {
	s, path := fileStore(t)
	rec := sampleRecord()
	rec.Availability = Availability{Product: "demo", CurrentVersion: "v1.0.0"}
	rec.Outcome = CheckMutableRelease
	if err := s.Save(context.Background(), rec); err != nil {
		t.Fatal(err)
	}
	if doc := readString(t, path); !strings.Contains(doc, `"outcome":"mutable-release"`) {
		t.Fatalf("document %s", doc)
	}
	got, err := s.Load(context.Background())
	if err != nil || got.Outcome != CheckMutableRelease || got.Availability != rec.Availability {
		t.Fatalf("got %+v, %v", got, err)
	}
}

// schema2CheckRecordJSON is a record as v1.3.0 to v1.5.1 wrote it.
const schema2CheckRecordJSON = `{"schema_version":2,"product":"demo","current_version":"v1.0.0","current_build":"release",` +
	`"target_version":"","platform":{"os":"linux","arch":"amd64"},"channel":"","available":true,"force_required":false,` +
	`"operation":"upgrade","selected_version":"v1.1.0","release_url":"https://example.invalid/v1.1.0",` +
	`"asset_name":"demo-linux-amd64","checked_at":"2026-09-30T12:00:00Z","not_before":""}` + "\n"

// TestFileCheckStoreOlderSchemaIsMiss: a record from before outcomes were
// kept loads as a miss, never as an error the caller sees.
func TestFileCheckStoreOlderSchemaIsMiss(t *testing.T) {
	s, path := fileStore(t)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	for _, doc := range []string{schema1CheckRecordJSON, schema2CheckRecordJSON} {
		if err := os.WriteFile(path, []byte(doc), 0o600); err != nil {
			t.Fatal(err)
		}
		if rec, err := s.Load(context.Background()); !errors.Is(err, ErrNoCheckRecord) {
			t.Fatalf("an older record loaded: %+v, %v", rec, err)
		}
	}
	c, src, req, _ := cacheEnv(t)
	if _, err := c.CheckCached(context.Background(), req, s, time.Hour); err != nil || networkCalls(src) != 1 {
		t.Fatalf("CheckCached over an older record: err = %v, calls = %v", err, src.calls)
	}
}
