'use strict';

// The sync network gate. The vendored sync engine reaches the server only through
// the global fetch (api-client.js, utils.js, and the EventSource stream's fetch in
// engine-sse.js), so replacing it is the one place every outbound sync request
// passes. A request goes out only while a key is held, and only to the configured
// server's exact origin (scheme, host and port). Every other origin, loopback
// included, is refused and counted: the service reaches Go over http.request, not
// fetch, so nothing of its own needs one.

class SyncHostRefused extends Error {
  constructor() {
    super('sync-host-refused');
    this.code = 'sync-host-refused';
  }
}

function originOf(input) {
  const href = typeof input === 'string' ? input : input instanceof URL ? input.href : input && input.url;
  try {
    return new URL(href).origin;
  } catch {
    return null;
  }
}

function installGate({ fetchImpl = globalThis.fetch } = {}) {
  let origin = null;
  let refused = 0;
  const previous = globalThis.fetch;
  globalThis.fetch = function gatedFetch(input, init) {
    if (origin === null || originOf(input) !== origin) {
      refused += 1;
      return Promise.reject(new SyncHostRefused());
    }
    return fetchImpl(input, init);
  };
  return {
    // hold opens the gate to serverUrl's origin while a key is held.
    hold(serverUrl) { origin = new URL(serverUrl).origin; },
    clear() { origin = null; },
    held() { return origin !== null; },
    refusedCount() { return refused; },
    restore() { globalThis.fetch = previous; },
  };
}

module.exports = { installGate, SyncHostRefused };
