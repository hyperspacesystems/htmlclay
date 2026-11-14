// Which save feedback is on the page, so the indicator chip and the automatic toasts
// never report the same save twice: the chip owns saving and saved, the toasts own a
// save that failed or went offline. Each side sets its own flag when it loads; both
// read them only when an event arrives, so load order does not matter. savedToast is
// the page's `clay.saveToast` switch: Saved goes to a toast instead of the chip.
export const saveFeedback = { chip: false, toasts: false, savedToast: false };

// Set on a failed save's event detail by a listener that put its own notice up (the block
// editor's retry and blocked notices), so the chip and the fallback toast stay out of it.
// core/save.js writes the same key without importing this file, which the edit boot does
// not otherwise fetch.
export const FAILURE_CLAIMED = Symbol.for("clay.saveFailureClaimed");
