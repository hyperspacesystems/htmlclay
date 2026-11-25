// Typing joins one undo item per sentence, the same as every other field on the page:
// Mod-z removes the last sentence typed, not the last letter or word. Only the editor's own input path marks a
// transaction; a command or a programmatic dispatch stays one undo item by itself.
import { state } from "../vendor/prosemirror.vendor.js";
import { STEP } from "./blocks.js";

export const HISTORY_INTENT = "blockeditor-history-intent";
export const GROUP_MS = 500;

const blockIdAt = (doc, pos) => {
  const $pos = doc.resolve(pos);
  return $pos.depth ? $pos.node(1).attrs.id : null;
};

const markKey = (st) => (st.storedMarks ?? st.selection.$from.marks()).map((m) => `${m.type.name}:${JSON.stringify(m.attrs)}`).sort().join(",");

// A sentence ends at a space typed right after ., ? or ! (closing quotes and brackets
// allowed between). The space belongs to the sentence it ends, so the step closes after it.
// A second space, or a space after a space, is not a new sentence. The rule is hyper-undo's
// `endsSentence`, kept here so the block editor does not load the engine.
const SENTENCE_END = /[.?!]["'\u201D\u2019)\]]*$/;

const endsSentence = (st, from, text) => {
  if (!/\s$/.test(text)) return false;
  const $from = st.doc.resolve(from);
  const head = $from.parent.textBetween(0, $from.parentOffset) + text.replace(/\s+$/, "");
  return !/\s$/.test(head) && SENTENCE_END.test(head);
};

export function typingIntent(st, from, to, text) {
  return { kind: "insert", blockId: blockIdAt(st.doc, from), from, end: from + text.length, replaced: from !== to, text, marks: markKey(st), breakAfter: endsSentence(st, from, text), at: Date.now() };
}

export function continuesGroup(prev, next) {
  if (!prev || !next || prev.kind !== next.kind) return false;
  if (next.kind === "compose") return prev.id === next.id;
  if (prev.blockId !== next.blockId || next.at - prev.at > GROUP_MS) return false;
  if (next.kind === "insert") return !prev.breakAfter && !next.replaced && next.from === prev.end && next.marks === prev.marks;
  return next.direction === prev.direction && (next.direction < 0 ? next.to === prev.from : next.from === prev.from);
}

const DELETES = { deleteContentBackward: -1, deleteContentForward: 1 };
const pendingDeletes = new WeakMap();
const PENDING_MS = 1000;
export const DERIVED = "blockeditor-derived";

// A native deletion arrives as a DOM change with no handler, sometimes after a delayed flush
// (Android waits 20 ms), so the beforeinput that announced it is held with the document it saw
// and claimed by the next document change, whatever that change turns out to be.
export function takePendingDelete(view) {
  const mark = pendingDeletes.get(view) ?? null;
  pendingDeletes.delete(view);
  return mark;
}

export function deletionIntent(tr, mark) {
  if (!mark || tr.before !== mark.doc || Date.now() - mark.at > PENDING_MS || tr.getMeta(STEP) || tr.steps.length !== 1) return null;
  const step = tr.steps[0];
  if (step.slice?.size !== 0 || step.from === step.to) return null;
  const $from = tr.before.resolve(step.from), $to = tr.before.resolve(step.to);
  if (!$from.parent.isTextblock || !$from.sameParent($to)) return null;
  return { kind: "delete", blockId: $from.depth ? $from.node(1).attrs.id : null, direction: mark.direction, from: step.from, to: step.to, at: mark.at };
}

export function intentOf(transactions, view = null) {
  const pending = view ? takePendingDelete(view) : null;
  const primary = transactions.filter((t) => !t.getMeta(DERIVED));
  if (primary.length !== 1) return null;
  const tr = primary[0];
  const composition = tr.getMeta("composition");
  if (composition != null) return { kind: "compose", id: composition, at: Date.now() };
  return tr.getMeta(HISTORY_INTENT) ?? deletionIntent(tr, pending);
}

export function historyIntentPlugin() {
  return new state.Plugin({
    props: {
      handleTextInput(view, from, to, text, deflt) {
        view.dispatch(deflt().setMeta(HISTORY_INTENT, typingIntent(view.state, from, to, text)));
        return true;
      },
      handleDOMEvents: {
        beforeinput(view, event) {
          const direction = DELETES[event.inputType];
          if (direction) pendingDeletes.set(view, { direction, at: Date.now(), doc: view.state.doc });
          return false;
        },
      },
    },
  });
}

// Each page entry owns one current Yjs StackItem, replaced by its inverse after replay.
export function createHistoryBridge({ um, undo, redo, article, focus }) {
  const page = () => window.clay?.undo
  const epoch = Math.random().toString(36).slice(2)
  let next = 0
  let running = false
  let recording = false
  let disposed = false

  const replay = (entry, direction) => {
    const fromKey = direction === 'undo' ? 'undoStack' : 'redoStack'
    const toKey = direction === 'undo' ? 'redoStack' : 'undoStack'
    const source = um[fromKey], destination = um[toKey]
    const index = source.indexOf(entry.item)
    if (index < 0) throw new Error('blockeditor: history item is no longer available')
    const remaining = source.filter((_, at) => at !== index)
    // Yjs scans BOTH stacks' deletion sets when checking map conflicts. Keep those
    // sets visible on the opposite stack while limiting the pop loop to one item.
    const temporary = [...destination, ...remaining]
    const boundary = temporary.length
    um[fromKey] = [entry.item]
    um[toKey] = temporary
    running = true
    let changed
    let inverse
    try {
      changed = direction === 'undo' ? undo() : redo()
    } finally {
      inverse = temporary.slice(boundary)
      um[fromKey] = remaining
      um[toKey] = [...destination, ...inverse]
      running = false
      um.stopCapturing()
    }
    if (!changed || !inverse.length) {
      entry.item = null
      throw new Error('blockeditor: history item was emptied by another edit')
    }
    entry.item = inverse[0]
    entry.applied = direction === 'redo'
  }

  const onAdded = (event) => {
    if (running || event.origin === um || event.type !== 'undo' || page()?.pageHistory !== true) return
    const entry = { item: event.stackItem, applied: true }
    const id = `${epoch}:${++next}`
    const oldValue = `${id}:undone`, newValue = `${id}:applied`
    const primitive = {
      kind: 'value',
      target: article,
      prop: `blockeditor-step:${id}`,
      oldValue,
      newValue,
      read: () => !disposed && entry.item && (entry.applied ? um.undoStack : um.redoStack).includes(entry.item)
        ? entry.applied ? newValue : oldValue
        : null,
      write: (target, wanted) => {
        const typing = !!document.activeElement?.closest?.('[contenteditable]:not([contenteditable="false"])')
        const before = article.textContent
        replay(entry, wanted === oldValue ? 'undo' : 'redo')
        if (typing && article.textContent !== before && typeof focus === 'function') focus()
      },
    }
    recording = true
    try {
      try { page().recordStep('Edit', primitive) } catch { page().record(primitive) }
    } finally {
      recording = false
    }
  }

  const onCommit = () => { if (!recording) um.stopCapturing() }
  um.on('stack-item-added', onAdded)
  let subscribed = null
  const subscribe = () => {
    const current = page()?.pageHistory === true ? page() : null
    if (current === subscribed) return
    subscribed?.off?.('commit', onCommit)
    subscribed = current
    subscribed?.on?.('commit', onCommit)
  }
  subscribe()
  document.addEventListener('clay:undo-session', subscribe)

  return {
    dispose() {
      disposed = true
      um.off('stack-item-added', onAdded)
      subscribed?.off?.('commit', onCommit)
      document.removeEventListener('clay:undo-session', subscribe)
    },
  }
}
