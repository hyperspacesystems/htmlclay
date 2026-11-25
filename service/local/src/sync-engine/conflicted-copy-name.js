const path = require('upath');

/**
 * The name of the nth copy beside `rel` (n = 1 is the first). Hyperclay accepts only lowercase
 * letters, digits, hyphens and underscores in a name (hyperclay shared/validation.js), so the
 * suffix is hyphenated: "board (conflicted copy).html" could never upload.
 */
function conflictedCopyName(rel, n = 1) {
  const ext = path.extname(rel);
  const stem = rel.slice(0, rel.length - ext.length);
  return `${stem}-conflicted-copy${n === 1 ? '' : `-${n}`}${ext}`;
}

module.exports = { conflictedCopyName };
