// Probe: a synced folder put back from an older copy of itself makes sync roll
// hyperclay.com back. The copy carries the folder's own .hyperclay/sync-root.json,
// so the session takes it for the folder it synced and compares it with the
// baseline of the folder that was lost: every page the copy holds in an older
// version reads as edited here and is uploaded over the newer one, and every page
// made since the copy reads as deleted here and is deleted on the server.
//
// Seven cases, each in its own world, all at once:
//
//   find-it                  the folder leaves its path, the person clicks Find it, picks the copy
//                            at another path and confirms the "different folder" sheet
//   find-it-unmarked         the same, with a copy made without the hidden .hyperclay folder
//   same-path                the copy takes the folder's own path in one step while the app runs;
//                            Find it is sent only if the server was still whole after a wait
//   same-path-unmarked       the same, with a copy made without the hidden .hyperclay folder
//   same-path-later          the folder leaves its path, and 15 s later the copy takes it
//   linked-find-it-unmarked  find-it-unmarked for a folder that was linked while it already held
//                            its pages ("sync into files already there")
//   linked-find-it-other     that linked folder, with Find it pointed at an unrelated folder
//
// Prints `FAILED restored-backup <case>: ...` while the bug exists and
// `OK restored-backup <case>` once it is gone. Exits 1 on any FAILED.
//
//   node scripts/probes/restored-backup-rolls-back-server.mjs --out <dir> [--only <case>]
import { execFileSync } from 'node:child_process';
import { randomBytes } from 'node:crypto';
import { cpSync, existsSync, mkdirSync, mkdtempSync, readFileSync, realpathSync, renameSync, writeFileSync } from 'node:fs';
import { createRequire } from 'node:module';
import os from 'node:os';
import path from 'node:path';
import { freePort, shellRequest, startShellHost } from '../served-product.mjs';
import { build, hex, markersOf, page, repo, save, syncedFiles, wait } from '../sync-harness.mjs';

const argv = process.argv.slice(2);
const flag = (name) => (argv.includes(name) ? argv[argv.indexOf(name) + 1] : null);
const out = path.resolve(flag('--out') || 'probe-out');
const only = flag('--only');
mkdirSync(out, { recursive: true });

const LOCAL = process.env.HTMLCLAY_SYNC_LOCAL || path.join(repo, '..', 'hyperclay-local');
const require = createRequire(import.meta.url);
const { startFakeSyncHttp } = require(path.join(LOCAL, 'trial', 'fake-sync-http.cjs'));

async function until(what, check, timeoutMs = 60000) {
  const deadline = Date.now() + timeoutMs;
  for (;;) {
    const value = await check();
    if (value) return value;
    if (Date.now() > deadline) throw new Error(`timed out waiting for ${what}`);
    await wait(250);
  }
}

const MARKER = path.join('.hyperclay', 'sync-root.json');

// The older copy of the folder, as a backup tool or a Finder duplicate makes it. A copy
// made from the files a person sees leaves the hidden .hyperclay folder out.
function copyFolder(world, { marked = true } = {}) {
  const copy = path.join(world.base, 'maya-backup');
  cpSync(world.folder, copy, { recursive: true, preserveTimestamps: true, filter: (source) => marked || path.basename(source) !== '.hyperclay' });
  console.log(`${world.name}: the copy ${existsSync(path.join(copy, MARKER)) ? `carries the folder's marker ${readFileSync(path.join(copy, MARKER), 'utf8').replace(/\s+/g, ' ')}` : 'has no sync marker'}`);
  return copy;
}

const markersAt = (file) => (existsSync(file) ? [...markersOf(readFileSync(file, 'utf8'))] : null);

async function startWorld(name, built, { env = {}, linked = false } = {}) {
  const first = { notes: `notes-old-${hex()}`, plan: `plan-old-${hex()}`, guide: `guide-old-${hex()}` };
  const key = `hcsk_${randomBytes(32).toString('hex')}`;
  const fake = await startFakeSyncHttp({
    nodes: [
      { id: 11, type: 'site', name: 'notes.html', parentId: 0, content: page('notes.html', first.notes) },
      { id: 12, type: 'site', name: 'plan.html', parentId: 0, content: page('plan.html', first.plan) },
      { id: 13, type: 'folder', name: 'docs', parentId: 0 },
      { id: 14, type: 'site', name: 'guide.html', parentId: 13, content: page('docs/guide.html', first.guide) },
    ],
    key,
    actor: { id: 12, username: 'maya', person: { id: 12, name: 'Maya' } },
    accounts: [
      { id: 12, kind: 'personal', username: 'maya', displayName: 'maya', role: 'owner', syncBase: '/_/sync', lifecycle: 'active', sync: { enabled: true, reason: null } },
    ],
  });
  const server = fake.server;
  const base = realpathSync(mkdtempSync(path.join(realpathSync(os.homedir()), 'htmlclay-backup-probe-')));
  const configBase = path.join(base, 'config');
  const configDir = path.join(configBase, 'htmlclay');
  mkdirSync(configDir, { recursive: true });
  writeFileSync(path.join(configDir, 'config.json'), `${JSON.stringify({ startOnLogin: false, port: await freePort(), updates: 'off', servedFolders: [] }, null, 2)}\n`);
  const host = await startShellHost({
    bin: built.bin,
    cwd: configBase,
    log: path.join(out, `${name}-host.log`),
    env: {
      ...process.env,
      HTMLCLAY_CONFIG_BASE: configBase,
      HTMLCLAY_SECRET_STORE: 'file',
      HTMLCLAY_RELEASE_INFO_URL: 'http://127.0.0.1:1/',
      HTMLCLAY_SYNC_SERVER: fake.url,
      HTMLCLAY_SERVICE_CMD: built.serviceCmd,
      ...env,
    },
  });

  let actions = 0;
  const send = async (type, args) => {
    actions += 1;
    const answer = await shellRequest(host, 'POST', '/shell/v1/actions', { id: `${name}-${actions}`, type, args });
    return answer.body ? JSON.parse(answer.body) : {};
  };
  const act = async (type, args) => {
    const body = await send(type, args);
    if (!body.ok) throw new Error(`${type} was refused: ${JSON.stringify(body.error ?? body)}`);
    return body;
  };
  const snapshot = async () => JSON.parse((await shellRequest(host, 'GET', '/shell/v1/snapshot')).body);
  const servicePid = () => {
    const marker = `--data-dir ${path.join(configDir, 'service')}`;
    const lines = execFileSync('ps', ['-A', '-o', 'pid=,command='], { encoding: 'utf8' }).split('\n');
    const line = lines.find((entry) => entry.includes(marker) && !entry.includes('ps -A'));
    return line ? Number(line.trim().split(/\s+/)[0]) : null;
  };

  await act('sync.connect', { key, username: 'maya' });
  const row = await until('the account row', async () =>
    (await snapshot()).folders.find((f) => f.origin?.kind === 'account' && f.origin?.username === 'maya' && f.state === 'not-on-this-computer'));
  const folder = path.join(base, 'maya');
  mkdirSync(folder, { recursive: true });
  if (linked) {
    // The folder already holds the account's pages, byte for byte, and is linked as it stands.
    mkdirSync(path.join(folder, 'docs'));
    for (const id of server.nodes.keys()) {
      if (server.get(id).type !== 'folder') writeFileSync(path.join(folder, server.relPathOf(id)), server.get(id).content);
    }
  }
  await act('folder.setUpTeam', { id: row.id, path: folder, existing: linked, confirmed: true });

  const rowNow = async () => (await snapshot()).folders.find((f) => f.id === row.id);
  const idle = async (timeoutMs = 120000) => {
    let quiet = 0;
    return until('the folder to settle', async () => {
      const snap = await snapshot();
      const f = snap.folders.find((x) => x.id === row.id);
      const settled = f && f.synced && snap.sync.syncingFiles === 0 && ['up-to-date', 'conflicts', 'paused'].includes(f.sync?.state);
      quiet = settled ? quiet + 1 : 0;
      return quiet >= 2 ? f : null;
    }, timeoutMs);
  };
  // A service start compares the whole folder with the server, as converge() in the harness relies on.
  const fullPass = async () => {
    const before = servicePid();
    if (before) process.kill(before, 'SIGKILL');
    await until('the service to start again', async () => {
      const pid = servicePid();
      return pid && pid !== before && (await snapshot()).host.service === 'running';
    });
    return idle();
  };
  const idOf = (rel) => [...server.nodes.keys()].find((id) => server.relPathOf(id) === rel) ?? null;
  const serverMarkers = (rel) => {
    const id = idOf(rel);
    return id === null ? null : [...markersOf(server.get(id).content.toString('utf8'))];
  };
  const conflicts = () => {
    const record = JSON.parse(readFileSync(path.join(configDir, 'sync.json'), 'utf8'));
    return (record.syncSessions || []).flatMap((session) => {
      const file = path.join(session.legacyMetaDir || path.join(configDir, 'service', 'sync-meta', 'v2', session.id), 'conflicts.json');
      if (!existsSync(file)) return [];
      return Object.values(JSON.parse(readFileSync(file, 'utf8'))).map((r) => `${r.kind}:${r.path}`);
    });
  };
  const pausedReason = () => {
    const record = JSON.parse(readFileSync(path.join(configDir, 'sync.json'), 'utf8'));
    return (record.syncSessions || []).map((session) => session.paused?.reason ?? 'none').join(',');
  };
  const close = async () => {
    await host.stop().catch(() => {});
    await fake.close();
  };

  await until('the first download', () => existsSync(path.join(folder, 'docs', 'guide.html')) && existsSync(path.join(folder, 'notes.html')) && existsSync(path.join(folder, 'plan.html')));
  await until("the folder's sync marker", () => existsSync(path.join(folder, MARKER)));
  await idle();
  return { name, first, server, base, folder, configDir, rowId: row.id, send, act, snapshot, rowNow, idle, fullPass, idOf, serverMarkers, conflicts, pausedReason, close };
}

// What a person and a teammate do after the copy was made: an edit saved here, an
// edit made online, and a page created online. Answers the markers of that work.
async function workSinceTheCopy(world) {
  const { server, folder, name } = world;
  const since = { notes: `notes-new-here-${hex()}`, plan: `plan-new-online-${hex()}`, fresh: `fresh-new-online-${hex()}` };
  const served = new URL((await world.rowNow()).url);
  const saved = await save({ port: Number(served.port), host: served.host, doc: 'notes.html', body: page('notes.html', since.notes) });
  if (saved.status !== 200) throw new Error(`the save to notes.html answered ${JSON.stringify(saved)}`);
  await until('the saved edit to reach the server', () => world.serverMarkers('notes.html')?.includes(since.notes));
  await server.putNodeContent(null, 12, page('plan.html', since.plan));
  await server.createNode(null, { type: 'site', name: 'fresh.html', parentId: 0, content: page('fresh.html', since.fresh) });
  await world.fullPass();
  await until('the online work to arrive here', () => markersAt(path.join(folder, 'plan.html'))?.includes(since.plan) && markersAt(path.join(folder, 'fresh.html'))?.includes(since.fresh));
  await world.idle();
  console.log(`${name}: before the folder is lost the server holds notes.html [${world.serverMarkers('notes.html')}], plan.html [${world.serverMarkers('plan.html')}], fresh.html [${world.serverMarkers('fresh.html')}]`);
  return since;
}

// What of the work since the copy the server no longer holds as its live page.
function lostOnServer(world, since) {
  const lost = [];
  const check = (rel, marker, what) => {
    const held = world.serverMarkers(rel);
    const trashed = [...world.server.trash.values()].some((node) => node.name === path.basename(rel));
    if (held === null) lost.push(`${rel} is ${trashed ? 'deleted on the server (in its trash)' : 'gone from the server'}`);
    else if (marker && !held.includes(marker)) lost.push(`${rel} now holds [${held}] in place of ${what}`);
  };
  check('notes.html', since.notes, 'the edit saved here');
  check('plan.html', since.plan, "the teammate's edit");
  check('fresh.html', since.fresh, "the teammate's new page");
  check('docs/guide.html', null, null);
  return lost;
}

function serverWrites(world, from) {
  return world.server.calls.slice(from)
    .filter((call) => ['putNodeContent', 'deleteNode', 'createNode', 'renameNode', 'moveNode'].includes(call.name))
    .map((call) => `${call.name}(${call.nodeId ?? call.targetId ?? call.args?.[0] ?? ''})`);
}

function report(world, since, how) {
  const { name, folder } = world;
  const lost = lostOnServer(world, since);
  console.log(`${name}: held conflicts: [${world.conflicts().join(', ')}], the session's pause: ${world.pausedReason()}`);
  console.log(`${name}: files here now: ${Object.keys(syncedFiles(folder)).sort().join(', ')}`);
  if (lost.length === 0) {
    console.log(`OK restored-backup ${name}`);
    return true;
  }
  console.log(`FAILED restored-backup ${name}: ${how}, and with no question asked sync changed hyperclay.com: ${lost.join('; ')}`);
  return false;
}

async function findIt(world, { marked, other = false }) {
  const { name, folder, base } = world;
  let copy = copyFolder(world, { marked });
  const since = await workSinceTheCopy(world);
  if (other) {
    copy = path.join(base, 'other-site');
    mkdirSync(copy);
    writeFileSync(path.join(copy, 'todo.html'), page('todo.html', `todo-${hex()}`));
  }

  renameSync(folder, path.join(base, 'maya-lost'));
  await wait(5000);
  const row = await world.rowNow();
  console.log(`${name}: with the folder gone from its path the row reads state=${row.state} sync=${row.sync?.state} pause=${world.pausedReason()}`);
  const early = lostOnServer(world, since);
  if (early.length > 0) console.log(`${name}: the server changed before Find it: ${early.join('; ')}`);

  const from = world.server.calls.length;
  const plain = await world.send('folder.relink', { id: world.rowId, path: copy });
  console.log(`${name}: Find it with nothing confirmed answers ${JSON.stringify(plain.error ?? plain)}`);
  const confirmed = await world.send('folder.relink', { id: world.rowId, path: copy, acknowledged: ['different-folder'] });
  if (!confirmed.ok) console.log(`${name}: Find it with the sheet confirmed answers ${JSON.stringify(confirmed.error ?? confirmed)}`);
  await wait(10000);
  await world.idle(60000).catch((error) => console.log(`${name}: ${error.message}`));
  const after = await world.rowNow();
  console.log(`${name}: after Find it the row reads state=${after.state} sync=${after.sync?.state}`);
  console.log(`${name}: writes the server received after Find it: ${serverWrites(world, from).join(', ') || 'none'}`);
  if (confirmed.ok) world.folder = copy;
  return report(world, since, `the person pointed Find it at ${other ? 'an unrelated folder' : 'an older copy of the folder'} and confirmed it is a different folder`);
}

async function samePath(world, { marked, gapMs = 0, watchMs = 20000 }) {
  const { name, folder, base } = world;
  const copy = copyFolder(world, { marked });
  const since = await workSinceTheCopy(world);

  const from = world.server.calls.length;
  renameSync(folder, path.join(base, 'maya-lost'));
  if (gapMs > 0) {
    await wait(gapMs);
    console.log(`${name}: ${gapMs / 1000} s after the folder left its path the row reads state=${(await world.rowNow()).state} pause=${world.pausedReason()}, server writes: ${serverWrites(world, from).join(', ') || 'none'}`);
  }
  renameSync(copy, folder);
  const deadline = Date.now() + watchMs;
  while (Date.now() < deadline && lostOnServer(world, since).length === 0) await wait(1000);
  await wait(5000);
  const row = await world.rowNow();
  console.log(`${name}: with the copy at the folder's path and no click the row reads state=${row.state} sync=${row.sync?.state} pause=${world.pausedReason()}`);
  console.log(`${name}: writes the server received with no click: ${serverWrites(world, from).join(', ') || 'none'}`);
  if (lostOnServer(world, since).length > 0) {
    return report(world, since, "an older copy was put back at the folder's own path while the app ran, with no click in HTML Clay");
  }

  const plain = await world.send('folder.relink', { id: world.rowId, path: folder });
  console.log(`${name}: Find it with nothing confirmed answers ${JSON.stringify(plain.error ?? plain)}`);
  if (!plain.ok) {
    const confirmed = await world.send('folder.relink', { id: world.rowId, path: folder, acknowledged: ['different-folder'] });
    if (!confirmed.ok) console.log(`${name}: Find it with the sheet confirmed answers ${JSON.stringify(confirmed.error ?? confirmed)}`);
  }
  await wait(10000);
  await world.idle(60000).catch((error) => console.log(`${name}: ${error.message}`));
  const after = await world.rowNow();
  console.log(`${name}: after Find it the row reads state=${after.state} sync=${after.sync?.state}`);
  console.log(`${name}: writes the server received in all: ${serverWrites(world, from).join(', ') || 'none'}`);
  return report(world, since, "an older copy was put back at the folder's own path and the person confirmed Find it");
}

const cases = {
  'find-it': { run: (world) => findIt(world, { marked: true }) },
  'find-it-unmarked': { run: (world) => findIt(world, { marked: false }) },
  'same-path': { run: (world) => samePath(world, { marked: true }) },
  'same-path-unmarked': { run: (world) => samePath(world, { marked: false }) },
  'same-path-later': { run: (world) => samePath(world, { marked: true, gapMs: 15000, watchMs: 40000 }) },
  'linked-find-it-unmarked': { linked: true, run: (world) => findIt(world, { marked: false }) },
  'linked-find-it-other': { linked: true, run: (world) => findIt(world, { marked: false, other: true }) },
};

const built = build(out);
const results = await Promise.all(Object.entries(cases).filter(([name]) => !only || only.split(',').includes(name)).map(async ([name, { env, linked, run }]) => {
  let world = null;
  try {
    world = await startWorld(name, built, { env, linked });
    return await run(world);
  } catch (error) {
    console.log(`ERROR restored-backup ${name}: ${error.stack || error.message}`);
    return false;
  } finally {
    if (world) await world.close();
  }
}));
process.exitCode = results.every(Boolean) ? 0 : 1;
