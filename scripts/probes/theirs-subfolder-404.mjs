// Probe: "Open theirs" (`GET /_/sync/theirs/<path>`) returns HTTP 404 for 100% of
// conflicted documents inside subfolders (`internal/served/sync_theirs.go:38-41`),
// because `serveTheirs` strips `.hyperclay/conflicts/` (`name = "apps/board.remote-<etag>.html"`)
// and rejects `strings.Contains(name, "/")`. Users must pick Keep mine / Keep theirs blind.
import http from 'node:http';
import { existsSync, mkdirSync, mkdtempSync } from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { build, page, save, startWorld, wait, withMarker } from '../sync-harness.mjs';

function parseOut(argv) {
  const idx = argv.indexOf('--out');
  const dir = idx !== -1 && argv[idx + 1]
    ? path.resolve(argv[idx + 1])
    : mkdtempSync(path.join(os.tmpdir(), 'probe-theirs-subfolder-'));
  mkdirSync(dir, { recursive: true });
  return dir;
}

async function until(what, check, timeoutMs = 30000) {
  const deadline = Date.now() + timeoutMs;
  for (;;) {
    const value = await check();
    if (value) return value;
    if (Date.now() > deadline) throw new Error(`timed out waiting for ${what}`);
    await wait(100);
  }
}

function getServed({ port, host, urlPath }) {
  return new Promise((resolve, reject) => {
    const req = http.request({
      host: '127.0.0.1',
      port,
      method: 'GET',
      path: urlPath,
      headers: { Host: host },
    }, (res) => {
      const chunks = [];
      res.on('data', (c) => chunks.push(c));
      res.on('end', () => resolve({ status: res.statusCode, body: Buffer.concat(chunks).toString('utf8') }));
    });
    req.on('error', reject);
    req.end();
  });
}

const out = parseOut(process.argv.slice(2));
const built = build(out);
const world = await startWorld({
  out,
  built,
  nodes: [
    { id: 10, type: 'folder', name: 'apps', parentId: 0 },
    { id: 11, type: 'site', name: 'board.html', parentId: 10, content: page('board.html', 'SEED-BOARD') },
  ],
});

try {
  await world.d.send('pause', { paused: true });
  const { host, port } = await world.served();
  await save({ port, host, doc: 'apps/board.html', body: page('board.html', 'LOCAL-SUBFOLDER-EDIT') });
  await world.server.putNodeContent(null, 11, withMarker(page('board.html', 'SEED-BOARD'), 'REMOTE-SUBFOLDER-EDIT'));
  await world.d.send('pause', { paused: false });

  const conflict = await until('both-edited conflict on apps/board.html', async () =>
    (await world.d.send('conflicts')).find((c) => c.path === 'apps/board.html' && c.remoteCopy));
  await world.d.send('idle', { timeoutMs: 30000 });

  const remoteCopyOnDisk = existsSync(path.join(world.folder, conflict.remoteCopy));
  const res = await getServed({ port, host, urlPath: '/_/sync/theirs/apps/board.html' });

  if (res.status !== 200 || !res.body.includes('REMOTE-SUBFOLDER-EDIT')) {
    console.log(
      `FAILED: Open theirs for subfolder conflict apps/board.html returned HTTP ${res.status} ` +
      `despite remoteCopy on disk (${conflict.remoteCopy}, exists=${remoteCopyOnDisk})`
    );
    process.exitCode = 1;
  } else {
    console.log('OK: Open theirs served subfolder remote copy');
  }
} finally {
  await world.close();
}
