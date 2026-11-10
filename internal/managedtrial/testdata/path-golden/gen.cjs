'use strict'

const fs = require('fs')
const os = require('os')
const path = require('path')

const checkout = process.argv[2]
const out = process.argv[3]
if (!checkout || !out) {
  console.error('usage: node gen.cjs <hyperclay-local checkout> <out.json>')
  process.exit(1)
}

const {
  PathError,
  decodeOnce,
  validateSegments,
  createConsentRegistry,
  resolveReadPath,
  resolveWritePath,
  resolveRequestRead
} = require(path.join(checkout, 'src', 'main', 'utils', 'path-resolver.js'))

const tmp = fs.realpathSync(fs.mkdtempSync(path.join(os.tmpdir(), 'path-golden-')))
const root = path.join(tmp, 'root')
const sibling = path.join(tmp, 'sibling')

function writeFile(target, body) {
  fs.mkdirSync(path.dirname(target), { recursive: true })
  fs.writeFileSync(target, body)
}

fs.mkdirSync(root)
fs.mkdirSync(sibling)

writeFile(path.join(root, 'welcome.html'), '<h1>welcome</h1>\n')
writeFile(path.join(root, 'notes.txt'), 'notes\n')
writeFile(path.join(root, '.hidden'), 'dotfile\n')
writeFile(path.join(root, 'sub', 'report.txt'), 'report\n')
writeFile(path.join(root, 'Folder With Space', 'index.html'), '<p>spaced</p>\n')
writeFile(path.join(root, 'café', 'menu.html'), '<p>menu</p>\n')
writeFile(path.join(root, 'sites-versions', 'welcome.html'), '<p>old</p>\n')
writeFile(path.join(root, 'node_modules', 'pkg', 'index.js'), 'module.exports = 1\n')
writeFile(path.join(root, 'target', 'inside.txt'), 'inside\n')
writeFile(path.join(sibling, 'outside.txt'), 'outside\n')

fs.symlinkSync('target', path.join(root, 'in-link'), 'dir')
fs.symlinkSync('../sibling', path.join(root, 'out-link'), 'dir')
fs.symlinkSync('loop-link', path.join(root, 'loop-link'))
fs.symlinkSync('does-not-exist.txt', path.join(root, 'dangling-link'))

const baseReal = fs.realpathSync(root)
const rel = (target) => path.relative(baseReal, target)

const logLines = []
const registry = createConsentRegistry(root, (line) => logLines.push(line))

const inputs = [
  '',
  'welcome.html',
  'sub',
  'sub/report.txt',
  'sub//report.txt',
  'sub/report.txt/',
  'Folder%20With%20Space/index.html',
  'caf%C3%A9/menu.html',
  'café/menu.html',
  'bad%zz',
  'bad%',
  'bad%2',
  'sub%2Freport.txt',
  '%2e%2e/notes.txt',
  '../notes.txt',
  'sub/../../notes.txt',
  'a%00b',
  '.hidden',
  'sites-versions/welcome.html',
  'sites-versions',
  'node_modules/pkg/index.js',
  'notes\\with\\backslash.txt',
  '/welcome.html',
  '%2Fwelcome.html',
  'welcome.html?x=1',
  'x'.repeat(255),
  'x'.repeat(256),
  'in-link',
  'in-link/inside.txt',
  'in-link/new-file.txt',
  'out-link',
  'out-link/outside.txt',
  'out-link/new-file.txt',
  'late-link',
  'late-link/outside.txt',
  'late-link/new-file.txt',
  'loop-link',
  'loop-link/new-file.txt',
  'dangling-link',
  'dangling-link/x.txt',
  'dangling-link/new-file.txt',
  'new-file.txt',
  'missing-folder/new-file.txt',
  'sub/new-file.txt',
  'target/../notes.txt'
]

function errorShape(error) {
  if (error instanceof PathError) {
    return { status: error.status, message: error.message }
  }
  return { thrown: error && error.name ? error.name : 'Error' }
}

async function attempt(fn) {
  try {
    return await fn()
  } catch (error) {
    return errorShape(error)
  }
}

async function main() {
  await registry.ready()

  const late = path.join(tmp, 'late')
  fs.mkdirSync(late)
  writeFile(path.join(late, 'outside.txt'), 'late\n')
  fs.symlinkSync('../late', path.join(root, 'late-link'), 'dir')

  const cases = []
  for (const input of inputs) {
    const entry = { input }
    entry.decodeOnce = await attempt(() => decodeOnce(input))
    entry.validateSegments = await attempt(() => validateSegments(input))
    entry.resolveRequestRead = await attempt(async () => {
      const result = await resolveRequestRead(registry, input)
      return { relPath: result.relPath, realPath: rel(result.realPath) }
    })
    entry.resolveReadPath = await attempt(async () => rel(await resolveReadPath(registry, input)))
    entry.resolveWritePath = await attempt(async () => rel(await resolveWritePath(registry, input)))
    cases.push(entry)
  }

  const sanitize = (line) => line.split(tmp).join('<tmp>')
  const golden = {
    inputs: cases,
    consentedRoots: registry.consentedRoots().map(rel),
    log: logLines.map(sanitize)
  }

  fs.writeFileSync(out, JSON.stringify(golden, null, 2) + '\n')
}

main()
  .catch((error) => {
    console.error(error)
    process.exit(1)
  })
  .finally(() => {
    fs.rmSync(tmp, { recursive: true, force: true })
  })
