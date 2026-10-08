package ghattest

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/maccavelli/go-selfupdate-lib/selfupdate"
	"github.com/maccavelli/go-selfupdate-lib/selfupdate/service"
)

// Tests for docs/decisions/0017-PLAN-verify-build-provenance-and-close-0015-open-items.md
// Q2, against a fake runner, and the gh output the probe of 2026-10-08
// recorded: aquaproj/aqua v2.64.0's checksums, which a reusable workflow in
// another repository attests, as this module's publish workflow attests
// its callers' releases.

// fakeTool records each command and answers with the next scripted output.
type fakeTool struct {
	calls   []service.Command
	outputs []service.Output
	err     error
	// read is the content of the file gh was given, when the call ran.
	read [][]byte
	// mode is that file's mode.
	mode []os.FileMode
	// sleep, when set, makes Run wait for the context.
	sleep bool
}

func (f *fakeTool) Run(ctx context.Context, c service.Command) (service.Output, error) {
	f.calls = append(f.calls, c)
	if len(c.Args) > 2 {
		b, _ := os.ReadFile(c.Args[2])
		f.read = append(f.read, b)
		if info, err := os.Stat(c.Args[2]); err == nil {
			f.mode = append(f.mode, info.Mode())
		}
	}
	if f.sleep {
		<-ctx.Done()
		return service.Output{}, ctx.Err()
	}
	if f.err != nil {
		return service.Output{}, f.err
	}
	if len(f.outputs) == 0 {
		return service.Output{}, nil
	}
	out := f.outputs[0]
	f.outputs = f.outputs[1:]
	return out, nil
}

const (
	aquaSigner = "suzuki-shunsuke/go-release-workflow/.github/workflows/release.yaml"
	aquaDigest = "cf03c29d97518871efb36bbb80bcc01a19645b49"
	aquaSums   = "651d4378614e8506c5814d3bed0c5643eea74b957e491601427ad8ebf241fa02"
)

// aquaPolicy is the policy aqua v2.64.0's attestation meets.
func aquaPolicy() Policy {
	return Policy{
		Repository:     selfupdate.Repository{Owner: "aquaproj", Name: "aqua"},
		SignerWorkflow: aquaSigner,
		SignerDigests:  []string{aquaDigest},
	}
}

// ghPath is an absolute gh path on this host.
func ghPath() string {
	if runtime.GOOS == "windows" {
		return `C:\Program Files\GitHub CLI\gh.exe`
	}
	return "/usr/local/bin/gh"
}

func options(p Policy, r service.Runner) Options {
	return Options{Policy: p, GH: ghPath(), Runner: r, Env: []string{"HOME=/home/<user>"}}
}

// fixture is the recorded gh output.
func fixture(t *testing.T) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", "verify-aqua-v2.64.0.json"))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// manifestCheck runs the manifest verifier for tag on body.
func manifestCheck(t *testing.T, o Options, tag string, body []byte) error {
	t.Helper()
	v, err := NewManifestVerifier(o)
	if err != nil {
		t.Fatal(err)
	}
	return v.VerifyManifest(context.Background(), selfupdate.ManifestVerification{
		Product: "aqua", Release: selfupdate.Release{Tag: tag}, Manifest: body,
	})
}

func TestNewRefuses(t *testing.T) {
	good := options(aquaPolicy(), &fakeTool{})
	cases := map[string]func(o *Options){
		"no repository":      func(o *Options) { o.Policy.Repository = selfupdate.Repository{} },
		"owner-dash":         func(o *Options) { o.Policy.Repository.Owner = "-x" },
		"name with a slash":  func(o *Options) { o.Policy.Repository.Name = "a/b" },
		"name with a colon":  func(o *Options) { o.Policy.Repository.Name = "a:b" },
		"name with a space":  func(o *Options) { o.Policy.Repository.Name = "a b" },
		"name dot-dot":       func(o *Options) { o.Policy.Repository.Name = ".." },
		"name control":       func(o *Options) { o.Policy.Repository.Name = "a\nb" },
		"workflow too short": func(o *Options) { o.Policy.SignerWorkflow = "o/r/release.yml" },
		"workflow not yml":   func(o *Options) { o.Policy.SignerWorkflow = "o/r/.github/workflows/release.sh" },
		"workflow elsewhere": func(o *Options) { o.Policy.SignerWorkflow = "o/r/.github/actions/release.yml" },
		"workflow dash":      func(o *Options) { o.Policy.SignerWorkflow = "-o/r/.github/workflows/release.yml" },
		"workflow space":     func(o *Options) { o.Policy.SignerWorkflow = "o/r/.github/workflows/re lease.yml" },
		"digest short":       func(o *Options) { o.Policy.SignerDigests = []string{"cf03c29d"} },
		"digest upper":       func(o *Options) { o.Policy.SignerDigests = []string{strings.ToUpper(aquaDigest)} },
		"no gh":              func(o *Options) { o.GH = "" },
		"relative gh":        func(o *Options) { o.GH = "gh" },
		"negative-timeout":   func(o *Options) { o.Timeout = -time.Second },
	}
	for name, edit := range cases {
		t.Run(name, func(t *testing.T) {
			o := good
			o.Policy.SignerDigests = slices.Clone(good.Policy.SignerDigests)
			edit(&o)
			if _, err := NewManifestVerifier(o); err == nil {
				t.Fatal("NewManifestVerifier accepted it")
			}
			if _, err := NewVerifier(o); err == nil {
				t.Fatal("NewVerifier accepted it")
			}
		})
	}
	if _, err := NewManifestVerifier(good); err != nil {
		t.Fatalf("the good options: %v", err)
	}
	o := good
	o.Policy.SignerWorkflow = ""
	if _, err := NewManifestVerifier(o); err != nil {
		t.Fatalf("an empty SignerWorkflow means the publish workflow: %v", err)
	}
}

// TestArguments: the exact argv for each policy, gh's path, and the
// environment with gh's four settings last.
func TestArguments(t *testing.T) {
	base := func(file, wf string) []string {
		return []string{"attestation", "verify", file, "--hostname", "github.com",
			"--repo", "aquaproj/aqua", "--signer-workflow", wf,
			"--source-ref", "refs/tags/v2.64.0", "--predicate-type", PredicateType,
			"--cert-oidc-issuer", Issuer, "--format", "json"}
	}
	cases := []struct {
		name    string
		edit    func(*Policy)
		wf      string
		tail    []string
		envNil  bool
		envWant []string
	}{
		{"no digest", func(p *Policy) { p.SignerDigests = nil }, aquaSigner, []string{"--deny-self-hosted-runners"}, false, nil},
		{"one digest", func(*Policy) {}, aquaSigner, []string{"--deny-self-hosted-runners", "--signer-digest", aquaDigest}, false, nil},
		{"two digests", func(p *Policy) { p.SignerDigests = append(p.SignerDigests, strings.Repeat("a", 40)) }, aquaSigner, []string{"--deny-self-hosted-runners"}, false, nil},
		{"self-hosted allowed", func(p *Policy) { p.AllowSelfHostedRunners = true; p.SignerDigests = nil }, aquaSigner, nil, false, nil},
		{"the publish workflow", func(p *Policy) { p.SignerWorkflow = ""; p.SignerDigests = nil }, PublishWorkflow, []string{"--deny-self-hosted-runners"}, true, nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p := aquaPolicy()
			p.SignerDigests = slices.Clone(p.SignerDigests)
			c.edit(&p)
			f := &fakeTool{outputs: []service.Output{{ExitCode: 1}}}
			o := options(p, f)
			if c.envNil {
				o.Env = nil
			}
			_ = manifestCheck(t, o, "v2.64.0", []byte("sums\n"))
			if len(f.calls) != 1 {
				t.Fatalf("%d runs", len(f.calls))
			}
			cmd := f.calls[0]
			want := append(base(cmd.Args[2], c.wf), c.tail...)
			if cmd.Path != ghPath() || !slices.Equal(cmd.Args, want) {
				t.Fatalf("ran %s %q\nwant %q", cmd.Path, cmd.Args, want)
			}
			tail := []string{"GH_PROMPT_DISABLED=1", "GH_NO_UPDATE_NOTIFIER=1", "GH_SPINNER_DISABLED=1", "NO_COLOR=1"}
			if !slices.Equal(cmd.Env[len(cmd.Env)-4:], tail) {
				t.Fatalf("environment ends %q", cmd.Env[len(cmd.Env)-4:])
			}
			head := cmd.Env[:len(cmd.Env)-4]
			if c.envNil {
				if !slices.Equal(head, os.Environ()) {
					t.Fatal("a nil Env is not this process's environment")
				}
			} else if !slices.Equal(head, []string{"HOME=/home/<user>"}) {
				t.Fatalf("environment %q", head)
			}
		})
	}
}

// TestExitCodes: every way gh can fail is an error naming it, with its
// output bounded to one line.
func TestExitCodes(t *testing.T) {
	long := strings.Repeat("x", 4000) + "\n\x1b[31mred"
	cases := []struct {
		name string
		f    *fakeTool
		o    func(*Options)
		want string
		is   error
	}{
		{"1", &fakeTool{outputs: []service.Output{{ExitCode: 1, Stderr: []byte("Error: expected SourceRepositoryRef\n" + long)}}}, nil, "attestation verification failed (exit 1): Error: expected SourceRepositoryRef", nil},
		{"4", &fakeTool{outputs: []service.Output{{ExitCode: 4, Stderr: []byte("To get started with GitHub CLI, please run: gh auth login")}}}, nil, "gh is not logged in (exit 4): To get started", nil},
		{"runner", &fakeTool{err: errors.New("exec: no such file")}, nil, "running gh: exec: no such file", nil},
		{"timeout", &fakeTool{sleep: true}, func(o *Options) { o.Timeout = time.Millisecond }, "running gh", context.DeadlineExceeded},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			o := options(aquaPolicy(), c.f)
			if c.o != nil {
				c.o(&o)
			}
			err := manifestCheck(t, o, "v2.64.0", []byte("sums\n"))
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("err = %v, want %q", err, c.want)
			}
			if c.is != nil && !errors.Is(err, c.is) {
				t.Fatalf("err = %v, want %v", err, c.is)
			}
			if strings.ContainsAny(err.Error(), "\n\x1b") || len(err.Error()) > maxDetail+300 {
				t.Fatalf("the error is not one bounded line: %d bytes", len(err.Error()))
			}
		})
	}
}

// mutate decodes the fixture, changes one field of the first result with
// edit, and encodes it again.
func mutate(t *testing.T, edit func(r map[string]any)) []byte {
	t.Helper()
	var results []map[string]any
	if err := json.Unmarshal(fixture(t), &results); err != nil {
		t.Fatal(err)
	}
	edit(results[0]["verificationResult"].(map[string]any))
	b, err := json.Marshal(results)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func cert(vr map[string]any) map[string]any {
	return vr["signature"].(map[string]any)["certificate"].(map[string]any)
}

// TestPolicyRecheck: gh's exit 0 is not enough. The certificate it reports
// must meet the whole policy, and a statement must name the file's digest
// (0017-PLAN N11).
func TestPolicyRecheck(t *testing.T) {
	check := func(t *testing.T, p Policy, tag, digest string, stdout []byte) error {
		t.Helper()
		c, err := newChecker(options(p, &fakeTool{}))
		if err != nil {
			t.Fatal(err)
		}
		return c.recheck(stdout, "refs/tags/"+tag, digest)
	}
	if err := check(t, aquaPolicy(), "v2.64.0", aquaSums, fixture(t)); err != nil {
		t.Fatalf("the recorded attestation: %v", err)
	}
	two := aquaPolicy()
	two.SignerDigests = []string{strings.Repeat("a", 40), aquaDigest}
	if err := check(t, two, "v2.64.0", aquaSums, fixture(t)); err != nil {
		t.Fatalf("two digests, the second the signer's: %v", err)
	}
	cases := []struct {
		name  string
		p     func(*Policy)
		tag   string
		sum   string
		out   func(t *testing.T) []byte
		field string
	}{
		{"predicate", nil, "", "", func(t *testing.T) []byte {
			return mutate(t, func(vr map[string]any) {
				vr["statement"].(map[string]any)["predicateType"] = "https://spdx.dev/Document"
			})
		}, "predicateType"},
		{"issuer", nil, "", "", func(t *testing.T) []byte {
			return mutate(t, func(vr map[string]any) { cert(vr)["issuer"] = "https://example.com" })
		}, "issuer"},
		{"repository", nil, "", "", func(t *testing.T) []byte {
			return mutate(t, func(vr map[string]any) { cert(vr)["sourceRepositoryURI"] = "https://github.com/aquaproj/other" })
		}, "sourceRepositoryURI"},
		{"ref", nil, "v2.63.0", "", nil, "sourceRepositoryRef"},
		{"signer", func(p *Policy) { p.SignerWorkflow = "suzuki-shunsuke/go-release-workflow/.github/workflows/other.yaml" }, "", "", nil, "buildSignerURI"},
		{"digest", func(p *Policy) { p.SignerDigests = []string{strings.Repeat("a", 40)} }, "", "", nil, "buildSignerDigest"},
		{"self-hosted", nil, "", "", func(t *testing.T) []byte {
			return mutate(t, func(vr map[string]any) { cert(vr)["runnerEnvironment"] = "self-hosted" })
		}, "runnerEnvironment"},
		{"subject", nil, "", strings.Repeat("0", 64), nil, "subject"},
		{"empty", nil, "", "", func(*testing.T) []byte { return []byte("[]") }, "no attestation"},
		{"garbage", nil, "", "", func(*testing.T) []byte { return []byte("not json") }, "not the expected JSON"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p := aquaPolicy()
			if c.p != nil {
				c.p(&p)
			}
			tag, sum, out := "v2.64.0", aquaSums, fixture(t)
			if c.tag != "" {
				tag = c.tag
			}
			if c.sum != "" {
				sum = c.sum
			}
			if c.out != nil {
				out = c.out(t)
			}
			err := check(t, p, tag, sum, out)
			if err == nil || !strings.Contains(err.Error(), c.field) {
				t.Fatalf("err = %v, want %q named", err, c.field)
			}
		})
	}
	// The case of a repository's name does not matter to GitHub.
	upper := aquaPolicy()
	upper.Repository = selfupdate.Repository{Owner: "AquaProj", Name: "Aqua"}
	if err := check(t, upper, "v2.64.0", aquaSums, fixture(t)); err != nil {
		t.Fatalf("the repository in another case: %v", err)
	}
}

// TestTagRefused: a tag that could not be a ref, or could be a flag, is
// refused before gh runs.
func TestTagRefused(t *testing.T) {
	for _, tag := range []string{"", "-x", "v1 2", "v1\n"} {
		f := &fakeTool{}
		err := manifestCheck(t, options(aquaPolicy(), f), tag, []byte("sums\n"))
		if err == nil || len(f.calls) != 0 {
			t.Errorf("tag %q: err = %v, runs = %d", tag, err, len(f.calls))
		}
	}
}

// TestManifestFile: gh reads a private file holding exactly the manifest,
// which is gone afterwards, whether the check passed or failed.
func TestManifestFile(t *testing.T) {
	sums := []byte("651d4378614e8506c5814d3bed0c5643eea74b957e491601427ad8ebf241fa02  aqua\n")
	for _, exit := range []int{0, 1} {
		f := &fakeTool{outputs: []service.Output{{ExitCode: exit, Stdout: []byte("[]")}}}
		o := options(aquaPolicy(), f)
		o.TempDir = t.TempDir()
		_ = manifestCheck(t, o, "v2.64.0", sums)
		if len(f.read) != 1 || !bytes.Equal(f.read[0], sums) {
			t.Fatalf("exit %d: gh read %q", exit, f.read)
		}
		if runtime.GOOS != "windows" && (len(f.mode) != 1 || f.mode[0].Perm() != 0o600) {
			t.Fatalf("exit %d: the file's mode %v", exit, f.mode)
		}
		left, err := os.ReadDir(o.TempDir)
		if err != nil {
			t.Fatal(err)
		}
		if len(left) != 0 {
			t.Fatalf("exit %d: left %v", exit, left)
		}
	}
}

// TestVerifierChecksDigest: the asset verifier refuses bytes that are not
// the staged asset's before gh runs; with the right bytes, gh reads them.
func TestVerifierChecksDigest(t *testing.T) {
	body := []byte("the staged asset")
	sum := sha256.Sum256(body)
	for _, c := range []struct {
		name   string
		served []byte
		runs   int
	}{
		{"other bytes", []byte("other bytes, same len"), 0},
		{"the asset", body, 1},
	} {
		t.Run(c.name, func(t *testing.T) {
			f := &fakeTool{outputs: []service.Output{{ExitCode: 1}}}
			v, err := NewVerifier(options(aquaPolicy(), f))
			if err != nil {
				t.Fatal(err)
			}
			err = v.Verify(context.Background(), selfupdate.Verification{
				Product: "aqua", Release: selfupdate.Release{Tag: "v2.64.0"},
				Size: int64(len(body)), SHA256: hex.EncodeToString(sum[:]),
				Open: func() (io.ReadCloser, error) { return io.NopCloser(bytes.NewReader(c.served)), nil },
			})
			if err == nil || len(f.calls) != c.runs {
				t.Fatalf("err = %v, runs = %d, want %d", err, len(f.calls), c.runs)
			}
			if c.runs == 1 && !bytes.Equal(f.read[0], body) {
				t.Fatalf("gh read %q", f.read[0])
			}
		})
	}
}
