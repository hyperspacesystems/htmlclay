import assert from 'node:assert/strict';
import { spawnSync } from 'node:child_process';
import { createHash } from 'node:crypto';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import test from 'node:test';
import { fileURLToPath } from 'node:url';
import { canonical, parseCandidateArgs, validateCandidateContext } from './conformance-candidate.mjs';

const HERE = path.dirname(fileURLToPath(import.meta.url));
const WRAPPER = path.join(HERE, 'conformance-candidate.mjs');
const PACKAGE = '@panphora/host-conformance';
const HOST = 'htmlclay';

const LAUNCHER = `import { writeFileSync } from 'node:fs';
const modes = {
  green: { code: 0, checks: [['row.one', 'pass']] },
  red: { code: 1, checks: [['row.one', 'pass'], ['row.two', 'fail']] },
  infra: { code: 3, checks: [['row.one', 'na']] },
  zero_with_fail: { code: 0, checks: [['row.one', 'fail']] },
  one_without_fail: { code: 1, checks: [['row.one', 'pass']] },
  missing: { code: 0 },
  malformed: { code: 0, raw: '{ not json' },
  wrong_version: { code: 0, version: 'not-a-version', checks: [['row.one', 'pass']] },
  signal: { code: null, signal: true },
};
const mode = modes[process.env.FAKE_MODE || 'green'];
if (process.env.FAKE_ARGS) writeFileSync(process.env.FAKE_ARGS, JSON.stringify(process.argv.slice(2)));
if (mode.raw) writeFileSync(process.env.CONFORMANCE_REPORT, mode.raw);
else if (mode.checks) writeFileSync(process.env.CONFORMANCE_REPORT, JSON.stringify({
  schema: 1,
  package: '${PACKAGE}',
  version: mode.version || '1.0.0',
  host: '${HOST}',
  checks: mode.checks.map(([id, status]) => ({ id, status })),
}));
if (mode.signal) process.kill(process.pid, 'SIGKILL');
process.exit(mode.code);
`;

function sha256(bytes) {
  return createHash('sha256').update(bytes).digest('hex');
}

function scratch() {
  return fs.mkdtempSync(path.join(os.tmpdir(), 'conformance-candidate-test-'));
}

function fixture({ platform = 'linux', manifest = {}, candidate = '{"schema":1}\n', writePlatform = true } = {}) {
  const cwd = scratch();
  fs.writeFileSync(path.join(cwd, 'go.mod'), 'module fixture\n');
  fs.writeFileSync(path.join(cwd, 'candidate.json'), candidate);
  const record = {
    manifest: {
      app: HOST,
      candidate_sha256: sha256(Buffer.from(candidate)),
      prepared_tree_sha256: 'b'.repeat(64),
      version: '1.0.0',
      workflow_sha: 'c'.repeat(40),
      checks: ['conformance'],
      artifacts: ['conformance-linux', 'conformance-linux-amd64-artifact', 'conformance-linux-arm64-artifact'],
      attempt: 'attempt-fixture',
      ...manifest,
    },
    source_sha: 'd'.repeat(40),
    payload_date: '2026-11-01T10:00:00Z',
  };
  if (writePlatform) fs.writeFileSync(path.join(cwd, 'platform-candidate.json'), JSON.stringify(record));
  const launcher = path.join(cwd, 'fake-launcher.mjs');
  fs.writeFileSync(launcher, LAUNCHER);
  const identity = sha256(Buffer.from(canonical(record.manifest)));
  const env = {
    PATH: process.env.PATH,
    APP: HOST,
    ATTEMPT: record.manifest.attempt,
    CANDIDATE_SHA256: record.manifest.candidate_sha256,
    WORKFLOW_SHA: record.manifest.workflow_sha,
    MANIFEST_IDENTITY: identity,
    CONFORMANCE_LAUNCHER: launcher,
    FAKE_MODE: 'green',
  };
  return { cwd, env, platform, manifest: record.manifest, identity };
}

function run({ cwd, env, args = [] }) {
  return spawnSync(process.execPath, [WRAPPER, ...args], {
    cwd,
    env: { ...env },
    encoding: 'utf8',
  });
}

function evidence(cwd, platform = 'linux') {
  return JSON.parse(fs.readFileSync(path.join(cwd, '.conformance', `${platform}.json`), 'utf8'));
}

test('valid green and red child verdicts become evidence and a passed job', () => {
  for (const [mode, exitCode, statuses] of [
    ['green', 0, ['pass']],
    ['red', 1, ['pass', 'fail']],
  ]) {
    const { cwd, env, identity } = fixture();
    const result = run({ cwd, env: { ...env, FAKE_MODE: mode }, args: ['--platform', 'linux'] });
    assert.equal(result.status, 0, result.stderr);
    const envelope = evidence(cwd);
    assert.deepEqual(envelope, {
      schema: 1,
      app: HOST,
      platform: 'linux',
      workflow_sha: 'c'.repeat(40),
      candidate_sha256: sha256(Buffer.from('{"schema":1}\n')),
      attempt: 'attempt-fixture',
      manifest_identity: identity,
      exitCode,
      report: {
        schema: 1,
        package: PACKAGE,
        version: '1.0.0',
        host: HOST,
        checks: statuses.map((status, index) => ({ id: ['row.one', 'row.two'][index], status })),
      },
    });
  }
});

test('an infrastructure exit fails the job and writes no evidence', () => {
  const { cwd, env } = fixture();
  const result = run({ cwd, env: { ...env, FAKE_MODE: 'infra' }, args: ['--platform', 'linux'] });
  assert.equal(result.status, 3);
  assert.match(result.stderr, /exited 3, which is not a verdict/);
  assert.equal(fs.existsSync(path.join(cwd, '.conformance', 'linux.json')), false);
});

test('a signal, a missing report and a malformed report all fail the job', () => {
  for (const mode of ['signal', 'missing', 'malformed']) {
    const { cwd, env } = fixture();
    const result = run({ cwd, env: { ...env, FAKE_MODE: mode }, args: ['--platform', 'linux'] });
    assert.equal(result.status, 3, mode);
    assert.equal(fs.existsSync(path.join(cwd, '.conformance', 'linux.json')), false, mode);
  }
});

test('a report that disagrees with the child exit is not a verdict', () => {
  for (const mode of ['zero_with_fail', 'one_without_fail', 'wrong_version']) {
    const { cwd, env } = fixture();
    const result = run({ cwd, env: { ...env, FAKE_MODE: mode }, args: ['--platform', 'linux'] });
    assert.equal(result.status, 3, mode);
    assert.equal(fs.existsSync(path.join(cwd, '.conformance', 'linux.json')), false, mode);
  }
});

test('candidate identity must match the verified dispatch and archive', () => {
  const mutations = [
    (env) => ({ ...env, WORKFLOW_SHA: 'e'.repeat(40) }),
    (env) => ({ ...env, MANIFEST_IDENTITY: 'f'.repeat(64) }),
    (env) => ({ ...env, CANDIDATE_SHA256: 'a'.repeat(64) }),
    (env) => ({ ...env, ATTEMPT: 'other-attempt' }),
    (env) => ({ ...env, APP: 'other-app' }),
    (env) => ({ ...env, MANIFEST_IDENTITY: '' }),
  ];
  for (const mutate of mutations) {
    const { cwd, env } = fixture();
    const result = run({ cwd, env: mutate(env), args: ['--platform', 'linux'] });
    assert.equal(result.status, 3, JSON.stringify(mutate(env)));
  }
  const { cwd, env } = fixture();
  fs.writeFileSync(path.join(cwd, 'candidate.json'), '{"schema":2}\n');
  assert.equal(run({ cwd, env, args: ['--platform', 'linux'] }).status, 3);
});

test('the platform must be one the candidate manifest declares', () => {
  const { cwd, env } = fixture();
  assert.equal(run({ cwd, env, args: ['--platform', 'windows'] }).status, 3);
  assert.equal(run({ cwd, env, args: ['--platform', 'linux-amd64-artifact'] }).status, 0);
  assert.equal(run({ cwd, env, args: ['--platform'] }).status, 3);
});

test('missing context files fail the job', () => {
  const { cwd, env } = fixture({ writePlatform: false });
  assert.equal(run({ cwd, env, args: ['--platform', 'linux'] }).status, 3);
  fs.rmSync(path.join(cwd, 'go.mod'));
  const second = fixture();
  fs.rmSync(path.join(second.cwd, 'go.mod'));
  assert.equal(run({ cwd: second.cwd, env: second.env, args: ['--platform', 'linux'] }).status, 3);
});

test('an envelope that cannot be written fails the job', () => {
  const { cwd, env } = fixture();
  fs.mkdirSync(path.join(cwd, '.conformance', 'linux.json'), { recursive: true });
  const result = run({ cwd, env, args: ['--platform', 'linux'] });
  assert.equal(result.status, 3);
});

test('attach arguments reach the launcher unchanged', () => {
  const { cwd, env } = fixture();
  const argsFile = path.join(cwd, 'args.json');
  const result = run({
    cwd,
    env: { ...env, FAKE_ARGS: argsFile },
    args: ['--platform', 'linux', '--url', 'http://docs.localhost:9', '--root', '/tmp/home',
      '--page', 'docs/_mhf-host-test.htmlclay', '--doc', '/docs/_mhf-host-test.htmlclay',
      '--target', '/docs/_mhf-host-test.htmlclay'],
  });
  assert.equal(result.status, 0, result.stderr);
  assert.deepEqual(JSON.parse(fs.readFileSync(argsFile, 'utf8')), [
    '--url', 'http://docs.localhost:9', '--root', '/tmp/home',
    '--page', 'docs/_mhf-host-test.htmlclay', '--doc', '/docs/_mhf-host-test.htmlclay',
    '--target', '/docs/_mhf-host-test.htmlclay',
  ]);
});

test('candidate arguments and manifest identity follow the dispatch', () => {
  assert.deepEqual(parseCandidateArgs(['--platform', 'linux', '--url', 'u']), { platform: 'linux', launcher: ['--url', 'u'] });
  assert.deepEqual(parseCandidateArgs(['--platform=linux', '--engine', 'http']), { platform: 'linux', launcher: ['--engine', 'http'] });
  assert.throws(() => parseCandidateArgs([]), /--platform is required/);
  assert.throws(() => parseCandidateArgs(['--platform', 'Linux']), /not a label/);
  const { cwd, env } = fixture();
  const context = validateCandidateContext({ cwd, env, platform: 'linux' });
  assert.equal(context.manifest_identity, env.MANIFEST_IDENTITY);
  assert.equal(canonical({ b: 1, a: [{ d: 2, c: 3 }] }), '{"a":[{"c":3,"d":2}],"b":1}');
});
