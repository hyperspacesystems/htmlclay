// Preloaded into every clay-agent process the contract run starts (node --import).
// It appends one line to AGENT_CONTRACT_TRACE for each connection the process
// opens, so a case can say that a refusal came before any request.
import { appendFileSync } from 'node:fs';
import net from 'node:net';

const trace = process.env.AGENT_CONTRACT_TRACE;
if (trace) {
  const connect = net.Socket.prototype.connect;
  net.Socket.prototype.connect = function tracedConnect(...args) {
    // net.connect hands over its own normalized [options, callback] pair.
    const first = Array.isArray(args[0]) ? args[0][0] : args[0];
    const target = first !== null && typeof first === 'object'
      ? `${first.host ?? 'localhost'}:${first.port ?? first.path}`
      : `${typeof args[1] === 'string' ? args[1] : 'localhost'}:${first}`;
    try {
      appendFileSync(trace, `${target}\n`);
    } catch {
      // A trace that cannot be written leaves no line, which fails the read cases.
    }
    return connect.apply(this, args);
  };
}
