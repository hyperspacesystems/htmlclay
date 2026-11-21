// clay.liveRoom in a browser: the room client (room-client.js) over the page's own host. The
// client speaks the reference host's routes and region bytes; this transport maps them onto the
// host port of plan 09 section 2.1, the same mapping hyperclay's headless client adapter keeps
// (hyperclay tests/helpers/ordered-room-client.js):
// - `/_/file` reads the page fresh from the host and answers its region (the `[sheet-workbook]`
//   element's inner bytes) with the page's document ETag;
// - `/_/save` puts the client's region bytes into the page it read with that ETag (or, through
//   `page`, into the page as ClayJS saves it) and sends it to the host's save route with If-Match;
// - room and copy routes go to the host with the document named by Document-URL;
// - the stream is the one live sync holds open: `room` events, file pushes, and a host's flush.
import { onStream, currentStream, onHostFlush } from "../live/stream-bridge.js";
import { saveToken } from "../core/host-attrs.js";
import { STRIP_FROM_SAVE } from "../lib/region-policy.js";
import { findRegion, spliceRegion } from "./region.js";
import { connect } from "./room-client.js";

const PAGES_KEPT = 4;
const STREAM_WAIT_MS = 10_000;

// Requests go to the page's own origin, as every other ClayJS request does, so a <base href> can
// never send them (or a save token in the path) elsewhere.
const onOrigin = (path) => (typeof location === "undefined" ? path : new URL(path, location.origin).href);

// A host may add elements to the page it serves that its file does not hold (hyperclay's trial bar
// is marked save-remove). ClayJS's own save drops them; a save built on the served page drops them
// the same way, so they never become part of the file. A page with none keeps its bytes exactly.
// The calculator's stale mark (<meta name="clay-stale" clay="no-save">) stays: the page as served
// still holds the old computed values, and only ClayJS's repaint, which clears the mark, renews them.
export function savedPage(base) {
  if (typeof DOMParser === "undefined") return base;
  const doc = new DOMParser().parseFromString(base, "text/html");
  const extra = [...doc.querySelectorAll(STRIP_FROM_SAVE)].filter((element) => !element.matches('meta[name="clay-stale"]'));
  if (!extra.length) return base;
  for (const element of extra) element.remove();
  const start = base.search(/<html\b/i);
  return `${start > 0 ? base.slice(0, start) : "<!DOCTYPE html>\n"}${doc.documentElement.outerHTML}`;
}

// What a host's live stream says when the file changed outside this tab's room: a push with the
// new ETag or page (hyperclay, HTML Clay's saved lane), HTML Clay's notice of an outside change
// ({type: "notification", data: {kind: "external-change"}}) or of a restored version (a
// notification with no kind). The tab reads the file again; reading too often costs one read.
const fileChanged = (data) => data?.type === "notification" || typeof data?.etag === "string" || typeof data?.html === "string" || data?.kind === "external-change";

const json = (status, body) => new Response(JSON.stringify(body), { status, headers: { "content-type": "application/json" } });

// The ETag of the document a page was built from: the `documentetag` the host stamps on <html>
// (spec section 4). An HTTP ETag header names the served bytes, which carry that stamp, so it is
// the fallback only.
function documentEtag(html, answer) {
  const tag = /<html\b[^>]*?\sdocumentetag\s*=\s*"([^"]*)"/i.exec(html);
  if (tag) return tag[1].replace(/&quot;/g, '"').replace(/&lt;/g, "<").replace(/&amp;/g, "&");
  return answer.headers.get("etag");
}

/**
 * @param {{ documentUrl?: string, fetch?: typeof fetch,
 *   page?: ((inner: string, base: string) => string | Promise<string>) | null, onFlush?: () => unknown }} options
 * `page` builds the bytes a save sends from the region's new inner bytes and the page they build
 * on; without it the region is spliced into the page as the host served it.
 */
export function browserTransport({ documentUrl = location.href.split("#")[0], fetch: send = (...args) => window.fetch(...args), page = null, onFlush = () => {}, onSaved = () => {}, token = saveToken } = {}) {
  const pages = new Map();
  const identity = { "Document-URL": documentUrl, "Page-URL": documentUrl };
  const credentials = () => (token() ? "omit" : "same-origin");

  function keep(etag, html) {
    pages.delete(etag);
    pages.set(etag, html);
    while (pages.size > PAGES_KEPT) pages.delete(pages.keys().next().value);
  }

  async function readPage() {
    const answer = await send(documentUrl, { cache: "no-store", credentials: credentials(), headers: { Accept: "text/html" } });
    if (!answer.ok) return { answer };
    const html = await answer.text();
    const etag = documentEtag(html, answer);
    if (etag) keep(etag, html);
    return { html, etag };
  }

  async function file() {
    const read = await readPage();
    if (read.answer) return read.answer;
    const region = findRegion(read.html);
    if (!region || !read.etag) return json(422, { error: "the page has no live room region or no document ETag" });
    return new Response(region.inner, { status: 200, headers: { etag: read.etag } });
  }

  // A save builds on the page read with its If-Match. When that page is no longer kept, the page is
  // read again: the same ETag means the same page; another one means the file moved (412).
  async function save(ifMatch, inner) {
    let base = pages.get(ifMatch);
    if (base === undefined) {
      const read = await readPage();
      if (read.answer) return read.answer;
      if (read.etag !== ifMatch) return json(412, { etag: read.etag });
      base = read.html;
    }
    const served = page ? base : savedPage(base);
    const region = findRegion(served);
    if (!region) return json(422, { error: "the page has no live room region" });
    const html = page ? await page(inner, base) : spliceRegion(served, region, inner);
    const saveAs = token();
    const answer = await send(onOrigin(saveAs ? `/_/save/${saveAs}` : "/_/save"), {
      method: "POST",
      credentials: credentials(),
      headers: { ...identity, "If-Match": ifMatch, "Save-Trigger": "user", "Content-Type": "text/plain;charset=UTF-8" },
      body: html,
    });
    let data = null;
    try {
      data = await answer.clone().json();
    } catch {}
    const headerEtag = answer.headers.get("etag");
    const strongHeader = typeof headerEtag === "string" && !/^W\//i.test(headerEtag.trim()) ? headerEtag : null;
    const etag = typeof data?.etag === "string" ? data.etag : strongHeader;
    // A 200 that is not JSON or names no saved version (a login page after a redirect, Express's
    // automatic weak ETag on HTML, a captive portal, a body cut short) proves nothing was written:
    // the client hears it as an answer it never got.
    if (answer.ok && (!data || typeof data !== "object" || !etag)) return json(502, { error: "the host's answer named no saved version" });
    if (answer.ok && etag) {
      keep(etag, html);
      // The page's own save (clay.save) builds on the file this save wrote, not on the one before
      // it, or the person's next save outside the sheet is refused as a conflict with themselves.
      onSaved(etag);
    }
    return json(answer.status, { ...(data && typeof data === "object" ? data : {}), etag });
  }

  // A host that mints save tokens (HTML Clay) names the document by the token in the path, right
  // after the route's name, as it does for its other per-document routes: /_/copies/<token>/<id>.
  async function forward(target, init, method) {
    target.searchParams.delete("page");
    const saveAs = token();
    if (saveAs && /^\/_\/copies(\/|$)/.test(target.pathname)) target.pathname = target.pathname.replace(/^\/_\/copies/, `/_/copies/${encodeURIComponent(saveAs)}`);
    const ifMatch = init.headers?.["if-match"];
    const answer = await send(onOrigin(`${target.pathname}${target.search}`), {
      method,
      credentials: credentials(),
      headers: { ...identity, ...(ifMatch ? { "If-Match": ifMatch } : {}), ...(init.body !== undefined ? { "Content-Type": init.headers?.["content-type"] ?? "text/plain;charset=UTF-8" } : {}) },
      body: init.body,
    });
    if (answer.ok && method === "POST" && target.pathname === "/_/sync/room" && target.searchParams.get("op") === "establish") {
      try {
        const body = await answer.clone().json();
        const etag = typeof body?.etag === "string" ? body.etag : answer.headers.get("etag");
        if (etag) onSaved(etag);
      } catch {}
    }
    return answer;
  }

  async function fetch(path, init = {}) {
    const target = new URL(path, "http://room.local");
    const method = (init.method ?? "GET").toUpperCase();
    if (target.pathname === "/_/file") return file();
    if (target.pathname === "/_/save") return save(init.headers?.["if-match"], typeof init.body === "string" ? init.body : String(init.body ?? ""));
    return forward(target, init, method);
  }

  // A stream that opens again may have missed events while it was down: the tab reads afresh.
  function subscribe(_page, _clientId, listener) {
    const offFlush = onHostFlush(onFlush);
    const offStream = onStream({
      open(info) {
        if (info.lane === "live") listener({ type: "file", etag: null });
      },
      event(type, data) {
        if (type === "room" && data && typeof data === "object") listener({ type: "room", ...data });
      },
      message(data) {
        if (fileChanged(data)) listener({ type: "file", etag: typeof data.etag === "string" ? data.etag : null });
      },
    });
    return () => {
      offFlush();
      offStream();
    };
  }

  return { fetch, subscribe };
}

// The room routes a frame `to` one client by the id its stream carries, so the room client uses
// that id. A page whose stream does not open in time (file-only hosts route nothing) mints one.
// A room host lets go of a client whose stream it does not hold, so there the tab waits for the
// stream however long it takes; only a file-only host, which routes nothing, gets a minted id.
// A room closed before the stream opens stops waiting (`stopped`), and answers null.
function streamClientId(ms, stopped) {
  const open = currentStream();
  if (open) return Promise.resolve(open.clientId);
  return new Promise((resolve) => {
    let off = () => {};
    stopped.then(() => {
      clearTimeout(timer);
      off();
      resolve(null);
    });
    const timer = ms === Infinity ? null : setTimeout(() => {
      off();
      resolve(`r${Math.random().toString(36).slice(2, 11)}${Date.now().toString(36)}`);
    }, ms);
    off = onStream({
      open(info) {
        clearTimeout(timer);
        off();
        resolve(info.clientId);
      },
    });
  });
}

/**
 * clay.liveRoom(name, { codec, page }): the ordered room of this document (plan 09 section 1.8).
 * The mode follows what the host announces and is fixed for the page load: `ordered` on a host
 * with a room, `file` on a host with the file contract, `unavailable` on any other host (edits
 * then stay in the tab).
 */
// ClayJS's page ETag (core/etag.js), loaded when first needed: it reads the page as it loads.
const recordPageEtag = (etag) => import("../core/etag.js").then((module) => module.recordEtag(etag));

export function liveRoom(name, { codec, page = null, documentUrl, fetch, discover = () => import("../core/host-meta.js").then((meta) => meta.hostMeta()), onSaved = recordPageEtag, ...timings } = {}) {
  if (!codec || typeof codec.apply !== "function" || typeof codec.read !== "function") throw new TypeError("clay.liveRoom: a room codec is required (read, write, apply, ...)");
  let inner = null;
  let mode = "connecting";
  let stop;
  const stopped = new Promise((resolve) => (stop = resolve));
  const listeners = new Map();
  const transport = browserTransport({
    documentUrl,
    fetch,
    page,
    onSaved,
    // A host about to quit asks every open sheet to save what it holds; live sync then closes
    // the stream (stream-bridge.js hostFlush).
    onFlush: () => inner?.flush(),
  });
  const ready = (async () => {
    const extensions = (await discover())?.extensions ?? [];
    const clientId = await streamClientId(extensions.includes("ordered-room") ? Infinity : STREAM_WAIT_MS, stopped);
    if (clientId === null) {
      mode = "closed";
      return;
    }
    inner = await connect({ page: name, codec, clientId, fetch: transport.fetch, subscribe: transport.subscribe, extensions, ...timings });
    if (!inner) {
      mode = "unavailable";
      return;
    }
    for (const [listener, held] of listeners) held.off = inner.subscribe(listener);
    await inner.ready;
    mode = inner.status.mode;
  })();

  const need = () => {
    if (!inner) throw new Error(mode === "unavailable" ? "clay.liveRoom: this host has no live room or file contract; edits stay in the tab" : "clay.liveRoom: wait for room.ready before submitting an edit");
    return inner;
  };

  return {
    ready,
    get status() {
      return inner ? inner.status : { mode, connected: false, pending: 0, unsaved: 0, unknownSave: false, generation: null, through: null, copies: 0, unstored: 0, error: null };
    },
    submit: (action) => need().submit(action),
    flush: () => (inner ? inner.flush() : Promise.resolve({})),
    view: () => inner?.view() ?? null,
    committed: () => inner?.committed?.() ?? null,
    notices: () => inner?.notices() ?? [],
    copies: () => inner?.copies() ?? [],
    restore: (id) => need().restore(id),
    decide: (job, picks) => need().decide(job, picks),
    subscribe(listener) {
      const held = { off: inner?.subscribe(listener) };
      listeners.set(listener, held);
      return () => {
        listeners.delete(listener);
        held.off?.();
      };
    },
    close: async () => {
      stop();
      await ready.catch(() => {});
      await inner?.close();
    },
  };
}
