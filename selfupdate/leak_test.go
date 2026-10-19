package selfupdate

import "github.com/maccavelli/go-selfupdate-lib/internal/testproc"

// checkNoLeak is testproc.CheckNoLeak, the leak check every package's tests
// share (docs/decisions/0025-MADR-align-with-go-sdk-design-practices.md D4).
// Call it first:
//
//	defer checkNoLeak(t)()
var checkNoLeak = testproc.CheckNoLeak
