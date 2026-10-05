package launchd

import (
	"encoding/json"
	"reflect"
	"testing"
)

// TestPlistBackupJSON pins the receipt's keys and round-trips it
// (docs/decisions/0011-PLAN-reference-service-lifecycles.md V5 step 0).
func TestPlistBackupJSON(t *testing.T) {
	v := PlistBackup{Path: "/Library/LaunchDaemons/demo.plist", Previous: []byte("a"), Mode: 0o644, UID: 0, GID: 80}
	const want = `{"path":"/Library/LaunchDaemons/demo.plist","previous":"YQ==","mode":420,"uid":0,"gid":80}`
	b, err := json.Marshal(v)
	if err != nil || string(b) != want {
		t.Fatalf("%s, %v; want %s", b, err, want)
	}
	var back PlistBackup
	if err := json.Unmarshal(b, &back); err != nil || !reflect.DeepEqual(back, v) {
		t.Fatalf("round trip %+v, %v", back, err)
	}
}
