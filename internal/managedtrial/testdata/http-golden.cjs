'use strict'

// Golden answers for the Go ports of Local's HTTP routes, recorded from Local's
// real server.
//
//   node http-golden.cjs <hyperclay-local checkout> <case dir> <out.json>
//
// The case's root/ is copied to a fresh temporary directory and every file's and
// folder's mtime is pinned, so a stamp (an ETag of a file, a Last-Modified) is
// reproducible. Local's own fixture boots on that copy, the cases in the case
// dir's requests.json are sent with node's http module (never fetch: undici
// rewrites Sec-Fetch-Mode), and the answers are recorded with the port replaced
// by {PORT}. The copy's file list is recorded afterwards so a GET that wrote
// something would show.

const fs = require('fs')
const http = require('http')
const os = require('os')
const path = require('path')
const crypto = require('crypto')
const { spawn } = require('child_process')

const checkout = process.argv[2]
const caseDir = process.argv[3]
const outPath = process.argv[4]
if (!checkout || !caseDir || !outPath) {
  process.stderr.write('usage: node http-golden.cjs <hyperclay-local checkout> <case dir> <out.json>\n')
  process.exit(1)
}

const MTIME = new Date('2020-01-02T03:04:05Z')
// PIN_CLOCK is the instant a case that pins the clock starts every write at. A
// case pins it with a pin-clock.js beside its requests.json, and the value is
// recorded so the Go side can reproduce the same version names.
const PIN_CLOCK = Date.UTC(2026, 10, 1, 6, 0, 0, 0)
const DROP = new Set(['date', 'connection', 'keep-alive', 'transfer-encoding'])
const source = path.join(caseDir, 'root')
const requests = JSON.parse(fs.readFileSync(path.join(caseDir, 'requests.json'), 'utf8'))

function walk(dir, rel, files, dirs) {
  for (const entry of fs.readdirSync(dir, { withFileTypes: true })) {
    const full = path.join(dir, entry.name)
    const childRel = rel ? rel + '/' + entry.name : entry.name
    if (entry.isDirectory()) {
      dirs.push({ full, rel: childRel })
      walk(full, childRel, files, dirs)
    } else {
      files.push({ full, rel: childRel })
    }
  }
}

function pin(root) {
  const files = []
  const dirs = []
  walk(root, '', files, dirs)
  for (const file of files) fs.utimesSync(file.full, MTIME, MTIME)
  dirs.sort((a, b) => b.rel.split('/').length - a.rel.split('/').length)
  for (const dir of dirs) fs.utimesSync(dir.full, MTIME, MTIME)
  fs.utimesSync(root, MTIME, MTIME)
}

function fileList(root) {
  const files = []
  walk(root, '', files, [])
  return files
    .filter((file) => !file.rel.endsWith('.tmp'))
    .sort((a, b) => (a.rel < b.rel ? -1 : a.rel > b.rel ? 1 : 0))
    .map((file) => ({ path: file.rel, sha256: crypto.createHash('sha256').update(fs.readFileSync(file.full)).digest('hex') }))
}

function validUtf8(buf) {
  return Buffer.from(buf.toString('utf8'), 'utf8').equals(buf)
}

function normalizeHeaders(headers, port) {
  const out = {}
  for (const name of Object.keys(headers).sort()) {
    const lower = name.toLowerCase()
    if (DROP.has(lower)) continue
    const value = Array.isArray(headers[name]) ? headers[name].join(', ') : String(headers[name])
    out[lower] = value.split(':' + port).join(':{PORT}')
  }
  return out
}

function normalizeBody(buf, port) {
  const swapped = Buffer.from(buf.toString('latin1').split(':' + port).join(':{PORT}'), 'latin1')
  if (validUtf8(swapped)) return { text: swapped.toString('utf8') }
  return { base64: swapped.toString('base64') }
}

// substitutePort fills a request header value's {PORT} with the root's port.
function substitutePort(value, port) {
  return String(value).split('{PORT}').join(String(port))
}

// clayjsVersion is the release the checkout's offline-runtime manifest selects,
// the version every {CLAYJS_VERSION} in a case's path stands for.
function clayjsVersion(out) {
  const manifest = JSON.parse(fs.readFileSync(path.join(out, 'src', 'main', 'offline-runtime', 'manifest.json'), 'utf8'))
  if (!manifest.selected) throw new Error('the checkout names no selected ClayJS release')
  return manifest.selected
}

function substituteVersion(value, version) {
  return String(value).split('{CLAYJS_VERSION}').join(version)
}

function answerJSON(answer) {
  if (answer.body.text !== undefined) return JSON.parse(answer.body.text)
  return JSON.parse(Buffer.from(answer.body.base64, 'base64').toString('utf8'))
}

// field reads one dotted path from a recorded JSON answer, the same way the Go
// side resolves a case's ifMatchFrom.
function field(value, dotted) {
  let current = value
  for (const part of dotted.split('.')) {
    if (current === null || typeof current !== 'object') return undefined
    current = current[part]
  }
  return current
}

// recordHeaders is the request as a golden records it: the port a case's
// {PORT} was filled with is put back, so the answer is reproducible across runs.
function recordHeaders(headers, port) {
  const out = {}
  for (const name of Object.keys(headers)) out[name] = String(headers[name]).split(':' + port).join(':{PORT}')
  return out
}

function request(port, testCase, headers, body) {
  return new Promise((resolve, reject) => {
    const sendHeaders = Object.assign({ Host: '127.0.0.1:' + port }, headers)
    const req = http.request({ host: '127.0.0.1', port, path: testCase.path, method: testCase.method, headers: sendHeaders }, (res) => {
      const chunks = []
      res.on('data', (chunk) => chunks.push(chunk))
      res.on('end', () => resolve({
        name: testCase.name,
        request: {
          method: testCase.method,
          path: testCase.path,
          headers: recordHeaders(headers, port),
          ...(testCase.ifNoneMatchFrom ? { ifNoneMatchFrom: testCase.ifNoneMatchFrom } : {}),
          ...(testCase.ifMatchFrom ? { ifMatchFrom: testCase.ifMatchFrom } : {}),
          ...(testCase.body !== undefined ? { body: testCase.body } : {}),
          ...(testCase.bodyFile ? { bodyFile: testCase.bodyFile } : {}),
        },
        status: res.statusCode,
        headers: normalizeHeaders(res.headers, port),
        body: normalizeBody(Buffer.concat(chunks), port),
      }))
    })
    req.on('error', reject)
    req.end(body)
  })
}

const wait = (ms) => new Promise((resolve) => setTimeout(resolve, ms))

async function main() {
  const temp = fs.mkdtempSync(path.join(os.tmpdir(), 'http-golden-'))
  const root = path.join(temp, 'root-a')
  fs.cpSync(source, root, { recursive: true })
  pin(root)

  // A case that pins the clock loads pin-clock.js into the fixture, so every
  // version name and page-state generation it writes is reproducible.
  const pinClock = path.resolve(caseDir, 'pin-clock.cjs')
  const generationLog = path.join(temp, 'generations.log')
  const pinClockEnv = fs.existsSync(pinClock)
    ? {
      ...process.env,
      GOLDEN_PIN_CLOCK: String(PIN_CLOCK),
      GOLDEN_PIN_GENERATIONS: generationLog,
      NODE_OPTIONS: `${process.env.NODE_OPTIONS || ''} --require ${pinClock}`.trim()
    }
    : process.env

  const scratch = path.join(temp, 'scratch')
  fs.mkdirSync(scratch)
  const manifestPath = path.join(scratch, 'manifest.json')
  const child = spawn(process.execPath, [
    path.join(checkout, 'tests', 'helpers', 'host-http-fixture.cjs'),
    '--manifest', manifestPath,
    '--source', checkout,
    '--root', 'root-a=' + root,
  ], {
    cwd: checkout,
    stdio: ['ignore', 'pipe', 'pipe'],
    env: pinClockEnv,
  })
  const exitPromise = new Promise((resolve) => child.once('exit', (code, signal) => resolve({ code, signal })))
  let log = ''
  child.stdout.setEncoding('utf8')
  child.stderr.setEncoding('utf8')
  child.stdout.on('data', (chunk) => { log += chunk })
  child.stderr.on('data', (chunk) => { log += chunk })

  let manifest = null
  const deadline = Date.now() + 30000
  while (Date.now() < deadline) {
    try {
      manifest = JSON.parse(fs.readFileSync(manifestPath, 'utf8'))
      break
    } catch (error) {
      log = log.slice(-4000)
    }
    if (child.exitCode !== null) break
    await wait(50)
  }
  if (!manifest) {
    child.kill('SIGKILL')
    process.stderr.write('the fixture wrote no manifest\n' + log + '\n')
    process.exit(1)
  }

  const entry = (manifest.roots || []).find((candidate) => candidate.id === 'root-a')
  if (!entry) {
    child.kill('SIGKILL')
    process.stderr.write('the fixture names no root-a\n')
    process.exit(1)
  }
  const port = new URL(entry.url).port

  const version = clayjsVersion(checkout)
  const answers = []
  try {
    for (const testCase of requests) {
      const path = substituteVersion(testCase.path, version)
      const headers = {}
      for (const name of Object.keys(testCase.headers || {})) {
        headers[name] = substituteVersion(substitutePort(testCase.headers[name], port), version)
      }
      if (testCase.ifNoneMatchFrom) {
        const answered = answers.find((answer) => answer.name === testCase.ifNoneMatchFrom)
        if (!answered || !answered.headers.etag) {
          throw new Error('no etag recorded for ' + testCase.ifNoneMatchFrom)
        }
        headers['If-None-Match'] = answered.headers.etag
      }
      if (testCase.ifMatchFrom) {
        const answered = answers.find((answer) => answer.name === testCase.ifMatchFrom.case)
        if (!answered) throw new Error('no answer recorded for ' + testCase.ifMatchFrom.case)
        const value = field(answerJSON(answered), testCase.ifMatchFrom.json)
        if (typeof value !== 'string' || value === '') {
          throw new Error('no ' + testCase.ifMatchFrom.json + ' in the answer for ' + testCase.ifMatchFrom.case)
        }
        headers['If-Match'] = value
      }
      let body
      if (testCase.bodyFile) body = fs.readFileSync(String(caseDir) + '/' + testCase.bodyFile)
      else if (testCase.body !== undefined) body = testCase.body
      answers.push(await request(Number(port), { ...testCase, path }, headers, body))
    }
  } catch (error) {
    child.kill('SIGKILL')
    process.stderr.write(error.stack + '\n' + log + '\n')
    process.exit(1)
  }

  // The data guard writes its record after the answer, so give it the moment it
  // needs to land before the tree is read.
  await wait(200)
  const files = fileList(root)

  // The clock and the page-state generations the recording consumed, so the Go
  // side can produce the same version names and the same sidecar bytes. Read
  // before the temp tree is removed.
  let pinned = {}
  if (fs.existsSync(pinClock)) {
    pinned = { clock: PIN_CLOCK, pages: [] }
    if (fs.existsSync(generationLog)) {
      pinned.pages = fs.readFileSync(generationLog, 'utf8').split('\n').filter(Boolean)
    }
  }

  const stop = manifest.control
  if (stop && stop.url && stop.token) {
    await new Promise((resolve) => {
      const req = http.request(stop.url + '/stop', {
        method: 'POST',
        headers: { authorization: 'Bearer ' + stop.token, 'content-type': 'application/json' },
      }, (res) => { res.resume(); res.on('end', resolve) })
      req.on('error', resolve)
      req.end('{}')
    })
  }
  if (child.exitCode === null) {
    child.kill('SIGTERM')
    const gone = await Promise.race([exitPromise, wait(5000).then(() => null)])
    if (!gone) {
      child.kill('SIGKILL')
      await exitPromise
    }
  }
  fs.rmSync(temp, { recursive: true, force: true })

  fs.writeFileSync(outPath, JSON.stringify({ cases: answers, files, ...pinned }, null, 2) + '\n')
  process.stdout.write('wrote ' + answers.length + ' cases and ' + files.length + ' files to ' + outPath + '\n')
}

main().catch((error) => {
  process.stderr.write(error.stack + '\n')
  process.exit(1)
})
