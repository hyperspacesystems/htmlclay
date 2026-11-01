#!/usr/bin/env python3
"""Private candidate transport (Python 3.12+). No public release operations.

The gzip archive contains the prepared app at its root, candidate.json (the
captured manifest), and platform-candidate.json with manifest (PlatformManifest),
source_sha (the captured source revision), and payload_date (frozen RFC3339 UTC).
The metadata and both manifests must agree with the dispatch before identity is
written. The source revision is provenance; the archive is the build input.
"""

import datetime
import hashlib
import json
import os
from pathlib import Path
import re
import subprocess
import sys
import tarfile
import tempfile

from release_candidate import identity_artifact, job_graph


BUCKET = "hyperclay-candidates"


def digest(path):
    checksum = hashlib.sha256()
    with path.open("rb") as stream:
        for block in iter(lambda: stream.read(1024 * 1024), b""):
            checksum.update(block)
    return checksum.hexdigest()


def aws(*args):
    command = ["aws", "--endpoint-url",
               "https://" + os.environ["R2_ACCOUNT_ID"] + ".r2.cloudflarestorage.com",
               "--region", "auto", "s3api", *args, "--bucket", BUCKET, "--output", "json"]
    return json.loads(subprocess.check_output(command, text=True) or "{}")


def extract(archive, destination):
    with tarfile.open(archive, "r:gz") as bundle:
        bundle.extractall(destination, filter="data")


def validate(archive, head, dispatch, destination):
    metadata = head.get("Metadata", {})
    if digest(archive) != metadata.get("sha256"):
        raise ValueError("candidate archive sha256 does not match R2 metadata")
    if metadata.get("manifest-identity") != dispatch["manifest_identity"]:
        raise ValueError("candidate R2 manifest identity does not match dispatch")
    extract(archive, destination)
    if digest(destination / "candidate.json") != dispatch["candidate_sha256"]:
        raise ValueError("candidate manifest sha256 does not match dispatch")
    record = json.loads((destination / "platform-candidate.json").read_text())
    manifest = record["manifest"]
    identity = hashlib.sha256(json.dumps(manifest, sort_keys=True,
                                        separators=(",", ":")).encode()).hexdigest()
    if identity != dispatch["manifest_identity"]:
        raise ValueError("platform manifest identity does not match dispatch")
    for field in ("app", "attempt", "candidate_sha256", "workflow_sha"):
        if manifest[field] != dispatch[field]:
            raise ValueError(field + " does not match dispatch")
    if manifest["checks"] != [job["id"] for job in job_graph()]:
        raise ValueError("platform manifest checks do not match workflow graph")
    if not re.fullmatch(r"[0-9]+\.[0-9]+\.[0-9]+", manifest["version"]):
        raise ValueError("version must be X.Y.Z")
    if not re.fullmatch(r"[0-9a-f]{40}", record["source_sha"]):
        raise ValueError("source_sha must be a full commit id")
    date = record["payload_date"]
    if not re.fullmatch(r"\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}Z", date):
        raise ValueError("payload_date must be frozen UTC RFC3339")
    datetime.datetime.strptime(date, "%Y-%m-%dT%H:%M:%SZ")
    if not (destination / "go.mod").is_file():
        raise ValueError("prepared application must be at archive root")
    return {"version": manifest["version"], "source_sha": record["source_sha"],
            "payload_date": date}


def verify():
    dispatch = {field: os.environ[field.upper()] for field in (
        "app", "attempt", "candidate_sha256", "workflow_sha", "manifest_identity")}
    identity = identity_artifact(dispatch)
    key = os.environ["CANDIDATE_KEY"]
    if "/" not in key or not key.endswith(".tar.gz"):
        raise ValueError("candidate key must name a tar.gz under its attempt prefix")
    archive = Path("candidate.tar.gz")
    head = aws("get-object", "--key", key, str(archive))
    with tempfile.TemporaryDirectory() as directory:
        outputs = validate(archive, head, dispatch, Path(directory))
    Path("identity").mkdir(exist_ok=True)
    Path("identity/identity.json").write_text(json.dumps(identity, indent=2) + "\n")
    with open(os.environ["GITHUB_OUTPUT"], "a") as stream:
        for name, value in outputs.items():
            stream.write(f"{name}={value}\n")


def retain(path, name):
    path = Path(path)
    if not path.is_file() or path.stat().st_size == 0 or Path(name).name != name:
        raise ValueError("expected exactly one nonempty output file")
    key = os.environ["CANDIDATE_KEY"].rsplit("/", 1)[0] + "/out/" + name
    sha256 = digest(path)
    aws("put-object", "--key", key, "--body", str(path), "--metadata", "sha256=" + sha256)
    with tempfile.TemporaryDirectory() as directory:
        retained = Path(directory) / "retained"
        head = aws("get-object", "--key", key, str(retained))
        if head.get("Metadata", {}).get("sha256") != sha256 or digest(retained) != sha256:
            raise ValueError("retained output hash mismatch")


def freeze_sparkle():
    path = Path("packaging/macos/sparkle-appcast.sh")
    source = path.read_text()
    before = '''PUB_DATE="$(LC_ALL=C date -u '+%a, %d %b %Y %H:%M:%S +0000')"'''
    if source.count(before) != 1:
        raise ValueError("Sparkle date assignment changed; cannot freeze it")
    date = datetime.datetime.strptime(os.environ["PAYLOAD_DATE"], "%Y-%m-%dT%H:%M:%SZ")
    frozen = date.strftime("%a, %d %b %Y %H:%M:%S +0000")
    path.write_text(source.replace(before, 'PUB_DATE="' + frozen + '"'))


if __name__ == "__main__":
    command, *args = sys.argv[1:]
    if command == "verify":
        verify()
    elif command == "retain":
        retain(*args)
    elif command == "extract":
        extract(Path(args[0]), Path(args[1]))
    elif command == "freeze-sparkle":
        freeze_sparkle()
    else:
        raise ValueError("unknown command")
