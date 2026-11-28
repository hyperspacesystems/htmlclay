// Recovery copies and the decision record on the host (plan 09 sections 1.12, 2.8 and 3.4), behind
// the core's synchronous port. The core writes a copy and reads it back in one call; here both are
// requests, so the first call starts them and the core hears "not stored yet" (null). The core then
// keeps the copy's edits in the tab (`tab.unstored`) and asks again on its next read, which `wake`
// asks for once the host has answered. A copy counts as stored only when the bytes read back equal
// the bytes written. A different copy under the same id is never overwritten: the host refuses it
// (409) and the edits stay in the tab. Decision marks go the same way through the copy's done route.
import { parsePage } from "./core/record.js";
import { CLOSING, COPY_VERSION } from "./core/recover.js";
import { useCodec } from "./core/codec.js";

const markKey = ({ copy, e, kind }) => `${copy}/${e}/${kind}`;
const copyPath = (id, query, rest = "") => `/_/copies/${encodeURIComponent(id)}${rest}?${query}`;

/**
 * @param {{ codec: object, fetch: typeof fetch, query: string, wake: () => void, remote?: boolean, retryMs?: number }} options
 * `remote` is false on a host that does not announce `recovery-copies`: nothing is ever stored, so
 * the core keeps every stranded edit in the tab and the status counts it as not stored.
 */
export function copyStore({ codec, fetch, query, wake, remote = true, retryMs = 10_000 }) {
  const held = new Map();
  const parsed = new Map();
  const marks = new Map();
  const tries = new Map();
  const timers = new Set();
  let closed = false;

  // A page that is not a whole copy (cut short, empty, another document's, a copy format this
  // version does not know) reads as no copy.
  function readCopy(body) {
    if (!body || !body.includes("</script>")) return null;
    try {
      useCodec(codec);
      const { record, wb } = parsePage(body);
      if (record?.v !== COPY_VERSION || !Array.isArray(record.edits)) return null;
      return { ...record, preview: wb };
    } catch {
      return null;
    }
  }

  // The pause is over when the timer fires, whatever the wall clock says.
  function later(key) {
    if (closed) return;
    const timer = setTimeout(() => {
      timers.delete(timer);
      const last = tries.get(key);
      if (last) last.at = -Infinity;
      if (!closed) wake();
    }, retryMs);
    timers.add(timer);
  }

  // One request chain per copy or mark at a time, and a failed one again only retryMs after it
  // ended. A refusal the host will repeat ("final": a copy too big, a bad id) is never sent again.
  function attempt(key, work) {
    const last = tries.get(key);
    if (!remote || closed || last?.busy || last?.final || (last && Date.now() - last.at < retryMs)) return;
    const now = { at: Date.now(), busy: true };
    tries.set(key, now);
    work()
      .catch(() => false)
      .then((ok) => {
        now.busy = false;
        now.at = Date.now();
        if (ok === "final") now.final = true;
        else if (ok) tries.delete(key);
        else later(key);
        if (!closed) wake();
      });
  }

  function keep(id, bytes) {
    held.set(id, bytes);
    parsed.delete(id);
  }

  // 201 created, 200 the same bytes were there already, 409 another copy holds the id: in every
  // case only the read back says what the host holds, and only equal bytes are kept.
  // A 413 (the copy does not fit), 400 or 409 (other bytes hold the id) is final: the edits stay in
  // the tab, counted as not stored.
  async function writeCopy(id, bytes) {
    const put = await fetch(copyPath(id, query), { method: "PUT", headers: { "content-type": "text/html;charset=UTF-8" }, body: bytes });
    if (put.status === 413 || put.status === 400 || put.status === 409) return "final";
    if (![200, 201].includes(put.status)) return false;
    const back = await fetch(copyPath(id, query));
    if (!back.ok || (await back.text()) !== bytes) return false;
    keep(id, bytes);
    return true;
  }

  async function writeMark(mark) {
    const answer = await fetch(copyPath(mark.copy, query, "/done"), { method: "POST", headers: { "content-type": "application/json" }, body: JSON.stringify({ e: mark.e, kind: mark.kind }) });
    // A host that will never take the mark (the copy is gone, 404; a bad mark, 400): this tab keeps
    // it, so its restore job goes on; only other tabs miss the label.
    if (answer.status === 404 || answer.status === 400) {
      marks.set(markKey(mark), { copy: mark.copy, e: mark.e, kind: mark.kind });
      return "final";
    }
    if (!answer.ok) return false;
    const { done = [] } = await answer.json();
    if (!done.some((one) => one.e === mark.e && one.kind === mark.kind)) return false;
    marks.set(markKey(mark), { copy: mark.copy, e: mark.e, kind: mark.kind });
    return true;
  }

  // What the host holds for this document: every copy (another tab's included) and every mark.
  // A stored copy never changes, so each body is kept once it reads as a whole copy. A list that
  // cannot be read leaves what this tab already knows: the tab's own step must go on.
  async function load() {
    if (!remote || closed) return;
    try {
      const answer = await fetch(`/_/copies?${query}`);
      if (!answer.ok) return;
      const { copies = [] } = await answer.json();
      for (const one of copies) {
        for (const mark of one.done ?? []) marks.set(markKey({ copy: one.id, ...mark }), { copy: one.id, e: mark.e, kind: mark.kind });
        if (held.has(one.id)) continue;
        const body = await fetch(copyPath(one.id, query));
        const text = body.ok ? await body.text() : null;
        if (readCopy(text)) keep(one.id, text);
      }
    } catch {
      // Read again on the next refresh.
    }
  }

  const record = (id) => {
    if (!parsed.has(id)) parsed.set(id, readCopy(held.get(id)));
    return parsed.get(id);
  };

  return {
    keepRecovery({ id, body }) {
      if (held.get(id) !== body) attempt(`copy ${id}`, () => writeCopy(id, body));
    },
    fetchRecovery: (id) => held.get(id) ?? null,
    // A copy the host will never hold: no copy store at all, or a final refusal (too big, bad id).
    unstorable: (id) => !remote || tries.get(`copy ${id}`)?.final === true,
    readRecovery: (id) => record(id),
    keepDecision(mark) {
      if (!marks.has(markKey(mark))) attempt(`mark ${markKey(mark)}`, () => writeMark(mark));
    },
    fetchDecision: (mark) => marks.has(markKey(mark)),
    marked: (e, kind) => [...marks.values()].some((one) => one.e === e && one.kind === kind),
    verdict(e) {
      const found = [...marks.values()].filter((one) => one.e === e);
      for (const kind of [...CLOSING, "open", "refused"]) {
        const one = found.find((mark) => mark.kind === kind);
        if (one) return one;
      }
      return null;
    },
    load,
    copies: () =>
      [...held.keys()].flatMap((id) => {
        const one = record(id);
        return one ? [{ id, ...one, body: held.get(id), marks: [...marks.values()].filter((mark) => mark.copy === id) }] : [];
      }),
    close() {
      closed = true;
      for (const timer of timers) clearTimeout(timer);
      timers.clear();
    },
  };
}
