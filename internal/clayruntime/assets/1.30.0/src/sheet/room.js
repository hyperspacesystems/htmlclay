import { findRegion, spliceRegion } from "../live-room/region.js";
import { savedPage } from "../live-room/browser.js";

const UNDO_LIMIT = 200;
const UNDO_MAX_BYTES = 64 * 1024 * 1024;
const UNDO_CAP_NOTICE = "Undo history is full, so your oldest steps were cleared. Recent changes still undo.";
const TOO_LARGE_NOTICE = "That change is too large to share.";
const TAB_ACTIONS = new Set(["insertSheet", "deleteSheet", "renameSheet", "moveSheet"]);

const isCodecMismatchError = (error) => {
  const msg = typeof error === "string" ? error : error?.message ?? "";
  return /sheet codec:/i.test(msg);
};

const isPermanentError = (error) => {
  const msg = typeof error === "string" ? error : error?.message ?? "";
  return /sheet codec:|no live room region/i.test(msg);
};

const gridRefused = (message) => {
  const err = new Error(message);
  err.name = "GridRefused";
  return err;
};

// A sheet in its document's live room (plan c10, "The shape"). Local edits apply at once through
// the Grid and go to the room; every room change reconciles the DOM to room.view(), which the
// client keeps equal to admitted, held and pending actions in the room's order. Only the room
// writes the sheet's bytes: `page` adds results and drawings when the tab is quiet.
export function createRoomBinding(
  book,
  {
    Sap = window.Sap,
    notice = () => {},
    onDrift = () => {},
    onLeave = () => {},
    reveal: revealTarget = null,
    setConflicts: paintConflicts = null,
    maxBytes = UNDO_MAX_BYTES,
    quietMs = 2000,
    win = window,
  } = {},
) {
  const codec = Sap.grid.roomCodec;
  let room = null;
  let ready = false;
  let peer = false;
  let unsubscribe = null;
  let unsavedHandle = null;
  let quietTimer = null;
  let capNoticed = false;
  let activeGroup = null;                // { tab, before, actions } while inside `group(tab, fn)`
  const queue = [];                      // edits made before the room was ready
  const stacks = new Map();              // tab id -> { done: [{ steps, bytes }], undone: [{ steps, bytes }], bytes }
  let committed = initialInner(book);    // the region inner bytes of the newest file this tab knows

  function hasUnsaved() {
    if (!room) return false;
    if (!ready && queue.length > 0) return true;
    const s = room.status;
    if (!s) return false;
    return s.pending > 0 || s.unsaved > 0 || s.unstored > 0 || Boolean(s.unknownSave) || (s.pendingMarks ?? 0) > 0;
  }

  const onBeforeUnload = (event) => {
    if (hasUnsaved()) {
      event.preventDefault();
      event.returnValue = "";
    }
  };

  const binding = {
    isReady: () => ready,
    committedInner,
    heard,
    page,
    dispatchFor,
    group,
    reconcile,
    undo,
    redo,
    canUndo: (tab) => (stacks.get(tab)?.done.length ?? 0) > 0,
    canRedo: (tab) => (stacks.get(tab)?.undone.length ?? 0) > 0,
    quietCheck,
    reveal: (target) => (typeof revealTarget === "function" ? revealTarget(target) : false),
    setConflicts: (targets) => paintConflicts?.(targets),
    start,
    stop,
  };
  return binding;

  function committedInner() {
    const fromRoom = room?.committed?.();
    if (typeof fromRoom === "string" && fromRoom && newer(fromRoom, committed)) {
      committed = fromRoom;
    }
    return committed;
  }

  // The newest file this tab has heard of: live sync delivers the room's saves like any other
  // page, and its region holds the drawings a save without the DOM must keep. Only a commit
  // record the room's or newer replaces the one held.
  function heard(incoming) {
    const index = [...document.querySelectorAll("[sheet-workbook]")].indexOf(book.root);
    const inner = servedInner(incoming, index);
    if (inner === null) return;
    if (!newer(inner, committed)) return;
    committed = inner;
  }

  function newer(inner, held) {
    const readRec = (bytes) => {
      if (!bytes) return null;
      try {
        return codec.read(bytes).record ?? null;
      } catch {
        return null;
      }
    };
    const next = readRec(inner);
    if (!next) return false;
    const was = readRec(held);
    if (!was) return true;
    if (next.v === 2 && was.v === 2) {
      const ne = Number(next.epoch ?? 0);
      const we = Number(was.epoch ?? 0);
      if (ne !== we) return ne > we;
      if (next.commit && was.commit && next.commit === was.commit) return true;
      const descends = (child, ancestor) =>
        Boolean(ancestor.commit) &&
        (child.parent === ancestor.commit || (Array.isArray(child.lineage) && child.lineage.includes(ancestor.commit)));
      if (descends(next, was)) return true;
      if (descends(was, next)) return false;
      if (next.gen === was.gen) return Number(next.through ?? 0) > Number(was.through ?? 0);
      const ng = genSeq(next.gen);
      const wg = genSeq(was.gen);
      if (Number.isFinite(ng) && Number.isFinite(wg) && ng !== wg) return ng > wg;
      return false;
    }
    if (next.v === 2) return true;
    const nr = Number(next.rev ?? NaN);
    const wr = Number(was.rev ?? NaN);
    if (!Number.isFinite(nr)) return false;
    return !Number.isFinite(wr) || nr >= wr;
  }

  // Submits one bound action (plain or batched) against `before`. When `collect` is provided (from
  // `flushGroup`), the guarded revert is appended there so one command stays one undo step; else
  // it is pushed onto the tab's undo stack directly.
  function sendAction(action, before, tabId, collect = null) {
    if (!ready) {
      queue.push({ action, tab: tabId });
      unsavedHandle?.changed();
      return true;
    }
    const probe = codec.apply(before, action);
    const handle = room.submit(action);
    if (!handle) {
      notice(TOO_LARGE_NOTICE);
      reconcile(room.view());
      throw gridRefused(TOO_LARGE_NOTICE);
    }
    unsavedHandle?.changed();
    const targetTab = tabId ?? action.sheet ?? (action.t === "batch" ? action.actions[0]?.sheet : null);
    if (probe?.inverse && targetTab && !TAB_ACTIONS.has(action.t)) {
      const stepItem = { revert: { ...probe.inverse, of: handle.id }, of: handle.id };
      handle.admitted?.then?.(() => {
        if (handle.inverse) {
          stepItem.revert = { ...handle.inverse, of: handle.id };
          recomputeStackBytes(targetTab);
        }
      });
      if (collect) {
        collect.push({ tab: targetTab, step: stepItem, handle });
      } else {
        push(targetTab, [stepItem]);
        handle.durable?.then?.((state) => {
          if (state === "refused" || state === "dropped") dropEntry(targetTab, handle.id);
        });
      }
    }
    return true;
  }

  // Flushes collected actions from `group(tab, fn)` into one room batch (keeping `rules` separate,
  // because sapjs forbids `rules` inside a `batch`) and one compound undo entry.
  function flushGroup(bucket) {
    if (bucket.actions.length === 0) return;
    const chunks = [];
    let pending = [];
    const pushPending = () => {
      if (pending.length === 0) return;
      chunks.push(pending.length === 1 ? pending[0] : { v: 1, t: "batch", actions: pending });
      pending = [];
    };
    for (const one of bucket.actions) {
      if (one.t === "rules" || TAB_ACTIONS.has(one.t)) {
        pushPending();
        chunks.push(one);
      } else if (one.t === "batch") {
        for (const inner of one.actions) {
          if (inner.t === "rules" || TAB_ACTIONS.has(inner.t)) {
            pushPending();
            chunks.push(inner);
          } else pending.push(inner);
        }
      } else {
        pending.push(one);
      }
    }
    pushPending();
    let base = bucket.before;
    const collected = [];
    try {
      for (const chunk of chunks) {
        sendAction(chunk, base, bucket.tab, collected);
        base = ready ? room.view() : null;
      }
    } finally {
      if (collected.length > 0) {
        const targetTab = bucket.tab ?? collected[0].tab;
        const steps = collected.map((one) => one.step).reverse();
        push(targetTab, steps);
        for (const one of collected) {
          one.handle.durable?.then?.((state) => {
            if (state === "refused" || state === "dropped") dropEntry(targetTab, one.step.of);
          });
        }
      }
    }
  }

  // Collects a multi-step UI command into one room batch and one guarded undo entry.
  function group(tab, fn) {
    if (!room || peer || activeGroup) return fn();
    const bucket = { tab, before: ready ? room.view() : null, actions: [] };
    activeGroup = bucket;
    let threw = false;
    try {
      return fn();
    } catch (error) {
      threw = true;
      throw error;
    } finally {
      activeGroup = null;
      if (threw) {
        if (bucket.actions.length > 0 && ready) reconcile(room.view());
      } else {
        flushGroup(bucket);
      }
    }
  }

  // Wraps one Grid's dispatch. A peer's reconcile never goes through it.
  function dispatchFor(grid, original) {
    return (unbound) => {
      if (peer || !room) return original(unbound);
      // Formulas carry refs bound in this tab's frame now, so a peer's insert admitted first
      // cannot change what they mean (plan c10, "The shape"). Applying it locally is unchanged.
      const action = Sap.grid.bound(book.sheets.workbook, unbound);
      if (activeGroup) {
        const event = original(action);                     // throws GridRefused as today
        activeGroup.actions.push(action);
        return event;
      }
      const before = ready ? room.view() : null;
      const event = original(action);                       // throws GridRefused as today
      sendAction(action, before, event?.sheet ?? action.sheet);
      return event;
    };
  }

  function entryBytes(steps) {
    return JSON.stringify(steps).length * 2;
  }

  function stackOf(tab) {
    let stack = stacks.get(tab);
    if (!stack) {
      stack = { done: [], undone: [], bytes: 0 };
      stacks.set(tab, stack);
    }
    return stack;
  }

  function trimStack(stack) {
    let droppedForCap = false;
    while (stack.done.length > UNDO_LIMIT) {
      const old = stack.done.shift();
      stack.bytes -= old.bytes;
    }
    while (stack.bytes > maxBytes && stack.done.length > 1) {
      const old = stack.done.shift();
      stack.bytes -= old.bytes;
      droppedForCap = true;
    }
    while (stack.bytes > maxBytes && stack.undone.length > 0) {
      const old = stack.undone.shift();
      stack.bytes -= old.bytes;
      droppedForCap = true;
    }
    if (droppedForCap && !capNoticed) {
      capNoticed = true;
      notice(UNDO_CAP_NOTICE);
    }
  }

  function push(tab, steps) {
    const stack = stackOf(tab);
    for (const item of stack.undone) stack.bytes -= item.bytes;
    stack.undone.length = 0;
    const item = { steps, bytes: entryBytes(steps) };
    stack.done.push(item);
    stack.bytes += item.bytes;
    trimStack(stack);
  }

  function recomputeStackBytes(tab) {
    const stack = stacks.get(tab);
    if (!stack) return;
    let total = 0;
    for (const item of stack.done) {
      item.bytes = entryBytes(item.steps);
      total += item.bytes;
    }
    for (const item of stack.undone) {
      item.bytes = entryBytes(item.steps);
      total += item.bytes;
    }
    stack.bytes = total;
  }

  function dropEntry(tab, id) {
    const stack = stacks.get(tab);
    if (!stack) return;
    for (const list of [stack.done, stack.undone]) {
      const at = list.findIndex((item) => item.steps.some((s) => s.of === id));
      if (at >= 0) {
        const [removed] = list.splice(at, 1);
        stack.bytes -= removed.bytes;
      }
    }
  }

  // Undo is the guarded revert: a target a coworker changed since is skipped, never overwritten.
  function undo(tab) {
    return step(tab, "done", "undone");
  }
  function redo(tab) {
    return step(tab, "undone", "done");
  }
  function step(tab, from, to) {
    const stack = stacks.get(tab);
    const popped = stack?.[from].pop();
    if (!popped) return { ok: false, why: "empty" };
    stack.bytes -= popped.bytes;
    const verb = from === "done" ? "undo" : "redo";
    const past = from === "done" ? "Undid" : "Redid";
    const initialView = room.view();
    const skippedAll = [];
    const nextSteps = [];
    const targets = [];
    let lastAction = null;
    let appliedAny = false;
    let tooLarge = false;

    for (const part of popped.steps) {
      const view = room.view();
      const revert = part.revert;
      lastAction = revert;
      const beforeBytes = tabSnapshot(codec, view, tab);
      const probeDoc = JSON.parse(JSON.stringify(view));
      const probe = codec.apply(probeDoc, revert);
      if (probe.skipped?.length) skippedAll.push(...probe.skipped);
      const changedTab =
        probe.out === "applied" &&
        probe.inverse &&
        probe.inverse.step !== null &&
        tabSnapshot(codec, probeDoc, tab) !== beforeBytes;
      if (!changedTab) continue;
      const handle = room.submit(revert);
      if (!handle) {
        tooLarge = true;
        break;
      }
      appliedAny = true;
      unsavedHandle?.changed();
      const stepTargets = codec.targets?.(probe.inverse, probeDoc) ?? codec.targets?.(revert, view) ?? [];
      for (const t of stepTargets) if (!targets.includes(t)) targets.push(t);
      const nextStep = { revert: { ...probe.inverse, of: handle.id }, of: handle.id };
      nextSteps.push(nextStep);
      handle.durable?.then?.((state) => {
        if (state === "refused" || state === "dropped") dropEntry(tab, handle.id);
      });
    }

    if (tooLarge && !appliedAny) {
      notice(TOO_LARGE_NOTICE);
      return { ok: false, why: "too_large" };
    }
    if (!appliedAny) {
      const who = labels(initialView, skippedAll, lastAction);
      notice(who ? `Can't ${verb}: ${who} changed after your edit.` : `Can't ${verb}: this sheet changed after your edit.`);
      return { ok: false, why: "changed", skipped: skippedAll };
    }
    const next = { steps: nextSteps.reverse(), bytes: 0 };
    next.bytes = entryBytes(next.steps);
    stack[to].push(next);
    stack.bytes += next.bytes;
    trimStack(stack);
    if (tooLarge) {
      notice(TOO_LARGE_NOTICE);
    } else if (skippedAll.length) {
      const who = labels(initialView, skippedAll, lastAction);
      notice(who ? `${past} your edit, leaving ${who} as changed since.` : `${past} the parts nobody else changed.`);
    }
    reconcile(room.view());
    return { ok: true, skipped: skippedAll, targets };
  }

  // Names for skipped targets, as the codec names them, else clean sheet/row/column names.
  function labels(view, targets = [], action = null) {
    let said = [];
    try {
      said = codec.describe?.(action, view)?.cells ?? [];
    } catch {
      said = [];
    }
    const named = new Map(said.map((one) => [one.target, one.label]));
    const out = [];
    const seen = new Set();
    for (const target of targets) {
      const label = named.get(target) ?? formatTargetLabel(view, target);
      if (!label || seen.has(label)) continue;
      seen.add(label);
      out.push(label);
    }
    return out.join(", ");
  }

  // The DOM becomes `doc`, writing only what differs, as a peer's change: no revision, no history.
  // With `dry`, nothing is written and the count of differences is returned.
  function reconcile(doc, { dry = false } = {}) {
    const made = book.sheets;
    if (!made || !doc) return 0;
    let differences = 0;
    const changed = { sheets: [], cells: [], rows: [], cols: [], table: [], sorted: [], structure: false };
    const tabChanges = [];
    const touched = () => {
      differences += 1;
      return !dry;
    };
    const whole = (id) => {
      changed.sheets.push(id);
      changed.structure = true;
    };
    const rule = (id) => changed.table.push({ sheet: id });
    peer = true;
    try {
      const workbook = made.workbook;
      const live = new Map(workbook.sheets().map((tab) => [tab.id, tab]));
      const wanted = new Set(doc.tabs.map((tab) => tab.id));
      for (const id of live.keys()) {
        if (wanted.has(id) || !touched()) continue;
        workbook.deleteSheet(id);
        whole(id);
        tabChanges.push(id);
      }
      for (const want of doc.tabs) {
        const here = live.get(want.id);
        if (!here) {
          if (!touched()) continue;
          workbook.insertSheet({ id: want.id, name: want.name, pos: want.pos, html: tableOf(codec, want) });
          whole(want.id);
          tabChanges.push(want.id);
          continue;
        }
        if (here.name !== want.name && touched()) {
          workbook.renameSheet(want.id, want.name);
          whole(want.id);
          tabChanges.push(want.id);
        }
        if (here.pos !== want.pos && touched()) {
          workbook.moveSheet(want.id, want.pos);
          changed.structure = true;
          tabChanges.push(want.id);
        }
        syncTab(want, here.backing, touched, changed, whole, rule);
      }
      if (!dry) {
        if (tabChanges.length > 0) book.syncTabs?.(tabChanges);
        if (changed.sheets.length > 0 || changed.cells.length > 0 || changed.rows.length > 0 || changed.cols.length > 0 || changed.table.length > 0 || changed.structure) {
          const results = made.engine.update(changed, { fromStep: false });
          made.engine.announce({ ...changed, results });
        }
      }
    } finally {
      peer = false;
    }
    return differences;
  }

  // One tab's records against the room's: columns, rows, the freeze and the cells. A dry pass
  // counts the tab as one difference and stops, since nothing it would write exists to read.
  function syncTab(want, backing, touched, changed, whole, rule) {
    const cols = backing.cols();
    const rows = backing.rows();
    const sheet = want.sheet;
    const fmtChangedCols = new Set();
    let needsFormatter = false;
    const checkFmt = (code) => {
      if (code && Sap.grid?.fmtKind?.(code) === "unknown") needsFormatter = true;
    };
    const structural =
      cols.length !== sheet.cols.length ||
      rows.length !== sheet.rows.length ||
      sheet.cols.some((col, i) => cols[i].id !== col.id) ||
      sheet.rows.some((row, i) => rows[i].id !== row.id) ||
      (backing.sheet.freezeCols ?? 0) !== (sheet.freezeCols ?? 0) ||
      bindingFrozen(rows) !== bindingFrozen(sheet.rows);
    if (structural) {
      if (!touched()) return;
      whole(want.id);
      syncRecords(want, backing, changed, fmtChangedCols, checkFmt);
    } else {
      sheet.cols.forEach((col, c) => {
        const here = cols[c];
        if (here.pos === col.pos && here.width === col.width && here.fmt === col.fmt && Boolean(here.hidden) === Boolean(col.hidden)) return;
        if (!touched()) return;
        if (here.fmt !== col.fmt) {
          fmtChangedCols.add(c);
          checkFmt(col.fmt);
        }
        backing.putCol(c, { pos: col.pos, width: col.width, fmt: col.fmt, hidden: col.hidden });
        changed.cols.push({ sheet: want.id, col: col.id, kind: "change" });
      });
      sheet.rows.forEach((row, r) => {
        const here = rows[r];
        if (here.pos === row.pos && here.style === row.style && Boolean(here.hidden) === Boolean(row.hidden)) return;
        if (!touched()) return;
        backing.putRow(r, { pos: row.pos, style: row.style, hidden: row.hidden });
        changed.rows.push({ sheet: want.id, row: row.id, kind: "change" });
      });
    }
    const widths = backing.cols();
    sheet.rows.forEach((row, r) => {
      const width = Math.max(row.cells.length, backing.rows()[r]?.cells.length ?? 0);
      for (let c = 0; c < width; c++) {
        const nextCell = row.cells[c] ?? null;
        if (sameContent(backing.cell(r, c), nextCell)) {
          if (fmtChangedCols.has(c) && nextCell?.f !== undefined && !nextCell.fmt && c < widths.length) {
            changed.cells.push({ sheet: want.id, row: row.id, col: widths[c].id });
          }
          continue;
        }
        if (!touched()) return;
        if (nextCell?.fmt) checkFmt(nextCell.fmt);
        backing.putCell(r, c, nextCell ? { ...nextCell } : null);
        if (c < widths.length) changed.cells.push({ sheet: want.id, row: row.id, col: widths[c].id });
      }
    });
    if (needsFormatter) backing.setStale?.(true);
    const here = JSON.stringify(backing.rules());
    const there = JSON.stringify(sheet.rules ?? []);
    if (here !== there && touched()) {
      backing.setRules(JSON.parse(JSON.stringify(sheet.rules ?? [])));
      rule(want.id);
    }
  }

  // Every column and row the room has and the page lacks, in the room's order, then the freeze.
  function syncRecords(want, backing, changed, fmtChangedCols, checkFmt) {
    const sheet = want.sheet;
    let reordered = false;
    const wanted = new Set(sheet.cols.map((col) => col.id));
    for (let i = backing.cols().length - 1; i >= 0; i--) {
      if (wanted.has(backing.cols()[i].id)) continue;
      changed.cols.push({ sheet: want.id, col: backing.cols()[i].id, kind: "delete" });
      backing.removeCol(i);
    }
    sheet.cols.forEach((col, at) => {
      const here = backing.colIndex(col.id);
      if (here === -1) {
        if (col.fmt) checkFmt?.(col.fmt);
        backing.insertCol(at, { ...col });
        changed.cols.push({ sheet: want.id, col: col.id, kind: "insert" });
        return;
      }
      if (here !== at) {
        backing.moveCol(here, at);
        changed.cols.push({ sheet: want.id, col: col.id, kind: "move" });
        reordered = true;
      }
      const now = backing.cols()[at];
      if (now.pos === col.pos && now.width === col.width && now.fmt === col.fmt && Boolean(now.hidden) === Boolean(col.hidden)) return;
      if (now.fmt !== col.fmt) {
        fmtChangedCols?.add(at);
        checkFmt?.(col.fmt);
      }
      backing.putCol(at, { pos: col.pos, width: col.width, fmt: col.fmt, hidden: col.hidden });
      changed.cols.push({ sheet: want.id, col: col.id, kind: "change" });
    });
    const kept = new Set(sheet.rows.map((row) => row.id));
    for (let i = backing.rows().length - 1; i >= 0; i--) {
      if (kept.has(backing.rows()[i].id)) continue;
      changed.rows.push({ sheet: want.id, row: backing.rows()[i].id, kind: "delete" });
      backing.removeRow(i);
    }
    sheet.rows.forEach((row, at) => {
      const here = backing.rowIndex(row.id);
      if (here === -1) {
        backing.insertRow(at, { id: row.id, pos: row.pos, style: row.style, hidden: row.hidden, frozen: row.frozen, cells: [] });
        changed.rows.push({ sheet: want.id, row: row.id, kind: "insert" });
        return;
      }
      if (here !== at) {
        backing.moveRow(here, at);
        changed.rows.push({ sheet: want.id, row: row.id, kind: "move" });
        reordered = true;
      }
      const now = backing.rows()[at];
      if (now.pos === row.pos && now.style === row.style && Boolean(now.hidden) === Boolean(row.hidden)) return;
      backing.putRow(at, { pos: row.pos, style: row.style, hidden: row.hidden });
      changed.rows.push({ sheet: want.id, row: row.id, kind: "change" });
    });
    if (reordered) changed.sorted.push(want.id);
    const frozen = { rows: bindingFrozen(sheet.rows), cols: sheet.freezeCols ?? 0 };
    const now = backing.frozen();
    if (now.rows !== frozen.rows || now.cols !== frozen.cols) {
      backing.setFrozen(frozen);
      changed.table.push({ sheet: want.id });
    }
    backing.tidy();
  }

  function scheduleQuietCheck() {
    if (quietTimer !== null || !ready || quietMs <= 0) return;
    quietTimer = setTimeout(() => {
      quietTimer = null;
      if (ready && room && room.status.pending === 0) quietCheck();
    }, quietMs);
  }

  // Quiet: nothing pending, and the DOM equals the view. A difference is repaired and reported.
  function quietCheck() {
    if (!ready || !room || room.status.pending !== 0) return false;
    const view = room.view();
    const differences = reconcile(view, { dry: true });
    if (differences === 0) return true;
    reconcile(view);
    console.warn(`clayjs: repaired ${differences} sheet view difference(s) against the live room`);
    onDrift(differences);
    book.root.dispatchEvent(new CustomEvent("sheet:view-drift", { bubbles: true, detail: { differences } }));
    return false;
  }

  // The room's save: the region it wrote, plus results and drawings while quiet, spliced into the
  // page as the host served it (never this tab's unsaved page). In file-only mode (`ordered-file`),
  // pending edits are already inside `doc`, so quietness checks whether the DOM matches `doc`.
  function page(inner, base) {
    const { doc, record } = codec.read(inner);
    const quiet =
      ready && room
        ? room.status?.mode === "file"
          ? reconcile(doc, { dry: true }) === 0
          : quietCheck()
        : false;
    const kept = Sap.grid.drawingsIn(committedInner());
    const extras = { drawing: (sheet, id) => (quiet ? figureOf(sheet, id) : null) ?? kept(sheet, id) };
    if (quiet) extras.result = (sheet, row, col) => resultOf(sheet, row, col);
    const out = codec.write(doc, record, extras);
    if (typeof room?.committed !== "function") committed = out;
    const served = savedPage(base);
    return spliceRegion(served, findRegion(served), out);
  }

  // { text, v, n, t } of the live cell, all four (the writer drops a missing n), or undefined.
  function resultOf(sheet, row, col) {
    const tab = book.sheets?.workbook.sheets().find((one) => one.id === sheet);
    if (!tab) return undefined;
    const backing = tab.backing;
    const r = backing.rowIndex(row);
    const c = backing.colIndex(col);
    if (r < 0 || c < 0) return undefined;
    const cell = backing.cell(r, c);
    if (!cell) return undefined;
    return { text: cell.text ?? "", v: cell.v, n: cell.n, t: cell.t };
  }

  // The figure's saved content for chart `id` on tab `sheet`, or null.
  function figureOf(sheet, id) {
    const section = [...book.root.children].find((el) => el.getAttribute?.("data-id") === sheet) ?? null;
    if (!section) return null;
    const figure = [...section.children].find((el) => el.localName === "figure" && el.getAttribute("chart") === id) ?? null;
    return figure ? figure.innerHTML : null;
  }

  function finishJoin() {
    if (ready || !room) return;
    ready = true;
    committedInner();
    for (const item of queue.splice(0)) {
      const before = room.view();
      try {
        sendAction(item.action, before, item.tab);
      } catch (error) {
        if (error?.name !== "GridRefused") throw error;
      }
    }
    reconcile(room.view());
    unsavedHandle?.changed();
  }

  function attachListener() {
    if (unsubscribe || !room) return;
    unsubscribe = room.subscribe((event) => {
      if (!room) return;
      const mode = room.status?.mode;
      const err = event.error ?? room.status?.error;
      if (mode === "unavailable" || mode === "closed" || (event.type === "error" && isPermanentError(err))) {
        const unsaved = hasUnsaved();
        const reason = event.type === "error" && isCodecMismatchError(err) ? "codec-mismatch" : (mode ?? "error");
        stop();
        onLeave(reason, { hasUnsaved: unsaved });
        return;
      }
      if (!ready && !room.status?.error && room.view()) {
        finishJoin();
        return;
      }
      if (ready && event.type === "change") {
        committedInner();
        const diff = reconcile(room.view());
        if (diff > 0) scheduleQuietCheck();
      }
      unsavedHandle?.changed();
    });
  }

  async function start(liveRoom) {
    room = liveRoom;
    book.bind(binding);
    win?.addEventListener?.("beforeunload", onBeforeUnload);
    unsavedHandle =
      win?.clay?.registerUnsavedState?.({
        id: "sheet:room",
        isPending: hasUnsaved,
        flush: () => (room?.flush ? room.flush().then(() => ({ ok: !hasUnsaved() })) : { ok: !hasUnsaved() }),
      }) ?? null;
    await room.ready;
    if (!room) return;
    const mode = room.status?.mode;
    const err = room.status?.error;
    if (mode === "unavailable" || mode === "closed" || isPermanentError(err)) {
      const unsaved = hasUnsaved();
      const reason = isCodecMismatchError(err) ? "codec-mismatch" : (mode ?? "unavailable");
      stop();
      onLeave(reason, { hasUnsaved: unsaved });
      return;
    }
    attachListener();
    if (!err && room.view()) {
      finishJoin();
    }
  }

  function stop() {
    if (quietTimer !== null) {
      clearTimeout(quietTimer);
      quietTimer = null;
    }
    unsubscribe?.();
    unsubscribe = null;
    unsavedHandle?.dispose?.();
    unsavedHandle = null;
    win?.removeEventListener?.("beforeunload", onBeforeUnload);
    const closing = room;
    const queuedTabs = !ready && queue.length > 0 ? [...new Set(queue.map((one) => one.tab).filter(Boolean))] : [];
    const hadQueued = !ready && queue.length > 0;
    queue.length = 0;
    room = null;
    ready = false;
    activeGroup = null;
    book.unbind(hadQueued ? { edited: queuedTabs.length > 0 ? queuedTabs : [""] } : {});
    closing?.close?.();
  }
}

// Numeric generation suffix from `"<epoch>.<seq>"`.
function genSeq(gen) {
  const s = String(gen ?? "");
  const dot = s.indexOf(".");
  return dot >= 0 ? Number(s.slice(dot + 1)) : NaN;
}

function tabSnapshot(codec, doc, tabId) {
  const tab = doc?.tabs?.find((one) => one.id === tabId);
  return tab ? codec.write({ tabs: [tab] }, {}) : "";
}

function colLetters(index) {
  let n = index + 1;
  let out = "";
  while (n > 0) {
    const r = (n - 1) % 26;
    out = String.fromCharCode(65 + r) + out;
    n = Math.floor((n - 1) / 26);
  }
  return out;
}

function formatTargetLabel(view, target) {
  if (typeof target !== "string" || !target) return "";
  const hash = target.indexOf("#");
  const base = hash >= 0 ? target.slice(0, hash) : target;
  const parts = base.split("/").map((p) => {
    try {
      return decodeURIComponent(p);
    } catch {
      return p;
    }
  });
  const tabId = parts[0] || "";
  const rowId = parts[1] || "";
  const colId = parts[2] || "";
  const tab = view?.tabs?.find((one) => one.id === tabId);
  const name = tab?.name || tabId;
  if (!tab) return name;
  const r = rowId ? tab.sheet.rows.findIndex((row) => row.id === rowId) : -1;
  const c = colId ? tab.sheet.cols.findIndex((col) => col.id === colId) : -1;
  if (r >= 0 && c >= 0) return `${name}!${colLetters(c)}${r + 1}`;
  if (r >= 0) return `${name} row ${r + 1}`;
  if (c >= 0) return `${name} column ${colLetters(c)}`;
  return name;
}

// The region inner bytes a page frame carries: its own bytes when live sync hands them over (a
// node's serialization writes a bare attribute as `name=""`, which the codec does not read back),
// else the parsed copy's workbook element with empty boolean attributes normalized for the codec.
function servedInner(incoming, index) {
  if (typeof incoming.html === "string") {
    let offset = 0;
    for (let i = 0; i <= index; i++) {
      const slice = incoming.html.slice(offset);
      const region = findRegion(slice);
      if (!region) break;
      if (i === index) return region.inner;
      offset += region.end;
    }
  }
  const root = incoming.document?.querySelectorAll("[sheet-workbook]")[index] ?? null;
  if (!root) return null;
  return root.innerHTML
    .replace(/<(?:"[^"]*"|'[^']*'|[^>"'])+>/g, (tag) =>
      tag.replace(/\s(grid|item|sheet-commit|rules|stale|hidden|n|t|open|stacked)=""(?=[\s/>])/g, " $1"),
    )
    .replace(/&nbsp;/g, "\u00a0");
}

// A tab's `<table grid>` bytes, as a tab insert takes them.
function tableOf(codec, tab) {
  const bytes = codec.write({ tabs: [tab] }, {});
  return bytes.match(/<table grid[\s\S]*<\/table>/)?.[0] ?? "";
}

const bindingFrozen = (rows) => rows.filter((row) => row.frozen).length;

// Whether a live cell and a room cell hold the same sheet: a formula by its text and its cell
// fields (its result is the engine's), a plain content by everything the file keeps.
function sameContent(here, there) {
  if (here === null || there === null) {
    const blank = (cell) =>
      cell === null ||
      (cell.text === "" && cell.f === undefined && cell.fmt === undefined && cell.style === undefined && cell.colspan === undefined && cell.rowspan === undefined && !cell.hidden);
    return here === null ? blank(there) : blank(here);
  }
  if (here.fmt !== there.fmt || here.style !== there.style || here.colspan !== there.colspan || here.rowspan !== there.rowspan) return false;
  if (Boolean(here.hidden) !== Boolean(there.hidden)) return false;
  if (here.f !== undefined && there.f !== undefined) return here.f === there.f;
  return here.f === there.f && here.text === there.text && here.v === there.v && Boolean(here.n) === Boolean(there.n) && Boolean(here.t) === Boolean(there.t);
}

// The workbook's region inner bytes as the page holds them at load. The element's own serialization
// is not those bytes (a bare attribute comes back as `name=\"\"`, which the codec refuses), so the
// tabs are written once through the writer a recovery save uses.
function initialInner(book) {
  return book.built ? book.serialize({ purpose: "recovery" }) : "";
}
