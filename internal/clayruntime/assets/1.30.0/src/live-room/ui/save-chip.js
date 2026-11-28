import { bevelBox, bevelButton, bevelText, protectIcon } from "../../ui/bevel-controls.js";
import { FONT_MONO, FONT_SANS, GLYPHS, TOKENS } from "../../ui/bevel.js";
import { resolveTheme } from "../../ui/theme.js";
import { markParts, partRules, themed } from "../../ui/theme-parts.js";
import { style } from "../../lib/hostile-css.js";

const CLOSING_MARKS = new Set(["taken", "saved", "kept", "misfit"]);

function fallbackOpenConflicts(room) {
  const closed = new Set();
  const open = new Set();
  for (const copy of room?.copies?.() ?? []) {
    for (const mark of copy?.marks ?? []) {
      if (mark?.e && CLOSING_MARKS.has(mark.kind)) closed.add(mark.e);
    }
  }
  const notices = typeof room?.notices === "function" ? room.notices() : [];
  if (Array.isArray(notices)) {
    const latestByCopy = new Map();
    for (const notice of notices) {
      if (notice && notice.kind === "restored" && notice.copy) {
        latestByCopy.set(notice.copy, notice);
      }
    }
    for (const notice of latestByCopy.values()) {
      if (!Array.isArray(notice.conflicts)) continue;
      for (const item of notice.conflicts) {
        if (item?.e && closed.has(item.e)) continue;
        if (item?.e) open.add(item.e);
      }
    }
  }
  for (const copy of room?.copies?.() ?? []) {
    for (const mark of copy?.marks ?? []) {
      if (mark?.e && mark.kind === "open" && !closed.has(mark.e)) open.add(mark.e);
    }
  }
  return open.size;
}

function recoveryAttention(room) {
  const notices = typeof room?.notices === "function" ? room.notices() ?? [] : [];
  const copies = typeof room?.copies === "function" ? room.copies() ?? [] : [];
  const latestByCopy = new Map();
  for (const notice of notices) {
    if (notice && notice.kind === "restored" && notice.copy) {
      latestByCopy.set(notice.copy, notice);
    }
  }
  let unapplied = 0;
  for (const notice of latestByCopy.values()) {
    unapplied += (notice.misfit?.length ?? 0) + (notice.left?.length ?? 0);
  }
  if (unapplied > 0) return { count: unapplied, kind: "unapplied" };
  if (room?.status?.mode === "file" && copies.length > 0) {
    const edits = copies.reduce((sum, copy) => sum + (Array.isArray(copy.edits) ? copy.edits.length : 0), 0);
    if (edits > 0) return { count: edits, kind: "file-copy" };
  }
  return null;
}

function classifyStatus(room) {
  const status = room?.status ?? {};
  const mode = status.mode ?? "connecting";
  const connected = status.connected === true;
  const pending = typeof status.pending === "number" && Number.isFinite(status.pending) ? status.pending : -1;
  const unsaved = typeof status.unsaved === "number" && Number.isFinite(status.unsaved) ? status.unsaved : -1;
  const unknownSave = Boolean(status.unknownSave);
  const unstored = typeof status.unstored === "number" && Number.isFinite(status.unstored) ? status.unstored : 0;
  const pendingMarks =
    status.pendingMarks === undefined
      ? 0
      : typeof status.pendingMarks === "number" && Number.isFinite(status.pendingMarks)
        ? status.pendingMarks
        : -1;
  const conflicts =
    typeof status.conflicts === "number" && Number.isFinite(status.conflicts)
      ? status.conflicts
      : fallbackOpenConflicts(room);
  const restoring = Boolean(status.restoring);
  const error = status.error ?? null;
  const attention = recoveryAttention(room);

  const isSaved =
    (mode === "ordered" || mode === "file") &&
    connected === true &&
    pending === 0 &&
    unsaved === 0 &&
    !unknownSave &&
    unstored === 0 &&
    pendingMarks === 0 &&
    conflicts === 0 &&
    !attention &&
    !restoring &&
    !error;

  if (mode === "unavailable") {
    return {
      key: "unavailable",
      tone: "ox",
      text: "Cannot save here",
      buttonLabel: "Download copy",
      buttonKind: "download",
      mono: false,
    };
  }

  if (unstored > 0) {
    const noun = unstored === 1 ? "copy" : "copies";
    return {
      key: "unstored",
      tone: "ox",
      text: null,
      buttonLabel: `${unstored} ${noun} not stored`,
      buttonKind: "unstored",
      mono: true,
    };
  }

  if (error) {
    return {
      key: "error",
      tone: "ox",
      text: pending > 0 || unsaved > 0 ? "Offline, kept in tab" : "Offline, retrying",
      buttonLabel: null,
      buttonKind: null,
      mono: false,
    };
  }

  if (mode === "connecting") {
    return {
      key: "connecting",
      tone: "faint",
      text: "Connecting…",
      buttonLabel: null,
      buttonKind: null,
      mono: false,
    };
  }

  if (!connected) {
    return {
      key: "offline",
      tone: mode === "closed" ? "faint" : "ox",
      text: mode === "closed" ? "Closed" : "Offline",
      buttonLabel: null,
      buttonKind: null,
      mono: false,
    };
  }

  if (restoring) {
    return {
      key: "restoring",
      tone: "brass",
      text: "Restoring edits…",
      buttonLabel: null,
      buttonKind: null,
      mono: false,
    };
  }

  if (conflicts > 0) {
    const noun = conflicts === 1 ? "conflict" : "conflicts";
    return {
      key: "conflicts",
      tone: "ox",
      text: null,
      buttonLabel: `${conflicts} ${noun} to review`,
      buttonKind: "conflicts",
      mono: true,
    };
  }

  if (unknownSave) {
    return {
      key: "checking",
      tone: "brass",
      text: "Checking save…",
      buttonLabel: null,
      buttonKind: null,
      mono: false,
    };
  }

  if (pending > 0) {
    const noun = pending === 1 ? "edit" : "edits";
    return {
      key: "pending",
      tone: "brass",
      text: mode === "file" ? `${pending} local ${noun}, saving…` : `${pending} local ${noun}`,
      buttonLabel: null,
      buttonKind: null,
      mono: true,
    };
  }

  if (unsaved > 0) {
    return {
      key: "saving",
      tone: "brass",
      text: "Shared, saving…",
      buttonLabel: null,
      buttonKind: null,
      mono: false,
    };
  }

  if (pendingMarks > 0) {
    return {
      key: "marking",
      tone: "brass",
      text: "Recording decision…",
      buttonLabel: null,
      buttonKind: null,
      mono: false,
    };
  }

  if (attention) {
    const noun = attention.count === 1 ? "edit" : "edits";
    return {
      key: "recovery",
      tone: "brass",
      text: null,
      buttonLabel:
        attention.kind === "file-copy"
          ? `${attention.count} recovered ${noun}`
          : `${attention.count} ${noun} not applied`,
      buttonKind: "recovery",
      mono: true,
    };
  }

  if (isSaved) {
    return {
      key: "saved",
      tone: "teal",
      text: "Saved",
      buttonLabel: null,
      buttonKind: null,
      mono: false,
    };
  }

  return {
    key: "syncing",
    tone: "brass",
    text: "Syncing…",
    buttonLabel: null,
    buttonKind: null,
    mono: false,
  };
}

function toneColor(tone) {
  if (tone === "teal") return TOKENS.teal;
  if (tone === "ox") return TOKENS.ox;
  if (tone === "brass") return TOKENS.brass;
  return TOKENS.faint;
}

function buildPip(tone, surfaceTheme) {
  if (tone === "teal") {
    const holder = bevelText(
      "span",
      ["display:inline-grid", "place-items:center", "flex:none", `color:${TOKENS.teal}`],
      undefined,
      { theme: surfaceTheme, parts: ["notice.icon"], role: null },
    );
    holder.innerHTML = GLYPHS.toastSuccess;
    protectIcon(holder.firstElementChild, 12);
    return holder;
  }
  const pip = bevelBox(
    "span",
    [
      "display:inline-block",
      "flex:none",
      "width:6px",
      "height:6px",
      `background:${toneColor(tone)}`,
    ],
    { theme: surfaceTheme, parts: ["notice.dot"] },
  );
  pip.setAttribute("aria-hidden", "true");
  return pip;
}

export function createSaveChip({ room, onOpenRecovery, onDownload, theme = null } = {}) {
  const surfaceTheme = theme ?? resolveTheme();
  const el = bevelBox(
    "div",
    [
      "display:inline-flex",
      "align-items:center",
      "gap:6px",
      "min-width:0",
      "max-width:100%",
      "min-height:24px",
      `color-scheme:${surfaceTheme.scheme}`,
      `font:500 11.5px/1.2 ${FONT_SANS}`,
      `color:${TOKENS["ink-2"]}`,
    ],
    { theme: surfaceTheme, parts: ["notice.root"] },
  );
  el.setAttribute("role", "status");
  el.setAttribute("aria-live", "polite");
  el.setAttribute("data-live-room-chip", "");
  if (themed(surfaceTheme)) {
    markParts(el, surfaceTheme, ["notice.root"]);
    style(el, partRules(surfaceTheme, ["notice.root"], {}));
  }

  let signature = "";
  let destroyed = false;

  function triggerAction(kind) {
    if (kind === "download") {
      onDownload?.();
      return;
    }
    if (kind === "unstored") {
      if (typeof onOpenRecovery === "function") onOpenRecovery();
      else onDownload?.();
      return;
    }
    if (kind === "conflicts" || kind === "recovery") {
      onOpenRecovery?.();
    }
  }

  function update() {
    if (destroyed) return;
    const state = classifyStatus(room);
    const hasOpen = typeof onOpenRecovery === "function" ? "1" : "0";
    const hasDownload = typeof onDownload === "function" ? "1" : "0";
    const nextSig = `${state.key}|${state.text ?? ""}|${state.buttonLabel ?? ""}|${hasOpen}|${hasDownload}`;
    if (nextSig === signature) return;
    signature = nextSig;

    el.setAttribute("data-save-state", state.key);
    el.replaceChildren();

    if (state.text) {
      const pill = bevelBox(
        "span",
        [
          "display:inline-flex",
          "align-items:center",
          "gap:5px",
          "min-width:0",
          "max-width:100%",
          "padding:2px 6px",
          ...(state.tone === "ox" ? [`background:${TOKENS["ox-soft"]}`] : []),
        ],
        { theme: surfaceTheme, parts: ["notice.panel"] },
      );
      pill.append(buildPip(state.tone, surfaceTheme));
      const label = bevelText(
        "span",
        [
          "overflow:hidden",
          "text-overflow:ellipsis",
          "white-space:nowrap",
          `font:500 11.5px/1.25 ${state.mono ? FONT_MONO : FONT_SANS}`,
          ...(state.mono ? ["font-variant-numeric:tabular-nums"] : []),
          `color:${state.tone === "ox" ? TOKENS.ox : state.key === "saved" ? TOKENS.ink : TOKENS["ink-2"]}`,
        ],
        state.text,
        { theme: surfaceTheme, parts: ["notice.text"], role: state.tone === "ox" ? null : "text" },
      );
      pill.append(label);
      el.append(pill);
    }

    if (state.buttonLabel) {
      const warnStyle =
        state.tone === "ox"
          ? [
              `background:${TOKENS["ox-soft"]}`,
              `color:${TOKENS.ox}`,
            ]
          : [];
      const btn = bevelButton(state.buttonLabel, {
        small: true,
        theme: surfaceTheme,
        parts: ["notice.button"],
        labelParts: ["notice.buttonLabel"],
        onClick: () => triggerAction(state.buttonKind),
        extra: [
          "height:24px",
          "padding:0 8px",
          "gap:5px",
          `font:500 11.5px/1.2 ${state.mono ? FONT_MONO : FONT_SANS}`,
          ...(state.mono ? ["font-variant-numeric:tabular-nums"] : []),
          ...warnStyle,
        ],
      });
      el.append(btn);
    }
  }

  const unsubscribe = typeof room?.subscribe === "function" ? room.subscribe(() => update()) : () => {};
  update();

  function destroy() {
    if (destroyed) return;
    destroyed = true;
    unsubscribe?.();
    el.remove();
  }

  return { el, update, destroy };
}
