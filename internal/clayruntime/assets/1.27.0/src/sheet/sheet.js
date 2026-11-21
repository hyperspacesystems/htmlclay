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
const originals = new Map();
const REPAIR_BANNER = "Repair this sheet to edit it with others.";
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

function leaveRoom() {
  if (!joined) return;
  const { book, binding } = joined;
  joined = null;
  binding.stop();
  unwrapGrids();
  if (gridsOf(book).some((grid) => grid.issues?.length)) showRepairBanner(book);
}

function showRepairBanner(book) {
  ui?.notice?.(REPAIR_BANNER, {
    label: "Repair",
    async onAction() {
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
  const [, book] = [...books.books.entries()][0] ?? [];
  if (!book || typeof window.clay?.liveRoom !== "function") return null;
  if (gridsOf(book).some((grid) => grid.issues?.length)) {
    showRepairBanner(book);
    return null;
  }
  const { createRoomBinding } = await import("./room.js");
  const binding = createRoomBinding(book, {
    notice: (text, action) => ui?.notice?.(text, action),
    onLeave: () => leaveRoom(),
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
  return binding;
}
