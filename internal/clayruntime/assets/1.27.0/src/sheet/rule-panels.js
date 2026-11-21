import { a1, colIndex, rowIndex, tabTable } from "./commit.js";
import { isKey, keyBetween } from "./format-commands.js";
import { commandArea, ownerOf, selectionBox } from "./selection.js";
import { set } from "../lib/hostile-css.js";
import {
  bevelBox,
  bevelButton,
  bevelInput,
  bevelSurface,
  bevelText,
  protectIcon,
  setShown,
} from "../ui/bevel-controls.js";
import { FONT_MONO, FONT_SANS, RULES, TOKENS } from "../ui/bevel.js";

const CHECK_SVG =
  '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.2" stroke-linecap="square" stroke-linejoin="miter" aria-hidden="true"><path d="m5 12 4 4L19 6"/></svg>';

// Light fill tints from the sheet's Bevel palette (>= 11.9:1 contrast with sheet ink).
export const FILL_SWATCHES = [
  { hex: "#dcebe7", label: "Soft teal" },
  { hex: "#f0e4cc", label: "Soft brass" },
  { hex: "#f6ded6", label: "Soft ox" },
  { hex: "#e5f0f6", label: "Soft sky" },
  { hex: "#f1e8f3", label: "Soft plum" },
  { hex: "#eceff4", label: "Cool tint" },
];

// Text colours that keep >= 4.5:1 contrast on #fffcf6 and on every fill tint above.
export const TEXT_SWATCHES = [
  { hex: "#141821", label: "Ledger ink" },
  { hex: "#7a2a1f", label: "Deep ox" },
  { hex: "#7a520c", label: "Deep brass" },
  { hex: "#256056", label: "Teal" },
  { hex: "#1d5b79", label: "Ledger blue" },
  { hex: "#6e4076", label: "Plum" },
];

export const SCALE_SWATCHES = [
  { hex: "#fffcf6", label: "Warm white" },
  { hex: "#b7dfd5", label: "Green" },
  { hex: "#f0d9a8", label: "Amber" },
  { hex: "#f2c2b8", label: "Red" },
  { hex: "#bdd9ea", label: "Blue" },
];

export const SCALE_PRESETS = [
  { id: "white-green", label: "White to green", low: "#fffcf6", mid: "", high: "#b7dfd5" },
  { id: "red-white-green", label: "Red to white to green", low: "#f2c2b8", mid: "#fffcf6", high: "#b7dfd5" },
  { id: "white-red", label: "White to red", low: "#fffcf6", mid: "", high: "#f2c2b8" },
];

// Every conditional-format test except `formula`, in sentence case. "Text is exactly"
// saves as `eq` alongside "Is equal to".
export const WHEN_CONDITIONS = [
  { value: "empty", test: "empty", label: "Is empty", args: 0 },
  { value: "filled", test: "filled", label: "Is not empty", args: 0 },
  { value: "contains", test: "contains", label: "Text contains", args: 1 },
  { value: "starts", test: "starts", label: "Text starts with", args: 1 },
  { value: "ends", test: "ends", label: "Text ends with", args: 1 },
  { value: "text_eq", test: "eq", label: "Text is exactly", args: 1 },
  { value: "gt", test: "gt", label: "Greater than", args: 1 },
  { value: "ge", test: "ge", label: "Greater than or equal", args: 1 },
  { value: "lt", test: "lt", label: "Less than", args: 1 },
  { value: "le", test: "le", label: "Less than or equal", args: 1 },
  { value: "eq", test: "eq", label: "Is equal to", args: 1 },
  { value: "ne", test: "ne", label: "Is not equal to", args: 1 },
  { value: "between", test: "between", label: "Is between", args: 2 },
  { value: "outside", test: "outside", label: "Is not between", args: 2 },
  { value: "duplicate", test: "duplicate", label: "Duplicate values", args: 0 },
  { value: "unique", test: "unique", label: "Unique values", args: 0 },
];

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

export function canonRange(raw) {
  const clean = String(raw ?? "").trim().replace(/\$/g, "").toUpperCase();
  const parts = clean.split(":");
  if (parts.length === 2 && parts[0] === parts[1] && /^[A-Z]+\d+$/.test(parts[0])) return parts[0];
  return clean;
}

export function mintRuleId(rules, newId) {
  const taken = new Set(rules.map((r) => r.id));
  for (let tries = 0; tries < 100; tries++) {
    const id = typeof newId === "function" ? newId("k") : `k${Date.now().toString(36)}${tries}`;
    if (id && !taken.has(id)) return id;
  }
  return `k${Date.now().toString(36)}`;
}

export function nextRulePos(rules) {
  let lastPos = null;
  for (const r of rules) {
    if (isKey(r.pos) && (lastPos === null || lastPos < r.pos)) lastPos = r.pos;
  }
  return keyBetween(lastPos, null);
}

// sapjs's cleanStyle orders declarations as font-weight, font-style, text-decoration, color, background-color
// with no spaces around `:` or `;`.
export function buildRuleStyle({ bold = false, italic = false, strikethrough = false, color = "", fill = "" } = {}) {
  const parts = [];
  if (bold) parts.push("font-weight:bold");
  if (italic) parts.push("font-style:italic");
  if (strikethrough) parts.push("text-decoration:line-through");
  if (color) parts.push(`color:${color.toLowerCase()}`);
  if (fill) parts.push(`background-color:${fill.toLowerCase()}`);
  return parts.join(";");
}

export function readRuleStyle(styleText) {
  const out = { bold: false, italic: false, strikethrough: false, color: "", fill: "" };
  for (const part of String(styleText ?? "").split(";")) {
    const colon = part.indexOf(":");
    if (colon === -1) continue;
    const key = part.slice(0, colon).trim().toLowerCase();
    const val = part.slice(colon + 1).trim().toLowerCase();
    if (key === "font-weight" && (val === "bold" || val === "bolder" || Number(val) >= 700)) out.bold = true;
    else if (key === "font-style" && (val === "italic" || val === "oblique")) out.italic = true;
    else if (key === "text-decoration" && /\bline-through\b/.test(val)) out.strikethrough = true;
    else if (key === "color") out.color = val;
    else if (key === "background-color") out.fill = val;
  }
  return out;
}

export function summarizeRule(rule) {
  if (rule.kind === "scale") return "Color scale";
  switch (rule.test) {
    case "empty":
      return "Is empty";
    case "filled":
      return "Is not empty";
    case "duplicate":
      return "Duplicate values";
    case "unique":
      return "Unique values";
    case "contains":
      return `Text contains ${rule.a ?? ""}`;
    case "starts":
      return `Text starts with ${rule.a ?? ""}`;
    case "ends":
      return `Text ends with ${rule.a ?? ""}`;
    case "gt":
      return `Greater than ${rule.a ?? ""}`;
    case "ge":
      return `Greater than or equal ${rule.a ?? ""}`;
    case "lt":
      return `Less than ${rule.a ?? ""}`;
    case "le":
      return `Less than or equal ${rule.a ?? ""}`;
    case "eq":
      return `Is equal to ${rule.a ?? ""}`;
    case "ne":
      return `Is not equal to ${rule.a ?? ""}`;
    case "between":
      return `Is between ${rule.a ?? ""} and ${rule.b ?? ""}`;
    case "outside":
      return `Is not between ${rule.a ?? ""} and ${rule.b ?? ""}`;
    case "formula":
      return `Formula ${rule.f ?? ""}`;
    default:
      return rule.test ?? "";
  }
}

function rectOf(ctx, anchor) {
  if (anchor && typeof anchor.getBoundingClientRect === "function") {
    const r = anchor.getBoundingClientRect();
    if (r && (r.width > 0 || r.height > 0 || r.left !== 0 || r.top !== 0)) return r;
  }
  if (anchor && typeof anchor.left === "number" && typeof anchor.top === "number") {
    const width = anchor.width ?? 0;
    const height = anchor.height ?? 0;
    return { left: anchor.left, top: anchor.top, right: anchor.left + width, bottom: anchor.top + height, width, height };
  }
  const sel = ctx.getSel();
  if (sel) {
    const table = tabTable(ctx.workbook, sel.tab);
    const view = ctx.viewOf(sel.tab);
    const box = selectionBox(view, sel);
    if (table && box) {
      const cb = ctx.overlays.cellBox(table, ctx.windowOf(table), box.active.r, box.active.c);
      if (cb) return { left: cb.left, top: cb.top, right: cb.left + cb.width, bottom: cb.top + cb.height, width: cb.width, height: cb.height };
    }
  }
  return { left: 16, top: 72, right: 136, bottom: 98, width: 120, height: 26 };
}

function placeSurface(win, panel, rect, targetWidth, maxCap = 480) {
  const vw = win?.innerWidth || 1024;
  const vh = win?.innerHeight || 768;
  const width = Math.min(targetWidth, Math.max(180, vw - 16));
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
  const active = panel.getRootNode()?.activeElement ?? doc.activeElement;
  const idx = nodes.indexOf(active);
  if (idx === -1) {
    event.preventDefault();
    nodes[0].focus();
  } else if (event.shiftKey && idx === 0) {
    event.preventDefault();
    nodes[nodes.length - 1].focus();
  } else if (!event.shiftKey && idx === nodes.length - 1) {
    event.preventDefault();
    nodes[0].focus();
  }
}

export function toggleCheckboxRule(ctx) {
  const sel = ctx.getSel();
  if (!sel || ctx.editor.editing) return false;
  const table = tabTable(ctx.workbook, sel.tab);
  const grid = ctx.gridOf(table);
  const view = ctx.viewOf(sel.tab);
  const box = selectionBox(view, sel);
  if (!grid || !box) return false;

  const area = commandArea(view, sel);
  const rules = grid.rules();
  const boxes = rules
    .filter((rule) => rule.kind === "check")
    .map((rule) => ({ rule, at: ctx.boundRange(rule.range, grid.size) }))
    .filter(({ at }) => at !== null);
  // The rule under each selected cell: a press beside it edits that rule, whatever its range.
  const covering = (r, c) => boxes.filter(({ at }) => r >= at.r0 && r <= at.r1 && c >= at.c0 && c <= at.c1);
  let everyCovered = true;
  const meets = new Set();
  for (let r = box.top; r <= box.bottom && everyCovered; r++) {
    for (let c = box.left; c <= box.right; c++) {
      const here = covering(r, c);
      if (here.length === 0) {
        everyCovered = false;
        break;
      }
      for (const { rule } of here) meets.add(rule);
    }
  }
  if (everyCovered && meets.size > 0) {
    const remove = [...meets].map((rule) => rule.id);
    if (!ctx.writeGrid(() => grid.setRules({ remove }))) return false;
    ctx.refreshStatus();
    ctx.syncUI({ announce: true });
    return true;
  }

  const emptyWrites = [];
  for (let r = box.top; r <= box.bottom; r++) {
    for (let c = box.left; c <= box.right; c++) {
      const cell = grid.cell(a1(r, c));
      if (!cell || cell.covered) continue;
      if (cell.content === null || cell.content === "") {
        emptyWrites.push([view.rows[r], view.cols[c], false]);
      }
    }
  }

  const rule = {
    id: mintRuleId(rules, ctx.win.Sap?.grid?.newId),
    pos: nextRulePos(rules),
    kind: "check",
    range: area,
  };

  const ok = ctx.writeGrid(() =>
    ctx.history.group(sel.tab, () => {
      if (emptyWrites.length > 0) {
        grid.dispatch({ v: 1, t: "set", sheet: sel.tab, cells: emptyWrites });
      }
      grid.setRules({ set: [rule] });
    }),
  );
  if (!ok) return false;
  ctx.refreshStatus();
  ctx.syncUI({ announce: true });
  return true;
}

export function openPickList(ctx, { tab, r, c }) {
  const doc = ctx.doc;
  const win = ctx.win;
  const table = tabTable(ctx.workbook, tab);
  const grid = ctx.gridOf(table);
  const view = ctx.viewOf(tab);
  if (!table || !grid || r < 0 || c < 0 || r >= view.rows.length || c >= view.cols.length) return null;

  const own = ownerOf(view, r, c);
  const rule = ctx.inputRuleAt(grid, own.r, own.c);
  if (rule?.kind !== "list") return null;

  const target = { tab, row: view.rows[own.r], col: view.cols[own.c] };
  const items = ctx.listItems(table, rule);
  const curText = (grid.cell(a1(own.r, own.c))?.text ?? "").trim();
  const curLower = curText.toLowerCase();

  const theme = ctx.chrome.theme;
  const panel = bevelSurface(
    "div",
    [
      "position:fixed",
      "display:flex",
      "flex-direction:column",
      "gap:2px",
      "padding:4px",
      "overflow-y:auto",
      "overscroll-behavior:contain",
      "z-index:2147483000",
      `color-scheme:${theme.scheme}`,
    ],
    { theme },
  );
  panel.setAttribute("role", "listbox");
  panel.setAttribute("aria-label", `Dropdown options for ${a1(own.r, own.c)}`);
  panel.setAttribute("data-pick-list", "");
  panel.tabIndex = -1;

  const optionEntries = [];
  let activeIndex = Math.max(0, items.findIndex((item) => curLower !== "" && item.toLowerCase() === curLower));
  let closed = false;
  let typeBuffer = "";
  let typeTimer = null;

  function paintActive(focusOption = false) {
    optionEntries.forEach(({ btn, baseExtra, stateExtra }, idx) => {
      const active = idx === activeIndex;
      btn.setAttribute("aria-selected", active ? "true" : "false");
      btn.tabIndex = active ? 0 : -1;
      stateExtra.splice(
        0,
        stateExtra.length,
        ...baseExtra,
        ...(active ? [`background:${TOKENS.sunk}`, `border:1px solid ${TOKENS["line-2"]}`] : []),
      );
      btn.pin({});
      if (active) {
        btn.scrollIntoView?.({ block: "nearest" });
        if (focusOption) btn.focus?.();
      }
    });
  }

  function pick(index) {
    if (closed || index < 0 || index >= items.length) return false;
    const chosen = items[index];
    const curTable = tabTable(ctx.workbook, target.tab);
    const rr = curTable ? rowIndex(curTable, target.row) : -1;
    const cc = curTable ? colIndex(curTable, target.col) : -1;
    const curGrid = ctx.gridOf(curTable);
    close();
    if (curGrid && rr !== -1 && cc !== -1) {
      const ref = a1(rr, cc);
      if (curGrid.input(ref) !== chosen) {
        if (ctx.writeGrid(() => curGrid.set(ref, chosen))) {
          ctx.refreshStatus();
          ctx.syncUI({ announce: true });
        }
      }
    }
    return true;
  }

  if (items.length === 0) {
    const empty = bevelText(
      "div",
      ["padding:6px 10px", `font:400 12.5px/1.35 ${FONT_SANS}`, `color:${TOKENS.muted}`],
      "No items",
      { theme, role: "mutedText" },
    );
    panel.append(empty);
  } else {
    items.forEach((item, idx) => {
      const isCurrent = curLower !== "" && item.toLowerCase() === curLower;
      const baseExtra = [
        "width:100%",
        "height:28px",
        "min-height:28px",
        "padding:0 8px",
        "justify-content:flex-start",
        `font:${isCurrent ? "600" : "400"} 12.5px/1.3 ${FONT_SANS}`,
        `color:${TOKENS.ink}`,
      ];
      const stateExtra = [...baseExtra];
      const btn = bevelButton("", {
        small: true,
        variant: "quiet",
        onClick: () => pick(idx),
        theme,
        extra: stateExtra,
        labelExtra: [
          "display:inline-flex",
          "align-items:center",
          "gap:6px",
          "width:100%",
          "overflow:hidden",
        ],
      });
      const span = btn.firstChild;
      span.textContent = "";
      const mark = bevelBox(
        "span",
        [
          "display:inline-grid",
          "place-items:center",
          "width:14px",
          "height:14px",
          "flex:none",
          `color:${isCurrent ? TOKENS.ink : "transparent"}`,
        ],
        { theme },
      );
      if (isCurrent) {
        mark.innerHTML = CHECK_SVG;
        protectIcon(mark.firstElementChild, 12);
        btn.setAttribute("data-current", "true");
      }
      const label = bevelText(
        "span",
        ["font:inherit", "color:inherit", "overflow:hidden", "text-overflow:ellipsis", "white-space:nowrap"],
        item,
        { theme, role: null },
      );
      span.append(mark, label);
      btn.setAttribute("role", "option");
      btn.setAttribute("data-pick-item", item);
      btn.addEventListener("pointerenter", () => {
        if (activeIndex !== idx) {
          activeIndex = idx;
          paintActive(false);
        }
      });
      optionEntries.push({ btn, baseExtra, stateExtra });
      panel.append(btn);
    });
  }

  let onDismissOutside = null;
  const handledEvents = new WeakSet();

  function close({ restoreFocus = true } = {}) {
    if (closed) return;
    closed = true;
    if (typeTimer !== null) win.clearTimeout(typeTimer);
    doc.removeEventListener("mousedown", onOutside, true);
    doc.removeEventListener("pointerdown", onOutside, true);
    doc.removeEventListener("keydown", onDocKey, true);
    panel.remove();
    if (restoreFocus) ctx.editor?.textarea?.focus?.();
  }

  function onOutside(event) {
    const path = event.composedPath?.() ?? [event.target];
    if (path.includes(panel)) return;
    onDismissOutside?.(target);
    close({ restoreFocus: false });
  }

  function jumpToLetter(ch) {
    if (items.length === 0) return;
    if (typeTimer !== null) win.clearTimeout(typeTimer);
    typeBuffer += ch.toLowerCase();
    typeTimer = win.setTimeout(() => {
      typeBuffer = "";
      typeTimer = null;
    }, 500);
    const sameChar = typeBuffer.length > 1 && [...typeBuffer].every((c) => c === typeBuffer[0]);
    const query = sameChar ? typeBuffer[0] : typeBuffer;
    const start = query.length === 1 ? (activeIndex + 1) % items.length : activeIndex;
    for (let i = 0; i < items.length; i++) {
      const idx = (start + i) % items.length;
      if (items[idx].toLowerCase().startsWith(query)) {
        activeIndex = idx;
        paintActive(true);
        return;
      }
    }
  }

  function handleKeyDown(event) {
    if (closed || handledEvents.has(event)) return false;
    handledEvents.add(event);
    const key = event.key;
    if (key === "Escape") {
      event.preventDefault();
      event.stopPropagation();
      close({ restoreFocus: true });
      return true;
    }
    // Tab leaves the list rather than walking its options: the list is a detour, not a trap.
    if (key === "Tab") {
      event.preventDefault();
      event.stopPropagation();
      close({ restoreFocus: true });
      return true;
    }
    if (items.length === 0) return false;
    if (key === "ArrowDown") {
      event.preventDefault();
      event.stopPropagation();
      activeIndex = (activeIndex + 1) % items.length;
      paintActive(true);
      return true;
    }
    if (key === "ArrowUp") {
      event.preventDefault();
      event.stopPropagation();
      activeIndex = (activeIndex - 1 + items.length) % items.length;
      paintActive(true);
      return true;
    }
    if (key === "Home") {
      event.preventDefault();
      event.stopPropagation();
      activeIndex = 0;
      paintActive(true);
      return true;
    }
    if (key === "End") {
      event.preventDefault();
      event.stopPropagation();
      activeIndex = items.length - 1;
      paintActive(true);
      return true;
    }
    if (key === "Enter" || key === " ") {
      event.preventDefault();
      event.stopPropagation();
      pick(activeIndex);
      return true;
    }
    if (key.length === 1 && !event.ctrlKey && !event.metaKey && !event.altKey) {
      event.preventDefault();
      event.stopPropagation();
      jumpToLetter(key);
      return true;
    }
    return false;
  }

  function onDocKey(event) {
    handleKeyDown(event);
  }

  ctx.chrome.topRoot.append(panel);

  const tl = ctx.overlays.cellBox(table, ctx.windowOf(table), own.r, own.c);
  const br = ctx.overlays.cellBox(table, ctx.windowOf(table), own.r + own.rows - 1, own.c + own.cols - 1);
  const cellEl = table.rows[own.r]?.cells?.[own.c] ?? null;
  const domRect = cellEl?.getBoundingClientRect?.();
  const rect =
    domRect && domRect.width > 0
      ? domRect
      : tl && br
        ? {
            left: tl.left,
            top: tl.top,
            right: br.left + br.width,
            bottom: br.top + br.height,
            width: br.left + br.width - tl.left,
            height: br.top + br.height - tl.top,
          }
        : rectOf(ctx, null);

  // At most ~8 rows shown (8 * 28px + gaps + padding = 246px) before scrolling.
  placeSurface(win, panel, rect, Math.min(300, Math.max(rect.width || 140, 140)), 246);
  paintActive(true);
  if (optionEntries.length === 0) panel.focus();

  doc.addEventListener("mousedown", onOutside, true);
  doc.addEventListener("pointerdown", onOutside, true);
  doc.addEventListener("keydown", onDocKey, true);

  return {
    tab,
    key: `${tab}:${target.row}:${target.col}`,
    close: () => close({ restoreFocus: false }),
    handleKeyDown,
    setOnDismissOutside(fn) {
      onDismissOutside = fn;
    },
  };
}

export function openDropdownPanel(ctx, anchor = null) {
  const sel = ctx.getSel();
  if (!sel || ctx.editor.editing) return null;
  const tabId = sel.tab;
  const table = tabTable(ctx.workbook, tabId);
  const grid = ctx.gridOf(table);
  const view = ctx.viewOf(tabId);
  const box = selectionBox(view, sel);
  if (!table || !grid || !box) return null;

  const doc = ctx.doc;
  const win = ctx.win;
  const theme = ctx.chrome.theme;
  const area = commandArea(view, sel);
  // The dropdown the selection sits in: one cell inside C1:C3 opens that rule, not a second one
  // over the one cell.
  const covering = ctx.inputRuleAt(grid, box.active.r, box.active.c);
  const existing = covering?.kind === "list" ? covering : null;
  const shownRange = existing ? existing.range : area;

  const panel = bevelSurface("div", [...PANEL_CSS, "width:min(300px, 100vw - 16px)", `color-scheme:${theme.scheme}`], { theme });
  panel.setAttribute("role", "dialog");
  panel.setAttribute("aria-label", "Dropdown");
  panel.setAttribute("data-dropdown-panel", "");

  const title = bevelText("div", [`font:600 13px/1.35 ${FONT_SANS}`], "Dropdown", { theme });

  const rangeRow = bevelBox(
    "div",
    [
      "display:flex",
      "align-items:center",
      "justify-content:space-between",
      "gap:8px",
      "padding:5px 8px",
      `background:${TOKENS.sunk}`,
      `border:1px solid ${TOKENS["line-2"]}`,
    ],
    { theme },
  );
  const rangeLabel = bevelText("span", [`font:500 12px/1.3 ${FONT_SANS}`, `color:${TOKENS.muted}`], "Range", {
    theme,
    role: "mutedText",
  });
  const rangeVal = bevelText("span", [`font:600 12px/1.3 ${FONT_MONO}`, `color:${TOKENS.ink}`], shownRange, { theme });
  rangeVal.setAttribute("data-dropdown-range", "");
  rangeRow.append(rangeLabel, rangeVal);

  const modeSelect = bevelInput("select", { theme, rules: ["appearance:auto", "cursor:pointer"] });
  modeSelect.setAttribute("aria-label", "Dropdown source");
  modeSelect.setAttribute("data-dropdown-mode", "");
  for (const [val, label] of [
    ["items", "Items"],
    ["source", "Values from a range"],
  ]) {
    const opt = doc.createElement("option");
    opt.value = val;
    opt.textContent = label;
    modeSelect.append(opt);
  }
  modeSelect.value = existing?.source !== undefined ? "source" : "items";

  const itemsWrap = bevelBox("div", ["display:flex", "flex-direction:column", "gap:4px"], { theme });
  const itemsHint = bevelText("span", [`font:500 11.5px/1.3 ${FONT_SANS}`, `color:${TOKENS.muted}`], "One item per line", {
    theme,
    role: "mutedText",
  });
  const itemsInput = bevelInput("textarea", {
    theme,
    rules: ["min-height:88px", "height:88px", "block-size:88px", "padding:6px 10px", "line-height:1.4", "resize:vertical"],
  });
  itemsInput.setAttribute("aria-label", "Items");
  itemsInput.setAttribute("data-dropdown-items", "");
  itemsInput.value = Array.isArray(existing?.items) ? existing.items.join("\n") : "";
  itemsWrap.append(itemsHint, itemsInput);

  const sourceWrap = bevelBox("div", ["display:flex", "flex-direction:column", "gap:4px"], { theme });
  const sourceHint = bevelText("span", [`font:500 11.5px/1.3 ${FONT_SANS}`, `color:${TOKENS.muted}`], "Range (for example Name!A2:A)", {
    theme,
    role: "mutedText",
  });
  const sourceInput = bevelInput("input", { theme, rules: [`font-family:${FONT_MONO}`] });
  sourceInput.setAttribute("aria-label", "Values from a range");
  sourceInput.setAttribute("data-dropdown-source", "");
  sourceInput.placeholder = "Name!A2:A";
  sourceInput.spellcheck = false;
  sourceInput.value = existing?.source ?? "";
  sourceWrap.append(sourceHint, sourceInput);

  const syncMode = () => {
    const isItems = modeSelect.value === "items";
    setShown(itemsWrap, isItems, "flex");
    setShown(sourceWrap, !isItems, "flex");
  };
  modeSelect.addEventListener("change", syncMode);
  syncMode();

  const openRow = bevelBox(
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
  const openCheck = doc.createElement("input");
  openCheck.type = "checkbox";
  openCheck.setAttribute("role", "switch");
  openCheck.setAttribute("data-dropdown-open", "");
  openCheck.checked = Boolean(existing?.open);
  openCheck.setAttribute("aria-checked", openCheck.checked ? "true" : "false");
  set(openCheck, "margin", "0");
  set(openCheck, "width", "14px");
  set(openCheck, "height", "14px");
  set(openCheck, "accent-color", TOKENS.brass);
  set(openCheck, "cursor", "pointer");
  openCheck.addEventListener("change", () => {
    openCheck.setAttribute("aria-checked", openCheck.checked ? "true" : "false");
  });
  const openLabel = bevelText("span", ["font:inherit", "color:inherit"], "Allow other values", { theme, role: null });
  openRow.append(openCheck, openLabel);

  const footer = bevelBox("div", ["display:flex", "align-items:center", "justify-content:flex-end", "gap:6px", "flex-wrap:wrap"], {
    theme,
  });
  if (existing) {
    const removeBtn = bevelButton("Remove dropdown", {
      small: true,
      variant: "danger",
      theme,
      extra: ["margin-inline-end:auto"],
      onClick: () => removeDropdown(),
    });
    removeBtn.setAttribute("data-dropdown-remove", "");
    footer.append(removeBtn);
  }
  const cancelBtn = bevelButton("Cancel", { small: true, theme, onClick: () => close(true) });
  const doneBtn = bevelButton("Done", { small: true, variant: "primary", theme, onClick: () => apply() });
  cancelBtn.setAttribute("data-dropdown-cancel", "");
  doneBtn.setAttribute("data-dropdown-done", "");
  footer.append(cancelBtn, doneBtn);

  panel.append(title, rangeRow, modeSelect, itemsWrap, sourceWrap, openRow, footer);
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
    return ctx.gridOf(tabTable(ctx.workbook, tabId));
  }

  function removeDropdown() {
    const g = currentGrid();
    if (!g || !existing) return close(true);
    if (!ctx.writeGrid(() => g.setRules({ remove: [existing.id] }))) return;
    ctx.refreshStatus();
    ctx.syncUI({ announce: true });
    close(true);
  }

  function apply() {
    const g = currentGrid();
    if (!g) return close(true);
    const rules = g.rules();
    const nextRule = {
      id: existing ? existing.id : mintRuleId(rules, ctx.win.Sap?.grid?.newId),
      pos: existing ? existing.pos : nextRulePos(rules),
      kind: "list",
      range: shownRange,
    };
    if (modeSelect.value === "source") {
      const source = sourceInput.value.trim();
      if (!source) {
        ctx.chrome.showNotice("Enter a range for the values.");
        sourceInput.focus();
        return;
      }
      nextRule.source = source;
    } else {
      const items = itemsInput.value
        .replace(/\r\n?/g, "\n")
        .split("\n")
        .map((line) => line.trim())
        .filter(Boolean);
      if (items.length === 0) {
        ctx.chrome.showNotice("Add at least one item.");
        itemsInput.focus();
        return;
      }
      nextRule.items = items;
    }
    if (openCheck.checked) nextRule.open = true;

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
    if (event.key === "Tab") trapTab(panel, doc, event);
  }

  placeSurface(win, panel, rectOf(ctx, anchor), 300, 440);
  doc.addEventListener("mousedown", onOutside, true);
  doc.addEventListener("pointerdown", onOutside, true);
  doc.addEventListener("keydown", onKey, true);

  (modeSelect.value === "source" ? sourceInput : itemsInput).focus();

  return {
    tab: tabId,
    close: () => close(false),
  };
}

function makeSwatchButton(theme, { hex, label, selected, attr, onClick }) {
  const isNone = !hex;
  const btn = bevelButton(isNone ? "–" : "", {
    small: true,
    variant: isNone ? "default" : "quiet",
    onClick,
    theme,
    extra: [
      "width:22px",
      "min-width:22px",
      "height:22px",
      "min-height:22px",
      "padding:0",
      ...(isNone ? [`font:600 11px/1 ${FONT_SANS}`, `color:${TOKENS.muted}`] : [`background:${hex}`]),
    ],
  });
  btn.setAttribute("aria-label", label);
  btn.title = label;
  btn.setAttribute(attr, isNone ? "none" : hex);
  const paint = (on) => {
    btn.setAttribute("aria-pressed", on ? "true" : "false");
    btn.pin({
      border: on ? `2px solid ${TOKENS.brass}` : `1px solid ${TOKENS["line-3"]}`,
    });
  };
  paint(selected);
  btn.setSelected = paint;
  return btn;
}

function makeStyleToggle(label, ariaLabel, attr, initial, theme, extraRules = [], onToggle = null) {
  let pressed = Boolean(initial);
  const baseExtra = ["width:28px", "min-width:28px", "height:26px", "min-height:26px", "padding:0", ...extraRules];
  const stateExtra = [...baseExtra, ...(pressed ? RULES.recess : [])];
  const btn = bevelButton(label, {
    small: true,
    theme,
    extra: stateExtra,
    onClick: () => {
      pressed = !pressed;
      sync();
      onToggle?.(pressed);
    },
  });
  btn.setAttribute("aria-label", ariaLabel);
  btn.title = ariaLabel;
  btn.setAttribute(attr, "");
  function sync() {
    btn.setAttribute("aria-pressed", pressed ? "true" : "false");
    stateExtra.splice(0, stateExtra.length, ...baseExtra, ...(pressed ? RULES.recess : []));
    btn.pin({});
  }
  sync();
  btn.isPressed = () => pressed;
  return btn;
}

export function openConditionalPanel(ctx, anchor = null) {
  const sel = ctx.getSel();
  if (!sel || ctx.editor.editing) return null;
  const tabId = sel.tab;
  const table = tabTable(ctx.workbook, tabId);
  const grid = ctx.gridOf(table);
  if (!table || !grid) return null;

  const doc = ctx.doc;
  const win = ctx.win;
  const theme = ctx.chrome.theme;
  const anchorRect = rectOf(ctx, anchor);

  const panel = bevelSurface("div", [...PANEL_CSS, "width:min(320px, 100vw - 16px)", `color-scheme:${theme.scheme}`], { theme });
  panel.setAttribute("role", "dialog");
  panel.setAttribute("aria-label", "Conditional formatting");
  panel.setAttribute("data-conditional-panel", "");

  let closed = false;

  const currentGrid = () => ctx.gridOf(tabTable(ctx.workbook, tabId));
  const condRules = () => (currentGrid()?.rules() ?? []).filter((r) => r.kind === "when" || r.kind === "scale");
  const defaultArea = () => {
    const curSel = ctx.getSel();
    if (!curSel || curSel.tab !== tabId) return "A1";
    return commandArea(ctx.viewOf(tabId), curSel);
  };

  function close(restoreFocus = true) {
    if (closed) return;
    closed = true;
    doc.removeEventListener("mousedown", onOutside, true);
    doc.removeEventListener("pointerdown", onOutside, true);
    doc.removeEventListener("keydown", onKey, true);
    panel.remove();
    if (restoreFocus) ctx.editor?.textarea?.focus?.();
  }

  function renderList() {
    panel.textContent = "";
    const title = bevelText("div", [`font:600 13px/1.35 ${FONT_SANS}`], "Conditional formatting", { theme });
    const list = bevelBox("div", ["display:flex", "flex-direction:column", "gap:6px", "max-height:240px", "overflow-y:auto"], {
      theme,
    });
    list.setAttribute("data-cond-list", "");

    const rules = condRules();
    if (rules.length === 0) {
      const empty = bevelText(
        "div",
        ["padding:8px 2px", `font:400 12.5px/1.4 ${FONT_SANS}`, `color:${TOKENS.muted}`],
        "No conditional formatting rules on this sheet.",
        { theme, role: "mutedText" },
      );
      list.append(empty);
    } else {
      for (const rule of rules) {
        const row = bevelBox(
          "div",
          [
            "display:flex",
            "align-items:center",
            "gap:8px",
            "padding:6px 8px",
            `background:${TOKENS.sunk}`,
            `border:1px solid ${TOKENS["line-2"]}`,
          ],
          { theme },
        );
        row.setAttribute("data-cond-rule", rule.id);

        // A formula rule is the person's own formula: the panel names it and can drop it, and
        // never rewrites it as one of its tests.
        const custom = rule.kind === "when" && rule.test === "formula";

        const sample = makeRuleBadge(rule, theme);
        const info = bevelBox("div", ["display:flex", "flex-direction:column", "gap:1px", "flex:1 1 auto", "min-width:0"], { theme });
        const summary = bevelText(
          "div",
          [`font:500 12.5px/1.3 ${FONT_SANS}`, `color:${TOKENS.ink}`, "overflow:hidden", "text-overflow:ellipsis", "white-space:nowrap"],
          custom ? "Custom formula" : summarizeRule(rule),
          { theme },
        );
        summary.setAttribute("data-cond-summary", "");
        const rangeText = bevelText(
          "div",
          [`font:500 11px/1.25 ${FONT_MONO}`, `color:${TOKENS.muted}`],
          custom ? (rule.f ?? "") : rule.range,
          { theme, role: "mutedText" },
        );
        if (custom) rangeText.setAttribute("data-cond-formula", "");
        info.append(summary, rangeText);

        const editBtn = custom ? null : bevelButton("Edit", {
          small: true,
          variant: "quiet",
          theme,
          onClick: () => renderEditor(rule),
        });
        editBtn?.setAttribute("data-cond-edit", rule.id);

        const delBtn = bevelButton("Delete", {
          small: true,
          variant: "quiet",
          theme,
          extra: [`color:${TOKENS.ox}`],
          onClick: () => {
            const g = currentGrid();
            if (!g) return;
            if (!ctx.writeGrid(() => g.setRules({ remove: [rule.id] }))) return;
            ctx.refreshStatus();
            ctx.syncUI({ announce: true });
            renderList();
          },
        });
        delBtn.setAttribute("data-cond-delete", rule.id);

        row.append(sample, info, ...(editBtn ? [editBtn] : []), delBtn);
        list.append(row);
      }
    }

    const footer = bevelBox("div", ["display:flex", "justify-content:space-between", "align-items:center", "gap:6px"], { theme });
    const addBtn = bevelButton("Add rule", {
      small: true,
      variant: "primary",
      theme,
      onClick: () => renderEditor(null),
    });
    addBtn.setAttribute("data-cond-add", "");
    const closeBtn = bevelButton("Close", {
      small: true,
      theme,
      onClick: () => close(true),
    });
    closeBtn.setAttribute("data-cond-close", "");
    footer.append(addBtn, closeBtn);

    panel.append(title, list, footer);
    placeSurface(win, panel, anchorRect, 320, 480);
    addBtn.focus();
  }

  // A colour a rule's style may name: one the stylesheet parser keeps, never a url() or a value
  // that would end the declaration.
  const safeColor = (value, fallback) =>
    typeof value === "string" && ctx.parseStyle(`background-color:${value}`).length > 0 ? value : fallback;

  function makeRuleBadge(rule, currentTheme) {
    if (rule.kind === "scale") {
      const low = safeColor(rule.low, TOKENS.surface);
      const high = safeColor(rule.high, TOKENS.surface);
      const stops = rule.mid ? `${low}, ${safeColor(rule.mid, low)}, ${high}` : `${low}, ${high}`;
      return bevelBox(
        "span",
        [
          "display:inline-block",
          "width:26px",
          "height:20px",
          "flex:none",
          `background:linear-gradient(90deg, ${stops})`,
          `border:1px solid ${TOKENS["line-3"]}`,
        ],
        { theme: currentTheme },
      );
    }
    const props = new Map(ctx.parseStyle(rule.style));
    const weight = props.get("font-weight") ?? "";
    const bold = /^(?:bold|bolder)$/i.test(weight) || Number(weight) >= 700;
    const italic = /^(?:italic|oblique)$/i.test(props.get("font-style") ?? "");
    const strike = /\bline-through\b/i.test(props.get("text-decoration") ?? "");
    return bevelText(
      "span",
      [
        "display:inline-grid",
        "place-items:center",
        "width:26px",
        "height:20px",
        "flex:none",
        `font:${italic ? "italic " : ""}${bold ? "700" : "500"} 11px/1 ${FONT_SANS}`,
        ...(strike ? ["text-decoration:line-through"] : []),
        `color:${safeColor(props.get("color"), TOKENS.ink)}`,
        `background:${safeColor(props.get("background-color"), TOKENS.surface)}`,
        `border:1px solid ${TOKENS["line-3"]}`,
      ],
      "Aa",
      { theme: currentTheme, role: null },
    );
  }

  function renderEditor(editingRule) {
    panel.textContent = "";
    const heading = bevelText(
      "div",
      [`font:600 13px/1.35 ${FONT_SANS}`],
      editingRule ? "Edit rule" : "Add rule",
      { theme },
    );

    const rangeField = bevelBox("label", ["display:flex", "flex-direction:column", "gap:4px"], { theme });
    rangeField.append(
      bevelText("span", [`font:500 11.5px/1.3 ${FONT_SANS}`, `color:${TOKENS.muted}`], "Range", { theme, role: "mutedText" }),
    );
    const rangeInput = bevelInput("input", { theme, rules: [`font-family:${FONT_MONO}`] });
    rangeInput.setAttribute("aria-label", "Range");
    rangeInput.setAttribute("data-cond-range", "");
    rangeInput.spellcheck = false;
    rangeInput.value = editingRule ? editingRule.range : defaultArea();
    rangeField.append(rangeInput);

    const kindSelect = bevelInput("select", { theme, rules: ["appearance:auto", "cursor:pointer"] });
    kindSelect.setAttribute("aria-label", "Rule kind");
    kindSelect.setAttribute("data-cond-kind", "");
    for (const [val, label] of [
      ["when", "Single color"],
      ["scale", "Color scale"],
    ]) {
      const opt = doc.createElement("option");
      opt.value = val;
      opt.textContent = label;
      kindSelect.append(opt);
    }
    kindSelect.value = editingRule?.kind === "scale" ? "scale" : "when";

    // Single color section
    const whenBox = bevelBox("div", ["display:flex", "flex-direction:column", "gap:8px"], { theme });
    const testSelect = bevelInput("select", { theme, rules: ["appearance:auto", "cursor:pointer"] });
    testSelect.setAttribute("aria-label", "Condition");
    testSelect.setAttribute("data-cond-test", "");
    for (const entry of WHEN_CONDITIONS) {
      const opt = doc.createElement("option");
      opt.value = entry.value;
      opt.textContent = entry.label;
      testSelect.append(opt);
    }
    const initTest = editingRule?.kind === "when" && WHEN_CONDITIONS.some((e) => e.value === editingRule.test)
      ? editingRule.test
      : "filled";
    testSelect.value = initTest;

    const valA = bevelInput("input", { theme });
    valA.setAttribute("aria-label", "Value");
    valA.setAttribute("data-cond-a", "");
    valA.placeholder = "Value";
    valA.value = editingRule?.kind === "when" ? (editingRule.a ?? "") : "";

    const valB = bevelInput("input", { theme });
    valB.setAttribute("aria-label", "Second value");
    valB.setAttribute("data-cond-b", "");
    valB.placeholder = "And";
    valB.value = editingRule?.kind === "when" ? (editingRule.b ?? "") : "";

    const syncArgs = () => {
      const found = WHEN_CONDITIONS.find((e) => e.value === testSelect.value) ?? WHEN_CONDITIONS[0];
      setShown(valA, found.args >= 1, "block");
      setShown(valB, found.args >= 2, "block");
    };
    testSelect.addEventListener("change", syncArgs);
    syncArgs();

    const parsedStyle = editingRule?.kind === "when" ? readRuleStyle(editingRule.style) : readRuleStyle("background-color:#dcebe7");
    let chosenFill = parsedStyle.fill;
    let chosenColor = parsedStyle.color;

    const fillLabel = bevelText("div", [`font:500 11.5px/1.3 ${FONT_SANS}`, `color:${TOKENS.muted}`], "Fill color", {
      theme,
      role: "mutedText",
    });
    const fillRow = bevelBox("div", ["display:flex", "align-items:center", "gap:4px", "flex-wrap:wrap"], { theme });
    const fillBtns = [];
    const selectFill = (hex) => {
      chosenFill = hex;
      for (const item of fillBtns) item.btn.setSelected(item.hex === chosenFill);
    };
    for (const sw of [{ hex: "", label: "No fill" }, ...FILL_SWATCHES]) {
      const btn = makeSwatchButton(theme, {
        hex: sw.hex,
        label: sw.label,
        selected: sw.hex === chosenFill,
        attr: "data-cond-fill",
        onClick: () => selectFill(sw.hex),
      });
      fillBtns.push({ hex: sw.hex, btn });
      fillRow.append(btn);
    }

    const colorLabel = bevelText("div", [`font:500 11.5px/1.3 ${FONT_SANS}`, `color:${TOKENS.muted}`], "Text color", {
      theme,
      role: "mutedText",
    });
    const colorRow = bevelBox("div", ["display:flex", "align-items:center", "gap:4px", "flex-wrap:wrap"], { theme });
    const colorBtns = [];
    const selectColor = (hex) => {
      chosenColor = hex;
      for (const item of colorBtns) item.btn.setSelected(item.hex === chosenColor);
    };
    for (const sw of [{ hex: "", label: "Default text color" }, ...TEXT_SWATCHES]) {
      const btn = makeSwatchButton(theme, {
        hex: sw.hex,
        label: sw.label,
        selected: sw.hex === chosenColor,
        attr: "data-cond-color",
        onClick: () => selectColor(sw.hex),
      });
      colorBtns.push({ hex: sw.hex, btn });
      colorRow.append(btn);
    }

    const togglesRow = bevelBox("div", ["display:flex", "align-items:center", "gap:6px"], { theme });
    const boldBtn = makeStyleToggle("B", "Bold", "data-cond-bold", parsedStyle.bold, theme, ["font-weight:700"]);
    const italicBtn = makeStyleToggle("I", "Italic", "data-cond-italic", parsedStyle.italic, theme, ["font-style:italic"]);
    const strikeBtn = makeStyleToggle("S", "Strikethrough", "data-cond-strike", parsedStyle.strikethrough, theme, [
      "text-decoration:line-through",
    ]);
    togglesRow.append(boldBtn, italicBtn, strikeBtn);

    whenBox.append(testSelect, valA, valB, fillLabel, fillRow, colorLabel, colorRow, togglesRow);

    // Color scale section
    const scaleBox = bevelBox("div", ["display:flex", "flex-direction:column", "gap:8px"], { theme });
    let lowHex = editingRule?.kind === "scale" ? editingRule.low : SCALE_PRESETS[0].low;
    let midHex = editingRule?.kind === "scale" ? (editingRule.mid ?? "") : SCALE_PRESETS[0].mid;
    let highHex = editingRule?.kind === "scale" ? editingRule.high : SCALE_PRESETS[0].high;

    const presetLabel = bevelText("div", [`font:500 11.5px/1.3 ${FONT_SANS}`, `color:${TOKENS.muted}`], "Preset scale", {
      theme,
      role: "mutedText",
    });
    const presetRow = bevelBox("div", ["display:flex", "flex-direction:column", "gap:4px"], { theme });
    const lowBtns = [];
    const midBtns = [];
    const highBtns = [];

    const syncScaleSwatches = () => {
      for (const item of lowBtns) item.btn.setSelected(item.hex === lowHex);
      for (const item of midBtns) item.btn.setSelected(item.hex === midHex);
      for (const item of highBtns) item.btn.setSelected(item.hex === highHex);
    };

    for (const preset of SCALE_PRESETS) {
      const btn = bevelButton(preset.label, {
        small: true,
        variant: "quiet",
        theme,
        extra: ["justify-content:flex-start", "height:26px"],
        onClick: () => {
          lowHex = preset.low;
          midHex = preset.mid;
          highHex = preset.high;
          syncScaleSwatches();
        },
      });
      btn.setAttribute("data-cond-preset", preset.id);
      presetRow.append(btn);
    }

    const buildScaleStopRow = (titleText, attr, includeNone, getVal, setVal, bucket) => {
      const wrap = bevelBox("div", ["display:flex", "align-items:center", "justify-content:space-between", "gap:6px"], { theme });
      wrap.append(bevelText("span", [`font:500 11.5px/1.3 ${FONT_SANS}`, `color:${TOKENS.muted}`], titleText, { theme, role: "mutedText" }));
      const swatches = bevelBox("div", ["display:flex", "align-items:center", "gap:4px"], { theme });
      const opts = includeNone ? [{ hex: "", label: "No middle" }, ...SCALE_SWATCHES] : SCALE_SWATCHES;
      for (const sw of opts) {
        const btn = makeSwatchButton(theme, {
          hex: sw.hex,
          label: `${titleText}: ${sw.label}`,
          selected: sw.hex === getVal(),
          attr,
          onClick: () => {
            setVal(sw.hex);
            syncScaleSwatches();
          },
        });
        bucket.push({ hex: sw.hex, btn });
        swatches.append(btn);
      }
      wrap.append(swatches);
      return wrap;
    };

    scaleBox.append(
      presetLabel,
      presetRow,
      buildScaleStopRow("Low", "data-cond-low", false, () => lowHex, (v) => { lowHex = v; }, lowBtns),
      buildScaleStopRow("Middle", "data-cond-mid", true, () => midHex, (v) => { midHex = v; }, midBtns),
      buildScaleStopRow("High", "data-cond-high", false, () => highHex, (v) => { highHex = v; }, highBtns),
    );

    const syncKind = () => {
      const isScale = kindSelect.value === "scale";
      setShown(whenBox, !isScale, "flex");
      setShown(scaleBox, isScale, "flex");
    };
    kindSelect.addEventListener("change", syncKind);
    syncKind();

    const footer = bevelBox("div", ["display:flex", "justify-content:flex-end", "gap:6px"], { theme });
    const cancelBtn = bevelButton("Cancel", { small: true, theme, onClick: () => renderList() });
    const doneBtn = bevelButton("Done", { small: true, variant: "primary", theme, onClick: () => saveRule() });
    cancelBtn.setAttribute("data-cond-cancel", "");
    doneBtn.setAttribute("data-cond-done", "");
    footer.append(cancelBtn, doneBtn);

    panel.append(heading, rangeField, kindSelect, whenBox, scaleBox, footer);
    placeSurface(win, panel, anchorRect, 320, 520);
    rangeInput.focus();

    function saveRule() {
      const g = currentGrid();
      if (!g) return close(true);
      const rules = g.rules();
      const id = editingRule ? editingRule.id : mintRuleId(rules, ctx.win.Sap?.grid?.newId);
      const pos = editingRule ? editingRule.pos : nextRulePos(rules);
      const range = rangeInput.value.trim();

      let nextRule;
      if (kindSelect.value === "scale") {
        nextRule = { id, pos, kind: "scale", range, low: lowHex };
        if (midHex) nextRule.mid = midHex;
        nextRule.high = highHex;
      } else {
        const cond = WHEN_CONDITIONS.find((e) => e.value === testSelect.value) ?? WHEN_CONDITIONS[0];
        const styleText = buildRuleStyle({
          bold: boldBtn.isPressed(),
          italic: italicBtn.isPressed(),
          strikethrough: strikeBtn.isPressed(),
          color: chosenColor,
          fill: chosenFill,
        });
        if (styleText === "") {
          ctx.chrome.showNotice("Choose a fill, a text colour or a text style.");
          fillRow.scrollIntoView?.({ block: "nearest" });
          return;
        }
        const value = valA.value.trim();
        const second = valB.value.trim();
        if (cond.args >= 1 && value === "") {
          ctx.chrome.showNotice("Enter a value.");
          valA.focus();
          return;
        }
        if (cond.args >= 2 && (value === "" || second === "")) {
          ctx.chrome.showNotice("Enter both values.");
          (value === "" ? valA : valB).focus();
          return;
        }
        nextRule = { id, pos, kind: "when", range, test: cond.test };
        if (cond.args >= 1) nextRule.a = value;
        if (cond.args >= 2) nextRule.b = second;
        nextRule.style = styleText;
      }

      if (!ctx.writeGrid(() => g.setRules({ set: [nextRule] }))) return;
      ctx.refreshStatus();
      ctx.syncUI({ announce: true });
      renderList();
    }
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
    if (event.key === "Tab") trapTab(panel, doc, event);
  }

  ctx.chrome.topRoot.append(panel);
  doc.addEventListener("mousedown", onOutside, true);
  doc.addEventListener("pointerdown", onOutside, true);
  doc.addEventListener("keydown", onKey, true);
  renderList();

  return {
    tab: tabId,
    close: () => close(false),
  };
}
