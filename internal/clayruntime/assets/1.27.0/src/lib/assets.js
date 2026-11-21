// Libraries a document asks for by name, loaded the first time it asks. Each is a module in this
// release, so whichever host serves ClayJS serves them too, offline included.
const ASSETS = {
  overtype: () => import("../vendor/overtype.vendor.js"),
  marked: () => import("../vendor/marked.vendor.js"),
  highlight: () => import("../vendor/highlight.vendor.js"),
  "highlight/bash": () => import("../vendor/hljs-bash.vendor.js"),
  "highlight/c": () => import("../vendor/hljs-c.vendor.js"),
  "highlight/cpp": () => import("../vendor/hljs-cpp.vendor.js"),
  "highlight/css": () => import("../vendor/hljs-css.vendor.js"),
  "highlight/diff": () => import("../vendor/hljs-diff.vendor.js"),
  "highlight/go": () => import("../vendor/hljs-go.vendor.js"),
  "highlight/java": () => import("../vendor/hljs-java.vendor.js"),
  "highlight/javascript": () => import("../vendor/hljs-javascript.vendor.js"),
  "highlight/json": () => import("../vendor/hljs-json.vendor.js"),
  "highlight/markdown": () => import("../vendor/hljs-markdown.vendor.js"),
  "highlight/python": () => import("../vendor/hljs-python.vendor.js"),
  "highlight/ruby": () => import("../vendor/hljs-ruby.vendor.js"),
  "highlight/rust": () => import("../vendor/hljs-rust.vendor.js"),
  "highlight/sql": () => import("../vendor/hljs-sql.vendor.js"),
  "highlight/swift": () => import("../vendor/hljs-swift.vendor.js"),
  "highlight/typescript": () => import("../vendor/hljs-typescript.vendor.js"),
  "highlight/xml": () => import("../vendor/hljs-xml.vendor.js"),
  "highlight/yaml": () => import("../vendor/hljs-yaml.vendor.js"),
  "highlight-lines": () => import("./highlight-lines.js").then((m) => m.withAssets(assets)),
  uimotion: () => import("./uimotion.js"),
};

export function createAssets(table) {
  const loading = new Map();
  function load(name) {
    if (!Object.hasOwn(table, name)) return Promise.reject(new Error(`clay.assets: no asset named "${name}"`));
    if (!loading.has(name)) {
      loading.set(name, table[name]().catch((error) => {
        loading.delete(name);
        throw error;
      }));
    }
    return loading.get(name);
  }
  return { load, names: () => Object.keys(table) };
}

export const assets = createAssets(ASSETS);
