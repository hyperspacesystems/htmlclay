// What each open file owes the platform on its next sync upload, keyed by filename
// within its own root's store: the same relative path in two roots is two different
// files, and one root's snapshot must never be uploaded as the other's. Two lanes
// fill it: /live-sync/save contributes the unstripped snapshot, /save contributes
// the provenance bit. Either half can arrive without the other, so both are optional
// and neither lane clears the other's.
const rootStores = new Map();

function snapshotStoreFor(rootId) {
  let store = rootStores.get(rootId);
  if (!store) {
    store = { snapshots: new Map(), etags: new Map() };
    rootStores.set(rootId, store);
  }
  return store;
}

module.exports = { rootStores, snapshotStoreFor };
