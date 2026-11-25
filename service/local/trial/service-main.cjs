'use strict';

// The managed trial's JS service as its own process (trial/protocol/v1.md,
// "Transport and bootstrap"). stdin carries the bootstrap: first
// `{"v":1,"secret":"<64 hex>"}`, answered on stdout with
// `READY {"url","epoch","pid"}`; then, once the supervisor has started Go,
// `{"host":{"url","secret","epoch","roots"}}` (Go's control URL, secret and
// epoch, and the roots of its READY line), answered with `ATTACHED`. EOF on
// stdin shuts the service down. A secret is never an argument or an env var.

const readline = require('readline');
const { createTrialService } = require('./service.cjs');

async function main() {
  const lines = readline.createInterface({ input: process.stdin, crlfDelay: Infinity })[Symbol.asyncIterator]();
  const first = await lines.next();
  if (first.done) throw new Error('no bootstrap line on stdin');
  const boot = JSON.parse(first.value);
  if (boot.v !== 1 || typeof boot.secret !== 'string' || !/^[0-9a-f]{64}$/.test(boot.secret)) {
    throw new Error('the bootstrap line needs v 1 and a 64-hex secret');
  }
  const service = createTrialService({ secret: boot.secret });
  const url = await service.listen();
  process.stdout.write(`READY ${JSON.stringify({ url, epoch: service.epoch, pid: process.pid })}\n`);
  for (;;) {
    const next = await lines.next();
    if (next.done) break;
    if (!next.value.trim()) continue;
    const message = JSON.parse(next.value);
    if (message.host) {
      await service.attachGo(message.host);
      process.stdout.write('ATTACHED\n');
    }
  }
  await service.close();
}

main().then(() => process.exit(0), (error) => {
  console.error('[trial-service]', error && error.stack ? error.stack : error);
  process.exit(3);
});
