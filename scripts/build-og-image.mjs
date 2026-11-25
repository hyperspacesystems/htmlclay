#!/usr/bin/env node
// Generates `website/og.png` (1200 x 630) from vector SVG + sharp so the Open
// Graph card stays crisp, reproducible, and free of 8-bit dark-gradient banding.

import { readFileSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';
import sharp from 'sharp';

const ROOT = join(dirname(fileURLToPath(import.meta.url)), '..');
const WEBSITE_DIR = join(ROOT, 'website');
const OUT_PNG = join(WEBSITE_DIR, 'og.png');

const markRaw = readFileSync(join(WEBSITE_DIR, 'assets', 'htmlclay-mark.svg'), 'utf8');
const markInner = markRaw
  .replace(/^<svg[^>]*>/, '')
  .replace(/<\/svg>\s*$/, '')
  .replace(/hc-mark-clay/g, 'og-mark-clay')
  .replace(/hc-mark-cube/g, 'og-mark-cube')
  .replace(/hc-mark-clip/g, 'og-mark-clip');

// Carbon 2 ("Perspective Wave Surface") outlined 3D wave icon chosen in bake-off
function cubicPt(p0, p1, p2, p3, t) {
  const mt = 1 - t;
  const mt2 = mt * mt;
  const t2 = t * t;
  return [
    mt2 * mt * p0[0] + 3 * mt2 * t * p1[0] + 3 * mt * t2 * p2[0] + t2 * t * p3[0],
    mt2 * mt * p0[1] + 3 * mt2 * t * p1[1] + 3 * mt * t2 * p2[1] + t2 * t * p3[1],
  ];
}

function fitOpenCatmull(pts) {
  const segs = [];
  for (let i = 0; i < pts.length - 1; i++) {
    const p0 = pts[Math.max(0, i - 1)];
    const p1 = pts[i];
    const p2 = pts[i + 1];
    const p3 = pts[Math.min(pts.length - 1, i + 2)];
    segs.push([
      p1,
      [p1[0] + (p2[0] - p0[0]) / 6, p1[1] + (p2[1] - p0[1]) / 6],
      [p2[0] - (p3[0] - p1[0]) / 6, p2[1] - (p3[1] - p1[1]) / 6],
      p2,
    ]);
  }
  return segs;
}

function buildShapeableWavePaths() {
  function surf(u, v) {
    const width = 18.2 - 4.0 * v;
    const xOffset = (18.2 - width) * 0.5 + v * 2.2;
    const baseX = xOffset + u * width;
    const phase = u * Math.PI * 2.35 - 0.35 + v * 0.28;
    const amp = 2.55 * (1 - 0.14 * v);
    const waveY = -amp * Math.sin(phase);
    const camber = 4 * v * (1 - v);
    const archY = -0.60 * Math.sin(phase) * camber;
    const baseY = 12.0 - v * 8.2;
    return [baseX, baseY + waveY + archY];
  }

  const segLists = [];
  for (const v of [0, 0.5, 1]) {
    const pts = [];
    for (let i = 0; i <= 28; i++) pts.push(surf(i / 28, v));
    segLists.push(fitOpenCatmull(pts));
  }
  for (const u of [0, 0.34, 0.67, 1]) {
    const pts = [];
    for (let j = 0; j <= 10; j++) pts.push(surf(u, j / 10));
    segLists.push(fitOpenCatmull(pts));
  }

  let minX = Infinity, maxX = -Infinity, minY = Infinity, maxY = -Infinity;
  for (const segs of segLists) {
    for (const [a, b, c, d] of segs) {
      for (let k = 0; k <= 24; k++) {
        const [x, y] = cubicPt(a, b, c, d, k / 24);
        if (x < minX) minX = x;
        if (x > maxX) maxX = x;
        if (y < minY) minY = y;
        if (y > maxY) maxY = y;
      }
    }
  }
  const scale = 20.4 / Math.max(maxX - minX, maxY - minY);
  const cx = (minX + maxX) / 2;
  const cy = (minY + maxY) / 2;
  const tx = (p) => [12 + (p[0] - cx) * scale, 12 + (p[1] - cy) * scale];

  return segLists
    .map((segs) => {
      const mapped = segs.map(([a, b, c, d]) => [tx(a), tx(b), tx(c), tx(d)]);
      let dStr = `M${mapped[0][0][0].toFixed(2)} ${mapped[0][0][1].toFixed(2)}`;
      for (const [, p1, p2, p3] of mapped) {
        dStr += ` C${p1[0].toFixed(2)} ${p1[1].toFixed(2)}, ${p2[0].toFixed(2)} ${p2[1].toFixed(2)}, ${p3[0].toFixed(2)} ${p3[1].toFixed(2)}`;
      }
      return `<path d="${dStr}"/>`;
    })
    .join('');
}

// Foreground transparent SVG (squircle + typography + 3 badges; background is
// synthesized in float RGB with triangular dither so dark gradients never band)
function buildForegroundSvg() {
  const wavePaths = buildShapeableWavePaths();
  // Optically centered lockup: squircle x=215..463, text/badges x=541..985 (215px margin both sides)
  const sqX = 215;
  const sqY = 191;
  const colX = 541;

  return `<svg xmlns="http://www.w3.org/2000/svg" width="1200" height="630" viewBox="0 0 1200 630">
  <defs>
    <style>
      .eyebrow {
        font-family: "IBM Plex Sans", -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif;
        font-size: 31px;
        font-weight: 600;
        fill: #F49B64;
        letter-spacing: -0.45px;
      }
      .headline {
        font-family: "IBM Plex Sans", -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif;
        font-size: 64px;
        font-weight: 600;
        fill: #F5F4F0;
        letter-spacing: -2px;
      }
      .bullet {
        font-family: "IBM Plex Sans", -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif;
        font-size: 34px;
        font-weight: 600;
        fill: #E2E4EC;
        letter-spacing: -0.6px;
      }
    </style>
    <linearGradient id="og-sq" x1="0" y1="0" x2="0" y2="1">
      <stop offset="0" stop-color="#252730"/>
      <stop offset="1" stop-color="#1A1C22"/>
    </linearGradient>
    <linearGradient id="og-blue" x1="0" y1="0" x2="0" y2="1">
      <stop offset="0" stop-color="#4A8FE7"/>
      <stop offset="1" stop-color="#3570C4"/>
    </linearGradient>
    <linearGradient id="og-purple" x1="0" y1="0" x2="0" y2="1">
      <stop offset="0" stop-color="#7C6EA8"/>
      <stop offset="1" stop-color="#5F5289"/>
    </linearGradient>
    <linearGradient id="og-amber" x1="0" y1="0" x2="0" y2="1">
      <stop offset="0" stop-color="#D4883A"/>
      <stop offset="1" stop-color="#B36A22"/>
    </linearGradient>
    <filter id="og-shadow-blur" x="-30%" y="-30%" width="160%" height="180%">
      <feGaussianBlur stdDeviation="20"/>
    </filter>
  </defs>

  <!-- Separate soft drop shadow behind squircle (avoids librsvg filter banding on the squircle fill) -->
  <rect x="${sqX + 6}" y="${sqY + 22}" width="236" height="236" rx="52" fill="#0B0C10" opacity="0.65" filter="url(#og-shadow-blur)"/>

  <!-- Left Claygoo Squircle -->
  <rect x="${sqX}" y="${sqY}" width="248" height="248" rx="56" fill="url(#og-sq)"/>
  <rect x="${sqX + 0.75}" y="${sqY + 0.75}" width="246.5" height="246.5" rx="55.25" fill="none" stroke="#FAF8F3" stroke-opacity="0.10" stroke-width="1.5"/>
  <svg x="${sqX + 50}" y="${sqY + 41}" width="148" height="166" viewBox="236 208 564 632">${markInner}</svg>

  <!-- Right Typography -->
  <text x="${colX}" y="116" class="eyebrow">HTML Clay</text>
  <text x="${colX}" y="184" class="headline">Software in a</text>
  <text x="${colX}" y="252" class="headline">single HTML file</text>

  <!-- Bullet 1: One portable file -->
  <rect x="${colX}" y="286" width="62" height="62" rx="17" fill="url(#og-blue)"/>
  <rect x="${colX + 0.75}" y="286.75" width="60.5" height="60.5" rx="16.25" fill="none" stroke="#FAF8F3" stroke-opacity="0.22" stroke-width="1"/>
  <svg x="${colX + 15.5}" y="301.5" width="31" height="31" viewBox="0 0 24 24" fill="none" stroke="#FAF8F3" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
    <path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z"/>
    <polyline points="14 2 14 8 20 8"/>
  </svg>
  <text x="${colX + 84}" y="329" class="bullet">One portable file</text>

  <!-- Bullet 2: Shapeable UI & code -->
  <rect x="${colX}" y="380" width="62" height="62" rx="17" fill="url(#og-purple)"/>
  <rect x="${colX + 0.75}" y="380.75" width="60.5" height="60.5" rx="16.25" fill="none" stroke="#FAF8F3" stroke-opacity="0.22" stroke-width="1"/>
  <svg x="${colX + 13.5}" y="393.5" width="35" height="35" viewBox="0 0 24 24" fill="none" stroke="#FAF8F3" stroke-width="1.75" stroke-linecap="round" stroke-linejoin="round">
    ${wavePaths}
  </svg>
  <text x="${colX + 84}" y="423" class="bullet">Shapeable UI &amp; code</text>

  <!-- Bullet 3: Local-first, no DB -->
  <rect x="${colX}" y="474" width="62" height="62" rx="17" fill="url(#og-amber)"/>
  <rect x="${colX + 0.75}" y="474.75" width="60.5" height="60.5" rx="16.25" fill="none" stroke="#FAF8F3" stroke-opacity="0.22" stroke-width="1"/>
  <svg x="${colX + 15.5}" y="489.5" width="31" height="31" viewBox="0 0 24 24" fill="none" stroke="#FAF8F3" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
    <rect x="2" y="4" width="20" height="14" rx="2"/>
    <path d="M8 21h8"/>
    <path d="M12 18v3"/>
  </svg>
  <text x="${colX + 84}" y="517" class="bullet">Local-first, no DB</text>
</svg>`;
}

// Deterministic hash noise in [-0.5, 0.5] for smooth banding-free dark background
function tpdfDither(x, y, ch) {
  let h1 = Math.imul(x + 1, 374761393) ^ Math.imul(y + 1, 668265263) ^ Math.imul(ch + 1, 1442695041);
  h1 = Math.imul(h1 ^ (h1 >>> 13), 1274126177);
  const u1 = ((h1 ^ (h1 >>> 16)) >>> 0) / 4294967296;
  let h2 = Math.imul(x + 17, 668265263) ^ Math.imul(y + 31, 374761393) ^ Math.imul(ch + 7, 2246822519);
  h2 = Math.imul(h2 ^ (h2 >>> 13), 1274126177);
  const u2 = ((h2 ^ (h2 >>> 16)) >>> 0) / 4294967296;
  return (u1 + u2 - 1) * 0.55;
}

function buildBackgroundBuffer(W, H) {
  const buf = Buffer.alloc(W * H * 4);
  const glowCx = 339;
  const glowCy = 315;
  const glowR = 320;

  for (let y = 0; y < H; y++) {
    const ty = y / (H - 1);
    // Smooth linear gradient #1E1F25 (30,31,37) -> #17181D (23,24,29)
    const baseR = 30 * (1 - ty) + 23 * ty;
    const baseG = 31 * (1 - ty) + 24 * ty;
    const baseB = 37 * (1 - ty) + 29 * ty;

    for (let x = 0; x < W; x++) {
      const dist = Math.hypot(x - glowCx, y - glowCy) / glowR;
      const g = dist < 1 ? Math.pow(1 - dist * dist, 2.2) * 0.12 : 0;
      // Blend warm glow (#E5824E = 229,130,78)
      const r = baseR * (1 - g) + 229 * g + tpdfDither(x, y, 0);
      const gr = baseG * (1 - g) + 130 * g + tpdfDither(x, y, 1);
      const b = baseB * (1 - g) + 78 * g + tpdfDither(x, y, 2);

      const idx = (y * W + x) * 4;
      buf[idx] = Math.max(1, Math.min(254, Math.round(r)));
      buf[idx + 1] = Math.max(1, Math.min(254, Math.round(gr)));
      buf[idx + 2] = Math.max(1, Math.min(254, Math.round(b)));
      buf[idx + 3] = 255;
    }
  }
  return buf;
}

async function main() {
  const W = 1200;
  const H = 630;
  const bgBuf = buildBackgroundBuffer(W, H);
  const fgSvg = Buffer.from(buildForegroundSvg(), 'utf8');

  // Render foreground SVG at 2x density for crisp text & icon anti-aliasing, then composite over dithered background
  const fgPng = await sharp(fgSvg, { density: 192 })
    .resize(W, H)
    .png()
    .toBuffer();

  const info = await sharp(bgBuf, { raw: { width: W, height: H, channels: 4 } })
    .composite([{ input: fgPng, top: 0, left: 0 }])
    .png({ compressionLevel: 9 })
    .toFile(OUT_PNG);

  console.log(`wrote ${OUT_PNG} (${info.width}x${info.height}, ${Math.round(info.size / 1024)} KB)`);
}

main().catch((err) => {
  console.error(err);
  process.exit(1);
});
