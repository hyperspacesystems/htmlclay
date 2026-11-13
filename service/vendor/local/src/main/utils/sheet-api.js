// The sheet side of the data API: the shared reader's loader, its URL parameter parser,
// patch writes and the rule that tells a patch body from data. Kept apart from
// data-extractor.js so a test that replaces the extractor still gets these real.
const cheerio = require('cheerio');

let readPromise = null;
let QueryRefused = null;
function loadRead() {
  if (!readPromise) {
    readPromise = Promise.all([
      import('hyper-html-api/read'),
      import('hyper-html-api/codecs/sheet')
    ]).then(([read, sheet]) => {
      QueryRefused = sheet.QueryRefused;
      return read;
    });
  }
  return readPromise;
}

let writePromise = null;
function loadWrite() {
  if (!writePromise) writePromise = import('hyper-html-api/write');
  return writePromise;
}

// The sheet read parameters (shape, sheet, range, render) in a request URL, parsed by
// the rule every host shares: null when there are none (a whole read); throws
// QueryRefused for a repeated or empty parameter or one mixed with ?data=. Values are
// checked by the reader.
const SHEET_PARAMS = ['shape', 'sheet', 'range', 'render'];

async function sheetQueryOf(url) {
  const params = new URLSearchParams(url.includes('?') ? url.slice(url.indexOf('?') + 1) : '');
  if (!SHEET_PARAMS.some((name) => params.has(name))) return null;
  const { parseSheetQuery } = await loadRead();
  return parseSheetQuery(params);
}

// A sheet query the reader refused (a bad range, render, sheet or patch). Only the
// reader throws one, so the class is loaded by the time this is asked.
function isQueryRefused(error) {
  return QueryRefused !== null && error instanceof QueryRefused;
}

// Applies a patch list (set and append operations on a sheet) through the document's own
// rules tag. Returns { html, changed: { cells, rows }, stale }; throws the reader's
// QueryRefused for a bad operation and CodecRefused for a table it cannot patch.
async function patchViaTag(html, patch, token) {
  const { patchDocument } = await loadWrite();
  return patchDocument(cheerio.load, html, patch, { token });
}

// Whether a POST body is a patch rather than data, by the rule all three hosts share
// (hyper-html-api's isPatchBody): { operationId?, patch: [operations] } sent to a
// document whose rules tag has no top-level `patch` key. A tag that cannot be read
// counts as having no rules; the write that follows reports why.
async function isPatchBodyViaTag(body, html, token) {
  if (!body || typeof body !== 'object' || !Array.isArray(body.patch)) return false;
  const [{ isPatchBody }, { findRulesIn }, { default: cheerioAdapter }] = await Promise.all([
    loadWrite(), import('hyper-html-api/engine'), import('hyper-html-api/cheerio')
  ]);
  let rules = null;
  try {
    const found = findRulesIn(cheerioAdapter, cheerio.load(html).root(), token);
    rules = found ? found.rules : null;
  } catch {}
  return isPatchBody(body, rules);
}

module.exports = { loadRead, isQueryRefused, sheetQueryOf, patchViaTag, isPatchBodyViaTag };
