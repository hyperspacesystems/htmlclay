import importlib.util
import json
import os
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest


SCRIPT = Path(__file__).resolve().parents[1] / "release_candidate.py"
SPEC = importlib.util.spec_from_file_location("release_candidate", SCRIPT)
candidate = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(candidate)

EXPECTED_IDS = [
    "verify", "test-macos", "test-linux", "test-windows", "license", "conformance",
    "build-linux-amd64", "build-linux-arm64", "build-windows-amd64", "build-windows-arm64",
    "build-macos", "sparkle", "update-feed", "checksums",
    "linux-amd64-install-uninstall", "linux-amd64-serve-save", "linux-amd64-tray-nobus",
    "linux-amd64-login-item", "linux-amd64-prompts",
    "linux-arm64-install-uninstall", "linux-arm64-serve-save", "linux-arm64-tray-nobus",
    "linux-arm64-login-item", "linux-arm64-prompts",
]
INPUTS = {"app": "htmlclay", "attempt": "attempt-3.4", "workflow_sha": "c" * 40,
          "candidate_sha256": "b" * 64, "manifest_identity": "a" * 64}


def green():
    return dict.fromkeys(EXPECTED_IDS, "passed")


class ReleaseCandidateTests(unittest.TestCase):
    def test_contract_cli(self):
        result = subprocess.run([sys.executable, str(SCRIPT), "contract"],
                                capture_output=True, text=True, check=True)
        contract = json.loads(result.stdout)
        self.assertEqual(contract["checks"], EXPECTED_IDS)
        self.assertEqual(contract["artifacts"], list(dict.fromkeys(
            ["identity"] + [{"sparkle-out/": "sparkle-out.tar.gz",
                             "stable.json or SKIPPED": "update-feed.tar.gz"}.get(name, name)
                            for job in candidate.job_graph() for name in job.get("artifacts", [])])))

    def test_full_inventory(self):
        graph = candidate.job_graph()
        self.assertEqual([job["id"] for job in graph], EXPECTED_IDS)
        self.assertEqual(len(set(EXPECTED_IDS)), len(graph))
        for job in graph:
            for dependency in job["needs"]:
                self.assertIn(dependency, EXPECTED_IDS[:EXPECTED_IDS.index(job["id"])])

    def test_no_test_to_build_edge(self):
        for job in candidate.job_graph():
            if job["kind"] in ("test", "build"):
                self.assertEqual(job["needs"], ["verify"])
        jobs = {job["id"]: job for job in candidate.job_graph()}
        self.assertEqual(jobs["sparkle"]["needs"], ["verify", "build-macos"])
        self.assertEqual(jobs["update-feed"]["needs"], ["verify", "build-linux-amd64",
                         "build-linux-arm64", "build-windows-amd64", "build-windows-arm64"])
        self.assertEqual(jobs["checksums"]["needs"], ["verify", "build-linux-amd64",
                         "build-linux-arm64", "build-windows-amd64", "build-windows-arm64", "build-macos"])

    def test_linux_and_windows_failures_leave_macos_reported(self):
        results = {**green(), "test-linux": "failed", "build-windows-arm64": "failed",
                   "build-macos": "running"}
        summary = candidate.summarize(results)
        self.assertFalse(summary["ok"])
        self.assertFalse(summary["complete"])
        self.assertEqual(summary["failed"], ["test-linux", "build-windows-arm64"])
        self.assertIn({"id": "build-macos", "status": "running"}, summary["jobs"])
        mac = next(job for job in candidate.job_graph() if job["id"] == "build-macos")
        self.assertTrue(all(results[dependency] == "passed" for dependency in mac["needs"]))
        results["build-macos"] = "passed"
        summary = candidate.summarize(results)
        self.assertTrue(summary["complete"])
        self.assertFalse(summary["ok"])
        self.assertIn({"id": "build-macos", "status": "passed"}, summary["jobs"])

    def test_every_missing_job_is_incomplete(self):
        for job_id in EXPECTED_IDS:
            with self.subTest(job=job_id):
                results = green()
                del results[job_id]
                summary = candidate.summarize(results)
                self.assertFalse(summary["ok"])
                self.assertFalse(summary["complete"])
                self.assertEqual(summary["missing"], [job_id])
        self.assertEqual(candidate.summarize({})["missing"], EXPECTED_IDS)

    def test_only_passed_qualifies(self):
        self.assertTrue(candidate.summarize(green())["ok"])
        for status in ("cancelled", "skipped", "neutral", "infrastructure-failed", "success", None):
            with self.subTest(status=status):
                summary = candidate.summarize({**green(), "license": status})
                self.assertFalse(summary["ok"])
                self.assertEqual(summary["failed"], ["license"])
        for status in ("pending", "queued", "running", "in_progress"):
            summary = candidate.summarize({**green(), "license": status})
            self.assertFalse(summary["complete"])
            self.assertEqual(summary["pending"], ["license"])

    def test_arm64_desktop_consumes_native_arm64_tarball(self):
        desktop = [job for job in candidate.job_graph() if job["kind"] == "desktop"]
        self.assertEqual(len(desktop), 10)
        for job in desktop:
            arch = job["arch"]
            with self.subTest(job=job["id"]):
                self.assertEqual(job["needs"], ["verify", f"build-linux-{arch}"])
                self.assertEqual(job["artifact"], f"htmlclay-linux-{arch}.tar.gz")
                self.assertEqual(job["runner"], "ubuntu-24.04-arm" if arch == "arm64" else "ubuntu-24.04")
                self.assertEqual(job["binary_source"], "extracted-tarball/htmlclay")
                if job["env"]["CHECKS"] == "install-uninstall":
                    self.assertEqual(job["env"]["HTMLCLAY_PACKAGE_DIR"], "$CANDIDATE_DIR")
                    self.assertNotIn("HTMLCLAY_BIN", job["env"])
                else:
                    self.assertEqual(job["env"]["HTMLCLAY_BIN"], "$CANDIDATE_DIR/htmlclay")
                self.assertEqual(job["command"], "bash scripts/linux-checks/run-all.sh")
                self.assertEqual(job["diagnostics_when"], "always")
        jobs = {job["id"]: job for job in candidate.job_graph()}
        self.assertEqual(jobs["build-linux-arm64"]["runner"], "ubuntu-24.04-arm")
        self.assertEqual(jobs["build-linux-amd64"]["runner"], "ubuntu-24.04")

    def test_install_staging_uses_retained_package_and_refuses_missing_installer(self):
        checkout = SCRIPT.parent.parent
        script = (checkout / "scripts/linux-checks/install-uninstall.sh").read_text()
        staging = script.split('source "$(dirname "$0")/lib.sh"\n', 1)[1]
        staging = staging.split('( cd "$HOME" && find . -type f | sort )', 1)[0]
        harness = 'set -euo pipefail\npass() { :; }\nfail() { echo "$*" >&2; exit 1; }\n'
        files = ("htmlclay", "htmlclay.desktop", "htmlclay-mime.xml", "htmlclay.png",
                 "htmlclay.svg", "application-x-htmlclay.png", "application-x-htmlclay.svg",
                 "install-icon.sh", "install.sh", "uninstall.sh")
        for missing in (None, "install.sh"):
            with self.subTest(missing=missing), tempfile.TemporaryDirectory() as directory:
                root = Path(directory)
                package = root / "candidate"
                package.mkdir()
                for name in files:
                    if name != missing:
                        (package / name).write_text(f"retained {name}\n")
                result = subprocess.run(["bash", "-c", harness + staging],
                                        env={**os.environ, "HTMLCLAY_PACKAGE_DIR": str(package),
                                             "LAB": str(root), "REPO": str(checkout),
                                             "BIN": str(package / "htmlclay")},
                                        capture_output=True, text=True)
                if missing:
                    self.assertNotEqual(result.returncode, 0, result.stdout)
                    self.assertIn("candidate package missing install.sh", result.stderr)
                    self.assertFalse((root / "htmlclay-linux.tar.gz").exists())
                else:
                    self.assertEqual(result.returncode, 0, result.stderr)
                    for name in files:
                        self.assertEqual((root / "unpacked" / name).read_bytes(),
                                         (package / name).read_bytes())

    def test_conformance_paths_are_absolute_and_rooted_at_checkout(self):
        job = next(job for job in candidate.job_graph() if job["id"] == "conformance")
        checkout = SCRIPT.parent.parent
        runner = Path(job["env"]["HTMLCLAY_HOST_GATE_RUNNER"])
        cwd = Path(job["env"]["HTMLCLAY_HOST_GATE_CWD"])
        self.assertTrue(runner.is_absolute())
        self.assertTrue(cwd.is_absolute())
        self.assertEqual(runner, checkout / "testdata/conformance/host-test.mjs")
        self.assertEqual(cwd, checkout)
        self.assertTrue(runner.is_file())
        self.assertTrue((runner.parent / "host-test.html").is_file())

    def test_signing_tailwind_and_update_inventory(self):
        jobs = {job["id"]: job for job in candidate.job_graph()}
        mac = jobs["build-macos"]
        self.assertEqual(mac["architectures"], ["amd64", "arm64"])
        self.assertIn("build-sign-notarize", [step["kind"] for step in mac["steps"]])
        self.assertEqual(mac["steps"][-1]["command"], "bash packaging/macos/build.sh --universal")
        for platform in ("linux", "windows"):
            for arch in ("amd64", "arm64"):
                job = jobs[f"build-{platform}-{arch}"]
                self.assertEqual(len(job["artifacts"]), 2)
                self.assertEqual(any(step["kind"] == "test" for step in job["steps"]), arch == "amd64")
                if platform == "windows":
                    self.assertIn("sign", [step["kind"] for step in job["steps"]])
                    self.assertTrue(next(step for step in job["steps"]
                                         if step["kind"] == "verify-signature")["timestamp_required"])
        self.assertIn("one key: fail", jobs["sparkle"]["optional_key_policy"])
        self.assertIn("appcast.xml", jobs["sparkle"]["required_when_enabled"])
        self.assertTrue(jobs["update-feed"]["frozen_date_required"])
        self.assertIn("seed without key id: fail", jobs["update-feed"]["optional_key_policy"])

    def test_identity_contract(self):
        artifact = candidate.identity_artifact(INPUTS)
        self.assertEqual(artifact, {field: INPUTS[field] for field in (
            "workflow_sha", "candidate_sha256", "attempt", "manifest_identity")})
        self.assertEqual(candidate.verify_candidate(artifact, INPUTS), {"ok": True, "problems": []})

    def test_mismatched_identity_refused(self):
        artifact = candidate.identity_artifact(INPUTS)
        for field in candidate.IDENTITY_FIELDS:
            with self.subTest(field=field):
                result = candidate.verify_candidate({**artifact, field: "wrong"}, INPUTS)
                self.assertEqual(result, {"ok": False, "problems": [f"{field}: identity does not match dispatch"]})
        self.assertFalse(candidate.verify_candidate(artifact, {**INPUTS, "app": "other"})["ok"])
        self.assertFalse(candidate.verify_candidate({**INPUTS, "app": "other"}, INPUTS)["ok"])
        self.assertEqual(len(candidate.verify_candidate({}, {})["problems"]), 5)
        for field in candidate.IDENTITY_FIELDS:
            for value in ("", None, 123):
                with self.subTest(field=field, value=value):
                    self.assertFalse(candidate.verify_candidate({**INPUTS, field: value}, INPUTS)["ok"])
        for field in ("workflow_sha", "candidate_sha256", "manifest_identity"):
            with self.assertRaisesRegex(ValueError, "invalid digest"):
                candidate.identity_artifact({**INPUTS, field: "bad"})

    def test_no_upload_or_publication(self):
        for job in candidate.job_graph():
            self.assertNotRegex(json.dumps(job), r"(?i)upload|publish|aws s3|gh release")

    def test_cli(self):
        with tempfile.TemporaryDirectory() as directory:
            source = Path(directory) / "input.json"

            def run(*args):
                return subprocess.run([sys.executable, str(SCRIPT), *map(str, args)],
                                      capture_output=True, text=True)

            self.assertEqual([job["id"] for job in json.loads(run("graph").stdout)], EXPECTED_IDS)
            source.write_text(json.dumps(INPUTS))
            self.assertEqual(run("identity", source, directory).returncode, 0)
            self.assertEqual(json.loads((Path(directory) / "identity.json").read_text()),
                             candidate.identity_artifact(INPUTS))
            source.write_text(json.dumps({"identity": candidate.identity_artifact(INPUTS), "expected": INPUTS}))
            self.assertEqual(run("verify", source).returncode, 0)
            source.write_text(json.dumps({"identity": {}, "expected": INPUTS}))
            self.assertEqual(run("verify", source).returncode, 1)
            source.write_text(json.dumps(green()))
            self.assertEqual(run("summarize", source).returncode, 0)
            source.write_text("{}")
            result = run("summarize", source)
            self.assertEqual(result.returncode, 1)
            self.assertEqual(json.loads(result.stdout)["missing"], EXPECTED_IDS)
            self.assertEqual(run("identity").returncode, 1)


if __name__ == "__main__":
    unittest.main()
