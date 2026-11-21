#!/usr/bin/env node
// Builds the live-room service blob one HTML Clay platform embeds (tag
// service_bundle): the official Node binary pinned in node-pins.json, plus
// app/ (main.cjs, the vendored Local tree, and its packages installed from
// package-lock.json with optional packages left out, so the tree is the same
// on every platform). Writes internal/service/blobs/<goos>-<goarch>.tar.gz and
// .sha256 ("<sha256> <size>").
//
// Usage: node service/build.mjs [--goos <goos>] [--goarch <goarch>] [--node-archive <file>]
// Downloads go to service/.cache/, checked against the pinned sha256 either way.

import { createHash } from 'node:crypto';
import { execFileSync } from 'node:child_process';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const serviceDir = path.dirname(fileURLToPath(import.meta.url));
const repo = path.dirname(serviceDir);

const goPlatform = { darwin: 'darwin', linux: 'linux', win32: 'windows' }[process.platform];
const onWindows = process.platform === 'win32';
// On a Windows host "tar" may be Git's GNU tar, which reads C:\ as a remote
// host. Windows' own bsdtar never does, and it also reads a zip.
const tarProgram = onWindows ? path.join(process.env.SystemRoot || 'C:\\Windows', 'System32', 'tar.exe') : 'tar';
const goArch = { arm64: 'arm64', x64: 'amd64' }[process.arch];

function fail(message) {
  console.error(`service/build: ${message}`);
  process.exit(1);
}

function args() {
  const out = { goos: goPlatform, goarch: goArch, nodeArchive: null };
  const argv = process.argv.slice(2);
  for (let i = 0; i < argv.length; i += 1) {
    const value = argv[i + 1];
    if (argv[i] === '--goos') out.goos = value;
    else if (argv[i] === '--goarch') out.goarch = value;
    else if (argv[i] === '--node-archive') out.nodeArchive = value;
    else fail(`unknown argument ${argv[i]}`);
    i += 1;
  }
  return out;
}

const sha256 = (bytes) => createHash('sha256').update(bytes).digest('hex');

async function nodeArchive(pins, entry, given) {
  if (given) return fs.readFileSync(given);
  const cache = path.join(serviceDir, '.cache', entry.name);
  if (fs.existsSync(cache)) return fs.readFileSync(cache);
  const url = `https://nodejs.org/dist/v${pins.version}/${entry.name}`;
  console.log(`downloading ${url}`);
  const response = await fetch(url);
  if (!response.ok) fail(`${url} answered ${response.status}`);
  const bytes = Buffer.from(await response.arrayBuffer());
  fs.mkdirSync(path.dirname(cache), { recursive: true });
  fs.writeFileSync(cache, bytes);
  return bytes;
}

function extractNode(archivePath, entry, goos, stage) {
  const top = entry.name.replace(/\.(tar\.gz|zip)$/, '');
  const scratch = fs.mkdtempSync(path.join(os.tmpdir(), 'htmlclay-node-'));
  try {
    if (goos === 'windows') {
      if (onWindows) execFileSync(tarProgram, ['-xf', archivePath, '-C', scratch, `${top}/node.exe`]);
      else execFileSync('unzip', ['-q', archivePath, `${top}/node.exe`, '-d', scratch]);
      fs.copyFileSync(path.join(scratch, top, 'node.exe'), path.join(stage, 'node.exe'));
    } else {
      execFileSync(tarProgram, ['-xzf', archivePath, '-C', scratch, `${top}/bin/node`]);
      fs.copyFileSync(path.join(scratch, top, 'bin', 'node'), path.join(stage, 'node'));
      fs.chmodSync(path.join(stage, 'node'), 0o755);
    }
  } finally {
    fs.rmSync(scratch, { recursive: true, force: true });
  }
}

// Symlinks are not packaged (Go's extractor refuses them); npm's .bin links
// are the only ones an --ignore-scripts install makes, and nothing runs them.
function dropBinLinks(dir) {
  for (const entry of fs.readdirSync(dir, { withFileTypes: true })) {
    const full = path.join(dir, entry.name);
    if (entry.isSymbolicLink()) {
      if (path.basename(dir) !== '.bin') fail(`unexpected symlink ${path.relative(serviceDir, full)}`);
      fs.rmSync(full);
    } else if (entry.isDirectory()) {
      dropBinLinks(full);
    }
  }
}

// Packing. Windows Defender scans every file the host unpacks, and 7,000 small
// package files made the first start after an install or update take a minute.
// So the installed app/ is bundled into two scripts plus the files its code
// reads by path, each kept at its original relative path.

// Bundled modules that name __dirname, __filename or import.meta.url, each
// reviewed for what it reads, with how many times it names them. The plugin
// gives each one its original folder, so those reads land on the files
// DATA_FILES ships. A module that is not listed, or whose count changed (a
// package update can add a worker file or a template), fails the build until
// someone checks what it reads now and updates the entry (and DATA_FILES).
const PATH_READERS = new Map([
  ['shell.cjs', { refs: 2, reads: 'workspace-manifest.json' }],
  ['local/src/main/server.js', { refs: 6, reads: 'templates/, assets/, local/assets/, offline-runtime/<v>/src/core/page-format.js' }],
  ['local/src/main/runtime-assets.js', { refs: 1, reads: 'offline-runtime/manifest.json and <v>/offline-manifest.json' }],
  ['local/src/main/workspace/commands.js', { refs: 1, reads: 'document-templates/' }],
  ['local/src/main/workspace/shell-server.js', { refs: 1, reads: 'document-templates/, the workspace local-app manifest' }],
  ['local/src/main/workspace/search-index.js', { refs: 1, reads: 'the search-worker.js bundle' }],
  ['node_modules/@panphora/clayjs/dist/server/core.cjs', { refs: 4, reads: 'page-compaction-worker.cjs' }],
  ['node_modules/@panphora/hyperclay-workspace/dist/server/index.cjs', { refs: 1, reads: 'anchors.js' }],
  ['node_modules/@panphora/hyperclay-workspace/dist/server/cheerio.cjs', { refs: 2, reads: 'nothing (stack traces only)' }],
  ['node_modules/undici/index.js', { refs: 3, reads: 'nothing (stack traces only)' }],
  ['node_modules/tailwind-hyperclay/index.js', { refs: 3, reads: 'nothing: only compileTailwind reads, and it refuses here (no-tailwind.cjs)' }],
]);

// Files and folders the bundled code reads by path, copied at their original
// relative paths. The ClayJS browser runtime under offline-runtime/<v>/ is not
// shipped: the Go host answers /_/clayjs/ itself and never forwards it here.
const DATA_FILES = [
  'package.json',
  'BUILD.json',
  'workspace-manifest.json',
  'local/assets',
  'local/src/main/assets',
  'local/src/main/templates',
  'local/src/main/document-templates',
  'local/src/main/offline-runtime/manifest.json',
  'node_modules/@panphora/hyperclay-workspace/package.json',
  'node_modules/@panphora/hyperclay-workspace/dist/local-app/manifest.json',
  'node_modules/@panphora/hyperclay-workspace/dist/server/anchors.js',
  'node_modules/@panphora/clayjs/dist/server/page-compaction-worker.cjs',
];

// The one warning left: shell-server.js resolves the workspace manifest as a
// default that HTML Clay never uses (shell.cjs passes the manifest in); the
// file ships anyway, so the path stays valid.
const ALLOWED_WARNING = '@panphora/hyperclay-workspace/local-app/manifest.json';
const JSDOM_SYNC_XHR = 'require.resolve ? require.resolve("./xhr-sync-worker.js") : null';
const MAX_PACKED_FILES = 100;

async function loadEsbuild() {
  try {
    return await import('esbuild');
  } catch {
    fail('esbuild is missing: run npm ci at the repo root');
  }
}

async function packApp(installed, packed) {
  const esbuild = await loadEsbuild();
  // esbuild reports real, long paths: the temp folder is a symlink on macOS, and on
  // Windows %TEMP% is often an 8.3 short path (C:\Users\RUNNER~1\...). The native
  // resolver expands both; fs.realpathSync expands neither on Windows.
  const app = fs.realpathSync.native(installed);
  const rel = (file) => path.relative(app, file).split(path.sep).join('/');
  const unreviewed = new Set();
  let jsdomPatched = false;
  const ownDirname = {
    name: 'own-dirname',
    setup(build) {
      build.onLoad({ filter: /\.(c?js|mjs)$/ }, async (loaded) => {
        let contents = await fs.promises.readFile(loaded.path, 'utf8');
        const name = rel(loaded.path);
        if (name.endsWith('jsdom/lib/jsdom/living/xhr/XMLHttpRequest-impl.js')) {
          if (!contents.includes(JSDOM_SYNC_XHR)) fail(`${name} no longer has the sync XHR worker lookup this build removes`);
          // The service's jsdom never runs page scripts, so synchronous XHR and its worker file are never used.
          contents = contents.replace(JSDOM_SYNC_XHR, 'null');
          jsdomPatched = true;
        }
        const refs = (contents.match(/\b__(dirname|filename)\b|import\.meta\.url/g) || []).length;
        if (refs > 0) {
          const reviewed = PATH_READERS.get(name);
          if (!reviewed) unreviewed.add(`${name} (names its own path ${refs} times; not listed)`);
          else if (reviewed.refs !== refs) unreviewed.add(`${name} (names its own path ${refs} times; reviewed at ${reviewed.refs})`);
          // Every reviewed reader is CommonJS, so these land inside its own module wrapper.
          contents = `var __dirname = require("path").join(__serviceRoot, ${JSON.stringify(rel(path.dirname(loaded.path)))});\n` +
            `var __filename = require("path").join(__serviceRoot, ${JSON.stringify(name)});\n${contents}`;
        }
        return { contents, loader: 'js' };
      });
    },
  };

  // tailwind-hyperclay requires tailwindcss as it loads, but this program uses
  // only its link helpers: Go compiles every stylesheet. So the bundle gets a
  // stand-in that refuses to compile, and neither tailwindcss nor its plugins ship.
  const noTailwind = {
    name: 'no-tailwind',
    setup(build) {
      build.onResolve({ filter: /^tailwindcss$/ }, (args) => {
        const from = rel(args.importer);
        if (!from.startsWith('node_modules/tailwind-hyperclay/')) fail(`${from} requires tailwindcss, which the packed program does not carry`);
        return { path: path.join(app, 'no-tailwind.cjs') };
      });
    },
  };

  const inputs = new Set();
  const bundle = async (entry, rootExpr) => {
    const result = await esbuild.build({
      entryPoints: [path.join(app, entry)],
      outfile: path.join(packed, entry),
      bundle: true,
      platform: 'node',
      target: 'node24',
      format: 'cjs',
      external: ['electron'],
      banner: { js: `const __serviceRoot = ${rootExpr};` },
      plugins: [noTailwind, ownDirname],
      logLevel: 'silent',
      metafile: true,
      legalComments: 'eof',
      // Paths in the bundle's module comments stay relative to app/, not the build machine's temp folder.
      absWorkingDir: app,
    });
    const unexpected = result.warnings.filter((w) => !w.text.includes(ALLOWED_WARNING));
    if (unexpected.length > 0) {
      const text = await esbuild.formatMessages(unexpected, { kind: 'warning' });
      fail(`esbuild warned while bundling ${entry}:\n${text.join('\n')}`);
    }
    for (const input of Object.keys(result.metafile.inputs)) inputs.add(path.resolve(app, input));
  };
  await bundle('main.cjs', '__dirname');
  await bundle('local/src/main/workspace/search-worker.js', 'require("path").resolve(__dirname, "../../../..")');

  if (!jsdomPatched) fail('jsdom was not bundled; check the JSDOM_SYNC_XHR patch is still needed');
  if (unreviewed.size > 0) {
    fail(`these bundled modules read files relative to themselves and need a fresh review in PATH_READERS:\n  ${[...unreviewed].sort().join('\n  ')}`);
  }

  const runtimeDir = fs.readdirSync(path.join(app, 'local/src/main/offline-runtime'), { withFileTypes: true })
    .filter((e) => e.isDirectory()).map((e) => e.name);
  const data = [...DATA_FILES];
  for (const version of runtimeDir) {
    data.push(`local/src/main/offline-runtime/${version}/offline-manifest.json`,
      `local/src/main/offline-runtime/${version}/src/core/page-format.js`);
  }
  for (const item of data) {
    const from = path.join(app, item);
    if (!fs.existsSync(from)) fail(`the packed service needs ${item}, which the installed app does not have`);
    fs.cpSync(from, path.join(packed, item), { recursive: true });
  }

  fs.writeFileSync(path.join(packed, 'THIRD_PARTY_LICENSES.txt'), thirdPartyLicenses(app, inputs));
}

// thirdPartyLicenses gathers the license of every package bundled into the two
// scripts, since bundling drops the LICENSE files node_modules carried.
function thirdPartyLicenses(app, inputs) {
  const packages = new Map();
  for (const input of inputs) {
    const relPath = path.relative(app, input).split(path.sep).join('/');
    const match = relPath.match(/^(.*node_modules\/(?:@[^/]+\/)?[^/]+)\//);
    if (match && !packages.has(match[1])) packages.set(match[1], path.join(app, match[1]));
  }
  const sections = [];
  for (const [name, dir] of [...packages].sort(([a], [b]) => a.localeCompare(b))) {
    const meta = JSON.parse(fs.readFileSync(path.join(dir, 'package.json'), 'utf8'));
    const files = fs.readdirSync(dir).filter((f) => /^(licen[cs]e|copying|notice|third-party-notices)([.-]|$)/i.test(f)).sort();
    const text = files.length > 0
      ? files.map((f) => fs.readFileSync(path.join(dir, f), 'utf8').trim()).join('\n\n')
      : `License: ${meta.license || 'not stated'}`;
    sections.push(`${meta.name}@${meta.version}\n\n${text}`);
  }
  return `${sections.join('\n\n' + '-'.repeat(72) + '\n\n')}\n`;
}

function checkPackedTree(root) {
  const files = fs.readdirSync(root, { withFileTypes: true, recursive: true }).filter((e) => e.isFile());
  const native = files.filter((e) => e.name.endsWith('.node'));
  if (native.length > 0) fail(`the packed service carries native addons: ${native.map((e) => e.name).join(', ')}`);
  if (files.length > MAX_PACKED_FILES) fail(`the packed service has ${files.length} files, more than ${MAX_PACKED_FILES}; Windows scans each one at first start`);
  console.log(`service/build: packed into ${files.length} files`);
}

async function main() {
  const { goos, goarch, nodeArchive: given } = args();
  const pins = JSON.parse(fs.readFileSync(path.join(serviceDir, 'node-pins.json'), 'utf8'));
  const entry = pins.archives[`${goos}/${goarch}`];
  if (!entry) fail(`no Node archive pinned for ${goos}/${goarch}`);

  const archive = await nodeArchive(pins, entry, given);
  if (sha256(archive) !== entry.sha256) fail(`${entry.name} does not match its pinned sha256`);

  const stage = fs.mkdtempSync(path.join(os.tmpdir(), 'htmlclay-service-'));
  try {
    const archivePath = path.join(stage, entry.name);
    fs.writeFileSync(archivePath, archive);
    const root = path.join(stage, 'root');
    const app = path.join(root, 'app');
    fs.mkdirSync(app, { recursive: true });
    extractNode(archivePath, entry, goos, root);
    fs.rmSync(archivePath);

    const sources = fs.readdirSync(serviceDir).filter((name) => name.endsWith('.cjs'));
    for (const name of [...sources, 'package.json', 'package-lock.json']) {
      fs.copyFileSync(path.join(serviceDir, name), path.join(app, name));
    }
    fs.cpSync(path.join(serviceDir, 'local'), path.join(app, 'local'), { recursive: true });
    fs.copyFileSync(path.join(repo, 'internal', 'dashboard', 'assets', 'local-app', 'manifest.json'),
      path.join(app, 'workspace-manifest.json'));
    // On Windows npm is npm.cmd, which Node only starts through a shell.
    execFileSync(onWindows ? 'npm.cmd' : 'npm', ['ci', '--omit=optional', '--omit=dev', '--ignore-scripts', '--no-audit', '--no-fund'], {
      cwd: app, stdio: 'inherit', shell: onWindows, env: { ...process.env, npm_config_min_release_age: '0' },
    });
    dropBinLinks(path.join(app, 'node_modules'));
    // On the staged copy, whose packages were just installed, so a build needs only the
    // repo root's npm ci and never service/node_modules.
    execFileSync(process.execPath, [path.join(serviceDir, 'check-closure.mjs'), app], { stdio: 'inherit', cwd: repo });
    fs.writeFileSync(path.join(app, 'BUILD.json'), `${JSON.stringify({ node: pins.version, goos, goarch }, null, 2)}\n`);

    const packed = path.join(stage, 'packed');
    await packApp(app, packed);
    fs.rmSync(app, { recursive: true, force: true });
    fs.renameSync(packed, app);
    checkPackedTree(root);

    const out = path.join(stage, 'service.tar.gz');
    execFileSync(tarProgram, ['-czf', out, '-C', root, '.'], { env: { ...process.env, COPYFILE_DISABLE: '1' } });
    const blob = fs.readFileSync(out);
    const blobs = path.join(repo, 'internal', 'service', 'blobs');
    fs.mkdirSync(blobs, { recursive: true });
    const base = path.join(blobs, `${goos}-${goarch}`);
    fs.writeFileSync(`${base}.tar.gz`, blob);
    fs.writeFileSync(`${base}.sha256`, `${sha256(blob)} ${blob.length}\n`);
    console.log(`service/build: ${path.relative(repo, base)}.tar.gz, ${(blob.length / 1e6).toFixed(1)} MB, Node ${pins.version}`);
  } finally {
    fs.rmSync(stage, { recursive: true, force: true });
  }
}

main().catch((error) => fail(error && error.stack ? error.stack : String(error)));
