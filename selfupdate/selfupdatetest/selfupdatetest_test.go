package selfupdatetest_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/maccavelli/go-selfupdate-lib/selfupdate"
	"github.com/maccavelli/go-selfupdate-lib/selfupdate/selfupdatetest"
)

var plats = []selfupdate.Platform{{OS: "linux", Arch: "amd64"}, {OS: "darwin", Arch: "arm64"}}

func body(p selfupdate.Platform) []byte { return []byte("demo " + p.OS + "/" + p.Arch) }

func sha(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// TestRequireCredentialForms: each form requires exactly what selfupdate
// sends, and Requests records the credential headers' names, never their
// values (0010-MADR A14, amendment A5).
func TestRequireCredentialForms(t *testing.T) {
	type send struct{ header, value string }
	for _, c := range []struct {
		name          string
		header, value string
		send          []send
		wantStatus    int
		wantRecorded  selfupdatetest.HeaderNames
	}{
		{"bearer", "", "t1", []send{{"Authorization", "Bearer t1"}}, 200, "Authorization"},
		{"bearer, raw value", "", "t1", []send{{"Authorization", "t1"}}, 401, "Authorization"},
		{"named Authorization is raw", "Authorization", "token t1", []send{{"Authorization", "token t1"}}, 200, "Authorization"},
		{"named Authorization refuses Bearer", "Authorization", "t1", []send{{"Authorization", "Bearer t1"}}, 401, "Authorization"},
		{"custom", "x-demo-key", "k1", []send{{"X-Demo-Key", "k1"}}, 200, "X-Demo-Key"},
		{"custom, wrong value", "X-Demo-Key", "k1", []send{{"X-Demo-Key", "k2"}}, 401, "X-Demo-Key"},
		{"custom, both sent", "X-Demo-Key", "k1", []send{{"Authorization", "Bearer k1"}, {"X-Demo-Key", "k1"}}, 200, "Authorization, X-Demo-Key"},
		{"anonymous", "", "", nil, 200, ""},
	} {
		t.Run(c.name, func(t *testing.T) {
			gh := selfupdatetest.NewGitHubServer(t, "o", "r", selfupdatetest.NewRelease("demo", "v1.0.0", plats, body))
			gh.RequireCredential(c.header, c.value)
			req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, gh.APIBase.String()+"/repos/o/r/releases/latest", nil)
			if err != nil {
				t.Fatal(err)
			}
			for _, s := range c.send {
				req.Header.Set(s.header, s.value)
			}
			resp, err := gh.Client.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			_ = resp.Body.Close()
			if resp.StatusCode != c.wantStatus {
				t.Fatalf("status %d, want %d", resp.StatusCode, c.wantStatus)
			}
			got := gh.Requests()
			if len(got) != 1 || got[0].CredentialHeaders != c.wantRecorded {
				t.Fatalf("recorded %+v, want %q", got, c.wantRecorded)
			}
			if strings.Contains(string(got[0].CredentialHeaders), "k1") || strings.Contains(string(got[0].CredentialHeaders), "t1") {
				t.Fatalf("a value was recorded: %q", got[0].CredentialHeaders)
			}
		})
	}
}

func TestHeaderNames(t *testing.T) {
	h := selfupdatetest.HeaderNames("Authorization, X-Demo-Key")
	if !slices.Equal(h.List(), []string{"Authorization", "X-Demo-Key"}) || !h.Has("x-demo-key") || h.Has("X-Other") {
		t.Fatalf("List %q", h.List())
	}
	var none selfupdatetest.HeaderNames
	if none.List() != nil || none.Has("Authorization") {
		t.Fatal("the zero value lists names")
	}
}

func TestNewReleaseDigests(t *testing.T) {
	spec := selfupdatetest.NewRelease("demo", "v1.1.0", plats, body)
	if spec.Tag != "v1.1.0" || !spec.Immutable || spec.Draft || spec.Prerelease {
		t.Fatalf("spec flags = %+v", spec)
	}
	if len(spec.Assets) != len(plats)+1 {
		t.Fatalf("assets = %d, want %d binaries and SHA256SUMS", len(spec.Assets), len(plats))
	}
	manifest := spec.Assets[len(spec.Assets)-1]
	if manifest.Name != "SHA256SUMS" {
		t.Fatalf("last asset = %q", manifest.Name)
	}
	entries, err := selfupdate.ParseSHA256SUMS(manifest.Body)
	if err != nil {
		t.Fatal(err)
	}
	for i, p := range plats {
		a := spec.Assets[i]
		if a.Name != selfupdate.ExactAssetName("demo", p) || string(a.Body) != string(body(p)) {
			t.Fatalf("asset %d = %q %q", i, a.Name, a.Body)
		}
		if entries[a.Name] != sha(a.Body) {
			t.Fatalf("manifest digest for %s = %q, want %q", a.Name, entries[a.Name], sha(a.Body))
		}
	}
}

func TestFakeSource(t *testing.T) {
	spec := selfupdatetest.NewRelease("demo", "v1.1.0", plats, body)
	spec.Assets[1].OmitDigest = true
	spec.Assets[1].State = "starter"
	src := selfupdatetest.NewFakeSource("v1.1.0", selfupdatetest.NewRelease("demo", "v1.0.0", plats, body), spec)
	ctx := context.Background()
	rel, err := src.Latest(ctx)
	if err != nil || rel.Tag != "v1.1.0" || !rel.Immutable || rel.ID <= 0 {
		t.Fatalf("Latest = %+v, %v", rel, err)
	}
	a0 := rel.Assets[0]
	if a0.State != selfupdate.AssetStateUploaded || a0.Size != int64(len(body(plats[0]))) || a0.Digest != "sha256:"+sha(body(plats[0])) {
		t.Fatalf("asset 0 = %+v", a0)
	}
	if a1 := rel.Assets[1]; a1.Digest != "" || a1.State != "starter" {
		t.Fatalf("asset 1 = %+v, want no digest and the spec's state", a1)
	}
	ids := map[int64]bool{rel.ID: true}
	for _, a := range rel.Assets {
		if ids[a.ID] {
			t.Fatalf("duplicate ID %d", a.ID)
		}
		ids[a.ID] = true
	}
	rc, err := src.OpenAsset(ctx, rel, a0)
	if err != nil {
		t.Fatal(err)
	}
	got, err := io.ReadAll(rc)
	if err != nil || string(got) != string(body(plats[0])) {
		t.Fatalf("OpenAsset = %q, %v", got, err)
	}
	if err := rc.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := src.ByTag(ctx, "v9.9.9"); err == nil {
		t.Fatal("an unknown tag was found")
	}
	old, err := src.ByTag(ctx, "v1.0.0")
	if err != nil || old.Tag != "v1.0.0" {
		t.Fatalf("ByTag = %+v, %v", old, err)
	}
	want := []string{"Latest", "OpenAsset " + a0.Name, "ByTag v9.9.9", "ByTag v1.0.0"}
	if !slices.Equal(src.Calls(), want) {
		t.Fatalf("calls = %v, want %v", src.Calls(), want)
	}
	if _, err := selfupdatetest.NewFakeSource("").Latest(ctx); err == nil {
		t.Fatal("Latest with no latest release succeeded")
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := src.Latest(canceled); !errors.Is(err, context.Canceled) {
		t.Fatalf("Latest on a canceled context = %v", err)
	}
}

func TestRecordingReporter(t *testing.T) {
	var r selfupdatetest.RecordingReporter
	for _, k := range []selfupdate.EventKind{selfupdate.EventSelected, selfupdate.EventComplete} {
		if err := r.Report(context.Background(), selfupdate.Event{Kind: k, Product: "demo"}); err != nil {
			t.Fatal(err)
		}
	}
	if !slices.Equal(r.Kinds(), []selfupdate.EventKind{selfupdate.EventSelected, selfupdate.EventComplete}) {
		t.Fatalf("kinds = %v", r.Kinds())
	}
	if ev := r.Events(); len(ev) != 2 || ev[1].Product != "demo" {
		t.Fatalf("events = %+v", ev)
	}
}

func TestScriptedConfirmer(t *testing.T) {
	c := &selfupdatetest.ScriptedConfirmer{Answers: []bool{true, false}}
	ctx := context.Background()
	for i, want := range []bool{true, false} {
		got, err := c.Confirm(ctx, selfupdate.Prompt{Target: "v" + string(rune('1'+i))})
		if err != nil || got != want {
			t.Fatalf("answer %d = %v, %v", i, got, err)
		}
	}
	if _, err := c.Confirm(ctx, selfupdate.Prompt{}); err == nil {
		t.Fatal("a prompt past the script was answered")
	}
	if p := c.Prompts(); len(p) != 3 || p[0].Target != "v1" || p[1].Target != "v2" {
		t.Fatalf("prompts = %+v", p)
	}
	failing := &selfupdatetest.ScriptedConfirmer{Answers: []bool{true}, Err: errors.New("no terminal")}
	if ok, err := failing.Confirm(ctx, selfupdate.Prompt{}); ok || err == nil || err.Error() != "no terminal" {
		t.Fatalf("Confirm with Err = %v, %v", ok, err)
	}
}

// fetched is one response, read to the end and closed.
type fetched struct {
	status  int
	header  http.Header
	body    []byte
	readErr error
}

func get(t *testing.T, g *selfupdatetest.GitHubServer, path string) fetched {
	t.Helper()
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, g.APIBase.String()+path, http.NoBody)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer test")
	resp, err := g.Client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, rerr := io.ReadAll(resp.Body)
	return fetched{status: resp.StatusCode, header: resp.Header, body: b, readErr: rerr}
}

type releaseDoc struct {
	ID        int64  `json:"id"`
	TagName   string `json:"tag_name"`
	Immutable bool   `json:"immutable"`
	Assets    []struct {
		ID     int64  `json:"id"`
		Name   string `json:"name"`
		Size   int64  `json:"size"`
		Digest string `json:"digest"`
	} `json:"assets"`
}

func decode(t *testing.T, f fetched) releaseDoc {
	t.Helper()
	var doc releaseDoc
	if err := json.Unmarshal(f.body, &doc); err != nil {
		t.Fatal(err)
	}
	return doc
}

func TestGitHubServer(t *testing.T) {
	pre := selfupdatetest.NewRelease("demo", "v1.2.0-rc.1", plats, body)
	pre.Prerelease = true
	g := selfupdatetest.NewGitHubServer(t, "owner", "repo",
		selfupdatetest.NewRelease("demo", "v1.0.0", plats, body),
		selfupdatetest.NewRelease("demo", "v1.1.0", plats, body), pre)

	latest := get(t, g, "/repos/owner/repo/releases/latest")
	if latest.status != http.StatusOK {
		t.Fatalf("latest status = %d", latest.status)
	}
	doc := decode(t, latest)
	if doc.TagName != "v1.1.0" || !doc.Immutable || len(doc.Assets) != 3 {
		t.Fatalf("latest = %+v, want v1.1.0: the last non-prerelease", doc)
	}
	if got := get(t, g, "/repos/owner/repo/releases/tags/v1.0.0"); decode(t, got).TagName != "v1.0.0" {
		t.Fatal("tags/v1.0.0 served another release")
	}
	if got := get(t, g, "/repos/owner/repo/releases/tags/v9.9.9"); got.status != http.StatusNotFound {
		t.Fatalf("unknown tag status = %d", got.status)
	}

	asset := doc.Assets[0]
	got := get(t, g, "/repos/owner/repo/releases/assets/"+itoa(asset.ID))
	if got.readErr != nil || string(got.body) != string(body(plats[0])) || "sha256:"+sha(got.body) != asset.Digest {
		t.Fatalf("asset body = %q, %v", got.body, got.readErr)
	}
	reqs := g.Requests()
	last2 := reqs[len(reqs)-2:]
	if last2[0].Host == last2[1].Host || !strings.HasSuffix(last2[0].Path, "/assets/"+itoa(asset.ID)) {
		t.Fatalf("asset requests = %+v, want an API request then another origin", last2)
	}
	if !last2[0].Authorization {
		t.Fatalf("the API request lost its Authorization: %+v", last2[0])
	}

	g.TruncateAssets(true)
	if short := get(t, g, "/repos/owner/repo/releases/assets/"+itoa(asset.ID)); !errors.Is(short.readErr, io.ErrUnexpectedEOF) {
		t.Fatalf("truncated read = %v, want io.ErrUnexpectedEOF", short.readErr)
	}
	g.TruncateAssets(false)

	g.RateLimit(http.StatusTooManyRequests, http.Header{"Retry-After": {"30"}})
	limited := get(t, g, "/repos/owner/repo/releases/latest")
	if limited.status != http.StatusTooManyRequests || limited.header.Get("Retry-After") != "30" {
		t.Fatalf("rate limit = %d %q", limited.status, limited.header.Get("Retry-After"))
	}
	g.RateLimit(0, nil)
	if got := get(t, g, "/repos/owner/repo/releases/latest"); got.status != http.StatusOK {
		t.Fatalf("after clearing the limit, status = %d", got.status)
	}

	// get sends "Authorization: Bearer test".
	g.RequireToken("other")
	if got := get(t, g, "/repos/owner/repo/releases/latest"); got.status != http.StatusUnauthorized {
		t.Fatalf("a wrong token: status = %d, want 401", got.status)
	}
	g.RequireToken("test")
	if got := get(t, g, "/repos/owner/repo/releases/latest"); got.status != http.StatusOK {
		t.Fatalf("the required token: status = %d", got.status)
	}
	short := get(t, g, "/repos/owner/repo/releases/assets/"+itoa(asset.ID))
	if short.readErr != nil || string(short.body) != string(body(plats[0])) {
		t.Fatalf("an asset under RequireToken: %q, %v", short.body, short.readErr)
	}
	g.RequireToken("")
}

func itoa(n int64) string { return strconv.FormatInt(n, 10) }

func TestFakeSourceListReleases(t *testing.T) {
	pre := selfupdatetest.NewRelease("demo", "v1.2.0-rc.1", plats, body)
	pre.Prerelease = true
	draft := selfupdatetest.NewRelease("demo", "v1.3.0", plats, body)
	draft.Draft = true
	src := selfupdatetest.NewFakeSource("v1.1.0",
		selfupdatetest.NewRelease("demo", "v1.1.0", plats, body), pre, draft)
	rels, err := src.ListReleases(context.Background(), selfupdate.ListOptions{})
	if err != nil {
		t.Fatal(err)
	}
	var tags []string
	for _, r := range rels {
		tags = append(tags, r.Tag)
	}
	if strings.Join(tags, ",") != "v1.1.0,v1.2.0-rc.1,v1.3.0" || !rels[1].Prerelease || !rels[2].Draft {
		t.Fatalf("ListReleases = %v, want every release in declared order, flags kept", tags)
	}
	if rels, _ := src.ListReleases(context.Background(), selfupdate.ListOptions{Limit: 2}); len(rels) != 2 {
		t.Fatalf("Limit 2 returned %d", len(rels))
	}
	if !slices.Contains(src.Calls(), "ListReleases") {
		t.Fatalf("calls = %v", src.Calls())
	}
}

func TestGitHubServerList(t *testing.T) {
	var specs []selfupdatetest.ReleaseSpec
	for i := range 35 {
		specs = append(specs, selfupdatetest.NewRelease("demo", "v1.0."+itoa(int64(i)), plats, body))
	}
	g := selfupdatetest.NewGitHubServer(t, "owner", "repo", specs...)
	tags := func(path string) []string {
		got := get(t, g, path)
		if got.status != http.StatusOK {
			t.Fatalf("%s: status %d", path, got.status)
		}
		var docs []struct {
			TagName string `json:"tag_name"`
		}
		if err := json.Unmarshal(got.body, &docs); err != nil {
			t.Fatal(err)
		}
		var out []string
		for _, d := range docs {
			out = append(out, d.TagName)
		}
		return out
	}
	if first := tags("/repos/owner/repo/releases"); len(first) != 30 || first[0] != "v1.0.0" {
		t.Fatalf("the default page = %d releases from %v", len(first), first[:1])
	}
	if second := tags("/repos/owner/repo/releases?per_page=30&page=2"); len(second) != 5 || second[0] != "v1.0.30" {
		t.Fatalf("page 2 = %v", second)
	}
	if beyond := tags("/repos/owner/repo/releases?page=9"); len(beyond) != 0 {
		t.Fatalf("a page beyond the end = %v", beyond)
	}
	g.RequireToken("other")
	if got := get(t, g, "/repos/owner/repo/releases"); got.status != http.StatusUnauthorized {
		t.Fatalf("the list under RequireToken: status %d", got.status)
	}
}
