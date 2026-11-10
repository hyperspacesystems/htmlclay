'use strict'

// Preloaded into the fixture process (NODE_OPTIONS=--require), so the answers a
// write produces are the same on every recording. A version's name carries the
// time it was written and a page-state generation is 16 random bytes, and
// neither is reproducible while the clock and the generator are real.

const REAL_DATE = Date
const FIXED_NOW = Number(process.env.GOLDEN_PIN_CLOCK)

class FakeDate extends REAL_DATE {
  constructor(...args) {
    if (args.length === 0) super(FIXED_NOW)
    else super(...args)
  }

  static now() {
    return FIXED_NOW
  }
}

global.Date = FakeDate

const fs = require('fs')
const crypto = require('crypto')
const log = process.env.GOLDEN_PIN_GENERATIONS || ''
let generations = 0
crypto.randomBytes = (size) => {
  const bytes = Buffer.alloc(size)
  if (size === 16) {
    bytes.writeUInt32BE(generations, 12)
    if (log) fs.appendFileSync(log, bytes.toString('hex') + '\n')
    generations++
  }
  return bytes
}
