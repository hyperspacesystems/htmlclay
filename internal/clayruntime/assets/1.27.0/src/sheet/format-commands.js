import { a1, colName, tabTable } from "./commit.js";
import { SWATCHES, createMenu, mountSheetDialog, openDeleteTabDialog, openMergeKeepDialog } from "./menu.js";
export { createMenu };
import {
  axisSpan,
  commandArea,
  createSelection,
  ownerOf,
  selectColsAt,
  selectionArea,
  selectionBox,
  selectRowsAt,
} from "./selection.js";
import { buildRangeSortAction, buildSheetSortAction, rangeAnchorCutAtRight } from "./sort.js";
import { bevelBox, bevelInput, bevelText } from "../ui/bevel-controls.js";
import { bevelDialog } from "../ui/bevel-dialog.js";
import { FONT_MONO, FONT_SANS, TOKENS } from "../ui/bevel.js";
import { resolveTheme } from "../ui/theme.js";

export const BORDER_LINE = "1px solid #5F6368";

export const NUMBER_FORMATS = [
  { id: "auto", label: "Automatic", fmt: null, group: 0 },
  { id: "text", label: "Plain text", fmt: "@", group: 0 },
  { id: "number", label: "Number", fmt: "#,##0.00", group: 1 },
  { id: "thousands", label: "Thousands", fmt: "#,##0", group: 1 },
  { id: "percent", label: "Percent", fmt: "0.00%", group: 1 },
  { id: "currency", label: "Currency", fmt: "$#,##0.00", group: 1 },
  { id: "date", label: "Date", fmt: "m/d/yyyy", group: 2 },
  { id: "long-date", label: "Long date", fmt: "mmmm d, yyyy", group: 2 },
  { id: "time", label: "Time", fmt: "h:mm AM/PM", group: 2 },
  { id: "datetime", label: "Date and time", fmt: "m/d/yyyy h:mm", group: 2 },
];

export const BORDER_PRESETS = [
  { id: "all", label: "All", preset: "all" },
  { id: "outer", label: "Outer", preset: "outer" },
  { id: "inner", label: "Inner", preset: "inner" },
  { id: "top", label: "Top", preset: "top" },
  { id: "bottom", label: "Bottom", preset: "bottom" },
  { id: "left", label: "Left", preset: "left" },
  { id: "right", label: "Right", preset: "right" },
  { id: "none", label: "None", preset: "none" },
];

export const ALIGN_OPTIONS = [
  { id: "left", label: "Left", align: "left" },
  { id: "center", label: "Center", align: "center" },
  { id: "right", label: "Right", align: "right" },
];

export const CLEAR_STYLE = {
  "font-weight": null,
  "font-style": null,
  "text-decoration": null,
  color: null,
  background: null,
  "background-color": null,
  "text-align": null,
  "white-space": null,
};

export function formatLabelOf(fmt) {
  const norm = fmt === "" || fmt === undefined ? null : fmt;
  const found = NUMBER_FORMATS.find((entry) => entry.fmt === norm);
  return found ? found.label : "Custom";
}

export const UNKNOWN_FORMAT_MESSAGE = "That code isn't a number format Sheets knows.";
export const FORMATS_FAILED_MESSAGE = "Number formats couldn't load. Check your connection and try again.";
export const LOADING_FORMATS_MESSAGE = "Loading formats...";

export function numberFormatMenuItems({ activeValue, activeFmt, formatValue, onPick, onCustom }) {
  const norm = activeFmt === "" || activeFmt === undefined ? null : activeFmt;
  const out = [];
  let prevGroup = 0;
  for (const entry of NUMBER_FORMATS) {
    if (entry.group !== prevGroup) {
      out.push({ type: "separator" });
      prevGroup = entry.group;
    }
    const detail = activeValue === null || activeValue === undefined || typeof formatValue !== "function"
      ? ""
      : (formatValue(activeValue, entry.fmt) ?? "");
    out.push({
      id: entry.id,
      label: entry.label,
      detail,
      checked: norm === entry.fmt,
      onSelect: () => onPick(entry.fmt),
    });
  }
  out.push(
    { type: "separator" },
    {
      id: "custom-format",
      label: "Custom format...",
      onSelect: () => onCustom?.(),
    },
  );
  return out;
}

export function openCustomFormatDialog({
  doc = document,
  win = doc.defaultView ?? window,
  theme = resolveTheme(),
  container = null,
  initialFmt = "",
  activeValue = null,
  getReturnFocus,
  onConfirm,
  onCancel,
} = {}) {
  const shell = bevelDialog({ zIndex: "2147483001", width: "420px", closable: true, titled: true, theme });
  const stack = bevelBox("div", ["display:flex", "flex-direction:column", "gap:10px"], { theme });

  const fieldLabel = bevelText(
    "label",
    ["display:flex", "flex-direction:column", "gap:6px", `font:500 12.5px/1.35 ${FONT_SANS}`, `color:${TOKENS["ink-2"]}`],
    undefined,
    { theme },
  );
  fieldLabel.append(bevelText("span", ["font:inherit", "color:inherit"], "Format code", { theme, role: null }));

  const monoTheme = {
    ...theme,
    parts: {
      ...theme.parts,
      "sheet.customFmt": { ...theme.parts?.["sheet.customFmt"], base: { ...theme.parts?.["sheet.customFmt"]?.base, "font-family": FONT_MONO } },
    },
  };
  const input = bevelInput("input", {
    theme: monoTheme,
    rules: [`font:400 13px/1.4 ${FONT_MONO}`],
    parts: ["sheet.customFmt"],
  });
  input.value = initialFmt ?? "";
  input.spellcheck = false;
  input.setAttribute("autocomplete", "off");
  input.setAttribute("aria-label", "Format code");
  input.setAttribute("data-custom-format-input", "");
  fieldLabel.append(input);

  const sampleRow = bevelBox(
    "div",
    [
      "display:flex",
      "align-items:baseline",
      "justify-content:space-between",
      "gap:10px",
      "min-height:32px",
      "padding:6px 10px",
      `background:${TOKENS.sunk}`,
      `border:1px solid ${TOKENS["line-2"]}`,
    ],
    { theme },
  );
  const sampleTitle = bevelText("span", [`font:500 12px/1.3 ${FONT_SANS}`, `color:${TOKENS.muted}`], "Sample", {
    theme,
    role: "mutedText",
  });
  const sampleValue = bevelText(
    "span",
    [`font:500 12.5px/1.35 ${FONT_MONO}`, "font-variant-numeric:tabular-nums", `color:${TOKENS.ink}`],
    "",
    { theme },
  );
  sampleValue.setAttribute("data-custom-format-sample", "");
  sampleRow.append(sampleTitle, sampleValue);

  stack.append(fieldLabel, sampleRow);
  shell.body.append(stack);

  let closed = false;
  let canApply = true;
  let formatsSettled = false;
  let formatsFailed = false;
  let loadingPromise = null;
  let handle = null;

  function ensureFormatsPlugin() {
    if (loadingPromise) return;
    const load = win?.Sap?.plugins?.load;
    if (typeof load !== "function") {
      formatsSettled = true;
      return;
    }
    loadingPromise = Promise.resolve(load.call(win.Sap.plugins, "grid-formats"))
      .then(
        () => {
          formatsSettled = true;
        },
        () => {
          formatsSettled = true;
          formatsFailed = true;
        },
      )
      .finally(() => {
        if (!closed) syncSample();
      });
  }

  function syncSample() {
    const raw = input.value;
    if (raw.length > 255) {
      canApply = false;
      handle?.confirmBtn.setDisabled(true);
      sampleValue.textContent = UNKNOWN_FORMAT_MESSAGE;
      return;
    }
    const fmt = raw === "" ? null : raw;
    const sapGrid = win?.Sap?.grid;
    const kind = fmt === null ? "auto" : (typeof sapGrid?.fmtKind === "function" ? sapGrid.fmtKind(fmt) : "unknown");
    if (kind === "unknown") {
      const installed = Boolean(win?.Sap?.plugins?.has?.("grid-formats"));
      if (!installed && !formatsSettled && typeof win?.Sap?.plugins?.load === "function") {
        canApply = false;
        handle?.confirmBtn.setDisabled(true);
        sampleValue.textContent = LOADING_FORMATS_MESSAGE;
        ensureFormatsPlugin();
        return;
      }
      canApply = false;
      handle?.confirmBtn.setDisabled(true);
      sampleValue.textContent = formatsFailed && !installed ? FORMATS_FAILED_MESSAGE : UNKNOWN_FORMAT_MESSAGE;
      return;
    }
    canApply = true;
    handle?.confirmBtn.setDisabled(false);
    if (activeValue === null || activeValue === undefined) {
      sampleValue.textContent = "";
      return;
    }
    const shown =
      typeof sapGrid?.formatValue === "function"
        ? (sapGrid.formatValue(activeValue, fmt) ?? sapGrid.formatValue(activeValue, null) ?? String(activeValue))
        : String(activeValue);
    sampleValue.textContent = shown;
  }

  shell.panel.addEventListener(
    "submit",
    (event) => {
      if (!canApply) {
        event.preventDefault();
        event.stopImmediatePropagation();
      }
    },
    true,
  );
  input.addEventListener("input", syncSample);

  handle = mountSheetDialog(doc, shell, {
    title: "Custom number format",
    confirmLabel: "Apply",
    confirmVariant: "primary",
    onConfirm: () => {
      closed = true;
      onConfirm?.(input.value === "" ? null : input.value);
    },
    onCancel: () => {
      closed = true;
      onCancel?.();
    },
    getReturnFocus,
    container,
  });

  syncSample();
  input.focus();
  input.select?.();
  return handle;
}

export function colorMenuItems({ current, onPick }) {
  return [
    { type: "swatches", colors: SWATCHES, current, onSelect: (hex) => onPick(hex) },
    { type: "separator" },
    { id: "reset", label: "Reset", onSelect: () => onPick(null) },
  ];
}

export function borderMenuItems({ onPick }) {
  return BORDER_PRESETS.map((entry, idx) =>
    idx === BORDER_PRESETS.length - 1
      ? [{ type: "separator" }, { id: entry.id, label: entry.label, onSelect: () => onPick(entry.preset) }]
      : [{ id: entry.id, label: entry.label, onSelect: () => onPick(entry.preset) }],
  ).flat();
}

export function alignMenuItems({ current, onPick }) {
  return ALIGN_OPTIONS.map((entry) => ({
    id: entry.id,
    label: entry.label,
    checked: current === entry.align,
    onSelect: () => onPick(entry.align),
  }));
}

export function parseStyleMap(css) {
  const map = Object.create(null);
  if (typeof css !== "string" || !css) return map;
  for (const part of css.split(";")) {
    const colon = part.indexOf(":");
    if (colon === -1) continue;
    const key = part.slice(0, colon).trim().toLowerCase();
    const val = part.slice(colon + 1).trim();
    if (key && val) map[key] = val;
  }
  return map;
}

export function cellStyleState(cell) {
  const style = parseStyleMap(cell?.style);
  const fw = (style["font-weight"] ?? "").toLowerCase();
  const fs = (style["font-style"] ?? "").toLowerCase();
  const td = (style["text-decoration"] ?? "").toLowerCase();
  const ws = (style["white-space"] ?? "").toLowerCase();
  return {
    bold: fw === "bold" || fw === "bolder" || (/^\d+$/.test(fw) && Number(fw) >= 700),
    italic: fs === "italic" || fs === "oblique",
    underline: /\bunderline\b/.test(td),
    strikethrough: /\bline-through\b/.test(td),
    wrap: ws === "pre-wrap",
    align: style["text-align"]?.toLowerCase() ?? null,
    color: style.color?.toLowerCase() ?? null,
    fillColor: (style["background-color"] ?? style.background)?.toLowerCase() ?? null,
    effectiveFmt: cell?.effectiveFmt ?? null,
    formatLabel: formatLabelOf(cell?.effectiveFmt ?? null),
  };
}

function decorationValue(underline, strikethrough) {
  if (underline && strikethrough) return "underline line-through";
  if (underline) return "underline";
  if (strikethrough) return "line-through";
  return null;
}

// Coalesces (r, c) coordinates carrying the same key into maximal A1 rectangles.
function coalesceRectangles(points) {
  const byRow = new Map();
  for (const { r, c } of points) {
    let list = byRow.get(r);
    if (!list) byRow.set(r, (list = []));
    list.push(c);
  }
  const rows = [...byRow.keys()].sort((a, b) => a - b);
  const open = new Map();
  const rects = [];
  let prevRow = -2;
  for (const r of rows) {
    if (r !== prevRow + 1) {
      for (const rect of open.values()) rects.push(rect);
      open.clear();
    }
    prevRow = r;
    const cols = byRow.get(r).sort((a, b) => a - b);
    const runs = [];
    for (const c of cols) {
      const last = runs[runs.length - 1];
      if (last && last.c1 + 1 === c) last.c1 = c;
      else runs.push({ c0: c, c1: c });
    }
    const nextOpen = new Map();
    for (const run of runs) {
      const key = `${run.c0},${run.c1}`;
      const existing = open.get(key);
      if (existing) {
        existing.r1 = r;
        nextOpen.set(key, existing);
        open.delete(key);
      } else {
        nextOpen.set(key, { r0: r, c0: run.c0, r1: r, c1: run.c1 });
      }
    }
    for (const rect of open.values()) rects.push(rect);
    open.clear();
    for (const [k, v] of nextOpen) open.set(k, v);
  }
  for (const rect of open.values()) rects.push(rect);
  return rects.map(({ r0, c0, r1, c1 }) =>
    r0 === r1 && c0 === c1 ? a1(r0, c0) : `${a1(r0, c0)}:${a1(r1, c1)}`,
  );
}

export function applyStyleToggle(grid, view, sel, history, kind) {
  const box = selectionBox(view, sel);
  if (!box) return;
  const active = cellStyleState(grid.cell(a1(box.active.r, box.active.c)));
  const area = commandArea(view, sel);

  if (kind === "bold") {
    grid.format(area, { style: { "font-weight": active.bold ? null : "bold" } });
    return;
  }
  if (kind === "italic") {
    grid.format(area, { style: { "font-style": active.italic ? null : "italic" } });
    return;
  }
  if (kind === "wrap") {
    grid.format(area, { style: { "white-space": active.wrap ? null : "pre-wrap" } });
    return;
  }
  if (kind !== "underline" && kind !== "strikethrough") return;

  const wantVal = kind === "underline" ? !active.underline : !active.strikethrough;
  const buckets = new Map();
  for (let r = box.top; r <= box.bottom; r++) {
    for (let c = box.left; c <= box.right; c++) {
      const cell = grid.cell(a1(r, c));
      if (cell?.covered) continue;
      const st = cellStyleState(cell);
      const next = kind === "underline"
        ? decorationValue(wantVal, st.strikethrough)
        : decorationValue(st.underline, wantVal);
      const key = next ?? "__null__";
      let list = buckets.get(key);
      if (!list) buckets.set(key, (list = { value: next, points: [] }));
      list.points.push({ r, c });
    }
  }
  if (buckets.size === 0) return;
  if (buckets.size === 1) {
    const [{ value }] = buckets.values();
    grid.format(area, { style: { "text-decoration": value } });
    return;
  }
  history.group(sel.tab, () => {
    for (const { value, points } of buckets.values()) {
      for (const rectArea of coalesceRectangles(points)) {
        grid.format(rectArea, { style: { "text-decoration": value } });
      }
    }
  });
}

export function applyAlign(grid, view, sel, align) {
  grid.format(commandArea(view, sel), { style: { "text-align": align } });
}

export function applyTextColor(grid, view, sel, hexOrNull) {
  grid.format(commandArea(view, sel), { style: { color: hexOrNull } });
}

export function applyFillColor(grid, view, sel, hexOrNull) {
  grid.format(commandArea(view, sel), { style: { "background-color": hexOrNull } });
}

export function applyNumberFormat(grid, view, sel, fmt) {
  grid.format(commandArea(view, sel), { fmt });
}

export function applyBorder(grid, view, sel, preset) {
  const border = preset === "none" ? { preset: "none" } : { preset, line: BORDER_LINE };
  grid.format(commandArea(view, sel), { border });
}

export function applyClearFormatting(grid, view, sel, history) {
  const area = commandArea(view, sel);
  history.group(sel.tab, () => {
    grid.format(area, { fmt: null, style: CLEAR_STYLE, border: { preset: "none" } });
  });
}

export function applyDecimals(grid, view, sel, history, delta, adjustDecimals) {
  if (typeof adjustDecimals !== "function") return;
  const box = selectionBox(view, sel);
  if (!box) return;

  const buckets = new Map();
  let totalUncovered = 0;
  for (let r = box.top; r <= box.bottom; r++) {
    for (let c = box.left; c <= box.right; c++) {
      const cell = grid.cell(a1(r, c));
      if (!cell || cell.covered) continue;
      totalUncovered += 1;
      const cur = cell.effectiveFmt ?? null;
      const next = adjustDecimals(cur, delta, cell.value) ?? null;
      if (next === cur) continue;
      let list = buckets.get(next);
      if (!list) buckets.set(next, (list = []));
      list.push({ r, c });
    }
  }
  if (buckets.size === 0) return;

  if (buckets.size === 1) {
    const [[nextFmt, points]] = buckets.entries();
    const colHasFmt = grid.cell(a1(box.top, box.left))?.effectiveFmt !== null;
    if (points.length === totalUncovered && (sel?.whole !== "cols" || colHasFmt)) {
      grid.format(commandArea(view, sel), { fmt: nextFmt });
      return;
    }
  }

  history.group(sel.tab, () => {
    for (const [nextFmt, points] of buckets) {
      for (const rectArea of coalesceRectangles(points)) {
        grid.format(rectArea, { fmt: nextFmt });
      }
    }
  });
}

export function inspectMerge(grid, view, sel, { scan = true } = {}) {
  const box = selectionBox(view, sel);
  if (!box || !grid) return { canAct: false, isSingleMerge: false, area: "A1", box: null, filled: [] };
  const cellCount = (box.bottom - box.top + 1) * (box.right - box.left + 1);
  if (cellCount < 2) return { canAct: false, isSingleMerge: false, area: selectionArea(view, sel), box, filled: [] };

  const tl = ownerOf(view, box.top, box.left);
  const isSingleMerge =
    tl.r === box.top &&
    tl.c === box.left &&
    tl.rows === box.bottom - box.top + 1 &&
    tl.cols === box.right - box.left + 1;
  const area = selectionArea(view, sel);
  if (isSingleMerge) return { canAct: true, isSingleMerge: true, area, box, filled: [] };
  if (!scan) return { canAct: true, isSingleMerge: false, area, box, filled: [] };

  const filled = [];
  for (let r = box.top; r <= box.bottom; r++) {
    for (let c = box.left; c <= box.right; c++) {
      const ref = a1(r, c);
      const cell = grid.cell(ref);
      if (!cell || cell.covered) continue;
      if (cell.content !== null && cell.content !== "") {
        filled.push({ r, c, ref, text: cell.text, content: cell.content, fmt: cell.effectiveFmt ?? null });
      }
    }
  }
  return { canAct: true, isSingleMerge: false, area, box, filled };
}

export function mergeKeepingChoice(grid, box, area, chosen, history, tab) {
  history.group(tab, () => {
    grid.merge(area);
    if (chosen.r !== box.top || chosen.c !== box.left) {
      grid.setRange(a1(box.top, box.left), [[{ content: chosen.content, fmt: chosen.fmt ?? null }]], { detect: false });
    }
  });
}

export function runMergeCommand({ grid, view, sel, history, runWrite, promptKeep }) {
  const state = inspectMerge(grid, view, sel);
  if (!state.canAct) return false;
  if (state.isSingleMerge) {
    return runWrite(() => grid.unmerge(a1(state.box.top, state.box.left)));
  }
  if (state.filled.length === 0) return runWrite(() => grid.merge(state.area));
  if (state.filled.length === 1) {
    const [only] = state.filled;
    return runWrite(() => mergeKeepingChoice(grid, state.box, state.area, only, history, sel.tab));
  }
  promptKeep(state.filled, (chosen) => {
    runWrite(() => mergeKeepingChoice(grid, state.box, state.area, chosen, history, sel.tab));
  });
  return true;
}

const DIGITS = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz";
const VALID_KEY = /^[0-9A-Za-z]*[1-9A-Za-z]$/;
const MAX_COUNT = 9999999;

export const isKey = (key) => typeof key === "string" && VALID_KEY.test(key);

function midpoint(a, b) {
  if (b !== null) {
    let n = 0;
    while ((a[n] || "0") === b[n]) n++;
    if (n > 0) return b.slice(0, n) + midpoint(a.slice(n), b.slice(n));
  }
  const da = a ? DIGITS.indexOf(a[0]) : 0;
  const db = b !== null ? DIGITS.indexOf(b[0]) : DIGITS.length;
  if (db - da > 1) return DIGITS[Math.round((da + db) / 2)];
  if (b !== null && b.length > 1) return b.slice(0, 1);
  return DIGITS[da] + midpoint(a.slice(1), null);
}

export function keyBetween(a = null, b = null) {
  const low = a && isKey(a) ? a : null;
  const high = b && isKey(b) && (!low || low < b) ? b : null;
  return midpoint(low ?? "", high);
}

export function keysBetween(a, b, n) {
  if (n <= 0) return [];
  if (n === 1) return [keyBetween(a, b)];
  const mid = keyBetween(a, b);
  const half = Math.floor(n / 2);
  return [...keysBetween(a, mid, half), mid, ...keysBetween(mid, b, n - half - 1)];
}

export function nextSheetName(tabs) {
  const taken = new Set(tabs.map((t) => t.name.toLowerCase()));
  for (let n = 1; ; n++) {
    const candidate = `Sheet ${n}`;
    if (!taken.has(candidate.toLowerCase())) return candidate;
  }
}

export function nextCopyName(baseName, tabs) {
  const taken = new Set(tabs.map((t) => t.name.toLowerCase()));
  const first = `Copy of ${baseName}`;
  if (!taken.has(first.toLowerCase())) return first;
  for (let n = 2; ; n++) {
    const candidate = `Copy of ${baseName} ${n}`;
    if (!taken.has(candidate.toLowerCase())) return candidate;
  }
}

export function mintTabId(tabs, newId) {
  const taken = new Set(tabs.map((t) => t.id));
  for (let tries = 0; tries < 100; tries++) {
    const id = typeof newId === "function" ? newId("s") : `s${Date.now().toString(36)}${tries}`;
    if (id && !taken.has(id)) return id;
  }
  return `s${Date.now().toString(36)}`;
}

const cleanText = (text) => String(text ?? "").replace(/\r\n?/g, "\n").replace(/\0/g, "");

function parseCount(raw, min) {
  if (raw === null || !/^[0-9]{1,7}$/.test(raw.trim())) return undefined;
  const n = Number(raw.trim());
  return n >= min && n <= MAX_COUNT ? n : undefined;
}

function readDomCell(td) {
  const cell = { text: cleanText(td.textContent) };
  let bare = true;
  const set = (key, value) => {
    if (value === undefined || value === null || value === false) return;
    cell[key] = value;
    bare = false;
  };
  set("n", td.hasAttribute("n"));
  set("t", td.hasAttribute("t"));
  for (const key of ["f", "v", "fmt"]) {
    const val = td.getAttribute(key);
    set(key, val === null ? undefined : cleanText(val));
  }
  set("colspan", parseCount(td.getAttribute("colspan"), 2));
  set("rowspan", parseCount(td.getAttribute("rowspan"), 2));
  if (td.hasAttribute("hidden")) {
    set("hidden", true);
    cell.text = "";
  }
  const style = td.getAttribute("style");
  set("style", style === null ? undefined : cleanText(style));
  return bare && cell.text === "" ? null : cell;
}

function readDomRow(tr, frozen) {
  const row = {
    id: cleanText(tr.getAttribute("data-id") ?? ""),
    pos: cleanText(tr.getAttribute("data-pos") ?? ""),
  };
  if (frozen) row.frozen = true;
  if (tr.hasAttribute("hidden")) row.hidden = true;
  const style = tr.getAttribute("style");
  if (style !== null) row.style = cleanText(style);
  row.cells = [...tr.children]
    .filter((el) => el.localName === "td" || el.localName === "th")
    .map(readDomCell);
  return row;
}

function readDomCol(colEl) {
  const col = {
    id: cleanText(colEl.getAttribute("data-id") ?? ""),
    pos: cleanText(colEl.getAttribute("data-pos") ?? ""),
  };
  const fmt = colEl.getAttribute("fmt");
  if (fmt !== null) col.fmt = cleanText(fmt);
  if (colEl.hasAttribute("hidden")) col.hidden = true;
  const width = /width:\s*([0-9]{1,7})px/.exec(colEl.getAttribute("style") ?? "");
  if (width) col.width = Number(width[1]);
  return col;
}

export function emptyTableHtml(tabId, sapGrid, { colsCount = 26, rowsCount = 100 } = {}) {
  const taken = new Set();
  const mint = (prefix) => {
    for (let i = 0; i < 100; i++) {
      const id = sapGrid.newId(prefix);
      if (!taken.has(id)) {
        taken.add(id);
        return id;
      }
    }
    return `${prefix}${taken.size + 1}`;
  };
  const cols = keysBetween(null, null, colsCount).map((pos) => ({ id: mint("c"), pos }));
  const rows = keysBetween(null, null, rowsCount).map((pos) => ({ id: mint("r"), pos, cells: [] }));
  return sapGrid.createSheetWriter().table(tabId, { cols, rows });
}

export function duplicateTableHtml(tabId, table, sapGrid) {
  const sheet = { cols: [], rows: [] };
  const freezeCols = parseCount(table.getAttribute("freeze-cols"), 1);
  if (freezeCols !== undefined) sheet.freezeCols = freezeCols;
  if (table.hasAttribute("stale")) sheet.stale = true;
  const frozen = [];
  const data = [];
  for (const part of table.children) {
    const name = part.localName;
    if (name === "colgroup") {
      for (const col of part.children) if (col.localName === "col") sheet.cols.push(readDomCol(col));
    } else if (name === "col") {
      sheet.cols.push(readDomCol(part));
    } else if (name === "thead" || name === "tbody" || name === "tfoot") {
      const into = name === "thead" ? frozen : data;
      for (const tr of part.children) if (tr.localName === "tr") into.push(readDomRow(tr, name === "thead"));
    } else if (name === "tr") {
      data.push(readDomRow(part, false));
    }
  }
  sheet.rows = [...frozen, ...data];
  const source = sapGrid(table);
  if (source) sheet.rules = source.rules();
  return sapGrid.createSheetWriter().table(tabId, sheet);
}

export function frozenOf(table) {
  const rows = table?.tHead?.rows.length ?? 0;
  const rawCols = Number.parseInt(table?.getAttribute("freeze-cols") ?? "", 10);
  const cols = Number.isInteger(rawCols) && rawCols > 0 ? rawCols : 0;
  return { rows, cols };
}

export function appendBlankTab({ tabs, sapGrid, gridOf, runTabStep, customName = null, rowsCount = 100, colsCount = 26 }) {
  const list = tabs.list();
  const grid = gridOf(list[0]?.table);
  if (!grid || typeof sapGrid?.createSheetWriter !== "function") return null;
  let maxPos = null;
  for (const t of list) {
    if (isKey(t.pos) && (maxPos === null || maxPos < t.pos)) maxPos = t.pos;
  }
  const pos = keyBetween(maxPos, null);
  const id = mintTabId(list, sapGrid.newId);
  const name = customName ?? nextSheetName(list);
  const html = emptyTableHtml(id, sapGrid, { colsCount, rowsCount });
  if (!runTabStep(() => grid.dispatch({ v: 1, t: "insertSheet", sheet: { id, name, pos, html } }))) return null;
  return { id, name };
}

export function createCommandActions({
  doc,
  win,
  workbook,
  chrome,
  overlays,
  editor,
  tabs,
  history,
  menu,
  getSel,
  viewOf,
  gridOf,
  windowOf,
  activeCellInfo,
  ensureReadyForAction,
  runFormat,
  writeGrid,
  runTabStep,
  syncUI,
  refreshStatus = () => {},
  applySelection,
  applyMove,
  subscribeGrids,
  restart,
  showDialog,
  clearDialog,
  onFillShortcut,
  onPasteSpecial,
  onClipboard,
  onShortcuts,
  onDropdown,
  onConditionalFormat,
  onInsertChart,
  onEditChart,
  onDeleteChart,
  isMac = () => false,
  measureText: customMeasure = null,
}) {
  let canvasCtx = null;
  let canvasTried = false;
  const measure = customMeasure ?? ((text, font) => {
    if (!canvasTried) {
      canvasTried = true;
      try {
        canvasCtx = doc.createElement("canvas").getContext?.("2d") ?? null;
      } catch {
        canvasCtx = null;
      }
    }
    if (!canvasCtx || typeof canvasCtx.measureText !== "function") return text.length * 8;
    if (font) canvasCtx.font = font;
    return canvasCtx.measureText(text).width;
  });

  function fitColumnWidth(table, rw, view, c) {
    if (!table || !view || c < 0 || c >= view.cols.length) return null;
    const cs = win.getComputedStyle(table);
    const font = cs.font || `${cs.fontWeight || "400"} ${cs.fontSize || "13px"} ${cs.fontFamily || "sans-serif"}`.trim();
    const visRows = overlays.visibleRows(table, rw, view.rows.length);
    let maxText = 0;
    let found = false;
    for (const r of visRows) {
      if (view.rowHidden?.(r)) continue;
      const cell = view.cell(r, c);
      if (!cell || cell.covered || (cell.span?.cols ?? 1) > 1 || !cell.text) continue;
      found = true;
      for (const line of cell.text.split("\n")) maxText = Math.max(maxText, measure(line, font) || 0);
    }
    return found ? Math.min(600, Math.max(8, Math.ceil(maxText + 12))) : null;
  }
  function insertRowsAt(insertIndex, count, likeRow, wholeRows) {
    const sel = getSel();
    if (!sel) return;
    const prevView = viewOf(sel.tab);
    const prevBox = selectionBox(prevView, sel);
    if (!prevBox) return;
    if (!runFormat((grid) => grid.insertRows(insertIndex + 1, count, { like: likeRow }))) return;
    const nextView = viewOf(sel.tab);
    if (nextView.rows.length === 0 || nextView.cols.length === 0) return;
    const top = Math.min(insertIndex, nextView.rows.length - 1);
    const bottom = Math.min(insertIndex + count - 1, nextView.rows.length - 1);
    const nextSel = wholeRows
      ? {
          tab: sel.tab,
          anchor: { row: nextView.rows[top], col: nextView.cols[0] },
          focus: { row: nextView.rows[bottom], col: nextView.cols[nextView.cols.length - 1] },
          whole: "rows",
        }
      : (() => {
          const own = ownerOf(nextView, top, Math.min(prevBox.left, nextView.cols.length - 1));
          return {
            tab: sel.tab,
            anchor: { row: nextView.rows[own.r], col: nextView.cols[own.c] },
            focus: { row: nextView.rows[bottom], col: nextView.cols[Math.min(prevBox.right, nextView.cols.length - 1)] },
          };
        })();
    applyMove(nextSel);
  }

  function insertColsAt(insertIndex, count, likeCol, wholeCols) {
    const sel = getSel();
    if (!sel) return;
    const prevView = viewOf(sel.tab);
    const prevBox = selectionBox(prevView, sel);
    if (!prevBox) return;
    if (!runFormat((grid) => grid.insertCols(colName(insertIndex), count, { like: likeCol }))) return;
    const nextView = viewOf(sel.tab);
    if (nextView.rows.length === 0 || nextView.cols.length === 0) return;
    const left = Math.min(insertIndex, nextView.cols.length - 1);
    const right = Math.min(insertIndex + count - 1, nextView.cols.length - 1);
    const nextSel = wholeCols
      ? {
          tab: sel.tab,
          anchor: { row: nextView.rows[0], col: nextView.cols[left] },
          focus: { row: nextView.rows[nextView.rows.length - 1], col: nextView.cols[right] },
          whole: "cols",
        }
      : (() => {
          const own = ownerOf(nextView, Math.min(prevBox.top, nextView.rows.length - 1), left);
          return {
            tab: sel.tab,
            anchor: { row: nextView.rows[own.r], col: nextView.cols[own.c] },
            focus: { row: nextView.rows[Math.min(prevBox.bottom, nextView.rows.length - 1)], col: nextView.cols[right] },
          };
        })();
    applyMove(nextSel);
  }

  const deleteRowsAt = (top, bottom) => runFormat((grid) => grid.deleteRows(`${top + 1}:${bottom + 1}`));
  const deleteColsAt = (left, right) => runFormat((grid) => grid.deleteCols(`${colName(left)}:${colName(right)}`));

  function sortSheetByCol(tabId, col, direction) {
    if (editor.editing && editor.canPick()) return editor.focus();
    if (!ensureReadyForAction()) return;
    const table = tabTable(workbook, tabId);
    const grid = gridOf(table);
    if (!grid) return;
    const view = viewOf(tabId);
    const frozen = frozenOf(table);
    const action = buildSheetSortAction(grid, view, {
      sheet: tabId,
      col,
      frozenRows: frozen.rows,
      direction,
    });
    if (!action) return;
    if (!writeGrid(() => grid.dispatch(action))) return;
    refreshStatus();
    syncUI({ announce: true });
    editor.focus();
  }

  function sortSelectedRange(tabId, direction) {
    if (editor.editing && editor.canPick()) return editor.focus();
    if (!ensureReadyForAction()) return;
    const curSel = getSel();
    if (!curSel || curSel.tab !== tabId) return;
    const table = tabTable(workbook, tabId);
    const grid = gridOf(table);
    if (!grid) return;
    const view = viewOf(tabId);
    const box = selectionBox(view, curSel);
    if (!box) return;
    const anchorCol = view.cols.indexOf(curSel.anchor?.col ?? "");
    const col = curSel.whole === "cols" && anchorCol >= box.left && anchorCol <= box.right
      ? anchorCol
      : box.active.c;
    const frozenRows = frozenOf(table).rows;
    const sortBox = curSel.whole === "cols" ? { ...box, top: Math.max(box.top, frozenRows) } : box;
    const action = buildRangeSortAction(grid, view, { sheet: tabId, box: sortBox, col, direction });
    if (!action) return;
    const prevWhole = curSel.whole ?? null;
    const ar = view.rows.indexOf(curSel.anchor?.row ?? "");
    const ac = view.cols.indexOf(curSel.anchor?.col ?? "");
    const fr = view.rows.indexOf(curSel.focus?.row ?? "");
    const fc = view.cols.indexOf(curSel.focus?.col ?? "");
    const ok = writeGrid(() => {
      if (sortBox.top < frozenRows) {
        throw Object.assign(new Error("Sort can't move a frozen row. Select rows below the frozen ones."), {
          name: "GridRefused",
          code: "bad_action",
        });
      }
      if (action.cols !== null && rangeAnchorCutAtRight(view, sortBox)) {
        throw Object.assign(new Error("Sort can't move part of a merged cell."), {
          name: "GridRefused",
          code: "merge_cut",
        });
      }
      grid.dispatch(action);
    });
    if (!ok) return;
    if (!prevWhole && ar >= 0 && ac >= 0 && fr >= 0 && fc >= 0) {
      const nextView = viewOf(tabId);
      if (nextView.rows.length > 0 && nextView.cols.length > 0) {
        const ownA = ownerOf(
          nextView,
          Math.min(ar, nextView.rows.length - 1),
          Math.min(ac, nextView.cols.length - 1),
        );
        applySelection({
          tab: tabId,
          anchor: { row: nextView.rows[ownA.r], col: nextView.cols[ownA.c] },
          focus: {
            row: nextView.rows[Math.min(fr, nextView.rows.length - 1)],
            col: nextView.cols[Math.min(fc, nextView.cols.length - 1)],
          },
        });
        editor.focus();
        return;
      }
    }
    refreshStatus();
    syncUI({ announce: true });
    editor.focus();
  }

  function openRowHeaderMenu(r, anchor, x, y) {
    if (editor.editing && editor.canPick()) return editor.focus();
    if (!ensureReadyForAction() || !getSel()) return;
    const sel = getSel();
    const view = viewOf(sel.tab);
    const curSpan = axisSpan(view, sel);
    if (curSpan?.axis !== "rows" || r < curSpan.first || r > curSpan.last) {
      applySelection(selectRowsAt(view, sel, r));
    }
    const span = axisSpan(viewOf(getSel().tab), getSel());
    if (!span) return;
    const count = span.last - span.first + 1;
    const frozen = frozenOf(tabTable(workbook, getSel().tab));
    menu.open({
      anchor: x === null ? anchor : null,
      x: x ?? 0,
      y: y ?? 0,
      label: `Row ${r + 1} options`,
      items: [
        { id: "insert-above", label: `Insert ${count} above`, onSelect: () => insertRowsAt(span.first, count, span.first + 1, true) },
        { id: "insert-below", label: `Insert ${count} below`, onSelect: () => insertRowsAt(span.last + 1, count, span.last + 1, true) },
        { id: "delete-rows", label: "Delete rows", onSelect: () => deleteRowsAt(span.first, span.last) },
        { type: "separator" },
        { id: "freeze-rows", label: `Freeze up to row ${r + 1}`, onSelect: () => runFormat((g) => g.freeze({ rows: r + 1, cols: frozen.cols })) },
        { id: "unfreeze-rows", label: "Unfreeze rows", disabled: frozen.rows === 0, onSelect: () => runFormat((g) => g.freeze({ rows: 0, cols: frozen.cols })) },
      ],
    });
  }

  function openColHeaderMenu(c, anchor, x, y) {
    if (editor.editing && editor.canPick()) return editor.focus();
    if (!ensureReadyForAction() || !getSel()) return;
    const sel = getSel();
    const view = viewOf(sel.tab);
    const curSpan = axisSpan(view, sel);
    if (curSpan?.axis !== "cols" || c < curSpan.first || c > curSpan.last) {
      applySelection(selectColsAt(view, sel, c));
    }
    const curSel = getSel();
    const tabId = curSel.tab;
    const curView = viewOf(tabId);
    const span = axisSpan(curView, curSel);
    if (!span) return;
    const count = span.last - span.first + 1;
    const frozen = frozenOf(tabTable(workbook, tabId));
    const letter = colName(c);
    const isFullSheetCols = span.first === 0 && span.last === curView.cols.length - 1;
    const canSortSheet = curView.rows.length - frozen.rows >= 2;
    const box = selectionBox(curView, curSel);
    const canSortRange = Boolean(box && box.bottom - Math.max(box.top, frozen.rows) + 1 >= 2);
    menu.open({
      anchor: x === null ? anchor : null,
      x: x ?? 0,
      y: y ?? 0,
      label: `Column ${letter} options`,
      items: [
        { id: "insert-left", label: `Insert ${count} left`, onSelect: () => insertColsAt(span.first, count, colName(span.first), true) },
        { id: "insert-right", label: `Insert ${count} right`, onSelect: () => insertColsAt(span.last + 1, count, colName(span.last), true) },
        { id: "delete-cols", label: "Delete columns", onSelect: () => deleteColsAt(span.first, span.last) },
        { type: "separator" },
        { id: "sort-sheet-asc", label: "Sort sheet A to Z", disabled: !canSortSheet, onSelect: () => sortSheetByCol(tabId, c, "asc") },
        { id: "sort-sheet-desc", label: "Sort sheet Z to A", disabled: !canSortSheet, onSelect: () => sortSheetByCol(tabId, c, "desc") },
        ...(!isFullSheetCols
          ? [
              { id: "sort-range-asc", label: "Sort range A to Z", disabled: !canSortRange, onSelect: () => sortSelectedRange(tabId, "asc") },
              { id: "sort-range-desc", label: "Sort range Z to A", disabled: !canSortRange, onSelect: () => sortSelectedRange(tabId, "desc") },
            ]
          : []),
        { type: "separator" },
        { id: "freeze-cols", label: `Freeze up to column ${letter}`, onSelect: () => runFormat((g) => g.freeze({ rows: frozen.rows, cols: c + 1 })) },
        { id: "unfreeze-cols", label: "Unfreeze columns", disabled: frozen.cols === 0, onSelect: () => runFormat((g) => g.freeze({ rows: frozen.rows, cols: 0 })) },
      ],
    });
  }

  function openCellContextMenu(r, c, x, y, { touch = false } = {}) {
    const sel = getSel();
    if (!sel) return;
    const tabId = sel.tab;
    const view = viewOf(tabId);
    const box = selectionBox(view, sel);
    let sourceBottom = box ? box.top : 0;
    let sourceRight = box ? box.left : 0;
    if (box) {
      for (let col = box.left; col <= box.right; col++) {
        const own = ownerOf(view, box.top, col);
        sourceBottom = Math.max(sourceBottom, own.r + own.rows - 1);
      }
      for (let row = box.top; row <= box.bottom; row++) {
        const own = ownerOf(view, row, box.left);
        sourceRight = Math.max(sourceRight, own.c + own.cols - 1);
      }
    }
    const canFillDown = Boolean(box && box.bottom > sourceBottom);
    const canFillRight = Boolean(box && box.right > sourceRight);
    const canSortRange = Boolean(box && box.bottom - box.top + 1 >= 2);
    const mac = Boolean(isMac?.());
    menu.open({
      x,
      y,
      label: "Cell options",
      items: [
        ...(touch
          ? [
              { id: "cut", label: "Cut", onSelect: () => onClipboard?.("cut") },
              { id: "copy", label: "Copy", onSelect: () => onClipboard?.("copy") },
              { id: "paste", label: "Paste", onSelect: () => onClipboard?.("paste") },
              { type: "separator" },
            ]
          : []),
        {
          id: "paste-special",
          label: "Paste special",
          items: [
            {
              id: "paste-values",
              label: "Values only",
              detail: mac ? "⌘⇧V" : "Ctrl+Shift+V",
              onSelect: () => onPasteSpecial?.("values"),
            },
            { id: "paste-format", label: "Format only", onSelect: () => onPasteSpecial?.("format") },
            { id: "paste-transpose", label: "Transposed", onSelect: () => onPasteSpecial?.("transpose") },
          ],
        },
        { type: "separator" },
        { id: "fill-down", label: "Fill down", disabled: !canFillDown, onSelect: () => onFillShortcut?.("down") },
        { id: "fill-right", label: "Fill right", disabled: !canFillRight, onSelect: () => onFillShortcut?.("right") },
        { type: "separator" },
        { id: "sort-range-asc", label: "Sort range A to Z", disabled: !canSortRange, onSelect: () => sortSelectedRange(tabId, "asc") },
        { id: "sort-range-desc", label: "Sort range Z to A", disabled: !canSortRange, onSelect: () => sortSelectedRange(tabId, "desc") },
        { type: "separator" },
        { id: "insert-row-above", label: "Insert row above", onSelect: () => insertRowsAt(r, 1, r + 1, false) },
        { id: "insert-col-left", label: "Insert column left", onSelect: () => insertColsAt(c, 1, colName(c), false) },
        { type: "separator" },
        { id: "delete-row", label: "Delete row", onSelect: () => deleteRowsAt(r, r) },
        { id: "delete-col", label: "Delete column", onSelect: () => deleteColsAt(c, c) },
        ...(gridOf(tabTable(workbook, tabId))
          ? [
              { type: "separator" },
              { id: "dropdown", label: "Dropdown…", disabled: editor.editing || !box, onSelect: () => onDropdown?.() },
              { id: "conditional-format", label: "Conditional formatting…", disabled: editor.editing, onSelect: () => onConditionalFormat?.() },
              { id: "chart", label: "Insert chart", disabled: editor.editing || !box, onSelect: () => onInsertChart?.() },
            ]
          : []),
        { type: "separator" },
        {
          id: "shortcuts",
          label: "Keyboard shortcuts",
          detail: mac ? "⌘/" : "Ctrl+/",
          onSelect: () => onShortcuts?.(),
        },
      ],
    });
  }

  function openChartContextMenu(ruleId, x, y) {
    if (!ensureReadyForAction()) return;
    menu.open({
      x: x ?? 0,
      y: y ?? 0,
      label: "Chart options",
      items: [
        { id: "edit-chart", label: "Edit chart…", onSelect: () => onEditChart?.(ruleId) },
        { id: "delete-chart", label: "Delete chart", danger: true, onSelect: () => onDeleteChart?.(ruleId) },
      ],
    });
  }

  function handleAddTab() {
    if (!ensureReadyForAction()) return;
    const added = appendBlankTab({ tabs, sapGrid: win.Sap?.grid, gridOf, runTabStep });
    if (!added) return;
    subscribeGrids();
    const view = viewOf(added.id);
    tabs.show(added.id);
    applySelection(createSelection(view, added.id), { announce: true, enteredTab: true });
    editor.focus();
  }

  function handleDuplicateTab(tabId) {
    if (!ensureReadyForAction()) return;
    const list = tabs.list();
    const idx = list.findIndex((t) => t.id === tabId);
    if (idx === -1) return;
    const srcTab = list[idx];
    const sapGrid = win.Sap?.grid;
    const grid = gridOf(srcTab.table);
    if (!grid || typeof sapGrid?.createSheetWriter !== "function") return;
    const low = isKey(srcTab.pos) ? srcTab.pos : null;
    let end = idx + 1;
    while (end < list.length && (!isKey(list[end].pos) || (low !== null && list[end].pos <= low))) end += 1;
    const high = end < list.length && isKey(list[end].pos) ? list[end].pos : null;
    const keys = keysBetween(low, high, 1 + (end - (idx + 1)));
    const newId = mintTabId(list, sapGrid.newId);
    const name = nextCopyName(srcTab.name, list);
    const html = duplicateTableHtml(newId, srcTab.table, sapGrid);
    const ok = runTabStep(() => {
      for (let k = idx + 1; k < end; k++) {
        grid.dispatch({ v: 1, t: "moveSheet", sheet: list[k].id, pos: keys[1 + (k - (idx + 1))] });
      }
      grid.dispatch({ v: 1, t: "insertSheet", sheet: { id: newId, name, pos: keys[0], html } });
    });
    if (!ok) return;
    subscribeGrids();
    const view = viewOf(newId);
    tabs.show(newId);
    applySelection(createSelection(view, newId), { announce: true, enteredTab: true });
    editor.focus();
  }

  function handleMoveTab(tabId, delta) {
    if (!ensureReadyForAction()) return;
    const list = tabs.list();
    const idx = list.findIndex((t) => t.id === tabId);
    if (idx === -1) return;
    const targetIdx = idx + delta;
    if (targetIdx < 0 || targetIdx >= list.length) return;
    const grid = gridOf(list[idx].table);
    if (!grid) return;
    const others = list.filter((_, i) => i !== idx);
    const low = targetIdx > 0 && isKey(others[targetIdx - 1].pos) ? others[targetIdx - 1].pos : null;
    let end = targetIdx;
    while (end < others.length && (!isKey(others[end].pos) || (low !== null && others[end].pos <= low))) end += 1;
    const high = end < others.length && isKey(others[end].pos) ? others[end].pos : null;
    const keys = keysBetween(low, high, 1 + (end - targetIdx));
    const ok = runTabStep(() => {
      for (let k = targetIdx; k < end; k++) {
        grid.dispatch({ v: 1, t: "moveSheet", sheet: others[k].id, pos: keys[1 + (k - targetIdx)] });
      }
      grid.dispatch({ v: 1, t: "moveSheet", sheet: tabId, pos: keys[0] });
    });
    if (!ok) return;
    syncUI();
    editor.focus();
  }

  function handleDeleteTab(tabId) {
    if (!ensureReadyForAction()) return;
    const tab = tabs.list().find((t) => t.id === tabId);
    if (!tab) return;
    showDialog(tabId, openDeleteTabDialog({
      doc,
      theme: chrome.theme,
      container: chrome.topRoot,
      tabName: tab.name,
      getReturnFocus: () => editor.textarea,
      onConfirm: () => {
        clearDialog();
        const freshTab = tabs.list().find((t) => t.id === tabId);
        const grid = gridOf(freshTab?.table ?? tabs.list()[0]?.table);
        if (!grid) return;
        if (!runTabStep(() => grid.dispatch({ v: 1, t: "deleteSheet", sheet: tabId }))) return;
        restart();
      },
      onCancel: () => {
        clearDialog();
      },
    }));
  }

  function openTabContextMenu(tabId, btn, x, y) {
    if (!ensureReadyForAction()) return;
    const list = tabs.list();
    const idx = list.findIndex((t) => t.id === tabId);
    if (idx === -1) return;
    const rect = btn?.getBoundingClientRect?.();
    const menuX = x || rect?.left || 0;
    const menuY = y || (rect ? rect.top - 4 : 0);
    menu.open({
      x: menuX,
      y: menuY,
      label: `Sheet ${list[idx].name} options`,
      items: [
        { id: "rename-tab", label: "Rename", onSelect: () => startTabRename(tabId) },
        { id: "duplicate-tab", label: "Duplicate", onSelect: () => handleDuplicateTab(tabId) },
        { type: "separator" },
        { id: "move-tab-left", label: "Move left", disabled: idx <= 0, onSelect: () => handleMoveTab(tabId, -1) },
        { id: "move-tab-right", label: "Move right", disabled: idx >= list.length - 1, onSelect: () => handleMoveTab(tabId, 1) },
        { type: "separator" },
        { id: "delete-tab", label: "Delete", danger: true, onSelect: () => handleDeleteTab(tabId) },
      ],
    });
  }

  function openCustomNumberFormat() {
    if (!ensureReadyForAction()) return;
    const sel = getSel();
    if (!sel) return;
    const info = activeCellInfo();
    showDialog(
      sel.tab,
      openCustomFormatDialog({
        doc,
        win,
        theme: chrome.theme,
        container: chrome.topRoot,
        initialFmt: info?.effectiveFmt ?? "",
        activeValue: info?.cell?.value ?? null,
        getReturnFocus: () => editor.textarea,
        onConfirm: (fmt) => {
          clearDialog();
          runFormat((g, v) => applyNumberFormat(g, v, getSel(), fmt));
        },
        onCancel: () => {
          clearDialog();
        },
      }),
    );
  }

  function openNumberFormatMenu() {
    const info = activeCellInfo();
    menu.open({
      anchor: chrome.getButton("number-format"),
      label: "Number format",
      items: numberFormatMenuItems({
        activeValue: info?.cell?.value ?? null,
        activeFmt: info?.effectiveFmt ?? null,
        formatValue: win.Sap?.grid?.formatValue,
        onPick: (fmt) => runFormat((g, v) => applyNumberFormat(g, v, getSel(), fmt)),
        onCustom: openCustomNumberFormat,
      }),
    });
  }

  function openColorMenu(kind) {
    const info = activeCellInfo();
    const st = cellStyleState(info?.cell);
    const isText = kind === "text";
    menu.open({
      anchor: chrome.getButton(isText ? "text-color" : "fill-color"),
      label: isText ? "Text color" : "Fill color",
      items: colorMenuItems({
        current: isText ? st.color : st.fillColor,
        onPick: (hex) => runFormat((g, v) => (isText ? applyTextColor(g, v, getSel(), hex) : applyFillColor(g, v, getSel(), hex))),
      }),
    });
  }

  function openBordersMenu() {
    menu.open({
      anchor: chrome.getButton("borders"),
      label: "Borders",
      items: borderMenuItems({
        onPick: (preset) => runFormat((g, v) => applyBorder(g, v, getSel(), preset)),
      }),
    });
  }

  function openAlignMenu() {
    const info = activeCellInfo();
    const st = cellStyleState(info?.cell);
    menu.open({
      anchor: chrome.getButton("align"),
      label: "Horizontal align",
      items: alignMenuItems({
        current: st.align,
        onPick: (align) => runFormat((g, v) => applyAlign(g, v, getSel(), align)),
      }),
    });
  }

  function triggerMerge() {
    const sel = getSel();
    if (editor.editing || !sel) return;
    const table = tabTable(workbook, sel.tab);
    const grid = gridOf(table);
    if (!grid) return;
    const view = viewOf(sel.tab);
    runMergeCommand({
      grid,
      view,
      sel,
      history,
      runWrite: (fn) => {
        const ok = writeGrid(fn);
        if (ok) syncUI({ announce: true });
        return ok;
      },
      promptKeep: (filled, onChoose) => {
        showDialog(sel.tab, openMergeKeepDialog({
          doc,
          theme: chrome.theme,
          container: chrome.topRoot,
          items: filled,
          totalCount: filled.length,
          getReturnFocus: () => editor.textarea,
          onConfirm: (chosen) => {
            clearDialog();
            onChoose(chosen);
          },
          onCancel: () => {
            clearDialog();
          },
        }));
      },
    });
  }

  function handleInsertAxisShortcut() {
    const sel = getSel();
    if (!sel) return;
    const view = viewOf(sel.tab);
    const span = axisSpan(view, sel);
    if (span?.axis === "cols") {
      insertColsAt(span.first, span.last - span.first + 1, colName(span.first), true);
      return;
    }
    if (span?.axis === "rows") {
      insertRowsAt(span.first, span.last - span.first + 1, span.first + 1, true);
      return;
    }
    const box = selectionBox(view, sel);
    if (box) insertRowsAt(box.top, 1, box.top + 1, false);
  }

  function handleDeleteAxisShortcut() {
    const sel = getSel();
    if (!sel) return;
    const span = axisSpan(viewOf(sel.tab), sel);
    if (span?.axis === "rows") deleteRowsAt(span.first, span.last);
    else if (span?.axis === "cols") deleteColsAt(span.first, span.last);
  }

  function handleContextMenuShortcut() {
    const sel = getSel();
    if (!sel) return;
    const view = viewOf(sel.tab);
    const span = axisSpan(view, sel);
    if (span?.axis === "rows") {
      const hdr = overlays.shadow.querySelector(`[data-row="${span.first}"]`);
      openRowHeaderMenu(span.first, hdr, null, null);
      return;
    }
    if (span?.axis === "cols") {
      const hdr = overlays.shadow.querySelector(`[data-col="${span.first}"]`);
      openColHeaderMenu(span.first, hdr, null, null);
      return;
    }
    const box = selectionBox(view, sel);
    if (!box) return;
    const table = tabTable(workbook, sel.tab);
    const cb = overlays.cellBox(table, windowOf(table), box.active.r, box.active.c);
    openCellContextMenu(box.active.r, box.active.c, cb?.left ?? 0, (cb?.top ?? 0) + (cb?.height ?? 0));
  }

  function handleResizeCol(c, px) {
    if (editor.editing && editor.canPick()) return editor.focus();
    if (!ensureReadyForAction() || !getSel()) return;
    const sel = getSel();
    const span = axisSpan(viewOf(sel.tab), sel);
    const multi = span?.axis === "cols" && c >= span.first && c <= span.last;
    const area = multi ? `${colName(span.first)}:${colName(span.last)}` : `${colName(c)}:${colName(c)}`;
    runFormat((grid) => grid.setColWidth(area, Math.max(8, Math.round(px))));
    editor.focus();
  }

  function handleResizeRow(r, px) {
    if (editor.editing && editor.canPick()) return editor.focus();
    if (!ensureReadyForAction() || !getSel()) return;
    const sel = getSel();
    const span = axisSpan(viewOf(sel.tab), sel);
    const multi = span?.axis === "rows" && r >= span.first && r <= span.last;
    const first = multi ? span.first : r;
    const last = multi ? span.last : r;
    runFormat((grid) => grid.setRowHeight(`${first + 1}:${last + 1}`, Math.max(12, Math.round(px))));
    editor.focus();
  }

  function handleFitCol(c) {
    if (editor.editing && editor.canPick()) return editor.focus();
    if (!ensureReadyForAction() || !getSel()) return;
    const sel = getSel();
    const table = tabTable(workbook, sel.tab);
    const view = viewOf(sel.tab);
    const span = axisSpan(view, sel);
    const rw = windowOf(table);
    const cols = span?.axis === "cols" && c >= span.first && c <= span.last
      ? Array.from({ length: span.last - span.first + 1 }, (_, i) => span.first + i)
      : [c];
    runFormat((grid) => {
      if (cols.length === 1) {
        const px = fitColumnWidth(table, rw, view, cols[0]);
        if (px !== null) grid.setColWidth(`${colName(cols[0])}:${colName(cols[0])}`, px);
        return;
      }
      history.group(sel.tab, () => {
        for (const col of cols) {
          const px = fitColumnWidth(table, rw, view, col);
          if (px !== null) grid.setColWidth(`${colName(col)}:${colName(col)}`, px);
        }
      });
    });
    editor.focus();
  }

  function startTabRename(tabId) {
    return chrome.mountTabRename(tabId, ({ tab, theme, isCurrent, finish }) => {
      const input = bevelInput("input", {
        theme,
        rules: [
          "display:inline-block",
          "width:116px",
          "height:26px",
          "block-size:26px",
          "min-block-size:26px",
          "padding:0 8px",
          `font:500 12px/24px ${FONT_SANS}`,
        ],
      });
      input.value = tab.name;
      input.setAttribute("aria-label", "Rename sheet");
      input.setAttribute("data-tab-rename", tabId);
      let settled = false;
      const commit = () => {
        if (settled || !isCurrent(input)) return false;
        settled = true;
        if (handleRenameTab(tabId, input.value) === false) {
          settled = false;
          input.focus();
          return false;
        }
        finish();
        return true;
      };
      input.addEventListener("keydown", (event) => {
        if (event.key === "Enter") {
          event.preventDefault();
          event.stopPropagation();
          commit();
        } else if (event.key === "Escape") {
          event.preventDefault();
          event.stopPropagation();
          settled = true;
          finish();
          handleRenameTab(tabId, null);
        }
      });
      input.addEventListener("blur", commit);
      return { input, commit };
    });
  }

  function handleRenameTab(tabId, nextName) {
    if (nextName === null) {
      chrome.hideNotice();
      editor.focus();
      return true;
    }
    const tab = tabs.list().find((t) => t.id === tabId);
    if (!tab) return true;
    if (nextName === tab.name) {
      chrome.hideNotice();
      editor.focus();
      return true;
    }
    const grid = gridOf(tab.table);
    if (!grid) return true;
    const ok = runTabStep(() => grid.dispatch({ v: 1, t: "renameSheet", sheet: tabId, name: nextName }));
    if (!ok) return false;
    syncUI();
    editor.focus();
    return true;
  }

  function runToolbarCommand(id) {
    const sel = getSel();
    switch (id) {
      case "number-format":
        openNumberFormatMenu();
        break;
      case "currency":
        runFormat((g, v) => applyNumberFormat(g, v, sel, "$#,##0.00"));
        break;
      case "percent":
        runFormat((g, v) => applyNumberFormat(g, v, sel, "0.00%"));
        break;
      case "decimals-less":
        runFormat((g, v) => applyDecimals(g, v, sel, history, -1, win.Sap?.grid?.adjustDecimals));
        break;
      case "decimals-more":
        runFormat((g, v) => applyDecimals(g, v, sel, history, 1, win.Sap?.grid?.adjustDecimals));
        break;
      case "bold":
      case "italic":
      case "strikethrough":
      case "wrap":
        runFormat((g, v) => applyStyleToggle(g, v, sel, history, id));
        break;
      case "text-color":
        openColorMenu("text");
        break;
      case "fill-color":
        openColorMenu("fill");
        break;
      case "borders":
        openBordersMenu();
        break;
      case "merge":
        triggerMerge();
        break;
      case "align":
        openAlignMenu();
        break;
      case "add-tab":
        handleAddTab();
        break;
      case "insert-row": {
        const view = viewOf(sel.tab);
        const span = axisSpan(view, sel);
        if (span?.axis === "rows") insertRowsAt(span.first, span.last - span.first + 1, span.first + 1, true);
        else {
          const box = selectionBox(view, sel);
          if (box) insertRowsAt(box.top, 1, box.top + 1, false);
        }
        break;
      }
    }
  }

  return {
    openRowHeaderMenu,
    openColHeaderMenu,
    openCellContextMenu,
    openChartContextMenu,
    handleAddTab,
    openTabContextMenu,
    openNumberFormatMenu,
    openColorMenu,
    openBordersMenu,
    openAlignMenu,
    triggerMerge,
    handleResizeCol,
    handleResizeRow,
    handleFitCol,
    startTabRename,
    handleRenameTab,
    runToolbarCommand,
    handleInsertAxisShortcut,
    handleDeleteAxisShortcut,
    handleContextMenuShortcut,
  };
}
