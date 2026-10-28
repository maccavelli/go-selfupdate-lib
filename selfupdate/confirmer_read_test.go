package selfupdate

import (
	"context"
	"io"
	"os"
	"testing"
	"time"
)

var upgradePrompt = Prompt{Product: "demo", Current: "v1.0.0", Target: "v1.1.0", Operation: OperationUpgrade}

// ttyPipe is a pipe, which a confirmer given alwaysTTY treats as a
// terminal.
// alwaysTTY is a terminal check that says yes: a test's pipe stands in for
// a terminal without changing any package state
// (docs/decisions/0038-MADR-pinned-plans-lifecycle-conformance-and-bubble-tests.md
// D3-A).
func alwaysTTY(int) bool { return true }

func ttyPipe(t *testing.T) (r, w *os.File) {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = r.Close()
		_ = w.Close()
	})
	return r, w
}

// TestConfirmLeavesHostInput: an answered Confirm reads its line and no
// further, so the host program reads its own next input (0004-MADR R2).
func TestConfirmLeavesHostInput(t *testing.T) {
	defer checkNoLeak(t)()
	r, w := ttyPipe(t)
	if _, err := io.WriteString(w, "y\nhost-command\n"); err != nil {
		t.Fatal(err)
	}
	ok, err := terminalConfirmerFrom(r, io.Discard, alwaysTTY).Confirm(context.Background(), upgradePrompt)
	if err != nil || !ok {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
	got := make(chan string, 1)
	go func() {
		buf := make([]byte, 64)
		n, _ := r.Read(buf)
		got <- string(buf[:n])
	}()
	select {
	case s := <-got:
		if s != "host-command\n" {
			t.Fatalf("host read %q, want its own line", s)
		}
	case <-time.After(2 * time.Second):
		_ = w.Close() // release the blocked read
		t.Fatal("the host's input was consumed by the confirmer")
	}
}

// TestConfirmersShareInput: a second confirmer on the same input receives
// its own answer; the first leaves nothing reading (0004-MADR R2).
func TestConfirmersShareInput(t *testing.T) {
	defer checkNoLeak(t)()
	r, w := ttyPipe(t)
	if _, err := io.WriteString(w, "n\n"); err != nil {
		t.Fatal(err)
	}
	if ok, err := terminalConfirmerFrom(r, io.Discard, alwaysTTY).Confirm(context.Background(), upgradePrompt); err != nil || ok {
		t.Fatalf("first: ok=%v err=%v", ok, err)
	}
	// Not an ordering handshake: the pause only gives a reader wrongly left
	// behind by the first confirmer time to block on the pipe, so the
	// failure it causes is observed rather than raced.
	time.Sleep(100 * time.Millisecond)
	if _, err := io.WriteString(w, "y\n"); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	ok, err := terminalConfirmerFrom(r, io.Discard, alwaysTTY).Confirm(ctx, upgradePrompt)
	if err != nil || !ok {
		t.Fatalf("second: ok=%v err=%v, want its own \"y\"", ok, err)
	}
}

// TestConfirmLeaksNoReader: once a Confirm is answered, no goroutine is
// left reading the input (0004-MADR R2).
func TestConfirmLeaksNoReader(t *testing.T) {
	r, w := ttyPipe(t)
	// Checked before ttyPipe's cleanup closes the pipe, which would release
	// a leaked reader.
	defer checkNoLeak(t)()
	if _, err := io.WriteString(w, "y\n"); err != nil {
		t.Fatal(err)
	}
	if ok, err := terminalConfirmerFrom(r, io.Discard, alwaysTTY).Confirm(context.Background(), upgradePrompt); err != nil || !ok {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
}
