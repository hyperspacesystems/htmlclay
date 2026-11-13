// Runs the host conformance package against a folder the real app serves, with
// the engines the trial binary can answer without a browser (`http,frozen`), and
// optionally checks that every check the trial binary passed in those engines
// passed here too.
//
//   node --import scripts/localhost-dns.mjs scripts/conformance-served.mjs \
//     --bin <htmlclay> --local <hyperclay-local tree> --out <dir> [--trial-report <p1 report>]
//
// Exit 0 pass, 1 fail, 3 could not run.
import { spawn } from 'node:child_process';
import dns from 'node:dns';
import { mkdirSync, mkdtempSync, readFileSync, rmSync, writeFileSync } from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { fileURLToPath, pathToFileURL } from 'node:url';
import { startProduct } from './served-product.mjs';

export const ENGINES = 'http,frozen';
const LOOPBACK = '127.0.0.1';
const TRIAL_ENGINES = ['http', 'frozen'];

function parseArgs(argv) {
  const options = {
    bin: null, local: null, out: null, trialReport: null,
  };
  for (let i = 0; i < argv.length; i += 1) {
    const flag = argv[i];
    const [name, inline] = flag.startsWith('--') && flag.includes('=')
      ? [flag.slice(0, flag.indexOf('=')), flag.slice(flag.indexOf('=') + 1)]
      : [flag, undefined];
    const known = ['--bin', '--local', '--out', '--trial-report'];
    if (!known.includes(name)) throw new Error(`unknown argument ${flag} (this script takes --bin, --local, --out, --trial-report)`);
    const value = inline ?? argv[(i += 1)];
    if (typeof value !== 'string' || !value) throw new Error(`${name} needs a value`);
    if (name === '--bin') options.bin = value;
    else if (name === '--local') options.local = value;
    else if (name === '--out') options.out = value;
    else options.trialReport = value;
  }
  for (const name of ['bin', 'local', 'out']) {
    if (!options[name]) throw new Error(`--${name} is required`);
  }
  return options;
}

// The package takes one address and sends it as the Host header, so the served
// name has to resolve to loopback in this process and in the package's own.
async function assertLoopbackResolution() {
  let answer;
  try {
    answer = await dns.promises.lookup('x.localhost');
  } catch (error) {
    throw new Error(`x.localhost does not resolve here (${error.code ?? error.message}); run this script with --import <htmlclay>/scripts/localhost-dns.mjs`);
  }
  if (answer?.address !== LOOPBACK) {
    throw new Error(`x.localhost resolves to ${answer?.address}, not ${LOOPBACK}; run this script with --import <htmlclay>/scripts/localhost-dns.mjs`);
  }
}

function readJson(file) {
  try {
    return JSON.parse(readFileSync(file, 'utf8'));
  } catch {
    return null;
  }
}

function runChild({ cwd, env, args }) {
  const child = spawn(process.execPath, args, {
    cwd, env, stdio: ['ignore', 'inherit', 'inherit'],
  });
  return new Promise((resolve, reject) => {
    child.on('error', (error) => reject(new Error(`the conformance package could not start: ${error.message}`)));
    child.on('exit', (code, signal) => (signal ? resolve({ signal }) : resolve({ code: code ?? 3 })));
  });
}

export async function main({ env = process.env, argv = process.argv.slice(2), log = console } = {}) {
  let options;
  try {
    options = parseArgs(argv);
  } catch (error) {
    log.error(`[conformance-served] ${error.message}`);
    return 3;
  }
  const bin = path.resolve(options.bin);
  const local = path.resolve(options.local);
  const out = path.resolve(options.out);
  const trialReportPath = options.trialReport ? path.resolve(options.trialReport) : null;
  const reportPath = path.join(out, 'conformance-served.json');
  const report = {
    bin,
    local,
    engines: ENGINES,
    packageExit: null,
    waivers: null,
    passed: [],
    failed: [],
    verdict: 'could-not-demonstrate',
  };

  let root = '';
  let product = null;
  let code = 3;
  let message = null;
  try {
    await assertLoopbackResolution();
    // `trial/p1.mjs` exports these; importing it runs nothing, because its main
    // is guarded by the invoked check at its end.
    const { placeDocuments, trialPackageArgs, judge, waiverCount } = await import(
      pathToFileURL(path.join(local, 'trial', 'p1.mjs')).href
    );
    const { PAGE, PROBE, PROBE_HTML, REQUIRED_EXTENSIONS, packageDir } = await import(
      pathToFileURL(path.join(local, 'scripts', 'conformance.mjs')).href
    );
    const dir = packageDir(local);
    const waivers = path.join(local, 'trial', 'trial-waivers.json');
    report.waivers = waiverCount(waivers);

    mkdirSync(out, { recursive: true });
    const packageReport = path.join(out, 'conformance-served.package.json');
    rmSync(packageReport, { force: true });

    root = mkdtempSync(path.join(os.homedir(), 'served-conformance-'));
    placeDocuments({ dir, root });
    product = await startProduct({
      bin, root, name: 'conf', log: path.join(out, 'product.log'),
    });

    const args = trialPackageArgs({
      dir,
      baseURL: product.url,
      root,
      reportPath: packageReport,
      extensions: REQUIRED_EXTENSIONS,
      waivers,
    });
    args[args.indexOf('--engine') + 1] = ENGINES;
    const dnsModule = pathToFileURL(path.join(path.dirname(fileURLToPath(import.meta.url)), 'localhost-dns.mjs')).href;
    const packageEnv = {
      ...env,
      NODE_OPTIONS: [env.NODE_OPTIONS ?? '', `--import=${dnsModule}`].filter(Boolean).join(' '),
    };
    const run = await runChild({ cwd: local, env: packageEnv, args });
    report.packageExit = run.code ?? null;
    report.packageReport = packageReport;

    const written = readJson(packageReport);
    const checks = Array.isArray(written?.checks) ? written.checks : [];
    report.passed = checks.filter((check) => check?.status === 'pass').map((check) => check.id);
    report.failed = checks.filter((check) => check?.status === 'fail').map((check) => check.id);

    let gateProblem = null;
    if (run.signal) gateProblem = `the conformance package was killed by ${run.signal}`;
    else if (!written) gateProblem = `the conformance package wrote no readable report at ${packageReport}`;

    let missing = [];
    if (trialReportPath) {
      const trial = readJson(trialReportPath);
      if (!trial) throw new Error(`cannot read the trial report ${trialReportPath}`);
      const trialPackage = readJson(
        typeof trial.packageReport === 'string' ? trial.packageReport : '',
      );
      if (!trialPackage) throw new Error(`the trial report ${trialReportPath} names no readable package report`);
      const passed = new Set(report.passed);
      missing = (trialPackage.checks ?? [])
        .filter((check) => check?.status === 'pass' && TRIAL_ENGINES.includes(check.engine))
        .map((check) => check.id)
        .filter((id) => !passed.has(id));
      report.missing = missing;
    }

    const judged = judge({
      packageExit: Number(report.packageExit),
      packageSignal: run.signal,
      waivers: report.waivers,
      gateProblem,
    });
    if (judged.code === 3) {
      code = 3;
      message = judged.message;
    } else if (judged.code === 1) {
      code = 1;
      message = judged.message;
    } else if (missing.length) {
      code = 1;
      message = `${missing.length} check(s) the trial binary passed in ${ENGINES} failed to pass here: ${missing.join(', ')}`;
    } else {
      code = 0;
    }
  } catch (error) {
    code = 3;
    message = error?.message || String(error);
  } finally {
    try {
      if (product) await product.stop();
    } catch {
      // stop() resolves once the child is gone; teardown cannot change a verdict.
    }
    if (root) rmSync(root, { recursive: true, force: true });
    report.verdict = code === 0 ? 'pass' : code === 1 ? 'fail' : 'could-not-demonstrate';
    try {
      mkdirSync(out, { recursive: true });
      writeFileSync(reportPath, `${JSON.stringify(report, null, 2)}\n`);
    } catch (error) {
      log.error(`[conformance-served] cannot write ${reportPath}: ${error.message}`);
      code = 3;
    }
  }
  if (message) log.error(`[conformance-served] ${message}`);
  return code;
}

const invoked = process.argv[1] !== undefined && import.meta.url === pathToFileURL(process.argv[1]).href;
if (invoked) process.exitCode = await main();
