'use strict';

// A program's write (the agent lane's save and restore, a data write no page
// sent) replaces a document with no page behind it, and a room open on that
// document holds its unsaved edits in memory only. Go marks the admit of such a
// write `recover` (internal/served/commit_lanes.go), and the edits become a
// recovered copy inside that consultation: an artifact Go writes before admit
// answers, so before the first byte of the document changes.
//
// Local's save admission keeps a recovered copy only for a page that needs
// adoption (vendor/local/src/main/document-host.js `admitSave`), and the vendored
// tree is pinned to a Local commit. So the rule is added around two of the
// service's own seams: its admit handler, which sees Go's flag, and each root's
// `admitSave`, which runs inside the consultation with the write's target and token.

const { AsyncLocalStorage } = require('async_hooks');
const { normCanonical } = require('./vendor/local/src/main/local-authority.js');

function keepUnsavedEdits(service) {
  const { authority } = service;
  const asked = new AsyncLocalStorage();
  const admit = service.handlers.get('/v1/admit');
  service.handlers.set('/v1/admit', (body, reply) => asked.run(body.recover === true, () => admit(body, reply)));

  const wrapped = new WeakSet();
  service.onRoots(() => {
    for (const { lanes } of service.roots.values()) {
      if (wrapped.has(lanes)) continue;
      wrapped.add(lanes);
      const { admitSave } = lanes;
      lanes.admitSave = async (request) => {
        const admitted = await admitSave(request);
        // The authority throws when it is asked about a document with no room.
        if (asked.getStore() && authority.rooms.get(normCanonical(request.target.canonicalPath))) {
          await authority.recoverUnsavedEdits(request.target, request.token);
        }
        return admitted;
      };
    }
  });
}

module.exports = { keepUnsavedEdits };
