// The tool host a tool's plugin can reach, filled by the tool view part when it
// loads, per editor. A document whose editor has no host yet notifies nothing.
export const toolViews = new WeakMap();

// The mounted tool blocks of an editor, by id, so work that must pause a tool can
// reach the block that runs it.
export const toolBlockViews = new WeakMap();
