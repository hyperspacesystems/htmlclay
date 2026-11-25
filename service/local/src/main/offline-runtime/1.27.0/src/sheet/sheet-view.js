import { startShell } from "./shell.js";

// The reader half of `sheet`, added by resolveModules for a sheet in view mode and never
// named in a URL: a reader gets the row window and nothing that edits.
startShell();

export const ready = Promise.resolve();
