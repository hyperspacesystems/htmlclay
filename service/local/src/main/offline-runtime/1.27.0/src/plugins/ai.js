import { engine, schemaFor } from "../vendor/hyper-html-api-data.vendor.js";
import { fetchRunFile } from "../core/run-files.js";

/**
 * clay.instructions and clay.ai.
 *
 * Hidden instructions are inert script tags the file carries for an agent, named so
 * a request can pick them: `<script type="text/x-clay-instructions" name="review">`.
 * ClayJS resolves names to text and sends the text; a host never parses these tags.
 *
 * clay.ai.ask sends one request to the host's `ai-tools` wire helper, which runs the
 * person's agent with read tools in a folder they chose and answers with data that
 * matches a schema, plus sources (a git range, file lines) the host expanded into run
 * files. ClayJS never writes the answer into the document: the page applies it.
 *
 * clay.ai.prompt builds the same request as text, for an agent in a terminal (Copy
 * prompt), with steps to write the answer back through clay-agent or as a file.
 *
 * Loads in view, edit and draft mode. In draft mode the wire is dropped, so ask
 * answers ai_unavailable and prompt still works: a visitor copies the prompt.
 */

const TYPE = "text/x-clay-instructions";
const HELPER = "ai-tools";
const KIB = 1024;
const LIMITS = { request: 64 * KIB, instructions: 128 * KIB, schema: 64 * KIB };
const TOOLS = new Set(["read", "change", "internet"]);
const FOLDERS = new Set(["remembered", "choose"]);
const WRITE_BACKS = new Set(["agent", "file", "auto"]);
const LIST_KEY = /^[A-Za-z0-9_$-]+$/;

const encoder = new TextEncoder();
const bytes = (text) => encoder.encode(text).length;
const coded = (code, message) => Object.assign(new Error(message), { code });

// A script's text ends at the first "</script", and "<!--" can start the escaped
// state that swallows the real end tag, so both are stored escaped and read plain.
const escapeText = (text) => String(text).replace(/<(\\*)\/(script)/gi, "<\\$1/$2").replace(/<(\\*)!--/g, "<\\$1!--");
const unescapeText = (text) => text.replace(/<\\(\\*)\/(script)/gi, "<$1/$2").replace(/<\\(\\*)!--/g, "<$1!--");

const tags = () => [...document.querySelectorAll(`script[type="${TYPE}"][name]`)].filter((el) => el.getAttribute("name"));
const tagNamed = (name) => tags().find((el) => el.getAttribute("name") === name) || null;
const canWrite = () => window.clay?.modeLifecycle?.canWrite?.() === true && !window.clay?.isDraftMode;

export const instructions = {
  get(name) {
    const tag = tagNamed(name);
    return tag ? unescapeText(tag.textContent).trim() : null;
  },

  names() {
    return [...new Set(tags().map((el) => el.getAttribute("name")))];
  },

  // An ordinary edit: the tag is saved content, so autosave and sync carry it.
  set(name, text) {
    if (typeof name !== "string" || !name || !canWrite()) return false;
    let tag = tagNamed(name);
    if (!tag) {
      tag = document.createElement("script");
      tag.setAttribute("type", TYPE);
      tag.setAttribute("name", name);
      document.head.append(tag);
    }
    const next = escapeText(text ?? "");
    if (tag.textContent !== next) tag.textContent = next;
    return true;
  },

  fromForm(form) {
    const names = (form?.getAttribute?.("instructions") || "").split(/\s+/).filter(Boolean);
    for (const el of form?.querySelectorAll?.("[instructions]") || []) {
      if (el.localName === "select") names.push(el.value);
      else if ((el.type === "checkbox" || el.type === "radio") && el.checked) names.push(el.value);
    }
    return [...new Set(names.filter(Boolean))];
  },
};

function resolveInstructions(names = []) {
  if (!Array.isArray(names) || names.some((name) => typeof name !== "string" || !name)) {
    throw coded("invalid_request", "instructions must be a list of instruction names.");
  }
  return [...new Set(names)].map((name) => {
    const text = instructions.get(name);
    if (text == null) throw coded("instruction_missing", `This page has no instructions named "${name}".`);
    return { name, text };
  });
}

function rules() {
  const found = engine.findRulesIn(document, "api");
  if (!found?.rules) throw coded("invalid_request", "This page has no api rules tag.");
  return found.rules;
}

// One answer: a list rule's row, or an object or scalar rule's value.
function schema({ result, omit = [] } = {}) {
  const all = rules();
  if (typeof result !== "string" || !Object.prototype.hasOwnProperty.call(all, result)) {
    throw coded("invalid_request", `The api rules have no "${result}" key.`);
  }
  const rule = all[result];
  return schemaFor(Array.isArray(rule) ? rule[1] : rule, Array.isArray(omit) ? omit : []);
}

function answerSchema(options) {
  if (options.schema && typeof options.schema === "object" && !Array.isArray(options.schema)) return options.schema;
  if (options.result !== undefined) return schema(options);
  return { type: "object" };
}

function requestPayload(options) {
  const { request, tools = "read", folder = "remembered", engine: agent = "", sources = false } = options;
  if (typeof request !== "string" || !request.trim()) throw coded("invalid_request", "Write a request first.");
  if (bytes(request) > LIMITS.request) throw coded("invalid_request", "The request is longer than 64 KB.");
  if (!TOOLS.has(tools)) throw coded("invalid_request", 'tools must be "read", "change" or "internet".');
  if (!FOLDERS.has(folder)) throw coded("invalid_request", 'folder must be "remembered" or "choose".');
  if (typeof agent !== "string") throw coded("invalid_request", "engine must be a string.");
  const resolved = resolveInstructions(options.instructions);
  if (resolved.reduce((sum, { text }) => sum + bytes(text), 0) > LIMITS.instructions) {
    throw coded("invalid_request", "The instructions are longer than 128 KB in all.");
  }
  const shape = answerSchema(options);
  if (bytes(JSON.stringify(shape)) > LIMITS.schema) throw coded("invalid_request", "The answer shape is larger than 64 KB.");
  return { request, instructions: resolved, tools, folder, schema: shape, sources: sources === true, engine: agent };
}

const EMPTY = { state: "error", data: null, sources: {}, folder: null, grant: null, engine: null, model: null, error: null };
const outcome = (fields) => ({ ...EMPTY, ...fields });

function failed(code, message) {
  return {
    id: null,
    get state() {
      return "error";
    },
    done: Promise.resolve(outcome({ state: "error", error: { code, message } })),
    cancel: () => false,
  };
}

function sourcesOf(list) {
  const out = {};
  if (!Array.isArray(list)) return out;
  for (const src of list) {
    if (!src || typeof src !== "object" || typeof src.id !== "string" || !src.id) continue;
    if (Object.prototype.hasOwnProperty.call(out, src.id)) continue;
    const { url, ...meta } = src;
    const text = fetchRunFile(url, "source_unavailable", "source");
    text.catch(() => {});
    const entry = { ...meta, skipped: Array.isArray(meta.skipped) ? meta.skipped : [], text: () => text };
    Object.defineProperty(out, src.id, { value: entry, enumerable: true, writable: true, configurable: true });
  }
  return out;
}

function settle(view) {
  if (view.state === "done") {
    const result = view.result && typeof view.result === "object" ? view.result : {};
    return outcome({
      state: "done",
      data: result.data ?? null,
      sources: sourcesOf(result.sources),
      folder: result.folder ?? null,
      grant: result.grant ?? null,
      engine: result.engine ?? null,
      model: result.model ?? null,
    });
  }
  if (view.state === "cancelled") {
    return outcome({ state: "cancelled", error: { code: view.errorCode || "cancelled", message: view.error || "Stopped." } });
  }
  return outcome({ state: "error", error: { code: view.errorCode || "error", message: view.error || "The agent request failed." } });
}

function ask(options = {}) {
  let payload;
  try {
    payload = requestPayload(options);
  } catch (error) {
    return failed(error.code || "invalid_request", error.message);
  }
  const wire = window.clay?.wire;
  if (!wire || typeof wire.send !== "function") {
    return failed("ai_unavailable", "This page can't reach an AI agent here. Use Copy prompt instead.");
  }
  const onStatus = typeof options.onStatus === "function" ? options.onStatus : undefined;
  const handle = wire.send(payload, { helper: HELPER, document: "none", onStatus, signal: options.signal });
  return {
    id: handle.id,
    get state() {
      return handle.state;
    },
    done: handle.done.then(settle),
    cancel: () => handle.cancel(),
  };
}

async function available() {
  const wire = window.clay?.wire;
  if (!wire || typeof wire.helpers !== "function") return null;
  try {
    const found = (await wire.helpers()).find((helper) => helper.name === HELPER);
    return found ? found.state : null;
  } catch {
    return null;
  }
}

const shellQuote = (text) => `'${String(text).replace(/'/g, `'\\''`)}'`;

function writeBackMode(writeBack) {
  if (writeBack !== "auto") return writeBack;
  const clay = window.clay || {};
  return clay.canEdit === true && !clay.isDraftMode && /^https?:$/.test(window.location.protocol) ? "agent" : "file";
}

function agentSteps(into) {
  const doc = shellQuote(window.location.origin + window.location.pathname);
  const filter = shellQuote(`{"${into}": ((.data["${into}"] // []) + $item)}`);
  return [
    "## Writing your answer back",
    "",
    "1. Save your JSON object to /tmp/clay-answer.json.",
    "2. Read the document's current data:",
    "",
    "```sh",
    `npx -y @panphora/clay-agent data ${doc} --json > /tmp/clay-read.json`,
    "```",
    "",
    `3. Add your answer to the end of the "${into}" list and write it back:`,
    "",
    "```sh",
    `jq --slurpfile item /tmp/clay-answer.json ${filter} /tmp/clay-read.json | npx -y @panphora/clay-agent data ${doc} --write - --if-match "$(jq -r .etag /tmp/clay-read.json)"`,
    "```",
    "",
    "4. If the write answers with a conflict, start again from step 2.",
  ].join("\n");
}

function fileSteps(fileName) {
  return [
    "## Writing your answer back",
    "",
    `1. Save your JSON object to a file named ${fileName}.`,
    "2. Tell the person to drop that file on the page, or paste its contents into it.",
  ].join("\n");
}

function prompt(options = {}) {
  const { request = "", writeBack = "auto", fileName = "answer.json", notes = "" } = options;
  if (!WRITE_BACKS.has(writeBack)) throw coded("invalid_request", 'writeBack must be "agent", "file" or "auto".');
  const resolved = resolveInstructions(options.instructions);
  const shape = answerSchema(options);
  const mode = writeBackMode(writeBack);
  if (mode === "agent" && !/^https?:$/.test(window.location.protocol)) {
    throw coded("invalid_request", "clay-agent can only write back to a document with a web address. Use writeBack \"file\".");
  }
  const parts = String(request).trim() ? [String(request).trim()] : [];
  if (resolved.length) {
    parts.push("## Instructions\n\n" + resolved.map(({ name, text }) => `### ${name}\n\n${text}`).join("\n\n"));
  }
  parts.push("## Answer shape\n\nAnswer with one JSON object that matches this JSON Schema:\n\n```json\n" + JSON.stringify(shape, null, 2) + "\n```");
  if (mode === "agent") {
    const into = options.into ?? options.result;
    if (typeof into !== "string" || !LIST_KEY.test(into) || !Array.isArray(rules()[into])) {
      throw coded("invalid_request", "into must name a list in the page's api rules.");
    }
    parts.push(agentSteps(into));
  } else {
    parts.push(fileSteps(String(fileName)));
  }
  if (notes) parts.push("## Notes from this page\n\n" + String(notes).trim());
  return parts.join("\n\n") + "\n";
}

export const ai = { available, ask, prompt, schema };

export default ai;
