import{b as Z}from"./c-CU64XBWR.js";var ge=`@layer clay-menu-safety {
  .clay-menu-dock [hidden], .clay-menu-spots[hidden], .clay-menu-dock[hidden] { display: none !important; }
  .clay-menu :focus-visible:not(.clay-menu-edit) { outline: 2px solid var(--bevel-brass) !important; outline-offset: -2px !important; }
  @media (pointer: coarse), (max-width: 680px) {
    /* A dismissible row's message and Dismiss sit inline on one line; their reach comes from MENU_TOUCH's pseudo-elements. */
    .clay-menu-goo, .clay-menu-list [part~="step"], .clay-menu-list [role^="menuitem"]:not(.clay-menu-row > *) { min-width: 44px !important; min-height: 44px !important; }
  }
  @media (prefers-reduced-motion: reduce) {
    .clay-menu-dock *, .clay-menu-dock *::before, .clay-menu-dock *::after { transition: none !important; animation: none !important; }
  }
  @media (forced-colors: active) {
    .clay-menu-list { border: 1px solid CanvasText !important; }
    .clay-menu :focus-visible:not(.clay-menu-edit) { outline-color: Highlight !important; }
  }
}
`,we={light:[255,252,246],dark:[26,29,44],mutedLight:[107,89,66],mutedDark:[163,169,194]},se=`.clay-menu-dock, .clay-menu-spots, .clay-seo {
  --bevel-font-sans: var(--clay-menu-font, system-ui, -apple-system, "Segoe UI", sans-serif);
  --bevel-font-serif: Newsreader, Georgia, "Times New Roman", serif;
  --bevel-font-mono: var(--clay-menu-font-mono, ui-monospace, SFMono-Regular, Menlo, Consolas, monospace);
  --bevel-ground: light-dark(#FDF8F0, #0B0C13);
  --bevel-surface: var(--clay-menu-bg, light-dark(#FFFCF6, #1A1D2C));
  --bevel-sunk: var(--clay-menu-hover-bg, light-dark(#F4ECDF, #22263A));
  --bevel-ink: var(--clay-menu-text, light-dark(#2B241B, #ECEAF2));
  --bevel-ink-2: var(--clay-menu-muted, light-dark(#4A4033, #C9CDE0));
  --bevel-muted: var(--clay-menu-muted, light-dark(#6B5942, #A3A9C2));
  --bevel-faint: light-dark(#9A8A70, #6A7090);
  --bevel-line: var(--clay-menu-hr, light-dark(#E8DCC6, #2C3149));
  --bevel-line-2: var(--clay-menu-border, light-dark(#D8C8AF, #3A4058));
  --bevel-line-3: var(--clay-menu-border, light-dark(#C4B192, #4A536F));
  --bevel-brass: var(--clay-menu-accent, light-dark(#8F6310, #D9A445));
  --bevel-brass-soft: light-dark(#F0E4CC, #3A2F17);
  --bevel-ox: light-dark(#A83A2C, #FF6B79);
  --bevel-ox-soft: light-dark(#F6DED6, #3A2226);
  --bevel-teal: light-dark(#256056, #4E9C8C);
  --bevel-teal-soft: light-dark(#DCEBE7, #17332E);
  --bevel-face: light-dark(#F1E7D4, #2A2F48);
  --bevel-edge-hi: light-dark(#FFF9ED, #454C6C);
  --bevel-edge-lo: light-dark(#D6C3A5, #171B2C);
  --bevel-face-in: light-dark(#E3D0B3, #20253B);
  --bevel-edge-in: light-dark(#B89F7B, #12152A);
  --bevel-edge-in-hi: light-dark(#F0E1C9, #4D587A);
  --bevel-knob: light-dark(#FCF8F1, #5A6286);
  --bevel-knob-hi: light-dark(#FFF9ED, #7A83A8);
  --clay-shadow: 0 1px 2px light-dark(rgb(43 36 27 / .07), rgb(0 0 0 / .35)), 0 10px 28px -10px light-dark(rgb(43 36 27 / .32), rgb(0 0 0 / .7));
  --clay-sheet-shadow: 0 16px 30px -22px light-dark(rgb(43 36 27 / .45), rgb(0 0 0 / .85));
  color-scheme: inherit;
  font-family: var(--clay-menu-font, var(--bevel-font-sans));
  color: var(--clay-menu-text, var(--bevel-ink));
  -webkit-text-size-adjust: 100%;
  text-size-adjust: 100%;
}

.clay-menu-dock { display: flex; align-items: center; gap: 6px; font-size: 13px; line-height: 1.4; }
.clay-menu-dock[data-embedded] { position: relative; display: inline-flex; }
.clay-menu-dock[hidden], .clay-menu-spots[hidden], .clay-menu-dock [hidden], .clay-seo [hidden] { display: none !important; }

.clay-menu { box-sizing: border-box; display: inline-flex; align-items: center; gap: 2px; user-select: none; -webkit-user-select: none; }

`,ce=`.clay-menu-goo { position: relative; display: inline-grid; place-items: center; box-sizing: border-box; margin: 0; padding: 0; cursor: pointer; }
.clay-menu-goo svg { display: block; }
.clay-menu-dot { position: absolute; top: -1px; right: -2px; width: 8px; height: 8px; border-radius: 50%; background: var(--bevel-brass); box-shadow: 0 0 0 2px var(--bevel-surface); }

.clay-menu :focus-visible, .clay-roster:focus-visible { outline: 2px solid var(--bevel-brass); outline-offset: 2px; }
.clay-menu-edit:focus-visible { outline: none; }
.clay-menu-edit:focus-visible .clay-menu-track { outline: 2px solid var(--bevel-brass); outline-offset: 2px; }

`,ue=`/* Popovers: the menu, the people panel, the status note. Below the dock, above it in a bottom corner. */
.clay-menu-list, .clay-people, .clay-menu-note { position: absolute; z-index: 1; top: calc(100% + 8px); right: 0; box-sizing: border-box; margin: 0; color: var(--bevel-ink); text-align: left; }
.clay-people, .clay-menu-note { border: 1px solid var(--bevel-line-2); background: var(--bevel-surface); box-shadow: var(--clay-shadow); }

.clay-menu-list { width: max-content; max-width: calc(100vw - 24px); max-height: calc(100dvh - 72px); overflow-y: auto; overscroll-behavior: contain; }
.clay-menu-list[popover] { position: fixed; inset: auto; margin: 0; }
.clay-menu-list [role^="menuitem"] { box-sizing: border-box; display: flex; align-items: center; justify-content: space-between; gap: 12px; width: 100%; margin: 0; border: 0; border-radius: var(--clay-menu-item-radius, 0px); background: none; text-align: left; text-decoration: none; cursor: pointer; }
.clay-menu-list [role^="menuitem"].clay-menu-nested { justify-content: flex-start; gap: 7px; width: calc(100% - 18px); min-height: 29px; margin: 2px 4px 4px 14px; padding: 5px 9px; border: 1px solid var(--clay-menu-hr, var(--bevel-line)); border-radius: max(3px, var(--clay-menu-item-radius, 0px)); background: color-mix(in srgb, var(--clay-menu-hover-bg, var(--bevel-sunk)) 58%, var(--clay-menu-bg, var(--bevel-surface))); color: var(--clay-menu-muted, var(--bevel-ink-2)); }
.clay-menu-list [role^="menuitem"].clay-menu-nested::after { content: "\u203A"; margin-left: auto; font-size: 13px; line-height: 1; opacity: .55; }
.clay-menu-list [role^="menuitem"].clay-menu-nested:hover:not([aria-disabled="true"]), .clay-menu-list [role^="menuitem"].clay-menu-nested:focus-visible { border-color: var(--clay-menu-border, var(--bevel-line-2)); color: var(--clay-menu-text, var(--bevel-ink)); }
.clay-menu-nested-icon { display: block; width: 14px; height: 14px; flex: none; fill: currentColor; opacity: .78; }
.clay-menu-list [role^="menuitem"]:hover:not([aria-disabled="true"]), .clay-menu-list [role^="menuitem"]:focus-visible { outline: none; background: var(--clay-menu-hover-bg, var(--bevel-sunk)); }
.clay-menu-list [role^="menuitem"][aria-disabled="true"] { cursor: default; opacity: var(--clay-menu-disabled, .45); }
.clay-menu-pair { display: flex; flex-wrap: wrap; gap: 12px; padding: 3px 9px; }
.clay-menu-list .clay-menu-pair [role="menuitem"] { flex: none; width: auto; justify-content: flex-start; gap: 9px; }
.clay-menu-list kbd { flex: none; white-space: nowrap; }
/* The message and a small Dismiss pinned cleanly at the right end of one flex line. */
.clay-menu-row { position: relative; display: flex; align-items: center; justify-content: space-between; gap: 8px; padding: 5px 9px; border-radius: var(--clay-menu-item-radius, 0px); }
.clay-menu-row:has(> [part~="item"]:hover, > [part~="item"]:focus-visible) { background: var(--clay-menu-hover-bg, var(--bevel-sunk)); }
.clay-menu-list .clay-menu-row > [part~="item"], .clay-menu-list .clay-menu-row > [part~="item"]:is(:hover, :focus-visible) { display: inline; width: auto; min-height: 0; padding: 0; background: none; }
.clay-menu-list .clay-menu-row > [part~="item"]::before { content: ""; position: absolute; inset: 0; }
.clay-menu-list .clay-menu-row > [part~="dismiss"], .clay-menu-list .clay-menu-row > [part~="dismiss"]:is(:hover, :focus-visible) { position: relative; z-index: 1; display: inline-flex; align-items: center; flex: none; width: auto; min-height: 0; margin-left: 6px; padding: 0; background: none; white-space: nowrap; }
.clay-menu-list .clay-menu-row > [part~="dismiss"]::after { content: ""; position: absolute; inset: -6px -6px; }
.clay-menu-row [part~="dismiss-label"] { display: inline-block; padding: 2px 7px; border-radius: 4px; background: var(--clay-menu-hover-bg, var(--bevel-sunk)); color: var(--clay-menu-muted, var(--bevel-ink-2)); font: 500 11px/1.3 var(--clay-menu-font, var(--bevel-font-sans)); }
.clay-menu-row > [part~="dismiss"]:hover [part~="dismiss-label"], .clay-menu-row > [part~="dismiss"]:focus-visible [part~="dismiss-label"] { color: var(--clay-menu-text, var(--bevel-ink)); }
.clay-menu-upgrade { display: inline-flex; align-items: center; gap: 8px; }
.clay-menu-upgrade::before { content: ""; width: 7px; height: 7px; border-radius: 50%; background: var(--bevel-brass); }
.clay-menu-toggle .clay-menu-track { flex: none; }
.clay-menu-version { box-sizing: border-box; margin: 0; padding: 7px 9px; }

.clay-menu-note { z-index: 2; width: max-content; max-width: 264px; padding: 8px 12px; font-size: 13px; line-height: 1.45; }

.clay-menu-stepper { display: flex; align-items: center; gap: 2px; padding: 0 2px 0 9px; font: 400 13px/1.35 var(--bevel-font-sans); color: var(--bevel-ink); }
.clay-menu-stepper-label { flex: 1; margin-right: 12px; white-space: nowrap; }
.clay-menu-stepper-value { min-width: 5.5em; text-align: center; white-space: nowrap; font-variant-numeric: tabular-nums; }
.clay-menu-list .clay-menu-stepper [role="menuitem"] { flex: none; display: inline-flex; align-items: center; justify-content: center; box-sizing: border-box; width: 36px; min-height: 36px; padding: 0; color: var(--bevel-ink); }
.clay-menu-stepper svg { display: block; width: 16px; height: 16px; fill: currentColor; }

`,ke=se+ce+ue,Ae=`/* The dock: the roster (when a page opts in) and the capsule, fixed in one corner. */
.clay-menu-dock { position: fixed; z-index: 2147482000; top: max(12px, env(safe-area-inset-top)); right: max(12px, env(safe-area-inset-right)); }
.clay-menu-dock[data-corner$="left"] { right: auto; left: max(12px, env(safe-area-inset-left)); flex-direction: row-reverse; }
.clay-menu-dock[data-corner^="bottom"] { top: auto; bottom: max(12px, env(safe-area-inset-bottom)); }
.clay-menu-dock[data-corner$="left"] :is(.clay-menu-list, .clay-people, .clay-menu-note) { right: auto; left: 0; }
.clay-menu-dock[data-corner^="bottom"] :is(.clay-menu-list, .clay-people, .clay-menu-note) { top: auto; bottom: calc(100% + 8px); }
`,Ee=`.clay-menu { height: 34px; padding: 0 4px 0 2px; border: 1px solid var(--clay-menu-border, var(--bevel-line-3)); border-radius: var(--clay-menu-button-radius, 0px); background: var(--clay-menu-bg, var(--bevel-surface)); box-shadow: var(--clay-shadow); }
.clay-menu-goo { width: 30px; height: 30px; min-height: 0; border-radius: var(--clay-menu-button-radius, 0px); border: 2px solid transparent; background: none; color: var(--clay-menu-text, var(--bevel-ink)); }
.clay-menu-goo::before { content: ""; position: absolute; inset: -8px -8px -8px -4px; }
.clay-menu-goo svg { width: 18px; height: 18px; }
.clay-menu-goo svg[fill="currentColor"] { width: 21px; height: auto; fill: currentColor; }
.clay-menu-goo:hover svg { translate: 0 -1px; }
.clay-menu-goo:active, .clay-menu-goo[aria-expanded="true"] { background: var(--clay-menu-hover-bg, var(--bevel-face-in)); border-color: var(--bevel-edge-in) var(--bevel-edge-in-hi) var(--bevel-edge-in-hi) var(--bevel-edge-in); }
.clay-menu-goo:active svg, .clay-menu-goo[aria-expanded="true"] svg { translate: 1px 1px; }
.clay-menu-list { min-width: 284px; padding: 6px; border: 1px solid var(--clay-menu-border, var(--bevel-line-2)); border-radius: var(--clay-menu-radius, 0px); background: var(--clay-menu-bg, var(--bevel-surface)); box-shadow: var(--clay-shadow); }
.clay-menu-list [role^="menuitem"] { min-height: 32px; padding: 5px 9px; font: 400 13px/1.35 var(--clay-menu-font, var(--bevel-font-sans)); color: var(--clay-menu-text, var(--bevel-ink)); }
.clay-menu-list [role^="menuitem"].clay-menu-nested { min-height: 29px; padding: 5px 9px; color: var(--clay-menu-muted, var(--bevel-ink-2)); }
.clay-menu-list .clay-menu-pair [role="menuitem"] { min-height: 26px; padding: 2px 4px 2px 7px; border: 1px solid var(--clay-menu-border, var(--bevel-line)); }
.clay-menu-list kbd { font: 500 11px/1 var(--clay-menu-font-mono, var(--bevel-font-mono)); padding: 3px 5px; border: 1px solid var(--clay-menu-border, var(--bevel-line-2)); background: var(--clay-menu-hover-bg, var(--bevel-sunk)); color: var(--clay-menu-muted, var(--bevel-muted)); }
.clay-menu-list [part~="quiet"] { font-size: 12px; color: var(--clay-menu-muted, var(--bevel-muted)); }
.clay-menu-sep { height: 1px; margin: 4px 2px; background: var(--clay-menu-hr, var(--bevel-line)); }
.clay-menu-version { font: 400 13px/1.35 var(--clay-menu-font, var(--bevel-font-sans)); color: var(--clay-menu-text, var(--bevel-ink)); }
`,Ce=`@media (pointer: coarse), (max-width: 680px) {
  /* A phone menu fits the screen and never scrolls sideways: a little wider than on a desktop, long
     labels wrap, and the shortcut keys a phone cannot press are hidden (David, Nov 11). The floating
     theme pill folds into the menu's Theme: row so the dock never overflows the viewport. */
  .clay-variants { display: none !important; }
  .clay-menu-list { min-width: min(300px, calc(100vw - 16px)) !important; max-width: calc(100vw - 16px) !important; overflow-x: hidden !important; }
  .clay-menu-list [role^="menuitem"], .clay-menu-list .clay-menu-version { white-space: normal; overflow-wrap: anywhere; }
  .clay-menu-list kbd { display: none !important; }
  .clay-menu-list .clay-menu-row { box-sizing: border-box; min-height: 44px; padding: 11px 9px; }
  /* Dismiss sits inline after the message's last word, so its reach never goes up or left into the
     message's own text: a taller label plus reach to the right and down makes the 44px. */
  .clay-menu-row [part~="dismiss-label"] { padding: 7px 12px; font-size: 13px; }
  .clay-menu-list .clay-menu-row > [part~="dismiss"]::after { inset: -2px -10px -11px 0; }
  .clay-menu-list [role^="menuitem"] { min-height: 44px; font-size: 15px; }
  .clay-menu-list .clay-menu-version { display: flex; align-items: center; min-height: 44px; font-size: 15px; }
  .clay-menu-list .clay-menu-pair [role="menuitem"] { min-height: 44px; }
  .clay-menu-list .clay-menu-stepper [role="menuitem"] { width: 44px; min-height: 44px; }
  :is(.clay-menu-dock, .clay-seo) :is(input, textarea, select) { font-size: 16px; }
  .clay-seo-input, .clay-people-input { min-height: 40px; }
  .clay-menu-btn { min-height: 40px; }
}
@media (prefers-reduced-motion: reduce) {
  .clay-menu-track, .clay-menu-thumb { transition: none; }
}
`;var G='<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linejoin="round" aria-hidden="true" focusable="false"><path d="M12 9a3 3 0 1 1 0 6 3 3 0 0 1 0-6M10 3h4l1.5 3 3.5-.5 2 4-2 2.5 2 2.5-2 4-3.5-.5-1.5 3h-4l-1.5-3-3.5.5-2-4 2-2.5-2-2.5 2-4 3.5.5Z"/></svg>';var W="1",w=8,de="Dismiss",pe='<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 256 256" aria-hidden="true" focusable="false"><path d="M224,128a8,8,0,0,1-8,8H59.31l58.35,58.34a8,8,0,0,1-11.32,11.32l-72-72a8,8,0,0,1,0-11.32l72-72a8,8,0,0,1,11.32,11.32L59.31,120H216A8,8,0,0,1,224,128Z"/></svg>',me='<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 256 256" aria-hidden="true" focusable="false"><path d="M221.66,133.66l-72,72a8,8,0,0,1-11.32-11.32L196.69,136H40a8,8,0,0,1,0-16H196.69L138.34,61.66a8,8,0,0,1,11.32-11.32l72,72A8,8,0,0,1,221.66,133.66Z"/></svg>',be='<svg xmlns="http://www.w3.org/2000/svg" class="clay-menu-nested-icon" viewBox="0 0 256 256" fill="currentColor" aria-hidden="true" focusable="false"><path d="M213.66,66.34l-40-40A8,8,0,0,0,168,24H88A16,16,0,0,0,72,40V56H56A16,16,0,0,0,40,72V216a16,16,0,0,0,16,16H168a16,16,0,0,0,16-16V200h16a16,16,0,0,0,16-16V72A8,8,0,0,0,213.66,66.34ZM168,216H56V72h76.69L168,107.31v84.53c0,.06,0,.11,0,.16s0,.1,0,.16V216Zm32-32H184V104a8,8,0,0,0-2.34-5.66l-40-40A8,8,0,0,0,136,56H88V40h76.69L200,75.31Zm-56-32a8,8,0,0,1-8,8H88a8,8,0,0,1,0-16h48A8,8,0,0,1,144,152Zm0,32a8,8,0,0,1-8,8H88a8,8,0,0,1,0-16h48A8,8,0,0,1,144,184Z"/></svg>',ye='<span class="clay-menu-track" aria-hidden="true"><span class="clay-menu-thumb"></span></span>',ve=6e3;function xe(){let v=document.querySelectorAll("[clay-menu-slot]");return v.length>1&&console.warn("[clay] This page has more than one clay-menu-slot. The menu uses the first."),v[0]||null}function fe(){let v=document.querySelector("style[clay-menu-skin]");return v?v.getAttribute("clay-menu-skin")!==W?(console.warn(`[clay] This runtime reads clay-menu-skin="${W}". The menu keeps its default look.`),null):v.textContent:null}function Be({capsuleStart:v="",sections:I=[null],icon:K=G,label:z="Clay menu"}={}){let X=new Set(I.flat().filter(Boolean)),x=xe(),h=!!x,Y=h?fe():null,c=document.createElement("div");c.className="clay-menu-dock",c.setAttribute("clay","editor-ui"),c.setAttribute("part","dock"),h&&(c.dataset.embedded=""),c.dataset.corner="top-right",c.innerHTML='<div class="clay-menu" part="capsule" clay="editor-ui">'+v+`<button type="button" class="clay-menu-goo" part="button" aria-haspopup="menu" aria-expanded="false" aria-label="${z}">${K}</button></div><div class="clay-menu-list" part="list" role="menu" aria-label="${z}" hidden></div><div class="clay-menu-note" part="note" role="status" hidden></div>`;let d=c.querySelector(".clay-menu-goo"),i=c.querySelector(".clay-menu-list"),k=c.querySelector(".clay-menu-note"),p=new Map,_=[];if(h){let e=x.querySelector("template[clay-menu-icon]");e&&d.replaceChildren(e.content.cloneNode(!0));let n=x.getAttribute("clay-menu-label");n&&(d.setAttribute("aria-label",n),d.title=n,i.setAttribute("aria-label",n))}let F=h&&typeof i.showPopover=="function";F&&i.setAttribute("popover","manual");function q(){let e=d.getBoundingClientRect(),{offsetWidth:n,offsetHeight:t}=i,o=Math.max(w,Math.min(e.right-n,window.innerWidth-n-w)),a=e.bottom+w,l=a+t>window.innerHeight-w?Math.max(w,e.top-t-w):a;i.style.left=`${Math.round(o)}px`,i.style.top=`${Math.round(l)}px`}let D=()=>c.getRootNode().activeElement||null,m=()=>!i.hidden;function R(e){i.hidden=!e,d.setAttribute("aria-expanded",e?"true":"false"),F&&(e?(i.showPopover(),q()):i.matches(":popover-open")&&i.hidePopover())}let B=()=>[...i.querySelectorAll('[role^="menuitem"]:not([aria-disabled="true"])')];function A(e){let n=B();n.length&&n[(e+n.length)%n.length].focus()}function f(){m()&&R(!1)}function N(){if(!m()){for(let e of _)e();E(),R(!0)}}function T(e){let n=e.href&&!e.disabled,t=document.createElement(n?"a":"button");n?(t.href=e.href,t.target="_blank",t.rel="noopener"):t.type="button";let o=["item"];if(e.quiet&&o.push("quiet"),e.nested&&o.push("nested"),t.setAttribute("role","menuitem"),t.setAttribute("part",o.join(" ")),e.nested&&t.classList.add("clay-menu-nested"),t.dataset.menuId=e.id,e.badge&&e.id==="upgrade"){let a=document.createElement("span");a.className="clay-menu-upgrade",a.textContent=e.label,t.append(a)}else if(e.nested){t.insertAdjacentHTML("beforeend",be);let a=document.createElement("span");a.textContent=e.label,t.append(a)}else t.textContent=e.label;if(e.keys){let a=document.createElement("kbd");a.textContent=e.keys,t.append(a)}return e.toggle&&(t.setAttribute("role","menuitemcheckbox"),t.setAttribute("aria-checked",e.checked?"true":"false"),t.classList.add("clay-menu-toggle"),t.insertAdjacentHTML("beforeend",ye)),e.disabled&&t.setAttribute("aria-disabled","true"),t.addEventListener("click",()=>{e.disabled||(!e.toggle&&!e.keepOpen&&f(),e.run&&e.run({anchor:d}))}),t}function J(e){if(e.text){let n=document.createElement("p");return n.className="clay-menu-version",n.setAttribute("part","text"),n.dataset.menuId=e.id,n.textContent=e.label,n}if(e.pair){let n=document.createElement("div");n.className="clay-menu-pair",n.dataset.menuId=e.id;for(let t of e.pair)n.append(T(t));return n}return e.stepper?ee(e):e.dismiss?Q(e):T(e)}function Q(e){let n=document.createElement("div");n.className="clay-menu-row",n.dataset.menuId=e.id;let t=document.createElement("button");t.type="button",t.setAttribute("role","menuitem"),t.setAttribute("part","dismiss"),t.dataset.menuId=`${e.id}.dismiss`,t.title=e.dismiss.label;let o=document.createElement("span");return o.setAttribute("part","dismiss-label"),o.textContent=de,t.append(o),t.addEventListener("click",async()=>{t.disabled=!0;let a;try{a=await e.dismiss.run?.()}catch(l){console.warn("[clay] A menu item could not be removed.",l),t.disabled=!1,$("That didn't work. Try again.");return}if(a===!1){t.disabled=!1;return}U(e.id)}),n.append(T(e),t),n}function ee(e){let{label:n,value:t,prevLabel:o,nextLabel:a}=e.stepper,l=document.createElement("div");l.className="clay-menu-stepper",l.setAttribute("part","stepper"),l.dataset.menuId=e.id;let r=document.createElement("span");r.className="clay-menu-stepper-label",r.textContent=n;let b=document.createElement("span");b.className="clay-menu-stepper-value",b.setAttribute("aria-live","polite"),b.textContent=t;let C=(g,H,M)=>{let u=document.createElement("button");return u.type="button",u.setAttribute("role","menuitem"),u.setAttribute("part","step"),u.setAttribute("aria-label",M),u.dataset.menuId=`${e.id}.${g}`,u.innerHTML=H,u.addEventListener("click",()=>p.get(e.id)?.stepper?.[g]?.()),u};return l.append(r,C("prev",pe,o),b,C("next",me,a)),l}function O(e,n){let t=e.replace(/^(?:clay|dom)\./,"");return n.has(e)||n.has(t)||e==="dom.appreciation"&&n.has("thanks")||e==="clay.previous"&&(n.has("new")||n.has("duplicate"))}function te(e){let n=(x?.getAttribute("clay-menu-allow")??document.documentElement?.getAttribute("clay-menu-allow")??"").trim();if(n){let o=new Set(n.split(/[\s,]+/).filter(Boolean));return O(e,o)}let t=(x?.getAttribute("clay-menu-deny")??document.documentElement?.getAttribute("clay-menu-deny")??"").trim();if(t){let o=new Set(t.split(/[\s,]+/).filter(Boolean));return!O(e,o)}return!0}function ne(){let e=l=>l.filter(r=>r&&te(r.id)),n=l=>e([...p.values()].filter(r=>r.section===l)),t=[...p.keys()].filter(l=>!X.has(l)&&!p.get(l).section),o=I.map(l=>e((l||t).map(r=>p.get(r)))),a=o.length>2?1:0;return o[a]=[...o[a]||[],...n("start")],[...o,n("end")].filter(l=>l.length)}function E(){let e=D(),n=e&&i.contains(e)?e.dataset.menuId:null;i.textContent="",ne().forEach((a,l)=>{if(l){let r=document.createElement("div");r.className="clay-menu-sep",r.setAttribute("role","separator"),r.setAttribute("part","separator"),i.append(r)}for(let r of a)i.append(J(r))});let t=d.querySelector(".clay-menu-dot"),o=[...p.values()].some(a=>a.badge);if(o&&!t){let a=document.createElement("span");a.className="clay-menu-dot",a.setAttribute("aria-hidden","true"),d.append(a)}else!o&&t&&t.remove();if(n){let a=B().find(l=>l.dataset.menuId===n);a?a.focus():A(0)}}let j=({id:e,label:n,run:t,badge:o,keys:a,disabled:l,href:r,keepOpen:b})=>({id:e,href:typeof r=="string"?r:"",label:String(n??""),run:typeof t=="function"?t:null,badge:!!o,keys:a?String(a):"",disabled:!!l,keepOpen:!!b});function ae({id:e,label:n,run:t,badge:o,keys:a,disabled:l,text:r,pair:b,href:C,section:g,quiet:H,nested:M,dismiss:u,toggle:oe,checked:re,keepOpen:ie,stepper:y}={}){if(!e)return null;let s=j({id:e,label:n,run:t,badge:o,keys:a,disabled:l,href:C,keepOpen:ie});if((g==="start"||g==="end")&&(s.section=g),H&&(s.quiet=!0),M&&(s.nested=!0),y){s.stepper={label:String(y.label??""),value:String(y.value??""),prev:typeof y.prev=="function"?y.prev:null,next:typeof y.next=="function"?y.next:null,prevLabel:String(y.prevLabel||"Previous"),nextLabel:String(y.nextLabel||"Next")};let P=[...i.querySelectorAll(".clay-menu-stepper")].find(S=>S.dataset.menuId===e)?.querySelector(".clay-menu-stepper-value"),L=p.get(e);if(L?.stepper&&["label","prevLabel","nextLabel"].every(S=>L.stepper[S]===s.stepper[S])&&L.section===s.section&&L.quiet===s.quiet&&P)return p.set(e,s),P.textContent=s.stepper.value,e}return r&&(s.text=!0),Array.isArray(b)&&(s.pair=b.map(j)),oe&&(s.toggle=!0,s.checked=!!re),u&&(s.dismiss={label:String(u.label||"Dismiss"),run:typeof u.run=="function"?u.run:null}),p.set(e,s),E(),e}function U(e){p.delete(e)&&E()}function $(e){e&&(k.textContent=e,k.hidden=!1,clearTimeout(V),V=setTimeout(()=>{k.hidden=!0,k.textContent=""},ve))}let V=0;d.addEventListener("click",()=>{m()?f():N()}),document.addEventListener("click",e=>{m()&&!e.composedPath().includes(c)&&f()}),c.addEventListener("keydown",e=>{if(e.key==="Escape"){if(!m())return;e.preventDefault(),f(),d.focus();return}if(e.target===d&&(e.key==="Enter"||e.key===" ")){e.preventDefault(),m()?f():(N(),A(0));return}if(!m())return;if(e.key==="Enter"&&i.contains(e.target)){e.preventDefault(),e.target.click();return}let n=B();if(!n.length)return;let t=n.indexOf(D());e.key==="ArrowDown"?(e.preventDefault(),A(t<0?0:t+1)):e.key==="ArrowUp"&&(e.preventDefault(),A(t<0?n.length-1:t-1))});function le(e){let n=Z({name:"menu",content:c,css:e,parent:x||document.body,attributes:{"clay-mode-toggle":""}});return h&&(window.addEventListener("resize",f),window.addEventListener("scroll",()=>{F&&m()&&q()},{capture:!0,passive:!0})),n}return{root:c,goo:d,list:i,slot:x,embedded:h,skin:Y,items:p,addItem:ae,removeItem:U,render:E,open:N,close:f,isOpen:m,note:$,beforeOpen:_,focusedEl:D,mount:le}}export{ge as a,we as b,se as c,ce as d,ue as e,ke as f,Ae as g,Ee as h,Ce as i,ye as j,fe as k,Be as l};
