package service

import (
	"encoding/json"
	"reflect"
	"testing"
)

// jsonRoundTrip marshals v, checks it against want, and unmarshals it back
// into a new value of v's type, which must equal v
// (docs/decisions/0011-PLAN-reference-service-lifecycles.md V5 step 0).
func jsonRoundTrip[T any](t *testing.T, v T, want string) {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != want {
		t.Fatalf("%T marshals as\n%s\nwant\n%s", v, b, want)
	}
	var back T
	if err := json.Unmarshal(b, &back); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(back, v) {
		t.Fatalf("%T round-trips as %+v, want %+v", v, back, v)
	}
}

func TestRecordsJSON(t *testing.T) {
	jsonRoundTrip(t, Detached{ID: "abc", Where: "process 42", ResultPath: "/opt/demo/.demo.selfupdate.handoff"},
		`{"id":"abc","where":"process 42","result_path":"/opt/demo/.demo.selfupdate.handoff"}`)
	jsonRoundTrip(t, Health{Ready: true, Instance: "42", Detail: "demo RUNNING"},
		`{"ready":true,"instance":"42","failed":false,"detail":"demo RUNNING"}`)
	jsonRoundTrip(t, Health{Failed: true},
		`{"ready":false,"failed":true}`)
	jsonRoundTrip(t, ExecState{Receipt: Receipt{SchemaVersion: 1, Verdict: VerdictUnchanged}, Executable: "/opt/demo/demo"},
		`{"receipt":{"schema_version":1,"verdict":"unchanged","changed":false,"reloaded":false},"executable":"/opt/demo/demo"}`)
}
