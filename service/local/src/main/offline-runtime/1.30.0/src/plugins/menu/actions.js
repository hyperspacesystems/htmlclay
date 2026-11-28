// The menu's editing items: Undo and Redo, Ask AI to improve this page, and Download a
// copy. Each one calls the service that already does the job and keeps no second copy
// of its state.
import { modeRefusalText } from "../../core/mode-refusal.js";
import { downloadHtml, documentName, portableHtml } from "../../core/download.js";
import { keys, revealLabel } from "./preferences.js";
import { hostMeta } from "../../core/host-meta.js";
import { newDocumentLabels } from "./new-document-item.js";

const RESUME_KEY = "menu-resume";
const READY_MS = 5000;
const PAGE_READY_MS = 8000;

const clay = () => window.clay || {};
const session = () => clay().storage && clay().storage.session;
const pageWants = (name) => {
  const names = clay().editPlugins;
  return Array.isArray(names) && names.includes(name);
};

// An edit-only plugin attaches after the mode change commits, so the item waits for it
// briefly instead of reporting it missing.
function whenPresent(get, ms = READY_MS) {
  return new Promise((resolve) => {
    const start = Date.now();
    const tick = () => {
      const value = get();
      if (value || Date.now() - start >= ms) resolve(value || null);
      else setTimeout(tick, 50);
    };
    tick();
  });
}

// A reload entry runs before the page session it reloaded into has finished preparing,
// so the surface it needs is not there yet. Waiting here is bounded: a page that never
// gets one ends at the same unavailable message a missing helper does.
function waitForPageAi(ms = PAGE_READY_MS) {
  const page = clay().blockeditor;
  if (!page) return Promise.resolve(true);
  const start = Date.now();
  return new Promise((resolve) => {
    let done = false;
    const finish = (ok) => { if (!done) { done = true; clearTimeout(timer); resolve(ok); } };
    const tick = () => {
      if (done) return;
      if (clay().blockeditor?.ai) return finish(true);
      if (Date.now() - start >= ms) return finish(false);
      setTimeout(tick, 50);
    };
    const timer = setTimeout(() => finish(false), ms);
    Promise.resolve(page.ready).catch(() => null).then(tick, tick);
  });
}

export function attachActions({ addItem, note, currentMode }) {
  const resumers = new Map();
  // A receipt for an item that registers later, once its host has answered.
  let pendingReceipt = null;

  // Runs `run` in edit mode, entering it first when the page is in view. A reload entry
  // leaves a one-shot receipt in session storage so the action runs once the page is
  // back; a refused entry says why and runs nothing.
  async function inEditMode(action, run) {
    if (currentMode() === "edit") return run();
    const storage = session();
    if (storage) storage.set(RESUME_KEY, action);
    let result;
    try {
      result = await clay().setMode?.("edit");
    } catch (err) {
      result = { ok: false, message: err && err.message };
    }
    if (result && result.ok !== false && result.reloading) return;
    if (storage) storage.remove(RESUME_KEY);
    if (!result || result.ok === false) {
      const text = result ? modeRefusalText(result) : "Couldn't switch modes.";
      if (text) note(text);
      return;
    }
    return run();
  }

  function resumable(action, run) {
    resumers.set(action, run);
    if (pendingReceipt === action) {
      pendingReceipt = null;
      if (currentMode() === "edit") run();
    }
    return () => inEditMode(action, run);
  }

  let history = { canUndo: false, canRedo: false };
  let heard = false;

  // A Document keeps its history in its own editor, so the pair reads the page's service
  // there and hyper-undo everywhere else. Reading it at use time means an editor that
  // closed after the item was drawn takes the press with it.
  const historySource = () => (pageWants("blockeditor") && clay().blockeditor?.history ? clay().blockeditor.history : pageWants("undo") ? clay().undo : null);

  function renderUndo() {
    if (!pageWants("blockeditor") && !pageWants("undo")) return;
    const live = currentMode() === "edit" && historySource();
    // The menu loads late, so edits made before it are read from the service itself.
    if (!heard && live && typeof live.canUndo === "boolean") history = { canUndo: live.canUndo, canRedo: !!live.canRedo };
    addItem({
      id: "clay.undo",
      pair: [
        {
          id: "clay.undo.undo",
          label: "Undo",
          keys: keys("⌘Z", "Ctrl+Z"),
          disabled: !(live && history.canUndo),
          run: () => historySource()?.undo(),
        },
        {
          id: "clay.undo.redo",
          label: "Redo",
          keys: keys("⌘⇧Z", "Ctrl+Shift+Z"),
          disabled: !(live && history.canRedo),
          run: () => historySource()?.redo(),
        },
      ],
    });
  }

  document.addEventListener("clay:undo-change", (event) => {
    const detail = event.detail || {};
    heard = true;
    history = { canUndo: !!detail.canUndo, canRedo: !!detail.canRedo };
    renderUndo();
  });
  document.addEventListener("clay:modechange", renderUndo);
  renderUndo();

  // Only a host that lists an ai-edit helper, ready or turned off, gets the item: on
  // hyperclay.com someone with an edit link who is not listed would otherwise see an
  // item that can only say it is unavailable. In view mode the plugin is not loaded
  // yet, so there is nobody to ask and the item waits for the click, as it always has.
  if (pageWants("ai-edit")) {
    (async () => {
      // ai-edit loads late, after the editor is ready, so it can attach after this menu.
      // Ask for it where it lands, as the item's own run does, rather than where the
      // menu boots: the item is drawn once there is a plugin to say what the host lists.
      const loaded = clay().aiEdit || (currentMode() === "edit" ? await whenPresent(() => clay().aiEdit) : null);
      if (loaded?.available) {
        if (!(await loaded.available())) return;
      } else if (!loaded && pageWants("blockeditor")) {
        // In view mode a Block document has no plugin to ask, so the host's own
        // discovery says whether an ai-edit helper exists at all.
        const meta = await hostMeta();
        const listed = Array.isArray(meta.document?.helpers) && meta.document.helpers.some(
          (helper) => helper && helper.name === "ai-edit" && (helper.state === "ready" || helper.state === "unavailable"),
        );
        if (!listed) return;
      }
      addItem({
        id: "clay.ai",
        label: "Ask AI to improve this page",
        keys: keys("⌘J", "Ctrl+J"),
        run: resumable("ai", async () => {
          const ai = await whenPresent(() => clay().aiEdit);
          const ready = ai && (pageWants("blockeditor") || !!clay().blockeditor) ? await waitForPageAi() : !!ai;
          const opened = ready ? await ai.pick() : false;
          if (!opened) note("Ask AI isn't available on this page right now.");
        }),
      });
    })();
  }

  if (pageWants("cms") && clay().cms?.hasRules?.(document)) {
    addItem({
      id: "clay.cms",
      label: "Open the CMS (Edit content)",
      run: resumable("cms", async () => {
        const cms = await whenPresent(() => clay().cms);
        let result = null;
        try {
          result = cms ? await cms.open() : { ok: false };
        } catch {
          result = { ok: false };
        }
        if (result && result.ok === false) note("Couldn't open the CMS. Try again.");
      }),
    });
  }

  async function exportCurrentHtml(filename) {
    let html = null;
    try {
      if (typeof clay().getHTML === "function") {
        html = clay().getHTML({ purpose: "export" });
      } else {
        // Clay-Source asks a local host for the file's own bytes, not the copy it serves with
        // its runtime addresses and page state written in.
        const response = await fetch(location.href, { cache: "no-store", credentials: "same-origin", headers: { "Clay-Source": "1" } });
        if (response.ok) html = await response.text();
      }
    } catch {}
    html = portableHtml(html);
    if (typeof html !== "string" || !(await downloadHtml({ html, name: filename }))) {
      note("Couldn't build the download. Try again.");
    }
  }

  // The copy is what a save would send now, unsaved edits included. A reader has no
  // unsaved edits, so the saved file is the same document.
  addItem({
    id: "clay.download",
    label: "Download a copy",
    run: () => exportCurrentHtml(`${documentName()}.html`),
  });

  // What the host offers for this document. A page opens in a new tab so this one keeps
  // whatever is unsaved; anything else the host does itself when asked, and may answer
  // with a page to open (a duplicate) or a reason it could not.
  async function askHost(url) {
    let body = null;
    try {
      const response = await fetch(url, { method: "POST", credentials: "same-origin", headers: { "Document-URL": location.href } });
      body = await response.json().catch(() => null);
      if (response.ok && body?.ok !== false) {
        if (typeof body?.href === "string") window.open(body.href, "_blank", "noopener");
        return;
      }
    } catch {}
    note(typeof body?.message === "string" && body.message ? body.message : "Couldn't do that. Try again.");
  }

  const hostItem = (id, label, action) => {
    if (action?.href) addItem({ id, label, href: action.href });
    else if (action?.post) addItem({ id, label, run: () => askHost(action.post) });
  };

  const duplicateLabel = () => newDocumentLabels()?.duplicateLabel || "Duplicate as a new page";

  import("../../core/host-meta.js")
    .then(({ hostActions }) => hostActions())
    .then((host) => {
      hostItem("clay.code", "Open the code editor", host.codeEditor);
      if (host.duplicate?.href || host.duplicate?.post) {
        hostItem("clay.duplicate", duplicateLabel(), host.duplicate);
      } else if (newDocumentLabels()) {
        addItem({
          id: "clay.duplicate",
          label: duplicateLabel(),
          run: () => exportCurrentHtml(`${documentName()}-copy.html`),
        });
      }
      hostItem("clay.export", "Export as zip", host.export);
      hostItem("clay.reveal", revealLabel(), host.reveal);
      hostItem("clay.versions", "Version history…", host.versions);
    })
    .catch(() => {});

  const storage = session();
  const pending = storage && storage.get(RESUME_KEY);
  if (pending) {
    storage.remove(RESUME_KEY);
    const run = resumers.get(pending);
    if (run && currentMode() === "edit") run();
    else if (!run) pendingReceipt = pending;
  }

  return { inEditMode, resumable, refresh: renderUndo };
}
