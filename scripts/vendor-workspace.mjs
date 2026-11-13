#!/usr/bin/env node
// Copies the workspace's browser bundle (@panphora/hyperclay-workspace
// dist/local-app) out of a hyperclay-workspace git commit into
// internal/dashboard/assets/local-app, and pins every file by digest in
// internal/dashboard/assets/lock.json. Files are read from the commit, never
// the working tree, so uncommitted work cannot leak in.
//
// Usage: node scripts/vendor-workspace.mjs --workspace <checkout> [--ref <git ref>]
//        node scripts/vendor-workspace.mjs --check
import { execFileSync } from "node:child_process";
import { createHash } from "node:crypto";
import { mkdir, readFile, readdir, rm, writeFile } from "node:fs/promises";
import path from "node:path";
import { fileURLToPath } from "node:url";

const ROOT = path.resolve(fileURLToPath(new URL("..", import.meta.url)));
const ASSETS = path.join(ROOT, "internal", "dashboard", "assets");
const FILES = path.join(ASSETS, "local-app");
const LOCK_FILE = path.join(ASSETS, "lock.json");
const SOURCE_DIR = "dist/local-app";

const sha256 = (bytes) => createHash("sha256").update(bytes).digest("hex");

function fail(message) {
  console.error(`vendor-workspace: ${message}`);
  process.exit(1);
}

function git(checkout, args, encoding = "utf8") {
  return execFileSync("git", ["-C", checkout, ...args], { encoding, maxBuffer: 64 * 1024 * 1024 });
}

async function walk(dir, base = dir) {
  const out = [];
  for (const entry of await readdir(dir, { withFileTypes: true })) {
    const full = path.join(dir, entry.name);
    if (entry.isDirectory()) out.push(...(await walk(full, base)));
    else out.push(path.relative(base, full).split(path.sep).join("/"));
  }
  return out.sort();
}

async function vendor(checkout, ref) {
  const commit = git(checkout, ["rev-parse", `${ref}^{commit}`]).trim();
  const committedAt = git(checkout, ["show", "-s", "--format=%cI", commit]).trim();
  const pkg = JSON.parse(git(checkout, ["show", `${commit}:package.json`]));
  const names = git(checkout, ["ls-tree", "-r", "--name-only", commit, SOURCE_DIR]).split("\n").filter(Boolean);
  if (!names.includes(`${SOURCE_DIR}/manifest.json`)) fail(`${commit} has no ${SOURCE_DIR}/manifest.json`);
  await rm(FILES, { recursive: true, force: true });
  const files = {};
  for (const name of names) {
    const rel = name.slice(SOURCE_DIR.length + 1);
    const bytes = git(checkout, ["show", `${commit}:${name}`], "buffer");
    await mkdir(path.dirname(path.join(FILES, rel)), { recursive: true });
    await writeFile(path.join(FILES, rel), bytes);
    files[rel] = sha256(bytes);
  }
  const lock = { package: { name: pkg.name, version: pkg.version, ref, commit, committedAt }, files };
  await writeFile(LOCK_FILE, `${JSON.stringify(lock, null, 2)}\n`);
  console.log(`vendor-workspace: ${names.length} files from ${pkg.name} ${pkg.version} at ${commit.slice(0, 12)}`);
}

async function check() {
  const lock = JSON.parse(await readFile(LOCK_FILE, "utf8"));
  const present = await walk(FILES);
  for (const rel of present) {
    if (!(rel in lock.files)) fail(`${rel} is not in the lock`);
    if (sha256(await readFile(path.join(FILES, rel))) !== lock.files[rel]) fail(`${rel} does not match the lock`);
  }
  for (const rel of Object.keys(lock.files)) if (!present.includes(rel)) fail(`${rel} is missing`);
  console.log(`vendor-workspace: ${present.length} files match ${lock.package.commit}`);
}

const argv = process.argv.slice(2);
if (argv[0] === "--check") await check();
else {
  const at = (flag) => { const i = argv.indexOf(flag); return i < 0 ? undefined : argv[i + 1]; };
  const checkout = at("--workspace");
  if (!checkout) fail("usage: --workspace <hyperclay-workspace checkout> [--ref <git ref>], or --check");
  await vendor(path.resolve(checkout), at("--ref") ?? "main");
}
