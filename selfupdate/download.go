package selfupdate

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/maccavelli/go-selfupdate-lib/internal/errprefix"
)

func readBounded(r io.Reader, limit int64) ([]byte, error) {
	if limit <= 0 {
		return nil, fmt.Errorf("selfupdate: body limit must be positive")
	}
	data, err := io.ReadAll(&io.LimitedReader{R: r, N: limit + 1})
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("selfupdate: response exceeds %d-byte limit", limit)
	}
	return data, nil
}

func decodeJSON(data []byte, dest any) error {
	dec := json.NewDecoder(bytes.NewReader(data))
	if err := dec.Decode(dest); err != nil {
		return errprefix.Wrap("selfupdate: malformed github json", err)
	}
	// Anything after the document, a lone closing delimiter included, is
	// refused: dec.More() alone misses a trailing '}' or ']' (0003-MADR A6).
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return fmt.Errorf("selfupdate: trailing github json")
	}
	return nil
}

// readTruncated reads at most limit bytes and discards the rest, for error
// bodies whose content is only diagnostic (0003-MADR A8).
func readTruncated(r io.Reader, limit int64) ([]byte, error) {
	if limit <= 0 {
		return nil, fmt.Errorf("selfupdate: body limit must be positive")
	}
	return io.ReadAll(&io.LimitedReader{R: r, N: limit})
}

func sanitizeDiagnostic(s string, limit int64) string {
	if int64(len(s)) > limit {
		s = s[:limit]
	}
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if r < 32 || r == 127 || (r >= 0x80 && r <= 0x9f) || isInvisibleControl(r) {
			b.WriteByte('?')
			continue
		}
		if !utf8.ValidRune(r) {
			b.WriteByte('?')
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

// isInvisibleControl reports Unicode format controls (category Cf, such as
// bidi overrides U+202E and isolates U+2066) and the line and paragraph
// separators, which reorder or break terminal text (0003-MADR A11).
func isInvisibleControl(r rune) bool {
	return unicode.Is(unicode.Cf, r) || r == ' ' || r == ' '
}

func copyLimited(r io.Reader, w io.Writer, advertised, limit int64) (written int64, digest string, err error) {
	if advertised <= 0 {
		return 0, "", errprefix.Wrap("selfupdate: advertised size must be positive", ErrIntegrity)
	}
	if advertised > limit {
		return 0, "", fmt.Errorf("selfupdate: advertised size %d exceeds limit %d: %w", advertised, limit, ErrIntegrity)
	}
	h := sha256.New()
	n, err := io.Copy(io.MultiWriter(w, h), &io.LimitedReader{R: r, N: advertised + 1})
	if err != nil {
		return n, "", err
	}
	if n != advertised {
		if n > advertised {
			return n, "", fmt.Errorf("selfupdate: body longer than advertised size %d: %w", advertised, ErrIntegrity)
		}
		return n, "", fmt.Errorf("selfupdate: body shorter than advertised size %d: %w", advertised, ErrIntegrity)
	}
	return n, hex.EncodeToString(h.Sum(nil)), nil
}

func downloadAsset(ctx context.Context, src ReleaseSource, rel Release, asset Asset, w io.Writer, limit int64) (digest string, err error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	rc, err := src.OpenAsset(ctx, rel, asset)
	if err != nil {
		return "", err
	}
	defer func() {
		if cerr := rc.Close(); cerr != nil && err == nil {
			err = cerr
		}
	}()
	_, digest, err = copyLimited(rc, w, asset.Size, limit)
	if err != nil {
		return "", err
	}
	if asset.Digest != "" {
		want, derr := parseGitHubDigest(asset.Digest)
		if derr != nil {
			return "", derr
		}
		if digest != want {
			return "", errprefix.Wrap("selfupdate: downloaded digest does not match github asset digest", ErrIntegrity)
		}
	}
	return digest, nil
}
