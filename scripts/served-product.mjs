// Boots the real app (`cmd/htmlclay`) serving one folder, on a throwaway config
// base, and answers once that folder is served. The trial binary answers a READY
// line; the real app only logs, so readiness is the `Serving folder <path> at
// <url>` line its own logger writes.
import { spawn } from 'node:child_process';
import { randomBytes } from 'node:crypto';
import {
  appendFileSync, mkdirSync, mkdtempSync, readFileSync, realpathSync, rmSync, statSync, writeFileSync,
} from 'node:fs';
import http from 'node:http';
import net from 'node:net';
import os from 'node:os';
import path from 'node:path';

export const READY_TIMEOUT_MS = 30000;
export const STOP_TIMEOUT_MS = 15000;
const TAIL_LINES = 40;

const wait = (ms) => new Promise((resolve) => setTimeout(resolve, ms));

const exited = (child) => child.exitCode !== null || child.signalCode !== null;

function escaped(text) {
  return text.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
}

// One port free on both loopback stacks, because the app holds 127.0.0.1 and ::1
// on the same number and asks the person what to do when only one is free.
function bind(host, port) {
  const server = net.createServer();
  return new Promise((resolve, reject) => {
    server.once('error', reject);
    server.listen({ host, port, exclusive: true }, () => resolve(server));
  });
}

const close = (server) => new Promise((resolve) => server.close(resolve));

export async function freePort() {
  for (let attempt = 0; attempt < 20; attempt += 1) {
    const v4 = await bind('127.0.0.1', 0);
    const { port } = v4.address();
    let v6 = null;
    try {
      v6 = await bind('::1', port);
    } catch (error) {
      if (error?.code === 'EADDRINUSE') {
        await close(v4);
        continue;
      }
      // No IPv6 loopback here: the app serves what it can bind, as it does.
    }
    await close(v4);
    if (v6) await close(v6);
    return port;
  }
  throw new Error('could not find a port free on both loopback stacks');
}

function readTail(logPath, limit = TAIL_LINES) {
  try {
    return readFileSync(logPath, 'utf8').trimEnd().split('\n').slice(-limit).join('\n');
  } catch {
    return '';
  }
}

// The folder has to be one the app can serve: a real folder inside the home
// folder, and not the home folder itself.
function servedRoot(root) {
  let real;
  try {
    real = realpathSync(root);
  } catch (error) {
    throw new Error(`the folder ${root} does not exist: ${error.message}`);
  }
  if (!statSync(real).isDirectory()) throw new Error(`${real} is not a folder`);
  const home = realpathSync(os.homedir());
  const rel = path.relative(home, real);
  if (!rel || rel.startsWith('..') || path.isAbsolute(rel) || rel.split(path.sep).includes('..')) {
    throw new Error(`the folder ${real} must be a folder inside ${home}, not the home folder itself`);
  }
  return real;
}

export async function startProduct({
  bin, root, name = 'conf', port, log,
}) {
  if (!bin) throw new Error('startProduct needs a binary path');
  const folder = servedRoot(root);
  const configBase = mkdtempSync(path.join(os.tmpdir(), 'htmlclay-served-product-'));
  const configDir = path.join(configBase, 'htmlclay');
  const logPath = log ?? path.join(configBase, 'product.log');
  mkdirSync(configDir, { recursive: true });
  mkdirSync(path.dirname(logPath), { recursive: true });
  writeFileSync(logPath, '');

  const chosen = port ?? await freePort();
  // A first launch with no port chooses one and merely announces it, and an open
  // update check talks to the network the box may not have: the config names the
  // port, and turns updates off, so this launch neither asks nor dials.
  const config = {
    startOnLogin: false,
    port: chosen,
    updates: 'off',
    servedFolders: [{
      id: randomBytes(8).toString('hex'),
      name,
      path: folder,
      identity: '',
      serving: true,
    }],
  };
  writeFileSync(path.join(configDir, 'config.json'), `${JSON.stringify(config, null, 2)}\n`);

  const child = spawn(bin, ['--no-tray'], {
    cwd: path.dirname(logPath),
    env: { ...process.env, HTMLCLAY_CONFIG_BASE: configBase },
    stdio: ['ignore', 'pipe', 'pipe'],
  });
  const exitPromise = new Promise((resolve) => {
    child.once('exit', (code, signal) => resolve({ code, signal }));
  });
  const readyPattern = new RegExp(
    `Serving folder ${escaped(folder)} at http://${escaped(name)}\\.htmlclay\\.localhost:${chosen}(?:\\s|$)`,
  );
  const record = (chunk) => {
    const text = chunk.toString('utf8');
    try {
      appendFileSync(logPath, text);
    } catch {
      // A log that cannot be written cannot change a verdict.
    }
    return text;
  };
  child.stdout.setEncoding('utf8');
  child.stderr.setEncoding('utf8');

  let stopPromise = null;
  const stop = () => {
    if (stopPromise) return stopPromise;
    stopPromise = (async () => {
      if (!exited(child)) {
        try {
          child.kill('SIGTERM');
        } catch {
          // A signal to a child already gone is the same as one gone.
        }
        const gone = await Promise.race([exitPromise, wait(STOP_TIMEOUT_MS).then(() => null)]);
        if (!gone) {
          try {
            child.kill('SIGKILL');
          } catch {
            // Same.
          }
          await exitPromise;
        }
      } else {
        await exitPromise;
      }
      rmSync(configBase, { recursive: true, force: true });
    })();
    return stopPromise;
  };

  const url = `http://${name}.htmlclay.localhost:${chosen}`;

  let scan = '';
  const answer = await new Promise((resolve, reject) => {
    const timer = setTimeout(() => {
      reject(new Error(`the app served ${folder} nowhere within ${READY_TIMEOUT_MS}ms; last log lines:\n${readTail(logPath)}`));
    }, READY_TIMEOUT_MS);
    const finish = (error) => {
      clearTimeout(timer);
      if (error) reject(error);
      else resolve({ url, port: chosen, configBase, logPath, pid: child.pid, stop });
    };
    child.on('error', (error) => finish(new Error(`the app could not start: ${error.message}`)));
    exitPromise.then(({ code, signal }) => {
      finish(new Error(`the app exited before serving ${folder} (${signal ?? `exit ${code}`}); last log lines:\n${readTail(logPath)}`));
    });
    const listen = (stream) => {
      stream.on('data', (chunk) => {
        scan += record(chunk);
        if (readyPattern.test(scan)) finish(null);
      });
    };
    listen(child.stdout);
    listen(child.stderr);
  }).catch(async (error) => {
    await stop();
    throw error;
  });

  return answer;
}

function tailOf(file, lines = TAIL_LINES) {
  try {
    return readFileSync(file, 'utf8').trimEnd().split('\n').slice(-lines).join('\n');
  } catch {
    return '';
  }
}

// The host as the Mac app starts it: the secret on stdin, the READY line on
// stdout, and stdin held open, because its end is what shuts the host down.
// Everything the host prints goes to log.
export async function startShellHost({ bin, cwd, env, log }) {
  const secret = randomBytes(32).toString('hex');
  const child = spawn(bin, ['--shell-stdin'], { cwd, env, stdio: ['pipe', 'pipe', 'pipe'] });
  const gone = new Promise((resolve) => child.once('exit', (code, signal) => resolve({ code, signal })));
  child.stdin.on('error', () => {});
  child.stdin.write(`${JSON.stringify({ v: 1, secret })}\n`);

  let scan = '';
  const ready = await new Promise((resolve, reject) => {
    const timer = setTimeout(() => {
      reject(new Error(`the host printed no READY line within ${READY_TIMEOUT_MS}ms; its last lines:\n${tailOf(log)}`));
    }, READY_TIMEOUT_MS);
    const finish = (error, value) => {
      clearTimeout(timer);
      if (error) reject(error);
      else resolve(value);
    };
    child.stdout.on('data', (chunk) => {
      const text = chunk.toString('utf8');
      appendFileSync(log, text);
      scan += text;
      const line = scan.split('\n').find((entry) => entry.startsWith('READY ') || entry.startsWith('FAILED '));
      if (!line || !scan.includes(`${line}\n`)) return;
      if (line.startsWith('FAILED ')) finish(new Error(`the host refused to start: ${line}`));
      else finish(null, JSON.parse(line.slice('READY '.length)));
    });
    child.stderr.on('data', (chunk) => appendFileSync(log, chunk.toString('utf8')));
    gone.then(({ code, signal }) => finish(new Error(`the host exited before it was ready (${signal ?? `exit ${code}`}):\n${tailOf(log)}`)));
  });

  const stop = async () => {
    if (exited(child)) return;
    child.stdin.end();
    if (await Promise.race([gone, wait(STOP_TIMEOUT_MS).then(() => null)])) return;
    child.kill('SIGKILL');
    await gone;
  };
  return { ready, secret, stop, child };
}

// One call on the host's private listener, the way the Mac app makes it. Not
// fetch: Node's fetch adds Sec-Fetch-Mode, and that listener refuses any request
// carrying a header a browser sends. Answers { status, body } with body as text.
export function shellRequest(host, method, pathname, json = null, timeoutMs = 60000) {
  const body = json === null ? '' : JSON.stringify(json);
  return new Promise((resolve, reject) => {
    const headers = { Authorization: `Bearer ${host.secret}` };
    if (json !== null) Object.assign(headers, { 'Content-Type': 'application/json', 'Content-Length': Buffer.byteLength(body) });
    const request = http.request(`${host.ready.shell}${pathname}`, { method, headers, timeout: timeoutMs }, (response) => {
      const chunks = [];
      response.on('data', (chunk) => chunks.push(chunk));
      response.on('end', () => resolve({ status: response.statusCode, body: Buffer.concat(chunks).toString('utf8') }));
    });
    request.on('timeout', () => request.destroy(new Error(`${method} ${pathname} got no answer`)));
    request.on('error', reject);
    request.end(body);
  });
}
