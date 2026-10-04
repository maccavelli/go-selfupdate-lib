package systemd

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/maccavelli/go-selfupdate-lib/selfupdate"
)

// dropInName is the drop-in this package owns (0011-MADR §5).
const dropInName = "90-selfupdate.conf"

// DropIn is the ReconcileResult.State of a rewrite: the drop-in written,
// and what was there before, which Restore puts back.
type DropIn struct {
	// Path is the drop-in file.
	Path string
	// Existed reports whether a file was there before.
	Existed bool
	// Previous is its content, when it existed.
	Previous []byte
}

// Reconcile implements selfupdate.Reconciler. The binary is replaced by
// rename at the same path, so the unit already names it: Reconcile checks
// that and changes nothing (0011-MADR §5). A unit that runs another binary
// is an error, unless Options.RewritePath allows a drop-in that points it
// at executable.
func (u *Unit) Reconcile(ctx context.Context, product, executable string) (selfupdate.ReconcileResult, error) {
	unit, err := u.unit(product)
	if err != nil {
		return selfupdate.ReconcileResult{}, err
	}
	p, err := u.show(ctx, unit, "ExecStart", "FragmentPath", "DropInPaths")
	if err != nil {
		return selfupdate.ReconcileResult{}, err
	}
	current := execStartPath(p["ExecStart"])
	if current == "" {
		return selfupdate.ReconcileResult{}, fmt.Errorf("selfupdate: systemd: %s has no ExecStart", unit)
	}
	if filepath.Clean(current) == filepath.Clean(executable) {
		return selfupdate.ReconcileResult{Detail: unit + " runs " + current}, nil
	}
	if !u.o.RewritePath {
		return selfupdate.ReconcileResult{}, fmt.Errorf(
			"selfupdate: systemd: %s runs %s, not %s; Options.RewritePath allows a drop-in", unit, current, executable)
	}
	return u.rewrite(ctx, unit, executable, p)
}

// execStartPath is the program of `show -p ExecStart`'s first command:
// "{ path=/usr/bin/x ; argv[]=/usr/bin/x a ; … }".
func execStartPath(v string) string {
	_, rest, ok := strings.Cut(v, "path=")
	if !ok {
		return ""
	}
	path, _, _ := strings.Cut(rest, " ;")
	return strings.TrimSpace(path)
}

// rewrite writes the drop-in: ExecStart= to reset the list, then the
// unit's effective ExecStart line with its program replaced. The line is
// read from the unit's files, not from `show`, which does not keep
// systemd's quoting of the arguments.
func (u *Unit) rewrite(ctx context.Context, unit, executable string, p properties) (selfupdate.ReconcileResult, error) {
	files := append([]string{p["FragmentPath"]}, strings.Fields(p["DropInPaths"])...)
	line, err := effectiveExecStart(files)
	if err != nil {
		return selfupdate.ReconcileResult{}, err
	}
	newLine, err := replaceProgram(line, executable)
	if err != nil {
		return selfupdate.ReconcileResult{}, err
	}
	dir, err := u.dropInDir(unit)
	if err != nil {
		return selfupdate.ReconcileResult{}, err
	}
	state := DropIn{Path: filepath.Join(dir, dropInName)}
	if prev, err := os.ReadFile(state.Path); err == nil { //nolint:gosec // this package's own drop-in
		state.Existed, state.Previous = true, prev
	} else if !errors.Is(err, os.ErrNotExist) {
		return selfupdate.ReconcileResult{}, err
	}
	body := "# Written by go-selfupdate-lib: the binary moved (0011-MADR §5).\n[Service]\nExecStart=\nExecStart=" + newLine + "\n"
	if err := writeAtomic(state.Path, []byte(body)); err != nil {
		return selfupdate.ReconcileResult{}, err
	}
	result := selfupdate.ReconcileResult{Changed: true, Detail: "drop-in " + state.Path, State: state}
	if err := u.reload(ctx, unit); err != nil {
		return result, err
	}
	return result, nil
}

// Restore implements selfupdate.Reconciler: it puts the drop-in back as it
// was, or removes it, and reloads.
func (u *Unit) Restore(ctx context.Context, product string, receipt selfupdate.ReconcileResult) error {
	if !receipt.Changed {
		return nil
	}
	state, ok := receipt.State.(DropIn)
	if !ok {
		return errors.New("selfupdate: systemd: the receipt is not a systemd drop-in")
	}
	unit, err := u.unit(product)
	if err != nil {
		return err
	}
	if state.Existed {
		err = writeAtomic(state.Path, state.Previous)
	} else {
		err = removeIfExists(state.Path)
	}
	if err != nil {
		return err
	}
	return u.reload(ctx, unit)
}

// reload runs daemon-reload, then checks the unit no longer needs one.
func (u *Unit) reload(ctx context.Context, unit string) error {
	out, err := u.systemctlRun(ctx, "daemon-reload", "--no-ask-password")
	if err != nil {
		return err
	}
	if out.ExitCode != 0 {
		return commandError("daemon-reload", unit, out)
	}
	p, err := u.show(ctx, unit, "NeedDaemonReload")
	if err != nil {
		return err
	}
	if p["NeedDaemonReload"] != "no" {
		return fmt.Errorf("selfupdate: systemd: %s still needs a daemon-reload", unit)
	}
	return nil
}

// dropInDir is <DropInDir or the scope's directory>/<unit>.d.
func (u *Unit) dropInDir(unit string) (string, error) {
	base := u.o.DropInDir
	if base == "" {
		if u.o.Scope == System {
			base = "/etc/systemd/system"
		} else {
			config := u.getenv("XDG_CONFIG_HOME")
			if config == "" {
				home, err := os.UserHomeDir()
				if err != nil {
					return "", err
				}
				config = filepath.Join(home, ".config")
			}
			base = filepath.Join(config, "systemd", "user")
		}
	}
	return filepath.Join(base, unit+".d"), nil
}

// effectiveExecStart reads ExecStart= from the files in order: an empty
// assignment resets the list, and the last command left is the one
// returned. A unit with several commands is refused: the rewrite replaces
// one program.
func effectiveExecStart(files []string) (string, error) {
	var cmds []string
	for _, f := range files {
		if f == "" {
			continue
		}
		raw, err := os.ReadFile(f) //nolint:gosec // a path systemd reported for the unit
		if err != nil {
			return "", err
		}
		for _, v := range serviceDirectives(raw, "ExecStart") {
			if v == "" {
				cmds = nil
			} else {
				cmds = append(cmds, v)
			}
		}
	}
	switch len(cmds) {
	case 0:
		return "", errors.New("selfupdate: systemd: the unit's files hold no ExecStart")
	case 1:
		return cmds[0], nil
	}
	return "", fmt.Errorf("selfupdate: systemd: the unit has %d ExecStart commands; the rewrite replaces one", len(cmds))
}

// serviceDirectives returns the values of key in a unit file's [Service]
// section, in order, joining lines continued with a trailing backslash.
func serviceDirectives(raw []byte, key string) []string {
	var out []string
	inService := false
	var logical strings.Builder
	sc := bufio.NewScanner(bytes.NewReader(raw))
	sc.Buffer(make([]byte, 64<<10), 1<<20)
	flush := func() {
		line := strings.TrimSpace(logical.String())
		logical.Reset()
		if line == "" || line[0] == '#' || line[0] == ';' {
			return
		}
		if line[0] == '[' {
			inService = line == "[Service]"
			return
		}
		if k, v, ok := strings.Cut(line, "="); ok && inService && strings.TrimSpace(k) == key {
			out = append(out, strings.TrimSpace(v))
		}
	}
	for sc.Scan() {
		text := sc.Text()
		if cont, ok := strings.CutSuffix(text, `\`); ok {
			logical.WriteString(cont + " ")
			continue
		}
		logical.WriteString(text)
		flush()
	}
	flush()
	return out
}

// replaceProgram swaps the program in an ExecStart command, keeping its
// prefix characters ("@-:+!") and the rest exactly as written. The program
// is the first word, which may be double-quoted.
func replaceProgram(line, executable string) (string, error) {
	if strings.ContainsAny(executable, "\"\\\n") {
		return "", fmt.Errorf("selfupdate: systemd: %q cannot be written to ExecStart", executable)
	}
	i := 0
	for i < len(line) && strings.ContainsRune("@-:+!|", rune(line[i])) {
		i++
	}
	prefix, rest := line[:i], line[i:]
	var end int
	if strings.HasPrefix(rest, `"`) {
		closing := strings.IndexByte(rest[1:], '"')
		if closing < 0 {
			return "", fmt.Errorf("selfupdate: systemd: unterminated quote in ExecStart=%s", line)
		}
		end = closing + 2
	} else {
		end = strings.IndexAny(rest, " \t")
		if end < 0 {
			end = len(rest)
		}
	}
	return prefix + `"` + executable + `"` + rest[end:], nil
}

// writeAtomic writes body to path by temporary file and rename, mode 0644,
// creating the directory.
func writeAtomic(path string, body []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil { //nolint:gosec // a unit drop-in directory is world-readable, as systemd's are
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	if _, err := tmp.Write(body); err != nil {
		return errors.Join(err, tmp.Close(), os.Remove(name))
	}
	if err := tmp.Sync(); err != nil {
		return errors.Join(err, tmp.Close(), os.Remove(name))
	}
	if err := tmp.Close(); err != nil {
		return errors.Join(err, os.Remove(name))
	}
	if err := os.Chmod(name, 0o644); err != nil { //nolint:gosec // systemd reads unit files as any user may
		return errors.Join(err, os.Remove(name))
	}
	if err := os.Rename(name, path); err != nil {
		return errors.Join(err, os.Remove(name))
	}
	return nil
}
