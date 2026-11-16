const fs = require('fs/promises');
const fsSync = require('fs');
const path = require('path');

const { withFileLock, atomicWriteFile } = require('../main/utils/write-queue');

const MAP_FILE = 'node-map.json';
const STATE_FILE = 'sync-state.json';
// Tombstones record paths whose nodes have been renamed/moved away; the local
// /save guard consults them so stale-tab writes to the pre-move URL return 409.
const TOMBSTONES_FILE = 'tombstones.json';
// Bounds the tombstone set size: entries older than this are pruned on load.
// Anything shorter than a stale tab's realistic lifetime is safe here.
const TOMBSTONE_TTL_MS = 7 * 24 * 60 * 60 * 1000;

// First bind saves the map from four downloads at once. The lock publishes those saves in call order, so an
// older snapshot can never land over a newer one, and atomicWriteFile retries the rename Windows refuses
// while another handle has the file.
function atomicWrite(filePath, data) {
  return withFileLock(filePath, () => atomicWriteFile(filePath, data));
}

async function load(metaDir, logger = null) {
  const filePath = path.join(metaDir, MAP_FILE);
  let data;
  try {
    data = await fs.readFile(filePath, 'utf8');
  } catch (err) {
    if (err.code === 'ENOENT') return new Map();
    throw err;
  }

  let parsed;
  try {
    parsed = JSON.parse(data);
  } catch (err) {
    console.warn(`[SYNC] Corrupt ${MAP_FILE}; starting with empty map`);
    if (logger) {
      logger.error('SYNC', 'node-map.json is corrupt — all node mappings lost, full re-sync required', {
        error: err.message,
        filePath
      });
    }
    return new Map();
  }
  const map = new Map();
  for (const [key, value] of Object.entries(parsed)) {
    let entry;
    if (typeof value === 'string') {
      entry = { path: value, checksum: null, inode: null };
    } else {
      entry = { ...value };
    }

    if (!entry.type && entry.path) {
      entry.type = /\.(html|htmlclay)$/i.test(entry.path) ? 'site' : 'upload';
    }

    map.set(key, entry);
  }
  return map;
}

function readBaseline(entry) {
  if (!entry) return null;
  const checksum = entry.checksum === undefined ? null : entry.checksum;
  return {
    remoteEtag: entry.remoteEtag === undefined ? checksum : entry.remoteEtag,
    localChecksum: entry.localChecksum === undefined ? checksum : entry.localChecksum,
    structureVersion: entry.structureVersion === undefined ? null : entry.structureVersion,
    uploadBlocked: entry.uploadBlocked === true && entry.type !== 'upload'
  };
}

function applyBaseline(entry, fields = {}) {
  const current = readBaseline(entry) || {
    remoteEtag: null,
    localChecksum: null,
    structureVersion: null,
    uploadBlocked: false
  };
  const localChecksum = fields.localChecksum === undefined ? current.localChecksum : fields.localChecksum;
  return {
    ...entry,
    remoteEtag: fields.remoteEtag === undefined ? current.remoteEtag : fields.remoteEtag,
    localChecksum,
    structureVersion: fields.structureVersion === undefined ? current.structureVersion : fields.structureVersion,
    uploadBlocked: fields.uploadBlocked === undefined ? current.uploadBlocked : fields.uploadBlocked,
    checksum: localChecksum
  };
}

async function save(metaDir, map, logger = null) {
  const obj = Object.fromEntries(map);
  try {
    await atomicWrite(path.join(metaDir, MAP_FILE), JSON.stringify(obj, null, 2));
  } catch (err) {
    console.error(`[SYNC] Failed to save ${MAP_FILE}:`, err);
    if (logger) {
      logger.error('SYNC', 'Failed to persist node-map to disk — sync state may be lost on restart', {
        error: err.message,
        metaDir
      });
    }
    throw err;
  }
}

async function loadState(metaDir) {
  const filePath = path.join(metaDir, STATE_FILE);
  let data;
  try {
    data = await fs.readFile(filePath, 'utf8');
  } catch (err) {
    if (err.code === 'ENOENT') return {};
    throw err;
  }
  try {
    return JSON.parse(data);
  } catch (err) {
    console.warn(`[SYNC] Corrupt ${STATE_FILE}; using empty state`);
    return {};
  }
}

async function saveState(metaDir, state) {
  await atomicWrite(path.join(metaDir, STATE_FILE), JSON.stringify(state, null, 2));
}

function identityOf(stat) {
  return `${stat.ino}:${stat.birthtimeNs}`;
}

async function getInode(filePath) {
  try {
    return identityOf(await fs.stat(filePath, { bigint: true }));
  } catch {
    return null;
  }
}

function getInodeSync(filePath) {
  try {
    return identityOf(fsSync.statSync(filePath, { bigint: true }));
  } catch {
    return null;
  }
}

// A FAT disk keeps modification times to 2 s, so a save there can read up to 2 s early.
const MTIME_SLACK_MS = 2000;

/**
 * When the bytes at filePath were seen here: the earlier of `before` (this computer's clock, read
 * before the read or write) and the file's change time after it (the disk's clock). Every later
 * save stamps a time no earlier than this on either clock, so a disk whose clock runs behind
 * never makes an edit look older.
 */
async function seenSince(filePath, before) {
  try {
    const { ctimeMs } = await fs.stat(filePath);
    return seenAt(before, ctimeMs);
  } catch {
    return before;
  }
}

/** seenSince for a change time already read: the earlier of `before` and `ctimeMs`. */
function seenAt(before, ctimeMs) {
  return Number.isFinite(ctimeMs) ? Math.min(before, Math.floor(ctimeMs)) : before;
}

/**
 * A local change whose file was modified before the entry's bytes were last seen here is an older
 * file put back, not an edit: a save stamps the time it saves, and a copy that keeps times
 * (Finder, Explorer, cp -p, rsync -a, tar, Time Machine, Previous Versions) keeps the old one. A
 * seen time later than now means the clock was set back since, which proves nothing.
 */
function olderThanSeen(mtimeMs, entry, now = Date.now()) {
  const seen = entry ? entry.localSeenAt : null;
  return Number.isFinite(seen) && seen <= now && mtimeMs < seen - MTIME_SLACK_MS;
}

async function upgradeIdentities(map, syncFolder) {
  for (const entry of map.values()) {
    if (!entry.path) continue;
    if (entry.inode === null || entry.inode === undefined) {
      const identity = await getInode(path.join(syncFolder, entry.path));
      if (identity) entry.inode = identity;
      continue;
    }
    if (typeof entry.inode !== 'number') continue;
    const identity = await getInode(path.join(syncFolder, entry.path));
    if (identity && identity.split(':')[0] === String(entry.inode)) entry.inode = identity;
  }
}

function walkDescendants(map, folderPath) {
  if (!folderPath) return [];
  const prefix = folderPath.endsWith('/') ? folderPath : folderPath + '/';
  const results = [];
  for (const [nodeId, entry] of map) {
    if (entry.path && entry.path.startsWith(prefix)) {
      results.push({ nodeId, entry });
    }
  }
  return results;
}

async function loadTombstones(metaDir) {
  const filePath = path.join(metaDir, TOMBSTONES_FILE);
  let data;
  try {
    data = await fs.readFile(filePath, 'utf8');
  } catch (err) {
    if (err.code === 'ENOENT') return new Map();
    throw err;
  }

  let parsed;
  try {
    parsed = JSON.parse(data);
  } catch (err) {
    console.warn(`[SYNC] Corrupt ${TOMBSTONES_FILE}; starting with empty tombstone set`);
    return new Map();
  }

  const map = new Map();
  const cutoff = Date.now() - TOMBSTONE_TTL_MS;
  for (const [key, value] of Object.entries(parsed)) {
    if (typeof value === 'number' && value >= cutoff) {
      map.set(key, value);
    }
  }
  return map;
}

async function saveTombstones(metaDir, map) {
  const obj = Object.fromEntries(map);
  await atomicWrite(path.join(metaDir, TOMBSTONES_FILE), JSON.stringify(obj, null, 2));
}

module.exports = {
  load,
  save,
  loadState,
  saveState,
  getInode,
  getInodeSync,
  identityOf,
  seenSince,
  seenAt,
  olderThanSeen,
  upgradeIdentities,
  walkDescendants,
  loadTombstones,
  saveTombstones,
  readBaseline,
  applyBaseline
};
