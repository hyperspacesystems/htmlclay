import { a1, colIndex, rowIndex, tabTable } from "./commit.js";
import { shiftFormula } from "./refs.js";
import { isFilled, ownerOf, selectionBox } from "./selection.js";

const STYLE_PROPS = [
  "font-weight",
  "font-style",
  "text-decoration",
  "color",
  "background-color",
  "text-align",
  "white-space",
  "border-top",
  "border-right",
  "border-bottom",
  "border-left",
];

const STYLE_SET = new Set(STYLE_PROPS);

export const FILL_CUT_MESSAGE =
  "Fill can't cover only part of a merged cell. Unmerge it, or fill a range that holds all of it.";

export function sourceStyleMap(css) {
  const out = Object.create(null);
  for (const prop of STYLE_PROPS) out[prop] = null;
  if (typeof css !== "string" || !css) return out;
  for (const decl of css.split(";")) {
    const colon = decl.indexOf(":");
    if (colon === -1) continue;
    const key = decl.slice(0, colon).trim().toLowerCase();
    const val = decl.slice(colon + 1).trim().replace(/\s+/g, " ");
    if (!key || !val) continue;
    const normKey = key === "background" ? "background-color" : key;
    if (key === "border") {
      out["border-top"] = val;
      out["border-right"] = val;
      out["border-bottom"] = val;
      out["border-left"] = val;
    } else if (STYLE_SET.has(normKey)) {
      out[normKey] = val;
    }
  }
  return out;
}

function fallbackFmtKind(fmt) {
  if (!fmt) return "auto";
  if (fmt === "@") return "text";
  if (/^(?:m\/d\/yyyy|yyyy-mm-dd|mmm d(?:, yyyy)?|mmmm d, yyyy)$/i.test(fmt)) return "date";
  if (/^m\/d\/yyyy h:mm(?::ss)?$/i.test(fmt)) return "datetime";
  return "unknown";
}

const sigDiff = (a, b) => Number((a - b).toPrecision(10));

function decimalPlacesOf(n) {
  const s = String(n);
  if (/[eE]/.test(s)) return null;
  const dot = s.indexOf(".");
  return dot === -1 ? 0 : s.length - dot - 1;
}

function makeStepEvaluator(values, step) {
  const places = [...values, step].map(decimalPlacesOf);
  const maxPlaces = places.every((p) => p !== null) ? Math.min(12, Math.max(0, ...places)) : null;
  const base = values[0];
  return (pos) => {
    const raw = base + pos * step;
    if (!Number.isFinite(raw)) return base;
    if (maxPlaces !== null) {
      const fixed = Number(raw.toFixed(maxPlaces));
      return Object.is(fixed, -0) ? 0 : fixed;
    }
    const prec = Number(raw.toPrecision(12));
    return Object.is(prec, -0) ? 0 : prec;
  };
}

function hasConstantNumericStep(values) {
  if (values.length < 2) return false;
  const first = sigDiff(values[1], values[0]);
  for (let i = 2; i < values.length; i++) {
    if (sigDiff(values[i], values[i - 1]) !== first) return false;
  }
  return true;
}

function parseTrailingWholeNumber(text) {
  if (typeof text !== "string" || !text) return null;
  const match = /^(.*[^\d]|)(\d+)$/s.exec(text);
  if (!match) return null;
  const [, prefix, digits] = match;
  if (digits.length > 15) return null;
  const value = Number(digits);
  if (!Number.isSafeInteger(value)) return null;
  return { prefix, digits, value };
}

function detectSeries(sourceCells, fmtKind) {
  const m = sourceCells.length;
  if (m === 0) return null;

  const kinds = sourceCells.map((c) => fmtKind(c?.effectiveFmt ?? c?.fmt ?? null));
  const isDate = (k) => k === "date" || k === "datetime";
  if ((kinds.some(isDate) && !kinds.every(isDate)) || (kinds.includes("time") && !kinds.every((k) => k === "time"))) {
    return null;
  }

  const allDateNums = sourceCells.every((c) => {
    if (typeof c?.content !== "number" || !Number.isFinite(c.content)) return false;
    const kind = fmtKind(c.effectiveFmt ?? c.fmt ?? null);
    return kind === "date" || kind === "datetime";
  });
  if (allDateNums) {
    const nums = sourceCells.map((c) => c.content);
    if (m === 1) return makeStepEvaluator(nums, 1);
    if (hasConstantNumericStep(nums)) return makeStepEvaluator(nums, sigDiff(nums[1], nums[0]));
    return null;
  }

  if (m >= 2 && sourceCells.every((c) => typeof c?.content === "number" && Number.isFinite(c.content))) {
    const nums = sourceCells.map((c) => c.content);
    if (hasConstantNumericStep(nums)) return makeStepEvaluator(nums, sigDiff(nums[1], nums[0]));
    return null;
  }

  const parsedText = sourceCells.map((c) => parseTrailingWholeNumber(c?.content));
  if (parsedText.every(Boolean)) {
    const prefix = parsedText[0].prefix;
    if (parsedText.every((p) => p.prefix === prefix)) {
      const nums = parsedText.map((p) => p.value);
      let step = null;
      if (m === 1) {
        step = 1;
      } else {
        const s0 = nums[1] - nums[0];
        if (nums.every((v, i) => i === 0 || v - nums[i - 1] === s0)) step = s0;
      }
      if (step !== null) {
        const zeroPadded = parsedText.filter((p) => p.digits.length > 1 && p.digits[0] === "0");
        const padWidth = zeroPadded.length > 0 ? Math.max(...zeroPadded.map((p) => p.digits.length)) : 1;
        return (pos) => {
          const next = nums[0] + pos * step;
          if (next < 0) return `${prefix}-${String(-next).padStart(padWidth, "0")}`;
          return `${prefix}${String(next).padStart(padWidth, "0")}`;
        };
      }
    }
  }

  return null;
}

export function fillLine(
  sourceCells,
  count,
  direction = 1,
  { axis = "vertical", fmtKind = fallbackFmtKind, tokenize, series = true } = {},
) {
  const m = sourceCells.length;
  if (m === 0 || count <= 0) return [];
  const stepSeries = series ? detectSeries(sourceCells, fmtKind) : null;
  const styles = sourceCells.map((src) => sourceStyleMap(src?.style));
  const fmts = sourceCells.map((src) => (src?.effectiveFmt !== undefined ? src.effectiveFmt : (src?.fmt ?? null)));
  const out = [];

  for (let t = 0; t < count; t++) {
    const pos = direction >= 0 ? m + t : t - count;
    const s = ((pos % m) + m) % m;
    const src = sourceCells[s];
    let content;
    if (stepSeries) {
      content = stepSeries(pos);
    } else if (src?.content !== null && typeof src?.content === "object" && typeof src.content.f === "string") {
      const delta = pos - s;
      const dr = axis === "vertical" ? delta : 0;
      const dc = axis === "horizontal" ? delta : 0;
      content = { f: shiftFormula(src.content.f, dr, dc, tokenize) };
    } else {
      content = src?.content ?? null;
    }
    out.push({ content, fmt: fmts[s], style: { ...styles[s] } });
  }
  return out;
}

function cutCoveredCell(view, targetBox) {
  for (let r = targetBox.top; r <= targetBox.bottom; r++) {
    for (let c = targetBox.left; c <= targetBox.right; c++) {
      const own = ownerOf(view, r, c);
      if (own.rows <= 1 && own.cols <= 1) continue;
      const mBottom = own.r + own.rows - 1;
      const mRight = own.c + own.cols - 1;
      const whole =
        own.r >= targetBox.top &&
        mBottom <= targetBox.bottom &&
        own.c >= targetBox.left &&
        mRight <= targetBox.right;
      if (whole) continue;
      if (r !== own.r || c !== own.c) return { r, c };
      return own.rows > 1 ? { r: own.r + 1, c: own.c } : { r: own.r, c: own.c + 1 };
    }
  }
  return null;
}

export function buildFillWrite(
  grid,
  view,
  sourceBox,
  targetBox,
  { fmtKind = fallbackFmtKind, tokenize, series = true } = {},
) {
  if (cutCoveredCell(view, targetBox)) return { cut: true };

  const vertical = targetBox.top > sourceBox.bottom || targetBox.bottom < sourceBox.top;
  const direction = vertical
    ? (targetBox.top > sourceBox.bottom ? 1 : -1)
    : (targetBox.left > sourceBox.right ? 1 : -1);
  const rows = targetBox.bottom - targetBox.top + 1;
  const cols = targetBox.right - targetBox.left + 1;
  const block = Array.from({ length: rows }, () => Array.from({ length: cols }, () => undefined));

  if (vertical) {
    for (let dc = 0; dc < cols; dc++) {
      const c = sourceBox.left + dc;
      const src = [];
      for (let r = sourceBox.top; r <= sourceBox.bottom; r++) src.push(grid.cell(a1(r, c)));
      const line = fillLine(src, rows, direction, { axis: "vertical", fmtKind, tokenize, series });
      for (let dr = 0; dr < rows; dr++) {
        if (!view.cell(targetBox.top + dr, c)?.covered && !view.rowHidden?.(targetBox.top + dr)) block[dr][dc] = line[dr];
      }
    }
  } else {
    for (let dr = 0; dr < rows; dr++) {
      const r = sourceBox.top + dr;
      if (view.rowHidden?.(r)) continue;
      const src = [];
      for (let c = sourceBox.left; c <= sourceBox.right; c++) src.push(grid.cell(a1(r, c)));
      const line = fillLine(src, cols, direction, { axis: "horizontal", fmtKind, tokenize, series });
      for (let dc = 0; dc < cols; dc++) {
        if (!view.cell(r, targetBox.left + dc)?.covered) block[dr][dc] = line[dc];
      }
    }
  }

  return {
    at: a1(targetBox.top, targetBox.left),
    block,
    unionBox: {
      top: Math.min(sourceBox.top, targetBox.top),
      left: Math.min(sourceBox.left, targetBox.left),
      bottom: Math.max(sourceBox.bottom, targetBox.bottom),
      right: Math.max(sourceBox.right, targetBox.right),
    },
  };
}

export function executeFill(grid, view, sourceBox, targetBox, options = {}) {
  const write = buildFillWrite(grid, view, sourceBox, targetBox, options);
  if (write.cut) {
    throw Object.assign(new Error(FILL_CUT_MESSAGE), { name: "GridRefused", code: "covered_cell", detail: FILL_CUT_MESSAGE });
  }
  grid.setRange(write.at, write.block, { grow: true, detect: false });
  return write.unionBox;
}

function neighbourCol(view, from, step) {
  for (let c = from; c >= 0 && c < view.cols.length; c += step) {
    if (!view.colHidden?.(c)) return c;
  }
  return -1;
}

export function findAutoFillEndRow(view, sourceBox) {
  if (!view || !sourceBox) return -1;
  const start = sourceBox.bottom + 1;
  if (start >= view.rows.length) return -1;
  for (const c of [neighbourCol(view, sourceBox.left - 1, -1), neighbourCol(view, sourceBox.right + 1, 1)]) {
    if (c === -1 || !isFilled(view, start, c)) continue;
    let end = start;
    for (let r = start + 1; r < view.rows.length; r++) {
      if (view.rowHidden?.(r) || !isFilled(view, r, c)) break;
      end = r;
    }
    return end;
  }
  return -1;
}

export function dragFillTarget(sourceBox, hit, { dx = 0, dy = 0, view = null } = {}) {
  if (!sourceBox || !hit) return null;
  const own = view ? ownerOf(view, hit.r, hit.c) : { r: hit.r, c: hit.c, rows: 1, cols: 1 };
  const hitTop = own.r;
  const hitBottom = own.r + own.rows - 1;
  const hitLeft = own.c;
  const hitRight = own.c + own.cols - 1;
  const downDist = hitBottom > sourceBox.bottom ? hitBottom - sourceBox.bottom : 0;
  const upDist = hitTop < sourceBox.top ? sourceBox.top - hitTop : 0;
  const rightDist = hitRight > sourceBox.right ? hitRight - sourceBox.right : 0;
  const leftDist = hitLeft < sourceBox.left ? sourceBox.left - hitLeft : 0;
  const rowDist = Math.max(downDist, upDist);
  const colDist = Math.max(rightDist, leftDist);
  if (rowDist === 0 && colDist === 0) return null;

  const moved = Math.abs(dx) > 0 || Math.abs(dy) > 0;
  const vertical =
    rowDist > 0 && colDist > 0 ? (moved ? Math.abs(dy) >= Math.abs(dx) : rowDist >= colDist) : rowDist > 0;

  if (vertical) {
    if (downDist > 0) {
      return { top: sourceBox.bottom + 1, bottom: hitBottom, left: sourceBox.left, right: sourceBox.right };
    }
    if (upDist > 0) {
      return { top: hitTop, bottom: sourceBox.top - 1, left: sourceBox.left, right: sourceBox.right };
    }
    return null;
  }
  if (rightDist > 0) {
    return { top: sourceBox.top, bottom: sourceBox.bottom, left: sourceBox.right + 1, right: hitRight };
  }
  if (leftDist > 0) {
    return { top: sourceBox.top, bottom: sourceBox.bottom, left: hitLeft, right: sourceBox.left - 1 };
  }
  return null;
}

export function shortcutFillBoxes(view, sel, axis) {
  const box = selectionBox(view, sel);
  if (!box) return null;
  if (axis === "down") {
    let sourceBottom = box.top;
    for (let c = box.left; c <= box.right; c++) {
      const own = ownerOf(view, box.top, c);
      sourceBottom = Math.max(sourceBottom, own.r + own.rows - 1);
    }
    if (box.bottom <= sourceBottom) return null;
    return {
      sourceBox: { top: box.top, bottom: sourceBottom, left: box.left, right: box.right },
      targetBox: { top: sourceBottom + 1, bottom: box.bottom, left: box.left, right: box.right },
    };
  }
  let sourceRight = box.left;
  for (let r = box.top; r <= box.bottom; r++) {
    const own = ownerOf(view, r, box.left);
    sourceRight = Math.max(sourceRight, own.c + own.cols - 1);
  }
  if (box.right <= sourceRight) return null;
  return {
    sourceBox: { top: box.top, bottom: box.bottom, left: box.left, right: sourceRight },
    targetBox: { top: box.top, bottom: box.bottom, left: sourceRight + 1, right: box.right },
  };
}

export function buildSelectionFillBlock(view, box, rawText, { activeR = box.active.r, activeC = box.active.c, isText = false, tokenize } = {}) {
  const isFormula = !isText && typeof rawText === "string" && rawText.startsWith("=") && rawText.length > 1;
  const rows = box.bottom - box.top + 1;
  const cols = box.right - box.left + 1;
  const block = [];
  for (let dr = 0; dr < rows; dr++) {
    const r = box.top + dr;
    const line = [];
    for (let dc = 0; dc < cols; dc++) {
      const c = box.left + dc;
      if (view.cell(r, c)?.covered || view.rowHidden?.(r)) {
        line.push(undefined);
        continue;
      }
      line.push(isFormula ? shiftFormula(rawText, r - activeR, c - activeC, tokenize) : rawText);
    }
    block.push(line);
  }
  return { at: a1(box.top, box.left), block };
}

export function applyFillAction(ctx, sourceBox, targetBox, { series = true } = {}) {
  const sel = ctx.getSel();
  if (!sel) return false;
  const grid = ctx.gridOf(tabTable(ctx.workbook, sel.tab));
  if (!grid) return false;
  const view = ctx.viewOf(sel.tab);
  const sap = ctx.win.Sap?.grid;
  const write = buildFillWrite(grid, view, sourceBox, targetBox, {
    fmtKind: sap?.fmtKind,
    tokenize: sap?.tokenize,
    series,
  });
  if (write.cut) {
    ctx.chrome.showNotice(FILL_CUT_MESSAGE);
    return false;
  }
  const { at, block, unionBox } = write;
  if (!ctx.writeGrid(() => grid.setRange(at, block, { grow: true, detect: false }))) return false;
  const nextView = ctx.viewOf(sel.tab);
  if (nextView.rows.length > 0 && nextView.cols.length > 0) {
    const own = ownerOf(nextView, unionBox.top, unionBox.left);
    ctx.applyMove({
      tab: sel.tab,
      anchor: { row: nextView.rows[own.r], col: nextView.cols[own.c] },
      focus: {
        row: nextView.rows[Math.min(unionBox.bottom, nextView.rows.length - 1)],
        col: nextView.cols[Math.min(unionBox.right, nextView.cols.length - 1)],
      },
    });
  } else {
    ctx.refreshStatus();
    ctx.syncUI({ announce: true });
  }
  return true;
}

export function runShortcutFill(ctx, axis) {
  const sel = ctx.getSel();
  if (ctx.editor.editing || !sel) return;
  const boxes = shortcutFillBoxes(ctx.viewOf(sel.tab), sel, axis);
  if (!boxes) return;
  applyFillAction(ctx, boxes.sourceBox, boxes.targetBox, { series: false });
  ctx.editor.focus();
}

export function runAutoFillDown(ctx) {
  const sel = ctx.getSel();
  if (ctx.editor.editing || !sel || sel.whole) return;
  const view = ctx.viewOf(sel.tab);
  const box = selectionBox(view, sel);
  if (!box) return;
  const endRow = findAutoFillEndRow(view, box);
  if (endRow > box.bottom) {
    applyFillAction(ctx, box, { top: box.bottom + 1, bottom: endRow, left: box.left, right: box.right });
  }
  ctx.editor.focus();
}

export function runCommitAll(ctx) {
  const sel = ctx.getSel();
  if (!ctx.editor.editing || !sel) return;
  const view = ctx.viewOf(sel.tab);
  const box = selectionBox(view, sel);
  if (!box) return;
  if (box.top === box.bottom && box.left === box.right) {
    const single = ctx.commitEdit();
    if (single.ok) {
      ctx.refreshStatus();
      ctx.syncUI({ announce: true });
      ctx.editor.focus();
    }
    return;
  }
  const res = ctx.withMine(() =>
    ctx.editor.commitWith(({ target, text, isText }) => {
      const table = tabTable(ctx.workbook, target.tab);
      const ar = table ? rowIndex(table, target.row) : -1;
      const ac = table ? colIndex(table, target.col) : -1;
      if (ar === -1 || ac === -1) return { ok: false, why: "gone" };
      const grid = ctx.gridOf(table);
      if (!grid) return { ok: false, why: "gone" };
      const curView = ctx.viewOf(target.tab);
      const curBox = selectionBox(curView, ctx.getSel()) ?? box;
      const { at, block } = buildSelectionFillBlock(curView, curBox, text, {
        activeR: ar,
        activeC: ac,
        isText,
        tokenize: ctx.win.Sap?.grid?.tokenize,
      });
      try {
        grid.setRange(at, block, { grow: true });
      } catch (error) {
        if (error?.name !== "GridRefused") throw error;
        return { ok: false, why: "refused", error };
      }
      return { ok: true, changed: true };
    }),
  );
  if (res.ok) {
    ctx.refreshStatus();
    ctx.syncUI({ announce: true });
    ctx.editor.focus();
  }
}
