package selfupdate

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// Tests for plans
// (docs/decisions/0038-MADR-pinned-plans-lifecycle-conformance-and-bubble-tests.md
// D1-A; 0038-PLAN Q14).

// tagSource is a scriptSource whose ByTag answers by tag, from tags, so a
// newer release can be published as the latest between Plan and Apply.
type tagSource struct {
	*scriptSource
	tags map[string]Release
}

func (s *tagSource) ByTag(_ context.Context, tag string) (Release, error) {
	s.note("ByTag")
	rel, ok := s.tags[tag]
	if !ok {
		return Release{}, fmt.Errorf("no release %s: %w", tag, ErrNoRelease)
	}
	return rel, nil
}

// planEnv is an Updater over a tagSource holding fixtureRelease's v1.1.0,
// with a standalone installer on a target holding "old-bytes".
type planEnv struct {
	u    *Updater
	src  *tagSource
	rep  *recReporter
	conf *recConfirmer
	exe  string
	rel  Release
}

func newPlanEnv(t *testing.T) *planEnv {
	t.Helper()
	_, exe := tempTarget(t)
	rel, bodies, plats := fixtureRelease(t, "demo")
	sel, err := NewExactAssetSelector(plats)
	if err != nil {
		t.Fatal(err)
	}
	inst, err := NewStandaloneInstaller(InstallOptions{TargetPolicy: targetPolicy(exe)})
	if err != nil {
		t.Fatal(err)
	}
	e := &planEnv{exe: exe, rel: rel, rep: &recReporter{}, conf: &recConfirmer{ok: true}}
	e.src = &tagSource{scriptSource: &scriptSource{rel: rel, bodies: bodies}, tags: map[string]Release{rel.Tag: rel}}
	e.u, err = New(Config{
		Source: e.src, Versions: NewStrictVersionPolicy(), Assets: sel,
		Installer: inst, Reporter: e.rep, Confirmer: e.conf, Limits: DefaultLimits(),
	})
	if err != nil {
		t.Fatal(err)
	}
	return e
}

// plan is Plan of applyReq, which must succeed; the source's calls start
// afresh after it.
func (e *planEnv) plan(t *testing.T) Plan {
	t.Helper()
	p, err := e.u.Plan(context.Background(), applyReq())
	if err != nil {
		t.Fatal(err)
	}
	e.src.calls = nil
	return p
}

// publish makes tag, with its own release and asset IDs and a binary
// holding bin, the latest release.
func (e *planEnv) publish(tag string, id int64, bin string) {
	binAsset, manAsset := e.rel.Assets[0], e.rel.Assets[1]
	sum := sha256.Sum256([]byte(bin))
	hexsum := hex.EncodeToString(sum[:])
	manifest := hexsum + "  " + binAsset.Name + "\n"
	binAsset.ID, binAsset.Size, binAsset.Digest = id+1, int64(len(bin)), "sha256:"+hexsum
	manAsset.ID, manAsset.Size = id+2, int64(len(manifest))
	rel := Release{ID: id, Tag: tag, URL: "https://example.invalid/" + tag, Immutable: true, Assets: []Asset{binAsset, manAsset}}
	e.src.bodies[binAsset.ID], e.src.bodies[manAsset.ID] = []byte(bin), []byte(manifest)
	e.src.tags[tag] = rel
	e.src.rel = rel
}

// apply is Apply with p's request and Yes.
func (e *planEnv) apply(p Plan) (Result, error) {
	req := p.Request()
	req.Yes = true
	return e.u.Apply(context.Background(), req, p)
}

func hexSHA256(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

// TestPlanThenApplyInstalls: Plan records the request, the release and its
// SHA256SUMS entry, and Apply installs that release.
func TestPlanThenApplyInstalls(t *testing.T) {
	t.Parallel()
	e := newPlanEnv(t)
	p := e.plan(t)
	sel := Selection{Binary: e.rel.Assets[0], Manifest: e.rel.Assets[1], ManifestName: e.rel.Assets[0].Name}
	want := Plan{
		Product: "demo", CurrentVersion: "v1.0.0", CurrentBuild: ReleaseBuild, Platform: e.u.runningPlatform(),
		TargetVersion: "v1.1.0", ReleaseID: 1, ReleaseURL: e.rel.URL, Latest: true, Operation: OperationUpgrade,
		Selection: sel, ManifestSHA256: hexSHA256("hello-bin"),
	}
	if p != want {
		t.Fatalf("Plan =\n%+v\nwant\n%+v", p, want)
	}
	res, err := e.apply(p)
	if err != nil || !res.Applied || res.TargetVersion != "v1.1.0" {
		t.Fatalf("Apply = %+v, %v", res, err)
	}
	if got := readString(t, e.exe); got != "hello-bin" {
		t.Fatalf("target holds %q", got)
	}
}

// TestApplyIgnoresNewerRelease: a release published after the plan
// changes nothing: Apply fetches the plan's tag and installs it, and asks
// the source for nothing else.
func TestApplyIgnoresNewerRelease(t *testing.T) {
	t.Parallel()
	e := newPlanEnv(t)
	p := e.plan(t)
	e.publish("v1.2.0", 10, "newer-bin")
	res, err := e.apply(p)
	if err != nil || !res.Applied || res.TargetVersion != "v1.1.0" {
		t.Fatalf("Apply = %+v, %v", res, err)
	}
	if got := readString(t, e.exe); got != "hello-bin" {
		t.Fatalf("target holds %q, want the planned v1.1.0", got)
	}
	if want := []string{"ByTag", "OpenAsset:3", "OpenAsset:2"}; !slices.Equal(e.src.calls, want) {
		t.Fatalf("source calls %v, want %v", e.src.calls, want)
	}
}

// TestApplyRefusesChangedRelease: each difference between the release and
// the plan is ErrPlanStale, before the binary is downloaded, and the target
// is untouched.
func TestApplyRefusesChangedRelease(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		// change changes the source, or the plan.
		change    func(e *planEnv, p *Plan)
		integrity bool
		// wantErr replaces ErrPlanStale.
		wantErr error
	}{
		{name: "release ID", change: func(e *planEnv, _ *Plan) {
			rel := e.src.tags["v1.1.0"]
			rel.ID = 9
			e.src.tags["v1.1.0"] = rel
		}},
		{name: "binary asset ID", change: func(e *planEnv, _ *Plan) {
			rel := e.src.tags["v1.1.0"]
			rel.Assets = slices.Clone(rel.Assets)
			rel.Assets[0].ID = 7
			e.src.bodies[7] = e.src.bodies[2]
			e.src.tags["v1.1.0"] = rel
		}},
		{name: "binary digest", change: func(e *planEnv, _ *Plan) {
			rel := e.src.tags["v1.1.0"]
			rel.Assets = slices.Clone(rel.Assets)
			rel.Assets[0].Digest = "sha256:" + hexSHA256("other-bin")
			e.src.tags["v1.1.0"] = rel
		}},
		{name: "SHA256SUMS entry", integrity: true, change: func(e *planEnv, _ *Plan) {
			e.src.bodies[3] = []byte(hexSHA256("other-bin") + "  " + e.rel.Assets[0].Name + "\n")
		}},
		{name: "operation", change: func(_ *planEnv, p *Plan) {
			p.Operation = OperationReinstall
		}},
		// A plan resolved as the latest never rolls back: an older latest
		// is ErrLatestOlder, as in Run, whatever operation it names.
		{name: "a latest plan's rollback", wantErr: ErrLatestOlder, change: func(_ *planEnv, p *Plan) {
			p.CurrentVersion, p.Operation = "v1.2.0", OperationRollback
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			e := newPlanEnv(t)
			p := e.plan(t)
			tc.change(e, &p)
			res, err := e.apply(p)
			want := tc.wantErr
			if want == nil {
				want = ErrPlanStale
			}
			if !errors.Is(err, want) || res.Applied {
				t.Fatalf("Apply = %+v, %v; want %v", res, err, want)
			}
			if tc.integrity != errors.Is(err, ErrIntegrity) {
				t.Fatalf("err = %v; ErrIntegrity %t, want %t", err, errors.Is(err, ErrIntegrity), tc.integrity)
			}
			if slices.Contains(e.src.calls, "OpenAsset:2") {
				t.Fatalf("the binary was downloaded: %v", e.src.calls)
			}
			if got := readString(t, e.exe); got != "old-bytes" {
				t.Fatalf("target holds %q", got)
			}
			if tc.wantErr == nil {
				last := e.rep.events[len(e.rep.events)-1]
				if last.Kind != EventFailed || last.Detail != "plan-stale" {
					t.Fatalf("last event %+v, want failed with plan-stale", last)
				}
			}
		})
	}
}

// TestApplyRefusesAnotherRequest: a request that is not the plan's is
// ErrPlanStale before any source call, naming what differs; so is an
// invalid plan.
func TestApplyRefusesAnotherRequest(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name   string
		change func(req *Request, p *Plan)
		want   string
	}{
		{"product", func(req *Request, _ *Plan) { req.Product = "other" }, "product"},
		{"current version", func(req *Request, _ *Plan) { req.CurrentVersion = "v1.0.1" }, "current version"},
		{"current build", func(req *Request, _ *Plan) { req.CurrentBuild = LocalBuild }, "current build"},
		{"platform", func(req *Request, _ *Plan) {
			req.Yes, req.DryRun, req.Platform = false, true, Platform{OS: "plan9", Arch: "amd64"}
		}, "platform"},
		{"channel", func(_ *Request, p *Plan) { p.Channel = "rc" }, "channel"},
		{"requested version", func(req *Request, _ *Plan) { req.TargetVersion = "v1.1.0" }, "requested version"},
		{"force", func(req *Request, _ *Plan) { req.Force = true }, "force"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			e := newPlanEnv(t)
			p := e.plan(t)
			req := p.Request()
			req.Yes = true
			tc.change(&req, &p)
			res, err := e.u.Apply(context.Background(), req, p)
			if !errors.Is(err, ErrPlanStale) || res.Applied || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("Apply = %+v, %v; want ErrPlanStale naming %s", res, err, tc.want)
			}
			if len(e.src.calls) != 0 {
				t.Fatalf("source calls %v, want none", e.src.calls)
			}
		})
	}
	t.Run("invalid plan", func(t *testing.T) {
		t.Parallel()
		e := newPlanEnv(t)
		res, err := e.u.Apply(context.Background(), applyReq(), Plan{})
		if err == nil || errors.Is(err, ErrPlanStale) || res != (Result{}) || len(e.src.calls) != 0 || len(e.rep.events) != 0 {
			t.Fatalf("Apply(Plan{}) = %+v, %v; calls %v, events %v", res, err, e.src.calls, e.rep.kinds)
		}
	})
}

// TestApplyConfirms: Apply asks the Confirmer as Run does, unless the
// request says Yes or DryRun, and a decline installs nothing.
func TestApplyConfirms(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name         string
		yes, dryRun  bool
		answer       bool
		wantAsked    bool
		wantApplied  bool
		wantDeclined bool
	}{
		{name: "yes", yes: true, wantApplied: true},
		{name: "dry run", dryRun: true},
		{name: "approved", answer: true, wantAsked: true, wantApplied: true},
		{name: "declined", wantAsked: true, wantDeclined: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			e := newPlanEnv(t)
			e.conf.ok = tc.answer
			p := e.plan(t)
			req := p.Request()
			req.Yes, req.DryRun = tc.yes, tc.dryRun
			res, err := e.u.Apply(context.Background(), req, p)
			if err != nil || res.Applied != tc.wantApplied || res.Declined != tc.wantDeclined {
				t.Fatalf("Apply = %+v, %v", res, err)
			}
			if asked := e.conf.calls == 1; asked != tc.wantAsked {
				t.Fatalf("Confirmer asked %d times", e.conf.calls)
			}
			want := map[bool]string{true: "hello-bin", false: "old-bytes"}[tc.wantApplied]
			if got := readString(t, e.exe); got != want {
				t.Fatalf("target holds %q, want %q", got, want)
			}
		})
	}
}

// TestApplyCheckOnly: Apply with CheckOnly checks the planned release as
// Run does, and installs nothing.
func TestApplyCheckOnly(t *testing.T) {
	t.Parallel()
	e := newPlanEnv(t)
	p := e.plan(t)
	req := p.Request()
	req.CheckOnly = true
	res, err := e.u.Apply(context.Background(), req, p)
	if !errors.Is(err, ErrUpdateAvailable) || !res.Checked || res.Applied || res.TargetVersion != "v1.1.0" {
		t.Fatalf("Apply with CheckOnly = %+v, %v; want ErrUpdateAvailable for v1.1.0", res, err)
	}
	if got := readString(t, e.exe); got != "old-bytes" {
		t.Fatalf("target holds %q", got)
	}
}

// TestApplyEventsMatchRun: Apply reports the events Run reports for the
// same release.
func TestApplyEventsMatchRun(t *testing.T) {
	t.Parallel()
	ran := newPlanEnv(t)
	req := applyReq()
	req.Yes = true
	if _, err := ran.u.Run(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	applied := newPlanEnv(t)
	if _, err := applied.apply(applied.plan(t)); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(applied.rep.events, ran.rep.events) {
		t.Fatalf("Apply's events\n%v\nRun's\n%v", applied.rep.events, ran.rep.events)
	}
}

// TestPlanReportsNothing: Plan reports no event, asks nothing, takes no
// lock, and downloads SHA256SUMS but no binary.
func TestPlanReportsNothing(t *testing.T) {
	t.Parallel()
	e := newPlanEnv(t)
	if _, err := e.u.Plan(context.Background(), applyReq()); err != nil {
		t.Fatal(err)
	}
	if len(e.rep.events) != 0 || e.conf.calls != 0 {
		t.Fatalf("events %v, Confirmer asked %d times", e.rep.kinds, e.conf.calls)
	}
	if want := []string{"Latest", "OpenAsset:3"}; !slices.Equal(e.src.calls, want) {
		t.Fatalf("source calls %v, want %v", e.src.calls, want)
	}
	entries, err := os.ReadDir(filepath.Dir(e.exe))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("the target's directory holds %v, want the target alone", entries)
	}
}

// TestPlanRefusesCheckOnly: a check is Checker.Check's.
func TestPlanRefusesCheckOnly(t *testing.T) {
	t.Parallel()
	e := newPlanEnv(t)
	req := applyReq()
	req.CheckOnly = true
	if _, err := e.u.Plan(context.Background(), req); err == nil || len(e.src.calls) != 0 {
		t.Fatalf("Plan with CheckOnly = %v; source calls %v", err, e.src.calls)
	}
}

// TestPlanUpToDate: a plan whose operation is OperationNone downloads
// nothing and carries no SHA256SUMS entry, and Apply reports the up-to-date
// result without beginning a session.
func TestPlanUpToDate(t *testing.T) {
	t.Parallel()
	e := newPlanEnv(t)
	req := applyReq()
	req.CurrentVersion = "v1.1.0"
	p, err := e.u.Plan(context.Background(), req)
	if err != nil || p.Operation != OperationNone || p.ManifestSHA256 != "" {
		t.Fatalf("Plan = %+v, %v", p, err)
	}
	if want := []string{"Latest"}; !slices.Equal(e.src.calls, want) {
		t.Fatalf("source calls %v, want %v", e.src.calls, want)
	}
	res, err := e.apply(p)
	if err != nil || res.Applied || res.Operation != OperationNone || res.TargetVersion != "v1.1.0" {
		t.Fatalf("Apply = %+v, %v", res, err)
	}
	if got := readString(t, e.exe); got != "old-bytes" {
		t.Fatalf("target holds %q", got)
	}
}

// TestApplySharesRunsGuard: Apply during a Run on the same Updater is
// ErrConcurrentUpdate.
func TestApplySharesRunsGuard(t *testing.T) {
	t.Parallel()
	e := newPlanEnv(t)
	p := e.plan(t)
	e.src.block, e.src.entered = make(chan struct{}), make(chan struct{})
	entered := e.src.entered
	done := make(chan error, 1)
	go func() {
		req := applyReq()
		req.Yes = true
		_, err := e.u.Run(context.Background(), req)
		done <- err
	}()
	<-entered
	_, err := e.apply(p)
	close(e.src.block)
	if runErr := <-done; runErr != nil {
		t.Fatalf("Run: %v", runErr)
	}
	if !errors.Is(err, ErrConcurrentUpdate) {
		t.Fatalf("Apply during Run = %v, want ErrConcurrentUpdate", err)
	}
}

// TestPlanDocumentRoundTrip: ParsePlan reads back what Document writes.
func TestPlanDocumentRoundTrip(t *testing.T) {
	t.Parallel()
	e := newPlanEnv(t)
	p := e.plan(t)
	data, err := json.Marshal(p.Document())
	if err != nil {
		t.Fatal(err)
	}
	got, err := ParsePlan(data)
	if err != nil || got != p {
		t.Fatalf("ParsePlan = %+v, %v; want %+v", got, err, p)
	}
	if req := p.Request(); req.Yes || req.DryRun || req.CheckOnly || req.TargetVersion != "" || req.Product != "demo" {
		t.Fatalf("Request() = %+v", req)
	}
}

// validPlanJSON is a valid plan document, as a map a case changes.
func validPlanJSON() map[string]any {
	asset := func(id int64, name string, size int64, digest string) map[string]any {
		m := map[string]any{"id": id, "name": name, "state": "uploaded", "size": size}
		if digest != "" {
			m["digest"] = digest
		}
		return m
	}
	return map[string]any{
		"schema_version": PlanSchema, "product": "demo", "current_version": "v1.0.0", "current_build": "release",
		"os": "linux", "arch": "amd64", "force": false, "target_version": "v1.1.0", "release_id": 1,
		"latest": true, "operation": "upgrade",
		"binary":   asset(2, "demo-linux-amd64", 9, "sha256:"+hexSHA256("hello-bin")),
		"manifest": asset(3, "SHA256SUMS", 82, ""), "manifest_name": "demo-linux-amd64", "packed": false,
		"manifest_sha256": hexSHA256("hello-bin"),
	}
}

// TestParsePlanRefuses: each rule of ParsePlan, on a document that is
// otherwise valid.
func TestParsePlanRefuses(t *testing.T) {
	t.Parallel()
	encode := func(t *testing.T, m map[string]any) []byte {
		t.Helper()
		b, err := json.Marshal(m)
		if err != nil {
			t.Fatal(err)
		}
		return b
	}
	if _, err := ParsePlan(encode(t, validPlanJSON())); err != nil {
		t.Fatalf("the valid document: %v", err)
	}
	set := func(key string, v any) func(map[string]any) {
		return func(m map[string]any) { m[key] = v }
	}
	setAsset := func(asset, key string, v any) func(map[string]any) {
		return func(m map[string]any) { m[asset].(map[string]any)[key] = v }
	}
	for _, tc := range []struct {
		name   string
		change func(map[string]any)
		raw    func([]byte) []byte
		want   string
	}{
		{name: "too large", raw: func(b []byte) []byte { return append(b, make([]byte, maxPlanSize)...) }, want: "more than 65536"},
		{name: "unknown field", change: set("yes", true), want: `unknown field "yes"`},
		{name: "trailing data", raw: func(b []byte) []byte { return append(b, []byte(" {}")...) }, want: "data after the plan"},
		{name: "schema 2", change: set("schema_version", 2), want: "schema_version: 2 is not supported; this library reads schema 1"},
		{name: "no schema", change: func(m map[string]any) { delete(m, "schema_version") }, want: "schema_version: 0 is not supported"},
		{name: "not an object", raw: func([]byte) []byte { return []byte("[]") }, want: "selfupdate: plan:"},
		{name: "product", change: set("product", "../demo"), want: "product"},
		{name: "build kind", change: set("current_build", "dirty"), want: "current_build"},
		{name: "unknown build kind", change: set("current_build", "unknown"), want: "current_build"},
		{name: "operation", change: set("operation", "downgrade"), want: "operation"},
		{name: "no current version", change: set("current_version", ""), want: "current_version"},
		{name: "no os", change: set("os", ""), want: "os, arch"},
		{name: "no target", change: set("target_version", ""), want: "target_version"},
		{name: "requested another tag", change: func(m map[string]any) { m["requested_version"], m["latest"] = "v1.0.5", false }, want: "requested_version"},
		{name: "latest with a requested tag", change: set("requested_version", "v1.1.0"), want: "latest"},
		{name: "release ID", change: set("release_id", 0), want: "release_id"},
		{name: "binary ID", change: setAsset("binary", "id", -1), want: "binary"},
		{name: "binary size", change: setAsset("binary", "size", 0), want: "binary"},
		{name: "binary name", change: setAsset("binary", "name", "bin/demo"), want: "binary"},
		{name: "binary state", change: setAsset("binary", "state", "open"), want: "binary"},
		{name: "binary digest", change: setAsset("binary", "digest", "md5:00"), want: "binary"},
		{name: "manifest name", change: set("manifest_name", ""), want: "manifest_name"},
		{name: "manifest digest", change: set("manifest_sha256", strings.ToUpper(hexSHA256("hello-bin"))), want: "manifest_sha256"},
		{name: "no manifest digest", change: func(m map[string]any) { delete(m, "manifest_sha256") }, want: "manifest_sha256"},
		{name: "a digest for none", change: set("operation", "none"), want: "manifest_sha256"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			m := validPlanJSON()
			if tc.change != nil {
				tc.change(m)
			}
			data := encode(t, m)
			if tc.raw != nil {
				data = tc.raw(data)
			}
			p, err := ParsePlan(data)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("ParsePlan = %+v, %v; want an error naming %q", p, err, tc.want)
			}
		})
	}
}
