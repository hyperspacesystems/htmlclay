const regions = new Set()
const knownRoots = new WeakMap()
const lastRevision = new WeakMap()
const reported = new WeakMap()
const listeners = new Set()
const registerListeners = new Set()

const NONCE = `${Math.random().toString(36).slice(2)}${Date.now().toString(36)}`
const HOLE = `clay-owned:${NONCE}:`
const OPEN = `<!--${HOLE}`
const CLOSE = '-->'
const QUIET = `<!--clay-owned-rev:${NONCE}:`
const BODY = `<!--clay-owned-body:${NONCE}:`
const BODY_MIN = 1024
export const OWNED_FROZEN_SELECTOR = '[clay~="freeze"], [freeze], [save-freeze]'
let marked = false
let failures = 0

function fail(message, code) {
  const error = new Error(message)
  error.code = code
  return error
}

function report(region, what, error) {
  const seen = reported.get(region) || new Set()
  if (seen.has(what)) return
  seen.add(what)
  reported.set(region, seen)
  const outcome = what === 'merge'
    ? 'its live content was left alone'
    : what === 'revision'
      ? 'the region counts as unsaved'
      : 'the region is saved by the generic path'
  console.error(`clayjs: the owner of "${region.selector}" failed in ${what}; ${outcome}`, error)
}

export function own(selector, { serialize, revision, merge = null, conflict = null } = {}) {
  if (typeof selector !== 'string' || !selector.trim()) throw new TypeError('clay.region.own: selector must be a non-empty string')
  if (typeof serialize !== 'function') throw new TypeError('clay.region.own: serialize must be a function')
  if (typeof revision !== 'function') throw new TypeError('clay.region.own: revision must be a function')
  if (merge !== null && typeof merge !== 'function') throw new TypeError('clay.region.own: merge must be a function')
  if (conflict !== null && typeof conflict !== 'function') throw new TypeError('clay.region.own: conflict must be a function')
  document.createDocumentFragment().querySelector(selector)
  const region = { selector, serialize, revision, merge, conflict }
  regions.add(region)
  for (const root of document.querySelectorAll(selector)) know(root, region, true)
  for (const listener of registerListeners) listener()
  return {
    dispose() { regions.delete(region) },
    notify(root = null) { if (regions.has(region)) ownedChanged(region, root) },
  }
}

export function hasOwnedRegions() {
  return regions.size > 0
}

function eligible(root) {
  return root.localName !== 'body' && !!root.parentElement?.closest('body') && !root.closest(OWNED_FROZEN_SELECTOR)
}

// A root's revision is recorded as saved only where its content is known to be saved:
// the roots present when the owner registers, and every root after a merge
// (syncOwnedRevisions). A root first seen anywhere else holds new content, so its first
// check counts as a move.
function know(root, region, seed = false) {
  if (knownRoots.get(root) === region || !eligible(root)) return
  knownRoots.set(root, region)
  if (seed) {
    try { lastRevision.set(root, revisionText(region, root)) } catch (_) {}
  }
}

function regionFor(root) {
  const region = knownRoots.get(root)
  return region && regions.has(region) ? region : null
}

export function isOwnedRoot(node) {
  return regions.size > 0 && knownRoots.has(node) && !!regionFor(node)
}

export function matchOwnedRoot(element) {
  if (!regions.size || element.nodeType !== 1) return null
  const known = regionFor(element)
  if (known) return known
  for (const region of regions) {
    if (element.matches(region.selector) && eligible(element)) {
      know(element, region)
      return region
    }
  }
  return null
}

export function ownedRootOf(node) {
  if (!regions.size) return null
  for (let at = node; at; at = at.parentNode) {
    if (isOwnedRoot(at)) return at
  }
  return null
}

function liveRoots() {
  const found = new Map()
  for (const region of regions) {
    for (const root of document.querySelectorAll(region.selector)) {
      if (found.has(root)) continue
      if (!eligible(root)) { knownRoots.delete(root); continue }
      found.set(root, region)
      know(root, region)
    }
  }
  for (const root of [...found.keys()]) {
    for (let at = root.parentElement; at; at = at.parentElement) {
      if (found.has(at)) { found.delete(root); break }
    }
  }
  return found
}

function revisionText(region, root) {
  const value = region.revision(root)
  if (typeof value === 'string') return value
  if (typeof value === 'number' && Number.isFinite(value)) return String(value)
  throw new TypeError('revision must return a string or a finite number')
}

function revisionOrUnsaved(region, root) {
  try {
    return revisionText(region, root)
  } catch (error) {
    report(region, 'revision', error)
    return `!${++failures}`
  }
}

function holeData(kind, region, revision, bytes) {
  return `${HOLE}${kind}${region.conflict ? 'q' : 'n'}:${revision.length}:${bytes.length}:${revision}${bytes}`
}

export function pinOwnedRegions(mode = 'bytes', purpose = 'save', refuse = false) {
  if (!regions.size) return null
  const pins = new Map()
  for (const [root, region] of liveRoots()) {
    const revision = revisionOrUnsaved(region, root)
    if (mode !== 'bytes') {
      pins.set(root, holeData('r', region, revision, ''))
      continue
    }
    try {
      const bytes = region.serialize(root, { purpose })
      if (typeof bytes !== 'string') throw new TypeError('serialize must return a string')
      pins.set(root, holeData('b', region, revision, bytes))
    } catch (error) {
      if (typeof error?.code !== 'string') report(region, 'serialize', error)
      else if (refuse) throw error
    }
  }
  if (!pins.size) return null
  marked = true
  return pins
}

function damaged() {
  return fail('An owned region marker is damaged; nothing was written', 'owned-region-damaged')
}

function readHole(text, at) {
  const kind = text[at]
  const quiet = text[at + 1]
  const a = text.indexOf(':', at + 3)
  const b = a === -1 ? -1 : text.indexOf(':', a + 1)
  const revisionLength = Number(text.slice(at + 3, a))
  const bytesLength = Number(text.slice(a + 1, b))
  if ((kind !== 'b' && kind !== 'r') || (quiet !== 'q' && quiet !== 'n') || text[at + 2] !== ':' || b === -1 || !Number.isSafeInteger(revisionLength) || !Number.isSafeInteger(bytesLength)) throw damaged()
  const revisionStart = b + 1
  const bytesStart = revisionStart + revisionLength
  const end = bytesStart + bytesLength
  if (end > text.length) throw damaged()
  return { kind, quiet: quiet === 'q', revision: text.slice(revisionStart, bytesStart), bytesStart, end }
}

function resolve(html, write) {
  if (!marked || typeof html !== 'string') return html
  let at = html.indexOf(OPEN)
  if (at === -1) return html
  const out = []
  let from = 0
  while (at !== -1) {
    const hole = readHole(html, at + OPEN.length)
    if (!html.startsWith(CLOSE, hole.end)) throw damaged()
    out.push(html.slice(from, at), write(hole, html))
    from = hole.end + CLOSE.length
    at = html.indexOf(OPEN, from)
  }
  out.push(html.slice(from))
  return out.join('')
}

export function ownedBytes(html) {
  return resolve(html, (hole, text) => {
    if (hole.kind !== 'b') throw fail('This capture holds no bytes for its owned regions', 'owned-region-unpinned')
    return text.slice(hole.bytesStart, hole.end)
  })
}

export function ownedRevisions(html) {
  return resolve(html, (hole) => hole.quiet
    ? `${QUIET}${hole.revision.length}:${hole.revision}${CLOSE}`
    : `<!--clay-owned-rev:${hole.revision}${CLOSE}`)
}

export function withoutQuietRevisions(text) {
  if (!marked || typeof text !== 'string') return text
  let at = text.indexOf(QUIET)
  if (at === -1) return text
  const out = []
  let from = 0
  while (at !== -1) {
    const colon = text.indexOf(':', at + QUIET.length)
    const length = colon === -1 ? NaN : Number(text.slice(at + QUIET.length, colon))
    const end = colon + 1 + length
    if (!Number.isSafeInteger(length) || !text.startsWith(CLOSE, end)) return text
    out.push(text.slice(from, at), QUIET, CLOSE)
    from = end + CLOSE.length
    at = text.indexOf(QUIET, from)
  }
  out.push(text.slice(from))
  return out.join('')
}

function holesIn(root) {
  const found = []
  if (!marked || !root) return found
  const walker = root.ownerDocument.createTreeWalker(root, 128)
  for (let node = walker.nextNode(); node; node = walker.nextNode()) {
    if (node.data.startsWith(HOLE)) found.push(node)
  }
  return found
}

export function holesToRevisions(root) {
  for (const node of holesIn(root)) {
    if (node.data[HOLE.length] !== 'b') continue
    const hole = readHole(node.data, HOLE.length)
    node.data = `${HOLE}r${hole.quiet ? 'q' : 'n'}:${hole.revision.length}:0:${hole.revision}`
  }
}

export function ownedBodies(root) {
  const bodies = new Set()
  for (const node of holesIn(root)) {
    const hole = readHole(node.data, HOLE.length)
    if (hole.kind === 'b' && hole.end - hole.bytesStart >= BODY_MIN) bodies.add(node.data.slice(hole.bytesStart, hole.end))
  }
  return [...bodies]
}

export function withoutOwnedBodies(html, bodies) {
  let out = html
  for (let i = 0; i < bodies.length; i++) {
    const parts = out.split(bodies[i])
    if (parts.length < 2) return null
    out = parts.join(`${BODY}${i}${CLOSE}`)
  }
  return out
}

export function onOwnedChange(listener) {
  listeners.add(listener)
  return () => { listeners.delete(listener) }
}

/** Called after every clay.region.own registration. */
export function onOwnedRegister(listener) {
  registerListeners.add(listener)
  return () => { registerListeners.delete(listener) }
}

// consume = false only looks: a paused drain must leave a move for the subscribers that
// cannot hear it yet, which Mutation announces when the last pause ends.
export function ownedRevisionMoved(root, consume = true) {
  const region = regionFor(root)
  if (!region || !root.isConnected) return false
  let revision
  try {
    revision = revisionText(region, root)
  } catch (error) {
    report(region, 'revision', error)
    return true
  }
  if (lastRevision.get(root) === revision) return false
  if (consume) lastRevision.set(root, revision)
  return true
}

/** Every live owned root whose revision moved since it was last announced, now announced. */
export function ownedRootsMoved() {
  const moved = []
  for (const root of liveRoots().keys()) if (ownedRevisionMoved(root)) moved.push(root)
  return moved
}

/** A merge brought these revisions: record them as nobody's edit, and return them. */
export function syncOwnedRevisions() {
  const at = new Map()
  for (const [root, region] of liveRoots()) {
    let revision = null
    try { revision = revisionText(region, root) } catch (_) {}
    if (revision === null) lastRevision.delete(root)
    else lastRevision.set(root, revision)
    at.set(root, revision)
  }
  return at
}

/** True when any root recorded by syncOwnedRevisions now has another revision. */
export function ownedMovedSince(at) {
  for (const [root, revision] of at) {
    const region = regionFor(root)
    if (!region || !root.isConnected) continue
    let now = null
    try { now = revisionText(region, root) } catch (_) {}
    if (now !== revision) return true
  }
  return false
}

function ownedChanged(region, only) {
  const roots = []
  for (const root of only ? [only] : document.querySelectorAll(region.selector)) {
    know(root, region)
    if (regionFor(root) === region) roots.push(root)
  }
  if (roots.length) for (const listener of listeners) listener(roots)
}

export function ownedRootsIn(tree) {
  const found = []
  if (!regions.size || !tree?.querySelectorAll) return found
  const seen = new Set()
  for (const region of regions) {
    for (const element of tree.querySelectorAll(region.selector)) {
      if (seen.has(element) || !eligible(element)) continue
      seen.add(element)
      found.push({ element, region })
    }
  }
  return found.filter(({ element }) => {
    for (let at = element.parentElement; at; at = at.parentElement) if (seen.has(at)) return false
    return true
  })
}

export function matchesOwnedSelector(element) {
  if (!regions.size || element.nodeType !== 1) return false
  for (const region of regions) if (element.matches(region.selector) && eligible(element)) return true
  return false
}

export function ownedConflictHandlers() {
  const handlers = new Set()
  for (const region of regions) if (region.conflict) handlers.add(region.conflict)
  return [...handlers]
}

export function beginOwnedMerge(incoming) {
  const steps = []
  let diverged = false
  let failed = false
  const active = new Set([...ownedRootsIn(document), ...ownedRootsIn(incoming.document)].map(({ region }) => region))
  for (const region of regions) {
    if (!active.has(region)) continue
    if (!region.merge) {
      report(region, 'merge', new Error('clay.region.own was given no merge function'))
      if (incoming.dirty) diverged = true
      continue
    }
    try {
      const after = region.merge(incoming)
      if (typeof after === 'function') steps.push([region, after])
    } catch (error) {
      report(region, 'merge', error)
      failed = true
    }
  }
  return () => {
    for (const [region, after] of steps) {
      try {
        if (after() === true) diverged = true
      } catch (error) {
        report(region, 'merge', error)
        failed = true
      }
    }
    return { diverged: diverged || failed, failed }
  }
}
