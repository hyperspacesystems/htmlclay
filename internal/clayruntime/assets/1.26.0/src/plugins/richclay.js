// richclay's toolbar, menus, floating toolbar and link dialog, raised above the host
// page by its skin. The vendor bundle is unchanged; the skin goes in first, so the
// editor never draws a frame of chrome the page can repaint.
import { installSkin } from "../ui/vendor-skin.js";
import { CSS, ROOTS } from "../ui/skins/richclay.js";
import { RichClay } from "../vendor/richclay.vendor.js";
import { startsAtImport } from "../core/is-edit-mode.js";
import { stripEphemeral } from "../lib/ephemeral.js";
import { loadedEngine, mergeEngine } from "../sync/merge-engine.js";

installSkin("richclay", CSS, { roots: ROOTS });

// A field the author kept out of page undo: its own literal no-undo (not the one RichClay
// adds while it runs), a clay="no-undo" token on it, or an ancestor region with no-undo.
const authoredNoUndo = (node) => {
  if (node.hasAttribute("no-undo") && !node.hasAttribute("data-richclay-runtime-no-undo")) return true;
  if ((node.getAttribute("clay") || "").split(/\s+/).includes("no-undo")) return true;
  const parent = node.parentElement;
  return !!parent && window.clay?.region?.resolveRegionPolicy?.(parent)?.undoable === false;
};
// The editor tidies a field's source whitespace the first time it is typed in. Whitespace
// that sits beside a block, or at the start or end of one, shows nothing, so it never
// counts as a change: a person's first letter is one step, and the tidy is not a step of
// its own or part of another field's.
const BLOCK = /^(P|DIV|UL|OL|LI|H[1-6]|BLOCKQUOTE|TABLE|THEAD|TBODY|TR|TD|TH|FIGURE|FIGCAPTION|HR|SECTION|ARTICLE|ASIDE|HEADER|FOOTER|DL|DT|DD)$/;
const isBlock = (node) => node?.nodeType === 1 && BLOCK.test(node.nodeName);
const dropLayoutWhitespace = (root) => {
  const walker = root.ownerDocument.createTreeWalker(root, 4);
  const blank = [];
  for (let text = walker.nextNode(); text; text = walker.nextNode()) {
    if (/\S/.test(text.data) || text.parentElement?.closest("pre")) continue;
    const parent = text.parentNode;
    const edge = !text.previousSibling || !text.nextSibling;
    if ((edge && (parent === root || isBlock(parent))) || isBlock(text.previousSibling) || isBlock(text.nextSibling)) blank.push(text);
  }
  blank.forEach((text) => text.remove());
};
// Ephemeral attributes (selection marks and the like) never save, so they come off a
// copy before the comparison, as they do for every other node undo certifies.
const savedForm = (node) => {
  const copy = node.cloneNode(true);
  stripEphemeral(copy);
  dropLayoutWhitespace(copy);
  return RichClay.savedContent(copy);
};

// Parts of a field that history does not own (editor chrome, a widget the author marked
// no-undo) never count as a change: the sample holds a placeholder for each, and a replay
// puts back what each one holds now.
const UNOWNED = '[editor-ui], [clay~="editor-ui"], [no-undo]:not([data-richclay-runtime-no-undo]), [clay~="no-undo"]';
const outermost = (root) => [...root.querySelectorAll(UNOWNED)].filter((el) => {
  for (let at = el.parentElement; at && at !== root; at = at.parentElement) if (at.matches(UNOWNED)) return false;
  return true;
});
const historyForm = (node) => {
  const copy = node.cloneNode(true);
  outermost(copy).forEach((el) => el.replaceWith(copy.ownerDocument.createElement("clay-kept")));
  return savedForm(copy);
};
const withUnowned = (element, html) => {
  if (!html.includes("<clay-kept")) return html;
  const template = element.ownerDocument.createElement("template");
  template.innerHTML = html;
  const live = outermost(element);
  [...template.content.querySelectorAll("clay-kept")].forEach((slot, at) => {
    if (live[at]) slot.replaceWith(live[at].cloneNode(true));
    else slot.remove();
  });
  return template.innerHTML;
};

// Undo certifies a rich field by the content it would save, so editor runtime state
// never refuses a step and a real text change always does. A field the author marked
// no-undo keeps its protection.
const richForm = (node) => (!node.matches?.(RichClay.selector) || authoredNoUndo(node) ? undefined : savedForm(node));

// The field's own typing is a step of the page history: the engine reads the content the
// field would save and, on a replay, hands it back through the host write. The editor's own
// history is off while the page owns it, so a field's Cmd+Z and its toolbar reach the same
// stack as everything else.
// The engine rebaselines the field right after the write, so the write must land before
// it returns. A loaded merge engine morphs at once. The first undo asks for it, so a boot
// pays nothing; until it lands a plain innerHTML write is exact for saved content (checked
// in Chrome), except in a field holding editor chrome, which only the morph keeps.
const writeInner = (root, html) => { root.innerHTML = html; };
const morphNow = (field) => {
  const engine = loadedEngine();
  if (engine) return engine.morph;
  mergeEngine().catch(() => {});
  return field.querySelector('[editor-ui], [clay~="editor-ui"]') ? undefined : writeInner;
};

// Rich fields belong to the page history only when the page records typing: in commands
// mode the page's history holds commands alone, and a field's own history is left to the
// editor, which is then the only history that has the typing.
const pageOwnsTyping = () => window.clay?.undo?.pageHistory === true && window.clay.undo.recordingMode !== "commands";

const tracked = new Map();

const track = (field) => {
  const undo = window.clay?.undo;
  if (!pageOwnsTyping() || authoredNoUndo(field)) return;
  tracked.set(field, undo.trackContent(field, { read: historyForm, write: (element, html) => { RichClay.setSavedContent(element, withUnowned(element, html), { morph: morphNow(element) }); } }));
  if (!loadedEngine() && field.querySelector('[editor-ui], [clay~="editor-ui"]') && typeof undo.hold === "function") {
    const release = undo.hold();
    mergeEngine().catch(() => {}).finally(release);
  }
};

const untrackAll = () => {
  for (const dispose of tracked.values()) dispose();
  tracked.clear();
};

document.addEventListener("richclay:mount", (event) => track(event.target));

// The rich form is handed to the page's undo when its session starts, rather than through
// a shared module, so a page without undo loads nothing for it. Each undo engine is asked
// once.
let formedFor = null;
const followHistory = () => {
  const undo = window.clay?.undo;
  if (undo?.addDurableForm && formedFor !== undo) {
    undo.addDurableForm(richForm);
    formedFor = undo;
  }
  RichClay.hostHistory = pageOwnsTyping();
  RichClay.keepsOwnHistory = authoredNoUndo;
  if (!RichClay.hostHistory) untrackAll();
  else document.querySelectorAll("[data-richclay-active]").forEach(track);
};

document.addEventListener("clay:undo-session", followHistory);
document.addEventListener("clay:undo-recording", followHistory);
// A boot starts the undo session while its own module is still being imported, so the
// session event can land before the loader has put `clay.undo` on the page: once every
// plugin is attached the same answer is asked for again.
document.addEventListener("clay:ready", followHistory);

document.addEventListener("richclay:history-exhausted", (event) => {
  const undo = window.clay?.undo;
  if (!undo || window.clay.modeLifecycle?.canWrite?.() === false) return;
  if (event.detail?.direction === "redo") undo.redo();
  else undo.undo();
});

let session = null;

const fault = (message, code) => Object.assign(new Error(`clay:richclay: ${message}`), { code });

const isStale = (context) => {
  if (!context) return false;
  if (context.signal?.aborted) return true;
  return typeof context.isCurrent === "function" && !context.isCurrent();
};

const ownership = (root) => ({
  root,
  win: root.ownerDocument?.defaultView || window,
  admitted: false,
  stopWatch: null,
  subscriber: null,
});

const elements = (record) => [...record.root.querySelectorAll(RichClay.selector)];

// Mounts the regions the page has, and repairs the ones an applied frame stripped: a frame's
// snapshot carries none of RichClay's runtime state, and the merge removes live attributes
// the merged page lacks, so an applied frame leaves editors that can no longer be typed
// into. RichClay's reattach() restores them.
function mount(record) {
  for (const editor of RichClay.init(elements(record))) editor.reattach();
}

// One edit session over the live DOM. `enable` mounts the page's regions and starts the one
// watcher for regions that arrive later; `quiesce` closes admission, so a region that
// arrives while the exit runs is given no editor; `resume` is the refused exit, over the
// same session; `dispose` destroys every instance and removes exactly the subscription it
// added, so the view it leaves behind holds no editor and the next entry begins from
// nothing.
function richclayLease(record) {
  const onFrame = () => mount(record);
  return {
    enable() {
      if (record.admitted) return;
      record.admitted = true;
      mount(record);
      record.stopWatch = RichClay.watch(record.win);
      const target = record.root.ownerDocument || document;
      record.subscriber = { target, type: "clay:sync-applied", handler: onFrame };
      target.addEventListener(record.subscriber.type, onFrame);
    },
    quiesce() {
      record.admitted = false;
      if (record.stopWatch) {
        record.stopWatch();
        record.stopWatch = null;
      }
    },
    resume() {
      if (record.admitted) return;
      record.admitted = true;
      record.stopWatch = RichClay.watch(record.win);
    },
    dispose() {
      record.admitted = false;
      if (record.stopWatch) {
        record.stopWatch();
        record.stopWatch = null;
      }
      const subscriber = record.subscriber;
      record.subscriber = null;
      if (subscriber) subscriber.target.removeEventListener(subscriber.type, subscriber.handler);
      for (const el of elements(record)) RichClay.stripElement(el);
      if (session === record) session = null;
    },
  };
}

/**
 * RichClay as a mode participant: the editors the commit that enters edit mounts, and the
 * ones the exit takes back.
 *
 * `prepare` takes the vendor's auto-watch out of the page first, so every instance is this
 * session's to quiesce and destroy and an entry never meets an editor it did not mount; a
 * vendor build without that lifecycle support is refused here rather than left half
 * mounted in a view. `enable` mounts the regions and starts the session's own watcher.
 * `quiesce` stops admitting regions. `dispose` destroys the instances and hands back the
 * frame subscription, leaving no editor behind for the reader's page.
 */
export function richclayParticipant(root = document) {
  return {
    id: "clay:richclay",
    order: 310,
    async prepare(context = {}) {
      if (context.initial === true) return adopt();
      if (typeof RichClay.watch !== "function" || typeof RichClay.stripElement !== "function") {
        throw fault("this vendor build has no watchable lifecycle", "unsupported-plugin");
      }
      if (isStale(context)) throw fault("prepared for a stale generation", "stale-document");
      const stopAutoWatch = RichClay.autoInit?.();
      if (typeof stopAutoWatch === "function") stopAutoWatch();
      return richclayLease(session || (session = ownership(root)));
    },
  };
}

// The boot this tab already made, adopted instead of prepared: it is the session `init`
// below enabled, or, on a boot that started none, nothing to hand back.
async function adopt() {
  const lease = await ready;
  return { enable() {}, quiesce: () => lease?.quiesce(), resume: () => lease?.resume(), dispose: () => lease?.dispose() };
}

// Auto-init when the module is imported; the loader awaits `ready` before resolving clay.ready
async function init() {
  if (!startsAtImport()) return null;
  const lease = await richclayParticipant().prepare();
  lease.enable();
  return lease;
}

const ready = init();

export { RichClay, init, ready };
export { richclayParticipant as modeParticipant };
export default RichClay;
