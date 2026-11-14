// a nice, simple alert
// ❗️ don't use too much text!
//
// The Hyperclay dashboard's toast (Bevel's React toast with the dashboard's overrides),
// drawn inline: a 3px tone edge, a 16px Bevel glyph, 13.5px semibold text, a quiet close.
import { set, style } from "../lib/hostile-css.js";
import { bevelBox, bevelSurface, bevelText, bevelButton, protectIcon, paintLook, restyleBox } from "./bevel-controls.js";
import { TOKENS, RULES, GLYPHS } from "./bevel.js";
import { resolveTheme } from "./theme.js";
import { themed, tokenRulesFor, partRules, markParts } from "./theme-parts.js";
import { mountEditorUI } from "./editor-ui.js";

const TONES = {
  success: { edge: TOKENS.teal, icon: GLYPHS.toastSuccess },
  error: { edge: TOKENS.ox, icon: GLYPHS.toastWarning },
  warning: { edge: TOKENS.brass, icon: GLYPHS.toastWarning },
  info: { edge: TOKENS.brass, icon: GLYPHS.toastInfo },
};

const CLOSE_ICON = GLYPHS.toastClose;

const HIDDEN = ["opacity:0", "transform:translateX(24px)"];

// The detail line and the action row start where the title's text does: after the 16px
// glyph and the 8px gap beside it. A quiet small button's label sits 10px inside the button
// (a 2px border and 8px of padding), so its row starts that much earlier.
const TEXT_INSET = 24;
const BUTTON_LABEL_INSET = 10;

function motion() {
  const reduced = typeof matchMedia === "function" && matchMedia("(prefers-reduced-motion: reduce)").matches;
  return reduced ? "none" : "opacity .5s ease-in-out, transform .5s ease-in-out";
}

// Looked up on every call, never cached: a live-sync morph or a caller can empty the
// body, and a detached stack would swallow every toast after it. The stack sits in its own
// shadow root: the inline !important rules already beat the page's stylesheets, and the
// root also keeps out what they cannot, such as a page's ::before content.
function stack(theme) {
  const existing = document.querySelector('clay-editor-ui[data-clay-ui="toasts"]')?.shadowRoot.querySelector("[data-clay-toasts]");
  if (existing) return existing;
  const box = bevelBox("div", [
    "position:fixed", "top:18px", "right:18px", "z-index:9999",
    "display:flex", "flex-direction:column", "align-items:flex-end", "gap:10px",
    "width:min(330px, calc(100vw - 36px))", "pointer-events:none",
  ], { theme, parts: ["toast.stack"] });
  box.setAttribute("data-clay-toasts", "");
  box.setAttribute("aria-live", "polite");
  mountEditorUI({ name: "toasts", content: box });
  return box;
}

function build(message, type, { detail = "", actions = [] } = {}, theme) {
  const tone = TONES[type];
  const el = bevelSurface("div", [
    "display:flex", "align-items:flex-start", ...RULES.toast, "width:100%",
    `border-inline-start:3px solid ${tone.edge}`,
    "cursor:pointer", "pointer-events:auto",
    ...HIDDEN, `transition:${motion()}`,
  ], { theme, parts: ["toast.panel"] });
  if (themed(theme)) style(el, [`border-inline-start-color:${tone.edge}`, ...partRules(theme, ["toast.panel"], {})]);
  el.setAttribute("data-clay-toast", type);
  const body = bevelBox("div", ["flex:1 1 auto", "min-width:0", "display:flex", "flex-direction:column", "gap:8px"], { theme });
  const head = bevelBox("div", ["display:flex", "align-items:flex-start", "gap:8px"], { theme });
  const glyph = bevelText("span", ["display:inline-grid", "flex:none", "padding-top:1px", `color:${tone.edge}`], undefined, { theme, parts: ["toast.icon"], role: null });
  glyph.innerHTML = tone.icon;
  protectIcon(glyph.firstElementChild, 16);
  // A message can carry a filename, and a filename is whatever whoever wrote the
  // file chose. It goes in as text so markup in a name can never become markup here.
  const text = bevelText("span", ["flex:1 1 auto", "min-width:120px", ...RULES.toastTitle, "overflow-wrap:anywhere"], message, { theme, parts: ["toast.text"] });
  head.append(glyph, text);
  body.append(head);
  // Detail and actions are the same untrusted text as the title: textContent for
  // both, so a reason string can never become markup in the toast either.
  if (detail) {
    body.append(bevelText("span", ["display:block", `padding-inline-start:${TEXT_INSET}px`, `color:${TOKENS.muted}`, "overflow-wrap:anywhere"], detail, { theme, parts: ["toast.text"], role: "mutedText" }));
  }
  if (actions.length) {
    const row = bevelBox("div", ["display:flex", "flex-wrap:wrap", "gap:8px", `padding-inline-start:${TEXT_INSET - BUTTON_LABEL_INSET}px`], { theme });
    for (const { label, onClick } of actions) {
      row.append(bevelButton(label, { small: true, variant: "quiet", onClick, extra: ["padding:4px 8px"], theme }));
    }
    body.append(row);
  }
  el.append(body);
  return el;
}

// The dashboard's toast close: a bare 24px square in the muted ink, the sunk ground on hover.
function closeButton(onClick, theme) {
  const base = [
    "flex:none", "display:inline-grid", "place-items:center", ...RULES.toastClose,
    "cursor:pointer",
  ];
  const b = bevelBox("button", base);
  b.type = "button";
  b.setAttribute("aria-label", "Dismiss");
  b.title = "Dismiss";
  b.innerHTML = CLOSE_ICON;
  if (!themed(theme)) {
    protectIcon(b.firstElementChild, 14);
    const paint = (lit) => {
      set(b, "color", lit ? TOKENS.ink : TOKENS.muted);
      set(b, "background", lit ? TOKENS.sunk : "transparent");
    };
    b.addEventListener("pointerenter", () => paint(true));
    b.addEventListener("pointerleave", () => paint(false));
    b.addEventListener("focus", () => { paint(true); set(b, "outline", `2px solid ${TOKENS.brass}`); set(b, "outline-offset", "1px"); });
    b.addEventListener("blur", () => { paint(false); set(b, "outline", "none"); });
    b.addEventListener("click", onClick);
    return b;
  }
  const names = ["close", "toast.close"];
  const icon = ["icon", "toast.closeIcon"];
  markParts(b, theme, names);
  markParts(b.firstElementChild, theme, icon);
  const flags = { hover: false, focus: false, pressed: false };
  const paint = () => {
    const act = { hover: flags.hover, active: flags.pressed, focus: flags.focus };
    const svg = b.firstElementChild;
    svg.style.cssText = "";
    protectIcon(svg, 14);
    style(svg, partRules(theme, icon, act));
    restyleBox(b, [...base, ...tokenRulesFor(theme, "close", act), ...partRules(theme, names, act)]);
  };
  b.addEventListener("pointerenter", () => { flags.hover = true; paint(); });
  b.addEventListener("pointerleave", () => { flags.hover = false; flags.pressed = false; paint(); });
  b.addEventListener("pointerdown", (e) => {
    if (e.button !== undefined && e.button !== 0) return;
    flags.pressed = true;
    paint();
  });
  b.addEventListener("pointerup", () => { flags.pressed = false; paint(); });
  b.addEventListener("focus", () => { flags.focus = true; paint(); });
  b.addEventListener("blur", () => { flags.focus = false; flags.pressed = false; paint(); });
  b.addEventListener("click", onClick);
  paint();
  return b;
}

function reveal(el, theme) {
  stack(theme).append(el);
  if (themed(theme)) set(el, "color-scheme", theme.scheme === "light dark" ? "light" : theme.scheme);
  else paintLook(el, { background: TOKENS.surface });
  setTimeout(() => {
    set(el, "opacity", "1");
    set(el, "transform", "none");
  }, 10);
}

function dismiss(el, after) {
  set(el, "opacity", "0");
  set(el, "transform", "translateX(24px)");
  setTimeout(() => {
    el.remove();
    after?.();
  }, 500);
}

function toast(message, messageType = "success") {
  const theme = resolveTheme();
  const el = build(message, TONES[messageType] ? messageType : "success", {}, theme);
  el.addEventListener("click", () => dismiss(el));
  reveal(el, theme);
  setTimeout(() => dismiss(el), 6600);
}

// Track active persistent toasts by message
const activePersistentToasts = new Map();

// Persistent toast function - doesn't auto-dismiss, requires click to close
function toastPersistent(message, messageType = "warning", { detail = "", actions = [] } = {}) {
  const existing = activePersistentToasts.get(message);
  if (existing) {
    dismiss(existing);
    activePersistentToasts.delete(message);
  }

  const theme = resolveTheme();
  const el = build(message, TONES[messageType] ? messageType : "warning", { detail, actions }, theme);
  set(el, "cursor", "default");
  const close = closeButton((e) => {
    e.stopPropagation();
    // A newer toast with the same message may have taken this entry since.
    dismiss(el, () => { if (activePersistentToasts.get(message) === el) activePersistentToasts.delete(message); });
  }, theme);
  el.append(close);

  activePersistentToasts.set(message, el);
  reveal(el, theme);
}

// Takes down the persistent toast showing this message, if one is up. `now` takes the
// node out at once: a session that is over has no one left to read the fade.
function dismissPersistent(message, { now = false } = {}) {
  const el = activePersistentToasts.get(message);
  if (!el) return;
  activePersistentToasts.delete(message);
  if (now) {
    el.remove();
    return;
  }
  dismiss(el);
}

export { toastPersistent, dismissPersistent };
export default toast;
