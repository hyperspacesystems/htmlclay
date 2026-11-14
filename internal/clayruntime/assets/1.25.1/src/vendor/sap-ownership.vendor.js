// GENERATED — do not edit. Vendored from sapjs/src/ownership.js
// via sapjs `npm run propagate`, which writes every client's copy from one
// table. Edit the sapjs source and re-run.

// What sapjs paints on an element, read from that element's own attributes. No
// expression runs and no engine state is read, so a host can ask it of any tree:
// the live page, a snapshot clone, or a parsed frame. Self-contained on purpose:
// ClayJS vendors this file verbatim.

export const BEACON = "sap-error";

// What a plugin paints inside a boundary of its own, as plain data, so a host knows it
// before that plugin's code has loaded, and in a tab where it never loads:
//   { plugin, within, select, text?, attrs? }
// `within` selects the boundary, and `select` the elements under it or the boundary
// itself. `text: true` says the element's text is painted, and `attrs` names painted
// attributes. Under a boundary only rules and the error marker say what is painted,
// because the engine stays out of it: its own directives, sap-ignore and templates mean
// nothing there. First party rules live in this table, because this is the one file
// ClayJS vendors. None ships yet. A plugin from anyone else gives the same rules in its
// definition's `ownership` field, and they count once its file has run.
export const PLUGIN_RULES = [];

const WHEN = /^(show-when|hide-when|option|option-not):/;
// Identity and the attributes that declare the record itself are never paint
// output. Form state is, unless a bind or persist owns it.
const NEVER = new Set([
  "id", "data-id", "class", "style",
  "sap", "scope", "items", "item", "bind", "state", "template", "cms-template", "sap-ignore",
]);
const FORM_STATE = new Set(["value", "checked", "selected"]);

function recordOwnsFormState(el) {
  return el.hasAttribute("bind") || el.hasAttribute("persist") || !!el.closest("select[bind]");
}

function inert(el) {
  return el.localName === "template" || el.hasAttribute("template") || el.hasAttribute("cms-template") || el.hasAttribute("sap-ignore");
}

// An element the engine enters as a scope of its own never starts a plugin boundary.
const SCOPES = ["sap", "item", "scope", "items", "detail", "each"];
const NONE = [];
let known = { rules: NONE, within: "", seen: NONE, counted: 0 };

// PLUGIN_RULES, then the rules of every other definition this page has seen, with one
// selector, `within`, for every boundary a rule names. Built again only when a
// definition or a rule has arrived.
function pluginRules() {
  const seen = typeof window !== "undefined" && Array.isArray(window.sapPlugins) ? window.sapPlugins : NONE;
  const counted = seen.length + PLUGIN_RULES.length;
  if (seen === known.seen && counted === known.counted) return known;
  const rules = PLUGIN_RULES.slice();
  for (const def of seen) {
    if (!def || !Array.isArray(def.ownership) || PLUGIN_RULES.some((rule) => rule.plugin === def.name)) continue;
    for (const rule of def.ownership) if (usable(rule)) rules.push({ ...rule, plugin: def.name });
  }
  known = { rules, within: [...new Set(rules.map((rule) => rule.within))].join(","), seen, counted };
  return known;
}

// A rule from a file this project did not write runs inside a host's dirty check, so
// one that would throw there is left out.
function usable(rule) {
  if (!rule || typeof rule.within !== "string" || typeof rule.select !== "string" || (rule.attrs && !Array.isArray(rule.attrs))) return false;
  try {
    const nowhere = document.createDocumentFragment();
    nowhere.querySelector(rule.within);
    nowhere.querySelector(rule.select);
    return true;
  } catch {
    return false;
  }
}

function startsBoundary(el) {
  return !SCOPES.some((name) => el.hasAttribute(name));
}

// The nearest boundary at or above el that `selector` matches and that sits in an app,
// or null. `host` continues the walk for a subtree that left the page, as in appOf.
function boundaryAt(el, selector, host = null) {
  for (let cur = el; cur; cur = cur.parentElement) {
    const found = cur.closest(selector);
    if (!found) break;
    if (startsBoundary(found) && appOf(found, host)) return found;
    cur = found;
  }
  return host && !el.isConnected ? boundaryAt(host, selector) : null;
}

// The boundary el sits under, not counting el itself.
function boundaryAbove(el, selector, host = null) {
  const from = el.parentElement || (el.isConnected ? null : host);
  return from ? boundaryAt(from, selector, host) : null;
}

function ruleApplies(rule, el, live, host = null) {
  return (!live || live(rule.plugin)) && el.matches(rule.select) && !!boundaryAt(el, rule.within, host);
}

// The [sap] root an element is painted under, or null when it sits outside every
// app or inside a subtree the engine never reads. A subtree just removed from the
// page has no ancestors left to ask; `host`, the element it was removed from,
// continues the walk, so the answer matches the one it had while attached.
export function appOf(el, host = null) {
  let top = null;
  for (let cur = el; cur && cur.nodeType === 1; cur = cur.parentElement) {
    if (inert(cur)) return null;
    if (cur.hasAttribute("sap")) return cur;
    top = cur;
  }
  return host && top && !top.isConnected ? appOf(host) : null;
}

function declares(el, name) {
  const decl = el.getAttribute("state");
  if (!decl) return false;
  for (const token of decl.trim().split(/\s+/)) {
    if (token.split("=")[0].split(":")[0] === name) return true;
  }
  return false;
}

function paintsHidden(el) {
  if (el.hasAttribute("show")) return true;
  for (const a of el.attributes) if (WHEN.test(a.name)) return true;
  return false;
}

// True when a sapjs paint writes attribute `name` on `el`. `live`, when given, is asked
// with a plugin's name and says whether that plugin's rules count: a host passes it
// where the question is "will something repaint this", which is only true of a plugin
// that is installed.
export function ownsAttr(el, name, host = null, live = null) {
  if (!el || el.nodeType !== 1 || !name) return false;
  const { rules, within } = pluginRules();
  if (rules.length && boundaryAt(el, within, host)) {
    for (const rule of rules) {
      if (rule.attrs && rule.attrs.includes(name) && ruleApplies(rule, el, live, host)) return true;
    }
    if (boundaryAbove(el, within, host)) return name === BEACON;
  }
  if (name === BEACON) return !!appOf(el, host);
  if (NEVER.has(name)) return false;
  if (FORM_STATE.has(name) && recordOwnsFormState(el)) return false;
  if (name === "hidden") return paintsHidden(el) && !!appOf(el, host);
  return el.hasAttribute("attr:" + name) && !declares(el, name) && !!appOf(el, host);
}

const blank = () => ({ attrs: [], classes: [], vars: [], text: false });

// Everything a paint writes on `el`: whole attributes, class tokens, --custom
// properties in style, and whether text owns its children. Null when nothing.
// `live` is the same optional question as in ownsAttr.
export function paintsOf(el, live = null) {
  if (!el || el.nodeType !== 1) return null;
  const { rules, within } = pluginRules();
  if (!rules.length || !boundaryAt(el, within)) return appOf(el) ? enginePaints(el, null) : null;
  let out = null;
  for (const rule of rules) if (ruleApplies(rule, el, live)) out = addRule(out, rule);
  // With no boundary above it, el is the boundary, and the engine paints on that.
  if (!boundaryAbove(el, within)) return enginePaints(el, out);
  if (el.hasAttribute(BEACON)) (out || (out = blank())).attrs.push(BEACON);
  return out;
}

// What the engine's own directives paint on el, added to `out`.
function enginePaints(el, out) {
  const add = (key, v) => ((out || (out = blank()))[key].push(v));
  for (const a of el.attributes) {
    const n = a.name;
    if (n === "text" || n.startsWith("text:")) {
      if (!el.hasAttribute("bind")) (out || (out = blank())).text = true;
    }
    else if (n.startsWith("attr:")) {
      const target = n.slice(5);
      if (target && !NEVER.has(target) && !(FORM_STATE.has(target) && recordOwnsFormState(el)) && !declares(el, target)) add("attrs", target);
    } else if (n.startsWith("class:")) add("classes", n.slice(6));
    else if (n.startsWith("css:")) add("vars", "--" + n.slice(4));
  }
  if (paintsHidden(el)) add("attrs", "hidden");
  if (el.hasAttribute(BEACON)) add("attrs", BEACON);
  return out;
}

function addRule(out, rule) {
  out = out || blank();
  if (rule.text) out.text = true;
  for (const name of rule.attrs || NONE) if (!out.attrs.includes(name)) out.attrs.push(name);
  return out;
}

// Calls visit(el, paints) for every painted element under root, in document order.
// `live` is the same optional question as in ownsAttr.
export function eachPainted(root, visit, live = null) {
  if (!root || root.nodeType !== 1 && root.nodeType !== 9) return;
  const apps = root.nodeType === 1 && root.hasAttribute("sap")
    ? [root]
    : [...root.querySelectorAll("[sap]")].filter((app) => !app.parentElement || !app.parentElement.closest("[sap]"));
  const known = pluginRules();
  for (const app of apps) {
    // A root handed in from under a boundary is asked element by element.
    if (known.rules.length && !boundaryAbove(app, known.within)) {
      walkPainted(app, known, live, visit);
      continue;
    }
    for (const el of [app, ...app.querySelectorAll("*")]) {
      const p = paintsOf(el, live);
      if (p) visit(el, p);
    }
  }
}

// The same visits as asking paintsOf of el and of every element under it, for an el
// that is under no boundary. It does not ask paintsOf of an element under a boundary:
// the boundary is asked for the elements its rules select.
function walkPainted(el, known, live, visit) {
  if (inert(el)) {
    // A [sap] root in a subtree the engine never reads is an app again.
    let last = null;
    for (const app of el.querySelectorAll("[sap]")) {
      if (last && last.contains(app)) continue;
      walkPainted(app, known, live, visit);
      last = app;
    }
    return;
  }
  const boundary = el.matches(known.within) && startsBoundary(el);
  const p = boundary ? paintsOf(el, live) : enginePaints(el, null);
  if (p) visit(el, p);
  if (boundary) visitBoundary(el, known.rules, live, visit);
  else for (let c = el.firstElementChild; c; c = c.nextElementSibling) walkPainted(c, known, live, visit);
}

function visitBoundary(top, rules, live, visit) {
  const found = new Map();
  const note = (el, rule) => found.set(el, addRule(found.get(el), rule));
  const selects = ["[" + BEACON + "]"];
  let all = NONE;
  for (const rule of rules) {
    if (live && !live(rule.plugin)) continue;
    selects.push(rule.select);
    if (top.matches(rule.within)) {
      const list = top.querySelectorAll(rule.select);
      for (const el of list) note(el, rule);
      if (list.length > all.length) all = list;
      continue;
    }
    // A boundary of another rule, nested in this one.
    for (const inner of top.querySelectorAll(rule.within)) {
      if (!startsBoundary(inner) || !appOf(inner)) continue;
      if (inner.matches(rule.select)) note(inner, rule);
      for (const el of inner.querySelectorAll(rule.select)) note(el, rule);
    }
  }
  // An error marker under a boundary is painted too, whatever the rules select.
  const marked = top.querySelectorAll("[" + BEACON + "]");
  for (const el of marked) {
    const p = found.get(el) || blank();
    p.attrs.push(BEACON);
    found.set(el, p);
  }
  // What one query found is in document order already. When that is not everything
  // found, one more query puts all of it in order.
  if (all.length < found.size) all = top.querySelectorAll(selects.join(","));
  for (const el of all) {
    const p = found.get(el);
    if (p) visit(el, p);
  }
}

// Removes every painted value from a detached tree, in place: owned attributes,
// owned class tokens and --properties, and the children of text paints.
// `live` is the same optional question as in ownsAttr.
export function stripPaints(root, live = null) {
  eachPainted(root, (el, p) => {
    for (const name of p.attrs) if (el.hasAttribute(name)) el.removeAttribute(name);
    for (const token of p.classes) if (el.classList.contains(token)) el.classList.remove(token);
    if (p.classes.length && !el.getAttribute("class") && el.hasAttribute("class")) el.removeAttribute("class");
    for (const v of p.vars) if (el.style.getPropertyValue(v) !== "") el.style.removeProperty(v);
    if (p.vars.length && el.hasAttribute("style") && !el.getAttribute("style").trim()) el.removeAttribute("style");
    if (p.text && el.firstChild) el.replaceChildren();
  }, live);
}
