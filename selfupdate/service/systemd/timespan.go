package systemd

import (
	"strconv"
	"strings"
	"time"
)

// timespanUnits are the units systemctl show prints a time span in, such as
// "1min 30s" for TimeoutStopUSec (systemd.time(7)).
var timespanUnits = map[string]time.Duration{
	"us":  time.Microsecond,
	"ms":  time.Millisecond,
	"s":   time.Second,
	"min": time.Minute,
	"h":   time.Hour,
	"d":   24 * time.Hour,
}

// parseTimespan reads a time span as systemctl show prints it: space-separated
// <integer><unit> parts. "infinity", an empty value, or anything else is not
// a span (0015-MADR B3).
func parseTimespan(s string) (time.Duration, bool) {
	fields := strings.Fields(s)
	if len(fields) == 0 {
		return 0, false
	}
	var total time.Duration
	for _, f := range fields {
		i := strings.IndexFunc(f, func(r rune) bool { return r < '0' || r > '9' })
		if i <= 0 {
			return 0, false
		}
		n, err := strconv.ParseInt(f[:i], 10, 64)
		unit, ok := timespanUnits[f[i:]]
		if err != nil || !ok || n > int64(time.Duration(1<<62)/unit) {
			return 0, false
		}
		total += time.Duration(n) * unit
	}
	return total, true
}
