import { bevelBox, bevelButton, bevelText } from "../ui/bevel-controls.js";
import { bevelDialog, dismissOf } from "../ui/bevel-dialog.js";
import { mountEditorUI } from "../ui/editor-ui.js";
import { FONT_MONO, FONT_SANS, TOKENS } from "../ui/bevel.js";
import { resolveTheme } from "../ui/theme.js";

const isMacPlatform = (platform) => /mac|iphone|ipad|ipod/i.test(String(platform ?? ""));

export function shortcutGroups(platform = "") {
  const mac = isMacPlatform(platform);
  const mod = mac ? "⌘" : "Ctrl+";
  const shift = mac ? "⇧" : "Shift+";
  const alt = mac ? "⌥" : "Alt+";

  return [
    {
      title: "Move and select",
      items: [
        { label: "Move one cell", keys: "Arrow keys" },
        { label: "Extend selection", keys: `${shift}Arrow keys` },
        { label: "Jump to data edge", keys: `${mod}Arrow keys` },
        { label: "Select to data edge", keys: `${mod}${shift}Arrow keys` },
        { label: "Move right or left", keys: `Tab / ${shift}Tab` },
        { label: "Move up", keys: `${shift}Enter` },
        { label: "Beginning or end of row", keys: "Home / End" },
        { label: "Select to row edge", keys: `${shift}Home / ${shift}End` },
        { label: "Beginning or end of sheet", keys: `${mod}Home / ${mod}End` },
        { label: "Select to sheet corner", keys: `${mod}${shift}Home / ${mod}${shift}End` },
        { label: "Move one screen up or down", keys: "Page Up / Page Down" },
        { label: "Select one screen up or down", keys: `${shift}Page Up / ${shift}Page Down` },
        { label: "Next or previous sheet", keys: `${mod}Page Down / ${mod}Page Up` },
        { label: "Select all", keys: `${mod}A` },
        { label: "Select columns", keys: `${mod}Space` },
        { label: "Select rows", keys: `${shift}Space` },
      ],
    },
    {
      title: "Edit",
      items: [
        { label: "Edit cell", keys: "Enter / F2" },
        { label: "Cancel edit", keys: "Escape" },
        { label: "Insert line break in cell", keys: `${alt}Enter` },
        { label: "Fill selection with entry", keys: `${mod}Enter` },
        { label: "Cycle reference lock ($A$1)", keys: "F4" },
        { label: "Clear selected cells", keys: "Delete / Backspace" },
        { label: "Undo", keys: `${mod}Z` },
        { label: "Redo", keys: mac ? "⌘⇧Z / ⌘Y" : "Ctrl+Y / Ctrl+Shift+Z" },
        { label: "Paste values only", keys: `${mod}${shift}V` },
        { label: "Fill down", keys: `${mod}D` },
        { label: "Fill right", keys: `${mod}R` },
      ],
    },
    {
      title: "Format",
      items: [
        { label: "Bold", keys: `${mod}B` },
        { label: "Italic", keys: `${mod}I` },
        { label: "Underline", keys: `${mod}U` },
        { label: "Strikethrough", keys: mac ? "⌘⇧X" : "Alt+Shift+5" },
        { label: "Align left", keys: `${mod}${shift}L` },
        { label: "Align center", keys: `${mod}${shift}E` },
        { label: "Align right", keys: `${mod}${shift}R` },
        { label: "Number format", keys: `${mod}${shift}1` },
        { label: "Currency format", keys: `${mod}${shift}4` },
        { label: "Percent format", keys: `${mod}${shift}5` },
        { label: "Clear formatting", keys: `${mod}\\` },
      ],
    },
    {
      title: "Rows and columns",
      items: [
        { label: "Insert rows or columns", keys: `${mod}${alt}=` },
        { label: "Delete rows or columns", keys: `${mod}${alt}-` },
      ],
    },
    {
      title: "Find and tools",
      items: [
        { label: "Find", keys: `${mod}F` },
        { label: "Find and replace", keys: mac ? "⌘⇧H" : "Ctrl+H" },
        { label: "Go to", keys: `${mod}J` },
        { label: "Context menu", keys: `${shift}F10` },
        { label: "Keyboard shortcuts", keys: `${mod}/` },
      ],
    },
  ];
}

export function openShortcutsDialog({
  doc = document,
  theme = resolveTheme(),
  platform = "",
  container = null,
  getReturnFocus,
  onClose,
} = {}) {
  const title = "Keyboard shortcuts";
  const shell = bevelDialog({ zIndex: "2147483001", width: "560px", closable: true, titled: true, theme });
  shell.root.setAttribute("sap-ignore", "");
  shell.root.setAttribute("data-sheet-shortcuts", "");
  shell.heading.textContent = title;
  shell.panel.setAttribute("aria-label", title);

  const stack = bevelBox("div", ["display:flex", "flex-direction:column", "gap:16px"], { theme });
  for (const group of shortcutGroups(platform)) {
    const section = bevelBox("section", ["display:flex", "flex-direction:column", "gap:6px"], { theme });
    section.setAttribute("data-shortcut-group", group.title);
    const heading = bevelText(
      "h3",
      ["margin:0", `font:600 13px/1.3 ${FONT_SANS}`, `color:${TOKENS.ink}`],
      group.title,
      { theme },
    );
    const list = bevelBox("div", ["display:flex", "flex-direction:column", "gap:4px"], { theme });
    for (const entry of group.items) {
      const row = bevelBox(
        "div",
        [
          "display:flex",
          "align-items:center",
          "justify-content:space-between",
          "gap:12px",
          "padding:4px 8px",
          `background:${TOKENS.sunk}`,
          `border:1px solid ${TOKENS["line-2"]}`,
        ],
        { theme },
      );
      row.setAttribute("data-shortcut-row", "");
      const label = bevelText("span", [`font:400 12.5px/1.35 ${FONT_SANS}`, `color:${TOKENS["ink-2"]}`], entry.label, { theme });
      const keys = bevelText(
        "kbd",
        [
          `font:600 11.5px/1.2 ${FONT_MONO}`,
          "font-variant-numeric:tabular-nums",
          "white-space:nowrap",
          "padding:2px 6px",
          `background:${TOKENS.surface}`,
          `border:1px solid ${TOKENS["line-2"]}`,
          `color:${TOKENS.ink}`,
        ],
        entry.keys,
        { theme },
      );
      keys.setAttribute("data-shortcut-keys", "");
      row.append(label, keys);
      list.append(row);
    }
    section.append(heading, list);
    stack.append(section);
  }
  shell.body.append(stack);
  // The list is longer than a laptop screen: the body takes focus so the arrow keys scroll it.
  shell.body.tabIndex = 0;

  let done = false;
  let pressedBackdrop = false;
  let detach = null;

  const closeBtn = bevelButton("Close", {
    variant: "primary",
    theme,
    parts: ["dialog.button"],
    labelParts: ["dialog.buttonLabel"],
    onClick: () => close(),
  });
  shell.footer.append(closeBtn);

  const focusables = () =>
    [...shell.panel.querySelectorAll('button:not([disabled]), [tabindex="0"]')].filter(
      (el) => !el.hidden && el.getAttribute("aria-disabled") !== "true",
    );

  function onKeyDown(event) {
    if (event.key === "Escape") {
      event.preventDefault();
      event.stopPropagation();
      close();
      return;
    }
    if (event.key === "Tab") {
      const nodes = focusables();
      if (!nodes.length) return;
      event.stopPropagation();
      const active = shell.panel.getRootNode()?.activeElement ?? doc.activeElement;
      const idx = nodes.indexOf(active);
      if (idx === -1) {
        event.preventDefault();
        nodes[0].focus();
      } else if (event.shiftKey && idx === 0) {
        event.preventDefault();
        nodes[nodes.length - 1].focus();
      } else if (!event.shiftKey && idx === nodes.length - 1) {
        event.preventDefault();
        nodes[0].focus();
      }
    }
  }

  function close() {
    if (done) return;
    done = true;
    doc.removeEventListener("keydown", onKeyDown, true);
    detach?.();
    detach = null;
    getReturnFocus?.()?.focus?.();
    onClose?.();
  }

  shell.close?.addEventListener("click", close);
  shell.panel.addEventListener("submit", (event) => {
    event.preventDefault();
    close();
  });
  shell.overlay.addEventListener("mousedown", (event) => {
    pressedBackdrop = event.target === shell.overlay;
  });
  shell.overlay.addEventListener("click", (event) => {
    if (pressedBackdrop && event.target === shell.overlay) close();
    pressedBackdrop = false;
  });
  doc.addEventListener("keydown", onKeyDown, true);
  dismissOf.set(shell.root, close);

  shell.root.setAttribute("aria-hidden", "false");
  if (container) {
    container.append(shell.root);
    detach = () => shell.root.remove();
  } else {
    const mounted = mountEditorUI({
      name: "sheet-dialog",
      content: shell.root,
      parent: doc.body,
      attributes: { "sap-ignore": "" },
    });
    detach = () => mounted.remove();
  }

  closeBtn.focus();
  return { shell, closeBtn, close };
}

export function showShortcutsDialog(ctx) {
  if (!ctx.ensureReadyForAction()) return null;
  const sel = ctx.getSel();
  const handle = openShortcutsDialog({
    doc: ctx.doc,
    theme: ctx.chrome.theme,
    platform: ctx.platform ?? ctx.win.navigator?.platform ?? "",
    container: ctx.chrome.topRoot,
    getReturnFocus: () => ctx.editor.textarea,
    onClose: () => ctx.clearDialog(),
  });
  return ctx.showDialog(sel?.tab ?? "", handle);
}
