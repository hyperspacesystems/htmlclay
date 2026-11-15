#!/usr/bin/env node
// Copies the JS files Hyperclay Local's live-room service needs out of a Local
// git checkout into service/vendor/local, and pins them by digest in a lock.
// Everything is read from the commit, never the working tree, so a dirty Local
// checkout cannot leak into the fork. `--check` re-hashes the vendored tree and
// compares the recorded package set without touching anything.
import { execFileSync } from "node:child_process";
import { createHash } from "node:crypto";
import { mkdir, readFile, readdir, rm, writeFile } from "node:fs/promises";
import { builtinModules } from "node:module";
import path from "node:path";
import { fileURLToPath } from "node:url";

const ROOT = path.resolve(fileURLToPath(new URL("..", import.meta.url)));
const SERVICE = path.join(ROOT, "service");
const FILES = path.join(SERVICE, "vendor", "local");
const LOCK_FILE = path.join(SERVICE, "vendor", "local.lock.json");
const PACKAGE_FILE = path.join(SERVICE, "package.json");
const LOCAL_PACKAGE_LOCK = "package-lock.json";
const ENTRIES = [
  "trial/service.cjs",
  "trial/service-main.cjs",
  "trial/go-document-host.cjs",
  "trial/go-process.cjs",
  "trial/fs-guard.cjs",
  "src/main/workspace/shell-server.js",
  "src/main/sync-manager.js",
  "trial/sync-observer.cjs",
];
const COMMIT = /^[0-9a-f]{40}$/;
const REQUIRED = /(?:^|[^.\w$])(require|import)\s*\(\s*(['"])((?:[^'"\\]|\\.)*)\2\s*\)/g;
const REQUIRE_CALL = /(?:^|[^.\w$])(require|import)\s*\(/g;
const DIRNAME = /__dirname\s*,\s*(['"])((?:[^'"\\]|\\.)*)\1/g;
const BUILTINS = new Set(builtinModules.map((name) => name.replace(/^node:/, "")));

const sha256 = (bytes) => createHash("sha256").update(bytes).digest("hex");

function fail(message) {
  console.error(`vendor-local: ${message}`);
  process.exit(1);
}

function git(local, args) {
  return execFileSync("git", ["-C", local, ...args], {
    maxBuffer: 64 * 1024 * 1024,
    stdio: ["ignore", "pipe", "pipe"],
  });
}

function gitText(local, args) {
  return git(local, args).toString("utf8").trim();
}

function hasPath(local, commit, file) {
  try {
    git(local, ["cat-file", "-e", `${commit}:${file}`]);
    return true;
  } catch {
    return false;
  }
}

function isFile(local, commit, file) {
  try {
    return gitText(local, ["cat-file", "-t", `${commit}:${file}`]) === "blob";
  } catch {
    return false;
  }
}

function isDirectory(local, commit, file) {
  try {
    return gitText(local, ["cat-file", "-t", `${commit}:${file}`]) === "tree";
  } catch {
    return false;
  }
}

function join(...parts) {
  return path.posix.normalize(parts.filter(Boolean).join("/"));
}

function dirOf(file) {
  const dir = path.posix.dirname(file);
  return dir === "." ? "" : dir;
}

function packageName(specifier) {
  const parts = specifier.split("/");
  return specifier.startsWith("@") ? parts.slice(0, 2).join("/") : parts[0];
}

function resolveSpecifier(local, commit, from, specifier) {
  const base = join(dirOf(from), specifier);
  const candidates = [base, `${base}.js`, `${base}.cjs`, `${base}.json`, `${base}/index.js`];
  for (const candidate of candidates) {
    if (isFile(local, commit, candidate)) return candidate;
  }
  return null;
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
  if (!lock.packages || typeof lock.packages !== "object" || Array.isArray(lock.packages)) {
    throw new Error("lock.packages is not an object");
  }
  if (!lock.requiredBy || typeof lock.requiredBy !== "object" || Array.isArray(lock.requiredBy)) {
    throw new Error("lock.requiredBy is not an object");
  }
  if (!Array.isArray(lock.unresolved)) throw new Error("lock.unresolved is not an array");
  return lock;
}

function scan(file, text) {
  const requires = [];
  const unresolved = [];
  const dirnameLiterals = [];
  const lines = text.split("\n");
  for (let index = 0; index < lines.length; index += 1) {
    const line = lines[index];
    const matched = new Set();
    REQUIRED.lastIndex = 0;
    let match;
    while ((match = REQUIRED.exec(line)) !== null) {
      matched.add(match.index + match[0].indexOf(match[1]));
      requires.push({ specifier: match[3], line: index + 1, text: line.trim() });
    }
    REQUIRE_CALL.lastIndex = 0;
    while ((match = REQUIRE_CALL.exec(line)) !== null) {
      if (!matched.has(match.index + match[0].indexOf(match[1]))) {
        unresolved.push({ file, line: index + 1, text: line.trim() });
      }
    }
    DIRNAME.lastIndex = 0;
    while ((match = DIRNAME.exec(line)) !== null) {
      dirnameLiterals.push({ literal: match[2], line: index + 1, text: line.trim() });
    }
  }
  return { requires, unresolved, dirnameLiterals };
}

async function readPackageLock(local, commit) {
  const text = git(local, ["show", `${commit}:${LOCAL_PACKAGE_LOCK}`]).toString("utf8");
  const parsed = JSON.parse(text);
  if (!parsed || typeof parsed !== "object" || !parsed.packages) {
    throw new Error(`${commit}:${LOCAL_PACKAGE_LOCK} has no packages`);
  }
  return parsed.packages;
}

async function vendor(local, ref) {
  const commit = gitText(local, ["rev-parse", `${ref}^{commit}`]);
  if (!COMMIT.test(commit)) throw new Error(`${ref} resolves to ${JSON.stringify(commit)}, not a commit`);
  const committedAt = gitText(local, ["show", "-s", "--format=%cI", commit]);
  if (committedAt === "") throw new Error(`${commit} has no commit time`);

  const contents = new Map();
  const queue = [...ENTRIES];
  const unresolved = [];
  const packageRequires = new Map();
  const electronRequires = new Set();
  const seen = new Set();

  while (queue.length) {
    const file = queue.shift();
    if (seen.has(file)) continue;
    seen.add(file);
    const bytes = git(local, ["show", `${commit}:${file}`]);
    contents.set(file, bytes);
    const text = bytes.toString("utf8");
    const found = scan(file, text);
    unresolved.push(...found.unresolved);
    for (const { specifier, line, text: source } of found.requires) {
      if (/^[a-zA-Z][a-zA-Z0-9+.-]*:\/\//.test(specifier) || specifier.startsWith("data:")) {
        unresolved.push({ file, line, text: source });
        continue;
      }
      if (specifier.startsWith(".")) {
        const resolved = resolveSpecifier(local, commit, file, specifier);
        if (resolved === null) {
          unresolved.push({ file, line, text: source });
          continue;
        }
        queue.push(resolved);
        continue;
      }
      const name = packageName(specifier);
      if (specifier === "electron" || name === "electron") {
        electronRequires.add(file);
        continue;
      }
      if (BUILTINS.has(name.replace(/^node:/, ""))) continue;
      if (!packageRequires.has(name)) packageRequires.set(name, new Set());
      packageRequires.get(name).add(file);
    }
    for (const { literal, line, text: source } of found.dirnameLiterals) {
      const target = join(dirOf(file), literal);
      if (isFile(local, commit, target)) {
        queue.push(target);
      } else if (isDirectory(local, commit, target)) {
        const listed = gitText(local, ["ls-tree", "-r", "--name-only", commit, target]);
        for (const child of listed.split("\n").filter(Boolean)) queue.push(child);
      } else {
        unresolved.push({ file, line, text: source });
      }
    }
  }

  const packageLock = await readPackageLock(local, commit);
  const packages = {};
  const requiredBy = {};
  for (const name of [...packageRequires.keys()].sort()) {
    const version = packageLock[`node_modules/${name}`]?.version;
    if (typeof version !== "string" || version === "") {
      throw new Error(`${commit}:${LOCAL_PACKAGE_LOCK} has no version for package ${name} (required by ${[...packageRequires.get(name)].sort().join(", ")})`);
    }
    packages[name] = version;
    requiredBy[name] = [...packageRequires.get(name)].sort();
  }

  const files = {};
  await rm(FILES, { recursive: true, force: true });
  for (const file of [...contents.keys()].sort()) {
    const bytes = contents.get(file);
    const target = path.join(FILES, ...file.split("/"));
    await mkdir(path.dirname(target), { recursive: true });
    await writeFile(target, bytes);
    files[file] = sha256(bytes);
  }

  const lock = {
    local: { ref, commit, committedAt },
    entries: ENTRIES,
    files,
    packages,
    requiredBy,
    unresolved,
  };
  await mkdir(path.dirname(LOCK_FILE), { recursive: true });
  await writeFile(LOCK_FILE, `${JSON.stringify(lock, null, 2)}\n`);

  const pkg = { name: "htmlclay-service", private: true, type: "commonjs", dependencies: packages };
  await writeFile(PACKAGE_FILE, `${JSON.stringify(pkg, null, 2)}\n`);

  if (electronRequires.size) {
    console.log(`vendor-local: electron is required by ${[...electronRequires].sort().join(", ")}`);
  }
  for (const item of unresolved) {
    console.log(`vendor-local: unresolved ${item.file}:${item.line}: ${item.text}`);
  }
  console.log(
    `vendor-local: vendored ${Object.keys(files).length} files and ${Object.keys(packages).length} packages from Local ${ref} ${commit.slice(0, 7)}`,
  );
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
    if (digest !== lock.files[name]) {
      return `${name}: sha256 is ${digest}, the lock records ${lock.files[name]}`;
    }
  }
  for (const rel of await walk(FILES)) {
    if (!Object.prototype.hasOwnProperty.call(lock.files, rel)) return `${rel}: the lock does not list it`;
  }
  let pkg;
  try {
    pkg = JSON.parse(await readFile(PACKAGE_FILE, "utf8"));
  } catch (err) {
    return `service/package.json: ${err.message}`;
  }
  const actual = pkg.dependencies ?? {};
  const expected = lock.packages;
  for (const name of new Set([...Object.keys(actual), ...Object.keys(expected)])) {
    if (actual[name] !== expected[name]) {
      return `service/package.json: ${name} is ${JSON.stringify(actual[name])}, the lock records ${JSON.stringify(expected[name])}`;
    }
  }
  console.log(`vendor-local: ${listed.length} files match ${lock.local.commit}`);
}

const USAGE = "usage: node scripts/vendor-local.mjs --local <dir> [--ref <ref>] | --check";

async function main() {
  const argv = process.argv.slice(2);
  let local;
  let ref = "main";
  let refSet = false;
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
      refSet = true;
      i += 1;
    } else if (arg === "--check") {
      checkOnly = true;
    } else {
      fail(`unknown argument ${arg}\n${USAGE}`);
    }
  }
  if (checkOnly) {
    if (local !== undefined || refSet) fail(`--check takes no --local or --ref\n${USAGE}`);
    const problem = await check();
    if (problem) fail(problem);
    return;
  }
  if (local === undefined) fail(USAGE);
  await vendor(local, ref);
}

main().catch((err) => fail(err.message));
