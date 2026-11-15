import { saveToken } from "../core/host-attrs.js";
import { storage } from "../lib/doc-storage.js";
import { isSaveInProgress } from "../core/save-core.js";

const TITLE = "Some saved data was removed";
const TITLES = {
  recent: "A change from outside this page removed something you typed",
  "all-typed": "A change from outside this page removed everything you typed",
  emptied: "A change from outside this page emptied this page's data",
  unreadable: "A change from outside this page left its data unreadable",
};
const TYPED_SHOWN = 3;
const HOUR_MS = 3600000;
const DONE_KEY = "data-guard-done";
const READ_TIMEOUT_MS = 6000;
const AFTER_SAVE_MS = 1500;
const MIN_READ_GAP_MS = 5000;
const VISIBLE_GAP_MS = 30000;
const EXTERNAL_WINDOW_MS = 10000;
const SETTLE_CAP_MS = 6000;
const SAVE_UNSETTLED_EVENTS = ["clay:save-error", "clay:save-offline", "clay:save-conflict"];
const UNREACHED = "Could not reach the server. Try again.";
const REFUSED = "That did not go through. Try again.";
const SAVE_FIRST = "Save your changes first, then try again.";
const RELOAD_FIRST = "Reload the page, then try again.";

let session = null;
let ui = null;

const plural = (count, word) => `${count} ${word}${count === 1 ? "" : "s"}`;
const isPending = (event) => Boolean(event) && typeof event.id === "string" && event.status === "pending";
const allowed = (current, event, choice, flag) => flag === true && !current.refused.has(`${event.id}:${choice}`);
const canRestore = (current, event) => allowed(current, event, "restore", event.restorable);
const canRevert = (current, event) => allowed(current, event, "revert", event.canRevert);
const canPutBack = (current, event) => allowed(current, event, "put-back", event.canPutBack);
const typed = (event) => (Array.isArray(event.typed) ? event.typed : []);
const reasoned = (event) => Boolean(event.reason);
const titleFor = (event) => TITLES[event.reason] ?? TITLE;
const typedLoss = (event) => event.reason === "recent" || event.reason === "all-typed";
const toneFor = (event) => (typedLoss(event) ? "info" : "warning");
const credentials = () => (saveToken() ? "omit" : "same-origin");
const settling = (current) =>
  isSaveInProgress() || (current.lastExternal > 0 && Date.now() - current.lastExternal < EXTERNAL_WINDOW_MS);
const headers = (extra = {}) => ({ "Document-URL": location.href, ...extra });

function removeServerBar() {
  for (const bar of document.querySelectorAll("[data-htmlclay-guard]")) bar.remove();
}

function say(message, type) {
  import("../ui/toast.js").then((module) => {
    ui = module;
    module.default(message, type);
  }, () => {});
}

function sourceOf(event) {
  const summary = event.lossSummary || {};
  const source = summary.provenance === "external" ? "A change from outside this page" : "A save from this page";
  const loss = summary.removedItems > 0
    ? `removed ${plural(summary.removedItems, "item")}`
    : summary.destroyedAtoms > 0 ? `erased ${plural(summary.destroyedAtoms, "value")}` : "changed saved data";
  const row = Array.isArray(event.preview) ? event.preview[0] : null;
  const parts = [`${source} ${loss}.`];
  if (row) parts.push(`${row.key}: ${row.yours} before, ${row.now} now.`);
  return parts;
}

function labelFor(path) {
  const parts = String(path ?? "").split(".").filter((part) => part && !/^\[[^\]]*\]$/.test(part));
  const last = (parts[parts.length - 1] ?? "").replace(/\[[^\]]*\]/g, "").trim();
  return last.replace(/[_-]+/g, " ") || "value";
}

function typedLine(event) {
  const rows = typed(event);
  if (!rows.length) return "";
  const shown = rows.slice(0, TYPED_SHOWN).map((row) => `${labelFor(row.path)}: "${row.yours}"`);
  const more = rows.length - shown.length;
  return `${shown.join("; ")}${more > 0 ? ` and ${more} more.` : "."}`;
}

function typedWhen(event) {
  if (event.reason === "all-typed") return "";
  const row = typed(event)[0];
  const at = typeof row?.at === "number" ? row.at : Date.parse(row?.at);
  if (!Number.isFinite(at)) return "";
  const ago = Date.now() - at;
  if (ago < 0 || ago >= HOUR_MS) return "";
  const minutes = Math.max(1, Math.floor(ago / 60000));
  return `You typed ${typed(event).length === 1 ? "it" : "them"} ${plural(minutes, "minute")} ago.`;
}

function detailFor(current, event) {
  if (!reasoned(event)) {
    const parts = sourceOf(event);
    if (canRestore(current, event)) {
      if (event.droppedAdditions > 0) parts.push(`Restoring removes ${plural(event.droppedAdditions, "item")} added or changed since.`);
    } else if (canRevert(current, event)) {
      parts.push("This data no longer fits the page, but the whole change can be undone.");
    } else {
      parts.push("It cannot be restored from here.");
    }
    if (canRevert(current, event)) parts.push("Undo the change also removes edits made since.");
    return parts.join(" ");
  }
  const parts = [];
  if (typedLoss(event)) {
    parts.push(typedLine(event) || sourceOf(event).join(" "));
    const when = typedWhen(event);
    if (when) parts.push(when);
  } else {
    parts.push(sourceOf(event).join(" "));
  }
  if (canRevert(current, event)) parts.push("Undo the change also removes edits made since.");
  return parts.join(" ");
}

function actionsFor(current, event) {
  const actions = [];
  if (!reasoned(event)) {
    if (canRestore(current, event)) actions.push({ label: "Restore", onClick: () => choose(current, event, "restore") });
    if (canRevert(current, event)) actions.push({ label: "Undo the change", onClick: () => choose(current, event, "revert") });
    actions.push({ label: "Keep as is", onClick: () => choose(current, event, "dismiss") });
    return actions;
  }
  if (canPutBack(current, event)) actions.push({ label: "Put it back", onClick: () => choose(current, event, "put-back") });
  if (canRevert(current, event)) actions.push({ label: "Undo the change", onClick: () => choose(current, event, "revert") });
  actions.push({ label: "Keep the change", onClick: () => choose(current, event, "dismiss") });
  return actions;
}

async function draw(current, event) {
  const title = titleFor(event);
  const detail = detailFor(current, event);
  const key = [
    event.id, title, event.reason ?? "", JSON.stringify(typed(event)),
    canRestore(current, event), canPutBack(current, event), canRevert(current, event),
    event.lastWriteAt, detail,
  ].join("|");
  if (current.drawn === key) return true;
  current.drawn = key;
  current.eventId = event.id;
  try {
    ui ??= await import("../ui/toast.js");
  } catch {
    if (current.drawn === key) {
      current.drawn = null;
      current.eventId = null;
    }
    return false;
  }
  if (session !== current || current.drawn !== key) return false;
  const previous = current.title;
  current.title = title;
  if (previous && previous !== title) ui.dismissPersistent(previous, { now: true });
  ui.dismissPersistent(title, { now: true });
  ui.toastPersistent(title, toneFor(event), { detail, actions: actionsFor(current, event) });
  return true;
}

function clearNotice(current) {
  const title = current.title;
  current.drawn = null;
  current.eventId = null;
  current.title = null;
  if (title) ui?.dismissPersistent(title, { now: true });
}

async function read(current) {
  const turn = ++current.turn;
  current.lastRead = Date.now();
  const controller = new AbortController();
  const timer = setTimeout(() => controller.abort(), READ_TIMEOUT_MS);
  let body;
  try {
    const res = await fetch(current.url, {
      method: "GET",
      credentials: credentials(),
      headers: headers(),
      cache: "no-store",
      redirect: "manual",
      signal: controller.signal,
    });
    if (!res.ok) return;
    body = await res.json();
  } catch {
    return;
  } finally {
    clearTimeout(timer);
  }
  if (session !== current || turn !== current.turn) return;
  if (!body || typeof body !== "object" || !("event" in body)) return;
  if (isPending(body.event)) {
    if (current.resolved.has(body.event.id) || current.waiting === body.event.id) return;
    if (!current.waited.has(body.event.id) && settling(current)) {
      waitForSettle(current, body.event.id);
      return;
    }
    if (await draw(current, body.event)) removeServerBar();
    return;
  }
  clearNotice(current);
  removeServerBar();
}

function forget(current, event) {
  current.resolved.add(event.id);
  if (current.eventId === event.id) clearNotice(current);
}

function finish(current, event, choice) {
  forget(current, event);
  if (choice === "dismiss") {
    say("Kept the page as it is.", "info");
    return;
  }
  storage.session.set(DONE_KEY, choice);
  current.reload();
}

function settle(current, event, choice, status, body) {
  if (session !== current) return;
  if (status === 200 && body?.ok === true) {
    finish(current, event, choice);
  } else if (status === 404) {
    forget(current, event);
    say("This was already taken care of somewhere else. Reload to see the current page.", "info");
  } else if (status === 409) {
    current.drawn = null;
    say("This page changed after the notice appeared. Check the notice and try again.", "warning");
    read(current);
  } else if (status === 422) {
    current.refused.add(`${event.id}:${choice}`);
    if (current.eventId === event.id && !current.resolved.has(event.id)) draw(current, event);
    say(choice === "restore" || choice === "put-back" ? "That data no longer fits this page, so it could not be restored." : "That earlier version is no longer available.", "warning");
  } else if (status === 401 || status === 403) {
    say(RELOAD_FIRST, "error");
  } else {
    say(status === 400 ? REFUSED : UNREACHED, "error");
  }
}

async function saveFirst() {
  const clay = window.clay;
  if (!clay?.hasUnsavedChanges?.()) return true;
  if (typeof clay.save?.flush !== "function") return false;
  try {
    await clay.save.flush();
  } catch {
    return false;
  }
  return !clay.hasUnsavedChanges();
}

async function choose(current, event, choice) {
  if (current.busy || session !== current) return;
  current.busy = true;
  try {
    if (choice !== "dismiss" && !(await saveFirst())) {
      if (session === current) say(SAVE_FIRST, "warning");
      return;
    }
    if (session !== current) return;
    const res = await fetch(current.url, {
      method: "POST",
      credentials: credentials(),
      headers: headers({ "Content-Type": "application/json" }),
      redirect: "manual",
      body: JSON.stringify({ id: event.id, choice }),
    });
    const body = await res.json().catch(() => null);
    settle(current, event, choice, res.status, body);
  } catch {
    if (session === current) say(UNREACHED, "error");
  } finally {
    current.busy = false;
  }
}

function onNotification(current, e) {
  const { msgType, action, data } = e.detail || {};
  if (msgType !== "data-loss") return;
  if (action === "resolved") {
    if (typeof data?.id !== "string") return;
    current.turn++;
    current.resolved.add(data.id);
    if (current.waiting === data.id) stopWaiting(current);
    if (current.eventId === data.id) clearNotice(current);
    return;
  }
  if (action === "raised" && isPending(data) && !current.resolved.has(data.id)) {
    current.turn++;
    if (current.waiting === data.id) return;
    if (!current.waited.has(data.id) && settling(current)) {
      waitForSettle(current, data.id);
      return;
    }
    draw(current, data).then((drawn) => { if (drawn) removeServerBar(); }, () => {});
  }
}

const readAt = (current) => Math.max(current.lastSave + AFTER_SAVE_MS, current.lastRead + MIN_READ_GAP_MS);

function stopWaiting(current) {
  if (!current.waiting) return false;
  current.waiting = null;
  clearTimeout(current.waitTimer);
  current.waitTimer = null;
  return true;
}

function endWait(current, { now = false } = {}) {
  if (!stopWaiting(current) || session !== current) return;
  if (now) read(current);
  else armRead(current);
}

function waitForSettle(current, id) {
  current.waited.add(id);
  current.waiting = id;
  clearTimeout(current.waitTimer);
  current.waitTimer = setTimeout(() => endWait(current, { now: true }), SETTLE_CAP_MS);
}

function armRead(current) {
  if (current.timer) return;
  current.timer = setTimeout(() => {
    current.timer = null;
    if (session !== current) return;
    if (readAt(current) > Date.now()) armRead(current);
    else read(current);
  }, Math.max(readAt(current) - Date.now(), 0));
}

export function start({ href, reload = () => window.location.reload() }) {
  if (session) return;
  let url;
  try {
    url = new URL(href, location.href);
  } catch {
    return;
  }
  if (url.origin !== location.origin) return;
  const listeners = new AbortController();
  const current = {
    url: url.href, reload, listeners, turn: 0, lastRead: 0, lastSave: 0, timer: null,
    lastExternal: 0, waiting: null, waitTimer: null, waited: new Set(),
    busy: false, drawn: null, eventId: null, title: null, resolved: new Set(), refused: new Set(),
  };
  session = current;
  const { signal } = listeners;
  document.addEventListener("clay:notification", (e) => onNotification(current, e), { signal });
  document.addEventListener("clay:sync-applied", (e) => {
    if (e.detail?.source === "disk") current.lastExternal = Date.now();
  }, { signal });
  document.addEventListener("clay:save-saved", () => {
    current.lastSave = Date.now();
    if (current.waiting) endWait(current);
    else armRead(current);
  }, { signal });
  for (const name of SAVE_UNSETTLED_EVENTS) {
    document.addEventListener(name, () => endWait(current, { now: true }), { signal });
  }
  document.addEventListener("visibilitychange", () => {
    if (document.visibilityState === "visible" && Date.now() - current.lastRead >= VISIBLE_GAP_MS) read(current);
  }, { signal });
  const done = storage.session.get(DONE_KEY);
  if (done) {
    storage.session.remove(DONE_KEY);
    say(done === "restore" || done === "put-back" ? "Your saved data is back." : "The earlier version is back.", "success");
  }
  read(current);
}

export function stop() {
  const current = session;
  if (!current) return;
  session = null;
  current.listeners.abort();
  clearTimeout(current.timer);
  clearTimeout(current.waitTimer);
  clearNotice(current);
}
