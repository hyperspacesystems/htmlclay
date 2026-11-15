// Probe: a full comparison whose folder scan stops early reads the files it did
// not reach as deleted here, and deletes them from the server while they are
// still on disk. Three ways to stop the scan, each in its own world:
//
//   backslash   a file whose name holds a backslash, as a Windows-made zip leaves on macOS or Linux
//   unreadable  a subfolder the scan may not list
//   churn       a script that keeps writing and removing .html files in the folder, as a
//               build that regenerates pages does, while sync starts (up to 5 starts)
//
// Prints `FAILED scan-abort-remote-delete <case>: ...` while the bug exists and
// `OK scan-abort-remote-delete <case>` once it is gone. Exits 1 on any FAILED.
//
//   node scripts/probes/scan-abort-remote-delete.mjs --out <dir> [--only backslash|unreadable|churn]
import { spawn } from 'node:child_process';
import { chmodSync, existsSync, mkdirSync, readFileSync, readdirSync, rmSync, writeFileSync } from 'node:fs';
import path from 'node:path';
import { build, hex, page, sha, startWorld, wait } from '../sync-harness.mjs';

const argv = process.argv.slice(2);
const flag = (name) => (argv.includes(name) ? argv[argv.indexOf(name) + 1] : null);
const out = path.resolve(flag('--out') || 'probe-out');
const only = flag('--only');
mkdirSync(out, { recursive: true });

function seed() {
  const nodes = [];
  for (let i = 0; i < 12; i += 1) {
    const doc = `doc-${String(i).padStart(2, '0')}.html`;
    nodes.push({ id: 11 + i, type: 'site', name: doc, parentId: 0, content: page(doc, `seed-${doc}-${hex()}`) });
  }
  nodes.push({ id: 40, type: 'folder', name: 'projects', parentId: 0 });
  for (let i = 0; i < 3; i += 1) {
    const doc = `plan-${i}.html`;
    nodes.push({ id: 41 + i, type: 'site', name: doc, parentId: 40, content: page(doc, `seed-${doc}-${hex()}`) });
  }
  ['logo.png', 'style.css', 'data.json'].forEach((name, i) => {
    nodes.push({ id: 60 + i, type: 'upload', name, parentId: 0, content: Buffer.from(`upload ${name} ${hex()}\n`) });
  });
  return nodes;
}

const filesOn = (server) => [...server.nodes.keys()].filter((id) => server.get(id).type !== 'folder').map((id) => server.relPathOf(id)).sort();
const deletesSent = (server) => server.calls.filter((call) => call.name === 'deleteNode').map((call) => call.pathBefore);

async function fullComparison(world, idleMs = 90000) {
  await world.d.send('restartService', { timeoutMs: 60000 });
  const settled = await world.d.send('idle', { timeoutMs: idleMs }).then(() => true, () => false);
  await wait(4000);
  return settled;
}

async function folderState(world) {
  const snap = await world.d.send('snapshot');
  return snap.folders.filter((f) => f.synced).map((f) => f.sync.state).join(',');
}

const churnLoop = `
const fs = require('fs');
const path = require('path');
const dir = process.argv[1];
for (let i = 0; ; i += 1) {
  const file = path.join(dir, 'aa-build-' + i + '.html');
  fs.writeFileSync(file, '<!doctype html><title>build</title>');
  fs.unlinkSync(file);
}
`;

const cases = {
  backslash: {
    stop(world) {
      writeFileSync(path.join(world.folder, 'export\\index.html'), page('index.html', `stray-${hex()}`));
      writeFileSync(path.join(world.folder, 'export\\logo.png'), Buffer.from('stray upload\n'));
    },
    clear(world) {
      rmSync(path.join(world.folder, 'export\\index.html'));
      rmSync(path.join(world.folder, 'export\\logo.png'));
    },
  },
  unreadable: {
    stop(world) {
      chmodSync(path.join(world.folder, 'projects'), 0o000);
    },
    clear(world) {
      chmodSync(path.join(world.folder, 'projects'), 0o755);
    },
  },
  churn: {
    attempts: 5,
    idleMs: 20000,
    stop(world) {
      world.churn = spawn(process.execPath, ['-e', churnLoop, world.folder], { stdio: 'ignore' });
    },
    async clear(world) {
      const exited = new Promise((resolve) => world.churn.once('exit', resolve));
      world.churn.kill('SIGKILL');
      await exited;
      for (const name of readdirSync(world.folder)) if (name.startsWith('aa-build-')) rmSync(path.join(world.folder, name), { force: true });
    },
  },
};

async function run(name, built) {
  const caseOut = path.join(out, name);
  mkdirSync(caseOut, { recursive: true });
  const nodes = seed();
  const world = await startWorld({ out: caseOut, built, nodes });
  const { server, folder, d } = world;
  try {
    const uploads = Object.fromEntries(nodes.filter((n) => n.type === 'upload').map((n) => [n.name, sha(n.content)]));
    await d.send('landed', { folder, files: uploads, timeoutMs: 60000 });
    await d.send('idle', { timeoutMs: 60000 });
    const before = filesOn(server);
    const idsBefore = Object.fromEntries(before.map((rel) => [rel, world.idOf(rel)]));

    await fullComparison(world);
    const controlDeletes = deletesSent(server);
    if (controlDeletes.length > 0 || filesOn(server).length !== before.length) {
      console.log(`INCONCLUSIVE scan-abort-remote-delete ${name}: a full comparison of an untouched folder already deleted ${controlDeletes.join(', ')}`);
      return false;
    }
    console.log(`CONTROL ${name}: a full comparison of the untouched folder sent 0 deletes, the server holds ${before.length} files`);

    cases[name].stop(world);
    await wait(name === 'churn' ? 500 : 6000);
    console.log(`${name}: folder listing order: ${readdirSync(folder).join(' | ')}`);
    console.log(`${name}: sync state before the restart: ${await folderState(world)}`);
    let settled = false;
    let starts = 0;
    while (starts < (cases[name].attempts || 1) && !deletesSent(server).some((rel) => before.includes(rel))) {
      starts += 1;
      settled = await fullComparison(world, cases[name].idleMs);
    }
    console.log(`${name}: sync state after ${starts} full comparison(s): ${await folderState(world)} (settled=${settled})`);

    await cases[name].clear(world);
    const deleted = deletesSent(server);
    const lost = before.filter((rel) => existsSync(path.join(folder, rel)) && world.idOf(rel) !== idsBefore[rel]);
    const recreated = lost.filter((rel) => server.hasPath(rel));
    const log = readFileSync(path.join(caseOut, 'host.log'), 'utf8').split('\n');
    for (const line of log.filter((entry) => /Error scanning directory|Directory scan failed/.test(entry)).slice(0, 3)) console.log(`${name}: host.log: ${line.slice(0, 220)}`);
    const stable = deleted.filter((rel) => before.includes(rel));
    console.log(`${name}: deleteNode requests the server received for the seeded files: ${stable.length} (${stable.join(', ')})`);
    if (lost.length === 0) {
      console.log(`OK scan-abort-remote-delete ${name}: every file still on disk is still the same node on the server (${before.length} files)`);
      return true;
    }
    console.log(`FAILED scan-abort-remote-delete ${name}: sync deleted ${lost.length} of ${before.length} files from the server while each is still on disk: ${lost.join(', ')} (${recreated.length} of them were already uploaded again as a new node)`);

    await fullComparison(world);
    const back = lost.filter((rel) => server.hasPath(rel));
    const moved = back.filter((rel) => world.idOf(rel) !== idsBefore[rel]);
    console.log(`AFTERMATH ${name}: with the cause removed, the next full comparison put ${back.length} of ${lost.length} back on the server, ${moved.length} of them as a new node (for example ${moved.slice(0, 3).map((rel) => `${rel} ${idsBefore[rel]} -> ${world.idOf(rel)}`).join(', ')}); still missing from the server: ${lost.filter((rel) => !server.hasPath(rel)).join(', ') || 'none'}`);
    return false;
  } finally {
    try {
      chmodSync(path.join(folder, 'projects'), 0o755);
    } catch {}
    if (world.churn) world.churn.kill('SIGKILL');
    await world.close();
  }
}

const built = build(out);
let failed = false;
for (const name of Object.keys(cases)) {
  if (only && only !== name) continue;
  try {
    if (!(await run(name, built))) failed = true;
  } catch (error) {
    failed = true;
    console.log(`INCONCLUSIVE scan-abort-remote-delete ${name}: ${error.stack || error.message}`);
  }
}
process.exit(failed ? 1 : 0);
