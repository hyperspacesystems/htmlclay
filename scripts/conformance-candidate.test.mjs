import assert from 'node:assert/strict';
import { spawnSync } from 'node:child_process';
import { createHash } from 'node:crypto';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import test from 'node:test';
import { fileURLToPath } from 'node:url';
import { canonical, evidencePathFor, parseCandidateArgs, validateCandidateContext, validateReport, verdictExit } from './conformance-candidate.mjs';

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
  return JSON.parse(fs.readFileSync(path.join(cwd, evidencePathFor(platform)), 'utf8'));
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
  assert.equal(fs.existsSync(path.join(cwd, '.conformance', 'conformance-linux.json')), false);
});

test('a signal, a missing report and a malformed report all fail the job', () => {
  for (const mode of ['signal', 'missing', 'malformed']) {
    const { cwd, env } = fixture();
    const result = run({ cwd, env: { ...env, FAKE_MODE: mode }, args: ['--platform', 'linux'] });
    assert.equal(result.status, 3, mode);
    assert.equal(fs.existsSync(path.join(cwd, '.conformance', 'conformance-linux.json')), false, mode);
  }
});

test('a report that disagrees with the child exit is not a verdict', () => {
  for (const mode of ['zero_with_fail', 'one_without_fail', 'wrong_version']) {
    const { cwd, env } = fixture();
    const result = run({ cwd, env: { ...env, FAKE_MODE: mode }, args: ['--platform', 'linux'] });
    assert.equal(result.status, 3, mode);
    assert.equal(fs.existsSync(path.join(cwd, '.conformance', 'conformance-linux.json')), false, mode);
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
  fs.mkdirSync(path.join(cwd, '.conformance', 'conformance-linux.json'), { recursive: true });
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

test('--engine is forwarded to the launcher and validated the same way', () => {
  assert.deepEqual(
    parseCandidateArgs(['--platform', 'linux', '--engine', 'page,http', '--url', 'u']),
    { platform: 'linux', launcher: ['--engine', 'page,http', '--url', 'u'] },
  );
  assert.deepEqual(
    parseCandidateArgs(['--platform', 'linux', '--engine=page,http']),
    { platform: 'linux', launcher: ['--engine=page,http'] },
  );
  assert.deepEqual(parseCandidateArgs(['--platform', 'linux']), { platform: 'linux', launcher: [] });
  for (const value of ['page,bogus', '', 'page,page']) {
    assert.throws(
      () => parseCandidateArgs(['--platform', 'linux', '--engine', value]),
      /--engine must be one of all, http, page, frozen/,
    );
  }
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

function reportFile(checks) {
  const cwd = scratch();
  const file = path.join(cwd, 'report.json');
  fs.writeFileSync(file, JSON.stringify({
    schema: 1,
    package: PACKAGE,
    version: '1.0.0',
    host: HOST,
    checks,
  }));
  return file;
}

test('validateReport accepts every row status the package emits', () => {
  const statuses = ['pass', 'fail', 'skip', 'na', 'waived', 'waiver-unused'];
  const report = validateReport(reportFile(statuses.map((status, index) => ({
    id: `row.${index}`,
    status,
    blocking: status === 'fail',
  }))));
  assert.deepEqual(report.checks.map((check) => check.status), statuses);
  assert.equal(validateReport(reportFile([{ id: 'row.one', status: 'na' }])).checks[0].status, 'na');

  assert.throws(() => validateReport(reportFile([{ id: 'row.one', status: 'maybe' }])),
    /gives row row\.one the unknown status "maybe"/);
  assert.throws(() => validateReport(reportFile([{ id: 'row.one', status: 'na', blocking: true }])),
    /leaves row row\.one unanswered/);
  assert.throws(() => validateReport(reportFile([{ id: 'row.one' }])),
    /gives row row\.one the unknown status null/);
  assert.throws(() => validateReport(reportFile([{ id: 'row.one', status: 'pass' }, { id: 'row.one', status: 'skip' }])),
    /needs nonempty unique row ids/);
});

const GATE_ROWS = [
  ['capability.conditional', 'pass', false], ['capability.data-guard-v1', 'pass', false],
  ['capability.data-read', 'pass', false], ['capability.data-write', 'pass', false],
  ['capability.page', 'pass', false], ['capability.page-awareness-v1', 'pass', false],
  ['capability.page-format-v1', 'pass', false], ['capability.page-recovery-v1', 'pass', false],
  ['capability.receipts', 'pass', false], ['capability.record-awareness-v1', 'pass', false],
  ['capability.sync', 'pass', false], ['capability.sync-worker', 'pass', false],
  ['capability.upload', 'pass', false], ['capability.wire', 'pass', false],
  ['capability.registered', 'pass', false], ['page.target-same-origin', 'pass', false],
  ['core.meta', 'pass', false], ['core.save-roundtrip', 'pass', false],
  ['core.document-block', 'pass', false], ['core.non-html-refused', 'pass', false],
  ['core.refusal-reason', 'pass', false], ['core.traversal', 'na', false],
  ['core.cross-origin', 'na', false], ['core.unknown-host-path', 'pass', false],
  ['format.opt-in', 'skip', false], ['sync.one-stream', 'pass', false],
  ['conditional.stale-if-match', 'pass', false], ['receipts.save-id', 'pass', false],
  ['upload.announced', 'pass', false], ['upload.url-resolves', 'pass', false],
  ['upload.no-listing', 'pass', false], ['upload.same-name', 'pass', false],
  ['upload.concurrent', 'pass', false], ['upload.document-refused', 'pass', false],
  ['upload.not-a-page', 'pass', false], ['upload.traversal', 'pass', false],
  ['upload.size-cap', 'pass', false], ['wire.document-url', 'pass', false],
  ['core.origin-probe', 'pass', true], ['frozen.inline-token-save', 'pass', false],
  ['frozen.welcome-alias-save', 'pass', false], ['frozen.oldest-cookie-lane', 'pass', false],
  ['frozen.bare-save-route', 'pass', false], ['frozen.marked-save-route', 'pass', false],
  ['frozen.unchanged-save', 'pass', false], ['frozen.lost-response', 'pass', false],
  ['frozen.foreign-save-not-overwritten', 'pass', false], ['http.h1', 'pass', false],
  ['http.h2', 'pass', false], ['http.h3-token', 'pass', false],
  ['http.h3-strip', 'pass', false], ['http.h4-identity', 'skip', false],
  ['http.h5', 'pass', false], ['http.h6', 'pass', false],
  ['http.h7', 'pass', false], ['http.h8', 'pass', false],
  ['http.h9', 'pass', false], ['http.h10-refused', 'pass', false],
  ['http.h11', 'pass', false], ['http.data-guard-restore', 'pass', false],
  ['http.data-guard-refusals', 'pass', false], ['http.data-guard-notified', 'pass', false],
];

test('a complete report shaped like the real gate carries evidence', () => {
  assert.equal(GATE_ROWS.filter(([, status]) => status === 'skip' || status === 'na').length, 4);
  const report = validateReport(reportFile(GATE_ROWS.map(([id, status, blocking]) => ({ id, status, blocking }))));
  assert.equal(report.checks.length, GATE_ROWS.length);
  assert.equal(verdictExit(report, 0), 0);
});

function uploadArtifactSteps(source) {
  const lines = source.split('\n');
  const steps = [];
  for (let i = 0; i < lines.length; i += 1) {
    const start = /^(\s*)-\s+uses:\s*actions\/upload-artifact@/.exec(lines[i]);
    if (!start) continue;
    const stepIndent = start[1].length;
    const body = [];
    for (let j = i + 1; j < lines.length; j += 1) {
      const line = lines[j];
      if (!line.trim()) continue;
      const indent = line.match(/^\s*/)[0].length;
      if (indent <= stepIndent) break;
      body.push(line);
    }
    const field = (key) => {
      const found = body
        .map((line) => new RegExp(`^\\s+${key}:\\s*(.+?)\\s*$`).exec(line))
        .find(Boolean);
      return found ? found[1].replace(/^['"]|['"]$/g, '') : null;
    };
    steps.push({ name: field('name'), path: field('path') });
  }
  return steps;
}

test('every conformance evidence upload names the file the wrapper writes', () => {
  const directory = path.join(HERE, '..', '.github', 'workflows');
  const files = fs.readdirSync(directory).filter((file) => /\.ya?ml$/.test(file)).sort();
  assert.ok(files.length, 'no workflows were found');
  const byFile = new Map();
  for (const file of files) {
    const uploads = uploadArtifactSteps(fs.readFileSync(path.join(directory, file), 'utf8'))
      .filter((step) => step.name && step.name.startsWith('conformance-'));
    if (!uploads.length) continue;
    byFile.set(file, uploads);
  }
  assert.ok(byFile.size, 'no workflow uploads conformance evidence');
  assert.ok(byFile.has('platform-candidate.yml'));
  for (const [file, uploads] of byFile) {
    for (const upload of uploads) {
      const expected = evidencePathFor(upload.name.slice('conformance-'.length));
      assert.ok(upload.path === expected || upload.path.endsWith(`/${expected}`),
        `${file} uploads ${upload.name} from ${upload.path}, not ${expected}`);
    }
  }
});
