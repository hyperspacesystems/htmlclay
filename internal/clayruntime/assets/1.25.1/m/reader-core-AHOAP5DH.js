import{a as W,b as x,c as B}from"./c-NUHP2XDB.js";import{g as F}from"./c-QCDVDZIR.js";import{a as m}from"./c-VKEDWYDF.js";import{b as V}from"./c-MZCOGNZL.js";import{e as z}from"./c-EQ5NBOGA.js";import"./c-IJUK2GPW.js";import{a as T,b as f,c as a,f as u,g as n,h as v,j as L,o as $}from"./c-C7SUMNMI.js";import{c as j,e as H}from"./c-BPF2NSPV.js";import"./c-P7KNDG34.js";import"./c-CYWLPPUS.js";import"./c-U4TWEIKZ.js";import"./c-MOP5KBWZ.js";import"./c-2YYDMLNZ.js";import"./c-HA6INT6J.js";import{d as q,s as U}from"./c-ICMAOOGK.js";import"./c-WRBKEJD4.js";import{a as S}from"./c-VHNSBD2M.js";var C={};S(C,{activateInitialMode:()=>ke,canEdit:()=>a,default:()=>Me,init:()=>A,isEditMode:()=>u,isOwner:()=>v,mode:()=>n,modeLifecycle:()=>xe,noteLiveSwitch:()=>Ee,setMode:()=>y,setPageTypeOnPageLoad:()=>Q,toggleEditMode:()=>X});var N=!1,ve=e=>{if(typeof e?.id=="string"&&e.id.startsWith("clay:"))throw new TypeError(`clay.modeLifecycle: the id "${e.id}" is reserved`);return $(e)},xe={register:ve,canWrite:()=>L().phase==="idle"&&u,get state(){return Object.freeze(L())}},D=null,J=()=>D||=import("./c-3WX5FABT.js").catch(()=>D=null),M=null,Ee=e=>{M=e};async function ke(){if(u)try{await(await J()).activateInitial()}catch(e){console.error("clayjs: the mode lifecycle failed to start:",e)}}var p=null;async function y(e){if(e!=="edit"&&e!=="view")throw new TypeError(`clay.setMode: unknown mode "${e}"`);if(p)return p.target===e?p.promise:{ok:!1,mode:n,code:"busy"};if(e===n&&!D)return{ok:!0,mode:n,changed:!1};if(f)return{ok:!1,mode:n,code:"not-allowed"};let t=Se(e);p={target:e,promise:t};try{return await t}finally{p?.promise===t&&(p=null)}}async function Se(e){let t=typeof window<"u"?window.clay?._requestHostMode:void 0;if(typeof t=="function"&&e!==n){let o;try{o=await t(e)}catch{return{ok:!1,mode:n,code:"host-failed"}}if(o?.reason!=="host-unsupported")return o?.ok?(N=!0,{ok:!0,mode:e,changed:!0,reloading:!0}):{ok:!1,mode:n,code:o?.reason||"host-refused"}}return e==="edit"&&n!=="edit"?Te():!a&&!(e==="view"&&n==="edit")?{ok:!1,mode:n,code:"not-allowed"}:Y(e)}async function Te(){let e=location.pathname,{checkHostPermission:t}=await import("./core/host-permission-5BYP24GC.js"),o=await t({fresh:!0});return location.pathname!==e?{ok:!1,mode:n,code:"cancelled"}:o===!1?{ok:!1,mode:n,code:"permission-denied"}:a?Y("edit"):{ok:!1,mode:n,code:"not-allowed"}}async function Y(e){if(M){let t=await J(),o=t&&await t.switchMode(e,M);if(o)return o}if(e===n)return{ok:!0,mode:n,changed:!1};if(n==="edit"){let t;try{t=await window.clay?.save?.flush?.()}catch(o){return{ok:!1,mode:n,code:"save-failed",state:o?.state}}if(t&&t.state&&t.state!=="saved"&&t.state!=="clean")return{ok:!1,mode:n,code:"save-failed",state:t.state}}return N=!0,T({mode:e,canEdit:a,internal:!0}),location.reload(),{ok:!0,mode:e,changed:!0,reloading:!0}}function Le(){let e=new URL(window.location.href);e.searchParams.set("editmode","true"),window.location.href=e.toString()}var X=()=>f?Le():y(n==="edit"?"view":"edit");function De(){!a||f||window.addEventListener("pagehide",()=>{N||T({mode:n,canEdit:a,internal:!1})})}function G(){a?document.documentElement.setAttribute("canedit","true"):document.documentElement.removeAttribute("canedit")}function Q(){m(()=>{document.documentElement.setAttribute("editmode",u?"true":"false"),document.documentElement.setAttribute("pageowner",v?"true":"false"),G(),document.addEventListener("clay:capabilitychange",G),f?document.documentElement.setAttribute("draftmode","true"):document.documentElement.removeAttribute("draftmode")})}function A(){Q(),De()}A();var Me={init:A,setMode:y,toggleEditMode:X,isEditMode:u,isOwner:v,canEdit:a,mode:n};var se={};var Z=typeof navigator<"u"&&/Mac|iPhone|iPad/.test(navigator.platform||navigator.userAgent),Ne=Z?"\u2318\u21E7E":"Ctrl+Shift+E",Ae=`.clay-mode-note {
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
}`,ee=new WeakSet;function te(e){import("./c-B4VYNXTQ.js").then(({mountEditorUI:t})=>{ee.has(e)||t({name:"mode-note",content:e,css:[Ae]})})}function oe(e){ee.add(e),e.getRootNode().host?.remove()}function ne(e){return y(e).then(t=>{t.ok||import("./c-QHIWXHRO.js").then(({modeRefusalText:o})=>{let i=o(t);i&&(typeof window.clay?.toast=="function"?window.clay.toast(i,"error"):typeof window.clay?.menu?.note=="function"?window.clay.menu.note(i):ae(i))})})}function Ce(e){(a||n==="edit")&&(!(Z?e.metaKey:e.ctrlKey)||!e.shiftKey||e.altKey||e.code!=="KeyE"||e.isComposing||(e.preventDefault(),ne(n==="edit"?"view":"edit")))}var Re="In view mode, so edits won't be saved.",Pe=":is(input, textarea, select):is([persist], [clay-persist])",R="[editmode\\:contenteditable], [viewmode\\:readonly], [viewmode\\:disabled]",s=null;function ie(e){if(!e.isConnected)return!1;let t=U(e);return t.persist===q.FULL&&!t.extension}function re(e){let t=e?.nodeType===1?e:e?.parentElement;if(!t)return null;let o=t.closest(Pe)||t.closest(R)||(t.isContentEditable?t:null);return o&&ie(o)?o:null}var Ie=e=>e.isContentEditable||e.matches("input, textarea, select")&&!e.readOnly&&!e.disabled,P=()=>a&&n==="view";function Oe(e){P()&&re(e.target)&&I()}function _e(e){if(!P()||e.defaultPrevented||e.isComposing||e.ctrlKey||e.metaKey||e.altKey||!(e.key.length===1&&e.key!==" ")&&e.key!=="Backspace"&&e.key!=="Delete")return;let t=e.target;if(t?.nodeType!==1||Ie(t)||t!==document.body&&!t.closest(R))return;re(t===document.body?document.getSelection()?.anchorNode:t)?.matches(R)&&I()}function Ke(e){if(!P())return;let t=e.target?.closest?.("[viewmode\\:disabled]");t&&ie(t)&&I()}function I(){s||(console.warn('clayjs: this tab is in view mode, so edits made here are not saved. clay.setMode("edit") switches; a page that must edit as soon as it opens declares <html opens-in="edit">.'),document.querySelector('clay-editor-ui[data-clay-ui="mode-note"]')?.remove(),s=document.createElement("div"),s.setAttribute("clay","editor-ui"),s.setAttribute("role","alert"),s.className="clay-mode-note clay-view-notice",s.innerHTML='<span></span><button type="button" class="clay-view-notice__edit">Switch to edit</button>',s.querySelector("span").textContent=Re,s.querySelector("button").addEventListener("click",()=>ne("edit")),te(s))}function je(){n==="view"||!s||(oe(s),s=null)}function ae(e){let t=document.createElement("div");t.setAttribute("clay","editor-ui"),t.setAttribute("role","status"),t.className="clay-mode-note",t.innerHTML=`<span>This page opens in view mode. Press <kbd>${Ne}</kbd> to edit.</span><button type="button" aria-label="Dismiss">\xD7</button>`,e&&(t.querySelector("span").textContent=e);let o=()=>oe(t);t.querySelector("button").addEventListener("click",o),addEventListener("keydown",i=>{i.key==="Escape"&&o()},{once:!0}),setTimeout(o,12e3),te(t)}f||(document.addEventListener("keydown",Ce),document.addEventListener("input",Oe,!0),document.addEventListener("pointerdown",Ke,!0),window.addEventListener("keydown",_e),document.addEventListener("clay:modechange",je),m(()=>{if(!a||n!=="view"||document.querySelector("[clay-mode-toggle]"))return;(t=>{let o=window.clay?.ready;window.clay?.storage||typeof o?.then!="function"?queueMicrotask(t):o.then(t,()=>{})})(()=>{if(window.clay?.menu||window.clay?.plugins?.includes?.("menu"))return;let t=window.clay?.storage?.local;!t||t.get("mode-note")||(t.set("mode-note",!0),ae())})}));var O={};S(O,{whenShown:()=>Ve});var He="This page can't be edited: the app serving it is out of date. Update HTML Clay to 1.9.0 or newer.",d=null,r=null;function h(){if(!d)return;let e=window.visualViewport,t=e?Math.max(0,window.innerHeight-e.height-e.offsetTop):0;x(d,"bottom",`calc(${16+t}px + env(safe-area-inset-bottom,0px))`)}function $e(){d&&(x(d,"display","none"),window.visualViewport?.removeEventListener("resize",h),window.visualViewport?.removeEventListener("scroll",h))}function qe(){let e=r.resolveTheme();d=r.bevelSurface("div",["position:fixed","left:50%","transform:translateX(-50%)","z-index:2147483001","display:flex","align-items:center","gap:10px 12px","width:max-content","max-width:calc(100vw - 24px)","flex-wrap:wrap","padding:8px 8px 8px 14px","text-align:left",`color-scheme:${r.pageScheme()}`],{theme:e,parts:["notice.root","notice.panel","staleHost.root","staleHost.panel"]}),d.setAttribute("clay",r.RUNTIME_ONLY),d.setAttribute("data-clay-stale-host",""),d.setAttribute("role","alert");let t=B("span",["all:initial","color-scheme:inherit","display:inline-block","flex:none","width:8px","height:8px","border-radius:50%",`background:${r.TOKENS.ox}`]);t.setAttribute("clay",r.RUNTIME_ONLY);let o=["notice.dot","staleHost.dot"];r.themed(e)&&(r.markParts(t,e,o),W(t,r.partRules(e,o,{})));let i=r.bevelButton("Dismiss",{small:!0,variant:"quiet",onClick:$e,theme:e,parts:["notice.button","staleHost.button"],labelParts:["notice.buttonLabel","staleHost.buttonLabel"]});i.setAttribute("aria-label","Dismiss this message"),d.append(t,r.bevelText("span",[],He,{theme:e,parts:["notice.text","staleHost.text"]}),i),document.body.appendChild(d),h(),r.themed(e)?x(d,"color-scheme",e.scheme==="light dark"?"light":e.scheme):r.paintLook(d,{background:r.TOKENS.surface}),window.visualViewport?.addEventListener("resize",h),window.visualViewport?.addEventListener("scroll",h)}async function Ue(){if(!H()||u)return;let[e,t,o,i]=await Promise.all([import("./c-33SS5A4Y.js"),import("./c-46LJAZZJ.js"),import("./c-OCSZ4QGN.js"),import("./c-ELXMSEN5.js")]);r={...e,...i,TOKENS:t.TOKENS,resolveTheme:o.resolveTheme},qe()}var de=Promise.resolve();m(()=>{de=Ue()});var Ve=()=>de;var K={};S(K,{createPageDataApi:()=>ue,readData:()=>Je,setPageDataLiveSync:()=>Xe,writeData:()=>Ye});var Fe="data-read",ze="data-write";function E(e,t,o){let i=new Error(t);return i.code=e,i.details=o,i}async function le(e){if(typeof e.json=="function")return e.json();let t=await e.text();return t?JSON.parse(t):null}async function We(e){let t={};try{t=await le(e)||{}}catch{}let o=t.message||t.error||e.statusText||`Request failed (${e.status})`,i=new Error(o);return i.status=e.status,i.error=t.error,i.details=t.details,i}function Be(e){let t=e.headers?.get?.("ETag");return typeof t=="string"&&t?t:null}function Ge(e,t){let o=`${t.origin}${t.pathname}`;return e!==void 0?`${o}?data=${encodeURIComponent(JSON.stringify(e))}`:`${t.origin}/_/api${t.pathname}`}async function ce(){let{isDraftMode:e}=await import("./core/is-edit-mode-73OXKK5H.js");return e?E("DataUnavailableInDraft","Page data is kept in this browser for this visitor."):null}function ue({supports:e=z,token:t=j,fetchImpl:o=(...g)=>fetch(...g),getLocation:i=()=>window.location,eventTarget:fe=document,loadWriteModules:me,syncTimeoutMs:pe,httpTimeoutMs:we}={}){let g=null,w=null;function ye(c){g=c}async function he(c){let b=await ce();if(b)throw b;if(!await e(Fe))throw E("DataReadUnsupported","This host does not support reading page data.");let l;try{l=await o(Ge(c,i()),{method:"GET",credentials:"same-origin",cache:"no-store",redirect:"error"})}catch(k){throw Object.assign(E("DataReadFailed",k?.message||"Could not read page data."),{cause:k})}if(!l.ok)throw await We(l);return{data:await le(l),etag:Be(l)}}function ge(){return w||(w=import("./c-34J5HC7N.js").then(({createPageDataWrite:c})=>c({supports:e,token:t,fetchImpl:o,getLocation:i,eventTarget:fe,getLiveSync:()=>g,loadWriteModules:me,syncTimeoutMs:pe,httpTimeoutMs:we})).catch(c=>{throw w=null,c}),w)}async function be(c,b){let l=await ce();if(l)throw l;if(!await e(ze))throw E("DataWriteUnsupported","This host does not support writing page data.");return(await ge()).writeData(c,b)}return{readData:he,writeData:be,setLiveSync:ye}}var _=ue(),Je=_.readData,Ye=_.writeData,Xe=_.setLiveSync;var ut={"lib/mutation.js":V,"core/preserve-authored.js":F,"core/edit-mode.js":C,"core/mode-key.js":se,"core/stale-host-notice.js":O,"core/page-data.js":K};export{ut as core};
