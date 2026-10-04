package selfupdate

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// Cached, rate-limit-aware checks for startup banners
// (docs/decisions/0004-MADR-evolve-selfupdate-api-and-tui-support.md §3).

// CheckRecord is one cached answer and its back-off state.
type CheckRecord struct {
	// Request is the question, with Platform normalized (never zero).
	Request CheckRequest
	// Availability is the answer as of CheckedAt. It is meaningful only
	// when CheckedAt is non-zero.
	Availability Availability
	// CheckedAt is when the answer was obtained.
	CheckedAt time.Time
	// NotBefore defers any network check until this instant, after a rate
	// limit.
	NotBefore time.Time
}

// CheckStore persists the last CheckRecord.
type CheckStore interface {
	Load(context.Context) (CheckRecord, error)
	Save(context.Context, CheckRecord) error
}

var (
	// ErrNoCheckRecord is returned by a CheckStore with no usable record:
	// none saved, or one that cannot be read.
	ErrNoCheckRecord = errors.New("selfupdate: no cached check record")
	// ErrCheckDeferred is returned by CheckCached while an earlier rate
	// limit still defers checks. It matches ErrRateLimited.
	ErrCheckDeferred = fmt.Errorf("selfupdate: check deferred by an earlier rate limit: %w", ErrRateLimited)
)

// CheckCached answers from store when its record is for the same request
// and younger than maxAge, and otherwise runs Check and saves the answer. It
// never prompts and never applies anything.
//
// While a saved NotBefore is in the future it returns the saved record and
// ErrCheckDeferred without touching the network. A rate-limited check saves
// a NotBefore taken from the error's Reset and RetryAfter, or one minute
// ahead when the response gave neither.
//
// Whatever the error, a returned record with a non-zero CheckedAt holds a
// real answer as of CheckedAt.
func (c *Checker) CheckCached(ctx context.Context, cr CheckRequest, store CheckStore, maxAge time.Duration) (CheckRecord, error) {
	if isNil(store) {
		return CheckRecord{}, fmt.Errorf("selfupdate: check store is required")
	}
	if maxAge <= 0 {
		return CheckRecord{}, fmt.Errorf("selfupdate: check max age must be positive")
	}
	req, err := c.prepare(cr)
	if err != nil {
		return CheckRecord{}, err
	}
	key := cr
	key.Platform = req.Platform
	now := timeNow()
	rec, lerr := store.Load(ctx)
	matched := lerr == nil && rec.Request == key
	if !matched {
		rec = CheckRecord{}
	}
	// A deferral beyond the cap is not one this package wrote; the record
	// is a miss (0010-MADR A4).
	if matched && rec.NotBefore.After(now.Add(maxCheckDeferral)) {
		matched, rec = false, CheckRecord{}
	}
	if matched && now.Before(rec.NotBefore) {
		return rec, ErrCheckDeferred
	}
	if matched && !rec.CheckedAt.IsZero() && !now.Before(rec.CheckedAt) && now.Sub(rec.CheckedAt) < maxAge {
		return rec, nil
	}
	avail, cerr := c.checkPrepared(withRunMark(ctx), req)
	if cerr == nil {
		fresh := CheckRecord{Request: key, Availability: avail, CheckedAt: now}
		if serr := store.Save(ctx, fresh); serr != nil {
			return fresh, fmt.Errorf("selfupdate: save check record: %w", serr)
		}
		return fresh, nil
	}
	var rl *RateLimitError
	if errors.As(cerr, &rl) {
		saved := rec
		saved.Request = key
		saved.NotBefore = notBefore(now, rl)
		if serr := store.Save(ctx, saved); serr != nil {
			return saved, errors.Join(cerr, fmt.Errorf("selfupdate: save check record: %w", serr))
		}
		return saved, cerr
	}
	return rec, cerr
}

// Bounds on a rate limit's deferral. GitHub's guidance for a secondary
// limit without retry-after is to wait at least a minute; one bad header
// must not defer every check for longer than an hour (0010-MADR A4).
const (
	minCheckDeferral = time.Minute
	maxCheckDeferral = time.Hour
)

// notBefore is the later of the rate limit's reset and now plus its
// Retry-After, capped at maxCheckDeferral from now. When the headers give
// no time in the future (none at all, or a reset already past, from a
// clock ahead of the server's) it is minCheckDeferral from now. A time the
// server gives that is sooner than the minimum is honoured
// (0010-PLAN-v1-5-1, deviation D1).
func notBefore(now time.Time, rl *RateLimitError) time.Time {
	nb := rl.Reset
	if rl.RetryAfter > 0 {
		if ra := now.Add(rl.RetryAfter); ra.After(nb) {
			nb = ra
		}
	}
	if !nb.After(now) {
		nb = now.Add(minCheckDeferral)
	}
	if hi := now.Add(maxCheckDeferral); nb.After(hi) {
		nb = hi
	}
	return nb
}

// maxCheckRecord bounds a check record file.
const maxCheckRecord = 64 << 10

type fileCheckStore struct {
	path string
}

// NewFileCheckStore returns a CheckStore that keeps one JSON document at
// path, which must be absolute. A typical path is under os.UserCacheDir. A
// missing, unreadable or unrecognized file loads as ErrNoCheckRecord. Save
// writes a temporary file beside path, syncs it and renames it over path.
func NewFileCheckStore(path string) (CheckStore, error) {
	if path == "" || !filepath.IsAbs(path) {
		return nil, fmt.Errorf("selfupdate: check store path must be absolute")
	}
	return fileCheckStore{path: filepath.Clean(path)}, nil
}

type fileCheckPlatform struct {
	OS   string `json:"os"`
	Arch string `json:"arch"`
}

// fileCheckRecord is the on-disk schema, version 2: version 1 plus channel
// (0005-PLAN Step 3). Field order is the document's key order.
type fileCheckRecord struct {
	SchemaVersion   int               `json:"schema_version"`
	Product         string            `json:"product"`
	CurrentVersion  string            `json:"current_version"`
	CurrentBuild    string            `json:"current_build"`
	TargetVersion   string            `json:"target_version"`
	Platform        fileCheckPlatform `json:"platform"`
	Channel         string            `json:"channel"`
	Available       bool              `json:"available"`
	ForceRequired   bool              `json:"force_required"`
	Operation       string            `json:"operation"`
	SelectedVersion string            `json:"selected_version"`
	ReleaseURL      string            `json:"release_url"`
	AssetName       string            `json:"asset_name"`
	CheckedAt       string            `json:"checked_at"`
	NotBefore       string            `json:"not_before"`
}

// checkRecordSchema is written; oldestCheckRecordSchema is the oldest still
// read. A schema-1 document has no channel, so it reads as the stable one.
const (
	checkRecordSchema       = 2
	oldestCheckRecordSchema = 1
)

func (s fileCheckStore) Load(context.Context) (CheckRecord, error) {
	f, err := os.Open(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return CheckRecord{}, ErrNoCheckRecord
	}
	if err != nil {
		return CheckRecord{}, fmt.Errorf("selfupdate: open check record: %w: %w", err, ErrNoCheckRecord)
	}
	data, rerr := readBounded(f, maxCheckRecord)
	if err := errors.Join(rerr, f.Close()); err != nil {
		return CheckRecord{}, fmt.Errorf("selfupdate: read check record: %w: %w", err, ErrNoCheckRecord)
	}
	var doc fileCheckRecord
	if err := json.Unmarshal(data, &doc); err != nil {
		return CheckRecord{}, fmt.Errorf("selfupdate: malformed check record: %w: %w", err, ErrNoCheckRecord)
	}
	if doc.SchemaVersion < oldestCheckRecordSchema || doc.SchemaVersion > checkRecordSchema {
		return CheckRecord{}, fmt.Errorf("selfupdate: check record schema %d: %w", doc.SchemaVersion, ErrNoCheckRecord)
	}
	rec, err := doc.record()
	if err != nil {
		return CheckRecord{}, fmt.Errorf("selfupdate: malformed check record: %w: %w", err, ErrNoCheckRecord)
	}
	return rec, nil
}

func (s fileCheckStore) Save(_ context.Context, rec CheckRecord) (err error) {
	data, err := json.Marshal(newFileCheckRecord(rec))
	if err != nil {
		return err
	}
	data = append(data, '\n')
	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(s.path)+".tmp-*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer func() {
		if err != nil {
			err = joinRemove(err, name)
		}
	}()
	if _, err := tmp.Write(data); err != nil {
		return joinClose(err, tmp)
	}
	if err := tmp.Sync(); err != nil {
		return joinClose(err, tmp)
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(name, 0o600); err != nil {
		return err
	}
	// On Windows os.Rename replaces an existing file (MoveFileEx with
	// MOVEFILE_REPLACE_EXISTING), though not atomically; a torn file loads
	// as a miss.
	return os.Rename(name, s.path)
}

func newFileCheckRecord(rec CheckRecord) fileCheckRecord {
	return fileCheckRecord{
		SchemaVersion:   checkRecordSchema,
		Product:         rec.Request.Product,
		CurrentVersion:  rec.Request.CurrentVersion,
		CurrentBuild:    rec.Request.CurrentBuild.String(),
		TargetVersion:   rec.Request.TargetVersion,
		Platform:        fileCheckPlatform{OS: rec.Request.Platform.OS, Arch: rec.Request.Platform.Arch},
		Channel:         rec.Request.Channel,
		Available:       rec.Availability.Available,
		ForceRequired:   rec.Availability.ForceRequired,
		Operation:       rec.Availability.Operation.String(),
		SelectedVersion: rec.Availability.TargetVersion,
		ReleaseURL:      rec.Availability.ReleaseURL,
		AssetName:       rec.Availability.AssetName,
		CheckedAt:       formatRecordTime(rec.CheckedAt),
		NotBefore:       formatRecordTime(rec.NotBefore),
	}
}

func (d fileCheckRecord) record() (CheckRecord, error) {
	build, err := parseBuildKind(d.CurrentBuild)
	if err != nil {
		return CheckRecord{}, err
	}
	op, err := parseOperation(d.Operation)
	if err != nil {
		return CheckRecord{}, err
	}
	checked, err := parseRecordTime(d.CheckedAt)
	if err != nil {
		return CheckRecord{}, err
	}
	nb, err := parseRecordTime(d.NotBefore)
	if err != nil {
		return CheckRecord{}, err
	}
	req := CheckRequest{
		Product:        d.Product,
		CurrentVersion: d.CurrentVersion,
		CurrentBuild:   build,
		TargetVersion:  d.TargetVersion,
		Platform:       Platform{OS: d.Platform.OS, Arch: d.Platform.Arch},
		Channel:        d.Channel,
	}
	return CheckRecord{
		Request: req,
		Availability: Availability{
			Product:        d.Product,
			CurrentVersion: d.CurrentVersion,
			TargetVersion:  d.SelectedVersion,
			ReleaseURL:     d.ReleaseURL,
			AssetName:      d.AssetName,
			Operation:      op,
			Available:      d.Available,
			ForceRequired:  d.ForceRequired,
		},
		CheckedAt: checked,
		NotBefore: nb,
	}, nil
}

func formatRecordTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339Nano)
}

func parseRecordTime(s string) (time.Time, error) {
	if s == "" {
		return time.Time{}, nil
	}
	return time.Parse(time.RFC3339Nano, s)
}

func parseBuildKind(s string) (BuildKind, error) {
	for _, k := range []BuildKind{BuildUnknown, ReleaseBuild, LocalBuild} {
		if k.String() == s {
			return k, nil
		}
	}
	return BuildUnknown, fmt.Errorf("unknown build kind %q", s)
}

func parseOperation(s string) (Operation, error) {
	for _, o := range []Operation{OperationNone, OperationUpgrade, OperationReinstall, OperationRollback, OperationReplaceLocal} {
		if o.String() == s {
			return o, nil
		}
	}
	return OperationNone, fmt.Errorf("unknown operation %q", s)
}
