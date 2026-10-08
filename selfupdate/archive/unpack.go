package archive

import (
	"archive/tar"
	"archive/zip"
	"bufio"
	"bytes"
	"cmp"
	"compress/gzip"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"maps"
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

// The zip format's fixed values this package checks (APPNOTE.TXT 4.3.7,
// 4.4.4, 4.6.1).
const (
	// localHeaderLen is a local file header's length before its name.
	localHeaderLen = 30
	// encryptedFlags are the general-purpose flags for an encrypted entry:
	// encrypted, strong encryption, and a masked local header.
	encryptedFlags = 0x2041
	// unicodePathTag is Info-ZIP's Unicode Path extra field.
	unicodePathTag = 0x7075
)

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
	if err := checkPortable(name, clean); err != nil {
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

// skip counts an entry that is not extracted, such as a PAX global header,
// toward the entry limit (0015-MADR E8).
func (e *entries) skip() error {
	e.count++
	if e.count > e.max {
		return refuse("more than %d entries", e.max)
	}
	return nil
}

// globalHeader checks a PAX global header, such as the one git archive
// writes with the commit as a comment. One that sets an entry's path, link,
// size or sparse map would change how another tool reads every entry, and
// is refused; any other is skipped, and counts as an entry (0015-MADR E8).
func globalHeader(hdr *tar.Header, seen *entries) error {
	for _, k := range slices.Sorted(maps.Keys(hdr.PAXRecords)) {
		if k == "path" || k == "linkpath" || k == "size" || strings.HasPrefix(k, "GNU.sparse.") {
			return refuse("a PAX global header sets %q", k)
		}
	}
	return seen.skip()
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

// checkPortable refuses a name that a file system could make another's:
// a byte outside printable ASCII, which APFS and NTFS may fold (U+017F ſ
// to s), and an element ending in a dot or a space, which Win32 strips.
// With both refused, strings.ToLower is the folding those file systems
// apply (0015-MADR E2).
func checkPortable(name, clean string) error {
	for i := range len(name) {
		if name[i] < 0x20 || name[i] > 0x7e {
			return refuse("entry name %q is not printable ASCII", name)
		}
	}
	for _, el := range strings.Split(clean, "/") {
		if el != "." && el != ".." && (strings.HasSuffix(el, ".") || strings.HasSuffix(el, " ")) {
			return refuse("entry name %q has an element ending in a dot or a space", name)
		}
	}
	return nil
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
		if hdr.Typeflag == tar.TypeXGlobalHeader {
			if err := globalHeader(hdr, seen); err != nil {
				return err
			}
			continue
		}
		clean, err := seen.add(hdr.Name, hdr.Size)
		if err != nil {
			return err
		}
		switch hdr.Typeflag {
		case tar.TypeDir:
			continue
		case tar.TypeReg, '\x00':
			// bsdtar makes a directory of it (0015-MADR E3).
			if strings.HasSuffix(hdr.Name, "/") {
				return refuse("regular file %q is named as a directory", hdr.Name)
			}
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

// headerRecorder is the io.ReaderAt the zip reader reads through. It
// records the last read, so the offset of the local header that
// zip.File.DataOffset reads, which archive/zip does not export, is known
// (0015-MADR E1).
type headerRecorder struct {
	r   io.ReaderAt
	off int64
	n   int
}

func (h *headerRecorder) ReadAt(p []byte, off int64) (int, error) {
	h.off, h.n = off, len(p)
	return h.r.ReadAt(p, off)
}

// local is one zip entry with its local header's offset and its data's.
type local struct {
	zf        *zip.File
	hdr, data int64
}

func (u *unpacker) extractZip(ctx context.Context, ra io.ReaderAt, size int64, w io.Writer, isProgram func(string) bool, limit int64) error {
	rec := &headerRecorder{r: ra}
	zr, err := zip.NewReader(rec, size)
	if err != nil {
		return refuse("zip: %v", err)
	}
	if len(zr.File) > u.maxEntries {
		return refuse("more than %d entries", u.maxEntries)
	}
	seen := newEntries(u.maxEntries, limit)
	var spans []span
	var locals []local
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
		// Info-ZIP's unzip extracts an entry by its name, whatever its
		// attributes say (0015-MADR E3).
		if mode.IsDir() != strings.HasSuffix(zf.Name, "/") {
			return refuse("entry %q: its directory attribute does not match its name", zf.Name)
		}
		if mode.IsDir() && (zf.UncompressedSize64 > 0 || zf.CompressedSize64 > 0) {
			return refuse("directory %q holds data", zf.Name)
		}
		if zf.Flags&encryptedFlags != 0 {
			return refuse("entry %q is encrypted (flags %#04x)", zf.Name, zf.Flags)
		}
		if zf.Method != zip.Store && zf.Method != zip.Deflate {
			return refuse("entry %q uses compression method %d", zf.Name, zf.Method)
		}
		rec.n = 0
		off, err := zf.DataOffset()
		if err != nil {
			return refuse("entry %q: %v", zf.Name, err)
		}
		if rec.n != localHeaderLen {
			return refuse("entry %q: its local header could not be located", zf.Name)
		}
		locals = append(locals, local{zf, rec.off, off})
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
	for _, l := range locals {
		if err := checkLocal(ra, l.zf, l.hdr, l.data); err != nil {
			return err
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

// checkLocal requires an entry's local header, at hdr, to agree with its
// central-directory record: the signature, no encryption, the method, the
// name, and the lengths that put its data at data. Go reads the central
// record; bsdtar and Info-ZIP read the local header, so an archive they
// would read differently is refused (0015-MADR E1, E4).
func checkLocal(ra io.ReaderAt, zf *zip.File, hdr, data int64) error {
	var b [localHeaderLen]byte
	if _, err := ra.ReadAt(b[:], hdr); err != nil {
		return refuse("entry %q: reading its local header: %v", zf.Name, err)
	}
	if string(b[:4]) != "PK\x03\x04" {
		return refuse("entry %q: its local header has no signature", zf.Name)
	}
	if flags := binary.LittleEndian.Uint16(b[6:]); flags&encryptedFlags != 0 {
		return refuse("entry %q is encrypted (local flags %#04x)", zf.Name, flags)
	}
	if m := binary.LittleEndian.Uint16(b[8:]); m != zf.Method {
		return refuse("entry %q: its local header names compression method %d, not %d", zf.Name, m, zf.Method)
	}
	n, m := int64(binary.LittleEndian.Uint16(b[26:])), int64(binary.LittleEndian.Uint16(b[28:]))
	if hdr+localHeaderLen+n+m != data {
		return refuse("entry %q: its local header does not end where its data starts", zf.Name)
	}
	rest := make([]byte, n+m)
	if _, err := ra.ReadAt(rest, hdr+localHeaderLen); err != nil {
		return refuse("entry %q: reading its local header: %v", zf.Name, err)
	}
	if string(rest[:n]) != zf.Name {
		return refuse("entry %q: its local header names %q", zf.Name, rest[:n])
	}
	if err := checkExtra(zf.Name, zf.Extra); err != nil {
		return err
	}
	return checkExtra(zf.Name, rest[n:])
}

// checkExtra walks an extra field's [tag][size][data] records. It refuses
// one that runs past the field, and Info-ZIP's Unicode Path field, which
// bsdtar and Info-ZIP use as the entry's name in place of the header's
// (0015-MADR E1).
func checkExtra(name string, b []byte) error {
	for len(b) > 0 {
		if len(b) < 4 {
			return refuse("entry %q: its extra field overruns", name)
		}
		tag, size := binary.LittleEndian.Uint16(b), int(binary.LittleEndian.Uint16(b[2:]))
		if 4+size > len(b) {
			return refuse("entry %q: its extra field overruns", name)
		}
		if tag == unicodePathTag {
			return refuse("entry %q has an Info-ZIP Unicode Path field", name)
		}
		b = b[4+size:]
	}
	return nil
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
