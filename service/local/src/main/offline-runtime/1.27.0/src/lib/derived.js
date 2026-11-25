import { ownsAttr, paintsOf, stripPaints, eachPainted } from "../vendor/sap-ownership.vendor.js";
import { bootPhase } from "../core/boot-phase.js";
import { ownedRootsIn } from "./owned-regions.js";

// Values a derive engine (sapjs) paints from the page's records: attr:, class:,
// css:, text and show output inside a [sap] root. They are saved like any other
// content, but they are never an edit: comparisons, the dirty gate and undo leave
// them out. A merge leaves out the ones this tab paints again (repaintsText and the
// rest below), and the engine repaints them after every merge, before anything
// records a baseline. Which parts of an element are painted is read
// from its own attributes, so the same answer holds for the live page, a
// snapshot clone and a parsed frame. No expression is evaluated here.

// `host` is the element a just-removed subtree belongs under (the undo engine passes
// it), so a detached card is read as painted the way it was while attached.
export const isDerivedAttr = (el, name, host = null) => ownsAttr(el, name, host);

export const ownsDerivedText = (el) => !!el && el.nodeType === 1 && !!paintsOf(el)?.text;

/** Remove every painted value from a detached tree, in place. Never the live page. */
export const stripDerived = (root) => stripPaints(root);

// A merge asks a different question from the three above: "if I leave this value out,
// will this tab paint it again?" Only a registered engine paints, and a plugin's rules
// count only while that plugin is installed here. With no plugin rules on the page,
// these answer as the three above do behind derivedActive().
const installed = (plugin) => !!window.Sap?.plugins?.has?.(plugin);

export const repaintsAttr = (el, name) => derivedActive() && ownsAttr(el, name, null, installed);

export const repaintsText = (el) => derivedActive() && !!el && el.nodeType === 1 && !!paintsOf(el, installed)?.text;

/** Remove every value this tab paints again from a detached merge input, in place. */
export function stripRepainted(root) {
  if (derivedActive()) stripPaints(root, installed);
}

/** Every painted value under root, in document order, as one string. */
export function paintSignature(root) {
  const parts = [];
  eachPainted(root, (el, p) => {
    parts.push(el.localName);
    for (const n of p.attrs) if (n !== "sap-error") parts.push(n + "=" + (el.getAttribute(n) ?? "\u0000"));
    for (const t of p.classes) parts.push("." + t + "=" + el.classList.contains(t));
    for (const v of p.vars) parts.push(v + "=" + el.style.getPropertyValue(v));
    if (p.text) parts.push("#" + el.textContent);
  });
  return parts.join("\u0001");
}

/** paintSignature of a document's bytes, such as the file on disk. */
export function paintSignatureOfHtml(html) {
  const doc = new DOMParser().parseFromString(html, "text/html");
  for (const { element } of ownedRootsIn(doc)) element.replaceChildren();
  return paintSignature(doc);
}

function withoutTokens(value, owned, split, join, nameOf) {
  return split(value || "").map((t) => t.trim()).filter((t) => t && !owned.includes(nameOf(t))).sort().join(join);
}

// A class or style change that only touched painted tokens.
function onlyPaintedTokens(el, name, before, after) {
  const p = paintsOf(el);
  if (!p) return false;
  if (name === "class") {
    if (!p.classes.length) return false;
    const rest = (v) => withoutTokens(v, p.classes, (s) => s.split(/\s+/), " ", (t) => t);
    return rest(before) === rest(after);
  }
  if (!p.vars.length) return false;
  const rest = (v) => withoutTokens(v, p.vars, (s) => s.split(";"), ";", (t) => t.split(":")[0].trim());
  return rest(before) === rest(after);
}

/** True when a mutation-hub change only rewrote painted output. */
export function isDerivedChange(change) {
  if (change.__derived !== undefined) return change.__derived;
  const el = change.element;
  let derived = false;
  if (el && el.nodeType === 1) {
    if (change.type === "attribute") {
      const name = change.attribute;
      derived = isDerivedAttr(el, name) ||
        ((name === "class" || name === "style") && onlyPaintedTokens(el, name, change.oldValue, change.newValue));
    } else if (change.type === "characterData") {
      derived = ownsDerivedText(el);
    }
  }
  change.__derived = derived;
  return derived;
}

// Each engine's repaint, and the flush it gave with it or null.
const engines = new Map();
let mountedOnce = false;

/** True while a derive engine is registered, or the loader holds boot for one. */
export function derivedActive() {
  return engines.size > 0 || bootPhase.holdSettle;
}

const mountWaiters = [];

/** Run every registered engine's repaint, synchronously. Live sync calls it right after a merge applies. */
export function repaintDerived() {
  for (const repaint of [...engines.keys()]) {
    try {
      repaint();
    } catch (err) {
      console.error("clayjs: a derived repaint failed:", err);
    }
  }
}

/**
 * Ask every engine that gave a flush to finish the work it has queued, synchronously. A
 * capture at a save boundary calls it before it clones the page, so an edit and a save
 * in one task save what the engine paints from that edit. What a flush returns is not read.
 */
export function flushDerived() {
  const flushes = [...engines.values()];
  if (!flushes.some(Boolean)) return;
  // An engine learns of edits from the mutation hub, which hands an edit made through
  // undo to its subscribers only on a later microtask. Deliver it first.
  try {
    window.clay?.Mutation?.deliverDeferred?.();
  } catch (err) {
    console.error("clayjs: a derived flush failed:", err);
  }
  for (const flush of flushes) {
    if (!flush) continue;
    try {
      flush();
    } catch (err) {
      console.error("clayjs: a derived flush failed:", err);
    }
  }
}

/** Calls fn once an engine has finished its first pass, or after timeoutMs without one. */
export function afterDerivedMount(fn, timeoutMs) {
  if (mountedOnce || typeof document === "undefined" || !document.querySelector("[sap]")) return fn();
  let done = false;
  const once = () => {
    if (done) return;
    done = true;
    fn();
  };
  mountWaiters.push(once);
  setTimeout(once, timeoutMs);
}

export const derived = {
  register(repaint, { flush = null } = {}) {
    engines.set(repaint, flush);
    return () => engines.delete(repaint);
  },
  mounted() {
    mountedOnce = true;
    for (const fn of mountWaiters.splice(0)) fn();
  },
};

// The stale mark: a host's data write leaves <meta name="clay-stale" clay="no-save">
// in the head of a document whose computed values it could not recalculate
// (calculator documents plan, M1b; hyper-html-api src/stale-mark.js). Every save
// strips no-save, so the first save after the page repaints removes it.
export const STALE_MARK_SELECTOR = 'meta[name="clay-stale"]';

export function carriesStaleMark(html) {
  return typeof html === "string" && html.includes('<meta name="clay-stale"');
}
