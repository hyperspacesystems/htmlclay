import { a1 } from "./commit.js";
import { ownerOf } from "./selection.js";

const ERROR_CODES = new Set([
  "#NULL!",
  "#DIV/0!",
  "#VALUE!",
  "#REF!",
  "#NAME?",
  "#NUM!",
  "#N/A",
  "#ERROR!",
]);

const COLLATOR_OPTS = { sensitivity: "base", numeric: true };

const isEmpty = (v) => v === null || v === undefined || v === "";

function errorCodeOf(v) {
  if (typeof v === "string" && ERROR_CODES.has(v)) return v;
  if (v !== null && typeof v === "object" && typeof v.error === "string") return v.error;
  return null;
}

function rankOf(v) {
  if (typeof v === "number") return 0;
  if (errorCodeOf(v) !== null) return 3;
  if (typeof v === "string") return 1;
  if (typeof v === "boolean") return 2;
  return 3;
}

function compareAscendingNonEmpty(a, b) {
  const ra = rankOf(a);
  const rb = rankOf(b);
  if (ra !== rb) return ra - rb;
  if (ra === 0) return a < b ? -1 : a > b ? 1 : 0;
  if (ra === 1) return a.localeCompare(b, undefined, COLLATOR_OPTS);
  if (ra === 2) return a === b ? 0 : a ? 1 : -1;
  const ea = errorCodeOf(a) ?? String(a);
  const eb = errorCodeOf(b) ?? String(b);
  return ea.localeCompare(eb, undefined, COLLATOR_OPTS);
}

export function compareSortValues(a, b, direction = "asc") {
  const aEmpty = isEmpty(a);
  const bEmpty = isEmpty(b);
  if (aEmpty && bEmpty) return 0;
  if (aEmpty) return 1;
  if (bEmpty) return -1;
  const cmp = compareAscendingNonEmpty(a, b);
  if (cmp === 0) return 0;
  return direction === "desc" ? -cmp : cmp;
}

export function sortRowOrder(entries, direction = "asc") {
  return entries
    .slice()
    .sort((a, b) => compareSortValues(a.value, b.value, direction))
    .map((entry) => entry.id);
}

const unmoved = (rows, entries) => rows.every((id, i) => id === entries[i].id);

export function rangeAnchorCutAtRight(view, box) {
  if (!view || !box) return false;
  for (let r = box.top; r <= box.bottom; r++) {
    const own = ownerOf(view, r, box.right);
    if (own.c === box.right && own.cols > 1 && own.rows === 1) return true;
  }
  return false;
}

export function buildSheetSortAction(grid, view, { sheet, col, frozenRows = 0, direction = "asc" }) {
  if (!grid || !view || col < 0 || col >= view.cols.length) return null;
  if (view.rows.length - frozenRows < 2) return null;
  const entries = [];
  for (let r = frozenRows; r < view.rows.length; r++) {
    entries.push({ id: view.rows[r], value: grid.cell(a1(r, col)).value });
  }
  const rows = sortRowOrder(entries, direction);
  if (unmoved(rows, entries)) return null;
  return { v: 1, t: "sort", sheet, rows, cols: null };
}

export function buildRangeSortAction(
  grid,
  view,
  { sheet, box, col = box?.active?.c ?? box?.left ?? 0, direction = "asc" },
) {
  if (!grid || !view || !box) return null;
  if (box.bottom - box.top + 1 < 2 || view.cols.length === 0) return null;
  const keyCol = Math.max(box.left, Math.min(box.right, col));
  const entries = [];
  for (let r = box.top; r <= box.bottom; r++) {
    entries.push({ id: view.rows[r], value: grid.cell(a1(r, keyCol)).value });
  }
  const fullWidth = box.left === 0 && box.right === view.cols.length - 1;
  const rows = sortRowOrder(entries, direction);
  if (unmoved(rows, entries)) return null;
  return { v: 1, t: "sort", sheet, rows, cols: fullWidth ? null : view.cols.slice(box.left, box.right + 1) };
}
