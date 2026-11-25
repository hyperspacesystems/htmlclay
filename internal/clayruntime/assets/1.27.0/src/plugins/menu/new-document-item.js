const STARTING = 'script[type="application/json"][clay-new-document]';
let pending = null;

// "New blank document": this page with its data emptied, as a new file the host keeps or
// the browser downloads. Offered only where the page says what a blank copy is, with a
// starting values tag (its template's choice); a page without one would keep data its rules
// do not name. Nothing is asked of the host until the item is clicked, and a second click
// while the first is under way makes nothing more.
export function addNewDocumentItem({ addItem, note }) {
  if (!document.querySelector(STARTING)) return;
  addItem({
    id: "clay.new",
    label: "New blank document",
    run: async () => {
      if (pending) return;
      let result = null;
      pending = Promise.resolve().then(() => window.clay.newDocument.create()).catch(() => null);
      try {
        result = await pending;
      } finally {
        pending = null;
      }
      if (result?.ok && typeof result.href === "string") {
        // Opened after the host answers, so a popup blocker may refuse it; then this tab goes.
        // "noopener" would make window.open answer null even when it opened, so opener is cut here.
        const opened = window.open(result.href, "_blank");
        if (opened) opened.opener = null;
        else window.location.assign(result.href);
      } else if (!result?.ok) note(result?.message || "Couldn't make a new document. Try again.");
    },
  });
}
