var kl=Object.defineProperty;var Tr=(e,t)=>{for(var r in t)kl(e,r,{get:t[r],enumerable:!0})};var Wn={};Tr(Wn,{cms:()=>_r,default:()=>id});var Lt=["textContent","innerText","innerHTML","outerHTML","value","checked","selected","disabled","readOnly","type","tagName","nodeName","nodeType","nodeValue","childElementCount","id","className","classList","baseURI","offsetWidth","offsetHeight","clientWidth","clientHeight","scrollWidth","scrollHeight","dataset","currentSrc","duration","paused","title","documentURI","contentType"],st=new Set(Lt),Or=new Set(["textContent","innerText","innerHTML","value","checked","selected","disabled","readOnly","type","id","className","title"]),Ae=new Set(["tagName","nodeName","nodeType","nodeValue","childElementCount","classList","baseURI","documentURI","contentType","offsetWidth","offsetHeight","clientWidth","clientHeight","scrollWidth","scrollHeight","currentSrc","duration","paused","dataset"]),ro=new Set(["checked","selected","disabled","readOnly","paused"]);function Rr(e,t){return t==null?ro.has(e)?!1:"":ro.has(e)?t==="false"?!1:!!t:t}var jt={};Tr(jt,{CodecRefused:()=>Mr,EmptyListInsert:()=>he,MAX_RULE_DEPTH:()=>me,MaxRuleDepthExceeded:()=>Se,NoRulesTag:()=>Lr,RuleTargetReadOnly:()=>at,RulesParseError:()=>ee,ShapeMismatch:()=>lt,UnknownCodec:()=>ct,UnknownRulesVersion:()=>qe,WriteNotRoundTripped:()=>Mt,WriteRefused:()=>Nt,WriteRejected:()=>Nr});var ee=class extends Error{constructor(t,r){super(t),this.name="RulesParseError",this.cause=r}},qe=class extends Error{constructor(t){super(`unknown rules version: ${t}. Library supports "1" and "2".`),this.name="UnknownRulesVersion",this.version=t}},me=20,Se=class extends Error{constructor(t){super(`rule depth exceeded ${me} at path: ${t.join(".")}`),this.name="MaxRuleDepthExceeded",this.path=t}},lt=class extends Error{constructor(t){super(`shape mismatch: ${t.length} field(s) failed validation`),this.name="ShapeMismatch",this.mismatches=t}},he=class extends Error{constructor(t,r){super(r?`cannot add items to empty list at "${t.join(".")}" \u2014 ${r}`:`cannot add items to empty list at "${t.join(".")}" \u2014 no sibling to clone as template. Seed the list with a hidden item first.`),this.name="EmptyListInsert",this.path=t}},at=class extends Error{constructor(t){super(`cannot write to read-only DOM property "${t}"`),this.name="RuleTargetReadOnly",this.target=t}},Nt=class extends Error{constructor(t){super(`write refused: ${t.length} write(s) broke the content-only policy`),this.name="WriteRefused",this.refusals=t}};function yl(e){return e.reduce((r,n)=>typeof n=="number"?`${r}[${n}]`:r?`${r}.${n}`:String(n),"")||"(root)"}var Mt=class extends Error{constructor(t){super(`write did not round-trip at "${yl(t)}"`),this.name="WriteNotRoundTripped",this.path=t}},Lr=class extends Error{constructor(t){super(`no <script data-rules-name~="${t}"> rules tag in this document`),this.name="NoRulesTag",this.token=t}},Nr=class extends Error{constructor(t,r){super(`write rejected: ${t.length} unknown key(s), ${r.length} rule(s) with no matching element`),this.name="WriteRejected",this.unknownKeys=t,this.unmatched=r}},ct=class extends Error{constructor(t){super(`unknown codec "${t}": no codec by that name is given to this call`),this.name="UnknownCodec",this.code="unknown-codec",this.codec=t}},Mr=class extends Error{constructor(t,r){super(r),this.name="CodecRefused",this.code=t}};function j(e){let t=0,r=null,n=-1;for(let o=0;o<e.length;o++){let i=e[o];i==="\\"?o++:r?i===r&&(r=null):i==='"'||i==="'"?r=i:i==="["||i==="("?t++:i==="]"||i===")"?t>0&&t--:i==="@"&&t===0&&(n=o)}return n}var no="[*]";function K(e){let t=j(e),r=t===-1?e:e.slice(0,t),n=r.endsWith(no),o={selector:n?r.slice(0,-no.length):r,prop:t===-1?null:e.slice(t+1)||null};return n&&(o.all=!0),o}function jr(e){let t=0,r=null,n=-1,o=!1;for(let i=0;i<e.length;i++){let s=e[i];s==="\\"?i++:r?s===r&&(r=null):s==='"'||s==="'"?r=s:s==="["||s==="("?t++:s==="]"||s===")"?t>0&&t--:t===0&&s===">"?n=i:t===0&&s===","&&(o=!0)}return o||n===-1||!vl(e,n+1)?null:e.slice(0,n).trim()||null}function vl(e,t){let r=t,n=e.replace(/\s+$/,"").length;for(;r<n&&/\s/.test(e[r]);)r++;let o=0,i=null;for(;r<n;r++){let s=e[r];if(s==="\\")r++;else if(i)s===i&&(i=null);else if(s==='"'||s==="'")i=s;else if(s==="["||s==="(")o++;else if(s==="]"||s===")")o>0&&o--;else if(o===0&&(s==="+"||s==="~"||/\s/.test(s)))return!1}return!0}var xl=/^codec\(([a-z][a-z0-9-]*)\)$/;function fe(e){if(typeof e!="string")return null;let t=j(e);if(t===-1)return null;let r=xl.exec(e.slice(t+1));if(!r)return null;let{selector:n,all:o}=K(e);if(o)throw new ee("A @codec(name) rule cannot use [*].");return{selector:n,name:r[1]}}import Ed from"./hyper-morph.vendor.js";var Cl=/^--[A-Za-z0-9_-]+$/;function Ee(e){return typeof e=="string"&&Cl.test(e)}function io(e){let t=[],r=[],n=null,o=0;for(let i=0;i<e.length;i++){let s=e[i];if(n){s==="\\"?i++:s===n&&(n=null);continue}if(s==="/"&&e[i+1]==="*"){let l=e.indexOf("*/",i+2);i=l===-1?e.length:l+1;continue}s==='"'||s==="'"?n=s:s==="("?r.push(")"):s==="["?r.push("]"):s==="{"?r.push("}"):(s===")"||s==="]"||s==="}")&&r[r.length-1]===s?r.pop():s===";"&&r.length===0&&(t.push(e.slice(o,i)),o=i+1)}return t.push(e.slice(o)),t}function Tl(e){let t="",r=null;for(let n=0;n<e.length;n++){let o=e[n];if(r){t+=o,o==="\\"&&n+1<e.length?t+=e[++n]:o===r&&(r=null);continue}if(o==="/"&&e[n+1]==="*"){let i=e.indexOf("*/",n+2);n=i===-1?e.length:i+1,t+=" ";continue}(o==='"'||o==="'")&&(r=o),t+=o}return t}function so(e){e=Tl(e);let t=e.indexOf(":");if(t===-1)return null;let r=e.slice(0,t).trim(),n=e.slice(t+1).trim(),o=/!\s*important\s*$/i.exec(n);return o&&(n=n.slice(0,o.index).trim()),{name:r,value:n,important:!!o}}function ut(e,t){if(e==null)return null;let r=null;for(let n of io(String(e))){let o=so(n);!o||o.name!==t||(!r||o.important||!r.important)&&(r=o)}return r?r.value:null}function lo(e,t,r){let n=e==null?"":String(e),o=r==null||r==="",i=io(n),s=i.map(so),l=s.some(f=>f&&f.name===t&&f.important),a=[],u=!1;i.forEach((f,h)=>{let m=s[h];if(!m||m.name!==t){a.push(f);return}if(o||u)return;let c=/^\s*/.exec(f)[0];a.push(`${c}${t}: ${r}${l?" !important":""}`),u=!0});let d=a.join(";");if(!o&&!u){let f=d.trimEnd();d=f===""?`${t}: ${r}`:`${f}${f.endsWith(";")?" ":"; "}${t}: ${r}`}return d.trim()===""?null:d}function qr(e,t,r,n){let o=e.attr(t,"style");if((ut(o,r)??"")===n)return;let i=lo(o,r,n);i==null?e.removeAttr(t,"style"):e.attr(t,"style",i)}var ao=/^[a-z][a-z0-9-]*$/,Pr=new Map;function Ir(e,t){if(typeof e!="string"||!ao.test(e))throw new Error(`hyper-html-api: invalid codec name ${JSON.stringify(e)} (must match ${ao})`);if(!t||typeof t.read!="function"||typeof t.write!="function"||typeof t.same!="function")throw new Error(`hyper-html-api: codec "${e}" needs read, write and same functions`);Pr.set(e,t)}function $r(e){return Pr.get(e)??null}function Fr(){return Object.fromEntries(Pr)}var Ol='The @codec(name) rule form needs a rules tag with data-rules-version="2".',Rl="A @codec(name) rule must be the value of a top level key.";function qt(e,t){let r=fe(e);return r?{...r,codec:t.codecs[r.name]}:null}function Pe(e,t={}){let r=t.version??"1",n=t.codecs??{};o(e,0);function o(i,s){if(!(s>me)){if(typeof i=="string"){let l=fe(i);if(!l)return;if(r!=="2")throw new ee(Ol);if(s!==1)throw new ee(Rl);if(!Object.prototype.hasOwnProperty.call(n,l.name))throw new ct(l.name);return}if(Array.isArray(i)){o(i[1],s+1);return}if(i&&typeof i=="object")for(let l of Object.values(i))o(l,s+1)}}}function pe(e,t,r,n={}){if(Object.prototype.hasOwnProperty.call(n,"exclude")&&e.semanticExclude===!1)throw new Error("This adapter does not support semantic exclude queries; omit exclude or use the DOM adapter.");return Pe(r,n),zr(e,t,r,{depth:0,path:[]},n)}function zr(e,t,r,n,o){if(n.depth>me)throw new Se(n.path);if(typeof r=="string")return Ll(e,t,r,n,o);if(Array.isArray(r)){let[i,s]=r,l=e.find(t,i,o);return co(o,n,l),l.map((a,u)=>zr(e,a,s,{depth:n.depth+1,path:[...n.path,u]},o))}if(typeof r=="object"&&r!==null){let i={};for(let[s,l]of Object.entries(r))i[s]=zr(e,t,l,{depth:n.depth+1,path:[...n.path,s]},o);return i}return null}function Ll(e,t,r,n,o){if(r.endsWith("[]")){let d=r.slice(0,-2),f=e.find(t,d,o);return co(o,n,f),f.map(h=>e.text(h))}let i=qt(r,o);if(i)return Nl(e,t,i,o);if(r.startsWith("@"))return Dr(e,t,r.slice(1));let s=j(r);if(s!==-1){let{selector:d,all:f}=K(r),h=r.slice(s+1),m=d?e.find(t,d,o):[t];return f&&o.allMatches?m.map(c=>Dr(e,c,h)):m.length===0?null:Dr(e,m[0],h)}if(r===".")return e.text(t);let{selector:l,all:a}=K(r),u=e.find(t,l,o);return a&&o.allMatches?u.map(d=>e.text(d)):u.length===0?null:e.text(u[0])}function Nl(e,t,{selector:r,codec:n},o){let i=r?e.find(t,r,o):[t];return i.length===0?null:n.read(e,i[0],{query:o.query,view:o.view,region:o.region})}function co(e,t,r){if(typeof e.onRowsRead=="function")try{e.onRowsRead(t.path.slice(),r)}catch(n){console.warn(`[hyper-html-api] onRowsRead threw at "${t.path.join(".")||"(root)"}"`,n)}}function Dr(e,t,r){if(Ee(r))return ut(e.attr(t,"style"),r);if(st.has(r)){let o=e.prop(t,r);return o==null?null:String(o)}let n=e.attr(t,r);return n??null}var Pt=Object.freeze({data:Object.freeze({tokens:["no-data"],bundles:["editor-ui"]}),save:Object.freeze({tokens:["no-save"],bundles:["editor-ui"]}),snapshot:Object.freeze({tokens:["no-snapshot"],bundles:["editor-ui"]}),watch:Object.freeze({tokens:["no-watch"],bundles:["editor-ui"]}),undo:Object.freeze({tokens:["no-undo"],bundles:["editor-ui"]}),history:Object.freeze({tokens:[],bundles:["editor-ui"]})}),Ml=Object.freeze({"editor-ui":Object.freeze(["no-data","no-save","no-snapshot","no-watch","no-undo"])}),uo=Object.freeze(["no-save","no-snapshot","no-trigger-autosave","no-dirty","no-watch","no-undo","no-data","freeze","editor-ui","live"]);function dt(e,t){if(!e||e.nodeType!==1)return!1;let r=e.getAttribute?.("clay");return!!(r&&r.split(/\s+/).includes(t)||e.hasAttribute?.(t))}function Ur(e,t){if(dt(e,t))return!0;for(let[r,n]of Object.entries(Ml))if(n.includes(t)&&dt(e,r))return!0;return!1}function ht(e){let t=Pt[e];if(!t)throw new Error(`Unknown region capability: ${e}`);return[...t.tokens,...t.bundles].flatMap(r=>[`[clay~="${r}"]`,`[${r}]`]).join(", ")}function jl(e,t){let r=e&&e.nodeType===1?e:e?.parentElement;for(;r&&r.nodeType===1;){let n=Pt[t];if(!n)throw new Error(`Unknown region capability: ${t}`);if(n.tokens.some(o=>Ur(r,o))||n.bundles.some(o=>dt(r,o)))return r;r=r.parentElement}return null}function ho(e,t){return!!jl(e,t)}var ql=/:(?:focus(?:-within|-visible)?|hover|active|visited|defined)\b/i;function Pl(e,t){if(!/^(INPUT|TEXTAREA|SELECT|OPTION)$/.test(e.tagName||""))return;let r=(e.getAttribute?.("type")||"").toLowerCase();"value"in e&&"value"in t&&e.tagName!=="OPTION"&&r!=="checkbox"&&r!=="radio"&&r!=="file"&&(t.value=e.value),e.tagName==="INPUT"&&(r==="checkbox"||r==="radio")&&(t.checked=e.checked),e.tagName==="OPTION"&&(t.selected=e.selected),"indeterminate"in e&&"indeterminate"in t&&(t.indeterminate=e.indeterminate)}function mo(e,t,r,n){if(n)return!!e.closest?.(t);let o=e;for(;o?.nodeType===1;){if(o.matches(t))return!0;if(o===r)break;o=o.parentElement}return!1}function fo(e,t,r,n,o,i){return(i?ho(e,t):mo(e,r,o,!1))||!!(n&&(i&&e!==o?e.matches(n):mo(e,n,o,i)))}function Il(e,t){let r=Pt[t];return r.tokens.some(n=>Ur(e,n))||r.bundles.some(n=>dt(e,n))}var $l=new Set(["INPUT","TEXTAREA","SELECT","OPTION"]);function po(e,t,r,n){if(e.nodeType===1&&n(e)){r.dropped.push(t);return}r.cloneToLive.set(t,e),r.liveToClone.set(e,t);let o=e,i=t;if(e.nodeType===1){if($l.has(e.tagName)&&r.controls.push(e,t),e.tagName==="TEMPLATE"){for(;t.firstChild;)t.removeChild(t.firstChild);o=e.content,i=t.content,r.cloneToLive.set(i,o),r.liveToClone.set(o,i)}}else if(e.nodeType!==11)return;let s=i.firstChild;for(let l=o.firstChild;l&&s;l=l.nextSibling)po(l,s,r,n),s=s.nextSibling}function mt(e,{capability:t="data",exclude:r=null,inherit:n=!0,provenance:o=null}={}){if(!e)throw new TypeError("createContentView requires a DOM context");let i=e.nodeType===9?e:e.ownerDocument;if(!i?.implementation?.createHTMLDocument)throw new TypeError("createContentView requires an HTML DOM implementation");let s=i.implementation.createHTMLDocument(""),l=o||new WeakMap,a=new WeakMap,u=e.nodeType===9?e.documentElement:e;r&&s.documentElement.matches(r);let d=ht(t),f=n?c=>Il(c,t)||!!(r&&c.matches(r)):c=>fo(c,t,d,r,u,!1),h=null;if(u.nodeType!==1||!fo(u,t,d,r,u,n)){h=s.importNode(u,!0);let c={cloneToLive:l,liveToClone:a,dropped:[],controls:[]};po(u,h,c,f);for(let p of c.dropped)p.remove();for(let p=0;p<c.controls.length;p+=2)Pl(c.controls[p],c.controls[p+1])}e.nodeType===9&&h&&(s.replaceChild(h,s.documentElement),a.set(e,s),l.set(s,e));let m=c=>{if(ql.test(c))throw new Error(`Filtered content queries do not support stateful selector: ${c}`)};return{root:h,document:s,capability:t,selector:ht(t),cloneToLive:l,liveToClone:a,original(c){return l.get(c)||null},cloneOf(c){return a.get(c)||null},query(c,p=h){return m(c),Array.from(p.querySelectorAll(c),b=>l.get(b)||b)},text(c=h){return(c===h?h:a.get(c))?.textContent||""},html(c=h){return(c===h?h:a.get(c))?.innerHTML??""},clone(c=h){let p=c===h?h:a.get(c);return p?s.importNode(p,!0):null}}}function bo(e){return e&&e.nodeType===1&&e.tagName==="SCRIPT"&&e.hasAttribute&&e.hasAttribute("data-rules-name")}function Fl(e){return e?(e.nodeType===9||e.nodeType===11,e):null}function go(e,t,r){!e||e.nodeType!==1||(t==="value"&&e.tagName==="INPUT"?e.value=r===null?"":r:t==="checked"&&e.tagName==="INPUT"?e.checked=r!==null:t==="selected"&&e.tagName==="OPTION"&&(e.selected=r!==null))}function ko(e,t){e&&e.tagName==="TEXTAREA"&&(e.value=t)}function yo(e){if(!e||e.nodeType!==1)return;let t=[e,...e.querySelectorAll?e.querySelectorAll("textarea, input, option"):[]];for(let r of t)if(r.tagName==="TEXTAREA")r.value=r.defaultValue;else if(r.tagName==="INPUT"){let n=r.type;n!=="file"&&n!=="checkbox"&&n!=="radio"&&(r.value=r.defaultValue),r.checked=r.defaultChecked}else r.tagName==="OPTION"&&(r.selected=r.defaultSelected)}var $={semanticExclude:!0,find(e,t,r={}){let n=Fl(e);if(!n||!n.querySelectorAll)return[];let o=Array.from(n.querySelectorAll(t));r.includeRulesTag||(o=o.filter(l=>!bo(l)));let i=[];r.skip&&i.push(r.skip);let s=r.templateAttr===null?null:r.templateAttr||"cms-template";if(s&&i.push("["+s+"]"),i.length){let l=i.join(", ");o=o.filter(a=>!a.closest||!a.closest(l))}return o},parent(e){return e?e.parentElement:null},children(e){return e?Array.from(e.children):[]},matches(e,t){return!!(e&&e.matches)&&e.matches(t)},text(e,t){if(t===void 0)return(e.textContent||"").trim();e.textContent=t,ko(e,t)},attr(e,t,r){if(r===void 0)return e.hasAttribute&&e.hasAttribute(t)?e.getAttribute(t):null;e.setAttribute(t,r),go(e,t,r)},removeAttr(e,t){e&&e.removeAttribute&&e.removeAttribute(t),go(e,t,null)},prop(e,t,r){if(r===void 0){let n=e?e[t]:void 0;return n!==void 0?n:null}e[t]=r},clone(e){let t=e.cloneNode(!0);return yo(t),t},insertAt(e,t,r){let n=e.children[r]||null;e.insertBefore(t,n)},insertBefore(e,t){e.parentNode.insertBefore(t,e)},insertAfter(e,t){e.parentNode.insertBefore(t,e.nextSibling)},identity(e){return e||null},remove(e){e&&e.parentNode&&e.parentNode.removeChild(e)},replaceWith(e,t){if(!e||!e.parentNode)throw new Error("dom.replaceWith: node has no parent");let n=e.ownerDocument.createElement("template");n.innerHTML=t;let o=n.content.firstElementChild;if(!o)throw new Error("dom.replaceWith: html did not parse to an element");return e.parentNode.replaceChild(o,e),o},stripIds(e){let t=0;return e.id&&(e.removeAttribute("id"),t++),(e.querySelectorAll?e.querySelectorAll("[id]"):[]).forEach(n=>{n.removeAttribute("id"),t++}),t},sameNode(e,t){return e===t}};function ie(e,t={}){if(t.exclude===null)return $;let r=[ht("data"),t.exclude].filter(Boolean).join(", "),n=e.nodeType===9?e:e.ownerDocument||e,o=e.nodeType===9?e.documentElement:e;if(!t._write&&!o?.matches?.(r)&&!o?.querySelector?.(r)&&!n?.querySelector?.(r))return $;let i=e.nodeType===9||!e.isConnected?e:e.ownerDocument,s=e.nodeType===9?e:e.ownerDocument,l=null,a=()=>l||(l=mt(i,{capability:"data",exclude:t.exclude||null})),u=c=>{let p=a();return c===e?p.cloneOf(e):p.cloneOf(c)||c},d=c=>l?.original(c)||c,f=(c,p)=>{a().liveToClone.set(c,p),a().cloneToLive.set(p,c);let b=c.childNodes||[],g=p.childNodes||[];for(let k=0;k<Math.min(b.length,g.length);k++)f(b[k],g[k])},h=(c,p,b,g)=>{let k=Array.from(c.childNodes).filter(O=>l.cloneOf(O)),A=k[0];for(;A&&k.includes(A);)A=A.nextSibling;for(let O of k)O.remove();if(b==="innerHTML"){let O=s.createElement("template");O.innerHTML=g;for(let B of Array.from(O.content.childNodes))c.insertBefore(B,A||null)}else c.insertBefore(s.createTextNode(g),A||null),ko(c,g);let S=mt(c,{capability:"data",exclude:t.exclude||null});p.replaceChildren(...Array.from(S.root?.childNodes||[]));let E=O=>{let B=S.original(O);B&&(l.liveToClone.set(B,O),l.cloneToLive.set(O,B));for(let de of Array.from(O.childNodes||[]))E(de)};for(let O of Array.from(p.childNodes))E(O)},m={...$,find(c,p,b={}){let g=u(c);if(!g?.querySelectorAll)return[];let k=a().query(p,g);b.includeRulesTag||(k=k.filter(S=>!bo(S)));let v=[];b.skip&&v.push(b.skip);let A=b.templateAttr===null?null:b.templateAttr||"cms-template";if(A&&v.push(`[${A}]`),v.length){let S=v.join(", ");k=k.filter(E=>!E.closest?.(S))}return k},parent(c){let p=u(c)?.parentElement;return p?d(p):null},children(c){return Array.from(u(c)?.children||[],d)},text(c,p){if(p===void 0){let k=d(c);return!k?.matches?.(r)&&!k?.querySelector?.(r)?$.text(k):(u(c)?.textContent||"").trim()}let b=d(c);if(!b?.matches?.(r)&&!b?.querySelector?.(r)&&!l){$.text(b,p);return}let g=a().cloneOf(b);g?h(b,g,"textContent",p):$.text(b,p)},attr(c,p,b){let g=d(c);if(b===void 0)return $.attr(g,p);$.attr(g,p,b);let k=l?.cloneOf(g);k&&($.attr(k,p,b),mt(g,{capability:"data",exclude:t.exclude||null}).root===null&&(k.remove(),l.liveToClone.delete(g),l.cloneToLive.delete(k)))},removeAttr(c,p){let b=d(c);$.removeAttr(b,p);let g=l?.cloneOf(b);g&&$.removeAttr(g,p)},prop(c,p,b){let g=d(c);if(b===void 0)return p==="innerHTML"||p==="outerHTML"||p==="textContent"||p==="innerText"?$.prop(u(g),p):$.prop(g,p);let k=p==="innerHTML"||p==="textContent"||p==="innerText",v=k?a().cloneOf(g):l?.cloneOf(g);v&&k?h(g,v,p,b):($.prop(g,p,b),v&&$.prop(v,p,b))},clone(c){let p=u(c);if(p===c)return $.clone(c);let b=s.importNode(p,!0);return yo(b),b},insertAt(c,p,b){let g=d(c),v=m.children(g)[b]||(()=>{for(let E=g.children.length-1;E>=0;E--){let O=g.children[E];if(a().cloneOf(O))return O.nextElementSibling}return g.firstElementChild})(),A=d(p);g.insertBefore(A,v||null);let S=a().cloneOf(g);if(S){let E=a().cloneOf(A);E||(E=s.importNode(A,!0),f(A,E));let O=Array.from(S.children);S.insertBefore(E,O[b]||null)}},insertBefore(c,p){let b=d(c),g=d(p);b.parentNode.insertBefore(g,b);let k=a().cloneOf(b.parentNode);if(k){let v=a().cloneOf(g);v||(v=s.importNode(g,!0),f(g,v));let A=a().cloneOf(b);A?A.before(v):k.appendChild(v)}},insertAfter(c,p){let b=d(c),g=d(p);b.parentNode.insertBefore(g,b.nextSibling);let k=a().cloneOf(b.parentNode);if(k){let v=a().cloneOf(g);v||(v=s.importNode(g,!0),f(g,v));let A=a().cloneOf(b);A?A.after(v):k.appendChild(v)}},identity(c){return d(c)||null},remove(c){let p=d(c),b=a().cloneOf(p);$.remove(p),b&&$.remove(b)},replaceWith(c,p){let b=d(c),g=a().cloneOf(b),k=$.replaceWith(b,p);if(g){let v=mt(k,{capability:"data",exclude:t.exclude||null});if(!v.root)g.remove();else{g.replaceWith(v.root);let A=S=>{let E=v.original(S);E&&(l.liveToClone.set(E,S),l.cloneToLive.set(S,E));for(let O of Array.from(S.childNodes||[]))A(O)};A(v.root)}}return k}};return m}var Ie=$;function Br(e){try{return JSON.parse(e)}catch(t){throw new ee(`Invalid strict JSON: ${t.message}`,t)}}function $e(e){try{return JSON.parse(e)}catch{}let t={BRACE_OPEN:"{",BRACE_CLOSE:"}",BRACKET_OPEN:"[",BRACKET_CLOSE:"]",COLON:":",COMMA:",",STRING:"STRING",SELECTOR:"SELECTOR",IDENTIFIER:"IDENTIFIER",NUMBER:"NUMBER",BOOLEAN:"BOOLEAN"};function r(o){let i=[],s=0;for(;s<o.length;){let l=o[s];if(/\s/.test(l)){s++;continue}if("{}".includes(l)){i.push({type:l,value:l}),s++;continue}if(l==="["){let f=!1,h=s+1;for(;h<o.length&&/\s/.test(o[h]);)h++;if(h<o.length&&/[a-zA-Z_]/.test(o[h])&&(f=!0),!f){i.push({type:l,value:l}),s++;continue}}if(l==="]"){i.push({type:l,value:l}),s++;continue}if(l===":"){i.push({type:t.COLON,value:l}),s++;continue}if(l===","){i.push({type:t.COMMA,value:l}),s++;continue}if(l==='"'||l==="'"){let f=l,h=s+1;for(;h<o.length&&o[h]!==f;)o[h]==="\\"&&h++,h++;i.push({type:t.STRING,value:o.substring(s+1,h),quoted:!0,sourceQuote:f}),s=h+1;continue}let a=s,u;for(;a<o.length&&!/[{},]/.test(o[a]);)if(o[a]===":"){let f=[":first",":last",":nth-child",":nth-of-type",":first-child",":last-child",":first-of-type",":last-of-type",":only-child",":only-of-type",":hover",":focus",":active",":visited",":disabled",":enabled",":checked",":empty",":root",":target",":not",":before",":after",":nth-last-child",":nth-last-of-type"],h=!1;for(let m of f){let c=m.substring(1);if(o.substring(a+1,a+1+c.length)===c){h=!0,a+=c.length;break}}if(!h)break}else if(o[a]==="["){for(a++;a<o.length&&o[a]!=="]";){if(o[a]==='"'||o[a]==="'"){let f=o[a];for(a++;a<o.length&&o[a]!==f;)o[a]==="\\"&&a++,a++}a++}a<o.length&&o[a]==="]"&&a++}else a++;u=o.substring(s,a);let d=t.IDENTIFIER;/^-?\d+(\.\d+)?$/.test(u)?d=t.NUMBER:u==="true"||u==="false"||u==="null"?d=t.BOOLEAN:/^[.#@\[]|[.#@\[]| /.test(u)&&(d=t.SELECTOR),i.push({type:d,value:u,quoted:!1}),s=a}return i}function n(o){let i="";for(let s=0;s<o.length;s++){let l=o[s];if("{}".includes(l.type)||"[]".includes(l.type)){i+=l.value;continue}if(l.type===t.COLON){i+=l.value;continue}if(l.type===t.COMMA){let a=o[s+1];if(a&&(a.type==="}"||a.type==="]"))continue;i+=l.value;continue}if(l.type===t.STRING&&l.quoted){let a=l.value;l.sourceQuote==="'"&&(a=a.replace(/\\'/g,"'"),a=a.replace(/(\\*)"/g,(u,d)=>d.length%2===0?d+'\\"':u)),i+=`"${a}"`;continue}if(l.type===t.NUMBER||l.type===t.BOOLEAN){i+=l.value;continue}if(l.type===t.SELECTOR||l.type===t.IDENTIFIER){i+=`"${l.value}"`;continue}i+=`"${l.value}"`}return i}try{let o=r(e),i=n(o);return JSON.parse(i)}catch(o){throw new ee("Invalid extraction rules syntax: "+o.message,o)}}var Hr=["1","2"],vo=/^[a-zA-Z0-9_-]+$/;function ft(e,t,r){let n;if(r===void 0)n="script[data-rules-name]";else{if(typeof r!="string"||!vo.test(r))throw new Error(`hyper-html-api: invalid rules token ${JSON.stringify(r)} (must match ${vo})`);n=`script[data-rules-name~="${r}"]`}let o=e.find(t,n,{includeRulesTag:!0});if(o.length===0)return null;r!==void 0&&o.length>1&&console.warn(`hyper-html-api: ${o.length} rules tags match data-rules-name~="${r}"; using the first.`);let i=o[0],s=e.attr(i,"data-rules-version");if(!Hr.includes(s))throw new qe(s);return{rules:$e(e.text(i)),tagNode:i,version:s}}var mh=new Function("url","return import(url)");function _o(e,t,r,n){let o=e.length,i=t.length,s=new Array(o).fill(-1);if(n){let g=new Set;for(let k=0;k<o;k++){let v=n[k];!(v>=0&&v<i)||g.has(v)||(s[k]=v,g.add(v))}}if(o===0||i===0)return s;let l=e.map(g=>So(g,r)),a=t.map(g=>So(g,r)),u=new Array(i).fill(!1);for(let g of s)g>=0&&(u[g]=!0);let d=new Map;a.forEach((g,k)=>{u[k]||d.set(g,d.has(g)?-1:k)});let f=new Map;l.forEach((g,k)=>{s[k]>=0||f.set(g,(f.get(g)||0)+1)}),l.forEach((g,k)=>{if(s[k]>=0||f.get(g)!==1)return;let v=d.get(g);v===void 0||v===-1||u[v]||(s[k]=v,u[v]=!0)});let h=[];for(let g=0;g<o;g++)s[g]<0&&h.push(g);let m=[];for(let g=0;g<i;g++)u[g]||m.push(g);if(h.length===0||m.length===0)return s;let c=o*i+1,p=Bl(e,t,r),b=(g,k)=>p(g,k)*c+Math.abs(g-k);for(let[g,k]of Hl(h,m,b))s[g]=k;return s}var Co=e=>typeof e=="object"&&e!==null;function So(e,t){if(!Co(t))return e==null?" null":String(e);let r=Object.keys(t);return JSON.stringify(r.map(n=>{let o=JSON.stringify(e?.[n]);return o===void 0?" undef":o}))}function Bl(e,t,r){if(!Co(r))return(l,a)=>e[l]===t[a]?0:1;let n=Object.keys(r),o=l=>n.map(a=>JSON.stringify(l?.[a])),i=e.map(o),s=t.map(o);return(l,a)=>{let u=i[l],d=s[a],f=0;for(let h=0;h<n.length;h++)u[h]!==d[h]&&f++;return f}}function Hl(e,t,r){return e.length<=t.length?Eo(e,t,r,!1):Eo(t,e,(n,o)=>r(o,n),!0)}function Eo(e,t,r,n){let o=e.length,i=t.length,s=[];for(let f=0;f<=o;f++)s.push(new Float64Array(i+1).fill(1/0));let l=[];for(let f=0;f<=o;f++)l.push(new Uint8Array(i+1));for(let f=0;f<=i;f++)s[o][f]=0;for(let f=o-1;f>=0;f--)for(let h=i-1;h>=0;h--){let m=r(e[f],t[h])+s[f+1][h+1],c=s[f][h+1];m<=c?(s[f][h]=m,l[f][h]=1):s[f][h]=c}let a=[],u=0,d=0;for(;u<o&&d<i;)l[u][d]&&(a.push(n?[t[d],e[u]]:[e[u],t[d]]),u++),d++;return a}function Wr(e,t,r,n,o,i,s,l={}){let a=e.find(t,r,l);if(o.length===0){a.forEach(A=>e.remove(A)),Vr(l,"onRowsApplied",i.path,[]);return}let u=o.length>a.length,d=a[0]||null;if(u&&!d&&(d=Gl(e,t,r,l),!d))throw new he(i.path);let f=a.map(A=>Kl(e,A,n,l)),h=null;if(u&&d){h=e.clone(d),l.templateAttr&&e.removeAttr(h,l.templateAttr);let A=e.stripIds(h);A>0&&console.warn(`[hyper-html-api] stripped ${A} id attribute(s) from cloned template at "${i.path.join(".")||"(root)"}"`)}let m=_o(o,f,n,Vl(e,a,o,i,l)),c=a.length>0?e.parent(a[0]):Zl(e,t,r,d,i,l),p=a.length>0?No(e,c,a[0]):0,b=ta(e,a),g=new Set,k=[],v=o.map((A,S)=>{let E=m[S];if(E>=0)return g.add(E),k.push(!1),a[E];k.push(!0);let O=e.clone(h);return e.stripIds(O),O});a.forEach((A,S)=>{g.has(S)||e.remove(A)}),b?ea(e,c,p,v,k):ra(e,v,k,c,p),Wl(e,v,n,o,i,s,l),Vr(l,"onRowsApplied",i.path,v)}function Vr(e,t,r,n){if(typeof e[t]=="function")try{return e[t](r.slice(),n)}catch(o){console.warn(`[hyper-html-api] ${t} threw at "${r.join(".")||"(root)"}"`,o);return}}function Vl(e,t,r,n,o){let i=Vr(o,"identifyRows",n.path,r);if(!Array.isArray(i))return null;let s=new Array(r.length).fill(-1),l=new Set,a=new Map(t.map((u,d)=>[e.identity(u),d]));for(let u=0;u<r.length;u++){if(!i[u])continue;let d=a.get(e.identity(i[u]));d===void 0||l.has(d)||(s[u]=d,l.add(d))}return s}function Wl(e,t,r,n,o,i,s){t.forEach((l,a)=>{if(r===null){let u=n[a],d=u==null?"":String(u);e.text(l)!==d&&e.text(l,d)}else{let u=i(e,l,r,n[a],{depth:o.depth+1,path:[...o.path,a]},s);u&&u!==l&&(t[a]=u)}})}function Kl(e,t,r,n){return r===null?e.text(t):pe(e,t,r,n.onRowsRead?{...n,onRowsRead:void 0}:n)}function No(e,t,r){let n=e.children(t);for(let o=0;o<n.length;o++)if(e.sameNode(n[o],r))return o;return-1}function Gl(e,t,r,n){if(!n.templateAttr)return null;let o=/^\s*:scope\s*>\s*/.test(r),i=new RegExp(":not\\(\\["+n.templateAttr.replace(/[.*+?^${}()|[\]\\-]/g,"\\$&")+"\\]\\)","g"),s=r.replace(/^\s*:scope\s*>\s*/,"").replace(i,""),l=o?Yl(s):0,a=jr(s),u=t;for(;u;){let d=e.find(u,s,{includeRulesTag:!1,templateAttr:null});for(let f of d)if(e.attr(f,n.templateAttr)!=null&&!(o&&!Jl(e,t,f,l))&&!(a&&!e.matches(e.parent(f),a)))return f;u=e.parent(u)}return null}function Yl(e){let t=0,r=null,n=1;for(let o=0;o<e.length;o++){let i=e[o];if(i==="\\")o++;else if(r)i===r&&(r=null);else if(i==='"'||i==="'")r=i;else if(i==="["||i==="(")t++;else if(i==="]"||i===")")t>0&&t--;else if(t===0&&i===">")n++;else{if(t===0&&(i==="+"||i==="~"))return null;if(t===0&&/\s/.test(i)){if(!Xl(e,o))return null;for(;o+1<e.length&&/\s/.test(e[o+1]);)o++}}}return n}function Xl(e,t){let r=t;for(;r<e.length&&/\s/.test(e[r]);)r++;let n=t>0?e[t-1]:"",o=r<e.length?e[r]:"";return To(n)||To(o)}function To(e){return e===">"||e==="+"||e==="~"}function Jl(e,t,r,n){if(n===null)return!0;let o=r;for(let a=0;a<n;a++)if(o=e.parent(o),!o)return!1;let i=e.prop(t,"tagName");if(!i)return!0;let s=e.prop(o,"tagName");if(!s||String(s).toLowerCase()!==String(i).toLowerCase())return!1;let l=new Set(Oo(e,o));return Oo(e,t).every(a=>l.has(a))}function Oo(e,t){let r=e.attr(t,"class");return r?String(r).split(/\s+/).filter(Boolean):[]}function Zl(e,t,r,n,o,i){let s=e.parent(n);if(s&&Ql(e,t,s)&&!Lo(e,s,i))return s;let l=jr(r);if(!l)throw new he(o.path,`the list selector "${r}" names no container to hold the new rows \u2014 write it as "container > row" (for example "ol.comments > li") so the cloned row lands in the list`);let u=(l===":scope"?e.prop(t,"tagName")?[t]:[]:e.find(t,l,i)).filter(d=>!Lo(e,d,i));if(u.length===0)throw new he(o.path,`no container matching "${l}" inside this context to hold the new rows`);if(u.length>1)throw new he(o.path,`"${l}" matches ${u.length} containers inside this context, so the new rows have no single destination`);return u[0]}function Ql(e,t,r){let n=r;for(;n;){if(e.sameNode(n,t))return!0;n=e.parent(n)}return e.parent(t)?!1:e.find(t,"*",{includeRulesTag:!0,templateAttr:null}).some(o=>e.sameNode(o,r))}var Ro=["no-data","editor-ui"];function Lo(e,t,r){let n=t;for(;n;){if(r.templateAttr&&e.attr(n,r.templateAttr)!=null||Ro.some(i=>e.attr(n,i)!=null))return!0;let o=e.attr(n,"clay");if(o&&o.split(/\s+/).some(i=>Ro.includes(i)))return!0;n=e.parent(n)}return!1}function ea(e,t,r,n,o){let i=e.children(t),s=i.map(a=>e.identity(a)),l=!1;n.forEach((a,u)=>{let d=e.identity(a),f=r+u;if(s[f]===d)return;if(!o[u]){let m=s.indexOf(d);m!==-1&&(s.splice(m,1),i.splice(m,1))}let h=Math.min(f,i.length);h<i.length?e.insertBefore(i[h],a):i.length?l?e.insertAfter(i[i.length-1],a):(e.insertAt(t,a,f),l=!0):e.insertAt(t,a,0),s.splice(h,0,d),i.splice(h,0,a)})}function ta(e,t){if(t.length<=1)return!0;let r=e.parent(t[0]);if(!r)return!1;let n=new Map(e.children(r).map((i,s)=>[e.identity(i),s])),o=[];for(let i of t){let s=n.get(e.identity(i));if(s===void 0)return!1;o.push(s)}return o.sort((i,s)=>i-s),o[o.length-1]-o[0]===o.length-1}function ra(e,t,r,n,o){let i=null,s=o;for(let l=0;l<t.length;l++){if(!r[l]){i=t[l];continue}let a=i?e.parent(i):n;if(!a)continue;let u=i?No(e,a,i)+1:s++;e.insertAt(a,t[l],u),i=t[l]}}function Fe(e,t,r,n,o={}){if(Object.prototype.hasOwnProperty.call(o,"exclude")&&e.semanticExclude===!1)throw new Error("This adapter does not support semantic exclude queries; omit exclude or use the DOM adapter.");Pe(r,o);let i=[];if(Kr(r,n,[],i),i.length)throw new lt(i);It(e,t,r,n,{depth:0,path:[]},o)}function It(e,t,r,n,o,i={}){if(o.depth>me)throw new Se(o.path);if(n===void 0)return t;if(typeof r=="string")return na(e,t,r,n,o,i);if(Array.isArray(r)){let[s,l]=r;return Wr(e,t,s,l,n,o,It,i),t}if(typeof r=="object"&&r!==null){for(let[s,l]of Object.entries(r)){let a=It(e,t,l,n==null?n:n[s],{depth:o.depth+1,path:[...o.path,s]},i);a&&a!==t&&(t=a)}return t}return t}function na(e,t,r,n,o,i){if(r.endsWith("[]")){let f=r.slice(0,-2);return Wr(e,t,f,null,n,o,It,i),t}let s=qt(r,i);if(s){let f=s.selector?e.find(t,s.selector,i):[t];return f.length&&s.codec.write(e,f[0],n,{query:i.query}),t}if(r.startsWith("@"))return jo(e,t,r.slice(1),n);let l=j(r);if(l!==-1){let{selector:f,all:h}=K(r),m=r.slice(l+1),c=f?e.find(t,f,i):[t];for(let p of h?c:c.slice(0,1))jo(e,p,m,n);return t}if(r===".")return Mo(e,t,n),t;let{selector:a,all:u}=K(r),d=e.find(t,a,i);for(let f of u?d:d.slice(0,1))Mo(e,f,n);return t}function Mo(e,t,r){let n=r==null?"":String(r);e.text(t)!==n&&e.text(t,n)}function oa(e,t,r,n){if(e.styleVar){e.styleVar(t,r,n);return}qr(e,t,r,n)}function jo(e,t,r,n){if(Ee(r))return oa(e,t,r,n==null?"":String(n).trim()),t;if(Ae.has(r)){let o=e.prop(t,r);if(o===n||o!=null&&n!=null&&String(o)===String(n))return t;throw new at(r)}if(r==="outerHTML"){let o=n==null?"":String(n);return e.replaceWith(t,o)}if(Or.has(r)){let o=Rr(r,n);return e.prop(t,r)!==o&&e.prop(t,r,o),t}if(n==null)e.attr(t,r)!=null&&e.removeAttr(t,r);else{let o=String(n);e.attr(t,r)!==o&&e.attr(t,r,o)}return t}function Kr(e,t,r,n){if(t!==void 0){if(typeof e=="string"){if(fe(e))return;if(e.endsWith("[]")){Array.isArray(t)?t.forEach((o,i)=>{typeof o=="object"&&o!==null&&n.push({path:gt([...r,i]),expected:"scalar",got:pt(o)})}):n.push({path:gt(r),expected:"array",got:pt(t)});return}t!==null&&typeof t=="object"&&n.push({path:gt(r),expected:"scalar",got:pt(t)});return}if(Array.isArray(e)){if(!Array.isArray(t)){n.push({path:gt(r),expected:"array",got:pt(t)});return}let o=e[1];t.forEach((i,s)=>Kr(o,i,[...r,s],n));return}if(typeof e=="object"&&e!==null){if(t===null||Array.isArray(t)||typeof t!="object"){n.push({path:gt(r),expected:"object",got:pt(t)});return}for(let[o,i]of Object.entries(e))Kr(i,t[o],[...r,o],n)}}}function pt(e){return e===null?"null":Array.isArray(e)?"array":typeof e}function gt(e){return e.join(".")}var Po=Object.freeze({refusedRuleForms:["innerHTML","outerHTML"],refusedTargets:["script","style","template","noscript","iframe","object","embed","xmp","plaintext","noembed","noframes","meta","link","base","animate","set","animatemotion","animatetransform"],refusedAttributes:["style","srcdoc","is","http-equiv","sandbox","allow","nonce","integrity","charset","htmlclaytoken","htmlclayid","data-rules-name","data-rules-version","clay","contenteditable",...uo],refusedAttributePrefixes:["on"],urlAttributes:["href","src","srcset","action","formaction","poster","cite","data","ping","background","longdesc","usemap","xlink:href","manifest","codebase","icon"],refusedSchemes:["javascript","vbscript","data"],customPropertyValues:Object.freeze({maxLength:200,refusedCharacters:[";","{","}","<",">","\\"],refusedFunctions:["url","src","image","image-set","-webkit-image-set","cross-fade","element","expression","paint"]})});function ga(e){if(/[\u0000-\u001f\u007f]/.test(e))return"control characters are not allowed in a custom property value";let t=[],r=null;for(let n=0;n<e.length;n++){let o=e[n];if(r){o===r&&(r=null);continue}if(o==='"'||o==="'")r=o;else{if(o==="/"&&e[n+1]==="*")return"comments are not allowed in a custom property value";if(o==="(")t.push(")");else if(o==="[")t.push("]");else if((o===")"||o==="]")&&t.pop()!==o)return`"${o}" does not close anything in this custom property value`}}return r?"a custom property value must close its quotes":t.length?"a custom property value must close its brackets":null}function Gr(e,t=Po){let r=t.customPropertyValues,n=String(e);if(n.length>r.maxLength)return`custom property values are limited to ${r.maxLength} characters`;let o=r.refusedCharacters.find(a=>n.includes(a));if(o)return`${JSON.stringify(o)} is not allowed in a custom property value`;let i=ga(n);if(i)return i;let s=n.toLowerCase(),l=r.refusedFunctions.find(a=>new RegExp(`(^|[^a-z0-9_-])${a.replace(/[-]/g,"\\-")}\\s*\\(`).test(s));return l?`"${l}()" is not allowed in a custom property value`:null}function Yr(e,t,r){if(r&&typeof r=="object")return{rules:r,tagNode:null,version:"1"};if(typeof r=="string"){let n=t&&t.ownerDocument?t.ownerDocument:t;return ft(e,n,r)}return null}var Xr=e=>({codecs:Fr(),...e}),U={extract:(e,t,r={})=>pe(ie(e,r),e,t,Xr(r)),apply:(e,t,r,n={})=>Fe(ie(e,{...n,_write:!0}),e,t,r,Xr(n)),findRulesIn:(e,t)=>ft(Ie,e,t),findRules:(e,t)=>Yr(Ie,e,t),bind:(e,t,r={})=>{let n=Yr(Ie,e,t);if(!n){let i=typeof t=="string"?`data-rules-name~="${t}"`:"the provided rules object";throw new Error(`hyper-html-api: could not resolve rules for ${i}`)}let o=Xr({version:n.version,...r});return{...n,get:()=>pe(ie(e,r),e,n.rules,o),set:i=>Fe(ie(e,{...r,_write:!0}),e,n.rules,i,o)}},codecs:{register:Ir,get:$r},parseStrict:Br,parseRelaxed:$e,ruleAttrIndex:j,splitRule:K,errors:jt,DOM_PROPERTIES:Lt};var Zr={};Tr(Zr,{fromString:()=>se,getRuleAtPath:()=>le,getValueAtPath:()=>xa,setAtPath:()=>Jr,toString:()=>va});function va(e){return e.map(String).join(".")}function se(e){return e===""?[]:e.split(".").map(t=>/^\d+$/.test(t)?Number(t):t)}function le(e,t){let r=e;for(let n of t){if(r==null)return;if(typeof r=="string"){if(r.endsWith("[]")&&(typeof n=="number"||n==="*")){r=r.slice(0,-2);continue}return}if(Array.isArray(r)){if(typeof n!="number"&&n!=="*")return;r=r[1];continue}if(typeof r=="object"){if(typeof n=="number"||!(n in r))return;r=r[n];continue}return}return r}function xa(e,t){let r=e;for(let n of t){if(r==null)return;r=r[n]}return r}function Jr(e,t,r){if(t.length===0)return r;let[n,...o]=t;if(typeof n=="number"){let i=Array.isArray(e)?[...e]:[];return i[n]=Jr(i[n],o,r),i}return{...e&&typeof e=="object"?e:{},[n]:Jr((e||{})[n],o,r)}}function bt(e){if(typeof e=="string")return e.endsWith("[]")?[]:"";if(Array.isArray(e))return[];if(typeof e=="object"&&e!==null){let t={};for(let[r,n]of Object.entries(e))t[r]=bt(n);return t}return""}import wa from"./hyper-morph.vendor.js";function $t(e,t,{ignoreActiveValue:r=!0}={}){wa.morph(e,t,{morphStyle:"innerHTML",ignoreActiveValue:r,restoreFocus:!0,formStateSync:"property",policy:"raw"})}import{morph as Uo}from"./hyper-morph.vendor.js";var Io=new WeakMap,Qr=e=>e.map(String).join(".");function Aa(e){return typeof CSS<"u"&&CSS.escape?CSS.escape(e):String(e).replace(/[^a-zA-Z0-9_\-.*]/g,t=>"\\"+t)}function $o(e,t){if(!e||!e.querySelector)return null;let r=e.querySelector(`[data-hcms-path="${Aa(t)}"]`),n=r&&r.querySelector(".hcms-array-items");return n?Array.from(n.children).filter(o=>o.matches&&o.matches("[data-hcms-card], [data-hcms-array-item]")):null}function Fo(e,t,r){let n=$o(e,t);!n||n.length!==r.length||n.forEach((o,i)=>{r[i]&&Io.set(o,r[i])})}function Ft(){let e=new Map;return{hooks:{onRowsRead(t,r){e.set(Qr(t),r)}},seed(t){for(let[r,n]of e)Fo(t,r,n);e.clear()}}}function Do(e){return{identifyRows(t,r){let n=$o(e,Qr(t));return!n||n.length!==r.length?null:n.map(o=>Io.get(o)||null)},onRowsApplied(t,r){Fo(e,Qr(t),r)}}}function te(e){return e&&e.nodeType===9?e.body||e.documentElement:e}function Dt(e,t){if(!t||t.nodeType!==9)return e;return r(e);function r(n){if(typeof n=="string")return n==="."?"body":n.startsWith("@")?`body${n}`:n;if(Array.isArray(n)||!n||typeof n!="object")return n;let o={};for(let[i,s]of Object.entries(n))o[i]=r(s);return o}}var zo={skip:"[data-hcms-shell]",templateAttr:"cms-template"},en="data-hcms-rollback-ui",Sa=0;function Bo(e,t,r,n={}){return Ea(e,t,r,n)}function Ea(e,t,r,n){let{shellRoot:o,structural:i,structuralPath:s,formRoot:l}=n,a=l?{...zo,...Do(l)}:zo;if(!i)try{return U.apply(e,t,r,a),{ok:!0}}catch(h){return{ok:!1,error:h}}let u=_a(e,t,s),d=u?Ta(u):null,f=u?null:Ra(te(e),o);try{return U.apply(e,t,r,a),{ok:!0}}catch(h){return d?Oa(u,d):f&&La(te(e),o,f),{ok:!1,error:h}}}function _a(e,t,r){if(!r||!e)return null;let n=se(r),o=[],i=t;for(let s of n){if(typeof i=="string"||i==null||Array.isArray(i))break;if(typeof i=="object"&&s in i){if(o.push(s),i=i[s],Array.isArray(i)||typeof i=="string"&&i.endsWith("[]"))break}else return null}return!Array.isArray(i)&&!(typeof i=="string"&&i.endsWith("[]"))?null:Ca(e,t,o)}function Ca(e,t,r){if(r.length===0)return null;let n=e,o=t;for(let i=0;i<r.length;i++){let s=r[i];if(!o||typeof o!="object"||Array.isArray(o))return null;let l=o[s];if(l==null)return null;if(i===r.length-1){if(Array.isArray(l)){let[a]=l;return n.querySelector?.(a)?.parentElement||null}if(typeof l=="string"&&l.endsWith("[]")){let a=l.slice(0,-2);return n.querySelector?.(a)?.parentElement||null}return null}o=l}return null}function Ta(e){let t=[],r=[];for(let n of Array.from(e.childNodes))t.push(tn(n,r));return{nodes:t,retained:r}}function Oa(e,t){let r=e.cloneNode(!1);for(let n of t.nodes)r.appendChild(n);Uo(e,Array.from(r.childNodes),Vo()),Ho(e,t.retained)}function Ra(e,t){let r=[],n=[];for(let o of Array.from(e.childNodes))o===t||t&&o.contains?.(t)||r.push(tn(o,n));return{nodes:r,retained:n}}function La(e,t,r){let n=e.cloneNode(!1);for(let o of r.nodes)n.appendChild(o);Uo(e,Array.from(n.childNodes),Vo()),Ho(e,r.retained)}function tn(e,t){let r=e.cloneNode(!1);if(e.nodeType===1&&e.matches('[editor-ui],[clay~="editor-ui"]')){let i=String(++Sa);r.setAttribute(en,i),t.push({id:i,node:e})}let n=e.nodeType===1&&e.tagName==="TEMPLATE"?e.content:e,o=r.nodeType===1&&r.tagName==="TEMPLATE"?r.content:r;for(let i of Array.from(n.childNodes||[]))o.appendChild(tn(i,t));return r}function Ho(e,t){for(let{id:r,node:n}of t){let o=e.querySelector(`[${en}="${r}"]`);o===n?n.removeAttribute(en):n.isConnected?o?.remove():o?.replaceWith(n)}}function Vo(){return{morphStyle:"innerHTML",policy:"raw",restoreFocus:!1,scripts:{handle:!1,merge:!1}}}var Na=/^--font-(size|weight|style|stretch|variant|feature-settings|variation-settings|optical-sizing|kerning|synthesis|smoothing|palette|size-adjust|display)(-|$)/;function _e(e){return e.replace(/([a-z])([A-Z])/g,"$1 $2").replace(/([A-Z]+)([A-Z][a-z])/g,"$1 $2").replace(/[-_]/g," ").replace(/\s+/g," ").trim().replace(/^./,t=>t.toUpperCase())}var Ma='<div class="hcms-drag-handle mirk-sortable__grip" aria-hidden="true"><div class="mirk-sortable__dots"><span class="mirk-sortable__dot"></span><span class="mirk-sortable__dot"></span><span class="mirk-sortable__dot"></span><span class="mirk-sortable__dot"></span><span class="mirk-sortable__dot"></span><span class="mirk-sortable__dot"></span><span class="mirk-sortable__dot"></span><span class="mirk-sortable__dot"></span></div></div>',zt='<svg class="hcms-x" viewBox="0 0 16 16" shape-rendering="crispEdges" aria-hidden="true"><path d="M4 4 L12 12 M12 4 L4 12" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="square"></path></svg>',Yo={"@scalar":`
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
      ${Ma}
      <div class="hcms-card-body mirk-sortable__body">
        <div class="hcms-card-fields"></div>
        <div class="hcms-card-controls">
          <button type="button" class="hcms-move hcms-move-up hcms-sr-only" data-hcms-action="move-up" aria-label="Move up">\u2191</button>
          <button type="button" class="hcms-move hcms-move-down hcms-sr-only" data-hcms-action="move-down" aria-label="Move down">\u2193</button>
          <button type="button" class="hcms-remove hcms-remove--card" data-hcms-action="remove" aria-label="Remove">${zt}</button>
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
        <button type="button" class="hcms-upload-clear" data-hcms-action="clear-upload" aria-label="Remove file">${zt}</button>
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
          <button type="button" class="hcms-upload-clear hcms-upload-clear--badge" data-hcms-action="clear-upload" aria-label="Remove image">${zt}</button>
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
        <button type="button" class="hcms-color-clear" data-hcms-action="color-clear" aria-label="Clear color">${zt}</button>
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
  `},ja=["@scalar","@object","@scalar-array","@scalar-array-item","@object-array","@object-array-item"];function Bt(e){let t=e.head||e.documentElement;if(t)for(let r of ja)ei(e,t,r)}function rn(e,t){if(!Yo[t])return null;let r=e&&(e.head||e.documentElement);return r?ei(e,r,t):null}var Wo={src:"@image",checked:"@checkbox",innerHTML:"@richtext"},Ut={image:"@image",file:"@file",checkbox:"@checkbox",toggle:"@toggle",select:"@select",radio:"@radio",textarea:"@textarea",number:"@number",richtext:"@richtext",color:"@color",font:"@font",variant:"@variant"},qa=new Set([...Object.values(Ut),"@chips","@chips-item"]);function ge(e,t,r,n){if(typeof e!="string")return"@scalar";let o=j(e),i=Qo(e,o,r,n),s=Wt(e,o,r,n,t,"data-hcms-component");if(s&&Ut[s]){let l=Ut[s],a=Array.isArray(r)&&r.some(u=>u==="*"||typeof u=="number");return l==="@number"&&!Ko(e,o,t,a,i).every(Fa)||(l==="@checkbox"||l==="@toggle")&&(o<0||e.slice(o+1)!=="checked")&&!Ko(e,o,t,a,i).every(Da)?"@scalar":l}if(o>=0){let l=e.slice(o+1);if(Wo[l])return Wo[l];if(/^--color(-|$)/.test(l)&&l!=="--color-scheme")return"@color";if(/^--font(-|$)/.test(l)&&!Na.test(l))return"@font"}return"@scalar"}function Xo(e,t,r,n){if(typeof e!="string")return null;let o=j(e),i=Wt(e,o,r,n,t,"data-hcms-component");return i&&Ut[i]||null}var Pa=new Set(["@color","@font","@variant"]),Ia=new Set(["html","body",":root"]);function kt(e,t,r,n){if(typeof e!="string"||e.endsWith("[]")||!Pa.has(ge(e,t,r,n)))return null;let o=Array.isArray(r)&&r.some(s=>s==="*"||typeof s=="number"),i=Zo(e,j(e)).trim();return!o&&Ia.has(i)?"page":"section"}var $a=/^-?\d+(\.\d+)?([eE][+-]?\d+)?$/;function Fa(e){return e==null||e===""?!0:$a.test(String(e))}function Da(e){return e==null||e===""||e==="true"||e==="false"}function Ko(e,t,r,n,o){if(!r||!r.querySelectorAll)return[];if(!o||o===".")return[];let i=null;try{i=r.querySelectorAll(o)}catch{return[]}let s=t>=0?e.slice(t+1):null,l=[];for(let a of i)if(!(a.closest&&a.closest("[cms-template], [data-hcms-shell]"))&&(s?s==="value"&&"value"in a?l.push(a.value):l.push(a.getAttribute?a.getAttribute(s):null):l.push((a.textContent||"").trim()),!n))break;return l}function Ht(e,t){if(typeof e!="string"||!e.endsWith("[]")||!t||!t.querySelector)return null;let r=e.slice(0,-2).trim();if(!r)return null;let n=null;try{n=t.querySelector(r)}catch{return null}let o=n&&n.closest?n.closest("[data-hcms-component]"):null;return(o&&o.getAttribute?o.getAttribute("data-hcms-component"):null)==="chips"?{array:"@chips",item:"@chips-item"}:null}function Ce(e,t,r){let n=e.join("."),o=e.map(i=>typeof i=="number"?"*":i).join(".");return n&&J(r,n)||o&&o!==n&&J(r,o)||J(r,t)}function Vt(e,t,r){let n=Ht(e,r);if(!n)return null;let o=Ce(t,n.array,r);return o&&o.getAttribute("data-hcms-tpl")===n.array?n:null}function yt(e,t,r,n){if(typeof e!="string")return null;let o=j(e),i=Wt(e,o,r,n,t,"data-hcms-options");if(i==null)return null;let s=i.trim().split(/\s+/).filter(Boolean);return s.length?s:null}function Jo(e,t,r,n){if(typeof e!="string")return null;let o=j(e);return Wt(e,o,r,n,t,"data-hcms-crop")}function Zo(e,t){return t>=0?e.slice(0,t):e}function Qo(e,t,r,n){let o=Zo(e,t);return o&&o!=="."?o:za(n,r)}function za(e,t){if(e==null||!Array.isArray(t))return"";let r=[],n=e;for(let o of t){if(n==null||typeof n=="string")break;if(Array.isArray(n)){if(typeof n[0]!="string"||o!=="*"&&typeof o!="number")return"";r.push(n[0]),n=n[1];continue}if(typeof n!="object"||!Object.prototype.hasOwnProperty.call(n,o))return"";n=n[o]}return r.join(" ")}function Go(e,t,r){if(!t||!t.querySelector||!e||e===".")return null;let n=null;try{n=t.querySelector(e)}catch{return null}return n&&n.getAttribute?n.getAttribute(r):null}function Wt(e,t,r,n,o,i){let s=Qo(e,t,r,n),l=t>=0?e.slice(t+1):null;if(l){let a=Go(s,o,`${i}:${l}`);if(a!=null)return a;if(l.startsWith("--"))return null}return Go(s,o,i)}function Kt(e,t){if(!e||t==null)return;r(t,[]);function r(n,o){let i=be(n);if(i==="scalar"){let s=ge(n,e,o,t);qa.has(s)&&rn(e,s);return}if(i==="scalar-array"){let s=Ht(n,e);s&&(rn(e,s.array),rn(e,s.item));return}if(i==="object"){for(let[s,l]of Object.entries(n))r(l,[...o,s]);return}if(i==="object-array"){let s=n[1],l=[...o,"*"];if(s&&typeof s=="object"&&!Array.isArray(s))for(let[a,u]of Object.entries(s))r(u,[...l,a]);else r(s,l)}}}function ei(e,t,r){let n=J(e,r);if(n)return n;let o=e.createElement("template");return o.setAttribute("data-hcms-tpl",r),o.setAttribute("save-remove",""),o.innerHTML=Yo[r].trim(),t.appendChild(o),o}function J(e,t){return!e||!e.querySelector?null:e.querySelector(`template[data-hcms-tpl="${Ua(t)}"]`)}function Ua(e){return typeof CSS<"u"&&CSS.escape?CSS.escape(e):String(e).replace(/[^a-zA-Z0-9_\-.*]/g,t=>"\\"+t)}function be(e){return typeof e=="string"?e.endsWith("[]")?"scalar-array":"scalar":Array.isArray(e)?"object-array":typeof e=="object"&&e!==null?"object":"scalar"}function De(e){return e?!!(e.content||e).querySelector("[data-hcms-field]"):!1}var ti={IMG:"src",A:"href"};function Gt(e){return e.map(t=>typeof t=="number"||/^[0-9]+$/.test(String(t))?"*":String(t)).join(".")}function Yt(e){if(!e)return"value";let t=(e.tagName||"").toUpperCase();return t==="INPUT"?(e.getAttribute("type")||"text").toLowerCase()==="checkbox"?"checked":"value":t==="TEXTAREA"||t==="SELECT"?"value":ti[t]?ti[t]:e.hasAttribute&&e.hasAttribute("contenteditable")?"innerHTML":null}function ri(e,t){let r=(e.tagName||"").toUpperCase(),n=(e.getAttribute&&e.getAttribute("type")||"").toLowerCase(),o=Yt(e),s=`${oi(r,n)}[data-hcms-field="${Te(t)}"]`;return r==="INPUT"&&n==="radio"?`${s}:checked@value`:o?`${s}@${o}`:s}function Ba(e){let t=(e.tagName||"").toUpperCase(),r=(e.getAttribute&&e.getAttribute("type")||"").toLowerCase(),n=Yt(e),i=`${oi(t,r)}[data-hcms-field]`;return t==="INPUT"&&r==="radio"?`${i}:checked@value`:n?`${i}@${n}`:i}function oi(e,t){return e==="INPUT"?t?`input[type="${t}"]`:"input":e==="TEXTAREA"?"textarea":e==="SELECT"?"select":e==="IMG"?"img":e==="A"?"a":':not([data-hcms-shape="scalar"]):not([data-hcms-shape="object"]):not([data-hcms-shape="object-array"]):not([data-hcms-shape="scalar-array"])'}function Te(e){return typeof CSS<"u"&&CSS.escape?CSS.escape(e):String(e).replace(/[^a-zA-Z0-9_\-.*]/g,t=>"\\"+t)}var ni=new Set(["__proto__","constructor","prototype"]);function Ha(e){let t=new Map;return r(e,null),t;function r(n,o){if(typeof n=="string"){typeof o=="string"&&!n.endsWith("[]")&&t.set(o,(t.get(o)||0)+1);return}if(Array.isArray(n)){r(n[1],null);return}if(n&&typeof n=="object")for(let[i,s]of Object.entries(n))r(s,i)}}function Xt(e,t){let r=Ha(e);return n(e,[]);function n(f,h){let m=be(f);if(m==="scalar")return o(f,h);if(m==="scalar-array")return s(f,h);if(m==="object-array")return l(f,h);if(m==="object"){let c=Object.create(null);for(let[p,b]of Object.entries(f)){if(ni.has(p))throw new Error(`hypercms: rule key "${p}" is forbidden at "${h.join(".")||"<root>"}"`);c[p]=n(b,[...h,p])}return c}return null}function o(f,h){let m=h.length?h[h.length-1]:null,c=typeof m=="string"?m:"__value",p=d(h,c);if(p)return i(h,c,ri(p,c));let b=u(ge(f,t,h,e),c);return b?i(h,c,ri(b,c)):i(h,c,`input[data-hcms-field="${Te(c)}"]@value`)}function i(f,h,m){if((r.get(h)||0)<2)return m;let c=`[data-hcms-path-shape="${Te(Gt(f))}"]`,p=m.lastIndexOf("@"),b=p===-1?m:m.slice(0,p),g=p===-1?"":m.slice(p);return`${c} ${b}, ${b}${c}${g}`}function s(f,h){let m=Vt(f,h,t),c=m&&u(m.item,null)||u("@scalar-array-item",null),p=c?Ba(c):"input[data-hcms-field]@value";return[a(h,"[data-hcms-array-item]"),p]}function l(f,h){let[,m]=f,c=[...h,"*"],p=a(h,"[data-hcms-card]");if(m&&typeof m=="object"&&!Array.isArray(m)){let b=Object.create(null);for(let[g,k]of Object.entries(m)){if(ni.has(g))throw new Error(`hypercms: rule key "${g}" is forbidden at "${c.join(".")}"`);b[g]=n(k,[...c,g])}return[p,b]}return[p,n(m,[...c,0])]}function a(f,h){let m=f.length?f[f.length-1]:"",c=f.some(g=>g==="*"),p=f.join(".");return`${c?`[data-hcms-field="${Te(m)}"]`:`[data-hcms-path="${Te(p)}"]`} > .hcms-array-items > ${h}`}function u(f,h){if(!t)return null;let m=J(t,f);if(!m)return null;let c=m.content||m;if(h){let p=c.querySelector(`[data-hcms-field="${Te(h)}"]`);if(p)return p}return c.querySelector("[data-hcms-field]")}function d(f,h){if(!t)return null;let m=f.map(b=>typeof b=="number"?"*":b).join("."),p=[f.join("."),m];for(let b=f.length-1;b>=0;b--){let g=f.slice(0,b).map(k=>typeof k=="number"?"*":k);be(le(e,g))==="object"&&p.push(g.join(".")),g.push("*"),p.push(g.join("."))}for(let b of p){if(!b)continue;let g=J(t,b);if(!g||!De(g))continue;let k=g.content||g,v=k.querySelector(`[data-hcms-field="${Te(h)}"]`)||k.querySelector("[data-hcms-field]");if(v)return v}return null}}var ii=["50","100","200","300","400","500","600","700","800","900","950"],si=["red","orange","amber","yellow","lime","green","emerald","teal","cyan","sky","blue","indigo","violet","purple","fuchsia","pink","rose","slate","gray","zinc","neutral","stone","mauve","olive","mist","taupe"],ze=Object.freeze([{id:"red-50",hue:"red",shade:"50",value:"oklch(97.1% 0.013 17.38)"},{id:"red-100",hue:"red",shade:"100",value:"oklch(93.6% 0.032 17.717)"},{id:"red-200",hue:"red",shade:"200",value:"oklch(88.5% 0.062 18.334)"},{id:"red-300",hue:"red",shade:"300",value:"oklch(80.8% 0.114 19.571)"},{id:"red-400",hue:"red",shade:"400",value:"oklch(70.4% 0.191 22.216)"},{id:"red-500",hue:"red",shade:"500",value:"oklch(63.7% 0.237 25.331)"},{id:"red-600",hue:"red",shade:"600",value:"oklch(57.7% 0.245 27.325)"},{id:"red-700",hue:"red",shade:"700",value:"oklch(50.5% 0.213 27.518)"},{id:"red-800",hue:"red",shade:"800",value:"oklch(44.4% 0.177 26.899)"},{id:"red-900",hue:"red",shade:"900",value:"oklch(39.6% 0.141 25.723)"},{id:"red-950",hue:"red",shade:"950",value:"oklch(25.8% 0.092 26.042)"},{id:"orange-50",hue:"orange",shade:"50",value:"oklch(98% 0.016 73.684)"},{id:"orange-100",hue:"orange",shade:"100",value:"oklch(95.4% 0.038 75.164)"},{id:"orange-200",hue:"orange",shade:"200",value:"oklch(90.1% 0.076 70.697)"},{id:"orange-300",hue:"orange",shade:"300",value:"oklch(83.7% 0.128 66.29)"},{id:"orange-400",hue:"orange",shade:"400",value:"oklch(75% 0.183 55.934)"},{id:"orange-500",hue:"orange",shade:"500",value:"oklch(70.5% 0.213 47.604)"},{id:"orange-600",hue:"orange",shade:"600",value:"oklch(64.6% 0.222 41.116)"},{id:"orange-700",hue:"orange",shade:"700",value:"oklch(55.3% 0.195 38.402)"},{id:"orange-800",hue:"orange",shade:"800",value:"oklch(47% 0.157 37.304)"},{id:"orange-900",hue:"orange",shade:"900",value:"oklch(40.8% 0.123 38.172)"},{id:"orange-950",hue:"orange",shade:"950",value:"oklch(26.6% 0.079 36.259)"},{id:"amber-50",hue:"amber",shade:"50",value:"oklch(98.7% 0.022 95.277)"},{id:"amber-100",hue:"amber",shade:"100",value:"oklch(96.2% 0.059 95.617)"},{id:"amber-200",hue:"amber",shade:"200",value:"oklch(92.4% 0.12 95.746)"},{id:"amber-300",hue:"amber",shade:"300",value:"oklch(87.9% 0.169 91.605)"},{id:"amber-400",hue:"amber",shade:"400",value:"oklch(82.8% 0.189 84.429)"},{id:"amber-500",hue:"amber",shade:"500",value:"oklch(76.9% 0.188 70.08)"},{id:"amber-600",hue:"amber",shade:"600",value:"oklch(66.6% 0.179 58.318)"},{id:"amber-700",hue:"amber",shade:"700",value:"oklch(55.5% 0.163 48.998)"},{id:"amber-800",hue:"amber",shade:"800",value:"oklch(47.3% 0.137 46.201)"},{id:"amber-900",hue:"amber",shade:"900",value:"oklch(41.4% 0.112 45.904)"},{id:"amber-950",hue:"amber",shade:"950",value:"oklch(27.9% 0.077 45.635)"},{id:"yellow-50",hue:"yellow",shade:"50",value:"oklch(98.7% 0.026 102.212)"},{id:"yellow-100",hue:"yellow",shade:"100",value:"oklch(97.3% 0.071 103.193)"},{id:"yellow-200",hue:"yellow",shade:"200",value:"oklch(94.5% 0.129 101.54)"},{id:"yellow-300",hue:"yellow",shade:"300",value:"oklch(90.5% 0.182 98.111)"},{id:"yellow-400",hue:"yellow",shade:"400",value:"oklch(85.2% 0.199 91.936)"},{id:"yellow-500",hue:"yellow",shade:"500",value:"oklch(79.5% 0.184 86.047)"},{id:"yellow-600",hue:"yellow",shade:"600",value:"oklch(68.1% 0.162 75.834)"},{id:"yellow-700",hue:"yellow",shade:"700",value:"oklch(55.4% 0.135 66.442)"},{id:"yellow-800",hue:"yellow",shade:"800",value:"oklch(47.6% 0.114 61.907)"},{id:"yellow-900",hue:"yellow",shade:"900",value:"oklch(42.1% 0.095 57.708)"},{id:"yellow-950",hue:"yellow",shade:"950",value:"oklch(28.6% 0.066 53.813)"},{id:"lime-50",hue:"lime",shade:"50",value:"oklch(98.6% 0.031 120.757)"},{id:"lime-100",hue:"lime",shade:"100",value:"oklch(96.7% 0.067 122.328)"},{id:"lime-200",hue:"lime",shade:"200",value:"oklch(93.8% 0.127 124.321)"},{id:"lime-300",hue:"lime",shade:"300",value:"oklch(89.7% 0.196 126.665)"},{id:"lime-400",hue:"lime",shade:"400",value:"oklch(84.1% 0.238 128.85)"},{id:"lime-500",hue:"lime",shade:"500",value:"oklch(76.8% 0.233 130.85)"},{id:"lime-600",hue:"lime",shade:"600",value:"oklch(64.8% 0.2 131.684)"},{id:"lime-700",hue:"lime",shade:"700",value:"oklch(53.2% 0.157 131.589)"},{id:"lime-800",hue:"lime",shade:"800",value:"oklch(45.3% 0.124 130.933)"},{id:"lime-900",hue:"lime",shade:"900",value:"oklch(40.5% 0.101 131.063)"},{id:"lime-950",hue:"lime",shade:"950",value:"oklch(27.4% 0.072 132.109)"},{id:"green-50",hue:"green",shade:"50",value:"oklch(98.2% 0.018 155.826)"},{id:"green-100",hue:"green",shade:"100",value:"oklch(96.2% 0.044 156.743)"},{id:"green-200",hue:"green",shade:"200",value:"oklch(92.5% 0.084 155.995)"},{id:"green-300",hue:"green",shade:"300",value:"oklch(87.1% 0.15 154.449)"},{id:"green-400",hue:"green",shade:"400",value:"oklch(79.2% 0.209 151.711)"},{id:"green-500",hue:"green",shade:"500",value:"oklch(72.3% 0.219 149.579)"},{id:"green-600",hue:"green",shade:"600",value:"oklch(62.7% 0.194 149.214)"},{id:"green-700",hue:"green",shade:"700",value:"oklch(52.7% 0.154 150.069)"},{id:"green-800",hue:"green",shade:"800",value:"oklch(44.8% 0.119 151.328)"},{id:"green-900",hue:"green",shade:"900",value:"oklch(39.3% 0.095 152.535)"},{id:"green-950",hue:"green",shade:"950",value:"oklch(26.6% 0.065 152.934)"},{id:"emerald-50",hue:"emerald",shade:"50",value:"oklch(97.9% 0.021 166.113)"},{id:"emerald-100",hue:"emerald",shade:"100",value:"oklch(95% 0.052 163.051)"},{id:"emerald-200",hue:"emerald",shade:"200",value:"oklch(90.5% 0.093 164.15)"},{id:"emerald-300",hue:"emerald",shade:"300",value:"oklch(84.5% 0.143 164.978)"},{id:"emerald-400",hue:"emerald",shade:"400",value:"oklch(76.5% 0.177 163.223)"},{id:"emerald-500",hue:"emerald",shade:"500",value:"oklch(69.6% 0.17 162.48)"},{id:"emerald-600",hue:"emerald",shade:"600",value:"oklch(59.6% 0.145 163.225)"},{id:"emerald-700",hue:"emerald",shade:"700",value:"oklch(50.8% 0.118 165.612)"},{id:"emerald-800",hue:"emerald",shade:"800",value:"oklch(43.2% 0.095 166.913)"},{id:"emerald-900",hue:"emerald",shade:"900",value:"oklch(37.8% 0.077 168.94)"},{id:"emerald-950",hue:"emerald",shade:"950",value:"oklch(26.2% 0.051 172.552)"},{id:"teal-50",hue:"teal",shade:"50",value:"oklch(98.4% 0.014 180.72)"},{id:"teal-100",hue:"teal",shade:"100",value:"oklch(95.3% 0.051 180.801)"},{id:"teal-200",hue:"teal",shade:"200",value:"oklch(91% 0.096 180.426)"},{id:"teal-300",hue:"teal",shade:"300",value:"oklch(85.5% 0.138 181.071)"},{id:"teal-400",hue:"teal",shade:"400",value:"oklch(77.7% 0.152 181.912)"},{id:"teal-500",hue:"teal",shade:"500",value:"oklch(70.4% 0.14 182.503)"},{id:"teal-600",hue:"teal",shade:"600",value:"oklch(60% 0.118 184.704)"},{id:"teal-700",hue:"teal",shade:"700",value:"oklch(51.1% 0.096 186.391)"},{id:"teal-800",hue:"teal",shade:"800",value:"oklch(43.7% 0.078 188.216)"},{id:"teal-900",hue:"teal",shade:"900",value:"oklch(38.6% 0.063 188.416)"},{id:"teal-950",hue:"teal",shade:"950",value:"oklch(27.7% 0.046 192.524)"},{id:"cyan-50",hue:"cyan",shade:"50",value:"oklch(98.4% 0.019 200.873)"},{id:"cyan-100",hue:"cyan",shade:"100",value:"oklch(95.6% 0.045 203.388)"},{id:"cyan-200",hue:"cyan",shade:"200",value:"oklch(91.7% 0.08 205.041)"},{id:"cyan-300",hue:"cyan",shade:"300",value:"oklch(86.5% 0.127 207.078)"},{id:"cyan-400",hue:"cyan",shade:"400",value:"oklch(78.9% 0.154 211.53)"},{id:"cyan-500",hue:"cyan",shade:"500",value:"oklch(71.5% 0.143 215.221)"},{id:"cyan-600",hue:"cyan",shade:"600",value:"oklch(60.9% 0.126 221.723)"},{id:"cyan-700",hue:"cyan",shade:"700",value:"oklch(52% 0.105 223.128)"},{id:"cyan-800",hue:"cyan",shade:"800",value:"oklch(45% 0.085 224.283)"},{id:"cyan-900",hue:"cyan",shade:"900",value:"oklch(39.8% 0.07 227.392)"},{id:"cyan-950",hue:"cyan",shade:"950",value:"oklch(30.2% 0.056 229.695)"},{id:"sky-50",hue:"sky",shade:"50",value:"oklch(97.7% 0.013 236.62)"},{id:"sky-100",hue:"sky",shade:"100",value:"oklch(95.1% 0.026 236.824)"},{id:"sky-200",hue:"sky",shade:"200",value:"oklch(90.1% 0.058 230.902)"},{id:"sky-300",hue:"sky",shade:"300",value:"oklch(82.8% 0.111 230.318)"},{id:"sky-400",hue:"sky",shade:"400",value:"oklch(74.6% 0.16 232.661)"},{id:"sky-500",hue:"sky",shade:"500",value:"oklch(68.5% 0.169 237.323)"},{id:"sky-600",hue:"sky",shade:"600",value:"oklch(58.8% 0.158 241.966)"},{id:"sky-700",hue:"sky",shade:"700",value:"oklch(50% 0.134 242.749)"},{id:"sky-800",hue:"sky",shade:"800",value:"oklch(44.3% 0.11 240.79)"},{id:"sky-900",hue:"sky",shade:"900",value:"oklch(39.1% 0.09 240.876)"},{id:"sky-950",hue:"sky",shade:"950",value:"oklch(29.3% 0.066 243.157)"},{id:"blue-50",hue:"blue",shade:"50",value:"oklch(97% 0.014 254.604)"},{id:"blue-100",hue:"blue",shade:"100",value:"oklch(93.2% 0.032 255.585)"},{id:"blue-200",hue:"blue",shade:"200",value:"oklch(88.2% 0.059 254.128)"},{id:"blue-300",hue:"blue",shade:"300",value:"oklch(80.9% 0.105 251.813)"},{id:"blue-400",hue:"blue",shade:"400",value:"oklch(70.7% 0.165 254.624)"},{id:"blue-500",hue:"blue",shade:"500",value:"oklch(62.3% 0.214 259.815)"},{id:"blue-600",hue:"blue",shade:"600",value:"oklch(54.6% 0.245 262.881)"},{id:"blue-700",hue:"blue",shade:"700",value:"oklch(48.8% 0.243 264.376)"},{id:"blue-800",hue:"blue",shade:"800",value:"oklch(42.4% 0.199 265.638)"},{id:"blue-900",hue:"blue",shade:"900",value:"oklch(37.9% 0.146 265.522)"},{id:"blue-950",hue:"blue",shade:"950",value:"oklch(28.2% 0.091 267.935)"},{id:"indigo-50",hue:"indigo",shade:"50",value:"oklch(96.2% 0.018 272.314)"},{id:"indigo-100",hue:"indigo",shade:"100",value:"oklch(93% 0.034 272.788)"},{id:"indigo-200",hue:"indigo",shade:"200",value:"oklch(87% 0.065 274.039)"},{id:"indigo-300",hue:"indigo",shade:"300",value:"oklch(78.5% 0.115 274.713)"},{id:"indigo-400",hue:"indigo",shade:"400",value:"oklch(67.3% 0.182 276.935)"},{id:"indigo-500",hue:"indigo",shade:"500",value:"oklch(58.5% 0.233 277.117)"},{id:"indigo-600",hue:"indigo",shade:"600",value:"oklch(51.1% 0.262 276.966)"},{id:"indigo-700",hue:"indigo",shade:"700",value:"oklch(45.7% 0.24 277.023)"},{id:"indigo-800",hue:"indigo",shade:"800",value:"oklch(39.8% 0.195 277.366)"},{id:"indigo-900",hue:"indigo",shade:"900",value:"oklch(35.9% 0.144 278.697)"},{id:"indigo-950",hue:"indigo",shade:"950",value:"oklch(25.7% 0.09 281.288)"},{id:"violet-50",hue:"violet",shade:"50",value:"oklch(96.9% 0.016 293.756)"},{id:"violet-100",hue:"violet",shade:"100",value:"oklch(94.3% 0.029 294.588)"},{id:"violet-200",hue:"violet",shade:"200",value:"oklch(89.4% 0.057 293.283)"},{id:"violet-300",hue:"violet",shade:"300",value:"oklch(81.1% 0.111 293.571)"},{id:"violet-400",hue:"violet",shade:"400",value:"oklch(70.2% 0.183 293.541)"},{id:"violet-500",hue:"violet",shade:"500",value:"oklch(60.6% 0.25 292.717)"},{id:"violet-600",hue:"violet",shade:"600",value:"oklch(54.1% 0.281 293.009)"},{id:"violet-700",hue:"violet",shade:"700",value:"oklch(49.1% 0.27 292.581)"},{id:"violet-800",hue:"violet",shade:"800",value:"oklch(43.2% 0.232 292.759)"},{id:"violet-900",hue:"violet",shade:"900",value:"oklch(38% 0.189 293.745)"},{id:"violet-950",hue:"violet",shade:"950",value:"oklch(28.3% 0.141 291.089)"},{id:"purple-50",hue:"purple",shade:"50",value:"oklch(97.7% 0.014 308.299)"},{id:"purple-100",hue:"purple",shade:"100",value:"oklch(94.6% 0.033 307.174)"},{id:"purple-200",hue:"purple",shade:"200",value:"oklch(90.2% 0.063 306.703)"},{id:"purple-300",hue:"purple",shade:"300",value:"oklch(82.7% 0.119 306.383)"},{id:"purple-400",hue:"purple",shade:"400",value:"oklch(71.4% 0.203 305.504)"},{id:"purple-500",hue:"purple",shade:"500",value:"oklch(62.7% 0.265 303.9)"},{id:"purple-600",hue:"purple",shade:"600",value:"oklch(55.8% 0.288 302.321)"},{id:"purple-700",hue:"purple",shade:"700",value:"oklch(49.6% 0.265 301.924)"},{id:"purple-800",hue:"purple",shade:"800",value:"oklch(43.8% 0.218 303.724)"},{id:"purple-900",hue:"purple",shade:"900",value:"oklch(38.1% 0.176 304.987)"},{id:"purple-950",hue:"purple",shade:"950",value:"oklch(29.1% 0.149 302.717)"},{id:"fuchsia-50",hue:"fuchsia",shade:"50",value:"oklch(97.7% 0.017 320.058)"},{id:"fuchsia-100",hue:"fuchsia",shade:"100",value:"oklch(95.2% 0.037 318.852)"},{id:"fuchsia-200",hue:"fuchsia",shade:"200",value:"oklch(90.3% 0.076 319.62)"},{id:"fuchsia-300",hue:"fuchsia",shade:"300",value:"oklch(83.3% 0.145 321.434)"},{id:"fuchsia-400",hue:"fuchsia",shade:"400",value:"oklch(74% 0.238 322.16)"},{id:"fuchsia-500",hue:"fuchsia",shade:"500",value:"oklch(66.7% 0.295 322.15)"},{id:"fuchsia-600",hue:"fuchsia",shade:"600",value:"oklch(59.1% 0.293 322.896)"},{id:"fuchsia-700",hue:"fuchsia",shade:"700",value:"oklch(51.8% 0.253 323.949)"},{id:"fuchsia-800",hue:"fuchsia",shade:"800",value:"oklch(45.2% 0.211 324.591)"},{id:"fuchsia-900",hue:"fuchsia",shade:"900",value:"oklch(40.1% 0.17 325.612)"},{id:"fuchsia-950",hue:"fuchsia",shade:"950",value:"oklch(29.3% 0.136 325.661)"},{id:"pink-50",hue:"pink",shade:"50",value:"oklch(97.1% 0.014 343.198)"},{id:"pink-100",hue:"pink",shade:"100",value:"oklch(94.8% 0.028 342.258)"},{id:"pink-200",hue:"pink",shade:"200",value:"oklch(89.9% 0.061 343.231)"},{id:"pink-300",hue:"pink",shade:"300",value:"oklch(82.3% 0.12 346.018)"},{id:"pink-400",hue:"pink",shade:"400",value:"oklch(71.8% 0.202 349.761)"},{id:"pink-500",hue:"pink",shade:"500",value:"oklch(65.6% 0.241 354.308)"},{id:"pink-600",hue:"pink",shade:"600",value:"oklch(59.2% 0.249 0.584)"},{id:"pink-700",hue:"pink",shade:"700",value:"oklch(52.5% 0.223 3.958)"},{id:"pink-800",hue:"pink",shade:"800",value:"oklch(45.9% 0.187 3.815)"},{id:"pink-900",hue:"pink",shade:"900",value:"oklch(40.8% 0.153 2.432)"},{id:"pink-950",hue:"pink",shade:"950",value:"oklch(28.4% 0.109 3.907)"},{id:"rose-50",hue:"rose",shade:"50",value:"oklch(96.9% 0.015 12.422)"},{id:"rose-100",hue:"rose",shade:"100",value:"oklch(94.1% 0.03 12.58)"},{id:"rose-200",hue:"rose",shade:"200",value:"oklch(89.2% 0.058 10.001)"},{id:"rose-300",hue:"rose",shade:"300",value:"oklch(81% 0.117 11.638)"},{id:"rose-400",hue:"rose",shade:"400",value:"oklch(71.2% 0.194 13.428)"},{id:"rose-500",hue:"rose",shade:"500",value:"oklch(64.5% 0.246 16.439)"},{id:"rose-600",hue:"rose",shade:"600",value:"oklch(58.6% 0.253 17.585)"},{id:"rose-700",hue:"rose",shade:"700",value:"oklch(51.4% 0.222 16.935)"},{id:"rose-800",hue:"rose",shade:"800",value:"oklch(45.5% 0.188 13.697)"},{id:"rose-900",hue:"rose",shade:"900",value:"oklch(41% 0.159 10.272)"},{id:"rose-950",hue:"rose",shade:"950",value:"oklch(27.1% 0.105 12.094)"},{id:"slate-50",hue:"slate",shade:"50",value:"oklch(98.4% 0.003 247.858)"},{id:"slate-100",hue:"slate",shade:"100",value:"oklch(96.8% 0.007 247.896)"},{id:"slate-200",hue:"slate",shade:"200",value:"oklch(92.9% 0.013 255.508)"},{id:"slate-300",hue:"slate",shade:"300",value:"oklch(86.9% 0.022 252.894)"},{id:"slate-400",hue:"slate",shade:"400",value:"oklch(70.4% 0.04 256.788)"},{id:"slate-500",hue:"slate",shade:"500",value:"oklch(55.4% 0.046 257.417)"},{id:"slate-600",hue:"slate",shade:"600",value:"oklch(44.6% 0.043 257.281)"},{id:"slate-700",hue:"slate",shade:"700",value:"oklch(37.2% 0.044 257.287)"},{id:"slate-800",hue:"slate",shade:"800",value:"oklch(27.9% 0.041 260.031)"},{id:"slate-900",hue:"slate",shade:"900",value:"oklch(20.8% 0.042 265.755)"},{id:"slate-950",hue:"slate",shade:"950",value:"oklch(12.9% 0.042 264.695)"},{id:"gray-50",hue:"gray",shade:"50",value:"oklch(98.5% 0.002 247.839)"},{id:"gray-100",hue:"gray",shade:"100",value:"oklch(96.7% 0.003 264.542)"},{id:"gray-200",hue:"gray",shade:"200",value:"oklch(92.8% 0.006 264.531)"},{id:"gray-300",hue:"gray",shade:"300",value:"oklch(87.2% 0.01 258.338)"},{id:"gray-400",hue:"gray",shade:"400",value:"oklch(70.7% 0.022 261.325)"},{id:"gray-500",hue:"gray",shade:"500",value:"oklch(55.1% 0.027 264.364)"},{id:"gray-600",hue:"gray",shade:"600",value:"oklch(44.6% 0.03 256.802)"},{id:"gray-700",hue:"gray",shade:"700",value:"oklch(37.3% 0.034 259.733)"},{id:"gray-800",hue:"gray",shade:"800",value:"oklch(27.8% 0.033 256.848)"},{id:"gray-900",hue:"gray",shade:"900",value:"oklch(21% 0.034 264.665)"},{id:"gray-950",hue:"gray",shade:"950",value:"oklch(13% 0.028 261.692)"},{id:"zinc-50",hue:"zinc",shade:"50",value:"oklch(98.5% 0 none)"},{id:"zinc-100",hue:"zinc",shade:"100",value:"oklch(96.7% 0.001 286.375)"},{id:"zinc-200",hue:"zinc",shade:"200",value:"oklch(92% 0.004 286.32)"},{id:"zinc-300",hue:"zinc",shade:"300",value:"oklch(87.1% 0.006 286.286)"},{id:"zinc-400",hue:"zinc",shade:"400",value:"oklch(70.5% 0.015 286.067)"},{id:"zinc-500",hue:"zinc",shade:"500",value:"oklch(55.2% 0.016 285.938)"},{id:"zinc-600",hue:"zinc",shade:"600",value:"oklch(44.2% 0.017 285.786)"},{id:"zinc-700",hue:"zinc",shade:"700",value:"oklch(37% 0.013 285.805)"},{id:"zinc-800",hue:"zinc",shade:"800",value:"oklch(27.4% 0.006 286.033)"},{id:"zinc-900",hue:"zinc",shade:"900",value:"oklch(21% 0.006 285.885)"},{id:"zinc-950",hue:"zinc",shade:"950",value:"oklch(14.1% 0.005 285.823)"},{id:"neutral-50",hue:"neutral",shade:"50",value:"oklch(98.5% 0 none)"},{id:"neutral-100",hue:"neutral",shade:"100",value:"oklch(97% 0 none)"},{id:"neutral-200",hue:"neutral",shade:"200",value:"oklch(92.2% 0 none)"},{id:"neutral-300",hue:"neutral",shade:"300",value:"oklch(87% 0 none)"},{id:"neutral-400",hue:"neutral",shade:"400",value:"oklch(70.8% 0 none)"},{id:"neutral-500",hue:"neutral",shade:"500",value:"oklch(55.6% 0 none)"},{id:"neutral-600",hue:"neutral",shade:"600",value:"oklch(43.9% 0 none)"},{id:"neutral-700",hue:"neutral",shade:"700",value:"oklch(37.1% 0 none)"},{id:"neutral-800",hue:"neutral",shade:"800",value:"oklch(26.9% 0 none)"},{id:"neutral-900",hue:"neutral",shade:"900",value:"oklch(20.5% 0 none)"},{id:"neutral-950",hue:"neutral",shade:"950",value:"oklch(14.5% 0 none)"},{id:"stone-50",hue:"stone",shade:"50",value:"oklch(98.5% 0.001 106.423)"},{id:"stone-100",hue:"stone",shade:"100",value:"oklch(97% 0.001 106.424)"},{id:"stone-200",hue:"stone",shade:"200",value:"oklch(92.3% 0.003 48.717)"},{id:"stone-300",hue:"stone",shade:"300",value:"oklch(86.9% 0.005 56.366)"},{id:"stone-400",hue:"stone",shade:"400",value:"oklch(70.9% 0.01 56.259)"},{id:"stone-500",hue:"stone",shade:"500",value:"oklch(55.3% 0.013 58.071)"},{id:"stone-600",hue:"stone",shade:"600",value:"oklch(44.4% 0.011 73.639)"},{id:"stone-700",hue:"stone",shade:"700",value:"oklch(37.4% 0.01 67.558)"},{id:"stone-800",hue:"stone",shade:"800",value:"oklch(26.8% 0.007 34.298)"},{id:"stone-900",hue:"stone",shade:"900",value:"oklch(21.6% 0.006 56.043)"},{id:"stone-950",hue:"stone",shade:"950",value:"oklch(14.7% 0.004 49.25)"},{id:"mauve-50",hue:"mauve",shade:"50",value:"oklch(98.5% 0 none)"},{id:"mauve-100",hue:"mauve",shade:"100",value:"oklch(96% 0.003 325.6)"},{id:"mauve-200",hue:"mauve",shade:"200",value:"oklch(92.2% 0.005 325.62)"},{id:"mauve-300",hue:"mauve",shade:"300",value:"oklch(86.5% 0.012 325.68)"},{id:"mauve-400",hue:"mauve",shade:"400",value:"oklch(71.1% 0.019 323.02)"},{id:"mauve-500",hue:"mauve",shade:"500",value:"oklch(54.2% 0.034 322.5)"},{id:"mauve-600",hue:"mauve",shade:"600",value:"oklch(43.5% 0.029 321.78)"},{id:"mauve-700",hue:"mauve",shade:"700",value:"oklch(36.4% 0.029 323.89)"},{id:"mauve-800",hue:"mauve",shade:"800",value:"oklch(26.3% 0.024 320.12)"},{id:"mauve-900",hue:"mauve",shade:"900",value:"oklch(21.2% 0.019 322.12)"},{id:"mauve-950",hue:"mauve",shade:"950",value:"oklch(14.5% 0.008 326)"},{id:"olive-50",hue:"olive",shade:"50",value:"oklch(98.8% 0.003 106.5)"},{id:"olive-100",hue:"olive",shade:"100",value:"oklch(96.6% 0.005 106.5)"},{id:"olive-200",hue:"olive",shade:"200",value:"oklch(93% 0.007 106.5)"},{id:"olive-300",hue:"olive",shade:"300",value:"oklch(88% 0.011 106.6)"},{id:"olive-400",hue:"olive",shade:"400",value:"oklch(73.7% 0.021 106.9)"},{id:"olive-500",hue:"olive",shade:"500",value:"oklch(58% 0.031 107.3)"},{id:"olive-600",hue:"olive",shade:"600",value:"oklch(46.6% 0.025 107.3)"},{id:"olive-700",hue:"olive",shade:"700",value:"oklch(39.4% 0.023 107.4)"},{id:"olive-800",hue:"olive",shade:"800",value:"oklch(28.6% 0.016 107.4)"},{id:"olive-900",hue:"olive",shade:"900",value:"oklch(22.8% 0.013 107.4)"},{id:"olive-950",hue:"olive",shade:"950",value:"oklch(15.3% 0.006 107.1)"},{id:"mist-50",hue:"mist",shade:"50",value:"oklch(98.7% 0.002 197.1)"},{id:"mist-100",hue:"mist",shade:"100",value:"oklch(96.3% 0.002 197.1)"},{id:"mist-200",hue:"mist",shade:"200",value:"oklch(92.5% 0.005 214.3)"},{id:"mist-300",hue:"mist",shade:"300",value:"oklch(87.2% 0.007 219.6)"},{id:"mist-400",hue:"mist",shade:"400",value:"oklch(72.3% 0.014 214.4)"},{id:"mist-500",hue:"mist",shade:"500",value:"oklch(56% 0.021 213.5)"},{id:"mist-600",hue:"mist",shade:"600",value:"oklch(45% 0.017 213.2)"},{id:"mist-700",hue:"mist",shade:"700",value:"oklch(37.8% 0.015 216)"},{id:"mist-800",hue:"mist",shade:"800",value:"oklch(27.5% 0.011 216.9)"},{id:"mist-900",hue:"mist",shade:"900",value:"oklch(21.8% 0.008 223.9)"},{id:"mist-950",hue:"mist",shade:"950",value:"oklch(14.8% 0.004 228.8)"},{id:"taupe-50",hue:"taupe",shade:"50",value:"oklch(98.6% 0.002 67.8)"},{id:"taupe-100",hue:"taupe",shade:"100",value:"oklch(96% 0.002 17.2)"},{id:"taupe-200",hue:"taupe",shade:"200",value:"oklch(92.2% 0.005 34.3)"},{id:"taupe-300",hue:"taupe",shade:"300",value:"oklch(86.8% 0.007 39.5)"},{id:"taupe-400",hue:"taupe",shade:"400",value:"oklch(71.4% 0.014 41.2)"},{id:"taupe-500",hue:"taupe",shade:"500",value:"oklch(54.7% 0.021 43.1)"},{id:"taupe-600",hue:"taupe",shade:"600",value:"oklch(43.8% 0.017 39.3)"},{id:"taupe-700",hue:"taupe",shade:"700",value:"oklch(36.7% 0.016 35.7)"},{id:"taupe-800",hue:"taupe",shade:"800",value:"oklch(26.8% 0.011 36.5)"},{id:"taupe-900",hue:"taupe",shade:"900",value:"oklch(21.4% 0.009 43.1)"},{id:"taupe-950",hue:"taupe",shade:"950",value:"oklch(14.7% 0.004 49.3)"}]);var Va=12,li=new Map(ze.map(e=>[e.id,e])),Wa=new Map(ze.map(e=>[e.value,e])),Ka=/^#(?:[0-9a-f]{3,4}|[0-9a-f]{6}|[0-9a-f]{8})$/i,Ga=/^(?:rgba?|hsla?|hwb|lab|lch|oklab|oklch|color|color-mix|light-dark)\(.*\)$/i,Ya=/[;{}<>\\]|url\s*\(/i,ai=new WeakMap;function nn(e){let t=ai.get(e);return t||ai.set(e,t=new Set),t}function ci(e,t){let r=String(e??"").trim();if(!r||Ya.test(r)||Gr(r))return!1;let n=t?.defaultView?.CSS;return n&&typeof n.supports=="function"?n.supports("color",r):Ka.test(r)||Ga.test(r)||/^[a-z]+$/i.test(r)}function ui(e){return`${e.hue[0].toUpperCase()}${e.hue.slice(1)} ${e.shade}`}function Jt(e){return{id:e.id,label:ui(e),value:e.value}}function di(e){let t=String(e??"").trim(),r=Wa.get(t);return r?ui(r):t}function Xa(e,t){if(!e||e.length===0)return{swatches:ze.map(Jt),custom:!0,full:!0};let r=[],n=new Set,o=!1,i=s=>{n.has(s.value)||(n.add(s.value),r.push(s))};for(let s of e)s==="custom"?o=!0:li.has(s)?i(Jt(li.get(s))):si.includes(s)?ze.filter(l=>l.hue===s).forEach(l=>i(Jt(l))):ii.includes(s)?ze.filter(l=>l.shade===s).forEach(l=>i(Jt(l))):ci(s,t)?i({id:null,label:s,value:s}):console.warn(`[hypercms] color option "${s}" is not a Tailwind color, hue, shade, CSS color or "custom"; ignoring it`);return{swatches:r,custom:o,full:!1}}function hi(e,{value:t,tokens:r,path:n,doc:o}){let{swatches:i,custom:s,full:l}=Xa(r,o),a=t==null?"":String(t).trim();a&&!i.some(h=>h.value===a)&&i.unshift({id:null,label:di(a),value:a});let u=i.length<=Va;e.setAttribute("data-hcms-color-path",n),e.classList.toggle("hcms-color--inline",u);let d=e.querySelector(".hcms-color-grid");if(d){d.classList.toggle("hcms-color-grid--full",l);for(let h of i){let m=o.createElement("button");m.type="button",m.className="hcms-color-swatch",m.setAttribute("data-hcms-action","color-pick"),m.setAttribute("data-hcms-color",h.value),m.setAttribute("aria-label",h.label),m.setAttribute("title",h.id||h.label),m.setAttribute("style",`--hcms-swatch: ${h.value}`),d.appendChild(m)}}s||e.querySelector(".hcms-color-custom")?.remove();let f=e.querySelector(".hcms-color-current");u&&f&&(f.removeAttribute("data-hcms-action"),f.removeAttribute("aria-expanded")),on(e,u||nn(o).has(n)),mi(e,a)}function mi(e,t){let r=t==null?"":String(t).trim();e.toggleAttribute("data-hcms-color-empty",r==="");let n=e.querySelector(".hcms-color-chip");n&&(r?n.setAttribute("style",`--hcms-swatch: ${r}`):n.removeAttribute("style"));let o=e.querySelector(".hcms-color-name");o&&(o.textContent=r?di(r):"Not set"),e.querySelectorAll(".hcms-color-swatch").forEach(l=>{l.setAttribute("aria-pressed",String(l.getAttribute("data-hcms-color")===r))});let i=e.querySelector('[data-hcms-color-input="text"]');i&&i.ownerDocument?.activeElement!==i&&(i.value=r);let s=e.querySelector('[data-hcms-color-input="native"]');s&&/^#[0-9a-f]{6}$/i.test(r)&&(s.value=r.toLowerCase())}function on(e,t){let r=e.querySelector(".hcms-color-panel");r&&(r.hidden=!t);let n=e.querySelector(".hcms-color-current[data-hcms-action]");n&&n.setAttribute("aria-expanded",String(t))}function Zt(e){return e?.closest?e.closest("[data-hcms-color-path]"):null}function Ja(e){return e.querySelector('input[type="hidden"][data-hcms-field]')}function fi(e,t){let r=e.querySelector(":scope > .hcms-error");r&&(r.textContent=t||"",r.hidden=!t)}function sn(e,t){let r=Ja(e);return!r||(fi(e,null),mi(e,t),r.value===t)?null:(r.value=t,r)}function pi(e){let t=Zt(e);return t?(t.classList.contains("hcms-color--inline")||(nn(t.ownerDocument).delete(t.getAttribute("data-hcms-color-path")),on(t,!1)),sn(t,e.getAttribute("data-hcms-color")||"")):null}function gi(e){let t=Zt(e);return t?sn(t,""):null}function bi(e){let t=Zt(e);if(!t)return;let r=t.getAttribute("data-hcms-color-path"),n=nn(t.ownerDocument),o=!n.has(r);o?n.add(r):n.delete(r),on(t,o)}function ki(e){let t=Zt(e);if(!t)return null;let r=String(e.value||"").trim();return r!==""&&!ci(r,t.ownerDocument)?(fi(t,`"${r}" is not a CSS color`),null):sn(t,r)}var an=["sans","serif","display","mono"],I="ui-sans-serif, system-ui, sans-serif",re="ui-serif, Georgia, serif",Ue="ui-monospace, SFMono-Regular, Menlo, monospace",oe=Object.freeze([{slug:"inter",family:"Inter",group:"sans",fallback:I,request:"ital,wght@0,100..900;1,100..900"},{slug:"roboto",family:"Roboto",group:"sans",fallback:I,request:"ital,wght@0,100..900;1,100..900"},{slug:"open-sans",family:"Open Sans",group:"sans",fallback:I,request:"ital,wght@0,300..800;1,300..800"},{slug:"lato",family:"Lato",group:"sans",fallback:I,request:"ital,wght@0,100;0,300;0,400;0,700;0,900;1,100;1,300;1,400;1,700;1,900"},{slug:"montserrat",family:"Montserrat",group:"sans",fallback:I,request:"ital,wght@0,100..900;1,100..900"},{slug:"poppins",family:"Poppins",group:"sans",fallback:I,request:"ital,wght@0,100;0,200;0,300;0,400;0,500;0,600;0,700;0,800;0,900;1,100;1,200;1,300;1,400;1,500;1,600;1,700;1,800;1,900"},{slug:"nunito",family:"Nunito",group:"sans",fallback:I,request:"ital,wght@0,200..1000;1,200..1000"},{slug:"raleway",family:"Raleway",group:"sans",fallback:I,request:"ital,wght@0,100..900;1,100..900"},{slug:"dm-sans",family:"DM Sans",group:"sans",fallback:I,request:"ital,wght@0,100..1000;1,100..1000"},{slug:"work-sans",family:"Work Sans",group:"sans",fallback:I,request:"ital,wght@0,100..900;1,100..900"},{slug:"source-sans-3",family:"Source Sans 3",group:"sans",fallback:I,request:"ital,wght@0,200..900;1,200..900"},{slug:"noto-sans",family:"Noto Sans",group:"sans",fallback:I,request:"ital,wght@0,100..900;1,100..900"},{slug:"plus-jakarta-sans",family:"Plus Jakarta Sans",group:"sans",fallback:I,request:"ital,wght@0,200..800;1,200..800"},{slug:"manrope",family:"Manrope",group:"sans",fallback:I,request:"wght@200..800"},{slug:"outfit",family:"Outfit",group:"sans",fallback:I,request:"wght@100..900"},{slug:"space-grotesk",family:"Space Grotesk",group:"sans",fallback:I,request:"wght@300..700"},{slug:"rubik",family:"Rubik",group:"sans",fallback:I,request:"ital,wght@0,300..900;1,300..900"},{slug:"ubuntu",family:"Ubuntu",group:"sans",fallback:I,request:"ital,wght@0,300;0,400;0,500;0,700;1,300;1,400;1,500;1,700"},{slug:"merriweather",family:"Merriweather",group:"serif",fallback:re,request:"ital,wght@0,300..900;1,300..900"},{slug:"lora",family:"Lora",group:"serif",fallback:re,request:"ital,wght@0,400..700;1,400..700"},{slug:"playfair-display",family:"Playfair Display",group:"serif",fallback:re,request:"ital,wght@0,400..900;1,400..900"},{slug:"libre-baskerville",family:"Libre Baskerville",group:"serif",fallback:re,request:"ital,wght@0,400..700;1,400..700"},{slug:"source-serif-4",family:"Source Serif 4",group:"serif",fallback:re,request:"ital,wght@0,200..900;1,200..900"},{slug:"pt-serif",family:"PT Serif",group:"serif",fallback:re,request:"ital,wght@0,400;0,700;1,400;1,700"},{slug:"noto-serif",family:"Noto Serif",group:"serif",fallback:re,request:"ital,wght@0,100..900;1,100..900"},{slug:"crimson-pro",family:"Crimson Pro",group:"serif",fallback:re,request:"ital,wght@0,200..900;1,200..900"},{slug:"eb-garamond",family:"EB Garamond",group:"serif",fallback:re,request:"ital,wght@0,400..800;1,400..800"},{slug:"cormorant-garamond",family:"Cormorant Garamond",group:"serif",fallback:re,request:"ital,wght@0,300..700;1,300..700"},{slug:"bebas-neue",family:"Bebas Neue",group:"display",fallback:I,request:"wght@400"},{slug:"oswald",family:"Oswald",group:"display",fallback:I,request:"wght@200..700"},{slug:"anton",family:"Anton",group:"display",fallback:I,request:"wght@400"},{slug:"abril-fatface",family:"Abril Fatface",group:"display",fallback:re,request:"wght@400"},{slug:"lobster",family:"Lobster",group:"display",fallback:"cursive",request:"wght@400"},{slug:"bungee",family:"Bungee",group:"display",fallback:I,request:"wght@400"},{slug:"roboto-mono",family:"Roboto Mono",group:"mono",fallback:Ue,request:"ital,wght@0,100..700;1,100..700"},{slug:"jetbrains-mono",family:"JetBrains Mono",group:"mono",fallback:Ue,request:"ital,wght@0,100..800;1,100..800"},{slug:"fira-code",family:"Fira Code",group:"mono",fallback:Ue,request:"wght@300..700"},{slug:"source-code-pro",family:"Source Code Pro",group:"mono",fallback:Ue,request:"ital,wght@0,200..900;1,200..900"},{slug:"ibm-plex-mono",family:"IBM Plex Mono",group:"mono",fallback:Ue,request:"ital,wght@0,100;0,200;0,300;0,400;0,500;0,600;0,700;1,100;1,200;1,300;1,400;1,500;1,600;1,700"},{slug:"space-mono",family:"Space Mono",group:"mono",fallback:Ue,request:"ital,wght@0,400;0,700;1,400;1,700"}]),Be=Object.freeze({slug:"system",family:null,group:"sans",fallback:"system-ui, sans-serif"}),Za=new Map(oe.map(e=>[e.family.toLowerCase(),e])),df=new Map(oe.map(e=>[e.slug,e]));function vt(e){return!e||e.slug==="system"?Be.fallback:`'${e.family}', ${e.fallback}`}function yi(e){return e==null?null:String(e).split(",")[0].trim().replace(/^['"]|['"]$/g,"")||null}function ln(e){let t=new Map;if(!e)return t;let r=String(e).split("?")[1]||"";for(let n of r.split("&")){if(!n.startsWith("family="))continue;let o=decodeURIComponent(n.slice(7).split(":")[0].replace(/\+/g," ")).trim();o&&t.set(o.toLowerCase(),n)}return t}function vi(e,t){let r=ln(e),n=ln(t);return r.size===n.size&&[...r.keys()].every(o=>n.has(o))}function cn(e){let t=yi(e);return t&&Za.get(t.toLowerCase())||null}function Qt(e,t=null){let r=ln(t),n=new Map;for(let i of e||[]){let s=cn(i);if(s){n.set(s.family.toLowerCase(),`family=${s.family.replace(/ /g,"+")}:${s.request}`);continue}let l=yi(i),a=l&&r.get(l.toLowerCase());a&&n.set(l.toLowerCase(),a)}return n.size===0?null:`https://fonts.googleapis.com/css2?${[...n.entries()].sort(([i],[s])=>i.localeCompare(s)).map(([,i])=>i).join("&")}&display=swap`}var Qa={sans:"Sans",serif:"Serif",display:"Display",mono:"Mono"},ec="link[data-hcms-font-preview]",xi=new WeakMap;function dn(e){let t=xi.get(e);return t||xi.set(e,t=new Set),t}function tc(e){return String(e??"").trim()===Be.fallback}function wi(e){let t=String(e??"").trim();if(!t)return"Not set";if(tc(t))return"System";let r=cn(t);return r?r.family:"Current font"}function rc(e){if(!e||e.length===0)return{system:!0,fonts:[...oe]};let t=!1,r=new Set;for(let n of e)n==="system"?t=!0:an.includes(n)?oe.filter(o=>o.group===n).forEach(o=>r.add(o)):oe.some(o=>o.slug===n)?r.add(oe.find(o=>o.slug===n)):console.warn(`[hypercms] font option "${n}" is not a catalog font, a group or "system"; ignoring it`);return{system:t,fonts:oe.filter(n=>r.has(n))}}function un(e,{value:t,label:r,family:n}){let o=e.createElement("button");return o.type="button",o.className="hcms-font-option",o.setAttribute("role","option"),o.setAttribute("data-hcms-action","font-pick"),o.setAttribute("data-hcms-font",t),o.setAttribute("style",`font-family: ${n}`),o.textContent=r,o}function Ai(e,{value:t,tokens:r,path:n,doc:o}){let{system:i,fonts:s}=rc(r),l=t==null?"":String(t).trim();e.setAttribute("data-hcms-font-path",n);let a=e.querySelector(".hcms-font-list");if(a){let u=new Set([...i?[Be.fallback]:[],...s.map(vt)]);l&&!u.has(l)&&a.appendChild(un(o,{value:l,label:wi(l),family:l})),i&&a.appendChild(un(o,{value:Be.fallback,label:"System",family:Be.fallback}));for(let d of an){let f=s.filter(m=>m.group===d);if(!f.length)continue;let h=o.createElement("div");h.className="hcms-font-group",h.textContent=Qa[d],a.appendChild(h);for(let m of f)a.appendChild(un(o,{value:vt(m),label:m.family,family:vt(m)}))}}hn(e,dn(o).has(n)),Si(e,l)}function Si(e,t){let r=t==null?"":String(t).trim(),n=e.querySelector(".hcms-font-name");n&&(n.textContent=wi(r),r?n.setAttribute("style",`font-family: ${r}`):n.removeAttribute("style")),e.querySelectorAll(".hcms-font-option").forEach(o=>{o.setAttribute("aria-selected",String(o.getAttribute("data-hcms-font")===r))})}function hn(e,t){let r=e.querySelector(".hcms-font-panel");r&&(r.hidden=!t),e.querySelector(".hcms-font-current")?.setAttribute("aria-expanded",String(t)),t&&nc(e.ownerDocument)}function nc(e){let t=e?.head;if(!t||t.querySelector(ec))return;let r=Qt(oe.map(vt)),n=[...new Set(oe.map(i=>i.family).join(""))].join(""),o=e.createElement("link");o.rel="stylesheet",o.href=`${r}&text=${encodeURIComponent(n)}`,o.setAttribute("data-hcms-font-preview","");for(let i of["editor-ui","no-save","save-remove","snapshot-remove"])o.setAttribute(i,"");t.appendChild(o)}function Ei(e){return e?.closest?e.closest("[data-hcms-font-path]"):null}function _i(e){let t=Ei(e);if(!t)return;let r=t.getAttribute("data-hcms-font-path"),n=dn(t.ownerDocument),o=!n.has(r);o?n.add(r):n.delete(r),hn(t,o)}function Ci(e){let t=Ei(e);if(!t)return null;let r=t.querySelector('input[type="hidden"][data-hcms-field]');if(!r)return null;dn(t.ownerDocument).delete(t.getAttribute("data-hcms-font-path")),hn(t,!1);let n=e.getAttribute("data-hcms-font")||"";return Si(t,n),r.value===n?null:(r.value=n,r)}var Z=e=>`<svg viewBox="0 0 32 20" width="32" height="20" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" aria-hidden="true">${e}</svg>`,F=(e,t,r,n,o="")=>`<rect x="${e}" y="${t}" width="${r}" height="${n}" rx="1"${o}/>`,H=(e,t,r)=>`<path d="M${e} ${t}H${r}"/>`,mn=' fill="currentColor" fill-opacity="0.3"',Ti=Object.freeze({"cols-1":{label:"One column",svg:Z(F(2,2,28,16))},"cols-2":{label:"Two columns",svg:Z(F(2,2,13,16)+F(17,2,13,16))},"cols-3":{label:"Three columns",svg:Z(F(2,2,8,16)+F(12,2,8,16)+F(22,2,8,16))},"cols-4":{label:"Four columns",svg:Z(F(2,2,5.5,16)+F(9.5,2,5.5,16)+F(17,2,5.5,16)+F(24.5,2,5.5,16))},"media-left":{label:"Image left",svg:Z(F(2,3,12,14,mn)+H(18,6,30)+H(18,10,30)+H(18,14,26))},"media-right":{label:"Image right",svg:Z(F(18,3,12,14,mn)+H(2,6,14)+H(2,10,14)+H(2,14,10))},"media-top":{label:"Image on top",svg:Z(F(2,2,28,8,mn)+H(2,14,30)+H(2,17,22))},list:{label:"List",svg:Z(H(3,5,4)+H(8,5,29)+H(3,10,4)+H(8,10,29)+H(3,15,4)+H(8,15,29))},cards:{label:"Cards",svg:Z(F(2,2,13,7)+F(17,2,13,7)+F(2,11,13,7)+F(17,11,13,7))},centered:{label:"Centered",svg:Z(H(6,5,26)+H(9,10,23)+H(11,15,21))},"card-flat":{label:"Flat",svg:Z(F(5,3,22,14))},"card-raised":{label:"Raised",svg:Z(F(7,5,22,14,' stroke-opacity="0.35"')+F(4,2,22,14,' fill="var(--mirk-bg, transparent)"'))}});function oc(e){return(e||[]).map(t=>{let r=t.indexOf(":"),n=r===-1?t:t.slice(0,r),o=r===-1?t:t.slice(r+1),i=Ti[o]||null;return{value:n,icon:i,label:i?i.label:_e(n)}})}function Oi(e,{tokens:t,value:r,name:n,doc:o}){let i=oc(t),s=r==null?"":String(r);s!==""&&!i.some(a=>a.value===s)&&i.unshift({value:s,icon:null,label:_e(s)});let l=e.querySelector(".hcms-variant-option");if(!l||!l.parentNode)return i.length;for(let a of i){let u=l.cloneNode(!0),d=u.querySelector('input[type="radio"]');d&&(d.value=a.value,d.name=n);let f=u.querySelector(".hcms-variant-icon");f&&(a.icon?f.innerHTML=a.icon.svg:f.remove());let h=u.querySelector(".hcms-variant-label");h&&(h.textContent=a.label),u.setAttribute("title",a.label),l.parentNode.insertBefore(u,l)}return l.remove(),i.length}function fn(e){if(!e)return"";let t=String(e).split(/[?#]/)[0],r=t.split("/").pop()||t;try{return decodeURIComponent(r)}catch{return r}}function He({pageRules:e,formRules:t,data:r,doc:n}){let o=n.createDocumentFragment(),i=ic(e,r,n);i&&o.appendChild(i);let s=er(e,[],r,n,e);return s&&o.appendChild(s),o}function ic(e,t,r){let n=[];if(a(e,t,[]),!n.length)return null;let o=J(r,"@object");if(!o)return null;let i=Oe(o,r);i.setAttribute("data-hcms-page-style","");let s=i.querySelector("[data-hcms-label]");s&&(s.textContent="Page style");let l=i.querySelector(".hcms-object-fields")||i;for(let[u,d,f]of n){let h=er(u,d,f,r,e);h&&l.appendChild(h)}return i;function a(u,d,f){if(typeof u=="string"){f.length&&kt(u,r,f,e)==="page"&&n.push([u,f,d]);return}if(!(!u||typeof u!="object"||Array.isArray(u))){if(f.length){let h=Ce(f,"@object",r);if(h&&De(h))return}for(let[h,m]of Object.entries(u))a(m,d==null?null:d[h],[...f,h])}}}function Li(e,t,r,n){let o=[],i=[];for(let[s,l]of Object.entries(e)){let a=kt(l,r,[...t,s],n);a!=="page"&&(a?o:i).push([s,l])}return[...o,...i]}function Ni({shape:e,itemShape:t,pathArr:r,data:n,doc:o,itemKey:i,pageRules:s}){if(e==="object-array-item")return qi(t,r,n,o,s);if(e==="scalar-array-item")return Pi(r,n,o,i||null);throw new Error(`hypercms: buildItem called with unknown shape "${e}"`)}function er(e,t,r,n,o){let i=be(e);return i==="scalar"?sc(e,t,r,n,o):i==="object"?cc(e,t,r,n,o):i==="object-array"?uc(e,t,r,n,o):i==="scalar-array"?dc(e,t,r,n):null}function sc(e,t,r,n,o){let i=ge(e,n,t,o),s=Ce(t,i,n);if(!s)throw new Error(`hypercms: missing template for scalar at "${t.join(".")}"`);let l=Xo(e,n,t,o);l==="@number"&&i==="@scalar"&&console.info(`[hypercms] field "${t.join(".")}" declares component "@number" but its value isn't a plain number; rendering a text input so the value is preserved`),(l==="@checkbox"||l==="@toggle")&&i==="@scalar"&&console.info(`[hypercms] field "${t.join(".")}" declares component "${l}" but its value isn't true/false; rendering a text input so the value is preserved`),Mi(s,l===i?l:null,t);let a=Oe(s,n);Ve(a,t);let u=s.getAttribute?.("data-hcms-tpl");if((i==="@select"||i==="@radio")&&u===i&&lc(a,e,t,r,n,i,o),i==="@variant"&&u==="@variant"){let d=yt(e,n,t,o);d||ji(a,"data-hcms-options required (value:icon tokens)"),Oi(a,{tokens:d,value:r,name:tr(t.join(".")),doc:n})===0&&a.querySelector(".hcms-variant-option")?.remove()}if(i==="@image"&&u==="@image"){let d=Jo(e,n,t,o);d!=null&&!a.hasAttribute("data-hcms-crop")&&a.setAttribute("data-hcms-crop",d)}return hc(a,ae(t)),rr(a,ae(t)),nr(a,ae(t)),zi(a,r),i==="@color"&&u==="@color"&&hi(a,{value:r,tokens:yt(e,n,t,o),path:t.join("."),doc:n}),i==="@font"&&u==="@font"&&Ai(a,{value:r,tokens:yt(e,n,t,o),path:t.join("."),doc:n}),i==="@file"&&ac(a),a}function Mi(e,t,r){if(!t)return;let n=e.getAttribute?.("data-hcms-tpl");n&&n!==t&&console.info(`[hypercms] field "${r.join(".")}" declares component "${t}" but custom template "${n}" wins`)}function lc(e,t,r,n,o,i,s){let l=yt(t,o,r,s),a=l?[...l]:[],u=n==null?"":String(n);if(u!==""&&!a.includes(u)&&a.unshift(u),!l&&(ji(e,"data-hcms-options required (space-separated values)"),a.length===0)){e.querySelector(".mirk-radio")?.remove();return}if(i==="@select"){let h=e.querySelector("select[data-hcms-field]");if(!h)return;for(let m of a){let c=o.createElement("option");c.value=m,c.textContent=_e(m),h.appendChild(c)}return}let d=e.querySelector(".mirk-radio");if(!d||!d.parentNode)return;let f=tr(r.join("."));for(let h of a){let m=d.cloneNode(!0),c=m.querySelector('input[type="radio"]');c&&(c.value=h,c.name=f);let p=m.querySelector(".mirk-radio__label");p&&(p.textContent=_e(h)),d.parentNode.insertBefore(m,d)}d.remove()}function tr(e){return"hcms-"+String(e).replace(/[^A-Za-z0-9_-]/g,"-")}function ji(e,t){let r=e.querySelector?e.querySelector(".hcms-error"):null;r&&(r.textContent=t,r.hidden=!1)}function ac(e){let t=e.querySelector?e.querySelector("a.mirk-file__name[data-hcms-field]"):null;t&&(t.textContent=fn(t.getAttribute("href")))}function cc(e,t,r,n,o){let i=Ce(t,"@object",n);if(!i)throw new Error(`hypercms: missing template for object at "${t.join(".")}"`);let s=Oe(i,n);if(Ve(s,t),rr(s,ae(t)),nr(s,ae(t)),De(i))return Bi(s,e,t),Ui(s,e,r),s;let l=or(s,".hcms-object-fields",i,t);for(let[a,u]of Li(e,t,n,o)){let d=r==null?null:r[a],f=er(u,[...t,a],d,n,o);f&&l.appendChild(f)}return t.length&&!l.children.length&&Object.keys(e).length?null:s}function uc(e,t,r,n,o){let i=Ce(t,"@object-array",n);if(!i)throw new Error(`hypercms: missing template for object-array at "${t.join(".")}"`);let s=Oe(i,n);Ve(s,t),rr(s,ae(t)),nr(s,ae(t)),$i(s,i),Di(s,i,t);let l=or(s,".hcms-array-items",i,t),[,a]=e;return(Array.isArray(r)?r:[]).forEach((d,f)=>{let h=qi(a,[...t,f],d,n,o);h&&l.appendChild(h)}),Fi(s),s}function qi(e,t,r,n,o){let i=Ii(t,"object-array-item",n);if(!i)throw new Error(`hypercms: missing item template for "${t.join(".")}"`);let s=Oe(i,n);if(s.setAttribute("data-hcms-card",""),s.classList.contains("hcms-card")||s.classList.add("hcms-card"),Ve(s,t),De(i))return e&&typeof e=="object"&&!Array.isArray(e)&&(Bi(s,e,t),Ui(s,e,r)),s;let l=or(s,".hcms-card-fields",i,t);if(e&&typeof e=="object"&&!Array.isArray(e))for(let[a,u]of Li(e,t,n,o)){let d=r==null?null:r[a],f=er(u,[...t,a],d,n,o);f&&l.appendChild(f)}return s}function dc(e,t,r,n){let o=Ht(e,n),i=Vt(e,t,n),s=o?o.array:"@scalar-array",l=Ce(t,s,n);if(!l)throw new Error(`hypercms: missing template for scalar-array at "${t.join(".")}"`);Mi(l,o?o.array:null,t);let a=Oe(l,n);Ve(a,t),rr(a,ae(t)),nr(a,ae(t)),$i(a,l),Di(a,l,t),i&&a.setAttribute("data-hcms-item-tpl",i.item);let u=or(a,".hcms-array-items",l,t);return(Array.isArray(r)?r:[]).forEach((f,h)=>{let m=Pi([...t,h],f,n,i?i.item:null);m&&u.appendChild(m)}),Fi(a),a}function Pi(e,t,r,n){let o=Ii(e,"scalar-array-item",r,n);if(!o)throw new Error(`hypercms: missing item template for "${e.join(".")}"`);let i=Oe(o,r);return i.setAttribute("data-hcms-array-item",""),i.classList.contains("hcms-array-item")||i.classList.add("hcms-array-item"),Ve(i,e),zi(i,t),i}function Ii(e,t,r,n){let o=e.map(i=>typeof i=="number"?"*":i).join(".");return J(r,o)||n&&J(r,n)||J(r,"@"+t)}function Oe(e,t){let r=e.content||e,n=t.createElement("div");return n.appendChild(r.cloneNode(!0)),n.firstElementChild||n}function Ve(e,t){e.setAttribute("data-hcms-path",t.join(".")),e.setAttribute("data-hcms-path-shape",Gt(t))}function hc(e,t){let r=t==null?"":String(t);if(e.matches&&e.matches("[data-hcms-field]")){e.getAttribute("data-hcms-field")||e.setAttribute("data-hcms-field",r);return}(e.querySelectorAll?e.querySelectorAll("[data-hcms-field]"):[]).forEach(o=>{o.getAttribute("data-hcms-field")||o.setAttribute("data-hcms-field",r)})}function rr(e,t){t==null||t===""||!e.setAttribute||e.hasAttribute?.("data-hcms-field")||e.setAttribute("data-hcms-field",String(t))}function nr(e,t){if(t==null||t==="")return;(e.querySelectorAll?e.querySelectorAll("[data-hcms-label]"):[]).forEach(n=>{(n.textContent||"").trim()===""&&(n.textContent=_e(String(t)))})}function $i(e,t){["data-hcms-no-add","data-hcms-no-remove","data-hcms-no-reorder"].forEach(r=>{t.hasAttribute(r)&&e.setAttribute(r,"")}),["data-hcms-min-items","data-hcms-max-items"].forEach(r=>{t.hasAttribute(r)&&e.setAttribute(r,t.getAttribute(r))})}function Fi(e){let t=e.querySelector?e.querySelector(".hcms-array-items"):null;if(!t)return;let r=Array.from(t.querySelectorAll(":scope > [data-hcms-card], :scope > [data-hcms-array-item]")),n=r.length,o=Ri(e,"data-hcms-max-items"),i=Ri(e,"data-hcms-min-items"),s=e.hasAttribute("data-hcms-no-add"),l=e.hasAttribute("data-hcms-no-remove"),a=e.hasAttribute("data-hcms-no-reorder"),u=e.querySelector('[data-hcms-action="add"]');u&&(u.hidden=s||o!=null&&n>=o),r.forEach((d,f)=>{let h=d.querySelector('[data-hcms-action="remove"]');h&&(h.hidden=l||i!=null&&n<=i);let m=d.querySelector('[data-hcms-action="move-up"]');m&&(m.hidden=a||f===0);let c=d.querySelector('[data-hcms-action="move-down"]');c&&(c.hidden=a||f===n-1)})}function Ri(e,t){if(!e||!e.hasAttribute(t))return null;let r=parseInt(e.getAttribute(t),10);return Number.isFinite(r)?r:null}function Di(e,t,r){if(e.hasAttribute("data-hcms-no-reorder")||t.hasAttribute("data-hcms-no-reorder"))return;let n=e.querySelector(".hcms-array-items");if(!n)return;let o="hcms-"+r.join(".");n.setAttribute("sortable",o),n.setAttribute("onsorted","hypercmsCommit && hypercmsCommit()")}function ae(e){return e.length?e[e.length-1]:null}function zi(e,t){let r=mc(e);if(r.length!==0)for(let n of r)Hi(n,t)}function mc(e){if(!e)return[];let t=[];return e.matches?.("[data-hcms-field]")&&fc(e)&&t.push(e),(e.querySelectorAll?e.querySelectorAll("input[data-hcms-field], textarea[data-hcms-field], select[data-hcms-field], img[data-hcms-field], a[data-hcms-field], [contenteditable][data-hcms-field]"):[]).forEach(n=>t.push(n)),t}function fc(e){let t=(e.tagName||"").toUpperCase();return!!(t==="INPUT"||t==="TEXTAREA"||t==="SELECT"||t==="IMG"||t==="A"||e.hasAttribute?.("contenteditable"))}function Ui(e,t,r){(e.querySelectorAll?e.querySelectorAll("[data-hcms-field]"):[]).forEach(o=>{let i=o.getAttribute("data-hcms-field");if(!i)return;if(!t||typeof t!="object"||!(i in t)){console.warn(`[hypercms] inline template field "${i}" is not in the rule shape; ignoring`);return}let s=r==null?null:r[i];Hi(o,s)})}function Bi(e,t,r){if(!e.querySelectorAll)return;e.querySelectorAll("[data-hcms-field]").forEach(o=>{let i=o.getAttribute("data-hcms-field");if(!i||t&&typeof t=="object"&&!(i in t))return;let s=[...r,i].join(".");o.setAttribute("data-hcms-path",s),o.setAttribute("data-hcms-path-shape",Gt([...r,i]))})}function or(e,t,r,n){if(!e.querySelector)return e;let o=e.querySelector(t);if(o)return o;let i=r?.getAttribute?.("data-hcms-tpl")||n.join(".");throw new Error(`hypercms: template "${i}" is in slotted mode but has no ${t} element`)}function Hi(e,t){let r=Yt(e),n=(e.tagName||"").toUpperCase(),o=(e.getAttribute("type")||"").toLowerCase();if(n==="INPUT"&&o==="radio"){e.checked=e.value!=null&&String(e.value)===String(t??"");return}if(r==="checked"){e.checked=t===!0||t==="true";return}if(r){e[r]=t==null?"":String(t);return}e.textContent=t==null?"":String(t)}var pc={Mutation:(e,t)=>e?.Mutation??t?.Mutation,undo:(e,t)=>e?.undo??t?.undo,onPrepareForSave:(e,t)=>e?.addDocumentTransform??t?.onPrepareForSave,onSnapshot:(e,t)=>e?.onSnapshot??t?.onSnapshot,consent:(e,t)=>e?.confirm??t?.consent,RichClay:(e,t)=>e?.RichClay??t?.RichClay,quickcrop:(e,t)=>e?.quickcrop??t?.quickcrop,upload:(e,t)=>e?.upload??(t?.uploadFileBasic?bc(t.uploadFileBasic):null)},gc={402:"payment-required",413:"too-large",415:"unsupported-type",401:"unauthorized",403:"forbidden",404:"not-found"},pn=()=>({ok:!1,msg:"Upload cancelled",msgType:"skipped",code:"aborted",uploads:[]});function bc(e){return async function(r,{onProgress:n,signal:o}={}){if(o?.aborted)return pn();try{let i=await e(r,{onProgress:l=>{n?.({loaded:null,total:null,percent:l})}});if(o?.aborted)return pn();let s=i&&i.uploads||[];return typeof s[0]?.url!="string"?{ok:!1,msg:"The host accepted the file but did not say where it put it",msgType:"error",code:"bad-response",uploads:[]}:{ok:!0,msg:i.msg||"Uploaded",msgType:i.msgType||"success",code:i.code||null,uploads:s}}catch(i){if(o?.aborted)return pn();let s={};try{s=JSON.parse(i?.response||"{}")}catch{s={}}let l=s.code||gc[i?.status]||"error";return{ok:!1,msg:i&&i.message||"Upload failed",msgType:"error",code:l,uploads:[]}}}}function D(e,t){let r=pc[e];if(!r)throw new Error(`hypercms: unknown platform capability "${e}"`);let n=t||(typeof window<"u"?window:null);return n&&r(n.clay,n.hyperclay)||null}var Vi=["clay:mutation-ready","hyperclay:mutation-ready"],Wi=["clay:sync-applied","hyperclay:livesync-applied"],Ki=["clay:ready","hyperclay:ready"];function We(e,t,r){let n=null,o=i=>{n!==null&&n!==i.type||(n=i.type,queueMicrotask(()=>{n=null}),r(i))};for(let i of t)e.addEventListener(i,o);return()=>{for(let i of t)e.removeEventListener(i,o)}}var ne="data-hcms-bound",ke="data-hcms-bound-id",gn=new Map;function Gi(e,t){gn.set(e,t)}function Yi(e){gn.delete(e)}function Xi(e){let t=e.getAttribute(ke);if(!t)return null;let r=gn.get(t);return!r||!r.restorable()?null:r.originalHTML}var Re="data-hcms-owns-richclay";function Ke(e){return e&&e.richclay&&e.richclay.RichClay||D("RichClay",e)||(e&&typeof e.RichClay=="function"?e.RichClay:null)}function Ji(e,t,r){!e||typeof e.setAttribute!="function"||(e.setAttribute(ne,t?"rich":"plain"),r&&e.setAttribute(Re,"true"))}function Zi(e,t){if(!e||typeof e.querySelectorAll!="function")return;let r=e.querySelectorAll(`[${ne}]`);if(!r.length)return;let n=Ke(t);if(n&&typeof n.stripFromClone=="function")try{n.stripFromClone(e)}catch(o){console.warn("[hypercms] richclay strip failed; editor state may reach the save",o)}for(let o of r)o.getAttribute(Re)==="true"&&o.removeAttribute("data-richclay"),o.removeAttribute(Re),o.removeAttribute(ne)}function Qi(e,t){if(!e||typeof e.removeAttribute!="function")return;let r=Ke(t);if(r&&typeof r.stripElement=="function")try{r.stripElement(e)}catch(n){console.warn("[hypercms] richclay element strip failed; the clone stays editable",n)}else e.removeAttribute("contenteditable"),e.removeAttribute("no-undo");e.getAttribute(Re)==="true"&&e.removeAttribute("data-richclay"),e.removeAttribute(Re),e.removeAttribute(ne),e.removeAttribute(ke)}function es(e,t){if(!t||e==null)return e;return r(e);function r(n){if(typeof n=="string"){if(n.endsWith("[]")||j(n)!==-1)return n;let o=null;try{o=t.querySelector(n)}catch{return n}return o&&o.children.length>0?n+"@innerHTML":n}if(Array.isArray(n))return n;if(n&&typeof n=="object"){let o=Object.create(null);for(let[i,s]of Object.entries(n))o[i]=r(s);return o}return n}}function ts(e,t){if(!t||e==null)return e;return r(e,[t]);function r(n,o){if(typeof n=="string")return n.endsWith("[]")||j(n)!==-1?n:o.some(i=>kc(i,n))?n+"@innerHTML":n;if(Array.isArray(n)){let[i,s]=n;if(typeof i!="string"||!i)return n;let l=yc(t,i);return l.length?[i,r(s,l)]:n}if(n&&typeof n=="object"){let i=Object.create(null);for(let[s,l]of Object.entries(n))i[s]=r(l,o);return i}return n}}function kc(e,t){let r=null;try{r=e.querySelector(t)}catch{return!1}return r?r.hasAttribute(ne)?r.getAttribute(ne)==="rich":r.children.length>0:!1}function yc(e,t){try{return[...e.querySelectorAll(t)]}catch{return[]}}function Ge(e){if(!e||e.tagName!=="TEXTAREA")return;let t=e.ownerDocument.defaultView||(typeof window<"u"?window:null);t&&t.CSS&&t.CSS.supports&&t.CSS.supports("field-sizing: content")||(e.style.height="auto",e.style.height=e.scrollHeight+"px")}function ye(e,t,r=!0){if(!e||!e.querySelectorAll||(e.querySelectorAll("textarea[data-hcms-field]").forEach(Ge),r===!1))return;let n=t&&t.defaultView||(typeof window<"u"?window:null),o=Ke(n);o&&e.querySelectorAll("[contenteditable][data-hcms-field]").forEach(i=>{if(i.__hcmsRichclay)return;let s;try{s=new o(i,{inline:!0,hyperclay:!1,toolbar:["bold","italic","link","undo","redo"]})}catch(a){console.warn("[hypercms] richclay activation failed; field stays plain contenteditable",a);return}i.__hcmsRichclay=s;let l=s&&s.squire;l&&typeof l.addEventListener=="function"&&l.addEventListener("input",()=>{let a=n&&n.Event||Event;i.dispatchEvent(new a("input",{bubbles:!0}))})})}var rs="link[data-hcms-fonts]",vc="link[data-hcms-fonts-preconnect]";function xc(e,t,r){let n=[],o=!1;return i(e,t,[]),{stacks:n,found:o};function i(s,l,a){if(typeof s=="string"){if(s.endsWith("[]")||ge(s,r,a,e)!=="@font")return;o=!0,l!=null&&l!==""&&n.push(String(l));return}if(Array.isArray(s)){let u=Array.isArray(l)?l:[];u.forEach(d=>i(s[1],d,[...a,"*"])),u.length||i(s[1],void 0,[...a,"*"]);return}if(s&&typeof s=="object")for(let[u,d]of Object.entries(s))i(d,l?.[u],[...a,u])}}function wc(e,t){let r=e?.head;if(!r)return;let n=r.querySelector(rs),o=r.querySelector(vc);if(!t){n?.remove(),o?.remove();return}o||(o=e.createElement("link"),o.setAttribute("rel","preconnect"),o.setAttribute("href","https://fonts.gstatic.com"),o.setAttribute("crossorigin",""),o.setAttribute("data-hcms-fonts-preconnect",""),o.setAttribute("no-undo",""),r.appendChild(o)),n||(n=e.createElement("link"),n.setAttribute("rel","stylesheet"),n.setAttribute("data-hcms-fonts",""),n.setAttribute("no-undo",""),o.after(n)),n.hasAttribute("no-undo")||n.setAttribute("no-undo",""),o.hasAttribute("no-undo")||o.setAttribute("no-undo",""),n.getAttribute("href")!==t&&n.setAttribute("href",t)}function Ye(e,t){if(!e?.doc||e.pageRules==null)return;let{stacks:r,found:n}=xc(e.pageRules,t,e.doc);if(!n)return;let o=e.doc.head?.querySelector(rs)?.getAttribute("href")||null,i=Qt(r,o);i&&o&&vi(o,i)||wc(e.doc,i)}var bn=new WeakSet;function Le(e,t){let r=D("undo");if(!r)return t();r.pause();try{let n=t();return n&&n.ok?r.commitCaptured(e):r.discardCaptured(),n}finally{r.resume()}}function ue(e){let t=D("undo");if(!t)return e();t.pause();try{return e()}finally{t.discardCaptured(),t.resume()}}function lr(e){let{formRoot:t}=e;if(!t||bn.has(t))return;bn.add(t);let r=s=>{let l=s.target;!l||!l.closest||l.closest("[data-hcms-form-root]")&&(l.matches("[data-hcms-color-input]")||l.matches("input, textarea, select, [contenteditable][data-hcms-field]")&&(l.tagName==="TEXTAREA"&&Ge(l),!l.matches('input[type="file"]')&&(!l.closest("[data-hcms-field]")&&!l.hasAttribute?.("data-hcms-field")||xt(l,e))))},n=s=>{let l=s.target;if(!(!l||!l.closest)&&l.closest("[data-hcms-form-root]")){if(l.matches("[data-hcms-color-input]")){let a=ki(l);a&&xt(a,e);return}if(l.matches('input[type="file"][data-hcms-upload]')){Rc(l,e);return}l.matches('input[type="checkbox"], input[type="radio"], select')&&xt(l,e)}},o=s=>{let l=s.target;if(!l||!l.closest)return;let a=l.closest("[data-hcms-action]");if(!a)return;let u=a.getAttribute("data-hcms-action");if(u==="add"||u==="remove"||u==="move-up"||u==="move-down"||u==="clear-upload"||u==="color-pick"||u==="color-clear"||u==="color-panel"||u==="font-pick"||u==="font-panel"){if(!a.closest("[data-hcms-form-root]"))return}else if(u==="close"&&!a.closest("[data-hcms-shell]"))return;if(u==="add"){let d=a.closest("[data-hcms-path]");if(!d)return;let f=d.getAttribute("data-hcms-path");wt(f,e)}else if(u==="remove"){let d=a.closest("[data-hcms-card], [data-hcms-array-item]");if(!d)return;vn(d,e)}else if(u==="move-up"||u==="move-down"){let d=a.closest("[data-hcms-card], [data-hcms-array-item]");if(!d)return;yn(d,u==="move-up"?-1:1,e)}else if(u==="clear-upload")jc(a,e);else if(u==="color-pick"||u==="color-clear"){let d=u==="color-pick"?pi(a):gi(a);d&&xt(d,e)}else if(u==="color-panel")bi(a);else if(u==="font-pick"){let d=Ci(a);d&&xt(d,e)}else u==="font-panel"?_i(a):u==="close"&&e.onCloseRequested?.()},i=t.ownerDocument;i.addEventListener("input",r,!0),i.addEventListener("change",n,!0),i.addEventListener("click",o,!0),e.detachEvents=()=>{i.removeEventListener("input",r,!0),i.removeEventListener("change",n,!0),i.removeEventListener("click",o,!0),bn.delete(t)}}var Ac=new Set(["value","checked"]);function Sc(e,t){if(!t)return null;let r=se(t);if(r.some(a=>typeof a=="number"||a==="*"))return null;let n=le(e.pageRules,r);if(typeof n!="string")return null;let o=U.ruleAttrIndex(n);if(o===-1)return null;let i=n.slice(o+1);if(!Ac.has(i))return null;let s=n.slice(0,o),l=s?e.pageRoot.querySelector(s):te(e.pageRoot);return l?{el:l,prop:i,oldValue:l[i]}:null}function xt(e,t){let n=(e.closest("[data-hcms-field]")||e).closest("[data-hcms-path]")?.getAttribute("data-hcms-path")||"",o=Sc(t,n);if(Y(V(t),{path:n,structural:!1},t),o){let i=D("undo");i&&typeof i.recordValue=="function"&&i.recordValue(o.el,{prop:o.prop,oldValue:o.oldValue,newValue:o.el[o.prop]})}}var Ec={type:"image/webp",quality:.85,maxWidth:2048,maxHeight:2048};async function _c(e,t){let r=t&&t.getAttribute?t.getAttribute("data-hcms-crop"):null;if(r==null)return{file:e};let n=D("quickcrop");if(typeof n!="function")return{file:e};try{let o=typeof window<"u"&&(window.clay?.modal??window.themodal)||"auto",i=await n(e,{aspect:Cc(r),modal:o,...Ec});return i===null?null:{file:Tc(i.blob,e.name),dataURL:i.dataURL}}catch(o){return Je(t,o&&o.message||"Crop failed"),null}}function Cc(e){let t=String(e??"").trim().toLowerCase();if(t===""||t==="free")return null;let r=t.match(/^(\d+(?:\.\d+)?)\s*[:/]\s*(\d+(?:\.\d+)?)$/);if(!r)return null;let n=parseFloat(r[1]),o=parseFloat(r[2]);return!n||!o?null:n/o}function Tc(e,t){let r=e.type==="image/webp"?".webp":e.type==="image/jpeg"?".jpg":".png",n=String(t||"image").replace(/\.[^.]+$/,"");try{return new File([e],n+r,{type:e.type})}catch{return e}}var Oc=new Set(["unsupported","payment-required"]);async function Rc(e,t){let r=e.files&&e.files[0];if(!r)return;let n=e.closest("[data-hcms-path]");if(!n)return;let o=n.getAttribute("data-hcms-path")||"";Je(n,null);let i=await _c(r,n);if(!i||t.closed){ve(e);return}let s=i.file,l=i.dataURL||null,a=D("upload");if(typeof a!="function")return kn(e,n,t,o,await ns(s,l,n),s);ls(n,l),os(n,0);let u=Nc(t),d;try{d=await a(s,{signal:u?.signal,onProgress:({percent:h})=>os(n,h)})}finally{Mc(t,u),Pc(n)}if(t.closed){ve(e);return}if(d.code==="aborted"){ve(e);return}if(d.ok)return kn(e,n,t,o,d.uploads[0].url,s);if(!Oc.has(d.code)){Je(n,d.msg||"Upload failed"),t.dispatch?.("hcms:error",{error:new Error(d.msg||"Upload failed"),code:d.code,path:o}),ve(e);return}let f=d.code==="payment-required"?"This file is stored in the page. Add a paid plan to upload files.":null;return kn(e,n,t,o,await ns(s,l,n),s,f)}function kn(e,t,r,n,o,i,s=null){if(r.closed){ve(e);return}if(ls(t,null),Je(t,null),!o){ve(e);return}ss(t,o,i.name),Y(V(r),{path:n,structural:!1},r),s&&Je(t,s,"info"),ve(e)}async function ns(e,t,r){return t||await Lc(e,r)}function Lc(e,t){let r=t?.ownerDocument?.defaultView?.FileReader||globalThis.FileReader;return r?new Promise(n=>{let o=new r;o.onload=()=>n(typeof o.result=="string"?o.result:""),o.onerror=()=>n("");try{o.readAsDataURL(e)}catch{n("")}}):Promise.resolve("")}function Nc(e){if(typeof AbortController!="function")return null;let t=new AbortController;return(e.uploads||(e.uploads=new Set)).add(t),t}function Mc(e,t){t&&e.uploads?.delete(t)}function jc(e,t){let r=e.closest("[data-hcms-path]");if(!r)return;let n=r.getAttribute("data-hcms-path")||"";ss(r,"","");let o=r.querySelector('input[type="file"][data-hcms-upload]');o&&ve(o),Je(r,null),Y(V(t),{path:n,structural:!1},t)}function qc(e){return e.querySelector?e.querySelector("img[data-hcms-field], a[data-hcms-field]"):null}function ss(e,t,r){let n=qc(e);if(!n)return;let o=(n.tagName||"").toUpperCase();o==="IMG"?n.src=t||"":o==="A"&&(n.href=t||"",n.textContent=t?r||fn(t):"")}function ve(e){try{e.value=""}catch{}}function ls(e,t){let r=e.querySelector?e.querySelector(".mirk-image__frame"):null;r&&(t?r.style.backgroundImage=`url("${t.replace(/"/g,"%22")}")`:r.style.removeProperty("background-image"))}function os(e,t){let r=Math.max(0,Math.min(100,Number(t)||0));e.setAttribute("data-hcms-uploading",""),e.style?.setProperty?.("--hcms-upload-progress",`${r}%`)}function Pc(e){e.removeAttribute("data-hcms-uploading"),e.style?.removeProperty?.("--hcms-upload-progress")}function Je(e,t,r="error"){let n=e.querySelector?e.querySelector(":scope > .hcms-error"):null;n&&(n.classList.toggle("hcms-error--info",!!t&&r==="info"),t?(n.textContent=t,n.hidden=!1):(n.textContent="",n.hidden=!0))}function wt(e,t){let{formRoot:r,pageRules:n}=t,o=r.querySelector(`[data-hcms-path="${Ne(e)}"]`);if(!o)throw new Error(`hypercms: no element at path "${e}"`);let i=o.querySelector(".hcms-array-items");if(!i)throw new Error(`hypercms: array container missing .hcms-array-items at "${e}"`);let s=se(e),l=zc(n,s),a=Array.isArray(l),u=typeof l=="string"&&l.endsWith("[]");if(!a&&!u)throw new Error(`hypercms: path "${e}" is not an array`);let d=sr(o,"data-hcms-max-items"),f=i.querySelectorAll(":scope > [data-hcms-card], :scope > [data-hcms-array-item]");if(o.hasAttribute("data-hcms-no-add")||d!=null&&f.length>=d)return;let h=f.length,m=a?l[1]:l.replace(/\[\]$/,""),c=bt(a?m:"string"),p=Ni({shape:a?"object-array-item":"scalar-array-item",itemShape:m,pathArr:[...s,h],data:c,doc:t.doc,itemKey:o.getAttribute("data-hcms-item-tpl")||null,pageRules:n});return i.appendChild(p),ye(p,t.doc,t.view?.enhanceFormRichText!==!1),wn(o),Le(`Add ${e}`,()=>Y(V(t),{path:e,structural:!0},t))}function yn(e,t,r){let n=e.closest('[data-hcms-shape="object-array"], [data-hcms-shape="scalar-array"]');if(!n||n.hasAttribute("data-hcms-no-reorder"))return;let o=n.querySelector(".hcms-array-items");if(!o)return;let i=Array.from(o.querySelectorAll(":scope > [data-hcms-card], :scope > [data-hcms-array-item]")),s=i.indexOf(e);if(s<0)return;let l=s+t;if(l<0||l>=i.length)return;let a=e.querySelector(`[data-hcms-action="${t<0?"move-up":"move-down"}"]`);return t<0?o.insertBefore(e,i[l]):o.insertBefore(e,i[l].nextSibling),An(o),wn(n),a&&typeof a.focus=="function"&&e.querySelector(`[data-hcms-action="${t<0?"move-up":"move-down"}"]`)?.focus?.(),Le(`Reorder ${n.getAttribute("data-hcms-path")||""}`,()=>Y(V(r),{path:n.getAttribute("data-hcms-path")||"",structural:!0},r))}var ir="Delete this item?";function Ic(e,t){let r=e&&e.getAttribute("data-hcms-confirm-remove");if(r!=null)return/^(off|false|no|0)$/i.test(r.trim())?null:r||ir;let n=t&&t.confirmRemove;return n===!1?null:typeof n=="string"?n||ir:n===!0||e&&e.getAttribute("data-hcms-shape")==="object-array"?ir:null}function vn(e,t){let r=e.closest('[data-hcms-shape="object-array"], [data-hcms-shape="scalar-array"]'),n=Ic(r,t);if(n==null)return Xe(e,t);let o=D("consent")||typeof window<"u"&&window.consent;typeof o=="function"?Promise.resolve(o(n)).then(()=>{t.closed||Xe(e,t)},()=>{}):typeof window<"u"&&typeof window.confirm=="function"?window.confirm(n)&&Xe(e,t):Xe(e,t)}function Xe(e,t){let r=e.getAttribute("data-hcms-path")||"",n=e.parentElement,o=e.closest('[data-hcms-shape="object-array"], [data-hcms-shape="scalar-array"]');if(!o?.hasAttribute("data-hcms-no-remove")){if(o){let i=sr(o,"data-hcms-min-items"),s=o.querySelector(".hcms-array-items"),l=s?s.querySelectorAll(":scope > [data-hcms-card], :scope > [data-hcms-array-item]").length:0;if(i!=null&&l<=i)return}return e.remove(),n&&An(n),o&&wn(o),Le(`Remove ${r}`,()=>Y(V(t),{path:r,structural:!0},t))}}function Y(e,t,r){if(r.closed)return{ok:!1,skipped:!0,closed:!0};let n=At(e);if(!t.structural&&n===r.lastFingerprint)return{ok:!0,skipped:!0};let o=Object.hasOwn(r,"writeRules")?r.writeRules:r.pageRules,i=Bo(r.pageRoot,o,e,{shellRoot:r.shellRoot,structural:!!t.structural,structuralPath:t.path||null,formRoot:r.formRoot});return i.ok?(r.lastFingerprint=n,r.lastData=e,Ye(r,e),is(r,null),r.dispatch?.("hcms:change",{data:e,path:t.path,structural:!!t.structural}),r.onChange?.(e,t)):(is(r,Dc(i.error,t.path)),r.dispatch?.("hcms:error",{error:i.error,attemptedData:e}),r.onError?.(i.error)),i}function ar(e,t){let r=Ne(t),n=`[data-hcms-path="${r}"] input[data-hcms-field], [data-hcms-path="${r}"] textarea[data-hcms-field], [data-hcms-path="${r}"] select[data-hcms-field], [data-hcms-path="${r}"] img[data-hcms-field], [data-hcms-path="${r}"] a[data-hcms-field], [data-hcms-path="${r}"] [contenteditable][data-hcms-field], input[data-hcms-path="${r}"][data-hcms-field], textarea[data-hcms-path="${r}"][data-hcms-field], select[data-hcms-path="${r}"][data-hcms-field], img[data-hcms-path="${r}"][data-hcms-field], a[data-hcms-path="${r}"][data-hcms-field], [contenteditable][data-hcms-path="${r}"][data-hcms-field]`;return e.querySelector(n)}function cr(e,t,r,n){let o=(e.tagName||"").toUpperCase(),i=(e.getAttribute("type")||"").toLowerCase();if(o==="INPUT"&&i==="checkbox"){e.checked=t===!0||t==="true";return}if(o==="INPUT"&&i==="radio"){let s=Ne(n),l=r.querySelectorAll(`[data-hcms-path="${s}"][data-hcms-field][type="radio"], [data-hcms-path="${s}"] [data-hcms-field][type="radio"]`);l.length?l.forEach(a=>{a.checked=String(a.value)===String(t??"")}):e.checked=String(e.value)===String(t??"");return}if(o==="IMG"){e.src=t==null?"":String(t);return}if(o==="A"){e.href=t==null?"":String(t);return}if(e.hasAttribute&&e.hasAttribute("contenteditable")){e.innerHTML=t==null?"":String(t);return}if("value"in e){e.value=t==null?"":String(t);return}e.textContent=t==null?"":String(t)}function V(e){let t=U.extract(e.formRoot,e.formRules,{exclude:null});return ce(t,e.formRules)}function ce(e,t){if(t==null||e==null)return e;if(typeof t=="string")return t.endsWith("@checked")?e===!0||e==="true":e;if(Array.isArray(t)){if(!Array.isArray(e))return e;let[,r]=t;return e.map(n=>ce(n,r))}if(typeof t=="object"){if(typeof e!="object"||Array.isArray(e))return e;let r={};for(let[n,o]of Object.entries(t))r[n]=ce(e[n],o);return r}return e}function is(e,t){e.lastErrors=t&&t.length?t:null,xn(e)}function xn(e){if($c(e),e.errorEl&&(e.errorEl.textContent="",e.errorEl.hidden=!0),!e.lastErrors)return;let t=[];for(let{message:r,path:n}of e.lastErrors){if(n!=null&&n!==""){let o=Fc(e.formRoot,n);if(o){o.textContent=o.textContent?`${o.textContent}
${r}`:r,o.hidden=!1;continue}}t.push(r)}t.length&&e.errorEl&&(e.errorEl.textContent=t.join(`
`),e.errorEl.hidden=!1)}function $c(e){if(e.formRoot)for(let t of e.formRoot.querySelectorAll(".hcms-error"))t.textContent="",t.hidden=!0}function Fc(e,t){if(!e)return null;let r=t.split(".");for(;r.length>0;){let n=r.join("."),o=typeof CSS<"u"&&CSS.escape?CSS.escape(n):n.replace(/[^a-zA-Z0-9_\-.*]/g,s=>"\\"+s),i=e.querySelector(`[data-hcms-path="${o}"]`);if(i){for(let s of i.children)if(s.classList&&s.classList.contains("hcms-error"))return s}r.pop()}return null}function Dc(e,t){return e?e.name==="EmptyListInsert"?[{message:"Add a seed item in HTML first.",path:t}]:e.name==="ShapeMismatch"&&Array.isArray(e.mismatches)&&e.mismatches.length?e.mismatches.map(r=>({message:`Shape mismatch: expected ${r.expected}, got ${r.got}`,path:r.path})):[{message:e.message||String(e),path:t}]:[{message:"unknown error",path:t}]}function zc(e,t){let r=e;for(let n of t){if(r==null||typeof r=="string")return;if(Array.isArray(r)){if(typeof n!="number"&&n!=="*")return;r=r[1];continue}if(typeof r=="object"){if(typeof n=="number"||!(n in r))return;r=r[n];continue}return}return r}function sr(e,t){if(!e||!e.hasAttribute(t))return null;let r=parseInt(e.getAttribute(t),10);return Number.isFinite(r)?r:null}function wn(e){if(!e)return;let t=e.querySelector(".hcms-array-items");if(!t)return;let r=Array.from(t.querySelectorAll(":scope > [data-hcms-card], :scope > [data-hcms-array-item]")),n=r.length,o=sr(e,"data-hcms-max-items"),i=sr(e,"data-hcms-min-items"),s=e.hasAttribute("data-hcms-no-add"),l=e.hasAttribute("data-hcms-no-remove"),a=e.hasAttribute("data-hcms-no-reorder"),u=e.querySelector(':scope > .hcms-add, :scope > * > .hcms-add, :scope > [data-hcms-action="add"]');u&&(u.hidden=s||o!=null&&n>=o),r.forEach((d,f)=>{let h=d.querySelector('[data-hcms-action="remove"]');h&&(h.hidden=l||i!=null&&n<=i);let m=d.querySelector('[data-hcms-action="move-up"]');m&&(m.hidden=a||f===0);let c=d.querySelector('[data-hcms-action="move-down"]');c&&(c.hidden=a||f===n-1)})}function ur(e){!e||!e.querySelectorAll||e.querySelectorAll(".hcms-array-items").forEach(t=>An(t))}function An(e){let t=e.querySelectorAll?Array.from(e.querySelectorAll('input[type="radio"][data-hcms-field]'),n=>[n,n.checked]):[],r=0;for(let n of e.children){if(!n.matches?.("[data-hcms-card], [data-hcms-array-item]"))continue;let o=n.getAttribute("data-hcms-path");if(!o)continue;let i=o.split(".");i[i.length-1]=String(r);let s=i.join(".");s!==o&&Uc(n,o,s),r++}for(let[n,o]of t)n.checked!==o&&(n.checked=o)}function Uc(e,t,r){let n=e.querySelectorAll("[data-hcms-path]");e.setAttribute("data-hcms-path",r);for(let o of n){let i=o.getAttribute("data-hcms-path");i===t?o.setAttribute("data-hcms-path",r):i&&i.startsWith(t+".")&&o.setAttribute("data-hcms-path",r+i.slice(t.length))}Bc(e)}function Bc(e){for(let t of e.querySelectorAll('input[type="radio"][data-hcms-field]')){if(!t.name||!t.name.startsWith("hcms-"))continue;let r=t.closest("[data-hcms-path]");r&&(t.name=tr(r.getAttribute("data-hcms-path")))}}function At(e){return JSON.stringify(e,(t,r)=>{if(r&&typeof r=="object"&&!Array.isArray(r)){let n=Object.create(null);for(let o of Object.keys(r).sort())n[o]=r[o];return n}return r})}function Ne(e){return typeof CSS<"u"&&CSS.escape?CSS.escape(e):String(e).replace(/[^a-zA-Z0-9_\-.*]/g,t=>"\\"+t)}var dr="hcms-shell-styles",Hc="hcms-bundled-styles-installed",hr="hcms-session-open",Vc='a[href], area[href], button:not([disabled]), input:not([disabled]):not([type="hidden"]), select:not([disabled]), textarea:not([disabled]), [tabindex]:not([tabindex="-1"])',Ze=new WeakSet,Sn="";function cs(e){Sn=e}function St(e){e?.body?.classList.add(hr)}function En(e){e?.body?.classList.remove(hr)}var Wc=0;function as(e){return String(e).replace(/[&<>"]/g,t=>({"&":"&amp;","<":"&lt;",">":"&gt;",'"':"&quot;"})[t])}function us({mountTo:e,side:t="right",overlay:r=!1,showSaveButton:n=!1,title:o="Page content",eyebrow:i="Edit",theme:s=null,doc:l}){ds(l);let a=`hcms-shell-title-${++Wc}`,u=l.createElement("div");u.setAttribute("data-hcms-shell",""),u.setAttribute("editor-ui",""),u.setAttribute("save-remove",""),u.setAttribute("save-ignore",""),u.setAttribute("tabindex","-1"),u.setAttribute("role","dialog"),u.setAttribute("aria-modal","true"),u.setAttribute("aria-labelledby",a);let d=s==="dark"?" dark":s==="light"?" light":"";u.className="hcms-shell pixel-quiet hcms-panel hcms-side-"+t+(r?" hcms-overlay":"")+d;let f=as(o),h=as(i);u.innerHTML=`
    <div class="hcms-shell-minibar" aria-hidden="true">
      <span class="hcms-shell-minibar-title">${f}</span>
      <button type="button" class="hcms-shell-close mirk-button mirk-button--small" data-hcms-action="close" aria-label="Close">
        <span class="mirk-button__label">\xD7</span>
      </button>
    </div>
    <div class="hcms-shell-body">
      <header class="hcms-shell-header">
        <div class="hcms-shell-heading">
          <div class="hcms-shell-eyebrow">${h}</div>
          <h2 class="hcms-shell-title" id="${a}">${f}</h2>
        </div>
        <button type="button" class="hcms-shell-close mirk-button mirk-button--small" data-hcms-action="close" aria-label="Close">
          <span class="mirk-button__label">\xD7</span>
        </button>
      </header>
      <div class="hcms-shell-notice" role="status" hidden></div>
      <div class="hcms-shell-error" role="alert" hidden></div>
      <div data-hcms-form-root class="hcms-form"></div>
      <footer class="hcms-shell-footer"${n?"":" hidden"}>
        <button type="button" class="hcms-shell-save mirk-button" trigger-save>
          <span class="mirk-button__label">Save</span>
        </button>
      </footer>
    </div>
  `,(e||l.body).appendChild(u);let c=l.body;c.classList.add("hcms-open"),St(l),r&&c.classList.add("hcms-overlay"),t==="left"&&c.classList.add("hcms-side-left");let p=Gc(u,l),b=Kc(u);return{root:u,formRoot:u.querySelector("[data-hcms-form-root]"),noticeEl:u.querySelector(".hcms-shell-notice"),errorEl:u.querySelector(".hcms-shell-error"),saveButton:u.querySelector(".hcms-shell-save"),destroy(){p.detach(),b.detach(),u.remove(),c.classList.remove("hcms-open","hcms-overlay","hcms-side-left"),En(l)},restoreChrome(){Qe(l),c.classList.add("hcms-open"),St(l),r&&c.classList.add("hcms-overlay"),t==="left"&&c.classList.add("hcms-side-left")}}}function Qe(e){e&&(e.getElementById(dr)||e.querySelector("style[data-hcms-bundled-styles]")||(Ze.delete(e),ds(e)))}function ds(e){if(e&&!Ze.has(e)){if(e[Hc]){Ze.add(e);return}if(e.getElementById(dr)||e.querySelector("style[data-hcms-bundled-styles]")){Ze.add(e);return}if(Sn){let t=e.createElement("style");t.id=dr,t.setAttribute("save-remove",""),t.setAttribute("save-ignore",""),t.textContent=Sn,(e.head||e.documentElement).appendChild(t),Ze.add(e);return}try{let t=new URL("./theme.generated.css",import.meta.url).href,r=e.createElement("link");r.rel="stylesheet",r.id=dr,r.setAttribute("save-remove",""),r.setAttribute("save-ignore",""),r.href=t,(e.head||e.documentElement).appendChild(r),Ze.add(e)}catch{let r=()=>{link.isConnected&&console.warn("hypercms: shell stylesheet not applied \u2014 cssText is empty and the co-located theme fallback is unavailable. Call installStyles(themeText) before opening the CMS.")};e.defaultView?.queueMicrotask?e.defaultView.queueMicrotask(r):r()}}}function Kc(e){let t=e.querySelector(".hcms-shell-body"),r=e.querySelector(".hcms-shell-header");if(!t||!r||typeof t.addEventListener!="function")return{detach(){}};let n=()=>{let o=(r.offsetHeight||0)-12;e.classList.toggle("is-condensed",t.scrollTop>o)};return t.addEventListener("scroll",n,{passive:!0}),n(),{detach(){t.removeEventListener("scroll",n)}}}function Gc(e,t){function r(n){if(n.key!=="Tab"||!e.contains(t.activeElement))return;let o=Array.from(e.querySelectorAll(Vc));if(o.length===0)return;let i=o[0],s=o[o.length-1];n.shiftKey&&t.activeElement===i?(n.preventDefault(),s.focus()):!n.shiftKey&&t.activeElement===s&&(n.preventDefault(),i.focus())}return t.addEventListener("keydown",r),{detach:()=>t.removeEventListener("keydown",r)}}var Yc="[hypercms]",hs={skip:"[data-hcms-shell]",templateAttr:"cms-template"},ms={skip:"[data-hcms-shell]",templateAttr:null},_n=class extends Error{constructor(t,r,n){super(`hypercms: rule at "${t}" has an invalid CSS selector: "${r}"`),this.name="InvalidRuleSelector",this.path=t,this.selector=r,this.cause=n}};function fr(e,t){let r=[],n=[],o=[];return Tn(ie(e),e,t,[],r,n,o),{missing:fs(r),twins:eu(n),readOnly:fs(o)}}function pr(e){return Cn(e)}function Cn(e){if(typeof e=="string")return ps(e)?void 0:e;if(Array.isArray(e)){let[t,r]=e;return[t,Cn(r)]}if(e&&typeof e=="object"){let t={};for(let[r,n]of Object.entries(e)){let o=Cn(n);o!==void 0&&(t[r]=o)}return t}return e}function Tn(e,t,r,n,o,i,s){if(typeof r=="string"){let l=Xc(r),a=l?mr(e,t,l,hs,n):[];if(ps(r)){s.push(et(n));return}if(!l)return;if(r.endsWith("[]")){a.length===0&&mr(e,t,l,ms,n).length===0&&o.push(et(n));return}a.length===0?o.push(et(n)):a.length>1&&i.push({path:et(n),count:a.length});return}if(Array.isArray(r)){let[l,a]=r;if(typeof l!="string"||!l)return;let u=mr(e,t,l,hs,n);if(u.length===0){mr(e,t,l,ms,n).length===0&&o.push(et(n));return}for(let d of u)Tn(e,d,a,[...n,"*"],o,i,s);return}if(r&&typeof r=="object")for(let[l,a]of Object.entries(r))Tn(e,t,a,[...n,l],o,i,s)}function mr(e,t,r,n,o){try{return e.find(t,r,n)}catch(i){throw new _n(et(o),r,i)}}function Xc(e){if(e==="."||e.startsWith("@"))return null;if(e.endsWith("[]"))return e.slice(0,-2)||null;let t=j(e);return(t===-1?e:e.slice(0,t))||null}function Jc(e){if(e.endsWith("[]"))return null;let t=j(e);return t===-1?null:e.slice(t+1)||null}function ps(e){let t=Jc(e);return t!=null&&Ae.has(t)}function tt(e){Zc(e),Qc(e)}function Zc(e){let t=e.noticeEl;if(!t)return;let r=e.unresolved&&e.unresolved.missing||[],n=e.unresolved&&e.unresolved.readOnly||[];if(r.length===0&&n.length===0){t.textContent="",t.hidden=!0;return}let o=[];if(r.length){let i=r.length===1?"1 field no longer matches this page":`${r.length} fields no longer match this page`;o.push(`${i}: ${r.join(", ")}`)}if(n.length){let i=n.length===1?"1 field reads a property the browser will not let anything write":`${n.length} fields read properties the browser will not let anything write`;o.push(`${i}: ${n.join(", ")}`)}t.textContent=o.join(`
`),t.hidden=!1}function Qc(e){let t=e.unresolved&&e.unresolved.twins||[],r=t.map(n=>`${n.path}:${n.count}`).join("|");if(r!==e.lastTwinSignature){e.lastTwinSignature=r;for(let{path:n,count:o}of t)console.warn(`${Yc} "${n}" matches ${o} elements; edits go to the first one.`)}}function fs(e){return[...new Set(e)]}function eu(e){let t=new Map;for(let r of e){let n=t.get(r.path);(!n||r.count>n.count)&&t.set(r.path,r)}return[...t.values()]}function et(e){return e.length?e.join("."):"(whole page)"}var tu={skip:"[data-hcms-shell]",templateAttr:"cms-template"};function xe(e,{ignoreActiveValue:t}={}){return ru(e,{ignoreActiveValue:t})}function ru(e,{ignoreActiveValue:t}){let r=U.findRules(e.doc,e.rulesSource||"cms");r&&(e.pageRules=e.view.prepareRules(Dt(r.rules,e.pageRoot)),e.rulesTagNode=r.tagNode),Bt(e.doc),Kt(e.doc,e.pageRules),e.formRules=Xt(e.pageRules,e.doc),e.writeRules=pr(e.pageRules),e.unresolved=fr(e.pageRoot,e.pageRules);let n=Ft(),o=ce(U.extract(e.pageRoot,e.pageRules,{...tu,...n.hooks}),e.pageRules),i=He({pageRules:e.pageRules,formRules:e.formRules,data:o,doc:e.doc});$t(e.formRoot,i,{ignoreActiveValue:t}),Ye(e,o),n.seed(e.formRoot),ye(e.formRoot,e.doc,e.view?.enhanceFormRichText!==!1),xn(e),tt(e),e.updateFingerprint&&e.updateFingerprint()}function gs({debounce:e=100,onRefresh:t}){let r=D("Mutation");if(!r||typeof r.onAnyChange!="function")throw new Error("hypercms: a mutation hub is required (clay.Mutation or hyperclay.Mutation). Load clayjs or hyperclayjs, or just the mutation utility, before initializing hypercms.");let n=r.onAnyChange({debounce:e},o=>{t(o)});return{unsubscribe:typeof n=="function"?n:()=>{}}}var nu='input:not([disabled]):not([type="hidden"]), textarea:not([disabled]), select:not([disabled]), button:not([disabled]), [tabindex]:not([tabindex="-1"])';function bs({doc:e,pageRoot:t,opts:r={}}){let n=r.richText!==!1,o=null;return{name:"sidebar",richText:n,enhanceFormRichText:!0,ctx:null,root:null,formRoot:null,errorEl:null,noticeEl:null,prepareRules(i){return n?es(i,t):i},mount(i){let s=this.ctx;o=ue(()=>us({mountTo:r.mountTo||e.body,side:r.side||"right",overlay:!!r.overlay,showSaveButton:!!r.showSaveButton,title:r.title,eyebrow:r.eyebrow,theme:r.theme,doc:e})),this.root=o.root,this.formRoot=o.formRoot,this.errorEl=o.errorEl,this.noticeEl=o.noticeEl;let l=He({pageRules:s.pageRules,formRules:s.formRules,data:i,doc:e});o.formRoot.appendChild(l),s.seeder.seed(o.formRoot),ye(o.formRoot,e),tt(s),lr(s)},refresh(i){if(i==="livesync"){o?.restoreChrome?.(),xe(this.ctx,{ignoreActiveValue:!0});return}if(i==="undo"){xe(this.ctx,{ignoreActiveValue:!1});return}xe(this.ctx)},focusOnOpen(){let i=this.root&&this.root.querySelector(nu);i&&typeof i.focus=="function"&&i.focus()},destroy(){o?.destroy(),o=null}}}var ou="[cms-template], [data-hcms-shell]";function rt(e){if(!e||typeof e.getBoundingClientRect!="function"||typeof e.closest=="function"&&e.closest(ou))return!1;let t=e.getBoundingClientRect();return t.width>=8&&t.height>=8}var iu={skip:"[data-hcms-shell]",templateAttr:"cms-template"},su={skip:"[data-hcms-shell]",templateAttr:null},lu="text",au="native",ks="handle",cu=new Set(["INPUT","TEXTAREA","SELECT"]),xs=new Set(["IMG","INPUT","TEXTAREA","SELECT","OPTION","BR","HR","VIDEO","AUDIO","IFRAME","EMBED","OBJECT","CANVAS","SOURCE","TRACK","AREA","COL","PARAM","BUTTON","STYLE","SCRIPT","LINK","META"]);function Rn(e,t,r=null){let n=[],o=[],i=ie(e);return On(i,e,t,[],n,o,r?(l,a)=>kt(l,r,a,t):()=>null),{targets:n,lists:o}}function On(e,t,r,n,o,i,s){if(typeof r=="string"){if(r.endsWith("[]")){let f=r.slice(0,-2);if(!f)return;let h=gr(e,t,f);h.forEach((m,c)=>{o.push(vs([...n,c],m,r,null))}),i.push(ys(e,t,n,f,h,!0));return}let l=j(r),a=l===-1?null:r.slice(l+1),u=l===-1?r:r.slice(0,l),d=!u||u==="."?t:gr(e,t,u)[0];d&&o.push(vs(n,d,r,a,s(r,n)));return}if(Array.isArray(r)){let[l,a]=r;if(typeof l!="string"||!l)return;let u=gr(e,t,l);u.forEach((d,f)=>On(e,d,a,[...n,f],o,i,s)),i.push(ys(e,t,n,l,u,typeof a=="string"));return}if(r&&typeof r=="object")for(let[l,a]of Object.entries(r))On(e,t,a,[...n,l],o,i,s)}function ys(e,t,r,n,o,i){let s=o[0]?o[0].parentElement:null;if(!s){let l=gr(e,t,n,su)[0];s=l?l.parentElement:null}return{path:r,items:o,container:s,scalar:i}}function vs(e,t,r,n,o=null){return{path:e,el:t,rule:r,attr:n,kind:uu(t,n),icon:hu(t,n),style:o}}function uu(e,t){let r=(e.tagName||"").toUpperCase();return!t||t==="innerHTML"?xs.has(r)?ks:lu:t==="value"&&cu.has(r)&&!e.readOnly&&!du(e)?au:ks}function du(e){return e.disabled?!0:typeof e.matches=="function"&&e.matches(":disabled")}function hu(e,t){let r=(e.tagName||"").toUpperCase();return(!t||t==="innerHTML")&&!xs.has(r)?null:r==="IMG"||t==="srcset"?"camera":r==="A"&&t==="href"?"paperclip":"pencil"}function gr(e,t,r,n=iu){try{return e.find(t,r,n)}catch{return[]}}function ws(e,t){let r=[],n=new Map,o=null;for(let i of e){if(!i.style)continue;let s=i.path.join(".");if(i.style==="page"){o||(o={scope:"page",el:i.el,list:null,paths:[]},r.push(o)),o.paths.push(s);continue}let l=mu(i.path,t),a=l?l.row:i.el,u=n.get(a);u||(u={scope:l?"row":"section",el:a,list:l?l.list:null,paths:[]},n.set(a,u),r.push(u)),u.paths.push(s)}return r}function mu(e,t){for(let r=e.length-1;r>=0;r--){if(typeof e[r]!="number")continue;let n=e.slice(0,r).join("."),o=t.find(s=>s.path.join(".")===n),i=o&&o.items[e[r]];if(i)return{list:o,row:i}}return null}var br=Object.freeze({data:Object.freeze({tokens:["no-data"],bundles:["editor-ui"]}),save:Object.freeze({tokens:["no-save"],bundles:["editor-ui"]}),snapshot:Object.freeze({tokens:["no-snapshot"],bundles:["editor-ui"]}),watch:Object.freeze({tokens:["no-watch"],bundles:["editor-ui"]}),undo:Object.freeze({tokens:["no-undo"],bundles:["editor-ui"]}),history:Object.freeze({tokens:[],bundles:["editor-ui"]})}),fu=Object.freeze({"editor-ui":Object.freeze(["no-data","no-save","no-snapshot","no-watch","no-undo"])}),xp=Object.freeze(["no-save","no-snapshot","no-trigger-autosave","no-dirty","no-watch","no-undo","no-data","freeze","editor-ui","live"]);function Et(e,t){if(!e||e.nodeType!==1)return!1;let r=e.getAttribute?.("clay");return!!(r&&r.split(/\s+/).includes(t)||e.hasAttribute?.(t))}function Ln(e,t){if(Et(e,t))return!0;for(let[r,n]of Object.entries(fu))if(n.includes(t)&&Et(e,r))return!0;return!1}function _t(e){let t=br[e];if(!t)throw new Error(`Unknown region capability: ${e}`);return[...t.tokens,...t.bundles].flatMap(r=>[`[clay~="${r}"]`,`[${r}]`]).join(", ")}function pu(e,t){let r=e&&e.nodeType===1?e:e?.parentElement;for(;r&&r.nodeType===1;){let n=br[t];if(!n)throw new Error(`Unknown region capability: ${t}`);if(n.tokens.some(o=>Ln(r,o))||n.bundles.some(o=>Et(r,o)))return r;r=r.parentElement}return null}function As(e,t){return!!pu(e,t)}var gu=/:(?:focus(?:-within|-visible)?|hover|active|visited|defined)\b/i;function bu(e,t){if(!/^(INPUT|TEXTAREA|SELECT|OPTION)$/.test(e.tagName||""))return;let r=(e.getAttribute?.("type")||"").toLowerCase();"value"in e&&"value"in t&&e.tagName!=="OPTION"&&r!=="checkbox"&&r!=="radio"&&r!=="file"&&(t.value=e.value),e.tagName==="INPUT"&&(r==="checkbox"||r==="radio")&&(t.checked=e.checked),e.tagName==="OPTION"&&(t.selected=e.selected),"indeterminate"in e&&"indeterminate"in t&&(t.indeterminate=e.indeterminate)}function Ss(e,t,r,n){if(n)return!!e.closest?.(t);let o=e;for(;o?.nodeType===1;){if(o.matches(t))return!0;if(o===r)break;o=o.parentElement}return!1}function Es(e,t,r,n,o,i){return(i?As(e,t):Ss(e,r,o,!1))||!!(n&&(i&&e!==o?e.matches(n):Ss(e,n,o,i)))}function ku(e,t){let r=br[t];return r.tokens.some(n=>Ln(e,n))||r.bundles.some(n=>Et(e,n))}var yu=new Set(["INPUT","TEXTAREA","SELECT","OPTION"]);function _s(e,t,r,n){if(e.nodeType===1&&n(e)){r.dropped.push(t);return}r.cloneToLive.set(t,e),r.liveToClone.set(e,t);let o=e,i=t;if(e.nodeType===1){if(yu.has(e.tagName)&&r.controls.push(e,t),e.tagName==="TEMPLATE"){for(;t.firstChild;)t.removeChild(t.firstChild);o=e.content,i=t.content,r.cloneToLive.set(i,o),r.liveToClone.set(o,i)}}else if(e.nodeType!==11)return;let s=i.firstChild;for(let l=o.firstChild;l&&s;l=l.nextSibling)_s(l,s,r,n),s=s.nextSibling}function Nn(e,{capability:t="data",exclude:r=null,inherit:n=!0,provenance:o=null}={}){if(!e)throw new TypeError("createContentView requires a DOM context");let i=e.nodeType===9?e:e.ownerDocument;if(!i?.implementation?.createHTMLDocument)throw new TypeError("createContentView requires an HTML DOM implementation");let s=i.implementation.createHTMLDocument(""),l=o||new WeakMap,a=new WeakMap,u=e.nodeType===9?e.documentElement:e;r&&s.documentElement.matches(r);let d=_t(t),f=n?c=>ku(c,t)||!!(r&&c.matches(r)):c=>Es(c,t,d,r,u,!1),h=null;if(u.nodeType!==1||!Es(u,t,d,r,u,n)){h=s.importNode(u,!0);let c={cloneToLive:l,liveToClone:a,dropped:[],controls:[]};_s(u,h,c,f);for(let p of c.dropped)p.remove();for(let p=0;p<c.controls.length;p+=2)bu(c.controls[p],c.controls[p+1])}e.nodeType===9&&h&&(s.replaceChild(h,s.documentElement),a.set(e,s),l.set(s,e));let m=c=>{if(gu.test(c))throw new Error(`Filtered content queries do not support stateful selector: ${c}`)};return{root:h,document:s,capability:t,selector:_t(t),cloneToLive:l,liveToClone:a,original(c){return l.get(c)||null},cloneOf(c){return a.get(c)||null},query(c,p=h){return m(c),Array.from(p.querySelectorAll(c),b=>l.get(b)||b)},text(c=h){return(c===h?h:a.get(c))?.textContent||""},html(c=h){return(c===h?h:a.get(c))?.innerHTML??""},clone(c=h){let p=c===h?h:a.get(c);return p?s.importNode(p,!0):null}}}import{morph as Tu}from"./hyper-morph.vendor.js";var vu=[16,8,0],z=8,xu=4;function Cs({anchor:e,bar:t,rail:r=t,viewport:n,current:o=null}){let i=h=>o===h?xu:0;if(e.bottom<=0||e.top>=n.height||e.right<=0||e.left>=n.width)return{mode:"hidden",x:0,y:0};let s=h=>Math.max(z,Math.min(e.left,n.width-h-z)),l=e.top-16-t.height;if(l>=z-i("above"))return{mode:"above",x:s(t.width),y:l};let a=e.bottom+16;if(a+t.height<=n.height-z+i("below"))return{mode:"below",x:s(t.width),y:a};let u=n.width-e.right-z,d=e.left-z,f=u>=d?[["rail-right",u],["rail-left",d]]:[["rail-left",d],["rail-right",u]];for(let[h,m]of f)for(let c of vu){if(r.width+c>m+i(h))continue;let p=h==="rail-right"?Math.min(e.right+c,n.width-r.width-z):Math.max(e.left-c-r.width,z),b=Math.min(e.bottom,n.height-z)-r.height,g=Math.max(z,Math.min(Math.max(e.top,z),b));return{mode:h,x:p,y:g,gap:c}}return{mode:"pinned",x:s(t.width),y:z}}function Mn({anchor:e,handle:t,viewport:r,inset:n=6,prefer:o="auto"}){let i=e.height>=t.height*1.5&&e.width>=t.width*2;if(o==="corner"||i){let h=Math.max(z,Math.min(e.right-t.width+n,r.width-t.width-z)),m=Math.max(z,Math.min(e.top-n,r.height-t.height-z));return{x:h,y:m,mode:"over"}}let s=4,l=e.right+s+t.width<=r.width-z,a=l?e.right+s:e.left-s-t.width,u=Math.max(z,a),d=e.top+e.height/2-t.height/2,f=Math.max(z,Math.min(d,r.height-t.height-z));return{x:u,y:f,mode:l?"beside-right":"beside-left"}}var yr=!1;function Ts(){if(yr)return;let e=D("onPrepareForSave");typeof e=="function"&&(e(t=>{Rs(t),Os(t)}),yr=!0)}function Os(e){let t=e&&e.querySelector&&e.querySelector("body");t&&t.classList.remove("hcms-open","hcms-overlay","hcms-side-left",hr)}function Rs(e){if(!(!e||typeof e.querySelectorAll!="function"))for(let t of e.querySelectorAll(`[${ke}]`)){let r=Xi(t);r!==null&&(t.innerHTML=r),t.removeAttribute(ke)}}var vr=!1;function xr(){if(vr)return;let e=D("onSnapshot");typeof e=="function"&&(e(t=>{Rs(t),Zi(t,typeof window<"u"?window:null),Os(t)}),vr=!0)}function Ls(){Ts(),xr(),(!yr||!vr)&&wu()}var kr=null;function wu(){if(kr||typeof document>"u")return;kr=We(document,Ki,()=>{Ts(),xr(),yr&&vr&&(kr?.(),kr=null)})}var Au={edit:"M8 10h2v6H8zm2 4h4v2h-4zm0-6h2v2h-2zm2-2h2v2h-2zm2-2h2v2h-2zm2-2h2v2h-2zm2 2h2v2h-2zm2 2h2v2h-2zm-2 2h2v2h-2zm-2 2h2v2h-2zm-2 2h2v2h-2zm-4 0h2v2h-2z","move-up":"M11 4h2v2h2v2h2v2h2v2h-4v-2h-2v10h-2V10H9v2H5v-2h2V8h2V6h2z","move-down":"M11 4h2v10h2v-2h4v2h-2v2h-2v2h-2v2h-2v-2H9v-2H7v-2H5v-2h4v2h2z",remove:"M5 5h3v3h3v3h2V8h3V5h3v3h-3v3h-3v2h3v3h3v3h-3v-3h-3v-3h-2v3H8v3H5v-3h3v-3h3v-2H8V8H5z",add:"M10 4h4v6h6v4h-6v6h-4v-6H4v-4h6z"};function Me(e){let t=e==="edit"?"6 0 18 18":"0 0 24 24",r=e==="settings"?'<circle cx="6" cy="12" r="1.5"/><circle cx="12" cy="12" r="1.5"/><circle cx="18" cy="12" r="1.5"/>':`<path d="${Au[e]}"/>`;return`<svg class="hcms-inline-icon" xmlns="http://www.w3.org/2000/svg" viewBox="${t}" width="24" height="24" fill="currentColor" aria-hidden="true" focusable="false">${r}</svg>`}var Ns="data-hcms-ghost",Su=_t("history");function Eu(e){let t=e.container,r=null;for(;t&&(t.namespaceURI!=="http://www.w3.org/1999/xhtml"||["SELECT","OPTGROUP","DATALIST"].includes(t.tagName));)r=t,t=t.parentElement;return{parent:t,anchor:r}}function Ms({doc:e,themeRoot:t,onAdd:r,onResize:n}){let o=e.defaultView,i=[],s=!1,l=null,a=new Map;function u(m,c,p){m.style[c]!==p&&(m.style[c]=p)}function d(){let m=new Set(i.map(c=>c.node.parentElement));for(let[c,p]of a)m.has(c)&&o?.Sortable?.get(c)===p.instance||(p.instance.option("draggable")===p.selector&&p.instance.option("draggable",p.original),a.delete(c));for(let c of m){let p=o?.Sortable?.get(c);if(!p)continue;let b=p.option("draggable");if(a.get(c)?.selector===b)continue;let g=b,v=`${g.trim().startsWith(">")?"> ":""}:is(${g.trim().replace(/^>\s*/,"")}):not(${Su})`;p.option("draggable",v),a.set(c,{instance:p,original:g,selector:v})}}function f(){for(let m of i){let{node:c,slot:p,list:b}=m,g=b.items.filter(k=>k.isConnected).map(k=>k.getBoundingClientRect()).filter(k=>k.width>0&&k.height>0);g.length&&(m.width=g.reduce((k,v)=>k+v.width,0)/g.length,m.height=g.reduce((k,v)=>k+v.height,0)/g.length),u(c,"width",m.width?`${Math.round(m.width)}px`:"100%"),u(p,"height",`${Math.max(48,Math.round(m.height||64))}px`),p!==c&&(p.colSpan=Math.max(1,...b.items.map(k=>[...k.children].reduce((v,A)=>v+(A.colSpan||1),0)))),c.hidden!==s&&(c.hidden=s)}d()}function h(m,c){let p=c.tagName,b=["TBODY","THEAD","TFOOT","TABLE"].includes(p),g=e.createElement(b?"tr":["UL","OL","MENU"].includes(p)?"li":"div");for(let S of[Ns,"data-hcms-shell","editor-ui","no-watch","no-save","save-remove","snapshot-remove"])g.setAttribute(S,"");g.setAttribute("draggable","false"),g.setAttribute("contenteditable","false"),g.className="hcms-shell pixel-quiet hcms-inline-ghost";for(let S of["light","dark"])g.classList.toggle(S,!!t?.classList.contains(S));let k=b?e.createElement("td"):g;k!==g&&(k.className="hcms-inline-ghost-cell",g.appendChild(k));let v=e.createElement("button");v.type="button",v.className="hcms-inline-list-button hcms-inline-list-add mirk-button mirk-button--small",v.setAttribute("data-hcms-list-action","add"),v.innerHTML=`<span class="mirk-button__label">${Me("add")}<span>Add</span></span>`,k.appendChild(v);let A={node:g,slot:k,button:v,list:m,width:0,height:0};v.addEventListener("click",S=>{S.preventDefault(),S.stopPropagation(),r(A.list)});for(let S of["pointerdown","mousedown","touchstart","click"])g.addEventListener(S,E=>E.stopPropagation());return g.addEventListener("dragstart",S=>{S.preventDefault(),S.stopPropagation()}),A}return{setLists(m){let c=new Set(i),p=[],b=new Set(i.map(k=>k.node)),g=new Set;for(let k of m||[]){if(!k.container||!k.container.isConnected)continue;let{parent:v,anchor:A}=Eu(k);if(!v)continue;if(!g.has(v)){for(let O of v.querySelectorAll(`:scope > [${Ns}]`))b.has(O)||O.remove();g.add(v)}let S=i.find(O=>c.has(O)&&O.list.container===k.container&&O.list.path.join(".")===k.path.join("."))||h(k,v);c.delete(S),S.list=k,S.button.setAttribute("data-hcms-list",k.path.join(".")),S.button.setAttribute("aria-label",`Add to ${k.path.join(".")||"the list"}`);let E=A||k.items.filter(O=>O.parentElement===v).at(-1);E?E.nextSibling!==S.node&&E.after(S.node):S.node.parentElement!==v&&v.appendChild(S.node),p.push(S)}for(let k of c)k.node.remove();i=p,l?.disconnect(),!l&&typeof o?.ResizeObserver=="function"&&(l=new o.ResizeObserver(()=>{f(),n?.()}));for(let k of i)for(let v of k.list.items)l?.observe(v);f()},update:f,setHidden(m){s=!!m,f()},destroy(){l?.disconnect();for(let m of i)m.node.remove();i=[],d()}}}var js=[["move-up","Move up"],["move-down","Move down"]],_u={prefer:"corner",inset:0};function Cu(e,t){return t.width<=e.width&&t.height<=e.height}function qs({doc:e,layerEl:t,onActivate:r,onListAction:n,onStyle:o}){let i=e.defaultView,s=[],l=new Map,a=0,u=new Map,d=null,f=new Set,h=0,m=!1,c=null,p=null,b=null,g=null,k=new Map,v=!1,A=null,S=Ms({doc:e,themeRoot:t.closest("[data-hcms-shell]"),onResize:O,onAdd:y=>n?.({action:"add",list:y,index:y.items.length})}),E=e.createElement("div");E.className="hcms-inline-highlight",E.hidden=!0,E.setAttribute("aria-hidden","true"),t.appendChild(E);function O(){if(!(h||!i)){if(typeof i.requestAnimationFrame!="function")return de();h=i.requestAnimationFrame(()=>{h=0,de()})}}let B=()=>b||g;function de(){if(!i)return;S.update();let y={width:i.innerWidth,height:i.innerHeight},x=B();for(let w of s){if(!w.visible){w.node.hidden=!0;continue}for(let T of w.members)T.node.hidden=v&&T.kind!=="handle";w.node.hidden=!1;let C=w.el.getBoundingClientRect(),_=w.node.getBoundingClientRect(),L=_.width,N=w.kind==="row"&&!Cu(C,_);if(N&&w.el!==x){for(let T of w.members)T.kind==="row"&&(T.node.hidden=!0);_=w.node.getBoundingClientRect()}if(w.members.every(T=>T.node.hidden)){w.node.hidden=!0;continue}if(w.kind==="style"){let T=Math.max(8,Math.min(C.left,y.width-_.width-8)),P=Math.max(8,Math.min(C.top,y.height-_.height-8));w.node.style.transform=`translate(${Math.round(T)}px, ${Math.round(P)}px)`;continue}let W=w.kind==="handle"?null:_u,{x:M,y:Q}=Mn({anchor:C,handle:_,viewport:y,...W}),je=w.members.find(T=>T.kind==="handle");if(N&&je){let T=je.node.getBoundingClientRect(),P=Mn({anchor:C,handle:T,viewport:y}),q=w.members.slice(w.members.indexOf(je)+1).filter(Cr=>!Cr.node.hidden).reduce((Cr,bl)=>Cr+bl.node.getBoundingClientRect().width+2,0);M=Math.max(8+L,Math.min(y.width-8,P.x+T.width+q))-_.width,Q=P.y}w.node.style.transform=`translate(${Math.round(M)}px, ${Math.round(Q)}px)`}if(A)if(!A.node.isConnected||A.node.closest("[hidden]"))X();else{let w=A.button.getBoundingClientRect(),C=A.menu.getBoundingClientRect();A.menu.style.left=`${Math.max(8,w.right-C.width)-w.left}px`,A.menu.style.right="auto",A.menu.style.top=w.bottom+6+C.height>y.height-8?"auto":"calc(100% + 6px)",A.menu.style.bottom=w.bottom+6+C.height>y.height-8?"calc(100% + 6px)":"auto"}it(),c?.()}function G(y){if(y?.closest?.("[data-hcms-ghost]"))return null;for(let x=y;x&&x.nodeType===1;x=x.parentElement){let w=k.get(x);if(w)return w}return null}function we(y){A&&!A.node.contains(y.target)&&X();let x=B();g=G(y.target),g&&t.contains(y.target)&&(b=null),B()!==x&&O()}function ot(y){if(y.relatedTarget||!g)return;let x=B();g=null,B()!==x&&O()}function it(){if(!p||E.hidden)return;let y=p.getBoundingClientRect();E.style.width=`${Math.round(y.width)}px`,E.style.height=`${Math.round(y.height)}px`,E.style.transform=`translate(${Math.round(y.left)}px, ${Math.round(y.top)}px)`}function ol(){if(!i||typeof i.IntersectionObserver!="function"){for(let x of s)x.visible=!0;return}d||(d=new i.IntersectionObserver(x=>{let w=!1;for(let C of x)for(let _ of l.get(C.target)||[])_.visible!==C.isIntersecting&&(_.visible=C.isIntersecting,w=!0);w&&O()},{threshold:0}));let y=new Set(l.keys());for(let x of f)y.has(x)||d.unobserve(x);for(let x of y)f.has(x)||d.observe(x);f=y}function il(){m||!i||(i.addEventListener("scroll",O,{passive:!0,capture:!0}),i.addEventListener("resize",O,{passive:!0}),e.addEventListener("focusin",we),e.addEventListener("focusout",ot),e.addEventListener("pointerdown",Zn,!0),e.addEventListener("keydown",Qn,!0),m=!0)}function sl(){!m||!i||(i.removeEventListener("scroll",O,{capture:!0}),i.removeEventListener("resize",O),e.removeEventListener("focusin",we),e.removeEventListener("focusout",ot),e.removeEventListener("pointerdown",Zn,!0),e.removeEventListener("keydown",Qn,!0),m=!1)}function Kn(y){return`${y.kind}\0${y.attr||""}`}function Gn(y,x){return y.container===x.container&&y.scalar===x.scalar}function ll(y,x){return y.kind!==x.kind?!1:y.kind==="style"?!0:y.kind==="handle"?Kn(y.target)===Kn(x.target):y.kind==="row"?y.row===x.row&&Gn(y.list,x.list):Gn(y.list,x.list)}function Yn(y,x){y.target=x;let w=x.path.join(".");y.node.setAttribute("data-hcms-target",w),x.icon?y.node.setAttribute("data-hcms-icon",x.icon):y.node.removeAttribute("data-hcms-icon"),y.node.setAttribute("aria-label",`Edit ${w}`)}function al(y){let x=e.createElement("button");x.type="button",x.className="hcms-inline-handle mirk-button mirk-button--small",x.innerHTML=`<span class="mirk-button__label">${Me("edit")}</span>`;let w={node:x,kind:"handle",target:y};return Yn(w,y),x.addEventListener("click",C=>{C.preventDefault(),C.stopPropagation(),r?.(w.target,x)}),w}function cl(y){let x=e.createElement("button");x.type="button",x.className="hcms-inline-style-chip mirk-button mirk-button--small",x.setAttribute("aria-label","Edit style"),x.innerHTML='<span class="mirk-button__label">Style</span>';let w={node:x,kind:"style",group:y.group};return x.addEventListener("click",C=>{C.preventDefault(),C.stopPropagation(),o?.(w.group,x)}),w}function Xn(y,x){let w=e.createElement("button");return w.type="button",w.className="hcms-inline-list-button mirk-button mirk-button--small",w.setAttribute("data-hcms-list-action",y),w.setAttribute("aria-label",x),w.innerHTML=`<span class="mirk-button__label">${Me(y)}${y==="add"?"<span>Add</span>":""}</span>`,w}function Jn(y,{list:x,row:w,rowIndex:C,count:_}){let L=x.path.join(".");y.list=x,y.row=w,y.index=C,y.count=_,y.node.setAttribute("data-hcms-list",L),y.node.setAttribute("data-hcms-row",String(C));for(let N of y.node.querySelectorAll("[data-hcms-list-action]")){let W=N.getAttribute("data-hcms-list-action"),M=js.find(([Q])=>Q===W)?.[1]||W;N.setAttribute("aria-label",`${M} ${L}.${C}`),N.disabled=W==="move-up"&&C===0||W==="move-down"&&C===_-1}}function ul(y,x,w,C){let _=e.createElement("div");_.className="hcms-inline-row-controls";let L={node:_,kind:"row",list:y,row:x,index:w,count:C};for(let[N,W]of js){let M=Xn(N,W);M.addEventListener("click",Q=>{Q.preventDefault(),Q.stopPropagation(),!M.disabled&&n?.({action:N,list:L.list,index:L.index,row:L.row})}),_.appendChild(M)}return Jn(L,{list:y,row:x,rowIndex:w,count:C}),L}function X(y=!1){if(!A)return;let x=A;A=null,x.menu.hidden=!0,x.button.setAttribute("aria-expanded","false"),x.node.parentElement?.classList.remove("has-open-settings"),y&&x.button.isConnected&&x.button.focus({preventScroll:!0})}function Zn(y){A&&!A.node.contains(y.target)&&X()}function Qn(y){if(A){if(y.key==="Escape")y.preventDefault(),y.stopPropagation(),X(!0);else if(y.key==="Tab")X(!0);else if(["ArrowDown","ArrowUp","Home","End"].includes(y.key)&&A.menu.contains(y.target)){y.preventDefault();let x=[...A.menu.querySelectorAll('[role="menuitem"]')],w=x.indexOf(y.target),C=y.key==="Home"?0:y.key==="End"?x.length-1:w===-1?0:(w+(y.key==="ArrowDown"?1:-1)+x.length)%x.length;x[C]?.focus({preventScroll:!0})}}}function eo(y,{list:x,row:w,rowIndex:C,style:_=null}){Object.assign(y,{list:x,row:w,index:C,style:_}),y.node.setAttribute("data-hcms-list",x.path.join(".")),y.node.setAttribute("data-hcms-row",String(C)),y.button.setAttribute("aria-label",`Settings ${x.path.join(".")}.${C}`),_&&!y.styleItem.isConnected?y.menu.insertBefore(y.styleItem,y.menu.firstChild):_||y.styleItem.remove()}function dl(y){let x=e.createElement("div");x.className="hcms-inline-settings";let w=Xn("settings","Settings");w.setAttribute("aria-haspopup","menu"),w.setAttribute("aria-expanded","false");let C=e.createElement("div");C.className="hcms-inline-settings-menu",C.setAttribute("role","menu"),C.setAttribute("aria-label","Item settings"),C.hidden=!0;let _=e.createElement("button");_.type="button",_.setAttribute("role","menuitem"),_.setAttribute("data-hcms-style-action",""),_.textContent="Style";let L=e.createElement("button");L.type="button",L.setAttribute("role","menuitem"),L.setAttribute("data-hcms-list-action","remove"),L.textContent="Delete",C.appendChild(L),x.append(w,C);let N={node:x,button:w,menu:C,styleItem:_,kind:"settings"};eo(N,y);let W=()=>{X(),A=N,C.hidden=!1,w.setAttribute("aria-expanded","true"),x.parentElement.classList.add("has-open-settings"),C.querySelector('[role="menuitem"]').focus({preventScroll:!0}),O()};return w.addEventListener("click",M=>{M.preventDefault(),M.stopPropagation(),A===N?X(!0):W()}),w.addEventListener("keydown",M=>{(M.key==="ArrowDown"||M.key==="ArrowUp")&&(M.preventDefault(),W())}),L.addEventListener("click",M=>{M.preventDefault(),M.stopPropagation(),X(!0),n?.({action:"remove",list:N.list,index:N.index,row:N.row})}),_.addEventListener("click",M=>{M.preventDefault(),M.stopPropagation(),X(),o?.(N.style,N.button)}),N}function hl(y,x){y.kind==="handle"?Yn(y,x.target):y.kind==="row"?Jn(y,x):y.kind==="settings"?eo(y,x):y.kind==="style"&&(y.group=x.group)}function Rt(y,x,w,C,_){let L=x.get(w)?.find(N=>N.category===_);if(!L){L={el:w,category:_,members:[]},y.push(L);let N=x.get(w);N?N.push(L):x.set(w,[L])}L.members.push(C)}function ml(y,x,w,C){let _=w.items||[];_.forEach((L,N)=>{rt(L)&&(Rt(y,x,L,{kind:"row",list:w,row:L,rowIndex:N,count:_.length},"item"),Rt(y,x,L,{kind:"settings",list:w,row:L,rowIndex:N,style:C.get(L)||null},"item"))})}function fl(y,x){let w=new Set(y.members),C=[];for(let _ of x){let L=y.members.find(N=>w.has(N)&&ll(N,_))||(_.kind==="handle"?al(_.target):_.kind==="row"?ul(_.list,_.row,_.rowIndex,_.count):_.kind==="style"?cl(_):dl(_));w.delete(L),hl(L,_),v&&L.kind!=="handle"&&(L.node.hidden=!0),C.push(L)}for(let _ of w)_===A&&X(),_.node.remove();C.sort((_,L)=>["row","handle","settings"].indexOf(_.kind)-["row","handle","settings"].indexOf(L.kind)),C.forEach((_,L)=>{y.node.children[L]!==_.node&&y.node.insertBefore(_.node,y.node.children[L]||null)}),y.members=C}function pl(y,x,w){let C=[],_=new Map,L=new Map,N=0;for(let T of y||[])T.style||(L.has(T.el)||L.set(T.el,T),!(T.kind!=="handle"||!rt(T.el))&&(Rt(C,_,T.el,{kind:"handle",target:T},"item"),N++));let W=new Map;for(let T of w||[])T.scope==="row"?W.set(T.el,T):T.scope==="section"&&rt(T.el)&&Rt(C,_,T.el,{kind:"style",group:T},"style");for(let T of x||[])ml(C,_,T,W);let M=new Set(s),Q=new Map;for(let T of s)Q.has(T.el)||Q.set(T.el,T.visible);let je=[];for(let T of C){let P=s.find(q=>M.has(q)&&q.el===T.el&&q.category===T.category);if(!P){let q=e.createElement("div");q.className="hcms-inline-item-controls",q.setAttribute("role","group"),P={el:T.el,node:q,category:T.category,kind:T.category==="add"?"add":"handle",members:[],visible:Q.get(T.el)??!1},t.appendChild(q)}M.delete(P),P.el=T.el,P.category=T.category,fl(P,T.members),P.kind=P.members.some(q=>q.kind==="row")?"row":P.members.some(q=>q.kind==="handle")?"handle":P.members.some(q=>q.kind==="style")?"style":"add",P.node.setAttribute("aria-label",P.category==="add"?"List controls":P.category==="style"?"Style controls":"Item controls"),je.push(P)}for(let T of M)T.node.remove();s=je,l=new Map,k=new Map;for(let T of s){let P=l.get(T.el);P?P.push(T):l.set(T.el,[T]);for(let q of T.members)q.kind!=="row"&&q.kind!=="settings"||(k.set(q.row,q.row),k.set(q.node,q.row),k.set(T.node,q.row))}b&&!k.has(b)&&(b=null),g&&!k.has(g)&&(g=null),u=L,a=N}function gl(){X(),d?.disconnect(),d=null,f=new Set;for(let y of s)y.node.remove();s=[],l=new Map,u=new Map,k=new Map,a=0}return{setTargets(y,x,w=[]){pl(y,x,w),S.setLists(x),ol(),il(),O()},refresh:O,get count(){return a},get controlsHidden(){return v},setControlsHidden(y){if(v=!!y,S.setHidden(v),v){X();for(let x of s){for(let w of x.members)w.kind!=="handle"&&(w.node.hidden=!0);x.members.every(w=>w.node.hidden)&&(x.node.hidden=!0)}}O()},elementToTarget(y){if(y?.closest?.("[data-hcms-ghost]"))return null;for(let x=y;x&&x.nodeType===1;x=x.parentElement){let w=u.get(x);if(w)return w}return null},setHoveredRow(y){let x=B();b=y?G(y):null,B()!==x&&O()},showHighlight(y){y&&(p=y,E.hidden=!1,it())},hideHighlight(){p=null,E.hidden=!0},setFollower(y){c=typeof y=="function"?y:null},destroy(){S.destroy(),h&&i&&i.cancelAnimationFrame(h),h=0,c=null,p=null,b=null,g=null,E.remove(),sl(),gl()}}}var Ou="hypercms-inline",nt="is-hcms-inline-active",qn="is-hcms-inline-onpath",Ps='input:not([disabled]):not([type="hidden"]), textarea:not([disabled]), select:not([disabled]), button:not([disabled]), [tabindex]:not([tabindex="-1"])',Ru=["bold","italic","link","undo","redo"],Lu=/^H[1-6]$/,Nu=0;function Ds({doc:e,pageRoot:t,opts:r={}}){let n=r.richText!==!1,o=null,i=null,s=null,l=new Map,a=null,u=null,d=[],f=null;function h(m,c,p,b,g){let k=Ke(e.defaultView);if(typeof k!="function")return null;let v=g||{},A=v.originalHTML??Nn(c,{capability:"history"}).html(),S=v.richClayIsOurs??!c.hasAttribute("data-richclay"),E=v.adopted??c.getAttribute("data-richclay-active")==="true",O=Ct(c,b,"history"),B=Mu(m,k,c,b,S);if(!B)return null;Ji(c,b==="innerHTML",S);let de=String(++Nu);c.setAttribute(ke,de),xr();let G={el:c,editor:B,path:p,prop:b,boundId:de,originalHTML:A,oldValue:O,richClayIsOurs:S,adopted:E,dirty:!1,written:!1,restorable(){return!this.adopted&&!this.dirty&&!this.written},lastEdited:void 0};l.set(c,G),Gi(de,G);let we=B.squire;if(we&&typeof we.addEventListener=="function"){let it=()=>{G.dirty=!0,G.lastEdited=Ct(c,G.prop,"history"),ju(m,G)};we.addEventListener("input",it),G.detachInput=()=>we.removeEventListener?.("input",it)}let ot=()=>zs(G);return c.addEventListener("blur",ot),G.detachBlur=()=>c.removeEventListener("blur",ot),G}return{name:"inline",richText:n,ctx:null,root:null,formRoot:null,errorEl:null,noticeEl:null,handoffEl:null,handoffCountEl:null,popEl:null,enhanceFormRichText:!1,prepareRules(m){return n?ts(m,t):m},bindText(m,c,p,b){return h(this.ctx,m,c,p,b)},mount(m){let c=this.ctx;o=$u(e,r.theme),this.root=o.root,this.formRoot=o.formRoot,this.errorEl=o.errorEl,this.noticeEl=o.noticeEl,this.handoffEl=o.handoffEl,this.handoffCountEl=o.handoffCountEl,this.popEl=o.popEl,ue(()=>St(e));let p=He({pageRules:c.pageRules,formRules:c.formRules,data:m,doc:e});o.formRoot.appendChild(p),c.seeder.seed(o.formRoot),ye(o.formRoot,e,this.enhanceFormRichText),tt(c),lr(c),i=qs({doc:e,layerEl:o.layerEl,onActivate:(b,g)=>this.activate(b,g),onListAction:b=>this.listAction(b),onStyle:(b,g)=>this.openStyle(b,g)}),i.setFollower(()=>this.placePopover()),o.closeEl.addEventListener("click",()=>this.deactivate()),o.toggleEl.addEventListener("click",b=>{b.preventDefault(),b.stopPropagation(),this.toggleControls()}),o.pageStyleEl.addEventListener("click",b=>{b.preventDefault(),b.stopPropagation();let g=d.find(k=>k.scope==="page");g&&this.openStyle(g,o.pageStyleEl)}),o.handoffEl.querySelector("[data-hcms-open-view]").addEventListener("click",b=>{b.preventDefault(),b.stopPropagation(),c.onViewRequested?.("sidebar")}),this.bindPage(),this.syncTargets()},listAction({action:m,list:c,index:p,row:b}){let g=this.ctx,k=c.path.join("."),v=g.formRoot.querySelector(`[data-hcms-path="${Ne(k)}"]`);if(!v)return;if(m==="add"){wt(k,g);return}let A=b?Du(g,k,b):p;if(A===-1)return;let S=Fu(v,A);if(S){if(m==="remove"){vn(S,g),this.syncTargets();return}yn(S,m==="move-up"?-1:1,g),this.syncTargets()}},toggleControls(){if(!i||!o)return;let m=!i.controlsHidden;i.setControlsHidden(m),o.toggleEl.setAttribute("aria-pressed",String(m));let c=o.toggleEl.querySelector(".mirk-button__label");c&&(c.textContent=m?"Show controls":"Hide controls")},bindPage(){let m=te(this.ctx.pageRoot),c=o.root,p=S=>{let E=i&&i.elementToTarget(S.target);E?i.showHighlight(E.el):i?.hideHighlight(),i?.setHoveredRow(S.target)},b=()=>{i?.hideHighlight(),i?.setHoveredRow(null)},g=S=>{if(c.contains(S.target))return;let E=i&&i.elementToTarget(S.target);!E||E.kind!=="text"||l.has(E.el)||h(this.ctx,E.el,E.path.join("."),Ar(E))},k=S=>{if(c.contains(S.target))return;let E=i&&i.elementToTarget(S.target);if(!E){this.deactivate();return}(typeof S.target.closest=="function"&&S.target.closest("a[href]")||E.kind!=="text")&&S.preventDefault(),this.activate(E)},v=S=>{S.key==="Escape"&&this.deactivate()},A=S=>{let E=S.detail;if(!(!E||E.pageRoot!==te(this.ctx.pageRoot)||!E.path))for(let O of l.values())O.path===E.path&&(O.written=!0)};m.addEventListener("pointerover",p),m.addEventListener("pointerleave",b),m.addEventListener("pointerdown",g),m.addEventListener("click",k),c.addEventListener("keydown",v),e.addEventListener("hcms:change",A),s=()=>{m.removeEventListener("pointerover",p),m.removeEventListener("pointerleave",b),m.removeEventListener("pointerdown",g),m.removeEventListener("click",k),c.removeEventListener("keydown",v),e.removeEventListener("hcms:change",A)}},activate(m,c){if(!m||!o||m.kind==="text"&&this.activateText(m))return;let p=Is(this,o,m.path.join("."));if(!p){this.deactivate();return}a=m,u=c||null,f=null,o.popTitleEl.textContent="Edit content",o.popEl.hidden=!1,o.formRoot.querySelectorAll(`.${nt}`).forEach(jn),this.placePopover(),Fs(p)},openStyle(m,c){if(!m||!o)return;let p=$s(this,o,m.paths);if(!p){this.deactivate();return}a={kind:"style",scope:m.scope,el:m.scope==="page"?o.pageStyleEl:m.el,path:m.paths[0].split("."),paths:m.paths},u=c||null,f=null,o.popTitleEl.textContent=m.scope==="page"?"Page style":"Style",o.popEl.hidden=!1,o.formRoot.querySelectorAll(`.${nt}`).forEach(jn),this.placePopover(),Fs(p)},activateText(m){let c=m.el,p=l.get(c);if(p)return wr(p),!0;let b=h(this.ctx,c,m.path.join("."),Ar(m));return b?(wr(b),!0):!1},placePopover(){if(!o||!a||o.popEl.hidden)return;let m=e.defaultView;if(!m)return;let c={width:m.innerWidth,height:m.innerHeight},p=a.el.getBoundingClientRect(),b=o.popEl.getBoundingClientRect(),{mode:g,x:k,y:v}=Cs({anchor:p,bar:b,viewport:c,current:f});f=g,o.popEl.style.transform=`translate(${Math.round(k)}px, ${Math.round(v)}px)`},deactivate(){if(!o||!a&&o.popEl.hidden)return;o.popEl.hidden=!0,Pn(o.root);let m=u;if(a=null,u=null,f=null,m&&e.contains(m)&&typeof m.focus=="function")try{m.focus({preventScroll:!0})}catch{m.focus()}},syncTargets(){if(!i)return;let m=this.ctx,c=Rn(m.pageRoot,m.pageRules,e),p=c.lists,b=c.targets.filter(v=>!v.style);d=ws(c.targets,p),o.pageStyleEl.hidden=!d.some(v=>v.scope==="page"),i.setTargets(c.targets,p,d);let g=qu(this,l,b,e);if(a?.kind==="style"){let v=a.scope==="page"?d.find(A=>A.scope==="page"):d.find(A=>A.el===a.el);v?a={...a,paths:v.paths,path:v.paths[0].split(".")}:this.deactivate()}else if(a){let v=g.get(a.el)||[],A=v.find(S=>Pu(S,a))||v.find(S=>S.path.join(".")===a.path.join("."))||(v.length===1?v[0]:null);A?a=A:this.deactivate()}Iu(this.ctx,l,e);let k=b.reduce((v,A)=>v+(rt(A.el)?0:1),0);this.handoffEl&&(this.handoffCountEl.textContent=k===0?"":`${k} ${k===1?"field isn't":"fields aren't"} visible right now.`,this.handoffEl.hidden=k===0)},refresh(m,c){m==="livesync"?(Qe(e),St(e),xe(this.ctx,{ignoreActiveValue:!0})):m==="undo"?xe(this.ctx,{ignoreActiveValue:!1}):xe(this.ctx),this.syncTargets(),(m==="livesync"||m==="undo")&&this.rebindText(m),this.restoreActive()},rebindText(m){let c=m!=="undo";for(let[p,b]of[...l]){if(!e.contains(p)){Tt(this.ctx,b,{restore:!1,record:c}),l.delete(p);continue}if(p.hasAttribute(ne)){let v=Ct(p,b.prop);v!==b.lastEdited&&(b.oldValue=v,b.lastEdited=void 0);continue}let g=e.activeElement===p;Tt(this.ctx,b,{restore:!1,record:c}),l.delete(p);let k=h(this.ctx,p,b.path,b.prop,{richClayIsOurs:b.richClayIsOurs,adopted:b.adopted});k&&g&&wr(k)}},restoreActive(){if(!o||!a||o.popEl.hidden)return;if(!e.contains(a.el)){this.deactivate();return}if(!(a.kind==="style"?$s(this,o,a.paths):Is(this,o,a.path.join(".")))){this.deactivate();return}o.formRoot.querySelectorAll(`.${nt}`).forEach(jn),this.placePopover()},focusOnOpen(){if(this.root&&typeof this.root.focus=="function")try{this.root.focus({preventScroll:!0})}catch{this.root.focus()}},destroy(){u=null,this.deactivate();for(let m of l.values())Tt(this.ctx,m);l.clear(),s?.(),s=null,i?.destroy(),i=null,o?.destroy(),o=null,En(e),this.popEl=null,this.handoffEl=null,this.handoffCountEl=null}}}function Mu(e,t,r,n,o){return ue(()=>{let i=null;try{i=new t(r,{inline:!0,hyperclay:!1,toolbar:n==="innerHTML"?Ru:!1,...Lu.test(r.tagName)?{singleLine:!0}:null})}catch(s){return console.warn("[hypercms] richclay activation failed; the field falls back to the popover",s),null}if(i.unsupported||!i.active){if(o)try{i.destroy()}catch{}return null}return typeof i.reattach=="function"&&i.reattach(),i})}function ju(e,{path:t,el:r,prop:n}){let o=ar(e.formRoot,t);o&&(cr(o,Ct(r,n),e.formRoot,t),Y(V(e),{path:t,structural:!1},e))}function Ct(e,t,r="data"){let n=Nn(e,{capability:r});return t==="innerHTML"?n.html():n.text().trim()}function zs(e){let{el:t,prop:r,oldValue:n,lastEdited:o}=e,i=o;if(i===void 0||i===n)return;e.oldValue=i;let s=D("undo");if(!s||typeof s.recordValue!="function")return;let l={prop:r,oldValue:n,newValue:i,read:a=>Ct(a,r,"history"),write:(a,u)=>Us(a,r,u)};s.isPaused?queueMicrotask(()=>{t.isConnected&&s.recordValue(t,l)}):s.recordValue(t,l)}function Us(e,t,r){let n=e.cloneNode(!1);t==="innerHTML"?n.innerHTML=r:n.textContent=r,Tu(e,Array.from(n.childNodes),{morphStyle:"innerHTML",policy:"history",restoreFocus:!1,scripts:{handle:!1,merge:!1}})}function Tt(e,t,{restore:r=!0,record:n=!0}={}){n&&zs(t),t.detachInput?.(),t.detachBlur?.(),ue(()=>{if(!t.adopted){try{t.editor.destroy()}catch(o){console.warn("[hypercms] richclay teardown failed; editor state may reach the save",o)}t.richClayIsOurs&&t.el.removeAttribute("data-richclay")}t.el.removeAttribute(ne),t.el.removeAttribute(Re),t.el.removeAttribute(ke),Yi(t.boundId),r&&t.restorable()&&Us(t.el,"innerHTML",t.originalHTML)})}function wr(e){let{editor:t,el:r}=e;if(r.ownerDocument.activeElement!==r)try{typeof t.focus=="function"?t.focus():r.focus()}catch{}}function qu(e,t,r,n){let o=new Map;for(let i of r){let s=o.get(i.el);s?s.push(i):o.set(i.el,[i])}for(let[i,s]of[...t]){let l=o.get(i)||[],a=l.find(h=>Ar(h)===s.prop)||l[0];if(!a){Tt(e.ctx,s,{restore:!0}),t.delete(i);continue}s.path=a.path.join(".");let u=Ar(a);if(u===s.prop)continue;let d=n.activeElement===i;Tt(e.ctx,s,{restore:!1}),t.delete(i);let f=e.bindText(i,s.path,u,{richClayIsOurs:s.richClayIsOurs,adopted:s.adopted,originalHTML:s.originalHTML});f&&d&&wr(f)}return o}function Pu(e,t){return e.el===t.el&&e.kind===t.kind&&e.attr===t.attr}function Ar(e){return e.attr==="innerHTML"?"innerHTML":"textContent"}function Iu(e,t,r){for(let n of e.pageRoot.querySelectorAll(`[${ne}]`))t.has(n)||ue(()=>Qi(n,r.defaultView))}function Is(e,t,r){let n=e.formRoot&&e.formRoot.querySelector(`[data-hcms-path="${Ne(r)}"]`);if(Pn(t.root),!n)return null;let o=n.closest("[data-hcms-card]"),i=o?[...o.querySelectorAll('[data-hcms-shape="scalar"][data-hcms-path]')].filter(s=>s.closest("[data-hcms-card]")===o&&s.closest('[data-hcms-shape="object-array"], [data-hcms-shape="scalar-array"]')===o.parentElement.closest('[data-hcms-shape="object-array"]')):[n];i.includes(n)||i.push(n);for(let s of i)Bs(e,s);return n}function $s(e,t,r){Pn(t.root);let n=null;for(let o of r){let i=e.formRoot&&e.formRoot.querySelector(`[data-hcms-path="${Ne(o)}"]`);i&&(n=n||i,Bs(e,i))}return n}function Bs(e,t){t.classList.add(nt),t.removeAttribute("draggable");for(let r=t.parentElement;r&&(r.classList.add(qn),r.removeAttribute("draggable"),r!==e.formRoot);r=r.parentElement);}function Pn(e){if(e)for(let t of e.querySelectorAll(`.${nt}, .${qn}`))t.classList.remove(nt,qn)}function jn(e){e.tagName==="TEXTAREA"&&Ge(e),e.querySelectorAll?.("textarea").forEach(Ge)}function Fs(e){let t=e.matches?.(Ps)?e:e.querySelector?.(Ps);if(!(!t||typeof t.focus!="function"))try{t.focus({preventScroll:!0})}catch{t.focus()}}function $u(e,t){Qe(e);let r=e.createElement(Ou),n=t==="dark"?" dark":t==="light"?" light":"";return r.className="hcms-shell pixel-quiet hcms-inline"+n,r.setAttribute("data-hcms-shell",""),r.setAttribute("editor-ui",""),r.setAttribute("no-save",""),r.setAttribute("save-remove",""),r.setAttribute("snapshot-remove",""),r.setAttribute("no-watch",""),r.setAttribute("tabindex","-1"),r.innerHTML=`
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
          <span class="mirk-button__label">${Me("remove")}</span>
        </button>
      </div>
      <div data-hcms-form-root class="hcms-form"></div>
    </div>
  `,e.body.appendChild(r),{root:r,formRoot:r.querySelector("[data-hcms-form-root]"),noticeEl:r.querySelector(".hcms-inline-notice"),errorEl:r.querySelector(".hcms-inline-error"),handoffEl:r.querySelector(".hcms-inline-handoff"),handoffCountEl:r.querySelector(".hcms-inline-handoff-count"),layerEl:r.querySelector(".hcms-inline-layer"),popEl:r.querySelector(".hcms-inline-pop"),closeEl:r.querySelector(".hcms-inline-pop-close"),toggleEl:r.querySelector("[data-hcms-controls-toggle]"),pageStyleEl:r.querySelector("[data-hcms-page-style-open]"),popTitleEl:r.querySelector(".hcms-inline-pop-title"),destroy(){r.remove()}}}function Fu(e,t){let r=e.querySelector(".hcms-array-items");return r&&r.querySelectorAll(":scope > [data-hcms-card], :scope > [data-hcms-array-item]")[t]||null}function Du(e,t,r){let{lists:n}=Rn(e.pageRoot,e.pageRules),o=n.find(i=>i.path.join(".")===t);return o?o.items.indexOf(r):-1}var zu="[hypercms]";function Hs(e,t){if(!e||!e.querySelectorAll||!t)return;let r=Uu(t);e.querySelectorAll("template[data-hcms-tpl]").forEach(o=>{let i=o.getAttribute("data-hcms-tpl");i&&(i.startsWith("@")||r.has(i)||console.warn(`${zu} template "${i}" doesn't match any rule path; ignored`))})}function Uu(e){let t=new Set;return r([],e),t;function r(n,o){let i=n.join("."),s=n.map(a=>typeof a=="number"?"*":a).join(".");i&&t.add(i),s&&t.add(s);let l=be(o);if(l==="object")for(let[a,u]of Object.entries(o))r([...n,a],u);else if(l==="object-array"||l==="scalar-array"){let a=[...n,"*"],u=a.map(d=>typeof d=="number"?"*":d).join(".");if(t.add(u),l==="object-array"){let d=o[1];if(d&&typeof d=="object"&&!Array.isArray(d))for(let[f,h]of Object.entries(d))r([...a,f],h)}}}}var In={skip:"[data-hcms-shell]",templateAttr:"cms-template"},R={isOpen:!1,ctx:null,opts:null};function Ws({view:e,doc:t,pageRoot:r,opts:n={},onCloseRequested:o,onViewRequested:i}){let s=n.rules!==void 0?n.rules:"cms",l=U.findRules(t,s);if(!l){let b=typeof s=="string"?`data-rules-name~="${s}"`:"the provided rules object";throw new Error(`hypercms: no rules found for ${b}`)}let a=e.prepareRules(Dt(l.rules,r)),u=l.tagNode;Bt(t),Kt(t,a),Hs(t,a);let d=Xt(a,t),f=pr(a),h=fr(r,a),m=Ft(),c=ce(U.extract(r,a,{...In,...m.hooks}),a),p={doc:t,pageRoot:r,pageRules:a,writeRules:f,formRules:d,rulesTagNode:u,rulesSource:s,richText:e.richText,view:e,seeder:m,initialData:c,get formRoot(){return e.formRoot},get shellRoot(){return e.root},get errorEl(){return e.errorEl},get noticeEl(){return e.noticeEl},unresolved:h,lastTwinSignature:null,lastFingerprint:null,lastData:null,observerHandle:null,undoUnsub:null,livesyncUnsub:null,onChange:n.onChange,onError:n.onError,confirmRemove:n.confirmRemove,previouslyFocused:t.activeElement,quiesced:!1,dispatch(b,g){let k=t.defaultView&&t.defaultView.CustomEvent||(typeof CustomEvent<"u"?CustomEvent:null);if(!k)return;let v={...g||{},pageRoot:te(r),view:e.name},A=new k(b,{bubbles:!0,cancelable:b==="hcms:change",detail:v});(e.root&&e.root.isConnected!==!1?e.root:te(r)).dispatchEvent(A)},onCloseRequested:o,onViewRequested:i};return p.updateFingerprint=()=>{p.lastFingerprint=At(V(p))},e.ctx=p,p}function Ks(e){e.updateFingerprint(),Ye(e,ce(U.extract(e.pageRoot,e.pageRules,In),e.pageRules)),e.observerHandle=gs({onRefresh:n=>e.view.refresh("observer",n)});let t=D("undo");if(t&&typeof t.on=="function"){let n=()=>{if(R.ctx!==e)return;Vs(e,"undo");let o=ce(U.extract(e.pageRoot,e.pageRules,In),e.pageRules);At(o)!==At(e.lastData)&&(e.lastData=o,e.onChange?.(o,{path:"",structural:!1}))};t.on("undo",n),t.on("redo",n),e.undoUnsub=()=>{t.off("undo",n),t.off("redo",n)}}let r=()=>Vs(e,"livesync");e.livesyncUnsub=We(e.doc,Wi,r),Dn.ctx=e,Vu(e.doc)}function Vs(e,t){R.ctx===e&&e.view.refresh(t)}function Ot(e,{dispatch:t=!0,restoreFocus:r=!0,updateUrl:n=!0,reason:o="close"}={}){if(!e||e.closed)return;e.closed=!0;for(let s of e.uploads||[])try{s.abort()}catch{}e.uploads?.clear();let i=e.previouslyFocused;if(t&&e.dispatch("hcms:close",{reason:o}),n&&Wu(),e.observerHandle?.unsubscribe?.(),e.undoUnsub?.(),e.livesyncUnsub?.(),e.detachEvents?.(),ue(()=>e.view.destroy()),Bu(),r&&typeof i?.focus=="function")try{i.focus()}catch{}}function Bu(){R.isOpen=!1,R.ctx=null,R.opts=null,Dn.ctx=null}function Sr(e){let t=e||(typeof window<"u"?window:null),r=t&&t.clay?t.clay.modeLifecycle:null;return r&&typeof r.canWrite=="function"?r:null}function Er(e){let t=Sr(e);if(!t)return!0;try{return t.canWrite()!==!1}catch{return!1}}var Hu="[data-hcms-uploading]";function Fn(e){if(!e)return[];let t=[];return e.uploads&&e.uploads.size>0&&t.push("upload"),e.pageRoot&&typeof e.pageRoot.querySelector=="function"&&e.pageRoot.querySelector(Hu)&&t.push("preview"),t}function Gs(e){if(!e||e.closed)return{ok:!0};let t=Fn(e);return t.length?{ok:!1,code:"pending-input",pending:t}:(e.quiesced=!0,{ok:!0})}function Ys(e){!e||e.closed||(e.quiesced=!1)}var Dn={ctx:null};function Vu(e){let t=e.defaultView||(typeof globalThis<"u"?globalThis:null);if(!t)return;let r=function(){let o=Dn.ctx;if(o)return ur(o.formRoot),Le("Reorder",()=>Y(V(o),{path:"",structural:!0},o))};typeof t.hypercmsCommit!="function"&&(t.hypercmsCommit=r),typeof globalThis<"u"&&typeof globalThis.hypercmsCommit!="function"&&(globalThis.hypercmsCommit=r)}var $n="cms";function Xs(e){let t=typeof e=="string"?e:"",r=t.indexOf("?"),n=r===-1?t:t.slice(r+1);if(!n)return t;let o=new URLSearchParams(n);return o.get($n)!=="true"?t:(o.set($n,"false"),"?"+o.toString())}function Js(e){let t=typeof e=="string"?e:"",r=t.indexOf("?"),n=r===-1?t:t.slice(r+1);return n?new URLSearchParams(n).get($n)==="true":!1}function Wu(){if(typeof window>"u"||!window.location||!window.history||typeof window.history.replaceState!="function")return;let e=window.location.search,t=Xs(e);t!==e&&window.history.replaceState(window.history.state,"",t+window.location.hash)}function Zs(e){if(cs(e),typeof document<"u"){let t=document.getElementById("hcms-shell-styles");t?.tagName==="LINK"&&t.remove(),Qe(document)}}var Bn={sidebar:bs,inline:Ds};function Ku(e,t){let r=t!==void 0?t:"cms";if(typeof r!="string")return"sidebar";let n=U.findRules(e,r)?.tagNode?.getAttribute?.("data-cms-view");return!n||n==="sidebar"?"sidebar":Bn[n]?n:(console.warn(`hypercms: unknown data-cms-view "${n}" on the rules tag, opening the sidebar`),"sidebar")}function Qs(e={}){return Hn(e)}function Gu(e){let t=e&&e.defaultView;if(!Sr(t))return null;let r=t.clay&&t.clay.setMode;return typeof r!="function"?null:r.call(t.clay,"view")}function Hn(e={},t=!1){let r=R.isOpen?{...R.opts,...e}:e,n=r.pageRoot||(typeof document<"u"?document:null);if(!n)throw new Error("hypercms: no pageRoot available");let o=(n.nodeType===9?n:n.ownerDocument)||(typeof document<"u"?document:null);if(!o)throw new Error("hypercms: no document available");if(!t&&!Er(o.defaultView))return{ok:!1,code:"not-allowed"};let i=e.view||(R.isOpen?R.ctx.view.name:Ku(o,r.rules)),s=Bn[i];if(!s)throw new Error(`hypercms: unknown view "${i}" (expected ${Object.keys(Bn).join(" or ")})`);let l=R.isOpen?{...r,view:i}:e,a=null,u=null;if(R.isOpen){if(R.ctx.view.name===i)return{ok:!0,view:i};a=R.ctx.previouslyFocused,u=R.ctx.view.name,Ot(R.ctx,{restoreFocus:!1,updateUrl:!1,reason:"switch"})}Ls();let d=s({doc:o,pageRoot:n,opts:l}),f=Ws({view:d,doc:o,pageRoot:n,opts:l,onCloseRequested:()=>Gu(o)||el(),onViewRequested:h=>Hn({view:h},!0)});a&&(f.previouslyFocused=a);try{d.mount(f.initialData),Ks(f),d.focusOnOpen(),R.isOpen=!0,R.ctx=f,R.opts=l,f.dispatch("hcms:open",{pageRoot:n,previous:u})}catch(h){throw Ot(f,{dispatch:!1,restoreFocus:!!a,updateUrl:!1}),h}return{ok:!0,view:i}}function el(){if(!R.isOpen)return{ok:!1,code:"no-session"};let e=R.ctx.doc;if(Sr(e&&e.defaultView)){let t=Fn(R.ctx);if(t.length)return{ok:!1,code:"pending-input",pending:t};if(!Er(e&&e.defaultView))return{ok:!1,code:"busy"}}return Ot(R.ctx),{ok:!0}}function tl(){R.isOpen&&R.ctx.view.refresh("api")}function Yu(){return R.isOpen&&R.ctx?R.ctx.view.name:null}function rl({doc:e,pageRoot:t,opts:r={}}={}){let n=null,o=!1;return{ready:Promise.resolve(),enable(){if(o)return;let i={...r};t&&(i.pageRoot=t),i.pageRoot||(i.pageRoot=e&&e.body||e||null),Hn(i,!0),n=R.ctx},quiesce(){return o||!n||R.ctx!==n?{ok:!0}:Gs(n)},resume(){o||!n||R.ctx!==n||Ys(n)},dispose(){o||(o=!0,n&&R.ctx===n&&Ot(n),n=null)}}}function Xu(e={}){return{id:"hypercms",order:1e3,prepare(){return rl(e)}}}function zn(){if(!R.isOpen)throw new Error("hypercms: cms is not open");if(R.ctx.quiesced)throw new Error("hypercms: the CMS is not admitting new changes")}var Ju={getData(){return R.isOpen?V(R.ctx):null},setValue(e,t){zn();let r=R.ctx,n=se(e),o=le(r.pageRules,n);if(o===void 0)throw new Error(`hypercms: no rule at path "${e}"`);if(typeof o!="string"||o.endsWith("[]"))throw new Error(`hypercms: setValue requires a leaf scalar path; "${e}" is not a leaf`);let i=ar(r.formRoot,e);if(!i)throw new Error(`hypercms: no field element at path "${e}"`);cr(i,t,r.formRoot,e),Y(V(r),{path:e,structural:!1},r)},addItem(e){zn(),wt(e,R.ctx)},removeItem(e){zn();let t=R.ctx,r=se(e);if(typeof r[r.length-1]!="number")throw new Error(`hypercms: removeItem requires an item path; "${e}" is not an array index`);let o=le(t.pageRules,r.slice(0,-1));if(!(Array.isArray(o)||typeof o=="string"&&o.endsWith("[]")))throw new Error(`hypercms: removeItem requires an item path; parent of "${e}" is not an array`);let s=t.formRoot.querySelector(`[data-hcms-path="${rd(e)}"]`);if(!s)throw new Error(`hypercms: no element at path "${e}"`);Xe(s,t)},refresh:tl,_commit(){if(!R.isOpen||R.ctx.quiesced)return;let e=R.ctx;return ur(e.formRoot),Le("Update",()=>Y(V(e),{path:"",structural:!0},e))}},Zu=250,Qu=1e4;function ed(){typeof window>"u"||typeof document>"u"||Js(window.location?window.location.search:"")&&(R.isOpen||td(()=>{if(!R.isOpen&&Er())try{Qs()}catch(e){console.warn("hypercms: auto-open failed",e)}}))}function Un(){return!!document.body&&!!D("Mutation")}function td(e){if(Un()){queueMicrotask(e);return}let t=Date.now()+Qu,r=!1,n=null,o=null,i=()=>{r||(r=!0,n!==null&&clearInterval(n),o&&o())};function s(){if(R.isOpen){i();return}Un()&&(i(),e())}o=We(document,Vi,s),n=setInterval(()=>{if(R.isOpen){i();return}if(Un()){i(),e();return}Date.now()>=t&&(i(),console.warn("hypercms: ?cms=true auto-open gave up \u2014 no mutation hub appeared. Load clayjs or hyperclayjs (or just the mutation utility) so the CMS can initialize."))},Zu)}ed();var Vn={open:Qs,close:el,refresh:tl,api:Ju,hasRules:(e=document)=>!!U.findRules(e,"cms"),prepareModeLease:rl,modeParticipant:Xu,get isOpen(){return R.isOpen},currentView:Yu,path:Zr,scaffold:bt,morphForm:$t};function rd(e){return typeof CSS<"u"&&CSS.escape?CSS.escape(e):String(e).replace(/[^a-zA-Z0-9_\-.*]/g,t=>"\\"+t)}var nl=`/* GENERATED by scripts/build-theme.js from mirk-interface/mirk.css \u2014 DO NOT EDIT.
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
`;typeof window<"u"&&typeof document<"u"&&(function(){if(window.__mirk)return;window.__mirk=!0;let e='<svg viewBox="0 0 16 16" aria-hidden="true"><path d="M4 4 12 12M12 4 4 12" stroke="currentColor" stroke-width="2" stroke-linecap="square" fill="none"/></svg>';document.addEventListener("click",r=>{let n=r.target.closest(".mirk-number__step");if(!n)return;let o=n.closest(".mirk-number").querySelector("input[type=number]");o&&(n.dataset.step==="up"?o.stepUp():o.stepDown(),o.dispatchEvent(new Event("change",{bubbles:!0})))}),document.addEventListener("input",r=>{let n=r.target.closest(".mirk-slider__input");n&&n.closest(".mirk-slider").style.setProperty("--mirk-value",`${n.value}%`)}),document.addEventListener("change",r=>{let n=r.target.closest(".mirk-file__input");if(!n||!n.files.length)return;let o=n.closest(".mirk-file"),i=o.querySelector(".mirk-file__name");if(!i)return;let s=n.files[0],l=document.createElement("a");if(l.className="mirk-file__name",l.dataset.filled="",l.href=URL.createObjectURL(s),l.target="_blank",l.rel="noopener",l.textContent=s.name,i.replaceWith(l),!o.querySelector(".mirk-file__remove")){let a=document.createElement("button");a.type="button",a.className="mirk-file__remove",a.setAttribute("aria-label","Remove file"),a.innerHTML=e,l.after(a)}}),document.addEventListener("change",r=>{let n=r.target.closest(".mirk-image__input");if(!n||!n.files.length)return;let o=n.closest(".mirk-image"),i=o.querySelector(".mirk-image__preview");if(!i)return;let s=o.querySelector(".mirk-image__placeholder"),l=new FileReader;l.onload=a=>{i.src=a.target.result,i.removeAttribute("hidden"),s&&s.setAttribute("hidden",""),o.querySelector(".mirk-image__thumb")?.removeAttribute("hidden"),o.querySelector(".mirk-image__upload")?.setAttribute("hidden","")},l.readAsDataURL(n.files[0])}),document.addEventListener("click",r=>{let n=r.target.closest(".mirk-file__remove");if(n){let i=n.closest(".mirk-file"),s=i?.querySelector(".mirk-file__input"),l=i?.querySelector(".mirk-file__name");if(s&&(s.value=""),l){let a=document.createElement("span");a.className="mirk-file__name",a.textContent="No file chosen",l.replaceWith(a)}n.remove();return}let o=r.target.closest(".mirk-image__remove");if(o){let i=o.closest(".mirk-image"),s=i?.querySelector(".mirk-image__input"),l=i?.querySelector(".mirk-image__preview");s&&(s.value=""),l&&(l.removeAttribute("src"),l.setAttribute("hidden","")),i?.querySelector(".mirk-image__thumb")?.setAttribute("hidden",""),i?.querySelector(".mirk-image__upload")?.removeAttribute("hidden")}});function t(r,n){let o=document.createElement("span");o.textContent=r;let i=document.createElement("input");i.type="hidden",i.name="tags[]",i.value=r;let s=document.createElement("button");s.type="button",s.className="mirk-tags__remove",s.textContent="\xD7";let l=document.createElement("span");if(l.className="mirk-tags__chip",n){let a=document.createElement("span");a.className="mirk-tags__chip-inner",a.append(o,i,s),l.append(a)}else l.append(o,i,s);return l}document.addEventListener("keydown",r=>{let n=r.target.closest(".mirk-tags__input");if(!n)return;let o=n.closest(".mirk-tags");if(r.key==="Enter"||r.key===","){let i=n.value.trim();if(!i)return;r.preventDefault(),n.before(t(i,o.classList.contains("mirk-tags--round"))),n.value=""}else if(r.key==="Backspace"&&!n.value){let i=o.querySelectorAll(".mirk-tags__chip");i[i.length-1]?.remove()}}),document.addEventListener("click",r=>{let n=r.target.closest(".mirk-tags__remove");if(n){n.closest(".mirk-tags__chip").remove();return}let o=r.target.closest(".mirk-tags");o&&r.target===o&&o.querySelector(".mirk-tags__input")?.focus()}),document.addEventListener("click",r=>{let n=r.target.closest("[data-mirk-chip]");if(!n)return;let o=n.getAttribute("data-mirk-chip");if(o==="open")n.closest(".mirk-chip")?.classList.add("mirk-chip--open");else if(o==="collapse")n.closest(".mirk-chip")?.classList.remove("mirk-chip--open");else if(o==="changes"){let i=n.closest(".mirk-chip__panel")?.classList.toggle("is-changes");n.textContent=i?"(hide changes)":"(view changes)"}}),document.addEventListener("click",r=>{let n=r.target.closest("[data-copy-btn]");if(!n)return;let o=n.closest("[data-copy]");if(!o)return;let i=o.cloneNode(!0);i.querySelectorAll("[data-copy-btn]").forEach(a=>a.remove());let l=o.getAttribute("data-copy")==="text"?i.textContent.replace(/^\s+|\s+$/g,""):i.innerHTML.replace(/\s+data-copy(="[^"]*")?/g,"").replace(/^\s*\n/gm,"").trim();navigator.clipboard.writeText(l).then(()=>{let a=n.textContent;n.textContent="copied",n.dataset.copied="",setTimeout(()=>{n.textContent=a,delete n.dataset.copied},1200)}).catch(()=>{n.textContent="error",setTimeout(()=>{n.textContent="copy"},1200)})})})();Zs(nl);var _r=Vn,id={cms:Vn};typeof window<"u"&&!window.__hyperclayNoAutoExport&&(window.hyperclay=window.hyperclay||{},window.hyperclay.hypercms=_r,window.h=window.hyperclay);var Ng=_r,Mg=Wn;export{Ng as cms,Mg as default};
