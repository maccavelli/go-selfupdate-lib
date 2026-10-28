package selfupdate

import (
	"context"
	"errors"
	"io"
	"net/http"
	"testing"
)

// Typed HTTP errors (docs/decisions/0025-MADR-align-with-go-sdk-design-practices.md
// D2): a status no sentinel names is an *HTTPError, with the text it had
// before.

func TestHTTPErrorAs(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	for _, c := range []struct {
		name      string
		status    int
		call      func(*GitHubSource) error
		want      string
		noRelease bool
	}{
		{"latest 500", http.StatusInternalServerError, func(s *GitHubSource) error {
			_, err := s.Latest(ctx)
			return err
		}, "selfupdate: github http 500: boom", false},
		{"latest 404", http.StatusNotFound, func(s *GitHubSource) error {
			_, err := s.Latest(ctx)
			return err
		}, "selfupdate: github http 404: boom", true},
		{"asset 404", http.StatusNotFound, func(s *GitHubSource) error {
			rel := Release{ID: 1, Tag: "v1.0.0", Assets: []Asset{{ID: 7, Name: "demo", State: AssetStateUploaded, Size: 4}}}
			rc, err := s.OpenAsset(ctx, rel, rel.Assets[0])
			if rc != nil {
				_, _ = io.Copy(io.Discard, rc)
				_ = rc.Close()
			}
			return err
		}, "selfupdate: github http 404: boom", false},
	} {
		env := newGitHubEnv(t, func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(c.status)
			_, _ = io.WriteString(w, "boom")
		}, "")
		err := c.call(env.src)
		var he *HTTPError
		if !errors.As(err, &he) {
			t.Errorf("%s: err = %v, want an *HTTPError", c.name, err)
			continue
		}
		if he.Source != "github" || he.StatusCode != c.status || he.Error() != c.want {
			t.Errorf("%s: HTTPError = %+v (%q), want github %d %q", c.name, *he, he.Error(), c.status, c.want)
		}
		if got := errors.Is(err, ErrNoRelease); got != c.noRelease {
			t.Errorf("%s: errors.Is(err, ErrNoRelease) = %t, want %t", c.name, got, c.noRelease)
		}
	}
}

// causeRecorder offers a fresh credential each time and records why it was
// asked.
type causeRecorder struct{ causes []error }

func (r *causeRecorder) Credential(_ context.Context, req CredentialRequest) (Credential, error) {
	r.causes = append(r.causes, req.Cause)
	return Credential{Value: []byte{byte('a' + len(r.causes))}, Source: "fixture"}, nil
}

// TestRefreshCauseIsHTTPError: the provider asked again after a 401 is
// told why with an *HTTPError, whose text is unchanged.
func TestRefreshCauseIsHTTPError(t *testing.T) {
	t.Parallel()
	rec := &causeRecorder{}
	env := newGitHubEnv(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}, "")
	src, err := NewGitHubSource(GitHubOptions{
		Repository: env.src.repo, Client: env.client, APIBaseURL: env.src.apiBase,
		UserAgent: "demo/v1.0.0", Limits: testGitHubLimits(), Credentials: rec,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := src.Latest(context.Background()); err == nil {
		t.Fatal("Latest succeeded against a server that refuses every credential")
	}
	if len(rec.causes) != 2 {
		t.Fatalf("the provider was asked %d times, want 2", len(rec.causes))
	}
	var he *HTTPError
	if !errors.As(rec.causes[1], &he) || he.StatusCode != http.StatusUnauthorized ||
		he.Error() != "selfupdate: github http 401: credential from fixture refused" {
		t.Fatalf("refresh cause = %v, want the 401 as an *HTTPError", rec.causes[1])
	}
}

func TestHTTPErrorText(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		e    *HTTPError
		want string
	}{
		{nil, "selfupdate: http error"},
		{&HTTPError{StatusCode: 502, Diagnostic: "bad gateway"}, "selfupdate: http 502: bad gateway"},
		{&HTTPError{Source: "github", StatusCode: 503, Diagnostic: "x"}, "selfupdate: github http 503: x"},
	} {
		if got := c.e.Error(); got != c.want {
			t.Errorf("Error() = %q, want %q", got, c.want)
		}
	}
}
