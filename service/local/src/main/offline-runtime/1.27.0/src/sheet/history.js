import { a1, colIndex, rowIndex, tabTable } from "./commit.js";

const LIMIT = 200;
const TAB_ACTIONS = new Set(["insertSheet", "deleteSheet", "renameSheet", "moveSheet"]);
const same = (a, b) => JSON.stringify(a ?? null) === JSON.stringify(b ?? null);

function actionCoords(action) {
  if (!action || typeof action !== "object") return [];
  if (action.t === "batch" && Array.isArray(action.actions)) return action.actions.flatMap(actionCoords);
  if (action.t === "merge" && Array.isArray(action.from)) return [{ row: action.from[0], col: action.from[1] }];
  if (action.t === "unmerge" && action.row && action.col) return [{ row: action.row, col: action.col }];
  return [];
}

function collectCoords(events) {
  const removedRows = new Set();
  const removedCols = new Set();
  const seen = new Map();
  for (const event of events) {
    for (const id of event.rows?.removed ?? []) removedRows.add(id);
    for (const id of event.cols?.removed ?? []) removedCols.add(id);
    if (event.action?.t === "sort") continue;
    for (const { row, col } of [...(event.cells ?? []), ...actionCoords(event.action)]) {
      if (!row || !col) continue;
      seen.set(`${row}\u0000${col}`, { row, col });
    }
  }
  return [...seen.values()].filter(({ row, col }) => !removedRows.has(row) && !removedCols.has(col));
}

function sortCorners(action, maps) {
  if (!Array.isArray(action.cols)) return [];
  const byRow = (a, b) => maps.rows.get(a).r - maps.rows.get(b).r;
  const byCol = (a, b) => maps.cols.get(a).c - maps.cols.get(b).c;
  const rows = (action.rows ?? []).filter((id) => maps.rows.has(id)).sort(byRow);
  const cols = action.cols.filter((id) => maps.cols.has(id)).sort(byCol);
  if (rows.length === 0 || cols.length === 0) return [];
  return [{ row: rows[0], col: cols[0] }, { row: rows.at(-1), col: cols.at(-1) }];
}

function replayCoords(events, maps) {
  const removedRows = new Set();
  const removedCols = new Set();
  const seen = new Map();
  for (const event of events) {
    for (const id of event.rows?.removed ?? []) removedRows.add(id);
    for (const id of event.cols?.removed ?? []) removedCols.add(id);
    if (event.action?.t === "sort") {
      for (const corner of sortCorners(event.action, maps)) seen.set(`${corner.row}\u0000${corner.col}`, corner);
      continue;
    }
    for (const { row, col } of [...(event.cells ?? []), ...actionCoords(event.action)]) {
      if (!row || !col) continue;
      seen.set(`${row}\u0000${col}`, { row, col });
    }
  }
  return [...seen.values()].filter(({ row, col }) => !removedRows.has(row) && !removedCols.has(col));
}

function tableMaps(table) {
  const rows = new Map();
  const cols = new Map();
  const colIds = [];
  if (!table) return { rows, cols, colIds };
  let r = 0;
  for (const section of [table.tHead, ...table.tBodies]) {
    if (!section) continue;
    const frozen = section === table.tHead;
    for (let tr = section.firstElementChild; tr !== null; tr = tr.nextElementSibling) {
      if (tr.localName !== "tr") continue;
      const id = tr.getAttribute("data-id");
      if (id) rows.set(id, { r, tr, frozen });
      r += 1;
    }
  }
  let group = null;
  for (let node = table.firstElementChild; node !== null; node = node.nextElementSibling) {
    if (node.localName === "colgroup") {
      group = node;
      break;
    }
  }
  let c = 0;
  for (let el = group?.firstElementChild ?? null; el !== null; el = el.nextElementSibling) {
    if (el.localName !== "col") continue;
    const id = el.getAttribute("data-id");
    if (id) {
      cols.set(id, { c, el });
      colIds.push(id);
    }
    c += 1;
  }
  return { rows, cols, colIds };
}

const rowCells = (tr) => [...tr.children].filter((el) => el.localName === "td" || el.localName === "th");

function isBareEmptyTd(td) {
  if (!td || td.textContent !== "") return false;
  for (const attr of ["f", "v", "fmt", "style", "colspan", "rowspan", "hidden"]) {
    if (td.hasAttribute(attr)) return false;
  }
  return true;
}

function cellSnapshot(grid, r, c, row, col) {
  const cell = grid && r >= 0 && c >= 0 ? grid.cell(a1(r, c)) : null;
  return {
    row,
    col,
    after: cell ? cell.content : null,
    fmt: cell ? cell.fmt : null,
    style: cell ? cell.style : null,
    span: cell ? cell.span : null,
    covered: Boolean(cell?.covered),
  };
}

function cellMatches(grid, maps, { row, col, after, fmt, style, span, covered }) {
  const r = maps.rows.get(row)?.r ?? -1;
  const c = maps.cols.get(col)?.c ?? -1;
  if (r === -1 || c === -1) return false;
  const cell = grid.cell(a1(r, c));
  return (
    same(cell.content, after) &&
    cell.fmt === fmt &&
    same(cell.style, style) &&
    same(cell.span, span) &&
    Boolean(cell.covered) === Boolean(covered)
  );
}

function buildSortSnap(grid, maps, action) {
  const orderedRows = [...(action.rows ?? [])].sort(
    (a, b) => (maps.rows.get(a)?.r ?? -1) - (maps.rows.get(b)?.r ?? -1),
  );
  const frozen = orderedRows.map((id) => Boolean(maps.rows.get(id)?.frozen));
  const cols = action.cols === null ? null : [...action.cols];
  const targetCols = cols ?? maps.colIds;
  const cells = [];
  const seen = new Set();
  for (const rowId of orderedRows) {
    const rowInfo = maps.rows.get(rowId);
    if (!rowInfo) continue;
    const tds = rowCells(rowInfo.tr);
    for (const colId of targetCols) {
      const c = maps.cols.get(colId)?.c ?? -1;
      if (c < 0 || !tds[c] || isBareEmptyTd(tds[c])) continue;
      seen.add(`${rowId}\u0000${colId}`);
      cells.push(cellSnapshot(grid, rowInfo.r, c, rowId, colId));
    }
  }
  return { rows: orderedRows, frozen, cols, cells, seen };
}

function sortStillHolds(grid, maps, snap) {
  if (snap.rows.length === 0) return true;
  const first = maps.rows.get(snap.rows[0]);
  if (!first) return false;
  for (let i = 0; i < snap.rows.length; i++) {
    const info = maps.rows.get(snap.rows[i]);
    if (!info || info.r !== first.r + i || info.frozen !== snap.frozen[i]) return false;
  }
  if (snap.cols !== null) {
    for (const colId of snap.cols) if (!maps.cols.has(colId)) return false;
  }
  for (const item of snap.cells) {
    if (!cellMatches(grid, maps, item)) return false;
  }
  const checkCols = snap.cols ?? maps.colIds;
  for (const rowId of snap.rows) {
    const info = maps.rows.get(rowId);
    const tds = rowCells(info.tr);
    for (const colId of checkCols) {
      if (snap.seen.has(`${rowId}\u0000${colId}`)) continue;
      const c = maps.cols.get(colId)?.c ?? -1;
      if (c < 0 || !tds[c] || isBareEmptyTd(tds[c])) continue;
      const cell = grid.cell(a1(info.r, c));
      if (
        cell.content !== null ||
        cell.fmt !== null ||
        cell.style !== null ||
        cell.span !== null ||
        Boolean(cell.covered)
      ) {
        return false;
      }
    }
  }
  return true;
}

function collectAdded(events, key) {
  const set = new Set();
  for (const event of events) {
    for (const id of event[key]?.added ?? []) set.add(id);
    for (const id of event[key]?.removed ?? []) set.delete(id);
  }
  return [...set];
}

// The sheet's own undo: one list per tab of the actions this interface made, each a Grid change
// event's inverse (or a group's inverses) and the cells it wrote. An undo applies the inverse only
// when every one of those cells still holds what the action wrote (content, fmt, style and merge
// state), so a peer's later edit is never undone (C10 replaces this guard with the room's guarded
// revert). Undo and redo return { ok, cells } or { ok: false, why } with why "empty" or "changed".
export function createHistory(root, gridOf) {
  const tabs = new Map();
  const openGroups = new Map();
  let replaying = false;

  const listOf = (tab) => {
    if (!tabs.has(tab)) tabs.set(tab, { done: [], undone: [] });
    return tabs.get(tab);
  };

  function buildEntry(tab, events) {
    const table = tabTable(root, tab);
    const grid = table ? gridOf(table) : null;
    const maps = tableMaps(table);
    const coords = collectCoords(events);
    const cells = coords.map(({ row, col }) =>
      cellSnapshot(grid, maps.rows.get(row)?.r ?? -1, maps.cols.get(col)?.c ?? -1, row, col),
    );
    const nonSort = events.filter((e) => e.action?.t !== "sort");
    const axisStates = [];
    for (const [kind, changed] of [
      ["row", nonSort.flatMap((e) => e.rows?.changed ?? [])],
      ["col", nonSort.flatMap((e) => e.cols?.changed ?? [])],
    ]) {
      for (const id of new Set(changed)) {
        const el = kind === "col" ? maps.cols.get(id)?.el ?? null : maps.rows.get(id)?.tr ?? null;
        axisStates.push({ kind, id, fmt: el?.getAttribute("fmt") ?? null, style: el?.getAttribute("style") ?? null });
      }
    }
    const sorts = events
      .filter((e) => e.action?.t === "sort")
      .map((e) => buildSortSnap(grid, maps, e.action));
    return {
      inverses: events.map((ev) => ev.inverse),
      cells,
      sorts,
      sortActions: events.filter((e) => e.action?.t === "sort").map((e) => e.action),
      rows: collectAdded(events, "rows"),
      cols: collectAdded(events, "cols"),
      axisStates,
    };
  }

  function pushDone(tab, entry) {
    const list = listOf(tab);
    list.done.push(entry);
    if (list.done.length > LIMIT) list.done.shift();
    list.undone.length = 0;
  }

  // Called with every change event of a tab's Grid that the interface caused.
  function record(tab, event) {
    if (replaying || event?.source !== "api" || !event.inverse || TAB_ACTIONS.has(event.action?.t)) return;
    const bucket = openGroups.get(tab);
    if (bucket) {
      bucket.push(event);
      return;
    }
    pushDone(tab, buildEntry(tab, [event]));
  }

  function group(tab, fn) {
    if (openGroups.has(tab)) return fn();
    const bucket = [];
    openGroups.set(tab, bucket);
    try {
      return fn();
    } finally {
      openGroups.delete(tab);
      if (bucket.length > 0) pushDone(tab, buildEntry(tab, bucket));
    }
  }

  function addedHoldOnlyOurs(table, grid, entry) {
    const ours = (key, id) => entry.cells.filter((cell) => cell[key] === id && cell.after !== null).length;
    const { rows, cols } = grid.size;
    for (const id of entry.rows) {
      const r = rowIndex(table, id);
      let filled = 0;
      if (r !== -1) for (let c = 0; c < cols; c++) if (grid.cell(a1(r, c)).content !== null) filled += 1;
      if (filled > ours("row", id)) return false;
    }
    for (const id of entry.cols) {
      const c = colIndex(table, id);
      let filled = 0;
      if (c !== -1) for (let r = 0; r < rows; r++) if (grid.cell(a1(r, c)).content !== null) filled += 1;
      if (filled > ours("col", id)) return false;
    }
    return true;
  }

  function stillHolds(table, grid, entry) {
    const maps = tableMaps(table);
    for (const item of entry.cells) {
      if (!cellMatches(grid, maps, item)) return false;
    }
    for (const snap of entry.sorts ?? []) {
      if (!sortStillHolds(grid, maps, snap)) return false;
    }
    for (const { kind, id, fmt, style } of entry.axisStates ?? []) {
      const el = kind === "col" ? maps.cols.get(id)?.el ?? null : maps.rows.get(id)?.tr ?? null;
      if (!el) return false;
      if ((el.getAttribute("fmt") ?? null) !== fmt || (el.getAttribute("style") ?? null) !== style) return false;
    }
    return addedHoldOnlyOurs(table, grid, entry);
  }

  function replay(tab, from, to) {
    const list = listOf(tab);
    const entry = list[from].pop();
    if (!entry) return { ok: false, why: "empty" };
    const table = tabTable(root, tab);
    const grid = table ? gridOf(table) : null;
    if (!grid || !stillHolds(table, grid, entry)) return { ok: false, why: "changed" };
    replaying = true;
    const events = [];
    try {
      for (let i = entry.inverses.length - 1; i >= 0; i--) {
        const curTable = tabTable(root, tab);
        const curGrid = curTable ? gridOf(curTable) : grid;
        const event = curGrid.apply(entry.inverses[i]);
        if (event?.inverse) events.push(event);
      }
    } finally {
      replaying = false;
    }
    // sapjs undoes a narrow sort by writing each cell back, so its events name every cell of
    // the range; the step reports the sort's two corners instead, both ways, as a sort does.
    const sortActions = entry.sortActions ?? [];
    if (events.length > 0) {
      const next = buildEntry(tab, events);
      if (next.sortActions.length === 0 && sortActions.length > 0) {
        // The range must still read as it does now when this step replays, tied cells included.
        const now = tabTable(root, tab);
        const nowMaps = tableMaps(now);
        next.sortActions = sortActions;
        next.sorts = sortActions.map((action) => buildSortSnap(gridOf(now), nowMaps, action));
      }
      list[to].push(next);
    }
    const maps = tableMaps(tabTable(root, tab));
    const cells = sortActions.length > 0
      ? [...new Map(sortActions.flatMap((action) => sortCorners(action, maps)).map((c) => [`${c.row}\u0000${c.col}`, c])).values()]
      : replayCoords(events, maps);
    return { ok: true, cells };
  }

  return {
    record,
    group,
    undo: (tab) => replay(tab, "done", "undone"),
    redo: (tab) => replay(tab, "undone", "done"),
    canUndo: (tab) => listOf(tab).done.length > 0,
    canRedo: (tab) => listOf(tab).undone.length > 0,
    get replaying() {
      return replaying;
    },
  };
}
