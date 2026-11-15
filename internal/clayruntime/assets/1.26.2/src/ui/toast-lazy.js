// The toast stack is fetched the first time a toast shows, so the page editor does
// not download it to become ready. Calls made before it arrives run in order once it has.
let loading = null;
// A download that failed (offline) is tried again by the next call, not remembered.
const load = () => (loading ??= import("./toast.js").catch((error) => {
  loading = null;
  throw error;
}));
const later = (name) => (...args) => { load().then((m) => (name ? m[name] : m.default)(...args)); };

export default later(null);
export const toastPersistent = later("toastPersistent");
// Fetches the stack ahead of the first toast, for a toast that may have to show offline.
export const preload = () => { load().catch(() => {}); };
// Dismissing a toast nobody showed does nothing, so it never fetches the stack: a page
// that tears down without having shown one keeps it unloaded. clay-ui shares the same
// stack, so a toast it put up (a failed save, say) is one this can take down.
export const dismissPersistent = (...args) => {
  if (loading || typeof window.clay?.toastPersistent === "function") later("dismissPersistent")(...args);
};
