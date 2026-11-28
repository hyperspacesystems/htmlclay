import { Marked } from "../../vendor/marked.vendor.js";
import { importMarkdownTokens, MarkdownImportError, MAX_BYTES } from "../../page/import-markdown.js";
import { fragmentCleaner, project, renderModel, revOf } from "../../page/projection.js";
import { documentTitle } from "../../page/document-title.js";

export { MarkdownImportError, MAX_BYTES };

/**
 * Markdown text as a Block editor article: the inner HTML of `<article data-blockeditor>`,
 * the `data-rev` that article carries, and the title a save would write. The same codec
 * and projection the editor uses, so the editor opens the result as its own page.
 * `document` is any DOM document (jsdom on a server); raw HTML blocks are cleaned with it.
 * `resolveImage(src)` may return a replacement address for an image, or null; `resolveLink(href)`
 * does the same for a link (a link to another local file is otherwise kept as text).
 */
export function markdownArticle(text, { document, resolveImage = null, resolveLink = null } = {}) {
  if (!document) throw new TypeError("markdownArticle needs a DOM document");
  const tokens = new Marked().lexer(String(text ?? ""));
  const { blocks, unsupported, refused } = importMarkdownTokens(tokens, { document, resolveImage, resolveLink });
  const model = project(blocks);
  const html = renderModel(model, { fallbackOf: () => "", clean: fragmentCleaner(document) });
  return { html, rev: revOf(model), title: documentTitle(model), unsupported, refused };
}
