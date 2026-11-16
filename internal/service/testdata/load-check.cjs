'use strict';
// Loads, from the file that names it, every package specifier the vendored
// service code requires or imports with a string literal, using only the Node
// running this script. Prints "LOADED <n>" or the first failure.
const fs = require('fs');
const path = require('path');
const { createRequire, builtinModules } = require('module');
const app = path.resolve(process.argv[2]);
const builtins = new Set(builtinModules.flatMap((m) => [m, `node:${m}`]));
const seen = new Map();
(function walk(dir) {
  for (const e of fs.readdirSync(dir, { withFileTypes: true })) {
    const full = path.join(dir, e.name);
    if (e.isDirectory()) { if (e.name !== 'node_modules' && e.name !== 'offline-runtime') walk(full); continue; }
    if (!/\.(c?js)$/.test(e.name)) continue;
    const text = fs.readFileSync(full, 'utf8');
    for (const m of text.matchAll(/\b(?:require|import)\(\s*'([^'./][^']*)'\s*\)/g)) {
      const spec = m[1];
      if (builtins.has(spec) || spec === 'electron' || spec.startsWith('node:')) continue;
      if (!seen.has(spec)) seen.set(spec, full);
    }
  }
})(path.join(app, 'local'));
(async () => {
  for (const [spec, from] of seen) {
    try {
      createRequire(from)(spec);
    } catch (error) {
      if (error.code !== 'ERR_REQUIRE_ESM' && error.code !== 'ERR_REQUIRE_ASYNC_MODULE') {
        console.log(`FAILED ${spec} from ${path.relative(app, from)}: ${error.code || ''} ${error.message.split('\n')[0]}`);
        process.exit(1);
      }
      const { pathToFileURL } = require('url');
      await import(pathToFileURL(createRequire(from).resolve(spec)).href);
    }
  }
  console.log(`LOADED ${seen.size}`);
})();
