// Ported from hyperclay server-lib/ordered-room/region.js (the host lanes' copy): the same tag walk,
// so a browser and a host find the same region in the same page.
// Where a document's ordered-room region sits in its page. The room registry and the sheet codec
// read and write only the region (the workbook element's inner bytes: every tab and the one
// `sheet-commit` script); the page around it is the host's. The region is the inner bytes of the
// first element that carries the region attribute. Comments, the doctype and the bodies of
// raw-text elements are stepped over, so a script or a comment that mentions the attribute is
// never taken for the region.

export const REGIONS = Object.freeze({ sheet: Object.freeze({ attribute: 'sheet-workbook', record: 'sheet-commit' }) });

const RAW = new Set(['script', 'style', 'textarea', 'title', 'xmp', 'iframe', 'noembed', 'noframes', 'noscript', 'plaintext']);
const NAME = /<(\/?)([a-zA-Z][a-zA-Z0-9-]*)/y;
const ATTR = /\s+([^\s"'>\/=]+)(?:\s*=\s*(?:"[^"]*"|'[^']*'|[^\s"'=<>`]+))?/y;
const CLOSE = /\s*\/?>/y;

function tagAt(html, at) {
  NAME.lastIndex = at;
  const name = NAME.exec(html);
  if (!name) return null;
  const attrs = new Set();
  let i = NAME.lastIndex;
  for (;;) {
    ATTR.lastIndex = i;
    const attr = ATTR.exec(html);
    if (!attr) break;
    attrs.add(attr[1].toLowerCase());
    i = ATTR.lastIndex;
  }
  CLOSE.lastIndex = i;
  if (!CLOSE.exec(html)) return null;
  return { closing: name[1] === '/', name: name[2].toLowerCase(), attrs, end: CLOSE.lastIndex };
}

// Where a raw-text element's body ends: its closing tag, matched without case and only when the
// name is followed by whitespace, `/` or `>` (so `</scripture>` inside a script does not end it).
// A regex search from the position keeps offsets exact: lowercasing the page first moves them
// after characters such as U+0130, whose lower case is two code units long.
function rawTextEnd(html, name, from) {
  const close = new RegExp(`</${name}(?=[\\s/>])`, 'gi');
  close.lastIndex = from;
  const found = close.exec(html);
  return found ? found.index : -1;
}

// Calls onTag(tag, at) for every start and end tag from `from` on, in order, until it returns
// false or the bytes end. The contents of a `<template>` are inert in a browser, so tags inside
// one are stepped over and never reported.
function walk(html, from, onTag) {
  let i = from;
  let template = 0;
  while (i < html.length) {
    const lt = html.indexOf('<', i);
    if (lt < 0) return;
    if (html.startsWith('<!--', lt)) {
      if (html[lt + 4] === '>') {
        i = lt + 5;
        continue;
      }
      if (html.startsWith('->', lt + 4)) {
        i = lt + 6;
        continue;
      }
      const end = html.indexOf('-->', lt + 4);
      if (end < 0) return;
      i = end + 3;
      continue;
    }
    if (html[lt + 1] === '!' || html[lt + 1] === '?') {
      const end = html.indexOf('>', lt);
      if (end < 0) return;
      i = end + 1;
      continue;
    }
    const tag = tagAt(html, lt);
    if (!tag) {
      i = lt + 1;
      continue;
    }
    if (tag.name === 'template') template = Math.max(0, template + (tag.closing ? -1 : 1));
    else if (template === 0 && onTag(tag, lt) === false) return;
    i = tag.end;
    if (!tag.closing && RAW.has(tag.name)) {
      const end = rawTextEnd(html, tag.name, i);
      if (end < 0) return;
      i = end;
    }
  }
}

/**
 * The region of a page: `{ codec, start, end, inner }` with html.slice(start, end) === inner, or
 * null when the page has no region element or the element never closes.
 * @param {string} html
 */
export function findRegion(html) {
  if (typeof html !== 'string') return null;
  for (const [codec, { attribute }] of Object.entries(REGIONS)) {
    if (!new RegExp(attribute, 'i').test(html)) continue;
    let open = null;
    walk(html, 0, (tag) => {
      if (tag.closing || !tag.attrs.has(attribute)) return true;
      open = tag;
      return false;
    });
    if (!open) continue;
    let depth = 1;
    let end = -1;
    walk(html, open.end, (tag, at) => {
      if (tag.name !== open.name) return true;
      depth += tag.closing ? -1 : 1;
      if (depth > 0) return true;
      end = at;
      return false;
    });
    if (end < 0) return null;
    return { codec, start: open.end, end, inner: html.slice(open.end, end) };
  }
  return null;
}

/** The page with its region's inner bytes replaced. */
export const spliceRegion = (html, region, inner) => html.slice(0, region.start) + inner + html.slice(region.end);

/**
 * The region when it holds a commit record (a `<script ... sheet-commit>` start tag), else null.
 * Every rule for documents that co-edit in an ordered room keys on this.
 * @param {string} html
 */
export function commitRegion(html) {
  const region = findRegion(html);
  if (!region) return null;
  const { record } = REGIONS[region.codec];
  let found = false;
  walk(region.inner, 0, (tag) => {
    if (tag.closing || tag.name !== 'script' || !tag.attrs.has(record)) return true;
    found = true;
    return false;
  });
  return found ? region : null;
}

/** The page with its commit record replaced by `record`, or the page as it is when it has none. */
export function replaceRecord(html, record) {
  const region = commitRegion(html);
  if (!region) return html;
  const { record: attribute } = REGIONS[region.codec];
  let body = null;
  walk(region.inner, 0, (tag) => {
    if (tag.closing || tag.name !== 'script' || !tag.attrs.has(attribute)) return true;
    body = { start: tag.end, end: rawTextEnd(region.inner, 'script', tag.end) };
    return false;
  });
  if (!body || body.end < 0) return html;
  const json = JSON.stringify(record).replaceAll('<', '\\u003c');
  return spliceRegion(html, region, region.inner.slice(0, body.start) + json + region.inner.slice(body.end));
}
