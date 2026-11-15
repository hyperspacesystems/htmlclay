import{b as qe,e as Jr}from"../c-JAX2667S.js";import{a as Xr}from"../c-KD2ME5UN.js";import"../c-J2ZW4HMR.js";import"../c-A5LASUEZ.js";import"../c-4JZDZFF6.js";import"../c-5RZNFTWD.js";import"../c-PKCCHJ5X.js";import"../c-NUHP2XDB.js";import"../c-VHNSBD2M.js";var Kr=`@layer clay-skin{
:is(.hcms-shell):not([data-theme]),:is(.hcms-shell):not([data-theme]) *{--mirk-canvas:light-dark(#FDF8F0, #0B0C13) !important;--mirk-bg:light-dark(#FFFCF6, #11131E) !important;--mirk-fg:light-dark(#2B241B, #ECEAF2) !important;--mirk-accent:light-dark(#F4ECDF, #1A1D2C) !important;--mirk-destructive:light-dark(#A83A2C, #FF5566) !important;--mirk-focus-color:light-dark(#8F6310, #D9A445) !important;--mirk-bevel-bg:light-dark(#F1E7D4, #22273E) !important;--mirk-bevel-fg:light-dark(#2B241B, #ECEAF2) !important;--mirk-bevel-tl:light-dark(#FFF9ED, #3C4260) !important;--mirk-bevel-br:light-dark(#D6C3A5, #1B2033) !important;--mirk-bevel-hover-bg:color-mix(in srgb, light-dark(#F1E7D4, #22273E), light-dark(#FFF9ED, #3C4260) 45%) !important;--mirk-pill-inner-top:light-dark(#FFF9ED, #3C4260) !important;--mirk-input-border:light-dark(#D8C8AF, #353B52) !important;--mirk-placeholder-color:light-dark(#9A8A70, #6A7090) !important;--mirk-ctrl-bg:light-dark(#6B5942, #8A90AB) !important;--mirk-toggle-bg:light-dark(#FCF8F1, #4A5275) !important;--mirk-toggle-hi:light-dark(#FFF9ED, #6C759A) !important;--mirk-toggle-lo:light-dark(#D6C3A5, #1B2033) !important;--mirk-mark-fg:light-dark(#2B241B, #ECEAF2) !important;--mirk-sortable-dot:light-dark(#D8C8AF, #353B52) !important;--mirk-sortable-shadow:light-dark(#D6C3A5, #1B2033) !important;--mirk-sortable-label:light-dark(#6B5942, #8A90AB) !important;--mirk-sortable-placeholder:light-dark(#9A8A70, #6A7090) !important;--mirk-slider-fill:light-dark(#F0E4CC, #3A2F17) !important;--mirk-slider-nub-bg:light-dark(#FCF8F1, #4A5275) !important;--mirk-slider-nub-hi:light-dark(#FFF9ED, #6C759A) !important;--mirk-slider-nub-lo:light-dark(#D6C3A5, #1B2033) !important;--mirk-chip-surface:light-dark(#FFFCF6, #11131E) !important;--mirk-chip-edge:light-dark(#D8C8AF, #353B52) !important;--mirk-chip-primary-bg:light-dark(#2B241B, #ECEAF2) !important;--mirk-chip-primary-fg:light-dark(#FDF8F0, #0B0C13) !important;--mirk-chip-alert:light-dark(#A83A2C, #FF5566) !important;--mirk-radius:0px !important;--mirk-focus-offset:2px !important}
:is(.hcms-shell){font-family:ui-monospace,SFMono-Regular,Menlo,Consolas,monospace !important}
.hcms-shell .mirk-chip__actions .mirk-chip__action--primary{--mirk-bevel-bg:var(--mirk-chip-primary-bg) !important;--mirk-bevel-fg:var(--mirk-chip-primary-fg) !important;--mirk-bevel-tl:color-mix(in srgb, var(--mirk-chip-primary-bg), white 24%) !important;--mirk-bevel-br:color-mix(in srgb, var(--mirk-chip-primary-bg), black 32%) !important;--mirk-bevel-hover-bg:color-mix(in srgb, var(--mirk-chip-primary-bg), white 10%) !important}
}`;var ta=Object.defineProperty,or=(e,t)=>{for(var r in t)ta(e,r,{get:t[r],enumerable:!0})},Vi={};or(Vi,{cms:()=>Dr,default:()=>Lh});var Wi=["textContent","innerText","innerHTML","outerHTML","value","checked","selected","disabled","readOnly","type","tagName","nodeName","nodeType","nodeValue","childElementCount","id","className","classList","baseURI","offsetWidth","offsetHeight","clientWidth","clientHeight","scrollWidth","scrollHeight","dataset","currentSrc","duration","paused","title","documentURI","contentType"],ra=new Set(Wi),ia=new Set(["textContent","innerText","innerHTML","value","checked","selected","disabled","readOnly","type","id","className","title"]),Gi=new Set(["tagName","nodeName","nodeType","nodeValue","childElementCount","classList","baseURI","documentURI","contentType","offsetWidth","offsetHeight","clientWidth","clientHeight","scrollWidth","scrollHeight","currentSrc","duration","paused","dataset"]),Zr=new Set(["checked","selected","disabled","readOnly","paused"]);function la(e,t){return t==null?Zr.has(e)?!1:"":Zr.has(e)?t==="false"?!1:!!t:t}var Ji={};or(Ji,{CodecRefused:()=>ha,EmptyListInsert:()=>Me,MAX_RULE_DEPTH:()=>He,MaxRuleDepthExceeded:()=>sr,NoRulesTag:()=>sa,RuleTargetReadOnly:()=>Zi,RulesParseError:()=>be,ShapeMismatch:()=>Ki,UnknownCodec:()=>Qi,UnknownRulesVersion:()=>Xi,WriteNotRoundTripped:()=>oa,WriteRefused:()=>na,WriteRejected:()=>ca});var be=class extends Error{constructor(e,t){super(e),this.name="RulesParseError",this.cause=t}},Xi=class extends Error{constructor(e){super(`unknown rules version: ${e}. Library supports "1" and "2".`),this.name="UnknownRulesVersion",this.version=e}},He=20,sr=class extends Error{constructor(e){super(`rule depth exceeded ${He} at path: ${e.join(".")}`),this.name="MaxRuleDepthExceeded",this.path=e}},Ki=class extends Error{constructor(e){super(`shape mismatch: ${e.length} field(s) failed validation`),this.name="ShapeMismatch",this.mismatches=e}},Me=class extends Error{constructor(e,t){super(t?`cannot add items to empty list at "${e.join(".")}" \u2014 ${t}`:`cannot add items to empty list at "${e.join(".")}" \u2014 no sibling to clone as template. Seed the list with a hidden item first.`),this.name="EmptyListInsert",this.path=e}},Zi=class extends Error{constructor(e){super(`cannot write to read-only DOM property "${e}"`),this.name="RuleTargetReadOnly",this.target=e}},na=class extends Error{constructor(e){super(`write refused: ${e.length} write(s) broke the content-only policy`),this.name="WriteRefused",this.refusals=e}};function aa(e){return e.reduce((t,r)=>typeof r=="number"?`${t}[${r}]`:t?`${t}.${r}`:String(r),"")||"(root)"}var oa=class extends Error{constructor(e){super(`write did not round-trip at "${aa(e)}"`),this.name="WriteNotRoundTripped",this.path=e}},sa=class extends Error{constructor(e){super(`no <script data-rules-name~="${e}"> rules tag in this document`),this.name="NoRulesTag",this.token=e}},ca=class extends Error{constructor(e,t){super(`write rejected: ${e.length} unknown key(s), ${t.length} rule(s) with no matching element`),this.name="WriteRejected",this.unknownKeys=e,this.unmatched=t}},Qi=class extends Error{constructor(e){super(`unknown codec "${e}": no codec by that name is given to this call`),this.name="UnknownCodec",this.code="unknown-codec",this.codec=e}},ha=class extends Error{constructor(e,t){super(t),this.name="CodecRefused",this.code=e}};function I(e){let t=0,r=null,i=-1;for(let l=0;l<e.length;l++){let n=e[l];n==="\\"?l++:r?n===r&&(r=null):n==='"'||n==="'"?r=n:n==="["||n==="("?t++:n==="]"||n===")"?t>0&&t--:n==="@"&&t===0&&(i=l)}return i}var Qr="[*]";function ke(e){let t=I(e),r=t===-1?e:e.slice(0,t),i=r.endsWith(Qr),l={selector:i?r.slice(0,-Qr.length):r,prop:t===-1?null:e.slice(t+1)||null};return i&&(l.all=!0),l}function Yi(e){let t=0,r=null,i=-1,l=!1;for(let n=0;n<e.length;n++){let a=e[n];a==="\\"?n++:r?a===r&&(r=null):a==='"'||a==="'"?r=a:a==="["||a==="("?t++:a==="]"||a===")"?t>0&&t--:t===0&&a===">"?i=n:t===0&&a===","&&(l=!0)}return l||i===-1||!da(e,i+1)?null:e.slice(0,i).trim()||null}function da(e,t){let r=t,i=e.replace(/\s+$/,"").length;for(;r<i&&/\s/.test(e[r]);)r++;let l=0,n=null;for(;r<i;r++){let a=e[r];if(a==="\\")r++;else if(n)a===n&&(n=null);else if(a==='"'||a==="'")n=a;else if(a==="["||a==="(")l++;else if(a==="]"||a===")")l>0&&l--;else if(l===0&&(a==="+"||a==="~"||/\s/.test(a)))return!1}return!0}var ua=/^codec\(([a-z][a-z0-9-]*)\)$/;function cr(e){if(typeof e!="string")return null;let t=I(e);if(t===-1)return null;let r=ua.exec(e.slice(t+1));if(!r)return null;let{selector:i,all:l}=ke(e);if(l)throw new be("A @codec(name) rule cannot use [*].");return{selector:i,name:r[1]}}var ma=/^--[A-Za-z0-9_-]+$/;function el(e){return typeof e=="string"&&ma.test(e)}function tl(e){let t=[],r=[],i=null,l=0;for(let n=0;n<e.length;n++){let a=e[n];if(i){a==="\\"?n++:a===i&&(i=null);continue}if(a==="/"&&e[n+1]==="*"){let o=e.indexOf("*/",n+2);n=o===-1?e.length:o+1;continue}a==='"'||a==="'"?i=a:a==="("?r.push(")"):a==="["?r.push("]"):a==="{"?r.push("}"):(a===")"||a==="]"||a==="}")&&r[r.length-1]===a?r.pop():a===";"&&r.length===0&&(t.push(e.slice(l,n)),l=n+1)}return t.push(e.slice(l)),t}function pa(e){let t="",r=null;for(let i=0;i<e.length;i++){let l=e[i];if(r){t+=l,l==="\\"&&i+1<e.length?t+=e[++i]:l===r&&(r=null);continue}if(l==="/"&&e[i+1]==="*"){let n=e.indexOf("*/",i+2);i=n===-1?e.length:n+1,t+=" ";continue}(l==='"'||l==="'")&&(r=l),t+=l}return t}function rl(e){e=pa(e);let t=e.indexOf(":");if(t===-1)return null;let r=e.slice(0,t).trim(),i=e.slice(t+1).trim(),l=/!\s*important\s*$/i.exec(i);return l&&(i=i.slice(0,l.index).trim()),{name:r,value:i,important:!!l}}function il(e,t){if(e==null)return null;let r=null;for(let i of tl(String(e))){let l=rl(i);!l||l.name!==t||(!r||l.important||!r.important)&&(r=l)}return r?r.value:null}function fa(e,t,r){let i=e==null?"":String(e),l=r==null||r==="",n=tl(i),a=n.map(rl),o=a.some(p=>p&&p.name===t&&p.important),s=[],h=!1;n.forEach((p,m)=>{let u=a[m];if(!u||u.name!==t){s.push(p);return}if(l||h)return;let c=/^\s*/.exec(p)[0];s.push(`${c}${t}: ${r}${o?" !important":""}`),h=!0});let d=s.join(";");if(!l&&!h){let p=d.trimEnd();d=p===""?`${t}: ${r}`:`${p}${p.endsWith(";")?" ":"; "}${t}: ${r}`}return d.trim()===""?null:d}function ga(e,t,r,i){let l=e.attr(t,"style");if((il(l,r)??"")===i)return;let n=fa(l,r,i);n==null?e.removeAttr(t,"style"):e.attr(t,"style",n)}var Yr=/^[a-z][a-z0-9-]*$/,hr=new Map;function ba(e,t){if(typeof e!="string"||!Yr.test(e))throw new Error(`hyper-html-api: invalid codec name ${JSON.stringify(e)} (must match ${Yr})`);if(!t||typeof t.read!="function"||typeof t.write!="function"||typeof t.same!="function")throw new Error(`hyper-html-api: codec "${e}" needs read, write and same functions`);hr.set(e,t)}function ka(e){return hr.get(e)??null}function va(){return Object.fromEntries(hr)}var ya='The @codec(name) rule form needs a rules tag with data-rules-version="2".',xa="A @codec(name) rule must be the value of a top level key.";function ll(e,t){let r=cr(e);return r?{...r,codec:t.codecs[r.name]}:null}function nl(e,t={}){let r=t.version??"1",i=t.codecs??{};l(e,0);function l(n,a){if(!(a>He)){if(typeof n=="string"){let o=cr(n);if(!o)return;if(r!=="2")throw new be(ya);if(a!==1)throw new be(xa);if(!Object.prototype.hasOwnProperty.call(i,o.name))throw new Qi(o.name);return}if(Array.isArray(n)){l(n[1],a+1);return}if(n&&typeof n=="object")for(let o of Object.values(n))l(o,a+1)}}}function Ht(e,t,r,i={}){if(Object.prototype.hasOwnProperty.call(i,"exclude")&&e.semanticExclude===!1)throw new Error("This adapter does not support semantic exclude queries; omit exclude or use the DOM adapter.");return nl(r,i),Ut(e,t,r,{depth:0,path:[]},i)}function Ut(e,t,r,i,l){if(i.depth>He)throw new sr(i.path);if(typeof r=="string")return wa(e,t,r,i,l);if(Array.isArray(r)){let[n,a]=r,o=e.find(t,n,l);return al(l,i,o),o.map((s,h)=>Ut(e,s,a,{depth:i.depth+1,path:[...i.path,h]},l))}if(typeof r=="object"&&r!==null){let n={};for(let[a,o]of Object.entries(r))n[a]=Ut(e,t,o,{depth:i.depth+1,path:[...i.path,a]},l);return n}return null}function wa(e,t,r,i,l){if(r.endsWith("[]")){let d=r.slice(0,-2),p=e.find(t,d,l);return al(l,i,p),p.map(m=>e.text(m))}let n=ll(r,l);if(n)return Aa(e,t,n,l);if(r.startsWith("@"))return qt(e,t,r.slice(1));let a=I(r);if(a!==-1){let{selector:d,all:p}=ke(r),m=r.slice(a+1),u=d?e.find(t,d,l):[t];return p&&l.allMatches?u.map(c=>qt(e,c,m)):u.length===0?null:qt(e,u[0],m)}if(r===".")return e.text(t);let{selector:o,all:s}=ke(r),h=e.find(t,o,l);return s&&l.allMatches?h.map(d=>e.text(d)):h.length===0?null:e.text(h[0])}function Aa(e,t,{selector:r,codec:i},l){let n=r?e.find(t,r,l):[t];return n.length===0?null:i.read(e,n[0],{query:l.query,view:l.view,region:l.region})}function al(e,t,r){if(typeof e.onRowsRead=="function")try{e.onRowsRead(t.path.slice(),r)}catch(i){console.warn(`[hyper-html-api] onRowsRead threw at "${t.path.join(".")||"(root)"}"`,i)}}function qt(e,t,r){if(el(r))return il(e.attr(t,"style"),r);if(ra.has(r)){let l=e.prop(t,r);return l==null?null:String(l)}return e.attr(t,r)??null}var dr=Object.freeze({data:Object.freeze({tokens:["no-data"],bundles:["editor-ui"]}),save:Object.freeze({tokens:["no-save"],bundles:["editor-ui"]}),snapshot:Object.freeze({tokens:["no-snapshot"],bundles:["editor-ui"]}),watch:Object.freeze({tokens:["no-watch"],bundles:["editor-ui"]}),undo:Object.freeze({tokens:["no-undo"],bundles:["editor-ui"]}),history:Object.freeze({tokens:[],bundles:["editor-ui"]})}),_a=Object.freeze({"editor-ui":Object.freeze(["no-data","no-save","no-snapshot","no-watch","no-undo"])}),Ea=Object.freeze(["no-save","no-snapshot","no-trigger-autosave","no-dirty","no-watch","no-undo","no-data","freeze","editor-ui","live"]);function lt(e,t){if(!e||e.nodeType!==1)return!1;let r=e.getAttribute?.("clay");return!!(r&&r.split(/\s+/).includes(t)||e.hasAttribute?.(t))}function ol(e,t){if(lt(e,t))return!0;for(let[r,i]of Object.entries(_a))if(i.includes(t)&&lt(e,r))return!0;return!1}function Vt(e){let t=dr[e];if(!t)throw new Error(`Unknown region capability: ${e}`);return[...t.tokens,...t.bundles].flatMap(r=>[`[clay~="${r}"]`,`[${r}]`]).join(", ")}function Sa(e,t){let r=e&&e.nodeType===1?e:e?.parentElement;for(;r&&r.nodeType===1;){let i=dr[t];if(!i)throw new Error(`Unknown region capability: ${t}`);if(i.tokens.some(l=>ol(r,l))||i.bundles.some(l=>lt(r,l)))return r;r=r.parentElement}return null}function Ca(e,t){return!!Sa(e,t)}var Ta=/:(?:focus(?:-within|-visible)?|hover|active|visited|defined)\b/i;function Ra(e,t){if(!/^(INPUT|TEXTAREA|SELECT|OPTION)$/.test(e.tagName||""))return;let r=(e.getAttribute?.("type")||"").toLowerCase();"value"in e&&"value"in t&&e.tagName!=="OPTION"&&r!=="checkbox"&&r!=="radio"&&r!=="file"&&(t.value=e.value),e.tagName==="INPUT"&&(r==="checkbox"||r==="radio")&&(t.checked=e.checked),e.tagName==="OPTION"&&(t.selected=e.selected),"indeterminate"in e&&"indeterminate"in t&&(t.indeterminate=e.indeterminate)}function ei(e,t,r,i){if(i)return!!e.closest?.(t);let l=e;for(;l?.nodeType===1;){if(l.matches(t))return!0;if(l===r)break;l=l.parentElement}return!1}function ti(e,t,r,i,l,n){return(n?Ca(e,t):ei(e,r,l,!1))||!!(i&&(n&&e!==l?e.matches(i):ei(e,i,l,n)))}function qa(e,t){let r=dr[t];return r.tokens.some(i=>ol(e,i))||r.bundles.some(i=>lt(e,i))}var Oa=new Set(["INPUT","TEXTAREA","SELECT","OPTION"]);function sl(e,t,r,i){if(e.nodeType===1&&i(e)){r.dropped.push(t);return}r.cloneToLive.set(t,e),r.liveToClone.set(e,t);let l=e,n=t;if(e.nodeType===1){if(Oa.has(e.tagName)&&r.controls.push(e,t),e.tagName==="TEMPLATE"){for(;t.firstChild;)t.removeChild(t.firstChild);l=e.content,n=t.content,r.cloneToLive.set(n,l),r.liveToClone.set(l,n)}}else if(e.nodeType!==11)return;let a=n.firstChild;for(let o=l.firstChild;o&&a;o=o.nextSibling)sl(o,a,r,i),a=a.nextSibling}function Ge(e,{capability:t="data",exclude:r=null,inherit:i=!0,provenance:l=null}={}){if(!e)throw new TypeError("createContentView requires a DOM context");let n=e.nodeType===9?e:e.ownerDocument;if(!n?.implementation?.createHTMLDocument)throw new TypeError("createContentView requires an HTML DOM implementation");let a=n.implementation.createHTMLDocument(""),o=l||new WeakMap,s=new WeakMap,h=e.nodeType===9?e.documentElement:e;r&&a.documentElement.matches(r);let d=Vt(t),p=i?c=>qa(c,t)||!!(r&&c.matches(r)):c=>ti(c,t,d,r,h,!1),m=null;if(h.nodeType!==1||!ti(h,t,d,r,h,i)){m=a.importNode(h,!0);let c={cloneToLive:o,liveToClone:s,dropped:[],controls:[]};sl(h,m,c,p);for(let f of c.dropped)f.remove();for(let f=0;f<c.controls.length;f+=2)Ra(c.controls[f],c.controls[f+1])}e.nodeType===9&&m&&(a.replaceChild(m,a.documentElement),s.set(e,a),o.set(a,e));let u=c=>{if(Ta.test(c))throw new Error(`Filtered content queries do not support stateful selector: ${c}`)};return{root:m,document:a,capability:t,selector:Vt(t),cloneToLive:o,liveToClone:s,original(c){return o.get(c)||null},cloneOf(c){return s.get(c)||null},query(c,f=m){return u(c),Array.from(f.querySelectorAll(c),b=>o.get(b)||b)},text(c=m){return(c===m?m:s.get(c))?.textContent||""},html(c=m){return(c===m?m:s.get(c))?.innerHTML??""},clone(c=m){let f=c===m?m:s.get(c);return f?a.importNode(f,!0):null}}}function cl(e){return e&&e.nodeType===1&&e.tagName==="SCRIPT"&&e.hasAttribute&&e.hasAttribute("data-rules-name")}function $a(e){return e?(e.nodeType===9||e.nodeType,e):null}function ri(e,t,r){!e||e.nodeType!==1||(t==="value"&&e.tagName==="INPUT"?e.value=r===null?"":r:t==="checked"&&e.tagName==="INPUT"?e.checked=r!==null:t==="selected"&&e.tagName==="OPTION"&&(e.selected=r!==null))}function hl(e,t){e&&e.tagName==="TEXTAREA"&&(e.value=t)}function dl(e){if(!e||e.nodeType!==1)return;let t=[e,...e.querySelectorAll?e.querySelectorAll("textarea, input, option"):[]];for(let r of t)if(r.tagName==="TEXTAREA")r.value=r.defaultValue;else if(r.tagName==="INPUT"){let i=r.type;i!=="file"&&i!=="checkbox"&&i!=="radio"&&(r.value=r.defaultValue),r.checked=r.defaultChecked}else r.tagName==="OPTION"&&(r.selected=r.defaultSelected)}var F={semanticExclude:!0,find(e,t,r={}){let i=$a(e);if(!i||!i.querySelectorAll)return[];let l=Array.from(i.querySelectorAll(t));r.includeRulesTag||(l=l.filter(o=>!cl(o)));let n=[];r.skip&&n.push(r.skip);let a=r.templateAttr===null?null:r.templateAttr||"cms-template";if(a&&n.push("["+a+"]"),n.length){let o=n.join(", ");l=l.filter(s=>!s.closest||!s.closest(o))}return l},parent(e){return e?e.parentElement:null},children(e){return e?Array.from(e.children):[]},matches(e,t){return!!(e&&e.matches)&&e.matches(t)},text(e,t){if(t===void 0)return(e.textContent||"").trim();e.textContent=t,hl(e,t)},attr(e,t,r){if(r===void 0)return e.hasAttribute&&e.hasAttribute(t)?e.getAttribute(t):null;e.setAttribute(t,r),ri(e,t,r)},removeAttr(e,t){e&&e.removeAttribute&&e.removeAttribute(t),ri(e,t,null)},prop(e,t,r){if(r===void 0){let i=e?e[t]:void 0;return i!==void 0?i:null}e[t]=r},clone(e){let t=e.cloneNode(!0);return dl(t),t},insertAt(e,t,r){let i=e.children[r]||null;e.insertBefore(t,i)},insertBefore(e,t){e.parentNode.insertBefore(t,e)},insertAfter(e,t){e.parentNode.insertBefore(t,e.nextSibling)},identity(e){return e||null},remove(e){e&&e.parentNode&&e.parentNode.removeChild(e)},replaceWith(e,t){if(!e||!e.parentNode)throw new Error("dom.replaceWith: node has no parent");let r=e.ownerDocument.createElement("template");r.innerHTML=t;let i=r.content.firstElementChild;if(!i)throw new Error("dom.replaceWith: html did not parse to an element");return e.parentNode.replaceChild(i,e),i},stripIds(e){let t=0;return e.id&&(e.removeAttribute("id"),t++),(e.querySelectorAll?e.querySelectorAll("[id]"):[]).forEach(r=>{r.removeAttribute("id"),t++}),t},sameNode(e,t){return e===t}};function pe(e,t={}){if(t.exclude===null)return F;let r=[Vt("data"),t.exclude].filter(Boolean).join(", "),i=e.nodeType===9?e:e.ownerDocument||e,l=e.nodeType===9?e.documentElement:e;if(!t._write&&!l?.matches?.(r)&&!l?.querySelector?.(r)&&!i?.querySelector?.(r))return F;let n=e.nodeType===9||!e.isConnected?e:e.ownerDocument,a=e.nodeType===9?e:e.ownerDocument,o=null,s=()=>o||(o=Ge(n,{capability:"data",exclude:t.exclude||null})),h=c=>{let f=s();return c===e?f.cloneOf(e):f.cloneOf(c)||c},d=c=>o?.original(c)||c,p=(c,f)=>{s().liveToClone.set(c,f),s().cloneToLive.set(f,c);let b=c.childNodes||[],g=f.childNodes||[];for(let k=0;k<Math.min(b.length,g.length);k++)p(b[k],g[k])},m=(c,f,b,g)=>{let k=Array.from(c.childNodes).filter(E=>o.cloneOf(E)),y=k[0];for(;y&&k.includes(y);)y=y.nextSibling;for(let E of k)E.remove();if(b==="innerHTML"){let E=a.createElement("template");E.innerHTML=g;for(let O of Array.from(E.content.childNodes))c.insertBefore(O,y||null)}else c.insertBefore(a.createTextNode(g),y||null),hl(c,g);let _=Ge(c,{capability:"data",exclude:t.exclude||null});f.replaceChildren(...Array.from(_.root?.childNodes||[]));let A=E=>{let O=_.original(E);O&&(o.liveToClone.set(O,E),o.cloneToLive.set(E,O));for(let H of Array.from(E.childNodes||[]))A(H)};for(let E of Array.from(f.childNodes))A(E)},u={...F,find(c,f,b={}){let g=h(c);if(!g?.querySelectorAll)return[];let k=s().query(f,g);b.includeRulesTag||(k=k.filter(A=>!cl(A)));let y=[];b.skip&&y.push(b.skip);let _=b.templateAttr===null?null:b.templateAttr||"cms-template";if(_&&y.push(`[${_}]`),y.length){let A=y.join(", ");k=k.filter(E=>!E.closest?.(A))}return k},parent(c){let f=h(c)?.parentElement;return f?d(f):null},children(c){return Array.from(h(c)?.children||[],d)},text(c,f){if(f===void 0){let k=d(c);return!k?.matches?.(r)&&!k?.querySelector?.(r)?F.text(k):(h(c)?.textContent||"").trim()}let b=d(c);if(!b?.matches?.(r)&&!b?.querySelector?.(r)&&!o){F.text(b,f);return}let g=s().cloneOf(b);g?m(b,g,"textContent",f):F.text(b,f)},attr(c,f,b){let g=d(c);if(b===void 0)return F.attr(g,f);F.attr(g,f,b);let k=o?.cloneOf(g);k&&(F.attr(k,f,b),Ge(g,{capability:"data",exclude:t.exclude||null}).root===null&&(k.remove(),o.liveToClone.delete(g),o.cloneToLive.delete(k)))},removeAttr(c,f){let b=d(c);F.removeAttr(b,f);let g=o?.cloneOf(b);g&&F.removeAttr(g,f)},prop(c,f,b){let g=d(c);if(b===void 0)return f==="innerHTML"||f==="outerHTML"||f==="textContent"||f==="innerText"?F.prop(h(g),f):F.prop(g,f);let k=f==="innerHTML"||f==="textContent"||f==="innerText",y=k?s().cloneOf(g):o?.cloneOf(g);y&&k?m(g,y,f,b):(F.prop(g,f,b),y&&F.prop(y,f,b))},clone(c){let f=h(c);if(f===c)return F.clone(c);let b=a.importNode(f,!0);return dl(b),b},insertAt(c,f,b){let g=d(c),k=u.children(g)[b]||(()=>{for(let A=g.children.length-1;A>=0;A--){let E=g.children[A];if(s().cloneOf(E))return E.nextElementSibling}return g.firstElementChild})(),y=d(f);g.insertBefore(y,k||null);let _=s().cloneOf(g);if(_){let A=s().cloneOf(y);A||(A=a.importNode(y,!0),p(y,A));let E=Array.from(_.children);_.insertBefore(A,E[b]||null)}},insertBefore(c,f){let b=d(c),g=d(f);b.parentNode.insertBefore(g,b);let k=s().cloneOf(b.parentNode);if(k){let y=s().cloneOf(g);y||(y=a.importNode(g,!0),p(g,y));let _=s().cloneOf(b);_?_.before(y):k.appendChild(y)}},insertAfter(c,f){let b=d(c),g=d(f);b.parentNode.insertBefore(g,b.nextSibling);let k=s().cloneOf(b.parentNode);if(k){let y=s().cloneOf(g);y||(y=a.importNode(g,!0),p(g,y));let _=s().cloneOf(b);_?_.after(y):k.appendChild(y)}},identity(c){return d(c)||null},remove(c){let f=d(c),b=s().cloneOf(f);F.remove(f),b&&F.remove(b)},replaceWith(c,f){let b=d(c),g=s().cloneOf(b),k=F.replaceWith(b,f);if(g){let y=Ge(k,{capability:"data",exclude:t.exclude||null});if(!y.root)g.remove();else{g.replaceWith(y.root);let _=A=>{let E=y.original(A);E&&(o.liveToClone.set(E,A),o.cloneToLive.set(A,E));for(let O of Array.from(A.childNodes||[]))_(O)};_(y.root)}}return k}};return u}var Ot=F;function La(e){try{return JSON.parse(e)}catch(t){throw new be(`Invalid strict JSON: ${t.message}`,t)}}function ul(e){try{return JSON.parse(e)}catch{}let t={BRACE_OPEN:"{",BRACE_CLOSE:"}",BRACKET_OPEN:"[",BRACKET_CLOSE:"]",COLON:":",COMMA:",",STRING:"STRING",SELECTOR:"SELECTOR",IDENTIFIER:"IDENTIFIER",NUMBER:"NUMBER",BOOLEAN:"BOOLEAN"};function r(l){let n=[],a=0;for(;a<l.length;){let o=l[a];if(/\s/.test(o)){a++;continue}if("{}".includes(o)){n.push({type:o,value:o}),a++;continue}if(o==="["){let p=!1,m=a+1;for(;m<l.length&&/\s/.test(l[m]);)m++;if(m<l.length&&/[a-zA-Z_]/.test(l[m])&&(p=!0),!p){n.push({type:o,value:o}),a++;continue}}if(o==="]"){n.push({type:o,value:o}),a++;continue}if(o===":"){n.push({type:t.COLON,value:o}),a++;continue}if(o===","){n.push({type:t.COMMA,value:o}),a++;continue}if(o==='"'||o==="'"){let p=o,m=a+1;for(;m<l.length&&l[m]!==p;)l[m]==="\\"&&m++,m++;n.push({type:t.STRING,value:l.substring(a+1,m),quoted:!0,sourceQuote:p}),a=m+1;continue}let s=a,h;for(;s<l.length&&!/[{},]/.test(l[s]);)if(l[s]===":"){let p=[":first",":last",":nth-child",":nth-of-type",":first-child",":last-child",":first-of-type",":last-of-type",":only-child",":only-of-type",":hover",":focus",":active",":visited",":disabled",":enabled",":checked",":empty",":root",":target",":not",":before",":after",":nth-last-child",":nth-last-of-type"],m=!1;for(let u of p){let c=u.substring(1);if(l.substring(s+1,s+1+c.length)===c){m=!0,s+=c.length;break}}if(!m)break}else if(l[s]==="["){for(s++;s<l.length&&l[s]!=="]";){if(l[s]==='"'||l[s]==="'"){let p=l[s];for(s++;s<l.length&&l[s]!==p;)l[s]==="\\"&&s++,s++}s++}s<l.length&&l[s]==="]"&&s++}else s++;h=l.substring(a,s);let d=t.IDENTIFIER;/^-?\d+(\.\d+)?$/.test(h)?d=t.NUMBER:h==="true"||h==="false"||h==="null"?d=t.BOOLEAN:/^[.#@\[]|[.#@\[]| /.test(h)&&(d=t.SELECTOR),n.push({type:d,value:h,quoted:!1}),a=s}return n}function i(l){let n="";for(let a=0;a<l.length;a++){let o=l[a];if("{}".includes(o.type)||"[]".includes(o.type)){n+=o.value;continue}if(o.type===t.COLON){n+=o.value;continue}if(o.type===t.COMMA){let s=l[a+1];if(s&&(s.type==="}"||s.type==="]"))continue;n+=o.value;continue}if(o.type===t.STRING&&o.quoted){let s=o.value;o.sourceQuote==="'"&&(s=s.replace(/\\'/g,"'"),s=s.replace(/(\\*)"/g,(h,d)=>d.length%2===0?d+'\\"':h)),n+=`"${s}"`;continue}if(o.type===t.NUMBER||o.type===t.BOOLEAN){n+=o.value;continue}if(o.type===t.SELECTOR||o.type===t.IDENTIFIER){n+=`"${o.value}"`;continue}n+=`"${o.value}"`}return n}try{let l=r(e),n=i(l);return JSON.parse(n)}catch(l){throw new be("Invalid extraction rules syntax: "+l.message,l)}}var ja=["1","2"],ii=/^[a-zA-Z0-9_-]+$/;function ml(e,t,r){let i;if(r===void 0)i="script[data-rules-name]";else{if(typeof r!="string"||!ii.test(r))throw new Error(`hyper-html-api: invalid rules token ${JSON.stringify(r)} (must match ${ii})`);i=`script[data-rules-name~="${r}"]`}let l=e.find(t,i,{includeRulesTag:!0});if(l.length===0)return null;r!==void 0&&l.length>1&&console.warn(`hyper-html-api: ${l.length} rules tags match data-rules-name~="${r}"; using the first.`);let n=l[0],a=e.attr(n,"data-rules-version");if(!ja.includes(a))throw new Xi(a);return{rules:ul(e.text(n)),tagNode:n,version:a}}var Bh=new Function("url","return import(url)");function Ma(e,t,r,i){let l=e.length,n=t.length,a=new Array(l).fill(-1);if(i){let g=new Set;for(let k=0;k<l;k++){let y=i[k];!(y>=0&&y<n)||g.has(y)||(a[k]=y,g.add(y))}}if(l===0||n===0)return a;let o=e.map(g=>li(g,r)),s=t.map(g=>li(g,r)),h=new Array(n).fill(!1);for(let g of a)g>=0&&(h[g]=!0);let d=new Map;s.forEach((g,k)=>{h[k]||d.set(g,d.has(g)?-1:k)});let p=new Map;o.forEach((g,k)=>{a[k]>=0||p.set(g,(p.get(g)||0)+1)}),o.forEach((g,k)=>{if(a[k]>=0||p.get(g)!==1)return;let y=d.get(g);y===void 0||y===-1||h[y]||(a[k]=y,h[y]=!0)});let m=[];for(let g=0;g<l;g++)a[g]<0&&m.push(g);let u=[];for(let g=0;g<n;g++)h[g]||u.push(g);if(m.length===0||u.length===0)return a;let c=l*n+1,f=Na(e,t,r),b=(g,k)=>f(g,k)*c+Math.abs(g-k);for(let[g,k]of za(m,u,b))a[g]=k;return a}var pl=e=>typeof e=="object"&&e!==null;function li(e,t){if(!pl(t))return e==null?" null":String(e);let r=Object.keys(t);return JSON.stringify(r.map(i=>{let l=JSON.stringify(e?.[i]);return l===void 0?" undef":l}))}function Na(e,t,r){if(!pl(r))return(o,s)=>e[o]===t[s]?0:1;let i=Object.keys(r),l=o=>i.map(s=>JSON.stringify(o?.[s])),n=e.map(l),a=t.map(l);return(o,s)=>{let h=n[o],d=a[s],p=0;for(let m=0;m<i.length;m++)h[m]!==d[m]&&p++;return p}}function za(e,t,r){return e.length<=t.length?ni(e,t,r,!1):ni(t,e,(i,l)=>r(l,i),!0)}function ni(e,t,r,i){let l=e.length,n=t.length,a=[];for(let p=0;p<=l;p++)a.push(new Float64Array(n+1).fill(1/0));let o=[];for(let p=0;p<=l;p++)o.push(new Uint8Array(n+1));for(let p=0;p<=n;p++)a[l][p]=0;for(let p=l-1;p>=0;p--)for(let m=n-1;m>=0;m--){let u=r(e[p],t[m])+a[p+1][m+1],c=a[p][m+1];u<=c?(a[p][m]=u,o[p][m]=1):a[p][m]=c}let s=[],h=0,d=0;for(;h<l&&d<n;)o[h][d]&&(s.push(i?[t[d],e[h]]:[e[h],t[d]]),h++),d++;return s}function fl(e,t,r,i,l,n,a,o={}){let s=e.find(t,r,o);if(l.length===0){s.forEach(_=>e.remove(_)),Wt(o,"onRowsApplied",n.path,[]);return}let h=l.length>s.length,d=s[0]||null;if(h&&!d&&(d=Ia(e,t,r,o),!d))throw new Me(n.path);let p=s.map(_=>Ba(e,_,i,o)),m=null;if(h&&d){m=e.clone(d);let _=o.templateAttr&&e.attr(d,o.templateAttr)!=null;o.templateAttr&&e.removeAttr(m,o.templateAttr),_&&e.removeAttr(m,"hidden");let A=e.stripIds(m);A>0&&console.warn(`[hyper-html-api] stripped ${A} id attribute(s) from cloned template at "${n.path.join(".")||"(root)"}"`)}let u=Ma(l,p,i,Fa(e,s,l,n,o)),c=s.length>0?e.parent(s[0]):Va(e,t,r,d,n,o),f=s.length>0?gl(e,c,s[0]):0,b=Ja(e,s),g=new Set,k=[],y=l.map((_,A)=>{let E=u[A];if(E>=0)return g.add(E),k.push(!1),s[E];k.push(!0);let O=e.clone(m);return e.stripIds(O),O});s.forEach((_,A)=>{g.has(A)||e.remove(_)}),b?Ga(e,c,f,y,k):Xa(e,y,k,c,f),Da(e,y,i,l,n,a,o),Wt(o,"onRowsApplied",n.path,y)}function Wt(e,t,r,i){if(typeof e[t]=="function")try{return e[t](r.slice(),i)}catch(l){console.warn(`[hyper-html-api] ${t} threw at "${r.join(".")||"(root)"}"`,l);return}}function Fa(e,t,r,i,l){let n=Wt(l,"identifyRows",i.path,r);if(!Array.isArray(n))return null;let a=new Array(r.length).fill(-1),o=new Set,s=new Map(t.map((h,d)=>[e.identity(h),d]));for(let h=0;h<r.length;h++){if(!n[h])continue;let d=s.get(e.identity(n[h]));d===void 0||o.has(d)||(a[h]=d,o.add(d))}return a}function Da(e,t,r,i,l,n,a){t.forEach((o,s)=>{if(r===null){let h=i[s],d=h==null?"":String(h);e.text(o)!==d&&e.text(o,d)}else{let h=n(e,o,r,i[s],{depth:l.depth+1,path:[...l.path,s]},a);h&&h!==o&&(t[s]=h)}})}function Ba(e,t,r,i){return r===null?e.text(t):Ht(e,t,r,i.onRowsRead?{...i,onRowsRead:void 0}:i)}function gl(e,t,r){let i=e.children(t);for(let l=0;l<i.length;l++)if(e.sameNode(i[l],r))return l;return-1}function Ia(e,t,r,i){if(!i.templateAttr)return null;let l=/^\s*:scope\s*>\s*/.test(r),n=new RegExp(":not\\(\\["+i.templateAttr.replace(/[.*+?^${}()|[\]\\-]/g,"\\$&")+"\\]\\)","g"),a=r.replace(/^\s*:scope\s*>\s*/,"").replace(n,""),o=l?Pa(a):0,s=Yi(a),h=t;for(;h;){let d=e.find(h,a,{includeRulesTag:!1,templateAttr:null});for(let p of d)if(e.attr(p,i.templateAttr)!=null&&!(l&&!Ua(e,t,p,o))&&!(s&&!e.matches(e.parent(p),s)))return p;h=e.parent(h)}return null}function Pa(e){let t=0,r=null,i=1;for(let l=0;l<e.length;l++){let n=e[l];if(n==="\\")l++;else if(r)n===r&&(r=null);else if(n==='"'||n==="'")r=n;else if(n==="["||n==="(")t++;else if(n==="]"||n===")")t>0&&t--;else if(t===0&&n===">")i++;else{if(t===0&&(n==="+"||n==="~"))return null;if(t===0&&/\s/.test(n)){if(!Ha(e,l))return null;for(;l+1<e.length&&/\s/.test(e[l+1]);)l++}}}return i}function Ha(e,t){let r=t;for(;r<e.length&&/\s/.test(e[r]);)r++;let i=t>0?e[t-1]:"",l=r<e.length?e[r]:"";return ai(i)||ai(l)}function ai(e){return e===">"||e==="+"||e==="~"}function Ua(e,t,r,i){if(i===null)return!0;let l=r;for(let s=0;s<i;s++)if(l=e.parent(l),!l)return!1;let n=e.prop(t,"tagName");if(!n)return!0;let a=e.prop(l,"tagName");if(!a||String(a).toLowerCase()!==String(n).toLowerCase())return!1;let o=new Set(oi(e,l));return oi(e,t).every(s=>o.has(s))}function oi(e,t){let r=e.attr(t,"class");return r?String(r).split(/\s+/).filter(Boolean):[]}function Va(e,t,r,i,l,n){let a=e.parent(i);if(a&&Wa(e,t,a)&&!ci(e,a,n))return a;let o=Yi(r);if(!o)throw new Me(l.path,`the list selector "${r}" names no container to hold the new rows \u2014 write it as "container > row" (for example "ol.comments > li") so the cloned row lands in the list`);let s=(o===":scope"?e.prop(t,"tagName")?[t]:[]:e.find(t,o,n)).filter(h=>!ci(e,h,n));if(s.length===0)throw new Me(l.path,`no container matching "${o}" inside this context to hold the new rows`);if(s.length>1)throw new Me(l.path,`"${o}" matches ${s.length} containers inside this context, so the new rows have no single destination`);return s[0]}function Wa(e,t,r){let i=r;for(;i;){if(e.sameNode(i,t))return!0;i=e.parent(i)}return e.parent(t)?!1:e.find(t,"*",{includeRulesTag:!0,templateAttr:null}).some(l=>e.sameNode(l,r))}var si=["no-data","editor-ui"];function ci(e,t,r){let i=t;for(;i;){if(r.templateAttr&&e.attr(i,r.templateAttr)!=null||si.some(n=>e.attr(i,n)!=null))return!0;let l=e.attr(i,"clay");if(l&&l.split(/\s+/).some(n=>si.includes(n)))return!0;i=e.parent(i)}return!1}function Ga(e,t,r,i,l){let n=e.children(t),a=n.map(s=>e.identity(s)),o=!1;i.forEach((s,h)=>{let d=e.identity(s),p=r+h;if(a[p]===d)return;if(!l[h]){let u=a.indexOf(d);u!==-1&&(a.splice(u,1),n.splice(u,1))}let m=Math.min(p,n.length);m<n.length?e.insertBefore(n[m],s):n.length?o?e.insertAfter(n[n.length-1],s):(e.insertAt(t,s,p),o=!0):e.insertAt(t,s,0),a.splice(m,0,d),n.splice(m,0,s)})}function Ja(e,t){if(t.length<=1)return!0;let r=e.parent(t[0]);if(!r)return!1;let i=new Map(e.children(r).map((n,a)=>[e.identity(n),a])),l=[];for(let n of t){let a=i.get(e.identity(n));if(a===void 0)return!1;l.push(a)}return l.sort((n,a)=>n-a),l[l.length-1]-l[0]===l.length-1}function Xa(e,t,r,i,l){let n=null,a=l;for(let o=0;o<t.length;o++){if(!r[o]){n=t[o];continue}let s=n?e.parent(n):i;if(!s)continue;let h=n?gl(e,s,n)+1:a++;e.insertAt(s,t[o],h),n=t[o]}}function hi(e,t,r,i,l={}){if(Object.prototype.hasOwnProperty.call(l,"exclude")&&e.semanticExclude===!1)throw new Error("This adapter does not support semantic exclude queries; omit exclude or use the DOM adapter.");nl(r,l);let n=[];if(Gt(r,i,[],n),n.length)throw new Ki(n);nt(e,t,r,i,{depth:0,path:[]},l)}function nt(e,t,r,i,l,n={}){if(l.depth>He)throw new sr(l.path);if(i===void 0)return t;if(typeof r=="string")return Ka(e,t,r,i,l,n);if(Array.isArray(r)){let[a,o]=r;return fl(e,t,a,o,i,l,nt,n),t}if(typeof r=="object"&&r!==null){for(let[a,o]of Object.entries(r)){let s=nt(e,t,o,i==null?i:i[a],{depth:l.depth+1,path:[...l.path,a]},n);s&&s!==t&&(t=s)}return t}return t}function Ka(e,t,r,i,l,n){if(r.endsWith("[]")){let p=r.slice(0,-2);return fl(e,t,p,null,i,l,nt,n),t}let a=ll(r,n);if(a){let p=a.selector?e.find(t,a.selector,n):[t];return p.length&&a.codec.write(e,p[0],i,{query:n.query}),t}if(r.startsWith("@"))return ui(e,t,r.slice(1),i);let o=I(r);if(o!==-1){let{selector:p,all:m}=ke(r),u=r.slice(o+1),c=p?e.find(t,p,n):[t];for(let f of m?c:c.slice(0,1))ui(e,f,u,i);return t}if(r===".")return di(e,t,i),t;let{selector:s,all:h}=ke(r),d=e.find(t,s,n);for(let p of h?d:d.slice(0,1))di(e,p,i);return t}function di(e,t,r){let i=r==null?"":String(r);e.text(t)!==i&&e.text(t,i)}function Za(e,t,r,i){if(e.styleVar){e.styleVar(t,r,i);return}ga(e,t,r,i)}function ui(e,t,r,i){if(el(r))return Za(e,t,r,i==null?"":String(i).trim()),t;if(Gi.has(r)){let l=e.prop(t,r);if(l===i||l!=null&&i!=null&&String(l)===String(i))return t;throw new Zi(r)}if(r==="outerHTML"){let l=i==null?"":String(i);return e.replaceWith(t,l)}if(ia.has(r)){let l=la(r,i);return e.prop(t,r)!==l&&e.prop(t,r,l),t}if(i==null)e.attr(t,r)!=null&&e.removeAttr(t,r);else{let l=String(i);e.attr(t,r)!==l&&e.attr(t,r,l)}return t}function Gt(e,t,r,i){if(t!==void 0){if(typeof e=="string"){if(cr(e))return;if(e.endsWith("[]")){Array.isArray(t)?t.forEach((l,n)=>{typeof l=="object"&&l!==null&&i.push({path:$e([...r,n]),expected:"scalar",got:Oe(l)})}):i.push({path:$e(r),expected:"array",got:Oe(t)});return}t!==null&&typeof t=="object"&&i.push({path:$e(r),expected:"scalar",got:Oe(t)});return}if(Array.isArray(e)){if(!Array.isArray(t)){i.push({path:$e(r),expected:"array",got:Oe(t)});return}let l=e[1];t.forEach((n,a)=>Gt(l,n,[...r,a],i));return}if(typeof e=="object"&&e!==null){if(t===null||Array.isArray(t)||typeof t!="object"){i.push({path:$e(r),expected:"object",got:Oe(t)});return}for(let[l,n]of Object.entries(e))Gt(n,t[l],[...r,l],i)}}}function Oe(e){return e===null?"null":Array.isArray(e)?"array":typeof e}function $e(e){return e.join(".")}var Qa=Object.freeze({refusedRuleForms:["innerHTML","outerHTML"],refusedTargets:["script","style","template","noscript","iframe","object","embed","xmp","plaintext","noembed","noframes","meta","link","base","animate","set","animatemotion","animatetransform"],refusedAttributes:["style","srcdoc","is","http-equiv","sandbox","allow","nonce","integrity","charset","htmlclaytoken","htmlclayid","data-rules-name","data-rules-version","clay","contenteditable",...Ea],refusedAttributePrefixes:["on"],urlAttributes:["href","src","srcset","action","formaction","poster","cite","data","ping","background","longdesc","usemap","xlink:href","manifest","codebase","icon"],refusedSchemes:["javascript","vbscript","data"],customPropertyValues:Object.freeze({maxLength:200,refusedCharacters:[";","{","}","<",">","\\"],refusedFunctions:["url","src","image","image-set","-webkit-image-set","cross-fade","element","expression","paint"]})});function Ya(e){if(/[\u0000-\u001f\u007f]/.test(e))return"control characters are not allowed in a custom property value";let t=[],r=null;for(let i=0;i<e.length;i++){let l=e[i];if(r){l===r&&(r=null);continue}if(l==='"'||l==="'")r=l;else{if(l==="/"&&e[i+1]==="*")return"comments are not allowed in a custom property value";if(l==="(")t.push(")");else if(l==="[")t.push("]");else if((l===")"||l==="]")&&t.pop()!==l)return`"${l}" does not close anything in this custom property value`}}return r?"a custom property value must close its quotes":t.length?"a custom property value must close its brackets":null}function eo(e,t=Qa){let r=t.customPropertyValues,i=String(e);if(i.length>r.maxLength)return`custom property values are limited to ${r.maxLength} characters`;let l=r.refusedCharacters.find(s=>i.includes(s));if(l)return`${JSON.stringify(l)} is not allowed in a custom property value`;let n=Ya(i);if(n)return n;let a=i.toLowerCase(),o=r.refusedFunctions.find(s=>new RegExp(`(^|[^a-z0-9_-])${s.replace(/[-]/g,"\\-")}\\s*\\(`).test(a));return o?`"${o}()" is not allowed in a custom property value`:null}function mi(e,t,r){if(r&&typeof r=="object")return{rules:r,tagNode:null,version:"1"};if(typeof r=="string"){let i=t&&t.ownerDocument?t.ownerDocument:t;return ml(e,i,r)}return null}var $t=e=>({codecs:va(),...e}),X={extract:(e,t,r={})=>Ht(pe(e,r),e,t,$t(r)),apply:(e,t,r,i={})=>hi(pe(e,{...i,_write:!0}),e,t,r,$t(i)),findRulesIn:(e,t)=>ml(Ot,e,t),findRules:(e,t)=>mi(Ot,e,t),bind:(e,t,r={})=>{let i=mi(Ot,e,t);if(!i){let n=typeof t=="string"?`data-rules-name~="${t}"`:"the provided rules object";throw new Error(`hyper-html-api: could not resolve rules for ${n}`)}let l=$t({version:i.version,...r});return{...i,get:()=>Ht(pe(e,r),e,i.rules,l),set:n=>hi(pe(e,{...r,_write:!0}),e,i.rules,n,l)}},codecs:{register:ba,get:ka},parseStrict:La,parseRelaxed:ul,ruleAttrIndex:I,splitRule:ke,errors:Ji,DOM_PROPERTIES:Wi},bl={};or(bl,{fromString:()=>ve,getRuleAtPath:()=>Be,getValueAtPath:()=>ro,setAtPath:()=>Jt,toString:()=>to});function to(e){return e.map(String).join(".")}function ve(e){return e===""?[]:e.split(".").map(t=>/^\d+$/.test(t)?Number(t):t)}function Be(e,t){let r=e;for(let i of t){if(r==null)return;if(typeof r=="string"){if(r.endsWith("[]")&&(typeof i=="number"||i==="*")){r=r.slice(0,-2);continue}return}if(Array.isArray(r)){if(typeof i!="number"&&i!=="*")return;r=r[1];continue}if(typeof r=="object"){if(typeof i=="number"||!(i in r))return;r=r[i];continue}return}return r}function ro(e,t){let r=e;for(let i of t){if(r==null)return;r=r[i]}return r}function Jt(e,t,r){if(t.length===0)return r;let[i,...l]=t;if(typeof i=="number"){let n=Array.isArray(e)?[...e]:[];return n[i]=Jt(n[i],l,r),n}return{...e&&typeof e=="object"?e:{},[i]:Jt((e||{})[i],l,r)}}function ur(e){if(typeof e=="string")return e.endsWith("[]")?[]:"";if(Array.isArray(e))return[];if(typeof e=="object"&&e!==null){let t={};for(let[r,i]of Object.entries(e))t[r]=ur(i);return t}return""}function kl(e,t,{ignoreActiveValue:r=!0}={}){Jr.morph(e,t,{morphStyle:"innerHTML",ignoreActiveValue:r,restoreFocus:!0,formStateSync:"property",policy:"raw"})}var vl=new WeakMap,Xt=e=>e.map(String).join(".");function io(e){return typeof CSS<"u"&&CSS.escape?CSS.escape(e):String(e).replace(/[^a-zA-Z0-9_\-.*]/g,t=>"\\"+t)}function yl(e,t){if(!e||!e.querySelector)return null;let r=e.querySelector(`[data-hcms-path="${io(t)}"]`),i=r&&r.querySelector(".hcms-array-items");return i?Array.from(i.children).filter(l=>l.matches&&l.matches("[data-hcms-card], [data-hcms-array-item]")):null}function xl(e,t,r){let i=yl(e,t);!i||i.length!==r.length||i.forEach((l,n)=>{r[n]&&vl.set(l,r[n])})}function wl(){let e=new Map;return{hooks:{onRowsRead(t,r){e.set(Xt(t),r)}},seed(t){for(let[r,i]of e)xl(t,r,i);e.clear()}}}function lo(e){return{identifyRows(t,r){let i=yl(e,Xt(t));return!i||i.length!==r.length?null:i.map(l=>vl.get(l)||null)},onRowsApplied(t,r){xl(e,Xt(t),r)}}}function ae(e){return e&&e.nodeType===9?e.body||e.documentElement:e}function Al(e,t){if(!t||t.nodeType!==9)return e;return r(e);function r(i){if(typeof i=="string")return i==="."?"body":i.startsWith("@")?`body${i}`:i;if(Array.isArray(i)||!i||typeof i!="object")return i;let l={};for(let[n,a]of Object.entries(i))l[n]=r(a);return l}}var pi={skip:"[data-hcms-shell]",templateAttr:"cms-template"},Kt="data-hcms-rollback-ui",no=0;function ao(e,t,r,i={}){return oo(e,t,r,i)}function oo(e,t,r,i){let{shellRoot:l,structural:n,structuralPath:a,formRoot:o}=i,s=o?{...pi,...lo(o)}:pi;if(!n)try{return X.apply(e,t,r,s),{ok:!0}}catch(m){return{ok:!1,error:m}}let h=so(e,t,a),d=h?ho(h):null,p=h?null:mo(ae(e),l);try{return X.apply(e,t,r,s),{ok:!0}}catch(m){return d?uo(h,d):p&&po(ae(e),l,p),{ok:!1,error:m}}}function so(e,t,r){if(!r||!e)return null;let i=ve(r),l=[],n=t;for(let a of i){if(typeof n=="string"||n==null||Array.isArray(n))break;if(typeof n=="object"&&a in n){if(l.push(a),n=n[a],Array.isArray(n)||typeof n=="string"&&n.endsWith("[]"))break}else return null}return!Array.isArray(n)&&!(typeof n=="string"&&n.endsWith("[]"))?null:co(e,t,l)}function co(e,t,r){if(r.length===0)return null;let i=e,l=t;for(let n=0;n<r.length;n++){let a=r[n];if(!l||typeof l!="object"||Array.isArray(l))return null;let o=l[a];if(o==null)return null;if(n===r.length-1){if(Array.isArray(o)){let[s]=o;return i.querySelector?.(s)?.parentElement||null}if(typeof o=="string"&&o.endsWith("[]")){let s=o.slice(0,-2);return i.querySelector?.(s)?.parentElement||null}return null}l=o}return null}function ho(e){let t=[],r=[];for(let i of Array.from(e.childNodes))t.push(mr(i,r));return{nodes:t,retained:r}}function uo(e,t){let r=e.cloneNode(!1);for(let i of t.nodes)r.appendChild(i);qe(e,Array.from(r.childNodes),El()),_l(e,t.retained)}function mo(e,t){let r=[],i=[];for(let l of Array.from(e.childNodes))l===t||t&&l.contains?.(t)||r.push(mr(l,i));return{nodes:r,retained:i}}function po(e,t,r){let i=e.cloneNode(!1);for(let l of r.nodes)i.appendChild(l);qe(e,Array.from(i.childNodes),El()),_l(e,r.retained)}function mr(e,t){let r=e.cloneNode(!1);if(e.nodeType===1&&e.matches('[editor-ui],[clay~="editor-ui"]')){let n=String(++no);r.setAttribute(Kt,n),t.push({id:n,node:e})}let i=e.nodeType===1&&e.tagName==="TEMPLATE"?e.content:e,l=r.nodeType===1&&r.tagName==="TEMPLATE"?r.content:r;for(let n of Array.from(i.childNodes||[]))l.appendChild(mr(n,t));return r}function _l(e,t){for(let{id:r,node:i}of t){let l=e.querySelector(`[${Kt}="${r}"]`);l===i?i.removeAttribute(Kt):i.isConnected?l?.remove():l?.replaceWith(i)}}function El(){return{morphStyle:"innerHTML",policy:"raw",restoreFocus:!1,scripts:{handle:!1,merge:!1}}}var fo=/^--font-(size|weight|style|stretch|variant|feature-settings|variation-settings|optical-sizing|kerning|synthesis|smoothing|palette|size-adjust|display)(-|$)/;function Ie(e){return e.replace(/([a-z])([A-Z])/g,"$1 $2").replace(/([A-Z]+)([A-Z][a-z])/g,"$1 $2").replace(/[-_]/g," ").replace(/\s+/g," ").trim().replace(/^./,t=>t.toUpperCase())}var go='<div class="hcms-drag-handle mirk-sortable__grip" aria-hidden="true"><div class="mirk-sortable__dots"><span class="mirk-sortable__dot"></span><span class="mirk-sortable__dot"></span><span class="mirk-sortable__dot"></span><span class="mirk-sortable__dot"></span><span class="mirk-sortable__dot"></span><span class="mirk-sortable__dot"></span><span class="mirk-sortable__dot"></span><span class="mirk-sortable__dot"></span></div></div>',Je='<svg class="hcms-x" viewBox="0 0 16 16" shape-rendering="crispEdges" aria-hidden="true"><path d="M4 4 L12 12 M12 4 L4 12" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="square"></path></svg>',Sl={"@scalar":`
    <label class="hcms-field" data-hcms-shape="scalar">
      <span class="hcms-label" data-hcms-label></span>
      <textarea class="mirk-textarea" rows="1" data-hcms-field></textarea>
      <div class="hcms-error" hidden></div>
    </label>
  `,"@object":`
    <section class="hcms-object" data-hcms-shape="object">
      <h3 class="hcms-object-title" data-hcms-label></h3>
      <div class="hcms-object-fields"></div>
      <div class="hcms-error" hidden></div>
    </section>
  `,"@scalar-array":`
    <section class="hcms-array hcms-scalar-array" data-hcms-shape="scalar-array">
      <header class="hcms-array-header">
        <h3 class="hcms-array-title" data-hcms-label></h3>
      </header>
      <ul class="hcms-array-items"></ul>
      <div class="hcms-error" hidden></div>
      <button type="button" class="hcms-add mirk-button mirk-button--small" data-hcms-action="add"><span class="mirk-button__label">+ Add</span></button>
    </section>
  `,"@scalar-array-item":`
    <li class="hcms-array-item" draggable="true">
      <input class="mirk-input" data-hcms-field />
      <button type="button" class="hcms-move hcms-move-up hcms-sr-only" data-hcms-action="move-up" aria-label="Move up">\u2191</button>
      <button type="button" class="hcms-move hcms-move-down hcms-sr-only" data-hcms-action="move-down" aria-label="Move down">\u2193</button>
      <button type="button" class="hcms-remove" data-hcms-action="remove" aria-label="Remove">\xD7</button>
      <div class="hcms-error" hidden></div>
    </li>
  `,"@object-array":`
    <section class="hcms-array hcms-object-array hcms-array--cards" data-hcms-shape="object-array">
      <header class="hcms-array-header">
        <h3 class="hcms-array-title" data-hcms-label></h3>
      </header>
      <div class="hcms-array-items"></div>
      <div class="hcms-error" hidden></div>
      <button type="button" class="hcms-add mirk-button mirk-button--small" data-hcms-action="add"><span class="mirk-button__label">+ Add</span></button>
    </section>
  `,"@object-array-item":`
    <article class="hcms-card mirk-sortable__item" draggable="true">
      ${go}
      <div class="hcms-card-body mirk-sortable__body">
        <div class="hcms-card-fields"></div>
        <div class="hcms-card-controls">
          <button type="button" class="hcms-move hcms-move-up hcms-sr-only" data-hcms-action="move-up" aria-label="Move up">\u2191</button>
          <button type="button" class="hcms-move hcms-move-down hcms-sr-only" data-hcms-action="move-down" aria-label="Move down">\u2193</button>
          <button type="button" class="hcms-remove hcms-remove--card" data-hcms-action="remove" aria-label="Remove">${Je}</button>
        </div>
      </div>
      <div class="hcms-error" hidden></div>
    </article>
  `,"@file":`
    <div class="hcms-field hcms-upload hcms-upload--file" data-hcms-shape="scalar">
      <span class="hcms-label" data-hcms-label></span>
      <div class="mirk-file mirk-file--compact mirk-file--round">
        <label class="mirk-button mirk-button--round mirk-button--small">
          <input type="file" data-hcms-upload />
          <span class="mirk-button__label">Choose</span>
        </label>
        <a class="mirk-file__name" data-hcms-field></a>
        <button type="button" class="hcms-upload-clear" data-hcms-action="clear-upload" aria-label="Remove file">${Je}</button>
      </div>
      <div class="hcms-error" hidden></div>
    </div>
  `,"@image":`
    <div class="hcms-field hcms-upload hcms-upload--image" data-hcms-shape="scalar">
      <span class="hcms-label" data-hcms-label></span>
      <div class="mirk-image mirk-image--compact mirk-image--rounded">
        <label class="mirk-button mirk-button--small mirk-image__upload">
          <input type="file" accept="image/*" data-hcms-upload />
          <span class="mirk-button__label">Upload image</span>
        </label>
        <figure class="mirk-image__thumb">
          <span class="mirk-image__frame"><img class="mirk-image__preview" data-hcms-field alt="" /></span>
          <button type="button" class="hcms-upload-clear hcms-upload-clear--badge" data-hcms-action="clear-upload" aria-label="Remove image">${Je}</button>
        </figure>
      </div>
      <div class="hcms-error" hidden></div>
    </div>
  `,"@checkbox":`
    <div class="hcms-field hcms-field--row" data-hcms-shape="scalar">
      <span class="hcms-label" data-hcms-label></span>
      <label class="mirk-checkbox">
        <input type="checkbox" class="mirk-sr-only" data-hcms-field />
        <span class="mirk-checkbox__box"><span class="mirk-checkbox__mark"></span></span>
      </label>
      <div class="hcms-error" hidden></div>
    </div>
  `,"@toggle":`
    <div class="hcms-field hcms-field--row" data-hcms-shape="scalar">
      <span class="hcms-label" data-hcms-label></span>
      <label class="mirk-toggle">
        <input type="checkbox" role="switch" class="mirk-sr-only" data-hcms-field />
        <span class="mirk-toggle__track"><span class="mirk-toggle__thumb"></span></span>
      </label>
      <div class="hcms-error" hidden></div>
    </div>
  `,"@select":`
    <label class="hcms-field" data-hcms-shape="scalar">
      <span class="hcms-label" data-hcms-label></span>
      <div class="mirk-select">
        <select class="mirk-select__field" data-hcms-field></select>
        <span aria-hidden="true" class="mirk-select__chevron">\u203A</span>
      </div>
      <div class="hcms-error" hidden></div>
    </label>
  `,"@radio":`
    <div class="hcms-field" data-hcms-shape="scalar">
      <span class="hcms-label" data-hcms-label></span>
      <div class="hcms-radio-row">
        <label class="mirk-radio">
          <input type="radio" class="mirk-sr-only" data-hcms-field />
          <span class="mirk-radio__ring"><span class="mirk-radio__fill"></span><span class="mirk-radio__dot"></span></span>
          <span class="mirk-radio__label"></span>
        </label>
      </div>
      <div class="hcms-error" hidden></div>
    </div>
  `,"@textarea":`
    <label class="hcms-field" data-hcms-shape="scalar">
      <span class="hcms-label" data-hcms-label></span>
      <textarea class="mirk-textarea" rows="3" data-hcms-field></textarea>
      <div class="hcms-error" hidden></div>
    </label>
  `,"@richtext":`
    <div class="hcms-field" data-hcms-shape="scalar">
      <span class="hcms-label" data-hcms-label></span>
      <div class="mirk-textarea hcms-richtext" contenteditable="true" data-hcms-field></div>
      <div class="hcms-error" hidden></div>
    </div>
  `,"@number":`
    <label class="hcms-field" data-hcms-shape="scalar">
      <span class="hcms-label" data-hcms-label></span>
      <input class="mirk-input" type="number" data-hcms-field />
      <div class="hcms-error" hidden></div>
    </label>
  `,"@color":`
    <div class="hcms-field hcms-color" data-hcms-shape="scalar">
      <span class="hcms-label" data-hcms-label></span>
      <input type="hidden" data-hcms-field />
      <div class="hcms-color-row">
        <button type="button" class="hcms-color-current" data-hcms-action="color-panel" aria-expanded="false">
          <span class="hcms-color-chip"></span>
          <span class="hcms-color-name"></span>
        </button>
        <button type="button" class="hcms-color-clear" data-hcms-action="color-clear" aria-label="Clear color">${Je}</button>
      </div>
      <div class="hcms-color-panel" hidden>
        <div class="hcms-color-grid"></div>
        <div class="hcms-color-custom">
          <input type="color" class="hcms-color-native" data-hcms-color-input="native" aria-label="Pick a custom color" />
          <input class="mirk-input hcms-color-text" data-hcms-color-input="text" spellcheck="false" autocomplete="off" placeholder="Any CSS color" aria-label="Custom color" />
        </div>
      </div>
      <div class="hcms-error" hidden></div>
    </div>
  `,"@font":`
    <div class="hcms-field hcms-font" data-hcms-shape="scalar">
      <span class="hcms-label" data-hcms-label></span>
      <input type="hidden" data-hcms-field />
      <button type="button" class="hcms-font-current" data-hcms-action="font-panel" aria-haspopup="listbox" aria-expanded="false">
        <span class="hcms-font-name"></span>
        <span aria-hidden="true" class="hcms-font-chevron">\u203A</span>
      </button>
      <div class="hcms-font-panel" hidden>
        <div class="hcms-font-list" role="listbox"></div>
      </div>
      <div class="hcms-error" hidden></div>
    </div>
  `,"@variant":`
    <div class="hcms-field hcms-variant" data-hcms-shape="scalar">
      <span class="hcms-label" data-hcms-label></span>
      <div class="hcms-variant-row" role="radiogroup">
        <label class="hcms-variant-option">
          <input type="radio" class="mirk-sr-only" data-hcms-field />
          <span class="hcms-variant-icon"></span>
          <span class="hcms-variant-label"></span>
        </label>
      </div>
      <div class="hcms-error" hidden></div>
    </div>
  `,"@chips":`
    <div class="hcms-field hcms-chips" data-hcms-shape="scalar-array">
      <span class="hcms-label" data-hcms-label></span>
      <div class="mirk-tags hcms-array-items"></div>
      <button type="button" class="hcms-add mirk-button mirk-button--small" data-hcms-action="add"><span class="mirk-button__label">+ Add</span></button>
      <div class="hcms-error" hidden></div>
    </div>
  `,"@chips-item":`
    <span class="mirk-tags__chip" data-hcms-array-item>
      <input class="hcms-chip-field" data-hcms-field aria-label="Item" placeholder="\u2026" />
      <button type="button" class="hcms-remove" data-hcms-action="remove" aria-label="Remove">\xD7</button>
    </span>
  `},bo=["@scalar","@object","@scalar-array","@scalar-array-item","@object-array","@object-array-item"];function Cl(e){let t=e.head||e.documentElement;if(t)for(let r of bo)$l(e,t,r)}function Lt(e,t){if(!Sl[t])return null;let r=e&&(e.head||e.documentElement);return r?$l(e,r,t):null}var fi={src:"@image",checked:"@checkbox",innerHTML:"@richtext"},at={image:"@image",file:"@file",checkbox:"@checkbox",toggle:"@toggle",select:"@select",radio:"@radio",textarea:"@textarea",number:"@number",richtext:"@richtext",color:"@color",font:"@font",variant:"@variant"},ko=new Set([...Object.values(at),"@chips","@chips-item"]);function Ue(e,t,r,i){if(typeof e!="string")return"@scalar";let l=I(e),n=ql(e,l,r,i),a=kt(e,l,r,i,t,"data-hcms-component");if(a&&at[a]){let o=at[a],s=Array.isArray(r)&&r.some(h=>h==="*"||typeof h=="number");return o==="@number"&&!gi(e,l,t,s,n).every(Ao)||(o==="@checkbox"||o==="@toggle")&&(l<0||e.slice(l+1)!=="checked")&&!gi(e,l,t,s,n).every(_o)?"@scalar":o}if(l>=0){let o=e.slice(l+1);if(fi[o])return fi[o];if(/^--color(-|$)/.test(o)&&o!=="--color-scheme")return"@color";if(/^--font(-|$)/.test(o)&&!fo.test(o))return"@font"}return"@scalar"}function vo(e,t,r,i){if(typeof e!="string")return null;let l=I(e),n=kt(e,l,r,i,t,"data-hcms-component");return n&&at[n]||null}var yo=new Set(["@color","@font","@variant"]),xo=new Set(["html","body",":root"]);function pr(e,t,r,i){if(typeof e!="string"||e.endsWith("[]")||!yo.has(Ue(e,t,r,i)))return null;let l=Array.isArray(r)&&r.some(a=>a==="*"||typeof a=="number"),n=Rl(e,I(e)).trim();return!l&&xo.has(n)?"page":"section"}var wo=/^-?\d+(\.\d+)?([eE][+-]?\d+)?$/;function Ao(e){return e==null||e===""?!0:wo.test(String(e))}function _o(e){return e==null||e===""||e==="true"||e==="false"}function gi(e,t,r,i,l){if(!r||!r.querySelectorAll)return[];if(!l||l===".")return[];let n=null;try{n=r.querySelectorAll(l)}catch{return[]}let a=t>=0?e.slice(t+1):null,o=[];for(let s of n)if(!(s.closest&&s.closest("[cms-template], [data-hcms-shell]"))&&(a?a==="value"&&"value"in s?o.push(s.value):o.push(s.getAttribute?s.getAttribute(a):null):o.push((s.textContent||"").trim()),!i))break;return o}function fr(e,t){if(typeof e!="string"||!e.endsWith("[]")||!t||!t.querySelector)return null;let r=e.slice(0,-2).trim();if(!r)return null;let i=null;try{i=t.querySelector(r)}catch{return null}let l=i&&i.closest?i.closest("[data-hcms-component]"):null;return(l&&l.getAttribute?l.getAttribute("data-hcms-component"):null)==="chips"?{array:"@chips",item:"@chips-item"}:null}function Ae(e,t,r){let i=e.join("."),l=e.map(n=>typeof n=="number"?"*":n).join(".");return i&&Q(r,i)||l&&l!==i&&Q(r,l)||Q(r,t)}function Tl(e,t,r){let i=fr(e,r);if(!i)return null;let l=Ae(t,i.array,r);return l&&l.getAttribute("data-hcms-tpl")===i.array?i:null}function Qe(e,t,r,i){if(typeof e!="string")return null;let l=I(e),n=kt(e,l,r,i,t,"data-hcms-options");if(n==null)return null;let a=n.trim().split(/\s+/).filter(Boolean);return a.length?a:null}function Eo(e,t,r,i){if(typeof e!="string")return null;let l=I(e);return kt(e,l,r,i,t,"data-hcms-crop")}function Rl(e,t){return t>=0?e.slice(0,t):e}function ql(e,t,r,i){let l=Rl(e,t);return l&&l!=="."?l:So(i,r)}function So(e,t){if(e==null||!Array.isArray(t))return"";let r=[],i=e;for(let l of t){if(i==null||typeof i=="string")break;if(Array.isArray(i)){if(typeof i[0]!="string"||l!=="*"&&typeof l!="number")return"";r.push(i[0]),i=i[1];continue}if(typeof i!="object"||!Object.prototype.hasOwnProperty.call(i,l))return"";i=i[l]}return r.join(" ")}function bi(e,t,r){if(!t||!t.querySelector||!e||e===".")return null;let i=null;try{i=t.querySelector(e)}catch{return null}return i&&i.getAttribute?i.getAttribute(r):null}function kt(e,t,r,i,l,n){let a=ql(e,t,r,i),o=t>=0?e.slice(t+1):null;if(o){let s=bi(a,l,`${n}:${o}`);if(s!=null)return s;if(o.startsWith("--"))return null}return bi(a,l,n)}function Ol(e,t){if(!e||t==null)return;r(t,[]);function r(i,l){let n=Pe(i);if(n==="scalar"){let a=Ue(i,e,l,t);ko.has(a)&&Lt(e,a);return}if(n==="scalar-array"){let a=fr(i,e);a&&(Lt(e,a.array),Lt(e,a.item));return}if(n==="object"){for(let[a,o]of Object.entries(i))r(o,[...l,a]);return}if(n==="object-array"){let a=i[1],o=[...l,"*"];if(a&&typeof a=="object"&&!Array.isArray(a))for(let[s,h]of Object.entries(a))r(h,[...o,s]);else r(a,o)}}}function $l(e,t,r){let i=Q(e,r);if(i)return i;let l=e.createElement("template");return l.setAttribute("data-hcms-tpl",r),l.setAttribute("save-remove",""),l.innerHTML=Sl[r].trim(),t.appendChild(l),l}function Q(e,t){return!e||!e.querySelector?null:e.querySelector(`template[data-hcms-tpl="${Co(t)}"]`)}function Co(e){return typeof CSS<"u"&&CSS.escape?CSS.escape(e):String(e).replace(/[^a-zA-Z0-9_\-.*]/g,t=>"\\"+t)}function Pe(e){return typeof e=="string"?e.endsWith("[]")?"scalar-array":"scalar":Array.isArray(e)?"object-array":typeof e=="object"&&e!==null?"object":"scalar"}function vt(e){return e?!!(e.content||e).querySelector("[data-hcms-field]"):!1}var ki={IMG:"src",A:"href"};function gr(e){return e.map(t=>typeof t=="number"||/^[0-9]+$/.test(String(t))?"*":String(t)).join(".")}function br(e){if(!e)return"value";let t=(e.tagName||"").toUpperCase();return t==="INPUT"?(e.getAttribute("type")||"text").toLowerCase()==="checkbox"?"checked":"value":t==="TEXTAREA"||t==="SELECT"?"value":ki[t]?ki[t]:e.hasAttribute&&e.hasAttribute("contenteditable")?"innerHTML":null}function vi(e,t){let r=(e.tagName||"").toUpperCase(),i=(e.getAttribute&&e.getAttribute("type")||"").toLowerCase(),l=br(e),n=`${Ll(r,i)}[data-hcms-field="${ne(t)}"]`;return r==="INPUT"&&i==="radio"?`${n}:checked@value`:l?`${n}@${l}`:n}function To(e){let t=(e.tagName||"").toUpperCase(),r=(e.getAttribute&&e.getAttribute("type")||"").toLowerCase(),i=br(e),l=`${Ll(t,r)}[data-hcms-field]`;return t==="INPUT"&&r==="radio"?`${l}:checked@value`:i?`${l}@${i}`:l}function Ll(e,t){return e==="INPUT"?t?`input[type="${t}"]`:"input":e==="TEXTAREA"?"textarea":e==="SELECT"?"select":e==="IMG"?"img":e==="A"?"a":':not([data-hcms-shape="scalar"]):not([data-hcms-shape="object"]):not([data-hcms-shape="object-array"]):not([data-hcms-shape="scalar-array"])'}function ne(e){return typeof CSS<"u"&&CSS.escape?CSS.escape(e):String(e).replace(/[^a-zA-Z0-9_\-.*]/g,t=>"\\"+t)}var yi=new Set(["__proto__","constructor","prototype"]);function Ro(e){let t=new Map;return r(e,null),t;function r(i,l){if(typeof i=="string"){typeof l=="string"&&!i.endsWith("[]")&&t.set(l,(t.get(l)||0)+1);return}if(Array.isArray(i)){r(i[1],null);return}if(i&&typeof i=="object")for(let[n,a]of Object.entries(i))r(a,n)}}function jl(e,t){let r=Ro(e);return i(e,[]);function i(p,m){let u=Pe(p);if(u==="scalar")return l(p,m);if(u==="scalar-array")return a(p,m);if(u==="object-array")return o(p,m);if(u==="object"){let c=Object.create(null);for(let[f,b]of Object.entries(p)){if(yi.has(f))throw new Error(`hypercms: rule key "${f}" is forbidden at "${m.join(".")||"<root>"}"`);c[f]=i(b,[...m,f])}return c}return null}function l(p,m){let u=m.length?m[m.length-1]:null,c=typeof u=="string"?u:"__value",f=d(m,c);if(f)return n(m,c,vi(f,c));let b=h(Ue(p,t,m,e),c);return b?n(m,c,vi(b,c)):n(m,c,`input[data-hcms-field="${ne(c)}"]@value`)}function n(p,m,u){if((r.get(m)||0)<2)return u;let c=`[data-hcms-path-shape="${ne(gr(p))}"]`,f=u.lastIndexOf("@"),b=f===-1?u:u.slice(0,f),g=f===-1?"":u.slice(f);return`${c} ${b}, ${b}${c}${g}`}function a(p,m){let u=Tl(p,m,t),c=u&&h(u.item,null)||h("@scalar-array-item",null),f=c?To(c):"input[data-hcms-field]@value";return[s(m,"[data-hcms-array-item]"),f]}function o(p,m){let[,u]=p,c=[...m,"*"],f=s(m,"[data-hcms-card]");if(u&&typeof u=="object"&&!Array.isArray(u)){let b=Object.create(null);for(let[g,k]of Object.entries(u)){if(yi.has(g))throw new Error(`hypercms: rule key "${g}" is forbidden at "${c.join(".")}"`);b[g]=i(k,[...c,g])}return[f,b]}return[f,i(u,[...c,0])]}function s(p,m){let u=p.length?p[p.length-1]:"",c=p.some(b=>b==="*"),f=p.join(".");return`${c?`[data-hcms-field="${ne(u)}"]`:`[data-hcms-path="${ne(f)}"]`} > .hcms-array-items > ${m}`}function h(p,m){if(!t)return null;let u=Q(t,p);if(!u)return null;let c=u.content||u;if(m){let f=c.querySelector(`[data-hcms-field="${ne(m)}"]`);if(f)return f}return c.querySelector("[data-hcms-field]")}function d(p,m){if(!t)return null;let u=p.map(f=>typeof f=="number"?"*":f).join("."),c=[p.join("."),u];for(let f=p.length-1;f>=0;f--){let b=p.slice(0,f).map(g=>typeof g=="number"?"*":g);Pe(Be(e,b))==="object"&&c.push(b.join(".")),b.push("*"),c.push(b.join("."))}for(let f of c){if(!f)continue;let b=Q(t,f);if(!b||!vt(b))continue;let g=b.content||b,k=g.querySelector(`[data-hcms-field="${ne(m)}"]`)||g.querySelector("[data-hcms-field]");if(k)return k}return null}}var qo=["50","100","200","300","400","500","600","700","800","900","950"],Oo=["red","orange","amber","yellow","lime","green","emerald","teal","cyan","sky","blue","indigo","violet","purple","fuchsia","pink","rose","slate","gray","zinc","neutral","stone","mauve","olive","mist","taupe"],Ne=Object.freeze([{id:"red-50",hue:"red",shade:"50",value:"oklch(97.1% 0.013 17.38)"},{id:"red-100",hue:"red",shade:"100",value:"oklch(93.6% 0.032 17.717)"},{id:"red-200",hue:"red",shade:"200",value:"oklch(88.5% 0.062 18.334)"},{id:"red-300",hue:"red",shade:"300",value:"oklch(80.8% 0.114 19.571)"},{id:"red-400",hue:"red",shade:"400",value:"oklch(70.4% 0.191 22.216)"},{id:"red-500",hue:"red",shade:"500",value:"oklch(63.7% 0.237 25.331)"},{id:"red-600",hue:"red",shade:"600",value:"oklch(57.7% 0.245 27.325)"},{id:"red-700",hue:"red",shade:"700",value:"oklch(50.5% 0.213 27.518)"},{id:"red-800",hue:"red",shade:"800",value:"oklch(44.4% 0.177 26.899)"},{id:"red-900",hue:"red",shade:"900",value:"oklch(39.6% 0.141 25.723)"},{id:"red-950",hue:"red",shade:"950",value:"oklch(25.8% 0.092 26.042)"},{id:"orange-50",hue:"orange",shade:"50",value:"oklch(98% 0.016 73.684)"},{id:"orange-100",hue:"orange",shade:"100",value:"oklch(95.4% 0.038 75.164)"},{id:"orange-200",hue:"orange",shade:"200",value:"oklch(90.1% 0.076 70.697)"},{id:"orange-300",hue:"orange",shade:"300",value:"oklch(83.7% 0.128 66.29)"},{id:"orange-400",hue:"orange",shade:"400",value:"oklch(75% 0.183 55.934)"},{id:"orange-500",hue:"orange",shade:"500",value:"oklch(70.5% 0.213 47.604)"},{id:"orange-600",hue:"orange",shade:"600",value:"oklch(64.6% 0.222 41.116)"},{id:"orange-700",hue:"orange",shade:"700",value:"oklch(55.3% 0.195 38.402)"},{id:"orange-800",hue:"orange",shade:"800",value:"oklch(47% 0.157 37.304)"},{id:"orange-900",hue:"orange",shade:"900",value:"oklch(40.8% 0.123 38.172)"},{id:"orange-950",hue:"orange",shade:"950",value:"oklch(26.6% 0.079 36.259)"},{id:"amber-50",hue:"amber",shade:"50",value:"oklch(98.7% 0.022 95.277)"},{id:"amber-100",hue:"amber",shade:"100",value:"oklch(96.2% 0.059 95.617)"},{id:"amber-200",hue:"amber",shade:"200",value:"oklch(92.4% 0.12 95.746)"},{id:"amber-300",hue:"amber",shade:"300",value:"oklch(87.9% 0.169 91.605)"},{id:"amber-400",hue:"amber",shade:"400",value:"oklch(82.8% 0.189 84.429)"},{id:"amber-500",hue:"amber",shade:"500",value:"oklch(76.9% 0.188 70.08)"},{id:"amber-600",hue:"amber",shade:"600",value:"oklch(66.6% 0.179 58.318)"},{id:"amber-700",hue:"amber",shade:"700",value:"oklch(55.5% 0.163 48.998)"},{id:"amber-800",hue:"amber",shade:"800",value:"oklch(47.3% 0.137 46.201)"},{id:"amber-900",hue:"amber",shade:"900",value:"oklch(41.4% 0.112 45.904)"},{id:"amber-950",hue:"amber",shade:"950",value:"oklch(27.9% 0.077 45.635)"},{id:"yellow-50",hue:"yellow",shade:"50",value:"oklch(98.7% 0.026 102.212)"},{id:"yellow-100",hue:"yellow",shade:"100",value:"oklch(97.3% 0.071 103.193)"},{id:"yellow-200",hue:"yellow",shade:"200",value:"oklch(94.5% 0.129 101.54)"},{id:"yellow-300",hue:"yellow",shade:"300",value:"oklch(90.5% 0.182 98.111)"},{id:"yellow-400",hue:"yellow",shade:"400",value:"oklch(85.2% 0.199 91.936)"},{id:"yellow-500",hue:"yellow",shade:"500",value:"oklch(79.5% 0.184 86.047)"},{id:"yellow-600",hue:"yellow",shade:"600",value:"oklch(68.1% 0.162 75.834)"},{id:"yellow-700",hue:"yellow",shade:"700",value:"oklch(55.4% 0.135 66.442)"},{id:"yellow-800",hue:"yellow",shade:"800",value:"oklch(47.6% 0.114 61.907)"},{id:"yellow-900",hue:"yellow",shade:"900",value:"oklch(42.1% 0.095 57.708)"},{id:"yellow-950",hue:"yellow",shade:"950",value:"oklch(28.6% 0.066 53.813)"},{id:"lime-50",hue:"lime",shade:"50",value:"oklch(98.6% 0.031 120.757)"},{id:"lime-100",hue:"lime",shade:"100",value:"oklch(96.7% 0.067 122.328)"},{id:"lime-200",hue:"lime",shade:"200",value:"oklch(93.8% 0.127 124.321)"},{id:"lime-300",hue:"lime",shade:"300",value:"oklch(89.7% 0.196 126.665)"},{id:"lime-400",hue:"lime",shade:"400",value:"oklch(84.1% 0.238 128.85)"},{id:"lime-500",hue:"lime",shade:"500",value:"oklch(76.8% 0.233 130.85)"},{id:"lime-600",hue:"lime",shade:"600",value:"oklch(64.8% 0.2 131.684)"},{id:"lime-700",hue:"lime",shade:"700",value:"oklch(53.2% 0.157 131.589)"},{id:"lime-800",hue:"lime",shade:"800",value:"oklch(45.3% 0.124 130.933)"},{id:"lime-900",hue:"lime",shade:"900",value:"oklch(40.5% 0.101 131.063)"},{id:"lime-950",hue:"lime",shade:"950",value:"oklch(27.4% 0.072 132.109)"},{id:"green-50",hue:"green",shade:"50",value:"oklch(98.2% 0.018 155.826)"},{id:"green-100",hue:"green",shade:"100",value:"oklch(96.2% 0.044 156.743)"},{id:"green-200",hue:"green",shade:"200",value:"oklch(92.5% 0.084 155.995)"},{id:"green-300",hue:"green",shade:"300",value:"oklch(87.1% 0.15 154.449)"},{id:"green-400",hue:"green",shade:"400",value:"oklch(79.2% 0.209 151.711)"},{id:"green-500",hue:"green",shade:"500",value:"oklch(72.3% 0.219 149.579)"},{id:"green-600",hue:"green",shade:"600",value:"oklch(62.7% 0.194 149.214)"},{id:"green-700",hue:"green",shade:"700",value:"oklch(52.7% 0.154 150.069)"},{id:"green-800",hue:"green",shade:"800",value:"oklch(44.8% 0.119 151.328)"},{id:"green-900",hue:"green",shade:"900",value:"oklch(39.3% 0.095 152.535)"},{id:"green-950",hue:"green",shade:"950",value:"oklch(26.6% 0.065 152.934)"},{id:"emerald-50",hue:"emerald",shade:"50",value:"oklch(97.9% 0.021 166.113)"},{id:"emerald-100",hue:"emerald",shade:"100",value:"oklch(95% 0.052 163.051)"},{id:"emerald-200",hue:"emerald",shade:"200",value:"oklch(90.5% 0.093 164.15)"},{id:"emerald-300",hue:"emerald",shade:"300",value:"oklch(84.5% 0.143 164.978)"},{id:"emerald-400",hue:"emerald",shade:"400",value:"oklch(76.5% 0.177 163.223)"},{id:"emerald-500",hue:"emerald",shade:"500",value:"oklch(69.6% 0.17 162.48)"},{id:"emerald-600",hue:"emerald",shade:"600",value:"oklch(59.6% 0.145 163.225)"},{id:"emerald-700",hue:"emerald",shade:"700",value:"oklch(50.8% 0.118 165.612)"},{id:"emerald-800",hue:"emerald",shade:"800",value:"oklch(43.2% 0.095 166.913)"},{id:"emerald-900",hue:"emerald",shade:"900",value:"oklch(37.8% 0.077 168.94)"},{id:"emerald-950",hue:"emerald",shade:"950",value:"oklch(26.2% 0.051 172.552)"},{id:"teal-50",hue:"teal",shade:"50",value:"oklch(98.4% 0.014 180.72)"},{id:"teal-100",hue:"teal",shade:"100",value:"oklch(95.3% 0.051 180.801)"},{id:"teal-200",hue:"teal",shade:"200",value:"oklch(91% 0.096 180.426)"},{id:"teal-300",hue:"teal",shade:"300",value:"oklch(85.5% 0.138 181.071)"},{id:"teal-400",hue:"teal",shade:"400",value:"oklch(77.7% 0.152 181.912)"},{id:"teal-500",hue:"teal",shade:"500",value:"oklch(70.4% 0.14 182.503)"},{id:"teal-600",hue:"teal",shade:"600",value:"oklch(60% 0.118 184.704)"},{id:"teal-700",hue:"teal",shade:"700",value:"oklch(51.1% 0.096 186.391)"},{id:"teal-800",hue:"teal",shade:"800",value:"oklch(43.7% 0.078 188.216)"},{id:"teal-900",hue:"teal",shade:"900",value:"oklch(38.6% 0.063 188.416)"},{id:"teal-950",hue:"teal",shade:"950",value:"oklch(27.7% 0.046 192.524)"},{id:"cyan-50",hue:"cyan",shade:"50",value:"oklch(98.4% 0.019 200.873)"},{id:"cyan-100",hue:"cyan",shade:"100",value:"oklch(95.6% 0.045 203.388)"},{id:"cyan-200",hue:"cyan",shade:"200",value:"oklch(91.7% 0.08 205.041)"},{id:"cyan-300",hue:"cyan",shade:"300",value:"oklch(86.5% 0.127 207.078)"},{id:"cyan-400",hue:"cyan",shade:"400",value:"oklch(78.9% 0.154 211.53)"},{id:"cyan-500",hue:"cyan",shade:"500",value:"oklch(71.5% 0.143 215.221)"},{id:"cyan-600",hue:"cyan",shade:"600",value:"oklch(60.9% 0.126 221.723)"},{id:"cyan-700",hue:"cyan",shade:"700",value:"oklch(52% 0.105 223.128)"},{id:"cyan-800",hue:"cyan",shade:"800",value:"oklch(45% 0.085 224.283)"},{id:"cyan-900",hue:"cyan",shade:"900",value:"oklch(39.8% 0.07 227.392)"},{id:"cyan-950",hue:"cyan",shade:"950",value:"oklch(30.2% 0.056 229.695)"},{id:"sky-50",hue:"sky",shade:"50",value:"oklch(97.7% 0.013 236.62)"},{id:"sky-100",hue:"sky",shade:"100",value:"oklch(95.1% 0.026 236.824)"},{id:"sky-200",hue:"sky",shade:"200",value:"oklch(90.1% 0.058 230.902)"},{id:"sky-300",hue:"sky",shade:"300",value:"oklch(82.8% 0.111 230.318)"},{id:"sky-400",hue:"sky",shade:"400",value:"oklch(74.6% 0.16 232.661)"},{id:"sky-500",hue:"sky",shade:"500",value:"oklch(68.5% 0.169 237.323)"},{id:"sky-600",hue:"sky",shade:"600",value:"oklch(58.8% 0.158 241.966)"},{id:"sky-700",hue:"sky",shade:"700",value:"oklch(50% 0.134 242.749)"},{id:"sky-800",hue:"sky",shade:"800",value:"oklch(44.3% 0.11 240.79)"},{id:"sky-900",hue:"sky",shade:"900",value:"oklch(39.1% 0.09 240.876)"},{id:"sky-950",hue:"sky",shade:"950",value:"oklch(29.3% 0.066 243.157)"},{id:"blue-50",hue:"blue",shade:"50",value:"oklch(97% 0.014 254.604)"},{id:"blue-100",hue:"blue",shade:"100",value:"oklch(93.2% 0.032 255.585)"},{id:"blue-200",hue:"blue",shade:"200",value:"oklch(88.2% 0.059 254.128)"},{id:"blue-300",hue:"blue",shade:"300",value:"oklch(80.9% 0.105 251.813)"},{id:"blue-400",hue:"blue",shade:"400",value:"oklch(70.7% 0.165 254.624)"},{id:"blue-500",hue:"blue",shade:"500",value:"oklch(62.3% 0.214 259.815)"},{id:"blue-600",hue:"blue",shade:"600",value:"oklch(54.6% 0.245 262.881)"},{id:"blue-700",hue:"blue",shade:"700",value:"oklch(48.8% 0.243 264.376)"},{id:"blue-800",hue:"blue",shade:"800",value:"oklch(42.4% 0.199 265.638)"},{id:"blue-900",hue:"blue",shade:"900",value:"oklch(37.9% 0.146 265.522)"},{id:"blue-950",hue:"blue",shade:"950",value:"oklch(28.2% 0.091 267.935)"},{id:"indigo-50",hue:"indigo",shade:"50",value:"oklch(96.2% 0.018 272.314)"},{id:"indigo-100",hue:"indigo",shade:"100",value:"oklch(93% 0.034 272.788)"},{id:"indigo-200",hue:"indigo",shade:"200",value:"oklch(87% 0.065 274.039)"},{id:"indigo-300",hue:"indigo",shade:"300",value:"oklch(78.5% 0.115 274.713)"},{id:"indigo-400",hue:"indigo",shade:"400",value:"oklch(67.3% 0.182 276.935)"},{id:"indigo-500",hue:"indigo",shade:"500",value:"oklch(58.5% 0.233 277.117)"},{id:"indigo-600",hue:"indigo",shade:"600",value:"oklch(51.1% 0.262 276.966)"},{id:"indigo-700",hue:"indigo",shade:"700",value:"oklch(45.7% 0.24 277.023)"},{id:"indigo-800",hue:"indigo",shade:"800",value:"oklch(39.8% 0.195 277.366)"},{id:"indigo-900",hue:"indigo",shade:"900",value:"oklch(35.9% 0.144 278.697)"},{id:"indigo-950",hue:"indigo",shade:"950",value:"oklch(25.7% 0.09 281.288)"},{id:"violet-50",hue:"violet",shade:"50",value:"oklch(96.9% 0.016 293.756)"},{id:"violet-100",hue:"violet",shade:"100",value:"oklch(94.3% 0.029 294.588)"},{id:"violet-200",hue:"violet",shade:"200",value:"oklch(89.4% 0.057 293.283)"},{id:"violet-300",hue:"violet",shade:"300",value:"oklch(81.1% 0.111 293.571)"},{id:"violet-400",hue:"violet",shade:"400",value:"oklch(70.2% 0.183 293.541)"},{id:"violet-500",hue:"violet",shade:"500",value:"oklch(60.6% 0.25 292.717)"},{id:"violet-600",hue:"violet",shade:"600",value:"oklch(54.1% 0.281 293.009)"},{id:"violet-700",hue:"violet",shade:"700",value:"oklch(49.1% 0.27 292.581)"},{id:"violet-800",hue:"violet",shade:"800",value:"oklch(43.2% 0.232 292.759)"},{id:"violet-900",hue:"violet",shade:"900",value:"oklch(38% 0.189 293.745)"},{id:"violet-950",hue:"violet",shade:"950",value:"oklch(28.3% 0.141 291.089)"},{id:"purple-50",hue:"purple",shade:"50",value:"oklch(97.7% 0.014 308.299)"},{id:"purple-100",hue:"purple",shade:"100",value:"oklch(94.6% 0.033 307.174)"},{id:"purple-200",hue:"purple",shade:"200",value:"oklch(90.2% 0.063 306.703)"},{id:"purple-300",hue:"purple",shade:"300",value:"oklch(82.7% 0.119 306.383)"},{id:"purple-400",hue:"purple",shade:"400",value:"oklch(71.4% 0.203 305.504)"},{id:"purple-500",hue:"purple",shade:"500",value:"oklch(62.7% 0.265 303.9)"},{id:"purple-600",hue:"purple",shade:"600",value:"oklch(55.8% 0.288 302.321)"},{id:"purple-700",hue:"purple",shade:"700",value:"oklch(49.6% 0.265 301.924)"},{id:"purple-800",hue:"purple",shade:"800",value:"oklch(43.8% 0.218 303.724)"},{id:"purple-900",hue:"purple",shade:"900",value:"oklch(38.1% 0.176 304.987)"},{id:"purple-950",hue:"purple",shade:"950",value:"oklch(29.1% 0.149 302.717)"},{id:"fuchsia-50",hue:"fuchsia",shade:"50",value:"oklch(97.7% 0.017 320.058)"},{id:"fuchsia-100",hue:"fuchsia",shade:"100",value:"oklch(95.2% 0.037 318.852)"},{id:"fuchsia-200",hue:"fuchsia",shade:"200",value:"oklch(90.3% 0.076 319.62)"},{id:"fuchsia-300",hue:"fuchsia",shade:"300",value:"oklch(83.3% 0.145 321.434)"},{id:"fuchsia-400",hue:"fuchsia",shade:"400",value:"oklch(74% 0.238 322.16)"},{id:"fuchsia-500",hue:"fuchsia",shade:"500",value:"oklch(66.7% 0.295 322.15)"},{id:"fuchsia-600",hue:"fuchsia",shade:"600",value:"oklch(59.1% 0.293 322.896)"},{id:"fuchsia-700",hue:"fuchsia",shade:"700",value:"oklch(51.8% 0.253 323.949)"},{id:"fuchsia-800",hue:"fuchsia",shade:"800",value:"oklch(45.2% 0.211 324.591)"},{id:"fuchsia-900",hue:"fuchsia",shade:"900",value:"oklch(40.1% 0.17 325.612)"},{id:"fuchsia-950",hue:"fuchsia",shade:"950",value:"oklch(29.3% 0.136 325.661)"},{id:"pink-50",hue:"pink",shade:"50",value:"oklch(97.1% 0.014 343.198)"},{id:"pink-100",hue:"pink",shade:"100",value:"oklch(94.8% 0.028 342.258)"},{id:"pink-200",hue:"pink",shade:"200",value:"oklch(89.9% 0.061 343.231)"},{id:"pink-300",hue:"pink",shade:"300",value:"oklch(82.3% 0.12 346.018)"},{id:"pink-400",hue:"pink",shade:"400",value:"oklch(71.8% 0.202 349.761)"},{id:"pink-500",hue:"pink",shade:"500",value:"oklch(65.6% 0.241 354.308)"},{id:"pink-600",hue:"pink",shade:"600",value:"oklch(59.2% 0.249 0.584)"},{id:"pink-700",hue:"pink",shade:"700",value:"oklch(52.5% 0.223 3.958)"},{id:"pink-800",hue:"pink",shade:"800",value:"oklch(45.9% 0.187 3.815)"},{id:"pink-900",hue:"pink",shade:"900",value:"oklch(40.8% 0.153 2.432)"},{id:"pink-950",hue:"pink",shade:"950",value:"oklch(28.4% 0.109 3.907)"},{id:"rose-50",hue:"rose",shade:"50",value:"oklch(96.9% 0.015 12.422)"},{id:"rose-100",hue:"rose",shade:"100",value:"oklch(94.1% 0.03 12.58)"},{id:"rose-200",hue:"rose",shade:"200",value:"oklch(89.2% 0.058 10.001)"},{id:"rose-300",hue:"rose",shade:"300",value:"oklch(81% 0.117 11.638)"},{id:"rose-400",hue:"rose",shade:"400",value:"oklch(71.2% 0.194 13.428)"},{id:"rose-500",hue:"rose",shade:"500",value:"oklch(64.5% 0.246 16.439)"},{id:"rose-600",hue:"rose",shade:"600",value:"oklch(58.6% 0.253 17.585)"},{id:"rose-700",hue:"rose",shade:"700",value:"oklch(51.4% 0.222 16.935)"},{id:"rose-800",hue:"rose",shade:"800",value:"oklch(45.5% 0.188 13.697)"},{id:"rose-900",hue:"rose",shade:"900",value:"oklch(41% 0.159 10.272)"},{id:"rose-950",hue:"rose",shade:"950",value:"oklch(27.1% 0.105 12.094)"},{id:"slate-50",hue:"slate",shade:"50",value:"oklch(98.4% 0.003 247.858)"},{id:"slate-100",hue:"slate",shade:"100",value:"oklch(96.8% 0.007 247.896)"},{id:"slate-200",hue:"slate",shade:"200",value:"oklch(92.9% 0.013 255.508)"},{id:"slate-300",hue:"slate",shade:"300",value:"oklch(86.9% 0.022 252.894)"},{id:"slate-400",hue:"slate",shade:"400",value:"oklch(70.4% 0.04 256.788)"},{id:"slate-500",hue:"slate",shade:"500",value:"oklch(55.4% 0.046 257.417)"},{id:"slate-600",hue:"slate",shade:"600",value:"oklch(44.6% 0.043 257.281)"},{id:"slate-700",hue:"slate",shade:"700",value:"oklch(37.2% 0.044 257.287)"},{id:"slate-800",hue:"slate",shade:"800",value:"oklch(27.9% 0.041 260.031)"},{id:"slate-900",hue:"slate",shade:"900",value:"oklch(20.8% 0.042 265.755)"},{id:"slate-950",hue:"slate",shade:"950",value:"oklch(12.9% 0.042 264.695)"},{id:"gray-50",hue:"gray",shade:"50",value:"oklch(98.5% 0.002 247.839)"},{id:"gray-100",hue:"gray",shade:"100",value:"oklch(96.7% 0.003 264.542)"},{id:"gray-200",hue:"gray",shade:"200",value:"oklch(92.8% 0.006 264.531)"},{id:"gray-300",hue:"gray",shade:"300",value:"oklch(87.2% 0.01 258.338)"},{id:"gray-400",hue:"gray",shade:"400",value:"oklch(70.7% 0.022 261.325)"},{id:"gray-500",hue:"gray",shade:"500",value:"oklch(55.1% 0.027 264.364)"},{id:"gray-600",hue:"gray",shade:"600",value:"oklch(44.6% 0.03 256.802)"},{id:"gray-700",hue:"gray",shade:"700",value:"oklch(37.3% 0.034 259.733)"},{id:"gray-800",hue:"gray",shade:"800",value:"oklch(27.8% 0.033 256.848)"},{id:"gray-900",hue:"gray",shade:"900",value:"oklch(21% 0.034 264.665)"},{id:"gray-950",hue:"gray",shade:"950",value:"oklch(13% 0.028 261.692)"},{id:"zinc-50",hue:"zinc",shade:"50",value:"oklch(98.5% 0 none)"},{id:"zinc-100",hue:"zinc",shade:"100",value:"oklch(96.7% 0.001 286.375)"},{id:"zinc-200",hue:"zinc",shade:"200",value:"oklch(92% 0.004 286.32)"},{id:"zinc-300",hue:"zinc",shade:"300",value:"oklch(87.1% 0.006 286.286)"},{id:"zinc-400",hue:"zinc",shade:"400",value:"oklch(70.5% 0.015 286.067)"},{id:"zinc-500",hue:"zinc",shade:"500",value:"oklch(55.2% 0.016 285.938)"},{id:"zinc-600",hue:"zinc",shade:"600",value:"oklch(44.2% 0.017 285.786)"},{id:"zinc-700",hue:"zinc",shade:"700",value:"oklch(37% 0.013 285.805)"},{id:"zinc-800",hue:"zinc",shade:"800",value:"oklch(27.4% 0.006 286.033)"},{id:"zinc-900",hue:"zinc",shade:"900",value:"oklch(21% 0.006 285.885)"},{id:"zinc-950",hue:"zinc",shade:"950",value:"oklch(14.1% 0.005 285.823)"},{id:"neutral-50",hue:"neutral",shade:"50",value:"oklch(98.5% 0 none)"},{id:"neutral-100",hue:"neutral",shade:"100",value:"oklch(97% 0 none)"},{id:"neutral-200",hue:"neutral",shade:"200",value:"oklch(92.2% 0 none)"},{id:"neutral-300",hue:"neutral",shade:"300",value:"oklch(87% 0 none)"},{id:"neutral-400",hue:"neutral",shade:"400",value:"oklch(70.8% 0 none)"},{id:"neutral-500",hue:"neutral",shade:"500",value:"oklch(55.6% 0 none)"},{id:"neutral-600",hue:"neutral",shade:"600",value:"oklch(43.9% 0 none)"},{id:"neutral-700",hue:"neutral",shade:"700",value:"oklch(37.1% 0 none)"},{id:"neutral-800",hue:"neutral",shade:"800",value:"oklch(26.9% 0 none)"},{id:"neutral-900",hue:"neutral",shade:"900",value:"oklch(20.5% 0 none)"},{id:"neutral-950",hue:"neutral",shade:"950",value:"oklch(14.5% 0 none)"},{id:"stone-50",hue:"stone",shade:"50",value:"oklch(98.5% 0.001 106.423)"},{id:"stone-100",hue:"stone",shade:"100",value:"oklch(97% 0.001 106.424)"},{id:"stone-200",hue:"stone",shade:"200",value:"oklch(92.3% 0.003 48.717)"},{id:"stone-300",hue:"stone",shade:"300",value:"oklch(86.9% 0.005 56.366)"},{id:"stone-400",hue:"stone",shade:"400",value:"oklch(70.9% 0.01 56.259)"},{id:"stone-500",hue:"stone",shade:"500",value:"oklch(55.3% 0.013 58.071)"},{id:"stone-600",hue:"stone",shade:"600",value:"oklch(44.4% 0.011 73.639)"},{id:"stone-700",hue:"stone",shade:"700",value:"oklch(37.4% 0.01 67.558)"},{id:"stone-800",hue:"stone",shade:"800",value:"oklch(26.8% 0.007 34.298)"},{id:"stone-900",hue:"stone",shade:"900",value:"oklch(21.6% 0.006 56.043)"},{id:"stone-950",hue:"stone",shade:"950",value:"oklch(14.7% 0.004 49.25)"},{id:"mauve-50",hue:"mauve",shade:"50",value:"oklch(98.5% 0 none)"},{id:"mauve-100",hue:"mauve",shade:"100",value:"oklch(96% 0.003 325.6)"},{id:"mauve-200",hue:"mauve",shade:"200",value:"oklch(92.2% 0.005 325.62)"},{id:"mauve-300",hue:"mauve",shade:"300",value:"oklch(86.5% 0.012 325.68)"},{id:"mauve-400",hue:"mauve",shade:"400",value:"oklch(71.1% 0.019 323.02)"},{id:"mauve-500",hue:"mauve",shade:"500",value:"oklch(54.2% 0.034 322.5)"},{id:"mauve-600",hue:"mauve",shade:"600",value:"oklch(43.5% 0.029 321.78)"},{id:"mauve-700",hue:"mauve",shade:"700",value:"oklch(36.4% 0.029 323.89)"},{id:"mauve-800",hue:"mauve",shade:"800",value:"oklch(26.3% 0.024 320.12)"},{id:"mauve-900",hue:"mauve",shade:"900",value:"oklch(21.2% 0.019 322.12)"},{id:"mauve-950",hue:"mauve",shade:"950",value:"oklch(14.5% 0.008 326)"},{id:"olive-50",hue:"olive",shade:"50",value:"oklch(98.8% 0.003 106.5)"},{id:"olive-100",hue:"olive",shade:"100",value:"oklch(96.6% 0.005 106.5)"},{id:"olive-200",hue:"olive",shade:"200",value:"oklch(93% 0.007 106.5)"},{id:"olive-300",hue:"olive",shade:"300",value:"oklch(88% 0.011 106.6)"},{id:"olive-400",hue:"olive",shade:"400",value:"oklch(73.7% 0.021 106.9)"},{id:"olive-500",hue:"olive",shade:"500",value:"oklch(58% 0.031 107.3)"},{id:"olive-600",hue:"olive",shade:"600",value:"oklch(46.6% 0.025 107.3)"},{id:"olive-700",hue:"olive",shade:"700",value:"oklch(39.4% 0.023 107.4)"},{id:"olive-800",hue:"olive",shade:"800",value:"oklch(28.6% 0.016 107.4)"},{id:"olive-900",hue:"olive",shade:"900",value:"oklch(22.8% 0.013 107.4)"},{id:"olive-950",hue:"olive",shade:"950",value:"oklch(15.3% 0.006 107.1)"},{id:"mist-50",hue:"mist",shade:"50",value:"oklch(98.7% 0.002 197.1)"},{id:"mist-100",hue:"mist",shade:"100",value:"oklch(96.3% 0.002 197.1)"},{id:"mist-200",hue:"mist",shade:"200",value:"oklch(92.5% 0.005 214.3)"},{id:"mist-300",hue:"mist",shade:"300",value:"oklch(87.2% 0.007 219.6)"},{id:"mist-400",hue:"mist",shade:"400",value:"oklch(72.3% 0.014 214.4)"},{id:"mist-500",hue:"mist",shade:"500",value:"oklch(56% 0.021 213.5)"},{id:"mist-600",hue:"mist",shade:"600",value:"oklch(45% 0.017 213.2)"},{id:"mist-700",hue:"mist",shade:"700",value:"oklch(37.8% 0.015 216)"},{id:"mist-800",hue:"mist",shade:"800",value:"oklch(27.5% 0.011 216.9)"},{id:"mist-900",hue:"mist",shade:"900",value:"oklch(21.8% 0.008 223.9)"},{id:"mist-950",hue:"mist",shade:"950",value:"oklch(14.8% 0.004 228.8)"},{id:"taupe-50",hue:"taupe",shade:"50",value:"oklch(98.6% 0.002 67.8)"},{id:"taupe-100",hue:"taupe",shade:"100",value:"oklch(96% 0.002 17.2)"},{id:"taupe-200",hue:"taupe",shade:"200",value:"oklch(92.2% 0.005 34.3)"},{id:"taupe-300",hue:"taupe",shade:"300",value:"oklch(86.8% 0.007 39.5)"},{id:"taupe-400",hue:"taupe",shade:"400",value:"oklch(71.4% 0.014 41.2)"},{id:"taupe-500",hue:"taupe",shade:"500",value:"oklch(54.7% 0.021 43.1)"},{id:"taupe-600",hue:"taupe",shade:"600",value:"oklch(43.8% 0.017 39.3)"},{id:"taupe-700",hue:"taupe",shade:"700",value:"oklch(36.7% 0.016 35.7)"},{id:"taupe-800",hue:"taupe",shade:"800",value:"oklch(26.8% 0.011 36.5)"},{id:"taupe-900",hue:"taupe",shade:"900",value:"oklch(21.4% 0.009 43.1)"},{id:"taupe-950",hue:"taupe",shade:"950",value:"oklch(14.7% 0.004 49.3)"}]),$o=12,xi=new Map(Ne.map(e=>[e.id,e])),Lo=new Map(Ne.map(e=>[e.value,e])),jo=/^#(?:[0-9a-f]{3,4}|[0-9a-f]{6}|[0-9a-f]{8})$/i,Mo=/^(?:rgba?|hsla?|hwb|lab|lch|oklab|oklch|color|color-mix|light-dark)\(.*\)$/i,No=/[;{}<>\\]|url\s*\(/i,wi=new WeakMap;function kr(e){let t=wi.get(e);return t||wi.set(e,t=new Set),t}function Ml(e,t){let r=String(e??"").trim();if(!r||No.test(r)||eo(r))return!1;let i=t?.defaultView?.CSS;return i&&typeof i.supports=="function"?i.supports("color",r):jo.test(r)||Mo.test(r)||/^[a-z]+$/i.test(r)}function Nl(e){return`${e.hue[0].toUpperCase()}${e.hue.slice(1)} ${e.shade}`}function Xe(e){return{id:e.id,label:Nl(e),value:e.value}}function zl(e){let t=String(e??"").trim(),r=Lo.get(t);return r?Nl(r):t}function zo(e,t){if(!e||e.length===0)return{swatches:Ne.map(Xe),custom:!0,full:!0};let r=[],i=new Set,l=!1,n=a=>{i.has(a.value)||(i.add(a.value),r.push(a))};for(let a of e)a==="custom"?l=!0:xi.has(a)?n(Xe(xi.get(a))):Oo.includes(a)?Ne.filter(o=>o.hue===a).forEach(o=>n(Xe(o))):qo.includes(a)?Ne.filter(o=>o.shade===a).forEach(o=>n(Xe(o))):Ml(a,t)?n({id:null,label:a,value:a}):console.warn(`[hypercms] color option "${a}" is not a Tailwind color, hue, shade, CSS color or "custom"; ignoring it`);return{swatches:r,custom:l,full:!1}}function Fo(e,{value:t,tokens:r,path:i,doc:l}){let{swatches:n,custom:a,full:o}=zo(r,l),s=t==null?"":String(t).trim();s&&!n.some(m=>m.value===s)&&n.unshift({id:null,label:zl(s),value:s});let h=n.length<=$o;e.setAttribute("data-hcms-color-path",i),e.classList.toggle("hcms-color--inline",h);let d=e.querySelector(".hcms-color-grid");if(d){d.classList.toggle("hcms-color-grid--full",o);for(let m of n){let u=l.createElement("button");u.type="button",u.className="hcms-color-swatch",u.setAttribute("data-hcms-action","color-pick"),u.setAttribute("data-hcms-color",m.value),u.setAttribute("aria-label",m.label),u.setAttribute("title",m.id||m.label),u.setAttribute("style",`--hcms-swatch: ${m.value}`),d.appendChild(u)}}a||e.querySelector(".hcms-color-custom")?.remove();let p=e.querySelector(".hcms-color-current");h&&p&&(p.removeAttribute("data-hcms-action"),p.removeAttribute("aria-expanded")),vr(e,h||kr(l).has(i)),Fl(e,s)}function Fl(e,t){let r=t==null?"":String(t).trim();e.toggleAttribute("data-hcms-color-empty",r==="");let i=e.querySelector(".hcms-color-chip");i&&(r?i.setAttribute("style",`--hcms-swatch: ${r}`):i.removeAttribute("style"));let l=e.querySelector(".hcms-color-name");l&&(l.textContent=r?zl(r):"Not set"),e.querySelectorAll(".hcms-color-swatch").forEach(o=>{o.setAttribute("aria-pressed",String(o.getAttribute("data-hcms-color")===r))});let n=e.querySelector('[data-hcms-color-input="text"]');n&&n.ownerDocument?.activeElement!==n&&(n.value=r);let a=e.querySelector('[data-hcms-color-input="native"]');a&&/^#[0-9a-f]{6}$/i.test(r)&&(a.value=r.toLowerCase())}function vr(e,t){let r=e.querySelector(".hcms-color-panel");r&&(r.hidden=!t);let i=e.querySelector(".hcms-color-current[data-hcms-action]");i&&i.setAttribute("aria-expanded",String(t))}function yt(e){return e?.closest?e.closest("[data-hcms-color-path]"):null}function Do(e){return e.querySelector('input[type="hidden"][data-hcms-field]')}function Dl(e,t){let r=e.querySelector(":scope > .hcms-error");r&&(r.textContent=t||"",r.hidden=!t)}function yr(e,t){let r=Do(e);return!r||(Dl(e,null),Fl(e,t),r.value===t)?null:(r.value=t,r)}function Bo(e){let t=yt(e);return t?(t.classList.contains("hcms-color--inline")||(kr(t.ownerDocument).delete(t.getAttribute("data-hcms-color-path")),vr(t,!1)),yr(t,e.getAttribute("data-hcms-color")||"")):null}function Io(e){let t=yt(e);return t?yr(t,""):null}function Po(e){let t=yt(e);if(!t)return;let r=t.getAttribute("data-hcms-color-path"),i=kr(t.ownerDocument),l=!i.has(r);l?i.add(r):i.delete(r),vr(t,l)}function Ho(e){let t=yt(e);if(!t)return null;let r=String(e.value||"").trim();return r!==""&&!Ml(r,t.ownerDocument)?(Dl(t,`"${r}" is not a CSS color`),null):yr(t,r)}var Bl=["sans","serif","display","mono"],N="ui-sans-serif, system-ui, sans-serif",Z="ui-serif, Georgia, serif",de="ui-monospace, SFMono-Regular, Menlo, monospace",ee=Object.freeze([{slug:"inter",family:"Inter",group:"sans",fallback:N,request:"ital,wght@0,100..900;1,100..900"},{slug:"roboto",family:"Roboto",group:"sans",fallback:N,request:"ital,wght@0,100..900;1,100..900"},{slug:"open-sans",family:"Open Sans",group:"sans",fallback:N,request:"ital,wght@0,300..800;1,300..800"},{slug:"lato",family:"Lato",group:"sans",fallback:N,request:"ital,wght@0,100;0,300;0,400;0,700;0,900;1,100;1,300;1,400;1,700;1,900"},{slug:"montserrat",family:"Montserrat",group:"sans",fallback:N,request:"ital,wght@0,100..900;1,100..900"},{slug:"poppins",family:"Poppins",group:"sans",fallback:N,request:"ital,wght@0,100;0,200;0,300;0,400;0,500;0,600;0,700;0,800;0,900;1,100;1,200;1,300;1,400;1,500;1,600;1,700;1,800;1,900"},{slug:"nunito",family:"Nunito",group:"sans",fallback:N,request:"ital,wght@0,200..1000;1,200..1000"},{slug:"raleway",family:"Raleway",group:"sans",fallback:N,request:"ital,wght@0,100..900;1,100..900"},{slug:"dm-sans",family:"DM Sans",group:"sans",fallback:N,request:"ital,wght@0,100..1000;1,100..1000"},{slug:"work-sans",family:"Work Sans",group:"sans",fallback:N,request:"ital,wght@0,100..900;1,100..900"},{slug:"source-sans-3",family:"Source Sans 3",group:"sans",fallback:N,request:"ital,wght@0,200..900;1,200..900"},{slug:"noto-sans",family:"Noto Sans",group:"sans",fallback:N,request:"ital,wght@0,100..900;1,100..900"},{slug:"plus-jakarta-sans",family:"Plus Jakarta Sans",group:"sans",fallback:N,request:"ital,wght@0,200..800;1,200..800"},{slug:"manrope",family:"Manrope",group:"sans",fallback:N,request:"wght@200..800"},{slug:"outfit",family:"Outfit",group:"sans",fallback:N,request:"wght@100..900"},{slug:"space-grotesk",family:"Space Grotesk",group:"sans",fallback:N,request:"wght@300..700"},{slug:"rubik",family:"Rubik",group:"sans",fallback:N,request:"ital,wght@0,300..900;1,300..900"},{slug:"ubuntu",family:"Ubuntu",group:"sans",fallback:N,request:"ital,wght@0,300;0,400;0,500;0,700;1,300;1,400;1,500;1,700"},{slug:"merriweather",family:"Merriweather",group:"serif",fallback:Z,request:"ital,wght@0,300..900;1,300..900"},{slug:"lora",family:"Lora",group:"serif",fallback:Z,request:"ital,wght@0,400..700;1,400..700"},{slug:"playfair-display",family:"Playfair Display",group:"serif",fallback:Z,request:"ital,wght@0,400..900;1,400..900"},{slug:"libre-baskerville",family:"Libre Baskerville",group:"serif",fallback:Z,request:"ital,wght@0,400..700;1,400..700"},{slug:"source-serif-4",family:"Source Serif 4",group:"serif",fallback:Z,request:"ital,wght@0,200..900;1,200..900"},{slug:"pt-serif",family:"PT Serif",group:"serif",fallback:Z,request:"ital,wght@0,400;0,700;1,400;1,700"},{slug:"noto-serif",family:"Noto Serif",group:"serif",fallback:Z,request:"ital,wght@0,100..900;1,100..900"},{slug:"crimson-pro",family:"Crimson Pro",group:"serif",fallback:Z,request:"ital,wght@0,200..900;1,200..900"},{slug:"eb-garamond",family:"EB Garamond",group:"serif",fallback:Z,request:"ital,wght@0,400..800;1,400..800"},{slug:"cormorant-garamond",family:"Cormorant Garamond",group:"serif",fallback:Z,request:"ital,wght@0,300..700;1,300..700"},{slug:"bebas-neue",family:"Bebas Neue",group:"display",fallback:N,request:"wght@400"},{slug:"oswald",family:"Oswald",group:"display",fallback:N,request:"wght@200..700"},{slug:"anton",family:"Anton",group:"display",fallback:N,request:"wght@400"},{slug:"abril-fatface",family:"Abril Fatface",group:"display",fallback:Z,request:"wght@400"},{slug:"lobster",family:"Lobster",group:"display",fallback:"cursive",request:"wght@400"},{slug:"bungee",family:"Bungee",group:"display",fallback:N,request:"wght@400"},{slug:"roboto-mono",family:"Roboto Mono",group:"mono",fallback:de,request:"ital,wght@0,100..700;1,100..700"},{slug:"jetbrains-mono",family:"JetBrains Mono",group:"mono",fallback:de,request:"ital,wght@0,100..800;1,100..800"},{slug:"fira-code",family:"Fira Code",group:"mono",fallback:de,request:"wght@300..700"},{slug:"source-code-pro",family:"Source Code Pro",group:"mono",fallback:de,request:"ital,wght@0,200..900;1,200..900"},{slug:"ibm-plex-mono",family:"IBM Plex Mono",group:"mono",fallback:de,request:"ital,wght@0,100;0,200;0,300;0,400;0,500;0,600;0,700;1,100;1,200;1,300;1,400;1,500;1,600;1,700"},{slug:"space-mono",family:"Space Mono",group:"mono",fallback:de,request:"ital,wght@0,400;0,700;1,400;1,700"}]),ze=Object.freeze({slug:"system",family:null,group:"sans",fallback:"system-ui, sans-serif"}),Uo=new Map(ee.map(e=>[e.family.toLowerCase(),e])),Hh=new Map(ee.map(e=>[e.slug,e]));function Ye(e){return!e||e.slug==="system"?ze.fallback:`'${e.family}', ${e.fallback}`}function Il(e){return e==null?null:String(e).split(",")[0].trim().replace(/^['"]|['"]$/g,"")||null}function Zt(e){let t=new Map;if(!e)return t;let r=String(e).split("?")[1]||"";for(let i of r.split("&")){if(!i.startsWith("family="))continue;let l=decodeURIComponent(i.slice(7).split(":")[0].replace(/\+/g," ")).trim();l&&t.set(l.toLowerCase(),i)}return t}function Vo(e,t){let r=Zt(e),i=Zt(t);return r.size===i.size&&[...r.keys()].every(l=>i.has(l))}function Pl(e){let t=Il(e);return t&&Uo.get(t.toLowerCase())||null}function Hl(e,t=null){let r=Zt(t),i=new Map;for(let l of e||[]){let n=Pl(l);if(n){i.set(n.family.toLowerCase(),`family=${n.family.replace(/ /g,"+")}:${n.request}`);continue}let a=Il(l),o=a&&r.get(a.toLowerCase());o&&i.set(a.toLowerCase(),o)}return i.size===0?null:`https://fonts.googleapis.com/css2?${[...i.entries()].sort(([l],[n])=>l.localeCompare(n)).map(([,l])=>l).join("&")}&display=swap`}var Wo={sans:"Sans",serif:"Serif",display:"Display",mono:"Mono"},Go="link[data-hcms-font-preview]",Ai=new WeakMap;function xr(e){let t=Ai.get(e);return t||Ai.set(e,t=new Set),t}function Jo(e){return String(e??"").trim()===ze.fallback}function Ul(e){let t=String(e??"").trim();if(!t)return"Not set";if(Jo(t))return"System";let r=Pl(t);return r?r.family:"Current font"}function Xo(e){if(!e||e.length===0)return{system:!0,fonts:[...ee]};let t=!1,r=new Set;for(let i of e)i==="system"?t=!0:Bl.includes(i)?ee.filter(l=>l.group===i).forEach(l=>r.add(l)):ee.some(l=>l.slug===i)?r.add(ee.find(l=>l.slug===i)):console.warn(`[hypercms] font option "${i}" is not a catalog font, a group or "system"; ignoring it`);return{system:t,fonts:ee.filter(i=>r.has(i))}}function jt(e,{value:t,label:r,family:i}){let l=e.createElement("button");return l.type="button",l.className="hcms-font-option",l.setAttribute("role","option"),l.setAttribute("data-hcms-action","font-pick"),l.setAttribute("data-hcms-font",t),l.setAttribute("style",`font-family: ${i}`),l.textContent=r,l}function Ko(e,{value:t,tokens:r,path:i,doc:l}){let{system:n,fonts:a}=Xo(r),o=t==null?"":String(t).trim();e.setAttribute("data-hcms-font-path",i);let s=e.querySelector(".hcms-font-list");if(s){let h=new Set([...n?[ze.fallback]:[],...a.map(Ye)]);o&&!h.has(o)&&s.appendChild(jt(l,{value:o,label:Ul(o),family:o})),n&&s.appendChild(jt(l,{value:ze.fallback,label:"System",family:ze.fallback}));for(let d of Bl){let p=a.filter(u=>u.group===d);if(!p.length)continue;let m=l.createElement("div");m.className="hcms-font-group",m.textContent=Wo[d],s.appendChild(m);for(let u of p)s.appendChild(jt(l,{value:Ye(u),label:u.family,family:Ye(u)}))}}wr(e,xr(l).has(i)),Vl(e,o)}function Vl(e,t){let r=t==null?"":String(t).trim(),i=e.querySelector(".hcms-font-name");i&&(i.textContent=Ul(r),r?i.setAttribute("style",`font-family: ${r}`):i.removeAttribute("style")),e.querySelectorAll(".hcms-font-option").forEach(l=>{l.setAttribute("aria-selected",String(l.getAttribute("data-hcms-font")===r))})}function wr(e,t){let r=e.querySelector(".hcms-font-panel");r&&(r.hidden=!t),e.querySelector(".hcms-font-current")?.setAttribute("aria-expanded",String(t)),t&&Zo(e.ownerDocument)}function Zo(e){let t=e?.head;if(!t||t.querySelector(Go))return;let r=Hl(ee.map(Ye)),i=[...new Set(ee.map(n=>n.family).join(""))].join(""),l=e.createElement("link");l.rel="stylesheet",l.href=`${r}&text=${encodeURIComponent(i)}`,l.setAttribute("data-hcms-font-preview","");for(let n of["editor-ui","no-save","save-remove","snapshot-remove"])l.setAttribute(n,"");t.appendChild(l)}function Wl(e){return e?.closest?e.closest("[data-hcms-font-path]"):null}function Qo(e){let t=Wl(e);if(!t)return;let r=t.getAttribute("data-hcms-font-path"),i=xr(t.ownerDocument),l=!i.has(r);l?i.add(r):i.delete(r),wr(t,l)}function Yo(e){let t=Wl(e);if(!t)return null;let r=t.querySelector('input[type="hidden"][data-hcms-field]');if(!r)return null;xr(t.ownerDocument).delete(t.getAttribute("data-hcms-font-path")),wr(t,!1);let i=e.getAttribute("data-hcms-font")||"";return Vl(t,i),r.value===i?null:(r.value=i,r)}var G=e=>`<svg viewBox="0 0 32 20" width="32" height="20" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" aria-hidden="true">${e}</svg>`,z=(e,t,r,i,l="")=>`<rect x="${e}" y="${t}" width="${r}" height="${i}" rx="1"${l}/>`,B=(e,t,r)=>`<path d="M${e} ${t}H${r}"/>`,Mt=' fill="currentColor" fill-opacity="0.3"',es=Object.freeze({"cols-1":{label:"One column",svg:G(z(2,2,28,16))},"cols-2":{label:"Two columns",svg:G(z(2,2,13,16)+z(17,2,13,16))},"cols-3":{label:"Three columns",svg:G(z(2,2,8,16)+z(12,2,8,16)+z(22,2,8,16))},"cols-4":{label:"Four columns",svg:G(z(2,2,5.5,16)+z(9.5,2,5.5,16)+z(17,2,5.5,16)+z(24.5,2,5.5,16))},"media-left":{label:"Image left",svg:G(z(2,3,12,14,Mt)+B(18,6,30)+B(18,10,30)+B(18,14,26))},"media-right":{label:"Image right",svg:G(z(18,3,12,14,Mt)+B(2,6,14)+B(2,10,14)+B(2,14,10))},"media-top":{label:"Image on top",svg:G(z(2,2,28,8,Mt)+B(2,14,30)+B(2,17,22))},list:{label:"List",svg:G(B(3,5,4)+B(8,5,29)+B(3,10,4)+B(8,10,29)+B(3,15,4)+B(8,15,29))},cards:{label:"Cards",svg:G(z(2,2,13,7)+z(17,2,13,7)+z(2,11,13,7)+z(17,11,13,7))},centered:{label:"Centered",svg:G(B(6,5,26)+B(9,10,23)+B(11,15,21))},"card-flat":{label:"Flat",svg:G(z(5,3,22,14))},"card-raised":{label:"Raised",svg:G(z(7,5,22,14,' stroke-opacity="0.35"')+z(4,2,22,14,' fill="var(--mirk-bg, transparent)"'))}});function ts(e){return(e||[]).map(t=>{let r=t.indexOf(":"),i=r===-1?t:t.slice(0,r),l=r===-1?t:t.slice(r+1),n=es[l]||null;return{value:i,icon:n,label:n?n.label:Ie(i)}})}function rs(e,{tokens:t,value:r,name:i,doc:l}){let n=ts(t),a=r==null?"":String(r);a!==""&&!n.some(s=>s.value===a)&&n.unshift({value:a,icon:null,label:Ie(a)});let o=e.querySelector(".hcms-variant-option");if(!o||!o.parentNode)return n.length;for(let s of n){let h=o.cloneNode(!0),d=h.querySelector('input[type="radio"]');d&&(d.value=s.value,d.name=i);let p=h.querySelector(".hcms-variant-icon");p&&(s.icon?p.innerHTML=s.icon.svg:p.remove());let m=h.querySelector(".hcms-variant-label");m&&(m.textContent=s.label),h.setAttribute("title",s.label),o.parentNode.insertBefore(h,o)}return o.remove(),n.length}function Gl(e){if(!e)return"";let t=String(e).split(/[?#]/)[0],r=t.split("/").pop()||t;try{return decodeURIComponent(r)}catch{return r}}function Ar({pageRules:e,formRules:t,data:r,doc:i}){let l=i.createDocumentFragment(),n=is(e,r,i);n&&l.appendChild(n);let a=xt(e,[],r,i,e);return a&&l.appendChild(a),l}function is(e,t,r){let i=[];if(s(e,t,[]),!i.length)return null;let l=Q(r,"@object");if(!l)return null;let n=se(l,r);n.setAttribute("data-hcms-page-style","");let a=n.querySelector("[data-hcms-label]");a&&(a.textContent="Page style");let o=n.querySelector(".hcms-object-fields")||n;for(let[h,d,p]of i){let m=xt(h,d,p,r,e);m&&o.appendChild(m)}return n;function s(h,d,p){if(typeof h=="string"){p.length&&pr(h,r,p,e)==="page"&&i.push([h,p,d]);return}if(!(!h||typeof h!="object"||Array.isArray(h))){if(p.length){let m=Ae(p,"@object",r);if(m&&vt(m))return}for(let[m,u]of Object.entries(h))s(u,d==null?null:d[m],[...p,m])}}}function Jl(e,t,r,i){let l=[],n=[];for(let[a,o]of Object.entries(e)){let s=pr(o,r,[...t,a],i);s!=="page"&&(s?l:n).push([a,o])}return[...l,...n]}function ls({shape:e,itemShape:t,pathArr:r,data:i,doc:l,itemKey:n,pageRules:a}){if(e==="object-array-item")return Zl(t,r,i,l,a);if(e==="scalar-array-item")return Ql(r,i,l,n||null);throw new Error(`hypercms: buildItem called with unknown shape "${e}"`)}function xt(e,t,r,i,l){let n=Pe(e);return n==="scalar"?ns(e,t,r,i,l):n==="object"?ss(e,t,r,i,l):n==="object-array"?cs(e,t,r,i,l):n==="scalar-array"?hs(e,t,r,i):null}function ns(e,t,r,i,l){let n=Ue(e,i,t,l),a=Ae(t,n,i);if(!a)throw new Error(`hypercms: missing template for scalar at "${t.join(".")}"`);let o=vo(e,i,t,l);o==="@number"&&n==="@scalar"&&console.info(`[hypercms] field "${t.join(".")}" declares component "@number" but its value isn't a plain number; rendering a text input so the value is preserved`),(o==="@checkbox"||o==="@toggle")&&n==="@scalar"&&console.info(`[hypercms] field "${t.join(".")}" declares component "${o}" but its value isn't true/false; rendering a text input so the value is preserved`),Xl(a,o===n?o:null,t);let s=se(a,i);_e(s,t);let h=a.getAttribute?.("data-hcms-tpl");if((n==="@select"||n==="@radio")&&h===n&&as(s,e,t,r,i,n,l),n==="@variant"&&h==="@variant"){let d=Qe(e,i,t,l);d||Kl(s,"data-hcms-options required (value:icon tokens)"),rs(s,{tokens:d,value:r,name:_r(t.join(".")),doc:i})===0&&s.querySelector(".hcms-variant-option")?.remove()}if(n==="@image"&&h==="@image"){let d=Eo(e,i,t,l);d!=null&&!s.hasAttribute("data-hcms-crop")&&s.setAttribute("data-hcms-crop",d)}return ds(s,te(t)),wt(s,te(t)),At(s,te(t)),ln(s,r),n==="@color"&&h==="@color"&&Fo(s,{value:r,tokens:Qe(e,i,t,l),path:t.join("."),doc:i}),n==="@font"&&h==="@font"&&Ko(s,{value:r,tokens:Qe(e,i,t,l),path:t.join("."),doc:i}),n==="@file"&&os(s),s}function Xl(e,t,r){if(!t)return;let i=e.getAttribute?.("data-hcms-tpl");i&&i!==t&&console.info(`[hypercms] field "${r.join(".")}" declares component "${t}" but custom template "${i}" wins`)}function as(e,t,r,i,l,n,a){let o=Qe(t,l,r,a),s=o?[...o]:[],h=i==null?"":String(i);if(h!==""&&!s.includes(h)&&s.unshift(h),!o&&(Kl(e,"data-hcms-options required (space-separated values)"),s.length===0)){e.querySelector(".mirk-radio")?.remove();return}if(n==="@select"){let m=e.querySelector("select[data-hcms-field]");if(!m)return;for(let u of s){let c=l.createElement("option");c.value=u,c.textContent=Ie(u),m.appendChild(c)}return}let d=e.querySelector(".mirk-radio");if(!d||!d.parentNode)return;let p=_r(r.join("."));for(let m of s){let u=d.cloneNode(!0),c=u.querySelector('input[type="radio"]');c&&(c.value=m,c.name=p);let f=u.querySelector(".mirk-radio__label");f&&(f.textContent=Ie(m)),d.parentNode.insertBefore(u,d)}d.remove()}function _r(e){return"hcms-"+String(e).replace(/[^A-Za-z0-9_-]/g,"-")}function Kl(e,t){let r=e.querySelector?e.querySelector(".hcms-error"):null;r&&(r.textContent=t,r.hidden=!1)}function os(e){let t=e.querySelector?e.querySelector("a.mirk-file__name[data-hcms-field]"):null;t&&(t.textContent=Gl(t.getAttribute("href")))}function ss(e,t,r,i,l){let n=Ae(t,"@object",i);if(!n)throw new Error(`hypercms: missing template for object at "${t.join(".")}"`);let a=se(n,i);if(_e(a,t),wt(a,te(t)),At(a,te(t)),vt(n))return an(a,e,t),nn(a,e,r),a;let o=_t(a,".hcms-object-fields",n,t);for(let[s,h]of Jl(e,t,i,l)){let d=r==null?null:r[s],p=xt(h,[...t,s],d,i,l);p&&o.appendChild(p)}return t.length&&!o.children.length&&Object.keys(e).length?null:a}function cs(e,t,r,i,l){let n=Ae(t,"@object-array",i);if(!n)throw new Error(`hypercms: missing template for object-array at "${t.join(".")}"`);let a=se(n,i);_e(a,t),wt(a,te(t)),At(a,te(t)),en(a,n),rn(a,n,t);let o=_t(a,".hcms-array-items",n,t),[,s]=e;return(Array.isArray(r)?r:[]).forEach((h,d)=>{let p=Zl(s,[...t,d],h,i,l);p&&o.appendChild(p)}),tn(a),a}function Zl(e,t,r,i,l){let n=Yl(t,"object-array-item",i);if(!n)throw new Error(`hypercms: missing item template for "${t.join(".")}"`);let a=se(n,i);if(a.setAttribute("data-hcms-card",""),a.classList.contains("hcms-card")||a.classList.add("hcms-card"),_e(a,t),vt(n))return e&&typeof e=="object"&&!Array.isArray(e)&&(an(a,e,t),nn(a,e,r)),a;let o=_t(a,".hcms-card-fields",n,t);if(e&&typeof e=="object"&&!Array.isArray(e))for(let[s,h]of Jl(e,t,i,l)){let d=r==null?null:r[s],p=xt(h,[...t,s],d,i,l);p&&o.appendChild(p)}return a}function hs(e,t,r,i){let l=fr(e,i),n=Tl(e,t,i),a=l?l.array:"@scalar-array",o=Ae(t,a,i);if(!o)throw new Error(`hypercms: missing template for scalar-array at "${t.join(".")}"`);Xl(o,l?l.array:null,t);let s=se(o,i);_e(s,t),wt(s,te(t)),At(s,te(t)),en(s,o),rn(s,o,t),n&&s.setAttribute("data-hcms-item-tpl",n.item);let h=_t(s,".hcms-array-items",o,t);return(Array.isArray(r)?r:[]).forEach((d,p)=>{let m=Ql([...t,p],d,i,n?n.item:null);m&&h.appendChild(m)}),tn(s),s}function Ql(e,t,r,i){let l=Yl(e,"scalar-array-item",r,i);if(!l)throw new Error(`hypercms: missing item template for "${e.join(".")}"`);let n=se(l,r);return n.setAttribute("data-hcms-array-item",""),n.classList.contains("hcms-array-item")||n.classList.add("hcms-array-item"),_e(n,e),ln(n,t),n}function Yl(e,t,r,i){let l=e.map(n=>typeof n=="number"?"*":n).join(".");return Q(r,l)||i&&Q(r,i)||Q(r,"@"+t)}function se(e,t){let r=e.content||e,i=t.createElement("div");return i.appendChild(r.cloneNode(!0)),i.firstElementChild||i}function _e(e,t){e.setAttribute("data-hcms-path",t.join(".")),e.setAttribute("data-hcms-path-shape",gr(t))}function ds(e,t){let r=t==null?"":String(t);if(e.matches&&e.matches("[data-hcms-field]")){e.getAttribute("data-hcms-field")||e.setAttribute("data-hcms-field",r);return}(e.querySelectorAll?e.querySelectorAll("[data-hcms-field]"):[]).forEach(i=>{i.getAttribute("data-hcms-field")||i.setAttribute("data-hcms-field",r)})}function wt(e,t){t==null||t===""||!e.setAttribute||e.hasAttribute?.("data-hcms-field")||e.setAttribute("data-hcms-field",String(t))}function At(e,t){t==null||t===""||(e.querySelectorAll?e.querySelectorAll("[data-hcms-label]"):[]).forEach(r=>{(r.textContent||"").trim()===""&&(r.textContent=Ie(String(t)))})}function en(e,t){["data-hcms-no-add","data-hcms-no-remove","data-hcms-no-reorder"].forEach(r=>{t.hasAttribute(r)&&e.setAttribute(r,"")}),["data-hcms-min-items","data-hcms-max-items"].forEach(r=>{t.hasAttribute(r)&&e.setAttribute(r,t.getAttribute(r))})}function tn(e){let t=e.querySelector?e.querySelector(".hcms-array-items"):null;if(!t)return;let r=Array.from(t.querySelectorAll(":scope > [data-hcms-card], :scope > [data-hcms-array-item]")),i=r.length,l=_i(e,"data-hcms-max-items"),n=_i(e,"data-hcms-min-items"),a=e.hasAttribute("data-hcms-no-add"),o=e.hasAttribute("data-hcms-no-remove"),s=e.hasAttribute("data-hcms-no-reorder"),h=e.querySelector('[data-hcms-action="add"]');h&&(h.hidden=a||l!=null&&i>=l),r.forEach((d,p)=>{let m=d.querySelector('[data-hcms-action="remove"]');m&&(m.hidden=o||n!=null&&i<=n);let u=d.querySelector('[data-hcms-action="move-up"]');u&&(u.hidden=s||p===0);let c=d.querySelector('[data-hcms-action="move-down"]');c&&(c.hidden=s||p===i-1)})}function _i(e,t){if(!e||!e.hasAttribute(t))return null;let r=parseInt(e.getAttribute(t),10);return Number.isFinite(r)?r:null}function rn(e,t,r){if(e.hasAttribute("data-hcms-no-reorder")||t.hasAttribute("data-hcms-no-reorder"))return;let i=e.querySelector(".hcms-array-items");if(!i)return;let l="hcms-"+r.join(".");i.setAttribute("sortable",l),i.setAttribute("onsorted","hypercmsCommit && hypercmsCommit()")}function te(e){return e.length?e[e.length-1]:null}function ln(e,t){let r=us(e);if(r.length!==0)for(let i of r)on(i,t)}function us(e){if(!e)return[];let t=[];return e.matches?.("[data-hcms-field]")&&ms(e)&&t.push(e),(e.querySelectorAll?e.querySelectorAll("input[data-hcms-field], textarea[data-hcms-field], select[data-hcms-field], img[data-hcms-field], a[data-hcms-field], [contenteditable][data-hcms-field]"):[]).forEach(r=>t.push(r)),t}function ms(e){let t=(e.tagName||"").toUpperCase();return!!(t==="INPUT"||t==="TEXTAREA"||t==="SELECT"||t==="IMG"||t==="A"||e.hasAttribute?.("contenteditable"))}function nn(e,t,r){(e.querySelectorAll?e.querySelectorAll("[data-hcms-field]"):[]).forEach(i=>{let l=i.getAttribute("data-hcms-field");if(!l)return;if(!t||typeof t!="object"||!(l in t)){console.warn(`[hypercms] inline template field "${l}" is not in the rule shape; ignoring`);return}let n=r==null?null:r[l];on(i,n)})}function an(e,t,r){e.querySelectorAll&&e.querySelectorAll("[data-hcms-field]").forEach(i=>{let l=i.getAttribute("data-hcms-field");if(!l||t&&typeof t=="object"&&!(l in t))return;let n=[...r,l].join(".");i.setAttribute("data-hcms-path",n),i.setAttribute("data-hcms-path-shape",gr([...r,l]))})}function _t(e,t,r,i){if(!e.querySelector)return e;let l=e.querySelector(t);if(l)return l;let n=r?.getAttribute?.("data-hcms-tpl")||i.join(".");throw new Error(`hypercms: template "${n}" is in slotted mode but has no ${t} element`)}function on(e,t){let r=br(e),i=(e.tagName||"").toUpperCase(),l=(e.getAttribute("type")||"").toLowerCase();if(i==="INPUT"&&l==="radio"){e.checked=e.value!=null&&String(e.value)===String(t??"");return}if(r==="checked"){e.checked=t===!0||t==="true";return}if(r){e[r]=t==null?"":String(t);return}e.textContent=t==null?"":String(t)}var ps={Mutation:(e,t)=>e?.Mutation??t?.Mutation,undo:(e,t)=>e?.undo??t?.undo,onPrepareForSave:(e,t)=>e?.addDocumentTransform??t?.onPrepareForSave,onSnapshot:(e,t)=>e?.onSnapshot??t?.onSnapshot,consent:(e,t)=>e?.confirm??t?.consent,RichClay:(e,t)=>e?.RichClay??t?.RichClay,quickcrop:(e,t)=>e?.quickcrop??t?.quickcrop,upload:(e,t)=>e?.upload??(t?.uploadFileBasic?gs(t.uploadFileBasic):null)},fs={402:"payment-required",413:"too-large",415:"unsupported-type",401:"unauthorized",403:"forbidden",404:"not-found"},Nt=()=>({ok:!1,msg:"Upload cancelled",msgType:"skipped",code:"aborted",uploads:[]});function gs(e){return async function(t,{onProgress:r,signal:i}={}){if(i?.aborted)return Nt();try{let l=await e(t,{onProgress:a=>{r?.({loaded:null,total:null,percent:a})}});if(i?.aborted)return Nt();let n=l&&l.uploads||[];return typeof n[0]?.url!="string"?{ok:!1,msg:"The host accepted the file but did not say where it put it",msgType:"error",code:"bad-response",uploads:[]}:{ok:!0,msg:l.msg||"Uploaded",msgType:l.msgType||"success",code:l.code||null,uploads:n}}catch(l){if(i?.aborted)return Nt();let n={};try{n=JSON.parse(l?.response||"{}")}catch{n={}}let a=n.code||fs[l?.status]||"error";return{ok:!1,msg:l&&l.message||"Upload failed",msgType:"error",code:a,uploads:[]}}}}function V(e,t){let r=ps[e];if(!r)throw new Error(`hypercms: unknown platform capability "${e}"`);let i=t||(typeof window<"u"?window:null);return i&&r(i.clay,i.hyperclay)||null}var bs=["clay:mutation-ready","hyperclay:mutation-ready"],ks=["clay:sync-applied","hyperclay:livesync-applied"],vs=["clay:ready","hyperclay:ready"];function Er(e,t,r){let i=null,l=n=>{i!==null&&i!==n.type||(i=n.type,queueMicrotask(()=>{i=null}),r(n))};for(let n of t)e.addEventListener(n,l);return()=>{for(let n of t)e.removeEventListener(n,l)}}var re="data-hcms-bound",ye="data-hcms-bound-id",Sr=new Map;function ys(e,t){Sr.set(e,t)}function xs(e){Sr.delete(e)}function ws(e){let t=e.getAttribute(ye);if(!t)return null;let r=Sr.get(t);return!r||!r.restorable()?null:r.originalHTML}var xe="data-hcms-owns-richclay";function Et(e){return e&&e.richclay&&e.richclay.RichClay||V("RichClay",e)||(e&&typeof e.RichClay=="function"?e.RichClay:null)}function As(e,t,r){!e||typeof e.setAttribute!="function"||(e.setAttribute(re,t?"rich":"plain"),r&&e.setAttribute(xe,"true"))}function _s(e,t){if(!e||typeof e.querySelectorAll!="function")return;let r=e.querySelectorAll(`[${re}]`);if(!r.length)return;let i=Et(t);if(i&&typeof i.stripFromClone=="function")try{i.stripFromClone(e)}catch(l){console.warn("[hypercms] richclay strip failed; editor state may reach the save",l)}for(let l of r)l.getAttribute(xe)==="true"&&l.removeAttribute("data-richclay"),l.removeAttribute(xe),l.removeAttribute(re)}function Es(e,t){if(!e||typeof e.removeAttribute!="function")return;let r=Et(t);if(r&&typeof r.stripElement=="function")try{r.stripElement(e)}catch(i){console.warn("[hypercms] richclay element strip failed; the clone stays editable",i)}else e.removeAttribute("contenteditable"),e.removeAttribute("no-undo");e.getAttribute(xe)==="true"&&e.removeAttribute("data-richclay"),e.removeAttribute(xe),e.removeAttribute(re),e.removeAttribute(ye)}function Ss(e,t){if(!t||e==null)return e;return r(e);function r(i){if(typeof i=="string"){if(i.endsWith("[]")||I(i)!==-1)return i;let l=null;try{l=t.querySelector(i)}catch{return i}return l&&l.children.length>0?i+"@innerHTML":i}if(Array.isArray(i))return i;if(i&&typeof i=="object"){let l=Object.create(null);for(let[n,a]of Object.entries(i))l[n]=r(a);return l}return i}}function Cs(e,t){if(!t||e==null)return e;return r(e,[t]);function r(i,l){if(typeof i=="string")return i.endsWith("[]")||I(i)!==-1?i:l.some(n=>Ts(n,i))?i+"@innerHTML":i;if(Array.isArray(i)){let[n,a]=i;if(typeof n!="string"||!n)return i;let o=Rs(t,n);return o.length?[n,r(a,o)]:i}if(i&&typeof i=="object"){let n=Object.create(null);for(let[a,o]of Object.entries(i))n[a]=r(o,l);return n}return i}}function Ts(e,t){let r=null;try{r=e.querySelector(t)}catch{return!1}return r?r.hasAttribute(re)?r.getAttribute(re)==="rich":r.children.length>0:!1}function Rs(e,t){try{return[...e.querySelectorAll(t)]}catch{return[]}}function ot(e){if(!e||e.tagName!=="TEXTAREA")return;let t=e.ownerDocument.defaultView||(typeof window<"u"?window:null);t&&t.CSS&&t.CSS.supports&&t.CSS.supports("field-sizing: content")||(e.style.height="auto",e.style.height=e.scrollHeight+"px")}function St(e,t,r=!0){if(!e||!e.querySelectorAll||(e.querySelectorAll("textarea[data-hcms-field]").forEach(ot),r===!1))return;let i=t&&t.defaultView||(typeof window<"u"?window:null),l=Et(i);l&&e.querySelectorAll("[contenteditable][data-hcms-field]").forEach(n=>{if(n.__hcmsRichclay)return;let a;try{a=new l(n,{inline:!0,hyperclay:!1,toolbar:["bold","italic","link","undo","redo"]})}catch(s){console.warn("[hypercms] richclay activation failed; field stays plain contenteditable",s);return}n.__hcmsRichclay=a;let o=a&&a.squire;o&&typeof o.addEventListener=="function"&&o.addEventListener("input",()=>{let s=i&&i.Event||Event;n.dispatchEvent(new s("input",{bubbles:!0}))})})}var sn="link[data-hcms-fonts]",qs="link[data-hcms-fonts-preconnect]";function Os(e,t,r){let i=[],l=!1;return n(e,t,[]),{stacks:i,found:l};function n(a,o,s){if(typeof a=="string"){if(a.endsWith("[]")||Ue(a,r,s,e)!=="@font")return;l=!0,o!=null&&o!==""&&i.push(String(o));return}if(Array.isArray(a)){let h=Array.isArray(o)?o:[];h.forEach(d=>n(a[1],d,[...s,"*"])),h.length||n(a[1],void 0,[...s,"*"]);return}if(a&&typeof a=="object")for(let[h,d]of Object.entries(a))n(d,o?.[h],[...s,h])}}function $s(e,t){let r=e?.head;if(!r)return;let i=r.querySelector(sn),l=r.querySelector(qs);if(!t){i?.remove(),l?.remove();return}l||(l=e.createElement("link"),l.setAttribute("rel","preconnect"),l.setAttribute("href","https://fonts.gstatic.com"),l.setAttribute("crossorigin",""),l.setAttribute("data-hcms-fonts-preconnect",""),l.setAttribute("no-undo",""),r.appendChild(l)),i||(i=e.createElement("link"),i.setAttribute("rel","stylesheet"),i.setAttribute("data-hcms-fonts",""),i.setAttribute("no-undo",""),l.after(i)),i.hasAttribute("no-undo")||i.setAttribute("no-undo",""),l.hasAttribute("no-undo")||l.setAttribute("no-undo",""),i.getAttribute("href")!==t&&i.setAttribute("href",t)}function Cr(e,t){if(!e?.doc||e.pageRules==null)return;let{stacks:r,found:i}=Os(e.pageRules,t,e.doc);if(!i)return;let l=e.doc.head?.querySelector(sn)?.getAttribute("href")||null,n=Hl(r,l);n&&l&&Vo(l,n)||$s(e.doc,n)}var zt=new WeakSet;function Ve(e,t){let r=V("undo");if(!r)return t();r.pause();try{let i=t();return i&&i.ok?r.commitCaptured(e):r.discardCaptured(),i}finally{r.resume()}}function Ee(e){let t=V("undo");if(!t)return e();t.pause();try{return e()}finally{t.discardCaptured(),t.resume()}}function cn(e){let{formRoot:t}=e;if(!t||zt.has(t))return;zt.add(t);let r=a=>{let o=a.target;!o||!o.closest||o.closest("[data-hcms-form-root]")&&(o.matches("[data-hcms-color-input]")||o.matches("input, textarea, select, [contenteditable][data-hcms-field]")&&(o.tagName==="TEXTAREA"&&ot(o),!o.matches('input[type="file"]')&&(!o.closest("[data-hcms-field]")&&!o.hasAttribute?.("data-hcms-field")||Le(o,e))))},i=a=>{let o=a.target;if(!(!o||!o.closest)&&o.closest("[data-hcms-form-root]")){if(o.matches("[data-hcms-color-input]")){let s=Ho(o);s&&Le(s,e);return}if(o.matches('input[type="file"][data-hcms-upload]')){Bs(o,e);return}o.matches('input[type="checkbox"], input[type="radio"], select')&&Le(o,e)}},l=a=>{let o=a.target;if(!o||!o.closest)return;let s=o.closest("[data-hcms-action]");if(!s)return;let h=s.getAttribute("data-hcms-action");if(h==="add"||h==="remove"||h==="move-up"||h==="move-down"||h==="clear-upload"||h==="color-pick"||h==="color-clear"||h==="color-panel"||h==="font-pick"||h==="font-panel"){if(!s.closest("[data-hcms-form-root]"))return}else if(h==="close"&&!s.closest("[data-hcms-shell]"))return;if(h==="add"){let d=s.closest("[data-hcms-path]");if(!d)return;let p=d.getAttribute("data-hcms-path");Tr(p,e)}else if(h==="remove"){let d=s.closest("[data-hcms-card], [data-hcms-array-item]");if(!d)return;mn(d,e)}else if(h==="move-up"||h==="move-down"){let d=s.closest("[data-hcms-card], [data-hcms-array-item]");if(!d)return;un(d,h==="move-up"?-1:1,e)}else if(h==="clear-upload")Us(s,e);else if(h==="color-pick"||h==="color-clear"){let d=h==="color-pick"?Bo(s):Io(s);d&&Le(d,e)}else if(h==="color-panel")Po(s);else if(h==="font-pick"){let d=Yo(s);d&&Le(d,e)}else h==="font-panel"?Qo(s):h==="close"&&e.onCloseRequested?.()},n=t.ownerDocument;n.addEventListener("input",r,!0),n.addEventListener("change",i,!0),n.addEventListener("click",l,!0),e.detachEvents=()=>{n.removeEventListener("input",r,!0),n.removeEventListener("change",i,!0),n.removeEventListener("click",l,!0),zt.delete(t)}}var Ls=new Set(["value","checked"]);function js(e,t){if(!t)return null;let r=ve(t);if(r.some(s=>typeof s=="number"||s==="*"))return null;let i=Be(e.pageRules,r);if(typeof i!="string")return null;let l=X.ruleAttrIndex(i);if(l===-1)return null;let n=i.slice(l+1);if(!Ls.has(n))return null;let a=i.slice(0,l),o=a?e.pageRoot.querySelector(a):ae(e.pageRoot);return o?{el:o,prop:n,oldValue:o[n]}:null}function Le(e,t){let r=(e.closest("[data-hcms-field]")||e).closest("[data-hcms-path]")?.getAttribute("data-hcms-path")||"",i=js(t,r);if(Y(J(t),{path:r,structural:!1},t),i){let l=V("undo");l&&typeof l.recordValue=="function"&&l.recordValue(i.el,{prop:i.prop,oldValue:i.oldValue,newValue:i.el[i.prop]})}}var Ms={type:"image/webp",quality:.85,maxWidth:2048,maxHeight:2048};async function Ns(e,t){let r=t&&t.getAttribute?t.getAttribute("data-hcms-crop"):null;if(r==null)return{file:e};let i=V("quickcrop");if(typeof i!="function")return{file:e};try{let l=typeof window<"u"&&(window.clay?.modal??window.themodal)||"auto",n=await i(e,{aspect:zs(r),modal:l,...Ms});return n===null?null:{file:Fs(n.blob,e.name),dataURL:n.dataURL}}catch(l){return we(t,l&&l.message||"Crop failed"),null}}function zs(e){let t=String(e??"").trim().toLowerCase();if(t===""||t==="free")return null;let r=t.match(/^(\d+(?:\.\d+)?)\s*[:/]\s*(\d+(?:\.\d+)?)$/);if(!r)return null;let i=parseFloat(r[1]),l=parseFloat(r[2]);return!i||!l?null:i/l}function Fs(e,t){let r=e.type==="image/webp"?".webp":e.type==="image/jpeg"?".jpg":".png",i=String(t||"image").replace(/\.[^.]+$/,"");try{return new File([e],i+r,{type:e.type})}catch{return e}}var Ds=new Set(["unsupported","payment-required"]);async function Bs(e,t){let r=e.files&&e.files[0];if(!r)return;let i=e.closest("[data-hcms-path]");if(!i)return;let l=i.getAttribute("data-hcms-path")||"";we(i,null);let n=await Ns(r,i);if(!n||t.closed){ie(e);return}let a=n.file,o=n.dataURL||null,s=V("upload");if(typeof s!="function")return Ft(e,i,t,l,await Ei(a,o,i),a);dn(i,o),Si(i,0);let h=Ps(t),d;try{d=await s(a,{signal:h?.signal,onProgress:({percent:m})=>Si(i,m)})}finally{Hs(t,h),Ws(i)}if(t.closed){ie(e);return}if(d.code==="aborted"){ie(e);return}if(d.ok)return Ft(e,i,t,l,d.uploads[0].url,a);if(!Ds.has(d.code)){we(i,d.msg||"Upload failed"),t.dispatch?.("hcms:error",{error:new Error(d.msg||"Upload failed"),code:d.code,path:l}),ie(e);return}let p=d.code==="payment-required"?"This file is stored in the page. Add a paid plan to upload files.":null;return Ft(e,i,t,l,await Ei(a,o,i),a,p)}function Ft(e,t,r,i,l,n,a=null){if(r.closed){ie(e);return}if(dn(t,null),we(t,null),!l){ie(e);return}hn(t,l,n.name),Y(J(r),{path:i,structural:!1},r),a&&we(t,a,"info"),ie(e)}async function Ei(e,t,r){return t||await Is(e,r)}function Is(e,t){let r=t?.ownerDocument?.defaultView?.FileReader||globalThis.FileReader;return r?new Promise(i=>{let l=new r;l.onload=()=>i(typeof l.result=="string"?l.result:""),l.onerror=()=>i("");try{l.readAsDataURL(e)}catch{i("")}}):Promise.resolve("")}function Ps(e){if(typeof AbortController!="function")return null;let t=new AbortController;return(e.uploads||(e.uploads=new Set)).add(t),t}function Hs(e,t){t&&e.uploads?.delete(t)}function Us(e,t){let r=e.closest("[data-hcms-path]");if(!r)return;let i=r.getAttribute("data-hcms-path")||"";hn(r,"","");let l=r.querySelector('input[type="file"][data-hcms-upload]');l&&ie(l),we(r,null),Y(J(t),{path:i,structural:!1},t)}function Vs(e){return e.querySelector?e.querySelector("img[data-hcms-field], a[data-hcms-field]"):null}function hn(e,t,r){let i=Vs(e);if(!i)return;let l=(i.tagName||"").toUpperCase();l==="IMG"?i.src=t||"":l==="A"&&(i.href=t||"",i.textContent=t?r||Gl(t):"")}function ie(e){try{e.value=""}catch{}}function dn(e,t){let r=e.querySelector?e.querySelector(".mirk-image__frame"):null;r&&(t?r.style.backgroundImage=`url("${t.replace(/"/g,"%22")}")`:r.style.removeProperty("background-image"))}function Si(e,t){let r=Math.max(0,Math.min(100,Number(t)||0));e.setAttribute("data-hcms-uploading",""),e.style?.setProperty?.("--hcms-upload-progress",`${r}%`)}function Ws(e){e.removeAttribute("data-hcms-uploading"),e.style?.removeProperty?.("--hcms-upload-progress")}function we(e,t,r="error"){let i=e.querySelector?e.querySelector(":scope > .hcms-error"):null;i&&(i.classList.toggle("hcms-error--info",!!t&&r==="info"),t?(i.textContent=t,i.hidden=!1):(i.textContent="",i.hidden=!0))}function Tr(e,t){let{formRoot:r,pageRules:i}=t,l=r.querySelector(`[data-hcms-path="${Se(e)}"]`);if(!l)throw new Error(`hypercms: no element at path "${e}"`);let n=l.querySelector(".hcms-array-items");if(!n)throw new Error(`hypercms: array container missing .hcms-array-items at "${e}"`);let a=ve(e),o=Zs(i,a),s=Array.isArray(o),h=typeof o=="string"&&o.endsWith("[]");if(!s&&!h)throw new Error(`hypercms: path "${e}" is not an array`);let d=st(l,"data-hcms-max-items"),p=n.querySelectorAll(":scope > [data-hcms-card], :scope > [data-hcms-array-item]");if(l.hasAttribute("data-hcms-no-add")||d!=null&&p.length>=d)return;let m=p.length,u=s?o[1]:o.replace(/\[\]$/,""),c=ur(s?u:"string"),f=ls({shape:s?"object-array-item":"scalar-array-item",itemShape:u,pathArr:[...a,m],data:c,doc:t.doc,itemKey:l.getAttribute("data-hcms-item-tpl")||null,pageRules:i});n.appendChild(f),St(f,t.doc,t.view?.enhanceFormRichText!==!1),ct(l);let b=Ve(`Add ${e}`,()=>Y(J(t),{path:e,structural:!0},t));return b&&b.ok===!1&&(f.remove(),ct(l)),b}function un(e,t,r){let i=e.closest('[data-hcms-shape="object-array"], [data-hcms-shape="scalar-array"]');if(!i||i.hasAttribute("data-hcms-no-reorder"))return;let l=i.querySelector(".hcms-array-items");if(!l)return;let n=Array.from(l.querySelectorAll(":scope > [data-hcms-card], :scope > [data-hcms-array-item]")),a=n.indexOf(e);if(a<0)return;let o=a+t;if(o<0||o>=n.length)return;let s=e.querySelector(`[data-hcms-action="${t<0?"move-up":"move-down"}"]`);return t<0?l.insertBefore(e,n[o]):l.insertBefore(e,n[o].nextSibling),Rr(l),ct(i),s&&typeof s.focus=="function"&&e.querySelector(`[data-hcms-action="${t<0?"move-up":"move-down"}"]`)?.focus?.(),Ve(`Reorder ${i.getAttribute("data-hcms-path")||""}`,()=>Y(J(r),{path:i.getAttribute("data-hcms-path")||"",structural:!0},r))}var Dt="Delete this item?";function Gs(e,t){let r=e&&e.getAttribute("data-hcms-confirm-remove");if(r!=null)return/^(off|false|no|0)$/i.test(r.trim())?null:r||Dt;let i=t&&t.confirmRemove;return i===!1?null:typeof i=="string"?i||Dt:i===!0||e&&e.getAttribute("data-hcms-shape")==="object-array"?Dt:null}function mn(e,t){let r=e.closest('[data-hcms-shape="object-array"], [data-hcms-shape="scalar-array"]'),i=Gs(r,t);if(i==null)return je(e,t);let l=V("consent")||typeof window<"u"&&window.consent;typeof l=="function"?Promise.resolve(l(i)).then(()=>{t.closed||je(e,t)},()=>{}):typeof window<"u"&&typeof window.confirm=="function"?window.confirm(i)&&je(e,t):je(e,t)}function je(e,t){let r=e.getAttribute("data-hcms-path")||"",i=e.parentElement,l=e.closest('[data-hcms-shape="object-array"], [data-hcms-shape="scalar-array"]');if(!l?.hasAttribute("data-hcms-no-remove")){if(l){let n=st(l,"data-hcms-min-items"),a=l.querySelector(".hcms-array-items"),o=a?a.querySelectorAll(":scope > [data-hcms-card], :scope > [data-hcms-array-item]").length:0;if(n!=null&&o<=n)return}return e.remove(),i&&Rr(i),l&&ct(l),Ve(`Remove ${r}`,()=>Y(J(t),{path:r,structural:!0},t))}}function Y(e,t,r){if(r.closed)return{ok:!1,skipped:!0,closed:!0};let i=ht(e);if(!t.structural&&i===r.lastFingerprint)return{ok:!0,skipped:!0};let l=Object.hasOwn(r,"writeRules")?r.writeRules:r.pageRules,n=ao(r.pageRoot,l,e,{shellRoot:r.shellRoot,structural:!!t.structural,structuralPath:t.path||null,formRoot:r.formRoot});return n.ok?(r.lastFingerprint=i,r.lastData=e,Cr(r,e),Ci(r,null),r.dispatch?.("hcms:change",{data:e,path:t.path,structural:!!t.structural}),r.onChange?.(e,t)):(Ci(r,Ks(n.error,t.path)),r.dispatch?.("hcms:error",{error:n.error,attemptedData:e}),r.onError?.(n.error)),n}function pn(e,t){let r=Se(t),i=`[data-hcms-path="${r}"] input[data-hcms-field], [data-hcms-path="${r}"] textarea[data-hcms-field], [data-hcms-path="${r}"] select[data-hcms-field], [data-hcms-path="${r}"] img[data-hcms-field], [data-hcms-path="${r}"] a[data-hcms-field], [data-hcms-path="${r}"] [contenteditable][data-hcms-field], input[data-hcms-path="${r}"][data-hcms-field], textarea[data-hcms-path="${r}"][data-hcms-field], select[data-hcms-path="${r}"][data-hcms-field], img[data-hcms-path="${r}"][data-hcms-field], a[data-hcms-path="${r}"][data-hcms-field], [contenteditable][data-hcms-path="${r}"][data-hcms-field]`;return e.querySelector(i)}function fn(e,t,r,i){let l=(e.tagName||"").toUpperCase(),n=(e.getAttribute("type")||"").toLowerCase();if(l==="INPUT"&&n==="checkbox"){e.checked=t===!0||t==="true";return}if(l==="INPUT"&&n==="radio"){let a=Se(i),o=r.querySelectorAll(`[data-hcms-path="${a}"][data-hcms-field][type="radio"], [data-hcms-path="${a}"] [data-hcms-field][type="radio"]`);o.length?o.forEach(s=>{s.checked=String(s.value)===String(t??"")}):e.checked=String(e.value)===String(t??"");return}if(l==="IMG"){e.src=t==null?"":String(t);return}if(l==="A"){e.href=t==null?"":String(t);return}if(e.hasAttribute&&e.hasAttribute("contenteditable")){e.innerHTML=t==null?"":String(t);return}if("value"in e){e.value=t==null?"":String(t);return}e.textContent=t==null?"":String(t)}function J(e){let t=X.extract(e.formRoot,e.formRules,{exclude:null});return oe(t,e.formRules)}function oe(e,t){if(t==null||e==null)return e;if(typeof t=="string")return t.endsWith("@checked")?e===!0||e==="true":e;if(Array.isArray(t)){if(!Array.isArray(e))return e;let[,r]=t;return e.map(i=>oe(i,r))}if(typeof t=="object"){if(typeof e!="object"||Array.isArray(e))return e;let r={};for(let[i,l]of Object.entries(t))r[i]=oe(e[i],l);return r}return e}function Ci(e,t){e.lastErrors=t&&t.length?t:null,gn(e)}function gn(e){if(Js(e),e.errorEl&&(e.errorEl.textContent="",e.errorEl.hidden=!0),!e.lastErrors)return;let t=[];for(let{message:r,path:i}of e.lastErrors){if(i!=null&&i!==""){let l=Xs(e.formRoot,i);if(l){l.textContent=l.textContent?`${l.textContent}
${r}`:r,l.hidden=!1;continue}}t.push(r)}t.length&&e.errorEl&&(e.errorEl.textContent=t.join(`
`),e.errorEl.hidden=!1)}function Js(e){if(e.formRoot)for(let t of e.formRoot.querySelectorAll(".hcms-error"))t.textContent="",t.hidden=!0}function Xs(e,t){if(!e)return null;let r=t.split(".");for(;r.length>0;){let i=r.join("."),l=typeof CSS<"u"&&CSS.escape?CSS.escape(i):i.replace(/[^a-zA-Z0-9_\-.*]/g,a=>"\\"+a),n=e.querySelector(`[data-hcms-path="${l}"]`);if(n){for(let a of n.children)if(a.classList&&a.classList.contains("hcms-error"))return a}r.pop()}return null}function Ks(e,t){return e?e.name==="EmptyListInsert"?[{message:"Add a seed item in HTML first.",path:t}]:e.name==="ShapeMismatch"&&Array.isArray(e.mismatches)&&e.mismatches.length?e.mismatches.map(r=>({message:`Shape mismatch: expected ${r.expected}, got ${r.got}`,path:r.path})):[{message:e.message||String(e),path:t}]:[{message:"unknown error",path:t}]}function Zs(e,t){let r=e;for(let i of t){if(r==null||typeof r=="string")return;if(Array.isArray(r)){if(typeof i!="number"&&i!=="*")return;r=r[1];continue}if(typeof r=="object"){if(typeof i=="number"||!(i in r))return;r=r[i];continue}return}return r}function st(e,t){if(!e||!e.hasAttribute(t))return null;let r=parseInt(e.getAttribute(t),10);return Number.isFinite(r)?r:null}function ct(e){if(!e)return;let t=e.querySelector(".hcms-array-items");if(!t)return;let r=Array.from(t.querySelectorAll(":scope > [data-hcms-card], :scope > [data-hcms-array-item]")),i=r.length,l=st(e,"data-hcms-max-items"),n=st(e,"data-hcms-min-items"),a=e.hasAttribute("data-hcms-no-add"),o=e.hasAttribute("data-hcms-no-remove"),s=e.hasAttribute("data-hcms-no-reorder"),h=e.querySelector(':scope > .hcms-add, :scope > * > .hcms-add, :scope > [data-hcms-action="add"]');h&&(h.hidden=a||l!=null&&i>=l),r.forEach((d,p)=>{let m=d.querySelector('[data-hcms-action="remove"]');m&&(m.hidden=o||n!=null&&i<=n);let u=d.querySelector('[data-hcms-action="move-up"]');u&&(u.hidden=s||p===0);let c=d.querySelector('[data-hcms-action="move-down"]');c&&(c.hidden=s||p===i-1)})}function bn(e){!e||!e.querySelectorAll||e.querySelectorAll(".hcms-array-items").forEach(t=>Rr(t))}function Rr(e){let t=e.querySelectorAll?Array.from(e.querySelectorAll('input[type="radio"][data-hcms-field]'),i=>[i,i.checked]):[],r=0;for(let i of e.children){if(!i.matches?.("[data-hcms-card], [data-hcms-array-item]"))continue;let l=i.getAttribute("data-hcms-path");if(!l)continue;let n=l.split(".");n[n.length-1]=String(r);let a=n.join(".");a!==l&&Qs(i,l,a),r++}for(let[i,l]of t)i.checked!==l&&(i.checked=l)}function Qs(e,t,r){let i=e.querySelectorAll("[data-hcms-path]");e.setAttribute("data-hcms-path",r);for(let l of i){let n=l.getAttribute("data-hcms-path");n===t?l.setAttribute("data-hcms-path",r):n&&n.startsWith(t+".")&&l.setAttribute("data-hcms-path",r+n.slice(t.length))}Ys(e)}function Ys(e){for(let t of e.querySelectorAll('input[type="radio"][data-hcms-field]')){if(!t.name||!t.name.startsWith("hcms-"))continue;let r=t.closest("[data-hcms-path]");r&&(t.name=_r(r.getAttribute("data-hcms-path")))}}function ht(e){return JSON.stringify(e,(t,r)=>{if(r&&typeof r=="object"&&!Array.isArray(r)){let i=Object.create(null);for(let l of Object.keys(r).sort())i[l]=r[l];return i}return r})}function Se(e){return typeof CSS<"u"&&CSS.escape?CSS.escape(e):String(e).replace(/[^a-zA-Z0-9_\-.*]/g,t=>"\\"+t)}var et="hcms-shell-styles",ec="hcms-bundled-styles-installed",qr="hcms-session-open",tc='a[href], area[href], button:not([disabled]), input:not([disabled]):not([type="hidden"]), select:not([disabled]), textarea:not([disabled]), [tabindex]:not([tabindex="-1"])',ue=new WeakSet,Qt="";function rc(e){Qt=e}function dt(e){e?.body?.classList.add(qr)}function kn(e){e?.body?.classList.remove(qr)}var ic=0;function Ti(e){return String(e).replace(/[&<>"]/g,t=>({"&":"&amp;","<":"&lt;",">":"&gt;",'"':"&quot;"})[t])}function lc({mountTo:e,side:t="right",overlay:r=!1,showSaveButton:i=!1,title:l="Page content",eyebrow:n="Edit",theme:a=null,doc:o}){vn(o);let s=`hcms-shell-title-${++ic}`,h=o.createElement("div");h.setAttribute("data-hcms-shell",""),h.setAttribute("editor-ui",""),h.setAttribute("save-remove",""),h.setAttribute("save-ignore",""),h.setAttribute("tabindex","-1"),h.setAttribute("role","dialog"),h.setAttribute("aria-modal","true"),h.setAttribute("aria-labelledby",s);let d=a==="dark"?" dark":a==="light"?" light":"";h.className="hcms-shell pixel-quiet hcms-panel hcms-side-"+t+(r?" hcms-overlay":"")+d;let p=Ti(l),m=Ti(n);h.innerHTML=`
    <div class="hcms-shell-minibar" aria-hidden="true">
      <span class="hcms-shell-minibar-title">${p}</span>
      <button type="button" class="hcms-shell-close mirk-button mirk-button--small" data-hcms-action="close" aria-label="Close">
        <span class="mirk-button__label">\xD7</span>
      </button>
    </div>
    <div class="hcms-shell-body">
      <header class="hcms-shell-header">
        <div class="hcms-shell-heading">
          <div class="hcms-shell-eyebrow">${m}</div>
          <h2 class="hcms-shell-title" id="${s}">${p}</h2>
        </div>
        <button type="button" class="hcms-shell-close mirk-button mirk-button--small" data-hcms-action="close" aria-label="Close">
          <span class="mirk-button__label">\xD7</span>
        </button>
      </header>
      <div class="hcms-shell-notice" role="status" hidden></div>
      <div class="hcms-shell-error" role="alert" hidden></div>
      <div data-hcms-form-root class="hcms-form"></div>
      <footer class="hcms-shell-footer"${i?"":" hidden"}>
        <button type="button" class="hcms-shell-save mirk-button" trigger-save>
          <span class="mirk-button__label">Save</span>
        </button>
      </footer>
    </div>
  `,(e||o.body).appendChild(h);let u=o.body;u.classList.add("hcms-open"),dt(o),r&&u.classList.add("hcms-overlay"),t==="left"&&u.classList.add("hcms-side-left");let c=ac(h,o),f=nc(h);return{root:h,formRoot:h.querySelector("[data-hcms-form-root]"),noticeEl:h.querySelector(".hcms-shell-notice"),errorEl:h.querySelector(".hcms-shell-error"),saveButton:h.querySelector(".hcms-shell-save"),destroy(){c.detach(),f.detach(),h.remove(),u.classList.remove("hcms-open","hcms-overlay","hcms-side-left"),kn(o)},restoreChrome(){Ct(o),u.classList.add("hcms-open"),dt(o),r&&u.classList.add("hcms-overlay"),t==="left"&&u.classList.add("hcms-side-left")}}}function Ct(e){e&&(e.getElementById(et)||e.querySelector("style[data-hcms-bundled-styles]")||(ue.delete(e),vn(e)))}function vn(e){if(e&&!ue.has(e)){if(e[ec]){ue.add(e);return}if(e.getElementById(et)||e.querySelector("style[data-hcms-bundled-styles]")){ue.add(e);return}if(Qt){let t=e.createElement("style");t.id=et,t.setAttribute("save-remove",""),t.setAttribute("save-ignore",""),t.textContent=Qt,(e.head||e.documentElement).appendChild(t),ue.add(e);return}try{let t=new URL("./theme.generated.css",import.meta.url).href,r=e.createElement("link");r.rel="stylesheet",r.id=et,r.setAttribute("save-remove",""),r.setAttribute("save-ignore",""),r.href=t,(e.head||e.documentElement).appendChild(r),ue.add(e)}catch{let t=()=>{link.isConnected&&console.warn("hypercms: shell stylesheet not applied \u2014 cssText is empty and the co-located theme fallback is unavailable. Call installStyles(themeText) before opening the CMS.")};e.defaultView?.queueMicrotask?e.defaultView.queueMicrotask(t):t()}}}function nc(e){let t=e.querySelector(".hcms-shell-body"),r=e.querySelector(".hcms-shell-header");if(!t||!r||typeof t.addEventListener!="function")return{detach(){}};let i=()=>{let l=(r.offsetHeight||0)-12;e.classList.toggle("is-condensed",t.scrollTop>l)};return t.addEventListener("scroll",i,{passive:!0}),i(),{detach(){t.removeEventListener("scroll",i)}}}function ac(e,t){function r(i){if(i.key!=="Tab"||!e.contains(t.activeElement))return;let l=Array.from(e.querySelectorAll(tc));if(l.length===0)return;let n=l[0],a=l[l.length-1];i.shiftKey&&t.activeElement===n?(i.preventDefault(),a.focus()):!i.shiftKey&&t.activeElement===a&&(i.preventDefault(),n.focus())}return t.addEventListener("keydown",r),{detach:()=>t.removeEventListener("keydown",r)}}var oc="[hypercms]",Ri={skip:"[data-hcms-shell]",templateAttr:"cms-template"},qi={skip:"[data-hcms-shell]",templateAttr:null},sc=class extends Error{constructor(e,t,r){super(`hypercms: rule at "${e}" has an invalid CSS selector: "${t}"`),this.name="InvalidRuleSelector",this.path=e,this.selector=t,this.cause=r}};function yn(e,t){let r=[],i=[],l=[];return er(pe(e),e,t,[],r,i,l),{missing:Oi(r),twins:mc(i),readOnly:Oi(l)}}function xn(e){return Yt(e)}function Yt(e){if(typeof e=="string")return wn(e)?void 0:e;if(Array.isArray(e)){let[t,r]=e;return[t,Yt(r)]}if(e&&typeof e=="object"){let t={};for(let[r,i]of Object.entries(e)){let l=Yt(i);l!==void 0&&(t[r]=l)}return t}return e}function er(e,t,r,i,l,n,a){if(typeof r=="string"){let o=cc(r),s=o?Ke(e,t,o,Ri,i):[];if(wn(r)){a.push(me(i));return}if(!o)return;if(r.endsWith("[]")){s.length===0&&Ke(e,t,o,qi,i).length===0&&l.push(me(i));return}s.length===0?l.push(me(i)):s.length>1&&n.push({path:me(i),count:s.length});return}if(Array.isArray(r)){let[o,s]=r;if(typeof o!="string"||!o)return;let h=Ke(e,t,o,Ri,i);if(h.length===0){Ke(e,t,o,qi,i).length===0&&l.push(me(i));return}for(let d of h)er(e,d,s,[...i,"*"],l,n,a);return}if(r&&typeof r=="object")for(let[o,s]of Object.entries(r))er(e,t,s,[...i,o],l,n,a)}function Ke(e,t,r,i,l){try{return e.find(t,r,i)}catch(n){throw new sc(me(l),r,n)}}function cc(e){if(e==="."||e.startsWith("@"))return null;if(e.endsWith("[]"))return e.slice(0,-2)||null;let t=I(e);return(t===-1?e:e.slice(0,t))||null}function hc(e){if(e.endsWith("[]"))return null;let t=I(e);return t===-1?null:e.slice(t+1)||null}function wn(e){let t=hc(e);return t!=null&&Gi.has(t)}function Or(e){dc(e),uc(e)}function dc(e){let t=e.noticeEl;if(!t)return;let r=e.unresolved&&e.unresolved.missing||[],i=e.unresolved&&e.unresolved.readOnly||[];if(r.length===0&&i.length===0){t.textContent="",t.hidden=!0;return}let l=[];if(r.length){let n=r.length===1?"1 field no longer matches this page":`${r.length} fields no longer match this page`;l.push(`${n}: ${r.join(", ")}`)}if(i.length){let n=i.length===1?"1 field reads a property the browser will not let anything write":`${i.length} fields read properties the browser will not let anything write`;l.push(`${n}: ${i.join(", ")}`)}t.textContent=l.join(`
`),t.hidden=!1}function uc(e){let t=e.unresolved&&e.unresolved.twins||[],r=t.map(i=>`${i.path}:${i.count}`).join("|");if(r!==e.lastTwinSignature){e.lastTwinSignature=r;for(let{path:i,count:l}of t)console.warn(`${oc} "${i}" matches ${l} elements; edits go to the first one.`)}}function Oi(e){return[...new Set(e)]}function mc(e){let t=new Map;for(let r of e){let i=t.get(r.path);(!i||r.count>i.count)&&t.set(r.path,r)}return[...t.values()]}function me(e){return e.length?e.join("."):"(whole page)"}var pc={skip:"[data-hcms-shell]",templateAttr:"cms-template"};function fe(e,{ignoreActiveValue:t}={}){return fc(e,{ignoreActiveValue:t})}function fc(e,{ignoreActiveValue:t}){let r=X.findRules(e.doc,e.rulesSource||"cms");r&&(e.pageRules=e.view.prepareRules(Al(r.rules,e.pageRoot)),e.rulesTagNode=r.tagNode),Cl(e.doc),Ol(e.doc,e.pageRules),e.formRules=jl(e.pageRules,e.doc),e.writeRules=xn(e.pageRules),e.unresolved=yn(e.pageRoot,e.pageRules);let i=wl(),l=oe(X.extract(e.pageRoot,e.pageRules,{...pc,...i.hooks}),e.pageRules),n=Ar({pageRules:e.pageRules,formRules:e.formRules,data:l,doc:e.doc});kl(e.formRoot,n,{ignoreActiveValue:t}),Cr(e,l),i.seed(e.formRoot),St(e.formRoot,e.doc,e.view?.enhanceFormRichText!==!1),gn(e),Or(e),e.updateFingerprint&&e.updateFingerprint()}function gc({debounce:e=100,onRefresh:t}){let r=V("Mutation");if(!r||typeof r.onAnyChange!="function")throw new Error("hypercms: a mutation hub is required (clay.Mutation or hyperclay.Mutation). Load clayjs or hyperclayjs, or just the mutation utility, before initializing hypercms.");let i=r.onAnyChange({debounce:e},l=>{t(l)});return{unsubscribe:typeof i=="function"?i:()=>{}}}var bc='input:not([disabled]):not([type="hidden"]), textarea:not([disabled]), select:not([disabled]), button:not([disabled]), [tabindex]:not([tabindex="-1"])';function kc({doc:e,pageRoot:t,opts:r={}}){let i=r.richText!==!1,l=null;return{name:"sidebar",richText:i,enhanceFormRichText:!0,ctx:null,root:null,formRoot:null,errorEl:null,noticeEl:null,prepareRules(n){return i?Ss(n,t):n},mount(n){let a=this.ctx;l=Ee(()=>lc({mountTo:r.mountTo||e.body,side:r.side||"right",overlay:!!r.overlay,showSaveButton:!!r.showSaveButton,title:r.title,eyebrow:r.eyebrow,theme:r.theme,doc:e})),this.root=l.root,this.formRoot=l.formRoot,this.errorEl=l.errorEl,this.noticeEl=l.noticeEl;let o=Ar({pageRules:a.pageRules,formRules:a.formRules,data:n,doc:e});l.formRoot.appendChild(o),a.seeder.seed(l.formRoot),St(l.formRoot,e),Or(a),cn(a)},refresh(n){if(n==="livesync"){l?.restoreChrome?.(),fe(this.ctx,{ignoreActiveValue:!0});return}if(n==="undo"){fe(this.ctx,{ignoreActiveValue:!1});return}fe(this.ctx)},focusOnOpen(){let n=this.root&&this.root.querySelector(bc);n&&typeof n.focus=="function"&&n.focus()},destroy(){l?.destroy(),l=null}}}var vc="[cms-template], [data-hcms-shell]";function tt(e){if(!e||typeof e.getBoundingClientRect!="function"||typeof e.closest=="function"&&e.closest(vc))return!1;let t=e.getBoundingClientRect();return t.width>=8&&t.height>=8}var yc={skip:"[data-hcms-shell]",templateAttr:"cms-template"},xc={skip:"[data-hcms-shell]",templateAttr:null},wc="text",Ac="native",$i="handle",_c=new Set(["INPUT","TEXTAREA","SELECT"]),An=new Set(["IMG","INPUT","TEXTAREA","SELECT","OPTION","BR","HR","VIDEO","AUDIO","IFRAME","EMBED","OBJECT","CANVAS","SOURCE","TRACK","AREA","COL","PARAM","BUTTON","STYLE","SCRIPT","LINK","META"]);function _n(e,t,r=null){let i=[],l=[],n=pe(e);return tr(n,e,t,[],i,l,r?(a,o)=>pr(a,r,o,t):()=>null),{targets:i,lists:l}}function tr(e,t,r,i,l,n,a){if(typeof r=="string"){if(r.endsWith("[]")){let p=r.slice(0,-2);if(!p)return;let m=rt(e,t,p);m.forEach((u,c)=>{l.push(ji([...i,c],u,r,null))}),n.push(Li(e,t,i,p,m,!0));return}let o=I(r),s=o===-1?null:r.slice(o+1),h=o===-1?r:r.slice(0,o),d=!h||h==="."?t:rt(e,t,h)[0];d&&l.push(ji(i,d,r,s,a(r,i)));return}if(Array.isArray(r)){let[o,s]=r;if(typeof o!="string"||!o)return;let h=rt(e,t,o);h.forEach((d,p)=>tr(e,d,s,[...i,p],l,n,a)),n.push(Li(e,t,i,o,h,typeof s=="string"));return}if(r&&typeof r=="object")for(let[o,s]of Object.entries(r))tr(e,t,s,[...i,o],l,n,a)}function Li(e,t,r,i,l,n){let a=l[0]?l[0].parentElement:null;if(!a){let o=rt(e,t,i,xc)[0];a=o?o.parentElement:null}return{path:r,items:l,container:a,scalar:n}}function ji(e,t,r,i,l=null){return{path:e,el:t,rule:r,attr:i,kind:Ec(t,i),icon:Cc(t,i),style:l}}function Ec(e,t){let r=(e.tagName||"").toUpperCase();return!t||t==="innerHTML"?An.has(r)?$i:wc:t==="value"&&_c.has(r)&&!e.readOnly&&!Sc(e)?Ac:$i}function Sc(e){return e.disabled?!0:typeof e.matches=="function"&&e.matches(":disabled")}function Cc(e,t){let r=(e.tagName||"").toUpperCase();return(!t||t==="innerHTML")&&!An.has(r)?null:r==="IMG"||t==="srcset"?"camera":r==="A"&&t==="href"?"paperclip":"pencil"}function rt(e,t,r,i=yc){try{return e.find(t,r,i)}catch{return[]}}function Tc(e,t){let r=[],i=new Map,l=null;for(let n of e){if(!n.style)continue;let a=n.path.join(".");if(n.style==="page"){l||(l={scope:"page",el:n.el,list:null,paths:[]},r.push(l)),l.paths.push(a);continue}let o=Rc(n.path,t),s=o?o.row:n.el,h=i.get(s);h||(h={scope:o?"row":"section",el:s,list:o?o.list:null,paths:[]},i.set(s,h),r.push(h)),h.paths.push(a)}return r}function Rc(e,t){for(let r=e.length-1;r>=0;r--){if(typeof e[r]!="number")continue;let i=e.slice(0,r).join("."),l=t.find(a=>a.path.join(".")===i),n=l&&l.items[e[r]];if(n)return{list:l,row:n}}return null}var $r=Object.freeze({data:Object.freeze({tokens:["no-data"],bundles:["editor-ui"]}),save:Object.freeze({tokens:["no-save"],bundles:["editor-ui"]}),snapshot:Object.freeze({tokens:["no-snapshot"],bundles:["editor-ui"]}),watch:Object.freeze({tokens:["no-watch"],bundles:["editor-ui"]}),undo:Object.freeze({tokens:["no-undo"],bundles:["editor-ui"]}),history:Object.freeze({tokens:[],bundles:["editor-ui"]})}),qc=Object.freeze({"editor-ui":Object.freeze(["no-data","no-save","no-snapshot","no-watch","no-undo"])}),Uh=Object.freeze(["no-save","no-snapshot","no-trigger-autosave","no-dirty","no-watch","no-undo","no-data","freeze","editor-ui","live"]);function ut(e,t){if(!e||e.nodeType!==1)return!1;let r=e.getAttribute?.("clay");return!!(r&&r.split(/\s+/).includes(t)||e.hasAttribute?.(t))}function En(e,t){if(ut(e,t))return!0;for(let[r,i]of Object.entries(qc))if(i.includes(t)&&ut(e,r))return!0;return!1}function rr(e){let t=$r[e];if(!t)throw new Error(`Unknown region capability: ${e}`);return[...t.tokens,...t.bundles].flatMap(r=>[`[clay~="${r}"]`,`[${r}]`]).join(", ")}function Oc(e,t){let r=e&&e.nodeType===1?e:e?.parentElement;for(;r&&r.nodeType===1;){let i=$r[t];if(!i)throw new Error(`Unknown region capability: ${t}`);if(i.tokens.some(l=>En(r,l))||i.bundles.some(l=>ut(r,l)))return r;r=r.parentElement}return null}function $c(e,t){return!!Oc(e,t)}var Lc=/:(?:focus(?:-within|-visible)?|hover|active|visited|defined)\b/i;function jc(e,t){if(!/^(INPUT|TEXTAREA|SELECT|OPTION)$/.test(e.tagName||""))return;let r=(e.getAttribute?.("type")||"").toLowerCase();"value"in e&&"value"in t&&e.tagName!=="OPTION"&&r!=="checkbox"&&r!=="radio"&&r!=="file"&&(t.value=e.value),e.tagName==="INPUT"&&(r==="checkbox"||r==="radio")&&(t.checked=e.checked),e.tagName==="OPTION"&&(t.selected=e.selected),"indeterminate"in e&&"indeterminate"in t&&(t.indeterminate=e.indeterminate)}function Mi(e,t,r,i){if(i)return!!e.closest?.(t);let l=e;for(;l?.nodeType===1;){if(l.matches(t))return!0;if(l===r)break;l=l.parentElement}return!1}function Ni(e,t,r,i,l,n){return(n?$c(e,t):Mi(e,r,l,!1))||!!(i&&(n&&e!==l?e.matches(i):Mi(e,i,l,n)))}function Mc(e,t){let r=$r[t];return r.tokens.some(i=>En(e,i))||r.bundles.some(i=>ut(e,i))}var Nc=new Set(["INPUT","TEXTAREA","SELECT","OPTION"]);function Sn(e,t,r,i){if(e.nodeType===1&&i(e)){r.dropped.push(t);return}r.cloneToLive.set(t,e),r.liveToClone.set(e,t);let l=e,n=t;if(e.nodeType===1){if(Nc.has(e.tagName)&&r.controls.push(e,t),e.tagName==="TEMPLATE"){for(;t.firstChild;)t.removeChild(t.firstChild);l=e.content,n=t.content,r.cloneToLive.set(n,l),r.liveToClone.set(l,n)}}else if(e.nodeType!==11)return;let a=n.firstChild;for(let o=l.firstChild;o&&a;o=o.nextSibling)Sn(o,a,r,i),a=a.nextSibling}function Cn(e,{capability:t="data",exclude:r=null,inherit:i=!0,provenance:l=null}={}){if(!e)throw new TypeError("createContentView requires a DOM context");let n=e.nodeType===9?e:e.ownerDocument;if(!n?.implementation?.createHTMLDocument)throw new TypeError("createContentView requires an HTML DOM implementation");let a=n.implementation.createHTMLDocument(""),o=l||new WeakMap,s=new WeakMap,h=e.nodeType===9?e.documentElement:e;r&&a.documentElement.matches(r);let d=rr(t),p=i?c=>Mc(c,t)||!!(r&&c.matches(r)):c=>Ni(c,t,d,r,h,!1),m=null;if(h.nodeType!==1||!Ni(h,t,d,r,h,i)){m=a.importNode(h,!0);let c={cloneToLive:o,liveToClone:s,dropped:[],controls:[]};Sn(h,m,c,p);for(let f of c.dropped)f.remove();for(let f=0;f<c.controls.length;f+=2)jc(c.controls[f],c.controls[f+1])}e.nodeType===9&&m&&(a.replaceChild(m,a.documentElement),s.set(e,a),o.set(a,e));let u=c=>{if(Lc.test(c))throw new Error(`Filtered content queries do not support stateful selector: ${c}`)};return{root:m,document:a,capability:t,selector:rr(t),cloneToLive:o,liveToClone:s,original(c){return o.get(c)||null},cloneOf(c){return s.get(c)||null},query(c,f=m){return u(c),Array.from(f.querySelectorAll(c),b=>o.get(b)||b)},text(c=m){return(c===m?m:s.get(c))?.textContent||""},html(c=m){return(c===m?m:s.get(c))?.innerHTML??""},clone(c=m){let f=c===m?m:s.get(c);return f?a.importNode(f,!0):null}}}var zc=[16,8,0],D=8,Fc=4;function Dc({anchor:e,bar:t,rail:r=t,viewport:i,current:l=null}){let n=m=>l===m?Fc:0;if(e.bottom<=0||e.top>=i.height||e.right<=0||e.left>=i.width)return{mode:"hidden",x:0,y:0};let a=m=>Math.max(D,Math.min(e.left,i.width-m-D)),o=e.top-16-t.height;if(o>=D-n("above"))return{mode:"above",x:a(t.width),y:o};let s=e.bottom+16;if(s+t.height<=i.height-D+n("below"))return{mode:"below",x:a(t.width),y:s};let h=i.width-e.right-D,d=e.left-D,p=h>=d?[["rail-right",h],["rail-left",d]]:[["rail-left",d],["rail-right",h]];for(let[m,u]of p)for(let c of zc){if(r.width+c>u+n(m))continue;let f=m==="rail-right"?Math.min(e.right+c,i.width-r.width-D):Math.max(e.left-c-r.width,D),b=Math.min(e.bottom,i.height-D)-r.height,g=Math.max(D,Math.min(Math.max(e.top,D),b));return{mode:m,x:f,y:g,gap:c}}return{mode:"pinned",x:a(t.width),y:D}}function zi({anchor:e,handle:t,viewport:r,inset:i=6,prefer:l="auto"}){let n=e.height>=t.height*1.5&&e.width>=t.width*2;if(l==="corner"||n){let m=Math.max(D,Math.min(e.right-t.width+i,r.width-t.width-D)),u=Math.max(D,Math.min(e.top-i,r.height-t.height-D));return{x:m,y:u,mode:"over"}}let a=4,o=e.right+a+t.width<=r.width-D,s=o?e.right+a:e.left-a-t.width,h=Math.max(D,s),d=e.top+e.height/2-t.height/2,p=Math.max(D,Math.min(d,r.height-t.height-D));return{x:h,y:p,mode:o?"beside-right":"beside-left"}}var mt=!1;function Tn(){if(mt)return;let e=V("onPrepareForSave");typeof e=="function"&&(e(t=>{qn(t),Rn(t)}),mt=!0)}function Rn(e){let t=e&&e.querySelector&&e.querySelector("body");t&&t.classList.remove("hcms-open","hcms-overlay","hcms-side-left",qr)}function qn(e){if(!(!e||typeof e.querySelectorAll!="function"))for(let t of e.querySelectorAll(`[${ye}]`)){let r=ws(t);r!==null&&(t.innerHTML=r),t.removeAttribute(ye)}}var pt=!1;function Lr(){if(pt)return;let e=V("onSnapshot");typeof e=="function"&&(e(t=>{qn(t),_s(t,typeof window<"u"?window:null),Rn(t)}),pt=!0)}function Bc(){Tn(),Lr(),(!mt||!pt)&&Ic()}var Ze=null;function Ic(){Ze||typeof document>"u"||(Ze=Er(document,vs,()=>{Tn(),Lr(),mt&&pt&&(Ze?.(),Ze=null)}))}var Pc={edit:"M8 10h2v6H8zm2 4h4v2h-4zm0-6h2v2h-2zm2-2h2v2h-2zm2-2h2v2h-2zm2-2h2v2h-2zm2 2h2v2h-2zm2 2h2v2h-2zm-2 2h2v2h-2zm-2 2h2v2h-2zm-2 2h2v2h-2zm-4 0h2v2h-2z","move-up":"M11 4h2v2h2v2h2v2h2v2h-4v-2h-2v10h-2V10H9v2H5v-2h2V8h2V6h2z","move-down":"M11 4h2v10h2v-2h4v2h-2v2h-2v2h-2v2h-2v-2H9v-2H7v-2H5v-2h4v2h2z",remove:"M5 5h3v3h3v3h2V8h3V5h3v3h-3v3h-3v2h3v3h3v3h-3v-3h-3v-3h-2v3H8v3H5v-3h3v-3h3v-2H8V8H5z",add:"M10 4h4v6h6v4h-6v6h-4v-6H4v-4h6z"};function ft(e){let t=e==="edit"?"6 0 18 18":"0 0 24 24",r=e==="settings"?'<circle cx="6" cy="12" r="1.5"/><circle cx="12" cy="12" r="1.5"/><circle cx="18" cy="12" r="1.5"/>':`<path d="${Pc[e]}"/>`;return`<svg class="hcms-inline-icon" xmlns="http://www.w3.org/2000/svg" viewBox="${t}" width="24" height="24" fill="currentColor" aria-hidden="true" focusable="false">${r}</svg>`}var Fi="data-hcms-ghost",Hc=rr("history");function Uc(e){let t=e.container,r=null;for(;t&&(t.namespaceURI!=="http://www.w3.org/1999/xhtml"||["SELECT","OPTGROUP","DATALIST"].includes(t.tagName));)r=t,t=t.parentElement;return{parent:t,anchor:r}}function Vc({doc:e,themeRoot:t,onAdd:r,onResize:i}){let l=e.defaultView,n=[],a=!1,o=null,s=new Map;function h(u,c,f){u.style[c]!==f&&(u.style[c]=f)}function d(){let u=new Set(n.map(c=>c.node.parentElement));for(let[c,f]of s)u.has(c)&&l?.Sortable?.get(c)===f.instance||(f.instance.option("draggable")===f.selector&&f.instance.option("draggable",f.original),s.delete(c));for(let c of u){let f=l?.Sortable?.get(c);if(!f)continue;let b=f.option("draggable");if(s.get(c)?.selector===b)continue;let g=b,k=`${g.trim().startsWith(">")?"> ":""}:is(${g.trim().replace(/^>\s*/,"")}):not(${Hc})`;f.option("draggable",k),s.set(c,{instance:f,original:g,selector:k})}}function p(){for(let u of n){let{node:c,slot:f,list:b}=u,g=b.items.filter(k=>k.isConnected).map(k=>k.getBoundingClientRect()).filter(k=>k.width>0&&k.height>0);g.length&&(u.width=g.reduce((k,y)=>k+y.width,0)/g.length,u.height=g.reduce((k,y)=>k+y.height,0)/g.length),h(c,"width",u.width?`${Math.round(u.width)}px`:"100%"),h(f,"height",`${Math.max(48,Math.round(u.height||64))}px`),f!==c&&(f.colSpan=Math.max(1,...b.items.map(k=>[...k.children].reduce((y,_)=>y+(_.colSpan||1),0)))),c.hidden!==a&&(c.hidden=a)}d()}function m(u,c){let f=c.tagName,b=["TBODY","THEAD","TFOOT","TABLE"].includes(f),g=e.createElement(b?"tr":["UL","OL","MENU"].includes(f)?"li":"div");for(let A of[Fi,"data-hcms-shell","editor-ui","no-watch","no-save","save-remove","snapshot-remove"])g.setAttribute(A,"");g.setAttribute("draggable","false"),g.setAttribute("contenteditable","false"),g.className="hcms-shell pixel-quiet hcms-inline-ghost";for(let A of["light","dark"])g.classList.toggle(A,!!t?.classList.contains(A));let k=b?e.createElement("td"):g;k!==g&&(k.className="hcms-inline-ghost-cell",g.appendChild(k));let y=e.createElement("button");y.type="button",y.className="hcms-inline-list-button hcms-inline-list-add mirk-button mirk-button--small",y.setAttribute("data-hcms-list-action","add"),y.innerHTML=`<span class="mirk-button__label">${ft("add")}<span>Add</span></span>`,k.appendChild(y);let _={node:g,slot:k,button:y,list:u,width:0,height:0};y.addEventListener("click",A=>{A.preventDefault(),A.stopPropagation(),r(_.list)});for(let A of["pointerdown","mousedown","touchstart","click"])g.addEventListener(A,E=>E.stopPropagation());return g.addEventListener("dragstart",A=>{A.preventDefault(),A.stopPropagation()}),_}return{setLists(u){let c=new Set(n),f=[],b=new Set(n.map(k=>k.node)),g=new Set;for(let k of u||[]){if(!k.container||!k.container.isConnected)continue;let{parent:y,anchor:_}=Uc(k);if(!y)continue;if(!g.has(y)){for(let O of y.querySelectorAll(`:scope > [${Fi}]`))b.has(O)||O.remove();g.add(y)}let A=n.find(O=>c.has(O)&&O.list.container===k.container&&O.list.path.join(".")===k.path.join("."))||m(k,y);c.delete(A),A.list=k,A.button.setAttribute("data-hcms-list",k.path.join(".")),A.button.setAttribute("aria-label",`Add to ${k.path.join(".")||"the list"}`);let E=_||k.items.filter(O=>O.parentElement===y).at(-1);E?E.nextSibling!==A.node&&E.after(A.node):A.node.parentElement!==y&&y.appendChild(A.node),f.push(A)}for(let k of c)k.node.remove();n=f,o?.disconnect(),!o&&typeof l?.ResizeObserver=="function"&&(o=new l.ResizeObserver(()=>{p(),i?.()}));for(let k of n)for(let y of k.list.items)o?.observe(y);p()},update:p,setHidden(u){a=!!u,p()},destroy(){o?.disconnect();for(let u of n)u.node.remove();n=[],d()}}}var Di=[["move-up","Move up"],["move-down","Move down"]],Wc={prefer:"corner",inset:0};function Gc(e,t){return t.width<=e.width&&t.height<=e.height}function Jc({doc:e,layerEl:t,onActivate:r,onListAction:i,onStyle:l}){let n=e.defaultView,a=[],o=new Map,s=0,h=new Map,d=null,p=new Set,m=0,u=!1,c=null,f=null,b=null,g=null,k=new Map,y=!1,_=null,A=Vc({doc:e,themeRoot:t.closest("[data-hcms-shell]"),onResize:O,onAdd:v=>i?.({action:"add",list:v,index:v.items.length})}),E=e.createElement("div");E.className="hcms-inline-highlight",E.hidden=!0,E.setAttribute("aria-hidden","true"),t.appendChild(E);function O(){if(!(m||!n)){if(typeof n.requestAnimationFrame!="function")return ce();m=n.requestAnimationFrame(()=>{m=0,ce()})}}let H=()=>b||g;function ce(){if(!n)return;A.update();let v={width:n.innerWidth,height:n.innerHeight},x=H();for(let w of a){if(!w.visible){w.node.hidden=!0;continue}for(let T of w.members)T.node.hidden=y&&T.kind!=="handle";w.node.hidden=!1;let C=w.el.getBoundingClientRect(),S=w.node.getBoundingClientRect(),R=S.width,$=w.kind==="row"&&!Gc(C,S);if($&&w.el!==x){for(let T of w.members)T.kind==="row"&&(T.node.hidden=!0);S=w.node.getBoundingClientRect()}if(w.members.every(T=>T.node.hidden)){w.node.hidden=!0;continue}if(w.kind==="style"){let T=Math.max(8,Math.min(C.left,v.width-S.width-8)),M=Math.max(8,Math.min(C.top,v.height-S.height-8));w.node.style.transform=`translate(${Math.round(T)}px, ${Math.round(M)}px)`;continue}let P=w.kind==="handle"?null:Wc,{x:L,y:K}=zi({anchor:C,handle:S,viewport:v,...P}),he=w.members.find(T=>T.kind==="handle");if($&&he){let T=he.node.getBoundingClientRect(),M=zi({anchor:C,handle:T,viewport:v}),j=w.members.slice(w.members.indexOf(he)+1).filter(Rt=>!Rt.node.hidden).reduce((Rt,ea)=>Rt+ea.node.getBoundingClientRect().width+2,0);L=Math.max(8+R,Math.min(v.width-8,M.x+T.width+j))-S.width,K=M.y}w.node.style.transform=`translate(${Math.round(L)}px, ${Math.round(K)}px)`}if(_)if(!_.node.isConnected||_.node.closest("[hidden]"))W();else{let w=_.button.getBoundingClientRect(),C=_.menu.getBoundingClientRect();_.menu.style.left=`${Math.max(8,w.right-C.width)-w.left}px`,_.menu.style.right="auto",_.menu.style.top=w.bottom+6+C.height>v.height-8?"auto":"calc(100% + 6px)",_.menu.style.bottom=w.bottom+6+C.height>v.height-8?"calc(100% + 6px)":"auto"}Re(),c?.()}function U(v){if(v?.closest?.("[data-hcms-ghost]"))return null;for(let x=v;x&&x.nodeType===1;x=x.parentElement){let w=k.get(x);if(w)return w}return null}function le(v){_&&!_.node.contains(v.target)&&W();let x=H();g=U(v.target),g&&t.contains(v.target)&&(b=null),H()!==x&&O()}function Te(v){if(v.relatedTarget||!g)return;let x=H();g=null,H()!==x&&O()}function Re(){if(!f||E.hidden)return;let v=f.getBoundingClientRect();E.style.width=`${Math.round(v.width)}px`,E.style.height=`${Math.round(v.height)}px`,E.style.transform=`translate(${Math.round(v.left)}px, ${Math.round(v.top)}px)`}function In(){if(!n||typeof n.IntersectionObserver!="function"){for(let x of a)x.visible=!0;return}d||(d=new n.IntersectionObserver(x=>{let w=!1;for(let C of x)for(let S of o.get(C.target)||[])S.visible!==C.isIntersecting&&(S.visible=C.isIntersecting,w=!0);w&&O()},{threshold:0}));let v=new Set(o.keys());for(let x of p)v.has(x)||d.unobserve(x);for(let x of v)p.has(x)||d.observe(x);p=v}function Pn(){u||!n||(n.addEventListener("scroll",O,{passive:!0,capture:!0}),n.addEventListener("resize",O,{passive:!0}),e.addEventListener("focusin",le),e.addEventListener("focusout",Te),e.addEventListener("pointerdown",Vr,!0),e.addEventListener("keydown",Wr,!0),u=!0)}function Hn(){!u||!n||(n.removeEventListener("scroll",O,{capture:!0}),n.removeEventListener("resize",O),e.removeEventListener("focusin",le),e.removeEventListener("focusout",Te),e.removeEventListener("pointerdown",Vr,!0),e.removeEventListener("keydown",Wr,!0),u=!1)}function Br(v){return`${v.kind}\0${v.attr||""}`}function Ir(v,x){return v.container===x.container&&v.scalar===x.scalar}function Un(v,x){return v.kind!==x.kind?!1:v.kind==="style"?!0:v.kind==="handle"?Br(v.target)===Br(x.target):v.kind==="row"?v.row===x.row&&Ir(v.list,x.list):Ir(v.list,x.list)}function Pr(v,x){v.target=x;let w=x.path.join(".");v.node.setAttribute("data-hcms-target",w),x.icon?v.node.setAttribute("data-hcms-icon",x.icon):v.node.removeAttribute("data-hcms-icon"),v.node.setAttribute("aria-label",`Edit ${w}`)}function Vn(v){let x=e.createElement("button");x.type="button",x.className="hcms-inline-handle mirk-button mirk-button--small",x.innerHTML=`<span class="mirk-button__label">${ft("edit")}</span>`;let w={node:x,kind:"handle",target:v};return Pr(w,v),x.addEventListener("click",C=>{C.preventDefault(),C.stopPropagation(),r?.(w.target,x)}),w}function Wn(v){let x=e.createElement("button");x.type="button",x.className="hcms-inline-style-chip mirk-button mirk-button--small",x.setAttribute("aria-label","Edit style"),x.innerHTML='<span class="mirk-button__label">Style</span>';let w={node:x,kind:"style",group:v.group};return x.addEventListener("click",C=>{C.preventDefault(),C.stopPropagation(),l?.(w.group,x)}),w}function Hr(v,x){let w=e.createElement("button");return w.type="button",w.className="hcms-inline-list-button mirk-button mirk-button--small",w.setAttribute("data-hcms-list-action",v),w.setAttribute("aria-label",x),w.innerHTML=`<span class="mirk-button__label">${ft(v)}${v==="add"?"<span>Add</span>":""}</span>`,w}function Ur(v,{list:x,row:w,rowIndex:C,count:S}){let R=x.path.join(".");v.list=x,v.row=w,v.index=C,v.count=S,v.node.setAttribute("data-hcms-list",R),v.node.setAttribute("data-hcms-row",String(C));for(let $ of v.node.querySelectorAll("[data-hcms-list-action]")){let P=$.getAttribute("data-hcms-list-action"),L=Di.find(([K])=>K===P)?.[1]||P;$.setAttribute("aria-label",`${L} ${R}.${C}`),$.disabled=P==="move-up"&&C===0||P==="move-down"&&C===S-1}}function Gn(v,x,w,C){let S=e.createElement("div");S.className="hcms-inline-row-controls";let R={node:S,kind:"row",list:v,row:x,index:w,count:C};for(let[$,P]of Di){let L=Hr($,P);L.addEventListener("click",K=>{K.preventDefault(),K.stopPropagation(),!L.disabled&&i?.({action:$,list:R.list,index:R.index,row:R.row})}),S.appendChild(L)}return Ur(R,{list:v,row:x,rowIndex:w,count:C}),R}function W(v=!1){if(!_)return;let x=_;_=null,x.menu.hidden=!0,x.button.setAttribute("aria-expanded","false"),x.node.parentElement?.classList.remove("has-open-settings"),v&&x.button.isConnected&&x.button.focus({preventScroll:!0})}function Vr(v){_&&!_.node.contains(v.target)&&W()}function Wr(v){if(_){if(v.key==="Escape")v.preventDefault(),v.stopPropagation(),W(!0);else if(v.key==="Tab")W(!0);else if(["ArrowDown","ArrowUp","Home","End"].includes(v.key)&&_.menu.contains(v.target)){v.preventDefault();let x=[..._.menu.querySelectorAll('[role="menuitem"]')],w=x.indexOf(v.target),C=v.key==="Home"?0:v.key==="End"?x.length-1:w===-1?0:(w+(v.key==="ArrowDown"?1:-1)+x.length)%x.length;x[C]?.focus({preventScroll:!0})}}}function Gr(v,{list:x,row:w,rowIndex:C,style:S=null}){Object.assign(v,{list:x,row:w,index:C,style:S}),v.node.setAttribute("data-hcms-list",x.path.join(".")),v.node.setAttribute("data-hcms-row",String(C)),v.button.setAttribute("aria-label",`Settings ${x.path.join(".")}.${C}`),S&&!v.styleItem.isConnected?v.menu.insertBefore(v.styleItem,v.menu.firstChild):S||v.styleItem.remove()}function Jn(v){let x=e.createElement("div");x.className="hcms-inline-settings";let w=Hr("settings","Settings");w.setAttribute("aria-haspopup","menu"),w.setAttribute("aria-expanded","false");let C=e.createElement("div");C.className="hcms-inline-settings-menu",C.setAttribute("role","menu"),C.setAttribute("aria-label","Item settings"),C.hidden=!0;let S=e.createElement("button");S.type="button",S.setAttribute("role","menuitem"),S.setAttribute("data-hcms-style-action",""),S.textContent="Style";let R=e.createElement("button");R.type="button",R.setAttribute("role","menuitem"),R.setAttribute("data-hcms-list-action","remove"),R.textContent="Delete",C.appendChild(R),x.append(w,C);let $={node:x,button:w,menu:C,styleItem:S,kind:"settings"};Gr($,v);let P=()=>{W(),_=$,C.hidden=!1,w.setAttribute("aria-expanded","true"),x.parentElement.classList.add("has-open-settings"),C.querySelector('[role="menuitem"]').focus({preventScroll:!0}),O()};return w.addEventListener("click",L=>{L.preventDefault(),L.stopPropagation(),_===$?W(!0):P()}),w.addEventListener("keydown",L=>{(L.key==="ArrowDown"||L.key==="ArrowUp")&&(L.preventDefault(),P())}),R.addEventListener("click",L=>{L.preventDefault(),L.stopPropagation(),W(!0),i?.({action:"remove",list:$.list,index:$.index,row:$.row})}),S.addEventListener("click",L=>{L.preventDefault(),L.stopPropagation(),W(),l?.($.style,$.button)}),$}function Xn(v,x){v.kind==="handle"?Pr(v,x.target):v.kind==="row"?Ur(v,x):v.kind==="settings"?Gr(v,x):v.kind==="style"&&(v.group=x.group)}function We(v,x,w,C,S){let R=x.get(w)?.find($=>$.category===S);if(!R){R={el:w,category:S,members:[]},v.push(R);let $=x.get(w);$?$.push(R):x.set(w,[R])}R.members.push(C)}function Kn(v,x,w,C){let S=w.items||[];S.forEach((R,$)=>{tt(R)&&(We(v,x,R,{kind:"row",list:w,row:R,rowIndex:$,count:S.length},"item"),We(v,x,R,{kind:"settings",list:w,row:R,rowIndex:$,style:C.get(R)||null},"item"))})}function Zn(v,x){let w=new Set(v.members),C=[];for(let S of x){let R=v.members.find($=>w.has($)&&Un($,S))||(S.kind==="handle"?Vn(S.target):S.kind==="row"?Gn(S.list,S.row,S.rowIndex,S.count):S.kind==="style"?Wn(S):Jn(S));w.delete(R),Xn(R,S),y&&R.kind!=="handle"&&(R.node.hidden=!0),C.push(R)}for(let S of w)S===_&&W(),S.node.remove();C.sort((S,R)=>["row","handle","settings"].indexOf(S.kind)-["row","handle","settings"].indexOf(R.kind)),C.forEach((S,R)=>{v.node.children[R]!==S.node&&v.node.insertBefore(S.node,v.node.children[R]||null)}),v.members=C}function Qn(v,x,w){let C=[],S=new Map,R=new Map,$=0;for(let T of v||[])T.style||(R.has(T.el)||R.set(T.el,T),!(T.kind!=="handle"||!tt(T.el))&&(We(C,S,T.el,{kind:"handle",target:T},"item"),$++));let P=new Map;for(let T of w||[])T.scope==="row"?P.set(T.el,T):T.scope==="section"&&tt(T.el)&&We(C,S,T.el,{kind:"style",group:T},"style");for(let T of x||[])Kn(C,S,T,P);let L=new Set(a),K=new Map;for(let T of a)K.has(T.el)||K.set(T.el,T.visible);let he=[];for(let T of C){let M=a.find(j=>L.has(j)&&j.el===T.el&&j.category===T.category);if(!M){let j=e.createElement("div");j.className="hcms-inline-item-controls",j.setAttribute("role","group"),M={el:T.el,node:j,category:T.category,kind:T.category==="add"?"add":"handle",members:[],visible:K.get(T.el)??!1},t.appendChild(j)}L.delete(M),M.el=T.el,M.category=T.category,Zn(M,T.members),M.kind=M.members.some(j=>j.kind==="row")?"row":M.members.some(j=>j.kind==="handle")?"handle":M.members.some(j=>j.kind==="style")?"style":"add",M.node.setAttribute("aria-label",M.category==="add"?"List controls":M.category==="style"?"Style controls":"Item controls"),he.push(M)}for(let T of L)T.node.remove();a=he,o=new Map,k=new Map;for(let T of a){let M=o.get(T.el);M?M.push(T):o.set(T.el,[T]);for(let j of T.members)j.kind!=="row"&&j.kind!=="settings"||(k.set(j.row,j.row),k.set(j.node,j.row),k.set(T.node,j.row))}b&&!k.has(b)&&(b=null),g&&!k.has(g)&&(g=null),h=R,s=$}function Yn(){W(),d?.disconnect(),d=null,p=new Set;for(let v of a)v.node.remove();a=[],o=new Map,h=new Map,k=new Map,s=0}return{setTargets(v,x,w=[]){Qn(v,x,w),A.setLists(x),In(),Pn(),O()},refresh:O,get count(){return s},get controlsHidden(){return y},setControlsHidden(v){if(y=!!v,A.setHidden(y),y){W();for(let x of a){for(let w of x.members)w.kind!=="handle"&&(w.node.hidden=!0);x.members.every(w=>w.node.hidden)&&(x.node.hidden=!0)}}O()},elementToTarget(v){if(v?.closest?.("[data-hcms-ghost]"))return null;for(let x=v;x&&x.nodeType===1;x=x.parentElement){let w=h.get(x);if(w)return w}return null},setHoveredRow(v){let x=H();b=v?U(v):null,H()!==x&&O()},showHighlight(v){v&&(f=v,E.hidden=!1,Re())},hideHighlight(){f=null,E.hidden=!0},setFollower(v){c=typeof v=="function"?v:null},destroy(){A.destroy(),m&&n&&n.cancelAnimationFrame(m),m=0,c=null,f=null,b=null,g=null,E.remove(),Hn(),Yn()}}}var Xc="hypercms-inline",ge="is-hcms-inline-active",ir="is-hcms-inline-onpath",Bi='input:not([disabled]):not([type="hidden"]), textarea:not([disabled]), select:not([disabled]), button:not([disabled]), [tabindex]:not([tabindex="-1"])',Kc=["bold","italic","link","undo","redo"],Zc=/^H[1-6]$/,Qc=0;function Yc({doc:e,pageRoot:t,opts:r={}}){let i=r.richText!==!1,l=null,n=null,a=null,o=new Map,s=null,h=null,d=[],p=null;function m(u,c,f,b,g){let k=Et(e.defaultView);if(typeof k!="function")return null;let y=g||{},_=y.originalHTML??Cn(c,{capability:"history"}).html(),A=y.richClayIsOurs??!c.hasAttribute("data-richclay"),E=y.adopted??c.getAttribute("data-richclay-active")==="true",O=Fe(c,b,"history"),H=eh(u,k,c,b,A);if(!H)return null;As(c,b==="innerHTML",A);let ce=String(++Qc);c.setAttribute(ye,ce),Lr();let U={el:c,editor:H,path:f,prop:b,boundId:ce,originalHTML:_,oldValue:O,richClayIsOurs:A,adopted:E,dirty:!1,written:!1,restorable(){return!this.adopted&&!this.dirty&&!this.written},lastEdited:void 0};o.set(c,U),ys(ce,U);let le=H.squire;if(le&&typeof le.addEventListener=="function"){let Re=()=>{U.dirty=!0,U.lastEdited=Fe(c,U.prop,"history"),th(u,U)};le.addEventListener("input",Re),U.detachInput=()=>le.removeEventListener?.("input",Re)}let Te=()=>On(U);return c.addEventListener("blur",Te),U.detachBlur=()=>c.removeEventListener("blur",Te),U}return{name:"inline",richText:i,ctx:null,root:null,formRoot:null,errorEl:null,noticeEl:null,handoffEl:null,handoffCountEl:null,popEl:null,enhanceFormRichText:!1,prepareRules(u){return i?Cs(u,t):u},bindText(u,c,f,b){return m(this.ctx,u,c,f,b)},mount(u){let c=this.ctx;l=nh(e,r.theme),this.root=l.root,this.formRoot=l.formRoot,this.errorEl=l.errorEl,this.noticeEl=l.noticeEl,this.handoffEl=l.handoffEl,this.handoffCountEl=l.handoffCountEl,this.popEl=l.popEl,Ee(()=>dt(e));let f=Ar({pageRules:c.pageRules,formRules:c.formRules,data:u,doc:e});l.formRoot.appendChild(f),c.seeder.seed(l.formRoot),St(l.formRoot,e,this.enhanceFormRichText),Or(c),cn(c),n=Jc({doc:e,layerEl:l.layerEl,onActivate:(b,g)=>this.activate(b,g),onListAction:b=>this.listAction(b),onStyle:(b,g)=>this.openStyle(b,g)}),n.setFollower(()=>this.placePopover()),l.closeEl.addEventListener("click",()=>this.deactivate()),l.toggleEl.addEventListener("click",b=>{b.preventDefault(),b.stopPropagation(),this.toggleControls()}),l.pageStyleEl.addEventListener("click",b=>{b.preventDefault(),b.stopPropagation();let g=d.find(k=>k.scope==="page");g&&this.openStyle(g,l.pageStyleEl)}),l.handoffEl.querySelector("[data-hcms-open-view]").addEventListener("click",b=>{b.preventDefault(),b.stopPropagation(),c.onViewRequested?.("sidebar")}),this.bindPage(),this.syncTargets()},listAction({action:u,list:c,index:f,row:b}){let g=this.ctx,k=c.path.join("."),y=g.formRoot.querySelector(`[data-hcms-path="${Se(k)}"]`);if(!y)return;if(u==="add"){Tr(k,g);return}let _=b?oh(g,k,b):f;if(_===-1)return;let A=ah(y,_);if(A){if(u==="remove"){mn(A,g),this.syncTargets();return}un(A,u==="move-up"?-1:1,g),this.syncTargets()}},toggleControls(){if(!n||!l)return;let u=!n.controlsHidden;n.setControlsHidden(u),l.toggleEl.setAttribute("aria-pressed",String(u));let c=l.toggleEl.querySelector(".mirk-button__label");c&&(c.textContent=u?"Show controls":"Hide controls")},bindPage(){let u=ae(this.ctx.pageRoot),c=l.root,f=A=>{let E=n&&n.elementToTarget(A.target);E?n.showHighlight(E.el):n?.hideHighlight(),n?.setHoveredRow(A.target)},b=()=>{n?.hideHighlight(),n?.setHoveredRow(null)},g=A=>{if(c.contains(A.target))return;let E=n&&n.elementToTarget(A.target);!E||E.kind!=="text"||o.has(E.el)||m(this.ctx,E.el,E.path.join("."),gt(E))},k=A=>{if(c.contains(A.target))return;let E=n&&n.elementToTarget(A.target);if(!E){this.deactivate();return}(typeof A.target.closest=="function"&&A.target.closest("a[href]")||E.kind!=="text")&&A.preventDefault(),this.activate(E)},y=A=>{A.key==="Escape"&&this.deactivate()},_=A=>{let E=A.detail;if(!(!E||E.pageRoot!==ae(this.ctx.pageRoot)||!E.path))for(let O of o.values())O.path===E.path&&(O.written=!0)};u.addEventListener("pointerover",f),u.addEventListener("pointerleave",b),u.addEventListener("pointerdown",g),u.addEventListener("click",k),c.addEventListener("keydown",y),e.addEventListener("hcms:change",_),a=()=>{u.removeEventListener("pointerover",f),u.removeEventListener("pointerleave",b),u.removeEventListener("pointerdown",g),u.removeEventListener("click",k),c.removeEventListener("keydown",y),e.removeEventListener("hcms:change",_)}},activate(u,c){if(!u||!l||u.kind==="text"&&this.activateText(u))return;let f=Ii(this,l,u.path.join("."));if(!f){this.deactivate();return}s=u,h=c||null,p=null,l.popTitleEl.textContent="Edit content",l.popEl.hidden=!1,l.formRoot.querySelectorAll(`.${ge}`).forEach(Bt),this.placePopover(),Hi(f)},openStyle(u,c){if(!u||!l)return;let f=Pi(this,l,u.paths);if(!f){this.deactivate();return}s={kind:"style",scope:u.scope,el:u.scope==="page"?l.pageStyleEl:u.el,path:u.paths[0].split("."),paths:u.paths},h=c||null,p=null,l.popTitleEl.textContent=u.scope==="page"?"Page style":"Style",l.popEl.hidden=!1,l.formRoot.querySelectorAll(`.${ge}`).forEach(Bt),this.placePopover(),Hi(f)},activateText(u){let c=u.el,f=o.get(c);if(f)return it(f),!0;let b=m(this.ctx,c,u.path.join("."),gt(u));return b?(it(b),!0):!1},placePopover(){if(!l||!s||l.popEl.hidden)return;let u=e.defaultView;if(!u)return;let c={width:u.innerWidth,height:u.innerHeight},f=s.el.getBoundingClientRect(),b=l.popEl.getBoundingClientRect(),{mode:g,x:k,y}=Dc({anchor:f,bar:b,viewport:c,current:p});p=g,l.popEl.style.transform=`translate(${Math.round(k)}px, ${Math.round(y)}px)`},deactivate(){if(!l||!s&&l.popEl.hidden)return;l.popEl.hidden=!0,jr(l.root);let u=h;if(s=null,h=null,p=null,u&&e.contains(u)&&typeof u.focus=="function")try{u.focus({preventScroll:!0})}catch{u.focus()}},syncTargets(){if(!n)return;let u=this.ctx,c=_n(u.pageRoot,u.pageRules,e),f=c.lists,b=c.targets.filter(y=>!y.style);d=Tc(c.targets,f),l.pageStyleEl.hidden=!d.some(y=>y.scope==="page"),n.setTargets(c.targets,f,d);let g=rh(this,o,b,e);if(s?.kind==="style"){let y=s.scope==="page"?d.find(_=>_.scope==="page"):d.find(_=>_.el===s.el);y?s={...s,paths:y.paths,path:y.paths[0].split(".")}:this.deactivate()}else if(s){let y=g.get(s.el)||[],_=y.find(A=>ih(A,s))||y.find(A=>A.path.join(".")===s.path.join("."))||(y.length===1?y[0]:null);_?s=_:this.deactivate()}lh(this.ctx,o,e);let k=b.reduce((y,_)=>y+(tt(_.el)?0:1),0);this.handoffEl&&(this.handoffCountEl.textContent=k===0?"":`${k} ${k===1?"field isn't":"fields aren't"} visible right now.`,this.handoffEl.hidden=k===0)},refresh(u,c){u==="livesync"?(Ct(e),dt(e),fe(this.ctx,{ignoreActiveValue:!0})):u==="undo"?fe(this.ctx,{ignoreActiveValue:!1}):fe(this.ctx),this.syncTargets(),(u==="livesync"||u==="undo")&&this.rebindText(u),this.restoreActive()},rebindText(u){let c=u!=="undo";for(let[f,b]of[...o]){if(!e.contains(f)){De(this.ctx,b,{restore:!1,record:c}),o.delete(f);continue}if(f.hasAttribute(re)){let y=Fe(f,b.prop);y!==b.lastEdited&&(b.oldValue=y,b.lastEdited=void 0);continue}let g=e.activeElement===f;De(this.ctx,b,{restore:!1,record:c}),o.delete(f);let k=m(this.ctx,f,b.path,b.prop,{richClayIsOurs:b.richClayIsOurs,adopted:b.adopted});k&&g&&it(k)}},restoreActive(){if(!(!l||!s||l.popEl.hidden)){if(!e.contains(s.el)){this.deactivate();return}if(!(s.kind==="style"?Pi(this,l,s.paths):Ii(this,l,s.path.join(".")))){this.deactivate();return}l.formRoot.querySelectorAll(`.${ge}`).forEach(Bt),this.placePopover()}},focusOnOpen(){if(this.root&&typeof this.root.focus=="function")try{this.root.focus({preventScroll:!0})}catch{this.root.focus()}},destroy(){h=null,this.deactivate();for(let u of o.values())De(this.ctx,u);o.clear(),a?.(),a=null,n?.destroy(),n=null,l?.destroy(),l=null,kn(e),this.popEl=null,this.handoffEl=null,this.handoffCountEl=null}}}function eh(e,t,r,i,l){return Ee(()=>{let n=null;try{n=new t(r,{inline:!0,hyperclay:!1,toolbar:i==="innerHTML"?Kc:!1,...Zc.test(r.tagName)?{singleLine:!0}:null})}catch(a){return console.warn("[hypercms] richclay activation failed; the field falls back to the popover",a),null}if(n.unsupported||!n.active){if(l)try{n.destroy()}catch{}return null}return typeof n.reattach=="function"&&n.reattach(),n})}function th(e,{path:t,el:r,prop:i}){let l=pn(e.formRoot,t);l&&(fn(l,Fe(r,i),e.formRoot,t),Y(J(e),{path:t,structural:!1},e))}function Fe(e,t,r="data"){let i=Cn(e,{capability:r});return t==="innerHTML"?i.html():i.text().trim()}function On(e){let{el:t,prop:r,oldValue:i,lastEdited:l}=e,n=l;if(n===void 0||n===i)return;e.oldValue=n;let a=V("undo");if(!a||typeof a.recordValue!="function")return;let o={prop:r,oldValue:i,newValue:n,read:s=>Fe(s,r,"history"),write:(s,h)=>$n(s,r,h)};a.isPaused?queueMicrotask(()=>{t.isConnected&&a.recordValue(t,o)}):a.recordValue(t,o)}function $n(e,t,r){let i=e.cloneNode(!1);t==="innerHTML"?i.innerHTML=r:i.textContent=r,qe(e,Array.from(i.childNodes),{morphStyle:"innerHTML",policy:"history",restoreFocus:!1,scripts:{handle:!1,merge:!1}})}function De(e,t,{restore:r=!0,record:i=!0}={}){i&&On(t),t.detachInput?.(),t.detachBlur?.(),Ee(()=>{if(!t.adopted){try{t.editor.destroy()}catch(l){console.warn("[hypercms] richclay teardown failed; editor state may reach the save",l)}t.richClayIsOurs&&t.el.removeAttribute("data-richclay")}t.el.removeAttribute(re),t.el.removeAttribute(xe),t.el.removeAttribute(ye),xs(t.boundId),r&&t.restorable()&&$n(t.el,"innerHTML",t.originalHTML)})}function it(e){let{editor:t,el:r}=e;if(r.ownerDocument.activeElement!==r)try{typeof t.focus=="function"?t.focus():r.focus()}catch{}}function rh(e,t,r,i){let l=new Map;for(let n of r){let a=l.get(n.el);a?a.push(n):l.set(n.el,[n])}for(let[n,a]of[...t]){let o=l.get(n)||[],s=o.find(m=>gt(m)===a.prop)||o[0];if(!s){De(e.ctx,a,{restore:!0}),t.delete(n);continue}a.path=s.path.join(".");let h=gt(s);if(h===a.prop)continue;let d=i.activeElement===n;De(e.ctx,a,{restore:!1}),t.delete(n);let p=e.bindText(n,a.path,h,{richClayIsOurs:a.richClayIsOurs,adopted:a.adopted,originalHTML:a.originalHTML});p&&d&&it(p)}return l}function ih(e,t){return e.el===t.el&&e.kind===t.kind&&e.attr===t.attr}function gt(e){return e.attr==="innerHTML"?"innerHTML":"textContent"}function lh(e,t,r){for(let i of e.pageRoot.querySelectorAll(`[${re}]`))t.has(i)||Ee(()=>Es(i,r.defaultView))}function Ii(e,t,r){let i=e.formRoot&&e.formRoot.querySelector(`[data-hcms-path="${Se(r)}"]`);if(jr(t.root),!i)return null;let l=i.closest("[data-hcms-card]"),n=l?[...l.querySelectorAll('[data-hcms-shape="scalar"][data-hcms-path]')].filter(a=>a.closest("[data-hcms-card]")===l&&a.closest('[data-hcms-shape="object-array"], [data-hcms-shape="scalar-array"]')===l.parentElement.closest('[data-hcms-shape="object-array"]')):[i];n.includes(i)||n.push(i);for(let a of n)Ln(e,a);return i}function Pi(e,t,r){jr(t.root);let i=null;for(let l of r){let n=e.formRoot&&e.formRoot.querySelector(`[data-hcms-path="${Se(l)}"]`);n&&(i=i||n,Ln(e,n))}return i}function Ln(e,t){t.classList.add(ge),t.removeAttribute("draggable");for(let r=t.parentElement;r&&(r.classList.add(ir),r.removeAttribute("draggable"),r!==e.formRoot);r=r.parentElement);}function jr(e){if(e)for(let t of e.querySelectorAll(`.${ge}, .${ir}`))t.classList.remove(ge,ir)}function Bt(e){e.tagName==="TEXTAREA"&&ot(e),e.querySelectorAll?.("textarea").forEach(ot)}function Hi(e){let t=e.matches?.(Bi)?e:e.querySelector?.(Bi);if(!(!t||typeof t.focus!="function"))try{t.focus({preventScroll:!0})}catch{t.focus()}}function nh(e,t){Ct(e);let r=e.createElement(Xc),i=t==="dark"?" dark":t==="light"?" light":"";return r.className="hcms-shell pixel-quiet hcms-inline"+i,r.setAttribute("data-hcms-shell",""),r.setAttribute("editor-ui",""),r.setAttribute("no-save",""),r.setAttribute("save-remove",""),r.setAttribute("snapshot-remove",""),r.setAttribute("no-watch",""),r.setAttribute("tabindex","-1"),r.innerHTML=`
    <div class="hcms-inline-bar">
      <div class="hcms-inline-handoff" hidden>
        <span class="hcms-inline-handoff-count"></span>
        <button type="button" class="hcms-inline-handoff-open mirk-button mirk-button--small" data-hcms-open-view="sidebar">
          <span class="mirk-button__label">Edit in the sidebar</span>
        </button>
      </div>
      <button type="button" class="hcms-inline-page-style mirk-button mirk-button--small" data-hcms-page-style-open hidden>
        <span class="mirk-button__label">Page style</span>
      </button>
      <button type="button" class="hcms-inline-toggle mirk-button mirk-button--small" data-hcms-controls-toggle aria-pressed="false" hidden>
        <span class="mirk-button__label">Hide controls</span>
      </button>
      <div class="hcms-inline-notice" role="status" hidden></div>
      <div class="hcms-inline-error" role="alert" hidden></div>
    </div>
    <div class="hcms-inline-layer"></div>
    <div class="hcms-inline-pop" role="dialog" aria-label="Edit content" hidden>
      <div class="hcms-inline-pop-header">
        <span class="hcms-inline-pop-title">Edit content</span>
        <button type="button" class="hcms-inline-pop-close mirk-button mirk-button--small" aria-label="Close field editor">
          <span class="mirk-button__label">${ft("remove")}</span>
        </button>
      </div>
      <div data-hcms-form-root class="hcms-form"></div>
    </div>
  `,e.body.appendChild(r),{root:r,formRoot:r.querySelector("[data-hcms-form-root]"),noticeEl:r.querySelector(".hcms-inline-notice"),errorEl:r.querySelector(".hcms-inline-error"),handoffEl:r.querySelector(".hcms-inline-handoff"),handoffCountEl:r.querySelector(".hcms-inline-handoff-count"),layerEl:r.querySelector(".hcms-inline-layer"),popEl:r.querySelector(".hcms-inline-pop"),closeEl:r.querySelector(".hcms-inline-pop-close"),toggleEl:r.querySelector("[data-hcms-controls-toggle]"),pageStyleEl:r.querySelector("[data-hcms-page-style-open]"),popTitleEl:r.querySelector(".hcms-inline-pop-title"),destroy(){r.remove()}}}function ah(e,t){let r=e.querySelector(".hcms-array-items");return r&&r.querySelectorAll(":scope > [data-hcms-card], :scope > [data-hcms-array-item]")[t]||null}function oh(e,t,r){let{lists:i}=_n(e.pageRoot,e.pageRules),l=i.find(n=>n.path.join(".")===t);return l?l.items.indexOf(r):-1}var sh="[hypercms]";function ch(e,t){if(!e||!e.querySelectorAll||!t)return;let r=hh(t);e.querySelectorAll("template[data-hcms-tpl]").forEach(i=>{let l=i.getAttribute("data-hcms-tpl");l&&(l.startsWith("@")||r.has(l)||console.warn(`${sh} template "${l}" doesn't match any rule path; ignored`))})}function hh(e){let t=new Set;return r([],e),t;function r(i,l){let n=i.join("."),a=i.map(s=>typeof s=="number"?"*":s).join(".");n&&t.add(n),a&&t.add(a);let o=Pe(l);if(o==="object")for(let[s,h]of Object.entries(l))r([...i,s],h);else if(o==="object-array"||o==="scalar-array"){let s=[...i,"*"],h=s.map(d=>typeof d=="number"?"*":d).join(".");if(t.add(h),o==="object-array"){let d=l[1];if(d&&typeof d=="object"&&!Array.isArray(d))for(let[p,m]of Object.entries(d))r([...s,p],m)}}}}var lr={skip:"[data-hcms-shell]",templateAttr:"cms-template"},q={isOpen:!1,ctx:null,opts:null};function dh({view:e,doc:t,pageRoot:r,opts:i={},onCloseRequested:l,onViewRequested:n}){let a=i.rules!==void 0?i.rules:"cms",o=X.findRules(t,a);if(!o){let b=typeof a=="string"?`data-rules-name~="${a}"`:"the provided rules object";throw new Error(`hypercms: no rules found for ${b}`)}let s=e.prepareRules(Al(o.rules,r)),h=o.tagNode;Cl(t),Ol(t,s),ch(t,s);let d=jl(s,t),p=xn(s),m=yn(r,s),u=wl(),c=oe(X.extract(r,s,{...lr,...u.hooks}),s),f={doc:t,pageRoot:r,pageRules:s,writeRules:p,formRules:d,rulesTagNode:h,rulesSource:a,richText:e.richText,view:e,seeder:u,initialData:c,get formRoot(){return e.formRoot},get shellRoot(){return e.root},get errorEl(){return e.errorEl},get noticeEl(){return e.noticeEl},unresolved:m,lastTwinSignature:null,lastFingerprint:null,lastData:null,observerHandle:null,undoUnsub:null,livesyncUnsub:null,onChange:i.onChange,onError:i.onError,confirmRemove:i.confirmRemove,previouslyFocused:t.activeElement,quiesced:!1,dispatch(b,g){let k=t.defaultView&&t.defaultView.CustomEvent||(typeof CustomEvent<"u"?CustomEvent:null);if(!k)return;let y={...g||{},pageRoot:ae(r),view:e.name},_=new k(b,{bubbles:!0,cancelable:b==="hcms:change",detail:y});(e.root&&e.root.isConnected!==!1?e.root:ae(r)).dispatchEvent(_)},onCloseRequested:l,onViewRequested:n};return f.updateFingerprint=()=>{f.lastFingerprint=ht(J(f))},e.ctx=f,f}function uh(e){e.updateFingerprint(),Cr(e,oe(X.extract(e.pageRoot,e.pageRules,lr),e.pageRules)),e.observerHandle=gc({onRefresh:i=>e.view.refresh("observer",i)});let t=V("undo");if(t&&typeof t.on=="function"){let i=()=>{if(q.ctx!==e)return;Ui(e,"undo");let l=oe(X.extract(e.pageRoot,e.pageRules,lr),e.pageRules);ht(l)!==ht(e.lastData)&&(e.lastData=l,e.onChange?.(l,{path:"",structural:!1}))};t.on("undo",i),t.on("redo",i),e.undoUnsub=()=>{t.off("undo",i),t.off("redo",i)}}let r=()=>Ui(e,"livesync");e.livesyncUnsub=Er(e.doc,ks,r),zr.ctx=e,bh(e.doc)}function Ui(e,t){q.ctx===e&&e.view.refresh(t)}function bt(e,{dispatch:t=!0,restoreFocus:r=!0,updateUrl:i=!0,reason:l="close"}={}){if(!e||e.closed)return;e.closed=!0;for(let a of e.uploads||[])try{a.abort()}catch{}e.uploads?.clear();let n=e.previouslyFocused;if(t&&e.dispatch("hcms:close",{reason:l}),i&&yh(),e.observerHandle?.unsubscribe?.(),e.undoUnsub?.(),e.livesyncUnsub?.(),e.detachEvents?.(),Ee(()=>e.view.destroy()),mh(),r&&typeof n?.focus=="function")try{n.focus()}catch{}}function mh(){q.isOpen=!1,q.ctx=null,q.opts=null,zr.ctx=null}function Mr(e){let t=e||(typeof window<"u"?window:null),r=t&&t.clay?t.clay.modeLifecycle:null;return r&&typeof r.canWrite=="function"?r:null}function Nr(e){let t=Mr(e);if(!t)return!0;try{return t.canWrite()!==!1}catch{return!1}}var ph="[data-hcms-uploading]";function jn(e){if(!e)return[];let t=[];return e.uploads&&e.uploads.size>0&&t.push("upload"),e.pageRoot&&typeof e.pageRoot.querySelector=="function"&&e.pageRoot.querySelector(ph)&&t.push("preview"),t}function fh(e){if(!e||e.closed)return{ok:!0};let t=jn(e);return t.length?{ok:!1,code:"pending-input",pending:t}:(e.quiesced=!0,{ok:!0})}function gh(e){!e||e.closed||(e.quiesced=!1)}var zr={ctx:null};function bh(e){let t=e.defaultView||(typeof globalThis<"u"?globalThis:null);if(!t)return;let r=function(){let i=zr.ctx;if(i)return bn(i.formRoot),Ve("Reorder",()=>Y(J(i),{path:"",structural:!0},i))};typeof t.hypercmsCommit!="function"&&(t.hypercmsCommit=r),typeof globalThis<"u"&&typeof globalThis.hypercmsCommit!="function"&&(globalThis.hypercmsCommit=r)}var nr="cms";function kh(e){let t=typeof e=="string"?e:"",r=t.indexOf("?"),i=r===-1?t:t.slice(r+1);if(!i)return t;let l=new URLSearchParams(i);return l.get(nr)!=="true"?t:(l.set(nr,"false"),"?"+l.toString())}function vh(e){let t=typeof e=="string"?e:"",r=t.indexOf("?"),i=r===-1?t:t.slice(r+1);return i?new URLSearchParams(i).get(nr)==="true":!1}function yh(){if(typeof window>"u"||!window.location||!window.history||typeof window.history.replaceState!="function")return;let e=window.location.search,t=kh(e);t!==e&&window.history.replaceState(window.history.state,"",t+window.location.hash)}function xh(e){if(rc(e),typeof document<"u"){let t=document.getElementById("hcms-shell-styles");t?.tagName==="LINK"&&t.remove(),Ct(document)}}var ar={sidebar:kc,inline:Yc};function wh(e,t){let r=t!==void 0?t:"cms";if(typeof r!="string")return"sidebar";let i=X.findRules(e,r)?.tagNode?.getAttribute?.("data-cms-view");return!i||i==="sidebar"?"sidebar":ar[i]?i:(console.warn(`hypercms: unknown data-cms-view "${i}" on the rules tag, opening the sidebar`),"sidebar")}function Mn(e={}){return Fr(e)}function Ah(e){let t=e&&e.defaultView;if(!Mr(t))return null;let r=t.clay&&t.clay.setMode;return typeof r!="function"?null:r.call(t.clay,"view")}function Fr(e={},t=!1){let r=q.isOpen?{...q.opts,...e}:e,i=r.pageRoot||(typeof document<"u"?document:null);if(!i)throw new Error("hypercms: no pageRoot available");let l=(i.nodeType===9?i:i.ownerDocument)||(typeof document<"u"?document:null);if(!l)throw new Error("hypercms: no document available");if(!t&&!Nr(l.defaultView))return{ok:!1,code:"not-allowed"};let n=e.view||(q.isOpen?q.ctx.view.name:wh(l,r.rules)),a=ar[n];if(!a)throw new Error(`hypercms: unknown view "${n}" (expected ${Object.keys(ar).join(" or ")})`);let o=q.isOpen?{...r,view:n}:e,s=null,h=null;if(q.isOpen){if(q.ctx.view.name===n)return{ok:!0,view:n};s=q.ctx.previouslyFocused,h=q.ctx.view.name,bt(q.ctx,{restoreFocus:!1,updateUrl:!1,reason:"switch"})}Bc();let d=a({doc:l,pageRoot:i,opts:o}),p=dh({view:d,doc:l,pageRoot:i,opts:o,onCloseRequested:()=>Ah(l)||Nn(),onViewRequested:m=>Fr({view:m},!0)});s&&(p.previouslyFocused=s);try{d.mount(p.initialData),uh(p),d.focusOnOpen(),q.isOpen=!0,q.ctx=p,q.opts=o,p.dispatch("hcms:open",{pageRoot:i,previous:h})}catch(m){throw bt(p,{dispatch:!1,restoreFocus:!!s,updateUrl:!1}),m}return{ok:!0,view:n}}function Nn(){if(!q.isOpen)return{ok:!1,code:"no-session"};let e=q.ctx.doc;if(Mr(e&&e.defaultView)){let t=jn(q.ctx);if(t.length)return{ok:!1,code:"pending-input",pending:t};if(!Nr(e&&e.defaultView))return{ok:!1,code:"busy"}}return bt(q.ctx),{ok:!0}}function zn(){q.isOpen&&q.ctx.view.refresh("api")}function _h(){return q.isOpen&&q.ctx?q.ctx.view.name:null}function Fn({doc:e,pageRoot:t,opts:r={}}={}){let i=null,l=!1;return{ready:Promise.resolve(),enable(){if(l)return;let n={...r};t&&(n.pageRoot=t),n.pageRoot||(n.pageRoot=e&&e.body||e||null),Fr(n,!0),i=q.ctx},quiesce(){return l||!i||q.ctx!==i?{ok:!0}:fh(i)},resume(){l||!i||q.ctx!==i||gh(i)},dispose(){l||(l=!0,i&&q.ctx===i&&bt(i),i=null)}}}function Eh(e={}){return{id:"hypercms",order:1e3,prepare(){return Fn(e)}}}function It(){if(!q.isOpen)throw new Error("hypercms: cms is not open");if(q.ctx.quiesced)throw new Error("hypercms: the CMS is not admitting new changes")}var Sh={getData(){return q.isOpen?J(q.ctx):null},setValue(e,t){It();let r=q.ctx,i=ve(e),l=Be(r.pageRules,i);if(l===void 0)throw new Error(`hypercms: no rule at path "${e}"`);if(typeof l!="string"||l.endsWith("[]"))throw new Error(`hypercms: setValue requires a leaf scalar path; "${e}" is not a leaf`);let n=pn(r.formRoot,e);if(!n)throw new Error(`hypercms: no field element at path "${e}"`);fn(n,t,r.formRoot,e),Y(J(r),{path:e,structural:!1},r)},addItem(e){It(),Tr(e,q.ctx)},removeItem(e){It();let t=q.ctx,r=ve(e);if(typeof r[r.length-1]!="number")throw new Error(`hypercms: removeItem requires an item path; "${e}" is not an array index`);let i=Be(t.pageRules,r.slice(0,-1));if(!(Array.isArray(i)||typeof i=="string"&&i.endsWith("[]")))throw new Error(`hypercms: removeItem requires an item path; parent of "${e}" is not an array`);let l=t.formRoot.querySelector(`[data-hcms-path="${Oh(e)}"]`);if(!l)throw new Error(`hypercms: no element at path "${e}"`);je(l,t)},refresh:zn,_commit(){if(!q.isOpen||q.ctx.quiesced)return;let e=q.ctx;return bn(e.formRoot),Ve("Update",()=>Y(J(e),{path:"",structural:!0},e))}},Ch=250,Th=1e4;function Rh(){typeof window>"u"||typeof document>"u"||vh(window.location?window.location.search:"")&&(q.isOpen||qh(()=>{if(!q.isOpen&&Nr())try{Mn()}catch(e){console.warn("hypercms: auto-open failed",e)}}))}function Pt(){return!!document.body&&!!V("Mutation")}function qh(e){if(Pt()){queueMicrotask(e);return}let t=Date.now()+Th,r=!1,i=null,l=null,n=()=>{r||(r=!0,i!==null&&clearInterval(i),l&&l())};function a(){if(q.isOpen){n();return}Pt()&&(n(),e())}l=Er(document,bs,a),i=setInterval(()=>{if(q.isOpen){n();return}if(Pt()){n(),e();return}Date.now()>=t&&(n(),console.warn("hypercms: ?cms=true auto-open gave up \u2014 no mutation hub appeared. Load clayjs or hyperclayjs (or just the mutation utility) so the CMS can initialize."))},Ch)}Rh();var Dn={open:Mn,close:Nn,refresh:zn,api:Sh,hasRules:(e=document)=>!!X.findRules(e,"cms"),prepareModeLease:Fn,modeParticipant:Eh,get isOpen(){return q.isOpen},currentView:_h,path:bl,scaffold:ur,morphForm:kl};function Oh(e){return typeof CSS<"u"&&CSS.escape?CSS.escape(e):String(e).replace(/[^a-zA-Z0-9_\-.*]/g,t=>"\\"+t)}var $h=`/* GENERATED by scripts/build-theme.js from mirk-interface/mirk.css \u2014 DO NOT EDIT.
   Source of truth: mirk-interface/mirk.css + src/theme/pixel-quiet.overrides.css.
   Regenerate with: npm run build:theme */

/* ===== mirk-interface@2.2.0, scoped to .hcms-shell ===== */
/*
 * mirk.css \u2014 the mirk UI kit, v2.
 * Hand-written, no build. Fourteen form components as semantic BEM classes in
 * @layer components, so utilities (Tailwind or your own) always override them
 * with zero !important. Renders fully standalone; Tailwind is optional.
 *
 * Two hinges (see mirk-ui-guide.md):
 *   1. Components live in @layer components \u2192 utilities win.
 *   2. State serializes into the DOM (native attrs, :has(), inline --mirk-value)
 *      so document.documentElement.outerHTML round-trips every visible state.
 *
 * This @layer statement makes the file self-sufficient without Tailwind, and
 * merges into Tailwind's own order (@layer theme, base, components, utilities)
 * when present.
 */
@layer base, components;

@layer base {
  /* Components are authored border-box (a 2px bevel must not grow the box). */
  .hcms-shell *, .hcms-shell *::before, .hcms-shell *::after { box-sizing: border-box; }
  .hcms-shell button, .hcms-shell input, .hcms-shell optgroup, .hcms-shell select, .hcms-shell textarea { margin: 0; }

  @font-face {
    font-family: 'Departure Mono';
    src: url('https://cdn.jsdelivr.net/npm/mirk-interface@2.2.0/fonts/DepartureMono-1.500/DepartureMono-Regular.woff2') format('woff2');
    font-weight: 400; font-style: normal; font-display: swap;
  }

  .hcms-shell { font-family: 'Departure Mono', ui-monospace, "Menlo", monospace; }
  /* Preflight sets these to ui-monospace; keep them in Departure Mono. */
  .hcms-shell pre, .hcms-shell code, .hcms-shell kbd, .hcms-shell samp { font-family: inherit; }

  /* 28 tokens, one value each. light-dark() picks the side from color-scheme.
     :root paints the DEFAULT theme \u2014 "Pixel Quiet" (see 0030): mirk's warm soul
     with the volume down. The louder original palette is the opt-in
     [data-theme="full-volume"] block below. */
  .hcms-shell {
    color-scheme: light dark;                 /* default: follow the OS */

    --mirk-canvas:        light-dark(#FDF8F0, #0B0C13);
    --mirk-bg:            light-dark(#FDF8F0, #11131E);
    --mirk-fg:            light-dark(#2B241B, #ECEAF2);
    --mirk-accent:        light-dark(#efefe5, #1D1F2F);
    --mirk-destructive:   light-dark(#C24A3A, #ff5566);
    --mirk-focus-color:   light-dark(#C7AE93, #4A506B);
    --mirk-bevel-bg:      light-dark(#FCF8F1, #1A1D2C);
    --mirk-bevel-fg:      light-dark(#2B241B, #ECEAF2);
    --mirk-bevel-tl:      light-dark(#F0E7D8, #2A2E42);
    --mirk-bevel-br:      light-dark(#E2D4BF, #14182A);
    --mirk-bevel-hover-bg: light-dark(#F4ECDF, #202436);
    --mirk-pill-inner-top: light-dark(#FBF6EE, #202436);
    --mirk-input-border:  light-dark(#D8C8AF, #353B52);
    --mirk-placeholder-color: light-dark(#A8987F, #6A7090);
    --mirk-ctrl-bg:       light-dark(#8C7660, #5F6582);
    --mirk-toggle-bg:     light-dark(#EFDBBD, #3E4660);
    --mirk-toggle-hi:     light-dark(#F4EADA, #4E567A);
    --mirk-toggle-lo:     light-dark(#C2A87E, #262B42);
    --mirk-mark-fg:       light-dark(#6B5942, #C9CDE0);
    --mirk-sortable-dot:  light-dark(#DDCBB0, #353B52);
    --mirk-sortable-shadow:    light-dark(#C9B493, #0E1120);
    --mirk-sortable-label:     light-dark(#8C7B62, #8A90AB);
    --mirk-sortable-placeholder: light-dark(#A8987F, #6A7090);
    --mirk-slider-fill:   light-dark(#F2E0BD, #2A2E42);
    --mirk-slider-nub-bg: light-dark(#EFDBBD, #3E4660);
    --mirk-slider-nub-hi: light-dark(#F4EADA, #4E567A);
    --mirk-slider-nub-lo: light-dark(#C2A87E, #262B42);

    /* Chip \u2014 the recovery/notification component reads from the kit's own tokens
       via 5 slim hooks, so it matches the kit by default and reskins by overriding
       a hook (not a rule). Each follows the theme (incl. the Full Volume variant)
       and OS light/dark with no per-theme repaint; the one exception is the primary
       fill, which the default (Pixel Quiet) leaves as the theme's fg ink while the
       Full Volume variant tints it a warm brown in light (see below). */
    --mirk-chip-surface:     var(--mirk-bg);            /* raised panel face */
    --mirk-chip-edge:        var(--mirk-input-border);  /* panel + recess outline */
    --mirk-chip-primary-bg:  var(--mirk-fg);            /* primary action fill \u2014 the theme's fg ink (Full Volume tints it warm brown in light) */
    --mirk-chip-primary-fg:  var(--mirk-bg);
    --mirk-chip-alert:       var(--mirk-destructive);   /* icon + struck "now" value */

    --mirk-radius: 5px;                        /* the "rounded" corner */
    --mirk-focus-offset: 2px;                  /* non-color \u2192 can't ride light-dark() */

    background: var(--mirk-canvas);
    color: var(--mirk-fg);
  }

  /* The one non-color token with a real light/dark split (was 2px / 3px). */
  @media (prefers-color-scheme: dark) { .hcms-shell { --mirk-focus-offset: 3px; } }

  /* Force a mode on any subtree with one attribute (class aliases for hosts that
     prefer class-based theming and Tailwind's dark-variant convention). Each also
     paints its own canvas so a wrapper visibly flips. */
  .hcms-shell[data-theme="light"], .hcms-shell.light {
    color-scheme: light; --mirk-focus-offset: 2px;
    background: var(--mirk-canvas); color: var(--mirk-fg);
  }
  .hcms-shell[data-theme="dark"], .hcms-shell.dark {
    color-scheme: dark; --mirk-focus-offset: 3px;
    background: var(--mirk-canvas); color: var(--mirk-fg);
  }

  /* Built-in brand variant \u2014 "Full Volume": mirk's original full-strength palette,
     the loud pole of the volume axis (Pixel Quiet, now the default, is the quiet
     end). Full-contrast bevel, warm cream / deep navy, crimson destructive.
     Authored with light-dark() like :root, so it follows the OS and still flips
     with .dark / .light. Opt in: data-theme="full-volume". Sits after :root
     (equal specificity, source order wins). --mirk-radius / --mirk-focus-offset /
     --mirk-ctrl-bg and the four shared --mirk-chip-* hooks inherit from :root
     unchanged; only the tokens that differ from the default are re-declared here. */
  .hcms-shell[data-theme="full-volume"] {
    --mirk-canvas:        light-dark(#F7F2EA, #0B0C13);
    --mirk-bg:            light-dark(#F7F2EA, #1D1F2F);
    --mirk-fg:            light-dark(#15120e, #F6F7F9);
    --mirk-accent:        light-dark(#efefe5, #232639);
    --mirk-destructive:   light-dark(#d4183d, #ff5566);
    --mirk-focus-color:   light-dark(#BBA288, #5A607F);
    --mirk-bevel-bg:      light-dark(#e9d3bd, #1D1F2F);
    --mirk-bevel-fg:      light-dark(#15120e, #F6F7F9);
    --mirk-bevel-tl:      light-dark(#f3ddc7, #474C65);
    --mirk-bevel-br:      light-dark(#c2ad95, #131725);
    --mirk-bevel-hover-bg: light-dark(#dfc9b3, #232639);
    --mirk-pill-inner-top: light-dark(#efdac7, #232639);
    --mirk-input-border:  light-dark(#957E65, #6E738E);
    --mirk-placeholder-color: light-dark(#7F7366, #545973);
    --mirk-toggle-bg:     light-dark(#DFC9AF, #656D95);
    --mirk-toggle-hi:     light-dark(#E9D6C3, #7F87AD);
    --mirk-toggle-lo:     light-dark(#C7A88A, #505677);
    --mirk-mark-fg:       light-dark(#3F3225, #E1E3EA);
    --mirk-sortable-dot:  light-dark(#e2c5a6, #393f5b);
    --mirk-sortable-shadow:    light-dark(#c7a47f, #111527);
    --mirk-sortable-label:     light-dark(#231e18, #edeef2);
    --mirk-sortable-placeholder: light-dark(#99826c, #6f7695);
    --mirk-slider-fill:   light-dark(#e9d3bd, #232639);
    --mirk-slider-nub-bg: light-dark(#DFC9AF, #656D95);
    --mirk-slider-nub-hi: light-dark(#E9D6C3, #7F87AD);
    --mirk-slider-nub-lo: light-dark(#C7A88A, #505677);
    /* :root (Pixel Quiet) leaves the chip primary as its own fg ink; the original
       default tinted it a warm brown in light \u2014 restore that here. */
    --mirk-chip-primary-bg: light-dark(#1C170E, var(--mirk-fg));
  }

  /* Roll your own the same way \u2014 an explicit [data-theme] block is the escape hatch:
     [data-theme="sunset"] { color-scheme: light; --mirk-accent: #f0a868; \u2026 } */
}

@layer components {
  /* Visually hidden, still focusable/announced. The hidden native input behind
     every custom control relies on it; Tailwind is optional now. */
  .hcms-shell .mirk-sr-only {
    position: absolute; width: 1px; height: 1px;
    padding: 0; margin: -1px; overflow: hidden;
    clip: rect(0, 0, 0, 0); white-space: nowrap; border-width: 0;
  }

  /* ============================ BUTTON ============================ */
  .hcms-shell .mirk-button {
    display: inline-flex; align-items: center; justify-content: center; gap: 0.5rem;
    font: inherit; line-height: 1.5; cursor: pointer; user-select: none;
    text-align: center;
    color: var(--mirk-bevel-fg);
    background: var(--mirk-bevel-bg);
    border: 2px solid;
    /* Raised bevel: light top+left, dark right+bottom. Shorthand is T R B L. */
    border-color: var(--mirk-bevel-tl) var(--mirk-bevel-br) var(--mirk-bevel-br) var(--mirk-bevel-tl);
    padding: 4px 14px 5px;                     /* medium */
    outline: none;
  }
  .hcms-shell .mirk-button__label { white-space: nowrap; user-select: none; display: inline-block; }

  /* States, written once, shared by every size and shape. */
  .hcms-shell .mirk-button:hover { background-color: var(--mirk-bevel-hover-bg); }
  .hcms-shell .mirk-button:active {
    border-color: var(--mirk-bevel-br) var(--mirk-bevel-tl) var(--mirk-bevel-tl) var(--mirk-bevel-br);
  }
  .hcms-shell .mirk-button:not(.mirk-button--round):active .mirk-button__label { translate: 1.5px 1.5px; }
  /* Direct focus (a real <button>) or a focus-visible descendant (a <label>
     wrapping a hidden input, as the file/image upload triggers do). */
  .hcms-shell .mirk-button:focus-visible, .hcms-shell .mirk-button:has(:focus-visible) {
    outline: 1px solid var(--mirk-focus-color); outline-offset: var(--mirk-focus-offset);
  }
  .hcms-shell .mirk-button:disabled { opacity: 0.5; cursor: not-allowed; }

  /* Sizes set padding + font (rect); round re-homes padding to the label below. */
  .hcms-shell .mirk-button--small { padding: 3px 12px; font-size: 14px; }
  .hcms-shell .mirk-button--large { padding: 4px 17px 7px; font-size: 18px; border-width: 3px; }

  /* Round register: a gradient pill frame with the label as the inner fill. */
  .hcms-shell .mirk-button--round {
    border: none; padding: 2px; border-radius: 14px;
    background-color: var(--mirk-canvas);
    background-image: linear-gradient(to top in oklab, var(--mirk-bevel-br), var(--mirk-bevel-tl));
    opacity: 0.9; transition: opacity 0.15s cubic-bezier(0.4, 0, 0.2, 1);
  }
  .hcms-shell .mirk-button--round:hover { opacity: 1; }
  .hcms-shell .mirk-button--round:active {
    background-image: linear-gradient(to bottom in oklab, var(--mirk-bevel-br), var(--mirk-bevel-tl));
  }
  .hcms-shell .mirk-button--round .mirk-button__label {
    display: flex; align-items: center; gap: 0.5rem;
    padding: 4px 17px 6px; border-radius: 12px;
    color: var(--mirk-bevel-fg);
    background-color: var(--mirk-bevel-bg);
    background-image: linear-gradient(to top in oklab, var(--mirk-bevel-bg), var(--mirk-pill-inner-top));
  }
  .hcms-shell .mirk-button--round.mirk-button--small { border-radius: 12px; }
  .hcms-shell .mirk-button--round.mirk-button--small .mirk-button__label { padding: 2px 14px; border-radius: 10px; }
  .hcms-shell .mirk-button--round.mirk-button--large { border-radius: 16px; }
  .hcms-shell .mirk-button--round.mirk-button--large .mirk-button__label { padding: 7px 24px 9px; border-radius: 14px; }

  /* Quiet: a borderless text button (transparent border keeps the hit area + the
     baseline aligned with neighbouring bevel buttons). For tertiary actions. */
  .hcms-shell .mirk-button--quiet {
    background: none; background-image: none;
    border-color: transparent; color: var(--mirk-placeholder-color);
  }
  .hcms-shell .mirk-button--quiet:hover { background: none; color: var(--mirk-fg); }
  .hcms-shell .mirk-button--quiet:active { border-color: transparent; }

  /* ============================ TEXT INPUT ============================ */
  .hcms-shell .mirk-input {
    width: 100%;
    background: var(--mirk-bevel-bg); color: var(--mirk-bevel-fg);
    border: 1px solid var(--mirk-input-border);
    padding: 5px 14px 6px;                     /* medium */
    font: inherit; line-height: 1.5; border-radius: 0; outline: none;
  }
  .hcms-shell .mirk-input::placeholder { color: var(--mirk-placeholder-color); }
  .hcms-shell .mirk-input:focus-visible {
    outline: 1px solid var(--mirk-focus-color); outline-offset: var(--mirk-focus-offset);
  }
  .hcms-shell .mirk-input--small { padding: 4px 12px; font-size: 14px; }
  .hcms-shell .mirk-input--large { padding: 6px 17px 9px; font-size: 18px; }
  .hcms-shell .mirk-input--rounded { border-radius: var(--mirk-radius); }

  /* ============================ TEXTAREA ============================ */
  .hcms-shell .mirk-textarea {
    width: 100%;
    background: var(--mirk-bevel-bg); color: var(--mirk-bevel-fg);
    border: 1px solid var(--mirk-input-border);
    padding: 6px 17px 9px; font: inherit; font-size: 18px; line-height: 1.5;
    border-radius: 0; outline: none; resize: vertical;
  }
  .hcms-shell .mirk-textarea::placeholder { color: var(--mirk-placeholder-color); }
  .hcms-shell .mirk-textarea:focus-visible {
    outline: 1px solid var(--mirk-focus-color); outline-offset: var(--mirk-focus-offset);
  }
  .hcms-shell .mirk-textarea--rounded { border-radius: var(--mirk-radius); }

  /* ============================ NUMBER ============================ */
  .hcms-shell .mirk-number {
    display: flex; align-items: stretch; width: 100%;
    background: var(--mirk-bevel-bg);
    border: 1px solid var(--mirk-input-border); border-radius: 0;
  }
  .hcms-shell .mirk-number:has(:focus-visible) {
    outline: 1px solid var(--mirk-focus-color); outline-offset: var(--mirk-focus-offset);
  }
  .hcms-shell .mirk-number__input {
    flex: 1; min-width: 0; background: transparent; color: var(--mirk-bevel-fg);
    padding: 5px 10px 6px 14px;                /* medium */
    font: inherit; line-height: 1.5; outline: none;
    appearance: textfield; -webkit-appearance: textfield;
  }
  .hcms-shell .mirk-number__input::-webkit-outer-spin-button, .hcms-shell .mirk-number__input::-webkit-inner-spin-button { -webkit-appearance: none; appearance: none; }
  .hcms-shell .mirk-number__steps { display: flex; flex-direction: column; padding: 2px; gap: 2px; }
  .hcms-shell .mirk-number__step {
    flex: 1; cursor: pointer; line-height: 1; padding: 0 10px; font-size: 9px;  /* medium */
    display: flex; align-items: center; justify-content: center;
    background: var(--mirk-bevel-bg); outline: none;
    border: 2px solid;
    border-color: var(--mirk-bevel-tl) var(--mirk-bevel-br) var(--mirk-bevel-br) var(--mirk-bevel-tl);
  }
  .hcms-shell .mirk-number__step:hover { background: var(--mirk-bevel-hover-bg); }
  .hcms-shell .mirk-number__step:active {
    border-color: var(--mirk-bevel-br) var(--mirk-bevel-tl) var(--mirk-bevel-tl) var(--mirk-bevel-br);
  }
  .hcms-shell .mirk-number__step:focus-visible { outline: 1px solid var(--mirk-focus-color); outline-offset: 1px; }

  .hcms-shell .mirk-number--small .mirk-number__input { padding: 4px 8px 4px 12px; font-size: 14px; }
  .hcms-shell .mirk-number--small .mirk-number__step { padding: 0 8px; font-size: 8px; }
  .hcms-shell .mirk-number--large .mirk-number__input { padding: 6px 12px 9px 17px; font-size: 18px; }
  .hcms-shell .mirk-number--large .mirk-number__step { padding: 0 12px; font-size: 10px; }

  .hcms-shell .mirk-number--rounded { border-radius: var(--mirk-radius); }
  .hcms-shell .mirk-number--rounded .mirk-number__step { border-radius: 3px; }

  /* ============================ SELECT / DROPDOWN ============================ */
  /* Keeps appearance:none + a real chevron (renders identically everywhere today);
     base-select/::picker is a future enhancement. */
  .hcms-shell .mirk-select { position: relative; }
  .hcms-shell .mirk-select__field {
    width: 100%; appearance: none; -webkit-appearance: none;
    background: var(--mirk-bevel-bg); color: var(--mirk-bevel-fg);
    border: 2px solid;
    border-color: var(--mirk-bevel-tl) var(--mirk-bevel-br) var(--mirk-bevel-br) var(--mirk-bevel-tl);
    padding: 4px 40px 5px 14px;                /* medium */
    font: inherit; line-height: 1.5; border-radius: 0; outline: none;
  }
  .hcms-shell .mirk-select__field:focus-visible {
    outline: 1px solid var(--mirk-focus-color); outline-offset: var(--mirk-focus-offset);
  }
  .hcms-shell .mirk-select__chevron {
    pointer-events: none; position: absolute; top: 50%; right: 14px;  /* medium */
    translate: 0 -50%; rotate: 90deg; display: inline-block; line-height: 1; font-size: 20px;
  }
  .hcms-shell .mirk-select--small .mirk-select__field { padding: 3px 36px 3px 12px; font-size: 14px; }
  .hcms-shell .mirk-select--small .mirk-select__chevron { right: 12px; font-size: 18px; }
  .hcms-shell .mirk-select--large .mirk-select__field { padding: 4px 48px 7px 17px; font-size: 18px; border-width: 3px; }
  .hcms-shell .mirk-select--large .mirk-select__chevron { right: 16px; font-size: 24px; }

  /* Round: gradient pill frame around a borderless, pill-filled select. */
  .hcms-shell .mirk-select--round .mirk-select__frame {
    padding: 2px; border-radius: 14px;         /* medium */
    background-color: var(--mirk-canvas);
    background-image: linear-gradient(to top in oklab, var(--mirk-bevel-br), var(--mirk-bevel-tl));
  }
  .hcms-shell .mirk-select--round .mirk-select__frame:has(:focus-visible) {
    outline: 1px solid var(--mirk-focus-color); outline-offset: var(--mirk-focus-offset);
  }
  .hcms-shell .mirk-select--round .mirk-select__field {
    border: none; background-color: transparent;
    background-image: linear-gradient(to top in oklab, var(--mirk-bevel-bg), var(--mirk-pill-inner-top));
    border-radius: 12px; padding: 4px 40px 6px 17px;   /* medium */
  }
  .hcms-shell .mirk-select--round.mirk-select--small .mirk-select__frame { border-radius: 12px; }
  .hcms-shell .mirk-select--round.mirk-select--small .mirk-select__field { border-radius: 10px; padding: 2px 36px 2px 14px; }
  .hcms-shell .mirk-select--round.mirk-select--large .mirk-select__frame { border-radius: 16px; }
  .hcms-shell .mirk-select--round.mirk-select--large .mirk-select__field { border-radius: 14px; padding: 7px 48px 9px 24px; }
  .hcms-shell .mirk-select--round.mirk-select--large .mirk-select__chevron { right: 16px; font-size: 24px; }

  /* ============================ CHECKBOX ============================ */
  .hcms-shell .mirk-checkbox { display: inline-flex; align-items: center; gap: 0.75rem; cursor: pointer; width: fit-content; }
  .hcms-shell .mirk-checkbox__box {
    position: relative; flex-shrink: 0; width: 22px; height: 22px;
    display: flex; align-items: center; justify-content: center; border-radius: 0;
    background: var(--mirk-bevel-bg);
    border: 2px solid;
    border-color: var(--mirk-bevel-tl) var(--mirk-bevel-br) var(--mirk-bevel-br) var(--mirk-bevel-tl);
  }
  .hcms-shell .mirk-checkbox__mark {
    opacity: 0; display: block; width: 6px; height: 12px;
    border-right: 2.5px solid var(--mirk-mark-fg); border-bottom: 2.5px solid var(--mirk-mark-fg);
    rotate: 45deg; translate: 0.5px -1.5px;
  }
  .hcms-shell .mirk-checkbox__label { font-size: 18px; line-height: 1.5; }

  .hcms-shell .mirk-checkbox:has(:checked) .mirk-checkbox__box { border-color: var(--mirk-input-border); }
  .hcms-shell .mirk-checkbox:has(:checked) .mirk-checkbox__mark { opacity: 1; }
  .hcms-shell .mirk-checkbox:has(:focus-visible) .mirk-checkbox__box {
    outline: 1px solid var(--mirk-focus-color); outline-offset: var(--mirk-focus-offset);
  }

  .hcms-shell .mirk-checkbox--small { gap: 0.5rem; }
  .hcms-shell .mirk-checkbox--small .mirk-checkbox__box { width: 18px; height: 18px; }
  .hcms-shell .mirk-checkbox--small .mirk-checkbox__mark {
    width: 5px; height: 10px;
    border-right-width: 2px; border-bottom-width: 2px; translate: 0.5px -1px;
  }
  .hcms-shell .mirk-checkbox--small .mirk-checkbox__label { font-size: 14px; }

  /* ============================ RADIO ============================ */
  .hcms-shell .mirk-radio { display: inline-flex; align-items: center; gap: 0.75rem; cursor: pointer; width: fit-content; }
  .hcms-shell .mirk-radio__ring {
    position: relative; flex-shrink: 0; width: 25px; height: 25px; border-radius: 9999px;
    background-image: linear-gradient(to top in oklab, var(--mirk-bevel-br), var(--mirk-bevel-tl));
  }
  .hcms-shell .mirk-radio__fill {
    display: block; position: absolute; inset: 2px; border-radius: 9999px;
    background-image: linear-gradient(to top in oklab, var(--mirk-bevel-bg), var(--mirk-pill-inner-top));
  }
  .hcms-shell .mirk-radio__dot {
    display: none; position: absolute; top: 50%; left: 50%; translate: -50% -50%;
    width: 9px; height: 9px; border-radius: 9999px; background: var(--mirk-mark-fg);
  }
  .hcms-shell .mirk-radio__label { font-size: 18px; line-height: 1.5; }

  .hcms-shell .mirk-radio:has(:checked) .mirk-radio__ring {
    background-image: none; background-color: var(--mirk-bevel-bg);
    border: 2px solid var(--mirk-input-border);
  }
  .hcms-shell .mirk-radio:has(:checked) .mirk-radio__fill { display: none; }
  .hcms-shell .mirk-radio:has(:checked) .mirk-radio__dot { display: block; }
  .hcms-shell .mirk-radio:has(:focus-visible) .mirk-radio__ring {
    outline: 1px solid var(--mirk-focus-color); outline-offset: var(--mirk-focus-offset);
  }

  .hcms-shell .mirk-radio--small { gap: 0.5rem; }
  .hcms-shell .mirk-radio--small .mirk-radio__ring { width: 20px; height: 20px; }
  .hcms-shell .mirk-radio--small .mirk-radio__dot { width: 7px; height: 7px; }
  .hcms-shell .mirk-radio--small .mirk-radio__label { font-size: 14px; }

  /* ============================ TOGGLE ============================ */
  .hcms-shell .mirk-toggle { display: inline-flex; align-items: center; gap: 0.75rem; cursor: pointer; width: fit-content; }
  .hcms-shell .mirk-toggle__track {
    position: relative; flex-shrink: 0; width: 49px; height: 27px; border-radius: 0;
    background: var(--mirk-canvas);            /* own recessed channel, like the slider track \u2014 never the host page (0033) */
    border: 1px solid var(--mirk-input-border);
  }
  .hcms-shell .mirk-toggle__thumb {
    position: absolute; top: 3px; left: 3px; width: 19px; height: 19px;
    background-color: var(--mirk-toggle-bg);
    border: 2px solid;
    border-color: var(--mirk-toggle-hi) var(--mirk-toggle-lo) var(--mirk-toggle-lo) var(--mirk-toggle-hi);
    transition-property: transform, translate, scale, rotate;
    transition-duration: 0.15s; transition-timing-function: cubic-bezier(0.4, 0, 0.2, 1);
  }
  .hcms-shell .mirk-toggle__label { font-size: 18px; line-height: 1.5; }

  .hcms-shell .mirk-toggle:has(:checked) .mirk-toggle__thumb { translate: 22px; }
  .hcms-shell .mirk-toggle:has(:focus-visible) .mirk-toggle__track {
    outline: 1px solid var(--mirk-focus-color); outline-offset: var(--mirk-focus-offset);
  }

  .hcms-shell .mirk-toggle--round .mirk-toggle__track { width: 50px; height: 29px; border-radius: 9999px; }
  .hcms-shell .mirk-toggle--round .mirk-toggle__thumb {
    width: 21px; height: 21px; border: none; border-radius: 9999px;
    background-color: transparent;
    background-image: linear-gradient(to top in oklab, var(--mirk-toggle-lo), var(--mirk-toggle-hi));
  }
  .hcms-shell .mirk-toggle--round .mirk-toggle__thumb::after {
    content: ""; position: absolute; inset: 2px; border-radius: 9999px; background: var(--mirk-toggle-bg);
  }
  .hcms-shell .mirk-toggle--round:has(:checked) .mirk-toggle__thumb { translate: 21px; }

  .hcms-shell .mirk-toggle--small { gap: 0.5rem; }
  .hcms-shell .mirk-toggle--small .mirk-toggle__track { width: 42px; height: 23px; }
  .hcms-shell .mirk-toggle--small .mirk-toggle__thumb { top: 2px; left: 2px; width: 17px; height: 17px; }
  .hcms-shell .mirk-toggle--small:has(:checked) .mirk-toggle__thumb { translate: 19px; }
  .hcms-shell .mirk-toggle--small .mirk-toggle__label { font-size: 14px; }
  .hcms-shell .mirk-toggle--round.mirk-toggle--small .mirk-toggle__track { width: 43px; height: 25px; }
  .hcms-shell .mirk-toggle--round.mirk-toggle--small .mirk-toggle__thumb { width: 19px; height: 19px; }
  .hcms-shell .mirk-toggle--round.mirk-toggle--small:has(:checked) .mirk-toggle__thumb { translate: 18px; }

  /* ============================ SLIDER ============================ */
  .hcms-shell .mirk-slider { position: relative; height: 32px; width: 100%; --mirk-value: 0%; }
  .hcms-shell .mirk-slider__input {
    position: absolute; inset: 0; width: 100%; height: 100%;
    opacity: 0; cursor: pointer; z-index: 10;
  }
  .hcms-shell .mirk-slider__track {
    position: absolute; left: 0; right: 0; top: 50%; translate: 0 -50%; height: 12px;
    background: var(--mirk-canvas); border: 1px solid var(--mirk-input-border); overflow: hidden;
  }
  .hcms-shell .mirk-slider__fill { height: 100%; width: var(--mirk-value); background: var(--mirk-slider-fill); }
  .hcms-shell .mirk-slider__nub {
    position: absolute; top: 50%; left: var(--mirk-value); translate: -50% -50%;
    width: 21px; height: 21px; pointer-events: none;
    background-color: var(--mirk-slider-nub-bg);
    border: 2px solid;
    border-color: var(--mirk-slider-nub-hi) var(--mirk-slider-nub-lo) var(--mirk-slider-nub-lo) var(--mirk-slider-nub-hi);
  }
  .hcms-shell .mirk-slider__input:focus-visible ~ .mirk-slider__nub {
    outline: 1px solid var(--mirk-focus-color); outline-offset: var(--mirk-focus-offset);
  }

  .hcms-shell .mirk-slider--round .mirk-slider__track { height: 10px; border-radius: 9999px; }
  .hcms-shell .mirk-slider--round .mirk-slider__nub {
    width: 24px; height: 24px; border: none; border-radius: 9999px;
    background-color: transparent;
    background-image: linear-gradient(to top in oklab, var(--mirk-slider-nub-lo), var(--mirk-slider-nub-hi));
  }
  .hcms-shell .mirk-slider--round .mirk-slider__nub::after {
    content: ""; position: absolute; inset: 2px; border-radius: 9999px; background: var(--mirk-slider-nub-bg);
  }

  .hcms-shell .mirk-slider--small { height: 24px; }
  .hcms-shell .mirk-slider--small .mirk-slider__track { height: 8px; }
  .hcms-shell .mirk-slider--small .mirk-slider__nub { width: 16px; height: 16px; }
  .hcms-shell .mirk-slider--round.mirk-slider--small .mirk-slider__track { height: 7px; }
  .hcms-shell .mirk-slider--round.mirk-slider--small .mirk-slider__nub { width: 18px; height: 18px; }

  /* ============================ DATE ============================ */
  .hcms-shell .mirk-date { position: relative; }
  .hcms-shell .mirk-date__field {
    width: 100%; background: var(--mirk-bevel-bg); color: var(--mirk-bevel-fg);
    border: 1px solid var(--mirk-input-border);
    padding: 6px 44px 9px 17px; font: inherit; font-size: 18px; line-height: 1.5;
    border-radius: 0; outline: none;
  }
  .hcms-shell .mirk-date__field:focus-visible {
    outline: 1px solid var(--mirk-focus-color); outline-offset: var(--mirk-focus-offset);
  }
  .hcms-shell .mirk-date__field::-webkit-calendar-picker-indicator {
    opacity: 0; position: absolute; right: 0; top: 0; bottom: 0; width: 44px; margin: 0; cursor: pointer;
  }
  .hcms-shell .mirk-date__field::-webkit-inner-spin-button { -webkit-appearance: none; appearance: none; }
  .hcms-shell .mirk-date__field::-webkit-clear-button { -webkit-appearance: none; appearance: none; }
  .hcms-shell .mirk-date__icon {
    pointer-events: none; position: absolute; right: 16px; top: 50%; translate: 0 -50%;
  }
  .hcms-shell .mirk-date--rounded .mirk-date__field { border-radius: var(--mirk-radius); }

  .hcms-shell .mirk-date--small .mirk-date__field { padding: 4px 36px 4px 12px; font-size: 14px; }
  .hcms-shell .mirk-date--small .mirk-date__field::-webkit-calendar-picker-indicator { width: 36px; }
  .hcms-shell .mirk-date--small .mirk-date__icon { right: 12px; }

  /* The native file/image inputs are visually hidden; their styled label drives
     them, and the focus ring rides :has(:focus-visible) on button or container. */
  .hcms-shell .mirk-file__input, .hcms-shell .mirk-image__input {
    position: absolute; width: 1px; height: 1px;
    padding: 0; margin: -1px; overflow: hidden;
    clip: rect(0, 0, 0, 0); white-space: nowrap; border-width: 0;
  }

  /* ============================ FILE ============================ */
  .hcms-shell .mirk-file { display: flex; align-items: center; gap: 0.75rem; width: 100%; }
  .hcms-shell .mirk-file__name {
    color: var(--mirk-placeholder-color); font-size: 18px; line-height: 1.5;
    min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap;
  }
  .hcms-shell .mirk-file__name[data-filled] { color: var(--mirk-bevel-fg); }

  /* Compact: a shared bordered container holds a smaller button + the name. */
  .hcms-shell .mirk-file--compact {
    padding: 4px 8px; background: var(--mirk-bevel-bg);
    border: 1px solid var(--mirk-input-border); border-radius: 0;
  }
  .hcms-shell .mirk-file--compact:has(:focus-visible) {
    outline: 1px solid var(--mirk-focus-color); outline-offset: var(--mirk-focus-offset);
  }
  /* The upload trigger is a .mirk-button; nudge it onto the container's left
     border so the bevel sits flush (covers compact and round-compact). */
  .hcms-shell .mirk-file--compact .mirk-button { margin-left: -1px; }
  .hcms-shell .mirk-file--compact .mirk-file__name { font-size: 16px; }
  .hcms-shell .mirk-file--compact.mirk-file--round { border-radius: 15px; }

  /* Filled: the name slot becomes a link to the chosen file, beside a circular \xD7
     to clear it: a 1px ring over a bevel fill that turns destructive on hover.
     Empty keeps the placeholder span. */
  .hcms-shell a.mirk-file__name { text-decoration: underline; text-underline-offset: 2px; }
  .hcms-shell .mirk-file__remove {
    appearance: none; -webkit-appearance: none; flex-shrink: 0;
    width: 18px; height: 18px; border-radius: 50%;
    display: inline-flex; align-items: center; justify-content: center;
    margin: 0; padding: 0; cursor: pointer; line-height: 0;
    color: var(--mirk-bevel-fg); background: var(--mirk-bevel-bg);
    border: 1px solid var(--mirk-input-border);
  }
  .hcms-shell .mirk-file__remove svg { display: block; width: 66%; height: 66%; }
  .hcms-shell .mirk-file__remove:hover { border-color: var(--mirk-destructive); color: var(--mirk-destructive); }

  /* Small: tighter name + gap + remove \xD7; pair the trigger with mirk-button--small.
     Composes with --compact (densest) and --round. */
  .hcms-shell .mirk-file--small { gap: 0.5rem; }
  .hcms-shell .mirk-file--small .mirk-file__name { font-size: 14px; }
  .hcms-shell .mirk-file--small .mirk-file__remove { width: 16px; height: 16px; }
  .hcms-shell .mirk-file--small.mirk-file--compact { padding: 3px 8px; }
  .hcms-shell .mirk-file--small.mirk-file--compact .mirk-file__name { font-size: 13px; }

  /* ============================ IMAGE ============================ */
  .hcms-shell .mirk-image { display: flex; flex-direction: column; gap: 0.5rem; }
  .hcms-shell .mirk-image__frame {
    position: relative; width: 120px; height: 120px; overflow: hidden; border-radius: 0;
    display: flex; align-items: center; justify-content: center;
    background: var(--mirk-bevel-bg); border: 1px solid var(--mirk-input-border);
    color: var(--mirk-placeholder-color); font-size: 14px; line-height: 1.5;
  }
  .hcms-shell .mirk-image__preview { position: absolute; inset: 0; width: 100%; height: 100%; object-fit: cover; }
  .hcms-shell .mirk-image--rounded .mirk-image__frame { border-radius: var(--mirk-radius); }
  /* The upload trigger is a .mirk-button; keep it hugging its label instead of
     stretching to fill this column-flex container. */
  .hcms-shell .mirk-image .mirk-button { width: fit-content; }

  /* Compact: a focused 56px thumbnail upload. Empty shows a small upload button;
     once an image is chosen the button hides and a thumbnail + corner \xD7 takes its
     place. The frame clips the image (overflow hidden) while the thumb wrapper
     stays visible, so the \xD7 can sit just outside the corner without being cut. */
  .hcms-shell .mirk-image--compact { flex-direction: row; align-items: center; gap: 0; }
  .hcms-shell .mirk-image__thumb { position: relative; display: inline-block; width: fit-content; margin: 0; line-height: 0; }
  .hcms-shell .mirk-image--compact .mirk-image__frame {
    width: 56px; height: 56px; overflow: hidden; border-radius: 0;
    border: 1px solid var(--mirk-input-border);
  }
  .hcms-shell .mirk-image--compact.mirk-image--rounded .mirk-image__frame { border-radius: var(--mirk-radius); }
  .hcms-shell .mirk-image--compact .mirk-image__preview {
    position: static; inset: auto; width: 100%; height: 100%; display: block; object-fit: cover;
  }
  .hcms-shell .mirk-image__remove {
    position: absolute; top: -7px; right: -7px;
    width: 18px; height: 18px; border-radius: 50%; padding: 0;
    display: inline-flex; align-items: center; justify-content: center;
    appearance: none; -webkit-appearance: none; cursor: pointer; line-height: 0;
    color: var(--mirk-bevel-fg); background: var(--mirk-bevel-bg);
    border: 1px solid var(--mirk-input-border);
  }
  .hcms-shell .mirk-image__remove svg { display: block; width: 10px; height: 10px; }
  .hcms-shell .mirk-image__remove:hover { color: var(--mirk-destructive); border-color: var(--mirk-destructive); }

  /* ============================ TAGS ============================ */
  .hcms-shell .mirk-tags {
    display: flex; flex-wrap: wrap; align-items: center; gap: 0.5rem; padding: 0.5rem;
    background: var(--mirk-bevel-bg); border: 1px solid var(--mirk-input-border);
    border-radius: 0; cursor: text;
  }
  .hcms-shell .mirk-tags:has(:focus-visible) {
    outline: 1px solid var(--mirk-focus-color); outline-offset: var(--mirk-focus-offset);
  }
  .hcms-shell .mirk-tags__chip {
    display: inline-flex; align-items: center; gap: 0.5rem;
    padding: 2px 8px 2px 12px; font-size: 14px; line-height: 1.5;
    color: var(--mirk-bevel-fg); background: var(--mirk-bevel-bg);
    border: 2px solid;
    border-color: var(--mirk-bevel-tl) var(--mirk-bevel-br) var(--mirk-bevel-br) var(--mirk-bevel-tl);
  }
  .hcms-shell .mirk-tags__remove {
    appearance: none; -webkit-appearance: none;
    margin: 0; padding: 0; border: 0; background: none; color: inherit;
    cursor: pointer; font-size: 14px; line-height: 1;
  }
  .hcms-shell .mirk-tags__remove:hover { color: var(--mirk-destructive); }
  .hcms-shell .mirk-tags__input {
    appearance: none; -webkit-appearance: none;
    border: 0; padding: 0;
    flex: 1; min-width: 120px; background: transparent; color: var(--mirk-bevel-fg);
    outline: none; font-size: 18px; line-height: 1.5;
  }
  .hcms-shell .mirk-tags__input::placeholder { color: var(--mirk-placeholder-color); }

  .hcms-shell .mirk-tags--round { border-radius: 15px; }
  .hcms-shell .mirk-tags--round .mirk-tags__chip {
    padding: 2px; border: none; border-radius: 12px;
    background-color: transparent;
    background-image: linear-gradient(to top in oklab, var(--mirk-bevel-br), var(--mirk-bevel-tl));
  }
  .hcms-shell .mirk-tags__chip-inner {
    display: inline-flex; align-items: center; gap: 0.5rem;
    padding: 1px 8px 1px 12px; border-radius: 10px;
    color: var(--mirk-bevel-fg); background-color: var(--mirk-bevel-bg);
    background-image: linear-gradient(to top in oklab, var(--mirk-bevel-bg), var(--mirk-pill-inner-top));
  }

  .hcms-shell .mirk-tags--small { gap: 0.375rem; padding: 0.375rem; }
  .hcms-shell .mirk-tags--small .mirk-tags__chip { padding: 1px 6px 1px 10px; font-size: 12px; }
  .hcms-shell .mirk-tags--small .mirk-tags__remove { font-size: 12px; }
  .hcms-shell .mirk-tags--small .mirk-tags__input { font-size: 14px; min-width: 90px; }
  .hcms-shell .mirk-tags--small.mirk-tags--round { border-radius: 12px; }
  .hcms-shell .mirk-tags--small.mirk-tags--round .mirk-tags__chip-inner { padding: 1px 6px 1px 10px; }

  /* ============================ SORTABLE ============================ */
  .hcms-shell .mirk-sortable { display: flex; flex-direction: column; gap: 0.5rem; }
  .hcms-shell .mirk-sortable__item {
    display: flex; flex-direction: row; width: 100%;
    background: var(--mirk-bevel-bg); border: 1px solid var(--mirk-input-border);
  }
  .hcms-shell .mirk-sortable__grip {
    width: 28px; flex-shrink: 0; display: flex; align-items: center; justify-content: center;
    cursor: grab; border-right: 1px solid var(--mirk-input-border);
  }
  .hcms-shell .mirk-sortable__grip:active { cursor: grabbing; }
  .hcms-shell .mirk-sortable__dots { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 3px; }
  .hcms-shell .mirk-sortable__dot {
    display: block; width: 4px; height: 4px; background: var(--mirk-sortable-dot);
    box-shadow: 1px 0 0 0 var(--mirk-sortable-shadow), 0 1px 0 0 var(--mirk-sortable-shadow), 1px 1px 0 0 var(--mirk-sortable-shadow);
  }
  .hcms-shell .mirk-sortable__body { display: flex; flex-direction: column; flex: 1; min-width: 0; }
  .hcms-shell .mirk-sortable__row { padding: 8px 17px 9px; }
  .hcms-shell .mirk-sortable__row:not(:last-child) { border-bottom: 1px solid var(--mirk-input-border); }
  .hcms-shell .mirk-sortable__label {
    display: block; font-size: 11px; text-transform: uppercase; letter-spacing: 0.2em;
    margin-bottom: 2px; color: var(--mirk-sortable-label);
  }
  .hcms-shell .mirk-sortable__field {
    width: 100%; background: transparent; color: var(--mirk-bevel-fg);
    font-size: 18px; line-height: 1.5; outline: none;
  }
  .hcms-shell .mirk-sortable__field::placeholder { color: var(--mirk-sortable-placeholder); }
  .hcms-shell .mirk-sortable__field:focus-visible {
    outline: 1px solid var(--mirk-focus-color); outline-offset: var(--mirk-focus-offset);
  }
  .hcms-shell .mirk-sortable--small { gap: 0.375rem; }
  .hcms-shell .mirk-sortable--small .mirk-sortable__grip { width: 24px; }
  .hcms-shell .mirk-sortable--small .mirk-sortable__row { padding: 5px 13px 6px; }
  .hcms-shell .mirk-sortable--small .mirk-sortable__label { font-size: 10px; margin-bottom: 1px; }
  .hcms-shell .mirk-sortable--small .mirk-sortable__field { font-size: 14px; }

  /* ============================ CHIP ============================ */
  /* A collapsible recovery/notification: a round pill that expands into a RAISED
     panel \u2014 the kit's one elevated surface (a distinct --mirk-bg face, a hairline
     outline, and a soft drop shadow, the only shadow in the kit, reserved for this
     raised tier). State lives in classes (--open, is-changes) so it round-trips via
     outerHTML; mirk.js only flips them on click. Color reads from generic kit
     tokens through 5 slim --mirk-chip-* hooks, so it matches the kit and reskins by
     overriding a hook, not a rule. In a Hyperclay app, add \`save-remove\` to the
     block so a transient prompt never persists into the saved file. */
  .hcms-shell .mirk-chip { display: inline-flex; flex-direction: column; align-items: flex-start; }
  .hcms-shell .mirk-chip__panel { display: none; }
  .hcms-shell .mirk-chip--open .mirk-chip__trigger { display: none; }
  .hcms-shell .mirk-chip--open .mirk-chip__panel { display: flex; }

  /* Collapsed chip \u2014 a round mirk-button pill (mirk-button--round in the markup),
     the alert glyph seated in the label, so it reads as a distinct affordance, not
     a flat button. A soft, tight lift sets it above the page; deeper on a dark
     canvas (same media + forced-mode pattern as the panel shadow below). */
  .hcms-shell .mirk-chip__trigger .mirk-button__label { display: inline-flex; align-items: center; gap: 6px; font-size: 14px; }
  .hcms-shell .mirk-chip__trigger { box-shadow: 0 6px 14px -8px rgba(43, 36, 27, 0.5), 0 2px 5px -3px rgba(43, 36, 27, 0.32); }
  @media (prefers-color-scheme: dark) { .hcms-shell .mirk-chip__trigger { box-shadow: 0 6px 14px -8px rgba(0, 0, 0, 0.6), 0 2px 6px -3px rgba(0, 0, 0, 0.5); } }
  .hcms-shell.light .mirk-chip__trigger, .hcms-shell[data-theme="light"] .mirk-chip__trigger { box-shadow: 0 6px 14px -8px rgba(43, 36, 27, 0.5), 0 2px 5px -3px rgba(43, 36, 27, 0.32); }
  .hcms-shell.dark .mirk-chip__trigger, .hcms-shell[data-theme="dark"] .mirk-chip__trigger { box-shadow: 0 6px 14px -8px rgba(0, 0, 0, 0.6), 0 2px 6px -3px rgba(0, 0, 0, 0.5); }
  /* Drive the warning fill from CSS, not an SVG fill="var(...)" presentation
     attribute (var() is not reliably honored there). */
  .hcms-shell .mirk-chip__warn { fill: var(--mirk-chip-alert); }
  .hcms-shell .mirk-chip__trigger .mirk-chip__warn { vertical-align: -2px; }

  /* The panel \u2014 the raised surface: a --mirk-bg face over the page, a hairline
     outline, a soft drop shadow. */
  .hcms-shell .mirk-chip__panel {
    width: 300px; max-width: calc(100vw - 44px);
    background: var(--mirk-chip-surface); color: var(--mirk-fg);
    border: 1px solid var(--mirk-chip-edge); border-radius: var(--mirk-radius);
    box-shadow: 0 20px 50px -30px rgba(43, 36, 27, 0.6);
    padding: 14px 15px 13px; flex-direction: column; gap: 12px;
  }
  /* The panel's larger drop (the collapsed pill above carries a tighter one); deepen
     both on a dark canvas. Mirrors the --focus-offset pattern: a media default for
     the OS, plus explicit forced-mode overrides. */
  @media (prefers-color-scheme: dark) { .hcms-shell .mirk-chip__panel { box-shadow: 0 20px 52px -26px rgba(0, 0, 0, 0.78); } }
  .hcms-shell.light .mirk-chip__panel, .hcms-shell[data-theme="light"] .mirk-chip__panel { box-shadow: 0 20px 50px -30px rgba(43, 36, 27, 0.6); }
  .hcms-shell.dark .mirk-chip__panel, .hcms-shell[data-theme="dark"] .mirk-chip__panel { box-shadow: 0 20px 52px -26px rgba(0, 0, 0, 0.78); }

  /* Head \u2014 icon, text, collapse glyph. */
  .hcms-shell .mirk-chip__head { display: flex; gap: 10px; align-items: flex-start; }
  .hcms-shell .mirk-chip__icon { flex-shrink: 0; line-height: 0; margin-top: 1px; }
  .hcms-shell .mirk-chip__headtext { min-width: 0; }
  .hcms-shell .mirk-chip__eyebrow {
    font-size: 9.5px; text-transform: uppercase; letter-spacing: 0.2em;
    color: var(--mirk-placeholder-color); margin-bottom: 3px;
  }
  .hcms-shell .mirk-chip__title { margin: 0; font-size: 13px; font-weight: 400; line-height: 1.3; color: var(--mirk-fg); }
  .hcms-shell .mirk-chip__collapse {
    margin-left: auto; flex-shrink: 0; width: 24px; height: 24px;
    display: inline-flex; align-items: center; justify-content: center;
    background: none; border: 0; cursor: pointer; padding: 0;
    color: var(--mirk-placeholder-color);
  }
  .hcms-shell .mirk-chip__collapse:hover { color: var(--mirk-fg); }
  .hcms-shell .mirk-chip__collapse svg { display: block; }

  /* Meta line. */
  .hcms-shell .mirk-chip__meta { font-size: 11px; letter-spacing: 0.04em; color: var(--mirk-placeholder-color); }
  .hcms-shell .mirk-chip__changes-toggle {
    background: none; border: 0; cursor: pointer; font: inherit; font-size: 11px; padding: 0 0 0 4px;
    color: var(--mirk-placeholder-color); text-decoration: underline; text-underline-offset: 2px;
  }
  .hcms-shell .mirk-chip__changes-toggle:hover { color: var(--mirk-fg); }

  /* Before/after field table \u2014 a recessed stack, revealed by the changes toggle. */
  .hcms-shell .mirk-chip__preview {
    display: none; flex-direction: column; gap: 11px;
    background: var(--mirk-bevel-bg); border: 1px solid var(--mirk-chip-edge);
    padding: 10px 11px;
  }
  .hcms-shell .mirk-chip__panel.is-changes .mirk-chip__preview { display: flex; }
  .hcms-shell .mirk-chip__row { display: flex; flex-direction: column; gap: 2px; }
  .hcms-shell .mirk-chip__key {
    font-size: 9.5px; text-transform: uppercase; letter-spacing: 0.1em;
    color: var(--mirk-placeholder-color);
  }
  .hcms-shell .mirk-chip__old, .hcms-shell .mirk-chip__new { max-width: 100%; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
  .hcms-shell .mirk-chip__old { font-size: 11px; color: var(--mirk-chip-alert); text-decoration: line-through; opacity: 0.85; }
  .hcms-shell .mirk-chip__new { font-size: 12px; color: var(--mirk-fg); }

  /* Action stack \u2014 full-width buttons by weight: an embossed primary, a plain bevel
     revert, a quiet (--quiet) dismiss. */
  .hcms-shell .mirk-chip__actions { display: flex; flex-direction: column; gap: 8px; margin-top: 1px; }
  .hcms-shell .mirk-chip__actions .mirk-button { width: 100%; }
  /* Primary action \u2014 a genuine kit bevel button whose bevel palette is derived from
     the chip's primary fill (lighter top-left, darker bottom-right), so it embosses
     on any color. The base .mirk-button rules then drive its border, hover, and the
     :active flip, identical to the kit's other buttons. */
  .hcms-shell .mirk-chip__actions .mirk-chip__action--primary {
    --mirk-bevel-bg:       var(--mirk-chip-primary-bg);
    --mirk-bevel-fg:       var(--mirk-chip-primary-fg);
    --mirk-bevel-tl:       color-mix(in srgb, var(--mirk-chip-primary-bg), white 24%);
    --mirk-bevel-br:       color-mix(in srgb, var(--mirk-chip-primary-bg), black 32%);
    --mirk-bevel-hover-bg: color-mix(in srgb, var(--mirk-chip-primary-bg), white 10%);
  }

  /* ============================ FIELD ============================ */
  /* Label + control + hint as one unit, so a form composes without utilities
     (and without needing the mirk-page rhythm). Any control drops in; the gap
     matches the page scaffold's label/hint hug. Consecutive fields space
     themselves. */
  .hcms-shell .mirk-field { display: flex; flex-direction: column; gap: 6px; }
  .hcms-shell .mirk-field__label { font-size: 16px; line-height: 1.5; }
  .hcms-shell .mirk-field + .mirk-field { margin-block-start: 18px; }
  .hcms-shell .mirk-field--small .mirk-field__label { font-size: 14px; }

  /* An alert hint inside a field stays hidden until a control in the field
     goes :user-invalid \u2014 a zero-JS native-validation message. Show one
     unconditionally (a server-side error) by placing it outside the field or
     overriding display. */
  .hcms-shell .mirk-field .mirk-hint--alert { display: none; }
  .hcms-shell .mirk-field:has(:user-invalid) .mirk-hint--alert { display: block; }

  /* ============================ INVALID ============================ */
  /* Native constraint validation, styled. :user-invalid fires only after the
     user interacts (unlike :invalid, which would paint required fields red on
     load). A flat destructive border is the 0012 stateful read; the focus ring
     stays the focus signal. Pair the message with mirk-hint--alert. */
  .hcms-shell .mirk-input:user-invalid, .hcms-shell .mirk-textarea:user-invalid, .hcms-shell .mirk-date__field:user-invalid, .hcms-shell .mirk-select__field:user-invalid, .hcms-shell .mirk-number:has(.mirk-number__input:user-invalid) {
    border-color: var(--mirk-destructive);
  }
  .hcms-shell .mirk-checkbox:has(:user-invalid) .mirk-checkbox__box, .hcms-shell .mirk-toggle:has(:user-invalid) .mirk-toggle__track {
    border-color: var(--mirk-destructive);
  }
  /* The unchecked ring is a borderless gradient pill; invalid swaps it for a
     flat destructive ring (the fill pill still seats inside). */
  .hcms-shell .mirk-radio:has(:user-invalid) .mirk-radio__ring {
    background-image: none;
    border: 2px solid var(--mirk-destructive);
  }

  /* ============================ PROGRESS ============================ */
  /* A native <progress> in the slider's clothes: the canvas channel, the
     slider-fill value bar. --blocks segments the fill into pixel blocks.
     Keep the -webkit and -moz rules separate \u2014 an unrecognized pseudo-element
     invalidates the whole selector list. */
  .hcms-shell .mirk-progress {
    appearance: none; -webkit-appearance: none;
    display: block; width: 100%; height: 12px;
    border: 1px solid var(--mirk-input-border);
    background: var(--mirk-canvas);
  }
  .hcms-shell .mirk-progress::-webkit-progress-bar { background: transparent; }
  .hcms-shell .mirk-progress::-webkit-progress-value { background: var(--mirk-slider-fill); }
  .hcms-shell .mirk-progress::-moz-progress-bar { background: var(--mirk-slider-fill); }
  .hcms-shell .mirk-progress--small { height: 8px; }
  .hcms-shell .mirk-progress--round { border-radius: 9999px; overflow: hidden; }
  .hcms-shell .mirk-progress--blocks::-webkit-progress-value {
    background: repeating-linear-gradient(to right,
      var(--mirk-slider-fill) 0 8px, transparent 8px 11px);
  }
  .hcms-shell .mirk-progress--blocks::-moz-progress-bar {
    background: repeating-linear-gradient(to right,
      var(--mirk-slider-fill) 0 8px, transparent 8px 11px);
  }

  /* ============================ NOTE ============================ */
  /* An informational callout. Flat on purpose: bevel means pressable in this
     kit (0012) and a note is content, so it gets the 1px content border over
     the recessed face, never the raised edge. The 4px left edge carries the
     status: neutral ink by default, destructive on --alert. */
  .hcms-shell .mirk-note {
    display: flex; flex-direction: column; gap: 4px;
    background: var(--mirk-bevel-bg);
    border: 1px solid var(--mirk-input-border);
    border-left: 4px solid var(--mirk-mark-fg);
    padding: 10px 14px 11px;
    font-size: 14px; line-height: 1.5;
  }
  .hcms-shell .mirk-note__title {
    font-size: 11px; text-transform: uppercase; letter-spacing: 0.2em;
    color: var(--mirk-mark-fg);
  }
  .hcms-shell .mirk-note__body { color: var(--mirk-fg); }
  .hcms-shell .mirk-note--alert { border-left-color: var(--mirk-destructive); }
  .hcms-shell .mirk-note--alert .mirk-note__title { color: var(--mirk-destructive); }
  .hcms-shell .mirk-note--rounded { border-radius: var(--mirk-radius); }

  /* ============================ HINT ============================ */
  /* Small print under a field: neutral help text, or --alert validation text. */
  .hcms-shell .mirk-hint { margin: 0; font-size: 13px; line-height: 1.5; color: var(--mirk-placeholder-color); }
  .hcms-shell .mirk-hint--alert { color: var(--mirk-destructive); }

  /* ============================ LIST ============================ */
  /* Content bullets. <ul> gets a square pixel dot (the sortable dot's idiom,
     one step larger); a nested <ul> hollows it. <ol> gets zero-padded counters
     in the muted label ink. Styles the bare <li> by descent, like the platform. */
  .hcms-shell .mirk-list {
    list-style: none; margin: 0; padding: 0;
    display: flex; flex-direction: column; gap: 6px;
    font-size: 16px; line-height: 1.5;
  }
  .hcms-shell .mirk-list li { position: relative; padding-left: 22px; }
  .hcms-shell ul.mirk-list > li::before {
    content: ""; position: absolute; left: 2px; top: 0.55em;
    width: 6px; height: 6px; background: var(--mirk-mark-fg);
    box-shadow: 1px 1px 0 0 var(--mirk-sortable-shadow);
  }
  .hcms-shell ol.mirk-list { counter-reset: mirk-li; }
  .hcms-shell ol.mirk-list > li { counter-increment: mirk-li; padding-left: 38px; }
  .hcms-shell ol.mirk-list > li::before {
    content: counter(mirk-li, decimal-leading-zero) ".";
    position: absolute; left: 0; top: 0;
    color: var(--mirk-sortable-label);
  }

  /* One nested level: hollow square, same stack. */
  .hcms-shell .mirk-list ul {
    list-style: none; margin: 6px 0 0; padding: 0;
    display: flex; flex-direction: column; gap: 6px;
  }
  .hcms-shell .mirk-list ul > li::before {
    content: ""; position: absolute; left: 2px; top: 0.55em;
    width: 6px; height: 6px; background: transparent;
    border: 1.5px solid var(--mirk-mark-fg); box-shadow: none;
  }

  .hcms-shell .mirk-list--small { font-size: 14px; gap: 4px; }
  .hcms-shell .mirk-list--small li { padding-left: 18px; }
  .hcms-shell ul.mirk-list--small > li::before, .hcms-shell .mirk-list--small ul > li::before { width: 5px; height: 5px; }
  .hcms-shell ol.mirk-list--small > li { padding-left: 32px; }

  /* ============================ BADGE ============================ */
  /* A static tag label \u2014 the display counterpart to the mirk-tags input. Flat
     on purpose: bevel means pressable (0012), a badge is content. */
  .hcms-shell .mirk-badge {
    display: inline-flex; align-items: center; gap: 0.375rem;
    padding: 1px 8px 2px; font-size: 12px; line-height: 1.5;
    color: var(--mirk-fg); background: var(--mirk-bevel-bg);
    border: 1px solid var(--mirk-input-border);
    white-space: nowrap; vertical-align: middle;
  }
  .hcms-shell .mirk-badge--accent { background: var(--mirk-accent); }
  .hcms-shell .mirk-badge--round { border-radius: 9999px; padding: 1px 10px 2px; }
  .hcms-shell .mirk-badge--alert { color: var(--mirk-destructive); border-color: var(--mirk-destructive); }

  /* ============================ TABLE ============================ */
  /* A flat data surface: the content face in a 1px frame, header cells in the
     eyebrow register, hairline row dividers. Semantic <table> styled by
     descent \u2014 no per-cell classes. The header/stripe tints ride color-mix
     toward the ink so they stay visible in every palette (Pixel Quiet's
     canvas and face are nearly the same value). Wrap in an overflow-x:auto
     div when the table can outgrow its column. */
  .hcms-shell .mirk-table {
    width: 100%; border-collapse: collapse;
    background: var(--mirk-bevel-bg);
    border: 1px solid var(--mirk-input-border);
    font-size: 14px; line-height: 1.5;
  }
  .hcms-shell .mirk-table th {
    text-align: left; font-weight: 400;
    font-size: 11px; text-transform: uppercase; letter-spacing: 0.15em;
    color: var(--mirk-placeholder-color);
    background: color-mix(in srgb, var(--mirk-bevel-bg), var(--mirk-fg) 4%);
    padding: 7px 14px;
    border-bottom: 1px solid var(--mirk-input-border);
  }
  .hcms-shell .mirk-table td {
    padding: 8px 14px;
    border-bottom: 1px solid var(--mirk-input-border);
    vertical-align: top;
  }
  .hcms-shell .mirk-table tbody tr:last-child td { border-bottom: 0; }
  .hcms-shell .mirk-table--striped tbody tr:nth-child(even) td {
    background: color-mix(in srgb, var(--mirk-bevel-bg), var(--mirk-fg) 3%);
  }
  .hcms-shell .mirk-table--small { font-size: 13px; }
  .hcms-shell .mirk-table--small th { padding: 5px 12px; font-size: 10px; }
  .hcms-shell .mirk-table--small td { padding: 5px 12px; }

  /* ============================ PAGE ============================ */
  /* The quickstart shell: mirk-page on <body> (or any wrapper) gives a centered
     column + typographic defaults, so two CDN tags and one class boot a full
     page. Every rule rides :where() (zero specificity): any utility, component
     class, or consumer rule beats it. Flow rhythm targets direct children only,
     so margins never leak inside component internals. */
  .hcms-shell .mirk-page { max-width: 640px; margin-inline: auto; padding: 48px 24px 96px; }
  .hcms-shell .mirk-page--wide { max-width: 960px; }

  /* Departure Mono ships one weight \u2014 hierarchy comes from size, never bold. */
  .hcms-shell .mirk-page :where(h1, h2, h3, h4) { margin: 0; font-weight: 400; line-height: 1.15; }
  .hcms-shell .mirk-page :where(h1) { font-size: 40px; }
  .hcms-shell .mirk-page :where(h2) { font-size: 26px; }
  .hcms-shell .mirk-page :where(h3) { font-size: 20px; }
  .hcms-shell .mirk-page :where(h4) { font-size: 16px; }
  .hcms-shell .mirk-page :where(p) { margin: 0; font-size: 16px; line-height: 1.6; }

  .hcms-shell .mirk-page :where(a) { color: inherit; text-decoration: underline; text-underline-offset: 2px; }
  .hcms-shell .mirk-page :where(code, kbd) {
    font-size: 0.875em; padding: 1px 4px;
    background: var(--mirk-bevel-bg); border: 1px solid var(--mirk-input-border);
  }
  .hcms-shell .mirk-page :where(pre) {
    margin: 0; padding: 12px 14px; font-size: 13px; line-height: 1.5; overflow-x: auto;
    background: var(--mirk-bevel-bg); border: 1px solid var(--mirk-input-border);
  }
  .hcms-shell .mirk-page :where(pre code) { padding: 0; border: 0; background: none; font-size: inherit; }
  .hcms-shell .mirk-page :where(hr) { border: 0; border-top: 1px solid var(--mirk-input-border); }

  /* Flow rhythm \u2014 direct children only; headings open sections, eyebrows and
     hints hug their neighbors. Equal specificity, so source order settles ties. */
  .hcms-shell .mirk-page > :where(* + *) { margin-block-start: 14px; }
  .hcms-shell .mirk-page > :where(h1, h2, h3, h4):where(* + *) { margin-block-start: 40px; }
  .hcms-shell .mirk-page > :where(.mirk-eyebrow + *), .hcms-shell .mirk-page > :where(* + .mirk-hint) { margin-block-start: 6px; }

  /* ============================ EYEBROW ============================ */
  /* The kit's signature section label, as a shippable class. Block so it works
     the same on <p>, <label>, or <div>, and flow margins always apply. */
  .hcms-shell .mirk-eyebrow {
    display: block; margin: 0;
    font-size: 11px; text-transform: uppercase; letter-spacing: 0.2em;
    color: var(--mirk-placeholder-color);
  }
}

/* Respect the machine \u2014 outside @layer components so it always wins. */
@media (prefers-reduced-motion: reduce) {
  .hcms-shell .mirk-button--round, .hcms-shell .mirk-toggle__thumb { transition: none; }
  .hcms-shell .mirk-button:active .mirk-button__label { translate: none; }
}
@media (forced-colors: active) {
  .hcms-shell .mirk-button, .hcms-shell .mirk-select__field, .hcms-shell .mirk-checkbox__box, .hcms-shell .mirk-toggle__track { border: 1px solid ButtonText; }
}

/* ===== pixel-quiet overrides (hypercms-owned) ===== */
/* =====================================================================
   Pixel Quiet \u2014 hypercms theme overrides (hypercms-owned).

   Adapted from ARCHIVE_PROJECTS/cms-sidebar/pixel-quiet/overrides.css. Two jobs:
     1. Retune --mirk-* tokens, scoped to .hcms-shell.pixel-quiet (never :root).
     2. Author the panel geometry + the functional chrome the static mockup
        doesn't have (error banner, add/remove/move controls, the engine's
        sortable cards, push / overlay / left-dock modes) on top of the
        .hcms-* structural hooks.

   This file is concatenated AFTER the scoped mirk base+components by
   scripts/build-theme.js, so plain rules here win over mirk's @layer
   components with zero !important. Loaded only inside .hcms-shell, so nothing
   here leaks onto the host page.
   ===================================================================== */

/* ============================================================
   TOKEN RETUNE \u2014 LIGHT (warm cream, gentle near-equal bevel)
   ============================================================ */
.hcms-shell.pixel-quiet {
  color-scheme: light;
  --mirk-canvas: #F7F2EA;
  --mirk-bg: #F7F2EA;
  --mirk-fg: #2B241B;
  --mirk-accent: #efefe5;
  --mirk-destructive: #C24A3A;
  --mirk-focus-color: #C7AE93;

  --mirk-bevel-bg: #FCF8F1;
  --mirk-bevel-fg: #2B241B;
  --mirk-bevel-tl: #F0E7D8;
  --mirk-bevel-br: #E2D4BF;
  --mirk-bevel-hover-bg: #F4ECDF;
  --mirk-pill-inner-top: #FBF6EE;
  --mirk-input-border: #D8C8AF;
  --mirk-placeholder-color: #A8987F;

  --mirk-mark-fg: #6B5942;
  --mirk-toggle-bg: #EFDBBD;
  --mirk-toggle-hi: #F4EADA;
  --mirk-toggle-lo: #C2A87E;
  --mirk-sortable-dot: #DDCBB0;
  --mirk-sortable-shadow: #C9B493;
  --mirk-sortable-label: #8C7B62;
  --mirk-sortable-placeholder: #A8987F;

  --mirk-radius: 5px;
  --mirk-focus-offset: 2px;
}

/* dark token deltas, shared by the explicit .dark opt-in and OS preference */
.hcms-shell.pixel-quiet.dark,
.hcms-shell.pixel-quiet[data-theme="dark"] {
  color-scheme: dark;
  --mirk-canvas: #0B0C13;
  --mirk-bg: #11131E;
  --mirk-fg: #ECEAF2;
  --mirk-accent: #1D1F2F;
  --mirk-focus-color: #4A506B;

  --mirk-bevel-bg: #1A1D2C;
  --mirk-bevel-fg: #ECEAF2;
  --mirk-bevel-tl: #2A2E42;
  --mirk-bevel-br: #14182A;
  --mirk-bevel-hover-bg: #202436;
  --mirk-pill-inner-top: #202436;
  --mirk-input-border: #353B52;
  --mirk-placeholder-color: #6A7090;

  --mirk-mark-fg: #C9CDE0;
  --mirk-toggle-bg: #3E4660;
  --mirk-toggle-hi: #4E567A;
  --mirk-toggle-lo: #262B42;
  --mirk-sortable-dot: #353B52;
  --mirk-sortable-shadow: #0E1120;
  --mirk-sortable-label: #8A90AB;
  --mirk-sortable-placeholder: #6A7090;
}

/* Auto-dark on OS preference, unless the shell pins light with .light */
@media (prefers-color-scheme: dark) {
  .hcms-shell.pixel-quiet:not(.light):not([data-theme="light"]) {
    color-scheme: dark;
    --mirk-canvas: #0B0C13;
    --mirk-bg: #11131E;
    --mirk-fg: #ECEAF2;
    --mirk-accent: #1D1F2F;
    --mirk-focus-color: #4A506B;

    --mirk-bevel-bg: #1A1D2C;
    --mirk-bevel-fg: #ECEAF2;
    --mirk-bevel-tl: #2A2E42;
    --mirk-bevel-br: #14182A;
    --mirk-bevel-hover-bg: #202436;
    --mirk-pill-inner-top: #202436;
    --mirk-input-border: #353B52;
    --mirk-placeholder-color: #6A7090;

    --mirk-mark-fg: #C9CDE0;
    --mirk-toggle-bg: #3E4660;
    --mirk-toggle-hi: #4E567A;
    --mirk-toggle-lo: #262B42;
    --mirk-sortable-dot: #353B52;
    --mirk-sortable-shadow: #0E1120;
    --mirk-sortable-label: #8A90AB;
    --mirk-sortable-placeholder: #6A7090;
  }
}

/* ============================================================
   SHELL GEOMETRY \u2014 fixed, docked panel, single column.
   position: fixed makes the shell a containing block so the absolute
   minibar anchors to it; flex column so the body owns the scroll.
   ============================================================ */
.hcms-shell.pixel-quiet.hcms-panel {
  box-sizing: border-box;
  position: fixed;
  top: 0;
  right: 0;
  bottom: 0;
  width: var(--hcms-shell-width, 380px);
  max-width: 100vw;
  z-index: 2147483000;
  display: flex;
  flex-direction: column;
  color: var(--mirk-fg);
  background: var(--mirk-bg);
  border-left: 1px solid var(--mirk-input-border);
  box-shadow: -16px 0 48px -28px rgba(43, 36, 27, 0.45);
}
.hcms-shell.pixel-quiet.hcms-panel.dark {
  box-shadow: -16px 0 48px -28px rgba(0, 0, 0, 0.6);
}

.hcms-shell.pixel-quiet.hcms-side-left {
  right: auto;
  left: 0;
  border-left: 0;
  border-right: 1px solid var(--mirk-input-border);
  box-shadow: 16px 0 48px -28px rgba(43, 36, 27, 0.45);
}

/* Push the page over so docked content is never hidden underneath. */
body.hcms-open { --hcms-shell-width: 380px; }
body.hcms-open:not(.hcms-overlay) { padding-right: var(--hcms-shell-width); }
body.hcms-open.hcms-side-left:not(.hcms-overlay) { padding-right: 0; padding-left: var(--hcms-shell-width); }
body.hcms-open.hcms-overlay { overflow: hidden; }

@media (max-width: 799px) {
  body.hcms-open { --hcms-shell-width: 100vw; }
  body.hcms-open:not(.hcms-overlay),
  body.hcms-open.hcms-side-left:not(.hcms-overlay) { padding-right: 0; padding-left: 0; }
  body.hcms-open { overflow: hidden; }
}

/* ---------- SCROLL REGION \u2014 holds the (scrollable) header + form + save ---------- */
.hcms-shell-body {
  flex: 1;
  min-height: 0;
  overflow-y: auto;
}

/* ---------- CONDENSED MINIBAR \u2014 appears once the full header scrolls away ---------- */
.hcms-shell-minibar {
  position: absolute;
  top: 0;
  left: 0;
  right: 0;
  z-index: 2;
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  padding: 9px 24px 10px;
  background: var(--mirk-bg);
  border-bottom: 1px solid var(--mirk-input-border);
  opacity: 0;
  transform: translateY(-100%);
  pointer-events: none;
  transition: opacity 140ms ease, transform 160ms ease;
}
.hcms-shell.is-condensed .hcms-shell-minibar {
  opacity: 1;
  transform: none;
  pointer-events: auto;
}
.hcms-shell-minibar-title {
  font-size: 14px;
  line-height: 1;
  color: var(--mirk-fg);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

/* ---------- HEADER (no underline rule \u2014 whitespace separates the bands) ---------- */
.hcms-shell-header {
  flex-shrink: 0;
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 12px;
  padding: 24px 24px 4px;
}
.hcms-shell-heading { display: flex; flex-direction: column; gap: 3px; min-width: 0; }
.hcms-shell-eyebrow {
  font-size: 11px;
  text-transform: uppercase;
  letter-spacing: 0.18em;
  color: var(--mirk-placeholder-color);
}
.hcms-shell-title {
  margin: 0;
  font-size: 18px;
  font-weight: 400;
  line-height: 1.2;
  color: var(--mirk-fg);
}
.hcms-shell-close.mirk-button {
  flex-shrink: 0;
  padding: 2px 9px 3px;
  line-height: 1;
}
.hcms-shell-close .mirk-button__label { font-size: 16px; }

/* ---------- FORM \u2014 generous, even vertical rhythm ---------- */
.hcms-form {
  display: flex;
  flex-direction: column;
  gap: 26px;
  padding: 12px 24px 28px;
}

/* one labeled scalar field */
.hcms-field { display: flex; flex-direction: column; gap: 9px; }
.hcms-field--row {
  flex-direction: row;
  align-items: center;
  justify-content: space-between;
}

/* themed field label */
.hcms-label {
  font-size: 11px;
  text-transform: uppercase;
  letter-spacing: 0.14em;
  color: var(--mirk-sortable-label);
}

/* comfortable control padding + readable size, in mirk's own mono */
.hcms-shell.pixel-quiet .mirk-input,
.hcms-shell.pixel-quiet .mirk-textarea {
  font-size: 15px;
  padding: 9px 14px 10px;
}
.hcms-shell.pixel-quiet textarea.mirk-input { min-height: 76px; resize: vertical; }

/* default scalar fields: one-row textareas that grow with their content.
   Browsers without field-sizing get a scrollHeight fallback (enhance.js). */
.hcms-shell.pixel-quiet .hcms-form textarea.mirk-textarea {
  field-sizing: content;
  resize: none;
  overflow: hidden;
  min-height: 0;
}

/* rich-text surface (@richtext): a contenteditable styled like a textarea */
.hcms-shell.pixel-quiet .hcms-richtext {
  min-height: 2.5em;
  cursor: text;
  overflow-wrap: break-word;
}
.hcms-shell.pixel-quiet .hcms-richtext a {
  color: inherit;
  text-decoration: underline;
  text-underline-offset: 2px;
}
.hcms-shell.pixel-quiet .hcms-richtext:empty::before {
  content: attr(data-hcms-placeholder);
  color: var(--mirk-placeholder-color);
}
.hcms-shell.pixel-quiet .mirk-select__field {
  font-size: 15px;
  padding: 8px 40px 9px 14px;
}
.hcms-shell.pixel-quiet .mirk-radio__label,
.hcms-shell.pixel-quiet .mirk-toggle__label,
.hcms-shell.pixel-quiet .mirk-tags__input { font-size: 15px; }

/* inline radio row */
.hcms-radio-row { display: flex; align-items: center; gap: 22px; flex-wrap: wrap; }

/* chip-field (the @chips built-in): a borderless inline input that sizes to its
   text, so chips read like static chips but stay inline-editable. */
.hcms-shell .mirk-tags__chip { padding-right: 6px; }
.hcms-shell .hcms-chip-field {
  border: 0; background: transparent; color: inherit; font: inherit;
  outline: none; min-width: 2ch; field-sizing: content; padding: 0;
}
.hcms-shell .hcms-chips .hcms-add { margin-top: 4px; align-self: flex-start; }

/* ---------- OBJECT GROUP \u2014 a quiet framed band, not a heavy card ---------- */
.hcms-object {
  display: flex;
  flex-direction: column;
  gap: 16px;
}
.hcms-object-title {
  margin: 0;
  font-size: 11px;
  font-weight: 400;
  text-transform: uppercase;
  letter-spacing: 0.14em;
  color: var(--mirk-sortable-label);
}
.hcms-object-fields { display: flex; flex-direction: column; gap: 16px; }

/* ---------- SCALAR ARRAY \u2014 a calm list of mirk-input rows ---------- */
.hcms-array { display: flex; flex-direction: column; gap: 14px; }
.hcms-array-header { display: flex; align-items: baseline; justify-content: space-between; }
.hcms-array-title {
  margin: 0;
  font-size: 11px;
  font-weight: 400;
  text-transform: uppercase;
  letter-spacing: 0.14em;
  color: var(--mirk-sortable-label);
}
/* The generic stacked-list layout is unlayered, so it would beat mirk's
   @layer-components rules on any slot that is also a mirk component. Exempt a
   mirk tags box so it keeps mirk's own row-wrap layout and inner padding. */
.hcms-array-items:not(.mirk-tags) {
  list-style: none;
  margin: 0;
  padding: 0;
  display: flex;
  flex-direction: column;
  gap: 10px;
}
.hcms-array-item {
  display: flex;
  align-items: center;
  gap: 8px;
}
.hcms-array-item .mirk-input { flex: 1; min-width: 0; }

/* ---------- OBJECT ARRAY \u2014 mirk-sortable cards from the engine markup ---------- */
.hcms-array--cards .hcms-array-items { gap: 14px; }
.hcms-card.mirk-sortable__item { background: var(--mirk-bevel-bg); position: relative; }
.hcms-card .hcms-card-fields { display: flex; flex-direction: column; flex: 1; min-width: 0; }
.hcms-card .hcms-field {
  gap: 2px;
  padding: 8px 14px 9px;
}
.hcms-card .hcms-field:not(:last-child) { border-bottom: 1px solid var(--mirk-input-border); }
.hcms-card .hcms-label { letter-spacing: 0.16em; }
/* fields inside a card read as quiet rows, not chunky boxed inputs */
.hcms-card .mirk-input,
.hcms-card .mirk-textarea {
  border: none;
  background: transparent;
  padding: 0;
  font-size: 15px;
}
.hcms-card .mirk-input:focus-visible,
.hcms-card .mirk-textarea:focus-visible { outline: none; }
/* the remove \xD7 is pulled out to the card corner (below), so the controls row
   now only holds the sr-only move buttons \u2014 collapse it until one is focused. */
.hcms-card-controls {
  display: flex;
  align-items: center;
  justify-content: flex-end;
  gap: 6px;
  padding: 0;
}

/* quiet \xD7 remove control, shared by scalar-array rows and object-array cards */
.hcms-remove {
  flex-shrink: 0;
  appearance: none;
  border: 0;
  background: none;
  color: var(--mirk-placeholder-color);
  cursor: pointer;
  font-size: 16px;
  line-height: 1;
  padding: 2px 6px;
}
.hcms-remove:hover { color: var(--mirk-destructive); }
.hcms-remove[hidden] { display: none; }

/* object-array card: the delete control is a square corner button pinned
   top-right, carrying the card's own 1px border + a crisp-line \xD7 icon. */
.hcms-remove--card {
  position: absolute;
  top: -1px;
  right: -1px;
  width: 18px;
  height: 18px;
  padding: 0;
  display: flex;
  align-items: center;
  justify-content: center;
  border: 1px solid var(--mirk-input-border);
  background: var(--mirk-bevel-bg);
  color: var(--mirk-placeholder-color);
}
.hcms-remove--card:hover { border-color: var(--mirk-destructive); }
.hcms-remove--card .hcms-x { width: 64%; height: 64%; display: block; }

/* "+ Add" \u2014 quiet, pinned left */
.hcms-add.mirk-button { align-self: flex-start; }

/* ---------- UPLOAD COMPONENTS (@file / @image) ----------
   Built on the kit's mirk-file / mirk-image--compact chrome. The native picker
   is visually hidden but focusable (the mirk-button label is the visible
   trigger and rings via :has(:focus-visible)); it is NOT .mirk-*__input, so the
   vendored mirk runtime never handles it. The empty/filled chrome is driven by
   the bound leaf's value attribute (src/href) in CSS \u2014 no JS state to desync. */
.hcms-upload input[type="file"] {
  position: absolute;
  width: 1px;
  height: 1px;
  padding: 0;
  margin: -1px;
  overflow: hidden;
  clip: rect(0, 0, 0, 0);
  white-space: nowrap;
  border: 0;
}

/* @image: show the upload button until the bound <img> carries a real src. */
.hcms-upload--image .mirk-image__thumb { display: none; }
.hcms-upload--image:has(.mirk-image__preview[src]:not([src=""])) .mirk-image__upload { display: none; }
.hcms-upload--image:has(.mirk-image__preview[src]:not([src=""])) .mirk-image__thumb { display: inline-block; }

/* @file: reveal the clear \xD7 and the filename only once the bound <a> has href. */
.hcms-upload--file .hcms-upload-clear { display: none; }
.hcms-upload--file:has(a.mirk-file__name[href]:not([href=""])) .hcms-upload-clear { display: inline-flex; }
/* A filled filename uses the bright foreground (the kit's [data-filled] look),
   driven by the bound href so there's no JS attribute to keep in sync \u2014 the
   vendored runtime that would otherwise stamp data-filled is inert here. */
.hcms-upload--file:has(a.mirk-file__name[href]:not([href=""])) a.mirk-file__name {
  color: var(--mirk-bevel-fg);
}
.hcms-upload--file a.mirk-file__name:empty {
  text-decoration: none;
  cursor: default;
}
.hcms-upload--file a.mirk-file__name:empty::after {
  content: "No file chosen";
  color: var(--mirk-placeholder-color);
}

/* ---------- COLOR (@color) ----------
   Swatch colors arrive as --hcms-swatch on each element's style, so a value
   never becomes a selector or a class. The full palette is an 11-column grid
   (one row per hue); a narrowed set wraps. */
.hcms-color-row { display: flex; align-items: center; gap: 8px; }
.hcms-color-current {
  display: inline-flex; align-items: center; gap: 8px;
  padding: 4px 12px 4px 4px; border: 1px solid var(--mirk-input-border); border-radius: 999px;
  background: var(--mirk-input-bg); color: var(--mirk-fg); font: inherit; cursor: pointer;
}
.hcms-color--inline .hcms-color-current { cursor: default; }
.hcms-color-chip, .hcms-color-swatch { background: var(--hcms-swatch, transparent); }
.hcms-color-chip {
  width: 20px; height: 20px; border-radius: 50%;
  box-shadow: inset 0 0 0 1px color-mix(in oklab, var(--mirk-fg) 20%, transparent);
}
.hcms-color[data-hcms-color-empty] .hcms-color-chip {
  background: repeating-conic-gradient(var(--mirk-input-border) 0 25%, transparent 0 50%) 50% / 8px 8px;
}
.hcms-color-clear {
  display: inline-flex; padding: 2px; border: 0; background: transparent;
  color: var(--mirk-placeholder-color); cursor: pointer;
}
.hcms-color-clear:hover { color: var(--mirk-fg); }
.hcms-color[data-hcms-color-empty] .hcms-color-clear { display: none; }
.hcms-color-panel { margin-top: 8px; }
.hcms-color-panel[hidden] { display: none; }
.hcms-color-grid { display: flex; flex-wrap: wrap; gap: 4px; }
.hcms-color-grid--full { display: grid; grid-template-columns: repeat(11, minmax(0, 1fr)); gap: 3px; }
.hcms-color-swatch {
  width: 22px; height: 22px; padding: 0; border: 0; border-radius: 4px; cursor: pointer;
  box-shadow: inset 0 0 0 1px color-mix(in oklab, var(--mirk-fg) 15%, transparent);
}
.hcms-color-grid--full .hcms-color-swatch { width: auto; height: auto; aspect-ratio: 1; }
.hcms-color-swatch[aria-pressed="true"] { box-shadow: 0 0 0 2px var(--mirk-bg), 0 0 0 4px var(--mirk-fg); }
.hcms-color-swatch:focus-visible { outline: 2px solid var(--mirk-focus-color); outline-offset: 2px; }
.hcms-color-custom { display: flex; align-items: center; gap: 8px; margin-top: 8px; }
.hcms-color-native {
  width: 32px; height: 28px; padding: 0; border: 1px solid var(--mirk-input-border);
  border-radius: 4px; background: none; cursor: pointer;
}
.hcms-color-text { flex: 1; min-width: 0; }

/* ---------- FONT (@font) ----------
   Each option sets its own font-family inline, so the list previews every
   family in its face once the editor-only preview stylesheet arrives. */
.hcms-font-current {
  display: flex; align-items: center; justify-content: space-between; gap: 8px; width: 100%;
  padding: 6px 10px; border: 1px solid var(--mirk-input-border); border-radius: 4px;
  background: var(--mirk-input-bg); color: var(--mirk-fg); font: inherit; cursor: pointer; text-align: left;
}
.hcms-font-name { font-size: 15px; }
.hcms-font-chevron { transform: rotate(90deg); color: var(--mirk-placeholder-color); }
.hcms-font-panel { margin-top: 6px; }
.hcms-font-panel[hidden] { display: none; }
.hcms-font-list {
  display: flex; flex-direction: column; max-height: 320px; overflow-y: auto;
  border: 1px solid var(--mirk-input-border); border-radius: 4px; background: var(--mirk-input-bg);
}
.hcms-font-group {
  padding: 8px 10px 4px; font-size: 11px; letter-spacing: 0.04em; text-transform: uppercase;
  color: var(--mirk-placeholder-color);
}
.hcms-font-option {
  padding: 6px 10px; border: 0; background: transparent; color: var(--mirk-fg);
  font-size: 16px; text-align: left; cursor: pointer;
}
.hcms-font-option:hover { background: color-mix(in oklab, var(--mirk-fg) 8%, transparent); }
.hcms-font-option[aria-selected="true"] { background: color-mix(in oklab, var(--mirk-fg) 14%, transparent); }
.hcms-font-option:focus-visible { outline: 2px solid var(--mirk-focus-color); outline-offset: -2px; }

/* ---------- VARIANT (@variant) ----------
   Native radios behind icon tiles: arrow keys move the choice, the tile with
   the checked radio is the selected one. */
.hcms-variant-row { display: flex; flex-wrap: wrap; gap: 8px; }
.hcms-variant-option {
  position: relative; display: inline-flex; flex-direction: column; align-items: center; gap: 4px;
  min-width: 64px; padding: 8px 10px; border: 1px solid var(--mirk-input-border); border-radius: 6px;
  background: var(--mirk-input-bg); color: var(--mirk-placeholder-color); font-size: 12px; cursor: pointer;
}
.hcms-variant-option:hover { color: var(--mirk-fg); }
.hcms-variant-option:has(input:checked) {
  color: var(--mirk-fg); border-color: var(--mirk-fg);
  box-shadow: inset 0 0 0 1px var(--mirk-fg);
}
.hcms-variant-option:has(input:focus-visible) { outline: 2px solid var(--mirk-focus-color); outline-offset: 2px; }
.hcms-variant-icon { display: inline-flex; }

/* ---------- UPLOADING (spec \xA79) ----------
   One attribute, [data-hcms-uploading], set on the field for as long as the host
   has the bytes, plus --hcms-upload-progress carrying the percent. No markup of
   its own: the templates keep their exact shape, so a half-finished upload cannot
   leave an orphan node behind in the form. The at-upload-time picture is painted
   on the frame as a background, never assigned to the bound <img>, because that
   img IS the field's value and a commit landing mid-upload would write a
   two-megabyte data URL straight into the live page. */
.hcms-upload[data-hcms-uploading] .mirk-image__frame {
  background-size: cover;
  background-position: center;
}
/* The empty state hides the thumb, so an upload into an empty field would have
   nowhere to show. Uploading reveals it, preview or not. */
.hcms-upload--image[data-hcms-uploading] .mirk-image__thumb { display: inline-block; }
.hcms-upload--image[data-hcms-uploading] .mirk-image__upload { display: none; }
/* No \xD7 mid-flight: clearing writes the leaf empty, which says nothing about the
   request still running and reads as a cancel that is not one. */
.hcms-upload[data-hcms-uploading] .hcms-upload-clear { display: none; }

.hcms-upload[data-hcms-uploading] .mirk-image__frame::after,
.hcms-upload--file[data-hcms-uploading] .mirk-file::after {
  content: "";
  position: absolute;
  left: 0;
  bottom: 0;
  height: 3px;
  width: var(--hcms-upload-progress, 0%);
  background: var(--mirk-bevel-fg);
  transition: width 120ms linear;
}
/* The @file row has no frame to hang the bar on, so it becomes the positioning
   context itself. */
.hcms-upload--file[data-hcms-uploading] .mirk-file { position: relative; }

/* clear-\xD7 (vendored-inert; data-hcms-action, never .mirk-*__remove). Bare \xD7 for
   @file, a pinned corner badge for @image \u2014 mirroring .hcms-remove / --card. */
.hcms-upload-clear {
  flex-shrink: 0;
  appearance: none;
  border: 0;
  background: none;
  color: var(--mirk-placeholder-color);
  cursor: pointer;
  line-height: 0;
  padding: 2px 4px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
}
.hcms-upload-clear:hover { color: var(--mirk-destructive); }
.hcms-upload-clear .hcms-x { display: block; width: 14px; height: 14px; }

.hcms-upload-clear--badge {
  position: absolute;
  top: -7px;
  right: -7px;
  width: 18px;
  height: 18px;
  padding: 0;
  border-radius: 50%;
  border: 1px solid var(--mirk-input-border);
  background: var(--mirk-bevel-bg);
  color: var(--mirk-bevel-fg);
}
.hcms-upload-clear--badge:hover { color: var(--mirk-destructive); border-color: var(--mirk-destructive); }
.hcms-upload-clear--badge .hcms-x { width: 10px; height: 10px; }

/* ---------- UNRESOLVED-FIELDS NOTICE ---------- */
.hcms-shell-notice {
  font-size: 12px;
  line-height: 1.45;
  white-space: pre-line;
  color: var(--mirk-mark-fg);
  background: var(--mirk-bevel-bg);
  border: 1px solid var(--mirk-input-border);
  padding: 8px 12px;
  margin: 0 24px 8px;
}
.hcms-shell-notice[hidden] { display: none; }

/* ---------- ERROR BANNER + inline errors ---------- */
.hcms-shell-error,
.hcms-error {
  font-size: 12px;
  line-height: 1.45;
  color: var(--mirk-destructive);
  background: var(--mirk-bevel-bg);
  border: 1px solid var(--mirk-destructive);
  padding: 8px 12px;
}
.hcms-shell-error { margin: 0 24px; }
.hcms-error { margin-top: 6px; }
.hcms-shell-error[hidden],
.hcms-error[hidden] { display: none; }

/* A note, not a refusal: the file WAS stored, in the page, and this says why it
   is not on the host. Same slot, so there is one place a field ever speaks. */
.hcms-error--info {
  color: var(--mirk-mark-fg);
  border-color: var(--mirk-input-border);
}

/* ---------- SAVE (sits at the end of the scrolling form, not pinned) ---------- */
.hcms-shell-footer {
  display: flex;
  justify-content: flex-end;
  padding: 4px 24px 28px;
}
.hcms-shell-footer[hidden] { display: none; }

/* ---------- sr-only move buttons: hidden, visible on keyboard focus ---------- */
.hcms-sr-only {
  position: absolute;
  width: 1px;
  height: 1px;
  padding: 0;
  margin: -1px;
  overflow: hidden;
  clip: rect(0, 0, 0, 0);
  white-space: nowrap;
  border: 0;
}
.hcms-sr-only:focus {
  position: static;
  width: auto;
  height: auto;
  padding: 2px 6px;
  margin: 0 2px;
  overflow: visible;
  clip: auto;
  white-space: normal;
  background: var(--mirk-bevel-bg);
  border: 1px solid var(--mirk-input-border);
  color: var(--mirk-fg);
  font-size: 12px;
  cursor: pointer;
}
.hcms-sr-only[hidden] { display: none; }

/* ============================================================
   INLINE VIEW GEOMETRY \u2014 not a panel. A transparent full-viewport
   host whose children are the only interactive surfaces, so the page
   underneath stays fully usable while the editor is open.
   ============================================================ */
.hcms-shell.pixel-quiet.hcms-inline {
  position: fixed;
  inset: 0;
  z-index: 2147483000;
  /* The host must never intercept a click meant for the page. Each child
     re-enables pointer events for itself. */
  pointer-events: none;
  background: none;
  border: 0;
  box-shadow: none;
  color: var(--mirk-fg);
}
.hcms-inline > *,
.hcms-inline-layer > * { pointer-events: auto; }
.hcms-inline-layer { position: absolute; inset: 0; pointer-events: none; }

.hcms-inline-bar {
  position: absolute;
  left: 50%;
  bottom: 16px;
  transform: translateX(-50%);
  max-width: min(560px, calc(100vw - 32px));
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.hcms-inline-notice,
.hcms-inline-error {
  padding: 8px 12px;
  border-radius: 8px;
  font-size: 14px;
  line-height: 1.4;
  border: 1px solid var(--mirk-input-border);
  background: var(--mirk-bg);
  color: var(--mirk-fg);
  box-shadow: 0 12px 32px -20px rgba(43, 36, 27, 0.5);
}
.hcms-inline-notice { white-space: pre-line; }
.hcms-inline-error { border-color: var(--mirk-destructive); color: var(--mirk-destructive); }
.hcms-inline-notice[hidden],
.hcms-inline-error[hidden] { display: none; }

.hcms-inline-pop {
  position: absolute;
  top: 0;
  left: 0;
  box-sizing: border-box;
  width: min(340px, calc(100vw - 24px));
  max-height: calc(100dvh - 24px);
  overflow: auto;
  padding: 16px;
  border-radius: 8px;
  border: 1px solid var(--mirk-input-border);
  background: var(--mirk-bg);
  color: var(--mirk-fg);
  box-shadow: 0 16px 40px -22px rgba(43, 36, 27, 0.55);
  /* Above the handles. They are positioned with z-index: 1 while the popover
     sat at auto, so a handle painted over the very field it had just opened
     and covered the text in it. The popover is what someone is using; a handle
     is only an offer to open one. */
  z-index: 2;
}
.hcms-inline-pop[hidden] { display: none; }
.hcms-inline-pop-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  margin-bottom: 16px;
  font-size: 14px;
}
.hcms-shell .hcms-inline-pop-close {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  min-width: 36px;
  min-height: 36px;
  padding: 4px;
}

.hcms-inline-item-controls {
  position: absolute;
  top: 0;
  left: 0;
  z-index: 1;
  will-change: transform;
  display: flex;
  align-items: center;
  gap: 2px;
}
.hcms-inline-item-controls[hidden],
.hcms-inline-handle[hidden] { display: none; }
.hcms-inline-item-controls.has-open-settings { z-index: 3; }
.hcms-inline-settings { position: relative; display: flex; flex: none; }
.hcms-inline-settings[hidden],
.hcms-inline-settings-menu[hidden] { display: none; }
.hcms-inline-settings-menu {
  position: absolute;
  top: calc(100% + 6px);
  right: 0;
  min-width: 120px;
  padding: 4px;
  border: 1px solid var(--mirk-input-border);
  border-radius: 5px;
  background: var(--mirk-bg);
  box-shadow: 0 8px 24px #0003;
}
.hcms-inline-settings-menu button {
  display: block;
  box-sizing: border-box;
  width: 100%;
  padding: 10px 12px;
  border: 0;
  background: transparent;
  color: var(--mirk-destructive);
  font: inherit;
  text-align: left;
  cursor: pointer;
}
.hcms-inline-settings-menu button:hover,
.hcms-inline-settings-menu button:focus-visible {
  background: var(--mirk-bevel-bg);
  outline: 1px solid var(--mirk-focus-color);
}

/* Buttons flow within the positioned item container. */
.hcms-inline-row-controls {
  display: flex;
  gap: 2px;
}
.hcms-inline-row-controls[hidden] { display: none; }

.hcms-inline-list-add[hidden] { display: none; }

.hcms-shell.hcms-inline-ghost {
  box-sizing: border-box;
  position: relative;
  display: flex;
  align-items: center;
  justify-content: center;
  flex: none;
  max-width: 100%;
  min-width: 0;
  margin: 8px 0 0;
  padding: 0;
  border: 1px dashed color-mix(in srgb, currentColor 25%, transparent);
  border-radius: 3px;
  background: color-mix(in srgb, currentColor 3%, transparent);
  color: inherit;
  list-style: none;
}
.hcms-shell.hcms-inline-ghost[hidden] { display: none; }
tr.hcms-shell.hcms-inline-ghost { display: table-row; }
.hcms-inline-ghost-cell { padding: 0; text-align: center; vertical-align: middle; }

.hcms-shell .hcms-inline-handle,
.hcms-shell .hcms-inline-list-button {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  min-width: 36px;
  min-height: 36px;
  padding: 4px;
  line-height: 1;
}
.hcms-inline-handle .mirk-button__label,
.hcms-inline-list-button .mirk-button__label {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  line-height: 1;
}
.hcms-inline-icon { display: block; width: 24px; height: 24px; flex: none; }
.hcms-inline-handle .hcms-inline-icon { width: 20.4px; height: 20.4px; transform: translateY(-1px); }
.hcms-inline-list-add .hcms-inline-icon { width: 16.8px; height: 16.8px; }
.hcms-shell .hcms-inline-list-add { padding-inline: 10px; }

.hcms-inline-list-button {
  flex: none;
}
.hcms-inline-list-button[hidden] { display: none; }
.hcms-shell .hcms-inline-list-button:disabled {
  opacity: 1;
  color: #626B96;
  background: var(--mirk-bevel-bg);
  border-color: var(--mirk-bevel-tl) var(--mirk-bevel-br) var(--mirk-bevel-br) var(--mirk-bevel-tl);
}
.hcms-shell .hcms-inline-list-button:disabled .mirk-button__label { translate: none; }

/* A finger is not a mouse pointer: on touch the same three controls get the
   44px target the platform guidelines ask for. */
@media (pointer: coarse) {
  .hcms-shell .hcms-inline-handle,
  .hcms-shell .hcms-inline-list-button {
    min-width: 44px;
    min-height: 44px;
  }
}

.hcms-inline-toggle { align-self: center; }
.hcms-inline-toggle[hidden] { display: none; }

/* Fields with no visible anchor can still be edited through the sidebar. */
.hcms-inline-handoff {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 6px 6px 6px 12px;
  border-radius: 999px;
  font-size: 13px;
  border: 1px solid var(--mirk-input-border);
  background: var(--mirk-bg);
  color: var(--mirk-fg);
  align-self: center;
}
.hcms-inline-handoff[hidden] { display: none; }

/* Selected fields only. Scoped to the inline host: [data-hcms-shell] is on BOTH
   hosts, so an unscoped rule hides the sidebar's whole form too. */
.hcms-inline .hcms-form [data-hcms-path] { display: none; }
.hcms-inline .hcms-form [data-hcms-path].is-hcms-inline-onpath,
.hcms-inline .hcms-form [data-hcms-path].is-hcms-inline-active { display: block; }
/* The Page style group has no data path of its own: hide it like a path until
   a reveal marks it on-path. */
.hcms-inline .hcms-form [data-hcms-page-style]:not(.is-hcms-inline-onpath) { display: none; }

.hcms-inline .hcms-form .is-hcms-inline-onpath,
.hcms-inline .hcms-form {
  margin: 0;
  padding: 0;
  border: 0;
  border-radius: 0;
  box-shadow: none;
  background: transparent;
  min-width: 0;
  min-height: 0;
  width: 100%;
}
.hcms-inline-pop .hcms-object-title,
.hcms-inline-pop .hcms-array-header,
.hcms-inline-pop .hcms-drag-handle,
.hcms-inline-pop [data-hcms-action="add"],
.hcms-inline-pop [data-hcms-action="remove"],
.hcms-inline-pop [data-hcms-action="move-up"],
.hcms-inline-pop [data-hcms-action="move-down"],
.hcms-inline-pop .hcms-card-controls { display: none; }
.hcms-inline .hcms-form .hcms-field.is-hcms-inline-active {
  display: flex;
  gap: 8px;
  padding: 0;
  margin: 0 0 16px;
  border: 0;
  min-width: min(260px, 100%);
}
.hcms-inline .hcms-form .hcms-field.is-hcms-inline-active:last-child { margin-bottom: 0; }
.hcms-shell.pixel-quiet .hcms-inline-pop .mirk-input,
.hcms-shell.pixel-quiet .hcms-inline-pop .mirk-textarea {
  box-sizing: border-box;
  width: 100%;
  min-width: min(260px, 100%);
  min-height: 42px;
  padding: 10px 12px;
  border: 1px solid var(--mirk-input-border);
  background: var(--mirk-input-bg);
}
.hcms-inline-pop .mirk-input:focus-visible,
.hcms-inline-pop .mirk-textarea:focus-visible {
  outline: 2px solid var(--mirk-focus-color);
  outline-offset: 2px;
}

.hcms-inline-highlight {
  position: absolute;
  top: 0;
  left: 0;
  pointer-events: none;
  border: 2px solid var(--mirk-focus-color);
  border-radius: 4px;
  box-sizing: border-box;
  will-change: transform;
}
.hcms-inline-highlight[hidden] { display: none; }
`;typeof window<"u"&&typeof document<"u"&&(function(){if(window.__mirk)return;window.__mirk=!0;let e='<svg viewBox="0 0 16 16" aria-hidden="true"><path d="M4 4 12 12M12 4 4 12" stroke="currentColor" stroke-width="2" stroke-linecap="square" fill="none"/></svg>';document.addEventListener("click",r=>{let i=r.target.closest(".mirk-number__step");if(!i)return;let l=i.closest(".mirk-number").querySelector("input[type=number]");l&&(i.dataset.step==="up"?l.stepUp():l.stepDown(),l.dispatchEvent(new Event("change",{bubbles:!0})))}),document.addEventListener("input",r=>{let i=r.target.closest(".mirk-slider__input");i&&i.closest(".mirk-slider").style.setProperty("--mirk-value",`${i.value}%`)}),document.addEventListener("change",r=>{let i=r.target.closest(".mirk-file__input");if(!i||!i.files.length)return;let l=i.closest(".mirk-file"),n=l.querySelector(".mirk-file__name");if(!n)return;let a=i.files[0],o=document.createElement("a");if(o.className="mirk-file__name",o.dataset.filled="",o.href=URL.createObjectURL(a),o.target="_blank",o.rel="noopener",o.textContent=a.name,n.replaceWith(o),!l.querySelector(".mirk-file__remove")){let s=document.createElement("button");s.type="button",s.className="mirk-file__remove",s.setAttribute("aria-label","Remove file"),s.innerHTML=e,o.after(s)}}),document.addEventListener("change",r=>{let i=r.target.closest(".mirk-image__input");if(!i||!i.files.length)return;let l=i.closest(".mirk-image"),n=l.querySelector(".mirk-image__preview");if(!n)return;let a=l.querySelector(".mirk-image__placeholder"),o=new FileReader;o.onload=s=>{n.src=s.target.result,n.removeAttribute("hidden"),a&&a.setAttribute("hidden",""),l.querySelector(".mirk-image__thumb")?.removeAttribute("hidden"),l.querySelector(".mirk-image__upload")?.setAttribute("hidden","")},o.readAsDataURL(i.files[0])}),document.addEventListener("click",r=>{let i=r.target.closest(".mirk-file__remove");if(i){let n=i.closest(".mirk-file"),a=n?.querySelector(".mirk-file__input"),o=n?.querySelector(".mirk-file__name");if(a&&(a.value=""),o){let s=document.createElement("span");s.className="mirk-file__name",s.textContent="No file chosen",o.replaceWith(s)}i.remove();return}let l=r.target.closest(".mirk-image__remove");if(l){let n=l.closest(".mirk-image"),a=n?.querySelector(".mirk-image__input"),o=n?.querySelector(".mirk-image__preview");a&&(a.value=""),o&&(o.removeAttribute("src"),o.setAttribute("hidden","")),n?.querySelector(".mirk-image__thumb")?.setAttribute("hidden",""),n?.querySelector(".mirk-image__upload")?.removeAttribute("hidden")}});function t(r,i){let l=document.createElement("span");l.textContent=r;let n=document.createElement("input");n.type="hidden",n.name="tags[]",n.value=r;let a=document.createElement("button");a.type="button",a.className="mirk-tags__remove",a.textContent="\xD7";let o=document.createElement("span");if(o.className="mirk-tags__chip",i){let s=document.createElement("span");s.className="mirk-tags__chip-inner",s.append(l,n,a),o.append(s)}else o.append(l,n,a);return o}document.addEventListener("keydown",r=>{let i=r.target.closest(".mirk-tags__input");if(!i)return;let l=i.closest(".mirk-tags");if(r.key==="Enter"||r.key===","){let n=i.value.trim();if(!n)return;r.preventDefault(),i.before(t(n,l.classList.contains("mirk-tags--round"))),i.value=""}else if(r.key==="Backspace"&&!i.value){let n=l.querySelectorAll(".mirk-tags__chip");n[n.length-1]?.remove()}}),document.addEventListener("click",r=>{let i=r.target.closest(".mirk-tags__remove");if(i){i.closest(".mirk-tags__chip").remove();return}let l=r.target.closest(".mirk-tags");l&&r.target===l&&l.querySelector(".mirk-tags__input")?.focus()}),document.addEventListener("click",r=>{let i=r.target.closest("[data-mirk-chip]");if(!i)return;let l=i.getAttribute("data-mirk-chip");if(l==="open")i.closest(".mirk-chip")?.classList.add("mirk-chip--open");else if(l==="collapse")i.closest(".mirk-chip")?.classList.remove("mirk-chip--open");else if(l==="changes"){let n=i.closest(".mirk-chip__panel")?.classList.toggle("is-changes");i.textContent=n?"(hide changes)":"(view changes)"}}),document.addEventListener("click",r=>{let i=r.target.closest("[data-copy-btn]");if(!i)return;let l=i.closest("[data-copy]");if(!l)return;let n=l.cloneNode(!0);n.querySelectorAll("[data-copy-btn]").forEach(o=>o.remove());let a=l.getAttribute("data-copy")==="text"?n.textContent.replace(/^\s+|\s+$/g,""):n.innerHTML.replace(/\s+data-copy(="[^"]*")?/g,"").replace(/^\s*\n/gm,"").trim();navigator.clipboard.writeText(a).then(()=>{let o=i.textContent;i.textContent="copied",i.dataset.copied="",setTimeout(()=>{i.textContent=o,delete i.dataset.copied},1200)}).catch(()=>{i.textContent="error",setTimeout(()=>{i.textContent="copy"},1200)})})})();xh($h);var Dr=Dn,Lh={cms:Dn};typeof window<"u"&&!window.__hyperclayNoAutoExport&&(window.hyperclay=window.hyperclay||{},window.hyperclay.hypercms=Dr,window.h=window.hyperclay);var Ce=Dr,jh=Vi;Xr("cms",Kr);var Tt=(e,t)=>Object.assign(new Error(`clay:cms: ${e}`),{code:t}),Mh=e=>e?e.signal?.aborted?!0:typeof e.isCurrent=="function"&&!e.isCurrent():!1;function Bn(e){let t=!1;return{ready:e.ready,enable(){t||e.enable()},quiesce(){if(t)return;let r=e.quiesce();if(r&&r.ok===!1)throw Object.assign(Tt("the CMS still has work to finish",r.code||"pending-input"),{pending:r.pending});return r},resume(){t||e.resume()},dispose(){t||(t=!0,e.dispose())}}}function Nh(){if(Ce.isOpen)throw Tt("the CMS this boot opened cannot be taken back","unsupported-plugin");let e=Bn(Ce.prepareModeLease({}));return{enable(){},quiesce:()=>e.quiesce(),resume:()=>e.resume(),dispose:()=>e.dispose()}}function cd(e={}){return{id:"clay:cms",order:1e3,async prepare(t={}){if(typeof Ce.prepareModeLease!="function")throw Tt("this vendor build has no quiescable session","unsupported-plugin");if(Mh(t))throw Tt("prepared for a stale generation","stale-document");return t.initial===!0?Nh():Bn(Ce.prepareModeLease(e))}}}export{Ce as cms,cd as cmsParticipant,jh as default,cd as modeParticipant};
