// /ai tools as contract A targets: the page's AI surface builds one of these and the
// shared panel in ai-edit.js drives it. Making a tool and revising one differ only in
// the request's target and current revision, and in what Keep writes.
import { state } from "../vendor/prosemirror.vendor.js";
import { schema } from "./schema.js";
import { asStep } from "./blocksel.js";
import { keyBetween, newId } from "./model.js";
import { createToolClient } from "./rich/page-tool.js";
import { createToolHost } from "./tools.js";
import { tablesFromDoc, toolContext } from "./tool-data.js";
import { showPreview, showWidget, clearPreview } from "./ai-preview.js";
import { validateArtifact, validateState } from "./rich/tool-runtime.js";
import { createToolTransport } from "./tool-transport.js";
import { toolBlockViews } from "./tool-views.js";

const { Selection } = state;

const GONE = "This page is not open for editing any more.";
const NO_WIRE = "AI tool making isn't available on this page right now.";
const NO_STORE = "This document can't save tool data.";
const MOVED = "The place for this tool moved. Close this and run /ai where you want the tool.";
const STALE_REVISION = "This tool changed while you were revising, so nothing was replaced. Ask again.";
const BAD_ARTIFACT = "AI did not make a usable tool. Try again.";
const BAD_STATE = "The tool data from AI is too large or not usable. Try again.";
const CHANGED = "This tool's data changed while AI was working, so it was not replaced. Ask again to include the latest data.";
const SAVE_FAILED = "The tool could not be saved, so it was not added. Try again.";
const UNSAVED = "Your tool is in the document, but saving failed. It will be saved with your next edit.";

function findBlock(doc, id) {
  let found = null;
  doc.forEach((node, at, index) => { if (node.attrs.id === id) found = { node, at, index }; });
  return found;
}

// The block the /ai was typed in and the text of that query. Keep uses it to place the
// tool and to remove exactly the slash text, never a caret that moved meanwhile.
function bookmarkIn(st, remove) {
  const $head = st.selection.$head;
  const index = Math.min(Math.max($head.index(0), 0), st.doc.childCount - 1);
  const host = st.doc.child(index);
  return {
    hostId: host?.attrs.id ?? null,
    slashText: remove ? st.doc.textBetween(remove.from, remove.to) : null,
    slashOffset: remove && $head.depth ? remove.from - $head.before(1) - 1 : null,
  };
}

// Where the tool goes: over an empty or slash-only block, else in a new block after it.
function placeIn(st, bookmark) {
  const found = bookmark?.hostId ? findBlock(st.doc, bookmark.hostId) : null;
  if (!found) return null;
  const next = st.doc.maybeChild(found.index + 1);
  const replace = found.node.isTextblock && (found.node.content.size === 0 || found.node.textContent === bookmark.slashText);
  return {
    ...found,
    replace,
    pos: replace ? found.node.attrs.pos : keyBetween(found.node.attrs.pos, next?.attrs.pos ?? null),
    insertAt: replace ? found.at : found.at + found.node.nodeSize,
  };
}

function insertCandidate(view, candidate, bookmark) {
  const st = view.state;
  const place = placeIn(st, bookmark);
  if (!place) return false;
  const { node: current, at, insertAt, replace } = place;
  const tool = schema.nodes.tool.create({
    id: candidate.id, pos: place.pos, parent: current.attrs.parent ?? null,
    artifact: candidate.artifact, rev: candidate.rev, state: candidate.state,
  });
  const tr = st.tr;
  if (!replace && bookmark?.hostId === current.attrs.id && bookmark.slashText && bookmark.slashOffset >= 0) {
    const from = at + 1 + bookmark.slashOffset;
    const to = from + bookmark.slashText.length;
    if (to <= at + current.nodeSize - 1 && st.doc.textBetween(from, to) === bookmark.slashText) tr.delete(from, to);
  }
  const mapped = tr.mapping.map(insertAt);
  tr.replaceWith(mapped, replace ? tr.mapping.map(at + current.nodeSize) : mapped, tool);
  tr.setSelection(Selection.near(tr.doc.resolve(Math.min(mapped + tool.nodeSize, tr.doc.content.size))));
  view.dispatch(asStep(tr).scrollIntoView());
  return true;
}

const emptyRect = () => ({ x: 0, y: 0, left: 0, top: 0, right: 0, bottom: 0, width: 0, height: 0 });

export function createToolTargets({
  getView,
  store = () => null,
  acceptState = () => globalThis.clay?.save?.flush?.(),
  newId: allocate = newId,
  transport = createToolTransport(),
} = {}) {
  const viewNow = () => {
    const view = getView();
    return view && !view.isDestroyed ? view : null;
  };
  const storeNow = () => (typeof store === "function" ? store() : store);

  const liveState = (id, rev) => {
    const value = storeNow()?.data?.[id]?.[rev];
    return value === undefined ? {} : value;
  };

  const targets = new Map();

  const canonical = (value) => {
    if (Array.isArray(value)) return `[${value.map(canonical).join(",")}]`;
    if (value && typeof value === "object") {
      const keys = Object.keys(value).sort();
      return `{${keys.map((key) => `${JSON.stringify(key)}:${canonical(value[key])}`).join(",")}}`;
    }
    return JSON.stringify(value) ?? "null";
  };

  function toolTarget(toolId = null, remove = null) {
    const opened = viewNow();
    const bookmark = toolId || !opened ? null : bookmarkIn(opened.state, remove);
    let onStatus = null;
    const client = createToolClient({
      transport: {
        request: (request) => transport.request(request, { onStatus: (status) => onStatus?.(status) }),
        cancel: (id) => transport.cancel(id),
      },
    });
    let pending = null;
    let candidate = null;
    let preview = null;
    let kept = false;
    let revision = null;
    let captured = null;

    const disposePreview = () => {
      preview?.host?.dispose();
      preview = null;
    };

    // The block that runs this tool, if this editor mounts it, so a revision can pause
    // it while the request and its preview are up.
    const blockNow = () => {
      const view = viewNow();
      return view && toolId ? toolBlockViews.get(view)?.get(toolId) ?? null : null;
    };

    const suspendTool = () => blockNow()?.suspend();
    const resumeTool = () => blockNow()?.resume();

    // Every way Keep can fail leaves the same trace: no proposal on screen, the block
    // and its state as they were, and an editor that answers again.
    const failKeep = (message) => {
      kept = false;
      const view = viewNow();
      disposePreview();
      if (view) clearPreview(view);
      resumeTool();
      return { status: "failed", message };
    };

    const target = {
      aiTarget: true,
      label: toolId ? "This tool" : "New tool",
      ref: toolId ? [toolId] : [],
      placeholder: toolId ? "What should change?" : "Describe the tool",
      get isConnected() {
        const view = viewNow();
        if (!view) return false;
        return toolId ? !!findBlock(view.state.doc, toolId) : true;
      },
      getBoundingClientRect() {
        const view = viewNow();
        if (!view) return emptyRect();
        if (preview) return preview.dom.getBoundingClientRect();
        const id = toolId ?? bookmark?.hostId;
        const found = id ? findBlock(view.state.doc, id) : null;
        const node = found ? view.nodeDOM(found.at) : null;
        return node && typeof node.getBoundingClientRect === "function" ? node.getBoundingClientRect() : emptyRect();
      },
      async start(comment) {
        candidate = null;
        kept = false;
        const view = viewNow();
        if (!view) return { status: "refused", code: "gone", message: GONE };
        // Read here, not imported: a page with exclude=wire must not pull the wire in.
        if (!globalThis.window?.clay?.wire) return { status: "refused", code: "no-wire", message: NO_WIRE };
        let payloadTarget = null;
        let current = null;
        if (toolId) {
          const found = findBlock(view.state.doc, toolId);
          if (found?.node.type !== schema.nodes.tool) return { status: "refused", code: "gone", message: GONE };
          revision = found.node.attrs.rev;
          payloadTarget = { toolId, rev: revision };
          // The tool must not write to its own data while the request runs or its
          // preview is shown, so it pauses and the snapshot is taken from its saved data.
          suspendTool();
          current = { artifact: found.node.attrs.artifact, state: liveState(toolId, revision) };
          captured = canonical(current.state);
        }
        pending = client.request(comment, toolContext(view.state.doc, view.dom.ownerDocument.title), payloadTarget, current);
        return {
          status: "ok",
          id: pending.requestId,
          send: (hooks = {}) => {
            onStatus = typeof hooks.onStatus === "function" ? hooks.onStatus : null;
            const done = client.wait(pending).then(
              (result) => {
                candidate = { id: toolId, rev: null, artifact: result.artifact, state: result.state };
                return { state: "done", result };
              },
              (error) => {
                resumeTool();
                return { state: "error", error: error?.message || error?.code || "the helper reported an error", code: error?.code ?? null };
              },
            );
            return { done, cancel: () => { resumeTool(); client.discard(pending); } };
          },
        };
      },
      async propose(result) {
        const view = viewNow();
        if (!view || !result) return { status: "refused", message: GONE };
        const artifact = validateArtifact(result.artifact);
        if (!artifact.ok) return { status: "refused", message: BAD_ARTIFACT };
        const held = validateState(result.state);
        if (!held.ok) return { status: "refused", message: BAD_STATE };
        candidate = { id: toolId, rev: null, artifact: result.artifact, state: result.state };
        const mounted = mountPreview(view);
        if (mounted.status !== "ready") candidate = null;
        return mounted;
      },
      async keep() {
        if (!candidate || kept) return failKeep(STALE_REVISION);
        const view = viewNow();
        if (!view) return failKeep(GONE);
        const tools = storeNow();
        if (!tools) return failKeep(NO_STORE);
        const liveChanged = () => toolId && captured !== null && canonical(liveState(toolId, revision)) !== captured;
        if (liveChanged()) return failKeep(CHANGED);
        kept = true;
        const id = candidate.id ?? allocate("b");
        const rev = allocate("rev");
        const edit = { ...candidate, id, rev };
        // Two stages: the state has to be accepted first, and only then does the block
        // appear, so a refused state leaves no block and one undo item.
        try {
          await tools.set([id, rev], edit.state);
        } catch (error) {
          return failKeep(error?.message ?? NO_STORE);
        }
        let accepted = null;
        try {
          accepted = await acceptState();
        } catch {
          return failKeep(SAVE_FAILED);
        }
        if (accepted?.ok !== true && !["saved", "clean"].includes(accepted?.state)) {
          return failKeep(accepted?.msg ?? accepted?.message ?? SAVE_FAILED);
        }
        if (toolId) {
          const found = findBlock(view.state.doc, toolId);
          if (found?.node.type !== schema.nodes.tool || found.node.attrs.rev !== revision) return failKeep(STALE_REVISION);
        } else if (!placeIn(view.state, bookmark)) {
          return failKeep(MOVED);
        }
        if (liveChanged()) return failKeep(CHANGED);
        disposePreview();
        clearPreview(view);
        if (toolId) {
          const found = findBlock(view.state.doc, toolId);
          view.dispatch(asStep(view.state.tr.setNodeMarkup(found.at, null, {
            ...found.node.attrs, artifact: edit.artifact, rev: edit.rev,
          })));
        } else if (!insertCandidate(view, edit, bookmark)) {
          return failKeep(MOVED);
        }
        // The block is in the document now: a failed save neither undoes it nor asks the
        // model again, it retries the save alone, once.
        let saved = false;
        for (let attempt = 0; attempt < 2 && !saved; attempt++) {
          try {
            const result = await acceptState();
            saved = result?.ok !== false;
          } catch {
            saved = false;
          }
        }
        if (!saved) return { status: "failed", message: UNSAVED };
        targets.delete(toolId ?? "new");
        return { status: "kept" };
      },
      discard() {
        disposePreview();
        const view = viewNow();
        if (view) clearPreview(view);
        client.discard(pending);
        resumeTool();
        targets.delete(toolId ?? "new");
      },
    };

    function mountPreview(view) {
      disposePreview();
      const host = createToolHost({
        mode: "view",
        tables: () => tablesFromDoc(view.state.doc).map(({ id, cols, rows }) => ({ id, cols, rows })),
      });
      const dom = view.dom.ownerDocument.createElement("div");
      dom.className = "blockeditor-ai-preview";
      let mounted = null;
      try {
        mounted = host.mount(dom, { id: "preview", rev: "preview", artifact: candidate.artifact, seed: candidate.state });
      } catch (error) {
        host.dispose();
        return { status: "refused", message: BAD_ARTIFACT };
      }
      if (mounted.failed) {
        host.dispose();
        return { status: "refused", message: BAD_ARTIFACT };
      }
      preview = { host, dom };
      // A revision covers the block it replaces; a new tool stands at its bookmark.
      if (toolId) {
        showPreview(view, { ids: [toolId], dom });
        return { status: "ready" };
      }
      const place = placeIn(view.state, bookmark);
      if (!place) {
        disposePreview();
        return { status: "refused", message: MOVED };
      }
      if (place.replace) showPreview(view, { ids: [place.node.attrs.id], dom });
      else showWidget(view, { at: place.insertAt, dom });
      return { status: "ready" };
    }

    if (toolId) targets.set(toolId, target);
    return target;
  }

  return {
    newTool(remove = null) {
      return toolTarget(null, remove);
    },
    // A revision target follows the block it names: the same tool asked about twice is
    // the same target, so its draft survives a closed panel.
    reviseTool(toolId) {
      const known = targets.get(toolId);
      if (known?.isConnected) return known;
      targets.delete(toolId);
      return toolTarget(toolId);
    },
  };
}
