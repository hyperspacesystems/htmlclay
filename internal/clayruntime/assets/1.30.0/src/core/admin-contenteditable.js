import { isEditMode, isOwner } from "./is-edit-mode.js";
import onDomReady from "../lib/dom-ready.js";
import { addDocumentTransform, originalSnapshotNode } from "./snapshot.js";
import { authoredAttr, canonicalContentEditable, captureAuthored, restoreAuthoredOrder } from "./preserve-authored.js";

export const SELECTOR = '[editmode\\:contenteditable]';

// `editable="plain"`: plain text, editable in edit mode, and nothing in the file but that
// one attribute. The file never holds `contenteditable`, so there is no inert spelling and
// no template form: edit mode adds it, and leaving edit and saving take it away.
// `clay-editable` is the spelling for a custom element that owns the bare name.
export const PLAIN_SELECTOR = '[editable~="plain"], [clay-editable~="plain"]';

// Plain fields inside <template> content are activated too, so a clone is editable the
// moment it is inserted: the code that inserts it can focus it at once, and undo records
// the row as it will stay. A template's content is its own fragment, outside the page's
// mutation watchers, so this records nothing; leaving edit and every save take it away.
function eachPlain(root, fn) {
  root.querySelectorAll(PLAIN_SELECTOR).forEach(fn);
  root.querySelectorAll("template").forEach(template => eachPlain(template.content, fn));
}

function disablePlain(root) {
  eachPlain(root, el => el.removeAttribute("contenteditable"));
}

export function disableContentEditableBeforeSave () {
  addDocumentTransform(docElem => {
    docElem.querySelectorAll(SELECTOR).forEach(el => {
      canonicalContentEditable(el);
      restoreAuthoredOrder(el, originalSnapshotNode(el) ?? el);
    });
    disablePlain(docElem);
  }, { required: true });
}

export function enableContentEditableForAdminOnPageLoad () {
  if (!isEditMode) return;

  onDomReady(() => {
    enableContentEditable();
  });
}

// Runtime toggle functions. `root` lets scoped live sync activate a parsed
// incoming document the same way boot activates the live one.
export function enableContentEditable(root = document) {
  root.querySelectorAll(SELECTOR).forEach(el => {
    captureAuthored(el);
    // Already active (a second activation of the same node) keeps the value it already
    // holds, so re-running this never downgrades `plaintext-only` to `true`.
    const val = el.getAttribute("inert-contenteditable")
      ?? el.getAttribute("contenteditable")
      ?? authoredAttr(el, "inert-contenteditable")
      ?? "true";
    // Keywords match ASCII case-insensitively, so `FALSE` means false. The author's own
    // spelling is what gets written, so the save transform puts it back unchanged.
    const keyword = val.toLowerCase();
    el.setAttribute("contenteditable", ["", "true", "false", "plaintext-only"].includes(keyword) ? val : "true");
    el.removeAttribute("inert-contenteditable");
  });
  eachPlain(root, el => {
    if (!el.hasAttribute("contenteditable")) el.setAttribute("contenteditable", "plaintext-only");
  });
}

export function disableContentEditable() {
  document.querySelectorAll(SELECTOR).forEach(el => {
    canonicalContentEditable(el);
    restoreAuthoredOrder(el);
  });
  disablePlain(document);
}

// Auto-initialize
export function init() {
  disableContentEditableBeforeSave();
  enableContentEditableForAdminOnPageLoad();
}
