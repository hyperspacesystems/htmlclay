// The sync race sweep (08-sync.md 8.53): two things that touch the same file at
// nearly the same moment, on the real pair and L1's fake, each run at several
// offsets between the two:
//
//   download-vs-save     a server edit announced and a tab's save of the same document
//   server-vs-unuploaded a tab's save, then a server edit before the save is uploaded
//   download-vs-rename   a server edit announced and a local rename of the same file
//   theirs-vs-save       Keep theirs on a held conflict and a tab's save of that document
//   double-restart       the service killed twice in a row, saves running
//   restart-mid-bind     the service killed during the first download, a page that landed being saved
//   gap                  the host killed (a new event epoch, so a gap) while files are written
//
// After each, a full comparison: no marker lost, no half file, the folder and the
// server agree apart from held conflicts, at most one service at any sampled
// moment, and the race's own rule (a local edit the server overtook is a held
// conflict or still live, never only an old version). Prints `RACE <case> ok|FAILED
// <why>` per case and `SYNC RACE SWEEP OK cases=<n>` or `SYNC RACE SWEEP FAILED ...`.
//
//   node scripts/hunt-sync-races.mjs [--only <race>] [--offsets 0,25,100] [--break unuploaded]
//
// --break unuploaded settles server-vs-unuploaded's conflict with Keep theirs
// after the run, leaving the state a silent replace would: it must fail.
import { execFileSync } from 'node:child_process';
import { mkdirSync, mkdtempSync, readFileSync, renameSync, rmSync, writeFileSync } from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import {
  build, converge, hex, page, presentMarkers, save, sha, startWorld, syncedFiles, wait, withMarker,
} from './sync-harness.mjs';

function parseArgs(argv) {
  const options = { only: null, offsets: [0, 25, 100], breakCase: null, out: null };
  for (let i = 0; i < argv.length; i += 1) {
    const flag = argv[i];
    const value = () => {
      i += 1;
      if (i >= argv.length) throw new Error(`${flag} needs a value`);
      return argv[i];
    };
    switch (flag) {
      case '--only': options.only = value(); break;
      case '--offsets': options.offsets = value().split(',').map(Number); break;
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
    await wait(10);
  }
}

const docNodes = (count) => Array.from({ length: count }, (_, index) => {
  const doc = `d${index}.html`;
  return { id: 11 + index, type: 'site', name: doc, parentId: 0, content: page(doc, `seed-${doc}-${hex()}`) };
});

const readText = (file) => {
  try {
    return readFileSync(file, 'utf8');
  } catch {
    return '';
  }
};

async function serverEdit(world, doc, marker) {
  const id = world.idOf(doc);
  const current = (await world.server.getNodeContent(null, id)).content;
  await world.server.putNodeContent(null, id, withMarker(Buffer.isBuffer(current) ? current.toString('utf8') : String(current), marker));
  world.fake.push({ type: 'node-saved', nodeId: id, nodeType: 'site' });
}

async function tabSave(world, doc, marker) {
  const { host, port } = await world.served();
  const answer = await save({ port, host, doc, body: page(doc, marker) });
  if (answer.status !== 200) throw new Error(`the save of ${doc} answered ${answer.status ?? answer.error}`);
}

// How many live-room services run for this world at once, from ps.
function servicesRunning(world) {
  const marker = `--data-dir ${path.join(world.base, 'config', 'htmlclay', 'service')}`;
  return execFileSync('ps', ['-A', '-o', 'command='], { encoding: 'utf8' }).split('\n').filter((line) => line.includes(marker)).length;
}

// The text of doc live on either side, for "still live" rules.
function liveText(world, doc) {
  const id = world.idOf(doc);
  const remote = id === null ? '' : world.server.get(id).content.toString('utf8');
  return readText(path.join(world.folder, doc)) + remote;
}

const races = {
  async 'download-vs-save'(world, offset) {
    const remote = `S:d0:${hex()}`;
    const local = `L:d0:${hex()}`;
    const editing = serverEdit(world, 'd0.html', remote);
    await wait(offset);
    await tabSave(world, 'd0.html', local);
    await editing;
    return { markers: [remote, local], mayConflict: ['d0.html'] };
  },

  async 'server-vs-unuploaded'(world, offset, { breakCase }) {
    const local = `L:d1:${hex()}`;
    const remote = `S:d1:${hex()}`;
    await tabSave(world, 'd1.html', local);
    await wait(offset);
    await serverEdit(world, 'd1.html', remote);
    return {
      markers: [local, remote], mayConflict: ['d1.html'],
      // The state a silent replace leaves: the server's bytes live, the save only in a
      // kept version, no conflict held (made here by choosing Keep theirs).
      afterConverge: breakCase === 'unuploaded'
        ? async () => {
          const held = (await world.d.send('conflicts')).find((c) => c.path === 'd1.html');
          if (held) await world.d.send('resolve', { conflict: held, keep: 'theirs' });
        }
        : null,
      rule: (conflicts) => {
        const live = liveText(world, 'd1.html');
        const held = conflicts.some((c) => c.path === 'd1.html') || Object.keys(syncedFiles(world.folder)).some((rel) => /conflicted/i.test(rel));
        return (live.includes(local) && live.includes(remote)) || held || 'the tab\'s save of d1.html was replaced with no conflict held';
      },
    };
  },

  async 'download-vs-rename'(world, offset) {
    const remote = `S:d2:${hex()}`;
    const editing = serverEdit(world, 'd2.html', remote);
    await wait(offset);
    try {
      renameSync(path.join(world.folder, 'd2.html'), path.join(world.folder, 'd2-renamed.html'));
    } catch (error) {
      if (error.code !== 'ENOENT') throw error;
    }
    await editing;
    return {
      markers: [remote], mayConflict: ['d2.html', 'd2-renamed.html'],
      rule: () => Object.values(Object.fromEntries(Object.keys(syncedFiles(world.folder)).map((rel) => [rel, readText(path.join(world.folder, rel))])))
        .some((text) => text.includes(remote)) || 'the server edit of d2.html is not in any live file after the rename',
    };
  },

  async 'theirs-vs-save'(world, offset) {
    await world.d.send('pause', { paused: true });
    const local = `L:d3:${hex()}`;
    const remote = `S:d3:${hex()}`;
    await tabSave(world, 'd3.html', local);
    await serverEdit(world, 'd3.html', remote);
    await world.d.send('pause', { paused: false });
    await until('the conflict on d3.html to be held', async () => (await world.d.send('conflicts')).find((c) => c.path === 'd3.html'));
    await world.d.send('idle', { timeoutMs: 120000 });
    const conflict = (await world.d.send('conflicts')).find((c) => c.path === 'd3.html');
    const late = `L2:d3:${hex()}`;
    const choosing = world.d.send('resolve', { conflict, keep: 'theirs' }).catch(() => {});
    await wait(offset);
    await tabSave(world, 'd3.html', late);
    await choosing;
    return { markers: [local, remote, late], mayConflict: ['d3.html'] };
  },

  async 'double-restart'(world, offset) {
    const markers = [];
    let saving = true;
    const saves = (async () => {
      let n = 0;
      while (saving) {
        const marker = `L:d0:n${n}:${hex()}`;
        n += 1;
        const { host, port } = await world.served();
        if ((await save({ port, host, doc: 'd0.html', body: page('d0.html', marker) })).status === 200) markers.push(marker);
        await wait(50);
      }
    })();
    await world.d.send('restartService', { timeoutMs: 60000 });
    await wait(offset);
    await world.d.send('restartService', { timeoutMs: 60000 });
    await wait(1000);
    saving = false;
    await saves;
    // A kill in the few ms between the server's answer and the saved baseline can leave one
    // both-edited conflict with both copies kept (08-sync.md F-8.55l); every save must still land.
    return { markers, mayConflict: ['d0.html'] };
  },

  async 'restart-mid-bind'(world) {
    const markers = [];
    let saving = true;
    const logged = readText(world.log).length;
    const seed = world.server.get(world.idOf('d0.html')).content;
    await world.d.send('landed', { folder: world.folder, files: { 'd0.html': sha(Buffer.from(seed)) }, timeoutMs: 120000 });
    const saves = (async () => {
      let n = 0;
      while (saving) {
        const marker = `L:d0:n${n}:${hex()}`;
        n += 1;
        const { host, port } = await world.served();
        if ((await save({ port, host, doc: 'd0.html', body: page('d0.html', marker) })).status === 200) markers.push(marker);
        await wait(50);
      }
    })();
    await until('a save of d0.html', async () => markers.length > 0, 30000);
    await world.d.send('restartService', { timeoutMs: 60000 });
    await wait(1500);
    saving = false;
    await saves;
    const interrupted = readText(world.log).slice(logged).includes('stopped before an answer');
    return { markers, mayConflict: [], rule: () => interrupted || 'the first download finished before the restart, so nothing was tested' };
  },

  async gap(world, offset) {
    const markers = [];
    let writing = true;
    const writes = (async () => {
      let n = 0;
      while (writing) {
        const marker = `G:n${n}:${hex()}`;
        writeFileSync(path.join(world.folder, `g${n}.html`), page(`g${n}.html`, marker));
        markers.push(marker);
        n += 1;
        await wait(40);
      }
    })();
    await wait(offset);
    await world.d.send('restartHost', { timeoutMs: 60000 });
    await wait(500);
    writing = false;
    await writes;
    return {
      markers, mayConflict: [],
      rule: () => {
        const remote = new Set();
        for (const node of world.server.nodes.values()) {
          if (node.content) for (const m of node.content.toString('utf8').matchAll(/data-soak="([^"]+)"/g)) remote.add(m[1]);
        }
        const missing = markers.filter((marker) => !remote.has(marker));
        return missing.length === 0 || `${missing.length} files written around the gap never reached the server`;
      },
    };
  },
};

async function runCase(name, offset, { out, built, breakCase }) {
  const midBind = name === 'restart-mid-bind';
  const world = await startWorld({ out, built, nodes: docNodes(midBind ? 80 : 4), waitForFirst: !midBind });
  world.log = path.join(out, 'host.log');
  let most = 0;
  let sampling = true;
  const sampler = (async () => {
    while (sampling) {
      most = Math.max(most, servicesRunning(world));
      await wait(20);
    }
  })();
  try {
    const result = await races[name](world, offset, { breakCase });
    sampling = false;
    await sampler;
    const { conflicts, differ } = await converge(world);
    let after = differ;
    if (result.afterConverge) {
      await result.afterConverge();
      after = (await converge(world)).differ;
    }
    const problems = [];
    const present = presentMarkers(world.folder, world.server);
    const lost = result.markers.filter((marker) => !present.has(marker));
    if (lost.length) problems.push(`lost ${lost.slice(0, 3).join(', ')}${lost.length > 3 ? ` and ${lost.length - 3} more` : ''}`);
    const half = Object.keys(syncedFiles(world.folder))
      .filter((rel) => rel.endsWith('.html') && !readText(path.join(world.folder, rel)).trimEnd().endsWith('</html>'));
    if (half.length) problems.push(`half files ${half.join(', ')}`);
    if (after.length) problems.push(`folder and server differ at ${JSON.stringify(after.slice(0, 3))}`);
    const stray = conflicts.filter((c) => !result.mayConflict.includes(c.path));
    if (stray.length) problems.push(`conflicts with no reason: ${stray.map((c) => `${c.path} (${c.kind}${c.reason ? `, ${c.reason}` : ''})`).join(', ')}`);
    if (most > 1) problems.push(`${most} services ran at once`);
    if (result.rule) {
      const verdict = result.rule(conflicts);
      if (verdict !== true) problems.push(verdict);
    }
    return { world, problems, conflicts: conflicts.length, markers: result.markers.length };
  } catch (error) {
    return { world, problems: [error.message], conflicts: 0, markers: 0 };
  } finally {
    sampling = false;
  }
}

async function main() {
  const options = parseArgs(process.argv.slice(2));
  const out = options.out ?? mkdtempSync(path.join(os.tmpdir(), 'hunt-sync-races-'));
  mkdirSync(out, { recursive: true });
  const built = build(out);
  const names = options.only ? [options.only] : Object.keys(races);
  for (const name of names) if (!races[name]) throw new Error(`no race ${name}`);
  const results = [];
  for (const name of names) {
    for (const offset of options.offsets) {
      const label = `${name}/${offset}ms`;
      const { world, problems, conflicts, markers } = await runCase(name, offset, { out, built, breakCase: options.breakCase });
      results.push({ label, problems });
      console.log(`RACE ${label} ${problems.length ? `FAILED ${problems.join('; ')}` : 'ok'} markers=${markers} conflicts=${conflicts}`);
      await world.close().catch(() => {});
      if (!problems.length) rmSync(world.base, { recursive: true, force: true });
    }
  }
  writeFileSync(path.join(out, 'race-sweep.json'), `${JSON.stringify(results, null, 2)}\n`);
  const failed = results.filter((r) => r.problems.length);
  if (failed.length === 0 && results.length === names.length * options.offsets.length) {
    console.log(`SYNC RACE SWEEP OK cases=${results.length}`);
    return 0;
  }
  console.log(`SYNC RACE SWEEP FAILED ${failed.length} of ${results.length}: ${failed.map((r) => r.label).join(', ')}`);
  return 1;
}

main().then((code) => process.exit(code), (error) => {
  console.error(`hunt-sync-races: ${error.stack || error.message}`);
  process.exit(2);
});
