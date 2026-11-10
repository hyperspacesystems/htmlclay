'use strict'

// Golden answers for the Go port of Local's page-state store,
// src/main/page-state-store.js: the sidecar that keeps a page editor's merge
// state out of the person's file.
//
//   node gen.cjs <hyperclay-local checkout> <out.json>
//
// The store asks crypto.randomBytes(16) for every new page lineage, so the
// generator stubs it with a counter before the module loads: call n answers 16
// bytes holding n big-endian, whose hex is n zero padded to 32 digits. The
// golden records that the stub was used.
//
// The module does not export diskRev or pageStateDirPath, so it is compiled from
// Local's own source with those two names appended: every answer still comes
// from Local's code, and the golden says which names were appended.

const fs = require('fs')
const os = require('os')
const path = require('path')
const crypto = require('crypto')
const Module = require('module')

const checkout = process.argv[2]
const outPath = process.argv[3]
if (!checkout || !outPath) {
  process.stderr.write('usage: node gen.cjs <hyperclay-local checkout> <out.json>\n')
  process.exit(1)
}

let generationCalls = 0
const randomBytes = crypto.randomBytes
crypto.randomBytes = function (size) {
  if (size !== 16) return randomBytes.call(crypto, size)
  generationCalls += 1
  const buf = Buffer.alloc(16)
  buf.writeUInt32BE(generationCalls, 12)
  return buf
}

function loadStore() {
  const file = path.join(checkout, 'src', 'main', 'page-state-store.js')
  const instance = new Module(file, null)
  instance.filename = file
  instance.paths = Module._nodeModulePaths(path.dirname(file))
  const source = fs.readFileSync(file, 'utf8') + '\nmodule.exports.diskRev = diskRev;\nmodule.exports.pageStateDirPath = pageStateDirPath;\n'
  instance._compile(source, file)
  return instance.exports
}

const checkoutRequire = Module.createRequire(path.join(checkout, 'package.json'))
const rooms = checkoutRequire('@panphora/clayjs/server/rooms')
const store = loadStore()

const templateDir = path.join(__dirname, '..', '..', '..', 'pageadmission', 'testdata')
const read = (name) => fs.readFileSync(path.join(templateDir, name), 'utf8')

const ypage = read('ypage-template.html')
const untagged = read('untagged-page-template.html')
const pageTag = /<script type="application\/json" merge="page" clay="live">[\s\S]*?<\/script>/.exec(ypage)[0]

const revA = '1111111111111111'
const revB = '2222222222222222'
const revC = '3333333333333333'
const revD = '4444444444444444'
const revE = '5555555555555555'

const article = (rev) => '<article class="page" clay="freeze" data-page="1" data-rev="' + rev + '"></article>'
const page = (lines) => ['<!doctype html>', '<html>', '<body>', ...lines, '</body>', '</html>', ''].join('\n')
const tagged = (rev) => page(['  ' + pageTag, article(rev)])
const bare = (rev) => page([article(rev)])

const storedA = rooms.splitPage(tagged(revA)).file
const stateA = rooms.splitPage(tagged(revA)).state
const stateB = rooms.splitPage(tagged(revB)).state

const scenarios = []
const scenario = (name, steps) => scenarios.push({ name, steps })

function recordEntries(base) {
  const entries = []
  const walk = (dir) => {
    for (const entry of fs.readdirSync(dir, { withFileTypes: true })) {
      const full = path.join(dir, entry.name)
      const rel = path.relative(base, full).split(path.sep).join('/')
      if (entry.isDirectory()) {
        entries.push({ path: rel, kind: 'dir' })
        walk(full)
      } else {
        entries.push({ path: rel, kind: 'file', text: fs.readFileSync(full, 'utf8') })
      }
    }
  }
  walk(base)
  return entries.sort((a, b) => (a.path < b.path ? -1 : a.path > b.path ? 1 : 0))
}

function writeFile(base, rel, text) {
  const full = path.join(base, rel)
  fs.mkdirSync(path.dirname(full), { recursive: true })
  fs.writeFileSync(full, text)
}

const relPath = (root, target) => path.relative(root, target).split(path.sep).join('/')
const revOrUndefined = (step) => (step.rev === undefined ? undefined : step.rev)

async function runStep(root, base, step) {
  const dir = step.base === undefined ? base : step.base
  switch (step.kind) {
    case 'writeFile':
      writeFile(base, step.path, step.text)
      return null
    case 'mkdir':
      fs.mkdirSync(path.join(base, step.path), { recursive: true })
      return null
    case 'storePageState': {
      const file = await store.storePageState(base, step.name, step.html)
      writeFile(base, step.name, file)
      return { file }
    }
    case 'readPageState':
      return store.readPageState(base, step.name, { rev: revOrUndefined(step), exact: !!step.exact })
    case 'readPageRecord':
      return store.readPageRecord(base, step.name, revOrUndefined(step))
    case 'writePageState':
      await store.writePageState(base, step.name, step.state)
      return null
    case 'pageStatePath':
      return { path: relPath(root, store.pageStatePath(base, step.name)) }
    case 'pageStateDirPath':
      return { path: relPath(root, store.pageStateDirPath(base, step.name)) }
    case 'dropPageState':
      await store.dropPageState(base, step.name)
      return null
    case 'dropPageStateDir':
      await store.dropPageStateDir(base, step.name)
      return null
    case 'movePageState':
      await store.movePageState(base, step.from, step.to)
      return null
    case 'movePageStateDir':
      await store.movePageStateDir(base, step.from, step.to)
      return null
    case 'logicalDocument':
      return { document: await store.logicalDocument(dir, step.name, step.stored) }
    case 'logicalBytes':
      return { bytes: (await store.logicalBytes(dir, step.name, Buffer.from(step.text, 'utf8'))).toString('utf8') }
    case 'diskRev':
      return { rev: await store.diskRev(base, step.name) }
  }
  throw new Error('unknown step ' + step.kind)
}

async function runScenario(entry) {
  const root = fs.mkdtempSync(path.join(os.tmpdir(), 'pagestate-golden-'))
  const base = path.join(root, 'root')
  fs.mkdirSync(base)
  generationCalls = 0
  const results = []
  const entries = []
  for (const step of entry.steps) {
    let result
    try {
      result = await runStep(root, base, step)
    } catch (err) {
      result = { error: err && err.message ? err.message : String(err) }
    }
    results.push(result === undefined ? null : result)
    entries.push(recordEntries(base))
  }
  fs.rmSync(root, { recursive: true, force: true })
  return { name: entry.name, steps: entry.steps, results, entries }
}

scenario('store a page with a tag', [
  { kind: 'storePageState', name: 'notes.html', html: tagged(revA) }
])

scenario('store again with a new data-rev', [
  { kind: 'storePageState', name: 'notes.html', html: tagged(revA) },
  { kind: 'storePageState', name: 'notes.html', html: tagged(revB) }
])

scenario('store after the file was replaced from outside', [
  { kind: 'storePageState', name: 'notes.html', html: tagged(revA) },
  { kind: 'writeFile', path: 'notes.html', text: bare(revC) },
  { kind: 'storePageState', name: 'notes.html', html: tagged(revD) }
])

scenario('read page state by revision', [
  { kind: 'storePageState', name: 'notes.html', html: tagged(revA) },
  { kind: 'storePageState', name: 'notes.html', html: tagged(revB) },
  { kind: 'readPageState', name: 'notes.html' },
  { kind: 'readPageState', name: 'notes.html', rev: revA },
  { kind: 'readPageState', name: 'notes.html', rev: revB },
  { kind: 'readPageState', name: 'notes.html', rev: revE },
  { kind: 'readPageState', name: 'notes.html', exact: true },
  { kind: 'readPageState', name: 'notes.html', rev: revA, exact: true },
  { kind: 'readPageState', name: 'notes.html', rev: revB, exact: true },
  { kind: 'readPageState', name: 'notes.html', rev: revE, exact: true },
  { kind: 'readPageRecord', name: 'notes.html', rev: revA },
  { kind: 'readPageRecord', name: 'notes.html', rev: revB },
  { kind: 'readPageRecord', name: 'notes.html' }
])

scenario('logicalDocument composes the tag back', [
  { kind: 'storePageState', name: 'notes.html', html: tagged(revA) },
  { kind: 'logicalDocument', name: 'notes.html', stored: storedA },
  { kind: 'logicalDocument', name: 'notes.html', stored: tagged(revA) },
  { kind: 'logicalBytes', name: 'notes.html', text: storedA },
  { kind: 'logicalDocument', name: 'other.html', stored: storedA },
  { kind: 'logicalBytes', name: 'other.html', text: storedA },
  { kind: 'logicalDocument', name: 'notes.html', stored: bare(revB) }
])

scenario('legacy sidecar adopted on read', [
  { kind: 'writeFile', path: 'notes.html', text: bare(revA) },
  { kind: 'writeFile', path: '.hyperclay/page-state/notes.json', text: JSON.stringify({ ...stateA, generation: 'aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa' }) },
  { kind: 'readPageState', name: 'notes.html', rev: revA },
  { kind: 'readPageRecord', name: 'notes.html', rev: revA }
])

scenario('legacy sidecar beside the new one', [
  { kind: 'writeFile', path: 'notes.html', text: bare(revB) },
  { kind: 'writeFile', path: '.hyperclay/page-state/notes.json', text: JSON.stringify(stateA) },
  { kind: 'writeFile', path: '.hyperclay/page-state/notes.html.json', text: JSON.stringify(stateB) },
  { kind: 'readPageState', name: 'notes.html', rev: revB },
  { kind: 'readPageState', name: 'notes.html', rev: revA }
])

scenario('legacy sidecar beside the other extension', [
  { kind: 'writeFile', path: 'notes.html', text: bare(revA) },
  { kind: 'writeFile', path: 'notes.htmlclay', text: bare(revA) },
  { kind: 'writeFile', path: '.hyperclay/page-state/notes.json', text: JSON.stringify(stateA) },
  { kind: 'readPageState', name: 'notes.html' }
])

scenario('legacy sidecar for htmlclay when no html document', [
  { kind: 'writeFile', path: 'notes.htmlclay', text: bare(revA) },
  { kind: 'writeFile', path: '.hyperclay/page-state/notes.json', text: JSON.stringify(stateA) },
  { kind: 'readPageState', name: 'notes.htmlclay', rev: revA }
])

scenario('corrupt sidecar', [
  { kind: 'writeFile', path: 'notes.html', text: bare(revA) },
  { kind: 'writeFile', path: '.hyperclay/page-state/notes.html.json', text: 'not json' },
  { kind: 'readPageState', name: 'notes.html' },
  { kind: 'writeFile', path: '.hyperclay/page-state/notes.html.json', text: '{"v":2,"rev":"' + revA + '","tag":"T"}' },
  { kind: 'readPageState', name: 'notes.html' },
  { kind: 'writeFile', path: '.hyperclay/page-state/notes.html.json', text: '{"v":1,"rev":"' + revA + '","tag":"T","generation":"nope"}' },
  { kind: 'readPageState', name: 'notes.html' },
  { kind: 'readPageRecord', name: 'notes.html', rev: revA },
  { kind: 'writeFile', path: '.hyperclay/page-state/notes.html.json', text: '{"v":1,"rev":"' + revA + '","tag":"T","prev":{"rev":"' + revB + '"}}' },
  { kind: 'readPageState', name: 'notes.html', rev: revB },
  { kind: 'writeFile', path: '.hyperclay/page-state/notes.html.json', text: '{"v":1,"rev":"' + revA + '","tag":"T","prev":{"rev":"' + revB + '","tag":"P","generation":"00000000000000000000000000000009"}}' },
  { kind: 'readPageState', name: 'notes.html', rev: revB },
  { kind: 'storePageState', name: 'notes.html', html: tagged(revC) }
])

scenario('writePageState writes Local exact bytes', [
  { kind: 'writePageState', name: 'notes.html', state: { v: 1, rev: revA, tag: '<script merge="page" clay="live">&amp;\u00e9</script>' } },
  { kind: 'readPageRecord', name: 'notes.html', rev: revA },
  { kind: 'writePageState', name: 'notes.html', state: { v: 1, rev: revB, tag: 'second' } },
  { kind: 'readPageRecord', name: 'notes.html', rev: revA },
  { kind: 'readPageRecord', name: 'notes.html', rev: revB }
])

scenario('movePageState for a document', [
  { kind: 'storePageState', name: 'notes.html', html: tagged(revA) },
  { kind: 'movePageState', from: 'notes.html', to: 'sub/renamed.html' },
  { kind: 'readPageState', name: 'sub/renamed.html', rev: revA },
  { kind: 'readPageState', name: 'notes.html' },
  { kind: 'movePageState', from: 'missing.html', to: 'elsewhere.html' }
])

scenario('movePageStateDir for a folder', [
  { kind: 'storePageState', name: 'sub/one.html', html: tagged(revA) },
  { kind: 'storePageState', name: 'sub/two.html', html: tagged(revB) },
  { kind: 'movePageStateDir', from: 'sub', to: 'renamed' },
  { kind: 'readPageState', name: 'renamed/two.html', rev: revB },
  { kind: 'movePageStateDir', from: 'missing', to: 'elsewhere' }
])

scenario('dropPageState and dropPageStateDir', [
  { kind: 'storePageState', name: 'notes.html', html: tagged(revA) },
  { kind: 'dropPageState', name: 'notes.html' },
  { kind: 'readPageState', name: 'notes.html' },
  { kind: 'dropPageState', name: 'notes.html' },
  { kind: 'storePageState', name: 'sub/one.html', html: tagged(revA) },
  { kind: 'storePageState', name: 'sub/two.html', html: tagged(revB) },
  { kind: 'dropPageStateDir', name: 'sub' },
  { kind: 'dropPageStateDir', name: 'missing' }
])

scenario('subfolder and non-ascii name', [
  { kind: 'pageStatePath', name: 'folder/na\u00efve notes.html' },
  { kind: 'pageStateDirPath', name: 'folder/na\u00efve notes' },
  { kind: 'storePageState', name: 'folder/na\u00efve notes.html', html: tagged(revA) },
  { kind: 'readPageState', name: 'folder/na\u00efve notes.html', rev: revA },
  { kind: 'pageStatePath', name: 'notes.html' },
  { kind: 'pageStateDirPath', name: '' }
])

scenario('a name that tries to escape', [
  { kind: 'pageStatePath', name: '../x.html' },
  { kind: 'pageStateDirPath', name: '../x' },
  { kind: 'pageStatePath', name: 'notes.html' },
  { kind: 'writePageState', name: '../x.html', state: { v: 1, rev: revA, tag: 't' } },
  { kind: 'dropPageState', name: '../x.html' },
  { kind: 'movePageState', from: 'notes.html', to: '../x.html' },
  { kind: 'readPageState', name: '../x.html' }
])

scenario('diskRev for a file with and without an article', [
  { kind: 'writeFile', path: 'one.html', text: bare(revA) },
  { kind: 'diskRev', name: 'one.html' },
  { kind: 'writeFile', path: 'plain.html', text: page(['<p>nothing</p>']) },
  { kind: 'diskRev', name: 'plain.html' },
  { kind: 'diskRev', name: 'missing.html' },
  { kind: 'writeFile', path: 'two.html', text: page([article(revA), article(revB)]) },
  { kind: 'diskRev', name: 'two.html' },
  { kind: 'writeFile', path: 'untagged.html', text: untagged },
  { kind: 'diskRev', name: 'untagged.html' }
])

scenario('store is a no-op without a tag', [
  { kind: 'storePageState', name: 'plain.html', html: bare(revA) },
  { kind: 'storePageState', name: 'no-rev.html', html: page([pageTag]) },
  { kind: 'storePageState', name: 'untagged.html', html: untagged }
])

scenario('logical document without a base', [
  { kind: 'logicalDocument', base: '', name: 'notes.html', stored: bare(revA) },
  { kind: 'logicalBytes', base: '', name: 'notes.html', text: bare(revA) },
  { kind: 'logicalDocument', base: '', name: '', stored: bare(revA) }
])

const revCases = [
  { name: 'one article', input: bare(revA) },
  { name: 'one article with no data-rev', input: page(['<article data-page="1"></article>']) },
  { name: 'two articles', input: page([article(revA), article(revB)]) },
  { name: 'no article', input: page(['<p>nothing</p>']) },
  {
    name: 'article inside template beside a real one',
    input: page(['<template><article data-page="1" data-rev="' + revB + '"></article></template>', article(revA)])
  },
  { name: 'article inside noscript beside a real one', input: page(['<noscript><article data-page="1"></article></noscript>', article(revA)]) },
  { name: 'ypage template', input: ypage },
  { name: 'untagged template', input: untagged }
]

async function main() {
  const recorded = []
  for (const entry of scenarios) recorded.push(await runScenario(entry))

  const golden = {
    generatedWith: {
      module: 'src/main/page-state-store.js',
      appendedExports: ['diskRev', 'pageStateDirPath'],
      generationStub: 'counter-hex',
      rooms: '@panphora/clayjs/server/rooms'
    },
    scenarios: recorded,
    revs: revCases.map((entry) => ({ name: entry.name, input: entry.input, rev: rooms.pageRev(entry.input) }))
  }
  fs.writeFileSync(outPath, JSON.stringify(golden, null, 2) + '\n')
}

main().catch((err) => {
  process.stderr.write(String(err && err.stack ? err.stack : err) + '\n')
  process.exit(1)
})
