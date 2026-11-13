import base64
import copy
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
import promote_candidate as promotion


class PromotionTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        (self.root / 'cmd/htmlclay').mkdir(parents=True)
        (self.root / 'cmd/htmlclay/main.go').write_text('var version = "1.2.3"\n')
        self.record = dict(schema=2, app='htmlclay', version='1.2.3', source_sha='a' * 40, release_sha='e' * 40,
                           manifest_identity='b' * 64, candidate_sha256='c' * 64,
                           payload_date='2026-11-01T10:00:00.000Z', bucket='hyperclay-candidates')
        installers, updates = promotion.names('1.2.3')
        self.installers, self.updates = installers, updates
        contents = {name: ('retained signed bytes: ' + name).encode() for name in installers + updates}
        zipname = 'HTMLClay-1.2.3-universal.app.zip'
        contents[zipname] = b'notarized Sparkle zip'
        artifacts = {}
        for name in updates:
            target = name[len('htmlclay-1.2.3-'):].rsplit('.', 1)[0]
            sha = promotion.digest(contents[name])
            artifacts[target] = dict(url=f'https://download.htmlclay.com/update/1.2.3/{name}?sha256={sha}',
                                     sha256=sha, size=len(contents[name]))
        payload = json.dumps(dict(version='1.2.3', sourceSha='a' * 40,
                                  date=self.record['payload_date'], artifacts=artifacts)).encode()
        self.stable = json.dumps(dict(payload=base64.b64encode(payload).decode(), signatures=['retained'])).encode()
        self.appcast = b'<sparkle:version>1.2.3</sparkle:version>'
        contents['update-payload.json'] = payload
        contents['update-feed.tar.gz'] = self.archive({'update-payload.json': payload, 'stable.json': self.stable})
        contents['sparkle-out.tar.gz'] = self.archive({'appcast.xml': self.appcast, zipname: contents[zipname]})
        contents['SHA256SUMS'] = ''.join(promotion.digest(contents[n]) + '  ' + n + '\n' for n in installers).encode()
        prefix = 'candidates/htmlclay/' + 'c' * 64 + '/attempt/out/'
        self.record['files'] = [dict(group=n, name=n, key=prefix+n, sha256=promotion.digest(b), size=len(b)) for n, b in contents.items()]
        self.private = {prefix+n: b for n, b in contents.items()}
        self.public = {}
        self.commands = []
        self.fail_key = None
        self.tagged = None
        self.head = self.record['release_sha']
        self.dispatched = False
        self.published = False
        self.remote_tag = None
        self.remote_head = None
        self.changed = ['website/index.html']
        self.ancestor = True
        self.stamp = False
        self.dirty = False
        self.env = dict(R2_ACCOUNT_ID='fixture', R2_ACCESS_KEY='public', R2_SECRET_KEY='public',
                        R2_BUCKET='public', R2_CANDIDATES_ACCESS_KEY_ID='private', R2_CANDIDATES_SECRET_ACCESS_KEY='private')
        self.stack = [patch.object(promotion, 'ROOT', self.root),
                      patch.object(promotion, 'run', self.command),
                      patch.object(promotion, 'private_env', return_value=dict(self.env)),
                      patch.dict(os.environ, self.env)]
        for context in self.stack:
            context.start()
            self.addCleanup(context.stop)

    @staticmethod
    def archive(files):
        stream = io.BytesIO()
        with tarfile.open(fileobj=stream, mode='w:gz') as archive:
            for name, data in files.items():
                member = tarfile.TarInfo('./' + name)
                member.size = len(data)
                archive.addfile(member, io.BytesIO(data))
        return stream.getvalue()

    def command(self, *args, **kwargs):
        self.commands.append(args)
        self.assertNotIn(args[0], ('go', 'codesign', 'xcrun', 'signtool', 'npm', 'jest'))
        if args[0] == 'aws':
            if 'get-object' in args:
                key = args[args.index('--key')+1]
                objects = self.public if args[args.index('--bucket')+1] == 'public' else self.private
                if key not in objects:
                    raise subprocess.CalledProcessError(1, args, stderr='(404) Not Found')
                Path(args[-1]).write_bytes(objects[key])
            elif 'cp' in args:
                source, destination = args[args.index('cp')+1:args.index('cp')+3]
                if source.startswith('s3://'):
                    Path(destination).write_bytes(self.public[source.split('/', 3)[3]])
                else:
                    key = destination.split('/', 3)[3]
                    if self.fail_key == key:
                        self.fail_key = None
                        raise subprocess.CalledProcessError(1, args)
                    self.public[key] = Path(source).read_bytes()
            else:
                self.fail('unexpected aws command: ' + str(args))
        elif args[0] == 'bash':
            self.assertTrue(args[1].endswith('r2-put-immutable.sh'))
            path, key = Path(args[2]), args[3]
            if key in self.public:
                self.assertEqual(self.public[key], path.read_bytes())
            else:
                self.public[key] = path.read_bytes()
        elif args[0] == 'git':
            if args[1:3] == ('status', '--porcelain'):
                return ' M tracked' if self.dirty else ''
            if args[1] == 'rev-parse':
                if args[-1] == 'HEAD':
                    return self.head
                if not self.tagged:
                    raise subprocess.CalledProcessError(1, args)
                return self.tagged
            if args[1] == 'branch':
                return 'main'
            if args[1] == 'merge-base' and not self.ancestor:
                raise subprocess.CalledProcessError(1, args)
            if args[1] == 'log':
                return '\n'.join(self.changed)
            if args[1] == 'ls-remote':
                sha = self.remote_tag if 'refs/tags/' in args[3] else self.remote_head
                return sha + '\t' + args[3] if sha else ''
            if args[1] == 'push':
                if args[-1] == 'HEAD':
                    self.remote_head = self.head
                else:
                    self.remote_tag = self.tagged
            if args[1] == 'diff':
                return 'website/index.html' if '--cached' in args and self.stamp else ''
            if args[1] == 'tag':
                self.assertIsNone(self.tagged)
                self.tagged = args[3]
            if args[1] == 'commit':
                self.head = 'd' * 40
        elif args[0] == 'gh':
            if args[1:3] == ('workflow', 'run'):
                self.dispatched = True
            elif args[1:3] == ('run', 'list'):
                identity = promotion.digest(json.dumps(self.record, sort_keys=True, separators=(',', ':')).encode())
                return json.dumps([dict(databaseId=1, displayTitle='unrelated', status='completed', conclusion='success')] +
                                  ([dict(databaseId=2, displayTitle='promote ' + identity,
                                         status='completed' if self.published else 'in_progress',
                                         conclusion='success' if self.published else '')] if self.dispatched else []))
            elif args[1:3] == ('run', 'watch'):
                self.assertEqual(args[3], '2')
                with tempfile.TemporaryDirectory() as directory:
                    promotion.publish(self.record, Path(directory))
                self.published = True
        elif args[0] == 'node':
            self.assertEqual(args[1], 'scripts/stamp-website.js')
            if args[-1] == '--check':
                if not self.stamp:
                    raise subprocess.CalledProcessError(2, args)
                return ''
            self.stamp = True
        else:
            self.fail('unexpected command: ' + str(args))
        return ''

    def publish(self):
        with tempfile.TemporaryDirectory() as directory:
            promotion.validate(self.record)
            promotion.publish(self.record, Path(directory))

    def test_exact_retained_bytes_and_publication_order(self):
        self.publish()
        for artifact in self.record['files']:
            name = artifact['name']
            if name in self.installers:
                key = name
            elif name in self.updates or name.endswith('.app.zip'):
                key = 'update/1.2.3/' + name
            else:
                continue
            self.assertEqual(self.public[key], self.private[artifact['key']])
        self.assertEqual(self.public['update/stable.json'], self.stable)
        self.assertEqual(self.public['update/macos/appcast.xml'], self.appcast)
        self.assertEqual(list(self.public)[-1], 'htmlclay-release-info.json')
        info = json.loads(self.public['htmlclay-release-info.json'])
        self.assertEqual(info['date'], self.record['payload_date'])
        self.assertEqual(info['sourceSha'], self.record['release_sha'])

    def test_missing_expired_corrupt_and_wrong_size_refuse_before_upload(self):
        artifact = self.record['files'][0]
        original = self.private[artifact['key']]
        for value in (None, b'corrupt', original + b'x'):
            with self.subTest(value=value):
                if value is None:
                    self.private.pop(artifact['key'], None)
                else:
                    self.private[artifact['key']] = value
                with self.assertRaisesRegex(ValueError, 'requalify'):
                    self.publish()
                self.assertEqual(self.public, {})
        self.private[artifact['key']] = bytes(reversed(original))
        with self.assertRaisesRegex(ValueError, 'requalify'):
            self.publish()

    def test_record_validation(self):
        for field, value in [('version', '1.2.3-dev'), ('source_sha', 'no'), ('release_sha', 'no'), ('bucket', 'public')]:
            record = dict(self.record, **{field: value})
            with self.subTest(field=field), self.assertRaises(ValueError):
                promotion.validate(record)
        record = copy.deepcopy(self.record)
        record['files'].pop()
        with self.assertRaisesRegex(ValueError, 'requalify'):
            promotion.validate(record)

    def test_release_or_version_mismatch_refuses_without_dispatch(self):
        for mismatch in ('release', 'version', 'tag'):
            with self.subTest(mismatch=mismatch), tempfile.TemporaryDirectory() as directory:
                self.head = 'a' * 40 if mismatch == 'release' else self.record['release_sha']
                self.ancestor = mismatch != 'release'
                self.tagged = 'f' * 40 if mismatch == 'tag' else None
                (self.root / 'cmd/htmlclay/main.go').write_text('var version = "' + ('1.2.2' if mismatch == 'version' else '1.2.3') + '"')
                with self.assertRaisesRegex(ValueError, 'differs'):
                    promotion.promote(self.record, Path(directory), True)
                self.assertFalse(self.dispatched)
                self.assertFalse(self.public)

    def test_interrupted_upload_reconciles_same_version_and_tag(self):
        self.fail_key = 'update/stable.json'
        with tempfile.TemporaryDirectory() as directory:
            with self.assertRaises(subprocess.CalledProcessError):
                promotion.promote(self.record, Path(directory), True)
        self.assertIsNone(self.tagged)
        self.assertNotIn('htmlclay-release-info.json', self.public)
        self.dispatched = False
        with tempfile.TemporaryDirectory() as directory:
            promotion.promote(self.record, Path(directory), True)
        self.assertEqual(self.tagged, self.record['release_sha'])
        before = len(self.commands)
        with tempfile.TemporaryDirectory() as directory:
            promotion.promote(self.record, Path(directory), True)
        repeated = self.commands[before:]
        self.assertFalse(any(c[:2] in (('gh', 'workflow'), ('gh', 'run')) and c[2] != 'list' for c in repeated))
        self.assertFalse(any(c[:2] in (('git', 'tag'), ('git', 'push'), ('git', 'commit')) for c in repeated))
        self.assertFalse(any(c[0] == 'node' and c[-1] != '--check' for c in repeated))
        tags = [c for c in self.commands if c[:2] == ('git', 'tag')]
        self.assertEqual(tags, [('git', 'tag', 'v1.2.3', self.record['release_sha'])])
        self.assertTrue(all(c[1] != 'build' for c in self.commands if len(c) > 1))

    def test_explicitly_skipped_optional_feeds_publish_no_feed(self):
        replacements = {
            'sparkle-out.tar.gz': self.archive({'SKIPPED': b''}),
            'update-feed.tar.gz': self.archive({'update-payload.json': next(
                self.private[a['key']] for a in self.record['files'] if a['name'] == 'update-payload.json'), 'SKIPPED': b''})}
        for artifact in self.record['files']:
            if artifact['name'] in replacements:
                data = replacements[artifact['name']]
                self.private[artifact['key']] = data
                artifact.update(sha256=promotion.digest(data), size=len(data))
        self.publish()
        self.assertNotIn('update/stable.json', self.public)
        self.assertNotIn('update/macos/appcast.xml', self.public)
        self.assertIn('htmlclay-release-info.json', self.public)

    def test_local_tag_created_before_interrupted_push_is_reused(self):
        original = promotion.run
        failed = False
        def fail_push(*args, **kwargs):
            nonlocal failed
            if args == ('git', 'push', 'origin', 'v1.2.3') and not failed:
                failed = True
                raise subprocess.CalledProcessError(1, args)
            return original(*args, **kwargs)
        with patch.object(promotion, 'run', fail_push), tempfile.TemporaryDirectory() as directory:
            with self.assertRaises(subprocess.CalledProcessError):
                promotion.promote(self.record, Path(directory), True)
        self.assertEqual(self.tagged, self.record['release_sha'])
        self.dispatched = False
        with tempfile.TemporaryDirectory() as directory:
            promotion.promote(self.record, Path(directory), True)
        self.assertEqual(len([c for c in self.commands if c[:2] == ('git', 'tag')]), 1)

    def real_checkout(self):
        def git(*args):
            return subprocess.check_output(['git', '-C', str(self.root), *args], text=True,
                                           stderr=subprocess.PIPE).strip()
        self.git = git
        git('init', '-b', 'main')
        git('config', 'user.name', 'Fixture')
        git('config', 'user.email', 'fixture@example.invalid')
        git('config', 'commit.gpgsign', 'false')
        (self.root / 'website').mkdir()
        (self.root / 'website/index.html').write_text(
            '<span data-version>v1.2.2</span>'
            '<span data-sha256="htmlclay-linux-amd64.tar.gz">old</span>')
        (self.root / 'scripts').mkdir()
        (self.root / 'scripts/stamp-website.js').write_text((ROOT / 'scripts/stamp-website.js').read_text())
        git('add', '.')
        git('commit', '-m', 'release fixture')
        self.record['release_sha'] = git('rev-parse', 'HEAD')
        remote = tempfile.TemporaryDirectory(prefix='htmlclay-remote-')
        self.addCleanup(remote.cleanup)
        subprocess.check_output(['git', 'init', '--bare', remote.name], stderr=subprocess.PIPE)
        git('remote', 'add', 'origin', remote.name)
        git('push', 'origin', 'main')

        def command(*args, **kwargs):
            if args[0] in ('git', 'node'):
                self.commands.append(args)
                return subprocess.check_output(args, cwd=self.root, text=True,
                                               stderr=subprocess.PIPE).strip()
            return self.command(*args, **kwargs)
        return command

    def test_real_tail_resumes_failed_head_push_and_skips_completed_steps(self):
        command = self.real_checkout()
        failed = False
        def fail_push(*args, **kwargs):
            nonlocal failed
            if args == ('git', 'push', 'origin', 'HEAD') and not failed:
                failed = True
                raise subprocess.CalledProcessError(1, args)
            return command(*args, **kwargs)
        with patch.object(promotion, 'run', fail_push):
            with tempfile.TemporaryDirectory() as directory:
                with self.assertRaises(subprocess.CalledProcessError):
                    promotion.promote(self.record, Path(directory), True)
            self.assertNotEqual(self.git('rev-parse', 'HEAD'), self.record['release_sha'])
            before = len(self.commands)
            with tempfile.TemporaryDirectory() as directory:
                promotion.promote(self.record, Path(directory), True)
            resumed = self.commands[before:]
            self.assertEqual([c for c in resumed if c[:2] == ('git', 'push')],
                             [('git', 'push', 'origin', 'HEAD')])
            before = len(self.commands)
            with tempfile.TemporaryDirectory() as directory:
                promotion.promote(self.record, Path(directory), True)
            repeated = self.commands[before:]
            for commands in (resumed, repeated):
                self.assertFalse(any(c[:2] in (('git', 'tag'), ('git', 'commit'), ('gh', 'workflow')) for c in commands))
                self.assertFalse(any(c[0] == 'bash' or (c[0] == 'aws' and 'cp' in c) for c in commands))
                self.assertFalse(any(c[0] == 'node' and c[-1] != '--check' for c in commands))
            self.assertFalse(any(c[:2] == ('git', 'push') for c in repeated))
            self.assertEqual(self.git('rev-parse', 'v1.2.3'), self.record['release_sha'])

    def test_non_release_owned_descendant_refuses(self):
        command = self.real_checkout()
        (self.root / 'cmd/htmlclay/main.go').write_text('var version = "1.2.3"\n// unqualified change\n')
        self.git('add', '.')
        self.git('commit', '-m', 'unqualified change')
        with patch.object(promotion, 'run', command), tempfile.TemporaryDirectory() as directory:
            with self.assertRaisesRegex(ValueError, 'checkout differs from promotion release commit; requalify'):
                promotion.promote(self.record, Path(directory), True)
        self.assertFalse(self.dispatched)

    def test_repeated_publication_skips_existing_uploads(self):
        self.publish()
        before = len(self.commands)
        self.publish()
        self.assertFalse(any(c[0] == 'aws' and 'cp' in c and not c[c.index('cp')+1].startswith('s3://')
                             for c in self.commands[before:]))

    def test_active_publication_is_watched_without_redispatch(self):
        self.dispatched = True
        with tempfile.TemporaryDirectory() as directory:
            promotion.promote(self.record, Path(directory), True)
        self.assertFalse(any(c[:2] == ('gh', 'workflow') for c in self.commands))
        self.assertTrue(self.published)

    def test_public_object_access_error_refuses_without_upload(self):
        original = promotion.run
        def denied(*args, **kwargs):
            if args[0] == 'aws' and 'get-object' in args and args[args.index('--bucket')+1] == 'public':
                raise subprocess.CalledProcessError(1, args, stderr='(403) AccessDenied')
            return original(*args, **kwargs)
        with patch.object(promotion, 'run', denied), self.assertRaises(subprocess.CalledProcessError):
            self.publish()
        self.assertEqual(self.public, {})

    def test_files_match_real_candidate_contract(self):
        contract = json.loads(subprocess.check_output(
            [sys.executable, str(ROOT / 'scripts/release_candidate.py'), 'contract'], text=True))
        expected = {name.replace('{version}', self.record['version'])
                    for name in contract['artifacts']
                    if name != 'identity' and not name.startswith('conformance-')}
        self.assertEqual({row['name'] for row in self.record['files']}, expected)
        promotion.validate(self.record)

    def test_identity_and_diagnostic_rows_refuse(self):
        for name in ('identity', 'test-report.json'):
            record = copy.deepcopy(self.record)
            row = dict(record['files'][0], group='checksums', name=name)
            row['key'] = row['key'].rsplit('/', 1)[0] + '/' + name
            record['files'].append(row)
            with self.subTest(name=name), self.assertRaises(ValueError):
                promotion.validate(record)

    def test_check_is_offline_and_has_no_side_effects(self):
        record = self.root / 'record.json'
        record.write_text(json.dumps(self.record))
        with patch.object(promotion.tempfile, 'TemporaryDirectory', side_effect=AssertionError('side effect')):
            promotion.main(['--check', str(record)])
        self.assertEqual(self.commands, [])
        before = set(self.root.rglob('*'))
        result = subprocess.run([sys.executable, str(ROOT / 'scripts/promote_candidate.py'),
                                 '--check', str(record)], env=dict(os.environ, PATH=''),
                                text=True, capture_output=True)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(set(self.root.rglob('*')), before)
        self.assertEqual(json.loads(record.read_text()), self.record)
        invalid = dict(self.record, schema=1)
        record.write_text(json.dumps(invalid))
        with self.assertRaisesRegex(ValueError, 'schema 2'):
            promotion.main(['--check', str(record)])
        self.assertEqual(self.commands, [])

    def test_utc_dates_normalize_without_rewriting_retained_bytes(self):
        for date in ('2026-11-01T10:00:00Z', '2026-11-01T10:00:00.000Z',
                     '2026-11-01T10:00:00+00:00', '2026-11-01T10:00:00+0000'):
            self.record['payload_date'] = date
            self.publish()
            self.assertEqual(self.record['payload_date'], '2026-11-01T10:00:00.000Z')
            self.assertEqual(self.public['update/stable.json'], self.stable)
        for date in ('2026-11-01T10:00:00', '2026-11-01T10:00:00+01:00', 'invalid'):
            with self.subTest(date=date), self.assertRaises(ValueError):
                promotion.validate(dict(self.record, payload_date=date))

    def test_dirty_tree_refuses_before_network(self):
        self.dirty = True
        with self.assertRaisesRegex(ValueError, 'uncommitted'):
            promotion.promote(self.record, self.root, True)
        self.assertEqual(self.commands, [('git', 'status', '--porcelain')])

    def test_shell_entry_bypasses_legacy_build_path(self):
        record = self.root / 'record.json'
        record.write_text(json.dumps(self.record))
        port = self.root / 'bin'
        port.mkdir()
        python = port / 'python3'
        python.write_text('#!/bin/sh\nprintf "%s\\n" "$@"\n')
        python.chmod(0o755)
        for options in ([], ['--no-install']):
            with self.subTest(options=options):
                result = subprocess.run(['/bin/bash', str(ROOT / 'scripts/release.sh'), '--from-candidate=' + str(record), *options],
                                        env=dict(os.environ, PATH=str(port)+':'+os.environ['PATH']), text=True, capture_output=True)
                self.assertEqual(result.returncode, 0, result.stderr)
                self.assertEqual(result.stdout.splitlines(), ['scripts/promote_candidate.py', str(record), *options])


if __name__ == '__main__':
    unittest.main()
