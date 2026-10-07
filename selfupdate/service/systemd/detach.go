package systemd

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/maccavelli/go-selfupdate-lib/selfupdate/service"
)

// readSelfCgroup is os.ReadFile("/proc/self/cgroup"), replaced in tests.
var readSelfCgroup = func() ([]byte, error) { return os.ReadFile("/proc/self/cgroup") }

// Inside implements service.Detacher: this process's cgroup is the unit's
// ControlGroup or below it, so stopping the unit would kill it
// (systemd.kill(5), KillMode=control-group). $INVOCATION_ID alone is not
// used: an agent's session can clear its environment (0011-MADR §3). It
// needs Options.Unit.
func (u *Unit) Inside(ctx context.Context) (bool, error) {
	if u.o.Unit == "" {
		return false, errors.New("selfupdate: systemd: the handoff needs Options.Unit")
	}
	return u.insideUnit(ctx, u.o.Unit)
}

func (u *Unit) insideUnit(ctx context.Context, unit string) (bool, error) {
	raw, err := readSelfCgroup()
	if err != nil {
		return false, fmt.Errorf("selfupdate: systemd: read this process's cgroup: %w", err)
	}
	self, err := parseCgroup(raw)
	if err != nil {
		return false, err
	}
	p, err := u.show(ctx, unit, "ControlGroup")
	if err != nil {
		return false, err
	}
	cg := p["ControlGroup"]
	if cg == "" {
		return false, nil
	}
	return self == cg || strings.HasPrefix(self, cg+"/"), nil
}

// parseCgroup finds this process's systemd cgroup in /proc/self/cgroup:
// the unified hierarchy's "0::" line, or cgroup v1's name=systemd line.
func parseCgroup(raw []byte) (string, error) {
	var v1 string
	sc := bufio.NewScanner(bytes.NewReader(raw))
	for sc.Scan() {
		line := sc.Text()
		if path, ok := strings.CutPrefix(line, "0::"); ok {
			return path, nil
		}
		if _, path, ok := strings.Cut(line, ":name=systemd:"); ok {
			v1 = path
		}
	}
	if v1 != "" {
		return v1, nil
	}
	return "", errors.New("selfupdate: systemd: /proc/self/cgroup names no systemd cgroup")
}

// Minimum systemd versions for the handoff: --collect arrived in 236, and
// Type=exec, which lets the environment file go once the run has started,
// in 240.
const (
	minHandOffVersion = 236
	typeExecVersion   = 240
)

// Detach implements service.Detacher. It starts spec as a transient
// service, <unit>-selfupdate-<id>.service, whose parent is the service
// manager, so stopping the unit does not kill it (systemd-run(1),
// 0011-MADR §9). The environment goes in a 0600 file, never on the command
// line, where `systemctl show` would publish it (0011-MADR amendment A2).
func (u *Unit) Detach(ctx context.Context, spec service.HandOff) (service.Detached, error) {
	if u.o.Unit == "" {
		return service.Detached{}, errors.New("selfupdate: systemd: the handoff needs Options.Unit")
	}
	version, err := u.version(ctx)
	if err != nil {
		return service.Detached{}, err
	}
	if version < minHandOffVersion {
		return service.Detached{}, fmt.Errorf("%w: the handoff needs systemd %d or later, this is %d",
			service.ErrUnsupported, minHandOffVersion, version)
	}
	name, err := transientName(u.o.Unit, spec.ID)
	if err != nil {
		return service.Detached{}, err
	}
	envFile, err := u.writeEnvFile(spec)
	if err != nil {
		return service.Detached{}, err
	}
	args := []string{"--unit=" + name, "--collect", "--quiet",
		"--description=selfupdate handoff for " + u.o.Unit, "-p", "EnvironmentFile=" + envFile}
	if u.o.Scope == User {
		args = append([]string{"--user"}, args...)
	}
	waits := version >= typeExecVersion
	if waits {
		// systemd-run returns once the program has started, so the file
		// can go.
		args = append(args, "-p", "Type=exec")
	} else {
		args = append(args, "--no-block")
	}
	args = append(append(args, "--", spec.Executable), spec.Args...)
	out, err := u.o.Runner.Run(ctx, service.Command{Path: u.run, Args: args, Env: u.env})
	if err == nil && out.ExitCode != 0 {
		err = commandError("systemd-run", name, out)
	}
	if waits || err != nil {
		err = errors.Join(err, removeIfExists(envFile))
	}
	if err != nil {
		return service.Detached{}, err
	}
	return service.Detached{ID: spec.ID, Where: "transient unit " + name, ResultPath: spec.ResultPath}, nil
}

// version reads the systemd version from `systemctl --version`'s first
// line, "systemd 255 (255.4-1ubuntu8)".
func (u *Unit) version(ctx context.Context) (int, error) {
	out, err := u.o.Runner.Run(ctx, service.Command{Path: u.systemctl, Args: []string{"--version"}, Env: u.env})
	if err != nil {
		return 0, err
	}
	first, _, _ := strings.Cut(string(out.Stdout), "\n")
	fields := strings.Fields(first)
	if out.ExitCode != 0 || len(fields) < 2 || fields[0] != "systemd" {
		return 0, fmt.Errorf("selfupdate: systemd: unrecognised systemctl --version output %q", first)
	}
	n, err := strconv.Atoi(fields[1])
	if err != nil {
		return 0, fmt.Errorf("selfupdate: systemd: unrecognised systemd version %q", fields[1])
	}
	return n, nil
}

// transientName is <unit>-selfupdate-<id>.service, with a template's "@"
// made "-" so the name is not read as an instance.
func transientName(unit, id string) (string, error) {
	base := strings.ReplaceAll(strings.TrimSuffix(unit, ".service"), "@", "-")
	name := base + "-selfupdate-" + id + ".service"
	return name, ValidUnitName(name)
}

// envDir is where the environment files live: a private directory on
// tmpfs.
func (u *Unit) envDir() (string, error) {
	if u.o.Scope == User {
		runtimeDir := u.getenv("XDG_RUNTIME_DIR")
		if runtimeDir == "" {
			return "", errors.New("selfupdate: systemd: user scope needs XDG_RUNTIME_DIR")
		}
		return filepath.Join(runtimeDir, "selfupdate"), nil
	}
	return "/run/selfupdate", nil
}

// writeEnvFile writes this process's environment, then spec.Env, to a new
// 0600 file in the unit's own 0700 directory, <envDir>/<unit>, removing the
// files this unit's earlier handoffs left. Another unit's are not this
// one's to remove: one may still be starting (0015-MADR D8). Files an
// earlier release left directly in <envDir> are left alone.
func (u *Unit) writeEnvFile(spec service.HandOff) (path string, err error) {
	top, err := u.envDir()
	if err != nil {
		return "", err
	}
	if err := privateDir(top); err != nil {
		return "", err
	}
	dir := filepath.Join(top, u.o.Unit)
	if err := privateDir(dir); err != nil {
		return "", err
	}
	if old, err := os.ReadDir(dir); err == nil {
		for _, e := range old {
			name := e.Name()
			if !strings.HasPrefix(name, "handoff-") || !strings.HasSuffix(name, ".env") {
				continue
			}
			// Best-effort: a file this unit's earlier handoff left is no
			// one's.
			if err := os.Remove(filepath.Join(dir, name)); err != nil {
				continue
			}
		}
	}
	path = filepath.Join(dir, "handoff-"+spec.ID+".env")
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600) //nolint:gosec // a fixed name in a private directory
	if err != nil {
		return "", err
	}
	if _, err := f.Write(envFileBody(append(os.Environ(), spec.Env...))); err != nil {
		return "", errors.Join(err, f.Close(), os.Remove(path))
	}
	if err := f.Close(); err != nil {
		return "", errors.Join(err, os.Remove(path))
	}
	return path, nil
}

// privateDir makes dir, or checks it: a real directory of mode 0700.
func privateDir(dir string) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	info, err := os.Lstat(dir)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("selfupdate: systemd: %s is not a directory", dir)
	}
	if info.Mode().Perm() != 0o700 {
		return os.Chmod(dir, 0o700) //nolint:gosec // a directory: 0700 is the private mode
	}
	return nil
}

// envFileBody writes vars, the last of each name winning, in the
// EnvironmentFile format: NAME="value" with \, ", ` and $ escaped. A name
// that is not a variable name is skipped (0011-MADR amendment A2).
func envFileBody(vars []string) []byte {
	order := []string{}
	values := map[string]string{}
	for _, kv := range vars {
		k, v, ok := strings.Cut(kv, "=")
		if !ok || !validEnvName(k) {
			continue
		}
		if _, seen := values[k]; !seen {
			order = append(order, k)
		}
		values[k] = v
	}
	var b bytes.Buffer
	for _, k := range order {
		b.WriteString(k)
		b.WriteString(`="`)
		for _, r := range values[k] {
			switch r {
			case '\\', '"', '`', '$':
				b.WriteByte('\\')
			}
			b.WriteRune(r)
		}
		b.WriteString("\"\n")
	}
	return b.Bytes()
}

func validEnvName(k string) bool {
	if k == "" || k[0] >= '0' && k[0] <= '9' {
		return false
	}
	for i := 0; i < len(k); i++ {
		c := k[i]
		if c != '_' && (c < 'a' || c > 'z') && (c < 'A' || c > 'Z') && (c < '0' || c > '9') {
			return false
		}
	}
	return true
}

func removeIfExists(path string) error {
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}
