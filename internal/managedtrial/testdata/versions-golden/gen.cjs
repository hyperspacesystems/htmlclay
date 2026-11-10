'use strict'

// Golden answers for the Go port of Local's version store. Runs Local's real
// modules (backup.js, prune-versions.js, real-dir-chain.js) over a temp root
// and records the resulting files, return values, emitted events, logger calls
// and console lines, with the temp prefix replaced by <root>.
//
//   TZ=America/New_York node gen.cjs <hyperclay-local checkout> <out.json>
//
// backup.js takes `now` in generateTimestamp/planVersionName but calls
// `new Date()` inside publishVersion and `Date.now()` inside maybePrune, so the
// whole date source is stubbed below (the Date constructor itself: `new Date()`
// does not go through Date.now).

const fs = require('fs')
const os = require('os')
const path = require('path')
const crypto = require('crypto')
const { execFileSync } = require('child_process')

const REAL_DATE = Date
let fakeNowMs = null
class FakeDate extends REAL_DATE {
  constructor(...args) {
    if (args.length === 0) super(fakeNowMs === null ? REAL_DATE.now() : fakeNowMs)
    else super(...args)
  }

  static now() {
    return fakeNowMs === null ? REAL_DATE.now() : fakeNowMs
  }
}
global.Date = FakeDate

const INSTANTS = [
  Date.UTC(2024, 2, 10, 6, 59, 59, 999),
  Date.UTC(2024, 2, 10, 7, 0, 0, 0),
  Date.UTC(2024, 10, 3, 5, 30, 0, 0),
  Date.UTC(2024, 10, 3, 6, 30, 0, 0),
  Date.UTC(2024, 10, 3, 6, 59, 59, 999),
  Date.UTC(2024, 10, 3, 7, 0, 0, 0),
  Date.UTC(2024, 6, 4, 16, 0, 0, 0),
  Date.UTC(2024, 0, 15, 5, 0, 0, 0),
  Date.UTC(2024, 0, 15, 5, 0, 0, 7)
]

if (process.argv[2] === '--timestamps') {
  const instants = JSON.parse(process.argv[3])
  const { generateTimestamp } = require(path.join(process.argv[4], 'src', 'main', 'utils', 'backup.js'))
  process.stdout.write(JSON.stringify(instants.map((ms) => ({
    instant: ms,
    stamp: generateTimestamp(new Date(ms))
  }))))
  process.exit(0)
}

const checkout = process.argv[2]
const outPath = process.argv[3]
if (!checkout || !outPath) {
  process.stderr.write('usage: node gen.cjs <hyperclay-local checkout> <out.json>\n')
  process.exit(1)
}
if (process.env.TZ !== 'America/New_York') {
  process.stderr.write('run with TZ=America/New_York\n')
  process.exit(1)
}

const utilsDir = path.join(checkout, 'src', 'main', 'utils')
const backupMod = require(path.join(utilsDir, 'backup.js'))
const pruneMod = require(path.join(utilsDir, 'prune-versions.js'))
const { VERSIONS_DIR, RECOVERED_DIR } = require(path.join(utilsDir, 'artifact-paths.js'))
const realDir = require(path.join(utilsDir, 'real-dir-chain.js'))

const tmp = fs.realpathSync(fs.mkdtempSync(path.join(os.tmpdir(), 'versions-golden-')))
const DAY = 24 * 60 * 60 * 1000

let active = null
let activeBase = ''

function norm(value) {
  if (typeof value !== 'string') return value
  return value.split(activeBase).join('<root>')
}

function line(args) {
  return args.map((arg) => {
    if (typeof arg === 'string') return norm(arg)
    if (arg instanceof Error) return norm(arg.message)
    return String(arg)
  }).join(' ')
}

function fieldsOf(obj) {
  const out = []
  for (const key of Object.keys(obj || {})) {
    const value = obj[key]
    out.push({ key, value: norm(value instanceof Error ? value.message : String(value)) })
  }
  return out
}

console.log = (...args) => { if (active) active.console.push({ level: 'log', message: line(args) }) }
console.warn = (...args) => { if (active) active.console.push({ level: 'warn', message: line(args) }) }
console.error = (...args) => { if (active) active.console.push({ level: 'error', message: line(args) }) }

const emit = (event, payload) => {
  if (active) active.emit.push({ event, fields: fieldsOf(payload) })
}

const logger = {
  info(tag, message, fields) {
    if (active) active.log.push({ level: 'info', tag, message: norm(message), fields: fieldsOf(fields) })
  },
  error(tag, message, fields) {
    if (active) active.log.push({ level: 'error', tag, message: norm(message), fields: fieldsOf(fields) })
  }
}

const sleep = (ms) => new Promise((resolve) => setTimeout(resolve, ms))

// backup.js keeps assertVersionsChain private; this is its body verbatim, in
// terms of the same real-dir-chain helpers it calls.
async function assertVersionsChain(base, versionsDir) {
  const canonicalBase = await realDir.canonicalizeBase(base)
  await realDir.assertRealDirChain(canonicalBase, realDir.rebaseOntoCanonical(canonicalBase, base, versionsDir))
}

function applyOp(base, op) {
  const target = path.join(base, op.path)
  if (op.op === 'mkdir') {
    fs.mkdirSync(target, { recursive: true })
  } else if (op.op === 'write') {
    fs.mkdirSync(path.dirname(target), { recursive: true })
    fs.writeFileSync(target, Buffer.from(op.contentB64, 'base64'))
  } else if (op.op === 'symlink') {
    fs.mkdirSync(path.dirname(target), { recursive: true })
    fs.symlinkSync(op.target, target, 'dir')
  } else if (op.op === 'mtime') {
    fs.utimesSync(target, op.mtimeMs / 1000, op.mtimeMs / 1000)
  } else {
    throw new Error(`unknown setup op ${op.op}`)
  }
}

function listFiles(root) {
  const out = []
  function walk(dir) {
    let entries
    try {
      entries = fs.readdirSync(dir, { withFileTypes: true })
    } catch {
      return
    }
    for (const entry of entries) {
      const full = path.join(dir, entry.name)
      if (entry.isDirectory()) walk(full)
      else if (entry.isFile()) {
        const bytes = fs.readFileSync(full)
        out.push({
          path: path.relative(root, full).split(path.sep).join('/'),
          size: bytes.length,
          sha256: crypto.createHash('sha256').update(bytes).digest('hex')
        })
      }
    }
  }
  walk(root)
  out.sort((a, b) => (a.path < b.path ? -1 : a.path > b.path ? 1 : 0))
  return out
}

async function runAction(base, action) {
  if (typeof action.clock === 'number') fakeNowMs = action.clock
  switch (action.kind) {
    case 'createBackup': {
      const created = await backupMod.createBackup(base, action.site, Buffer.from(action.contentB64, 'base64').toString('utf8'), emit, logger)
      return created === null ? null : norm(created)
    }
    case 'createBackupIfExists': {
      const created = await backupMod.createBackupIfExists(path.join(base, action.filePath), action.site, base, emit, logger)
      return created === null ? null : norm(created)
    }
    case 'createBinaryBackup': {
      const created = await backupMod.createBinaryBackup(base, action.upload, Buffer.from(action.contentB64, 'base64'), emit, logger)
      return created === null ? null : norm(created)
    }
    case 'createBinaryBackupIfExists': {
      const created = await backupMod.createBinaryBackupIfExists(path.join(base, action.filePath), action.upload, base, emit, logger)
      return created === null ? null : norm(created)
    }
    case 'publishRecoveredCopy': {
      try {
        const published = await backupMod.publishRecoveredCopy(base, action.rel, action.html)
        return { error: null, filename: published.filename, full: norm(published.full) }
      } catch (error) {
        return { error: norm(error.message), filename: null, full: null }
      }
    }
    case 'assertVersionsChain': {
      try {
        await assertVersionsChain(base, path.join(base, action.versionsDir))
        return { error: null }
      } catch (error) {
        return { error: norm(error.message) }
      }
    }
    case 'pruneSiteVersions': {
      const result = await pruneMod.pruneSiteVersions(base, path.join(base, action.dir), action.now)
      return { kept: result.kept, deleted: result.deleted }
    }
    case 'pruneAllVersions': {
      const result = await pruneMod.pruneAllVersions(base, action.now)
      return { sites: result.sites, deleted: result.deleted }
    }
    default:
      throw new Error(`unknown action kind ${action.kind}`)
  }
}

function parseRows(base, dir, filenames) {
  return filenames.map((filename) => {
    const full = path.join(base, dir, filename)
    const mtimeMs = fs.statSync(full).mtimeMs
    const entry = { name: filename, mtimeMs }
    const parsed = pruneMod.parseVersionTimestamp(filename)
    return {
      filename,
      mtimeMs,
      isVersion: pruneMod.VERSION_NAME.test(filename),
      parsed,
      sortKey: pruneMod.sortKey(entry),
      suffix: pruneMod.collisionSuffix(filename),
      stamp: pruneMod.versionStamp(filename)
    }
  })
}

async function main() {
  const scenarios = []
  for (const spec of SCENARIOS) {
    const base = path.join(tmp, spec.name)
    fs.mkdirSync(base, { recursive: true })
    for (const op of spec.setup) applyOp(base, op)

    const record = {
      name: spec.name,
      setup: spec.setup,
      actions: spec.actions,
      results: [],
      console: [],
      emit: [],
      log: [],
      files: []
    }
    active = record
    activeBase = base
    fakeNowMs = null

    for (const action of spec.actions) {
      record.results.push({
        kind: action.kind,
        value: action.kind === 'parse'
          ? { rows: parseRows(base, action.dir, action.filenames) }
          : await runAction(base, action)
      })
    }

    if (spec.settle) await sleep(spec.settle)
    fakeNowMs = null
    active = null
    record.files = listFiles(base)
    scenarios.push(record)
  }

  const kolkata = JSON.parse(execFileSync(process.execPath, [__filename, '--timestamps', JSON.stringify(INSTANTS), checkout], {
    env: { ...process.env, TZ: 'Asia/Kolkata' },
    encoding: 'utf8'
  }))

  const ny = INSTANTS.map((ms) => ({ instant: ms, stamp: backupMod.generateTimestamp(new Date(ms)) }))

  const golden = {
    zone: 'America/New_York',
    secondZone: 'Asia/Kolkata',
    timestamps: { 'America/New_York': ny, 'Asia/Kolkata': kolkata },
    constants: {
      versionsDir: VERSIONS_DIR,
      recoveredDir: RECOVERED_DIR,
      maxAgeMs: pruneMod.MAX_AGE_MS,
      keepNewest: pruneMod.KEEP_NEWEST,
      versionName: pruneMod.VERSION_NAME.source,
      versionNameFlags: pruneMod.VERSION_NAME.flags
    },
    scenarios
  }

  fs.writeFileSync(outPath, `${JSON.stringify(golden, null, 2)}\n`)
  fs.rmSync(tmp, { recursive: true, force: true })
}

const PRUNE_NOW = Date.UTC(2026, 5, 1, 16, 0, 0)
const B64 = (text) => Buffer.from(text, 'utf8').toString('base64')

function versionAt(ms) {
  return backupMod.generateTimestamp(new Date(ms))
}

function pruneFixture() {
  const setup = [{ op: 'mkdir', path: '.hyperclay/versions/site' }]
  const names = []
  for (let k = 0; k < 30; k += 1) {
    const name = `${versionAt(PRUNE_NOW - k * 3 * DAY)}.html`
    names.push(name)
    setup.push({ op: 'write', path: `.hyperclay/versions/site/${name}`, contentB64: B64(`version ${k}\n`) })
    setup.push({ op: 'mtime', path: `.hyperclay/versions/site/${name}`, mtimeMs: PRUNE_NOW - k * 3 * DAY })
  }
  setup.push({ op: 'write', path: '.hyperclay/versions/site/2020-01-01-00-00-00-000.html', contentB64: B64('old legacy\n') })
  setup.push({ op: 'mtime', path: '.hyperclay/versions/site/2020-01-01-00-00-00-000.html', mtimeMs: PRUNE_NOW - 200 * DAY })
  setup.push({ op: 'write', path: '.hyperclay/versions/site/2021-05-05-05-05-05-000.html', contentB64: B64('fresh legacy\n') })
  setup.push({ op: 'mtime', path: '.hyperclay/versions/site/2021-05-05-05-05-05-000.html', mtimeMs: PRUNE_NOW - DAY })
  setup.push({ op: 'write', path: '.hyperclay/versions/site/notes.txt', contentB64: B64('not a version\n') })
  return setup
}

const SCENARIOS = [
  {
    name: 'publish-same-millisecond',
    setup: [{ op: 'mkdir', path: '.hyperclay/versions/site' }],
    settle: 200,
    actions: [
      { kind: 'createBackup', clock: Date.UTC(2026, 10, 1, 5, 30, 0, 431), site: 'site', contentB64: B64('<h1>one</h1>\n') },
      { kind: 'createBackup', clock: Date.UTC(2026, 10, 1, 5, 30, 0, 431), site: 'site', contentB64: B64('<h1>two</h1>\n') },
      { kind: 'createBackup', clock: Date.UTC(2026, 10, 1, 5, 30, 0, 431), site: 'site', contentB64: B64('<h1>three</h1>\n') }
    ]
  },
  {
    name: 'publish-clock-rollback',
    setup: [{ op: 'mkdir', path: '.hyperclay/versions/site' }],
    settle: 200,
    actions: [
      { kind: 'createBackup', clock: Date.UTC(2026, 10, 1, 5, 30, 0, 431), site: 'site', contentB64: B64('<h1>first</h1>\n') },
      { kind: 'createBackup', clock: Date.UTC(2026, 10, 1, 5, 29, 55, 0), site: 'site', contentB64: B64('<h1>rolled back</h1>\n') }
    ]
  },
  {
    name: 'backup-subfolder',
    setup: [],
    settle: 200,
    actions: [
      { kind: 'createBackup', clock: Date.UTC(2026, 6, 4, 16, 0, 0, 0), site: 'blog/posts', contentB64: B64('<p>nested</p>\n') }
    ]
  },
  {
    name: 'backup-spaced-unicode',
    setup: [],
    settle: 200,
    actions: [
      { kind: 'createBackup', clock: Date.UTC(2026, 6, 4, 16, 0, 0, 0), site: 'Folder With Space/café', contentB64: B64('<p>héllo</p>\n') }
    ]
  },
  {
    name: 'binary-upload',
    setup: [],
    settle: 200,
    actions: [
      { kind: 'createBinaryBackup', clock: Date.UTC(2026, 6, 4, 16, 0, 0, 0), upload: 'images/logo.png', contentB64: Buffer.from([0x89, 0x50, 0x4e, 0x47, 0x00, 0x01, 0xff, 0xfe, 0x7f]).toString('base64') }
    ]
  },
  {
    name: 'binary-upload-if-exists-missing',
    setup: [],
    actions: [
      { kind: 'createBinaryBackupIfExists', clock: Date.UTC(2026, 6, 4, 16, 0, 0, 0), filePath: 'images/missing.png', upload: 'images/missing.png' }
    ]
  },
  {
    name: 'recovered-copy',
    setup: [],
    settle: 200,
    actions: [
      { kind: 'publishRecoveredCopy', clock: Date.UTC(2026, 10, 3, 6, 30, 0, 0), rel: 'docs/my page.html', html: '<p>recovered</p>\n' }
    ]
  },
  {
    name: 'recovered-copy-invalid-path',
    setup: [],
    actions: [
      { kind: 'publishRecoveredCopy', clock: Date.UTC(2026, 10, 3, 6, 30, 0, 0), rel: '../escape.html', html: '<p>nope</p>\n' }
    ]
  },
  {
    name: 'backup-if-exists-missing',
    setup: [],
    actions: [
      { kind: 'createBackupIfExists', clock: Date.UTC(2026, 6, 4, 16, 0, 0, 0), filePath: 'missing.html', site: 'site' }
    ]
  },
  {
    name: 'backup-if-exists-present',
    setup: [{ op: 'write', path: 'site.html', contentB64: B64('<p>live</p>\n') }],
    settle: 200,
    actions: [
      { kind: 'createBackupIfExists', clock: Date.UTC(2026, 6, 4, 16, 0, 0, 0), filePath: 'site.html', site: 'site' }
    ]
  },
  {
    name: 'parse-names',
    setup: [
      { op: 'write', path: '.hyperclay/versions/site/2026-11-01-01-30-00-431-0400.html', contentB64: B64('a\n') },
      { op: 'mtime', path: '.hyperclay/versions/site/2026-11-01-01-30-00-431-0400.html', mtimeMs: Date.UTC(2030, 0, 1) },
      { op: 'write', path: '.hyperclay/versions/site/2026-11-01-01-30-00-431-0500.html', contentB64: B64('b\n') },
      { op: 'mtime', path: '.hyperclay/versions/site/2026-11-01-01-30-00-431-0500.html', mtimeMs: Date.UTC(2030, 0, 1) },
      { op: 'write', path: '.hyperclay/versions/site/2026-03-10-12-00-00-000Z.html', contentB64: B64('c\n') },
      { op: 'mtime', path: '.hyperclay/versions/site/2026-03-10-12-00-00-000Z.html', mtimeMs: Date.UTC(2030, 0, 1) },
      { op: 'write', path: '.hyperclay/versions/site/2026-05-01-09-00-00-000.html', contentB64: B64('d\n') },
      { op: 'mtime', path: '.hyperclay/versions/site/2026-05-01-09-00-00-000.html', mtimeMs: Date.UTC(2021, 0, 1) },
      { op: 'write', path: '.hyperclay/versions/site/2026-11-01-01-30-00-431-0400-001.html', contentB64: B64('e\n') },
      { op: 'mtime', path: '.hyperclay/versions/site/2026-11-01-01-30-00-431-0400-001.html', mtimeMs: Date.UTC(2030, 0, 1) },
      { op: 'write', path: '.hyperclay/versions/site/2026-11-01-01-30-00-431+0530.html', contentB64: B64('f\n') },
      { op: 'mtime', path: '.hyperclay/versions/site/2026-11-01-01-30-00-431+0530.html', mtimeMs: Date.UTC(2030, 0, 1) },
      { op: 'write', path: '.hyperclay/versions/site/notes.txt', contentB64: B64('g\n') },
      { op: 'mtime', path: '.hyperclay/versions/site/notes.txt', mtimeMs: Date.UTC(2030, 0, 1) },
      { op: 'write', path: '.hyperclay/versions/site/.hyperclay-ver-deadbeef.tmp', contentB64: B64('h\n') },
      { op: 'mtime', path: '.hyperclay/versions/site/.hyperclay-ver-deadbeef.tmp', mtimeMs: Date.UTC(2030, 0, 1) }
    ],
    actions: [
      {
        kind: 'parse',
        dir: '.hyperclay/versions/site',
        filenames: [
          '2026-11-01-01-30-00-431-0400.html',
          '2026-11-01-01-30-00-431-0500.html',
          '2026-03-10-12-00-00-000Z.html',
          '2026-05-01-09-00-00-000.html',
          '2026-11-01-01-30-00-431-0400-001.html',
          '2026-11-01-01-30-00-431+0530.html',
          'notes.txt',
          '.hyperclay-ver-deadbeef.tmp'
        ]
      }
    ]
  },
  {
    name: 'prune-site',
    setup: pruneFixture(),
    settle: 200,
    actions: [
      { kind: 'pruneSiteVersions', clock: PRUNE_NOW, now: PRUNE_NOW, dir: '.hyperclay/versions/site' }
    ]
  },
  {
    name: 'prune-all',
    setup: pruneFixture(),
    settle: 200,
    actions: [
      { kind: 'pruneAllVersions', clock: PRUNE_NOW, now: PRUNE_NOW }
    ]
  },
  {
    name: 'prune-refuses-symlinked-hyperclay',
    setup: [
      { op: 'mkdir', path: 'alt/versions/site' },
      { op: 'write', path: 'alt/versions/site/2026-01-01-00-00-00-000-0500.html', contentB64: B64('x\n') },
      { op: 'symlink', path: '.hyperclay', target: 'alt' }
    ],
    actions: [
      { kind: 'pruneSiteVersions', clock: PRUNE_NOW, now: PRUNE_NOW, dir: '.hyperclay/versions/site' },
      { kind: 'assertVersionsChain', versionsDir: '.hyperclay/versions/site' }
    ]
  },
  {
    name: 'prune-refuses-symlinked-versions',
    setup: [
      { op: 'mkdir', path: '.hyperclay' },
      { op: 'mkdir', path: 'alt-versions/site' },
      { op: 'write', path: 'alt-versions/site/2026-01-01-00-00-00-000-0500.html', contentB64: B64('x\n') },
      { op: 'symlink', path: '.hyperclay/versions', target: '../alt-versions' }
    ],
    actions: [
      { kind: 'pruneSiteVersions', clock: PRUNE_NOW, now: PRUNE_NOW, dir: '.hyperclay/versions/site' },
      { kind: 'pruneAllVersions', clock: PRUNE_NOW, now: PRUNE_NOW },
      { kind: 'assertVersionsChain', versionsDir: '.hyperclay/versions/site' }
    ]
  },
  {
    name: 'backup-refuses-symlinked-hyperclay',
    setup: [
      { op: 'mkdir', path: 'alt/versions' },
      { op: 'symlink', path: '.hyperclay', target: 'alt' }
    ],
    settle: 200,
    actions: [
      { kind: 'createBackup', clock: Date.UTC(2026, 6, 4, 16, 0, 0, 0), site: 'site', contentB64: B64('<p>nope</p>\n') }
    ]
  },
  {
    name: 'real-chain',
    setup: [{ op: 'mkdir', path: '.hyperclay/versions/site' }],
    actions: [
      { kind: 'assertVersionsChain', versionsDir: '.hyperclay/versions/site' },
      { kind: 'assertVersionsChain', versionsDir: '.hyperclay/versions/not-yet-made' }
    ]
  }
]

main().catch((error) => {
  process.stderr.write(`${error && error.stack ? error.stack : error}\n`)
  process.exit(1)
})
