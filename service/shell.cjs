'use strict';

// The dashboard's workspace API: Hyperclay Local's own workspace routes
// (vendor/local/src/main/workspace/shell-server.js) mounted on the service's
// listener under /v1/shell/. Go admits each request at the dashboard
// (internal/dashboard) and forwards it with the bearer, both epochs and
// X-Trial-Public-Origin, the dashboard's origin. Every served folder is a root of
// kind "local" with no account. Every file command writes through Go: Local's
// commands run with the root's Go document host (commit, restructure and page
// state become control calls) and with Go making folders. Commands are switched on
// group by group as each is tested against the real Go host; the rest answer 501.

const fs = require('fs');
const path = require('path');
const { randomBytes } = require('crypto');
const { Readable } = require('stream');
const chokidar = require('chokidar');
const { createShellServer } = require('./vendor/local/src/main/workspace/shell-server.js');
const { IGNORED } = require('./vendor/local/src/main/root-observer.js');

const PREFIX = '/v1/shell';
const READ = /^\/api\/(bootstrap|roots|events|roots\/[^/]+\/(tree|list|search|document))$/;
const COMMANDS_PATH = /^\/api\/roots\/[^/]+\/commands$/;
const COMMANDS = new Set(['create', 'folder']);
const MAX_BODY = 1024 * 1024;

// The bundle's manifest, for the protocol number the bootstrap reports: beside
// this file in a packaged service (build.mjs), in the Go tree when run from source.
function readManifest() {
  const found = [
    path.join(__dirname, 'workspace-manifest.json'),
    path.join(__dirname, '..', 'internal', 'dashboard', 'assets', 'local-app', 'manifest.json'),
  ].find((file) => fs.existsSync(file));
  if (!found) throw new Error('the workspace manifest is missing');
  return JSON.parse(fs.readFileSync(found, 'utf8'));
}

// A read-only watcher feeding one root's file tree (RootTree calls subscribe and
// start). The room observer is not used: in the service it never watches, because
// Go watches the folders and reconciles through it.
function treeWatcher(root) {
  const listeners = new Set();
  let watcher = null;
  return {
    subscribe(listener) {
      listeners.add(listener);
      return () => listeners.delete(listener);
    },
    start() {
      if (watcher) return;
      watcher = chokidar.watch('**/*', { cwd: root.path, ignoreInitial: true, followSymlinks: false, ignored: IGNORED });
      for (const event of ['add', 'addDir', 'change', 'unlink', 'unlinkDir']) {
        watcher.on(event, (rel) => {
          for (const listener of listeners) listener({ event, rel: rel.split(path.sep).join('/') });
        });
      }
      watcher.on('error', (error) => console.error(`[htmlclay-service] watching ${root.id}: ${error.message}`));
    },
    close() {
      const closing = watcher?.close();
      watcher = null;
      return closing ?? Promise.resolve();
    },
  };
}

// Reads a request's body and answers it with a copy of the request that can be
// read again, for the routes that look at a command before Local's code runs it.
async function buffered(req) {
  const chunks = [];
  let size = 0;
  for await (const chunk of req) {
    size += chunk.length;
    if (size > MAX_BODY) return { tooLarge: true };
    chunks.push(chunk);
  }
  const body = Buffer.concat(chunks);
  const copy = Readable.from(body.length ? [body] : []);
  Object.assign(copy, { method: req.method, url: req.url, headers: req.headers });
  return { copy, body };
}

function commandOf(body) {
  try {
    return JSON.parse(body.toString('utf8'))?.command;
  } catch {
    return null;
  }
}

function refuse(res, status, reason, error) {
  res.writeHead(status, { 'Content-Type': 'application/json; charset=utf-8', 'Cache-Control': 'no-store' });
  res.end(JSON.stringify({ error, reason }));
}

// Mounts the workspace routes on `service` (createTrialService). Answers close().
function mountShell(service, { appVersion }) {
  const manifest = readManifest();
  const watchers = new Map();
  let shell = null;
  let origin = null;

  const record = (id) => service.roots.get(id);
  const settings = () => ({
    roots: [...service.roots.values()].map((item) => ({ id: item.id, kind: 'local', path: item.path })),
    syncSessions: [],
  });
  const observerFor = (id) => {
    if (!record(id)) return null;
    if (!watchers.has(id)) watchers.set(id, treeWatcher(record(id)));
    return watchers.get(id);
  };

  // The writes Local's commands make outside the document host, made by Go.
  const structure = async (id, kind, steps) => {
    const answer = await service.goCall('POST', '/v1/restructure', { opId: randomBytes(16).toString('hex'), root: id, kind, steps });
    if (answer?.outcome !== 'committed') {
      throw Object.assign(new Error(`${kind} did not complete: ${answer?.reason ?? 'no answer'}`), { code: answer?.reason ?? 'failed' });
    }
  };
  const commandHostFor = (root) => ({
    documentHost: record(root.id).goHost,
    files: {
      mkdir: (destination, { rel }) => structure(root.id, 'folder', [{ to: rel }]),
      emptyTrash: () => structure(root.id, 'empty-trash', []),
    },
  });

  const create = () => createShellServer({
    authority: service.authority,
    rootPool: { get: (id) => (record(id) ? { state: 'running', deps: {} } : null) },
    observerFor, settings, appVersion, origin, manifest,
    connection: () => ({ online: true, account: { signedIn: false, username: null } }),
    rootTitleFor: (root) => path.basename(root.path),
    rootOriginFor: (root) => new URL(record(root.id).origin).origin,
    commandHostFor,
  });

  const stopRoots = service.onRoots(() => {
    shell?.rootsChanged();
    for (const [id, watcher] of watchers) {
      if (!record(id)) {
        watchers.delete(id);
        watcher.close().catch(() => {});
      }
    }
  });

  service.mount(`${PREFIX}/`, async (req, res) => {
    const publicOrigin = String(req.headers['x-trial-public-origin'] ?? '');
    if (!/^http:\/\/[^/]+$/.test(publicOrigin)) return refuse(res, 400, 'forbidden', 'route');
    if (!shell) {
      origin = publicOrigin;
      shell = create();
    }
    // The dashboard's port is fixed for the app's life, so its origin is too.
    if (publicOrigin !== origin) return refuse(res, 400, 'forbidden', 'route');
    req.url = req.url.slice(PREFIX.length);
    delete req.headers.authorization;
    for (const name of Object.keys(req.headers)) {
      if (name.startsWith('x-trial-')) delete req.headers[name];
    }
    const { pathname } = new URL(req.url, origin);
    if (req.method === 'POST' && COMMANDS_PATH.test(pathname)) {
      const { copy, body, tooLarge } = await buffered(req);
      if (tooLarge) return refuse(res, 413, 'forbidden', 'Request is too large');
      if (!COMMANDS.has(commandOf(body))) return refuse(res, 501, 'unsupported', 'Not available yet');
      return shell.api(copy, res);
    }
    if (req.method !== 'GET' || !READ.test(pathname)) return refuse(res, 501, 'unsupported', 'Not available yet');
    return shell.api(req, res);
  });

  return {
    async close() {
      stopRoots();
      await shell?.stop();
      await Promise.all([...watchers.values()].map((watcher) => watcher.close()));
      watchers.clear();
    },
  };
}

module.exports = { mountShell };
