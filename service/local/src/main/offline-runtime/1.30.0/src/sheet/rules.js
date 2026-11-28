// How a tab's saved rules look (sapjs S8). sapjs stores and moves the rules; this module
// decides which cells each rule touches and paints them through one constructable
// stylesheet per table, adopted on the document. Nothing is written into the file: a save
// never sees the colours, a reader without the editor sees them, and print keeps them.
// Rules run in their saved order, and an earlier rule wins each style property.
//
// The shell hands in its own helpers (`createRules(tools)`) instead of this file importing
// them: a lazy chunk that imports a start-up module splits that module into a chunk of its
// own in the built release, one more request on every sheet page.

export function createRules({ boundRange, isEmpty, listItems, matchKey, parseStyle, readBox, readTexts, canAdopt, scopeOf }) {
  const number = (value) => {
    if (typeof value === "number") return value;
    if (typeof value === "string" && value.trim() !== "" && Number.isFinite(Number(value))) return Number(value);
    return null;
  };

  // A cell against a rule's stored text: a number cell by size against a number, a text cell
  // as text, case-blind (a ZIP code "02134" stays text), and a number never orders against
  // other text.
  function order(value, against) {
    if (typeof value === "number") {
      const other = number(against);
      return other === null ? null : Math.sign(value - other);
    }
    const a = matchKey(value);
    const b = matchKey(against ?? "");
    return a < b ? -1 : a > b ? 1 : 0;
  }

  function holds(rule, value, counts) {
    const text = isEmpty(value) ? "" : matchKey(value);
    switch (rule.test) {
      case "empty": return isEmpty(value);
      case "filled": return !isEmpty(value);
      case "duplicate": return !isEmpty(value) && counts.get(text) > 1;
      case "unique": return !isEmpty(value) && counts.get(text) === 1;
      case "contains": return !isEmpty(value) && text.includes(matchKey(rule.a ?? ""));
      case "starts": return !isEmpty(value) && text.startsWith(matchKey(rule.a ?? ""));
      case "ends": return !isEmpty(value) && text.endsWith(matchKey(rule.a ?? ""));
      case "between":
      case "outside": {
        const lo = number(rule.a);
        const hi = number(rule.b);
        if (typeof value !== "number" || lo === null || hi === null) return false;
        const inside = value >= Math.min(lo, hi) && value <= Math.max(lo, hi);
        return rule.test === "between" ? inside : !inside;
      }
      case "ne": {
        if (isEmpty(value)) return !isEmpty(rule.a);
        return order(value, rule.a) !== 0;
      }
      case "eq":
      case "gt":
      case "ge":
      case "lt":
      case "le": {
        if (isEmpty(value)) return false;
        const o = order(value, rule.a);
        if (o === null) return false;
        return rule.test === "eq" ? o === 0 : rule.test === "gt" ? o > 0 : rule.test === "ge" ? o >= 0 : rule.test === "lt" ? o < 0 : o <= 0;
      }
      default:
        // `formula` waits for a sapjs evaluator for a formula at a given cell.
        return false;
    }
  }

  const rgb = (color) => [1, 3, 5].map((i) => parseInt(color.slice(i, i + 2), 16));
  const mix = (from, to, t) => {
    const a = rgb(from);
    const b = rgb(to);
    return `#${a.map((x, i) => Math.round(x + (b[i] - x) * Math.min(1, Math.max(0, t))).toString(16).padStart(2, "0")).join("")}`;
  };

  // A number's colour on a scale: low at the range's smallest number, high at its largest,
  // and mid, when set, at its median.
  // The range's smallest, median and largest numbers, found once per rule.
  function scaleStats(numbers) {
    const sorted = [...numbers].sort((a, b) => a - b);
    const half = sorted.length >> 1;
    const median = sorted.length % 2 ? sorted[half] : (sorted[half - 1] + sorted[half]) / 2;
    return { min: sorted[0], max: sorted[sorted.length - 1], median };
  }

  function scaleColor(rule, value, { min, max, median }) {
    if (max === min) return rule.mid ?? rule.low;
    if (!rule.mid) return mix(rule.low, rule.high, (value - min) / (max - min));
    return value <= median
      ? mix(rule.low, rule.mid, median === min ? 1 : (value - min) / (median - min))
      : mix(rule.mid, rule.high, max === median ? 1 : (value - median) / (max - median));
  }

  // What every touched cell shows, keyed "row,col" (0-based): `style` (property to value),
  // `check` (a checkbox, checked or not) and `list` ("ok", or "bad" for a value its
  // dropdown does not hold).
  function evaluate(rules, size, read, itemsOf, readText = () => []) {
    const cells = new Map();
    const at = (r, c) => {
      const key = `${r},${c}`;
      let cell = cells.get(key);
      if (!cell) cells.set(key, (cell = { style: new Map() }));
      return cell;
    };
    for (const rule of rules) {
      const box = boundRange(rule.range, size);
      if (!box) continue;
      const values = read(box);
      const each = (fn) => {
        for (let r = box.r0; r <= box.r1; r++) {
          for (let c = box.c0; c <= box.c1; c++) fn(r, c, values[r - box.r0]?.[c - box.c0]);
        }
      };
      if (rule.kind === "check") {
        // A box only for TRUE, FALSE or a blank: a heading or other text in the range shows as itself.
        each((r, c, value) => {
          const cell = at(r, c);
          if (cell.check !== undefined || cell.list !== undefined) return;
          const flag = typeof value === "string" ? value.toUpperCase() : value;
          if (flag === true || flag === "TRUE") cell.check = true;
          else if (flag === false || flag === "FALSE" || isEmpty(value)) cell.check = false;
        });
      } else if (rule.kind === "list") {
        const items = new Set(itemsOf(rule).map((item) => item.toLowerCase()));
        const texts = readText(box);
        each((r, c, value) => {
          const cell = at(r, c);
          if (cell.check !== undefined || cell.list !== undefined) return;
          const text = String(texts[r - box.r0]?.[c - box.c0] ?? "").toLowerCase();
          cell.list = isEmpty(value) || items.has(matchKey(value)) || items.has(text) ? "ok" : "bad";
        });
      } else if (rule.kind === "when") {
        const props = parseStyle(rule.style);
        if (props.length === 0) continue;
        const counts = new Map();
        if (rule.test === "duplicate" || rule.test === "unique") {
          each((r, c, value) => {
            if (!isEmpty(value)) counts.set(matchKey(value), (counts.get(matchKey(value)) ?? 0) + 1);
          });
        }
        each((r, c, value) => {
          if (!holds(rule, value, counts)) return;
          const cell = at(r, c);
          for (const [prop, val] of props) if (!cell.style.has(prop)) cell.style.set(prop, val);
        });
      } else if (rule.kind === "scale") {
        const numbers = [];
        each((r, c, value) => {
          if (typeof value === "number") numbers.push(value);
        });
        if (numbers.length === 0) continue;
        const stats = scaleStats(numbers);
        each((r, c, value) => {
          if (typeof value !== "number") return;
          const cell = at(r, c);
          if (!cell.style.has("background-color")) cell.style.set("background-color", scaleColor(rule, value, stats));
        });
      }
    }
    return cells;
  }

  const svg = (body) => `url("data:image/svg+xml,${encodeURIComponent(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 14 14">${body}</svg>`)}")`;
  const BOX_ON = svg('<defs><mask id="m"><rect width="14" height="14" rx="3" fill="#fff"/><path d="M3.5 7.2l2.3 2.3 4.7-5" fill="none" stroke="#000" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"/></mask></defs><rect width="14" height="14" rx="3" mask="url(#m)"/>');
  const BOX_OFF = svg('<rect x="0.75" y="0.75" width="12.5" height="12.5" rx="2.5" fill="none" stroke="#000" stroke-width="1.5"/>');
  const CHEVRON = svg('<path d="M3.5 5.5L7 9l3.5-3.5" fill="none" stroke="#5B6577" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"/>');

  // An id inside a CSS string: quotes and backslashes escaped, and line breaks written as
  // escapes, so an id from a file can never end the string.
  const escapeId = (id) => String(id).replace(/["\\]/g, "\\$&").replace(/[\n\r\f]/g, (ch) => `\\${ch.charCodeAt(0).toString(16)} `);

  // The CSS for evaluated cells: each cell is named by its row's id and its column's place
  // (a merge's covered cells stay in the row as hidden tds, so the place is the column).
  function ruleCss(cells, scope, rowIds) {
    const groups = new Map();
    const add = (decl, selector) => {
      const list = groups.get(decl);
      if (list) list.push(selector);
      else groups.set(decl, [selector]);
    };
    for (const [key, cell] of cells) {
      const [r, c] = key.split(",").map(Number);
      const id = rowIds[r];
      if (!id) continue;
      const td = `${scope}>*>tr[data-id="${escapeId(id)}"]>td:nth-child(${c + 1})`;
      if (cell.style.size > 0) add(`${[...cell.style].map(([prop, value]) => `${prop}:${value} !important`).join(";")};${PRINT}`, td);
      if (cell.check !== undefined) {
        add("font-size:0 !important;text-align:center !important", td);
        const mask = cell.check ? BOX_ON : BOX_OFF;
        const colour = cell.check ? "var(--clay-ui-accent, #1D5B79)" : "var(--clay-ui-muted-text, #5B6577)";
        add(`content:"";display:inline-block;vertical-align:middle;width:14px;height:14px;${PRINT};background-color:${colour};-webkit-mask:${mask} center/contain no-repeat;mask:${mask} center/contain no-repeat`, `${td}::before`);
      }
      if (cell.list === "bad") add("box-shadow:inset 0 -2px 0 var(--clay-ui-danger, #A83A2C)", td);
      if (cell.list !== undefined) {
        add(`background-image:${CHEVRON};background-repeat:no-repeat;background-position:right 4px center;background-size:10px 10px;padding-right:18px !important`, `html[editmode="true"] ${td}`);
      }
    }
    return [...groups].map(([decl, selectors]) => `${selectors.join(",")}{${decl}}`).join("");
  }

  const rowIdsOf = (table) => Array.from(table.rows, (tr) => tr.getAttribute("data-id"));

  // Print keeps rule colours and boxes, which browsers drop as backgrounds by default.
  const PRINT = "print-color-adjust:exact;-webkit-print-color-adjust:exact";

  // One stylesheet per tab, named by the tab's scope rather than its table element: a peer's
  // merged save replaces the table, and the new one must repaint the same sheet.
  const sheets = new WeakMap();
  const sheetFor = (doc, scope) => {
    let byScope = sheets.get(doc);
    if (!byScope) sheets.set(doc, (byScope = new Map()));
    return { get: () => byScope.get(scope), set: (sheet) => byScope.set(scope, sheet) };
  };

  // Paints `table`'s rules, or clears them when it has none left. A table that never had
  // rules gets no stylesheet.
  function paintRules(table, grid, doc = table.ownerDocument) {
    if (!canAdopt(doc)) return;
    const rules = grid.rules();
    const scope = scopeOf(table);
    const slot = sheetFor(doc, scope);
    let sheet = slot.get();
    if (!sheet) {
      if (rules.length === 0) return;
      sheet = new doc.defaultView.CSSStyleSheet();
      slot.set(sheet);
      doc.adoptedStyleSheets = [...doc.adoptedStyleSheets, sheet];
    }
    if (rules.length === 0) {
      sheet.replaceSync("");
      return;
    }
    const cells = evaluate(
      rules,
      grid.size,
      (box) => readBox(grid, box),
      (rule) => listItems(table, rule),
      (box) => readTexts(grid, box),
    );
    sheet.replaceSync(ruleCss(cells, scope, rowIdsOf(table)));
  }

  return { parseStyle, holds, scaleColor, scaleStats, evaluate, ruleCss, rowIdsOf, paintRules };
}
