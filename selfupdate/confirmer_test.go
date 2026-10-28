package selfupdate

import (
	"context"
	"errors"
	"io"
	"os"
	"strings"
	"testing"
	"time"
)

func TestTerminalConfirmerRequiresTTY(t *testing.T) {
	defer checkNoLeak(t)()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	defer w.Close()
	c := NewTerminalConfirmer(r, io.Discard)
	_, err = c.Confirm(context.Background(), Prompt{Product: "demo", Current: "v1", Target: "v2", Operation: OperationUpgrade})
	if !errors.Is(err, ErrConfirmationRequired) {
		t.Fatalf("err = %v", err)
	}
}

func TestTerminalConfirmerYesNo(t *testing.T) {
	defer checkNoLeak(t)()

	t.Run("yes", func(t *testing.T) {
		in, out := pipeFile(t, "yes\n")
		var buf strings.Builder
		c := terminalConfirmerFrom(in, &buf, alwaysTTY)
		ok, err := c.Confirm(context.Background(), Prompt{Product: "demo", Current: "v1.0.0", Target: "v1.1.0", Operation: OperationUpgrade})
		if err != nil || !ok {
			t.Fatalf("ok=%v err=%v", ok, err)
		}
		if !strings.Contains(buf.String(), "upgrade") {
			t.Fatalf("prompt = %q", buf.String())
		}
		_ = out
	})
	t.Run("no", func(t *testing.T) {
		in, _ := pipeFile(t, "n\n")
		c := terminalConfirmerFrom(in, io.Discard, alwaysTTY)
		ok, err := c.Confirm(context.Background(), Prompt{Product: "demo", Current: "v1.0.0", Target: "v1.1.0", Operation: OperationUpgrade})
		if err != nil || ok {
			t.Fatalf("ok=%v err=%v", ok, err)
		}
	})
}

// TestTerminalConfirmerEOFDeclines: end of input before an answer is the
// default "N", not an error (0003-MADR C6).
func TestTerminalConfirmerEOFDeclines(t *testing.T) {
	defer checkNoLeak(t)()
	in, _ := pipeFile(t, "")
	c := terminalConfirmerFrom(in, io.Discard, alwaysTTY)
	ok, err := c.Confirm(context.Background(), Prompt{Product: "demo", Current: "v1.0.0", Target: "v1.1.0", Operation: OperationUpgrade})
	if err != nil || ok {
		t.Fatalf("ok=%v err=%v, want a decline", ok, err)
	}
}

// TestTerminalConfirmerCancelKeepsLine: a cancelled Confirm does not lose
// the line typed afterwards; the next Confirm receives it (0003-MADR C7).
func TestTerminalConfirmerCancelKeepsLine(t *testing.T) {
	defer checkNoLeak(t)()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = r.Close()
		_ = w.Close()
	})
	prompted := &signalWriter{ch: make(chan struct{}, 4)}
	c := terminalConfirmerFrom(r, prompted, alwaysTTY)
	p := Prompt{Product: "demo", Current: "v1.0.0", Target: "v1.1.0", Operation: OperationUpgrade}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	// A context already cancelled returns before reading.
	if _, err := c.Confirm(ctx, p); !errors.Is(err, context.Canceled) {
		t.Fatalf("pre-cancelled err = %v", err)
	}
	// Cancel only once the prompt is out and the reader is blocked on input.
	ctx2, cancel2 := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := c.Confirm(ctx2, p)
		done <- err
	}()
	// Once the prompt is out, Confirm has started its read or is about to,
	// before it waits: cancelling at any point from here leaves that read
	// pending (0004-MADR R2), so no pause is needed to order the two.
	select {
	case <-prompted.ch:
	case <-time.After(5 * time.Second):
		t.Fatal("prompt never written")
	}
	cancel2()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled err = %v", err)
	}
	// The answer is typed while no Confirm is waiting. Give the reader left
	// behind by the cancelled Confirm time to consume it: before C7's fix
	// that reader swallowed the line and the next Confirm never saw it.
	if _, err := io.WriteString(w, "y\n"); err != nil {
		t.Fatal(err)
	}
	time.Sleep(200 * time.Millisecond)
	ctx3, cancel3 := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel3()
	ok, err := c.Confirm(ctx3, p)
	if err != nil || !ok {
		t.Fatalf("ok=%v err=%v, want the line typed after cancellation", ok, err)
	}
	// The answered Confirm consumed the pending read: nothing is left
	// reading the input (0004-MADR R2).
	c.(*terminalConfirmer).mu.Lock()
	pending := c.(*terminalConfirmer).pending
	c.(*terminalConfirmer).mu.Unlock()
	if pending != nil {
		t.Fatal("a read is still outstanding after the answered Confirm")
	}
}

// signalWriter discards writes and signals each one.
type signalWriter struct{ ch chan struct{} }

func (s *signalWriter) Write(b []byte) (int, error) {
	select {
	case s.ch <- struct{}{}:
	default:
	}
	return len(b), nil
}

func pipeFile(t *testing.T, input string) (in *os.File, w *os.File) {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = r.Close()
		_ = w.Close()
	})
	if _, err := io.WriteString(w, input); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return r, w
}
