package selfupdate

import (
	"regexp"
	"runtime"
	"strings"
	"testing"
)

// uaShape is the only shape UserAgent may return
// (0004-PLAN-v1-4-0-command-surface.md, A7).
var uaShape = regexp.MustCompile(`^[^\s/()]+/[^\s/()]+ \([a-z0-9]+/[a-z0-9]+\)$`)

func TestUserAgent(t *testing.T) {
	t.Parallel()
	plat := " (" + runtime.GOOS + "/" + runtime.GOARCH + ")"
	for _, tc := range []struct {
		product, version, want string
	}{
		{"demo", "v1.2.3", "demo/v1.2.3"},
		{"", "v1.2.3", "unknown/v1.2.3"},
		{"demo", "", "demo/unknown"},
		{"demo", "v1 .2 / 3", "demo/v1-.2-3"},
		{"my tool", "1.0 (beta)", "my-tool/1.0-beta"},
		{"de\nmo", "v1\x7f.2\u0085.3", "demo/v1.2.3"},
		{"de\xffmo", "v1.2.3", "demo/v1.2.3"},
		{"  /()  ", "\t\n", "unknown/unknown"},
		{"-demo-", "--", "demo/unknown"},
	} {
		got := UserAgent(tc.product, tc.version)
		if got != tc.want+plat {
			t.Errorf("UserAgent(%q, %q) = %q, want %q", tc.product, tc.version, got, tc.want+plat)
		}
		if err := validateUserAgent(got); err != nil {
			t.Errorf("UserAgent(%q, %q) = %q fails validateUserAgent: %v", tc.product, tc.version, got, err)
		}
		if !uaShape.MatchString(got) {
			t.Errorf("UserAgent(%q, %q) = %q has the wrong shape", tc.product, tc.version, got)
		}
	}
}

func FuzzUserAgent(f *testing.F) {
	for _, s := range []string{"demo", "", "a b/c(d)", "\x00\x7f\u0085", "\xff\xfe", "日本語", "-"} {
		f.Add(s, s)
	}
	f.Fuzz(func(t *testing.T, product, version string) {
		got := UserAgent(product, version)
		if err := validateUserAgent(got); err != nil {
			t.Fatalf("UserAgent(%q, %q) = %q: %v", product, version, got, err)
		}
		if !uaShape.MatchString(got) {
			t.Fatalf("UserAgent(%q, %q) = %q has the wrong shape", product, version, got)
		}
		p, v, _ := strings.Cut(strings.TrimSuffix(got, " ("+runtime.GOOS+"/"+runtime.GOARCH+")"), "/")
		if again := UserAgent(p, v); again != got {
			t.Fatalf("not stable: UserAgent(%q, %q) = %q, then %q", product, version, got, again)
		}
	})
}
