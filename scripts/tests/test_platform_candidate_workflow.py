import copy
import hashlib
import io
import json
import os
from pathlib import Path
import subprocess
import sys
import tarfile
import tempfile
import unittest
from unittest.mock import patch


ROOT = Path(__file__).resolve().parents[2]
sys.path.insert(0, str(ROOT / 'scripts'))
import platform_candidate_io as transport
import release_candidate as core


def load_workflow():
    source = ROOT / '.github/workflows/platform-candidate.yml'
    try:
        import yaml
    except ImportError:
        return json.loads(subprocess.check_output([
            'ruby', '-ryaml', '-rjson', '-e',
            'puts JSON.generate(YAML.load_file(ARGV[0]))', str(source)], text=True))
    return yaml.safe_load(source.read_text())


class PlatformWorkflowTests(unittest.TestCase):
    def setUp(self):
        self.workflow = load_workflow()
        self.jobs = self.workflow['jobs']

    def test_exact_graph_and_independent_jobs(self):
        self.assertEqual(list(self.jobs), [job['id'] for job in core.job_graph()])
        for expected in core.job_graph():
            job = self.jobs[expected['id']]
            self.assertEqual(job['name'], expected['id'])
            self.assertEqual(job.get('needs', []), expected['needs'])
            if 'runner' in expected:
                self.assertEqual(job['runs-on'], expected['runner'])
            if 'strategy' in job:
                self.assertIs(job['strategy'].get('fail-fast'), False)
        self.assertNotIn('concurrency', self.workflow)

    def test_dispatch_contract(self):
        self.assertEqual(self.workflow['run-name'], 'platform ${{ inputs.attempt }}')
        self.assertEqual(set(self.workflow['on']), {'workflow_dispatch'})
        inputs = self.workflow['on']['workflow_dispatch']['inputs']
        self.assertEqual(set(inputs), {'attempt', 'manifest_identity', 'candidate_sha256', 'app', 'candidate_key'})
        for value in inputs.values():
            self.assertEqual(value['type'], 'string')
            self.assertIs(value['required'], True)
        self.assertEqual(self.workflow['permissions'], {'contents': 'read'})
        self.assertEqual(self.workflow['env']['WORKFLOW_SHA'], '${{ github.workflow_sha }}')
        uploads = [s['with'] for s in self.jobs['verify']['steps']
                   if s.get('uses') == 'actions/upload-artifact@v4']
        self.assertIn({'name': 'identity', 'path': 'identity/identity.json', 'if-no-files-found': 'error'}, uploads)

    def test_candidate_is_every_jobs_source(self):
        for name, job in self.jobs.items():
            checkouts = [s for s in job['steps'] if s.get('uses') == 'actions/checkout@v4']
            self.assertEqual(len(checkouts), 1)
            self.assertEqual(checkouts[0]['with'], {'ref': '${{ github.workflow_sha }}', 'path': 'trusted'})
            if name == 'verify':
                continue
            self.assertEqual(job['defaults']['run']['working-directory'], 'candidate')
            download = next(s for s in job['steps'] if s.get('name') == 'Download private candidate')
            self.assertIn('download transfer "candidate.tar.gz" "candidate-input/candidate.tar.gz"', download['run'])
            self.assertEqual(download['working-directory'], '.')
            self.assertTrue(any(s.get('name') == 'Install private storage CLI' for s in job['steps']))
            extract = next(s for s in job['steps'] if s.get('name') == 'Extract verified candidate')
            self.assertIn('platform_candidate_io.py extract candidate-input/candidate.tar.gz candidate', extract['run'])
            self.assertEqual(extract['working-directory'], '.')
        conformance = next(s for s in self.jobs['conformance']['steps'] if s.get('name') == 'Conformance gate')
        self.assertEqual(conformance['run'], 'node scripts/conformance-candidate.mjs --platform linux')
        self.assertNotIn('env', conformance)
        evidence = [s for s in self.jobs['conformance']['steps']
                    if s.get('uses') == 'actions/upload-artifact@v4' and s['with']['name'].startswith('conformance-')]
        self.assertEqual([step['with'] for step in evidence], [{
            'name': 'conformance-linux',
            'path': 'candidate/.conformance/conformance-linux.json',
            'if-no-files-found': 'error',
        }])
        self.assertEqual([step['if'] for step in evidence], ['always()'])
        driver = next(s for s in self.jobs['conformance']['steps'] if s.get('name') == 'Install the browser driver')
        self.assertEqual(driver['run'].splitlines(),
                         ['npm ci', 'npx --no-install playwright install --with-deps chromium'])
        for graph in core.job_graph():
            if graph['kind'] != 'desktop':
                continue
            job = self.jobs[graph['id']]
            check = next(s for s in job['steps'] if s.get('name') == 'Linux check')
            self.assertEqual(check['env']['HTMLCLAY_PACKAGE_DIR'], '${{ github.workspace }}/retained/package')
            self.assertEqual(check['env']['HTMLCLAY_BIN'], '${{ github.workspace }}/retained/package/htmlclay')
            self.assertEqual(check['env']['CHECKS'], graph['env']['CHECKS'])
            self.assertTrue(any('download out "' + graph['artifact'] + '"' in s.get('run', '') for s in job['steps']))
            evidence = [s for s in job['steps']
                        if s.get('uses') == 'actions/upload-artifact@v4'
                        and s['with']['name'].startswith('conformance-')]
            if graph['env']['CHECKS'] == 'serve-save':
                self.assertEqual(check['env']['CONFORMANCE_PLATFORM'], 'linux-' + graph['arch'] + '-artifact')
                self.assertEqual([step['with'] for step in evidence], [{
                    'name': graph['artifacts'][0],
                    'path': 'candidate/.conformance/' + graph['artifacts'][0] + '.json',
                    'if-no-files-found': 'error',
                }])
                self.assertEqual([step['if'] for step in evidence], ['always()'])
            else:
                self.assertEqual(evidence, [])
            diagnostics = job['steps'][-1]
            self.assertEqual(diagnostics['if'], 'always()')

    def test_no_publication_and_private_retention_of_every_output(self):
        text = json.dumps(self.workflow)
        self.assertNotRegex(text, r'(?i)gh release|softprops/action-gh-release|git push|R2_BUCKET|R2_ACCESS_KEY|cancel-in-progress')
        self.assertNotIn('${{ inputs.version }}', text)
        self.assertNotIn('${{ matrix.', text)
        retained = []
        uploads = []
        for name, job in self.jobs.items():
            for step in job['steps']:
                uses = step.get('uses', '')
                self.assertFalse(uses.startswith('actions/download-artifact@'))
                if uses.startswith('actions/upload-artifact@'):
                    artifact = step['with']
                    uploads.append(artifact)
                    self.assertEqual(artifact['if-no-files-found'], 'error')
                    allowed = ('identity/identity.json' if artifact['name'] == 'identity'
                               else 'desktop-diagnostics.tar.gz' if artifact['name'] == name + '-diagnostics'
                               and name.startswith('linux-')
                               else 'candidate/.conformance/' + artifact['name'] + '.json'
                               if artifact['name'].startswith('conformance-') else None)
                    self.assertEqual(artifact['path'], allowed,
                                     'GitHub artifacts must contain only identity or check diagnostics')
                if step.get('name', '').startswith('Retain '):
                    retained.append(step['name'][7:])
                    self.assertIn('platform_candidate_io.py" retain ', step['run'])
                if 'AWS_ACCESS_KEY_ID' in step.get('env', {}):
                    self.assertTrue(step['name'].startswith(('Retain ', 'Download ', 'Transfer ')))
        self.assertEqual(len(uploads), 14)
        contract = json.loads(subprocess.check_output(
            [sys.executable, str(ROOT / 'scripts/release_candidate.py'), 'contract'], text=True))
        self.assertEqual(list(self.jobs), contract['checks'])
        evidence = ['conformance-linux', 'conformance-linux-amd64-artifact', 'conformance-linux-arm64-artifact']
        self.assertEqual([name for name in contract['artifacts'] if name.startswith('conformance-')], evidence)
        self.assertEqual(sorted(artifact['name'] for artifact in uploads
                                if artifact['name'].startswith('conformance-')), sorted(evidence))
        self.assertCountEqual([name.replace('${{ needs.verify.outputs.version }}', '{version}') for name in retained],
                              [name for name in contract['artifacts']
                               if name != 'identity' and name not in evidence])
        source = (ROOT / 'scripts/platform_candidate_io.py').read_text()
        self.assertIn('BUCKET = "hyperclay-candidates"', source)
        self.assertNotIn('s3://', source)

    def test_signature_and_feed_requirements_preserved(self):
        for arch in ('amd64', 'arm64'):
            steps = {s.get('name'): s for s in self.jobs['build-windows-' + arch]['steps']}
            self.assertIn('Invoke-TrustedSigning', steps['Sign with Azure Trusted Signing']['run'])
            self.assertIn('TimeStamperCertificate', steps['Verify signature']['run'])
            self.assertIn('-tags tailwind_bundle', steps['Build']['run'])
            self.assertIn(arch, steps['Build']['run'])
        mac = {s.get('name'): s for s in self.jobs['build-macos']['steps']}
        self.assertEqual(mac['Build, sign & notarize (universal)']['run'], 'bash packaging/macos/build.sh --universal')
        self.assertIn('SPARKLE_PUBLIC_ED_KEY', mac['Build, sign & notarize (universal)']['env'])
        sparkle = next(s for s in self.jobs['sparkle']['steps'] if s.get('name') == 'Sign the Sparkle update')
        self.assertIn('touch sparkle-out/SKIPPED', sparkle['run'])
        self.assertIn('exit 1', sparkle['run'])
        feed = next(s for s in self.jobs['update-feed']['steps'] if s.get('name') == 'Prepare and sign private update feed')
        self.assertEqual(feed['env']['PAYLOAD_DATE'], '${{ needs.verify.outputs.payload_date }}')
        self.assertIn('-date "$PAYLOAD_DATE"', feed['run'])
        self.assertIn('test -n "$HTMLCLAY_UPDATE_KEY_ID"', feed['run'])
        self.assertIn('go run ./cmd/mkfeed verify', feed['run'])


class CandidateTransportTests(unittest.TestCase):
    def fixture(self, directory, mutate=None):
        root = Path(directory)
        tree = root / 'source'
        tree.mkdir()
        candidate = b'{"schema":1}\n'
        sha = hashlib.sha256(candidate).hexdigest()
        manifest = {'app': 'htmlclay', 'candidate_sha256': sha, 'prepared_tree_sha256': 'b' * 64,
                    'version': '1.2.3', 'workflow_sha': 'c' * 40,
                    'checks': [j['id'] for j in core.job_graph()], 'artifacts': ['SHA256SUMS'],
                    'attempt': 'attempt-fixture'}
        record = {'manifest': manifest, 'source_sha': 'd' * 40, 'payload_date': '2026-11-01T10:00:00Z'}
        if mutate:
            mutate(record)
        identity = hashlib.sha256(json.dumps(manifest, sort_keys=True, separators=(',', ':')).encode()).hexdigest()
        (tree / 'candidate.json').write_bytes(candidate)
        (tree / 'platform-candidate.json').write_text(json.dumps(record))
        (tree / 'go.mod').write_text('module fixture\n')
        archive = root / 'candidate.tar.gz'
        with tarfile.open(archive, 'w:gz') as bundle:
            for source in tree.iterdir():
                bundle.add(source, arcname=source.name)
        dispatch = {k: manifest[k] for k in ('app', 'attempt', 'candidate_sha256', 'workflow_sha')}
        dispatch['manifest_identity'] = identity
        head = {'ContentLength': archive.stat().st_size,
                'Metadata': {'sha256': transport.digest(archive), 'manifest-identity': identity}}
        return archive, head, dispatch

    def test_validated_archive_and_dispatch_mismatches(self):
        with tempfile.TemporaryDirectory() as directory:
            archive, head, dispatch = self.fixture(directory)
            output = Path(directory) / 'output'
            values = transport.validate(archive, head, dispatch, output)
            self.assertEqual(values, {'version': '1.2.3', 'source_sha': 'd' * 40, 'payload_date': '2026-11-01T10:00:00Z'})
            self.assertTrue((output / 'go.mod').is_file())
            for field in ('app', 'attempt', 'candidate_sha256', 'workflow_sha', 'manifest_identity'):
                with self.subTest(field=field), self.assertRaises(ValueError):
                    transport.validate(archive, head, dict(dispatch, **{field: 'wrong'}), output)
            with self.assertRaisesRegex(ValueError, 'ContentLength'):
                transport.validate(archive, dict(head, ContentLength=1), dispatch, output)
            bad = copy.deepcopy(head)
            bad['Metadata']['sha256'] = '0' * 64
            with self.assertRaisesRegex(ValueError, 'archive sha256'):
                transport.validate(archive, bad, dispatch, output)

    def test_verify_writes_identity_only_after_integrity_checks(self):
        for valid in (True, False):
            with self.subTest(valid=valid), tempfile.TemporaryDirectory() as directory:
                archive, head, dispatch = self.fixture(directory)
                contents = archive.read_bytes()
                root = Path(directory) / 'workspace'
                root.mkdir()
                output = root / 'outputs'
                env = {key.upper(): value for key, value in dispatch.items()}
                env.update(CANDIDATE_KEY='candidates/attempt/candidate.tar.gz', GITHUB_OUTPUT=str(output))
                def aws(*args):
                    Path(args[-1]).write_bytes(contents if valid else b'corrupt')
                    return head
                previous = Path.cwd()
                try:
                    os.chdir(root)
                    with patch.dict(os.environ, env), patch.object(transport, 'aws', aws):
                        if valid:
                            transport.verify()
                        else:
                            with self.assertRaisesRegex(ValueError, 'archive sha256'):
                                transport.verify()
                finally:
                    os.chdir(previous)
                if valid:
                    identity = json.loads((root / 'identity/identity.json').read_text())
                    self.assertEqual(identity, core.identity_artifact(dispatch))
                    self.assertIn('version=1.2.3\n', output.read_text())
                else:
                    self.assertFalse((root / 'identity/identity.json').exists())
                    self.assertFalse(output.exists())

    def test_invalid_version_date_or_graph_refused(self):
        mutations = [lambda r: r['manifest'].update(version='1.2.3; echo bad'),
                     lambda r: r.update(payload_date='not-a-date'),
                     lambda r: r['manifest']['checks'].pop()]
        for mutate in mutations:
            with self.subTest(mutate=mutate), tempfile.TemporaryDirectory() as directory:
                archive, head, dispatch = self.fixture(directory, mutate)
                with self.assertRaises(ValueError):
                    transport.validate(archive, head, dispatch, Path(directory) / 'out')

    def test_archive_cannot_write_outside_candidate(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            archive = root / 'bad.tar.gz'
            with tarfile.open(archive, 'w:gz') as bundle:
                entry = tarfile.TarInfo('../escaped')
                entry.size = 3
                bundle.addfile(entry, io.BytesIO(b'bad'))
            with self.assertRaises(tarfile.FilterError):
                transport.extract(archive, root / 'out')
            self.assertFalse((root / 'escaped').exists())

    def test_retain_private_exact_bytes_and_reject_corruption(self):
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / 'signed.exe'
            path.write_bytes(b'signed timestamped fixture')
            calls = []
            def aws(*args):
                calls.append(args)
                if args[0] == 'get-object':
                    Path(args[-1]).write_bytes(path.read_bytes())
                    return {'ContentLength': path.stat().st_size, 'Metadata': {'sha256': transport.digest(path)}}
                return {}
            with patch.dict(os.environ, APP='htmlclay', CANDIDATE_SHA256='a' * 64, ATTEMPT='attempt'), patch.object(transport, 'aws', aws):
                transport.retain(path, 'signed.exe')
                self.assertEqual(calls[0][2], 'candidates/htmlclay/' + 'a' * 64 + '/attempt/out/signed.exe')
                self.assertEqual(calls[0][-1], 'sha256=' + transport.digest(path))
            def corrupt(*args):
                if args[0] == 'get-object':
                    Path(args[-1]).write_bytes(b'corrupt')
                return {}
            with patch.dict(os.environ, APP='htmlclay', CANDIDATE_SHA256='a' * 64, ATTEMPT='attempt'), patch.object(transport, 'aws', corrupt):
                with self.assertRaisesRegex(ValueError, 'hash mismatch'):
                    transport.retain(path, 'signed.exe')

    def test_transfer_and_download_require_digest_and_length(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            source = root / 'candidate.tar.gz'
            source.write_bytes(b'private source fixture')
            objects = {}
            def aws(*args):
                key = args[2]
                if args[0] == 'put-object':
                    objects[key] = (Path(args[4]).read_bytes(), args[6][7:])
                    return {}
                contents, checksum = objects[key]
                Path(args[-1]).write_bytes(contents)
                return {'Metadata': {'sha256': checksum}, 'ContentLength': len(contents)}
            env = dict(APP='htmlclay', CANDIDATE_SHA256='a' * 64, ATTEMPT='fixture')
            with patch.dict(os.environ, env), patch.object(transport, 'aws', aws):
                transport.transfer(source, source.name)
                key = 'candidates/htmlclay/' + 'a' * 64 + '/fixture/transfer/candidate.tar.gz'
                self.assertEqual(objects[key][0], source.read_bytes())
                target = root / 'input/candidate.tar.gz'
                transport.download('transfer', source.name, target)
                self.assertEqual(target.read_bytes(), source.read_bytes())
                for head in ({'Metadata': {}, 'ContentLength': source.stat().st_size},
                             {'Metadata': {'sha256': transport.digest(source)}, 'ContentLength': 1}):
                    with self.subTest(head=head), patch.object(transport, 'aws', return_value=head):
                        with self.assertRaises(ValueError):
                            transport.download('transfer', source.name, target)

    def test_sparkle_uses_frozen_date_without_changing_checkout(self):
        original = ROOT / 'packaging/macos/sparkle-appcast.sh'
        contents = original.read_bytes()
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            target = root / 'packaging/macos/sparkle-appcast.sh'
            target.parent.mkdir(parents=True)
            target.write_bytes(contents)
            previous = Path.cwd()
            try:
                os.chdir(root)
                with patch.dict(os.environ, PAYLOAD_DATE='2026-11-01T10:00:00Z'):
                    transport.freeze_sparkle()
            finally:
                os.chdir(previous)
            self.assertIn('PUB_DATE="Sun, 01 Nov 2026 10:00:00 +0000"', target.read_text())
        self.assertEqual(original.read_bytes(), contents)


if __name__ == '__main__':
    unittest.main()
