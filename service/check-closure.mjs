#!/usr/bin/env node
// Replaces scripts/vendor-local.mjs --check after SP18a moves the sync engine
// and room runtime into service/local/. Checks three invariants before
// service/build.mjs or CI packages the service:
// 1. Every relative require/import from service/main.cjs and its local/ closure
//    resolves to an existing file inside service/.
// 2. Every external package specifier is declared in service/package.json
//    dependencies (only the two lazy logger fallbacks may mention 'electron').
// 3. Every service entrypoint loads in plain Node with Module._load refusing
//    'electron'.

import fs from 'node:fs';
import path from 'node:path';
import { builtinModules, createRequire } from 'node:module';
import { fileURLToPath } from 'node:url';

// The folder to check: service/ itself, or the staged copy build.mjs passes once
// its packages are installed.
const serviceDir = process.argv[2] ? path.resolve(process.argv[2]) : path.dirname(fileURLToPath(import.meta.url));
const requireCjs = createRequire(import.meta.url);
const Module = requireCjs('node:module');

const ALLOWED_LAZY_ELECTRON = new Set([
  'local/src/main/error-logger.js',
  'local/src/sync-engine/logger.js',
]);

const ENTRY_FILES = [
  'main.cjs',
  'sync.cjs',
  'shell.cjs',
  'recover.cjs',
  'sync-net.cjs',
  'local/trial/service.cjs',
  'local/trial/fs-guard.cjs',
  'local/trial/sync-observer.cjs',
  'local/src/main/sync-manager.js',
  'local/src/sync-engine/index.js',
  'local/src/main/workspace/shell-server.js',
];

const LOAD_ROOTS = [
  'sync.cjs',
  'shell.cjs',
  'recover.cjs',
  'sync-net.cjs',
  'local/trial/service.cjs',
  'local/trial/sync-observer.cjs',
  'local/src/main/sync-manager.js',
  'local/src/sync-engine/index.js',
  'local/src/main/workspace/shell-server.js',
];

function fail(msg) {
  console.error(`service/check-closure: ${msg}`);
  process.exit(1);
}

const builtins = new Set(builtinModules.flatMap((m) => [m, `node:${m}`]));
const pkg = JSON.parse(fs.readFileSync(path.join(serviceDir, 'package.json'), 'utf8'));
const declaredDeps = new Set(Object.keys(pkg.dependencies || {}));

function pkgNameOf(spec) {
  if (spec.startsWith('@')) {
    const parts = spec.split('/');
    return parts.length >= 2 ? `${parts[0]}/${parts[1]}` : spec;
  }
  return spec.split('/')[0];
}

function resolveLocal(fromFile, spec) {
  const base = path.resolve(path.dirname(fromFile), spec);
  const candidates = [
    base,
    `${base}.js`,
    `${base}.cjs`,
    `${base}.json`,
    path.join(base, 'index.js'),
    path.join(base, 'index.cjs'),
  ];
  for (const candidate of candidates) {
    if (fs.existsSync(candidate) && fs.statSync(candidate).isFile()) {
      const rel = path.relative(serviceDir, candidate).split(path.sep).join('/');
      if (rel.startsWith('..') || path.isAbsolute(rel)) {
        fail(`${path.relative(serviceDir, fromFile)} escapes service/ via ${spec}`);
      }
      return candidate;
    }
  }
  return null;
}

const visited = new Set();
const queue = ENTRY_FILES.map((rel) => {
  const full = path.join(serviceDir, rel);
  if (!fs.existsSync(full)) fail(`missing entrypoint ${rel}`);
  return full;
});

while (queue.length > 0) {
  const current = queue.shift();
  if (visited.has(current)) continue;
  visited.add(current);
  if (current.endsWith('.json')) continue;

  const relFromService = path.relative(serviceDir, current).split(path.sep).join('/');
  const source = fs.readFileSync(current, 'utf8');
  const specs = [
    ...source.matchAll(/\brequire\(\s*['"]([^'"]+)['"]\s*\)/g),
    ...source.matchAll(/\bimport\(\s*['"]([^'"]+)['"]\s*\)/g),
  ].map((m) => m[1]);

  for (const spec of specs) {
    if (spec.startsWith('./') || spec.startsWith('../')) {
      const target = resolveLocal(current, spec);
      if (!target) fail(`unresolved ${spec} in ${relFromService}`);
      queue.push(target);
      continue;
    }
    if (builtins.has(spec) || spec.startsWith('node:')) continue;
    if (spec === 'electron') {
      if (!ALLOWED_LAZY_ELECTRON.has(relFromService)) {
        fail(`forbidden require('electron') in ${relFromService}`);
      }
      continue;
    }
    const pkgName = pkgNameOf(spec);
    if (!declaredDeps.has(pkgName)) {
      fail(`undeclared dependency "${pkgName}" (from "${spec}" in ${relFromService})`);
    }
  }
}

const originalLoad = Module._load;
Module._load = function guardElectron(request, parent, isMain) {
  if (request === 'electron') {
    throw new Error(`eager require('electron') from ${parent?.filename || 'unknown'}`);
  }
  return originalLoad.call(this, request, parent, isMain);
};

for (const rel of LOAD_ROOTS) {
  requireCjs(path.join(serviceDir, rel));
}

console.log(`service/check-closure: ok (${visited.size} reachable modules, ${declaredDeps.size} declared deps, 0 eager electron loads)`);
