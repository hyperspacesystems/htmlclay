// Hyperclay Local's last release passes holdRoot: a folder that moved to HTML
// Clay never syncs there again, whether its old session starts or it is set up
// again by path.
const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const os = require('node:os');
const path = require('node:path');
const { SyncManager } = require('../local/src/main/sync-manager.js');

test('holdRoot stops a moved folder starting or being set up again', { timeout: 30000 }, async (t) => {
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), 'hold-root-'));
  t.after(() => fs.rmSync(dir, { recursive: true, force: true }));
  const moved = path.join(dir, 'Studio');
  fs.mkdirSync(moved);
  let settings = { roots: [{ id: 'r1', path: moved }], syncSessions: [{ id: 's1', rootId: 'r1', accountId: 40, kind: 'team', cached: { username: 'studio' } }] };
  let saves = 0;
  const manager = new SyncManager({
    userData: dir,
    deviceId: 'd',
    serverUrl: 'http://127.0.0.1:1',
    getApiKey: () => 'made-up-key',
    settingsStore: { get: () => settings, save: (next) => { settings = next; saves += 1; } },
    holdRoot: (rootId, folder) => rootId === 'r1' || folder === moved,
  });
  await assert.rejects(
    manager.bind({ session: { id: 's2', rootId: 'r2', accountId: 40, kind: 'team' }, root: { id: 'r2', path: moved }, account: { id: 40 } }),
    (error) => error.code === 'folder-moved',
  );
  assert.equal(saves, 0, 'a refused set-up writes no settings');
  assert.deepEqual(settings.roots.map((r) => r.id), ['r1']);
  t.after(async () => { for (const id of [...manager.sessions.keys()]) await manager.stop(id).catch(() => {}); });
  await Promise.race([manager.startEnabledSessions().catch(() => {}), new Promise((resolve) => setTimeout(resolve, 3000))]);
  assert.equal(manager.sessions.size, 0, 'the moved folder\'s session did not start');
});
