// Probe: an HTML file that is not UTF-8 (an older page saved as windows-1252)
// is uploaded with every non-ASCII byte replaced by U+FFFD, so the server's copy
// differs from the file on disk, and each full comparison uploads it again.
//
// Prints `FAILED non-utf8-upload: ...` while the bug exists and `OK non-utf8-upload`
// once it is gone (the same bytes on the server, or the file left unsynced with
// the server holding nothing changed). Exits 1 on FAILED.
//
//   node scripts/probes/non-utf8-upload.mjs --out <dir>
import { mkdirSync, readFileSync, writeFileSync } from 'node:fs';
import path from 'node:path';
import { build, converge, hex, page, sha, startWorld, wait } from '../sync-harness.mjs';

const argv = process.argv.slice(2);
const flag = (name) => (argv.includes(name) ? argv[argv.indexOf(name) + 1] : null);
const out = path.resolve(flag('--out') || 'probe-out');
mkdirSync(out, { recursive: true });

const doc = 'menu.html';
const latin1 = (text) => Buffer.from(text, 'latin1');
const bytes = latin1(`<!doctype html>\n<html><head><meta charset="windows-1252"><title>Café</title></head><body><p data-soak="legacy-${hex()}">Crème brûlée, 4£</p></body></html>\n`);
const replacement = Buffer.from('�', 'utf8');
const count = (buffer, needle) => {
  let found = 0;
  for (let at = buffer.indexOf(needle); at !== -1; at = buffer.indexOf(needle, at + needle.length)) found += 1;
  return found;
};

const built = build(out);
const world = await startWorld({ out, built, nodes: [{ id: 11, type: 'site', name: 'keep.html', parentId: 0, content: page('keep.html', `seed-keep-${hex()}`) }] });
const { server, folder } = world;
let failed = false;
try {
  writeFileSync(path.join(folder, doc), bytes);
  const deadline = Date.now() + 60000;
  while (!server.hasPath(doc) && Date.now() < deadline) await wait(500);
  if (!server.hasPath(doc)) await converge(world, { limitMs: 20000 });
  if (!server.hasPath(doc)) {
    console.log(`OK non-utf8-upload: ${doc} was not uploaded at all`);
  } else {
    await wait(3000);
    const writes = () => server.calls.filter((call) => (call.name === 'putNodeContent' || call.name === 'createNode') && (call.pathBefore === doc || call.args[0] === doc)).length;
    const sent = server.get(world.idOf(doc)).content;
    const local = readFileSync(path.join(folder, doc));
    const firstWrites = writes();
    await converge(world, { limitMs: 20000 });
    await converge(world, { limitMs: 20000 });
    const laterWrites = writes() - firstWrites;
    console.log(`local ${doc}: ${local.length} bytes sha ${sha(local).slice(0, 12)}, unchanged since written: ${local.equals(bytes)}`);
    console.log(`server ${doc}: ${sent.length} bytes sha ${sha(sent).slice(0, 12)}, U+FFFD count ${count(sent, replacement)}`);
    console.log(`uploads of ${doc} in two more full comparisons with nothing changed: ${laterWrites}`);
    if (sent.equals(bytes)) {
      console.log('OK non-utf8-upload: the server holds the same bytes as the file');
    } else {
      failed = true;
      console.log(`FAILED non-utf8-upload: the server's ${doc} is not the file on disk, ${count(sent, replacement)} characters were replaced by U+FFFD (server title: ${/<title>(.*?)<\/title>/.exec(sent.toString('utf8'))?.[1]})`);
    }
  }
} catch (error) {
  failed = true;
  console.log(`INCONCLUSIVE non-utf8-upload: ${error.stack || error.message}`);
} finally {
  await world.close();
}
process.exit(failed ? 1 : 0);
