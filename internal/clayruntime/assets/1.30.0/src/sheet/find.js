import { a1 } from "./commit.js";
import { keyIntent } from "./keys.js";
import { colIndexFromLetters } from "./refs.js";
import { createSelection, ownerOf, selectCell } from "./selection.js";
import {
  bevelBox,
  bevelButton,
  bevelIconButton,
  bevelInput,
  bevelText,
  setShown,
} from "../ui/bevel-controls.js";
import { FONT_MONO, FONT_SANS, TOKENS } from "../ui/bevel.js";

const PREV_SVG = '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.2" stroke-linecap="square" stroke-linejoin="miter" aria-hidden="true"><path d="m18 15-6-6-6 6"/></svg>';
const NEXT_SVG = '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.2" stroke-linecap="square" stroke-linejoin="miter" aria-hidden="true"><path d="m6 9 6 6 6-6"/></svg>';
const CLOSE_SVG = '<svg viewBox="0 0 256 256" fill="currentColor" aria-hidden="true"><path d="M205.66,194.34a8,8,0,0,1-11.32,11.32L128,139.31,61.66,205.66a8,8,0,0,1-11.32-11.32L116.69,128,50.34,61.66A8,8,0,0,1,61.66,50.34L128,116.69l66.34-66.35a8,8,0,0,1,11.32,11.32L139.31,128Z"/></svg>';

const BINARY = { "=": 1, "<>": 1, "<": 1, ">": 1, "<=": 1, ">=": 1, "&": 2, "+": 3, "-": 3, "*": 4, "/": 4, "^": 5 };
const UNARY = 6;
const POSTFIX = 7;

const isFormulaCell = (cell) =>
  Boolean(cell && cell.content !== null && typeof cell.content === "object" && typeof cell.content.f === "string");

export function replaceOccurrences(source, query, replacement) {
  const q = String(query ?? "");
  if (!q) return String(source ?? "");
  const escaped = q.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
  const repl = String(replacement ?? "");
  return String(source ?? "").replace(new RegExp(escaped, "gi"), () => repl);
}

export function isValidFormula(text, tokenize) {
  if (typeof text !== "string" || !text.startsWith("=") || text.length < 2) return false;
  if (/[\0\r]/.test(text)) return false;
  if (typeof tokenize !== "function") return true;
  const tokens = tokenize(text.slice(1));
  if (!Array.isArray(tokens) || tokens.length === 0) return false;
  if (tokens.some((t) => t.type === "invalid")) return false;

  let index = 0;
  const isCoord = (t) => t?.type === "cell" || t?.type === "col" || t?.type === "row";

  function parsePrefix() {
    const token = tokens[index++];
    if (!token) return false;
    switch (token.type) {
      case "num":
      case "str":
      case "bool":
      case "err":
        return true;
      case "cell":
      case "col":
      case "row": {
        const next = tokens[index];
        if (!next || next.type !== "punct" || next.value !== ":") return token.type === "cell";
        index += 1;
        const second = tokens[index++];
        if (!second || !isCoord(second)) return false;
        // sapjs's own rule (formula/text.js parseReference): a row pairs only with a row, and a
        // column cannot start a range that ends in a cell (A1:A is open-ended, A:A1 is not).
        if ((token.type === "row") !== (second.type === "row")) return false;
        if (token.type === "col" && second.type !== "col") return false;
        if (token.sheet && second.sheet && token.sheet !== second.sheet) return false;
        return true;
      }
      case "op":
        if (token.value !== "+" && token.value !== "-") return false;
        return parseExpr(UNARY);
      case "punct": {
        if (token.value !== "(") return false;
        if (!parseExpr(0)) return false;
        const close = tokens[index++];
        return Boolean(close && close.type === "punct" && close.value === ")");
      }
      case "name": {
        const open = tokens[index++];
        if (!open || open.type !== "punct" || open.value !== "(") return false;
        if (tokens[index]?.type === "punct" && tokens[index].value === ")") {
          index += 1;
          return true;
        }
        for (;;) {
          if (!parseExpr(0)) return false;
          const sep = tokens[index++];
          if (!sep || sep.type !== "punct") return false;
          if (sep.value === ")") return true;
          if (sep.value !== ",") return false;
        }
      }
      default:
        return false;
    }
  }

  function parseExpr(minPower) {
    if (!parsePrefix()) return false;
    for (;;) {
      const token = tokens[index];
      if (!token) break;
      if (token.type === "punct" && token.value === "%") {
        if (POSTFIX <= minPower) break;
        index += 1;
        continue;
      }
      if (token.type !== "op") break;
      const power = BINARY[token.value];
      if (power === undefined || power <= minPower) break;
      index += 1;
      if (!parseExpr(token.value === "^" ? power - 1 : power)) return false;
    }
    return true;
  }

  return parseExpr(0) && index === tokens.length;
}

export function replaceNoticeText(replaced, refused = 0) {
  const head = `Replaced ${replaced} ${replaced === 1 ? "cell" : "cells"}.`;
  if (refused <= 0) return head;
  return `${head} ${refused} ${refused === 1 ? "formula" : "formulas"} would not be valid and ${refused === 1 ? "was" : "were"} left alone.`;
}

function applyCellReplacement(grid, ref, { query, replacement, searchFormulas, tokenize }) {
  const cell = grid?.cell?.(ref);
  if (!cell || cell.covered) return "skipped";
  const needle = String(query ?? "").toLowerCase();
  if (!needle) return "skipped";

  if (isFormulaCell(cell)) {
    if (!searchFormulas) return "skipped";
    const cur = grid.input(ref);
    if (!cur.toLowerCase().includes(needle)) return "skipped";
    const next = replaceOccurrences(cur, query, replacement);
    if (!isValidFormula(next, tokenize)) return "refused";
    try {
      grid.set(ref, next);
    } catch (error) {
      if (error?.name !== "GridRefused") throw error;
      return "refused";
    }
    return "replaced";
  }

  const cur = grid.input(ref);
  if (!cur.toLowerCase().includes(needle)) return "skipped";
  const next = replaceOccurrences(cur, query, replacement);
  if (cell.effectiveFmt !== "@" && next.startsWith("=") && next.length > 1 && !isValidFormula(next, tokenize)) {
    return "refused";
  }
  try {
    grid.set(ref, next);
  } catch (error) {
    if (error?.name !== "GridRefused") throw error;
    return "refused";
  }
  return "replaced";
}

export function replaceAllOnTab(tab, { query, replacement, searchFormulas = false, history = null, runWrite = (fn) => fn(), tokenize } = {}) {
  const grid = tab?.grid;
  if (!grid || !query) return { replaced: 0, refused: 0 };
  const matches = findMatches([tab], { query, searchFormulas });
  let replaced = 0;
  let refused = 0;
  const exec = () => {
    for (const match of matches) {
      const outcome = applyCellReplacement(grid, match.ref, { query, replacement, searchFormulas, tokenize });
      if (outcome === "replaced") replaced += 1;
      else if (outcome === "refused") refused += 1;
    }
  };
  runWrite(() => {
    if (history?.group) history.group(tab.id, exec);
    else exec();
  });
  return { replaced, refused };
}

export function findMatches(tabs, { query, searchFormulas = false } = {}) {
  const raw = String(query ?? "");
  if (!raw) return [];
  const needle = raw.toLowerCase();
  const matches = [];

  for (const tab of tabs) {
    const size = tab.grid ? tab.grid.size : (tab.size ?? { rows: 0, cols: 0 });
    const rows = size.rows ?? 0;
    const cols = size.cols ?? 0;
    for (let r = 0; r < rows; r++) {
      if (tab.rowHidden?.(r)) continue;
      for (let c = 0; c < cols; c++) {
        const ref = a1(r, c);
        const cell = tab.grid ? tab.grid.cell(ref) : tab.cell?.(r, c);
        if (!cell || cell.covered) continue;
        const hasFormula = isFormulaCell(cell);
        const haystack = searchFormulas && hasFormula ? cell.content.f : (cell.text ?? "");
        if (haystack && haystack.toLowerCase().includes(needle)) {
          matches.push({ tab: tab.id, tabName: tab.name ?? "", r, c, ref, formula: hasFormula });
        }
      }
    }
  }
  return matches;
}

function splitSheetPrefix(text) {
  if (text.startsWith("'")) {
    let i = 1;
    let sheet = "";
    while (i < text.length) {
      if (text[i] === "'") {
        if (text[i + 1] === "'") {
          sheet += "'";
          i += 2;
          continue;
        }
        if (text[i + 1] !== "!") return null;
        return { sheet, coord: text.slice(i + 2) };
      }
      sheet += text[i];
      i += 1;
    }
    return null;
  }
  const bang = text.indexOf("!");
  if (bang === -1) return { sheet: null, coord: text };
  const sheet = text.slice(0, bang).trim();
  if (!sheet || sheet.includes("'")) return null;
  return { sheet, coord: text.slice(bang + 1) };
}

export function parseGoToRef(raw, tabs = [], activeTab = "") {
  const trimmed = String(raw ?? "").trim();
  if (!trimmed) return null;
  const parts = splitSheetPrefix(trimmed);
  if (!parts || !parts.coord) return null;

  let tab = activeTab;
  if (parts.sheet !== null) {
    const want = parts.sheet.toLowerCase();
    const found = tabs.find((t) => (t.name ?? "").toLowerCase() === want);
    if (!found) return null;
    tab = found.id;
  }

  const coord = parts.coord.trim();
  let m = /^\$?([A-Za-z]{1,3})\$?([1-9]\d*)$/.exec(coord);
  if (m) {
    const c = colIndexFromLetters(m[1]);
    const r = Number(m[2]) - 1;
    if (c < 0 || !Number.isSafeInteger(r)) return null;
    return { tab, r0: r, c0: c, r1: r, c1: c, whole: null };
  }

  m = /^\$?([A-Za-z]{1,3})\$?([1-9]\d*):\$?([A-Za-z]{1,3})\$?([1-9]\d*)$/.exec(coord);
  if (m) {
    const ca = colIndexFromLetters(m[1]);
    const cb = colIndexFromLetters(m[3]);
    const ra = Number(m[2]) - 1;
    const rb = Number(m[4]) - 1;
    if (ca < 0 || cb < 0 || !Number.isSafeInteger(ra) || !Number.isSafeInteger(rb)) return null;
    return {
      tab,
      r0: Math.min(ra, rb),
      c0: Math.min(ca, cb),
      r1: Math.max(ra, rb),
      c1: Math.max(ca, cb),
      whole: null,
    };
  }

  m = /^\$?([A-Za-z]{1,3}):\$?([A-Za-z]{1,3})$/.exec(coord);
  if (m) {
    const ca = colIndexFromLetters(m[1]);
    const cb = colIndexFromLetters(m[2]);
    if (ca < 0 || cb < 0) return null;
    return { tab, r0: 0, c0: Math.min(ca, cb), r1: null, c1: Math.max(ca, cb), whole: "cols" };
  }

  m = /^\$?([1-9]\d*):\$?([1-9]\d*)$/.exec(coord);
  if (m) {
    const ra = Number(m[1]) - 1;
    const rb = Number(m[2]) - 1;
    if (!Number.isSafeInteger(ra) || !Number.isSafeInteger(rb)) return null;
    return { tab, r0: Math.min(ra, rb), c0: 0, r1: Math.max(ra, rb), c1: null, whole: "rows" };
  }

  return null;
}

function compareMatchPos(tabOrder, a, b) {
  const ta = tabOrder.get(a.tab) ?? 0;
  const tb = tabOrder.get(b.tab) ?? 0;
  if (ta !== tb) return ta - tb;
  if (a.r !== b.r) return a.r - b.r;
  return a.c - b.c;
}

export function createFindBar({
  theme,
  container,
  getTabs,
  getActivePos,
  getActiveTab,
  history = null,
  runWrite = (fn) => {
    fn();
    return true;
  },
  getTokenize = () => globalThis.window?.Sap?.grid?.tokenize,
  onSelectMatch,
  onNotice,
  onAfterReplace,
  onBeforeAction = () => true,
  onClose,
}) {
  const doc = container.ownerDocument ?? document;
  let openState = false;
  let replaceOpen = false;
  let matches = [];
  let currentIndex = -1;
  let lastBuiltKey = "";
  const stateKey = () => `${input.value}\u0000${formulaCheck.checked}`;
  const activeTabOf = () => (typeof getActiveTab === "function" ? getActiveTab() : (getActivePos()?.tab ?? getTabs()[0]?.id ?? ""));

  const bar = bevelBox("div", [
    "display:none",
    "flex-direction:column",
    "gap:6px",
    "min-height:32px",
    "padding:4px 10px",
    `background:${TOKENS.surface}`,
    `border-bottom:1px solid ${TOKENS["line-2"]}`,
    `font:500 12.5px/1.3 ${FONT_SANS}`,
    `color:${TOKENS.ink}`,
  ], { theme });
  bar.setAttribute("role", "region");
  bar.setAttribute("aria-label", "Find in sheet");
  bar.setAttribute("data-sheet-find", "");
  bar.hidden = true;

  const findRow = bevelBox("div", ["display:flex", "align-items:center", "flex-wrap:wrap", "gap:8px"], { theme });

  const inputRules = [
    "width:196px",
    "max-width:100%",
    "height:24px",
    "block-size:24px",
    "min-block-size:24px",
    "padding:0 8px",
    `font:400 12.5px/22px ${FONT_SANS}`,
  ];
  const input = bevelInput("input", { theme, rules: inputRules });
  input.type = "text";
  input.placeholder = "Find";
  input.spellcheck = false;
  input.setAttribute("autocomplete", "off");
  input.setAttribute("aria-label", "Find");
  input.setAttribute("data-find-input", "");

  const formulaLabel = bevelBox("label", [
    "display:inline-flex",
    "align-items:center",
    "gap:5px",
    "cursor:pointer",
    "user-select:none",
    `font:500 12px/1.2 ${FONT_SANS}`,
    `color:${TOKENS["ink-2"]}`,
  ], { theme });
  const formulaCheck = doc.createElement("input");
  formulaCheck.type = "checkbox";
  formulaCheck.setAttribute("data-find-formulas", "");
  formulaCheck.style.cssText = `margin:0;width:13px;height:13px;accent-color:${TOKENS.brass};cursor:pointer`;
  const formulaText = bevelText("span", ["font:inherit", "color:inherit"], "Search formulas", { theme, role: null });
  formulaLabel.append(formulaCheck, formulaText);

  const countText = bevelText("span", [
    "min-width:56px",
    "text-align:right",
    `font:500 11.5px/1 ${FONT_MONO}`,
    "font-variant-numeric:tabular-nums",
    `color:${TOKENS.muted}`,
  ], "", { theme, role: "mutedText" });
  countText.setAttribute("data-find-count", "");

  const navGroup = bevelBox("div", ["display:inline-flex", "align-items:center", "gap:2px"], { theme });
  const prevBtn = bevelIconButton(PREV_SVG, { label: "Previous match", onClick: () => step(-1), theme });
  prevBtn.setAttribute("data-find-prev", "");
  prevBtn.addEventListener("mousedown", (e) => e.preventDefault());
  const nextBtn = bevelIconButton(NEXT_SVG, { label: "Next match", onClick: () => step(1), theme });
  nextBtn.setAttribute("data-find-next", "");
  nextBtn.addEventListener("mousedown", (e) => e.preventDefault());
  const closeBtn = bevelIconButton(CLOSE_SVG, { label: "Close find", onClick: () => close({ restoreFocus: true }), theme });
  closeBtn.setAttribute("data-find-close", "");
  closeBtn.addEventListener("mousedown", (e) => e.preventDefault());
  navGroup.append(prevBtn, nextBtn, closeBtn);
  findRow.append(input, formulaLabel, countText, navGroup);

  const replaceRow = bevelBox("div", ["display:none", "align-items:center", "flex-wrap:wrap", "gap:8px"], { theme });
  replaceRow.setAttribute("data-find-replace-row", "");
  replaceRow.hidden = true;

  const replaceInput = bevelInput("input", { theme, rules: inputRules });
  replaceInput.type = "text";
  replaceInput.placeholder = "Replace with";
  replaceInput.spellcheck = false;
  replaceInput.setAttribute("autocomplete", "off");
  replaceInput.setAttribute("aria-label", "Replace with");
  replaceInput.setAttribute("data-replace-input", "");

  const replaceBtn = bevelButton("Replace", {
    small: true,
    onClick: () => replaceCurrent(),
    theme,
    extra: ["height:24px", "padding:0 10px", "font-size:12px"],
  });
  replaceBtn.setAttribute("data-find-replace", "");
  replaceBtn.addEventListener("mousedown", (e) => e.preventDefault());

  const replaceAllBtn = bevelButton("Replace all", {
    small: true,
    onClick: () => replaceAllInActiveTab(),
    theme,
    extra: ["height:24px", "padding:0 10px", "font-size:12px"],
  });
  replaceAllBtn.setAttribute("data-find-replace-all", "");
  replaceAllBtn.addEventListener("mousedown", (e) => e.preventDefault());

  replaceRow.append(replaceInput, replaceBtn, replaceAllBtn);
  bar.append(findRow, replaceRow);
  container.append(bar);

  function syncReplaceControls() {
    if (!input.value || matches.length === 0) {
      replaceBtn.setDisabled(true);
      replaceAllBtn.setLabel("Replace all");
      replaceAllBtn.setDisabled(true);
      return;
    }
    replaceBtn.setDisabled(false);
    const activeTab = activeTabOf();
    // A formula's result matches the search, but only the formula text can be replaced.
    const tabCount = matches.filter((m) => m.tab === activeTab && (formulaCheck.checked || !m.formula)).length;
    replaceAllBtn.setLabel(tabCount > 0 ? `Replace all (${tabCount})` : "Replace all");
    replaceAllBtn.setDisabled(tabCount === 0);
  }

  function syncControls() {
    if (!input.value) {
      countText.textContent = "";
      prevBtn.setDisabled(true);
      nextBtn.setDisabled(true);
      syncReplaceControls();
      return;
    }
    if (matches.length === 0) {
      countText.textContent = "0 of 0";
      prevBtn.setDisabled(true);
      nextBtn.setDisabled(true);
      syncReplaceControls();
      return;
    }
    countText.textContent = `${currentIndex + 1} of ${matches.length}`;
    prevBtn.setDisabled(false);
    nextBtn.setDisabled(false);
    syncReplaceControls();
  }

  function rebuild({ jump = false } = {}) {
    lastBuiltKey = stateKey();
    if (!input.value) {
      matches = [];
      currentIndex = -1;
      syncControls();
      return;
    }
    const tabs = getTabs();
    const tabOrder = new Map(tabs.map((t, i) => [t.id, i]));
    const prev = !jump && currentIndex >= 0 ? matches[currentIndex] : null;
    matches = findMatches(tabs, { query: input.value, searchFormulas: formulaCheck.checked });
    if (matches.length === 0) {
      currentIndex = -1;
      syncControls();
      return;
    }
    const validPrev = prev && tabOrder.has(prev.tab) ? prev : null;
    const anchor = jump ? getActivePos() : (validPrev ?? getActivePos());
    if (anchor) {
      const exact = !jump
        ? matches.findIndex((m) => m.tab === anchor.tab && m.r === anchor.r && m.c === anchor.c)
        : -1;
      if (exact !== -1) {
        currentIndex = exact;
      } else {
        const after = matches.findIndex((m) => compareMatchPos(tabOrder, m, anchor) >= 0);
        currentIndex = after !== -1 ? after : 0;
      }
    } else {
      currentIndex = Math.max(0, Math.min(currentIndex, matches.length - 1));
    }
    syncControls();
    if (jump && currentIndex >= 0) onSelectMatch?.(matches[currentIndex]);
  }

  function step(delta) {
    if (!input.value) return;
    if (stateKey() !== lastBuiltKey || matches.length === 0) {
      rebuild({ jump: true });
      return;
    }
    currentIndex = ((currentIndex + delta) % matches.length + matches.length) % matches.length;
    syncControls();
    onSelectMatch?.(matches[currentIndex]);
  }

  function replaceCurrent() {
    if (!input.value || !onBeforeAction()) return;
    if (stateKey() !== lastBuiltKey || matches.length === 0) rebuild({ jump: true });
    if (currentIndex < 0 || currentIndex >= matches.length) return;
    const target = matches[currentIndex];
    const tabs = getTabs();
    const tabObj = tabs.find((t) => t.id === target.tab);
    if (!tabObj?.grid) return;
    // Undo is kept per tab, so the match's tab is shown before its cell changes.
    if (target.tab !== activeTabOf()) onSelectMatch?.(target);

    let outcome = "skipped";
    runWrite(() => {
      outcome = applyCellReplacement(tabObj.grid, target.ref, {
        query: input.value,
        replacement: replaceInput.value,
        searchFormulas: formulaCheck.checked,
        tokenize: getTokenize?.(),
      });
    });

    if (outcome === "refused") {
      onNotice?.("1 formula would not be valid and was left alone.");
    } else if (outcome === "skipped" && target.formula && !formulaCheck.checked) {
      onNotice?.("That match is a formula's result. Turn on Search formulas to change the formula.");
    }
    onAfterReplace?.();

    const freshTabs = getTabs();
    const tabOrder = new Map(freshTabs.map((t, i) => [t.id, i]));
    lastBuiltKey = stateKey();
    matches = findMatches(freshTabs, { query: input.value, searchFormulas: formulaCheck.checked });
    if (matches.length === 0) {
      currentIndex = -1;
      syncControls();
      return;
    }
    const nextIdx = matches.findIndex((m) => compareMatchPos(tabOrder, m, target) > 0);
    currentIndex = nextIdx !== -1 ? nextIdx : 0;
    syncControls();
    onSelectMatch?.(matches[currentIndex]);
  }

  let replacing = false;

  function replaceAllInActiveTab() {
    if (!input.value || !onBeforeAction()) return;
    const activeTab = activeTabOf();
    const tabObj = getTabs().find((t) => t.id === activeTab);
    if (!tabObj?.grid) return;
    // Each cell written announces a change; searching every tab again per cell made a large
    // Replace all freeze the tab, so it searches once at the end.
    replacing = true;
    let result;
    try {
      result = replaceAllOnTab(tabObj, {
        query: input.value,
        replacement: replaceInput.value,
        searchFormulas: formulaCheck.checked,
        history,
        runWrite,
        tokenize: getTokenize?.(),
      });
    } finally {
      replacing = false;
    }
    const { replaced, refused } = result;
    onAfterReplace?.();
    rebuild({ jump: false });
    onNotice?.(replaceNoticeText(replaced, refused));
  }

  function open({ replace = false } = {}) {
    openState = true;
    replaceOpen = Boolean(replace);
    setShown(replaceRow, replaceOpen, "flex");
    setShown(bar, true, "flex");
    rebuild({ jump: false });
    input.focus();
    input.select?.();
  }

  function close({ restoreFocus = false } = {}) {
    if (!openState) return;
    openState = false;
    setShown(bar, false, "flex");
    if (restoreFocus) onClose?.();
  }

  input.addEventListener("input", () => rebuild({ jump: true }));
  formulaCheck.addEventListener("change", () => rebuild({ jump: true }));

  bar.addEventListener("keydown", (event) => {
    // Inside the bar, Ctrl+H would open the browser's history and Ctrl+F the browser's find.
    const intent = keyIntent(event, { platform: container.ownerDocument?.defaultView?.navigator?.platform ?? "" });
    if (intent?.type === "findReplace" || intent?.type === "find") {
      event.preventDefault();
      event.stopPropagation();
      open({ replace: intent.type === "findReplace" || replaceOpen });
      return;
    }
    if (event.key === "Escape") {
      event.preventDefault();
      event.stopPropagation();
      close({ restoreFocus: true });
      return;
    }
    if (event.key === "Enter" && event.target === input) {
      event.preventDefault();
      event.stopPropagation();
      step(event.shiftKey ? -1 : 1);
      return;
    }
    if (event.key === "Enter" && event.target === replaceInput) {
      event.preventDefault();
      event.stopPropagation();
      replaceCurrent();
    }
  });

  syncControls();

  return {
    el: bar,
    input,
    replaceInput,
    replaceRow,
    replaceBtn,
    replaceAllBtn,
    formulaCheck,
    countText,
    get isOpen() {
      return openState;
    },
    get isReplaceOpen() {
      return openState && replaceOpen;
    },
    get matches() {
      return matches.slice();
    },
    get currentIndex() {
      return currentIndex;
    },
    open,
    close,
    step,
    replaceCurrent,
    replaceAll: replaceAllInActiveTab,
    syncTab() {
      if (openState) syncReplaceControls();
    },
    refresh() {
      if (openState && !replacing) rebuild({ jump: false });
    },
    dispose() {
      openState = false;
      bar.remove();
    },
  };
}

export function createFindController(ctx) {
  let bar = null;

  function ensureBar() {
    bar ??= createFindBar({
      theme: ctx.chrome.theme,
      container: ctx.chrome.findSlot,
      getTabs: () => ctx.tabs.list().map((t) => ({ id: t.id, name: t.name, grid: ctx.gridOf(t.table), rowHidden: ctx.viewOf(t.id).rowHidden })),
      getActiveTab: () => ctx.tabs.activeId || ctx.getSel()?.tab || "",
      getActivePos: () => {
        const info = ctx.activeCellInfo();
        const sel = ctx.getSel();
        return info
          ? { tab: info.tab, r: Math.max(0, info.r), c: Math.max(0, info.c) }
          : { tab: sel?.tab ?? "", r: 0, c: 0 };
      },
      history: ctx.history,
      runWrite: (fn) => ctx.writeGrid(fn),
      getTokenize: () => ctx.win.Sap?.grid?.tokenize,
      onBeforeAction: () => ctx.ensureReadyForAction(),
      onAfterReplace: () => {
        ctx.refreshStatus();
        ctx.syncUI({ announce: true });
      },
      onNotice: (msg) => ctx.chrome.showNotice(msg),
      onSelectMatch: (match) => {
        if (!ctx.ensureReadyForAction()) return;
        const view = ctx.viewOf(match.tab);
        if (view.rows.length === 0 || view.cols.length === 0) return;
        const sel = ctx.getSel();
        const entered = match.tab !== sel?.tab;
        if (entered) ctx.tabs.show(match.tab);
        const base = entered ? createSelection(view, match.tab) : sel;
        ctx.applyMove(selectCell(view, base, match.r, match.c, { extend: false }), { announce: true, enteredTab: entered });
      },
      onClose: () => ctx.editor.focus(),
    });
    return bar;
  }

  function goTo(raw) {
    if (!ctx.ensureReadyForAction()) return;
    const trimmed = String(raw ?? "").trim();
    const sel = ctx.getSel();
    const parsed = parseGoToRef(trimmed, ctx.tabs.list(), sel?.tab ?? ctx.tabs.activeId);
    if (!parsed) {
      ctx.chrome.showNotice(`Could not find "${trimmed}".`);
      return;
    }
    const view = ctx.viewOf(parsed.tab);
    if (view.rows.length === 0 || view.cols.length === 0) return;
    const r1 = parsed.whole === "cols" ? view.rows.length - 1 : parsed.r1;
    const c1 = parsed.whole === "rows" ? view.cols.length - 1 : parsed.c1;
    if (r1 >= view.rows.length || c1 >= view.cols.length) {
      ctx.chrome.showNotice(`"${trimmed}" is outside this sheet.`);
      return;
    }
    ctx.chrome.hideNotice();
    const entered = parsed.tab !== sel?.tab;
    if (entered) ctx.tabs.show(parsed.tab);
    let nextSel;
    if (parsed.whole === "cols") {
      nextSel = {
        tab: parsed.tab,
        anchor: { row: view.rows[0], col: view.cols[parsed.c0] },
        focus: { row: view.rows[view.rows.length - 1], col: view.cols[c1] },
        whole: "cols",
      };
    } else if (parsed.whole === "rows") {
      nextSel = {
        tab: parsed.tab,
        anchor: { row: view.rows[parsed.r0], col: view.cols[0] },
        focus: { row: view.rows[r1], col: view.cols[view.cols.length - 1] },
        whole: "rows",
      };
    } else {
      const ownA = ownerOf(view, parsed.r0, parsed.c0);
      nextSel = {
        tab: parsed.tab,
        anchor: { row: view.rows[ownA.r], col: view.cols[ownA.c] },
        focus: { row: view.rows[r1], col: view.cols[c1] },
      };
    }
    ctx.applyMove(nextSel, { announce: true, enteredTab: entered });
    ctx.editor.focus();
  }

  return {
    open: (opts) => ensureBar().open(opts),
    goTo,
    syncTab: () => bar?.syncTab(),
    refresh: () => bar?.refresh(),
    dispose: () => bar?.dispose(),
  };
}
