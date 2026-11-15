// Probe: hyperclay.com lets Logo.png and logo.png sit in one folder (its disk
// tells them apart), while the default macOS and Windows disks hold one file for
// both names. Sync then treats one node's bytes as the other's: a node is
// deleted from the server or given the other file's content.
//
// On Linux it makes a case-insensitive ext4 folder (mkfs.ext4 -O casefold and
// chattr +F, needs passwordless sudo), standing in for the macOS and Windows
// default. On macOS it uses a plain folder in home, which is case-insensitive
// by default.
// Prints `FAILED case-collision: ...` while the bug exists, `OK case-collision`
// once every seeded node is still on the server with its own bytes, and
// `INCONCLUSIVE` when the box cannot make a case-insensitive folder.
//
//   node scripts/probes/case-collision.mjs --out <dir>
import { execFileSync } from 'node:child_process';
import { randomBytes } from 'node:crypto';
import { existsSync, mkdirSync, readdirSync, realpathSync, writeFileSync } from 'node:fs';
import { createRequire } from 'node:module';
import os from 'node:os';
import path from 'node:path';
import { build, driver, hex, page, sha, wait } from '../sync-harness.mjs';

const argv = process.argv.slice(2);
const flag = (name) => (argv.includes(name) ? argv[argv.indexOf(name) + 1] : null);
const out = path.resolve(flag('--out') || 'probe-out');
mkdirSync(out, { recursive: true });

const require = createRequire(import.meta.url);
const { startFakeSyncHttp } = require(path.join(process.env.HTMLCLAY_SYNC_LOCAL, 'trial', 'fake-sync-http.cjs'));

const mount = path.join(realpathSync(os.homedir()), `.htmlclay-casefold-${hex()}`);
const folder = path.join(mount, 'maya');

function caseInsensitiveFolder() {
  const image = path.join(out, 'casefold.img');
  if (process.platform !== 'linux') mkdirSync(folder, { recursive: true });
  else execFileSync('bash', ['-c', `set -e
truncate -s 400M ${JSON.stringify(image)}
mkfs.ext4 -q -O casefold ${JSON.stringify(image)}
mkdir -p ${JSON.stringify(mount)}
sudo -n mount -o loop ${JSON.stringify(image)} ${JSON.stringify(mount)}
sudo -n chown "$(id -u):$(id -g)" ${JSON.stringify(mount)}
mkdir ${JSON.stringify(folder)}
chattr +F ${JSON.stringify(folder)}`], { stdio: 'inherit' });
  writeFileSync(path.join(folder, 'Probe.tmp'), 'x');
  const insensitive = existsSync(path.join(folder, 'probe.tmp'));
  execFileSync('rm', [path.join(folder, 'Probe.tmp')]);
  return insensitive;
}

const pairs = [
  { dir: 'assets', dirId: 40, names: ['Logo.png', 'logo.png'] },
  { dir: 'uploads', dirId: 50, names: ['Photo.jpg', 'photo.jpg'] },
];
const nodes = [{ id: 11, type: 'site', name: 'keep.html', parentId: 0, content: page('keep.html', `seed-keep-${hex()}`) }];
const seeded = [];
for (const pair of pairs) {
  nodes.push({ id: pair.dirId, type: 'folder', name: pair.dir, parentId: 0 });
  pair.names.forEach((name, i) => {
    const node = { id: pair.dirId + 1 + i, type: 'upload', name, parentId: pair.dirId, content: Buffer.from(`bytes of ${pair.dir}/${name} ${hex()}\n`) };
    nodes.push(node);
    seeded.push({ id: node.id, rel: `${pair.dir}/${name}`, sum: sha(node.content) });
  });
}

let failed = false;
let d = null;
let fake = null;
try {
  if (!caseInsensitiveFolder()) {
    console.log('INCONCLUSIVE case-collision: the folder is not case-insensitive on this box');
    process.exit(2);
  }
  const built = build(out);
  const key = `hcsk_${randomBytes(32).toString('hex')}`;
  fake = await startFakeSyncHttp({
    nodes,
    key,
    actor: { id: 12, username: 'maya', person: { id: 12, name: 'Maya' } },
    accounts: [{ id: 12, kind: 'personal', username: 'maya', displayName: 'maya', role: 'owner', syncBase: '/_/sync', lifecycle: 'active', sync: { enabled: true, reason: null } }],
  });
  const { server } = fake;
  d = driver();
  await d.ready;
  await d.send('start', { bin: built.bin, configBase: path.join(mount, 'config'), log: path.join(out, 'host.log'), syncServer: fake.url, serviceCmd: built.serviceCmd });
  await d.send('connect', { key });
  await d.send('setUp', { account: 'maya', folder });
  await d.send('landed', { folder, files: { 'keep.html': sha(Buffer.from(nodes[0].content)) }, timeoutMs: 120000 });
  await d.send('idle', { timeoutMs: 30000 }).catch(() => {});
  for (let pass = 0; pass < 2; pass += 1) {
    await d.send('restartService', { timeoutMs: 60000 });
    await d.send('idle', { timeoutMs: 30000 }).catch(() => {});
    await wait(4000);
  }
  for (const pair of pairs) console.log(`on disk in ${pair.dir}/: ${existsSync(path.join(folder, pair.dir)) ? readdirSync(path.join(folder, pair.dir)).join(', ') : '(no folder)'}`);
  const wrong = [];
  for (const entry of seeded) {
    const node = server.get(entry.id);
    const owner = node ? seeded.find((other) => other.sum === sha(node.content)) : null;
    const state = !node ? 'deleted from the server' : owner?.id === entry.id ? 'its own bytes' : `now holds ${owner ? `the bytes of ${owner.rel}` : 'other bytes'}`;
    console.log(`server ${entry.rel} (node ${entry.id}): ${state}`);
    if (!node || owner?.id !== entry.id) wrong.push(`${entry.rel} ${state}`);
  }
  const calls = server.calls.filter((call) => ['deleteNode', 'putNodeContent'].includes(call.name) && seeded.some((entry) => entry.id === call.nodeId));
  console.log(`requests the server received for these nodes: ${calls.map((call) => `${call.name} ${call.pathBefore}`).join('; ') || 'none'}`);
  if (wrong.length === 0) {
    console.log('OK case-collision: every seeded node is still on the server with its own bytes');
  } else {
    failed = true;
    console.log(`FAILED case-collision: ${wrong.join('; ')}`);
  }
} catch (error) {
  failed = true;
  console.log(`INCONCLUSIVE case-collision: ${error.stack || error.message}`);
} finally {
  if (d) {
    await d.send('stop').catch(() => {});
    d.child.disconnect();
  }
  if (fake) await fake.close();
  if (process.platform === 'linux') {
    try {
      execFileSync('sudo', ['-n', 'umount', '-l', mount], { stdio: 'ignore' });
    } catch {}
  }
}
process.exit(failed ? 1 : 0);
