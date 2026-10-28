//go:build windows

package selfupdate

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/maccavelli/go-selfupdate-lib/internal/winproc"
)

// TestVersionProberRefusesBatchFile: the version prober refuses a .cmd
// target before running it (0021-MADR D8).
func TestVersionProberRefusesBatchFile(t *testing.T) {
	t.Parallel()
	shim := filepath.Join(t.TempDir(), "shim.cmd")
	if err := os.WriteFile(shim, []byte("@echo off\r\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	p, err := NewVersionProber([]string{"--version"}, nil, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	err = p.Probe(context.Background(), ProbeRequest{Product: "x", TargetVersion: "v1.0.0", Path: shim, Phase: ProbeStaged})
	if !errors.Is(err, winproc.ErrBatchFile) {
		t.Fatalf("Probe: %v, want ErrBatchFile", err)
	}
	if !errors.Is(err, ErrBatchFile) {
		t.Fatalf("Probe: %v, want the exported selfupdate.ErrBatchFile (0022-MADR D3)", err)
	}
}
