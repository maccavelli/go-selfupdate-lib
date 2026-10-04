package selfupdate

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// Running the new binary before and after it is installed
// (docs/decisions/0004-MADR-evolve-selfupdate-api-and-tui-support.md G9,
// amendment A2).

// ProbePhase says which copy of the binary a probe runs.
type ProbePhase uint8

const (
	// ProbeStaged runs the verified staging file, before anything is
	// replaced.
	ProbeStaged ProbePhase = iota + 1
	// ProbeInstalled runs the installed binary, before the replacement is
	// committed; a failure rolls it back.
	ProbeInstalled
)

// String implements fmt.Stringer.
func (p ProbePhase) String() string {
	switch p {
	case ProbeStaged:
		return "staged"
	case ProbeInstalled:
		return "installed"
	default:
		return "probephase(" + itoa(uint64(p)) + ")"
	}
}

// ProbeRequest describes one probe run.
type ProbeRequest struct {
	// Product is the requested product name.
	Product string
	// TargetVersion is the release tag being installed.
	TargetVersion string
	// Path is the absolute path of the binary to run.
	Path string
	// Phase says which copy Path is.
	Phase ProbePhase
}

// Prober runs a binary to prove it works.
type Prober interface {
	Probe(context.Context, ProbeRequest) error
}

// ProberFunc adapts a function to Prober.
type ProberFunc func(context.Context, ProbeRequest) error

// Probe implements Prober.
func (f ProberFunc) Probe(ctx context.Context, r ProbeRequest) error {
	return f(ctx, r)
}

// maxProbeOutput bounds the stdout a version probe keeps.
const maxProbeOutput = 64 << 10

type versionProber struct {
	args    []string
	want    func(string) string
	timeout time.Duration
}

// NewVersionProber returns a Prober that runs the binary with args, with no
// stdin, stderr discarded and the environment inherited, and requires it to
// exit 0 within timeout with stdout containing want(TargetVersion). A nil
// want is the identity: the output must contain the tag itself.
func NewVersionProber(args []string, want func(tag string) string, timeout time.Duration) (Prober, error) {
	if len(args) == 0 {
		return nil, fmt.Errorf("selfupdate: version prober needs arguments")
	}
	if timeout <= 0 {
		return nil, fmt.Errorf("selfupdate: version prober timeout must be positive")
	}
	if want == nil {
		want = func(tag string) string { return tag }
	}
	return versionProber{args: append([]string(nil), args...), want: want, timeout: timeout}, nil
}

func (p versionProber) Probe(ctx context.Context, r ProbeRequest) error {
	ctx, cancel := context.WithTimeout(ctx, p.timeout)
	defer cancel()
	out := &cappedBuffer{limit: maxProbeOutput}
	// Running the staged or installed binary is the probe's purpose; the
	// path is the library's own staging or target file (0004-MADR G9).
	cmd := exec.CommandContext(ctx, r.Path, p.args...) //nolint:gosec // G204: executes the release under test by design
	cmd.Stdout = out
	cmd.WaitDelay = time.Second
	err := cmd.Run()
	if ctxErr := ctx.Err(); errors.Is(ctxErr, context.DeadlineExceeded) {
		return fmt.Errorf("selfupdate: %s probe of %s timed out after %s", r.Phase, sanitizeText(r.Product), p.timeout)
	}
	if err != nil {
		return fmt.Errorf("selfupdate: %s probe of %s failed: %w", r.Phase, sanitizeText(r.Product), err)
	}
	if want := p.want(r.TargetVersion); !strings.Contains(out.String(), want) {
		return fmt.Errorf("selfupdate: %s probe of %s printed %q, want %q",
			r.Phase, sanitizeText(r.Product), firstLine(out.String()), sanitizeText(want))
	}
	return nil
}

// cappedBuffer keeps the first limit bytes and discards the rest, without
// failing the writer. The buffer is a named field, not embedded: an
// embedded bytes.Buffer would lend it ReadFrom, which io.Copy prefers, and
// the cap would never apply (0010-MADR B1).
type cappedBuffer struct {
	buf   bytes.Buffer
	limit int
}

func (b *cappedBuffer) Write(p []byte) (int, error) {
	if room := b.limit - b.buf.Len(); room > 0 {
		if len(p) > room {
			b.buf.Write(p[:room])
		} else {
			b.buf.Write(p)
		}
	}
	return len(p), nil
}

// Len is the number of bytes kept.
func (b *cappedBuffer) Len() int { return b.buf.Len() }

// String is the kept output.
func (b *cappedBuffer) String() string { return b.buf.String() }

// firstLine is the probe output's first line, sanitized and at most 200
// bytes.
func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	if len(s) > 200 {
		s = s[:200]
	}
	return sanitizeText(s)
}
