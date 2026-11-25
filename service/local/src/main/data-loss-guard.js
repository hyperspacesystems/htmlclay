/**
 * data-loss-guard (Hyperclay Local) — the per-file guard store, detection
 * orchestration, the "restore my data" merge, and the panel transport for the
 * desktop app. CommonJS counterpart of hyperclay/server-lib/data-loss-guard.js;
 * shares the exact same detection brain (the vendored data-loss-core.cjs).
 *
 * Store: a private, sync-ignored JSON file per site at
 *   {baseDir}/.hyperclay/guard/<base>.json
 * (mirrors the .hyperclay/api/ sidecar pattern — already excluded by every sync
 * scan). Whole-file Revert backups live beside it as <base>.recover.html, or
 * fall back to the newest .hyperclay/versions/<base>/*.html for raw external writes.
 *
 * Everything here is NON-FATAL to the write: a guard failure logs and returns.
 * See plans/hyperclay-local/data-clobber-guard-plan.md.
 */
const crypto = require('crypto');
const fs = require('fs').promises;
const path = require('upath');
const cheerio = require('cheerio');
const { createRootLive } = require('./utils/root-live');
const core = require('./data-loss-core.cjs');
const { extractViaTag } = require('./utils/data-extractor');
const { compareNewestFirst } = require('./utils/prune-versions');
const { canonicalizeBase, rebaseOntoCanonical, assertRealDirChain } = require('./utils/real-dir-chain');
const { GUARD_DIR, VERSIONS_DIR, LEGACY_VERSIONS_DIR } = require('./utils/artifact-paths');

const {
  classifyDestruction,
  shouldFire,
  uiProvenance,
  lossSummary,
  islandsEqual,
  meaningful,
  isEmptyIsland,
  CODECS_KEY,
  FORM_KEY,
  codecRowChanges,
} = core;

const RULES_NAME = 'api';
const GUARD_CODECS = {};
const rooms = () => require('@panphora/clayjs/server/rooms');

// Cached dynamic import of the ESM engine (apply/findRulesIn/errors) — same
// bridge pattern as utils/data-extractor.js. cheerio is a plain require.
let enginePromise = null;
function loadEngine() {
  if (!enginePromise) {
    enginePromise = Promise.all([
      import('hyper-html-api/engine'),
      import('hyper-html-api/cheerio'),
      import('hyper-html-api/codecs/sheet'),
      import('hyper-html-api/read'),
    ]).then(([engine, cheerioAdapterMod, sheetModule, reader]) => {
      GUARD_CODECS.sheet = { rowChange: sheetModule.grid.rowChange };
      return {
        readDocument: reader.readDocument,
        documentRules: reader.documentRules,
        extract: engine.extract,
        codecOf: engine.codecOf,
        sheet: sheetModule.sheet,
        apply: engine.apply,
        findRulesIn: engine.findRulesIn,
        errors: engine.errors,
        cheerioAdapter: cheerioAdapterMod.default,
      };
    });
  }
  return enginePromise;
}

// --- extraction (async). { api: data } | null; throws on version skew. ---
async function extractIsland(html) {
  if (typeof html === 'string' && !/data-rules-name/i.test(html)) return null;
  const { extract, codecOf, sheet, findRulesIn, cheerioAdapter, readDocument, documentRules } = await loadEngine();
  const file = rooms().splitPage(html).file;
  if (file.includes('@codec(')) {
    try {
      const found = documentRules(cheerio.load, file, { token: RULES_NAME });
      const rules = found?.rules;
      const entries = rules && typeof rules === 'object' && !Array.isArray(rules) ? Object.entries(rules) : [];
      const declared = Object.fromEntries(entries.map(([key, rule]) => [key, codecOf(rule)?.name]).filter(([, name]) => name));
      if (Object.keys(declared).length) {
        const { data } = readDocument(cheerio.load, file, { token: RULES_NAME, view: 'digest' });
        return { [RULES_NAME]: data, [CODECS_KEY]: { [RULES_NAME]: declared }, [FORM_KEY]: 'stored' };
      }
    } catch {}
  }
  const root = cheerio.load(file).root();
  const found = findRulesIn(cheerioAdapter, root, RULES_NAME);
  if (!found) return null;
  const opts = { version: found.version, codecs: { sheet }, view: 'digest' };
  const declared = {};
  const ordinary = {};
  if (found.rules && typeof found.rules === 'object' && !Array.isArray(found.rules)) {
    for (const [key, rule] of Object.entries(found.rules)) {
      let codec;
      try { codec = codecOf(rule); } catch { codec = { name: '' }; }
      if (codec) declared[key] = codec.name;
      else ordinary[key] = rule;
    }
  }
  if (!Object.keys(declared).length) {
    const data = extract(cheerioAdapter, root, found.rules, opts);
    return data == null ? null : { [RULES_NAME]: data, [FORM_KEY]: 'stored' };
  }
  const data = extract(cheerioAdapter, root, ordinary, opts) || {};
  for (const key of Object.keys(declared)) {
    try {
      data[key] = extract(cheerioAdapter, root, { [key]: found.rules[key] }, opts)[key];
    } catch {}
  }
  return { [RULES_NAME]: data, [CODECS_KEY]: { [RULES_NAME]: declared }, [FORM_KEY]: 'stored' };
}
async function safeExtractIsland(html) {
  try {
    return { ok: true, island: await extractIsland(html) };
  } catch {
    return { ok: false, island: null };
  }
}

function namesExactVersion(ifMatch) {
  if (typeof ifMatch !== 'string') return false;
  const tags = ifMatch.split(',').map((tag) => tag.trim()).filter(Boolean);
  return tags.length > 0 && !tags.includes('*');
}

const islandHash = core.islandHash;

// classifyDestruction + log the cost-guard fallback (plan §4: "log() the skip").
// The pure core only flags it on D; the environment wrapper does the logging.
function classify(base, inc) {
  const D = classifyDestruction(base, inc, { codecs: GUARD_CODECS });
  if (D.fuzzyFallback) {
    console.warn('[data-guard] item list too large for fuzzy matching; used a coarser index-aligned compare (detection may be less precise).');
  }
  return D;
}

// The loss is undone only when the current island still CONTAINS every pinned
// recoverable atom (additions allowed). `!anyDestruction` alone is too loose: it
// also passes when a recoverable atom was MODIFIED to a different value or reset
// to a placeholder, which would silently drop the recovery chip without the data
// ever coming back. D here is classify(recoverableData, currentIsland).
function lossUndone(D) {
  return !D.anyDestruction && D.modifiedAtoms === 0 && D.placeholderResets === 0;
}

// ---------------------------------------------------------------------------
// Store — one JSON file per site, serialized by a per-file in-process lock.
// ---------------------------------------------------------------------------
const locks = new Map();
function withLock(key, fn) {
  const prev = locks.get(key) || Promise.resolve();
  const next = prev.then(fn, fn);
  // Keep the chain alive but don't leak rejections into the next waiter.
  locks.set(key, next.catch(() => {}));
  return next;
}

function guardRelPath(name) {
  const base = name.replace(/\.(html|htmlclay)$/, '');
  return path.join(GUARD_DIR, base + '.json');
}
function resolveGuardPath(baseDir, name) {
  const abs = path.resolve(path.join(baseDir, guardRelPath(name)));
  const root = path.resolve(baseDir);
  if (!abs.startsWith(root + path.sep)) throw new Error('Guard path escapes base directory');
  return abs;
}
function recoverHtmlPath(baseDir, name) {
  const base = name.replace(/\.(html|htmlclay)$/, '');
  const abs = path.resolve(path.join(baseDir, GUARD_DIR, base + '.recover.html'));
  const root = path.resolve(baseDir);
  if (!abs.startsWith(root + path.sep)) throw new Error('Recover path escapes base directory');
  return abs;
}

async function readGuard(baseDir, name) {
  try {
    const txt = await fs.readFile(resolveGuardPath(baseDir, name), 'utf8');
    return JSON.parse(txt);
  } catch {
    return null;
  }
}
async function writeGuard(baseDir, name, guard) {
  const unstampedKey = `${baseDir}::${name}`;
  if (guard.baseline && guard.lastSeen === undefined && unstampedSeen.has(unstampedKey)) {
    guard.lastSeen = unstampedSeen.get(unstampedKey);
    unstampedSeen.delete(unstampedKey);
  }
  const abs = resolveGuardPath(baseDir, name);
  await fs.mkdir(path.dirname(abs), { recursive: true });
  // Atomic write: a crash mid-write would otherwise corrupt the JSON and lose
  // the pinned recoverableData (readGuard self-heals to an empty guard).
  const tmp = abs + '.tmp';
  await fs.writeFile(tmp, JSON.stringify(guard), 'utf8');
  await fs.rename(tmp, abs);
}
function emptyGuard() {
  return { baseline: null, uiWorkPending: false, event: null, status: 'none' };
}

// Newest <versions>/<base>/*.html (the last-good full file for a raw write).
// Ranked by parsed instant, NOT by filename: legacy backup names are local wall
// time, which repeats across a DST fall-back, so a lexical sort could hand back
// the older of the two as "newest" and silently revert to stale content.
async function newestVersionPath(baseDir, name) {
  const base = name.replace(/\.(html|htmlclay)$/, '');
  // .hyperclay/versions first; sites-versions is the pre-.hyperclay location and
  // stays readable, so a file whose last backup predates the move still recovers.
  for (const versionsDir of [VERSIONS_DIR, LEGACY_VERSIONS_DIR]) {
    const found = await newestVersionIn(baseDir, versionsDir, base);
    if (found) return found;
  }
  return null;
}

async function newestVersionIn(baseDir, versionsDir, base) {
  try {
    const dir = path.join(baseDir, versionsDir, base);
    // Refuse a symlinked chain: the path this returns feeds a Revert that
    // overwrites the live file, so it must not resolve out of the served tree.
    const canonicalBase = await canonicalizeBase(baseDir);
    await assertRealDirChain(canonicalBase, rebaseOntoCanonical(canonicalBase, baseDir, dir));
    const files = (await fs.readdir(dir)).filter((f) => f.endsWith('.html'));
    if (!files.length) return null;

    const ranked = [];
    for (const file of files) {
      const full = path.join(dir, file);
      try {
        ranked.push({ full, name: file, mtimeMs: (await fs.stat(full)).mtimeMs });
      } catch {}
    }
    if (!ranked.length) return null;
    // Same comparator the pruner uses, so "newest" means one thing everywhere:
    // instant first, then the collision suffix that orders a same-millisecond
    // burst.
    ranked.sort(compareNewestFirst);
    return ranked[0].full;
  } catch {
    return null;
  }
}

// ---------------------------------------------------------------------------
// runDataLossGuard — one detection pass for all three local write paths.
//
// baseDir       : sync folder
// name          : file path with extension (liveSync key shape)
// newHtml       : the bytes just written
// prevContent   : pre-write body (string) or null (raw watcher has only a hash)
// prov          : 'external' | 'ui-background' | 'ui-gestured'
//
// live          : the served root's live transport; the personal one by default
//
// Returns the client-safe event when a loss is raised, else null. Never throws.
// ---------------------------------------------------------------------------
async function runDataLossGuard({ baseDir, name, newHtml, prevContent, prov, ifMatched = false, live = createRootLive(null) }) {
  try {
    const inc = await safeExtractIsland(newHtml);

    let raised = null;
    let autoResolvedId = null;

    await withLock(`${baseDir}::${name}`, async () => {
      const guard = (await readGuard(baseDir, name)) || emptyGuard();
      normalizeComposedBaseline(guard, prevContent);
      if (!inc.ok && !guard.event && (!guard.baseline || isEmptyIsland(guard.baseline.data))) {
        const seed = guard.baseline || prevContent == null ? null : await safeExtractIsland(prevContent);
        if (!seed || !seed.ok || !seed.island || isEmptyIsland(seed.island)) return;
      }

      // Cold start (seed-on-first-sight from KNOWN-GOOD content only).
      if (!guard.baseline) {
        if (prevContent != null) {
          const seed = await safeExtractIsland(prevContent);
          if (seed.ok && seed.island && !isEmptyIsland(seed.island)) {
            guard.baseline = { data: seed.island, hash: islandHash(seed.island), at: Date.now() };
            guard.uiWorkPending = false;
            // fall through and detect THIS write against the seed
          } else {
            await seedBlind(guard, inc.island);
            await writeGuard(baseDir, name, guard);
            return;
          }
        } else {
          // Raw watcher first sight — blind seed, can't detect this write.
          await seedBlind(guard, inc.island);
          await writeGuard(baseDir, name, guard);
          return;
        }
      }

      const base = guard.baseline.data;

      if (guard.event) {
        // Cross-environment auto-clear: the loss is undone once an incoming write
        // still CONTAINS every pinned recoverable atom — an exact restore, OR a
        // restore that also kept legitimate additions. `base` is the pinned
        // recoverableData while an event is open (pin-once never advances it), so
        // D measures destruction/modification of the recoverable set. A write that
        // merely modifies a recoverable atom to a new value has NOT restored it, so
        // lossUndone stays false and the (valid) warning is preserved.
        const D = classify(base, inc.island);
        if (lossUndone(classify(comparableRecovery(guard.event.recoverableData, newHtml), inc.island))) {
          autoResolvedId = guard.event.id;
          guard.event = null;
          guard.status = 'restored';
          guard.uiWorkPending = false;
          guard.baseline = { data: inc.island, hash: islandHash(inc.island), at: Date.now() };
          await writeGuard(baseDir, name, guard);
          return;
        }
        if (shouldFire(prov, guard.uiWorkPending && !ifMatched, base, inc.island, D)) {
          guard.event.lastWriteAt = Date.now();
        }
        if (prov === 'external') guard.uiWorkPending = false;
        await writeGuard(baseDir, name, guard);
        return;
      }

      const D = classify(base, inc.island);
      const fire = shouldFire(prov, guard.uiWorkPending && !ifMatched, base, inc.island, D);

      if (fire) {
        const recoverableHtmlBackup = await captureRecoverPath(baseDir, name, prevContent);
        guard.event = {
          id: crypto.randomUUID(),
          recoverableData: base,
          recoverableHtmlBackup,
          firstDetectedAt: Date.now(),
          lastWriteAt: Date.now(),
          lossSummary: lossSummary(prov, D),
          status: 'pending',
        };
        guard.status = 'pending';
        if (prov === 'external') guard.uiWorkPending = false;
        await writeGuard(baseDir, name, guard);
        raised = guard.event;
        return;
      }

      if (prov === 'ui-gestured') {
        guard.baseline = { data: inc.island, hash: islandHash(inc.island), at: Date.now() };
        guard.uiWorkPending = true;
      } else if (prov === 'external') {
        guard.uiWorkPending = false;
      }
      await writeGuard(baseDir, name, guard);
    });

    if (autoResolvedId) {
      notifyResolved(name, autoResolvedId, live);
      return null;
    }
    if (raised) {
      const clientEvent = await toClientEvent(raised, newHtml);
      notifyRaised(name, clientEvent, live);
      return clientEvent;
    }
    return null;
  } catch (e) {
    console.error('[data-guard] runDataLossGuard failed (non-fatal):', e && e.message ? e.message : e);
    return null;
  }
}

function stripPageState(value) {
  if (typeof value === 'string') {
    if (!value.includes('merge=')) return value;
    const { file, state } = rooms().splitPage(value);
    return state ? file : value;
  }
  if (!value || typeof value !== 'object') return value;
  let copy = null;
  for (const [key, child] of Object.entries(value)) {
    const next = stripPageState(child);
    if (next === child) continue;
    copy ??= Array.isArray(value) ? [...value] : { ...value };
    copy[key] = next;
  }
  return copy ?? value;
}

function isBlockEditorDocument(html) {
  return typeof html === 'string' && /<article/i.test(html) && rooms().pageRev(html);
}

function comparableRecovery(recoverable, html) {
  return recoverable?.[FORM_KEY] === 'stored' || !isBlockEditorDocument(html) ? recoverable : stripPageState(recoverable);
}

function normalizeComposedBaseline(guard, prevContent) {
  if (!guard.baseline || guard.baseline.data[FORM_KEY] === 'stored' || !isBlockEditorDocument(prevContent)) return;
  const data = { ...stripPageState(guard.baseline.data), [FORM_KEY]: 'stored' };
  guard.baseline.data = data;
  guard.baseline.hash = islandHash(data);
}

async function seedBlind(guard, island) {
  if (island && !isEmptyIsland(island)) {
    guard.baseline = { data: island, hash: islandHash(island), at: Date.now() };
  }
  guard.uiWorkPending = false;
}

// The whole last-good file for Revert, always COPIED INTO THE GUARD'S OWN
// STORAGE. Prefer the pre-write body; else the newest version entry.
//
// Returning a versions path directly (as this used to for raw writes)
// would pin a file the retention pruner is free to delete. The alternative —
// teaching the pruner an exemption list — would need that list consulted under
// a lock the pruner does not hold, which is a race. Copying the bytes here
// keeps the pruner a dumb, predictable function with no shared state.
async function captureRecoverPath(baseDir, name, prevContent) {
  let body = prevContent;

  if (body == null) {
    const versionPath = await newestVersionPath(baseDir, name);
    if (versionPath == null) return null;
    try {
      body = await fs.readFile(versionPath, 'utf8');
    } catch (e) {
      console.error('[data-guard] captureRecoverPath read failed:', e && e.message ? e.message : e);
      return null;
    }
  }

  try {
    const abs = recoverHtmlPath(baseDir, name);
    await fs.mkdir(path.dirname(abs), { recursive: true });
    await fs.writeFile(abs, body, 'utf8');
    return abs;
  } catch (e) {
    console.error('[data-guard] captureRecoverPath write failed:', e && e.message ? e.message : e);
    return null;
  }
}

// ---------------------------------------------------------------------------
// Provenance for the local /save path (always a UI save, split by userDriven).
// ---------------------------------------------------------------------------

// What caused this save. Spec §9 defines `Save-Trigger: user` (a human gesture)
// and `auto` (autosave or script), and obliges a host to treat any unknown value
// as `auto`, so document code cannot widen its own privileges by inventing one.
// The pre-spec X-Hyperclay-User-Driven header is read when Save-Trigger is
// absent, since hyperclayjs 1.x still sends that spelling. Neither header ->
// undefined (old client; tie-break silent), which is deliberately not the same
// answer as `auto`. Same three states as the platform's reader.
function userDrivenFromHeader(req) {
  const header = (name) => (req && typeof req.get === 'function' && req.get(name)) || '';
  const trigger = header('save-trigger').trim().toLowerCase();
  if (trigger) return trigger === 'user';
  const h = header('x-hyperclay-user-driven');
  if (h === '1' || h === 'true') return true;
  if (h === '0' || h === 'false') return false;
  return undefined;
}

function provenanceForLocalSave(userDriven) {
  return uiProvenance(userDriven);
}

// ---------------------------------------------------------------------------
// Live transport.
// ---------------------------------------------------------------------------
function notifyRaised(name, clientEvent, live = createRootLive(null)) {
  try {
    live.notify(name, { msgType: 'data-loss', action: 'raised', msg: 'Saved data overwritten', data: clientEvent });
  } catch (e) {
    console.error('[data-guard] notifyRaised failed:', e && e.message ? e.message : e);
  }
}
function notifyResolved(name, eventId, live = createRootLive(null)) {
  try {
    live.notify(name, { msgType: 'data-loss', action: 'resolved', msg: 'Data guard resolved', data: { id: eventId } });
  } catch (e) {
    console.error('[data-guard] notifyResolved failed:', e && e.message ? e.message : e);
  }
}

// ---------------------------------------------------------------------------
// "Restore my data" merge (async). See the platform note re shapeMatch.
// ---------------------------------------------------------------------------
async function restoreDataOntoHtml(currentHtml, recoverableData) {
  const { apply, findRulesIn, cheerioAdapter } = await loadEngine();
  const $ = cheerio.load(currentHtml);
  const root = $.root();
  let found;
  try {
    found = findRulesIn(cheerioAdapter, root, RULES_NAME);
  } catch {
    return { failed: RULES_NAME };
  }
  if (!found || !recoverableData || !(RULES_NAME in recoverableData)) return { failed: RULES_NAME };
  if (Object.keys(declaredCodecs(recoverableData)).length) return { failed: RULES_NAME };
  const { rules } = found;
  const data = recoverableData[RULES_NAME];
  try {
    apply(cheerioAdapter, root, rules, data);
  } catch (e) {
    // Any apply failure (ShapeMismatch, EmptyListInsert when a clobber emptied a
    // list so there's no sibling to clone, etc.) means we can't cleanly
    // round-trip — offer Revert instead of writing a wrong restore.
    return { failed: RULES_NAME };
  }
  const mergedHtml = $.html();
  let reExtracted;
  try {
    reExtracted = await extractViaTag(mergedHtml, RULES_NAME);
  } catch {
    return { failed: RULES_NAME };
  }
  if (!islandsEqual({ [RULES_NAME]: reExtracted }, { [RULES_NAME]: data })) return { failed: RULES_NAME };
  return { html: mergedHtml };
}

// ---------------------------------------------------------------------------
// Client-safe event payload (now/yours preview + Restore dry-run).
// ---------------------------------------------------------------------------
async function toClientEvent(event, currentHtml) {
  const recoverable = event.recoverableData || {};
  let currentIsland = null;
  try {
    currentIsland = await extractIsland(currentHtml);
  } catch {
    currentIsland = null;
  }
  const dry = await restoreDataOntoHtml(currentHtml, recoverable);
  const preview = buildPreview(recoverable, currentIsland);
  return {
    id: event.id,
    firstDetectedAt: event.firstDetectedAt,
    lastWriteAt: event.lastWriteAt,
    lossSummary: event.lossSummary,
    fieldCount: preview.rows.length,
    preview: preview.rows,
    droppedAdditions: core.droppedItems(ordinaryData(recoverable), ordinaryData(currentIsland)),
    restorable: !dry.failed,
    canRevert: event.recoverableHtmlBackup != null,
    status: event.status || 'pending',
  };
}

const PREVIEW_CODEC_ROWS = 50;

function declaredCodecs(island) {
  const declared = island && island[CODECS_KEY] && island[CODECS_KEY][RULES_NAME];
  return declared && typeof declared === 'object' ? declared : {};
}

// The island's data without its codec keys, for the readers that know only ordinary values.
function ordinaryData(island) {
  const data = island ? island[RULES_NAME] : null;
  if (!data || typeof data !== 'object' || Array.isArray(data)) return data ?? null;
  const declared = declaredCodecs(island);
  return Object.fromEntries(Object.entries(data).filter(([key]) => !(key in declared)));
}

function cellCount(n) {
  return `${n} filled cell${n === 1 ? '' : 's'}`;
}

function codecPreviewRows(recoverable, current) {
  const rows = [];
  const was = declaredCodecs(recoverable);
  const now = declaredCodecs(current);
  const changed = [];
  for (const [key, name] of Object.entries(was)) {
    const codec = now[key] === name ? GUARD_CODECS[name] || null : null;
    const baseView = recoverable[RULES_NAME] ? recoverable[RULES_NAME][key] : undefined;
    const incView = codec && current && current[RULES_NAME] ? current[RULES_NAME][key] : undefined;
    for (const { tab, tabIndex, before, now: after, change } of codecRowChanges(baseView, incView, codec)) {
      if (change !== 'destruction' && change !== 'modification') continue;
      const tabName = tab.name || tab.id || `Sheet ${tabIndex + 1}`;
      const rowName = before[0] !== undefined && before[0] !== null && before[0] !== '' ? `row ${before[0]}` : 'a row';
      const nowText = !after ? '— removed —' : change === 'modification' ? `changed, ${cellCount(after[1])}` : cellCount(after[1]);
      (change === 'destruction' ? rows : changed).push({ key: `${key} › ${tabName} › ${rowName}`, now: nowText, yours: cellCount(before[1]) });
    }
  }
  // Destroyed rows first, so changed rows never push a lost row out of the shown ones.
  const all = rows.concat(changed);
  const shown = all.slice(0, PREVIEW_CODEC_ROWS);
  if (all.length > shown.length) shown.push({ key: 'more rows', now: '', yours: `${all.length - shown.length} more` });
  return shown;
}

function buildPreview(recoverable, current) {
  const recoverableData = ordinaryData(recoverable);
  const currentData = ordinaryData(current);
  const rows = [];
  const rd = recoverableData && typeof recoverableData === 'object' ? recoverableData : {};
  const cd = currentData && typeof currentData === 'object' ? currentData : {};

  if (Array.isArray(rd) || Array.isArray(cd)) {
    // Top-level list island: one synthetic row summarizing item counts.
    const yoursN = Array.isArray(rd) ? rd.length : 0;
    const nowN = Array.isArray(cd) ? cd.length : 0;
    if (yoursN !== nowN) {
      rows.push({ key: 'items', now: nowN ? `${nowN} items` : '— empty —', yours: yoursN ? `${yoursN} items` : '— empty —' });
    }
    return { rows };
  }

  const wasCodec = declaredCodecs(recoverable);
  const nowCodec = declaredCodecs(current);
  const keys = new Set([...Object.keys(rd), ...Object.keys(cd)]);
  for (const key of keys) {
    if (key in wasCodec) continue;
    const yours = formatValue(rd[key]);
    const now = key in nowCodec ? `replaced by a ${nowCodec[key]}` : formatValue(cd[key]);
    if (yours !== now) rows.push({ key, now, yours });
  }
  rows.push(...codecPreviewRows(recoverable || {}, current));
  return { rows };
}

function formatValue(v) {
  if (!meaningful(v)) return '— empty —';
  if (typeof v === 'string') return truncate(v.trim());
  if (typeof v === 'number' || typeof v === 'boolean') return String(v);
  if (Array.isArray(v)) {
    if (v.every((x) => x !== null && typeof x === 'object' && !Array.isArray(x))) {
      return `${v.length} item${v.length === 1 ? '' : 's'}`;
    }
    return truncate(v.map((x) => (typeof x === 'object' ? JSON.stringify(x) : String(x))).join(', '));
  }
  if (typeof v === 'object') return truncate(JSON.stringify(v));
  return String(v);
}
function truncate(s, n = 80) {
  s = String(s);
  return s.length > n ? s.slice(0, n - 1) + '…' : s;
}

// ---------------------------------------------------------------------------
// Page-load read. Seeds the baseline from the current disk island on first
// sight (external-blessed) so an already-open file is covered.
// ---------------------------------------------------------------------------
async function getGuardEvent(baseDir, name, currentHtml, live = createRootLive(null)) {
  try {
    const guard = await readGuard(baseDir, name);
    if (guard && guard.event) {
      // Re-validate on read: if the current content still contains every pinned
      // recoverable atom, the loss was already undone (restored/reverted elsewhere
      // and synced here) — clear the stale event instead of showing a dead chip.
      const cur = currentHtml != null ? await safeExtractIsland(currentHtml) : { ok: false };
      if (cur.ok && cur.island && lossUndone(classify(comparableRecovery(guard.event.recoverableData, currentHtml), cur.island))) {
        const resolvedId = guard.event.id;
        await clearEvent(baseDir, name, cur.island, 'restored');
        notifyResolved(name, resolvedId, live);
        return null;
      }
      return await toClientEvent(guard.event, currentHtml != null ? currentHtml : '');
    }
    if (!guard?.baseline && currentHtml != null) {
      const seed = await safeExtractIsland(currentHtml);
      if (seed.ok && seed.island && !isEmptyIsland(seed.island)) {
        await withLock(`${baseDir}::${name}`, async () => {
          const g = (await readGuard(baseDir, name)) || emptyGuard();
          if (!g.baseline) {
            g.baseline = { data: seed.island, hash: islandHash(seed.island), at: Date.now() };
            g.uiWorkPending = false;
            await writeGuard(baseDir, name, g);
          }
        });
      }
    }
    return null;
  } catch (e) {
    console.error('[data-guard] getGuardEvent failed (non-fatal):', e && e.message ? e.message : e);
    return null;
  }
}

// ---------------------------------------------------------------------------
// The outside-edit catch-up. The root observer starts with ignoreInitial, so an
// edit made while Local was closed has no watcher pass behind it and no notice.
// The compare is the bytes on disk against the last bytes the guard or the host
// saw — never against the island baseline — so a save that does not advance
// the baseline (ui-unknown) stops mattering: its bytes are simply the last ones
// seen.
//
// pendingSeen is set synchronously by the host, before it renames new bytes into
// place, so a page load that reads those bytes while the save is still finishing
// or before the guard persist lands still sees them as the host's own.
// ---------------------------------------------------------------------------
const pendingSeen = new Map();

// A save's stamp that arrived before its document had a guard baseline. The save's own guard pass
// extracts the island before it takes the lock, so on a new document the stamp lands first and
// finds nothing to write into. It waits here, and the first write of a baseline records it.
const unstampedSeen = new Map();

// Called by the host just before it renames new bytes into place, so a page load that reads them
// while the save is still finishing knows they are the host's own.
function markPending(baseDir, name, bytes) {
  pendingSeen.set(`${baseDir}::${name}`, crypto.createHash('sha256').update(bytes).digest('hex'));
}

function noteSeen(baseDir, name, bytes) {
  const key = `${baseDir}::${name}`;
  const hash = crypto.createHash('sha256').update(bytes).digest('hex');
  pendingSeen.set(key, hash);
  return withLock(key, async () => {
    const guard = await readGuard(baseDir, name);
    if (guard && guard.baseline) {
      unstampedSeen.delete(key);
      if (guard.lastSeen !== hash) {
        guard.lastSeen = hash;
        await writeGuard(baseDir, name, guard);
      }
    } else {
      unstampedSeen.set(key, hash);
    }
  }).catch((e) => console.error('[data-guard] noteSeen failed:', e && e.message ? e.message : e))
    .finally(() => { if (pendingSeen.get(key) === hash) pendingSeen.delete(key); });
}

// A guard written before lastSeen existed is seeded on first read and raises
// nothing: the one edit that could have happened across the upgrade is not
// flagged. A crash between the rename and the persist makes the next read run
// one external pass on the host's own bytes, which raises a notice only if
// those bytes really lost data against the baseline.
async function catchUpOutsideEdit(baseDir, name, bytes, html, live) {
  const hash = crypto.createHash('sha256').update(bytes).digest('hex');
  if (pendingSeen.get(`${baseDir}::${name}`) === hash) return;
  const guard = await readGuard(baseDir, name);
  if (!guard || !guard.baseline || guard.lastSeen === hash) return;
  if (guard.lastSeen !== undefined) {
    await runDataLossGuard({ baseDir, name, newHtml: html, prevContent: null, prov: 'external', live });
  }
  await noteSeen(baseDir, name, bytes);
}

// ---------------------------------------------------------------------------
// Resolve (dismiss / revert / restore). writeBack performs the actual save
// through the caller's backup-then-write helper so resolution is versioned.
// ---------------------------------------------------------------------------
async function resolveGuard({ baseDir, name, id, choice, currentHtml, writeBack, live = createRootLive(null) }) {
  const guard = await readGuard(baseDir, name);
  if (!guard || !guard.event) return { ok: false, statusCode: 404, error: 'No pending data-loss event' };
  if (id && guard.event.id !== id) return { ok: false, statusCode: 409, error: 'Event id mismatch (already resolved?)' };
  const event = guard.event;

  if (choice === 'dismiss') {
    const cur = await safeExtractIsland(currentHtml || '');
    // forceBaseline: Dismiss accepts current even when the clobber emptied the
    // island, else the old baseline stays and the next write re-fires.
    await clearEvent(baseDir, name, cur.ok ? cur.island : null, 'dismissed', true);
    notifyResolved(name, event.id, live);
    // rider 1: return the control payload so server.js can nudge the platform
    // (and thence the owner's other devices). resolveGuard stays transport-pure.
    return {
      ok: true, choice, status: 'dismissed',
      control: { fileKey: name, recoverableDataHash: islandHash(event.recoverableData) },
    };
  }

  if (choice === 'restore') {
    const merged = await restoreDataOntoHtml(currentHtml || '', event.recoverableData);
    if (merged.failed) return { ok: false, statusCode: 422, error: 'Data could not be restored onto the current page; use Revert.' };
    await writeBack(merged.html);
    const after = await safeExtractIsland(merged.html);
    await clearEvent(baseDir, name, after.ok ? after.island : event.recoverableData, 'restored');
    notifyResolved(name, event.id, live);
    return { ok: true, choice, status: 'restored' };
  }

  if (choice === 'revert') {
    if (!event.recoverableHtmlBackup) return { ok: false, statusCode: 422, error: 'No whole-file backup is available to revert to.' };
    // Containment guard: the recover path is always under baseDir
    // (.hyperclay/guard or .hyperclay/versions); reject anything else in case a
    // tampered/corrupt guard file points the read elsewhere.
    const recoverAbs = path.resolve(event.recoverableHtmlBackup);
    if (!recoverAbs.startsWith(path.resolve(baseDir) + path.sep)) {
      return { ok: false, statusCode: 422, error: 'Recover path is outside the sync folder.' };
    }
    let html;
    try {
      html = await fs.readFile(recoverAbs, 'utf8');
    } catch {
      return { ok: false, statusCode: 422, error: 'Could not read the backup to revert to.' };
    }
    await writeBack(html);
    const after = await safeExtractIsland(html);
    await clearEvent(baseDir, name, after.ok ? after.island : null, 'reverted');
    notifyResolved(name, event.id, live);
    return { ok: true, choice, status: 'reverted' };
  }

  return { ok: false, statusCode: 400, error: 'Unknown choice' };
}

// The read-modify-write body of a clear, WITHOUT withLock. Shared by clearEvent
// (which wraps it in withLock) and applyRemoteResolution (which already holds the
// lock for the whole check-and-clear). Splitting this out is what avoids the
// non-reentrant withLock re-acquiring itself and hanging (plan §4c).
async function clearEventLocked(baseDir, name, keptIsland, status, forceBaseline = false) {
  const g = (await readGuard(baseDir, name)) || emptyGuard();
  g.event = null;
  g.status = status;
  g.uiWorkPending = false;
  if (forceBaseline) {
    // Dismiss: adopt current as the baseline even when empty (the SCOPE gate
    // keeps it quiet until data returns), so a dismissed wipe stays dismissed.
    g.baseline = keptIsland ? { data: keptIsland, hash: islandHash(keptIsland), at: Date.now() } : null;
  } else if (keptIsland && !isEmptyIsland(keptIsland)) {
    g.baseline = { data: keptIsland, hash: islandHash(keptIsland), at: Date.now() };
  }
  await writeGuard(baseDir, name, g);
}

async function clearEvent(baseDir, name, keptIsland, status, forceBaseline = false) {
  await withLock(`${baseDir}::${name}`, async () => {
    await clearEventLocked(baseDir, name, keptIsland, status, forceBaseline);
  });
}

// ---------------------------------------------------------------------------
// applyRemoteResolution — apply an inbound cross-device Dismiss (control lane,
// rider 1). Clears THIS device's matching event iff the pinned recoverableData
// hash equals the message's; never creates, never clears on mismatch. The whole
// check-and-clear runs under ONE withLock (via clearEventLocked, NOT clearEvent,
// which would re-acquire the same non-reentrant lock and hang). Non-fatal.
// Returns true only if this side actually cleared.
// ---------------------------------------------------------------------------
async function applyRemoteResolution({ baseDir, name, recoverableDataHash }) {
  try {
    // Remote input — this is THE containment guard for the rider: reject '..',
    // absolute, backslash, non-site, or a bad hash before any fs read.
    // (handleControlFrame is a generic transport hop and does NOT pre-validate a
    // rider's fileKey; readGuard/writeGuard also route through resolveGuardPath,
    // which throws on escape — defense in depth.)
    if (typeof name !== 'string' || name.startsWith('/') || name.includes('..') || name.includes('\\')) return false;
    if (!/\.(html|htmlclay)$/.test(name)) return false;
    if (!/^[0-9a-f]{64}$/.test(recoverableDataHash || '')) return false;
    let resolvedId = null;
    await withLock(`${baseDir}::${name}`, async () => {
      const g = await readGuard(baseDir, name);
      if (!g || !g.event) return;                                              // never create
      if (islandHash(g.event.recoverableData) !== recoverableDataHash) return; // keep warning on mismatch
      resolvedId = g.event.id;                                                 // real id for notifyResolved
      let cur = { ok: false, island: null };
      try { cur = await safeExtractIsland(await fs.readFile(path.join(baseDir, name), 'utf8')); } catch {}
      await clearEventLocked(baseDir, name, cur.ok ? cur.island : null, 'dismissed', true); // NO nested withLock
    });
    if (resolvedId) notifyResolved(name, resolvedId);
    return resolvedId != null;
  } catch (e) {
    console.error('[data-guard] applyRemoteResolution failed (non-fatal):', e && e.message ? e.message : e);
    return false;
  }
}

module.exports = {
  extractIsland,
  namesExactVersion,
  runDataLossGuard,
  provenanceForLocalSave,
  userDrivenFromHeader,
  getGuardEvent,
  noteSeen,
  markPending,
  catchUpOutsideEdit,
  resolveGuard,
  applyRemoteResolution,
  restoreDataOntoHtml,
  toClientEvent,
  // test seams
  _readGuard: readGuard,
  _writeGuard: writeGuard,
  _resolveGuardPath: resolveGuardPath,
  _captureRecoverPath: captureRecoverPath,
  _newestVersionPath: newestVersionPath,
};
