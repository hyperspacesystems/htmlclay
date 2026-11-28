import { BOTTOM_UI_CHANGED, announceBottomUi, bottomBarHeight } from "./bottom-ui.js";
import { mountEditorUI } from "../../ui/editor-ui.js";

const ASIDE = `<aside class="hcp" id="hcp" hidden aria-label="Download this app" data-hcp-state="off" data-hcp-reason="error" sap-ignore clay="editor-ui">
  <section class="hcp-card" id="hcp-card" role="dialog" aria-modal="false" aria-labelledby="hcp-title">
    <button type="button" class="hcp-x" data-hcp="minimize" aria-label="Minimize"><svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="square" aria-hidden="true"><path d="M5 12h14"/></svg></button>
    <div class="hcp-kicker">Browser storage unavailable</div>
    <h2 class="hcp-title" id="hcp-title"><span data-hcp-for="error">Your work isn't being saved in this browser.</span></h2>
    <p class="hcp-body"><span data-hcp-for="error">Storage is full or turned off here. Download the file to keep what you've done.</span></p>
    <div class="hcp-file">
      <svg width="20" height="20" viewBox="0 0 256 256" fill="currentColor" aria-hidden="true"><path d="M224,144v64a8,8,0,0,1-8,8H40a8,8,0,0,1-8-8V144a8,8,0,0,1,16,0v56H208V144a8,8,0,0,1,16,0Zm-101.66,5.66a8,8,0,0,0,11.32,0l40-40a8,8,0,0,0-11.32-11.32L136,124.69V32a8,8,0,0,0-16,0v92.69L93.66,98.34a8,8,0,0,0-11.32,11.32Z"/></svg>
      <div><b class="hcp-name" data-hcp-name>This app</b><small>One file with the whole app and your work in it</small></div>
    </div>
    <div class="hcp-actions">
      <a class="hcp-btn hcp-btn--primary" data-hcp-link href="/_/download?as=htmlclay"><span><svg width="16" height="16" viewBox="0 0 256 256" fill="currentColor" aria-hidden="true"><path d="M224,144v64a8,8,0,0,1-8,8H40a8,8,0,0,1-8-8V144a8,8,0,0,1,16,0v56H208V144a8,8,0,0,1,16,0Zm-101.66,5.66a8,8,0,0,0,11.32,0l40-40a8,8,0,0,0-11.32-11.32L136,124.69V32a8,8,0,0,0-16,0v92.69L93.66,98.34a8,8,0,0,0-11.32,11.32Z"/></svg>Download free</span></a>
    </div>
    <div class="hcp-foot"><button type="button" class="hcp-quiet" data-hcp="dismiss">Minimize</button></div>
  </section>
  <div class="hcp-dock">
    <button type="button" class="hcp-btn hcp-tab" data-hcp="open" aria-controls="hcp-card" aria-expanded="false" aria-label="Storage warning: download your work"><span><svg width="16" height="16" viewBox="0 0 256 256" fill="currentColor" aria-hidden="true"><path d="M224,144v64a8,8,0,0,1-8,8H40a8,8,0,0,1-8-8V144a8,8,0,0,1,16,0v56H208V144a8,8,0,0,1,16,0Zm-101.66,5.66a8,8,0,0,0,11.32,0l40-40a8,8,0,0,0-11.32-11.32L136,124.69V32a8,8,0,0,0-16,0v92.69L93.66,98.34a8,8,0,0,0-11.32,11.32Z"/></svg><span class="hcp-tab__label">Save copy</span><span class="hcp-tab__mini">Keep</span></span></button>
    <button type="button" class="hcp-x" data-hcp="minimize" aria-label="Minimize storage warning"><svg width="12" height="12" viewBox="0 0 12 12" fill="currentColor" aria-hidden="true"><path d="M1.4 0 6 4.6 10.6 0 12 1.4 7.4 6 12 10.6 10.6 12 6 7.4 1.4 12 0 10.6 4.6 6 0 1.4z"/></svg></button>
  </div>
</aside>`;

// Lazily mounted only when browser draft storage fails (`clay:drafts-store-failed` or an
// interrupted download flush), so visitors get an emergency rescue download card without any
// resting bottom-right popover during normal use.
export async function mountPrompt({ memory, appName, link, settle, canDownloadCurrent = () => true }) {
  const { PROMPT_CSS } = await import("./prompt-style.js");
  const existing = document.querySelector('clay-editor-ui[data-clay-ui="download-prompt"]');
  let root = existing?.shadowRoot.getElementById("hcp");
  if (root?._hcpApi) return root._hcpApi;
  if (!root) {
    const parsed = document.createElement("template");
    parsed.innerHTML = ASIDE;
    root = parsed.content.firstElementChild;
    mountEditorUI({ name: "download-prompt", content: root, css: [PROMPT_CSS], attributes: { "sap-ignore": "" } });
  }
  const ui = memory.get();
  if (!ui.prompt || typeof ui.prompt !== "object") ui.prompt = {};
  const mem = ui.prompt;
  const TEXTY = /^(INPUT|TEXTAREA|SELECT)$/;
  const OUR_UI = "#hcp, .clay-intro, .clay-owner";

  const linkEl = root.querySelector("[data-hcp-link]");
  linkEl.href = link;
  if (appName) root.querySelector("[data-hcp-name]").textContent = appName;

  const tab = root.querySelector("[data-hcp=open]");
  function set(state, reason) {
    if (state === "off") {
      root.hidden = true;
      root.setAttribute("data-hcp-state", "off");
      if (reason) root.setAttribute("data-hcp-reason", reason);
      tab.setAttribute("aria-expanded", "false");
      lift();
      return;
    }
    root.hidden = false;
    root.setAttribute("data-hcp-state", state);
    if (reason) root.setAttribute("data-hcp-reason", reason);
    tab.setAttribute("aria-expanded", state === "card" ? "true" : "false");
    root.setAttribute("data-hcp-arrived", "");
    lift();
  }

  function isHostField(el) {
    return el && !root.contains(el) && !el.closest(OUR_UI) && (TEXTY.test(el.tagName) || el.isContentEditable);
  }

  function onFocusIn(e) {
    if (!isHostField(e.target) || root.hidden) return;
    if (root.getAttribute("data-hcp-state") === "card") set("mini");
    if (innerWidth < 700) root.setAttribute("data-hcp-typing", "");
  }
  function onFocusOut() {
    setTimeout(() => { if (!isHostField(document.activeElement)) root.removeAttribute("data-hcp-typing"); }, 50);
  }

  let liftFrame = 0;
  let announcing = false;
  function lift() {
    if (liftFrame) return;
    liftFrame = requestAnimationFrame(() => {
      liftFrame = 0;
      const raise = bottomBarHeight(innerWidth - 24, (el) => root.contains(el) || el.closest("[data-hcp-preview], .clay-owner"));
      const next = raise + "px";
      if (root.style.getPropertyValue("--hcp-lift") !== next) root.style.setProperty("--hcp-lift", next);
      announcing = true;
      announceBottomUi();
      announcing = false;
    });
  }

  async function downloadCurrent() {
    const getHTML = window.clay && window.clay.getHTML;
    if (typeof getHTML !== "function" || typeof URL === "undefined" || typeof URL.createObjectURL !== "function") return false;
    const segment = location.pathname.split("/").filter(Boolean).pop() || "index.html";
    try {
      const { downloadHtml } = await import("../../core/download.js");
      return await downloadHtml({ html: getHTML(), name: segment.replace(/\.html$/, ".htmlclay") });
    } catch {
      return false;
    }
  }

  let rescueRequested = false;
  root.addEventListener("click", async (e) => {
    const t = e.target.closest("[data-hcp]");
    if (t) {
      const act = t.getAttribute("data-hcp");
      if (act === "open") { set("card", "error"); linkEl.focus(); }
      else if (act === "minimize" || act === "dismiss") { set("mini"); tab.focus(); }
      return;
    }
    if (e.target.closest("[data-hcp-link]")) {
      if (!canDownloadCurrent()) return;
      let rescue = rescueRequested || root.getAttribute("data-hcp-reason") === "error";
      if (!rescue) {
        const waiting = settle?.();
        if (waiting) {
          e.preventDefault();
          if (await waiting !== false) {
            linkEl.click();
            return;
          }
          rescue = true;
        }
      }
      if (rescue) {
        e.preventDefault();
        if (!(await downloadCurrent())) {
          showError();
          root.querySelector(".hcp-body [data-hcp-for='error']").textContent = "The download could not be made. Keep this page open and try Download free again.";
          return;
        }
      }
      mem.downloaded = true;
      memory.save();
    }
  });
  root.addEventListener("keydown", (e) => {
    if (e.key === "Escape" && root.getAttribute("data-hcp-state") === "card") { set("mini"); tab.focus(); }
  });

  function showError() {
    root.querySelector(".hcp-body [data-hcp-for='error']").textContent = "Storage is full or turned off here. Download the file to keep what you've done.";
    set("card", "error");
  }
  function clearError() {
    if (root.getAttribute("data-hcp-reason") === "error") set("off", "saved");
  }
  const api = {
    set,
    root,
    showError,
    clearError,
    download: (rescue = false) => {
      rescueRequested = rescue;
      try { linkEl.click(); } finally { rescueRequested = false; }
    },
  };

  document.addEventListener("focusin", onFocusIn);
  document.addEventListener("focusout", onFocusOut);
  addEventListener("scroll", lift, { passive: true });
  addEventListener("resize", lift);
  document.addEventListener(BOTTOM_UI_CHANGED, () => { if (!announcing) lift(); });
  document.addEventListener("transitionend", lift, true);
  root.addEventListener("transitionend", lift);
  root._hcpApi = api;

  return api;
}
