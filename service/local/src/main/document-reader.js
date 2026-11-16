const fs = require('fs').promises;
const { AsyncLocalStorage } = require('async_hooks');

// Where the lane halves and the room authority read documents, page state and
// versions from. Outside any scope it is the disk. The managed trial's JS
// service runs each handler inside withReader(snapshotReader, ...), so those
// reads are answered from the snapshot the Go host sent while it holds the
// document's lock, and the service never reads a served root.
//
// A reader has readFile(file, encoding) and stat(file, options), shaped like
// fs/promises, and may have versionFiles(dir, baseDir), shaped like
// blockeditor-rebase's newestVersionFiles, which is used when it is absent.
const disk = Object.freeze({
  readFile: (file, encoding) => fs.readFile(file, encoding),
  stat: (file, options) => fs.stat(file, options),
});

const scope = new AsyncLocalStorage();

function reader() {
  return scope.getStore() ?? disk;
}

function withReader(custom, fn) {
  return scope.run(custom, fn);
}

module.exports = { reader, withReader, disk };
