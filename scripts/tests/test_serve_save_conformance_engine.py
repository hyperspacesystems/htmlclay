from pathlib import Path
import unittest


ROOT = Path(__file__).resolve().parents[2]
SCRIPT = ROOT / 'scripts/linux-checks/serve-save.sh'


def logical_lines(source):
    lines = []
    pending = ''
    for line in source.split('\n'):
        if line.rstrip().endswith('\\'):
            pending += line.rstrip()[:-1]
            continue
        lines.append(pending + line)
        pending = ''
    if pending:
        lines.append(pending)
    return lines


class ServeSaveConformanceEngines(unittest.TestCase):
    def setUp(self):
        self.invocations = [line for line in logical_lines(SCRIPT.read_text())
                            if 'scripts/conformance' in line]

    def test_both_branches_run_only_the_page_and_http_engines(self):
        self.assertEqual(len(self.invocations), 2, self.invocations)
        self.assertEqual(
            sum('scripts/conformance-candidate.mjs' in line for line in self.invocations), 1)
        self.assertEqual(
            sum('scripts/conformance.mjs' in line for line in self.invocations), 1)
        for line in self.invocations:
            self.assertIn('--engine page,http ', line, line)


if __name__ == '__main__':
    unittest.main()
