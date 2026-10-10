package winproc

import (
	"context"
	"path/filepath"
	"slices"
	"testing"
)

// TestCommandPassesProgramAndArgs: Command builds the command it was asked
// for, with every argument in order.
func TestCommandPassesProgramAndArgs(t *testing.T) {
	path := filepath.Join(t.TempDir(), "prog")
	cmd, err := Command(context.Background(), path, "a", "b c")
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{path, "a", "b c"}; cmd.Path != path || !slices.Equal(cmd.Args, want) {
		t.Fatalf("Command: Path %q, Args %q; want %q, %q", cmd.Path, cmd.Args, path, want)
	}
}

// TestDetachedPassesProgramAndArgs: Detached builds the command it was
// asked for, with every argument in order.
func TestDetachedPassesProgramAndArgs(t *testing.T) {
	path := filepath.Join(t.TempDir(), "prog")
	cmd, err := Detached(path, []string{"a", "b c"}, true)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{path, "a", "b c"}; cmd.Path != path || !slices.Equal(cmd.Args, want) {
		t.Fatalf("Detached: Path %q, Args %q; want %q, %q", cmd.Path, cmd.Args, path, want)
	}
}
