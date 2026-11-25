// The sync kill sweep (08-sync.md 8.52): the service, and then the host, killed
// at each moment sync is most exposed, on the real pair and L1's fake. The harness
// watches from outside (the folder, the fake's request log) and kills the moment
// the point arrives, so the product carries no kill hook:
//
//   first-download   after 10 of 40 files of the first download have landed
//   download-commit  the instant a server edit lands in the folder, before the
//                    engine can save its baseline
//   kept-copy        the instant a kept server copy appears under
//                    .hyperclay/conflicts/, before the conflict record
//   keep-both        the instant Keep both's copy appears, between its two writes
//   upload           while the fake is answering an upload
//   configure        15 ms after a pause is sent, while the service reconfigures
//
// After each kill, a full comparison: no marker lost, no half file, the folder
// and the server agree apart from held conflicts, no conflict without a reason
// (two sides edited), and a held conflict is still held. Prints one
// `KILL <case> ok|FAILED <why>` line per case, then `SYNC KILL SWEEP OK cases=12`
// or `SYNC KILL SWEEP FAILED ...`.
//
//   node scripts/hunt-sync-kill.mjs [--only <point>] [--break upload]
//
// --break upload puts the old bytes back on the server after the upload case's
// kill, as an engine uploading stale bytes would: the sweep must print FAILED.
import { mkdirSync, mkdtempSync, readFileSync, rmSync, writeFileSync } from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import {
  build, converge, hex, page, presentMarkers, save, startWorld, syncedFiles, wait, walk, withMarker,
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

// Polls check every 5 ms until it answers something, or fails the case.
async function until(what, check, timeoutMs = 60000) {
  const deadline = Date.now() + timeoutMs;
  for (;;) {
    const value = await check();
    if (value) return value;
    if (Date.now() > deadline) throw new Error(`the kill point never came: ${what}`);
    await wait(5);
  }
}

const docNodes = (count) => Array.from({ length: count }, (_, index) => {
  const doc = `d${index}.html`;
  return { id: 11 + index, type: 'site', name: doc, parentId: 0, content: page(doc, `seed-${doc}-${hex()}`) };
});

const kill = (world, who) => world.d.send(who === 'service' ? 'restartService' : 'restartHost', { timeoutMs: 60000 });

const readText = (file) => {
  try {
    return readFileSync(file, 'utf8');
  } catch {
    return '';
  }
};

// A server edit to doc with marker, announced on the stream.
async function serverEdit(world, doc, marker) {
  const id = world.idOf(doc);
  const current = (await world.server.getNodeContent(null, id)).content;
  await world.server.putNodeContent(null, id, withMarker(Buffer.isBuffer(current) ? current.toString('utf8') : String(current), marker));
  world.fake.push({ type: 'node-saved', nodeId: id, nodeType: 'site' });
}

// Edits on both sides of doc while sync is paused, then sync on again: a conflict
// with a reason. Answers the two markers.
async function bothSides(world, doc) {
  const { host, port } = await world.served();
  await world.d.send('pause', { paused: true });
  const local = `L:${doc}:${hex()}`;
  const remote = `S:${doc}:${hex()}`;
  const answer = await save({ port, host, doc, body: page(doc, local) });
  if (answer.status !== 200) throw new Error(`the local save of ${doc} answered ${answer.status ?? answer.error}`);
  await serverEdit(world, doc, remote);
  return { local, remote };
}

const conflictsDir = (world) => path.join(world.folder, '.hyperclay', 'conflicts');

// Each point sets its world up, kills who at its moment, and answers what the
// judging needs: the markers that must survive, whether a conflict has a reason,
// the conflicted document, and a server check of its own.
const points = {
  async 'first-download'(who, { out, built }) {
    const nodes = docNodes(40);
    const world = await startWorld({ out, built, nodes, waitForFirst: false });
    await until('10 files of the first download', () => Object.keys(syncedFiles(world.folder)).length >= 10, 120000);
    await kill(world, who);
    return { world, markers: nodes.map((node) => [...node.content.matchAll(/data-soak="([^"]+)"/g)][0][1]), conflictReason: false };
  },

  async 'download-commit'(who, { out, built }) {
    const world = await startWorld({ out, built, nodes: docNodes(4) });
    const marker = `S:d0:${hex()}`;
    await serverEdit(world, 'd0.html', marker);
    await until('the server edit landing in d0.html', () => readText(path.join(world.folder, 'd0.html')).includes(marker));
    await kill(world, who);
    return {
      world, markers: [marker], conflictReason: false,
      serverCheck: () => world.server.get(world.idOf('d0.html')).content.toString('utf8').includes(marker) || 'the server lost its own edit to d0.html',
    };
  },

  async 'kept-copy'(who, { out, built }) {
    const world = await startWorld({ out, built, nodes: docNodes(4) });
    const { local, remote } = await bothSides(world, 'd1.html');
    const sending = world.d.send('pause', { paused: false });
    await until('a kept server copy under .hyperclay/conflicts/', () => walk(conflictsDir(world)).length > 0);
    await sending.catch(() => {});
    await kill(world, who);
    return { world, markers: [local, remote], conflictReason: true, conflicted: 'd1.html' };
  },

  async 'keep-both'(who, { out, built }) {
    const world = await startWorld({ out, built, nodes: docNodes(4) });
    const { local, remote } = await bothSides(world, 'd1.html');
    await world.d.send('pause', { paused: false });
    await until('the conflict on d1.html to be held', async () =>
      (await world.d.send('conflicts')).find((c) => c.path === 'd1.html'));
    await world.d.send('idle', { timeoutMs: 120000 });
    // A person whose click is refused (the list moved on, or the service was
    // restarting) sees the refusal and clicks again on the refreshed list.
    let resolving = null;
    let refused = null;
    let tries = 0;
    const click = async () => {
      tries += 1;
      const conflict = (await world.d.send('conflicts')).find((c) => c.path === 'd1.html');
      if (!conflict) throw new Error('the conflict on d1.html went away before Keep both');
      refused = null;
      resolving = world.d.send('resolve', { conflict, keep: 'both' }).catch((error) => {
        refused = error;
        console.log(`KEEP-BOTH try ${tries} refused ${error.code ?? error.message} for ${conflict.id}`);
      });
    };
    await click();
    await until('Keep both\'s copy of d1.html', async () => {
      if (refused) {
        if (tries >= 5) throw new Error(`Keep both was refused ${tries} times, last: ${refused.code ?? refused.message}`);
        await wait(500);
        await click();
        return false;
      }
      return Object.keys(syncedFiles(world.folder)).some((rel) => /conflicted/i.test(rel));
    });
    await kill(world, who);
    await resolving;
    return { world, markers: [local, remote], conflictReason: true, conflicted: 'd1.html', settledByChoice: true };
  },

  async upload(who, { out, built, breakCase }) {
    const world = await startWorld({ out, built, nodes: docNodes(4) });
    const { host, port } = await world.served();
    const before = world.server.get(world.idOf('d2.html')).content.toString('utf8');
    const from = world.fake.requests.length;
    const marker = `L:d2:${hex()}`;
    const answer = await save({ port, host, doc: 'd2.html', body: page('d2.html', marker) });
    if (answer.status !== 200) throw new Error(`the save of d2.html answered ${answer.status ?? answer.error}`);
    const id = world.idOf('d2.html');
    await until('the upload of d2.html', () => world.fake.requests.slice(from).some((r) => r.method === 'PUT' && r.path === `/_/sync/nodes/${id}/content`));
    await kill(world, who);
    return {
      world, markers: [marker], conflictReason: false,
      afterConverge: breakCase === 'upload' ? () => world.server.putNodeContent(null, id, before) : null,
      serverCheck: () => world.server.get(id).content.toString('utf8').includes(marker) || 'the server does not hold the uploaded save of d2.html',
    };
  },

  async configure(who, { out, built }) {
    const world = await startWorld({ out, built, nodes: docNodes(4) });
    const { host, port } = await world.served();
    const marker = `L:d3:${hex()}`;
    const answer = await save({ port, host, doc: 'd3.html', body: page('d3.html', marker) });
    if (answer.status !== 200) throw new Error(`the save of d3.html answered ${answer.status ?? answer.error}`);
    const pausing = world.d.send('pause', { paused: true }).catch(() => {});
    await wait(15);
    await kill(world, who);
    await pausing;
    return { world, markers: [marker], conflictReason: false };
  },
};

// The judging every case shares, after a full comparison.
async function judge(result) {
  const { world, markers, conflictReason, conflicted, settledByChoice, serverCheck, afterConverge } = result;
  const { conflicts, differ } = await converge(world);
  if (afterConverge) await afterConverge();
  const problems = [];
  const present = presentMarkers(world.folder, world.server);
  const lost = markers.filter((marker) => !present.has(marker));
  if (lost.length) problems.push(`lost ${lost.join(', ')}`);
  const half = Object.keys(syncedFiles(world.folder))
    .filter((rel) => rel.endsWith('.html') && !readText(path.join(world.folder, rel)).trimEnd().endsWith('</html>'));
  if (half.length) problems.push(`half files ${half.join(', ')}`);
  const after = afterConverge ? (await converge(world)).differ : differ;
  if (after.length) problems.push(`folder and server differ at ${JSON.stringify(after.slice(0, 3))}`);
  if (!conflictReason && conflicts.length) problems.push(`conflicts with no reason: ${conflicts.map((c) => c.path).join(', ')}`);
  if (conflictReason && !settledByChoice && !conflicts.some((c) => c.path === conflicted)) problems.push(`the conflict on ${conflicted} is no longer held`);
  if (settledByChoice && !conflicts.some((c) => c.path === conflicted)
    && !Object.keys(syncedFiles(world.folder)).some((rel) => /conflicted/i.test(rel))) {
    problems.push(`the conflict on ${conflicted} is neither held nor settled with both copies`);
  }
  if (serverCheck) {
    const verdict = serverCheck();
    if (verdict !== true) problems.push(verdict);
  }
  return { problems, conflicts: conflicts.length };
}

async function main() {
  const options = parseArgs(process.argv.slice(2));
  const out = options.out ?? mkdtempSync(path.join(os.tmpdir(), 'hunt-sync-kill-'));
  mkdirSync(out, { recursive: true });
  const built = build(out);
  const names = options.only ? [options.only] : Object.keys(points);
  for (const name of names) if (!points[name]) throw new Error(`no kill point ${name}`);
  const results = [];
  for (const name of names) {
    for (const who of ['service', 'host']) {
      const label = `${name}/${who}`;
      let world = null;
      try {
        const result = await points[name](who, { out, built, breakCase: options.breakCase });
        world = result.world;
        const { problems, conflicts } = await judge(result);
        results.push({ label, problems });
        console.log(`KILL ${label} ${problems.length ? `FAILED ${problems.join('; ')}` : 'ok'} conflicts=${conflicts}`);
      } catch (error) {
        results.push({ label, problems: [error.message] });
        console.log(`KILL ${label} FAILED ${error.message}`);
      } finally {
        if (world) {
          await world.close().catch(() => {});
          if (!results.at(-1).problems.length) rmSync(world.base, { recursive: true, force: true });
        }
      }
    }
  }
  writeFileSync(path.join(out, 'kill-sweep.json'), `${JSON.stringify(results, null, 2)}\n`);
  const failed = results.filter((r) => r.problems.length);
  if (failed.length === 0 && results.length === names.length * 2) {
    console.log(`SYNC KILL SWEEP OK cases=${results.length}`);
    return 0;
  }
  console.log(`SYNC KILL SWEEP FAILED ${failed.length} of ${results.length}: ${failed.map((r) => r.label).join(', ')}`);
  return 1;
}

main().then((code) => process.exit(code), (error) => {
  console.error(`hunt-sync-kill: ${error.stack || error.message}`);
  process.exit(2);
});
