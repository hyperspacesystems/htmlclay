import{b as s}from"./c-Y6F2FORL.js";import{b as h}from"./c-QY7F6EN6.js";import{a as d}from"./c-2YYDMLNZ.js";import"./c-ICMAOOGK.js";import"./c-WRBKEJD4.js";import"./c-VHNSBD2M.js";var m=`html:has(> body > .clay-intro) { height: auto !important; overflow-y: auto !important; }
body:has(> .clay-intro) { height: auto !important; overflow: visible !important; }
.clay-intro {
  position: relative;
  display: block;
}
@media print { .clay-intro { display: none; } }
`,f=`:host { color: inherit !important; }
.clay-intro__close {
  position: absolute; top: 10px; right: 10px; z-index: 1;
  display: inline-flex; align-items: center; justify-content: center;
  width: 28px; height: 28px; min-height: 0; margin: 0; padding: 0;
  border: 0; border-radius: 4px; background: none; box-shadow: none;
  color: inherit; opacity: .55; cursor: pointer; line-height: 1;
}
.clay-intro__close:hover { opacity: 1; background: none; color: inherit; }
.clay-intro__close:focus-visible { opacity: 1; outline: 2px solid currentColor; outline-offset: 2px; }
.clay-intro__close svg { display: block; width: 12px; height: 12px; fill: currentColor; stroke: none; }
@media (pointer: coarse) { .clay-intro__close { width: 44px; height: 44px; top: 4px; right: 4px; } }
.clay-intro__close[data-manual] {
  width: auto; height: 36px; padding: 0 14px; opacity: 1;
  border-radius: 999px; font-size: 12px; font-weight: 500;
  text-transform: uppercase; letter-spacing: .04em; white-space: nowrap;
}
@media (pointer: coarse), (max-width: 680px) { .clay-intro__close[data-manual] { height: 44px; } }
`;var u={close:"Hide this introduction",hide:"Hide intro",label:t=>`About ${t}`},v="a[href], button, input, select, textarea, [tabindex]:not([tabindex='-1'])",C="textarea, select",_=["checkbox","radio","button","submit","reset","file"];function E(t){return t.isContentEditable||t.closest(C)?!0:t.tagName!=="INPUT"?!1:_.indexOf(t.type)<0}function g(){try{return new URLSearchParams(window.location.search).get("intro")==="1"}catch{return!1}}function S(){if(document.getElementById("clay-intro-style"))return;let t=document.createElement("style");t.id="clay-intro-style",t.setAttribute("clay","editor-ui"),t.textContent=m,document.head.appendChild(t)}function b(){let t=[...document.querySelectorAll(v)].find(e=>!e.disabled&&!E(e)&&!e.closest("[clay~='editor-ui']")&&(typeof e.checkVisibility!="function"||e.checkVisibility()));t?t.focus({preventScroll:!0}):document.body.focus({preventScroll:!0})}function y({template:t,appName:e,labelText:r,onClose:i}){S();let o=document.createElement("section");o.className="clay-intro",o.setAttribute("clay","editor-ui"),o.setAttribute("sap-ignore",""),o.setAttribute("aria-label",u.label(e)),o.appendChild(t.content.cloneNode(!0));let n=document.createElement("button");return n.type="button",n.className="clay-intro__close",r?(n.textContent=r,n.setAttribute("data-manual","")):(n.setAttribute("aria-label",u.close),n.innerHTML='<svg width="12" height="12" viewBox="0 0 12 12" fill="currentColor" aria-hidden="true"><path d="M1.4 0 6 4.6 10.6 0 12 1.4 7.4 6 12 10.6 10.6 12 6 7.4 1.4 12 0 10.6 4.6 6 0 1.4z"/></svg>'),n.addEventListener("click",()=>i()),h({name:"intro-close",content:n,css:[f],parent:o}),o.addEventListener("click",c=>{if(c.target.closest("[visitor-intro-dismiss]")){i();return}c.target.closest("[visitor-intro-start]")&&(c.preventDefault(),window.scrollTo({top:window.scrollY+o.getBoundingClientRect().bottom,behavior:"smooth"}))}),o}function M({memory:t,template:e,appName:r,hadDraft:i}){if(!e||!e.content.textContent.trim()&&!e.content.querySelector("img, svg, video, canvas"))return null;if(e.getAttribute("visitor-intro")==="manual"){let o=window.matchMedia&&window.matchMedia("print").matches,n=g()||!(t.get().intro.done||d.servedHash||o);return T({memory:t,template:e,appName:r,open:n})}return!g()&&(t.get().intro.done||i||d.servedHash||window.matchMedia&&window.matchMedia("print").matches)?null:A({memory:t,template:e,appName:r})}function A({memory:t,template:e,appName:r}){let i=y({template:e,appName:r,onClose:()=>c("closed")}),o=!1,n=0;function c(l){if(o)return;o=!0,removeEventListener("scroll",p),n&&cancelAnimationFrame(n);let a=i.getBoundingClientRect().height,x=window.scrollY;if(t.get().intro.done=l,t.save(),i.remove(),l==="scrolled"){let w=/hidden|clip/.test(getComputedStyle(document.body).overflowY+getComputedStyle(document.documentElement).overflowY);window.scrollTo(0,w?0:Math.max(0,x-a))}s(),l!=="scrolled"&&b()}function p(){n||(n=requestAnimationFrame(()=>{n=0;let l=i.getBoundingClientRect().bottom,a=scrollY>0&&scrollY+innerHeight>=document.documentElement.scrollHeight-1;(l<1||a&&l<innerHeight)&&c("scrolled")}))}return document.body.prepend(i),s(),addEventListener("scroll",p,{passive:!0}),i}function T({memory:t,template:e,appName:r,open:i}){if(!i)return null;let o=y({template:e,appName:r,labelText:u.hide,onClose:n});function n(){t.get().intro.done="closed",t.save(),o.remove(),s(),b()}return document.body.prepend(o),s(),o}export{u as COPY,M as mountIntro};
