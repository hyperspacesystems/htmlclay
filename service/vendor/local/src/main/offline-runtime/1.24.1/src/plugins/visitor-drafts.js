import { isDraftMode, isEditMode, isOwner } from "../core/is-edit-mode.js";
import { hasSaveToken } from "../core/host-attrs.js";
import { draftSlot, draftStoreFailed, setDraftTransport, setDraftFlush, setDraftUnconfirmed } from "../core/draft-mode.js";
import { reloadAfterDiscard } from "../core/unsaved-warning.js";
import { createStore, adoptLegacy, draftPath, legacyKey, safeStorage, storageFailure } from "./visitor-drafts/store.js";
import { createUiMemory } from "./visitor-drafts/ui-memory.js";
import { rememberSource, downloadUrl } from "./visitor-drafts/source.js";
import { mountPrompt } from "./visitor-drafts/prompt.js";
import { DOWNLOAD_URL, backToDraft, editOriginal, inertVisitorDrafts } from "./visitor-drafts/inert.js";

let store = null;
// This tab's side of the draft database, once it has opened. Without one, `localStorage`
// holds the record for this page load, as it did before the database.
let session = null;
let direct = true;
let database = false;
let opening = Promise.resolve();
// The database did not open and the full draft is only there: nothing is saved over it
// for the rest of this page load.
let refused = false;
let restoredDetail = null;
let downloadLink = DOWNLOAD_URL;
let source = null;
let ready = Promise.resolve();
// Settles once the draft is on the page. Already settled for a first visit and for a
// draft `localStorage` holds in full; the loader awaits it before the next plugin loads.
let restoring = Promise.resolve();
// The synchronous half of a save: false while something this tab changed is neither
// stored nor in its journal slot.
let keep = () => true;
// A promise while the page's last changes are not yet where the download page reads them,
// null once they are.
let settle = () => null;
// Past this deadline the download uses this page's current HTML.
const DOWNLOAD_WAIT_MS = 3000;

// The chip is a convenience: a failed import leaves the draft working without it.
function mountChip() {
  return import("./visitor-drafts/owner-chip.js")
    .then((m) => m.mountOwnerChip())
    .catch((error) => {
      console.warn("visitor-drafts: the owner chip did not load", error);
      return null;
    });
}

// Most visitors never see the intro (no template, a draft already kept, or dismissed),
// so it is fetched only for a page that has one; a failed import costs only the intro.
function mountIntroLate(options) {
  if (!options.template) return Promise.resolve(null);
  return import("./visitor-drafts/intro.js")
    .then((m) => m.mountIntro(options))
    .catch((error) => {
      console.warn("visitor-drafts: the intro did not load", error);
      return null;
    });
}

const slot = draftSlot();
const rulesTag = document.querySelector('script[data-rules-name~="api"]');

if (isDraftMode && slot && rulesTag) {
  const path = draftPath();
  const moved = adoptLegacy(slot, { path });
  const local = safeStorage();
  store = createStore({ path, fallback: moved === "failed" ? legacyKey(slot) : null });
  direct = !(typeof globalThis.indexedDB?.open === "function" && globalThis.IDBKeyRange);
  database = !direct;
  // The session loads beside the database open, off the path of a draft `localStorage` holds
  // in full. Until it is there a save still makes its operation and journals it.
  opening = direct ? Promise.resolve() : import("./visitor-drafts/session.js")
    .then((m) => m.openSession({ store, storage: local, reload: () => reloadAfterDiscard({ isCurrent: () => true, reload: () => window.location.reload() }) }))
    .then((opened) => { session = opened; }, () => { direct = true; });

  setDraftTransport((html) => {
    const recapture = !restoredDetail;
    return restoring.then(() => {
      if (refused) throw storageFailure(document, new Error("the draft database did not open"));
      if (recapture) html = window.clay.getHTML();
      if (direct) return void store.write(html);
      store.capture(html);
      return opening.then(() => {
        if (session) return session.sync().then(() => {});
        store.write(html);
      });
    });
  });

  const memory = createUiMemory(store.key, local);
  source = rememberSource(memory);
  downloadLink = downloadUrl(source);

  function finish(result) {
    restoredDetail = { restored: result.restored, hadDraft: result.hadDraft, parked: Boolean(result.parked), ...(result.partial && { partial: true }), ...(result.blocked && { blocked: true }) };
    document.dispatchEvent(new CustomEvent("clay:drafts-restored", { detail: restoredDetail }));
    const appName = document.querySelector('meta[name="application-name"]')?.content?.trim() || document.title.split(/:|\s\|\s/)[0].trim() || "This app";
    const template = document.querySelector("template[visitor-intro]");
    const start = () => {
      ready = mountPrompt({ memory, appName, link: downloadLink, settle: () => settle(), canDownloadCurrent: () => !refused && !restoredDetail.blocked }).then((prompt) => mountIntroLate({ memory, template, appName, hadDraft: result.hadDraft || store.kept() }).then(() => prompt)).then((prompt) => {
        document.addEventListener("clay:drafts-store-failed", () => prompt.showError());
        document.addEventListener("clay:drafts-saved", () => prompt.clearError());
        if (draftStoreFailed()) prompt.showError();
        return (isOwner ? mountChip() : Promise.resolve(null)).then(() => prompt);
      });
    };
    document.readyState === "loading" ? document.addEventListener("DOMContentLoaded", start, { once: true }) : start();
  }

  keep = () => {
    if (refused) return false;
    if (!restoredDetail) return true;
    if (direct) return store.flush();
    const safe = store.keep();
    session?.hide();
    return safe;
  };
  setDraftFlush(keep);
  setDraftUnconfirmed(() => !direct && store.unsafe());
  settle = () => {
    if (refused) return Promise.resolve(false);
    if (!restoredDetail) return restoring.then(() => settle());
    if (direct) {
      return store.flush() ? null : Promise.resolve(false);
    }
    const safe = store.keep();
    if (!store.pending()) return safe ? null : Promise.resolve(false);
    const committed = opening.then(() => session ? session.sync().then(() => true) : store.flush()).catch(() => false);
    let timer;
    return Promise.race([committed, new Promise((resolve) => { timer = setTimeout(() => resolve(false), DOWNLOAD_WAIT_MS); })])
      .finally(() => clearTimeout(timer));
  };
  document.addEventListener("visibilitychange", () => { if (document.visibilityState === "hidden") keep(); });
  window.addEventListener("pagehide", keep);

  // Another tab's save reaches a page on the database through its session. With
  // `localStorage` as the record it arrives as that key's storage event, once the page
  // editor listens (an event before that is missed), and when the tab comes back from the
  // page cache.
  const takeOtherTab = () => {
    if (!direct || refused || !restoredDetail) return;
    // Read before adopting: adopting reads the record as this tab's own.
    const unit = store.externalUnit();
    // A page another tab wrote goes to the page editor, which merges it into its own.
    if (unit) document.dispatchEvent(new CustomEvent("clay:drafts-blockeditor-external", { detail: { live: unit.live, adopted: () => store.adoptedUnit(unit) } }));
    store.adoptExternal();
  };
  window.addEventListener("storage", (event) => { if (event.key === store.key) takeOtherTab(); });
  document.addEventListener("clay:drafts-blockeditor-listening", takeOtherTab);
  window.addEventListener("pageshow", (event) => { if (event.persisted) takeOtherTab(); });

  const held = store.held();
  const head = held?.v === 2 && held.idb === true;
  const foreignReset = !direct && held?.v === 1 && held.w === undefined && store.generation() !== "";
  if (head || foreignReset || (!direct && held?.w && held.w.reset !== store.generation())) {
    // The full draft is only in the database, or this copy is from before the last Start
    // over: the page waits for the database's record, and plugins that read the document
    // wait with the page.
    restoring = opening.then(() => session?.first).then((result) => {
      if (result) return finish(store.restore(result.record && { raw: JSON.stringify(result.record), record: result.record }));
      throw new Error("the draft database did not open");
    }).catch((error) => {
      if (!head) {
        if (!foreignReset) { try { local?.removeItem(store.key); } catch {} }
        return finish(store.restore());
      }
      refused = true;
      storageFailure(document, error);
      finish({ restored: false, hadDraft: true, blocked: true });
    });
  } else {
    finish(store.restore());
    // The database did not open: `localStorage` is the record for this page load, and what
    // this tab journaled while it waited is saved there.
    opening.then(() => {
      if (session) return;
      store.release();
      store.flush();
    });
  }
} else if (!slot || !rulesTag) {
  const missing = [];
  if (!slot) missing.push('a visitor-drafts="<slot>" attribute on <html>');
  if (!rulesTag) missing.push('a <script data-rules-name~="api"> rules tag');
  console.warn(`visitor-drafts: this page keeps no draft (missing ${missing.join(" and ")})`);
} else if (isOwner && isEditMode && !hasSaveToken()) {
  ready = mountChip();
}

export { ready, restoring };

export const visitorDrafts = store ? {
  active: true,
  slot: store.slot,
  key: store.key,
  hasDraft: () => store.kept(),
  restored: () => restoredDetail,
  saveNow() {
    if (refused || !restoredDetail) return false;
    if (direct) return store.writeLive();
    try { store.capture(window.clay.getHTML()); } catch { return false; }
    session?.sync().catch(() => {});
    return !store.unsafe();
  },
  reset() {
    if (!database) {
      store.clear();
      window.location.reload();
      return true;
    }
    // The generation ends in the database before the page loads again.
    opening.then(() => {
      if (session) return session.startOver();
      if (direct && !refused) return store.clear();
      throw new Error("the draft database did not open");
    }).then(() => window.location.reload(), (error) => storageFailure(document, error));
    return true;
  },
  onRestore(fn) {
    if (restoredDetail) {
      fn(restoredDetail);
      return true;
    }
    document.addEventListener("clay:drafts-restored", (event) => fn(event.detail), { once: true });
    return false;
  },
  editOriginal,
  backToDraft,
  downloadUrl: () => downloadLink,
  download: () => ready.then(async (prompt) => {
    const saved = await settle();
    return prompt?.download(saved === false);
  }),
  source,
} : inertVisitorDrafts();
