package selfupdate_test

import (
	"context"
	"sync"
	"testing"

	"github.com/maccavelli/go-selfupdate-lib/selfupdate"
)

// Regression tests for docs/decisions/0010-MADR-remediate-second-debugging-pass-findings.md
// A2 and C10 (docs/decisions/0010-PLAN-v1-5-1-contract-preserving-fixes.md P2).

// TestPromptAfterStartupCheck: a startup Check outside a Stream, where
// PromptCredential has nothing to offer, does not stop a later Start on the
// same Updater from prompting (A2).
func TestPromptAfterStartupCheck(t *testing.T) {
	gh := e2eServer(t)
	gh.RequireToken("good")
	u, _ := e2eUpdater(t, gh, selfupdate.GitHubOptions{Credentials: selfupdate.PromptCredential()})
	if _, err := u.Checker().Check(context.Background(), selfupdate.CheckRequest{
		Product: "demo", CurrentVersion: "v1.0.0", CurrentBuild: selfupdate.ReleaseBuild, Platform: goldenPlatform,
	}); err == nil {
		t.Fatal("the anonymous startup check succeeded against a token-only server")
	}
	n, fin := answerCredentials(t, selfupdate.Start(context.Background(), u, e2eReq()), func(_ int, c *selfupdate.CredentialNeeded) {
		c.Supply(selfupdate.Credential{Value: []byte("good"), Source: "prompt"})
	})
	if n != 1 || fin.Err != nil || !fin.Result.Applied {
		t.Fatalf("later Start: prompts=%d applied=%v err=%v, want one prompt and an applied update", n, fin.Result.Applied, fin.Err)
	}
}

// TestPromptAgainAfterSkip: a skipped prompt holds for its own run only;
// the next Start asks again (A2).
func TestPromptAgainAfterSkip(t *testing.T) {
	gh := e2eServer(t)
	gh.RequireToken("good")
	u, _ := e2eUpdater(t, gh, selfupdate.GitHubOptions{Credentials: selfupdate.PromptCredential()})
	n1, fin1 := answerCredentials(t, selfupdate.Start(context.Background(), u, e2eReq()), func(_ int, c *selfupdate.CredentialNeeded) {
		c.Cancel(nil)
	})
	if n1 != 1 || fin1.Err == nil {
		t.Fatalf("skipped Start: prompts=%d err=%v, want one prompt and a 401", n1, fin1.Err)
	}
	n2, fin2 := answerCredentials(t, selfupdate.Start(context.Background(), u, e2eReq()), func(_ int, c *selfupdate.CredentialNeeded) {
		c.Supply(selfupdate.Credential{Value: []byte("good"), Source: "prompt"})
	})
	if n2 != 1 || fin2.Err != nil || !fin2.Result.Applied {
		t.Fatalf("second Start: prompts=%d applied=%v err=%v, want one prompt and an applied update", n2, fin2.Result.Applied, fin2.Err)
	}
}

type interactiveRecorder struct {
	mu   sync.Mutex
	seen []bool
}

func (r *interactiveRecorder) Credential(_ context.Context, req selfupdate.CredentialRequest) (selfupdate.Credential, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.seen = append(r.seen, req.Interactive)
	return selfupdate.Credential{Value: []byte("good"), Source: "recorder"}, nil
}

func (r *interactiveRecorder) last() []bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]bool(nil), r.seen...)
}

// TestCredentialRequestInteractive: a provider is told it may prompt when,
// and only when, the run carries a Stream (C10).
func TestCredentialRequestInteractive(t *testing.T) {
	for _, viaStart := range []bool{false, true} {
		gh := e2eServer(t)
		gh.RequireToken("good")
		rec := &interactiveRecorder{}
		u, _ := e2eUpdater(t, gh, selfupdate.GitHubOptions{Credentials: rec})
		if viaStart {
			_, fin := answerCredentials(t, selfupdate.Start(context.Background(), u, e2eReq()), func(int, *selfupdate.CredentialNeeded) {
				t.Error("a provider that never prompts produced a prompt")
			})
			if fin.Err != nil {
				t.Fatalf("Start: %v", fin.Err)
			}
		} else if _, err := u.Run(context.Background(), e2eReq()); err != nil {
			t.Fatalf("Run: %v", err)
		}
		seen := rec.last()
		if len(seen) == 0 {
			t.Fatalf("viaStart=%v: the provider was never asked", viaStart)
		}
		for _, got := range seen {
			if got != viaStart {
				t.Fatalf("viaStart=%v: Interactive = %v", viaStart, got)
			}
		}
	}
}
