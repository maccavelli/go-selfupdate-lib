package selfupdate

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"testing"
)

// Tests for docs/decisions/0005-PLAN-opt-in-prerelease-channels.md Step 4:
// GitHubSource.ListReleases.

type listServer struct {
	srv      *httptest.Server
	total    int
	mu       sync.Mutex
	pages    []string // the page parameter of each request
	auth     []string
	tooBig   bool // pad the first page past any small limit
	limitAt2 bool // answer page 2 with a 429
}

func newListServer(t *testing.T, total int) *listServer {
	t.Helper()
	ls := &listServer{total: total}
	ls.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/repos/maccavelli/demo/releases" {
			http.NotFound(w, r)
			return
		}
		q := r.URL.Query()
		page, _ := strconv.Atoi(q.Get("page"))
		per, _ := strconv.Atoi(q.Get("per_page"))
		ls.mu.Lock()
		ls.pages = append(ls.pages, q.Get("page"))
		ls.auth = append(ls.auth, r.Header.Get("Authorization"))
		tooBig, limitAt2 := ls.tooBig, ls.limitAt2
		ls.mu.Unlock()
		if limitAt2 && page == 2 {
			w.Header().Set("Retry-After", "30")
			http.Error(w, `{"message":"API rate limit exceeded"}`, http.StatusTooManyRequests)
			return
		}
		var out []githubReleaseJSON
		for i := (page - 1) * per; i < ls.total && i < page*per; i++ {
			out = append(out, githubReleaseJSON{ID: int64(i + 1), TagName: fmt.Sprintf("v1.0.%d", i), Immutable: true,
				Assets: []githubAssetJSON{{ID: int64(1000 + i), Name: "demo-linux-amd64", State: "uploaded", Size: 1}}})
		}
		if tooBig && page == 1 && len(out) > 0 {
			out[0].HTMLURL = strings.Repeat("x", 4096)
		}
		_ = json.NewEncoder(w).Encode(out)
	}))
	t.Cleanup(ls.srv.Close)
	return ls
}

func (ls *listServer) source(t *testing.T, limits Limits) *GitHubSource {
	t.Helper()
	base, err := url.Parse(ls.srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	src, err := NewGitHubSource(GitHubOptions{
		Repository: Repository{Owner: "maccavelli", Name: "demo"},
		Client:     ls.srv.Client(), APIBaseURL: base, UserAgent: "demo/v1.0.0",
		Token: "list-token", Limits: limits,
	})
	if err != nil {
		t.Fatal(err)
	}
	return src
}

func (ls *listServer) requests() ([]string, []string) {
	ls.mu.Lock()
	defer ls.mu.Unlock()
	return append([]string(nil), ls.pages...), append([]string(nil), ls.auth...)
}

func TestGitHubSourceListReleases(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	ls := newListServer(t, 70)
	rels, err := ls.source(t, DefaultLimits()).ListReleases(ctx, ListOptions{})
	if err != nil {
		t.Fatal(err)
	}
	pages, auth := ls.requests()
	if len(rels) != 70 || strings.Join(pages, ",") != "1,2,3" {
		t.Fatalf("got %d releases from pages %v; want 70 from 1,2,3 (the third short)", len(rels), pages)
	}
	for i, a := range auth {
		if a != "Bearer list-token" {
			t.Errorf("page %d sent Authorization %q", i+1, a)
		}
	}
	if rels[0].Tag != "v1.0.0" || rels[69].Tag != "v1.0.69" || len(rels[5].Assets) != 1 {
		t.Fatalf("releases out of order or empty: %+v … %+v", rels[0], rels[69])
	}

	ls = newListServer(t, 200)
	rels, err = ls.source(t, DefaultLimits()).ListReleases(ctx, ListOptions{Limit: 40})
	pages, _ = ls.requests()
	if err != nil || len(rels) != 40 || strings.Join(pages, ",") != "1,2" {
		t.Fatalf("Limit 40: %d releases from pages %v, %v; want 40 from 1,2", len(rels), pages, err)
	}

	ls = newListServer(t, 200)
	rels, err = ls.source(t, DefaultLimits()).ListReleases(ctx, ListOptions{})
	pages, _ = ls.requests()
	if err != nil || len(rels) != 90 || len(pages) != 3 {
		t.Fatalf("the default: %d releases from %d pages, %v; want 90 from 3", len(rels), len(pages), err)
	}

	for _, limit := range []int{-1, 301} {
		if _, err := ls.source(t, DefaultLimits()).ListReleases(ctx, ListOptions{Limit: limit}); err == nil {
			t.Errorf("Limit %d accepted", limit)
		}
	}
}

func TestGitHubSourceListReleasesBounds(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	ls := newListServer(t, 70)
	ls.tooBig = true
	small := DefaultLimits()
	small.ReleaseJSON = 4096
	if _, err := ls.source(t, small).ListReleases(ctx, ListOptions{}); err == nil {
		t.Fatal("a page larger than Limits.ReleaseJSON was accepted")
	}

	ls = newListServer(t, 70)
	ls.limitAt2 = true
	_, err := ls.source(t, DefaultLimits()).ListReleases(ctx, ListOptions{})
	var rl *RateLimitError
	if !errors.As(err, &rl) || rl.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("a 429 on page 2 = %v, want a RateLimitError", err)
	}
}
