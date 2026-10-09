package selfupdate

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"
)

// Cached, rate-limit-aware checks for startup banners
// (docs/decisions/0004-MADR-evolve-selfupdate-api-and-tui-support.md §3).

// CheckOutcome is how a cached check ended: with an answer, or with an
// error that the same question would get again until a release changes
// (0010-MADR Q1). Its zero value is CheckAnswered, so a CheckStore written
// before outcomes existed returns answers.
type CheckOutcome uint8

const (
	// CheckAnswered means the check succeeded: Availability is the answer.
	CheckAnswered CheckOutcome = iota
	// CheckLatestOlder means the check failed with ErrLatestOlder.
	CheckLatestOlder
	// CheckUnsupportedPlatform means the check failed with
	// ErrUnsupportedPlatform: the release has no asset for the platform.
	CheckUnsupportedPlatform
	// CheckMutableRelease means the check failed with ErrMutableRelease.
	CheckMutableRelease
	// CheckNoRelease means the check failed with ErrNoRelease: the
	// repository has no such release yet (0015-MADR A1).
	CheckNoRelease
)

// checkOutcomes pairs each failed outcome with its error, and with the
// name a CheckStore may persist. Names match EventFailed's Detail classes.
var checkOutcomes = []struct {
	outcome CheckOutcome
	err     error
	name    string
}{
	{CheckAnswered, nil, "answered"},
	{CheckLatestOlder, ErrLatestOlder, "latest-older"},
	{CheckUnsupportedPlatform, ErrUnsupportedPlatform, "unsupported-platform"},
	{CheckMutableRelease, ErrMutableRelease, "mutable-release"},
	{CheckNoRelease, ErrNoRelease, "no-release"},
}

// String implements fmt.Stringer. A known outcome's name is stable, and
// safe to persist; an unknown value prints as CheckOutcome(N).
func (o CheckOutcome) String() string {
	if int(o) < len(checkOutcomes) {
		return checkOutcomes[o].name
	}
	return "CheckOutcome(" + strconv.Itoa(int(o)) + ")"
}

// Err returns the error a failed outcome stands for, which errors.Is
// matches against the exported sentinel. It is nil for CheckAnswered and
// for an unknown value.
func (o CheckOutcome) Err() error {
	if int(o) < len(checkOutcomes) {
		return checkOutcomes[o].err
	}
	return nil
}

// valid reports whether o is a known outcome.
func (o CheckOutcome) valid() bool {
	return int(o) < len(checkOutcomes)
}

// outcomeOf classifies a check's error. Only a deterministic error has an
// outcome; a rate limit, a cancellation or any other error does not.
func outcomeOf(err error) (CheckOutcome, bool) {
	if errors.Is(err, ErrRateLimited) || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return CheckAnswered, false
	}
	for _, c := range checkOutcomes[1:] {
		if errors.Is(err, c.err) {
			return c.outcome, true
		}
	}
	return CheckAnswered, false
}

func parseCheckOutcome(s string) (CheckOutcome, error) {
	for _, c := range checkOutcomes {
		if c.name == s {
			return c.outcome, nil
		}
	}
	return CheckAnswered, fmt.Errorf("unknown check outcome %q", s)
}

// CheckRecord is one cached answer and its back-off state.
type CheckRecord struct {
	// Request is the question, with Platform normalized (never zero).
	Request CheckRequest
	// Availability is the answer as of CheckedAt. It is meaningful only
	// when CheckedAt is non-zero. After a failed outcome it holds only
	// Product and CurrentVersion, and Available is false.
	Availability Availability
	// Outcome is how the check at CheckedAt ended. It is meaningful only
	// when CheckedAt is non-zero.
	Outcome CheckOutcome
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
// An answer is a success, or one of the errors the same question gets again
// until a release changes: ErrLatestOlder, ErrUnsupportedPlatform,
// ErrMutableRelease and ErrNoRelease. Each is saved with its CheckOutcome
// and served for maxAge; a cached one returns an error that matches its
// sentinel. Any other error is returned and not saved, so the next call
// checks again.
//
// While a saved NotBefore is in the future it returns the saved record and
// ErrCheckDeferred without touching the network. A rate-limited check saves
// a NotBefore taken from the error's RetryAfter, and its Reset when no
// requests remain, or one minute ahead when the response gave neither.
//
// Whatever the error, a returned record with a non-zero CheckedAt holds a
// real answer as of CheckedAt, and its Outcome says which.
func (c *Checker) CheckCached(ctx context.Context, cr CheckRequest, store CheckStore, maxAge time.Duration) (CheckRecord, error) {
	if !c.constructed() {
		return CheckRecord{}, ErrNotConstructed
	}
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
	// An outcome this package does not know is not one it wrote.
	matched := lerr == nil && rec.Request == key && rec.Outcome.valid()
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
		if err := rec.Outcome.Err(); err != nil {
			return rec, wrapRun(req, fmt.Errorf("cached check from %s: %w", formatRecordTime(rec.CheckedAt), err))
		}
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
	if outcome, ok := outcomeOf(cerr); ok {
		fresh := CheckRecord{
			Request:      key,
			Availability: Availability{Product: req.Product, CurrentVersion: req.CurrentVersion},
			Outcome:      outcome,
			CheckedAt:    now,
		}
		if serr := store.Save(ctx, fresh); serr != nil {
			return fresh, errors.Join(cerr, fmt.Errorf("selfupdate: save check record: %w", serr))
		}
		return fresh, cerr
	}
	if rl, ok := errors.AsType[*RateLimitError](cerr); ok {
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

// notBefore is the later of the rate limit's reset, when no requests
// remain, and now plus its Retry-After, capped at maxCheckDeferral from
// now. A secondary limit with quota left waits its Retry-After, not the
// primary window's reset (0015-MADR A5). When the headers give
// no time in the future (none at all, or a reset already past, from a
// clock ahead of the server's) it is minCheckDeferral from now. A time the
// server gives that is sooner than the minimum is honoured
// (0010-PLAN-v1-5-1, deviation D1).
func notBefore(now time.Time, rl *RateLimitError) time.Time {
	var nb time.Time
	if rl.Remaining == 0 {
		nb = rl.Reset
	}
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

// fileCheckRecord is the on-disk schema, version 3: version 2 plus outcome
// (0010-PLAN-v1-6-0 S1). Version 2 added channel (0005-PLAN Step 3). Field
// order is the document's key order.
type fileCheckRecord struct {
	SchemaVersion   int               `json:"schema_version"`
	Product         string            `json:"product"`
	CurrentVersion  string            `json:"current_version"`
	CurrentBuild    string            `json:"current_build"`
	TargetVersion   string            `json:"target_version"`
	Platform        fileCheckPlatform `json:"platform"`
	Channel         string            `json:"channel"`
	Outcome         string            `json:"outcome"`
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
// read. An older document is a miss: it costs one check, and is never an
// error (0010-PLAN-v1-6-0 S1).
const (
	checkRecordSchema       = 3
	oldestCheckRecordSchema = 3
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
		Outcome:         rec.Outcome.String(),
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
	outcome, err := parseCheckOutcome(d.Outcome)
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
		Outcome:   outcome,
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
