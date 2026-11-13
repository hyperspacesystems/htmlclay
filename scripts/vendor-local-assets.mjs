#!/usr/bin/env node
// Copies the few Hyperclay Local static files the served-folder host answers with out
// of a Local git checkout into the Go binary's embedded asset tree, and pins them by
// digest in a lock. `--check` re-hashes the checked-in tree without touching anything,
// so CI can prove the vendored bytes still match the lock.
import { execFileSync } from "node:child_process";
import { createHash } from "node:crypto";
import { mkdir, readFile, readdir, rm, writeFile } from "node:fs/promises";
import path from "node:path";
import { fileURLToPath } from "node:url";

const ROOT = path.resolve(fileURLToPath(new URL("..", import.meta.url)));
const FILES = path.join(ROOT, "internal", "served", "assets", "files");
const LOCK_NAME = "served-assets.lock.json";
const LOCK_FILE = path.join(FILES, LOCK_NAME);
const PATHS = [
  "assets/favicon.ico",
  "assets/favicon.png",
  "assets/favicon.svg",
  "src/main/templates/directory-listing.css",
  "src/main/assets/sync-worker.js",
  "src/main/assets/sync-data-worker.js",
];
const COMMIT = /^[0-9a-f]{40}$/;

const sha256 = (bytes) => createHash("sha256").update(bytes).digest("hex");

function fail(message) {
  console.error(`vendor-local-assets: ${message}`);
  process.exit(1);
}

function git(local, args) {
  return execFileSync("git", ["-C", local, ...args], { maxBuffer: 64 * 1024 * 1024 });
}

function gitText(local, args) {
  return git(local, args).toString("utf8").trim();
}

async function walk(dir, prefix = "") {
  const found = [];
  for (const entry of await readdir(dir, { withFileTypes: true })) {
    const rel = prefix ? `${prefix}/${entry.name}` : entry.name;
    if (entry.isDirectory()) found.push(...(await walk(path.join(dir, entry.name), rel)));
    else found.push(rel);
  }
  return found;
}

async function readLock() {
  const lock = JSON.parse(await readFile(LOCK_FILE, "utf8"));
  if (!lock || typeof lock !== "object" || Array.isArray(lock)) throw new Error("the lock is not an object");
  if (typeof lock.local?.ref !== "string" || lock.local.ref === "") {
    throw new Error("lock.local.ref is not a non-empty string");
  }
  if (typeof lock.local?.commit !== "string" || !COMMIT.test(lock.local.commit)) {
    throw new Error(`lock.local.commit ${JSON.stringify(lock.local?.commit)} is not a full commit`);
  }
  if (typeof lock.local?.committedAt !== "string" || lock.local.committedAt === "") {
    throw new Error("lock.local.committedAt is not a non-empty string");
  }
  if (!lock.files || typeof lock.files !== "object" || Array.isArray(lock.files)) {
    throw new Error("lock.files is not an object");
  }
  if (!Object.keys(lock.files).length) throw new Error("lock.files lists no files");
  return lock;
}

async function vendor(local, ref) {
  const commit = gitText(local, ["rev-parse", `${ref}^{commit}`]);
  if (!COMMIT.test(commit)) throw new Error(`${ref} resolves to ${JSON.stringify(commit)}, not a commit`);
  const committedAt = gitText(local, ["show", "-s", "--format=%cI", commit]);
  if (committedAt === "") throw new Error(`${commit} has no commit time`);
  const contents = new Map();
  for (const name of PATHS) {
    const bytes = git(local, ["show", `${commit}:${name}`]);
    if (!bytes.length) throw new Error(`${commit}:${name} is empty`);
    contents.set(name, bytes);
  }
  await rm(FILES, { recursive: true, force: true });
  const files = {};
  for (const name of [...PATHS].sort()) {
    const bytes = contents.get(name);
    const target = path.join(FILES, ...name.split("/"));
    await mkdir(path.dirname(target), { recursive: true });
    await writeFile(target, bytes);
    files[name] = { sha256: sha256(bytes), bytes: bytes.length };
  }
  const lock = {
    local: { ref, commit, committedAt },
    files,
  };
  await writeFile(LOCK_FILE, `${JSON.stringify(lock, null, 2)}\n`);
  console.log(`vendor-local-assets: vendored ${PATHS.length} files from Local ${ref} ${commit.slice(0, 7)}`);
}

async function check() {
  const lock = await readLock();
  const listed = Object.keys(lock.files).sort();
  for (const name of listed) {
    let bytes;
    try {
      bytes = await readFile(path.join(FILES, ...name.split("/")));
    } catch (err) {
      return `${name}: ${err.message}`;
    }
    const digest = sha256(bytes);
    if (digest !== lock.files[name]?.sha256) {
      return `${name}: sha256 is ${digest}, the lock records ${lock.files[name]?.sha256}`;
    }
    if (bytes.length !== lock.files[name]?.bytes) {
      return `${name}: is ${bytes.length} bytes, the lock records ${lock.files[name]?.bytes}`;
    }
  }
  const known = new Set([...listed, LOCK_NAME]);
  for (const rel of await walk(FILES)) {
    if (!known.has(rel)) return `${rel}: the lock does not list it`;
  }
  console.log(`vendor-local-assets: ok (${listed.length} files, Local ${lock.local.ref} ${lock.local.commit.slice(0, 7)})`);
}

const USAGE = "usage: node scripts/vendor-local-assets.mjs --local <dir> --ref <ref> | --check";

async function main() {
  const argv = process.argv.slice(2);
  let local;
  let ref;
  let checkOnly = false;
  for (let i = 0; i < argv.length; i += 1) {
    const arg = argv[i];
    if (arg === "--local") {
      const value = argv[i + 1];
      if (!value || value.startsWith("--")) fail(`${arg} needs a directory`);
      local = path.resolve(value);
      i += 1;
    } else if (arg === "--ref") {
      const value = argv[i + 1];
      if (!value || value.startsWith("--")) fail(`${arg} needs a ref`);
      ref = value;
      i += 1;
    } else if (arg === "--check") {
      checkOnly = true;
    } else {
      fail(`unknown argument ${arg}\n${USAGE}`);
    }
  }
  if (checkOnly) {
    if (local !== undefined || ref !== undefined) fail(`--check takes no --local or --ref\n${USAGE}`);
    const problem = await check();
    if (problem) fail(problem);
    return;
  }
  if (local === undefined || ref === undefined) fail(USAGE);
  await vendor(local, ref);
}

main().catch((err) => fail(err.message));
