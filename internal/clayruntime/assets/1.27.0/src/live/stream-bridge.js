// Live sync owns the document's stream. clay.live's fast lane and clay.liveRoom need to
// know when that stream opens (and as which client), when it drops, and some of its named
// events; clay.liveRoom also hears its unnamed frames (file pushes) and a host's flush. This
// module carries them across, so neither side imports the other and a page without either
// plugin pays for one small module.

export const LIVE_EVENTS = Object.freeze(["data", "data-state", "data-writer", "room"]);

const handlers = new Set();
let current = null;

function each(method, ...args) {
  for (const handler of handlers) {
    try {
      handler[method]?.(...args);
    } catch (err) {
      console.error("clay.live: stream handler failed:", err);
    }
  }
}

/** Live sync: the stream is open as this client on this lane. Every (re)open. */
export function streamOpened({ clientId, lane }) {
  current = { clientId, lane };
  each("open", current);
}

/** Live sync: the stream dropped or stopped. */
export function streamClosed() {
  if (!current) return;
  current = null;
  each("close");
}

/** Live sync: one of LIVE_EVENTS arrived, its data already parsed. */
export function streamEvent(type, data) {
  if (!LIVE_EVENTS.includes(type)) return;
  each("event", type, data);
}

/** Live sync: an unnamed frame arrived (after its duplicate check), its data already parsed. */
export function streamMessage(data) {
  each("message", data);
}

/**
 * Listen: { open(info), close(), event(type, data), message(data) }, each optional. A stream
 * that is already open is reported to the new handler at once. Returns an
 * unsubscribe function.
 */
export function onStream(handler) {
  handlers.add(handler);
  if (current) {
    try {
      handler.open?.(current);
    } catch (err) {
      console.error("clay.live: stream handler failed:", err);
    }
  }
  return () => handlers.delete(handler);
}

const flushers = new Set();

/**
 * A live room: save what it holds when the host asks (HTML Clay's `{"type":"flush"}` before it
 * quits or restarts for an update). `flush` returns a promise. Returns an unsubscribe function.
 */
export function onHostFlush(flush) {
  flushers.add(flush);
  return () => flushers.delete(flush);
}

/**
 * Live sync: the host asked to flush. Resolves once every live room on the page saved what it held
 * (or failed to), so the stream can close; at once when the page has none.
 */
export function hostFlush(data) {
  return Promise.allSettled([...flushers].map((flush) => flush(data)));
}

/** The open stream's { clientId, lane }, or null. */
export function currentStream() {
  return current;
}
