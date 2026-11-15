/**
 * Session-level reconcile entry points (C3 §5.6, §9).
 *
 * The session's state machine (reconcile/session-runner.js) drives the engine
 * through these: one full reconcile per generation, one node re-read per live
 * stream invalidation, and the pending work dropped when a generation ends.
 * Everything here is fenced by the generation the runner passed in and by the
 * signal it aborted on pause, restart or stop, so a stale continuation writes
 * nothing. Methods here are installed onto SyncEngine.prototype.
 */

const fsp = require('fs').promises;
const path = require('upath');
const { fileExists } = require('./file-operations');
const { listNodes } = require('./api-client');
const nodeMap = require('./node-map');
const conflictStore = require('./reconcile/conflicts');
const { classifyRoot, putRootMarker, recordIdentity, rootDirIdentity } = require('./root-marker');
const { decidePath, A } = require('./reconcile/decide');

module.exports = {
  /**
   * Why the session's folder cannot be trusted right now, or null. The folder has to exist
   * and, once the session is bound to a marked folder, carry its marker (root-marker.js): an
   * unmounted volume, a recreated folder or an evicted one reads as every file deleted
   * otherwise. A missing root is never recreated (CONTRACTS §6). A folder an older install
   * never marked is marked here, the first time it is seen with its files.
   */
  rootRefusal() {
    if (!fileExists(this.syncFolder)) return 'folder-missing';
    const { refusal, adopt } = classifyRoot(this.syncFolder, {
      rootId: this.rootId ?? null,
      required: this.rootMarkerRequired === true,
      baselineSize: this.repo ? this.repo.size : 0,
    });
    return refusal || null;
  },

  /**
   * The marker says this folder belongs to the session; only the directory's own identity
   * says it is the folder the baseline was recorded in. A copy (a backup put back, Find it
   * pointed at an older copy, a move to another disk) carries the marker but not the
   * directory: its pages are neither newer nor older than the server's until a recheck
   * compares them. Nothing recorded yet proves nothing either way, so it is not unproven.
   */
  rootUnproven() {
    if (this.rootDir == null) return false;
    return rootDirIdentity(this.syncFolder) !== this.rootDir;
  },

  /**
   * Record the folder as the session's: the marker when it has none, then `identity.json`
   * with the marker flag and the directory's identity. Runs when a session starts on a
   * folder the marker proves, and after a recheck proved it. An unproven folder is left to
   * the recheck.
   */
  async adoptRoot() {
    if (!fileExists(this.syncFolder) || this.rootUnproven()) return;
    const { refusal, adopt } = classifyRoot(this.syncFolder, {
      rootId: this.rootId ?? null,
      required: this.rootMarkerRequired === true,
      baselineSize: this.repo ? this.repo.size : 0,
    });
    if (refusal || this.rootId == null) return;
    const dir = rootDirIdentity(this.syncFolder);
    if (!adopt && this.rootMarkerRequired && this.rootDir === dir) return;
    if (adopt) {
      try {
        await putRootMarker(require('./local-boundary').documentHostFor(this), this.rootId);
      } catch {
        // A root the app cannot write into (read-only mount, permissions) syncs as it did
        // before the marker: the marker is not required until it has been written.
        return;
      }
    }
    recordIdentity(this.metaDir, { rootMarker: true, rootDir: dir });
    this.rootMarkerRequired = true;
    this.rootDir = dir;
  },

  /**
   * Every request that changes the server passes here (api-client calls `conn.guard`). A
   * missing or replaced folder pauses the session, as before. A folder the marker names but
   * the directory does not is refused too: the write is skipped and a live session starts
   * again, so its next pass is a recheck.
   */
  assertRootWritable() {
    this.assertRootPresent();
    if (!this.rootUnproven()) return;
    if (!this.recheckPass && this.runner && this.runner.state === 'live') this.requestReconcile();
    throw Object.assign(new Error('The sync folder is not the one last synced'), { code: 'folder-unproven' });
  },

  /**
   * One pass over a folder the session cannot prove is its own: the baseline may change this
   * computer (download, adopt, trash a file deleted online) but never the server, so a page
   * that differs from the baseline is held as a conflict, a page missing here is downloaded,
   * and a file only here waits. A pass that completes with every item settled proves the
   * folder: its identity is recorded, and the caller runs the normal pass next.
   *
   * @returns {Promise<boolean>} true when the folder is now proven
   */
  async recheckRoot(inventory, { generation, signal } = {}) {
    const checked = rootDirIdentity(this.syncFolder);
    // Deletes the live structure job is holding were measured against the folder that left:
    // none of them may reach the server once this folder is proven.
    this.clearStructureState({ forgetDeletes: true });
    this.recheckPass = true;
    this.recheckFailed = false;
    try {
      await this.reconcileAll(inventory, { generation, signal });
    } finally {
      this.recheckPass = false;
    }
    if (this.recheckFailed || (signal && signal.aborted) || rootDirIdentity(this.syncFolder) !== checked) return false;
    this.rootDir = null;
    await this.adoptRoot();
    return true;
  },

  rootPresent() {
    return this.rootRefusal() === null;
  },

  /**
   * Refuse a remote delete while the root cannot be trusted: an unmounted, deleted or
   * replaced folder looks like every file was deleted. The session pauses instead, and
   * the delete is not sent.
   */
  assertRootPresent() {
    const reason = this.rootRefusal();
    if (!reason) return;
    if (this.runner) this.runner.pause(reason);
    const message = reason === 'folder-replaced' ? 'The sync folder was replaced' : 'The sync folder is missing';
    throw Object.assign(new Error(message), { code: reason });
  },

  /**
   * Reconcile the whole disk against one inventory the session already listed
   * (`complete: true`, or a legacy list that proves nothing). The inventory is
   * handed to the three passes as their cache, so they never list again.
   *
   * @param {Array} inventory the session's list, `complete` on the array or on
   *   the envelope the api returned
   * @param {{generation?:number, signal?:AbortSignal, bootstrap?:boolean}} [work]
   */
  reconcileAll(inventory, { generation, signal, bootstrap = false } = {}) {
    if (signal && signal.aborted) return Promise.resolve();
    const startedAtCall = this._laneStarted;
    const busyAtCall = this.inLane();
    return this.serial(() => {
      // Lane work that ran while this waited may have changed the server since the inventory was listed.
      const stale = busyAtCall || this._laneStarted - 1 > startedAtCall;
      return this.reconcileAllInLane(inventory, { generation, signal, bootstrap, stale });
    });
  },

  async reconcileAllInLane(inventory, { generation, signal, bootstrap = false, stale = false } = {}) {
    if (signal && signal.aborted) return;
    if (stale) {
      const gen = this.generation;
      const fresh = await listNodes(this.conn);
      if (gen !== this.generation) return;
      inventory = fresh;
    }
    const nodes = Array.isArray(inventory) ? inventory : ((inventory && inventory.nodes) || []);
    const work = this.beginSessionWork(generation, signal);
    this.serverNodesCache = nodes;
    this.serverNodesCacheTime = Date.now();
    this.serverNodesComplete = (inventory && inventory.complete === true) || nodes.complete === true;
    this.serverFilesCache = null;
    this.bootstrapPass = bootstrap === true;

    try {
      await this.clearFileLimitBlocks();
      await this.setCaseTwins(nodes);
      await this.performInitialFolderSync(nodes);
      if (this.staleWork(generation, signal, work)) return;
      await this.performInitialSync();
      if (this.staleWork(generation, signal, work)) return;
      await this.performInitialUploadSync();
    } finally {
      this.bootstrapPass = false;
      this.movedRemotely = new Set();
      this.pathUnresolved = new Set();
      this.restoredFolders = [];
    }
  },

  /**
   * Re-read one node — content, path and structure — from a fresh list and run
   * `decide` for it. This is what a live stream frame triggers: the frame is
   * only an invalidation, never content to apply.
   *
   * @param {number|string} nodeId
   * @param {{generation?:number, signal?:AbortSignal}} [work]
   * @returns {Promise<string|null>} the action, or null when nothing was decided
   */
  refreshNode(nodeId, { generation, signal } = {}) {
    if (signal && signal.aborted) return Promise.resolve(null);
    const id = String(nodeId);
    const seqAtCall = this._mutationSeq || 0;
    const work = this.beginSessionWork(generation, signal);

    // One list in flight per session: every invalidation of this generation
    // waits for the same fetch instead of listing once per node. The read is
    // shared here; the decision it feeds runs inside the lane.
    if (!this.nodeListInFlight) {
      this.nodeListInFlight = this.refreshInventory({ inventory: null, refreshed: new Set() })
        .finally(() => { this.nodeListInFlight = null; });
      this.nodeListInFlight.catch(() => {});
    }
    const inventory = this.nodeListInFlight;
    return this.serial(async () => {
      let listed = await inventory;
      if ((this._mutationSeq || 0) !== seqAtCall) {
        const gen = this.generation;
        const fresh = await listNodes(this.conn);
        if (gen !== this.generation) return null;
        listed = fresh;
      }
      if (this.staleWork(generation, signal, work)) return null;
      return this.refreshNodeInLane(id, listed, { generation, signal, work });
    });
  },

  async refreshNodeInLane(id, inventory, { generation, signal, work }) {
    await this.setCaseTwins(inventory);
    const node = (inventory || []).find((n) => String(n.id) === id);
    if (!node) await this.markDeletedConflicts(inventory || []);
    let entry = this.repo.get(id) || null;
    if (entry && entry.path && this.passLeavesToLiveJob(entry.path)) {
      this._structureState().refreshDeferred = true;
      this.markDirty(entry.path);
      return null;
    }
    const remote = node ? this.remoteViewOf(node) : null;
    const isFolder = (remote && remote.type === 'folder') || (entry && entry.type === 'folder');

    if (entry && remote && entry.path && remote.path !== entry.path) {
      await this.applyRemotePath(id, entry, remote, isFolder);
      if (this.staleWork(generation, signal, work)) return null;
      entry = this.repo.get(id) || null;
    }

    // A folder has no content to decide: a new one is created, a deleted one is trashed only
    // on a complete list, and a moved one was relocated above.
    if (isFolder) {
      if (remote && !entry) {
        await this._applyNodeSavedFolder({ nodeId: node.id, nodeType: 'folder', name: node.name, path: remote.path, parentId: node.parentId });
        return 'create-folder';
      }
      if (!remote && entry && this.serverNodesComplete) {
        const plan = await this.folderDeletePlan(id, entry.path);
        if (this.staleWork(generation, signal, work)) return null;
        if (plan) {
          await this.runPlan(plan);
          return 'keep-folder';
        }
        await this._applyFolderDelete(id, entry.path);
        return 'trash-folder';
      }
      return null;
    }

    const rel = remote ? remote.path : entry && entry.path;
    if (!rel) return null;

    const item = await this.decideNode({
      nodeId: id,
      rel,
      entry,
      remote,
      localPresent: fileExists(path.join(this.syncFolder, rel)),
    });
    if (this.staleWork(generation, signal, work)) return null;

    await this.runPlanItem(item, { inventory, refreshed: new Set() });
    return item.decision.action;
  },

  /**
   * A folder deleted online whose subtree holds work this disk has not sent is
   * not trashed whole: each file is decided as a deleted file is, so a changed
   * one stays as a remote-deleted conflict, and the folders are forgotten.
   * Null when nothing under it is unsent.
   */
  async folderDeletePlan(folderId, rel) {
    const descendants = this.repo.walkDescendants(rel);
    const files = [];
    for (const { nodeId, entry } of descendants) {
      if (entry.type === 'folder') continue;
      files.push(await this.decideNode({
        nodeId,
        rel: entry.path,
        entry,
        remote: null,
        localPresent: fileExists(path.join(this.syncFolder, entry.path)),
        complete: true,
      }));
    }
    const unsent = files.some((item) => item.decision.action !== A.TRASH_LOCAL && item.decision.action !== A.FORGET)
      || (await this.holdsUntrackedFile(rel));
    if (!unsent) return null;
    const folders = [
      { nodeId: folderId, entry: this.repo.get(folderId) },
      ...descendants.filter(({ entry }) => entry.type === 'folder'),
    ];
    return [...files, ...folders.map(({ nodeId, entry }) => this.folderItem(nodeId, entry.path, { action: A.FORGET }, { parentId: entry.parentId }))];
  },

  async holdsUntrackedFile(rel) {
    let found;
    try {
      found = await fsp.readdir(path.join(this.syncFolder, rel), { recursive: true, withFileTypes: true });
    } catch {
      return false;
    }
    return found.some((dirent) => dirent.isFile() && !dirent.name.startsWith('.') &&
      !this.repo.getByPath(path.relative(this.syncFolder, path.join(dirent.parentPath, dirent.name))));
  },

  /**
   * A conflict held against a node that a complete list no longer has: the node, or its folder,
   * was deleted online after the conflict was found. It is held as remote-deleted from now on, so
   * Keep mine creates the page again and Keep theirs moves this copy to the trash.
   */
  async markDeletedConflicts(nodes) {
    if (this.serverNodesComplete !== true || !this.metaDir) return;
    const listed = new Set(nodes.map((node) => String(node.id)));
    const records = await conflictStore.load(this.metaDir);
    for (const [key, record] of Object.entries(records)) {
      if (!/^\d+$/.test(key) || listed.has(key) || record.kind === conflictStore.KINDS.REMOTE_DELETED) continue;
      await conflictStore.update(this.metaDir, key, { kind: conflictStore.KINDS.REMOTE_DELETED });
      this.emit('file-synced', { file: record.path, action: 'conflict', kind: conflictStore.KINDS.REMOTE_DELETED });
    }
  },

  /**
   * Whether the file at `rel` is the one the entry's baseline was recorded for: the same inode
   * and birth time. A weak identity (no birth time) proves nothing.
   */
  isTrackedFile(entry, rel) {
    const identity = nodeMap.getInodeSync(path.join(this.syncFolder, rel));
    return identity !== null && !String(identity).endsWith(':0') && String(entry.inode) === String(identity);
  },

  /**
   * A node whose listed path differs from its baseline path moved on one side. The local file is
   * either still at the baseline path (a teammate moved it: apply the move here) or already at the
   * listed path (adopt it). A local move is the watcher's to send, so it is never undone here.
   */
  async applyRemotePath(id, entry, remote, isFolder) {
    const atBase = fileExists(path.join(this.syncFolder, entry.path));
    const atRemote = fileExists(path.join(this.syncFolder, remote.path));
    const localPath = atBase || !atRemote ? entry.path : remote.path;
    if (!isFolder && !atBase && atRemote && !this.isTrackedFile(entry, remote.path)) {
      // Another file at the node's new path, the node's own gone: compared, never uploaded.
      await this.repo.delete(id);
      return 'unbound';
    }
    const { action } = decidePath({ basePath: entry.path, localPath, remotePath: remote.path });
    if (action === 'adopt-path') {
      await this.repo.set(id, { ...entry, path: remote.path, parentId: remote.parentId ?? entry.parentId });
      return action;
    }
    if (isFolder) await this._applyFolderRelocate(id, entry.path, remote.path);
    else await this._applyFileRelocate(id, entry.path, remote.path, remote.type || entry.type);
    return action;
  },

  /**
   * Drop everything this session queued: the debounce queue and its retry
   * timers, and the invalidation work in flight. The generation bump makes
   * every continuation started before this point stale, which is how a pause
   * touches no files.
   */
  dropPendingWork() {
    this.generation += 1;
    this.sessionInvalidated = true;
    this.sessionWork = null;
    this.nodeListInFlight = null;
    this.syncQueue.clear();
    this.clearStructureState();
    this.settleQueueWaiters();
  },

  /**
   * Work this session still owes the server, anywhere it can sit: queued or
   * executing items, retry timers, lane work (initial passes, structure jobs,
   * stream handlers) queued or running, structure marks still debouncing,
   * deletions held before they are sent, and a mass delete awaiting an answer.
   * Stopping the engine drops all of it, which is why the update drain asks.
   */
  hasUnsettledWork() {
    const queue = this.syncQueue;
    if (queue.length() > 0 || queue.isProcessingQueue() || queue.retryTimers.size > 0) return true;
    if (this._lanePending > 0) return true;
    const structure = this._structure;
    if (!structure) return false;
    return structure.dirty.size > 0 || !!structure.timer || !!structure.wakeTimer || !!structure.massDelete;
  },

  /**
   * Record the generation and signal the session is working under, and return
   * the engine generation this work belongs to: `dropPendingWork` bumps it, so
   * a continuation can tell that its generation ended even without a signal.
   */
  beginSessionWork(generation, signal) {
    if (generation !== undefined && generation !== null) this.sessionGeneration = generation;
    this.sessionWork = { generation, signal };
    this.sessionInvalidated = false;
    return this.generation;
  },

  /**
   * Is the work a caller asked for still current? A pause, restart or stop
   * aborts the signal the runner handed over; `dropPendingWork` bumps the
   * engine generation; a newer session generation makes an older one's
   * reconcile stale. Callers check this after every await, once
   * `beginSessionWork` has recorded their generation.
   */
  staleWork(generation, signal, engineGeneration) {
    if (signal && signal.aborted) return true;
    if (engineGeneration !== undefined && engineGeneration !== this.generation) return true;
    if (generation === undefined || generation === null) return false;
    return generation !== this.sessionGeneration;
  },

  /** The same question for a pass that is already running (runPlan). */
  sessionStale() {
    if (this.sessionInvalidated) return true;
    const work = this.sessionWork;
    return !!(work && work.signal && work.signal.aborted);
  },
};
