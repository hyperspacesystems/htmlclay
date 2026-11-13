import { spawn } from 'node:child_process';
import { createHash } from 'node:crypto';
import { existsSync, mkdirSync, readFileSync, rmSync, writeFileSync } from 'node:fs';
import path from 'node:path';
import { fileURLToPath, pathToFileURL } from 'node:url';
import { HOST, PACKAGE, parseEngine } from './conformance.mjs';

const HERE = path.dirname(fileURLToPath(import.meta.url));
const PLATFORM = /^[a-z0-9]+(?:-[a-z0-9]+)*$/;
const STATUSES = ['pass', 'fail', 'skip', 'na', 'waived', 'waiver-unused'];

export function parseCandidateArgs(argv) {
  let platform = '';
  const launcher = [];
  for (let i = 0; i < argv.length; i += 1) {
    const flag = argv[i];
    if (flag === '--platform') platform = argv[(i += 1)] ?? '';
    else if (flag.startsWith('--platform=')) platform = flag.slice('--platform='.length);
    else launcher.push(flag);
  }
  if (!platform) throw new Error('--platform is required');
  if (!PLATFORM.test(platform)) throw new Error(`--platform ${JSON.stringify(platform)} is not a label`);
  for (let i = 0; i < launcher.length; i += 1) {
    const flag = launcher[i];
    if (flag === '--engine') parseEngine(launcher[(i += 1)]);
    else if (flag.startsWith('--engine=')) parseEngine(flag.slice('--engine='.length));
  }
  return { platform, launcher };
}

export const evidencePathFor = (platform) => path.join('.conformance', `conformance-${platform}.json`);

export function canonical(value) {
  if (Array.isArray(value)) return `[${value.map(canonical).join(',')}]`;
  if (value && typeof value === 'object') {
    return `{${Object.keys(value).sort()
      .map((key) => `${JSON.stringify(key)}:${canonical(value[key])}`)
      .join(',')}}`;
  }
  return JSON.stringify(value);
}

function sha256(bytes) {
  return createHash('sha256').update(bytes).digest('hex');
}

function parseJson(bytes, name) {
  try {
    return JSON.parse(Buffer.isBuffer(bytes) ? bytes.toString('utf8') : bytes);
  } catch (error) {
    throw new Error(`${name} is not JSON: ${error.message}`);
  }
}

function contextFile(cwd, name) {
  try {
    return readFileSync(path.join(cwd, name));
  } catch {
    throw new Error(`the verified candidate context is missing ${name}`);
  }
}

export function validateCandidateContext({ cwd, env = process.env, platform }) {
  const candidate = contextFile(cwd, 'candidate.json');
  const record = parseJson(contextFile(cwd, 'platform-candidate.json'), 'platform-candidate.json');
  const manifest = record?.manifest;
  if (!manifest || typeof manifest !== 'object' || Array.isArray(manifest)) {
    throw new Error('platform-candidate.json names no manifest');
  }
  if (typeof manifest.app !== 'string' || !manifest.app) throw new Error('the candidate manifest names no app');
  if (env.APP && env.APP !== manifest.app) {
    throw new Error(`the candidate manifest belongs to ${manifest.app}, not ${env.APP}`);
  }
  if (typeof manifest.version !== 'string' || !/^[0-9]+\.[0-9]+\.[0-9]+$/.test(manifest.version)) {
    throw new Error('the candidate manifest version is not X.Y.Z');
  }
  if (typeof manifest.workflow_sha !== 'string' || !/^[0-9a-f]{40}$/.test(manifest.workflow_sha)) {
    throw new Error('the candidate manifest workflow_sha is not a full commit id');
  }
  if (typeof manifest.candidate_sha256 !== 'string' || !/^[0-9a-f]{64}$/.test(manifest.candidate_sha256)) {
    throw new Error('the candidate manifest candidate_sha256 is not a digest');
  }
  if (sha256(candidate) !== manifest.candidate_sha256) {
    throw new Error('candidate.json does not match the manifest candidate_sha256');
  }
  if (typeof record.source_sha !== 'string' || !/^[0-9a-f]{40}$/.test(record.source_sha)) {
    throw new Error('platform-candidate.json source_sha is not a full commit id');
  }
  if (!existsSync(path.join(cwd, 'go.mod'))) throw new Error('the verified candidate has no go.mod at its root');
  if (!Array.isArray(manifest.artifacts) || !manifest.artifacts.includes(`conformance-${platform}`)) {
    throw new Error(`the candidate manifest declares no conformance-${platform} artifact`);
  }
  const identity = sha256(Buffer.from(canonical(manifest)));
  const expected = {
    attempt: env.ATTEMPT,
    candidate_sha256: env.CANDIDATE_SHA256,
    workflow_sha: env.WORKFLOW_SHA,
    manifest_identity: env.MANIFEST_IDENTITY,
  };
  for (const [field, value] of Object.entries(expected)) {
    if (!value) throw new Error(`the verified dispatch supplies no ${field.toUpperCase()}`);
    const actual = field === 'manifest_identity' ? identity : manifest[field];
    if (actual !== value) throw new Error(`the candidate manifest ${field} does not match the dispatch`);
  }
  return {
    app: manifest.app,
    platform,
    version: manifest.version,
    workflow_sha: manifest.workflow_sha,
    candidate_sha256: manifest.candidate_sha256,
    attempt: manifest.attempt,
    manifest_identity: identity,
  };
}

export function validateReport(reportPath) {
  let report;
  try {
    report = parseJson(readFileSync(reportPath), reportPath);
  } catch (error) {
    throw new Error(`the conformance report is unusable: ${error.message}`);
  }
  if (report?.schema !== 1) throw new Error('the conformance report names no schema 1');
  if (report.package !== PACKAGE) throw new Error(`the conformance report names package ${JSON.stringify(report.package ?? null)}`);
  if (report.host !== HOST) throw new Error(`the conformance report names host ${JSON.stringify(report.host ?? null)}`);
  if (typeof report.version !== 'string' || !/^[0-9]+\.[0-9]+\.[0-9]+$/.test(report.version)) {
    throw new Error('the conformance report names no package version');
  }
  if (!Array.isArray(report.checks) || !report.checks.length) throw new Error('the conformance report holds no checks');
  const ids = report.checks.map((check) => check?.id);
  if (ids.some((id) => typeof id !== 'string' || !id) || new Set(ids).size !== ids.length) {
    throw new Error('the conformance report needs nonempty unique row ids');
  }
  for (const check of report.checks) {
    if (!STATUSES.includes(check?.status)) {
      throw new Error(`the conformance report gives row ${check?.id} the unknown status ${JSON.stringify(check?.status ?? null)}`);
    }
    if (check.status === 'na' && check.blocking === true) throw new Error(`the conformance report leaves row ${check.id} unanswered`);
  }
  return report;
}

export function verdictExit(report, code) {
  const failed = report.checks.filter((check) => check.status === 'fail').length;
  if (code === 0 && failed) throw new Error(`the conformance launcher exited 0 with ${failed} failed row(s)`);
  if (code === 1 && !failed) throw new Error('the conformance launcher exited 1 without a failed row');
  return code;
}

function runLauncher({ launcher, args, cwd, env }) {
  const child = spawn(process.execPath, [launcher, ...args], {
    cwd,
    env,
    stdio: ['ignore', 'inherit', 'inherit'],
  });
  return new Promise((resolve) => {
    child.on('error', (error) => resolve({ error }));
    child.on('exit', (code, signal) => resolve({ code: code ?? null, signal }));
  });
}

function failure(log, message) {
  log.error(`[conformance-candidate] ${message}`);
  return 3;
}

export async function main({
  env = process.env,
  cwd = process.cwd(),
  argv = process.argv.slice(2),
  log = console,
  run = runLauncher,
} = {}) {
  let parsed;
  let context;
  try {
    parsed = parseCandidateArgs(argv);
    context = validateCandidateContext({ cwd, env, platform: parsed.platform });
  } catch (error) {
    return failure(log, error.message);
  }

  const reportPath = path.resolve(
    cwd, env.CONFORMANCE_REPORT || path.join('.conformance', `${parsed.platform}-report.json`),
  );
  const evidencePath = path.resolve(
    cwd, env.CONFORMANCE_EVIDENCE || evidencePathFor(parsed.platform),
  );
  try {
    mkdirSync(path.dirname(reportPath), { recursive: true });
    rmSync(reportPath, { force: true });
    rmSync(evidencePath, { force: true });

    const launcher = env.CONFORMANCE_LAUNCHER || path.join(HERE, 'conformance.mjs');
    const outcome = await run({
      launcher,
      args: parsed.launcher,
      cwd,
      env: { ...env, CONFORMANCE_REPORT: reportPath },
    });
    if (outcome?.error) throw new Error(`the conformance launcher could not start: ${outcome.error.message}`);
    if (outcome?.signal) throw new Error(`the conformance launcher was killed by ${outcome.signal}`);
    if (![0, 1].includes(outcome?.code)) {
      throw new Error(`the conformance launcher exited ${outcome?.code ?? '(none)'}, which is not a verdict`);
    }
    const report = validateReport(reportPath);
    const exitCode = verdictExit(report, outcome.code);
    const envelope = {
      schema: 1,
      app: context.app,
      platform: context.platform,
      workflow_sha: context.workflow_sha,
      candidate_sha256: context.candidate_sha256,
      attempt: context.attempt,
      manifest_identity: context.manifest_identity,
      exitCode,
      report,
    };
    mkdirSync(path.dirname(evidencePath), { recursive: true });
    writeFileSync(evidencePath, JSON.stringify(envelope, null, 2) + '\n');
    log.log(`[conformance-candidate] ${context.platform} exited ${exitCode}; evidence at ${evidencePath}`);
    return 0;
  } catch (error) {
    return failure(log, error?.message || String(error));
  }
}

const invoked = process.argv[1] !== undefined && import.meta.url === pathToFileURL(process.argv[1]).href;
if (invoked) process.exitCode = await main();
