package systemd

import (
	"encoding/json"
	"reflect"
	"testing"
)

// TestDropInJSON pins the receipt's keys and round-trips it
// (docs/decisions/0011-PLAN-reference-service-lifecycles.md V5 step 0).
func TestDropInJSON(t *testing.T) {
	for v, want := range map[*DropIn]string{
		{Path: "/etc/systemd/system/demo.service.d/90-selfupdate.conf", Existed: true, Previous: []byte("a")}: `{"path":"/etc/systemd/system/demo.service.d/90-selfupdate.conf","existed":true,"previous":"YQ=="}`,
		{Path: "/x"}: `{"path":"/x","existed":false}`,
	} {
		b, err := json.Marshal(*v)
		if err != nil || string(b) != want {
			t.Fatalf("%s, %v; want %s", b, err, want)
		}
		var back DropIn
		if err := json.Unmarshal(b, &back); err != nil || !reflect.DeepEqual(back, *v) {
			t.Fatalf("round trip %+v, %v", back, err)
		}
	}
}
