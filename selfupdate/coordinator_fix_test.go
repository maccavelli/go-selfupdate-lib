package selfupdate

import (
	"context"
	"os"
	"testing"
)

// captureInstaller records the InstallRequest its session receives.
type captureInstaller struct {
	*logInstaller
	got *InstallRequest
}

func (c captureInstaller) Begin(ctx context.Context, t Target) (InstallSession, error) {
	s, err := c.logInstaller.Begin(ctx, t)
	if err != nil {
		return nil, err
	}
	return &captureSession{logSession: s.(*logSession), got: c.got}, nil
}

type captureSession struct {
	*logSession
	got *InstallRequest
}

func (s *captureSession) Install(ctx context.Context, req InstallRequest) (InstallResult, error) {
	*s.got = req
	return s.logSession.Install(ctx, req)
}

// growTransformer appends bytes to the staged file, as a re-signing
// transform does.
type growTransformer struct{ extra []byte }

func (g growTransformer) Transform(_ context.Context, r TransformRequest) error {
	f, err := os.OpenFile(r.Path, os.O_WRONLY|os.O_APPEND, 0)
	if err != nil {
		return err
	}
	if _, err := f.Write(g.extra); err != nil {
		return joinClose(err, f)
	}
	return f.Close()
}

// TestInstallRequestCarriesTransformedSize: StagedArtifact.Size is the
// staged length after the transform, as its doc says (0004-MADR G8).
func TestInstallRequestCarriesTransformedSize(t *testing.T) {
	t.Parallel()
	env := newContractEnv(t)
	env.xf = growTransformer{extra: []byte("-signed")}
	var got InstallRequest
	rel, _, plats := fixtureRelease(t, "demo")
	sel, err := NewExactAssetSelector(plats)
	if err != nil {
		t.Fatal(err)
	}
	u, err := New(Config{
		Source: env.src, Versions: NewStrictVersionPolicy(), Assets: sel, Transformer: env.xf,
		Installer: captureInstaller{logInstaller: env.inst, got: &got},
		Reporter:  env.rep, Confirmer: env.conf, Limits: env.lim,
	})
	if err != nil {
		t.Fatal(err)
	}
	req := applyReq()
	req.Yes = true
	if _, err := u.Run(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	selected, err := sel.Select(rel, "demo", normalizePlatform(Platform{}, runtimePlatform()))
	if err != nil {
		t.Fatal(err)
	}
	want := selected.Binary.Size + int64(len("-signed"))
	if got.Artifact.Size != want {
		t.Fatalf("StagedArtifact.Size = %d, want the transformed length %d (advertised %d)",
			got.Artifact.Size, want, selected.Binary.Size)
	}
}

// TestAssetStructureRefusesDotNames: "." and ".." are not file names
// (0004-MADR R9).
func TestAssetStructureRefusesDotNames(t *testing.T) {
	t.Parallel()
	for _, name := range []string{".", ".."} {
		if err := validateAssetStructure(Asset{ID: 1, Name: name}); err == nil {
			t.Errorf("asset name %q accepted", name)
		}
	}
	if err := validateAssetStructure(Asset{ID: 1, Name: ".hidden"}); err != nil {
		t.Errorf("a dot-prefixed name is a file name: %v", err)
	}
}
