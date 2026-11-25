// The ordered room type: one host process's ordered rooms over the live-room core
// (src/live-room/core, generated from the lab). It does no I/O. The host reads and
// writes every file, sends every frame, and tells the registry what happened.
import { randomInt } from 'node:crypto';
import { admit, dropConnection, newInstance, rejoin, retained, stateReserve } from '../../live-room/core/room.js';
import { establishing, fileAccepted as accepted, outsideStep, render, sameTables } from '../../live-room/core/host.js';
import { parse } from '../../live-room/core/record.js';
import { useCodec } from '../../live-room/core/codec.js';
import { REAL_LIMITS, ROOM_HEAP } from '../../live-room/core/limits.js';
import { LIVE_LIMITS } from './live-data-rooms.js';

const silent = new Proxy({}, { get: () => () => {} });
const ID = /^[A-Za-z0-9_.:/#-]{1,128}$/;
const PART_SLACK = 512;
const STAGE_COST = 1024;
const SLOT_COST = 64;
const STAGES_PER_CLIENT = 8;
const BYTES_PER_OP = 8;
const whole = (n) => Number.isSafeInteger(n) && n >= 0;
const id = (text) => typeof text === 'string' && ID.test(text) && !(text in Object.prototype);
const list = (value) => value === undefined || value === null || Array.isArray(value);
const utf8 = (text) => Buffer.byteLength(text, 'utf8');
const sizeOf = (frame) => utf8(JSON.stringify(frame));

const NOBODY = Object.freeze({ used: () => 0, size: () => 0 });

/**
 * @param {{ codecs: Record<string, object>, limits?: object, now?: () => number, newId?: () => number,
 *   liveBytes?: number, maxRooms?: number, retainMs?: number, heap?: { doc: number, tail: number }, onSpace?: (key: string) => void,
 *   others?: { used: () => number, size: () => number } }} options
 */
export function createOrderedRooms({ codecs, limits = REAL_LIMITS, now = Date.now, newId = () => randomInt(2 ** 40), liveBytes = LIVE_LIMITS.liveBytes, maxRooms = LIVE_LIMITS.rooms, retainMs = LIVE_LIMITS.retainMs, heap = ROOM_HEAP, onSpace = () => {}, others = NOBODY } = {}) {
  if (!codecs || !Object.keys(codecs).length) throw new Error('createOrderedRooms needs at least one codec');
  const slots = new Map();
  const byConn = new Map();
  const counters = {};
  const maxParts = Math.max(1, Math.ceil(limits.roomBytes / (limits.opsPerFrame * 8)));

  const next = (counter) => (counters[counter] = (counters[counter] ?? newId()) + 1);

  function envOf(slot) {
    useCodec(codecs[slot.codec]);
    return {
      hostMade: true,
      removals: false,
      split: false,
      limits,
      observer: silent,
      port: {
        room: () => slot.room,
        setRoom: (room) => {
          slot.room = room;
        },
        next,
        onFull: (by) => slot.notices.push({ to: by, k: 'full' }),
        onSpace: () => slot.notices.push({ to: '*', k: 'space' }),
        tabs: () => [],
      },
    };
  }

  const open = (slot) => (slot?.room?.gen ? slot.room : null);
  const stageBytes = (stage) => [...stage.got.values()].reduce((sum, bytes) => sum + bytes, 0);
  const stageCost = (stage) => heap.tail * stageBytes(stage) + SLOT_COST * stage.m + STAGE_COST;
  const partBytes = (slot) => [...slot.parts.values()].reduce((sum, stage) => sum + stageCost(stage), 0);
  const roomBytes = (room) => (room?.gen ? room.bytes.tail + room.bytes.staged + room.bytes.waiting : 0);
  const copyBytes = (slot) => (slot.room?.restores?.size ? slot.copyBytes : 0);
  const held = (slot) => (slot?.room ? Math.ceil(heap.doc * slot.docBytes + stateReserve(limits) + heap.tail * (roomBytes(slot.room) + copyBytes(slot)) + partBytes(slot)) : 0);
  const used = () => [...slots.values()].reduce((sum, slot) => sum + held(slot), 0);
  const dirty = (room) => Boolean(room?.gen) && room.seq > room.base.through;
  const slotOf = (room) => [...slots.values()].find((slot) => slot.room === room) ?? null;

  // A room refused for the process total hears nothing from its own room when another room frees
  // space, so the registry tells the host which rooms may send again; the host sends their tabs
  // `space`. The host also calls wake() when the live rooms free space.
  function wake() {
    for (const [key, slot] of slots) {
      if (!slot.starved || used() + others.used() >= liveBytes) continue;
      slot.starved = false;
      onSpace(key);
    }
  }

  function forget(key) {
    const slot = slots.get(key);
    slots.delete(key);
    for (const [conn, owner] of byConn) if (owner === slot) byConn.delete(conn);
    return slot;
  }

  function sent(slot, from) {
    const send = [...(slot.room?.log.slice(from) ?? []), ...slot.notices];
    slot.notices = [];
    return send.filter(Boolean);
  }

  function read(slot, bytes, etag) {
    useCodec(codecs[slot.codec]);
    return { etag, ...parse(bytes) };
  }

  const validOrder = (order) => Boolean(order) && typeof order === 'object' && ((id(order.c) && whole(order.p)) || (typeof order.from === 'string' && whole(order.n)));

  const action = (codec, value, checked) => (!checked || codec.validate(value) ? null : 'bad-action');

  function validItem(codec, item, checked) {
    if (!item || typeof item !== 'object' || !whole(item.n) || !id(item.c) || !whole(item.rn)) return 'bad-frame';
    if (!list(item.via) || !list(item.org) || !list(item.origin)) return 'bad-frame';
    return item.action === null ? null : action(codec, item.action, checked);
  }

  function validWhole(codec, frame, checked) {
    if (!frame || typeof frame !== 'object' || typeof frame.gen !== 'string') return 'bad-frame';
    if (frame.k === 'req') {
      if (!id(frame.c) || !whole(frame.n) || frame.n < 1 || !whole(frame.p) || !(frame.b === null || whole(frame.b)) || !id(frame.e)) return 'bad-frame';
      return action(codec, frame.action, checked);
    }
    if (frame.k === 'adopt') {
      if (typeof frame.from !== 'string' || !Array.isArray(frame.items) || !frame.items.length) return 'bad-frame';
      for (const item of frame.items) {
        const bad = validItem(codec, item, checked);
        if (bad) return bad;
      }
      return null;
    }
    if (frame.k === 'copy') {
      if (!id(frame.job) || !whole(frame.i) || !whole(frame.n) || !id(frame.c) || !id(frame.e) || !list(frame.via) || !list(frame.origin)) return 'bad-frame';
      return action(codec, frame.action, checked);
    }
    return 'bad-frame';
  }

  function validFrame(codec, frame) {
    if (!frame || typeof frame !== 'object' || typeof frame.gen !== 'string') return 'bad-frame';
    if (frame.k === 'receipt') return typeof frame.commit === 'string' && whole(frame.through) ? null : 'bad-frame';
    if (frame.k !== 'part') return validWhole(codec, frame, true);
    if (!whole(frame.i) || !whole(frame.m) || frame.i >= frame.m || frame.m > maxParts || !id(frame.key) || !validOrder(frame.order)) return 'bad-frame';
    if (!whole(frame.size) || frame.size > limits.roomBytes) return 'bad-size';
    if (frame.m > Math.ceil(frame.size / (limits.opsPerFrame * BYTES_PER_OP)) + 1) return 'bad-frame';
    if (frame.i > 0) return frame.frame === null || frame.frame === undefined ? null : 'bad-frame';
    const head = frame.frame;
    if (!head || (head.k === 'adopt' && head.items?.length !== 1)) return 'bad-frame';
    return validWhole(codec, { ...head, gen: frame.gen }, false);
  }

  function admitPart(slot, room, clientId, frame) {
    const key = `${clientId}/${frame.key}`;
    const before = slot.parts.get(key);
    if (!before && [...slot.parts.keys()].filter((staged) => staged.startsWith(`${clientId}/`)).length >= STAGES_PER_CLIENT) return 'full';
    if (before && before.m !== frame.m) return 'bad-frame';
    // A restore's copy frame carries the rows it made (`made`) in its first part's header, which
    // the declared size of the action leaves out.
    const extra = frame.i === 0 && frame.frame?.made ? sizeOf(frame.frame.made) : (before?.extra ?? 0);
    const stage = { m: frame.m, got: new Map(before?.got), extra };
    stage.got.set(frame.i, sizeOf(frame));
    if (stageBytes(stage) > 2 * frame.size + frame.m * PART_SLACK + extra) return 'bad-size';
    slot.parts.set(key, stage);
    const answer = admit(envOf(slot), room, frame, clientId);
    for (const staged of slot.parts.keys()) if (!room.staging.has(staged)) slot.parts.delete(staged);
    return answer;
  }

  return {
    get: (key) => open(slots.get(key)),

    capability: (codec) => Object.hasOwn(codecs, codec),

    mayFit(key, bytes) {
      if (!slots.has(key) && slots.size + others.size() >= maxRooms) return false;
      const size = typeof bytes === 'string' ? utf8(bytes) : bytes;
      return used() - held(slots.get(key)) + others.used() + heap.doc * size + stateReserve(limits) <= liveBytes;
    },

    // The room instance a joining client is told about before any generation is open, so a tab
    // never makes an instance of its own: the host makes it, as `establish` does.
    instance(key, codec) {
      let slot = slots.get(key);
      if (!slot) {
        if (!Object.hasOwn(codecs, codec) || !this.mayFit(key, 0)) return null;
        slot = { codec, room: null, bytes: null, docBytes: 0, copyBytes: 0, parts: new Map(), notices: [], idle: now() };
        slots.set(key, slot);
      }
      if (!slot.room) slot.room = newInstance(envOf(slot));
      return slot.room;
    },

    establish(key, { bytes, etag, codec }) {
      if (!Object.hasOwn(codecs, codec) || !this.mayFit(key, bytes)) return null;
      let slot = slots.get(key);
      const fresh = !slot || slot.codec !== codec;
      const candidate = fresh ? { codec, room: null, bytes: null, docBytes: 0, copyBytes: 0, parts: new Map(), notices: [], idle: now() } : slot;
      let file;
      try {
        file = read(candidate, bytes, etag);
      } catch {
        return null;
      }
      if (fresh) {
        if (slot) forget(key);
        slot = candidate;
        slots.set(key, slot);
      }
      const env = envOf(slot);
      if (!slot.room) slot.room = newInstance(env);
      slot.docBytes = Math.max(slot.docBytes, utf8(bytes));
      const built = establishing(env, file);
      return { bytes: built.body, record: built.record };
    },

    open(key, { bytes, etag }) {
      const slot = slots.get(key);
      if (!slot?.room) return null;
      let file;
      try {
        file = read(slot, bytes, etag);
      } catch {
        return null;
      }
      if (open(slot)) return file.record.gen === slot.room.gen && file.record.room === slot.room.id ? slot.room : null;
      if (accepted(envOf(slot), file, read(slot, slot.bytes ?? bytes, null)) !== 'establishing') return null;
      slot.bytes = bytes;
      slot.etag = etag;
      slot.docBytes = utf8(bytes);
      slot.parts.clear();
      slot.notices = [];
      slot.idle = now();
      return slot.room;
    },

    attach(room, clientId, conn) {
      const slot = slotOf(room);
      if (!slot || !open(slot) || !id(clientId) || conn === undefined || conn === null) return null;
      const before = byConn.get(conn);
      if (before && before !== slot) this.leave(before.room, conn);
      byConn.set(conn, slot);
      (slot.conns ??= new Map()).set(conn, clientId);
      rejoin(room, clientId);
      return { gen: room.gen, base: { ...room.base }, tail: retained(room) };
    },

    frame(room, clientId, frame) {
      const slot = slotOf(room);
      if (!slot || !open(slot)) return { answer: 'closed', send: [] };
      const bad = validFrame(codecs[slot.codec], frame);
      if (bad) return { answer: bad, send: [] };
      const adds = frame.k === 'receipt' ? 0 : frame.k !== 'part' ? heap.tail * sizeOf(frame) : slot.parts.has(`${clientId}/${frame.key}`) ? 0 : 2 * heap.tail * frame.size + SLOT_COST * frame.m + STAGE_COST;
      if (adds && used() + others.used() + adds > liveBytes) {
        slot.starved = true;
        return { answer: 'full', send: [] };
      }
      const from = room.log.length;
      if (!room.restores.size) slot.copyBytes = 0;
      let answer;
      try {
        answer = frame.k === 'part' ? admitPart(slot, room, clientId, frame) : admit(envOf(slot), room, frame, clientId);
      } catch {
        answer = 'bad-frame';
      }
      // The room holds a copy frame's bytes only while it keeps the frame for a person's decision
      // (an answer with no place in the log). A frame it settled, answered before (a second
      // restore of a finished copy), or appended (charged with the log) holds nothing more.
      const keptForDecision = typeof answer === 'object' && answer !== null && answer.seq === undefined;
      if (keptForDecision && frame.k === 'copy') slot.copyBytes += sizeOf(frame);
      if (keptForDecision && frame.k === 'part' && frame.of === 'copy') slot.copyBytes += frame.size;
      return { answer, send: sent(slot, from) };
    },

    fileAccepted(key, { bytes, etag }) {
      const slot = slots.get(key);
      if (!slot?.room) return { answer: 'none', send: [] };
      const from = slot.room.log.length;
      const wasDirty = dirty(slot.room);
      let answer;
      try {
        answer = accepted(envOf(slot), read(slot, bytes, etag), read(slot, slot.bytes ?? bytes, null));
      } catch {
        answer = 'outside';
      }
      if (answer === 'outside') {
        forget(key);
        wake();
        return { answer, send: [], dirty: wasDirty };
      }
      slot.bytes = bytes;
      slot.etag = etag;
      slot.docBytes = utf8(bytes);
      if (answer === 'establishing') {
        slot.parts.clear();
        slot.copyBytes = 0;
      }
      const send = answer === 'establishing' ? sent(slot, slot.room.log.length) : sent(slot, from);
      wake();
      return { answer, send };
    },

    // Whether writing `bytes` would end the open room (fileAccepted answering 'outside'), judged
    // before the write. A browser save that is not the room's next commit is refused with this
    // (plan 09 section 2.2): a tab from before the room opened, or one that lost track of it, would
    // otherwise end the room and drop what it admitted since the last commit. Outside writes never
    // ask: they go through outsideWrite, which backs the room up.
    ends(key, bytes) {
      const slot = slots.get(key);
      const room = open(slot);
      if (!room) return false;
      let file;
      try {
        file = read(slot, bytes, null);
      } catch {
        return true;
      }
      const { record } = file;
      if (record.gen !== room.gen) return !(record.room === room.id && record.epoch > room.epoch);
      if (record.commit === room.base.commit) return Boolean(slot.bytes) && !sameTables(file, read(slot, slot.bytes, null));
      return !(record.room === room.id && record.parent === room.base.commit);
    },

    // The host's own commit of the room's state (its final save when the last tab leaves a dirty
    // room). Null when the file on disk already holds everything the room admitted. The host
    // writes `bytes` with If-Match on the ETag it holds and reports the write with fileAccepted.
    render(room) {
      const slot = slotOf(room);
      if (!slot || !open(slot) || !slot.bytes) return null;
      const file = read(slot, slot.bytes, slot.etag ?? null);
      if (file.record.gen !== room.gen || file.record.through >= room.seq) return null;
      const built = render(envOf(slot), file);
      return { bytes: built.body, record: built.record, ifMatch: slot.etag ?? null };
    },

    // The one host step for every write that did not come from the room: a version restore, an
    // agent's write, an upload of an older copy. `bytes` is the file the host wants to write and
    // `current` the file on disk. Answers `{ same: true, bytes }` when the tables are equal:
    // write those bytes and report them with fileAccepted (the incoming bytes when they carry the
    // current record, the current file otherwise, so the room is kept). Otherwise answers the bytes to
    // write (a new generation with a cut) and, when the room held unsaved work, a backup to keep
    // as a version first. With `keep` the room reopens on the new generation when fileAccepted
    // sees the write; without it the room is dropped now. The host holds the document's write
    // lock from this call until fileAccepted, so no frame lands between them.
    outsideWrite(key, { bytes, current, codec, keep = true, kind = null }) {
      let slot = slots.get(key);
      const kindName = slot?.codec ?? codec;
      if (!Object.hasOwn(codecs, kindName)) return null;
      const probe = slot ?? { codec: kindName };
      let source;
      let disk;
      try {
        source = read(probe, bytes, null);
        disk = read(probe, current, null);
      } catch {
        return null;
      }
      const reopen = keep && this.mayFit(key, bytes);
      const made = !slot && reopen;
      if (made) {
        slot = { codec: kindName, room: null, bytes: current, docBytes: utf8(current), copyBytes: 0, parts: new Map(), notices: [], idle: now() };
        slots.set(key, slot);
      }
      const env = slot ? envOf(slot) : { hostMade: true, limits, observer: silent, port: { room: () => null, setRoom: () => {}, next } };
      const replace = kind === 'restore' || kind === 'reset' || kind === 'upload';
      const step = outsideStep(env, source, disk, reopen, replace);
      if (step.same) {
        if (made) forget(key);
        // Equal tables: nothing changes for the room. The other bytes (a title) are taken only when
        // the incoming file carries the current record; an older record over equal tables (a
        // version restored twice) would end the room without a backup, so the file stays as it is.
        const kept = source.record.gen === disk.record.gen && source.record.commit === disk.record.commit;
        return { same: true, bytes: kept ? bytes : current };
      }
      const wasDirty = step.dirty;
      if (!reopen && slot) {
        forget(key);
        wake();
      }
      return { same: false, bytes: step.body, record: step.record, backup: step.backup?.body ?? null, dirty: wasDirty, kept: reopen };
    },

    leave(room, conn) {
      const slot = byConn.get(conn);
      if (!slot) return { empty: true, dirty: false, send: [] };
      byConn.delete(conn);
      const clientId = slot.conns.get(conn);
      slot.conns.delete(conn);
      if (![...slot.conns.values()].includes(clientId) && slot.room?.gen) {
        const before = roomBytes(slot.room);
        dropConnection(slot.room, clientId);
        for (const stage of slot.parts.keys()) if (stage.startsWith(`${clientId}/`)) slot.parts.delete(stage);
        if (roomBytes(slot.room) < before) slot.notices.push({ to: '*', k: 'space' });
      }
      if (!slot.conns.size) slot.idle = now();
      const send = sent(slot, slot.room?.log.length ?? 0);
      wake();
      return { empty: !slot.conns.size, dirty: dirty(slot.room), send };
    },

    sweep(connectedOf = () => 0) {
      const dropped = [];
      for (const [key, slot] of [...slots]) {
        if (slot.conns?.size || connectedOf(key) > 0) continue;
        if (dirty(slot.room)) continue;
        if (now() - slot.idle < retainMs) continue;
        forget(key);
        dropped.push(key);
      }
      if (dropped.length) wake();
      return dropped;
    },

    drop(key) {
      const slot = forget(key);
      if (slot) wake();
      return slot ? { dirty: dirty(slot.room) } : null;
    },

    wake,

    used,
    size: () => slots.size,
  };
}
