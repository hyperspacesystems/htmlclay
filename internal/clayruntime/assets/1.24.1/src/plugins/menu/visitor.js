import { createShell } from "./shell.js";
import { MENU_SAFETY, SHELL_BASE, SHELL_FLOATING, MENU_SKIN, MENU_TOUCH } from "./shell-style.js";
import { activeLook, labelOf, looks, nameOf, paintLook } from "./looks.js";
import { keys } from "./preferences.js";

// The page's looks for a visitor: painted and readable through clay.variants, with no row to switch them.
function publishLooks() {
  const announce = () => {
    const name = activeLook() && nameOf(activeLook());
    document.dispatchEvent(new CustomEvent("clay:variant-change", { detail: { active: name, shown: name } }));
  };
  const choose = (name) => {
    if (!looks().some((style) => nameOf(style) === name)) return { ok: false, code: "unknown-variant" };
    document.documentElement.setAttribute("look", name);
    paintLook(name);
    window.clay?.save?.soon?.();
    announce();
    return { ok: true };
  };
  if (looks().length >= 2) {
    window.clay.variants = {
      list: () => looks().map((style) => ({ name: nameOf(style), label: labelOf(style), active: style === activeLook() })),
      get active() {
        return activeLook() && nameOf(activeLook());
      },
      get shown() {
        return activeLook() && nameOf(activeLook());
      },
      preview: choose,
      choose,
      keep: () => Promise.resolve({ ok: false, code: "not-editing" }),
    };
    document.addEventListener("clay:drafts-restored", () => {
      announce();
    });
  }
}

// A visitor's settings gear, not the Claygoo: a page's own clay-menu-icon still replaces it.
const GEAR = '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linejoin="round" aria-hidden="true"><path d="M12 9a3 3 0 1 1 0 6 3 3 0 0 1 0-6M10 3h4l1.5 3 3.5-.5 2 4-2 2.5 2 2.5-2 4-3.5-.5-1.5 3h-4l-1.5-3-3.5.5-2-4 2-2.5-2-2.5 2-4 3.5.5Z"/></svg>';

// The page's history: a Document's own editor, else hyper-undo. Read at use time, as the owner menu does.
const historySource = () => window.clay?.blockeditor?.history ?? window.clay?.undo ?? null;

function attachUndo({ addItem, beforeOpen }) {
  let history = null;
  const draw = () => {
    const live = historySource();
    if (!live) return;
    const state = history ?? { canUndo: !!live.canUndo, canRedo: !!live.canRedo };
    addItem({
      id: "clay.undo",
      pair: [
        { id: "clay.undo.undo", label: "Undo", keys: keys("⌘Z", "Ctrl+Z"), disabled: !state.canUndo, run: () => historySource()?.undo() },
        { id: "clay.undo.redo", label: "Redo", keys: keys("⌘⇧Z", "Ctrl+Shift+Z"), disabled: !state.canRedo, run: () => historySource()?.redo() },
      ],
    });
  };
  document.addEventListener("clay:undo-change", (event) => {
    history = { canUndo: !!event.detail?.canUndo, canRedo: !!event.detail?.canRedo };
    draw();
  });
  draw();
  // The undo plugin can arrive after the menu and says nothing until the first edit, so the
  // pair is drawn again each time the menu opens.
  beforeOpen.push(draw);
}

// Hyperclay's copy page for this document, like the download: it asks a signed-out visitor to sign in.
function copyUrl(loc = window.location) {
  const path = loc.pathname.replace(/\/+$/, "") || "/";
  return `/_/copy${path === "/" ? "" : path}`;
}

// A visitor's menu holds exactly Undo, Redo, Download copy and Make your own copy (David, Nov 11):
// no theme row, no page items, no Start over.
export function start() {
  if (document.querySelector('clay-editor-ui[data-clay-ui="menu"]')) return;
  const shell = createShell({ sections: [["clay.undo"], null], icon: GEAR, label: "Settings" });
  const { embedded, skin, addItem, removeItem, open, close, note, goo, beforeOpen } = shell;
  shell.mount(embedded ? [MENU_SAFETY, SHELL_BASE, skin ?? MENU_SKIN, MENU_TOUCH] : [MENU_SAFETY, SHELL_BASE, SHELL_FLOATING, MENU_SKIN, MENU_TOUCH]);
  publishLooks();
  attachUndo({ addItem, beforeOpen });
  addItem({ id: "clay.download", label: "Download copy", run: () => window.clay.visitorDrafts.download() });
  addItem({ id: "clay.copy", label: "Make your own copy", href: copyUrl() });
  window.clay.menu = { addItem, removeItem, open, close, note, embedded, button: goo };
  document.dispatchEvent(new CustomEvent("clay:menu-ready"));
}
