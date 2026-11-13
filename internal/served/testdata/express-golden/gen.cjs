'use strict'

const fs = require('fs')
const http = require('http')
const path = require('path')

const dir = process.argv[2]
const out = process.argv[3]
if (!dir || !out) {
  console.error('usage: node gen.cjs <hyperclay-local checkout> <out.json>')
  process.exit(1)
}

const express = require(path.join(dir, 'node_modules', 'express'))
const app = express()

const cookieOptions = { httpOnly: false, secure: false, sameSite: 'lax' }
const fixture = path.join(__dirname, 'fixture.txt')
const FIXED_MTIME = new Date('2020-01-02T03:04:05.000Z')
fs.utimesSync(fixture, FIXED_MTIME, FIXED_MTIME)

app.get('/text', (req, res) => res.send('hello world'))
app.get('/empty', (req, res) => res.send(''))
app.get('/buffer', (req, res) => res.send(Buffer.from([0, 1, 2, 254, 255])))
app.get('/json', (req, res) => res.json({
  html: '<b>&</b>',
  accent: 'é',
  emoji: '🎉',
  line: '\u2028',
  n: 1,
  ok: true,
  nested: { a: 'x' }
}))
app.get('/forbidden', (req, res) => res.status(403).send('Invalid Host header'))
app.get('/missing', (req, res) => res.status(404).send('File not found'))
app.get('/cookies', (req, res) => {
  res.cookie('isAdminOfCurrentResource', 'true', cookieOptions)
  res.cookie('isLoggedIn', 'true', cookieOptions)
  res.send('ok')
})
app.get('/file', (req, res) => res.sendFile(fixture))

app.use((err, req, res, next) => {
  const status = err.status || err.statusCode ||
    (err.code === 'ENOENT' || err.code === 'ENOTDIR' ? 404 : 500)
  if (res.headersSent) return next(err)
  const body = status === 404 ? 'File not found'
    : status === 403 ? 'Access denied'
    : status === 400 ? 'Bad request'
    : 'Internal server error'
  res.status(status).send(body)
})

function send(server, method, pathname, headers) {
  const hdrs = headers || {}
  return new Promise((resolve, reject) => {
    const req = http.request({
      host: '127.0.0.1',
      port: server.address().port,
      method,
      path: pathname,
      headers: hdrs
    }, (res) => {
      const chunks = []
      res.on('data', (chunk) => chunks.push(chunk))
      res.on('end', () => resolve({
        request: { method, path: pathname, headers: hdrs },
        status: res.statusCode,
        headers: res.headers,
        body: Buffer.concat(chunks)
      }))
    })
    req.on('error', reject)
    req.end()
  })
}

const DROPPED = new Set(['date', 'connection', 'keep-alive'])

function record(result) {
  const headers = {}
  Object.keys(result.headers).sort().forEach((name) => {
    if (DROPPED.has(name)) return
    const value = result.headers[name]
    headers[name] = Array.isArray(value) ? value.join(', ') : value
  })
  return {
    request: {
      method: result.request.method,
      path: result.request.path,
      headers: result.request.headers
    },
    status: result.status,
    headers,
    bodyBase64: result.body.toString('base64')
  }
}

async function main() {
  const server = await new Promise((resolve) => {
    const s = app.listen(0, '127.0.0.1', () => resolve(s))
  })
  try {
    const cases = []
    const run = async (method, pathname, headers) => {
      const result = await send(server, method, pathname, headers)
      cases.push(record(result))
      return result
    }

    const text = await run('GET', '/text')
    const textEtag = text.headers.etag
    const file = await run('GET', '/file')
    const fileEtag = file.headers.etag
    const lastModified = file.headers['last-modified']
    const afterMtime = new Date(FIXED_MTIME.getTime() + 1000).toUTCString()
    const beforeMtime = new Date(FIXED_MTIME.getTime() - 1000).toUTCString()

    await run('HEAD', '/text')
    await run('GET', '/text', { 'if-none-match': textEtag })
    await run('GET', '/text', { 'if-none-match': '"0000000-deadbeef"' })
    await run('GET', '/text', { 'if-none-match': '*' })
    await run('GET', '/text', { 'if-none-match': textEtag, 'cache-control': 'no-cache' })
    await run('GET', '/file', { 'if-none-match': fileEtag })
    await run('GET', '/file', { 'if-modified-since': afterMtime })
    await run('GET', '/file', { 'if-modified-since': beforeMtime })
    await run('GET', '/file', { 'range': 'bytes=0-4' })
    await run('GET', '/file', { 'range': 'bytes=-3' })
    await run('GET', '/file', { 'range': 'bytes=1000-2000' })
    await run('GET', '/file', { 'range': 'bytes=1000-2000', 'if-range': fileEtag })
    await run('GET', '/file', { 'range': 'bytes=1000-2000', 'if-range': '"0000000-deadbeef"' })
    await run('GET', '/file', { 'range': 'bytes=0-1,3-4' })
    await run('GET', '/file', { 'range': 'bytes=0-2', 'if-range': fileEtag })
    await run('GET', '/file', { 'range': 'bytes=0-2', 'if-range': '"0000000-deadbeef"' })
    await run('GET', '/file', { 'if-match': '"0000000-deadbeef"' })
    await run('HEAD', '/file')
    await run('GET', '/json')
    await run('GET', '/empty')
    await run('GET', '/buffer')
    await run('GET', '/forbidden')
    await run('GET', '/missing')
    await run('GET', '/cookies')
    await run('GET', '/file', { 'if-none-match': '*', 'if-modified-since': lastModified })

    fs.writeFileSync(out, JSON.stringify(cases, null, 2) + '\n')
  } finally {
    await new Promise((resolve) => server.close(resolve))
  }
}

main().catch((error) => {
  console.error(error)
  process.exit(1)
})
