package scm

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/maccavelli/go-selfupdate-lib/selfupdate"
)

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
// refused.
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
	args, err := s.m.decompose(line)
	if err == nil && len(args) == 0 {
		err = errors.New("empty")
	}
	return args, err
}

// samePath compares two Windows paths, cleaned and case-insensitively.
func samePath(a, b string) bool {
	return strings.EqualFold(filepath.Clean(a), filepath.Clean(b))
}

func mapErrorOrNil(step, name string, err error) error {
	if err == nil {
		return nil
	}
	return mapError(step, name, err)
}
