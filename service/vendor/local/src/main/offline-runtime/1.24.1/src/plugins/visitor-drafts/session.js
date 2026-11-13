/**
 * One tab's side of the draft database.
 *
 * The store (`store.js`) turns each save into an operation and keeps it in this tab's
 * journal slot. This module delivers them: `sync()` runs one `transact()` (`commit.js`)
 * carrying this tab's unconfirmed operations and every journal slot `localStorage` holds,
 * a closed tab's included, and `took()` acts on what that transaction stored. An operation
 * is saved once its transaction completed; a failed one stays where it is and goes again
 * with the next `sync()`.
 *
 * After each transaction `localStorage` gets a cache of the record: the record itself with
 * a `w` field `{rev, reset}` while it is at most FULL_COPY_LIMIT characters, beyond that a
 * head (`{v: 2, idb: true, data: the small keys}`). A page boots from the cache, the
 * first-paint look and the download page read it, and a tab on a released ClayJS (1.24.0
 * and earlier) takes it for the record. It is never read back as the record here.
 *
 * Such a tab writes its own record over the cache. That record has no `w`, so the next
 * transaction takes it in (`commit.js`), and the cache is never written over one that has
 * not been taken in.
 */

import { openDrafts } from "./idb.js";
import { emptyMeta, foreignMark, isForeign, markerFor, markerGen, recoveryKey, recoveryMark, rowKey, settled, transact } from "./commit.js";
import { setDraftStoreFailed } from "../../core/draft-mode.js";

export const FULL_COPY_LIMIT = 256 * 1024;
const HEAD_FIELD_LIMIT = 2 * 1024;
const HEAD_KEYS = new Set(["v", "savedAt", "id", "ancestry", "reset", "data"]);
// A settled journal slot younger than this may belong to a tab that is still writing it.
const SLOT_AGE_MS = 10 * 1000;
// Start over reloads the page once its transaction completed, or after this long: the
// marker in `localStorage` then carries it to the next page load's first transaction.
const START_OVER_WAIT_MS = 1500;
const FIRST_TRANSACTION_TIMEOUT_MS = 2000;
const STARTED_OVER = "This draft was started over in another tab.";
const RESET_KEPT_ASIDE = "This draft was started over in another tab. The earlier draft was kept aside in this browser.";
const KEPT_ASIDE = "A tab that was open before an update saved another copy of this draft. That copy was kept aside, and this draft was not changed. Reload that tab to go on from this draft.";

const parse = (text) => {
  if (text == null) return null;
  try { return JSON.parse(text); } catch { return null; }
};

// Only data fits in a head: a page (`live`, `blockeditor`) or any field a later version adds
// is held only in the database, so a head of such a record never says it is `full`.
function headOf(record, w) {
  const data = {};
  let full = Object.keys(record).every((name) => HEAD_KEYS.has(name));
  for (const [name, value] of Object.entries(record.data)) {
    if (JSON.stringify(value).length < HEAD_FIELD_LIMIT) data[name] = value;
    else full = false;
  }
  return JSON.stringify({ v: 2, savedAt: record.savedAt, id: record.id, ancestry: record.ancestry, idb: true, ...(full && { full: true }), data, w });
}

/**
 * Open the database for this tab and run its first transaction, which takes in what closed
 * tabs left in their journal slots. Rejects when the database does not open.
 *
 * @param {Object} options
 * @param {Object} options.store - the tab's store (`createStore`)
 * @param {?Storage} options.storage
 */
export async function openSession({ store, storage, doc = document, now = () => Date.now(), reload = () => window.location.reload() }) {
  const drafts = await openDrafts();
  const { key } = store;
  const resetKey = key + ":reset";
  const read = (name) => { try { return storage?.getItem(name) ?? null; } catch { return null; } };
  const remove = (name) => { try { storage?.removeItem(name); } catch {} };

  // A page that loaded again because another tab started over says so once, and keeps
  // saying it until it is closed: the reload happened while the visitor was in the other
  // tab. The flag is per tab and outlives only that reload.
  const noticeKey = key + ":started-over";
  const recoveryNoticeKey = key + ":recovery-notice";
  try {
    if (window.sessionStorage.getItem(noticeKey)) {
      window.sessionStorage.removeItem(noticeKey);
      if (!window.sessionStorage.getItem(recoveryNoticeKey)) import("../../ui/toast.js").then((m) => m.toastPersistent(STARTED_OVER, "info")).catch(() => {});
    }
  } catch {}

  // The stored revision this page last matched, and the marker the transaction in flight read.
  let rev = null;
  let marker = "";
  // The number of the generation this page shows, once a transaction has said it.
  let gen = null;
  // This tab's own Start over: the generation it ends, until a transaction has carried it
  // (`sent` is what the transaction in flight carries).
  let ending = null;
  let sent = null;
  let carried = 0;
  let retiring = false;
  let shownId = store.held()?.id;
  // The latest full cache copy this tab saw at the key, and the text of another writer's
  // record a storage event says replaced it: such a record merges from that copy.
  let lastCache = null;
  let replacedBy = null;
  const told = new Set();
  try {
    const notice = window.sessionStorage.getItem(recoveryNoticeKey);
    if (notice) {
      window.sessionStorage.removeItem(recoveryNoticeKey);
      const saved = parse(notice);
      keptAside({ id: saved?.mark ?? notice }, Boolean(saved?.reset));
    }
  } catch {}

  function rememberRecovery(mark) {
    try { window.sessionStorage.setItem(recoveryNoticeKey, JSON.stringify({ mark, reset: true })); } catch {}
  }

  function rememberGeneration(reset) {
    try { window.sessionStorage.setItem(key + ":generation", JSON.stringify({ marker, reset })); } catch {}
  }

  function input() {
    const held = read(resetKey);
    marker = held ?? "";
    const others = store.journalSlots().filter((name) => name !== store.slotKey).map((name) => parse(read(name)));
    sent = store.stopped() ? null : ending;
    carried = store.chain().ops.at(-1)?.seq ?? 0;
    const local = store.held();
    if (local?.v === 1 && local.w !== undefined) lastCache = local;
    const resets = sent ? [sent] : [];
    return {
      seed: store.seed(),
      newId: store.newId,
      merge3: store.merge3,
      unitMark: store.unitMark,
      now: now(),
      marker: held,
      resets,
      local,
      seen: lastCache,
      direct: replacedBy !== null && read(key) === replacedBy,
      chains: store.pending() ? [...others, store.chain()] : others,
    };
  }

  // Another tab's slot is removed once nothing more can come of it and its tab has gone quiet.
  function sweep(meta) {
    const at = now();
    for (const name of store.journalSlots()) {
      if (name === store.slotKey) continue;
      const chain = parse(read(name));
      if (settled(chain, meta) && at - (chain?.at ?? 0) > SLOT_AGE_MS) remove(name);
    }
  }

  function cache({ meta, record }) {
    const held = store.held();
    // Another writer's record stays until the database has taken it in: one written since
    // this transaction read `localStorage` goes with the next.
    if (isForeign(held) && foreignMark(held) !== meta.foreign) return void setTimeout(quietly, 0);
    if ((read(resetKey) ?? "") !== meta.reset) return;
    if (!record) return void (held && remove(key));
    if (held?.w && held.w.reset === meta.reset && held.w.rev >= meta.rev) return;
    const w = { rev: meta.rev, reset: meta.reset };
    const full = JSON.stringify({ ...record, w });
    try {
      if (full.length <= FULL_COPY_LIMIT) {
        storage.setItem(key, full);
        lastCache = { ...record, w };
        return;
      }
    } catch {}
    // No room for the full copy: a head still says a draft exists.
    try { storage.setItem(key, headOf(record, w)); } catch {}
  }

  // Operations captured after the transaction read stay journaled for the next load.
  async function ended(result) {
    retiring = true;
    const chain = store.chain();
    const mark = recoveryMark(chain);
    const kept = chain.ops.some((op) => op.seq > carried) || result.meta.recovery?.includes(mark)
      ? mark : result.meta.recovery?.includes(shownId) ? shownId : null;
    store.acknowledge(carried);
    store.stop({ keepPending: true });
    if (store.unsafe()) {
      try { await drafts.putRow(rowKey(key, store.tab), JSON.stringify(store.chain())); }
      catch (error) { throw store.failed(error); }
      store.acknowledge(store.chain().ops.at(-1)?.seq ?? 0);
    }
    cache(result);
    if (kept) rememberRecovery(kept);
    const held = store.held();
    if (held?.w && held.w.reset !== result.meta.reset) remove(key);
    try { window.sessionStorage.setItem(noticeKey, "1"); } catch {}
    rememberGeneration(result.meta.reset);
    retiring = false;
    reload();
    return result;
  }

  // Start over ends the generation this page shows, and only that one: when another Start
  // over has ended it since this page loaded, what was saved after that one stays. A page
  // that does not know its generation's number yet waits for the first transaction.
  async function startOver() {
    const shown = store.generation();
    const stop = () => { store.stop(); setDraftStoreFailed(false); };
    const stale = () => (read(resetKey) ?? "") !== shown && (gen === null || (read(resetKey) ?? "") !== marker);
    if (stale()) return stop();
    let known = gen ?? markerGen(shown);
    if (known === null) {
      const result = await first.catch(() => drafts.change(key, ({ meta }) => ({
        puts: [],
        deletes: [],
        result: { meta: parse(meta) ?? emptyMeta(shown) },
      }), { timeout: FIRST_TRANSACTION_TIMEOUT_MS }));
      if (stale()) return stop();
      known = result.meta.gen;
      if (result.meta.reset !== shown) {
        ending = { ends: null, marker: shown };
        const applied = await sync();
        if (store.stopped()) return stop();
        known = applied.meta.gen;
      }
    }
    const next = markerFor(known + 1, store.newId());
    ending = { ends: known, marker: next };
    const before = read(key);
    if (store.held()?.w) remove(key);
    try { storage.setItem(resetKey, next); } catch {}
    let timer;
    try {
      const committed = sync();
      if (read(resetKey) !== next) await committed;
      else await Promise.race([committed.catch(() => {}), new Promise((resolve) => { timer = setTimeout(resolve, START_OVER_WAIT_MS); })]);
      stop();
    } catch (error) {
      ending = null;
      if (before !== null && read(key) === null && (read(resetKey) ?? "") === shown) {
        try { storage.setItem(key, before); } catch {}
      }
      throw error;
    } finally { clearTimeout(timer); }
  }

  // Each kind of recovery notice is shown once per page load.
  function keptAside(copy, reset = false) {
    console.warn(reset
      ? "visitor-drafts: the draft was started over; the earlier draft was kept in this browser's draft database under"
      : "visitor-drafts: a tab on an earlier ClayJS saved a copy that could not be merged exactly; it was kept in this browser's draft database under", recoveryKey(key, recoveryMark(copy)));
    if (told.has(reset)) return;
    told.add(reset);
    import("../../ui/toast.js").then((m) => m.toastPersistent(reset ? RESET_KEPT_ASIDE : KEPT_ASIDE, "warning")).catch(() => {});
  }

  function took(result) {
    const { meta, record } = result;
    // A marker the transaction did not take named a generation that had already ended.
    if (marker !== meta.reset && (read(resetKey) ?? "") === marker) {
      try { storage.setItem(resetKey, meta.reset); } catch {}
    }
    if (sent && sent === ending && meta.reset !== store.generation()) {
      if (sent.marker === meta.reset) rememberGeneration(meta.reset);
      store.stop();
      setDraftStoreFailed(false);
    }
    if (sent === ending) ending = null;
    // A stopped page is on its way out, after its own Start over or another tab's: it only
    // keeps the cache in step, for the page load that follows.
    if (store.stopped()) {
      if (retiring) return ended(result);
      cache(result);
      return result;
    }
    if (result.conflicts.length) console.warn("visitor-drafts: another tab changed the same thing; its copy was kept in this browser's draft database under", key + ":conflict");
    if (result.stale.includes(store.tab)) return ended(result);
    if (meta.reset !== store.generation()) {
      if (!store.fresh()) return ended(result);
      store.enter(meta.reset);
    }
    if (result.released.length) keptAside(result.released[0]);
    gen = meta.gen;
    rememberGeneration(meta.reset);
    const saved = store.acknowledge(meta.tabs[store.tab]?.seq ?? 0);
    sweep(meta);
    cache(result);
    if (store.pending()) return result;
    if (saved) {
      setDraftStoreFailed(false);
      doc.dispatchEvent(new CustomEvent("clay:drafts-saved", { detail: { data: record.data } }));
    }
    if (!store.shown()) {
      rev = meta.rev;
      return result;
    }
    // A page with unsaved changes is never rewritten: its next operation merges instead.
    if (!store.changed()) {
      if (record && meta.rev !== rev) store.take(record.data);
      shownId = record?.id;
      rev = meta.rev;
    }
    const unit = store.externalUnit(record);
    if (unit) doc.dispatchEvent(new CustomEvent("clay:drafts-blockeditor-external", { detail: { live: unit.live, adopted: () => store.adoptedUnit(unit) } }));
    return result;
  }

  // One transaction at a time, and at most one waiting: it reads the chain when it starts,
  // so it carries every operation made before then.
  let active = null;
  let waiting = null;
  function run(options) {
    active = transact(drafts, key, input, options).then(took, (error) => {
      throw store.pending() ? store.failed(error) : error;
    }).finally(() => { active = null; });
    return active;
  }
  function sync() {
    if (waiting) return waiting;
    if (!active) return run();
    waiting = active.then(() => {}, () => {}).then(() => {
      waiting = null;
      return run();
    });
    return waiting;
  }
  const quietly = () => { sync().catch(() => {}); };

  // At a hide, an operation too large for the journal slot goes into the database blind, with
  // no read and no callback, so it can land while the page is going away.
  function hide() {
    if (store.unsafe()) drafts.putRow(rowKey(key, store.tab), JSON.stringify(store.chain())).catch(() => {});
    if (store.pending()) quietly();
  }

  window.addEventListener("storage", (event) => {
    if (event.key === key) {
      const [was, is] = [parse(event.oldValue), parse(event.newValue)];
      if (is?.v === 1 && is.w !== undefined) lastCache = is;
      else if (isForeign(is) && was?.v === 1 && was.w !== undefined) [lastCache, replacedBy] = [was, event.newValue];
    }
    if (event.key === null || event.key === key || event.key === resetKey) quietly();
  });
  window.addEventListener("pageshow", (event) => { if (event.persisted) quietly(); });
  // The page editor missed a page another tab stored before it listened.
  doc.addEventListener("clay:drafts-blockeditor-listening", quietly);

  const first = run({ timeout: FIRST_TRANSACTION_TIMEOUT_MS });
  first.catch(() => {});
  return { first, sync, hide, startOver };
}
