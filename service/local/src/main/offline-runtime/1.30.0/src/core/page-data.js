import { hostSupports } from "./host-meta.js";
import { saveToken } from "./host-attrs.js";

const DATA_READ = "data-read";
const DATA_WRITE = "data-write";

function refusal(code, message, details) {
  const error = new Error(message);
  error.code = code;
  error.details = details;
  return error;
}

const META_FIELDS = ["changed", "recalculated", "stale", "why", "code"];

// A host's answer carries plain data, plus a __meta__ key when computed values are
// old or the request asked with ?meta. The key is the host's, never the page's data.
// Data that is a list or a single value has no room for the key, so the Clay-Stale
// header is its only signal.
function splitMeta(body, response) {
  const headerStale = response?.headers?.get?.("Clay-Stale") === "true";
  const withHeader = (fields) => (headerStale && fields.stale === undefined ? { ...fields, stale: true } : fields);
  if (body === null || typeof body !== "object" || Array.isArray(body) || !Object.hasOwn(body, "__meta__")) return { data: body, meta: withHeader({}) };
  const { __meta__: meta, ...data } = body;
  if (meta === null || typeof meta !== "object" || Array.isArray(meta)) return { data: body, meta: withHeader({}) };
  const fields = {};
  for (const key of META_FIELDS) if (Object.hasOwn(meta, key)) fields[key] = meta[key];
  return { data, meta: withHeader(fields) };
}

async function responseBody(response) {
  if (typeof response.json === "function") return response.json();
  const text = await response.text();
  return text ? JSON.parse(text) : null;
}

async function responseError(response) {
  let body = {};
  try {
    body = (await responseBody(response)) || {};
  } catch {}

  const message = body.message || body.error || response.statusText || `Request failed (${response.status})`;
  const error = new Error(message);
  error.status = response.status;
  error.error = body.error;
  error.details = body.details;
  return error;
}

function responseEtag(response) {
  const value = response.headers?.get?.("ETag");
  return typeof value === "string" && value ? value : null;
}

function dataUrl(rules, location) {
  const base = `${location.origin}${location.pathname}`;
  if (rules !== undefined) return `${base}?data=${encodeURIComponent(JSON.stringify(rules))}`;
  return `${location.origin}/_/api${location.pathname}`;
}

async function draftRefusal() {
  const { isDraftMode } = await import("./is-edit-mode.js");
  if (!isDraftMode) return null;
  return refusal("DataUnavailableInDraft", "Page data is kept in this browser for this visitor.");
}

export function createPageDataApi({
  supports = hostSupports,
  token = saveToken,
  fetchImpl = (...args) => fetch(...args),
  getLocation = () => window.location,
  eventTarget = document,
  loadWriteModules,
  syncTimeoutMs,
  httpTimeoutMs,
} = {}) {
  let liveSync = null;
  let writeEngine = null;

  function setLiveSync(sync) {
    liveSync = sync;
  }

  async function readData(rules) {
    const draft = await draftRefusal();
    if (draft) throw draft;

    if (!(await supports(DATA_READ))) {
      throw refusal("DataReadUnsupported", "This host does not support reading page data.");
    }

    let response;
    try {
      response = await fetchImpl(dataUrl(rules, getLocation()), {
        method: "GET",
        credentials: "same-origin",
        cache: "no-store",
        redirect: "error",
      });
    } catch (cause) {
      throw Object.assign(refusal("DataReadFailed", cause?.message || "Could not read page data."), { cause });
    }

    if (!response.ok) throw await responseError(response);
    const { data, meta } = splitMeta(await responseBody(response), response);
    return { data, etag: responseEtag(response), ...meta };
  }

  function loadWriteEngine() {
    if (writeEngine) return writeEngine;

    writeEngine = import("./page-data-write.js").then(({ createPageDataWrite }) => createPageDataWrite({
      supports,
      token,
      fetchImpl,
      getLocation,
      eventTarget,
      getLiveSync: () => liveSync,
      loadWriteModules,
      syncTimeoutMs,
      httpTimeoutMs,
      splitMeta,
    })).catch((error) => {
      writeEngine = null;
      throw error;
    });

    return writeEngine;
  }

  async function writeData(values, options) {
    const draft = await draftRefusal();
    if (draft) throw draft;

    if (!(await supports(DATA_WRITE))) {
      throw refusal("DataWriteUnsupported", "This host does not support writing page data.");
    }

    const engine = await loadWriteEngine();
    return engine.writeData(values, options);
  }

  return { readData, writeData, setLiveSync };
}

const pageData = createPageDataApi();

export const readData = pageData.readData;
export const writeData = pageData.writeData;
export const setPageDataLiveSync = pageData.setLiveSync;
