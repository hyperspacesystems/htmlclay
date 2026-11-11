// HTML Clay's standalone conformance gate. It boots this repo's own fixture server on a
// scratch document root, hands that host to the installed @panphora/host-conformance
// package, and answers with the package's exit codes: 0 nothing failed, 1 a check
// failed, 3 the run could not answer. 3 is also this launcher's answer for a setup step
// that never reached a verdict, so a caller can tell "the host failed" from "the gate
// could not run".
import { spawn } from 'node:child_process';
import { copyFileSync, mkdirSync, mkdtempSync, readFileSync, rmSync, writeFileSync } from 'node:fs';
import { createRequire } from 'node:module';
import os from 'node:os';
import path from 'node:path';
import { pathToFileURL } from 'node:url';

export const HOST = 'htmlclay';
export const PACKAGE = '@panphora/host-conformance';
export const PAGE = '_mhf-host-test.htmlclay';
export const PROBE = '_conformance-probe.html';
export const REPORT_DEFAULT = '.conformance/latest.json';
export const READY_TIMEOUT_MS = 20000;
export const BUILD_TIMEOUT_MS = 300000;
// `--engine` takes `all`, which is every engine, or a comma list of the engines the package
// can run on its own. The package parses that list, so the launcher forwards the string
// untouched and only refuses what the package would refuse.
export const ENGINES = ['all', 'http', 'page', 'frozen'];
const ENGINE_LIST = ['http', 'page', 'frozen'];

export function parseEngine(value) {
  const engines = typeof value === 'string' ? value.split(',') : [];
  const listed = engines.length > 0
    && engines.every((engine) => ENGINE_LIST.includes(engine))
    && new Set(engines).size === engines.length;
  if (!listed && !(engines.length === 1 && engines[0] === 'all')) {
    throw new Error(`--engine must be one of ${ENGINES.join(', ')}`);
  }
  return engines;
}

// The frozen rows write their documents at the root of --root and navigate to them, which
// a host that serves only what it registered cannot answer, so a run that leaves frozen out
// must not place them either.
export const runsFrozen = (engine) => engine === 'all' || parseEngine(engine).includes('frozen');

// The standalone-server capability set. `people` is absent on purpose: it needs the
// app's profile hooks, which a fixture server does not install, so asserting it would
// demand a capability this host only has inside the app. `blockeditor-format-v1` is announced
// because the vendored runtime carries page-format.js.
export const REQUIRED_EXTENSIONS = [
  'blockeditor', 'blockeditor-awareness-v1', 'blockeditor-format-v1', 'blockeditor-recovery-v1', 'conditional', 'data-guard-v1',
  'data-read', 'data-write', 'receipts', 'record-awareness-v1', 'sync', 'sync-worker', 'upload', 'wire',
];

const PROBE_TITLE = 'Conformance probe · HTML Clay';
const PROBE_HTML = [
  '<!DOCTYPE html>',
  '<html lang="en">',
  `<head><meta charset="utf-8"><title>${PROBE_TITLE}</title></head>`,
  '<body>',
  `<h1>${PROBE_TITLE}</h1>`,
  '<p>Written by HTML Clay\u2019s conformance gate; the suite overwrites it on every run.</p>',
  '</body>',
  '</html>',
  '',
].join('\n');

// A save token is a credential: it is all a caller needs to write the document, and the
// fixture server puts one in every registered file's save URL. Nothing this launcher
// prints may carry one.
const TOKEN_ROUTE = /\/_\/[A-Za-z0-9-]+\/[A-Za-z0-9_-]{16,}/g;
const TOKEN_ATTR = /\b(savetoken|htmlclaytoken)(\s*=\s*)(?:"[^"]*"|'[^']*'|[^\s>]+)/gi;

export function scrub(text) {
  return String(text)
    .replace(TOKEN_ROUTE, '/_/<route>/<token>')
    .replace(TOKEN_ATTR, (_, name, eq) => `${name}${eq}"<token>"`);
}

// Every guard failure below is this: the gate never reached a verdict.
class Infrastructure extends Error {}

const remaining = (deadline) => Math.max(deadline - Date.now(), 0);

function wait(ms) {
  return new Promise((resolve) => setTimeout(resolve, ms));
}

export function packageDir(cwd) {
  const require = createRequire(path.join(cwd, 'package.json'));
  return path.dirname(require.resolve(`${PACKAGE}/package.json`));
}

function packageVersion(dir) {
  return JSON.parse(readFileSync(path.join(dir, 'package.json'), 'utf8')).version;
}

// This host registers what --root holds when it boots, so the frozen rows' documents have
// to be on disk before the fixture starts. Placing them is the package's own work: the
// launcher only has to ask for it early, and then hold the fixture to the names it placed.
async function placeFrozen(root, cwd) {
  const entry = createRequire(path.join(cwd, 'package.json')).resolve(PACKAGE);
  const { placeFrozenDocuments } = await import(pathToFileURL(entry).href);
  return placeFrozenDocuments(root);
}

export function parseArgs(argv) {
  const attach = {};
  let engine = 'all';
  for (let i = 0; i < argv.length; i += 1) {
    const flag = argv[i];
    const [name, inline] = flag.startsWith('--') && flag.includes('=')
      ? [flag.slice(0, flag.indexOf('=')), flag.slice(flag.indexOf('=') + 1)]
      : [flag, undefined];
    if (name === '--engine') {
      const value = inline ?? argv[(i += 1)];
      parseEngine(value);
      engine = value;
      continue;
    }
    if (name === '--url' || name === '--root' || name === '--page' || name === '--doc' || name === '--target') {
      const value = inline ?? argv[(i += 1)];
      if (typeof value !== 'string' || !value) throw new Error(`${name} needs a value`);
      attach[name.slice(2)] = value;
      continue;
    }
    throw new Error(`unknown argument ${flag} (this gate takes --engine ${ENGINES.join('|')} or attach options --url, --root, --page, --doc, --target)`);
  }
  if (!Object.keys(attach).length) return { engine, attach: null };
  const missing = ['url', 'root', 'page', 'doc'].filter((name) => !attach[name]);
  if (missing.length) throw new Error(`attach mode needs ${missing.map((name) => `--${name}`).join(', ')}`);
  return { engine, attach };
}

// CGO off for every build: the binary runs on a scratch machine, not necessarily this
// one, and a cgo build would tie it to the host's toolchain.
export function buildFixture({ cwd, scratch, env = process.env, timeoutMs = BUILD_TIMEOUT_MS } = {}) {
  const name = process.platform === 'win32' ? 'offline-fixture-server.exe' : 'offline-fixture-server';
  const binary = path.join(scratch, name);
  const child = spawn('go', ['build', '-o', binary, './cmd/offline-fixture-server'], {
    cwd,
    env: { ...env, CGO_ENABLED: '0' },
    stdio: ['ignore', 'ignore', 'pipe'],
  });
  let stderr = '';
  child.stderr.setEncoding('utf8');
  child.stderr.on('data', (chunk) => { stderr = (stderr + chunk).slice(-4000); });
  return new Promise((resolve, reject) => {
    const timer = setTimeout(() => {
      child.kill('SIGKILL');
      reject(new Infrastructure(`go build did not finish within ${timeoutMs}ms`));
    }, timeoutMs);
    child.on('error', (error) => {
      clearTimeout(timer);
      reject(new Infrastructure(`go build could not start: ${error.message}`));
    });
    child.on('exit', (code, signal) => {
      clearTimeout(timer);
      if (code === 0) resolve(binary);
      else reject(new Infrastructure(`go build failed (${signal ?? `exit ${code}`}): ${stderr.trim()}`));
    });
  });
}

function readReady(child, timeoutMs) {
  return new Promise((resolve, reject) => {
    let timer;
    let buffer = '';
    let settled = false;
    const finish = (fn, value) => {
      if (settled) return;
      settled = true;
      clearTimeout(timer);
      child.stdout?.off('data', onData);
      child.off('exit', onExit);
      child.off('error', onError);
      fn(value);
    };
    const onData = (chunk) => {
      buffer += String(chunk);
      const lines = buffer.split('\n');
      buffer = lines.pop();
      for (const line of lines) {
        if (!line.startsWith('READY ')) continue;
        let record;
        try {
          record = JSON.parse(line.slice('READY '.length));
        } catch (error) {
          finish(reject, new Infrastructure(`the fixture server's READY line is not JSON: ${error.message}`));
          return;
        }
        finish(resolve, record);
        return;
      }
    };
    const onExit = (code, signal) => finish(
      reject,
      new Infrastructure(`the fixture server stopped before printing READY (${signal ?? `exit ${code}`})`),
    );
    const onError = (error) => finish(reject, new Infrastructure(`the fixture server could not start: ${error.message}`));
    timer = setTimeout(
      () => finish(reject, new Infrastructure(`the fixture server printed no READY line within ${timeoutMs}ms`)),
      timeoutMs,
    );
    child.stdout?.on('data', onData);
    child.once('exit', onExit);
    child.once('error', onError);
  });
}

function assertDocuments(record, required) {
  let baseURL;
  try {
    baseURL = new URL(record?.baseURL);
  } catch {
    throw new Infrastructure('the READY record names no base URL');
  }
  if (baseURL.protocol !== 'http:') throw new Infrastructure(`the READY record names ${baseURL.protocol}, not http`);
  const documents = record?.documents;
  if (!documents || typeof documents !== 'object') throw new Infrastructure('the READY record names no documents');
  const missing = required.filter((name) => typeof documents[name]?.url !== 'string');
  if (missing.length) throw new Infrastructure(`the READY record does not name ${missing.join(', ')}`);
  return baseURL.href.replace(/\/+$/, '');
}

async function waitForHttp(baseURL, timeoutMs, request) {
  const deadline = Date.now() + timeoutMs;
  const url = new URL(PROBE, `${baseURL}/`).href;
  let last = 'no answer';
  for (;;) {
    try {
      const response = await request(url, { signal: AbortSignal.timeout(Math.max(remaining(deadline), 1)) });
      if (response.ok) return;
      last = `HTTP ${response.status}`;
    } catch (error) {
      last = error?.message || String(error);
    }
    if (remaining(deadline) <= 0) throw new Infrastructure(`the fixture server never served ${url}: ${last}`);
    await wait(100);
  }
}

// One valid READY record plus a served probe document, both inside the same deadline: a
// listener that printed its line but never answers is not a host the package can read.
export async function waitForReady(child, { timeoutMs = READY_TIMEOUT_MS, required = [PAGE, PROBE], request = fetch } = {}) {
  const deadline = Date.now() + timeoutMs;
  const record = await readReady(child, remaining(deadline));
  const baseURL = assertDocuments(record, required);
  await waitForHttp(baseURL, remaining(deadline), request);
  return { baseURL, documents: record.documents };
}

function stopChild(child, { graceMs = 10000 } = {}) {
  return new Promise((resolve) => {
    if (child.exitCode !== null || child.signalCode !== null) {
      resolve();
      return;
    }
    const timer = setTimeout(() => child.kill('SIGKILL'), graceMs);
    child.once('exit', () => {
      clearTimeout(timer);
      resolve();
    });
    child.on('error', () => {});
    child.stdin?.end();
  });
}

export function startFixture({ bin, root, env = process.env, timeoutMs = READY_TIMEOUT_MS, required } = {}) {
  const child = spawn(bin, ['--root', root], { env, stdio: ['pipe', 'pipe', 'pipe'] });
  let stderr = '';
  child.stderr.setEncoding('utf8');
  child.stderr.on('data', (chunk) => { stderr = (stderr + chunk).slice(-4000); });
  const stop = () => stopChild(child);
  return waitForReady(child, { timeoutMs, required }).then(
    ({ baseURL }) => ({ baseURL, child, stop }),
    async (error) => {
      await stop();
      const detail = stderr.trim();
      throw detail ? new Infrastructure(`${error.message}: ${detail}`) : error;
    },
  );
}

export function packageArgs({
  dir, baseURL, root, engine, reportPath, page = PAGE, doc = `/${PROBE}`, target = '',
}) {
  const args = [
    path.join(dir, 'bin', 'host-conformance.js'),
    '--engine', engine,
    '--host', HOST,
    '--url', baseURL,
    '--root', root,
    '--page', page,
    '--doc', doc,
    '--token-from-page',
    '--keep',
    '--require-extensions', REQUIRED_EXTENSIONS.join(','),
    '--report', reportPath,
  ];
  if (target) args.splice(args.indexOf('--doc') + 2, 0, '--target', target);
  return args;
}

// cwd is the host repo, not this script's directory: the package resolves playwright from
// the caller's directory, and playwright is installed here.
export function runPackage({
  cwd, env = process.env, dir, baseURL, root, engine, reportPath, page, doc, target,
}) {
  const child = spawn(process.execPath, packageArgs({
    dir, baseURL, root, engine, reportPath, page, doc, target,
  }), {
    cwd,
    env,
    stdio: ['ignore', 'inherit', 'inherit'],
  });
  return new Promise((resolve, reject) => {
    child.on('error', (error) => reject(new Infrastructure(`the conformance package could not start: ${error.message}`)));
    child.on('exit', (code, signal) => resolve(signal ? { signal } : { code: code ?? 3 }));
  });
}

function readReport(reportPath) {
  let text;
  try {
    text = readFileSync(reportPath, 'utf8');
  } catch {
    throw new Infrastructure(`the conformance package wrote no report at ${reportPath}`);
  }
  try {
    return JSON.parse(text);
  } catch (error) {
    throw new Infrastructure(`the report at ${reportPath} is not JSON: ${error.message}`);
  }
}

function reportProblem(report, version) {
  if (report?.schema !== 1) return `names schema ${JSON.stringify(report?.schema ?? null)}, not 1`;
  if (report.package !== PACKAGE) return `names package ${JSON.stringify(report.package ?? null)}, not ${PACKAGE}`;
  if (report.host !== HOST) return `names host ${JSON.stringify(report.host ?? null)}, not ${HOST}`;
  if (report.version !== version) return `names package version ${JSON.stringify(report.version ?? null)}, not the installed ${version}`;
  if (!Array.isArray(report.checks) || !report.checks.length) return 'holds no checks';
  return null;
}

function tally(checks) {
  let fail = 0;
  let unanswered = 0;
  for (const check of checks) {
    if (check?.status === 'fail') fail += 1;
    if (check?.status === 'na' && check.blocking) unanswered += 1;
  }
  return { fail, unanswered };
}

// The package's exit code is the contract, and the report is what a person reads. They
// have to agree: a report that disagrees with the exit is a harness fault, not a verdict.
export function verdict({ reportPath, outcome, version }) {
  if (outcome?.signal) return { code: 3, message: `the conformance package was killed by ${outcome.signal}` };
  const exit = outcome?.code;
  if (![0, 1, 3].includes(exit)) {
    return { code: 3, message: `the conformance package exited ${exit ?? '(none)'}, which is not a verdict` };
  }
  const report = readReport(reportPath);
  const problem = reportProblem(report, version);
  if (problem) return { code: 3, message: `the report at ${reportPath} ${problem}` };

  const { fail, unanswered } = tally(report.checks);
  if (exit === 3) return { code: 3, message: `the run could not answer: ${unanswered} unanswered check(s), see ${reportPath}` };
  if (exit === 1) {
    if (!fail) return { code: 3, message: `the package exited 1 but ${reportPath} names no failed check` };
    return { code: 1, message: null };
  }
  if (fail || unanswered) {
    return { code: 3, message: `the package exited 0 but ${reportPath} names ${fail} failed and ${unanswered} unanswered check(s)` };
  }
  return { code: 0, message: null };
}

export async function main({
  env = process.env,
  cwd = process.cwd(),
  argv = process.argv.slice(2),
  log = console,
  build = buildFixture,
  start = startFixture,
  run = runPackage,
} = {}) {
  let options;
  try {
    options = parseArgs(argv);
  } catch (error) {
    log.error(`[conformance] ${error.message}`);
    return 3;
  }
  const { engine, attach } = options;

  let scratch = '';
  let fixture = null;
  let code = 3;
  try {
    const reportPath = path.resolve(cwd, env.CONFORMANCE_REPORT || REPORT_DEFAULT);
    mkdirSync(path.dirname(reportPath), { recursive: true });
    rmSync(reportPath, { force: true });

    const dir = packageDir(cwd);
    let root;
    let baseURL;
    if (attach) {
      root = attach.root;
      baseURL = attach.url;
    } else {
      scratch = mkdtempSync(path.join(os.tmpdir(), 'htmlclay-conformance-'));
      const docs = path.join(scratch, 'docs');
      mkdirSync(docs, { recursive: true });
      copyFileSync(path.join(dir, 'page', 'host-test.html'), path.join(docs, PAGE));
      writeFileSync(path.join(docs, PROBE), PROBE_HTML);
      const frozen = runsFrozen(engine) ? await placeFrozen(docs, cwd) : [];
      const required = [PAGE, PROBE, ...new Set(frozen.map((row) => row.name))];
      const bin = await build({ cwd, scratch, env });
      fixture = await start({ bin, root: docs, env, required });
      root = docs;
      baseURL = fixture.baseURL;
    }

    const outcome = await run({
      cwd, env, dir, baseURL, root, engine, reportPath,
      page: attach?.page, doc: attach?.doc, target: attach?.target,
    });
    const answer = verdict({ reportPath, outcome, version: packageVersion(dir) });
    if (answer.message) log.error(`[conformance] ${scrub(answer.message)}`);
    code = answer.code;
  } catch (error) {
    log.error(`[conformance] ${scrub(error?.message || String(error))}`);
    code = 3;
  } finally {
    // Teardown cannot change a verdict, and the scratch root is this run's alone: the
    // host repo, the report and the package install are all outside it.
    try {
      if (fixture?.stop) await fixture.stop();
    } catch {
      // stop() resolves once the child is gone; a teardown error cannot change a verdict.
    }
    if (scratch) rmSync(scratch, { recursive: true, force: true });
  }
  return code;
}

const invoked = process.argv[1] !== undefined && import.meta.url === pathToFileURL(process.argv[1]).href;
if (invoked) process.exitCode = await main();
