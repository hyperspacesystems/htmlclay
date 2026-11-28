// Re-exports the vendored hyper-undo singleton and auto-starts it in edit mode
// so apps "just work". The loader wires it onto clay.undo; the vendor's own
// window auto-export is suppressed by the bootstrap's __hyperclayNoAutoExport
// flag.

import { undo } from '../vendor/hyper-undo.vendor.js'
import { isEditMode, startsAtImport } from '../core/is-edit-mode.js'
import { isTabLocalAttr } from '../lib/root-attrs.js'
import { isDerivedAttr } from '../lib/derived.js'
import { PERSIST } from '../lib/attr-aliases.js'
import { isGestureItem, isRuntimeAttribute } from '../lib/undo/capture-move.js'
import { gateMarkDirty } from '../lib/dirty-gate.js'

// Where the caret goes after an undo or redo: at the text that changed.
//
// The engine's receipt carries a hint for it: `{ node, offset }` for a text run, and
// `{ root, textOffset }` for a field whose saved content is its text. The person goes on
// typing from there, so the field takes focus when they were already typing in a field.
// When they were not (a button, the dock's Undo on a phone), the field is only scrolled
// into view: focusing it would open the keyboard. A structural step carries no hint, so
// an item coming back moves neither the caret nor the focus.

const EDITABLE = '[contenteditable]:not([contenteditable="false"])'

// The text node an offset into the field's text lands on: count the field's text nodes
// until the offset is inside one. Past the end is the end of the last text node, and an
// empty field is the field itself.
function resolve(root, textOffset) {
  let left = Number(textOffset) || 0
  if (left < 0) left = 0
  const doc = root.ownerDocument || document
  const walker = doc.createTreeWalker(root, 4)
  let last = null
  for (let node = walker.nextNode(); node; node = walker.nextNode()) {
    if (left <= node.data.length) return { node, offset: left }
    left -= node.data.length
    last = node
  }
  return last ? { node: last, offset: last.data.length } : { node: root, offset: root.childNodes.length }
}

// The field the hint is inside. A field in a hidden section is neither scrolled to nor
// focused, and a node that is no longer in the page has nowhere to put a caret.
function hostOf(node) {
  const element = node.nodeType === 1 ? node : node.parentElement
  const host = element?.closest?.(EDITABLE)
  if (!host || !host.isConnected) return null
  if (typeof host.checkVisibility === 'function' && host.checkVisibility() === false) return null
  return host
}

function placeCaret(hint) {
  if (!hint) return
  const at = hint.root ? resolve(hint.root, hint.textOffset) : hint
  if (!at?.node) return
  const host = hostOf(at.node)
  if (!host) return
  if (typeof host.scrollIntoView === 'function') host.scrollIntoView({ block: 'nearest', inline: 'nearest' })
  if (!document.activeElement?.closest?.(EDITABLE)) return
  if (document.activeElement !== host) host.focus({ preventScroll: true })
  document.getSelection()?.collapse(at.node, at.offset)
}

// Tab-local attributes (ephemeral state, route-active, root library state) are
// never undone, and neither is what a drag's vendor writes on the item it holds:
// `draggable` and `style` are the gesture's own bookkeeping, and a click that
// writes them is not an edit. The shared mutation hub already drops tab-local
// attributes for the main instance; a scope made with create() runs its own
// observer, so it is told too.
// The page's own maintenance attributes, kept outside history by name (a card number a
// repair rewrites, for example): saved like any attribute, never an edit to undo.
const pageIgnored = new Set()
// What a structural undo step compares for a node whose live markup is not its saved
// markup. An editor that owns a field registers one function (`clay.undo.addDurableForm`):
// it answers a string for a node it owns and undefined for every other node.
const durableForms = new Set()
const durableFormOf = (node, host = null) => {
  for (const form of durableForms) {
    const own = form(node, host)
    if (typeof own === 'string') return own
  }
  return undefined
}
const withTabLocal = (opts = {}) => {
  const own = opts.ignoreAttribute
  return {
    ...opts,
    ignoreAttribute: (name, target, host) => pageIgnored.has(name) || isTabLocalAttr(name, target) || isDerivedAttr(target, name, host) || isRuntimeAttribute(name, target) || (own ? own(name, target, host) : false),
    isProtected: opts.isProtected || keepsOwnHistory,
    ignoreNode: opts.ignoreNode || isGestureItem,
    durableForm: opts.durableForm || durableFormOf,
  }
}

// Saved content that undo does not record (an authored no-undo block) has a history
// undo cannot restore, so a parent's undo or redo must not erase it. A node with a
// durable form (a mounted rich field) is certified by that form instead, and chrome
// that is never saved (editor-ui) loses nothing; neither is protected.
function keepsOwnHistory(node) {
  const policy = window.clay?.region?.resolveRegionPolicy
  if (!policy || node.nodeType !== 1) return false
  if (durableFormOf(node) !== undefined) return false
  const own = policy(node)
  if (own.undoable !== false || own.persist === 'none') return false
  const parent = node.parentElement
  return !parent || policy(parent).undoable !== false
}

const createScope = undo.create.bind(undo)
undo.create = (opts) => createScope(withTabLocal(opts))

// The recording mode is the page's choice, and a session starts a new scope over the
// page as it stands: remember it here, so an exit and a re-entry come back in the mode
// the page picked instead of the default one.
let recording = null
const setRecording = undo.setRecording.bind(undo)
undo.setRecording = (mode) => {
  recording = mode ?? null
  document.dispatchEvent(new CustomEvent('clay:undo-recording', { detail: { mode: recording } }))
  return setRecording(mode)
}

Object.defineProperty(undo, 'recordingMode', { get: () => recording, configurable: true })

// An authored history action is a user edit, and it writes the DOM while a sync
// frame's apply may hold both the mutation hub and the gate paused: neither feed
// records it then, and the apply would take the write into its saved baseline.
// So every commit, command, undo and redo that changed the document marks the
// page dirty at the gate's own entry point, whose count is what the capture token
// is compared against — an explicit mark advances it even while the gate is paused.
const lastEntryId = () => undo.history.at(-1)?.id ?? null

const command = undo.command.bind(undo)
undo.command = (label, fn) => {
  const receipt = command(label, fn)
  if (receipt) gateMarkDirty()
  return receipt
}

// Undo and redo mark it in afterReplay, which the engine calls on every path that
// replays a step: the facade and the keyboard shortcut bound to the engine itself.

// One entry holding one step that another editor stands behind (the block editor's own
// Yjs stack item). It goes through the engine's own command, whose flush closes whatever
// the page was typing, so the order of the two histories is the order of the edits. The
// editor's step is not a document write of its own, so recording it never marks the page
// dirty; the editor's own dirty feed is the one that does.
undo.recordStep = (label, primitive) => command(label, () => undo.record(primitive))

// A step that stands for an editor's own undo item holds tokens, not text: its prop names
// it, and what a person wrote lives in the editor, not in a recovery copy.
const isBlockStep = (entry) => !!entry?.steps?.length && entry.steps.every((step) => step.kind === 'value' && /^blockeditor-step(?::|$)/.test(step.prop))

// A commit inside clay.effect drains the paused hub's records, so the effect's
// own writes reach autosave and re-trigger it. Warn once; the commit still runs.
const commit = undo.commit.bind(undo)
let warnedEffectCommit = false
undo.commit = (label, fn) => {
  if (!warnedEffectCommit && window.clay?.effect?.running) {
    warnedEffectCommit = true
    console.warn(`clay.effect: undo.commit("${label}") was called inside an effect. Run commands from event handlers, not from an effect.`)
  }
  const before = lastEntryId()
  const receipt = commit(label, fn)
  if (receipt || lastEntryId() !== before) gateMarkDirty()
  return receipt
}

// Undo and redo restore attributes. A control the user has touched shows its
// live property instead (the WHATWG dirty flag), so copy the attributes back, as
// a sync merge does. Undo is the user's own request, so the focused field is
// restored too, with its selection clamped to the new length. Controls persist
// never writes (password, hidden, file) and regions undo does not record are
// left alone.
const CONTROLS = `input${PERSIST}:not([type="password"]):not([type="hidden"]):not([type="file"]), textarea${PERSIST}, select${PERSIST}`

function restoreControls() {
  const undoable = (el) => window.clay?.region?.resolveRegionPolicy?.(el)?.undoable !== false
  document.querySelectorAll(CONTROLS).forEach((el) => {
    if (!undoable(el)) return
    if (el.tagName === 'SELECT') {
      for (const opt of el.options) {
        const want = opt.hasAttribute('selected')
        if (opt.selected !== want) opt.selected = want
      }
      return
    }
    const type = (el.getAttribute('type') || 'text').toLowerCase()
    if (el.tagName === 'INPUT' && (type === 'checkbox' || type === 'radio')) {
      const want = el.hasAttribute('checked')
      if (el.checked !== want) el.checked = want
      return
    }
    const want = el.tagName === 'TEXTAREA'
      ? (el.getAttribute('data-value') ?? el.textContent)
      : (el.getAttribute('value') ?? '')
    if (el.value === want) return
    const focused = el === document.activeElement
    let start, end
    try { start = el.selectionStart; end = el.selectionEnd } catch {}
    el.value = want
    if (focused && start != null) {
      try { el.setSelectionRange(Math.min(start, want.length), Math.min(end, want.length)) } catch {}
    }
  })
}

// The engine emits its receipt only after a whole replay applied, so a refused
// undo or redo never reaches the control restore.
const afterReplay = (receipt) => {
  if (!receipt?.ok) return
  if (!isBlockStep(undo.describe(receipt.entryId))) gateMarkDirty()
  restoreControls()
  if (receipt.caret && undo.pageHistory) placeCaret(receipt.caret)
}

// A refusal is announced for whoever presents it: the engine's receipt, once.
const refuse = ({ entryId, label, direction, code }) => {
  document.dispatchEvent(new CustomEvent('clay:undo-refused', {
    detail: { ok: false, code, entryId, label, direction },
  }))
  keepRefused({ ok: false, code, entryId, label, direction })
}

// A step the engine can no longer replay is skipped instead of blocking the press: the
// person asked once and gets the next older step. Its recovery copy is made from the
// description the engine hands over, because the entry is already out of the history.
const skip = ({ entryId, label, direction, code, description }) => {
  document.dispatchEvent(new CustomEvent('clay:undo-skipped', {
    detail: { entryId, label, direction, code },
  }))
  if (isBlockStep(description)) return
  keepRefused({ ok: false, code, entryId, label, direction, description })
}

// A refused step is kept as recovery before a reload can take it away: the refusal is
// announced synchronously above and the copy is made behind it, so a store that will
// not answer can never hold up the receipt. The module is fetched on the first refusal
// or the first app keep; no reader and no boot pays for it, and nothing it writes is a
// save.
let recoveryLoad = null

function loadRecovery() {
  recoveryLoad ??= import('../recovery/undo.js').catch((error) => {
    recoveryLoad = null
    console.error('clayjs: the undo recovery module could not be downloaded; a refused step stays in this tab only', error)
    return null
  })
  return recoveryLoad
}

async function keepRefused(receipt) {
  let written = { ok: false, code: 'unavailable' }
  try {
    const module = await loadRecovery()
    if (module) written = await module.keepRefusal(undo, receipt)
  } catch (_) {
    written = { ok: false, code: 'error' }
  }
  const detail = written?.ok === true
    ? { ok: true, entryId: receipt.entryId, direction: receipt.direction }
    : { ok: false, code: written?.code || 'error', entryId: receipt.entryId, direction: receipt.direction }
  document.dispatchEvent(new CustomEvent('clay:undo-kept', { detail }))
}

// The page's own records on the same channel and in the same store: `keep` stages one,
// `kept` is what a later session can review, and `dismissKept` is the explicit,
// revision-checked way one leaves. All three fetch the recovery module on demand.
const withRecovery = (call, fallback) => (...args) =>
  loadRecovery().then(
    (module) => (module ? module[call](...args) : fallback),
    () => fallback
  )

// An app's kept copy is announced like a refused step's, so the review notice shows it
// in this session instead of only after a reload.
const keepApp = withRecovery('keep', { ok: false, code: 'unavailable' })
undo.addDurableForm = (form) => {
  durableForms.add(form)
  return () => { durableForms.delete(form) }
}

undo.ignoreAttribute = (name) => {
  const key = String(name || '')
  if (!key) return () => {}
  pageIgnored.add(key)
  return () => { pageIgnored.delete(key) }
}

undo.keep = async (spec) => {
  const written = await keepApp(spec)
  if (written?.ok === true) {
    document.dispatchEvent(new CustomEvent('clay:undo-kept', { detail: { ok: true, key: spec?.key == null ? null : `app:${spec.key}` } }))
  }
  return written
}
undo.kept = withRecovery('kept', { ok: false, code: 'unavailable', records: [] })
undo.dismissKept = withRecovery('dismissKept', { ok: false, code: 'unavailable' })

// The page's own actions on a kept copy. A provider is asked about every record the
// review panel draws, and each action it returns becomes a button on that row, so a
// template offers Keep as new without the panel knowing what a card is. A provider
// that throws costs its own buttons only, never the row or the copy.
const reviewProviders = new Set()

undo.reviewActions = (fn) => {
  if (typeof fn !== 'function') return () => {}
  reviewProviders.add(fn)
  return () => { reviewProviders.delete(fn) }
}

undo.reviewActionsFor = (record) => {
  const actions = []
  for (const fn of reviewProviders) {
    let offered
    try {
      offered = fn(record)
    } catch (error) {
      console.error('clayjs: a review action could not describe itself; the copy stays reviewable', error)
      continue
    }
    for (const item of Array.isArray(offered) ? offered : []) {
      if (item && typeof item.run === 'function' && item.label) actions.push(item)
    }
  }
  return actions
}

// One event for whatever presents the history (Undo and Redo buttons, a refusal banner):
// after every entry, replay, refusal, dismissal and clear, and when a session starts or
// ends. `undo` and `redo` are the entries the next press would replay, with `refused`
// set on the one that was just refused.
const HISTORY_EVENTS = ['commit', 'undo', 'redo', 'refused', 'skipped', 'discard', 'clear']

// Whatever else owns the moment (a proposal preview, a dialog) holds the page history: while
// any hold is live the engine refuses a replay, so nothing behind the hold can be undone by a
// stray Cmd+Z, and the event reports neither direction as available.
const holds = new Set()

// The engine's own answer for each direction: a getter on the engine object itself, so it is
// read once here and called through, whatever else happens to the property.
const engineDirection = (name) => {
  for (let owner = undo; owner; owner = Object.getPrototypeOf(owner)) {
    const desc = Object.getOwnPropertyDescriptor(owner, name)
    if (!desc) continue
    if (typeof desc.get === 'function') return () => desc.get.call(undo)
    return () => Boolean(desc.value.call(undo))
  }
  return () => false
}

const engineCanUndo = engineDirection('canUndo')
const engineCanRedo = engineDirection('canRedo')

// While a hold is live neither direction is available, so a dock button, the editor's own
// history and the event all answer the same thing.
Object.defineProperty(undo, 'canUndo', { get: () => holds.size === 0 && engineCanUndo(), configurable: true })
Object.defineProperty(undo, 'canRedo', { get: () => holds.size === 0 && engineCanRedo(), configurable: true })

undo.hold = () => {
  const token = Symbol('hold')
  holds.add(token)
  announceChange()
  let released = false
  return () => {
    if (released) return
    released = true
    holds.delete(token)
    announceChange()
  }
}

const announceChange = () => {
  const live = !!session?.started && holds.size === 0
  document.dispatchEvent(new CustomEvent('clay:undo-change', {
    detail: {
      canUndo: live && undo.canUndo,
      canRedo: live && undo.canRedo,
      undo: (live && undo.history.at(-1)) || null,
      redo: (live && undo.future?.[0]) || null,
    },
  }))
}

// One edit session owns the page's DOM history. A session starts its own over the page as
// it stands, so nothing an earlier session recorded, and nothing a reader painted while the
// page was in view, is this session's to undo. The durable recovery records belong to the
// recovery service and outlive every session; nothing here reads, clears or rewrites them.
let session = null

// There is one history in Edit and it is the page's: a live session is what puts a rich
// field's typing on it and gives Cmd+Z in any editable field to it. No attribute decides
// this, and a page that never loads this plugin never starts a session, so nothing about
// it changes.
Object.defineProperty(undo, 'pageHistory', {
  get: () => !!session?.started,
  configurable: true,
})

const announceSession = () => {
  if (typeof document === 'undefined') return
  document.dispatchEvent(new CustomEvent('clay:undo-session', {
    detail: { started: !!session?.started, pageHistory: undo.pageHistory },
  }))
}

function ownership() {
  return { started: false, admitted: false, subscriptions: [] }
}

const isStale = (context) => {
  if (!context) return false
  if (context.signal?.aborted) return true
  return typeof context.isCurrent === 'function' && !context.isCurrent()
}

// The default scope is document.body, so a boot that is still parsing waits for the body
// to exist before the session starts.
function bodyReady() {
  if (typeof document === 'undefined') return Promise.resolve()
  if (document.readyState !== 'loading') return Promise.resolve()
  return new Promise((resolve) => document.addEventListener('DOMContentLoaded', resolve, { once: true }))
}

// One edit session over the live page. `enable` starts a fresh history and the control
// restore that follows it; `quiesce` closes recording so the exit barrier's writes are not
// entries, keeping the history itself; `resume` is the refused exit; `dispose` detaches
// exactly the subscriptions this session added and takes the history and its keydown
// binding back, so a later entry starts from nothing.
function undoLease(record) {
  return {
    enable() {
      if (record.started) return
      record.started = true
      record.admitted = true
      undo.start(withTabLocal({
        bindKeys: true,
        editableKeys: 'page',
        ownKeysIn: ['.blockeditor-editor'],
        typingBreak: 'sentence',
        refused: 'skip',
        holdReplay: () => holds.size > 0,
        ...(recording ? { recording } : {}),
      }))
      for (const type of ['undo', 'redo']) {
        undo.on(type, afterReplay)
        record.subscriptions.push({ type, handler: afterReplay })
      }
      undo.on('refused', refuse)
      record.subscriptions.push({ type: 'refused', handler: refuse })
      undo.on('skipped', skip)
      record.subscriptions.push({ type: 'skipped', handler: skip })
      for (const type of HISTORY_EVENTS) {
        undo.on(type, announceChange)
        record.subscriptions.push({ type, handler: announceChange })
      }
      announceChange()
      announceSession()
    },
    quiesce() {
      if (!record.admitted) return
      record.admitted = false
      undo.pause()
    },
    resume() {
      if (record.admitted) return
      record.admitted = true
      undo.resume()
    },
    dispose() {
      record.admitted = false
      if (session === record) session = null
      if (!record.started) return
      record.started = false
      for (const { type, handler } of record.subscriptions.splice(0)) undo.off(type, handler)
      undo.stop()
      announceChange()
      announceSession()
    },
  }
}

/**
 * Undo as a mode participant: the history the commit that enters edit starts, and the one
 * the exit takes back.
 *
 * `prepare` waits for the body the default scope needs, and answers with the session's
 * lease; that wait can outlive the session that asked for it, so the generation is asked
 * again once the body exists. `enable` starts the history. `quiesce` stops recording
 * without losing what the session already holds. `dispose` hands back the subscriptions,
 * the history and the keydown binding.
 */
export function undoParticipant() {
  return {
    id: 'clay:undo',
    order: 350,
    async prepare(context = {}) {
      if (context.initial === true) return adopt()
      await bodyReady()
      if (isStale(context)) {
        throw Object.assign(new Error('clay:undo prepared for a stale generation'), { code: 'stale-document' })
      }
      return undoLease(session || (session = ownership()))
    },
  }
}

// The boot this tab already made, adopted instead of prepared: it is the history `init`
// below started, or, on a boot that started none, nothing to hand back.
async function adopt() {
  const lease = await ready
  return { enable() {}, quiesce: () => lease?.quiesce(), resume: () => lease?.resume(), dispose: () => lease?.dispose() }
}

// Auto-init when the module is imported; the loader awaits `ready` before resolving clay.ready
async function init() {
  if (!startsAtImport()) return null
  if (typeof document === 'undefined') return null
  const lease = await undoParticipant().prepare()
  lease.enable()
  return lease
}

const ready = init()

export { undo, restoreControls, init, ready }
export { undoParticipant as modeParticipant }
export default undo
