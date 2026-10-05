package codesign

import "runtime"

func isDarwin() bool { return runtime.GOOS == "darwin" }
