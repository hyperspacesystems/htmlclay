import { engine, extractData, applyData, blankData } from "../../vendor/hyper-html-api-data.vendor.js";

const STARTING = 'script[type="application/json"][clay-new-document]';
const MAX_PREVIOUS_DRAFTS = 15;
let pending = null;

export function newDocumentLabels(doc = document) {
  const tag = doc.querySelector(STARTING);
  if (!tag) return null;
  const noun = (tag.getAttribute("clay-new-document") || "").trim();
  const pluralAttr = (tag.getAttribute("clay-new-document-plural") || "").trim();
  const plural = pluralAttr || (noun ? `${noun}s` : "documents");
  return {
    noun: noun || "document",
    plural,
    newLabel: noun ? `New ${noun}` : "New blank document",
    duplicateLabel: `Duplicate ${noun || "document"}`,
    previousLabel: (count) => `Previous ${plural} (${count})`,
  };
}

function startingValues(doc = document) {
  const tag = doc.querySelector(STARTING);
  if (!tag) return {};
  try {
    const value = JSON.parse(tag.textContent);
    if (value && typeof value === "object" && !Array.isArray(value)) return value;
  } catch {}
  return {};
}

function writableRules(rules, doc = document) {
  const keep = new Set((doc.querySelector(STARTING)?.getAttribute("keep") || "").split(/\s+/).filter(Boolean));
  const active = {};
  for (const [key, rule] of Object.entries(rules || {})) {
    if (key === "site" || rule === "html@look" || keep.has(key) || engine.codecOf(rule)) continue;
    active[key] = rule;
  }
  return active;
}

function extractableRules(rules) {
  const active = {};
  for (const [key, rule] of Object.entries(rules || {})) {
    if (key === "site" || engine.codecOf(rule)) continue;
    active[key] = rule;
  }
  return active;
}

// "New [item name]": this page with its data emptied, as a new file the host keeps or
// the browser downloads. Offered only where the page says what a blank copy is, with a
// starting values tag (its template's choice); a page without one would keep data its rules
// do not name. Nothing is asked of the host until the item is clicked, and a second click
// while the first is under way makes nothing more.
export function addNewDocumentItem({ addItem, note }) {
  const labels = newDocumentLabels();
  if (!labels) return;
  addItem({
    id: "clay.new",
    label: labels.newLabel,
    run: async () => {
      if (pending) return;
      let result = null;
      pending = Promise.resolve().then(() => window.clay.newDocument.create()).catch(() => null);
      try {
        result = await pending;
      } finally {
        pending = null;
      }
      if (result?.ok && typeof result.href === "string") {
        // Opened after the host answers, so a popup blocker may refuse it; then this tab goes.
        // "noopener" would make window.open answer null even when it opened, so opener is cut here.
        const opened = window.open(result.href, "_blank");
        if (opened) opened.opener = null;
        else window.location.assign(result.href);
      } else if (!result?.ok) note(result?.message || "Couldn't make a new document. Try again.");
    },
  });
}

// Visitor demo mode: clicking "New [item name]" archives the current draft in browser storage,
// resets the live page to its starting values, and reveals the indented "Previous [items] (N)"
// row underneath. Never shown on the platform (hyperclay.com owner), HTML Clay, or Hyperclay Local,
// and never shown before the visitor has clicked "New [item name]" at least once.
export function addVisitorNewDocumentItem({ addItem, removeItem, note, goo }) {
  const labels = newDocumentLabels();
  if (!labels) return;

  const baseKey = () =>
    window.clay?.visitorDrafts?.key ||
    `clay:visitor-drafts:${document.documentElement.getAttribute("visitor-drafts") || "default"}@${location.pathname}`;
  const listKey = () => `${baseKey()}:previous-drafts`;
  const starterKey = () => `${baseKey()}:starter-seed`;

  function readJson(key, fallback) {
    try {
      const raw = localStorage.getItem(key);
      return raw ? JSON.parse(raw) : fallback;
    } catch {
      return fallback;
    }
  }

  function writeJson(key, value) {
    try {
      if (value == null || (Array.isArray(value) && value.length === 0)) localStorage.removeItem(key);
      else localStorage.setItem(key, JSON.stringify(value));
      return true;
    } catch {
      return false;
    }
  }

  function readPreviousDrafts() {
    const list = readJson(listKey(), []);
    return Array.isArray(list) ? list.filter((item) => item && typeof item.id === "string") : [];
  }

  function captureSnapshot(kind = "draft") {
    const found = engine.findRulesIn(document, "api");
    const rules = extractableRules(found?.rules);
    let data = {};
    try {
      if (Object.keys(rules).length) data = extractData(document, rules);
    } catch {}
    const workbook = document.querySelector("[sheet-workbook]")?.innerHTML ?? null;
    const article = document.querySelector("article[data-blockeditor]");
    const liveStores = {};
    for (const tag of document.querySelectorAll('script[type="application/json"][merge][clay~="live"]')) {
      liveStores[tag.getAttribute("merge")] = tag.textContent;
    }
    const blockeditor = article
      ? { rev: article.getAttribute("data-rev") || "", html: article.innerHTML, liveStores }
      : null;
    const rawTitle =
      data.name ||
      data.boardName ||
      data.title ||
      document.querySelector("article[data-blockeditor] h1, #docName, #docTitle, #board-name, h1")?.textContent ||
      "";
    const title = String(rawTitle).trim() || (kind === "starter" ? `Starter ${labels.noun}` : `Untitled ${labels.noun}`);
    return {
      id: `${kind}-${Date.now()}-${Math.random().toString(36).slice(2, 7)}`,
      kind,
      title,
      savedAt: new Date().toISOString(),
      data,
      workbook,
      blockeditor,
    };
  }

  function applySnapshot(snapshot) {
    if (!snapshot || typeof snapshot !== "object") return;
    const found = engine.findRulesIn(document, "api");
    const rules = extractableRules(found?.rules);
    if (snapshot.data && Object.keys(rules).length) {
      try {
        applyData(document, snapshot.data, rules);
      } catch {}
    }
    if (typeof snapshot.workbook === "string") {
      const wb = document.querySelector("[sheet-workbook]");
      if (wb) wb.innerHTML = snapshot.workbook;
    }
    if (snapshot.blockeditor) {
      const article = document.querySelector("article[data-blockeditor]");
      if (article) {
        if (snapshot.blockeditor.rev) article.setAttribute("data-rev", snapshot.blockeditor.rev);
        article.innerHTML = snapshot.blockeditor.html || "";
      }
      const stores = snapshot.blockeditor.liveStores || {};
      for (const [name, text] of Object.entries(stores)) {
        const tag = document.querySelector(`script[type="application/json"][merge="${name}"][clay~="live"]`);
        if (tag) tag.textContent = text;
      }
    }
    forgetHistory();
    try { window.Sap?.refresh?.(); } catch {}
    try { window.clay?.visitorDrafts?.saveNow?.(); } catch {}
  }

  // A whole draft replaced the page, so the edits before it belong to another document:
  // undoing one now would replay it into this draft, or do nothing. Same turn as the writes,
  // so the undo engine drops their buffered records too and the switch itself is no step.
  function forgetHistory() {
    try { window.clay?.undo?.clear?.(); } catch {}
  }

  // Keep the untouched starter example in memory on load so a reload with no edits leaves
  // zero keys in localStorage; persist it only once the visitor edits or starts a new copy.
  const bootStarter =
    !readJson(starterKey(), null) && !window.clay?.visitorDrafts?.hasDraft?.()
      ? captureSnapshot("starter")
      : null;

  function ensureStarterSaved() {
    if (bootStarter && !readJson(starterKey(), null)) {
      writeJson(starterKey(), bootStarter);
    }
  }

  document.addEventListener("clay:drafts-saved", (event) => {
    if (!event.detail?.unchanged) ensureStarterSaved();
  });

  function syncPreviousItem() {
    const drafts = readPreviousDrafts();
    if (!drafts.length) {
      removeItem?.("clay.previous");
      return;
    }
    addItem({
      id: "clay.previous",
      nested: true,
      label: labels.previousLabel(drafts.length),
      run: () => openPreviousModal().catch(() => note?.("Couldn't open previous drafts. Try again.")),
    });
  }

  function startNewVisitorDraft() {
    ensureStarterSaved();
    try { window.clay?.visitorDrafts?.saveNow?.(); } catch {}
    const current = captureSnapshot("draft");
    const list = [current, ...readPreviousDrafts()].slice(0, MAX_PREVIOUS_DRAFTS);
    writeJson(listKey(), list);

    const found = engine.findRulesIn(document, "api");
    const rules = writableRules(found?.rules, document);
    if (Object.keys(rules).length > 0) {
      applyData(document, blankData(rules), rules);
      const starting = startingValues(document);
      if (Object.keys(starting).some((key) => Object.prototype.hasOwnProperty.call(rules, key))) {
        applyData(document, blankData(rules, starting), rules);
      }
    }
    forgetHistory();
    document.dispatchEvent(new CustomEvent("clay:newdocument", { detail: { document, live: true } }));
    try { window.Sap?.refresh?.(); } catch {}
    try { window.clay?.visitorDrafts?.saveNow?.(); } catch {}
    syncPreviousItem();
  }

  async function openPreviousModal() {
    const [{ bevelBox, bevelButton, bevelText }, { FONT_MONO }, { resolveTheme }, { default: modal, SURFACE_THEME }] =
      await Promise.all([
        import("../../ui/bevel-controls.js"),
        import("../../ui/bevel.js"),
        import("../../ui/theme.js"),
        import("../../ui/modal.js"),
      ]);
    const theme = resolveTheme();
    const box = bevelBox("div", ["display:flex", "flex-direction:column", "gap:14px"], { theme, parts: ["dialog.content"] });
    box.dataset.previousDrafts = "";
    box.append(
      bevelText(
        "p",
        ["display:block", "margin:0"],
        `Saved in this browser on this device. Switch back to an earlier ${labels.noun} or download the one you want to keep.`,
        { theme, role: "mutedText" },
      ),
    );
    const list = bevelBox("ol", ["display:flex", "flex-direction:column", "gap:10px", "padding:0", "margin:0", "list-style:none"], { theme });
    box.append(list);

    function draw() {
      list.replaceChildren();
      const active = captureSnapshot("active");
      const activeItem = bevelBox(
        "li",
        ["display:flex", "align-items:center", "justify-content:space-between", "gap:12px", "padding:10px 12px", `border:1px solid ${theme.border}`, `border-radius:${theme.radius}`],
        { theme },
      );
      const activeCopy = bevelBox("div", ["display:flex", "flex-direction:column", "gap:2px"], { theme });
      activeCopy.append(
        bevelText("strong", ["display:block"], active.title, { theme }),
        bevelText("span", ["display:block", `font-family:${FONT_MONO}`, "font-size:12px"], "Active draft in this tab", { theme, role: "mutedText" }),
      );
      const statusBadge = bevelText("span", ["display:inline-flex", "align-items:center", "gap:6px", "font-size:12px", "font-weight:600", `color:${theme.accent}`], "● Currently open", { theme });
      statusBadge.dataset.draftStatus = "open";
      activeItem.append(activeCopy, statusBadge);
      list.append(activeItem);

      const drafts = readPreviousDrafts();
      for (const row of drafts) {
        const item = bevelBox(
          "li",
          ["display:flex", "align-items:center", "justify-content:space-between", "gap:12px", "padding:10px 12px", `border:1px solid ${theme.border}`, `border-radius:${theme.radius}`],
          { theme },
        );
        item.dataset.draftId = row.id;
        const date = new Date(row.savedAt);
        const when = Number.isNaN(date.getTime()) ? "Saved earlier" : date.toLocaleString();
        const copy = bevelBox("div", ["display:flex", "flex-direction:column", "gap:2px"], { theme });
        copy.append(
          bevelText("strong", ["display:block"], row.title || `Untitled ${labels.noun}`, { theme }),
          bevelText("time", ["display:block", `font-family:${FONT_MONO}`, "font-size:12px"], when, { theme, role: "mutedText" }),
        );
        const actions = bevelBox("div", ["display:flex", "align-items:center", "gap:8px"], { theme });
        const switchBtn = bevelButton("Switch", {
          small: true,
          theme,
          onClick: () => {
            const current = captureSnapshot("draft");
            const remaining = readPreviousDrafts().filter((entry) => entry.id !== row.id);
            writeJson(listKey(), [current, ...remaining].slice(0, MAX_PREVIOUS_DRAFTS));
            applySnapshot(row);
            syncPreviousItem();
            modal.close();
          },
        });
        const removeBtn = bevelButton("Remove", {
          small: true,
          theme,
          onClick: () => {
            const remaining = readPreviousDrafts().filter((entry) => entry.id !== row.id);
            writeJson(listKey(), remaining);
            syncPreviousItem();
            draw();
          },
        });
        actions.append(switchBtn, removeBtn);
        item.append(copy, actions);
        list.append(item);
      }

      const starter = readJson(starterKey(), null) || bootStarter;
      if (starter) {
        const starterItem = bevelBox(
          "li",
          ["display:flex", "align-items:center", "justify-content:space-between", "gap:12px", "padding:10px 12px", `border:1px dashed ${theme.border}`, `border-radius:${theme.radius}`],
          { theme },
        );
        const starterCopy = bevelBox("div", ["display:flex", "flex-direction:column", "gap:2px"], { theme });
        starterCopy.append(
          bevelText("strong", ["display:block"], `${starter.title} (Starter example)`, { theme }),
          bevelText("span", ["display:block", `font-family:${FONT_MONO}`, "font-size:12px"], "Original populated demo", { theme, role: "mutedText" }),
        );
        const restoreBtn = bevelButton("Restore", {
          small: true,
          theme,
          onClick: () => {
            const current = captureSnapshot("draft");
            writeJson(listKey(), [current, ...readPreviousDrafts()].slice(0, MAX_PREVIOUS_DRAFTS));
            applySnapshot(starter);
            syncPreviousItem();
            modal.close();
          },
        });
        starterItem.append(starterCopy, restoreBtn);
        list.append(starterItem);
      }
    }

    draw();
    modal.title = `Previous ${labels.plural}`;
    modal.html = box;
    modal.width = "560px";
    modal[SURFACE_THEME] = theme;
    modal.closeHtml = "x";
    modal.no = "Close";
    modal.onNo(() => goo?.focus());
    modal.open();
    return box;
  }

  function duplicateVisitorDraft() {
    ensureStarterSaved();
    try { window.clay?.visitorDrafts?.saveNow?.(); } catch {}
    const current = captureSnapshot("draft");
    const list = [current, ...readPreviousDrafts()].slice(0, MAX_PREVIOUS_DRAFTS);
    writeJson(listKey(), list);
    try { window.clay?.visitorDrafts?.saveNow?.(); } catch {}
    syncPreviousItem();
  }

  addItem({
    id: "clay.new",
    label: labels.newLabel,
    run: startNewVisitorDraft,
  });
  addItem({
    id: "clay.duplicate",
    label: labels.duplicateLabel,
    run: duplicateVisitorDraft,
  });
  syncPreviousItem();
}
