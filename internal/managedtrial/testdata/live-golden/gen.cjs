'use strict'

// Golden answers for the Go port of Local's live-sync hub, its replay store and
// its stream routes, recorded from Local's real server.
//
//   node gen.cjs <hyperclay-local checkout> <case dir> <out.json>
//
// The case's root/ is copied to a fresh temporary directory and every file's and
// folder's mtime is pinned, exactly as http-golden.cjs does, so a stamp is
// reproducible. Local's own fixture boots on that copy and the case's
// script.json steps are run against it.
//
// The HTTP recorder cannot hold a stream open, so this one can: `open` starts a
// stream and keeps reading it, `close` ends one, and every frame it received is
// recorded as it arrived, split on blank lines. Requests are recorded the way
// requests.json cases are, which is what a `request` step is.
//
// The trial host has no rooms, so the fixture runs with its local authority
// dropped: the paths a request takes then are the ones Local takes for a
// legacy createApp(dir), and the ones the Go host answers as.

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
  process.stderr.write('usage: node gen.cjs <hyperclay-local checkout> <case dir> <out.json>\n')
  process.exit(1)
}

const MTIME = new Date('2020-01-02T03:04:05Z')
const PIN_CLOCK = Date.UTC(2026, 10, 1, 6, 0, 0, 0)
const DROP = new Set(['date', 'connection', 'keep-alive', 'transfer-encoding'])
const SETTLE_MS = 300

const source = path.join(caseDir, 'root')
const script = JSON.parse(fs.readFileSync(path.join(caseDir, 'script.json'), 'utf8'))

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

function swapPort(text, port) {
  return String(text).split(':' + port).join(':{PORT}')
}

function normalizeHeaders(headers, port) {
  const out = {}
  for (const name of Object.keys(headers).sort()) {
    const lower = name.toLowerCase()
    if (DROP.has(lower)) continue
    const value = Array.isArray(headers[name]) ? headers[name].join(', ') : String(headers[name])
    out[lower] = swapPort(value, port)
  }
  return out
}

function normalizeBody(buf, port) {
  const swapped = Buffer.from(buf.toString('latin1').split(':' + port).join(':{PORT}'), 'latin1')
  if (validUtf8(swapped)) return { text: swapped.toString('utf8') }
  return { base64: swapped.toString('base64') }
}

function recordHeaders(headers, port) {
  const out = {}
  for (const name of Object.keys(headers)) out[name] = swapPort(headers[name], port)
  return out
}

function answerJSON(answer) {
  if (answer.body.text !== undefined) return JSON.parse(answer.body.text)
  return JSON.parse(Buffer.from(answer.body.base64, 'base64').toString('utf8'))
}

function field(value, dotted) {
  let current = value
  for (const part of dotted.split('.')) {
    if (current === null || typeof current !== 'object') return undefined
    current = current[part]
  }
  return current
}

const wait = (ms) => new Promise((resolve) => setTimeout(resolve, ms))

// substitute fills a step's {PORT} with the root's port and its {FRAME:<stream>:<index>}
// with the id line of a frame a stream already received, so a link-back Resume is
// recorded exactly as a reconnecting EventSource would send it.
function substitute(value, port, streams) {
  let text = String(value).split('{PORT}').join(String(port))
  text = text.replace(/\{FRAME:([^:}]+):(\d+)\}/g, (whole, name, index) => {
    const stream = streams.find((candidate) => candidate.name === name)
    if (!stream) throw new Error('no stream named ' + name)
    const frame = stream.frames[Number(index)]
    if (frame === undefined) throw new Error('stream ' + name + ' has no frame ' + index)
    const match = frame.match(/^id: (\d+)$/m)
    if (!match) throw new Error('stream ' + name + ' frame ' + index + ' carries no id')
    return match[1]
  })
  return text
}

// noAuthority is preloaded into the fixture: the trial has no rooms, so the
// recording server runs without a local authority, which is the legacy
// createApp(dir) path this host ports.
const NO_AUTHORITY = `'use strict'
const Module = require('module')
const load = Module._load
const PATCHED = Symbol('patched')
const drop = (options) => Object.assign({}, options, { localAuthority: null })
Module._load = function (request, parent, isMain) {
  const exported = load.apply(this, arguments)
  if (typeof request !== 'string' || !exported) return exported
  if (/src[\\\\/]main[\\\\/]root-servers(\\\\.js)?$/.test(request) && exported.RootServerPool && !exported[PATCHED]) {
    const Original = exported.RootServerPool
    Object.defineProperty(exported, 'RootServerPool', {
      value: class WithoutAuthority extends Original { constructor(options, ...rest) { super(drop(options), ...rest) } },
      writable: true, configurable: true,
    })
    exported[PATCHED] = true
  }
  if (/src[\\\\/]main[\\\\/]root-observer(\\\\.js)?$/.test(request) && exported.RootObserver && !exported[PATCHED]) {
    const Original = exported.RootObserver
    Object.defineProperty(exported, 'RootObserver', {
      value: class WithoutAuthority extends Original { constructor(root, options, ...rest) { super(root, drop(options), ...rest) } },
      writable: true, configurable: true,
    })
    exported[PATCHED] = true
  }
  return exported
}
`

async function main() {
  const temp = fs.mkdtempSync(path.join(os.tmpdir(), 'live-golden-'))
  const root = path.join(temp, 'root-a')
  fs.cpSync(source, root, { recursive: true })
  pin(root)

  const pinClock = path.resolve(caseDir, 'pin-clock.cjs')
  const generationLog = path.join(temp, 'generations.log')
  const noAuthority = path.join(temp, 'no-authority.cjs')
  fs.writeFileSync(noAuthority, NO_AUTHORITY)
  const preloads = ['--require ' + noAuthority]
  if (fs.existsSync(pinClock)) preloads.unshift('--require ' + pinClock)
  const env = {
    ...process.env,
    GOLDEN_PIN_CLOCK: String(PIN_CLOCK),
    GOLDEN_PIN_GENERATIONS: generationLog,
    NODE_OPTIONS: `${process.env.NODE_OPTIONS || ''} ${preloads.join(' ')}`.trim(),
  }

  const scratch = path.join(temp, 'scratch')
  fs.mkdirSync(scratch)
  const manifestPath = path.join(scratch, 'manifest.json')
  const child = spawn(process.execPath, [
    path.join(checkout, 'tests', 'helpers', 'host-http-fixture.cjs'),
    '--manifest', manifestPath,
    '--source', checkout,
    '--root', 'root-a=' + root,
  ], { cwd: checkout, stdio: ['ignore', 'pipe', 'pipe'], env })
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
    process.stderr.write('the fixture names no root-a\n' + log + '\n')
    process.exit(1)
  }
  const port = Number(new URL(entry.url).port)

  const streams = []
  const answers = []
  const open = []

  const sendRequest = (step, headers, body) => new Promise((resolve, reject) => {
    const req = http.request({
      host: '127.0.0.1', port, path: step.path, method: step.method,
      headers: Object.assign({ Host: '127.0.0.1:' + port }, headers),
    }, (res) => {
      const chunks = []
      res.on('data', (chunk) => chunks.push(chunk))
      res.on('end', () => resolve({
        name: step.name,
        request: {
          method: step.method,
          path: swapPort(step.path, port),
          headers: recordHeaders(headers, port),
          ...(step.ifNoneMatchFrom ? { ifNoneMatchFrom: step.ifNoneMatchFrom } : {}),
          ...(step.ifMatchFrom ? { ifMatchFrom: step.ifMatchFrom } : {}),
          ...(step.fromCase ? { fromCase: step.fromCase } : {}),
          ...(step.body !== undefined ? { body: step.body } : {}),
          ...(step.bodyFile ? { bodyFile: step.bodyFile } : {}),
        },
        status: res.statusCode,
        headers: normalizeHeaders(res.headers, port),
        body: normalizeBody(Buffer.concat(chunks), port),
      }))
    })
    req.on('error', reject)
    req.end(body)
  })

  try {
    for (const step of script) {
      if (step.op === 'wait') {
        await wait(step.ms)
        continue
      }
      if (step.op === 'open') {
        const headers = {}
        for (const name of Object.keys(step.headers || {})) headers[name] = substitute(step.headers[name], port, streams)
        const target = substitute(step.path, port, streams)
        const stream = {
          name: step.name,
          request: { path: swapPort(target, port), headers: recordHeaders(headers, port) },
          status: 0,
          headers: {},
          frames: [],
          buffer: '',
        }
        streams.push(stream)
        const req = http.request({
          host: '127.0.0.1', port, path: target, method: 'GET',
          headers: Object.assign({ Host: '127.0.0.1:' + port }, headers),
        }, (res) => {
          stream.status = res.statusCode
          stream.headers = normalizeHeaders(res.headers, port)
          res.setEncoding('utf8')
          res.on('data', (chunk) => {
            stream.buffer += chunk
            let at
            while ((at = stream.buffer.indexOf('\n\n')) >= 0) {
              stream.frames.push(swapPort(stream.buffer.slice(0, at), port))
              stream.buffer = stream.buffer.slice(at + 2)
            }
          })
          res.on('error', () => {})
        })
        req.on('error', () => {})
        req.end()
        open.push({ name: step.name, req })
        continue
      }
      if (step.op === 'close') {
        const at = open.findIndex((candidate) => candidate.name === step.name)
        if (at < 0) throw new Error('no open stream named ' + step.name)
        open[at].req.destroy()
        open.splice(at, 1)
        continue
      }
      if (step.op === 'request') {
        const headers = {}
        for (const name of Object.keys(step.headers || {})) headers[name] = substitute(step.headers[name], port, streams)
        if (step.ifNoneMatchFrom) {
          const answered = answers.find((answer) => answer.name === step.ifNoneMatchFrom)
          if (!answered || !answered.headers.etag) throw new Error('no etag recorded for ' + step.ifNoneMatchFrom)
          headers['If-None-Match'] = answered.headers.etag
        }
        if (step.ifMatchFrom) {
          const answered = answers.find((answer) => answer.name === step.ifMatchFrom.case)
          if (!answered) throw new Error('no answer recorded for ' + step.ifMatchFrom.case)
          const value = field(answerJSON(answered), step.ifMatchFrom.json)
          if (typeof value !== 'string' || value === '') throw new Error('no ' + step.ifMatchFrom.json + ' in the answer for ' + step.ifMatchFrom.case)
          headers['If-Match'] = value
        }
        let body
        if (step.bodyFile) body = fs.readFileSync(path.join(caseDir, step.bodyFile))
        else if (step.bodyRepeat) body = step.bodyRepeat.unit.repeat(step.bodyRepeat.times)
        else if (step.body !== undefined) body = step.body
        let requestPath = substitute(step.path, port, streams)
        if (step.fromCase) {
          const answered = answers.find((answer) => answer.name === step.fromCase.case)
          if (!answered) throw new Error('no answer recorded for ' + step.fromCase.case)
          const value = field(answerJSON(answered), step.fromCase.json)
          if (typeof value !== 'string' || value === '') throw new Error('no ' + step.fromCase.json + ' in the answer for ' + step.fromCase.case)
          requestPath = requestPath.split('{FROM}').join(value)
        }
        answers.push(await sendRequest({ ...step, path: requestPath }, headers, body))
        continue
      }
      throw new Error('unknown step op: ' + step.op)
    }

    await wait(SETTLE_MS)
  } catch (error) {
    child.kill('SIGKILL')
    process.stderr.write(error.stack + '\n' + log + '\n')
    process.exit(1)
  }

  for (const stream of open) stream.req.destroy()
  await wait(50)
  const files = fileList(root)

  const pinned = { clock: PIN_CLOCK, pages: [] }
  if (fs.existsSync(generationLog)) {
    pinned.pages = fs.readFileSync(generationLog, 'utf8').split('\n').filter(Boolean)
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

  fs.writeFileSync(outPath, JSON.stringify({ streams, cases: answers, files, ...pinned }, null, 2) + '\n')
  process.stdout.write('wrote ' + streams.length + ' streams, ' + answers.length + ' cases and ' + files.length + ' files to ' + outPath + '\n')
}

main().catch((error) => {
  process.stderr.write(error.stack + '\n')
  process.exit(1)
})
