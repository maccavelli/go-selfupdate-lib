package systemd

import (
	"errors"
	"fmt"
	"strings"
)

// maxUnitName is the longest unit name systemd accepts (systemd.unit(5)).
const maxUnitName = 255

// ValidUnitName checks name against systemd.unit(5)'s grammar, for the
// service units this package manages: a name of ASCII letters, digits and
// ":-_.\", ending ".service", at most 255 bytes, with "@" only as a
// template's separator, directly before an instance or the suffix. A name
// may not begin with "-".
func ValidUnitName(name string) error {
	if len(name) > maxUnitName {
		return fmt.Errorf("selfupdate: systemd: unit name is longer than %d bytes", maxUnitName)
	}
	prefix, ok := strings.CutSuffix(name, ".service")
	if !ok || prefix == "" {
		return fmt.Errorf("selfupdate: systemd: unit %q is not a .service unit", name)
	}
	if strings.HasPrefix(prefix, "-") {
		return fmt.Errorf("selfupdate: systemd: unit %q begins with -", name)
	}
	base, instance, templated := strings.Cut(prefix, "@")
	if base == "" || strings.Contains(instance, "@") {
		return fmt.Errorf("selfupdate: systemd: unit %q has a misplaced @", name)
	}
	if templated && instance == "" {
		return fmt.Errorf("selfupdate: systemd: unit %q is a template; name an instance (TemplateInstance)", name)
	}
	for _, part := range []string{base, instance} {
		for i := 0; i < len(part); i++ {
			if !unitChar(part[i]) {
				return fmt.Errorf("selfupdate: systemd: unit %q has the character %q", name, part[i])
			}
		}
	}
	return nil
}

func unitChar(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' ||
		c == ':' || c == '-' || c == '_' || c == '.' || c == '\\'
}

// Escape escapes s for use in a unit name, by systemd.unit(5)'s "String
// Escaping": "/" becomes "-", and every byte that is not an ASCII letter
// or digit, ":", "_" or ".", and a leading ".", becomes \xNN. It is what
// systemd-escape does without --path.
func Escape(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c == '/':
			b.WriteByte('-')
		case c == '.' && i == 0:
			fmt.Fprintf(&b, `\x%02x`, c)
		case c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == ':' || c == '_' || c == '.':
			b.WriteByte(c)
		default:
			fmt.Fprintf(&b, `\x%02x`, c)
		}
	}
	return b.String()
}

// TemplateInstance names template's instance for instance, escaped:
// TemplateInstance("relay@.service", "eu/1") is "relay@eu-1.service".
func TemplateInstance(template, instance string) (string, error) {
	base, ok := strings.CutSuffix(template, "@.service")
	if !ok || base == "" {
		return "", fmt.Errorf("selfupdate: systemd: %q is not a template such as name@.service", template)
	}
	if instance == "" {
		return "", errors.New("selfupdate: systemd: a template instance needs a name")
	}
	name := base + "@" + Escape(instance) + ".service"
	return name, ValidUnitName(name)
}
