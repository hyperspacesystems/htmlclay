// The save chip's drawing: loaded at the person's first input or the first save state,
// so a page nobody edits never pays for the surface it is drawn with.
import { set, style } from "../lib/hostile-css.js";
import { bevelSurface, pageScheme, paintLook } from "../ui/bevel-controls.js";
import { TOKENS, FONT_SANS } from "../ui/bevel.js";
import { saveFeedback, FAILURE_CLAIMED } from "../core/save-feedback.js";

// No 'conflict' here on purpose. core/conflict-notice.js owns that state now,
// and it ships in every document rather than only the ones that turned this chip on.
// Both listen to clay:save-conflict, so keeping a label here put a chip in the corner
// saying the same thing as the bar at the same moment. Dropping it from this side
// leaves core unaware that this plugin exists, which is the direction that
// dependency has to point.
const LABELS = {
  error: "Couldn't save",
  offline: "Offline, will retry",
  blocked: "Can't save here",
  unavailable: "Can't store changes: download a copy",
};

// The chip has words only for a save that did not go through. A save on its way says
// nothing and leaves a standing failure up, and a save that landed takes the chip down.
// The block editor's retrying takes down a plain "Couldn't save" (it keeps trying on its
// own and raises its own notice if that runs long) but leaves "Offline, will retry" up,
// which is still true.
const QUIET = new Set(["saving", "pending", "acknowledged"]);
const CLEARS = new Set(["saved", "checkpointed"]);

const ALARM = [
  `background:${TOKENS["ox-soft"]}`,
  `color:${TOKENS.ox}`,
  `border-color:color-mix(in srgb, ${TOKENS.ox} 28%, ${TOKENS["ox-soft"]})`,
];

let el = null;

function ensure() {
  // Looked up on every call, never trusted once detached: a live-sync morph or a
  // caller can empty the body, and a detached chip would swallow every later state.
  if (el && el.isConnected) return el;
  const reduced = window.matchMedia?.("(prefers-reduced-motion: reduce)").matches;
  el = bevelSurface("div", [
    "position:fixed", "right:16px", "bottom:16px", "z-index:2147483000",
    "padding:4px 12px", `font:500 13px/1.6 ${FONT_SANS}`,
    "opacity:0", `transition:${reduced ? "none" : "opacity .25s"}`, "pointer-events:none",
    `color-scheme:${pageScheme()}`,
  ]);
  el.setAttribute("data-clay-indicator", "");
  el.setAttribute("role", "status");
  document.body.appendChild(el);
  return el;
}

function hide() {
  if (!el) return;
  set(el, "opacity", "0");
  el.textContent = "";
  delete el.dataset.state;
}

// Another notice is already saying this, so the chip says nothing: two save notices at once
// read as one problem reported twice.
export function standDown() {
  hide();
}

export function show(state, detail) {
  if (QUIET.has(state)) return;
  if (CLEARS.has(state)) return hide();
  if (state === "retrying") {
    if (el?.dataset.state === "error") hide();
    return;
  }
  if (!LABELS[state]) return;
  // The block editor put its own notice up for this failure. Read here, after every
  // listener has seen the event, so the order the two subscribed in does not matter.
  if (state === "error" && detail?.[FAILURE_CLAIMED]) return;
  // A failed or offline save is the persistent toast's to report when clay-ui is loaded.
  // Blocked is the exception: the page cannot save at all, so the chip stays even beside
  // the generic toast.
  if (saveFeedback.toasts && state !== "blocked") {
    hide();
    return;
  }
  const node = ensure();
  node.textContent = LABELS[state];
  node.dataset.state = state;
  style(node, ALARM);
  paintLook(node, { background: null });
  set(node, "opacity", "1");
}

export function remove() {
  el?.remove();
  el = null;
}
