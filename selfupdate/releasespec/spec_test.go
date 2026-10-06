package releasespec

import (
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/maccavelli/go-selfupdate-lib/selfupdate"
	"github.com/maccavelli/go-selfupdate-lib/selfupdate/archive"
)

// Tests for docs/decisions/0013-PLAN-build-and-stage-release-workflow.md
// B1: the release spec (0013-MADR §2, §3).

var (
	linuxAMD64   = selfupdate.Platform{OS: "linux", Arch: "amd64"}
	linuxARM64   = selfupdate.Platform{OS: "linux", Arch: "arm64"}
	darwinARM64  = selfupdate.Platform{OS: "darwin", Arch: "arm64"}
	windowsAMD64 = selfupdate.Platform{OS: "windows", Arch: "amd64"}
	windowsARM64 = selfupdate.Platform{OS: "windows", Arch: "arm64"}
)

func readFixture(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func mustParse(t *testing.T, data []byte) Spec {
	t.Helper()
	s, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	return s
}

// base is the smallest valid spec, as a value a case can change.
func base() map[string]any {
	return map[string]any{
		"schema":    1,
		"products":  []any{map[string]any{"name": "relay", "package": "."}},
		"platforms": []any{map[string]any{"os": "linux", "arch": "amd64"}},
	}
}

func product(m map[string]any) map[string]any {
	return m["products"].([]any)[0].(map[string]any)
}

func encode(t *testing.T, m map[string]any) []byte {
	t.Helper()
	b, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func repeat(n int, f func(i int) any) []any {
	out := make([]any, n)
	for i := range out {
		out[i] = f(i)
	}
	return out
}

func TestParseAccepts(t *testing.T) {
	for _, name := range []string{"fleet.json", "minimal.json", "archive.json"} {
		t.Run(name, func(t *testing.T) {
			s := mustParse(t, readFixture(t, name))
			if s.Schema != SchemaVersion {
				t.Fatalf("schema %d", s.Schema)
			}
		})
	}
	t.Run("fleet fields", func(t *testing.T) {
		s := mustParse(t, readFixture(t, "fleet.json"))
		want := Spec{
			Schema: 1,
			Products: []Product{
				{Name: "mcremote", Package: "./cmd/mcremote", IdentityArgs: []string{"version"}},
				{Name: "mcrelay", Package: "./cmd/mcrelay"},
			},
			Platforms: []Platform{
				{OS: "linux", Arch: "amd64"}, {OS: "linux", Arch: "arm64"},
				{OS: "darwin", Arch: "arm64"}, {OS: "windows", Arch: "amd64"},
			},
			Packaging: PackagingBinary,
			Extras: []Extra{
				{Name: "install.sh", Path: "scripts/install.sh"},
				{Name: "install.ps1", Path: "scripts/install.ps1"},
				{Name: "magic-cli-remote-{tag}-arm64.apk"},
			},
			PrereleaseChannels: []string{"rc", "beta"},
		}
		if !reflect.DeepEqual(s, want) {
			t.Fatalf("got %+v\nwant %+v", s, want)
		}
	})
	t.Run("empty optional lists are nil", func(t *testing.T) {
		s := mustParse(t, readFixture(t, "archive.json"))
		if s.Extras != nil || s.PrereleaseChannels != nil {
			t.Fatalf("extras %#v channels %#v", s.Extras, s.PrereleaseChannels)
		}
	})
	t.Run("no packaging is binary", func(t *testing.T) {
		s := mustParse(t, readFixture(t, "minimal.json"))
		if _, ok := s.FormatFor(linuxAMD64); ok {
			t.Fatal("a spec without packaging is packed")
		}
	})
}

func TestParseRefuses(t *testing.T) {
	valid := string(encode(t, base()))
	cases := []struct {
		name string
		raw  string                 // used when set
		edit func(m map[string]any) // applied to base otherwise
		want string
	}{
		// The JSON's shape.
		{name: "unknown field", edit: func(m map[string]any) { m["platform"] = 1 }, want: `unknown field "platform"`},
		{name: "unknown nested field", edit: func(m map[string]any) { product(m)["bin"] = "x" }, want: `unknown field "bin"`},
		{name: "key in another case", raw: strings.Replace(valid, `"schema"`, `"Schema"`, 1), want: `key "Schema" is not a field name`},
		{name: "duplicate key", raw: strings.Replace(valid, `"schema":1`, `"schema":1,"schema":1`, 1), want: `duplicate key "schema"`},
		{name: "trailing data", raw: valid + " {}", want: "data after the spec"},
		{name: "two values", raw: valid + valid, want: "data after the spec"},
		{name: "too large", raw: valid + strings.Repeat(" ", MaxSize), want: "more than 65536"},
		{name: "not JSON", raw: "{", want: "releasespec: "},
		{name: "null", raw: "null", want: "schema: 0 is not supported"},
		// schema
		{name: "schema 0", edit: func(m map[string]any) { m["schema"] = 0 }, want: "schema: 0 is not supported"},
		{name: "schema 2", edit: func(m map[string]any) { m["schema"] = 2 }, want: "schema: 2 is not supported"},
		{name: "no schema", edit: func(m map[string]any) { delete(m, "schema") }, want: "schema: 0 is not supported"},
		// products
		{name: "no products", edit: func(m map[string]any) { m["products"] = []any{} }, want: "products: 0 listed"},
		{name: "17 products", edit: func(m map[string]any) {
			m["products"] = repeat(17, func(i int) any { return map[string]any{"name": "p" + string(rune('a'+i)), "package": "."} })
		}, want: "products: 17 listed"},
		{name: "duplicate product", edit: func(m map[string]any) {
			m["products"] = repeat(2, func(int) any { return map[string]any{"name": "relay", "package": "."} })
		}, want: "products[1].name: \"relay\" is listed twice"},
		{name: "product in another case", edit: func(m map[string]any) {
			m["products"] = []any{map[string]any{"name": "relay", "package": "."}, map[string]any{"name": "Relay", "package": "."}}
		}, want: "products[1].name: \"Relay\" is listed twice"},
		{name: "product leading dash", edit: func(m map[string]any) { product(m)["name"] = "-relay" }, want: "products[0].name"},
		{name: "product space", edit: func(m map[string]any) { product(m)["name"] = "re lay" }, want: "products[0].name"},
		{name: "product slash", edit: func(m map[string]any) { product(m)["name"] = "re/lay" }, want: "products[0].name"},
		{name: "product 129 characters", edit: func(m map[string]any) { product(m)["name"] = strings.Repeat("r", 129) }, want: "products[0].name"},
		// package
		{name: "package without ./", edit: func(m map[string]any) { product(m)["package"] = "cmd/relay" }, want: `products[0].package: "cmd/relay" must be`},
		{name: "package ..", edit: func(m map[string]any) { product(m)["package"] = "./../relay" }, want: "products[0].package"},
		{name: "package empty element", edit: func(m map[string]any) { product(m)["package"] = "./a//b" }, want: "products[0].package"},
		{name: "package absolute", edit: func(m map[string]any) { product(m)["package"] = "/cmd/relay" }, want: "products[0].package"},
		{name: "package ./", edit: func(m map[string]any) { product(m)["package"] = "./" }, want: "products[0].package"},
		{name: "package ./.", edit: func(m map[string]any) { product(m)["package"] = "./." }, want: "products[0].package"},
		{name: "package dot element", edit: func(m map[string]any) { product(m)["package"] = "./a/./b" }, want: "products[0].package"},
		{name: "package backslash", edit: func(m map[string]any) { product(m)["package"] = `./a\b` }, want: "products[0].package"},
		{name: "package colon", edit: func(m map[string]any) { product(m)["package"] = "./c:d" }, want: "products[0].package"},
		{name: "package empty", edit: func(m map[string]any) { product(m)["package"] = "" }, want: "products[0].package"},
		// tags and identity_args
		{name: "bad tag", edit: func(m map[string]any) { product(m)["tags"] = []any{"net-go"} }, want: "products[0].tags[0]"},
		{name: "17 tags", edit: func(m map[string]any) { product(m)["tags"] = repeat(17, func(int) any { return "netgo" }) }, want: "products[0].tags: 17 listed"},
		{name: "empty identity_args", edit: func(m map[string]any) { product(m)["identity_args"] = []any{} }, want: "products[0].identity_args: an empty list"},
		{name: "17 identity_args", edit: func(m map[string]any) {
			product(m)["identity_args"] = repeat(17, func(int) any { return "version" })
		}, want: "products[0].identity_args: 17 listed"},
		{name: "empty identity arg", edit: func(m map[string]any) { product(m)["identity_args"] = []any{"version", ""} }, want: "products[0].identity_args[1]"},
		{name: "NUL identity arg", edit: func(m map[string]any) { product(m)["identity_args"] = []any{"ver\x00sion"} }, want: "products[0].identity_args[0]"},
		// platforms
		{name: "no platforms", edit: func(m map[string]any) { m["platforms"] = []any{} }, want: "platforms: 0 listed"},
		{name: "33 platforms", edit: func(m map[string]any) {
			m["platforms"] = repeat(33, func(i int) any {
				return map[string]any{"os": "os" + string(rune('a'+i%26)) + string(rune('a'+i/26)), "arch": "amd64"}
			})
		}, want: "platforms: 33 listed"},
		{name: "duplicate platform", edit: func(m map[string]any) {
			m["platforms"] = repeat(2, func(int) any { return map[string]any{"os": "linux", "arch": "amd64"} })
		}, want: "platforms[1]: linux/amd64 is listed twice"},
		{name: "uppercase os", edit: func(m map[string]any) { m["platforms"] = []any{map[string]any{"os": "Linux", "arch": "amd64"}} }, want: "platforms[0].os"},
		{name: "bad arch", edit: func(m map[string]any) { m["platforms"] = []any{map[string]any{"os": "linux", "arch": "amd-64"}} }, want: "platforms[0].arch"},
		{name: "format under binary", edit: func(m map[string]any) {
			m["platforms"] = []any{map[string]any{"os": "linux", "arch": "amd64", "format": "tar.gz"}}
		}, want: "platforms[0].format: \"tar.gz\" needs \"packaging\""},
		{name: "unknown format", edit: func(m map[string]any) {
			m["packaging"] = "archive"
			m["platforms"] = []any{map[string]any{"os": "linux", "arch": "amd64", "format": "tar.xz"}}
		}, want: "platforms[0].format: \"tar.xz\" is not"},
		{name: "unknown packaging", edit: func(m map[string]any) { m["packaging"] = "zip" }, want: "packaging: \"zip\""},
		// extras
		{name: "33 extras", edit: func(m map[string]any) {
			m["extras"] = repeat(33, func(i int) any { return map[string]any{"name": "e" + string(rune('a'+i%26)) + string(rune('a'+i/26))} })
		}, want: "extras: 33 listed"},
		{name: "extra SHA256SUMS", edit: func(m map[string]any) { m["extras"] = []any{map[string]any{"name": "SHA256SUMS"}} }, want: "extras[0].name: \"SHA256SUMS\" is reserved"},
		{name: "extra SHA256SUMS-", edit: func(m map[string]any) { m["extras"] = []any{map[string]any{"name": "SHA256SUMS-1.0"}} }, want: "extras[0].name"},
		{name: "extra canonical name", edit: func(m map[string]any) { m["extras"] = []any{map[string]any{"name": "relay-linux-amd64"}} }, want: "is already a canonical asset's name"},
		{name: "extra canonical name in another case", edit: func(m map[string]any) {
			m["extras"] = []any{map[string]any{"name": "Relay-Linux-AMD64"}}
		}, want: "is already a canonical asset's name"},
		{name: "extra bad name", edit: func(m map[string]any) { m["extras"] = []any{map[string]any{"name": "install script"}} }, want: "extras[0].name"},
		{name: "extra bad after tag", edit: func(m map[string]any) { m["extras"] = []any{map[string]any{"name": "{tag}/notes"}} }, want: "extras[0].name: \"v1.2.3/notes\""},
		{name: "extra unknown placeholder", edit: func(m map[string]any) { m["extras"] = []any{map[string]any{"name": "notes-{version}"}} }, want: "extras[0].name"},
		{name: "duplicate extra", edit: func(m map[string]any) {
			m["extras"] = repeat(2, func(int) any { return map[string]any{"name": "install.sh"} })
		}, want: "extras[1].name: \"install.sh\" is already another extra's name"},
		{name: "duplicate extra after tag", edit: func(m map[string]any) {
			m["extras"] = []any{map[string]any{"name": "notes-{tag}"}, map[string]any{"name": "notes-v1.2.3"}}
		}, want: "extras[1].name"},
		{name: "extra path ..", edit: func(m map[string]any) { m["extras"] = []any{map[string]any{"name": "a", "path": "../a"}} }, want: "extras[0].path"},
		{name: "extra path absolute", edit: func(m map[string]any) { m["extras"] = []any{map[string]any{"name": "a", "path": "/a"}} }, want: "extras[0].path"},
		{name: "extra path empty element", edit: func(m map[string]any) { m["extras"] = []any{map[string]any{"name": "a", "path": "a//b"}} }, want: "extras[0].path"},
		{name: "extra path backslash", edit: func(m map[string]any) { m["extras"] = []any{map[string]any{"name": "a", "path": `a\b`}} }, want: "extras[0].path"},
		{name: "extra path dot", edit: func(m map[string]any) { m["extras"] = []any{map[string]any{"name": "a", "path": "."}} }, want: "extras[0].path"},
		// prerelease_channels, as check-release-tag.sh refuses them.
		{name: "bad channel", edit: func(m map[string]any) { m["prerelease_channels"] = []any{"RC"} }, want: "prerelease_channels[0]"},
		{name: "ascending channels", edit: func(m map[string]any) { m["prerelease_channels"] = []any{"beta", "rc"} }, want: "prerelease_channels[1]: \"beta\" must come after \"rc\""},
		{name: "duplicate channel", edit: func(m map[string]any) { m["prerelease_channels"] = []any{"rc", "rc"} }, want: "prerelease_channels[1]"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			data := []byte(tc.raw)
			if tc.raw == "" {
				m := base()
				tc.edit(m)
				data = encode(t, m)
			}
			_, err := Parse(data)
			if err == nil {
				t.Fatalf("Parse accepted %s", data)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error %q does not contain %q", err, tc.want)
			}
			if !strings.HasPrefix(err.Error(), "releasespec: ") {
				t.Fatalf("error %q lacks the package prefix", err)
			}
		})
	}
}

func TestValidateRefusesABuiltSpec(t *testing.T) {
	s := Spec{Schema: 1, Products: []Product{{Name: "relay", Package: ".", IdentityArgs: []string{}}}, Platforms: []Platform{{OS: "linux", Arch: "amd64"}}}
	if err := s.Validate(); err == nil || !strings.Contains(err.Error(), "identity_args: an empty list") {
		t.Fatalf("Validate: %v", err)
	}
	for name, f := range map[string]func() error{
		"AssetSelector": func() error { _, err := s.AssetSelector(); return err },
		"Unpacker":      func() error { _, err := s.Unpacker(); return err },
	} {
		if err := f(); err == nil {
			t.Errorf("%s accepted an invalid spec", name)
		}
	}
}

func TestProduct(t *testing.T) {
	s := mustParse(t, readFixture(t, "fleet.json"))
	p, err := s.Product("mcrelay")
	if err != nil || p.Package != "./cmd/mcrelay" {
		t.Fatalf("Product(mcrelay) = %+v, %v", p, err)
	}
	_, err = s.Product("magic-cli-remote")
	if err == nil || !strings.Contains(err.Error(), `product "magic-cli-remote" is not in the spec (products: mcremote, mcrelay)`) {
		t.Fatalf("Product(magic-cli-remote): %v", err)
	}
}

func TestTargets(t *testing.T) {
	s := mustParse(t, readFixture(t, "fleet.json"))
	want := []selfupdate.Platform{linuxAMD64, linuxARM64, darwinARM64, windowsAMD64}
	if got := s.Targets(); !reflect.DeepEqual(got, want) {
		t.Fatalf("Targets() = %v", got)
	}
}

func TestFormatFor(t *testing.T) {
	s := mustParse(t, readFixture(t, "archive.json"))
	for p, want := range map[selfupdate.Platform]archive.Format{
		linuxAMD64: archive.TarGz, linuxARM64: archive.Gz, darwinARM64: archive.TarGz,
		windowsAMD64: archive.Zip, windowsARM64: archive.TarGz,
	} {
		if got, ok := s.FormatFor(p); !ok || got != want {
			t.Errorf("FormatFor(%v) = %q, %v; want %q", p, got, ok, want)
		}
	}
	if got, ok := s.FormatFor(selfupdate.Platform{OS: "freebsd", Arch: "amd64"}); ok || got != "" {
		t.Errorf("FormatFor(unlisted) = %q, %v", got, ok)
	}
	bin := mustParse(t, readFixture(t, "fleet.json"))
	if got, ok := bin.FormatFor(windowsAMD64); ok || got != "" {
		t.Errorf("binary FormatFor = %q, %v", got, ok)
	}
}

func TestAssetName(t *testing.T) {
	for _, fixture := range []string{"fleet.json", "archive.json"} {
		s := mustParse(t, readFixture(t, fixture))
		for _, prod := range s.Products {
			for _, p := range s.Targets() {
				got, err := s.AssetName(prod.Name, p)
				if err != nil {
					t.Fatalf("%s: AssetName(%s, %v): %v", fixture, prod.Name, p, err)
				}
				want := selfupdate.ExactAssetName(prod.Name, p)
				if f, ok := s.FormatFor(p); ok {
					want, _ = archive.FleetName(prod.Name, "", p, f)
				}
				if got != want {
					t.Errorf("%s: AssetName(%s, %v) = %q, want %q", fixture, prod.Name, p, got, want)
				}
			}
		}
	}
	s := mustParse(t, readFixture(t, "archive.json"))
	for p, want := range map[selfupdate.Platform]string{
		linuxARM64: "relay-linux-arm64.gz", windowsAMD64: "relay-windows-amd64.zip", windowsARM64: "relay-windows-arm64.tar.gz",
	} {
		if got, _ := s.AssetName("relay", p); got != want {
			t.Errorf("AssetName(relay, %v) = %q, want %q", p, got, want)
		}
	}
	if _, err := s.AssetName("nope", linuxAMD64); err == nil {
		t.Error("AssetName accepted an unlisted product")
	}
	if _, err := s.AssetName("relay", selfupdate.Platform{OS: "freebsd", Arch: "amd64"}); err == nil {
		t.Error("AssetName accepted an unlisted platform")
	}
}

// release holds the named assets, as uploaded.
func release(names ...string) selfupdate.Release {
	rel := selfupdate.Release{Tag: "v1.2.3"}
	for i, n := range names {
		rel.Assets = append(rel.Assets, selfupdate.Asset{ID: int64(i + 1), Name: n, State: selfupdate.AssetStateUploaded, Size: 1})
	}
	return rel
}

func TestSelectorAndUnpackerMatch(t *testing.T) {
	for _, fixture := range []string{"fleet.json", "archive.json"} {
		t.Run(fixture, func(t *testing.T) {
			s := mustParse(t, readFixture(t, fixture))
			sel, err := s.AssetSelector()
			if err != nil {
				t.Fatal(err)
			}
			u, err := s.Unpacker()
			if err != nil {
				t.Fatal(err)
			}
			prod := s.Products[0].Name
			for _, p := range s.Targets() {
				name, err := s.AssetName(prod, p)
				if err != nil {
					t.Fatal(err)
				}
				got, err := sel.Select(release(name, "SHA256SUMS"), prod, p)
				if err != nil {
					t.Fatalf("Select(%v): %v", p, err)
				}
				if got.Binary.Name != name || got.Packed != (u != nil) {
					t.Fatalf("Select(%v) = %+v; want %q, packed %v", p, got, name, u != nil)
				}
			}
			if _, err := sel.Select(release("x", "SHA256SUMS"), prod, selfupdate.Platform{OS: "freebsd", Arch: "amd64"}); err == nil {
				t.Fatal("the selector accepted an unlisted platform")
			}
		})
	}
}

func TestExtraNames(t *testing.T) {
	s := mustParse(t, readFixture(t, "fleet.json"))
	got, err := s.ExtraNames("v0.20.2")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"install.sh", "install.ps1", "magic-cli-remote-v0.20.2-arm64.apk"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ExtraNames = %v", got)
	}
	if _, err := s.ExtraNames("v1/2"); err == nil {
		t.Fatal("ExtraNames accepted a tag that makes a bad name")
	}
	if names, err := mustParse(t, readFixture(t, "minimal.json")).ExtraNames("v1.0.0"); err != nil || len(names) != 0 {
		t.Fatalf("no extras: %v, %v", names, err)
	}
}

func TestPublishInputs(t *testing.T) {
	fleet := mustParse(t, readFixture(t, "fleet.json"))
	arch := mustParse(t, readFixture(t, "archive.json"))
	extras, err := fleet.ExtrasJSON("v0.20.2")
	if err != nil {
		t.Fatal(err)
	}
	noExtras, err := arch.ExtrasJSON("v1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct{ name, got, want string }{
		{"fleet products", fleet.ProductsJSON(), `["mcremote","mcrelay"]`},
		{"fleet platforms", fleet.PlatformsJSON(), `[{"os":"linux","arch":"amd64"},{"os":"linux","arch":"arm64"},{"os":"darwin","arch":"arm64"},{"os":"windows","arch":"amd64"}]`},
		{"fleet extras", extras, `["install.sh","install.ps1","magic-cli-remote-v0.20.2-arm64.apk"]`},
		{"fleet channels", fleet.ChannelsJSON(), `["rc","beta"]`},
		{"archive platforms", arch.PlatformsJSON(), `[{"os":"linux","arch":"amd64","format":"tar.gz"},{"os":"linux","arch":"arm64","format":"gz"},` +
			`{"os":"darwin","arch":"arm64","format":"tar.gz"},{"os":"windows","arch":"amd64","format":"zip"},{"os":"windows","arch":"arm64","format":"tar.gz"}]`},
		{"archive extras", noExtras, `[]`},
		{"archive channels", arch.ChannelsJSON(), `[]`},
	} {
		if c.got != c.want {
			t.Errorf("%s:\ngot  %s\nwant %s", c.name, c.got, c.want)
		}
	}
}
