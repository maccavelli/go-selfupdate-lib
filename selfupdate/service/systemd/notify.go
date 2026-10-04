package systemd

import (
	"errors"
	"net"
	"os"
	"strconv"
	"strings"
	"time"
)

// Notify sends states to the service manager by the sd_notify protocol: the
// lines, joined by newlines, as one datagram to $NOTIFY_SOCKET, a path or,
// when it begins with "@", an abstract socket (sd_notify(3); 0011-MADR §7,
// owner answer Q3). With $NOTIFY_SOCKET unset it returns false and no
// error, as sd_notify returns 0: the process is not a Type=notify service.
// It never unsets the variable.
func Notify(states ...string) (sent bool, err error) {
	socket := os.Getenv("NOTIFY_SOCKET")
	if socket == "" {
		return false, nil
	}
	if len(states) == 0 {
		return false, errors.New("selfupdate: systemd: notify needs a state")
	}
	for _, s := range states {
		if strings.Contains(s, "\n") {
			return false, errors.New("selfupdate: systemd: a notify state is one line")
		}
	}
	addr := &net.UnixAddr{Name: socket, Net: "unixgram"}
	if strings.HasPrefix(socket, "@") {
		addr.Name = "\x00" + socket[1:]
	}
	conn, err := net.DialUnix("unixgram", nil, addr)
	if err != nil {
		return false, err
	}
	_, werr := conn.Write([]byte(strings.Join(states, "\n")))
	return werr == nil, errors.Join(werr, conn.Close())
}

// Ready reports that start-up is complete: READY=1. Under Type=notify,
// systemctl start returns only now, and the unit becomes active.
func Ready() (bool, error) { return Notify("READY=1") }

// Reloading reports that a configuration reload began: RELOADING=1, with
// the monotonic time systemd requires since v253 (MONOTONIC_USEC). Send
// Ready when the reload is done.
func Reloading() (bool, error) {
	return Notify("RELOADING=1", "MONOTONIC_USEC="+strconv.FormatInt(monotonicMicros(), 10))
}

// Stopping reports that shutdown began: STOPPING=1.
func Stopping() (bool, error) { return Notify("STOPPING=1") }

// Watchdog keeps the watchdog from firing: WATCHDOG=1. Send it more often
// than WatchdogInterval.
func Watchdog() (bool, error) { return Notify("WATCHDOG=1") }

// Status sets the free-form status systemctl status shows: STATUS=text.
func Status(text string) (bool, error) { return Notify("STATUS=" + text) }

// WatchdogInterval reports the watchdog timeout systemd expects from this
// process, from $WATCHDOG_USEC, when $WATCHDOG_PID is unset or names this
// process. ok is false when there is no watchdog.
func WatchdogInterval() (d time.Duration, ok bool) {
	usec, err := strconv.ParseUint(os.Getenv("WATCHDOG_USEC"), 10, 63)
	if err != nil || usec == 0 {
		return 0, false
	}
	if pid := os.Getenv("WATCHDOG_PID"); pid != "" && pid != strconv.Itoa(os.Getpid()) {
		return 0, false
	}
	return time.Duration(usec) * time.Microsecond, true
}
