// Probe: a file that sync moves to the folder's .trash/ replaces an earlier
// trashed file of the same path, so the earlier copy is gone from this computer.
// Two cases, each in its own world:
//
//   twice-deleted  the server deletes journal.html, a new journal.html is made and deleted too
//   kept-edits     local edits meet a server delete, the person picks Keep theirs (the edited
//                  copy goes to .trash/), then a new journal.html is made and deleted
//
// Prints `FAILED trash-overwrite <case>: ...` while the bug exists and
// `OK trash-overwrite <case>` once it is gone. Exits 1 on any FAILED.
//
//   node scripts/probes/trash-overwrite.mjs --out <dir> [--only twice-deleted|kept-edits]
import { existsSync, mkdirSync, readFileSync, writeFileSync } from 'node:fs';
import path from 'node:path';
import { build, converge, hex, markersOf, page, startWorld, wait, walk } from '../sync-harness.mjs';

const argv = process.argv.slice(2);
const flag = (name) => (argv.includes(name) ? argv[argv.indexOf(name) + 1] : null);
const out = path.resolve(flag('--out') || 'probe-out');
const only = flag('--only');
mkdirSync(out, { recursive: true });

const doc = 'journal.html';

function whereLocal(folder, marker) {
  return walk(folder).filter((file) => markersOf(readFileSync(file, 'utf8')).has(marker)).map((file) => path.relative(folder, file));
}

function onServer(server, marker) {
  return [...server.nodes.values(), ...server.trash.values()].some((node) => node.content && markersOf(node.content.toString('utf8')).has(marker));
}

async function until(what, check, timeoutMs = 60000) {
  const deadline = Date.now() + timeoutMs;
  for (;;) {
    const value = await check();
    if (value) return value;
    if (Date.now() > deadline) throw new Error(`timed out waiting for ${what}`);
    await wait(500);
  }
}

async function secondLife(world, name, first) {
  const { server, folder } = world;
  const second = `second-${hex()}`;
  const created = await server.createNode(null, { type: 'site', name: doc, parentId: 0, content: page(doc, second) });
  await converge(world, { limitMs: 30000 });
  await until(`the new ${doc} to arrive`, () => existsSync(path.join(folder, doc)) && markersOf(readFileSync(path.join(folder, doc), 'utf8')).has(second));
  await server.deleteNode(null, created.id);
  await converge(world, { limitMs: 30000 });
  await until(`the new ${doc} to be trashed`, () => !existsSync(path.join(folder, doc)));
  const firstAt = whereLocal(folder, first);
  const secondAt = whereLocal(folder, second);
  console.log(`${name}: after the second delete, the first copy is at [${firstAt.join(', ')}], the second at [${secondAt.join(', ')}]`);
  console.log(`${name}: files under .trash: ${walk(path.join(folder, '.trash')).map((file) => path.relative(folder, file)).join(', ')}`);
  return { firstAt, firstOnServer: onServer(server, first) };
}

const cases = {
  async 'twice-deleted'(world, name) {
    const { server, folder } = world;
    const first = world.firstMarker;
    await server.deleteNode(null, world.idOf(doc));
    await converge(world, { limitMs: 30000 });
    await until(`${doc} to be trashed`, () => !existsSync(path.join(folder, doc)));
    console.log(`${name}: after the first delete, the first copy is at [${whereLocal(folder, first).join(', ')}]`);
    if (whereLocal(folder, first).length === 0) throw new Error('the first trashed copy never reached the folder');
    const { firstAt, firstOnServer } = await secondLife(world, name, first);
    if (firstAt.length > 0) return true;
    console.log(`FAILED trash-overwrite ${name}: the copy of ${doc} that sync trashed first is nowhere on this computer, .trash/${doc} now holds the later file (still in the server's trash: ${firstOnServer})`);
    return false;
  },
  async 'kept-edits'(world, name) {
    const { server, folder, d } = world;
    const edited = `local-edit-${hex()}`;
    await d.send('pause', { paused: true });
    writeFileSync(path.join(folder, doc), page(doc, edited));
    await server.deleteNode(null, world.idOf(doc));
    await converge(world, { limitMs: 30000 });
    const conflict = await until('the held conflict', async () => (await d.send('conflicts')).find((c) => c.path === doc), 60000);
    console.log(`${name}: held conflict kind=${conflict.kind}`);
    await until('Keep theirs to be accepted', async () => {
      const held = (await d.send('conflicts')).find((c) => c.path === doc);
      if (!held) return true;
      return d.send('resolve', { conflict: held, keep: 'theirs' }).then(() => true, () => false);
    }, 60000);
    await until(`${doc} to leave the folder`, () => !existsSync(path.join(folder, doc)));
    await d.send('idle', { timeoutMs: 60000 });
    console.log(`${name}: after Keep theirs, the edited copy is at [${whereLocal(folder, edited).join(', ')}], on the server: ${onServer(server, edited)}`);
    if (whereLocal(folder, edited).length === 0) throw new Error('Keep theirs left no copy of the edits to overwrite');
    const { firstAt, firstOnServer } = await secondLife(world, name, edited);
    if (firstAt.length > 0 || firstOnServer) return true;
    console.log(`FAILED trash-overwrite ${name}: the person's edits to ${doc}, which Keep theirs moved to .trash/${doc}, are now nowhere on this computer and nowhere on the server`);
    return false;
  },
};

const built = build(out);
let failed = false;
for (const name of Object.keys(cases)) {
  if (only && only !== name) continue;
  const caseOut = path.join(out, name);
  mkdirSync(caseOut, { recursive: true });
  const firstMarker = `first-${hex()}`;
  const nodes = [
    { id: 11, type: 'site', name: 'keep.html', parentId: 0, content: page('keep.html', `seed-keep-${hex()}`) },
    { id: 12, type: 'site', name: doc, parentId: 0, content: page(doc, firstMarker) },
  ];
  let world = null;
  try {
    world = await startWorld({ out: caseOut, built, nodes });
    world.firstMarker = firstMarker;
    if (await cases[name](world, name)) console.log(`OK trash-overwrite ${name}: the earlier copy is still on this computer`);
    else failed = true;
  } catch (error) {
    failed = true;
    console.log(`INCONCLUSIVE trash-overwrite ${name}: ${error.stack || error.message}`);
  } finally {
    if (world) await world.close();
  }
}
process.exit(failed ? 1 : 0);
