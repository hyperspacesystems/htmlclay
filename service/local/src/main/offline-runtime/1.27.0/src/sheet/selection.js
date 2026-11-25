import { a1, colName } from "./commit.js";

const clamp = (n, lo, hi) => Math.max(lo, Math.min(hi, n));

// The nearest index a move may land on: `dir` names the way the person is going, and the
// search turns around only when there is no visible index left in front of it.
export function visibleIndex(len, idx, dir, isHidden) {
  if (len <= 0) return 0;
  const step = dir >= 0 ? 1 : -1;
  for (let i = clamp(idx, 0, len - 1); i >= 0 && i < len; i += step) if (!isHidden?.(i)) return i;
  for (let i = clamp(idx, 0, len - 1); i >= 0 && i < len; i -= step) if (!isHidden?.(i)) return i;
  return clamp(idx, 0, len - 1);
}

function visibleRow(view, r, dir) {
  return visibleIndex(view.rows.length, r, dir, view.rowHidden);
}

function visibleCol(view, c, dir) {
  return visibleIndex(view.cols.length, c, dir, view.colHidden);
}

export function ownerOf(view, r, c) {
  const rowCount = view.rows.length;
  const colCount = view.cols.length;
  if (rowCount === 0 || colCount === 0) return { r: 0, c: 0, rows: 1, cols: 1 };
  const cr = clamp(r, 0, rowCount - 1);
  const cc = clamp(c, 0, colCount - 1);
  const direct = view.cell?.(cr, cc);
  if (!direct?.covered) {
    const span = direct?.span;
    return {
      r: cr,
      c: cc,
      rows: clamp(span?.rows ?? 1, 1, rowCount - cr),
      cols: clamp(span?.cols ?? 1, 1, colCount - cc),
    };
  }
  for (let rr = cr; rr >= 0; rr--) {
    for (let kc = cc; kc >= 0; kc--) {
      const cand = view.cell?.(rr, kc);
      if (!cand || cand.covered || !cand.span) continue;
      const rows = cand.span.rows ?? 1;
      const cols = cand.span.cols ?? 1;
      if (rr + rows > cr && kc + cols > cc) {
        return {
          r: rr,
          c: kc,
          rows: clamp(rows, 1, rowCount - rr),
          cols: clamp(cols, 1, colCount - kc),
        };
      }
    }
  }
  return { r: cr, c: cc, rows: 1, cols: 1 };
}

export function isFilled(view, r, c) {
  const own = ownerOf(view, r, c);
  const cell = view.cell?.(own.r, own.c);
  if (!cell) return false;
  if (cell.content !== undefined) return cell.content !== null && cell.content !== "";
  return typeof cell.text === "string" && cell.text !== "";
}

function resolveAxis(list, id, prevList) {
  const at = list.indexOf(id);
  if (at !== -1) return at;
  const was = prevList ? prevList.indexOf(id) : -1;
  return clamp(was === -1 ? 0 : was, 0, list.length - 1);
}

function coordsOf(view, sel, prevView = null) {
  const ar = resolveAxis(view.rows, sel?.anchor?.row ?? "", prevView?.rows);
  const ac = resolveAxis(view.cols, sel?.anchor?.col ?? "", prevView?.cols);
  const fr = resolveAxis(view.rows, sel?.focus?.row ?? "", prevView?.rows);
  const fc = resolveAxis(view.cols, sel?.focus?.col ?? "", prevView?.cols);
  return { ar, ac, fr, fc };
}

export function createSelection(view, tab = "") {
  if (view.rows.length === 0 || view.cols.length === 0) {
    return { tab, anchor: { row: "", col: "" }, focus: { row: "", col: "" } };
  }
  const own = ownerOf(view, visibleRow(view, 0, 1), visibleCol(view, 0, 1));
  const point = { row: view.rows[own.r], col: view.cols[own.c] };
  return { tab, anchor: point, focus: { ...point } };
}

export function clipSelection(view, sel, prevView = null) {
  const tab = sel?.tab ?? "";
  if (view.rows.length === 0 || view.cols.length === 0) {
    return { tab, anchor: { row: "", col: "" }, focus: { row: "", col: "" } };
  }
  const { ar, ac, fr, fc } = coordsOf(view, sel, prevView);
  if (sel?.whole === "cols") {
    const left = visibleCol(view, Math.min(ac, fc), 1);
    const right = visibleCol(view, Math.max(ac, fc), -1);
    return {
      tab,
      anchor: { row: view.rows[0], col: view.cols[left] },
      focus: { row: view.rows[view.rows.length - 1], col: view.cols[right] },
      whole: "cols",
    };
  }
  if (sel?.whole === "rows") {
    const top = visibleRow(view, Math.min(ar, fr), 1);
    const bottom = visibleRow(view, Math.max(ar, fr), -1);
    return {
      tab,
      anchor: { row: view.rows[top], col: view.cols[0] },
      focus: { row: view.rows[bottom], col: view.cols[view.cols.length - 1] },
      whole: "rows",
    };
  }
  const single = sel?.anchor?.row === sel?.focus?.row && sel?.anchor?.col === sel?.focus?.col;
  const ownA = ownerOf(view, visibleRow(view, ar, 1), visibleCol(view, ac, 1));
  const anchor = { row: view.rows[ownA.r], col: view.cols[ownA.c] };
  if (single) return { tab, anchor, focus: { ...anchor } };
  const ownF = ownerOf(view, visibleRow(view, fr, 1), visibleCol(view, fc, 1));
  return { tab, anchor, focus: { row: view.rows[ownF.r], col: view.cols[ownF.c] } };
}

// Grows [top..bottom, left..right] until every merge it touches is whole inside it.
export function growBox(view, rawTop, rawLeft, rawBottom, rawRight) {
  const maxR = view.rows.length - 1;
  const maxC = view.cols.length - 1;
  let top = clamp(Math.min(rawTop, rawBottom), 0, maxR);
  let left = clamp(Math.min(rawLeft, rawRight), 0, maxC);
  let bottom = clamp(Math.max(rawTop, rawBottom), 0, maxR);
  let right = clamp(Math.max(rawLeft, rawRight), 0, maxC);
  if (top === 0 && left === 0 && bottom === maxR && right === maxC) return { top, left, bottom, right };
  let grew = true;
  while (grew) {
    grew = false;
    for (let r = top; r <= bottom; r++) {
      for (let c = left; c <= right; c++) {
        const own = ownerOf(view, r, c);
        const b = own.r + own.rows - 1;
        const rt = own.c + own.cols - 1;
        if (own.r < top) { top = own.r; grew = true; }
        if (own.c < left) { left = own.c; grew = true; }
        if (b > bottom) { bottom = b; grew = true; }
        if (rt > right) { right = rt; grew = true; }
      }
    }
  }
  return { top, left, bottom, right };
}

export function selectionBox(view, sel) {
  if (view.rows.length === 0 || view.cols.length === 0) return null;
  const { ar, ac, fr, fc } = coordsOf(view, sel);
  const active = ownerOf(view, ar, ac);
  if (sel?.whole === "cols") return { top: 0, bottom: view.rows.length - 1, left: Math.min(ac, fc), right: Math.max(ac, fc), active };
  if (sel?.whole === "rows") return { top: Math.min(ar, fr), bottom: Math.max(ar, fr), left: 0, right: view.cols.length - 1, active };
  const box = growBox(view, active.r, active.c, fr, fc);
  return { ...box, active };
}

export function selectionArea(view, sel) {
  const box = selectionBox(view, sel);
  if (!box) return "A1";
  if (box.top === box.bottom && box.left === box.right) return a1(box.top, box.left);
  return `${a1(box.top, box.left)}:${a1(box.bottom, box.right)}`;
}

export function axisSpan(view, sel) {
  if (view.rows.length === 0 || view.cols.length === 0) return null;
  const { ar, ac, fr, fc } = coordsOf(view, sel);
  if (sel?.whole === "rows") return { axis: "rows", first: Math.min(ar, fr), last: Math.max(ar, fr) };
  if (sel?.whole === "cols") return { axis: "cols", first: Math.min(ac, fc), last: Math.max(ac, fc) };
  return null;
}

export function commandArea(view, sel) {
  const span = axisSpan(view, sel);
  if (span?.axis === "cols") return `${colName(span.first)}:${colName(span.last)}`;
  if (span?.axis === "rows") return `${span.first + 1}:${span.last + 1}`;
  return selectionArea(view, sel);
}

function landSingle(view, tab, r, c, dr = 0, dc = 0) {
  const own = ownerOf(view, visibleRow(view, r, dr), visibleCol(view, c, dc));
  const point = { row: view.rows[own.r], col: view.cols[own.c] };
  return { tab, anchor: point, focus: { ...point } };
}

function landExtend(view, sel, ownA, r, c, dr = 0, dc = 0) {
  const nr = visibleRow(view, r, dr);
  const nc = visibleCol(view, c, dc);
  return {
    tab: sel.tab,
    anchor: { row: view.rows[ownA.r], col: view.cols[ownA.c] },
    focus: { row: view.rows[nr], col: view.cols[nc] },
  };
}

export function selectCell(view, sel, r, c, { extend = false } = {}) {
  if (view.rows.length === 0 || view.cols.length === 0) return sel;
  if (!extend) return landSingle(view, sel.tab, r, c);
  const { ar, ac } = coordsOf(view, sel);
  return landExtend(view, sel, ownerOf(view, ar, ac), r, c);
}

export function moveSelection(view, sel, { dr = 0, dc = 0, extend = false } = {}) {
  if (view.rows.length === 0 || view.cols.length === 0) return sel;
  const { ar, ac, fr, fc } = coordsOf(view, sel);
  const ownA = ownerOf(view, ar, ac);
  if (!extend) {
    const r = dr > 0 ? ownA.r + ownA.rows - 1 + dr : ownA.r + dr;
    const c = dc > 0 ? ownA.c + ownA.cols - 1 + dc : ownA.c + dc;
    return landSingle(view, sel.tab, r, c, dr, dc);
  }
  const ownF = ownerOf(view, fr, fc);
  const r = dr > 0 ? ownF.r + ownF.rows - 1 + dr : dr < 0 ? ownF.r + dr : fr;
  const c = dc > 0 ? ownF.c + ownF.cols - 1 + dc : dc < 0 ? ownF.c + dc : fc;
  return landExtend(view, sel, ownA, r, c, dr, dc);
}

// The next index in `step`s direction that a person can see, or -1 at the end of the axis.
function nextVisible(length, from, step, hiddenAt) {
  for (let i = from; i >= 0 && i < length; i += step) if (!hiddenAt?.(i)) return i;
  return -1;
}

// The Sheets Ctrl+arrow rule along one axis: from a filled run to its last filled cell before
// a blank; otherwise to the next filled cell; otherwise to the sheet edge. Hidden rows and
// columns are stepped over, never landed on.
function scanEdge(length, start, step, filledAt, hiddenAt) {
  const edge = visibleIndex(length, step > 0 ? length - 1 : 0, step, hiddenAt);
  const next = nextVisible(length, start + step, step, hiddenAt);
  if (next === -1) return edge;
  if (filledAt(start) && filledAt(next)) {
    let at = next;
    for (let k = nextVisible(length, at + step, step, hiddenAt); k !== -1 && filledAt(k); k = nextVisible(length, at + step, step, hiddenAt)) at = k;
    return at;
  }
  for (let at = next; at !== -1; at = nextVisible(length, at + step, step, hiddenAt)) {
    if (filledAt(at)) return at;
  }
  return edge;
}

export function edgeSelection(view, sel, { dr = 0, dc = 0, extend = false } = {}) {
  if (view.rows.length === 0 || view.cols.length === 0) return sel;
  const { ar, ac, fr, fc } = coordsOf(view, sel);
  const ownA = ownerOf(view, ar, ac);
  const base = extend ? ownerOf(view, fr, fc) : ownA;
  const lineR = extend ? fr : ownA.r;
  const lineC = extend ? fc : ownA.c;
  if (dr !== 0) {
    const start = dr > 0 ? base.r + base.rows - 1 : base.r;
    const r = scanEdge(view.rows.length, start, dr > 0 ? 1 : -1, (k) => isFilled(view, k, lineC), view.rowHidden);
    return extend ? landExtend(view, sel, ownA, r, lineC, dr, 0) : landSingle(view, sel.tab, r, lineC, dr, 0);
  }
  if (dc !== 0) {
    const start = dc > 0 ? base.c + base.cols - 1 : base.c;
    const c = scanEdge(view.cols.length, start, dc > 0 ? 1 : -1, (k) => isFilled(view, lineR, k), view.colHidden);
    return extend ? landExtend(view, sel, ownA, lineR, c, 0, dc) : landSingle(view, sel.tab, lineR, c, 0, dc);
  }
  return sel;
}

export function rowEdgeSelection(view, sel, { end = false, extend = false } = {}) {
  if (view.rows.length === 0 || view.cols.length === 0) return sel;
  const { ar, ac, fr } = coordsOf(view, sel);
  const ownA = ownerOf(view, ar, ac);
  const c = end ? view.cols.length - 1 : 0;
  const dc = end ? 1 : -1;
  return extend ? landExtend(view, sel, ownA, fr, c, 0, dc) : landSingle(view, sel.tab, ownA.r, c, 0, dc);
}

export function sheetCornerSelection(view, sel, { end = false, extend = false } = {}) {
  if (view.rows.length === 0 || view.cols.length === 0) return sel;
  const { ar, ac } = coordsOf(view, sel);
  const ownA = ownerOf(view, ar, ac);
  const r = end ? view.rows.length - 1 : 0;
  const c = end ? view.cols.length - 1 : 0;
  const dir = end ? 1 : -1;
  return extend ? landExtend(view, sel, ownA, r, c, dir, dir) : landSingle(view, sel.tab, r, c, dir, dir);
}

export function pageSelection(view, sel, { dir = 1, pageSize = 20, extend = false } = {}) {
  if (view.rows.length === 0 || view.cols.length === 0) return sel;
  const { ar, ac, fr, fc } = coordsOf(view, sel);
  const ownA = ownerOf(view, ar, ac);
  const step = (dir >= 0 ? 1 : -1) * Math.max(1, pageSize);
  if (!extend) return landSingle(view, sel.tab, ownA.r + step, ownA.c, step, 0);
  return landExtend(view, sel, ownA, fr + step, fc, step, 0);
}

export function selectAll(view, sel) {
  if (view.rows.length === 0 || view.cols.length === 0) return sel;
  const own = ownerOf(view, 0, 0);
  return {
    tab: sel.tab,
    anchor: { row: view.rows[own.r], col: view.cols[own.c] },
    focus: { row: view.rows[view.rows.length - 1], col: view.cols[view.cols.length - 1] },
  };
}

export function selectCols(view, sel) {
  const box = selectionBox(view, sel);
  if (!box) return sel;
  return {
    tab: sel.tab,
    anchor: { row: view.rows[0], col: view.cols[box.left] },
    focus: { row: view.rows[view.rows.length - 1], col: view.cols[box.right] },
    whole: "cols",
  };
}

export function selectRows(view, sel) {
  const box = selectionBox(view, sel);
  if (!box) return sel;
  return {
    tab: sel.tab,
    anchor: { row: view.rows[box.top], col: view.cols[0] },
    focus: { row: view.rows[box.bottom], col: view.cols[view.cols.length - 1] },
    whole: "rows",
  };
}

export const selectionKey = (path = "/") => `clay:sheet:selection:${path}`;

export function readSelection(storage, path = "/") {
  try {
    const raw = storage?.getItem?.(selectionKey(path));
    if (!raw) return null;
    const parsed = JSON.parse(raw);
    if (
      typeof parsed?.tab === "string" &&
      typeof parsed?.anchor?.row === "string" &&
      typeof parsed?.anchor?.col === "string" &&
      typeof parsed?.focus?.row === "string" &&
      typeof parsed?.focus?.col === "string"
    ) {
      const out = {
        tab: parsed.tab,
        anchor: { row: parsed.anchor.row, col: parsed.anchor.col },
        focus: { row: parsed.focus.row, col: parsed.focus.col },
      };
      if (parsed.whole === "cols" || parsed.whole === "rows") out.whole = parsed.whole;
      return out;
    }
  } catch {}
  return null;
}

export function writeSelection(storage, path, sel) {
  try {
    storage?.setItem?.(selectionKey(path), JSON.stringify(sel));
  } catch {}
}

export function selectColsAt(view, sel, c, { extend = false } = {}) {
  const from = extend && sel?.whole === "cols" ? coordsOf(view, sel).ac : c;
  return { tab: sel.tab, anchor: { row: view.rows[0], col: view.cols[from] }, focus: { row: view.rows[view.rows.length - 1], col: view.cols[c] }, whole: "cols" };
}

export function selectRowsAt(view, sel, r, { extend = false } = {}) {
  const from = extend && sel?.whole === "rows" ? coordsOf(view, sel).ar : r;
  return { tab: sel.tab, anchor: { row: view.rows[from], col: view.cols[0] }, focus: { row: view.rows[r], col: view.cols[view.cols.length - 1] }, whole: "rows" };
}
