/**
 * Carrying a page's merge state over device sync.
 *
 * The record travels beside the content, never instead of it: on an upload the
 * record goes to the server before the bytes do, and on a download it lands in
 * the sidecar before the file is written. Every call here is best-effort — an
 * older platform answers 404 and a network failure answers nothing, and either
 * way the content sync carries on.
 */

const path = require('upath');
const { readPageState } = require('../main/page-state-store');
const { getPageState, putPageState } = require('./api-client');
const { documentHostFor, withLocalDocumentWrite } = require('./local-boundary');

const PAGE_PATTERN = /\.(html|htmlclay)$/i;

const reasonOf = (error) => (error && error.message ? error.message : error);

/** The merge state this device holds for `rel`, on its way to `nodeId`. */
async function sendPageState(engine, nodeId, rel) {
  if (!PAGE_PATTERN.test(rel)) return false;
  try {
    // The record for the file on disk, which need not be the newest one the sidecar holds.
    const file = await documentHostFor(engine).read(rel, path.join(engine.syncFolder, rel));
    if (!file) return false;
    const rev = require('@panphora/clayjs/server/rooms').pageRev(file.bytes.toString('utf8'));
    const record = await readPageState(engine.syncFolder, rel, { rev, exact: true });
    if (!record) return false;
    return await putPageState(engine.conn, nodeId, record);
  } catch (error) {
    console.warn(`[SYNC] Could not send the page state for ${rel}:`, reasonOf(error));
    return false;
  }
}

/** The server's merge state for `nodeId`, or null when it has none. */
async function readPageRecord(engine, nodeId, rel) {
  if (!PAGE_PATTERN.test(rel)) return null;
  try {
    return (await getPageState(engine.conn, nodeId)) || null;
  } catch (error) {
    console.warn(`[SYNC] Could not read the page state for ${rel}:`, reasonOf(error));
    return null;
  }
}

// Node and revision pairs checked this run: true once the server answered, or the
// lookup in flight, which concurrent opens share. A failed lookup is forgotten, so the
// next open asks again.
const caughtUp = new Map();

/**
 * The server's record for a synced Block document that has none beside it.
 *
 * A Local from before the Block editor rename asked for records on a route that no
 * longer answers, read that as "no record", and kept none. Local reads a record again
 * only when a changed file downloads, so an unchanged file would never get one, and
 * its editor would start a second history that the next upload writes over the
 * server's. Called before the document is served, so the record is in place before
 * any editor can start.
 */
async function catchUpPageRecord(engine, rel) {
  if (!engine || !engine.repo || !engine.syncFolder || !PAGE_PATTERN.test(rel)) return false;
  const tracked = engine.repo.getByPath(rel);
  if (!tracked) return false;
  const file = await documentHostFor(engine).read(rel, path.join(engine.syncFolder, rel));
  const rev = file ? require('@panphora/clayjs/server/rooms').pageRev(file.bytes.toString('utf8')) : null;
  const key = `${tracked.nodeId}:${rev}`;
  if (!rev || caughtUp.get(key) === true) return false;
  if (caughtUp.has(key)) return caughtUp.get(key);
  const lookup = catchUp(engine, rel, tracked.nodeId, rev);
  caughtUp.set(key, lookup);
  try {
    const applied = await lookup;
    caughtUp.set(key, true);
    return applied;
  } catch (error) {
    caughtUp.delete(key);
    throw error;
  }
}

async function catchUp(engine, rel, nodeId, rev) {
  const held = () => readPageState(engine.syncFolder, rel, { rev, exact: true });
  if (await held()) return false;
  const record = await getPageState(engine.conn, nodeId, { strict: true });
  if (!record || record.rev !== rev) return false;
  return withLocalDocumentWrite(engine, rel, async () => {
    if (await held()) return false;
    await applyPageRecord(engine, rel, record);
    return true;
  });
}

/** The server's record beside the file, written before the file itself. */
async function applyPageRecord(engine, rel, record) {
  if (!record || !PAGE_PATTERN.test(rel)) return;
  await documentHostFor(engine).blockeditorState({ op: 'write', name: rel, record });
}

// A relocation already happened on disk; a sidecar that cannot follow it is
// logged and left behind, never allowed to undo the content move.
async function moveState(engine, fromRel, toRel, op) {
  try {
    await documentHostFor(engine).blockeditorState({ op, name: fromRel, to: toRel });
  } catch (error) {
    console.warn(`[SYNC] Could not move the page state for ${fromRel}:`, reasonOf(error));
  }
}

/** A renamed document takes its merge state with it. */
async function moveDocumentPageState(engine, fromRel, toRel) {
  if (!fromRel || fromRel === toRel || !PAGE_PATTERN.test(fromRel)) return;
  await moveState(engine, fromRel, toRel, 'move');
}

/** A renamed folder takes its documents' merge state with it. */
async function moveFolderPageState(engine, fromRel, toRel) {
  if (!fromRel || fromRel === toRel) return;
  await moveState(engine, fromRel, toRel, 'moveDir');
}

// A deleted document's state goes with it: a copy restored from trash rebuilds its
// page from the article it carries, which is a new lineage.
async function dropDocumentPageState(engine, rel) {
  if (!rel || !PAGE_PATTERN.test(rel)) return;
  try {
    await documentHostFor(engine).blockeditorState({ op: 'drop', name: rel });
  } catch (error) {
    console.warn(`[SYNC] Could not drop the page state for ${rel}:`, reasonOf(error));
  }
}

/** A deleted folder takes its documents' merge state with it. */
async function dropFolderPageState(engine, rel) {
  if (!rel) return;
  try {
    await documentHostFor(engine).blockeditorState({ op: 'dropDir', name: rel });
  } catch (error) {
    console.warn(`[SYNC] Could not drop the page state for ${rel}:`, reasonOf(error));
  }
}

module.exports = {
  sendPageState, readPageRecord, applyPageRecord, catchUpPageRecord,
  moveDocumentPageState, moveFolderPageState, dropDocumentPageState, dropFolderPageState
};
