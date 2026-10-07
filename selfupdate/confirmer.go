package selfupdate

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"

	"golang.org/x/term"
)

var isTerminal = term.IsTerminal

// terminalConfirmer is the line confirmer behind both NewTerminalConfirmer
// and NewPromptConfirmer; allow decides, at each Confirm, whether it may
// prompt at all.
type terminalConfirmer struct {
	in    io.Reader
	out   io.Writer
	allow func() error

	// pending is the one outstanding read, if any. A Confirm starts a read
	// only when none is outstanding, and the read ends after one line, so
	// an answered Confirm leaves nothing reading the input (0004-MADR R2).
	// A Confirm cancelled mid-read leaves its read pending, and the next
	// Confirm receives that line rather than losing it (0003-MADR C7).
	mu      sync.Mutex
	pending chan lineResult
}

type lineResult struct {
	line string
	err  error
}

// NewTerminalConfirmer prompts on out and reads from in. A non-terminal input
// returns ErrConfirmationRequired instead of hanging. End of input before an
// answer is a decline. A cancelled Confirm returns the context error; a line
// typed afterwards answers the next Confirm on the same confirmer.
func NewTerminalConfirmer(in *os.File, out io.Writer) Confirmer {
	c := &terminalConfirmer{}
	// A nil pointer behind the interface is nil (0015-MADR C7).
	if !isNil(out) {
		c.out = out
	}
	// A nil *os.File must not become a non-nil io.Reader.
	if in != nil {
		c.in = in
	}
	c.allow = func() error {
		if in == nil {
			return fmt.Errorf("selfupdate: confirmation input is nil: %w", ErrConfirmationRequired)
		}
		if !isTerminal(int(in.Fd())) {
			return fmt.Errorf("selfupdate: pass --yes to apply without a TTY: %w", ErrConfirmationRequired)
		}
		return nil
	}
	return c
}

// NewPromptConfirmer prompts on out and reads one answer line from in,
// for any reader: a pipe, a test buffer, or a UI's input. interactive
// false, or a nil in, returns ErrConfirmationRequired without prompting.
// It reads byte by byte and leaves no read outstanding once answered, and
// a line typed after a cancelled Confirm answers the next one, exactly as
// NewTerminalConfirmer does (0004-MADR G6).
func NewPromptConfirmer(in io.Reader, out io.Writer, interactive bool) Confirmer {
	c := &terminalConfirmer{}
	// A nil pointer behind the interface is nil (0015-MADR C7).
	if !isNil(out) {
		c.out = out
	}
	if !isNil(in) {
		c.in = in
	}
	c.allow = func() error {
		if c.in == nil {
			return fmt.Errorf("selfupdate: confirmation input is nil: %w", ErrConfirmationRequired)
		}
		if !interactive {
			return fmt.Errorf("selfupdate: pass --yes to apply without prompting: %w", ErrConfirmationRequired)
		}
		return nil
	}
	return c
}

// maxAnswer bounds the answer text kept from one line; the rest of an
// over-long line is read and discarded.
const maxAnswer = 4096

// readLine reads one line from r a byte at a time. A buffered reader
// would read past the newline and take input the host program reads
// next (0004-MADR R2).
func readLine(r io.Reader) lineResult {
	var line []byte
	var b [1]byte
	for {
		n, err := r.Read(b[:])
		if n == 1 {
			if b[0] == '\n' {
				return lineResult{line: strings.TrimSuffix(string(line), "\r")}
			}
			if len(line) < maxAnswer {
				line = append(line, b[0])
			}
		}
		if err != nil {
			if errors.Is(err, io.EOF) && len(line) > 0 {
				return lineResult{line: string(line)}
			}
			return lineResult{err: err}
		}
	}
}

// nextLine returns the outstanding read, starting one when there is none.
func (c *terminalConfirmer) nextLine() chan lineResult {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.pending == nil {
		ch := make(chan lineResult, 1)
		go func() { ch <- readLine(c.in) }()
		c.pending = ch
	}
	return c.pending
}

func (c *terminalConfirmer) consumed() {
	c.mu.Lock()
	c.pending = nil
	c.mu.Unlock()
}

func (c *terminalConfirmer) Confirm(ctx context.Context, p Prompt) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if err := c.allow(); err != nil {
		return false, err
	}
	if c.out == nil {
		return false, fmt.Errorf("selfupdate: confirmation output is nil")
	}
	prompt := fmt.Sprintf("selfupdate: %s %s from %s to %s? [y/N] ",
		sanitizeText(p.Operation.String()),
		sanitizeText(p.Product),
		sanitizeText(p.Current),
		sanitizeText(p.Target),
	)
	if _, err := io.WriteString(c.out, prompt); err != nil {
		return false, err
	}
	lines := c.nextLine()
	select {
	case <-ctx.Done():
		return false, ctx.Err()
	case r := <-lines:
		c.consumed()
		if errors.Is(r.err, io.EOF) {
			// End of input before an answer is the default: decline.
			return false, nil
		}
		if r.err != nil {
			return false, r.err
		}
		switch strings.ToLower(strings.TrimSpace(r.line)) {
		case "y", "yes":
			return true, nil
		default:
			return false, nil
		}
	}
}
