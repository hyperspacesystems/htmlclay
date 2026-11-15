#!/usr/bin/env python3
"""Writes the Sparkle feed text: the new item, and optionally the last 1.x item.

A Mac below the new item's floor skips it and takes the legacy item, so 1.x
users on macOS 11 to 13 keep updating within 1.x and are never offered 2.x.
Pure: everything comes in as arguments, the feed text goes to stdout.
"""
import argparse
import sys
import xml.etree.ElementTree as ET

SPARKLE = "http://www.andymatuschak.org/xml-namespaces/sparkle"
NS = {"sparkle": SPARKLE}


class FeedError(Exception):
    pass


def version_tuple(text):
    parts = text.split(".")
    if len(parts) != 3 or not all(p.isdigit() for p in parts):
        raise FeedError(f"version must be X.Y.Z: {text!r}")
    return tuple(int(p) for p in parts)


def floor_tuple(text):
    parts = text.split(".")
    if not 1 <= len(parts) <= 3 or not all(p.isdigit() for p in parts):
        raise FeedError(f"minimum system version must be numeric: {text!r}")
    return tuple(int(p) for p in parts) + (0,) * (3 - len(parts))


def legacy_facts(item_xml):
    try:
        item = ET.fromstring(item_xml)
    except ET.ParseError as error:
        raise FeedError(f"the legacy item is not XML: {error}") from error
    if item.tag != "item":
        raise FeedError(f"the legacy item's root is <{item.tag}>, not <item>")
    version = item.findtext("sparkle:version", namespaces=NS) or ""
    floor = item.findtext("sparkle:minimumSystemVersion", namespaces=NS) or ""
    enclosure = item.find("enclosure")
    signature = enclosure.get(f"{{{SPARKLE}}}edSignature", "") if enclosure is not None else ""
    if not signature:
        raise FeedError("the legacy item has no enclosure with an edSignature")
    if not floor:
        raise FeedError("the legacy item has no minimumSystemVersion")
    return version_tuple(version), floor_tuple(floor)


def feed(version, zip_name, sha256, length, signature, floor, base_url, pub_date, legacy_item=""):
    new_version = version_tuple(version)
    new_floor = floor_tuple(floor)
    if new_version[0] >= 2 and new_floor < (14, 0, 0):
        raise FeedError(f"version {version} needs a floor of 14.0 or more, not {floor}")
    legacy = legacy_item.strip()
    if legacy:
        legacy_version, legacy_floor = legacy_facts(legacy)
        if legacy_version >= new_version:
            raise FeedError(f"the legacy item is not older than {version}")
        if legacy_floor > new_floor:
            raise FeedError(f"the legacy item's floor is above {floor}")
    items = [f"""    <item>
      <title>HTML Clay {version}</title>
      <pubDate>{pub_date}</pubDate>
      <sparkle:version>{version}</sparkle:version>
      <sparkle:shortVersionString>{version}</sparkle:shortVersionString>
      <sparkle:minimumSystemVersion>{floor}</sparkle:minimumSystemVersion>
      <enclosure url="{base_url}/{version}/{zip_name}?sha256={sha256}" length="{length}" type="application/octet-stream" sparkle:edSignature="{signature}"/>
    </item>"""]
    if legacy:
        items.append("    " + legacy)
    body = "\n".join(items)
    return f"""<?xml version="1.0" encoding="utf-8"?>
<rss version="2.0" xmlns:sparkle="{SPARKLE}">
  <channel>
    <title>HTML Clay</title>
{body}
  </channel>
</rss>
"""


def main(argv):
    parser = argparse.ArgumentParser(description=__doc__)
    for name in ("version", "zip-name", "sha256", "length", "signature", "floor", "base-url", "pub-date"):
        parser.add_argument(f"--{name}", required=True)
    parser.add_argument("--legacy-item", default="", help="file holding one <item> from the 1.x feed")
    args = parser.parse_args(argv)
    legacy = ""
    if args.legacy_item:
        with open(args.legacy_item, encoding="utf-8") as handle:
            legacy = handle.read()
    try:
        sys.stdout.write(feed(args.version, args.zip_name, args.sha256, args.length, args.signature,
                              args.floor, args.base_url, args.pub_date, legacy))
    except FeedError as error:
        print(f"appcast_xml.py: {error}", file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
