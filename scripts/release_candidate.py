#!/usr/bin/env python3
"""Describe and validate publication-disabled HTML Clay candidates."""

import json
from pathlib import Path
import re
import sys


IDENTITY_FIELDS = ("workflow_sha", "candidate_sha256", "attempt", "manifest_identity")
LINUX_CHECKS = ("install-uninstall", "serve-save", "tray-nobus", "login-item", "prompts")


def job_graph():
    checkout = Path(__file__).resolve().parents[1]
    jobs = [{"id": "verify", "kind": "verify", "needs": []}]
    for platform, runner in (("macos", "macos-latest"), ("linux", "ubuntu-latest"),
                             ("windows", "windows-latest")):
        jobs.append({"id": f"test-{platform}", "kind": "test", "runner": runner,
                     "command": "go test ./... -count=1", "needs": ["verify"]})
    jobs.extend([
        {"id": "license", "kind": "test", "runner": "ubuntu-latest",
         "command": "python3 scripts/ablation-check.py LICENSE", "needs": ["verify"]},
        {"id": "conformance", "kind": "test", "runner": "ubuntu-latest",
         "command": "go test ./internal/server -run TestMalleableHTMLFileHostGate -count=1 -v",
         "env": {"HTMLCLAY_HOST_GATE": "1",
                 "HTMLCLAY_HOST_GATE_RUNNER": str(checkout / "testdata/conformance/host-test.mjs"),
                 "HTMLCLAY_HOST_GATE_CWD": str(checkout)}, "needs": ["verify"]},
    ])
    for platform in ("linux", "windows"):
        for arch in ("amd64", "arm64"):
            steps = [{"kind": "prepare", "name": "Fetch Tailwind",
                      "command": f"go run ./cmd/fetchtailwind -goos {platform} -goarch {arch}"}]
            if platform == "windows":
                steps.insert(0, {"kind": "prepare", "name": "Embed icon and version resources"})
            if arch == "amd64":
                steps.append({"kind": "test", "name": "Test the Tailwind bundle",
                              "command": "go test -tags tailwind_bundle ./internal/tailwind/ -count=1"})
            steps.append({"kind": "build", "name": "Build", "tags": ["tailwind_bundle"]})
            if platform == "windows":
                steps.extend([
                    {"kind": "sign", "name": "Sign with Azure Trusted Signing"},
                    {"kind": "verify-signature", "name": "Verify signature",
                     "timestamp_required": True},
                ])
            steps.extend([{"kind": "package", "name": "Package"},
                          {"kind": "package", "name": "Update artifact"}])
            installer = f"htmlclay-{platform}-{arch}." + ("tar.gz" if platform == "linux" else "zip")
            update = f"htmlclay-{{version}}-{platform}-{arch}." + ("gz" if platform == "linux" else "exe")
            jobs.append({"id": f"build-{platform}-{arch}", "kind": "build",
                         "platform": f"{platform}-{arch}", "arch": arch,
                         "runner": ("ubuntu-24.04-arm" if arch == "arm64" else "ubuntu-24.04")
                         if platform == "linux" else "windows-latest",
                         "needs": ["verify"], "steps": steps,
                         "artifacts": [installer, update]})
    jobs.append({"id": "build-macos", "kind": "build", "platform": "macos",
                 "architectures": ["amd64", "arm64"], "runner": "macos-14",
                 "needs": ["verify"], "steps": [
                     {"kind": "test", "name": "Test the Tailwind bundle",
                      "command": "go run ./cmd/fetchtailwind -goos darwin -goarch arm64\n"
                                 "go test -tags tailwind_bundle ./internal/tailwind/ -count=1"},
                     {"kind": "build-sign-notarize", "name": "Build, sign & notarize (universal)",
                      "command": "bash packaging/macos/build.sh --universal"}],
                 "artifacts": ["HTMLClay-{version}-universal.dmg",
                               "HTMLClay-{version}-universal.app.zip"]})
    jobs.append({"id": "sparkle", "kind": "prepare", "needs": ["verify", "build-macos"],
                 "command": 'bash packaging/macos/sparkle-appcast.sh "$VERSION" '
                            '"HTMLClay-${VERSION}-universal.app.zip" sparkle-out',
                 "optional_key_policy": "neither key: SKIPPED; one key: fail; both keys: sign",
                 "artifacts": ["sparkle-out/"],
                 "required_when_enabled": ["HTMLClay-{version}-universal.app.zip", "appcast.xml"]})
    jobs.append({"id": "update-feed", "kind": "prepare",
                 "needs": ["verify", "build-linux-amd64", "build-linux-arm64",
                           "build-windows-amd64", "build-windows-arm64"],
                 "frozen_date_required": True,
                 "optional_key_policy": "no seed: skip signed feed; seed without key id: fail",
                 "steps": [{"kind": "prepare", "name": "Build the update feed payload",
                            "command": 'go run ./cmd/mkfeed payload -version "$VERSION" '
                                       '-source-sha "$SOURCE_SHA" -date "$PAYLOAD_DATE" updates > update-payload.json'},
                           {"kind": "sign", "name": "Sign the update feed",
                            "command": 'go run ./cmd/signfeed sign -key-id "$HTMLCLAY_UPDATE_KEY_ID" '
                                       'update-payload.json > stable.json'},
                           {"kind": "verify-signature", "name": "Verify update feed",
                            "command": 'go run ./cmd/mkfeed verify -version "$VERSION" stable.json'}],
                 "artifacts": ["update-payload.json", "stable.json or SKIPPED"]})
    jobs.append({"id": "checksums", "kind": "prepare",
                 "needs": ["verify", "build-linux-amd64", "build-linux-arm64",
                           "build-windows-amd64", "build-windows-arm64", "build-macos"],
                 "command": "(cd artifacts && sha256sum *) > SHA256SUMS",
                 "artifacts": ["SHA256SUMS"]})
    for arch, runner in (("amd64", "ubuntu-24.04"), ("arm64", "ubuntu-24.04-arm")):
        for check in LINUX_CHECKS:
            jobs.append({"id": f"linux-{arch}-{check}", "kind": "desktop",
                         "platform": f"linux-{arch}", "arch": arch, "runner": runner,
                         "needs": ["verify", f"build-linux-{arch}"],
                         "artifact": f"htmlclay-linux-{arch}.tar.gz",
                         "binary_source": "extracted-tarball/htmlclay",
                         "env": {("HTMLCLAY_PACKAGE_DIR" if check == "install-uninstall" else "HTMLCLAY_BIN"):
                                 "$CANDIDATE_DIR" if check == "install-uninstall" else "$CANDIDATE_DIR/htmlclay",
                                 "CHECKS": check},
                         "command": "bash scripts/linux-checks/run-all.sh",
                         "diagnostics": "linux-check-artifacts/", "diagnostics_when": "always"})
    return jobs


def verify_candidate(identity, expected):
    identity = identity or {}
    expected = expected or {}
    problems = []
    if expected.get("app") != "htmlclay" or identity.get("app", "htmlclay") != expected.get("app"):
        problems.append("app: expected htmlclay")
    for field in IDENTITY_FIELDS:
        value = identity.get(field)
        other = expected.get(field)
        if not isinstance(value, str) or not value or not isinstance(other, str) or not other:
            problems.append(f"{field}: missing identity or expected value")
        elif value != other:
            problems.append(f"{field}: identity does not match dispatch")
        elif field != "attempt" and not re.fullmatch(
                r"[a-f0-9]{40}" if field == "workflow_sha" else r"[a-f0-9]{64}", value):
            problems.append(f"{field}: invalid digest")
    return {"ok": not problems, "problems": problems}


def identity_artifact(inputs):
    result = verify_candidate(inputs, inputs)
    if not result["ok"]:
        raise ValueError("\n".join(result["problems"]))
    return {field: inputs[field] for field in IDENTITY_FIELDS}


def summarize(results):
    results = results or {}
    jobs = [{"id": job["id"], "status": results.get(job["id"], "missing")} for job in job_graph()]
    missing = [job["id"] for job in jobs if job["status"] == "missing"]
    pending = [job["id"] for job in jobs if job["status"] in ("pending", "queued", "running", "in_progress")]
    failed = [job["id"] for job in jobs if job["status"] != "passed"
              and job["id"] not in missing and job["id"] not in pending]
    problems = [f'{job["id"]}: {job["status"]}' for job in jobs if job["status"] != "passed"]
    return {"ok": not problems, "complete": not missing and not pending,
            "jobs": jobs, "missing": missing, "pending": pending, "failed": failed, "problems": problems}


def main(argv):
    counts = {"graph": 1, "verify": 2, "summarize": 2, "identity": 3}
    if not argv or len(argv) != counts.get(argv[0]):
        raise ValueError("use graph, verify <json>, summarize <results.json>, or identity <inputs.json> <directory>")
    command = argv[0]
    inputs = json.loads(Path(argv[1]).read_text()) if len(argv) > 1 else None
    if command == "identity":
        artifact = identity_artifact(inputs)
        directory = Path(argv[2])
        directory.mkdir(parents=True, exist_ok=True)
        (directory / "identity.json").write_text(json.dumps(artifact, indent=2) + "\n")
        return 0
    result = (job_graph() if command == "graph" else
              verify_candidate(inputs.get("identity"), inputs.get("expected")) if command == "verify" else
              summarize(inputs))
    print(json.dumps(result, indent=2))
    return int(isinstance(result, dict) and not result["ok"])


if __name__ == "__main__":
    try:
        sys.exit(main(sys.argv[1:]))
    except (ValueError, OSError, TypeError, AttributeError) as error:
        print(error, file=sys.stderr)
        sys.exit(1)
