import { a1, colIndex, rowIndex, tabTable } from "./commit.js";
import { createChrome } from "./chrome.js";
import { copyParts, fillBlock, fillFormula, pasteBlock, transposeMatrix } from "./clipboard.js";
import { createEditor } from "./editor.js";
import { allowedInList, boundRange, COL_DEFAULT, createGeometry, filteredRows, inputRuleAt, listItems, parseRuleStyle, ROW_ESTIMATE, rowOutOfSight, setFilteredRows, tableMetrics } from "./geometry.js";
import { createHistory } from "./history.js";
import { keyIntent } from "./keys.js";
import { createOverlays } from "./overlays.js";
import { loadCharts, peekCharts, sheetHooks } from "./shell.js";
import {
  axisSpan,
  clipSelection,
  commandArea,
  createSelection,
  edgeSelection,
  moveSelection,
  ownerOf,
  pageSelection,
  readSelection,
  rowEdgeSelection,
  selectAll,
  selectCell,
  selectCols,
  selectColsAt,
  selectionArea,
  selectionBox,
  selectRows,
  selectRowsAt,
  sheetCornerSelection,
  writeSelection,
} from "./selection.js";
import { createTabs } from "./tabs.js";

const WORKBOOK = "[sheet-workbook]";
const STATUS_CELL_CAP = 100_000;
const CHANGED_MSG = "Those cells changed since, so that step was not undone.";
const REDONE_MSG = "Those cells changed since, so that step was not redone.";

const FORMAT_LABELS = new Map([
  [null, "Automatic"],
  ["@", "Plain text"],
  ["#,##0.00", "Number"],
  ["#,##0", "Thousands"],
  ["0.00%", "Percent"],
  ["$#,##0.00", "Currency"],
  ["m/d/yyyy", "Date"],
  ["mmmm d, yyyy", "Long date"],
  ["h:mm AM/PM", "Time"],
  ["m/d/yyyy h:mm", "Date and time"],
]);

const LAZY_TOOLBAR_IDS = [
  "number-format",
  "currency",
  "percent",
  "decimals-less",
  "decimals-more",
  "bold",
  "italic",
  "strikethrough",
  "text-color",
  "fill-color",
  "borders",
  "merge",
  "align",
  "wrap",
  "insert-row",
  "add-tab",
];

let importChain = Promise.resolve();
const queueImport = (loader) => {
  const next = importChain.then(loader);
  importChain = next.catch(() => {});
  return next;
};

const LAZY_FAIL_MESSAGE = "That tool could not load. Check your connection and try again.";

export function lazyModule(loader, onFail) {
  let promise = null;
  let value = null;
  return {
    peek: () => value,
    put: (mod) => {
      value = mod;
      return mod;
    },
    load: () => {
      if (promise) return promise;
      promise = queueImport(loader).then(
        (mod) => {
          value = mod;
          return mod;
        },
        () => {
          promise = null;
          onFail?.();
          return null;
        },
      );
      return promise;
    },
  };
}

const samePoint = (a, b) => a?.row === b?.row && a?.col === b?.col;

function sameSelection(a, b) {
  if (a === b) return true;
  if (!a || !b) return false;
  return a.tab === b.tab && samePoint(a.anchor, b.anchor) && samePoint(a.focus, b.focus);
}

function scrollerOf(el) {
  const doc = el.ownerDocument ?? document;
  for (let node = el.parentElement; node && node !== doc.body && node !== doc.documentElement; node = node.parentElement) {
    const { overflow, overflowY, overflowX } = getComputedStyle(node);
    if (/\b(auto|scroll)\b/.test(`${overflow ?? ""} ${overflowY ?? ""} ${overflowX ?? ""}`)) return node;
  }
  return null;
}

function tableRows(table) {
  const rows = [];
  for (const section of [table.tHead, ...table.tBodies]) {
    if (!section) continue;
    for (const tr of section.rows) rows.push(tr);
  }
  return rows;
}

function tableCols(table) {
  const cols = [];
  for (let node = table.firstElementChild; node !== null; node = node.nextElementSibling) {
    if (node.localName !== "colgroup") continue;
    for (let col = node.firstElementChild; col !== null; col = col.nextElementSibling) {
      if (col.localName === "col") cols.push(col);
    }
    break;
  }
  return cols;
}

function columnOfCell(td) {
  let c = 0;
  for (let prev = td.previousElementSibling; prev !== null; prev = prev.previousElementSibling) {
    if (prev.localName === "td" || prev.localName === "th") c += 1;
  }
  return c;
}

const formatCount = (n, singular, plural) => `${n.toLocaleString("en-US")} ${n === 1 ? singular : plural}`;

function quickToolbarStyle(cell) {
  const css = typeof cell?.style === "string" ? cell.style : "";
  let fw = "";
  let fs = "";
  let td = "";
  let ws = "";
  if (css) {
    for (const part of css.split(";")) {
      const colon = part.indexOf(":");
      if (colon === -1) continue;
      const k = part.slice(0, colon).trim().toLowerCase();
      const v = part.slice(colon + 1).trim().toLowerCase();
      if (k === "font-weight") fw = v;
      else if (k === "font-style") fs = v;
      else if (k === "text-decoration") td = v;
      else if (k === "white-space") ws = v;
    }
  }
  const rawFmt = cell?.effectiveFmt;
  const normFmt = rawFmt === "" || rawFmt === undefined ? null : rawFmt;
  return {
    bold: fw === "bold" || fw === "bolder" || (/^\d+$/.test(fw) && Number(fw) >= 700),
    italic: fs === "italic" || fs === "oblique",
    strikethrough: /\bline-through\b/.test(td),
    wrap: ws === "pre-wrap",
    formatLabel: FORMAT_LABELS.get(normFmt) ?? "Custom",
  };
}

function quickMergeState(grid, view, sel) {
  const box = selectionBox(view, sel);
  if (!box || !grid) return { canAct: false, isSingleMerge: false };
  if ((box.bottom - box.top + 1) * (box.right - box.left + 1) < 2) return { canAct: false, isSingleMerge: false };
  const tl = ownerOf(view, box.top, box.left);
  return {
    canAct: true,
    isSingleMerge:
      tl.r === box.top &&
      tl.c === box.left &&
      tl.rows === box.bottom - box.top + 1 &&
      tl.cols === box.right - box.left + 1,
  };
}

function safeLinkUrl(raw) {
  if (typeof raw !== "string") return null;
  const str = raw.trim();
  if (!str || /\s/.test(str) || !/^(?:https?:\/\/|mailto:)/i.test(str)) return null;
  let parsed;
  try {
    parsed = new URL(str);
  } catch {
    return null;
  }
  if (parsed.protocol === "http:" || parsed.protocol === "https:") return parsed.hostname ? str : null;
  if (parsed.protocol === "mailto:") return parsed.pathname.trim() ? str : null;
  return null;
}

function extractHyperlinkLiteral(formula, tokenize) {
  if (typeof tokenize !== "function") return null;
  const tokens = tokenize(formula);
  if (
    tokens.length < 5 ||
    tokens[0]?.value !== "=" ||
    tokens[1]?.type !== "name" ||
    tokens[1]?.value !== "HYPERLINK" ||
    tokens[2]?.type !== "punct" ||
    tokens[2]?.value !== "(" ||
    tokens[3]?.type !== "str" ||
    tokens[4]?.type !== "punct" ||
    (tokens[4].value !== "," && tokens[4].value !== ")")
  ) {
    return null;
  }
  let depth = 0;
  for (let i = 2; i < tokens.length; i++) {
    const t = tokens[i];
    if (t.type === "punct" && t.value === "(") depth += 1;
    else if (t.type === "punct" && t.value === ")") {
      depth -= 1;
      if (depth === 0) return i === tokens.length - 1 ? { literal: tokens[3].value } : null;
    }
  }
  return null;
}

export function extractCellLink(cell, tokenize) {
  if (!cell) return null;
  if (cell.content !== null && typeof cell.content === "object" && typeof cell.content.f === "string") {
    const hl = extractHyperlinkLiteral(cell.content.f, tokenize);
    if (hl) return safeLinkUrl(hl.literal);
  }
  return safeLinkUrl(cell.text);
}

function createWorkbookInterface(workbook, { shell, storage, tabOptions, commands, platform, measureText, loaders = {} }) {
  const doc = workbook.ownerDocument ?? document;
  const win = doc.defaultView ?? window;
  const scrollBox = scrollerOf(workbook) ?? workbook;
  const scrollTarget = scrollerOf(workbook) ?? win;
  const path = win.location?.pathname ?? "/";
  const externalCommands = { ...commands };

  const tabs = createTabs(workbook, tabOptions);
  const lastViews = new Map();
  // One filter per tab, this viewer's only, never saved:
  // { header: rowId, cols: [colId], criteria: Map<colId, { values: Set<string> | null, condition: { op, arg } | null }> }
  const filters = new Map();
  const unsubGrids = [];
  const seenKeys = new WeakSet();
  let sel = null;
  let disposed = false;
  let mine = false;
  let valuesNext = false;
  let activeDialog = null;
  let dialogTab = "";
  let liveMenu = null;
  let cmdActions = null;
  let findCtrl = null;
  let fnHelp = null;
  let fillDrag = null;
  let selectedChart = null;
  let chartDrag = null;
  let statusCache = "";

  const alive = () => !disposed;
  const onLazyFail = () => {
    if (alive()) chrome.showNotice(LAZY_FAIL_MESSAGE);
  };
  const cmdTool = lazyModule(loaders.commands ?? (() => import("./format-commands.js")), onLazyFail);
  const fillTool = lazyModule(loaders.fill ?? (() => import("./fill.js")), onLazyFail);
  const findTool = lazyModule(loaders.find ?? (() => import("./find.js")), onLazyFail);
  const pasteSpecialTool = lazyModule(loaders.pasteSpecial ?? (() => import("./paste-special.js")), onLazyFail);
  const shortcutsTool = lazyModule(loaders.shortcuts ?? (() => import("./shortcuts.js")), onLazyFail);
  // Function help is never asked for, so a failed load stays quiet instead of a notice per keystroke.
  const functionHelpTool = lazyModule(loaders.functionHelp ?? (() => import("./function-help.js")), null);
  const csvTool = lazyModule(
    loaders.csv ??
      (async () => {
        if (!cmdTool.peek()) cmdTool.put(await import("./format-commands.js"));
        return import("./csv.js");
      }),
    onLazyFail,
  );
  const xlsxTool = lazyModule(loaders.xlsx ?? (() => import("./xlsx.js")), onLazyFail);
  const filterTool = lazyModule(loaders.filter ?? (() => import("./filter.js")), onLazyFail);
  const rulesTool = lazyModule(loaders.rules ?? (() => import("./rule-panels.js")), onLazyFail);
  const chartPanelTool = lazyModule(loaders.chartPanel ?? (() => import("./chart-panel.js")), onLazyFail);
  let pendingLazy = null;

  function runLazy(holder, fn, { guardSel = false } = {}) {
    const snap = sel;
    const ready = holder.peek();
    if (ready) {
      pendingLazy = null;
      if (!guardSel || sameSelection(sel, snap)) fn(ready);
      return;
    }
    pendingLazy = fn;
    holder.load().then((mod) => {
      if (!alive() || !mod || pendingLazy !== fn) return;
      pendingLazy = null;
      if (guardSel && !sameSelection(sel, snap)) return;
      fn(mod);
    });
  }
  const gridOf = (table) => (table && typeof win.Sap?.grid === "function" ? win.Sap.grid(table) : null);
  const rawHistory = createHistory(workbook, gridOf);
  // A live room owns the workbook's history while bound: undo is the room's guarded revert,
  // and multi-step commands grouped for one undo travel as one room batch.
  const room = () => sheetHooks.bookOf?.(workbook)?.room ?? null;
  const history = {
    ...rawHistory,
    group(tab, fn) {
      const live = room();
      if (live?.group) return live.group(tab, fn);
      return rawHistory.group(tab, fn);
    },
  };
  const canUndo = (tab) => room()?.canUndo(tab) ?? history.canUndo(tab);
  const canRedo = (tab) => room()?.canRedo(tab) ?? history.canRedo(tab);
  let conflictTargets = [];
  const isMac = () => /mac|iphone|ipad|ipod/i.test(String(platform ?? win.navigator?.platform ?? ""));

  function withMine(fn) {
    mine = true;
    try {
      return fn();
    } finally {
      mine = false;
    }
  }

  function writeGrid(fn) {
    try {
      withMine(fn);
      chrome.hideNotice();
      return true;
    } catch (error) {
      if (error?.name !== "GridRefused") throw error;
      if (!sheetHooks.onGridRefused?.(workbook)) {
        chrome.showNotice(error.message || "That step could not be applied.");
      }
      return false;
    }
  }

  function runTabStep(fn) {
    try {
      fn();
      chrome.hideNotice();
      return true;
    } catch (error) {
      if (error?.name !== "GridRefused") throw error;
      chrome.showNotice(error.message || "That sheet action could not be applied.");
      return false;
    }
  }

  function clearDialog() {
    activeDialog = null;
    dialogTab = "";
  }

  function closeDialog() {
    const handle = activeDialog;
    clearDialog();
    handle?.close?.();
  }

  function showDialog(tabId, handle) {
    closeDialog();
    activeDialog = handle;
    dialogTab = tabId;
    return handle;
  }

  function closeTransient() {
    if (liveMenu?.isOpen) liveMenu.close({ restoreFocus: false });
    closeFilterPanel();
    closeRuleSurface();
    closeDialog();
  }

  let filterPanelClose = null;
  let filterPanelKey = "";
  let ruleSurface = null;
  let dismissedPickKey = "";

  function closeRuleSurface() {
    const cur = ruleSurface;
    ruleSurface = null;
    cur?.close?.();
  }

  function closeFilterPanel() {
    const close = filterPanelClose;
    filterPanelClose = null;
    filterPanelKey = "";
    close?.();
  }

  const filterPanelOpen = () => Boolean(chrome.topRoot?.querySelector?.("[data-filter-panel]"));

  function openFilterAt(tabId, c, anchor) {
    const colId = viewOf(tabId).cols[c];
    if (!colId) return;
    const key = `${tabId}:${colId}`;
    closeRuleSurface();
    if (filterPanelOpen()) {
      const same = filterPanelKey === key;
      closeFilterPanel();
      if (same) return;
    }
    runLazy(filterTool, (f) => {
      const close = f.openFilterPanel(sharedCtx(), tabId, colId, anchor);
      if (typeof close === "function") {
        filterPanelClose = close;
        filterPanelKey = key;
      }
    });
  }

  function openPickListForCell(tabId, r, c) {
    const view = viewOf(tabId);
    const own = ownerOf(view, r, c);
    const key = `${tabId}:${view.rows[own.r]}:${view.cols[own.c]}`;
    if (dismissedPickKey === key) {
      dismissedPickKey = "";
      return;
    }
    if (ruleSurface?.key === key) {
      closeRuleSurface();
      return;
    }
    closeTransient();
    runLazy(rulesTool, (mod) => {
      const handle = mod.openPickList(sharedCtx(), { tab: tabId, r: own.r, c: own.c });
      if (handle) {
        handle.setOnDismissOutside?.((target) => {
          ruleSurface = null;
          dismissedPickKey = `${target.tab}:${target.row}:${target.col}`;
          win.setTimeout(() => {
            dismissedPickKey = "";
          }, 0);
        });
        ruleSurface = handle;
      }
    });
  }

  function openRulePanel(kind, anchor = null) {
    if (!ensureReadyForAction()) return;
    closeTransient();
    runLazy(rulesTool, (mod) => {
      ruleSurface = kind === "dropdown" ? mod.openDropdownPanel(sharedCtx(), anchor) : mod.openConditionalPanel(sharedCtx(), anchor);
    });
  }

  const CHART_BIG = { rows: 10000000, cols: 16384 };
  const MAX_CHART_COL = 16383;
  const MAX_CHART_ROW = 9999998;
  const CHART_DRAG_SLOP = 4;

  function activeChart() {
    if (!selectedChart || selectedChart.tab !== tabs.activeId) return null;
    const table = tabTable(workbook, selectedChart.tab);
    const grid = gridOf(table);
    const rule = grid?.rules?.().find((r) => r.kind === "chart" && r.id === selectedChart.id) ?? null;
    if (!rule) {
      selectedChart = null;
      return null;
    }
    return { tab: selectedChart.tab, table, grid, rule };
  }

  function chartUnderActiveCell() {
    const table = sel ? tabTable(workbook, sel.tab) : null;
    const grid = gridOf(table);
    const box = sel ? selectionBox(viewOf(sel.tab), sel) : null;
    if (!grid || !box) return null;
    const rules = grid.rules?.() ?? [];
    for (let i = rules.length - 1; i >= 0; i--) {
      const rule = rules[i];
      if (rule.kind !== "chart") continue;
      const r = boundRange(rule.range, CHART_BIG);
      if (r && box.active.r >= r.r0 && box.active.r <= r.r1 && box.active.c >= r.c0 && box.active.c <= r.c1) {
        return { tab: sel.tab, table, grid, rule };
      }
    }
    return null;
  }

  function chartHitAt(clientX, clientY) {
    const table = tabTable(workbook, tabs.activeId);
    if (!table) return null;
    return peekCharts()?.chartAt(clientX, clientY, table) ?? null;
  }

  function watchTabCharts(table, grid) {
    const ready = peekCharts();
    if (ready) {
      ready.watch(table, grid, () => windowOf(table));
      return;
    }
    loadCharts().then((charts) => {
      if (!alive()) return;
      charts.watch(table, grid, () => windowOf(table));
      syncUI({ announce: false });
    });
  }

  function openChartPanel(ruleId, anchor = null) {
    if (!ensureReadyForAction()) return;
    const tabId = tabs.activeId;
    closeTransient();
    runLazy(chartPanelTool, (mod) => {
      ruleSurface = mod.openChartPanel(sharedCtx(), { tab: tabId, id: ruleId }, anchor);
    });
  }

  function runInsertChart() {
    if (!ensureReadyForAction() || !sel) return;
    const view = viewOf(sel.tab);
    if (!selectionBox(view, sel)) return;
    closeTransient();
    loadCharts();
    runLazy(chartPanelTool, (mod) => {
      ruleSurface = mod.insertChart(sharedCtx(), chrome.getButton("chart"));
    });
  }

  function deleteChartById(tabId, ruleId) {
    if (!ensureReadyForAction()) return;
    const table = tabTable(workbook, tabId);
    const grid = gridOf(table);
    if (!grid) return;
    if (!writeGrid(() => grid.setRules({ remove: [ruleId] }))) return;
    if (selectedChart?.id === ruleId) selectedChart = null;
    chartDrag = null;
    refreshStatus();
    syncUI({ announce: true });
    editor.focus();
  }

  function nearestGridLine(table, rw, clientX, clientY) {
    const headRows = table.tHead?.rows.length ?? 0;
    const geom = rw ? rw.geometry : createGeometry(table);
    const metrics = tableMetrics(table, rw);
    const first = metrics.cellBox(0, 0);
    const colCount = tableCols(table).length;

    let col = 0;
    const relX = clientX - first.left;
    if (relX <= 0) {
      col = 0;
    } else if (colCount > 0 && relX < geom.width) {
      const c = geom.colAt(relX);
      const cb = metrics.cellBox(0, c);
      col = clientX >= cb.left + cb.width / 2 ? c + 1 : c;
    } else {
      col = colCount + Math.round((relX - geom.width) / COL_DEFAULT);
    }

    let row = 0;
    const bodyFirst = metrics.cellBox(headRows, 0);
    if (clientY <= first.top) {
      row = 0;
    } else if (headRows > 0 && clientY < bodyFirst.top) {
      for (let r = 0; r < headRows; r++) {
        const hb = metrics.cellBox(r, 0);
        if (clientY < hb.top + hb.height) {
          row = clientY >= hb.top + hb.height / 2 ? r + 1 : r;
          break;
        }
        row = r + 1;
      }
    } else {
      const relY = clientY - bodyFirst.top;
      if (relY <= 0) {
        row = headRows;
      } else if (geom.rowCount > 0 && relY < geom.total()) {
        const br = geom.rowAt(relY);
        const rb = metrics.cellBox(headRows + br, 0);
        row = headRows + (clientY >= rb.top + rb.height / 2 ? br + 1 : br);
      } else {
        row = headRows + geom.rowCount + Math.round((relY - geom.total()) / ROW_ESTIMATE);
      }
    }
    return { row: Math.max(0, row), col: Math.max(0, col) };
  }

  function touchesSelection(tabId, cells) {
    if (!cells?.length || !sel || sel.tab !== tabId) return false;
    const view = viewOf(tabId);
    const box = selectionBox(view, sel);
    if (!box) return false;
    const rows = new Set(view.rows.slice(box.top, box.bottom + 1));
    const cols = new Set(view.cols.slice(box.left, box.right + 1));
    return cells.some((cell) => rows.has(cell.row) && cols.has(cell.col));
  }

  function onForeignChange(tabId, event = null) {
    if (room() && event) {
      const structural = Boolean(
        event.structure ||
          event.rows?.added?.length ||
          event.rows?.removed?.length ||
          event.cols?.added?.length ||
          event.cols?.removed?.length,
      );
      if (!structural && !touchesSelection(tabId, event.cells)) return;
    }
    if (liveMenu?.isOpen && tabId === sel?.tab) liveMenu.close({ restoreFocus: false });
    if (ruleSurface && ruleSurface.tab === tabId) closeRuleSurface();
    if (activeDialog && dialogTab === tabId) closeDialog();
  }

  const windowOf = (table) => shell?.windowOf?.(table) ?? null;

  function viewOf(tabId) {
    const table = tabTable(workbook, tabId);
    if (!table) return { rows: [], cols: [], frozenRows: 0, rowHidden: () => false, colHidden: () => false, cell: () => null };
    const grid = gridOf(table);
    const trs = tableRows(table);
    const colEls = tableCols(table);
    const rows = trs.map((tr) => tr.getAttribute("data-id") ?? "");
    const cols = colEls.map((col) => col.getAttribute("data-id") ?? "");
    return {
      rows,
      cols,
      frozenRows: table.tHead?.rows.length ?? 0,
      rowHidden: (r) => rowOutOfSight(trs[r]),
      colHidden: (c) => Boolean(colEls[c]?.hidden),
      cell(r, c) {
        if (!grid || r < 0 || c < 0 || r >= rows.length || c >= cols.length) return null;
        return grid.cell(a1(r, c));
      },
    };
  }

  function applyHiddenRows(tabId, ids) {
    const table = tabTable(workbook, tabId);
    if (!table) return;
    setFilteredRows(table, ids);
    windowOf(table)?.refresh?.();
    peekCharts()?.refresh(table);
    refreshStatus();
    syncUI();
  }

  function setFilter(tabId, spec) {
    if (spec) filters.set(tabId, spec);
    else {
      filters.delete(tabId);
      applyHiddenRows(tabId, null);
    }
    syncUI();
  }

  function snapshotViews() {
    for (const tab of tabs.list()) {
      const v = viewOf(tab.id);
      lastViews.set(tab.id, { rows: v.rows, cols: v.cols });
    }
  }

  function computeStatusText() {
    if (!sel || tabs.activeId !== sel.tab) return "";
    const grid = gridOf(tabTable(workbook, sel.tab));
    if (!grid) return "";
    const box = selectionBox(viewOf(sel.tab), sel);
    if (!box) return "";
    const spanCount = (box.bottom - box.top + 1) * (box.right - box.left + 1);
    if (spanCount < 2 || spanCount > STATUS_CELL_CAP) return "";
    if (
      box.top === box.active.r &&
      box.left === box.active.c &&
      box.bottom === box.active.r + box.active.rows - 1 &&
      box.right === box.active.c + box.active.cols - 1
    ) {
      return "";
    }
    const filtered = filteredRows(tabTable(workbook, sel.tab));
    const view = viewOf(sel.tab);
    let uncovered = 0;
    let filled = 0;
    let nums = 0;
    let sum = 0;
    for (let r = box.top; r <= box.bottom; r++) {
      if (filtered.has(view.rows[r])) continue;
      for (let c = box.left; c <= box.right; c++) {
        const cell = grid.cell(a1(r, c));
        if (!cell || cell.covered) continue;
        uncovered += 1;
        if (cell.content !== null && cell.content !== "") filled += 1;
        if (typeof cell.value === "number" && Number.isFinite(cell.value)) {
          nums += 1;
          sum += cell.value;
        }
      }
    }
    if (uncovered < 2) return "";
    if (nums > 0) {
      const activeFmt = grid.cell(a1(box.active.r, box.active.c))?.effectiveFmt ?? null;
      const sap = win.Sap?.grid;
      const kind = typeof sap?.fmtKind === "function" ? sap.fmtKind(activeFmt) : "auto";
      const numFmt = kind === "number" || kind === "currency" || kind === "percent" ? activeFmt : null;
      const fmtNum = (n) =>
        typeof sap?.formatValue === "function" ? (sap.formatValue(n, numFmt) ?? sap.formatValue(n, null) ?? String(n)) : String(n);
      return `Sum ${fmtNum(Number(sum.toPrecision(15)))} · Average ${fmtNum(Number((sum / nums).toPrecision(15)))} · Count ${nums}`;
    }
    return filled > 0 ? `Count ${filled}` : "";
  }

  const refreshStatus = () => {
    statusCache = computeStatusText();
  };

  function activeCellInfo(overrideTarget = null) {
    const point = overrideTarget ?? (sel ? { tab: sel.tab, row: sel.anchor.row, col: sel.anchor.col } : null);
    if (!point?.tab) return null;
    const table = tabTable(workbook, point.tab);
    if (!table) return { ...point, r: -1, c: -1, ref: "A1", text: "", input: "", effectiveFmt: null, cell: null };
    const r = rowIndex(table, point.row);
    const c = colIndex(table, point.col);
    if (r === -1 || c === -1) return { ...point, r: -1, c: -1, ref: "A1", text: "", input: "", effectiveFmt: null, cell: null };
    const ref = a1(r, c);
    const grid = gridOf(table);
    const cell = grid ? grid.cell(ref) : null;
    return {
      tab: point.tab,
      row: point.row,
      col: point.col,
      r,
      c,
      ref,
      text: cell?.text ?? "",
      input: grid ? grid.input(ref) : "",
      effectiveFmt: cell?.effectiveFmt ?? null,
      cell,
    };
  }

  function cellSpeech(enteredTab = false) {
    const info = activeCellInfo();
    if (!info) return "";
    const valuePart = info.text ? `${info.ref}, ${info.text}` : `${info.ref}, empty`;
    if (!enteredTab) return valuePart;
    const tab = tabs.list().find((t) => t.id === info.tab);
    const view = viewOf(info.tab);
    return `${tab?.name || info.tab}, ${formatCount(view.rows.length, "row", "rows")}, ${formatCount(view.cols.length, "column", "columns")}, ${valuePart}`;
  }

  function ensureReadyForAction() {
    editor.settle();
    if (editor.editing && !commitEdit().ok) return false;
    return true;
  }

  function runFormat(fn) {
    if (editor.editing || !sel) return false;
    if (activeChart()) {
      selectedChart = null;
      syncUI({ announce: false });
      return false;
    }
    const table = tabTable(workbook, sel.tab);
    const grid = gridOf(table);
    if (!grid) return false;
    const view = viewOf(sel.tab);
    if (view.rows.length === 0 || view.cols.length === 0) return false;
    if (!writeGrid(() => fn(grid, view, table))) return false;
    refreshStatus();
    syncUI({ announce: true });
    return true;
  }

  const sharedCtx = () => ({
    doc,
    win,
    workbook,
    chrome,
    overlays,
    editor,
    tabs,
    history,
    getSel: () => sel,
    setSel: (next) => {
      sel = next;
    },
    getFilter: (tabId) => filters.get(tabId) ?? null,
    setFilter,
    applyHiddenRows,
    viewOf,
    gridOf,
    windowOf,
    boundRange,
    inputRuleAt,
    listItems,
    parseStyle: parseRuleStyle,
    activeCellInfo,
    ensureReadyForAction,
    runFormat,
    writeGrid,
    withMine,
    commitEdit,
    runTabStep,
    syncUI,
    refreshStatus,
    applySelection,
    applyMove,
    subscribeGrids,
    restart,
    showDialog,
    clearDialog,
    applyPasteData,
    chartRect: (table, id) => peekCharts()?.chartRect(table, id) ?? null,
    onChartInserted(tabId, table, grid, rule) {
      selectedChart = { tab: tabId, id: rule.id };
      chartDrag = null;
      watchTabCharts(table, grid);
      refreshStatus();
      syncUI({ announce: true });
    },
    clearChartSelection(id) {
      if (!id || selectedChart?.id === id) selectedChart = null;
      chartDrag = null;
    },
    // Not through runLazy: a picked file must reach the import even if another lazy action starts
    // while xlsx.js loads.
    xlsx: (fn) => xlsxTool.load().then((mod) => mod && alive() && fn(mod)),
    platform: platform ?? win.navigator?.platform ?? "",
    isMac,
    measureText,
  });

  const triggerFillShortcut = (axis) => runLazy(fillTool, (f) => f.runShortcutFill(sharedCtx(), axis), { guardSel: true });

  function ensureCommands(b) {
    liveMenu ??= b.createMenu({
      container: chrome.topRoot,
      theme: chrome.theme,
      getReturnFocus: () => editor.textarea,
    });
    cmdActions ??= b.createCommandActions({
      ...sharedCtx(),
      menu: liveMenu,
      onFillShortcut: triggerFillShortcut,
      onPasteSpecial: (mode) => runLazy(pasteSpecialTool, (ps) => ps.runPasteSpecial(sharedCtx(), mode)),
      onClipboard: (kind) => runClipboard(kind),
      onShortcuts: () => runLazy(shortcutsTool, (s) => s.showShortcutsDialog(sharedCtx())),
      onDropdown: () => openRulePanel("dropdown"),
      onConditionalFormat: () => openRulePanel("conditional"),
      onInsertChart: () => (typeof externalCommands.chart === "function" ? externalCommands.chart() : runInsertChart()),
      onEditChart: (ruleId) => openChartPanel(ruleId),
      onDeleteChart: (ruleId) => deleteChartById(tabs.activeId, ruleId),
    });
    return { menu: liveMenu, actions: cmdActions };
  }

  const ensureFind = (f) => (findCtrl ??= f.createFindController(sharedCtx()));

  function ensureFunctionHelp(mod) {
    fnHelp ??= mod.createFunctionHelp({
      // The docked edit bar has its quick keys where the hint would sit.
      isDocked: () => chrome.docked(),
      container: chrome.topRoot,
      win,
      theme: chrome.theme,
      editor,
      overlays,
    });
    return fnHelp;
  }

  function syncFunctionHelp() {
    if (!editor.editing || editor.isTextCell() || !editor.buffer.startsWith("=")) {
      fnHelp?.update();
      return;
    }
    const ready = functionHelpTool.peek();
    if (ready) {
      ensureFunctionHelp(ready).update();
      return;
    }
    functionHelpTool.load().then((mod) => {
      if (!alive() || !mod) return;
      ensureFunctionHelp(mod).update();
    });
  }

  const overlays = createOverlays(workbook, {
    onSelectCol(c, extend) {
      editor.settle();
      if (editor.editing && editor.canPick()) return editor.focus();
      if (editor.editing && !commitEdit().ok) return;
      applySelection(selectColsAt(viewOf(sel.tab), sel, c, { extend }));
      editor.focus();
    },
    onSelectRow(r, extend) {
      editor.settle();
      if (editor.editing && editor.canPick()) return editor.focus();
      if (editor.editing && !commitEdit().ok) return;
      applySelection(selectRowsAt(viewOf(sel.tab), sel, r, { extend }));
      editor.focus();
    },
    onSelectAll() {
      editor.settle();
      if (editor.editing && editor.canPick()) return;
      if (editor.editing && !commitEdit().ok) return;
      applySelection(selectAll(viewOf(sel.tab), sel));
      editor.focus();
    },
    onResizeCol: (c, px) => runLazy(cmdTool, (b) => ensureCommands(b).actions.handleResizeCol(c, px)),
    onResizeRow: (r, px) => runLazy(cmdTool, (b) => ensureCommands(b).actions.handleResizeRow(r, px)),
    onFitCol: (c) => runLazy(cmdTool, (b) => ensureCommands(b).actions.handleFitCol(c)),
    onColMenu: (c, hdr, x, y) => runLazy(cmdTool, (b) => ensureCommands(b).actions.openColHeaderMenu(c, hdr, x, y)),
    onRowMenu: (r, hdr, x, y) => runLazy(cmdTool, (b) => ensureCommands(b).actions.openRowHeaderMenu(r, hdr, x, y)),
    onFillHandleDown(e) {
      if (editor.editing || !sel || sel.whole) return;
      const box = selectionBox(viewOf(sel.tab), sel);
      if (!box) return;
      fillDrag = {
        tab: sel.tab,
        sourceBox: { top: box.top, left: box.left, bottom: box.bottom, right: box.right },
        startX: e.clientX ?? 0,
        startY: e.clientY ?? 0,
        hasCoords: (e.clientX ?? 0) !== 0 || (e.clientY ?? 0) !== 0,
        lastHit: null,
        lastDx: 0,
        lastDy: 0,
        targetBox: null,
      };
      fillTool.load();
    },
    onFillHandleDblClick: () => runLazy(fillTool, (f) => f.runAutoFillDown(sharedCtx()), { guardSel: true }),
    onTouchHandleDown(corner, event) {
      if (editor.editing || !sel) return;
      const box = selectionBox(viewOf(sel.tab), sel);
      if (!box) return;
      cancelLongPress();
      const fixed = corner === "start" ? { r: box.bottom, c: box.right } : { r: box.top, c: box.left };
      handleDrag = { tab: sel.tab, fixed, pointerId: event.pointerId, moved: false };
    },
    onFilterButton: (c, el) => {
      editor.settle?.();
      if (editor.editing && editor.canPick()) return editor.focus();
      if (!ensureReadyForAction()) return;
      if (!filters.get(tabs.activeId)) return;
      openFilterAt(tabs.activeId, c, el);
    },
    onChartResizeDown(e) {
      const chart = activeChart();
      if (!chart || editor.editing) return;
      const origBox = boundRange(chart.rule.range, CHART_BIG);
      const rect =
        peekCharts()?.chartRect(chart.table, chart.rule.id) ??
        (origBox ? tableMetrics(chart.table, windowOf(chart.table)).spanBox(origBox.r0, origBox.c0, origBox.r1, origBox.c1) : null);
      if (!origBox || !rect) return;
      const startX = e.clientX ?? 0;
      const startY = e.clientY ?? 0;
      chartDrag = {
        mode: "resize",
        tab: chart.tab,
        id: chart.rule.id,
        origBox,
        box: { ...origBox },
        startX,
        startY,
        offsetX: startX - (rect.left + rect.width),
        offsetY: startY - (rect.top + rect.height),
        moved: false,
      };
      editor.focus({ preventScroll: true });
    },
  });

  const chrome = createChrome({
    scrollBox,
    commands: externalCommands,
    onSelectTab: handleTabClick,
    onTabMenu: (tabId, btn, x, y) => runLazy(cmdTool, (b) => ensureCommands(b).actions.openTabContextMenu(tabId, btn, x, y)),
    onRenameTab(tabId, nextName) {
      const ready = cmdTool.peek();
      if (ready) return ensureCommands(ready).actions.handleRenameTab(tabId, nextName);
      runLazy(cmdTool, (b) => ensureCommands(b).actions.handleRenameTab(tabId, nextName));
      return true;
    },
    onRenameTabStart(tabId) {
      if (!ensureReadyForAction()) return;
      runLazy(cmdTool, (b) => ensureCommands(b).actions.startTabRename(tabId));
    },
    onGoTo(raw) {
      if (!ensureReadyForAction()) return false;
      runLazy(findTool, (f) => ensureFind(f).goTo(raw));
      return true;
    },
    onCancelGoTo: () => editor.focus(),
    onCommit() {
      if (!editor.editing) return;
      const res = commitEdit();
      if (res.ok) {
        applySelection(moveSelection(viewOf(sel.tab), sel, { dr: 1, dc: 0, extend: false }));
        revealFocus();
        editor.focus();
      }
    },
    onCancel: () => editor.cancel(),
    onMoreMenu: (items, btn) => runLazy(cmdTool, (b) => ensureCommands(b).menu.open({ anchor: btn, label: "More tools", items })),
    onQuickKey: (text) => editor.insertText(text),
    onLayoutChange: () => {
      if (disposed || !sel) return;
      if (liveMenu?.isOpen) liveMenu.close({ restoreFocus: false });
      syncUI();
    },
  });

  const editor = createEditor({
    workbook,
    editorSlot: overlays.editorSlot,
    formulaInput: chrome.formulaInput,
    getActiveCell: activeCellInfo,
    getView: viewOf,
    getTabs: () => tabs.list(),
    getShownTab: () => tabs.activeId,
    showTab: (id) => tabs.show(id),
    onStateChange: () => syncUI(),
    onCaretChange: () => fnHelp?.update(),
    onNotice: (msg) => chrome.showNotice(msg),
    refuseValue: ({ tab, row, col }, text) => {
      const table = tabTable(workbook, tab);
      const r = table ? rowIndex(table, row) : -1;
      const c = table ? colIndex(table, col) : -1;
      if (r === -1 || c === -1) return null;
      const rule = inputRuleAt(window.Sap.grid(table), r, c);
      return rule?.kind === "list" && !allowedInList(table, rule, text) ? "Pick one of the dropdown's items." : null;
    },
    onClearNotice: () => chrome.hideNotice(),
  });

  function syncToolbar(selView) {
    const selGrid = gridOf(tabTable(workbook, sel.tab));
    const canEdit = !editor.editing && Boolean(selGrid);
    if (editor.editing && liveMenu?.isOpen) liveMenu.close({ restoreFocus: false });

    const info = activeCellInfo();
    const st = quickToolbarStyle(info?.cell);
    chrome.setButtonPressed("bold", st.bold);
    chrome.setButtonPressed("italic", st.italic);
    chrome.setButtonPressed("strikethrough", st.strikethrough);
    chrome.setButtonPressed("wrap", st.wrap);
    chrome.setButtonText("number-format", st.formatLabel);

    const mState = canEdit ? quickMergeState(selGrid, selView, sel) : { canAct: false, isSingleMerge: false };
    chrome.setButtonText(
      "merge",
      mState.isSingleMerge ? "Unmerge" : "Merge",
      mState.isSingleMerge ? "Unmerge cells" : "Merge cells",
    );

    chrome.registerCommand("undo", canEdit && canUndo(sel.tab) ? () => replayStep("undo") : null);
    chrome.registerCommand("redo", canEdit && canRedo(sel.tab) ? () => replayStep("redo") : null);
    for (const id of LAZY_TOOLBAR_IDS) {
      const ok = canEdit && (id !== "merge" || mState.canAct);
      chrome.registerCommand(id, ok ? () => runLazy(cmdTool, (b) => ensureCommands(b).actions.runToolbarCommand(id)) : null);
    }

    const extFind = externalCommands.find;
    chrome.registerCommand(
      "find",
      !editor.editing
        ? typeof extFind === "function"
          ? extFind
          : canEdit
            ? () => ensureReadyForAction() && runLazy(findTool, (f) => ensureFind(f).open({ replace: true }))
            : null
        : null,
    );
    const extImport = externalCommands.import;
    chrome.registerCommand(
      "import",
      !editor.editing
        ? typeof extImport === "function"
          ? extImport
          : canEdit
            ? () => ensureReadyForAction() && sel && runLazy(csvTool, (c) => c.openCsvMenu(sharedCtx(), ensureCommands(cmdTool.peek()).menu))
            : null
        : null,
    );

    const extFilter = externalCommands.filter;
    chrome.registerCommand(
      "filter",
      !editor.editing
        ? typeof extFilter === "function"
          ? extFilter
          : canEdit
            ? () => ensureReadyForAction() && runLazy(filterTool, (f) => f.toggleFilter(sharedCtx()))
            : null
        : null,
    );
    chrome.setButtonPressed("filter", filters.has(sel.tab));

    const hasSel = Boolean(sel && selectionBox(selView, sel));
    const extCheckbox = externalCommands.checkbox;
    chrome.registerCommand(
      "checkbox",
      !editor.editing
        ? typeof extCheckbox === "function"
          ? extCheckbox
          : canEdit && hasSel
            ? () => ensureReadyForAction() && runLazy(rulesTool, (r) => r.toggleCheckboxRule(sharedCtx()))
            : null
        : null,
    );
    const extDropdown = externalCommands.dropdown;
    chrome.registerCommand(
      "dropdown",
      !editor.editing
        ? typeof extDropdown === "function"
          ? extDropdown
          : canEdit && hasSel
            ? () => openRulePanel("dropdown", chrome.getButton("dropdown"))
            : null
        : null,
    );
    const extCond = externalCommands["conditional-format"];
    chrome.registerCommand(
      "conditional-format",
      !editor.editing
        ? typeof extCond === "function"
          ? extCond
          : canEdit
            ? () => openRulePanel("conditional", chrome.getButton("conditional-format"))
            : null
        : null,
    );

    const extChart = externalCommands.chart;
    chrome.registerCommand(
      "chart",
      !editor.editing
        ? typeof extChart === "function"
          ? extChart
          : canEdit && hasSel
            ? () => runInsertChart()
            : null
        : null,
    );
    const extSort = externalCommands.sort;
    chrome.registerCommand("sort", !editor.editing && typeof extSort === "function" ? extSort : null);
  }

  function syncUI({ announce = false, enteredTab = false } = {}) {
    if (disposed || !sel) return;
    const shownId = tabs.activeId;
    const table = tabTable(workbook, shownId);
    const view = viewOf(shownId);
    const selView = shownId === sel.tab ? view : viewOf(sel.tab);
    const box = shownId === sel.tab ? selectionBox(view, sel) : null;

    chrome.setEditing(editor.editing);
    syncToolbar(selView);
    chrome.setCoord(selectionArea(selView, sel));
    chrome.setStatus(chrome.docked() && !editor.editing ? readingStatus(selView) : statusCache);
    chrome.renderTabs(tabs.list(), shownId);
    if (!editor.editing) editor.refreshReady();

    const speech = cellSpeech(enteredTab);
    editor.setAriaLabel(speech);
    if (announce) overlays.announce(speech);

    const activeInfo = shownId === sel.tab && !editor.editing ? activeCellInfo() : null;
    const linkUrl = activeInfo ? extractCellLink(activeInfo.cell, win.Sap?.grid?.tokenize) : null;

    const filterButtons = [];
    const filterSpec = filters.get(shownId);
    if (filterSpec) {
      const headerRow = view.rows.indexOf(filterSpec.header);
      if (headerRow !== -1) {
        for (const id of filterSpec.cols) {
          const c = view.cols.indexOf(id);
          if (c !== -1) filterButtons.push({ r: headerRow, c, active: filterSpec.criteria.has(id) });
        }
      }
    }

    let chartOverlayBox = null;
    if (chartDrag?.tab === shownId && chartDrag.moved && chartDrag.box && table) {
      const b = chartDrag.box;
      chartOverlayBox = tableMetrics(table, windowOf(table)).spanBox(b.r0, b.c0, b.r1, b.c1);
    } else {
      const chart = activeChart();
      if (chart && table) {
        chartOverlayBox = peekCharts()?.chartRect(table, chart.rule.id) ?? null;
        if (!chartOverlayBox) {
          const b = boundRange(chart.rule.range, CHART_BIG);
          if (b) chartOverlayBox = tableMetrics(table, windowOf(table)).spanBox(b.r0, b.c0, b.r1, b.c1);
        }
      }
    }

    overlays.render({
      table,
      win: windowOf(table),
      view,
      box,
      whole: shownId === sel.tab ? (sel.whole ?? null) : null,
      editing: editor.editing && shownId === sel.tab,
      refs: editor.refs(),
      conflicts: resolveConflictBoxes(),
      shownTab: shownId,
      fillPreviewBox: fillDrag?.tab === shownId ? fillDrag.targetBox : null,
      linkUrl,
      touch: isTouch(),
      filterButtons,
      chartBox: chartOverlayBox,
    });
    findCtrl?.syncTab();
    syncFunctionHelp();
  }

  function revealFocus() {
    if (!sel) return;
    const table = tabTable(workbook, sel.tab);
    if (!table) return;
    const r = rowIndex(table, sel.focus.row);
    const c = colIndex(table, sel.focus.col);
    if (r < (table.tHead?.rows.length ?? 0) || c < 0) return;
    const cb = overlays.cellBox(table, windowOf(table), r, c);
    if (!cb) return;
    const scroller = scrollerOf(workbook);
    if (scroller && scroller.clientHeight <= 0) return;
    const sb = scroller
      ? scroller.getBoundingClientRect()
      : { top: 0, left: 0, bottom: win.innerHeight ?? 0, right: win.innerWidth ?? 0 };
    const topEdge = sb.top + (table.tHead?.offsetHeight ?? 0) + 24;
    const leftEdge = sb.left + 42;
    let dy = 0;
    let dx = 0;
    if (cb.top < topEdge) dy = cb.top - topEdge;
    else if (cb.top + cb.height > sb.bottom) dy = cb.top + cb.height - sb.bottom;
    if (cb.left < leftEdge) dx = cb.left - leftEdge;
    else if (cb.left + cb.width > sb.right) dx = cb.left + cb.width - sb.right;
    if (!dy && !dx) return;
    if (scroller) {
      scroller.scrollTop += dy;
      scroller.scrollLeft += dx;
    } else {
      win.scrollBy?.(dx, dy);
    }
  }

  function applyMove(nextSel, { announce = true, enteredTab = false } = {}) {
    applySelection(nextSel, { announce, enteredTab });
    revealFocus();
  }

  function applySelection(nextSel, { announce = true, enteredTab = false } = {}) {
    selectedChart = null;
    chartDrag = null;
    sel = nextSel;
    if (!editor.editing && tabs.activeId !== sel.tab) tabs.show(sel.tab);
    writeSelection(storage, path, sel);
    snapshotViews();
    refreshStatus();
    syncUI({ announce, enteredTab });
  }

  function initSelection() {
    const list = tabs.list();
    const firstId = list[0]?.id ?? "";
    const stored = readSelection(storage, path);
    const chosenTab = stored && list.some((t) => t.id === stored.tab) ? stored.tab : firstId;
    const view = viewOf(chosenTab);
    sel = stored && stored.tab === chosenTab
      ? clipSelection(view, stored)
      : createSelection(view, chosenTab);
    tabs.show(sel.tab);
    writeSelection(storage, path, sel);
    snapshotViews();
    refreshStatus();
    syncUI({ announce: true, enteredTab: true });
  }

  function handleTabClick(tabId) {
    editor.settle();
    const targetTab = tabs.list().find((t) => t.id === tabId);
    if (!targetTab) return;

    if (editor.editing && editor.canPick()) {
      if (tabId === tabs.activeId) return editor.focus();
      tabs.show(tabId);
      editor.pickCell({ tab: tabId, tabName: targetTab.name, r: 0, c: 0, extend: false });
      refreshStatus();
      syncUI({ announce: false });
      return;
    }

    if (editor.editing && !commitEdit().ok) return;
    if (tabId === sel.tab && tabs.activeId === tabId) return editor.focus();

    closeTransient();
    const view = viewOf(tabId);
    tabs.show(tabId);
    applySelection(createSelection(view, tabId), { announce: true, enteredTab: true });
    editor.focus();
  }

  function pageRowsCount() {
    const scroller = scrollerOf(workbook);
    const h = scroller ? scroller.clientHeight : win.innerHeight;
    return Math.max(1, Math.floor((h || 520) / ROW_ESTIMATE));
  }

  const isTicked = (value) => value === true || (typeof value === "string" && value.toUpperCase() === "TRUE");

  // A cell a box may cover: TRUE, FALSE or blank, as the rule paints a box. A heading or any
  // other text in the range keeps showing as itself and is never ticked.
  const mayTick = (value) => {
    if (value === null || value === undefined || value === "") return true;
    const flag = typeof value === "string" ? value.toUpperCase() : value;
    return flag === true || flag === false || flag === "TRUE" || flag === "FALSE";
  };

  // The checkbox cells a box covers, in reading order, each with whether it is ticked. Rows the
  // viewer filtered out or hid, hidden columns and a merge's covered cells are out of sight, so
  // Space leaves them alone, as a Delete does.
  function checkboxCells(table, grid, view, box, tab) {
    // The rules are read once: Space runs this over the whole selection on every press.
    const inputs = grid.rules()
      .filter((rule) => rule.kind === "check" || rule.kind === "list")
      .map((rule) => ({ kind: rule.kind, at: boundRange(rule.range, grid.size) }))
      .filter(({ at }) => at && at.r0 <= box.bottom && at.r1 >= box.top && at.c0 <= box.right && at.c1 >= box.left);
    if (!inputs.some(({ kind }) => kind === "check")) return [];
    const kindAt = (r, c) => inputs.find(({ at }) => r >= at.r0 && r <= at.r1 && c >= at.c0 && c <= at.c1)?.kind;
    const filtered = filteredRows(table);
    const cells = [];
    for (let r = box.top; r <= box.bottom; r++) {
      if (filtered.has(view.rows[r]) || view.rowHidden?.(r)) continue;
      for (let c = box.left; c <= box.right; c++) {
        if (view.colHidden?.(c) || kindAt(r, c) !== "check") continue;
        if (grid.cell(a1(r, c))?.covered) continue;
        const value = grid.get(a1(r, c));
        if (!mayTick(value)) continue;
        cells.push({ tab, row: view.rows[r], col: view.cols[c], on: isTicked(value) });
      }
    }
    return cells;
  }

  // Every one of those cells turns on, or all turn off when every one is already on: one grid
  // write of real booleans, so one undo step and a formula counts them.
  function writeCheckboxes(grid, cells) {
    const next = !cells.every((cell) => cell.on);
    const writes = cells.map((cell) => [cell.row, cell.col, next]);
    if (!writeGrid(() => grid.dispatch({ v: 1, t: "set", sheet: cells[0].tab, cells: writes }))) return false;
    refreshStatus();
    syncUI({ announce: true });
    return true;
  }

  // A plain click on a checkbox cell ticks it.
  function toggleCheckboxAt(hit) {
    const table = tabTable(workbook, hit.tab);
    const grid = gridOf(table);
    if (!grid || inputRuleAt(grid, hit.r, hit.c)?.kind !== "check") return;
    const cells = checkboxCells(table, grid, viewOf(hit.tab), { top: hit.r, left: hit.c, bottom: hit.r, right: hit.c }, hit.tab);
    if (cells.length > 0) writeCheckboxes(grid, cells);
  }

  // Space over a selection that covers a checkbox cell ticks them all, or clears them all when
  // they all are ticked. False when no selected cell is a checkbox, so Space types as it always has.
  function spaceTogglesCheckboxes() {
    const table = tabTable(workbook, sel.tab);
    const grid = gridOf(table);
    const view = viewOf(sel.tab);
    const box = selectionBox(view, sel);
    if (!grid || !box) return false;
    const cells = checkboxCells(table, grid, view, box, sel.tab);
    return cells.length > 0 && writeCheckboxes(grid, cells);
  }

  // Rows this viewer filtered out are out of sight, so a Delete or a cut leaves them alone.
  function clearSelectedCells() {
    const table = tabTable(workbook, sel.tab);
    const grid = gridOf(table);
    if (!grid) return;
    const view = viewOf(sel.tab);
    if (view.rows.length === 0 || view.cols.length === 0) return;
    const box = selectionBox(view, sel);
    if (!box) return;
    const filtered = filteredRows(table);
    const rows = [];
    for (let r = box.top; r <= box.bottom; r++) if (!filtered.has(view.rows[r])) rows.push(r);
    const wholeRows = sel.whole === "rows";
    const cells = [];
    for (const r of rows) {
      const c0 = wholeRows ? 0 : box.left;
      const c1 = wholeRows ? view.cols.length - 1 : box.right;
      for (let c = c0; c <= c1; c++) {
        const cell = grid.cell(a1(r, c));
        if (cell && !cell.covered && cell.content !== null && cell.content !== undefined) cells.push([view.rows[r], view.cols[c], null]);
      }
    }
    const area = commandArea(view, sel);
    const unchanged = rows.length === box.bottom - box.top + 1;
    if (!writeGrid(() => (unchanged ? grid.clear(area) : cells.length > 0 && grid.dispatch({ v: 1, t: "set", sheet: sel.tab, cells })))) return;
    refreshStatus();
    syncUI({ announce: true });
  }

  function commitEdit() {
    return withMine(() => editor.commit());
  }

  function boxSelection(view, current, coords) {
    const top = Math.min(...coords.map((p) => p.r));
    const left = Math.min(...coords.map((p) => p.c));
    const bottom = Math.max(...coords.map((p) => p.r));
    const right = Math.max(...coords.map((p) => p.c));
    const own = ownerOf(view, top, left);
    return {
      tab: current.tab,
      anchor: { row: view.rows[own.r], col: view.cols[own.c] },
      focus: { row: view.rows[bottom], col: view.cols[right] },
    };
  }

  function replayStep(kind) {
    const live = room();
    if (live) {
      const res = kind === "undo" ? live.undo(sel.tab) : live.redo(sel.tab);
      if (res.ok) {
        if (!res.skipped?.length) chrome.hideNotice();
        const coords = [];
        for (const target of res.targets ?? []) {
          const box = resolveTarget(target);
          if (!box || box.tab !== sel.tab) continue;
          coords.push({ r: box.r0, c: box.c0 }, { r: box.r1, c: box.c1 });
        }
        if (coords.length > 0) applyMove(boxSelection(viewOf(sel.tab), sel, coords));
        else {
          refreshStatus();
          syncUI({ announce: true });
        }
      } else if (res.why !== "changed") syncUI();
      return;
    }
    let res;
    try {
      res = kind === "undo" ? history.undo(sel.tab) : history.redo(sel.tab);
    } catch (error) {
      if (error?.name !== "GridRefused") throw error;
      chrome.showNotice(error.message || "That step could not be applied.");
      syncUI();
      return;
    }
    if (res.why === "changed") {
      chrome.showNotice(kind === "undo" ? CHANGED_MSG : REDONE_MSG);
      syncUI();
      return;
    }
    if (!res.ok) return;
    chrome.hideNotice();
    const table = tabTable(workbook, sel.tab);
    const coords = [];
    for (const { row, col } of res.cells) {
      const r = table ? rowIndex(table, row) : -1;
      const c = table ? colIndex(table, col) : -1;
      if (r >= 0 && c >= 0) coords.push({ r, c });
    }
    if (coords.length > 0) applyMove(boxSelection(viewOf(sel.tab), sel, coords));
    else {
      refreshStatus();
      syncUI({ announce: true });
    }
  }

  function copySelection(cut = false) {
    const grid = gridOf(tabTable(workbook, sel.tab));
    if (!grid) return null;
    const view = viewOf(sel.tab);
    if (view.rows.length === 0 || view.cols.length === 0) return null;
    const box = selectionBox(view, sel);
    if (!box) return null;
    const filtered = filteredRows(tabTable(workbook, sel.tab));
    const skipRows = new Set();
    for (let r = box.top; r <= box.bottom; r++) if (filtered.has(view.rows[r])) skipRows.add(r);
    return { grid, box, skipRows, parts: copyParts(grid, box, a1, { cut, skipRows }) };
  }

  async function runClipboard(kind) {
    const clip = win.navigator?.clipboard;
    const refuse = () =>
      chrome.showNotice(
        kind === "paste"
          ? "This browser didn't let the sheet read the clipboard. Tap the cell twice and paste into the edit bar."
          : "This browser didn't let the sheet use the clipboard.",
      );
    if (!clip) return refuse();
    if (kind === "paste") {
      let html = "";
      let text = "";
      try {
        if (typeof clip.read === "function") {
          for (const item of await clip.read()) {
            if (!html && item.types.includes("text/html")) html = await (await item.getType("text/html")).text();
            if (!text && item.types.includes("text/plain")) text = await (await item.getType("text/plain")).text();
          }
        } else {
          text = await clip.readText();
        }
      } catch {
        return refuse();
      }
      if (!disposed) applyPasteData({ html, text });
      return;
    }
    const before = sel;
    const copied = copySelection(kind === "cut");
    if (!copied) return;
    try {
      const Item = win.ClipboardItem;
      if (typeof clip.write === "function" && typeof Item === "function") {
        await clip.write([
          new Item({
            "text/plain": new win.Blob([copied.parts.text], { type: "text/plain" }),
            "text/html": new win.Blob([copied.parts.html], { type: "text/html" }),
          }),
        ]);
      } else {
        await clip.writeText(copied.parts.text);
      }
    } catch {
      return refuse();
    }
    if (kind === "cut" && !disposed && sel === before) clearSelectedCells();
  }

  function onCopy(event) {
    if (disposed || editor.editing || !sel || !event.clipboardData) return;
    if (activeChart()) {
      event.preventDefault();
      return;
    }
    const copied = copySelection();
    if (!copied) return;
    event.preventDefault();
    event.clipboardData.setData("text/plain", copied.parts.text);
    event.clipboardData.setData("text/html", copied.parts.html);
  }

  function onCut(event) {
    if (disposed || editor.editing || !sel || !event.clipboardData) return;
    if (activeChart()) {
      event.preventDefault();
      return;
    }
    const copied = copySelection(true);
    if (!copied) return;
    event.preventDefault();
    event.clipboardData.setData("text/plain", copied.parts.text);
    event.clipboardData.setData("text/html", copied.parts.html);
    clearSelectedCells();
  }

  function onBeforeClipboard(event) {
    if (!disposed && !editor.editing) event.preventDefault();
  }

  function applyPasteData({ html = "", text = "", valuesOnly = false, transpose = false, formatOnly = false } = {}) {
    if (disposed || editor.editing || !sel) return false;
    const grid = gridOf(tabTable(workbook, sel.tab));
    if (!grid) return false;
    const view = viewOf(sel.tab);
    if (view.rows.length === 0 || view.cols.length === 0) return false;
    const box = selectionBox(view, sel);
    if (!box) return false;
    const at = a1(box.top, box.left);
    const singleMerge =
      box.top === box.active.r &&
      box.left === box.active.c &&
      box.bottom === box.active.r + box.active.rows - 1 &&
      box.right === box.active.c + box.active.cols - 1;
    const [selRows, selCols] = singleMerge ? [1, 1] : [box.bottom - box.top + 1, box.right - box.left + 1];
    const parsed = pasteBlock(html, { r: box.top, c: box.left }, { valuesOnly, transpose, formatOnly });
    const uncovered = (write) =>
      write.map((line, i) => line.map((item, j) => (view.cell(box.top + i, box.left + j)?.covered ? undefined : item)));

    let rows = 0;
    let cols = 0;

    if (formatOnly) {
      if (parsed?.kind !== "ours") {
        chrome.showNotice("Format only works with cells copied from a sheet.");
        return false;
      }
      const { block } = parsed;
      const write = block.length === 1 && block[0].length === 1 ? fillBlock(block, selRows, selCols) : block;
      // Formats never grow the sheet: what falls past the last row or column is left off.
      rows = Math.min(write.length, view.rows.length - box.top);
      cols = Math.min(write[0]?.length ?? 0, view.cols.length - box.left);
      if (rows <= 0 || cols <= 0) return false;
      const ok = writeGrid(() =>
        history.group(sel.tab, () => {
          // One source cell formats the whole target in one Grid step; a cell at a time froze large ranges.
          if (block.length === 1 && block[0].length === 1) {
            const item = block[0][0];
            grid.format(`${at}:${a1(box.top + rows - 1, box.left + cols - 1)}`, { fmt: item.fmt ?? null, style: item.style });
            return;
          }
          for (let i = 0; i < rows; i++) {
            for (let j = 0; j < cols; j++) {
              if (view.cell(box.top + i, box.left + j)?.covered) continue;
              const item = write[i]?.[j];
              if (!item) continue;
              grid.format(a1(box.top + i, box.left + j), { fmt: item.fmt ?? null, style: item.style });
            }
          }
        }),
      );
      if (!ok) return false;
    } else if (parsed && (parsed.kind === "ours" || !text)) {
      const { kind, block, origin } = parsed;
      let write = block;
      if (
        kind === "ours" &&
        !valuesOnly &&
        block.length === 1 &&
        block[0].length === 1 &&
        block[0][0]?.content &&
        typeof block[0][0].content === "object" &&
        typeof block[0][0].content.f === "string"
      ) {
        const item = block[0][0];
        write = uncovered(fillFormula(item.content.f, item.fmt, box.top - origin.r, box.left - origin.c, selRows, selCols, item.style));
        rows = selRows;
        cols = selCols;
      } else if (block.length === 1 && block[0].length === 1) {
        write = uncovered(fillBlock(block, selRows, selCols));
        rows = selRows;
        cols = selCols;
      } else {
        rows = block.length;
        cols = block[0]?.length ?? 0;
      }
      const options =
        kind === "ours"
          ? valuesOnly
            ? { grow: true, detect: false, formats: false }
            : { grow: true }
          : { grow: true, formulas: !valuesOnly };
      if (!writeGrid(() => grid.setRange(at, write, options))) return false;
    } else if (text) {
      const flat = text.replace(/\r?\n$/, "");
      let lines;
      if (!/[\t\r\n]/.test(flat)) {
        lines = [[flat]];
      } else {
        try {
          lines = win.Sap?.grid?.csv?.parseDelimited(text, { delimiter: "\t" }) ?? [];
        } catch (error) {
          if (error?.name !== "DelimitedError") throw error;
          lines = [[flat]];
        }
      }
      const matrix = transpose ? transposeMatrix(lines, "") : lines;
      const one = matrix.length === 1 && (matrix[0]?.length ?? 0) === 1;
      const filled = fillBlock(matrix, selRows, selCols);
      rows = filled.length;
      cols = Math.max(0, ...filled.map((line) => line.length));
      if (!rows || !cols) return false;
      const options = { grow: true, formulas: !valuesOnly };
      let wrote;
      if (one) {
        wrote = writeGrid(() => grid.setRange(at, uncovered(filled), options));
      } else if (!transpose) {
        wrote = writeGrid(() => grid.importDelimited(text, at, { delimiter: "\t", formulas: !valuesOnly }));
      } else if (typeof win.Sap?.grid?.csv?.writeDelimited === "function") {
        const transposedText = win.Sap.grid.csv.writeDelimited(matrix, { delimiter: "\t" });
        wrote = writeGrid(() => grid.importDelimited(transposedText, at, { delimiter: "\t", formulas: !valuesOnly }));
      } else {
        wrote = writeGrid(() => grid.setRange(at, uncovered(filled), options));
      }
      if (!wrote) return false;
    } else {
      return false;
    }

    if (!rows || !cols) return false;
    const nextView = viewOf(sel.tab);
    if (nextView.rows.length === 0 || nextView.cols.length === 0) return false;
    const bottom = Math.min(box.top + rows - 1, nextView.rows.length - 1);
    const right = Math.min(box.left + cols - 1, nextView.cols.length - 1);
    applyMove({
      tab: sel.tab,
      anchor: { row: nextView.rows[box.top], col: nextView.cols[box.left] },
      focus: { row: nextView.rows[bottom], col: nextView.cols[right] },
    });
    return true;
  }

  function onPaste(event) {
    if (disposed || editor.editing || !sel) return;
    if (activeChart()) {
      event.preventDefault();
      return;
    }
    event.preventDefault();
    try {
      const html = event.clipboardData?.getData("text/html") ?? "";
      const text = event.clipboardData?.getData("text/plain") ?? "";
      applyPasteData({ html, text, valuesOnly: valuesNext });
    } finally {
      valuesNext = false;
    }
  }

  function runKey(event) {
    if (disposed || seenKeys.has(event)) return;
    seenKeys.add(event);

    if (ruleSurface?.handleKeyDown?.(event)) return;
    if (editor.editing && fnHelp?.handleKeyDown(event)) return;

    if (!editor.editing) {
      const plainKey = !event.ctrlKey && !event.metaKey && !event.altKey;
      if (chartDrag && event.key === "Escape" && plainKey) {
        event.preventDefault();
        chartDrag = null;
        syncUI({ announce: false });
        return;
      }
      const curChart = activeChart();
      if (curChart && plainKey) {
        if (event.key === "Escape" && !event.shiftKey) {
          event.preventDefault();
          selectedChart = null;
          syncUI({ announce: false });
          return;
        }
        if (event.key === "Enter" && !event.shiftKey) {
          event.preventDefault();
          openChartPanel(curChart.rule.id);
          return;
        }
        if (event.key === "Delete" || event.key === "Backspace") {
          event.preventDefault();
          deleteChartById(curChart.tab, curChart.rule.id);
          return;
        }
        if (event.key.length === 1) {
          event.preventDefault();
          return;
        }
      }
    }

    const intent = keyIntent(event, {
      platform: platform ?? win.navigator?.platform ?? "",
      editing: editor.editing,
      picking: editor.canPick({ keyboard: true }),
      composing: editor.composing,
    });
    if (intent?.type === "pasteValues") {
      valuesNext = true;
      return;
    }
    if (!["Shift", "Control", "Meta", "Alt"].includes(event.key) && intent?.type !== "pasteValues") valuesNext = false;
    if (!editor.editing && event.altKey && event.key === "ArrowDown" && sel) {
      const tabId = tabs.activeId;
      const table = tabTable(workbook, tabId);
      const grid = gridOf(table);
      const r = table ? rowIndex(table, sel.anchor.row) : -1;
      const c = table ? colIndex(table, sel.anchor.col) : -1;
      const spec = filters.get(tabId);
      const view = viewOf(tabId);
      const heading = spec ? view.rows.indexOf(spec.header) : -1;
      const fc = view.cols.indexOf(sel.anchor.col);
      if (spec && sel.tab === tabId && heading !== -1 && view.rows[heading] === sel.anchor.row && fc !== -1 && spec.cols.includes(view.cols[fc])) {
        event.preventDefault();
        const el = overlays.shadow?.querySelector?.(`[data-filter-col="${fc}"]`) ?? null;
        openFilterAt(tabId, fc, el ?? chrome.getButton("filter"));
        return;
      }
      if (grid && sel.tab === tabId && r !== -1 && c !== -1 && inputRuleAt(grid, r, c)?.kind === "list") {
        event.preventDefault();
        openPickListForCell(tabId, r, c);
        return;
      }
    }
    if (
      sel &&
      !editor.editing &&
      event.key === " " &&
      !event.shiftKey &&
      !event.altKey &&
      !event.ctrlKey &&
      !event.metaKey &&
      spaceTogglesCheckboxes()
    ) {
      event.preventDefault();
      return;
    }
    if (!intent) return;

    event.preventDefault();
    const view = viewOf(sel.tab);

    switch (intent.type) {
      case "move":
        applyMove(moveSelection(view, sel, intent));
        break;
      case "edge":
        applyMove(edgeSelection(view, sel, intent));
        break;
      case "rowEdge":
        applyMove(rowEdgeSelection(view, sel, intent));
        break;
      case "sheetCorner":
        applyMove(sheetCornerSelection(view, sel, intent));
        break;
      case "page":
        applyMove(pageSelection(view, sel, { ...intent, pageSize: pageRowsCount() }));
        break;
      case "selectAll":
        applyMove(selectAll(view, sel));
        break;
      case "selectCols":
        applySelection(selectCols(view, sel));
        break;
      case "selectRows":
        applySelection(selectRows(view, sel));
        break;
      case "tab": {
        const list = tabs.list();
        if (list.length < 2) break;
        const idx = list.findIndex((t) => t.id === sel.tab);
        handleTabClick(list[(idx + intent.delta + list.length) % list.length].id);
        break;
      }
      case "clear":
        clearSelectedCells();
        break;
      case "undo":
        replayStep("undo");
        break;
      case "redo":
        replayStep("redo");
        break;
      case "fillDown":
        triggerFillShortcut("down");
        break;
      case "fillRight":
        triggerFillShortcut("right");
        break;
      case "find":
        if (ensureReadyForAction()) runLazy(findTool, (f) => ensureFind(f).open({ replace: false }));
        break;
      case "findReplace":
        if (ensureReadyForAction()) runLazy(findTool, (f) => ensureFind(f).open({ replace: true }));
        break;
      case "shortcuts":
        if (ensureReadyForAction()) runLazy(shortcutsTool, (s) => s.showShortcutsDialog(sharedCtx()));
        break;
      case "goto":
        chrome.focusNameBox();
        break;
      case "bold":
      case "italic":
      case "underline":
      case "strikethrough":
        runLazy(cmdTool, (b) => runFormat((g, v) => b.applyStyleToggle(g, v, sel, history, intent.type)), { guardSel: true });
        break;
      case "align":
        runLazy(cmdTool, (b) => runFormat((g, v) => b.applyAlign(g, v, sel, intent.align)), { guardSel: true });
        break;
      case "numberFormat":
        runLazy(cmdTool, (b) => runFormat((g, v) => b.applyNumberFormat(g, v, sel, intent.fmt)), { guardSel: true });
        break;
      case "clearFormat":
        runLazy(cmdTool, (b) => runFormat((g, v) => b.applyClearFormatting(g, v, sel, history)), { guardSel: true });
        break;
      case "insertAxis":
        runLazy(cmdTool, (b) => ensureCommands(b).actions.handleInsertAxisShortcut());
        break;
      case "deleteAxis":
        runLazy(cmdTool, (b) => ensureCommands(b).actions.handleDeleteAxisShortcut());
        break;
      case "contextMenu": {
        const curChart = activeChart() ?? chartUnderActiveCell();
        if (curChart) {
          const rect = peekCharts()?.chartRect(curChart.table, curChart.rule.id);
          openChartMenuAt(curChart.rule.id, rect?.left ?? 0, (rect?.top ?? 0) + (rect?.height ?? 0));
          break;
        }
        runLazy(cmdTool, (b) => ensureCommands(b).actions.handleContextMenuShortcut());
        break;
      }
      case "edit":
        openEdit();
        break;
      case "cancel":
        editor.cancel();
        break;
      case "newline":
        editor.insertNewline();
        break;
      case "cycleRef":
        editor.cycleRef();
        break;
      case "pick":
        editor.pickMove(intent);
        break;
      case "commitAll":
        runLazy(fillTool, (f) => f.runCommitAll(sharedCtx()), { guardSel: true });
        break;
      case "commit": {
        const res = commitEdit();
        if (res.ok) {
          applyMove(moveSelection(viewOf(sel.tab), sel, { dr: intent.dr, dc: intent.dc, extend: false }));
          editor.focus();
        }
        break;
      }
    }
  }

  function onWindowKeyDown(event) {
    const target = event.composedPath?.()[0] ?? event.target;
    if (target !== editor.textarea && target !== chrome.formulaInput) return;
    runKey(event);
  }

  function locateCellEvent(event) {
    const el = event.target?.nodeType === 1 ? event.target : event.target?.parentElement;
    if (el) {
      const cellEl = el.closest?.("td, th");
      if (cellEl && workbook.contains(cellEl)) {
        const table = cellEl.closest("table[grid]");
        const section = table?.parentElement;
        const tr = cellEl.parentElement;
        if (table && section?.matches("section[item]") && tr?.localName === "tr") {
          const tab = section.getAttribute("data-id") ?? "";
          const r = tableRows(table).indexOf(tr);
          if (r !== -1) return { tab, r, c: columnOfCell(cellEl) };
        }
      }
    }
    const shownId = tabs.activeId;
    const table = tabTable(workbook, shownId);
    if (!table) return null;
    const hit = overlays.cellAtPoint(table, windowOf(table), event.clientX ?? 0, event.clientY ?? 0);
    return hit ? { tab: shownId, r: hit.r, c: hit.c } : null;
  }

  function actOnCellPointer(hit, { extend = false } = {}) {
    editor.settle();
    const tabInfo = tabs.list().find((t) => t.id === hit.tab);
    if (editor.editing) {
      if (editor.canPick()) {
        editor.pickCell({ tab: hit.tab, tabName: tabInfo?.name ?? "", r: hit.r, c: hit.c, extend });
        return;
      }
      if (!commitEdit().ok) return;
    }
    const view = viewOf(hit.tab);
    const entered = hit.tab !== sel.tab;
    if (entered) tabs.show(hit.tab);
    const baseSel = entered ? createSelection(view, hit.tab) : sel;
    applySelection(selectCell(view, baseSel, hit.r, hit.c, { extend: !entered && extend }), {
      announce: true,
      enteredTab: entered,
    });
    editor.focus();
  }

  function openCellLinkIfMod(event, hit) {
    if (editor.editing || event.shiftKey || event.altKey) return false;
    const modClick = isMac() ? event.metaKey && !event.ctrlKey : event.ctrlKey && !event.metaKey;
    if (!modClick) return false;
    const view = viewOf(hit.tab);
    const own = ownerOf(view, hit.r, hit.c);
    const link = extractCellLink(view.cell(own.r, own.c), win.Sap?.grid?.tokenize);
    if (!link) return false;
    win.open?.(link, "_blank", "noopener,noreferrer");
    return true;
  }

  let handledPointerDown = false;
  let dragging = false;

  const LONG_PRESS_MS = 420;
  const LONG_PRESS_SLOP = 10;
  let lastPointer = win.matchMedia?.("(pointer: coarse)")?.matches ? "touch" : "mouse";
  let longPress = null;
  let suppressTap = false;
  let handleDrag = null;
  let touchTapCell = null;
  // A press on a checkbox cell ticks only when its lift lands on the same cell: not a right
  // click, a long press, a drag or the second press of a double click.
  let pendingTick = null;
  const TICK_SLOP = 4;
  const isTouch = () => lastPointer === "touch" || lastPointer === "pen";

  function cancelLongPress() {
    if (!longPress) return;
    win.clearTimeout(longPress.timer);
    longPress = null;
  }

  const isOverlayOrChromeTarget = (event) => {
    const t = event?.target;
    if (!t || typeof t !== "object") return false;
    return Boolean(overlays.host?.contains?.(t) || chrome.topHost?.contains?.(t) || chrome.bottomHost?.contains?.(t));
  };

  // A press on the scroller's own scrollbar is not a press on a cell or a chart.
  const isScrollbarTarget = (event) => {
    if (event?.target !== scrollBox) return false;
    const rect = scrollBox.getBoundingClientRect?.();
    if (!rect || !(scrollBox.clientWidth > 0 || scrollBox.clientHeight > 0)) return false;
    const x = event.clientX ?? 0;
    const y = event.clientY ?? 0;
    return (
      x >= rect.left + (scrollBox.clientLeft ?? 0) + (scrollBox.clientWidth ?? 0) ||
      y >= rect.top + (scrollBox.clientTop ?? 0) + (scrollBox.clientHeight ?? 0)
    );
  };

  function onPointerDown(event) {
    if (isOverlayOrChromeTarget(event) || isScrollbarTarget(event)) return;
    const wasTouch = isTouch();
    lastPointer = event.pointerType || "mouse";
    suppressTap = false;
    cancelLongPress();
    chrome.setTouch(isTouch());
    if (wasTouch !== isTouch()) syncUI();
    if (!isTouch() || event.isPrimary === false) return;
    const x = event.clientX ?? 0;
    const y = event.clientY ?? 0;
    const chartHit = chartHitAt(x, y);
    if (chartHit) {
      longPress = {
        x,
        y,
        chartId: chartHit.rule.id,
        timer: win.setTimeout(() => {
          longPress = null;
          fireChartLongPress(chartHit.rule.id, x, y);
        }, LONG_PRESS_MS),
      };
      return;
    }
    const hit = locateCellEvent(event);
    if (!hit) return;
    longPress = {
      x,
      y,
      hit,
      timer: win.setTimeout(() => {
        longPress = null;
        fireLongPress(hit, x, y);
      }, LONG_PRESS_MS),
    };
  }

  function fireChartLongPress(chartId, x, y) {
    if (disposed || editor.editing) return;
    suppressTap = true;
    touchTapCell = null;
    pendingTick = null;
    openChartMenuAt(chartId, x, y);
  }

  function fireLongPress(hit, x, y) {
    if (disposed || editor.editing || !tabs.list().some((t) => t.id === hit.tab)) return;
    suppressTap = true;
    touchTapCell = null;
    pendingTick = null;
    openCellMenuAt(hit, x, y, { touch: true });
  }

  function onPointerMove(event) {
    if (longPress && Math.hypot((event.clientX ?? 0) - longPress.x, (event.clientY ?? 0) - longPress.y) > LONG_PRESS_SLOP) {
      cancelLongPress();
    }
    if (!handleDrag || event.pointerId !== handleDrag.pointerId) return;
    event.preventDefault?.();
    const table = tabTable(workbook, handleDrag.tab);
    const hit = overlays.cellAtPoint(table, windowOf(table), event.clientX ?? 0, event.clientY ?? 0);
    if (!hit) return;
    const view = viewOf(handleDrag.tab);
    const anchored = selectCell(view, { ...sel, tab: handleDrag.tab }, handleDrag.fixed.r, handleDrag.fixed.c, { extend: false });
    const next = selectCell(view, anchored, hit.r, hit.c, { extend: true });
    const same =
      next.focus.row === sel.focus.row && next.focus.col === sel.focus.col && next.anchor.row === sel.anchor.row && next.anchor.col === sel.anchor.col;
    if (!same) {
      handleDrag.moved = true;
      applySelection(next, { announce: false });
    }
  }

  // After a long press the finger's lift still sends mousedown and click; on iOS that mousedown
  // would close the menu the press just opened. The next finger down ends the swallowing.
  function swallowAfterLongPress(event) {
    if (!suppressTap) return;
    event.stopPropagation();
    event.preventDefault();
    if (event.type === "click") suppressTap = false;
  }

  function cellKey(hit) {
    const view = viewOf(hit.tab);
    const own = ownerOf(view, hit.r, hit.c);
    return `${hit.tab}\u0000${view.rows[own.r]}\u0000${view.cols[own.c]}`;
  }

  // On a phone the formula bar is hidden while reading, so the status line names the selection
  // and, for one cell, what it holds.
  function readingStatus(selView) {
    const area = selectionArea(selView, sel);
    if (statusCache) return `${area} · ${statusCache}`;
    const input = activeCellInfo()?.input ?? "";
    if (!input) return area;
    return `${area} · ${input.length > 60 ? `${input.slice(0, 59)}…` : input}`;
  }

  function onPointerEnd(event) {
    cancelLongPress();
    // The lift's mouse events follow at once; after that nothing is left to swallow (Android sends
    // none after its own contextmenu).
    if (suppressTap) win.setTimeout(() => {
      suppressTap = false;
    }, 1000);
    if (handleDrag && event.pointerId === handleDrag.pointerId) {
      const done = handleDrag;
      handleDrag = null;
      // A handle covers the corners of the cells around it: a tap on it that never dragged is a
      // tap on the cell under the finger.
      const table = tabTable(workbook, done.tab);
      const hit = done.moved ? null : overlays.cellAtPoint(table, windowOf(table), event.clientX ?? 0, event.clientY ?? 0);
      if (hit && !isOnlySelectedCell({ ...hit, tab: done.tab })) {
        actOnCellPointer({ ...hit, tab: done.tab });
        touchTapCell = cellKey({ ...hit, tab: done.tab });
        return;
      }
      syncUI({ announce: true });
    }
  }

  function isOnlySelectedCell(hit) {
    if (!sel || hit.tab !== sel.tab) return false;
    const view = viewOf(sel.tab);
    const box = selectionBox(view, sel);
    if (!box) return false;
    const own = ownerOf(view, hit.r, hit.c);
    return box.top === own.r && box.left === own.c && box.bottom === own.r + own.rows - 1 && box.right === own.c + own.cols - 1;
  }

  // On a phone the edit happens in the docked formula bar, which sits above the keyboard.
  function openEdit() {
    const phone = chrome.docked();
    if (phone) chrome.setEditing(true);
    editor.openWithInput(phone ? chrome.formulaInput : editor.textarea);
    if (!editor.editing) chrome.setEditing(false);
  }

  function dropdownChevronCell(event, hit) {
    if (event.shiftKey || event.altKey || event.ctrlKey || event.metaKey) return null;
    const table = tabTable(workbook, hit.tab);
    const grid = gridOf(table);
    if (!table || !grid) return null;
    const view = viewOf(hit.tab);
    const own = ownerOf(view, hit.r, hit.c);
    if (inputRuleAt(grid, own.r, own.c)?.kind !== "list") return null;
    const x = event.clientX;
    if (typeof x !== "number") return null;
    const td = (event.target?.nodeType === 1 ? event.target : event.target?.parentElement)?.closest?.("td, th");
    const tdRect = td && table.contains(td) ? td.getBoundingClientRect() : null;
    let left = 0;
    let right = 0;
    if (tdRect && tdRect.width > 0) {
      left = tdRect.left;
      right = tdRect.right;
    } else {
      const tl = overlays.cellBox(table, windowOf(table), own.r, own.c);
      const br = overlays.cellBox(table, windowOf(table), own.r + own.rows - 1, own.c + own.cols - 1);
      if (!tl || !br) return null;
      left = tl.left;
      right = br.left + br.width;
    }
    if (right <= left || x < Math.max(left, right - 20) || x > right) return null;
    return { tab: hit.tab, r: own.r, c: own.c };
  }

  function onMouseDown(event) {
    if (isOverlayOrChromeTarget(event) || isScrollbarTarget(event)) return;
    pendingTick = null;
    if (event.button !== undefined && event.button !== 0) return;
    if (suppressTap) {
      event.preventDefault();
      return;
    }
    const plain = !event.shiftKey && !event.ctrlKey && !event.metaKey && !event.altKey;
    if (plain && !(editor.editing && editor.canPick())) {
      const startX = event.clientX ?? 0;
      const startY = event.clientY ?? 0;
      const chartHit = chartHitAt(startX, startY);
      if (chartHit) {
        editor.settle();
        if (editor.editing && !commitEdit().ok) return;
        chrome.commitRename();
        event.preventDefault();
        handledPointerDown = true;
        dragging = false;
        selectedChart = { tab: tabs.activeId, id: chartHit.rule.id };
        const origBox = boundRange(chartHit.rule.range, CHART_BIG);
        chartDrag = origBox && !isTouch()
          ? {
              mode: "move",
              tab: tabs.activeId,
              id: chartHit.rule.id,
              origBox,
              box: { ...origBox },
              startX,
              startY,
              offsetX: startX - chartHit.rect.left,
              offsetY: startY - chartHit.rect.top,
              moved: false,
            }
          : null;
        syncUI({ announce: false });
        editor.focus({ preventScroll: true });
        return;
      }
    }
    const hit = locateCellEvent(event);
    if (!hit) return;
    if (!event.shiftKey && !event.ctrlKey && !event.metaKey && !event.altKey && event.detail === 1) {
      pendingTick = { tab: hit.tab, r: hit.r, c: hit.c, x: event.clientX ?? 0, y: event.clientY ?? 0 };
    }
    chrome.commitRename();
    event.preventDefault();
    handledPointerDown = true;
    const touch = isTouch();
    // A tap edits only when the touch tap before it selected this same cell: not the cell
    // selected at load, and not the tap that closed a long-press menu.
    const tapOnSelected =
      touch && !editor.editing && !event.shiftKey && isOnlySelectedCell(hit) && touchTapCell === cellKey(hit);
    const wasPicking = editor.editing && editor.canPick();
    actOnCellPointer(hit, { extend: event.shiftKey });
    if (!wasPicking && openCellLinkIfMod(event, hit)) {
      dragging = false;
      pendingTick = null;
      return;
    }
    const chevron = !wasPicking ? dropdownChevronCell(event, hit) : null;
    if (chevron) {
      dragging = false;
      touchTapCell = null;
      pendingTick = null;
      openPickListForCell(chevron.tab, chevron.r, chevron.c);
      return;
    }
    dismissedPickKey = "";
    if (touch) {
      dragging = false;
      touchTapCell = tapOnSelected ? null : cellKey(hit);
      if (tapOnSelected) {
        pendingTick = null;
        openEdit();
      }
      return;
    }
    dragging = true;
  }

  function onMouseMove(event) {
    if (chartDrag) {
      const cx = event.clientX ?? 0;
      const cy = event.clientY ?? 0;
      if (!chartDrag.moved && Math.hypot(cx - chartDrag.startX, cy - chartDrag.startY) < CHART_DRAG_SLOP) return;
      const table = tabTable(workbook, chartDrag.tab);
      if (!table) return;
      chartDrag.moved = true;
      const { origBox } = chartDrag;
      if (chartDrag.mode === "move") {
        const colSpan = origBox.c1 - origBox.c0;
        const rowSpan = origBox.r1 - origBox.r0;
        const { row, col } = nearestGridLine(table, windowOf(table), cx - chartDrag.offsetX, cy - chartDrag.offsetY);
        const nextC0 = Math.min(col, MAX_CHART_COL - colSpan);
        const nextR0 = Math.min(row, MAX_CHART_ROW - rowSpan);
        chartDrag.box = { r0: nextR0, c0: nextC0, r1: nextR0 + rowSpan, c1: nextC0 + colSpan };
      } else {
        const { row, col } = nearestGridLine(table, windowOf(table), cx - chartDrag.offsetX, cy - chartDrag.offsetY);
        const nextC1 = Math.min(MAX_CHART_COL, Math.max(origBox.c0 + 1, col - 1));
        const nextR1 = Math.min(MAX_CHART_ROW, Math.max(origBox.r0 + 3, row - 1));
        chartDrag.box = { r0: origBox.r0, c0: origBox.c0, r1: nextR1, c1: nextC1 };
      }
      syncUI({ announce: false });
      return;
    }
    if (fillDrag) {
      const hit = locateCellEvent(event);
      if (!hit || hit.tab !== fillDrag.tab) return;
      fillDrag.lastHit = hit;
      fillDrag.lastDx = fillDrag.hasCoords ? (event.clientX ?? 0) - fillDrag.startX : 0;
      fillDrag.lastDy = fillDrag.hasCoords ? (event.clientY ?? 0) - fillDrag.startY : 0;
      const fillMod = fillTool.peek();
      if (fillMod) {
        fillDrag.targetBox = fillMod.dragFillTarget(fillDrag.sourceBox, hit, {
          dx: fillDrag.lastDx,
          dy: fillDrag.lastDy,
          view: viewOf(hit.tab),
        });
        syncUI({ announce: false });
      }
      return;
    }
    if (!dragging) return;
    const hit = locateCellEvent(event);
    if (!hit) return;
    if (editor.editing && editor.canPick()) {
      const tabInfo = tabs.list().find((t) => t.id === hit.tab);
      editor.pickCell({ tab: hit.tab, tabName: tabInfo?.name ?? "", r: hit.r, c: hit.c, extend: true });
      return;
    }
    if (!editor.editing && hit.tab === sel.tab) {
      const nextSel = selectCell(viewOf(sel.tab), sel, hit.r, hit.c, { extend: true });
      if (nextSel.focus.row !== sel.focus.row || nextSel.focus.col !== sel.focus.col || nextSel.whole !== sel.whole) {
        applySelection(nextSel, { announce: false });
      }
    }
  }

  function onMouseUp(event) {
    const tick = pendingTick;
    pendingTick = null;
    if (chartDrag) {
      const done = chartDrag;
      chartDrag = null;
      dragging = false;
      if (done.moved && done.box) {
        const table = tabTable(workbook, done.tab);
        const grid = gridOf(table);
        const rule = grid?.rules?.().find((r) => r.kind === "chart" && r.id === done.id) ?? null;
        const nextRange =
          done.box.r0 === done.box.r1 && done.box.c0 === done.box.c1
            ? a1(done.box.r0, done.box.c0)
            : `${a1(done.box.r0, done.box.c0)}:${a1(done.box.r1, done.box.c1)}`;
        if (rule && nextRange !== rule.range && writeGrid(() => grid.setRules({ set: [{ ...rule, range: nextRange }] }))) {
          refreshStatus();
        }
        syncUI({ announce: true });
        editor.focus({ preventScroll: true });
      }
      return;
    }
    if (fillDrag) {
      const done = fillDrag;
      fillDrag = null;
      if (done.lastHit) {
        runLazy(fillTool, (f) => {
          const target =
            done.targetBox ??
            f.dragFillTarget(done.sourceBox, done.lastHit, {
              dx: done.lastDx,
              dy: done.lastDy,
              view: viewOf(done.tab),
            });
          if (target) f.applyFillAction(sharedCtx(), done.sourceBox, target);
          else syncUI({ announce: false });
          editor.focus();
        }, { guardSel: true });
      } else {
        syncUI({ announce: false });
      }
    }
    dragging = false;
    if (!tick || suppressTap || !event) return;
    if (Math.hypot((event.clientX ?? 0) - tick.x, (event.clientY ?? 0) - tick.y) >= TICK_SLOP) return;
    const hit = locateCellEvent(event);
    if (!hit || hit.tab !== tick.tab || hit.r !== tick.r || hit.c !== tick.c) return;
    toggleCheckboxAt(hit);
  }

  function onClick(event) {
    if (isOverlayOrChromeTarget(event) || isScrollbarTarget(event)) return;
    if (suppressTap) return;
    if (handledPointerDown) {
      handledPointerDown = false;
      return;
    }
    const plain = !event.shiftKey && !event.ctrlKey && !event.metaKey && !event.altKey;
    if (plain && !(editor.editing && editor.canPick())) {
      const chartHit = chartHitAt(event.clientX ?? 0, event.clientY ?? 0);
      if (chartHit) {
        editor.settle();
        if (editor.editing && !commitEdit().ok) return;
        selectedChart = { tab: tabs.activeId, id: chartHit.rule.id };
        syncUI({ announce: false });
        editor.focus({ preventScroll: true });
        return;
      }
    }
    const hit = locateCellEvent(event);
    if (!hit) return;
    const wasPicking = editor.editing && editor.canPick();
    actOnCellPointer(hit, { extend: event.shiftKey });
    if (!wasPicking && !openCellLinkIfMod(event, hit)) {
      const chevron = dropdownChevronCell(event, hit);
      if (chevron) openPickListForCell(chevron.tab, chevron.r, chevron.c);
    }
  }

  function onDblClick(event) {
    if (isOverlayOrChromeTarget(event) || isScrollbarTarget(event)) return;
    if (editor.editing && editor.canPick()) return;
    const chartHit = chartHitAt(event.clientX ?? 0, event.clientY ?? 0);
    if (chartHit) {
      if (!ensureReadyForAction()) return;
      event.preventDefault();
      chartDrag = null;
      selectedChart = { tab: tabs.activeId, id: chartHit.rule.id };
      syncUI({ announce: false });
      openChartPanel(chartHit.rule.id, chartHit.rect);
      return;
    }
    const hit = locateCellEvent(event);
    if (hit) {
      if (editor.editing && !commitEdit().ok) return;
      applySelection(selectCell(viewOf(hit.tab), { ...sel, tab: hit.tab }, hit.r, hit.c, { extend: false }));
    }
    if (!editor.editing) openEdit();
  }

  function openChartMenuAt(ruleId, cx, cy) {
    pendingTick = null;
    chartDrag = null;
    if (!ensureReadyForAction()) return;
    selectedChart = { tab: tabs.activeId, id: ruleId };
    syncUI({ announce: false });
    runLazy(cmdTool, (b) => {
      ensureCommands(b).actions.openChartContextMenu(ruleId, cx, cy);
    });
  }

  function openCellMenuAt(hit, cx, cy, { touch = false } = {}) {
    pendingTick = null;
    if (!ensureReadyForAction()) return;
    const view = viewOf(hit.tab);
    const box = hit.tab === sel.tab ? selectionBox(view, sel) : null;
    if (!box || hit.r < box.top || hit.r > box.bottom || hit.c < box.left || hit.c > box.right) {
      actOnCellPointer(hit, { extend: false });
    } else {
      selectedChart = null;
      syncUI({ announce: false });
    }
    const span = hit.tab === sel.tab ? axisSpan(viewOf(sel.tab), sel) : null;
    runLazy(cmdTool, (b) => {
      const { actions } = ensureCommands(b);
      if (span?.axis === "rows") actions.openRowHeaderMenu(hit.r, null, cx, cy);
      else if (span?.axis === "cols") actions.openColHeaderMenu(hit.c, null, cx, cy);
      else actions.openCellContextMenu(hit.r, hit.c, cx, cy, { touch });
    });
  }

  function onContextMenu(event) {
    if (isOverlayOrChromeTarget(event) || isScrollbarTarget(event)) return;
    const cx = event.clientX ?? 0;
    const cy = event.clientY ?? 0;
    const chartHit = chartHitAt(cx, cy);
    if (chartHit) {
      event.preventDefault();
      if (isTouch()) {
        if (longPress) {
          cancelLongPress();
          fireChartLongPress(chartHit.rule.id, cx, cy);
        }
        return;
      }
      openChartMenuAt(chartHit.rule.id, cx, cy);
      return;
    }
    const hit = locateCellEvent(event);
    if (!hit) return;
    event.preventDefault();
    if (isTouch()) {
      // Android fires its own contextmenu during a long press: the sheet's timer opens the menu, once.
      if (longPress) {
        const pending = longPress;
        cancelLongPress();
        if (pending.chartId) fireChartLongPress(pending.chartId, pending.x, pending.y);
        else fireLongPress(pending.hit, pending.x, pending.y);
      }
      return;
    }
    openCellMenuAt(hit, cx, cy);
  }

  function subscribeGrids() {
    shell?.restart?.();
    for (const off of unsubGrids.splice(0)) off();
    resizeObserver?.disconnect();
    for (const tab of tabs.list()) {
      resizeObserver?.observe(tab.table);
      const grid = gridOf(tab.table);
      if (!grid) continue;
      unsubGrids.push(
        grid.on("change", (event) => {
          if (disposed) return;
          if (mine) {
            if (!room()) history.record(tab.id, event);
          } else if (event.source !== "derived") {
            onForeignChange(tab.id, event);
          }
          const rw = windowOf(tab.table);
          if (rw?.geometry && event.rows?.changed?.length) {
            for (const rowId of event.rows.changed) {
              const idx = rw.geometry.indexOf(rowId);
              if (idx === -1) continue;
              const tr = rw.geometry.row(idx);
              if (tr.offsetHeight === 0 && !rowOutOfSight(tr)) {
                const h = Number.parseFloat(tr.style?.height);
                rw.geometry.setRowHeight(idx, Number.isFinite(h) && h > 0 ? h : ROW_ESTIMATE);
              }
            }
          }
          if (tab.id === sel?.tab) {
            sel = clipSelection(viewOf(sel.tab), sel, lastViews.get(sel.tab) ?? null);
            writeSelection(storage, path, sel);
          }
          if (event.structure) pruneFilters();
          if (tab.id === tabs.activeId) refreshStatus();
          findCtrl?.refresh();
          snapshotViews();
          redraw();
        }),
      );
    }
  }

  function restart() {
    if (disposed) return;
    cancelLongPress();
    handleDrag = null;
    closeTransient();
    const list = tabs.list();
    const firstId = list[0]?.id ?? "";
    const keepTab = list.some((t) => t.id === sel?.tab) ? sel.tab : firstId;
    const entered = keepTab !== sel?.tab;
    const view = viewOf(keepTab);
    sel = entered
      ? createSelection(view, keepTab)
      : clipSelection(view, sel, lastViews.get(keepTab) ?? null);
    if (!editor.editing) tabs.show(sel.tab);
    else tabs.refresh();
    writeSelection(storage, path, sel);
    snapshotViews();
    subscribeGrids();
    refreshStatus();
    findCtrl?.refresh();
    pruneFilters();
    syncUI({ announce: entered, enteredTab: entered });
  }

  function pruneFilters() {
    const live = new Set(tabs.list().map((tab) => tab.id));
    for (const tabId of [...filters.keys()]) {
      if (!live.has(tabId)) {
        filters.delete(tabId);
        continue;
      }
      const table = tabTable(workbook, tabId);
      const view = viewOf(tabId);
      const spec = filters.get(tabId);
      const header = view.rows.indexOf(spec.header);
      const criteriaBefore = spec.criteria.size;
      spec.cols = spec.cols.filter((id) => view.cols.includes(id));
      for (const id of [...spec.criteria.keys()]) {
        if (!spec.cols.includes(id)) spec.criteria.delete(id);
      }
      if (!table || header === -1 || spec.cols.length === 0) {
        setFilter(tabId, null);
        continue;
      }
      if (spec.criteria.size !== criteriaBefore) {
        if (spec.criteria.size === 0) spec.hidden = new Set();
        else {
          const mod = filterTool.peek();
          if (mod) spec.hidden = mod.hiddenRowsFor(view, spec);
        }
      }
      const keep = new Set(view.rows);
      spec.hidden = new Set([...(spec.hidden ?? [])].filter((id) => keep.has(id)));
      applyHiddenRows(tabId, spec.hidden);
    }
  }

  const raf = typeof win.requestAnimationFrame === "function" ? win.requestAnimationFrame.bind(win) : null;
  let framePending = false;

  function redraw() {
    if (disposed || framePending) return;
    if (!raf) return syncUI({ announce: false });
    framePending = true;
    raf(() => {
      framePending = false;
      if (!disposed) syncUI({ announce: false });
    });
  }

  const resizeObserver = typeof win.ResizeObserver === "function" ? new win.ResizeObserver(() => redraw()) : null;
  const onViewportMove = () => {
    cancelLongPress();
    redraw();
  };
  const cleanups = [];
  const listen = (el, evt, fn, opts) => {
    el.addEventListener(evt, fn, opts);
    cleanups.push(() => el.removeEventListener(evt, fn, opts));
  };

  listen(win, "keydown", onWindowKeyDown, true);
  listen(editor.textarea, "keydown", runKey);
  listen(editor.textarea, "beforecopy", onBeforeClipboard);
  listen(editor.textarea, "beforecut", onBeforeClipboard);
  listen(editor.textarea, "copy", onCopy);
  listen(editor.textarea, "cut", onCut);
  listen(editor.textarea, "paste", onPaste);
  listen(chrome.formulaInput, "keydown", runKey);
  listen(workbook, "mousedown", onMouseDown);
  listen(win, "mousemove", onMouseMove);
  listen(win, "mouseup", onMouseUp);
  listen(workbook, "click", onClick);
  listen(workbook, "dblclick", onDblClick);
  listen(workbook, "contextmenu", onContextMenu);
  listen(workbook, "pointerdown", onPointerDown);
  listen(win, "pointerdown", () => {
    suppressTap = false;
  }, true);
  listen(win, "mousedown", swallowAfterLongPress, true);
  listen(win, "click", swallowAfterLongPress, true);
  listen(win, "pointermove", onPointerMove);
  listen(win, "pointerup", onPointerEnd);
  listen(win, "pointercancel", onPointerEnd);
  listen(scrollTarget, "scroll", onViewportMove, { passive: true });
  listen(win, "resize", onViewportMove);

  initSelection();
  subscribeGrids();

  const menuProxy = {
    open: (opts) => runLazy(cmdTool, (b) => ensureCommands(b).menu.open(opts)),
    close: (opts) => liveMenu?.close(opts),
    get isOpen() {
      return Boolean(liveMenu?.isOpen);
    },
    get surface() {
      return liveMenu?.surface ?? null;
    },
    dispose: () => liveMenu?.dispose(),
  };

  function dispose() {
    if (disposed) return;
    disposed = true;
    cancelLongPress();
    fillDrag = null;
    chartDrag = null;
    selectedChart = null;
    closeFilterPanel();
    closeRuleSurface();
    closeDialog();
    liveMenu?.dispose();
    findCtrl?.dispose();
    fnHelp?.dispose();
    fnHelp = null;
    for (const off of unsubGrids.splice(0)) off();
    for (const off of cleanups.splice(0)) off();
    for (const table of workbook.querySelectorAll("table[grid]")) setFilteredRows(table, null);
    resizeObserver?.disconnect();
    tabs.dispose();
    overlays.dispose();
    chrome.dispose();
  }

  // Resolves a room target id (`s1/r2/c2`, `s1#rule:k1`) or `{ tab, row, col }` / `{ tab, rule }`
  // into a cell box on its tab so C9b's conflict panel can reveal and tint it.
  function resolveTarget(target) {
    if (!target) return null;
    let tab = null;
    let row = null;
    let col = null;
    let ruleId = null;
    if (typeof target === "string") {
      const hash = target.indexOf("#");
      const base = hash >= 0 ? target.slice(0, hash) : target;
      const extra = hash >= 0 ? target.slice(hash + 1) : "";
      const parts = base.split("/").map((p) => {
        try {
          return decodeURIComponent(p);
        } catch {
          return p;
        }
      });
      tab = parts[0] || null;
      row = parts[1] || null;
      col = parts[2] || null;
      if (extra.startsWith("rule:")) ruleId = extra.slice(5);
    } else if (typeof target === "object") {
      tab = target.tab ?? target.sheet ?? null;
      row = target.row ?? null;
      col = target.col ?? null;
      ruleId = target.rule ?? null;
    }
    if (!tab) return null;
    const table = tabTable(workbook, tab);
    if (!table) return null;
    const view = viewOf(tab);
    if (view.rows.length === 0 || view.cols.length === 0) return null;
    if (ruleId) {
      const grid = gridOf(table);
      const rule = grid?.rules?.()?.find((one) => one.id === ruleId) ?? null;
      const b = rule ? boundRange(rule.range, { rows: view.rows.length, cols: view.cols.length }) : null;
      if (!b) return null;
      return { tab, r0: b.r0, c0: b.c0, r1: b.r1, c1: b.c1 };
    }
    if (row && col) {
      const r = view.rows.indexOf(row);
      const c = view.cols.indexOf(col);
      if (r < 0 || c < 0) return null;
      return { tab, r0: r, c0: c, r1: r, c1: c };
    }
    if (row && !col) {
      const r = view.rows.indexOf(row);
      if (r < 0) return null;
      return { tab, r0: r, c0: 0, r1: r, c1: view.cols.length - 1 };
    }
    if (!row && col) {
      const c = view.cols.indexOf(col);
      if (c < 0) return null;
      return { tab, r0: 0, c0: c, r1: view.rows.length - 1, c1: c };
    }
    return { tab, r0: 0, c0: 0, r1: 0, c1: 0 };
  }

  function resolveConflictBoxes() {
    const out = [];
    for (const target of conflictTargets) {
      const box = resolveTarget(target);
      if (box) out.push(box);
    }
    return out;
  }

  function reveal(target) {
    const box = resolveTarget(target);
    if (!box) return false;
    const view = viewOf(box.tab);
    if (tabs.activeId !== box.tab) tabs.show(box.tab);
    applyMove({
      tab: box.tab,
      anchor: { row: view.rows[box.r0], col: view.cols[box.c0] },
      focus: { row: view.rows[box.r1], col: view.cols[box.c1] },
    });
    revealFocus();
    return true;
  }

  function setConflicts(targets = []) {
    conflictTargets = Array.isArray(targets) ? [...targets] : [];
    syncUI({ announce: false });
  }

  return {
    workbook,
    chrome,
    overlays,
    editor,
    tabs,
    menu: menuProxy,
    get selection() {
      return sel ? { tab: sel.tab, anchor: { ...sel.anchor }, focus: { ...sel.focus } } : null;
    },
    registerCommand(id, fn) {
      if (typeof fn === "function") externalCommands[id] = fn;
      else delete externalCommands[id];
      chrome.registerCommand(id, fn);
    },
    // What the sheet says to the person: the interface's own notice line.
    notice: (text, action) => chrome.showNotice(text, action),
    hideNotice: () => chrome.hideNotice(),
    reveal,
    setConflicts,
    restart,
    dispose,
  };
}

export function startInterface({
  shell = null,
  root = document,
  storage = globalThis.sessionStorage,
  tabOptions = {},
  commands = {},
  platform,
  measureText = null,
  loaders = {},
} = {}) {
  const instances = new Map();

  function mountMissing() {
    const live = new Set(root.querySelectorAll(WORKBOOK));
    for (const [el, inst] of instances) {
      if (live.has(el)) continue;
      inst.dispose();
      instances.delete(el);
    }
    for (const el of live) {
      if (!instances.has(el)) {
        instances.set(el, createWorkbookInterface(el, { shell, storage, tabOptions, commands, platform, measureText, loaders }));
      }
    }
  }

  mountMissing();

  return {
    get selection() {
      const first = instances.values().next().value;
      return first ? first.selection : null;
    },
    of: (workbook) => instances.get(workbook) ?? null,
    notice(text, action) {
      const first = instances.values().next().value;
      first?.notice(text, action);
    },
    hideNotice() {
      const first = instances.values().next().value;
      first?.hideNotice();
    },
    reveal(target) {
      const first = instances.values().next().value;
      return first ? first.reveal(target) : false;
    },
    setConflicts(targets) {
      const first = instances.values().next().value;
      first?.setConflicts(targets);
    },
    restart() {
      mountMissing();
      for (const inst of instances.values()) inst.restart();
    },
    dispose() {
      for (const inst of instances.values()) inst.dispose();
      instances.clear();
    },
  };
}
