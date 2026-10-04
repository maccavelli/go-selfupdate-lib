//go:build unix

package systemd

import (
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"testing"
	"time"
)

// Tests for docs/decisions/0011-PLAN-reference-service-lifecycles.md V2
// step 7: Notify, over a test datagram socket.

// listen opens a unixgram socket at name ("@x" for an abstract one) and
// returns a function that reads one datagram.
func listen(t *testing.T, name string) func() string {
	t.Helper()
	addr := &net.UnixAddr{Name: name, Net: "unixgram"}
	if name[0] == '@' {
		addr.Name = "\x00" + name[1:]
	}
	conn, err := net.ListenUnixgram("unixgram", addr)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return func() string {
		t.Helper()
		buf := make([]byte, 4096)
		if err := conn.SetReadDeadline(time.Now().Add(5 * time.Second)); err != nil {
			t.Fatal(err)
		}
		n, err := conn.Read(buf)
		if err != nil {
			t.Fatal(err)
		}
		return string(buf[:n])
	}
}

func TestNotifyPathSocket(t *testing.T) {
	// A short path: a socket path is limited to about 100 bytes.
	dir, err := os.MkdirTemp("/tmp", "nfy")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	sock := filepath.Join(dir, "s")
	read := listen(t, sock)
	t.Setenv("NOTIFY_SOCKET", sock)
	for _, c := range []struct {
		send func() (bool, error)
		want string
	}{
		{Ready, "READY=1"},
		{Stopping, "STOPPING=1"},
		{Watchdog, "WATCHDOG=1"},
		{func() (bool, error) { return Status("syncing") }, "STATUS=syncing"},
		{func() (bool, error) { return Notify("READY=1", "STATUS=up") }, "READY=1\nSTATUS=up"},
	} {
		if sent, err := c.send(); err != nil || !sent {
			t.Fatalf("sent %t, err %v", sent, err)
		}
		if got := read(); got != c.want {
			t.Fatalf("datagram %q, want %q", got, c.want)
		}
	}
	if sent, err := Reloading(); err != nil || !sent {
		t.Fatalf("Reloading: %t, %v", sent, err)
	}
	if got := read(); len(got) < len("RELOADING=1\nMONOTONIC_USEC=") || got[:len("RELOADING=1\nMONOTONIC_USEC=")] != "RELOADING=1\nMONOTONIC_USEC=" {
		t.Fatalf("Reloading sent %q", got)
	}
}

func TestNotifyAbstractSocket(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("abstract sockets are Linux's")
	}
	name := "@selfupdate-notify-" + strconv.Itoa(os.Getpid())
	read := listen(t, name)
	t.Setenv("NOTIFY_SOCKET", name)
	if sent, err := Ready(); err != nil || !sent {
		t.Fatalf("sent %t, err %v", sent, err)
	}
	if got := read(); got != "READY=1" {
		t.Fatalf("datagram %q", got)
	}
}

func TestNotifyUnsetAndRefused(t *testing.T) {
	t.Setenv("NOTIFY_SOCKET", "")
	if sent, err := Ready(); err != nil || sent {
		t.Fatalf("unset: sent %t, err %v; want false and no error, as sd_notify", sent, err)
	}
	t.Setenv("NOTIFY_SOCKET", "/nonexistent/socket")
	if _, err := Notify("READY=1\nX=1"); err == nil {
		t.Fatal("a state with a newline was accepted")
	}
	if _, err := Notify(); err == nil {
		t.Fatal("no state was accepted")
	}
	if sent, err := Ready(); err == nil || sent {
		t.Fatalf("a missing socket: sent %t, err %v", sent, err)
	}
}

func TestWatchdogInterval(t *testing.T) {
	me := strconv.Itoa(os.Getpid())
	for _, c := range []struct {
		usec, pid string
		want      time.Duration
		ok        bool
	}{
		{"", "", 0, false},
		{"0", "", 0, false},
		{"30000000", "", 30 * time.Second, true},
		{"30000000", me, 30 * time.Second, true},
		{"30000000", "1", 0, false},
		{"junk", "", 0, false},
	} {
		t.Setenv("WATCHDOG_USEC", c.usec)
		t.Setenv("WATCHDOG_PID", c.pid)
		if d, ok := WatchdogInterval(); d != c.want || ok != c.ok {
			t.Errorf("WATCHDOG_USEC=%q WATCHDOG_PID=%q: %s %t; want %s %t", c.usec, c.pid, d, ok, c.want, c.ok)
		}
	}
}
