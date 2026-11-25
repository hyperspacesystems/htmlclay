import { applyData, extractData } from "../../vendor/hyper-html-api-data.vendor.js";
import { finalizeControlForSave } from "../../vendor/control-serialize.vendor.js";
import { downloadHtml, documentName } from "../../core/download.js";

export const COPY = {
  label: "Earlier drafts",
  intro: "This page kept your earlier drafts aside, listed by time.",
};

export function createRecovery({ store, session, getHTML = () => window.clay.getHTML(), download = downloadHtml }) {
  const recordOf = (copy) => session.recoveryRecord(copy);
  const savedAt = (copy) => copy?.savedAt ?? copy?.ops?.at(-1)?.savedAt ?? (copy?.at ? new Date(copy.at).toISOString() : null);

  async function list() {
    const rows = await session.listRecovery();
    return rows.map(({ mark, copy }) => {
      const record = recordOf(copy);
      const fields = copy?.ops ? new Set(copy.ops.flatMap((op) => Object.keys(op.keys ?? {}))).size : Object.keys(record?.data ?? {}).length;
      return { mark, copy, savedAt: savedAt(copy), fields, available: Boolean(record) };
    }).sort((a, b) => (Date.parse(b.savedAt) || 0) - (Date.parse(a.savedAt) || 0));
  }

  function html(copy) {
    const record = recordOf(copy);
    if (!record) throw new Error("This draft cannot be opened by this version.");
    const doc = new DOMParser().parseFromString(getHTML(), "text/html");
    applyData(doc, store.seed(), store.rules);
    applyData(doc, record.data, store.rules);
    if (!store.fitsInto(record.data, extractData(doc, store.rules))) throw new Error("This draft no longer fits the page. Its saved copy is still kept.");
    const unit = record.blockeditor ? record : store.servedUnit();
    const article = doc.querySelector("article[data-blockeditor]");
    if (unit) {
      if (!article) throw new Error("This draft's page editor is no longer available.");
      for (const tag of doc.querySelectorAll('script[type="application/json"][merge][clay~="live"]')) tag.remove();
      for (const [name, value] of Object.entries(unit.live)) {
        const tag = doc.createElement("script");
        tag.type = "application/json";
        tag.setAttribute("merge", name);
        tag.setAttribute("clay", "live");
        tag.textContent = JSON.stringify(value).replace(/</g, "\\u003c");
        article.before(tag);
      }
      article.setAttribute("data-rev", unit.blockeditor.rev);
      article.innerHTML = unit.blockeditor.html;
    }
    for (const control of doc.querySelectorAll('input:not([type="password"]):not([type="file"]), textarea, select')) finalizeControlForSave(control);
    return "<!DOCTYPE html>\n" + doc.documentElement.outerHTML;
  }

  async function downloadCopy(copy) {
    const ok = await download({ html: html(copy), name: documentName() + ".htmlclay" });
    if (!ok) throw new Error("The download could not be made. Try again.");
    return true;
  }

  async function restore(mark) {
    const row = (await list()).find((row) => row.mark === mark);
    if (!row) throw new Error("That draft is no longer available. Open the list again.");
    html(row.copy);
    return session.restoreRecovery(mark);
  }

  return { list, html, download: downloadCopy, restore, remove: (mark) => session.removeRecovery(mark) };
}
