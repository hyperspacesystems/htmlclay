'use strict'

// Golden answers for the Go port of Local's static catch-all, its document
// serving and its folder listing, recorded from Local's real HTTP server.
//
//   node gen.cjs <hyperclay-local checkout> <out.json>
//
// The sample root beside this file is copied to a fresh temporary directory and
// every file's and folder's mtime is pinned, so a stamp (an ETag of a file, a
// Last-Modified) is reproducible. Local's own fixture boots on that copy, the
// cases in requests.json are sent with node's http module (never fetch: undici
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
const outPath = process.argv[3]
if (!checkout || !outPath) {
  process.stderr.write('usage: node gen.cjs <hyperclay-local checkout> <out.json>\n')
  process.exit(1)
}

const MTIME = new Date('2020-01-02T03:04:05Z')
const DROP = new Set(['date', 'connection', 'keep-alive', 'transfer-encoding'])
const source = path.join(__dirname, 'root')
const requests = JSON.parse(fs.readFileSync(path.join(__dirname, 'requests.json'), 'utf8'))

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

function request(port, testCase) {
  return new Promise((resolve, reject) => {
    const headers = Object.assign({ Host: '127.0.0.1:' + port }, testCase.headers)
    const req = http.request({ host: '127.0.0.1', port, path: testCase.path, method: testCase.method, headers }, (res) => {
      const chunks = []
      res.on('data', (chunk) => chunks.push(chunk))
      res.on('end', () => resolve({
        name: testCase.name,
        request: {
          method: testCase.method,
          path: testCase.path,
          headers: testCase.headers,
          ...(testCase.ifNoneMatchFrom ? { ifNoneMatchFrom: testCase.ifNoneMatchFrom } : {}),
        },
        status: res.statusCode,
        headers: normalizeHeaders(res.headers, port),
        body: normalizeBody(Buffer.concat(chunks), port),
      }))
    })
    req.on('error', reject)
    req.end()
  })
}

const wait = (ms) => new Promise((resolve) => setTimeout(resolve, ms))

async function main() {
  const temp = fs.mkdtempSync(path.join(os.tmpdir(), 'serve-golden-'))
  const root = path.join(temp, 'root-a')
  fs.cpSync(source, root, { recursive: true })
  pin(root)

  const scratch = path.join(temp, 'scratch')
  fs.mkdirSync(scratch)
  const manifestPath = path.join(scratch, 'manifest.json')
  const child = spawn(process.execPath, [
    path.join(checkout, 'tests', 'helpers', 'host-http-fixture.cjs'),
    '--manifest', manifestPath,
    '--source', checkout,
    '--root', 'root-a=' + root,
  ], { cwd: checkout, stdio: ['ignore', 'pipe', 'pipe'] })
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

  const answers = []
  try {
    for (const testCase of requests) {
      const resolved = Object.assign({}, testCase)
      if (testCase.ifNoneMatchFrom) {
        const answered = answers.find((answer) => answer.name === testCase.ifNoneMatchFrom)
        if (!answered || !answered.headers.etag) {
          throw new Error('no etag recorded for ' + testCase.ifNoneMatchFrom)
        }
        resolved.headers = Object.assign({}, testCase.headers, { 'If-None-Match': answered.headers.etag })
      }
      answers.push(await request(Number(port), resolved))
    }
  } catch (error) {
    child.kill('SIGKILL')
    process.stderr.write(error.stack + '\n' + log + '\n')
    process.exit(1)
  }

  const files = fileList(root)

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

  fs.writeFileSync(outPath, JSON.stringify({ cases: answers, files }, null, 2) + '\n')
  process.stdout.write('wrote ' + answers.length + ' cases and ' + files.length + ' files to ' + outPath + '\n')
}

main().catch((error) => {
  process.stderr.write(error.stack + '\n')
  process.exit(1)
})
