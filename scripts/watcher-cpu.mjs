// Idle CPU of the real app serving a large folder: risk 7 in the real-app meta plan.
// Generates <files> files in <folders> folders under the home folder, serves it with
// startProduct, waits for the first walk to settle, then reads the app's CPU time
// before and after <seconds> of idle. Pass: under 2 percent of one core.
//   node scripts/watcher-cpu.mjs --bin <htmlclay> [--files 50000] [--folders 2000] [--seconds 120]
import { execFileSync } from 'node:child_process';
import { mkdirSync, mkdtempSync, rmSync, writeFileSync } from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { startProduct } from './served-product.mjs';

const arg = (name, fallback) => {
  const i = process.argv.indexOf(`--${name}`);
  return i >= 0 ? process.argv[i + 1] : fallback;
};
const bin = arg('bin');
if (!bin) { console.error('usage: watcher-cpu.mjs --bin <htmlclay> [--files N] [--folders N] [--seconds N]'); process.exit(3); }
const files = Number(arg('files', 50000));
const folders = Number(arg('folders', 2000));
const seconds = Number(arg('seconds', 120));
const settle = Number(arg('settle', 15));

const cpuSeconds = (pid) => {
  const t = execFileSync('ps', ['-o', 'time=', '-p', String(pid)], { encoding: 'utf8' }).trim();
  const [clock, frac = '0'] = t.replace(/^\d+-/, '').split('.');
  const parts = clock.split(':').map(Number);
  const whole = parts.reduce((sum, n) => sum * 60 + n, 0);
  return whole + Number(`0.${frac}`);
};

const root = mkdtempSync(path.join(os.homedir(), 'watcher-cpu-'));
let product;
try {
  for (let i = 0; i < files; i++) {
    const dir = path.join(root, `d${String(i % folders).padStart(4, '0')}`);
    if (i < folders) mkdirSync(dir);
    writeFileSync(path.join(dir, `f${i}.txt`), `file ${i}\n`);
  }
  product = await startProduct({ bin, root, name: 'cpu', log: path.join(os.tmpdir(), `watcher-cpu-${process.pid}.log`) });
  await new Promise((r) => setTimeout(r, settle * 1000));
  const before = cpuSeconds(product.pid);
  await new Promise((r) => setTimeout(r, seconds * 1000));
  const used = cpuSeconds(product.pid) - before;
  const percent = (used / seconds) * 100;
  const verdict = percent < 2 ? 'PASS' : 'FAIL';
  console.log(`watcher-cpu ${os.platform()} files=${files} folders=${folders} idle=${seconds}s cpu=${used.toFixed(2)}s ${percent.toFixed(2)}% of one core ${verdict}`);
  process.exitCode = verdict === 'PASS' ? 0 : 1;
} finally {
  if (product) await product.stop();
  rmSync(root, { recursive: true, force: true });
}
