// What the sync soak (soak-sync.mjs) and the kill sweep (hunt-sync-kill.mjs)
// share: a synctest build of htmlclay, the product driver, L1's fake sync server
// with a disposable key, a synced and served folder inside home, saves through
// /_/save, and the ledger's markers and folder walks.
import { execFileSync, fork } from 'node:child_process';
import { createHash, randomBytes } from 'node:crypto';
import { chmodSync, existsSync, mkdtempSync, readdirSync, readFileSync, realpathSync, writeFileSync } from 'node:fs';
import http from 'node:http';
import { createRequire } from 'node:module';
import os from 'node:os';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

export const repo = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
const LOCAL = process.env.HTMLCLAY_SYNC_LOCAL || path.join(repo, '..', 'hyperclay-local');
const FAKE = path.join(LOCAL, 'trial', 'fake-sync-http.cjs');

export const wait = (ms) => new Promise((resolve) => setTimeout(resolve, ms));
export const sha = (bytes) => createHash('sha256').update(bytes).digest('hex');
export const hex = () => randomBytes(4).toString('hex');

export const page = (doc, marker) => `<!doctype html>
<html><head><title>${doc}</title></head><body><h1>${doc}</h1><p data-soak="${marker}">${marker}</p></body></html>
`;

// A server edit keeps what the server held and adds its own marker, the way a
// person adding a paragraph online would.
export const withMarker = (content, marker) => {
  const line = `<p data-soak="${marker}">${marker}</p>`;
  return content.includes('</body>') ? content.replace('</body>', `${line}</body>`) : `${content}${line}\n`;
};

// Local's shouldSkipEntry (src/sync-engine/file-operations.js): what sync never
// carries, so comparisons leave it out on both sides.
export const skipped = (name) => name.startsWith('.') || name === 'node_modules' || name === 'sites-versions' || name === 'tailwindcss';

const markerPattern = /data-soak="([^"]+)"/g;
export const markersOf = (text) => new Set([...text.matchAll(markerPattern)].map((match) => match[1]));

// Every file under dir, hidden ones included (versions, .trash, kept copies).
export function walk(dir, files = []) {
  let entries;
  try {
    entries = readdirSync(dir, { withFileTypes: true });
  } catch {
    return files;
  }
  for (const entry of entries) {
    const full = path.join(dir, entry.name);
    if (entry.isDirectory()) walk(full, files);
    else if (entry.isFile()) files.push(full);
  }
  return files;
}

// The files sync carries, by path relative to folder, with their checksums.
export function syncedFiles(folder) {
  const sums = {};
  const visit = (dir, rel) => {
    for (const entry of readdirSync(dir, { withFileTypes: true })) {
      if (skipped(entry.name)) continue;
      const childRel = rel ? `${rel}/${entry.name}` : entry.name;
      const full = path.join(dir, entry.name);
      if (entry.isDirectory()) visit(full, childRel);
      else if (entry.isFile()) sums[childRel] = sha(readFileSync(full));
    }
  };
  visit(folder, '');
  return sums;
}

// The fake server's files, by path, with their checksums.
export function serverFiles(server) {
  const sums = {};
  for (const id of server.nodes.keys()) {
    const node = server.get(id);
    if (node.type !== 'folder') sums[server.relPathOf(id)] = sha(node.content);
  }
  return sums;
}

// Every marker on disk under folder and on the fake server, its trash included.
export function presentMarkers(folder, server, { skipVersions = false } = {}) {
  const present = new Set();
  const versions = `${path.sep}.hyperclay${path.sep}versions${path.sep}`;
  for (const file of walk(folder)) {
    if (skipVersions && file.includes(versions)) continue;
    for (const marker of markersOf(readFileSync(file, 'utf8'))) present.add(marker);
  }
  for (const node of [...server.nodes.values(), ...server.trash.values()]) {
    if (node.content) for (const marker of markersOf(node.content.toString('utf8'))) present.add(marker);
  }
  return present;
}

export function build(out) {
  const bin = path.join(out, 'htmlclay');
  const env = process.platform === 'linux' ? { ...process.env, CGO_ENABLED: '0' } : process.env;
  execFileSync('go', ['build', '-tags', 'synctest', '-o', bin, './cmd/htmlclay'], { cwd: repo, stdio: 'inherit', env });
  const serviceCmd = path.join(out, 'service.sh');
  writeFileSync(serviceCmd, `#!/bin/sh\nexec ${JSON.stringify(process.execPath)} ${JSON.stringify(path.join(repo, 'service', 'main.cjs'))} "$@"\n`);
  chmodSync(serviceCmd, 0o755);
  return { bin, serviceCmd };
}

export function driver() {
  const child = fork(path.join(repo, 'scripts', 'sync-product-driver.mjs'), [], { stdio: ['ignore', 'inherit', 'inherit', 'ipc'] });
  let next = 0;
  const pending = new Map();
  const ready = new Promise((resolve) => child.once('message', resolve));
  child.on('message', (message) => {
    const waiter = pending.get(message.id);
    if (!waiter) return;
    pending.delete(message.id);
    if (message.ok) waiter.resolve(message.result);
    else waiter.reject(Object.assign(new Error(message.error.message), { code: message.error.code }));
  });
  const send = (cmd, args = {}) => new Promise((resolve, reject) => {
    next += 1;
    pending.set(next, { resolve, reject });
    child.send({ id: next, cmd, args });
  });
  return { child, ready, send };
}

// POST /_/save of one document to a served folder, as a tab sends it.
export function save({ port, host, doc, body }) {
  return new Promise((resolve) => {
    const request = http.request({
      host: '127.0.0.1',
      port,
      method: 'POST',
      path: '/_/save',
      headers: {
        Host: host,
        Origin: `http://${host}`,
        'Sec-Fetch-Site': 'same-origin',
        'Content-Type': 'text/html',
        'Document-URL': `http://${host}/${doc}`,
        'Content-Length': Buffer.byteLength(body),
      },
      timeout: 15000,
    }, (response) => {
      response.resume();
      response.on('end', () => resolve({ status: response.statusCode }));
    });
    request.on('timeout', () => request.destroy(new Error('timeout')));
    request.on('error', (error) => resolve({ error: error.code || error.message }));
    request.end(body);
  });
}

// One synced, served folder for the account maya on L1's fake, seeded with nodes.
// Answers once the first download has landed, unless waitForFirst is false (the
// kill sweep watches it land itself). beforeSetUp(folder) runs just before the
// folder is set up, for a folder that needs preparing (a small disk).
export async function startWorld({ out, built, nodes, waitForFirst = true, log = 'host.log', beforeSetUp = null }) {
  if (!existsSync(FAKE)) throw new Error(`no Hyperclay Local checkout at ${LOCAL} (set HTMLCLAY_SYNC_LOCAL)`);
  const key = `hcsk_${randomBytes(32).toString('hex')}`;
  const require = createRequire(import.meta.url);
  const { startFakeSyncHttp } = require(FAKE);
  const fake = await startFakeSyncHttp({
    nodes,
    key,
    actor: { id: 12, username: 'maya', person: { id: 12, name: 'Maya' } },
    accounts: [
      { id: 12, kind: 'personal', username: 'maya', displayName: 'maya', role: 'owner', syncBase: '/_/sync', lifecycle: 'active', sync: { enabled: true, reason: null } },
    ],
  });
  const server = fake.server;
  const base = realpathSync(mkdtempSync(path.join(realpathSync(os.homedir()), '.htmlclay-sync-hunt-')));
  const folder = path.join(base, 'maya');
  const d = driver();
  await d.ready;
  await d.send('start', { bin: built.bin, configBase: path.join(base, 'config'), log: path.join(out, log), syncServer: fake.url, serviceCmd: built.serviceCmd });
  await d.send('connect', { key });
  if (beforeSetUp) await beforeSetUp(folder);
  await d.send('setUp', { account: 'maya', folder });
  const world = {
    d, fake, server, folder, base,
    idOf: (rel) => [...server.nodes.keys()].find((id) => server.relPathOf(id) === rel) ?? null,
    async served() {
      const row = (await d.send('snapshot')).folders.find((f) => f.fullPath === folder || f.path === folder);
      if (!row || !row.url) throw new Error(`the synced folder is not served: ${JSON.stringify(row)}`);
      const url = new URL(row.url);
      return { url: row.url, host: url.host, port: Number(url.port) };
    },
    async close() {
      await d.send('stop').catch(() => {});
      d.child.disconnect();
      await fake.close();
    },
  };
  if (waitForFirst) {
    const first = Object.fromEntries(nodes.filter((node) => node.type === 'site').map((node) => [server.relPathOf(node.id), sha(Buffer.from(node.content))]));
    await d.send('landed', { folder, files: first, timeoutMs: 300000 });
    await d.send('idle', { timeoutMs: 120000 });
  }
  return world;
}

// The paths that differ between the folder and the server, apart from held ones.
export function differences(folder, server, isHeld = () => false) {
  const local = syncedFiles(folder);
  const remote = serverFiles(server);
  return [...new Set([...Object.keys(local), ...Object.keys(remote)])]
    .filter((rel) => !isHeld(rel) && local[rel] !== remote[rel])
    .map((rel) => ({ path: rel, local: local[rel] ? 'present' : 'absent', server: remote[rel] ? 'present' : 'absent' }));
}

// The held conflicts' own paths and their kept server copies, for comparisons.
export function heldMatcher(conflicts) {
  const held = new Set(conflicts.flatMap((c) => [c.path, c.remoteCopy].filter(Boolean)));
  return (rel) => held.has(rel) || [...held].some((h) => rel.startsWith(h.replace(/\.html$/i, '')) && rel.includes('conflict'));
}

// Sync on, a full comparison (a service start compares the whole folder), quiet,
// and then the folder and the server agree apart from held conflicts, within limit.
export async function converge(world, { limitMs = 120000 } = {}) {
  const { d, folder, server } = world;
  await d.send('pause', { paused: false });
  await d.send('restartService', { timeoutMs: 60000 }).catch(() => {});
  await d.send('idle', { timeoutMs: 300000 });
  const conflicts = await d.send('conflicts');
  const isHeld = heldMatcher(conflicts);
  const settle = Date.now() + limitMs;
  let differ = differences(folder, server, isHeld);
  while (differ.length > 0 && Date.now() < settle) {
    await wait(2000);
    differ = differences(folder, server, isHeld);
  }
  return { conflicts, differ };
}
