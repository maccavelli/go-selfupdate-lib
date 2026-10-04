package selfupdate

import (
	"context"
	"encoding/json"
	"slices"
	"strings"
	"testing"
)

// Tests for docs/decisions/0010-PLAN-v1-6-0-owner-contracts.md S3 and its
// deviation D2: EventWarning, Result.Warnings and the Warnings type.

// isComparable instantiates only with a comparable type.
func isComparable[T comparable]() {}

// Result, Finished and ResultDocument stay comparable (amendment A4); this
// does not compile otherwise.
var _ = []func(){isComparable[Result], isComparable[Finished], isComparable[ResultDocument], isComparable[Warnings]}

// TestLateErrorWarns: each late error is one EventWarning after complete,
// and the same text in Result.Warnings.
func TestLateErrorWarns(t *testing.T) {
	for name, mutate := range lateErrorCases() {
		t.Run(name, func(t *testing.T) {
			env := newContractEnv(t)
			req := applyReq()
			req.Yes = true
			mutate(env, &req)
			res, err := env.build(t).Run(context.Background(), req)
			if err != nil {
				t.Fatal(err)
			}
			at := slices.Index(env.rep.kinds, EventComplete)
			after := env.rep.events[at+1:]
			if len(after) != 1 || after[0].Kind != EventWarning || !strings.HasPrefix(after[0].Detail, "fixture: ") {
				t.Fatalf("events after complete: %+v", after)
			}
			if got := res.Warnings.List(); len(got) != 1 || got[0] != after[0].Detail {
				t.Fatalf("Warnings = %q, want the event's %q", got, after[0].Detail)
			}
			if doc := res.Document(); doc.Warnings != res.Warnings {
				t.Fatalf("document warnings %q", doc.Warnings)
			}
		})
	}
}

func TestWarningsType(t *testing.T) {
	w := NewWarnings("first", "", "two\nlines\x1b", "  ")
	if got := w.List(); !slices.Equal(got, []string{"first", "two lines?"}) || w.Len() != 2 {
		t.Fatalf("List = %q, Len = %d", got, w.Len())
	}
	more := w.Add("third")
	if w.Len() != 2 || more.Len() != 3 || more.List()[2] != "third" {
		t.Fatalf("Add changed its receiver, or lost the entry: %q, %q", w, more)
	}
	var zero Warnings
	if zero.List() != nil || zero.Len() != 0 || NewWarnings() != zero || NewWarnings("") != zero {
		t.Fatalf("zero value: %q", zero.List())
	}
	if NewWarnings("a", "b") != NewWarnings("a").Add("b") {
		t.Fatal("equal warnings compare unequal")
	}
}

func TestWarningsJSON(t *testing.T) {
	for _, c := range []struct {
		w    Warnings
		json string
	}{
		{"", `[]`},
		{NewWarnings("one"), `["one"]`},
		{NewWarnings("one", `say "two"`), `["one","say \"two\""]`},
	} {
		b, err := json.Marshal(c.w)
		if err != nil || string(b) != c.json {
			t.Errorf("%q: %s, %v; want %s", c.w, b, err, c.json)
		}
		var back Warnings
		if err := json.Unmarshal(b, &back); err != nil || back != c.w {
			t.Errorf("%s: read back %q, %v", b, back, err)
		}
	}
	var w Warnings
	if err := json.Unmarshal([]byte(`["a\nb", ""]`), &w); err != nil || w != NewWarnings("a b") {
		t.Errorf("unsanitized input read as %q, %v", w, err)
	}
	if err := json.Unmarshal([]byte(`null`), &w); err != nil || w != "" {
		t.Errorf("null read as %q, %v", w, err)
	}
	if err := json.Unmarshal([]byte(`"a"`), &w); err == nil {
		t.Error("a string was read as warnings")
	}
}

// TestDocumentWarnings: the document omits warnings when there are none,
// and otherwise writes them as an array that reads back.
func TestDocumentWarnings(t *testing.T) {
	res := Result{Product: "demo", CurrentVersion: "v1.0.0", Applied: true}
	b, err := json.Marshal(res.Document())
	if err != nil || strings.Contains(string(b), "warnings") || !strings.Contains(string(b), `"schema_version":2`) {
		t.Fatalf("no warnings: %s, %v", b, err)
	}
	res.Warnings = NewWarnings("unlock failed")
	b, err = json.Marshal(res.Document())
	if err != nil || !strings.HasSuffix(string(b), `"service_started":false,"warnings":["unlock failed"]}`) {
		t.Fatalf("with warnings: %s, %v", b, err)
	}
	var doc ResultDocument
	if err := json.Unmarshal(b, &doc); err != nil || doc != res.Document() {
		t.Fatalf("read back %+v, %v", doc, err)
	}
}
