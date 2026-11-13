const { spawnSync } = require('child_process')
process.exit(spawnSync(process.execPath, [require('path').join(__dirname, '..', 'http-golden.cjs'), process.argv[2], __dirname, process.argv[3]], { stdio: 'inherit' }).status)
