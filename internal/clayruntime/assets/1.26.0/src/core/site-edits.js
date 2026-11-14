/**
 * Shared site values edited on this page.
 *
 * On a multi-page host a page's `site` rule names values every page of the site shows:
 * a nav, a footer. The host overwrites them with the site store's on every save, which
 * keeps a stale tab from putting old values back and would also undo an edit made right
 * here. So each save says which top-level `site` keys the person changed in this tab
 * (`Site-Data: keys=nav,brand`), and the host writes exactly those to the store before
 * it overwrites the rest.
 *
 * Both sides of the comparison are read from captured bytes, never from the live DOM:
 * the baseline from the save pipeline's own record of what the file held at boot (which
 * leaves out anything typed while the editor was still loading), moved on by this tab's
 * accepted saves and by any other change to that record (a clean live-sync frame), and
 * each save from the string being sent. Whatever the save pipeline normalizes is then normalized on both sides,
 * so a value nobody touched cannot read as edited. That matters most in a stale tab,
 * where a wrongly declared key would write old content over newer.
 */
import { addSaveHeaderSource } from "./save-core.js";
import { onSaveAccepted } from "./save-accepted.js";
import { getLastSavedContents, savePageThrottled } from "./save.js";
import Mutation from "../lib/mutation.js";

export const SITE_DATA_HEADER = "Site-Data";

const RULES_TAG = 'script[data-rules-name~="api"]';
// A top-level key: it opens the object or follows a comma, so `.site:first-child` in a
// selector string is not a declaration.
const DECLARES_SITE = /(?:^|[{,])\s*["']?site["']?\s*:/;
const EMPTY = { html: null, now: null, keys: [] };

let engine = null;
let baseline = null;
let last = EMPTY;
// The save pipeline's record the baseline was last taken from. Whenever it changes (this
// tab's accepted save, a clean live-sync frame), every key the host did not refuse takes
// the record's value: for this tab's own save that is the value it sent, so an edit typed
// while the save was in flight still differs, and for a frame it is what the file now holds.
let knownSaved = null;
const refusedKeys = new Set();
// Values the host refreshed, with the value this tab had sent. The accepted hook runs before
// the save pipeline installs that save's record, so the record then still holds the sent
// value: the refreshed one stands. A record holding anything else is newer, and wins.
const refreshed = new Map();

// False while there is no record yet: no baseline, so no header.
function followSavedRecord() {
  const record = getLastSavedContents();
  if (typeof record !== "string" || !record) return false;
  if (record === knownSaved) return true;
  const values = valuesOf(parse(record));
  if (!baseline) baseline = values;
  else {
    for (const [key, value] of Object.entries(values)) {
      if (refusedKeys.has(key)) continue;
      const host = refreshed.get(key);
      baseline[key] = host && host.sent === value ? host.value : value;
    }
  }
  refreshed.clear();
  knownSaved = record;
  last = EMPTY;
  return true;
}

function siteRule(root) {
  const rule = engine.engine.findRulesIn(root, "api")?.rules?.site;
  return rule !== null && typeof rule === "object" && !Array.isArray(rule) ? rule : null;
}

function valuesOf(root) {
  const rule = siteRule(root);
  if (!rule) return {};
  const site = engine.extractData(root, { site: rule }).site ?? {};
  return Object.fromEntries(Object.entries(site).map(([key, value]) => [key, JSON.stringify(value)]));
}

const parse = (html) => new DOMParser().parseFromString(html, "text/html").documentElement;

function read(html) {
  if (last.html !== html) {
    const now = valuesOf(parse(html));
    last = { html, now, keys: Object.keys(now).filter((key) => now[key] !== baseline[key]) };
  }
  return last;
}

// No header until the baseline exists: without one this tab cannot tell an edit from a
// stale value, and a host that sees no header treats every shared value as stale.
function siteHeader(html) {
  if (!engine || typeof html !== "string" || !followSavedRecord()) return null;
  return { [SITE_DATA_HEADER]: `keys=${read(html).keys.map(encodeURIComponent).join(",")}` };
}

// Values the host stored that this tab was behind on. Taken only for a key the person has
// not touched since the save was captured, and never for a key the host refused: that
// one still shows the edit that was not stored, and the page says so.
function takeCurrent(current, sent, refused) {
  const root = document.documentElement;
  const rule = siteRule(root);
  const live = valuesOf(root);
  const applied = [];
  for (const [key, value] of Object.entries(current)) {
    if (refused.has(key) || !rule || !Object.hasOwn(rule, key) || live[key] !== sent[key]) continue;
    // Outside undo history and the edit observers, the way a sync frame lands: an undo
    // must not bring back the value this tab was behind on, and then declare it.
    const fields = new Map([...root.querySelectorAll("[data-richclay], [richclay], [editable], [clay-editable]")].map((field) => [field, field.innerHTML]));
    Mutation.pause();
    try {
      engine.applyData(root, { site: { [key]: value } }, { site: { [key]: rule[key] } });
      baseline[key] = valuesOf(root)[key];
      refreshed.set(key, { sent: sent[key], value: baseline[key] });
      applied.push(key);
    } catch (error) {
      console.error(`clayjs: the shared value "${key}" could not be refreshed on this page`, error);
    } finally {
      Mutation.resume();
    }
    // An editor that keeps its own undo history (RichClay) is told its field was rewritten,
    // so its undo starts from the new value rather than stepping back to the old one.
    for (const [field, before] of fields) {
      if (field.isConnected && field.innerHTML !== before) field.dispatchEvent(new CustomEvent("clay:external-write"));
    }
  }
  // The writes above ran with the edit observers paused, so the save pipeline still
  // records the older value: typing that value back would then look like no change and
  // never be saved. Ask for the same automatic save live sync asks for after it applies a
  // frame, which stores what the page now shows and moves the record to it.
  if (applied.length) savePageThrottled();
  return applied;
}

function accept(html, answer) {
  if (!engine || !followSavedRecord()) return;
  refreshed.clear();
  const sent = read(html).now;
  // A `stale` key was edited on a value older than the store's: the host keeps the newer one
  // and sends it as `current`, which this tab takes instead of declaring its edit again.
  const refused = new Set((answer?.refused ?? []).filter((entry) => entry.code !== "stale").map((entry) => entry.key));
  for (const key of Object.keys(sent)) {
    if (refused.has(key)) refusedKeys.add(key);
    else {
      baseline[key] = sent[key];
      refusedKeys.delete(key);
    }
  }
  last = EMPTY;
  if (!answer) return;
  const applied = answer.current ? takeCurrent(answer.current, sent, refused) : [];
  document.dispatchEvent(new CustomEvent("clay:site-data", {
    detail: { written: answer.written ?? [], refused: answer.refused ?? [], applied },
  }));
}

// Resolves once the engine has loaded, or at once on a page that declares no `site` key. The
// baseline itself is read from the save pipeline's record when it is first needed.
export const ready = DECLARES_SITE.test(document.querySelector(RULES_TAG)?.textContent ?? "")
  ? (() => {
      const loading = import("../vendor/hyper-html-api-data.vendor.js");
      addSaveHeaderSource(siteHeader);
      onSaveAccepted((html, accepted) => accept(html, accepted.siteData ?? null));
      return loading.then((loaded) => {
        engine = loaded;
      }).catch((error) => console.error("clayjs: shared site edits are not tracked on this page", error));
    })()
  : Promise.resolve();
