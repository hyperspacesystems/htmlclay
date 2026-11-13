import { saveFeedback } from "./save-feedback.js";
import { toastPersistent, dismissPersistent, preload } from "../ui/toast-lazy.js";

// A failed save is never silent. core/save.js fetches this file at the person's first
// input and hands it every failed save no listener claimed. A page with the indicator chip, clay-ui or its own
// notices (`clay.saveFeedback = "own"`) already reports failures, so nothing is added
// there. Everywhere else the toast stack shows what clay-ui would have shown. Core keeps
// calling in for every later save, so a failure the block editor claims stays its own,
// and the next save that lands takes the toast down. The toast stack is the block
// editor's too, so its dismissals reach these toasts.
const TOASTS = {
  error: ["Couldn't save", "error"],
  offline: ["Offline, will retry", "warning"],
};

let shown = false;

const unheard = () => !(saveFeedback.chip || saveFeedback.toasts || window.clay?.saveFeedback === "own");

// core/save.js calls this at the person's first input, so the toast can still show offline.
export function warm() {
  if (unheard()) preload();
}

export function reportFailure(state) {
  if (!TOASTS[state] || !unheard()) return;
  shown = true;
  // One failure notice at a time: a connection that comes and goes swaps them.
  for (const [other, [message]] of Object.entries(TOASTS)) if (other !== state) dismissPersistent(message);
  toastPersistent(...TOASTS[state]);
}

export function clearFailures() {
  if (!shown) return;
  shown = false;
  for (const [message] of Object.values(TOASTS)) dismissPersistent(message);
}
