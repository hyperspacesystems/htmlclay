// /_/offline/sw.js, served by the HTML Clay host. Version: {{VERSION}}
const FIRST_WAIT_MS = 4000;
const SHORT_WAIT_MS = 800;
const DOWN_MS = 30000;
const HAND_OVER_EVERY_MS = 10 * 60 * 1000;
const MAX_BYTES = Number('{{MAX_BYTES}}');
const KEEP_IMAGES = Number('{{IMAGES}}') === 1;
const KEEP_VIDEO = Number('{{VIDEO}}') === 1;
const KEEP_AUDIO = Number('{{AUDIO}}') === 1;
const downUntil = new Map();
const handedAt = new Map();

self.addEventListener('install', (event) => {
  self.skipWaiting();
  if (event.addRoutes) {
    event.waitUntil(event.addRoutes([
      { condition: { requestMode: 'navigate' }, source: 'network' },
      { condition: { urlPattern: new URLPattern(self.location.origin + '/*') }, source: 'network' },
    ]).catch(() => {}));
  }
});
self.addEventListener('activate', (event) => event.waitUntil(self.clients.claim()));
self.addEventListener('fetch', (event) => {
  const req = event.request;
  if (media(req)) event.respondWith(mediaFirst(event, req));
  else if (mirrored(req)) event.respondWith(networkFirst(event, req));
});

function mirrored(req) {
  if (req.method !== 'GET' || req.mode === 'navigate') return false;
  let url;
  try { url = new URL(req.url); } catch { return false; }
  if (url.origin === self.location.origin) return false;
  if (url.protocol !== 'https:' && url.protocol !== 'http:') return false;
  if (url.username || url.password) return false;
  if (req.headers.has('range') || req.headers.has('authorization')) return false;
  if (req.mode === 'cors' && req.credentials === 'include') return false;
  if ((req.headers.get('accept') || '').includes('text/event-stream')) return false;
  return !['video', 'audio', 'track'].includes(req.destination) && (KEEP_IMAGES || req.destination !== 'image');
}

async function networkFirst(event, req) {
  const host = new URL(req.url).host;
  const wait = (downUntil.get(host) || 0) > Date.now() ? SHORT_WAIT_MS : FIRST_WAIT_MS;
  const live = fetch(req);
  const first = await Promise.race([
    live.then((res) => ({ res }), (error) => ({ error })),
    new Promise((resolve) => setTimeout(() => resolve({ slow: true }), wait)),
  ]);
  if (first.res) {
    downUntil.delete(host);
    event.waitUntil(handOver(req, first.res.clone()));
    return first.res;
  }
  downUntil.set(host, Date.now() + DOWN_MS);
  const copy = await fromCopy(req.url);
  if (first.slow) {
    // The page may still get this same response (no copy yet), so hand over a clone.
    event.waitUntil(live.then((res) => { downUntil.delete(host); return handOver(req, res.clone()); }, () => {}));
    return copy || live;
  }
  return copy || Response.error();
}

// A media element's request (it usually carries Range): copied whole by the host,
// answered from the copy by byte range when the network is gone.
function media(req) {
  if (req.method !== 'GET') return false;
  if (!(req.destination === 'video' ? KEEP_VIDEO : req.destination === 'audio' ? KEEP_AUDIO : false)) return false;
  let url;
  try { url = new URL(req.url); } catch { return false; }
  if (url.origin === self.location.origin || url.username || url.password) return false;
  if (url.protocol !== 'https:' && url.protocol !== 'http:') return false;
  if (req.headers.has('authorization')) return false;
  if (req.mode === 'cors' && req.credentials === 'include') return false;
  return true;
}

async function mediaFirst(event, req) {
  const host = new URL(req.url).host;
  const wait = (downUntil.get(host) || 0) > Date.now() ? SHORT_WAIT_MS : FIRST_WAIT_MS;
  const live = fetch(req);
  const first = await Promise.race([
    live.then((res) => ({ res }), (error) => ({ error })),
    new Promise((resolve) => setTimeout(() => resolve({ slow: true }), wait)),
  ]);
  if (first.res) {
    downUntil.delete(host);
    if (due(req.url, '')) event.waitUntil(nominateMedia(req));
    return first.res;
  }
  downUntil.set(host, Date.now() + DOWN_MS);
  const copy = await fromCopyRange(req.url, req.headers.get('range'));
  if (first.slow) {
    event.waitUntil(live.then(() => { downUntil.delete(host); return due(req.url, '') ? nominateMedia(req) : undefined; }, () => {}));
    return copy || live;
  }
  return copy || Response.error();
}

function nominateMedia(req) {
  return fetch('/_/offline/nominate', {
    method: 'POST', headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify([{ url: req.url, kind: req.destination }]),
  }).catch(() => {});
}

async function fromCopyRange(url, range) {
  const hit = await fetch('/_/offline/copy?url=' + encodeURIComponent(url), {
    cache: 'no-store', headers: range ? { Range: range } : {},
  }).catch(() => null);
  if (!hit || (hit.status !== 200 && hit.status !== 206)) return null;
  const headers = { 'Content-Type': hit.headers.get('Content-Type') || 'application/octet-stream', 'Accept-Ranges': 'bytes' };
  for (const name of ['Content-Length', 'Content-Range']) {
    const v = hit.headers.get(name);
    if (v) headers[name] = v;
  }
  return new Response(hit.body, { status: hit.status, headers });
}

async function fromCopy(url) {
  const hit = await fetch('/_/offline/copy?url=' + encodeURIComponent(url), { cache: 'no-store' }).catch(() => null);
  if (!hit || !hit.ok) return null;
  return new Response(hit.body, { status: 200, headers: { 'Content-Type': hit.headers.get('Content-Type') || 'application/octet-stream' } });
}

const tooBig = (res) => Number(res.headers.get('content-length') || 0) > MAX_BYTES;
const READ_CAP = Math.min(MAX_BYTES, 1 << 30);

// Reads a body in chunks and gives up past the cap, so a response with no length (a
// live feed, a large stream) never sits whole in the worker's memory.
async function capped(res) {
  if (!res.body) return null;
  const reader = res.body.getReader();
  const parts = [];
  let size = 0;
  for (;;) {
    const { done, value } = await reader.read();
    if (done) break;
    size += value.byteLength;
    if (size > READ_CAP) { reader.cancel().catch(() => {}); return null; }
    parts.push(value);
  }
  return new Blob(parts);
}
const validator = (res) => res.headers.get('etag') || res.headers.get('last-modified') || '';

// Hands the host a copy when the file changed (a new ETag or Last-Modified), and for a
// file with neither, at most once per 10 minutes per worker lifetime.
function due(url, tag) {
  const seen = handedAt.get(url);
  if (seen && seen.tag === tag && (tag || Date.now() - seen.at < HAND_OVER_EVERY_MS)) return false;
  handedAt.set(url, { tag, at: Date.now() });
  return true;
}

async function handOver(req, res) {
  if (res.type === 'opaque') {
    // The browser could not read this answer, so reading the bytes takes a second
    // request. Gate it like any hand-over, so a counter or pixel is not hit twice per load.
    if (!due(req.url, '')) return;
    const src = await fetch(req.url, { mode: 'cors', credentials: 'omit' }).catch(() => null);
    if (!src) return send(req, null, '');
    if (src.status !== 200 || tooBig(src)) return;
    const body = await capped(src);
    if (body) return send(req, body, src.headers.get('content-type') || '');
    return;
  }
  if (res.status !== 200 || tooBig(res) || !due(req.url, validator(res))) return;
  const body = await capped(res);
  if (body) return send(req, body, res.headers.get('content-type') || '');
}

function send(req, body, type) {
  return fetch('/_/offline/store?url=' + encodeURIComponent(req.url) + '&dest=' + encodeURIComponent(req.destination), {
    method: 'POST', headers: body ? { 'Clay-Content-Type': type } : {}, body: body || undefined,
  }).catch(() => {});
}
