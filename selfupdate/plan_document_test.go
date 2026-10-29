package selfupdate_test

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"testing"

	"github.com/maccavelli/go-selfupdate-lib/selfupdate"
)

// TestPlanDocumentGolden pins PlanDocument's JSON, byte for byte: its keys,
// their order, and the build kind and operation as strings
// (docs/decisions/0038-MADR-pinned-plans-lifecycle-conformance-and-bubble-tests.md
// D1-A). go test -run TestPlanDocumentGolden -update rewrites it.
func TestPlanDocumentGolden(t *testing.T) {
	t.Parallel()
	sum := sha256.Sum256([]byte("demo v1.1.0\n"))
	digest := hex.EncodeToString(sum[:])
	p := selfupdate.Plan{
		Product: "demo", CurrentVersion: "v1.0.0", CurrentBuild: selfupdate.ReleaseBuild,
		Platform:      selfupdate.Platform{OS: "linux", Arch: "amd64"},
		TargetVersion: "v1.1.0", ReleaseID: 101, ReleaseURL: "https://example.invalid/releases/v1.1.0",
		Latest: true, Operation: selfupdate.OperationUpgrade,
		Selection: selfupdate.Selection{
			Binary: selfupdate.Asset{
				ID: 102, Name: "demo-linux-amd64", State: selfupdate.AssetStateUploaded, Size: 12, Digest: "sha256:" + digest,
			},
			Manifest:     selfupdate.Asset{ID: 103, Name: "SHA256SUMS", State: selfupdate.AssetStateUploaded, Size: 83},
			ManifestName: "demo-linux-amd64",
		},
		ManifestSHA256: digest,
	}
	got, err := json.MarshalIndent(p.Document(), "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	compareGolden(t, "plan.golden", append(got, '\n'))
	back, err := selfupdate.ParsePlan(got)
	if err != nil || back != p {
		t.Fatalf("ParsePlan of the golden document = %+v, %v; want %+v", back, err, p)
	}
}
