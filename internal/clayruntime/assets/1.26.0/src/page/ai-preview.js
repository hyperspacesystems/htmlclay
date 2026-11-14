// The preview of proposed blocks: the chosen blocks hidden, the proposal drawn in their
// place. Nothing is dispatched into the document, so Discard leaves no trace and Keep is
// still one transaction away. While a preview is up the editor answers read-only.
import { state, view as pmView } from "../vendor/prosemirror.vendor.js";

const { Plugin, PluginKey } = state;
const PREVIEW = "blockeditor-ai-preview";

export const aiPreviewKey = new PluginKey("blockeditor-ai-preview");

// One node decoration per hidden block, and the proposal as a widget at the first of
// them. Side -1 puts it above the block it stands for, and it takes no part in
// selection or key handling.
function decorationsFor(doc, ids, dom) {
  const wanted = new Set(ids);
  const decos = [];
  let first = -1;
  doc.forEach((node, at) => {
    if (!wanted.has(node.attrs?.id)) return;
    if (first < 0) first = at;
    decos.push(pmView.Decoration.node(at, at + node.nodeSize, { class: "blockeditor-ai-hidden" }));
  });
  if (first < 0) return null;
  decos.push(pmView.Decoration.widget(first, dom, { side: -1, ignoreSelection: true, stopEvent: () => true }));
  return pmView.DecorationSet.create(doc, decos);
}

// A proposal that stands for no block of its own: a new tool appears at the bookmark
// where it will be inserted, and the page around it stays as it is.
function widgetFor(doc, at, dom) {
  if (typeof at !== "number" || at < 0 || at > doc.content.size) return null;
  return pmView.DecorationSet.create(doc, [
    pmView.Decoration.widget(at, dom, { side: 1, ignoreSelection: true, stopEvent: () => true }),
  ]);
}

export function aiPreviewPlugin() {
  return new Plugin({
    key: aiPreviewKey,
    state: {
      init: () => null,
      // The set is built from the dispatch that shows it and mapped through every other
      // transaction, so a peer's edit moves the hidden blocks and the proposal with it.
      apply(tr, value, prev, next) {
        const meta = tr.getMeta(PREVIEW);
        if (meta === undefined) {
          if (!value) return null;
          const decorations = value.decorations.map(tr.mapping, tr.doc);
          return decorations ? { decorations } : null;
        }
        if (!meta) return null;
        const decorations = meta.ids ? decorationsFor(next.doc, meta.ids, meta.dom) : widgetFor(next.doc, meta.at, meta.dom);
        return decorations ? { decorations } : null;
      },
    },
    props: {
      decorations: (st) => aiPreviewKey.getState(st)?.decorations ?? null,
      // Any props.editable answering false makes ProseMirror read-only, which is how
      // this composes with the boot's own setEditorActive rather than replacing it.
      editable: (st) => (aiPreviewKey.getState(st) ? false : undefined),
    },
  });
}

export function showPreview(view, { ids, dom }) {
  if (!view || view.isDestroyed || !ids?.length || !dom) return;
  view.dispatch(view.state.tr.setMeta(PREVIEW, { ids: [...ids], dom }));
}

export function showWidget(view, { at, dom }) {
  if (!view || view.isDestroyed || typeof at !== "number" || !dom) return;
  view.dispatch(view.state.tr.setMeta(PREVIEW, { at, dom }));
}

export function clearPreview(view) {
  if (!view || view.isDestroyed || !previewShown(view.state)) return;
  view.dispatch(view.state.tr.setMeta(PREVIEW, null));
}

export const previewShown = (state) => !!aiPreviewKey.getState(state);
