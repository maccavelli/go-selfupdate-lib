#!/bin/sh
# Verify a staged self-update release directory against the canonical
# product/platform/extra matrix. Performs no publication.
set -eu

usage() {
	echo "usage: verify-selfupdate-release.sh --dir DIR --products JSON --platforms JSON --extras JSON [--tag TAG [--channels JSON]]" >&2
	exit 2
}

DIR=""
PRODUCTS_JSON=""
PLATFORMS_JSON=""
EXTRAS_JSON="[]"
TAG=""
CHANNELS_JSON="[]"

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

python3 -B - "$SCRIPTS" "$DIR" "$PRODUCTS_JSON" "$PLATFORMS_JSON" "$EXTRAS_JSON" <<'PY'
import hashlib, json, os, re, stat, sys

scripts_dir, dirpath, products_raw, platforms_raw, extras_raw = sys.argv[1:6]
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

seen_plats = set()
for plat in platforms:
    if not isinstance(plat, dict) or set(plat.keys()) != {"os", "arch"}:
        fail("platform objects must have only os and arch")
    osname, arch = plat["os"], plat["arch"]
    if not isinstance(osname, str) or not os_arch_re.fullmatch(osname):
        fail("invalid platform os %r" % osname)
    if not isinstance(arch, str) or not os_arch_re.fullmatch(arch):
        fail("invalid platform arch %r" % arch)
    key = (osname, arch)
    if key in seen_plats:
        fail("duplicate platform %s/%s" % key)
    seen_plats.add(key)

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
    if osname == "windows":
        name += ".exe"
    return name

canonical = []
for product in products:
    for osname, arch in seen_plats:
        canonical.append(asset_name(product, osname, arch))

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
    fail("SHA256SUMS must contain exactly the canonical binaries")

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

print("verify-selfupdate-release: ok")
PY
