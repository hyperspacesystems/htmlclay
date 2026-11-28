// The one way a typed value reaches the workbook. The tab, row and column are found again by id
// when the person commits, because a peer's merge or a rebuild can move rows or replace the
// table while the editor is open: the value lands in the cell the person chose, or nowhere.

const childOf = (el, test) => {
  for (let node = el?.firstElementChild ?? null; node !== null; node = node.nextElementSibling) if (test(node)) return node;
  return null;
};

export function tabTable(root, tab) {
  const section = childOf(root, (el) => el.localName === "section" && el.hasAttribute("item") && el.getAttribute("data-id") === tab);
  return childOf(section, (el) => el.localName === "table" && el.hasAttribute("grid"));
}

export function rowIndex(table, id) {
  let i = 0;
  for (const section of [table.tHead, ...table.tBodies]) {
    if (!section) continue;
    for (const tr of section.rows) {
      if (tr.getAttribute("data-id") === id) return i;
      i += 1;
    }
  }
  return -1;
}

export function colIndex(table, id) {
  const group = childOf(table, (el) => el.localName === "colgroup");
  let i = 0;
  for (let col = group?.firstElementChild ?? null; col !== null; col = col.nextElementSibling) {
    if (col.localName !== "col") continue;
    if (col.getAttribute("data-id") === id) return i;
    i += 1;
  }
  return -1;
}

export function colName(c) {
  let name = "";
  for (let n = c + 1; n > 0; n = Math.floor((n - 1) / 26)) name = String.fromCharCode(65 + ((n - 1) % 26)) + name;
  return name;
}

export const a1 = (r, c) => `${colName(c)}${r + 1}`;

// Writes `text` into the cell { tab, row, col } as typed text (one `grid.set`, one action).
// Returns { ok: true, changed } or { ok: false, why } with why "gone" (the tab, row or column
// is no longer in the workbook) or "refused" (the Grid refused it; `error` says why). Text equal
// to the cell's own editor text changes nothing and makes no action.
export function commitCell(root, { tab, row, col }, text) {
  const table = tabTable(root, tab);
  const r = table ? rowIndex(table, row) : -1;
  const c = table ? colIndex(table, col) : -1;
  if (r === -1 || c === -1) return { ok: false, why: "gone" };
  const grid = window.Sap.grid(table);
  const ref = a1(r, c);
  if (grid.input(ref) === text) return { ok: true, changed: false };
  try {
    grid.set(ref, text);
  } catch (error) {
    if (error?.name !== "GridRefused") throw error;
    return { ok: false, why: "refused", error };
  }
  return { ok: true, changed: true };
}
