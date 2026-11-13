const fs = require('fs').promises;
const { calculateChecksum } = require('../sync-engine/utils');
const path = require('upath');
const { logicalBytes } = require('./page-state-store.js');
const { publishRecoveredCopy } = require('./utils/backup.js');
const { reader } = require('./document-reader');
const { normCanonical } = require('./local-authority');

const KINDS = {
  'browser-save': { lane: 'save', origin: 'save' },
  'data-api': { lane: 'save', origin: 'api' },
  restore: { lane: 'replace' },
  'guard-restore': { lane: 'replace' },
  'sync-download': { lane: 'exact' },
  'external-rebase': { lane: 'external-rebase' },
  workspace: { lane: null },
  create: { lane: 'create' },
};

const REQUIRED_DEPS = [
  'ctx', 'baseDir', 'live', 'store', 'resolveTailwindWrite',
  'canonicalForWrite', 'formatHtml', 'scopeTailwindLink', 'stripSaveToken', 'splitWrite',
  'storePageState', 'writePageState', 'logicalDocument',
  'createBackup', 'createBackupIfExists', 'createBinaryBackupIfExists', 'atomicWriteFile',
  'assertWriteContext', 'assertStructureContext', 'stripCopyBytes',
  'writeApiSidecar', 'refreshDerivedArtifacts', 'getTailwindCssName', 'compileTailwind', 'dataGuard', 'documentEtag',
  'rebaseStaleSave', 'newestVersionFiles', 'needsAdoption', 'roomsPackage',
  'MAX_EXTERNAL_HTML', 'VERSIONS_DIR', 'TAILWIND_DIR',
  'emit', 'logger',
];

const MAX_EXTERNAL_HTML = 12 * 1024 * 1024;

const DOCUMENT_NAME = /\.(html|htmlclay)$/i;

const METADATA_DIR = '.hyperclay';

const roomsPackage = () => require('@panphora/clayjs/server/rooms');

// A tab still on the runtime from before the Block editor rename saves its live tag
// under the old name, which this host no longer reads. Its write is refused before
// every other check, so it always gets ClayJS's refusal and not a conflict, and it is
// never told to reload: the person keeps the tab and copies their words out first.
function refuseOldFormat(html) {
  const { oldBlockeditorFormat, oldFormatRefusal } = roomsPackage();
  return oldBlockeditorFormat(String(html)) ? oldFormatRefusal() : null;
}

// The logical document a write stores and the clean file part that reaches disk.
async function splitWrite(baseDir, name, html, { keepState = true } = {}) {
  const { file, state } = roomsPackage().splitPage(html);
  if (state) return { file, logical: roomsPackage().composePage(file, state) };
  return { file, logical: keepState ? await require('./page-state-store.js').logicalDocument(baseDir, name, file) : file };
}

// The other direction, for the bytes that become the document. A browser holding a
// projected document saves what it holds, and nothing guarantees the client puts the
// public URLs back: an older ClayJS, a hand-written save, the JSON data API, a restored
// version or an incoming relay frame can all carry `/_/clayjs/` URLs or the descriptor.
// Bytes like that only work on this host, and platform sync would upload them. Every
// authoritative write turns them back into the public URLs and drops the descriptor
// first. One pass, so the descriptor is read once: the document URL is the first loopback
// origin the text names before `/_/clayjs/` (any port, since each root server listens on
// its own), and a root-relative `/_/clayjs/` URL resolves under any origin.
const LOOPBACK_RUNTIME = /\bhttps?:\/\/(?:localhost|127\.0\.0\.1)(?::\d{1,5})?(?=\/_\/clayjs\/)/i;

function canonicalForWrite(html) {
  const runtimeAssets = require('./runtime-assets');
  const origin = html.match(LOOPBACK_RUNTIME);
  const documentURL = origin ? `${origin[0].toLowerCase()}/` : 'http://localhost/';
  const result = runtimeAssets.canonicalizeRuntimeHTML(html, { releases: runtimeAssets.releases, documentURL });
  return result.changed === 0 && !result.descriptorRemoved ? html : result.html;
}

function refused(reason) {
  return { outcome: 'refused', reason, etag: null, stored: null, content: null };
}

const STRUCTURE_KINDS = {
  move: { retire: false, copy: false, crossDevice: true },
  trash: { retire: true, copy: false, crossDevice: true },
  duplicate: { copy: true },
  'sync-move': { retire: false, copy: false, crossDevice: true },
  'sync-trash': { retire: true, copy: false, crossDevice: true },
};

function withCause(result, error) {
  if (error) Object.defineProperty(result, 'error', { value: error, enumerable: false });
  return result;
}

async function moveEntry(from, to, { crossDevice }) {
  await fs.mkdir(path.dirname(to), { recursive: true });
  try {
    await fs.rename(from, to);
    return false;
  } catch (error) {
    if (!crossDevice || error.code !== 'EXDEV') throw error;
    await fs.copyFile(from, to);
    await fs.unlink(from);
    return true;
  }
}

function createDocumentHost(deps) {
  for (const name of REQUIRED_DEPS) {
    if (deps[name] === undefined) throw new TypeError(`createDocumentHost is missing ${name}`);
  }
  const {
    ctx, baseDir, live, resolveTailwindWrite, emit, logger,
    canonicalForWrite, formatHtml, scopeTailwindLink, stripSaveToken, splitWrite,
    storePageState, writePageState, logicalDocument, createBackup, createBackupIfExists, createBinaryBackupIfExists,
    atomicWriteFile, assertWriteContext, assertStructureContext, stripCopyBytes,
    writeApiSidecar, refreshDerivedArtifacts, getTailwindCssName, compileTailwind, documentEtag, roomsPackage, TAILWIND_DIR,
    store, rebaseStaleSave, newestVersionFiles, needsAdoption, dataGuard, MAX_EXTERNAL_HTML, VERSIONS_DIR,
    withDocument = require('./utils/write-queue').withDocumentWrite,
    loadCheckpoint = require('./utils/page-api').loadCheckpoint,
    createDocument = null,
    getObserver = () => null,
  } = deps;

  async function placeBytes({ name, filePath, logical, durable, beforeRename }) {
    const stored = await storePageState(baseDir, name, logical, durable);
    dataGuard.markPending(baseDir, name, stored);
    try {
      await atomicWriteFile(filePath, stored, 'utf8', beforeRename, durable);
    } catch (error) {
      if (error && error.afterRename) error.indeterminate = true;
      throw error;
    }
    live.markBrowserSave(name);
    return stored;
  }

  async function refreshTailwind(content, { quiet = false } = {}) {
    const tailwindName = getTailwindCssName(content);
    if (!tailwindName) return;
    try {
      const css = await compileTailwind(content);
      const cssPath = await resolveTailwindWrite(tailwindName);
      await atomicWriteFile(cssPath, css);
      if (!quiet) console.log(`Generated Tailwind CSS: ${TAILWIND_DIR}/${tailwindName}.css`);
    } catch (e) {
      if (!quiet) console.error('compileTailwind failed (non-fatal):', e && e.message ? e.message : e);
    }
  }

  async function read(name, filePath) {
    let bytes;
    try {
      bytes = await fs.readFile(filePath);
    } catch (error) {
      if (error.code === 'ENOENT') return null;
      throw error;
    }
    return { bytes, checksum: await calculateChecksum(bytes) };
  }

  function events(since, listener) {
    const observer = getObserver();
    if (!observer) throw new Error('This document host has no observer');
    if (since !== null) listener({ type: 'gap', since });
    return observer.subscribe(({ event, rel }) => listener({ type: 'change', event, rel }));
  }

  async function preconditionFailed(request) {
    const { expected } = request;
    if (!expected || expected.type === 'any') return false;
    const current = await read(request.name, request.context?.target?.canonicalPath ?? request.filePath);
    if (expected.type === 'absent') return current !== null;
    if (expected.type === 'checksum') return current === null || current.checksum !== expected.value;
    throw new TypeError(`Unknown expected type: ${expected.type}`);
  }

  async function confirmExact({ name, stored, target, token }) {
    const authority = ctx.localAuthority;
    const options = { sender: 'sync-engine' };
    if (!DOCUMENT_NAME.test(name) || typeof stored !== 'string') {
      return authority.confirmKnownWrite(target, stored, token, options);
    }
    const html = await logicalDocument(baseDir, name, stored);
    const room = authority.rooms.get(normCanonical(target.canonicalPath));
    if (room && authority.roomIsDirty(room)) return authority.observeExternal(target, html, token);
    return authority.confirmKnownWrite(target, html, token, options);
  }

  async function commitExact({ name, filePath: requestedPath, bytes, modifiedAt = null, blockeditorRecord = null, context = {} }) {
    const { target = null, token = null } = context;
    const authorityWrite = Boolean(ctx.localAuthority && target && token);
    if (authorityWrite) assertWriteContext(token, target.canonicalPath);
    const filePath = authorityWrite ? target.canonicalPath : requestedPath;
    const isDocument = typeof bytes === 'string' && DOCUMENT_NAME.test(name);
    const previous = isDocument ? await fs.readFile(filePath, 'utf8').catch(() => null) : null;
    if (isDocument) {
      await createBackupIfExists(filePath, name.replace(DOCUMENT_NAME, ''), baseDir, emit, logger);
      if (blockeditorRecord) await writePageState(baseDir, name, blockeditorRecord);
    } else {
      await createBinaryBackupIfExists(filePath, name, baseDir, emit, logger);
    }
    const stored = isDocument ? await storePageState(baseDir, name, bytes) : bytes;
    if (isDocument) dataGuard.markPending(baseDir, name, stored);
    try {
      await atomicWriteFile(filePath, stored, Buffer.isBuffer(stored) ? null : 'utf8');
    } catch (error) {
      if (error && error.afterRename) error.indeterminate = true;
      throw error;
    }
    live.markBrowserSave(name);
    try {
      if (modifiedAt) {
        const stamp = new Date(modifiedAt);
        await fs.utimes(filePath, stamp, stamp);
      }
      if (authorityWrite) await confirmExact({ name, stored, target, token });
    } catch (error) {
      error.indeterminate = true;
      throw error;
    }
    if (isDocument) {
      try {
        await refreshDerivedArtifacts(baseDir, name, bytes);
      } catch (error) {
        if (!token) throw error;
        console.error('[SYNC] derived artifact refresh failed:', error && error.message ? error.message : error);
      }
      dataGuard.runDataLossGuard({ baseDir, name, newHtml: bytes, prevContent: previous, prov: 'external', live })
        .catch(err => console.error('[data-guard] sync guard error:', err && err.message ? err.message : err));
    }
    return { outcome: 'committed', reason: null, etag: null, checksum: await calculateChecksum(stored), stored, content: bytes };
  }

  // The replace lane's first half, which consults no room and writes nothing.
  async function prepareReplace({ name, bytes }) {
    // Response-scoped root metadata (a save token, this host's own documentetag)
    // comes back from clients and from a stored version, and none of it belongs in
    // the bytes this writes: stripped here, once, so the data-loss writeBack and the
    // restore route cannot each forget it.
    const html = canonicalForWrite(stripSaveToken(bytes));
    const formatted = formatHtml(scopeTailwindLink(name, html));
    const write = await splitWrite(baseDir, name, formatted);
    return { content: write.logical, fileContent: write.file };
  }

  // The replace lane's admission, before the first backup: the room's replacement
  // reservation, and with `safety` a version of what is being replaced (the room's
  // unsaved edits rendered over it when a room is open) plus, for an edit of the
  // page, a recovered copy of those edits. A refusal is answered, not thrown, as
  // the lane refuses; a reservation taken before a refusal or a throw is released
  // here.
  async function admitReplace({
    name, filePath, content, edit = false, safety = false, target = null, token = null,
    readCurrent = async () => logicalBytes(baseDir, name, await reader().readFile(filePath)),
    writeSafetyVersion = html => createBackup(baseDir, name.replace(/\.(html|htmlclay)$/, ''), html),
    writeRecoveredCopy = html => publishRecoveredCopy(baseDir, name, html),
  }) {
    const authority = ctx.localAuthority;
    const authorityWrite = Boolean(authority && target && token);
    let pageToken = null;
    const refuse = reason => {
      if (pageToken) authority.rooms.releasePage(pageToken);
      pageToken = null;
      return { pageToken: null, refusal: reason };
    };
    try {
      if (authorityWrite && !edit) {
        // Explicit replacement admission, before the first backup, and held across
        // the backup and the atomic write: no frame can change the room while the
        // disk result is pending. An edit of the page (a restore stamped with the
        // record's rev) continues the room instead of replacing it, so it reserves
        // nothing and the open tabs merge the result.
        pageToken = authority.rooms.reservePageReplacement(filePath, {
          nodeId: target.identity,
          html: content,
        });
      }
      if (safety) {
        let current = null;
        try {
          current = await readCurrent();
        } catch (error) {
          if (error.code !== 'ENOENT') {
            console.error(`Refusing to replace ${name}: current file cannot be read`);
            return refuse('room-unavailable');
          }
        }
        if (current !== null) {
          const currentHtml = current.toString('utf8');
          let safetyHtml;
          try {
            // An edit of the page keeps the room: its unsaved edits stay in it and
            // merge the restored article, so the backup is the plain document and
            // the room's own edits are rendered into a recovered copy below.
            safetyHtml = edit || !ctx.localCheckpoint
              ? currentHtml
              : await ctx.localCheckpoint.safetyBackup(filePath, currentHtml);
          } catch (error) {
            console.error(`Refusing to replace ${name}: live room cannot be backed up`, error);
            return refuse('room-unavailable');
          }
          const safetyCopy = await writeSafetyVersion(safetyHtml);
          if (!safetyCopy) {
            console.error(`Refusing to replace ${name}: safety backup failed`);
            return refuse('backup-failed');
          }
          // The room keeps its unsaved edits through an edit-restore, so they are
          // written to a recovered copy too: they survive even if no tab adopts
          // the restored article. A rendering failure never stops the restore.
          if (edit && ctx.localCheckpoint && ctx.localCheckpoint.dirty(ctx.localAuthority?.rooms.get(normCanonical(filePath)))) {
            try {
              const unsaved = await ctx.localCheckpoint.safetyBackup(filePath, currentHtml);
              await writeRecoveredCopy(unsaved);
            } catch (error) {
              console.error(`Recovered copy of ${name} failed (non-fatal):`, error && error.message ? error.message : error);
            }
          }
        }
      }
      return { pageToken, refusal: null };
    } catch (error) {
      if (pageToken) authority.rooms.releasePage(pageToken);
      throw error;
    }
  }

  // The replace lane's confirmation half, the moment the bytes are on disk.
  async function settleReplace({ name, content, pageToken = null, target = null, token = null }) {
    const authority = ctx.localAuthority;
    if (authority && target && token) {
      // The bytes are on disk: the reservation commits into the one room and the
      // authority stamps and publishes exactly what landed, once per registered
      // alias of this canonical file. That is why the old single-root broadcast
      // below is the no-authority path only.
      if (pageToken) authority.rooms.commitPage(pageToken);
      await authority.confirmKnownWrite(target, content, token, { sender: 'server-save' });
    } else {
      // Morph view-mode tabs with the persisted on-disk HTML.
      live.broadcast(name, { html: roomsPackage().splitPage(content).file, sender: 'server-save' }, { lane: 'saved' });
    }
  }

  // The one publish sequence for a write this app makes to a live document — the
  // data-loss guard's writeBack and F1's restore both call it, so the two cannot
  // drift. Format as the save path does, version the new bytes, write atomically,
  // mark the write as ours so the watcher does not re-run the guard on it, morph
  // view-mode tabs, then refresh everything derived from the bytes. Non-fatal on
  // the derived artifacts, for the same reason the save route is: the document is
  // already written, backed up and broadcast by that point.
  // Returns the composed document, which is what the stamp describes.
  async function commitReplace({ name, filePath: requestedPath, bytes, edit = false, safety = false, context = {} }) {
    const { target = null, token = null } = context;
    let filePath = requestedPath;
    const backupName = name.replace(/\.(html|htmlclay)$/, '');
    const { content, fileContent } = await prepareReplace({ name, bytes });
    // An already owned helper: a caller arrives inside runDocumentWrite's boundary,
    // so this claims nothing and locks nothing of its own. With an authority it
    // proves that boundary and writes the path that boundary admitted.
    const authority = ctx.localAuthority;
    const authorityWrite = Boolean(authority && target && token);
    if (authorityWrite) {
      assertWriteContext(token, target.canonicalPath);
      filePath = target.canonicalPath;
    }
    let pageToken = null;
    let stored;
    try {
      const admitted = await admitReplace({ name, filePath, content, edit, safety, target, token });
      if (admitted.refusal) return refused(admitted.refusal);
      pageToken = admitted.pageToken;
      await createBackup(baseDir, backupName, fileContent);
      stored = await placeBytes({ name, filePath, logical: content });
      await settleReplace({ name, content, pageToken, target, token });
    } finally {
      if (pageToken) authority.rooms.releasePage(pageToken);
    }
    try { await writeApiSidecar(baseDir, name, content); } catch {}
    await refreshTailwind(content, { quiet: true });
    return { outcome: 'committed', reason: null, etag: documentEtag(content), stored, content };
  }

  // The save lane's first half, which consults no room and writes nothing:
  // Local's normalization, the rebase of a save made against an older page, and
  // the split into the logical document and the file part that reaches disk.
  async function prepareSave({ name, bytes, blockeditorConversion = null }) {
    const refusal = refuseOldFormat(bytes);
    if (refusal) throw refusal;
    // Normalized once, and before anything is admitted or backed up: the page
    // admission below has to decide against the exact bytes this call would
    // store, and a refusal must leave no backup and no written byte behind.
    let content = canonicalForWrite(formatHtml(scopeTailwindLink(name, bytes)));

    // The logical document keeps the page editor's merge state and is what every
    // admission, stamp, broadcast and answer below describes. Only the file part
    // reaches disk, with that state kept in its sidecar. Composing the two back
    // together makes `content` exactly what a later read of this save composes,
    // so the stamp returned here is the one the next If-Match is judged against.
    //
    // A client that never held the state — the code editor is sent the clean
    // file — saves the document without it. It is still the same document, so
    // the state its sidecar holds is put back here too. A page conversion is the
    // one save where the absence is the point, so that one is never put back.
    if (!blockeditorConversion) {
      const rebased = await rebaseStaleSave(baseDir, name, content, reader().versionFiles ?? newestVersionFiles);
      if (rebased) content = rebased.html;
    }
    const write = await splitWrite(baseDir, name, content, { keepState: !blockeditorConversion });
    return { content: write.logical, fileContent: write.file };
  }

  // The save lane's room admission, before the first backup. Returns the
  // admitted document, its file part with the page format floor, and the page
  // reservation, which the caller releases once the write is settled. A throw
  // after a reservation was taken releases it here, so it never outlives a
  // refused save.
  async function admitSave({
    name, filePath, origin, content, fileContent, dataLossPrev = null, ifMatch = null,
    blockeditorConversion = null, blockeditorWrite = null, pageFormat = null, persistentToken = null, target, token,
    writeSafetyVersion = html => createBackup(baseDir, name.replace(/\.(html|htmlclay)$/, ''), html),
  }) {
    const authority = ctx.localAuthority;
    let pageToken = null;
    try {
      // The shared rooms package owns page authority: an initialization or a
      // conversion is admitted here, before the first backup, and its
      // reservation is held across the backup and the atomic write so no
      // frame can change the room while the disk result is pending.
      const { checkPageWriteAdmission, pageLineageLost, checkPageFormatAdmission, formatCapability, withPageFormatFloor, PAGE_FORMAT, PAGE_FORMAT_UPGRADE } = require('@panphora/clayjs/server/rooms');
      const { createDocument } = require('./page-rebase.js');
      const rooms = authority.rooms;
      const open = rooms.get(normCanonical(filePath));
      const room = open && String(open.nodeId) === String(target.identity) ? open : null;
      const currentHtml = dataLossPrev ?? '';
      const admission = checkPageWriteAdmission({
        currentHtml, html: content, header: blockeditorConversion ?? null, ifMatch, room,
        blockeditorWrite: blockeditorWrite ?? null, createDocument,
      });
      const capability = origin === 'checkpoint' ? PAGE_FORMAT : formatCapability(pageFormat);
      const admitted = authority.admittedFormats(filePath);
      const format = checkPageFormatAdmission({ currentHtml, html: content, capability, admitted });
      content = format.html;
      fileContent = withPageFormatFloor(fileContent, format.floor);
      if (admission === 'initial') {
        pageToken = rooms.reservePageInitial(filePath, {
          nodeId: target.identity, currentHtml, html: content, ifMatch,
          blockeditorWrite: blockeditorWrite ?? null, createDocument,
        });
      } else if (admission === 'conversion') {
        pageToken = rooms.reservePage(filePath, {
          nodeId: target.identity, header: blockeditorConversion, currentHtml, html: content, ifMatch,
          blockeditorWrite: blockeditorWrite ?? null, createDocument,
        });
      } else if (admission === 'ordinary' && room && pageLineageLost(room, content)) {
        // The open room still holds the article this save replaces, and the
        // document being written no longer carries the page tag that bound them:
        // an outside edit or a code save replaced the page under it. Restore's
        // pattern, applied here: the room's unsaved edits become a version of
        // their own, and the room restarts on the new document instead of
        // keeping the old page store, which its next checkpoint would render
        // back over the replacement. A room whose edits cannot be kept is
        // refused, so the person can save them rather than lose them.
        if (authority.roomIsDirty(room)) {
          let kept = null;
          try {
            kept = ctx.localCheckpoint
              ? await ctx.localCheckpoint.safetyBackup(filePath, currentHtml)
              : null;
          } catch (error) {
            console.error(`Refusing a save of ${name}: the open room cannot be backed up`, error);
          }
          if (kept === null) {
            const error = new Error('Unsaved edits could not be kept. Save them before replacing this page.');
            error.code = 'unsaved-edits';
            throw error;
          }
          if (!(await writeSafetyVersion(kept))) {
            const error = new Error('Unsaved edits could not be kept. Save them before replacing this page.');
            error.code = 'unsaved-edits';
            throw error;
          }
        }
        pageToken = rooms.reservePageReplacement(filePath, {
          nodeId: target.identity, html: content,
        });
      }
      if (persistentToken) rooms.checkPersistent(persistentToken, { html: content });
      // An outside edit the room keeps (same revision or merged onto an older
      // one) waits for a tab to adopt it, and until then the room's unsaved
      // edits live only in memory: they get a recovered copy, rendered over
      // the document this write replaces.
      if (room && origin !== 'checkpoint' && !pageToken && content.includes('data-blockeditor') && needsAdoption(content)) {
        await authority.recoverUnsavedEdits(target, token);
      }
      return { content, fileContent, pageToken };
    } catch (error) {
      if (pageToken) authority.rooms.releasePage(pageToken);
      throw error;
    }
  }

  // The save lane's confirmation half, run the moment the bytes are on disk.
  async function settleSave({ name, origin, content, fileContent, pageToken = null, target = null, token = null, persistentToken = null }) {
    const authority = ctx.localAuthority;
    const authorityWrite = Boolean(authority && target && token);
    // The bytes are on disk: the page reservation commits into the one room,
    // and the authority stamps and publishes exactly what landed, once per
    // registered alias of this canonical file.
    if (pageToken) authority.rooms.commitPage(pageToken);
    if (authorityWrite) {
      await authority.confirmKnownWrite(target, content, token, {
        sender: origin === 'api' ? 'data-api' : 'server-save', persistentToken,
      });
    }

    // Morph view-mode tabs with the persisted on-disk HTML. Edit-mode tabs
    // are untouched — they sync via /live-sync/save on the live lane.
    //
    // A data API write has no tab behind it to relay its snapshot, so it has to
    // reach edit-mode tabs the way an external disk change does: the same
    // `external-change` notify the observer's _notifyExternal sends, so ClayJS
    // runs it through dirty-region protection before morphing. The saved-lane
    // broadcast stays, for the viewers. Same payload as the observer, different
    // sender — the writer here is the data API, not the file system.
    //
    // With an authority the saved lane is already published, once per alias, by
    // the confirmation above; this route is still its own publisher for a
    // no-authority caller, which is every legacy createApp(dir).
    const adopt = origin !== 'checkpoint' && content.includes('data-blockeditor') && needsAdoption(content);
    if (adopt) {
      ctx.observer?.publishExternal(
        name,
        Buffer.from(content, 'utf8'),
        `${path.basename(name)} was changed outside the editor`,
        { saved: false, continues: true }
      );
    }
    if (origin === 'api' && !adopt) {
      live.notify(name, {
        msgType: 'warning',
        msg: `${path.basename(name)} was updated through the data API`,
        action: 'reload',
        data: Buffer.byteLength(fileContent, 'utf8') <= MAX_EXTERNAL_HTML
          ? { kind: 'external-change', html: fileContent, sender: 'data-api', etag: documentEtag(content) }
          : { kind: 'external-change', sender: 'data-api' },
      });
    }
    if (!authorityWrite) {
      live.broadcast(name, { html: fileContent, sender: origin === 'api' ? 'data-api' : 'server-save' }, { lane: 'saved' });
    }
  }

  // The provenance bit platform sync reads when it uploads this page.
  function recordSaveProvenance(name, userDriven) {
    // Record the provenance bit for platform sync. The sync engine reads it
    // when uploading, so the platform guard can split a UI save from a
    // background-script save. The snapshot beside it comes from
    // /live-sync/save, which is why this merges rather than replaces: the two
    // lanes contribute different halves of the same entry.
    {
      const prev = store.snapshots.get(name);
      store.snapshots.set(name, {
        html: prev ? prev.html : null,
        userDriven,
        timestamp: Date.now()
      });
    }
  }

  async function commitSave(request, { origin }) {
    const {
      name, filePath, bytes, dataLossPrev, userDriven, saveId,
      ifMatch, blockeditorConversion, blockeditorWrite, pageFormat,
      persistentToken = null, readIdentity = null, context = {},
    } = request;
    const { target = null, token = null } = context;
    // ANCILLARY DISK CONVENTION: versions dir and backups use baseName without
    // extension (matches platform's versions/{baseName}/ layout).
    // Do NOT reuse `backupName` as a liveSync channel key — liveSync keys must
    // carry the extension (Rule 1).
    const backupName = name.replace(/\.(html|htmlclay)$/, '');

    let { content, fileContent } = await prepareSave({ name, bytes, blockeditorConversion });

    const authority = ctx.localAuthority;
    const authorityWrite = Boolean(authority && target && token);
    let pageToken = null;
    try {
      if (authorityWrite) {
        ({ content, fileContent, pageToken } = await admitSave({
          name, filePath, origin, content, fileContent, dataLossPrev, ifMatch,
          blockeditorConversion, blockeditorWrite, pageFormat, persistentToken, target, token,
        }));
      }

      // Check if this is the first save (no versions exist yet)
      const siteVersionsDir = path.join(baseDir, VERSIONS_DIR, backupName);
      let isFirstSave = false;
      try {
        const versionFiles = await fs.readdir(siteVersionsDir);
        isFirstSave = versionFiles.length === 0;
      } catch (error) {
        // Directory doesn't exist yet, so this is the first save
        isFirstSave = true;
      }

      // The bytes already on disk, read once: the first-save backup stores their
      // file part, and a save that changed only the merge state leaves them alone.
      let existingContent = null;
      try {
        existingContent = await fs.readFile(filePath, 'utf8');
      } catch (error) {
        // File doesn't exist yet, that's OK
      }

      // If first save, backup the existing site content first
      if (isFirstSave && existingContent !== null) {
        try {
          await createBackup(baseDir, backupName, roomsPackage().splitPage(existingContent).file);
          console.log(`Created initial backup of existing ${name}`);
        } catch (error) {
          // File doesn't exist yet, that's OK
        }
      }

      // Create backup of the new content. A save that changed only the merge
      // state leaves the file unchanged, so it writes no new version.
      if (existingContent !== fileContent) await createBackup(baseDir, backupName, fileContent);

      // Write via temp + rename: a crash or a full disk can never leave the
      // served file holding partial bytes.
      const beforeRename = readIdentity ? async () => {
        const current = await fs.stat(filePath, { bigint: true }).catch(error => {
          if (error.code === 'ENOENT') return null;
          throw error;
        });
        if (!current || ['dev', 'ino', 'size', 'mtimeNs'].some(name => current[name] !== readIdentity[name])) {
          const error = new Error('Checkpoint target changed since it was read');
          error.code = 'moved-or-deleted';
          throw error;
        }
      } : null;
      // The sidecar lands before the file, and only the file part reaches disk.
      // A checkpoint is the write ClayJS is told contains a person's edits, so it
      // is the one write that must fail rather than report a rename the disk may
      // not keep.
      const durable = origin === 'checkpoint' ? { durable: true } : undefined;
      const stored = await placeBytes({ name, filePath, logical: content, durable, beforeRename });

      // Recorded from the bytes actually written, so a later conflict can tell a
      // second tab of this person's from a text editor. Set after the write, since
      // before it this would claim authorship of a save that then failed.
      // Keyed on the canonical filePath, the same key the write queue uses, so two
      // spellings of one file (an in-tree symlink, or a case-insensitive volume) can
      // never keep two separate records of who last wrote it.
      //
      // The mtime rides along with the stamp because the stamp alone cannot answer
      // the question. A file can go B -> C -> B, and once it is back at B the digest
      // matches this host's last write again, so an external editor's undo reads
      // exactly like another of this person's tabs. The mtime moved for both of those
      // writes and does not come back.
      const wroteAt = await fs.stat(filePath, { bigint: true }).catch(() => null);
      store.etags.set(filePath, {
        etag: documentEtag(content),
        mtimeNs: wroteAt ? wroteAt.mtimeNs : null,
        // Who wrote these bytes: 'save' for a tab, 'api' for a write with no tab
        // behind it. /save reads it to decide whether a conflicting stamp is
        // honestly another tab's, because a data API write is nobody's tab.
        writer: origin,
        // §6's receipt, bound here and nowhere else: this is the one moment this
        // host knows both which request's body it stored and what those stored
        // bytes stamp to. An id-less save records '' and so replaces any id
        // remembered from an earlier one, which is what keeps a remembered id from
        // outliving its own bytes when a later save happens to restore them.
        saveId
      });

      await settleSave({ name, origin, content, fileContent, pageToken, target, token, persistentToken });

      // Refresh the per-site API data sidecar BEFORE the fallible Tailwind compile,
      // so a Tailwind failure can't skip it and leave stale API data on disk
      // (mirrors the platform ordering in node-content.js). Non-fatal: a sidecar
      // error must never fail the save.
      try {
        await writeApiSidecar(baseDir, name, content);
      } catch (e) {
        console.error('writeApiSidecar failed (non-fatal):', e && e.message ? e.message : e);
      }

      // Data-clobber guard (non-blocking, non-fatal). A browser /save is always
      // a UI save, split by the userDriven bit into ui-gestured / ui-background.
      // A data API write has no tab behind it at all, so it is judged as an
      // external edit, exactly like a write the watcher saw on disk.
      {
        const dataLossProv = origin === 'api' ? 'external' : dataGuard.provenanceForLocalSave(userDriven);
        dataGuard.runDataLossGuard({
          baseDir, name, newHtml: content, prevContent: dataLossPrev, prov: dataLossProv, live,
          ifMatched: origin === 'api' && typeof ifMatch === 'string' && dataGuard.namesExactVersion(ifMatch),
        }).catch(err => console.error('[data-guard] /save guard error:', err && err.message ? err.message : err));
      }

      // Generate Tailwind CSS if site uses it. `tailwindName` from
      // getTailwindCssName includes any path prefix present in the URL
      // (e.g. "blog/post"), so path.join naturally nests the CSS file. After
      // the replaceTailwindLink above, the URL is always scoped to the site's
      // folder, which mirrors the platform's
      // public-assets/tailwindcss/{username}/{path}/{baseName}.css layout.
      // Non-fatal, for the same reason as the sidecar above and the two remote
      // writers: the file is already written, backed up and broadcast by this
      // point, so a compiler error must not report the save as failed, and must
      // not skip the snapshot cache below.
      await refreshTailwind(content);

      recordSaveProvenance(name, userDriven);
      return { outcome: 'committed', reason: null, etag: documentEtag(content), stored, content, file: fileContent };
    } finally {
      if (pageToken) authority.rooms.releasePage(pageToken);
    }
  }

  // The create lane's confirmation: a new document's bytes stamped into the room
  // registry, which clears a retirement an explicit create admitted.
  async function settleCreate({ bytes, target, token }) {
    await ctx.localAuthority.confirmKnownWrite(target, bytes, token);
  }

  async function commitCreate({ name, filePath: requestedPath, bytes, context = {} }) {
    const { target = null, token = null } = context;
    const filePath = target ? target.canonicalPath : requestedPath;
    if (target) assertWriteContext(token, filePath);
    const stored = await storePageState(baseDir, name, bytes);
    dataGuard.markPending(baseDir, name, stored);
    await atomicWriteFile(filePath, stored, null);
    if (target) await settleCreate({ bytes, target, token });
    return { outcome: 'committed', reason: null, etag: documentEtag(bytes), checksum: null, stored, content: bytes };
  }

  // The external rebase lane's admission: the open room's unsaved edits kept as a
  // recovered copy before an outside edit is rebased over them.
  async function admitRebase({ target, token }) {
    await ctx.localAuthority.recoverUnsavedEdits(target, token);
  }

  // Its confirmation: the authority learns the bytes the watcher will see, without
  // publishing them.
  async function settleRebase({ bytes, target, token }) {
    await ctx.localAuthority.confirmKnownWrite(target, Buffer.from(bytes, 'utf8'), token, { sender: 'file-watcher', publish: false });
  }

  async function commitRebase({ name, filePath: requestedPath, bytes, context = {} }) {
    const { target, token } = context;
    assertWriteContext(token, target.canonicalPath);
    await admitRebase({ target, token });
    await createBackup(baseDir, name.replace(DOCUMENT_NAME, ''), roomsPackage().splitPage(bytes).file);
    const stored = await storePageState(baseDir, name, bytes);
    dataGuard.markPending(baseDir, name, stored);
    await atomicWriteFile(target.canonicalPath, stored);
    await settleRebase({ bytes, target, token });
    return { outcome: 'committed', reason: null, etag: documentEtag(bytes), checksum: null, stored, content: bytes };
  }

  // A duplicate is a new document, so its documents are filtered before they reach the
  // destination: nothing raw is ever written first for the root watcher to see. Each entry is
  // created exclusively, so an occupied destination is never overwritten, and the source is only
  // ever read. Versions and automatic recovery state keep their bytes.
  async function copyStructure(from, to) {
    const stat = await fs.lstat(from);
    if (stat.isDirectory()) {
      if (path.basename(from) === METADATA_DIR) {
        await fs.cp(from, to, { recursive: true, errorOnExist: true, force: false });
        return;
      }
      const mode = stat.mode & 0o777;
      await fs.mkdir(to, { mode });
      await fs.chmod(to, mode);
      for (const name of await fs.readdir(from)) await copyStructure(path.join(from, name), path.join(to, name));
      return;
    }
    if (stat.isSymbolicLink()) {
      await fs.cp(from, to, { errorOnExist: true, force: false });
      return;
    }
    if (DOCUMENT_NAME.test(path.basename(from))) {
      const mode = stat.mode & 0o777;
      const bytes = await fs.readFile(from);
      await fs.writeFile(to, stripCopyBytes(bytes), { flag: 'wx', mode });
      await fs.chmod(to, mode);
      return;
    }
    await fs.copyFile(from, to, fs.constants.COPYFILE_EXCL);
  }

  // The structure lane's room half before any entry moves: the authority's plan
  // for these steps, which holds every document they touch. Null without an
  // authority, and for the copy kinds, which take no plan.
  function prepareRestructure({ kind, rootId = null, steps, entries, token }) {
    const policy = STRUCTURE_KINDS[kind];
    const authority = ctx.localAuthority;
    if (!authority || policy.copy) return null;
    return authority.prepareStructure(token, {
      rootId,
      steps: steps.map((step, index) => ({
        from: entries[index].from, fromRel: step.from,
        to: policy.retire ? null : entries[index].to, toRel: policy.retire ? null : step.to,
      })),
    });
  }

  // Its confirmation once the entries moved: the completed steps commit into the
  // authority, and the plan is released on every outcome.
  function settleRestructure({ plan, completed, copiedSteps = [], token }) {
    if (!plan) return;
    const authority = ctx.localAuthority;
    try {
      if (completed) authority.commitStructure(plan, completed, token, { copiedSteps });
    } finally {
      authority.releaseStructure(plan, token);
    }
  }

  // A copy's confirmation, after each entry lands: the copied destination is
  // admitted as a live document path.
  function admitCopy({ rootId = null, step, entry, token }) {
    const authority = ctx.localAuthority;
    if (!authority) return;
    authority.setAdmission(token, { rootId, logicalPath: step.to,
      canonicalPath: entry.to.canonical, active: true, prefix: entry.from.isDir });
  }

  async function restructure({ kind, rootId = null, steps, context = {} }) {
    if (!Object.hasOwn(STRUCTURE_KINDS, kind)) throw new TypeError(`Unknown structure kind: ${kind}`);
    const policy = STRUCTURE_KINDS[kind];
    const { token = null, entries } = context;
    const authority = ctx.localAuthority;
    if (authority) assertStructureContext(token);
    if (policy.copy) {
      let completed = 0;
      let failure = null;
      try {
        for (const [index, entry] of entries.entries()) {
          await fs.mkdir(path.dirname(entry.to.canonical), { recursive: true });
          await copyStructure(entry.from.location, entry.to.canonical);
          admitCopy({ rootId, step: steps[index], entry, token });
          completed = index + 1;
        }
      } catch (error) {
        failure = error;
      }
      const outcome = !failure ? 'committed' : completed ? 'partial' : 'failed';
      return withCause({ outcome, completed, reason: failure ? (failure.code ?? 'failed') : null }, failure);
    }
    const plan = prepareRestructure({ kind, rootId, steps, entries, token });
    let completed = 0;
    let failure = null;
    const copiedSteps = [];
    try {
      for (const [index, entry] of entries.entries()) {
        if (await moveEntry(entry.from.location, entry.to.canonical, policy)) copiedSteps.push(index);
        completed = index + 1;
      }
    } catch (error) {
      failure = error;
    } finally {
      settleRestructure({ plan, completed, copiedSteps, token });
    }
    const outcome = !failure ? 'committed' : completed ? 'partial' : 'failed';
    return withCause({ outcome, completed, reason: failure ? (failure.code ?? 'failed') : null }, failure);
  }

  const LANES = {
    create: commitCreate,
    'external-rebase': commitRebase,
    exact: commitExact,
    replace: commitReplace,
    save: commitSave,
  };

  async function commit(request) {
    if (!request || !Object.hasOwn(KINDS, request.kind)) {
      throw new TypeError(`Unknown document write kind: ${request && request.kind}`);
    }
    if (request.hooks) throw new TypeError('hooks ended in M16');
    const policy = KINDS[request.kind];
    const lane = LANES[policy.lane];
    if (!lane) throw new Error(`Document write kind ${request.kind} is not migrated yet`);
    if (await preconditionFailed(request)) return refused('changed');
    try {
      const result = await lane(request, policy);
      // The bytes the guard must treat as the last ones seen. Not awaited: a save
      // gains no step, and pendingSeen is set synchronously so a page load that
      // lands before the persist still reads them as seen.
      if (result.outcome === 'committed') dataGuard.noteSeen(baseDir, request.name, result.stored);
      return result;
    } catch (error) {
      if (error && error.indeterminate) {
        return withCause({ outcome: 'indeterminate', reason: error.code ?? 'after-rename', etag: null, stored: null, content: null }, error);
      }
      throw error;
    }
  }

  // The checkpoint's room half, run inside the document boundary the caller
  // holds: the room and its generation still match, the document is read once
  // with its identity, the room is reconciled with that read, and a dirty room is
  // rendered over it. Writes nothing. Answers `render` with what the save lane
  // needs, or Local's `clean`, `current` or `refused`.
  async function renderCheckpoint({
    core, key, roomIdentity, generation, target, room, claim, token,
    readCurrent = async () => {
      const handle = await fs.open(key, 'r');
      try {
        const readIdentity = await handle.stat({ bigint: true });
        return { html: await handle.readFile('utf8'), readIdentity };
      } finally { await handle.close(); }
    },
  }) {
    const authority = ctx.localAuthority;
    authority.validateAdmission(claim, target, token);
    if (authority.rooms.get(normCanonical(key)) !== room || String(room.nodeId) !== String(roomIdentity)) {
      return { kind: 'refused', reason: 'invalid-identity' };
    }
    let currentHtml;
    let readIdentity;
    try {
      ({ html: currentHtml, readIdentity } = await readCurrent());
    }
    catch (error) {
      if (error.code === 'ENOENT') return { kind: 'refused', reason: 'invalid-identity' };
      throw error;
    }
    if (authority.logicalHtml) currentHtml = await authority.logicalHtml(target, currentHtml);
    const observed = await authority.reconcileDisk(target, currentHtml, token);
    if (observed.kind === 'replacement' || authority.rooms.get(normCanonical(key)) !== room ||
        (room.blockeditorTransition?.id ?? null) !== generation) {
      return { kind: 'refused', reason: 'invalid-identity' };
    }
    let capture;
    try { capture = core.captureCheckpoint(room, { nodeId: target.identity, roomKey: key }); }
    catch (error) { return { kind: 'refused', reason: 'invalid-document', error }; }
    if (!authority.roomIsDirty(room)) return { kind: 'clean' };
    if (core.isCheckpointCurrent(currentHtml, capture)) {
      authority.rooms.markSaved(room, capture);
      return { kind: 'current' };
    }
    const { JSDOM } = require('jsdom');
    const rendered = core.renderCheckpoint(currentHtml, capture, {
      createDocument: createDocument ?? (() => new JSDOM('<!doctype html><html><body></body></html>', {
        runScripts: undefined, resources: undefined,
      }).window.document),
    });
    if (rendered.kind !== 'ok') return { kind: 'refused', reason: rendered.reason ?? 'invalid-document' };
    return { kind: 'render', html: rendered.html, revision: rendered.revision, capture, currentHtml, readIdentity };
  }

  // A dirty room's unsaved edits, rendered and stored through the save lane. The
  // caller hands in the room it means and the page generation it was made at, and
  // the save lane runs inside the boundary this call already holds.
  async function checkpoint({ name, filePath: key }, { roomIdentity, generation }, { target, room }) {
    let core;
    try { core = await loadCheckpoint(); }
    catch (error) { return { kind: 'refused', reason: 'checkpoint-unavailable', message: error.message }; }
    const authority = ctx.localAuthority;
    const claim = authority.captureAdmission(target);
    return withDocument(() => ({ canonicalPath: key }), async (_locked, token) => {
      const rendered = await renderCheckpoint({ core, key, roomIdentity, generation, target, room, claim, token });
      if (rendered.kind !== 'render') return rendered;
      const written = await commitSave({
        name, filePath: key, bytes: rendered.html, dataLossPrev: rendered.currentHtml,
        userDriven: false, saveId: '', readIdentity: rendered.readIdentity, context: { target, token },
      }, { origin: 'checkpoint' });
      if (written.outcome === 'indeterminate') throw written.error;
      if (written.outcome === 'committed') dataGuard.noteSeen(baseDir, name, written.stored);
      return { kind: 'ok', revision: rendered.revision, written: written.content };
    });
  }

  async function blockeditorState({ op, name, to = null, record = null }) {
    const store = require('./page-state-store.js');
    if (op === 'write') return writePageState(baseDir, name, record);
    if (op === 'move') return store.movePageState(baseDir, name, to);
    if (op === 'moveDir') return store.movePageStateDir(baseDir, name, to);
    if (op === 'drop') return store.dropPageState(baseDir, name);
    if (op === 'dropDir') return store.dropPageStateDir(baseDir, name);
    throw new TypeError(`Unknown blockeditor-state operation: ${op}`);
  }

  return {
    commit, read, restructure, checkpoint, events, blockeditorState,
    lanes: {
      prepareSave, admitSave, settleSave, recordSaveProvenance,
      prepareReplace, admitReplace, settleReplace,
      admitRebase, settleRebase, settleCreate, settleExact: confirmExact,
      renderCheckpoint,
      prepareRestructure, settleRestructure, admitCopy,
    },
  };
}

function createRootDocumentHost({ ctx, live = require('./utils/root-live.js').createRootLive(ctx.root), emit = () => {}, logger = null, loadCheckpoint, getObserver = () => null }) {
  if (process.env.HYPERCLAY_LOCAL_DOCUMENT_HOST === 'go') {
    return require('../../trial/go-document-host.cjs').createGoDocumentHost({ ctx, live, emit, logger });
  }
  const { getConsentRegistry, resolveWritePath, validateSegments } = require('./utils/path-resolver.js');
  const { snapshotStoreFor } = require('./utils/snapshot-store.js');
  const { TAILWIND_DIR, VERSIONS_DIR } = require('./utils/artifact-paths.js');
  const { storePageState, writePageState, logicalDocument } = require('./page-state-store.js');
  const { createBackup, createBackupIfExists, createBinaryBackupIfExists } = require('./utils/backup.js');
  const { atomicWriteFile, assertWriteContext, assertStructureContext } = require('./utils/write-queue.js');
  const { writeApiSidecar } = require('./utils/api-sidecar.js');
  const { refreshDerivedArtifacts } = require('./utils/derived-artifacts.js');
  const { compileTailwind, getTailwindCssName } = require('tailwind-hyperclay');
  const { scopeTailwindLink } = require('./utils/tailwind-scoping.js');
  const { stripSaveToken } = require('./utils/root-attrs.js');
  const { stripCopyBytes } = require('./copy-content.js');
  const dataGuard = require('./data-loss-guard.js');
  const { documentEtag } = require('./spec-wire.js');
  const { rebaseStaleSave, newestVersionFiles, needsAdoption } = require('./page-rebase.js');
  const formatHtml = require('./format-html.js');

  const baseDir = ctx.root.path;
  const paths = getConsentRegistry(baseDir);
  return createDocumentHost({
    ctx, baseDir, live, emit, logger,
    store: snapshotStoreFor(ctx.root.id),
    resolveTailwindWrite: async (name) => {
      validateSegments(`${name}.css`);
      return resolveWritePath(paths, `${TAILWIND_DIR}/${name}.css`);
    },
    canonicalForWrite, formatHtml, scopeTailwindLink, stripSaveToken, splitWrite,
    storePageState, writePageState, logicalDocument,
    createBackup, createBackupIfExists, createBinaryBackupIfExists,
    atomicWriteFile, assertWriteContext, assertStructureContext, stripCopyBytes,
    writeApiSidecar, refreshDerivedArtifacts, getTailwindCssName, compileTailwind,
    dataGuard, documentEtag, rebaseStaleSave, newestVersionFiles, needsAdoption, roomsPackage,
    MAX_EXTERNAL_HTML, VERSIONS_DIR, TAILWIND_DIR, loadCheckpoint, getObserver,
  });
}

module.exports = { createDocumentHost, createRootDocumentHost, canonicalForWrite, splitWrite, refuseOldFormat, MAX_EXTERNAL_HTML, LOOPBACK_RUNTIME };
