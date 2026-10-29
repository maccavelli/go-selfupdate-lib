package selfupdatetest

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"
	"unicode"

	"github.com/maccavelli/go-selfupdate-lib/selfupdate"
)

// The source conformance check
// (docs/decisions/0025-MADR-align-with-go-sdk-design-practices.md D11,
// Amendment A1).

// SourceCase names what VerifyReleaseSource expects of a source.
type SourceCase struct {
	// LatestTag is the tag Latest must return.
	LatestTag string
	// MissingTag is a tag ByTag must report as missing.
	MissingTag string
}

// VerifyReleaseSource checks src against the contract the Updater relies
// on, and returns every violation joined, or nil. A source for a host other
// than GitHub runs it in its tests:
//
//   - Latest returns LatestTag, Immutable, and not a draft;
//   - ByTag(LatestTag) returns the same release, with the release ID and
//     asset IDs Latest returned, which selfupdate.Updater.Apply compares
//     with its plan's
//     (docs/decisions/0038-MADR-pinned-plans-lifecycle-conformance-and-bubble-tests.md
//     D1-A);
//   - ByTag(MissingTag) returns an error that wraps selfupdate.ErrNoRelease;
//   - every asset of that release has a positive ID, a basename for Name,
//     State selfupdate.AssetStateUploaded, a positive Size, and a Digest that
//     is empty or sha256:<64 hex>;
//   - OpenAsset returns exactly Size bytes for each asset, whose SHA-256 is
//     Digest when it is set.
func VerifyReleaseSource(ctx context.Context, src selfupdate.ReleaseSource, c SourceCase) error {
	if src == nil {
		return errors.New("selfupdatetest: source is nil")
	}
	if c.LatestTag == "" || c.MissingTag == "" {
		return errors.New("selfupdatetest: SourceCase needs LatestTag and MissingTag")
	}
	var errs []error
	fail := func(format string, args ...any) {
		errs = append(errs, fmt.Errorf("selfupdatetest: "+format, args...))
	}
	if _, err := src.ByTag(ctx, c.MissingTag); !errors.Is(err, selfupdate.ErrNoRelease) {
		fail("ByTag(%q) = %v; want an error that wraps selfupdate.ErrNoRelease", c.MissingTag, err)
	}
	latest, err := src.Latest(ctx)
	if err != nil {
		fail("Latest: %v", err)
		return errors.Join(errs...)
	}
	if latest.Tag != c.LatestTag {
		fail("Latest returned %q, want %q", latest.Tag, c.LatestTag)
	}
	if !latest.Immutable {
		fail("release %q is not Immutable; the Updater refuses it with ErrMutableRelease", latest.Tag)
	}
	if latest.Draft {
		fail("Latest returned the draft %q", latest.Tag)
	}
	switch byTag, err := src.ByTag(ctx, c.LatestTag); {
	case err != nil:
		fail("ByTag(%q): %v", c.LatestTag, err)
	case byTag.Tag != latest.Tag || len(byTag.Assets) != len(latest.Assets):
		fail("ByTag(%q) is not the release Latest returned", c.LatestTag)
	case byTag.ID != latest.ID || !slices.EqualFunc(byTag.Assets, latest.Assets, sameAssetID):
		fail("ByTag(%q) returned release ID %d and asset IDs %v, Latest %d and %v; Updater.Apply needs the same IDs each time",
			c.LatestTag, byTag.ID, assetIDs(byTag.Assets), latest.ID, assetIDs(latest.Assets))
	}
	for _, a := range latest.Assets {
		verifyAsset(ctx, src, latest, a, fail)
	}
	return errors.Join(errs...)
}

func sameAssetID(a, b selfupdate.Asset) bool {
	return a.ID == b.ID
}

func assetIDs(assets []selfupdate.Asset) []int64 {
	ids := make([]int64, len(assets))
	for i, a := range assets {
		ids[i] = a.ID
	}
	return ids
}

func verifyAsset(ctx context.Context, src selfupdate.ReleaseSource, rel selfupdate.Release, a selfupdate.Asset, fail func(string, ...any)) {
	if a.ID <= 0 {
		fail("asset %q has ID %d; the Updater requires a positive ID", a.Name, a.ID)
	}
	if a.Name == "" || a.Name == "." || a.Name == ".." || strings.ContainsAny(a.Name, `/\`) || strings.IndexFunc(a.Name, unicode.IsControl) >= 0 {
		fail("asset name %q is not a basename", a.Name)
	}
	if a.State != selfupdate.AssetStateUploaded {
		fail("asset %q has State %q, want %q", a.Name, a.State, selfupdate.AssetStateUploaded)
	}
	if a.Size <= 0 {
		fail("asset %q has Size %d, want a positive size", a.Name, a.Size)
		return
	}
	want, ok := digestHex(a.Digest)
	if !ok {
		fail("asset %q has Digest %q, want empty or sha256:<64 hex>", a.Name, a.Digest)
	}
	rc, err := src.OpenAsset(ctx, rel, a)
	if err != nil {
		fail("OpenAsset(%q): %v", a.Name, err)
		return
	}
	h := sha256.New()
	n, err := io.Copy(h, io.LimitReader(rc, a.Size+1))
	if cerr := rc.Close(); err == nil {
		err = cerr
	}
	switch {
	case err != nil:
		fail("reading %q: %v", a.Name, err)
	case n != a.Size:
		fail("OpenAsset(%q) returned %d bytes, want Size %d", a.Name, n, a.Size)
	case want != "" && hex.EncodeToString(h.Sum(nil)) != want:
		fail("asset %q does not match its Digest", a.Name)
	}
}

// digestHex is the lowercase hex of a sha256:<hex> digest, "" for an
// empty one, and ok false for any other form.
func digestHex(d string) (string, bool) {
	if d == "" {
		return "", true
	}
	h, found := strings.CutPrefix(d, "sha256:")
	if !found || len(h) != sha256.Size*2 {
		return "", false
	}
	if _, err := hex.DecodeString(h); err != nil {
		return "", false
	}
	return strings.ToLower(h), true
}
