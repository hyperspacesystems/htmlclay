import { saveToken } from "../core/host-attrs.js";
import { storage } from "../lib/doc-storage.js";

const TITLE = "Saved data was overwritten";
const DONE_KEY = "data-guard-done";
const READ_TIMEOUT_MS = 6000;
const AFTER_SAVE_MS = 1500;
const MIN_READ_GAP_MS = 5000;
const VISIBLE_GAP_MS = 30000;
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
const credentials = () => (saveToken() ? "omit" : "same-origin");
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

function describe(current, event) {
  const summary = event.lossSummary || {};
  const source = summary.provenance === "external" ? "A change from outside this page" : "A save from this page";
  const loss = summary.removedItems > 0
    ? `removed ${plural(summary.removedItems, "item")}`
    : summary.destroyedAtoms > 0 ? `erased ${plural(summary.destroyedAtoms, "value")}` : "changed saved data";
  const row = Array.isArray(event.preview) ? event.preview[0] : null;
  const parts = [`${source} ${loss}.`];
  if (row) parts.push(`${row.key}: ${row.yours} before, ${row.now} now.`);
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

function actionsFor(current, event) {
  const actions = [];
  if (canRestore(current, event)) actions.push({ label: "Restore", onClick: () => choose(current, event, "restore") });
  if (canRevert(current, event)) actions.push({ label: "Undo the change", onClick: () => choose(current, event, "revert") });
  actions.push({ label: "Keep as is", onClick: () => choose(current, event, "dismiss") });
  return actions;
}

async function draw(current, event) {
  const key = [event.id, canRestore(current, event), canRevert(current, event), event.lastWriteAt, describe(current, event)].join("|");
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
  ui.dismissPersistent(TITLE, { now: true });
  ui.toastPersistent(TITLE, "warning", { detail: describe(current, event), actions: actionsFor(current, event) });
  return true;
}

function clearNotice(current) {
  current.drawn = null;
  current.eventId = null;
  ui?.dismissPersistent(TITLE, { now: true });
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
    if (current.resolved.has(body.event.id)) return;
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
    say(choice === "restore" ? "That data no longer fits this page, so it could not be restored." : "That earlier version is no longer available.", "warning");
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
    if (current.eventId === data.id) clearNotice(current);
    return;
  }
  if (action === "raised" && isPending(data) && !current.resolved.has(data.id)) {
    current.turn++;
    draw(current, data).then((drawn) => { if (drawn) removeServerBar(); }, () => {});
  }
}

const readAt = (current) => Math.max(current.lastSave + AFTER_SAVE_MS, current.lastRead + MIN_READ_GAP_MS);

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
    busy: false, drawn: null, eventId: null, resolved: new Set(), refused: new Set(),
  };
  session = current;
  const { signal } = listeners;
  document.addEventListener("clay:notification", (e) => onNotification(current, e), { signal });
  document.addEventListener("clay:save-saved", () => {
    current.lastSave = Date.now();
    armRead(current);
  }, { signal });
  document.addEventListener("visibilitychange", () => {
    if (document.visibilityState === "visible" && Date.now() - current.lastRead >= VISIBLE_GAP_MS) read(current);
  }, { signal });
  const done = storage.session.get(DONE_KEY);
  if (done) {
    storage.session.remove(DONE_KEY);
    say(done === "restore" ? "Your saved data is back." : "The earlier version is back.", "success");
  }
  read(current);
}

export function stop() {
  const current = session;
  if (!current) return;
  session = null;
  current.listeners.abort();
  clearTimeout(current.timer);
  clearNotice(current);
}
