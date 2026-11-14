import { COPY } from "./recovery.js";
import { bevelBox, bevelButton, bevelText, setShown } from "../../ui/bevel-controls.js";
import { FONT_MONO } from "../../ui/bevel.js";
import { resolveTheme } from "../../ui/theme.js";
import modal, { SURFACE_THEME } from "../../ui/modal.js";

export function attachRecoveryMenu(shell, load) {
  let request = 0;
  const refresh = async () => {
    const id = ++request;
    try {
      const recovery = await load();
      const rows = recovery ? await recovery.list() : [];
      if (id !== request) return;
      if (!rows.length) return shell.removeItem("clay.earlier-drafts");
      shell.addItem({ id: "clay.earlier-drafts", label: COPY.label, run: () => openRecovery(recovery, { refresh, anchor: shell.goo }).catch(() => shell.note("The drafts could not be opened. Try again.")) });
    } catch {
      if (id === request) shell.removeItem("clay.earlier-drafts");
    }
  };
  shell.beforeOpen.push(refresh);
  refresh();
  return refresh;
}

export async function openRecovery(recovery, { refresh = () => {}, anchor = document.activeElement } = {}) {
  const theme = resolveTheme();
  const box = bevelBox("div", ["display:flex", "flex-direction:column", "gap:16px"], { theme, parts: ["dialog.content"] });
  box.dataset.draftCopies = "";
  box.append(bevelText("p", ["display:block", "margin:0"], COPY.intro, { theme }));
  const status = bevelText("p", ["display:block", "margin:0"], "", { theme });
  status.setAttribute("role", "status");
  const list = bevelBox("ol", ["display:flex", "flex-direction:column", "gap:16px", "padding:0", "margin:0", "list-style:none"], { theme });
  box.append(status, list);
  let busy = false;

  async function draw() {
    const rows = await recovery.list();
    list.replaceChildren();
    if (!rows.length) status.textContent = "No earlier drafts are kept.";
    for (const row of rows) {
      const item = bevelBox("li", ["display:flex", "flex-direction:column", "gap:8px"], { theme });
      item.dataset.draftMark = row.mark;
      const date = new Date(row.savedAt);
      const when = Number.isNaN(date.getTime()) ? "Time unavailable" : date.toLocaleString();
      const time = bevelText("time", ["display:block", `font-family:${FONT_MONO}`], when, { theme });
      if (row.savedAt) time.dateTime = row.savedAt;
      item.append(time, bevelText("p", ["display:block", "margin:0"], `${row.fields} saved ${row.fields === 1 ? "field" : "fields"}`, { theme, role: "mutedText" }));
      const actions = bevelBox("div", ["display:flex", "flex-wrap:wrap", "gap:8px"], { theme });
      const preview = bevelBox("div", ["display:block"], { theme });
      setShown(preview, false);
      const confirm = bevelBox("div", ["display:flex", "flex-wrap:wrap", "gap:8px", "align-items:center"], { theme });
      setShown(confirm, false);
      const action = (label, fn, options = {}) => {
        const button = bevelButton(label, { small: true, theme, ...options, onClick: async () => {
          if (busy) return;
          busy = true;
          button.setDisabled(true);
          status.textContent = "";
          try { await fn(button); }
          catch (error) { status.textContent = error.message || "That did not work. Try again."; }
          finally { busy = false; button.setDisabled(false); }
        } });
        button.setAttribute("aria-label", `${label}: ${when}`);
        return button;
      };
      const open = action("Preview", () => {
        if (!preview.hidden) { setShown(preview, false); open.setAttribute("aria-expanded", "false"); return; }
        const doc = new DOMParser().parseFromString(recovery.html(row.copy), "text/html");
        for (const el of doc.querySelectorAll('script, meta[http-equiv="refresh" i]')) el.remove();
        for (const el of doc.querySelectorAll("input, textarea, select, button")) el.disabled = true;
        for (const el of doc.querySelectorAll("[contenteditable]")) el.removeAttribute("contenteditable");
        for (const el of doc.querySelectorAll("a, area")) {
          el.removeAttribute("href");
          el.removeAttributeNS("http://www.w3.org/1999/xlink", "href");
          el.removeAttribute("xlink:href");
        }
        const policy = doc.createElement("meta");
        policy.httpEquiv = "Content-Security-Policy";
        policy.content = "default-src 'none'; style-src 'unsafe-inline'; img-src data: blob:; form-action 'none'";
        doc.head.prepend(policy);
        const frame = bevelBox("iframe", ["display:block", "width:100%", "height:45vh", "border:0"], { theme });
        frame.title = `Draft preview: ${when}`;
        frame.setAttribute("sandbox", "");
        frame.srcdoc = "<!DOCTYPE html>\n" + doc.documentElement.outerHTML;
        preview.replaceChildren(frame);
        setShown(preview, true);
        open.setAttribute("aria-expanded", "true");
      });
      open.setAttribute("aria-expanded", "false");
      const restore = action("Restore", async () => { await recovery.restore(row.mark); status.textContent = "Draft restored. Reloading the page."; await refresh(); });
      const download = action("Download", () => recovery.download(row.copy));
      for (const button of [open, restore, download]) button.setDisabled(!row.available);
      const remove = action("Remove", () => { setShown(confirm, true, "flex"); cancel.focus(); });
      const cancel = bevelButton("Keep draft", { small: true, theme, onClick: () => { setShown(confirm, false); remove.focus(); } });
      confirm.append(bevelText("p", ["margin:0"], "Remove this saved draft?", { theme }), cancel, action("Remove draft", async () => {
        await recovery.remove(row.mark);
        await draw();
        await refresh();
        list.querySelector("button")?.focus();
      }, { variant: "danger" }));
      actions.append(open, restore, download, remove);
      item.append(actions, confirm, preview);
      list.append(item);
    }
  }

  await draw();
  modal.title = COPY.label;
  modal.html = box;
  modal.width = "720px";
  modal[SURFACE_THEME] = theme;
  modal.closeHtml = "x";
  modal.no = "Close";
  modal.onNo(() => anchor?.focus());
  modal.open();
  return box;
}
