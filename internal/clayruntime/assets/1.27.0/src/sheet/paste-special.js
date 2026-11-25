export const clipboardDeniedNotice = (mac) =>
  mac
    ? "Your browser didn't let the sheet read the clipboard. Press ⌘V to paste, or ⌘⇧V for values only."
    : "Your browser didn't let the sheet read the clipboard. Press Ctrl+V to paste, or Ctrl+Shift+V for values only.";

export const FORMAT_ONLY_FOREIGN_NOTICE = "Format only works with cells copied from a sheet.";

async function blobText(blob) {
  if (!blob) return "";
  if (typeof blob.text === "function") return blob.text();
  if (typeof blob.arrayBuffer === "function") {
    return new TextDecoder().decode(await blob.arrayBuffer());
  }
  if (typeof FileReader === "function") {
    return new Promise((resolve) => {
      const reader = new FileReader();
      reader.onload = () => resolve(typeof reader.result === "string" ? reader.result : "");
      reader.onerror = () => resolve("");
      reader.readAsText(blob);
    });
  }
  if (typeof Response === "function") return new Response(blob).text();
  return "";
}

export async function readClipboardData(win = window) {
  const read = win?.navigator?.clipboard?.read;
  if (typeof read !== "function") return { ok: false };
  try {
    const items = await read.call(win.navigator.clipboard);
    let html = "";
    let text = "";
    for (const item of items ?? []) {
      const types = item?.types ?? [];
      if (!html && types.includes("text/html")) {
        html = await blobText(await item.getType("text/html"));
      }
      if (!text && types.includes("text/plain")) {
        text = await blobText(await item.getType("text/plain"));
      }
    }
    return { ok: true, html, text };
  } catch {
    return { ok: false };
  }
}

export async function runPasteSpecial(ctx, mode) {
  if (!ctx.ensureReadyForAction()) return false;
  const clip = await readClipboardData(ctx.win);
  if (!clip.ok) {
    ctx.chrome.showNotice(clipboardDeniedNotice(Boolean(ctx.isMac?.())));
    return false;
  }
  return ctx.applyPasteData({
    html: clip.html,
    text: clip.text,
    valuesOnly: mode === "values",
    formatOnly: mode === "format",
    transpose: mode === "transpose",
  });
}
