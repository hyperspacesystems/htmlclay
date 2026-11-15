// 08-sync.md step 8.27: one desktop of the product, the real `htmlclay` started
// the way the Mac app starts it (--shell-stdin), for a round-trip test to drive.
// It is the product's twin of hyperclay-local/tests/round-trip/desktop-driver.cjs:
// forked as a child, it takes { id, cmd, args } over IPC and answers
// { id, ok, result } or { id, ok: false, error }.
//
// Every command goes through the shell protocol the Mac app speaks, except what
// a person would see on disk: checksums walk the folder, and conflicts reads the
// held conflict files Go itself reads (cmd/htmlclay/sync_conflicts.go).
//
// PRODUCT_SYNC_BREAK=keep-theirs makes resolve with keep "theirs" do nothing, so
// a round-trip run can prove its Keep theirs scenario fails when the choice is lost.
import { execFileSync } from 'node:child_process';
import { createHash } from 'node:crypto';
import { existsSync, mkdirSync, readFileSync, readdirSync, writeFileSync } from 'node:fs';
import path from 'node:path';
import { freePort, shellRequest, startShellHost } from './served-product.mjs';

const wait = (ms) => new Promise((resolve) => setTimeout(resolve, ms));

// Local's shouldSkipEntry (src/sync-engine/file-operations.js): what sync never
// carries, so a checksum listing leaves it out on both sides.
const skipped = (name) => name.startsWith('.') || name === 'node_modules' || name === 'sites-versions' || name === 'tailwindcss';

let world = null;
let host = null;

function configDir() {
  return path.join(world.configBase, 'htmlclay');
}

function environment() {
  // A synctest build's release check goes to a closed local port instead of htmlclay.com.
  const env = { ...process.env, HTMLCLAY_CONFIG_BASE: world.configBase, HTMLCLAY_SECRET_STORE: 'file', HTMLCLAY_RELEASE_INFO_URL: 'http://127.0.0.1:1/' };
  if (world.syncServer) env.HTMLCLAY_SYNC_SERVER = world.syncServer;
  if (world.serviceCmd) env.HTMLCLAY_SERVICE_CMD = world.serviceCmd;
  else delete env.HTMLCLAY_SERVICE_CMD;
  return env;
}

async function boot() {
  host = await startShellHost({ bin: world.bin, cwd: world.configBase, env: environment(), log: world.log });
}

async function snapshot() {
  const answer = await shellRequest(host, 'GET', '/shell/v1/snapshot');
  if (answer.status !== 200) throw new Error(`snapshot answered ${answer.status}`);
  return JSON.parse(answer.body);
}

let actions = 0;
async function act(type, args) {
  actions += 1;
  const answer = await shellRequest(host, 'POST', '/shell/v1/actions', { id: `driver-${process.pid}-${actions}`, type, args });
  const body = answer.body ? JSON.parse(answer.body) : {};
  if (body.ok) return body;
  const error = new Error(`${type} was refused: ${body.error?.code ?? answer.status}`);
  error.code = body.error?.code ?? `status-${answer.status}`;
  throw error;
}

async function until(what, check, timeoutMs) {
  const deadline = Date.now() + timeoutMs;
  for (;;) {
    const value = await check();
    if (value) return value;
    if (Date.now() > deadline) throw Object.assign(new Error(`timed out waiting for ${what}`), { code: 'timeout' });
    await wait(250);
  }
}

function syncRecord() {
  const file = path.join(configDir(), 'sync.json');
  return existsSync(file) ? JSON.parse(readFileSync(file, 'utf8')) : { syncSessions: [] };
}

// The live-room service is the one process started with this desktop's data dir.
function servicePid() {
  const marker = `--data-dir ${path.join(configDir(), 'service')}`;
  const lines = execFileSync('ps', ['-A', '-o', 'pid=,command='], { encoding: 'utf8' }).split('\n');
  const line = lines.find((entry) => entry.includes(marker) && !entry.includes('ps -A'));
  return line ? Number(line.trim().split(/\s+/)[0]) : null;
}

function listFiles(folder) {
  const sums = {};
  const walk = (dir, rel) => {
    for (const entry of readdirSync(dir, { withFileTypes: true })) {
      if (skipped(entry.name)) continue;
      const childRel = rel ? `${rel}/${entry.name}` : entry.name;
      const full = path.join(dir, entry.name);
      if (entry.isDirectory()) walk(full, childRel);
      else if (entry.isFile()) sums[childRel] = createHash('sha256').update(readFileSync(full)).digest('hex');
    }
  };
  walk(folder, '');
  return sums;
}

const handlers = {
  // bin, configBase (an empty folder), log; syncServer and serviceCmd optional.
  async start({ bin, configBase, log, syncServer = null, serviceCmd = null }) {
    world = { bin, configBase, log, syncServer, serviceCmd };
    mkdirSync(configDir(), { recursive: true });
    const config = path.join(configDir(), 'config.json');
    if (!existsSync(config)) {
      const port = await freePort();
      writeFileSync(config, `${JSON.stringify({ startOnLogin: false, port, updates: 'off', servedFolders: [] }, null, 2)}\n`);
    }
    await boot();
    return { pid: host.child.pid, port: host.ready.port };
  },
  async connect({ key, username = '' }) {
    await act('sync.connect', { key, username });
    const snap = await snapshot();
    return { connected: snap.sync.connected, username: snap.sync.username };
  },
  // The account's folder row (a team's name or the person's own username) set up
  // at folder. existing: sync into files already there (own account only).
  async setUp({ account, folder, existing = false, timeoutMs = 30000 }) {
    // A team row names the team in origin.team; the person's own row names them
    // in origin.username (cmd/htmlclay/sync.go, the account rows).
    const row = await until(`the folder row for ${account}`, async () => (await snapshot()).folders.find((f) =>
      ((f.origin.kind === 'team' && f.origin.team === account) || (f.origin.kind === 'account' && f.origin.username === account)) &&
      f.state === 'not-on-this-computer'), timeoutMs);
    mkdirSync(folder, { recursive: true });
    await act('folder.setUpTeam', { id: row.id, path: folder, existing, confirmed: true });
    return { folderID: row.id };
  },
  // Every synced folder up to date, holding conflicts or paused, with nothing
  // syncing, twice in a row a quarter second apart. A live edit does not move a folder's
  // last finished time (that is the engine's last full pass), so waiting for a
  // change to arrive is landed's job.
  async idle({ timeoutMs = 60000 } = {}) {
    let quiet = 0;
    return until('every synced folder to settle', async () => {
      const snap = await snapshot();
      const synced = snap.folders.filter((f) => f.synced);
      const settled = synced.length > 0 && snap.sync.syncingFiles === 0 &&
        synced.every((f) => ['up-to-date', 'conflicts', 'paused'].includes(f.sync.state));
      quiet = settled ? quiet + 1 : 0;
      return quiet >= 2 ? { folders: synced.map((f) => ({ id: f.id, state: f.sync.state, lastFinishedAt: f.sync.lastFinishedAt })) } : null;
    }, timeoutMs);
  },
  // Waits until each named file in folder has the given sha256, or is absent for
  // null, then answers the folder's checksums.
  async landed({ folder, files, timeoutMs = 60000 }) {
    return until(`${Object.keys(files).join(', ')} to land`, async () => {
      const sums = listFiles(folder);
      return Object.entries(files).every(([rel, want]) => (want === null ? !(rel in sums) : sums[rel] === want)) ? sums : null;
    }, timeoutMs);
  },
  async snapshot() {
    return snapshot();
  },
  async checksums({ folder }) {
    return listFiles(folder);
  },
  // Every held conflict, with the id Go gives it and the folder it is in.
  async conflicts() {
    const record = syncRecord();
    const held = [];
    for (const session of record.syncSessions || []) {
      const dir = session.legacyMetaDir || path.join(configDir(), 'service', 'sync-meta', 'v2', session.id);
      const file = path.join(dir, 'conflicts.json');
      if (!existsSync(file)) continue;
      const records = JSON.parse(readFileSync(file, 'utf8'));
      for (const key of Object.keys(records).sort()) {
        const r = records[key];
        held.push({ id: `${session.id}:${key}:${r.detectedAt}`, folderID: session.rootId, key, path: r.path, kind: r.kind, remoteCopy: r.remoteCopy || null });
      }
    }
    return held;
  },
  // A conflict from conflicts() and the choice: mine, theirs or both.
  async resolve({ conflict, keep }) {
    if (keep === 'theirs' && process.env.PRODUCT_SYNC_BREAK === 'keep-theirs') return { broken: true };
    await act('conflict.resolve', { id: conflict.id, path: conflict.path, keep });
    return { resolved: true };
  },
  // Turns one folder's sync off or on (folder.setSynced); on answers once the
  // live-room service the folder syncs in runs.
  async setSynced({ folderID, on, timeoutMs = 30000 }) {
    await act('folder.setSynced', { id: folderID, on });
    if (on) await until('the service to run for the folder', async () => (await snapshot()).host.service === 'running', timeoutMs);
    return { synced: (await snapshot()).folders.find((f) => f.id === folderID)?.sync?.state !== 'off' };
  },
  async pause({ paused = true } = {}) {
    await act('sync.setPaused', { paused });
    return { paused: (await snapshot()).sync.paused };
  },
  // Kills the live-room service the way a crash would; Go starts it again.
  async restartService({ timeoutMs = 30000 } = {}) {
    const before = servicePid();
    if (!before) throw new Error('no live-room service is running for this desktop');
    process.kill(before, 'SIGKILL');
    const after = await until('the service to start again', async () => {
      const pid = servicePid();
      const snap = await snapshot();
      return pid && pid !== before && snap.host.service === 'running' ? pid : null;
    }, timeoutMs);
    return { before, after };
  },
  // Kills the host the way a crash would and starts it again on the same config;
  // answers once its live-room service runs again, if a folder syncs.
  async restartHost({ timeoutMs = 30000 } = {}) {
    const before = host.child.pid;
    host.child.kill('SIGKILL');
    await new Promise((resolve) => (host.child.exitCode !== null || host.child.signalCode !== null ? resolve() : host.child.once('exit', resolve)));
    const stray = servicePid();
    if (stray) await until('the orphaned service to exit', async () => !servicePid(), 15000).catch(() => process.kill(stray, 'SIGKILL'));
    await boot();
    await until('the service to start after the host', async () => {
      const snap = await snapshot();
      return !snap.folders.some((f) => f.synced) || snap.host.service === 'running';
    }, timeoutMs);
    return { before, after: host.child.pid };
  },
  async stop() {
    if (host) await host.stop();
    host = null;
    return true;
  },
};

process.on('message', async ({ id, cmd, args }) => {
  try {
    if (!handlers[cmd]) throw new Error(`unknown command ${cmd}`);
    process.send({ id, ok: true, result: await handlers[cmd](args || {}) });
  } catch (error) {
    process.send({ id, ok: false, error: { message: error.message, code: error.code } });
  }
});
process.on('unhandledRejection', (reason) => {
  console.error('[FATAL] Unhandled rejection:', reason instanceof Error ? reason : new Error(String(reason)));
});
// An orphaned driver never outlives the test that forked it, nor leaves its host.
process.on('disconnect', async () => {
  if (host) await host.stop().catch(() => {});
  process.exit(0);
});
process.send({ ready: true });
