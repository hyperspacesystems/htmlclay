
// The workbook owner: the `[sheet-workbook]` element is one owned region, holding every tab and
// the commit record. A save writes each tab's table through sapjs's cached writer with the
// results the formula engine holds, so a save of 20,000 cells costs about what an empty one does.
// Everything else in the region is written as the generic serializer writes it.
//
// Every write into the workbook goes through a Grid or this owner. ClayJS hides changes inside an
// owned root from sapjs, so a write from anywhere else would never reach the Grid's mirror or the
// writer (C7 and C8 keep this rule).
//
// ClayJS's own modules are reached through `window.clay` (assembled before any plugin loads),
// never imported: a second importer moves a shared module into a chunk of its own, and every
// page then makes one more request.

export const WORKBOOK = "[sheet-workbook]";

const isTab = (node) => node.nodeType === 1 && node.localName === "section" && node.hasAttribute("item");
// A section's own children are read one by one: a selector query walks every cell under the
// section in some engines (jsdom takes 27 ms at 20,000 cells).
const childOf = (el, test) => {
  for (const child of el.children) if (test(child)) return child;
  return null;
};
const isGridTable = (el) => el.localName === "table" && el.hasAttribute("grid");
const tableIn = (section) => childOf(section, isGridTable);
const nameIn = (section) => childOf(section, (el) => el.getAttribute("bind") === "name");
const tabsIn = (root) => [...root.children].filter(isTab);

// A copy cleaned as a save cleans the page: nodes no save keeps go, and this tab's own state
// (route-active, ephemeral state, sapjs's error beacon) is left out. Null when the node
// itself is not saved.
const cleaned = (copy) => {
  const clean = window.clay?.region?.cleanSaveCopy;
  return typeof clean === "function" ? clean(copy) : copy;
};

// A node as the page's own serializer writes it for a save.
function htmlOf(node) {
  const copy = cleaned(node.cloneNode(true));
  if (!copy) return "";
  const holder = document.createElement("template");
  holder.content.append(copy);
  return holder.innerHTML;
}

// An element's start tag as the page's own serializer writes it for a save.
function startTag(el) {
  const html = cleaned(el.cloneNode(false))?.outerHTML ?? el.cloneNode(false).outerHTML;
  return html.slice(0, html.length - `</${el.localName}>`.length);
}

const commitIn = (el) => (el ? childOf(el, (child) => child.localName === "script" && child.hasAttribute("sheet-commit")) : null);

// A table's content for a merge: its bytes after one trip through the HTML parser, so the
// page's text and the writer's bytes compare in one form, with every formula's result left out.
// A result is the engine's to compute from the merged inputs, so a peer's edit on one tab that
// changes results on another is no edit of that other tab.
function parsedTable(html) {
  const holder = document.createElement("template");
  holder.innerHTML = html;
  const table = holder.content.querySelector("table");
  if (!table) return "";
  for (const cell of table.querySelectorAll("[f]")) {
    cell.removeAttribute("v");
    cell.removeAttribute("t");
    cell.removeAttribute("n");
    cell.textContent = "";
  }
  return table.outerHTML;
}

// Tabs order by data-pos, then by id, as sapjs's `byPos` orders them.
const before = (a, b) => (a.pos !== b.pos ? a.pos < b.pos : a.id < b.id);

// The tabs of a workbook element in a parsed document (a peer's page or the disk base).
function tabsOf(root) {
  const tabs = new Map();
  if (!root) return tabs;
  for (const section of tabsIn(root)) {
    const table = tableIn(section);
    tabs.set(section.getAttribute("data-id"), {
      section,
      name: nameIn(section)?.textContent ?? "",
      pos: section.getAttribute("data-pos") ?? "",
      table: table ? parsedTable(table.outerHTML) : "",
    });
  }
  return tabs;
}

// Whether two tabs (or two absences) are the same. `table` false compares names and places only:
// a tab with no unsaved edit holds no work of ours, whatever the engine painted.
function same(a, b, table) {
  if (a === null || b === null) return a === b;
  return a.name === b.name && a.pos === b.pos && (!table || a.table === b.table);
}

function createBook(root, { onRebuild, notify }) {
  let made = null;
  let writer = null;
  let revision = 0;
  let room = null;
  // Per tab: the revision of its last edit, and the last of those a save wrote. A tab holds
  // unsaved work of ours while the first is past the second.
  const editedAt = new Map();
  const savedAt = new Map();
  // The edit marks a save capture wrote, by the capture's id, made saved when that save
  // succeeds. A capture that is not a save (a download, `clay.getHTML()`) never gets an id here.
  let pending = null;
  const byCapture = new Map();
  const unsaved = (id) => (editedAt.get(id) ?? 0) > (savedAt.get(id) ?? 0);
  const watched = new Map();

  function onChange(event) {
    writer.touch(event.sheet, event);
    if (event.structure && watchTabs()) onRebuild();
    if (room || event.source === "derived") return;
    revision += 1;
    editedAt.set(event.sheet, revision);
    notify(root);
  }

  // A listener on every tab's Grid, and the tab's grid backing; a tab a step added gets both, a
  // removed tab's go, and the writer forgets both. Returns whether the tabs changed.
  function watchTabs() {
    const tables = new Set(tabsIn(root).map(tableIn).filter(Boolean));
    let changed = false;
    for (const [table, { off, id }] of watched) {
      if (tables.has(table)) continue;
      off();
      watched.delete(table);
      writer.touch(id);
      changed = true;
    }
    let backings = null;
    for (const table of tables) {
      if (watched.has(table)) continue;
      backings ??= new Map(made.workbook.sheets().map((tab) => [tab.id, tab.backing]));
      const grid = made.grid(table);
      const id = grid.sheet.id;
      watched.set(table, { off: grid.on("change", onChange), id, backing: backings.get(id) });
      writer.touch(grid.sheet.id);
      changed = true;
    }
    return changed;
  }

  function build() {
    for (const { off } of watched.values()) off();
    watched.clear();
    made = window.Sap.grid.sheets(root);
    writer = window.Sap.grid.createSheetWriter();
    watchTabs();
  }

  function tabTable({ id, backing }) {
    // The engine writes every result into the Grid's backing, so the backing's cells are the
    // saved ones; the engine's `extras` would drop the shown text of a format it cannot draw.
    return writer.table(id, backing.sheet);
  }

  function serialize({ purpose }) {
    if (!made) return htmlOf(root).replace(/^<[^>]*>|<\/[^>]*>$/g, "");
    if (room && room.isReady?.() !== false && (purpose === "save" || purpose === "sync")) return room.committedInner();
    pending = purpose === "save" ? new Map(editedAt) : null;
    let out = "";
    for (const node of root.childNodes) {
      if (!isTab(node)) {
        out += htmlOf(node);
        continue;
      }
      out += startTag(node);
      for (const part of node.childNodes) {
        const tab = part.nodeType === 1 ? watched.get(part) : undefined;
        out += tab?.backing ? tabTable(tab) : htmlOf(part);
      }
      out += `</${node.localName}>`;
    }
    return out;
  }

  // `clay:snapshot-ready` names the save capture this book just wrote for.
  function captured(captureId) {
    if (pending && captureId != null) byCapture.set(captureId, pending);
    pending = null;
  }

  function saved(captureId) {
    const marks = byCapture.get(captureId);
    if (!marks) return;
    for (const key of [...byCapture.keys()]) {
      byCapture.delete(key);
      if (key === captureId) break;
    }
    for (const [id, at] of marks) savedAt.set(id, Math.max(at, savedAt.get(id) ?? 0));
  }

  // The live tabs, in the form `tabsOf` gives a parsed document's.
  function ours() {
    const tabs = new Map();
    for (const section of tabsIn(root)) {
      const table = tableIn(section);
      const tab = table ? watched.get(table) : undefined;
      tabs.set(section.getAttribute("data-id"), {
        section,
        name: nameIn(section)?.textContent ?? "",
        pos: section.getAttribute("data-pos") ?? "",
        table: tab?.backing ? parsedTable(tabTable(tab)) : table ? parsedTable(table.outerHTML) : "",
      });
    }
    return tabs;
  }

  // Puts a section at its place among the tabs.
  function place(section) {
    const key = { id: section.getAttribute("data-id"), pos: section.getAttribute("data-pos") ?? "" };
    const others = tabsIn(root).filter((el) => el !== section);
    const next = others.find((el) => before(key, { id: el.getAttribute("data-id"), pos: el.getAttribute("data-pos") ?? "" })) ?? null;
    root.insertBefore(section, next ?? others.at(-1)?.nextSibling ?? commitIn(root));
  }

  // A clash (a tab changed here and in the version that arrived, differently) throws: ClayJS
  // then leaves the live content alone, holds autosave and shows that the page could not be
  // merged, so neither person's work is overwritten. Otherwise `after()` takes every tab the
  // peer changed and we did not, and reports whether work of ours is still unsaved.
  function merge(incoming) {
    if (!made) throw new Error("the sheet's workbook has not started");
    if (room) {
      room.heard(incoming);
      return () => false;
    }
    // Read now: live sync empties the peer's owned roots once merge returns.
    const index = [...document.querySelectorAll(WORKBOOK)].indexOf(root);
    const theirs = tabsOf(incoming.document?.querySelectorAll(WORKBOOK)[index] ?? null);
    const baseDoc = incoming.base;
    const base = baseDoc ? tabsOf(baseDoc.querySelectorAll(WORKBOOK)[index] ?? null) : null;
    // The commit record: ours stays unless the peer's changed it and ours did not.
    const theirRoot = incoming.document?.querySelectorAll(WORKBOOK)[index] ?? null;
    const theirCommit = commitIn(theirRoot);
    const commitText = (el) => (el ? htmlOf(el) : null);
    let takeCommit = false;
    if (baseDoc) {
      const was = commitText(commitIn(baseDoc.querySelectorAll(WORKBOOK)[index] ?? null));
      const here = commitText(commitIn(root));
      const there = commitText(theirCommit);
      if (there !== was && here !== there) {
        if (here !== was) throw new Error("the sheet's commit record changed here and in the version that arrived");
        takeCommit = true;
      }
    }
    const mine = ours();
    const take = [];
    const agreed = [];
    let diverged = false;
    for (const id of new Set([...mine.keys(), ...theirs.keys(), ...(base ? base.keys() : [])])) {
      const here = mine.get(id) ?? null;
      const there = theirs.get(id) ?? null;
      const was = base ? base.get(id) ?? null : here;
      // With no base, only an edit no save has written yet is work of ours.
      const changedHere = base === null ? unsaved(id) : !same(here, was, unsaved(id));
      const changedThere = !same(there, was, true);
      if (!changedThere) {
        // Work of ours the peer does not hold is still unsaved: the page stays dirty.
        if (changedHere) diverged = true;
        continue;
      }
      if (changedHere) {
        if (!same(here, there, true)) throw new Error(`the sheet's tab "${id}" changed here and in the version that arrived`);
        agreed.push(id);
        continue;
      }
      take.push({ id, here, there });
    }
    return () => {
      for (const id of agreed) savedAt.set(id, editedAt.get(id) ?? 0);
      replaceTabs(take);
      if (takeCommit) {
        const ourCommit = commitIn(root);
        if (!theirCommit) ourCommit?.remove();
        else if (ourCommit) ourCommit.replaceWith(document.importNode(theirCommit, true));
        else root.append(document.importNode(theirCommit, true));
      }
      // Moved inside after(): live sync records it as the merge's, never as an edit.
      if (takeCommit && take.length === 0) revision += 1;
      if (take.length > 0) {
        build();
        revision += 1;
        onRebuild();
        window.Sap.flush?.();
      }
      return diverged;
    };
  }

  // Every tab the peer changed and we did not becomes its: a tab removed, a tab added, or a tab
  // renamed, given the peer's table, or moved to its place. Nothing counts as an edit of ours.
  function replaceTabs(take) {
    for (const { id, here, there } of take) {
      editedAt.delete(id);
      savedAt.delete(id);
      if (there === null) {
        here.section.remove();
      } else if (here === null) {
        place(document.importNode(there.section, true));
      } else {
        if (here.name !== there.name) nameIn(here.section).textContent = there.name;
        if (here.table !== there.table) tableIn(here.section).replaceWith(document.importNode(tableIn(there.section), true));
        if (here.pos !== there.pos) {
          here.section.setAttribute("data-pos", there.pos);
          place(here.section);
        }
      }
    }
  }

  return {
    root,
    build,
    serialize,
    merge,
    // A live room owns this workbook's bytes while bound: no Grid change moves the page's
    // revision, ClayJS's page save carries the committed region as the file holds it, and a
    // merge from live sync updates that region without touching the tabs (the room delivers them).
    bind(binding) {
      room = binding;
    },
    unbind({ edited = [] } = {}) {
      room = null;
      if (edited.length > 0) {
        revision += 1;
        const ids = edited.filter(Boolean);
        for (const id of ids.length > 0 ? ids : tabsIn(root).map((el) => el.getAttribute("data-id")).filter(Boolean)) {
          editedAt.set(id, revision);
        }
        notify(root);
      }
    },
    get room() {
      return room;
    },
    // After a live room adds, removes, renames or moves tabs directly on the workbook, mount any
    // new Grid, unhook removed ones, drop their cached HTML and let the shell and UI rebuild.
    syncTabs(ids = []) {
      for (const id of ids) writer?.touch(id);
      const changed = watchTabs();
      if (changed || ids.length > 0) onRebuild?.();
    },
    // The live tabs become `theirs` (a parsed workbook's tabs, as `merge` takes them).
    replaceTabs,
    // A chart drawn on open that differs from its saved drawing: one change for the page, so the
    // next save writes it, never an edit of a tab (a merge must not count it as ours).
    redrawn() {
      if (room) return;
      revision += 1;
      notify(root);
    },
    captured,
    saved,
    get built() {
      return made !== null;
    },
    get revision() {
      return revision;
    },
    get sheets() {
      return made;
    },
  };
}

export function ownWorkbooks({ onRebuild = () => {} } = {}) {
  const books = new Map();
  let handle = null;
  const notify = (root) => handle?.notify(root);
  const bookOf = (root) => {
    let book = books.get(root);
    if (!book) {
      book = createBook(root, { onRebuild, notify });
      books.set(root, book);
    }
    return book;
  };
  for (const root of document.querySelectorAll(WORKBOOK)) bookOf(root);
  handle = window.clay.region.own(WORKBOOK, {
    serialize: (root, options = {}) => bookOf(root).serialize({ purpose: options.purpose ?? "save" }),
    revision: (root) => bookOf(root).revision,
    merge: (incoming) => {
      const steps = [...books.values()].map((book) => book.merge(incoming));
      return () => steps.map((after) => after()).some(Boolean);
    },
  });
  document.addEventListener("clay:snapshot-ready", (event) => {
    if (!event.detail?.forSave) return;
    for (const book of books.values()) book.captured(event.detail.captureId);
  });
  document.addEventListener("clay:save-saved", (event) => {
    for (const book of books.values()) book.saved(event.detail?.captureId);
  });

  // The engine is sapjs's grid plugin, which derive.js installs. The engine's own wait has a
  // time limit, so a slow download is joined here: the mount waits for this ready. A grid that
  // could not load at all (offline) is tried again when the connection comes back.
  const buildAll = async () => {
    // derive.js could not load the engine either (offline from the start).
    if (!window.Sap?.plugins) await import("../vendor/sap-core.vendor.js");
    if (typeof window.Sap?.grid !== "function") await window.Sap?.plugins?.load?.("grid");
    if (typeof window.Sap?.grid?.sheets !== "function") return false;
    let late = false;
    for (const book of books.values()) {
      if (book.built) continue;
      book.build();
      late = true;
    }
    return late;
  };
  // derive.js is loaded before this module (PLUGIN_ORDER); a dynamic import reaches the same
  // module without joining it to this one's chunk.
  const engineReady = import("../plugins/derive.js").then((derive) => derive.ready);
  const ready = engineReady.then(buildAll).then(
    () => {},
    (error) => {
      console.error("clayjs: the sheet's workbook could not start", error);
      window.addEventListener("online", function retry() {
        buildAll().then((late) => {
          if ([...books.values()].some((book) => !book.built)) return;
          window.removeEventListener("online", retry);
          if (late) onRebuild();
        }, () => {});
      });
    },
  );
  // A chart the lazy charts.js drew that differs from the drawing the file holds: the page has a
  // change to save, and no tab is edited by it.
  const redrawn = (table) => books.get(table.closest(WORKBOOK))?.redrawn();
  return { books, ready, redrawn };
}
