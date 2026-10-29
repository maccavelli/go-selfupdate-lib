package selfupdate_test

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/maccavelli/go-selfupdate-lib/selfupdate"
	"github.com/maccavelli/go-selfupdate-lib/selfupdate/selfupdatetest"
)

// Tests for docs/decisions/0004-PLAN-v1-2-0-interaction-stream.md Step 4:
// whole runs through a Stream.

func streamUpdater(t *testing.T, src selfupdate.ReleaseSource) (*selfupdate.Updater, string) {
	t.Helper()
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
	selfupdate.SetRunningPlatform(u, goldenPlatform)
	return u, exe
}

func streamReq(yes bool) selfupdate.Request {
	return selfupdate.Request{
		Product: "demo", CurrentVersion: "v1.0.0", CurrentBuild: selfupdate.ReleaseBuild,
		Platform: goldenPlatform, Yes: yes,
	}
}

// drain reads until Finished, answering any ConfirmNeeded with answer. It
// fails when an interaction follows Finished.
func drain(t *testing.T, s *selfupdate.Stream, answer *bool) ([]selfupdate.Interaction, selfupdate.Finished) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	var seen []selfupdate.Interaction
	for {
		it, err := s.Next(ctx)
		if err != nil {
			t.Fatalf("Next after %d interactions: %v", len(seen), err)
		}
		seen = append(seen, it)
		switch v := it.(type) {
		case *selfupdate.ConfirmNeeded:
			if answer == nil {
				t.Fatalf("unexpected ConfirmNeeded %+v", v.Prompt)
			}
			v.Answer(*answer)
		case selfupdate.Finished:
			if it, err := s.Next(ctx); !errors.Is(err, io.EOF) {
				t.Fatalf("after Finished: %#v, %v; want io.EOF", it, err)
			}
			return seen, v
		}
	}
}

func kinds(seen []selfupdate.Interaction) []string {
	var out []string
	for _, it := range seen {
		if p, ok := it.(selfupdate.Progressed); ok {
			out = append(out, p.Event.Kind.String())
		}
	}
	return out
}

func TestStreamDeliversRunInOrder(t *testing.T) {
	selfupdate.CheckNoLeak(t)
	u, exe := streamUpdater(t, selfupdatetest.NewFakeSource("v1.1.0", goldenRelease()))
	seen, fin := drain(t, selfupdate.Start(context.Background(), u, streamReq(true)), nil)
	if fin.Err != nil || !fin.Result.Applied {
		t.Fatalf("Finished = %+v", fin)
	}
	f, err := os.Open(filepath.Join("testdata", "golden", "jsonl-upgrade.golden"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	var want []string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		var line struct{ Kind string }
		if err := json.Unmarshal(sc.Bytes(), &line); err != nil {
			t.Fatal(err)
		}
		want = append(want, line.Kind)
	}
	if got := kinds(seen); !slices.Equal(got, want) {
		t.Fatalf("kinds = %v, want the golden upgrade's %v", got, want)
	}
	if got, _ := os.ReadFile(exe); string(got) != string(releaseBody("v1.1.0")(goldenPlatform)) {
		t.Fatalf("target = %q", got)
	}
}

func TestStreamConfirm(t *testing.T) {
	selfupdate.CheckNoLeak(t)
	for _, answer := range []bool{true, false} {
		u, exe := streamUpdater(t, selfupdatetest.NewFakeSource("v1.1.0", goldenRelease()))
		s := selfupdate.Start(context.Background(), u, streamReq(false))
		var asked *selfupdate.ConfirmNeeded
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		for asked == nil {
			it, err := s.Next(ctx)
			if err != nil {
				t.Fatal(err)
			}
			asked, _ = it.(*selfupdate.ConfirmNeeded)
		}
		cancel()
		if p := asked.Prompt; p.Target != "v1.1.0" || p.Current != "v1.0.0" || p.Operation != selfupdate.OperationUpgrade {
			t.Fatalf("prompt = %+v", p)
		}
		asked.Answer(answer)
		asked.Answer(!answer) // ignored, and must not block
		_, fin := drain(t, s, nil)
		if fin.Err != nil || fin.Result.Applied != answer || fin.Result.Declined == answer {
			t.Fatalf("answer %v: Finished = %+v", answer, fin)
		}
		got, _ := os.ReadFile(exe)
		if answer != (string(got) != "old-bytes") {
			t.Fatalf("answer %v: target = %q", answer, got)
		}
	}
}

func TestStreamCancelWhileConfirming(t *testing.T) {
	selfupdate.CheckNoLeak(t)
	u, exe := streamUpdater(t, selfupdatetest.NewFakeSource("v1.1.0", goldenRelease()))
	s := selfupdate.Start(context.Background(), u, streamReq(false))
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	var asked *selfupdate.ConfirmNeeded
	for asked == nil {
		it, err := s.Next(ctx)
		if err != nil {
			t.Fatal(err)
		}
		asked, _ = it.(*selfupdate.ConfirmNeeded)
	}
	s.Cancel()
	_, fin := drain(t, s, nil)
	if !errors.Is(fin.Err, context.Canceled) || fin.Result.Applied {
		t.Fatalf("Finished = %+v, want context.Canceled", fin)
	}
	asked.Answer(true) // after the run: ignored, and must not block
	if got, _ := os.ReadFile(exe); string(got) != "old-bytes" {
		t.Fatalf("target changed: %q", got)
	}
}

// blockingBinary serves the manifest, and a binary body that blocks until
// the run's context ends.
type blockingBinary struct {
	selfupdate.ReleaseSource
	once    sync.Once
	started chan struct{}
}

type ctxReader struct{ ctx context.Context }

func (r ctxReader) Read([]byte) (int, error) {
	<-r.ctx.Done()
	return 0, r.ctx.Err()
}

func (b *blockingBinary) OpenAsset(ctx context.Context, rel selfupdate.Release, a selfupdate.Asset) (io.ReadCloser, error) {
	if a.Name == "SHA256SUMS" {
		return b.ReleaseSource.OpenAsset(ctx, rel, a)
	}
	b.once.Do(func() { close(b.started) })
	return io.NopCloser(ctxReader{ctx}), nil
}

// TestStreamCancelDuringDownload: Cancel stops a download in progress, and
// Finished arrives only once the run has returned (O6).
func TestStreamCancelDuringDownload(t *testing.T) {
	selfupdate.CheckNoLeak(t)
	src := &blockingBinary{ReleaseSource: selfupdatetest.NewFakeSource("v1.1.0", goldenRelease()), started: make(chan struct{})}
	u, exe := streamUpdater(t, src)
	s := selfupdate.Start(context.Background(), u, streamReq(true))
	select {
	case <-src.started:
	case <-time.After(10 * time.Second):
		t.Fatal("the binary download never started")
	}
	s.Cancel()
	s.Cancel() // idempotent
	seen, fin := drain(t, s, nil)
	if !errors.Is(fin.Err, context.Canceled) {
		t.Fatalf("Finished = %+v, want context.Canceled", fin)
	}
	if k := kinds(seen); len(k) == 0 || k[len(k)-1] != "failed" {
		t.Fatalf("kinds = %v, want the run's own EventFailed before Finished", k)
	}
	entries, err := os.ReadDir(filepath.Dir(exe))
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".demo.selfupdate-") {
			t.Fatalf("staging %s still present when Finished was read", e.Name())
		}
	}
	if got, _ := os.ReadFile(exe); string(got) != "old-bytes" {
		t.Fatalf("target changed: %q", got)
	}
	// A run on an ended context returns at once, and is refused only if the
	// first run still holds the Updater.
	ended, endNow := context.WithCancel(context.Background())
	endNow()
	if _, err := u.Run(ended, streamReq(true)); errors.Is(err, selfupdate.ErrConcurrentUpdate) {
		t.Fatal("the run still held the Updater after Finished")
	}
}

func TestStreamAll(t *testing.T) {
	selfupdate.CheckNoLeak(t)
	u, _ := streamUpdater(t, selfupdatetest.NewFakeSource("v1.1.0", goldenRelease()))
	var last selfupdate.Interaction
	n := 0
	for it := range selfupdate.Start(context.Background(), u, streamReq(true)).All(context.Background()) {
		last, n = it, n+1
	}
	if fin, ok := last.(selfupdate.Finished); !ok || !fin.Result.Applied || n < 2 {
		t.Fatalf("All yielded %d, ending with %#v", n, last)
	}

	u2, _ := streamUpdater(t, selfupdatetest.NewFakeSource("v1.1.0", goldenRelease()))
	s := selfupdate.Start(context.Background(), u2, streamReq(false))
	for it := range s.All(context.Background()) {
		if _, ok := it.(*selfupdate.ConfirmNeeded); ok {
			break
		}
	}
	short, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if it, err := s.Next(short); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("after break: %#v, %v; want the run still waiting", it, err)
	}
	s.Cancel()
	if _, fin := drain(t, s, nil); !errors.Is(fin.Err, context.Canceled) {
		t.Fatalf("Finished = %+v", fin)
	}
}

func TestStreamConcurrentRun(t *testing.T) {
	selfupdate.CheckNoLeak(t)
	u, _ := streamUpdater(t, selfupdatetest.NewFakeSource("v1.1.0", goldenRelease()))
	first := selfupdate.Start(context.Background(), u, streamReq(false))
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	var asked *selfupdate.ConfirmNeeded
	for asked == nil {
		it, err := first.Next(ctx)
		if err != nil {
			t.Fatal(err)
		}
		asked, _ = it.(*selfupdate.ConfirmNeeded)
	}
	if _, fin := drain(t, selfupdate.Start(context.Background(), u, streamReq(true)), nil); !errors.Is(fin.Err, selfupdate.ErrConcurrentUpdate) {
		t.Fatalf("second Start = %+v, want ErrConcurrentUpdate", fin)
	}
	asked.Answer(true)
	if _, fin := drain(t, first, nil); fin.Err != nil || !fin.Result.Applied {
		t.Fatalf("first run = %+v", fin)
	}
}

func TestStartReporterFansOut(t *testing.T) {
	selfupdate.CheckNoLeak(t)
	u, _ := streamUpdater(t, selfupdatetest.NewFakeSource("v1.1.0", goldenRelease()))
	rec := &selfupdatetest.RecordingReporter{}
	seen, _ := drain(t, selfupdate.Start(context.Background(), u, streamReq(true), selfupdate.WithReporter(rec)), nil)
	var recorded []string
	for _, k := range rec.Kinds() {
		recorded = append(recorded, k.String())
	}
	if got := kinds(seen); len(got) == 0 || !slices.Equal(got, recorded) {
		t.Fatalf("stream kinds %v, reporter kinds %v", got, recorded)
	}
}

func TestStartConfirmerReplaces(t *testing.T) {
	selfupdate.CheckNoLeak(t)
	u, _ := streamUpdater(t, selfupdatetest.NewFakeSource("v1.1.0", goldenRelease()))
	conf := &selfupdatetest.ScriptedConfirmer{Answers: []bool{true}}
	_, fin := drain(t, selfupdate.Start(context.Background(), u, streamReq(false), selfupdate.WithConfirmer(conf)), nil)
	if fin.Err != nil || !fin.Result.Applied || len(conf.Prompts()) != 1 {
		t.Fatalf("Finished = %+v, prompts = %d", fin, len(conf.Prompts()))
	}
}

// Tests for Step 5: credential prompts through a Stream.

// answerCredentials reads until Finished, giving each CredentialNeeded to
// answer with its 0-based index.
func answerCredentials(t *testing.T, s *selfupdate.Stream, answer func(int, *selfupdate.CredentialNeeded)) (int, selfupdate.Finished) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	n := 0
	for {
		it, err := s.Next(ctx)
		if err != nil {
			t.Fatalf("Next after %d prompts: %v", n, err)
		}
		switch v := it.(type) {
		case *selfupdate.CredentialNeeded:
			answer(n, v)
			n++
		case selfupdate.Finished:
			return n, v
		}
	}
}

type acceptedTokens struct {
	n    atomic.Int32
	last atomic.Value
}

func (a *acceptedTokens) Accepted(_ context.Context, c selfupdate.Credential) {
	a.n.Add(1)
	a.last.Store(string(c.Value))
}

func TestStreamCredentialPrompt(t *testing.T) {
	selfupdate.CheckNoLeak(t)
	gh := e2eServer(t)
	gh.RequireToken("good")
	seen := &acceptedTokens{}
	u, exe := e2eUpdater(t, gh, selfupdate.GitHubOptions{
		Credentials: selfupdate.ChainCredentials(selfupdate.EnvCredential("", "SELFUPDATE_TEST_UNSET"), selfupdate.PromptCredential()),
		Observer:    seen,
	})
	var causes []error
	n, fin := answerCredentials(t, selfupdate.Start(context.Background(), u, e2eReq()), func(i int, c *selfupdate.CredentialNeeded) {
		causes = append(causes, c.Request.Cause)
		if c.Request.Origin == nil || c.Request.Origin.Host != gh.APIBase.Host {
			t.Errorf("prompt %d origin = %v, want the API origin", i, c.Request.Origin)
		}
		token := "bad"
		if i > 0 {
			token = "good"
		}
		c.Supply(selfupdate.Credential{Value: []byte(token), Source: "prompt"})
		c.Supply(selfupdate.Credential{Value: []byte("ignored")}) // must not block
	})
	if fin.Err != nil || !fin.Result.Applied {
		t.Fatalf("Finished = %+v", fin)
	}
	if n != 2 || causes[0] != nil || causes[1] == nil {
		t.Fatalf("prompts = %d, causes = %v; want a first prompt, then one with the refusal", n, causes)
	}
	if seen.n.Load() != 1 || seen.last.Load() != "good" {
		t.Fatalf("observer told %d times, last %v", seen.n.Load(), seen.last.Load())
	}
	if got, _ := os.ReadFile(exe); string(got) != string(releaseBody("v1.1.0")(goldenPlatform)) {
		t.Fatalf("target = %q", got)
	}
}

func TestStreamCredentialPerRunOption(t *testing.T) {
	selfupdate.CheckNoLeak(t)
	gh := e2eServer(t)
	gh.RequireToken("good")
	u, _ := e2eUpdater(t, gh, selfupdate.GitHubOptions{})
	s := selfupdate.Start(context.Background(), u, e2eReq(), selfupdate.WithCredentials(selfupdate.PromptCredential()))
	n, fin := answerCredentials(t, s, func(_ int, c *selfupdate.CredentialNeeded) {
		c.Supply(selfupdate.Credential{Value: []byte("good"), Source: "prompt"})
	})
	if n != 1 || fin.Err != nil || !fin.Result.Applied {
		t.Fatalf("prompts = %d, Finished = %+v", n, fin)
	}
	if _, err := u.Run(context.Background(), e2eReq()); err == nil || !strings.Contains(err.Error(), "github http 401") {
		t.Fatalf("a plain Run after the Stream = %v, want the 401", err)
	}
}

func TestCredentialNeededCancel(t *testing.T) {
	selfupdate.CheckNoLeak(t)
	t.Run("nil goes anonymous", func(t *testing.T) {
		gh := e2eServer(t)
		gh.RequireToken("good")
		u, exe := e2eUpdater(t, gh, selfupdate.GitHubOptions{Credentials: selfupdate.PromptCredential()})
		_, fin := answerCredentials(t, selfupdate.Start(context.Background(), u, e2eReq()), func(_ int, c *selfupdate.CredentialNeeded) {
			c.Cancel(nil)
		})
		if fin.Err == nil || !strings.Contains(fin.Err.Error(), "github http 401") {
			t.Fatalf("Finished = %+v, want the anonymous 401", fin)
		}
		if got, _ := os.ReadFile(exe); string(got) != "old-bytes" {
			t.Fatalf("target changed: %q", got)
		}
	})
	t.Run("an error ends the run", func(t *testing.T) {
		gh := e2eServer(t)
		u, _ := e2eUpdater(t, gh, selfupdate.GitHubOptions{Credentials: selfupdate.PromptCredential()})
		refused := errors.New("the user closed the dialog")
		_, fin := answerCredentials(t, selfupdate.Start(context.Background(), u, e2eReq()), func(_ int, c *selfupdate.CredentialNeeded) {
			c.Cancel(refused)
		})
		if !errors.Is(fin.Err, refused) {
			t.Fatalf("Finished = %+v, want the host's error", fin)
		}
	})
	t.Run("Stream.Cancel while pending", func(t *testing.T) {
		gh := e2eServer(t)
		u, _ := e2eUpdater(t, gh, selfupdate.GitHubOptions{Credentials: selfupdate.PromptCredential()})
		s := selfupdate.Start(context.Background(), u, e2eReq())
		_, fin := answerCredentials(t, s, func(_ int, _ *selfupdate.CredentialNeeded) {
			s.Cancel()
		})
		if !errors.Is(fin.Err, context.Canceled) {
			t.Fatalf("Finished = %+v, want context.Canceled", fin)
		}
	})
}

// Plans through a Stream and over the fake GitHub API
// (docs/decisions/0038-MADR-pinned-plans-lifecycle-conformance-and-bubble-tests.md
// D1-A; 0038-PLAN Q15).

func newerRelease() selfupdatetest.ReleaseSpec {
	return selfupdatetest.NewRelease("demo", "v1.2.0", []selfupdate.Platform{goldenPlatform}, releaseBody("v1.2.0"))
}

// TestStartApplyConfirms: an Apply stream asks for confirmation as a Run
// stream does, and installs the planned release, not the newer one
// published since.
func TestStartApplyConfirms(t *testing.T) {
	selfupdate.CheckNoLeak(t)
	src := selfupdatetest.NewFakeSource("v1.1.0", goldenRelease())
	u, exe := streamUpdater(t, src)
	plan, err := u.Plan(context.Background(), streamReq(false))
	if err != nil {
		t.Fatal(err)
	}
	src.Publish(newerRelease())
	yes := true
	seen, fin := drain(t, selfupdate.StartApply(context.Background(), u, plan.Request(), plan), &yes)
	var prompts []selfupdate.Prompt
	for _, it := range seen {
		if c, ok := it.(*selfupdate.ConfirmNeeded); ok {
			prompts = append(prompts, c.Prompt)
		}
	}
	if len(prompts) != 1 || prompts[0].Target != "v1.1.0" || prompts[0].Operation != selfupdate.OperationUpgrade {
		t.Fatalf("prompts = %+v, want one for v1.1.0", prompts)
	}
	if fin.Err != nil || !fin.Result.Applied || fin.Result.TargetVersion != "v1.1.0" {
		t.Fatalf("Finished = %+v", fin)
	}
	if got, _ := os.ReadFile(exe); string(got) != string(releaseBody("v1.1.0")(goldenPlatform)) {
		t.Fatalf("target = %q, want the planned v1.1.0", got)
	}
}

// TestStartApplyRefusesBeforeRunning: a nil Updater or an invalid plan
// gives a Stream whose only interaction is Finished, with no source call.
func TestStartApplyRefusesBeforeRunning(t *testing.T) {
	selfupdate.CheckNoLeak(t)
	src := selfupdatetest.NewFakeSource("v1.1.0", goldenRelease())
	u, _ := streamUpdater(t, src)
	for name, s := range map[string]*selfupdate.Stream{
		"nil updater":  selfupdate.StartApply(context.Background(), nil, streamReq(true), selfupdate.Plan{}),
		"invalid plan": selfupdate.StartApply(context.Background(), u, streamReq(true), selfupdate.Plan{}),
	} {
		seen, fin := drain(t, s, nil)
		if len(seen) != 1 || fin.Err == nil || errors.Is(fin.Err, selfupdate.ErrPlanStale) {
			t.Fatalf("%s: %d interactions, Finished = %+v", name, len(seen), fin)
		}
	}
	if calls := src.Calls(); len(calls) != 0 {
		t.Fatalf("source calls %v, want none", calls)
	}
}

// TestPlanSurvivesNewerReleaseOnGitHub is the MADR's end-to-end check: over
// the fake GitHub API, a plan made for v1.1.0 installs v1.1.0 after v1.2.0
// is published, and a Run after it installs v1.2.0.
func TestPlanSurvivesNewerReleaseOnGitHub(t *testing.T) {
	t.Parallel()
	gh := selfupdatetest.NewGitHubServer(t, "owner", "demo", goldenRelease())
	src, err := selfupdate.NewGitHubSource(selfupdate.GitHubOptions{
		Repository: selfupdate.Repository{Owner: "owner", Name: "demo"},
		Client:     gh.Client, APIBaseURL: gh.APIBase, UserAgent: "demo/v1.0.0", Limits: selfupdate.DefaultLimits(),
	})
	if err != nil {
		t.Fatal(err)
	}
	u, exe := streamUpdater(t, src)
	ctx := context.Background()
	plan, err := u.Plan(ctx, streamReq(false))
	if err != nil {
		t.Fatal(err)
	}
	gh.Publish(newerRelease())
	req := plan.Request()
	req.Yes = true
	res, err := u.Apply(ctx, req, plan)
	if err != nil || !res.Applied || res.TargetVersion != "v1.1.0" {
		t.Fatalf("Apply = %+v, %v", res, err)
	}
	if got, _ := os.ReadFile(exe); string(got) != string(releaseBody("v1.1.0")(goldenPlatform)) {
		t.Fatalf("after Apply the target = %q, want the planned v1.1.0", got)
	}
	run := streamReq(true)
	run.CurrentVersion = "v1.1.0"
	res, err = u.Run(ctx, run)
	if err != nil || !res.Applied || res.TargetVersion != "v1.2.0" {
		t.Fatalf("Run = %+v, %v", res, err)
	}
	if got, _ := os.ReadFile(exe); string(got) != string(releaseBody("v1.2.0")(goldenPlatform)) {
		t.Fatalf("after Run the target = %q, want v1.2.0", got)
	}
}
