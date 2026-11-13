import{a as u,b as m,c as b,d as k}from"./c-Y6F2FORL.js";import{b as y}from"./c-QY7F6EN6.js";import{d as v}from"./c-PKCCHJ5X.js";import{b as g}from"./c-EDE45NM7.js";import"./c-BPF2NSPV.js";import"./c-P7KNDG34.js";import"./c-CYWLPPUS.js";import"./c-U4NTUVDZ.js";import"./c-HA6INT6J.js";import"./c-ICMAOOGK.js";import"./c-WRBKEJD4.js";import"./c-VHNSBD2M.js";var A=.8,c={draft:"You're seeing a private draft, like a visitor.",draftAction:"Edit the original",edit:"You're editing the original. Visitors get these changes.",editAction:"Back to my draft",close:"Hide this note"},L=`.clay-owner {
  --hcp-surface: light-dark(#FFFCF6, #11131E);
  --hcp-ground: var(--bevel-surface, var(--hcp-surface));
  --hcp-ink: light-dark(#2B241B, #ECEAF2);
  --hcp-ink-2: light-dark(#4A4033, #C9CDE0);
  --hcp-muted: light-dark(#6B5942, #8A90AB);
  --hcp-line-2: color-mix(in srgb, var(--hcp-ink) 22%, var(--hcp-ground));
  --hcp-brass: light-dark(#8F6310, #D9A445);
  --hcp-shadow: 0 1px 2px light-dark(rgba(23, 28, 36, .06), rgba(0, 0, 0, .4)), 0 14px 34px -14px light-dark(rgba(43, 36, 27, .32), rgba(0, 0, 0, .7));
  --hcp-sans: "IBM Plex Sans", system-ui, -apple-system, "Segoe UI", sans-serif;
  --hcp-lift: 0px;
  color-scheme: light dark;
}
.clay-owner, .clay-owner *, .clay-owner *::before, .clay-owner *::after {
  box-sizing: border-box; margin: 0; padding: 0; border: 0;
  font: inherit; letter-spacing: normal; text-transform: none; text-shadow: none;
  line-height: inherit; color: inherit; background: none; float: none; max-width: none;
}
.clay-owner {
  position: fixed; left: 12px; bottom: calc(12px + var(--hcp-lift)); z-index: 2147482000;
  display: flex; align-items: center; gap: 10px;
  max-width: min(460px, calc(100vw - 24px));
  padding: 8px 8px 8px 12px;
  font-family: var(--hcp-sans); font-size: 13px; font-weight: 400; line-height: 1.45;
  color: var(--hcp-ink-2); text-align: left; background: var(--hcp-ground);
  border: 1px solid var(--hcp-line-2);
  box-shadow: var(--hcp-shadow);
  -webkit-font-smoothing: antialiased;
}
.clay-owner__action {
  flex: none; min-height: 0; font-weight: 600; color: var(--hcp-brass); cursor: pointer;
  text-decoration: underline; text-underline-offset: 2px; text-decoration-color: var(--hcp-line-2);
  white-space: nowrap;
}
.clay-owner .clay-owner__action:hover { color: var(--hcp-ink); background: none; }
.clay-owner__x {
  flex: none; display: inline-flex; align-items: center; justify-content: center;
  width: 24px; height: 24px; min-height: 0; font-size: 15px; color: var(--hcp-muted); cursor: pointer;
}
.clay-owner .clay-owner__x:hover { color: var(--hcp-ink); background: none; }
.clay-owner__action:focus-visible, .clay-owner__x:focus-visible { outline: 2px solid var(--hcp-brass); outline-offset: 2px; }

.clay-owner svg {
  display: block; flex: none; width: 1em; height: 1em; line-height: 1;
  fill: currentColor; stroke: none; stroke-width: 0; stroke-linecap: butt; stroke-linejoin: miter;
}
.clay-owner button, .clay-owner a { border-radius: 0; line-height: 1.45; }
@media (pointer: coarse) {
  .clay-owner__action, .clay-owner__x { min-height: 44px; }
  .clay-owner__x { width: 44px; }
}
@media (prefers-reduced-motion: reduce) { .clay-owner { transition: none; } }
@media (forced-colors: active) { .clay-owner { border: 1px solid CanvasText; } }
@media print { .clay-owner { display: none; } }
`,B=()=>document.querySelector('clay-editor-ui[data-clay-ui="owner"]')?.shadowRoot.getElementById("clay-owner")??null;function D(){if(B())return null;let l=g,e=document.createElement("div");e.id="clay-owner",e.className="clay-owner",e.setAttribute("clay","editor-ui"),e.setAttribute("sap-ignore",""),e.setAttribute("role","status");let s=document.createElement("span");s.className="clay-owner__lead",s.textContent=l?c.draft:c.edit;let a=document.createElement("button");a.type="button",a.className="clay-owner__action",a.textContent=l?c.draftAction:c.editAction,a.addEventListener("click",()=>{let n=window.clay&&window.clay.visitorDrafts;n&&(l?n.editOriginal():n.backToDraft())});let r=document.createElement("button");r.type="button",r.className="clay-owner__x",r.setAttribute("aria-label",c.close),r.textContent="\xD7",r.addEventListener("click",()=>{f.remove(),p(),_()}),e.append(s,a,r);let f=y({name:"owner",content:e,css:[L],attributes:{"sap-ignore":""}}),i=document.createElement("style");i.id="clay-owner-vars",i.setAttribute("clay","editor-ui"),document.head.appendChild(i);let x="";function E(n){let o=`:root { --clay-owner-room: ${Math.ceil(e.getBoundingClientRect().height)+16}px; }`;i.textContent!==o&&(i.textContent=o),x!==o+n&&(x=o+n,m())}function _(){i.remove(),m()}let d=0;function t(){d||(d=requestAnimationFrame(()=>{d=0;let n=b(24,w=>e.contains(w)||w.closest("#hcp")),o=k(e.getBoundingClientRect()),h=o>=window.innerHeight/3;o&&!h&&(n=Math.max(n,o-4)),e.style.visibility=h?"hidden":"",e.style.setProperty("--hcp-lift",n+"px"),E(h?-1:n)}))}function p(){C?.(),removeEventListener("scroll",t),removeEventListener("resize",t),document.removeEventListener(u,t),document.removeEventListener("transitionend",t,!0),document.removeEventListener("pointerup",t,!0),window.removeEventListener("hashchange",t)}addEventListener("scroll",t,{passive:!0}),addEventListener("resize",t),document.addEventListener(u,t),document.addEventListener("transitionend",t,!0),document.addEventListener("pointerup",t,!0),window.addEventListener("hashchange",t),e.addEventListener("transitionend",t),t();let C=v(e,e,void 0,A);return e.liftOff=p,e.unmount=()=>{p(),f.remove()},e}export{c as COPY,D as mountOwnerChip,B as ownerChip};
