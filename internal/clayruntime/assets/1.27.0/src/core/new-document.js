import { engine, applyData, blankData } from "../vendor/hyper-html-api-data.vendor.js";
import { finalizeControlForSave } from "../vendor/control-serialize.vendor.js";
import { TAB_LOCAL_ROOT_ATTRS, stripRouteAttrs } from "../lib/root-attrs.js";
import { cleanSaveCopy } from "../lib/region-policy.js";
import { stripCopyContent } from "./copy-content.js";
import { downloadHtml, documentName, portableHtml, withoutPageStore } from "./download.js";

/**
 * New blank document: this document with its data emptied, as a new file.
 *
 * The page's `api` rules name its data, so a blank copy is the page with blankData
 * applied, starting values (`<script type="application/json" clay-new-document>`)
 * winning. Copy content (`clay="no-copy"`) is cut from the HTML string first, as every
 * copy does. A page whose rules do not cover everything (a Block page's article and
 * store) blanks the rest in a `clay:newdocument` listener, which gets the detached copy.
 * The live page is never touched.
 */

const STARTING = 'script[type="application/json"][clay-new-document]';
const CONTROLS = 'input:not([type="password"]):not([type="file"]), textarea, select';
const FAILED = "Couldn't make a new document. Try again.";

function startingValues(doc) {
  const tag = doc.querySelector(STARTING);
  if (!tag) return {};
  try {
    const value = JSON.parse(tag.textContent);
    if (value && typeof value === "object" && !Array.isArray(value)) return value;
  } catch {}
  console.warn("clayjs: the clay-new-document tag does not hold a JSON object, so every value starts blank");
  return {};
}

async function currentHtml() {
  if (typeof window.clay?.getHTML === "function" && (!window.clay?.isDraftMode || !/^https?:$/.test(location.protocol))) {
    return window.clay.getHTML();
  }
  const response = await fetch(location.href, {
    cache: "no-store",
    credentials: "same-origin",
    headers: { "Clay-Source": "1" },
  });
  if (!response.ok) throw new Error(`the page answered ${response.status}`);
  return response.text();
}

function cleanDocument(doc) {
  const root = doc.documentElement;
  if (!root) return;
  cleanSaveCopy(root);
  // Who last wrote the live parts, and when, belongs to the source's history.
  for (const tag of doc.querySelectorAll('script[clay~="live"]')) {
    tag.removeAttribute("live-at");
    tag.removeAttribute("live-by");
  }
  for (const name of TAB_LOCAL_ROOT_ATTRS) root.removeAttribute(name);
  stripRouteAttrs(root);
}

// The keys a blank copy empties. `site` holds what the host keeps for every page of a site
// (core/site-edits.js), `html@look` is the chosen theme its variant styles follow, a codec rule has no blank the engine knows, and the starting values
// tag's `keep` attribute names keys whose values the new document carries over as they are.
function writableRules(rules, doc) {
  const keep = new Set((doc.querySelector(STARTING)?.getAttribute("keep") || "").split(/\s+/).filter(Boolean));
  const active = {};
  for (const [key, rule] of Object.entries(rules)) {
    if (key === "site" || rule === "html@look" || keep.has(key) || engine.codecOf(rule)) continue;
    active[key] = rule;
  }
  return active;
}

function serialize(doc) {
  const doctype = doc.doctype ? `<!DOCTYPE ${doc.doctype.name}>\n` : "";
  return doctype + doc.documentElement.outerHTML;
}

export async function build() {
  // A Block page's store holds its old writing, and the editor would restore it; the article
  // seeds the new page, and the template's clay:newdocument listener blanks the article.
  // Content only the paid plans keep stays for the host to judge; a download strips it all.
  const html = withoutPageStore(stripCopyContent(await currentHtml(), []));
  const doc = new DOMParser().parseFromString(html, "text/html");
  cleanDocument(doc);
  const found = engine.findRulesIn(doc, "api");
  if (!found?.rules) throw Object.assign(new Error("This page has no api rules tag."), { code: "no_rules" });
  const rules = writableRules(found.rules, doc);
  if (Object.keys(rules).length > 0) {
    applyData(doc, blankData(rules), rules);
    const starting = startingValues(doc);
    if (Object.keys(starting).some((key) => Object.prototype.hasOwnProperty.call(rules, key))) {
      applyData(doc, blankData(rules, starting), rules);
    }
  }
  document.dispatchEvent(new CustomEvent("clay:newdocument", { detail: { document: doc } }));
  for (const control of doc.querySelectorAll(CONTROLS)) finalizeControlForSave(control);
  return serialize(doc);
}

async function hostAction() {
  if (!/^https?:$/.test(location.protocol)) return null;
  try {
    const { hostActions } = await import("./host-meta.js");
    return (await hostActions()).newDocument ?? null;
  } catch {
    return null;
  }
}

export async function create() {
  let html;
  try {
    html = await build();
  } catch (error) {
    console.error("clayjs: could not build the new document", error);
    return { ok: false, message: FAILED };
  }
  const action = await hostAction();
  if (action?.post) {
    let body = null;
    try {
      const response = await fetch(action.post, {
        method: "POST",
        credentials: "same-origin",
        headers: { "Content-Type": "text/html;charset=utf-8", "Document-URL": location.href },
        body: html,
      });
      body = await response.json().catch(() => null);
      const href = typeof body?.href === "string" ? new URL(body.href, location.href) : null;
      if (response.ok && body?.ok !== false && /^https?:$/.test(href?.protocol)) return { ok: true, href: href.href };
    } catch {}
    const text = body?.msg ?? body?.message;
    return { ok: false, message: typeof text === "string" && text ? text : FAILED };
  }
  const downloaded = await downloadHtml({ html: portableHtml(html), name: `${documentName()}-new.html` });
  return downloaded ? { ok: true, downloaded: true } : { ok: false, message: FAILED };
}
