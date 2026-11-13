'use strict'

// The managed trial host's Tailwind compiler: tailwind-hyperclay's own
// compileTailwind, loaded from the Hyperclay Local checkout named by
// --local-source, so a stylesheet this host writes is byte-identical to the one
// Local writes for the same document. The document arrives on stdin and the
// CSS leaves on stdout.

const path = require('path')

const flag = process.argv.indexOf('--local-source')
if (flag === -1 || !process.argv[flag + 1]) {
  process.stderr.write('usage: node tailwind-compile.cjs --local-source <hyperclay-local checkout>\n')
  process.exit(2)
}

const { compileTailwind } = require(path.join(process.argv[flag + 1], 'node_modules', 'tailwind-hyperclay'))

const chunks = []
process.stdin.on('data', (chunk) => chunks.push(chunk))
process.stdin.on('end', () => {
  compileTailwind(Buffer.concat(chunks).toString('utf8')).then(
    (css) => process.stdout.write(css),
    (error) => {
      process.stderr.write((error && error.message ? error.message : String(error)) + '\n')
      process.exitCode = 1
    },
  )
})
