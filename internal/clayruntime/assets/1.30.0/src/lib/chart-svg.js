export const CHART_COLOURS = Object.freeze([
  "#3170b8", "#b87118", "#268576", "#c94a38", "#835cc0", "#2284a8", "#b8487a", "#6b8326",
]);

const TYPES = new Set(["column", "bar", "line", "area", "pie", "scatter"]);
const LONE = /[\uD800-\uDBFF](?![\uDC00-\uDFFF])|(?<![\uD800-\uDBFF])[\uDC00-\uDFFF]/g;
const isNum = (v) => typeof v === "number" && Number.isFinite(v);
const clamp = (v, lo, hi) => Math.min(hi, Math.max(lo, v));
const stepK = (minPx, step) => Math.max(1, Math.ceil(minPx / Math.max(step, 1e-4)));
const clean = (v) => String(v ?? "").replace(/[\r\0]/g, "").replace(LONE, "");
const flat = (v) => clean(v).replace(/[\t\n]+/g, " ");
const esc = (v) => clean(v).replace(/&/g, "&amp;").replace(/</g, "&lt;").replace(/>/g, "&gt;").replace(/\u00A0/g, "&nbsp;");
const colOf = (i) => CHART_COLOURS[i % 8];
const escAttr = (v) => {
  let s = flat(v).replace(/[<>\\]/g, "");
  while (/\/\*|url\s*\(/i.test(s)) s = s.replace(/\/\*/g, "").replace(/url\s*\(/gi, "url ");
  return s.replace(/&/g, "&amp;").replace(/"/g, "&quot;").replace(/\u00A0/g, "&nbsp;");
};
const num = (n) => !Number.isFinite(n) ? "0" : Math.abs(n) >= 1e15 ? String(BigInt(Math.round(n))) : String(Number(n.toFixed(2)) || 0);
const cut = (raw, maxPx, sz = 11) => {
  const s = flat(raw), m = Math.floor(maxPx / (sz * 0.58));
  if (!s || m <= 0) return "";
  if (s.length <= m) return s;
  const c = Array.from(s);
  return c.length <= m ? s : m === 1 ? "…" : `${c.slice(0, m - 1).join("").trimEnd() || c[0]}…`;
};
const ticks = (min, max, cnt = 5) => {
  if (min === max) { if (!min) max = 1; else { const p = Math.abs(min) * 0.1 || 1; min -= p; max += p; } }
  let e = Math.max(-2, Math.floor(Math.log10((max - min) / Math.max(1, cnt - 1)))), m = 0, st = 10 ** e;
  while (Math.ceil(max / st - 1e-9) - Math.floor(min / st + 1e-9) + 1 > Math.max(2, cnt)) st = [1, 2, 5][m = (m + 1) % 3] * 10 ** (m ? e : ++e);
  const out = [];
  for (let i = Math.floor(min / st + 1e-9), hi = Math.ceil(max / st - 1e-9); i <= hi; i++) out.push(Number((i * st).toPrecision(12)) || 0);
  return out.length >= 2 ? out : [min, max];
};

const txt = (x, y, s, anchor = "start", size = 11, extra = "") =>
  `<text x="${num(x)}" y="${num(y)}" text-anchor="${anchor}" dominant-baseline="central" fill="currentColor" font-size="${size}"${extra}>${esc(s)}</text>`;
const rect = (x, y, w, h, fill) => `<rect x="${num(x)}" y="${num(y)}" width="${num(w)}" height="${num(h)}" fill="${fill}"></rect>`;
const line = (x1, y1, x2, y2, op = "", stroke = "currentColor", sw = 1) =>
  `<line x1="${num(x1)}" y1="${num(y1)}" x2="${num(x2)}" y2="${num(y2)}" stroke="${stroke}"${op ? ` stroke-opacity="${op}"` : ""} stroke-width="${sw}"></line>`;
const dot = (cx, cy, r, fill, op = "") => `<circle cx="${num(cx)}" cy="${num(cy)}" r="${r}" fill="${fill}"${op ? ` fill-opacity="${op}"` : ""}></circle>`;

function buildDesc(type, series, cats, xs) {
  const list = type === "pie" ? series.slice(0, 1) : series, groups = [];
  let total = 0, shown = 0;
  for (let s = 0; s < list.length; s++) {
    const vals = list[s]?.values ?? [], items = [];
    for (let i = 0; i < vals.length; i++) {
      const v = vals[i];
      if (!isNum(v) || (type === "pie" && v <= 0) || (type === "scatter" && !isNum(xs?.[i]))) continue;
      if (++total <= 200) { const c = flat(cats?.[i] ?? "") || (type === "scatter" ? num(xs[i]) : ""); items.push(c ? `${c}: ${num(v)}` : num(v)); shown += 1; }
    }
    if (items.length) groups.push(`${flat(list[s]?.name ?? "").trim() || `Series ${s + 1}`}: ${items.join(", ")}`);
  }
  return total ? { total, text: `${groups.join("; ")}${total > shown ? `, and ${total - shown} more` : ""}` } : { total: 0, text: "No numbers to chart" };
}

function legend(items, W, H, x, y, w, h) {
  const rW = clamp(Math.round(W * 0.28), 96, 156), right = W >= 360 && W / H >= 1.35 && w - rW - 12 >= 120;
  if (!items.length || W < 200 || h < (right ? 90 : 112)) return { area: { x, y, w, h }, nodes: "" };
  const legW = right ? rW : w, cols = right ? 1 : clamp(Math.floor(w / 92), 1, items.length), rows = right ? clamp(Math.floor(h / 18), 1, items.length) : Math.min(2, Math.ceil(items.length / cols));
  const cap = items.length > cols * rows, n = cap ? Math.max(1, cols * rows - 1) : items.length, lx = right ? x + w - legW : x, ly = right ? y : y + h - rows * 18, cw = legW / cols, out = ["<g>"];
  for (let i = 0; i < n; i++) {
    const it = items[i], cx = lx + (i % cols) * cw, cy = ly + Math.floor(i / cols) * 18 + 9, lbl = cut(it.label, cw - 18 - (it.suffix ? (it.suffix.length + 3) * 6.4 : 0));
    out.push(rect(cx, cy - 5, 10, 10, it.color), txt(cx + 14, cy, it.suffix ? (lbl ? `${lbl} (${it.suffix})` : it.suffix) : lbl));
  }
  if (cap) out.push(txt(lx + (n % cols) * cw + (right ? 14 : 0), ly + Math.floor(n / cols) * 18 + 9, `+${items.length - n} more`, "start", 11, ' opacity="0.7"'));
  return { area: right ? { x, y, w: w - legW - 12, h } : { x, y, w, h: h - rows * 18 - 4 }, nodes: `${out.join("")}</g>` };
}

function thinPts(pts, thin) {
  if (!thin || pts.length <= 4) return pts;
  const out = [];
  for (let i = 0, j, lo, hi; i < pts.length; i = j) {
    for (j = i + 1, lo = i, hi = i; j < pts.length && Math.round(pts[j].x) === Math.round(pts[i].x); j++) {
      if (pts[j].y < pts[lo].y) lo = j;
      if (pts[j].y > pts[hi].y) hi = j;
    }
    for (const k of new Set([i, Math.min(lo, hi), Math.max(lo, hi), j - 1])) out.push(pts[k]);
  }
  return out;
}

export function chartSvg(model, size = {}) {
  const W = Math.max(120, Math.round(Number(size?.width) || 480)), H = Math.max(80, Math.round(Number(size?.height) || 300));
  const type = TYPES.has(model?.type) ? model.type : "column", stacked = Boolean(model?.stacked) && /column|bar|area/.test(type);
  const cats = Array.isArray(model?.categories) ? model.categories : [], xs = Array.isArray(model?.xs) ? model.xs : null;
  const series = Array.isArray(model?.series) ? model.series : [];
  const title = flat(model?.title ?? "").trim(), fb = `Chart: ${type}`, desc = buildDesc(type, series, cats, xs);
  const parts = [`<svg xmlns="http://www.w3.org/2000/svg" width="${W}" height="${H}" viewBox="0 0 ${W} ${H}" role="img" aria-label="${escAttr(title) || escAttr(fb)}"><title>${esc(title || fb)}</title><desc>${esc(desc.text)}</desc>`];
  const th = title ? 22 : 0, tCut = title ? cut(title, W - 20, 12) : "";
  if (tCut) parts.push(txt(10, 16, tCut, "start", 12, ' font-weight="600"'));
  if (!desc.total) return [...parts, txt(W / 2, 8 + th + (H - 16 - th) / 2, "No numbers to chart", "middle", 12, ' opacity="0.65"'), "</svg>"].join("");
  const by = 8 + th, bw = W - 16, bh = Math.max(16, H - 16 - th);

  if (type === "pie") {
    const vals = series[0]?.values ?? [], ls = [];
    let sum = 0;
    for (let i = 0; i < vals.length; i++) if (isNum(vals[i]) && vals[i] > 0) { ls.push({ label: flat(cats[i] ?? ""), v: vals[i] }); sum += vals[i]; }
    if (ls.length > 64) ls.splice(63, ls.length, { label: "Other", v: ls.slice(63).reduce((a, b) => a + b.v, 0) });
    const sl = ls.map((e, i) => ({ label: e.label, f: e.v / sum, suffix: `${num(Math.round((e.v / sum) * 1e3) / 10)}%`, color: colOf(ls.length > 1 && i === ls.length - 1 && !(i % 8) ? 1 : i) }));
    const leg = legend(sl, W, H, 8, by, bw, bh), { x, y, w, h } = leg.area, cx = x + w / 2, cy = y + h / 2, r = Math.max(8, Math.min(w, h) / 2 - 4);
    parts.push("<g>");
    const full = sl.find((slice) => slice.f >= 0.9999);
    if (sl.length === 1 || full) parts.push(dot(cx, cy, num(r), (full ?? sl[0]).color));
    else for (let i = 0, a = -Math.PI / 2; i < sl.length; i++) {
      const s = sl[i], b = a + s.f * Math.PI * 2, [x1, y1, x2, y2] = [cx + r * Math.cos(a), cy + r * Math.sin(a), cx + r * Math.cos(b), cy + r * Math.sin(b)].map(num);
      if (x1 !== x2 || y1 !== y2 || s.f > 0.5) parts.push(`<path d="M ${num(cx)} ${num(cy)} L ${x1} ${y1} A ${num(r)} ${num(r)} 0 ${s.f > 0.5 ? 1 : 0} 1 ${x2} ${y2} Z" fill="${s.color}" stroke="currentColor" stroke-opacity="0.18" stroke-width="1"></path>`);
      a = b;
    }
    return [...parts, "</g>", leg.nodes, "</svg>"].join("");
  }

  const leg = legend(series.length > 1 ? series.map((s, i) => ({ color: colOf(i), label: flat(s?.name ?? "").trim() || `Series ${i + 1}`, suffix: "" })) : [], W, H, 8, by, bw, bh);
  const { area: ar } = leg, show = ar.w >= 136 && ar.h >= 70, n = Math.max(1, cats.length, ...series.map((s) => s?.values?.length ?? 0));
  const zeroed = /column|bar|area/.test(type), horiz = type === "bar";
  let vMin = zeroed ? 0 : Infinity, vMax = zeroed ? 0 : -Infinity, xMin = Infinity, xMax = -Infinity;
  for (let i = 0; i < n; i++) {
    let pos = 0, neg = 0;
    for (const s of series) {
      const v = s?.values?.[i];
      if (!isNum(v) || (type === "scatter" && !isNum(xs?.[i]))) continue;
      if (stacked) { if (v >= 0) pos += v; else neg += v; }
      else { vMin = Math.min(vMin, v); vMax = Math.max(vMax, v); if (type === "scatter") { xMin = Math.min(xMin, xs[i]); xMax = Math.max(xMax, xs[i]); } }
    }
    if (stacked) { vMax = Math.max(vMax, pos); vMin = Math.min(vMin, neg); }
  }

  const vTicks = ticks(vMin, vMax, clamp(Math.floor(horiz ? ar.w / 60 : ar.h / 34), 2, 6)), dMin = vTicks[0], dMax = vTicks.at(-1);
  const hasCats = cats.some((c) => flat(c).trim() !== ""), maxLP = Math.floor(ar.w * 0.34);
  const maxCat = horiz && show && hasCats ? clamp(Math.max(0, ...cats.slice(0, 64).map((c) => flat(c).length)), 0, 20) : 0;
  const lPad = !show ? 4 : horiz ? (hasCats ? clamp(Math.ceil(maxCat * 6.4) + 10, 32, maxLP) : 8) : clamp(Math.ceil(Math.max(...vTicks.map((t) => num(t).length)) * 6.5) + 10, 26, maxLP);
  const px = ar.x + lPad, py = ar.y + 6, pw = Math.max(12, ar.w - lPad - (!show ? 6 : horiz ? 10 : 8)), ph = Math.max(12, ar.h - 6 - (!show ? 4 : horiz || type === "scatter" || hasCats ? 20 : 6));
  const xTicks = type === "scatter" ? ticks(xMin, xMax, clamp(Math.floor(pw / 58), 2, 6)) : null;
  const yVal = (v) => py + ((dMax - v) / (dMax - dMin)) * ph, xVal = (v) => px + ((v - dMin) / (dMax - dMin)) * pw;
  const xScat = (v) => px + ((v - xTicks[0]) / (xTicks.at(-1) - xTicks[0])) * pw, xIdx = (i) => px + (n <= 1 ? pw / 2 : (i * pw) / (n - 1));
  const bLbl = (xx, raw, slot) => { const s = show ? cut(raw, Math.min(slot, 2 * Math.min(xx - 3, W - 3 - xx))) : ""; if (s) parts.push(txt(xx, py + ph + 12, s, "middle")); };
  const lLbl = (yy, raw) => { const s = show ? cut(raw, lPad - 8) : ""; if (s) parts.push(txt(px - 6, yy, s, "end")); };
  const drawXT = (ts, posOf, zOp) => {
    const st = pw / Math.max(1, ts.length - 1), k = stepK(38, st);
    ts.forEach((t, ti) => { parts.push(line(posOf(t), py, posOf(t), py + ph, t ? "0.14" : zOp)); if (!(ti % k)) bLbl(posOf(t), num(t), k * st - 4); });
  };

  parts.push("<g>");
  if (horiz) {
    drawXT(vTicks, xVal, "0.35");
    if (hasCats) for (let i = 0, bH = ph / n, k = stepK(15, bH); i < n; i += k) lLbl(py + (i + 0.5) * bH, cats[i]);
  } else {
    for (const t of vTicks) { parts.push(line(px, yVal(t), px + pw, yVal(t), t ? "0.14" : "0.35")); lLbl(yVal(t), num(t)); }
    if (type === "scatter" && xTicks) drawXT(xTicks, xScat, "0.3");
    else if (hasCats) for (let i = 0, b = type === "column", st = b ? pw / n : pw / Math.max(1, n - 1), k = stepK(44, st); i < n; i += k) bLbl(b ? px + (i + 0.5) * st : xIdx(i), cats[i], k * st - 6);
  }
  parts.push("</g><g>");

  const posS = stacked ? new Float64Array(n) : null, negS = stacked ? new Float64Array(n) : null, zc = clamp(0, dMin, dMax);
  const stepStack = (i, v, base, valAt) => {
    if (!stacked) return [base, valAt(v)];
    const arr = v >= 0 ? posS : negS, p0 = valAt(arr[i]);
    arr[i] += v;
    return [p0, valAt(arr[i])];
  };

  if (type === "column" || type === "bar") {
    const valAt = horiz ? xVal : yVal, cs = horiz ? py : px, spn = horiz ? ph : pw, base = valAt(zc);
    const band = spn / n, sc = Math.max(1, series.length), grp = Math.min((horiz ? 32 : 48) * sc, band * 0.74);
    const gap = !stacked && grp / sc >= 5 ? 1 : 0, tk = stacked ? clamp(band * 0.68, 0.5, horiz ? 36 : 48) : Math.max(0.5, (grp - gap * (sc - 1)) / sc);
    for (let s = 0, st = Math.max(1, Math.ceil((n * sc) / 8e3)); s < series.length; s++) {
      const col = colOf(s), vs = series[s]?.values ?? [];
      for (let i = 0; i < n; i += st) {
        if (!isNum(vs[i])) continue;
        const [p0, p1] = stepStack(i, vs[i], base, valAt), a = Math.min(p0, p1), sp = Math.abs(p1 - p0);
        const cp = cs + (i + 0.5) * band - (stacked ? tk / 2 : grp / 2 - s * (tk + gap));
        parts.push(horiz ? rect(a, cp, sp, tk, col) : rect(cp, a, tk, sp, col));
      }
    }
  } else if (type === "line" || type === "area") {
    const thin = n > pw * 1.5, dots = type === "line" && n <= 32 && series.length <= 4, base = yVal(zc);
    for (let s = 0; s < series.length; s++) {
      const col = colOf(s), vs = series[s]?.values ?? [];
      let run = [];
      const flush = () => {
        if (!run.length) return;
        const pts = thinPts(run, thin), p0 = pts[0];
        if (pts.length === 1) {
          if (type === "area") parts.push(line(p0.x, p0.y0, p0.x, p0.y, "", col, 2));
          parts.push(dot(p0.x, p0.y, 3, col));
        } else {
          const top = pts.map((p) => `${num(p.x)},${num(p.y)}`);
          if (type === "area") parts.push(`<polygon points="${top.join(" ")} ${stacked ? pts.slice().reverse().map((p) => `${num(p.x)},${num(p.y0)}`).join(" ") : `${num(pts.at(-1).x)},${num(base)} ${num(p0.x)},${num(base)}`}" fill="${col}" fill-opacity="${stacked ? "0.45" : "0.24"}"></polygon>`);
          parts.push(`<polyline points="${top.join(" ")}" fill="none" stroke="${col}" stroke-width="2"></polyline>`);
          if (dots) for (const p of pts) parts.push(dot(p.x, p.y, 2.5, col));
        }
        run = [];
      };
      for (let i = 0; i < n; i++) {
        if (!isNum(vs[i])) { flush(); continue; }
        const [y0, y] = stepStack(i, vs[i], base, yVal);
        run.push({ x: xIdx(i), y, y0 });
      }
      flush();
    }
  } else {
    for (let s = 0, st = Math.max(1, Math.ceil(desc.total / 5e3)); s < series.length; s++) {
      const col = colOf(s), vs = series[s]?.values ?? [];
      for (let i = 0; i < vs.length; i += st) if (isNum(xs?.[i]) && isNum(vs[i])) parts.push(dot(xScat(xs[i]), yVal(vs[i]), 3, col, "0.85"));
    }
  }
  parts.push("</g>", leg.nodes, "</svg>");
  return parts.join("");
}
