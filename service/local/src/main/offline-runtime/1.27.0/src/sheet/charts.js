import { chartSvg } from "../lib/chart-svg.js";

// A tab's charts (sapjs S8d: a `kind="chart"` rule; GRAMMAR.md "Charts' drawings"). The rule is the
// chart; the drawing is derived output, like a formula's result. Each chart rule has one
// <figure chart="ID" sap-ignore=""> after the tab's table, in rules order, holding the last drawing,
// so a reader without scripts sees it. While scripts run, an adopted stylesheet lays each figure
// over its box of cells; nothing but its content is ever set on the figure.

const isNumber = (value) => typeof value === "number" && Number.isFinite(value);
const isBlank = (value) => value === null || value === undefined || value === "";
const transpose = (rows) => {
  const width = Math.max(0, ...rows.map((row) => row.length));
  return Array.from({ length: width }, (_, c) => rows.map((row) => row[c] ?? null));
};
const cssString = (text) => `"${String(text).replace(/["\\]/g, "\\$&").replace(/[\n\r\f]/g, " ")}"`;

// A chart's model from its source's values and their shown text, the rows a viewer cannot see
// already left out. Columns are series unless the rule says rows. The first column is category
// labels when a cell below the first row holds text (for scatter, it is always the x values); the
// first row is series names when none of its series cells is a number and one holds text.
export function chartModel(rule, values, texts) {
  const byRows = rule.series === "rows";
  const vals = byRows ? transpose(values) : values;
  const shown = byRows ? transpose(texts) : texts;
  const model = { type: rule.type, title: rule.title ?? "", stacked: Boolean(rule.stacked), categories: [], xs: null, series: [] };
  const rows = vals.length;
  const cols = Math.max(0, ...vals.map((row) => row.length));
  if (rows === 0 || cols === 0) return model;
  const at = (r, c) => vals[r]?.[c] ?? null;
  const textAt = (r, c) => String(shown[r]?.[c] ?? "");
  const scatter = rule.type === "scatter";
  const labels = cols > 1;
  const first = labels ? 1 : 0;
  let anyText = false;
  let anyNumber = false;
  for (let c = first; c < cols; c++) {
    const value = at(0, c);
    if (isNumber(value)) anyNumber = true;
    else if (!isBlank(value)) anyText = true;
  }
  const header = rows > 1 && anyText && !anyNumber;
  const r0 = header ? 1 : 0;
  for (let r = r0; r < rows; r++) model.categories.push(labels ? textAt(r, 0) : String(r - r0 + 1));
  if (scatter) {
    model.xs = [];
    for (let r = r0; r < rows; r++) model.xs.push(labels ? (isNumber(at(r, 0)) ? at(r, 0) : null) : r - r0 + 1);
  }
  for (let c = first; c < cols; c++) {
    const values = [];
    for (let r = r0; r < rows; r++) values.push(isNumber(at(r, c)) ? at(r, c) : null);
    model.series.push({ name: header ? textAt(0, c) : `Series ${c - first + 1}`, values });
  }
  return model;
}

// Keeps a tab's figures matching its chart rules: one per chart, in rules order, right after the
// table; others go. `drawingOf(rule)` gives the drawing, or null to keep the figure's content as it
// is. Returns whether any figure's saved content changed.
export function syncFigures(section, table, rules, drawingOf) {
  const charts = rules.filter((rule) => rule.kind === "chart");
  const wanted = new Set(charts.map((rule) => rule.id));
  const figures = new Map();
  let changed = false;
  for (const el of [...section.children]) {
    if (el.localName !== "figure" || !el.hasAttribute("chart")) continue;
    const id = el.getAttribute("chart");
    if (wanted.has(id) && !figures.has(id)) figures.set(id, el);
    else {
      el.remove();
      changed = true;
    }
  }
  let after = table;
  for (const rule of charts) {
    let figure = figures.get(rule.id);
    if (!figure) {
      figure = section.ownerDocument.createElement("figure");
      figure.setAttribute("chart", rule.id);
      figure.setAttribute("sap-ignore", "");
      changed = true;
    }
    const svg = drawingOf(rule);
    if (svg !== null) {
      const before = figure.innerHTML;
      figure.innerHTML = svg;
      if (figure.innerHTML !== before) changed = true;
    }
    if (after.nextElementSibling !== figure) after.after("\n", figure);
    after = figure;
  }
  return changed;
}

const BIG = { rows: 10000000, cols: 16384 };

// The screen-only rule every placed figure gets: opaque, out of the flow, and deaf to presses
// unless the editor is the one running (there a press on a chart is a press on the figure).
const FIGURE_RULE = `[sheet-workbook] section[item]>figure[chart]{position:absolute;margin:0;z-index:7;overflow:hidden;pointer-events:none;background:var(--clay-ui-surface, Canvas)}`;
const EDIT_RULE = `[sheet-workbook] section[item]>figure[chart]{pointer-events:auto}`;
// A wide drawing fits the paper it is printed on.
const PRINT_RULE = `@media print{[sheet-workbook] section[item]>figure[chart]>svg{max-width:100%;height:auto}}`;

// The charts a workbook has drawn once, by `sectionId + "\u0000" + chartId`: only a chart's first
// drawing after the page loaded counts as a change, and a table a merge rebuilt finds its charts
// here, so a peer's drawing never counts as ours.
const drawnOnce = new WeakMap();
function drawnIn(table) {
  const root = table.closest("[sheet-workbook]") ?? table.ownerDocument;
  let set = drawnOnce.get(root);
  if (!set) drawnOnce.set(root, (set = new Set()));
  return set;
}

export function createCharts(tools) {
  const { boundRange, readBox, readTexts, rowOutOfSight, tableMetrics, tableParts, tableNamed, scopeOf, canAdopt, gridOf, onRedrawn } = tools;
  const { ROW_ESTIMATE, COL_DEFAULT } = tools;
  const editMode = () => Boolean(tools.editMode?.());
  const draw = tools.chartSvg ?? chartSvg;
  const screenRule = (extra = "") => `@media screen{${FIGURE_RULE}${editMode() ? EDIT_RULE : ""}${extra}}`;
  // table -> { grid, off, windowOf, keys: Map(id -> key), shift: Map(id -> {x, y}), first }
  const tabs = new Map();
  let sheet = null;
  let frame = 0;
  let observer = null;
  let doc = null;

  // A chart's source as typed values and the shown text `chartModel` asks for: rows out of sight
  // in this viewer and columns the file hides are left out, and an open-ended source stops at its
  // last row or column that holds something. Only the first row and the first column are read as
  // text; those are the only texts a chart's model uses.
  function sourceOf(table, rule) {
    const area = gridOf.areaOf?.(String(rule.source ?? ""));
    if (!area) return { values: [], texts: [] };
    const source = area.sheet ? tableNamed(table, area.sheet) : table;
    if (!source) return { values: [], texts: [] };
    const grid = gridOf(source);
    const size = grid.size;
    const box = {
      r0: area.top ?? 0,
      c0: area.left ?? 0,
      r1: Math.min(area.bottom ?? size.rows - 1, size.rows - 1),
      c1: Math.min(area.right ?? size.cols - 1, size.cols - 1),
    };
    if (box.r1 < box.r0 || box.c1 < box.c0) return { values: [], texts: [] };
    const raw = readBox(grid, box);
    const sourceRows = source.rows;
    const rows = [];
    for (let i = 0; i < raw.length; i++) if (!rowOutOfSight(sourceRows[box.r0 + i])) rows.push(i);
    const cols = [];
    const colEls = tableParts(source).cols;
    for (let j = 0; j <= box.c1 - box.c0; j++) if (!colEls[box.c0 + j]?.hidden) cols.push(j);
    const values = rows.map((i) => cols.map((j) => raw[i]?.[j] ?? null));
    const width = () => {
      let widest = 0;
      for (const row of values) widest = Math.max(widest, row.length);
      return widest;
    };
    if (area.bottom == null) while (values.length > 0 && values[values.length - 1].every(isBlank)) values.pop();
    if (area.right == null) {
      let w = width();
      while (w > 0 && values.every((row) => isBlank(row[w - 1]))) {
        for (const row of values) row.pop();
        w -= 1;
      }
    }
    if (values.length === 0 || rows.length === 0 || cols.length === 0 || width() === 0) return { values, texts: [] };
    const headRow = box.r0 + rows[0];
    const headCol = box.c0 + cols[0];
    const head = readTexts(grid, { r0: headRow, r1: headRow, c0: box.c0, c1: box.c1 })[0] ?? [];
    const lead = readTexts(grid, { r0: box.r0, r1: box.r1, c0: headCol, c1: headCol }).map((row) => row[0] ?? "");
    const texts = values.map((row, ri) =>
      row.map((_, j) => (ri === 0 ? head[cols[j]] ?? "" : j === 0 ? lead[rows[ri]] ?? "" : "")),
    );
    return { values, texts };
  }

  // The drawing's size comes from the document: the box's `<col>` widths and `<tr>` heights as the
  // file saves them, never from layout, so an unchanged file draws the same drawing on every open.
  function sizeOf(table, box) {
    const colEls = tableParts(table).cols;
    const trs = table.rows;
    let width = 0;
    let height = 0;
    for (let c = box.c0; c <= box.c1; c++) {
      const col = colEls[c];
      if (!col) width += COL_DEFAULT;
      else if (!col.hidden) {
        const w = parseFloat(col.style.width);
        width += Number.isFinite(w) ? w : COL_DEFAULT;
      }
    }
    for (let r = box.r0; r <= box.r1; r++) {
      const tr = trs[r];
      if (!tr) height += ROW_ESTIMATE;
      else if (!tr.hidden) {
        const h = parseFloat(tr.style.height);
        height += Number.isFinite(h) ? h : ROW_ESTIMATE;
      }
    }
    return { width: Math.max(120, Math.round(width)), height: Math.max(80, Math.round(height)) };
  }

  function pixelBox(state, table, rule) {
    const box = boundRange(rule.range, BIG);
    if (!box) return null;
    return tableMetrics(table, state.windowOf()).spanBox(box.r0, box.c0, box.r1, box.c1);
  }

  function drawingFor(state, table, rule, force) {
    const box = boundRange(rule.range, BIG);
    const size = box ? sizeOf(table, box) : { width: 120, height: 80 };
    const { values, texts } = sourceOf(table, rule);
    const model = chartModel(rule, values, texts);
    const key = JSON.stringify([model, size.width, size.height]);
    if (!force && state.keys.get(rule.id) === key) return null;
    state.keys.set(rule.id, key);
    let svg = draw(model, size);
    // A drawing the room codec would refuse is never saved: it would lock the file out of rooms.
    if (gridOf.drawingProblem?.(svg) != null) svg = draw({ ...model, categories: [], xs: null, series: [] }, size);
    return svg;
  }

  // The cells a Grid change touched, by the ids of their row and column.
  function touchedBy(event) {
    const cells = [];
    for (const cell of [...(event.cells ?? []), ...(event.results ?? [])]) {
      if (cell && (cell.row || cell.col)) cells.push({ row: cell.row ?? null, col: cell.col ?? null });
    }
    return cells;
  }

  // A change that may move any cell of a tab: rows or columns added, removed or resized, a rule
  // written, or the tab rebuilt. Every chart of the tab is drawn again.
  function structural(event) {
    if (!event) return true;
    if (event.structure || event.rescanned) return true;
    for (const side of [event.rows, event.cols]) {
      if (!side) continue;
      if (Array.isArray(side) ? side.length > 0 : Boolean(side.added?.length || side.removed?.length || side.changed?.length)) return true;
    }
    return false;
  }

  // Whether a cell a change touched lands in a chart's source box, so only the charts a change
  // can move are read and drawn again.
  function readsChanged(table, rule, changed) {
    const area = gridOf.areaOf?.(String(rule.source ?? ""));
    if (!area) return true;
    const source = area.sheet ? tableNamed(table, area.sheet) : table;
    if (!source) return false;
    if (source !== changed.table) return false;
    const size = gridOf(source).size;
    const r0 = Math.max(0, area.top ?? 0);
    const r1 = Math.min(area.bottom ?? size.rows - 1, size.rows - 1);
    const c0 = Math.max(0, area.left ?? 0);
    const c1 = Math.min(area.right ?? size.cols - 1, size.cols - 1);
    const trs = source.rows;
    const colEls = tableParts(source).cols;
    for (const cell of changed.cells) {
      let inRows = cell.row === null;
      for (let r = r0; !inRows && r <= Math.min(r1, trs.length - 1); r++) {
        inRows = trs[r]?.getAttribute("data-id") === cell.row;
      }
      if (!inRows) continue;
      let inCols = cell.col === null;
      for (let c = c0; !inCols && c <= Math.min(c1, colEls.length - 1); c++) {
        inCols = colEls[c]?.getAttribute("data-id") === cell.col;
      }
      if (inCols) return true;
    }
    return false;
  }

  function refresh(table, changed = null) {
    const state = tabs.get(table);
    if (!state) return;
    if (!table.isConnected) {
      forget(table);
      return;
    }
    const rules = state.grid.rules?.() ?? [];
    const first = state.first;
    state.first = false;
    const drawn = drawnIn(table);
    const sectionId = table.parentElement?.getAttribute("data-id") ?? "";
    let counted = false;
    syncFigures(table.parentElement, table, rules, (rule) => {
      if (changed && !readsChanged(table, rule, changed)) return null;
      const figure = table.parentElement.querySelector(`:scope > figure[chart=${cssString(rule.id)}]`);
      const key = `${sectionId}\u0000${rule.id}`;
      const fresh = !drawn.has(key);
      const svg = drawingFor(state, table, rule, first || fresh || !figure);
      drawn.add(key);
      // A file a room or a host saved holds old drawings: a chart's first drawing that differs
      // from the saved one counts as one change, so the next save writes the drawings back.
      if (fresh && figure && svg !== null && figure.innerHTML !== svg) counted = true;
      return svg;
    });
    if (counted) onRedrawn?.(table);
    schedule();
  }

  // Another tab's charts may read this tab's cells.
  function refreshReaders(source, changed = null) {
    for (const [table, state] of tabs) {
      if (table === source) continue;
      const reads = (state.grid.rules?.() ?? []).some((rule) => rule.kind === "chart" && String(rule.source ?? "").includes("!"));
      if (reads) refresh(table, changed);
    }
  }

  // Where a chart's top-left cell is on screen, for a box that starts in a frozen row too.
  function boxTop(metrics, table, r0, c0) {
    const headRows = table.tHead?.rows.length ?? 0;
    if (r0 >= headRows) return metrics.cellBox(r0, c0).top;
    const headHeight = (r) => {
      const tr = table.tHead.rows[r];
      if (!tr || tr.hidden) return 0;
      const h = parseFloat(tr.style.height);
      return Number.isFinite(h) ? h : ROW_ESTIMATE;
    };
    let above = 0;
    for (let r = r0; r < headRows; r++) above += headHeight(r);
    if (tableParts(table).rows.length === 0) {
      let before = 0;
      for (let r = 0; r < r0; r++) before += headHeight(r);
      return table.getBoundingClientRect().top + before;
    }
    return metrics.cellBox(headRows, c0).top - above;
  }

  function place() {
    frame = 0;
    if (!sheet) return;
    const parts = [];
    const pads = [];
    for (const [table, state] of tabs) {
      if (!table.isConnected) continue;
      // A figure with no client rects is on a hidden tab: its shift stays as it was.
      const shown = [];
      for (const rule of state.grid.rules?.() ?? []) {
        if (rule.kind !== "chart") continue;
        const figure = table.parentElement.querySelector(`:scope > figure[chart=${cssString(rule.id)}]`);
        if (figure && figure.getClientRects().length > 0) shown.push({ rule, figure });
      }
      if (shown.length === 0) continue;
      const scope = scopeOf(table);
      const metrics = tableMetrics(table, state.windowOf());
      const tableBottom = table.getBoundingClientRect().bottom;
      let overhang = 0;
      for (const { rule, figure } of shown) {
        const box = boundRange(rule.range, BIG);
        if (!box) continue;
        const size = sizeOf(table, box);
        const left = metrics.cellBox(box.r0, box.c0).left;
        const top = boxTop(metrics, table, box.r0, box.c0);
        // The figure's own box at its current shift gives where a shift of 0 puts it, whatever
        // the page's positioned ancestors and scrollers are.
        const at = figure.getBoundingClientRect();
        const was = state.shift.get(rule.id) ?? { x: 0, y: 0 };
        const x = Math.round(left - (at.left - was.x));
        const y = Math.round(top - (at.top - was.y));
        state.shift.set(rule.id, { x, y });
        parts.push(
          `${scope}~figure[chart=${cssString(rule.id)}]{translate:${x}px ${y}px;width:${size.width}px;height:${size.height}px}`,
        );
        overhang = Math.max(overhang, top + size.height - tableBottom);
      }
      // A reader's charts are out of the flow, so a tab keeps the height its lowest chart needs.
      if (overhang > 0) pads.push(`${scope.replace(/>table\[grid\]$/, "")}{padding-bottom:${Math.round(overhang)}px}`);
    }
    sheet.replaceSync(`${screenRule(parts.join("") + pads.join(""))}${PRINT_RULE}`);
  }

  function schedule() {
    if (frame || !doc) return;
    const win = doc.defaultView;
    frame = win.requestAnimationFrame ? win.requestAnimationFrame(place) : (place(), 0);
  }

  function start(table) {
    if (doc) return;
    doc = table.ownerDocument;
    const win = doc.defaultView;
    if (canAdopt(doc)) {
      sheet = new win.CSSStyleSheet();
      doc.adoptedStyleSheets = [...doc.adoptedStyleSheets, sheet];
      // The base rule lands before the first measure, so the first frame is of a figure that is
      // already absolutely placed.
      sheet.replaceSync(`${screenRule()}${PRINT_RULE}`);
    }
    doc.addEventListener("scroll", schedule, { capture: true, passive: true });
    win.addEventListener("resize", schedule);
    observer = win.ResizeObserver ? new win.ResizeObserver(schedule) : null;
  }

  function forget(table) {
    const state = tabs.get(table);
    if (!state) return;
    state.off?.();
    observer?.unobserve(table);
    tabs.delete(table);
  }

  // Follows a tab: draws its charts now and after each change of its Grid (results included).
  function watch(table, grid, windowOf = () => null) {
    start(table);
    let state = tabs.get(table);
    if (state && state.grid !== grid) {
      forget(table);
      state = null;
    }
    if (!state) {
      state = { grid, off: null, windowOf, keys: new Map(), shift: new Map(), first: true };
      tabs.set(table, state);
      state.off = grid.on("change", (event) => {
        const changed = structural(event) ? null : { table, cells: touchedBy(event) };
        refresh(table, changed);
        refreshReaders(table, changed);
      });
      observer?.observe(table);
    }
    for (const other of [...tabs.keys()]) if (!other.isConnected) forget(other);
    refresh(table);
    refreshReaders(table);
  }

  function chartRect(table, ruleId) {
    const state = tabs.get(table);
    if (!state || !table.isConnected) return null;
    const rule = (state.grid.rules?.() ?? []).find((r) => r.kind === "chart" && r.id === ruleId);
    return rule ? pixelBox(state, table, rule) : null;
  }

  function chartAt(clientX, clientY, onlyTable = null) {
    const entries = onlyTable ? (tabs.has(onlyTable) ? [[onlyTable, tabs.get(onlyTable)]] : []) : [...tabs].reverse();
    for (const [table, state] of entries) {
      if (!table.isConnected) continue;
      if (!onlyTable && doc?.defaultView?.getComputedStyle?.(table.parentElement)?.display === "none") continue;
      const rules = state.grid.rules?.() ?? [];
      for (let i = rules.length - 1; i >= 0; i--) {
        const rule = rules[i];
        if (rule.kind !== "chart") continue;
        const rect = pixelBox(state, table, rule);
        if (
          rect &&
          rect.width > 0 &&
          rect.height > 0 &&
          clientX >= rect.left &&
          clientX < rect.left + rect.width &&
          clientY >= rect.top &&
          clientY < rect.top + rect.height
        ) {
          return { table, rule, rect };
        }
      }
    }
    return null;
  }

  return { watch, refresh, place, chartAt, chartRect };
}
