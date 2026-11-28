import { ROW_ESTIMATE, tableMetrics } from "./geometry.js";
import { colName } from "./commit.js";
import { FONT_MONO, FONT_SANS, TOKENS } from "../ui/bevel.js";
import { resolveTheme } from "../ui/theme.js";
import { mountEditorUI } from "../ui/editor-ui.js";

const GUTTER_W = 42;
const HEADER_H = 24;
const GRIP_PX = 6;
const MIN_COL_W = 8;
const MIN_ROW_H = 12;

const REF_COLORS = [
  { border: TOKENS.brass, fill: TOKENS["brass-soft"] },
  { border: TOKENS.teal, fill: TOKENS["teal-soft"] },
  { border: "light-dark(#6E4076, #C388CE)", fill: "light-dark(#F1E8F3, #2B1E33)" },
  { border: "light-dark(#9A4E1C, #D8844F)", fill: "light-dark(#F6E6DA, #2E1F16)" },
];

const OVERLAY_CSS = `
.sheet-overlays {
  position: absolute;
  top: 0;
  left: 0;
  width: 0;
  height: 0;
  overflow: visible;
  z-index: 8;
  font-family: ${FONT_MONO};
  font-variant-numeric: tabular-nums;
  color-scheme: inherit;
  --sheet-sunk: var(--sunk, var(--clay-ui-background, var(--sheet-fb-sunk, ${TOKENS.sunk})));
  --sheet-surface: var(--surface, var(--clay-ui-surface, var(--sheet-fb-surface, ${TOKENS.surface})));
  --sheet-muted: var(--muted, var(--clay-ui-muted-text, var(--sheet-fb-muted, ${TOKENS.muted})));
  --sheet-line-2: var(--line-2, var(--clay-ui-border, var(--sheet-fb-line-2, ${TOKENS["line-2"]})));
  --sheet-sel: var(--sel, var(--clay-ui-accent, var(--sheet-fb-sel, light-dark(#1D5B79, #5CA8CC))));
  --sheet-sel-fill: var(--sel-fill, light-dark(#E5F0F6, #182B3A));
  --sheet-sel-head: var(--sel-head, var(--sel-fill, light-dark(#D8E8F1, #1E3548)));
  --sheet-sel-line: var(--sel-line, light-dark(#ADCBE0, #2D4F6C));
  --sheet-brass: var(--brass, ${TOKENS.brass});
}
.sheet-overlays, .sheet-overlays * {
  box-sizing: border-box;
}
.sheet-corner, .sheet-col-hdr, .sheet-row-hdr {
  position: absolute;
  display: flex;
  align-items: center;
  justify-content: center;
  background: var(--sheet-sunk);
  color: var(--sheet-muted);
  font-family: ${FONT_MONO};
  font-size: 11px;
  font-weight: 500;
  line-height: 1;
  user-select: none;
  cursor: pointer;
  border-right: 1px solid var(--sheet-line-2);
  border-bottom: 1px solid var(--sheet-line-2);
}
.sheet-corner[hidden], .sheet-col-hdr[hidden], .sheet-row-hdr[hidden] {
  display: none;
}
.sheet-col-hdr[data-active="true"], .sheet-row-hdr[data-active="true"] {
  background: var(--sheet-sel-head);
  color: var(--sheet-sel);
  font-weight: 600;
}
.sheet-row-hdr[data-active-next="true"] {
  border-bottom-color: var(--sheet-sel-line);
}
.sheet-col-grip {
  position: absolute;
  top: 0;
  right: 0;
  width: ${GRIP_PX}px;
  height: 100%;
  cursor: col-resize;
}
.sheet-row-grip {
  position: absolute;
  left: 0;
  bottom: 0;
  width: 100%;
  height: ${GRIP_PX}px;
  cursor: row-resize;
}
.sheet-resize-guide {
  position: absolute;
  pointer-events: none;
  background: var(--sheet-sel);
  z-index: 7;
}
.sheet-sel-fill {
  position: absolute;
  pointer-events: none;
  background: color-mix(in srgb, var(--sheet-sel-fill) 42%, transparent);
  border: 2px solid var(--sheet-sel);
  z-index: 2;
}
.sheet-active-ring {
  position: absolute;
  pointer-events: none;
  border: 2px solid var(--sheet-sel);
  z-index: 4;
}
.sheet-active-ring[data-editing="true"] {
  border-color: var(--sheet-brass);
  box-shadow: 0 2px 8px color-mix(in srgb, ${TOKENS.ink} 14%, transparent);
}
.sheet-ref-box {
  position: absolute;
  pointer-events: none;
  border: 2px dashed var(--ref-border);
  background: color-mix(in srgb, var(--ref-fill) 32%, transparent);
  z-index: 3;
}
.sheet-conflict-box {
  position: absolute;
  pointer-events: none;
  background: color-mix(in srgb, ${TOKENS["ox-soft"]} 65%, transparent);
  z-index: 2;
}
.sheet-fill-handle {
  position: absolute;
  width: 7px;
  height: 7px;
  background: var(--sheet-sel);
  border: 1px solid var(--sheet-surface);
  cursor: crosshair;
  pointer-events: auto;
  z-index: 5;
}
.sheet-touch-handle {
  position: absolute;
  pointer-events: auto;
  touch-action: none;
  z-index: 6;
}
.sheet-touch-handle::after {
  content: "";
  position: absolute;
  left: 16px;
  top: 16px;
  width: 12px;
  height: 12px;
  box-sizing: border-box;
  border-radius: 50%;
  background: var(--sheet-sel);
  border: 2px solid var(--sheet-surface);
}
.sheet-fill-preview {
  position: absolute;
  pointer-events: none;
  border: 1.5px dashed var(--sheet-sel);
  z-index: 5;
}
.sheet-chart-ring {
  position: absolute;
  pointer-events: none;
  border: 2px solid var(--sheet-sel);
  z-index: 8;
}
.sheet-chart-handle {
  position: absolute;
  width: 9px;
  height: 9px;
  background: var(--sheet-sel);
  border: 1.5px solid var(--sheet-surface);
  cursor: nwse-resize;
  pointer-events: auto;
  z-index: 9;
}
.sheet-link-chip {
  position: absolute;
  z-index: 7;
  display: inline-flex;
  align-items: center;
  gap: 8px;
  padding: 4px 8px;
  background: var(--sheet-surface);
  color: ${TOKENS.ink};
  border: 1px solid ${TOKENS["line-3"]};
  font: 500 12px/1.3 ${FONT_SANS};
  pointer-events: auto;
  white-space: nowrap;
  box-shadow: 3px 3px 0 color-mix(in srgb, ${TOKENS.ink} 10%, transparent);
}
.sheet-link-chip[hidden] {
  display: none;
}
.sheet-link-chip span {
  font: 400 11.5px/1.2 ${FONT_MONO};
  color: ${TOKENS["ink-2"]};
}
.sheet-link-chip a {
  font: 600 12px/1.2 ${FONT_SANS};
  color: ${TOKENS.teal};
  text-decoration: underline;
  cursor: pointer;
}
.sheet-editor-slot {
  position: absolute;
  z-index: 6;
  pointer-events: none;
}
.sheet-filter-btn {
  position: absolute;
  width: 18px;
  height: 18px;
  display: grid;
  place-items: center;
  padding: 0;
  border: 1px solid var(--sheet-line, #C5CCD8);
  background: var(--sheet-surface);
  color: var(--sheet-sel);
  pointer-events: auto;
  cursor: pointer;
  z-index: 7;
}
.sheet-filter-btn[data-active="true"] {
  background: var(--sheet-sel);
  color: var(--sheet-surface);
}
@media (pointer: coarse) {
  .sheet-filter-btn { width: 28px; height: 28px; }
}
.sheet-sr-only {
  position: absolute;
  width: 1px;
  height: 1px;
  padding: 0;
  margin: -1px;
  overflow: hidden;
  clip: rect(0, 0, 0, 0);
  white-space: nowrap;
  border: 0;
}
`;

function placeBox(el, origin, box) {
  el.style.left = `${box.left - origin.left}px`;
  el.style.top = `${box.top - origin.top}px`;
  el.style.width = `${box.width}px`;
  el.style.height = `${box.height}px`;
}

export function createOverlays(
  workbook,
  {
    onSelectCol,
    onSelectRow,
    onSelectAll,
    onResizeCol,
    onResizeRow,
    onFitCol,
    onColMenu,
    onRowMenu,
    onFillHandleDown,
    onFillHandleDblClick,
    onTouchHandleDown,
    onFilterButton,
    onChartResizeDown,
  } = {},
) {
  const doc = workbook.ownerDocument ?? document;
  const winObj = doc.defaultView ?? window;
  const theme = resolveTheme();
  const root = doc.createElement("div");
  root.className = "sheet-overlays";
  const pageScheme = winObj.getComputedStyle?.(doc.documentElement)?.colorScheme;
  if (!pageScheme || pageScheme === "normal") root.style.colorScheme = theme.scheme;
  if (theme.tokens?.accent) root.style.setProperty("--sheet-fb-sel", theme.tokens.accent);
  if (theme.tokens?.surface) root.style.setProperty("--sheet-fb-surface", theme.tokens.surface);
  if (theme.tokens?.background) root.style.setProperty("--sheet-fb-sunk", theme.tokens.background);
  if (theme.tokens?.border) root.style.setProperty("--sheet-fb-line-2", theme.tokens.border);
  if (theme.tokens?.mutedText) root.style.setProperty("--sheet-fb-muted", theme.tokens.mutedText);

  let lastState = { table: null, win: null, view: null };
  let resizing = null;

  const corner = doc.createElement("div");
  corner.className = "sheet-corner";
  corner.setAttribute("role", "presentation");
  corner.addEventListener("mousedown", (e) => {
    if (e.button !== 0) return;
    e.preventDefault();
    onSelectAll?.();
  });

  function gripOf(e, gripAttr, hdrAttr, horiz) {
    const grip = e.target?.closest?.(`[${gripAttr}]`);
    if (grip) return Number(grip.getAttribute(gripAttr));
    const hdr = e.target?.closest?.(`[${hdrAttr}]`);
    const pos = horiz ? e.clientX : e.clientY;
    if (!hdr || !lastState.table || typeof pos !== "number") return null;
    const idx = Number(hdr.getAttribute(hdrAttr));
    const b = api.cellBox(lastState.table, lastState.win, horiz ? 0 : idx, horiz ? idx : 0);
    const size = b ? (horiz ? b.width : b.height) : 0;
    const end = b ? (horiz ? b.left : b.top) + size : 0;
    return size > GRIP_PX && pos >= end - GRIP_PX && pos <= end + 2 ? idx : null;
  }

  function paintGuide() {
    if (!resizing || !lastState.table) return;
    const origin = root.getBoundingClientRect();
    const metrics = tableMetrics(lastState.table, lastState.win);
    const first = metrics.cellBox(0, 0);
    resizeGuide.hidden = false;
    placeBox(
      resizeGuide,
      origin,
      resizing.axis === "col"
        ? {
            left: resizing.baseStart + resizing.nextSize,
            top: first.top - HEADER_H,
            width: 2,
            height: HEADER_H + Math.max(metrics.totalHeight, HEADER_H),
          }
        : {
            left: first.left - GUTTER_W,
            top: resizing.baseStart + resizing.nextSize,
            width: GUTTER_W + Math.max(metrics.width, GUTTER_W),
            height: 2,
          },
    );
  }

  function startResize(axis, index, pos) {
    const b = lastState.table ? api.cellBox(lastState.table, lastState.win, axis === "col" ? 0 : index, axis === "col" ? index : 0) : null;
    const startSize = axis === "col" ? (b?.width || 100) : (b?.height || ROW_ESTIMATE);
    resizing = {
      axis,
      index,
      startPos: pos ?? 0,
      startSize,
      baseStart: (axis === "col" ? b?.left : b?.top) ?? 0,
      nextSize: startSize,
      moved: false,
    };
    paintGuide();
  }

  const colsLayer = doc.createElement("div");
  colsLayer.addEventListener("mousedown", (e) => {
    if (e.button !== undefined && e.button !== 0) return;
    const gripCol = gripOf(e, "data-col-resize", "data-col", true);
    if (gripCol !== null) {
      e.preventDefault();
      e.stopPropagation();
      startResize("col", gripCol, e.clientX);
      return;
    }
    const hdr = e.target?.closest?.("[data-col]");
    if (!hdr) return;
    e.preventDefault();
    onSelectCol?.(Number(hdr.getAttribute("data-col")), e.shiftKey);
  });
  colsLayer.addEventListener("dblclick", (e) => {
    const gripCol = gripOf(e, "data-col-resize", "data-col", true);
    if (gripCol === null) return;
    e.preventDefault();
    e.stopPropagation();
    onFitCol?.(gripCol);
  });
  colsLayer.addEventListener("contextmenu", (e) => {
    const hdr = e.target?.closest?.("[data-col]");
    if (!hdr) return;
    e.preventDefault();
    e.stopPropagation();
    onColMenu?.(Number(hdr.getAttribute("data-col")), hdr, e.clientX ?? 0, e.clientY ?? 0);
  });

  const rowsLayer = doc.createElement("div");
  rowsLayer.addEventListener("mousedown", (e) => {
    if (e.button !== undefined && e.button !== 0) return;
    const gripRow = gripOf(e, "data-row-resize", "data-row", false);
    if (gripRow !== null) {
      e.preventDefault();
      e.stopPropagation();
      startResize("row", gripRow, e.clientY);
      return;
    }
    const hdr = e.target?.closest?.("[data-row]");
    if (!hdr) return;
    e.preventDefault();
    onSelectRow?.(Number(hdr.getAttribute("data-row")), e.shiftKey);
  });
  rowsLayer.addEventListener("contextmenu", (e) => {
    const hdr = e.target?.closest?.("[data-row]");
    if (!hdr) return;
    e.preventDefault();
    e.stopPropagation();
    onRowMenu?.(Number(hdr.getAttribute("data-row")), hdr, e.clientX ?? 0, e.clientY ?? 0);
  });

  function onWinMouseMove(e) {
    if (!resizing) return;
    resizing.moved = true;
    const floor = resizing.axis === "col" ? MIN_COL_W : MIN_ROW_H;
    const pos = resizing.axis === "col" ? (e.clientX ?? 0) : (e.clientY ?? 0);
    resizing.nextSize = Math.max(floor, Math.round(resizing.startSize + (pos - resizing.startPos)));
    paintGuide();
  }

  function onWinMouseUp() {
    if (!resizing) return;
    const done = resizing;
    resizing = null;
    resizeGuide.hidden = true;
    if (!done.moved) return;
    if (done.axis === "col") onResizeCol?.(done.index, done.nextSize);
    else onResizeRow?.(done.index, done.nextSize);
  }

  winObj.addEventListener("mousemove", onWinMouseMove);
  winObj.addEventListener("mouseup", onWinMouseUp);

  const refsLayer = doc.createElement("div");
  const conflictLayer = doc.createElement("div");
  conflictLayer.className = "sheet-conflict-layer";
  const filterLayer = doc.createElement("div");
  filterLayer.className = "sheet-filter-layer";
  const selFill = doc.createElement("div");
  selFill.className = "sheet-sel-fill";
  selFill.hidden = true;

  const activeRing = doc.createElement("div");
  activeRing.className = "sheet-active-ring";
  activeRing.hidden = true;

  const resizeGuide = doc.createElement("div");
  resizeGuide.className = "sheet-resize-guide";
  resizeGuide.hidden = true;

  const fillPreview = doc.createElement("div");
  fillPreview.className = "sheet-fill-preview";
  fillPreview.setAttribute("data-fill-preview", "");
  fillPreview.hidden = true;

  const fillHandle = doc.createElement("div");
  fillHandle.className = "sheet-fill-handle";
  fillHandle.setAttribute("data-fill-handle", "");
  fillHandle.hidden = true;
  fillHandle.addEventListener("mousedown", (e) => {
    if (e.button !== undefined && e.button !== 0) return;
    e.preventDefault();
    e.stopPropagation();
    onFillHandleDown?.(e);
  });
  fillHandle.addEventListener("dblclick", (e) => {
    e.preventDefault();
    e.stopPropagation();
    onFillHandleDblClick?.(e);
  });

  function makeTouchHandle(corner) {
    const el = doc.createElement("div");
    el.className = "sheet-touch-handle";
    el.setAttribute("data-touch-handle", corner);
    el.hidden = true;
    el.addEventListener("pointerdown", (e) => {
      e.preventDefault();
      e.stopPropagation();
      el.setPointerCapture?.(e.pointerId);
      onTouchHandleDown?.(corner, e);
    });
    return el;
  }
  const startHandle = makeTouchHandle("start");
  const endHandle = makeTouchHandle("end");

  const chartRing = doc.createElement("div");
  chartRing.className = "sheet-chart-ring";
  chartRing.setAttribute("data-chart-ring", "");
  chartRing.hidden = true;

  const chartHandle = doc.createElement("div");
  chartHandle.className = "sheet-chart-handle";
  chartHandle.setAttribute("data-chart-handle", "");
  chartHandle.hidden = true;
  chartHandle.addEventListener("mousedown", (e) => {
    if (e.button !== undefined && e.button !== 0) return;
    e.preventDefault();
    e.stopPropagation();
    onChartResizeDown?.(e);
  });

  const linkChip = doc.createElement("div");
  linkChip.className = "sheet-link-chip";
  linkChip.setAttribute("data-sheet-link-chip", "");
  linkChip.hidden = true;
  const linkUrlText = doc.createElement("span");
  linkUrlText.setAttribute("data-link-url", "");
  const linkOpen = doc.createElement("a");
  linkOpen.textContent = "Open";
  linkOpen.target = "_blank";
  linkOpen.rel = "noopener noreferrer";
  linkOpen.setAttribute("data-link-open", "");
  linkChip.append(linkUrlText, linkOpen);

  const editorSlot = doc.createElement("div");
  editorSlot.className = "sheet-editor-slot";

  const live = doc.createElement("div");
  live.className = "sheet-sr-only";
  live.setAttribute("aria-live", "polite");
  live.setAttribute("aria-atomic", "true");

  root.append(corner, colsLayer, rowsLayer, conflictLayer, refsLayer, filterLayer, selFill, activeRing, fillPreview, fillHandle, startHandle, endHandle, chartRing, chartHandle, resizeGuide, linkChip, editorSlot, live);

  const ui = mountEditorUI({
    name: "sheet-overlays",
    content: root,
    css: [OVERLAY_CSS],
    parent: workbook.parentElement ?? doc.body,
    before: workbook.nextSibling,
    attributes: { "sap-ignore": "" },
  });

  function setHeaderContent(el, label, gripClass, gripAttr, index) {
    let labelEl = el.firstElementChild;
    let gripEl = labelEl?.nextElementSibling ?? null;
    if (!labelEl || !gripEl) {
      el.textContent = "";
      labelEl = doc.createElement("span");
      gripEl = doc.createElement("div");
      gripEl.className = gripClass;
      el.append(labelEl, gripEl);
    }
    if (labelEl.textContent !== label) labelEl.textContent = label;
    gripEl.setAttribute(gripAttr, String(index));
  }

  function render({
    table,
    win,
    view,
    box,
    whole = null,
    editing = false,
    refs = [],
    conflicts = [],
    shownTab = "",
    fillPreviewBox = null,
    linkUrl = null,
    touch = false,
    filterButtons = [],
    chartBox = null,
  }) {
    lastState = { table, win, view };
    if (!table || !view || view.rows.length === 0 || view.cols.length === 0) {
      colsLayer.textContent = "";
      rowsLayer.textContent = "";
      conflictLayer.textContent = "";
      refsLayer.textContent = "";
      filterLayer.textContent = "";
      selFill.hidden = true;
      activeRing.hidden = true;
      fillHandle.hidden = true;
      startHandle.hidden = true;
      endHandle.hidden = true;
      chartRing.hidden = true;
      chartHandle.hidden = true;
      fillPreview.hidden = true;
      linkChip.hidden = true;
      corner.hidden = true;
      return;
    }

    const origin = root.getBoundingClientRect();
    const metrics = tableMetrics(table, win);
    const firstCell = metrics.cellBox(0, 0);

    corner.hidden = false;
    placeBox(corner, origin, {
      left: firstCell.left - GUTTER_W,
      top: firstCell.top - HEADER_H,
      width: GUTTER_W,
      height: HEADER_H,
    });

    while (colsLayer.children.length > view.cols.length) colsLayer.lastElementChild.remove();
    for (let c = 0; c < view.cols.length; c++) {
      let el = colsLayer.children[c];
      if (!el) {
        el = doc.createElement("div");
        el.className = "sheet-col-hdr";
        colsLayer.append(el);
      }
      const cb = metrics.cellBox(0, c);
      el.hidden = cb.width === 0;
      el.setAttribute("data-col", String(c));
      el.setAttribute("data-active", box && c >= box.left && c <= box.right ? "true" : "false");
      setHeaderContent(el, colName(c), "sheet-col-grip", "data-col-resize", c);
      placeBox(el, origin, { left: cb.left, top: firstCell.top - HEADER_H, width: cb.width, height: HEADER_H });
    }

    const visRows = metrics.visibleRows(view.rows.length);
    while (rowsLayer.children.length > visRows.length) rowsLayer.lastElementChild.remove();
    visRows.forEach((r, idx) => {
      let el = rowsLayer.children[idx];
      if (!el) {
        el = doc.createElement("div");
        el.className = "sheet-row-hdr";
        rowsLayer.append(el);
      }
      const rb = metrics.cellBox(r, 0);
      el.hidden = rb.height === 0;
      el.setAttribute("data-row", String(r));
      const active = Boolean(box && r >= box.top && r <= box.bottom);
      el.setAttribute("data-active", active ? "true" : "false");
      el.setAttribute("data-active-next", active && r < box.bottom ? "true" : "false");
      setHeaderContent(el, String(r + 1), "sheet-row-grip", "data-row-resize", r);
      placeBox(el, origin, { left: firstCell.left - GUTTER_W, top: rb.top, width: GUTTER_W, height: rb.height });
    });

    const visibleConflicts = conflicts.filter((item) => item.tab === shownTab);
    while (conflictLayer.children.length > visibleConflicts.length) conflictLayer.lastElementChild.remove();
    visibleConflicts.forEach((item, idx) => {
      let el = conflictLayer.children[idx];
      if (!el) {
        el = doc.createElement("div");
        el.className = "sheet-conflict-box";
        el.setAttribute("data-sheet-conflict", "");
        conflictLayer.append(el);
      }
      const r0 = Math.max(0, Math.min(view.rows.length - 1, item.r0 ?? 0));
      const r1 = Math.max(0, Math.min(view.rows.length - 1, item.r1 ?? r0));
      const c0 = Math.max(0, Math.min(view.cols.length - 1, item.c0 ?? 0));
      const c1 = Math.max(0, Math.min(view.cols.length - 1, item.c1 ?? c0));
      placeBox(el, origin, metrics.spanBox(r0, c0, r1, c1));
    });

    const visibleRefs = refs.filter((ref) => ref.tab === shownTab);
    while (refsLayer.children.length > visibleRefs.length) refsLayer.lastElementChild.remove();
    visibleRefs.forEach((ref, idx) => {
      let el = refsLayer.children[idx];
      if (!el) {
        el = doc.createElement("div");
        el.className = "sheet-ref-box";
        refsLayer.append(el);
      }
      const r0 = Math.max(0, Math.min(view.rows.length - 1, ref.area.r0 ?? 0));
      const r1 = Math.max(0, Math.min(view.rows.length - 1, ref.area.r1 ?? view.rows.length - 1));
      const c0 = Math.max(0, Math.min(view.cols.length - 1, ref.area.c0 ?? 0));
      const c1 = Math.max(0, Math.min(view.cols.length - 1, ref.area.c1 ?? view.cols.length - 1));
      const palette = REF_COLORS[ref.colorIndex % REF_COLORS.length];
      el.style.setProperty("--ref-border", palette.border);
      el.style.setProperty("--ref-fill", palette.fill);
      placeBox(el, origin, metrics.spanBox(r0, c0, r1, c1));
    });

    const buttonSize = winObj.matchMedia?.("(pointer: coarse)")?.matches ? 28 : 18;
    while (filterLayer.children.length > filterButtons.length) filterLayer.lastElementChild.remove();
    filterButtons.forEach((item, idx) => {
      let el = filterLayer.children[idx];
      if (!el) {
        el = doc.createElement("button");
        el.type = "button";
        el.className = "sheet-filter-btn";
        el.innerHTML =
          '<svg viewBox="0 0 24 24" width="12" height="12" fill="none" stroke="currentColor" stroke-width="2"><path d="M3 5h18l-7 8v6l-4 2v-8Z"/></svg>';
        el.addEventListener("mousedown", (e) => {
          e.preventDefault();
          e.stopPropagation();
        });
        el.addEventListener("click", (e) => {
          e.preventDefault();
          e.stopPropagation();
          onFilterButton?.(Number(el.getAttribute("data-filter-col")), el);
        });
        filterLayer.append(el);
      }
      el.setAttribute("data-filter-col", String(item.c));
      el.setAttribute("aria-label", `Filter column ${colName(item.c)}`);
      el.setAttribute("data-active", item.active ? "true" : "false");
      const cell = metrics.spanBox(item.r, item.c, item.r, item.c);
      const visible = cell.width > 0 && cell.height > 0;
      el.style.display = visible ? "" : "none";
      if (visible) {
        placeBox(el, origin, {
          left: cell.left + cell.width - buttonSize - 3,
          top: cell.top + (cell.height - buttonSize) / 2,
          width: buttonSize,
          height: buttonSize,
        });
      }
    });

    const hasChart = Boolean(chartBox && chartBox.width > 0 && chartBox.height > 0);
    chartRing.hidden = !hasChart;
    chartHandle.hidden = !hasChart;
    if (hasChart) {
      placeBox(chartRing, origin, chartBox);
      placeBox(chartHandle, origin, {
        left: chartBox.left + chartBox.width - 5,
        top: chartBox.top + chartBox.height - 5,
        width: 9,
        height: 9,
      });
    }

    if (!box) {
      selFill.hidden = true;
      activeRing.hidden = true;
      fillHandle.hidden = true;
      startHandle.hidden = true;
      endHandle.hidden = true;
      fillPreview.hidden = true;
      linkChip.hidden = true;
      placeBox(editorSlot, origin, { left: origin.left, top: origin.top, width: 1, height: 1 });
      editorSlot.style.opacity = "0";
      editorSlot.style.pointerEvents = "none";
      return;
    }
    editorSlot.style.opacity = "";
    editorSlot.style.pointerEvents = "";

    const activeBox = metrics.spanBox(
      box.active.r,
      box.active.c,
      box.active.r + box.active.rows - 1,
      box.active.c + box.active.cols - 1,
    );
    const wholeBox = metrics.spanBox(box.top, box.left, box.bottom, box.right);
    const multi =
      box.top !== box.active.r ||
      box.left !== box.active.c ||
      box.bottom !== box.active.r + box.active.rows - 1 ||
      box.right !== box.active.c + box.active.cols - 1;

    placeBox(editorSlot, origin, activeBox);
    if (hasChart) {
      selFill.hidden = true;
      activeRing.hidden = true;
      fillHandle.hidden = true;
      startHandle.hidden = true;
      endHandle.hidden = true;
      fillPreview.hidden = true;
      linkChip.hidden = true;
      return;
    }

    selFill.hidden = !multi;
    if (multi) placeBox(selFill, origin, wholeBox);

    activeRing.hidden = false;
    activeRing.setAttribute("data-editing", editing ? "true" : "false");
    placeBox(activeRing, origin, activeBox);

    const showHandle = !editing && whole !== "rows" && whole !== "cols" && wholeBox.width > 0 && wholeBox.height > 0;
    fillHandle.hidden = !showHandle || touch;
    startHandle.hidden = !showHandle || !touch;
    endHandle.hidden = !showHandle || !touch;
    if (showHandle && !touch) {
      placeBox(fillHandle, origin, {
        left: wholeBox.left + wholeBox.width - 4,
        top: wholeBox.top + wholeBox.height - 4,
        width: 7,
        height: 7,
      });
    }
    if (showHandle && touch) {
      placeBox(startHandle, origin, { left: wholeBox.left - 22, top: wholeBox.top - 22, width: 44, height: 44 });
      placeBox(endHandle, origin, { left: wholeBox.left + wholeBox.width - 22, top: wholeBox.top + wholeBox.height - 22, width: 44, height: 44 });
    }

    if (fillPreviewBox) {
      fillPreview.hidden = false;
      placeBox(fillPreview, origin, metrics.spanBox(fillPreviewBox.top, fillPreviewBox.left, fillPreviewBox.bottom, fillPreviewBox.right));
    } else {
      fillPreview.hidden = true;
    }

    if (!editing && linkUrl) {
      const shownUrl = linkUrl.length > 60 ? `${linkUrl.slice(0, 59)}…` : linkUrl;
      if (linkUrlText.textContent !== shownUrl) linkUrlText.textContent = shownUrl;
      if (linkOpen.getAttribute("href") !== linkUrl) linkOpen.setAttribute("href", linkUrl);
      linkChip.style.left = `${activeBox.left - origin.left}px`;
      linkChip.style.top = `${activeBox.top + activeBox.height + 4 - origin.top}px`;
      linkChip.hidden = false;
    } else {
      linkChip.hidden = true;
    }
  }

  const api = {
    host: ui.host,
    shadow: ui.shadow,
    root,
    editorSlot,
    resizeGuide,
    fillHandle,
    fillPreview,
    chartRing,
    chartHandle,
    linkChip,
    live,
    render,
    announce(text) {
      if (live.textContent !== text) live.textContent = text;
    },
    cellAtPoint: (table, win, x, y) => (table ? tableMetrics(table, win).cellAt(x, y) : null),
    cellBox: (table, win, r, c) => (table ? tableMetrics(table, win).cellBox(r, c) : null),
    visibleRows: (table, win, total) => (table ? tableMetrics(table, win).visibleRows(total) : []),
    dispose() {
      resizing = null;
      winObj.removeEventListener("mousemove", onWinMouseMove);
      winObj.removeEventListener("mouseup", onWinMouseUp);
      ui.remove();
    },
  };
  return api;
}
