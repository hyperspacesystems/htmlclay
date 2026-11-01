#!/usr/bin/env python3
"""Publish qualified HTML Clay bytes, with no build or signing toolchain."""

import base64
import datetime
import hashlib
import json
import os
from pathlib import Path
import re
import shlex
import subprocess
import sys
import tarfile
import tempfile
import time

ROOT = Path(__file__).resolve().parents[1]
EXPIRED = 'candidate expired or incomplete: requalify'


def run(*args, **kwargs):
    return subprocess.check_output(args, text=True, **kwargs).strip()


def digest(data):
    return hashlib.sha256(data).hexdigest()


def names(version):
    installers = [f'htmlclay-{platform}-{arch}.{extension}'
                  for platform, extension in [('linux', 'tar.gz'), ('windows', 'zip')]
                  for arch in ('amd64', 'arm64')]
    installers.append(f'HTMLClay-{version}-universal.dmg')
    updates = [f'htmlclay-{version}-{platform}-{arch}.{extension}'
               for platform, extension in [('linux', 'gz'), ('windows', 'exe')]
               for arch in ('amd64', 'arm64')]
    return installers, updates


def normalize_date(value):
    if not isinstance(value, str):
        raise ValueError('invalid promotion payload date: expected UTC ISO 8601')
    try:
        date = datetime.datetime.fromisoformat(value.replace('Z', '+00:00'))
    except ValueError as error:
        raise ValueError('invalid promotion payload date: expected UTC ISO 8601') from error
    if date.utcoffset() != datetime.timedelta(0):
        raise ValueError('invalid promotion payload date: expected UTC ISO 8601')
    precision = 'milliseconds' if date.microsecond % 1000 == 0 else 'microseconds'
    return date.isoformat(timespec=precision).replace('+00:00', 'Z')


def validate(record):
    if record.get('schema') != 2 or record.get('app') != 'htmlclay':
        raise ValueError('expected htmlclay promotion record schema 2')
    version = record.get('version', '')
    if not re.fullmatch(r'(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)', version) or any(int(c) > 65535 for c in version.split('.')):
        raise ValueError('invalid promotion version')
    for field, length in [('source_sha', 40), ('release_sha', 40), ('candidate_sha256', 64), ('manifest_identity', 64)]:
        if not re.fullmatch('[0-9a-f]{' + str(length) + '}', record.get(field, '')):
            raise ValueError('invalid ' + field)
    record['payload_date'] = normalize_date(record['payload_date'])
    if record.get('bucket') != 'hyperclay-candidates':
        raise ValueError('invalid candidate bucket')
    installers, updates = names(version)
    expected = set(installers + updates + [f'HTMLClay-{version}-universal.app.zip',
                   'sparkle-out.tar.gz', 'update-feed.tar.gz', 'update-payload.json', 'SHA256SUMS'])
    artifacts = record.get('files', [])
    if len(artifacts) != len(expected) or {a['name'] for a in artifacts} != expected:
        raise ValueError(EXPIRED + ': missing or unexpected declared artifacts')
    if len({a['key'] for a in artifacts}) != len(artifacts):
        raise ValueError('duplicate candidate object key')
    prefixes = set()
    for artifact in artifacts:
        if not isinstance(artifact.get('group'), str) or not artifact['group'] or artifact['group'] == 'identity':
            raise ValueError('invalid publication file group')
        key = artifact['key']
        prefix = 'candidates/htmlclay/' + record['candidate_sha256'] + '/'
        if not key.startswith(prefix) or not re.fullmatch(r'[A-Za-z0-9_-]+/out/' + re.escape(artifact['name']), key[len(prefix):]):
            raise ValueError('invalid candidate object key')
        prefixes.add(key.rsplit('/out/', 1)[0])
        if not re.fullmatch('[0-9a-f]{64}', artifact['sha256']) or type(artifact['size']) is not int or artifact['size'] <= 0:
            raise ValueError('invalid candidate digest or size')
    if len(prefixes) != 1:
        raise ValueError('candidate artifacts mix qualification attempts')


def private_env():
    env = dict(os.environ)
    path = Path(os.environ.get('HYPERSAVE_R2_ENV', '~/.config/hypersave/r2-candidates.env')).expanduser()
    if path.is_file():
        for line in path.read_text().splitlines():
            parts = shlex.split(line, comments=True)
            if parts and '=' in parts[0]:
                key, value = parts[0].split('=', 1)
                if key in ('AWS_ACCESS_KEY_ID', 'AWS_SECRET_ACCESS_KEY', 'R2_ACCOUNT_ID'):
                    env.setdefault(key, value)
    for target, source in [('AWS_ACCESS_KEY_ID', 'R2_CANDIDATES_ACCESS_KEY_ID'),
                           ('AWS_SECRET_ACCESS_KEY', 'R2_CANDIDATES_SECRET_ACCESS_KEY')]:
        if source in env:
            env[target] = env[source]
    return env


def aws(env, *args):
    return run('aws', '--endpoint-url', 'https://' + env['R2_ACCOUNT_ID'] + '.r2.cloudflarestorage.com',
               '--region', 'auto', *args, env=env)


def bundle(path, allowed):
    with tarfile.open(path, 'r:gz') as archive:
        files = {}
        for member in archive.getmembers():
            if member.isdir() and member.name in ('.', './'):
                continue
            name = member.name.removeprefix('./')
            if not member.isfile() or name not in allowed or name in files:
                raise ValueError(EXPIRED + ': unexpected bundle member')
            files[name] = archive.extractfile(member).read()
        return files


def download(record, directory):
    env = private_env()
    for artifact in record['files']:
        path = directory / artifact['name']
        try:
            aws(env, 's3api', 'get-object', '--bucket', record['bucket'], '--key', artifact['key'], str(path))
        except subprocess.CalledProcessError as error:
            raise ValueError(EXPIRED) from error
        if not path.is_file() or path.stat().st_size != artifact['size'] or digest(path.read_bytes()) != artifact['sha256']:
            raise ValueError(EXPIRED + ': object hash or size mismatch')
    installers, updates = names(record['version'])
    checksums = {}
    for line in (directory / 'SHA256SUMS').read_text().splitlines():
        checksum, name = line.split()
        if name in checksums:
            raise ValueError(EXPIRED + ': duplicate checksum')
        checksums[name] = checksum
    if checksums != {name: digest((directory / name).read_bytes()) for name in installers}:
        raise ValueError(EXPIRED + ': installer checksums disagree')
    feed = bundle(directory / 'update-feed.tar.gz', {'update-payload.json', 'stable.json', 'SKIPPED'})
    sparkle = bundle(directory / 'sparkle-out.tar.gz', {'appcast.xml', 'SKIPPED', f'HTMLClay-{record["version"]}-universal.app.zip'})
    if set(feed) not in ({'update-payload.json', 'stable.json'}, {'update-payload.json', 'SKIPPED'}):
        raise ValueError(EXPIRED + ': incomplete feed bundle')
    if set(sparkle) not in ({'SKIPPED'}, {'appcast.xml', f'HTMLClay-{record["version"]}-universal.app.zip'}):
        raise ValueError(EXPIRED + ': incomplete Sparkle bundle')
    payload = (directory / 'update-payload.json').read_bytes()
    if feed['update-payload.json'] != payload:
        raise ValueError(EXPIRED + ': payload differs')
    release = json.loads(payload)
    for field, value in [('version', record['version']), ('sourceSha', record['source_sha']), ('date', record['payload_date'])]:
        actual = normalize_date(release.get(field)) if field == 'date' else release.get(field)
        if actual != value:
            raise ValueError('retained feed ' + field + ' differs from record')
    expected = {}
    for name in updates:
        target = name[len('htmlclay-' + record['version'] + '-'):].rsplit('.', 1)[0]
        checksum = digest((directory / name).read_bytes())
        expected[target] = {'url': f'https://download.htmlclay.com/update/{record["version"]}/{name}?sha256={checksum}',
                            'sha256': checksum, 'size': (directory / name).stat().st_size}
    if release.get('artifacts') != expected:
        raise ValueError(EXPIRED + ': feed artifacts disagree')
    if 'stable.json' in feed and base64.b64decode(json.loads(feed['stable.json'])['payload'], validate=True) != payload:
        raise ValueError(EXPIRED + ': signed feed payload differs')
    zipname = f'HTMLClay-{record["version"]}-universal.app.zip'
    if 'appcast.xml' in sparkle:
        if sparkle[zipname] != (directory / zipname).read_bytes() or f'<sparkle:version>{record["version"]}</sparkle:version>'.encode() not in sparkle['appcast.xml']:
            raise ValueError(EXPIRED + ': Sparkle bundle differs')
    for name in ('stable.json',):
        if name in feed:
            (directory / name).write_bytes(feed[name])
    if 'appcast.xml' in sparkle:
        (directory / 'appcast.xml').write_bytes(sparkle['appcast.xml'])
    return installers, updates


def publish(record, directory):
    installers, updates = download(record, directory)
    env = dict(os.environ)
    env['AWS_ACCESS_KEY_ID'] = env['R2_ACCESS_KEY']
    env['AWS_SECRET_ACCESS_KEY'] = env['R2_SECRET_KEY']
    bucket = env['R2_BUCKET']
    version = record['version']

    def put(name, key, content_type='application/octet-stream', immutable=False):
        path = directory / name
        if immutable:
            immutable_env = dict(env, ENDPOINT='https://' + env['R2_ACCOUNT_ID'] + '.r2.cloudflarestorage.com')
            run('bash', str(ROOT / 'scripts/r2-put-immutable.sh'), str(path), key, content_type, env=immutable_env)
        else:
            aws(env, 's3', 'cp', str(path), f's3://{bucket}/{key}', '--content-type', content_type,
                '--cache-control', 'public, max-age=60')
            readback = directory / 'readback'
            aws(env, 's3', 'cp', f's3://{bucket}/{key}', str(readback))
            if readback.read_bytes() != path.read_bytes():
                raise ValueError('publication readback differs: ' + key)

    for name in installers:
        put(name, name, immutable=name.endswith('.dmg'))
    for name in updates:
        put(name, f'update/{version}/{name}', immutable=True)
    if (directory / 'stable.json').exists():
        put('stable.json', f'update/{version}/stable.json', 'application/json', True)
        put('stable.json', 'update/stable.json', 'application/json')
    if (directory / 'appcast.xml').exists():
        name = f'HTMLClay-{version}-universal.app.zip'
        put(name, f'update/{version}/{name}', 'application/zip', True)
        put('appcast.xml', f'update/{version}/appcast.xml', 'application/xml', True)
        put('appcast.xml', 'update/macos/appcast.xml', 'application/xml')
    info = {'schema': 2, 'latest': version, 'sourceSha': record['source_sha'],
            'url': 'https://htmlclay.com/#downloads', 'date': record['payload_date'],
            'files': installers, 'sha256': {name: digest((directory / name).read_bytes()) for name in installers}}
    (directory / 'htmlclay-release-info.json').write_text(json.dumps(info) + '\n')
    put('SHA256SUMS', 'SHA256SUMS', 'text/plain')
    put('htmlclay-release-info.json', 'htmlclay-release-info.json', 'application/json')


def promote(record, directory, no_install=False):
    release, version = record['release_sha'], record['version']
    if run('git', 'status', '--porcelain'):
        raise ValueError('uncommitted changes detected')
    current = re.search(r'var version = "([^"]+)"', (ROOT / 'cmd/htmlclay/main.go').read_text())[1]
    if current != version:
        raise ValueError('source version differs from promotion version; requalify')
    head = run('git', 'rev-parse', 'HEAD')
    if head != release:
        raise ValueError('checkout differs from promotion release commit; requalify')
    run('git', 'fetch', '--tags', '--quiet', 'origin')
    tag = 'v' + version
    try:
        tagged = run('git', 'rev-parse', '-q', '--verify', 'refs/tags/' + tag + '^{commit}', stderr=subprocess.DEVNULL)
    except subprocess.CalledProcessError:
        tagged = None
    if tagged and tagged != head:
        raise ValueError('release tag differs from promotion release commit; requalify')
    download(record, directory)
    text = json.dumps(record, sort_keys=True, separators=(',', ':'))
    identity = digest(text.encode())
    branch = run('git', 'branch', '--show-current')
    previous = {r['databaseId'] for r in json.loads(run('gh', 'run', 'list', '--workflow=promote.yml',
                '--event', 'workflow_dispatch', '--limit', '30', '--json', 'databaseId,displayTitle'))}
    run('gh', 'workflow', 'run', 'promote.yml', '--ref', branch, '-f', 'record=' + text, '-f', 'record_sha256=' + identity)
    run_id = None
    for _ in range(15):
        runs = json.loads(run('gh', 'run', 'list', '--workflow=promote.yml', '--event', 'workflow_dispatch',
                              '--limit', '30', '--json', 'databaseId,displayTitle'))
        run_id = next((r['databaseId'] for r in runs if r['displayTitle'] == 'promote ' + identity and r['databaseId'] not in previous), None)
        if run_id:
            break
        time.sleep(3)
    if not run_id:
        raise ValueError('promotion run not found; retry with the same --from-candidate record')
    run('gh', 'run', 'watch', str(run_id), '--exit-status')
    if not tagged:
        run('git', 'tag', tag, head)
    run('git', 'push', 'origin', tag)
    run('node', 'scripts/stamp-website.js', version, str(directory / 'SHA256SUMS'))
    run('git', 'add', 'website/index.html')
    if run('git', 'diff', '--cached', '--name-only'):
        run('git', 'commit', '-m', 'chore: point downloads at v' + version, '--', 'website/index.html')
    run('git', 'push', 'origin', 'HEAD')
    if not no_install:
        result = subprocess.run(['bash', 'scripts/update-canary.sh', version], check=False)
        if result.returncode not in (0, 10):
            print('Release published; local update failed', file=sys.stderr)
    print('Promoted v' + version + ' from release ' + release)


def main(args):
    if not args or (args[0] == '--check' and len(args) != 2):
        raise ValueError('use <record> [--no-install], --check <record>, or publish')
    if args[0] == '--check':
        validate(json.loads(Path(args[1]).read_text()))
        return
    if args[0] == 'publish':
        text = os.environ['PROMOTION_RECORD']
        if digest(text.encode()) != os.environ['PROMOTION_RECORD_SHA256']:
            raise ValueError('promotion record digest differs')
        record = json.loads(text)
    else:
        record = json.loads(Path(args[0]).read_text())
    validate(record)
    with tempfile.TemporaryDirectory(prefix='htmlclay-promote-') as directory:
        if args[0] == 'publish':
            publish(record, Path(directory))
        else:
            promote(record, Path(directory), '--no-install' in args[1:])


if __name__ == '__main__':
    try:
        main(sys.argv[1:])
    except (ValueError, KeyError, OSError, subprocess.CalledProcessError) as error:
        print('promotion refused: ' + str(error), file=sys.stderr)
        sys.exit(1)
