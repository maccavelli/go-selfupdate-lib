package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/maccavelli/go-selfupdate-lib/selfupdate"
)

// The environment a detached run carries (0011-MADR §9). A process with
// EnvHandOff set never hands off again.
const (
	// EnvHandOff holds the handoff's ID.
	EnvHandOff = "SELFUPDATE_HANDOFF"
	// EnvHandOffResult holds the path the detached run writes its
	// HandOffResult to.
	EnvHandOffResult = "SELFUPDATE_HANDOFF_RESULT"
)

// A Detacher is a backend that can run an update outside its service's
// kill scope (0011-MADR §9).
type Detacher interface {
	// Inside reports whether this process would die with the service.
	Inside(ctx context.Context) (bool, error)
	// Detach starts spec outside the service's kill scope, and returns
	// once it has started. spec is complete: HandOffIfInside fills it.
	Detach(ctx context.Context, spec HandOff) (Detached, error)
}

// HandOff is the command a detached run executes: the same program, with
// the same arguments, outside the service.
type HandOff struct {
	// ID names the handoff. Empty means a random one.
	ID string
	// Executable is the program to run. Empty means this process's own.
	Executable string
	// Args are the update command's arguments, such as os.Args[1:].
	Args []string
	// Env is added to this process's environment, after EnvHandOff and
	// EnvHandOffResult.
	Env []string
	// ResultPath is where the detached run writes its HandOffResult. Empty
	// means .<base>.selfupdate.handoff beside Executable.
	ResultPath string
}

// Detached describes a started handoff.
type Detached struct {
	// ID is the handoff's ID.
	ID string `json:"id"`
	// Where names what runs it: a transient unit, a job label, a process
	// ID.
	Where string `json:"where"`
	// ResultPath is where its HandOffResult will be.
	ResultPath string `json:"result_path"`
}

// HandOffResultSchema is the HandOffResult version this package writes.
const HandOffResultSchema = 1

// HandOffResult is what a detached run writes when its update ends.
type HandOffResult struct {
	SchemaVersion int                       `json:"schema_version"`
	ID            string                    `json:"id"`
	StartedAt     time.Time                 `json:"started_at"`
	FinishedAt    time.Time                 `json:"finished_at"`
	ExitCode      int                       `json:"exit_code"`
	Error         string                    `json:"error,omitempty"`
	Result        selfupdate.ResultDocument `json:"result"`
}

// HandOffIfInside asks d whether this process is inside the service, and
// when it is, completes spec and detaches it. handedOff false means the
// caller runs the update itself. An error from Inside is returned, not
// read as "outside": proceeding could stop the caller's own service. In a
// detached run, which has EnvHandOff set, it never hands off again.
func HandOffIfInside(ctx context.Context, d Detacher, spec HandOff) (Detached, bool, error) {
	if os.Getenv(EnvHandOff) != "" {
		return Detached{}, false, nil
	}
	if d == nil {
		return Detached{}, false, errors.New("selfupdate: service: detacher is required")
	}
	inside, err := d.Inside(ctx)
	if err != nil {
		return Detached{}, false, fmt.Errorf("selfupdate: service: cannot tell whether this process runs inside the service: %w", err)
	}
	if !inside {
		return Detached{}, false, nil
	}
	spec, err = complete(spec)
	if err != nil {
		return Detached{}, false, err
	}
	det, err := d.Detach(ctx, spec)
	if err != nil {
		return Detached{}, false, fmt.Errorf("selfupdate: service: hand the update off: %w", err)
	}
	return det, true, nil
}

// complete fills spec's defaults and adds the handoff environment.
func complete(spec HandOff) (HandOff, error) {
	exe, err := absExecutable(spec.Executable)
	if err != nil {
		return HandOff{}, fmt.Errorf("selfupdate: service: locate the executable: %w", err)
	}
	spec.Executable = exe
	if spec.ID == "" {
		var b [8]byte
		if _, err := rand.Read(b[:]); err != nil {
			return HandOff{}, err
		}
		spec.ID = hex.EncodeToString(b[:])
	}
	if strings.ContainsAny(spec.ID, "/\\ \t\n") {
		return HandOff{}, fmt.Errorf("selfupdate: service: handoff ID %q is not a plain name", spec.ID)
	}
	if spec.ResultPath == "" {
		spec.ResultPath = DefaultResultPath(exe)
	}
	if !filepath.IsAbs(spec.ResultPath) {
		return HandOff{}, fmt.Errorf("selfupdate: service: handoff result path %q is not absolute", spec.ResultPath)
	}
	spec.ResultPath = filepath.Clean(spec.ResultPath)
	spec.Args = append([]string(nil), spec.Args...)
	spec.Env = append([]string{EnvHandOff + "=" + spec.ID, EnvHandOffResult + "=" + spec.ResultPath}, spec.Env...)
	return spec, nil
}

// DefaultResultPath is .<base>.selfupdate.handoff beside executable: the
// directory an update can already write.
func DefaultResultPath(executable string) string {
	return filepath.Join(filepath.Dir(executable), "."+filepath.Base(executable)+".selfupdate.handoff")
}

// HandOffFunc returns the cli.HandOff Detach hook for d and spec: it hands
// off when this process is inside the service, and its detail names the
// handoff and its result file. The detached run cannot ask for
// confirmation, so inside the service a request without Yes is refused
// with ErrConfirmationRequired (0015-MADR C3).
func HandOffFunc(d Detacher, spec HandOff) func(context.Context, selfupdate.Request) (bool, string, error) {
	return func(ctx context.Context, req selfupdate.Request) (bool, string, error) {
		if !req.Yes && d != nil && os.Getenv(EnvHandOff) == "" {
			inside, err := d.Inside(ctx)
			if err != nil {
				return false, "", fmt.Errorf("selfupdate: service: cannot tell whether this process runs inside the service: %w", err)
			}
			if inside {
				return false, "", fmt.Errorf("selfupdate: service: an update from inside the service runs detached and cannot ask; pass --yes: %w",
					selfupdate.ErrConfirmationRequired)
			}
		}
		det, handedOff, err := HandOffIfInside(ctx, d, spec)
		if err != nil || !handedOff {
			return false, "", err
		}
		return true, fmt.Sprintf("%s (%s); result in %s", det.ID, det.Where, det.ResultPath), nil
	}
}

// ReportFunc returns the cli.HandOff Report hook. In a detached run it
// writes the update's HandOffResult to EnvHandOffResult's path; in any
// other run it does nothing. Call it at start-up: the time it is called is
// the result's StartedAt.
//
// It first applies a private environment file, LoadHandOffEnv. A failure to
// load it is reported in the result, and returned, with the run's outcome.
func ReportFunc() func(selfupdate.Result, error) error {
	started := time.Now().UTC()
	loadErr := LoadHandOffEnv()
	return func(res selfupdate.Result, runErr error) error {
		id, path := os.Getenv(EnvHandOff), os.Getenv(EnvHandOffResult)
		if id == "" || path == "" {
			return loadErr
		}
		runErr = errors.Join(loadErr, runErr)
		r := HandOffResult{
			SchemaVersion: HandOffResultSchema,
			ID:            id,
			StartedAt:     started,
			FinishedAt:    time.Now().UTC(),
			ExitCode:      selfupdate.ExitCode(res, runErr),
			Result:        res.Document(),
		}
		if runErr != nil {
			r.Error = runErr.Error()
		}
		return errors.Join(loadErr, WriteHandOffResult(path, r))
	}
}

// WriteHandOffResult writes r to path by temporary file and rename, so a
// reader never sees a partial result. The file is readable by others: a
// result holds no secret, and the caller that handed off may run as
// another user than the detached run.
//
// path must be absolute and clean: in a detached run it comes from
// EnvHandOffResult, which the process that handed off set.
func WriteHandOffResult(path string, r HandOffResult) error {
	if !filepath.IsAbs(path) || filepath.Clean(path) != path {
		return fmt.Errorf("selfupdate: service: handoff result path %q is not absolute and clean", path)
	}
	if r.SchemaVersion == 0 {
		r.SchemaVersion = HandOffResultSchema
	}
	body, err := json.Marshal(r)
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".tmp-*") //nolint:gosec // path is absolute and clean, checked above
	if err != nil {
		return err
	}
	name := tmp.Name()
	discard := func(err error) error {
		return errors.Join(err, os.Remove(name)) //nolint:gosec // name is the temporary file made beside the checked path
	}
	if err := writeAndClose(tmp, append(body, '\n')); err != nil {
		return discard(err)
	}
	if err := os.Chmod(name, 0o644); err != nil { //nolint:gosec // a handoff result holds no secret, and another user may read it
		return discard(err)
	}
	if err := os.Rename(name, path); err != nil { //nolint:gosec // path is absolute and clean, checked above
		return discard(err)
	}
	return nil
}

// writeAndClose writes body to f, syncs it and closes it.
func writeAndClose(f *os.File, body []byte) error {
	if _, err := f.Write(body); err != nil {
		return errors.Join(err, f.Close())
	}
	if err := f.Sync(); err != nil {
		return errors.Join(err, f.Close())
	}
	return f.Close()
}

// maxHandOffResult bounds a result file.
const maxHandOffResult = 64 << 10

// ReadHandOffResult reads what a detached run wrote to path.
func ReadHandOffResult(path string) (HandOffResult, error) {
	f, err := os.Open(path) //nolint:gosec // the caller names its own result file
	if err != nil {
		return HandOffResult{}, err
	}
	body, rerr := io.ReadAll(io.LimitReader(f, maxHandOffResult+1))
	if err := errors.Join(rerr, f.Close()); err != nil {
		return HandOffResult{}, err
	}
	if len(body) > maxHandOffResult {
		return HandOffResult{}, errors.New("selfupdate: service: handoff result is too large")
	}
	var r HandOffResult
	if err := json.Unmarshal(body, &r); err != nil {
		return HandOffResult{}, fmt.Errorf("selfupdate: service: malformed handoff result: %w", err)
	}
	return r, nil
}

// ProcessDetacher is a Detacher for a lifecycle with no backend in this
// module, such as a Task Scheduler task: inside is the caller's own check,
// and Detach is DetachProcess.
func ProcessDetacher(inside func(context.Context) (bool, error)) Detacher {
	return processDetacher{inside: inside}
}

type processDetacher struct {
	inside func(context.Context) (bool, error)
}

func (p processDetacher) Inside(ctx context.Context) (bool, error) {
	if p.inside == nil {
		return false, errors.New("selfupdate: service: process detacher has no inside check")
	}
	return p.inside(ctx)
}

func (processDetacher) Detach(ctx context.Context, spec HandOff) (Detached, error) {
	return DetachProcess(ctx, spec)
}
