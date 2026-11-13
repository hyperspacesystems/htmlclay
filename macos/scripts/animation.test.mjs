import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import vm from 'node:vm';

const file = process.env.ANIM_FILE || new URL('../Resources/icon-film.html', import.meta.url);
const html = readFileSync(file, 'utf8');
const json = id => html.match(new RegExp(`<script type="application/json" id="${id}">([\\s\\S]*?)</script>`))[1];
const code = [...html.matchAll(/<script>([\s\S]*?)<\/script>/g)].map(m => m[1]);

function boot({ calm = false } = {}) {
  const el = () => ({ attrs: {}, setAttribute(a, v) { this.attrs[a] = v; } });
  const parts = {};
  let onClick = null;
  const host = { dataset: {}, querySelector: s => (parts[s] ??= el()), addEventListener: (t, f) => { if (t === 'click') onClick = f; } };
  const queue = [];
  let now = 0;
  const ctx = {
    document: {
      getElementById: id => ({ textContent: json(id) }),
      querySelector: s => (s === '[data-film]' ? host : null),
    },
    requestAnimationFrame: f => queue.push(f),
    Math, JSON, Object, Array, Number, String, Float32Array, Float64Array, Int16Array, Uint8Array, Int8Array, Int32Array, Uint16Array, Map, Set, Error, atob,
  };
  ctx.window = { matchMedia: () => ({ matches: calm }) };
  ctx.globalThis = ctx;
  vm.createContext(ctx);
  for (const c of code) vm.runInContext(c, ctx);
  ctx.window.iconAnim ??= ctx.iconAnim;
  const api = ctx.window.iconAnim;
  const step = ms => { now += ms; const fs = queue.splice(0); fs.forEach(f => f(now)); return fs.length; };
  const runOut = () => { let n = 0; while (queue.length && n < 100000) { step(16.7); n++; } return n; };
  return { api, host, queue, step, runOut, click: () => onClick() };
}

test('plays once, then stops scheduling frames and holds the settled pose', () => {
  const b = boot();
  assert.equal(b.api.playing, true);
  assert.equal(b.host.dataset.state, 'playing');
  const n = b.runOut();
  assert.ok(n > 60, `ran ${n} frames`);
  assert.equal(b.api.playing, false);
  assert.equal(b.host.dataset.state, 'settled');
  assert.equal(b.api.time, b.api.LAST);
  assert.equal(b.queue.length, 0, 'no frame scheduled after settling');
  assert.equal(b.step(1000), 0);
  assert.equal(b.host.querySelector('[data-k="rest"]').attrs.display, 'inline');
});

test('a click while moving does nothing; a click after settling replays', () => {
  const b = boot();
  for (let i = 0; i < 30; i++) b.step(16.7);
  const t = b.api.time;
  assert.ok(t > 0.3);
  b.click();
  assert.equal(b.api.time, t, 'mid-play click ignored');
  b.runOut();
  b.click();
  assert.equal(b.api.playing, true);
  assert.equal(b.api.time, 0);
  b.runOut();
  assert.equal(b.api.playing, false);
});

test('reduced motion starts on the settled pose, and a click plays it once', () => {
  const b = boot({ calm: true });
  assert.equal(b.api.playing, false);
  assert.equal(b.queue.length, 0);
  assert.equal(b.host.dataset.state, 'settled');
  b.click();
  assert.equal(b.api.playing, true);
});
