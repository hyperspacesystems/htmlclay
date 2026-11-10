import json
from pathlib import Path
import subprocess
import unittest


ROOT = Path(__file__).resolve().parents[2]
WORKFLOWS = ROOT / '.github/workflows'
GO_TESTS = ('ci.yml', 'platform-candidate.yml', 'release.yml')


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


class GoTestJobsInstallNodePackages(unittest.TestCase):
    def test_every_go_test_job_installs_the_locked_packages_first(self):
        found = []
        for source in sorted(WORKFLOWS.glob('*.yml')) + sorted(WORKFLOWS.glob('*.yaml')):
            jobs = load_workflow(source).get('jobs') or {}
            for name, job in jobs.items():
                steps = job.get('steps') or []
                for index, step in enumerate(steps):
                    if 'go test' not in run_text(step):
                        continue
                    found.append((source.name, name))
                    earlier = steps[:index]
                    self.assertTrue(
                        any('npm ci' in run_text(step) for step in earlier),
                        f'{source.name}:{name} runs go test without an earlier npm ci')
                    self.assertTrue(
                        any((step.get('uses') or '').startswith('actions/setup-node')
                            for step in earlier),
                        f'{source.name}:{name} runs go test without an earlier actions/setup-node')
        self.assertTrue(found, 'no workflow job runs go test')
        names = {name for name, _ in found}
        for expected in GO_TESTS:
            self.assertIn(expected, names, f'{expected} runs no Go tests')


if __name__ == '__main__':
    unittest.main()
