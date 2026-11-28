// Where things are in a sheet table: column offsets from the <col> widths, and row
// offsets from a prefix-sum index of row heights. A row's height is an estimate until
// the row has been laid out once; the row window measures the groups it shows and
// records each row's height by its id, so a rebuilt geometry keeps what was measured.
// Rows are the data rows, the <tr> of every <tbody> in order; frozen <thead> rows are
// never windowed and are not in the index.

export const ROW_ESTIMATE = 26;
export const COL_DEFAULT = 100;

// Values by index in a Fenwick tree: one change and one prefix sum each cost log n.
function fenwick(values) {
  const n = values.length;
  const tree = new Float64Array(n + 1);
  for (let i = 1; i <= n; i++) {
    tree[i] += values[i - 1];
    const up = i + (i & -i);
    if (up <= n) tree[up] += tree[i];
  }
  let top = 1;
  while (top * 2 <= n) top *= 2;
  return {
    add(i, delta) {
      for (let k = i + 1; k <= n; k += k & -k) tree[k] += delta;
    },
    // The sum of values [0, i).
    sum(i) {
      let total = 0;
      for (let k = i; k > 0; k -= k & -k) total += tree[k];
      return total;
    },
    // How many leading values fit in y: the index of the value that holds y.
    find(y) {
      let at = 0;
      let left = y;
      for (let step = top; step > 0; step >>= 1) {
        const next = at + step;
        if (next <= n && tree[next] <= left) {
          at = next;
          left -= tree[next];
        }
      }
      return at;
    },
  };
}

export function tableParts(table) {
  const groups = [...table.tBodies];
  const rows = [];
  const groupStart = [];
  for (const group of groups) {
    groupStart.push(rows.length);
    rows.push(...group.rows);
  }
  const cols = [...table.querySelectorAll(":scope > colgroup > col")];
  return { groups, rows, groupStart, cols };
}

const widthOf = (col) => {
  if (col.hidden) return 0;
  const width = parseFloat(col.style.width);
  return Number.isFinite(width) ? width : COL_DEFAULT;
};

export function createGeometry(table, { measured = new Map() } = {}) {
  const { groups, rows, groupStart, cols } = tableParts(table);
  const ids = rows.map((tr) => tr.getAttribute("data-id"));
  const indexOf = new Map(ids.map((id, i) => [id, i]));
  const heights = rows.map((tr, i) => (rowOutOfSight(tr) ? 0 : measured.get(ids[i]) ?? ROW_ESTIMATE));
  const index = fenwick(heights);
  const groupOf = new Int32Array(rows.length);
  const groupHeights = groupStart.map((start, g) => {
    const end = groupStart[g + 1] ?? rows.length;
    groupOf.fill(g, start, end);
    return index.sum(end) - index.sum(start);
  });
  const widths = cols.map(widthOf);
  const lefts = [];
  let width = 0;
  for (const w of widths) {
    lefts.push(width);
    width += w;
  }

  return {
    rowCount: rows.length,
    groupCount: groups.length,
    width,
    row: (i) => rows[i],
    group: (g) => groups[g],
    groupRows: (g) => [groupStart[g], groupStart[g + 1] ?? rows.length],
    indexOf: (id) => indexOf.get(id) ?? -1,
    rowTop: (i) => (i <= rows.length ? index.sum(i) : index.sum(rows.length) + (i - rows.length) * ROW_ESTIMATE),
    rowHeight: (i) => heights[i] ?? ROW_ESTIMATE,
    // The row under y (body coordinates); the last row for y past the end, -1 with no rows.
    rowAt: (y) => (rows.length === 0 ? -1 : Math.min(index.find(Math.max(0, y)), rows.length - 1)),
    total: () => index.sum(rows.length),
    groupOfRow: (i) => groupOf[i],
    groupTop: (g) => index.sum(groupStart[g]),
    groupHeights: () => groupHeights,
    colLeft: (c) => (c < widths.length ? lefts[c] : width + (c - widths.length) * COL_DEFAULT),
    colWidth: (c) => widths[c] ?? COL_DEFAULT,
    // The column under x; the last column for x past the end, -1 with no columns.
    colAt(x) {
      if (widths.length === 0) return -1;
      let lo = 0;
      let hi = widths.length - 1;
      while (lo < hi) {
        const mid = (lo + hi + 1) >> 1;
        if (lefts[mid] <= x) lo = mid;
        else hi = mid - 1;
      }
      return lo;
    },
    // Records a laid out row's height and returns the change, so a caller keeping a row
    // on screen can move the scroll position by it.
    setRowHeight(i, height) {
      const delta = height - heights[i];
      // A row with no id yet (sapjs repairs it) is not kept: such rows would share one entry.
      if (ids[i]) measured.set(ids[i], height);
      if (delta === 0) return 0;
      heights[i] = height;
      index.add(i, delta);
      groupHeights[groupOf[i]] += delta;
      return delta;
    },
  };
}

export function tableMetrics(table, win) {
  const headRows = table.tHead?.rows.length ?? 0;
  const geom = win ? win.geometry : createGeometry(table);
  const tBox = table.getBoundingClientRect();
  const headHeight = table.tHead?.offsetHeight ?? headRows * ROW_ESTIMATE;

  function cellBox(r, c) {
    if (r >= headRows && win) return win.cellBox(r - headRows, c);
    if (r < headRows) {
      const tr = table.tHead.rows[r];
      const trBox = tr.getBoundingClientRect();
      const styleH = parseFloat(tr.style?.height);
      const fallbackH = tr.hidden ? 0 : (Number.isFinite(styleH) ? styleH : ROW_ESTIMATE);
      const top = trBox.height > 0 ? trBox.top : tBox.top + r * ROW_ESTIMATE;
      const height = trBox.height > 0 ? trBox.height : fallbackH;
      return { left: tBox.left + geom.colLeft(c), top, width: geom.colWidth(c), height };
    }
    const br = r - headRows;
    return {
      left: tBox.left + geom.colLeft(c),
      top: tBox.top + headHeight + geom.rowTop(br),
      width: geom.colWidth(c),
      height: geom.rowHeight(br),
    };
  }

  function spanBox(top, left, bottom, right) {
    const tl = cellBox(top, left);
    const br = cellBox(bottom, right);
    return {
      left: tl.left,
      top: tl.top,
      width: Math.max(0, br.left + br.width - tl.left),
      height: Math.max(0, br.top + br.height - tl.top),
    };
  }

  function visibleRows(totalRows) {
    const list = [];
    for (let r = 0; r < Math.min(headRows, totalRows); r++) list.push(r);
    if (win?.shown && win.shown.last >= win.shown.first) {
      for (let g = win.shown.first - 1; g <= win.shown.last - 1; g++) {
        const [start, end] = geom.groupRows(g);
        for (let i = start; i < end && headRows + i < totalRows; i++) list.push(headRows + i);
      }
      return list;
    }
    for (let r = headRows; r < totalRows; r++) list.push(r);
    return list;
  }

  function cellAt(x, y) {
    if (headRows > 0 && x >= tBox.left && x < tBox.left + geom.width && y >= tBox.top && y < tBox.top + headHeight) {
      const c = geom.colAt(x - tBox.left);
      if (c === -1) return null;
      for (let r = 0; r < headRows; r++) {
        const b = cellBox(r, c);
        if (y >= b.top && y < b.top + b.height) return { r, c };
      }
    }
    if (win) {
      const hit = win.cellAt(x, y);
      return hit ? { r: hit.r + headRows, c: hit.c } : null;
    }
    const bodyTop = tBox.top + headHeight;
    if (x < tBox.left || x >= tBox.left + geom.width || y < bodyTop || y >= bodyTop + geom.total()) return null;
    const br = geom.rowAt(y - bodyTop);
    const c = geom.colAt(x - tBox.left);
    return br === -1 || c === -1 ? null : { r: br + headRows, c };
  }

  return { cellBox, spanBox, visibleRows, cellAt, width: geom.width, totalHeight: headHeight + geom.total() };
}

// Rows this viewer filtered out, per table: a view of the sheet, never saved and never shared.
const filtered = new WeakMap();
const NONE = new Set();

export const filteredRows = (table) => (table && filtered.get(table)) || NONE;

export function setFilteredRows(table, ids) {
  if (!table) return;
  if (ids && ids.size > 0) filtered.set(table, new Set(ids));
  else filtered.delete(table);
}

// A row is out of sight when the file hides it or this viewer filtered it out.
export function rowOutOfSight(tr) {
  if (!tr) return false;
  if (tr.hidden) return true;
  const table = tr.closest?.("table");
  return Boolean(table && filtered.get(table)?.has(tr.getAttribute("data-id")));
}

// Where a tab's saved rules apply (sapjs S8: `<template rules>`). sapjs stores rules and
// moves their ranges; ClayJS decides what they mean for a cell. This part is what the
// editor needs on every keystroke: which checkbox or dropdown covers a cell, and a
// dropdown's items. It lives here, in a module every sheet page already loads, so it adds
// no request at start-up; the styling of a whole tab is in the lazy rules.js.

const PART = /^([A-Z]+)?(\d+)?$/;

const colIndex = (letters) => {
  let n = 0;
  for (const ch of letters) n = n * 26 + ch.charCodeAt(0) - 64;
  return n - 1;
};

export const colLetters = (index) => {
  let s = "";
  for (let n = index + 1; n > 0; n = Math.floor((n - 1) / 26)) s = String.fromCharCode(65 + ((n - 1) % 26)) + s;
  return s;
};

// "C2:C40", the open-ended "C2:C", "B3", "A:A" or "2:5" on a sheet of `size` rows and
// columns, as a 0-based inclusive box clipped to the sheet, or null when nothing is left.
export function boundRange(range, size) {
  const [from, to = from] = String(range ?? "").toUpperCase().replace(/\$/g, "").split(":");
  const a = PART.exec(from);
  const b = PART.exec(to);
  if (!a || !b || (!a[1] && !a[2]) || (!b[1] && !b[2])) return null;
  if (a[2] === "0" || b[2] === "0") return null;
  const r0 = a[2] ? Number(a[2]) - 1 : 0;
  const c0 = a[1] ? colIndex(a[1]) : 0;
  const r1 = b[2] ? Number(b[2]) - 1 : size.rows - 1;
  const c1 = b[1] ? colIndex(b[1]) : size.cols - 1;
  const box = {
    r0: Math.min(r0, r1),
    c0: Math.min(c0, c1),
    r1: Math.min(Math.max(r0, r1), size.rows - 1),
    c1: Math.min(Math.max(c0, c1), size.cols - 1),
  };
  return box.r0 > box.r1 || box.c0 > box.c1 ? null : box;
}

export const boxA1 = ({ r0, c0, r1, c1 }) => `${colLetters(c0)}${r0 + 1}:${colLetters(c1)}${r1 + 1}`;

export const isEmpty = (value) => value === null || value === undefined || value === "";

// The text a value shows as in a list, and the key two values match by (case-blind).
export const shown = (value) => (typeof value === "boolean" ? (value ? "TRUE" : "FALSE") : String(value));
export const matchKey = (value) => shown(value).toLowerCase();

const STYLE_PROPS = new Set(["background-color", "color", "font-weight", "font-style", "text-decoration"]);
// A colour, a keyword list or a number: never a url(), a quote, a brace or an unclosed bracket.
const SAFE_VALUE = /^(?:[#\w\s.,%-]+|rgba?\([\d\s.,%]+\))$/;

// A saved rule's style as the declarations ClayJS draws: a property it knows, and a value that
// cannot break out of its declaration.
export function parseRuleStyle(style) {
  const props = [];
  for (const part of String(style ?? "").split(";")) {
    const at = part.indexOf(":");
    if (at === -1) continue;
    const prop = part.slice(0, at).trim().toLowerCase();
    const value = part.slice(at + 1).trim();
    if (STYLE_PROPS.has(prop) && SAFE_VALUE.test(value)) props.push([prop, value]);
  }
  return props;
}

// A grid range read as rows of typed values, whatever shape grid.get gives one cell.
export function readBox(grid, box) {
  const value = grid.get(boxA1(box));
  return Array.isArray(value) ? value.map((row) => (Array.isArray(row) ? row : [row])) : [[value]];
}

// The first checkbox or dropdown rule covering row `row`, column `col` (0-based), in the
// rules' saved order: an earlier rule wins.
export function inputRuleAt(grid, row, col) {
  const size = grid.size;
  for (const rule of grid.rules?.() ?? []) {
    if (rule.kind !== "check" && rule.kind !== "list") continue;
    const box = boundRange(rule.range, size);
    if (box && row >= box.r0 && row <= box.r1 && col >= box.c0 && col <= box.c1) return rule;
  }
  return null;
}

export function tableNamed(table, name) {
  const book = table.closest("[sheet-workbook]");
  for (const section of book?.querySelectorAll(":scope > section[item]") ?? []) {
    const label = section.querySelector(':scope > [bind="name"]')?.textContent.trim();
    if (label?.toLowerCase() === name.toLowerCase()) return section.querySelector(":scope > table[grid]");
  }
  return null;
}

// A dropdown's items: its own list, or the distinct values of its `source` range, which
// may name another tab ("Lists!A2:A", "'Price list'!B2:B20").
export function listItems(table, rule) {
  if (Array.isArray(rule.items)) return rule.items;
  const source = String(rule.source ?? "");
  const bang = source.lastIndexOf("!");
  const target = bang === -1 ? table : tableNamed(table, source.slice(0, bang).replace(/^'(.*)'$/, "$1").replace(/''/g, "'"));
  const grid = target && window.Sap?.grid?.(target);
  if (!grid) return [];
  const box = boundRange(bang === -1 ? source : source.slice(bang + 1), grid.size);
  if (!box) return [];
  const seen = new Set();
  const items = [];
  for (const text of readTexts(grid, box).flat()) {
    if (text === "") continue;
    if (seen.has(text)) continue;
    seen.add(text);
    items.push(text);
  }
  return items;
}

// A range read as rows of the text each cell shows ("50%", "3/14/2026"), which is what a
// dropdown lists and matches: a picked "50%" is stored as 0.5.
export function readTexts(grid, box) {
  const rows = [];
  for (let r = box.r0; r <= box.r1; r++) {
    const row = [];
    for (let c = box.c0; c <= box.c1; c++) row.push(String(grid.cell(`${colLetters(c)}${r + 1}`)?.text ?? ""));
    rows.push(row);
  }
  return rows;
}

// A typed value a closed dropdown keeps: blank, a formula, or one of its items.
export function allowedInList(table, rule, text) {
  if (rule.open || isEmpty(text) || String(text).startsWith("=")) return true;
  const key = String(text).toLowerCase();
  return listItems(table, rule).some((item) => item.toLowerCase() === key);
}
