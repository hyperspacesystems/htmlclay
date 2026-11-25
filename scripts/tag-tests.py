#!/usr/bin/env python3
"""Which Go tests a build-tag step in .testbox runs, read from `go list` with and without the tag.

  tag-tests.py run-pattern <tag> <pkg>... [--also <Test>]...
      ^(TestA|TestB)$ for the tests defined only in test files the tag adds, plus each --also name.
      Exit 2 when the tag adds no test, or an --also name is not a test in those packages.
  tag-tests.py skip-pattern
      ^(TestX|TestY)$ for TRIAL_EXEMPT below, the untagged served tests measured not to reach the
      code the managedtrial tag swaps in. Prints nothing when the list is empty, and nothing plus a
      warning when the swapped files changed since the measurement (then every test runs).
  tag-tests.py measure <tag> <pkg> [--only <Test>[,<Test>]...] [--jobs <n>]
      Runs each test the two builds share alone in the tagged build, with coverage, and prints
      whether it reaches swapped code: a covered line in a file the tag swaps in, outside init(),
      or a covered line in a shared file that names a symbol the untagged build declares in a file
      the tag swaps out, or that an init() the tag swaps in sets. Ends with
      the TRIAL_EXEMPT block to paste below. Run it on a box, from the repo root, in the step's
      environment.
"""
import concurrent.futures
import hashlib
import json
import os
import re
import subprocess
import sys
import tempfile
from pathlib import Path

TRIAL_TAG = "managedtrial"
TRIAL_PKG = "./internal/served"
TRIAL_EXEMPT_HASH = "216b59a9fc4d2943"
TRIAL_EXEMPT = [
    "TestCommitAD2StaleLineage",
    "TestConsultAdmitProceed",
    "TestConsultAdmitRejectsBadAnswers",
    "TestConsultArtifactBatches",
    "TestConsultRunawayBatches",
    "TestConsultSettle",
    "TestConsultStructureFolderUnconsulted",
    "TestConsultStructureRefused",
    "TestConsultStructureUnreachable",
    "TestControlArtifactNoService",
    "TestControlCheckpointNoService",
    "TestControlReadSidecarIsStoredRecord",
    "TestControlReadWaitsForTheLock",
    "TestControlRequiresSecret",
    "TestControlRestructureWorkspaceKinds",
    "TestDashboardAPIForward",
    "TestDirSyncUnsupported",
    "TestExpressGolden",
    "TestGateByName",
    "TestGateByPortUnchanged",
    "TestGatesBareAllowed",
    "TestGatesCookiesOnPassingRequests",
    "TestGatesHostHeaderTable",
    "TestGatesHostRunsBeforeOrigin",
    "TestGatesIsOwnOrigin",
    "TestGatesMarkerRewrite",
    "TestGatesOriginGate",
    "TestGatesOriginHelpers",
    "TestGatesRootHandlerCarriesCookiesAndPoweredBy",
    "TestGatesStripSystemRouteMarker",
    "TestGatesSyncDataRefusalCarriesCode",
    "TestGuardGolden",
    "TestHasPageStore",
    "TestListingRender",
    "TestLiveBoundedWriter",
    "TestLiveParseSharedEntry",
    "TestLiveReplayWindow",
    "TestLiveStalledReaderDropped",
    "TestM3RenameMovesOnlyTheFilesHistory",
    "TestM4CallService",
    "TestM4SyncReport",
    "TestManagedLegacyRecoveryComparison",
    "TestManagedMetaExtensionsSorted",
    "TestManagedSheetExtraction",
    "TestNextWalkWaitScalesWithTheWalk",
    "TestPageSplitGolden",
    "TestPageStampGolden",
    "TestPathsConsentWalkBudget",
    "TestPathsConsentWalkSkips",
    "TestPathsDecodeOnceTable",
    "TestPathsValidateSegmentsTable",
    "TestProductFSLoadsInTheHost",
    "TestPublishRecoveredCopy",
    "TestRebaseVersions",
    "TestRespondBytesContentType",
    "TestRespondHeadHasHeadersAndNoBody",
    "TestRespondJSONContentType",
    "TestRespondJSONEscaping",
    "TestRespondNoCacheIsStale",
    "TestRespondNotModifiedStripsEntityHeaders",
    "TestRespondOrderedKeys",
    "TestRespondStatus204HasNoBody",
    "TestRespondWeakETagOfEmptyBody",
    "TestRestructureFolderAndEmptyTrash",
    "TestRestructureRefusesATakenName",
    "TestStorageSidecarPaths",
    "TestUploadNameSplitting",
    "TestUploadReaderAnswers",
    "TestUploadReaderLimit",
    "TestVersionsGolden",
]

TEST_FUNC = re.compile(r"^func (Test\w*)\(\w+ \*testing\.T\)", re.M)
TOP_DECL = re.compile(r"^(?:func (?:\([^)]*\) )?(\w+)|(?:var|const|type) (\w+))")
BLOCK_NAME = re.compile(r"^\t(\w+)\b")
ASSIGNED = re.compile(r"\b(\w+) =[^=]")


def go_list(tag, pkg):
    cmd = ["go", "list", "-e", "-json"] + ([f"-tags={tag}"] if tag else []) + [pkg]
    return json.loads(subprocess.run(cmd, check=True, capture_output=True, text=True).stdout)


def test_files(info):
    return set(info.get("TestGoFiles") or []) | set(info.get("XTestGoFiles") or [])


def tests_in(directory, files):
    names = []
    for name in sorted(files):
        names += TEST_FUNC.findall(Path(directory, name).read_text())
    return names


def builds(tag, pkg):
    return go_list(None, pkg), go_list(tag, pkg)


def pattern(names):
    return "^(" + "|".join(names) + ")$"


def run_pattern(args):
    also = []
    while "--also" in args:
        at = args.index("--also")
        also.append(args[at + 1])
        del args[at:at + 2]
    tag, pkgs = args[0], args[1:]
    if not pkgs:
        sys.exit("usage: tag-tests.py run-pattern <tag> <pkg>... [--also <Test>]...")
    names, every = [], set()
    for pkg in pkgs:
        plain, tagged = builds(tag, pkg)
        names += tests_in(tagged["Dir"], test_files(tagged) - test_files(plain))
        every |= set(tests_in(tagged["Dir"], test_files(tagged)))
    if not names:
        print(f"tag-tests: -tags {tag} adds no test in {' '.join(pkgs)}", file=sys.stderr)
        sys.exit(2)
    missing = [name for name in also if name not in every]
    if missing:
        print(f"tag-tests: no test named {', '.join(missing)} in {' '.join(pkgs)}", file=sys.stderr)
        sys.exit(2)
    print(pattern(sorted(set(names)) + [name for name in also if name not in names]))


def swapped(tag, pkg):
    plain, tagged = builds(tag, pkg)
    before, after = set(plain["GoFiles"]), set(tagged["GoFiles"])
    return plain, tagged, sorted(after - before), sorted(before - after), sorted(before & after)


def swap_hash(tag, pkg):
    # Whether a test reaches swapped code depends on the shared files and the tests' own helpers
    # too, so every Go file of both builds goes into the hash, not only the swapped ones.
    plain, tagged, added, removed, shared = swapped(tag, pkg)
    names = set(added) | set(removed) | set(shared) | test_files(plain) | test_files(tagged)
    names |= set(plain.get("EmbedFiles") or []) | set(tagged.get("EmbedFiles") or [])
    digest = hashlib.sha256()
    for name in sorted(names):
        digest.update(name.encode() + b"\0" + Path(plain["Dir"], name).read_bytes() + b"\0")
    return digest.hexdigest()[:16]


def skip_pattern(args):
    if not TRIAL_EXEMPT:
        return
    measured = swap_hash(TRIAL_TAG, TRIAL_PKG)
    if measured != TRIAL_EXEMPT_HASH:
        print(f"tag-tests: {TRIAL_PKG} changed since TRIAL_EXEMPT was measured "
              f"({TRIAL_EXEMPT_HASH} now {measured}), so step 2 runs every test; measure again", file=sys.stderr)
        return
    print(pattern(TRIAL_EXEMPT))


def init_lines(lines):
    spans, start = [], None
    for number, line in enumerate(lines, 1):
        if start is None and line.startswith("func init()"):
            if line.rstrip().endswith("}"):
                spans.append((number, number))
            else:
                start = number
        elif start is not None and line.startswith("}"):
            spans.append((start, number))
            start = None
    return spans


def declared(lines):
    names, block = set(), False
    for line in lines:
        if line.startswith(("var (", "const (", "type (")):
            block = True
        elif block and line.startswith(")"):
            block = False
        elif block and BLOCK_NAME.match(line):
            names.add(BLOCK_NAME.match(line).group(1))
        match = TOP_DECL.match(line)
        if match:
            names.add(match.group(1) or match.group(2))
    return names


def swap_symbols(directory, added, removed):
    names = set()
    for name in removed:
        names |= declared(Path(directory, name).read_text().splitlines())
    for name in added:
        lines = Path(directory, name).read_text().splitlines()
        for start, end in init_lines(lines):
            for line in lines[start - 1:end]:
                names.update(ASSIGNED.findall(line.split("{", 1)[-1] if line.startswith("func init()") else line))
    names -= {"init", "_"}
    return names


def symbol_lines(directory, files, symbols):
    word = re.compile(r"\b(" + "|".join(sorted(symbols)) + r")\b")
    found = {}
    for name in files:
        for number, line in enumerate(Path(directory, name).read_text().splitlines(), 1):
            code = line.split("//", 1)[0]
            if word.search(code):
                found.setdefault(name, set()).add(number)
    return found


def reach(profile, import_path, added_init, reads):
    for line in profile.read_text().splitlines()[1:]:
        place, _, count = line.rpartition(" ")
        if count == "0":
            continue
        where = place.rsplit(" ", 1)[0]
        path, _, span = where.rpartition(":")
        if path.rsplit("/", 1)[0] != import_path:
            continue
        name = path.rsplit("/", 1)[1]
        first = int(span.split(".", 1)[0])
        last = int(span.split(",", 1)[1].split(".", 1)[0])
        if name in added_init:
            if not any(start <= first <= end for start, end in added_init[name]):
                return f"{name}:{first}"
        elif name in reads:
            hit = sorted(n for n in reads[name] if first <= n <= last)
            if hit:
                return f"{name}:{hit[0]}"
    return None


def measure(args):
    only, jobs = None, os.cpu_count() or 4
    if "--only" in args:
        at = args.index("--only")
        only = set(args[at + 1].split(","))
        del args[at:at + 2]
    if "--jobs" in args:
        at = args.index("--jobs")
        jobs = int(args[at + 1])
        del args[at:at + 2]
    if len(args) != 2:
        sys.exit("usage: tag-tests.py measure <tag> <pkg> [--only <Test>[,<Test>]...] [--jobs <n>]")
    tag, pkg = args
    plain, tagged, added, removed, shared = swapped(tag, pkg)
    directory, import_path = tagged["Dir"], tagged["ImportPath"]
    names = tests_in(directory, test_files(tagged) & test_files(plain))
    if only is not None:
        unknown = only - set(names)
        if unknown:
            sys.exit(f"tag-tests: not a test both builds share: {', '.join(sorted(unknown))}")
        names = [name for name in names if name in only]
    added_init = {name: init_lines(Path(directory, name).read_text().splitlines()) for name in added}
    symbols = swap_symbols(directory, added, removed)
    reads = symbol_lines(directory, shared, symbols)
    print(f"swapped in: {' '.join(added)}; swapped out: {' '.join(removed)}")
    print(f"symbols: {' '.join(sorted(symbols))}")
    print(f"shared lines naming them: {sum(len(v) for v in reads.values())} in {len(reads)} files")
    with tempfile.TemporaryDirectory(prefix="tag-tests-") as work:
        binary = Path(work, "pkg.test")
        subprocess.run(["go", "test", "-c", f"-tags={tag}", "-cover", "-covermode=set", f"-coverpkg={import_path}",
                        "-o", str(binary), pkg], check=True)

        def one(name):
            profile = Path(work, f"{name}.cover")
            run = subprocess.run([str(binary), "-test.run", f"^{name}$", "-test.count=1", "-test.v",
                                  f"-test.coverprofile={profile}"], cwd=directory, capture_output=True, text=True)
            if run.returncode != 0:
                return name, "failed", None
            if re.search(rf"^\s*--- SKIP: {name}(/| )", run.stdout, re.M):
                return name, "skipped", None
            where = reach(profile, import_path, added_init, reads)
            return name, ("reaches" if where else "exempt"), where

        with concurrent.futures.ThreadPoolExecutor(max_workers=jobs) as pool:
            results = sorted(pool.map(one, names))
    for name, verdict, where in results:
        print(f"{verdict:8} {name}" + (f" ({where})" if where else ""))
    counts = {}
    for _, verdict, _ in results:
        counts[verdict] = counts.get(verdict, 0) + 1
    print("total: " + ", ".join(f"{counts[k]} {k}" for k in sorted(counts)))
    exempt = [name for name, verdict, _ in results if verdict == "exempt"]
    print(f'\nTRIAL_EXEMPT_HASH = "{swap_hash(tag, pkg)}"')
    print("TRIAL_EXEMPT = [" + "".join(f'\n    "{name}",' for name in exempt) + ("\n]" if exempt else "]"))


COMMANDS = {"run-pattern": run_pattern, "skip-pattern": skip_pattern, "measure": measure}

if __name__ == "__main__":
    if len(sys.argv) < 2 or sys.argv[1] not in COMMANDS:
        sys.exit(__doc__)
    COMMANDS[sys.argv[1]](sys.argv[2:])
