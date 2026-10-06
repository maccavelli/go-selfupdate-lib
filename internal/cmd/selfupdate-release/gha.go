package main

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"strings"
)

// output is one GitHub Actions step output.
type output struct{ name, value string }

// writeOutputs appends outputs to the GITHUB_OUTPUT file at path, each as a
// name<<DELIM block with a random delimiter, so a value can hold newlines
// and can never end its block early. An empty path writes nothing.
func writeOutputs(path string, outs []output) error {
	if path == "" {
		return nil
	}
	var b strings.Builder
	for _, o := range outs {
		delim, err := delimiter()
		if err != nil {
			return err
		}
		if strings.Contains(o.value, delim) {
			return fmt.Errorf("output %s holds its delimiter", o.name)
		}
		fmt.Fprintf(&b, "%s<<%s\n%s\n%s\n", o.name, delim, o.value, delim)
	}
	return appendFile(path, b.String())
}

func delimiter() (string, error) {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	return "ghadelimiter_" + hex.EncodeToString(raw[:]), nil
}

// appendSummary appends Markdown to the step summary at path. An empty
// path writes nothing.
func appendSummary(path, markdown string) error {
	if path == "" {
		return nil
	}
	return appendFile(path, markdown)
}

func appendFile(path, text string) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0o644) //nolint:gosec // the runner's own output and summary files
	if err != nil {
		return err
	}
	if _, err := f.WriteString(text); err != nil {
		return errors.Join(err, f.Close())
	}
	return f.Close()
}
