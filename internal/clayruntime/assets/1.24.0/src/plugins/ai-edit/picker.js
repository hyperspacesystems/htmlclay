// The area picker: the overlay that turns a page into a choice of blocks. Hover
// outlines one block, a click picks it, a drag or a Shift-click extends across blocks,
// and "Whole page" stays one press away. Behaviour only: the composer, the request and
// the save are ai-edit's, and the target it hands over is whatever the surface names.
import { set } from "../../lib/hostile-css.js";
import { STRIP_FROM_SAVE } from "../../lib/region-policy.js";
import { bevelBox, bevelButton, bevelSurface, bevelText, pageScheme, paintLook, setShown } from "../../ui/bevel-controls.js";
import { TOKENS, FONT_SANS, FONT_MONO, SHADOW } from "../../ui/bevel.js";

// The same block rules ai-edit.js picks a selection's target with, so a person sees
// exactly what will be rewritten.
const BLOCK_SELECTOR = "p,h1,h2,h3,h4,h5,h6,li,dt,dd,blockquote,pre,figcaption,td,th";
const CONTAINER_SELECTOR = "section,article,aside,header,footer,nav,main,div,ul,ol,dl,table,figure,form";
const DRAG_SLOP = 4;

const isCoarse = () => typeof matchMedia === "function" && !!matchMedia("(pointer: coarse)").matches;

function marked(el, name, value) {
  el.setAttribute(name, value);
  return el;
}

// The hit-testing half for a page that is not a Block document. The overlay sits on
// top of everything, so the first element under the pointer is ClayJS chrome: it is
// skipped along with every other piece of AI chrome and everything the save strips.
export function genericSurface(doc = document) {
  const usable = (node) =>
    node instanceof doc.defaultView.Element && node !== doc.body && node !== doc.documentElement &&
    !node.closest("[data-clay-ai-edit]") && !node.closest(STRIP_FROM_SAVE);

  const targetFor = (node) => {
    if (!node || !usable(node)) return null;
    const target = node.closest(BLOCK_SELECTOR) || node.closest(CONTAINER_SELECTOR);
    return target && target !== doc.body ? target : null;
  };

  return {
    targetAt(x, y) {
      const under = typeof doc.elementsFromPoint === "function" ? doc.elementsFromPoint(x, y) : [];
      for (const node of under || []) {
        const target = targetFor(node);
        if (target) return target;
      }
      return null;
    },
    // The smallest common ancestor, then the same block and container rules upward, so
    // a drag across two paragraphs inside one section names the section.
    between(a, b) {
      if (!a) return b || null;
      if (!b || a === b) return a;
      const first = a.compareDocumentPosition(b) & 4 ? a : b;
      const last = first === a ? b : a;
      let common = first;
      while (common && !common.contains(last)) common = common.parentElement;
      if (!common) return null;
      if (common === doc.body) return common;
      if (common.matches(BLOCK_SELECTOR) || common.matches(CONTAINER_SELECTOR)) return common;
      return targetFor(common) || common;
    },
    rectOf(target) {
      return target.getBoundingClientRect();
    },
  };
}

function defaultChrome({ coarse }) {
  const scheme = `color-scheme:${pageScheme()}`;
  const part = (el, name) => marked(el, "data-clay-ai-edit-part", name);

  const overlay = marked(bevelBox("div", [
    "position:fixed", "inset:0", "z-index:99997", "background:transparent",
    "touch-action:pan-y", "cursor:crosshair", scheme,
  ]), "data-clay-ai-edit", "picker");

  // The ring is the hover outline and the drag outline both: one piece of chrome that
  // says which blocks a click would rewrite.
  const ring = marked(bevelBox("div", [
    "box-sizing:content-box", "position:fixed", "z-index:99998", "pointer-events:none",
    `border:2px solid ${TOKENS.brass}`, "border-radius:0", scheme,
  ]), "data-clay-ai-edit", "picker-ring");
  setShown(ring, false);

  const bar = marked(bevelSurface("div", [
    "position:fixed", "left:50%", "bottom:16px", "transform:translateX(-50%)",
    "z-index:99999", "width:min(35rem, calc(100vw - 32px))",
    "padding:8px 10px", "border-radius:12px", "gap:8px", "align-items:center",
    `box-shadow:${SHADOW}`, `font:13px/1.5 ${FONT_SANS}`, scheme,
  ]), "data-clay-ai-edit", "picker-bar");
  setShown(bar, false, "flex");

  const hint = part(bevelText("div", [
    "display:block", "flex:1", "min-width:0", "overflow:hidden", "text-overflow:ellipsis",
    "white-space:nowrap", `color:${TOKENS.muted}`, `font:12.5px/1.5 ${FONT_MONO}`,
  ]), "picker-hint");
  hint.textContent = "Click or drag blocks";

  const whole = part(bevelButton("Whole page", { extra: ["flex:none"] }), "picker-whole");
  const cancel = part(bevelButton("Cancel", { variant: "quiet", extra: ["flex:none"] }), "picker-cancel");
  // A tap cannot say both "this one" and "and also that one", so the touch bar keeps a
  // Continue that opens the composer once the blocks are chosen.
  const more = coarse ? part(bevelButton("Continue", { variant: "primary", extra: ["flex:none"] }), "picker-continue") : null;
  if (more) setShown(more, false, "inline-flex");
  bar.append(hint, whole, ...(more ? [more] : []), cancel);

  document.body.append(overlay, ring, bar);
  for (const el of [overlay, ring, bar]) paintLook(el);
  return { overlay, ring, bar, hint, whole, more, cancel };
}

export function createPicker({ surface, onPicked, onWholePage, onCancel, build } = {}) {
  const doc = document;
  const hits = surface || genericSurface(doc);
  let chrome = null;
  let listeners = null;
  let picking = false;
  let chosen = null;
  let down = null;

  const rectOf = (target) => {
    if (!target) return null;
    const rect = typeof hits.rectOf === "function" ? hits.rectOf(target) : target.getBoundingClientRect();
    return rect || null;
  };

  // What a click at this point would rewrite. A drag asks the surface for the span
  // between where it started and where the pointer is.
  const pointOf = (event) => hits.targetAt(event.clientX, event.clientY) || null;

  function outline(target) {
    if (!chrome) return;
    const rect = rectOf(target);
    if (!rect) { setShown(chrome.ring, false); return; }
    set(chrome.ring, "left", rect.left - 5 + "px");
    set(chrome.ring, "top", rect.top - 5 + "px");
    set(chrome.ring, "width", rect.width + 6 + "px");
    set(chrome.ring, "height", rect.height + 6 + "px");
    setShown(chrome.ring, true);
  }

  function choose(target, extend) {
    if (!target) return;
    // Shift extends from the block already chosen; on a coarse pointer every later tap
    // extends, because there is no key to hold.
    const next = chosen && (extend || isCoarse()) ? hits.between(chosen, target) || target : target;
    chosen = next;
    outline(next);
    if (chrome.more) setShown(chrome.more, true, "inline-flex");
    // A tap only selects: the composer would swallow the next tap that extends.
    if (isCoarse()) return;
    onPicked?.(next);
  }

  function onPointerDown(event) {
    if (!picking || event.button > 0) return;
    event.preventDefault();
    down = { x: event.clientX, y: event.clientY, from: pointOf(event), moved: false, shift: !!event.shiftKey };
  }

  function onPointerMove(event) {
    if (!picking) return;
    const at = pointOf(event);
    if (down) {
      const dx = event.clientX - down.x;
      const dy = event.clientY - down.y;
      if (!down.moved && dx * dx + dy * dy > DRAG_SLOP * DRAG_SLOP) down.moved = true;
      if (down.moved) { outline(down.from && at ? hits.between(down.from, at) || at : at || down.from); return; }
    }
    outline(at);
  }

  function onPointerUp(event) {
    if (!picking || !down) return;
    const start = down;
    down = null;
    const dx = event.clientX - start.x;
    const dy = event.clientY - start.y;
    const moved = start.moved || dx * dx + dy * dy > DRAG_SLOP * DRAG_SLOP;
    const at = pointOf(event);
    const target = moved
      ? (start.from && at ? hits.between(start.from, at) || at : at || start.from)
      : (at || start.from);
    if (!target) return;
    choose(target, !!(start.shift || event.shiftKey));
  }

  function onKey(event) {
    if (!picking || event.key !== "Escape") return;
    close();
    onCancel?.();
  }

  function open() {
    if (picking) return;
    picking = true;
    chosen = null;
    down = null;
    chrome = (build || defaultChrome)({ coarse: isCoarse() });
    listeners = new AbortController();
    const { signal } = listeners;
    const { overlay } = chrome;
    overlay.addEventListener("pointerdown", onPointerDown, { signal });
    overlay.addEventListener("pointermove", onPointerMove, { signal });
    overlay.addEventListener("pointerup", onPointerUp, { signal });
    overlay.addEventListener("pointercancel", () => { down = null; }, { signal });
    chrome.whole.addEventListener("click", () => { if (picking) onWholePage?.(); }, { signal });
    chrome.more?.addEventListener("click", () => { if (picking && chosen) onPicked?.(chosen); }, { signal });
    chrome.cancel.addEventListener("click", () => { close(); onCancel?.(); }, { signal });
    doc.addEventListener("keydown", onKey, { signal });
    setShown(chrome.bar, true, "flex");
    chrome.whole.focus({ preventScroll: true });
  }

  function close() {
    if (!picking && !chrome) return;
    picking = false;
    chosen = null;
    down = null;
    listeners?.abort();
    listeners = null;
    if (chrome) for (const node of [chrome.overlay, chrome.ring, chrome.bar]) node.remove();
    chrome = null;
  }

  function contains(node) {
    if (!chrome || !node) return false;
    return chrome.overlay.contains(node) || chrome.bar.contains(node);
  }

  return {
    open,
    close,
    contains,
    get picking() { return picking; },
  };
}
