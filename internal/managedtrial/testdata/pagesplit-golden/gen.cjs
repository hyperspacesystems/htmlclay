'use strict'

// Golden answers for the Go port of ClayJS's splitPage and composePage, the two
// functions that keep a page editor's merge state out of a person's file.
//
//   node gen.cjs <hyperclay-local checkout> <out.json>
//
// The answers come from the package's `require` export, dist/server/rooms.cjs.
// Should a future bundle drop splitPage from it, the ESM bundle
// (dist/server/rooms.mjs) is loaded with a dynamic import() instead, and the
// golden records which module answered.

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

async function load() {
  const cjs = require(CJS)
  if (typeof cjs.splitPage === 'function' && typeof cjs.composePage === 'function') {
    return { module: cjs, source: 'dist/server/rooms.cjs' }
  }
  const esm = await import(pathToFileURL(MJS).href)
  if (typeof esm.splitPage !== 'function' || typeof esm.composePage !== 'function') {
    throw new Error('ClayJS exports no splitPage/composePage from either bundle')
  }
  return { module: esm, source: 'dist/server/rooms.mjs' }
}

const templateDir = path.join(__dirname, '..', '..', '..', 'pageadmission', 'testdata')
const read = (name) => fs.readFileSync(path.join(templateDir, name), 'utf8')

const ypage = read('ypage-template.html')
const untagged = read('untagged-page-template.html')
const tag = /<script type="application\/json" merge="blockeditor" clay="live">[\s\S]*?<\/script>/.exec(ypage)[0]
const rev = '093360993130d818'
const article = '<article class="blockeditor" clay="freeze" data-blockeditor="1" data-rev="' + rev + '"></article>'

const page = (lines, newline = '\n') =>
  ['<!doctype html>', '<html>', '<body>', ...lines, '</body>', '</html>', ''].join(newline)

const cases = [
  { name: 'ypage template', input: ypage },
  { name: 'untagged template', input: untagged },
  { name: 'tag on its own indented line', input: page(['  ' + tag, article]) },
  { name: 'tag sharing a line with other markup', input: page(['<div>' + tag + '</div>', article]) },
  { name: 'crlf file', input: page([tag, article], '\r\n') },
  { name: 'cr file', input: page([tag, article], '\r') },
  { name: 'lf file', input: page([tag, article]) },
  { name: 'tag after the article', input: page([article, tag]) },
  {
    name: 'tag inside the article',
    input: page(['<article class="blockeditor" data-blockeditor="1" data-rev="' + rev + '">before' + tag + 'after</article>'])
  },
  {
    name: 'two articles',
    input: page([
      '<article data-blockeditor="1" data-rev="aaa"></article>',
      '<article data-blockeditor="2" data-rev="bbb"></article>',
      tag
    ])
  },
  {
    name: 'article inside template beside a real one',
    input: page(['<template><article data-blockeditor="1"></article></template>', tag, article])
  },
  {
    name: 'article inside noscript beside a real one',
    input: page(['<noscript><article data-blockeditor="1"></article></noscript>', tag, article])
  },
  {
    name: 'article with no data-rev beside a tag',
    input: page([tag, '<article class="blockeditor" clay="freeze" data-blockeditor="1"></article>'])
  },
  { name: 'two page tags', input: page([tag, tag, article]) },
  {
    name: 'tag without clay live',
    input: page([tag.replace(' clay="live"', ''), article])
  },
  {
    name: 'tag with clay x live y',
    input: page([tag.replace('clay="live"', 'clay="x live y"'), article])
  },
  {
    name: 'merge page tag that is not yjs',
    input: page(['<script type="application/json" merge="blockeditor" clay="live">{"blocks":{}}</script>', article])
  },
  {
    name: 'non-ascii text before the tag',
    input: page(['<p>héllo 😀 — naïve</p>', '  ' + tag, article])
  },
  { name: 'no article', input: page([tag, '<p>nothing</p>']) },
  { name: 'empty input', input: '' },
  { name: 'non-string input', input: null }
]

const composeOnly = [
  {
    name: 'compose onto a file that already holds a tag',
    file: page([tag, article])
  },
  {
    name: 'compose onto an article whose data-rev differs',
    file: page(['<article class="blockeditor" clay="freeze" data-blockeditor="1" data-rev="ffffffffffffffff"></article>'])
  },
  { name: 'compose onto a tab indented article line', file: page(['\t' + article]) },
  { name: 'compose onto a space indented article line', file: page(['    ' + article]) },
  { name: 'compose onto an article that shares its line', file: page(['<div>' + article + '</div>']) },
  { name: 'compose onto a file with no article', file: page(['<p>nothing</p>']) }
]

async function main() {
  const { module, source } = await load()
  const { splitPage, composePage } = module

  const recorded = cases.map((entry) => {
    const split = splitPage(entry.input)
    return {
      name: entry.name,
      input: entry.input,
      split,
      compose: composePage(split.file, split.state),
      composeStale: composePage(split.file, { ...split.state, rev: 'other' })
    }
  })

  const state = { v: 1, rev, tag }
  const composed = composeOnly.map((entry) => ({
    name: entry.name,
    file: entry.file,
    state,
    out: composePage(entry.file, state)
  }))

  const version = JSON.parse(fs.readFileSync(path.join(packageDir, 'package.json'), 'utf8')).version
  const golden = {
    generatedWith: { package: '@panphora/clayjs', version, module: source },
    cases: recorded,
    composeOnly: composed
  }
  fs.writeFileSync(outPath, JSON.stringify(golden, null, 2) + '\n')
}

main().catch((err) => {
  process.stderr.write(String(err && err.stack ? err.stack : err) + '\n')
  process.exit(1)
})
