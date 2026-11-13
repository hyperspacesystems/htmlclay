import { readFileSync, writeFileSync, existsSync } from 'node:fs';

const [cssPath, swiftPath, flag] = process.argv.slice(2);
if (!cssPath || !swiftPath) {
  console.error('usage: node gen-tokens.mjs <00-styles.css> <KilnPalette.generated.swift> [--check]');
  process.exit(2);
}

const css = readFileSync(cssPath, 'utf8');

function block(mode) {
  const start = css.indexOf(`.mode-col[data-mode="${mode}"] {`);
  if (start < 0) throw new Error(`no ${mode} block`);
  return css.slice(start, css.indexOf('\n}', start));
}

function camel(name) {
  return name.replace(/^--n-/, '').replace(/^--/, '').replace(/-([a-z0-9])/g, (_, c) => c.toUpperCase());
}

function colour(value) {
  const hex = value.match(/^#([0-9A-Fa-f]{6})$/);
  if (hex) return `Color(css: 0x${hex[1].toUpperCase()})`;
  const rgba = value.match(/^rgba\(\s*(\d+),\s*(\d+),\s*(\d+),\s*([\d.]+)\s*\)$/);
  if (rgba) {
    const [r, g, b] = rgba.slice(1, 4).map(n => Number(n).toString(16).padStart(2, '0').toUpperCase());
    return `Color(css: 0x${r}${g}${b}, alpha: ${rgba[4]})`;
  }
  return null;
}

function tokens(mode) {
  const out = new Map();
  const skipped = [];
  const pattern = /(--(?:n|lvl)-[a-z0-9-]+):\s*([^;]+);/g;
  for (const [, name, raw] of block(mode).matchAll(pattern)) {
    const value = raw.trim().replace(/\s+/g, ' ');
    const single = colour(value);
    const gradient = value.match(/^linear-gradient\(180deg, (#[0-9A-Fa-f]{6}) 0%, (#[0-9A-Fa-f]{6}) 100%\)$/);
    if (single) out.set(camel(name), { type: 'Color', swift: single });
    else if (gradient) out.set(camel(name), { type: '[Color]', swift: `[${colour(gradient[1])}, ${colour(gradient[2])}]` });
    else skipped.push(name);
  }
  return { out, skipped };
}

const light = tokens('light');
const dark = tokens('dark');
const names = [...light.out.keys()];
const missing = names.filter(n => !dark.out.has(n)).concat([...dark.out.keys()].filter(n => !light.out.has(n)));
if (missing.length) {
  console.error(`tokens not in both modes: ${missing.join(', ')}`);
  process.exit(1);
}

const fields = names.map(n => `    public let ${n}: ${light.out.get(n).type}`).join('\n');
const make = (mode, set) => `    static let ${mode} = KilnPalette(\n${names.map(n => `        ${n}: ${set.out.get(n).swift}`).join(',\n')})`;
const swift = `import SwiftUI

public struct KilnPalette: Sendable {
${fields}
}

public extension KilnPalette {
${make('light', light)}

${make('dark', dark)}
}
`;

if (flag === '--check') {
  const current = existsSync(swiftPath) ? readFileSync(swiftPath, 'utf8') : '';
  if (current !== swift) {
    console.error('TOKENS STALE: run gen-tokens.mjs without --check');
    process.exit(1);
  }
  console.log(`TOKENS OK ${names.length} tokens x 2 modes (skipped: ${light.skipped.join(', ') || 'none'})`);
} else {
  writeFileSync(swiftPath, swift);
  console.log(`TOKENS WROTE ${names.length} tokens x 2 modes (skipped: ${light.skipped.join(', ') || 'none'})`);
}
