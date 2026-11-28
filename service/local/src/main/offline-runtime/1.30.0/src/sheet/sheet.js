import { sheetHooks, startShell } from "./shell.js";
import { ownWorkbooks } from "./workbook.js";
import { startInterface } from "./interface.js";

// The sheet editor (`?plugins=sheet`, edit mode): the row window (C5a), the workbook owner that
// saves every tab through sapjs's cached writer (C5b), and the interface (C6). Undo and paste
// land in C7, the other editing commands in C8, the optional features and their loader in C11.
const shell = startShell();
let ui = null;
let mounted = false;
// While the first workbook is in its document's live room: the binding, and each Grid's original
// dispatch and repair methods before the room wrapped them.
let joined = null;
let joining = null;
let roomUI = null;
const originals = new Map();
const REPAIR_BANNER = "This sheet's markup has errors and is read-only until repaired.";
const RELOAD_BANNER = "This sheet was updated with a newer version. Reload to keep editing.";
const RELOAD_UNSAVED_BANNER = "This sheet was updated with a newer version. Download your copy or reload to keep editing.";
const books = ownWorkbooks({
  onRebuild: () => {
    shell.restart();
    wrapGrids();
    if (ui) ui.restart();
    else if (mounted) ui = startInterface({ shell });
  },
});
sheetHooks.chartRedrawn = (table) => books.redrawn(table);
sheetHooks.bookOf = (root) => books.books.get(root) ?? null;
sheetHooks.onGridRefused = (root) => {
  const book = books.books.get(root);
  if (!book || !gridsOf(book).some((grid) => grid.issues?.length)) return false;
  showRepairBanner(book);
  return true;
};

export const sheet = {
  windows: shell.windows,
  books: books.books,
  get ui() {
    return ui;
  },
  get room() {
    return joined?.binding ?? null;
  },
  leaveRoom() {
    leaveRoom();
  },
};

// The mount waits for this: the workbook's Grids exist on one engine before the first pass. The
// interface starts after the mount, on the Grids the page holds then.
export const ready = books.ready;
Promise.resolve(window.clay?.ready)
  .then(() => {
    mounted = true;
    return shell.followed;
  })
  .then(() => {
    ui = startInterface({ shell });
    return joinRooms();
  })
  .catch((error) => console.error("clayjs: the sheet interface could not start", error));
export default sheet;

// Every tab's Grid: the same objects the plugin keeps for each `<table grid>`.
const gridsOf = (book) => [...book.root.querySelectorAll(":scope > section[item] > table[grid]")].map((table) => window.Sap.grid(table));

// The Grids of the joined workbook take the room's dispatch, once each, and block `repair()`
// while joined (plan c10 section 6). A tab a step added is wrapped at the next rebuild.
function wrapGrids() {
  if (!joined) return;
  const live = new Set(gridsOf(joined.book));
  for (const grid of [...originals.keys()]) {
    if (!live.has(grid)) originals.delete(grid);
  }
  for (const grid of live) {
    if (originals.has(grid)) continue;
    originals.set(grid, { dispatch: grid.dispatch, repair: grid.repair });
    grid.dispatch = joined.binding.dispatchFor(grid, grid.dispatch.bind(grid));
    grid.repair = () => {
      throw new Error("clayjs: grid.repair() is not available while joined to a live room");
    };
  }
}

function unwrapGrids() {
  for (const [grid, saved] of originals) {
    grid.dispatch = saved.dispatch;
    grid.repair = saved.repair;
  }
  originals.clear();
}

function teardownRoomUI() {
  if (!roomUI) return;
  const { chip, panel, inst } = roomUI;
  roomUI = null;
  try { chip?.destroy?.(); } catch {}
  try { panel?.destroy?.(); } catch {}
  inst?.chrome?.setRoomChip?.(null);
  inst?.chrome?.setRoomMenu?.([]);
  inst?.setConflicts?.([]);
}

async function downloadSheetCopy(book, payload = null) {
  const base =
    typeof window.clay?.getHTML === "function"
      ? window.clay.getHTML({ purpose: "export" })
      : typeof document?.documentElement?.outerHTML === "string"
        ? `<!DOCTYPE html>\n${document.documentElement.outerHTML}`
        : book.serialize({ purpose: "export" });
  const codec = window.Sap?.grid?.roomCodec;
  const drawings =
    typeof window.Sap?.grid?.drawingsIn === "function"
      ? window.Sap.grid.drawingsIn(base)
      : undefined;
  const enrichInner = (rawInner) => {
    if (!codec || typeof codec.read !== "function" || typeof codec.write !== "function") return rawInner;
    try {
      const { doc, record } = codec.read(rawInner);
      const extras = {
        ...(typeof codec.results === "function" ? { result: codec.results(doc) } : {}),
        ...(drawings ? { drawing: drawings } : {}),
      };
      return codec.write(doc, record, extras);
    } catch {
      return rawInner;
    }
  };
  let html = base;
  let body = typeof payload?.body === "string" ? payload.body : null;
  if (!body && payload?.preview && typeof codec?.write === "function") {
    try {
      const extras = {
        ...(typeof codec.results === "function" ? { result: codec.results(payload.preview) } : {}),
        ...(drawings ? { drawing: drawings } : {}),
      };
      body = codec.write(payload.preview, {}, extras);
    } catch {}
  }
  if (typeof body === "string") {
    const isFullDoc = /^\s*(?:<!--[\s\S]*?-->\s*)*(?:<!doctype\b|<html\b)/i.test(body);
    if (isFullDoc) {
      html = body;
    } else {
      const enriched = enrichInner(body);
      const { findRegion, spliceRegion } = await import("../live-room/region.js");
      const region = findRegion(base);
      html = region
        ? spliceRegion(base, region, enriched)
        : `<!DOCTYPE html>\n<html><body><div sheet-workbook>${enriched}</div></body></html>`;
    }
  }
  const name = payload?.id ? `recovered-${payload.id}.html` : `${(document.title || "sheet").replace(/[^\w.-]+/g, "-")}.html`;
  if (typeof Blob === "undefined" || typeof URL?.createObjectURL !== "function") return;
  const blob = new Blob([html], { type: "text/html;charset=utf-8" });
  const href = URL.createObjectURL(blob);
  const a = document.createElement("a");
  a.href = href;
  a.download = name;
  a.click();
  setTimeout(() => URL.revokeObjectURL?.(href), 1000);
}

async function mountRoomUI(book, binding, room) {
  const inst = ui?.of?.(book.root);
  if (!inst) return;
  try {
    const { createSaveChip } = await import("../live-room/ui/save-chip.js");
    if (joined?.binding !== binding) return;
    const onDownload = (payload) => downloadSheetCopy(book, payload);
    let panel = null;
    let panelPromise = null;
    const openRecovery = async () => {
      if (joined?.binding !== binding) return;
      if (!panel) {
        panelPromise ??= import("../live-room/ui/recovery-panel.js").then(({ createRecoveryPanel }) => {
          if (joined?.binding !== binding) return null;
          const created = createRecoveryPanel({
            room,
            codec: window.Sap.grid.roomCodec,
            onReveal: (target) => inst.reveal(target),
            onDownload,
            onConflictsHighlight: (targets) => inst.setConflicts(targets),
            theme: inst.chrome.theme,
          });
          if (joined?.binding !== binding) {
            created.destroy();
            return null;
          }
          panel = created;
          if (roomUI) roomUI.panel = panel;
          return panel;
        }).catch((err) => {
          panelPromise = null;
          throw err;
        });
        const readyPanel = await panelPromise;
        if (!readyPanel || joined?.binding !== binding) return;
      }
      panel.open();
    };
    const chip = createSaveChip({
      room,
      onOpenRecovery: openRecovery,
      onDownload,
      theme: inst.chrome.theme,
    });
    roomUI = { chip, panel, inst };
    inst.chrome.setRoomChip(chip.el);
    inst.chrome.setRoomMenu([{ id: "recovered-edits", label: "Recovered edits", onSelect: openRecovery }]);
  } catch (error) {
    console.error("clayjs: could not mount live room UI", error);
  }
}

function showReloadBanner(book, { hasUnsaved = false } = {}) {
  ui?.notice?.(hasUnsaved ? RELOAD_UNSAVED_BANNER : RELOAD_BANNER, {
    label: "Reload",
    onAction: () => window.location?.reload?.(),
    ...(hasUnsaved
      ? {
          secondary: {
            label: "Download copy",
            onAction: () => downloadSheetCopy(book),
          },
        }
      : {}),
  });
}

function leaveRoom(reason = null, detail = null) {
  if (!joined) return;
  const { book, binding } = joined;
  joined = null;
  teardownRoomUI();
  binding.stop();
  unwrapGrids();
  if (reason === "codec-mismatch") {
    showReloadBanner(book, detail ?? {});
    return;
  }
  if (gridsOf(book).some((grid) => grid.issues?.length)) showRepairBanner(book);
}

function showRepairBanner(book) {
  ui?.notice?.(REPAIR_BANNER, {
    label: "Repair sheet",
    async onAction() {
      if (joined && joined.book === book) leaveRoom();
      for (const grid of gridsOf(book)) {
        if (grid.issues?.length) grid.repair();
      }
      ui?.hideNotice?.();
      if (typeof window.clay?.save === "function") {
        const res = await window.clay.save();
        const failed = res === false || (res && typeof res === "object" && res.msgType && res.msgType !== "saved");
        if (failed) {
          showRepairBanner(book);
          return;
        }
      }
      if (!gridsOf(book).some((grid) => grid.issues?.length)) await joinRooms();
    },
  });
}

// A sheet in edit mode joins its document's live room when the host has one: the first
// workbook only, and only when no tab has sheet issues (those are repaired first, outside a room).
export async function joinRooms() {
  if (joined) return joined.binding;
  if (joining) return joining;
  joining = (async () => {
    const [, book] = [...books.books.entries()][0] ?? [];
    if (!book || typeof window.clay?.liveRoom !== "function") return null;
    if (gridsOf(book).some((grid) => grid.issues?.length)) {
      showRepairBanner(book);
      return null;
    }
    const { createRoomBinding } = await import("./room.js");
    const binding = createRoomBinding(book, {
      notice: (text, action) => ui?.notice?.(text, action),
      onLeave: (reason, detail) => leaveRoom(reason, detail),
      reveal: (target) => ui?.reveal?.(target) ?? false,
      setConflicts: (targets) => ui?.setConflicts?.(targets),
    });
    const room = window.clay.liveRoom("sheet", { codec: window.Sap.grid.roomCodec, page: binding.page });
    joined = { book, binding };
    wrapGrids();
    const started = await binding.start(room).then(() => true, () => false);
    if (!started || !book.room || room.status.mode === "unavailable" || room.status.mode === "closed") {
      leaveRoom();
      return null;
    }
    mountRoomUI(book, binding, room);
    return binding;
  })().finally(() => {
    joining = null;
  });
  return joining;
}
