import { fetchRunFile } from "../core/run-files.js";

/**
 * clay.programs.
 *
 * A page carries programs in inert script tags, each plain JavaScript with an
 * async `main(input, clay)`: `<script type="text/x-clay-program" name="release-notes" can="read">`.
 * Another tag of the same page is a module a program loads with `require("./name")`;
 * programs have no `import` or `export`. `can` asks for `read` (the default: files and
 * git history in a folder the person chooses), `change` (also write files there) or
 * `internet` (fetch public addresses, no files). A program never gets both the
 * internet and files, and two tags with one name are both refused.
 *
 * clay.programs.run sends the host's `programs` wire helper a program's name and its
 * input, never its text: the host reads the tag from the saved file and runs that
 * version, outside the browser, once the person allowed the page. An edit to a tag
 * runs only after it is saved. The console streams to onLog, clay.status to
 * onStatus, and done answers what main returned as data, plus the outputs the
 * program kept with clay.output.
 *
 * Loads in view, edit and draft mode. In draft mode the wire is dropped, so run
 * answers programs_unavailable and list still works.
 */

const TYPE = "text/x-clay-program";
const HELPER = "programs";
const NAME = /^[a-z0-9](?:[a-z0-9-]{0,30}[a-z0-9])?$/;
const RESERVED_NAMES = new Set(["ai-tools", "ai-edit", "document-trust", "programs"]);
const XHTML_NS = "http://www.w3.org/1999/xhtml";
const FOLDERS = new Set(["remembered", "choose"]);
const MAX_INPUT = 256 * 1024;
const MAX_PROGRAMS = 8;
const MAX_PROGRAM_BYTES = 256 * 1024;

const encoder = new TextEncoder();
const bytes = (text) => encoder.encode(text).length;

function levelOf(can) {
  let level = "read";
  let files = false;
  let internet = false;
  for (const word of can.toLowerCase().split(/\s+/).filter(Boolean)) {
    if (word === "read" || word === "git") files = true;
    else if (word === "change") {
      files = true;
      level = "change";
    } else if (word === "internet") internet = true;
    else return null;
  }
  if (internet && files) return null;
  return internet ? "internet" : level;
}

function list() {
  const tags = [];
  const count = new Map();
  let read = 0;
  for (const el of document.querySelectorAll(`script[type="${TYPE}"][name]`)) {
    if (el.namespaceURI !== XHTML_NS) continue;
    const name = el.getAttribute("name");
    if (!NAME.test(name) || RESERVED_NAMES.has(name)) continue;
    const seen = count.has(name);
    count.set(name, (count.get(name) || 0) + 1);
    if (seen) continue;
    const can = (el.getAttribute("can") || "").trim();
    let level = null;
    if (bytes(el.textContent) <= MAX_PROGRAM_BYTES && read < MAX_PROGRAMS) {
      read += 1;
      level = levelOf(can);
    }
    tags.push({ name, can, level });
  }
  return tags.map((tag) => (count.get(tag.name) > 1 ? { ...tag, level: null } : tag));
}

const EMPTY = { state: "error", program: null, data: null, log: "", logCut: false, outputs: {}, folder: null, grant: null, error: null };
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

function outputsOf(list) {
  const out = {};
  if (!Array.isArray(list)) return out;
  for (const item of list) {
    if (!item || typeof item !== "object" || typeof item.name !== "string" || !item.name) continue;
    if (Object.prototype.hasOwnProperty.call(out, item.name)) continue;
    const text = fetchRunFile(item.url, "output_unavailable", "output");
    text.catch(() => {});
    const entry = { name: item.name, bytes: Number.isFinite(item.bytes) ? item.bytes : null, text: () => text };
    Object.defineProperty(out, item.name, { value: entry, enumerable: true, writable: true, configurable: true });
  }
  return out;
}

function settle(view) {
  if (view.state === "done") {
    const result = view.result && typeof view.result === "object" ? view.result : {};
    return outcome({
      state: "done",
      program: result.program ?? null,
      data: result.data ?? null,
      log: typeof result.log === "string" ? result.log : "",
      logCut: result.logCut === true,
      outputs: outputsOf(result.outputs),
      folder: result.folder ?? null,
      grant: result.grant ?? null,
    });
  }
  if (view.state === "cancelled") {
    return outcome({ state: "cancelled", error: { code: view.errorCode || "cancelled", message: view.error || "Stopped." } });
  }
  return outcome({ state: "error", error: { code: view.errorCode || "error", message: view.error || "The program failed." } });
}

function inputText(input) {
  try {
    return JSON.stringify(input === undefined ? null : input);
  } catch {
    return undefined;
  }
}

function run(name, options) {
  if (typeof name !== "string" || !NAME.test(name) || RESERVED_NAMES.has(name)) {
    return failed("invalid_request", "A program name is 1 to 32 lowercase letters, digits and hyphens, and cannot be a built-in helper name.");
  }
  if (options !== undefined && (options === null || typeof options !== "object" || Array.isArray(options))) {
    return failed("invalid_request", "options must be an object.");
  }
  const opts = options || {};
  const { input = null, folder = "remembered" } = opts;
  if (!FOLDERS.has(folder)) return failed("invalid_request", 'folder must be "remembered" or "choose".');
  const text = inputText(input);
  if (typeof text !== "string") return failed("invalid_request", "input must be JSON: plain objects, arrays, strings, numbers, booleans and null.");
  if (bytes(text) > MAX_INPUT) return failed("invalid_request", "A program's input is larger than 256 KB.");
  const wire = window.clay?.wire;
  if (!wire || typeof wire.send !== "function") {
    return failed("programs_unavailable", "This page can't run its programs here. Open it in HTML Clay.");
  }
  const onLog = typeof opts.onLog === "function" ? opts.onLog : null;
  const onStatus = typeof opts.onStatus === "function" ? opts.onStatus : null;
  const handle = wire.send(
    { program: name, input: JSON.parse(text), folder },
    {
      helper: HELPER,
      document: "none",
      signal: opts.signal,
      onStatus: (status) => {
        if (status.phase === "log") onLog?.(status.text);
        else onStatus?.(status);
      },
    }
  );
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

export const programs = { available, list, run };

export default programs;
