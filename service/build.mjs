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
      execFileSync('unzip', ['-q', archivePath, `${top}/node.exe`, '-d', scratch]);
      fs.copyFileSync(path.join(scratch, top, 'node.exe'), path.join(stage, 'node.exe'));
    } else {
      execFileSync('tar', ['-xzf', archivePath, '-C', scratch, `${top}/bin/node`]);
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

async function main() {
  const { goos, goarch, nodeArchive: given } = args();
  const pins = JSON.parse(fs.readFileSync(path.join(serviceDir, 'node-pins.json'), 'utf8'));
  const entry = pins.archives[`${goos}/${goarch}`];
  if (!entry) fail(`no Node archive pinned for ${goos}/${goarch}`);

  execFileSync(process.execPath, [path.join(repo, 'scripts', 'vendor-local.mjs'), '--check'], { stdio: 'inherit', cwd: repo });

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

    for (const name of ['main.cjs', 'package.json', 'package-lock.json']) {
      fs.copyFileSync(path.join(serviceDir, name), path.join(app, name));
    }
    fs.cpSync(path.join(serviceDir, 'vendor'), path.join(app, 'vendor'), { recursive: true });
    execFileSync('npm', ['ci', '--omit=optional', '--omit=dev', '--ignore-scripts', '--no-audit', '--no-fund'], {
      cwd: app, stdio: 'inherit', env: { ...process.env, npm_config_min_release_age: '0' },
    });
    dropBinLinks(path.join(app, 'node_modules'));
    fs.writeFileSync(path.join(app, 'BUILD.json'), `${JSON.stringify({ node: pins.version, goos, goarch }, null, 2)}\n`);

    const out = path.join(stage, 'service.tar.gz');
    execFileSync('tar', ['-czf', out, '-C', root, '.'], { env: { ...process.env, COPYFILE_DISABLE: '1' } });
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
