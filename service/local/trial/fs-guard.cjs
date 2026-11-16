'use strict';

const fs = require('fs');
const path = require('path');
const url = require('url');

const rawWriteFileSync = fs.writeFileSync;
const rawMkdirSync = fs.mkdirSync;
const rawRealpathSync = fs.realpathSync;
const rawOpen = fs.promises.open;

const ROOTS_ENV = 'FS_GUARD_ROOTS';
const OUT_ENV = 'FS_GUARD_OUT';

const GO_STORAGE_DIRS = ['versions', 'recovered', 'blockeditor-state', 'guard', 'api-v2', 'tailwindcss'];

const forbidden = [];
const allowed = [];
let written = false;

function readConfig() {
  const rootsRaw = process.env[ROOTS_ENV];
  const out = process.env[OUT_ENV];
  if (!rootsRaw || !out) return null;
  let roots;
  try {
    roots = JSON.parse(rootsRaw);
  } catch (error) {
    return null;
  }
  if (!Array.isArray(roots) || roots.length === 0) return null;
  if (!roots.every((root) => typeof root === 'string' && root)) return null;
  if (typeof out !== 'string' || !path.isAbsolute(out)) return null;
  return { roots, out };
}

const config = readConfig();

function realOrNearest(target) {
  let current = path.resolve(target);
  const tail = [];
  for (;;) {
    try {
      const real = rawRealpathSync(current);
      return tail.length ? path.join(real, ...tail) : real;
    } catch (error) {
      if (error.code !== 'ENOENT' && error.code !== 'ENOTDIR') throw error;
      const parent = path.dirname(current);
      if (parent === current) throw error;
      tail.unshift(path.basename(current));
      current = parent;
    }
  }
}

const realRoots = config ? config.roots.map((root) => realOrNearest(root)) : [];

function toPathString(target) {
  if (typeof target === 'string') return target;
  if (Buffer.isBuffer(target)) return target.toString();
  if (target instanceof url.URL) return url.fileURLToPath(target);
  return null;
}

function classifyRelative(rel, op) {
  // The root marker is written to its own temp file and renamed into place
  // (src/sync-engine/root-marker.js), so both names are the marker's write.
  if (rel === '.hyperclay/sync-root.json' || rel === '.hyperclay/sync-root.json.tmp' || rel.startsWith('.hyperclay/conflicts/')) {
    return { class: 'sync-state', allowed: true };
  }
  for (const dir of GO_STORAGE_DIRS) {
    const prefix = '.hyperclay/' + dir;
    if (rel === prefix || rel.startsWith(prefix + '/')) return { class: 'go-storage', allowed: false };
  }
  if (rel === 'sites-versions' || rel.startsWith('sites-versions/')) return { class: 'go-storage', allowed: false };
  if (op === 'mkdir' || op === 'mkdirSync') return { class: 'directory', allowed: true };
  return { class: 'document', allowed: false };
}

function classifyTarget(target, op) {
  const value = toPathString(target);
  if (value === null) return null;
  let real;
  try {
    real = realOrNearest(value);
  } catch (error) {
    return null;
  }
  for (const root of realRoots) {
    let rel = path.relative(root, real);
    if (rel !== '' && (rel === '..' || rel.startsWith('..' + path.sep) || path.isAbsolute(rel))) continue;
    rel = rel.split(path.sep).join('/');
    const info = classifyRelative(rel, op);
    return { path: rel, class: info.class, allowed: info.allowed };
  }
  return null;
}

function captureStack() {
  return String(new Error().stack || '').split('\n').slice(0, 8).join('\n');
}

function record(op, target) {
  if (!config) return;
  let info;
  try {
    info = classifyTarget(target, op);
  } catch (error) {
    return;
  }
  if (!info) return;
  if (info.allowed) {
    allowed.push({ op, path: info.path, class: info.class });
  } else {
    forbidden.push({ op, path: info.path, class: info.class, stack: captureStack() });
  }
}

function isWriteFlags(flags) {
  if (flags === undefined || flags === null) return false;
  if (typeof flags === 'number') {
    const constants = fs.constants;
    const mask = constants.O_WRONLY | constants.O_RDWR | constants.O_CREAT | constants.O_TRUNC | constants.O_APPEND | constants.O_EXCL;
    return (flags & mask) !== 0;
  }
  if (typeof flags !== 'string') return false;
  return /[wa+x]/.test(flags);
}

function recordOpen(name, args) {
  if (isWriteFlags(args[1])) record(name, args[0]);
}

function recordFirst(name, args) {
  record(name, args[0]);
}

function recordSecond(name, args) {
  record(name, args[1]);
}

function recordRename(name, args) {
  record(name, args[1]);
  record(name + '-source', args[0]);
}

const SINGLE_TARGET_OPS = [
  'writeFile',
  'appendFile',
  'createWriteStream',
  'rm',
  'rmdir',
  'unlink',
  'mkdir',
  'mkdtemp',
  'truncate',
  'utimes',
  'lutimes',
  'chmod',
  'chown',
];

const SECOND_TARGET_OPS = ['copyFile', 'cp', 'link', 'symlink'];

function wrapFn(holder, name, handler) {
  if (!holder) return;
  const original = holder[name];
  if (typeof original !== 'function') return;
  const wrapped = function (...args) {
    try {
      handler(name, args);
    } catch (error) {
      /* never let the guard change behavior */
    }
    return original.apply(this, args);
  };
  try {
    Object.defineProperty(wrapped, 'name', { value: original.name, configurable: true });
  } catch (error) {
    /* ignore */
  }
  holder[name] = wrapped;
}

function wrapTriple(baseName, handler) {
  wrapFn(fs, baseName, handler);
  wrapFn(fs, baseName + 'Sync', handler);
  wrapFn(fs.promises, baseName, handler);
}

for (const baseName of SINGLE_TARGET_OPS) {
  wrapTriple(baseName, recordFirst);
}

for (const baseName of SECOND_TARGET_OPS) {
  wrapTriple(baseName, recordSecond);
}

wrapTriple('rename', recordRename);
wrapFn(fs, 'open', recordOpen);
wrapFn(fs, 'openSync', recordOpen);

const handlePaths = new WeakMap();

fs.promises.open = function open(...args) {
  try {
    recordOpen('open', args);
  } catch (error) {
    /* ignore */
  }
  const promise = rawOpen.apply(this, args);
  const target = toPathString(args[0]);
  return promise.then((handle) => {
    if (handle && target !== null) handlePaths.set(handle, path.resolve(target));
    return handle;
  });
};

const FILE_HANDLE_METHODS = ['writeFile', 'appendFile', 'write', 'writev', 'truncate', 'utimes', 'chmod', 'chown'];
const FILE_HANDLE_DISABLED = Boolean(process.env.FS_GUARD_DISABLE_FILEHANDLE);

function patchFileHandlePrototype(proto) {
  if (!proto) return;
  for (const name of FILE_HANDLE_METHODS) {
    const original = proto[name];
    if (typeof original !== 'function') continue;
    proto[name] = function (...args) {
      try {
        const target = handlePaths.get(this);
        if (target !== undefined) record(name, target);
      } catch (error) {
        /* ignore */
      }
      return original.apply(this, args);
    };
  }
}

if (!FILE_HANDLE_DISABLED) {
  if (typeof fs.promises.FileHandle === 'function') {
    patchFileHandlePrototype(fs.promises.FileHandle.prototype);
  } else {
    rawOpen.call(fs.promises, process.execPath, 'r').then(
      (handle) => {
        try {
          patchFileHandlePrototype(handle.constructor.prototype);
        } finally {
          handle.close().catch(() => {});
        }
      },
      () => {}
    );
  }
}

function flush() {
  if (!config || written) return;
  written = true;
  const payload = {
    roots: config.roots,
    forbidden,
    allowed,
    counts: { forbidden: forbidden.length, allowed: allowed.length },
  };
  try {
    rawMkdirSync(path.dirname(config.out), { recursive: true });
    rawWriteFileSync(config.out, JSON.stringify(payload, null, 2));
  } catch (error) {
    try {
      process.stderr.write('fs-guard: could not write ' + config.out + ': ' + error.message + '\n');
    } catch (ignored) {
      /* ignore */
    }
  }
}

if (config) process.on('exit', flush);

module.exports = { config: Boolean(config) };
