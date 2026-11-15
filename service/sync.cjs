'use strict';

// Sync in the service. Go holds the sync key in the system's secret store and
// hands it over on stdin (08-sync.md, C2); the service keeps it only in memory and
// never writes, logs or echoes it, or any part of the line that carried it.

const KEY = /^hcsk_[0-9a-f]{64}$/;
const LOOPBACK = new Set(['127.0.0.1', 'localhost', '[::1]']);

// serverOrigin answers the origin a key may be sent to, or null. https always; http
// only to loopback, which is where a test's stand-in server listens.
function serverOrigin(serverUrl) {
  let url;
  try {
    url = new URL(serverUrl);
  } catch {
    return null;
  }
  if (url.username || url.password) return null;
  if (url.protocol === 'https:') return url.origin;
  if (url.protocol === 'http:' && LOOPBACK.has(url.hostname)) return url.origin;
  return null;
}

function createSync({ gate }) {
  let held = null;
  return {
    // handle answers the stdout line for one {"sync":...} message.
    async handle(message) {
      if (!message || message.v !== 1) return 'SYNC KEY REFUSED';
      if (message.key === null) {
        held = null;
        gate.clear();
        return 'SYNC KEY CLEARED';
      }
      const origin = serverOrigin(message.serverUrl);
      if (typeof message.key !== 'string' || !KEY.test(message.key) || origin === null) {
        return 'SYNC KEY REFUSED';
      }
      held = { key: message.key, serverOrigin: origin };
      gate.hold(origin);
      return 'SYNC KEY HELD';
    },
    connection() { return held && { ...held }; },
  };
}

module.exports = { createSync, serverOrigin };
