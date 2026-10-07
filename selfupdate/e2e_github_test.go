package selfupdate_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/maccavelli/go-selfupdate-lib/selfupdate"
	"github.com/maccavelli/go-selfupdate-lib/selfupdate/selfupdatetest"
)

// Tests for docs/decisions/0004-PLAN-v1-1-0-core-api.md Step 11. The fixed
// platform keeps asset names, and so the golden files, identical on every
// host.
var goldenPlatform = selfupdate.Platform{OS: "linux", Arch: "amd64"}

func releaseBody(tag string) func(selfupdate.Platform) []byte {
	return func(p selfupdate.Platform) []byte { return []byte("demo " + tag + " " + p.OS + "/" + p.Arch + "\n") }
}

// tempTarget writes an old binary in a temporary directory the target
// policy allows. It is a plain file, not the running binary. The fixtures
// are built for goldenPlatform, so it is made the running platform: an
// apply for any other is refused (0010-MADR Q4).
func tempTarget(t *testing.T) (string, selfupdate.TargetPolicy) {
	t.Helper()
	selfupdate.SetRunningPlatform(t, goldenPlatform)
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	exe := filepath.Join(dir, "demo")
	if err := os.WriteFile(exe, []byte("old-bytes"), 0o755); err != nil {
		t.Fatal(err)
	}
	return exe, selfupdate.TargetPolicy{ExecutablePath: exe, AllowedRoots: []string{dir}}
}

func TestE2EGitHubRedirectedDownload(t *testing.T) {
	plats := []selfupdate.Platform{goldenPlatform}
	gh := selfupdatetest.NewGitHubServer(t, "owner", "demo",
		selfupdatetest.NewRelease("demo", "v1.0.0", plats, releaseBody("v1.0.0")),
		selfupdatetest.NewRelease("demo", "v1.1.0", plats, releaseBody("v1.1.0")))
	src, err := selfupdate.NewGitHubSource(selfupdate.GitHubOptions{
		Repository: selfupdate.Repository{Owner: "owner", Name: "demo"},
		Client:     gh.Client, APIBaseURL: gh.APIBase, UserAgent: "demo/v1.0.0",
		Token: "e2e-token", Limits: selfupdate.DefaultLimits(),
	})
	if err != nil {
		t.Fatal(err)
	}
	exe, policy := tempTarget(t)
	inst, err := selfupdate.NewStandaloneInstaller(selfupdate.InstallOptions{TargetPolicy: policy})
	if err != nil {
		t.Fatal(err)
	}
	sel, err := selfupdate.NewExactAssetSelector(plats)
	if err != nil {
		t.Fatal(err)
	}
	u, err := selfupdate.New(selfupdate.Config{
		Source: src, Versions: selfupdate.NewStrictVersionPolicy(), Assets: sel, Installer: inst,
		Reporter: selfupdate.DiscardReporter(), Confirmer: selfupdate.NonInteractiveConfirmer(),
		Limits: selfupdate.DefaultLimits(),
	})
	if err != nil {
		t.Fatal(err)
	}
	res, err := u.Run(context.Background(), selfupdate.Request{
		Product: "demo", CurrentVersion: "v1.0.0", CurrentBuild: selfupdate.ReleaseBuild,
		Platform: goldenPlatform, Yes: true,
	})
	if code := selfupdate.ExitCode(res, err); code != 0 || !res.Applied {
		t.Fatalf("ExitCode = %d, res = %+v, err = %v", code, res, err)
	}
	got, err := os.ReadFile(exe)
	if err != nil {
		t.Fatal(err)
	}
	if want := releaseBody("v1.1.0")(goldenPlatform); string(got) != string(want) {
		t.Fatalf("target = %q, want %q", got, want)
	}

	reqs := gh.Requests()
	assetHops := 0
	for i, r := range reqs {
		onAPI := r.Host == gh.APIBase.Host
		if onAPI && !r.Authorization {
			t.Errorf("API request %s had no Authorization", r.Path)
		}
		if !onAPI {
			if r.Authorization {
				t.Errorf("the asset host received Authorization on %s", r.Path)
			}
			continue
		}
		if !strings.Contains(r.Path, "/releases/assets/") {
			continue
		}
		// One cross-origin hop per asset: the next request is the download,
		// on the other origin, and the one after is not.
		if i+1 >= len(reqs) || reqs[i+1].Host == gh.APIBase.Host {
			t.Fatalf("asset request %s was not redirected to another origin: %+v", r.Path, reqs)
		}
		if i+2 < len(reqs) && reqs[i+2].Host != gh.APIBase.Host {
			t.Fatalf("asset request %s took more than one hop: %+v", r.Path, reqs)
		}
		assetHops++
	}
	if assetHops != 2 {
		t.Fatalf("asset requests = %d, want the manifest and the binary: %+v", assetHops, reqs)
	}
}

// e2eUpdater is TestE2EGitHubRedirectedDownload's composition, with an
// anonymous source.
func e2eUpdater(t *testing.T, gh *selfupdatetest.GitHubServer, opts selfupdate.GitHubOptions) (*selfupdate.Updater, string) {
	t.Helper()
	return e2eUpdaterWithEnvToken(t, gh, opts, "")
}

// e2eUpdaterWithEnvToken is e2eUpdater with GH_TOKEN set to ghToken, which
// the source reads when it is built.
func e2eUpdaterWithEnvToken(t *testing.T, gh *selfupdatetest.GitHubServer, opts selfupdate.GitHubOptions, ghToken string) (*selfupdate.Updater, string) {
	t.Helper()
	t.Setenv("GH_TOKEN", ghToken)
	t.Setenv("GITHUB_TOKEN", "")
	opts.Repository = selfupdate.Repository{Owner: "owner", Name: "demo"}
	opts.Client, opts.APIBaseURL, opts.UserAgent, opts.Limits = gh.Client, gh.APIBase, "demo/v1.0.0", selfupdate.DefaultLimits()
	src, err := selfupdate.NewGitHubSource(opts)
	if err != nil {
		t.Fatal(err)
	}
	exe, policy := tempTarget(t)
	inst, err := selfupdate.NewStandaloneInstaller(selfupdate.InstallOptions{TargetPolicy: policy})
	if err != nil {
		t.Fatal(err)
	}
	sel, err := selfupdate.NewExactAssetSelector([]selfupdate.Platform{goldenPlatform})
	if err != nil {
		t.Fatal(err)
	}
	u, err := selfupdate.New(selfupdate.Config{
		Source: src, Versions: selfupdate.NewStrictVersionPolicy(), Assets: sel, Installer: inst,
		Reporter: selfupdate.DiscardReporter(), Confirmer: selfupdate.NonInteractiveConfirmer(),
		Limits: selfupdate.DefaultLimits(),
	})
	if err != nil {
		t.Fatal(err)
	}
	return u, exe
}

func e2eReq() selfupdate.Request {
	return selfupdate.Request{
		Product: "demo", CurrentVersion: "v1.0.0", CurrentBuild: selfupdate.ReleaseBuild,
		Platform: goldenPlatform, Yes: true,
	}
}

func e2eServer(t *testing.T) *selfupdatetest.GitHubServer {
	t.Helper()
	plats := []selfupdate.Platform{goldenPlatform}
	return selfupdatetest.NewGitHubServer(t, "owner", "demo",
		selfupdatetest.NewRelease("demo", "v1.1.0", plats, releaseBody("v1.1.0")))
}

type staticCredential struct{ value string }

func (s staticCredential) Credential(context.Context, selfupdate.CredentialRequest) (selfupdate.Credential, error) {
	return selfupdate.Credential{Value: []byte(s.value), Source: "static"}, nil
}

// TestRunWithCredentialsIsPerRun: a run's credential applies to that run
// only (0004-PLAN-v1-2-0-interaction-stream.md Step 3).
func TestRunWithCredentialsIsPerRun(t *testing.T) {
	gh := e2eServer(t)
	gh.RequireToken("run-token")
	u, exe := e2eUpdater(t, gh, selfupdate.GitHubOptions{})
	res, err := u.RunWith(context.Background(), e2eReq(), selfupdate.WithCredentials(staticCredential{"run-token"}))
	if err != nil || !res.Applied {
		t.Fatalf("RunWith: res=%+v err=%v", res, err)
	}
	if got, _ := os.ReadFile(exe); string(got) != string(releaseBody("v1.1.0")(goldenPlatform)) {
		t.Fatalf("target = %q", got)
	}
	for _, r := range gh.Requests() {
		if r.Host != gh.APIBase.Host && r.Authorization {
			t.Fatalf("the download origin received Authorization on %s", r.Path)
		}
	}
	if _, err := u.Run(context.Background(), e2eReq()); err == nil || !strings.Contains(err.Error(), "github http 401") {
		t.Fatalf("a plain Run after RunWith = %v, want the 401: the run's credential leaked", err)
	}
}

// TestRunWithCredentialsNeedsCredentialedSource: a source that cannot take
// a per-run provider is refused before any call.
func TestRunWithCredentialsNeedsCredentialedSource(t *testing.T) {
	src := selfupdatetest.NewFakeSource("v1.1.0", goldenRelease())
	_, policy := tempTarget(t)
	inst, err := selfupdate.NewStandaloneInstaller(selfupdate.InstallOptions{TargetPolicy: policy})
	if err != nil {
		t.Fatal(err)
	}
	sel, err := selfupdate.NewExactAssetSelector([]selfupdate.Platform{goldenPlatform})
	if err != nil {
		t.Fatal(err)
	}
	u, err := selfupdate.New(selfupdate.Config{
		Source: src, Versions: selfupdate.NewStrictVersionPolicy(), Assets: sel, Installer: inst,
		Reporter: selfupdate.DiscardReporter(), Confirmer: selfupdate.NonInteractiveConfirmer(),
		Limits: selfupdate.DefaultLimits(),
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = u.RunWith(context.Background(), e2eReq(), selfupdate.WithCredentials(staticCredential{"x"}))
	const want = "selfupdate: WithCredentials: the source does not accept per-run credentials"
	if err == nil || err.Error() != want {
		t.Fatalf("err = %v, want %q", err, want)
	}
	if calls := src.Calls(); len(calls) != 0 {
		t.Fatalf("the source was called: %v", calls)
	}
}
