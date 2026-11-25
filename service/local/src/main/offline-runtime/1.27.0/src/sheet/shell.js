import { boundRange, COL_DEFAULT, isEmpty, listItems, matchKey, parseRuleStyle, readBox, readTexts, ROW_ESTIMATE, rowOutOfSight, tableMetrics, tableParts, tableNamed } from "./geometry.js";
import { canAdopt, createRowWindow, EARLY, scopeOf } from "./row-window.js";

// sapjs's two grid extensions, carried by this release but addressed only on sheet pages, so a
// Document page's plugin table stays as it was. Each is a literal import, so the chunk build splits it.
const EXTENSIONS = {
  "grid-functions": () => import("../vendor/sap-grid-functions.vendor.js"),
  "grid-formats": () => import("../vendor/sap-grid-formats.vendor.js"),
};

const ADDRESSED = Symbol.for("clayjs.sheet.gridExtensions");

// Teaches the page's plugin loader where the two extensions are, once; other names go to the
// loader that was there before.
export function addressGridExtensions(plugins) {
  if (!plugins || plugins[ADDRESSED]) return;
  const before = plugins.loader;
  plugins.loader = (name, facts) => (Object.hasOwn(EXTENSIONS, name) ? EXTENSIONS[name]() : before?.(name, facts));
  plugins[ADDRESSED] = true;
}

// A tab's saved rules paint through the lazy rules.js, loaded only once some table has rules.
let rulesModule = null;
const rulesPending = new Set();
const watchedGrids = new WeakSet();
const watchedTables = new Map();
function paintRulesSoon(table, grid) {
  const rules = grid?.rules?.();
  if (!rules) return;
  if (!rulesModule && rules.length === 0) return;
  if (rulesPending.has(table)) return;
  rulesPending.add(table);
  rulesModule ??= import("./rules.js").then((module) =>
    module.createRules({ boundRange, isEmpty, listItems, matchKey, parseStyle: parseRuleStyle, readBox, readTexts, canAdopt, scopeOf }));
  rulesModule.then((rules) => {
    rulesPending.delete(table);
    rules.paintRules(table, grid);
  });
}

// Charts draw through the lazy charts.js, loaded only once some tab's rules hold a chart (or the
// editor inserts one). The editor sets `sheetHooks.chartRedrawn` so a drawing that differs from
// the saved one on open counts as a change; readers never set it.
export const sheetHooks = { chartRedrawn: null, bookOf: null };
let chartsModule = null;
let chartsReady = null;
const chartsWatched = new WeakSet();
export const peekCharts = () => chartsReady;
export function loadCharts() {
  chartsModule ??= import("./charts.js").then((module) => {
    chartsReady = module.createCharts({
      boundRange, COL_DEFAULT, readBox, readTexts, ROW_ESTIMATE, rowOutOfSight, tableMetrics, tableParts, tableNamed,
      scopeOf, canAdopt,
      gridOf: window.Sap.grid,
      editMode: () => Boolean(sheetHooks.chartRedrawn),
      onRedrawn: (table) => sheetHooks.chartRedrawn?.(table),
    });
    // Charts follow every tab from the moment the module loads, so a chart that reads another
    // tab's cells hears that tab's first edit.
    for (const [table, entry] of watchedTables) {
      if (!table.isConnected) continue;
      chartsReady.watch(table, entry.grid, entry.windowOf);
    }
    return chartsReady;
  });
  return chartsModule;
}
function drawChartsSoon(table, grid, windowOf) {
  const rules = grid?.rules?.();
  if (!rules) return;
  if (!chartsModule && !rules.some((rule) => rule.kind === "chart")) return;
  if (chartsWatched.has(grid)) return;
  chartsWatched.add(grid);
  loadCharts().then((charts) => charts.watch(table, grid, windowOf));
}

// A dropdown may read another tab's range, so a change on the tab that holds the items repaints
// the dropdowns that point at it: their red underlines go as the items arrive.
function repaintListTabs(source) {
  for (const [table, entry] of watchedTables) {
    if (table === source) continue;
    if (!table.isConnected) {
      watchedTables.delete(table);
      continue;
    }
    const reads = entry.grid.rules?.().some((rule) => rule.kind === "list" && String(rule.source ?? "").includes("!"));
    if (reads) paintRulesSoon(table, entry.grid);
  }
}

function watchRules(table, grid, windowOf) {
  paintRulesSoon(table, grid);
  drawChartsSoon(table, grid, windowOf);
  if (watchedGrids.has(grid)) return;
  watchedGrids.add(grid);
  watchedTables.set(table, { grid, windowOf });
  grid.on("change", () => {
    paintRulesSoon(table, grid);
    drawChartsSoon(table, grid, windowOf);
    repaintListTabs(table);
  });
}

const IDLE_DELAY_MS = 3000;
const IDLE_TIMEOUT_MS = 5000;

// What both halves of the sheet run: a row window on every grid table under a [sap]
// root, following its Grid once the engine has mounted, plus the idle function-pack
// load and on-demand custom number formats load. The page's own early rule is dropped
// once every table has its window.
export function startShell({ root = document, windowOptions = {} } = {}) {
  const doc = root.ownerDocument ?? root;
  const win = doc.defaultView ?? window;
  const windows = new Map();
  const unsubscribe = [];
  const custom = Boolean(windowOptions.sheet || windowOptions.adopt);
  const canWindow = custom || canAdopt(doc);
  let disposed = false;
  let delayTimer = null;
  let idleHandle = null;
  let formatsRequested = false;

  const formatsInstalled = () => Boolean(win.Sap?.plugins?.has?.("grid-formats"));

  function requestFormats(sapRef) {
    if (disposed || win.Sap !== sapRef || formatsRequested || formatsInstalled()) return;
    const load = win.Sap?.plugins?.load;
    if (typeof load !== "function") return;
    formatsRequested = true;
    Promise.resolve(load.call(win.Sap.plugins, "grid-formats")).catch((error) => {
      console.error("clayjs: the sheet could not load number formats", error);
    });
  }

  function scanOpenFormats(gridOf, sapRef) {
    if (disposed || formatsRequested || formatsInstalled() || typeof gridOf?.fmtKind !== "function") return;
    for (const el of root.querySelectorAll("[sheet-workbook] [fmt], [sap] table[grid] [fmt]")) {
      const code = el.getAttribute("fmt");
      if (code && gridOf.fmtKind(code) === "unknown") {
        requestFormats(sapRef);
        return;
      }
    }
  }

  function checkChangeFormats(table, gridOf, sapRef, event) {
    if (disposed || formatsRequested || formatsInstalled() || typeof gridOf?.fmtKind !== "function") return;
    if (event?.source === "derived") return;
    if (event?.rescanned) {
      for (const el of table.querySelectorAll("[fmt]")) {
        const code = el.getAttribute("fmt");
        if (code && gridOf.fmtKind(code) === "unknown") return requestFormats(sapRef);
      }
      return;
    }
    const colIds = new Set([...(event?.cols?.changed ?? []), ...(event?.cols?.added ?? [])]);
    for (const cell of event?.cells ?? []) if (cell.col) colIds.add(cell.col);
    if (colIds.size > 0) {
      for (let node = table.firstElementChild; node !== null; node = node.nextElementSibling) {
        if (node.localName !== "colgroup") continue;
        for (let col = node.firstElementChild; col !== null; col = col.nextElementSibling) {
          if (col.localName !== "col" || !colIds.has(col.getAttribute("data-id"))) continue;
          const code = col.getAttribute("fmt");
          if (code && gridOf.fmtKind(code) === "unknown") return requestFormats(sapRef);
        }
        break;
      }
    }
    const rowIds = new Set([...(event?.rows?.added ?? []), ...(event?.cells ?? []).map((c) => c.row).filter(Boolean)]);
    if (rowIds.size === 0) return;
    for (const section of [table.tHead, ...(table.tBodies ?? [])]) {
      if (!section) continue;
      for (const tr of section.rows) {
        if (!rowIds.has(tr.getAttribute("data-id"))) continue;
        for (let td = tr.firstElementChild; td !== null; td = td.nextElementSibling) {
          const code = td.getAttribute("fmt");
          if (code && gridOf.fmtKind(code) === "unknown") return requestFormats(sapRef);
        }
      }
    }
  }

  function cancelFunctionsSchedule() {
    if (delayTimer !== null) {
      win.clearTimeout(delayTimer);
      delayTimer = null;
    }
    if (idleHandle !== null) {
      if (typeof win.cancelIdleCallback === "function") win.cancelIdleCallback(idleHandle);
      else win.clearTimeout(idleHandle);
      idleHandle = null;
    }
  }

  function loadFunctionsPack(sapRef) {
    if (disposed || win.Sap !== sapRef || win.Sap?.plugins?.has?.("grid-functions")) return;
    const load = win.Sap?.plugins?.load;
    if (typeof load !== "function") return;
    Promise.resolve(load.call(win.Sap.plugins, "grid-functions")).catch((error) => {
      console.error("clayjs: the sheet could not load extended functions", error);
    });
  }

  function scheduleFunctionsPack(sapRef) {
    if (disposed || delayTimer !== null || idleHandle !== null || win.Sap?.plugins?.has?.("grid-functions")) return;
    if (typeof win.Sap?.plugins?.load !== "function") return;
    delayTimer = win.setTimeout(() => {
      delayTimer = null;
      if (disposed || win.Sap !== sapRef) return;
      if (typeof win.requestIdleCallback === "function") {
        idleHandle = win.requestIdleCallback(
          () => {
            idleHandle = null;
            loadFunctionsPack(sapRef);
          },
          { timeout: IDLE_TIMEOUT_MS },
        );
      } else {
        loadFunctionsPack(sapRef);
      }
    }, IDLE_DELAY_MS);
    delayTimer?.unref?.();
  }

  // A window for every table that has none; a table that left the page loses its window. A
  // table that stayed keeps its window and the row heights it measured.
  function open() {
    if (!canWindow) return;
    const keep = new Set();
    for (const table of root.querySelectorAll("[sap] table[grid]")) {
      // A rule that would also select another table hides that table's rows too, so a
      // table its scope does not name alone gets no window: it shows every row, slower.
      const named = doc.querySelectorAll(scopeOf(table));
      if (named.length !== 1 || named[0] !== table) continue;
      keep.add(table);
      if (!windows.has(table)) windows.set(table, createRowWindow(table, windowOptions));
    }
    for (const [table, winRow] of windows) {
      if (keep.has(table)) continue;
      winRow.dispose();
      windows.delete(table);
    }
  }

  // Every window follows its table's Grid, and measures what it shows again, once.
  function subscribe(gridOf) {
    for (const off of unsubscribe.splice(0)) off();
    const sapRef = win.Sap;
    scanOpenFormats(gridOf, sapRef);
    if (!formatsRequested && !formatsInstalled() && typeof gridOf?.fmtKind === "function") {
      for (const table of root.querySelectorAll("[sap] table[grid]")) {
        unsubscribe.push(gridOf(table).on("change", (ev) => checkChangeFormats(table, gridOf, sapRef, ev)));
      }
    }
    for (const [table, winRow] of windows) {
      unsubscribe.push(gridOf(table).on("change", winRow.follow));
      watchRules(table, gridOf(table), () => windows.get(table) ?? null);
      winRow.follow({ rescanned: true });
    }
  }

  open();
  if (canWindow) dropEarly(win, doc);
  // Sap.grid exists once the grid plugin installed; clay.ready resolves after the mount. The
  // engine stops waiting for plugins after a few seconds, so on a slow connection the mount can
  // come before the grid: this joins the download still on its way.
  const followed = Promise.resolve(win.clay?.ready).then(async () => {
    if (disposed || (windows.size === 0 && !root.querySelector("[sap] table[grid]"))) return;
    if (typeof win.Sap?.grid !== "function") await win.Sap?.plugins?.load?.("grid");
    if (disposed) return;
    if (typeof win.Sap?.grid === "function") {
      addressGridExtensions(win.Sap.plugins);
      subscribe(win.Sap.grid);
      scheduleFunctionsPack(win.Sap);
    }
  });
  followed.catch((error) => console.error("clayjs: the sheet could not follow its grid", error));

  // The workbook was rebuilt (a merge took a peer's tabs, or the grid loaded late): every window
  // follows the tables and Grids the page holds now.
  function restart() {
    if (disposed) return;
    open();
    if (typeof win.Sap?.grid === "function") subscribe(win.Sap.grid);
  }

  function dispose() {
    if (disposed) return;
    disposed = true;
    cancelFunctionsSchedule();
    for (const off of unsubscribe.splice(0)) off();
    for (const winRow of windows.values()) winRow.dispose();
    windows.clear();
  }

  return { windows, windowOf: (table) => windows.get(table) ?? null, followed, restart, dispose };
}

function dropEarly(win = window, doc = document) {
  const early = win[EARLY];
  if (!early) return;
  delete win[EARLY];
  if ("adoptedStyleSheets" in doc) {
    doc.adoptedStyleSheets = doc.adoptedStyleSheets.filter((sheet) => sheet !== early);
  }
}
