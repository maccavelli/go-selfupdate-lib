package archive

import "testing"

// TestWindowsReserved: the device names Windows reserves, before the first
// dot and ignoring case and trailing spaces, as Go's isReservedName reads
// them, with COM0 and LPT0 too
// (docs/decisions/0017-PLAN-verify-build-provenance-and-close-0015-open-items.md
// P2, N5).
func TestWindowsReserved(t *testing.T) {
	for _, el := range []string{
		"aux", "AUX", "Aux", "aux.txt", "AUX.tar.gz", "con", "prn", "nul", "NUL.txt",
		"com0", "com1", "COM9", "com1.tar.gz", "lpt0", "LPT9.log",
		"conin$", "CONOUT$", "conout$.x", "aux .txt",
	} {
		if !windowsReserved(el) {
			t.Errorf("%q is not reserved", el)
		}
	}
	for _, el := range []string{
		"com10", "lpt", "com", "comx", "auxiliary", "aux_", "xaux", "con1", "conin",
		"relay", "relay.exe", "", ".aux",
	} {
		if windowsReserved(el) {
			t.Errorf("%q is reserved", el)
		}
	}
}
