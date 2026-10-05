package archive

import (
	"archive/tar"
	"archive/zip"
	"bufio"
	"bytes"
	"cmp"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"math"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/maccavelli/go-selfupdate-lib/selfupdate"
)

// defaultMaxEntries is UnpackOptions.MaxEntries when it is zero.
const defaultMaxEntries = 4096

// headerAllowance is what a tar.gz stream may hold beyond its entries'
// contents: headers, PAX records and padding (0012-PLAN U3 step 3).
const headerAllowance = 8 << 20

// UnpackOptions configure NewUnpacker.
type UnpackOptions struct {
	// Member is the program's path inside the archive, such as
	// "relay_1.2.3/bin/relay". Empty means the one regular file whose base
	// name is the product (plus ".exe" on windows), at the top level or
	// one directory down. A .gz asset has no members, and ignores it.
	Member string
	// MaxEntries bounds the entries an archive may hold. Zero means 4096.
	MaxEntries int
}

type unpacker struct {
	member     string
	maxEntries int
}

// NewUnpacker returns the selfupdate.Unpacker for the assets NewSelector
// selects. It writes the program to the session's staging, refuses any
// archive that two tools could read differently or that exceeds the
// limits, and checks the program's executable image for the platform. Its
// refusals wrap selfupdate.ErrIntegrity (0012-MADR §4).
func NewUnpacker(o UnpackOptions) (selfupdate.Unpacker, error) {
	if o.Member != "" && !fs.ValidPath(o.Member) {
		return nil, fmt.Errorf("selfupdate: archive: invalid member path %q", o.Member)
	}
	if o.MaxEntries < 0 {
		return nil, fmt.Errorf("selfupdate: archive: MaxEntries must not be negative")
	}
	u := &unpacker{member: o.Member, maxEntries: o.MaxEntries}
	if u.maxEntries == 0 {
		u.maxEntries = defaultMaxEntries
	}
	return u, nil
}

// refuse is an archive refusal: an integrity failure.
func refuse(format string, args ...any) error {
	return fmt.Errorf("selfupdate: archive: "+format+": %w", append(args, selfupdate.ErrIntegrity)...)
}

// formatOf is the format an asset name's suffix names.
func formatOf(name string) (Format, error) {
	switch {
	case strings.HasSuffix(name, ".tar.gz"), strings.HasSuffix(name, ".tgz"):
		return TarGz, nil
	case strings.HasSuffix(name, ".zip"):
		return Zip, nil
	case strings.HasSuffix(name, ".gz"):
		return Gz, nil
	}
	return "", refuse("%q is not a .tar.gz, .tgz, .zip or .gz asset", name)
}

var (
	gzipMagic     = []byte{0x1f, 0x8b}
	zipMagic      = []byte("PK\x03\x04")
	zipEmptyMagic = []byte("PK\x05\x06")
)

// Unpack implements selfupdate.Unpacker.
func (u *unpacker) Unpack(ctx context.Context, req selfupdate.UnpackRequest) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if req.Limit <= 0 {
		return fmt.Errorf("selfupdate: archive: the limit must be positive")
	}
	f, err := formatOf(req.AssetName)
	if err != nil {
		return err
	}
	if err := u.extractFile(ctx, f, req); err != nil {
		return err
	}
	prog, err := os.Open(req.Program)
	if err != nil {
		return err
	}
	err = selfupdate.CheckImage(prog, req.Platform)
	if cerr := prog.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return fmt.Errorf("selfupdate: archive: %w", err)
	}
	return nil
}

// extractFile opens the archive and the program file, checks the magic,
// and extracts.
func (u *unpacker) extractFile(ctx context.Context, f Format, req selfupdate.UnpackRequest) (err error) {
	in, err := os.Open(req.Archive)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, in.Close()) }()
	info, err := in.Stat()
	if err != nil {
		return err
	}
	out, err := os.OpenFile(req.Program, os.O_WRONLY|os.O_TRUNC, 0)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, out.Close()) }()
	return u.extract(ctx, f, in, info.Size(), out, programMatcher(req.Product, req.Platform, u.member), req.Limit)
}

// extract writes the program from the archive in ra, of size bytes, in
// format f, to w.
func (u *unpacker) extract(ctx context.Context, f Format, ra io.ReaderAt, size int64, w io.Writer, isProgram func(string) bool, limit int64) error {
	head := make([]byte, 4)
	n, err := ra.ReadAt(head, 0)
	if err != nil && !errors.Is(err, io.EOF) {
		return err
	}
	head = head[:n]
	switch f {
	case TarGz, Gz:
		if !bytes.HasPrefix(head, gzipMagic) {
			return refuse("the asset is named as gzip but is not gzip")
		}
	case Zip:
		if !bytes.HasPrefix(head, zipMagic) && !bytes.HasPrefix(head, zipEmptyMagic) {
			return refuse("the asset is named as zip but is not zip")
		}
	}
	r := io.NewSectionReader(ra, 0, size)
	switch f {
	case TarGz:
		return u.extractTarGz(ctx, r, w, isProgram, limit)
	case Zip:
		return u.extractZip(ctx, r, size, w, isProgram, limit)
	default:
		return extractGz(r, w, limit)
	}
}

// programMatcher reports whether an entry's cleaned name is the program.
func programMatcher(product string, p selfupdate.Platform, member string) func(string) bool {
	if member != "" {
		want := path.Clean(member)
		return func(name string) bool { return name == want }
	}
	want := product
	if p.OS == "windows" {
		want += ".exe"
	}
	return func(name string) bool {
		return path.Base(name) == want && strings.Count(name, "/") <= 1
	}
}

// entries checks every entry of one archive: its name, its uniqueness, the
// count, and the total of the declared sizes.
type entries struct {
	max   int
	limit int64
	count int
	total int64
	seen  map[string]bool
}

func newEntries(maxEntries int, limit int64) *entries {
	return &entries{max: maxEntries, limit: limit, seen: map[string]bool{}}
}

// add checks one entry and returns its cleaned name.
func (e *entries) add(name string, size int64) (string, error) {
	e.count++
	if e.count > e.max {
		return "", refuse("more than %d entries", e.max)
	}
	clean, err := checkName(name)
	if err != nil {
		return "", err
	}
	key := strings.ToLower(clean)
	if e.seen[key] {
		return "", refuse("entry %q repeats a name, or differs from another only in case", name)
	}
	e.seen[key] = true
	if size < 0 || size > e.limit || e.total > e.limit-size {
		return "", refuse("the entries declare more than %d bytes", e.limit)
	}
	e.total += size
	return clean, nil
}

// checkName refuses a name that is empty, absolute, holds "..", a
// backslash or a NUL, or is not local on this OS, whatever GODEBUG says
// (0012-MADR §4). It returns the name cleaned.
func checkName(name string) (string, error) {
	if name == "" || strings.ContainsAny(name, "\\\x00") || strings.HasPrefix(name, "/") {
		return "", refuse("entry name %q is not safe", name)
	}
	// path.Clean folds every ".." it can, so one that is left climbs out,
	// and IsLocal refuses it.
	clean := path.Clean(name)
	if clean != "." && !filepath.IsLocal(filepath.FromSlash(clean)) {
		return "", refuse("entry name %q is not safe", name)
	}
	return clean, nil
}

// capReader fails once more than n bytes have been read.
type capReader struct {
	r io.Reader
	n int64
}

var errOverCap = errors.New("the archive expands past the limit")

func (c *capReader) Read(p []byte) (int, error) {
	if c.n <= 0 {
		return 0, errOverCap
	}
	if int64(len(p)) > c.n {
		p = p[:c.n]
	}
	n, err := c.r.Read(p)
	c.n -= int64(n)
	return n, err
}

// copyProgram copies the program to w, refusing more than limit bytes, and
// returns how many it copied.
func copyProgram(w io.Writer, r io.Reader, limit int64) (int64, error) {
	n, err := io.Copy(w, io.LimitReader(r, limit+1))
	if err != nil {
		return n, refuse("reading the program: %v", err)
	}
	if n > limit {
		return n, refuse("the program is larger than %d bytes", limit)
	}
	return n, nil
}

func (u *unpacker) extractTarGz(ctx context.Context, r io.Reader, w io.Writer, isProgram func(string) bool, limit int64) error {
	gz, err := gzip.NewReader(r)
	if err != nil {
		return refuse("gzip: %v", err)
	}
	tr := tar.NewReader(&capReader{r: gz, n: limit + headerAllowance})
	seen := newEntries(u.maxEntries, limit)
	found := false
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return refuse("tar: %v", err)
		}
		clean, err := seen.add(hdr.Name, hdr.Size)
		if err != nil {
			return err
		}
		switch hdr.Typeflag {
		case tar.TypeDir:
			continue
		case tar.TypeReg, '\x00':
		default:
			return refuse("entry %q is not a regular file or a directory (type %q)", hdr.Name, hdr.Typeflag)
		}
		for k := range hdr.PAXRecords {
			if strings.HasPrefix(k, "GNU.sparse.") {
				return refuse("entry %q is a sparse file", hdr.Name)
			}
		}
		if !isProgram(clean) {
			continue
		}
		if found {
			return refuse("the archive holds more than one program, the second at %q", hdr.Name)
		}
		found = true
		n, err := copyProgram(w, tr, limit)
		if err != nil {
			return err
		}
		if n != hdr.Size {
			return refuse("entry %q holds %d bytes, not the %d it declares", hdr.Name, n, hdr.Size)
		}
	}
	if !found {
		return refuse("the archive holds no program")
	}
	return nil
}

type span struct{ start, end int64 }

func (u *unpacker) extractZip(ctx context.Context, ra io.ReaderAt, size int64, w io.Writer, isProgram func(string) bool, limit int64) error {
	zr, err := zip.NewReader(ra, size)
	if err != nil {
		return refuse("zip: %v", err)
	}
	if len(zr.File) > u.maxEntries {
		return refuse("more than %d entries", u.maxEntries)
	}
	seen := newEntries(u.maxEntries, limit)
	var spans []span
	var program *zip.File
	for _, zf := range zr.File {
		usize, ok := toInt64(zf.UncompressedSize64)
		if !ok || usize > limit {
			return refuse("entry %q declares more than %d bytes", zf.Name, limit)
		}
		clean, err := seen.add(zf.Name, usize)
		if err != nil {
			return err
		}
		mode := zf.Mode()
		if !mode.IsDir() && !mode.IsRegular() {
			return refuse("entry %q is not a regular file or a directory (mode %v)", zf.Name, mode)
		}
		if zf.Method != zip.Store && zf.Method != zip.Deflate {
			return refuse("entry %q uses compression method %d", zf.Name, zf.Method)
		}
		off, err := zf.DataOffset()
		if err != nil {
			return refuse("entry %q: %v", zf.Name, err)
		}
		csize, ok := toInt64(zf.CompressedSize64)
		if !ok || off < 0 || csize > size || off > size-csize {
			return refuse("entry %q's data lies outside the archive", zf.Name)
		}
		if csize > 0 {
			spans = append(spans, span{off, off + csize})
		}
		if mode.IsRegular() && isProgram(clean) {
			if program != nil {
				return refuse("the archive holds more than one program, the second at %q", zf.Name)
			}
			program = zf
		}
	}
	slices.SortFunc(spans, func(a, b span) int { return cmp.Compare(a.start, b.start) })
	for i := 1; i < len(spans); i++ {
		if spans[i].start < spans[i-1].end {
			return refuse("two entries' data overlap")
		}
	}
	if program == nil {
		return refuse("the archive holds no program")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	rc, err := program.Open()
	if err != nil {
		return refuse("entry %q: %v", program.Name, err)
	}
	_, err = copyProgram(w, rc, limit)
	return errors.Join(err, rc.Close())
}

// toInt64 converts a zip size, failing on one no file could have.
func toInt64(u uint64) (int64, bool) {
	if u > math.MaxInt64 {
		return 0, false
	}
	return int64(u), true
}

// extractGz writes one gzip member's contents, and refuses anything after
// it.
func extractGz(r io.Reader, w io.Writer, limit int64) error {
	br := bufio.NewReader(r)
	gz, err := gzip.NewReader(br)
	if err != nil {
		return refuse("gzip: %v", err)
	}
	gz.Multistream(false)
	if _, err := copyProgram(w, gz, limit); err != nil {
		return err
	}
	if _, err := br.ReadByte(); !errors.Is(err, io.EOF) {
		return refuse("data follows the gzip member")
	}
	return nil
}
