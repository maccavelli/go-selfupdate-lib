package taskscheduler

import (
	"encoding/json"
	"testing"
)

// TestDefinitionBackupJSON pins the receipt's keys and round-trips it,
// with a non-ASCII path intact.
func TestDefinitionBackupJSON(t *testing.T) {
	v := DefinitionBackup{Folder: `\Vendor`, Name: "demo", XML: `<Command>C:\café\demo.exe</Command>`}
	const want = `{"folder":"\\Vendor","name":"demo","xml":"\u003cCommand\u003eC:\\café\\demo.exe\u003c/Command\u003e"}`
	b, err := json.Marshal(v)
	if err != nil || string(b) != want {
		t.Fatalf("%s, %v; want %s", b, err, want)
	}
	var back DefinitionBackup
	if err := json.Unmarshal(b, &back); err != nil || back != v {
		t.Fatalf("round trip %+v, %v", back, err)
	}
}
