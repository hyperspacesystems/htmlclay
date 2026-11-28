// End-to-end proof that a document served by the real HTML Clay host keeps working
// offline. One folder is served by a real `cmd/htmlclay` binary; a stand-in CDN on
// 127.0.0.1 plays the internet and can be stopped, made to hang, or switched to new
// bytes. Each check prints `PASS <name>` or `FAIL <name>: <detail>`, and the run exits
// 0 only when every check passes. With TESTBOX_OUT set, the summary is also written
// to $TESTBOX_OUT/offline-mirror-e2e-<browser>.json.
//
// node scripts/offline-mirror-e2e.mjs [chromium|firefox|webkit]   (default chromium)
import { execFileSync, spawn } from 'node:child_process';
import crypto from 'node:crypto';
import fs from 'node:fs';
import http from 'node:http';
import { createRequire } from 'node:module';
import net from 'node:net';
import os from 'node:os';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const require = createRequire(import.meta.url);
const playwright = require('playwright');
const BROWSER = process.argv[2] || 'chromium';
if (!['chromium', 'firefox', 'webkit'].includes(BROWSER)) throw new Error(`unknown browser ${BROWSER}`);
const LAUNCH = {
  chromium: { args: ['--mute-audio'] },
  firefox: { firefoxUserPrefs: { 'media.volume_scale': '0.0' } },
  webkit: {},
}[BROWSER];

const repo = path.dirname(path.dirname(fileURLToPath(import.meta.url)));
const wait = (ms) => new Promise((resolve) => setTimeout(resolve, ms));

const CHECKS = [
  'one online open then offline loads every GET kind',
  'network back gives the new bytes',
  'POST is never replayed',
  'the boot script never stays in the page',
  'a save writes only the edit',
  'the document keeps no-store while the worker controls the page',
  'a fresh profile offline recovers with one reload, and only one',
  'a page with its own scope-root worker keeps it',
  '100 cross-origin files cost little',
];

const checks = [];
function record(name, ok, detail = '') {
  checks.push({ name, ok, detail });
  console.log(`${ok ? 'PASS' : 'FAIL'} ${name}${ok ? '' : `: ${detail}`}`);
}

function expect(ok, message) {
  if (!ok) throw new Error(message);
}

// ---------------------------------------------------------------- the stand-in CDN

const FONT = fs.readFileSync(path.join(repo, 'service', 'local', 'assets', 'fonts', 'fixedsys-webfont.woff2'));
const PNG = Buffer.from('iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg==', 'base64');
const SRI_BODY = "window.__r.sri = 'ok';\n";
const SRI_HASH = `sha256-${crypto.createHash('sha256').update(SRI_BODY).digest('base64')}`;

function cdnFiles(v) {
  return {
    '/lib/classic.js': ['text/javascript', `window.__r.classic = '${v}';\n`],
    '/lib/sri.js': ['text/javascript', SRI_BODY],
    '/css/style.css': ['text/css', `@import url("/css/root.css");\n@font-face { font-family: SpikeFont; src: url("../fonts/spike.woff2") format("woff2"); }\nbody { --style: ${v}; }\n`],
    '/css/root.css': ['text/css', `body { --root: ${v}; }\n`],
    '/fonts/spike.woff2': ['font/woff2', FONT],
    '/esm/main.js': ['text/javascript', `import { dep } from './dep.js';\nimport { root } from '/esm/rootdep.js';\nwindow.__r.module = dep + '+' + root;\n`],
    '/esm/dep.js': ['text/javascript', `export const dep = 'dep-${v}';\n`],
    '/esm/rootdep.js': ['text/javascript', `export const root = 'root-${v}';\n`],
    '/esm/lazy.js': ['text/javascript', `import { more } from './lazydep.js';\nexport const lazy = 'lazy-${v}+' + more;\n`],
    '/esm/lazydep.js': ['text/javascript', `export const more = 'more-${v}';\n`],
    '/data.json': ['application/json', JSON.stringify({ v })],
    '/img/p.png': ['image/png', PNG],
  };
}

// state.mode: 'up' answers, 'hang' accepts and never answers (a half-working
// network). state.version is the body of every versioned file, so switching it to
// 'v2' proves a copy is fetched network first.
function startCdn(port, state) {
  const log = [];
  const server = http.createServer((req, res) => {
    const url = new URL(req.url, 'http://x');
    log.push(`${req.method} ${url.pathname}`);
    if (state.mode === 'hang') return;
    const cors = { 'Access-Control-Allow-Origin': '*', 'Cache-Control': 'no-cache' };
    if (req.method === 'OPTIONS') {
      res.writeHead(204, { ...cors, 'Access-Control-Allow-Methods': 'GET,POST', 'Access-Control-Allow-Headers': '*' });
      return res.end();
    }
    if (req.method === 'POST' && url.pathname === '/echo') {
      res.writeHead(200, { ...cors, 'Content-Type': 'text/plain' });
      return res.end(`posted-${state.version}`);
    }
    const many = url.pathname.match(/^\/many\/(\d+)\.js$/);
    if (many) {
      res.writeHead(200, { ...cors, 'Content-Type': 'text/javascript' });
      return res.end('window.__n=(window.__n||0)+1;\n');
    }
    const file = cdnFiles(state.version)[url.pathname];
    if (!file) {
      res.writeHead(404, cors);
      return res.end('nf');
    }
    const etag = `"${crypto.createHash('sha256').update(file[1]).digest('hex').slice(0, 16)}"`;
    res.writeHead(200, { ...cors, 'Content-Type': file[0], ETag: etag, 'Access-Control-Expose-Headers': 'ETag' });
    return res.end(file[1]);
  });
  const sockets = new Set();
  server.on('connection', (socket) => {
    sockets.add(socket);
    socket.on('close', () => sockets.delete(socket));
  });
  return new Promise((resolve) => server.listen(port, '127.0.0.1', () => resolve({
    log,
    stop: () => new Promise((done) => {
      for (const socket of sockets) socket.destroy();
      server.close(() => done());
    }),
  })));
}

// The test page: every kind of cross-origin GET a page makes, plus a POST. Each
// answer lands in window.__r, and window.__finished marks the end of the run.
const DOC = (cdn) => `<!DOCTYPE html>
<html lang="en"><head>
<meta charset="utf-8">
<title>offline mirror</title>
<script>window.__r = {}; window.__t0 = performance.now();</script>
<link rel="stylesheet" href="${cdn}/css/style.css">
<script src="${cdn}/lib/classic.js"></script>
<script src="${cdn}/lib/sri.js" integrity="${SRI_HASH}" crossorigin="anonymous"></script>
<script type="module" src="${cdn}/esm/main.js"></script>
</head>
<body>
<p style="font-family: SpikeFont">ABCD</p>
<img id="img" src="${cdn}/img/p.png" alt="">
<script type="module">
  const r = window.__r;
  r.controlledAtStart = !!(navigator.serviceWorker && navigator.serviceWorker.controller);
  const jobs = [];
  jobs.push(import('${cdn}/esm/lazy.js').then((m) => { r.lazy = m.lazy; }, () => { r.lazy = 'ERR'; }));
  jobs.push(fetch('${cdn}/data.json').then((x) => x.json()).then((j) => { r.fetch = j.v; }, () => { r.fetch = 'ERR'; }));
  jobs.push(fetch('${cdn}/echo', { method: 'POST', body: 'x', signal: AbortSignal.timeout(4000) }).then((x) => x.text()).then((t) => { r.post = t; }, () => { r.post = 'ERR'; }));
  jobs.push(document.fonts.load('16px SpikeFont', 'ABCD').then((f) => { r.font = f.length ? 'ok' : 'none'; }, () => { r.font = 'ERR'; }));
  const img = document.getElementById('img');
  jobs.push(new Promise((ok) => { if (img.complete) ok(); else { img.onload = ok; img.onerror = ok; } }).then(() => { r.img = img.naturalWidth ? 'ok' : 'ERR'; }));
  await Promise.allSettled(jobs);
  const cs = getComputedStyle(document.body);
  r.style = cs.getPropertyValue('--style').trim() || 'ERR';
  r.root = cs.getPropertyValue('--root').trim() || 'ERR';
  r.controlledAtEnd = !!(navigator.serviceWorker && navigator.serviceWorker.controller);
  r.bootInDom = !!document.querySelector('script[src="/_/offline/boot.js"]');
  r.ms = Math.round(performance.now() - window.__t0);
  window.__finished = true;
</script>
</body></html>
`;

// A page that loads 100 small cross-origin scripts, and reports when its load event fired.
const MANY = (cdn) => `<!DOCTYPE html>
<html lang="en"><head>
<meta charset="utf-8">
<title>many</title>
<script>
  window.__n = 0;
  window.__loadAt = null;
  addEventListener('load', function () { window.__loadAt = performance.now(); window.__finished = true; });
</script>
${Array.from({ length: 100 }, (_, i) => `<script src="${cdn}/many/${i}.js"></script>`).join('\n')}
</head>
<body><p>many</p></body></html>
`;

// A page that brings its own scope-root worker. The boot script must leave it alone.
const OWN_SW_PAGE = `<!DOCTYPE html>
<html lang="en"><head>
<meta charset="utf-8">
<title>own worker</title>
<script>addEventListener('load', function () { navigator.serviceWorker.register('/own-sw.js', { scope: '/' }).catch(function () {}); });</script>
</head>
<body><p>own worker</p>
<script>window.__finished = true;</script>
</body></html>
`;
const OWN_SW = "self.addEventListener('fetch', () => {});\n";

// ------------------------------------------------------------- the real HTML Clay host

const READY_TIMEOUT_MS = 30000;

// One port free on both loopback stacks, because the app holds 127.0.0.1 and ::1 on
// the same number and asks the person what to do when only one is free.
function bind(host, port) {
  const server = net.createServer();
  return new Promise((resolve, reject) => {
    server.once('error', reject);
    server.listen({ host, port, exclusive: true }, () => resolve(server));
  });
}

const closeServer = (server) => new Promise((resolve) => server.close(resolve));

async function freePort() {
  for (let attempt = 0; attempt < 20; attempt += 1) {
    const v4 = await bind('127.0.0.1', 0);
    const { port } = v4.address();
    let v6 = null;
    try {
      v6 = await bind('::1', port);
    } catch (error) {
      if (error?.code === 'EADDRINUSE') {
        await closeServer(v4);
        continue;
      }
    }
    await closeServer(v4);
    if (v6) await closeServer(v6);
    return port;
  }
  throw new Error('could not find a port free on both loopback stacks');
}

// The folder has to be one the app can serve: a real folder inside the home folder,
// and not the home folder itself.
function servedRoot(root) {
  const real = fs.realpathSync(root);
  if (!fs.statSync(real).isDirectory()) throw new Error(`${real} is not a folder`);
  const home = fs.realpathSync(os.homedir());
  const rel = path.relative(home, real);
  if (!rel || rel.startsWith('..') || path.isAbsolute(rel) || rel.split(path.sep).includes('..')) {
    throw new Error(`the folder ${real} must be a folder inside ${home}, not the home folder itself`);
  }
  return real;
}

// scripts/served-product.mjs startProduct, plus the two things this test needs: extra
// environment for the child (the stand-in CDN and a cache folder of its own) and the
// offline copies switch in the config it writes.
async function startProduct({ bin, root, name, port, extraEnv = {}, offlineCopiesOff = false }) {
  const folder = servedRoot(root);
  const configBase = fs.mkdtempSync(path.join(os.tmpdir(), 'htmlclay-offline-e2e-conf-'));
  const configDir = path.join(configBase, 'htmlclay');
  const logPath = path.join(configBase, 'product.log');
  fs.mkdirSync(configDir, { recursive: true });
  fs.writeFileSync(logPath, '');

  const chosen = port ?? await freePort();
  const config = {
    startOnLogin: false,
    port: chosen,
    updates: 'off',
    offlineCopiesOff,
    servedFolders: [{
      id: crypto.randomBytes(8).toString('hex'),
      name,
      path: folder,
      identity: '',
      serving: true,
    }],
  };
  fs.writeFileSync(path.join(configDir, 'config.json'), `${JSON.stringify(config, null, 2)}\n`);

  const child = spawn(bin, ['--no-tray'], {
    cwd: configBase,
    env: { ...process.env, HTMLCLAY_CONFIG_BASE: configBase, ...extraEnv },
    stdio: ['ignore', 'pipe', 'pipe'],
  });
  const exitPromise = new Promise((resolve) => child.once('exit', (code, signal) => resolve({ code, signal })));
  const escaped = folder.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
  const readyPattern = new RegExp(`Serving folder ${escaped} at http://${name}\\.htmlclay\\.localhost:${chosen}(?:\\s|$)`);
  const record = (chunk) => {
    const text = chunk.toString('utf8');
    try {
      fs.appendFileSync(logPath, text);
    } catch {
      // A log that cannot be written cannot change a verdict.
    }
    return text;
  };
  child.stdout.setEncoding('utf8');
  child.stderr.setEncoding('utf8');

  const tail = () => {
    try {
      return fs.readFileSync(logPath, 'utf8').trimEnd().split('\n').slice(-20).join('\n');
    } catch {
      return '';
    }
  };

  let stopPromise = null;
  const stop = () => {
    if (stopPromise) return stopPromise;
    stopPromise = (async () => {
      if (child.exitCode === null && child.signalCode === null) {
        try {
          child.kill('SIGTERM');
        } catch {
          // A signal to a child already gone is the same as one gone.
        }
        const gone = await Promise.race([exitPromise, wait(15000).then(() => null)]);
        if (!gone) {
          try {
            child.kill('SIGKILL');
          } catch {
            // Same.
          }
          await exitPromise;
        }
      } else {
        await exitPromise;
      }
      fs.rmSync(configBase, { recursive: true, force: true });
    })();
    return stopPromise;
  };

  const url = `http://${name}.htmlclay.localhost:${chosen}`;
  let scan = '';
  const ready = await new Promise((resolve, reject) => {
    const timer = setTimeout(() => reject(new Error(`the app served ${folder} nowhere within ${READY_TIMEOUT_MS}ms; last log lines:\n${tail()}`)), READY_TIMEOUT_MS);
    const finish = (error) => {
      clearTimeout(timer);
      if (error) reject(error);
      else resolve({ url, port: chosen, folder, logPath, stop, configBase });
    };
    child.on('error', (error) => finish(new Error(`the app could not start: ${error.message}`)));
    exitPromise.then(({ code, signal }) => finish(new Error(`the app exited before serving ${folder} (${signal ?? `exit ${code}`}); last log lines:\n${tail()}`)));
    const listen = (stream) => {
      stream.on('data', (chunk) => {
        scan += record(chunk);
        if (readyPattern.test(scan)) finish(null);
      });
    };
    listen(child.stdout);
    listen(child.stderr);
  }).catch(async (error) => {
    await stop();
    throw error;
  });
  return ready;
}

// POST /_/save of a whole document, the way scripts/soak-served.mjs saves one.
function saveDocument({ port, host, documentURL, body }) {
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
        'Document-URL': documentURL,
        'Content-Length': Buffer.byteLength(body),
      },
      timeout: 15000,
    }, (response) => {
      const chunks = [];
      response.on('data', (chunk) => chunks.push(chunk));
      response.on('end', () => resolve({ status: response.statusCode, body: Buffer.concat(chunks).toString('utf8').slice(0, 400) }));
    });
    request.on('timeout', () => request.destroy(new Error('timeout')));
    request.on('error', (error) => resolve({ error: error.code || error.message }));
    request.end(body);
  });
}

// ------------------------------------------------------------------------ the run

const state = { version: 'v1', mode: 'up' };

const V1 = {
  classic: 'v1', sri: 'ok', style: 'v1', root: 'v1', font: 'ok', img: 'ok',
  module: 'dep-v1+root-v1', lazy: 'lazy-v1+more-v1', fetch: 'v1', post: 'ERR',
};
const V2 = {
  classic: 'v2', sri: 'ok', style: 'v2', root: 'v2', font: 'ok', img: 'ok',
  module: 'dep-v2+root-v2', lazy: 'lazy-v2+more-v2', fetch: 'v2', post: 'ERR',
};
// The same page with the network up: the one POST is answered.
const V1_ONLINE = { ...V1, post: 'posted-v1' };
const V2_ONLINE = { ...V2, post: 'posted-v2' };
const V2_VERSIONED = {
  classic: 'v2', style: 'v2', root: 'v2', module: 'dep-v2+root-v2', lazy: 'lazy-v2+more-v2', fetch: 'v2',
};

const diff = (got, want) => Object.keys(want)
  .filter((key) => got[key] !== want[key])
  .map((key) => `${key}=${got[key] ?? '∅'} (want ${want[key]})`);

const median = (xs) => {
  const sorted = [...xs].sort((a, b) => a - b);
  const middle = Math.floor(sorted.length / 2);
  return sorted.length % 2 ? sorted[middle] : (sorted[middle - 1] + sorted[middle]) / 2;
};

const tempRoot = fs.mkdtempSync(path.join(os.tmpdir(), 'htmlclay-offline-e2e-'));
const homeBase = fs.mkdtempSync(path.join(os.homedir(), 'htmlclay-offline-e2e-'));
const folder = path.join(homeBase, 'main');
const folderOff = path.join(homeBase, 'off');
const storeCache = path.join(tempRoot, 'cache');
const offCache = path.join(tempRoot, 'cache-off');
for (const dir of [storeCache, offCache]) fs.mkdirSync(dir, { recursive: true });

let cdnOrigin = '';
const contexts = [];
const hosts = [];
const visits = [];
let cdn = null;

async function newContext(label) {
  const dir = path.join(tempRoot, `profile-${label}`);
  const context = await playwright[BROWSER].launchPersistentContext(dir, { headless: true, ...LAUNCH });
  contexts.push(context);
  return context;
}

async function firstPage(context) {
  return context.pages()[0] ?? await context.newPage();
}

// Everything a page can say about itself at the end of a load: the answers the test
// page recorded, whether the boot tag is still in the DOM, and whether a serialization
// of the document would carry it.
async function readPage(page) {
  const seen = await page.evaluate(() => ({
    values: window.__r || {},
    bootInDom: !!document.querySelector('script[src="/_/offline/boot.js"]'),
    text: `<!DOCTYPE html>\n${document.documentElement.outerHTML}`,
  }));
  return { values: seen.values, bootInDom: seen.bootInDom, hasTag: seen.text.includes('<script src="/_/offline/boot.js"') };
}

function remember(label, seen) {
  visits.push({ label, ...seen });
}

async function visit(page, url, { first = false, settleMs = 2000, timeoutMs = 15000 } = {}) {
  if (first) await page.goto(url, { waitUntil: 'domcontentloaded', timeout: timeoutMs });
  else await page.reload({ waitUntil: 'domcontentloaded', timeout: timeoutMs });
  await page.waitForFunction(() => window.__finished === true, null, { timeout: timeoutMs });
  await page.waitForTimeout(settleMs);
  const seen = await readPage(page);
  remember(new URL(url).pathname, seen);
  return seen;
}

async function measureLoad(page, url, label) {
  await page.reload({ waitUntil: 'domcontentloaded', timeout: 20000 });
  await page.waitForFunction(() => window.__loadAt !== null, null, { timeout: 20000 });
  const seen = await readPage(page);
  remember(label, seen);
  const at = await page.evaluate(() => window.__loadAt);
  const scripts = await page.evaluate(() => window.__n);
  return { at, scripts };
}

// Every index file of the copy store under root: <store>/origins/*.json.
function originIndexes(root) {
  const found = [];
  const walk = (dir) => {
    let entries;
    try {
      entries = fs.readdirSync(dir, { withFileTypes: true });
    } catch {
      return;
    }
    for (const entry of entries) {
      const full = path.join(dir, entry.name);
      if (entry.isDirectory()) walk(full);
      else if (entry.name.endsWith('.json') && path.basename(dir) === 'origins') found.push(full);
    }
  };
  walk(root);
  return found;
}

function writeFolder(dir) {
  fs.mkdirSync(dir, { recursive: true });
  fs.writeFileSync(path.join(dir, 'doc.html'), DOC(cdnOrigin));
  fs.writeFileSync(path.join(dir, 'many.html'), MANY(cdnOrigin));
  fs.writeFileSync(path.join(dir, 'own-sw.html'), OWN_SW_PAGE);
  fs.writeFileSync(path.join(dir, 'own-sw.js'), OWN_SW);
}

async function run() {
  const bin = path.join(tempRoot, 'htmlclay');
  execFileSync('go', ['build', '-o', bin, './cmd/htmlclay'], { cwd: repo, stdio: ['ignore', 'pipe', 'pipe'] });

  const cdnPort = await freePort();
  cdnOrigin = `http://127.0.0.1:${cdnPort}`;
  writeFolder(folder);
  writeFolder(folderOff);

  cdn = await startCdn(cdnPort, state);
  const host = await startProduct({
    bin,
    root: folder,
    name: 'conf',
    extraEnv: {
      HTMLCLAY_OFFLINE_TEST_UPSTREAM: `127.0.0.1:${cdnPort}`,
      XDG_CACHE_HOME: storeCache,
    },
  });
  hosts.push(host);
  const hostOff = await startProduct({
    bin,
    root: folderOff,
    name: 'off',
    offlineCopiesOff: true,
    extraEnv: { XDG_CACHE_HOME: offCache },
  });
  hosts.push(hostOff);

  const docURL = `${host.url}/doc.html`;

  // 1: one online open, then the network goes away.
  const context = await newContext('main');
  const page = await firstPage(context);
  const online = await visit(page, docURL, { first: true, settleMs: 4000 });
  await cdn.stop();
  cdn = null;
  const offline = await visit(page, docURL);
  const failures = [
    ...diff(online.values, V1_ONLINE).map((line) => `online open: ${line}`),
    ...diff(offline.values, V1).map((line) => `offline reload: ${line}`),
  ];
  record(CHECKS[0], failures.length === 0, failures.join('; '));

  // 2: the network comes back with new bytes.
  state.version = 'v2';
  cdn = await startCdn(cdnPort, state);
  const back = await visit(page, docURL);
  const changed = diff(back.values, V2_ONLINE);
  record(CHECKS[1], changed.length === 0, changed.join('; '));

  // 3: a POST is never replayed, and never stored.
  const stored = originIndexes(path.join(host.configBase, 'htmlclay-cache', 'offline-copies'));
  const echoing = stored.filter((file) => fs.readFileSync(file, 'utf8').includes('/echo'));
  const postFailures = [];
  if (offline.values.post !== 'ERR') postFailures.push(`the offline POST answered ${offline.values.post}`);
  if (!stored.length) postFailures.push(`the copy store holds no origin index under ${host.configBase}`);
  if (echoing.length) postFailures.push(`the store holds /echo in ${echoing.map((f) => path.basename(f)).join(', ')}`);
  record(CHECKS[2], postFailures.length === 0, postFailures.join('; '));

  // 5: a save of the page writes only the person's edit.
  const edited = await page.evaluate(() => {
    document.querySelector('p').textContent = 'edited';
    return `<!DOCTYPE html>\n${document.documentElement.outerHTML}`;
  });
  const saved = await saveDocument({
    port: host.port,
    host: host.url.replace('http://', ''),
    documentURL: docURL,
    body: edited,
  });
  const saveFailures = [];
  if (saved.error || !(saved.status >= 200 && saved.status < 300)) {
    saveFailures.push(`POST /_/save answered ${saved.status ?? saved.error}: ${saved.body ?? ''}`);
  } else {
    const onDisk = fs.readFileSync(path.join(folder, 'doc.html'), 'utf8');
    if (!onDisk.includes('edited')) saveFailures.push('the saved file does not hold the edit');
    if (onDisk.includes('<script src="/_/offline/boot.js"')) saveFailures.push('the saved file holds the boot script');
  }
  record(CHECKS[4], saveFailures.length === 0, saveFailures.join('; '));

  // 6: a controlled navigation is answered by the network, not by the worker.
  const navigations = [];
  const noteNavigation = (response) => {
    if (response.request().isNavigationRequest() && response.frame() === page.mainFrame()) navigations.push(response);
  };
  page.on('response', noteNavigation);
  await page.reload({ waitUntil: 'domcontentloaded', timeout: 20000 });
  await page.waitForFunction(() => window.__finished === true, null, { timeout: 15000 });
  page.off('response', noteNavigation);
  const seen = await readPage(page);
  remember('controlled reload', seen);
  const navigation = navigations.at(-1);
  const cacheControl = navigation ? ((await navigation.allHeaders())['cache-control'] ?? '') : '';
  const fromWorker = navigation ? navigation.fromServiceWorker() : null;
  const controlled = await page.evaluate(() => !!(navigator.serviceWorker && navigator.serviceWorker.controller));
  // A served document is answered from the network: the worker keeps copies of
  // cross-origin files, never of the document itself.
  const kept = [];
  if (!navigation) kept.push('the controlled reload produced no navigation response');
  if (!cacheControl.includes('no-store') && fromWorker !== false) {
    kept.push(`the navigation came from the worker with cache-control ${cacheControl || 'none'}`);
  }
  if (!controlled) kept.push('no worker controlled the page');
  record(CHECKS[5], kept.length === 0, kept.length ? kept.join('; ') : `cache-control=${cacheControl || 'none'} fromServiceWorker=${fromWorker}`);

  // 9: a hundred cross-origin files cost about what they cost without a worker.
  const many = `${host.url}/many.html`;
  const manyOff = `${hostOff.url}/many.html`;
  const contextMany = await newContext('many');
  const pageMany = await firstPage(contextMany);
  await visit(pageMany, many, { first: true });
  await measureLoad(pageMany, many, 'many warm-up');
  const controlledLoads = [];
  for (let i = 0; i < 5; i += 1) controlledLoads.push(await measureLoad(pageMany, many, 'many'));
  const contextManyOff = await newContext('many-off');
  const pageManyOff = await firstPage(contextManyOff);
  await visit(pageManyOff, manyOff, { first: true });
  await measureLoad(pageManyOff, manyOff, 'many-off warm-up');
  const plainLoads = [];
  for (let i = 0; i < 5; i += 1) plainLoads.push(await measureLoad(pageManyOff, manyOff, 'many-off'));
  const controlledMedian = median(controlledLoads.map((load) => load.at));
  const plainMedian = median(plainLoads.map((load) => load.at));
  const budget = Math.max(plainMedian * 1.15, plainMedian + 150);
  const counts = [...controlledLoads, ...plainLoads].map((load) => load.scripts);
  const costFailures = [];
  if (counts.some((n) => n !== 100)) costFailures.push(`a page loaded ${counts.join('/')} of 100 scripts`);
  if (controlledMedian > budget) costFailures.push('the controlled page took longer than the budget');
  record(CHECKS[8], costFailures.length === 0, costFailures.length
    ? `${costFailures.join('; ')} (controlled median ${controlledMedian.toFixed(0)}ms, plain ${plainMedian.toFixed(0)}ms, budget ${budget.toFixed(0)}ms)`
    : `controlled median ${controlledMedian.toFixed(0)}ms, plain ${plainMedian.toFixed(0)}ms, budget ${budget.toFixed(0)}ms`);

  // 7: a browser that never saw this page recovers from the copies with one reload.
  await cdn.stop();
  cdn = null;
  const contextFresh = await newContext('fresh');
  const pageFresh = await firstPage(contextFresh);
  let docNavigations = 0;
  pageFresh.on('framenavigated', (frame) => {
    if (frame === pageFresh.mainFrame() && frame.url().startsWith(docURL)) docNavigations += 1;
  });
  await pageFresh.goto(docURL, { waitUntil: 'domcontentloaded', timeout: 20000 });
  await pageFresh.waitForFunction(
    () => window.__finished === true && window.__r && window.__r.classic === 'v2',
    null,
    { timeout: 15000 },
  );
  const fresh = await readPage(pageFresh);
  remember('fresh profile offline', fresh);
  const rescueFailures = diff(fresh.values, V2_VERSIONED);
  const navigationType = await pageFresh.evaluate(() => performance.getEntriesByType('navigation')[0]?.type ?? '');
  if (navigationType !== 'reload') rescueFailures.push(`the recovered page is a ${navigationType || 'unknown'} navigation, not a reload`);
  if (docNavigations !== 2) rescueFailures.push(`${docNavigations} navigations of the document, want 2`);
  record(CHECKS[6], rescueFailures.length === 0, rescueFailures.join('; '));

  // 8: a document that brings its own scope-root worker keeps it.
  const contextOwn = await newContext('own-sw');
  const pageOwn = await firstPage(contextOwn);
  await pageOwn.goto(`${host.url}/own-sw.html`, { waitUntil: 'domcontentloaded', timeout: 20000 });
  await pageOwn.waitForTimeout(3000);
  let ownScript = '';
  for (let attempt = 0; attempt < 14; attempt += 1) {
    ownScript = await pageOwn.evaluate(async () => {
      const registration = await navigator.serviceWorker.getRegistration('/');
      return registration && registration.active ? registration.active.scriptURL : '';
    });
    if (ownScript.endsWith('/own-sw.js')) break;
    await pageOwn.waitForTimeout(500);
  }
  // Look again a moment later: a worker registered after the page's own would replace it.
  await pageOwn.waitForTimeout(2000);
  ownScript = await pageOwn.evaluate(async () => {
    const registration = await navigator.serviceWorker.getRegistration('/');
    const w = registration && (registration.installing || registration.waiting || registration.active);
    return w ? w.scriptURL : '';
  });
  remember('own-sw', await readPage(pageOwn));
  record(CHECKS[7], ownScript.endsWith('/own-sw.js'), `the page's own worker is not the active one: ${ownScript || 'none'}`);

  // 4: no visit ever kept the boot script, in the DOM or in a serialization.
  const carrying = visits.filter((entry) => entry.bootInDom || entry.hasTag);
  record(CHECKS[3], carrying.length === 0, carrying.length
    ? `the boot script stayed in: ${carrying.map((entry) => `${entry.label}${entry.bootInDom ? ' (DOM)' : ''}${entry.hasTag ? ' (serialization)' : ''}`).join(', ')}`
    : `${visits.length} visits`);
}

async function cleanup() {
  for (const context of contexts) {
    try {
      await context.close();
    } catch {
      // A browser already gone is a browser that is gone.
    }
  }
  for (const host of hosts) {
    try {
      await host.stop();
    } catch {
      // Same.
    }
  }
  if (cdn) {
    try {
      await cdn.stop();
    } catch {
      // Same.
    }
    cdn = null;
  }
  for (const dir of [tempRoot, homeBase]) {
    try {
      fs.rmSync(dir, { recursive: true, force: true });
    } catch {
      // Same.
    }
  }
}

let failure = null;
try {
  await run();
} catch (error) {
  failure = error;
  console.log(`the run stopped early: ${error.message.split('\n')[0]}`);
} finally {
  await cleanup();
}

for (const name of CHECKS) {
  if (checks.some((check) => check.name === name)) continue;
  record(name, false, failure ? `not reached: ${failure.message.split('\n')[0]}` : 'the run did not reach it');
}

const ordered = CHECKS.map((name) => checks.find((check) => check.name === name));
const passed = ordered.filter((check) => check.ok).length;
const failed = ordered.length - passed;

if (process.env.TESTBOX_OUT) {
  fs.mkdirSync(process.env.TESTBOX_OUT, { recursive: true });
  fs.writeFileSync(
    path.join(process.env.TESTBOX_OUT, `offline-mirror-e2e-${BROWSER}.json`),
    `${JSON.stringify({ passed, failed, checks: ordered }, null, 2)}\n`,
  );
}

console.log(`${passed} passed, ${failed} failed`);
process.exitCode = failed === 0 ? 0 : 1;
