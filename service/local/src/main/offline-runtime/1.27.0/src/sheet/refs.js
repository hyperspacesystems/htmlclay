import { colName } from "./commit.js";

export const REF_PALETTE_SIZE = 4;

const MAX_COLS = 16384;
const isCoord = (token) => token?.type === "cell" || token?.type === "col" || token?.type === "row";

export function colIndexFromLetters(letters) {
  if (typeof letters !== "string" || !/^[A-Za-z]{1,3}$/.test(letters)) return -1;
  let n = 0;
  for (const ch of letters) n = n * 26 + (ch.toUpperCase().charCodeAt(0) - 64);
  return n > MAX_COLS ? -1 : n - 1;
}

export function quoteSheetName(name) {
  return /^[A-Za-z_][A-Za-z0-9_.]*$/.test(name) && !/^[A-Za-z]{1,3}[0-9]+$/.test(name)
    ? name
    : `'${String(name).split("'").join("''")}'`;
}

const getTokenize = (custom) => custom ?? globalThis.window?.Sap?.grid?.tokenize ?? null;

const COL_LIMIT = 18278; // ZZZ, the last column a reference can name

const colNumber = (letters) => [...letters].reduce((n, ch) => n * 26 + (ch.charCodeAt(0) - 64), 0);

function colLetters(n) {
  let out = "";
  for (let k = n; k > 0; k = Math.floor((k - 1) / 26)) out = String.fromCharCode(65 + ((k - 1) % 26)) + out;
  return out;
}

const WORD_PART = /[A-Za-z0-9_.'$]/;

// The formula `text` as it reads `dr` rows and `dc` columns away: every relative row and column of
// a reference moves, an absolute one ($) stays, and a reference moved off the sheet becomes #REF!.
// A bare column or row word moves only as one end of a range (A:C, 2:5), so a function name being
// typed is never taken for a column. Text that is not a formula comes back as it is.
export function shiftFormula(text, dr, dc, customTokenize) {
  if (dr === 0 && dc === 0) return text;
  return rewriteRefs(text, customTokenize, (t) => ({
    col: t.col === undefined ? null : colNumber(t.col) + (t.absCol ? 0 : dc),
    row: t.row === undefined ? null : t.row + (t.absRow ? 0 : dr),
  }));
}

// The formula of the cell at `src` (0-based { r, c }) as it reads pasted transposed at `dst`: a
// relative cell reference keeps its distance from the formula with rows and columns swapped, as
// Excel and Sheets transpose. A whole-column or whole-row range only moves, as shiftFormula does.
export function transposeFormula(text, src, dst, customTokenize) {
  const dr = dst.r - src.r;
  const dc = dst.c - src.c;
  return rewriteRefs(text, customTokenize, (t) => {
    if (t.col === undefined || t.row === undefined) {
      return {
        col: t.col === undefined ? null : colNumber(t.col) + (t.absCol ? 0 : dc),
        row: t.row === undefined ? null : t.row + (t.absRow ? 0 : dr),
      };
    }
    return {
      col: t.absCol ? colNumber(t.col) : dst.c + 1 + (t.row - 1 - src.r),
      row: t.absRow ? t.row : dst.r + 1 + (colNumber(t.col) - 1 - src.c),
    };
  });
}

function rewriteRefs(text, customTokenize, place) {
  const tokenize = getTokenize(customTokenize);
  if (typeof text !== "string" || !text.startsWith("=") || typeof tokenize !== "function") return text;
  const tokens = tokenize(text);
  const colon = (t) => t?.type === "punct" && t.value === ":";
  const whole = (t) =>
    t && !WORD_PART.test(text[t.start - 1] ?? "") && !WORD_PART.test(text[t.end] ?? "") && (t.row === undefined || Number.isInteger(t.row));
  const okEnd = (t, want) => whole(t) && (t.type === want || (want === "col" && t.type === "cell"));
  const paired = (i) =>
    (colon(tokens[i - 1]) && okEnd(tokens[i - 2], tokens[i].type)) || (colon(tokens[i + 1]) && whole(tokens[i + 2]) && tokens[i + 2].type === tokens[i].type);
  let out = "";
  let at = 0;
  tokens.forEach((t, i) => {
    if (!whole(t) || (t.type !== "cell" && !((t.type === "col" || t.type === "row") && paired(i)))) return;
    const src = text.slice(t.start, t.end);
    const bang = src.lastIndexOf("!");
    const prefix = bang === -1 ? "" : src.slice(0, bang + 1);
    const { col, row } = place(t);
    const off = (col !== null && (col < 1 || col > COL_LIMIT)) || (row !== null && row < 1);
    const coord = (col === null ? "" : (t.absCol ? "$" : "") + colLetters(col)) + (row === null ? "" : (t.absRow ? "$" : "") + row);
    out += text.slice(at, t.start) + (off ? "#REF!" : prefix + coord);
    at = t.end;
  });
  return out + text.slice(at);
}

function resolveTabId(sheetName, activeTab, tabs) {
  if (sheetName === null || sheetName === undefined) return activeTab;
  const low = sheetName.toLowerCase();
  return tabs.find((tab) => tab.name.toLowerCase() === low)?.id ?? null;
}

function coordBounds(first, second) {
  if (!second) {
    const c = colIndexFromLetters(first.col);
    const r = first.row - 1;
    if (c < 0 || r < 0) return null;
    return { r0: r, c0: c, r1: r, c1: c };
  }
  const c0 = first.col ? colIndexFromLetters(first.col) : null;
  const c1 = second.col ? colIndexFromLetters(second.col) : null;
  if ((first.col && c0 < 0) || (second.col && c1 < 0)) return null;
  const r0 = first.row != null ? first.row - 1 : null;
  const r1 = second.row != null ? second.row - 1 : null;
  if ((r0 !== null && r0 < 0) || (r1 !== null && r1 < 0)) return null;
  return {
    r0: r0 === null ? null : Math.min(r0, r1),
    c0: c0 === null ? null : Math.min(c0, c1),
    r1: r0 === null ? null : Math.max(r0, r1),
    c1: c0 === null ? null : Math.max(c0, c1),
  };
}

export function formulaRefs(text, { activeTab = null, tabs = [], tokenize: customTokenize } = {}) {
  if (typeof text !== "string" || !text.startsWith("=") || text.length <= 1) return [];
  const tokenize = getTokenize(customTokenize);
  if (typeof tokenize !== "function") return [];
  const tokens = tokenize(text);
  const refs = [];

  for (let i = 1; i < tokens.length; i++) {
    const first = tokens[i];
    const colon = tokens[i + 1];
    const second = tokens[i + 2];
    if (
      isCoord(first) &&
      colon?.type === "punct" &&
      colon.value === ":" &&
      isCoord(second) &&
      (first.type === "col") === (second.type === "col") &&
      (first.type === "row") === (second.type === "row") &&
      !(first.sheet && second.sheet && first.sheet !== second.sheet)
    ) {
      const area = coordBounds(first, second);
      if (area) {
        const sheet = first.sheet ?? second.sheet ?? null;
        refs.push({
          span: [first.start, second.end],
          sheet,
          tab: resolveTabId(sheet, activeTab, tabs),
          area,
          from: first,
          to: second,
          colorIndex: refs.length % REF_PALETTE_SIZE,
        });
      }
      i += 2;
      continue;
    }
    if (first?.type === "cell") {
      const area = coordBounds(first, null);
      if (area) {
        const sheet = first.sheet ?? null;
        refs.push({
          span: [first.start, first.end],
          sheet,
          tab: resolveTabId(sheet, activeTab, tabs),
          area,
          from: first,
          to: null,
          colorIndex: refs.length % REF_PALETTE_SIZE,
        });
      }
    }
  }
  return refs;
}

export function canInsertRef(text, caret, end = caret, { tokenize: customTokenize } = {}) {
  if (typeof text !== "string" || !text.startsWith("=") || caret < 1 || end < caret || end > text.length) return false;
  const tokenize = getTokenize(customTokenize);
  if (typeof tokenize !== "function") return false;
  const rest = text.slice(end).trimStart();
  if (rest && !/^[),:%+\-*/^&=<>]/.test(rest)) return false;
  const last = tokenize(text.slice(0, caret)).at(-1);
  return Boolean(last && (last.type === "op" || (last.type === "punct" && (last.value === "(" || last.value === "," || (last.value === ":" && rest[0] !== ":")))));
}

function nextAbsState(absCol, absRow) {
  if (!absCol && !absRow) return { absCol: true, absRow: true };
  if (absCol && absRow) return { absCol: false, absRow: true };
  if (!absCol && absRow) return { absCol: true, absRow: false };
  return { absCol: false, absRow: false };
}

const printCoord = (token, { absCol, absRow }) => {
  const col = token.col ? `${absCol ? "$" : ""}${token.col}` : "";
  const row = token.row != null ? `${absRow ? "$" : ""}${token.row}` : "";
  return `${col}${row}`;
};

export function cycleRefAtCaret(text, caret, { tokenize: customTokenize } = {}) {
  const refs = formulaRefs(text, { tokenize: customTokenize });
  const hit = refs.find((ref) => caret >= ref.span[0] && caret <= ref.span[1]);
  if (!hit || hit.from.type !== "cell") return null;
  const full = text.slice(hit.span[0], hit.span[1]);
  const bangAt = hit.sheet ? full.lastIndexOf("!") : -1;
  const prefix = bangAt === -1 ? "" : full.slice(0, bangAt + 1);
  const next = nextAbsState(Boolean(hit.from.absCol), Boolean(hit.from.absRow));
  const body = hit.to
    ? `${printCoord(hit.from, next)}:${printCoord(hit.to, next)}`
    : printCoord(hit.from, next);
  const replaced = `${prefix}${body}`;
  const updated = text.slice(0, hit.span[0]) + replaced + text.slice(hit.span[1]);
  const end = hit.span[0] + replaced.length;
  return { text: updated, start: hit.span[0], end, caret: end };
}

export function formatPickedRef({ tab, homeTab, tabName, anchor, focus }) {
  const prefix = tab === homeTab ? "" : `${quoteSheetName(tabName)}!`;
  const r0 = Math.min(anchor.r, focus.r);
  const c0 = Math.min(anchor.c, focus.c);
  const r1 = Math.max(anchor.r, focus.r);
  const c1 = Math.max(anchor.c, focus.c);
  const coord = r0 === r1 && c0 === c1 ? `${colName(c0)}${r0 + 1}` : `${colName(c0)}${r0 + 1}:${colName(c1)}${r1 + 1}`;
  return `${prefix}${coord}`;
}
