// The sync hostile input sweep (08-sync.md 8.54): what a broken or hostile server
// can send, and disk trouble, on the real pair and L1's fake. Each case has its
// own world with one good document beside the hostile item, and must end with
// the hostile item refused or held, nothing written outside the folder, the good
// document landed (sync is not wedged), the host still answering, and no half
// file. Prints `HOSTILE <case> ok|FAILED <why> (<what became of it>)` per case and
// `SYNC HOSTILE SWEEP OK cases=<n>` or `SYNC HOSTILE SWEEP FAILED ...`.
//
//   node scripts/hunt-sync-hostile.mjs [--only <case>] [--break outside]
//
// --break outside writes one file beside the folder during the dot-dot case, as
// an escape would: it must fail. The disk-full case needs passwordless sudo (a
// test box) or a run under `unshare --map-root-user --mount` to mount a small
// tmpfs; without either the case is reported skipped.
import { execFileSync } from 'node:child_process';
import {
  chmodSync, existsSync, mkdirSync, mkdtempSync, readFileSync, readdirSync, rmSync, symlinkSync, writeFileSync,
} from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import {
  build, hex, page, save, sha, startWorld, syncedFiles, wait, walk, withMarker,
} from './sync-harness.mjs';

function parseArgs(argv) {
  const options = { only: null, breakCase: null, out: null };
  for (let i = 0; i < argv.length; i += 1) {
    const flag = argv[i];
    const value = () => {
      i += 1;
      if (i >= argv.length) throw new Error(`${flag} needs a value`);
      return argv[i];
    };
    switch (flag) {
      case '--only': options.only = value(); break;
      case '--break': options.breakCase = value(); break;
      case '--out': options.out = path.resolve(value()); break;
      default: throw new Error(`unknown argument ${flag}`);
    }
  }
  return options;
}

async function until(what, check, timeoutMs = 60000) {
  const deadline = Date.now() + timeoutMs;
  for (;;) {
    const value = await check();
    if (value) return value;
    if (Date.now() > deadline) throw new Error(`timed out waiting for ${what}`);
    await wait(100);
  }
}

const site = (id, name, parentId = 0, content = page(name, `seed-${hex()}`)) => ({ id, type: 'site', name, parentId, content });
const folderNode = (id, name, parentId = 0) => ({ id, type: 'folder', name, parentId });
const GOOD = 'good.html';

// Every file under base that is neither in the folder nor in the host's config.
function outsideFiles(world) {
  const inside = [world.folder + path.sep, path.join(world.base, 'config') + path.sep];
  return walk(world.base).filter((file) => !inside.some((prefix) => file.startsWith(prefix)));
}

// Overrides what the fake says about one node: its declared size or its etag.
function lieAbout(server, id, { size, etag }) {
  const list = server._listEntry.bind(server);
  server._listEntry = (node) => {
    const entry = list(node);
    if (node.id !== id) return entry;
    return { ...entry, ...(size !== undefined ? { size } : {}), ...(etag !== undefined ? { etag } : {}) };
  };
  const content = server.getNodeContent.bind(server);
  server.getNodeContent = async (conn, nodeId) => {
    const answer = await content(conn, nodeId);
    if (nodeId !== id) return answer;
    return { ...answer, ...(size !== undefined ? { size } : {}), ...(etag !== undefined ? { etag } : {}) };
  };
}

// Root inside a user namespace (`unshare --map-root-user --mount`) mounts with no
// sudo; elsewhere passwordless sudo does.
const asRoot = () => process.getuid && process.getuid() === 0;
const privileged = (args) => execFileSync(asRoot() ? args[0] : 'sudo', asRoot() ? args.slice(1) : ['-n', ...args]);
const canSudo = () => {
  if (asRoot()) return process.platform === 'linux';
  try {
    execFileSync('sudo', ['-n', 'true'], { stdio: 'ignore' });
    return process.platform === 'linux';
  } catch {
    return false;
  }
};

// Each case answers its nodes, how to set the world up, what to do once synced,
// and a check of its own that answers true or a sentence.
const cases = {
  'dot-dot': {
    // hyperclay.com refuses this name (file-command-service.js), so only a broken
    // server sends it; the engine stops that folder's first download (H2-d), so
    // the good document is not required to land.
    serverRefuses: true,
    nodes: () => [site(11, GOOD), site(12, '../x.html')],
    check: (world) => !existsSync(path.join(world.base, 'x.html')) || 'x.html was written beside the folder',
    after: (world, { breakCase }) => {
      if (breakCase === 'outside') writeFileSync(path.join(world.base, 'x.html'), 'escaped');
    },
  },
  'hyperclay-versions': {
    // hyperclay.com refuses names starting with a dot; Go refuses the write into
    // its bookkeeping (H2-c), and the folder's first download stops as in H2-d.
    serverRefuses: true,
    nodes: () => [site(11, GOOD), folderNode(12, '.hyperclay'), folderNode(13, 'versions', 12), site(14, 'x.html', 13, page('x.html', 'planted'))],
    check: (world) => !walk(path.join(world.folder, '.hyperclay')).some((file) => readFileSync(file, 'utf8').includes('planted'))
      || 'a server node was written into .hyperclay/',
  },
  nul: {
    // hyperclay.com refuses this name (file-command-service.js), so only a broken
    // server sends it; the engine stops that folder's first download (H2-d), so
    // the good document is not required to land.
    serverRefuses: true,
    nodes: () => [site(11, GOOD), site(12, 'a\u0000b.html')],
    check: (world) => !readdirSync(world.folder).some((name) => name.startsWith('a') && name !== GOOD) || `a file was made from the NUL name: ${readdirSync(world.folder).join(', ')}`,
  },
  'long-name': {
    // hyperclay.com refuses this name (file-command-service.js), so only a broken
    // server sends it; the engine stops that folder's first download (H2-d), so
    // the good document is not required to land.
    serverRefuses: true,
    nodes: () => [site(11, GOOD), site(12, `${'a'.repeat(295)}.html`)],
    check: () => true,
  },
  'case-twins': {
    nodes: () => [site(11, GOOD), site(12, 'Case.html', 0, page('Case.html', 'upper')), site(13, 'case.html', 0, page('case.html', 'lower'))],
    // Each twin, where both are on disk, keeps its own bytes; on a folder that
    // folds case, one is held rather than written over the other.
    check: (world) => {
      const names = readdirSync(world.folder);
      const upper = names.includes('Case.html') ? readFileSync(path.join(world.folder, 'Case.html'), 'utf8') : null;
      const lower = names.includes('case.html') ? readFileSync(path.join(world.folder, 'case.html'), 'utf8') : null;
      if (upper !== null && lower !== null && upper === lower) return 'the case twins hold the same bytes on disk';
      return true;
    },
  },
  'link-outside': {
    nodes: () => [site(11, GOOD)],
    // A local link to a folder outside, with a file in it, then the server makes a
    // folder of the same name with a document in it.
    after: async (world) => {
      const outside = path.join(world.base, 'outside');
      mkdirSync(outside, { recursive: true });
      writeFileSync(path.join(outside, 'secret.html'), page('secret.html', 'OUTSIDE-SECRET'));
      symlinkSync(outside, path.join(world.folder, 'out'));
      const made = await world.server.createNode(null, { type: 'folder', name: 'out', parentId: 0 });
      const doc = await world.server.createNode(null, { type: 'site', name: 'x.html', parentId: made.id, content: page('x.html', 'through-link') });
      world.fake.push({ type: 'node-saved', nodeId: doc.id, nodeType: 'site' });
      world.outsideAllowed = ['outside/secret.html'];
    },
    check: (world) => {
      const outside = readdirSync(path.join(world.base, 'outside'));
      if (outside.length !== 1) return `the server wrote through the link: outside holds ${outside.join(', ')}`;
      const leaked = [...world.server.nodes.values()].some((node) => node.content && node.content.toString('utf8').includes('OUTSIDE-SECRET'));
      return !leaked || 'a file outside the folder was uploaded through the link';
    },
  },
  'huge-size': {
    nodes: () => [site(11, GOOD), site(12, 'big.html')],
    setup: (world) => lieAbout(world.server, 12, { size: 5 * 1024 ** 3 }),
    check: () => true,
  },
  'etag-slash': {
    nodes: () => [site(11, GOOD), site(12, 'd.html')],
    // A conflict on d.html while the server names its copy with an etag that climbs.
    after: async (world) => {
      lieAbout(world.server, 12, { etag: '../../../../escape/x' });
      await world.d.send('pause', { paused: true });
      const { host, port } = await world.served();
      const answer = await save({ port, host, doc: 'd.html', body: page('d.html', `L:${hex()}`) });
      if (answer.status !== 200) throw new Error(`the save of d.html answered ${answer.status ?? answer.error}`);
      const current = (await world.server.getNodeContent(null, 12)).content;
      await world.server.putNodeContent(null, 12, withMarker(String(current), `S:${hex()}`));
      await world.d.send('pause', { paused: false });
      await world.d.send('idle', { timeoutMs: 120000 });
    },
    check: (world) => {
      const escaped = walk(world.base).filter((file) => file.includes(`${path.sep}escape${path.sep}`) && !file.startsWith(path.join(world.folder, '.hyperclay', 'conflicts') + path.sep));
      return escaped.length === 0 || `the kept copy left .hyperclay/conflicts/: ${escaped.join(', ')}`;
    },
  },
  'disk-full': {
    nodes: () => [site(11, GOOD), ...Array.from({ length: 30 }, (_, i) => site(12 + i, `f${i}.html`, 0, page(`f${i}.html`, `big-${i}-${'x'.repeat(8000)}`)))],
    needsSudo: true,
    // A 64 KB disk at the folder: the first download runs out of room part way.
    beforeSetUp: (folder) => {
      mkdirSync(folder, { recursive: true });
      privileged(['mount', '-t', 'tmpfs', '-o', `size=64k,uid=${process.getuid()},gid=${process.getgid()}`, 'tmpfs', folder]);
    },
    waitForGood: false,
    check: () => true,
    cleanup: (world) => privileged(['umount', '-l', world.folder]),
  },
  'read-only': {
    nodes: () => [site(11, GOOD), site(12, 'd.html')],
    // The folder made read only after the first download, then a server edit and a
    // new document; once writable again, both land.
    after: async (world) => {
      chmodSync(world.folder, 0o555);
      const current = (await world.server.getNodeContent(null, 12)).content;
      world.readOnlyMarker = `S:${hex()}`;
      await world.server.putNodeContent(null, 12, withMarker(String(current), world.readOnlyMarker));
      world.fake.push({ type: 'node-saved', nodeId: 12, nodeType: 'site' });
      const made = await world.server.createNode(null, { type: 'site', name: 'new.html', parentId: 0, content: page('new.html', 'new') });
      world.fake.push({ type: 'node-saved', nodeId: made.id, nodeType: 'site' });
      await wait(8000);
      chmodSync(world.folder, 0o755);
    },
    // Once writable again, the engine's own retries land both, without a restart.
    check: async (world) => {
      const landed = () => readFileSync(path.join(world.folder, 'd.html'), 'utf8').includes(world.readOnlyMarker) && existsSync(path.join(world.folder, 'new.html'));
      return await until('the read-only folder to catch up', landed, 90000).then(() => true, () => 'once writable again, the server edit and the new document did not land within 90 s');
    },
  },
};

async function runCase(name, { out, built, breakCase }) {
  const spec = cases[name];
  if (spec.needsSudo && !canSudo()) return { skipped: true, problems: [] };
  const nodes = spec.nodes();
  const good = nodes.find((node) => node.name === GOOD);
  const world = await startWorld({ out, built, nodes, waitForFirst: false, beforeSetUp: spec.beforeSetUp ?? null });
  const problems = [];
  try {
    if (spec.setup) spec.setup(world);
    if (spec.waitForGood !== false && !spec.serverRefuses) {
      await world.d.send('landed', { folder: world.folder, files: { [GOOD]: sha(Buffer.from(good.content)) }, timeoutMs: 120000 });
    }
    if (spec.serverRefuses) await wait(5000);
    else await world.d.send('idle', { timeoutMs: 120000 }).catch((error) => problems.push(`never settled: ${error.message}`));
    if (spec.after) await spec.after(world, { breakCase });
    await wait(2000);
    if (!spec.serverRefuses) await world.d.send('idle', { timeoutMs: 120000 }).catch((error) => problems.push(`never settled after: ${error.message}`));
    const snap = await world.d.send('snapshot').catch(() => null);
    if (!snap) problems.push('the host stopped answering');
    const outside = outsideFiles(world).map((file) => path.relative(world.base, file)).filter((rel) => !(world.outsideAllowed ?? []).includes(rel));
    if (outside.length) problems.push(`written outside the folder: ${outside.join(', ')}`);
    const half = Object.keys(syncedFiles(world.folder))
      .filter((rel) => rel.endsWith('.html') && !readFileSync(path.join(world.folder, rel), 'utf8').trimEnd().endsWith('</html>'));
    if (half.length) problems.push(`half files ${half.join(', ')}`);
    if (walk(path.join(world.folder, '.hyperclay')).some((file) => /seed-|planted/.test(readFileSync(file, 'utf8')) && !file.includes(`${path.sep}versions${path.sep}`) && !file.includes(`${path.sep}conflicts${path.sep}`))) {
      problems.push('server bytes landed in .hyperclay/');
    }
    const verdict = await spec.check(world);
    if (verdict !== true) problems.push(verdict);
    const row = snap?.folders.find((f) => f.fullPath === world.folder || f.path === world.folder);
    const goodLanded = existsSync(path.join(world.folder, GOOD));
    const became = `good=${goodLanded ? 'landed' : 'not landed'} state=${row?.sync?.state ?? '?'} waiting=${JSON.stringify((row?.waiting ?? []).map((w) => w.kind ?? w))} files=${Object.keys(syncedFiles(world.folder)).length}`;
    return { world, problems, became };
  } catch (error) {
    problems.push(error.message);
    return { world, problems, became: '' };
  } finally {
    if (spec.cleanup) {
      try {
        spec.cleanup(world);
      } catch {}
    }
  }
}

async function main() {
  const options = parseArgs(process.argv.slice(2));
  const out = options.out ?? mkdtempSync(path.join(os.tmpdir(), 'hunt-sync-hostile-'));
  mkdirSync(out, { recursive: true });
  const built = build(out);
  const names = options.only ? [options.only] : Object.keys(cases);
  for (const name of names) if (!cases[name]) throw new Error(`no case ${name}`);
  const results = [];
  for (const name of names) {
    const { world, problems, became, skipped } = await runCase(name, { out, built, breakCase: options.breakCase });
    if (skipped) {
      console.log(`HOSTILE ${name} skipped (needs passwordless sudo on Linux)`);
      results.push({ name, skipped: true, problems: [] });
      continue;
    }
    results.push({ name, problems, became });
    console.log(`HOSTILE ${name} ${problems.length ? `FAILED ${problems.join('; ')}` : 'ok'} (${became})`);
    await world.close().catch(() => {});
    try {
      chmodSync(world.folder, 0o755);
    } catch {}
    if (!problems.length) rmSync(world.base, { recursive: true, force: true });
  }
  writeFileSync(path.join(out, 'hostile-sweep.json'), `${JSON.stringify(results, null, 2)}\n`);
  const failed = results.filter((r) => r.problems.length);
  const ran = results.filter((r) => !r.skipped).length;
  if (failed.length === 0 && ran > 0) {
    console.log(`SYNC HOSTILE SWEEP OK cases=${ran} skipped=${results.length - ran}`);
    return 0;
  }
  console.log(`SYNC HOSTILE SWEEP FAILED ${failed.length} of ${results.length}: ${failed.map((r) => r.name).join(', ')}`);
  return 1;
}

main().then((code) => process.exit(code), (error) => {
  console.error(`hunt-sync-hostile: ${error.stack || error.message}`);
  process.exit(2);
});
