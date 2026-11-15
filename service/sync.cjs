'use strict';

// Sync in the service. Go holds the sync key in the system's secret store and
// hands it over on stdin (08-sync.md, C2); the service keeps it only in memory and
// never writes, logs or echoes it, or any part of the line that carried it.

const { getAccounts } = require('./vendor/local/src/sync-engine/api-client.js');

const KEY = /^hcsk_[0-9a-f]{64}$/;
const PREFIX = '/sync/v1/';
const BODY_LIMIT = 64 * 1024;
// Local's rule (sync-manager.js REQUIRED_FEATURES): a server missing any of these
// is one that needs updating before it can sync, not one to sync with blind.
const REQUIRED_FEATURES = ['accountScopes', 'accountEvents', 'conditionalContent', 'conditionalStructure', 'completeInventory'];
const LOOPBACK = new Set(['127.0.0.1', 'localhost', '[::1]']);

// serverOrigin answers the origin a key may be sent to, or null. https always; http
// only to loopback, which is where a test's stand-in server listens.
function serverOrigin(serverUrl) {
  let url;
  try {
    url = new URL(serverUrl);
  } catch {
    return null;
  }
  if (url.username || url.password) return null;
  if (url.protocol === 'https:') return url.origin;
  if (url.protocol === 'http:' && LOOPBACK.has(url.hostname)) return url.origin;
  return null;
}

function createSync({ gate }) {
  let held = null;
  return {
    // handle answers the stdout line for one {"sync":...} message.
    async handle(message) {
      if (!message || message.v !== 1) return 'SYNC KEY REFUSED';
      if (message.key === null) {
        held = null;
        gate.clear();
        return 'SYNC KEY CLEARED';
      }
      const origin = serverOrigin(message.serverUrl);
      if (typeof message.key !== 'string' || !KEY.test(message.key) || origin === null) {
        return 'SYNC KEY REFUSED';
      }
      held = { key: message.key, serverOrigin: origin };
      gate.hold(origin);
      return 'SYNC KEY HELD';
    },
    connection() { return held && { ...held }; },
    discover() { return discover(held); },
  };
}

// discover answers C3's /sync/v1/discover body for the held key.
async function discover(held) {
  if (!held) return { ok: false, code: 'not-connected' };
  let answer;
  try {
    answer = await getAccounts({ serverUrl: held.serverOrigin, apiKey: held.key });
  } catch (error) {
    const status = error && error.statusCode;
    if (status === 401 || status === 403) return { ok: false, code: 'key-refused', status };
    return { ok: false, code: 'offline', ...(status ? { status } : {}) };
  }
  const features = (answer && answer.features) || {};
  if (!REQUIRED_FEATURES.every((name) => features[name] === true) || !(answer.protocol >= 2)) {
    return { ok: false, code: 'server-update-required' };
  }
  const actor = answer.actor;
  if (!actor || !Number.isInteger(actor.id) || typeof actor.username !== 'string' || !Array.isArray(answer.accounts)) {
    return { ok: false, code: 'bad-answer' };
  }
  return {
    ok: true,
    actor: { id: actor.id, username: actor.username, fullName: (actor.person && actor.person.name) || actor.username },
    accounts: answer.accounts,
  };
}

function readJson(req) {
  return new Promise((resolve) => {
    let size = 0;
    const chunks = [];
    req.on('data', (chunk) => {
      size += chunk.length;
      if (size > BODY_LIMIT) {
        resolve(undefined);
        req.destroy();
        return;
      }
      chunks.push(chunk);
    });
    req.on('end', () => {
      try {
        resolve(chunks.length ? JSON.parse(Buffer.concat(chunks).toString('utf8')) : {});
      } catch {
        resolve(undefined);
      }
    });
    req.on('error', () => resolve(undefined));
  });
}

function send(res, status, body) {
  res.writeHead(status, { 'content-type': 'application/json' });
  res.end(JSON.stringify(body));
}

// mountSync answers Go's calls under /sync/v1/ (C3). The listener has already
// checked the host, Go's bearer and both epochs before a mount is reached.
function mountSync(service, sync) {
  const routes = new Map([[`${PREFIX}discover`, () => sync.discover()]]);
  service.mount(PREFIX, async (req, res) => {
    const { pathname } = new URL(req.url, 'http://service');
    const route = req.method === 'POST' ? routes.get(pathname) : undefined;
    if (!route) return send(res, 404, { ok: false, error: 'not-found' });
    const body = await readJson(req);
    if (body === undefined) return send(res, 400, { ok: false, error: 'bad-request' });
    try {
      send(res, 200, await route(body));
    } catch (error) {
      console.error('[htmlclay-service] sync route failed:', error && error.message);
      send(res, 500, { ok: false, error: 'failed' });
    }
  });
}

module.exports = { createSync, mountSync, serverOrigin };
