const fs = require('fs').promises;
const path = require('upath');
const {
  withFileLock,
  withDocumentWrite,
  withStructureChange,
  assertStructureContext,
} = require('../main/utils/write-queue');
const { getConsentRegistry, resolveWritePath, realpathNearestParent } = require('../main/utils/path-resolver');
const { LocalAuthorityError } = require('../main/local-authority');
const { createRootDocumentHost } = require('../main/document-host');

class StaleLocalWriteError extends Error {}

function documentHostFor(engine) {
  if (!engine.documentHost) {
    engine.documentHost = createRootDocumentHost({
      ctx: { root: { id: engine.rootId, path: engine.syncFolder }, localAuthority: engine.localAuthority },
      live: engine.live, emit: engine.emit?.bind(engine), logger: engine.logger,
      getObserver: () => engine._subscribedObserver ?? engine.observer,
    });
  }
  return engine.documentHost;
}

async function canonicalPathFor(engine, rel) {
  engine.resolveContainedPath(rel);
  return resolveWritePath(getConsentRegistry(engine.syncFolder), rel);
}

async function structureEntryFor(engine, rel) {
  const registry = getConsentRegistry(engine.syncFolder);
  await registry.ready();
  engine.resolveContainedPath(rel);
  const lexical = path.resolve(path.join(engine.syncFolder, rel));
  const parent = await realpathNearestParent(path.dirname(lexical));
  const location = path.join(parent, path.basename(lexical));
  const stat = await fs.lstat(location).catch((error) => {
    if (error.code === 'ENOENT' || error.code === 'ENOTDIR') return null;
    throw error;
  });
  if (!stat) return { location, canonical: location, exists: false };
  return {
    location,
    canonical: stat.isSymbolicLink() ? await resolveWritePath(registry, rel) : location,
    isDir: stat.isDirectory(),
    isSymlink: stat.isSymbolicLink(),
    exists: true,
  };
}

async function claimLocalDocumentTarget(engine, rel) {
  const canonicalPath = await canonicalPathFor(engine, rel);
  const authority = engine.localAuthority;
  const admissionClaim = authority ? authority.captureAdmission({
    rootId: engine.rootId, logicalPath: rel, canonicalPath,
  }) : null;
  return Object.freeze({ canonicalPath, admissionClaim, target: null });
}

async function withLocalDocumentWrite(engine, rel, fn, { generation = engine.generation, claim = null, explicitCreate = false } = {}) {
  const admitted = claim ?? await claimLocalDocumentTarget(engine, rel);
  const claimedPath = admitted.canonicalPath;
  const authority = engine.localAuthority;
  if (!authority) {
    return withFileLock(claimedPath, () => fn({ canonicalPath: claimedPath }, null));
  }

  const admit = async (token) => {
    if (generation !== engine.generation) throw new StaleLocalWriteError();
    const currentPath = await canonicalPathFor(engine, rel);
    if (currentPath !== claimedPath) throw new StaleLocalWriteError();
    const location = { rootId: engine.rootId, logicalPath: rel, canonicalPath: currentPath };
    const absent = explicitCreate &&
      await fs.stat(currentPath).then(() => false, (error) => error.code === 'ENOENT');
    if (explicitCreate && authority.isRetired(location) && !absent) throw new StaleLocalWriteError();
    try {
      return authority.admitWriteTarget(admitted.admissionClaim, {
        ...location, origin: null, live: engine.live,
      }, token, { explicitCreate: absent });
    } catch (error) {
      if (error instanceof LocalAuthorityError && error.code === 'moved') throw new StaleLocalWriteError();
      throw error;
    }
  };
  const boundary = (work) => withDocumentWrite(async () => ({ canonicalPath: claimedPath }), (_locked, token) => work(token));
  try {
    // A host that consults the service back (releasesBoundary) must be called with
    // no boundary held, or its settle waits on this one (protocol v1: the service
    // never holds a lock or boundary of its own across a call to Go). The admission
    // checks still run under the boundary; the write runs after it, under Go's lock.
    if (documentHostFor(engine).releasesBoundary) {
      const target = await boundary(admit);
      return await fn(target, null);
    }
    return await boundary(async (token) => fn(await admit(token), token));
  } catch (error) {
    if (error instanceof StaleLocalWriteError) return { stale: true };
    throw error;
  }
}

async function withLocalDirectoryChange(engine, rel, fn, { generation = engine.generation } = {}) {
  const claimedPath = await canonicalPathFor(engine, rel);
  const authority = engine.localAuthority;
  if (!authority) return fn({ canonicalPath: claimedPath }, null);

  const location = { rootId: engine.rootId, logicalPath: rel, canonicalPath: claimedPath };
  const claim = authority.captureAdmission(location);
  const check = async (token) => {
    assertStructureContext(token);
    if (generation !== engine.generation) throw new StaleLocalWriteError();
    const currentPath = await canonicalPathFor(engine, rel);
    if (currentPath !== claimedPath) throw new StaleLocalWriteError();
    try {
      authority.validateAdmission(claim, location, token, { allowRetired: true });
    } catch (error) {
      if (error instanceof LocalAuthorityError && error.code === 'moved') throw new StaleLocalWriteError();
      throw error;
    }
    return currentPath;
  };
  const settle = async (currentPath, before, token) => {
    const after = await fs.stat(currentPath).then((stat) => stat, (error) => {
      if (error.code === 'ENOENT') throw new StaleLocalWriteError();
      throw error;
    });
    if (!after.isDirectory()) throw new StaleLocalWriteError();
    if (!before) authority.setAdmission(token, { ...location, active: true, prefix: true });
  };
  try {
    const planned = await withStructureChange(async (token) => {
      const currentPath = await check(token);
      const retired = authority.isRetired(location);
      const before = await fs.stat(currentPath).then((stat) => stat, (error) => {
        if (error.code === 'ENOENT') return null;
        if (retired) throw new StaleLocalWriteError();
        throw error;
      });
      if (retired && before) throw new StaleLocalWriteError();
      if (documentHostFor(engine).releasesBoundary) return { currentPath, before };
      const result = await fn({ canonicalPath: currentPath }, token);
      await settle(currentPath, before, token);
      return { done: true, result };
    });
    if (planned.done) return planned.result;
    // Released: the folder is made with no boundary held (Go's structure lock
    // orders it), then the boundary is taken again to record what landed.
    const result = await fn({ canonicalPath: planned.currentPath }, null);
    await withStructureChange(async (token) => {
      await check(token);
      await settle(planned.currentPath, planned.before, token);
    });
    return result;
  } catch (error) {
    if (error instanceof StaleLocalWriteError) return { stale: true };
    throw error;
  }
}

// Sync's own files (the marker, the kept server copies) and the folders a download makes
// are written by the root's document host, like every other write in a root.
async function putSyncFile(engine, rel, bytes) {
  const result = await documentHostFor(engine).syncFile({ op: 'put', rel, bytes });
  if (result.outcome !== 'committed') {
    throw Object.assign(new Error(`${rel} was not written: ${result.reason}`), { code: result.reason });
  }
}

async function removeSyncFile(engine, rel) {
  return documentHostFor(engine).syncFile({ op: 'remove', rel });
}

async function ensureLocalFolder(engine, rel) {
  const result = await documentHostFor(engine).ensureFolder(rel);
  if (result.outcome !== 'committed') {
    throw Object.assign(new Error(`the folder ${rel || '.'} was not made: ${result.reason}`), { code: result.reason });
  }
  return result.created;
}

function relocateEntryFor(engine, rel) {
  const location = path.join(engine.syncFolder, rel);
  return { location, canonical: location };
}

async function restructureMove(engine, fromRel, toRel) {
  return documentHostFor(engine).restructure({
    kind: 'sync-move',
    rootId: engine.rootId,
    steps: [{ from: fromRel, to: toRel }],
    context: {
      token: null,
      entries: [{ from: relocateEntryFor(engine, fromRel), to: relocateEntryFor(engine, toRel) }],
    },
  });
}

async function restructureTrash(engine, rel, trashRel) {
  return documentHostFor(engine).restructure({
    kind: 'sync-trash',
    rootId: engine.rootId,
    steps: [{ from: rel, to: trashRel }],
    context: {
      token: null,
      entries: [{ from: relocateEntryFor(engine, rel), to: relocateEntryFor(engine, trashRel) }],
    },
  });
}

// A sync structure change either completes or throws, so the engine never records a move or a
// trash the disk did not make. The host reports a failure as a result; this turns it back into
// the error the old fs call would have thrown.
function requireCommitted(result) {
  if (result.outcome === 'committed') return result;
  throw result.error ?? new Error(`the document host reported this structure change as ${result.outcome}`);
}

// A structure change in two halves: plan runs under the structure boundary and
// answers { done } to stop there, or what act needs. With a host that releases
// the boundary, act runs after the boundary ends (token null), so Go's prepare
// can take it; with any other host, act runs inside it as before.
async function withLocalStructurePlan(engine, plan, act) {
  const released = Boolean(documentHostFor(engine).releasesBoundary);
  const step = await withStructureChange(async (token) => {
    assertStructureContext(token);
    const planned = await plan(token);
    if (planned.done) return { result: planned.done };
    return released ? { planned } : { result: await act(planned, token) };
  });
  return step.planned ? act(step.planned, null) : step.result;
}

async function withLocalStructureDelete(engine, rel, trashRel, { generation = engine.generation, isDir = false, fallbackRel = null } = {}) {
  const authority = engine.localAuthority;
  if (!authority) return { legacy: true };
  const plan = async (token) => {
    if (generation !== engine.generation) return { done: { stale: true } };
    const source = await structureEntryFor(engine, rel);
    if (!source.exists) {
      authority.setAdmission(token, {
        rootId: engine.rootId,
        logicalPath: rel,
        canonicalPath: source.canonical,
        active: false,
        prefix: isDir,
      });
      return { done: { missing: true } };
    }
    return { source };
  };
  return withLocalStructurePlan(engine, plan, async ({ source }, token) => {
    const trash = async (toRel) => {
      const destination = relocateEntryFor(engine, toRel);
      return documentHostFor(engine).restructure({
        kind: 'sync-trash',
        rootId: engine.rootId,
        steps: [{ from: rel, to: toRel }],
        context: {
          token,
          entries: [{ from: source, to: { location: destination.location, canonical: destination.location } }],
        },
      });
    };
    const first = await trash(trashRel);
    if (first.outcome !== 'failed' || !fallbackRel) return requireCommitted(first);
    return requireCommitted(await trash(fallbackRel));
  });
}

module.exports = { withLocalDocumentWrite, withLocalDirectoryChange, withLocalStructureDelete, withLocalStructurePlan, claimLocalDocumentTarget, structureEntryFor, documentHostFor, restructureMove, restructureTrash, requireCommitted, putSyncFile, removeSyncFile, ensureLocalFolder };
