package ghattest

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/maccavelli/go-selfupdate-lib/selfupdate"
	"github.com/maccavelli/go-selfupdate-lib/selfupdate/service"
)

// PublishWorkflow is this module's publish workflow, the signer of every
// release it publishes for a caller
// (docs/decisions/0013-MADR-build-and-stage-release-workflow.md).
const PublishWorkflow = "maccavelli/go-selfupdate-lib/.github/workflows/publish-selfupdate-release.yml"

// PredicateType and Issuer are what every check requires: SLSA build
// provenance, from a GitHub Actions OIDC token.
const (
	PredicateType = "https://slsa.dev/provenance/v1"
	Issuer        = "https://token.actions.githubusercontent.com"
)

const (
	defaultTimeout = 2 * time.Minute
	// maxDetail bounds the gh output an error carries.
	maxDetail = 1024
	// ghExitAuth is gh's exit code when it needs to be logged in.
	ghExitAuth = 4
)

// ghSettings keep gh from prompting, checking for updates, animating or
// colouring its output (0017-PLAN N13).
var ghSettings = []string{"GH_PROMPT_DISABLED=1", "GH_NO_UPDATE_NOTIFIER=1", "GH_SPINNER_DISABLED=1", "NO_COLOR=1"}

var (
	digestRe = regexp.MustCompile(`^[0-9a-f]{40}$`)
	sha256Re = regexp.MustCompile(`^[0-9a-f]{64}$`)
)

// Policy is what an attestation must say. A verifier that does not run gh
// (0017-MADR 1E) is meant to take the same Policy.
type Policy struct {
	// Repository is the program's own repository, which the attestation's
	// source must be. Required.
	Repository selfupdate.Repository
	// SignerWorkflow is owner/repo/.github/workflows/<file> of the workflow
	// that signed. Empty means PublishWorkflow.
	SignerWorkflow string
	// SignerDigests, when set, are the signer workflow's commits accepted:
	// 40 lower-case hex each.
	SignerDigests []string
	// AllowSelfHostedRunners accepts an attestation made on a self-hosted
	// runner.
	AllowSelfHostedRunners bool
}

// Options configure a verifier. GH is required.
type Options struct {
	// Policy is what the attestation must say.
	Policy Policy
	// GH is gh's absolute path; nothing is looked up on PATH.
	GH string
	// Env is gh's environment; nil means this process's. gh finds its
	// login through HOME, GH_CONFIG_DIR or GH_TOKEN. The package appends
	// settings that keep gh from prompting.
	Env []string
	// Runner runs gh. Nil means service.ExecRunner.
	Runner service.Runner
	// Timeout bounds one gh run. Zero means 2 minutes; negative is refused.
	Timeout time.Duration
	// TempDir holds the file gh reads. Empty means os.TempDir.
	TempDir string
}

// checker runs gh for one Options.
type checker struct {
	policy  Policy
	gh      string
	env     []string
	runner  service.Runner
	timeout time.Duration
	tempDir string
}

func newChecker(o Options) (*checker, error) {
	p := o.Policy
	if err := validName("repository owner", p.Repository.Owner); err != nil {
		return nil, err
	}
	if err := validName("repository name", p.Repository.Name); err != nil {
		return nil, err
	}
	if p.SignerWorkflow == "" {
		p.SignerWorkflow = PublishWorkflow
	}
	if err := validWorkflow(p.SignerWorkflow); err != nil {
		return nil, err
	}
	for _, d := range p.SignerDigests {
		if !digestRe.MatchString(d) {
			return nil, fmt.Errorf("selfupdate: ghattest: signer digest %q is not 40 lower-case hex", d)
		}
	}
	p.SignerDigests = slices.Clone(p.SignerDigests)
	if o.GH == "" || !filepath.IsAbs(o.GH) {
		return nil, fmt.Errorf("selfupdate: ghattest: gh's path %q is not absolute", o.GH)
	}
	if o.Timeout < 0 {
		return nil, fmt.Errorf("selfupdate: ghattest: the timeout must not be negative")
	}
	timeout := o.Timeout
	if timeout == 0 {
		timeout = defaultTimeout
	}
	runner := o.Runner
	if runner == nil {
		runner = service.ExecRunner()
	}
	var env []string
	if o.Env != nil {
		env = slices.Clone(o.Env)
	}
	return &checker{policy: p, gh: o.GH, env: env, runner: runner, timeout: timeout, tempDir: o.TempDir}, nil
}

// validName refuses an owner or repository name that is empty, a path, or
// could be read as a flag.
func validName(what, s string) error {
	if s == "" || s == "." || s == ".." || strings.HasPrefix(s, "-") || strings.ContainsAny(s, "/\\:? ") || hasControl(s) {
		return fmt.Errorf("selfupdate: ghattest: %s %q is not a GitHub name", what, s)
	}
	return nil
}

// validWorkflow requires owner/repo/.github/workflows/<file>.yml or .yaml.
func validWorkflow(s string) error {
	parts := strings.Split(s, "/")
	if len(parts) != 5 || parts[2] != ".github" || parts[3] != "workflows" ||
		(!strings.HasSuffix(parts[4], ".yml") && !strings.HasSuffix(parts[4], ".yaml")) ||
		validName("workflow", parts[4]) != nil || validName("owner", parts[0]) != nil || validName("repository", parts[1]) != nil {
		return fmt.Errorf("selfupdate: ghattest: signer workflow %q is not owner/repo/.github/workflows/<file>.yml", s)
	}
	return nil
}

func hasControl(s string) bool {
	return strings.ContainsFunc(s, func(r rune) bool { return r < 0x20 || r == 0x7f })
}

// NewManifestVerifier returns a selfupdate.ManifestVerifier that verifies
// the release's SHA256SUMS against o.Policy with gh, before any binary is
// downloaded. Add it to selfupdate.Config.ManifestVerifiers.
func NewManifestVerifier(o Options) (selfupdate.ManifestVerifier, error) {
	c, err := newChecker(o)
	if err != nil {
		return nil, err
	}
	return manifestVerifier{c}, nil
}

// NewVerifier returns a selfupdate.Verifier that verifies the downloaded
// asset itself against o.Policy with gh. Add it to selfupdate.Config.Verifiers.
func NewVerifier(o Options) (selfupdate.Verifier, error) {
	c, err := newChecker(o)
	if err != nil {
		return nil, err
	}
	return assetVerifier{c}, nil
}

type manifestVerifier struct{ c *checker }

func (v manifestVerifier) VerifyManifest(ctx context.Context, m selfupdate.ManifestVerification) error {
	return v.c.check(ctx, m.Release.Tag, func(w io.Writer) (string, error) {
		h := sha256.New()
		if _, err := io.MultiWriter(w, h).Write(m.Manifest); err != nil {
			return "", err
		}
		return hex.EncodeToString(h.Sum(nil)), nil
	})
}

type assetVerifier struct{ c *checker }

func (v assetVerifier) Verify(ctx context.Context, a selfupdate.Verification) error {
	return v.c.check(ctx, a.Release.Tag, func(w io.Writer) (_ string, err error) {
		if a.Open == nil || !sha256Re.MatchString(a.SHA256) || a.Size < 0 {
			return "", fmt.Errorf("selfupdate: ghattest: the asset to verify is not described")
		}
		r, err := a.Open()
		if err != nil {
			return "", err
		}
		defer func() { err = errors.Join(err, r.Close()) }()
		h := sha256.New()
		n, err := io.Copy(io.MultiWriter(w, h), io.LimitReader(r, a.Size+1))
		if err != nil {
			return "", err
		}
		digest := hex.EncodeToString(h.Sum(nil))
		if n != a.Size || digest != a.SHA256 {
			return "", fmt.Errorf("selfupdate: ghattest: the asset read (%d bytes, sha256 %s) is not the one staged", n, digest)
		}
		return digest, nil
	})
}

// check writes the file gh verifies with write, runs gh for tag, and checks
// what gh reports.
func (c *checker) check(ctx context.Context, tag string, write func(io.Writer) (string, error)) error {
	if tag == "" || strings.HasPrefix(tag, "-") || strings.ContainsAny(tag, " ") || hasControl(tag) || !utf8.ValidString(tag) {
		return fmt.Errorf("selfupdate: ghattest: tag %q cannot be checked", tag)
	}
	ref := "refs/tags/" + tag
	f, err := os.CreateTemp(c.tempDir, "selfupdate-ghattest-*")
	if err != nil {
		return fmt.Errorf("selfupdate: ghattest: %w", err)
	}
	defer os.Remove(f.Name()) //nolint:errcheck // a private temporary file
	digest, werr := write(f)
	if err := errors.Join(werr, f.Close()); err != nil {
		return fmt.Errorf("selfupdate: ghattest: %w", err)
	}
	rctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	out, err := c.runner.Run(rctx, service.Command{Path: c.gh, Args: c.args(f.Name(), ref), Env: c.environ()})
	if err != nil {
		return fmt.Errorf("selfupdate: ghattest: running gh: %w", err)
	}
	switch out.ExitCode {
	case 0:
		return c.recheck(out.Stdout, ref, digest)
	case ghExitAuth:
		return fmt.Errorf("selfupdate: ghattest: gh is not logged in (exit %d): %s", ghExitAuth, detail(out))
	}
	return fmt.Errorf("selfupdate: ghattest: attestation verification failed (exit %d): %s", out.ExitCode, detail(out))
}

// args is gh's command line for file and ref (0017-PLAN Q2 step 3).
func (c *checker) args(file, ref string) []string {
	p := c.policy
	args := []string{"attestation", "verify", file, "--hostname", "github.com",
		"--repo", p.Repository.Owner + "/" + p.Repository.Name,
		"--signer-workflow", p.SignerWorkflow,
		"--source-ref", ref,
		"--predicate-type", PredicateType,
		"--cert-oidc-issuer", Issuer,
		"--format", "json"}
	if !p.AllowSelfHostedRunners {
		args = append(args, "--deny-self-hosted-runners")
	}
	if len(p.SignerDigests) == 1 {
		args = append(args, "--signer-digest", p.SignerDigests[0])
	}
	return args
}

// environ is gh's environment: the configured one, or this process's, then
// the settings that keep gh quiet.
func (c *checker) environ() []string {
	env := c.env
	if env == nil {
		env = os.Environ()
	}
	return append(slices.Clone(env), ghSettings...)
}

// result is the part of gh attestation verify --format json this package
// reads: the statement and the certificate, which gh says are what it
// verified.
type result struct {
	VerificationResult struct {
		Statement struct {
			PredicateType string `json:"predicateType"`
			Subject       []struct {
				Digest map[string]string `json:"digest"`
			} `json:"subject"`
		} `json:"statement"`
		Signature struct {
			Certificate struct {
				Issuer              string `json:"issuer"`
				SourceRepositoryURI string `json:"sourceRepositoryURI"`
				SourceRepositoryRef string `json:"sourceRepositoryRef"`
				BuildSignerURI      string `json:"buildSignerURI"`
				BuildSignerDigest   string `json:"buildSignerDigest"`
				RunnerEnvironment   string `json:"runnerEnvironment"`
			} `json:"certificate"`
		} `json:"signature"`
	} `json:"verificationResult"`
}

// recheck requires an attestation gh reported to meet the whole policy and
// name the file's digest, so a gh that ignored a flag, or a list of signer
// digests, is still enforced here (0017-PLAN N11).
func (c *checker) recheck(stdout []byte, ref, digest string) error {
	var results []result
	if err := json.Unmarshal(stdout, &results); err != nil {
		return fmt.Errorf("selfupdate: ghattest: gh's output is not the expected JSON: %w", err)
	}
	if len(results) == 0 {
		return fmt.Errorf("selfupdate: ghattest: gh reported success, but no attestation matches the policy: it reported none")
	}
	first := ""
	for _, r := range results {
		field := c.mismatch(r, ref, digest)
		if field == "" {
			return nil
		}
		if first == "" {
			first = field
		}
	}
	return fmt.Errorf("selfupdate: ghattest: gh reported success, but no attestation matches the policy: %s", first)
}

// mismatch is the first field of r that does not meet the policy, or "".
func (c *checker) mismatch(r result, ref, digest string) string {
	p := c.policy
	st, cert := r.VerificationResult.Statement, r.VerificationResult.Signature.Certificate
	switch {
	case st.PredicateType != PredicateType:
		return "predicateType " + st.PredicateType
	case cert.Issuer != Issuer:
		return "issuer " + cert.Issuer
	case !strings.EqualFold(cert.SourceRepositoryURI, "https://github.com/"+p.Repository.Owner+"/"+p.Repository.Name):
		return "sourceRepositoryURI " + cert.SourceRepositoryURI
	case cert.SourceRepositoryRef != ref:
		return "sourceRepositoryRef " + cert.SourceRepositoryRef
	case !hasFoldPrefix(cert.BuildSignerURI, "https://github.com/"+p.SignerWorkflow+"@"):
		return "buildSignerURI " + cert.BuildSignerURI
	case len(p.SignerDigests) > 0 && !slices.Contains(p.SignerDigests, cert.BuildSignerDigest):
		return "buildSignerDigest " + cert.BuildSignerDigest
	case !p.AllowSelfHostedRunners && cert.RunnerEnvironment != "github-hosted":
		return "runnerEnvironment " + cert.RunnerEnvironment
	}
	for _, s := range st.Subject {
		if s.Digest["sha256"] == digest {
			return ""
		}
	}
	return "subject: no sha256 " + digest
}

func hasFoldPrefix(s, prefix string) bool {
	return len(s) >= len(prefix) && strings.EqualFold(s[:len(prefix)], prefix)
}

// detail is gh's output for an error: control characters folded to
// spaces, on one line, at most maxDetail bytes (as codesign's is,
// selfupdate/codesign/codesign.go).
func detail(out service.Output) string {
	s := strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f {
			return ' '
		}
		return r
	}, string(bytes.TrimSpace(out.Stderr))+" "+string(bytes.TrimSpace(out.Stdout)))
	s = strings.Join(strings.Fields(s), " ")
	if len(s) > maxDetail {
		s = strings.ToValidUTF8(s[:maxDetail], "") + "…"
	}
	if !utf8.ValidString(s) {
		s = strings.ToValidUTF8(s, "?")
	}
	return s
}
