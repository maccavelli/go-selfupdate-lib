#!/bin/sh
# Offline fixtures for scripts/verify-selfupdate-release.sh. Performs no publication.
set -eu

ROOT=$(cd -- "$(dirname "$0")/.." && pwd)
SCRIPT="$ROOT/scripts/verify-selfupdate-release.sh"
WORKDIR=$(mktemp -d)
trap 'rm -rf "$WORKDIR"' EXIT

pass() {
	echo "ok - $1"
}

fail() {
	echo "not ok - $1" >&2
	exit 1
}

PRODUCTS='["demo"]'
PLATFORMS='[{"os":"linux","arch":"amd64"},{"os":"windows","arch":"amd64"}]'
EXTRAS='["install.sh"]'

digest_of() {
	python3 -c 'import hashlib,sys; print(hashlib.sha256(open(sys.argv[1],"rb").read()).hexdigest())' "$1"
}

make_valid() {
	d="$1"
	mkdir -p "$d"
	printf 'linux-body' >"$d/demo-linux-amd64"
	printf 'win-body' >"$d/demo-windows-amd64.exe"
	printf 'installer' >"$d/install.sh"
	linux=$(digest_of "$d/demo-linux-amd64")
	win=$(digest_of "$d/demo-windows-amd64.exe")
	printf '%s  demo-linux-amd64\n%s  demo-windows-amd64.exe\n' "$linux" "$win" >"$d/SHA256SUMS"
}

run_ok() {
	label="$1"
	shift
	if "$SCRIPT" "$@"; then
		pass "$label"
	else
		fail "$label"
	fi
}

run_fail() {
	label="$1"
	shift
	if "$SCRIPT" "$@" >/dev/null 2>&1; then
		fail "$label (expected failure)"
	else
		pass "$label"
	fi
}

# run_refused LABEL MESSAGE ARGS...: a validation failure (exit 1) whose
# output holds MESSAGE, so the case fails for the reason it names.
run_refused() {
	label="$1"
	msg="$2"
	shift 2
	rc=0
	out=$("$SCRIPT" "$@" 2>&1) || rc=$?
	case "$out" in
	*"$msg"*) [ "$rc" -eq 1 ] && pass "$label" && return ;;
	esac
	fail "$label (expected exit 1 with [$msg], got $rc: $out)"
}

# A usage error is exit 2, distinct from a validation failure (exit 1).
run_usage() {
	label="$1"
	shift
	rc=0
	"$SCRIPT" "$@" >/dev/null 2>&1 || rc=$?
	if [ "$rc" -eq 2 ]; then
		pass "$label"
	else
		fail "$label (expected usage exit 2, got $rc)"
	fi
}

VALID="$WORKDIR/valid"
make_valid "$VALID"
run_ok "valid artifact" \
	--dir "$VALID" --products "$PRODUCTS" --platforms "$PLATFORMS" --extras "$EXTRAS"

MISSING="$WORKDIR/missing"
cp -R "$VALID" "$MISSING"
rm -f "$MISSING/demo-linux-amd64"
run_fail "missing binary" \
	--dir "$MISSING" --products "$PRODUCTS" --platforms "$PLATFORMS" --extras "$EXTRAS"

DUP="$WORKDIR/dup"
cp -R "$VALID" "$DUP"
linux=$(digest_of "$DUP/demo-linux-amd64")
printf '%s  demo-linux-amd64\n%s  demo-linux-amd64\n' "$linux" "$linux" >"$DUP/SHA256SUMS"
run_fail "duplicate checksum entry" \
	--dir "$DUP" --products "$PRODUCTS" --platforms "$PLATFORMS" --extras "$EXTRAS"

EXTRA="$WORKDIR/extra"
cp -R "$VALID" "$EXTRA"
printf 'nope' >"$EXTRA/unexpected"
run_fail "undeclared extra file" \
	--dir "$EXTRA" --products "$PRODUCTS" --platforms "$PLATFORMS" --extras "$EXTRAS"

MALFORMED="$WORKDIR/malformed"
cp -R "$VALID" "$MALFORMED"
printf 'not-a-digest  demo-linux-amd64\n' >"$MALFORMED/SHA256SUMS"
run_fail "malformed SHA256SUMS" \
	--dir "$MALFORMED" --products "$PRODUCTS" --platforms "$PLATFORMS" --extras "$EXTRAS"

run_ok "strict tag accepted" \
	--dir "$VALID" --products "$PRODUCTS" --platforms "$PLATFORMS" --extras "$EXTRAS" \
	--tag v1.2.3

run_fail "non-strict tag rejected" \
	--dir "$VALID" --products "$PRODUCTS" --platforms "$PLATFORMS" --extras "$EXTRAS" \
	--tag v1.2

# 0005-MADR §5: a prerelease tag needs its channel named, and the rule is
# check-release-tag.sh's.
run_fail "prerelease tag without channels" \
	--dir "$VALID" --products "$PRODUCTS" --platforms "$PLATFORMS" --extras "$EXTRAS" \
	--tag v1.2.3-rc.1
run_ok "prerelease tag on a named channel" \
	--dir "$VALID" --products "$PRODUCTS" --platforms "$PLATFORMS" --extras "$EXTRAS" \
	--tag v1.2.3-rc.1 --channels '["rc","beta"]'
run_fail "prerelease tag on another channel" \
	--dir "$VALID" --products "$PRODUCTS" --platforms "$PLATFORMS" --extras "$EXTRAS" \
	--tag v1.2.3-alpha.1 --channels '["rc","beta"]'
run_usage "channels out of order" \
	--dir "$VALID" --products "$PRODUCTS" --platforms "$PLATFORMS" --extras "$EXTRAS" \
	--tag v1.2.3 --channels '["beta","rc"]'

# The v0.16.0 compatibility bridge was not carried over from mcplib
# (docs/decisions/0002-MADR-rehome-selfupdate-from-mcplib.md §3).
run_usage "--bridge is not an option" \
	--dir "$VALID" --products "$PRODUCTS" --platforms "$PLATFORMS" --extras "$EXTRAS" \
	--bridge true

run_usage "missing required arguments" \
	--products "$PRODUCTS" --platforms "$PLATFORMS"

run_usage "--repository is not an option" \
	--dir "$VALID" --products "$PRODUCTS" --platforms "$PLATFORMS" --extras "$EXTRAS" \
	--repository maccavelli/magic-cli-remote

# 0003-MADR D1: the fixtures the Go client's TestManifestParityFixtures runs.
# The gate must accept exactly the manifests the client can parse.
PARITY="$ROOT/selfupdate/testdata/manifest-parity"
count=0
for case_dir in "$PARITY"/*/; do
	name=$(basename "$case_dir")
	d="$WORKDIR/parity-$name"
	mkdir -p "$d"
	printf 'linux-body' >"$d/demo-linux-amd64"
	printf 'win-body' >"$d/demo-windows-amd64.exe"
	cp "$case_dir/SHA256SUMS" "$d/SHA256SUMS"
	expect=$(cat "$case_dir/expect")
	case "$expect" in
	accept)
		run_ok "parity $name" --dir "$d" --products "$PRODUCTS" --platforms "$PLATFORMS" --extras '[]'
		;;
	reject)
		run_fail "parity $name" --dir "$d" --products "$PRODUCTS" --platforms "$PLATFORMS" --extras '[]'
		;;
	*)
		fail "parity $name: bad expect file"
		;;
	esac
	count=$((count + 1))
done
[ "$count" -ge 20 ] || fail "only $count parity fixtures found"

# 0003-MADR D10: staging-set cases.
NOEXE="$WORKDIR/noexe"
make_valid "$NOEXE"
mv "$NOEXE/demo-windows-amd64.exe" "$NOEXE/demo-windows-amd64"
printf '%s  demo-linux-amd64\n%s  demo-windows-amd64\n' "$linux" "$win" >"$NOEXE/SHA256SUMS"
run_fail "windows binary without .exe" \
	--dir "$NOEXE" --products "$PRODUCTS" --platforms "$PLATFORMS" --extras "$EXTRAS"

run_fail "empty platforms" \
	--dir "$VALID" --products "$PRODUCTS" --platforms '[]' --extras "$EXTRAS"

SUBDIR="$WORKDIR/subdir"
make_valid "$SUBDIR"
mkdir "$SUBDIR/nested"
run_fail "directory in staging" \
	--dir "$SUBDIR" --products "$PRODUCTS" --platforms "$PLATFORMS" --extras "$EXTRAS"

# 0003-MADR D6: a staged symlink is refused, even when its target's bytes
# match. Git Bash may make a copy instead of a link; only a real link tests
# anything.
LINKED="$WORKDIR/linked"
make_valid "$LINKED"
mv "$LINKED/demo-linux-amd64" "$WORKDIR/linked-target"
ln -s "$WORKDIR/linked-target" "$LINKED/demo-linux-amd64" 2>/dev/null || true
if [ -L "$LINKED/demo-linux-amd64" ]; then
	run_fail "symlinked binary in staging" \
		--dir "$LINKED" --products "$PRODUCTS" --platforms "$PLATFORMS" --extras "$EXTRAS"
else
	echo "skip - symlinked binary in staging (this shell cannot create a symlink)"
fi

# 0003-MADR D5: extras must be safe single shell words.
for extra in 'install me.sh' '*' 'a;b' '-flag'; do
	UNSAFE="$WORKDIR/unsafe-$count"
	count=$((count + 1))
	make_valid "$UNSAFE"
	printf 'x' >"$UNSAFE/$extra"
	run_fail "unsafe extra name [$extra]" \
		--dir "$UNSAFE" --products "$PRODUCTS" --platforms "$PLATFORMS" --extras "[\"$extra\"]"
done
for extra in 'install.ps1' 'magic-cli-remote-v0.20.0-arm64.apk'; do
	SAFE="$WORKDIR/safe-$count"
	count=$((count + 1))
	make_valid "$SAFE"
	rm -f "$SAFE/install.sh"
	printf 'x' >"$SAFE/$extra"
	run_ok "consumer extra name [$extra]" \
		--dir "$SAFE" --products "$PRODUCTS" --platforms "$PLATFORMS" --extras "[\"$extra\"]"
done

# 0004-MADR R6: a trailing newline is part of neither a strict tag nor a
# safe name. Python's "$" matches before a final newline, so each check must
# match the whole string.
nl='
'
run_fail "tag with a trailing newline" \
	--dir "$VALID" --products "$PRODUCTS" --platforms "$PLATFORMS" --extras "$EXTRAS" \
	--tag "v1.2.3$nl"
NLX="$WORKDIR/nl-extra"
make_valid "$NLX"
rm -f "$NLX/install.sh"
# The file must exist, or the old check refuses the name for being missing
# rather than for its newline.
if printf 'x' >"$NLX/notes$nl" 2>/dev/null; then
	run_fail "extra name with a trailing newline" \
		--dir "$NLX" --products "$PRODUCTS" --platforms "$PLATFORMS" --extras '["notes\n"]'
else
	echo "skip - extra name with a trailing newline (this filesystem cannot hold one)"
fi

# 0010-MADR D2: the two core checks, each with a case only it can catch.
TAMPERED="$WORKDIR/tampered"
make_valid "$TAMPERED"
printf 'tampered-body' >"$TAMPERED/demo-linux-amd64"
run_fail "binary bytes differ from its SHA256SUMS line" \
	--dir "$TAMPERED" --products "$PRODUCTS" --platforms "$PLATFORMS" --extras "$EXTRAS"
LISTSEXTRA="$WORKDIR/lists-extra"
make_valid "$LISTSEXTRA"
printf '%s  install.sh\n' "$(digest_of "$LISTSEXTRA/install.sh")" >>"$LISTSEXTRA/SHA256SUMS"
run_fail "SHA256SUMS also lists an extra" \
	--dir "$LISTSEXTRA" --products "$PRODUCTS" --platforms "$PLATFORMS" --extras "$EXTRAS"
SUMSX="$WORKDIR/sums-x"
make_valid "$SUMSX"
printf 'x' >"$SUMSX/SHA256SUMS-x"
run_fail "extra named SHA256SUMS-x" \
	--dir "$SUMSX" --products "$PRODUCTS" --platforms "$PLATFORMS" --extras '["install.sh","SHA256SUMS-x"]'

# 0010-MADR D8: the client refuses an asset of size 0, so the verifier must.
EMPTYBIN="$WORKDIR/empty-binary"
make_valid "$EMPTYBIN"
: >"$EMPTYBIN/demo-linux-amd64"
printf '%s  demo-linux-amd64\n%s  demo-windows-amd64.exe\n' \
	"$(digest_of "$EMPTYBIN/demo-linux-amd64")" "$(digest_of "$EMPTYBIN/demo-windows-amd64.exe")" >"$EMPTYBIN/SHA256SUMS"
run_fail "empty canonical binary" \
	--dir "$EMPTYBIN" --products "$PRODUCTS" --platforms "$PLATFORMS" --extras "$EXTRAS"

# 0013-MADR §8: a packed release. Every platform carries a format; the
# canonical assets are the archives, and SHA256SUMS lists them.
PACKED_PLATFORMS='[{"os":"linux","arch":"amd64","format":"tar.gz"},{"os":"windows","arch":"amd64","format":"zip"},{"os":"darwin","arch":"arm64","format":"gz"}]'
make_packed() {
	d="$1"
	mkdir -p "$d"
	printf 'tgz-body' >"$d/demo-linux-amd64.tar.gz"
	printf 'zip-body' >"$d/demo-windows-amd64.zip"
	printf 'gz-body' >"$d/demo-darwin-arm64.gz"
	printf 'installer' >"$d/install.sh"
	{
		printf '%s  demo-darwin-arm64.gz\n' "$(digest_of "$d/demo-darwin-arm64.gz")"
		printf '%s  demo-linux-amd64.tar.gz\n' "$(digest_of "$d/demo-linux-amd64.tar.gz")"
		printf '%s  demo-windows-amd64.zip\n' "$(digest_of "$d/demo-windows-amd64.zip")"
	} >"$d/SHA256SUMS"
}
PACKED="$WORKDIR/packed"
make_packed "$PACKED"
run_ok "packed release: tar.gz, zip and gz" \
	--dir "$PACKED" --products "$PRODUCTS" --platforms "$PACKED_PLATFORMS" --extras "$EXTRAS"

# Staged to match a mixed declaration exactly, so only the mixed rule can
# refuse it.
MIXED="$WORKDIR/mixed"
make_packed "$MIXED"
mv "$MIXED/demo-windows-amd64.zip" "$MIXED/demo-windows-amd64.exe"
{
	printf '%s  demo-darwin-arm64.gz\n' "$(digest_of "$MIXED/demo-darwin-arm64.gz")"
	printf '%s  demo-linux-amd64.tar.gz\n' "$(digest_of "$MIXED/demo-linux-amd64.tar.gz")"
	printf '%s  demo-windows-amd64.exe\n' "$(digest_of "$MIXED/demo-windows-amd64.exe")"
} >"$MIXED/SHA256SUMS"
run_refused "a format on some platforms only" "a format is on some platforms only" \
	--dir "$MIXED" --products "$PRODUCTS" --extras "$EXTRAS" \
	--platforms '[{"os":"linux","arch":"amd64","format":"tar.gz"},{"os":"windows","arch":"amd64"},{"os":"darwin","arch":"arm64","format":"gz"}]'
run_refused "an unknown format" "unknown archive format 'tar.xz'" \
	--dir "$PACKED" --products "$PRODUCTS" --extras "$EXTRAS" \
	--platforms '[{"os":"linux","arch":"amd64","format":"tar.xz"},{"os":"windows","arch":"amd64","format":"zip"},{"os":"darwin","arch":"arm64","format":"gz"}]'
run_refused "a platform key other than format" "nothing else but a format" \
	--dir "$PACKED" --products "$PRODUCTS" --extras "$EXTRAS" \
	--platforms '[{"os":"linux","arch":"amd64","format":"tar.gz","variant":"v1"},{"os":"windows","arch":"amd64","format":"zip"},{"os":"darwin","arch":"arm64","format":"gz"}]'

RAWINPACKED="$WORKDIR/raw-in-packed"
make_packed "$RAWINPACKED"
mv "$RAWINPACKED/demo-linux-amd64.tar.gz" "$RAWINPACKED/demo-linux-amd64"
run_refused "a raw binary staged where an archive is declared" "file set mismatch missing=['demo-linux-amd64.tar.gz'] extra=['demo-linux-amd64']" \
	--dir "$RAWINPACKED" --products "$PRODUCTS" --platforms "$PACKED_PLATFORMS" --extras "$EXTRAS"

RAWSUMS="$WORKDIR/raw-sums"
make_packed "$RAWSUMS"
for n in demo-linux-amd64 demo-windows-amd64.exe demo-darwin-arm64; do
	printf 'body' >"$RAWSUMS/$n"
done
{
	printf '%s  demo-darwin-arm64\n' "$(digest_of "$RAWSUMS/demo-darwin-arm64")"
	printf '%s  demo-linux-amd64\n' "$(digest_of "$RAWSUMS/demo-linux-amd64")"
	printf '%s  demo-windows-amd64.exe\n' "$(digest_of "$RAWSUMS/demo-windows-amd64.exe")"
} >"$RAWSUMS/SHA256SUMS"
run_refused "SHA256SUMS lists the raw binaries in a packed release" "SHA256SUMS must contain exactly the canonical archives" \
	--dir "$RAWSUMS" --products "$PRODUCTS" --platforms "$PACKED_PLATFORMS" \
	--extras '["install.sh","demo-linux-amd64","demo-windows-amd64.exe","demo-darwin-arm64"]'

run_refused "an archive name listed as an extra" "'demo-linux-amd64.tar.gz' is named like a canonical asset" \
	--dir "$PACKED" --products "$PRODUCTS" --platforms "$PACKED_PLATFORMS" \
	--extras '["install.sh","demo-linux-amd64.tar.gz"]'
run_refused "an archive name listed as an extra, in another case" "'Demo-Linux-AMD64.tar.gz' is named like a canonical asset" \
	--dir "$PACKED" --products "$PRODUCTS" --platforms "$PACKED_PLATFORMS" \
	--extras '["install.sh","Demo-Linux-AMD64.tar.gz"]'
# A raw release keeps its behaviour from before 0013: an extra named like a
# binary is merged into that binary's entry, as it always was (D5).
run_ok "raw release: an extra named like a binary is accepted, as before" \
	--dir "$VALID" --products "$PRODUCTS" --platforms "$PLATFORMS" \
	--extras '["install.sh","demo-linux-amd64"]'

# --github-output: packed=true or packed=false after a pass, nothing after
# a failure.
OUTFILE="$WORKDIR/github-output"
: >"$OUTFILE"
run_ok "packed release with --github-output" \
	--dir "$PACKED" --products "$PRODUCTS" --platforms "$PACKED_PLATFORMS" --extras "$EXTRAS" --github-output "$OUTFILE"
[ "$(cat "$OUTFILE")" = "packed=true" ] || fail "packed output is [$(cat "$OUTFILE")]"
pass "a packed release writes packed=true"
: >"$OUTFILE"
run_ok "raw release with --github-output" \
	--dir "$VALID" --products "$PRODUCTS" --platforms "$PLATFORMS" --extras "$EXTRAS" --github-output "$OUTFILE"
[ "$(cat "$OUTFILE")" = "packed=false" ] || fail "raw output is [$(cat "$OUTFILE")]"
pass "a raw release writes packed=false"
: >"$OUTFILE"
run_fail "failed verification with --github-output" \
	--dir "$RAWINPACKED" --products "$PRODUCTS" --platforms "$PACKED_PLATFORMS" --extras "$EXTRAS" --github-output "$OUTFILE"
[ ! -s "$OUTFILE" ] || fail "a failed verification wrote [$(cat "$OUTFILE")]"
pass "a failed verification writes no output"
run_usage "--github-output without a file" \
	--dir "$VALID" --products "$PRODUCTS" --platforms "$PLATFORMS" --extras "$EXTRAS" --github-output ""

echo "verify-selfupdate-release_test: all fixtures passed"
