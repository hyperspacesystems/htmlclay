const ARROWS = {
  ArrowUp: { dr: -1, dc: 0 },
  ArrowDown: { dr: 1, dc: 0 },
  ArrowLeft: { dr: 0, dc: -1 },
  ArrowRight: { dr: 0, dc: 1 },
};

const isMacPlatform = (platform) => /mac|iphone|ipad|ipod/i.test(String(platform ?? ""));

export function keyIntent(event, { platform = "", editing = false, picking = false, composing = false } = {}) {
  if (!event || composing || event.isComposing || event.keyCode === 229) return null;

  const mac = isMacPlatform(platform);
  const ctrl = Boolean(event.ctrlKey);
  const meta = Boolean(event.metaKey);
  const alt = Boolean(event.altKey);
  const shift = Boolean(event.shiftKey);
  const mod = mac ? meta && !ctrl : ctrl && !meta;
  const modOrMacCtrl = mod || (mac && ctrl && !meta);
  const plain = !ctrl && !meta && !alt;
  const key = event.key;

  if (editing) {
    if (key === "Escape" && plain && !shift) return { type: "cancel" };
    if (key === "Enter") {
      if (alt && !ctrl && !meta) return { type: "newline" };
      if (mod && !alt && !shift) return { type: "commitAll" };
      if (plain) return { type: "commit", dr: shift ? -1 : 1, dc: 0 };
      return null;
    }
    if (key === "Tab" && plain) return { type: "commit", dr: 0, dc: shift ? -1 : 1 };
    if (key === "F4" && plain && !shift) return { type: "cycleRef" };
    const arrow = ARROWS[key];
    if (arrow && picking && plain) return { type: "pick", dr: arrow.dr, dc: arrow.dc, extend: shift };
    return null;
  }

  const arrow = ARROWS[key];
  if (arrow) {
    if (alt) return null;
    if (mod) return { type: "edge", dr: arrow.dr, dc: arrow.dc, extend: shift };
    if (plain) return { type: "move", dr: arrow.dr, dc: arrow.dc, extend: shift };
    return null;
  }

  if (key === "Tab" && plain) return { type: "move", dr: 0, dc: shift ? -1 : 1, extend: false };
  if (key === "Enter" && plain) return shift ? { type: "move", dr: -1, dc: 0, extend: false } : { type: "edit" };
  if (key === "F2" && plain && !shift) return { type: "edit" };
  if ((key === "Delete" || key === "Backspace") && plain) return { type: "clear" };

  if (key === "Home" && !alt) {
    if (mod) return { type: "sheetCorner", end: false, extend: shift };
    if (plain) return { type: "rowEdge", end: false, extend: shift };
    return null;
  }
  if (key === "End" && !alt) {
    if (mod) return { type: "sheetCorner", end: true, extend: shift };
    if (plain) return { type: "rowEdge", end: true, extend: shift };
    return null;
  }
  if ((key === "PageDown" || key === "PageUp") && !alt) {
    const dir = key === "PageDown" ? 1 : -1;
    if (mod && !shift) return { type: "tab", delta: dir };
    if (plain) return { type: "page", dir, extend: shift };
    return null;
  }

  if ((key === "a" || key === "A") && mod && !alt && !shift) return { type: "selectAll" };
  if (key === " ") {
    if (mod && !alt && !shift) return { type: "selectCols" };
    if (shift && plain) return { type: "selectRows" };
  }

  if ((key === "z" || key === "Z") && mod && !alt) return shift ? { type: "redo" } : { type: "undo" };
  if ((key === "y" || key === "Y") && mod && !alt && !shift) return { type: "redo" };
  if ((key === "v" || key === "V") && mod && !alt && shift) return { type: "pasteValues" };
  if ((key === "d" || key === "D") && mod && !alt && !shift) return { type: "fillDown" };
  if ((key === "r" || key === "R") && mod && !alt && !shift) return { type: "fillRight" };
  if ((key === "f" || key === "F") && mod && !alt && !shift) return { type: "find" };
  if ((key === "h" || key === "H") && mod && !alt && shift === mac) return { type: "findReplace" };
  if ((key === "j" || key === "J") && mod && !alt && !shift) return { type: "goto" };
  if (key === "/" && mod && !alt) return { type: "shortcuts" };

  if ((key === "b" || key === "B") && mod && !alt && !shift) return { type: "bold" };
  if ((key === "i" || key === "I") && mod && !alt && !shift) return { type: "italic" };
  if ((key === "u" || key === "U") && mod && !alt && !shift) return { type: "underline" };
  const altGr = Boolean(event.getModifierState?.("AltGraph"));
  if (
    (mac && mod && !alt && shift && (key === "x" || key === "X")) ||
    (!mac && !ctrl && !meta && alt && shift && (key === "5" || key === "%" || event.code === "Digit5"))
  ) {
    return { type: "strikethrough" };
  }
  if ((key === "l" || key === "L") && mod && !alt && shift) return { type: "align", align: "left" };
  if ((key === "e" || key === "E") && mod && !alt && shift) return { type: "align", align: "center" };
  if ((key === "r" || key === "R") && mod && !alt && shift) return { type: "align", align: "right" };
  if ((key === "1" || key === "!" || event.code === "Digit1") && modOrMacCtrl && !alt && shift) {
    return { type: "numberFormat", fmt: "#,##0.00" };
  }
  if ((key === "4" || key === "$" || event.code === "Digit4") && modOrMacCtrl && !alt && shift) {
    return { type: "numberFormat", fmt: "$#,##0.00" };
  }
  if ((key === "5" || key === "%" || event.code === "Digit5") && modOrMacCtrl && !alt && shift) {
    return { type: "numberFormat", fmt: "0.00%" };
  }
  if (key === "\\" && mod && !alt && !shift) return { type: "clearFormat" };

  if (modOrMacCtrl && alt && !altGr && ((key === "=" && !shift) || (key === "+" && shift))) {
    return { type: "insertAxis" };
  }
  if (modOrMacCtrl && alt && !altGr && !shift && key === "-") {
    return { type: "deleteAxis" };
  }
  if ((key === "ContextMenu" && plain && !shift) || (key === "F10" && plain && shift)) {
    return { type: "contextMenu" };
  }

  return null;
}
