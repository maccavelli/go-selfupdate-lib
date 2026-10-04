package selfupdate

import (
	"errors"
	"testing"
	"time"
)

// Tests for docs/decisions/0010-PLAN-v1-6-0-owner-contracts.md S5 (Q5, C9):
// the zero values of ConfirmNeeded and CredentialNeeded are safe to answer.
// These use only the v1.5.1 API, so they run against the unfixed code too.

// returnsAtOnce fails the test when f blocks. The bound is generous, so a
// slow, raced CI host cannot flake it; a blocked send never returns.
func returnsAtOnce(t *testing.T, name string, f func()) {
	t.Helper()
	done := make(chan struct{})
	go func() {
		defer close(done)
		f()
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Errorf("%s on a zero value blocked", name)
	}
}

func TestZeroValueRepliesReturn(t *testing.T) {
	defer checkNoLeak(t)()
	returnsAtOnce(t, "ConfirmNeeded.Answer", func() { var c ConfirmNeeded; c.Answer(true) })
	returnsAtOnce(t, "ConfirmNeeded.Cancel", func() { var c ConfirmNeeded; c.Cancel(nil) })
	returnsAtOnce(t, "CredentialNeeded.Supply", func() { var c CredentialNeeded; c.Supply(Credential{Value: []byte("t")}) })
	returnsAtOnce(t, "CredentialNeeded.Cancel", func() { var c CredentialNeeded; c.Cancel(nil) })
	returnsAtOnce(t, "a second reply", func() {
		var c CredentialNeeded
		c.Cancel(nil)
		c.Supply(Credential{})
	})
}

// queued returns the reply already waiting in ch. A reply is sent before
// Answer, Supply or Cancel returns, so an empty channel fails the test at
// once rather than blocking it.
func queued[T any](t *testing.T, ch chan T) T {
	t.Helper()
	select {
	case r := <-ch:
		return r
	default:
		t.Fatal("no reply was sent")
		panic("unreachable")
	}
}

// TestRealRepliesDeliver: a request made as the Stream makes it still gets
// its first reply, and only that one.
func TestRealRepliesDeliver(t *testing.T) {
	c := &ConfirmNeeded{reply: make(chan confirmReply, 1)}
	c.Answer(true)
	c.Cancel(errors.New("too late"))
	if r := queued(t, c.reply); !r.ok || r.err != nil {
		t.Fatalf("confirm reply %+v, want the approval", r)
	}
	n := &CredentialNeeded{reply: make(chan credentialReply, 1)}
	n.Supply(Credential{Value: []byte("token")})
	n.Cancel(nil)
	if r := queued(t, n.reply); r.err != nil || string(r.cred.Value) != "token" {
		t.Fatalf("credential reply %+v, want the token", r)
	}
	select {
	case r := <-n.reply:
		t.Fatalf("a second reply was sent: %+v", r)
	default:
	}
}
