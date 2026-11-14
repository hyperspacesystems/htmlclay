import * as Y from "../../vendor/yjs.vendor.js";
import { LOCAL, REMOTE_META, idKey, mintOp } from "./layout.js";
import { state as pmState } from "../../vendor/prosemirror.vendor.js";
import { walk } from "./walk.js";
import { docOf } from "./doc.js";
import { createProjectionMap } from "./projection-map.js";
import { beforeFence } from "./writer.js";
import { coversOriginal, unitIds } from "./origins.js";
import { COPY_ACTIVE, REMOVE, TEXT_DELETE, attrKey, copyActiveKey, parseGoneIntents, parsePhysicalKey, physicalKeyOf, recoveredRoot, removeKey, restoreKey, visibleOriginIndex } from "./recovery.js";

const refuse = (code) => ({ status: "refused", code });

const attrsKey = (attrs) => {
  if (!attrs) return "";
  const out = [];
  for (const key of Object.keys(attrs).sort()) out.push(`${key}=${JSON.stringify(attrs[key])}`);
  return out.join("\n");
};

const visibleBlocks = (pd, current, view) => {
  const doc = view ? view.state.doc : null;
  const settled = doc && doc.childCount === current.visible.length && current.visible.every((seg, index) => doc.child(index).attrs.id === seg.id);
  const source = settled ? doc : docOf(pd, current);
  const out = [];
  source.forEach((node) => {
    if (node.attrs.id === null || node.attrs.id === undefined) return;
    out.push({ id: node.attrs.id, parent: node.attrs.parent ?? null, t: node.type.name });
  });
  return out;
};

const ancestors = (byId, id) => {
  const out = [];
  let cur = byId.get(id)?.parent ?? null;
  let guard = 0;
  while (cur !== null && cur !== undefined && guard++ <= byId.size) {
    out.push(cur);
    cur = byId.get(cur)?.parent ?? null;
  }
  return out;
};

const isAncestor = (byId, ancestor, id) => ancestors(byId, id).includes(ancestor);
const inSubtree = (byId, id, root) => id === root || isAncestor(byId, root, id);

const setAttr = (pd, seg, id, field, value) => {
  if (seg?.recovered) {
    const key = attrKey(seg.sourceKey, field);
    if ((pd.gone.has(key) ? pd.gone.get(key) : seg.fields?.[field] ?? null) !== value) pd.gone.set(key, value);
    return;
  }
  let map = pd.attrs.get(id);
  if (!map) { map = new Y.Map(); pd.attrs.set(id, map); }
  if (map.get(field) !== value) map.set(field, value);
};

const setParent = (pd, seg, id, parent) => setAttr(pd, seg, id, "parent", parent);

const MAX_INDENT = 8;
const ATTR_RULES = {
  indent: (value, block) => block.t === "list_item" && Number.isInteger(value) && value >= 0 && value <= MAX_INDENT,
  open: (value, block) => block.t === "toggle" && typeof value === "boolean",
  parent: (value, block, byId) => value === null || (typeof value === "string" && byId.get(value)?.t === "toggle" && !inSubtree(byId, value, block.id)),
};

const attrChanges = (args, byId) => {
  if (args?.attrs === undefined) return [];
  if (!args.attrs || typeof args.attrs !== "object" || Array.isArray(args.attrs)) return null;
  const out = [];
  for (const [id, fields] of Object.entries(args.attrs)) {
    const block = byId.get(id);
    if (!block || !fields || typeof fields !== "object" || Array.isArray(fields)) return null;
    for (const [field, value] of Object.entries(fields)) {
      if (!ATTR_RULES[field] || !ATTR_RULES[field](value, block, byId)) return null;
      out.push({ id, field, value });
    }
  }
  return out;
};

const flowUnits = (pd, origins) => {
  const ids = unitIds(pd.flow, 0, pd.flow.length);
  const out = [];
  let at = 0;
  for (const part of pd.flow.toDelta()) {
    const attrs = part.attributes;
    if (typeof part.insert === "string") {
      for (let i = 0; i < part.insert.length; i++) out.push({ ch: part.insert[i], attrs, source: origins.resolve(ids[at++]) });
    } else out.push({ embed: part.insert, attrs, source: origins.resolve(ids[at++]) });
  }
  if (at !== ids.length) throw new Error(`The flow delta has ${at} units but the document has ${ids.length}`);
  return out;
};

const liveRemovals = (pd, current, selected) => {
  const observed = new Map();
  for (const seg of current.visible) if (selected.has(seg.id) && seg.brk) observed.set(physicalKeyOf(seg.brk), seg.id);
  for (const id of selected) for (const key of current.copies.get(id) ?? []) observed.set(key, id);
  const out = [];
  for (let it = pd.flow._start; it; it = it.right) {
    if (it.deleted || !(it.content instanceof Y.ContentEmbed)) continue;
    const id = it.content.embed?.b;
    if (id === undefined || observed.get(physicalKeyOf(it.id)) !== id) continue;
    let end = null;
    for (let next = it.right; next; next = next.right) {
      if (next.deleted || !(next.content instanceof Y.ContentEmbed)) continue;
      const other = next.content.embed?.b;
      if (other === undefined || other === id) continue;
      end = physicalKeyOf(next.id);
      break;
    }
    out.push({ key: physicalKeyOf(it.id), blockId: id, end });
  }
  return out;
};

export function moveBlocks({ pd, view, um, origins, render, walked = null }, args = {}) {
  const ids = Array.isArray(args?.ids) ? [...new Set(args.ids)] : [];
  const afterId = args?.afterId === undefined ? null : args.afterId;
  const hasParent = args?.parent !== undefined;

  const current = walked ?? walk(pd, origins);
  // An opaque block is an operation this runtime must refuse before writing: it
  // has no turn-into, it cannot be carried, and its record is never rewritten.
  const protectedIds = new Set(current.visible.filter((seg) => seg.opaque).map((seg) => seg.id));
  const list = visibleBlocks(pd, current, view);
  const byId = new Map(list.map((block) => [block.id, block]));
  const order = list.map((block) => block.id);

  if (!ids.length) return refuse("missing-source");
  for (const id of ids) if (!byId.has(id)) return refuse("missing-source");
  const changes = attrChanges(args, byId);
  if (!changes) return refuse("invalid-attrs");
  const parentChanges = changes.filter((change) => change.field === "parent");

  const roots = new Set(ids.filter((id) => !ids.some((other) => other !== id && isAncestor(byId, other, id))));
  const selected = list.filter((block) => ids.some((root) => inSubtree(byId, block.id, root)));
  const selectedSet = new Set(selected.map((block) => block.id));
  if (selected.some((block) => protectedIds.has(block.id)) || changes.some((change) => protectedIds.has(change.id))) return refuse("unsupported-format");

  const segments = new Map(current.visible.map((seg) => [seg.id, seg]));
  for (const block of selected) {
    const seg = segments.get(block.id);
    if (!seg || seg.implicit || !seg.brk) return refuse("missing-source");
  }

  let parent;
  if (hasParent) parent = args.parent ?? null;
  else {
    const parents = new Set(selected.filter((block) => roots.has(block.id)).map((block) => block.parent));
    if (parents.size !== 1) return refuse("invalid-parent");
    parent = [...parents][0];
  }
  if (hasParent && parent !== null) {
    const node = byId.get(parent);
    if (!node || node.t !== "toggle") return refuse("invalid-parent");
    if (selectedSet.has(parent)) return refuse("cycle");
  }
  if (afterId !== null) {
    if (typeof afterId !== "string" || !byId.has(afterId)) return refuse("missing-destination");
    if (selectedSet.has(afterId)) return refuse("cycle");
    if (byId.get(afterId).parent !== parent) return refuse("missing-destination");
  }

  const remaining = list.filter((block) => !selectedSet.has(block.id));
  let insertAt = 0;
  if (afterId === null) {
    if (parent !== null) {
      const first = remaining.findIndex((block) => block.parent === parent);
      const own = remaining.findIndex((block) => block.id === parent);
      insertAt = first >= 0 ? first : own < 0 ? 0 : own + 1;
    }
  } else {
    insertAt = remaining.findIndex((block) => block.id === afterId) + 1;
    while (insertAt < remaining.length && inSubtree(byId, remaining[insertAt].id, afterId)) insertAt++;
  }

  const desired = [...remaining.slice(0, insertAt), ...selected, ...remaining.slice(insertAt)];
  const sameOrder = desired.length === order.length && desired.every((block, index) => block.id === order[index]);
  const movedRoots = selected.filter((block) => roots.has(block.id) && block.parent !== parent);
  const movedIds = selected.map((block) => block.id);
  if (sameOrder && !movedRoots.length && !changes.length) return { status: "unchanged" };

  const caret = (() => {
    const sel = view?.state.selection;
    if (!(sel instanceof pmState.TextSelection)) return null;
    const at = (pos) => { const $pos = view.state.doc.resolve(pos); return $pos.depth ? { id: $pos.node(1).attrs.id, offset: $pos.parentOffset } : null; };
    const anchor = at(sel.anchor), head = at(sel.head);
    return anchor && head && (selectedSet.has(anchor.id) || selectedSet.has(head.id)) ? { anchor, head } : null;
  })();
  const restoreCaret = () => {
    if (!caret || !view) return;
    const place = ({ id, offset }) => {
      let found = null;
      view.state.doc.forEach((node, pos) => { if (found === null && node.attrs.id === id) found = pos + 1 + Math.min(offset, node.content.size); });
      return found;
    };
    const anchor = place(caret.anchor), head = place(caret.head);
    if (anchor !== null && head !== null) view.dispatch(view.state.tr.setSelection(pmState.TextSelection.create(view.state.doc, anchor, head)).setMeta(REMOTE_META, true).setMeta("addToHistory", false));
  };
  if (sameOrder) {
    um?.stopCapturing();
    pd.ydoc.transact(() => {
      if (parentChanges.length) for (const change of parentChanges) setParent(pd, segments.get(change.id), change.id, change.value);
      else for (const block of movedRoots) setParent(pd, segments.get(block.id), block.id, parent);
      for (const change of changes) if (change.field !== "parent") setAttr(pd, segments.get(change.id), change.id, change.field, change.value);
    }, LOCAL);
    um?.stopCapturing();
    render();
    restoreCaret();
    return { status: "moved", ids: movedIds };
  }

  const sv = [...Y.decodeStateVector(Y.encodeStateVector(pd.ydoc)).entries()].sort((left, right) => left[0] - right[0]);
  const operationId = mintOp(pd);
  const units = flowUnits(pd, origins);
  const before = createProjectionMap(current, pd.flow.length);
  const captured = [];
  let cursor = 0;
  for (const seg of current.visible) {
    if (selectedSet.has(seg.id)) {
      const block = [];
      for (const [from, to] of before.physicalRanges(cursor, cursor + seg.len)) for (let unit = from; unit < to; unit++) block.push(units[unit]);
      captured.push({ seg, units: block });
    }
    cursor += seg.len;
  }
  for (let index = 0; index < captured.length; index++) {
    const block = captured[index];
    if (block.seg.id !== movedIds[index]) throw new Error(`Captured ${block.seg.id} where ${movedIds[index]} was expected`);
    if (!block.units.length || block.units[0].embed === undefined || block.units[0].embed.b !== block.seg.id) throw new Error(`The selected block ${block.seg.id} has no physical marker to copy`);
  }

  const removals = liveRemovals(pd, current, selectedSet);
  const intents = parseGoneIntents(pd.gone);
  const promotions = new Map();
  for (const block of selected) {
    const seg = segments.get(block.id);
    if (!seg.recovered) continue;
    const removalsOf = current.removals.filter((removal) => removal.sourceKey === seg.sourceKey);
    const winner = recoveredRoot(seg.sourceKey, removalsOf, intents);
    const baseline = intents.restores.find((restore) => restore.key === winner?.key)?.baseline ?? { type: seg.t, attrs: seg.fields ?? {} };
    promotions.set(seg.sourceKey, { covers: removalsOf.map((removal) => removal.key), baseline: JSON.parse(JSON.stringify(baseline)) });
  }
  if (!removals.length) throw new Error("The selected blocks have no live physical markers to retire");

  um?.stopCapturing();
  pd.ydoc.transact(() => {
    for (const removal of removals) pd.gone.set(removeKey(removal.key, operationId), { blockId: removal.blockId, end: removal.end, sv });

    const after = walk(pd, origins);
    const map = createProjectionMap(after, pd.flow.length);
    const afterIds = after.visible.map((seg) => seg.id);
    let index = 0;
    if (insertAt > 0) {
      const found = afterIds.indexOf(remaining[insertAt - 1].id);
      index = found < 0 ? 0 : found + 1;
    }
    let position = 0;
    for (let i = 0; i < index; i++) position += after.visible[i].len;
    let cursor = map.toPhysical(position, "before");
    const spans = [];
    for (const block of captured) {
      const start = cursor;
      let text = null, textAttrs = null;
      const flush = () => {
        if (text === null) return;
        beforeFence(pd, cursor, after.fences, () => pd.flow.insert(cursor, text, textAttrs ?? undefined));
        cursor += text.length;
        text = null;
      };
      for (const unit of block.units) {
        if (unit.ch !== undefined) {
          if (text !== null && attrsKey(unit.attrs) !== attrsKey(textAttrs)) flush();
          if (text === null) { text = ""; textAttrs = unit.attrs; }
          text += unit.ch;
        } else {
          flush();
          beforeFence(pd, cursor, after.fences, () => pd.flow.insertEmbed(cursor, unit.embed, unit.attrs));
          cursor += 1;
        }
      }
      flush();
      const dest = unitIds(pd.flow, start, cursor);
      if (dest.length !== block.units.length) throw new Error(`Copied ${block.seg.id} as ${dest.length} units instead of ${block.units.length}`);
      for (let i = 1; i < block.units.length; i++) spans.push({ dest: dest[i], source: block.units[i].source });
      origins.recordBoundary(dest[0], block.seg.recovered ? { blockId: block.seg.id, kind: "copy", recoveryOf: block.seg.sourceKey } : { blockId: block.seg.id, kind: "copy" });
      if (block.seg.recovered) pd.gone.set(restoreKey(block.seg.sourceKey, operationId), promotions.get(block.seg.sourceKey));
      pd.gone.set(copyActiveKey(physicalKeyOf(dest[0])), true);
    }
    if (spans.length) origins.recordSpans(spans);
    if (parentChanges.length) for (const change of parentChanges) setParent(pd, segments.get(change.id), change.id, change.value);
    else for (const block of movedRoots) setParent(pd, segments.get(block.id), block.id, parent);
    for (const change of changes) if (change.field !== "parent") setAttr(pd, segments.get(change.id), change.id, change.field, change.value);
  }, LOCAL);
  um?.stopCapturing();
  render();
  restoreCaret();
  return { status: "moved", ids: movedIds };
}

const CARRY = "blockeditor-move-carry";

const COPY_ACTIVE_KEYS = `["${COPY_ACTIVE}",`;
const REMOVE_KEYS = `["${REMOVE}",`;
const TEXT_DELETE_KEYS = `["${TEXT_DELETE}",`;

const startOf = (pd, sourceKey) => {
  const id = parsePhysicalKey(sourceKey);
  if (!id) return null;
  let it = null;
  try { it = Y.getItem(pd.ydoc.store, id); } catch { return null; }
  return it instanceof Y.Item && !it.deleted && it.parent === pd.flow ? it : null;
};

const targetOf = (pd, origins, walked, map, blockId) => {
  let cursor = 0, seg = null;
  for (const candidate of walked.visible) {
    if (candidate.id === blockId) { seg = candidate; break; }
    cursor += candidate.len;
  }
  if (!seg || seg.implicit || seg.atom || seg.opaque || !seg.brk) return null;
  const places = new Map();
  let head = null;
  for (const [from, to] of map.physicalRanges(cursor, cursor + seg.len)) {
    const ids = unitIds(pd.flow, from, to);
    for (let i = 0; i < ids.length; i++) {
      if (head === null) head = from;
      else places.set(idKey(origins.resolve(ids[i])), from + i);
    }
  }
  return head === null ? null : { head, places };
};

const formatsBefore = (pd, stop) => {
  const out = new Map();
  for (let it = pd.flow._start; it && it !== stop; it = it.right) {
    if (it.deleted || !(it.content instanceof Y.ContentFormat)) continue;
    if (it.content.value === null) out.delete(it.content.key);
    else out.set(it.content.key, it.content.value);
  }
  return out;
};

const rangesOf = (ranges) => {
  const set = Y.createDeleteSet();
  for (const { client, clock, len } of ranges) (set.clients.get(client) ?? set.clients.set(client, []).get(client)).push({ clock, len });
  return set;
};

const markerOf = (it) => it.content instanceof Y.ContentEmbed && it.content.embed?.b !== undefined;

// The block a unit was typed into: the first block marker on the chain of left neighbours
// each unit was inserted after. The chain never changes, so every tab finds the same block,
// and a word has one home copy whose owner alone may carry it.
const homeOf = (pd, homes, it) => {
  const path = [];
  let at = it, home = null;
  for (;;) {
    const known = homes.get(at);
    if (known !== undefined) { home = known; break; }
    path.push(at);
    let next = null;
    if (at.origin) try { next = Y.getItem(pd.ydoc.store, at.origin); } catch { next = null; }
    if (!(next instanceof Y.Item)) break;
    if (markerOf(next)) { home = next; break; }
    at = next;
  }
  for (const item of path) homes.set(item, home);
  return home;
};

// Only a copy's own words move, only deletions a person's record backs are mirrored, and the
// words join the history step that shows the region again, so that step puts each word back
// exactly where it was typed. A region a Redo hid again is only mirrored: what reached it
// since the Undo is set aside, as before the carry existed.
function carryAcross({ pd, origins, walked, placed }, owned, homes, flip = null, records = false) {
  const entries = walked.hidden.filter((entry) => owned.has(entry.key));
  if (!entries.length) return false;
  const self = pd.ydoc.clientID;
  const deletes = flip !== null || records ? parseGoneIntents(pd.gone).textDeletes.filter((record) => record.actor !== self) : [];
  const removed = new Map();
  const taken = new Set();
  const inserts = [];
  const folds = new Map();
  let visible = null, map = null;
  for (const entry of entries) {
    const owner = owned.get(entry.key);
    const { left, past } = owner;
    const start = startOf(pd, entry.sourceKey);
    if (!start) continue;
    const live = new Map(entry.items.map((item) => [item.id, item]));
    const region = [];
    for (let it = start.right; it; it = it.right) {
      if (it.countable && !it.deleted && !live.has(idKey(it.id))) break;
      region.push(it);
    }
    const words = new Set(owner.copy ? region.filter((it) => !it.deleted && it.countable && !markerOf(it) && !left.has(`${idKey(it.id)}+${it.length}`) && homeOf(pd, homes, it) === start) : []);
    const covering = deletes.filter((record) => region.some((it) => it.deleted && it.countable && it.id.client === record.client && record.clock < it.id.clock + it.length && it.id.clock < record.clock + record.length));
    if (!words.size && !covering.length) continue;
    const seen = owner.seen ?? new Map(entry.sv);
    const fresh = (client, clock) => clock >= (seen.get(client) ?? 0);
    const formats = words.size ? formatsBefore(pd, start) : null;
    let attrs = null;
    const units = [];
    const shown = new Map();
    for (const it of region) {
      if (!it.countable) {
        if (formats && it.content instanceof Y.ContentFormat && (!it.deleted || Y.isDeleted(past, it.id))) {
          if (it.content.value === null) formats.delete(it.content.key);
          else formats.set(it.content.key, it.content.value);
          attrs = null;
        }
        continue;
      }
      const item = it.deleted ? null : live.get(idKey(it.id));
      const word = words.has(it);
      for (let offset = 0; offset < it.length; offset++) {
        const id = { client: it.id.client, clock: it.id.clock + offset };
        const source = origins.resolve(id);
        const key = idKey(source);
        if (!(it.deleted && covering.length && coversOriginal(covering, self, id))) shown.set(key, true);
        else if (!shown.has(key) && (!owner.alive || fresh(id.client, id.clock) || owner.alive.has(idKey(id)))) shown.set(key, false);
        units.push({ it, item, offset, id, source, key, attrs: word ? (attrs ??= Object.fromEntries(formats)) : null });
      }
    }
    map ??= createProjectionMap(walked, pd.flow.length);
    const target = targetOf(pd, origins, walked, map, entry.blockId);
    if (!target) {
      for (const it of words) left.add(`${idKey(it.id)}+${it.length}`);
      continue;
    }
    if (covering.length) for (const [key, place] of target.places) if (shown.get(key) === false && !removed.has(place)) removed.set(place, null);
    const groups = new Map();
    let at = target.head + 1;
    for (const unit of units) {
      const { it, item, offset, id, source } = unit;
      if (unit.attrs) {
        if (!taken.has(idKey(id)) && !(visible ??= visibleOriginIndex(pd.flow, walked, origins))(source.client, source.clock)) {
          taken.add(idKey(id));
          removed.set(item.at + offset, owner);
          if (owner.step && !folds.has(owner)) folds.set(owner, { deleted: [], inserted: [] });
          const group = groups.get(at) ?? groups.set(at, []).get(at);
          group.push(item.kind === "text" ? { ch: it.content.str[offset], attrs: unit.attrs, source } : { embed: it.content.embed, attrs: unit.attrs, source });
          continue;
        }
        left.add(`${idKey(it.id)}+${it.length}`);
      }
      const place = target.places.get(unit.key);
      if (place !== undefined) at = place + 1;
    }
    for (const [place, list] of groups) inserts.push({ at: place, units: list, owner });
  }
  if (!removed.size && !inserts.length) return false;
  const ops = [...[...removed].map(([at, owner]) => ({ at, owner })), ...inserts].sort((a, b) => b.at - a.at || (a.units ? 1 : 0) - (b.units ? 1 : 0));
  pd.ydoc.transact((tr) => {
    let run = null;
    // Deleting a word also deletes the format items that only it needed. The step that
    // brings the word back has to bring those back with it, so it keeps all of them.
    const cut = () => {
      if (!run) return;
      const fold = folds.get(run.owner);
      const sizes = fold ? new Map([...tr.deleteSet.clients].map(([client, ranges]) => [client, ranges.length])) : null;
      pd.flow.delete(run.from, run.to - run.from);
      if (fold) for (const [client, ranges] of tr.deleteSet.clients) for (const { clock, len } of ranges.slice(sizes.get(client) ?? 0)) {
        const struct = Y.getItem(pd.ydoc.store, { client, clock });
        if (struct instanceof Y.Item) struct.keep = true;
        fold.deleted.push({ client, clock, len });
      }
      run = null;
    };
    for (const op of ops) {
      if (!op.units) {
        if (run && op.at === run.from - 1 && op.owner === run.owner) run.from = op.at;
        else { cut(); run = { from: op.at, to: op.at + 1, owner: op.owner }; }
        continue;
      }
      cut();
      const before = Y.getState(pd.ydoc.store, self);
      let cursor = op.at, text = "", attrs = null;
      const put = () => {
        if (!text) return;
        const chars = text;
        beforeFence(pd, cursor, walked.fences, () => pd.flow.insert(cursor, chars, { ...attrs }));
        cursor += chars.length;
        text = "";
      };
      for (const unit of op.units) {
        if (unit.ch === undefined) {
          put();
          beforeFence(pd, cursor, walked.fences, () => pd.flow.insertEmbed(cursor, { ...unit.embed }, { ...unit.attrs }));
          cursor += 1;
          continue;
        }
        if (text && attrsKey(unit.attrs) !== attrsKey(attrs)) put();
        attrs = unit.attrs;
        text += unit.ch;
      }
      put();
      folds.get(op.owner)?.inserted.push({ client: self, clock: before, len: Y.getState(pd.ydoc.store, self) - before });
      const dest = unitIds(pd.flow, op.at, cursor);
      if (dest.length !== op.units.length) throw new Error(`Carried ${dest.length} units instead of ${op.units.length}`);
      origins.recordSpans(dest.map((id, i) => ({ dest: id, source: op.units[i].source })));
      placed.push({ ids: dest, step: op.owner.step });
    }
    cut();
  }, CARRY);
  for (const [{ step }, fold] of folds) {
    step.deletions = Y.mergeDeleteSets([step.deletions, rangesOf(fold.deleted)]);
    step.insertions = Y.mergeDeleteSets([step.insertions, rangesOf(fold.inserted)]);
  }
  return true;
}

const eachUnit = (pd, set, f) => {
  for (const [client, ranges] of set.clients) for (const { clock, len } of ranges) {
    for (let at = clock; at < clock + len;) {
      let it = null;
      try { it = Y.getItem(pd.ydoc.store, { client, clock: at }); } catch { break; }
      const end = Math.min(clock + len, it.id.clock + it.length);
      if (it instanceof Y.Item && it.parent === pd.flow && it.countable) for (let unit = at; unit < end; unit++) f(it, { client, clock: unit });
      at = end;
    }
  }
};

const recorded = (records, id) => records.some((record) => record.client === id.client && id.clock >= record.clock && id.clock < record.clock + record.length);

const gone = (pd, id) => {
  try { return Y.getItem(pd.ydoc.store, id).deleted; } catch { return true; }
};

// Before a step is replayed. A word it inserted that a carry or another replay moved (that
// unit and its source unit are both gone, and no person's record deleted them) is followed to
// every live unit holding it, so the typist's Undo takes the word out where it now is. A unit
// the step would re-create is left out when nothing holds its word any more, so a Redo never
// brings back a word its typist undid or a person deleted.
const followStep = (pd, origins, step) => {
  const dead = [];
  eachUnit(pd, step.insertions, (it, id) => { if (it.deleted) dead.push(id); });
  if (!dead.length) return;
  const records = parseGoneIntents(pd.gone).textDeletes;
  const words = new Map();
  for (const id of dead) {
    const source = origins.resolve(id);
    const word = words.get(idKey(source)) ?? words.set(idKey(source), { source, follow: false, redo: [] }).get(idKey(source));
    if (!word.follow && !recorded(records, id) && gone(pd, source)) word.follow = true;
  }
  eachUnit(pd, step.deletions, (it, id) => words.get(idKey(origins.resolve(id)))?.redo.push(id));
  const wanted = new Map();
  for (const word of words.values()) {
    if (!word.follow && !word.redo.length) continue;
    word.lives = [];
    (wanted.get(word.source.client) ?? wanted.set(word.source.client, []).get(word.source.client)).push(word);
  }
  if (!wanted.size) return;
  for (let it = pd.flow._start; it; it = it.right) {
    if (it.deleted || !it.countable) continue;
    let at = it.id.clock;
    for (const piece of origins.resolveRange(it.id.client, it.id.clock, it.length)) {
      for (const word of wanted.get(piece.client) ?? []) {
        const offset = word.source.clock - piece.clock;
        if (offset >= 0 && offset < piece.length) word.lives.push({ client: it.id.client, clock: at + offset, len: 1 });
      }
      at += piece.length;
    }
  }
  const adds = [];
  for (const list of wanted.values()) for (const word of list) {
    if (word.follow) adds.push(...word.lives);
    if (!word.lives.length) for (const id of word.redo) adds.push({ client: id.client, clock: id.clock, len: 1 });
  }
  if (adds.length) step.insertions = Y.mergeDeleteSets([step.insertions, rangesOf(adds)]);
};

const holds = (it, id) => id !== null && it.id.client === id.client && id.clock >= it.id.clock && id.clock < it.id.clock + it.length;

// A block marker another tab inserted between a carried word and the unit the carry put it
// after, without having seen the word (its right neighbour lies past the word). Yjs orders such
// ties by client number, so the word can end up inside that tab's new block. Returns the marker
// nearest the unit the carry put the word after.
const cutIn = (pd, ids) => {
  let item;
  try { item = Y.getItem(pd.ydoc.store, ids[0]); } catch { return null; }
  if (item.id.clock !== ids[0].clock || !item.origin) return null;
  const between = [item];
  let edge = null;
  for (let it = item.left; it && !holds(it, item.origin); it = it.left) {
    if (markerOf(it) && it.id.client !== pd.ydoc.clientID && !between.some((other) => holds(other, it.rightOrigin))) edge = it;
    between.push(it);
  }
  return edge;
};

// The carrying tab alone puts such a word back in front of that marker, into the block it was
// carried to. If that block is hidden by then, the word is set aside there as any hidden word is.
const reseat = (pd, origins, walked, placed) => {
  const kept = [], moved = [];
  for (const group of placed) {
    const live = group.ids.filter((id) => !gone(pd, id));
    if (!live.length) continue;
    const edge = cutIn(pd, group.ids);
    if (edge) moved.push({ group, edge, live: new Set(live.map(idKey)) });
    else kept.push(group);
  }
  if (!moved.length) return placed;
  const self = pd.ydoc.clientID;
  pd.ydoc.transact(() => {
    for (const { group, edge, live } of moved) {
      const formats = new Map();
      const units = [];
      let at = 0;
      for (let it = pd.flow._start; it; it = it.right) {
        if (it.content instanceof Y.ContentFormat) {
          if (!it.deleted) { if (it.content.value === null) formats.delete(it.content.key); else formats.set(it.content.key, it.content.value); }
          continue;
        }
        if (it.deleted || !it.countable) continue;
        for (let offset = 0; offset < it.length; offset++) {
          const id = { client: it.id.client, clock: it.id.clock + offset };
          if (live.has(idKey(id))) units.push({ at: at + offset, ch: it.content instanceof Y.ContentString ? it.content.str[offset] : undefined, embed: it.content.embed, attrs: Object.fromEntries(formats), source: origins.resolve(id) });
        }
        at += it.length;
      }
      for (let i = units.length - 1; i >= 0; i--) pd.flow.delete(units[i].at, 1);
      let cursor = 0;
      for (let it = pd.flow._start; it && it !== edge; it = it.right) if (!it.deleted && it.countable) cursor += it.length;
      const start = cursor;
      const before = Y.getState(pd.ydoc.store, self);
      const fences = [...walked.fences, edge.id];
      for (const unit of units) {
        if (unit.ch === undefined) beforeFence(pd, cursor, fences, () => pd.flow.insertEmbed(cursor, { ...unit.embed }, { ...unit.attrs }));
        else beforeFence(pd, cursor, fences, () => pd.flow.insert(cursor, unit.ch, { ...unit.attrs }));
        cursor += 1;
      }
      const dest = unitIds(pd.flow, start, cursor);
      if (dest.length !== units.length) throw new Error(`Moved ${dest.length} carried units instead of ${units.length}`);
      origins.recordSpans(dest.map((id, i) => ({ dest: id, source: units[i].source })));
      if (group.step) group.step.insertions = Y.mergeDeleteSets([group.step.insertions, rangesOf([{ client: self, clock: before, len: Y.getState(pd.ydoc.store, self) - before }])]);
      kept.push({ ids: dest, step: group.step });
    }
  }, CARRY);
  return kept;
};

// Whether Yjs will change anything when it pops this step. It pops every step that would not,
// in the same press, so each of those needs its moved words followed too.
const acts = (pd, step) => {
  const store = pd.ydoc.store;
  for (const [client, ranges] of step.insertions.clients) for (const { clock, len } of ranges) {
    for (let at = clock; at < clock + len;) {
      let it;
      try { it = Y.getItem(store, { client, clock: at }); } catch { break; }
      let live = it;
      while (live instanceof Y.Item && live.redone) { try { live = Y.getItem(store, live.redone); } catch { break; } }
      if (live instanceof Y.Item && !live.deleted) return true;
      at = it.id.clock + it.length;
    }
  }
  for (const [client, ranges] of step.deletions.clients) for (const { clock } of ranges) {
    if (!Y.isDeleted(step.insertions, { client, clock })) return true;
  }
  return false;
};

const liveIn = (pd, sourceKey) => {
  const live = new Set();
  for (let it = startOf(pd, sourceKey)?.right ?? null; it; it = it.right) {
    if (!it.countable || it.deleted) continue;
    if (markerOf(it)) break;
    for (let offset = 0; offset < it.length; offset++) live.add(idKey({ client: it.id.client, clock: it.id.clock + offset }));
  }
  return live;
};

// Only the tab whose own Undo retired a copy carries that copy's words: a word's home copy is
// fixed (homeOf), so no two tabs carry the same word. A Redo carries nothing forward and
// mirrors only the recorded loss of a unit alive at the Undo (`alive`) or newer (`seen`).
export function createMoveCarry({ pd, origins, um, walked }) {
  const owned = new Map();
  const shown = new Map();
  const homes = new WeakMap();
  let flip = null;
  let placed = [];
  const run = (flipped = null, records = false) => owned.size > 0 && carryAcross({ pd, origins, walked: walked(), placed }, owned, homes, flipped, records);
  return {
    note(t) {
      const keys = [...(t.changed.get(pd.gone) ?? [])].filter((key) => typeof key === "string");
      if (!keys.some((key) => key.startsWith(COPY_ACTIVE_KEYS))) return;
      const hidden = new Set();
      const past = rangesOf([...t.deleteSet.clients].flatMap(([client, ranges]) => ranges.map(({ clock, len }) => ({ client, clock, len }))));
      for (const key of keys) {
        const copy = key.startsWith(COPY_ACTIVE_KEYS);
        if (!copy && !key.startsWith(REMOVE_KEYS)) continue;
        if (pd.gone.has(key) === copy) {
          owned.delete(key);
          if (!copy) shown.set(key, { seen: t.afterState, alive: liveIn(pd, JSON.parse(key)[1]) });
          continue;
        }
        hidden.add(key);
        owned.set(key, { copy, left: new Set(), past, seen: null, alive: null, step: null, ...(copy ? null : shown.get(key)) });
      }
      if (hidden.size) flip = hidden;
    },
    replay(direction) {
      const pending = direction === "undo" ? um.undoStack : um.redoStack;
      for (let i = pending.length - 1; i >= 0; i--) {
        followStep(pd, origins, pending[i]);
        if (acts(pd, pending[i])) break;
      }
      const inverse = direction === "undo" ? um.redoStack : um.undoStack;
      const size = inverse.length;
      flip = null;
      if (um[direction]() === null) return false;
      const flipped = flip;
      flip = null;
      if (flipped && inverse.length > size) for (const key of flipped) owned.get(key).step = inverse[inverse.length - 1];
      run(flipped);
      return true;
    },
    received(transactions) {
      if (!owned.size && !placed.length) return;
      let late = false, records = false;
      for (const t of transactions) {
        if (t.local) continue;
        late = true;
        for (const key of t.changed.get(pd.gone) ?? []) if (typeof key === "string" && key.startsWith(TEXT_DELETE_KEYS)) records = true;
      }
      if (!late) return;
      run(null, records);
      if (placed.length) placed = reseat(pd, origins, walked(), placed);
    },
  };
}
