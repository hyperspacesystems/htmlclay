import { commitCell } from "./commit.js";
import { canInsertRef, cycleRefAtCaret, formatPickedRef, formulaRefs } from "./refs.js";
import { ownerOf, visibleIndex } from "./selection.js";
import { bevelInput, restyleBox } from "../ui/bevel-controls.js";
import { FONT_MONO, TOKENS } from "../ui/bevel.js";
import { resolveTheme } from "../ui/theme.js";

const GONE_MESSAGE = "That cell was removed. Copy your text, then press Escape.";

const READY_RULES = [
  "width:100%",
  "height:100%",
  "min-width:1px",
  "min-height:1px",
  "padding:0",
  "margin:0",
  "border:0",
  "outline:none",
  "resize:none",
  "overflow:hidden",
  "background:transparent",
  "color:transparent",
  "caret-color:transparent",
  "opacity:0",
  "pointer-events:none",
];

const EDIT_RULES = [
  "width:100%",
  "min-width:100px",
  "min-height:100%",
  "padding:2px 6px",
  "margin:0",
  `background:${TOKENS.surface}`,
  `color:${TOKENS.ink}`,
  `border:2px solid ${TOKENS.brass}`,
  "outline:none",
  "resize:none",
  `font:500 12.5px/1.35 ${FONT_MONO}`,
  "font-variant-numeric:tabular-nums",
  "opacity:1",
  "pointer-events:auto",
];

function setCaret(field, pos) {
  try {
    field.setSelectionRange(pos, pos);
  } catch {}
}

export function createEditor({
  workbook,
  editorSlot,
  formulaInput,
  getActiveCell,
  getView,
  getTabs,
  getShownTab,
  showTab,
  onStateChange,
  onCaretChange,
  onNotice,
  refuseValue,
  onClearNotice,
}) {
  const theme = resolveTheme();
  const monoTheme = {
    ...theme,
    parts: { ...theme.parts, "sheet.editor": { ...theme.parts?.["sheet.editor"], base: { ...theme.parts?.["sheet.editor"]?.base, "font-family": FONT_MONO } } },
  };
  const liveRules = [...READY_RULES];
  const textarea = bevelInput("textarea", { theme: monoTheme, rules: liveRules, parts: ["sheet.editor"] });
  textarea.rows = 1;
  textarea.spellcheck = false;
  textarea.setAttribute("autocomplete", "off");
  textarea.setAttribute("inputmode", "none");
  editorSlot.append(textarea);

  let editing = false;
  let composing = false;
  let target = null;
  let buffer = "";
  let activeField = textarea;
  let pickState = null;
  let syncing = false;

  function applyTextareaLook() {
    const next = editing ? EDIT_RULES : READY_RULES;
    liveRules.splice(0, liveRules.length, ...next);
    restyleBox(textarea, next);
  }

  function syncFields(nextText, caret = null) {
    syncing = true;
    try {
      buffer = nextText;
      if (textarea.value !== nextText) textarea.value = nextText;
      if (formulaInput.value !== nextText) formulaInput.value = nextText;
      if (caret !== null) {
        if (!(composing && activeField === textarea)) setCaret(textarea, caret);
        if (!(composing && activeField === formulaInput)) setCaret(formulaInput, caret);
      }
    } finally {
      syncing = false;
    }
  }

  function currentRefs() {
    if (!editing || !buffer.startsWith("=") || isTextCell()) return [];
    return formulaRefs(buffer, { activeTab: target?.tab ?? null, tabs: getTabs() });
  }

  function isTextCell() {
    const active = getActiveCell(target);
    return active?.effectiveFmt === "@";
  }

  function caretOf(field = activeField) {
    return typeof field?.selectionStart === "number" ? field.selectionStart : buffer.length;
  }

  function beginEdit(initialText, focusField = textarea, caret = initialText.length) {
    const active = getActiveCell();
    if (!active) return;
    onClearNotice?.();
    editing = true;
    textarea.setAttribute("inputmode", "text");
    target = { tab: active.tab, row: active.row, col: active.col };
    pickState = null;
    activeField = focusField;
    applyTextareaLook();
    syncFields(initialText, caret);
    onStateChange?.();
  }

  function finishEdit() {
    const homeTab = target?.tab ?? null;
    editing = false;
    textarea.setAttribute("inputmode", "none");
    target = null;
    pickState = null;
    buffer = "";
    applyTextareaLook();
    syncing = true;
    try {
      textarea.value = "";
    } finally {
      syncing = false;
    }
    if (homeTab && getShownTab() !== homeTab && getTabs().some((t) => t.id === homeTab)) showTab(homeTab);
    onClearNotice?.();
  }

  function refreshReady() {
    if (editing) return;
    const active = getActiveCell();
    syncing = true;
    try {
      textarea.value = "";
      formulaInput.value = active ? active.input : "";
    } finally {
      syncing = false;
    }
  }

  function openWithInput(focusField = textarea) {
    const active = getActiveCell();
    if (!active) return;
    const text = active.input;
    beginEdit(text, focusField, text.length);
    focusField.focus();
  }

  function canPick({ keyboard = false } = {}) {
    if (!editing || !buffer.startsWith("=") || isTextCell()) return false;
    if (pickState) return true;
    const start = caretOf(activeField);
    const end = typeof activeField.selectionEnd === "number" ? activeField.selectionEnd : start;
    return (!keyboard || start === end) && canInsertRef(buffer, start, end);
  }

  // Ends an IME composition before a pointer action that may commit, so the field holds the
  // composed text rather than half of it.
  function settle() {
    if (!composing) return;
    const field = activeField;
    field.blur();
    field.focus();
  }

  function applyPick({ tab, tabName, anchor, focus }) {
    const view = getView(tab);
    if (!view || view.rows.length === 0 || view.cols.length === 0) return false;
    const clampR = (r) => Math.max(0, Math.min(view.rows.length - 1, r));
    const clampC = (c) => Math.max(0, Math.min(view.cols.length - 1, c));
    const a = { r: clampR(anchor.r), c: clampC(anchor.c) };
    const f = { r: clampR(focus.r), c: clampC(focus.c) };
    const formatted = formatPickedRef({
      tab,
      homeTab: target.tab,
      tabName,
      anchor: a,
      focus: f,
    });
    const start = pickState ? pickState.start : caretOf(activeField);
    const end = pickState
      ? pickState.end
      : typeof activeField.selectionEnd === "number"
        ? activeField.selectionEnd
        : start;
    const nextText = buffer.slice(0, start) + formatted + buffer.slice(end);
    const nextEnd = start + formatted.length;
    pickState = { start, end: nextEnd, tab, anchor: a, focus: f };
    syncFields(nextText, nextEnd);
    activeField.focus();
    onStateChange?.();
    return true;
  }

  function pickCell({ tab, tabName, r, c, extend = false }) {
    if (!canPick()) return false;
    const view = getView(tab);
    if (!view) return false;
    const own = ownerOf(view, r, c);
    if (extend && pickState && pickState.tab === tab) {
      return applyPick({ tab, tabName, anchor: pickState.anchor, focus: { r, c } });
    }
    return applyPick({ tab, tabName, anchor: { r: own.r, c: own.c }, focus: { r: own.r, c: own.c } });
  }

  function pickMove({ dr = 0, dc = 0, extend = false }) {
    if (!canPick()) return false;
    const tab = pickState ? pickState.tab : getShownTab();
    const tabInfo = getTabs().find((t) => t.id === tab);
    const view = getView(tab);
    if (!view || view.rows.length === 0 || view.cols.length === 0) return false;
    const tabName = tabInfo?.name ?? "";

    const step = (cur) => {
      const r = dr > 0 ? cur.r + cur.rows - 1 + dr : cur.r + dr;
      const c = dc > 0 ? cur.c + cur.cols - 1 + dc : cur.c + dc;
      return ownerOf(
        view,
        visibleIndex(view.rows.length, r, dr, view.rowHidden),
        visibleIndex(view.cols.length, c, dc, view.colHidden),
      );
    };

    if (!pickState) {
      const origin = tab === target.tab ? getActiveCell(target) : null;
      const baseR = origin && origin.r >= 0 ? origin.r : 0;
      const baseC = origin && origin.c >= 0 ? origin.c : 0;
      const own = step(ownerOf(view, baseR, baseC));
      return applyPick({ tab, tabName, anchor: { r: own.r, c: own.c }, focus: { r: own.r, c: own.c } });
    }

    if (!extend) {
      const own = step(ownerOf(view, pickState.focus.r, pickState.focus.c));
      return applyPick({ tab, tabName, anchor: { r: own.r, c: own.c }, focus: { r: own.r, c: own.c } });
    }
    return applyPick({
      tab,
      tabName,
      anchor: pickState.anchor,
      focus: {
        r: visibleIndex(view.rows.length, pickState.focus.r + dr, dr, view.rowHidden),
        c: visibleIndex(view.cols.length, pickState.focus.c + dc, dc, view.colHidden),
      },
    });
  }

  function insertText(text) {
    if (!editing) return;
    const field = activeField;
    const start = caretOf(field);
    const end = typeof field.selectionEnd === "number" ? field.selectionEnd : start;
    field.value = field.value.slice(0, start) + text + field.value.slice(end);
    field.focus();
    setCaret(field, start + text.length);
    field.dispatchEvent(new field.ownerDocument.defaultView.Event("input", { bubbles: true }));
  }

  function insertNewline() {
    if (!editing) return;
    const start = caretOf(activeField);
    const end = typeof activeField.selectionEnd === "number" ? activeField.selectionEnd : start;
    const nextText = `${buffer.slice(0, start)}\n${buffer.slice(end)}`;
    pickState = null;
    syncFields(nextText, start + 1);
    onStateChange?.();
  }

  function cycleRef() {
    if (!editing || !buffer.startsWith("=") || isTextCell()) return false;
    const cycled = cycleRefAtCaret(buffer, caretOf(activeField));
    if (!cycled) return false;
    if (pickState && pickState.start === cycled.start) {
      pickState = { ...pickState, end: cycled.end };
    }
    syncFields(cycled.text, cycled.caret);
    onStateChange?.();
    return true;
  }

  function replaceRange(start, end, replacement) {
    if (!editing) return false;
    const s = Math.max(0, Math.min(buffer.length, start));
    const e = Math.max(s, Math.min(buffer.length, end));
    const nextText = buffer.slice(0, s) + replacement + buffer.slice(e);
    const nextCaret = s + replacement.length;
    pickState = null;
    syncFields(nextText, nextCaret);
    activeField.focus();
    onStateChange?.();
    return true;
  }

  function commit() {
    if (!editing || !target) return { ok: true, changed: false };
    // The page may refuse a value before it reaches the Grid (a closed dropdown's list).
    const refusal = refuseValue?.(target, buffer);
    const result = refusal
      ? { ok: false, why: "refused", error: Object.assign(new Error(refusal), { name: "GridRefused", code: "refused_by_page" }) }
      : commitCell(workbook, target, buffer);
    if (!result.ok) {
      if (result.why === "gone") onNotice?.(GONE_MESSAGE);
      else onNotice?.(result.error?.message || "That value could not be saved in this cell.");
      activeField.focus();
      return result;
    }
    finishEdit();
    onStateChange?.();
    return result;
  }

  function commitWith(runner) {
    if (!editing || !target) return { ok: true, changed: false };
    const refusal = refuseValue?.(target, buffer);
    const result = refusal
      ? { ok: false, why: "refused", error: Object.assign(new Error(refusal), { name: "GridRefused", code: "refused_by_page" }) }
      : runner({ target, text: buffer, isText: isTextCell() });
    if (!result.ok) {
      if (result.why === "gone") onNotice?.(GONE_MESSAGE);
      else onNotice?.(result.error?.message || "That value could not be saved in this cell.");
      activeField.focus();
      return result;
    }
    finishEdit();
    refreshReady();
    onStateChange?.();
    return result;
  }

  function cancel() {
    if (!editing) return;
    finishEdit();
    refreshReady();
    onStateChange?.();
    textarea.focus();
  }

  textarea.addEventListener("focus", () => {
    activeField = textarea;
    if (editing && buffer.startsWith("=")) onCaretChange?.();
  });
  formulaInput.addEventListener("focus", () => {
    activeField = formulaInput;
    if (!editing) {
      const active = getActiveCell();
      if (active) beginEdit(active.input, formulaInput, caretOf(formulaInput));
    } else if (buffer.startsWith("=")) {
      onCaretChange?.();
    }
  });

  for (const field of [textarea, formulaInput]) {
    field.addEventListener("compositionstart", () => {
      composing = true;
      if (!editing) beginEdit("", field, 0);
    });
    field.addEventListener("compositionend", () => {
      composing = false;
    });
    field.addEventListener("click", () => {
      if (pickState && caretOf(field) !== pickState.end) pickState = null;
      if (editing && buffer.startsWith("=")) onCaretChange?.();
    });
    field.addEventListener("keyup", (event) => {
      if (!editing || !buffer.startsWith("=")) return;
      if (/^(Arrow(Left|Right|Up|Down)|Home|End)$/.test(event.key)) {
        onCaretChange?.();
      }
    });
  }

  textarea.addEventListener("input", () => {
    if (syncing) return;
    activeField = textarea;
    pickState = null;
    if (!editing) {
      beginEdit(textarea.value, textarea, textarea.value.length);
      return;
    }
    syncFields(textarea.value);
    onStateChange?.();
  });

  formulaInput.addEventListener("input", () => {
    if (syncing) return;
    activeField = formulaInput;
    pickState = null;
    if (!editing) {
      beginEdit(formulaInput.value, formulaInput, caretOf(formulaInput));
      return;
    }
    syncFields(formulaInput.value);
    onStateChange?.();
  });

  applyTextareaLook();

  return {
    textarea,
    formulaInput,
    get editing() {
      return editing;
    },
    get composing() {
      return composing;
    },
    get buffer() {
      return buffer;
    },
    get target() {
      return target;
    },
    get activeField() {
      return activeField;
    },
    get caret() {
      return caretOf(activeField);
    },
    isTextCell,
    refs: currentRefs,
    canPick,
    settle,
    openWithInput,
    pickCell,
    pickMove,
    insertText,
    insertNewline,
    cycleRef,
    replaceRange,
    commit,
    commitWith,
    cancel,
    refreshReady,
    setAriaLabel(label) {
      if (textarea.getAttribute("aria-label") !== label) textarea.setAttribute("aria-label", label);
    },
    focus(opts) {
      textarea.focus(opts);
    },
  };
}
