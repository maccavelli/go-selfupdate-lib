// Command selfupdate-note signs a release's SHA256SUMS with a C2SP signed
// note that selfupdate/verify/signednote checks, and makes the key pair it
// signs with (docs/decisions/0025-MADR-align-with-go-sdk-design-practices.md
// D10). It is never released: publish-selfupdate-release.yml runs it from
// the called workflow's commit, and an owner runs keygen on their own
// machine.
//
// Usage:
//
//	selfupdate-note keygen -name NAME -out FILE
//	selfupdate-note sign   -dir DIR -manifest NAME -repository HOST/OWNER/NAME -tag TAG -products-json JSON -verifier-key KEY -out FILE
//
// keygen writes the signer key to FILE, mode 0600, and prints the verifier
// key. sign reads the signer key from SELFUPDATE_NOTE_KEY, never from a
// flag, signs the statement for DIR/NAME, opens the note it made with KEY
// so a key that does not match fails here, and writes FILE. Neither
// overwrites a file, and neither prints the signer key.
//
// It exits 0 on success, 1 when a check fails, and 2 on a usage error.
package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"

	"golang.org/x/mod/sumdb/note"

	"github.com/maccavelli/go-selfupdate-lib/selfupdate/verify/signednote"
)

// keyEnv names the variable sign reads the signer key from.
const keyEnv = "SELFUPDATE_NOTE_KEY"

func main() {
	os.Exit(run(context.Background(), os.Args[1:], os.Getenv, os.Stdout, os.Stderr))
}

// usageError is a mistake in the command line, reported with exit 2.
type usageError struct{ msg string }

func (e usageError) Error() string { return e.msg }

func usagef(format string, args ...any) error {
	return usageError{fmt.Sprintf(format, args...)}
}

func run(_ context.Context, args []string, getenv func(string) string, stdout, stderr io.Writer) int {
	var err error
	switch {
	case len(args) == 0:
		err = usagef("usage: selfupdate-note keygen|sign [flags]")
	case args[0] == "keygen":
		err = keygen(args[1:], stdout)
	case args[0] == "sign":
		err = sign(args[1:], getenv)
	default:
		err = usagef("unknown subcommand %q; want keygen, sign", args[0])
	}
	if err == nil {
		return 0
	}
	code := 1
	if errors.As(err, new(usageError)) {
		code = 2
	}
	if _, werr := fmt.Fprintf(stderr, "selfupdate-note: %v\n", err); werr != nil && code == 0 {
		code = 1
	}
	return code
}

// parse parses args into fs, and requires every named flag to be set.
func parse(fs *flag.FlagSet, args []string, values map[string]*string) error {
	fs.SetOutput(io.Discard)
	if err := fs.Parse(args); err != nil {
		return usagef("%v", err)
	}
	if fs.NArg() != 0 {
		return usagef("unexpected argument %q", fs.Arg(0))
	}
	for name, v := range values {
		if *v == "" {
			return usagef("-%s is required", name)
		}
	}
	return nil
}

func keygen(args []string, stdout io.Writer) error {
	fs := flag.NewFlagSet("keygen", flag.ContinueOnError)
	name, out := fs.String("name", "", ""), fs.String("out", "", "")
	if err := parse(fs, args, map[string]*string{"name": name, "out": out}); err != nil {
		return err
	}
	if strings.ContainsAny(*name, " \t\n+") {
		return usagef("-name %q may not contain a space or +", *name)
	}
	skey, vkey, err := note.GenerateKey(rand.Reader, *name)
	if err != nil {
		return err
	}
	if err := writeNew(*out, []byte(skey+"\n"), 0o600); err != nil {
		return err
	}
	_, err = fmt.Fprintln(stdout, vkey)
	return err
}

func sign(args []string, getenv func(string) string) error {
	fs := flag.NewFlagSet("sign", flag.ContinueOnError)
	dir, manifest := fs.String("dir", "", ""), fs.String("manifest", "", "")
	repo, tag := fs.String("repository", "", ""), fs.String("tag", "", "")
	products, vkey, out := fs.String("products-json", "", ""), fs.String("verifier-key", "", ""), fs.String("out", "", "")
	if err := parse(fs, args, map[string]*string{
		"dir": dir, "manifest": manifest, "repository": repo, "tag": tag,
		"products-json": products, "verifier-key": vkey, "out": out,
	}); err != nil {
		return err
	}
	skey := strings.TrimSpace(getenv(keyEnv))
	if skey == "" {
		return fmt.Errorf("%s is empty: the caller must pass the secret to the workflow", keyEnv)
	}
	var names []string
	if err := json.Unmarshal([]byte(*products), &names); err != nil {
		return usagef("-products-json: %v", err)
	}
	slices.Sort(names)
	data, err := os.ReadFile(filepath.Join(*dir, filepath.Base(*manifest)))
	if err != nil {
		return err
	}
	sum := sha256.Sum256(data)
	st := signednote.Statement{
		Repository: *repo, Tag: *tag, Products: names,
		Manifest: *manifest, ManifestSHA256: hex.EncodeToString(sum[:]),
	}
	text, err := st.Text()
	if err != nil {
		return err
	}
	signer, err := note.NewSigner(skey)
	if err != nil {
		// The parse error never quotes the key.
		return errors.New("the signer key in " + keyEnv + " cannot be parsed")
	}
	msg, err := note.Sign(&note.Note{Text: text}, signer)
	if err != nil {
		return err
	}
	opened, err := signednote.Open(msg, []string{*vkey})
	if err != nil {
		return fmt.Errorf("the note does not open with -verifier-key: %w", err)
	}
	if !reflect.DeepEqual(opened, st) {
		return errors.New("the note's statement does not read back")
	}
	return writeNew(*out, msg, 0o644)
}

// writeNew writes data to a new file at path, refusing one that exists.
func writeNew(path string, data []byte, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode) //nolint:gosec // the -out path the operator names; O_EXCL refuses an existing file
	if err != nil {
		return err
	}
	if _, err := f.Write(data); err != nil {
		return errors.Join(err, f.Close(), os.Remove(path))
	}
	return f.Close()
}
