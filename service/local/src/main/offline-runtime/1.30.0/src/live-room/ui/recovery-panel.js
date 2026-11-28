import {
  RUNTIME_ONLY,
  bevelBox,
  bevelButton,
  bevelIconButton,
  bevelSurface,
  bevelText,
  bevelWell,
  setShown,
} from "../../ui/bevel-controls.js";
import { FONT_MONO, FONT_SANS, GLYPHS, RULES, SHADOW, TOKENS } from "../../ui/bevel.js";
import { resolveTheme } from "../../ui/theme.js";
import { mountEditorUI } from "../../ui/editor-ui.js";
import { set, style } from "../../lib/hostile-css.js";
import { markParts, partRules, themed } from "../../ui/theme-parts.js";

const CLOSING_MARKS = new Set(["taken", "saved", "kept", "misfit"]);
const MAX_INLINE_CELLS = 24;
const MAX_KEEP_REST_STEPS = 5000;
const plural = (n, singular, pluralWord = `${singular}s`) => `${n} ${n === 1 ? singular : pluralWord}`;

function copySortTuple(copy) {
  if (typeof copy?.time === "number" && Number.isFinite(copy.time)) return [0, copy.time, 0];
  if (typeof copy?.at === "number" && Number.isFinite(copy.at)) return [0, copy.at, 0];
  if (typeof copy?.at === "string") {
    const parsed = Date.parse(copy.at);
    if (Number.isFinite(parsed)) return [0, parsed, 0];
  }
  if (Array.isArray(copy?.at)) {
    const genEpoch = Number.parseInt(String(copy.at[0] ?? "").split(".")[0], 10);
    const through = typeof copy.at[1] === "number" ? copy.at[1] : 0;
    if (Number.isFinite(genEpoch)) return [1, genEpoch, through];
  }
  if (typeof copy?.epoch === "number" && Number.isFinite(copy.epoch)) return [1, copy.epoch, 0];
  return null;
}

function compareCopiesOldestFirst(a, b) {
  const ta = copySortTuple(a);
  const tb = copySortTuple(b);
  if (!ta || !tb || ta[0] !== tb[0]) return 0;
  if (ta[1] !== tb[1]) return ta[1] - tb[1];
  if (a?.at?.[0] && a?.at?.[0] === b?.at?.[0] && ta[2] !== tb[2]) return ta[2] - tb[2];
  return 0;
}

function colLetters(n) {
  let out = "";
  for (n += 1; n > 0; n = Math.floor((n - 1) / 26)) out = "ABCDEFGHIJKLMNOPQRSTUVWXYZ"[(n - 1) % 26] + out;
  return out;
}

function formatTargetValue(doc, target, value) {
  if (typeof target === "string" && doc?.tabs) {
    const hash = target.indexOf("#");
    const extra = hash >= 0 ? target.slice(hash + 1) : "";
    if (extra === "freezeRows" || extra === "freezeCols") {
      if (value === null || value === undefined || value === "") return "None";
      const tabId = decodeURIComponent((hash >= 0 ? target.slice(0, hash) : target).split("/")[0] ?? "");
      const tab = doc.tabs.find((one) => one.id === tabId);
      if (tab && extra === "freezeRows") {
        const idx = tab.sheet.rows.findIndex((r) => r.id === value);
        if (idx >= 0) return `Row ${idx + 1}`;
        return "Deleted row";
      }
      if (tab && extra === "freezeCols") {
        const idx = tab.sheet.cols.findIndex((c) => c.id === value);
        if (idx >= 0) return `Column ${colLetters(idx)}`;
        return "Deleted column";
      }
    }
  }
  return formatValue(value);
}

function formatFallbackTarget(doc, target) {
  if (typeof target !== "string" || !target) return "";
  if (!doc?.tabs) return target;
  const hash = target.indexOf("#");
  const base = hash >= 0 ? target.slice(0, hash) : target;
  const parts = base.split("/").map((p) => {
    try {
      return decodeURIComponent(p);
    } catch {
      return p;
    }
  });
  const [tabId, rowId, colId] = parts;
  const tab = doc.tabs.find((one) => one.id === tabId);
  if (!tab) return tabId || target;
  const r = rowId ? tab.sheet.rows.findIndex((one) => one.id === rowId) : -1;
  const c = colId ? tab.sheet.cols.findIndex((one) => one.id === colId) : -1;
  if (r >= 0 && c >= 0) return `${tab.name}!${colLetters(c)}${r + 1}`;
  if (r >= 0) return `${tab.name} · Row ${r + 1}`;
  if (c >= 0) return `${tab.name} · Column ${colLetters(c)}`;
  if (rowId && !colId) return `${tab.name} (removed row)`;
  if (!rowId && colId) return `${tab.name} (removed column)`;
  return tab.name;
}

function formatValue(value) {
  if (value === undefined || value === null || value === "") return "(empty)";
  if (typeof value === "string") return value;
  if (typeof value === "number" || typeof value === "boolean") return String(value);
  if (typeof value === "object" && typeof value.f === "string") {
    return value.v !== undefined && value.v !== "" ? `${value.f} (${value.v})` : value.f;
  }
  return JSON.stringify(value);
}

function rawTextValue(value) {
  if (value === undefined || value === null) return "";
  if (typeof value === "string") return value;
  if (typeof value === "number" || typeof value === "boolean") return String(value);
  if (typeof value === "object" && typeof value.f === "string") return value.f;
  return JSON.stringify(value);
}

function clientFromEditId(id) {
  const s = String(id ?? "");
  const sep = s.lastIndexOf(":");
  return (sep > 0 ? s.slice(0, sep) : s) || "editor";
}

async function writeClipboard(doc, text) {
  const nav = doc?.defaultView?.navigator ?? (typeof navigator !== "undefined" ? navigator : null);
  if (nav?.clipboard && typeof nav.clipboard.writeText === "function") {
    try {
      await nav.clipboard.writeText(text);
      return true;
    } catch {}
  }
  const targetDoc = doc ?? (typeof document !== "undefined" ? document : null);
  if (!targetDoc?.body) return false;
  const field = targetDoc.createElement("textarea");
  field.value = text;
  field.setAttribute("clay", RUNTIME_ONLY);
  set(field, "position", "fixed");
  set(field, "top", "0");
  set(field, "left", "0");
  set(field, "opacity", "0");
  set(field, "pointer-events", "none");
  targetDoc.body.append(field);
  field.select?.();
  let copied = false;
  try {
    copied = targetDoc.execCommand?.("copy") === true;
  } catch {}
  field.remove();
  return copied;
}

function setControlDisabled(btn, disabled) {
  const next = Boolean(disabled);
  btn.disabled = next;
  btn.setDisabled?.(next);
}

function isNarrowScreen(win) {
  if (win?.matchMedia?.("(max-width: 760px)")?.matches) return true;
  if (win?.matchMedia?.("(pointer: coarse)")?.matches) return true;
  const width = win?.innerWidth ?? 0;
  return width > 0 && width <= 760;
}

function describeEntry(codec, entry, doc, conflictCells = null) {
  const action = entry?.action;
  const raw = action && typeof codec?.describe === "function" ? codec.describe(action, doc) : null;
  const title = raw?.title || "Edit";
  const describedCells = Array.isArray(raw?.cells) ? raw.cells : [];

  if (describedCells.length > 0) {
    const digestSaw = codec?.name === "sheet";
    return {
      title,
      cells: describedCells.map((cell, index) => ({
        target: cell.target ?? null,
        label: cell.label ?? cell.target ?? `Item ${index + 1}`,
        stored: cell.stored,
        saw: cell.saw !== undefined ? cell.saw : (!digestSaw && Array.isArray(entry?.saw) ? entry.saw[index] : undefined),
      })),
    };
  }

  if (Array.isArray(conflictCells) && conflictCells.length > 0) {
    const mapped = conflictCells.map((item, index) => {
      if (Array.isArray(item)) {
        if (item.length === 2) {
          const target = String(item[0] ?? "");
          return { target: target || null, label: target || `Item ${index + 1}`, stored: "", saw: undefined };
        }
        const [first, second, , fourth] = item;
        const target = second ? `${first}/${second}` : String(first ?? "");
        const label = second ? `${first} ${second}` : String(first ?? `Item ${index + 1}`);
        const saw =
          entry?.saw && typeof entry.saw === "object" && !Array.isArray(entry.saw) && second != null
            ? entry.saw[second]
            : undefined;
        return { target: target || null, label, stored: fourth ?? "", saw };
      }
      if (item && typeof item === "object") {
        return {
          target: item.target ?? null,
          label: item.label ?? item.target ?? `Item ${index + 1}`,
          stored: item.stored ?? "",
          saw: undefined,
        };
      }
      const id = String(item);
      return { target: id, label: id, stored: "", saw: undefined };
    });
    return { title, cells: mapped };
  }

  const fallbackTarget = action
    ? (codec?.targets?.(action)?.[0] ??
      codec?.removes?.(action)?.[0] ??
      codec?.creates?.(action)?.[0] ??
      codec?.needs?.(action)?.[0] ??
      null)
    : null;

  if (entry?.saw && typeof entry.saw === "object" && !Array.isArray(entry.saw)) {
    const pairs = Object.entries(entry.saw);
    if (pairs.length > 0) {
      return {
        title,
        cells: pairs.map(([key, sawVal]) => ({
          target: fallbackTarget ? `${fallbackTarget}/${key}` : key,
          label: fallbackTarget ? `${formatFallbackTarget(doc, fallbackTarget)} ${key}` : key,
          stored: "",
          saw: sawVal,
        })),
      };
    }
  }

  return {
    title,
    cells: fallbackTarget ? [{ target: fallbackTarget, label: formatFallbackTarget(doc, fallbackTarget), stored: "", saw: undefined }] : [],
  };
}

function conflictTargetSet(conflict, descCells) {
  const out = new Set();
  if (Array.isArray(conflict?.cells)) {
    for (const item of conflict.cells) {
      if (typeof item === "string") out.add(item);
      else if (Array.isArray(item) && item.length >= 4 && item[0] != null && item[1] != null) {
        out.add(`${item[0]}/${item[1]}`);
      } else if (Array.isArray(item) && item[0] != null) {
        out.add(String(item[0]));
      } else if (item && typeof item === "object" && item.target) {
        out.add(String(item.target));
      }
    }
  }
  if (out.size === 0) {
    for (const cell of descCells) {
      if (cell.target) out.add(cell.target);
    }
  }
  return out;
}

function primaryTargetOf(codec, entry, descCells, conflict = null) {
  if (descCells[0]?.target) return descCells[0].target;
  const action = entry?.action;
  const fromCodec = action
    ? (codec?.targets?.(action)?.[0] ??
      codec?.removes?.(action)?.[0] ??
      codec?.needs?.(action)?.[0] ??
      codec?.creates?.(action)?.[0])
    : null;
  if (fromCodec) return fromCodec;
  const firstConflict = conflict?.cells?.[0];
  if (typeof firstConflict === "string") return firstConflict;
  if (Array.isArray(firstConflict) && firstConflict[0] != null) {
    return firstConflict.length >= 4 && firstConflict[1] != null ? `${firstConflict[0]}/${firstConflict[1]}` : String(firstConflict[0]);
  }
  return null;
}

function safeCurrent(codec, doc, target) {
  if (!doc || !target || typeof codec?.current !== "function") return undefined;
  try {
    return codec.current(doc, target);
  } catch {
    return undefined;
  }
}

function analyzeCopy(copy, allNotices, dismissedEdits, { codec = null, currentDoc = null, fileMode = false } = {}) {
  const edits = Array.isArray(copy.edits) ? copy.edits : [];
  const editById = new Map(edits.map((entry) => [entry.e, entry]));
  const marksByEdit = new Map();
  for (const mark of copy.marks ?? []) {
    if (!mark?.e) continue;
    const prev = marksByEdit.get(mark.e);
    if (!prev || CLOSING_MARKS.has(mark.kind)) marksByEdit.set(mark.e, mark.kind);
  }

  const copyNotices = allNotices.filter((one) => one?.kind === "restored" && one.copy === copy.id);
  const latestNotice = copyNotices.at(-1) ?? null;

  if (!latestNotice) {
    const refusedSet = new Set(copy.refused ?? []);
    const allMarkedClosed =
      edits.length > 0 &&
      edits.every((entry) => CLOSING_MARKS.has(marksByEdit.get(entry.e)) || refusedSet.has(entry.e));

    const counts = {
      restored: 0,
      kept: 0,
      alreadySaved: 0,
      superseded: 0,
      replaced: 0,
      refused: 0,
      conflicts: 0,
      waiting: 0,
      misfit: 0,
      unjudged: 0,
      fileCopy: 0,
    };
    const unsettleable = [];
    for (const entry of edits) {
      const mark = marksByEdit.get(entry.e);
      if (mark === "taken") counts.restored += 1;
      else if (mark === "saved") counts.alreadySaved += 1;
      else if (mark === "kept") counts.kept += 1;
      else if (mark === "misfit") {
        counts.misfit += 1;
        if (!dismissedEdits.has(`${copy.id}:${entry.e}`)) {
          unsettleable.push({ kind: "misfit", e: entry.e, entry });
        }
      } else if (refusedSet.has(entry.e) || entry.out === "refused" || mark === "refused") {
        counts.refused += 1;
        if (!dismissedEdits.has(`${copy.id}:${entry.e}`)) {
          unsettleable.push({ kind: "refused", e: entry.e, entry });
        }
      } else if (fileMode) {
        counts.fileCopy += 1;
        if (!dismissedEdits.has(`${copy.id}:${entry.e}`)) {
          unsettleable.push({ kind: "file-copy", e: entry.e, entry });
        }
      }
    }

    const needsRestore = !fileMode && !allMarkedClosed;
    const isSettled = !needsRestore && unsettleable.length === 0;
    return {
      copy,
      editById,
      latestNotice: null,
      needsRestore,
      counts,
      authorGroups: [],
      orphanHeld: [],
      unsettleable,
      isSettled,
    };
  }

  const everApplied = new Set();
  const everKept = new Set();
  const everMisfit = new Set();
  for (const notice of copyNotices) {
    for (const id of notice.applied ?? []) everApplied.add(id);
    for (const id of notice.kept ?? []) everKept.add(id);
    for (const id of notice.misfit ?? []) everMisfit.add(id);
  }
  for (const [id, kind] of marksByEdit) {
    if (kind === "taken") everApplied.add(id);
    else if (kind === "kept") everKept.add(id);
    else if (kind === "misfit") everMisfit.add(id);
  }

  const openConflictList = (latestNotice.conflicts ?? []).filter((item) => !CLOSING_MARKS.has(marksByEdit.get(item.e)));
  const openHeldList = (latestNotice.held ?? []).filter((item) => !CLOSING_MARKS.has(marksByEdit.get(item.e)));
  const conflictById = new Map(openConflictList.map((item) => [item.e, item]));
  const heldById = new Map(openHeldList.map((item) => [item.e, item]));
  const leftSet = new Set(latestNotice.left ?? []);
  const misfitSet = new Set(latestNotice.misfit ?? []);
  const refusedSet = new Set(latestNotice.refused ?? []);
  const replacedSet = new Set(latestNotice.replaced ?? []);
  const appliedSet = new Set(latestNotice.applied ?? []);
  const keptSet = new Set(latestNotice.kept ?? []);
  const savedSet = new Set(latestNotice.saved ?? []);
  const countedSet = new Set(latestNotice.counted ?? []);

  const allIds = [];
  const seenIds = new Set();
  const addId = (id) => {
    if (id && !seenIds.has(id)) {
      seenIds.add(id);
      allIds.push(id);
    }
  };
  for (const entry of edits) addId(entry.e);
  for (const item of latestNotice.conflicts ?? []) addId(item.e);
  for (const item of latestNotice.held ?? []) addId(item.e);
  for (const list of [
    latestNotice.applied,
    latestNotice.kept,
    latestNotice.saved,
    latestNotice.counted,
    latestNotice.replaced,
    latestNotice.misfit,
    latestNotice.refused,
    latestNotice.left,
  ]) {
    for (const id of list ?? []) addId(id);
  }

  const counts = {
    restored: 0,
    kept: 0,
    alreadySaved: 0,
    superseded: 0,
    replaced: 0,
    refused: 0,
    conflicts: 0,
    waiting: 0,
    misfit: 0,
    unjudged: 0,
    fileCopy: 0,
  };
  const unsettleable = [];

  for (const id of allIds) {
    const entry = editById.get(id) ?? { e: id, c: clientFromEditId(id), action: null, saw: null };
    const closedMark = marksByEdit.get(id);
    if (conflictById.has(id)) {
      counts.conflicts += 1;
    } else if (heldById.has(id)) {
      counts.waiting += 1;
    } else if (leftSet.has(id)) {
      counts.unjudged += 1;
      if (!dismissedEdits.has(`${copy.id}:${id}`)) {
        unsettleable.push({ kind: "unjudged", e: id, entry });
      }
    } else if (misfitSet.has(id) || closedMark === "misfit") {
      counts.misfit += 1;
      if (!dismissedEdits.has(`${copy.id}:${id}`)) {
        unsettleable.push({ kind: "misfit", e: id, entry });
      }
    } else if (refusedSet.has(id)) {
      counts.refused += 1;
      if (!dismissedEdits.has(`${copy.id}:${id}`)) {
        unsettleable.push({ kind: "refused", e: id, entry });
      }
    } else if (replacedSet.has(id)) {
      counts.replaced += 1;
    } else if (appliedSet.has(id) || closedMark === "taken") {
      counts.restored += 1;
    } else if (keptSet.has(id) || closedMark === "kept") {
      counts.kept += 1;
    } else if (savedSet.has(id) || closedMark === "saved") {
      if (everApplied.has(id)) counts.restored += 1;
      else if (everKept.has(id)) counts.kept += 1;
      else if (everMisfit.has(id)) {
        counts.misfit += 1;
        if (!dismissedEdits.has(`${copy.id}:${id}`)) {
          unsettleable.push({ kind: "misfit", e: id, entry });
        }
      } else {
        counts.alreadySaved += 1;
      }
    } else if (countedSet.has(id)) {
      const fitsCurrent =
        !currentDoc || !entry?.action || typeof codec?.fits !== "function"
          ? true
          : (() => {
              try {
                return codec.fits(currentDoc, entry.action);
              } catch {
                return true;
              }
            })();
      if (everApplied.has(id)) counts.restored += 1;
      else if (everKept.has(id)) counts.kept += 1;
      else if (everMisfit.has(id) || !fitsCurrent) {
        counts.misfit += 1;
        if (!dismissedEdits.has(`${copy.id}:${id}`)) {
          unsettleable.push({ kind: "misfit", e: id, entry });
        }
      } else {
        counts.superseded += 1;
      }
    } else if (entry.out === "refused") {
      counts.refused += 1;
      if (!dismissedEdits.has(`${copy.id}:${id}`)) {
        unsettleable.push({ kind: "refused", e: id, entry });
      }
    }
  }

  const authorOrder = [];
  const conflictsByAuthor = new Map();
  for (const conflict of openConflictList) {
    const entry = editById.get(conflict.e) ?? {
      e: conflict.e,
      c: clientFromEditId(conflict.e),
      action: null,
      saw: null,
    };
    const author = entry.c || "editor";
    if (!conflictsByAuthor.has(author)) {
      conflictsByAuthor.set(author, []);
      authorOrder.push(author);
    }
    conflictsByAuthor.get(author).push({ conflict, entry });
  }

  const rootConflictByEdit = new Map();
  for (const author of authorOrder) {
    const primary = conflictsByAuthor.get(author)[0];
    rootConflictByEdit.set(primary.conflict.e, author);
  }

  function resolveWaitingOwner(heldItem) {
    let cursor = heldItem.behind;
    const visited = new Set();
    while (cursor && !visited.has(cursor)) {
      visited.add(cursor);
      if (rootConflictByEdit.has(cursor)) return rootConflictByEdit.get(cursor);
      const conflictAuthor = authorOrder.find((a) =>
        conflictsByAuthor.get(a).some((item) => item.conflict.e === cursor),
      );
      if (conflictAuthor) return conflictAuthor;
      cursor = heldById.get(cursor)?.behind ?? null;
    }
    const entry = editById.get(heldItem.e);
    if (entry?.c && conflictsByAuthor.has(entry.c)) return entry.c;
    return null;
  }

  const waitingByAuthor = new Map(authorOrder.map((author) => [author, []]));
  const orphanHeld = [];
  for (const heldItem of openHeldList) {
    const entry = editById.get(heldItem.e) ?? {
      e: heldItem.e,
      c: clientFromEditId(heldItem.e),
      action: null,
      saw: null,
    };
    const owner = resolveWaitingOwner(heldItem);
    if (owner && waitingByAuthor.has(owner)) {
      waitingByAuthor.get(owner).push({ held: heldItem, entry });
    } else {
      orphanHeld.push({ held: heldItem, entry });
    }
  }

  const authorGroups = authorOrder.map((author) => {
    const list = conflictsByAuthor.get(author);
    const primary = list[0];
    const authorLabel = copy.names?.[author] || author;
    return {
      author,
      authorLabel,
      conflict: primary.conflict,
      entry: primary.entry,
      extraConflictsCount: Math.max(0, list.length - 1),
      waiting: waitingByAuthor.get(author) ?? [],
    };
  });

  const isSettled =
    counts.conflicts === 0 &&
    counts.waiting === 0 &&
    unsettleable.length === 0;

  return {
    copy,
    editById,
    latestNotice,
    needsRestore: false,
    counts,
    authorGroups,
    orphanHeld,
    unsettleable,
    isSettled,
  };
}

function outcomeBadgesFor(counts, needsRestore, totalEdits) {
  if (needsRestore && Object.values(counts).every((n) => n === 0)) {
    return [{ text: `${plural(totalEdits, "edit")} to check`, tone: "brass" }];
  }
  const items = [];
  if (counts.conflicts > 0) {
    items.push({
      text: `${counts.conflicts} ${counts.conflicts === 1 ? "needs" : "need"} your decision`,
      tone: "ox",
    });
  }
  if (counts.waiting > 0) {
    items.push({ text: `${counts.waiting} waiting`, tone: "brass" });
  }
  if (counts.unjudged > 0) {
    items.push({ text: `${counts.unjudged} cannot be judged`, tone: "ox" });
  }
  if (counts.misfit > 0) {
    items.push({
      text: `${counts.misfit} no longer ${counts.misfit === 1 ? "has" : "have"} a place`,
      tone: "brass",
    });
  }
  if (counts.refused > 0) {
    items.push({ text: `${counts.refused} refused when made`, tone: "brass" });
  }
  if (counts.fileCopy > 0) {
    items.push({ text: `${plural(counts.fileCopy, "edit")} to copy`, tone: "brass" });
  }
  if (counts.restored > 0) {
    items.push({ text: `${counts.restored} restored and saved`, tone: "teal" });
  }
  if (counts.kept > 0) {
    items.push({ text: `${counts.kept} kept current`, tone: "muted" });
  }
  if (counts.alreadySaved > 0) {
    items.push({ text: `${counts.alreadySaved} already saved`, tone: "teal" });
  }
  if (counts.superseded > 0) {
    items.push({ text: `${counts.superseded} replaced by later edit`, tone: "muted" });
  }
  if (counts.replaced > 0) {
    items.push({ text: `${counts.replaced} removed by version restore`, tone: "muted" });
  }
  return items;
}

export function createRecoveryPanel({
  room,
  codec,
  onReveal,
  onDownload,
  onConflictsHighlight,
  parent = null,
  theme = null,
} = {}) {
  const hostParent = parent ?? document.body;
  const doc = hostParent?.ownerDocument ?? document;
  const win = doc?.defaultView ?? (typeof window !== "undefined" ? window : null);
  const surfaceTheme = theme ?? resolveTheme();

  const drawer = bevelSurface(
    "aside",
    [
      "position:fixed",
      "top:0",
      "right:0",
      "bottom:0",
      "z-index:2147482500",
      "display:none",
      "flex-direction:column",
      "width:min(400px, 100vw)",
      "max-width:100vw",
      "height:100%",
      `color-scheme:${surfaceTheme.scheme}`,
      `background:${TOKENS.surface}`,
      `color:${TOKENS.ink}`,
      `border:1px solid ${TOKENS["line-2"]}`,
      `box-shadow:${SHADOW}`,
    ],
    { theme: surfaceTheme, parts: ["conflict.panel"] },
  );
  drawer.setAttribute("role", "dialog");
  drawer.setAttribute("aria-label", "Recovered edits");
  drawer.setAttribute("data-live-room-recovery", "");
  drawer.hidden = true;

  const header = bevelBox(
    "div",
    [
      "display:flex",
      "align-items:center",
      "justify-content:space-between",
      "gap:10px",
      "flex:none",
      "padding:14px 16px 12px",
      `background:${TOKENS.ground}`,
      `border-bottom:1px solid ${TOKENS["line-2"]}`,
    ],
    { theme: surfaceTheme, parts: ["conflict.header"] },
  );

  const heading = bevelText(
    "h2",
    [
      ...RULES.dialogHeading,
      "font-size:19px",
      "line-height:1.2",
      `color:${TOKENS.ink}`,
    ],
    "Recovered edits",
    { theme: surfaceTheme, parts: ["conflict.title"], role: "heading" },
  );

  const closeBtn = bevelIconButton(GLYPHS.dialogClose, {
    label: "Close recovered edits",
    onClick: () => close(),
    theme: surfaceTheme,
    parts: ["close", "conflict.close"],
  });
  closeBtn.setAttribute("data-focus-key", "close-drawer");
  header.append(heading, closeBtn);

  const body = bevelBox(
    "div",
    [
      "display:flex",
      "flex-direction:column",
      "gap:12px",
      "flex:1 1 auto",
      "min-height:0",
      "overflow-y:auto",
      "overflow-x:hidden",
      "padding:14px 16px 20px",
    ],
    { theme: surfaceTheme },
  );

  drawer.append(header, body);

  const mounted = mountEditorUI({
    name: "live-room-recovery",
    content: drawer,
    parent: hostParent,
    attributes: { "sap-ignore": "" },
  });
  mounted.host.hidden = true;

  let opened = false;
  let destroyed = false;
  let roomUnsub = null;
  let previousFocus = null;
  let busyAction = false;
  let keepRestState = null;
  let copiedKey = null;
  let copiedTimer = null;
  let lastHighlightSig = null;

  const initialCardValues = new Map();
  const lastCardValues = new Map();
  const changedCardTargets = new Set();
  const changedCards = new Set();
  const openDisclosures = new Set();
  const dismissedEdits = new Set();

  function focusablesInDrawer() {
    return [...drawer.querySelectorAll("button, summary, [tabindex]")].filter((node) => {
      if (node.disabled || node.getAttribute("aria-disabled") === "true") return false;
      if (node.hidden || node.closest("[hidden]")) return false;
      const closedDetails = node.closest("details:not([open])");
      if (closedDetails && node.parentElement !== closedDetails) return false;
      return true;
    });
  }

  function onKeydown(event) {
    if (!opened) return;
    if (event.key === "Escape") {
      event.preventDefault();
      event.stopPropagation();
      close();
      return;
    }
    if (event.key === "Tab") {
      const nodes = focusablesInDrawer();
      if (nodes.length === 0) return;
      const rootNode = drawer.getRootNode?.() ?? doc;
      const active = rootNode?.activeElement ?? doc.activeElement;
      const index = nodes.indexOf(active);
      if (index === -1) {
        event.preventDefault();
        nodes[0].focus();
      } else if (event.shiftKey && index === 0) {
        event.preventDefault();
        nodes[nodes.length - 1].focus();
      } else if (!event.shiftKey && index === nodes.length - 1) {
        event.preventDefault();
        nodes[0].focus();
      }
    }
  }

  function onResize() {
    if (opened) render();
  }

  function applyTouchTarget(btn, narrow) {
    if (narrow) btn.pin?.({ "min-height": "44px" });
  }

  function makeBadge(text, tone = "muted") {
    const palette =
      tone === "ox"
        ? { bg: TOKENS["ox-soft"], fg: TOKENS.ox }
        : tone === "teal"
          ? { bg: TOKENS["teal-soft"], fg: TOKENS.teal }
          : tone === "brass"
            ? { bg: TOKENS["brass-soft"], fg: TOKENS.ink }
            : { bg: TOKENS.sunk, fg: TOKENS["ink-2"] };
    return bevelText(
      "span",
      [
        "display:inline-flex",
        "align-items:center",
        "padding:2px 7px",
        `font:500 11.5px/1.35 ${/\d/.test(text) ? FONT_MONO : FONT_SANS}`,
        "font-variant-numeric:tabular-nums",
        `background:${palette.bg}`,
        `color:${palette.fg}`,
      ],
      text,
      { theme: surfaceTheme, role: null },
    );
  }

  function makeStorageTag(storedInHost) {
    return bevelText(
      "span",
      [
        "display:inline-flex",
        "align-items:center",
        "padding:2px 7px",
        `font:500 11.5px/1.35 ${FONT_SANS}`,
        `background:${storedInHost ? TOKENS.sunk : TOKENS["ox-soft"]}`,
        `color:${storedInHost ? TOKENS.muted : TOKENS.ox}`,
      ],
      storedInHost ? "Saved with this document" : "Kept in this tab: keep open or download",
      { theme: surfaceTheme, role: storedInHost ? "mutedText" : null },
    );
  }

  async function runSingleDecision(jobId, editId, choice) {
    if (busyAction || room?.status?.restoring || typeof room?.decide !== "function") return;
    busyAction = true;
    render();
    try {
      await room.decide(jobId, [[editId, choice]]);
    } finally {
      busyAction = false;
      if (opened) render();
    }
  }

  async function advanceKeepRest() {
    if (!keepRestState || busyAction) return;
    if (room?.status?.restoring) {
      if (opened) render();
      return;
    }
    const allNotices = room?.notices?.() ?? [];
    const copies = room?.copies?.() ?? [];
    const copy = copies.find((one) => one.id === keepRestState.copyId);
    if (!copy) {
      keepRestState = null;
      if (opened) render();
      return;
    }
    const analyzed = analyzeCopy(copy, allNotices, dismissedEdits);
    const notice = analyzed.latestNotice;
    if (!notice || keepRestState.steps >= MAX_KEEP_REST_STEPS) {
      keepRestState = null;
      if (opened) render();
      return;
    }
    const group = analyzed.authorGroups.find((g) => g.author === keepRestState.author);
    if (!group) {
      keepRestState = null;
      if (opened) render();
      return;
    }
    if (
      keepRestState.lastJob !== null &&
      notice.job === keepRestState.lastJob &&
      group.conflict.e === keepRestState.lastEditId
    ) {
      if (opened) render();
      return;
    }
    const jobId = notice.job;
    const editId = group.conflict.e;
    keepRestState.lastJob = jobId;
    keepRestState.lastEditId = editId;
    keepRestState.steps += 1;
    busyAction = true;
    if (opened) render();
    let started = false;
    try {
      started = await room.decide(jobId, [[editId, "keep"]]);
    } catch {
      started = false;
    } finally {
      busyAction = false;
    }
    if (!started) {
      keepRestState = null;
      if (opened) render();
      return;
    }
    const afterNotices = room?.notices?.() ?? [];
    const afterAnalyzed = analyzeCopy(copy, afterNotices, dismissedEdits);
    const afterNotice = afterAnalyzed.latestNotice;
    const afterGroup = afterAnalyzed.authorGroups.find((g) => g.author === keepRestState?.author);
    const moved =
      afterNotice &&
      (afterNotice.job !== jobId || !afterGroup || afterGroup.conflict.e !== editId);
    if (moved && !room?.status?.restoring) {
      await advanceKeepRest();
      return;
    }
    if (opened) render();
  }

  function startKeepRest(copyId, author) {
    if (busyAction || room?.status?.restoring || typeof room?.decide !== "function") return;
    keepRestState = { copyId, author, lastJob: null, lastEditId: null, steps: 0 };
    void advanceKeepRest();
  }

  async function runRestore(copyId) {
    if (busyAction || room?.status?.restoring || typeof room?.restore !== "function") return;
    busyAction = true;
    render();
    try {
      await room.restore(copyId);
    } finally {
      busyAction = false;
      if (opened) render();
    }
  }

  async function handleCopyValue(copyKey, text) {
    const ok = await writeClipboard(doc, text);
    if (!ok) return;
    copiedKey = copyKey;
    clearTimeout(copiedTimer);
    copiedTimer = setTimeout(() => {
      copiedKey = null;
      if (opened) render();
    }, 1400);
    if (opened) render();
  }

  function buildValueRow(label, valueText, { mono = true, highlight = false, muted = false } = {}) {
    const row = bevelBox(
      "div",
      [
        "display:grid",
        "grid-template-columns:76px minmax(0, 1fr)",
        "gap:8px",
        "align-items:baseline",
      ],
      { theme: surfaceTheme },
    );
    row.append(
      bevelText(
        "span",
        [`font:500 11.5px/1.4 ${FONT_SANS}`, `color:${TOKENS.muted}`],
        label,
        { theme: surfaceTheme, parts: ["conflict.valueLabel"], role: "mutedText" },
      ),
      bevelText(
        "span",
        [
          "overflow-wrap:anywhere",
          `font:${highlight ? "600" : "400"} 12.5px/1.45 ${mono ? FONT_MONO : FONT_SANS}`,
          "font-variant-numeric:tabular-nums",
          `color:${muted ? TOKENS["ink-2"] : TOKENS.ink}`,
          ...(highlight ? [`background:${TOKENS["brass-soft"]}`, "padding:1px 5px"] : []),
        ],
        valueText,
        {
          theme: surfaceTheme,
          parts: [highlight ? "conflict.mine" : "conflict.theirs", "conflict.value"],
          role: muted ? "mutedText" : "text",
        },
      ),
    );
    return row;
  }

  function buildWaitingDisclosure(disclosureKey, waitingItems, currentDoc, narrow) {
    if (waitingItems.length === 0) return null;
    const details = bevelBox(
      "details",
      [
        "display:block",
        `background:${TOKENS.sunk}`,
        `border:1px solid ${TOKENS["line-2"]}`,
        "margin-top:6px",
      ],
      { theme: surfaceTheme, parts: ["conflict.well"] },
    );
    details.setAttribute("data-waiting-group", "");
    if (openDisclosures.has(disclosureKey)) details.open = true;
    details.addEventListener("toggle", () => {
      if (details.open) openDisclosures.add(disclosureKey);
      else openDisclosures.delete(disclosureKey);
    });

    const summaryText = `${waitingItems.length} later ${
      waitingItems.length === 1 ? "edit waits" : "edits wait"
    } for this decision`;

    const summary = bevelText(
      "summary",
      [
        "display:flex",
        "align-items:center",
        "cursor:pointer",
        "user-select:none",
        "padding:8px 10px",
        ...(narrow ? ["min-height:44px"] : ["min-height:32px"]),
        `font:500 12px/1.35 ${FONT_MONO}`,
        "font-variant-numeric:tabular-nums",
        `color:${TOKENS["ink-2"]}`,
      ],
      summaryText,
      { theme: surfaceTheme, role: "mutedText" },
    );
    summary.setAttribute("data-focus-key", `summary:${disclosureKey}`);
    details.append(summary);

    const list = bevelBox(
      "div",
      [
        "display:flex",
        "flex-direction:column",
        `border-top:1px solid ${TOKENS.line}`,
      ],
      { theme: surfaceTheme, parts: ["conflict.list"] },
    );

    for (const [idx, item] of waitingItems.entries()) {
      const desc = describeEntry(codec, item.entry, currentDoc);
      const reason = item.held.row
        ? "Waits for an earlier insert"
        : item.held.behind
          ? "Waits for the conflict above to be decided"
          : "Waits for an earlier decision";

      const row = bevelBox(
        "div",
        [
          "display:flex",
          "flex-direction:column",
          "gap:4px",
          "padding:8px 10px",
          ...(idx > 0 ? [`border-top:1px solid ${TOKENS.line}`] : []),
        ],
        { theme: surfaceTheme, parts: ["conflict.row"] },
      );

      const titleLine = desc.cells.length === 1 && desc.cells[0].label
        ? `${desc.title} · ${desc.cells[0].label}`
        : desc.title;

      row.append(
        bevelText(
          "div",
          [`font:600 12px/1.35 ${FONT_SANS}`, `color:${TOKENS.ink}`],
          titleLine,
          { theme: surfaceTheme, parts: ["conflict.location"] },
        ),
        bevelText(
          "div",
          [`font:400 11.5px/1.35 ${FONT_SANS}`, `color:${TOKENS.muted}`],
          reason,
          { theme: surfaceTheme, role: "mutedText" },
        ),
      );

      for (const cell of desc.cells.slice(0, 6)) {
        row.append(buildValueRow("Stored", formatValue(cell.stored), { highlight: true }));
      }
      if (desc.cells.length > 6) {
        row.append(
          bevelText(
            "div",
            [`font:400 11.5px/1.35 ${FONT_MONO}`, `color:${TOKENS.muted}`],
            `+${desc.cells.length - 6} more cells`,
            { theme: surfaceTheme, role: "mutedText" },
          ),
        );
      }

      list.append(row);
    }

    details.append(list);
    return details;
  }

  function buildConflictCard(copy, latestNotice, group, currentDoc, narrow, isPrimaryCard, activeTargetsOut) {
    const { author, authorLabel, conflict, entry, extraConflictsCount, waiting } = group;
    const cardKey = `${copy.id}:${conflict.e}`;
    const desc = describeEntry(codec, entry, currentDoc, conflict.cells);
    const conflictTargets = conflictTargetSet(conflict, desc.cells);
    for (const target of conflictTargets) {
      if (target) activeTargetsOut.add(target);
    }
    const revealTarget = primaryTargetOf(codec, entry, desc.cells, conflict);
    if (revealTarget && conflictTargets.size === 0) activeTargetsOut.add(revealTarget);

    let cardChanged = changedCards.has(cardKey);
    const cellSnapshots = desc.cells.map((cell, idx) => {
      const targetKey = `${cardKey}:${cell.target ?? idx}`;
      const liveRaw = safeCurrent(codec, currentDoc, cell.target);
      const liveText = formatTargetValue(currentDoc, cell.target, liveRaw);
      if (!initialCardValues.has(targetKey)) {
        initialCardValues.set(targetKey, liveText);
        lastCardValues.set(targetKey, liveText);
      } else {
        const initialText = initialCardValues.get(targetKey);
        const previousText = lastCardValues.get(targetKey);
        if (liveText !== initialText || liveText !== previousText) {
          changedCardTargets.add(targetKey);
          changedCards.add(cardKey);
          cardChanged = true;
        }
        lastCardValues.set(targetKey, liveText);
      }
      return {
        ...cell,
        liveText,
        changedSinceLooked: changedCardTargets.has(targetKey),
        isConflictingCell: !cell.target || conflictTargets.size === 0 || conflictTargets.has(cell.target),
      };
    });

    const isRestoring = Boolean(room?.status?.restoring);
    const isKeepingThisAuthor =
      keepRestState?.copyId === copy.id && keepRestState?.author === author;
    const controlsDisabled = isRestoring || busyAction || Boolean(keepRestState);

    const card = bevelBox(
      "div",
      [
        "display:flex",
        "flex-direction:column",
        "gap:8px",
        "padding:12px",
        `background:color-mix(in srgb, ${TOKENS["ox-soft"]} 45%, ${TOKENS.surface})`,
        `border:1px solid ${TOKENS["line-2"]}`,
      ],
      { theme: surfaceTheme, parts: ["conflict.row"] },
    );
    card.setAttribute("data-conflict-card", conflict.e);

    const topRow = bevelBox(
      "div",
      [
        "display:flex",
        "flex-wrap:wrap",
        "align-items:center",
        "justify-content:space-between",
        "gap:6px",
      ],
      { theme: surfaceTheme },
    );

    const titleText =
      desc.cells.length === 1 && desc.cells[0].label
        ? `${desc.title} · ${desc.cells[0].label}`
        : desc.title;

    topRow.append(
      bevelText(
        "div",
        [`font:600 13.5px/1.35 ${FONT_SANS}`, `color:${TOKENS.ink}`],
        titleText,
        { theme: surfaceTheme, parts: ["conflict.location"] },
      ),
    );
    if (authorLabel) {
      topRow.append(makeBadge(authorLabel, "muted"));
    }
    if (extraConflictsCount > 0) {
      topRow.append(
        makeBadge(
          `${extraConflictsCount} more ${extraConflictsCount === 1 ? "conflict" : "conflicts"} next`,
          "ox",
        ),
      );
    }
    card.append(topRow);

    if (cardChanged) {
      const changedNote = bevelText(
        "div",
        [
          "display:inline-flex",
          "align-items:center",
          "align-self:flex-start",
          "padding:3px 8px",
          `font:600 11.5px/1.35 ${FONT_SANS}`,
          `background:${TOKENS["brass-soft"]}`,
          `color:${TOKENS.ink}`,
        ],
        "Changed since you looked",
        { theme: surfaceTheme, parts: ["conflict.warning"] },
      );
      changedNote.setAttribute("data-changed-since-looked", "");
      card.append(changedNote);
    }

    const prioritized =
      cellSnapshots.length > MAX_INLINE_CELLS
        ? [
            ...cellSnapshots.filter((c) => c.isConflictingCell),
            ...cellSnapshots.filter((c) => !c.isConflictingCell),
          ].slice(0, MAX_INLINE_CELLS)
        : cellSnapshots;

    const cellsWrap = bevelWell(
      [
        "display:flex",
        "flex-direction:column",
        "gap:8px",
        "padding:9px 10px",
        `background:${TOKENS.surface}`,
        `border:1px solid ${TOKENS["line-2"]}`,
      ],
      { theme: surfaceTheme, parts: ["conflict.well"] },
    );

    for (const [idx, cell] of prioritized.entries()) {
      const cellBlock = bevelBox(
        "div",
        [
          "display:flex",
          "flex-direction:column",
          "gap:4px",
          ...(idx > 0 ? [`border-top:1px solid ${TOKENS.line}`, "padding-top:6px"] : []),
        ],
        { theme: surfaceTheme },
      );

      if (cellSnapshots.length > 1) {
        const cellHeader = bevelBox(
          "div",
          ["display:flex", "align-items:center", "justify-content:space-between", "gap:6px"],
          { theme: surfaceTheme },
        );
        cellHeader.append(
          bevelText(
            "span",
            [`font:600 12px/1.3 ${FONT_MONO}`, `color:${TOKENS.ink}`],
            cell.label,
            { theme: surfaceTheme },
          ),
        );
        if (cell.isConflictingCell) {
          cellHeader.append(makeBadge("Changed", "ox"));
        }
        cellBlock.append(cellHeader);
      }

      cellBlock.append(buildValueRow("Copy", formatTargetValue(currentDoc, cell.target, cell.stored), { highlight: true }));
      if (cell.saw !== undefined) {
        cellBlock.append(buildValueRow("Saw then", formatValue(cell.saw), { muted: true }));
      }
      cellBlock.append(
        buildValueRow("Current", cell.liveText, {
          highlight: cell.changedSinceLooked,
          muted: !cell.changedSinceLooked,
        }),
      );
      cellsWrap.append(cellBlock);
    }

    if (cellSnapshots.length > prioritized.length) {
      cellsWrap.append(
        bevelText(
          "div",
          [`font:400 11.5px/1.35 ${FONT_MONO}`, `color:${TOKENS.muted}`],
          `+${cellSnapshots.length - prioritized.length} more cells in this action`,
          { theme: surfaceTheme, role: "mutedText" },
        ),
      );
    }

    card.append(cellsWrap);

    if (isRestoring) {
      const restoringLine = bevelText(
        "div",
        [`font:500 12px/1.4 ${FONT_SANS}`, `color:${TOKENS.muted}`],
        "Restoring edits now. Decisions unlock when this pass finishes.",
        { theme: surfaceTheme, role: "mutedText" },
      );
      restoringLine.setAttribute("data-restoring-status", "");
      card.append(restoringLine);
    } else if (isKeepingThisAuthor) {
      card.append(
        bevelText(
          "div",
          [`font:500 12px/1.4 ${FONT_SANS}`, `color:${TOKENS.muted}`],
          "Keeping current values for remaining conflicts…",
          { theme: surfaceTheme, role: "mutedText" },
        ),
      );
    }

    const actionsRow = bevelBox(
      "div",
      ["display:flex", "flex-wrap:wrap", "align-items:center", "gap:6px"],
      { theme: surfaceTheme, parts: ["conflict.actions"] },
    );

    const keepBtn = bevelButton("Keep current", {
      small: true,
      variant: "default",
      theme: surfaceTheme,
      parts: ["conflict.button"],
      labelParts: ["conflict.buttonLabel"],
      onClick: () => runSingleDecision(latestNotice.job, conflict.e, "keep"),
    });
    keepBtn.setAttribute("data-focus-key", `keep:${cardKey}`);
    setControlDisabled(keepBtn, controlsDisabled);
    applyTouchTarget(keepBtn, narrow);

    const takeBtn = bevelButton("Take copy", {
      small: true,
      variant: isPrimaryCard ? "primary" : "default",
      theme: surfaceTheme,
      parts: ["conflict.button"],
      labelParts: ["conflict.buttonLabel"],
      onClick: () => runSingleDecision(latestNotice.job, conflict.e, "take"),
    });
    takeBtn.setAttribute("data-focus-key", `take:${cardKey}`);
    setControlDisabled(takeBtn, controlsDisabled);
    applyTouchTarget(takeBtn, narrow);

    const keepRestBtn = bevelButton("Keep current for the rest", {
      small: true,
      variant: "quiet",
      theme: surfaceTheme,
      parts: ["conflict.button"],
      labelParts: ["conflict.buttonLabel"],
      onClick: () => startKeepRest(copy.id, author),
    });
    keepRestBtn.setAttribute("data-focus-key", `keep-rest:${copy.id}:${author}`);
    setControlDisabled(keepRestBtn, controlsDisabled);
    applyTouchTarget(keepRestBtn, narrow);

    const revealBtn = bevelButton("Show in sheet", {
      small: true,
      variant: "quiet",
      theme: surfaceTheme,
      parts: ["conflict.button"],
      labelParts: ["conflict.buttonLabel"],
      onClick: () => onReveal?.(revealTarget),
    });
    revealBtn.setAttribute("data-focus-key", `reveal:${cardKey}`);
    applyTouchTarget(revealBtn, narrow);

    actionsRow.append(keepBtn, takeBtn, keepRestBtn, revealBtn);
    card.append(actionsRow);

    const waitingBox = buildWaitingDisclosure(
      `wait:${copy.id}:${author}`,
      waiting,
      currentDoc,
      narrow,
    );
    if (waitingBox) card.append(waitingBox);

    return card;
  }

  function buildUnsettleableCard(copy, item, currentDoc, narrow) {
    const { kind, e, entry } = item;
    const itemKey = `${copy.id}:${e}`;
    const desc = describeEntry(codec, entry, currentDoc);
    const revealTarget = primaryTargetOf(codec, entry, desc.cells);

    const reasonText =
      kind === "misfit"
        ? "No longer has a place in this document."
        : kind === "refused"
          ? "Could not be applied when it was made."
          : kind === "file-copy"
            ? "Saved in a recovery copy. Copy any values you want to keep."
            : "This document can no longer tell whether this edit was already applied.";

    const card = bevelBox(
      "div",
      [
        "display:flex",
        "flex-direction:column",
        "gap:8px",
        "padding:12px",
        `background:${TOKENS.sunk}`,
        `border:1px solid ${TOKENS["line-2"]}`,
      ],
      { theme: surfaceTheme, parts: ["conflict.row"] },
    );
    card.setAttribute("data-unsettleable-card", e);
    card.setAttribute("data-unsettleable-kind", kind);

    const titleText =
      desc.cells.length === 1 && desc.cells[0].label
        ? `${desc.title} · ${desc.cells[0].label}`
        : desc.title;

    card.append(
      bevelText(
        "div",
        [`font:600 13px/1.35 ${FONT_SANS}`, `color:${TOKENS.ink}`],
        titleText,
        { theme: surfaceTheme, parts: ["conflict.location"] },
      ),
      bevelText(
        "div",
        [`font:400 12px/1.4 ${FONT_SANS}`, `color:${TOKENS["ink-2"]}`],
        reasonText,
        { theme: surfaceTheme, parts: ["conflict.text"], role: "mutedText" },
      ),
    );

    const valuesBox = bevelWell(
      [
        "display:flex",
        "flex-direction:column",
        "gap:6px",
        "padding:8px 10px",
        `background:${TOKENS.surface}`,
        `border:1px solid ${TOKENS["line-2"]}`,
      ],
      { theme: surfaceTheme, parts: ["conflict.well"] },
    );

    if (desc.cells.length > 0) {
      for (const [idx, cell] of desc.cells.slice(0, MAX_INLINE_CELLS).entries()) {
        const liveVal = safeCurrent(codec, currentDoc, cell.target);
        const block = bevelBox(
          "div",
          [
            "display:flex",
            "flex-direction:column",
            "gap:3px",
            ...(idx > 0 ? [`border-top:1px solid ${TOKENS.line}`, "padding-top:5px"] : []),
          ],
          { theme: surfaceTheme },
        );
        if (desc.cells.length > 1) {
          block.append(
            bevelText(
              "span",
              [`font:600 11.5px/1.3 ${FONT_MONO}`, `color:${TOKENS.ink}`],
              cell.label,
              { theme: surfaceTheme },
            ),
          );
        }
        block.append(
          buildValueRow("Stored", formatTargetValue(currentDoc, cell.target, cell.stored), { highlight: true }),
          buildValueRow("Current", formatTargetValue(currentDoc, cell.target, liveVal), { muted: true }),
        );
        valuesBox.append(block);
      }
    } else {
      const fallbackCurrent = revealTarget ? safeCurrent(codec, currentDoc, revealTarget) : undefined;
      valuesBox.append(
        buildValueRow("Stored", formatValue(entry?.action?.value ?? ""), { highlight: true }),
        buildValueRow("Current", formatValue(fallbackCurrent), { muted: true }),
      );
    }

    card.append(valuesBox);

    const copyPayload =
      desc.cells.length === 1
        ? rawTextValue(desc.cells[0].stored)
        : desc.cells.length > 1
          ? desc.cells.map((c) => `${c.label}\t${rawTextValue(c.stored)}`).join("\n")
          : rawTextValue(entry?.action?.value ?? "");

    const actionsRow = bevelBox(
      "div",
      ["display:flex", "flex-wrap:wrap", "align-items:center", "gap:6px"],
      { theme: surfaceTheme, parts: ["conflict.actions"] },
    );

    const copyBtn = bevelButton("Copy value", {
      small: true,
      variant: "default",
      theme: surfaceTheme,
      parts: ["conflict.button"],
      labelParts: ["conflict.buttonLabel"],
      onClick: () => handleCopyValue(itemKey, copyPayload),
    });
    copyBtn.setAttribute("data-focus-key", `copy-val:${itemKey}`);
    applyTouchTarget(copyBtn, narrow);

    const revealBtn = bevelButton("Show in sheet", {
      small: true,
      variant: "quiet",
      theme: surfaceTheme,
      parts: ["conflict.button"],
      labelParts: ["conflict.buttonLabel"],
      onClick: () => onReveal?.(revealTarget),
    });
    revealBtn.setAttribute("data-focus-key", `reveal-unsettleable:${itemKey}`);
    applyTouchTarget(revealBtn, narrow);

    const downloadBtn = bevelButton("Download copy", {
      small: true,
      variant: "quiet",
      theme: surfaceTheme,
      parts: ["conflict.button"],
      labelParts: ["conflict.buttonLabel"],
      onClick: () => onDownload?.(copy),
    });
    downloadBtn.setAttribute("data-focus-key", `download:${itemKey}`);
    applyTouchTarget(downloadBtn, narrow);

    const dismissBtn = bevelButton("Dismiss", {
      small: true,
      variant: "quiet",
      theme: surfaceTheme,
      parts: ["conflict.button"],
      labelParts: ["conflict.buttonLabel"],
      onClick: () => {
        dismissedEdits.add(itemKey);
        render();
      },
    });
    dismissBtn.setAttribute("data-focus-key", `dismiss:${itemKey}`);
    applyTouchTarget(dismissBtn, narrow);

    actionsRow.append(copyBtn, revealBtn, downloadBtn, dismissBtn);
    if (copiedKey === itemKey) {
      actionsRow.append(makeBadge("Copied", "teal"));
    }

    card.append(actionsRow);
    return card;
  }

  function buildUnstoredCard(draft, currentDoc, narrow) {
    const edits = draft?.about?.edits ?? draft?.edits ?? [];
    const card = bevelBox(
      "section",
      [
        "display:flex",
        "flex-direction:column",
        "gap:8px",
        "padding:12px",
        `background:color-mix(in srgb, ${TOKENS["ox-soft"]} 55%, ${TOKENS.surface})`,
        `border:1px solid ${TOKENS["line-2"]}`,
      ],
      { theme: surfaceTheme, parts: ["conflict.row"] },
    );
    card.setAttribute("data-unstored-copy", draft.id ?? "");

    card.append(
      makeStorageTag(false),
      bevelText(
        "div",
        [`font:500 12.5px/1.45 ${FONT_SANS}`, `color:${TOKENS.ink}`],
        `${plural(edits.length || 1, "edit")} could not be stored on the host yet. Keep this tab open or download a copy now.`,
        { theme: surfaceTheme, parts: ["conflict.text"] },
      ),
    );

    if (edits.length > 0) {
      const previewList = bevelBox(
        "div",
        ["display:flex", "flex-direction:column", "gap:4px"],
        { theme: surfaceTheme },
      );
      for (const entry of edits.slice(0, 4)) {
        const desc = describeEntry(codec, entry, currentDoc);
        const summary =
          desc.cells.length === 1
            ? `${desc.title} · ${desc.cells[0].label}: ${formatValue(desc.cells[0].stored)}`
            : desc.title;
        previewList.append(
          bevelText(
            "div",
            [`font:400 12px/1.35 ${FONT_MONO}`, `color:${TOKENS["ink-2"]}`],
            summary,
            { theme: surfaceTheme, role: "mutedText" },
          ),
        );
      }
      card.append(previewList);
    }

    const actions = bevelBox(
      "div",
      ["display:flex", "flex-wrap:wrap", "gap:6px"],
      { theme: surfaceTheme, parts: ["conflict.actions"] },
    );
    const downloadBtn = bevelButton("Download copy", {
      small: true,
      variant: "default",
      theme: surfaceTheme,
      parts: ["conflict.button"],
      labelParts: ["conflict.buttonLabel"],
      onClick: () => onDownload?.(draft),
    });
    downloadBtn.setAttribute("data-focus-key", `download-unstored:${draft.id ?? ""}`);
    applyTouchTarget(downloadBtn, narrow);
    actions.append(downloadBtn);
    card.append(actions);

    return card;
  }

  function buildCopySection(analyzed, currentDoc, narrow, primaryBudget, activeTargetsOut) {
    const { copy, latestNotice, needsRestore, counts, authorGroups, orphanHeld, unsettleable } = analyzed;
    const totalEdits = Array.isArray(copy.edits) ? copy.edits.length : 0;
    const isRestoring = Boolean(room?.status?.restoring);

    const section = bevelBox(
      "section",
      [
        "display:flex",
        "flex-direction:column",
        "gap:10px",
        "padding:12px",
        `background:${TOKENS.ground}`,
        `border:1px solid ${TOKENS["line-2"]}`,
      ],
      { theme: surfaceTheme },
    );
    section.setAttribute("data-copy-section", copy.id);

    const metaRow = bevelBox(
      "div",
      ["display:flex", "flex-wrap:wrap", "align-items:center", "gap:6px"],
      { theme: surfaceTheme },
    );
    metaRow.append(makeStorageTag(true));
    for (const badge of outcomeBadgesFor(counts, needsRestore, totalEdits)) {
      metaRow.append(makeBadge(badge.text, badge.tone));
    }
    section.append(metaRow);

    if (needsRestore) {
      section.append(
        bevelText(
          "div",
          [`font:400 12.5px/1.45 ${FONT_SANS}`, `color:${TOKENS["ink-2"]}`],
          "This recovery copy has not been checked against the current document in this tab yet.",
          { theme: surfaceTheme, role: "mutedText" },
        ),
      );
      if (isRestoring) {
        const statusEl = bevelText(
          "div",
          [`font:500 12px/1.4 ${FONT_SANS}`, `color:${TOKENS.muted}`],
          "Restoring edits now. Please wait until this pass finishes.",
          { theme: surfaceTheme, role: "mutedText" },
        );
        statusEl.setAttribute("data-restoring-status", "");
        section.append(statusEl);
      }
      const actions = bevelBox(
        "div",
        ["display:flex", "flex-wrap:wrap", "gap:6px"],
        { theme: surfaceTheme, parts: ["conflict.actions"] },
      );
      const restoreBtn = bevelButton("Check and restore", {
        small: true,
        variant: primaryBudget.used ? "default" : "primary",
        theme: surfaceTheme,
        parts: ["conflict.button"],
        labelParts: ["conflict.buttonLabel"],
        onClick: () => runRestore(copy.id),
      });
      primaryBudget.used = true;
      restoreBtn.setAttribute("data-focus-key", `restore:${copy.id}`);
      setControlDisabled(restoreBtn, isRestoring || busyAction);
      applyTouchTarget(restoreBtn, narrow);

      const downloadBtn = bevelButton("Download copy", {
        small: true,
        variant: "quiet",
        theme: surfaceTheme,
        parts: ["conflict.button"],
        labelParts: ["conflict.buttonLabel"],
        onClick: () => onDownload?.(copy),
      });
      downloadBtn.setAttribute("data-focus-key", `download-copy:${copy.id}`);
      applyTouchTarget(downloadBtn, narrow);

      actions.append(restoreBtn, downloadBtn);
      section.append(actions);
    }

    for (const group of authorGroups) {
      const usePrimary = !primaryBudget.used;
      if (usePrimary) primaryBudget.used = true;
      section.append(
        buildConflictCard(copy, latestNotice, group, currentDoc, narrow, usePrimary, activeTargetsOut),
      );
    }

    if (orphanHeld.length > 0) {
      const orphanBox = buildWaitingDisclosure(
        `wait-orphan:${copy.id}`,
        orphanHeld,
        currentDoc,
        narrow,
      );
      if (orphanBox) section.append(orphanBox);
    }

    for (const item of unsettleable) {
      section.append(buildUnsettleableCard(copy, item, currentDoc, narrow));
    }

    return section;
  }

  function buildEarlierSection(settledList, narrow) {
    const details = bevelBox(
      "details",
      [
        "display:block",
        `background:${TOKENS.sunk}`,
        `border:1px solid ${TOKENS["line-2"]}`,
      ],
      { theme: surfaceTheme },
    );
    details.setAttribute("data-earlier-section", "");
    if (openDisclosures.has("earlier")) details.open = true;
    details.addEventListener("toggle", () => {
      if (details.open) openDisclosures.add("earlier");
      else openDisclosures.delete("earlier");
    });

    const summary = bevelText(
      "summary",
      [
        "display:flex",
        "align-items:center",
        "justify-content:space-between",
        "cursor:pointer",
        "user-select:none",
        "padding:9px 12px",
        ...(narrow ? ["min-height:44px"] : ["min-height:34px"]),
        `font:600 12.5px/1.35 ${FONT_SANS}`,
        `color:${TOKENS["ink-2"]}`,
      ],
      `Earlier (${settledList.length})`,
      { theme: surfaceTheme, role: "mutedText" },
    );
    summary.setAttribute("data-focus-key", "summary:earlier");
    details.append(summary);

    const stack = bevelBox(
      "div",
      [
        "display:flex",
        "flex-direction:column",
        "gap:8px",
        "padding:10px 12px",
        `border-top:1px solid ${TOKENS.line}`,
      ],
      { theme: surfaceTheme },
    );

    for (const analyzed of settledList) {
      const { copy, counts } = analyzed;
      const item = bevelBox(
        "div",
        [
          "display:flex",
          "flex-direction:column",
          "gap:6px",
          "padding:10px",
          `background:${TOKENS.surface}`,
          `border:1px solid ${TOKENS.line}`,
        ],
        { theme: surfaceTheme },
      );
      item.setAttribute("data-settled-copy", copy.id);

      const badges = bevelBox(
        "div",
        ["display:flex", "flex-wrap:wrap", "align-items:center", "gap:6px"],
        { theme: surfaceTheme },
      );
      badges.append(makeStorageTag(true));
      for (const b of outcomeBadgesFor(counts, false, copy.edits?.length ?? 0)) {
        badges.append(makeBadge(b.text, b.tone));
      }
      item.append(badges);

      const dlBtn = bevelButton("Download copy", {
        small: true,
        variant: "quiet",
        theme: surfaceTheme,
        parts: ["conflict.button"],
        labelParts: ["conflict.buttonLabel"],
        onClick: () => onDownload?.(copy),
      });
      dlBtn.setAttribute("data-focus-key", `download-settled:${copy.id}`);
      applyTouchTarget(dlBtn, narrow);
      const row = bevelBox("div", ["display:flex", "flex-wrap:wrap", "gap:6px"], { theme: surfaceTheme });
      row.append(dlBtn);
      item.append(row);

      stack.append(item);
    }

    details.append(stack);
    return details;
  }

  function emitHighlights(targets, force = false) {
    const sig = targets.join("\u0000");
    if (!force && sig === lastHighlightSig) return;
    lastHighlightSig = sig;
    onConflictsHighlight?.(targets);
  }

  function render({ forceHighlight = false } = {}) {
    if (destroyed) return;
    const narrow = isNarrowScreen(win);
    applyTouchTarget(closeBtn, narrow);
    set(body, "padding", narrow ? "12px 12px 20px" : "14px 16px 20px");

    const rootNode = drawer.getRootNode?.() ?? doc;
    const activeEl = rootNode?.activeElement ?? null;
    const focusedKey = activeEl && drawer.contains(activeEl) ? activeEl.getAttribute("data-focus-key") : null;
    const savedScroll = body.scrollTop;

    const currentDoc = typeof room?.view === "function" ? room.view() : null;
    const fileMode = room?.status?.mode === "file";
    const allNotices = typeof room?.notices === "function" ? room.notices() ?? [] : [];
    const rawCopies = typeof room?.copies === "function" ? (room.copies() ?? []).slice() : [];
    rawCopies.sort(compareCopiesOldestFirst);
    const unstoredDrafts = typeof room?.unstoredCopies === "function" ? room.unstoredCopies() ?? [] : [];

    const analyzedCopies = rawCopies.map((copy) => analyzeCopy(copy, allNotices, dismissedEdits, { codec, currentDoc, fileMode }));
    const openCopies = analyzedCopies.filter((c) => !c.isSettled);
    const settledCopies = analyzedCopies.filter((c) => c.isSettled);

    body.replaceChildren();
    const activeTargets = new Set();
    const primaryBudget = { used: false };

    if (room?.status?.restoring && openCopies.length === 0) {
      const banner = bevelBox(
        "div",
        [
          "padding:10px 12px",
          `background:${TOKENS["brass-soft"]}`,
          `border:1px solid ${TOKENS["line-2"]}`,
          `font:500 12.5px/1.4 ${FONT_SANS}`,
          `color:${TOKENS.ink}`,
        ],
        { theme: surfaceTheme },
      );
      banner.setAttribute("data-restoring-status", "");
      banner.textContent = "Restoring edits now. Decisions unlock when this pass finishes.";
      body.append(banner);
    }

    for (const draft of unstoredDrafts) {
      body.append(buildUnstoredCard(draft, currentDoc, narrow));
    }

    for (const openCopy of openCopies) {
      body.append(buildCopySection(openCopy, currentDoc, narrow, primaryBudget, activeTargets));
    }

    if (unstoredDrafts.length === 0 && openCopies.length === 0 && settledCopies.length === 0) {
      body.append(
        bevelText(
          "p",
          ["margin:0", `font:400 13px/1.5 ${FONT_SANS}`, `color:${TOKENS.muted}`],
          "No recovery copies for this document.",
          { theme: surfaceTheme, role: "mutedText" },
        ),
      );
    }

    if (settledCopies.length > 0) {
      body.append(buildEarlierSection(settledCopies, narrow));
    }

    if (themed(surfaceTheme)) {
      markParts(drawer, surfaceTheme, ["conflict.panel"]);
      style(drawer, partRules(surfaceTheme, ["conflict.panel"], {}));
    }

    body.scrollTop = savedScroll;
    if (focusedKey) {
      const match = [...drawer.querySelectorAll("[data-focus-key]")].find(
        (node) => node.getAttribute("data-focus-key") === focusedKey && !node.disabled,
      );
      match?.focus?.({ preventScroll: true });
    }

    if (opened) {
      emitHighlights([...activeTargets], forceHighlight);
    }
  }

  function onRoomEvent() {
    if (keepRestState) {
      void advanceKeepRest();
    }
    if (opened) {
      render();
    }
  }

  function open() {
    if (destroyed) return;
    const wasOpen = opened;
    if (!wasOpen) {
      previousFocus = doc?.activeElement ?? null;
      opened = true;
      initialCardValues.clear();
      lastCardValues.clear();
      changedCardTargets.clear();
      changedCards.clear();
      lastHighlightSig = null;
      mounted.host.hidden = false;
      setShown(drawer, true, "flex");
      roomUnsub = typeof room?.subscribe === "function" ? room.subscribe(onRoomEvent) : null;
      doc?.addEventListener("keydown", onKeydown, true);
      win?.addEventListener("resize", onResize);
    }
    render({ forceHighlight: !wasOpen });
    Promise.resolve(room?.refreshCopies?.()).then(
      () => {
        if (opened && !destroyed) render();
      },
      () => {},
    );
    if (!wasOpen) {
      const first = focusablesInDrawer()[0];
      first?.focus?.({ preventScroll: true });
    }
  }

  function close() {
    if (!opened) {
      mounted.host.hidden = true;
      setShown(drawer, false, "flex");
      return;
    }
    opened = false;
    keepRestState = null;
    roomUnsub?.();
    roomUnsub = null;
    doc?.removeEventListener("keydown", onKeydown, true);
    win?.removeEventListener("resize", onResize);
    mounted.host.hidden = true;
    setShown(drawer, false, "flex");
    lastHighlightSig = null;
    onConflictsHighlight?.([]);
    if (previousFocus && previousFocus.isConnected && typeof previousFocus.focus === "function") {
      previousFocus.focus({ preventScroll: true });
    }
    previousFocus = null;
  }

  function destroy() {
    if (destroyed) return;
    if (opened) close();
    destroyed = true;
    clearTimeout(copiedTimer);
    mounted.remove();
  }

  return {
    open,
    close,
    isOpen: () => opened,
    el: drawer,
    destroy,
  };
}
