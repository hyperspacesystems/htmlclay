'use strict'

// Golden answers for the Go port of the managed trial host's data-loss guard:
// Local's own src/main/data-loss-guard.js run over a temp root, recording, per
// scenario, every call's return value, the live notifications, and the final
// `.hyperclay/guard/` and `.hyperclay/versions/` trees.
//
//   node gen.cjs <hyperclay-local checkout> <out.json>
//
// Date.now and crypto.randomUUID are stubbed BEFORE the module is required, both
// counter based, so ids and stamps are reproducible byte for byte. The root path
// is replaced by `<root>` in the recorded file bytes: a guard record stores the
// ABSOLUTE recover path, and no other byte of it is machine-dependent.

const fs = require('fs')
const os = require('os')
const path = require('path')
const crypto = require('crypto')

const checkout = process.argv[2]
const outPath = process.argv[3]
if (!checkout || !outPath) {
  process.stderr.write('usage: node gen.cjs <hyperclay-local checkout> <out.json>\n')
  process.exit(1)
}

const REAL_DATE = Date
const BASE_NOW = REAL_DATE.UTC(2026, 10, 1, 6, 0, 0, 0)
let nowCounter = 0
REAL_DATE.now = () => BASE_NOW + nowCounter++
Date.now = REAL_DATE.now

let uuidCounter = 0
crypto.randomUUID = () => '00000000-0000-4000-8000-' + String(uuidCounter++).padStart(12, '0')
const stubs = { dateNow: 'counter', randomUUID: 'counter' }

const guard = require(path.join(checkout, 'src', 'main', 'data-loss-guard.js'))
const core = require(path.join(checkout, 'src', 'main', 'data-loss-core.cjs'))

const sha256 = (value) => crypto.createHash('sha256').update(value).digest('hex')
const tmp = fs.realpathSync(fs.mkdtempSync(path.join(os.tmpdir(), 'guard-golden-')))
let activeRoot = tmp

function normalize(value) {
  if (value === null || value === undefined) return null
  if (Buffer.isBuffer(value)) return { sha256: sha256(value) }
  if (typeof value === 'string') {
    if (activeRoot && value === activeRoot) return { path: '.' }
    if (activeRoot && value.startsWith(activeRoot + path.sep)) return { path: value.slice(activeRoot.length + 1) }
    if (value.length > 40) return { sha256: sha256(value), length: value.length }
    return value
  }
  if (typeof value === 'number' || typeof value === 'boolean') return value
  if (typeof value === 'function') return '<fn>'
  if (Array.isArray(value)) return value.map((item) => normalize(item))
  if (typeof value === 'object') {
    if (value === live) return '<live>'
    const out = {}
    for (const key of Object.keys(value).sort()) {
      if (value[key] === undefined) continue
      out[key] = normalize(value[key])
    }
    return out
  }
  return String(value)
}

let calls = []
let live = null

const RULES_TAG = JSON.stringify({
  title: 'h1',
  subtitle: 'p.sub',
  items: 'ul.items li[]',
  notes: 'p.notes',
  people: ['ul.people li', { name: 'b' }],
  meta: { author: 'span.author', tag: 'span.tag' }
})

function page(sections) {
  return [
    '<!doctype html>',
    '<html lang="en">',
    '  <head>',
    '    <meta charset="utf-8">',
    '    <title>Doc</title>',
    '    <script type="application/json" data-rules-name="api" data-rules-version="1">' + RULES_TAG + '</script>',
    '  </head>',
    '  <body>',
    ...sections,
    '  </body>',
    '</html>',
    ''
  ].join('\n')
}

const LONG_NOTE = 'A note long enough that the recovery preview has to cut it: it keeps going past the eighty character preview limit and then some more.'

const RICH = page([
  '    <h1>Reading list</h1>',
  '    <p class="sub">Kept by hand</p>',
  '    <ul class="items">',
  '      <li>alpha</li>',
  '      <li>beta</li>',
  '      <li>gamma</li>',
  '    </ul>',
  '    <p class="notes">' + LONG_NOTE + '</p>',
  '    <ul class="people">',
  '      <li><b>Ada</b></li>',
  '      <li><b>Grace</b></li>',
  '    </ul>',
  '    <p class="meta"><span class="author">Ada</span> <span class="tag">draft</span></p>'
])

const EDITED = RICH.replace('<p class="sub">Kept by hand</p>', '<p class="sub">Kept by hand, mostly</p>')

const CLOBBERED = page([
  '    <h1>Reading list</h1>',
  '    <p class="sub"></p>',
  '    <ul class="items">',
  '      <li>alpha</li>',
  '    </ul>',
  '    <p class="notes"></p>',
  '    <ul class="people">',
  '      <li><b>Ada</b></li>',
  '    </ul>',
  '    <p class="meta"><span class="author">Someone</span> <span class="tag"></span></p>'
])

const EMPTIED = page([
  '    <h1>Reading list</h1>',
  '    <p class="sub"></p>',
  '    <ul class="items"></ul>',
  '    <p class="notes"></p>',
  '    <ul class="people"></ul>',
  '    <p class="meta"><span class="author"></span> <span class="tag"></span></p>'
])

const UNTAGGED = '<!doctype html>\n<html>\n  <body>\n    <h1>Plain</h1>\n  </body>\n</html>\n'

const VERSION_PATH = '.hyperclay/versions/doc/2026-09-01-00-00-00-000+0000.html'
const SUBFOLDER_VERSION_PATH = '.hyperclay/versions/notes/doc/2026-09-01-00-00-00-000+0000.html'

const scenarios = [
  {
    name: 'no-rules-tag',
    seed: [['plain.html', UNTAGGED]],
    steps: [
      { fn: 'runDataLossGuard', name: 'plain.html', newHtml: UNTAGGED, prevContent: UNTAGGED, prov: 'external' }
    ]
  },
  {
    name: 'first-run-seeds',
    seed: [['doc.html', RICH]],
    steps: [
      { fn: 'runDataLossGuard', name: 'doc.html', newHtml: RICH, prevContent: RICH, prov: 'external' }
    ]
  },
  {
    name: 'non-destructive-save',
    seed: [['doc.html', RICH]],
    steps: [
      { fn: 'runDataLossGuard', name: 'doc.html', newHtml: RICH, prevContent: RICH, prov: 'external' },
      { fn: 'runDataLossGuard', name: 'doc.html', newHtml: EDITED, prevContent: RICH, prov: 'ui-gestured' }
    ]
  },
  {
    name: 'destructive-browser-save',
    seed: [['doc.html', CLOBBERED]],
    steps: [
      { fn: 'runDataLossGuard', name: 'doc.html', newHtml: CLOBBERED, prevContent: RICH, prov: 'ui-background' }
    ]
  },
  {
    name: 'guard-event',
    seed: [['doc.html', CLOBBERED]],
    steps: [
      { fn: 'runDataLossGuard', name: 'doc.html', newHtml: CLOBBERED, prevContent: RICH, prov: 'ui-background' },
      { fn: 'getGuardEvent', name: 'doc.html', currentHtml: CLOBBERED }
    ]
  },
  {
    name: 'resolve-restore',
    seed: [['doc.html', CLOBBERED]],
    steps: [
      { fn: 'runDataLossGuard', name: 'doc.html', newHtml: CLOBBERED, prevContent: RICH, prov: 'ui-background' },
      { fn: 'resolveGuard', name: 'doc.html', choice: 'restore', id: 'current', currentHtml: CLOBBERED }
    ]
  },
  {
    name: 'resolve-dismiss',
    seed: [['doc.html', CLOBBERED]],
    steps: [
      { fn: 'runDataLossGuard', name: 'doc.html', newHtml: CLOBBERED, prevContent: RICH, prov: 'ui-background' },
      { fn: 'resolveGuard', name: 'doc.html', choice: 'dismiss', id: 'current', currentHtml: CLOBBERED }
    ]
  },
  {
    name: 'outside-edit',
    seed: [['doc.html', CLOBBERED], [VERSION_PATH, RICH]],
    steps: [
      { fn: 'runDataLossGuard', name: 'doc.html', newHtml: RICH, prevContent: RICH, prov: 'external' },
      { fn: 'runDataLossGuard', name: 'doc.html', newHtml: CLOBBERED, prevContent: null, prov: 'external' }
    ]
  },
  {
    name: 'catch-up-outside-edit',
    seed: [['doc.html', CLOBBERED], [VERSION_PATH, RICH]],
    steps: [
      { fn: 'runDataLossGuard', name: 'doc.html', newHtml: RICH, prevContent: RICH, prov: 'external' },
      { fn: 'noteSeen', name: 'doc.html', bytes: RICH },
      { fn: 'catchUpOutsideEdit', name: 'doc.html', bytes: CLOBBERED, html: CLOBBERED },
      { fn: 'getGuardEvent', name: 'doc.html', currentHtml: CLOBBERED }
    ]
  },
  {
    name: 'mark-pending-then-seen',
    seed: [['doc.html', CLOBBERED]],
    steps: [
      { fn: 'markPending', name: 'doc.html', bytes: CLOBBERED },
      { fn: 'noteSeen', name: 'doc.html', bytes: CLOBBERED },
      { fn: 'runDataLossGuard', name: 'doc.html', newHtml: CLOBBERED, prevContent: RICH, prov: 'external' }
    ]
  },
  {
    name: 'apply-remote-resolution',
    seed: [['doc.html', CLOBBERED]],
    steps: [
      { fn: 'runDataLossGuard', name: 'doc.html', newHtml: CLOBBERED, prevContent: RICH, prov: 'ui-background' },
      { fn: 'applyRemoteResolution', name: 'doc.html', hash: '0000000000000000000000000000000000000000000000000000000000000000' },
      { fn: 'applyRemoteResolution', name: 'doc.html', hash: 'current' }
    ]
  },
  {
    name: 'second-destructive-save',
    seed: [['doc.html', CLOBBERED]],
    steps: [
      { fn: 'runDataLossGuard', name: 'doc.html', newHtml: CLOBBERED, prevContent: RICH, prov: 'ui-background' },
      { fn: 'runDataLossGuard', name: 'doc.html', newHtml: EMPTIED, prevContent: CLOBBERED, prov: 'ui-background' }
    ]
  },
  {
    name: 'preview-limits',
    seed: [['doc.html', EMPTIED]],
    steps: [
      { fn: 'runDataLossGuard', name: 'doc.html', newHtml: EMPTIED, prevContent: RICH, prov: 'ui-background' },
      { fn: 'getGuardEvent', name: 'doc.html', currentHtml: EMPTIED }
    ]
  },
  {
    name: 'subfolder',
    seed: [['notes/doc.html', CLOBBERED], [SUBFOLDER_VERSION_PATH, RICH]],
    steps: [
      { fn: 'runDataLossGuard', name: 'notes/doc.html', newHtml: CLOBBERED, prevContent: RICH, prov: 'ui-background' }
    ]
  }
]

function writeFile(root, rel, content) {
  const target = path.join(root, rel)
  fs.mkdirSync(path.dirname(target), { recursive: true })
  fs.writeFileSync(target, content)
}

function tree(root) {
  const files = []
  const walk = (dir, prefix) => {
    for (const entry of fs.readdirSync(dir, { withFileTypes: true }).sort((a, b) => (a.name < b.name ? -1 : 1))) {
      const rel = prefix ? prefix + '/' + entry.name : entry.name
      const full = path.join(dir, entry.name)
      if (entry.isDirectory()) walk(full, rel)
      else if (entry.isFile()) {
        const text = fs.readFileSync(full, 'utf8').split(root).join('<root>')
        files.push({ path: rel, b64: Buffer.from(text, 'utf8').toString('base64') })
      } else files.push({ path: rel, other: true })
    }
  }
  for (const top of ['.hyperclay/guard', '.hyperclay/versions']) {
    if (fs.existsSync(path.join(root, top))) walk(path.join(root, top), top)
  }
  return files
}

async function openEventId(root, name) {
  const record = await guard._readGuard(root, name)
  return record && record.event ? record.event.id : null
}

async function openEventHash(root, name) {
  const record = await guard._readGuard(root, name)
  return record && record.event ? core.islandHash(record.event.recoverableData) : null
}

async function runStep(root, step) {
  const name = step.name
  let args = null
  let returns = null

  if (step.fn === 'runDataLossGuard') {
    args = { name, newHtml: step.newHtml, prevContent: step.prevContent === undefined ? null : step.prevContent, prov: step.prov }
    returns = await guard.runDataLossGuard({
      baseDir: root,
      name,
      newHtml: step.newHtml,
      prevContent: step.prevContent === undefined ? null : step.prevContent,
      prov: step.prov,
      live
    })
    calls.push({ fn: step.fn, args: [normalize(args)], returns: normalize(returns) })
    return { fn: step.fn, args }
  }

  if (step.fn === 'getGuardEvent') {
    args = { name, currentHtml: step.currentHtml }
    returns = await guard.getGuardEvent(root, name, step.currentHtml, live)
    calls.push({ fn: step.fn, args: [normalize(args)], returns: normalize(returns) })
    return { fn: step.fn, args }
  }

  if (step.fn === 'resolveGuard') {
    const id = step.id === 'current' ? await openEventId(root, name) : step.id
    args = { name, id, choice: step.choice, currentHtml: step.currentHtml }
    returns = await guard.resolveGuard({
      baseDir: root,
      name,
      id,
      choice: step.choice,
      currentHtml: step.currentHtml,
      writeBack: async (html) => {
        const island = await guard.extractIsland(html)
        calls.push({ fn: 'writeBack', args: [normalize({ data: island ? island.api : null })], returns: null })
      },
      live
    })
    calls.push({ fn: step.fn, args: [normalize(args)], returns: normalize(returns) })
    return { fn: step.fn, args }
  }

  if (step.fn === 'markPending') {
    args = { name, bytes: step.bytes }
    guard.markPending(root, name, step.bytes)
    calls.push({ fn: step.fn, args: [normalize(args)], returns: null })
    return { fn: step.fn, args }
  }

  if (step.fn === 'noteSeen') {
    args = { name, bytes: step.bytes }
    await guard.noteSeen(root, name, step.bytes)
    calls.push({ fn: step.fn, args: [normalize(args)], returns: null })
    return { fn: step.fn, args }
  }

  if (step.fn === 'catchUpOutsideEdit') {
    args = { name, bytes: step.bytes, html: step.html }
    await guard.catchUpOutsideEdit(root, name, step.bytes, step.html, live)
    calls.push({ fn: step.fn, args: [normalize(args)], returns: null })
    return { fn: step.fn, args }
  }

  if (step.fn === 'applyRemoteResolution') {
    const hash = step.hash === 'current' ? await openEventHash(root, name) : step.hash
    args = { name, hash }
    returns = await guard.applyRemoteResolution({ baseDir: root, name, recoverableDataHash: hash })
    calls.push({ fn: step.fn, args: [normalize(args)], returns: normalize(returns) })
    return { fn: step.fn, args }
  }

  throw new Error('unknown step ' + step.fn)
}

async function runScenario(scenario) {
  const root = fs.realpathSync(fs.mkdtempSync(path.join(tmp, 'root-')))
  activeRoot = root
  for (const [rel, content] of scenario.seed) writeFile(root, rel, content)

  nowCounter = 0
  uuidCounter = 0
  calls = []
  live = {
    notify(name, message) {
      calls.push({ fn: 'live.notify', args: [normalize(name), normalize(message)], returns: null })
    }
  }

  const steps = []
  for (const step of scenario.steps) steps.push(await runStep(root, step))

  return {
    name: scenario.name,
    seed: scenario.seed.map(([rel, content]) => ({ path: rel, b64: Buffer.from(content, 'utf8').toString('base64') })),
    steps,
    calls,
    tree: tree(root)
  }
}

async function main() {
  const answers = []
  for (const scenario of scenarios) answers.push(await runScenario(scenario))
  fs.writeFileSync(outPath, JSON.stringify({ stubs, scenarios: answers }, null, 2) + '\n')
}

main().catch((error) => {
  process.stderr.write((error && error.stack ? error.stack : String(error)) + '\n')
  process.exit(1)
})
