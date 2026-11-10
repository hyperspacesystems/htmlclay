'use strict'

// Golden answers for the Go port of the managed trial host's commit core: the
// dispatcher, the `exact`, `create` and `save` lanes, and the hooks they call.
// Runs Local's real document host (src/main/document-host.js) over a temp root
// and records, per scenario, the results, the hook calls and the exact file
// tree.
//
//   node gen.cjs <hyperclay-local checkout> <out.json>
//
// The clock is frozen (version names carry the time) and crypto.randomBytes is
// made deterministic (page-state generations and backup temp names), so the
// answer is reproducible byte for byte.

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
let fakeNowMs = REAL_DATE.UTC(2026, 10, 1, 6, 0, 0, 0)
class FakeDate extends REAL_DATE {
  constructor(...args) {
    if (args.length === 0) super(fakeNowMs)
    else super(...args)
  }

  static now() {
    return fakeNowMs
  }
}
global.Date = FakeDate

const REAL_RANDOM = crypto.randomBytes
const pageGenerations = []
let pageGenerationCounter = 0
let tempCounter = 0
crypto.randomBytes = (size) => {
  const bytes = Buffer.alloc(size)
  if (size === 16) {
    bytes.writeUInt32BE(pageGenerationCounter, 12)
    pageGenerationCounter++
    pageGenerations.push(bytes.toString('hex'))
    return bytes
  }
  tempCounter++
  bytes.writeUInt32BE(tempCounter, 0)
  return bytes
}

const sha256 = (value) => crypto.createHash('sha256').update(value).digest('hex')

const tmp = fs.realpathSync(fs.mkdtempSync(path.join(os.tmpdir(), 'commit-golden-')))
let activeRoot = tmp

const hooks = []
let recording = false
const record = (fn, args, returns) => {
  if (!recording) return
  hooks.push({ fn, args: args.map((arg) => normalize(arg)), returns: normalize(returns) })
}

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
  if (value instanceof REAL_DATE) return { date: value.getTime() }
  if (Array.isArray(value)) return value.map((item) => normalize(item))
  if (typeof value === 'object') {
    if (typeof value.broadcast === 'function' || typeof value.markBrowserSave === 'function') return '<live>'
    const out = {}
    for (const key of Object.keys(value).sort()) {
      if (value[key] === undefined) continue
      out[key] = normalize(value[key])
    }
    return out
  }
  return String(value)
}

const live = {
  markBrowserSave(name) {
    record('live.markBrowserSave', [name], null)
  },
  broadcast(name, message, options) {
    record('live.broadcast', [name, message, options], null)
  },
  notify(name, message) {
    record('live.notify', [name, message], null)
  }
}

const emit = (event, fields) => record('emit', [event, fields], null)

const dataGuard = require(path.join(checkout, 'src', 'main', 'data-loss-guard.js'))
for (const key of Object.keys(dataGuard)) {
  if (typeof dataGuard[key] !== 'function') continue
  const original = dataGuard[key]
  dataGuard[key] = function (...args) {
    const returns = original.apply(this, args)
    record('dataGuard.' + key, args, returns && typeof returns.then === 'function' ? '<promise>' : returns)
    return returns
  }
}

const tailwind = require(path.join(checkout, 'node_modules', 'tailwind-hyperclay'))
const originalCompile = tailwind.compileTailwind
tailwind.compileTailwind = function (...args) {
  const returns = originalCompile.apply(this, args)
  record('compileTailwind', args, returns && typeof returns.then === 'function' ? '<promise>' : returns)
  return returns
}

const derived = require(path.join(checkout, 'src', 'main', 'utils', 'derived-artifacts.js'))
const originalDerived = derived.refreshDerivedArtifacts
derived.refreshDerivedArtifacts = function (...args) {
  const returns = originalDerived.apply(this, args)
  record('refreshDerivedArtifacts', args, returns && typeof returns.then === 'function' ? '<promise>' : returns)
  return returns
}

const { createRootDocumentHost } = require(path.join(checkout, 'src', 'main', 'document-host.js'))
const { withDocumentWrite } = require(path.join(checkout, 'src', 'main', 'utils', 'write-queue.js'))

// commitRebase is the one lane Local only runs with a local authority: it proves
// the document boundary it arrived in and confirms the write through it. The
// boundary below is Local's own; the authority is the smallest stand-in that lets
// the lane's own steps -- the backup, the page state, the bytes -- run, which are
// the ones this golden records.
const boundaryOnlyAuthority = {
  recoverUnsavedEdits: async () => {},
  confirmKnownWrite: async () => {}
}
const YJS_TAG = "<script type=\"application/json\" merge=\"page\" clay=\"live\">{\"v\":1,\"g\":0,\"y\":{\"seed\":\"AAACQQMEAQAIQgALJwAoAQUAJwAoAIUkHGF0dHJzdGl0bGVsZXZlbHRmbG93YXR0cnNwMXRFAQEEBQIBBwEAAAEBAQACQQACQQEBBwB9AXcHaGVhZGluZ3YBAWJ3BXRpdGxldwlwYXJhZ3JhcGh2AQFidwJwMQA=\"}}</script>"
const PAGE_REV = '093360993130d818'
const ARTICLE = '<article class="page" clay="freeze" data-page="1" data-rev="' + PAGE_REV + '"><h1 data-id="title">Title</h1>\n<p data-id="p1">Body</p></article>'

const page = (lines) => ['<!doctype html>', '<html lang="en">', '<head>', '<meta charset="utf-8">', '<title>Doc</title>', '</head>', '<body>', ...lines, '</body>', '</html>', ''].join('\n')

const PLAIN = page(['<h1>One</h1>'])
const PLAIN_NEXT = page(['<h1>Two</h1>', '<p>After</p>'])
const PAGE_UNTAGGED = page([ARTICLE])
const PAGE_TAGGED = page([ARTICLE]).replace('<body>\n', '<body>\n' + YJS_TAG + '\n')
const PAGE_TAGGED_NEXT = PAGE_TAGGED.replace('<h1 data-id="title">Title</h1>', '<h1 data-id="title">Edited outside</h1>')

const PNG = Buffer.from([0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a, 0x00, 0x01, 0xff, 0xfe, 0x7f])
const LATIN1 = Buffer.from('<!doctype html>\n<html><body><p>caf\xe9</p></body></html>\n', 'latin1')

const clock = (index) => REAL_DATE.UTC(2026, 10, 1, 6, 0, index, 0)

const scenarios = [
  {
    name: 'browser-save',
    seed: [['a.html', PLAIN]],
    requests: [
      { kind: 'browser-save', name: 'a.html', text: PLAIN_NEXT, userDriven: true, saveId: 'save-1', dataLossPrev: null }
    ]
  },
  {
    name: 'browser-save-repeat',
    seed: [['a.html', PLAIN]],
    requests: [
      { kind: 'browser-save', name: 'a.html', text: PLAIN_NEXT, userDriven: true, saveId: 'save-1', dataLossPrev: null },
      { kind: 'browser-save', name: 'a.html', text: PLAIN_NEXT, userDriven: true, saveId: 'save-2', dataLossPrev: null }
    ]
  },
  {
    name: 'expected-checksum-mismatch',
    seed: [['a.html', PLAIN]],
    requests: [
      { kind: 'browser-save', name: 'a.html', text: PLAIN_NEXT, userDriven: true, dataLossPrev: null, expected: { type: 'checksum', value: '0000000000000000' } }
    ]
  },
  {
    name: 'expected-absent',
    seed: [['a.html', PLAIN]],
    requests: [
      { kind: 'browser-save', name: 'a.html', text: PLAIN_NEXT, userDriven: false, dataLossPrev: null, expected: { type: 'absent' } }
    ]
  },
  {
    name: 'page-inline-tag',
    seed: [['p.html', PAGE_UNTAGGED]],
    requests: [
      { kind: 'browser-save', name: 'p.html', text: PAGE_TAGGED, userDriven: true, dataLossPrev: null }
    ]
  },
  {
    name: 'page-state-restored',
    seed: [['p.html', PAGE_UNTAGGED]],
    requests: [
      { kind: 'browser-save', name: 'p.html', text: PAGE_TAGGED, userDriven: true },
      { kind: 'browser-save', name: 'p.html', text: PAGE_UNTAGGED, userDriven: true, dataLossPrev: null }
    ]
  },
  {
    name: 'data-api',
    seed: [['d.html', PLAIN]],
    requests: [
      { kind: 'data-api', name: 'd.html', text: PLAIN_NEXT, userDriven: false, dataLossPrev: null }
    ]
  },
  {
    name: 'create',
    seed: [['existing.html', PLAIN]],
    requests: [
      { kind: 'create', name: 'fresh.html', text: PLAIN_NEXT },
      { kind: 'create', name: 'existing.html', text: PLAIN_NEXT }
    ]
  },
  {
    name: 'sync-download-string',
    seed: [['s.html', PLAIN]],
    requests: [
      { kind: 'sync-download', name: 's.html', text: PLAIN_NEXT }
    ]
  },
  {
    name: 'sync-download-binary',
    seed: [],
    requests: [
      { kind: 'sync-download', name: 'img/logo.png', bytes: PNG }
    ]
  },
  {
    name: 'sync-download-latin1',
    seed: [],
    requests: [
      { kind: 'sync-download', name: 'weird.html', bytes: LATIN1 }
    ]
  },
  {
    name: 'sync-download-modified-at',
    seed: [['m.html', PLAIN]],
    requests: [
      { kind: 'sync-download', name: 'm.html', text: PLAIN_NEXT, modifiedAt: REAL_DATE.UTC(2021, 4, 5, 6, 7, 8, 9) }
    ]
  },
  {
    name: 'sync-download-page-record',
    seed: [['r.html', PAGE_UNTAGGED]],
    requests: [
      {
        kind: 'sync-download',
        name: 'r.html',
        text: PAGE_UNTAGGED,
        pageRecord: { v: 1, rev: PAGE_REV, tag: YJS_TAG }
      }
    ]
  },
  {
    name: 'sync-download-subfolder',
    seed: [],
    requests: [
      { kind: 'sync-download', name: 'deep/new/folder/doc.html', text: PLAIN_NEXT }
    ]
  },
  {
    name: 'restore',
    seed: [['a.html', PLAIN_NEXT]],
    requests: [
      { kind: 'restore', name: 'a.html', text: PLAIN }
    ]
  },
  {
    name: 'restore-safety',
    seed: [['a.html', PLAIN_NEXT]],
    requests: [
      { kind: 'restore', name: 'a.html', text: PLAIN, safety: true }
    ]
  },
  {
    name: 'guard-restore',
    seed: [['a.html', PLAIN_NEXT]],
    requests: [
      { kind: 'guard-restore', name: 'a.html', text: PLAIN, edit: true, safety: true }
    ]
  },
  {
    name: 'external-rebase',
    seed: [['p.html', PAGE_TAGGED]],
    requests: [
      { kind: 'external-rebase', name: 'p.html', text: PAGE_TAGGED_NEXT }
    ]
  },
  {
    name: 'restructure-move',
    seed: [
      ['a.html', PLAIN],
      ['.hyperclay/versions/a/2026-09-01-00-00-00-000+0000.html', PLAIN],
      ['.hyperclay/api/a.html.json', '{"v":1}']
    ],
    requests: [
      { structure: { kind: 'move', from: 'a.html', to: 'new/a.html' } }
    ]
  },
  {
    name: 'restructure-copy',
    seed: [['b.htmlclay', PAGE_TAGGED]],
    requests: [
      { structure: { kind: 'duplicate', from: 'b.htmlclay', to: 'b-copy.htmlclay' } }
    ]
  },
  {
    name: 'restructure-trash',
    seed: [['c.html', PLAIN]],
    requests: [
      { structure: { kind: 'trash', from: 'c.html', to: '.trash/c.html' } }
    ]
  },
  {
    name: 'restructure-folder-rename',
    seed: [['docs/one.html', PLAIN], ['docs/two.html', PLAIN_NEXT]],
    requests: [
      { structure: { kind: 'move', from: 'docs', to: 'archive' } }
    ]
  },
  {
    name: 'restructure-move-onto-existing',
    seed: [['x.html', PLAIN], ['y.html', PLAIN_NEXT]],
    requests: [
      { structure: { kind: 'move', from: 'x.html', to: 'y.html' } }
    ]
  },
  {
    name: 'ad2-stale-lineage',
    seed: [['stale.html', PAGE_UNTAGGED.replace(PAGE_REV, 'bbbbbbbbbbbbbbbb')]],
    storedRecord: { v: 1, rev: 'bbbbbbbbbbbbbbbb', tag: YJS_TAG },
    versionAtRevA: PAGE_UNTAGGED.replace(PAGE_REV, 'aaaaaaaaaaaaaaaa'),
    requests: [
      { kind: 'browser-save', name: 'stale.html', text: PAGE_UNTAGGED.replace(PAGE_REV, 'aaaaaaaaaaaaaaaa'), userDriven: true, dataLossPrev: null }
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
      if (rel === '.hyperclay/guard' || rel.startsWith('.hyperclay/guard/')) continue
      const full = path.join(dir, entry.name)
      if (entry.isDirectory()) walk(full, rel)
      else if (entry.isFile()) files.push({ path: rel, b64: fs.readFileSync(full).toString('base64') })
      else files.push({ path: rel, other: true })
    }
  }
  walk(root, '')
  return files
}

async function runScenario(scenario) {
  const root = fs.realpathSync(fs.mkdtempSync(path.join(tmp, 'root-')))
  activeRoot = root
  hooks.length = 0
  pageGenerations.length = 0
  pageGenerationCounter = 0
  tempCounter = 0

  for (const [rel, content] of scenario.seed) writeFile(root, rel, content)
  if (scenario.storedRecord) {
    writeFile(root, '.hyperclay/page-state/' + 'stale.html' + '.json', JSON.stringify(scenario.storedRecord))
  }
  if (scenario.versionAtRevA) {
    writeFile(root, '.hyperclay/versions/stale/2026-09-01-00-00-00-000+0000.html', scenario.versionAtRevA)
  }
  const before = tree(root)

  const authority = scenario.requests.some((request) => request.kind === 'external-rebase')
    ? boundaryOnlyAuthority
    : null
  const host = createRootDocumentHost({
    ctx: { root: { id: 'root-a', path: root, port: 0 }, localAuthority: authority },
    live,
    emit,
    logger: null
  })

  const results = []
  const modifiedTimes = {}
  recording = true
  for (const [index, request] of scenario.requests.entries()) {
    fakeNowMs = clock(index)
    if (request.structure) {
      const step = request.structure
      const fromLocation = path.join(root, step.from)
      const entry = {
        from: { location: fromLocation, canonical: fromLocation, isDir: fs.statSync(fromLocation).isDirectory(), isSymlink: false },
        to: { location: path.join(root, step.to), canonical: path.join(root, step.to) }
      }
      const answer = await host.restructure({
        kind: step.kind,
        rootId: 'root-a',
        steps: [{ from: step.from, to: step.to }],
        context: { token: null, entries: [entry] }
      })
      results.push(normalizeResult(answer))
      continue
    }
    const payload = {
      kind: request.kind,
      name: request.name,
      filePath: path.join(root, request.name)
    }
    if (request.text !== undefined) payload.bytes = request.text
    if (request.bytes !== undefined) payload.bytes = request.bytes
    if (request.userDriven !== undefined) payload.userDriven = request.userDriven
    if (request.saveId !== undefined) payload.saveId = request.saveId
    if (request.kind === 'browser-save' || request.kind === 'data-api') {
      payload.dataLossPrev = request.dataLossPrev === undefined ? null : request.dataLossPrev
    }
    if (request.edit !== undefined) payload.edit = request.edit
    if (request.safety !== undefined) payload.safety = request.safety
    if (request.expected !== undefined) payload.expected = request.expected
    if (request.modifiedAt !== undefined) payload.modifiedAt = request.modifiedAt
    if (request.pageRecord !== undefined) payload.pageRecord = request.pageRecord
    const result = payload.kind === 'external-rebase'
      ? await withDocumentWrite(
        async () => ({ canonicalPath: payload.filePath }),
        async (target, token) => host.commit({ ...payload, context: { target, token } })
      )
      : await host.commit(payload)
    results.push(normalizeResult(result))
    if (request.modifiedAt !== undefined) {
      modifiedTimes[request.name] = fs.statSync(path.join(root, request.name)).mtimeMs
    }
  }
  recording = false
  await new Promise((resolve) => setTimeout(resolve, 60))

  return {
    name: scenario.name,
    seed: scenario.seed.map(([rel, content]) => ({ path: rel, b64: Buffer.from(content).toString('base64') })),
    storedRecord: scenario.storedRecord || null,
    storedRecordB64: scenario.storedRecord ? Buffer.from(JSON.stringify(scenario.storedRecord)).toString('base64') : null,
    versionAtRevA: scenario.versionAtRevA || null,
    before: before,
    requests: scenario.requests.map((request, index) => {
      const out = { clock: clock(index), kind: request.kind, name: request.name }
      if (request.text !== undefined) out.text = request.text
      if (request.bytes !== undefined) out.b64 = request.bytes.toString('base64')
      if (request.userDriven !== undefined) out.userDriven = request.userDriven
      if (request.saveId !== undefined) out.saveId = request.saveId
      if (request.kind === 'browser-save' || request.kind === 'data-api') {
        out.dataLossPrev = request.dataLossPrev === undefined ? null : request.dataLossPrev
      }
      if (request.expected !== undefined) out.expected = request.expected
      if (request.modifiedAt !== undefined) out.modifiedAt = request.modifiedAt
      if (request.pageRecord !== undefined) out.pageRecord = request.pageRecord
      if (request.edit !== undefined) out.edit = request.edit
      if (request.safety !== undefined) out.safety = request.safety
      if (request.structure !== undefined) out.structure = request.structure
      return out
    }),
    results,
    modifiedTimes,
    hooks: hooks.slice(),
    pages: pageGenerations.slice(),
    tree: tree(root)
  }
}

function normalizeResult(result) {
  if (result === null || result === undefined) return null
  const out = {}
  for (const key of Object.keys(result)) {
    if (Object.getOwnPropertyDescriptor(result, key) && !Object.getOwnPropertyDescriptor(result, key).enumerable) continue
    const value = result[key]
    if (value === undefined) continue
    out[key] = Buffer.isBuffer(value) ? { b64: value.toString('base64') } : value
  }
  return out
}

async function main() {
  const answers = []
  for (const scenario of scenarios) answers.push(await runScenario(scenario))
  fs.writeFileSync(outPath, JSON.stringify({ scenarios: answers }, null, 2) + '\n')
}

main().catch((error) => {
  process.stderr.write((error && error.stack ? error.stack : String(error)) + '\n')
  process.exit(1)
})
