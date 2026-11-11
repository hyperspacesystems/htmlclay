import { state } from "../vendor/prosemirror.vendor.js";

export function announcementPlugin() {
  return new state.Plugin({
    view(view) {
      const live = view.dom.ownerDocument.createElement("div");
      live.className = "blockeditor-live";
      live.setAttribute("role", "status");
      live.setAttribute("aria-live", "polite");
      view.dom.parentElement.appendChild(live);
      return { destroy: () => live.remove() };
    },
  });
}

export function announce(view, text) {
  const region = view.dom.parentElement?.querySelector(".blockeditor-live");
  if (!region) return;
  region.textContent = "";
  region.textContent = text;
}
