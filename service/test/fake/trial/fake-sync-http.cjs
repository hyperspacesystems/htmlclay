'use strict';

const http = require('node:http');
const { FakeServer } = require('../tests/helpers/sync-fake-server.js');

const FEATURES = {
  accountScopes: true,
  accountEvents: true,
  conditionalContent: true,
  conditionalStructure: true,
  completeInventory: true
};

function send(res, status, body) {
  const payload = JSON.stringify(body);
  res.writeHead(status, {
    'content-type': 'application/json',
    'content-length': Buffer.byteLength(payload)
  });
  res.end(payload);
}

function failure(res, error) {
  if (error && typeof error.statusCode === 'number') {
    const body = { error: error.message };
    if (error.code !== undefined) body.code = error.code;
    return send(res, error.statusCode, body);
  }
  return send(res, 500, { error: 'fake-server-failed' });
}

function readBody(req) {
  return new Promise((resolve, reject) => {
    const chunks = [];
    req.on('data', (chunk) => chunks.push(chunk));
    req.on('end', () => resolve(Buffer.concat(chunks).toString('utf8')));
    req.on('error', reject);
  });
}

function parseJson(text) {
  if (!text) return {};
  return JSON.parse(text);
}

function encodeContent(content) {
  if (Buffer.isBuffer(content)) return content.toString('base64');
  return content;
}

function decodeContent(content, nodeType) {
  if (nodeType === 'upload') return Buffer.from(content, 'base64');
  return content;
}

function stripQuotes(value) {
  if (typeof value !== 'string') return value;
  return value.replace(/^"(.*)"$/s, '$1');
}

async function startFakeSyncHttp({ nodes = [], key, actor, accounts = [] } = {}) {
  const server = new FakeServer(nodes);
  const requests = [];
  const streams = new Set();

  const httpServer = http.createServer(async (req, res) => {
    const parsed = new URL(req.url, 'http://127.0.0.1');
    const path = parsed.pathname;
    const ifMatchHeader = req.headers['if-match'];
    const authorized = req.headers['x-api-key'] === key;

    requests.push({
      method: req.method,
      path,
      ifMatch: ifMatchHeader === undefined ? null : ifMatchHeader,
      authorized
    });

    if (!authorized) return send(res, 401, { error: 'unauthorized' });

    try {
      if (req.method === 'GET' && path === '/_/sync/nodes') {
        const list = await server.listNodes();
        return send(res, 200, { nodes: list, complete: true });
      }

      if (req.method === 'POST' && path === '/_/sync/nodes') {
        const body = parseJson(await readBody(req));
        const created = await server.createNode(null, {
          type: body.type,
          name: body.name,
          parentId: body.parentId,
          content: decodeContent(body.content, body.type),
          modifiedAt: body.modifiedAt
        });
        return send(res, 200, {
          node: created,
          etag: created.etag,
          checksum: created.checksum,
          structureVersion: created.structureVersion
        });
      }

      if (req.method === 'GET' && path === '/_/sync/accounts') {
        return send(res, 200, { protocol: 2, features: FEATURES, actor, accounts });
      }

      if (req.method === 'GET' && path === '/_/sync/status') {
        return send(res, 200, await server.getServerStatus());
      }

      if (req.method === 'POST' && path === '/_/sync/control') {
        await readBody(req);
        return send(res, 200, { ok: true });
      }

      if (req.method === 'GET' && path === '/_/sync/stream') {
        res.writeHead(200, {
          'content-type': 'text/event-stream',
          'cache-control': 'no-cache'
        });
        // The engine reads unnamed messages whose JSON names the frame's type
        // (engine-sse.js onmessage, session-runner.js), as the server sends them.
        res.write(': connected\n\n');
        res.write(`data: ${JSON.stringify({ type: 'sync-ready', sync: { enabled: true } })}\n\n`);
        streams.add(res);
        res.on('close', () => streams.delete(res));
        return;
      }

      const contentMatch = path.match(/^\/_\/sync\/nodes\/(\d+)\/content$/);
      if (contentMatch) {
        const id = Number(contentMatch[1]);
        if (req.method === 'GET') {
          const result = await server.getNodeContent(null, id);
          return send(res, 200, { ...result, content: encodeContent(result.content) });
        }
        if (req.method === 'PUT') {
          const body = parseJson(await readBody(req));
          const current = await server.getNodeContent(null, id);
          if (ifMatchHeader !== undefined) {
            const expected = stripQuotes(ifMatchHeader);
            if (expected !== current.etag) {
              return send(res, 412, { code: 'conflict', etag: current.etag });
            }
          }
          const node = server.get(id);
          const content = decodeContent(body.content, node ? node.type : undefined);
          const result = await server.putNodeContent(null, id, content);
          return send(res, 200, { ...result, etag: result.checksum });
        }
      }

      const stateMatch = path.match(/^\/_\/sync\/nodes\/(\d+)\/blockeditor-state$/);
      if (stateMatch) {
        const id = Number(stateMatch[1]);
        if (req.method === 'GET') {
          const record = await server.getPageState(null, id);
          return send(res, 200, { record });
        }
        if (req.method === 'PUT') {
          const body = parseJson(await readBody(req));
          await server.putPageState(null, id, body.record === undefined ? null : body.record);
          return send(res, 200, { success: true });
        }
      }

      const renameMatch = path.match(/^\/_\/sync\/nodes\/(\d+)\/rename$/);
      if (renameMatch && req.method === 'PATCH') {
        const body = parseJson(await readBody(req));
        return send(res, 200, await server.renameNode(null, Number(renameMatch[1]), body.newName));
      }

      const moveMatch = path.match(/^\/_\/sync\/nodes\/(\d+)\/move$/);
      if (moveMatch && req.method === 'PATCH') {
        const body = parseJson(await readBody(req));
        const result = await server.moveNode(
          null,
          Number(moveMatch[1]),
          body.targetParentId,
          body.newName || null
        );
        return send(res, 200, result);
      }

      const restoreMatch = path.match(/^\/_\/sync\/nodes\/(\d+)\/restore$/);
      if (restoreMatch && req.method === 'POST') {
        return send(res, 200, await server.restoreNode(null, Number(restoreMatch[1])));
      }

      const nodeMatch = path.match(/^\/_\/sync\/nodes\/(\d+)$/);
      if (nodeMatch && req.method === 'DELETE') {
        const cascade = parsed.searchParams.get('cascade') === 'true';
        return send(res, 200, await server.deleteNode(null, Number(nodeMatch[1]), { cascade }));
      }

      return send(res, 404, { error: 'not-found' });
    } catch (error) {
      return failure(res, error);
    }
  });

  await new Promise((resolve, reject) => {
    httpServer.once('error', reject);
    httpServer.listen(0, '127.0.0.1', resolve);
  });

  const { port } = httpServer.address();

  return {
    url: `http://127.0.0.1:${port}`,
    server,
    requests,
    // How many event streams a client holds open right now.
    openStreams() {
      return streams.size;
    },
    // Sends one frame to every open stream, as the server does when a node changes.
    push(data) {
      for (const res of streams) res.write(`data: ${JSON.stringify(data)}\n\n`);
    },
    close() {
      return new Promise((resolve, reject) => {
        for (const res of streams) {
          try {
            res.end();
          } catch {
            // the client may already be gone
          }
        }
        streams.clear();
        // A client still syncing holds keep-alive connections open; without this,
        // close would wait for it forever.
        httpServer.close((error) => (error ? reject(error) : resolve()));
        httpServer.closeAllConnections();
        if (typeof httpServer.closeAllConnections === 'function') {
          httpServer.closeAllConnections();
        }
      });
    }
  };
}

module.exports = { startFakeSyncHttp };
