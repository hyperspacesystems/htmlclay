/**
 * The one change a visitor draft's stored state ever takes.
 *
 * IndexedDB holds one record per draft key K, and only `transact()` changes it: one
 * readwrite transaction reads what is stored, runs `commit()` over it, and writes the
 * result. IndexedDB runs readwrite transactions on a store one after another across every
 * tab of the origin, so this is the single writer, with no lock and no leader tab.
 *
 * What it applies:
 *
 *   - operations: for each top-level key a tab changed, the value before (`b`) and after
 *     (`m`), a missing one meaning the key was absent. Each carries its own merge base, so
 *     the store's `merge3` lays it over whatever is stored. A tab's operations are a
 *     chain `{tab, reset, acked, ops: [{seq, savedAt, keys, unit?}]}`, where `reset` is
 *     the Start over marker of the generation the tab shows (a tab knows that before its
 *     first transaction; it does not know the number). An operation applies only when
 *     its chain's `reset` is the stored one and its `seq` is the next for its tab, so
 *     each applies exactly once and in order, whichever tab delivers it;
 *   - Start over: "end generation g", given explicitly or as a marker `<g+1>~<random>`
 *     in `localStorage`; an unverified marker starts empty and keeps recovery copies;
 *   - the move: the full version 1 record in `localStorage`, taken in as the first record
 *     when nothing is stored yet;
 *   - another writer's record: a version 1 record in `localStorage` without the cache's
 *     `w` field, written by a tab on a released ClayJS or by a page load of this one that
 *     had no database. It is taken in once: merged when the copy it was built on is held
 *     here, kept whole as a recovery copy when it is not. Nothing is merged on a guess.
 *
 * Stored rows (JSON text): K (the record, today's format), `K:meta`
 * `{gen, rev, reset, ended?, tabs: {tab: {seq, at}}, foreign, unit, recovery?}`, `K:v1base` (the
 * last record of another writer that was merged), `K:op:<tab>` (a chain a tab left at a
 * hide), `K:conflict:<id>` with `K:conflict` naming the latest, and `K:recovery:<mark>`.
 * `meta.unit` is the mark of the Block editor page the record holds (`unitMark`). A record
 * id or ancestry identifies another writer's held base or the reset its copy follows.
 * Operations of this ClayJS never read one. Recovery rows can also hold exact ended chains.
 *
 * This module imports nothing: the page fetches it with the database, after the boot, and
 * the store that is already on the page hands it `merge3` and `unitMark`.
 */

export const LINEAGE = 32;
// Recovery copies kept per draft, newest first.
export const RECOVERY_KEEP = 8;
// A tab not heard from in this long is forgotten; a chain it still sends carries `acked`.
export const TAB_TTL_MS = 30 * 24 * 60 * 60 * 1000;

const clone = (value) => (value === undefined ? undefined : JSON.parse(JSON.stringify(value)));
const isPlain = (value) => value !== null && typeof value === "object" && !Array.isArray(value);

export const metaKey = (key) => key + ":meta";
export const rowKey = (key, tab) => `${key}:op:${tab}`;
export const conflictKey = (key, id) => (id === undefined ? key + ":conflict" : `${key}:conflict:${id}`);
export const recoveryKey = (key, mark) => `${key}:recovery:${mark}`;

// A version 1 record without the cache's `w` field is another writer's.
export const isForeign = (value) => isPlain(value) && value.v === 1 && value.w === undefined && isPlain(value.data);
// What names one: its id, or its time for a record from before ids (ClayJS 1.22.0 and earlier).
export const foreignMark = (record) => (typeof record.id === "string" ? record.id : "at:" + record.savedAt);

export const markerFor = (gen, nonce) => `${gen}~${nonce}`;
export function markerGen(marker) {
  const match = /^(\d+)~/.exec(marker ?? "");
  return match ? Number(match[1]) : null;
}

// One operation's keys laid over `data` with the store's three-way merge.
export function applyKeys(data, keys, merge3) {
  const base = { ...data };
  const mine = { ...data };
  for (const [name, pair] of Object.entries(keys)) {
    if ("b" in pair) base[name] = pair.b; else delete base[name];
    if ("m" in pair) mine[name] = pair.m; else delete mine[name];
  }
  return merge3(base, mine, data);
}

export const isChain = (value) => isPlain(value) && typeof value.tab === "string" && typeof value.reset === "string" && Array.isArray(value.ops);
export const recoveryMark = (copy) => isChain(copy) ? `op:${copy.tab}:${copy.ops.at(-1)?.seq ?? copy.acked ?? 0}` : foreignMark(copy);

// Ended journals carry only the fields they changed; other fields use the served page.
export function recoveryRecord(copy, seed) {
  if (isPlain(copy) && copy.v === 1 && isPlain(copy.data)) return clone(copy);
  if (!isChain(copy) || !copy.ops.length) return null;
  const data = clone(seed);
  let unit = null;
  for (const op of copy.ops) {
    for (const [name, pair] of Object.entries(op.keys ?? {})) {
      if ("m" in pair) Object.defineProperty(data, name, { value: clone(pair.m), enumerable: true, writable: true, configurable: true });
      else delete data[name];
    }
    if (op.unit !== undefined) unit = op.unit.to;
  }
  return { v: 1, savedAt: copy.ops.at(-1).savedAt, data, ...(unit && { live: clone(unit.live), blockeditor: clone(unit.blockeditor) }) };
}

// Cleanup needs proof for this exact chain, even when its generation has ended.
export function settled(chain, meta) {
  if (isPlain(chain) && meta.recovery?.includes(recoveryMark(chain))) return true;
  if (!isChain(chain)) return false;
  const at = Math.max(chain.acked ?? 0, chain.reset === meta.reset ? meta.tabs[chain.tab]?.seq ?? 0 : 0);
  return chain.ops.every((op) => op.seq <= at);
}

export function emptyMeta(marker = null) {
  return { gen: markerGen(marker) ?? 1, rev: 0, reset: marker ?? "", tabs: {}, foreign: null, unit: null };
}

const unitOf = (record) => (record && record.blockeditor ? { live: record.live, blockeditor: record.blockeditor } : null);

// The ids a record built on `record` descends from, nearest first.
function lineage(first, record, meta) {
  const line = [...first, record?.id, ...(record?.ancestry ?? [])].filter((id) => typeof id === "string");
  return line.length ? line.slice(0, LINEAGE) : ["root:" + meta.reset];
}

// The copy another writer's record was built on, when this side holds it and what it held
// is in the stored record: the stored record itself, a cache copy of this generation, or
// the last record of another writer that was merged. A record that descends from one of
// them (its ancestry names it, or a storage event says it replaced that cache copy)
// carries every change made since as its difference; the nearest one is the base.
function foreignBase(line, { record, seen, direct, v1base, meta }) {
  const cache = isPlain(seen) && seen.v === 1 && isPlain(seen.data) && seen.w?.reset === meta.reset ? seen : null;
  const at = (held) => (held && typeof held.id === "string" ? line.indexOf(held.id) : -1);
  const held = [[record, at(record)], [cache, cache && direct ? 0 : at(cache)], [v1base, at(v1base)]].filter(([, index]) => index >= 0);
  return held.length ? held.reduce((nearest, next) => (next[1] < nearest[1] ? next : nearest))[0] : null;
}

/**
 * @param {{meta: ?Object, record: ?Object, v1base: ?Object}} stored
 * @param {Object} input
 * @param {Object} input.seed - the data a generation starts from (the served page's)
 * @param {() => string} input.newId
 * @param {number} input.now - milliseconds
 * @param {Function} input.merge3 - the store's three-way merge
 * @param {Function} input.unitMark - the store's mark of a Block editor page
 * @param {?string} [input.marker] - the Start over marker `localStorage` holds now
 * @param {Array<{ends: number, marker: string}>} [input.resets]
 * @param {Array<Object>} [input.chains] - oldest carriers first, the caller's own last
 * @param {?Object} [input.local] - what `localStorage` holds at the draft's key now, parsed
 * @param {?Object} [input.seen] - the latest cache copy the caller saw at the draft's key
 * @param {boolean} [input.direct] - a storage event says `local` replaced exactly that copy
 */
export function commit(stored, { seed, newId, now, merge3, same, unitMark, marker = null, resets = [], chains = [], local = null, seen = null, direct = false, recovery = null, copies = [] }) {
  const meta = stored.meta ? clone(stored.meta) : emptyMeta(marker);
  // A database the browser cleared counts on from this ClayJS's cache, so the cache never
  // reads as newer than the record.
  if (!stored.meta && Number.isInteger(local?.w?.rev)) meta.rev = local.w.rev;
  let record = stored.record ? clone(stored.record) : null;
  let v1base = stored.v1base ? clone(stored.v1base) : null;
  const out = { conflicts: [], recovered: [], released: [], dropped: [], stale: [], applied: {}, changed: !stored.meta };

  function recover(copy, supersedes = []) {
    const mark = recoveryMark(copy);
    if (meta.recovery?.includes(mark)) return;
    out.recovered.push(clone(copy));
    const kept = [mark, ...(meta.recovery ?? []).filter((held) => held !== mark && !supersedes.includes(held))].slice(0, RECOVERY_KEEP);
    out.dropped = [...new Set([...out.dropped, ...(meta.recovery ?? [])])].filter((held) => !kept.includes(held));
    meta.recovery = kept;
    out.changed = true;
    return true;
  }

  const ends = [...resets];
  if (marker && marker !== meta.reset) ends.push({ ends: markerGen(marker) === null ? null : markerGen(marker) - 1, marker });
  for (const reset of ends) {
    if (reset.marker === meta.reset || meta.ended?.includes(reset.marker) || (reset.ends !== null && reset.ends < meta.gen)) continue;
    if (reset.ends !== meta.gen) {
      if (record) recover(record);
    }
    const follows = typeof local?.reset === "string" ? local.reset === reset.marker
      : reset.ends === null || (Array.isArray(local?.ancestry) && local.ancestry.includes("root:" + reset.marker));
    if (isForeign(local) && foreignMark(local) !== meta.foreign && !follows) {
      recover(local);
      meta.foreign = foreignMark(local);
    }
    meta.ended = [...(meta.ended ?? []), meta.reset].slice(-LINEAGE);
    Object.assign(meta, { gen: meta.gen + 1, reset: reset.marker, tabs: {}, unit: null });
    meta.rev++;
    record = null;
    v1base = null;
    out.changed = true;
  }

  // Nothing stored yet: the full copy in `localStorage` is the first record, whether a
  // 1.23.0 tab wrote it (the move) or it is this ClayJS's cache, of this generation, of a
  // database the browser has since cleared.
  if (!stored.meta && !record && isPlain(local) && local.v === 1 && isPlain(local.data) && (local.w === undefined || local.w.reset === meta.reset)) {
    const { w, ...copy } = local;
    record = clone(copy);
    if (w === undefined) {
      v1base = clone(copy);
      meta.foreign = foreignMark(local);
      meta.rev++;
    }
  }
  if (!stored.meta) meta.unit = unitMark(unitOf(record));

  // Another writer's record met after the move, taken in once and before any chain.
  if (stored.meta && isForeign(local) && foreignMark(local) !== meta.foreign) {
    const mark = foreignMark(local);
    const line = Array.isArray(local.ancestry) ? local.ancestry : [];
    const base = record ? foreignBase(line, { record, seen, direct, v1base, meta }) : null;
    if (typeof local.reset === "string" && meta.ended?.includes(local.reset)) {
      if (recover(local, line)) out.released.push(clone(local));
    } else if (!record) {
      // Nothing is stored in this generation: its page is the draft, as it is between two
      // tabs of that ClayJS.
      record = clone(local);
      v1base = clone(local);
      meta.unit = unitMark(unitOf(record));
      meta.rev++;
    } else if (base) {
      const merged = merge3(base.data, local.data, record.data);
      // A page it carries counts as changed only when it differs from its base's.
      const page = unitOf(local);
      const paged = Boolean(page) && unitMark(page) !== unitMark(unitOf(base));
      const pageMoved = paged && unitOf(record) && unitMark(unitOf(base)) !== meta.unit;
      if (merged.conflict || pageMoved) out.conflicts.push(clone(record));
      const unit = paged ? page : unitOf(record);
      record = {
        v: 1,
        savedAt: local.savedAt ?? new Date(now).toISOString(),
        id: newId(),
        ancestry: lineage(typeof local.id === "string" ? [local.id] : [], record, meta),
        data: merged.value,
        ...(unit && { live: unit.live, blockeditor: unit.blockeditor }),
      };
      if (paged) meta.unit = unitMark(unit);
      v1base = clone(local);
      meta.rev++;
    } else {
      // The copy it was built on is not held here: it is kept whole, beside the draft. A
      // later copy of the same line replaces the one it descends from.
      if (recover(local, line)) out.released.push(clone(local));
    }
    meta.foreign = mark;
    out.changed = true;
  }

  for (const chain of chains) {
    if (!isChain(chain)) continue;
    if (chain.reset !== meta.reset) {
      const acked = chain.reset === stored.meta?.reset ? stored.meta.tabs[chain.tab]?.seq ?? 0 : 0;
      if (chain.ops.some((op) => op.seq > Math.max(acked, chain.acked ?? 0))) {
        recover(chain);
      }
      if (!out.stale.includes(chain.tab)) out.stale.push(chain.tab);
      continue;
    }
    const before = meta.tabs[chain.tab]?.seq ?? 0;
    let at = Math.max(before, Number.isInteger(chain.acked) ? chain.acked : 0);
    for (const op of chain.ops) {
      if (op.seq <= at) continue;
      if (op.seq !== at + 1) break;
      const merged = applyKeys(record ? record.data : seed, op.keys ?? {}, merge3);
      const unitMoved = op.unit !== undefined && record && unitOf(record) && op.unit.seen !== meta.unit;
      if (record && (merged.conflict || unitMoved)) out.conflicts.push(clone(record));
      const unit = op.unit !== undefined ? op.unit.to : unitOf(record);
      const id = newId();
      record = {
        v: 1,
        savedAt: op.savedAt ?? new Date(now).toISOString(),
        id,
        ancestry: lineage([], record, meta),
        data: merged.value,
        ...(unit && { live: unit.live, blockeditor: unit.blockeditor }),
      };
      if (op.unit !== undefined) meta.unit = unitMark(unit);
      meta.rev++;
      at = op.seq;
      out.changed = true;
    }
    if (at !== before || meta.tabs[chain.tab]) {
      meta.tabs[chain.tab] = { seq: at, at: now };
      out.changed = true;
    }
    out.applied[chain.tab] = at;
  }

  if (recovery) {
    const found = copies.find(([mark]) => mark === recovery.mark);
    if (!found) throw new Error("That draft is no longer available. Open the list again.");
    if (recovery.type === "remove") {
      meta.recovery = (meta.recovery ?? []).filter((mark) => mark !== recovery.mark);
      out.dropped.push(recovery.mark);
      out.removedMark = recovery.mark;
      out.changed = true;
    } else if (recovery.type === "restore" && recovery.reset === meta.reset) {
      const chosen = recoveryRecord(found[1], seed);
      if (!chosen) throw new Error("This draft cannot be restored by this version.");
      if (meta.recovery?.includes(recovery.mark)) meta.recovery = [recovery.mark, ...meta.recovery.filter((mark) => mark !== recovery.mark)];
      const alreadyKept = record && copies.some(([mark, copy]) => {
        if (!meta.recovery?.includes(mark)) return false;
        const kept = recoveryRecord(copy, seed);
        return kept && same(record.data, kept.data) && same(unitOf(record), unitOf(kept));
      });
      if (record && !alreadyKept) recover(record);
      record = {
        v: 1, savedAt: new Date(now).toISOString(), id: newId(),
        ancestry: lineage([], record, meta), data: chosen.data,
        ...(chosen.blockeditor && { live: chosen.live, blockeditor: chosen.blockeditor }),
      };
      meta.unit = unitMark(unitOf(record));
      meta.rev++;
      out.changed = true;
      out.restoredCopy = true;
    }
  }

  for (const [tab, entry] of Object.entries(meta.tabs)) {
    if (now - (entry.at ?? 0) > TAB_TTL_MS) {
      delete meta.tabs[tab];
      out.changed = true;
    }
  }
  return { meta, record, v1base, ...out };
}

const parse = (text) => {
  if (text == null) return null;
  try { return JSON.parse(text); } catch { return null; }
};

/**
 * Run `commit()` in one readwrite transaction and write what it returns. `input` is a
 * function, called inside the transaction once its reads are in, so what it reads from
 * `localStorage` (journal slots, the marker, the cache) is read at that moment.
 * Resolves on the transaction's `complete` event with `commit()`'s result.
 */
export function transact(drafts, key, input, options) {
  return drafts.change(key, (raw) => {
    const stored = { meta: parse(raw.meta), record: parse(raw.record), v1base: parse(raw.v1base) };
    const rows = raw.rows.map(([name, text]) => [name, parse(text)]);
    const given = input(stored);
    const copies = (raw.recovery ?? []).map(([name, text]) => [name.slice((key + ":recovery:").length), parse(text)]);
    const result = commit(stored, { ...given, copies, chains: [...rows.map(([, chain]) => chain), ...(given.chains ?? [])] });
    const puts = [];
    const deletes = [];
    if (result.changed) {
      puts.push([metaKey(key), JSON.stringify(result.meta)]);
      if (result.record) puts.push([key, JSON.stringify(result.record)]); else if (stored.record) deletes.push(key);
      if (result.v1base) puts.push([key + ":v1base", JSON.stringify(result.v1base)]); else if (stored.v1base) deletes.push(key + ":v1base");
    }
    for (const copy of result.conflicts) puts.push([conflictKey(key, foreignMark(copy)), JSON.stringify(copy)]);
    if (result.conflicts.length) puts.push([conflictKey(key), JSON.stringify(result.conflicts[result.conflicts.length - 1])]);
    for (const copy of result.recovered) puts.push([recoveryKey(key, recoveryMark(copy)), JSON.stringify(copy)]);
    for (const mark of result.dropped) deletes.push(recoveryKey(key, mark));
    for (const [name, chain] of rows) if (settled(chain, result.meta) || (result.removedMark && isChain(chain) && recoveryMark(chain) === result.removedMark)) deletes.push(name);
    return { puts, deletes, result };
  }, options);
}
