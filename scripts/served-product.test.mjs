// Proves the real app serves a folder: build `cmd/htmlclay`, point it at a
// throwaway config that serves one folder under the home folder, and read a file
// back over the served name. Skips with HTMLCLAY_SKIP_PRODUCT_TEST=1, and uses
// HTMLCLAY_BIN instead of building when the gate has already built one.
import assert from 'node:assert/strict';
import { spawnSync } from 'node:child_process';
import { mkdtempSync, rmSync, writeFileSync } from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import test from 'node:test';
import { fileURLToPath } from 'node:url';
import './localhost-dns.mjs';
import { startProduct } from './served-product.mjs';

const HERE = path.dirname(fileURLToPath(import.meta.url));
const REPO = path.resolve(HERE, '..');
const SKIP = process.env.HTMLCLAY_SKIP_PRODUCT_TEST === '1' ? 'HTMLCLAY_SKIP_PRODUCT_TEST=1' : false;

function buildBinary() {
  const dir = mkdtempSync(path.join(os.tmpdir(), 'htmlclay-served-product-bin-'));
  const bin = path.join(dir, 'htmlclay');
  const env = { ...process.env };
  if (process.platform === 'linux') env.CGO_ENABLED = '0';
  const built = spawnSync('go', ['build', '-o', bin, './cmd/htmlclay'], {
    cwd: REPO, env, encoding: 'utf8', timeout: 300000,
  });
  if (built.error || built.status !== 0) {
    throw new Error(`go build ./cmd/htmlclay failed: ${built.error?.message ?? ''}${built.stderr ?? ''}`);
  }
  return { bin, dir };
}

test('the app serves a folder and answers its files', { skip: SKIP, timeout: 360000 }, async () => {
  const built = process.env.HTMLCLAY_BIN ? null : buildBinary();
  const bin = built ? built.bin : path.resolve(process.env.HTMLCLAY_BIN);
  const root = mkdtempSync(path.join(os.homedir(), '.served-product-test-'));
  writeFileSync(path.join(root, 'index.html'), '<!DOCTYPE html><title>served</title><p>served by the app</p>\n');
  let product = null;
  try {
    product = await startProduct({ bin, root, name: 'conf' });
    const answer = await fetch(`${product.url}/index.html`);
    assert.equal(answer.status, 200);
    assert.match(await answer.text(), /served by the app/);
  } finally {
    if (product) await product.stop();
    rmSync(root, { recursive: true, force: true });
    if (built) rmSync(built.dir, { recursive: true, force: true });
  }
});
