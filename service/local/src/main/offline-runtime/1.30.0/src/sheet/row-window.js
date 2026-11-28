import { createGeometry, filteredRows, rowOutOfSight } from "./geometry.js";

// The row window: every <tbody> group of a sheet table stays in the document, and one
// constructable stylesheet per table hides the groups outside the viewport and gives
// their height back as the table's top and bottom margins (spike 05, PASS at 20,000
// cells). It writes nothing into the document, so a save never sees it and no mutation
// fires. `@media screen` keeps print whole. The same sheet carries the table's width,
// the sum of its <col> widths: `table-layout: fixed` ignores <col> widths on a table
// whose width is auto. The window corrects the scroll position itself when rows it
// measures move the rows on screen, so the table takes no part in the browser's scroll
// anchoring, which would move it a second time.

// The sheet the template's inline head script adopted before the table parsed. It shows
// the first three groups until the plugin's own windows take over.
export const EARLY = "__sheetEarlyWindow";

// The groups that meet the viewport, plus `overscan` on each side, and the pixels of the
// hidden groups above and below. Group numbers are 1-based, for :nth-of-type.
export function windowFor({ scrollTop, viewport, heights, overscan = 1 }) {
  if (heights.length === 0) return { first: 1, last: 0, before: 0, after: 0 };
  const starts = [];
  let total = 0;
  for (const height of heights) {
    starts.push(total);
    total += height;
  }
  const top = Math.max(0, scrollTop);
  const bottom = top + viewport;
  let firstIndex = -1;
  let lastIndex = -1;
  for (let i = 0; i < heights.length; i++) {
    const start = starts[i];
    const end = start + heights[i];
    if (end > top && start < bottom) {
      if (firstIndex === -1) firstIndex = i;
      lastIndex = i;
    }
  }
  if (firstIndex === -1) {
    firstIndex = top <= 0 ? 0 : heights.length - 1;
    lastIndex = firstIndex;
  }
  const first = Math.max(0, firstIndex - overscan);
  const last = Math.min(heights.length - 1, lastIndex + overscan);
  const before = starts[first];
  const after = total - (starts[last] + heights[last]);
  return { first: first + 1, last: last + 1, before, after };
}

export function ruleText({ first, last, before, after }, scope, width = 0, hiddenIds = []) {
  const size = width > 0 ? `${scope}{width:${width}px}` : "";
  const hidden = hiddenIds.length > 0
    ? `${hiddenIds.map((id) => `${scope}>tbody>tr[data-id="${String(id).replace(/["\\]/g, "\\$&")}"]`).join(",")}{display:none}`
    : "";
  return `${size}@media screen{${scope}{margin-top:${before}px;margin-bottom:${after}px;overflow-anchor:none}` +
    `${scope}>tbody:nth-of-type(-n+${first - 1}),${scope}>tbody:nth-of-type(n+${last + 1}){display:none}}${hidden}`;
}

// A tab's table is named by its section's id (sapjs counts a table as a tab only when its
// parent is the section); a bare table is named alone, so it must be the page's only one.
export function scopeOf(table) {
  const section = table.parentElement;
  const id = section?.matches("section[item][data-id]") ? section.getAttribute("data-id") : null;
  if (id) return `section[data-id="${id.replace(/["\\]/g, "\\$&")}"]>table[grid]`;
  const tableId = table.getAttribute?.("id");
  return tableId ? `table[grid][id="${tableId.replace(/["\\]/g, "\\$&")}"]` : "table[grid]";
}

export function canAdopt(doc = document) {
  return "adoptedStyleSheets" in doc && typeof doc.defaultView?.CSSStyleSheet?.prototype?.replaceSync === "function";
}

const adoptSheet = (sheet) => {
  document.adoptedStyleSheets = [...document.adoptedStyleSheets, sheet];
};
const dropSheet = (sheet) => {
  document.adoptedStyleSheets = document.adoptedStyleSheets.filter((one) => one !== sheet);
};

// The nearest scrolling ancestor, or null for the window.
function scrollerOf(table) {
  for (let el = table.parentElement; el && el !== document.body && el !== document.documentElement; el = el.parentElement) {
    const { overflowY } = getComputedStyle(el);
    if (overflowY === "auto" || overflowY === "scroll") return el;
  }
  return null;
}

export function createRowWindow(table, {
  sheet = new CSSStyleSheet(),
  adopt = adoptSheet,
  drop = dropSheet,
  schedule = (fn) => requestAnimationFrame(fn),
  overscan = 1,
} = {}) {
  const scope = scopeOf(table);
  const scroller = scrollerOf(table);
  const target = scroller ?? window;
  const measured = new Map();
  const done = new Set();
  let geometry = createGeometry(table, { measured });
  let current = null;
  // The top margin the adopted rule gives the table right now. A refresh forgets the
  // window but not the rule, so bodyTop reads this, never the forgotten window.
  let appliedBefore = 0;
  let queued = false;
  let disposed = false;

  const view = () =>
    scroller
      ? { top: scroller.scrollTop, height: scroller.clientHeight, offset: scroller.getBoundingClientRect().top }
      : { top: window.scrollY, height: window.innerHeight, offset: 0 };

  const scrollBy = (delta) => {
    if (scroller) scroller.scrollTop += delta;
    else window.scrollBy(0, delta);
  };

  // Where the first group starts, in the scroller's content, with nothing hidden: the
  // table's box moved down by the margin the rule gave it, plus the frozen head.
  function bodyTop(v) {
    return table.getBoundingClientRect().top - v.offset + v.top - appliedBefore + (table.tHead?.offsetHeight ?? 0);
  }

  // Reads the height of every row in the shown groups that were not measured since they
  // last changed. A group with no layout (jsdom, a tab that is not shown) is left for
  // later. A row above the top edge that changed height moves the scroll position by the
  // same amount, so the rows on screen stay still. Returns whether anything changed.
  function measureShown(top) {
    let changed = false;
    let shift = 0;
    for (let g = current.first - 1; g <= current.last - 1; g++) {
      if (done.has(g) || geometry.group(g).offsetHeight === 0) continue;
      done.add(g);
      const [start, end] = geometry.groupRows(g);
      for (let i = start; i < end; i++) {
        if (rowOutOfSight(geometry.row(i))) continue;
        // Rows measured before this one moved it by `shift`, and the viewport with it.
        const above = geometry.rowTop(i) + geometry.rowHeight(i) <= top + shift;
        const delta = geometry.setRowHeight(i, geometry.row(i).offsetHeight);
        if (delta === 0) continue;
        changed = true;
        if (above) shift += delta;
      }
    }
    if (shift !== 0) scrollBy(shift);
    return changed;
  }

  function update(measure = true) {
    queued = false;
    if (disposed) return;
    const v = view();
    const top = v.top - bodyTop(v);
    const next = windowFor({ scrollTop: top, viewport: v.height, heights: geometry.groupHeights(), overscan });
    const moved = !current || next.first !== current.first || next.last !== current.last ||
      next.before !== current.before || next.after !== current.after;
    current = next;
    if (moved) {
      sheet.replaceSync(ruleText(next, scope, geometry.width, [...filteredRows(table)]));
      appliedBefore = next.before;
    }
    // Measuring can change the window once; the second pass does not measure again.
    if (measure && measureShown(top)) update(false);
  }

  const queue = () => {
    if (queued || disposed) return;
    queued = true;
    schedule(() => update());
  };

  // Rows added, removed or moved, or new groups: read the table again, keeping every
  // height measured so far.
  function refresh() {
    geometry = createGeometry(table, { measured });
    done.clear();
    current = null;
    queue();
  }

  // A Grid change event (sapjs `grid.on("change")`): structure reads the table again;
  // cell changes only mark their rows' groups to be measured again when next shown. A
  // column change or a whole new read can change any row's height (wrapping): the shown
  // groups are measured again, and the hidden ones keep their last heights, which are
  // closer than the estimate, so the rows on screen do not move.
  function follow(event) {
    const cols = event.cols;
    if (event.rescanned || event.structure || cols?.added?.length || cols?.removed?.length || cols?.changed?.length ||
      event.rows?.added?.length || event.rows?.removed?.length) return refresh();
    const ids = new Set([...(event.rows?.changed ?? []), ...(event.cells ?? []).map((cell) => cell.row)]);
    for (const id of ids) {
      const i = geometry.indexOf(id);
      if (i === -1) continue;
      measured.delete(id);
      done.delete(geometry.groupOfRow(i));
    }
    if (ids.size > 0) queue();
  }

  const onScroll = () => queue();
  target.addEventListener("scroll", onScroll, { passive: true });
  window.addEventListener("resize", onScroll);
  // The table's own box changes when its tab is shown: a hidden tab could not place or
  // measure its groups, and showing it fires no scroll.
  const resized = typeof ResizeObserver === "function" ? new ResizeObserver(onScroll) : null;
  resized?.observe(table);
  if (scroller) resized?.observe(scroller);

  adopt(sheet);
  update();

  return {
    table,
    get geometry() {
      return geometry;
    },
    get shown() {
      return current;
    },
    // Where cell (r, c) is on screen, in client coordinates, from the geometry: the rows the
    // window hides have no layout, so a box is never measured from the DOM. The table's top is
    // moved down by the margin the rule gives it, which stands for the hidden rows above.
    cellBox(r, c) {
      const box = table.getBoundingClientRect();
      const top = box.top - appliedBefore + (table.tHead?.offsetHeight ?? 0) + geometry.rowTop(r);
      return { left: box.left + geometry.colLeft(c), top, width: geometry.colWidth(c), height: geometry.rowHeight(r) };
    },
    // The row and column under a client point, or null outside the table's body.
    cellAt(x, y) {
      const box = table.getBoundingClientRect();
      const top = box.top - appliedBefore + (table.tHead?.offsetHeight ?? 0);
      if (x < box.left || x >= box.left + geometry.width || y < top || y >= top + geometry.total()) return null;
      return { r: geometry.rowAt(y - top), c: geometry.colAt(x - box.left) };
    },
    refresh,
    follow,
    dispose() {
      disposed = true;
      target.removeEventListener("scroll", onScroll);
      window.removeEventListener("resize", onScroll);
      resized?.disconnect();
      drop(sheet);
    },
  };
}
