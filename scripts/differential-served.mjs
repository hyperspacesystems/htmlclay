// Runs the trial's differential against the real app: the trial binary (the
// reference, `--a`) and `cmd/htmlclay` (the subject, `--b`), each serving its own
// copy of the synthetic root, compared by `<local>/trial/differential.mjs`.
//
//   node scripts/differential-served.mjs --bin <htmlclay> --trial-bin <managed-trial> \
//     --local <hyperclay-local tree> --source <synthetic root> --out <dir>
//
// The differential's own exit code is this script's: 0 no judged difference, 1 a
// difference, 2 it could not run. 3 means the hosts never came up.
import { spawnSync } from 'node:child_process';
import { cpSync, mkdirSync, mkdtempSync, rmSync, writeFileSync } from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { fileURLToPath, pathToFileURL } from 'node:url';
import { startProduct } from './served-product.mjs';

const MAX_BUFFER = 64 * 1024 * 1024;

function parseArgs(argv) {
  const options = {
    bin: null, trialBin: null, local: null, source: null, out: null,
  };
  for (let i = 0; i < argv.length; i += 1) {
    const flag = argv[i];
    const [name, inline] = flag.startsWith('--') && flag.includes('=')
      ? [flag.slice(0, flag.indexOf('=')), flag.slice(flag.indexOf('=') + 1)]
      : [flag, undefined];
    const known = ['--bin', '--trial-bin', '--local', '--source', '--out'];
    if (!known.includes(name)) throw new Error(`unknown argument ${flag} (this script takes --bin, --trial-bin, --local, --source, --out)`);
    const value = inline ?? argv[(i += 1)];
    if (typeof value !== 'string' || !value) throw new Error(`${name} needs a value`);
    if (name === '--bin') options.bin = value;
    else if (name === '--trial-bin') options.trialBin = value;
    else if (name === '--local') options.local = value;
    else if (name === '--source') options.source = value;
    else options.out = value;
  }
  for (const name of ['bin', 'trialBin', 'local', 'source', 'out']) {
    if (!options[name]) throw new Error(`--${name === 'trialBin' ? 'trial-bin' : name} is required`);
  }
  return options;
}

export async function main({ env = process.env, argv = process.argv.slice(2), log = console } = {}) {
  let options;
  try {
    options = parseArgs(argv);
  } catch (error) {
    log.error(`[differential-served] ${error.message}`);
    return 3;
  }
  const bin = path.resolve(options.bin);
  const trialBin = path.resolve(options.trialBin);
  const local = path.resolve(options.local);
  const source = path.resolve(options.source);
  const out = path.resolve(options.out);
  const dnsModule = pathToFileURL(path.join(path.dirname(fileURLToPath(import.meta.url)), 'localhost-dns.mjs')).href;

  let scratch = '';
  let managed = null;
  let product = null;
  let code = 3;
  try {
    mkdirSync(out, { recursive: true });
    scratch = mkdtempSync(path.join(os.homedir(), 'served-differential-'));
    const trialRoot = path.join(scratch, 'trial-root');
    const trialSpare = path.join(scratch, 'trial-spare');
    const productRoot = path.join(scratch, 'product-root');
    mkdirSync(trialSpare, { recursive: true });
    for (const root of [trialRoot, productRoot]) cpSync(source, root, { recursive: true, preserveTimestamps: true });

    // `trial/p8.mjs` starts its managed side exactly this way: two roots, the
    // second empty, and a scratch folder for the two children's logs.
    const { startManaged } = await import(pathToFileURL(path.join(local, 'trial', 'supervisor.mjs')).href);
    managed = await startManaged({
      bin: trialBin, scratch, roots: [trialRoot, trialSpare], localSource: local, env,
    });
    log.error(`[differential-served] reference ${managed.url}`);
    product = await startProduct({
      bin, root: productRoot, name: 'subject', log: path.join(out, 'product.log'),
    });
    log.error(`[differential-served] subject ${product.url}`);

    const differentialOut = path.join(out, 'differential');
    const run = spawnSync(process.execPath, [
      '--import', dnsModule,
      path.join(local, 'trial', 'differential.mjs'),
      '--a', managed.url,
      '--b', product.url,
      '--root', productRoot,
      '--out', differentialOut,
      // native until the product runs the Node service (sub plan 4, step 4.12): the
      // service's extensions are the AD1 allowance. File times are kept in the copies,
      // because a document's ETag carries its modification time.
      '--stage', 'native',
    ], {
      cwd: local, env, encoding: 'utf8', maxBuffer: MAX_BUFFER,
    });
    writeFileSync(
      path.join(out, 'differential.log'),
      `${run.stdout || ''}${run.stderr || ''}`,
    );
    if (run.error) throw new Error(`the differential could not run: ${run.error.message}`);
    code = run.status ?? 3;
    if (run.status === 0) log.error('[differential-served] no judged difference');
  } catch (error) {
    code = 3;
    log.error(`[differential-served] ${error?.message || String(error)}`);
  } finally {
    try {
      if (product) await product.stop();
    } catch {
      // stop() resolves once the child is gone; teardown cannot change a verdict.
    }
    try {
      if (managed) await managed.stop();
    } catch {
      // Same.
    }
    if (scratch) rmSync(scratch, { recursive: true, force: true });
  }
  return code;
}

const invoked = process.argv[1] !== undefined && import.meta.url === pathToFileURL(process.argv[1]).href;
if (invoked) process.exitCode = await main();
