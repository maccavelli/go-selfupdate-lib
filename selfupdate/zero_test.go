package selfupdate

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"
)

// TestZeroValues: an Updater, Checker or Stream not made by its
// constructor fails with ErrNotConstructed, or does nothing, instead of
// panicking (docs/decisions/0015-MADR-remediate-third-debugging-pass-findings.md C8).
func TestZeroValues(t *testing.T) {
	ctx := context.Background()
	req := Request{Product: "demo", CurrentVersion: "v1.0.0", CurrentBuild: ReleaseBuild, CheckOnly: true}
	cr := CheckRequest{Product: "demo", CurrentVersion: "v1.0.0", CurrentBuild: ReleaseBuild}
	store, err := NewFileCheckStore(filepath.Join(t.TempDir(), "check.json"))
	if err != nil {
		t.Fatal(err)
	}
	for _, u := range []*Updater{nil, {}} {
		if _, err := u.Run(ctx, req); !errors.Is(err, ErrNotConstructed) {
			t.Errorf("Run on %#v: %v", u, err)
		}
		if _, err := u.RunWith(ctx, req); !errors.Is(err, ErrNotConstructed) {
			t.Errorf("RunWith on %#v: %v", u, err)
		}
	}
	for _, c := range []*Checker{nil, {}, (&Updater{}).Checker()} {
		if _, err := c.Check(ctx, cr); !errors.Is(err, ErrNotConstructed) {
			t.Errorf("Check on %#v: %v", c, err)
		}
		if _, err := c.CheckCached(ctx, cr, store, time.Hour); !errors.Is(err, ErrNotConstructed) {
			t.Errorf("CheckCached on %#v: %v", c, err)
		}
	}
	s := Start(ctx, &Updater{}, req)
	it, err := s.Next(ctx)
	if f, ok := it.(Finished); err != nil || !ok || !errors.Is(f.Err, ErrNotConstructed) {
		t.Errorf("Start(&Updater{}): %#v, %v", it, err)
	}
	var zero Stream
	zero.Cancel()
	var nilStream *Stream
	nilStream.Cancel()
}
