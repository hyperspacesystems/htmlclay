import { FUNCTION_BY_NAME, FUNCTION_HELP } from "./function-help-data.js";
import { bevelBox, bevelButton, bevelSurface, bevelText, setShown } from "../ui/bevel-controls.js";
import { FONT_MONO, FONT_SANS, TOKENS } from "../ui/bevel.js";
import { set } from "../lib/hostile-css.js";
import { resolveTheme } from "../ui/theme.js";

export const MAX_COMPLETIONS = 8;
export const POPUP_WIDTH = 400;

const WORD_BEFORE = /[A-Za-z0-9_$'!:.]/;
const WORD_AFTER = /[A-Za-z0-9_.(]/;

const getTokenize = (custom) => custom ?? globalThis.window?.Sap?.grid?.tokenize ?? null;

export function completionAt(text, caret, { tokenize: customTokenize } = {}) {
  if (typeof text !== "string" || !text.startsWith("=")) return null;
  if (typeof caret !== "number" || caret < 2 || caret > text.length) return null;
  if (WORD_AFTER.test(text[caret] ?? "")) return null;

  const beforeCaret = text.slice(1, caret);
  const run = /([A-Za-z][A-Za-z.]*)$/.exec(beforeCaret);
  if (!run) return null;

  const prefix = run[1].toUpperCase();
  const start = caret - run[1].length;
  if (WORD_BEFORE.test(text[start - 1] ?? "")) return null;

  const tokenize = getTokenize(customTokenize);
  if (typeof tokenize === "function") {
    const tokens = tokenize(`${text.slice(0, caret)}(`);
    const idx = tokens.findIndex((t) => t.start === start && t.end === caret);
    if (idx === -1 || tokens[idx].type !== "name") return null;
    const prev = tokens[idx - 1];
    if (!prev || !(prev.type === "op" || (prev.type === "punct" && (prev.value === "(" || prev.value === ",")))) {
      return null;
    }
  } else {
    const lead = text.slice(0, start).trimEnd();
    if (!lead || !/[=+\-*/^&<>(,]$/.test(lead)) return null;
  }

  const items = FUNCTION_HELP.filter((entry) => entry.name.startsWith(prefix)).slice(0, MAX_COMPLETIONS);
  if (items.length === 0) return null;
  return { prefix, start, end: caret, items };
}

export function argumentHintAt(text, caret, { tokenize: customTokenize } = {}) {
  if (typeof text !== "string" || !text.startsWith("=")) return null;
  if (typeof caret !== "number" || caret < 2 || caret > text.length) return null;
  const tokenize = getTokenize(customTokenize);
  if (typeof tokenize !== "function") return null;

  const tokens = tokenize(text.slice(1, caret));
  const stack = [];
  for (let i = 0; i < tokens.length; i++) {
    const t = tokens[i];
    if (t.type === "punct" && t.value === "(") {
      const prev = tokens[i - 1];
      stack.push({ fnName: prev?.type === "name" ? prev.value.toUpperCase() : null, argIndex: 0 });
    } else if (t.type === "punct" && t.value === ",") {
      const top = stack[stack.length - 1];
      if (top && top.fnName !== null) top.argIndex += 1;
    } else if (t.type === "punct" && t.value === ")") {
      if (stack.length > 0) stack.pop();
    }
  }

  let call = null;
  for (let k = stack.length - 1; k >= 0; k--) {
    if (stack[k].fnName !== null) {
      call = stack[k];
      break;
    }
  }
  if (!call) return null;
  const entry = FUNCTION_BY_NAME.get(call.fnName);
  if (!entry) return null;

  let activeArgIndex = -1;
  if (entry.args.length > 0) {
    if (call.argIndex < entry.args.length) {
      activeArgIndex = call.argIndex;
    } else {
      const last = entry.args[entry.args.length - 1];
      if (last.includes("...")) {
        if (entry.args.length >= 2 && entry.args[entry.args.length - 2].includes("...")) {
          const base = entry.args.length - 2;
          activeArgIndex = base + ((call.argIndex - base) % 2);
        } else {
          activeArgIndex = entry.args.length - 1;
        }
      }
    }
  }

  return { entry, argIndex: call.argIndex, activeArgIndex };
}

export function createFunctionHelp({
  isDocked = () => false,
  container,
  win = window,
  theme = resolveTheme(),
  editor,
  overlays,
  getTokenize: customGetTokenize = null,
} = {}) {
  const resolveTokenize = () => (typeof customGetTokenize === "function" ? customGetTokenize() : win?.Sap?.grid?.tokenize);

  const listSurface = bevelSurface(
    "div",
    [
      "display:none",
      "position:fixed",
      "left:0px",
      "top:0px",
      `width:${POPUP_WIDTH}px`,
      "max-width:calc(100vw - 16px)",
      "z-index:2147483000",
      "flex-direction:column",
      "gap:2px",
      "padding:4px",
      `color-scheme:${theme.scheme}`,
      `box-shadow:6px 6px 0 color-mix(in srgb, ${TOKENS.ink} 10%, transparent)`,
    ],
    { theme },
  );
  listSurface.setAttribute("role", "listbox");
  listSurface.setAttribute("aria-label", "Formula completions");
  listSurface.setAttribute("data-function-completions", "");
  listSurface.hidden = true;

  const hintSurface = bevelSurface(
    "div",
    [
      "display:none",
      "position:fixed",
      "left:0px",
      "top:0px",
      `width:${POPUP_WIDTH}px`,
      "max-width:calc(100vw - 16px)",
      "z-index:2147483000",
      "flex-direction:column",
      "gap:4px",
      "padding:8px 10px",
      `color-scheme:${theme.scheme}`,
      `box-shadow:6px 6px 0 color-mix(in srgb, ${TOKENS.ink} 10%, transparent)`,
    ],
    { theme },
  );
  hintSurface.setAttribute("data-function-hint", "");
  hintSurface.hidden = true;

  const sigRow = bevelBox("div", [`font:500 12px/1.4 ${FONT_MONO}`, `color:${TOKENS.ink}`, "overflow-wrap:anywhere"], { theme });
  sigRow.setAttribute("data-function-signature", "");
  const summaryRow = bevelText(
    "div",
    [`font:400 12px/1.4 ${FONT_SANS}`, `color:${TOKENS["ink-2"]}`],
    "",
    { theme },
  );
  summaryRow.setAttribute("data-function-summary", "");
  hintSurface.append(sigRow, summaryRow);

  // The editor keeps focus: a click on either popup's padding would blur it while it still edits.
  for (const surface of [listSurface, hintSurface]) surface.addEventListener("mousedown", (event) => event.preventDefault());
  container.append(listSurface, hintSurface);

  let activeCompletion = null;
  let selectedIndex = 0;
  let dismissedKey = "";
  let optionButtons = [];

  const isTooNarrow = () => {
    const w = win?.innerWidth ?? 0;
    return w > 0 && w < POPUP_WIDTH;
  };

  function placeUnderEditor(surface) {
    const field = editor.activeField;
    const anchorEl = field === editor.formulaInput ? editor.formulaInput : (overlays?.editorSlot ?? editor.textarea);
    const rect = anchorEl?.getBoundingClientRect?.() ?? { left: 0, bottom: 0 };
    const maxW = win?.innerWidth ?? 0;
    const maxH = win?.innerHeight ?? 0;
    const left = maxW > POPUP_WIDTH + 16 ? Math.max(8, Math.min(rect.left, maxW - POPUP_WIDTH - 8)) : Math.max(4, rect.left);
    const top = maxH > 180 ? Math.max(4, Math.min(rect.bottom + 4, maxH - 140)) : Math.max(0, rect.bottom + 4);
    set(surface, "left", `${left}px`);
    set(surface, "top", `${top}px`);
  }

  function hideAll() {
    activeCompletion = null;
    optionButtons = [];
    setShown(listSurface, false, "flex");
    setShown(hintSurface, false, "flex");
  }

  function paintSelectedOption() {
    optionButtons.forEach(({ btn, baseExtra, stateExtra }, idx) => {
      const sel = idx === selectedIndex;
      btn.setAttribute("aria-selected", sel ? "true" : "false");
      stateExtra.splice(
        0,
        stateExtra.length,
        ...baseExtra,
        ...(sel ? [`background:${TOKENS.sunk}`, `border:1px solid ${TOKENS["line-2"]}`] : []),
      );
      btn.pin({});
    });
  }

  function acceptCompletion(index = selectedIndex) {
    if (!activeCompletion) return false;
    const chosen = activeCompletion.items[index];
    if (!chosen) return false;
    const { start, end } = activeCompletion;
    dismissedKey = "";
    return editor.replaceRange(start, end, `${chosen.name}(`);
  }

  function renderCompletions(comp) {
    const prevSelectedName = activeCompletion?.items?.[selectedIndex]?.name ?? "";
    activeCompletion = comp;
    const keptIdx = comp.items.findIndex((it) => it.name === prevSelectedName);
    selectedIndex = keptIdx !== -1 ? keptIdx : 0;

    listSurface.textContent = "";
    optionButtons = comp.items.map((item, idx) => {
      const baseExtra = [
        "display:flex",
        "width:100%",
        "height:auto",
        "min-height:30px",
        "padding:5px 8px",
        "justify-content:flex-start",
        "text-align:left",
      ];
      const stateExtra = [...baseExtra];
      const btn = bevelButton("", {
        small: true,
        variant: "quiet",
        theme,
        onClick: () => acceptCompletion(idx),
        extra: stateExtra,
        labelExtra: [
          "display:flex",
          "flex-direction:column",
          "align-items:flex-start",
          "gap:1px",
          "width:100%",
          "white-space:normal",
          "text-align:left",
        ],
      });
      btn.setAttribute("role", "option");
      btn.setAttribute("data-completion-item", item.name);
      btn.tabIndex = -1;
      btn.addEventListener("mousedown", (e) => e.preventDefault());
      const span = btn.firstChild;
      span.textContent = "";
      const nameEl = bevelText("span", [`font:600 12px/1.25 ${FONT_MONO}`, `color:${TOKENS.ink}`], item.name, {
        theme,
        role: null,
      });
      nameEl.setAttribute("data-completion-name", "");
      const sumEl = bevelText("span", [`font:400 11.5px/1.3 ${FONT_SANS}`, `color:${TOKENS.muted}`], item.summary, {
        theme,
        role: "mutedText",
      });
      sumEl.setAttribute("data-completion-summary", "");
      span.append(nameEl, sumEl);
      listSurface.append(btn);
      return { btn, baseExtra, stateExtra };
    });

    paintSelectedOption();
    setShown(hintSurface, false, "flex");
    placeUnderEditor(listSurface);
    setShown(listSurface, true, "flex");
  }

  function renderHint(hint) {
    activeCompletion = null;
    optionButtons = [];
    setShown(listSurface, false, "flex");

    const { entry, activeArgIndex } = hint;
    sigRow.textContent = "";
    sigRow.append(bevelText("span", ["font:inherit", `color:${TOKENS.ink}`, "font-weight:600"], `${entry.name}(`, { theme, role: null }));
    entry.args.forEach((arg, idx) => {
      if (idx > 0) {
        sigRow.append(bevelText("span", ["font:inherit", `color:${TOKENS.muted}`], ", ", { theme, role: null }));
      }
      if (idx === activeArgIndex) {
        const boldEl = bevelText(
          "strong",
          [
            "font:inherit",
            "font-weight:700",
            `color:${TOKENS.brass}`,
            `background:${TOKENS["brass-soft"]}`,
            "padding:0 2px",
          ],
          arg,
          { theme, role: null },
        );
        boldEl.setAttribute("data-active-arg", "");
        sigRow.append(boldEl);
      } else {
        sigRow.append(bevelText("span", ["font:inherit", `color:${TOKENS["ink-2"]}`], arg, { theme, role: null }));
      }
    });
    sigRow.append(bevelText("span", ["font:inherit", `color:${TOKENS.ink}`, "font-weight:600"], ")", { theme, role: null }));
    summaryRow.textContent = entry.summary;

    placeUnderEditor(hintSurface);
    setShown(hintSurface, true, "flex");
  }

  function update() {
    if (!editor.editing || editor.isTextCell() || !editor.buffer.startsWith("=") || isTooNarrow() || isDocked()) {
      dismissedKey = "";
      hideAll();
      return;
    }
    const text = editor.buffer;
    const caret = editor.caret;
    const stateKey = `${text}\u0000${caret}`;
    if (dismissedKey && dismissedKey !== stateKey) dismissedKey = "";

    const tokenize = resolveTokenize();
    if (dismissedKey !== stateKey) {
      const comp = completionAt(text, caret, { tokenize });
      if (comp) {
        renderCompletions(comp);
        return;
      }
    }

    const hint = argumentHintAt(text, caret, { tokenize });
    if (hint) {
      renderHint(hint);
      return;
    }
    hideAll();
  }

  function handleKeyDown(event) {
    if (!activeCompletion || listSurface.hidden || editor.composing || event.isComposing || event.keyCode === 229) {
      return false;
    }
    const plain = !event.ctrlKey && !event.metaKey && !event.altKey;
    if (event.key === "ArrowDown" && plain && !event.shiftKey) {
      event.preventDefault();
      selectedIndex = (selectedIndex + 1) % activeCompletion.items.length;
      paintSelectedOption();
      return true;
    }
    if (event.key === "ArrowUp" && plain && !event.shiftKey) {
      event.preventDefault();
      selectedIndex = (selectedIndex - 1 + activeCompletion.items.length) % activeCompletion.items.length;
      paintSelectedOption();
      return true;
    }
    if ((event.key === "Tab" || event.key === "Enter") && plain && !event.shiftKey) {
      event.preventDefault();
      acceptCompletion(selectedIndex);
      return true;
    }
    if (event.key === "Escape" && plain && !event.shiftKey) {
      event.preventDefault();
      dismissedKey = `${editor.buffer}\u0000${editor.caret}`;
      activeCompletion = null;
      optionButtons = [];
      setShown(listSurface, false, "flex");
      const hint = argumentHintAt(editor.buffer, editor.caret, { tokenize: resolveTokenize() });
      if (hint && !isTooNarrow()) renderHint(hint);
      return true;
    }
    return false;
  }

  return {
    listSurface,
    hintSurface,
    get isCompletionOpen() {
      return Boolean(activeCompletion && !listSurface.hidden);
    },
    get isHintOpen() {
      return !hintSurface.hidden;
    },
    update,
    handleKeyDown,
    dispose() {
      hideAll();
      listSurface.remove();
      hintSurface.remove();
    },
  };
}
