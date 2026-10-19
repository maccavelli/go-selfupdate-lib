package selfupdate

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/url"
	"strconv"
	"strings"
)

// Pluggable credentials for release sources
// (docs/decisions/0004-MADR-evolve-selfupdate-api-and-tui-support.md G10).

// Credential is one HTTP credential. An empty Header means
// "Authorization: Bearer <Value>"; otherwise "Header: Value" is sent as
// given (0004-MADR amendment A6). Source labels where it came from, for
// diagnostics; it never holds the secret.
//
// Formatting a Credential with fmt or log/slog never prints Value;
// encoding/json does, so a CredentialObserver can save it. slog's JSON
// handler encodes a value that holds a Credential, such as a struct field,
// with encoding/json too, so log the Credential itself, or give the
// holder a LogValue
// (docs/decisions/0025-MADR-align-with-go-sdk-design-practices.md D1,
// Amendment A3).
type Credential struct {
	// Header is the HTTP header name, or empty for a bearer token.
	Header string
	// Value is the secret. Callers must not modify it after returning it.
	Value []byte
	// Source is a label such as "env:GH_TOKEN".
	Source string
}

// String implements fmt.Stringer. It prints Header and Source, and
// Value only as <redacted>, or <empty> when there is none.
func (c Credential) String() string {
	return "Credential{Header:" + strconv.Quote(c.Header) + ", Source:" + strconv.Quote(c.Source) +
		", Value:" + redacted(len(c.Value) > 0) + "}"
}

// GoString implements fmt.GoStringer. It is String.
func (c Credential) GoString() string { return c.String() }

// Format implements fmt.Formatter: every verb, %#v, %d, %x and %q
// included, prints String, so no directive reaches Value.
func (c Credential) Format(f fmt.State, _ rune) {
	_, _ = io.WriteString(f, c.String()) //nolint:errcheck // fmt.Formatter has no error to return; fmt drops write errors
}

// LogValue implements slog.LogValuer: Header and Source, and Value only
// as <redacted> or <empty>.
func (c Credential) LogValue() slog.Value {
	return slog.GroupValue(slog.String("header", c.Header), slog.String("source", c.Source),
		slog.String("value", redacted(len(c.Value) > 0)))
}

// redacted stands in for a secret: <redacted> when there is one.
func redacted(set bool) string {
	if set {
		return "<redacted>"
	}
	return "<empty>"
}

// setOrNil stands in for a value a formatter does not print.
func setOrNil(set bool) string {
	if set {
		return "<set>"
	}
	return "<nil>"
}

// CredentialRequest describes the request that needs a credential.
type CredentialRequest struct {
	// Origin is a copy of the API origin the credential will be sent to.
	Origin *url.URL
	// Cause is non-nil when a credential already sent was refused with 401.
	Cause error
	// Interactive reports whether the provider may prompt: true when the
	// run was started with Start, so a Stream can carry a prompt
	// (0010-MADR C10).
	Interactive bool
}

// CredentialProvider supplies a credential. Returning ErrNoCredential
// passes to the next provider in a chain, or to an anonymous request.
type CredentialProvider interface {
	Credential(context.Context, CredentialRequest) (Credential, error)
}

// CredentialObserver learns that a credential was accepted: a request that
// carried it succeeded. A program uses it to save a token only once it is
// known to work.
type CredentialObserver interface {
	Accepted(context.Context, Credential)
}

// ErrNoCredential reports that a provider has no credential to offer.
var ErrNoCredential = errors.New("selfupdate: no credential")

type chainCredentials []CredentialProvider

// PromptCredential returns a provider that asks the host. Inside a run
// started with Start, it delivers a *CredentialNeeded and waits for the
// reply or for the run to end; anywhere else it returns ErrNoCredential at
// once, so one chain serves Run and Start alike. A refused credential
// prompts once more, with Cause set. Use it last in a chain, after the
// environment, so a configured token is never prompted for:
//
//	ChainCredentials(EnvCredential("", "GH_TOKEN", "GITHUB_TOKEN"), PromptCredential())
//
// It finds the run's Stream through the context it is called with
// (0004-MADR amendment B1).
func PromptCredential() CredentialProvider {
	return promptCredential{}
}

type promptCredential struct{}

func (promptCredential) Credential(ctx context.Context, r CredentialRequest) (Credential, error) {
	s, ok := ctx.Value(streamKey{}).(*Stream)
	if !ok {
		return Credential{}, ErrNoCredential
	}
	if r.Origin != nil {
		origin := *r.Origin
		r.Origin = &origin
	}
	req := &CredentialNeeded{Request: r, reply: make(chan credentialReply, 1)}
	s.push(req)
	select {
	case rep := <-req.reply:
		return rep.cred, rep.err
	case <-ctx.Done():
		return Credential{}, ctx.Err()
	}
}

// CredentialedSource is a ReleaseSource that can take a per-run credential
// provider. WithCredentials returns a copy whose provider is p, with fresh
// credential state, and leaves the receiver unchanged. RunWith uses it for
// WithCredentials (0004-MADR amendment B1).
type CredentialedSource interface {
	ReleaseSource
	WithCredentials(p CredentialProvider) ReleaseSource
}

// ChainCredentials asks each provider in order. The first result other than
// ErrNoCredential wins, credential or error. Nil and typed-nil providers are
// dropped.
func ChainCredentials(providers ...CredentialProvider) CredentialProvider {
	kept := make(chainCredentials, 0, len(providers))
	for _, p := range providers {
		if !isNil(p) {
			kept = append(kept, p)
		}
	}
	return kept
}

func (c chainCredentials) Credential(ctx context.Context, req CredentialRequest) (Credential, error) {
	for _, p := range c {
		cred, err := p.Credential(ctx, req)
		if errors.Is(err, ErrNoCredential) {
			continue
		}
		return cred, err
	}
	return Credential{}, ErrNoCredential
}

type envCredential struct {
	header string
	names  []string
}

// EnvCredential returns the first non-empty environment variable among
// names as a credential for header, with Source "env:<NAME>". With none
// set it returns ErrNoCredential.
func EnvCredential(header string, names ...string) CredentialProvider {
	return envCredential{header: header, names: append([]string(nil), names...)}
}

func (e envCredential) Credential(context.Context, CredentialRequest) (Credential, error) {
	for _, name := range e.names {
		if v := lookupEnv(name); v != "" {
			return Credential{Header: e.header, Value: []byte(v), Source: "env:" + name}, nil
		}
	}
	return Credential{}, ErrNoCredential
}

// validateCredential refuses a header name that is not an RFC 7230 token
// and a value that could split or truncate the header. Errors never
// include the value.
func validateCredential(c Credential) error {
	if c.Header != "" {
		for i := 0; i < len(c.Header); i++ {
			if !isTokenChar(c.Header[i]) {
				return fmt.Errorf("selfupdate: invalid credential from %s", sanitizeText(c.Source))
			}
		}
	}
	if len(c.Value) == 0 {
		return fmt.Errorf("selfupdate: invalid credential from %s", sanitizeText(c.Source))
	}
	// Every control byte but HTAB is refused, as net/http refuses it in a
	// header value (0010-MADR A9).
	for _, b := range c.Value {
		if (b < 0x20 && b != '\t') || b == 0x7f {
			return fmt.Errorf("selfupdate: invalid credential from %s", sanitizeText(c.Source))
		}
	}
	return nil
}

func isTokenChar(b byte) bool {
	switch {
	case b >= 'a' && b <= 'z', b >= 'A' && b <= 'Z', b >= '0' && b <= '9':
		return true
	}
	return strings.IndexByte("!#$%&'*+-.^_`|~", b) >= 0
}

func sameCredential(a, b Credential) bool {
	return a.Header == b.Header && bytes.Equal(a.Value, b.Value)
}
