import { colName, tabTable } from "./commit.js";
import { selectionBox, selectCell } from "./selection.js";
import { set } from "../lib/hostile-css.js";
import { bevelBox, bevelButton, bevelInput, bevelSurface, bevelText, setShown } from "../ui/bevel-controls.js";
import { FONT_SANS, TOKENS } from "../ui/bevel.js";
import { buildRangeSortAction } from "./sort.js";

export const CONDITIONS = [
  ["none", "None"],
  ["empty", "Is empty"],
  ["notEmpty", "Is not empty"],
  ["contains", "Text contains"],
  ["notContains", "Text does not contain"],
  ["startsWith", "Text starts with"],
  ["endsWith", "Text ends with"],
  ["exactly", "Text is exactly"],
  ["eq", "Is equal to"],
  ["ne", "Is not equal to"],
  ["gt", "Greater than"],
  ["gte", "Greater than or equal to"],
  ["lt", "Less than"],
  ["lte", "Less than or equal to"],
];

const TEXT_OPS = new Set(["contains", "notContains", "startsWith", "endsWith", "exactly"]);
const NUMBER_OPS = new Set(["eq", "ne", "gt", "gte", "lt", "lte"]);
const NO_ARG_OPS = new Set(["none", "empty", "notEmpty"]);

export const cellText = (cell) => cell?.text ?? "";

const filled = (view, r, c) => cellText(view.cell(r, c)).trim() !== "";

function spanHasText(view, top, left, bottom, right) {
  for (let r = Math.max(0, top); r <= Math.min(view.rows.length - 1, bottom); r++) {
    for (let c = Math.max(0, left); c <= Math.min(view.cols.length - 1, right); c++) {
      if (filled(view, r, c)) return true;
    }
  }
  return false;
}

export function currentRegion(view, box) {
  let { top, left, bottom, right } = box;
  let grew = true;
  while (grew) {
    grew = false;
    if (top > 0 && spanHasText(view, top - 1, left - 1, top - 1, right + 1)) {
      top -= 1;
      grew = true;
    }
    if (bottom < view.rows.length - 1 && spanHasText(view, bottom + 1, left - 1, bottom + 1, right + 1)) {
      bottom += 1;
      grew = true;
    }
    if (left > 0 && spanHasText(view, top - 1, left - 1, bottom + 1, left - 1)) {
      left -= 1;
      grew = true;
    }
    if (right < view.cols.length - 1 && spanHasText(view, top - 1, right + 1, bottom + 1, right + 1)) {
      right += 1;
      grew = true;
    }
  }
  return { top, left, bottom, right };
}

export function rangeRows(view, spec) {
  const heading = view.rows.indexOf(spec.header);
  if (heading === -1) return [];
  const colIndexes = spec.cols.map((id) => view.cols.indexOf(id)).filter((c) => c !== -1);
  if (colIndexes.length === 0) return [];
  let stop = view.rows.length - 1;
  if (spec.bottom !== undefined) {
    const bottom = view.rows.indexOf(spec.bottom);
    if (bottom !== -1) stop = Math.min(stop, bottom);
  }
  const first = Math.max(heading + 1, view.frozenRows ?? 0);
  let last = heading;
  for (let r = first; r <= stop; r++) {
    if (colIndexes.some((c) => filled(view, r, c))) last = r;
  }
  const rows = [];
  for (let r = first; r <= last; r++) rows.push(r);
  return rows;
}

const isFiniteNumber = (value) => typeof value === "number" && Number.isFinite(value);

export function passes(cell, criterion) {
  const text = cellText(cell);
  if (criterion.values instanceof Set && !criterion.values.has(text)) return false;
  const condition = criterion.condition;
  if (!condition || condition.op === "none") return true;
  const op = condition.op;
  const arg = condition.arg ?? "";
  if (op === "empty") return text === "";
  if (op === "notEmpty") return text !== "";
  if (TEXT_OPS.has(op)) {
    const haystack = text.toLowerCase();
    const needle = String(arg).toLowerCase();
    if (op === "contains") return haystack.includes(needle);
    if (op === "notContains") return !haystack.includes(needle);
    if (op === "startsWith") return haystack.startsWith(needle);
    if (op === "endsWith") return haystack.endsWith(needle);
    return haystack === needle;
  }
  if (NUMBER_OPS.has(op)) {
    if (text === "") return op === "ne";
    const right = Number(arg);
    if (!Number.isFinite(right) || String(arg).trim() === "") {
      if (op === "eq") return text.toLowerCase() === String(arg).toLowerCase();
      if (op === "ne") return text.toLowerCase() !== String(arg).toLowerCase();
      return false;
    }
    const left = Number(cell?.value);
    if (!Number.isFinite(left)) return op === "ne";
    if (op === "eq") return left === right;
    if (op === "ne") return left !== right;
    if (op === "gt") return left > right;
    if (op === "gte") return left >= right;
    if (op === "lt") return left < right;
    return left <= right;
  }
  return true;
}

export function hiddenRowsFor(view, spec) {
  const hidden = new Set();
  const criteria = [...spec.criteria.entries()]
    .map(([id, criterion]) => [view.cols.indexOf(id), criterion])
    .filter(([c]) => c !== -1);
  for (const r of rangeRows(view, spec)) {
    if (criteria.some(([c, criterion]) => !passes(view.cell(r, c), criterion))) hidden.add(view.rows[r]);
  }
  return hidden;
}

export function distinctValues(view, spec, colId) {
  const c = view.cols.indexOf(colId);
  if (c === -1) return [];
  const counts = new Map();
  const firstValue = new Map();
  for (const r of rangeRows(view, spec)) {
    const cell = view.cell(r, c);
    const text = cellText(cell);
    counts.set(text, (counts.get(text) ?? 0) + 1);
    if (!firstValue.has(text)) firstValue.set(text, cell?.value ?? null);
  }
  const numeric = (text, value) => text !== "" && isFiniteNumber(value);
  const entries = [...counts.entries()].map(([text, count]) => ({ text, count, value: firstValue.get(text) }));
  entries.sort((a, b) => {
    if (a.text === "" && b.text !== "") return 1;
    if (b.text === "" && a.text !== "") return -1;
    const an = numeric(a.text, a.value);
    const bn = numeric(b.text, b.value);
    if (an && bn) return Number(a.value) - Number(b.value);
    if (an) return -1;
    if (bn) return 1;
    return a.text.localeCompare(b.text);
  });
  return entries.map(({ text, count }) => ({ text, count }));
}

export function toggleFilter(ctx) {
  const sel = ctx.getSel();
  if (!sel) return;
  const tabId = sel.tab;
  if (ctx.getFilter(tabId)) {
    ctx.setFilter(tabId, null);
    return;
  }
  const view = ctx.viewOf(tabId);
  let box = selectionBox(view, sel);
  if (!box) return;
  const own =
    box.top === box.active.r &&
    box.left === box.active.c &&
    box.bottom === box.active.r + (box.active.rows ?? 1) - 1 &&
    box.right === box.active.c + (box.active.cols ?? 1) - 1;
  if (own) box = currentRegion(view, box);
  if (!spanHasText(view, box.top, box.left, box.bottom, box.right)) {
    ctx.chrome.showNotice("Select the table to filter, with its headings in the top row.");
    return;
  }
  const next = {
    header: view.rows[box.top],
    cols: view.cols.slice(box.left, box.right + 1),
    criteria: new Map(),
  };
  if (sel.whole !== "cols" && box.bottom > box.top) next.bottom = view.rows[box.bottom];
  ctx.setFilter(tabId, next);
}

const SLOT_CSS = [
  "position:fixed",
  "width:min(260px, 100vw - 16px)",
  "display:flex",
  "flex-direction:column",
  "gap:8px",
  "padding:10px",
  "overflow-y:auto",
  "overscroll-behavior:contain",
  "z-index:2147483000",
];

const valueLabel = (text) => (text === "" ? "(Blanks)" : text);

export function openFilterPanel(ctx, tabId, colId, anchor) {
  const doc = ctx.doc;
  const win = ctx.win;
  const spec = ctx.getFilter(tabId);
  if (!spec) return;
  const view = ctx.viewOf(tabId);
  const colIndex = view.cols.indexOf(colId);
  if (colIndex === -1) return;
  const criterion = spec.criteria.get(colId) ?? null;
  const values = distinctValues(view, spec, colId);
  const allTexts = values.map((value) => value.text);
  const checked = criterion?.values ? new Set(criterion.values) : new Set(allTexts);
  const theme = ctx.chrome.theme;
  const panel = bevelSurface("div", SLOT_CSS, { theme });
  panel.setAttribute("role", "dialog");
  panel.setAttribute("aria-label", `Filter column ${colName(colIndex)}`);
  panel.setAttribute("data-filter-panel", "");

  const note = bevelText("div", ["display:block", `font:11px/1.4 ${FONT_SANS}`, `color:${TOKENS.muted}`], "Filters only your view — other editors keep theirs", { theme, role: "mutedText" });

  const sortAsc = bevelButton("Sort A to Z", { small: true, theme, onClick: () => sortBy("asc") });
  const sortDesc = bevelButton("Sort Z to A", { small: true, theme, onClick: () => sortBy("desc") });
  sortAsc.setAttribute("data-filter-sort-asc", "");
  sortDesc.setAttribute("data-filter-sort-desc", "");
  const sortRow = bevelBox("div", ["display:flex", "gap:6px", "flex-wrap:wrap"], { theme });
  sortRow.append(sortAsc, sortDesc);
  const sep = bevelBox("div", ["display:block", "height:1px", `background:${TOKENS["line-2"]}`], { theme });

  const conditionLabel = bevelText("div", ["display:block", `font:600 12px/1.4 ${FONT_SANS}`], "Filter by condition", { theme });
  const condition = bevelInput("select", { theme, rules: ["appearance:auto", "cursor:pointer"] });
  condition.setAttribute("data-filter-condition", "");
  for (const [op, label] of CONDITIONS) {
    const option = doc.createElement("option");
    option.value = op;
    option.textContent = label;
    condition.append(option);
  }
  condition.value = criterion?.condition?.op ?? "none";
  const arg = bevelInput("input", { theme });
  arg.setAttribute("data-filter-arg", "");
  arg.value = criterion?.condition?.arg ?? "";
  const argWrap = bevelBox("div", ["display:block"], { theme });
  argWrap.append(arg);
  const showArg = () => setShown(argWrap, !NO_ARG_OPS.has(condition.value), "block");
  condition.addEventListener("change", showArg);
  showArg();

  const search = bevelInput("input", { theme });
  search.setAttribute("data-filter-search", "");
  search.placeholder = "Search values";

  const list = bevelBox("div", ["display:block", "max-height:180px", "overflow-y:auto"], { theme, role: "well" });
  list.setAttribute("data-filter-values", "");

  const allBtn = bevelButton("Select all", { small: true, variant: "quiet", theme, onClick: () => setShownValues(true) });
  const clearBtn = bevelButton("Clear", { small: true, variant: "quiet", theme, onClick: () => setShownValues(false) });
  allBtn.setAttribute("data-filter-all", "");
  clearBtn.setAttribute("data-filter-clear", "");
  const pickRow = bevelBox("div", ["display:flex", "gap:6px"], { theme });
  pickRow.append(allBtn, clearBtn);

  const cancelBtn = bevelButton("Cancel", { theme, onClick: () => close() });
  const applyBtn = bevelButton("Apply", { variant: "primary", theme, onClick: () => apply() });
  cancelBtn.setAttribute("data-filter-cancel", "");
  applyBtn.setAttribute("data-filter-apply", "");
  const footer = bevelBox("div", ["display:flex", "justify-content:flex-end", "gap:6px"], { theme });
  footer.append(cancelBtn, applyBtn);

  panel.append(note, sortRow, sep, conditionLabel, condition, argWrap, search, list, pickRow, footer);
  ctx.chrome.topRoot.append(panel);

  const matching = () => {
    const query = search.value.trim().toLowerCase();
    return query === "" ? values : values.filter((value) => valueLabel(value.text).toLowerCase().includes(query));
  };

  function renderValues() {
    list.textContent = "";
    const rows = matching();
    const shown = rows.slice(0, 500);
    for (const value of shown) {
      const row = bevelBox("label", ["display:flex", "align-items:center", "gap:6px", "padding:2px 4px", "cursor:pointer"], { theme });
      row.setAttribute("data-filter-value", value.text);
      const box = doc.createElement("input");
      box.type = "checkbox";
      box.checked = checked.has(value.text);
      box.addEventListener("change", () => {
        if (box.checked) checked.add(value.text);
        else checked.delete(value.text);
      });
      const label = bevelText(
        "span",
        [`font:400 12.5px/1.35 ${FONT_SANS}`, `color:${TOKENS.ink}`],
        `${valueLabel(value.text)} (${value.count})`,
        { theme, role: null },
      );
      row.append(box, label);
      list.append(row);
    }
    if (rows.length > 500) {
      const more = bevelText("div", ["display:block", "padding:4px", `color:${TOKENS.muted}`], "Search to see more values", { theme, role: "mutedText" });
      list.append(more);
    }
  }

  function setShownValues(next) {
    for (const value of matching()) {
      if (next) checked.add(value.text);
      else checked.delete(value.text);
    }
    renderValues();
  }

  function close() {
    doc.removeEventListener("mousedown", onOutside, true);
    doc.removeEventListener("pointerdown", onOutside, true);
    doc.removeEventListener("keydown", onKey, true);
    panel.remove();
    ctx.editor?.textarea?.focus?.();
  }

  function apply() {
    const current = ctx.viewOf(tabId);
    if (current.rows.indexOf(spec.header) === -1 || current.cols.indexOf(colId) === -1) {
      close();
      return;
    }
    const conditionValue = condition.value === "none" ? null : { op: condition.value, arg: arg.value };
    const keptValues = allTexts.filter((text) => checked.has(text));
    const nextValues = keptValues.length === allTexts.length ? null : new Set(keptValues);
    if (nextValues === null && conditionValue === null) spec.criteria.delete(colId);
    else spec.criteria.set(colId, { values: nextValues, condition: conditionValue });
    const hidden = hiddenRowsFor(current, spec);
    spec.hidden = hidden;
    ctx.setFilter(tabId, spec);
    ctx.applyHiddenRows(tabId, hidden);
    const sel = ctx.getSel();
    if (sel && sel.tab === tabId && hidden.has(sel.anchor?.row)) {
      const heading = current.rows.indexOf(spec.header);
      const c = current.cols.indexOf(colId);
      if (heading !== -1 && c !== -1) ctx.applySelection(selectCell(current, sel, heading, c, { extend: false }));
    }
    close();
  }

  function sortBy(direction) {
    const current = ctx.viewOf(tabId);
    const liveColIndex = current.cols.indexOf(colId);
    const table = tabTable(ctx.workbook, tabId);
    const grid = ctx.gridOf(table);
    const rows = rangeRows(current, spec);
    const heading = current.rows.indexOf(spec.header);
    if (!grid || heading === -1 || liveColIndex === -1 || rows.length < 2) {
      close();
      return;
    }
    const box = { top: heading + 1, left: 0, bottom: rows[rows.length - 1], right: current.cols.length - 1 };
    const action = buildRangeSortAction(grid, current, { sheet: tabId, box, col: liveColIndex, direction });
    if (action) {
      ctx.writeGrid(() => grid.dispatch(action));
      ctx.refreshStatus();
      ctx.syncUI({ announce: true });
    }
    close();
  }

  function onOutside(event) {
    const path = event.composedPath?.() ?? [];
    if (path.includes(panel)) return;
    close();
  }

  function onKey(event) {
    if (event.key === "Escape") {
      event.preventDefault();
      close();
      return;
    }
    if (event.key === "Tab") trapFocus(event);
  }

  function focusables() {
    return [...panel.querySelectorAll("button, input, select")].filter(
      (el) => !el.disabled && !el.closest("[hidden]") && el.getAttribute("aria-disabled") !== "true",
    );
  }

  function trapFocus(event) {
    const nodes = focusables();
    if (nodes.length === 0) return;
    const active = panel.getRootNode()?.activeElement ?? doc.activeElement;
    const idx = nodes.indexOf(active);
    if (idx === -1) return;
    if (event.shiftKey && idx === 0) {
      event.preventDefault();
      nodes[nodes.length - 1].focus();
    } else if (!event.shiftKey && idx === nodes.length - 1) {
      event.preventDefault();
      nodes[0].focus();
    }
  }

  doc.addEventListener("mousedown", onOutside, true);
  doc.addEventListener("pointerdown", onOutside, true);
  doc.addEventListener("keydown", onKey, true);
  search.addEventListener("input", renderValues);
  renderValues();

  const rect = anchor.getBoundingClientRect();
  const width = Math.min(260, (win.innerWidth || 0) - 16);
  const roomBelow = (win.innerHeight || 0) - rect.bottom - 8;
  const roomAbove = rect.top - 8;
  const panelHeight = panel.scrollHeight || 0;
  const below = roomBelow >= roomAbove;
  const top = below ? rect.bottom + 4 : Math.max(8, rect.top - 4 - Math.min(panelHeight, roomAbove));
  const maxHeight = Math.max(160, below ? roomBelow : roomAbove);
  set(panel, "left", `${Math.max(8, Math.min(rect.left, (win.innerWidth || 0) - width - 8))}px`);
  set(panel, "top", `${top}px`);
  set(panel, "width", `${width}px`);
  set(panel, "max-height", `${maxHeight}px`);

  const first = panel.querySelector("button:not([disabled]), input:not([disabled]), select:not([disabled])");
  first?.focus?.();

  return close;
}
