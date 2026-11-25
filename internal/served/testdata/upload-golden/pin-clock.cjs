'use strict'

// Preloaded into the fixture process (NODE_OPTIONS=--require), so the answers
// that stamp a write are the same on every recording. An upload's file is
// stamped with the instant it was written, and that instant is nobody else's.
// The host creates one with an exclusive write, so the stamp is set on the
// handle it closes.

const fs = require('fs')

const PINNED = new Date(Number(process.env.GOLDEN_PIN_CLOCK))

const realOpen = fs.promises.open.bind(fs.promises)
fs.promises.open = async (file, flags, mode) => {
  const handle = await realOpen(file, flags, mode)
  if (typeof flags === 'string' && flags.includes('w')) {
    const realClose = handle.close.bind(handle)
    handle.close = async (...args) => {
      try {
        await handle.utimes(PINNED, PINNED)
      } catch { /* a handle that cannot stamp is still closed */ }
      return realClose(...args)
    }
  }
  return handle
}
