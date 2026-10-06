#!/bin/sh
# Verify a staged self-update release directory against the canonical
# product/platform/extra matrix. Performs no publication.
#
# A platform object may carry "format" (tar.gz, zip or gz), on every object
# or none: the release is then packed, each canonical asset is
# <product>-<os>-<arch>.<format>, and SHA256SUMS lists those archives
# (docs/decisions/0013-MADR-build-and-stage-release-workflow.md §8).
# --github-output FILE appends packed=true or packed=false to FILE after a
# successful verification, and nothing on a failure.
set -eu

usage() {
	echo "usage: verify-selfupdate-release.sh --dir DIR --products JSON --platforms JSON --extras JSON [--tag TAG [--channels JSON]] [--github-output FILE]" >&2
	exit 2
}

DIR=""
PRODUCTS_JSON=""
PLATFORMS_JSON=""
EXTRAS_JSON="[]"
TAG=""
CHANNELS_JSON="[]"
GH_OUTPUT=""

while [ $# -gt 0 ]; do
	case "$1" in
	--dir)
		DIR="${2:-}"
		shift 2
		;;
	--products)
		PRODUCTS_JSON="${2:-}"
		shift 2
		;;
	--platforms)
		PLATFORMS_JSON="${2:-}"
		shift 2
		;;
	--extras)
		EXTRAS_JSON="${2:-}"
		shift 2
		;;
	--tag)
		TAG="${2:-}"
		shift 2
		;;
	--channels)
		CHANNELS_JSON="${2:-}"
		shift 2
		;;
	--github-output)
		GH_OUTPUT="${2:-}"
		[ -n "$GH_OUTPUT" ] || usage
		shift 2
		;;
	*)
		usage
		;;
	esac
done

if [ -z "$DIR" ] || [ -z "$PRODUCTS_JSON" ] || [ -z "$PLATFORMS_JSON" ]; then
	usage
fi
[ -d "$DIR" ] || {
	echo "verify-selfupdate-release: staging directory $DIR is missing" >&2
	exit 1
}

# The SHA256SUMS parser is selfupdate_manifest.py beside this script, so
# selfupdate's differential test runs the same code (0004-MADR H2, C1).
# -B writes no __pycache__ into the tools checkout.
SCRIPTS=$(cd -- "$(dirname -- "$0")" && pwd)

# The tag rule is check-release-tag.sh's, so the workflow's first check and
# this one cannot drift (0005-MADR §5). Its usage errors stay exit 2.
if [ -n "$TAG" ]; then
	rc=0
	sh "$SCRIPTS/check-release-tag.sh" "$TAG" "$CHANNELS_JSON" || rc=$?
	if [ "$rc" -ne 0 ]; then
		echo "verify-selfupdate-release: tag is not admitted" >&2
		exit "$rc"
	fi
fi

python3 -B - "$SCRIPTS" "$DIR" "$PRODUCTS_JSON" "$PLATFORMS_JSON" "$EXTRAS_JSON" "$GH_OUTPUT" <<'PY'
import hashlib, json, os, re, stat, sys

scripts_dir, dirpath, products_raw, platforms_raw, extras_raw, gh_output = sys.argv[1:7]
sys.path.insert(0, scripts_dir)
from selfupdate_manifest import ManifestError, parse_manifest

os_arch_re = re.compile(r"^[a-z0-9][a-z0-9_]*$")
product_re = re.compile(r"^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$")

def fail(msg):
    print("verify-selfupdate-release: " + msg, file=sys.stderr)
    sys.exit(1)

try:
    products = json.loads(products_raw)
    platforms = json.loads(platforms_raw)
    extras = json.loads(extras_raw)
except json.JSONDecodeError as e:
    fail("invalid JSON: %s" % e)

if not isinstance(products, list) or not products:
    fail("products-json must be a non-empty JSON array")
if not isinstance(platforms, list) or not platforms:
    fail("platforms-json must be a non-empty JSON array")
if not isinstance(extras, list):
    fail("extra-assets-json must be a JSON array")

seen_products = set()
for p in products:
    if not isinstance(p, str) or not product_re.fullmatch(p):
        fail("invalid product %r" % p)
    if p in seen_products:
        fail("duplicate product %s" % p)
    seen_products.add(p)

# The archive formats of 0013-MADR §8, and their extensions.
formats = {"tar.gz": ".tar.gz", "zip": ".zip", "gz": ".gz"}

seen_plats = set()
plat_format = {}
for plat in platforms:
    if not isinstance(plat, dict) or not {"os", "arch"} <= set(plat.keys()) <= {"os", "arch", "format"}:
        fail("platform objects must have os and arch, and nothing else but a format")
    osname, arch = plat["os"], plat["arch"]
    fmt = plat.get("format")
    if fmt is not None and fmt not in formats:
        fail("unknown archive format %r; want tar.gz, zip or gz" % (fmt,))
    if not isinstance(osname, str) or not os_arch_re.fullmatch(osname):
        fail("invalid platform os %r" % osname)
    if not isinstance(arch, str) or not os_arch_re.fullmatch(arch):
        fail("invalid platform arch %r" % arch)
    key = (osname, arch)
    if key in seen_plats:
        fail("duplicate platform %s/%s" % key)
    seen_plats.add(key)
    plat_format[key] = fmt

packed = any(f is not None for f in plat_format.values())
if packed and not all(f is not None for f in plat_format.values()):
    fail("a format is on some platforms only; a release is all archives or all binaries")

seen_extras = set()
for extra in extras:
    # The product-name character class: never a path, never a shell glob or
    # word separator, so the upload step can pass names safely
    # (0003-MADR D5).
    if not isinstance(extra, str) or not product_re.fullmatch(extra):
        fail("invalid extra asset %r" % extra)
    if extra in seen_extras or extra == "SHA256SUMS" or extra.startswith("SHA256SUMS-"):
        fail("invalid or duplicate extra asset %r" % extra)
    seen_extras.add(extra)

def asset_name(product, osname, arch):
    name = "%s-%s-%s" % (product, osname, arch)
    fmt = plat_format[(osname, arch)]
    if fmt is not None:
        return name + formats[fmt]
    if osname == "windows":
        name += ".exe"
    return name

canonical = []
for product in products:
    for osname, arch in seen_plats:
        canonical.append(asset_name(product, osname, arch))

# In a packed release, an extra may not take a canonical asset's name, in
# any case, as releasespec refuses it (0013-MADR §2). A raw release keeps
# its behaviour from before 0013, as §8 promises
# (0013-PLAN-build-and-stage-release-workflow.md D5).
if packed:
    canonical_lower = {c.lower() for c in canonical}
    for extra in seen_extras:
        if extra.lower() in canonical_lower:
            fail("extra asset %r is named like a canonical asset" % extra)

expected = set(canonical)
expected.add("SHA256SUMS")
expected.update(seen_extras)

present = []
for name in os.listdir(dirpath):
    path = os.path.join(dirpath, name)
    mode = os.lstat(path).st_mode
    if stat.S_ISDIR(mode):
        fail("unexpected directory %s in staging" % name)
    if not stat.S_ISREG(mode):
        fail("staged entry %s is not a regular file (0003-MADR D6)" % name)
    present.append(name)

present_set = set(present)
if present_set != expected:
    missing = sorted(expected - present_set)
    extra = sorted(present_set - expected)
    fail("file set mismatch missing=%s extra=%s" % (missing, extra))

with open(os.path.join(dirpath, "SHA256SUMS"), "rb") as f:
    raw = f.read()
try:
    sums = parse_manifest(raw, "SHA256SUMS")
except ManifestError as e:
    fail(str(e))
if set(sums) != set(canonical):
    fail("SHA256SUMS must contain exactly the canonical %s" % ("archives" if packed else "binaries"))

def sha256_file(path):
    h = hashlib.sha256()
    with open(path, "rb") as f:
        for chunk in iter(lambda: f.read(1024 * 1024), b""):
            h.update(chunk)
    return h.hexdigest()

for name in canonical:
    path = os.path.join(dirpath, name)
    # The client refuses an asset of size 0 (0010-MADR D8).
    if os.path.getsize(path) == 0:
        fail("%s is empty" % name)
    got = sha256_file(path)
    if got != sums[name]:
        fail("SHA256SUMS mismatch for %s" % name)

if gh_output:
    with open(gh_output, "a") as f:
        f.write("packed=%s\n" % ("true" if packed else "false"))
print("verify-selfupdate-release: ok%s" % (" (packed)" if packed else ""))
PY
