import assert from 'node:assert/strict';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import test from 'node:test';
import { fileURLToPath } from 'node:url';
import {
  HOST, main, packageArgs, packageDir, PAGE, PROBE, REQUIRED_EXTENSIONS, scrub, startFixture, verdict,
} from './conformance.mjs';

// Everything here drives the launcher with a fake fixture or a fake package: the real
// gate is the all-engine run in CI, and a browser has no business in a unit test. What
// these cover is the part that decides whether a run reached a verdict at all.

const HERE = path.dirname(fileURLToPath(import.meta.url));
const REPO = path.resolve(HERE, '..');
const DIR = packageDir(REPO);
const VERSION = JSON.parse(fs.readFileSync(path.join(DIR, 'package.json'), 'utf8')).version;

function scratch() {
  return fs.mkdtempSync(path.join(os.tmpdir(), 'conformance-launcher-test-'));
}

function rows(statuses) {
  return statuses.map((status, index) => ({
    id: `http.h${index + 1}`,
    title: 'a row',
    engine: 'http',
    status,
    detail: null,
    blocking: status === 'na',
  }));
}

function report({ checks = rows(['pass']), host = HOST, version = VERSION, schema = 1 } = {}) {
  const count = (status) => checks.filter((check) => check.status === status).length;
  const unanswered = checks.filter((check) => check.status === 'na' && check.blocking).length;
  return {
    schema,
    package: '@panphora/host-conformance',
    version,
    host,
    url: 'http://127.0.0.1:1',
    startedAt: '2026-01-01T00:00:00.000Z',
    finishedAt: '2026-01-01T00:00:01.000Z',
    checks,
    summary: {
      pass: count('pass'), fail: count('fail'), skip: count('skip'), waived: 0, unused: 0, unanswered,
    },
    ok: count('fail') === 0 && unanswered === 0,
  };
}

function quiet() {
  const lines = [];
  return { lines, error: (line) => lines.push(String(line)), log: () => {}, warn: () => {} };
}

// main's scratch root, a package that answers with `outcome`, and whatever report it
// would have written. The build and fixture steps are replaced, so nothing here needs
// Go, a browser, or a port.
async function run({ exit = 0, body = report(), reportPath = null, argv = [], start, build, run: answer } = {}) {
  const dir = scratch();
  const log = quiet();
  const seen = { scratch: null, reportPath: reportPath ?? path.join(dir, 'conformance.json') };
  const code = await main({
    cwd: REPO,
    env: { CONFORMANCE_REPORT: seen.reportPath },
    argv,
    log,
    build: build ?? (async ({ scratch: root }) => {
      seen.scratch = root;
      return path.join(root, 'offline-fixture-server');
    }),
    start: start ?? (async () => ({ baseURL: 'http://127.0.0.1:1', stop: async () => {} })),
    run: answer ?? (async ({ reportPath: where }) => {
      if (body !== null) fs.writeFileSync(where, JSON.stringify(body));
      return { code: exit };
    }),
  });
  return { code, log, dir, seen };
}

test('a package that exits 1 with a failed row stays 1', async () => {
  const outcome = await run({ exit: 1, body: report({ checks: rows(['pass', 'fail']) }) });
  assert.equal(outcome.code, 1);
  assert.deepEqual(outcome.log.lines, []);
});

test('a package that exits 0 with a failed row is a harness fault, not a pass', async () => {
  const outcome = await run({ exit: 0, body: report({ checks: rows(['pass', 'fail']) }) });
  assert.equal(outcome.code, 3);
  assert.match(outcome.log.lines.join('\n'), /exited 0 .* names 1 failed/);
});

test('a package exit of 1 with no failed row is a harness fault', async () => {
  const outcome = await run({ exit: 1, body: report() });
  assert.equal(outcome.code, 3);
  assert.match(outcome.log.lines.join('\n'), /exited 1 .* names no failed check/);
});

test('an exit the package never promises is infrastructure', async () => {
  const outcome = await run({ exit: 7 });
  assert.equal(outcome.code, 3);
  assert.match(outcome.log.lines.join('\n'), /exited 7, which is not a verdict/);
});

test('a signal is infrastructure, never a verdict', async () => {
  const outcome = await run({ run: async () => ({ signal: 'SIGKILL' }) });
  assert.equal(outcome.code, 3);
  assert.match(outcome.log.lines.join('\n'), /killed by SIGKILL/);
});

test('a missing report is infrastructure', async () => {
  const outcome = await run({ body: null });
  assert.equal(outcome.code, 3);
  assert.match(outcome.log.lines.join('\n'), /wrote no report/);
});

test('a malformed report is infrastructure', async () => {
  const outcome = await run({
    run: async ({ reportPath: where }) => {
      fs.writeFileSync(where, '{ not json');
      return { code: 0 };
    },
  });
  assert.equal(outcome.code, 3);
  assert.match(outcome.log.lines.join('\n'), /is not JSON/);
});

test('a report for another host, package or schema is infrastructure', async () => {
  for (const [body, reason] of [
    [report({ host: 'hyperclay' }), /names host "hyperclay"/],
    [report({ schema: 2 }), /names schema 2/],
    [report({ version: '0.0.1' }), /names package version "0.0.1"/],
    [report({ checks: [] }), /holds no checks/],
  ]) {
    const outcome = await run({ body });
    assert.equal(outcome.code, 3, reason.source);
    assert.match(outcome.log.lines.join('\n'), reason);
  }
});

test('a stale report is removed before the run', async () => {
  const dir = scratch();
  const reportPath = path.join(dir, 'conformance.json');
  fs.writeFileSync(reportPath, JSON.stringify(report()));
  let stale = true;
  const outcome = await run({
    reportPath,
    body: null,
    run: async ({ reportPath: where }) => {
      stale = fs.existsSync(where);
      fs.writeFileSync(where, JSON.stringify(report()));
      return { code: 0 };
    },
  });
  assert.equal(stale, false);
  assert.equal(outcome.code, 0);
});

test('the scratch root is removed after a red verdict', async () => {
  const outcome = await run({ exit: 1, body: report({ checks: rows(['fail']) }) });
  assert.equal(outcome.code, 1);
  assert.ok(outcome.seen.scratch, 'the launcher made no scratch root');
  assert.equal(fs.existsSync(outcome.seen.scratch), false);
});

test('the scratch root is removed after an infrastructure failure', async () => {
  let root = null;
  const outcome = await run({
    build: async ({ scratch: seen }) => {
      root = seen;
      fs.writeFileSync(path.join(seen, 'marker'), 'here');
      throw new Error('go build is not installed');
    },
  });
  assert.equal(outcome.code, 3);
  assert.match(outcome.log.lines.join('\n'), /go build is not installed/);
  assert.ok(root, 'the launcher made no scratch root');
  assert.equal(fs.existsSync(root), false);
});

test('a fixture that never prints READY exits 3 and leaves no scratch root', async () => {
  const dir = scratch();
  // A fixture that reads stdin to EOF and prints nothing: exactly what a server whose
  // registration panic ate its READY line looks like.
  const bin = path.join(dir, 'fixture');
  fs.writeFileSync(bin, '#!/bin/sh\ncat > /dev/null\n', { mode: 0o755 });
  const log = quiet();
  let root = null;
  const code = await main({
    cwd: REPO,
    env: { CONFORMANCE_REPORT: path.join(dir, 'conformance.json') },
    log,
    build: async ({ scratch: seen }) => {
      root = seen;
      return bin;
    },
    start: (options) => startFixture({ ...options, timeoutMs: 300 }),
    run: async () => {
      throw new Error('the package must not run without a host');
    },
  });
  assert.equal(code, 3);
  assert.match(log.lines.join('\n'), /printed no READY line within/);
  assert.ok(root, 'the launcher made no scratch root');
  assert.equal(fs.existsSync(root), false);
});

test('a READY record that omits a registered document is infrastructure', async () => {
  const dir = scratch();
  const bin = path.join(dir, 'fixture');
  const record = { baseURL: 'http://127.0.0.1:9', documents: { [PAGE]: { url: 'http://127.0.0.1:9/' + PAGE } } };
  fs.writeFileSync(
    bin,
    `#!${process.execPath}\nprocess.stdout.write('READY ' + ${JSON.stringify(JSON.stringify(record))} + '\\n');\nsetTimeout(() => process.exit(0), 100);\n`,
    { mode: 0o755 },
  );
  await assert.rejects(startFixture({ bin, root: dir, timeoutMs: 5000 }), new RegExp(PROBE));
});

// The fixture registers what --root holds when it boots, so the launcher places the frozen
// rows' documents first and holds the READY record to every name it placed. Here the fake
// fixture lists the root and drops one of them.
test('a READY record that omits a placed frozen document exits 3 naming it', async () => {
  const dir = scratch();
  const bin = path.join(dir, 'fixture');
  fs.writeFileSync(
    bin,
    [
      `#!${process.execPath}`,
      "const fs = require('node:fs');",
      'const argv = process.argv.slice(2);',
      "const root = argv[argv.indexOf('--root') + 1];",
      'const names = fs.readdirSync(root).sort();',
      "const omit = names.find((name) => name.startsWith('_frozen-'));",
      'const documents = {};',
      'for (const name of names) if (name !== omit) documents[name] = { url: `http://127.0.0.1:9/${name}` };',
      "fs.writeFileSync(`${process.argv[1]}.omitted`, String(omit));",
      "process.stdout.write(`READY ${JSON.stringify({ baseURL: 'http://127.0.0.1:9', documents })}\\n`);",
      'setTimeout(() => process.exit(0), 200);',
      '',
    ].join('\n'),
    { mode: 0o755 },
  );
  const log = quiet();
  const code = await main({
    cwd: REPO,
    env: { CONFORMANCE_REPORT: path.join(dir, 'conformance.json') },
    log,
    build: async ({ scratch: root }) => {
      assert.ok(fs.readdirSync(path.join(root, 'docs')).some((name) => name.startsWith('_frozen-')), 'the launcher placed no frozen document');
      return bin;
    },
    start: (options) => startFixture({ ...options, timeoutMs: 5000 }),
    run: async () => {
      throw new Error('the package must not run without a registered host');
    },
  });
  const omitted = fs.readFileSync(`${bin}.omitted`, 'utf8');
  assert.match(omitted, /^_frozen-.+\.html$/);
  assert.equal(code, 3);
  assert.ok(log.lines.join('\n').includes(omitted), `the verdict did not name ${omitted}`);
});

test('the package arguments are the fixed standalone capability set', () => {
  const args = packageArgs({ dir: DIR, baseURL: 'http://127.0.0.1:4321', root: '/tmp/docs', engine: 'all', reportPath: '/tmp/report.json' });
  const value = (flag) => args[args.indexOf(flag) + 1];
  assert.equal(value('--engine'), 'all');
  assert.equal(value('--host'), HOST);
  assert.equal(value('--page'), PAGE);
  assert.equal(value('--doc'), `/${PROBE}`);
  assert.equal(value('--require-extensions'), REQUIRED_EXTENSIONS.join(','));
  assert.equal(value('--report'), '/tmp/report.json');
  assert.ok(args.includes('--token-from-page'));
  assert.ok(args.includes('--keep'));
  const attached = packageArgs({ dir: DIR, baseURL: 'http://127.0.0.1:4321', root: '/tmp/home', engine: 'all', reportPath: '/tmp/report.json', page: 'docs/page.htmlclay', doc: '/docs/page.htmlclay', target: '/docs/page.htmlclay' });
  const attachedValue = (flag) => attached[attached.indexOf(flag) + 1];
  assert.equal(attachedValue('--page'), 'docs/page.htmlclay');
  assert.equal(attachedValue('--doc'), '/docs/page.htmlclay');
  assert.equal(attachedValue('--target'), '/docs/page.htmlclay');
  assert.deepEqual(
    REQUIRED_EXTENSIONS,
    ['conditional', 'data-guard-v1', 'data-read', 'data-write', 'page', 'page-awareness-v1', 'page-format-v1',
      'page-recovery-v1', 'receipts', 'record-awareness-v1', 'sync', 'sync-worker', 'upload', 'wire'],
  );
});

test('a verdict never prints a token-bearing route', () => {
  const token = 'abcdefghijklmnopqrstuvwxyz012345';
  assert.equal(scrub(`/_/save/${token}`), '/_/<route>/<token>');
  assert.equal(scrub(`<html savetoken="${token}">`), '<html savetoken="<token>">');
});

test('--engine and the attach options are the launcher\'s only arguments', async () => {
  const http = await run({ argv: ['--engine', 'http'] });
  assert.equal(http.code, 0);
  const bad = await run({ argv: ['--engine', 'browser'] });
  assert.equal(bad.code, 3);
  assert.match(bad.log.lines.join('\n'), /--engine must be one of all, http, page/);
  const incomplete = await run({ argv: ['--url', 'http://elsewhere'] });
  assert.equal(incomplete.code, 3);
  assert.match(incomplete.log.lines.join('\n'), /attach mode needs --root, --page, --doc/);
  const unknown = await run({ argv: ['--bogus'] });
  assert.equal(unknown.code, 3);
  assert.match(unknown.log.lines.join('\n'), /unknown argument --bogus/);
});

test('--engine page,http reaches the package unchanged', async () => {
  let seen = null;
  const outcome = await run({
    argv: ['--engine', 'page,http'],
    run: async ({ engine, reportPath }) => {
      seen = engine;
      fs.writeFileSync(reportPath, JSON.stringify(report()));
      return { code: 0 };
    },
  });
  assert.equal(outcome.code, 0);
  assert.equal(seen, 'page,http');
});

test('with no --engine the package gets all', async () => {
  let seen = null;
  const outcome = await run({
    run: async ({ engine, reportPath }) => {
      seen = engine;
      fs.writeFileSync(reportPath, JSON.stringify(report()));
      return { code: 0 };
    },
  });
  assert.equal(outcome.code, 0);
  assert.equal(seen, 'all');
});

test('--engine refuses a list the package would refuse', async () => {
  for (const value of ['page,bogus', '', 'page,page']) {
    const outcome = await run({ argv: ['--engine', value] });
    assert.equal(outcome.code, 3, `--engine ${JSON.stringify(value)} was accepted`);
    assert.match(outcome.log.lines.join('\n'), /--engine must be one of all, http, page, frozen/);
  }
});

// The frozen rows write their documents at the root of --root, which a host that serves
// only what it registered cannot answer, so an engine list without frozen places none.
test('an engine list without frozen places no frozen document', async () => {
  const dir = scratch();
  const log = quiet();
  let placed = null;
  const code = await main({
    cwd: REPO,
    env: { CONFORMANCE_REPORT: path.join(dir, 'conformance.json') },
    argv: ['--engine', 'page,http'],
    log,
    build: async ({ scratch: root }) => {
      placed = fs.readdirSync(path.join(root, 'docs')).filter((name) => name.startsWith('_frozen-'));
      return path.join(root, 'offline-fixture-server');
    },
    start: async ({ required }) => {
      assert.ok(!required.some((name) => name.startsWith('_frozen-')), 'the launcher required a frozen document');
      return { baseURL: 'http://127.0.0.1:1', stop: async () => {} };
    },
    run: async ({ reportPath }) => {
      fs.writeFileSync(reportPath, JSON.stringify(report()));
      return { code: 0 };
    },
  });
  assert.equal(code, 0);
  assert.deepEqual(placed, []);
});

test('attach mode uses the supplied host and skips fixture boot', async () => {
  const dir = scratch();
  const log = quiet();
  let seen = null;
  const code = await main({
    cwd: REPO,
    env: { CONFORMANCE_REPORT: path.join(dir, 'conformance.json') },
    argv: ['--url', 'http://127.0.0.1:9', '--root', '/tmp/home', '--page', 'docs/page.htmlclay',
      '--doc', '/docs/page.htmlclay', '--target', '/docs/page.htmlclay'],
    log,
    build: async () => { throw new Error('attach mode must not build'); },
    start: async () => { throw new Error('attach mode must not start a fixture'); },
    run: async (options) => {
      seen = options;
      fs.writeFileSync(options.reportPath, JSON.stringify(report()));
      return { code: 0 };
    },
  });
  assert.equal(code, 0);
  assert.equal(seen.baseURL, 'http://127.0.0.1:9');
  assert.equal(seen.root, '/tmp/home');
  assert.equal(seen.page, 'docs/page.htmlclay');
  assert.equal(seen.doc, '/docs/page.htmlclay');
  assert.equal(seen.target, '/docs/page.htmlclay');
});

test('verdict maps the package codes it is handed', () => {
  const dir = scratch();
  const reportPath = path.join(dir, 'conformance.json');
  const write = (body) => fs.writeFileSync(reportPath, JSON.stringify(body));
  write(report());
  assert.deepEqual(verdict({ reportPath, outcome: { code: 0 }, version: VERSION }), { code: 0, message: null });
  write(report({ checks: rows(['fail']) }));
  assert.equal(verdict({ reportPath, outcome: { code: 1 }, version: VERSION }).code, 1);
  write(report({ checks: rows(['na']) }));
  assert.equal(verdict({ reportPath, outcome: { code: 3 }, version: VERSION }).code, 3);
});

// The old gate drove this package from a nested Go test, switched on by an env var. It is
// gone, and `npm run conformance` is the only gate; a file that brings the env var back
// would make `go test ./...` depend on node, a browser and this package again.
test('the retired Go gate cannot come back', () => {
  const suite = path.join(REPO, 'internal');
  assert.equal(fs.existsSync(path.join(suite, 'server', 'hostgate_test.go')), false);
  const offenders = fs.readdirSync(suite, { recursive: true })
    .map((entry) => path.join(suite, entry))
    .filter((file) => fs.statSync(file).isFile())
    .filter((file) => fs.readFileSync(file, 'utf8').includes('HTMLCLAY_HOST_GATE'));
  assert.deepEqual(offenders, []);
});
