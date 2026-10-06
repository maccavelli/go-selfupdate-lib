package releasespec

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// FuzzParse checks that Parse never panics, and that an accepted spec
// encodes and parses back to an equal value whose publish inputs are the
// same (0013-PLAN B1).
func FuzzParse(f *testing.F) {
	seeds, err := filepath.Glob(filepath.Join("testdata", "*.json"))
	if err != nil {
		f.Fatal(err)
	}
	for _, path := range seeds {
		data, err := os.ReadFile(path)
		if err != nil {
			f.Fatal(err)
		}
		f.Add(data)
	}
	f.Add([]byte(`{"schema":1,"products":[{"name":"a","package":"."}],"platforms":[{"os":"linux","arch":"amd64"}],"extras":[{"name":"x-{tag}"}]}`))
	f.Fuzz(func(t *testing.T, data []byte) {
		s, err := Parse(data)
		if err != nil {
			return
		}
		again, err := json.Marshal(s)
		if err != nil {
			t.Fatalf("Marshal: %v", err)
		}
		s2, err := Parse(again)
		if err != nil {
			t.Fatalf("an accepted spec re-encodes to one Parse refuses: %v\n%s", err, again)
		}
		if !reflect.DeepEqual(s, s2) {
			t.Fatalf("round trip changed the spec:\n%+v\n%+v", s, s2)
		}
		if s.ProductsJSON() != s2.ProductsJSON() || s.PlatformsJSON() != s2.PlatformsJSON() || s.ChannelsJSON() != s2.ChannelsJSON() {
			t.Fatal("round trip changed the publish inputs")
		}
		if _, err := s.AssetSelector(); err != nil {
			t.Fatalf("an accepted spec has no selector: %v", err)
		}
	})
}
