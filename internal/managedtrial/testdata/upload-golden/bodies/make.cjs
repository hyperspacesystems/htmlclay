'use strict'

// Builds the committed multipart bodies the upload cases send.
//
//   node bodies/make.cjs
//
// Each body names the same fixed boundary the matching request's Content-Type
// names, so an upload is the same bytes on every recording. Only the bodies a
// case cannot spell as `body` in requests.json live here: a real PNG's bytes,
// an empty part, a body with no file part, and one that stops before its
// closing boundary.

const fs = require('fs')
const path = require('path')

const BOUNDARY = '----htmlclay-upload-golden'

// The smallest PNG: one transparent pixel.
const PNG = Buffer.from(
  'iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNkYPhfDwAChwGA60e6kgAAAABJRU5ErkJggg==',
  'base64'
)

function part(headers, body) {
  const lines = [`--${BOUNDARY}`]
  for (const [name, value] of Object.entries(headers)) lines.push(`${name}: ${value}`)
  lines.push('', '')
  return Buffer.concat([Buffer.from(lines.join('\r\n'), 'latin1'), body, Buffer.from('\r\n')])
}

function close() {
  return Buffer.from(`--${BOUNDARY}--\r\n`, 'latin1')
}

function file(filename, contentType, body) {
  return Buffer.concat([
    part({ 'Content-Disposition': `form-data; name="file"; filename="${filename}"`, 'Content-Type': contentType }, body),
    close(),
  ])
}

const bodies = {
  // A PNG a person picked in the document editor.
  'png.bin': file('photo.png', 'image/png', PNG),
  // A document, which a folder beside a document may not hold.
  'html.bin': file('x.html', 'text/html', Buffer.from('<html><body>hi</body></html>', 'latin1')),
  // SVG is deliberately accepted and served inert.
  'svg.bin': file('logo.svg', 'image/svg+xml', Buffer.from('<svg xmlns="http://www.w3.org/2000/svg"></svg>', 'latin1')),
  // A file part with no bytes in it.
  'empty.bin': file('empty.txt', 'text/plain', Buffer.alloc(0)),
  // A field, never a file.
  'nofile.bin': Buffer.concat([
    part({ 'Content-Disposition': 'form-data; name="note"' }, Buffer.from('no file here', 'latin1')),
    close(),
  ]),
  // Bytes that hold no multipart form at all, sent under a multipart type.
  'malformed.bin': Buffer.from('not a multipart body, and no boundary in it\r\n', 'latin1'),
}

for (const [name, bytes] of Object.entries(bodies)) {
  fs.writeFileSync(path.join(__dirname, name), bytes)
}
