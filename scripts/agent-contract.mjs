// Step 7.43's contract run: the real clay-agent, as its CLI and as its stdio MCP
// server, drives the real host built from this checkout. One temporary home
// holds the host's config folder, a served folder (notes/) and a loose .htmlclay
// file the host has open, so both halves read the same config.json and
// agent-token. Every address the host would open in a browser goes to a log
// instead (HTMLCLAY_OPEN_LOG).
//
//   node scripts/agent-contract.mjs --cli "$TESTBOX_CLAY_AGENT/bin/clay-agent.js" --output agent-contract.json
//
// It refuses to run unless TESTBOX_CLAY_AGENT names a clay-agent package; the
// fork's .testbox packs one and sets it. Each case is a row
// {case, document, via, pass, detail} in the output. Prints `AGENT CONTRACT OK
// cases=<n>` and exits 0, or `AGENT CONTRACT FAILED ...` and exits 1.
//
// --wrong-token is the proof that the two halves ran against each other: once
// the host is up, the token file is replaced with another valid token, and every
// case that reaches the lane must fail with `unauthorized`. --bin names a host
// already built, in place of building this checkout's.
// Linux and macOS, and not as root, which writes into a read-only version store.
import { execFileSync, spawn } from 'node:child_process';
import { randomBytes } from 'node:crypto';
import {
  appendFileSync, chmodSync, copyFileSync, existsSync, mkdirSync, mkdtempSync, readdirSync, readFileSync, realpathSync, renameSync,
  rmSync, statSync, writeFileSync,
} from 'node:fs';
import http from 'node:http';
import os from 'node:os';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { isDeepStrictEqual } from 'node:util';
import { READY_TIMEOUT_MS, STOP_TIMEOUT_MS, freePort } from './served-product.mjs';

const repo = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
const fixtures = path.join(repo, 'scripts', 'agent-contract');
const tracer = path.join(fixtures, 'trace-connects.mjs');
const CALL_TIMEOUT_MS = 60000;

const FIXTURES = { served: path.join('notes', 'index.html'), loose: 'journal.htmlclay' };

function parseArgs(argv) {
  const options = { cli: null, output: null, bin: null, wrongToken: false };
  for (let i = 0; i < argv.length; i += 1) {
    const flag = argv[i];
    const value = () => {
      i += 1;
      if (i >= argv.length) throw new Error(`${flag} needs a value`);
      return path.resolve(argv[i]);
    };
    switch (flag) {
      case '--cli': options.cli = value(); break;
      case '--output': options.output = value(); break;
      case '--bin': options.bin = value(); break;
      case '--wrong-token': options.wrongToken = true; break;
      default: throw new Error(`unknown argument ${flag}`);
    }
  }
  return options;
}

const wait = (ms) => new Promise((resolve) => setTimeout(resolve, ms));
const exited = (child) => child.exitCode !== null || child.signalCode !== null;
const read = (file) => readFileSync(file, 'utf8');

function check(condition, message) {
  if (!condition) throw new Error(message);
}

function configBase(home) {
  return process.platform === 'darwin' ? path.join(home, 'Library', 'Application Support') : path.join(home, '.config');
}

// Both halves find the config folder from HOME alone, so nothing the caller's
// shell exports may point either of them somewhere else.
function environment(home, extra = {}) {
  const env = { ...process.env, HOME: home, ...extra };
  for (const name of ['XDG_CONFIG_HOME', 'HTMLCLAY_CONFIG_BASE', 'HTMLCLAY_FOLDER_LOCKS', 'HTMLCLAY_SERVICE_CMD', 'CLAY_AGENT_URL', 'CLAY_AGENT_KEY']) {
    delete env[name];
  }
  return env;
}

function build(out) {
  const bin = path.join(out, 'htmlclay');
  // The box builds without cgo; macOS needs it for the tray.
  const env = process.platform === 'linux' ? { ...process.env, CGO_ENABLED: '0' } : process.env;
  execFileSync('go', ['build', '-o', bin, './cmd/htmlclay'], { cwd: repo, stdio: 'inherit', env });
  return bin;
}

function writeConfig(dir, port, notes) {
  mkdirSync(dir, { recursive: true });
  // The config names the port and turns updates off, so this launch neither asks
  // which port to use nor downloads an update.
  const config = {
    startOnLogin: false,
    port,
    updates: 'off',
    servedFolders: [{ id: randomBytes(8).toString('hex'), name: 'notes', path: notes, identity: '', serving: true }],
  };
  writeFileSync(path.join(dir, 'config.json'), `${JSON.stringify(config, null, 2)}\n`);
}

// The one temporary home: the config folder where the platform puts it, the
// served folder, the loose file, a file nothing holds, and beside the home a
// file no program may reach through this host.
function seed(root, port) {
  const home = path.join(root, 'home');
  const configDir = path.join(configBase(home), 'htmlclay');
  const notes = path.join(home, 'notes');
  const place = (file, fixture) => {
    mkdirSync(path.dirname(file), { recursive: true });
    copyFileSync(path.join(fixtures, fixture), file);
    return file;
  };
  const world = {
    root,
    home,
    configDir,
    port,
    notes,
    openLog: path.join(root, 'open.log'),
    hostLog: path.join(root, 'host.log'),
    unopened: place(path.join(home, 'shelf', 'unopened.htmlclay'), FIXTURES.loose),
    outside: place(path.join(root, 'outside.htmlclay'), FIXTURES.loose),
    documents: {
      served: {
        name: 'served',
        file: place(path.join(notes, 'index.html'), FIXTURES.served),
        address: `http://notes.htmlclay.localhost:${port}/index.html`,
        versions: path.join(notes, '.hyperclay', 'versions'),
      },
      loose: {
        name: 'loose',
        file: place(path.join(home, 'desk', 'journal.htmlclay'), FIXTURES.loose),
        versions: path.join(configDir, 'versions'),
      },
    },
  };
  world.documents.loose.address = world.documents.loose.file;
  writeConfig(configDir, port, notes);
  return world;
}

function tail(file, lines = 40) {
  try {
    return read(file).trimEnd().split('\n').slice(-lines).join('\n');
  } catch {
    return '';
  }
}

// The host as the Mac app starts it: the secret on stdin, the READY line on
// stdout, and stdin held open, because its end is what shuts the host down.
async function startHost(bin, world) {
  const secret = randomBytes(32).toString('hex');
  const child = spawn(bin, ['--shell-stdin'], {
    cwd: world.root,
    env: environment(world.home, { HTMLCLAY_OPEN_LOG: world.openLog }),
    stdio: ['pipe', 'pipe', 'pipe'],
  });
  const gone = new Promise((resolve) => child.once('exit', (code, signal) => resolve({ code, signal })));
  child.stdin.on('error', () => {});
  child.stdin.write(`${JSON.stringify({ v: 1, secret })}\n`);

  let scan = '';
  const ready = await new Promise((resolve, reject) => {
    const timer = setTimeout(() => {
      reject(new Error(`the host printed no READY line within ${READY_TIMEOUT_MS}ms; its last lines:\n${tail(world.hostLog)}`));
    }, READY_TIMEOUT_MS);
    const finish = (error, value) => {
      clearTimeout(timer);
      if (error) reject(error);
      else resolve(value);
    };
    child.stdout.on('data', (chunk) => {
      const text = chunk.toString('utf8');
      appendFileSync(world.hostLog, text);
      scan += text;
      const line = scan.split('\n').find((entry) => entry.startsWith('READY ') || entry.startsWith('FAILED '));
      if (!line || !scan.includes(`${line}\n`)) return;
      if (line.startsWith('FAILED ')) finish(new Error(`the host refused to start: ${line}`));
      else finish(null, JSON.parse(line.slice('READY '.length)));
    });
    child.stderr.on('data', (chunk) => appendFileSync(world.hostLog, chunk.toString('utf8')));
    gone.then(({ code, signal }) => finish(new Error(`the host exited before it was ready (${signal ?? `exit ${code}`}):\n${tail(world.hostLog)}`)));
  });

  const stop = async () => {
    if (exited(child)) return;
    child.stdin.end();
    if (await Promise.race([gone, wait(STOP_TIMEOUT_MS).then(() => null)])) return;
    child.kill('SIGKILL');
    await gone;
  };
  return { ready, secret, stop };
}

// One action on the host's private listener, the way the Mac app sends it. Not
// fetch: Node's fetch adds Sec-Fetch-Mode, and that listener refuses any request
// carrying a header a browser sends.
function shellAction(host, id, type, args) {
  const body = JSON.stringify({ id, type, args });
  return new Promise((resolve, reject) => {
    const request = http.request(`${host.ready.shell}/shell/v1/actions`, {
      method: 'POST',
      headers: { Authorization: `Bearer ${host.secret}`, 'Content-Type': 'application/json', 'Content-Length': Buffer.byteLength(body) },
      timeout: CALL_TIMEOUT_MS,
    }, (response) => {
      const chunks = [];
      response.on('data', (chunk) => chunks.push(chunk));
      response.on('end', () => resolve({ status: response.statusCode, body: Buffer.concat(chunks).toString('utf8') }));
    });
    request.on('timeout', () => request.destroy(new Error(`${type} got no answer`)));
    request.on('error', reject);
    request.end(body);
  });
}

function connections(trace) {
  try {
    return read(trace).split('\n').filter(Boolean).length;
  } catch {
    return 0;
  }
}

// What one call through clay-agent answered, in the shape both front doors
// share: the result, or the refusal's code, status and etag. Each call is also
// noted for the row of the case that made it.
function answerOf(agent, run, { result = null, error = null, exit = null, connects }) {
  const answer = { run, ok: error === null && result !== null, connects };
  const note = { run, connects };
  if (exit !== null) answer.exit = note.exit = exit;
  if (answer.ok) answer.result = result;
  else {
    answer.code = note.code = error?.code ?? 'no-answer';
    answer.status = error?.status ?? null;
    answer.etag = error?.etag ?? null;
    answer.message = error?.message ?? null;
    if (answer.status !== null) note.status = answer.status;
  }
  agent.calls.push(note);
  return answer;
}

function parseJson(text) {
  try {
    return JSON.parse(text);
  } catch {
    return null;
  }
}

// One run of the real CLI, always with --json: the result on stdout, a refusal
// on stderr, and the exit code that names it.
function runCli(agent, args, input = '') {
  const trace = path.join(agent.traces, `cli-${agent.nextTrace()}.trace`);
  return new Promise((resolve, reject) => {
    const child = spawn(process.execPath, ['--import', tracer, agent.cli, ...args, '--json'], {
      cwd: agent.cwd,
      env: { ...agent.env, AGENT_CONTRACT_TRACE: trace },
      stdio: ['pipe', 'pipe', 'pipe'],
    });
    const out = [];
    const err = [];
    const timer = setTimeout(() => child.kill('SIGKILL'), CALL_TIMEOUT_MS);
    child.stdout.on('data', (chunk) => out.push(chunk));
    child.stderr.on('data', (chunk) => err.push(chunk));
    child.on('error', reject);
    child.on('close', (code) => {
      clearTimeout(timer);
      const stdout = Buffer.concat(out).toString('utf8');
      const stderr = Buffer.concat(err).toString('utf8');
      const failure = code === 0 ? null : parseJson(stderr)?.error ?? { code: 'no-answer', message: stderr.trim() || `exit ${code}` };
      resolve(answerOf(agent, args[0], { result: code === 0 ? parseJson(stdout) : null, error: failure, exit: code, connects: connections(trace) }));
    });
    child.stdin.on('error', () => {});
    child.stdin.end(input);
  });
}

function cliDriver(agent) {
  const matching = (etag) => (etag === undefined ? [] : ['--if-match', etag]);
  return {
    via: 'cli',
    session: false,
    read: (address) => runCli(agent, ['read', address]),
    data: (address) => runCli(agent, ['data', address]),
    writeData: (address, data, etag) => runCli(agent, ['data', address, '--write', JSON.stringify(data), ...matching(etag)]),
    save: (address, html, etag) => runCli(agent, ['save', address, '--file', '-', ...matching(etag)], html),
    versions: (address) => runCli(agent, ['versions', address]),
    version: (address, id) => runCli(agent, ['version', address, id]),
    restore: (address, id, etag) => runCli(agent, ['restore', address, id, ...matching(etag)]),
    list: () => runCli(agent, ['list', '--host', 'htmlclay']),
    fresh: async (use) => use(cliDriver(agent)),
    stop: async () => {},
  };
}

// The stdio MCP server from the same package, spoken to in newline-framed
// JSON-RPC: initialize, notifications/initialized, then tools/call.
async function startMcp(agent) {
  const trace = path.join(agent.traces, `mcp-${agent.nextTrace()}.trace`);
  const child = spawn(process.execPath, ['--import', tracer, agent.cli, 'mcp'], {
    cwd: agent.cwd,
    env: { ...agent.env, AGENT_CONTRACT_TRACE: trace },
    stdio: ['pipe', 'pipe', 'pipe'],
  });
  const gone = new Promise((resolve) => child.once('exit', resolve));
  const waiting = new Map();
  let stderr = '';
  let buffered = '';
  let nextId = 1;
  child.stdin.on('error', () => {});
  child.stderr.on('data', (chunk) => { stderr = (stderr + chunk.toString('utf8')).slice(-4096); });
  child.stdout.on('data', (chunk) => {
    buffered += chunk.toString('utf8');
    for (let end = buffered.indexOf('\n'); end !== -1; end = buffered.indexOf('\n')) {
      const message = parseJson(buffered.slice(0, end));
      buffered = buffered.slice(end + 1);
      const settle = message && waiting.get(message.id);
      if (!settle) continue;
      waiting.delete(message.id);
      settle(message);
    }
  });
  gone.then(() => {
    for (const settle of waiting.values()) settle({ error: { message: `the MCP server exited: ${stderr.trim()}` } });
    waiting.clear();
  });

  const send = (message) => child.stdin.write(`${JSON.stringify({ jsonrpc: '2.0', ...message })}\n`);
  const request = (method, params) => new Promise((resolve, reject) => {
    const id = nextId;
    nextId += 1;
    const timer = setTimeout(() => {
      waiting.delete(id);
      reject(new Error(`the MCP server did not answer ${method} within ${CALL_TIMEOUT_MS}ms`));
    }, CALL_TIMEOUT_MS);
    waiting.set(id, (message) => {
      clearTimeout(timer);
      if (message.error) reject(new Error(`${method}: ${message.error.message}`));
      else resolve(message.result);
    });
    send({ id, method, params });
  });

  await request('initialize', {
    protocolVersion: '2025-06-18',
    capabilities: {},
    clientInfo: { name: 'agent-contract', version: '1' },
  });
  send({ method: 'notifications/initialized' });

  const call = async (name, args) => {
    const before = connections(trace);
    const answer = await request('tools/call', { name, arguments: args });
    const content = answer.structuredContent ?? null;
    return answerOf(agent, name, {
      result: answer.isError ? null : content,
      error: answer.isError ? content?.error ?? { code: 'no-answer' } : null,
      connects: connections(trace) - before,
    });
  };
  const stop = async () => {
    if (exited(child)) return;
    child.stdin.end();
    if (await Promise.race([gone.then(() => true), wait(3000).then(() => false)])) return;
    child.kill('SIGKILL');
    await gone;
  };
  return { call, stop };
}

// An etag left out means "the one this session last read", which is how an MCP
// client is told to write.
async function mcpDriver(agent) {
  const server = await startMcp(agent);
  return {
    via: 'mcp',
    session: true,
    read: (document) => server.call('read_document', { document }),
    data: (document) => server.call('read_data', { document }),
    writeData: (document, data, etag) => server.call('write_data', { document, data, etag }),
    save: (document, html, etag) => server.call('save_document', { document, html, etag }),
    versions: (document) => server.call('list_versions', { document }),
    version: (document, version) => server.call('read_version', { document, version }),
    restore: (document, version, etag) => server.call('restore_version', { document, version, etag }),
    list: () => server.call('list_documents', { host: 'htmlclay' }),
    fresh: async (use) => {
      const other = await mcpDriver(agent);
      try {
        return await use(other);
      } finally {
        await other.stop();
      }
    },
    stop: server.stop,
  };
}

// Every folder under root made read-only, so the next version cannot be stored.
// Answers the function that puts the modes back.
function lockFolders(root) {
  mkdirSync(root, { recursive: true });
  const modes = [];
  const visit = (dir) => {
    for (const entry of readdirSync(dir, { withFileTypes: true })) {
      if (entry.isDirectory()) visit(path.join(dir, entry.name));
    }
    modes.push([dir, statSync(dir).mode & 0o777]);
    chmodSync(dir, 0o555);
  };
  visit(root);
  return () => {
    for (const [dir, mode] of modes) chmodSync(dir, mode);
  };
}

// How many version files a store holds on disk.
function keptFiles(root) {
  let count = 0;
  for (const entry of readdirSync(root, { withFileTypes: true })) {
    if (entry.isDirectory()) count += keptFiles(path.join(root, entry.name));
    else if (entry.name.endsWith('.html')) count += 1;
  }
  return count;
}

// A change made by another program: the whole file swapped in one step, as an
// editor saves, so the host never sees it half written.
function editOutside(world, file, text) {
  const staged = path.join(world.root, 'edited-outside.tmp');
  writeFileSync(staged, text);
  renameSync(staged, file);
}

// lossyProxy stands between one MCP server and the host. It forwards every
// request with the Host the lane admits, and on request drops one write's answer
// after the host gave it: the write happened and the program never hears so.
async function startLossyProxy(port) {
  const writes = [];
  let dropNext = false;
  const server = http.createServer((request, response) => {
    const chunks = [];
    request.on('data', (chunk) => chunks.push(chunk));
    request.on('end', () => {
      const body = Buffer.concat(chunks);
      const headers = { ...request.headers, host: `127.0.0.1:${port}` };
      delete headers.connection;
      const upstream = http.request({ host: '127.0.0.1', port, method: request.method, path: request.url, headers, agent: false }, (answer) => {
        const parts = [];
        answer.on('data', (chunk) => parts.push(chunk));
        answer.on('end', () => {
          const answered = Buffer.concat(parts);
          const write = request.method !== 'GET';
          const dropped = write && dropNext;
          if (write) {
            dropNext = false;
            writes.push({
              method: request.method,
              operationId: parseJson(body.toString('utf8'))?.operationId ?? null,
              status: answer.statusCode,
              answer: answered.toString('utf8'),
              dropped,
            });
          }
          if (dropped) {
            request.socket.destroy();
            return;
          }
          response.writeHead(answer.statusCode, { 'content-type': answer.headers['content-type'] ?? 'application/json', 'content-length': answered.length });
          response.end(answered);
        });
      });
      upstream.on('error', () => request.socket.destroy());
      upstream.end(body);
    });
  });
  await new Promise((resolve) => server.listen(0, '127.0.0.1', resolve));
  return {
    port: server.address().port,
    writes,
    dropNextAnswer: () => { dropNext = true; },
    close: () => new Promise((resolve) => {
      server.closeAllConnections();
      server.close(resolve);
    }),
  };
}

const edited = (html, marker) => html.replace(/<p class="note">[^<]*<\/p>/, `<p class="note">${marker}</p>`);

// The data a fixture document publishes under its api rules, read from its HTML
// here so a case can hold the lane's answer against the file.
const dataOf = (html) => ({
  title: /<h1>([^<]*)<\/h1>/.exec(html)?.[1],
  note: /<p class="note">([^<]*)<\/p>/.exec(html)?.[1],
  items: [...html.matchAll(/<span class="name">([^<]*)<\/span>/g)].map(([, name]) => ({ name })),
});

// The id of the newest kept version holding exactly html, or null.
async function versionHolding(driver, address, html) {
  for (const row of expectOk(await driver.versions(address)).versions) {
    if (expectOk(await driver.version(address, row.id)).html === html) return row.id;
  }
  return null;
}

function expectOk(answer) {
  check(answer.ok, `${answer.run} was refused: ${answer.code}${answer.status ? ` (${answer.status})` : ''} ${answer.message ?? ''}`.trim());
  if (answer.exit !== undefined) check(answer.exit === 0, `${answer.run} exited ${answer.exit}`);
  return answer.result;
}

function expectRefusal(answer, code, status, exit) {
  check(!answer.ok, `${answer.run} was not refused`);
  check(answer.code === code, `${answer.run} answered ${answer.code}, not ${code}`);
  check(answer.status === status, `${answer.run} answered status ${answer.status}, not ${status}`);
  if (answer.exit !== undefined) check(answer.exit === exit, `${answer.run} exited ${answer.exit}, not ${exit}`);
}

// The cases every document runs through each front door, in order. Each one
// reads what it needs first, so one failure does not take the next case with it.
function lifecycle(driver, doc) {
  // The etag of this driver's own last read: a session already holds it.
  const own = (etag) => (driver.session ? undefined : etag);
  const disk = () => read(doc.file);
  const mark = (label) => `${label} by ${driver.via}`;
  let replaced = null;

  return {
    async read() {
      // A process that has read nothing yet has to open a connection to read,
      // whatever it does with connections later.
      const answer = await driver.fresh((unused) => unused.read(doc.address));
      const result = expectOk(answer);
      check(answer.connects >= 1, 'the connection trace saw nothing, so it cannot vouch for the cases that must connect nowhere');
      check(result.host === 'htmlclay' && result.ref === doc.file, `the read names ${result.host} ${result.ref}`);
      check(result.html === disk(), 'the read is not the bytes on disk');
      check(typeof result.etag === 'string' && result.etag !== '', 'the read carries no etag');
      return { etag: result.etag, size: result.size };
    },

    async 'read data'() {
      const result = expectOk(await driver.data(doc.address));
      check(isDeepStrictEqual(result.data, dataOf(disk())), `the data is ${JSON.stringify(result.data)}`);
      check(result.data.items.length === 2, `the data holds ${result.data.items.length} items, and the document has 2`);
      check(typeof result.etag === 'string' && result.etag !== '', 'the data carries no etag');
      return { etag: result.etag, data: result.data };
    },

    async 'write data'() {
      const before = expectOk(await driver.data(doc.address));
      const title = mark('Renamed');
      const written = expectOk(await driver.writeData(doc.address, { title }, own(before.etag)));
      check(written.changed === true, 'the write says nothing changed');
      const after = expectOk(await driver.data(doc.address));
      check(isDeepStrictEqual(after.data, { ...before.data, title }), `the data is now ${JSON.stringify(after.data)}`);
      check(after.etag === written.etag && after.etag !== before.etag, 'the write did not answer the etag the document now has');
      check(disk().includes(`<h1>${title}</h1>`), 'the file does not hold the new title');
      return { etag: written.etag };
    },

    async 'whole save'() {
      const before = expectOk(await driver.read(doc.address));
      const html = edited(before.html, mark('saved'));
      const saved = expectOk(await driver.save(doc.address, html, own(before.etag)));
      check(saved.changed === true, 'the save says nothing changed');
      check(typeof saved.undoVersionId === 'string' && saved.undoVersionId !== '', 'the save names no version to undo to');
      check(disk() === html, 'the file is not the saved bytes');
      const after = expectOk(await driver.read(doc.address));
      check(after.etag === saved.etag && after.html === html, 'a read after the save does not answer the save');
      replaced = { html: before.html, version: saved.undoVersionId };
      return { etag: saved.etag, undoVersionId: saved.undoVersionId };
    },

    async versions() {
      const listed = expectOk(await driver.versions(doc.address));
      check(replaced, 'the whole save kept no version to look for');
      const row = listed.versions.find((version) => version.id === replaced.version);
      check(row, `the list does not hold ${replaced.version}`);
      check(typeof row.createdAt === 'string' && Number.isInteger(row.size), `the version row is ${JSON.stringify(row)}`);
      const one = expectOk(await driver.version(doc.address, row.id));
      check(one.html === replaced.html, 'the version is not the bytes the save replaced');
      return { count: listed.versions.length, version: row };
    },

    async restore() {
      const before = expectOk(await driver.read(doc.address));
      check(replaced, 'the whole save kept no version to restore');
      const restored = expectOk(await driver.restore(doc.address, replaced.version, own(before.etag)));
      check(disk() === replaced.html, 'the file is not the restored version');
      const after = expectOk(await driver.read(doc.address));
      check(after.etag === restored.etag, 'the restore did not answer the etag the document now has');
      // clay-agent's restore answer names no undo version, so the kept bytes are
      // looked for in the list.
      const undo = await versionHolding(driver, doc.address, before.html);
      check(undo, 'no kept version holds the bytes the restore replaced');
      return { etag: restored.etag, undoVersionId: undo };
    },

    async 'stale save'() {
      const first = expectOk(await driver.read(doc.address));
      const moved = expectOk(await driver.save(doc.address, edited(first.html, mark('moved on')), own(first.etag)));
      const before = disk();
      const stale = await driver.save(doc.address, edited(first.html, mark('stale')), first.etag);
      expectRefusal(stale, 'conflict', 412, 2);
      check(stale.etag === moved.etag, `the refusal names etag ${stale.etag}, and the document has ${moved.etag}`);
      check(disk() === before, 'the refused save changed the file');
      return { status: stale.status, code: stale.code };
    },

    async 'data write with no read'() {
      const before = disk();
      const removal = { items: [dataOf(before).items[0]] };
      const blind = await driver.fresh((unread) => unread.writeData(doc.address, removal, undefined));
      expectRefusal(blind, 'read-first', null, 1);
      check(blind.connects === 0, `clay-agent connected ${blind.connects} times for a write with no etag`);
      const guessed = await driver.writeData(doc.address, removal, '0'.repeat(16));
      const answered = `${guessed.status} ${guessed.code}`;
      check(!guessed.ok && ['412 conflict', '428 read-required'].includes(answered), `the lane answered ${answered}`);
      check(disk() === before, 'the refused write changed the file');
      return { lane: answered, withoutEtag: blind.code };
    },

    async 'version that cannot be kept'() {
      check(process.getuid?.() !== 0, 'root writes into a read-only folder, so this case cannot run as root');
      // Every earlier save left its bytes in history, and a save keeps nothing
      // new for bytes already there. A change from another program, made once
      // the store is read-only, is bytes no version holds.
      const outside = edited(disk(), mark('edited outside'));
      const html = edited(outside, mark('kept nowhere'));
      const unlock = lockFolders(doc.versions);
      let before;
      let refused;
      try {
        const kept = keptFiles(doc.versions);
        editOutside(doc.world, doc.file, outside);
        before = expectOk(await driver.read(doc.address));
        check(before.html === outside, 'the read is not the change made outside');
        refused = await driver.save(doc.address, html, own(before.etag));
        check(disk() === outside, 'the refused save changed the file');
        check(keptFiles(doc.versions) === kept, 'the store gained a version while it was read-only');
      } finally {
        unlock();
      }
      expectRefusal(refused, 'recovery-unavailable', 503, 1);
      const again = expectOk(await driver.save(doc.address, html, own(before.etag)));
      check(again.changed === true && disk() === html, 'the same save did not go through once a version could be kept');
      const undo = expectOk(await driver.version(doc.address, again.undoVersionId));
      check(undo.html === outside, 'the version kept once the store could be written is not the bytes the save replaced');
      return { status: refused.status, code: refused.code };
    },

    async 'not reachable'() {
      const before = read(doc.unopened);
      const answer = await driver.read(doc.unopened);
      expectRefusal(answer, 'not-reachable', 404, 1);
      const saved = await driver.save(doc.unopened, edited(before, mark('unreachable')), '0'.repeat(16));
      expectRefusal(saved, 'not-reachable', 404, 1);
      check(read(doc.unopened) === before, 'the refused save changed the file');
      return { status: answer.status, code: answer.code };
    },
  };
}

// A repeated save with one operation id. The CLI has no flag for an operation
// id and mints a new one in every process, so this goes through an MCP server,
// which keeps the id while the outcome is unknown, and a proxy that loses the
// first answer.
async function repeatedSave(agent, world, doc) {
  const proxy = await startLossyProxy(world.port);
  // The MCP server's home is the folder above the real one, so it finds this
  // config, which names the proxy's port, and every document is still inside it.
  const configDir = path.join(configBase(world.root), 'htmlclay');
  writeConfig(configDir, proxy.port, world.notes);
  copyFileSync(path.join(world.configDir, 'agent-token'), path.join(configDir, 'agent-token'));
  const driver = await mcpDriver({ ...agent, env: environment(world.root) });
  try {
    const before = expectOk(await driver.read(doc.file));
    const html = edited(before.html, 'saved twice');
    const versionIds = async () => expectOk(await driver.versions(doc.file)).versions.map((version) => version.id);
    const written = () => {
      const { ino, mtimeMs } = statSync(doc.file);
      return { ino, mtimeMs };
    };

    proxy.dropNextAnswer();
    const lost = await driver.save(doc.file, html, undefined);
    expectRefusal(lost, 'outcome-unknown', null, undefined);
    check(read(doc.file) === html, 'the save whose answer was lost is not on disk');
    const once = { file: written(), versions: await versionIds() };

    const again = expectOk(await driver.save(doc.file, html, undefined));
    check(again.changed === true, 'the repeat was not answered as the save that changed the document');
    check(isDeepStrictEqual(written(), once.file) && read(doc.file) === html, 'the repeat wrote the file again');
    check(isDeepStrictEqual(await versionIds(), once.versions), 'the repeat kept another version');
    const undo = expectOk(await driver.version(doc.file, again.undoVersionId));
    check(undo.html === before.html, 'the repeat names a version that is not the bytes the first save replaced');
    check(expectOk(await driver.read(doc.file)).etag === again.etag, 'the repeat did not answer the etag the document has');

    const [first, second] = proxy.writes;
    check(proxy.writes.length === 2 && first.dropped && !second.dropped, `the proxy saw ${proxy.writes.length} writes`);
    check(first.operationId && first.operationId === second.operationId, 'the repeat carried another operation id');
    check(first.status === 200 && second.answer === first.answer, 'the lane did not answer the repeat with the first answer');
    check(parseJson(first.answer)?.undoVersionId === again.undoVersionId, 'the repeat names another undo version than the first answer');
    return { operationId: first.operationId, undoVersionId: again.undoVersionId, cli: 'not driven: the CLI takes no operation id and mints one per process' };
  } finally {
    await driver.stop();
    await proxy.close();
  }
}

// A path outside every folder is refused by clay-agent itself: a process that
// has done nothing else opens no connection, and the file it names is untouched.
function outsideCases(world) {
  const escaped = `http://notes.htmlclay.localhost:${world.port}/..%2F..%2Foutside.htmlclay`;
  const refusedHere = (driver, target, code, exit) => driver.fresh(async (unused) => {
    const before = read(world.outside);
    for (const answer of [await unused.read(target), await unused.save(target, edited(before, 'outside'), '0'.repeat(16))]) {
      expectRefusal(answer, code, null, exit);
      check(answer.connects === 0, `clay-agent connected ${answer.connects} times before refusing`);
    }
    check(read(world.outside) === before, 'the refused save changed the file');
    return { code };
  });
  return {
    served: (driver) => refusedHere(driver, escaped, 'not-found', 1),
    loose: (driver) => refusedHere(driver, world.outside, 'host-unavailable', 3),
  };
}

// The list names both documents the way the lane holds them, and the address it
// gives a document reads that document.
async function listed(driver, doc) {
  const result = expectOk(await driver.list());
  const row = result.documents.find((entry) => entry.path === doc.file);
  check(row, `the list does not hold ${doc.file}`);
  const kind = doc.name === 'served' ? 'served' : 'open';
  check(row.kind === kind && row.host === 'htmlclay', `the list calls it ${row.kind} on ${row.host}`);
  if (doc.name === 'served') check(row.url === doc.address, `the list gives the address ${row.url}`);
  const byAddress = expectOk(await driver.read(row.url));
  check(byAddress.ref === doc.file && byAddress.html === read(doc.file), `${row.url} does not read ${doc.file}`);
  return { url: row.url, kind: row.kind };
}

async function main() {
  const options = parseArgs(process.argv.slice(2));
  const pack = process.env.TESTBOX_CLAY_AGENT;
  if (!pack || !existsSync(path.join(pack, 'bin', 'clay-agent.js'))) {
    console.error('agent-contract: TESTBOX_CLAY_AGENT must name a clay-agent package, a folder with bin/clay-agent.js.');
    console.error('`testbox run` sets it when ../clay-agent exists or CLAY_AGENT_SRC names the source (see .testbox).');
    return 2;
  }
  if (process.platform === 'win32') {
    console.error('agent-contract: Linux and macOS only.');
    return 2;
  }
  const cli = options.cli ?? path.join(path.resolve(pack), 'bin', 'clay-agent.js');
  if (!existsSync(cli)) {
    console.error(`agent-contract: there is no CLI at ${cli}`);
    return 2;
  }

  // The host listens on a socket in its config folder, and macOS's own temp
  // folder leaves that path longer than a socket's may be.
  const temp = process.platform === 'darwin' ? '/tmp' : os.tmpdir();
  const root = realpathSync(mkdtempSync(path.join(temp, 'htmlclay-agent-contract-')));
  const rows = [];
  let host = null;
  let failure = null;
  const report = { ok: false, cli, clayAgent: null, host: null, mutation: options.wrongToken ? 'wrong-token' : null, cases: rows };
  try {
    const bin = options.bin ?? build(root);
    const world = seed(root, await freePort());
    host = await startHost(bin, world);
    check(host.ready.port === world.port, `the host answers on port ${host.ready.port}, not the ${world.port} its config names`);
    report.host = { version: host.ready.version, port: host.ready.port };

    const opened = await shellAction(host, 'open-loose', 'file.open', { path: world.documents.loose.file });
    check(opened.status === 200, `file.open answered ${opened.status}: ${opened.body}`);
    const opens = tail(world.openLog).split('\n').filter((line) => line.startsWith('http://') && line.endsWith('/journal.htmlclay'));
    check(opens.length === 1, `the open log holds ${opens.length} addresses for the loose file:\n${tail(world.openLog)}`);

    if (options.wrongToken) writeFileSync(path.join(world.configDir, 'agent-token'), `${randomBytes(32).toString('hex')}\n`);

    let traceCount = 0;
    const agent = {
      cli,
      cwd: root,
      env: environment(world.home),
      traces: path.join(root, 'traces'),
      nextTrace: () => { traceCount += 1; return traceCount; },
      calls: [],
    };
    mkdirSync(agent.traces);
    report.clayAgent = execFileSync(process.execPath, [cli, '--version'], { env: agent.env, encoding: 'utf8' }).trim();

    const run = async (name, document, via, body) => {
      const row = { case: name, document, via, pass: false, detail: null };
      const from = agent.calls.length;
      try {
        row.detail = await body();
        row.pass = true;
      } catch (error) {
        row.detail = { error: error.message };
      }
      row.detail.calls = agent.calls.slice(from);
      rows.push(row);
      console.log(`${row.pass ? 'ok  ' : 'FAIL'} ${via.padEnd(4)} ${document.padEnd(7)} ${name}${row.pass ? '' : `: ${row.detail.error}`}`);
    };

    const outside = outsideCases(world);
    for (const start of [async () => cliDriver(agent), () => mcpDriver(agent)]) {
      const driver = await start();
      try {
        for (const doc of Object.values(world.documents)) {
          const cases = lifecycle(driver, { ...doc, unopened: world.unopened, world });
          for (const [name, body] of Object.entries(cases)) await run(name, doc.name, driver.via, body);
          await run('list', doc.name, driver.via, () => listed(driver, doc));
          await run('path outside every folder', doc.name, driver.via, () => outside[doc.name](driver));
        }
      } finally {
        await driver.stop();
      }
    }
    for (const doc of Object.values(world.documents)) {
      await run('repeated save', doc.name, 'mcp', () => repeatedSave(agent, world, doc));
    }
  } catch (error) {
    failure = error;
  } finally {
    if (host) await host.stop();
    if (options.output) {
      mkdirSync(path.dirname(options.output), { recursive: true });
      if (existsSync(path.join(root, 'host.log'))) copyFileSync(path.join(root, 'host.log'), `${options.output}.host.log`);
    }
    rmSync(root, { recursive: true, force: true });
  }

  const failed = rows.filter((row) => !row.pass);
  report.ok = !failure && rows.length > 0 && failed.length === 0;
  if (failure) report.error = failure.message;
  if (options.output) writeFileSync(options.output, `${JSON.stringify(report, null, 2)}\n`);
  if (failure) console.error(`agent-contract: ${failure.message}`);
  console.log(report.ok ? `AGENT CONTRACT OK cases=${rows.length}` : `AGENT CONTRACT FAILED cases=${rows.length} failed=${failed.length}${failure ? ' (the run stopped early)' : ''}`);
  return report.ok ? 0 : 1;
}

process.exitCode = await main();
