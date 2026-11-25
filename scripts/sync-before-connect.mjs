// 08-sync.md Gates, "Nothing before Connect": the real htmlclay (tag synctest) with the
// live-room service runs for some minutes against a recorder standing in for hyperclay.com,
// and is used without ever connecting: tabs save to a served folder, sync is paused and
// resumed. The recorder counts every TCP connection. Any connection fails the run.
//
// node scripts/sync-before-connect.mjs [--minutes 5] [--out <dir>] [--break-connect]
// --break-connect connects once at the end, proving the recorder sees a connection.
// Prints BEFORE CONNECT OK connections=0 saves=<n>, or FAILED and exits 1.
import { spawn } from 'node:child_process';
import { randomBytes } from 'node:crypto';
import { mkdirSync, mkdtempSync, realpathSync, writeFileSync } from 'node:fs';
import net from 'node:net';
import os from 'node:os';
import path from 'node:path';
import { freePort } from './served-product.mjs';
import { build, page, save, wait } from './sync-harness.mjs';

const argv = process.argv.slice(2);
const flag = (name) => {
  const at = argv.indexOf(name);
  return at === -1 ? null : argv[at + 1];
};
const minutes = Number(flag('--minutes') || 5);
const breakConnect = argv.includes('--break-connect');
const out = flag('--out') || mkdtempSync(path.join(os.tmpdir(), 'sync-before-connect-'));
mkdirSync(out, { recursive: true });
const log = (line) => console.log(`${new Date().toISOString().slice(11, 19)} ${line}`);

const built = build(out);

const seen = [];
const recorder = net.createServer((socket) => {
  seen.push(new Date().toISOString());
  socket.destroy();
});
await new Promise((resolve) => recorder.listen(0, '127.0.0.1', resolve));
const serverUrl = `http://127.0.0.1:${recorder.address().port}`;

const home = os.homedir();
const folder = realpathSync(mkdtempSync(path.join(home, 'before-connect-')));
const configBase = mkdtempSync(path.join(os.tmpdir(), 'before-connect-config-'));
const name = 'notes';
const port = await freePort();
const host = `${name}.htmlclay.localhost:${port}`;
const docs = ['a.html', 'b.html', 'c.html'];
for (const doc of docs) writeFileSync(path.join(folder, doc), page(doc, `seed-${doc}`));
mkdirSync(path.join(configBase, 'htmlclay'), { recursive: true });
writeFileSync(path.join(configBase, 'htmlclay', 'config.json'), `${JSON.stringify({
  startOnLogin: false,
  port,
  updates: 'off',
  servedFolders: [{ id: randomBytes(8).toString('hex'), name, path: folder, identity: '', serving: true }],
}, null, 2)}\n`);

const env = {
  ...process.env,
  HTMLCLAY_CONFIG_BASE: configBase,
  HTMLCLAY_SYNC_SERVER: serverUrl,
  HTMLCLAY_SECRET_STORE: 'file',
  HTMLCLAY_SERVICE_CMD: built.serviceCmd,
  HTMLCLAY_SYNC_DISCOVERY_MS: '2000',
};

const app = spawn(built.bin, ['--no-tray'], { env, stdio: ['ignore', 'pipe', 'pipe'] });
const appLines = [];
for (const stream of [app.stdout, app.stderr]) {
  stream.setEncoding('utf8');
  stream.on('data', (chunk) => appLines.push(chunk));
}
const appExit = new Promise((resolve) => app.once('exit', (code, signal) => resolve({ code, signal })));

function cli(args, stdin = '') {
  return new Promise((resolve) => {
    const child = spawn(built.bin, args, { env, stdio: ['pipe', 'pipe', 'pipe'] });
    let text = '';
    child.stdout.on('data', (chunk) => { text += chunk; });
    child.stderr.on('data', (chunk) => { text += chunk; });
    child.on('exit', (code) => resolve({ code, out: text.trim() }));
    child.stdin.end(stdin);
  });
}

let failed = null;
let saves = 0;
try {
  const ready = Date.now() + 30000;
  while ((await save({ port, host, doc: docs[0], body: page(docs[0], 'ready') })).status !== 200) {
    if (Date.now() > ready) throw new Error('the served folder never answered a save');
    await wait(200);
  }
  log(`app up, serving ${folder} at ${host}; recorder at ${serverUrl}`);

  const deadline = Date.now() + minutes * 60000;
  let lastToggle = Date.now();
  let paused = false;
  while (Date.now() < deadline) {
    const doc = docs[saves % docs.length];
    const answer = await save({ port, host, doc, body: page(doc, `n${saves}-${randomBytes(4).toString('hex')}`) });
    if (answer.status === 200) saves += 1;
    if (Date.now() - lastToggle > 20000) {
      const toggled = await cli(['sync', paused ? 'resume' : 'pause']);
      log(`sync ${paused ? 'resume' : 'pause'}: exit ${toggled.code} ${toggled.out.split('\n').join(' / ')}`);
      paused = !paused;
      lastToggle = Date.now();
    }
    await wait(100);
  }
  log(`used for ${minutes} minutes without connecting: ${saves} saves, ${seen.length} connections`);

  if (breakConnect) {
    const connected = await cli(['sync', 'connect', 'maya'], `hcsk_${randomBytes(32).toString('hex')}\n`);
    log(`connect: exit ${connected.code} ${connected.out.split('\n').join(' / ')}`);
    await wait(3000);
  }
} catch (error) {
  failed = error;
}

app.kill('SIGTERM');
const gone = await Promise.race([appExit, wait(20000).then(() => null)]);
if (!gone) {
  app.kill('SIGKILL');
  await appExit;
}
writeFileSync(path.join(out, 'app.log'), appLines.join(''));
recorder.close();

if (failed) {
  console.log(`BEFORE CONNECT FAILED: ${failed.message}`);
  process.exit(1);
}
if (seen.length > 0) {
  console.log(`BEFORE CONNECT FAILED connections=${seen.length} first=${seen[0]} saves=${saves}`);
  process.exit(1);
}
if (saves === 0) {
  console.log('BEFORE CONNECT FAILED: no save landed, so the app was not used');
  process.exit(1);
}
console.log(`BEFORE CONNECT OK connections=0 saves=${saves}`);
