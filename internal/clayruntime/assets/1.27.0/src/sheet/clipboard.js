import { shiftFormula, transposeFormula } from "./refs.js";

const escapeHtml = (text) => String(text).replace(/[&<>"']/g, (ch) => `&#${ch.charCodeAt(0)};`);

const STYLE_PROPS = [
  "font-weight",
  "font-style",
  "text-decoration",
  "color",
  "background-color",
  "text-align",
  "white-space",
  "border-top",
  "border-right",
  "border-bottom",
  "border-left",
];

const STYLE_SET = new Set(STYLE_PROPS);
const STYLE_VALUE = /^(?:[a-zA-Z0-9#%.,+ -]|(?:rgb|rgba|hsl|hsla)\([0-9.,%+ -]*\))+$/;

export function clipboardStyleMap(css) {
  const out = Object.create(null);
  for (const prop of STYLE_PROPS) out[prop] = null;
  if (typeof css !== "string" || !css) return out;
  for (const decl of css.split(";")) {
    const colon = decl.indexOf(":");
    if (colon === -1) continue;
    const key = decl.slice(0, colon).trim().toLowerCase();
    const val = decl.slice(colon + 1).trim().replace(/\s+/g, " ");
    if (!key || !val || !STYLE_VALUE.test(val)) continue;
    const normKey = key === "background" ? "background-color" : key;
    if (key === "border") {
      out["border-top"] = val;
      out["border-right"] = val;
      out["border-bottom"] = val;
      out["border-left"] = val;
    } else if (STYLE_SET.has(normKey)) {
      out[normKey] = val;
    }
  }
  return out;
}

// The two clipboard parts for an area of a Grid: tab-separated shown values, and a table whose
// cells carry their content, value, format and inline style, from which a paste rebuilds them
// (formulas moved by the distance to where they land).
export function copyParts(grid, { top, left, bottom, right }, a1, { cut = false, skipRows = null } = {}) {
  const rows = [];
  const kept = [];
  for (let r = top; r <= bottom; r++) {
    if (skipRows?.has(r)) continue;
    kept.push(r);
    const cells = [];
    for (let c = left; c <= right; c++) {
      const cell = grid.cell(a1(r, c));
      const attrs = [
        `data-content="${escapeHtml(JSON.stringify(cell.content ?? null))}"`,
        `data-value="${escapeHtml(JSON.stringify(cell.value ?? null))}"`,
      ];
      if (cell.effectiveFmt !== null && cell.effectiveFmt !== undefined) {
        attrs.push(`data-fmt="${escapeHtml(cell.effectiveFmt)}"`);
      }
      if (cell.style !== null && cell.style !== undefined && cell.style !== "") {
        attrs.push(`data-style="${escapeHtml(cell.style)}"`);
      }
      cells.push(`<td ${attrs.join(" ")}>${escapeHtml(cell.text)}</td>`);
    }
    rows.push(`<tr data-sheet-row="${r - (kept[0] ?? top)}">${cells.join("")}</tr>`);
  }
  const html = `<table data-sheet-origin="${kept[0] ?? top},${left}"${cut ? " data-sheet-cut" : ""}><tbody>${rows.join("")}</tbody></table>`;
  const text = skipRows?.size > 0
    ? kept
        .map((r) => grid.toDelimited(`${a1(r, left)}:${a1(r, right)}`, { delimiter: "\t", values: "shown" }).replace(/^""$/, ""))
        .join("\r\n")
    : grid.toDelimited(`${a1(top, left)}:${a1(bottom, right)}`, { delimiter: "\t", values: "shown" }).replace(/(^|\r\n)""$/, "$1");
  return { html, text };
}

const parseJson = (raw) => {
  try {
    return JSON.parse(raw);
  } catch {
    return null;
  }
};

export function transposeMatrix(matrix, emptyValue = "") {
  const rowCount = matrix.length;
  const colCount = Math.max(0, ...matrix.map((row) => row.length));
  if (rowCount === 0 || colCount === 0) return [];
  return Array.from({ length: colCount }, (_, j) =>
    Array.from({ length: rowCount }, (_, i) => (j < matrix[i].length ? matrix[i][j] : emptyValue)),
  );
}

// The block a paste writes at (r, c), or null when the HTML holds no single table. Our own table
// gives { kind: "ours", block, origin }: items { content, fmt, style } (or { fmt, style } when
// `formatOnly`), with formulas moved by the distance from the source, or plain values when
// `valuesOnly`. `transpose` swaps rows and columns and moves each formula by the distance from its
// source cell to its transposed target cell.
export function pasteBlock(html, at, { valuesOnly = false, transpose = false, formatOnly = false } = {}) {
  if (!html) return null;
  const doc = new DOMParser().parseFromString(html, "text/html");
  const tables = [...doc.body.querySelectorAll("table")].filter((t) => !t.parentElement.closest("table"));
  if (tables.length !== 1) return null;
  const table = tables[0];
  const rows = [...table.rows].map((tr) => [...tr.cells]);
  if (rows.length === 0) return null;
  if (rows.every((cells) => cells.length === 0)) return null;
  const offsets = [...table.rows].map((tr, i) => {
    const n = Number.parseInt(tr.getAttribute("data-sheet-row") ?? "", 10);
    return Number.isInteger(n) && n >= 0 ? n : i;
  });
  const rawOrigin = table.getAttribute("data-sheet-origin");
  if (!rawOrigin || !/^\d+,\d+$/.test(rawOrigin)) {
    const foreign = rows.map((cells) => cells.map((td) => td.textContent ?? ""));
    return { kind: "foreign", block: transpose ? transposeMatrix(foreign, "") : foreign };
  }
  const [or, oc] = rawOrigin.split(",").map(Number);
  const cut = table.hasAttribute("data-sheet-cut");
  const one = rows.length === 1 && rows[0].length === 1;

  const buildItem = (td, srcR, srcC, dstR, dstC) => {
    if (!td) {
      if (valuesOnly) return null;
      const emptyStyle = clipboardStyleMap(null);
      return formatOnly ? { fmt: null, style: emptyStyle } : { content: null, fmt: null, style: emptyStyle };
    }
    if (valuesOnly) return parseJson(td.getAttribute("data-value"));
    const fmt = td.getAttribute("data-fmt");
    const style = clipboardStyleMap(td.getAttribute("data-style"));
    if (formatOnly) return { fmt, style };
    const content = parseJson(td.getAttribute("data-content"));
    const [dr, dc] = cut ? [0, 0] : [dstR - srcR, dstC - srcC];
    const formula = !one && content !== null && typeof content === "object" && typeof content.f === "string";
    const moved = !formula
      ? content
      : transpose && !cut
        ? { f: transposeFormula(content.f, { r: srcR, c: srcC }, { r: dstR, c: dstC }) }
        : { f: shiftFormula(content.f, dr, dc) };
    return { content: moved, fmt, style };
  };

  let block;
  if (!transpose) {
    block = rows.map((cells, i) => cells.map((td, j) => buildItem(td, or + offsets[i], oc + j, at.r + i, at.c + j)));
  } else {
    const rowCount = rows.length;
    const colCount = Math.max(0, ...rows.map((cells) => cells.length));
    block = Array.from({ length: colCount }, (_, j) =>
      Array.from({ length: rowCount }, (_, i) => buildItem(rows[i]?.[j] ?? null, or + offsets[i], oc + j, at.r + j, at.c + i)),
    );
  }
  return { kind: "ours", block, origin: cut ? { r: at.r, c: at.c } : { r: or, c: oc } };
}

// One cell's item repeated over a larger selection, as a one-cell paste fills it.
export function fillBlock(block, rows, cols) {
  if (block.length !== 1 || block[0].length !== 1 || (rows === 1 && cols === 1)) return block;
  return Array.from({ length: rows }, () => Array.from({ length: cols }, () => block[0][0]));
}

// A one-cell formula repeated over a larger selection, moved from the source origin to each target.
export const fillFormula = (rawF, fmt, dr, dc, rows, cols, style = undefined) =>
  Array.from({ length: rows }, (_, i) =>
    Array.from({ length: cols }, (_, j) => {
      const item = { content: { f: shiftFormula(rawF, dr + i, dc + j) }, fmt };
      if (style !== undefined) item.style = style;
      return item;
    }),
  );
