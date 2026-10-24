package procfs

import (
	"io"
	"os"
	"testing"
)

// helperEnv makes the test binary a helper process: "wait" writes the file
// named by PROCFS_READY and blocks until its standard input closes; "exit"
// exits at once.
const helperEnv = "PROCFS_HELPER"

func TestMain(m *testing.M) {
	switch os.Getenv(helperEnv) {
	case "wait":
		if err := os.WriteFile(os.Getenv("PROCFS_READY"), nil, 0o600); err != nil {
			os.Exit(3)
		}
		_, _ = io.Copy(io.Discard, os.Stdin)
		os.Exit(0)
	case "exit":
		os.Exit(0)
	}
	os.Exit(m.Run())
}
