import { a1, colName, tabTable } from "./commit.js";
import { appendBlankTab } from "./format-commands.js";
import { mountSheetDialog } from "./menu.js";
import { createSelection, ownerOf, selectionBox } from "./selection.js";
import { bevelBox, bevelButton, bevelText, setShown } from "../ui/bevel-controls.js";
import { bevelDialog } from "../ui/bevel-dialog.js";
import { FONT_MONO, FONT_SANS, TOKENS } from "../ui/bevel.js";
import { resolveTheme } from "../ui/theme.js";

export const MAX_IMPORT_BYTES = 5 * 1024 * 1024;
export const MAX_IMPORT_CELLS = 200_000;
export const PREVIEW_ROW_LIMIT = 20;
export const CSV_FILE_ACCEPT = ".csv,.tsv,.txt,text/csv,text/tab-separated-values";
export const MAX_XLSX_BYTES = 20 * 1024 * 1024;
export const XLSX_FILE_ACCEPT = ".xlsx,application/vnd.openxmlformats-officedocument.spreadsheetml.sheet";

const DELIMITER_CHOICES = [
  { value: ",", label: "Comma (,)" },
  { value: ";", label: "Semicolon (;)" },
  { value: "\t", label: "Tab" },
];

export function utf8ByteLength(text) {
  const str = String(text ?? "");
  if (typeof TextEncoder === "function") return new TextEncoder().encode(str).byteLength;
  return str.length;
}

export function countDelimitedCells(text, delimiter = ",", limit = MAX_IMPORT_CELLS) {
  const src = String(text ?? "").replace(/^\uFEFF/, "");
  if (!src) return 0;
  let rows = 0;
  let maxCols = 0;
  let total = 0;
  let rowCols = 1;
  let quoted = false;
  let fieldEmpty = true;

  for (let i = 0; i < src.length; i++) {
    const ch = src[i];
    if (quoted) {
      if (ch === '"') {
        if (src[i + 1] === '"') i += 1;
        else quoted = false;
      }
      continue;
    }
    if (ch === '"' && fieldEmpty) {
      quoted = true;
      fieldEmpty = false;
    } else if (ch === delimiter) {
      rowCols += 1;
      fieldEmpty = true;
    } else if (ch === "\r" || ch === "\n") {
      if (ch === "\r" && src[i + 1] === "\n") i += 1;
      rows += 1;
      if (rowCols > maxCols) maxCols = rowCols;
      total += rowCols;
      if (total > limit || rows * maxCols > limit) return Math.max(total, rows * maxCols);
      rowCols = 1;
      fieldEmpty = true;
    } else {
      fieldEmpty = false;
    }
  }

  if (rowCols > 1 || !fieldEmpty || !/[\r\n]$/.test(src)) {
    rows += 1;
    if (rowCols > maxCols) maxCols = rowCols;
    total += rowCols;
  }
  return Math.max(total, rows * maxCols);
}

export function inspectDelimitedText(text, { byteLength = null, delimiter = undefined, csv } = {}) {
  const str = String(text ?? "");
  const bytes = typeof byteLength === "number" ? Math.max(byteLength, utf8ByteLength(str)) : utf8ByteLength(str);
  if (bytes > MAX_IMPORT_BYTES) {
    return {
      ok: false,
      reason: "too_many_bytes",
      message: "This file is over 5 MB and cannot be imported.",
    };
  }
  const detected = delimiter ?? csv.detectDelimiter(str);
  const cells = countDelimitedCells(str, detected, MAX_IMPORT_CELLS);
  if (cells > MAX_IMPORT_CELLS) {
    return {
      ok: false,
      reason: "too_many_cells",
      message: "This file has over 200,000 cells and cannot be imported.",
    };
  }
  let rows;
  try {
    rows = csv.parseDelimited(str, { delimiter: detected });
  } catch (error) {
    if (error?.name === "DelimitedError") {
      return {
        ok: false,
        reason: "parse_error",
        line: error.line,
        message: `Could not parse delimited text on line ${error.line}.`,
      };
    }
    throw error;
  }
  const colCount = rows.reduce((widest, row) => Math.max(widest, row.length), 0);
  if (rows.length * colCount > MAX_IMPORT_CELLS) {
    return {
      ok: false,
      reason: "too_many_cells",
      message: "This file has over 200,000 cells and cannot be imported.",
    };
  }
  return {
    ok: true,
    delimiter: detected,
    rows,
    previewRows: rows.slice(0, PREVIEW_ROW_LIMIT),
    colCount,
  };
}

export function freeImportTabName(fileName, existingTabs = []) {
  const leaf = String(fileName ?? "").split(/[/\\]/).pop() ?? "";
  const withoutExt = leaf.replace(/\.[^.]+$/, "");
  const base = withoutExt.replace(/[\r\n\t\0]+/g, " ").trim().slice(0, 80) || "Imported";
  const taken = new Set(existingTabs.map((t) => (t.name ?? "").toLowerCase()));
  if (!taken.has(base.toLowerCase())) return base;
  const stem = base.slice(0, 72);
  for (let n = 2; ; n++) {
    const candidate = `${stem} ${n}`;
    if (!taken.has(candidate.toLowerCase())) return candidate;
  }
}

export function exportTabDelimited(grid, tabName, delimiter = ",", { doc = document, urlApi = globalThis.URL } = {}) {
  const text = grid.toDelimited(undefined, { delimiter, values: "shown", protect: true });
  const isTsv = delimiter === "\t";
  const ext = isTsv ? "tsv" : "csv";
  const mime = isTsv ? "text/tab-separated-values;charset=utf-8" : "text/csv;charset=utf-8";
  const safeBase = String(tabName ?? "").replace(/[/\\\0]/g, "-").trim() || "Sheet";
  const filename = `${safeBase}.${ext}`;
  const blob = new Blob([text], { type: mime });
  const href = urlApi.createObjectURL(blob);
  const link = doc.createElement("a");
  link.href = href;
  link.download = filename;
  link.setAttribute("sap-ignore", "");
  link.setAttribute("clay", "no-save no-watch no-snapshot");
  link.style.display = "none";
  doc.body.append(link);
  try {
    link.click();
  } finally {
    link.remove();
    setTimeout(() => urlApi.revokeObjectURL(href), 40000)?.unref?.();
  }
  return { filename, text, mime, blob };
}

export const importCsvIntoGrid = (grid, text, at, { delimiter, types }) =>
  grid.importDelimited(text, at, { delimiter, types, formulas: false });

export function importExportMenuItems({ onImportPick, onImportXlsx, onExportCsv, onExportTsv, onExportXlsx }) {
  return [
    { id: "import-delimited", label: "Import CSV or TSV…", onSelect: onImportPick },
    ...(onImportXlsx ? [{ id: "import-xlsx", label: "Import Excel file (.xlsx)…", onSelect: onImportXlsx }] : []),
    { type: "separator" },
    { id: "export-csv", label: "Download this tab as CSV", onSelect: onExportCsv },
    { id: "export-tsv", label: "Download this tab as TSV", onSelect: onExportTsv },
    ...(onExportXlsx ? [{ id: "export-xlsx", label: "Download all tabs as Excel file (.xlsx)", onSelect: onExportXlsx }] : []),
  ];
}

function readFileBytes(file) {
  if (typeof file?.arrayBuffer === "function") return file.arrayBuffer();
  return new Promise((resolve, reject) => {
    const reader = new FileReader();
    reader.onload = () => resolve(reader.result ?? new ArrayBuffer(0));
    reader.onerror = () => reject(reader.error ?? new Error("Could not read file."));
    reader.readAsArrayBuffer(file);
  });
}

function decodeFileBytes(buffer) {
  const bytes = buffer instanceof Uint8Array ? buffer : new Uint8Array(buffer);
  if (typeof TextDecoder !== "function") return String.fromCharCode(...bytes);
  try {
    return new TextDecoder("utf-8", { fatal: true }).decode(bytes);
  } catch {
    return new TextDecoder("windows-1252").decode(bytes);
  }
}

// The picker opens inside the click; the .xlsx tools load only once a file is chosen.
export function openXlsxFilePicker({ doc = document, container, onPickFile, onNotice }) {
  container?.querySelector?.("[data-xlsx-file-input]")?.remove();
  const input = doc.createElement("input");
  input.type = "file";
  input.accept = XLSX_FILE_ACCEPT;
  input.hidden = true;
  input.setAttribute("data-xlsx-file-input", "");
  input.addEventListener("change", async () => {
    const file = input.files?.[0] ?? null;
    input.remove();
    if (!file) return;
    if (typeof file.size === "number" && file.size > MAX_XLSX_BYTES) {
      onNotice?.("This file is over 20 MB and cannot be imported.");
      return;
    }
    try {
      onPickFile?.({ name: file.name || "Imported.xlsx", bytes: new Uint8Array(await readFileBytes(file)) });
    } catch {
      onNotice?.("Could not read the selected file.");
    }
  });
  (container ?? doc.body).append(input);
  input.click();
  return input;
}

async function readFileText(file) {
  return decodeFileBytes(await readFileBytes(file));
}

export function openCsvFilePicker({ doc = document, container, onPickFile, onNotice }) {
  container?.querySelector?.("[data-csv-file-input]")?.remove();
  const input = doc.createElement("input");
  input.type = "file";
  input.accept = CSV_FILE_ACCEPT;
  input.hidden = true;
  input.setAttribute("data-csv-file-input", "");

  input.addEventListener("change", async () => {
    const file = input.files?.[0] ?? null;
    input.remove();
    if (!file) return;
    if (typeof file.size === "number" && file.size > MAX_IMPORT_BYTES) {
      onNotice?.("This file is over 5 MB and cannot be imported.");
      return;
    }
    try {
      const text = await readFileText(file);
      onPickFile?.({ name: file.name || "Imported.csv", size: file.size, text });
    } catch {
      onNotice?.("Could not read the selected file.");
    }
  });

  (container ?? doc.body).append(input);
  input.click();
  return input;
}

const selectCss = () =>
  [
    "height:26px",
    "padding:0 6px",
    `background:${TOKENS.sunk}`,
    `color:${TOKENS.ink}`,
    `border:1px solid ${TOKENS["line-2"]}`,
    "border-radius:0",
    `font:500 12px/1.2 ${FONT_SANS}`,
    "cursor:pointer",
  ].join(";");

export function openImportDialog({
  doc = document,
  theme = resolveTheme(),
  container = null,
  fileName = "Imported.csv",
  text = "",
  byteLength = null,
  initialInspection = null,
  csv,
  existingTabs = [],
  getReturnFocus,
  onConfirm,
  onCancel,
}) {
  let state = initialInspection ?? inspectDelimitedText(text, { byteLength, csv });
  const tabName = freeImportTabName(fileName, existingTabs);
  let destination = "new";
  let colTypes = Array.from({ length: state.ok ? state.colCount : 0 }, () => "auto");
  let handle = null;

  const shell = bevelDialog({ zIndex: "2147483001", width: "620px", closable: true, titled: true, theme });
  const stack = bevelBox("div", ["display:flex", "flex-direction:column", "gap:12px"], { theme });

  const controlsRow = bevelBox("div", [
    "display:flex",
    "flex-wrap:wrap",
    "align-items:center",
    "justify-content:space-between",
    "gap:12px",
  ], { theme });

  const delimGroup = bevelBox("label", [
    "display:inline-flex",
    "align-items:center",
    "gap:6px",
    `font:500 12.5px/1.3 ${FONT_SANS}`,
    `color:${TOKENS["ink-2"]}`,
  ], { theme });
  delimGroup.append(bevelText("span", ["font:inherit", "color:inherit"], "Separator", { theme, role: null }));
  const delimSelect = doc.createElement("select");
  delimSelect.setAttribute("data-import-delimiter", "");
  delimSelect.setAttribute("aria-label", "Delimiter");
  delimSelect.style.cssText = selectCss();
  for (const opt of DELIMITER_CHOICES) {
    const optionEl = doc.createElement("option");
    optionEl.value = opt.value;
    optionEl.textContent = opt.label;
    delimSelect.append(optionEl);
  }
  if (state.ok) delimSelect.value = state.delimiter;
  delimGroup.append(delimSelect);

  const destGroup = bevelBox("div", ["display:inline-flex", "align-items:center", "gap:6px"], { theme });
  destGroup.setAttribute("role", "radiogroup");
  destGroup.setAttribute("aria-label", "Import location");

  const destDefs = [
    { id: "new", label: `New tab (${tabName})` },
    { id: "cell", label: "At the selected cell" },
  ];
  const destButtons = [];
  const paintDest = (nextDest) => {
    destination = nextDest;
    for (const entry of destButtons) {
      const checked = entry.id === nextDest;
      entry.btn.setAttribute("aria-checked", checked ? "true" : "false");
      entry.btn.pin({
        border: checked ? `2px solid ${TOKENS.brass}` : `1px solid ${TOKENS["line-2"]}`,
        "font-weight": checked ? "600" : "500",
      });
    }
  };
  destDefs.forEach((def, idx) => {
    const btn = bevelButton(def.label, {
      small: true,
      variant: "quiet",
      theme,
      onClick: () => paintDest(def.id),
      extra: [
        "height:26px",
        "padding:0 10px",
        `background:${TOKENS.sunk}`,
        `border:1px solid ${TOKENS["line-2"]}`,
        `font:500 12px/1.2 ${FONT_SANS}`,
        `color:${TOKENS.ink}`,
      ],
    });
    btn.setAttribute("role", "radio");
    btn.setAttribute("data-import-dest", def.id);
    btn.setAttribute("aria-checked", idx === 0 ? "true" : "false");
    btn.addEventListener("keydown", (event) => {
      if (event.key === "ArrowLeft" || event.key === "ArrowRight") {
        event.preventDefault();
        event.stopPropagation();
        const other = destButtons[(idx + 1) % destButtons.length];
        paintDest(other.id);
        other.btn.focus();
      }
    });
    destButtons.push({ id: def.id, btn });
    destGroup.append(btn);
  });
  paintDest(destination);

  controlsRow.append(delimGroup, destGroup);
  stack.append(controlsRow);

  const errorBox = bevelText("div", [
    "display:none",
    "padding:6px 8px",
    `background:${TOKENS["ox-soft"]}`,
    `border:1px solid ${TOKENS.ox}`,
    `font:500 12.5px/1.4 ${FONT_SANS}`,
    `color:${TOKENS.ox}`,
  ], "", { theme });
  errorBox.setAttribute("data-import-error", "");
  errorBox.hidden = true;
  stack.append(errorBox);

  const previewWrap = bevelBox("div", [
    "max-height:240px",
    "overflow:auto",
    `background:${TOKENS.surface}`,
    `border:1px solid ${TOKENS["line-2"]}`,
  ], { theme });
  previewWrap.setAttribute("data-import-preview", "");
  stack.append(previewWrap);

  function renderPreview() {
    previewWrap.textContent = "";
    if (!state.ok) {
      errorBox.textContent = state.message;
      setShown(errorBox, true, "block");
      setShown(previewWrap, false, "block");
      handle?.confirmBtn.setDisabled(true);
      return;
    }
    errorBox.textContent = "";
    setShown(errorBox, false, "block");
    setShown(previewWrap, true, "block");
    handle?.confirmBtn.setDisabled(false);

    const cols = Math.max(1, state.colCount);
    while (colTypes.length < cols) colTypes.push("auto");

    const table = doc.createElement("table");
    table.style.cssText = `width:100%;border-collapse:collapse;font:12px/1.35 ${FONT_MONO};color:${TOKENS.ink}`;

    const thead = doc.createElement("thead");
    const headRow = doc.createElement("tr");
    for (let c = 0; c < cols; c++) {
      const th = doc.createElement("th");
      th.style.cssText = `padding:6px;background:${TOKENS.sunk};border-bottom:1px solid ${TOKENS["line-2"]};border-right:1px solid ${TOKENS.line};text-align:left;vertical-align:top`;
      const letter = bevelText("div", [`font:600 11px/1.2 ${FONT_MONO}`, `color:${TOKENS.muted}`, "margin-bottom:4px"], colName(c), { theme, role: "mutedText" });
      const typeSelect = doc.createElement("select");
      typeSelect.setAttribute("data-col-type", String(c));
      typeSelect.setAttribute("aria-label", `Column ${colName(c)} type`);
      typeSelect.style.cssText = selectCss();
      for (const [val, label] of [["auto", "Automatic"], ["text", "Plain text"]]) {
        const opt = doc.createElement("option");
        opt.value = val;
        opt.textContent = label;
        typeSelect.append(opt);
      }
      typeSelect.value = colTypes[c] ?? "auto";
      typeSelect.addEventListener("change", () => {
        colTypes[c] = typeSelect.value === "text" ? "text" : "auto";
      });
      th.append(letter, typeSelect);
      headRow.append(th);
    }
    thead.append(headRow);
    table.append(thead);

    const tbody = doc.createElement("tbody");
    for (const row of state.previewRows) {
      const tr = doc.createElement("tr");
      for (let c = 0; c < cols; c++) {
        const td = doc.createElement("td");
        td.style.cssText = `padding:4px 6px;border-bottom:1px solid ${TOKENS.line};border-right:1px solid ${TOKENS.line};white-space:pre;max-width:180px;overflow:hidden;text-overflow:ellipsis`;
        td.textContent = row[c] ?? "";
        tr.append(td);
      }
      tbody.append(tr);
    }
    table.append(tbody);
    previewWrap.append(table);
  }

  delimSelect.addEventListener("change", () => {
    state = inspectDelimitedText(text, { byteLength, delimiter: delimSelect.value, csv });
    renderPreview();
  });

  shell.body.append(stack);

  handle = mountSheetDialog(doc, shell, {
    title: `Import ${fileName}`,
    confirmLabel: "Import",
    confirmVariant: "primary",
    onConfirm: () => {
      if (!state.ok) return;
      onConfirm?.({
        delimiter: state.delimiter,
        types: colTypes.slice(0, state.colCount),
        destination,
        tabName,
        rowCount: state.rows.length,
        colCount: state.colCount,
      });
    },
    onCancel,
    getReturnFocus,
    container,
  });

  renderPreview();
  handle.confirmBtn.focus();
  return handle;
}

export function openCsvMenu(ctx, menu) {
  const { doc, win, workbook, chrome, editor, tabs } = ctx;

  function applyCsvImport(choice, text) {
    const { delimiter, types, destination, tabName, rowCount, colCount } = choice;
    if (destination === "new") {
      const added = appendBlankTab({
        tabs,
        sapGrid: win.Sap?.grid,
        gridOf: ctx.gridOf,
        runTabStep: ctx.runTabStep,
        customName: tabName,
        rowsCount: Math.max(100, rowCount),
        colsCount: Math.max(26, colCount),
      });
      if (!added) return;
      ctx.subscribeGrids();
      const newGrid = ctx.gridOf(tabTable(workbook, added.id));
      tabs.show(added.id);
      ctx.setSel(createSelection(ctx.viewOf(added.id), added.id));
      if (newGrid) {
        ctx.writeGrid(() => importCsvIntoGrid(newGrid, text, "A1", { delimiter, types }));
      }
      ctx.applySelection(createSelection(ctx.viewOf(added.id), added.id), { announce: true, enteredTab: true });
      editor.focus();
      return;
    }
    const sel = ctx.getSel();
    if (!sel) return;
    const grid = ctx.gridOf(tabTable(workbook, sel.tab));
    if (!grid) return;
    const before = ctx.viewOf(sel.tab);
    const box = selectionBox(before, sel);
    if (!box) return;
    if (box.top + rowCount - before.rows.length > 10000 || box.left + colCount - before.cols.length > 1000) {
      chrome.showNotice("This file is too large to place here. Import it into a new tab.");
      return;
    }
    const at = a1(box.top, box.left);
    if (!ctx.writeGrid(() => importCsvIntoGrid(grid, text, at, { delimiter, types }))) return;
    const nextView = ctx.viewOf(sel.tab);
    if (rowCount > 0 && colCount > 0 && nextView.rows.length > 0 && nextView.cols.length > 0) {
      const own = ownerOf(nextView, box.top, box.left);
      const bottom = Math.min(box.top + rowCount - 1, nextView.rows.length - 1);
      const right = Math.min(box.left + colCount - 1, nextView.cols.length - 1);
      ctx.applyMove({
        tab: sel.tab,
        anchor: { row: nextView.rows[own.r], col: nextView.cols[own.c] },
        focus: { row: nextView.rows[bottom], col: nextView.cols[right] },
      });
    } else {
      ctx.refreshStatus();
      ctx.syncUI({ announce: true });
    }
    editor.focus();
  }

  function handlePicked({ name, size, text }) {
    const csvApi = win.Sap?.grid?.csv;
    if (!csvApi) return;
    const inspected = inspectDelimitedText(text, { byteLength: size, csv: csvApi });
    if (!inspected.ok) {
      chrome.showNotice(inspected.message);
      return;
    }
    chrome.hideNotice();
    ctx.showDialog(
      ctx.getSel()?.tab ?? "",
      openImportDialog({
        doc,
        theme: chrome.theme,
        container: chrome.topRoot,
        fileName: name,
        text,
        byteLength: size,
        initialInspection: inspected,
        csv: csvApi,
        existingTabs: tabs.list(),
        getReturnFocus: () => editor.textarea,
        onConfirm: (choice) => {
          ctx.clearDialog();
          applyCsvImport(choice, text);
        },
        onCancel: () => ctx.clearDialog(),
      }),
    );
  }

  function runExport(delimiter) {
    const sel = ctx.getSel();
    if (!sel) return;
    const tab = tabs.list().find((t) => t.id === sel.tab);
    const grid = ctx.gridOf(tab?.table);
    if (!grid) return;
    try {
      exportTabDelimited(grid, tab.name, delimiter, { doc, urlApi: win.URL ?? globalThis.URL });
      chrome.hideNotice();
    } catch (error) {
      if (error?.name !== "GridRefused") throw error;
      chrome.showNotice(error.message || "Could not export this sheet.");
    }
  }

  menu.open({
    anchor: chrome.getButton("import"),
    label: "Import or export",
    items: importExportMenuItems({
      onImportPick: () =>
        openCsvFilePicker({
          doc,
          container: chrome.topRoot,
          onNotice: (msg) => chrome.showNotice(msg),
          onPickFile: handlePicked,
        }),
      onImportXlsx: ctx.xlsx
        ? () =>
            openXlsxFilePicker({
              doc,
              container: chrome.topRoot,
              onNotice: (msg) => chrome.showNotice(msg),
              onPickFile: (picked) => {
                chrome.showNotice("Reading the file...");
                ctx.xlsx((x) => x.importXlsxFile(ctx, picked));
              },
            })
        : undefined,
      onExportCsv: () => runExport(","),
      onExportTsv: () => runExport("\t"),
      onExportXlsx: ctx.xlsx ? () => ctx.xlsx((x) => x.downloadXlsx(ctx)) : undefined,
    }),
  });
}
