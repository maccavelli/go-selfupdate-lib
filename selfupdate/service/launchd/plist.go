package launchd

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"strings"

	"github.com/maccavelli/go-selfupdate-lib/selfupdate/service"
)

// plistValue reads key from the job's property list with plutil. It first
// converts the list to XML: plutil -lint passes a file holding a bare word,
// a valid old-style property list but no job, so the root must be a
// dictionary (0015-MADR amendment A1, the D7 probe evidence). It returns
// the value's type as plutil -type names it, the raw form of a bool or an
// integer, and whether the key is present. A missing file wraps
// service.ErrNotInstalled; an unreadable or invalid one is an error.
func (j *Job) plistValue(ctx context.Context, key string) (typ, raw string, present bool, err error) {
	if _, err := statFile(j.o.Plist); errors.Is(err, fs.ErrNotExist) {
		return "", "", false, fmt.Errorf("%w: %s", service.ErrNotInstalled, j.o.Plist)
	}
	out, err := j.plutilRun(ctx, "-convert", "xml1", "-o", "-", "--", j.o.Plist)
	if err != nil {
		return "", "", false, err
	}
	if out.ExitCode != 0 {
		return "", "", false, fmt.Errorf("selfupdate: launchd: %s is not a readable property list: plutil exit %d: %s",
			j.o.Plist, out.ExitCode, strings.TrimSpace(string(out.Stderr)))
	}
	if !dictRoot(out.Stdout) {
		return "", "", false, fmt.Errorf("selfupdate: launchd: %s is not a property-list dictionary", j.o.Plist)
	}
	out, err = j.plutilRun(ctx, "-type", key, "--", j.o.Plist)
	if err != nil {
		return "", "", false, err
	}
	if out.ExitCode != 0 {
		// The list is readable, so exit 1 is "No value at that key path".
		return "", "", false, nil
	}
	typ = strings.TrimSpace(string(out.Stdout))
	if typ != "bool" && typ != "integer" {
		return typ, "", true, nil
	}
	out, err = j.plutilRun(ctx, "-extract", key, "raw", "-o", "-", "--", j.o.Plist)
	if err != nil {
		return "", "", false, err
	}
	if out.ExitCode != 0 {
		return "", "", false, fmt.Errorf("selfupdate: launchd: plutil -extract %s: exit %d: %s",
			key, out.ExitCode, strings.TrimSpace(string(out.Stderr)))
	}
	return typ, strings.TrimSpace(string(out.Stdout)), true, nil
}

func (j *Job) plutilRun(ctx context.Context, args ...string) (service.Output, error) {
	return j.o.Runner.Run(ctx, service.Command{Path: j.plutil, Args: args, Env: env})
}

// dictRoot reports whether plutil's XML has a dictionary at its root: the
// first element after <plist ...>.
func dictRoot(xml []byte) bool {
	s := string(xml)
	i := strings.Index(s, "<plist")
	if i < 0 {
		return false
	}
	end := strings.Index(s[i:], ">")
	if end < 0 {
		return false
	}
	rest := strings.TrimSpace(s[i+end+1:])
	return strings.HasPrefix(rest, "<dict>") || strings.HasPrefix(rest, "<dict/>")
}
