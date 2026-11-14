/**
 * The IndexedDB half of the visitor draft store.
 *
 * One database per origin (`clay-visitor-drafts`), two stores:
 *
 *   records  key: the draft's text key (`clay:visitor-drafts:<slot>@<path>`) and its
 *            siblings `<key>:<name>`; value: JSON text
 *   parts    key: `[recordKey, regionKey, partId]`; value: `{ rev, text }`
 *
 * Every change to a draft goes through `change()`, one readwrite transaction that reads,
 * merges (the caller's `build`) and writes; see `commit.js`. The only other write is
 * `putRow()`, a tab's own chain left blind at a hide. A transaction resolves only once it
 * completes. Every failure rejects with an Error whose `code` says what happened
 * (`unavailable`, `blocked`, `quota`, `aborted`, `unknown-schema`, `closed`, `bad-op`,
 * `error`).
 */

import { codeFor } from "../../lib/idb-error-code.js";

export const DB_NAME = "clay-visitor-drafts";
export const DB_VERSION = 1;
export const RECORDS = "records";
export const PARTS = "parts";
const OPEN_TIMEOUT_MS = 2000;

function failure(code, cause) {
  const error = new Error(`visitor-drafts: the draft database failed (${code})`);
  error.code = code;
  if (cause !== undefined) error.cause = cause;
  return error;
}

function cursorEntries(source, range) {
  return new Promise((resolve, reject) => {
    const out = [];
    const req = source.openCursor(range);
    req.onerror = () => reject(req.error);
    req.onsuccess = () => {
      const cursor = req.result;
      if (!cursor) return resolve(out);
      out.push([cursor.key, cursor.value]);
      cursor.continue();
    };
  });
}

/**
 * Open (and, the first time, create) the draft database.
 *
 * A database that does not open within `timeout` milliseconds (another tab holds an
 * upgrade, or the browser never answers) rejects with `blocked`, so the caller can
 * refuse to write over a draft it could not read rather than wait forever.
 *
 * @param {Object} [options]
 * @param {?IDBFactory} [options.factory]
 * @param {?typeof IDBKeyRange} [options.keyRange]
 * @returns {Promise<{readAll: Function, readParts: Function, change: Function, putRow: Function, close: Function}>}
 */
export function openDrafts({ factory = globalThis.indexedDB, keyRange = globalThis.IDBKeyRange, name = DB_NAME, timeout = OPEN_TIMEOUT_MS } = {}) {
  return new Promise((resolve, reject) => {
    if (!factory || typeof factory.open !== "function" || !keyRange) return reject(failure("unavailable"));
    let req;
    try {
      req = factory.open(name, DB_VERSION);
    } catch (error) {
      return reject(failure(codeFor(error), error));
    }
    let settled = false;
    const finish = (fn, value) => {
      if (settled) return false;
      settled = true;
      clearTimeout(timer);
      fn(value);
      return true;
    };
    const timer = setTimeout(() => finish(reject, failure("blocked")), timeout);
    req.onupgradeneeded = () => {
      const db = req.result;
      if (!db.objectStoreNames.contains(RECORDS)) db.createObjectStore(RECORDS);
      if (!db.objectStoreNames.contains(PARTS)) db.createObjectStore(PARTS);
    };
    req.onsuccess = () => {
      const db = req.result;
      // An open that answered after the timeout is closed again, or it would hold the
      // database against another tab's upgrade for as long as this page stays open.
      if (!finish(resolve, wrap(db, keyRange))) db.close();
    };
    req.onerror = () => finish(reject, failure(codeFor(req.error), req.error));
    req.onblocked = () => finish(reject, failure("blocked"));
  });
}

function wrap(db, keyRange) {
  let closed = false;
  // Another tab opening a newer version needs this connection to stand aside.
  db.onversionchange = () => {
    closed = true;
    try { db.close(); } catch {}
  };

  function transaction(stores, mode) {
    if (closed) throw failure("closed");
    try {
      return db.transaction(stores, mode);
    } catch (error) {
      throw failure(codeFor(error), error);
    }
  }

  // The record named `recordKey` and its siblings (`<recordKey>:<anything>`), as a Map
  // from key to stored text. A key that only starts with `recordKey`, such as another
  // document whose path extends this one's, is not a sibling.
  async function readAll(recordKey) {
    const tx = transaction(RECORDS, "readonly");
    const range = keyRange.bound(recordKey, recordKey + ":￿");
    try {
      const entries = await cursorEntries(tx.objectStore(RECORDS), range);
      return new Map(entries.filter(([key]) => key === recordKey || key.startsWith(recordKey + ":")));
    } catch (error) {
      throw failure(codeFor(error), error);
    }
  }

  // The parts of one region of one record, as a Map from part id to `{ rev, text }`.
  // An array sorts after every string, so `[recordKey, regionKey, []]` bounds them all.
  async function readParts(recordKey, regionKey) {
    const tx = transaction(PARTS, "readonly");
    const range = keyRange.bound([recordKey, regionKey], [recordKey, regionKey, []]);
    try {
      const entries = await cursorEntries(tx.objectStore(PARTS), range);
      return new Map(entries.map(([key, value]) => [key[2], value]));
    } catch (error) {
      throw failure(codeFor(error), error);
    }
  }

  // The one read-merge-write. Reads the record, `:meta`, `:v1base` and every `:op:` row of
  // `key`, hands them to `build` inside the transaction, and writes the `puts` and
  // `deletes` it returns before the transaction can complete. A `build` or a write that
  // throws aborts it, so nothing lands. Resolves with `build`'s `result` on `complete`.
  function change(key, build, { timeout = 0 } = {}) {
    return new Promise((resolve, reject) => {
      let tx;
      try {
        tx = transaction([RECORDS, PARTS], "readwrite");
      } catch (error) {
        return reject(error);
      }
      let thrown = null;
      let result;
      let settled = false;
      let timer;
      const finish = (fn, value) => {
        if (settled) return;
        settled = true;
        clearTimeout(timer);
        fn(value);
      };
      if (timeout > 0) timer = setTimeout(() => {
        thrown = failure("blocked");
        try { tx.abort(); } catch {}
        finish(reject, thrown);
      }, timeout);
      tx.oncomplete = () => finish(resolve, result);
      tx.onabort = () => finish(reject, thrown || failure(tx.error ? codeFor(tx.error) : "aborted", tx.error));
      try {
        const store = tx.objectStore(RECORDS);
        const rows = keyRange.bound(key + ":op:", key + ":op:\uffff");
        const reads = { record: store.get(key), meta: store.get(key + ":meta"), v1base: store.get(key + ":v1base"), names: store.getAllKeys(rows), values: store.getAll(rows) };
        // Requests in one transaction answer in the order they were made: the last one in
        // means every read is.
        reads.values.onsuccess = () => {
          if (settled) return;
          try {
            const raw = {
              record: reads.record.result ?? null,
              meta: reads.meta.result ?? null,
              v1base: reads.v1base.result ?? null,
              rows: reads.names.result.map((name, i) => [name, reads.values.result[i]]),
            };
            const planned = build(raw);
            for (const [name, value] of planned.puts) store.put(value, name);
            for (const name of planned.deletes) store.delete(name);
            result = planned.result;
          } catch (error) {
            thrown = failure(codeFor(error), error);
            try { tx.abort(); } catch {}
          }
        };
      } catch (error) {
        thrown = failure(codeFor(error), error);
        try { tx.abort(); } catch {}
      }
    });
  }

  // A tab's own unconfirmed chain, written blind (no read) at a hide, with an explicit
  // commit so it needs no callback in a page about to go away. `change()` applies it later.
  function putRow(name, value) {
    return new Promise((resolve, reject) => {
      if (!/:op:[^:]+$/.test(name)) return reject(failure("bad-op"));
      let tx;
      try {
        tx = transaction(RECORDS, "readwrite");
        tx.objectStore(RECORDS).put(value, name);
        tx.commit?.();
      } catch (error) {
        return reject(error instanceof Error && error.code ? error : failure(codeFor(error), error));
      }
      tx.oncomplete = () => resolve();
      tx.onabort = () => reject(failure(tx.error ? codeFor(tx.error) : "aborted", tx.error));
    });
  }

  function close() {
    closed = true;
    try { db.close(); } catch {}
  }

  return { readAll, readParts, change, putRow, close };
}
