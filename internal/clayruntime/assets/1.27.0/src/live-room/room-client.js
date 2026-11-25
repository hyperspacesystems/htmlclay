// The ordered room client over the live-room core (src/live-room/core, generated from the lab).
// The core's tab expects a synchronous port; this client is the event loop around it. Before each
// step it reads the file and the room from the host, so the core's `read()` and `room()` answer
// what the host held just before the call, which is the lab's read at that instant. Sends go out
// as requests, and their answers come back through the core's own entry points (`deliver`,
// `answered`, `toTab`, `waited`). The transport is injected: `fetch` and `subscribe`.
import { newTab, edit as addEdit, flush as flushTab, post, answered, waited, toTab, sync, link, capture, deliver, view, attached, retry, restoreCopy, decide } from "./core/tab.js";
import { view as fileView, edit as fileEdit, save as fileSave, deliver as fileDeliver, refresh as fileRefresh, push as filePush } from "./core/fileonly.js";
import { parse } from "./core/record.js";
import { useCodec } from "./core/codec.js";
import { REAL_LIMITS } from "./core/limits.js";
import { copyStore } from "./copy-store.js";

const silent = new Proxy({}, { get: () => () => {} });
const SETTLED = new Set(["saved", "dropped", "recovered", "kept-refused"]);
const ADMITTED = new Set(["shared", "saved"]);
const LOOP_CAP = 100_000;

/**
 * @param {{ page: string, codec: object, clientId: string, fetch: typeof fetch,
 *   subscribe: (page: string, clientId: string, listener: (event: object) => void) => () => void,
 *   limits?: object, idleMs?: number, maxMs?: number, waitMs?: number, firstId?: number,
 *   copies?: boolean }} options
 * `copies` is whether the host announces `recovery-copies`.
 */
export function createRoomClient({ page, codec, clientId, fetch, subscribe, limits = REAL_LIMITS, idleMs = 2000, maxMs = 8000, waitMs = 10000, firstId = Math.floor(Math.random() * 2 ** 40), copies = true }) {
  const tab = newTab(clientId);
  const timers = new Set();
  let lastEstablish = -Infinity;
  let held = null;
  let flushing = false;
  const whenSaved = [];
  const answers = [];
  const handles = new Map();
  const listeners = new Set();
  const counters = {};
  const query = `page=${encodeURIComponent(page)}`;
  let file = null;
  let fileRaw = null;
  let mirror = null;
  let chain = Promise.resolve();
  let batch = null;
  let idleTimer = null;
  let maxTimer = null;
  let waitTimer = null;
  let closed = false;
  let failure = null;
  let unsubscribe = () => {};
  const restoring = new Set();
  const lostFrames = new Set();
  let spaceMarks = 0;
  let lastThrough = 0;
  // The store asks for a read when the host answered: the core's read retries unstored copies and
  // unwritten marks (`storeAgain`, `writeMarks`). It needs no new download of the page: the read
  // works on the file this tab holds.
  const store = copyStore({ codec, fetch, query, remote: copies, retryMs: waitMs, wake: () => { tab.wake = true; nudge(); } });

  const port = {
    read: () => file,
    room: () => mirror,
    setRoom: (room) => {
      mirror = room;
    },
    next: (counter) => (counters[counter] = (counters[counter] ?? firstId) + 1),
    tabs: () => [tab],
    verdict: (e) => store.verdict(e),
    marked: (e, kind) => store.marked(e, kind),
    keepDecision: (mark) => store.keepDecision(mark),
    fetchDecision: (mark) => store.fetchDecision(mark),
    keepRecovery: (copy) => store.keepRecovery(copy),
    fetchRecovery: (id) => store.fetchRecovery(id),
    readRecovery: (id) => store.readRecovery(id),
    removals: () => [],
    keepRemovals: () => {},
    send: (_, io) => send(io),
    post: (_, frame) => queue(frame),
    takeAnswer: (at) => answers.splice(at, 1)[0],
    scrambled: () => false,
    pick: () => 0,
    onFull: () => {},
    onSpace: () => {},
  };
  const env = { hostMade: true, removals: false, split: false, limits, observer: silent, port };

  // A step that throws (the host unreachable for a moment) is tried again after a pause: nothing
  // else would wake the tab, and its edits would stay unsaved for the rest of the page load.
  function pump(work) {
    chain = chain
      .then(() => (closed ? undefined : (useCodec(codec), work())))
      .then(
        () => {
          failure = null;
        },
        (error) => {
          failure = error;
          emit({ type: "error", error });
          armWait();
        },
      );
    return chain;
  }

  // The file first, then the room: a room read after the file is never older than it, and a room
  // newer than the file (a save landed between the reads) makes the core ask for the file again.
  async function refresh() {
    const got = await fetch(`/_/file?${query}`);
    if (!got.ok) throw new Error(`the host did not answer the file read (${got.status})`);
    const body = await got.text();
    file = { etag: got.headers.get("etag"), ...parse(body) };
    fileRaw = body;
    await readRoom();
    await store.load();
    failure = null;
  }

  // The room alone, for the answers to a batch of frames: when the room is ahead of the file this
  // tab holds, the core asks for the file and settle() reads it.
  async function readRoom() {
    const joined = await fetch(`/_/sync/room?${query}&client-id=${encodeURIComponent(clientId)}`);
    if (!joined.ok) throw new Error(`the host did not answer the room read (${joined.status})`);
    const room = await joined.json();
    if (!mirror || mirror.id !== room.id || mirror.gen !== room.gen) mirror = { id: room.id, gen: room.gen, base: room.base, log: [], floor: room.floor };
    else {
      if (room.base) mirror.base = room.base;
      mirror.floor = room.floor;
    }
    room.tail.forEach((frame, i) => {
      mirror.log[room.floor + i] ??= frame;
    });
  }

  // Everything the tab can do now, after the outside event that woke it.
  function drive() {
    if (closed) return;
    // An establish held back for want of an edit is moot once another tab opened a room: answer it
    // as refused, so the core reads the room and attaches.
    // A viewer whose file changed while no room was open (a coworker saved, then the room closed)
    // loads the new file: the core loads a file only into a tab with no sheet, and a tab with
    // nothing unsettled loses nothing by loading again.
    const stale = held && !mirror?.gen && file && file.etag !== held.etag && !editing();
    if (held && (mirror?.gen || stale)) {
      const io = held;
      held = null;
      if (stale) tab.state = null;
      answers.push({ tab, io, status: 412 });
      deliver(env, answers.length - 1, false);
    }
    if (!attached(env, tab) || tab.fetched || tab.wake) sync(env, tab);
    // Work that arrived after the establish was held (a copy the read just found stored, a
    // coworker's frames to carry) sends it.
    if (held && editing()) releaseHeld();
    for (let tries = 0; tab.fetched && tries < 8; tries++) link(env, tab);
    let turns = 0;
    while (toTab(env, tab)) if (++turns > LOOP_CAP) throw new Error("the room client stopped: the room log did not advance");
    if ((tab.base?.through ?? 0) > lastThrough) {
      lastThrough = tab.base.through;
      spaceMarks += 1;
    }
    // A copy this tab stored starts its restore job once the tab is attached (plan 09 section 3.2),
    // oldest first, each copy once per page load.
    if (!tab.job && attached(env, tab)) {
      const next = restorable();
      if (next && restoreCopy(env, tab, next.copy)) restoring.add(next.copy);
    }
    flushTab(env, tab);
    while (post(env, tab)) if (++turns > 2 * LOOP_CAP) throw new Error("the room client stopped: frames did not drain");
    report();
    plan();
    // The core starts a new generation when the ledger of who has edited is past its mark, from a
    // capture with nothing unsaved; without this a sheet opened often enough stops saving. A tab
    // rotates only once its own edits are settled (a request on the way would be moved to a
    // recovery copy), and only while attached, so a tab that only looks never opens a room for it.
    if (!editing() && attached(env, tab) && !tab.io && !tab.unknown && !tab.wake && !tab.fetched) {
      rotating = true;
      capture(env, tab);
      rotating = false;
    }
  }

  // A file older than the room leaves the tab asking for the file again: read again a few times,
  // then wait, so the tab never stays unattached until some later event.
  async function settle() {
    for (let round = 0; ; round++) {
      drive();
      if (closed || !tab.fetched) return;
      if (round === 2) return armWait();
      await refresh();
    }
  }

  const step = () =>
    pump(async () => {
      await refresh();
      await settle();
    });
  // A new edit acts on what the tab holds: the stream keeps the room's log current, and when the
  // core needs the file or the room again, settle() reads them. Reading the whole file for every
  // keystroke would make typing wait on page downloads.
  const nudge = () => pump(settle);

  // At most one establish per waitMs: a refused establish leads straight to another inside
  // `deliver`, and without the pause a host that cannot hold the room would get thousands a second.
  // A room starts with the first edit, not the first open (plan 09 section 2.3): a tab with no
  // unsettled edit holds its establish until it has one, so a person who only looks changes nothing.
  // A tab holding admitted actions the file lacks (a coworker's, whose tab is gone) is editing too:
  // it must open a room again to save them.
  // A copy of this tab's own edits that is stored and not yet restored is work to do too: the tab
  // opens a room for its restore job.
  const restorable = () => tab.notices.find((one) => one.kind === "recovery" && !restoring.has(one.copy) && store.fetchRecovery(one.copy));
  const editing = () => Boolean(tab.job) || Boolean(restorable()) || holding() || [...tab.edits.values()].some((one) => !SETTLED.has(one.state));
  let rotating = false;
  function send(io) {
    const establish = Boolean(io.ask);
    if (establish && !rotating && !editing()) {
      held = io;
      return;
    }
    const path = establish ? `/_/sync/room?${query}&op=establish` : `/_/save?${query}`;
    const go = () => {
      timers.delete(timer);
      if (closed) return;
      const body = establish ? undefined : typeof io.body === "function" ? io.body() : io.body;
      fetch(path, { method: "POST", headers: { "if-match": io.etag }, body }).then(
        // Only a 4xx proves the host wrote nothing: a 5xx can come after the file was written
        // (hyperclay's 503 save-unknown, a proxy timeout), so it is an answer this tab never got.
        (answer) => (answer.status >= 500 ? land(io, 0, true) : land(io, answer.status, false)),
        () => land(io, 0, true),
      );
    };
    const pause = establish ? Math.max(0, lastEstablish + waitMs - Date.now()) : 0;
    if (establish) lastEstablish = Date.now() + pause;
    const timer = pause ? setTimeout(go, pause) : null;
    if (timer) timers.add(timer);
    else go();
  }

  // An establish held back for want of an edit goes out as soon as one arrives, or a restore starts.
  function releaseHeld() {
    if (!held) return;
    const io = held;
    held = null;
    send(io);
  }

  // The answer reaches the core even when the read after it fails (the network that lost the
  // answer is still down): `deliver` frees the tab's one request. An answer this tab never got is
  // settled by the next read of the file, so the tab reads at once when it can.
  function land(io, status, lost) {
    pump(async () => {
      const read = await refresh().then(
        () => true,
        () => false,
      );
      answers.push({ tab, io, status });
      deliver(env, answers.length - 1, lost || !read);
      if (lost && read) tab.wake = true;
      await settle();
    });
  }

  function queue(frame) {
    if (batch) return batch.push(frame);
    batch = [frame];
    queueMicrotask(() => {
      const frames = batch;
      batch = null;
      if (closed) return;
      const sentSpace = spaceMarks;
      const sentThrough = tab.base?.through ?? 0;
      fetch(`/_/sync/room?${query}&client-id=${encodeURIComponent(clientId)}`, { method: "POST", body: JSON.stringify({ frames }) })
        .then((answer) => (answer.ok ? answer.json() : Promise.reject(new Error(`frames answered ${answer.status}`))))
        .then(
          ({ answers: results }) =>
            pump(async () => {
              await readRoom();
              frames.forEach((one, i) => {
                const res = results[i];
                // An explicit "stale" answer on a request whose wire was never lost proves the
                // old room rejected it before admission, so a retired client can safely rekey it
                // instead of stranding it as unjudged.
                if (res === "stale" && one.k === "req" && !lostFrames.has(one.e)) {
                  const req = tab.pending.find((entry) => entry.id === one.e);
                  if (req) req.sent = false;
                }
                answered(env, tab, one, res);
              });
              // If a receipt or space notice arrived while this POST was in flight and set
              // spaceMarks or base.through ahead, retry immediately instead of staying throttled
              // on a clean room.
              if (tab.throttled && (spaceMarks > sentSpace || (mirror?.base?.through ?? 0) > sentThrough || (tab.base?.through ?? 0) > sentThrough)) {
                retry(env, tab);
              }
              // Every frame refused for an old room, or for a room that forgot this tab (its stream
              // ended while the frames were on the way): read the room again after a pause, which
              // joins again and sends what is unanswered, instead of sending at once and flooding.
              // A "closed" answer means the host let go of this tab's connection (its stream
              // dropped): after the pause the tab reads again, which joins it again, and its
              // read sends everything unanswered once more.
              const rejoin = results.includes("closed");
              if (attached(env, tab) && (rejoin || results.every((result) => result === "stale"))) {
                report();
                clearTimeout(waitTimer);
                if (!closed)
                  waitTimer = setTimeout(() => {
                    if (rejoin) tab.wake = true;
                    step();
                  }, waitMs);
              } else await settle();
            }),
          () => {
            for (const one of frames) if (one.e) lostFrames.add(one.e);
            armWait();
          },
        );
    });
  }

  function armWait() {
    if (closed) return;
    clearTimeout(waitTimer);
    waitTimer = setTimeout(
      () =>
        pump(async () => {
          for (const one of tab.flight) if (one.e) lostFrames.add(one.e);
          await refresh();
          if (tab.unknown) tab.wake = true;
          waited(env, tab);
          if (tab.throttled && !unsaved()) retry(env, tab);
          await settle();
        }),
      waitMs,
    );
  }

  function hear(event) {
    pump(async () => {
      if (event.type === "room" && event.frame.to !== undefined) {
        if (event.frame.k === "full") tab.throttled = true;
        if (event.frame.k === "space") {
          spaceMarks += 1;
          retry(env, tab);
        }
      } else if (event.type === "room" && mirror && event.gen === mirror.gen && event.at >= 0 && (event.at <= mirror.floor || mirror.log[event.at - 1] !== undefined)) {
        // An event whose predecessor never arrived (the stream dropped one) goes to the room read
        // below instead, which fills the hole.
        mirror.log[event.at] = event.frame;
        if (event.frame.k === "receipt") {
          spaceMarks += 1;
          mirror.base = { commit: event.frame.commit, etag: event.frame.etag, through: event.frame.through };
        }
        // Another tab's copies and decision marks reach this one only through the host's list: a
        // save, and a restore job's frames, are when they may have changed. A returned edit is
        // judged by its mark, so the list is read before the frame is.
        if (event.frame.k === "receipt" || event.frame.k === "adopt" || event.frame.of === "adopt" || event.frame.copy) await store.load();
      } else {
        await refresh();
      }
      await settle();
    });
  }

  // The save loop: a save when edits pause for idleMs, and at the latest maxMs after the first
  // unsaved one. Every save is a conditional write, so a second writer is refused, never merged.
  const unsaved = () => Boolean(tab.state) && tab.a > (tab.base?.through ?? 0);
  // Admitted actions the file lacks, counting a coworker's frames this tab still has to carry into
  // a new room (`tab.held`, after the room was lost).
  const holding = () => unsaved() || tab.held.length > 0;
  const uploading = () => tab.unstored.some((draft) => !store.unstorable(draft.id));
  function plan() {
    if (!closed && tab.throttled && !unsaved() && !tab.io) armWait();
    if (closed || !unsaved()) {
      clearTimeout(idleTimer);
      clearTimeout(maxTimer);
      idleTimer = maxTimer = null;
      if (closed || (!tab.job && !tab.held.length && !uploading() && !tab.marks.length && !restorable() && ![...tab.edits.values()].some((one) => one.state === "pending"))) {
        flushing = false;
        whenSaved.splice(0).forEach((resolve) => resolve());
      }
      return;
    }
    clearTimeout(idleTimer);
    // A flush saves as soon as there is something admitted to save and no save on the way.
    idleTimer = setTimeout(save, flushing && !tab.io ? 0 : idleMs);
    maxTimer ??= setTimeout(save, maxMs);
  }
  function save() {
    clearTimeout(idleTimer);
    clearTimeout(maxTimer);
    idleTimer = maxTimer = null;
    pump(async () => {
      await refresh();
      capture(env, tab);
      await settle();
    });
  }

  function report() {
    for (const [id, one] of tab.edits) {
      const handle = handles.get(id);
      if (!handle || handle.state === one.state) continue;
      handle.state = one.state;
      if (ADMITTED.has(one.state) || SETTLED.has(one.state)) handle.admit(one.state);
      if (SETTLED.has(one.state)) handle.settle(one.state);
    }
    emit({ type: "change" });
  }

  function emit(event) {
    for (const listener of listeners) listener(event);
  }

  const ready = pump(async () => {
    unsubscribe = subscribe(page, clientId, hear);
    await refresh();
    await settle();
  });

  return {
    ready,
    get status() {
      const states = [...tab.edits.values()].map((one) => one.state);
      return {
        mode: "ordered",
        connected: !closed,
        pending: states.filter((state) => state === "pending").length,
        // A coworker's admitted actions this tab holds count too: closing the tab would lose them.
        unsaved: Math.max(states.filter((state) => state === "shared").length, unsaved() ? tab.a - tab.base.through : 0, tab.held.reduce((sum, one) => sum + one.frames.length, 0)),
        unknownSave: Boolean(tab.unknown),
        generation: tab.gen,
        through: tab.base?.through ?? null,
        copies: store.copies().length,
        unstored: tab.unstored.length,
        error: failure,
      };
    },
    // The action is recorded with what the author saw, before any other task can change the view.
    submit(action) {
      if (!tab.state) throw new Error("clay.liveRoom: wait for room.ready before submitting an edit");
      const id = addEdit(env, tab, action, view(tab));
      if (!id) return null;
      const handle = { state: "pending" };
      handle.admitted = new Promise((resolve) => (handle.admit = resolve));
      handle.durable = new Promise((resolve) => (handle.settle = resolve));
      handles.set(id, handle);
      releaseHeld();
      nudge();
      return { id, admitted: handle.admitted, durable: handle.durable };
    },
    // Waits for the edits that exist now and for the file to hold everything this tab has seen
    // admitted (a coworker's edit included), and says what happened to each of this tab's edits.
    // After close, an edit not settled resolves with its last state ("pending" or "shared").
    async flush() {
      const ids = [...handles.keys()];
      flushing = true;
      const saved = new Promise((resolve) => whenSaved.push(resolve));
      save();
      const states = await Promise.all(ids.map((id) => handles.get(id).durable));
      await saved;
      return Object.fromEntries(ids.map((id, i) => [id, states[i]]));
    },
    view: () => view(tab),
    committed: () => fileRaw,
    notices: () => tab.notices,
    copies: () => store.copies(),
    // A person's restore of a stored copy (another tab's included), and their decisions on a finished
    // job's conflicts: picks are [[e, "keep" | "take"], ...] for edits that job listed as conflicts.
    // Both read the host's copies and marks first: another tab may have stored the copy or decided
    // some of its edits since this tab last looked. Each resolves true when the job started.
    async restore(copyId) {
      let started = false;
      await pump(async () => {
        await store.load();
        started = restoreCopy(env, tab, copyId);
        if (started) restoring.add(copyId);
      });
      if (started) {
        releaseHeld();
        nudge();
      }
      return started;
    },
    async decide(jobId, picks) {
      let started = false;
      await pump(async () => {
        await store.load();
        started = decide(env, tab, jobId, picks);
      });
      if (started) {
        releaseHeld();
        nudge();
      }
      return started;
    },
    subscribe(listener) {
      listeners.add(listener);
      return () => listeners.delete(listener);
    },
    async close() {
      closed = true;
      store.close();
      clearTimeout(idleTimer);
      clearTimeout(maxTimer);
      clearTimeout(waitTimer);
      for (const timer of timers) clearTimeout(timer);
      await chain;
      await unsubscribe();
      for (const handle of handles.values()) {
        handle.admit?.(handle.state);
        handle.settle(handle.state);
      }
      whenSaved.splice(0).forEach((resolve) => resolve());
    },
  };
}

// File-only mode (a host that announces `ordered-file`): no room, every save a conditional write
// of the whole file, over the generated file-only core.
export function createFileClient({ page, codec, clientId, fetch, subscribe, limits = REAL_LIMITS, idleMs = 2000, maxMs = 8000, firstId = Math.floor(Math.random() * 2 ** 40), copies = true }) {
  const tab = { id: clientId, alive: true, client: clientId, clients: new Set([clientId]), born: null, n: 0, base: null, pending: [], io: null, unknown: null, unstored: [], judged: new Map(), edits: new Map(), notices: [] };
  const answers = [];
  const pushes = [];
  const handles = new Map();
  const listeners = new Set();
  const counters = {};
  const query = `page=${encodeURIComponent(page)}`;
  let file = null;
  let fileRaw = null;
  let chain = Promise.resolve();
  let idleTimer = null;
  let maxTimer = null;
  let flushing = false;
  let retryTimer = null;
  let closed = false;
  let failure = null;
  let unsubscribe = () => {};
  // File-only mode keeps copies for a person to read and copy (plan 09 section 3.7): no restore job.
  const store = copyStore({ codec, fetch, query, remote: copies, retryMs: idleMs * 5, wake: () => pump(async () => { fileRefresh(env, tab); after(); }) });

  const port = {
    read: () => file,
    etag: () => file?.etag ?? null,
    next: (counter) => (counters[counter] = (counters[counter] ?? firstId) + 1),
    keepRecovery: (copy) => store.keepRecovery(copy),
    fetchRecovery: (id) => store.fetchRecovery(id),
    send: (_, io) => send(io),
    takeAnswer: (at) => answers.splice(at, 1)[0],
    takePush: () => pushes.shift() ?? null,
  };
  const env = { limits, observer: silent, port };

  // A step that throws (the host unreachable for a moment) is tried again after a pause.
  function pump(work) {
    chain = chain
      .then(() => (closed ? undefined : (useCodec(codec), work())))
      .then(
        () => {
          failure = null;
        },
        (error) => {
          failure = error;
          emit({ type: "error", error });
          if (closed) return;
          clearTimeout(retryTimer);
          retryTimer = setTimeout(
            () =>
              pump(async () => {
                await readFile();
                fileRefresh(env, tab);
                after();
              }),
            idleMs * 5,
          );
        },
      );
    return chain;
  }

  async function readFile() {
    const got = await fetch(`/_/file?${query}`);
    if (!got.ok) throw new Error(`the host did not answer the file read (${got.status})`);
    const body = await got.text();
    file = { etag: got.headers.get("etag"), ...parse(body) };
    fileRaw = body;
    failure = null;
    return { etag: file.etag, body };
  }

  // A 200 whose body names no saved version (cut short, empty, not the host's answer) is an answer
  // this tab never got, like a 5xx: the next read of the file says what happened.
  function send(io) {
    fetch(`/_/save?${query}`, { method: "POST", headers: { "if-match": io.etag }, body: io.body }).then(
      async (answer) => {
        if (answer.status >= 500) return land(io, 0, null, true);
        if (!answer.ok) return land(io, answer.status, null, false);
        const etag = await answer.json().then(
          (data) => (typeof data?.etag === "string" ? data.etag : null),
          () => null,
        );
        return etag ? land(io, answer.status, etag, false) : land(io, 0, null, true);
      },
      () => land(io, 0, null, true),
    );
  }

  // The answer reaches the core even when the read after it fails, which frees the tab's request.
  function land(io, status, etag, lost) {
    pump(async () => {
      const read = await readFile().then(
        () => true,
        () => false,
      );
      answers.push({ tab, io, status, etag });
      fileDeliver(env, answers.length - 1, lost || !read);
      after();
    });
  }

  function hear(event) {
    if (event.type !== "file") return;
    pump(async () => {
      const { etag, body } = await readFile();
      await store.load();
      pushes.push({ tab, etag, body });
      filePush(env, tab);
      after();
    });
  }

  // The file-only core holds every save until each recovery copy is stored. A copy the host will
  // never hold stays in this tab (counted in status.unstored) and stops holding the saves.
  const kept = [];
  function keepUnstorable() {
    for (const draft of tab.unstored.filter((one) => store.unstorable(one.id))) {
      kept.push(draft);
      tab.unstored = tab.unstored.filter((one) => one !== draft);
      // Its edits are held only in this tab: flush() says so ("unstored") instead of waiting forever.
      for (const id of draft.fresh ?? draft.edits?.map((entry) => entry.e) ?? []) {
        const one = tab.edits.get(id);
        if (one) one.state = "unstored";
        const handle = handles.get(id);
        if (!handle) continue;
        handle.state = "unstored";
        handle.settle("unstored");
      }
    }
  }
  function after() {
    keepUnstorable();
    report();
    if (closed || !tab.pending.length) {
      clearTimeout(idleTimer);
      clearTimeout(maxTimer);
      idleTimer = maxTimer = null;
      flushing = false;
      return;
    }
    clearTimeout(idleTimer);
    // A flush (a host about to quit gives it two seconds) saves again at once after a refusal.
    idleTimer = setTimeout(save, flushing && !tab.io ? 0 : idleMs);
    maxTimer ??= setTimeout(save, maxMs);
  }

  function save() {
    clearTimeout(idleTimer);
    clearTimeout(maxTimer);
    idleTimer = maxTimer = null;
    pump(async () => {
      await readFile();
      keepUnstorable();
      fileSave(env, tab);
      // A save the core held back (a copy it had to judge first) is tried again.
      after();
    });
  }

  function report() {
    for (const [id, one] of tab.edits) {
      const handle = handles.get(id);
      if (!handle || handle.state === one.state) continue;
      handle.state = one.state;
      if (SETTLED.has(one.state)) handle.settle(one.state);
    }
    emit({ type: "change" });
  }

  function emit(event) {
    for (const listener of listeners) listener(event);
  }

  const ready = pump(async () => {
    unsubscribe = subscribe(page, clientId, hear);
    await readFile();
    await store.load();
    fileRefresh(env, tab);
  });

  return {
    ready,
    get status() {
      const states = [...tab.edits.values()].map((one) => one.state);
      return { mode: "file", connected: !closed, pending: states.filter((state) => state === "pending").length, unsaved: 0, unknownSave: Boolean(tab.unknown), generation: tab.base?.record.gen ?? null, through: null, copies: store.copies().length, unstored: tab.unstored.length + kept.length, error: failure };
    },
    submit(action) {
      if (!tab.base) throw new Error("clay.liveRoom: wait for room.ready before submitting an edit");
      const id = fileEdit(env, tab, action);
      const handle = { state: "pending" };
      handle.admitted = Promise.resolve("local");
      handle.durable = new Promise((resolve) => (handle.settle = resolve));
      handles.set(id, handle);
      after();
      return { id, admitted: handle.admitted, durable: handle.durable };
    },
    async flush() {
      const ids = [...handles.keys()];
      flushing = true;
      save();
      const states = await Promise.all(ids.map((id) => handles.get(id).durable));
      return Object.fromEntries(ids.map((id, i) => [id, states[i]]));
    },
    view: () => fileView(tab),
    committed: () => fileRaw,
    notices: () => tab.notices,
    copies: () => store.copies(),
    restore: async () => false,
    decide: async () => false,
    subscribe(listener) {
      listeners.add(listener);
      return () => listeners.delete(listener);
    },
    async close() {
      closed = true;
      store.close();
      clearTimeout(idleTimer);
      clearTimeout(maxTimer);
      clearTimeout(retryTimer);
      await chain;
      await unsubscribe();
      for (const handle of handles.values()) handle.settle(handle.state);
    },
  };
}

// What the host announces in its discovery answer (`GET /_/meta`: `{ spec, extensions }`).
async function announced(fetch) {
  const answer = await fetch("/_/meta");
  if (!answer.ok) return [];
  const meta = await answer.json().catch(() => null);
  return Array.isArray(meta?.extensions) ? meta.extensions : [];
}

// The mode is fixed for the page load: room mode on a host that announces `ordered-room`, file-only
// mode on one that announces `ordered-file`, and neither otherwise (the edits stay in the tab).
// `extensions` is the host's list when the caller already has it (a browser's host discovery).
export async function connect(options) {
  const extensions = options.extensions ?? (await announced(options.fetch));
  const copies = options.copies ?? extensions.includes("recovery-copies");
  if (extensions.includes("ordered-room")) return createRoomClient({ ...options, copies });
  if (extensions.includes("ordered-file")) return createFileClient({ ...options, copies });
  return null;
}
