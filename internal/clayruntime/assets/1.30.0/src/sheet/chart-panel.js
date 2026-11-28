import { a1, colName, tabTable } from "./commit.js";
import { isKey, keyBetween } from "./format-commands.js";
import { axisSpan, selectionArea, selectionBox } from "./selection.js";
import { set } from "../lib/hostile-css.js";
import {
  bevelBox,
  bevelButton,
  bevelInput,
  bevelSurface,
  bevelText,
  setShown,
} from "../ui/bevel-controls.js";
import { FONT_MONO, FONT_SANS, TOKENS } from "../ui/bevel.js";

export const CHART_TYPES = [
  { value: "column", label: "Column" },
  { value: "bar", label: "Bar" },
  { value: "line", label: "Line" },
  { value: "area", label: "Area" },
  { value: "pie", label: "Pie" },
  { value: "scatter", label: "Scatter" },
];

const STACKABLE = new Set(["column", "bar", "area"]);
const DATA_RANGE_NOTICE = "Enter a data range, like A1:C10.";
const MAX_BOX_COL = 16383;
const MAX_BOX_ROW = 9999998;
const CHART_COLS = 6;
const CHART_ROWS = 15;
const MAX_TITLE_LENGTH = 255;

const PANEL_CSS = [
  "position:fixed",
  "display:flex",
  "flex-direction:column",
  "gap:8px",
  "padding:12px",
  "overflow-y:auto",
  "overscroll-behavior:contain",
  "z-index:2147483000",
];

function mintRuleId(rules, newId) {
  const taken = new Set(rules.map((r) => r.id));
  for (let tries = 0; tries < 100; tries++) {
    const id = typeof newId === "function" ? newId("k") : `k${Date.now().toString(36)}${tries}`;
    if (id && !taken.has(id)) return id;
  }
  return `k${Date.now().toString(36)}`;
}

function nextRulePos(rules) {
  let lastPos = null;
  for (const r of rules) {
    if (isKey(r.pos) && (lastPos === null || lastPos < r.pos)) lastPos = r.pos;
  }
  return keyBetween(lastPos, null);
}

function rectOf(ctx, table, ruleId, anchor) {
  if (anchor && typeof anchor.getBoundingClientRect === "function") {
    const r = anchor.getBoundingClientRect();
    if (r && (r.width > 0 || r.height > 0 || r.left !== 0 || r.top !== 0)) return r;
  }
  if (anchor && typeof anchor.left === "number" && typeof anchor.top === "number") {
    const width = anchor.width ?? 0;
    const height = anchor.height ?? 0;
    return { left: anchor.left, top: anchor.top, right: anchor.left + width, bottom: anchor.top + height, width, height };
  }
  const cRect = ctx.chartRect?.(table, ruleId);
  if (cRect && (cRect.width > 0 || cRect.height > 0)) {
    return {
      left: cRect.left,
      top: cRect.top,
      right: cRect.left + cRect.width,
      bottom: cRect.top + cRect.height,
      width: cRect.width,
      height: cRect.height,
    };
  }
  const btn = ctx.chrome.getButton?.("chart");
  if (btn && typeof btn.getBoundingClientRect === "function") {
    const r = btn.getBoundingClientRect();
    if (r && (r.width > 0 || r.height > 0 || r.left !== 0 || r.top !== 0)) return r;
  }
  return { left: 16, top: 72, right: 136, bottom: 98, width: 120, height: 26 };
}

function placeSurface(win, panel, rect, targetWidth = 300, maxCap = 460) {
  const vw = win?.innerWidth || 1024;
  const vh = win?.innerHeight || 768;
  const width = Math.min(targetWidth, Math.max(180, vw - 16));
  const roomRight = vw - rect.right - 8;
  const roomLeft = rect.left - 8;
  if (rect.height > 120 && (roomRight >= width + 4 || roomLeft >= width + 4)) {
    const left = roomRight >= width + 4 ? rect.right + 6 : Math.max(8, rect.left - width - 6);
    const top = Math.max(8, Math.min(rect.top, vh - Math.min(panel.scrollHeight || 320, maxCap) - 8));
    set(panel, "left", `${left}px`);
    set(panel, "top", `${top}px`);
    set(panel, "width", `${width}px`);
    set(panel, "max-height", `${Math.min(maxCap, Math.max(120, vh - top - 8))}px`);
    return;
  }
  const roomBelow = vh - rect.bottom - 8;
  const roomAbove = rect.top - 8;
  const panelHeight = panel.scrollHeight || 0;
  const below = roomBelow >= 160 || roomBelow >= roomAbove;
  const maxHeight = Math.min(maxCap, Math.max(120, below ? roomBelow : roomAbove));
  const top = below ? rect.bottom + 4 : Math.max(8, rect.top - 4 - Math.min(panelHeight || maxHeight, maxHeight));
  const left = Math.max(8, Math.min(rect.left, vw - width - 8));
  set(panel, "left", `${left}px`);
  set(panel, "top", `${top}px`);
  set(panel, "width", `${width}px`);
  set(panel, "max-height", `${maxHeight}px`);
}

function focusablesIn(panel) {
  return [...panel.querySelectorAll("button, input, select, textarea")].filter(
    (el) => !el.disabled && !el.hidden && !el.closest("[hidden]") && el.getAttribute("aria-disabled") !== "true",
  );
}

function trapTab(panel, doc, event) {
  const nodes = focusablesIn(panel);
  if (nodes.length === 0) return;
  event.preventDefault();
  const active = panel.getRootNode()?.activeElement ?? doc.activeElement;
  const idx = nodes.indexOf(active);
  const next = idx === -1 ? 0 : (idx + (event.shiftKey ? -1 : 1) + nodes.length) % nodes.length;
  nodes[next].focus();
}

function makeSwitchRow(doc, theme, labelText, attr, initial) {
  const row = bevelBox(
    "label",
    [
      "display:inline-flex",
      "align-items:center",
      "gap:7px",
      "cursor:pointer",
      "user-select:none",
      `font:500 12.5px/1.35 ${FONT_SANS}`,
      `color:${TOKENS.ink}`,
    ],
    { theme },
  );
  const input = doc.createElement("input");
  input.type = "checkbox";
  input.setAttribute("role", "switch");
  input.setAttribute("aria-label", labelText);
  input.setAttribute(attr, "");
  input.checked = Boolean(initial);
  input.setAttribute("aria-checked", input.checked ? "true" : "false");
  set(input, "margin", "0");
  set(input, "width", "14px");
  set(input, "height", "14px");
  set(input, "accent-color", TOKENS.brass);
  set(input, "cursor", "pointer");
  input.addEventListener("change", () => {
    input.setAttribute("aria-checked", input.checked ? "true" : "false");
  });
  const span = bevelText("span", ["font:inherit", "color:inherit"], labelText, { theme, role: null });
  row.append(input, span);
  return { row, input };
}

export function insertChart(ctx, anchor = null) {
  const sel = ctx.getSel();
  if (!sel || ctx.editor.editing) return null;
  const tabId = sel.tab;
  const table = tabTable(ctx.workbook, tabId);
  const grid = ctx.gridOf(table);
  const view = ctx.viewOf(tabId);
  const box = selectionBox(view, sel);
  if (!table || !grid || !box) return null;

  const span = axisSpan(view, sel);
  const source =
    span?.axis === "cols"
      ? `${colName(span.first)}:${colName(span.last)}`
      : span?.axis === "rows"
        ? `${span.first + 1}:${span.last + 1}`
        : selectionArea(view, sel);
  // The box goes beside the data, not beside the selection's last column on an empty sheet.
  let dataRight = box.right;
  for (let c = box.right; c >= box.left; c--) {
    let filled = false;
    for (let r = box.top; r <= box.bottom && !filled; r++) {
      const cell = view.cell(r, c);
      filled = Boolean(cell && cell.content !== null && cell.content !== "");
    }
    if (filled) {
      dataRight = c;
      break;
    }
  }
  const c0 = Math.min(Math.min(box.right, dataRight) + 2, MAX_BOX_COL - (CHART_COLS - 1));
  const r0 = Math.min(box.top, MAX_BOX_ROW - (CHART_ROWS - 1));
  const range = `${a1(r0, c0)}:${a1(r0 + CHART_ROWS - 1, c0 + CHART_COLS - 1)}`;
  const rules = grid.rules();
  const rule = {
    id: mintRuleId(rules, ctx.win.Sap?.grid?.newId),
    pos: nextRulePos(rules),
    kind: "chart",
    range,
    source,
    type: "column",
  };

  if (!ctx.writeGrid(() => grid.setRules({ set: [rule] }))) return null;
  ctx.onChartInserted?.(tabId, table, grid, rule);
  return openChartPanel(ctx, { tab: tabId, id: rule.id }, anchor);
}

export function openChartPanel(ctx, { tab, id }, anchor = null) {
  if (!tab || !id || ctx.editor.editing) return null;
  const table = tabTable(ctx.workbook, tab);
  const grid = ctx.gridOf(table);
  const existing = grid?.rules?.().find((r) => r.kind === "chart" && r.id === id) ?? null;
  if (!table || !grid || !existing) return null;

  const doc = ctx.doc;
  const win = ctx.win;
  const theme = ctx.chrome.theme;

  const panel = bevelSurface("div", [...PANEL_CSS, "width:min(300px, 100vw - 16px)", `color-scheme:${theme.scheme}`], { theme });
  panel.setAttribute("role", "dialog");
  panel.setAttribute("aria-label", "Chart");
  panel.setAttribute("data-chart-panel", "");

  const heading = bevelText("div", [`font:600 13px/1.35 ${FONT_SANS}`], "Chart", { theme });

  const typeField = bevelBox("label", ["display:flex", "flex-direction:column", "gap:4px"], { theme });
  typeField.append(
    bevelText("span", [`font:500 11.5px/1.3 ${FONT_SANS}`, `color:${TOKENS.muted}`], "Chart type", { theme, role: "mutedText" }),
  );
  const typeSelect = bevelInput("select", { theme, rules: ["appearance:auto", "cursor:pointer"] });
  typeSelect.setAttribute("aria-label", "Chart type");
  typeSelect.setAttribute("data-chart-type", "");
  for (const entry of CHART_TYPES) {
    const opt = doc.createElement("option");
    opt.value = entry.value;
    opt.textContent = entry.label;
    typeSelect.append(opt);
  }
  typeSelect.value = CHART_TYPES.some((t) => t.value === existing.type) ? existing.type : "column";
  typeField.append(typeSelect);

  const sourceField = bevelBox("label", ["display:flex", "flex-direction:column", "gap:4px"], { theme });
  sourceField.append(
    bevelText("span", [`font:500 11.5px/1.3 ${FONT_SANS}`, `color:${TOKENS.muted}`], "Data range", { theme, role: "mutedText" }),
  );
  const sourceInput = bevelInput("input", { theme, rules: [`font-family:${FONT_MONO}`] });
  sourceInput.setAttribute("aria-label", "Data range");
  sourceInput.setAttribute("data-chart-source", "");
  sourceInput.placeholder = "A1:C10";
  sourceInput.spellcheck = false;
  sourceInput.value = existing.source ?? "";
  sourceField.append(sourceInput);

  const titleField = bevelBox("label", ["display:flex", "flex-direction:column", "gap:4px"], { theme });
  titleField.append(
    bevelText("span", [`font:500 11.5px/1.3 ${FONT_SANS}`, `color:${TOKENS.muted}`], "Title", { theme, role: "mutedText" }),
  );
  const titleInput = bevelInput("input", { theme });
  titleInput.setAttribute("aria-label", "Title");
  titleInput.setAttribute("data-chart-title", "");
  titleInput.maxLength = MAX_TITLE_LENGTH;
  titleInput.value = existing.title ?? "";
  titleField.append(titleInput);

  const stacked = makeSwitchRow(doc, theme, "Stacked", "data-chart-stacked", Boolean(existing.stacked));
  const seriesRows = makeSwitchRow(doc, theme, "Series in rows", "data-chart-series-rows", existing.series === "rows");

  const syncType = () => {
    setShown(stacked.row, STACKABLE.has(typeSelect.value), "inline-flex");
  };
  typeSelect.addEventListener("change", syncType);
  syncType();

  const footer = bevelBox("div", ["display:flex", "align-items:center", "justify-content:flex-end", "gap:6px", "flex-wrap:wrap"], {
    theme,
  });
  const removeBtn = bevelButton("Remove chart", {
    small: true,
    variant: "danger",
    theme,
    extra: ["margin-inline-end:auto"],
    onClick: () => removeChart(),
  });
  removeBtn.setAttribute("data-chart-remove", "");
  const cancelBtn = bevelButton("Cancel", { small: true, theme, onClick: () => close(true) });
  const doneBtn = bevelButton("Done", { small: true, variant: "primary", theme, onClick: () => apply() });
  cancelBtn.setAttribute("data-chart-cancel", "");
  doneBtn.setAttribute("data-chart-done", "");
  footer.append(removeBtn, cancelBtn, doneBtn);

  panel.append(heading, typeField, sourceField, titleField, stacked.row, seriesRows.row, footer);
  ctx.chrome.topRoot.append(panel);

  let closed = false;

  function close(restoreFocus = true) {
    if (closed) return;
    closed = true;
    doc.removeEventListener("mousedown", onOutside, true);
    doc.removeEventListener("pointerdown", onOutside, true);
    doc.removeEventListener("keydown", onKey, true);
    panel.remove();
    if (restoreFocus) ctx.editor?.textarea?.focus?.();
  }

  function currentGrid() {
    return ctx.gridOf(tabTable(ctx.workbook, tab));
  }

  function removeChart() {
    const g = currentGrid();
    if (!g) return close(true);
    if (!ctx.writeGrid(() => g.setRules({ remove: [id] }))) return;
    ctx.clearChartSelection?.(id);
    ctx.refreshStatus();
    ctx.syncUI({ announce: true });
    close(true);
  }

  function apply() {
    const g = currentGrid();
    if (!g) return close(true);
    const source = sourceInput.value.trim();
    const areaOf = ctx.win.Sap?.grid?.areaOf;
    if (!source || (typeof areaOf === "function" && areaOf(source) === null)) {
      ctx.chrome.showNotice(DATA_RANGE_NOTICE);
      sourceInput.focus();
      return;
    }
    const latest = g.rules().find((r) => r.kind === "chart" && r.id === id) ?? existing;
    const type = CHART_TYPES.some((t) => t.value === typeSelect.value) ? typeSelect.value : "column";
    const cleanTitle = titleInput.value
      .replace(/[\r\0]/g, "")
      .trim()
      .slice(0, MAX_TITLE_LENGTH)
      .replace(/[\uD800-\uDBFF](?![\uDC00-\uDFFF])|(?<![\uD800-\uDBFF])[\uDC00-\uDFFF]/g, "");
    const nextRule = {
      id: latest.id,
      pos: latest.pos,
      kind: "chart",
      range: latest.range,
      source,
      type,
    };
    if (cleanTitle) nextRule.title = cleanTitle;
    if (STACKABLE.has(type) && stacked.input.checked) nextRule.stacked = true;
    if (seriesRows.input.checked) nextRule.series = "rows";

    if (!ctx.writeGrid(() => g.setRules({ set: [nextRule] }))) return;
    ctx.refreshStatus();
    ctx.syncUI({ announce: true });
    close(true);
  }

  function onOutside(event) {
    const path = event.composedPath?.() ?? [event.target];
    if (path.includes(panel)) return;
    close(false);
  }

  function onKey(event) {
    if (event.key === "Escape") {
      event.preventDefault();
      event.stopPropagation();
      close(true);
      return;
    }
    if (event.key === "Tab") {
      event.stopPropagation();
      trapTab(panel, doc, event);
      return;
    }
    const target = event.composedPath?.()[0] ?? event.target;
    if (event.key === "Enter" && (target === sourceInput || target === titleInput)) {
      event.preventDefault();
      event.stopPropagation();
      apply();
    }
  }

  placeSurface(win, panel, rectOf(ctx, table, id, anchor));
  doc.addEventListener("mousedown", onOutside, true);
  doc.addEventListener("pointerdown", onOutside, true);
  doc.addEventListener("keydown", onKey, true);

  typeSelect.focus();

  return {
    tab,
    close: () => close(false),
  };
}
