// Review probe (8.55 round 2): two ways an older copy may still change hyperclay.com after the
// identity fix. Built from restored-backup-rolls-back-server.mjs (same world and checks).
//
//   progressive-copy  a backup is copied into the folder's own path in two steps (as Finder,
//                     Explorer or cp -R write a folder): the hidden folder and docs/ land first,
//                     the session is compared (Find it on the same path), then the older pages land
//   second-find-it    Find it on an older copy (recheck holds notes.html as a conflict), then
//                     Find it on another older copy that has no notes.html
//   in-place-copy     a backup's pages are copied over the folder's own while the app runs, keeping
//                     their times (rsync -a, Previous Versions): the folder itself is never replaced
//
//   node scripts/probes/review855-restore-races.mjs --out <dir> [--only <case>]
import { execFileSync } from 'node:child_process';
import { randomBytes } from 'node:crypto';
import { cpSync, existsSync, mkdirSync, mkdtempSync, readFileSync, realpathSync, renameSync, rmSync, writeFileSync } from 'node:fs';
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

const LOCAL = process.env.HTMLCLAY_SYNC_LOCAL || path.join(repo, 'service', 'test', 'fake');
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

async function relinkSamePathOrCopy(world, target) {
  const plain = await world.send('folder.relink', { id: world.rowId, path: target });
  console.log(`${world.name}: Find it with nothing confirmed answers ${JSON.stringify(plain.error ?? plain)}`);
  if (!plain.ok) {
    const confirmed = await world.send('folder.relink', { id: world.rowId, path: target, acknowledged: ['different-folder'] });
    console.log(`${world.name}: Find it with the sheet confirmed answers ${JSON.stringify(confirmed.error ?? confirmed)}`);
    return confirmed.ok;
  }
  return true;
}

async function progressiveCopy(world) {
  const { name, folder, base } = world;
  const copy = copyFolder(world, { marked: true });
  const since = await workSinceTheCopy(world);
  const from = world.server.calls.length;

  renameSync(folder, path.join(base, 'maya-lost'));
  mkdirSync(folder);
  cpSync(path.join(copy, '.hyperclay'), path.join(folder, '.hyperclay'), { recursive: true, preserveTimestamps: true });
  cpSync(path.join(copy, 'docs'), path.join(folder, 'docs'), { recursive: true, preserveTimestamps: true });
  console.log(`${name}: first part of the copy written: ${Object.keys(syncedFiles(folder)).sort().join(', ')}`);
  await wait(10000);
  console.log(`${name}: 10 s into the copy the row reads state=${(await world.rowNow()).state} sync=${(await world.rowNow()).sync?.state} pause=${world.pausedReason()}, server writes: ${serverWrites(world, from).join(', ') || 'none'}`);
  await relinkSamePathOrCopy(world, folder);
  await wait(8000);
  await world.idle(60000).catch((error) => console.log(`${name}: ${error.message}`));
  console.log(`${name}: after the compare: conflicts [${world.conflicts().join(', ')}], files here ${Object.keys(syncedFiles(folder)).sort().join(', ')}, server writes: ${serverWrites(world, from).join(', ') || 'none'}`);
  console.log(`${name}: notes.html here holds [${markersAt(path.join(folder, 'notes.html'))}] plan.html here holds [${markersAt(path.join(folder, 'plan.html'))}]`);

  cpSync(path.join(copy, 'notes.html'), path.join(folder, 'notes.html'), { preserveTimestamps: true });
  cpSync(path.join(copy, 'plan.html'), path.join(folder, 'plan.html'), { preserveTimestamps: true });
  console.log(`${name}: the rest of the copy written (the backup's notes.html and plan.html)`);
  await wait(15000);
  console.log(`${name}: 15 s later, server writes: ${serverWrites(world, from).join(', ') || 'none'}`);
  await world.fullPass().catch((error) => console.log(`${name}: ${error.message}`));
  await wait(5000);
  console.log(`${name}: after a restart, server writes in all: ${serverWrites(world, from).join(', ') || 'none'}`);
  return report(world, since, "a backup was copied into the folder's own path while the app ran, and the session was compared before the copy finished");
}

async function secondFindIt(world) {
  const { name, base } = world;
  const copyA = copyFolder(world, { marked: true });
  const copyB = path.join(base, 'maya-older');
  cpSync(copyA, copyB, { recursive: true, preserveTimestamps: true, filter: (source) => path.basename(source) !== 'notes.html' });
  const since = await workSinceTheCopy(world);

  renameSync(world.folder, path.join(base, 'maya-lost'));
  await wait(5000);
  const from = world.server.calls.length;
  if (!(await relinkSamePathOrCopy(world, copyA))) throw new Error('Find it on the first copy was refused');
  world.folder = copyA;
  await wait(8000);
  await world.idle(60000).catch((error) => console.log(`${name}: ${error.message}`));
  console.log(`${name}: after Find it on copy A: conflicts [${world.conflicts().join(', ')}], server writes: ${serverWrites(world, from).join(', ') || 'none'}`);

  renameSync(copyA, path.join(base, 'maya-a-set-aside'));
  await wait(5000);
  if (!(await relinkSamePathOrCopy(world, copyB))) throw new Error('Find it on the second copy was refused');
  world.folder = copyB;
  await wait(8000);
  await world.idle(60000).catch((error) => console.log(`${name}: ${error.message}`));
  console.log(`${name}: after Find it on copy B (no notes.html): conflicts [${world.conflicts().join(', ')}], notes.html here: ${existsSync(path.join(copyB, 'notes.html'))}`);
  console.log(`${name}: server writes in all: ${serverWrites(world, from).join(', ') || 'none'}`);
  return report(world, since, 'the person pointed Find it at one older copy, then at another older copy that has no notes.html');
}


async function progressiveRestart(world) {
  const { name, folder, base } = world;
  const copy = copyFolder(world, { marked: true });
  const since = await workSinceTheCopy(world);
  const from = world.server.calls.length;

  renameSync(folder, path.join(base, 'maya-lost'));
  mkdirSync(folder);
  cpSync(path.join(copy, '.hyperclay'), path.join(folder, '.hyperclay'), { recursive: true, preserveTimestamps: true });
  cpSync(path.join(copy, 'docs'), path.join(folder, 'docs'), { recursive: true, preserveTimestamps: true });
  console.log(`${name}: first part of the copy written; the sync service restarts (relaunch or wake), no click`);
  await world.fullPass().catch((error) => console.log(`${name}: ${error.message}`));
  await wait(8000);
  console.log(`${name}: after the restart: row state=${(await world.rowNow()).state} conflicts [${world.conflicts().join(', ')}], files here ${Object.keys(syncedFiles(folder)).sort().join(', ')}, server writes: ${serverWrites(world, from).join(', ') || 'none'}`);
  cpSync(path.join(copy, 'notes.html'), path.join(folder, 'notes.html'), { preserveTimestamps: true });
  cpSync(path.join(copy, 'plan.html'), path.join(folder, 'plan.html'), { preserveTimestamps: true });
  console.log(`${name}: the rest of the copy written`);
  await wait(15000);
  await world.fullPass().catch((error) => console.log(`${name}: ${error.message}`));
  await wait(5000);
  console.log(`${name}: server writes in all: ${serverWrites(world, from).join(', ') || 'none'}`);
  return report(world, since, "a backup was copied into the folder's own path, and the app restarted before the copy finished");
}

async function inPlaceCopy(world) {
  const { name, folder } = world;
  const copy = copyFolder(world, { marked: true });
  // The work since the backup comes later than the files in it, as it does when a backup is restored.
  await wait(5000);
  const since = await workSinceTheCopy(world);
  const from = world.server.calls.length;

  cpSync(path.join(copy, 'notes.html'), path.join(folder, 'notes.html'), { preserveTimestamps: true });
  cpSync(path.join(copy, 'plan.html'), path.join(folder, 'plan.html'), { preserveTimestamps: true });
  console.log(`${name}: the backup's notes.html and plan.html copied over the folder's own, keeping their times`);
  await wait(15000);
  console.log(`${name}: 15 s later: conflicts [${world.conflicts().join(', ')}], server writes: ${serverWrites(world, from).join(', ') || 'none'}`);
  await world.fullPass().catch((error) => console.log(`${name}: ${error.message}`));
  await wait(5000);
  console.log(`${name}: after a restart, server writes in all: ${serverWrites(world, from).join(', ') || 'none'}`);
  return report(world, since, "a backup's pages were copied over the folder's own while the app ran");
}

async function deleteConflicted(world) {
  const { name, base } = world;
  const copyA = copyFolder(world, { marked: true });
  const since = await workSinceTheCopy(world);
  renameSync(world.folder, path.join(base, 'maya-lost'));
  await wait(5000);
  const from = world.server.calls.length;
  if (!(await relinkSamePathOrCopy(world, copyA))) throw new Error('Find it on the copy was refused');
  world.folder = copyA;
  await wait(8000);
  await world.idle(60000).catch((error) => console.log(`${name}: ${error.message}`));
  console.log(`${name}: after Find it on the copy: conflicts [${world.conflicts().join(', ')}], server writes: ${serverWrites(world, from).join(', ') || 'none'}`);
  rmSync(path.join(copyA, 'notes.html'));
  console.log(`${name}: the person deletes the old notes.html here to take the server's version`);
  await wait(15000);
  console.log(`${name}: 15 s later, server writes: ${serverWrites(world, from).join(', ') || 'none'}`);
  await world.fullPass().catch((error) => console.log(`${name}: ${error.message}`));
  await wait(5000);
  console.log(`${name}: server writes in all: ${serverWrites(world, from).join(', ') || 'none'}`);
  return report(world, since, 'the person pointed Find it at an older copy, then deleted the old copy of a page Needs you held as a conflict');
}

const cases = {
  'progressive-restart': { run: progressiveRestart },
  'delete-conflicted': { run: deleteConflicted },
  'progressive-copy': { run: progressiveCopy },
  'second-find-it': { run: secondFindIt },
  'in-place-copy': { run: inPlaceCopy },
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
