#!/bin/bash
# Usage: capture-legacy-item.sh [FEED_URL_OR_FILE]
#
# Run by hand at the release candidate, never by tests. Copies the newest 1.x
# <item> out of the public feed, unchanged, into appcast-legacy-item.xml beside
# this script. sparkle-appcast.sh hands that file to appcast_xml.py, so the feed
# it writes still carries a 1.x update for Macs that cannot run 2.x.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
FEED="${1:-https://download.htmlclay.com/update/macos/appcast.xml}"
OUT="$SCRIPT_DIR/appcast-legacy-item.xml"

case "$FEED" in
  http://*|https://*) FEED_TEXT="$(curl -fsSL "$FEED")" ;;
  *) FEED_TEXT="$(cat "$FEED")" ;;
esac

FEED_TMP="$(mktemp)"
trap 'rm -f "$FEED_TMP"' EXIT
printf '%s' "$FEED_TEXT" > "$FEED_TMP"

python3 - "$FEED_TMP" "$OUT" <<'PY'
import sys
import xml.etree.ElementTree as ET

SPARKLE = "http://www.andymatuschak.org/xml-namespaces/sparkle"
NS = {"sparkle": SPARKLE}
feed_path = sys.argv[1]
out_path = sys.argv[2]


def fail(message):
    print(f"capture-legacy-item.sh: {message}", file=sys.stderr)
    sys.exit(1)


try:
    root = ET.parse(feed_path).getroot()
except ET.ParseError as error:
    fail(f"the feed is not XML: {error}")

items = root.findall(".//item")
if not items:
    fail("the feed has no <item>")


def version_of(item):
    version = item.findtext("sparkle:version", namespaces=NS) or ""
    parts = version.split(".")
    try:
        return tuple(int(part) for part in parts)
    except ValueError:
        fail(f"sparkle:version is not numeric: {version!r}")


newest = max(items, key=version_of)
version = newest.findtext("sparkle:version", namespaces=NS) or ""
floor = newest.findtext("sparkle:minimumSystemVersion", namespaces=NS) or ""
enclosure = newest.find("enclosure")
signature = enclosure.get(f"{{{SPARKLE}}}edSignature", "") if enclosure is not None else ""

if version_of(newest)[0] != 1:
    fail(f"the newest item is {version}, not a 1.x release")
if floor != "11.0":
    fail(f"the newest 1.x item's floor is {floor or 'missing'}, not 11.0")
if not signature:
    fail("the newest 1.x item has no enclosure with an edSignature")

ET.register_namespace("sparkle", SPARKLE)
with open(out_path, "w", encoding="utf-8") as handle:
    handle.write(ET.tostring(newest, encoding="unicode"))
    handle.write("\n")
print(f"Captured HTML Clay {version} (floor 11.0) into {out_path}")
PY
