// The ledger: three tabs, one per open document, save together on a one-second
// tick through one rehearsal case, and afterwards every save the app
// acknowledged must still be in the document or its version history. Each tab keeps its address and its
// save token from before the update, so what it records is what a person with
// these documents open in a browser would have seen.
//
// Usage: node scripts/upgrade-ledger.mjs --port <n> --config <dir> --out <dir>
//   --mode first|next|rollback|quit --stop-file <path> [--times <dir>] [--home <dir>]
//   [--break-version] <file> <file> <file>
//
// --home is the app's home folder. It is needed where the config folder does
// not sit at <home>/Library/Application Support/htmlclay (Linux, Windows).
//
// Exit codes: 0 every save survived and every tab behaved as its mode expects,
// 1 one of the checks failed, 2 usage. The last stdout line is the verdict and
// the per-tab lines and the JSON stay in --out.
import fs from 'node:fs';
import http from 'node:http';
import path from 'node:path';

const USAGE = 'usage: upgrade-ledger.mjs --port <n> --config <dir> --out <dir> --mode first|next|rollback|quit --stop-file <path> [--times <dir>] [--home <dir>] [--break-version] <file> <file> <file>';

function usage(message) {
  process.stderr.write(`[ledger] ${message}\n${USAGE}\n`);
  process.exit(2);
}

function fail(message) {
  process.stdout.write(`LEDGER FAILED ${message}\n`);
  process.exit(1);
}

function delay(ms) {
  return new Promise((resolve) => setTimeout(resolve, ms));
}

function readText(file) {
  try {
    return fs.readFileSync(file, 'utf8');
  } catch {
    return null;
  }
}

function realpathOrNull(file) {
  try {
    return fs.realpathSync(file);
  } catch {
    return null;
  }
}

// walk lists the files under dir, recursively, in a stable order.
function walk(dir, out = []) {
  let entries;
  try {
    entries = fs.readdirSync(dir, { withFileTypes: true });
  } catch {
    return out;
  }
  entries.sort((a, b) => (a.name < b.name ? -1 : a.name > b.name ? 1 : 0));
  for (const entry of entries) {
    const file = path.join(dir, entry.name);
    if (entry.isDirectory()) walk(file, out);
    else if (entry.isFile()) out.push(file);
  }
  return out;
}

const MARKER_RE = /<!-- ledger (\d+) (\d+) -->/g;

function markersIn(text) {
  const markers = new Set();
  MARKER_RE.lastIndex = 0;
  let match;
  while ((match = MARKER_RE.exec(text)) !== null) markers.add(match[0]);
  return markers;
}

function markerFor(tab, n) {
  return `<!-- ledger ${tab} ${n} -->`;
}

// withMarker puts the marker right before the body's end, which is where a
// person's own note would sit.
function withMarker(original, marker) {
  const at = original.lastIndexOf('</body>');
  if (at < 0) return original + marker;
  return original.slice(0, at) + marker + original.slice(at);
}

const opts = { port: 0, config: '', out: '', mode: '', stopFile: '', times: '', home: '', breakVersion: false, files: [] };

for (let i = 0; i < process.argv.length - 2; i += 1) {
  const arg = process.argv[i + 2];
  if (arg === '--port') {
    opts.port = Number(process.argv[i + 3]);
    i += 1;
  } else if (arg === '--config') {
    opts.config = process.argv[i + 3];
    i += 1;
  } else if (arg === '--out') {
    opts.out = process.argv[i + 3];
    i += 1;
  } else if (arg === '--mode') {
    opts.mode = process.argv[i + 3];
    i += 1;
  } else if (arg === '--stop-file') {
    opts.stopFile = process.argv[i + 3];
    i += 1;
  } else if (arg === '--times') {
    opts.times = process.argv[i + 3];
    i += 1;
  } else if (arg === '--home') {
    opts.home = process.argv[i + 3];
    i += 1;
  } else if (arg === '--break-version') {
    opts.breakVersion = true;
  } else if (arg.startsWith('--')) {
    usage(`unknown argument ${arg}`);
  } else {
    opts.files.push(arg);
  }
}

if (!opts.port || !opts.config || !opts.out || !opts.stopFile) usage('--port, --config, --out and --stop-file are all required');
if (!['first', 'next', 'rollback', 'quit'].includes(opts.mode)) usage(`--mode must be first, next, rollback or quit, not '${opts.mode}'`);
if (opts.files.length === 0) usage('at least one document is required');

// One HTTP round trip to the app, the same shape scripts/sync-harness.mjs's
// save() uses: Node's fetch cannot set Host, and every site answers by Host.
function request(options, body, timeout) {
  return new Promise((resolve) => {
    const req = http.request({
      host: '127.0.0.1',
      port: options.port,
      method: options.method,
      path: options.path,
      headers: options.headers,
      timeout,
    }, (response) => {
      let text = '';
      response.setEncoding('utf8');
      response.on('data', (chunk) => { text += chunk; });
      response.on('end', () => resolve({ status: response.statusCode, body: text }));
      response.on('error', (error) => resolve({ error: error.code || error.message }));
    });
    req.on('timeout', () => req.destroy(new Error('timeout')));
    req.on('error', (error) => resolve({ error: error.code || error.message }));
    req.end(body);
  });
}

const configFile = path.join(opts.config, 'config.json');
const configText = readText(configFile);
if (configText === null) fail(`no config at ${configFile}`);
let config;
try {
  config = JSON.parse(configText);
} catch {
  fail(`no config at ${configFile}`);
}
const slugs = (config && config.siteSlugs) || {};
const labelByRealFolder = new Map();
for (const [folder, label] of Object.entries(slugs)) {
  const real = realpathOrNull(folder);
  if (real && typeof label === 'string') labelByRealFolder.set(real, label);
}

const tabs = opts.files.map((file, index) => {
  const real = realpathOrNull(path.dirname(file));
  const label = real ? labelByRealFolder.get(real) : undefined;
  if (!label) fail(`no site for ${file}`);
  return { index, file, label, host: `${label}.localhost:${opts.port}` };
});

// documentPaths lists where the app answers a document: by its name alone, and
// by the path it hands a browser, which is the document's path under the app's
// home: --home, or on macOS the folder the config sits in
// (<home>/Library/Application Support/htmlclay).
function documentPaths(file) {
  const candidates = [`/${path.basename(file)}`];
  const suffix = `${path.sep}Library${path.sep}Application Support${path.sep}htmlclay`;
  const home = opts.home || (opts.config.endsWith(suffix) ? opts.config.slice(0, opts.config.length - suffix.length) : '');
  if (home) {
    const rel = path.relative(home, file);
    if (rel && !rel.startsWith('..') && !path.isAbsolute(rel)) {
      const served = `/${rel.split(path.sep).join('/')}`;
      if (!candidates.includes(served)) candidates.push(served);
    }
  }
  return candidates;
}

async function documentToken(tab) {
  const paths = documentPaths(tab.file);
  for (let attempt = 1; attempt <= 10; attempt += 1) {
    for (const documentPath of paths) {
      const response = await request({
        port: opts.port,
        method: 'GET',
        path: documentPath,
        headers: { Host: tab.host, 'Sec-Fetch-Dest': 'document', 'Sec-Fetch-Site': 'none' },
      }, undefined, 5000);
      if (!response.body) continue;
      const match = /savetoken="([^"]+)"/.exec(response.body);
      if (match) return match[1];
    }
    if (attempt < 10) await delay(500);
  }
  return null;
}

for (const tab of tabs) {
  tab.original = readText(tab.file);
  if (tab.original === null) fail(`no document at ${tab.file}`);
  tab.token = await documentToken(tab);
  if (!tab.token) fail(`no save token for ${tab.file}`);
}

const records = [];
const inFlight = new Set();
const acksPerTab = new Map(tabs.map((tab) => [tab.index, 0]));
let ready = false;
let stopped = false;

function noteAck(tab) {
  acksPerTab.set(tab.index, (acksPerTab.get(tab.index) || 0) + 1);
  if (ready) return;
  for (const other of tabs) if ((acksPerTab.get(other.index) || 0) < 1) return;
  ready = true;
  try {
    fs.mkdirSync(opts.out, { recursive: true });
    fs.writeFileSync(path.join(opts.out, 'ledger-ready'), '');
  } catch {
    ready = false;
  }
}

// saveOnce sends one tab's marker and settles into { outcome, status, code }:
// ack for any 2xx, refused for any other answer (code from its JSON body when
// that parses), error for a connection error or a timeout.
async function saveOnce(tab, body) {
  const response = await request({
    port: opts.port,
    method: 'POST',
    path: `/_/save/${tab.token}`,
    headers: {
      Host: tab.host,
      Origin: `http://${tab.host}`,
      'Sec-Fetch-Site': 'same-origin',
      'Content-Type': 'text/html',
      'Content-Length': Buffer.byteLength(body),
    },
  }, body, 5000);
  if (response.error) return { outcome: 'error', status: null, code: response.error };
  const status = response.status;
  if (status >= 200 && status < 300) return { outcome: 'ack', status, code: null };
  let code = null;
  try {
    const parsed = JSON.parse(response.body);
    if (parsed && typeof parsed.code === 'string') code = parsed.code;
  } catch {
    code = null;
  }
  return { outcome: 'refused', status, code };
}

// Every tab sends one save on the same tick and does not wait for an answer
// before the next, so a slow or refused save cannot hold the other tabs up. The
// tabs leave most of each second quiet on purpose: the old app restarts for an
// update only after 500 ms with no write in flight.
const nextN = new Map(tabs.map((tab) => [tab.index, 0]));

function sendOnce(tab) {
  const n = (nextN.get(tab.index) || 0) + 1;
  nextN.set(tab.index, n);
  const marker = markerFor(tab.index, n);
  const record = { tab: tab.index, n, marker, t: Date.now(), outcome: null, status: null, code: null };
  records.push(record);
  const pending = saveOnce(tab, withMarker(tab.original, marker)).then((result) => {
    record.outcome = result.outcome;
    record.status = result.status;
    record.code = result.code;
    if (result.outcome === 'ack') noteAck(tab);
    inFlight.delete(pending);
  });
  inFlight.add(pending);
  return pending;
}

// runTabs ticks every 1000 ms from the start of the previous tick and waits for
// the whole round before the next one, so a slow round delays the next tick
// instead of overlapping it.
async function runTabs() {
  let tick = Date.now();
  while (!stopped) {
    await Promise.all(tabs.map((tab) => sendOnce(tab)));
    tick += 1000;
    const wait = tick - Date.now();
    if (wait > 0) await delay(wait);
  }
}

const ticker = runTabs();
while (!stopped) {
  if (fs.existsSync(opts.stopFile)) {
    stopped = true;
    break;
  }
  await delay(100);
}
await ticker;
while (inFlight.size > 0) await delay(20);

const ackedRecords = records.filter((record) => record.outcome === 'ack');

const docTexts = new Map(tabs.map((tab) => [tab.index, readText(tab.file) || '']));

if (opts.breakVersion) {
  const ackedMarkers = new Set(ackedRecords.map((record) => record.marker));
  const versionsFile = path.join(opts.config, 'versions');
  const texts = walk(versionsFile).map((file) => ({ file, text: readText(file) || '' }));
  const counts = new Map();
  for (const { text } of texts) {
    for (const marker of markersIn(text)) {
      if (ackedMarkers.has(marker)) counts.set(marker, (counts.get(marker) || 0) + 1);
    }
  }
  // A marker held by two version files would survive deleting either one, so a
  // marker in exactly one is preferred; any candidate is the fallback.
  let chosen = null;
  let fallback = null;
  for (const { file, text } of texts) {
    for (const marker of markersIn(text)) {
      if (!ackedMarkers.has(marker)) continue;
      const record = ackedRecords.find((candidate) => candidate.marker === marker);
      const doc = docTexts.get(record.tab) || '';
      if (doc.includes(marker)) continue;
      if (!fallback) fallback = { file, marker };
      if (counts.get(marker) === 1 && !chosen) chosen = { file, marker };
    }
    if (chosen) break;
  }
  const target = chosen || fallback;
  if (!target) fail('--break-version found nothing to delete');
  fs.unlinkSync(target.file);
  process.stderr.write(`[ledger] deleted ${target.file} to prove the check\n`);
}

const versionFiles = walk(path.join(opts.config, 'versions'));
const present = new Set();
for (const file of [...tabs.map((tab) => tab.file), ...versionFiles]) {
  const text = readText(file);
  if (text === null) continue;
  for (const marker of markersIn(text)) present.add(marker);
}

const firstSiteTab = tabs.find((tab) => tab.label === 'htmlclay') || null;

// The update's two moments: when the old bundle's CFBundleVersion became the
// new one (the rehearsal's watch_timing writes it) and when 2.0 first reported
// serving. A tab that saved on both sides of them spanned the update.
function oldUntilMs() {
  const text = opts.times ? readText(path.join(opts.times, 'version-changed.ms')) : null;
  const value = text === null ? Number.NaN : Number(text.trim());
  return Number.isInteger(value) && value > 0 ? value : null;
}

function newFromMs() {
  const text = readText(path.join(opts.config, 'updates', 'health.json'));
  if (text === null) return null;
  let health;
  try {
    health = JSON.parse(text);
  } catch {
    return null;
  }
  if (!health || typeof health.time !== 'string') return null;
  const ms = Date.parse(health.time.replace(/\.(\d{3})\d+/, '.$1'));
  return Number.isFinite(ms) ? ms : null;
}

// first and next both end with the new version kept, so every tab has saves on both sides.
const kept = opts.mode === 'first' || opts.mode === 'next';
const oldUntil = kept ? oldUntilMs() : null;
const newFrom = kept ? newFromMs() : null;

const verdicts = tabs.map((tab) => {
  const own = records.filter((record) => record.tab === tab.index).sort((a, b) => a.n - b.n);
  const acks = own.filter((record) => record.outcome === 'ack');
  const bad = own.filter((record) => record.outcome !== 'ack');
  const refused = own.filter((record) => record.outcome === 'refused');
  const errors = own.filter((record) => record.outcome === 'error');
  const reasons = [];
  let oldCount = null;
  let newCount = null;

  const lost = acks.filter((record) => !present.has(record.marker));
  if (lost.length > 0) {
    reasons.push(`lost ${lost.length} of ${acks.length} acked saves, first ${lost[0].marker} at ${lost[0].t}`);
  }

  const doc = docTexts.get(tab.index) || '';
  if (acks.length === 0) reasons.push('no acked save');
  else if (!doc.includes(acks[acks.length - 1].marker) &&
    // A save sent after the last ack that timed out may still have been written:
    // its outcome is unknown, so the document may hold it instead.
    !own.some((record) => record.n > acks[acks.length - 1].n && record.outcome === 'error' && doc.includes(record.marker))) {
    reasons.push(`the document does not hold the last ack ${acks[acks.length - 1].marker} at ${acks[acks.length - 1].t}`);
  }

  if (opts.mode === 'quit') {
    if (acks.length === 0) reasons.push('no acked save at all');
  } else if (opts.mode === 'first' && tab === firstSiteTab) {
    if (bad.length > 0 && !acks.some((record) => record.n < bad[0].n)) {
      reasons.push(`no ack before the first bad record (n=${bad[0].n} ${bad[0].outcome})`);
    }
    const moved = own.find((record) => record.outcome === 'refused' && record.status === 409 && record.code === 'site-moved');
    if (!moved) reasons.push('no 409 site-moved refusal');
    else if (acks.some((record) => record.n > moved.n)) {
      const after = acks.find((record) => record.n > moved.n);
      reasons.push(`a save was acked after site-moved (n=${after.n})`);
    }
  } else if (kept) {
    if (oldUntil === null || newFrom === null) {
      reasons.push('no update times (version-changed.ms or health.json)');
    } else {
      oldCount = acks.filter((record) => record.t < oldUntil).length;
      newCount = acks.filter((record) => record.t > newFrom).length;
      if (oldCount === 0) reasons.push('no ack by the old version');
      if (newCount === 0) reasons.push('no ack by the new version');
    }
  } else if (bad.length === 0) {
    reasons.push('no bad record');
  } else {
    if (!acks.some((record) => record.n < bad[0].n)) {
      reasons.push(`no ack before the first bad record (n=${bad[0].n} ${bad[0].outcome})`);
    }
    const lastBad = bad[bad.length - 1];
    if (!acks.some((record) => record.n > lastBad.n)) {
      reasons.push(`no ack after the last bad record (n=${lastBad.n} ${lastBad.outcome})`);
    }
  }

  return { tab: tab.index, host: tab.host, file: tab.file, ok: reasons.length === 0, reasons, acks: acks.length, refused: refused.length, errors: errors.length, old: oldCount, new: newCount, records: own };
});

try {
  fs.mkdirSync(opts.out, { recursive: true });
  fs.writeFileSync(path.join(opts.out, 'ledger.json'), `${JSON.stringify({ mode: opts.mode, port: opts.port, tabs: verdicts }, null, 2)}\n`);
} catch {}

for (const verdict of verdicts) {
  const shown = verdict.old === null && verdict.new === null ? '' : ` old=${verdict.old} new=${verdict.new}`;
  process.stderr.write(`[ledger] tab ${verdict.tab} ${verdict.host} acks=${verdict.acks} refused=${verdict.refused} errors=${verdict.errors}${shown} ${verdict.ok ? 'ok' : `FAILED ${verdict.reasons.join('; ')}`}\n`);
}

const failed = verdicts.filter((verdict) => !verdict.ok);
if (failed.length === 0) {
  process.stdout.write(`LEDGER OK ${ackedRecords.length} saves, 0 lost\n`);
  process.exit(0);
}
process.stdout.write(`LEDGER FAILED ${failed.map((verdict) => `tab ${verdict.tab}: ${verdict.reasons.join('; ')}`).join('; ')}\n`);
process.exit(1);
