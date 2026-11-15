/**
 * The one executor for a `decide` result.
 *
 * Every action `decide` returns has a branch here, and the baseline entry for a
 * node is advanced only after the local write or the server acknowledgment
 * succeeded. An uncertain outcome — a request that left without an answer —
 * refetches the node before it counts as either succeeded or failed, and a
 * conflict never loses bytes: the local file stays where it is, the remote
 * bytes land in `<root>/.hyperclay/conflicts/`, and the record in the session's
 * `conflicts.json` keeps both until the user picks mine or theirs.
 *
 * Reused rather than re-implemented: the api client (getNodeContent,
 * putNodeContent, createNode, deleteNode), the shared document boundary, and
 * the engine's `_applyFileDelete` for `trash-local` (the `.trash/` move and its
 * collision handling).
 *
 * Not wired into initial sync or the watcher yet; C3.5 does that.
 */

const fs = require('fs').promises;
const path = require('upath');
const {
  readFileBuffer,
  fileExists,
} = require('../file-operations');
const { calculateChecksum } = require('../utils');
const { conflictedCopyName } = require('../conflicted-copy-name');
const {
  createNode,
  deleteNode,
  getNodeContent,
  putNodeContent,
} = require('../api-client');
const { withLocalDocumentWrite, withLocalDirectoryChange, claimLocalDocumentTarget, documentHostFor, putSyncFile, removeSyncFile, ensureLocalFolder } = require('../local-boundary');
const { getConsentRegistry, resolveWritePath } = require('../../main/utils/path-resolver');
const nodeMap = require('../node-map');
const store = require('./conflicts');
const { A } = require('./decide');
const { isUnderLockedFolder, lockedFolderError } = require('../locked-folder');
const { sendPageState, readPageRecord, applyPageRecord } = require('../page-state');

const SITE_PATTERN = /\.(html|htmlclay)$/i;

/**
 * `session` is the session's engine: the per-session connection, baseline,
 * outbox, root and logger all live there. A C2 session entry holding `engine`
 * is accepted too.
 */
function engineOf(session) {
  return session && session.engine ? session.engine : session;
}

function relPathOf(entry, context, nodeId) {
  const rel = (entry && entry.path) || context.path;
  if (!rel) throw new Error(`No path known for node ${nodeId}`);
  return rel;
}

function typeOf(entry, context, rel) {
  if (entry && entry.type) return entry.type;
  if (context.type) return context.type;
  return SITE_PATTERN.test(rel) ? 'site' : 'upload';
}

function dirOf(rel) {
  const dir = path.dirname(rel);
  return dir === '.' ? '' : dir;
}

function idOf(nodeId) {
  const id = parseInt(nodeId, 10);
  return Number.isNaN(id) ? null : id;
}

async function localPathFor(engine, rel) {
  engine.resolveContainedPath(rel);
  return resolveWritePath(getConsentRegistry(engine.syncFolder), rel);
}

async function readLocalBytes(engine, localPath) {
  const read = await documentHostFor(engine).read(null, localPath);
  if (!read) throw Object.assign(new Error(`ENOENT: ${localPath}`), { code: 'ENOENT' });
  return { buffer: read.bytes, checksum: read.checksum };
}

// An attachment over the account's per-file cap stays on this computer. The
// baseline is left where it was, so the next pass asks again with the cap it
// has then: an upgrade sends it without anything else happening.
async function keepLocal(engine, rel, bytes, limit) {
  await engine.blockUpload(rel, bytes, limit);
  return { action: A.NOOP, blocked: true };
}

function overLimit(engine, isSite, bytes) {
  const limit = engine.uploadLimit;
  return !isSite && Number.isFinite(limit) && bytes > limit;
}


const STALE = 'stale';
const CHANGED = 'changed';

/**
 * Backup, atomic write and the data-loss guard around one local overwrite —
 * the sequence `downloadFile` runs for a synced-down file.
 *
 * `gen` and `expectLocal` are checked under the file lock, after any save queued ahead of this
 * write: a generation that ended, or local bytes that changed since the caller read them
 * (`null` = the file was absent), refuse the write. Returns `{ localPath, refused }`.
 */
const expectedFrom = (expectLocal) => expectLocal === undefined ? undefined
  : expectLocal === null ? { type: 'absent' } : { type: 'checksum', value: expectLocal };

async function writeLocal(engine, rel, content, { modifiedAt = null, gen, expectLocal, claim = null, explicitCreate = false, blockeditorRecord = null } = {}) {
  const generation = gen !== undefined ? gen : engine.generation;
  const host = documentHostFor(engine);
  let localPath = null;
  let refused = null;
  let checksum = null;
  const outcome = await withLocalDocumentWrite(engine, rel, async (target, token) => {
    localPath = target.canonicalPath;
    if (gen !== undefined && gen !== engine.generation) { refused = STALE; return null; }
    const written = await host.commit({
      kind: 'sync-download', name: rel, filePath: localPath, bytes: content, modifiedAt, blockeditorRecord,
      expected: expectedFrom(expectLocal), context: { target, token },
    });
    if (written.outcome === 'indeterminate') throw written.error;
    if (written.outcome === 'refused' && written.reason === 'backup-failed') {
      // The bytes on disk could not be kept as a version, so they are not replaced. That is
      // not an edit on this side: the download is retried after a back-off, never a conflict.
      throw Object.assign(new Error(`${rel} was not replaced: no version of it could be kept`), { code: 'backup-failed', statusCode: 503 });
    }
    if (written.outcome === 'refused') { refused = CHANGED; return null; }
    checksum = written.checksum;
    return written;
  }, { generation, claim, explicitCreate });
  if (outcome && outcome.stale) return { localPath, refused: STALE, checksum: null };
  return { localPath, refused, checksum };
}

/** Merge baseline fields into an entry (or a brand-new one) and persist. */
async function saveEntry(engine, nodeId, entry, fields, extra = {}) {
  const merged = { ...(entry || {}), ...extra };
  if (merged.path && merged.type !== 'folder' && extra.inode === undefined) {
    const identity = await nodeMap.getInode(await localPathFor(engine, merged.path));
    if (identity) merged.inode = identity;
  }
  const next = nodeMap.applyBaseline(merged, fields);
  await engine.repo.set(nodeId, next);
  return next;
}

/** Type, path, inode and syncedAt for an entry the baseline has none for. */
async function entryMeta(engine, rel, type, parentId) {
  const localPath = await localPathFor(engine, rel);
  const extra = { type, path: rel, inode: await nodeMap.getInode(localPath), syncedAt: Date.now() };
  if (parentId !== undefined && parentId !== null) extra.parentId = parentId;
  return extra;
}

async function openConflict(engine, nodeId, rel) {
  const records = await store.load(engine.metaDir);
  return store.forNode(records, nodeId, rel);
}

/**
 * Park the remote bytes and write the record. `fetchRemote` is false for
 * `remote-deleted` (no remote bytes are left) and for `name-taken` (the node
 * that answered is a different one).
 */
async function recordConflict(engine, nodeId, rel, { kind, localChecksum = null, remoteEtag = null, fetchRemote = true }) {
  // The same conflict found again (a restart compares from scratch) keeps the time it was first
  // found: that time is part of the id a shell shows the person and resolves by.
  const open = await openConflict(engine, nodeId, rel);
  const detectedAt = open && open.kind === kind && open.path === rel ? open.detectedAt : Date.now();
  const record = { kind, path: rel, localChecksum, remoteEtag, remoteCopy: null, detectedAt };

  if (fetchRemote && nodeId !== null && nodeId !== undefined) {
    try {
      const response = await getNodeContent(engine.conn, idOf(nodeId));
      const etag = response.etag || response.checksum || remoteEtag;
      await putSyncFile(engine, store.remoteCopyRef(rel, etag), response.content);
      record.remoteEtag = etag;
      record.remoteCopy = store.remoteCopyRef(rel, etag);
    } catch (error) {
      console.error(`[SYNC] Could not park the remote bytes for ${rel}:`, error.message);
    }
  }

  const stored = await store.set(engine.metaDir, { ...record, nodeId });
  engine.emit('file-synced', { file: rel, action: 'conflict', kind });
  return { key: stored.key, ...stored.record };
}

async function localChecksumOf(engine, rel) {
  const read = await documentHostFor(engine).read(rel, await localPathFor(engine, rel));
  return read ? read.checksum : null;
}

/**
 * A remote frame for a node that already has a record may only replace the
 * parked remote bytes and the record's etag; the local file is not touched.
 */
async function refreshRemoteCopy(engine, nodeId, rel, open, gen) {
  const response = await getNodeContent(engine.conn, idOf(nodeId));
  if (gen !== undefined && gen !== engine.generation) return { action: A.CONFLICT, stale: true };

  const etag = response.etag || response.checksum || null;
  await putSyncFile(engine, store.remoteCopyRef(rel, etag), response.content);
  const updated = await store.update(engine.metaDir, open.key, {
    remoteEtag: etag,
    remoteCopy: store.remoteCopyRef(rel, etag),
  });
  return { action: A.CONFLICT, kind: open.kind, record: { key: updated.key, ...updated.record } };
}

async function upload(engine, nodeId, entry, context, gen) {
  const rel = relPathOf(entry, context, nodeId);
  const open = await openConflict(engine, nodeId, rel);
  if (open) return { action: A.CONFLICT, kind: open.kind, record: open };

  const isSite = SITE_PATTERN.test(rel);
  const localPath = await localPathFor(engine, rel);
  const size = (await fs.stat(localPath)).size;
  if (overLimit(engine, isSite, size)) return keepLocal(engine, rel, size, engine.uploadLimit);
  const { buffer, checksum: localChecksum } = await readLocalBytes(engine, localPath);
  if (overLimit(engine, isSite, buffer.length)) return keepLocal(engine, rel, buffer.length, engine.uploadLimit);
  const content = isSite ? buffer.toString('utf8') : buffer;
  const baseline = engine.repo.getBaseline(nodeId) || {};

  const options = { modifiedAt: (await fs.stat(localPath)).mtime, ifMatch: baseline.remoteEtag };
  const snap = engine.snapshots && engine.snapshots.take ? await engine.snapshots.take(rel) : null;
  if (snap) {
    options.snapshotHtml = snap.html;
    if (snap.userDriven !== undefined) options.userDriven = snap.userDriven;
  }

  let response;
  engine.outbox.markInFlight('save', idOf(nodeId));
  await sendPageState(engine, idOf(nodeId), rel);
  try {
    response = await putNodeContent(engine.conn, idOf(nodeId), content, options);
  } catch (error) {
    if (error.statusCode === 412) {
      const record = await recordConflict(engine, nodeId, rel, {
        kind: store.KINDS.REJECTED,
        localChecksum,
        remoteEtag: error.etag || baseline.remoteEtag || null,
      });
      return { action: A.CONFLICT, kind: store.KINDS.REJECTED, record };
    }
    if (!error.statusCode) {
      const settled = await settleUpload(engine, nodeId, entry, context, localChecksum);
      if (settled) return settled;
    }
    if (!isSite && error.statusCode === 413 && error.code === 'too-large') {
      if (Number.isFinite(error.limit)) await engine.setUploadLimit(error.limit);
      return keepLocal(engine, rel, buffer.length, Number.isFinite(error.limit) ? error.limit : engine.uploadLimit);
    }
    throw error;
  }
  if (gen !== engine.generation) return { action: A.UPLOAD, stale: true };

  const etag = response.etag || response.checksum || localChecksum;
  await saveEntry(engine, nodeId, entry, {
    remoteEtag: etag,
    localChecksum: etag,
    structureVersion: response.structureVersion,
  });
  await engine.clearUploadBlock(rel);
  engine.emit('file-synced', { file: rel, action: 'upload', type: typeOf(entry, context, rel) });
  return { action: A.UPLOAD, etag, checksum: etag };
}

/**
 * A request that left but never answered: refetch the node before treating it
 * as failed. Remote bytes that now match ours mean the write landed.
 */
async function settleUpload(engine, nodeId, entry, context, localChecksum) {
  let response;
  try {
    response = await getNodeContent(engine.conn, idOf(nodeId));
  } catch {
    return null;
  }
  if (!response || response.checksum !== localChecksum) return null;

  const rel = relPathOf(entry, context, nodeId);
  const type = typeOf(entry, context, rel);
  const extra = entry ? {} : await entryMeta(engine, rel, type, context.parentId);
  const etag = response.etag || response.checksum || localChecksum;
  await saveEntry(engine, nodeId, entry, {
    remoteEtag: etag,
    localChecksum: etag,
    structureVersion: response.structureVersion,
  }, extra);
  return { action: A.UPLOAD, etag, checksum: etag, recovered: true };
}

/**
 * A folder has no bytes to fetch: its download is the directory on disk and
 * the entry in the baseline. The path comes from the context first, because a
 * folder the server moved is downloaded at its new path while the baseline
 * still holds the old one. The mkdir is cascade-marked so the watcher never
 * turns it into a folder create of its own.
 */
async function downloadFolder(engine, nodeId, entry, context, gen) {
  const rel = context.path || relPathOf(entry, context, nodeId);
  engine.resolveContainedPath(rel);

  let localPath = null;
  const created = await withLocalDirectoryChange(engine, rel, (target) => {
    localPath = target.canonicalPath;
    return engine._applyRemoteFsChange([rel], () => ensureLocalFolder(engine, rel));
  }, { generation: gen });
  if (created && created.stale) return { action: A.DOWNLOAD, stale: true };
  if (!created) engine.cascade.consume(rel);
  if (gen !== engine.generation) return { action: A.DOWNLOAD, stale: true };

  const parentId = context.parentId !== undefined && context.parentId !== null
    ? context.parentId
    : entry ? entry.parentId : null;
  await engine.repo.set(nodeId, {
    ...(entry || {}),
    type: 'folder',
    path: rel,
    parentId,
    inode: await nodeMap.getInode(localPath),
  });
  engine.emit('file-synced', { file: rel, action: 'create', type: 'folder' });
  return { action: A.DOWNLOAD };
}

async function download(engine, nodeId, entry, context, gen) {
  const rel = relPathOf(entry, context, nodeId);
  if (typeOf(entry, context, rel) === 'folder') return downloadFolder(engine, nodeId, entry, context, gen);
  const open = await openConflict(engine, nodeId, rel);
  if (open) return refreshRemoteCopy(engine, nodeId, rel, open, gen);

  const expectLocal = await localChecksumOf(engine, rel);
  const claim = engine.localAuthority ? await claimLocalDocumentTarget(engine, rel) : null;
  const response = await getNodeContent(engine.conn, idOf(nodeId));
  if (gen !== engine.generation) return { action: A.DOWNLOAD, stale: true };

  const blockeditorRecord = await readPageRecord(engine, idOf(nodeId), rel);
  if (gen !== engine.generation) return { action: A.DOWNLOAD, stale: true };

  const written = await writeLocal(engine, rel, response.content, { modifiedAt: response.modifiedAt, gen, expectLocal, claim, explicitCreate: !entry || entry.path === rel, blockeditorRecord });
  if (written.refused === STALE) return { action: A.DOWNLOAD, stale: true };
  if (written.refused === CHANGED) {
    return conflicted(engine, nodeId, entry, context, {
      conflictKind: expectLocal === null ? store.KINDS.UNBOUND : store.KINDS.BOTH_EDITED,
    });
  }
  const localPath = written.localPath;
  const localChecksum = written.checksum;
  const type = typeOf(entry, context, rel);
  const parentId = entry ? entry.parentId : context.parentId;

  await saveEntry(engine, nodeId, entry, {
    remoteEtag: response.etag || response.checksum || context.etag || null,
    localChecksum,
    structureVersion: response.structureVersion !== undefined ? response.structureVersion : context.structureVersion,
  }, await entryMeta(engine, rel, type, parentId));

  await releaseFileLimitHold(engine, rel);
  engine.emit('file-synced', { file: rel, action: 'download', type });
  return { action: A.DOWNLOAD, checksum: localChecksum };
}

// A noop pass is where a version the desktop did not cause (a teammate's rename of a parent,
// a folder's subtree) reaches the baseline, so the next structural change sends it.
async function refreshStructureVersion(engine, nodeId, entry, context) {
  if (!entry) return;
  if (entry.path && entry.type !== 'folder') {
    const identity = await nodeMap.getInode(await localPathFor(engine, entry.path));
    if (identity && identity !== entry.inode) await engine.repo.set(nodeId, { ...entry, inode: identity });
  }
  if (!context.structureVersion) return;
  const baseline = engine.repo.getBaseline(nodeId);
  if (!baseline || baseline.structureVersion === context.structureVersion) return;
  await engine.repo.updateBaseline(nodeId, { structureVersion: context.structureVersion });
}

// A site held at the file limit that is now bound to a server node (another device uploaded it, or
// it came down) is synced: it leaves the held list, and the next held file is asked for.
async function releaseFileLimitHold(engine, rel) {
  if (engine.uploadBlocks?.files?.[rel]?.reason !== 'file-limit') return;
  await engine.clearUploadBlock(rel);
  engine.retryFileLimitBlocks?.();
}

async function adopt(engine, nodeId, entry, context) {
  const rel = relPathOf(entry, context, nodeId);
  const localChecksum = await localChecksumOf(engine, rel);
  const type = typeOf(entry, context, rel);
  const parentId = entry ? entry.parentId : context.parentId;

  await saveEntry(engine, nodeId, entry, {
    remoteEtag: context.etag || localChecksum,
    localChecksum,
    structureVersion: context.structureVersion === undefined ? null : context.structureVersion,
  }, await entryMeta(engine, rel, type, parentId));

  await releaseFileLimitHold(engine, rel);
  return { action: A.ADOPT, checksum: localChecksum };
}

async function trashLocal(engine, nodeId, entry, context) {
  const rel = relPathOf(entry, context, nodeId);
  await engine._applyFileDelete(idOf(nodeId), rel, typeOf(entry, context, rel));
  return { action: A.TRASH_LOCAL, path: rel };
}

async function deleteRemote(engine, nodeId, entry, context) {
  const rel = relPathOf(entry, context, nodeId);
  // The root's `uploads` folder is never deleted: it holds every attachment.
  if (isUnderLockedFolder(rel)) throw lockedFolderError();
  engine.assertRootPresent();
  const type = typeOf(entry, context, rel);
  const id = idOf(nodeId);
  // A folder is deleted against the version the decision was made from: the server refuses a
  // cascade when anything under the folder changed since.
  const decidedVersion = type === 'folder' && engine.protocol === 2 ? context.structureVersion : undefined;
  const expectedVersion = decidedVersion ?? (await engine._expectedVersion(id)) ?? null;
  // The platform refuses to delete a folder with contents unless the delete cascades.
  const options = type === 'folder' ? { expectedVersion, cascade: true } : { expectedVersion };

  engine.outbox.markInFlight('delete', id);
  try {
    await deleteNode(engine.conn, id, options);
  } catch (error) {
    if (!error.statusCode && (await deleteLanded(engine, id))) {
      await finishDelete(engine, nodeId, rel, type);
      return { action: A.DELETE_REMOTE, path: rel, recovered: true };
    }
    throw error;
  }

  await finishDelete(engine, nodeId, rel, type);
  return { action: A.DELETE_REMOTE, path: rel };
}

async function deleteLanded(engine, id) {
  try {
    await getNodeContent(engine.conn, id);
    return false;
  } catch (error) {
    return error.statusCode === 404;
  }
}

async function finishDelete(engine, nodeId, rel, type) {
  if (type === 'folder') {
    const descendants = engine.repo.walkDescendants(rel);
    await engine.repo.apply(async (map) => {
      for (const { nodeId: descId } of descendants) map.delete(descId);
      map.delete(String(nodeId));
    });
  } else {
    await engine.repo.delete(nodeId);
  }
  if (typeof engine.invalidateServerNodesCache === 'function') engine.invalidateServerNodesCache();
  engine.emit('file-synced', { file: rel, action: 'delete', type });
}

async function createRemote(engine, nodeId, entry, context, gen) {
  const rel = relPathOf(entry, context, nodeId);
  const isSite = SITE_PATTERN.test(rel);
  const localPath = await localPathFor(engine, rel);
  const size = (await fs.stat(localPath)).size;
  if (overLimit(engine, isSite, size)) return keepLocal(engine, rel, size, engine.uploadLimit);
  const { buffer, checksum: localChecksum } = await readLocalBytes(engine, localPath);
  if (overLimit(engine, isSite, buffer.length)) return keepLocal(engine, rel, buffer.length, engine.uploadLimit);
  const content = isSite ? buffer.toString('utf8') : buffer;
  const type = typeOf(entry, context, rel);

  // This pass already met the plan's file limit: the rest of the plan is held
  // without a request each. The queue path retries one at a time instead.
  if (isSite && engine.fileLimitReached) {
    await engine.blockFileLimit(rel);
    return { action: A.NOOP, blocked: true };
  }

  let created;
  try {
    created = await createNode(engine.conn, {
      type,
      name: path.basename(rel),
      parentId: engine.resolveParentIdByPath(dirOf(rel)),
      content,
      modifiedAt: (await fs.stat(localPath)).mtime,
    });
  } catch (error) {
    if (error.statusCode === 409 && error.code === 'name-conflict') {
      const record = await recordConflict(engine, nodeId, rel, {
        kind: store.KINDS.NAME_TAKEN,
        localChecksum,
        fetchRemote: false,
      });
      return { action: A.CONFLICT, kind: store.KINDS.NAME_TAKEN, record };
    }
    if (!isSite && error.statusCode === 413 && error.code === 'too-large') {
      if (Number.isFinite(error.limit)) await engine.setUploadLimit(error.limit);
      return keepLocal(engine, rel, buffer.length, Number.isFinite(error.limit) ? error.limit : engine.uploadLimit);
    }
    if (isSite && error.statusCode === 403 && error.code === 'file-limit') {
      engine.fileLimitReached = true;
      await engine.blockFileLimit(rel);
      return { action: A.NOOP, blocked: true };
    }
    throw error;
  }
  if (gen !== engine.generation) return { action: A.CREATE_REMOTE, stale: true };

  engine.outbox.markInFlight('save', created.id);
  await sendPageState(engine, created.id, rel);
  const etag = created.etag || created.checksum || localChecksum;
  await saveEntry(engine, created.id, null, {
    remoteEtag: etag,
    localChecksum: etag,
    structureVersion: created.structureVersion,
  }, await entryMeta(engine, rel, type, created.parentId));

  // The cached list predates this node; a later event must not read its absence as a delete.
  if (typeof engine.invalidateServerNodesCache === 'function') engine.invalidateServerNodesCache();

  const heldAtFileLimit = engine.uploadBlocks?.files?.[rel]?.reason === 'file-limit';
  await engine.clearUploadBlock(rel);
  // A site that landed means the plan has room again: the rest of this pass may
  // try, and a file held earlier is asked for again, one request at a time.
  if (isSite) engine.fileLimitReached = false;
  if (heldAtFileLimit) engine.retryFileLimitBlocks?.();
  engine.emit('file-synced', { file: rel, action: 'upload', type });
  return { action: A.CREATE_REMOTE, nodeId: created.id, etag };
}

async function forget(engine, nodeId) {
  if (nodeId !== null && nodeId !== undefined) await engine.repo.delete(nodeId);
  return { action: A.FORGET };
}

async function conflicted(engine, nodeId, entry, context, decision) {
  const rel = relPathOf(entry, context, nodeId);
  const kind = decision.conflictKind || store.KINDS.BOTH_EDITED;
  const record = await recordConflict(engine, nodeId, rel, {
    kind,
    localChecksum: await localChecksumOf(engine, rel),
    fetchRemote: kind !== store.KINDS.REMOTE_DELETED,
  });
  return { action: A.CONFLICT, kind, record };
}

/**
 * Carry out one `decide` result for one node.
 *
 * @param {object} session the session's engine (or a C2 entry holding `engine`)
 * @param {number|string|null} nodeId the node's id, or null for a file with no
 *   node id yet (then `context.path` names it)
 * @param {{action:string, conflictKind?:string}} decision the result of `decide`
 * @param {{path?:string, type?:string, parentId?:number, etag?:string, structureVersion?:string}} [context]
 *   the remote view for a node the baseline has no entry for
 * @returns {Promise<{action:string}>}
 */
async function executeDecision(session, nodeId, decision, context = {}) {
  const engine = engineOf(session);
  const gen = engine.generation;
  const entry = nodeId === null || nodeId === undefined ? null : engine.repo.get(nodeId);

  switch (decision.action) {
    case A.NOOP:
      await refreshStructureVersion(engine, nodeId, entry, context);
      return { action: decision.action };
    case A.DEFER:
      return { action: decision.action };
    case A.FORGET:
      return forget(engine, nodeId);
    case A.ADOPT:
      return adopt(engine, nodeId, entry, context);
    case A.UPLOAD:
      return upload(engine, nodeId, entry, context, gen);
    case A.DOWNLOAD:
      return download(engine, nodeId, entry, context, gen);
    case A.CONFLICT:
      return conflicted(engine, nodeId, entry, context, decision);
    case A.TRASH_LOCAL:
      return trashLocal(engine, nodeId, entry, context);
    case A.DELETE_REMOTE:
      return deleteRemote(engine, nodeId, entry, context);
    case A.CREATE_REMOTE:
      return createRemote(engine, nodeId, entry, context, gen);
    default:
      return { action: decision.action };
  }
}

async function removeCopy(engine, record) {
  if (!record || !record.remoteCopy) return;
  const result = await removeSyncFile(engine, record.remoteCopy)
    .catch((error) => ({ outcome: 'refused', reason: error.message }));
  if (result.outcome !== 'committed') console.error(`[SYNC] Could not remove ${record.remoteCopy}:`, result.reason);
}

/**
 * Pick a side for an open conflict record, found by its path. Both choices end
 * with the record closed and the parked copy removed; a 412 on `mine` refreshes
 * the record and leaves it open.
 *
 * @param {object} session the session's engine (or a C2 entry holding `engine`)
 * @param {{path:string, choice:'mine'|'theirs'}} input
 * @returns {Promise<{ok:boolean, error?:string}>}
 */
const KEEP_BOTH_KINDS = new Set([store.KINDS.BOTH_EDITED, store.KINDS.REJECTED]);

async function conflictedCopyPath(engine, rel) {
  const remote = new Set((engine.serverNodesCache || []).map(n => (n.path ? `${n.path}/${n.name}` : n.name)));
  for (let n = 1; n <= 100; n++) {
    const candidate = conflictedCopyName(rel, n);
    if (remote.has(candidate)) continue;
    if (!(await fileExists(path.join(engine.syncFolder, candidate)))) return candidate;
  }
  return null;
}

async function resolveConflict(session, { path: rel, choice } = {}) {
  const engine = engineOf(session);
  if (choice !== 'mine' && choice !== 'theirs' && choice !== 'both') return { ok: false, error: 'bad-choice' };

  const records = await store.load(engine.metaDir);
  const found = store.byPath(records, rel);
  if (!found) return { ok: false, error: 'no-conflict' };

  const id = idOf(found.key);
  const isSite = SITE_PATTERN.test(found.path);
  const type = isSite ? 'site' : 'upload';
  const localPath = await localPathFor(engine, found.path);
  const known = engine.repo.get(found.key);
  const extra = await entryMeta(engine, found.path, type, known ? known.parentId : undefined);

  let copy = null;
  let copiedAside;
  if (choice === 'both') {
    if (!KEEP_BOTH_KINDS.has(found.kind) || !found.remoteCopy) return { ok: false, error: 'no-remote-copy' };
    copy = await conflictedCopyPath(engine, found.path);
    if (!copy) return { ok: false, error: 'no-free-name' };
    const { buffer, checksum: copiedChecksum } = await readLocalBytes(engine, localPath);
    copiedAside = copiedChecksum;
    const written = await writeLocal(engine, copy, isSite ? buffer.toString('utf8') : buffer, { expectLocal: null, explicitCreate: true });
    if (written.refused) return { ok: false, error: 'copy-taken' };
    try {
      await createRemote(engine, null, null, { path: copy, type }, engine.generation);
    } catch (error) {
      console.error('[SYNC] keep both: the copy stays local and uploads on the next pass:', error && error.message ? error.message : error);
    }
  }

  if (choice === 'theirs' || choice === 'both') {
    if (found.kind === store.KINDS.REMOTE_DELETED) {
      await engine._applyFileDelete(id, found.path, type);
    } else {
      if (!found.remoteCopy) return { ok: false, error: 'no-remote-copy' };
      const other = await readFileBuffer(path.join(engine.syncFolder, found.remoteCopy));
      // A save that lands after the choice is not replaced: the write expects the bytes the
      // choice was made over (for Keep both, the bytes it just copied aside).
      const expectLocal = copy ? copiedAside : await localChecksumOf(engine, found.path);
      const restored = await writeLocal(engine, found.path, isSite ? other.toString('utf8') : other, { expectLocal });
      if (restored.refused) return { ok: false, error: 'changed' };
      await saveEntry(engine, found.key, known, {
        remoteEtag: found.remoteEtag,
        localChecksum: restored.checksum,
      }, extra);
    }
    await engine.clearUploadBlock(found.path);
    await store.clear(engine.metaDir, found.key);
    await removeCopy(engine, found);
    return copy ? { ok: true, copy } : { ok: true };
  }

  const size = (await fs.stat(localPath)).size;
  if (overLimit(engine, isSite, size)) {
    await keepLocal(engine, found.path, size, engine.uploadLimit);
    return { ok: false, error: 'too-large' };
  }
  const { buffer, checksum: localChecksum } = await readLocalBytes(engine, localPath);
  if (overLimit(engine, isSite, buffer.length)) {
    await keepLocal(engine, found.path, buffer.length, engine.uploadLimit);
    return { ok: false, error: 'too-large' };
  }
  const content = isSite ? buffer.toString('utf8') : buffer;
  const modifiedAt = (await fs.stat(localPath)).mtime;

  if (id === null || found.kind === store.KINDS.REMOTE_DELETED) {
    const created = await createNode(engine.conn, {
      type,
      name: path.basename(found.path),
      parentId: engine.resolveParentIdByPath(dirOf(found.path)),
      content,
      modifiedAt,
    });
    const etag = created.etag || created.checksum || localChecksum;
    await saveEntry(engine, created.id, null, {
      remoteEtag: etag,
      localChecksum: etag,
      structureVersion: created.structureVersion,
    }, await entryMeta(engine, found.path, type, created.parentId));
    await store.clear(engine.metaDir, found.key);
    await removeCopy(engine, found);
    return { ok: true };
  }

  let response;
  engine.outbox.markInFlight('save', id);
  try {
    response = await putNodeContent(engine.conn, id, content, { modifiedAt, ifMatch: found.remoteEtag });
  } catch (error) {
    if (error.statusCode === 412) {
      await refreshRemoteCopy(engine, id, found.path, found);
      return { ok: false, error: 'stale-etag' };
    }
    throw error;
  }

  const etag = response.etag || response.checksum || localChecksum;
  await saveEntry(engine, found.key, known, {
    remoteEtag: etag,
    localChecksum: etag,
    structureVersion: response.structureVersion,
  }, extra);
  await store.clear(engine.metaDir, found.key);
  await removeCopy(engine, found);
  return { ok: true };
}

module.exports = {
  executeDecision,
  resolveConflict,
  recordConflict,
};
