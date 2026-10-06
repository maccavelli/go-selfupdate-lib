package releasespec

import (
	"reflect"
	"strings"
	"testing"
)

// Tests for docs/decisions/0014-PLAN-shared-installer-templates.md I1: the
// spec's installer field (0014-MADR §2).

func TestInstallerAccepted(t *testing.T) {
	s := mustParse(t, readFixture(t, "installer.json"))
	want := &Installer{
		Name: "relay-suite", EnvPrefix: "RELAY",
		Hooks: []Hook{
			{When: HookBeforeInstall, Product: "relayctl", Args: []string{"service", "stop", "--if-running"}},
			{When: HookAfterInstall, Product: "relay", Args: []string{"configure"}},
		},
	}
	if !reflect.DeepEqual(s.Installer, want) {
		t.Fatalf("installer = %+v", s.Installer)
	}

	m := base()
	m["installer"] = map[string]any{}
	empty := mustParse(t, encode(t, m))
	if empty.Installer == nil {
		t.Fatal(`"installer": {} did not turn the installers on`)
	}

	without := mustParse(t, readFixture(t, "minimal.json"))
	if without.Installer != nil || without.InstallerScripts() != nil {
		t.Fatalf("a spec without installer: %+v, %v", without.Installer, without.InstallerScripts())
	}
}

func TestInstallerRefused(t *testing.T) {
	hook := func(when, product string, args ...any) map[string]any {
		return map[string]any{"when": when, "product": product, "args": args}
	}
	cases := []struct {
		name      string
		installer map[string]any
		extras    []any
		want      string
	}{
		{"bad name", map[string]any{"name": "-relay"}, nil, "installer.name"},
		{"lower-case prefix", map[string]any{"env_prefix": "relay"}, nil, "installer.env_prefix"},
		{"prefix of 33", map[string]any{"env_prefix": "R" + strings.Repeat("X", 32)}, nil, "installer.env_prefix"},
		{"nine hooks", map[string]any{"hooks": repeat(9, func(int) any { return hook("after_install", "relay", "x") })}, nil, "installer.hooks: 9 listed"},
		{"a hook at another time", map[string]any{"hooks": []any{hook("after_uninstall", "relay", "x")}}, nil, "installer.hooks[0].when"},
		{"a hook on an unlisted product", map[string]any{"hooks": []any{hook("after_install", "other", "x")}}, nil, `installer.hooks[0].product: "other" is not a product`},
		{"a hook without arguments", map[string]any{"hooks": []any{hook("after_install", "relay")}}, nil, "installer.hooks[0].args: 0 listed"},
		{"a hook with 17 arguments", map[string]any{"hooks": []any{hook("after_install", "relay", repeat(17, func(int) any { return "x" })...)}}, nil, "installer.hooks[0].args: 17 listed"},
		{"an empty hook argument", map[string]any{"hooks": []any{hook("after_install", "relay", "configure", "")}}, nil, "installer.hooks[0].args[1]"},
		{"a NUL hook argument", map[string]any{"hooks": []any{hook("after_install", "relay", "con\x00figure")}}, nil, "installer.hooks[0].args[0]"},
		{"a quote in a hook argument", map[string]any{"hooks": []any{hook("after_install", "relay", "it's")}}, nil, "installer.hooks[0].args[0]: \"it's\" must match"},
		{"a space in a hook argument", map[string]any{"hooks": []any{hook("after_install", "relay", "a b")}}, nil, "as the installers embed it"},
		{"a dollar in a hook argument", map[string]any{"hooks": []any{hook("after_install", "relay", "$HOME")}}, nil, "installer.hooks[0].args[0]"},
		{"a backtick in a hook argument", map[string]any{"hooks": []any{hook("after_install", "relay", "`id`")}}, nil, "installer.hooks[0].args[0]"},
		{"an extra named install.sh", map[string]any{}, []any{map[string]any{"name": "install.sh"}}, `extras[0].name: "install.sh" is already an installer's name`},
		{"an extra named INSTALL.PS1", map[string]any{}, []any{map[string]any{"name": "INSTALL.PS1"}}, `extras[0].name: "INSTALL.PS1" is already an installer's name`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := base()
			m["installer"] = tc.installer
			if tc.extras != nil {
				m["extras"] = tc.extras
			}
			_, err := Parse(encode(t, m))
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("Parse: %v; want %q", err, tc.want)
			}
		})
	}

	// Without the installer, the names are ordinary extras.
	m := base()
	m["extras"] = []any{map[string]any{"name": "install.sh"}, map[string]any{"name": "install.ps1"}}
	if _, err := Parse(encode(t, m)); err != nil {
		t.Fatalf("installer names refused without an installer: %v", err)
	}
}

func TestInstallerScriptsAndExtraNames(t *testing.T) {
	s := mustParse(t, readFixture(t, "installer.json"))
	if got := s.InstallerScripts(); !reflect.DeepEqual(got, []string{InstallerScript, InstallerPowerShell}) {
		t.Fatalf("InstallerScripts = %v", got)
	}
	names, err := s.ExtraNames("v1.2.3-rc.1")
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"relay-notes-v1.2.3-rc.1.md", "install.sh", "install.ps1"}; !reflect.DeepEqual(names, want) {
		t.Fatalf("ExtraNames = %v, want %v", names, want)
	}
	for _, tc := range []struct {
		platforms []any
		want      []string
	}{
		{[]any{map[string]any{"os": "linux", "arch": "amd64"}}, []string{InstallerScript}},
		{[]any{map[string]any{"os": "darwin", "arch": "arm64"}}, []string{InstallerScript}},
		{[]any{map[string]any{"os": "windows", "arch": "arm64"}}, []string{InstallerPowerShell}},
	} {
		m := base()
		m["platforms"] = tc.platforms
		m["installer"] = map[string]any{}
		spec := mustParse(t, encode(t, m))
		if got := spec.InstallerScripts(); !reflect.DeepEqual(got, tc.want) {
			t.Errorf("%v: InstallerScripts = %v, want %v", tc.platforms, got, tc.want)
		}
		names, err := spec.ExtraNames("v1.0.0")
		if err != nil || !reflect.DeepEqual(names, tc.want) {
			t.Errorf("%v: ExtraNames = %v, %v", tc.platforms, names, err)
		}
	}
}

func TestInstallerDefaults(t *testing.T) {
	m := base()
	m["installer"] = map[string]any{}
	bare := mustParse(t, encode(t, m))
	if got := bare.InstallerName("magic-cli-remote"); got != "magic-cli-remote" {
		t.Errorf("InstallerName = %q", got)
	}
	for repo, want := range map[string]string{
		"magic-cli-remote":      "MAGIC_CLI_REMOTE",
		"mcp-server-recall":     "MCP_SERVER_RECALL",
		"prepare.commit_msg":    "PREPARE_COMMIT_MSG",
		"selfupdate-rehearsal2": "SELFUPDATE_REHEARSAL2",
	} {
		got, err := bare.InstallerEnvPrefix(repo)
		if err != nil || got != want {
			t.Errorf("InstallerEnvPrefix(%q) = %q, %v; want %q", repo, got, err, want)
		}
	}
	for _, repo := range []string{"1password", strings.Repeat("r", 33)} {
		if _, err := bare.InstallerEnvPrefix(repo); err == nil || !strings.Contains(err.Error(), "set installer.env_prefix") {
			t.Errorf("InstallerEnvPrefix(%q): %v", repo, err)
		}
	}

	named := mustParse(t, readFixture(t, "installer.json"))
	if got := named.InstallerName("ignored"); got != "relay-suite" {
		t.Errorf("InstallerName = %q", got)
	}
	if got, err := named.InstallerEnvPrefix("ignored"); err != nil || got != "RELAY" {
		t.Errorf("InstallerEnvPrefix = %q, %v", got, err)
	}
	m = base()
	m["installer"] = map[string]any{"name": "relay-suite"}
	nameOnly := mustParse(t, encode(t, m))
	if got, err := nameOnly.InstallerEnvPrefix("ignored"); err != nil || got != "RELAY_SUITE" {
		t.Errorf("InstallerEnvPrefix from name = %q, %v", got, err)
	}
}

// TestInstallerArgumentCharset: with the installers on, identity_args must
// be safe to embed too; without them, any argument is allowed, as before
// (0014-PLAN deviation D1).
func TestInstallerArgumentCharset(t *testing.T) {
	m := base()
	product(m)["identity_args"] = []any{"version", "--format=it's"}
	if _, err := Parse(encode(t, m)); err != nil {
		t.Fatalf("without the installers, a quote in identity_args was refused: %v", err)
	}
	m["installer"] = map[string]any{}
	if _, err := Parse(encode(t, m)); err == nil || !strings.Contains(err.Error(), "products[0].identity_args[1]") {
		t.Fatalf("with the installers: %v", err)
	}
	m = base()
	product(m)["identity_args"] = []any{"version", "--format=short", "-v", "a.b/c:d=e,f+g@h%i"}
	m["installer"] = map[string]any{"hooks": []any{map[string]any{"when": "after_install", "product": "relay", "args": []any{"configure", "--encrypt-db=true"}}}}
	if _, err := Parse(encode(t, m)); err != nil {
		t.Fatalf("safe arguments refused: %v", err)
	}
}
