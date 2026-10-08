package releasespec

import (
	"fmt"
	"regexp"
	"strings"
)

// The installer scripts the build workflow generates
// (docs/decisions/0014-MADR-shared-installer-templates.md §2).
const (
	// InstallerScript is the Unix installer, generated when the spec lists
	// a platform that is not Windows.
	InstallerScript = "install.sh"
	// InstallerPowerShell is the Windows installer, generated when the
	// spec lists a Windows platform.
	InstallerPowerShell = "install.ps1"
)

// The hook times.
const (
	// HookBeforeInstall runs the installed copy, when there is one, before
	// the new binary takes its place.
	HookBeforeInstall = "before_install"
	// HookAfterInstall runs the new copy after the swap and the identity
	// check.
	HookAfterInstall = "after_install"
)

// Installer asks the build workflow to generate install.sh and install.ps1
// for the release. Present, even empty, it turns them on; null is refused,
// not read as absent (0015-MADR E7).
type Installer struct {
	// Name names the Windows install folder,
	// %LOCALAPPDATA%\Programs\<Name>, and the default environment prefix.
	// Empty means the repository's name.
	Name string `json:"name,omitempty"`
	// EnvPrefix is the prefix of the installers' environment variables,
	// such as <EnvPrefix>_VERSION. Empty means Name in upper case, with
	// every character other than a letter or digit as "_".
	EnvPrefix string `json:"env_prefix,omitempty"`
	// Hooks run release products around the install, at most 8.
	Hooks []Hook `json:"hooks,omitempty"`
}

// Hook runs one product of the release with fixed arguments.
type Hook struct {
	// When is HookBeforeInstall or HookAfterInstall.
	When string `json:"when"`
	// Product is a product the spec lists.
	Product string `json:"product"`
	// Args are its arguments, 1 to 16, each non-empty and without NUL.
	Args []string `json:"args"`
}

const maxHooks = 8

var (
	// envPrefixRe is an installer environment prefix.
	envPrefixRe = regexp.MustCompile(`^[A-Z][A-Z0-9_]{0,31}$`)
	// installerArgRe is an argument the installers embed in shell and
	// PowerShell code: hook arguments and identity_args, when the
	// installers are on. It admits no quote, space or expansion
	// character (0014-PLAN deviation D1).
	installerArgRe = regexp.MustCompile(`^[A-Za-z0-9._:=/,+@%-]+$`)
)

func (s Spec) validateInstaller() error {
	in := s.Installer
	if in == nil {
		return nil
	}
	if in.Name != "" && !assetNameRe.MatchString(in.Name) {
		return fmt.Errorf("releasespec: installer.name: %q must match %s", in.Name, assetNameRe)
	}
	if in.EnvPrefix != "" && !envPrefixRe.MatchString(in.EnvPrefix) {
		return fmt.Errorf("releasespec: installer.env_prefix: %q must match %s", in.EnvPrefix, envPrefixRe)
	}
	if len(in.Hooks) > maxHooks {
		return fmt.Errorf("releasespec: installer.hooks: %d listed; at most %d are allowed", len(in.Hooks), maxHooks)
	}
	for i, h := range in.Hooks {
		at := fmt.Sprintf("releasespec: installer.hooks[%d]", i)
		if h.When != HookBeforeInstall && h.When != HookAfterInstall {
			return fmt.Errorf("%s.when: %q is not %q or %q", at, h.When, HookBeforeInstall, HookAfterInstall)
		}
		if _, err := s.Product(h.Product); err != nil {
			return fmt.Errorf("%s.product: %q is not a product of the spec", at, h.Product)
		}
		if len(h.Args) == 0 || len(h.Args) > maxList {
			return fmt.Errorf("%s.args: %d listed; 1 to %d are allowed", at, len(h.Args), maxList)
		}
		for j, arg := range h.Args {
			if !installerArgRe.MatchString(arg) {
				return fmt.Errorf("%s.args[%d]: %q must match %s, as the installers embed it", at, j, arg, installerArgRe)
			}
		}
	}
	for i, p := range s.Products {
		for j, arg := range p.IdentityArgs {
			if !installerArgRe.MatchString(arg) {
				return fmt.Errorf("releasespec: products[%d].identity_args[%d]: %q must match %s, as the installers embed it", i, j, arg, installerArgRe)
			}
		}
	}
	return nil
}

// InstallerScripts returns the installers the build workflow generates for
// the spec: InstallerScript when a platform is not Windows, then
// InstallerPowerShell when one is. It returns nil without Installer.
func (s Spec) InstallerScripts() []string {
	if s.Installer == nil {
		return nil
	}
	var unix, windows bool
	for _, p := range s.Platforms {
		if p.OS == goosWindows {
			windows = true
		} else {
			unix = true
		}
	}
	var out []string
	if unix {
		out = append(out, InstallerScript)
	}
	if windows {
		out = append(out, InstallerPowerShell)
	}
	return out
}

// InstallerName returns Installer.Name, or repo, the repository's name,
// when it is empty.
func (s Spec) InstallerName(repo string) string {
	if s.Installer != nil && s.Installer.Name != "" {
		return s.Installer.Name
	}
	return repo
}

// InstallerEnvPrefix returns Installer.EnvPrefix, or the prefix made from
// InstallerName(repo): upper case, with every character other than a
// letter or digit as "_". A made prefix that is not a valid one, such as
// one that starts with a digit or is longer than 32, is an error naming
// the field to set.
func (s Spec) InstallerEnvPrefix(repo string) (string, error) {
	if s.Installer != nil && s.Installer.EnvPrefix != "" {
		return s.Installer.EnvPrefix, nil
	}
	name := s.InstallerName(repo)
	var b strings.Builder
	for _, r := range strings.ToUpper(name) {
		if (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		} else {
			b.WriteByte('_')
		}
	}
	prefix := b.String()
	if !envPrefixRe.MatchString(prefix) {
		return "", fmt.Errorf("releasespec: installer.env_prefix: the prefix made from %q, %q, must match %s; set installer.env_prefix", name, prefix, envPrefixRe)
	}
	return prefix, nil
}
