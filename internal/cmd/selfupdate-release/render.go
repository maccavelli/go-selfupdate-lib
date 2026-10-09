package main

import (
	"bytes"
	"embed"
	"fmt"
	"regexp"
	"strings"

	"github.com/maccavelli/go-selfupdate-lib/selfupdate/releasespec"
)

// The installer templates (docs/decisions/0014-MADR-shared-installer-templates.md
// §3): complete scripts whose values block the renderer replaces.
//
//go:embed installer/install.sh installer/install.ps1
var installerTemplates embed.FS

const (
	blockStart = "# >>> selfupdate-release values"
	blockEnd   = "# <<< selfupdate-release values"
)

var (
	// repositoryRe is an owner/name pair.
	repositoryRe = regexp.MustCompile(`^[A-Za-z0-9-]+/[A-Za-z0-9._-]+$`)
	// safeValueRe is every value the renderer writes between quotes: no
	// quote, space, newline or expansion character can reach the scripts.
	safeValueRe = regexp.MustCompile(`^[A-Za-z0-9._:=/,+@%-]+$`)
)

// asset is one product on one platform, as the installers see it.
type asset struct {
	product, os, arch, name, format string
}

// installerValues are what the templates' values blocks hold.
type installerValues struct {
	repository, tag, name, envPrefix string
	channels, products               []string
	assets                           []asset
	identity                         []releasespec.Product // with IdentityArgs
	hooks                            []releasespec.Hook
}

func newInstallerValues(spec releasespec.Spec, repository, tag string) (installerValues, error) {
	if !repositoryRe.MatchString(repository) {
		return installerValues{}, usagef("-repository %q must be owner/name", repository)
	}
	if !nameRe.MatchString(tag) {
		return installerValues{}, usagef("tag %q must match %s", tag, nameRe)
	}
	repoName := repository[strings.Index(repository, "/")+1:]
	prefix, err := spec.InstallerEnvPrefix(repoName)
	if err != nil {
		return installerValues{}, err
	}
	v := installerValues{
		repository: repository, tag: tag, name: spec.InstallerName(repoName), envPrefix: prefix,
		channels: spec.PrereleaseChannels,
	}
	for _, prod := range spec.Products {
		v.products = append(v.products, prod.Name)
		if len(prod.IdentityArgs) > 0 {
			v.identity = append(v.identity, prod)
		}
		for _, p := range spec.Targets() {
			name, err := spec.AssetName(prod.Name, p)
			if err != nil {
				return installerValues{}, err
			}
			format := "binary"
			if f, ok := spec.FormatFor(p); ok {
				format = string(f)
			}
			v.assets = append(v.assets, asset{product: prod.Name, os: p.OS, arch: p.Arch, name: name, format: format})
		}
	}
	if spec.Installer != nil {
		v.hooks = spec.Installer.Hooks
	}
	return v, v.check()
}

// check refuses any value that is not safe to write between single quotes,
// whatever validated it before.
func (v installerValues) check() error {
	values := []string{v.repository, v.tag, v.name, v.envPrefix}
	values = append(values, v.channels...)
	values = append(values, v.products...)
	for _, a := range v.assets {
		values = append(values, a.product, a.os, a.arch, a.name, a.format)
	}
	for _, p := range v.identity {
		values = append(values, p.IdentityArgs...)
	}
	for _, h := range v.hooks {
		values = append(values, h.When, h.Product)
		values = append(values, h.Args...)
	}
	for _, s := range values {
		if !safeValueRe.MatchString(s) {
			return fmt.Errorf("installer value %q must match %s", s, safeValueRe)
		}
	}
	return nil
}

func (v installerValues) shBlock() []string {
	var assets []string
	for _, a := range v.assets {
		if a.os != goosWindows {
			assets = append(assets, strings.Join([]string{a.product, a.os, a.arch, a.name, a.format}, " "))
		}
	}
	var identity []string
	for _, p := range v.identity {
		identity = append(identity, p.Name+" "+strings.Join(p.IdentityArgs, " "))
	}
	var hooks []string
	for _, h := range v.hooks {
		hooks = append(hooks, h.When+" "+h.Product+" "+strings.Join(h.Args, " "))
	}
	return []string{
		"REPOSITORY='" + v.repository + "'",
		"TAG='" + v.tag + "'",
		"CHANNELS='" + strings.Join(v.channels, " ") + "'",
		"INSTALLER_NAME='" + v.name + "'",
		"ENV_PREFIX='" + v.envPrefix + "'",
		"PRODUCTS='" + strings.Join(v.products, " ") + "'",
		"ASSETS='" + strings.Join(assets, "\n") + "'",
		"IDENTITY='" + strings.Join(identity, "\n") + "'",
		"HOOKS='" + strings.Join(hooks, "\n") + "'",
	}
}

// psList is a PowerShell array of quoted strings.
func psList(items []string) string {
	quoted := make([]string, len(items))
	for i, s := range items {
		quoted[i] = "'" + s + "'"
	}
	return "@(" + strings.Join(quoted, ", ") + ")"
}

func (v installerValues) psBlock() []string {
	lines := []string{
		"$Repository = '" + v.repository + "'",
		"$Tag = '" + v.tag + "'",
		"$Channels = " + psList(v.channels),
		"$InstallerName = '" + v.name + "'",
		"$EnvPrefix = '" + v.envPrefix + "'",
		"$Products = " + psList(v.products),
		"$Assets = @(",
	}
	for _, a := range v.assets {
		if a.os == goosWindows {
			lines = append(lines, fmt.Sprintf("    @{ Product = '%s'; Arch = '%s'; Name = '%s'; Format = '%s' }", a.product, a.arch, a.name, a.format))
		}
	}
	lines = append(lines, ")")
	identity := make([]string, len(v.identity))
	for i, p := range v.identity {
		identity[i] = "'" + p.Name + "' = " + psList(p.IdentityArgs)
	}
	lines = append(lines, "$Identity = @{ "+strings.Join(identity, "; ")+" }", "$Hooks = @(")
	for _, h := range v.hooks {
		lines = append(lines, fmt.Sprintf("    @{ When = '%s'; Product = '%s'; Args = %s }", h.When, h.Product, psList(h.Args)))
	}
	return append(lines, ")")
}

// replaceBlock replaces the lines between the one start marker and the one
// end marker of template with body, each line indented as the start
// marker is. The markers stay.
func replaceBlock(template []byte, body []string) ([]byte, error) {
	if bytes.Contains(template, []byte("\r")) {
		return nil, fmt.Errorf("the template has CR line endings")
	}
	lines := strings.Split(string(template), "\n")
	start, end := -1, -1
	for i, l := range lines {
		switch strings.TrimSpace(l) {
		case blockStart:
			if start >= 0 {
				return nil, fmt.Errorf("the template has two value blocks")
			}
			start = i
		case blockEnd:
			if end >= 0 {
				return nil, fmt.Errorf("the template has two value block ends")
			}
			end = i
		}
	}
	if start < 0 || end < start {
		return nil, fmt.Errorf("the template has no value block")
	}
	indent := lines[start][:len(lines[start])-len(strings.TrimLeft(lines[start], " "))]
	out := append([]string{}, lines[:start+1]...)
	out = append(out, indent+"# Generated by go-selfupdate-lib's build workflow from the release spec.")
	for _, b := range body {
		for l := range strings.SplitSeq(b, "\n") {
			out = append(out, indent+l)
		}
	}
	out = append(out, lines[end:]...)
	return []byte(strings.Join(out, "\n")), nil
}

// renderInstallers returns the spec's installers, by file name, rendered
// for the repository and tag. It returns nothing for a spec without
// installer.
func renderInstallers(spec releasespec.Spec, repository, tag string) (map[string][]byte, error) {
	scripts := spec.InstallerScripts()
	if len(scripts) == 0 {
		return nil, nil
	}
	v, err := newInstallerValues(spec, repository, tag)
	if err != nil {
		return nil, err
	}
	out := map[string][]byte{}
	for _, name := range scripts {
		template, err := installerTemplates.ReadFile("installer/" + name)
		if err != nil {
			return nil, err
		}
		template = bytes.TrimPrefix(template, []byte("\xef\xbb\xbf"))
		body := v.shBlock()
		if name == releasespec.InstallerPowerShell {
			body = v.psBlock()
		}
		rendered, err := replaceBlock(template, body)
		if err != nil {
			return nil, fmt.Errorf("installer/%s: %w", name, err)
		}
		out[name] = rendered
	}
	return out, nil
}
