import { a1, colName, tabTable } from "./commit.js";
import { appendBlankTab, BORDER_LINE, frozenOf } from "./format-commands.js";
import { FUNCTION_BY_NAME } from "./function-help-data.js";
import { mountSheetDialog } from "./menu.js";
import { colIndexFromLetters, quoteSheetName, shiftFormula } from "./refs.js";
import { createSelection } from "./selection.js";
import { bevelBox, bevelText } from "../ui/bevel-controls.js";
import { bevelDialog } from "../ui/bevel-dialog.js";
import { FONT_SANS, TOKENS } from "../ui/bevel.js";
import { resolveTheme } from "../ui/theme.js";

export const MAX_TAB_ROWS = 10_000;
export const MAX_TAB_COLS = 1_000;
export const MAX_IMPORT_CELLS = 200_000;
const BLOCK_ROWS = 10_000;
// The sheet's default column width (COL_DEFAULT in geometry.js, a boot module this lazy one does
// not import, so the chunk build keeps geometry.js in the boot chunk).
const DEFAULT_COL_PX = 100;
const DAY_MS = 86_400_000;
const EPOCH_DAYS = 25_569;
const ERRORS = new Set(["#NULL!", "#DIV/0!", "#VALUE!", "#REF!", "#NAME?", "#NUM!", "#N/A", "#SPILL!", "#CALC!", "#GETTING_DATA", "#ERROR!"]);
const XLSX_MIME = "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet";

// ExcelJS's value types (ExcelJS.ValueType), fixed by its file format code.
const T = { Null: 0, Merge: 1, Number: 2, String: 3, Date: 4, Hyperlink: 5, Formula: 6, SharedString: 7, RichText: 8, Boolean: 9, Error: 10 };

export const loadExcel = () => import("../vendor/exceljs.vendor.js").then((m) => m.default);

// What does not come across, in the order the dialog lists it. Each label is followed by its count.
const LOSS_LABELS = {
  functions: "Formulas using functions this sheet doesn't have, kept as their saved values",
  names: "Formulas using named ranges or other files, kept as their saved values",
  differs: "Formulas this sheet computes differently (array formulas, math on whole ranges, empty arguments), kept as their saved values",
  tables: "Excel tables, kept as plain cells",
  charts: "Charts",
  pictures: "Pictures",
  comments: "Tabs with comments or notes",
  pivots: "Pivot tables",
  macros: "Macros",
  validation: "Cells with dropdowns or data rules",
  conditional: "Conditional formatting rules",
  filters: "Filters",
  hiddenTabs: "Hidden tabs, imported as visible",
  tabColors: "Tab colors",
  unfrozen: "Tabs whose frozen rows or columns would cut a merged cell, imported unfrozen",
  formatPast: "Tabs with formatting past their last filled cell, left out",
  hiddenLines: "Hidden rows and columns, shown",
  richText: "Cells with mixed formatting inside, kept as plain text",
  links: "Links, kept as their text",
  fonts: "Cells in another font or size",
  vertical: "Cells aligned to the top or middle",
  rotated: "Cells with turned or indented text",
  fills: "Cells with patterned or gradient fills",
  borders: "Dashed, dotted, double or diagonal borders, drawn as solid lines or left out",
  colors: "Colors that could not be read",
};

// The names of a zip's files, from its central directory: an .xlsx keeps its charts, pictures and
// comments in files ExcelJS reads past, so their names are how the import counts them.
export function zipEntryNames(bytes) {
  const data = bytes instanceof Uint8Array ? bytes : new Uint8Array(bytes);
  const view = new DataView(data.buffer, data.byteOffset, data.byteLength);
  let end = -1;
  for (let i = data.length - 22; i >= Math.max(0, data.length - 65_557); i--) {
    if (view.getUint32(i, true) === 0x06054b50) {
      end = i;
      break;
    }
  }
  if (end < 0) return [];
  const count = view.getUint16(end + 10, true);
  const decoder = new TextDecoder();
  const names = [];
  let p = view.getUint32(end + 16, true);
  for (let n = 0; n < count && p + 46 <= data.length && view.getUint32(p, true) === 0x02014b50; n++) {
    const nameLength = view.getUint16(p + 28, true);
    names.push(decoder.decode(data.subarray(p + 46, p + 46 + nameLength)));
    p += 46 + nameLength + view.getUint16(p + 30, true) + view.getUint16(p + 32, true);
  }
  return names;
}

// Whether a number format shows a calendar date, not only a time of day or a duration.
export function showsDate(fmt) {
  const bare = String(fmt ?? "").replace(/"[^"]*"|\[[^\]]*\]|\\./g, "");
  return /[yd]|m{3,}/i.test(bare) || (/m/i.test(bare) && !/[hs:]/i.test(bare));
}

// A value ExcelJS read as a Date, as the sheet's number (day 0 is 1899-12-30). ExcelJS turns
// Excel's number into a date linearly, from 1904-01-01 in a 1904 file. A calendar date before
// 1900-03-01 in a 1900 file comes out one day early, because Excel counts a 29 February 1900 that
// never was; a time of day or a duration under `fmt` keeps Excel's own number.
export function serialOf(date, fmt, date1904 = false) {
  const linear = (date.getTime() + EPOCH_DAYS * DAY_MS) / DAY_MS;
  const excel = date1904 ? linear - 1462 : linear;
  if (!showsDate(fmt)) return excel;
  if (date1904) return linear;
  return excel >= 1 && excel < 60 ? excel + 1 : excel;
}

// The number Excel stores for a sheet number under `fmt`: one less for a calendar date before
// 1900-03-01, the same otherwise.
export const excelSerialOf = (serial, fmt) => (showsDate(fmt) && serial >= 2 && serial < 61 ? serial - 1 : serial);

// ---- colors ----

function themePalette(workbook) {
  const xml = Object.values(workbook?._themes ?? {})[0];
  if (typeof xml !== "string") return null;
  const scheme = /<a:clrScheme\b[\s\S]*?<\/a:clrScheme>/.exec(xml)?.[0];
  if (!scheme) return null;
  const read = (tag) => {
    const block = new RegExp(`<a:${tag}>([\\s\\S]*?)</a:${tag}>`).exec(scheme)?.[1] ?? "";
    return (/srgbClr val="([0-9A-Fa-f]{6})"/.exec(block) ?? /lastClr="([0-9A-Fa-f]{6})"/.exec(block))?.[1] ?? null;
  };
  // Excel numbers theme colors light 1, dark 1, light 2, dark 2, then the six accents and the links.
  return ["lt1", "dk1", "lt2", "dk2", "accent1", "accent2", "accent3", "accent4", "accent5", "accent6", "hlink", "folHlink"].map(read);
}

function tinted(hex, tint) {
  if (!tint) return hex;
  const [r, g, b] = [0, 2, 4].map((i) => parseInt(hex.slice(i, i + 2), 16) / 255);
  const max = Math.max(r, g, b);
  const min = Math.min(r, g, b);
  let l = (max + min) / 2;
  const d = max - min;
  const s = d === 0 ? 0 : d / (1 - Math.abs(2 * l - 1));
  let h = 0;
  if (d !== 0) {
    if (max === r) h = ((g - b) / d + 6) % 6;
    else if (max === g) h = (b - r) / d + 2;
    else h = (r - g) / d + 4;
  }
  l = tint < 0 ? l * (1 + tint) : l * (1 - tint) + tint;
  const c = (1 - Math.abs(2 * l - 1)) * s;
  const x = c * (1 - Math.abs((h % 2) - 1));
  const m = l - c / 2;
  const [r1, g1, b1] = h < 1 ? [c, x, 0] : h < 2 ? [x, c, 0] : h < 3 ? [0, c, x] : h < 4 ? [0, x, c] : h < 5 ? [x, 0, c] : [c, 0, x];
  return [r1, g1, b1].map((v) => Math.round(Math.min(1, Math.max(0, v + m)) * 255).toString(16).padStart(2, "0")).join("");
}

// A file color as "#rrggbb", or null when it is automatic or cannot be read (counted in `losses`).
function colorOf(color, palette, losses) {
  if (!color) return null;
  if (typeof color.argb === "string" && /^[0-9A-Fa-f]{8}$/.test(color.argb)) return `#${color.argb.slice(2).toLowerCase()}`;
  if (Number.isInteger(color.theme)) {
    const base = palette?.[color.theme];
    if (base) return `#${tinted(base, color.tint ?? 0).toLowerCase()}`;
  }
  if (color.indexed === 64 || color.auto) return null;
  losses.colors += 1;
  return null;
}

// ---- styles ----

const BORDER_WIDTHS = { thin: 1, medium: 2, thick: 3 };
const DEFAULT_BORDER_COLOR = BORDER_LINE.split(" ").pop();

function borderOf(side, palette, losses) {
  if (!side?.style) return null;
  let width = BORDER_WIDTHS[side.style];
  if (!width) {
    losses.borders += 1;
    width = /medium|thick|double/.test(side.style) ? 2 : 1;
  }
  return `${width}px solid ${colorOf(side.color, palette, losses) ?? DEFAULT_BORDER_COLOR}`;
}

function styleOf(cell, palette, losses, fonts) {
  const style = {};
  const font = cell.font ?? {};
  if (font.bold) style["font-weight"] = "bold";
  if (font.italic) style["font-style"] = "italic";
  const lines = [font.underline && font.underline !== "none" ? "underline" : "", font.strike ? "line-through" : ""].filter(Boolean);
  if (lines.length) style["text-decoration"] = lines.join(" ");
  const ink = colorOf(font.color, palette, losses);
  if (ink && ink !== "#000000") style.color = ink;
  if (font.name || font.size) fonts.set(`${font.name ?? ""}|${font.size ?? ""}`, (fonts.get(`${font.name ?? ""}|${font.size ?? ""}`) ?? 0) + 1);

  const fill = cell.fill;
  if (fill?.type === "pattern" && fill.pattern === "solid") {
    const paint = colorOf(fill.fgColor, palette, losses);
    if (paint && paint !== "#ffffff") style["background-color"] = paint;
  } else if (fill && !(fill.type === "pattern" && (fill.pattern === "none" || !fill.pattern))) {
    losses.fills += 1;
  }

  const align = cell.alignment ?? {};
  const horizontal = align.horizontal === "centerContinuous" ? "center" : align.horizontal;
  if (horizontal === "left" || horizontal === "center" || horizontal === "right") style["text-align"] = horizontal;
  if (align.wrapText) style["white-space"] = "pre-wrap";
  if (align.vertical === "top" || align.vertical === "middle") losses.vertical += 1;
  if (align.textRotation || align.indent) losses.rotated += 1;

  const border = cell.border ?? {};
  for (const side of ["top", "right", "bottom", "left"]) {
    const line = borderOf(border[side], palette, losses);
    if (line) style[`border-${side}`] = line;
  }
  if (border.diagonal?.up || border.diagonal?.down) losses.borders += 1;
  return Object.keys(style).length ? style : undefined;
}

// ---- formulas ----

const COORD = new Set(["cell", "col", "row"]);
const EXCEL_COLS = 16_384;

// The functions a formula calls that the sheet lacks, and whether it uses something the sheet
// cannot read at all (a named range, another file, a tab the file does not have). The tokenizer
// reads a defined name as coordinates ("MyName2" is column MYN then cell AME2), so a column or row
// word that is not one end of a range means a name.
export function checkFormula(text, tokenize, sheetNames, definedNames = new Set()) {
  const tokens = tokenize(text);
  const missing = [];
  let unreadable = false;
  let differs = false;
  const isColon = (token) => token?.type === "punct" && token.value === ":";
  const isPunct = (token, value) => token?.type === "punct" && token.value === value;
  tokens.forEach((token, i) => {
    const prev = tokens[i - 1];
    const next = tokens[i + 1];
    if (token.type === "invalid") unreadable = true;
    if (token.type === "name") {
      if (next?.type === "punct" && next.value === "(") {
        if (!FUNCTION_BY_NAME.has(token.value.toUpperCase())) missing.push(token.value.toUpperCase());
      } else unreadable = true;
    }
    if ((token.type === "col" || token.type === "row") && !isColon(prev) && !isColon(next)) unreadable = true;
    if (COORD.has(token.type) && token.sheet != null && !sheetNames.has(token.sheet.toLowerCase())) unreadable = true;
    // Excel's last column is XFD, so a longer "column" is a name; so is a word the file defines.
    if (token.col !== undefined && !(colIndexFromLetters(token.col) >= 0 && colIndexFromLetters(token.col) < EXCEL_COLS)) unreadable = true;
    if ((token.type === "cell" || token.type === "col") && token.sheet == null && definedNames.has(text.slice(token.start, token.end).toLowerCase())) unreadable = true;
    // Q1:Q4!B2 spans tabs (a 3D reference), which the engine reads as one tab's rectangle.
    if (COORD.has(token.type) && isColon(next) && tokens[i + 2]?.sheet != null && tokens[i + 2].sheet !== token.sheet) unreadable = true;
    // An empty argument (`IF(a,,b)`, `VLOOKUP(x,A:B,2,)`) is one the engine refuses.
    if ((isPunct(token, ",") && (isPunct(next, ",") || isPunct(next, ")"))) || (isPunct(token, "(") && isPunct(next, ","))) differs = true;
    // Math or a comparison on a whole range is array math, which the engine does not do.
    if (COORD.has(token.type) && isColon(next) && COORD.has(tokens[i + 2]?.type)) {
      const before = i > 1 ? tokens[i - 1] : null;
      if (before?.type === "op" || tokens[i + 3]?.type === "op") differs = true;
    }
  });
  return { tokens, missing: [...new Set(missing)], unreadable, differs };
}

// The formula with each edit applied: `edit(token, i)` returns { from, to, text } or null, in
// token order.
function rewrite(text, tokens, edit) {
  let out = "";
  let at = 0;
  tokens.forEach((token, i) => {
    const change = edit(token, i);
    if (!change) return;
    out += text.slice(at, change.from) + change.text;
    at = change.to;
  });
  return out + text.slice(at);
}

// An edit that replaces every tab name a formula names with that tab's new name (`rename` maps
// lower-case old names to new ones).
const renameTabs = (text, rename) => (token) => {
  if (!COORD.has(token.type) || token.sheet == null) return null;
  const to = rename.get(token.sheet.toLowerCase());
  return to === undefined ? null : { from: token.start, to: text.lastIndexOf("!", token.end), text: quoteSheetName(to) };
};

const renamedRefs = (text, tokens, rename) => rewrite(text, tokens, renameTabs(text, rename));

const plainFormula = (text) => `=${String(text).replace(/_xlfn\.|_xlws\./gi, "")}`;

// ---- reading ----

function freeName(name, taken) {
  const base = String(name ?? "").replace(/[\r\n\t\0]+/g, " ").trim() || "Sheet";
  let candidate = base;
  for (let n = 2; taken.has(candidate.toLowerCase()); n++) candidate = `${base} (${n})`;
  taken.add(candidate.toLowerCase());
  return candidate;
}

function savedValue(result, fmt, date1904) {
  if (result instanceof Date) return serialOf(result, fmt, date1904);
  if (typeof result === "number" || typeof result === "boolean") return result;
  if (typeof result === "string" && !ERRORS.has(result)) return result;
  return undefined;
}

// Excel's built-in date formats as ExcelJS names them, as the sheet's own date formats (Excel
// shows format 14 in the reader's short date, which in the US is m/d/yyyy).
const FMT_ALIASES = { "mm-dd-yy": "m/d/yyyy", "m/d/yy h:mm": "m/d/yyyy h:mm" };

function cellItem(cell, env) {
  const { losses } = env;
  const value = cell.value;
  let content = null;
  switch (cell.type) {
    case T.Number:
    case T.Boolean:
      content = value;
      break;
    case T.String:
    case T.SharedString:
      content = value === "" ? null : value;
      break;
    case T.Date:
      content = serialOf(value, cell.numFmt, env.date1904);
      break;
    case T.RichText:
      content = (value.richText ?? []).map((run) => run.text ?? "").join("") || null;
      losses.richText += 1;
      break;
    case T.Hyperlink:
      content = typeof value.text === "string" ? value.text : (value.text?.richText ?? []).map((run) => run.text ?? "").join("");
      losses.links += 1;
      break;
    case T.Error:
      content = ERRORS.has(value.error) ? { f: `=${value.error}` } : null;
      break;
    case T.Formula:
      content = formulaContent(cell, env);
      break;
    default:
      content = null;
  }
  // The sheet keeps a format up to 255 characters on one line, as Excel does.
  const own = cell.numFmt && cell.numFmt !== "General" && cell.numFmt.length <= 255 && !/[\r\n\0]/.test(cell.numFmt) ? cell.numFmt : null;
  const fmt = own ? FMT_ALIASES[own] ?? own : cell.type === T.Date ? "yyyy-mm-dd" : undefined;
  if (cell.hyperlink && cell.type !== T.Hyperlink) losses.links += 1;
  const style = styleOf(cell, env.palette, losses, env.fonts);
  if (fmt === undefined && style === undefined) return content;
  return { content, ...(fmt === undefined ? {} : { fmt }), ...(style === undefined ? {} : { style }) };
}

// A filled formula (a "shared formula") is stored once on its first cell; each copy is that text
// moved by the copy's distance, with the sheet's own reference rules.
function formulaText(cell, env) {
  const shared = cell.value?.sharedFormula;
  if (!shared) return plainFormula(cell.value?.formula ?? cell.formula ?? "");
  const first = cell.worksheet.getCell(shared);
  return shiftFormula(plainFormula(first.value?.formula ?? ""), cell.row - first.row, cell.col - first.col, env.tokenize);
}

function formulaContent(cell, env) {
  const text = formulaText(cell, env);
  const { tokens, missing, unreadable, differs } = checkFormula(text, env.tokenize, env.sheetNames, env.definedNames);
  // Google saves some plain formulas as one-cell array formulas; only a spill over several cells
  // is one the engine cannot compute (array math in one cell is caught by checkFormula).
  const box = cell.value?.shareType === "array" ? rangeBox(cell.value.ref) : null;
  const array = Boolean(box && (box.bottom > box.top || box.right > box.left));
  if (missing.length === 0 && !unreadable && !differs && !array) return { f: renamedRefs(text, tokens, env.rename) };
  if (unreadable) env.losses.names += 1;
  else if (differs || array) env.losses.differs += 1;
  else {
    env.losses.functions += 1;
    for (const name of missing) env.missing.add(name);
  }
  const saved = savedValue(cell.value?.result, cell.numFmt, env.date1904);
  if (saved !== undefined) return saved;
  // No usable saved value: the formula stays, so the cell shows an error as the file's would.
  return { f: renamedRefs(text, tokens, env.rename) };
}

function newLosses() {
  return Object.fromEntries(Object.keys(LOSS_LABELS).map((key) => [key, 0]));
}

// Counts per file part: how many charts, pictures and so on the zip holds.
function countParts(names, losses) {
  for (const name of names) {
    if (/^xl\/charts\/chart(Ex)?\d+\.xml$/.test(name)) losses.charts += 1;
    else if (/^xl\/media\/[^/]+$/.test(name)) losses.pictures += 1;
    else if (/^xl\/comments\d*\.xml$/.test(name)) losses.comments += 1;
    else if (/^xl\/tables\/table\d+\.xml$/.test(name)) losses.tables += 1;
    else if (/^xl\/pivotTables\/pivotTable\d+\.xml$/.test(name)) losses.pivots += 1;
    else if (/vbaProject\.bin$/.test(name)) losses.macros += 1;
  }
}

function widthsOf(ws, cols) {
  const out = [];
  for (let c = 1; c <= cols; c++) {
    const column = ws.getColumn(c);
    out.push(Number.isFinite(column.width) ? Math.min(9999, Math.max(1, Math.round(column.width * 7 + 5))) : null);
  }
  return out;
}

// Everything an import will write, read from the workbook before anything is written: the tabs
// (name, size, a block of items per row, merges, freeze, sizes), what will not come across, and
// the tabs refused for size. `existingTabs` are the workbook's tabs, so names never collide.
export function planImport(workbook, bytes, { tokenize, existingTabs = [] }) {
  const losses = newLosses();
  countParts(zipEntryNames(bytes), losses);
  const sheets = workbook.worksheets ?? [];
  const taken = new Set(existingTabs.map((t) => String(t.name ?? "").toLowerCase()));
  const rename = new Map();
  const refused = [];
  const accepted = [];
  let cells = 0;
  for (const ws of sheets) {
    const { rows, cols } = usedSize(ws);
    if (rows > MAX_TAB_ROWS || cols > MAX_TAB_COLS || cells + rows * cols > MAX_IMPORT_CELLS) {
      refused.push({ name: ws.name, rows, cols });
      continue;
    }
    cells += rows * cols;
    if ((ws.rowCount ?? 0) > rows || (ws.columnCount ?? 0) > cols) losses.formatPast += 1;
    const name = freeName(ws.name, taken);
    rename.set(ws.name.toLowerCase(), name);
    accepted.push({ ws, name, rows, cols });
  }
  // Only tabs that come in can be named: a formula naming a refused tab keeps its saved value.
  const sheetNames = new Set(accepted.map(({ ws }) => ws.name.toLowerCase()));
  const date1904 = Boolean(workbook.properties?.date1904);
  const definedNames = new Set((workbook.definedNames?.model ?? []).map((entry) => String(entry.name ?? "").toLowerCase()));
  const env = { losses, tokenize, sheetNames, definedNames, rename, date1904, palette: themePalette(workbook), fonts: new Map(), missing: new Set() };
  const tabs = accepted.map(({ ws, name, rows, cols }) => readTab(ws, name, rows, cols, env));
  const fontCounts = [...env.fonts.values()];
  losses.fonts = fontCounts.length > 1 ? fontCounts.reduce((a, b) => a + b, 0) - Math.max(...fontCounts) : 0;
  return { tabs, refused, losses, missing: [...env.missing].sort() };
}

// A range like "B4:C5" as 1-based { top, left, bottom, right }.
function rangeBox(range) {
  const match = /^\$?([A-Za-z]+)\$?(\d+):\$?([A-Za-z]+)\$?(\d+)$/.exec(String(range));
  if (!match) return null;
  return { top: Number(match[2]), left: colIndexFromLetters(match[1]) + 1, bottom: Number(match[4]), right: colIndexFromLetters(match[3]) + 1 };
}

// The tab's size up to its last value, formula or merge: Excel also counts cells that only carry
// formatting, which would make a tab with borders down to row 12,000 too large to import.
function usedSize(ws) {
  let rows = 0;
  let cols = 0;
  ws.eachRow({ includeEmpty: false }, (row, r) => {
    row.eachCell({ includeEmpty: false }, (cell, c) => {
      if (cell.type === T.Null) return;
      if (r > rows) rows = r;
      if (c > cols) cols = c;
    });
  });
  for (const range of ws.model?.merges ?? []) {
    const box = rangeBox(range);
    if (box) {
      rows = Math.max(rows, box.bottom);
      cols = Math.max(cols, box.right);
    }
  }
  return { rows, cols };
}

const cutsMerge = (box, freeze) =>
  (freeze.rows > 0 && box.top <= freeze.rows && box.bottom > freeze.rows) || (freeze.cols > 0 && box.left <= freeze.cols && box.right > freeze.cols);

function readTab(ws, name, rows, cols, env) {
  const { losses } = env;
  if (ws.state === "hidden" || ws.state === "veryHidden") losses.hiddenTabs += 1;
  if (ws.autoFilter) losses.filters += 1;
  if (ws.properties?.tabColor) losses.tabColors += 1;
  losses.conditional += (ws.conditionalFormattings ?? []).reduce((n, cf) => n + (cf.rules?.length ?? 0), 0);
  const block = [];
  const heights = [];
  for (let r = 1; r <= rows; r++) {
    const row = ws.getRow(r);
    if (row.hidden) losses.hiddenLines += 1;
    heights.push(Number.isFinite(row.height) ? Math.min(9999, Math.max(1, Math.round((row.height * 4) / 3))) : null);
    const line = [];
    for (let c = 1; c <= cols; c++) {
      const cell = row.getCell(c);
      if (cell.dataValidation) losses.validation += 1;
      line.push(cell.type === T.Merge ? null : cellItem(cell, env));
    }
    block.push(line);
  }
  for (let c = 1; c <= cols; c++) if (ws.getColumn(c).hidden) losses.hiddenLines += 1;
  const view = (ws.views ?? [])[0];
  const merges = [...(ws.model?.merges ?? [])];
  let freeze = view?.state === "frozen" ? { rows: view.ySplit ?? 0, cols: view.xSplit ?? 0 } : null;
  if (freeze && merges.some((range) => { const box = rangeBox(range); return box && cutsMerge(box, freeze); })) {
    freeze = null;
    losses.unfrozen += 1;
  }
  return { name, source: ws.name, rows, cols, block, merges, freeze, widths: widthsOf(ws, cols), heights };
}

// The dialog's list of what will not come across: [{ key, label, count }], biggest loss first in
// the fixed order of LOSS_LABELS, with the missing functions named.
export function lossLines(plan) {
  const lines = [];
  for (const [key, label] of Object.entries(LOSS_LABELS)) {
    const count = plan.losses[key] ?? 0;
    if (count === 0) continue;
    const named = key === "functions" && plan.missing.length ? ` (${plan.missing.join(", ")})` : "";
    lines.push({ key, label: `${label}${named}`, count });
  }
  return lines;
}

// ---- writing ----

// Runs of equal sizes as [first, last, px], skipping sizes the file leaves at the default.
function runsOf(sizes) {
  const runs = [];
  sizes.forEach((px, i) => {
    if (px === null) return;
    const last = runs[runs.length - 1];
    if (last && last[1] === i - 1 && last[2] === px) last[1] = i;
    else runs.push([i, i, px]);
  });
  return runs;
}

// One tab's content into its Grid: cells in blocks, then merges, sizes and freeze.
export function writeTab(grid, tab) {
  for (let start = 0; start < tab.block.length; start += BLOCK_ROWS) {
    grid.setRange(a1(start, 0), tab.block.slice(start, start + BLOCK_ROWS), { detect: false });
  }
  for (const range of tab.merges) grid.merge(range);
  for (const [first, last, px] of runsOf(tab.widths)) grid.setColWidth(`${colName(first)}:${colName(last)}`, px);
  for (const [first, last, px] of runsOf(tab.heights)) grid.setRowHeight(`${first + 1}:${last + 1}`, px);
  if (tab.freeze && (tab.freeze.rows > 0 || tab.freeze.cols > 0)) grid.freeze(tab.freeze);
}

export class ImportStopped extends Error {
  constructor(reason) {
    super(reason);
    this.name = "ImportStopped";
  }
}

// Adds every tab of the plan, each filled in one history step. A failure partway removes the tabs
// this import added and throws ImportStopped with the reason, so nothing half done stays.
export function applyImport(ctx, plan) {
  const { win, workbook, tabs, history } = ctx;
  // Each tab's insert event: undoing it with its own inverse puts back formulas on other tabs that
  // the new tab's name had bound, where a plain delete would cut them to #REF!.
  const added = [];
  const sapGrid = win.Sap?.grid;
  try {
    for (const tab of plan.tabs) {
      let inserted = null;
      const made = appendBlankTab({
        tabs,
        sapGrid,
        gridOf: ctx.gridOf,
        runTabStep: (fn) => {
          inserted = fn();
          return true;
        },
        customName: tab.name,
        rowsCount: Math.max(100, tab.rows, (tab.freeze?.rows ?? 0) + 1),
        colsCount: Math.max(26, tab.cols, (tab.freeze?.cols ?? 0) + 1),
      });
      if (!made) throw new ImportStopped("A new tab could not be made.");
      added.push({ id: made.id, inverse: inserted?.inverse ?? null });
      ctx.subscribeGrids();
      const grid = ctx.gridOf(tabTable(workbook, made.id));
      if (!grid) throw new ImportStopped("A new tab could not be made.");
      const write = () => writeTab(grid, tab);
      ctx.withMine(() => (history?.group ? history.group(made.id, write) : write()));
    }
  } catch (error) {
    removeTabs(ctx, added);
    if (error instanceof ImportStopped) throw error;
    throw new ImportStopped(error?.message || "Something in the file could not be written.");
  }
  return added.map((tab) => tab.id);
}

function removeTabs(ctx, added) {
  for (const { id, inverse } of added.reverse()) {
    const keep = ctx.tabs.list().find((t) => t.id !== id);
    try {
      ctx.gridOf(keep?.table)?.dispatch(inverse ?? { v: 1, t: "deleteSheet", sheet: id });
    } catch {
      // A tab that cannot be removed stays; the notice still says the import stopped.
    }
  }
  ctx.subscribeGrids();
}

// ---- export ----

function parseStyle(text) {
  const out = {};
  for (const part of String(text ?? "").split(";")) {
    const at = part.indexOf(":");
    if (at > 0) out[part.slice(0, at).trim()] = part.slice(at + 1).trim();
  }
  return out;
}

const argb = (color) => (/^#[0-9a-f]{6}$/i.test(color ?? "") ? { argb: `FF${color.slice(1).toUpperCase()}` } : undefined);

function borderSide(value) {
  const match = /^(\d+)px\s+\w+\s+(#[0-9a-fA-F]{6})/.exec(value ?? "");
  if (!match) return undefined;
  const width = Number(match[1]);
  return { style: width >= 3 ? "thick" : width === 2 ? "medium" : "thin", color: argb(match[2]) };
}

// The cell's style string as ExcelJS's font, fill, alignment and border.
export function excelStyleOf(text) {
  const css = parseStyle(text);
  const out = {};
  const font = {};
  if (css["font-weight"] === "bold" || Number(css["font-weight"]) >= 600) font.bold = true;
  if (css["font-style"] === "italic") font.italic = true;
  if (/underline/.test(css["text-decoration"] ?? "")) font.underline = true;
  if (/line-through/.test(css["text-decoration"] ?? "")) font.strike = true;
  if (argb(css.color)) font.color = argb(css.color);
  if (Object.keys(font).length) out.font = font;
  const paint = argb(css["background-color"] ?? css.background);
  if (paint) out.fill = { type: "pattern", pattern: "solid", fgColor: paint };
  const alignment = {};
  if (["left", "center", "right"].includes(css["text-align"])) alignment.horizontal = css["text-align"];
  if (css["white-space"] === "pre-wrap" || css["white-space"] === "normal") alignment.wrapText = true;
  if (Object.keys(alignment).length) out.alignment = alignment;
  const border = {};
  for (const side of ["top", "right", "bottom", "left"]) {
    const line = borderSide(css[`border-${side}`] ?? css.border);
    if (line) border[side] = line;
  }
  if (Object.keys(border).length) out.border = border;
  return out;
}

// Functions Excel's file format stores with an "_xlfn." prefix (added after Excel 2007); without
// it Excel shows #NAME?. The sheet's other functions are stored as they are.
const XLFN = new Set([
  "AGGREGATE", "CEILING.MATH", "CONCAT", "DAYS", "FILTER", "FLOOR.MATH", "FORECAST.LINEAR", "IFNA", "IFS", "ISOWEEKNUM",
  "LET", "MAXIFS", "MINIFS", "MODE.SNGL", "NORM.DIST", "NUMBERVALUE", "PERCENTILE.EXC", "PERCENTILE.INC", "QUARTILE.EXC",
  "QUARTILE.INC", "RANDARRAY", "RANK.AVG", "RANK.EQ", "SEQUENCE", "SORT", "SORTBY", "STDEV.P", "STDEV.S", "SWITCH",
  "TEXTJOIN", "UNIQUE", "VAR.P", "VAR.S", "XLOOKUP", "XMATCH",
]);

// A formula as Excel's file stores it: no "=", newer functions prefixed, tab names as exported.
function excelFormula(text, tokenize, rename) {
  if (!tokenize) return text.replace(/^=/, "");
  const tokens = tokenize(text);
  const tabs = renameTabs(text, rename);
  const out = rewrite(text, tokens, (token, i) => {
    const next = tokens[i + 1];
    if (token.type === "name" && next?.type === "punct" && next.value === "(" && XLFN.has(token.value.toUpperCase())) {
      return { from: token.start, to: token.start, text: "_xlfn." };
    }
    return tabs(token);
  });
  return out.replace(/^=/, "");
}

function exportValue(cell, { fmtKind, tokenize, rename }) {
  const { content, value } = cell;
  const fmt = cell.effectiveFmt ?? cell.fmt;
  if (typeof content === "number" && fmt && fmtKind?.(fmt) === "date") return excelSerialOf(content, fmt);
  if (content && typeof content === "object" && typeof content.f === "string") {
    const result = typeof value === "string" && ERRORS.has(value) ? { error: value } : value;
    return { formula: excelFormula(content.f, tokenize, rename), ...(result === null || result === undefined ? {} : { result }) };
  }
  return content;
}

const EXCEL_NAME_BAD = /[\\/?*[\]:]/g;

function excelSheetName(name, taken) {
  const base = String(name ?? "").replace(EXCEL_NAME_BAD, " ").replace(/^'+|'+$/g, "").trim().slice(0, 31) || "Sheet";
  let candidate = base;
  for (let n = 2; taken.has(candidate.toLowerCase()); n++) candidate = `${base.slice(0, 31 - String(n).length - 3)} (${n})`;
  taken.add(candidate.toLowerCase());
  return candidate;
}

function rowsOf(table) {
  return [...table.querySelectorAll(":scope > thead > tr, :scope > tbody > tr")];
}

// Every tab, in order, as an ExcelJS workbook: values, formulas with their current results, number
// formats, the bounded style, merges, frozen rows and columns, widths and heights.
export function buildWorkbook(ExcelJS, tabList, gridOf, { fmtKind, tokenize } = {}) {
  const book = new ExcelJS.Workbook();
  const taken = new Set();
  // Excel's tab name rules can change a name, so every formula naming it is rewritten to match.
  const names = new Map(tabList.map((tab) => [tab, excelSheetName(tab.name, taken)]));
  const rename = new Map(tabList.filter((tab) => names.get(tab) !== tab.name).map((tab) => [String(tab.name).toLowerCase(), names.get(tab)]));
  const options = { fmtKind, tokenize, rename };
  for (const tab of tabList) {
    const grid = gridOf(tab.table);
    if (!grid) continue;
    const ws = book.addWorksheet(names.get(tab));
    ws.properties.defaultColWidth = (DEFAULT_COL_PX - 5) / 7;
    const { rows, cols } = grid.size;
    for (let r = 0; r < rows; r++) {
      for (let c = 0; c < cols; c++) {
        const cell = grid.cell(a1(r, c));
        if (cell.covered) continue;
        const value = exportValue(cell, options);
        // A column's format reaches its cells as their effective format.
        const fmt = cell.effectiveFmt ?? cell.fmt;
        const hasLook = fmt || cell.style;
        if ((value === null || value === undefined) && !hasLook && !cell.span) continue;
        const target = ws.getCell(r + 1, c + 1);
        if (value !== null && value !== undefined) target.value = value;
        if (fmt) target.numFmt = fmt;
        if (cell.style) Object.assign(target, excelStyleOf(cell.style));
        const span = cell.span;
        if (span && (span.rows > 1 || span.cols > 1)) ws.mergeCells(r + 1, c + 1, r + span.rows, c + span.cols);
      }
    }
    sizeColumns(ws, tab.table);
    sizeRows(ws, tab.table);
    const frozen = frozenOf(tab.table);
    if (frozen.rows > 0 || frozen.cols > 0) ws.views = [{ state: "frozen", xSplit: frozen.cols, ySplit: frozen.rows }];
  }
  return book;
}

function sizeColumns(ws, table) {
  table.querySelectorAll(":scope > colgroup > col").forEach((col, c) => {
    const px = parseFloat(col.style.width);
    if (Number.isFinite(px)) ws.getColumn(c + 1).width = Math.max(0, (px - 5) / 7);
  });
}

function sizeRows(ws, table) {
  rowsOf(table).forEach((tr, r) => {
    const px = parseFloat(tr.style.height);
    if (Number.isFinite(px)) ws.getRow(r + 1).height = (px * 3) / 4;
  });
}

export function downloadName(title) {
  const base = String(title ?? "").replace(/[/\\\0:*?"<>|]/g, "-").trim() || "Sheet";
  return `${base}.xlsx`;
}

export async function exportXlsx({ ExcelJS, tabList, gridOf, fmtKind, tokenize, title, doc = document, urlApi = globalThis.URL }) {
  const book = buildWorkbook(ExcelJS, tabList, gridOf, { fmtKind, tokenize });
  const buffer = await book.xlsx.writeBuffer();
  const blob = new Blob([buffer], { type: XLSX_MIME });
  const href = urlApi.createObjectURL(blob);
  const link = doc.createElement("a");
  link.href = href;
  link.download = downloadName(title);
  link.setAttribute("sap-ignore", "");
  link.setAttribute("clay", "no-save no-watch no-snapshot");
  link.style.display = "none";
  doc.body.append(link);
  try {
    link.click();
  } finally {
    link.remove();
    setTimeout(() => urlApi.revokeObjectURL(href), 40000)?.unref?.();
  }
  return { filename: link.download, blob };
}

// ---- the dialog ----

const plural = (n, one, many) => `${n.toLocaleString("en-US")} ${n === 1 ? one : many}`;

// Shows what the import will add and what will not come across, before anything is written.
export function openXlsxImportDialog({ doc = document, theme = resolveTheme(), container = null, fileName, plan, getReturnFocus, onConfirm, onCancel }) {
  const shell = bevelDialog({ zIndex: "2147483001", width: "560px", closable: true, titled: true, theme });
  const stack = bevelBox("div", ["display:flex", "flex-direction:column", "gap:12px"], { theme });
  const heading = (text) => bevelText("div", [`font:600 12.5px/1.3 ${FONT_SANS}`, `color:${TOKENS.ink}`], text, { theme, role: null });
  const list = (items, attr) => {
    const ul = doc.createElement("ul");
    ul.setAttribute(attr, "");
    ul.tabIndex = 0;
    ul.style.cssText = `margin:0;padding:0 0 0 18px;max-height:180px;overflow:auto;font:500 12.5px/1.45 ${FONT_SANS};color:${TOKENS["ink-2"]}`;
    for (const text of items) {
      const li = doc.createElement("li");
      li.textContent = text;
      ul.append(li);
    }
    return ul;
  };

  stack.append(
    heading(plan.tabs.length === 1 ? "This adds one tab:" : `This adds ${plan.tabs.length} tabs:`),
    list(plan.tabs.map((t) => `${t.name}: ${plural(t.rows, "row", "rows")}, ${plural(t.cols, "column", "columns")}`), "data-xlsx-tabs"),
  );
  const lines = lossLines(plan);
  if (lines.length) {
    stack.append(heading("What won't come across:"), list(lines.map((l) => `${l.label}: ${l.count.toLocaleString("en-US")}`), "data-xlsx-losses"));
  }
  if (plan.refused.length) {
    stack.append(
      heading(`Not imported, too large (over ${MAX_TAB_ROWS.toLocaleString("en-US")} rows, ${MAX_TAB_COLS.toLocaleString("en-US")} columns, or ${MAX_IMPORT_CELLS.toLocaleString("en-US")} cells in all):`),
      list(plan.refused.map((t) => `${t.name}: ${plural(t.rows, "row", "rows")}, ${plural(t.cols, "column", "columns")}`), "data-xlsx-refused"),
    );
  }
  shell.body.append(stack);
  const handle = mountSheetDialog(doc, shell, {
    title: `Import ${fileName}`,
    confirmLabel: "Import",
    confirmVariant: "primary",
    onConfirm: () => onConfirm?.(),
    onCancel,
    getReturnFocus,
    container,
  });
  handle.confirmBtn.focus();
  return handle;
}

// ---- the menu actions ----

const TOOLS_FAILED = "The Excel tools couldn't load. Check your connection and try again.";

async function loadTools(chrome, loadExcelJS) {
  try {
    return await loadExcelJS();
  } catch {
    chrome.showNotice(TOOLS_FAILED);
    return null;
  }
}

// Reads a picked file, shows the dialog, and on Import adds the tabs. `loadExcelJS` is the vendor
// loader, given by tests.
export async function importXlsxFile(ctx, { name, bytes }, { loadExcelJS = loadExcel } = {}) {
  const { chrome, tabs, editor, win } = ctx;
  chrome.showNotice("Reading the file...");
  const ExcelJS = await loadTools(chrome, loadExcelJS);
  if (!ExcelJS) return null;
  let plan;
  try {
    const book = new ExcelJS.Workbook();
    await book.xlsx.load(bytes.buffer.slice(bytes.byteOffset, bytes.byteOffset + bytes.byteLength));
    plan = planImport(book, bytes, { tokenize: win.Sap.grid.tokenize, existingTabs: tabs.list() });
  } catch {
    chrome.showNotice("This file could not be read as an Excel workbook (.xlsx).");
    return null;
  }
  if (plan.tabs.length === 0) {
    chrome.showNotice(
      plan.refused.length ? "Nothing was imported: every tab in this file is over 10,000 rows, 1,000 columns, or 200,000 cells in all." : "This file has no tabs to import.",
    );
    return null;
  }
  if (ctx.ensureReadyForAction && !ctx.ensureReadyForAction()) return null;
  chrome.hideNotice();
  // The import only adds tabs, so its dialog belongs to no tab: a recalculation on the open tab
  // (the function pack arriving) must not close it.
  ctx.showDialog(
    "",
    openXlsxImportDialog({
      doc: ctx.doc,
      theme: chrome.theme,
      container: chrome.topRoot,
      fileName: name,
      plan,
      getReturnFocus: () => editor.textarea,
      onConfirm: () => {
        ctx.clearDialog();
        confirmImport(ctx, plan);
      },
      onCancel: () => ctx.clearDialog(),
    }),
  );
  return plan;
}

function confirmImport(ctx, plan) {
  const { chrome, editor } = ctx;
  let added;
  try {
    added = applyImport(ctx, plan);
  } catch (error) {
    chrome.showNotice(`The import stopped and nothing was added. ${error.message}`);
    return;
  }
  const first = added[0];
  ctx.tabs.show(first);
  const sel = createSelection(ctx.viewOf(first), first);
  ctx.setSel(sel);
  ctx.applySelection(sel, { announce: true, enteredTab: true });
  chrome.hideNotice();
  editor.focus();
}

export async function downloadXlsx(ctx, { loadExcelJS = loadExcel } = {}) {
  const { chrome, tabs, doc, win } = ctx;
  chrome.showNotice("Making the file...");
  const ExcelJS = await loadTools(chrome, loadExcelJS);
  if (!ExcelJS) return;
  try {
    await exportXlsx({ ExcelJS, tabList: tabs.list(), gridOf: ctx.gridOf, fmtKind: win.Sap?.grid?.fmtKind, tokenize: win.Sap?.grid?.tokenize, title: doc.title, doc, urlApi: win.URL ?? globalThis.URL });
    chrome.hideNotice();
  } catch {
    chrome.showNotice("The .xlsx file could not be made. Try again.");
  }
}
