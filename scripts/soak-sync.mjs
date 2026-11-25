// The sync ledger soak (08-sync.md 8.49): the real htmlclay and live-room service
// sync one folder with L1's fake sync server for a while. Tabs save through
// /_/save, the fake server edits the same documents, both sides create, rename
// and remove files of their own, the service and the host are killed on a
// schedule, and sync is paused and resumed. Every save the host acknowledged and
// every server edit carries a marker of its own. At the end, after a full
// comparison, each marker must be in the folder (a document, a kept version, the
// folder's .trash/, a kept server copy) or on the fake server (a node or its
// trash), and the folder and the server must hold the same files apart from held
// conflicts.
//
//   node scripts/soak-sync.mjs --minutes 10 --tabs 4 --kill-service-every 120 --kill-host-every 300
//
// Prints `SYNC LEDGER OK markers=<n> lost=0 conflicts=<n>` and exits 0, or
// `SYNC LEDGER FAILED ...` and exits 1. --break-download deletes, before the
// check, one kept version holding a marker found nowhere else (the bytes a
// download replaced), which must print FAILED: the proof the check can fail.
// --seed <dir or tar archive> copies a folder, or unpacks an archive, into the
// run's own directory (never touching the original) for the fake server's first
// state; its .html files become the documents the tabs and the server edit.
//
// Uses the fork's fake sync server (HTMLCLAY_SYNC_LOCAL, default
// service/test/fake). The folder sits inside home, as a served folder must.
import { execFileSync } from 'node:child_process';
import {
  appendFileSync, cpSync, existsSync, mkdirSync, mkdtempSync, readdirSync, readFileSync, renameSync, rmSync, statSync,
  unlinkSync, writeFileSync,
} from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import {
  build, converge, hex, markersOf, page, presentMarkers, save, skipped, startWorld, wait, walk, withMarker,
} from './sync-harness.mjs';

const stamp = () => new Date().toISOString();

function parseArgs(argv) {
  const options = {
    minutes: 10, tabs: 4, docs: 4, killServiceEvery: 120, killHostEvery: 300, pauseEvery: 90,
    out: null, seed: null, breakDownload: false,
  };
  for (let i = 0; i < argv.length; i += 1) {
    const flag = argv[i];
    const value = () => {
      i += 1;
      if (i >= argv.length) throw new Error(`${flag} needs a value`);
      return argv[i];
    };
    switch (flag) {
      case '--minutes': options.minutes = Number(value()); break;
      case '--tabs': options.tabs = Number(value()); break;
      case '--docs': options.docs = Number(value()); break;
      case '--kill-service-every': options.killServiceEvery = Number(value()); break;
      case '--kill-host-every': options.killHostEvery = Number(value()); break;
      case '--pause-every': options.pauseEvery = Number(value()); break;
      case '--out': options.out = path.resolve(value()); break;
      case '--seed': options.seed = path.resolve(value()); break;
      case '--break-download': options.breakDownload = true; break;
      default: throw new Error(`unknown argument ${flag}`);
    }
  }
  for (const key of ['minutes', 'tabs', 'docs', 'killServiceEvery', 'killHostEvery', 'pauseEvery']) {
    if (!Number.isFinite(options[key]) || options[key] < 0) throw new Error(`--${key} must be a number`);
  }
  if (options.tabs < 1 || options.docs < 1) throw new Error('--tabs and --docs must be at least 1');
  return options;
}

// The seed's documents and folders as fake server nodes, from a copy of a folder
// or an unpacked archive. Bookkeeping (.hyperclay/, .trash/) never travels.
function seedNodes(seed, out) {
  const copy = path.join(out, 'seed');
  if (statSync(seed).isDirectory()) {
    cpSync(seed, copy, { recursive: true, filter: (source) => !['.hyperclay', '.trash'].includes(path.basename(source)) });
  } else {
    mkdirSync(copy, { recursive: true });
    execFileSync('tar', ['-xf', seed, '-C', copy, '--exclude=.hyperclay', '--exclude=.trash']);
  }
  const nodes = [];
  const docs = [];
  let id = 10;
  const visit = (abs, rel, parentId) => {
    for (const entry of readdirSync(abs, { withFileTypes: true }).sort((a, b) => a.name.localeCompare(b.name))) {
      if (skipped(entry.name)) continue;
      const childRel = rel ? `${rel}/${entry.name}` : entry.name;
      if (entry.isDirectory()) {
        id += 1;
        nodes.push({ id, type: 'folder', name: entry.name, parentId });
        visit(path.join(abs, entry.name), childRel, id);
      } else if (entry.isFile() && /\.html$/i.test(entry.name)) {
        id += 1;
        nodes.push({ id, type: 'site', name: entry.name, parentId, content: readFileSync(path.join(abs, entry.name), 'utf8') });
        docs.push(childRel);
      }
    }
  };
  visit(copy, '', 0);
  return { nodes, docs };
}

async function main() {
  const options = parseArgs(process.argv.slice(2));
  const out = options.out ?? mkdtempSync(path.join(os.tmpdir(), 'soak-sync-'));
  mkdirSync(out, { recursive: true });
  const note = (line) => {
    const text = `${stamp()} ${line}`;
    console.log(text);
    appendFileSync(path.join(out, 'soak.log'), `${text}\n`);
  };
  const built = build(out);

  const seeded = options.seed
    ? seedNodes(options.seed, out)
    : (() => {
      const docs = Array.from({ length: options.docs }, (_, index) => `d${index}.html`);
      return { docs, nodes: docs.map((doc, index) => ({ id: 11 + index, type: 'site', name: doc, parentId: 0, content: page(doc, `seed-${doc}`) })) };
    })();
  if (seeded.docs.length === 0) throw new Error('the seed holds no .html documents');
  const docs = seeded.docs.slice(0, Math.max(options.docs, 1));
  note(`seed: ${seeded.nodes.length} nodes, ${seeded.docs.length} documents; tabs and the server edit ${docs.length}`);

  const world = await startWorld({ out, built, nodes: seeded.nodes });
  const { d, fake, server, folder, idOf } = world;
  const { url, host, port } = await world.served();
  note(`synced and served at ${url}, folder ${folder}`);

  const ledger = [];
  const counts = {
    acked: 0, refused: 0, errors: 0, serverEdits: 0, serverChurn: 0, localChurn: 0, churnErrors: 0,
    serviceKills: 0, hostKills: 0, pauses: 0, chaosErrors: 0, statuses: {},
  };
  const deadline = Date.now() + options.minutes * 60000;
  let running = true;
  const live = () => running && Date.now() < deadline;

  const tab = async (index) => {
    let n = 0;
    while (live()) {
      const doc = docs[index % docs.length];
      const marker = `L:${doc}:t${index}:n${n}:${hex()}`;
      n += 1;
      const answer = await save({ port, host, doc, body: page(doc, marker) });
      if (answer.status === 200) {
        ledger.push({ side: 'local', doc, marker });
        counts.acked += 1;
      } else if (answer.status) {
        counts.refused += 1;
        counts.statuses[answer.status] = (counts.statuses[answer.status] ?? 0) + 1;
      } else {
        counts.errors += 1;
      }
      await wait(200 + Math.floor(Math.random() * 400));
    }
  };

  const serverEditor = async () => {
    let n = 0;
    while (live()) {
      await wait(1000 + Math.floor(Math.random() * 2000));
      const doc = docs[n % docs.length];
      const id = idOf(doc);
      if (id === null) continue;
      const marker = `S:${doc}:n${n}:${hex()}`;
      n += 1;
      const current = (await server.getNodeContent(null, id)).content;
      await server.putNodeContent(null, id, withMarker(Buffer.isBuffer(current) ? current.toString('utf8') : String(current), marker));
      ledger.push({ side: 'server', doc, marker });
      counts.serverEdits += 1;
      fake.push({ type: 'node-saved', nodeId: id, nodeType: 'site' });
    }
  };

  // Files of each side's own that are made, renamed and removed; they carry no
  // ledger marker (a person removing a file means it), only the convergence check.
  const localChurn = async () => {
    let n = 0;
    while (live()) {
      await wait(4000 + Math.floor(Math.random() * 3000));
      try {
        writeFileSync(path.join(folder, `cl-${n}.html`), page(`cl-${n}.html`, `churn-${hex()}`).replace('data-soak', 'data-churn'));
        if (n >= 1 && existsSync(path.join(folder, `cl-${n - 1}.html`))) renameSync(path.join(folder, `cl-${n - 1}.html`), path.join(folder, `cl-${n - 1}-r.html`));
        if (n >= 2 && existsSync(path.join(folder, `cl-${n - 2}-r.html`))) unlinkSync(path.join(folder, `cl-${n - 2}-r.html`));
        counts.localChurn += 1;
      } catch (error) {
        counts.churnErrors += 1;
        note(`local churn ${n}: ${error.message}`);
      }
      n += 1;
    }
  };

  const serverChurn = async () => {
    let n = 0;
    while (live()) {
      await wait(5000 + Math.floor(Math.random() * 4000));
      try {
        const made = await server.createNode(null, { type: 'site', name: `cs-${n}.html`, parentId: 0, content: page(`cs-${n}.html`, `churn-${hex()}`).replace('data-soak', 'data-churn') });
        fake.push({ type: 'node-saved', nodeId: made.id, nodeType: 'site' });
        const previous = n >= 1 ? idOf(`cs-${n - 1}.html`) : null;
        if (previous !== null) {
          await server.renameNode(null, previous, `cs-${n - 1}-r.html`);
          fake.push({ type: 'node-renamed', nodeId: previous });
        }
        const older = n >= 2 ? idOf(`cs-${n - 2}-r.html`) : null;
        if (older !== null) {
          await server.deleteNode(null, older);
          fake.push({ type: 'node-deleted', nodeId: older });
        }
        counts.serverChurn += 1;
      } catch (error) {
        counts.churnErrors += 1;
        note(`server churn ${n}: ${error.message}`);
      }
      n += 1;
    }
  };

  const chaos = async () => {
    const every = (seconds) => (seconds > 0 ? Date.now() + seconds * 1000 : Infinity);
    let nextService = every(options.killServiceEvery);
    let nextHost = every(options.killHostEvery);
    let nextPause = every(options.pauseEvery);
    while (live()) {
      await wait(500);
      try {
        if (Date.now() >= nextHost) {
          nextHost = every(options.killHostEvery);
          const answer = await d.send('restartHost', { timeoutMs: 60000 });
          counts.hostKills += 1;
          note(`killed the host ${answer.before}, up again ${answer.after} (${counts.hostKills})`);
        } else if (Date.now() >= nextService) {
          nextService = every(options.killServiceEvery);
          const answer = await d.send('restartService', { timeoutMs: 60000 });
          counts.serviceKills += 1;
          note(`killed the service ${answer.before}, up again ${answer.after} (${counts.serviceKills})`);
        } else if (Date.now() >= nextPause) {
          nextPause = every(options.pauseEvery);
          await d.send('pause', { paused: true });
          await wait(5000 + Math.floor(Math.random() * 5000));
          await d.send('pause', { paused: false });
          counts.pauses += 1;
          note(`paused and resumed (${counts.pauses})`);
        }
      } catch (error) {
        counts.chaosErrors += 1;
        note(`chaos: ${error.message}`);
      }
    }
  };

  const ticker = setInterval(() => note(`acked ${counts.acked}, server edits ${counts.serverEdits}, refused ${counts.refused}, errors ${counts.errors}`), 60000);
  await Promise.all([chaos(), serverEditor(), localChurn(), serverChurn(), ...Array.from({ length: options.tabs }, (_, index) => tab(index))]);
  running = false;
  clearInterval(ticker);

  const { conflicts, differ } = await converge(world);
  await world.close();

  if (options.breakDownload) {
    const elsewhere = presentMarkers(folder, server, { skipVersions: true });
    const recorded = new Set(ledger.map((entry) => entry.marker));
    const victim = walk(path.join(folder, '.hyperclay', 'versions'))
      .find((file) => [...markersOf(readFileSync(file, 'utf8'))].some((marker) => recorded.has(marker) && !elsewhere.has(marker)));
    if (!victim) throw new Error('--break-download found no kept version holding a marker that is nowhere else');
    unlinkSync(victim);
    note(`deleted ${path.relative(folder, victim)} behind the host's back`);
  }

  const present = presentMarkers(folder, server);
  const lost = ledger.filter((entry) => !present.has(entry.marker));
  writeFileSync(path.join(out, 'ledger.json'), `${JSON.stringify({ counts, markers: ledger.length, lost, differ, conflicts }, null, 2)}\n`);
  note(`saves acked ${counts.acked}, server edits ${counts.serverEdits}, refused ${counts.refused} ${JSON.stringify(counts.statuses)}, errors ${counts.errors}, churn ${counts.localChurn}/${counts.serverChurn} (errors ${counts.churnErrors}), service kills ${counts.serviceKills}, host kills ${counts.hostKills}, pauses ${counts.pauses}, chaos errors ${counts.chaosErrors}`);

  if (ledger.length > 0 && lost.length === 0 && differ.length === 0) {
    rmSync(world.base, { recursive: true, force: true });
    console.log(`SYNC LEDGER OK markers=${ledger.length} lost=0 conflicts=${conflicts.length}`);
    return 0;
  }
  console.log(`SYNC LEDGER FAILED markers=${ledger.length} lost=${lost.length} differ=${differ.length} conflicts=${conflicts.length}`
    + `${lost.length ? `; first lost: ${lost.slice(0, 5).map((entry) => entry.marker).join(', ')}` : ''}`
    + `${differ.length ? `; first differ: ${JSON.stringify(differ.slice(0, 3))}` : ''}; folder kept at ${folder}`);
  return 1;
}

main().then((code) => process.exit(code), (error) => {
  console.error(`soak-sync: ${error.stack || error.message}`);
  process.exit(2);
});
