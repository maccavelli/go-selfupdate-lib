package selfupdate

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
)

type jsonReporter struct {
	w io.Writer
}

// jsonEvent is one JSON Lines record. Field order is key order.
type jsonEvent struct {
	Kind    string `json:"kind"`
	Product string `json:"product,omitempty"`
	Current string `json:"current,omitempty"`
	Target  string `json:"target,omitempty"`
	Asset   string `json:"asset,omitempty"`
	Bytes   int64  `json:"bytes,omitempty"`
	Total   int64  `json:"total,omitempty"`
	Detail  string `json:"detail,omitempty"`
}

// NewJSONReporter writes each event as one JSON object followed by "\n"
// (JSON Lines: https://jsonlines.org/), in a single Write per event. The
// keys, in order, are kind, product, current, target, asset, bytes, total
// and detail; empty strings and zero numbers are omitted, except kind,
// which is EventKind.String(). Strings are sanitized as the text reporter
// sanitizes them. Unlike NewTextReporter it reports EventProgress
// (0004-MADR §3, "Structured output").
func NewJSONReporter(w io.Writer) Reporter {
	// A nil pointer behind the interface is nil (0015-MADR C7).
	if isNil(w) {
		w = nil
	}
	return jsonReporter{w: w}
}

func (r jsonReporter) Report(_ context.Context, ev Event) error {
	if r.w == nil {
		return fmt.Errorf("selfupdate: reporter writer is nil")
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(jsonEvent{
		Kind:    sanitizeText(ev.Kind.String()),
		Product: sanitizeText(ev.Product),
		Current: sanitizeText(ev.Current),
		Target:  sanitizeText(ev.Target),
		Asset:   sanitizeText(ev.Asset),
		Bytes:   ev.Bytes,
		Total:   ev.Total,
		Detail:  sanitizeText(ev.Detail),
	}); err != nil {
		return err
	}
	_, err := r.w.Write(buf.Bytes())
	return err
}
