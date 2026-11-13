// The area engine: chosen blocks out as a request, a reply back as one undoable
// transaction. No UI, no transport: a panel sends capture.html and proposes what
// comes back, then keeps the proposal through keepAreaTransaction.
import { planAdoption, adoptionTransaction, readBack, recordsOf } from "./adopt.js";
import { canon, fragmentCleaner, parseArticle, renderModel, resolveIds } from "./projection.js";
import { blockMap } from "./model.js";
import { asStep } from "./blocksel.js";

export const AREA_GUIDE = "This element holds blocks from a document editor. Reply with the same outer div. Inside it use p, h1, h2, h3, ul, ol, li, blockquote, pre with code, details with summary, hr, table with thead, tbody, tr, th, td, and the inline tags strong, em, s, u, code, a. You may also keep any tag already in the element. Keep each data-id on the block it belongs to, and give a new block no data-id. An hr with data-clay-keep stands for something you cannot see: keep it where it belongs unless the request says to remove it. Keep each checkbox on a list item unless the request says to remove it. No classes, no style attributes, no scripts.";

const HELD_TYPES = new Set(["image", "file", "chart", "board", "toc", "tool", "preserved"]);
const REWRITE_TYPES = new Set(["paragraph", "heading", "list_item", "quote", "callout", "code_block", "toggle", "divider", "table"]);

const refused = (code, message) => ({ status: "refused", code, message });
const body = ({ id, parent, ...rest }) => ({ ...rest, parent: parent ?? null });
const same = (a, b) => canon(body(a)) === canon(body(b));
const freshSeed = () => globalThis.crypto?.randomUUID?.() ?? `${Date.now().toString(36)}-${Math.random().toString(36).slice(2)}`;
const CARRY_FIELDS = ["owner", "due", "pageIcon", "pageCoverSrc", "pageCoverAlt", "pageCoverX", "pageCoverY"];
const carryFields = (was, item) => {
  const out = { ...item };
  for (const key of CARRY_FIELDS) {
    if (out[key] === undefined || out[key] === null) {
      if (was[key] !== undefined) out[key] = was[key];
    }
  }
  return out;
};

const reconcileColumns = (item, was) => {
  const used = new Set(item.cols.map((col) => col.id).filter(Boolean));
  const cols = item.cols.map((col, i) => {
    if (col.id) return col;
    const source = was.cols[i];
    if (!source?.id || used.has(source.id)) return col;
    used.add(source.id);
    return { ...col, id: source.id };
  }).map((col) => {
    const source = col.id ? was.cols.find((base) => base.id === col.id) : null;
    if (!source) return col;
    const out = { ...col };
    if (out.kind === undefined && source.kind !== undefined) out.kind = source.kind;
    if (out.width === undefined && source.width !== undefined) out.width = source.width;
    return out;
  });
  return { ...item, cols };
};

export function captureArea(doc, ids, { dom = document } = {}) {
  const blocks = blockMap(doc);
  if (ids.some((id) => blocks[id]?.t === "opaque_block")) return refused("opaque", "This area holds a block this editor cannot rewrite.");
  const baseItems = readBack(recordsOf(blocks), dom);
  const wanted = new Set(ids);
  const span = baseItems.filter((item) => wanted.has(item.id));
  if (span.length !== ids.length) return refused("not-found", "Some of these blocks are no longer in the document.");
  const held = new Map();
  const typeOf = new Map();
  const model = span.map((item) => {
    const t = blocks[item.id]?.t ?? item.t;
    if (!HELD_TYPES.has(t)) return item;
    held.set(item.id, item);
    typeOf.set(item.id, t);
    return { id: item.id, t: "divider", parent: item.parent };
  });
  if (span.every((item) => held.has(item.id))) return refused("no-text", "This area holds nothing that can be rewritten.");
  const template = dom.createElement("template");
  template.innerHTML = renderModel(model, { clean: fragmentCleaner(dom) });
  for (const el of template.content.querySelectorAll("hr[data-id]")) {
    const id = el.getAttribute("data-id");
    if (!held.has(id)) continue;
    el.setAttribute("data-clay-keep", typeOf.get(id));
  }
  const lo = baseItems.indexOf(span[0]);
  const hi = baseItems.indexOf(span[span.length - 1]);
  if (hi - lo + 1 !== span.length) return refused("not-found", "These blocks are not next to each other.");
  return {
    status: "ok",
    capture: {
      ids: span.map((item) => item.id),
      blocks,
      baseItems,
      before: baseItems.slice(0, lo),
      span,
      after: baseItems.slice(hi + 1),
      parent: span[0].parent ?? null,
      held,
      html: `<div data-clay-area>${template.innerHTML}</div>`,
    },
  };
}

export function staleIds(doc, capture, dom = document) {
  const items = readBack(recordsOf(blockMap(doc)), dom);
  const now = new Map(items.map((item) => [item.id, item]));
  const changed = capture.span
    .filter((item) => !now.has(item.id) || !same(item, now.get(item.id)))
    .map((item) => item.id);
  if (changed.length) return changed;
  const lo = items.findIndex((item) => item.id === capture.ids[0]);
  const unbroken = capture.ids.every((id, n) => items[lo + n]?.id === id);
  return unbroken ? [] : [...capture.ids];
}

export function proposeArea(state, capture, replyHtml, { seed = "", dom = document } = {}) {
  const stale = staleIds(state.doc, capture, dom);
  if (stale.length) return refused("stale", "These blocks changed while AI was working, so nothing was changed.");

  const template = dom.createElement("template");
  template.innerHTML = replyHtml ?? "";
  const elements = [...template.content.children];
  const only = elements.length === 1 ? elements[0] : null;
  const wrapper = only && only.tagName === "DIV" && (only.hasAttribute("data-clay-area") || !only.hasAttribute("data-id")) ? only : null;
  const article = dom.createElement("article");
  article.append(...(wrapper ? wrapper.childNodes : template.content.childNodes));
  let items = parseArticle(article);
  if (!items.length) return refused("empty", "AI did not write any blocks. Try again.");

  const inside = new Set(capture.ids);
  items = items.map((item) => (item.id && !inside.has(item.id) ? { ...item, id: null } : item));
  if (items.some((item) => !(item.id && capture.held.has(item.id)) && !REWRITE_TYPES.has(item.t))) {
    return refused("active", "AI used formatting this editor cannot put in a document. Try again.");
  }

  const base = new Map(capture.span.map((item) => [item.id, item]));
  const used = new Set(items.map((item) => item.id).filter(Boolean));
  let cursor = 0;
  const repaired = items.map((item) => {
    if (item.id) {
      const at = capture.ids.indexOf(item.id);
      if (at >= 0) cursor = at + 1;
      return item;
    }
    const id = capture.ids[cursor];
    const was = id ? base.get(id) : null;
    if (!id || used.has(id) || !was || was.t !== item.t) return item;
    used.add(id);
    cursor++;
    return { ...item, id };
  });

  const settled = repaired.map((item) => {
    const was = item.id ? base.get(item.id) : null;
    return was?.t === "table" && item.t === "table" && Array.isArray(item.cols) && Array.isArray(was.cols)
      ? reconcileColumns(item, was)
      : item;
  });

  const taken = new Set(capture.baseItems.filter((item) => !base.has(item.id)).map((item) => item.id));
  let reply = resolveIds(settled, { seed, known: base, taken });
  for (let attempt = 0; attempt < 4 && reply.some((item) => taken.has(item.id)); attempt++) {
    reply = resolveIds(settled, { seed: freshSeed(), known: base, taken });
  }
  if (reply.some((item) => taken.has(item.id))) return refused("active", "The new blocks from AI could not be added to the document. Try again.");
  reply = reply.map((item) => (item.id && capture.held.has(item.id) ? { ...capture.held.get(item.id) } : item));
  reply = reply.map((item) => {
    const was = item.id ? base.get(item.id) : null;
    return was && was.t === item.t ? carryFields(was, item) : item;
  });
  if (capture.parent !== null) reply = reply.map((item) => (item.parent ? item : { ...item, parent: capture.parent }));

  const plan = planAdoption(capture.blocks, [...capture.before, ...reply, ...capture.after], capture.baseItems);
  if (!plan) return refused("no-change", "AI did not change anything.");
  const tr = adoptionTransaction(state, plan);
  if (!tr) return refused("no-change", "AI did not change anything.");
  const nodes = new Map();
  tr.doc.forEach((node) => nodes.set(node.attrs.id, node));
  return { status: "ready", plan, reply, nodes: reply.map((item) => nodes.get(item.id)).filter(Boolean) };
}

export function keepAreaTransaction(state, capture, proposal, dom = document) {
  const stale = staleIds(state.doc, capture, dom);
  if (stale.length) return { status: "stale", ids: stale };
  const blocks = recordsOf(blockMap(state.doc));
  const items = readBack(blocks, dom);
  const lo = items.findIndex((item) => item.id === capture.ids[0]);
  if (lo < 0) return { status: "stale", ids: [...capture.ids] };
  const hi = lo + capture.ids.length - 1;
  const plan = planAdoption(blocks, [...items.slice(0, lo), ...proposal.reply, ...items.slice(hi + 1)], items);
  if (!plan) return { status: "stale", ids: [] };
  const tr = adoptionTransaction(state, plan);
  if (!tr) return { status: "stale", ids: [] };
  return { status: "ok", tr: asStep(tr) };
}
