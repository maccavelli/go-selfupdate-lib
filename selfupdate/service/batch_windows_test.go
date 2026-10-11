//go:build windows

package service

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/maccavelli/go-selfupdate-lib/internal/winproc"
	"github.com/maccavelli/go-selfupdate-lib/selfupdate"
)

// TestBatchFileRefusedAtStartSites: runExec and DetachProcess refuse a
// .cmd program before starting anything (0021-MADR D8). The shim takes no
// arguments, so a broken guard runs nothing worse than an empty script.
func TestBatchFileRefusedAtStartSites(t *testing.T) {
	shim := filepath.Join(t.TempDir(), "shim.cmd")
	if err := os.WriteFile(shim, []byte("@echo off\r\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := runExec(context.Background(), Command{Path: shim}); !errors.Is(err, winproc.ErrBatchFile) {
		t.Errorf("runExec: %v, want ErrBatchFile", err)
	}
	if _, err := DetachProcess(context.Background(), HandOff{ID: "x", Executable: shim}); !errors.Is(err, winproc.ErrBatchFile) {
		t.Errorf("DetachProcess: %v, want ErrBatchFile", err)
	}
	// The exported name a program matches (0022-MADR D3), through the
	// exported entry points.
	if _, err := ExecRunner().Run(context.Background(), Command{Path: shim}); !errors.Is(err, selfupdate.ErrBatchFile) {
		t.Errorf("ExecRunner: %v, want selfupdate.ErrBatchFile", err)
	}
	if _, err := DetachProcess(context.Background(), HandOff{ID: "x", Executable: shim}); !errors.Is(err, selfupdate.ErrBatchFile) {
		t.Errorf("DetachProcess: %v, want selfupdate.ErrBatchFile", err)
	}
}
