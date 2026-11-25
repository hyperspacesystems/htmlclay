// Browsers, and the RFC 6761 rule they follow, send every `*.localhost` name to
// loopback. Node asks the system resolver instead, which on macOS and on some
// Linux images answers `*.localhost` with `::1`, with a public address, or with
// nothing at all. This module wraps `dns.lookup` and `dns.promises.lookup` so a
// name that is `localhost` or ends in `.localhost` answers `127.0.0.1`, and
// every other name reaches the resolver it would have reached anyway. Load it
// with `node --import <path>/scripts/localhost-dns.mjs`.
import dns from 'node:dns';

export const LOOPBACK = '127.0.0.1';

export function isLoopbackName(hostname) {
  const name = typeof hostname === 'string' ? hostname.toLowerCase() : '';
  return name === 'localhost' || name.endsWith('.localhost');
}

// `options` is a family number, a deprecated encoding string, an object, or a
// callback in its place. Only `all` changes the answer's shape; a family of 6
// cannot be honoured by an IPv4-only answer, but a caller asking for it against
// a `.localhost` name is asking for the loopback address either way.
function wantsAll(options) {
  return options !== null && typeof options === 'object' && options.all === true;
}

const originalLookup = dns.lookup;
const originalPromisesLookup = dns.promises.lookup;

dns.lookup = function lookup(hostname, options, callback) {
  let opts = options;
  let cb = callback;
  if (typeof options === 'function') {
    cb = options;
    opts = undefined;
  }
  if (!isLoopbackName(hostname) || typeof cb !== 'function') {
    return originalLookup.call(dns, hostname, opts, cb);
  }
  const all = wantsAll(opts);
  process.nextTick(() => {
    if (all) cb(null, [{ address: LOOPBACK, family: 4 }]);
    else cb(null, LOOPBACK, 4);
  });
  return {};
};

dns.promises.lookup = async function lookup(hostname, options) {
  if (!isLoopbackName(hostname)) {
    return originalPromisesLookup.call(dns.promises, hostname, options);
  }
  if (wantsAll(options)) return [{ address: LOOPBACK, family: 4 }];
  return { address: LOOPBACK, family: 4 };
};
