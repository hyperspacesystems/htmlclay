// The ledger soak (end gate item 5): the real app serves one folder with the
// live-room service on, several tabs save into a few documents for a while, the
// service and the host are killed on a schedule, and at the end every save the
// host acknowledged must still be on disk, as the stored document or one of its
// kept versions. Each save carries a marker of its own, so the check is a search.
//
//   node scripts/soak-served.mjs --minutes 60 --tabs 8 --kill-service-every 300 --kill-host-every 900
//
// Prints `LEDGER OK saves=<n> lost=0` and exits 0, or `LEDGER FAILED ...` and
// exits 1. --break-version deletes one kept version that holds an acknowledged
// save before the check, which must print LEDGER FAILED (the proof the check can
// fail). Linux and macOS: the service is found as the host's child with pgrep.
import { execFileSync, spawn } from 'node:child_process';
import { randomBytes } from 'node:crypto';
import {
  appendFileSync, mkdirSync, mkdtempSync, readdirSync, readFileSync, realpathSync, rmSync, unlinkSync, writeFileSync,
} from 'node:fs';
import http from 'node:http';
import os from 'node:os';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { freePort } from './served-product.mjs';

const repo = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');

function parseArgs(argv) {
  const options = {
    minutes: 60, tabs: 8, docs: 4, killServiceEvery: 300, killHostEvery: 900, bin: null, out: null, breakVersion: false,
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
      case '--bin': options.bin = path.resolve(value()); break;
      case '--out': options.out = path.resolve(value()); break;
      case '--break-version': options.breakVersion = true; break;
      default: throw new Error(`unknown argument ${flag}`);
    }
  }
  for (const key of ['minutes', 'tabs', 'docs', 'killServiceEvery', 'killHostEvery']) {
    if (!Number.isFinite(options[key]) || options[key] < 0) throw new Error(`--${key} must be a number`);
  }
  if (options.tabs < 1 || options.docs < 1) throw new Error('--tabs and --docs must be at least 1');
  return options;
}

const wait = (ms) => new Promise((resolve) => setTimeout(resolve, ms));
const stamp = () => new Date().toISOString();

const page = (doc, marker) => `<!doctype html>
<html><head><title>${doc}</title></head><body><h1>${doc}</h1><p data-soak="${marker}">${marker}</p></body></html>
`;

function build(out) {
  const bin = path.join(out, 'htmlclay');
  // The box builds without cgo; macOS needs it for the tray.
  const env = process.platform === 'linux' ? { ...process.env, CGO_ENABLED: '0' } : process.env;
  execFileSync('go', ['build', '-o', bin, './cmd/htmlclay'], { cwd: repo, stdio: 'inherit', env });
  return bin;
}

// One host process on a config base kept across restarts, answered once its
// folder is served.
async function startHost({ bin, configBase, folder, name, port, log }) {
  const child = spawn(bin, ['--no-tray'], {
    env: {
      ...process.env,
      HTMLCLAY_CONFIG_BASE: configBase,
      HTMLCLAY_SERVICE_CMD: `node ${path.join(repo, 'service', 'main.cjs')}`,
    },
    stdio: ['ignore', 'pipe', 'pipe'],
  });
  const exited = new Promise((resolve) => child.once('exit', (code, signal) => resolve({ code, signal })));
  const ready = `Serving folder ${folder} at http://${name}.htmlclay.localhost:${port}`;
  let scan = '';
  await new Promise((resolve, reject) => {
    const timer = setTimeout(() => reject(new Error(`the host did not serve ${folder} within 30s`)), 30000);
    const take = (chunk) => {
      const text = chunk.toString('utf8');
      appendFileSync(log, text);
      scan = (scan + text).slice(-8192);
      if (scan.includes(ready)) {
        clearTimeout(timer);
        resolve();
      }
    };
    child.stdout.on('data', take);
    child.stderr.on('data', take);
    exited.then(({ code, signal }) => {
      clearTimeout(timer);
      reject(new Error(`the host exited before serving (${signal ?? `exit ${code}`})`));
    });
  });
  return { child, exited };
}

function save({ port, host, doc, body }) {
  return new Promise((resolve) => {
    const request = http.request({
      host: '127.0.0.1',
      port,
      method: 'POST',
      path: '/_/save',
      headers: {
        Host: host,
        Origin: `http://${host}`,
        'Sec-Fetch-Site': 'same-origin',
        'Content-Type': 'text/html',
        'Document-URL': `http://${host}/${doc}`,
        'Content-Length': Buffer.byteLength(body),
      },
      timeout: 15000,
    }, (response) => {
      response.resume();
      response.on('end', () => resolve({ status: response.statusCode }));
    });
    request.on('timeout', () => request.destroy(new Error('timeout')));
    request.on('error', (error) => resolve({ error: error.code || error.message }));
    request.end(body);
  });
}

function serviceChildren(hostPid) {
  try {
    return execFileSync('pgrep', ['-P', String(hostPid)], { encoding: 'utf8' }).split('\n').filter(Boolean).map(Number);
  } catch {
    return [];
  }
}

// Every file under dir, for the version search.
function walk(dir, files = []) {
  let entries;
  try {
    entries = readdirSync(dir, { withFileTypes: true });
  } catch {
    return files;
  }
  for (const entry of entries) {
    const full = path.join(dir, entry.name);
    if (entry.isDirectory()) walk(full, files);
    else if (entry.isFile()) files.push(full);
  }
  return files;
}

const markerPattern = /data-soak="([^"]+)"/g;

function markersIn(file) {
  const found = new Set();
  for (const match of readFileSync(file, 'utf8').matchAll(markerPattern)) found.add(match[1]);
  return found;
}

async function main() {
  const options = parseArgs(process.argv.slice(2));
  const out = options.out ?? mkdtempSync(path.join(os.tmpdir(), 'soak-served-'));
  mkdirSync(out, { recursive: true });
  const log = path.join(out, 'host.log');
  writeFileSync(log, '');
  const bin = options.bin ?? build(out);

  // The served folder must sit inside home and not be home itself.
  const home = realpathSync(os.homedir());
  const folder = realpathSync(mkdtempSync(path.join(home, 'soak-served-')));
  const configBase = mkdtempSync(path.join(os.tmpdir(), 'soak-served-config-'));
  const name = 'soak';
  const port = await freePort();
  const host = `${name}.htmlclay.localhost:${port}`;
  const docs = Array.from({ length: options.docs }, (_, index) => `d${index}.html`);
  for (const doc of docs) writeFileSync(path.join(folder, doc), page(doc, `seed-${doc}`));
  mkdirSync(path.join(configBase, 'htmlclay'), { recursive: true });
  writeFileSync(path.join(configBase, 'htmlclay', 'config.json'), `${JSON.stringify({
    startOnLogin: false,
    port,
    updates: 'off',
    servedFolders: [{ id: randomBytes(8).toString('hex'), name, path: folder, identity: '', serving: true }],
  }, null, 2)}\n`);

  const ledger = [];
  const counts = { acked: 0, refused: 0, errors: 0, serviceKills: 0, hostKills: 0, statuses: {} };
  const note = (line) => {
    const text = `${stamp()} ${line}`;
    console.log(text);
    appendFileSync(path.join(out, 'soak.log'), `${text}\n`);
  };

  let current = await startHost({ bin, configBase, folder, name, port, log });
  note(`host up pid ${current.child.pid}, folder ${folder}, port ${port}`);
  const deadline = Date.now() + options.minutes * 60000;
  let running = true;

  const tab = async (index) => {
    let n = 0;
    while (running && Date.now() < deadline) {
      const doc = docs[index % docs.length];
      const marker = `${doc}:t${index}:n${n}:${randomBytes(4).toString('hex')}`;
      n += 1;
      const answer = await save({ port, host, doc, body: page(doc, marker) });
      if (answer.status === 200) {
        ledger.push({ doc, marker, at: Date.now() });
        counts.acked += 1;
      } else if (answer.status) {
        counts.refused += 1;
        counts.statuses[answer.status] = (counts.statuses[answer.status] ?? 0) + 1;
      } else {
        counts.errors += 1;
      }
      await wait(50 + Math.floor(Math.random() * 150));
    }
  };

  const chaos = async () => {
    let nextService = options.killServiceEvery > 0 ? Date.now() + options.killServiceEvery * 1000 : Infinity;
    let nextHost = options.killHostEvery > 0 ? Date.now() + options.killHostEvery * 1000 : Infinity;
    while (running && Date.now() < deadline) {
      await wait(500);
      if (Date.now() >= nextHost) {
        nextHost = Date.now() + options.killHostEvery * 1000;
        current.child.kill('SIGKILL');
        await current.exited;
        counts.hostKills += 1;
        note(`killed the host (${counts.hostKills})`);
        current = await startHost({ bin, configBase, folder, name, port, log });
        note(`host up again pid ${current.child.pid}`);
      } else if (Date.now() >= nextService) {
        nextService = Date.now() + options.killServiceEvery * 1000;
        const children = serviceChildren(current.child.pid);
        for (const pid of children) {
          try {
            process.kill(pid, 'SIGKILL');
          } catch {
            // Gone already.
          }
        }
        counts.serviceKills += 1;
        note(`killed the service ${children.join(',') || '(none running)'} (${counts.serviceKills})`);
      }
    }
  };

  const progress = setInterval(() => note(`acked ${counts.acked}, refused ${counts.refused}, errors ${counts.errors}`), 60000);
  await Promise.all([chaos(), ...Array.from({ length: options.tabs }, (_, index) => tab(index))]);
  running = false;
  clearInterval(progress);

  // A clean quit, so the check reads what the host itself left.
  current.child.kill('SIGTERM');
  const gone = await Promise.race([current.exited, wait(20000).then(() => null)]);
  if (!gone) {
    current.child.kill('SIGKILL');
    await current.exited;
    note('the host did not quit within 20s of SIGTERM; killed');
  }

  const versionsDir = path.join(folder, '.hyperclay', 'versions');
  const versionFiles = walk(versionsDir);
  if (options.breakVersion) {
    const kept = new Set(docs.flatMap((doc) => [...markersIn(path.join(folder, doc))]));
    const victim = versionFiles.find((file) => [...markersIn(file)].some((marker) => !kept.has(marker)
      && ledger.some((entry) => entry.marker === marker)));
    if (!victim) throw new Error('--break-version found no kept version holding an acknowledged save');
    unlinkSync(victim);
    note(`deleted ${path.relative(folder, victim)} behind the host's back`);
  }
  const present = new Set();
  for (const file of [...docs.map((doc) => path.join(folder, doc)), ...walk(versionsDir)]) {
    for (const marker of markersIn(file)) present.add(marker);
  }
  const lost = ledger.filter((entry) => !present.has(entry.marker));
  writeFileSync(path.join(out, 'ledger.json'), `${JSON.stringify({ counts, lost, saves: ledger.length }, null, 2)}\n`);
  note(`saves acked ${counts.acked}, refused ${counts.refused} ${JSON.stringify(counts.statuses)}, errors ${counts.errors}, service kills ${counts.serviceKills}, host kills ${counts.hostKills}, versions ${versionFiles.length}`);

  rmSync(configBase, { recursive: true, force: true });
  if (ledger.length > 0 && lost.length === 0) {
    rmSync(folder, { recursive: true, force: true });
    console.log(`LEDGER OK saves=${ledger.length} lost=0`);
    return 0;
  }
  console.log(`LEDGER FAILED saves=${ledger.length} lost=${lost.length}${lost.length ? `; first: ${lost.slice(0, 5).map((entry) => entry.marker).join(', ')}` : ''}; folder kept at ${folder}`);
  return 1;
}

main().then((code) => process.exit(code), (error) => {
  console.error(`soak-served: ${error.stack || error.message}`);
  process.exit(2);
});
