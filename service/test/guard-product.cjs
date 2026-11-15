'use strict';

// 08-sync.md step 8.26: the live-room service's write classes in the product.
//
// Loaded with `node --require guard-product.cjs main.cjs`, it records every write
// the service makes through the trial's fs-guard (roots "/", so nothing escapes
// it) and, on exit, rewrites that report in the product's classes:
//   own    the service's data dir, its temp dir, /dev and /proc: allowed
//   root   anything inside a served root: forbidden. Go makes every write there,
//          so unlike the trial's guard a sync-state file or a new folder is no
//          exception
//   other  anywhere else: forbidden
// Env: PRODUCT_GUARD_OUT (the report), PRODUCT_GUARD_OWN and PRODUCT_GUARD_ROOTS
// (JSON arrays of absolute folders).
//
// Run as a program, it reads a run's reports and strace logs and prints one line:
//   node guard-product.cjs analyze <dir> --own a,b --roots c [--expect <fragment>]
// <dir> holds guard/*.json and strace/*.txt. Exit 0 when both methods saw a
// process and neither saw a forbidden write; with --expect, exit 0 only when both
// saw a write whose path contains the fragment (the canary control).

const fs = require('node:fs');
const path = require('node:path');

const SYSTEM_OWN = ['/dev', '/proc'];

function under(target, dir) {
  return target === dir || target.startsWith(dir.endsWith('/') ? dir : `${dir}/`);
}

function classify(target, own, roots) {
  if ([...own, ...SYSTEM_OWN].some((dir) => under(target, dir))) return 'own';
  if (roots.some((dir) => under(target, dir))) return 'root';
  return 'other';
}

function realList(list) {
  return list.map((dir) => {
    try {
      return fs.realpathSync(dir);
    } catch {
      return path.resolve(dir);
    }
  });
}

function hook() {
  const out = process.env.PRODUCT_GUARD_OUT;
  if (!out || !path.isAbsolute(out)) return;
  const own = realList(JSON.parse(process.env.PRODUCT_GUARD_OWN || '[]'));
  const roots = realList(JSON.parse(process.env.PRODUCT_GUARD_ROOTS || '[]'));
  const raw = `${out}.raw`;
  process.env.FS_GUARD_ROOTS = '["/"]';
  process.env.FS_GUARD_OUT = raw;
  require('../vendor/local/trial/fs-guard.cjs');
  // fs-guard flushes its own report on exit first; this handler runs after it.
  process.on('exit', () => {
    let report;
    try {
      report = JSON.parse(fs.readFileSync(raw, 'utf8'));
    } catch (error) {
      process.stderr.write(`guard-product: no fs-guard report: ${error.message}\n`);
      return;
    }
    const writes = [...report.forbidden, ...report.allowed].map((entry) => {
      const target = `/${entry.path}`;
      return { op: entry.op, path: target, class: classify(target, own, roots), stack: entry.stack };
    });
    const forbidden = writes.filter((w) => w.class !== 'own');
    fs.writeFileSync(out, JSON.stringify({ own, roots, forbidden, own_writes: writes.length - forbidden.length }, null, 2));
  });
}

const WRITE_OPEN = /O_WRONLY|O_RDWR|O_CREAT|O_TRUNC|O_APPEND/;
const MUTATING = /^(creat|rename|renameat2?|unlink|unlinkat|mkdir|mkdirat|rmdir|link|linkat|symlink|symlinkat|truncate|chmod|fchmodat|chown|fchownat|lchown|utimensat|setxattr|lsetxattr|removexattr|mknod|mknodat)$/;

function analyze(dir, own, roots, expect) {
  const list = (sub) => (fs.existsSync(path.join(dir, sub)) ? fs.readdirSync(path.join(dir, sub)) : []);
  const result = { guard: { reports: 0, forbidden: [] }, strace: { files: 0, forbidden: [] } };
  for (const name of list('guard').filter((n) => n.endsWith('.json'))) {
    const report = JSON.parse(fs.readFileSync(path.join(dir, 'guard', name), 'utf8'));
    result.guard.reports += 1;
    for (const w of report.forbidden) result.guard.forbidden.push({ file: name, ...w });
  }
  for (const name of list('strace')) {
    result.strace.files += 1;
    for (const line of fs.readFileSync(path.join(dir, 'strace', name), 'utf8').split('\n')) {
      const m = /^(\d+)\s+(\w+)\((.*)\)\s+=\s+(-?\d+|\?)/.exec(line);
      if (!m) continue;
      const [, pid, call, args, ret] = m;
      if (ret.startsWith('-')) continue;
      const isOpen = call === 'open' || call === 'openat' || call === 'openat2';
      if (isOpen ? !WRITE_OPEN.test(args) : !MUTATING.test(call)) continue;
      for (const [, p] of args.matchAll(/"((?:[^"\\]|\\.)*)"/g)) {
        // A relative path cannot be placed, so it counts against the run.
        const cls = p.startsWith('/') ? classify(p, own, roots) : 'relative';
        if (cls !== 'own') result.strace.forbidden.push({ file: name, pid, call, path: p, class: cls, args: args.slice(0, 240) });
      }
    }
  }
  fs.writeFileSync(path.join(dir, 'analysis.json'), JSON.stringify(result, null, 2));
  const g = result.guard.forbidden.length;
  const s = result.strace.forbidden.length;
  console.log(`guard reports ${result.guard.reports}, forbidden ${g}; strace files ${result.strace.files}, forbidden ${s}`);
  for (const w of [...result.guard.forbidden, ...result.strace.forbidden].slice(0, 15)) console.log(`  ${w.class} ${w.op || w.call} ${w.path}`);
  if (expect) {
    const seen = (writes) => writes.some((w) => w.path.includes(expect));
    const both = seen(result.guard.forbidden) && seen(result.strace.forbidden);
    console.log(both ? 'canary=seen' : `canary=missed guard=${seen(result.guard.forbidden)} strace=${seen(result.strace.forbidden)}`);
    return both ? 0 : 1;
  }
  console.log(`forbidden=${g} strace=${s}`);
  return result.guard.reports > 0 && result.strace.files > 0 && g === 0 && s === 0 ? 0 : 1;
}

if (require.main === module) {
  const argv = process.argv.slice(2);
  const flag = (name) => {
    const i = argv.indexOf(name);
    return i >= 0 ? argv[i + 1] : null;
  };
  if (argv[0] !== 'analyze' || !argv[1] || !flag('--own') || !flag('--roots')) {
    console.error('usage: guard-product.cjs analyze <dir> --own a,b --roots c [--expect <fragment>]');
    process.exit(2);
  }
  process.exit(analyze(argv[1], realList(flag('--own').split(',')), realList(flag('--roots').split(',')), flag('--expect')));
} else {
  hook();
}
