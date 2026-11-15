// Probe: Setting up sync on an existing folder (`mode: 'existing'`, `htmlclay sync setup ... --existing`)
// explicitly promises the user (`internal/appcore/copy.go:45`, `service/sync.cjs:45`):
//   "%d files go up, %d come down, and %d differ and are kept both ways. Nothing is deleted."
// Yet when files on both sides differ, `decide()` records `kind: "unbound"` (with `remoteCopy`
// parked on disk), while BOTH `internal/appcore/actions.go:215` (`"unbound": {"mine", "theirs"}`)
// and `service/vendor/local/src/sync-engine/reconcile/execute.js:613` (`KEEP_BOTH_KINDS`)
// omit `"unbound"` — refusing "Keep both" and forcing the user to overwrite one side!
import { randomBytes } from 'node:crypto';
import { createRequire } from 'node:module';
import { existsSync, mkdirSync, mkdtempSync, realpathSync, writeFileSync } from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { build, driver, page, repo, wait } from '../sync-harness.mjs';

function parseOut(argv) {
  const idx = argv.indexOf('--out');
  const dir = idx !== -1 && argv[idx + 1]
    ? path.resolve(argv[idx + 1])
    : mkdtempSync(path.join(os.tmpdir(), 'probe-unbound-keep-both-'));
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

const LOCAL = process.env.HTMLCLAY_SYNC_LOCAL || path.join(repo, '..', 'hyperclay-local');
const require = createRequire(import.meta.url);
const { startFakeSyncHttp } = require(path.join(LOCAL, 'trial', 'fake-sync-http.cjs'));

const out = parseOut(process.argv.slice(2));
const built = build(out);
const key = `hcsk_${randomBytes(32).toString('hex')}`;
const fake = await startFakeSyncHttp({
  nodes: [{ id: 11, type: 'site', name: 'notes.html', parentId: 0, content: page('notes.html', 'REMOTE-PREEXISTING-V0') }],
  key,
  actor: { id: 12, username: 'maya', person: { id: 12, name: 'Maya' } },
  accounts: [
    { id: 12, kind: 'personal', username: 'maya', displayName: 'maya', role: 'owner', syncBase: '/_/sync', lifecycle: 'active', sync: { enabled: true, reason: null } },
  ],
});

const base = realpathSync(mkdtempSync(path.join(realpathSync(os.homedir()), '.htmlclay-unbound-probe-')));
const folder = path.join(base, 'maya-existing');
mkdirSync(folder, { recursive: true });
writeFileSync(path.join(folder, 'notes.html'), page('notes.html', 'LOCAL-PREEXISTING-V0'));

const d = driver();
await d.ready;

try {
  await d.send('start', { bin: built.bin, configBase: path.join(base, 'config'), log: path.join(out, 'host.log'), syncServer: fake.url, serviceCmd: built.serviceCmd });
  await d.send('connect', { key });
  await d.send('setUp', { account: 'maya', folder, existing: true });

  const conflict = await until('unbound conflict on notes.html in Needs You', async () => {
    const held = (await d.send('conflicts')).find((c) => c.path === 'notes.html' && c.kind === 'unbound');
    if (!held) return null;
    const snap = await d.send('snapshot');
    return snap.waiting?.some((w) => w.id === `w-${held.id}`) ? held : null;
  });
  await d.send('idle', { timeoutMs: 30000 });

  let keepBothRefusal = null;
  try {
    await d.send('resolve', { conflict, keep: 'both' });
    await d.send('idle', { timeoutMs: 30000 });
  } catch (err) {
    keepBothRefusal = err.code || err.message;
  }

  // The dashboard's Keep list for unbound is checked by cmd/htmlclay's
  // sync_conflicts_test.go; here the choice itself must keep both copies.
  const copy = path.join(folder, 'notes-conflicted-copy.html');
  if (keepBothRefusal || !existsSync(copy) || !existsSync(path.join(folder, 'notes.html'))) {
    console.log(
      `FAILED: existing-folder sync promised differing files "are kept both ways", ` +
      `but Keep both on the unbound conflict on notes.html (remoteCopy=${Boolean(conflict.remoteCopy)}) ` +
      `was refused with ${keepBothRefusal} or left no copy (copy there: ${existsSync(copy)})`
    );
    process.exitCode = 1;
  } else {
    console.log('OK: unbound conflict supports Keep both as promised');
  }
} finally {
  await d.send('stop').catch(() => {});
  d.child.disconnect();
  await fake.close();
}
