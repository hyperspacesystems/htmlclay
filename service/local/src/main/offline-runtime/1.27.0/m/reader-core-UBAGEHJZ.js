import{a as X,b as S,c as Q}from"./c-NUHP2XDB.js";import{g as J}from"./c-3WMYTUG7.js";import{a as w}from"./c-VKEDWYDF.js";import{b as G}from"./c-YECX4ATP.js";import{e as Y}from"./c-UND2IXIL.js";import"./c-IJUK2GPW.js";import{a as A,b as p,c as r,f,g as n,h as k,j as M,o as z}from"./c-APIIHRRQ.js";import"./c-P7KNDG34.js";import{c as U,e as V}from"./c-BPF2NSPV.js";import"./c-CYWLPPUS.js";import"./c-U4TWEIKZ.js";import"./c-FJL6KJLW.js";import"./c-2YYDMLNZ.js";import{N as B,y as W}from"./c-65KMWBMM.js";import"./c-HA6INT6J.js";import"./c-WRBKEJD4.js";import{a as D}from"./c-VHNSBD2M.js";var O={};D(O,{activateInitialMode:()=>Le,canEdit:()=>r,default:()=>_e,init:()=>R,isEditMode:()=>f,isOwner:()=>k,mode:()=>n,modeLifecycle:()=>Se,noteLiveSwitch:()=>Te,setMode:()=>g,setPageTypeOnPageLoad:()=>ne,toggleEditMode:()=>oe});var C=!1,ke=e=>{if(typeof e?.id=="string"&&e.id.startsWith("clay:"))throw new TypeError(`clay.modeLifecycle: the id "${e.id}" is reserved`);return z(e)},Se={register:ke,canWrite:()=>M().phase==="idle"&&f,get state(){return Object.freeze(M())}},N=null,ee=()=>N||=import("./c-N4X2JFGQ.js").catch(()=>N=null),_=null,Te=e=>{_=e};async function Le(){if(f)try{await(await ee()).activateInitial()}catch(e){console.error("clayjs: the mode lifecycle failed to start:",e)}}var h=null;async function g(e){if(e!=="edit"&&e!=="view")throw new TypeError(`clay.setMode: unknown mode "${e}"`);if(h)return h.target===e?h.promise:{ok:!1,mode:n,code:"busy"};if(e===n&&!N)return{ok:!0,mode:n,changed:!1};if(p)return{ok:!1,mode:n,code:"not-allowed"};let t=De(e);h={target:e,promise:t};try{return await t}finally{h?.promise===t&&(h=null)}}async function De(e){let t=typeof window<"u"?window.clay?._requestHostMode:void 0;if(typeof t=="function"&&e!==n){let o;try{o=await t(e)}catch{return{ok:!1,mode:n,code:"host-failed"}}if(o?.reason!=="host-unsupported")return o?.ok?(C=!0,{ok:!0,mode:e,changed:!0,reloading:!0}):{ok:!1,mode:n,code:o?.reason||"host-refused"}}return e==="edit"&&n!=="edit"?Ae():!r&&!(e==="view"&&n==="edit")?{ok:!1,mode:n,code:"not-allowed"}:te(e)}async function Ae(){let e=location.pathname,{checkHostPermission:t}=await import("./core/host-permission-XKJL5EMB.js"),o=await t({fresh:!0});return location.pathname!==e?{ok:!1,mode:n,code:"cancelled"}:o===!1?{ok:!1,mode:n,code:"permission-denied"}:r?te("edit"):{ok:!1,mode:n,code:"not-allowed"}}async function te(e){if(_){let t=await ee(),o=t&&await t.switchMode(e,_);if(o)return o}if(e===n)return{ok:!0,mode:n,changed:!1};if(n==="edit"){let t;try{t=await window.clay?.save?.flush?.()}catch(o){return{ok:!1,mode:n,code:"save-failed",state:o?.state}}if(t&&t.state&&t.state!=="saved"&&t.state!=="clean")return{ok:!1,mode:n,code:"save-failed",state:t.state}}return C=!0,A({mode:e,canEdit:r,internal:!0}),location.reload(),{ok:!0,mode:e,changed:!0,reloading:!0}}function Me(){let e=new URL(window.location.href);e.searchParams.set("editmode","true"),window.location.href=e.toString()}var oe=()=>p?Me():g(n==="edit"?"view":"edit");function Ne(){!r||p||window.addEventListener("pagehide",()=>{C||A({mode:n,canEdit:r,internal:!1})})}function Z(){r?document.documentElement.setAttribute("canedit","true"):document.documentElement.removeAttribute("canedit")}function ne(){w(()=>{document.documentElement.setAttribute("editmode",f?"true":"false"),document.documentElement.setAttribute("pageowner",k?"true":"false"),Z(),document.addEventListener("clay:capabilitychange",Z),p?document.documentElement.setAttribute("draftmode","true"):document.documentElement.removeAttribute("draftmode")})}function R(){ne(),Ne()}R();var _e={init:R,setMode:g,toggleEditMode:oe,isEditMode:f,isOwner:k,canEdit:r,mode:n};var fe={};var ie=typeof navigator<"u"&&/Mac|iPhone|iPad/.test(navigator.platform||navigator.userAgent),Ce=ie?"\u2318\u21E7E":"Ctrl+Shift+E",Re=`.clay-mode-note {
  position: fixed;
  right: calc(16px + env(safe-area-inset-right, 0px));
  bottom: calc(16px + env(safe-area-inset-bottom, 0px));
  z-index: 2147482000;
  display: flex;
  align-items: center;
  gap: 10px;
  max-width: min(420px, calc(100vw - 32px));
  padding: 10px 10px 10px 14px;
  border: 1px solid rgba(23, 28, 36, .16);
  border-radius: 12px;
  font-family: system-ui, -apple-system, "Segoe UI", sans-serif;
  font-size: 13px;
  line-height: 1.45;
  text-align: left;
  color: var(--clay-ui-fg, #1F2328);
  background: var(--clay-ui-bg, #F4F4F5);
  box-shadow: 0 8px 24px -12px rgba(23, 28, 36, .45);
  transition: opacity .25s ease, transform .25s ease;
}
.clay-mode-note kbd {
  font: inherit;
  font-weight: 600;
  padding: 1px 5px;
  border: 1px solid rgba(23, 28, 36, .2);
  border-radius: 5px;
  background: rgba(23, 28, 36, .07);
}
.clay-mode-note button {
  flex: none;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 24px;
  height: 24px;
  padding: 0;
  border: 0;
  border-radius: 6px;
  font: inherit;
  font-size: 16px;
  line-height: 1;
  color: inherit;
  background: none;
  cursor: pointer;
  opacity: .65;
}
.clay-mode-note button:hover { opacity: 1; background: rgba(23, 28, 36, .08); }
.clay-mode-note .clay-view-notice__edit {
  width: auto;
  padding: 0 10px;
  border: 1px solid rgba(23, 28, 36, .2);
  font-size: 13px;
  font-weight: 600;
  opacity: 1;
}
@media (prefers-color-scheme: dark) {
  .clay-mode-note .clay-view-notice__edit { border-color: rgba(233, 233, 236, .22); }
  .clay-mode-note {
    border-color: rgba(233, 233, 236, .16);
    color: var(--clay-ui-fg, #E9E9EC);
    background: var(--clay-ui-bg, #26282C);
    box-shadow: 0 8px 24px -12px rgba(0, 0, 0, .7);
  }
  .clay-mode-note kbd { border-color: rgba(233, 233, 236, .22); background: rgba(233, 233, 236, .1); }
  .clay-mode-note button:hover { background: rgba(233, 233, 236, .12); }
}
@media (prefers-reduced-motion: reduce) {
  .clay-mode-note { transition: none; }
}
@media (max-width: 559px) {
  .clay-mode-note {
    left: calc(16px + env(safe-area-inset-left, 0px));
    right: calc(16px + env(safe-area-inset-right, 0px));
    max-width: none;
  }
}`,ae=new WeakSet;function re(e){import("./c-HFN7W6JR.js").then(({mountEditorUI:t})=>{ae.has(e)||t({name:"mode-note",content:e,css:[Re]})})}function se(e){ae.add(e),e.getRootNode().host?.remove()}function ce(e){return g(e).then(t=>{t.ok||import("./c-QHIWXHRO.js").then(({modeRefusalText:o})=>{let i=o(t);i&&(typeof window.clay?.toast=="function"?window.clay.toast(i,"error"):typeof window.clay?.menu?.note=="function"?window.clay.menu.note(i):ue(i))})})}function Oe(e){(r||n==="edit")&&(!(ie?e.metaKey:e.ctrlKey)||!e.shiftKey||e.altKey||e.code!=="KeyE"||e.isComposing||(e.preventDefault(),ce(n==="edit"?"view":"edit")))}var Pe="In view mode, so edits won't be saved.",je=":is(input, textarea, select):is([persist], [clay-persist])",P="[editmode\\:contenteditable], [viewmode\\:readonly], [viewmode\\:disabled]",s=null;function de(e){if(!e.isConnected)return!1;let t=B(e);return t.persist===W.FULL&&!t.extension}function le(e){let t=e?.nodeType===1?e:e?.parentElement;if(!t)return null;let o=t.closest(je)||t.closest(P)||(t.isContentEditable?t:null);return o&&de(o)?o:null}var Ie=e=>e.isContentEditable||e.matches("input, textarea, select")&&!e.readOnly&&!e.disabled,j=()=>r&&n==="view";function Ke(e){j()&&le(e.target)&&I()}function He(e){if(!j()||e.defaultPrevented||e.isComposing||e.ctrlKey||e.metaKey||e.altKey||!(e.key.length===1&&e.key!==" ")&&e.key!=="Backspace"&&e.key!=="Delete")return;let t=e.target;if(t?.nodeType!==1||Ie(t)||t!==document.body&&!t.closest(P))return;le(t===document.body?document.getSelection()?.anchorNode:t)?.matches(P)&&I()}function $e(e){if(!j())return;let t=e.target?.closest?.("[viewmode\\:disabled]");t&&de(t)&&I()}function I(){s||(console.warn('clayjs: this tab is in view mode, so edits made here are not saved. clay.setMode("edit") switches; a page that must edit as soon as it opens declares <html opens-in="edit">.'),document.querySelector('clay-editor-ui[data-clay-ui="mode-note"]')?.remove(),s=document.createElement("div"),s.setAttribute("clay","editor-ui"),s.setAttribute("role","alert"),s.className="clay-mode-note clay-view-notice",s.innerHTML='<span></span><button type="button" class="clay-view-notice__edit">Switch to edit</button>',s.querySelector("span").textContent=Pe,s.querySelector("button").addEventListener("click",()=>ce("edit")),re(s))}function qe(){n==="view"||!s||(se(s),s=null)}function ue(e){let t=document.createElement("div");t.setAttribute("clay","editor-ui"),t.setAttribute("role","status"),t.className="clay-mode-note",t.innerHTML=`<span>This page opens in view mode. Press <kbd>${Ce}</kbd> to edit.</span><button type="button" aria-label="Dismiss">\xD7</button>`,e&&(t.querySelector("span").textContent=e);let o=()=>se(t);t.querySelector("button").addEventListener("click",o),addEventListener("keydown",i=>{i.key==="Escape"&&o()},{once:!0}),setTimeout(o,12e3),re(t)}p||(document.addEventListener("keydown",Oe),document.addEventListener("input",Ke,!0),document.addEventListener("pointerdown",$e,!0),window.addEventListener("keydown",He),document.addEventListener("clay:modechange",qe),w(()=>{if(!r||n!=="view"||document.querySelector("[clay-mode-toggle]"))return;(t=>{let o=window.clay?.ready;window.clay?.storage||typeof o?.then!="function"?queueMicrotask(t):o.then(t,()=>{})})(()=>{if(window.clay?.menu||window.clay?.plugins?.includes?.("menu"))return;let t=window.clay?.storage?.local;!t||t.get("mode-note")||(t.set("mode-note",!0),ue())})}));var K={};D(K,{whenShown:()=>We});var Fe="This page can't be edited: the app serving it is out of date. Update HTML Clay to 1.9.0 or newer.",c=null,a=null;function b(){if(!c)return;let e=window.visualViewport,t=e?Math.max(0,window.innerHeight-e.height-e.offsetTop):0;S(c,"bottom",`calc(${16+t}px + env(safe-area-inset-bottom,0px))`)}function Ue(){c&&(S(c,"display","none"),window.visualViewport?.removeEventListener("resize",b),window.visualViewport?.removeEventListener("scroll",b))}function Ve(){let e=a.resolveTheme();c=a.bevelSurface("div",["position:fixed","left:50%","transform:translateX(-50%)","z-index:2147483001","display:flex","align-items:center","gap:10px 12px","width:max-content","max-width:calc(100vw - 24px)","flex-wrap:wrap","padding:8px 8px 8px 14px","text-align:left",`color-scheme:${a.pageScheme()}`],{theme:e,parts:["notice.root","notice.panel","staleHost.root","staleHost.panel"]}),c.setAttribute("clay",a.RUNTIME_ONLY),c.setAttribute("data-clay-stale-host",""),c.setAttribute("role","alert");let t=Q("span",["all:initial","color-scheme:inherit","display:inline-block","flex:none","width:8px","height:8px","border-radius:50%",`background:${a.TOKENS.ox}`]);t.setAttribute("clay",a.RUNTIME_ONLY);let o=["notice.dot","staleHost.dot"];a.themed(e)&&(a.markParts(t,e,o),X(t,a.partRules(e,o,{})));let i=a.bevelButton("Dismiss",{small:!0,variant:"quiet",onClick:Ue,theme:e,parts:["notice.button","staleHost.button"],labelParts:["notice.buttonLabel","staleHost.buttonLabel"]});i.setAttribute("aria-label","Dismiss this message"),c.append(t,a.bevelText("span",[],Fe,{theme:e,parts:["notice.text","staleHost.text"]}),i),document.body.appendChild(c),b(),a.themed(e)?S(c,"color-scheme",e.scheme==="light dark"?"light":e.scheme):a.paintLook(c,{background:a.TOKENS.surface}),window.visualViewport?.addEventListener("resize",b),window.visualViewport?.addEventListener("scroll",b)}async function ze(){if(!V()||f)return;let[e,t,o,i]=await Promise.all([import("./c-KHWZGJDG.js"),import("./c-46LJAZZJ.js"),import("./c-OCSZ4QGN.js"),import("./c-ELXMSEN5.js")]);a={...e,...i,TOKENS:t.TOKENS,resolveTheme:o.resolveTheme},Ve()}var me=Promise.resolve();w(()=>{me=ze()});var We=()=>me;var $={};D($,{createPageDataApi:()=>ye,readData:()=>Ze,setPageDataLiveSync:()=>tt,writeData:()=>et});var Be="data-read",Ge="data-write";function T(e,t,o){let i=new Error(t);return i.code=e,i.details=o,i}var Je=["changed","recalculated","stale","why","code"];function pe(e,t){let o=t?.headers?.get?.("Clay-Stale")==="true",i=d=>o&&d.stale===void 0?{...d,stale:!0}:d;if(e===null||typeof e!="object"||Array.isArray(e)||!Object.hasOwn(e,"__meta__"))return{data:e,meta:i({})};let{__meta__:m,...L}=e;if(m===null||typeof m!="object"||Array.isArray(m))return{data:e,meta:i({})};let v={};for(let d of Je)Object.hasOwn(m,d)&&(v[d]=m[d]);return{data:L,meta:i(v)}}async function he(e){if(typeof e.json=="function")return e.json();let t=await e.text();return t?JSON.parse(t):null}async function Ye(e){let t={};try{t=await he(e)||{}}catch{}let o=t.message||t.error||e.statusText||`Request failed (${e.status})`,i=new Error(o);return i.status=e.status,i.error=t.error,i.details=t.details,i}function Xe(e){let t=e.headers?.get?.("ETag");return typeof t=="string"&&t?t:null}function Qe(e,t){let o=`${t.origin}${t.pathname}`;return e!==void 0?`${o}?data=${encodeURIComponent(JSON.stringify(e))}`:`${t.origin}/_/api${t.pathname}`}async function we(){let{isDraftMode:e}=await import("./core/is-edit-mode-IN2WAWR3.js");return e?T("DataUnavailableInDraft","Page data is kept in this browser for this visitor."):null}function ye({supports:e=Y,token:t=U,fetchImpl:o=(...x)=>fetch(...x),getLocation:i=()=>window.location,eventTarget:m=document,loadWriteModules:L,syncTimeoutMs:v,httpTimeoutMs:d}={}){let x=null,y=null;function ge(l){x=l}async function be(l){let E=await we();if(E)throw E;if(!await e(Be))throw T("DataReadUnsupported","This host does not support reading page data.");let u;try{u=await o(Qe(l,i()),{method:"GET",credentials:"same-origin",cache:"no-store",redirect:"error"})}catch(F){throw Object.assign(T("DataReadFailed",F?.message||"Could not read page data."),{cause:F})}if(!u.ok)throw await Ye(u);let{data:q,meta:Ee}=pe(await he(u),u);return{data:q,etag:Xe(u),...Ee}}function ve(){return y||(y=import("./c-ZBWQ5MOM.js").then(({createPageDataWrite:l})=>l({supports:e,token:t,fetchImpl:o,getLocation:i,eventTarget:m,getLiveSync:()=>x,loadWriteModules:L,syncTimeoutMs:v,httpTimeoutMs:d,splitMeta:pe})).catch(l=>{throw y=null,l}),y)}async function xe(l,E){let u=await we();if(u)throw u;if(!await e(Ge))throw T("DataWriteUnsupported","This host does not support writing page data.");return(await ve()).writeData(l,E)}return{readData:be,writeData:xe,setLiveSync:ge}}var H=ye(),Ze=H.readData,et=H.writeData,tt=H.setLiveSync;var wt={"lib/mutation.js":G,"core/preserve-authored.js":J,"core/edit-mode.js":O,"core/mode-key.js":fe,"core/stale-host-notice.js":K,"core/page-data.js":$};export{wt as core};
