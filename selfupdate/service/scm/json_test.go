package scm

import (
	"encoding/json"
	"testing"
)

// TestPathBackupJSON pins the receipt's keys and round-trips it
// (docs/decisions/0011-PLAN-reference-service-lifecycles.md V5 step 0).
func TestPathBackupJSON(t *testing.T) {
	v := PathBackup{Name: "demo", Previous: `"C:\Old\demo.exe" run`}
	const want = `{"name":"demo","previous":"\"C:\\Old\\demo.exe\" run"}`
	b, err := json.Marshal(v)
	if err != nil || string(b) != want {
		t.Fatalf("%s, %v; want %s", b, err, want)
	}
	var back PathBackup
	if err := json.Unmarshal(b, &back); err != nil || back != v {
		t.Fatalf("round trip %+v, %v", back, err)
	}
}
