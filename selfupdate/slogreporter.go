package selfupdate

import (
	"context"
	"fmt"
	"log/slog"
)

type slogReporter struct {
	l *slog.Logger
}

// NewSlogReporter logs each event to l, with the event kind as the message
// and its fields as attributes: product, current, target, asset, bytes,
// total and detail, each left out when empty or zero, and each string
// sanitized as the text reporter's are. EventFailed logs at Error,
// EventRolledBack and EventWarning at Warn, EventProgress at Debug, and
// every other kind at Info. A nil l makes Report fail, as a nil writer does
// for NewTextReporter
// (docs/decisions/0025-MADR-align-with-go-sdk-design-practices.md D12).
func NewSlogReporter(l *slog.Logger) Reporter {
	return slogReporter{l: l}
}

func (r slogReporter) Report(ctx context.Context, ev Event) error {
	if r.l == nil {
		return fmt.Errorf("selfupdate: slog reporter logger is nil")
	}
	var attrs []slog.Attr
	for _, f := range []struct{ key, value string }{
		{"product", ev.Product}, {"current", ev.Current}, {"target", ev.Target}, {"asset", ev.Asset},
	} {
		if f.value != "" {
			attrs = append(attrs, slog.String(f.key, sanitizeText(f.value)))
		}
	}
	if ev.Bytes != 0 {
		attrs = append(attrs, slog.Int64("bytes", ev.Bytes))
	}
	if ev.Total != 0 {
		attrs = append(attrs, slog.Int64("total", ev.Total))
	}
	if ev.Detail != "" {
		attrs = append(attrs, slog.String("detail", sanitizeText(ev.Detail)))
	}
	r.l.LogAttrs(ctx, slogLevel(ev.Kind), ev.Kind.String(), attrs...)
	return nil
}

// slogLevel is the level an event kind is logged at.
func slogLevel(k EventKind) slog.Level {
	switch k {
	case EventFailed:
		return slog.LevelError
	case EventRolledBack, EventWarning:
		return slog.LevelWarn
	case EventProgress:
		return slog.LevelDebug
	}
	return slog.LevelInfo
}
