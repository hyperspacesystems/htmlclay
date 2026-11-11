// clay.blockeditor.ai: the page half of "rewrite a chosen area". It builds a target for the
// chosen blocks, sends their captured HTML as one request, previews the reply over them
// and keeps it as one undoable transaction. The panel that drives it is ai-edit's.
import { state, model } from "../vendor/prosemirror.vendor.js";
import { schema } from "./schema.js";
import { outline, siblingSpan, touchedBlocks } from "./blocks.js";
import { currentSpan } from "./blocksel.js";
import { showPreview, clearPreview } from "./ai-preview.js";
import { MAX_PAYLOAD_BYTES, contextRefsOf, wantsPage } from "../lib/ai-comment.js";
import { createToolTargets } from "./ai-tool.js";

const { TextSelection } = state;
const HELPER = "ai-edit";
const EDIT_ID = "page area";
const TOO_LARGE = "This section is too large for AI editing. Select a smaller part.";
const STALE = "Someone changed this area while AI was working, so nothing was changed.";
const GONE = "This page is not open for editing any more.";
const NO_WIRE = "AI editing isn't available on this page right now.";

let requestCounter = 0;

// Every block is a top-level node, so one pass finds any of them.
function nodeOf(doc, id) {
  let found = null;
  doc.forEach((node, at) => { if (node.attrs?.id === id) found = { node, at }; });
  return found;
}

const isTool = (doc, id) => {
  const found = nodeOf(doc, id);
  return found?.node?.type === schema.nodes.tool;
};

const idsOf = (ents, lo, hi) => ents.slice(lo, hi + 1).map((e) => e.node.attrs.id).filter(Boolean);

const freshSeed = () => globalThis.crypto?.randomUUID?.() ?? `${Date.now().toString(36)}-${Math.random().toString(36).slice(2)}`;

const emptyRect = () => ({ x: 0, y: 0, left: 0, top: 0, right: 0, bottom: 0, width: 0, height: 0 });

function unionRect(rects) {
  if (!rects.length) return emptyRect();
  const left = Math.min(...rects.map((r) => r.left));
  const top = Math.min(...rects.map((r) => r.top));
  const right = Math.max(...rects.map((r) => r.right));
  const bottom = Math.max(...rects.map((r) => r.bottom));
  return { x: left, y: top, left, top, right, bottom, width: right - left, height: bottom - top };
}

// A non-empty text selection inside one block says which words the request is about.
// Its offsets are counted from the start of that block, as the DOM path counts them
// from the start of its element.
function quoteIn(st, ents, span) {
  const sel = st.selection;
  if (!(sel instanceof TextSelection) || sel.empty) return null;
  const [a, b] = touchedBlocks(st.doc, sel, ents);
  if (a !== b || span.lo !== a || span.hi !== a) return null;
  const block = ents[a];
  if (sel.from < block.at || sel.to > block.end) return null;
  const text = st.doc.textBetween(sel.from, sel.to, " ");
  if (!text.trim()) return null;
  const start = st.doc.textBetween(block.at + 1, sel.from, " ").length;
  return { text, start, end: start + text.length };
}

export function createPageAi({ getView, store = () => null, acceptState } = {}) {
  const viewNow = () => {
    const view = getView();
    return view && !view.isDestroyed ? view : null;
  };

  const targets = new Map();
  const tools = createToolTargets({
    getView: viewNow,
    store,
    acceptState,
  });

  const reviseTarget = (id) => tools.reviseTool(id);

  function areaTarget(ids, { quote = null, label = null } = {}) {
    for (const [key, cached] of targets) if (!cached.isConnected) targets.delete(key);
    const cached = targets.get(ids.join(","));
    if (cached) {
      cached.quote = quote ?? undefined;
      return cached;
    }
    let capture = null;
    let proposal = null;
    const target = {
      aiTarget: true,
      label: label ?? (ids.length === 1 ? "This block" : `${ids.length} blocks`),
      ref: [...ids],
      get isConnected() {
        const view = viewNow();
        return !!view && ids.some((id) => nodeOf(view.state.doc, id));
      },
      getBoundingClientRect() {
        const view = viewNow();
        if (!view) return emptyRect();
        const preview = view.dom.querySelector(".blockeditor-ai-preview");
        if (preview) return preview.getBoundingClientRect();
        const ends = [target.ref[0], target.ref[target.ref.length - 1]];
        const rects = ends
          .map((id) => nodeOf(view.state.doc, id))
          .map((found) => (found ? view.nodeDOM(found.at) : null))
          .filter((node) => node && typeof node.getBoundingClientRect === "function")
          .map((node) => node.getBoundingClientRect());
        return unionRect(rects);
      },
      async start(comment, argument) {
        const view = viewNow();
        if (!view) return { status: "refused", code: "gone", message: GONE };
        // Read here, not imported: a page with exclude=wire must not pull the wire in.
        const wire = window.clay?.wire;
        if (!wire) return { status: "refused", code: "no-wire", message: NO_WIRE };
        // Fetched where a person asks, as the /ai tool client is, so a boot that never
        // rewrites anything never downloads the area engine.
        const { captureArea, AREA_GUIDE } = await import("./ai-area.js");
        const found = captureArea(view.state.doc, target.ref);
        capture = null;
        proposal = null;
        if (found.status !== "ok") return { status: "refused", code: found.code, message: found.message };
        const quoted = argument ?? target.quote;
        const payload = {
          id: "req-" + Date.now().toString(36) + "-" + (++requestCounter),
          editId: EDIT_ID,
          tag: "div",
          elementHTML: found.capture.html,
          comment: comment + "\n\n" + AREA_GUIDE,
          contextRefs: contextRefsOf(comment),
        };
        if (quoted) {
          payload.quote = quoted.text;
          if (Number.isInteger(quoted.start)) payload.selection = { start: quoted.start, end: quoted.end };
        }
        if (wantsPage(comment)) payload.page = true;
        // Refused here, before anything is sent: the envelope would refuse it anyway,
        // and a refusal the page cannot explain is worse than one it can.
        if (new Blob([JSON.stringify(payload)]).size > MAX_PAYLOAD_BYTES) {
          return { status: "refused", code: "too-large", message: TOO_LARGE };
        }
        capture = found.capture;
        return {
          status: "ok",
          id: payload.id,
          send: (hooks = {}) => {
            let live = true;
            let handle = null;
            const done = (async () => {
              // @page reads the file the host has on disk, so the page is saved first.
              if (payload.page) await window.clay?.save?.();
              if (!live) return { state: "cancelled" };
              handle = wire.send(payload, { helper: HELPER, onStatus: hooks.onStatus });
              return handle.done;
            })();
            return { done, cancel() { live = false; handle?.cancel(); } };
          },
        };
      },
      async propose(result) {
        const view = viewNow();
        if (!view || !capture) return { status: "refused", message: "This area is no longer open for AI editing." };
        const { proposeArea } = await import("./ai-area.js");
        const ready = proposeArea(view.state, capture, result?.html ?? "", { seed: freshSeed() });
        if (ready.status !== "ready") return { status: "refused", message: ready.message };
        proposal = ready;
        const dom = view.dom.ownerDocument.createElement("div");
        dom.className = "blockeditor-ai-preview";
        const serializer = model.DOMSerializer.fromSchema(schema);
        dom.appendChild(serializer.serializeFragment(model.Fragment.from(ready.nodes), { document: view.dom.ownerDocument }));
        showPreview(view, { ids: capture.ids, dom });
        return { status: "ready" };
      },
      async keep() {
        const view = viewNow();
        if (!view || !capture || !proposal) return { status: "failed", message: STALE };
        const { keepAreaTransaction } = await import("./ai-area.js");
        const result = keepAreaTransaction(view.state, capture, proposal);
        clearPreview(view);
        if (result.status !== "ok") return { status: "failed", message: STALE };
        view.dispatch(result.tr);
        if (view.dom.isConnected) view.focus();
        try {
          await window.clay?.save?.flush?.();
        } catch {
          return { status: "failed", message: "Your change is in the document, but saving failed. It will be saved with your next edit." };
        }
        return { status: "kept" };
      },
      discard() {
        const view = viewNow();
        if (!view) return;
        clearPreview(view);
        if (view.dom.isConnected) view.focus();
      },
    };
    target.quote = quote ?? undefined;
    targets.set(ids.join(","), target);
    return target;
  }

  return {
    active: () => !!viewNow(),
    targetFromSelection() {
      const view = viewNow();
      if (!view) return null;
      const ents = outline(view.state.doc);
      if (!ents.length) return null;
      const span = currentSpan(view.state, ents);
      const ids = idsOf(ents, span.lo, span.hi);
      if (!ids.length) return null;
      // One tool block is revised, not rewritten: its menu's Revise and Cmd+J on it
      // open the same panel on the same target.
      if (ids.length === 1 && isTool(view.state.doc, ids[0])) return reviseTarget(ids[0]);
      return areaTarget(ids, { quote: quoteIn(view.state, ents, span) });
    },
    targetAt(x, y) {
      const view = viewNow();
      if (!view) return null;
      const hit = view.posAtCoords({ left: x, top: y });
      if (!hit) return null;
      const ents = outline(view.state.doc);
      if (!ents.length) return null;
      const at = hit.inside >= 0 ? hit.inside : hit.pos;
      // A tool is one atomic node, so a point on it resolves to the boundary just after
      // it: the block the person pointed at is the one that ends there.
      const ends = ents.findIndex((e) => e.node.type === schema.nodes.tool && e.end === at);
      const index = ends >= 0 ? ends : Math.min(Math.max(view.state.doc.resolve(at).index(0), 0), ents.length - 1);
      const { lo, hi } = siblingSpan(ents, index, index);
      const ids = idsOf(ents, lo, hi);
      if (!ids.length) return null;
      if (ids.length === 1 && isTool(view.state.doc, ids[0])) return reviseTarget(ids[0]);
      return areaTarget(ids);
    },
    between(a, b) {
      const view = viewNow();
      if (!view) return null;
      const ents = outline(view.state.doc);
      const indexes = [...(a?.ref ?? []), ...(b?.ref ?? [])]
        .map((id) => ents.findIndex((e) => e.node.attrs.id === id))
        .filter((at) => at >= 0);
      if (!indexes.length) return null;
      const { lo, hi } = siblingSpan(ents, Math.min(...indexes), Math.max(...indexes));
      const ids = idsOf(ents, lo, hi);
      return ids.length ? areaTarget(ids) : null;
    },
    wholePage() {
      const view = viewNow();
      const ids = view ? outline(view.state.doc).map((e) => e.node.attrs.id).filter(Boolean) : [];
      return areaTarget(ids, { label: "This page" });
    },
    // `/ai` and Revise are targets the same panel drives, with the tool's own request
    // shape and the tool's own Keep.
    newTool(remove) { return tools.newTool(remove ?? null); },
    reviseTool(toolId) { return tools.reviseTool(toolId); },
  };
}
