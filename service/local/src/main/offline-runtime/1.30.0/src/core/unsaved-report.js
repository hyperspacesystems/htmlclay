/**
 * unsaved-report.js: tell the host which open tabs hold edits not saved yet.
 *
 * A desktop host asks before it quits when something would be lost, but it can only
 * see what reaches it. A page that saves on Ctrl+S, or an edit the autosave has not
 * sent yet, lives only in this tab. A host that announces `unsaved-report-v1` takes a
 * small report from each editable tab: `{tab, seq, unsaved}`, posted to `/_/unsaved`
 * (with the save token when the page has one). The host forgets a tab that stops
 * reporting, so a tab that stays unsaved repeats its report every minute.
 *
 * Nothing is fetched until the page first holds unsaved work, so a page that is only
 * read never asks the host anything. A host without the extension, or one that refuses
 * a report, turns reporting off for the rest of the page's life. A report never
 * throws and never holds a save.
 *
 * The module imports nothing: it loads at a page's first edit, and its caller hands it
 * the host functions it already holds.
 */


export const UNSAVED_REPORT_EXTENSION = "unsaved-report-v1";
const REPORT_PATH = "/_/unsaved";
// An edit is checked a second after it starts, so typing costs one comparison, not
// one per key.
const SETTLE_MS = 1000;
// The host forgets a tab ten minutes after its last report. A minute leaves room for
// a hidden tab whose timers the browser slows down.
const REPEAT_MS = 60_000;
// A host that was not there (restarting, or the network dropped) is asked again.
const RETRY_MS = 5_000;
// While the host holds "unsaved", typing is checked at most this often: often enough
// that undoing back to the saved text clears the question soon, rarely enough that a
// large page is not captured on every pause.
const TYPING_RECHECK_MS = 5_000;

function randomTab() {
  const bytes = new Uint8Array(16);
  crypto.getRandomValues(bytes);
  return Array.from(bytes, (b) => b.toString(16).padStart(2, "0")).join("");
}

/**
 * Start reporting this tab's unsaved state to the host.
 *
 * @param {() => boolean} isUnsaved - the tab's answer to "would closing now lose work?"
 * @param {Function} hostMeta
 * @param {Function} hostMetaOutcome
 * @param {() => string|null} saveToken
 * @returns {{check: () => Promise<void>, soon: () => void, stop: () => void}}
 */
export function startUnsavedReports(isUnsaved, hostMeta, hostMetaOutcome, saveToken) {
  // Made with the first report, so a page that never reports does no work for it.
  let tab = null;
  let seq = 0;
  // What the host last accepted from this tab: null before the first report.
  let sent = null;
  let sentAt = 0;
  let supported = null;
  let freshMeta = false;
  let off = false;
  // The page is going away or kept for Back: nothing is checked or reported until it
  // shows again, and a report that lands after it hid changes nothing.
  let hidden = false;
  // The last state put on the wire, answered or not, so leaving clears an
  // "unsaved" that is still on its way.
  let wireUnsaved = false;
  let timer = null;
  let checking = null;
  let again = false;
  let watching = null;

  const listeners = [];
  const listen = (target, type, fn, options) => {
    target.addEventListener(type, fn, options);
    listeners.push(() => target.removeEventListener(type, fn, options));
  };

  function stop() {
    off = true;
    clearTimeout(timer);
    clearInterval(watching);
    for (const remove of listeners.splice(0)) remove();
  }

  function unsavedNow() {
    try {
      return !!isUnsaved();
    } catch {
      return true;
    }
  }

  // Pinned to the page's own origin, as saves are (save-core.js resolveSaveUrl): a
  // `<base href>` elsewhere would otherwise carry the token to the origin it names.
  function post(unsaved, { keepalive = false, frozen = false } = {}) {
    const token = saveToken();
    seq += 1;
    wireUnsaved = unsaved;
    tab ??= randomTab();
    const path = token ? `${REPORT_PATH}/${token}` : REPORT_PATH;
    return fetch(new URL(path, window.location.origin).href, {
      method: "POST",
      credentials: token ? "omit" : "same-origin",
      keepalive,
      headers: {
        "Content-Type": "text/plain;charset=UTF-8",
        "Document-URL": window.location.href,
      },
      body: JSON.stringify(frozen ? { tab, seq, unsaved, frozen } : { tab, seq, unsaved }),
    });
  }

  async function hostTakesReports() {
    if (supported === null) {
      const meta = await hostMeta(freshMeta ? { fresh: true } : undefined);
      // A host that failed to answer (restarting for an update, say) has not said
      // it takes no reports: ask again shortly.
      if (hostMetaOutcome(meta) === "failed") {
        freshMeta = true;
        schedule(RETRY_MS);
        return false;
      }
      supported = meta.extensions.includes(UNSAVED_REPORT_EXTENSION);
    }
    if (!supported) stop();
    return supported;
  }

  async function report(unsaved) {
    const mine = seq + 1;
    let res;
    try {
      res = await post(unsaved);
    } catch {
      res = null;
    }
    // A newer report, or the page hiding, has spoken since this one left.
    if (off || hidden || seq !== mine) return;
    if (res && res.ok) {
      sent = unsaved;
      sentAt = Date.now();
      return;
    }
    // A host that answers but will not take the report (no route, a token it no
    // longer knows, a body it refuses) will not take the next one either.
    if (res && res.status >= 400 && res.status < 500) {
      stop();
      return;
    }
    schedule(RETRY_MS);
  }

  // Once a minute from the first edit on. While the host holds "unsaved" the report
  // is repeated, so it never waits two. Otherwise the page is checked while the
  // dirty gate says it may hold work, which catches an edit no input event
  // announced (a drag, a script) after the gate had already turned dirty.
  function watch() {
    if (watching || off) return;
    watching = setInterval(() => {
      if (sent) {
        sentAt = 0;
        check();
      } else if (document.documentElement.hasAttribute("unsaved")) {
        check();
      }
    }, REPEAT_MS);
  }

  async function runCheck() {
    if (off || hidden) return;
    const unsaved = unsavedNow();
    const repeatDue = unsaved && Date.now() - sentAt >= REPEAT_MS;
    if (unsaved === (sent ?? false) && !repeatDue) return;
    if (!(await hostTakesReports()) || hidden) return;
    await report(unsaved);
  }

  // One check at a time; a request for another while one runs is served by a check
  // right after it, so the last state always reaches the host.
  function check() {
    clearTimeout(timer);
    timer = null;
    if (checking) {
      again = true;
      return checking;
    }
    checking = runCheck().finally(() => {
      checking = null;
      if (again) {
        again = false;
        check();
      }
    });
    return checking;
  }

  function schedule(ms = SETTLE_MS) {
    if (off || timer) return;
    watch();
    timer = setTimeout(check, ms);
  }

  const soon = () => schedule();
  const typed = () => schedule(sent ? TYPING_RECHECK_MS : SETTLE_MS);
  listen(document, "input", typed, true);
  listen(document, "change", typed, true);
  for (const type of ["clay:dirty", "clay:unsaved-state-changed", "clay:saves-soon", "clay:sync-applied", "clay:modechange"]) {
    listen(document, type, soon);
  }
  // A save answers the question at once: the person may have pressed Ctrl+S to
  // answer HTML Clay's Quit dialog, and pressed "Check and quit" right after.
  listen(document, "clay:save-saved", () => check());
  listen(document, "clay:clean", () => check());
  listen(document, "visibilitychange", () => {
    if (document.visibilityState === "hidden" && timer) check();
  });
  // Leaving the browser for HTML Clay's dialog: a check that was waiting runs now.
  listen(window, "blur", () => {
    if (timer) check();
  });
  // The tab is going away: the close warning has already asked, so its edits leave
  // with it. Sent as keepalive so the request outlives the page. A page the browser
  // keeps for Back reports again when it comes back.
  listen(window, "pagehide", () => {
    hidden = true;
    clearTimeout(timer);
    timer = null;
    const told = sent || wireUnsaved;
    sent = null;
    sentAt = 0;
    if (told) post(false, { keepalive: true }).catch(() => {});
  });
  // A background tab the browser freezes runs no timers, so it cannot repeat its
  // report. It says so as it freezes, and the host keeps a frozen tab's report
  // until the tab speaks again; it reports afresh when it wakes.
  listen(document, "freeze", () => {
    if (sent) post(true, { keepalive: true, frozen: true }).catch(() => {});
  });
  listen(document, "resume", () => {
    if (sent) sentAt = 0;
    check();
  });
  listen(window, "pageshow", (event) => {
    if (!event.persisted) return;
    hidden = false;
    schedule();
  });

  return { check, soon: () => schedule(), stop };
}
