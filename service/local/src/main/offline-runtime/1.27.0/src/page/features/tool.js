// The tool block: its node view and its CSS. Loaded the first time a document
// has or takes a tool block; it mounts through the host its caller supplies, or
// through a host it makes for an editor that has none.
import { openPageMenu } from "../chrome.js";
import { schema } from "../schema.js";
import { createToolHost } from "../tools.js";
import { toolBlockViews, toolViews } from "../tool-views.js";

export const TOOL_CSS = `
.blockeditor-tool-block { position: relative; min-height: 48px; border: 1px solid var(--blockeditor-rule); border-radius: 8px; padding: 12px; }
.blockeditor-tool-settings { position: absolute; top: 4px; inset-inline-end: 4px; border: 0; border-radius: 5px; background: var(--blockeditor-pop-bg); color: var(--blockeditor-ink-soft); font: 12px var(--blockeditor-font); padding: 5px 8px; cursor: pointer; opacity: 0; }
.blockeditor-tool-block:hover .blockeditor-tool-settings, .blockeditor-tool-block:focus-within .blockeditor-tool-settings { opacity: 1; }
[data-tool-note], [data-tool-error] { color: var(--blockeditor-danger); font-size: 0.85em; }`;

export class ToolView {
  constructor(node, view, getPos, { host, revise = null, blocks = null }) {
    this.node = node;
    this.view = view;
    this.getPos = getPos;
    this.host = host;
    this.revise = revise;
    this.blocks = blocks;
    this.showingFallback = false;
    this.suspended = false;
    const doc = view.dom.ownerDocument;
    this.dom = doc.createElement("section");
    this.dom.className = "blockeditor-tool-block";
    this.dom.dataset.type = "tool";
    this.dom.dataset.id = node.attrs.id ?? "";
    this.dom.contentEditable = "false";
    this.content = doc.createElement("div");
    this.content.className = "blockeditor-tool-content";
    this.menu = doc.createElement("button");
    this.menu.type = "button";
    this.menu.className = "blockeditor-tool-settings";
    this.menu.textContent = "Tool options";
    this.menu.setAttribute("aria-label", "Tool options");
    this.menu.addEventListener("click", () => this.openMenu());
    this.dom.append(this.content, this.menu);
    if (this.blocks && this.dom.dataset.id) this.blocks.set(this.dom.dataset.id, this);
    this.mount();
  }

  mount() {
    this.instance?.dispose();
    this.instance = null;
    if (this.suspended || this.showingFallback) this.content.innerHTML = this.node.attrs.artifact?.fallbackHtml ?? "";
    else this.instance = this.host.mount(this.content, {
      id: this.node.attrs.id,
      artifact: this.node.attrs.artifact,
      rev: this.node.attrs.rev,
      seed: this.node.attrs.state,
    });
  }

  suspend() {
    if (this.suspended) return;
    this.suspended = true;
    this.mount();
  }

  resume() {
    if (!this.suspended) return;
    this.suspended = false;
    this.mount();
  }

  openMenu() {
    const items = [];
    if (this.revise) items.push({ label: "Revise…", run: () => this.revise(this.node.attrs.id, this.node.attrs.rev) });
    items.push({ label: this.showingFallback ? "Show tool" : "Show fallback", run: () => {
      this.showingFallback = !this.showingFallback;
      this.mount();
    } });
    items.push({ label: "Delete", run: () => {
      const pos = this.getPos();
      if (pos == null || this.view.state.doc.nodeAt(pos)?.type !== schema.nodes.tool) return;
      this.view.dispatch(this.view.state.tr.delete(pos, pos + this.node.nodeSize));
    } });
    openPageMenu(this.view, this.menu, items);
  }

  update(node) {
    if (node.type !== schema.nodes.tool || node.attrs.id !== this.node.attrs.id) return false;
    const changed = node.attrs.rev !== this.node.attrs.rev || JSON.stringify(node.attrs.artifact) !== JSON.stringify(this.node.attrs.artifact);
    this.node = node;
    if (changed) {
      this.suspended = false;
      this.mount();
    }
    return true;
  }

  stopEvent(event) { return this.dom.contains(event.target); }
  ignoreMutation() { return true; }
  destroy() {
    const id = this.node.attrs.id;
    if (this.blocks?.get(id) === this) this.blocks.delete(id);
    this.instance?.dispose();
    this.instance = null;
  }
}

export function createFeature({ toolHost = null, revise = null, view = null } = {}) {
  const host = toolHost ?? createToolHost({ mode: "edit" });
  const blocks = new Map();
  if (view) {
    toolViews.set(view, host);
    toolBlockViews.set(view, blocks);
  }
  return {
    id: "tool",
    nodeViews: {
      tool: (node, editorView, getPos) => new ToolView(node, editorView, getPos, { host, revise, blocks }),
    },
    css: TOOL_CSS,
    dispose() {
      if (view && toolViews.get(view) === host) toolViews.delete(view);
      if (view && toolBlockViews.get(view) === blocks) toolBlockViews.delete(view);
      if (!toolHost) host.dispose();
    },
  };
}
