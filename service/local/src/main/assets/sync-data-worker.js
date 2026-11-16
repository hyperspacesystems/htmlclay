// Hyperclay Local's shared data worker: one /_/sync connection per origin for
// every edit tab, carrying each editor's room events and roster.
//
// It sits beside sync-worker.js, which is a byte-for-byte copy of HTML Clay's
// worker and stays untouched: this file is Local's own, speaks only the v2 wire
// (`v: 2`, plus `presence`), and its algorithm -- the snapshot content cache, the
// replay baseline, snapshot frames sent to hidden pages too, a debounced rebuild,
// exponential backoff and the port sweep -- is derived from that copy. An edit
// tab can hold one of the browser's six connections to localhost, so six open
// documents would otherwise starve the seventh; here every page shares this
// worker's one stream, and the server keys one member per editing page inside it.
//
// State: `subs` is keyed by lane plus document, `members` by document plus client
// id, and `ports` by the page's MessagePort. A subscribe that names the same
// document as another page joins that snapshot entry; a live subscribe that names
// its own client id gets its own member, and the member's named events are the
// only frames it receives from that entry's room.
"use strict";

const timing = Object.assign(
  { debounce: 50, portTTL: 60000, hiddenPortTTL: 300000, sweep: 30000, backoffMin: 1000, backoffMax: 30000 },
  self.claySyncTiming
);

// The host's caps on one stream (livesync.go maxSharedSubs and its member list).
// Checked here so the page past either one is answered on its own port, instead
// of the host refusing the whole list and every other page with it.
const maxSubs = 256;
const maxMembers = 256;

// The whole list rides in the stream's URL, and a host answers nothing to a request
// whose headers pass its limit (16 KB in Node), which would cut every page off at
// once. Half of that is left for the other headers: a browser sends every cookie set
// for localhost, whatever port set it. A page whose entry would take the query past
// this is answered on its own port, like one past either count.
const limits = Object.assign({ maxQueryBytes: 8192 }, self.claySyncLimits);

// The client id a live member's proof is minted for, and the three room events
// plus the roster that a member envelope may carry.
const clientIdPattern = /^[A-Za-z0-9_-]{1,64}$/;
const namedEvents = ["data", "data-state", "data-writer", "presence"];

// key -> { document, lane, since, ready, latest, content, needsResync, ports, index }
const subs = new Map();
// key -> { document, clientId, ready, presence, ports, index }
const members = new Map();
// port -> { subKey, memberKey, visible, cursored, open, seen, lastSeen }
const ports = new Map();

let stream = null;
let rebuildTimer = null;
let reconnectTimer = null;
let backoff = timing.backoffMin;

function subKeyOf(lane, document) {
  return lane + "\n" + document;
}

function memberKeyOf(document, clientId) {
  return document + "\n" + clientId;
}

function send(port, msg) {
  try {
    port.postMessage(Object.assign({ v: 2 }, msg));
  } catch (_) {
    drop(port);
  }
}

function notify(state) {
  for (const port of ports.keys()) send(port, { type: "status", state });
}

self.onconnect = (event) => {
  const port = event.ports[0];
  ports.set(port, {
    subKey: null,
    memberKey: null,
    visible: true,
    cursored: false,
    open: false,
    seen: 0,
    lastSeen: Date.now(),
  });
  port.onmessage = (e) => handle(port, e.data);
  port.onmessageerror = () => drop(port);
  if (typeof port.addEventListener === "function") port.addEventListener("close", () => drop(port));
  // The feature list is how a page knows this worker speaks room events and the
  // roster. It proves the script loaded, not that any subscription is ready.
  send(port, { type: "hello", features: ["live-data", "presence"] });
};

function handle(port, msg) {
  const meta = ports.get(port);
  if (!meta || !msg || msg.v !== 2) return;
  meta.lastSeen = Date.now();
  switch (msg.type) {
    case "subscribe":
      subscribe(port, meta, msg);
      break;
    case "unsubscribe":
      unsubscribe(port, meta);
      break;
    case "visible":
      meta.visible = true;
      deliverLatest(port, meta);
      break;
    case "hidden":
      meta.visible = false;
      break;
    case "ping":
      send(port, { type: "pong" });
      break;
  }
}

// The query a rebuild would send with `extra` entries added, measured as rebuild
// builds it.
function queryBytes(extra) {
  const params = new URLSearchParams();
  params.set("v", "2");
  for (const sub of subs.values()) params.append("s", sub.lane + ":" + sub.since + ":" + sub.document);
  for (const member of members.values()) params.append("m", member.clientId + ":" + member.document);
  for (const [name, value] of extra) params.append(name, value);
  return params.toString().length;
}

function subscribe(port, meta, msg) {
  const live = msg.lane === "live";
  // Everything is validated, and both post-replacement map sizes are checked,
  // before the port's previous subscription is disturbed: a page refused here is
  // refused on its own, and every other page keeps the stream it had.
  if (typeof msg.document !== "string" || msg.document === "" ||
      (msg.lane !== "live" && msg.lane !== "saved") ||
      !Number.isSafeInteger(msg.since) || msg.since < 0 ||
      (live
        ? (typeof msg.clientId !== "string" || !clientIdPattern.test(msg.clientId))
        : msg.clientId !== undefined)) {
    send(port, { type: "gone" });
    return;
  }
  const subKey = subKeyOf(msg.lane, msg.document);
  const memberKey = live ? memberKeyOf(msg.document, msg.clientId) : null;
  const extra = [];
  if (!subs.has(subKey)) extra.push(["s", msg.lane + ":" + msg.since + ":" + msg.document]);
  if (memberKey !== null && !members.has(memberKey)) extra.push(["m", msg.clientId + ":" + msg.document]);
  if ((!subs.has(subKey) && subs.size >= maxSubs) ||
      (memberKey !== null && !members.has(memberKey) && members.size >= maxMembers) ||
      (extra.length > 0 && queryBytes(extra) > limits.maxQueryBytes)) {
    send(port, { type: "capacity" });
    return;
  }
  unsubscribe(port, meta);
  let added = false;
  let sub = subs.get(subKey);
  if (!sub) {
    sub = {
      document: msg.document,
      lane: msg.lane,
      since: msg.since,
      ready: false,
      latest: null,
      content: new Map(),
      needsResync: false,
      ports: new Set(),
      index: -1,
    };
    subs.set(subKey, sub);
    added = true;
  } else if (!sub.ready && msg.since > 0 && (sub.since === 0 || msg.since < sub.since)) {
    // Two pages asking for one document before its stream is open: resume from
    // the lower position, and let each page's own staleness check drop what it
    // has already applied. Zero is the absence of a position, not a low one.
    sub.since = msg.since;
  }
  sub.ports.add(port);
  meta.subKey = subKey;
  meta.seen = 0;
  meta.since = msg.since;
  meta.cursored = false;
  meta.open = false;
  if (memberKey !== null) {
    let member = members.get(memberKey);
    if (!member) {
      member = { document: msg.document, clientId: msg.clientId, ready: false, presence: null, ports: new Set(), index: -1 };
      members.set(memberKey, member);
      added = true;
    }
    member.ports.add(port);
    meta.memberKey = memberKey;
  }
  if (added) {
    scheduleRebuild();
    return;
  }
  // A port joining a pair this connection already has opens at once, with no new
  // stream: its cursor, then its open, then the roster that member last saw.
  if (sub.ready) {
    sendCursor(port, meta, sub, sub.needsResync);
    tryOpen(port, meta);
  }
}

function sendCursor(port, meta, sub, resync) {
  const behind = !meta.cursored && meta.since > 0 && meta.since < sub.since && sub.content.size === 0;
  meta.cursored = true;
  send(port, { type: "cursor", seq: sub.since, resync: resync || behind });
}

// A live port needs this connection's snapshot cursor AND its member cursor; a
// saved port needs the snapshot cursor alone. Open is sent exactly once per
// connection, and the roster only after it: a page starts a fresh presence
// session when it opens, so a roster before open would lose its token.
function tryOpen(port, meta) {
  if (meta.open || meta.subKey === null) return;
  const sub = subs.get(meta.subKey);
  if (!sub || !sub.ready || !meta.cursored) return;
  if (meta.memberKey !== null) {
    const member = members.get(meta.memberKey);
    if (!member || !member.ready) return;
  }
  meta.open = true;
  send(port, { type: "status", state: "open" });
  deliverPresence(port, meta);
  deliverLatest(port, meta);
}

function deliverPresence(port, meta) {
  if (!meta.open || meta.memberKey === null) return;
  const member = members.get(meta.memberKey);
  if (!member || member.presence === null) return;
  send(port, { type: "event", event: "presence", data: member.presence });
}

function unsubscribe(port, meta) {
  const subKey = meta.subKey;
  meta.subKey = null;
  meta.cursored = false;
  meta.open = false;
  if (subKey !== null) {
    const sub = subs.get(subKey);
    if (sub) {
      sub.ports.delete(port);
      if (sub.ports.size === 0) {
        subs.delete(subKey);
        if (sub.lane === "live") releaseMembersOf(sub.document);
        scheduleRebuild();
      }
    }
  }
  const memberKey = meta.memberKey;
  meta.memberKey = null;
  if (memberKey !== null) {
    const member = members.get(memberKey);
    if (member) {
      member.ports.delete(port);
      if (member.ports.size === 0) {
        members.delete(memberKey);
        scheduleRebuild();
      }
    }
  }
}

function releaseMembersOf(document) {
  for (const [key, member] of Array.from(members)) {
    if (member.document !== document) continue;
    for (const port of member.ports) {
      const meta = ports.get(port);
      if (meta) meta.memberKey = null;
    }
    member.ports.clear();
    members.delete(key);
  }
}

function drop(port) {
  const meta = ports.get(port);
  if (!meta) return;
  ports.delete(port);
  unsubscribe(port, meta);
  try {
    port.close();
  } catch (_) {}
}

function deliverLatest(port, meta) {
  if (meta.subKey === null || !meta.open) return;
  const sub = subs.get(meta.subKey);
  if (!sub || sub.latest === null || sub.latest.id === meta.seen) return;
  const frames = [...sub.content.values(), sub.latest].sort((a, b) => a.id - b.id);
  for (const frame of frames) {
    if (frame.id <= meta.seen) continue;
    meta.seen = frame.id;
    send(port, { type: "frame", data: frame.data });
  }
}

function scheduleRebuild() {
  clearTimeout(rebuildTimer);
  rebuildTimer = setTimeout(rebuild, timing.debounce);
}

function rebuild() {
  clearTimeout(rebuildTimer);
  rebuildTimer = null;
  clearTimeout(reconnectTimer);
  reconnectTimer = null;
  if (stream !== null) {
    stream.close();
    stream = null;
  }
  // Every subscription and membership is unready for the new connection, and
  // every port is waiting for a cursor and an open that have not arrived yet.
  // A roster belongs to the connection that carried it: its session ends with
  // that connection, so a page that opens on the new one must not be handed it.
  for (const sub of subs.values()) {
    sub.ready = false;
    sub.index = -1;
  }
  for (const member of members.values()) {
    member.ready = false;
    member.index = -1;
    member.presence = null;
  }
  for (const meta of ports.values()) {
    meta.cursored = false;
    meta.open = false;
  }
  if (subs.size === 0) return;
  const subOrder = Array.from(subs.values());
  const memberOrder = Array.from(members.values());
  const params = new URLSearchParams();
  params.set("v", "2");
  subOrder.forEach((sub, i) => {
    sub.index = i;
    params.append("s", sub.lane + ":" + sub.since + ":" + sub.document);
  });
  memberOrder.forEach((member, j) => {
    member.index = j;
    params.append("m", member.clientId + ":" + member.document);
  });
  const es = new EventSource(self.location.origin + "/_/sync?" + params);
  stream = es;
  notify("connecting");
  // HTTP open only resets the backoff. A port opens on its own cursors, and only
  // a valid hello proves the page's worker, not the subscription beneath it.
  es.onopen = () => {
    if (stream !== es) return;
    backoff = timing.backoffMin;
  };
  es.addEventListener("cursor", (e) => {
    if (stream !== es) return;
    onCursor(subOrder, memberOrder, e);
  });
  subOrder.forEach((sub, i) => es.addEventListener("s" + i, (e) => {
    if (stream !== es) return;
    onFrame(sub, e);
  }));
  memberOrder.forEach((member, j) => es.addEventListener("m" + j, (e) => {
    if (stream !== es) return;
    onNamed(member, e);
  }));
  es.onerror = () => {
    if (stream !== es) return;
    // The browser's own reconnect would reuse this URL and every position in it.
    // Rebuilding instead resumes each subscription from where it is now.
    es.close();
    stream = null;
    notify("connecting");
    reconnectTimer = setTimeout(rebuild, backoff);
    backoff = Math.min(backoff * 2, timing.backoffMax);
  };
}

function onCursor(subOrder, memberOrder, e) {
  let cursor;
  try {
    cursor = JSON.parse(e.data);
  } catch (_) {
    return;
  }
  if (!cursor || typeof cursor !== "object" || Array.isArray(cursor)) return;
  if (cursor.v !== 2) {
    // A v2 request answered without the marker is a host that rolled back its
    // wire, not a document that went away: the pages take their own direct
    // stream rather than wait for members that will never arrive.
    unsupported();
    return;
  }
  const hasSub = Number.isInteger(cursor.sub);
  const hasMember = Number.isInteger(cursor.member);
  if (hasSub === hasMember) return;
  if (hasSub) {
    const sub = subOrder[cursor.sub];
    if (!sub || subs.get(subKeyOf(sub.lane, sub.document)) !== sub) return;
    if (cursor.error) {
      notFoundSub(sub);
      return;
    }
    sub.ready = true;
    if (cursor.resync === true) {
      sub.latest = null;
      sub.content.clear();
      // A cursor cannot prove that a later frame repairs both peer and disk state.
      sub.needsResync = true;
    }
    if (typeof cursor.seq === "number") sub.since = cursor.seq;
    for (const port of sub.ports) {
      const meta = ports.get(port);
      if (!meta) continue;
      sendCursor(port, meta, sub, cursor.resync === true);
      tryOpen(port, meta);
    }
    return;
  }
  const member = memberOrder[cursor.member];
  if (!member || members.get(memberKeyOf(member.document, member.clientId)) !== member) return;
  if (cursor.error) {
    notFoundMember(member);
    return;
  }
  member.ready = true;
  for (const port of member.ports) {
    const meta = ports.get(port);
    if (meta) tryOpen(port, meta);
  }
}

function onFrame(sub, e) {
  const id = Number(e.lastEventId);
  if (Number.isFinite(id) && id > sub.since) sub.since = id;
  sub.latest = { id: sub.since, data: e.data };
  try {
    const data = JSON.parse(e.data);
    if (data.type === "notification" && (data.data?.kind === "external-change" ||
        (typeof data.msg === "string" && data.msg.endsWith("changed on disk outside this tab")))) {
      sub.content.set("disk", sub.latest);
    } else if (typeof data.html === "string") {
      sub.content.set("peer", sub.latest);
    }
  } catch (_) {}
  // Hidden pages get every frame at once. A hidden edit tab that waited to be
  // shown kept the stamp of the version before a peer's save, and its next save
  // was refused as a conflict. Visibility only sets how long a silent port lives.
  for (const port of sub.ports) {
    const meta = ports.get(port);
    if (!meta || !meta.open) continue;
    meta.seen = sub.latest.id;
    send(port, { type: "frame", data: e.data });
  }
}

// A member's envelope: one of the room's named events, or the roster, with an
// object payload. Nothing here advances a snapshot position -- a room sequence
// and a replay cursor are different numbers -- and nothing is flattened or
// rewritten on the way through.
function onNamed(member, e) {
  let envelope;
  try {
    envelope = JSON.parse(e.data);
  } catch (_) {
    return;
  }
  if (!envelope || typeof envelope !== "object" || Array.isArray(envelope)) return;
  if (namedEvents.indexOf(envelope.event) < 0) return;
  const data = envelope.data;
  if (data === null || typeof data !== "object" || Array.isArray(data)) return;
  if (envelope.event === "presence") member.presence = data;
  for (const port of member.ports) {
    const meta = ports.get(port);
    if (!meta) continue;
    // Named events reach a hidden port at once; the roster waits only for open.
    if (envelope.event === "presence" && !meta.open) continue;
    send(port, { type: "event", event: envelope.event, data });
  }
}

// One entry the host will not serve is answered on the ports that asked for it,
// and only those: the other documents keep their stream.
function notFoundSub(sub) {
  for (const port of Array.from(sub.ports)) {
    const meta = ports.get(port);
    if (meta) meta.subKey = null;
    send(port, { type: "gone" });
    drop(port);
  }
  sub.ports.clear();
  subs.delete(subKeyOf(sub.lane, sub.document));
  if (sub.lane === "live") releaseMembersOf(sub.document);
  scheduleRebuild();
}

function notFoundMember(member) {
  for (const port of Array.from(member.ports)) {
    const meta = ports.get(port);
    if (meta) meta.memberKey = null;
    send(port, { type: "gone" });
    drop(port);
  }
  member.ports.clear();
  members.delete(memberKeyOf(member.document, member.clientId));
  scheduleRebuild();
}

function unsupported() {
  clearTimeout(rebuildTimer);
  rebuildTimer = null;
  clearTimeout(reconnectTimer);
  reconnectTimer = null;
  if (stream !== null) {
    stream.close();
    stream = null;
  }
  for (const port of Array.from(ports.keys())) {
    send(port, { type: "unsupported" });
    ports.delete(port);
    try {
      port.close();
    } catch (_) {}
  }
  subs.clear();
  members.clear();
}

// A page that closed without saying so stops pinging. A visible page is gone
// after portTTL; a hidden one is kept longer, because Chrome runs a hidden tab's
// timers about once a minute and a healthy hidden page pings that slowly.
setInterval(() => {
  const now = Date.now();
  for (const [port, meta] of Array.from(ports)) {
    const ttl = meta.visible ? timing.portTTL : timing.hiddenPortTTL;
    if (meta.lastSeen < now - ttl) drop(port);
  }
}, timing.sweep);
