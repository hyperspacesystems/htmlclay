import importlib.util
from pathlib import Path
import shutil
import subprocess
import sys
import tempfile
import unittest
import xml.etree.ElementTree as ET

MACOS = Path(__file__).resolve().parents[2] / "packaging" / "macos"
SCRIPT = MACOS / "appcast_xml.py"
CAPTURE = MACOS / "capture-legacy-item.sh"
SPEC = importlib.util.spec_from_file_location("appcast_xml", SCRIPT)
appcast = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(appcast)

SPARKLE = "http://www.andymatuschak.org/xml-namespaces/sparkle"
NS = {"sparkle": SPARKLE}
BASE = "https://example.test/update"
ZIP = "htmlclay-2.0.0.zip"
SHA = "a" * 64
LENGTH = "1234567"
SIGNATURE = "new-signature"
PUB_DATE = "Mon, 05 Jan 2026 00:00:00 +0000"

LEGACY = """<item xmlns:sparkle="http://www.andymatuschak.org/xml-namespaces/sparkle">
      <title>HTML Clay 1.16.4</title>
      <pubDate>Mon, 01 Dec 2025 00:00:00 +0000</pubDate>
      <sparkle:version>1.16.4</sparkle:version>
      <sparkle:shortVersionString>1.16.4</sparkle:shortVersionString>
      <sparkle:minimumSystemVersion>11.0</sparkle:minimumSystemVersion>
      <enclosure url="https://example.test/update/1.16.4/htmlclay-1.16.4.zip" length="99" type="application/octet-stream" sparkle:edSignature="abc"/>
    </item>"""


def legacy_item(version="1.16.4", floor="11.0", signature="abc"):
    item = LEGACY.replace("1.16.4", version).replace(">11.0<", f">{floor}<")
    if signature is None:
        return item.replace(' sparkle:edSignature="abc"', "")
    return item.replace('sparkle:edSignature="abc"', f'sparkle:edSignature="{signature}"')


def make(**overrides):
    facts = dict(version="2.0.0", zip_name=ZIP, sha256=SHA, length=LENGTH, signature=SIGNATURE,
                 floor="14.0", base_url=BASE, pub_date=PUB_DATE)
    facts.update(overrides)
    return appcast.feed(**facts)


def items(text):
    return ET.fromstring(text).findall("./channel/item")


class FeedTests(unittest.TestCase):
    def test_one_item_has_the_floor(self):
        feed = items(make())
        self.assertEqual(len(feed), 1)
        item = feed[0]
        self.assertEqual(item.findtext("sparkle:minimumSystemVersion", namespaces=NS), "14.0")
        enclosure = item.find("enclosure")
        self.assertEqual(enclosure.get("url"), f"{BASE}/2.0.0/{ZIP}?sha256={SHA}")
        self.assertEqual(enclosure.get(f"{{{SPARKLE}}}edSignature"), SIGNATURE)
        self.assertEqual(enclosure.get("length"), LENGTH)

    def test_two_items_new_first(self):
        feed = items(make(legacy_item=LEGACY))
        self.assertEqual(len(feed), 2)
        self.assertEqual(feed[0].findtext("sparkle:version", namespaces=NS), "2.0.0")
        self.assertEqual(feed[0].findtext("sparkle:minimumSystemVersion", namespaces=NS), "14.0")
        self.assertEqual(feed[1].findtext("sparkle:version", namespaces=NS), "1.16.4")
        self.assertEqual(feed[1].findtext("sparkle:minimumSystemVersion", namespaces=NS), "11.0")
        self.assertEqual(feed[1].find("enclosure").get(f"{{{SPARKLE}}}edSignature"), "abc")

    def test_refuses_low_floor_for_2x(self):
        with self.assertRaises(appcast.FeedError):
            make(floor="13.0")
        self.assertEqual(len(items(make(version="1.17.0", floor="11.0"))), 1)

    def test_refuses_legacy_not_older(self):
        with self.assertRaises(appcast.FeedError):
            make(legacy_item=legacy_item(version="2.0.0"))

    def test_refuses_legacy_floor_above(self):
        with self.assertRaises(appcast.FeedError):
            make(legacy_item=legacy_item(floor="15.0"))

    def test_refuses_legacy_without_signature(self):
        with self.assertRaises(appcast.FeedError):
            make(legacy_item=legacy_item(signature=None))

    def test_cli_refusal_exits_1(self):
        result = subprocess.run(
            [sys.executable, str(SCRIPT), "--version", "2.0.0", "--zip-name", ZIP,
             "--sha256", SHA, "--length", LENGTH, "--signature", SIGNATURE,
             "--floor", "13.0", "--base-url", BASE, "--pub-date", PUB_DATE],
            capture_output=True, text=True)
        self.assertEqual(result.returncode, 1)
        self.assertIn("appcast_xml.py:", result.stderr)
        self.assertEqual(result.stdout, "")


class CaptureTests(unittest.TestCase):
    def fixture(self, top_version, top_signature="sig", second_version="1.16.3"):
        return f"""<?xml version="1.0" encoding="utf-8"?>
<rss version="2.0" xmlns:sparkle="{SPARKLE}">
  <channel>
    <title>HTML Clay</title>
{legacy_item(version=second_version)}
{legacy_item(version=top_version, signature=top_signature)}
  </channel>
</rss>
"""

    def run_capture(self, feed_text):
        workdir = Path(tempfile.mkdtemp())
        self.addCleanup(shutil.rmtree, workdir)
        script = workdir / "capture-legacy-item.sh"
        shutil.copy(CAPTURE, script)
        script.chmod(0o755)
        feed = workdir / "feed.xml"
        feed.write_text(feed_text, encoding="utf-8")
        result = subprocess.run([str(script), str(feed)], capture_output=True, text=True)
        return result, workdir / "appcast-legacy-item.xml"

    def test_capture_legacy_item(self):
        result, written = self.run_capture(self.fixture("1.16.4"))
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertIn("Captured HTML Clay 1.16.4 (floor 11.0)", result.stdout)
        captured = written.read_text(encoding="utf-8")
        self.assertIn("sparkle:", captured)
        self.assertNotIn("ns0:", captured)
        item = ET.fromstring(captured)
        self.assertEqual(item.findtext("sparkle:version", namespaces=NS), "1.16.4")
        self.assertEqual(item.findtext("sparkle:minimumSystemVersion", namespaces=NS), "11.0")
        feed = items(make(legacy_item=captured))
        self.assertEqual(feed[1].findtext("sparkle:version", namespaces=NS), "1.16.4")

        refused, _ = self.run_capture(self.fixture("2.0.0"))
        self.assertEqual(refused.returncode, 1)
        self.assertIn("capture-legacy-item.sh:", refused.stderr)


if __name__ == "__main__":
    unittest.main()
