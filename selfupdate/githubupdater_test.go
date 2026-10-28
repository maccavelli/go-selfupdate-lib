package selfupdate_test

import (
	"context"
	"net/http"
	"runtime"
	"sync"
	"testing"

	"github.com/maccavelli/go-selfupdate-lib/selfupdate"
	"github.com/maccavelli/go-selfupdate-lib/selfupdate/selfupdatetest"
)

// Tests for NewGitHubUpdater (docs/decisions/0025-MADR-align-with-go-sdk-design-practices.md
// D5, Amendment A1).

func TestNewGitHubUpdaterRefuses(t *testing.T) {
	t.Parallel()
	ok := selfupdate.GitHubUpdaterOptions{
		Repository: selfupdate.Repository{Owner: "o", Name: "r"},
		Product:    "demo",
		Version:    "v1.0.0",
	}
	if _, err := selfupdate.NewGitHubUpdater(ok); err != nil {
		t.Fatalf("valid options: %v", err)
	}
	for name, edit := range map[string]func(*selfupdate.GitHubUpdaterOptions){
		"empty owner":     func(o *selfupdate.GitHubUpdaterOptions) { o.Repository.Owner = "" },
		"empty name":      func(o *selfupdate.GitHubUpdaterOptions) { o.Repository.Name = "" },
		"empty product":   func(o *selfupdate.GitHubUpdaterOptions) { o.Product = "" },
		"invalid product": func(o *selfupdate.GitHubUpdaterOptions) { o.Product = "a b" },
		"empty version":   func(o *selfupdate.GitHubUpdaterOptions) { o.Version = "" },
		"invalid platform": func(o *selfupdate.GitHubUpdaterOptions) {
			o.Platforms = []selfupdate.Platform{{OS: "Linux", Arch: "amd64"}}
		},
		"no platforms": func(o *selfupdate.GitHubUpdaterOptions) { o.Platforms = []selfupdate.Platform{} },
	} {
		o := ok
		edit(&o)
		if u, err := selfupdate.NewGitHubUpdater(o); err == nil || u != nil {
			t.Errorf("%s: updater %v, err %v; want a refusal", name, u, err)
		}
	}
}

// agentRecorder records each request's User-Agent before sending it on.
type agentRecorder struct {
	base   http.RoundTripper
	mu     sync.Mutex
	agents []string
}

func (a *agentRecorder) RoundTrip(r *http.Request) (*http.Response, error) {
	a.mu.Lock()
	a.agents = append(a.agents, r.Header.Get("User-Agent"))
	a.mu.Unlock()
	return a.base.RoundTrip(r)
}

// TestNewGitHubUpdaterChecks: the composed Updater finds a newer release
// for the running platform, and sends UserAgent(product, version).
func TestNewGitHubUpdaterChecks(t *testing.T) {
	t.Parallel()
	running := []selfupdate.Platform{{OS: runtime.GOOS, Arch: runtime.GOARCH}}
	body := func(tag string) func(selfupdate.Platform) []byte {
		return func(selfupdate.Platform) []byte { return []byte("binary " + tag) }
	}
	gh := selfupdatetest.NewGitHubServer(t, "owner", "demo",
		selfupdatetest.NewRelease("demo", "v1.0.0", running, body("v1.0.0")),
		selfupdatetest.NewRelease("demo", "v1.1.0", running, body("v1.1.0")))
	rec := &agentRecorder{base: gh.Client.Transport}
	u, err := selfupdate.NewGitHubUpdater(selfupdate.GitHubUpdaterOptions{
		Repository: selfupdate.Repository{Owner: "owner", Name: "demo"},
		Product:    "demo",
		Version:    "v1.0.0",
		APIBaseURL: gh.APIBase,
		Client:     &http.Client{Transport: rec},
	})
	if err != nil {
		t.Fatal(err)
	}
	av, err := u.Checker().Check(context.Background(), selfupdate.CheckRequest{
		Product: "demo", CurrentVersion: "v1.0.0", CurrentBuild: selfupdate.ReleaseBuild,
	})
	if err != nil || !av.Available || av.TargetVersion != "v1.1.0" {
		t.Fatalf("Check = %+v, %v; want v1.1.0 available", av, err)
	}
	want := selfupdate.UserAgent("demo", "v1.0.0")
	rec.mu.Lock()
	defer rec.mu.Unlock()
	if len(rec.agents) == 0 {
		t.Fatal("no request was sent")
	}
	for _, got := range rec.agents {
		if got != want {
			t.Errorf("User-Agent %q, want %q", got, want)
		}
	}
}
