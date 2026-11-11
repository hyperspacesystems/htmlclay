import{b as Z}from"./c-QY7F6EN6.js";var he=`@layer clay-menu-safety {
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
`,ie=`.clay-menu-dock, .clay-menu-spots, .clay-seo {
  --bevel-font-sans: system-ui, -apple-system, "Segoe UI", sans-serif;
  --bevel-font-serif: Newsreader, Georgia, "Times New Roman", serif;
  --bevel-font-mono: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace;
  --bevel-ground: light-dark(#FDF8F0, #0B0C13);
  --bevel-surface: light-dark(#FFFCF6, #1A1D2C);
  --bevel-sunk: light-dark(#F4ECDF, #22263A);
  --bevel-ink: light-dark(#2B241B, #ECEAF2);
  --bevel-ink-2: light-dark(#4A4033, #C9CDE0);
  --bevel-muted: light-dark(#6B5942, #A3A9C2);
  --bevel-faint: light-dark(#9A8A70, #6A7090);
  --bevel-line: light-dark(#E8DCC6, #2C3149);
  --bevel-line-2: light-dark(#D8C8AF, #3A4058);
  --bevel-line-3: light-dark(#C4B192, #4A536F);
  --bevel-brass: light-dark(#8F6310, #D9A445);
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
  font-family: var(--bevel-font-sans);
  color: var(--bevel-ink);
  -webkit-text-size-adjust: 100%;
  text-size-adjust: 100%;
}

.clay-menu-dock { display: flex; align-items: center; gap: 6px; font-size: 13px; line-height: 1.4; }
.clay-menu-dock[data-embedded] { position: relative; display: inline-flex; }
.clay-menu-dock[hidden], .clay-menu-spots[hidden], .clay-menu-dock [hidden], .clay-seo [hidden] { display: none !important; }

.clay-menu { box-sizing: border-box; display: inline-flex; align-items: center; gap: 2px; user-select: none; -webkit-user-select: none; }

`,le=`.clay-menu-goo { position: relative; display: inline-grid; place-items: center; box-sizing: border-box; margin: 0; padding: 0; cursor: pointer; }
.clay-menu-goo svg { display: block; }
.clay-menu-dot { position: absolute; top: -1px; right: -2px; width: 8px; height: 8px; border-radius: 50%; background: var(--bevel-brass); box-shadow: 0 0 0 2px var(--bevel-surface); }

.clay-menu :focus-visible, .clay-roster:focus-visible { outline: 2px solid var(--bevel-brass); outline-offset: 2px; }
.clay-menu-edit:focus-visible { outline: none; }
.clay-menu-edit:focus-visible .clay-menu-track { outline: 2px solid var(--bevel-brass); outline-offset: 2px; }

`,re=`/* Popovers: the menu, the people panel, the status note. Below the dock, above it in a bottom corner. */
.clay-menu-list, .clay-people, .clay-menu-note { position: absolute; z-index: 1; top: calc(100% + 8px); right: 0; box-sizing: border-box; margin: 0; color: var(--bevel-ink); text-align: left; }
.clay-people, .clay-menu-note { border: 1px solid var(--bevel-line-2); background: var(--bevel-surface); box-shadow: var(--clay-shadow); }

.clay-menu-list { width: max-content; max-width: calc(100vw - 24px); max-height: calc(100dvh - 72px); overflow-y: auto; overscroll-behavior: contain; }
.clay-menu-list[popover] { position: fixed; inset: auto; margin: 0; }
.clay-menu-list [role^="menuitem"] { box-sizing: border-box; display: flex; align-items: center; justify-content: space-between; gap: 12px; width: 100%; margin: 0; border: 0; background: none; text-align: left; text-decoration: none; cursor: pointer; }
.clay-menu-list [role^="menuitem"]:hover:not([aria-disabled="true"]), .clay-menu-list [role^="menuitem"]:focus-visible { outline: none; background: var(--bevel-sunk); }
.clay-menu-list [role^="menuitem"][aria-disabled="true"] { cursor: default; }
.clay-menu-pair { display: flex; flex-wrap: wrap; gap: 12px; padding: 3px 9px; }
.clay-menu-list .clay-menu-pair [role="menuitem"] { flex: none; width: auto; justify-content: flex-start; gap: 9px; }
.clay-menu-list kbd { flex: none; white-space: nowrap; }
/* The message, then a small Dismiss right after its last word (David, Nov 11). Both are inline, so
   Dismiss follows the text wherever it wraps; the message reaches over the whole row, and Dismiss
   sits above it with its own reach. */
.clay-menu-row { position: relative; padding: 5px 9px; }
.clay-menu-row:has(> [part~="item"]:hover, > [part~="item"]:focus-visible) { background: var(--bevel-sunk); }
.clay-menu-list .clay-menu-row > [part~="item"], .clay-menu-list .clay-menu-row > [part~="item"]:is(:hover, :focus-visible) { display: inline; width: auto; padding: 0; background: none; }
.clay-menu-list .clay-menu-row > [part~="item"]::before { content: ""; position: absolute; inset: 0; }
.clay-menu-list .clay-menu-row > [part~="dismiss"], .clay-menu-list .clay-menu-row > [part~="dismiss"]:is(:hover, :focus-visible) { position: relative; z-index: 1; display: inline; width: auto; min-height: 0; margin-left: 6px; padding: 0; background: none; white-space: nowrap; }
.clay-menu-list .clay-menu-row > [part~="dismiss"]::after { content: ""; position: absolute; inset: -6px -6px; }
.clay-menu-row [part~="dismiss-label"] { display: inline-block; padding: 2px 7px; border-radius: 4px; background: var(--bevel-sunk); color: var(--bevel-ink-2); font: 500 11px/1.3 var(--bevel-font-sans); }
.clay-menu-row > [part~="dismiss"]:hover [part~="dismiss-label"], .clay-menu-row > [part~="dismiss"]:focus-visible [part~="dismiss-label"] { color: var(--bevel-ink); }
.clay-menu-upgrade { display: inline-flex; align-items: center; gap: 8px; }
.clay-menu-upgrade::before { content: ""; width: 7px; height: 7px; border-radius: 50%; background: var(--bevel-brass); }
.clay-menu-toggle .clay-menu-track { flex: none; }
.clay-menu-version { box-sizing: border-box; margin: 0; padding: 7px 9px; }

.clay-menu-note { z-index: 2; width: max-content; max-width: 264px; padding: 8px 12px; font-size: 13px; line-height: 1.45; }

.clay-menu-stepper { display: flex; align-items: center; gap: 2px; padding-left: 9px; font: 400 13px/1.35 var(--bevel-font-sans); color: var(--bevel-ink); }
.clay-menu-stepper-label { flex: 1; margin-right: 12px; white-space: nowrap; }
.clay-menu-stepper-value { min-width: 5.5em; text-align: center; white-space: nowrap; font-variant-numeric: tabular-nums; }
.clay-menu-list .clay-menu-stepper [role="menuitem"] { flex: none; display: inline-grid; place-items: center; box-sizing: border-box; width: 36px; min-height: 36px; padding: 0; }
.clay-menu-stepper svg { display: block; width: 16px; height: 16px; fill: currentColor; }

`,xe=ie+le+re,ye=`/* The dock: the roster (when a page opts in) and the capsule, fixed in one corner. */
.clay-menu-dock { position: fixed; z-index: 2147482000; top: max(12px, env(safe-area-inset-top)); right: max(12px, env(safe-area-inset-right)); }
.clay-menu-dock[data-corner$="left"] { right: auto; left: max(12px, env(safe-area-inset-left)); flex-direction: row-reverse; }
.clay-menu-dock[data-corner^="bottom"] { top: auto; bottom: max(12px, env(safe-area-inset-bottom)); }
.clay-menu-dock[data-corner$="left"] :is(.clay-menu-list, .clay-people, .clay-menu-note) { right: auto; left: 0; }
.clay-menu-dock[data-corner^="bottom"] :is(.clay-menu-list, .clay-people, .clay-menu-note) { top: auto; bottom: calc(100% + 8px); }
`,ge=`.clay-menu { height: 34px; padding: 0 4px 0 2px; border: 1px solid var(--bevel-line-3); background: var(--bevel-surface); box-shadow: var(--clay-shadow); }
.clay-menu-goo { width: 28px; height: 28px; min-height: 0; border-radius: 0; border: 2px solid transparent; background: none; color: var(--bevel-ink); }
.clay-menu-goo::before { content: ""; position: absolute; inset: -8px -8px -8px -4px; }
.clay-menu-goo svg { width: 18px; height: auto; fill: currentColor; }
.clay-menu-goo:hover svg { translate: 0 -1px; }
.clay-menu-goo:active, .clay-menu-goo[aria-expanded="true"] { background: var(--bevel-face-in); border-color: var(--bevel-edge-in) var(--bevel-edge-in-hi) var(--bevel-edge-in-hi) var(--bevel-edge-in); }
.clay-menu-goo:active svg, .clay-menu-goo[aria-expanded="true"] svg { translate: 1px 1px; }
.clay-menu-list { min-width: 284px; padding: 6px; border: 1px solid var(--bevel-line-2); background: var(--bevel-surface); box-shadow: var(--clay-shadow); }
.clay-menu-list [role^="menuitem"] { min-height: 32px; padding: 5px 9px; font: 400 13px/1.35 var(--bevel-font-sans); color: var(--bevel-ink); }
.clay-menu-list [role^="menuitem"][aria-disabled="true"] { color: var(--bevel-faint); }
.clay-menu-list [role^="menuitem"][aria-disabled="true"] kbd { opacity: .6; }
.clay-menu-list .clay-menu-pair [role="menuitem"] { min-height: 30px; padding: 4px 6px 4px 9px; border: 1px solid var(--bevel-line); }
.clay-menu-list kbd { font: 500 11px/1 var(--bevel-font-mono); padding: 3px 5px; border: 1px solid var(--bevel-line-2); background: var(--bevel-sunk); color: var(--bevel-muted); }
.clay-menu-list [part~="quiet"] { font-size: 12px; color: var(--bevel-muted); }
.clay-menu-sep { height: 1px; margin: 4px 2px; background: var(--bevel-line); }
.clay-menu-version { font: 400 13px/1.35 var(--bevel-font-sans); color: var(--bevel-ink); }
`,ve=`@media (pointer: coarse), (max-width: 680px) {
  /* A phone menu fits the screen and never scrolls sideways: a little wider than on a desktop, long
     labels wrap, and the shortcut keys a phone cannot press are hidden (David, Nov 11). */
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
`;var j='<svg xmlns="http://www.w3.org/2000/svg" viewBox="-45.87 -57.96 100.26 60.98" fill="currentColor" aria-hidden="true" focusable="false"><path fill-rule="evenodd" d="M44.35 -4.56Q40.03 -0.52 34.69 -0.39Q29.34 -0.25 24.81 -0.14Q20.27 -0.02 15.6 0Q10.93 0.02 6.41 0Q1.89 -0.02 -2.79 -0.02Q-7.48 -0.02 -12.11 -0.14Q-16.73 -0.25 -21.64 -0.27Q-26.55 -0.3 -31.72 -2.25Q-36.88 -4.2 -39.87 -9.12Q-42.87 -14.03 -42.8 -19.61Q-42.74 -25.18 -40.43 -29.94Q-38.12 -34.69 -34.82 -38.78Q-31.52 -42.87 -27.25 -45.6Q-22.98 -48.33 -18.35 -50.59Q-13.73 -52.85 -8.7 -53.88Q-3.68 -54.91 1.32 -54.93Q6.32 -54.96 11.44 -54.56Q16.55 -54.16 21.35 -52.2Q26.16 -50.24 30.69 -48.01Q35.22 -45.78 38.99 -42.18Q42.76 -38.59 45.67 -34.37Q48.59 -30.15 49.99 -24.87Q51.39 -19.59 50.04 -14.09Q48.68 -8.6 44.35 -4.56ZM0.66 -30.49C0.5 -28.73 -0.88 -27.42 -2.42 -27.57C-3.96 -27.71 -5.08 -29.25 -4.91 -31.01C-4.75 -32.77 -3.36 -34.08 -1.83 -33.94C-0.29 -33.79 0.83 -32.25 0.66 -30.49ZM13.61 -29.28C13.44 -27.52 12.06 -26.21 10.52 -26.35C8.98 -26.5 7.87 -28.04 8.03 -29.8C8.2 -31.56 9.58 -32.87 11.12 -32.73C12.66 -32.58 13.77 -31.04 13.61 -29.28ZM-1.23 -17.71Q-0.62 -17.35 -0.03 -17.08Q0.56 -16.81 1.15 -16.54Q1.73 -16.28 2.4 -16Q3.07 -15.72 3.87 -15.6Q4.66 -15.48 5.4 -15.55Q6.13 -15.62 6.77 -15.79Q7.41 -15.96 7.97 -16.23Q8.53 -16.5 9 -16.91Q9.46 -17.32 9.73 -17.9Q9.99 -18.47 10.08 -19.12Q10.17 -19.77 10.08 -20.42Q9.99 -21.06 9.45 -21.32Q8.91 -21.58 8.27 -21.38Q7.63 -21.17 6.99 -20.91Q6.36 -20.65 5.7 -20.61Q5.04 -20.56 4.33 -20.73Q3.62 -20.9 2.92 -21.14Q2.23 -21.39 1.54 -21.68Q0.85 -21.97 0.16 -22.27Q-0.52 -22.56 -1.22 -22.79Q-1.92 -23.03 -2.49 -22.61Q-3.05 -22.19 -3.23 -21.5Q-3.4 -20.81 -3.2 -20.09Q-3.01 -19.38 -3.02 -19.15Q-3.02 -18.93 -2.42 -18.5Q-1.83 -18.06 -1.23 -17.71Z"/></svg>';var G="1",w=8,se="Dismiss",ce='<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 256 256" aria-hidden="true" focusable="false"><path d="M224,128a8,8,0,0,1-8,8H59.31l58.35,58.34a8,8,0,0,1-11.32,11.32l-72-72a8,8,0,0,1,0-11.32l72-72a8,8,0,0,1,11.32,11.32L59.31,120H216A8,8,0,0,1,224,128Z"/></svg>',ue='<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 256 256" aria-hidden="true" focusable="false"><path d="M221.66,133.66l-72,72a8,8,0,0,1-11.32-11.32L196.69,136H40a8,8,0,0,1,0-16H196.69L138.34,61.66a8,8,0,0,1,11.32-11.32l72,72A8,8,0,0,1,221.66,133.66Z"/></svg>',de='<span class="clay-menu-track" aria-hidden="true"><span class="clay-menu-thumb"></span></span>',pe=6e3;function me(){let h=document.querySelectorAll("[clay-menu-slot]");return h.length>1&&console.warn("[clay] This page has more than one clay-menu-slot. The menu uses the first."),h[0]||null}function be(){let h=document.querySelector("style[clay-menu-skin]");return h?h.getAttribute("clay-menu-skin")!==G?(console.warn(`[clay] This runtime reads clay-menu-skin="${G}". The menu keeps its default look.`),null):h.textContent:null}function Ce({capsuleStart:h="",sections:z=[null],icon:W=j,label:I="Clay menu"}={}){let K=new Set(z.flat().filter(Boolean)),k=me(),y=!!k,V=y?be():null,s=document.createElement("div");s.className="clay-menu-dock",s.setAttribute("clay","editor-ui"),s.setAttribute("part","dock"),y&&(s.dataset.embedded=""),s.dataset.corner="top-right",s.innerHTML='<div class="clay-menu" part="capsule" clay="editor-ui">'+h+`<button type="button" class="clay-menu-goo" part="button" aria-haspopup="menu" aria-expanded="false" aria-label="${I}">${W}</button></div><div class="clay-menu-list" part="list" role="menu" aria-label="${I}" hidden></div><div class="clay-menu-note" part="note" role="status" hidden></div>`;let u=s.querySelector(".clay-menu-goo"),i=s.querySelector(".clay-menu-list"),A=s.querySelector(".clay-menu-note"),d=new Map,H=[];if(y){let e=k.querySelector("template[clay-menu-icon]");e&&u.replaceChildren(e.content.cloneNode(!0));let n=k.getAttribute("clay-menu-label");n&&(u.setAttribute("aria-label",n),u.title=n,i.setAttribute("aria-label",n))}let F=y&&typeof i.showPopover=="function";F&&i.setAttribute("popover","manual");function q(){let e=u.getBoundingClientRect(),{offsetWidth:n,offsetHeight:t}=i,a=Math.max(w,Math.min(e.right-n,window.innerWidth-n-w)),o=e.bottom+w,l=o+t>window.innerHeight-w?Math.max(w,e.top-t-w):o;i.style.left=`${Math.round(a)}px`,i.style.top=`${Math.round(l)}px`}let D=()=>s.getRootNode().activeElement||null,m=()=>!i.hidden;function _(e){i.hidden=!e,u.setAttribute("aria-expanded",e?"true":"false"),F&&(e?(i.showPopover(),q()):i.matches(":popover-open")&&i.hidePopover())}let B=()=>[...i.querySelectorAll('[role^="menuitem"]:not([aria-disabled="true"])')];function E(e){let n=B();n.length&&n[(e+n.length)%n.length].focus()}function x(){m()&&_(!1)}function N(){if(!m()){for(let e of H)e();C(),_(!0)}}function T(e){let n=e.href&&!e.disabled,t=document.createElement(n?"a":"button");if(n?(t.href=e.href,t.target="_blank",t.rel="noopener"):t.type="button",t.setAttribute("role","menuitem"),t.setAttribute("part",e.quiet?"item quiet":"item"),t.dataset.menuId=e.id,e.badge&&e.id==="upgrade"){let a=document.createElement("span");a.className="clay-menu-upgrade",a.textContent=e.label,t.append(a)}else t.textContent=e.label;if(e.keys){let a=document.createElement("kbd");a.textContent=e.keys,t.append(a)}return e.toggle&&(t.setAttribute("role","menuitemcheckbox"),t.setAttribute("aria-checked",e.checked?"true":"false"),t.classList.add("clay-menu-toggle"),t.insertAdjacentHTML("beforeend",de)),e.disabled&&t.setAttribute("aria-disabled","true"),t.addEventListener("click",()=>{e.disabled||(!e.toggle&&!e.keepOpen&&x(),e.run&&e.run({anchor:u}))}),t}function X(e){if(e.text){let n=document.createElement("p");return n.className="clay-menu-version",n.setAttribute("part","text"),n.dataset.menuId=e.id,n.textContent=e.label,n}if(e.pair){let n=document.createElement("div");n.className="clay-menu-pair",n.dataset.menuId=e.id;for(let t of e.pair)n.append(T(t));return n}return e.stepper?J(e):e.dismiss?Y(e):T(e)}function Y(e){let n=document.createElement("div");n.className="clay-menu-row",n.dataset.menuId=e.id;let t=document.createElement("button");t.type="button",t.setAttribute("role","menuitem"),t.setAttribute("part","dismiss"),t.dataset.menuId=`${e.id}.dismiss`,t.title=e.dismiss.label;let a=document.createElement("span");return a.setAttribute("part","dismiss-label"),a.textContent=se,t.append(a),t.addEventListener("click",async()=>{t.disabled=!0;let o;try{o=await e.dismiss.run?.()}catch(l){console.warn("[clay] A menu item could not be removed.",l),t.disabled=!1,$("That didn't work. Try again.");return}if(o===!1){t.disabled=!1;return}R(e.id)}),n.append(T(e),t),n}function J(e){let{label:n,value:t,prevLabel:a,nextLabel:o}=e.stepper,l=document.createElement("div");l.className="clay-menu-stepper",l.setAttribute("part","stepper"),l.dataset.menuId=e.id;let r=document.createElement("span");r.className="clay-menu-stepper-label",r.textContent=n;let b=document.createElement("span");b.className="clay-menu-stepper-value",b.setAttribute("aria-live","polite"),b.textContent=t;let Q=(g,M,v)=>{let p=document.createElement("button");return p.type="button",p.setAttribute("role","menuitem"),p.setAttribute("part","step"),p.setAttribute("aria-label",v),p.dataset.menuId=`${e.id}.${g}`,p.innerHTML=M,p.addEventListener("click",()=>d.get(e.id)?.stepper?.[g]?.()),p};return l.append(r,Q("prev",ce,a),b,Q("next",ue,o)),l}function ee(){let e=a=>[...d.values()].filter(o=>o.section===a),n=[...d.keys()].filter(a=>!K.has(a)&&!d.get(a).section),t=z.map(a=>(a||n).map(o=>d.get(o)).filter(Boolean));return[e("start"),...t,e("end")].filter(a=>a.length)}function C(){let e=D(),n=e&&i.contains(e)?e.dataset.menuId:null;i.textContent="",ee().forEach((o,l)=>{if(l){let r=document.createElement("div");r.className="clay-menu-sep",r.setAttribute("role","separator"),r.setAttribute("part","separator"),i.append(r)}for(let r of o)i.append(X(r))});let t=u.querySelector(".clay-menu-dot"),a=[...d.values()].some(o=>o.badge);if(a&&!t){let o=document.createElement("span");o.className="clay-menu-dot",o.setAttribute("aria-hidden","true"),u.append(o)}else!a&&t&&t.remove();if(n){let o=B().find(l=>l.dataset.menuId===n);o?o.focus():E(0)}}let O=({id:e,label:n,run:t,badge:a,keys:o,disabled:l,href:r,keepOpen:b})=>({id:e,href:typeof r=="string"?r:"",label:String(n??""),run:typeof t=="function"?t:null,badge:!!a,keys:o?String(o):"",disabled:!!l,keepOpen:!!b});function te({id:e,label:n,run:t,badge:a,keys:o,disabled:l,text:r,pair:b,href:Q,section:g,quiet:M,dismiss:v,toggle:p,checked:ae,keepOpen:oe,stepper:f}={}){if(!e)return null;let c=O({id:e,label:n,run:t,badge:a,keys:o,disabled:l,href:Q,keepOpen:oe});if((g==="start"||g==="end")&&(c.section=g),M&&(c.quiet=!0),f){c.stepper={label:String(f.label??""),value:String(f.value??""),prev:typeof f.prev=="function"?f.prev:null,next:typeof f.next=="function"?f.next:null,prevLabel:String(f.prevLabel||"Previous"),nextLabel:String(f.nextLabel||"Next")};let P=[...i.querySelectorAll(".clay-menu-stepper")].find(L=>L.dataset.menuId===e)?.querySelector(".clay-menu-stepper-value"),S=d.get(e);if(S?.stepper&&["label","prevLabel","nextLabel"].every(L=>S.stepper[L]===c.stepper[L])&&S.section===c.section&&S.quiet===c.quiet&&P)return d.set(e,c),P.textContent=c.stepper.value,e}return r&&(c.text=!0),Array.isArray(b)&&(c.pair=b.map(O)),p&&(c.toggle=!0,c.checked=!!ae),v&&(c.dismiss={label:String(v.label||"Dismiss"),run:typeof v.run=="function"?v.run:null}),d.set(e,c),C(),e}function R(e){d.delete(e)&&C()}function $(e){e&&(A.textContent=e,A.hidden=!1,clearTimeout(U),U=setTimeout(()=>{A.hidden=!0,A.textContent=""},pe))}let U=0;u.addEventListener("click",()=>{m()?x():N()}),document.addEventListener("click",e=>{m()&&!e.composedPath().includes(s)&&x()}),s.addEventListener("keydown",e=>{if(e.key==="Escape"){if(!m())return;e.preventDefault(),x(),u.focus();return}if(e.target===u&&(e.key==="Enter"||e.key===" ")){e.preventDefault(),m()?x():(N(),E(0));return}if(!m())return;if(e.key==="Enter"&&i.contains(e.target)){e.preventDefault(),e.target.click();return}let n=B();if(!n.length)return;let t=n.indexOf(D());e.key==="ArrowDown"?(e.preventDefault(),E(t<0?0:t+1)):e.key==="ArrowUp"&&(e.preventDefault(),E(t<0?n.length-1:t-1))});function ne(e){let n=Z({name:"menu",content:s,css:e,parent:k||document.body,attributes:{"clay-mode-toggle":""}});return y&&(window.addEventListener("resize",x),window.addEventListener("scroll",()=>{F&&m()&&q()},{capture:!0,passive:!0})),n}return{root:s,goo:u,list:i,slot:k,embedded:y,skin:V,items:d,addItem:te,removeItem:R,render:C,open:N,close:x,isOpen:m,note:$,beforeOpen:H,focusedEl:D,mount:ne}}export{he as a,ie as b,le as c,re as d,xe as e,ye as f,ge as g,ve as h,de as i,be as j,Ce as k};
