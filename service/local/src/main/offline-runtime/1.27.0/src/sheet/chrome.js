import {
  bevelBox,
  bevelButton,
  bevelIconButton,
  bevelInput,
  bevelSurface,
  bevelText,
  protectIcon,
  setShown,
} from "../ui/bevel-controls.js";
import { FONT_MONO, FONT_SANS, RULES, TOKENS } from "../ui/bevel.js";
import { set } from "../lib/hostile-css.js";
import { resolveTheme } from "../ui/theme.js";
import { mountEditorUI } from "../ui/editor-ui.js";

const LONG_PRESS_MS = 420;

const svgFill = (d) => `<svg viewBox="0 0 256 256" fill="currentColor" aria-hidden="true"><path d="${d}"/></svg>`;
const svgStroke = (d, w = 1.75) =>
  `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="${w}" stroke-linecap="square" stroke-linejoin="miter" aria-hidden="true"><path d="${d}"/></svg>`;

const ICONS = {
  undo: svgFill("M224,128a96,96,0,0,1-94.71,96H128A95.38,95.38,0,0,1,62.1,197.8a8,8,0,0,1,11-11.63A80,80,0,1,0,71.43,71.39l-.26.25L44.59,96H72a8,8,0,0,1,0,16H24a8,8,0,0,1-8-8V56a8,8,0,0,1,16,0V85.8L60.25,60A96,96,0,0,1,224,128Z"),
  redo: svgFill("M240,56v48a8,8,0,0,1-8,8H184a8,8,0,0,1,0-16H211.4L184.81,71.64l-.25-.24a80,80,0,1,0-1.67,114.78,8,8,0,0,1,11,11.63A95.44,95.44,0,0,1,128,224h-1.32A96,96,0,1,1,195.75,60L224,85.8V56a8,8,0,1,1,16,0Z"),
  bold: svgFill("M178.48,115.7A44,44,0,0,0,148,40H80a8,8,0,0,0-8,8V200a8,8,0,0,0,8,8h80a48,48,0,0,0,18.48-92.3ZM88,56h60a28,28,0,0,1,0,56H88Zm72,136H88V128h72a32,32,0,0,1,0,64Z"),
  italic: svgFill("M200,56a8,8,0,0,1-8,8H157.77L115.1,192H144a8,8,0,0,1,0,16H64a8,8,0,0,1,0-16H98.23L140.9,64H112a8,8,0,0,1,0-16h80A8,8,0,0,1,200,56Z"),
  strike: svgFill("M224,128a8,8,0,0,1-8,8H175.93c9.19,7.11,16.07,17.2,16.07,32,0,13.34-7,25.7-19.75,34.79C160.33,211.31,144.61,216,128,216s-32.33-4.69-44.25-13.21C71,193.7,64,181.34,64,168a8,8,0,0,1,16,0c0,17.35,22,32,48,32s48-14.65,48-32c0-14.85-10.54-23.58-38.77-32H40a8,8,0,0,1,0-16H216A8,8,0,0,1,224,128Z"),
  fill: svgStroke("M10 12l8-8a2 2 0 0 1 3 3l-8 8ZM10 12c-3-1-5 1-5 4 0 2-2 4-2 4s5 1 8-1c2-1 3-3 2-4"),
  grid: svgStroke("M3 3h7v7H3ZM14 3h7v7h-7ZM3 14h7v7H3ZM14 14h7v7h-7Z"),
  wrap: svgStroke("M19 3v12H5m5-5-5 5 5 5"),
  filter: svgStroke("M3 5h18l-7 8v6l-4 2v-8Z"),
  sort: svgStroke("M4 6h16M7 12h10M10 18h4"),
  checkCircle: svgStroke("M21 12a9 9 0 1 1-18 0 9 9 0 0 1 18 0M8 12l3 3 5-6"),
  chart: svgStroke("M4 3v18M4 5h11v4.5H4M4 9.5h16V14H4M4 14h8v4.5H4"),
  search: svgStroke("M20 20l-4.5-4.5M17 10a7 7 0 1 1-14 0 7 7 0 0 1 14 0"),
  close: svgFill("M205.66,194.34a8,8,0,0,1-11.32,11.32L128,139.31,61.66,205.66a8,8,0,0,1-11.32-11.32L116.69,128,50.34,61.66A8,8,0,0,1,61.66,50.34L128,116.69l66.34-66.35a8,8,0,0,1,11.32,11.32L139.31,128Z"),
  check: svgStroke("m5 12 4 4L19 6", 2.2),
  caret: svgStroke("m6 9 6 6 6-6", 2.2),
  more: svgFill("M140,128a12,12,0,1,1-12-12A12,12,0,0,1,140,128ZM196,116a12,12,0,1,0,12,12A12,12,0,0,0,196,116ZM60,116a12,12,0,1,0,12,12A12,12,0,0,0,60,116Z"),
};

// [id, label, icon, text, caret, extraFlags]
const GROUPS = [
  ["history", false, [
    ["undo", "Undo", "undo"],
    ["redo", "Redo", "redo"],
  ]],
  ["number", false, [
    ["number-format", "Number format", null, "Automatic", true, ["min-width:88px", "justify-content:space-between"]],
    ["currency", "Format as currency", null, "$", false, ["width:26px", "padding:0"], true],
    ["percent", "Format as percent", null, "%", false, ["width:26px", "padding:0"], true],
    ["decimals-less", "Fewer decimal places", null, ".0", false, ["padding:0 8px", "font-size:11px"], true],
    ["decimals-more", "More decimal places", null, ".00", false, ["padding:0 8px", "font-size:11px"], true],
  ]],
  ["text", false, [
    ["bold", "Bold", "bold"],
    ["italic", "Italic", "italic"],
    ["strikethrough", "Strikethrough", "strike"],
    ["text-color", "Text color", null, "A", false, ["width:26px", "padding:0", "font-weight:700"]],
    ["fill-color", "Fill color", "fill"],
    ["borders", "Borders", "grid", null, true],
    ["merge", "Merge cells", null, "Merge"],
    ["align", "Horizontal align", null, "Align", true],
    ["wrap", "Text wrapping", "wrap", "Wrap"],
  ]],
  ["data", false, [
    ["filter", "Filter", "filter", "Filter"],
    ["sort", "Sort", "sort", "Sort", true],
    ["checkbox", "Insert checkbox", "checkCircle", "Check"],
    ["dropdown", "Dropdown…"],
    ["conditional-format", "Conditional formatting…"],
    ["chart", "Insert chart", "chart", "Chart"],
    ["find", "Find and replace", "search", "Find"],
    ["insert-row", "Insert row", null, "Row +"],
  ]],
  ["trailing", true, [
    ["import", "Import or export", null, "Import / Export", true],
  ]],
];

export const PHONE_MAX = 760;
export const COMPACT_MAX = 960;

// The sheet's own width picks the layout, so a sheet in a narrow column acts like one on a phone.
export function layoutFor(width) {
  if (!(width > 0)) return "wide";
  return width < PHONE_MAX ? "phone" : width < COMPACT_MAX ? "compact" : "wide";
}

const PHONE_ORDER = ["undo", "bold", "number-format", "filter", "insert-row"];
const PHONE_ONLY = new Set(["insert-row"]);
const MORE_ONLY = new Set(["dropdown", "conditional-format"]);
const ALWAYS_IN_MORE = new Set(["chart"]);
const COMPACT_MORE_GROUPS = new Set(["data"]);
const PHONE_LABELS = { "number-format": "$%" };
const QUICK_KEYS = ["=", "(", ")", ":", "+", "$"];
const PHONE_ICON_ONLY = new Set(["filter"]);

// Bevel controls rebuild their inline style on each state change; pinned declarations survive it.
function pinStyle(el, props) {
  if (el.pin) el.pin(props);
  else for (const [prop, value] of Object.entries(props)) set(el, prop, value);
}

function appendSvg(parent, markup, size) {
  const holder = parent.ownerDocument.createElement("span");
  holder.innerHTML = markup;
  const svg = holder.firstElementChild;
  if (svg) {
    protectIcon(svg, size);
    parent.append(svg);
  }
}

function makeToolButton([id, label, icon, text, caret, extra, mono], theme, onCommand) {
  const iconOnly = Boolean(icon && !text && !caret);
  const baseExtra = iconOnly
    ? ["width:26px", "height:26px", "padding:0", "display:inline-grid", "place-items:center"]
    : [
        "height:26px",
        "padding:0 8px",
        "gap:4px",
        "font-size:12px",
        ...(mono ? [`font-family:${FONT_MONO}`, "font-variant-numeric:tabular-nums"] : []),
        ...(extra ?? []),
      ];
  const stateExtra = [...baseExtra];
  const btn = bevelButton(text ?? "", {
    small: true,
    onClick: () => onCommand(id),
    theme,
    parts: ["sheet.tool"],
    extra: stateExtra,
    labelExtra: iconOnly
      ? ["display:inline-grid", "width:12px", "height:12px"]
      : ["display:inline-flex", "align-items:center", "gap:4px"],
  });
  const span = btn.firstChild;
  span.textContent = "";
  let textEl = null;
  if (iconOnly) {
    span.innerHTML = ICONS[icon];
    protectIcon(span.firstElementChild, 12);
  } else {
    if (icon) appendSvg(span, ICONS[icon], 14);
    if (text) {
      textEl = bevelText("span", ["font:inherit", "color:inherit"], text, { theme, role: null });
      span.append(textEl);
    }
    if (caret) appendSvg(span, ICONS.caret, 11);
  }
  btn.setAttribute("aria-label", label);
  btn.title = label;
  btn.setAttribute("data-command", id);
  btn.addEventListener("mousedown", (e) => e.preventDefault());
  return { btn, textEl, baseExtra, stateExtra, pressed: false };
}

export function createChrome({
  scrollBox,
  commands = {},
  onSelectTab,
  onTabMenu,
  onRenameTab,
  onRenameTabStart,
  onGoTo,
  onCancelGoTo,
  onCommit,
  onCancel,
  onMoreMenu,
  onQuickKey,
  onLayoutChange,
} = {}) {
  const doc = scrollBox.ownerDocument ?? document;
  const win = doc.defaultView ?? window;
  const parent = scrollBox.parentElement ?? doc.body;
  const theme = resolveTheme();
  const registry = new Map(Object.entries(commands));
  const buttons = new Map();
  const buttonMeta = new Map();
  let longPressTimer = null;

  const clearLongPress = () => {
    if (longPressTimer !== null) {
      clearTimeout(longPressTimer);
      longPressTimer = null;
    }
  };

  const topRoot = bevelBox("div", [
    "display:block",
    "width:100%",
    `color-scheme:${theme.scheme}`,
    `font:13px/1.4 ${FONT_SANS}`,
    `background:${TOKENS.ground}`,
    `color:${TOKENS.ink}`,
  ], { theme });

  const toolbar = bevelBox("div", [
    "display:flex",
    "align-items:center",
    "flex-wrap:wrap",
    "gap:4px",
    "min-height:36px",
    "padding:4px 10px",
    `background:${TOKENS.ground}`,
    `border-bottom:1px solid ${TOKENS["line-2"]}`,
  ], { theme, role: "well" });
  toolbar.setAttribute("role", "toolbar");
  toolbar.setAttribute("aria-label", "Formatting and sheet tools");

  const groupEls = new Map();
  const toolGroup = new Map();
  const lastText = new Map();
  GROUPS.forEach(([name, trailing, items], gIdx) => {
    let sep = null;
    if (gIdx > 0 && !trailing) {
      sep = bevelBox("span", [
        "display:inline-block",
        "width:1px",
        "height:18px",
        "margin:0 4px",
        "flex:none",
        `background:${TOKENS["line-2"]}`,
      ], { theme });
      sep.setAttribute("aria-hidden", "true");
      toolbar.append(sep);
    }
    const groupEl = bevelBox("div", [
      "display:inline-flex",
      "align-items:center",
      "gap:3px",
      ...(trailing ? ["margin-inline-start:auto"] : []),
    ], { theme });
    groupEl.setAttribute("data-toolbar-group", name);
    for (const spec of items) {
      const id = spec[0];
      const meta = makeToolButton(spec, theme, (cmdId) => registry.get(cmdId)?.());
      meta.btn.setDisabled(!registry.has(id));
      buttons.set(id, meta.btn);
      buttonMeta.set(id, meta);
      toolGroup.set(id, name);
      if (spec[3] != null) lastText.set(id, spec[3]);
      groupEl.append(meta.btn);
    }
    groupEls.set(name, { groupEl, sep });
    toolbar.append(groupEl);
  });

  let layout = null;
  const shownIn = (id, at = layout) => {
    if (MORE_ONLY.has(id)) return false;
    if (PHONE_ONLY.has(id)) return at === "phone";
    if (at === "phone") return PHONE_ORDER.includes(id);
    if (at === "compact") return !COMPACT_MORE_GROUPS.has(toolGroup.get(id));
    return true;
  };
  const inMore = (id) => ALWAYS_IN_MORE.has(id) || (!PHONE_ONLY.has(id) && !shownIn(id));

  const moreBtn = makeToolButton(["more", "More tools", "more"], theme, () => onMoreMenu?.(moreItems(), moreBtn)).btn;
  moreBtn.setAttribute("aria-haspopup", "menu");
  toolbar.append(moreBtn);

  function paintMore() {
    moreBtn.setDisabled(![...buttonMeta.keys()].some((id) => inMore(id) && registry.has(id)));
  }

  function moreItems() {
    const items = [];
    for (const [, , specs] of GROUPS) {
      const ids = specs.map((spec) => spec[0]).filter(inMore);
      if (ids.length === 0) continue;
      if (items.length) items.push({ type: "separator" });
      for (const id of ids) {
        items.push({
          id,
          label: buttonMeta.get(id).btn.getAttribute("aria-label"),
          disabled: !registry.has(id),
          onSelect: () => registry.get(id)?.(),
        });
      }
    }
    return items;
  }

  const formulaBar = bevelBox("div", [
    "display:flex",
    "align-items:center",
    "gap:6px",
    "height:30px",
    "padding:0 8px",
    `background:${TOKENS.surface}`,
    `border-bottom:1px solid ${TOKENS["line-2"]}`,
  ], { theme });

  const monoTheme = {
    ...theme,
    parts: {
      ...theme.parts,
      "sheet.formulaInput": { ...theme.parts?.["sheet.formulaInput"], base: { ...theme.parts?.["sheet.formulaInput"]?.base, "font-family": FONT_MONO } },
      "sheet.nameBox": { ...theme.parts?.["sheet.nameBox"], base: { ...theme.parts?.["sheet.nameBox"]?.base, "font-family": FONT_MONO } },
    },
  };

  const nameBox = bevelInput("input", {
    theme: monoTheme,
    rules: [
      "width:88px",
      "min-width:66px",
      "height:22px",
      "block-size:22px",
      "min-block-size:22px",
      "padding:0 6px",
      "text-align:center",
      `font:600 12px/20px ${FONT_MONO}`,
      "font-variant-numeric:tabular-nums",
    ],
    parts: ["sheet.nameBox"],
  });
  nameBox.type = "text";
  nameBox.spellcheck = false;
  nameBox.setAttribute("autocomplete", "off");
  nameBox.setAttribute("aria-label", "Name box");
  nameBox.setAttribute("data-sheet-name-box", "");
  nameBox.title = "Name box";
  Object.defineProperty(nameBox, "textContent", {
    configurable: true,
    get() {
      return this.value;
    },
    set(next) {
      this.value = String(next ?? "");
    },
  });

  const fxActions = bevelBox("div", ["display:inline-flex", "align-items:center", "gap:2px"], { theme });
  const cancelBtn = bevelIconButton(ICONS.close, { label: "Cancel edit", onClick: () => onCancel?.(), theme });
  cancelBtn.addEventListener("mousedown", (e) => e.preventDefault());
  const commitBtn = bevelIconButton(ICONS.check, { label: "Commit edit", onClick: () => onCommit?.(), theme });
  commitBtn.addEventListener("mousedown", (e) => e.preventDefault());
  cancelBtn.setDisabled(true);
  commitBtn.setDisabled(true);
  fxActions.append(cancelBtn, commitBtn);

  const fxBadge = bevelText("span", [
    "display:inline-block",
    "padding:0 4px",
    "font:italic 600 13px/1 Georgia,serif",
    `color:${TOKENS.muted}`,
    `border-right:1px solid ${TOKENS.line}`,
  ], "fx", { theme, role: "mutedText" });
  fxBadge.setAttribute("aria-hidden", "true");

  const formulaInput = bevelInput("textarea", {
    theme: monoTheme,
    rules: [
      "flex:1 1 auto",
      "min-width:0",
      "height:22px",
      "block-size:22px",
      "padding:0 8px",
      `font:400 12.5px/22px ${FONT_MONO}`,
      "font-variant-numeric:tabular-nums",
      "resize:none",
      "overflow:hidden",
      "white-space:pre",
    ],
    parts: ["sheet.formulaInput"],
  });
  formulaInput.rows = 1;
  formulaInput.spellcheck = false;
  formulaInput.setAttribute("autocomplete", "off");
  formulaInput.setAttribute("aria-label", "Formula bar");

  formulaBar.append(nameBox, fxActions, fxBadge, formulaInput);

  const quickKeys = bevelBox("div", ["display:none", "flex-basis:100%", "gap:6px", "padding:6px 0 0"], { theme });
  quickKeys.setAttribute("data-sheet-quick-keys", "");
  quickKeys.setAttribute("role", "group");
  quickKeys.setAttribute("aria-label", "Formula symbols");
  for (const key of QUICK_KEYS) {
    const btn = bevelButton(key, {
      small: true,
      onClick: () => onQuickKey?.(key),
      theme,
      extra: ["flex:1 1 0", "min-width:44px", "padding:0", `font:600 15px/1 ${FONT_MONO}`],
    });
    btn.setAttribute("aria-label", `Type ${key}`);
    btn.addEventListener("pointerdown", (e) => e.preventDefault());
    btn.addEventListener("mousedown", (e) => e.preventDefault());
    quickKeys.append(btn);
  }
  formulaBar.append(quickKeys);

  let editingNow = false;
  let keyboardInset = 0;
  const DOCK = {
    position: "fixed",
    left: "0",
    right: "0",
    "z-index": "2147482000",
    height: "auto",
    "min-height": "52px",
    padding: "6px 8px calc(6px + env(safe-area-inset-bottom))",
    "flex-wrap": "wrap",
    "border-top": `1px solid ${TOKENS["line-2"]}`,
  };
  const BAR = {
    position: "static",
    left: "auto",
    right: "auto",
    "z-index": "auto",
    height: "30px",
    "min-height": "0",
    padding: "0 8px",
    "flex-wrap": "nowrap",
    "border-top": "0",
  };
  // On a phone the formula bar is the edit bar: hidden while reading, docked above the
  // keyboard while editing.
  // A phone-width sheet docks its edit bar only for a finger: a mouse user in a narrow
  // window keeps the formula bar and the name box (Go to) where they always are.
  let touchMode = Boolean(win.matchMedia?.("(pointer: coarse)")?.matches);
  const docked = () => layout === "phone" && touchMode;
  function paintFormulaBar() {
    const phone = docked();
    for (const btn of [cancelBtn, commitBtn]) pinStyle(btn, { "min-width": phone ? "44px" : "0" });
    const dock = phone && editingNow;
    for (const [prop, value] of Object.entries(dock ? DOCK : BAR)) set(formulaBar, prop, value);
    set(formulaBar, "bottom", dock ? `${keyboardInset}px` : "auto");
    setShown(formulaBar, !phone || editingNow, "flex");
    setShown(quickKeys, dock, "flex");
  }
  const viewport = win.visualViewport ?? null;
  const onViewport = () => {
    const next = Math.max(0, Math.round(win.innerHeight - viewport.height - viewport.offsetTop));
    if (next === keyboardInset) return;
    keyboardInset = next;
    if (docked() && editingNow) paintFormulaBar();
  };
  viewport?.addEventListener("resize", onViewport);
  viewport?.addEventListener("scroll", onViewport);

  const noticeText = bevelText("span", ["flex:1 1 auto", `font:500 12.5px/1.4 ${FONT_SANS}`, `color:${TOKENS.ox}`], "", { theme });
  let noticeActionFn = null;
  const noticeActionBtn = bevelButton("", {
    small: true,
    variant: "primary",
    onClick: () => noticeActionFn?.(),
    theme,
  });
  noticeActionBtn.setAttribute("data-sheet-notice-action", "");
  noticeActionBtn.addEventListener("mousedown", (e) => e.preventDefault());
  setShown(noticeActionBtn, false, "inline-flex");
  const noticeDismiss = bevelButton("Dismiss", {
    small: true,
    variant: "quiet",
    onClick: () => hideNotice(),
    theme,
  });
  noticeDismiss.addEventListener("mousedown", (e) => e.preventDefault());
  const noticeBar = bevelSurface("div", [
    "display:none",
    "align-items:center",
    "justify-content:space-between",
    "gap:10px",
    "padding:6px 10px",
    `background:${TOKENS["ox-soft"]}`,
    `border:1px solid ${TOKENS.ox}`,
    "box-shadow:none",
  ], { theme });
  noticeBar.setAttribute("role", "alert");
  noticeBar.setAttribute("data-sheet-notice", "");
  noticeBar.hidden = true;
  noticeBar.append(noticeText, noticeActionBtn, noticeDismiss);

  const findSlot = doc.createElement("div");
  topRoot.append(toolbar, formulaBar, findSlot, noticeBar);

  const bottomRoot = bevelBox("div", [
    "display:flex",
    "align-items:stretch",
    "justify-content:space-between",
    "gap:8px",
    "min-height:30px",
    "padding:0 10px",
    `color-scheme:${theme.scheme}`,
    `background:${TOKENS.sunk}`,
    `border-top:1px solid ${TOKENS["line-2"]}`,
    `font:12px/1.3 ${FONT_SANS}`,
    `color:${TOKENS.ink}`,
  ], { theme, role: "well" });

  const tabStrip = bevelBox("div", ["display:flex", "align-items:stretch", "gap:2px", "overflow-x:auto"], { theme });
  tabStrip.setAttribute("role", "tablist");
  tabStrip.setAttribute("aria-label", "Workbook sheets");

  const addTabBtn = bevelButton("+", {
    small: true,
    variant: "quiet",
    onClick: () => registry.get("add-tab")?.(),
    theme,
    extra: ["width:28px", "height:28px", "padding:0"],
  });
  addTabBtn.setAttribute("aria-label", "Add sheet");
  addTabBtn.setAttribute("data-command", "add-tab");
  addTabBtn.setDisabled(!registry.has("add-tab"));
  addTabBtn.addEventListener("mousedown", (e) => e.preventDefault());
  buttons.set("add-tab", addTabBtn);

  const tabsHolder = bevelBox("div", ["display:inline-flex", "align-items:center", "gap:2px"], { theme });
  tabStrip.append(addTabBtn, tabsHolder);

  const statusText = bevelText("div", [
    "display:inline-flex",
    "align-items:center",
    `font:500 11.5px/1 ${FONT_MONO}`,
    "font-variant-numeric:tabular-nums",
    `color:${TOKENS["ink-2"]}`,
  ], "", { theme, role: "mutedText" });
  statusText.setAttribute("data-sheet-status", "");

  bottomRoot.append(tabStrip, statusText);

  const topUI = mountEditorUI({
    name: "sheet-top",
    content: topRoot,
    parent,
    before: scrollBox,
    attributes: { "sap-ignore": "" },
  });
  const bottomUI = mountEditorUI({
    name: "sheet-bottom",
    content: bottomRoot,
    parent,
    before: scrollBox.nextSibling,
    attributes: { "sap-ignore": "" },
  });

  function showNotice(message, action = null) {
    noticeText.textContent = message;
    if (action && typeof action.onAction === "function") {
      noticeActionFn = action.onAction;
      noticeActionBtn.textContent = action.label || "Fix";
      setShown(noticeActionBtn, true, "inline-flex");
    } else {
      noticeActionFn = null;
      setShown(noticeActionBtn, false, "inline-flex");
    }
    setShown(noticeBar, true, "flex");
  }

  function hideNotice() {
    noticeText.textContent = "";
    noticeActionFn = null;
    setShown(noticeActionBtn, false, "inline-flex");
    setShown(noticeBar, false, "flex");
  }

  function setEditing(editing) {
    cancelBtn.setDisabled(!editing);
    commitBtn.setDisabled(!editing);
    editingNow = editing;
    paintFormulaBar();
  }

  function applyLayout(next, notify = true) {
    if (next === layout) return;
    layout = next;
    const phone = layout === "phone";
    set(toolbar, "flex-wrap", phone ? "nowrap" : "wrap");
    set(toolbar, "overflow", phone ? "hidden" : "visible");
    set(toolbar, "justify-content", phone ? "space-between" : "flex-start");
    for (const [id, meta] of buttonMeta) {
      setShown(meta.btn, shownIn(id), "inline-flex");
      const order = phone && PHONE_ORDER.includes(id) ? String(PHONE_ORDER.indexOf(id)) : "0";
      // A finger needs 44 px across as well as down; six tools fit 320 px with Filter as an icon.
      const minWidth = phone ? "44px" : id === "number-format" ? "88px" : "0";
      pinStyle(meta.btn, { order, "min-width": minWidth });
      if (meta.textEl && PHONE_ICON_ONLY.has(id)) setShown(meta.textEl, !phone, "inline");
    }
    for (const [name, { groupEl, sep }] of groupEls) {
      const any = [...toolGroup].some(([id, group]) => group === name && shownIn(id));
      setShown(groupEl, phone || any, phone ? "contents" : "inline-flex");
      if (sep) setShown(sep, !phone && any, "inline-block");
    }
    setShown(moreBtn, [...buttonMeta.keys()].some(inMore), "inline-flex");
    pinStyle(moreBtn, { order: phone ? "99" : "0", "min-width": phone ? "44px" : "0" });
    paintMore();
    for (const id of Object.keys(PHONE_LABELS)) paintButtonText(id);
    paintFormulaBar();
    if (notify) onLayoutChange?.(layout);
  }

  const measure = () => applyLayout(layoutFor(parent.clientWidth || win.innerWidth || 0));
  const Observer = win.ResizeObserver;
  const resizeWatch = typeof Observer === "function" ? new Observer(measure) : null;
  if (resizeWatch) resizeWatch.observe(parent);
  else win.addEventListener("resize", measure);
  applyLayout(layoutFor(parent.clientWidth || win.innerWidth || 0), false);

  let lastCoord = "";
  const isNameBoxActive = () => topUI.shadow?.activeElement === nameBox || doc.activeElement === nameBox;

  let nameBoxDirty = false;

  nameBox.addEventListener("input", () => {
    nameBoxDirty = true;
  });
  nameBox.addEventListener("focus", () => {
    nameBoxDirty = false;
    nameBox.select?.();
  });
  nameBox.addEventListener("keydown", (event) => {
    if (event.key === "Enter") {
      event.preventDefault();
      event.stopPropagation();
      onGoTo?.(nameBox.value);
    } else if (event.key === "Escape") {
      event.preventDefault();
      event.stopPropagation();
      nameBox.value = lastCoord;
      onCancelGoTo?.();
    }
  });
  nameBox.addEventListener("blur", () => {
    nameBoxDirty = false;
    if (nameBox.value !== lastCoord) nameBox.value = lastCoord;
  });

  function setCoord(area) {
    lastCoord = area;
    set(nameBox, "width", `${Math.max(docked() ? 56 : 88, (area.length + 3) * 8)}px`);
    if ((!isNameBoxActive() || !nameBoxDirty) && nameBox.value !== area) nameBox.value = area;
  }

  function setStatus(text) {
    if (statusText.textContent !== text) statusText.textContent = text;
  }

  function focusNameBox() {
    nameBox.focus();
    nameBox.select?.();
  }

  function setButtonPressed(id, pressed) {
    const meta = buttonMeta.get(id);
    if (!meta) return;
    const next = Boolean(pressed);
    const val = next ? "true" : "false";
    if (meta.btn.getAttribute("aria-pressed") === val && meta.pressed === next) return;
    meta.pressed = next;
    meta.btn.setAttribute("aria-pressed", val);
    meta.stateExtra.splice(0, meta.stateExtra.length, ...meta.baseExtra, ...(next ? RULES.recess : []));
    meta.btn.pin({});
  }

  function paintButtonText(id) {
    const meta = buttonMeta.get(id);
    const text = layout === "phone" && PHONE_LABELS[id] ? PHONE_LABELS[id] : lastText.get(id);
    if (meta?.textEl && text != null && meta.textEl.textContent !== text) meta.textEl.textContent = text;
  }
  function setButtonText(id, text, ariaLabel) {
    const meta = buttonMeta.get(id);
    if (!meta) return;
    lastText.set(id, text);
    paintButtonText(id);
    if (ariaLabel && meta.btn.getAttribute("aria-label") !== ariaLabel) {
      meta.btn.setAttribute("aria-label", ariaLabel);
      meta.btn.title = ariaLabel;
    }
  }

  let lastTabsKey = "";
  let currentTabs = [];
  let currentActiveId = "";
  let renaming = null;

  function finishRenameUI() {
    if (!renaming) return;
    renaming = null;
    lastTabsKey = "";
    renderTabs(currentTabs, currentActiveId);
  }

  function mountTabRename(tabId, buildInput) {
    const tab = currentTabs.find((t) => t.id === tabId);
    if (!tab) return null;
    if (renaming) {
      if (renaming.tabId === tabId) {
        renaming.input.focus();
        return renaming.input;
      }
      finishRenameUI();
    }
    const tabBtn = [...tabsHolder.children].find((el) => el.getAttribute("data-tab-id") === tabId);
    if (!tabBtn) return null;
    const built = buildInput({
      tab,
      theme,
      isCurrent: (inp) => renaming?.input === inp,
      finish: finishRenameUI,
      onRenameTab,
    });
    renaming = { tabId, input: built.input, commit: built.commit };
    tabBtn.replaceWith(built.input);
    built.input.focus();
    built.input.select?.();
    return built.input;
  }

  const tabBtnOf = (e) => e.target?.closest?.("[data-tab-id]") ?? null;
  tabsHolder.addEventListener("mousedown", (e) => {
    if (tabBtnOf(e)) e.preventDefault();
  });
  tabsHolder.addEventListener("dblclick", (e) => {
    const btn = tabBtnOf(e);
    if (!btn) return;
    e.preventDefault();
    onRenameTabStart?.(btn.getAttribute("data-tab-id"));
  });
  tabsHolder.addEventListener("keydown", (e) => {
    const btn = tabBtnOf(e);
    if (btn && (e.key === "ContextMenu" || (e.key === "F10" && e.shiftKey))) {
      e.preventDefault();
      onTabMenu?.(btn.getAttribute("data-tab-id"), btn, 0, 0);
    }
  });
  tabsHolder.addEventListener("contextmenu", (e) => {
    const btn = tabBtnOf(e);
    if (!btn) return;
    clearLongPress();
    e.preventDefault();
    e.stopPropagation();
    onTabMenu?.(btn.getAttribute("data-tab-id"), btn, e.clientX ?? 0, e.clientY ?? 0);
  });
  tabsHolder.addEventListener("pointerdown", (e) => {
    const btn = tabBtnOf(e);
    if (!btn || (e.button !== undefined && e.button !== 0)) return;
    clearLongPress();
    const id = btn.getAttribute("data-tab-id");
    const x = e.clientX ?? 0;
    const y = e.clientY ?? 0;
    longPressTimer = setTimeout(() => {
      longPressTimer = null;
      onTabMenu?.(id, btn, x, y);
    }, LONG_PRESS_MS);
  });
  for (const evt of ["pointerup", "pointercancel"]) {
    tabsHolder.addEventListener(evt, clearLongPress);
  }
  tabsHolder.addEventListener("pointerout", (e) => {
    if (!tabBtnOf(e)?.contains(e.relatedTarget)) clearLongPress();
  });

  function renderTabs(tabs, activeId) {
    currentTabs = tabs;
    currentActiveId = activeId;
    const key = `${activeId}\u0000${tabs.map((t) => `${t.id}:${t.name}`).join("\u0000")}`;
    if (key === lastTabsKey) return;
    renaming = null;
    lastTabsKey = key;
    tabsHolder.textContent = "";
    for (const tab of tabs) {
      const selected = tab.id === activeId;
      const btn = bevelButton(tab.name, {
        small: true,
        variant: selected ? "default" : "quiet",
        onClick: () => onSelectTab?.(tab.id),
        theme,
        parts: ["sheet.tab"],
        extra: [
          "height:28px",
          "padding:0 12px",
          "font-size:12px",
          ...(selected ? [`background:${TOKENS.surface}`, `color:${TOKENS.ink}`, "font-weight:600"] : []),
        ],
      });
      btn.setAttribute("role", "tab");
      btn.setAttribute("aria-selected", selected ? "true" : "false");
      btn.setAttribute("data-tab-id", tab.id);
      tabsHolder.append(btn);
    }
  }

  function registerCommand(id, fn) {
    if (typeof fn === "function") registry.set(id, fn);
    else registry.delete(id);
    buttons.get(id)?.setDisabled(!registry.has(id));
    if (layout) paintMore();
  }

  return {
    theme,
    topRoot,
    topHost: topUI.host,
    bottomHost: bottomUI.host,
    topShadow: topUI.shadow,
    bottomShadow: bottomUI.shadow,
    formulaInput,
    nameBox,
    findSlot,
    noticeBar,
    tabStrip,
    statusText,
    showNotice,
    hideNotice,
    setEditing,
    layout: () => layout,
    setLayout: (next) => applyLayout(next),
    docked,
    setTouch(next) {
      if (Boolean(next) === touchMode) return;
      touchMode = Boolean(next);
      paintFormulaBar();
    },
    setCoord,
    setStatus,
    focusNameBox,
    setButtonPressed,
    setButtonText,
    getButton: (id) => (buttons.has(id) && inMore(id) ? moreBtn : buttons.get(id)) ?? null,
    mountTabRename,
    startRenameTab: (tabId) => onRenameTabStart?.(tabId),
    commitRename: () => renaming?.commit?.() ?? false,
    renderTabs,
    registerCommand,
    dispose() {
      clearLongPress();
      resizeWatch?.disconnect();
      win.removeEventListener("resize", measure);
      viewport?.removeEventListener("resize", onViewport);
      viewport?.removeEventListener("scroll", onViewport);
      renaming = null;
      topUI.remove();
      bottomUI.remove();
    },
  };
}
