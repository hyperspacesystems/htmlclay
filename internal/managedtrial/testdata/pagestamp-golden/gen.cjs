'use strict'

// Golden answers for the Go port of ClayJS's stampPageRev and of the two format
// -floor helpers it calls, recorded from the package's `require` export.
//
//   node gen.cjs <hyperclay-local checkout> <out.json>
//
// stampPageRev is what a restore stamps onto a version's bytes before it writes
// them: the stored page record's revision onto the one non-inert
// article[data-blockeditor], and the floor the write declares. Should a future bundle
// drop it from the CJS export, the ESM bundle (dist/server/rooms.mjs) is loaded
// with a dynamic import() instead, and which module answered is recorded.

const fs = require('fs')
const path = require('path')
const { pathToFileURL } = require('url')

const checkout = process.argv[2]
const outPath = process.argv[3]
if (!checkout || !outPath) {
  process.stderr.write('usage: node gen.cjs <hyperclay-local checkout> <out.json>\n')
  process.exit(1)
}

const packageDir = path.join(checkout, 'node_modules', '@panphora', 'clayjs')
const CJS = path.join(packageDir, 'dist', 'server', 'rooms.cjs')
const MJS = path.join(packageDir, 'dist', 'server', 'rooms.mjs')

const wanted = ['stampPageRev', 'blockeditorFormatFloor', 'withPageFormatFloor']
const has = (module) => wanted.every((name) => typeof module[name] === 'function')

async function load() {
  const cjs = require(CJS)
  if (has(cjs)) return { module: cjs, source: 'dist/server/rooms.cjs' }
  const esm = await import(pathToFileURL(MJS).href)
  if (!has(esm)) {
    throw new Error('ClayJS exports no stampPageRev/blockeditorFormatFloor/withPageFormatFloor from either bundle')
  }
  return { module: esm, source: 'dist/server/rooms.mjs' }
}

const REV = '093360993130d818'
const NEXT = '5f2b9c0d1e3a4b6c'

const page = (lines, newline = '\n') =>
  ['<!doctype html>', '<html>', '<body>', ...lines, '</body>', '</html>', ''].join(newline)

const article = '<article class="blockeditor" clay="freeze" data-blockeditor="1"></article>'
const revisioned = '<article class="blockeditor" clay="freeze" data-blockeditor="1" data-rev="' + REV + '"></article>'

// Each input is a document; every one is stamped at each floor in FLOORS, so the
// floor's half of the answer is proven on the same bytes as the revision's.
const inputs = [
  { name: 'one page article', html: page([article]) },
  { name: 'article already carrying data-rev', html: page([revisioned]) },
  {
    name: 'two articles',
    html: page(['<article data-blockeditor="1" data-rev="aaa"></article>', '<article data-blockeditor="2" data-rev="bbb"></article>'])
  },
  { name: 'article inside a template', html: page(['<template><article data-blockeditor="1"></article></template>']) },
  { name: 'article inside a noscript', html: page(['<noscript><article data-blockeditor="1"></article></noscript>']) },
  {
    name: 'article inside a template beside a real one',
    html: page(['<template><article data-blockeditor="1"></article></template>', article])
  },
  {
    name: 'article inside a noscript beside a real one',
    html: page(['<noscript><article data-blockeditor="1"></article></noscript>', article])
  },
  { name: 'no article', html: page(['<p>nothing</p>']) },
  { name: 'empty document', html: '' },
  { name: 'single quoted attributes', html: page(["<article data-blockeditor='1' data-rev='old'></article>"]) },
  { name: 'unquoted attributes', html: page(['<article data-blockeditor=1 data-rev=old></article>']) },
  { name: 'odd attribute case', html: page(['<ARTICLE DATA-BLOCKEDITOR="1" DATA-REV="old"></ARTICLE>']) },
  { name: 'spaces around the attribute values', html: page(['<article data-blockeditor="1" data-rev = "old" ></article>']) },
  { name: 'data-rev before data-blockeditor', html: page(['<article data-rev="old" data-blockeditor="1"></article>']) },
  { name: 'data-rev with no value', html: page(['<article data-blockeditor data-rev>']) },
  { name: 'duplicate data-rev', html: page(['<article data-blockeditor="1" data-rev="old" data-rev="older"></article>']) },
  { name: 'self closing article', html: page(['<article data-blockeditor="1"/>']) },
  { name: 'article sharing a line with other markup', html: page(['<div>' + article + '</div>']) },
  {
    name: 'document already at a floor',
    html: '<!doctype html>\n<html>\n<head>\n<meta name="clay-blockeditor-format" content="1">\n</head>\n<body>\n<article data-blockeditor="1"></article>\n</body>\n</html>\n'
  },
  {
    name: 'document at floor two in odd case',
    html: '<!doctype html>\n<html>\n<head>\n<META NAME="clay-blockeditor-format" CONTENT=\'2\'>\n</head>\n<body>\n<article data-blockeditor="1"></article>\n</body>\n</html>\n'
  },
  {
    name: 'non-ascii before the article',
    html: page(['<p>héllo 😀 — naïve</p>', '<article data-blockeditor="1"></article>'])
  }
]

const FLOORS = [0, 1, 2]

async function main() {
  const { module, source } = await load()
  const { stampPageRev, blockeditorFormatFloor, withPageFormatFloor } = module

  const cases = []
  for (const input of inputs) {
    for (const floor of FLOORS) {
      cases.push({
        name: input.name + ' at floor ' + floor,
        html: input.html,
        rev: NEXT,
        floor,
        stamp: stampPageRev(input.html, NEXT, floor),
        floorOnly: withPageFormatFloor(input.html, floor),
        blockeditorFormatFloor: blockeditorFormatFloor(input.html)
      })
    }
  }
  cases.push({
    name: 'empty rev',
    html: page([article]),
    rev: '',
    floor: 0,
    stamp: stampPageRev(page([article]), '', 0),
    floorOnly: withPageFormatFloor(page([article]), 0),
    blockeditorFormatFloor: blockeditorFormatFloor(page([article]))
  })

  const golden = { generatedWith: { module: source }, cases }
  fs.mkdirSync(path.dirname(outPath), { recursive: true })
  fs.writeFileSync(outPath, JSON.stringify(golden, null, 2) + '\n')
}

main().catch((err) => {
  process.stderr.write(String(err && err.stack ? err.stack : err) + '\n')
  process.exit(1)
})
