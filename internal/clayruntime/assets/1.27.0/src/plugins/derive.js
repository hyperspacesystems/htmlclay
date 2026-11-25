// The sapjs core, for a page with a [sap] root and no full sapjs. It paints the
// presentations the page declares from its records (text, show, attr:, class:,
// css:), and ClayJS treats what it paints as derived (lib/derived.js). Markup
// that needs more is a mount error that names full sapjs. Full sapjs, loaded by
// its own script tag, takes the page over, and this plugin imports no core.
//
// Whichever engine has the page gets its plugins from this release, and the mount
// phase waits for the ones the page asks for: the first pass, and the boot baseline
// taken after it, come once they installed.

// The first party sapjs plugins this release carries, by the name the engine asks for.
// Each is a literal import, like MODULES in loader-logic.js, so the chunk build can
// split it. The table stays in this module: one the loader's own graph also imported
// would become a chunk that every page fetches. The grid is the sheet's engine; a page
// asks for it with `plugins="grid"` on its [sap] root.
export const SAP_PLUGINS = {
  grid: () => import("../vendor/sap-grid.vendor.js"),
};

// What the engine must know of a carried plugin before its code is here. The engine stops
// waiting for plugins after a few seconds; a mount that comes first then leaves a grid
// table's cells alone, and the grid takes the table once it installs.
const DECLARED = {
  grid: { name: "grid", boundaries: { grid: { opaque: true, on: "table" } } },
};

function fullSapjs() {
  const sap = window.Sap;
  return Boolean((sap && !sap.core) || window.clay?.loaded?.sap);
}

async function start() {
  if (!fullSapjs()) await import("../vendor/sap-core.vendor.js");
  const plugins = window.Sap?.plugins;
  if (!plugins) return;
  // A name this release does not carry goes to the loader the page set before, and
  // with none, the engine falls back to its own address rules.
  const pageLoader = plugins.loader;
  plugins.loader = (name, facts) => (Object.hasOwn(SAP_PLUGINS, name) ? SAP_PLUGINS[name]() : pageLoader?.(name, facts));
  for (const root of document.querySelectorAll("[sap][plugins]")) {
    for (const name of root.getAttribute("plugins").match(/\S+/g) ?? []) {
      if (Object.hasOwn(DECLARED, name)) plugins.declare(DECLARED[name]);
    }
  }
  // Null when the page asks for nothing that is missing. Otherwise it resolves once
  // each plugin has installed or failed, or at the engine's own time limit.
  await plugins.ready(document);
}

export const ready = start();
