const cheerio = require('cheerio');
const { normCanonical } = require('../local-authority');

let pageModules;
let checkpointModule;

function hasPageStore(html) {
  const $ = cheerio.load(html);
  return $('script[merge][clay~="live"]').toArray().some((node) => /^blockeditor(?:\.[0-9]+)?$/.test($(node).attr('merge')));
}

function loadPageModules() {
  pageModules ??= Promise.all([
    import('hyper-html-api/page'),
    import('@panphora/clayjs/page/yjs'),
  ]).then(([page, codec]) => ({ ...page, codec }));
  return pageModules;
}

function loadCheckpoint() {
  checkpointModule ??= import('@panphora/clayjs/server/checkpoint');
  return checkpointModule;
}

async function authoritativePageHtml(authority, target, html) {
  const roomKey = normCanonical(target.canonicalPath);
  const room = authority.rooms.get(roomKey);
  if (!room || String(room.nodeId) !== String(target.identity) || !authority.roomIsDirty(room)) return html;
  const { captureCheckpoint, renderCheckpoint } = await loadCheckpoint();
  const { JSDOM } = require('jsdom');
  const capture = captureCheckpoint(room, { nodeId: target.identity, roomKey });
  const rendered = renderCheckpoint(html, capture, {
    createDocument: () => new JSDOM('<!doctype html><html><body></body></html>', {
      runScripts: undefined, resources: undefined,
    }).window.document,
  });
  if (rendered.kind !== 'ok') {
    const error = new Error(`The live page cannot be rendered: ${rendered.reason}`);
    error.code = rendered.reason;
    throw error;
  }
  return rendered.html;
}

module.exports = { hasPageStore, loadPageModules, loadCheckpoint, authoritativePageHtml };
