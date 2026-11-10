'use strict'

// Golden answers for the Go port of Local's atomic write and its `/_/api` data
// sidecar. Runs Local's real modules (write-queue.js, api-sidecar.js and the
// hyper-html-api engine behind data-extractor.js) over a temp root and records
// the resulting files, return values and non-fatal console failures, with the
// temp prefix replaced by <tmp> and each scenario's own root by <root>.
//
//   node gen.cjs <hyperclay-local checkout> <out.json>
//
// The pid in the temp file name and the process-wide temp counter are the only
// values that differ per run, so the temp name is recorded as a shape with both
// replaced, and the directory entries never include one (a leftover temp is a
// scenario failure, recorded as a flag instead).

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

const utilsDir = path.join(checkout, 'src', 'main', 'utils')
const writeQueue = require(path.join(utilsDir, 'write-queue.js'))
const sidecarMod = require(path.join(utilsDir, 'api-sidecar.js'))
const { API_DIR } = require(path.join(utilsDir, 'artifact-paths.js'))

const tmp = fs.realpathSync(fs.mkdtempSync(path.join(os.tmpdir(), 'storage-golden-')))
let activeRoot = tmp
let consoleLines = null

console.error = (...args) => {
  if (consoleLines) {
    consoleLines.push({ level: 'error', prefix: consolePrefix(line(args)) })
  }
}

function line(args) {
  return args.map((arg) => (arg instanceof Error ? arg.message : String(arg))).join(' ')
}

// A non-fatal failure emits one of Local's own sentences plus whatever Node's
// errno string adds, and an errno's portable identity is its code. Only the
// sentence is recorded: it names which non-fatal path ran, and it is the part
// the Go port reproduces exactly.
function consolePrefix(message) {
  const marker = ' failed (non-fatal)'
  const at = message.indexOf(marker)
  if (at >= 0) return message.slice(0, at + marker.length)
  return message.split(tmp).join('<tmp>')
}

function sha(data) {
  return crypto.createHash('sha256').update(data).digest('hex')
}

function rel(target) {
  return path.relative(activeRoot, target).split(path.sep).join('/')
}

// Every entry a scenario leaves behind: files with their mode, size, digest and
// text (the fixtures are all text), directories and symlinks.
function recordEntries(base) {
  const out = []
  const walk = (dir) => {
    const items = fs.readdirSync(dir, { withFileTypes: true })
    items.sort((a, b) => (a.name < b.name ? -1 : a.name > b.name ? 1 : 0))
    for (const item of items) {
      const full = path.join(dir, item.name)
      const entry = { path: rel(full) }
      const st = fs.lstatSync(full)
      if (st.isSymbolicLink()) {
        entry.kind = 'symlink'
        entry.target = fs.readlinkSync(full).split(path.sep).join('/')
        out.push(entry)
        continue
      }
      if (st.isDirectory()) {
        entry.kind = 'dir'
        out.push(entry)
        walk(full)
        continue
      }
      const data = fs.readFileSync(full)
      entry.kind = 'file'
      entry.mode = st.mode & 0o777
      entry.size = data.length
      entry.sha256 = sha(data)
      entry.text = data.toString('utf8')
      out.push(entry)
    }
  }
  if (fs.existsSync(base)) walk(base)
  return out.sort((a, b) => (a.path < b.path ? -1 : a.path > b.path ? 1 : 0))
}

function errorCode(error) {
  return error && error.code ? error.code : String(error && error.message ? error.message : error)
}

function applySetup(base, setup) {
  for (const op of setup) {
    const full = path.join(base, op.path)
    if (op.kind === 'dir') {
      fs.mkdirSync(full, { recursive: true })
      if (op.mode) fs.chmodSync(full, op.mode)
      continue
    }
    if (op.kind === 'symlink') {
      fs.mkdirSync(path.dirname(full), { recursive: true })
      fs.symlinkSync(op.target, full)
      continue
    }
    fs.mkdirSync(path.dirname(full), { recursive: true })
    fs.writeFileSync(full, Buffer.from(op.contentB64, 'base64'), op.mode ? { mode: op.mode } : undefined)
    if (op.mode) fs.chmodSync(full, op.mode)
    if (typeof op.mtimeMs === 'number') fs.utimesSync(full, op.mtimeMs / 1000, op.mtimeMs / 1000)
  }
}

const RULES_TAG = JSON.stringify({
  title: 'h1',
  links: 'a@href',
  items: 'ul.items li[]',
  intro: 'p@innerHTML'
})

const DOC_TAG = [
  '<!doctype html>',
  '<html>',
  '  <head>',
  '    <script data-rules-name="api" data-rules-version="1">' + RULES_TAG + '</script>',
  '  </head>',
  '  <body>',
  '    <h1>Hello &amp; welcome</h1>',
  '    <ul class="items"><li>one</li><li>two</li></ul>',
  '    <ul class="links"><li><a href="/a">a</a></li><li><a href="/b">b</a></li></ul>',
  '    <p>Some <b>bold</b> text</p>',
  '  </body>',
  '</html>',
  ''
].join('\n')

const DOC_NO_TAG = '<!doctype html>\n<html><body><h1>Plain</h1></body></html>\n'

const DOC_BAD_TAG =
  '<!doctype html>\n<html><head><script data-rules-name="api" data-rules-version="2">{"title":"h1"}</script></head>' +
  '<body><h1>x</h1></body></html>\n'

const atomicScenarios = []
const sidecarScenarios = []
const T0 = Date.UTC(2024, 3, 1, 12, 0, 0, 0)

async function atomicScenario(name, setup, action) {
  const dir = path.join(tmp, 'atomic', name)
  fs.mkdirSync(dir, { recursive: true })
  activeRoot = dir
  applySetup(dir, setup)
  consoleLines = []

  const target = path.join(dir, action.path)
  let observedTemp = null
  let beforeRename = null
  if (action.beforeRename === 'throw') {
    beforeRename = () => {
      throw new Error('blocked by test')
    }
  } else if (action.observeTemp) {
    beforeRename = () => {
      const parent = path.dirname(target)
      const temp = fs.readdirSync(parent).find((entry) => entry.startsWith('.') && entry.endsWith('.tmp'))
      if (!temp) throw new Error('no temp file observed')
      const parsed = /^\.(.*)\.(\d+)\.(\d+)\.tmp$/.exec(temp)
      if (!parsed) throw new Error('unexpected temp name ' + temp)
      observedTemp = {
        shape: '.<base>.<pid>.<n>.tmp',
        base: parsed[1],
        mode: fs.statSync(path.join(parent, temp)).mode & 0o777
      }
    }
  }

  const content = Buffer.from(action.contentB64, 'base64')
  let result
  try {
    await writeQueue.atomicWriteFile(target, content, action.encoding === 'buffer' ? null : 'utf8', beforeRename)
    result = null
  } catch (error) {
    result = errorCode(error)
  }

  const entries = recordEntries(dir)
  atomicScenarios.push({
    name,
    setup,
    action: {
      path: action.path,
      encoding: action.encoding,
      contentB64: action.contentB64,
      beforeRename: action.beforeRename === undefined ? null : action.beforeRename,
      observeTemp: action.observeTemp === true
    },
    result,
    temp: observedTemp,
    tempLeft: entries.some((entry) => entry.path.endsWith('.tmp')),
    console: consoleLines,
    entries
  })
}

async function sidecarScenario(name, setup, actions) {
  const dir = path.join(tmp, 'sidecar', name)
  fs.mkdirSync(path.join(dir, 'root'), { recursive: true })
  activeRoot = dir
  applySetup(dir, setup)
  consoleLines = []

  const results = []
  const recorded = []
  for (const action of actions) {
    results.push(await runSidecarAction(path.join(dir, 'root'), action))
    recorded.push(recordedAction(action))
  }

  sidecarScenarios.push({
    name,
    setup,
    actions: recorded,
    results,
    console: consoleLines,
    entries: recordEntries(dir)
  })
}

// A recorded action carries the exact bytes Local wrote, so the Go port replays
// the same content rather than re-serializing (and possibly re-ordering) it.
function recordedAction(action) {
  const out = Object.assign({}, action)
  if (action.kind === 'writeSidecarData') {
    out.dataText = action.data === undefined || action.data === null ? null : JSON.stringify(action.data)
  }
  return out
}

async function runSidecarAction(root, action) {
  switch (action.kind) {
    case 'writeSidecar':
      await sidecarMod.writeApiSidecar(root, action.name, action.html)
      return null
    case 'writeSidecarData':
      await sidecarMod.writeApiSidecarData(root, action.name, action.data === undefined ? null : action.data)
      return null
    case 'deleteSidecar':
      await sidecarMod.deleteApiSidecar(root, action.name)
      return null
    case 'readFresh': {
      const text = await sidecarMod.readFreshSidecar(root, action.name, action.sourceMtimeMs)
      return { text: text === null ? null : text }
    }
    case 'setTimes': {
      const full = path.join(root, action.path)
      fs.utimesSync(full, action.mtimeMs / 1000, action.mtimeMs / 1000)
      return null
    }
    case 'resolve': {
      try {
        return { path: rel(await sidecarMod.resolveSidecarCanonical(root, action.name)) }
      } catch (error) {
        return { error: errorCode(error) }
      }
    }
  }
  throw new Error('unknown sidecar action ' + action.kind)
}

function b64(text) {
  return Buffer.from(text, 'utf8').toString('base64')
}

async function main() {
  // ---- atomic write -------------------------------------------------------
  await atomicScenario('new-file-missing-folder', [], {
    path: 'nested/deeper/new.txt',
    encoding: 'utf8',
    contentB64: b64('hello\n')
  })
  await atomicScenario('replace-0600', [
    { path: 'save.txt', kind: 'file', mode: 0o600, contentB64: b64('old\n') }
  ], { path: 'save.txt', encoding: 'utf8', contentB64: b64('new content\n') })
  await atomicScenario('replace-0664', [
    { path: 'save.txt', kind: 'file', mode: 0o664, contentB64: b64('old\n') }
  ], { path: 'save.txt', encoding: 'utf8', contentB64: b64('new content\n') })
  await atomicScenario('buffer-non-ascii', [
    { path: 'notes.txt', kind: 'file', mode: 0o644, contentB64: b64('old\n') }
  ], { path: 'notes.txt', encoding: 'buffer', contentB64: b64('héllo — ✓\n') })
  await atomicScenario('string-non-ascii', [], {
    path: 'café.txt',
    encoding: 'utf8',
    contentB64: b64('héllo — ✓\n')
  })
  await atomicScenario('before-rename-throws', [
    { path: 'keep.txt', kind: 'file', mode: 0o640, contentB64: b64('untouched\n') }
  ], { path: 'keep.txt', encoding: 'utf8', contentB64: b64('replacement\n'), beforeRename: 'throw' })
  await atomicScenario('target-is-directory', [
    { path: 'taken', kind: 'dir' },
    { path: 'taken/inside.txt', kind: 'file', mode: 0o644, contentB64: b64('still here\n') }
  ], { path: 'taken', encoding: 'utf8', contentB64: b64('nope\n') })
  await atomicScenario('temp-name-pattern', [
    { path: 'doc.html', kind: 'file', mode: 0o600, contentB64: b64('old\n') }
  ], { path: 'doc.html', encoding: 'utf8', contentB64: b64('new\n'), observeTemp: true })

  // ---- sidecar ------------------------------------------------------------
  await sidecarScenario('doc-with-tag', [], [
    { kind: 'writeSidecar', name: 'page.html', html: DOC_TAG }
  ])
  await sidecarScenario('doc-without-tag', [], [
    { kind: 'writeSidecarData', name: 'page.html', data: { stale: true } },
    { kind: 'writeSidecar', name: 'page.html', html: DOC_NO_TAG }
  ])
  await sidecarScenario('doc-bad-tag', [], [
    { kind: 'writeSidecarData', name: 'page.html', data: { stale: true } },
    { kind: 'writeSidecar', name: 'page.html', html: DOC_BAD_TAG }
  ])
  await sidecarScenario('subfolder-doc', [], [
    { kind: 'writeSidecar', name: 'blog/post.html', html: DOC_TAG }
  ])
  await sidecarScenario('space-non-ascii-doc', [], [
    {
      kind: 'writeSidecarData',
      name: 'Folder With Space/café.html',
      data: { title: 'Café ☕', items: ['one', 'two'] }
    }
  ])
  await sidecarScenario('data-empty-values', [], [
    { kind: 'writeSidecarData', name: 'empties.html', data: [] },
    { kind: 'writeSidecarData', name: 'empty-object.html', data: {} }
  ])
  await sidecarScenario('delete-present', [], [
    { kind: 'writeSidecarData', name: 'gone.html', data: { a: 1 } },
    { kind: 'deleteSidecar', name: 'gone.html' },
    { kind: 'deleteSidecar', name: 'gone.html' }
  ])
  await sidecarScenario('delete-absent', [], [
    { kind: 'deleteSidecar', name: 'never.html' }
  ])
  await sidecarScenario('read-newer', [], [
    { kind: 'writeSidecarData', name: 'page.html', data: { v: 1 } },
    { kind: 'setTimes', path: '.hyperclay/api-v2/page.html.json', mtimeMs: T0 + 1000 },
    { kind: 'readFresh', name: 'page.html', sourceMtimeMs: T0 }
  ])
  await sidecarScenario('read-equal', [], [
    { kind: 'writeSidecarData', name: 'page.html', data: { v: 1 } },
    { kind: 'setTimes', path: '.hyperclay/api-v2/page.html.json', mtimeMs: T0 },
    { kind: 'readFresh', name: 'page.html', sourceMtimeMs: T0 }
  ])
  await sidecarScenario('read-older', [], [
    { kind: 'writeSidecarData', name: 'page.html', data: { v: 1 } },
    { kind: 'setTimes', path: '.hyperclay/api-v2/page.html.json', mtimeMs: T0 - 1000 },
    { kind: 'readFresh', name: 'page.html', sourceMtimeMs: T0 }
  ])
  await sidecarScenario('read-absent', [], [
    { kind: 'readFresh', name: 'missing.html', sourceMtimeMs: T0 }
  ])
  await sidecarScenario('hyperclay-symlink-outside', [
    { path: 'outside/.hyperclay/api-v2', kind: 'dir' },
    { path: 'outside/.hyperclay/api-v2/keep.json', kind: 'file', mode: 0o644, contentB64: b64('{"keep":true}') },
    { path: 'root/.hyperclay', kind: 'symlink', target: '../outside/.hyperclay' }
  ], [
    { kind: 'writeSidecarData', name: 'page.html', data: { a: 1 } },
    { kind: 'deleteSidecar', name: 'page.html' },
    { kind: 'readFresh', name: 'page.html', sourceMtimeMs: T0 },
    { kind: 'resolve', name: 'page.html' }
  ])
  await sidecarScenario('name-escape', [], [
    { kind: 'writeSidecarData', name: '../x.html', data: { a: 1 } },
    { kind: 'writeSidecar', name: '../x.html', html: DOC_TAG },
    { kind: 'deleteSidecar', name: '../x.html' },
    { kind: 'readFresh', name: '../x.html', sourceMtimeMs: T0 },
    { kind: 'resolve', name: '../x.html' },
    { kind: 'writeSidecarData', name: '../../../x.html', data: { a: 2 } },
    { kind: 'writeSidecar', name: '../../../x.html', html: DOC_TAG },
    { kind: 'deleteSidecar', name: '../../../x.html' },
    { kind: 'readFresh', name: '../../../x.html', sourceMtimeMs: T0 },
    { kind: 'resolve', name: '../../../x.html' }
  ])

  // ---- pure resolution ----------------------------------------------------
  const resolveRoot = path.join(tmp, 'paths', 'root')
  fs.mkdirSync(resolveRoot, { recursive: true })
  activeRoot = resolveRoot
  const paths = []
  const names = [
    'page.html',
    'blog/post.html',
    'Folder With Space/café.html',
    '',
    '../x.html',
    'a/../../x.html',
    'nested/deep/doc.html',
    '../../../x.html',
    '../../../../x.html'
  ]
  for (const name of names) {
    const row = { name, relPath: null, lexical: null, canonical: null }
    try {
      row.relPath = sidecarMod.sidecarRelPath(name)
      row.lexical = rel(sidecarMod.resolveSidecarPath(resolveRoot, name))
      row.canonical = rel(await sidecarMod.resolveSidecarCanonical(resolveRoot, name))
    } catch (error) {
      row.error = errorCode(error)
    }
    paths.push(row)
  }

  const golden = {
    generatedWith: {
      node: process.version,
      platform: process.platform,
      module: 'hyperclay-local/src/main/utils/{write-queue,api-sidecar,artifact-paths}.js'
    },
    constants: {
      sidecarDir: API_DIR,
      tempMode: 0o600,
      defaultMode: 0o644,
      renameAttempts: 6,
      renameRetryCodes: ['EACCES', 'EBUSY', 'EPERM'],
      dirSyncUnsupported: ['EACCES', 'EBADF', 'EINVAL', 'EISDIR', 'ENOTSUP', 'EPERM']
    },
    atomic: atomicScenarios,
    sidecar: sidecarScenarios,
    paths
  }

  fs.writeFileSync(outPath, JSON.stringify(golden, null, 2) + '\n')
}

main()
  .catch((error) => {
    console.error(error)
    process.exit(1)
  })
  .finally(() => {
    fs.rmSync(tmp, { recursive: true, force: true })
  })
