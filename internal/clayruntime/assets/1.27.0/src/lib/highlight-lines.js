// Highlighted code as lines, for diff and file views that put each line in its own
// row. highlight.js leaves a span open across a line break inside a multi-line string
// or comment, so splitting its output on "\n" breaks the rows; every line here reopens
// what an earlier line left open and closes what it opened. A file's grammar loads
// the first time a file in that language is shown, and never otherwise.
//
// The asset table hands itself in (`withAssets`) rather than this file importing it: a
// static import back into assets.js would split the table into a chunk of its own, one
// more request on every page.

const EXTENSIONS = {
  sh: "bash", bash: "bash", zsh: "bash",
  c: "c", h: "c",
  cc: "cpp", cpp: "cpp", cxx: "cpp", hh: "cpp", hpp: "cpp", hxx: "cpp",
  css: "css",
  diff: "diff", patch: "diff",
  go: "go",
  java: "java",
  js: "javascript", mjs: "javascript", cjs: "javascript", jsx: "javascript",
  json: "json",
  md: "markdown", markdown: "markdown",
  py: "python",
  rb: "ruby",
  rs: "rust",
  sql: "sql",
  swift: "swift",
  ts: "typescript", tsx: "typescript", mts: "typescript", cts: "typescript",
  html: "xml", htm: "xml", htmlclay: "xml", xml: "xml", svg: "xml",
  yml: "yaml", yaml: "yaml",
};

const ESCAPES = { "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;" };
const escapeHtml = (text) => text.replace(/[&<>"]/g, (c) => ESCAPES[c]);
const TAG = /<span(?:\s(?:[^>"']|"[^"]*"|'[^']*')*)?>|<\/span>/gi;
// Grammars that hand embedded code to another: JSX and TSX tags, HTML in Markdown,
// and <script> and <style> inside HTML stay plain until those are registered too.
const EMBEDDED = { javascript: ["xml"], typescript: ["xml"], markdown: ["xml"], xml: ["css", "javascript"] };

export function languageFor(path) {
  const name = String(path ?? "").split("/").pop().toLowerCase();
  const dot = name.lastIndexOf(".");
  if (dot === -1) return null;
  const ext = name.slice(dot + 1);
  return Object.prototype.hasOwnProperty.call(EXTENSIONS, ext) ? EXTENSIONS[ext] : null;
}

export function balanceLines(html) {
  const lines = [];
  const open = [];
  for (const line of String(html).split("\n")) {
    const reopened = open.join("");
    // A closing tag with nothing open would close the row's own markup, so it is dropped.
    const body = line.replace(TAG, (tag) => {
      if (tag.toLowerCase() !== "</span>") {
        open.push(tag);
        return tag;
      }
      if (!open.length) return "";
      open.pop();
      return tag;
    });
    lines.push(reopened + body + "</span>".repeat(open.length));
  }
  return lines;
}

// `highlightLines(code, language)` gives one entry per line of `code`, each a balanced
// HTML string. A language with no grammar in this release gives escaped plain lines.
export function withAssets(assets) {
  async function highlightLines(code, language) {
    const text = String(code ?? "");
    if (!language || !assets.names().includes(`highlight/${language}`)) return text.split("\n").map(escapeHtml);
    const names = [language, ...(EMBEDDED[language] || [])];
    const [{ default: hljs }, ...grammars] = await Promise.all([assets.load("highlight"), ...names.map((name) => assets.load(`highlight/${name}`))]);
    names.forEach((name, i) => {
      if (!hljs.getLanguage(name)) hljs.registerLanguage(name, grammars[i].default);
    });
    return balanceLines(hljs.highlight(text, { language, ignoreIllegals: true }).value);
  }
  return { languageFor, balanceLines, highlightLines };
}
