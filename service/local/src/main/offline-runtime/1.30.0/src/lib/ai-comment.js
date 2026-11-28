// The comment conventions an AI request shares: the payload ceiling, the @tokens that
// name a context file, and whether a comment asks for the saved page.

// The request ceiling, kept below the host's 1 MiB envelope so the refusal happens
// here, where the message can say what to do about it.
export const MAX_PAYLOAD_BYTES = 900 * 1024;

// Context refs are @tokens containing a dot or slash (@notes.md, @src/x.js). Bare
// @words are not refs: a leading one is an engine token (@fable, @codex — routed
// helper-side), and mid-text ones are prose.
// The @ has to start a word, so an email address or a URL is prose too.
export function contextRefsOf(comment) {
  return [...String(comment ?? '').matchAll(/(?<![\w\u00C0-\u024F.@\/:-])@([\w.-]*[/.][\w./-]*)/g)]
    .map(m => m[1].replace(/\.+$/, '')) // a sentence's full stop is not part of the name
    .filter(ref => ref !== 'page')
    .filter(ref => /[/.]/.test(ref));
}

// Every request that names @page reads the file from disk rather than a copy in the
// payload, so the caller has to save first and then ask for it by flag.
export function wantsPage(comment) {
  return /(?<![\w\u00C0-\u024F.@\/:-])@page(?![\w\/-]|\.\w)/.test(String(comment ?? ''));
}
