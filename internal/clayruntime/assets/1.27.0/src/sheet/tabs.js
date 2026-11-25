const canAdopt = (doc = document) =>
  "adoptedStyleSheets" in doc && typeof doc.defaultView?.CSSStyleSheet?.prototype?.replaceSync === "function";

const childOf = (el, test) => {
  for (let node = el?.firstElementChild ?? null; node !== null; node = node.nextElementSibling) {
    if (test(node)) return node;
  }
  return null;
};

const escapeAttr = (value) => String(value).replace(/["\\]/g, "\\$&");

export function readTabs(workbook) {
  const list = [];
  for (let node = workbook?.firstElementChild ?? null; node !== null; node = node.nextElementSibling) {
    if (node.localName !== "section" || !node.hasAttribute("item")) continue;
    const id = node.getAttribute("data-id") ?? "";
    const pos = node.getAttribute("data-pos") ?? "";
    const heading = childOf(node, (el) => el.localName === "h2" && el.getAttribute("bind") === "name");
    const table = childOf(node, (el) => el.localName === "table" && el.hasAttribute("grid"));
    list.push({ id, name: heading?.textContent ?? "", pos, section: node, table });
  }
  return list;
}

export function tabRuleText(id, scope = "[sheet-workbook]") {
  const hideOthers = id ? `${scope} > section[item]:not([data-id="${escapeAttr(id)}"]) { display: none }` : "";
  return `@media screen { ${hideOthers} ${scope} > section[item] > h2[bind="name"] { display: none } ${scope} > section[item] > table[grid] { -webkit-touch-callout: none; -webkit-user-select: none; user-select: none; touch-action: manipulation } }`;
}

export function createTabs(workbook, { sheet: customSheet = null, adopt: customAdopt = null, drop: customDrop = null } = {}) {
  const doc = workbook.ownerDocument ?? document;
  const scope = "[sheet-workbook]";
  const adoptable = Boolean(customSheet || customAdopt || canAdopt(doc));
  const sheet = customSheet ?? (adoptable ? new doc.defaultView.CSSStyleSheet() : null);
  let fallbackStyle = null;
  let activeId = "";
  let disposed = false;

  const adopt = customAdopt ?? ((s) => {
    doc.adoptedStyleSheets = [...doc.adoptedStyleSheets, s];
  });
  const drop = customDrop ?? ((s) => {
    doc.adoptedStyleSheets = doc.adoptedStyleSheets.filter((one) => one !== s);
  });

  if (sheet) {
    adopt(sheet);
  } else {
    fallbackStyle = doc.createElement("style");
    fallbackStyle.setAttribute("clay", "editor-ui");
    fallbackStyle.setAttribute("sap-ignore", "");
    (doc.head ?? doc.documentElement).append(fallbackStyle);
  }

  function applyRule(id) {
    if (disposed) return;
    activeId = id;
    const css = tabRuleText(id, scope);
    if (sheet) sheet.replaceSync(css);
    else if (fallbackStyle && fallbackStyle.textContent !== css) fallbackStyle.textContent = css;
  }

  const initial = readTabs(workbook)[0]?.id ?? "";
  applyRule(initial);

  return {
    get activeId() {
      return activeId;
    },
    get sheet() {
      return sheet;
    },
    list: () => readTabs(workbook),
    show(id) {
      if (id !== activeId) applyRule(id);
    },
    refresh() {
      const tabs = readTabs(workbook);
      if (tabs.some((tab) => tab.id === activeId)) return activeId;
      const next = tabs[0]?.id ?? "";
      applyRule(next);
      return next;
    },
    dispose() {
      if (disposed) return;
      disposed = true;
      if (sheet) drop(sheet);
      fallbackStyle?.remove();
    },
  };
}
