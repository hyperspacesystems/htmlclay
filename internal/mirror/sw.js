// /_/offline/sw.js, served by the HTML Clay host. Version: {{VERSION}}
const FIRST_WAIT_MS = 4000;
const SHORT_WAIT_MS = 800;
const DOWN_MS = 30000;
const HAND_OVER_EVERY_MS = 10 * 60 * 1000;
const MAX_BYTES = {{MAX_BYTES}};
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
  if (mirrored(event.request)) event.respondWith(networkFirst(event, event.request));
});

function mirrored(req) {
  if (req.method !== 'GET' || req.mode === 'navigate') return false;
  let url;
  try { url = new URL(req.url); } catch { return false; }
  if (url.origin === self.location.origin) return false;
  if (url.protocol !== 'https:' && url.protocol !== 'http:') return false;
  if (req.headers.has('range') || req.headers.has('authorization')) return false;
  if (req.mode === 'cors' && req.credentials === 'include') return false;
  if ((req.headers.get('accept') || '').includes('text/event-stream')) return false;
  return !['video', 'audio', 'track'].includes(req.destination);
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
    event.waitUntil(live.then((res) => { downUntil.delete(host); return handOver(req, res); }, () => {}));
    return copy || live;
  }
  return copy || Response.error();
}

async function fromCopy(url) {
  const hit = await fetch('/_/offline/copy?url=' + encodeURIComponent(url), { cache: 'no-store' }).catch(() => null);
  if (!hit || !hit.ok) return null;
  return new Response(hit.body, { status: 200, headers: { 'Content-Type': hit.headers.get('Content-Type') || 'application/octet-stream' } });
}

const tooBig = (res) => Number(res.headers.get('content-length') || 0) > MAX_BYTES;
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
  let src = res;
  if (res.type === 'opaque') {
    src = await fetch(req.url, { mode: 'cors', credentials: 'omit' }).catch(() => null);
    if (!src) return due(req.url, 'host') && send(req, null, '');
  }
  if (src.status !== 200 || tooBig(src) || !due(req.url, validator(src))) return;
  return send(req, await src.arrayBuffer(), src.headers.get('content-type') || '');
}

function send(req, body, type) {
  return fetch('/_/offline/store?url=' + encodeURIComponent(req.url) + '&dest=' + encodeURIComponent(req.destination), {
    method: 'POST', headers: body ? { 'Clay-Content-Type': type } : {}, body: body || undefined,
  }).catch(() => {});
}
