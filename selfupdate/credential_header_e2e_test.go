package selfupdate_test

import (
	"context"
	"strings"
	"testing"

	"github.com/maccavelli/go-selfupdate-lib/selfupdate"
)

// Tests for docs/decisions/0010-PLAN-v1-6-0-owner-contracts.md S7 (A14,
// amendment A5): a custom-header credential, required by the test server,
// reaches the API origin and never the asset origin.

type headerCredential struct{ header, value string }

func (h headerCredential) Credential(context.Context, selfupdate.CredentialRequest) (selfupdate.Credential, error) {
	return selfupdate.Credential{Header: h.header, Value: []byte(h.value), Source: "static"}, nil
}

func TestCustomHeaderCredentialStaysOnAPI(t *testing.T) {
	gh := e2eServer(t)
	gh.RequireCredential("X-Demo-Key", "k1")
	u, _ := e2eUpdater(t, gh, selfupdate.GitHubOptions{Credentials: headerCredential{"x-demo-key", "k1"}})
	if res, err := u.Run(context.Background(), e2eReq()); err != nil || !res.Applied {
		t.Fatalf("Run = %+v, %v", res, err)
	}
	api, assets := 0, 0
	for _, r := range gh.Requests() {
		switch r.Host {
		case gh.APIBase.Host:
			api++
			if !r.CredentialHeaders.Has("X-Demo-Key") || r.Authorization {
				t.Fatalf("API request %s carried %q, Authorization %t", r.Path, r.CredentialHeaders, r.Authorization)
			}
		default:
			assets++
			if r.CredentialHeaders != "" {
				t.Fatalf("the asset origin received %q on %s", r.CredentialHeaders, r.Path)
			}
		}
	}
	if api == 0 || assets == 0 {
		t.Fatalf("%d API and %d asset requests; want both", api, assets)
	}
}

// TestCustomHeaderCredentialRequired: without the header's value, the API
// refuses the run.
func TestCustomHeaderCredentialRequired(t *testing.T) {
	gh := e2eServer(t)
	gh.RequireCredential("X-Demo-Key", "k1")
	for _, opts := range []selfupdate.GitHubOptions{{}, {Credentials: headerCredential{"X-Demo-Key", "wrong"}}} {
		u, _ := e2eUpdater(t, gh, opts)
		if _, err := u.Run(context.Background(), e2eReq()); err == nil || !strings.Contains(err.Error(), "github http 401") {
			t.Fatalf("Run without the credential = %v, want the 401", err)
		}
	}
}
