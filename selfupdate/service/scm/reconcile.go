package scm

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/maccavelli/go-selfupdate-lib/selfupdate"
	"github.com/maccavelli/go-selfupdate-lib/selfupdate/service"
)

// statFile is os.Stat, replaced in tests.
var statFile = os.Stat

// PathBackup is the ReconcileResult.State of a rewrite: the service's
// command line as it was, which Restore puts back.
type PathBackup struct {
	// Name is the service.
	Name string `json:"name"`
	// Previous is its BinaryPathName before the rewrite.
	Previous string `json:"previous"`
}

// Reconcile implements selfupdate.Reconciler. The binary is replaced by
// rename at the same path, so the service already names it: Reconcile
// compares the command line's program with executable, cleaned and
// case-insensitively, and changes nothing (0011-MADR §5). A service that
// runs another binary is an error, unless Options.RewritePath allows
// pointing it at executable, keeping its arguments. A path holding " is
// refused. An unquoted command line whose program path holds a space is
// read as CreateProcess reads it; a rewrite refuses one whose reading
// depends on which files exist (0015-MADR D5).
func (s *Service) Reconcile(_ context.Context, product, executable string) (_ selfupdate.ReconcileResult, err error) {
	name, err := s.name(product)
	if err != nil {
		return selfupdate.ReconcileResult{}, err
	}
	access := uint32(accessQueryConfig)
	if s.o.RewritePath {
		access |= accessChangeConfig
	}
	h, err := s.open(name, access)
	if err != nil {
		return selfupdate.ReconcileResult{}, err
	}
	defer func() { err = errors.Join(err, h.close()) }()
	c, err := h.config()
	if err != nil {
		return selfupdate.ReconcileResult{}, mapError("query the configuration of", name, err)
	}
	args, err := s.programAndArgs(c.BinaryPathName, executable)
	if err != nil {
		return selfupdate.ReconcileResult{}, fmt.Errorf("selfupdate: scm: %s: unreadable command line %q: %w", name, c.BinaryPathName, err)
	}
	if samePath(args[0], executable) {
		return selfupdate.ReconcileResult{Detail: name + " already runs " + executable}, nil
	}
	if !s.o.RewritePath {
		return selfupdate.ReconcileResult{}, fmt.Errorf("selfupdate: scm: %s runs %s, not %s; Options.RewritePath allows rewriting it", name, args[0], executable)
	}
	if strings.Contains(executable, `"`) {
		return selfupdate.ReconcileResult{}, fmt.Errorf("selfupdate: scm: %q holds a quote, which a command line cannot carry", executable)
	}
	line := s.m.compose(append([]string{executable}, args[1:]...))
	if err := h.setBinaryPathName(line); err != nil {
		return selfupdate.ReconcileResult{}, mapError("change the command line of", name, err)
	}
	return selfupdate.ReconcileResult{
		Changed: true,
		Detail:  name + " now runs " + executable,
		State:   PathBackup{Name: name, Previous: c.BinaryPathName},
	}, nil
}

// Restore puts back the command line a rewrite replaced.
func (s *Service) Restore(_ context.Context, _ string, receipt selfupdate.ReconcileResult) error {
	if !receipt.Changed {
		return nil
	}
	b, ok := receipt.State.(PathBackup)
	if !ok {
		return errors.New("selfupdate: scm: the receipt is not a path backup")
	}
	h, err := s.open(b.Name, accessChangeConfig)
	if err != nil {
		return err
	}
	return errors.Join(mapErrorOrNil("restore the command line of", b.Name, h.setBinaryPathName(b.Previous)), h.close())
}

// programAndArgs splits a service's command line. An unquoted line that
// starts with executable and a space is read as executable and the rest,
// as the SCM itself resolves it: CommandLineToArgv would split the path at
// its first space.
func (s *Service) programAndArgs(line, executable string) ([]string, error) {
	if !strings.HasPrefix(line, `"`) && samePath(line, executable) {
		return []string{line}, nil
	}
	if !strings.HasPrefix(line, `"`) && len(line) > len(executable) &&
		samePath(line[:len(executable)], executable) && line[len(executable)] == ' ' {
		rest, err := s.m.decompose(line[len(executable)+1:])
		if err != nil {
			return nil, err
		}
		return append([]string{line[:len(executable)]}, rest...), nil
	}
	if !strings.HasPrefix(line, `"`) && strings.ContainsAny(line, " \t") {
		program, rest, err := s.unquotedProgram(line)
		if err != nil {
			return nil, err
		}
		args, err := s.m.decompose(rest)
		if err != nil {
			return nil, err
		}
		return append([]string{program}, args...), nil
	}
	args, err := s.m.decompose(line)
	if err == nil && len(args) == 0 {
		err = errors.New("empty")
	}
	return args, err
}

// unquotedProgram reads the program of an unquoted command line whose path
// may hold spaces, as CreateProcess does: each prefix ending at a space,
// then the whole line, with .exe appended when it has no extension; the
// first that is an existing file is the program. When none exists, the
// first prefix ending in .exe is. Otherwise, and under RewritePath when
// the two readings differ, it refuses: a rewrite must not guess
// (0015-MADR D5).
func (s *Service) unquotedProgram(line string) (program, rest string, err error) {
	type split struct{ program, rest string }
	var cands []split
	for i, r := range line {
		if r == ' ' || r == '\t' {
			cands = append(cands, split{line[:i], line[i+1:]})
		}
	}
	cands = append(cands, split{line, ""})
	var exists, dotExe *split
	for i := range cands {
		c := &cands[i]
		name := c.program
		if !hasExtension(name) {
			name += ".exe"
		}
		if exists == nil {
			if info, err := statFile(name); err == nil && info.Mode().IsRegular() {
				exists = c
			}
		}
		if dotExe == nil && strings.HasSuffix(strings.ToLower(c.program), ".exe") {
			dotExe = c
		}
	}
	switch {
	case exists != nil && dotExe != nil && exists.program != dotExe.program && s.o.RewritePath:
		return "", "", fmt.Errorf("the unquoted command line %q could run %s or %s; quote it", line, exists.program, dotExe.program)
	case exists != nil:
		return exists.program, exists.rest, nil
	case dotExe != nil:
		return dotExe.program, dotExe.rest, nil
	}
	return "", "", fmt.Errorf("cannot tell where the program ends in the unquoted command line %q; quote it", line)
}

// hasExtension reports a dot in p's last element, by either separator.
func hasExtension(p string) bool {
	return strings.Contains(p[strings.LastIndexAny(p, `\/`)+1:], ".")
}

// samePath compares two Windows paths, cleaned and case-insensitively, then
// by file identity, which matches an 8.3 short name (service.SameExecutable,
// 0011-MADR amendment A5).
func samePath(a, b string) bool {
	return strings.EqualFold(filepath.Clean(a), filepath.Clean(b)) || service.SameExecutable(a, b)
}

func mapErrorOrNil(step, name string, err error) error {
	if err == nil {
		return nil
	}
	return mapError(step, name, err)
}
