package selfupdate

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"
)

// Tests for NewSlogReporter (docs/decisions/0025-MADR-align-with-go-sdk-design-practices.md
// D12).

func TestSlogReporterLevelsAndAttrs(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	r := NewSlogReporter(slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})))
	for _, ev := range []Event{
		{Kind: EventSelected, Product: "demo", Current: "v1.0.0", Target: "v1.1.0", Asset: "demo-linux-amd64"},
		{Kind: EventProgress, Product: "demo", Bytes: 10, Total: 20},
		{Kind: EventWarning, Product: "demo", Detail: "post-commit"},
		{Kind: EventRolledBack, Product: "demo"},
		{Kind: EventFailed, Product: "demo", Detail: "integrity\x1b[31m"},
	} {
		if err := r.Report(context.Background(), ev); err != nil {
			t.Fatal(err)
		}
	}
	want := []struct {
		level, msg string
		attrs      map[string]any
	}{
		{"INFO", EventSelected.String(), map[string]any{"product": "demo", "current": "v1.0.0", "target": "v1.1.0", "asset": "demo-linux-amd64"}},
		{"DEBUG", EventProgress.String(), map[string]any{"product": "demo", "bytes": float64(10), "total": float64(20)}},
		{"WARN", EventWarning.String(), map[string]any{"product": "demo", "detail": "post-commit"}},
		{"WARN", EventRolledBack.String(), map[string]any{"product": "demo"}},
		{"ERROR", EventFailed.String(), map[string]any{"product": "demo", "detail": sanitizeText("integrity\x1b[31m")}},
	}
	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) != len(want) {
		t.Fatalf("%d lines, want %d:\n%s", len(lines), len(want), buf.String())
	}
	for i, line := range lines {
		var got map[string]any
		if err := json.Unmarshal([]byte(line), &got); err != nil {
			t.Fatal(err)
		}
		if got["level"] != want[i].level || got["msg"] != want[i].msg {
			t.Errorf("line %d: level %v msg %v, want %s %s", i, got["level"], got["msg"], want[i].level, want[i].msg)
		}
		delete(got, "time")
		delete(got, "level")
		delete(got, "msg")
		if len(got) != len(want[i].attrs) {
			t.Errorf("line %d: attrs %v, want %v", i, got, want[i].attrs)
		}
		for k, v := range want[i].attrs {
			if got[k] != v {
				t.Errorf("line %d: %s = %v, want %v", i, k, got[k], v)
			}
		}
	}
	if strings.Contains(buf.String(), "\x1b") || strings.Contains(buf.String(), `\u001b`) {
		t.Errorf("a control character was logged: %s", buf.String())
	}
}

func TestSlogReporterNilLogger(t *testing.T) {
	t.Parallel()
	if err := NewSlogReporter(nil).Report(context.Background(), Event{Kind: EventComplete}); err == nil {
		t.Fatal("a nil logger reported nothing")
	}
}
