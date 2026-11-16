const { shouldSkipEntry } = require('./file-operations');

// What the structure pass never syncs: any path with a segment the file scanner
// skips, an OS junk file, or an editor's backup or lock file.
const OS_JUNK = new Set(['Thumbs.db', 'desktop.ini']);
const isEditorBackup = (segment) => segment.endsWith('~') || (segment.length > 1 && segment.startsWith('#') && segment.endsWith('#'));
const isIgnoredRel = (rel) => rel.split('/').some((segment) => shouldSkipEntry(segment) || OS_JUNK.has(segment) || isEditorBackup(segment));

module.exports = { isIgnoredRel };
