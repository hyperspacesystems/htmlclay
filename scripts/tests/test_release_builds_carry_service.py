import json
from pathlib import Path
import re
import subprocess
import unittest


ROOT = Path(__file__).resolve().parents[2]
WORKFLOWS = ROOT / '.github/workflows'
RELEASE_WORKFLOWS = ('platform-candidate.yml', 'release.yml')
BOTH_TAGS = '-tags "tailwind_bundle service_bundle"'
TAGS_CHECK = '-tags=tailwind_bundle,service_bundle'


def load_workflow(source):
    try:
        import yaml
    except ImportError:
        return json.loads(subprocess.check_output([
            'ruby', '-ryaml', '-rjson', '-e',
            'puts JSON.generate(YAML.load_file(ARGV[0]))', str(source)], text=True))
    return yaml.safe_load(source.read_text())


def run_text(step):
    return step.get('run') or ''


def release_builds():
    """Every step that builds the Windows or Linux app a release ships."""
    for name in RELEASE_WORKFLOWS:
        jobs = load_workflow(WORKFLOWS / name).get('jobs') or {}
        for job_name, job in jobs.items():
            steps = job.get('steps') or []
            for index, step in enumerate(steps):
                run = run_text(step)
                if 'go build' in run and './cmd/htmlclay' in run and 'main.updateChannel=stable' in run:
                    yield name, job_name, steps[:index], run


class ReleaseBuildsCarryTheService(unittest.TestCase):
    """A 2.0 release with no service inside saves documents but runs no live
    rooms and no sync, and nothing else in the release would notice."""

    def test_every_windows_and_linux_release_build_bundles_the_service(self):
        found = []
        for workflow, job, earlier, run in release_builds():
            where = f'{workflow}:{job}'
            found.append(where)
            self.assertIn(BOTH_TAGS, run, f'{where} builds without the service bundle tag')
            self.assertIn(TAGS_CHECK, run, f'{where} does not check the tags of the binary it built')
            target = 'windows' if 'htmlclay.exe' in run else 'linux'
            pattern = re.compile(r'node service/build\.mjs --goos ' + target + r' --goarch \S+')
            self.assertTrue(any(pattern.search(run_text(step)) for step in earlier),
                            f'{where} builds the app before the {target} service bundle')
            self.assertTrue(any((step.get('uses') or '').startswith('actions/setup-node') for step in earlier),
                            f'{where} builds the service bundle without actions/setup-node')
        # Two workflows, each with Linux and Windows on amd64 and arm64. The
        # release workflow covers both processors with one matrix job each.
        self.assertEqual(sorted(found), [
            'platform-candidate.yml:build-linux-amd64', 'platform-candidate.yml:build-linux-arm64',
            'platform-candidate.yml:build-windows-amd64', 'platform-candidate.yml:build-windows-arm64',
            'release.yml:build-linux', 'release.yml:build-windows'])

    def test_a_build_that_can_run_on_its_runner_is_asked_what_it_carries(self):
        asked = [f'{workflow}:{job}' for workflow, job, _, run in release_builds()
                 if 'update-info' in run and '"service":true' in run]
        self.assertEqual(sorted(asked), [
            'platform-candidate.yml:build-linux-amd64', 'platform-candidate.yml:build-linux-arm64',
            'platform-candidate.yml:build-windows-amd64',
            'release.yml:build-linux', 'release.yml:build-windows'])

    def test_the_packaging_scripts_bundle_the_service_too(self):
        linux = (ROOT / 'packaging/linux/build.sh').read_text()
        windows = (ROOT / 'packaging/windows/build.ps1').read_text()
        for name, text in (('packaging/linux/build.sh', linux), ('packaging/windows/build.ps1', windows)):
            self.assertIn(BOTH_TAGS, text, f'{name} builds without the service bundle tag')
            self.assertIn('node service/build.mjs', text, f'{name} never builds the service bundle')
            self.assertIn('update-info', text, f'{name} never asks the binary what it carries')


if __name__ == '__main__':
    unittest.main()
