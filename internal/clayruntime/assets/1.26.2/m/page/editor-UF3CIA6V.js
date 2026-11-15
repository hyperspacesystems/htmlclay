import{a as ho,c as R}from"../c-OBMZASYQ.js";import"../c-IW6GGOU2.js";import"../c-HAWFNBYG.js";import{b as go}from"../c-7NCWENHV.js";import{Q as So,R as Mo,f as lo,j as bo,k as po,l as ko,m as so,o as uo,r as xo,u as vo,z as wo}from"../c-WHMZCRQD.js";import"../c-LAKBBJOU.js";import"../c-EIUKME3W.js";import{n as io}from"../c-DZT6W2NG.js";import"../c-SYOA4T7V.js";import"../c-ND3W7D5U.js";import"../c-WHQUZM2V.js";import{A as Ao,B as Po,C as Co,I as Bo,b as ro,c as to,f as V,k as yo,p as zo,x as Eo}from"../c-3XGLJAS7.js";import{A as Q,E as Z,G as oo,J as eo,d as g,n as Y,x as J}from"../c-OH2FPYSY.js";import{a as q,c as Do,d as T}from"../c-N362HNGO.js";import{a as fo}from"../c-Z6MROEQ4.js";import{a as mo}from"../c-Q26XHBLQ.js";import{a as co,b as ao,d as no}from"../c-FSQXFG5T.js";import{a as U,b as h,d as m,e as S,f as M,g as y,h as X,i as G}from"../c-RETTCOWO.js";import"../c-JC4RZW4E.js";import"../c-57XDY6HC.js";import{b as L}from"../c-NL4I4KNG.js";import"../c-7SWMYKAI.js";import"../c-UGHH4MB3.js";import"../c-MZCOGNZL.js";import"../c-IJUK2GPW.js";import"../c-MOP5KBWZ.js";import"../c-2YYDMLNZ.js";import"../c-HA6INT6J.js";import"../c-ICMAOOGK.js";import"../c-WRBKEJD4.js";import"../c-VHNSBD2M.js";var{Decoration:$,DecorationSet:_o}=m,No="Type / for blocks",Wo="Untitled",Uo="To do";function Xo(o){let e=!1,r=null;return o.forEach((t,i)=>{e||t.type!==g.nodes.heading||t.attrs.level!==1||(t.attrs.parent??null)!==null||(e=!0,t.content.size||(r=$.node(i,i+t.nodeSize,{class:"blockeditor-title-placeholder","data-placeholder":Wo})))}),r}function Go(o){let e=[];return o.forEach((r,t)=>{r.type!==g.nodes.list_item||r.attrs.kind!=="check"||r.content.size||r.attrs.checked||e.push($.node(t,t+r.nodeSize,{class:"blockeditor-check-placeholder","data-placeholder":Uo}))}),e}function Fo(o=No){return new h.Plugin({props:{decorations(e){let r=Go(e.doc),t=Xo(e.doc);t&&r.push(t);let{selection:i}=e;if(i.empty){let d=i.$from;if(d.depth===1){let c=d.parent;if(c.type===g.nodes.paragraph&&!c.content.size){let b=d.before(1);r.push($.node(b,b+c.nodeSize,{class:"blockeditor-placeholder","data-placeholder":o}))}}}return r.length?_o.create(e.doc,r):null}}})}var To=new h.Plugin({props:{handleKeyDown(o,e){if(!(e.metaKey||e.ctrlKey)||!e.shiftKey||e.altKey)return!1;let r=e.key.toLowerCase()==="o"?"owner":e.key.toLowerCase()==="d"?"date":null;if(!r||o.dom.getAttribute(r==="owner"?"data-assignees":"data-dates")==="false")return!1;for(let t of co.get(o)??[])if(t.focusControl?.(o,r))return!0;return!1}}});var Oo="blockeditor-editor-style",Yo=[1,2,3,4,5,6,7,8],Jo=`
  --blockeditor-bg: #F7F6F2; --blockeditor-ink: #23221F; --blockeditor-ink-soft: #6B6862; --blockeditor-ink-faint: #9B968C;
  --blockeditor-rule: rgba(35, 34, 31, 0.10); --blockeditor-accent: #2F5E9E; --blockeditor-link: #164583; --blockeditor-selection: rgba(88, 120, 170, 0.20);
  --blockeditor-check-border: rgba(35, 34, 31, 0.35); --blockeditor-check-fill: #23221F; --blockeditor-check-ink: #F7F6F2;
  --blockeditor-callout-bg: rgba(35, 34, 31, 0.04); --blockeditor-code-bg: rgba(35, 34, 31, 0.05);
  --blockeditor-color-gray: #5B5953; --blockeditor-color-brown: #8A5A1E; --blockeditor-color-orange: #B4531A; --blockeditor-color-yellow: #7A6210; --blockeditor-color-green: #3E6A28;
  --blockeditor-color-blue: #1F5A8A; --blockeditor-color-purple: #5E4596; --blockeditor-color-pink: #A23E6E; --blockeditor-color-red: #9E3B2F;
  --blockeditor-mark-bg-gray: rgba(35, 34, 31, 0.12); --blockeditor-mark-bg-brown: rgba(138, 90, 30, 0.20); --blockeditor-mark-bg-orange: rgba(180, 83, 26, 0.20);
  --blockeditor-mark-bg-yellow: rgba(122, 98, 16, 0.22); --blockeditor-mark-bg-green: rgba(62, 106, 40, 0.20); --blockeditor-mark-bg-blue: rgba(31, 90, 138, 0.18);
  --blockeditor-mark-bg-purple: rgba(94, 69, 150, 0.18); --blockeditor-mark-bg-pink: rgba(162, 62, 110, 0.18); --blockeditor-mark-bg-red: rgba(158, 59, 47, 0.18);
  --blockeditor-code-token-keyword: #8A3B6E; --blockeditor-code-token-constant: #1F5A8A; --blockeditor-code-token-string: #3E6A28; --blockeditor-code-token-string-expression: #3E6A28; --blockeditor-code-token-comment: #8B877E; --blockeditor-code-token-function: #5E4596; --blockeditor-code-token-parameter: #8A5A1E; --blockeditor-code-token-punctuation: var(--blockeditor-ink-soft); --blockeditor-code-token-link: var(--blockeditor-link);
  --blockeditor-chrome-ink-rest: rgba(35, 34, 31, 0.66); --blockeditor-chrome-ink-moved: rgba(35, 34, 31, 0.80);
  --blockeditor-chrome-bg-rest: rgba(35, 34, 31, 0.02); --blockeditor-chrome-bg-moved: rgba(35, 34, 31, 0.04);
  --blockeditor-chrome-line-rest: rgba(35, 34, 31, 0.06); --blockeditor-chrome-line-moved: rgba(35, 34, 31, 0.11);
  --blockeditor-pop-bg: #FBFAF7; --blockeditor-pop-ink: #23221F; --blockeditor-pop-muted: #6B6862; --blockeditor-pop-line: rgba(35, 34, 31, 0.10);
  --blockeditor-pop-shadow: 0 1px 2px rgba(35, 34, 31, 0.05), 0 8px 24px rgba(35, 34, 31, 0.08); --blockeditor-pop-hover: rgba(35, 34, 31, 0.06);
  --blockeditor-scrim: rgba(35, 34, 31, 0.12);
  --blockeditor-danger: #9E3B2F; --blockeditor-block-selected: rgba(88, 120, 170, 0.12);`,jo=`
  --blockeditor-bg: #1A1A18; --blockeditor-ink: #E7E4DC; --blockeditor-ink-soft: #A3A098; --blockeditor-ink-faint: #75726B;
  --blockeditor-rule: rgba(231, 228, 220, 0.10); --blockeditor-accent: #8DB0E6; --blockeditor-link: #8DB0E6; --blockeditor-selection: rgba(120, 155, 210, 0.28);
  --blockeditor-check-border: rgba(231, 228, 220, 0.35); --blockeditor-check-fill: #E7E4DC; --blockeditor-check-ink: #1A1A18;
  --blockeditor-callout-bg: rgba(231, 228, 220, 0.05); --blockeditor-code-bg: rgba(231, 228, 220, 0.06);
  --blockeditor-color-gray: #A3A098; --blockeditor-color-brown: #E0B47A; --blockeditor-color-orange: #E8A177; --blockeditor-color-yellow: #DEC97A; --blockeditor-color-green: #A9C98B;
  --blockeditor-color-blue: #8DB0E6; --blockeditor-color-purple: #BBA6E6; --blockeditor-color-pink: #E3A3C4; --blockeditor-color-red: #E08A7A;
  --blockeditor-mark-bg-gray: rgba(231, 228, 220, 0.14); --blockeditor-mark-bg-brown: rgba(224, 180, 122, 0.22); --blockeditor-mark-bg-orange: rgba(232, 161, 119, 0.22);
  --blockeditor-mark-bg-yellow: rgba(222, 201, 122, 0.24); --blockeditor-mark-bg-green: rgba(169, 201, 139, 0.22); --blockeditor-mark-bg-blue: rgba(141, 176, 230, 0.22);
  --blockeditor-mark-bg-purple: rgba(187, 166, 230, 0.22); --blockeditor-mark-bg-pink: rgba(227, 163, 196, 0.22); --blockeditor-mark-bg-red: rgba(224, 138, 122, 0.22);
  --blockeditor-code-token-keyword: #D9A0C8; --blockeditor-code-token-constant: #8DB0E6; --blockeditor-code-token-string: #A9C98B; --blockeditor-code-token-string-expression: #A9C98B; --blockeditor-code-token-comment: #75726B; --blockeditor-code-token-function: #BBA6E6; --blockeditor-code-token-parameter: #E0B47A; --blockeditor-code-token-punctuation: var(--blockeditor-ink-soft); --blockeditor-code-token-link: var(--blockeditor-link);
  --blockeditor-chrome-ink-rest: rgba(231, 228, 220, 0.58); --blockeditor-chrome-ink-moved: rgba(231, 228, 220, 0.72);
  --blockeditor-chrome-bg-rest: rgba(231, 228, 220, 0.02); --blockeditor-chrome-bg-moved: rgba(231, 228, 220, 0.04);
  --blockeditor-chrome-line-rest: rgba(231, 228, 220, 0.06); --blockeditor-chrome-line-moved: rgba(231, 228, 220, 0.11);
  --blockeditor-pop-bg: #222220; --blockeditor-pop-ink: #E7E4DC; --blockeditor-pop-muted: #A3A098; --blockeditor-pop-line: rgba(231, 228, 220, 0.10);
  --blockeditor-pop-shadow: 0 1px 2px rgba(10, 10, 9, 0.3), 0 10px 28px rgba(10, 10, 9, 0.35); --blockeditor-pop-hover: rgba(231, 228, 220, 0.07);
  --blockeditor-scrim: rgba(10, 10, 9, 0.35);
  --blockeditor-danger: #E08A7A; --blockeditor-block-selected: rgba(120, 155, 210, 0.18);
  --chart-1: #8DB0E6; --chart-2: #D9A0C8; --chart-3: #A9C98B; --chart-4: #BBA6E6; --chart-5: #E0B47A; --chart-6: #A3A098;`,Qo=`
@layer blockeditor {
:where(article[data-blockeditor], .blockeditor-shell) {${Jo}
  --chart-1: #2F5E9E; --chart-2: #164583; --chart-3: #5B5953; --chart-4: #8A3B6E; --chart-5: #3E6A28; --chart-6: #8A5A1E;
  --blockeditor-ink-strong: var(--blockeditor-ink); --blockeditor-caret: var(--blockeditor-accent);
  --blockeditor-font: system-ui, -apple-system, "Segoe UI", Roboto, sans-serif; --blockeditor-font-display: var(--blockeditor-font); --blockeditor-font-heading: var(--blockeditor-font);
  --blockeditor-font-mono: ui-monospace, "SF Mono", Menlo, Consolas, monospace;
  --blockeditor-font-size: 17px; --blockeditor-line-height: 1.65; --blockeditor-measure: 680px; --blockeditor-pad-top: 72px;
  --blockeditor-h1-size: 34px; --blockeditor-h1-weight: 600; --blockeditor-h1-tracking: -0.01em; --blockeditor-h1-line: 1.2;
  --blockeditor-h2-size: 21px; --blockeditor-h2-weight: 600; --blockeditor-h3-size: 17px; --blockeditor-h3-weight: 600;
  --blockeditor-block-gap: 0.35em; --blockeditor-indent: 1.6em;
  --blockeditor-marker: var(--blockeditor-ink-soft); --blockeditor-bullet-size: 0.36em; --blockeditor-check-size: 16px; --blockeditor-check-radius: 4px; --blockeditor-done-ink: var(--blockeditor-ink-faint);
  --blockeditor-toggle-ink: var(--blockeditor-ink-soft); --blockeditor-callout-line: transparent; --blockeditor-code-ink: var(--blockeditor-ink);
  --blockeditor-chrome-shadow-rest: none; --blockeditor-chrome-shadow-moved: none; --blockeditor-chrome-radius: 7px; --blockeditor-plus-size: 24px; --blockeditor-fade: 240ms;
  --blockeditor-pop-radius: 10px; --blockeditor-bar-bg: var(--blockeditor-pop-bg); --blockeditor-bar-ink: var(--blockeditor-pop-ink);
}
@media (prefers-color-scheme: dark) { :where(:root:not([data-theme="light"]) :is(article[data-blockeditor], .blockeditor-shell)) {${jo} } }
:where(:root[data-theme="dark"] :is(article[data-blockeditor], .blockeditor-shell)) {${jo} }

.blockeditor-shell { position: relative; background: var(--blockeditor-bg); color: var(--blockeditor-ink); font-family: var(--blockeditor-font); font-size: var(--blockeditor-font-size); line-height: var(--blockeditor-line-height); padding: 0 16px; }
.blockeditor-editor { max-width: var(--blockeditor-measure); margin: 0 auto; padding: var(--blockeditor-pad-top) 0 30vh; outline: none; white-space: pre-wrap; overflow-wrap: break-word; caret-color: var(--blockeditor-caret); }
:where(article[data-blockeditor], .blockeditor-editor) ::selection { background: var(--blockeditor-selection); }
:where(article[data-blockeditor], .blockeditor-editor) > * { margin: var(--blockeditor-block-gap) 0; margin-inline-start: calc(var(--blockeditor-depth, 0) * var(--blockeditor-indent)); }
:where(article[data-blockeditor], .blockeditor-editor) > h1 { font-family: var(--blockeditor-font-display); font-size: var(--blockeditor-h1-size); font-weight: var(--blockeditor-h1-weight); letter-spacing: var(--blockeditor-h1-tracking); line-height: var(--blockeditor-h1-line); color: var(--blockeditor-ink-strong); margin-block: 0 0.6em; }
:where(article[data-blockeditor], .blockeditor-editor) > h2 { font-family: var(--blockeditor-font-heading); font-size: var(--blockeditor-h2-size); font-weight: var(--blockeditor-h2-weight); color: var(--blockeditor-ink-strong); line-height: 1.35; margin-block-start: 1.4em; }
:where(article[data-blockeditor], .blockeditor-editor) > h3 { font-family: var(--blockeditor-font-heading); font-size: var(--blockeditor-h3-size); font-weight: var(--blockeditor-h3-weight); color: var(--blockeditor-ink-strong); margin-block-start: 1.2em; }
:where(article[data-blockeditor], .blockeditor-editor) a { color: var(--blockeditor-link, var(--blockeditor-accent)); }
:where(article[data-blockeditor], .blockeditor-editor) code { font-family: var(--blockeditor-font-mono); font-size: 0.88em; background: var(--blockeditor-code-bg); color: var(--blockeditor-code-ink); border-radius: 4px; padding: 0.1em 0.3em; }
:where(article[data-blockeditor], .blockeditor-editor) pre { font-family: var(--blockeditor-font-mono); font-size: 0.88em; background: var(--blockeditor-code-bg); color: var(--blockeditor-code-ink); border-radius: 8px; padding: 12px 14px; }
:where(article[data-blockeditor], .blockeditor-editor) pre code { background: none; padding: 0; font-size: inherit; }
:where(article[data-blockeditor], .blockeditor-editor) pre { position: relative; padding-top: 32px; white-space: pre-wrap; overflow-wrap: anywhere; }
:where(article[data-blockeditor], .blockeditor-editor) pre[data-lang] { white-space: pre; overflow-wrap: normal; }
:where(article[data-blockeditor], .blockeditor-editor) pre[data-lang] > code { display: block; overflow-x: auto; }
.blockeditor-code-lang { position: absolute; top: 5px; inset-inline-end: 6px; max-width: 120px; height: 24px; padding: 0 4px; border: 0; border-radius: 5px; background: none; color: var(--blockeditor-ink-faint); font: 12px/1 var(--blockeditor-font); opacity: 0; cursor: pointer; transition: opacity var(--blockeditor-fade); }
.blockeditor-editor pre:hover .blockeditor-code-lang, .blockeditor-editor pre:focus-within .blockeditor-code-lang, .blockeditor-code-lang:focus-visible, .blockeditor-editor pre[data-lang] .blockeditor-code-lang { opacity: 1; }
.blockeditor-code-lang:hover, .blockeditor-code-lang:focus-visible { background: var(--blockeditor-pop-hover); color: var(--blockeditor-ink-soft); }
:where(article[data-blockeditor], .blockeditor-editor) .blockeditor-callout { background: var(--blockeditor-callout-bg); border: 1px solid var(--blockeditor-callout-line); border-radius: 8px; padding: 10px 14px; }
/* The four floor-1 formats. A quote is set apart by its own inset and ink, with
   no left-border accent; the palette's keys are fixed tokens, in both themes. */
:where(article[data-blockeditor], .blockeditor-editor) blockquote { padding-inline-start: var(--blockeditor-indent); color: var(--blockeditor-ink-soft); }
:where(article[data-blockeditor], .blockeditor-editor) hr { border: 0; border-top: 1px solid var(--blockeditor-rule); margin-block: 1.2em; }
:where(article[data-blockeditor], .blockeditor-editor) .blockeditor-hidden { display: none; }
:where(article[data-blockeditor], .blockeditor-editor) .blockeditor-li { position: relative; --li-indent: 0; padding-inline-start: calc((var(--li-indent) + 1) * var(--blockeditor-indent)); }
${Yo.map(o=>`:where(article[data-blockeditor], .blockeditor-editor) .blockeditor-li[data-indent="${o}"] { --li-indent: ${o}; }`).join(`
`)}
:where(article[data-blockeditor], .blockeditor-editor) .blockeditor-li::before { position: absolute; inset-inline-start: calc(var(--li-indent) * var(--blockeditor-indent)); width: var(--blockeditor-indent); text-align: center; color: var(--blockeditor-marker); content: "\\2022"; }
:where(article[data-blockeditor], .blockeditor-editor) .blockeditor-li[data-kind="bullet"]::before { content: ""; box-sizing: border-box; width: var(--blockeditor-bullet-size); height: var(--blockeditor-bullet-size); top: calc((1em * var(--blockeditor-line-height) - var(--blockeditor-bullet-size)) / 2); inset-inline-start: calc(var(--li-indent) * var(--blockeditor-indent) + (var(--blockeditor-indent) - var(--blockeditor-bullet-size)) / 2); border-radius: 50%; background: currentColor; }
:where(article[data-blockeditor], .blockeditor-editor) .blockeditor-li[data-kind="ordered"]::before { content: attr(data-marker); font-variant-numeric: tabular-nums; }
:where(article[data-blockeditor], .blockeditor-editor) .blockeditor-li[data-kind="check"]::before { content: ""; box-sizing: border-box; width: var(--blockeditor-check-size); height: var(--blockeditor-check-size); top: calc((1em * var(--blockeditor-line-height) - var(--blockeditor-check-size)) / 2); inset-inline-start: calc(var(--li-indent) * var(--blockeditor-indent) + (var(--blockeditor-indent) - var(--blockeditor-check-size)) / 2); border: 1px solid var(--blockeditor-check-border); border-radius: var(--blockeditor-check-radius); cursor: pointer; }
:where(article[data-blockeditor], .blockeditor-editor) .blockeditor-li[data-kind="check"][data-checked="true"]::before { background: var(--blockeditor-check-fill); border-color: var(--blockeditor-check-fill); }
:where(article[data-blockeditor], .blockeditor-editor) .blockeditor-li[data-kind="check"][data-checked="true"]::after { content: ""; position: absolute; width: var(--blockeditor-check-size); height: var(--blockeditor-check-size); top: calc((1em * var(--blockeditor-line-height) - var(--blockeditor-check-size)) / 2); inset-inline-start: calc(var(--li-indent) * var(--blockeditor-indent) + (var(--blockeditor-indent) - var(--blockeditor-check-size)) / 2); background: var(--blockeditor-check-ink); -webkit-mask: url("data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' viewBox='0 0 16 16'%3E%3Cpath d='M3.8 8.4l2.9 2.9 5.5-6.4' fill='none' stroke='%23000' stroke-width='2.2' stroke-linecap='round' stroke-linejoin='round'/%3E%3C/svg%3E") center / 72% no-repeat; mask: url("data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' viewBox='0 0 16 16'%3E%3Cpath d='M3.8 8.4l2.9 2.9 5.5-6.4' fill='none' stroke='%23000' stroke-width='2.2' stroke-linecap='round' stroke-linejoin='round'/%3E%3C/svg%3E") center / 72% no-repeat; pointer-events: none; }
:where(article[data-blockeditor], .blockeditor-editor) .blockeditor-li[data-kind="check"][data-checked="true"] { color: var(--blockeditor-done-ink); text-decoration: line-through; }
:where(article[data-blockeditor], .blockeditor-editor) .blockeditor-toggle { position: relative; padding-inline-start: var(--blockeditor-indent); }
.blockeditor-editor .blockeditor-toggle-button { position: absolute; inset-inline-start: 0; top: 0; width: var(--blockeditor-indent); height: calc(1em * var(--blockeditor-line-height)); display: grid; place-items: center; border: 0; padding: 0; background: none; color: var(--blockeditor-toggle-ink); font: inherit; line-height: 1; cursor: pointer; transition: transform var(--blockeditor-fade); }
.blockeditor-editor .blockeditor-toggle-button::before { content: "\\25B8"; font-size: 1.4em; }
.blockeditor-editor .blockeditor-toggle[data-expanded="true"] > .blockeditor-toggle-button { transform: rotate(90deg); }

.blockeditor-dock, .blockeditor-plus { color: var(--blockeditor-chrome-ink-rest); background: var(--blockeditor-chrome-bg-rest); border: 1px solid var(--blockeditor-chrome-line-rest); box-shadow: var(--blockeditor-chrome-shadow-rest); border-radius: var(--blockeditor-chrome-radius); transition: color var(--blockeditor-fade), background-color var(--blockeditor-fade), border-color var(--blockeditor-fade), box-shadow var(--blockeditor-fade); }
.blockeditor-shell.is-moved .blockeditor-dock, .blockeditor-shell.is-moved .blockeditor-plus { color: var(--blockeditor-chrome-ink-moved); background: var(--blockeditor-chrome-bg-moved); border-color: var(--blockeditor-chrome-line-moved); box-shadow: var(--blockeditor-chrome-shadow-moved); }
.blockeditor-dock { position: sticky; top: 12px; z-index: 3; display: flex; align-items: center; gap: 2px; width: max-content; margin: 12px auto 0; padding: 3px; }
/* One formatting bar at a time: the selection bar covers what the fixed bar offers. Hidden, not removed, so the page does not shift under the selection. */
.blockeditor-shell:has(.blockeditor-bubble:not([hidden])) .blockeditor-dock { visibility: hidden; }
.blockeditor-dock, .blockeditor-shell.is-moved .blockeditor-dock { background: var(--blockeditor-pop-bg); }
.blockeditor-dock + .blockeditor-editor { padding-top: max(16px, calc(var(--blockeditor-pad-top) - 48px)); }
.blockeditor-tool { display: inline-flex; align-items: center; justify-content: center; min-width: 28px; height: 28px; padding: 0 6px; border: 0; border-radius: 5px; background: none; color: inherit; font: 500 13px/1 var(--blockeditor-font); cursor: pointer; }
.blockeditor-tool:hover, .blockeditor-tool[aria-pressed="true"] { background: var(--blockeditor-pop-hover); }
.blockeditor-tool-turn { gap: 4px; padding: 0 8px; }
/* The phone's fourth control folds away everything the narrow bar hides; on a desktop it
   holds what has no button of its own (colors, icon and cover, the exports). */
.blockeditor-tool-sep { width: 1px; height: 16px; margin: 0 3px; background: currentColor; opacity: 0.5; }
.blockeditor-safe-probe { position: fixed; left: 0; bottom: 0; width: 0; height: 0; padding-bottom: env(safe-area-inset-bottom); visibility: hidden; pointer-events: none; }
.blockeditor-plus { position: absolute; z-index: 2; top: calc(var(--blockeditor-plus-top, 0px) - var(--blockeditor-plus-size) / 2); left: calc(var(--blockeditor-plus-left, 0px) - var(--blockeditor-plus-size) - 12px); width: var(--blockeditor-plus-size); height: var(--blockeditor-plus-size); display: grid; place-items: center; padding: 0; cursor: pointer; }
.blockeditor-plus[aria-expanded="true"] { color: var(--blockeditor-chrome-ink-moved); background: var(--blockeditor-chrome-bg-moved); }
.blockeditor-grip { position: absolute; z-index: 2; top: calc(var(--blockeditor-plus-top, 0px) - var(--blockeditor-grip-size, var(--blockeditor-plus-size)) / 2); left: calc(var(--blockeditor-plus-left, 0px) - var(--blockeditor-plus-size) - 12px - var(--blockeditor-grip-size, var(--blockeditor-plus-size)) - 2px); width: var(--blockeditor-grip-size, var(--blockeditor-plus-size)); height: var(--blockeditor-grip-size, var(--blockeditor-plus-size)); display: grid; place-items: center; padding: 0; border: 0; border-radius: var(--blockeditor-chrome-radius); background: none; color: var(--blockeditor-chrome-ink-moved); opacity: 0; cursor: grab; touch-action: none; transition: opacity var(--blockeditor-fade); }
.blockeditor-shell.is-moved .blockeditor-grip, .blockeditor-grip.is-on, .blockeditor-grip:focus-visible, .blockeditor-grip[aria-expanded="true"] { opacity: 1; }
.blockeditor-bubble, .blockeditor-menu { position: absolute; z-index: 4; background: var(--blockeditor-pop-bg); color: var(--blockeditor-pop-ink); border: 1px solid var(--blockeditor-pop-line); box-shadow: var(--blockeditor-pop-shadow); border-radius: var(--blockeditor-pop-radius); }
.blockeditor-bubble { display: flex; gap: 1px; padding: 3px; left: var(--blockeditor-pop-x, 0px); top: var(--blockeditor-pop-y, 0px); transform: translate(-50%, calc(-100% - 8px)); }
.blockeditor-menu { left: var(--blockeditor-pop-x, 0px); top: var(--blockeditor-pop-y, 0px); width: 260px; max-height: 340px; overflow-y: auto; padding: 6px; font-size: 14px; }
.blockeditor-settings { position: absolute; z-index: 4; box-sizing: border-box; left: var(--blockeditor-pop-x, 0px); top: var(--blockeditor-pop-y, 0px); width: min(264px, calc(100vw - 28px)); padding: 9px 11px; background: var(--blockeditor-pop-bg); color: var(--blockeditor-pop-ink); border: 1px solid var(--blockeditor-pop-line); border-radius: var(--blockeditor-pop-radius); box-shadow: var(--blockeditor-pop-shadow); font-size: 13px; }
.blockeditor-settings-head { display: flex; align-items: center; justify-content: space-between; gap: 10px; padding-bottom: 6px; margin-bottom: 2px; border-bottom: 1px solid var(--blockeditor-pop-line); }
.blockeditor-settings-title { font-size: 12.5px; font-weight: 600; color: var(--blockeditor-pop-ink); }
.blockeditor-settings-close { display: inline-grid; place-content: center; width: 20px; height: 20px; padding: 0; border: 1px solid transparent; border-radius: 5px; background: none; color: var(--blockeditor-pop-muted); cursor: pointer; }
.blockeditor-settings-close:hover { background: var(--blockeditor-pop-hover); color: var(--blockeditor-pop-ink); }
.blockeditor-settings-row { display: flex; align-items: center; justify-content: space-between; gap: 10px; padding: 6px 0; }
.blockeditor-settings-name { min-width: 0; font-weight: 500; line-height: 1.3; }
.blockeditor-settings-switch { position: relative; flex: 0 0 30px; box-sizing: border-box; width: 30px; height: 18px; padding: 0; border: 0; border-radius: 9px; background: var(--blockeditor-chrome-line-moved); cursor: pointer; transition: background-color var(--blockeditor-fade); }
.blockeditor-settings-switch[aria-checked="true"] { background: var(--blockeditor-accent); }
.blockeditor-settings-knob { position: absolute; top: 2px; left: 2px; width: 14px; height: 14px; border-radius: 50%; background: var(--blockeditor-pop-bg); transition: transform var(--blockeditor-fade); }
.blockeditor-settings-switch[aria-checked="true"] .blockeditor-settings-knob { transform: translateX(12px); }
.blockeditor-settings-note { margin: 4px 0 2px; color: var(--blockeditor-pop-muted); font-size: 11px; line-height: 1.4; }
.blockeditor-menu-title { padding: 4px 10px 6px; color: var(--blockeditor-pop-muted); font-size: 12px; }
.blockeditor-menu-item { display: flex; align-items: center; gap: 10px; width: 100%; min-height: 34px; padding: 4px 10px; border: 0; border-radius: 7px; background: none; color: inherit; font: inherit; text-align: start; cursor: pointer; }
.blockeditor-menu-item[aria-selected="true"], .blockeditor-menu-item.is-active, .blockeditor-menu-item:hover { background: var(--blockeditor-pop-hover); }
.blockeditor-menu-icon { display: grid; place-items: center; width: 24px; height: 24px; border: 1px solid var(--blockeditor-pop-line); border-radius: 6px; color: var(--blockeditor-pop-muted); font-size: 12px; flex: none; }
.blockeditor-menu-icon svg { display: block; }
.blockeditor-menu-hint { margin-inline-start: auto; color: var(--blockeditor-pop-muted); font-size: 12px; }
.blockeditor-menu-text { display: flex; flex-direction: column; min-width: 0; }
.blockeditor-menu-desc { color: var(--blockeditor-pop-muted); font-size: 12px; line-height: 1.3; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.blockeditor-menu-empty { padding: 8px 10px; color: var(--blockeditor-pop-muted); font-size: 13px; }
.blockeditor-menu-note { padding: 8px 10px; color: var(--blockeditor-pop-muted); font-size: 13px; }
.blockeditor-shell.is-dragging, .blockeditor-shell.is-dragging .blockeditor-grip { cursor: grabbing; }
.blockeditor-shell[data-drop="none"] { cursor: no-drop; }
.blockeditor-drop { position: absolute; z-index: 3; left: var(--blockeditor-drop-x, 0px); top: var(--blockeditor-drop-y, 0px); width: var(--blockeditor-drop-w, 0px); height: 2px; margin-top: -1px; border-radius: 1px; background: var(--blockeditor-drop, var(--blockeditor-accent)); pointer-events: none; }
.blockeditor-shell.is-dragging .blockeditor-block-selected { opacity: 0.45; }
.blockeditor-editor.ProseMirror-hideselection { caret-color: transparent; }
.blockeditor-editor.ProseMirror-hideselection *::selection { background: transparent; }
.blockeditor-editor .blockeditor-block-selected { background: var(--blockeditor-block-selected, rgba(88, 120, 170, 0.12)); box-shadow: -6px 0 0 var(--blockeditor-block-selected, rgba(88, 120, 170, 0.12)), 6px 0 0 var(--blockeditor-block-selected, rgba(88, 120, 170, 0.12)); border-radius: 4px; }
.blockeditor-ai-hidden { display: none; }
.blockeditor-ai-preview { margin: var(--blockeditor-block-gap) 0; padding: 2px 8px; border-radius: 4px; background: var(--blockeditor-block-selected, rgba(88, 120, 170, 0.12)); }
.blockeditor-editor .blockeditor-link-pending { background: var(--blockeditor-selection); }
.blockeditor-link { position: absolute; z-index: 4; left: var(--blockeditor-pop-x, 0px); top: var(--blockeditor-pop-y, 0px); display: flex; align-items: center; gap: 2px; padding: 4px; max-width: min(420px, calc(100% - 32px)); background: var(--blockeditor-pop-bg); color: var(--blockeditor-pop-ink); border: 1px solid var(--blockeditor-pop-line); box-shadow: var(--blockeditor-pop-shadow); border-radius: var(--blockeditor-pop-radius); font-size: 14px; }
.blockeditor-link-input { flex: 1; min-width: 220px; height: 30px; padding: 0 8px; border: 0; border-radius: 6px; background: var(--blockeditor-pop-hover); color: inherit; font: inherit; outline: none; }
.blockeditor-link-input[aria-invalid="true"] { box-shadow: inset 0 0 0 1px var(--blockeditor-danger, #9E3B2F); }
.blockeditor-link-url { max-width: 260px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; padding: 0 8px; color: var(--blockeditor-pop-muted); text-decoration: none; }
.blockeditor-link-btn { display: inline-grid; place-items: center; min-width: 30px; height: 30px; padding: 0 6px; border: 0; border-radius: 6px; background: none; color: inherit; font: inherit; cursor: pointer; }
.blockeditor-link-btn:hover { background: var(--blockeditor-pop-hover); }
.blockeditor-live { position: absolute; width: 1px; height: 1px; overflow: hidden; clip-path: inset(50%); white-space: nowrap; }
.ProseMirror-focused .blockeditor-placeholder::before { content: attr(data-placeholder); float: left; height: 0; color: var(--blockeditor-ink-faint); pointer-events: none; }
.blockeditor-editor .blockeditor-title-placeholder::before { content: attr(data-placeholder); float: left; height: 0; color: var(--blockeditor-ink-faint); pointer-events: none; }
.blockeditor-csv-error { margin: 8px 0; color: var(--blockeditor-danger); font-size: 14px; }
.blockeditor-editor .blockeditor-table { max-width: 100%; min-width: 0; border: 1px solid var(--blockeditor-rule); border-radius: 8px; }
.blockeditor-editor .blockeditor-chart { position: relative; max-width: 100%; min-width: 0; margin-inline: 0; border: 1px solid var(--blockeditor-rule); border-radius: var(--blockeditor-chrome-radius); color: var(--blockeditor-ink); }
.blockeditor-chart-plot svg { display: block; width: 100%; height: auto; color: var(--chart-text, var(--blockeditor-ink-soft)); font: 12px var(--blockeditor-font); }
.blockeditor-chart-head { display: flex; flex-wrap: wrap; align-items: center; gap: 8px; padding: 10px 12px; border-bottom: 1px solid var(--blockeditor-rule); }
.blockeditor-chart-title { min-width: 0; margin-inline-end: auto; font-size: 14px; font-weight: 600; line-height: 1.3; }
.blockeditor-chart-title.is-empty { color: var(--blockeditor-ink-faint); font-weight: 500; }
.blockeditor-chart-source { display: inline-flex; align-items: center; gap: 5px; box-sizing: border-box; height: 22px; padding: 0 7px; border: 1px solid var(--blockeditor-pop-line); border-radius: var(--blockeditor-chrome-radius); background: var(--blockeditor-pop-bg); color: var(--blockeditor-pop-muted); font: inherit; font-size: 11.5px; cursor: pointer; }
.blockeditor-chart-source:hover { color: var(--blockeditor-ink); border-color: var(--blockeditor-chrome-line-moved); }
.blockeditor-chart-source svg, .blockeditor-chart-edit svg, .blockeditor-chart-remove svg { flex: none; }
.blockeditor-chart-edit { display: inline-flex; align-items: center; gap: 6px; box-sizing: border-box; height: 28px; padding: 0 10px; border: 1px solid var(--blockeditor-rule); border-radius: var(--blockeditor-chrome-radius); background: none; color: var(--blockeditor-ink); font: inherit; font-size: 12.5px; font-weight: 500; cursor: pointer; }
.blockeditor-chart-edit:hover, .blockeditor-chart-edit[aria-expanded="true"] { background: var(--blockeditor-pop-hover); }
.blockeditor-chart-panel { position: absolute; z-index: 5; top: 46px; inset-inline-end: 10px; box-sizing: border-box; width: 304px; padding: 12px; border: 1px solid var(--blockeditor-pop-line); border-radius: var(--blockeditor-pop-radius); background: var(--blockeditor-pop-bg); box-shadow: var(--blockeditor-pop-shadow); color: var(--blockeditor-pop-ink); font-family: var(--blockeditor-font); font-size: 13px; text-align: start; }
.blockeditor-chart-panel:focus { outline: none; }
.blockeditor-chart-row { display: flex; flex-direction: column; gap: 4px; margin-bottom: 10px; }
.blockeditor-chart-row:last-child { margin-bottom: 0; }
.blockeditor-chart-label { color: var(--blockeditor-pop-muted); font-size: 12px; font-weight: 500; }
.blockeditor-chart-types { display: grid; grid-template-columns: repeat(3, minmax(0, 1fr)); gap: 4px; }
.blockeditor-chart-type, .blockeditor-chart-remove, .blockeditor-chart-done { display: inline-flex; align-items: center; justify-content: center; gap: 6px; box-sizing: border-box; height: 28px; padding: 0 10px; border: 1px solid var(--blockeditor-pop-line); border-radius: var(--blockeditor-chrome-radius); background: none; color: var(--blockeditor-pop-ink); font: inherit; font-size: 12.5px; font-weight: 500; cursor: pointer; }
.blockeditor-chart-type:hover, .blockeditor-chart-remove:hover, .blockeditor-chart-done:hover { background: var(--blockeditor-pop-hover); }
.blockeditor-chart-type[aria-pressed="true"] { border-color: var(--blockeditor-chrome-line-moved); background: var(--blockeditor-pop-hover); font-weight: 600; }
.blockeditor-chart-select, .blockeditor-chart-title-input { box-sizing: border-box; width: 100%; height: 28px; padding: 0 8px; border: 1px solid var(--blockeditor-pop-line); border-radius: var(--blockeditor-chrome-radius); background: var(--blockeditor-pop-bg); color: var(--blockeditor-pop-ink); font: inherit; font-size: 12.5px; }
.blockeditor-chart-values { display: flex; flex-direction: column; gap: 1px; padding: 5px 6px; border: 1px solid var(--blockeditor-pop-line); border-radius: var(--blockeditor-chrome-radius); }
.blockeditor-chart-value { display: flex; align-items: center; gap: 7px; padding: 2px 4px; font-size: 12.5px; cursor: pointer; }
.blockeditor-chart-value:hover { background: var(--blockeditor-pop-hover); }
.blockeditor-chart-value input { margin: 0; accent-color: var(--blockeditor-accent); }
.blockeditor-chart-swatch { flex: none; width: 10px; height: 10px; border-radius: 2px; }
.blockeditor-chart-hint { color: var(--blockeditor-pop-muted); font-size: 11px; line-height: 1.4; }
.blockeditor-chart-foot { display: flex; align-items: center; justify-content: space-between; gap: 8px; margin-top: 10px; padding-top: 8px; border-top: 1px solid var(--blockeditor-pop-line); }
.blockeditor-chart-remove { color: var(--blockeditor-danger); }
.blockeditor-chart-broken { box-sizing: border-box; padding: 16px; border: 1px solid var(--blockeditor-pop-line); border-radius: var(--blockeditor-chrome-radius); background: var(--blockeditor-pop-hover); font-family: var(--blockeditor-font); text-align: start; }
.blockeditor-chart-broken[hidden] { display: none; }
.blockeditor-chart-plot[hidden] { display: none; }
.blockeditor-chart-broken-head { margin-bottom: 4px; font-size: 13.5px; font-weight: 600; line-height: 1.3; }
.blockeditor-chart-broken-text { margin: 0 0 12px; color: var(--blockeditor-pop-muted); font-size: 12.5px; line-height: 1.45; }
.blockeditor-chart-broken-actions { display: flex; flex-wrap: wrap; align-items: center; gap: 8px; }
.blockeditor-chart-use, .blockeditor-chart-delete { display: inline-flex; align-items: center; justify-content: center; box-sizing: border-box; height: 28px; padding: 0 10px; border: 1px solid var(--blockeditor-pop-line); border-radius: var(--blockeditor-chrome-radius); background: var(--blockeditor-pop-bg); color: var(--blockeditor-pop-ink); font: inherit; font-size: 12.5px; font-weight: 500; cursor: pointer; }
.blockeditor-chart-use:hover, .blockeditor-chart-delete:hover { background: var(--blockeditor-pop-bg); border-color: var(--blockeditor-chrome-line-moved); }
.blockeditor-chart-delete { color: var(--blockeditor-danger); }
.blockeditor-chart-broken-actions .blockeditor-chart-select { width: auto; min-width: 0; flex: 1 1 140px; }
@media (max-width: 560px) {
  .blockeditor-chart-panel { position: static; inset-inline-end: auto; width: auto; border: 0; border-top: 1px solid var(--blockeditor-pop-line); border-radius: 0; box-shadow: none; }
  .blockeditor-chart-use, .blockeditor-chart-delete, .blockeditor-chart-broken-actions .blockeditor-chart-select { height: 44px; }
}
.blockeditor-table-scroll { max-width: 100%; overflow-x: auto; }
.blockeditor-table table { border-collapse: collapse; min-width: max-content; width: 100%; table-layout: auto; }
.blockeditor-table th, .blockeditor-table td { min-width: 120px; padding: 6px 10px; border-inline-end: 1px solid var(--blockeditor-rule); border-block-end: 1px solid var(--blockeditor-rule); text-align: start; white-space: pre-wrap; }
.blockeditor-table th { background: var(--blockeditor-code-bg); color: var(--blockeditor-ink-soft); font-weight: 600; }
.blockeditor-table th[data-kind="number"], .blockeditor-table td[data-kind="number"] { text-align: end; }
.blockeditor-table th[aria-sort="ascending"] button::after { content: " \u2191"; }
.blockeditor-table th[aria-sort="descending"] button::after { content: " \u2193"; }
.blockeditor-table th button, .blockeditor-table-row-menu { border: 0; border-radius: 5px; background: none; color: inherit; font: inherit; cursor: pointer; }
.blockeditor-table th button:hover, .blockeditor-table-row-menu:hover { background: var(--blockeditor-pop-hover); }
.blockeditor-editor td > span { display: block; }
.blockeditor-table-row-menu { float: inline-end; margin-inline-start: 4px; opacity: 0; font-size: 12px; color: var(--blockeditor-ink-soft); display: inline-grid; place-items: center; width: 22px; height: 22px; padding: 0; }
.blockeditor-table tr:hover .blockeditor-table-row-menu, .blockeditor-table tr:focus-within .blockeditor-table-row-menu, .blockeditor-table-row-menu:focus-visible { opacity: 1; }
/* The tags float to the end of the item's first line, so long text wraps beside and
   below them without the text's own marks being pulled apart. The item's whole line
   stays the click target for an empty item, which has no text of its own. */
.blockeditor-li.blockeditor-check-placeholder::after { content: attr(data-placeholder); position: absolute; inset-inline-start: calc((var(--li-indent) + 1) * var(--blockeditor-indent)); color: var(--blockeditor-ink-faint); pointer-events: none; }
.blockeditor-check-controls { display: inline-flex; float: inline-end; flex-wrap: wrap; align-items: center; gap: 5px; margin-inline-start: 10px; text-decoration: none; }
.blockeditor-check-chip { display: inline-flex; align-items: center; gap: 4px; box-sizing: border-box; height: 23px; padding: 0 7px; border: 1px solid var(--blockeditor-rule); border-radius: var(--blockeditor-check-radius); background: transparent; color: var(--blockeditor-ink-soft); font: 400 12px/1 var(--blockeditor-font); white-space: nowrap; cursor: pointer; }
.blockeditor-check-chip svg { flex: none; }
.blockeditor-check-chip:hover { background: var(--blockeditor-pop-hover); }
.blockeditor-check-chip[data-filled="true"] { border-color: transparent; background: var(--blockeditor-chrome-bg-moved); color: var(--blockeditor-ink); font-weight: 500; }
.blockeditor-check-chip[data-overdue="true"] { border-color: var(--blockeditor-danger); color: var(--blockeditor-danger); }
.blockeditor-li[data-checked="true"] .blockeditor-check-controls { opacity: 0.62; }
[data-assignees="false"] .blockeditor-check-chip[data-check="owner"], [data-dates="false"] .blockeditor-check-chip[data-check="date"] { display: none; }
[data-assignees="false"][data-dates="false"] .blockeditor-check-controls { display: none; }
.blockeditor-check-popup { display: inline-flex; gap: 4px; padding: 4px; background: var(--blockeditor-pop-bg); color: var(--blockeditor-pop-ink); border: 1px solid var(--blockeditor-pop-line); border-radius: var(--blockeditor-pop-radius); box-shadow: var(--blockeditor-pop-shadow); }
.blockeditor-check-popup input { font: inherit; color: inherit; background: var(--blockeditor-pop-bg); border: 1px solid var(--blockeditor-pop-line); }
.blockeditor-check-popup.blockeditor-check-owner, .blockeditor-check-popup.blockeditor-check-date { position: absolute; z-index: 5; display: block; box-sizing: border-box; width: min(264px, calc(100vw - 28px)); padding: 8px; background: var(--blockeditor-pop-bg); color: var(--blockeditor-pop-ink); border: 1px solid var(--blockeditor-pop-line); border-radius: var(--blockeditor-pop-radius); box-shadow: var(--blockeditor-pop-shadow); font-family: var(--blockeditor-font); }
.blockeditor-check-search { box-sizing: border-box; width: 100%; height: 28px; padding: 0 8px; border: 1px solid var(--blockeditor-pop-line); border-radius: 7px; background: var(--blockeditor-pop-bg); color: var(--blockeditor-pop-ink); font: inherit; font-size: 12.5px; }
.blockeditor-check-search::placeholder { color: var(--blockeditor-ink-faint); }
.blockeditor-check-subhead { padding: 7px 4px 4px; color: var(--blockeditor-pop-muted); font-size: 11.5px; font-weight: 500; }
.blockeditor-check-people { display: flex; flex-direction: column; gap: 1px; }
.blockeditor-check-person { display: flex; align-items: center; gap: 7px; width: 100%; padding: 5px 6px; border: 0; background: none; color: var(--blockeditor-pop-ink); font: inherit; font-size: 12.5px; text-align: start; cursor: pointer; }
.blockeditor-check-person:hover { background: var(--blockeditor-pop-hover); }
.blockeditor-check-person[aria-pressed="true"] { font-weight: 500; }
.blockeditor-check-person .blockeditor-board-face { flex: 0 0 18px; width: 18px; height: 18px; }
.blockeditor-check-person-name { min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.blockeditor-check-person-meta { margin-inline-start: auto; color: var(--blockeditor-pop-muted); font-size: 11px; }
.blockeditor-check-person svg { flex: none; }
.blockeditor-check-divider { height: 1px; margin: 6px 0; background: var(--blockeditor-pop-line); }
.blockeditor-check-danger { color: var(--blockeditor-danger); }
.blockeditor-check-danger:hover { background: var(--blockeditor-pop-hover); }
.blockeditor-check-danger[disabled] { opacity: 0.45; background: none; cursor: default; }
.blockeditor-check-quick { display: flex; flex-direction: column; gap: 1px; }
.blockeditor-check-quick-row { display: flex; align-items: center; gap: 7px; width: 100%; padding: 5px 6px; border: 0; background: none; color: var(--blockeditor-pop-ink); font: inherit; font-size: 12.5px; text-align: start; cursor: pointer; }
.blockeditor-check-quick-row:hover { background: var(--blockeditor-pop-hover); }
.blockeditor-check-quick-row svg { flex: none; }
.blockeditor-check-quick-label { min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.blockeditor-check-quick-meta { margin-inline-start: auto; color: var(--blockeditor-pop-muted); font-family: var(--blockeditor-font-mono); font-size: 11px; font-variant-numeric: tabular-nums; }
.blockeditor-check-cal { margin-top: 6px; padding-top: 6px; border-top: 1px solid var(--blockeditor-pop-line); }
.blockeditor-check-cal-head { display: flex; align-items: center; justify-content: space-between; margin-bottom: 5px; padding: 0 2px; }
.blockeditor-check-cal-month { color: var(--blockeditor-pop-ink); font-size: 12px; font-weight: 600; }
.blockeditor-check-cal-year { font-family: var(--blockeditor-font-mono); font-size: 11.5px; font-variant-numeric: tabular-nums; }
.blockeditor-check-cal-nav { display: inline-flex; gap: 3px; }
.blockeditor-check-cal-btn { display: inline-grid; place-content: center; width: 22px; height: 22px; padding: 0; border: 1px solid var(--blockeditor-pop-line); border-radius: var(--blockeditor-check-radius); background: none; color: var(--blockeditor-pop-ink); cursor: pointer; }
.blockeditor-check-cal-btn:hover { background: var(--blockeditor-pop-hover); }
.blockeditor-check-cal-grid { display: grid; grid-template-columns: repeat(7, 1fr); gap: 2px; text-align: center; }
.blockeditor-check-cal-dow { padding: 2px 0; color: var(--blockeditor-pop-muted); font-size: 10.5px; font-weight: 500; }
.blockeditor-check-cal-day { height: 28px; padding: 0; border: 1px solid transparent; border-radius: var(--blockeditor-check-radius); background: transparent; color: var(--blockeditor-pop-ink); font-family: var(--blockeditor-font-mono); font-size: 11.5px; font-variant-numeric: tabular-nums; cursor: pointer; }
.blockeditor-check-cal-day:hover { background: var(--blockeditor-pop-hover); }
.blockeditor-check-cal-day[data-outside="true"] { opacity: 0.5; }
.blockeditor-check-cal-day[data-today="true"] { border-color: var(--blockeditor-accent); font-weight: 500; }
.blockeditor-check-cal-day[aria-pressed="true"] { background: var(--blockeditor-chrome-bg-moved); font-weight: 500; }
.blockeditor-board-owners { display: flex; flex-direction: column; gap: 6px; min-width: 0; padding-inline: 8px; }
.blockeditor-board-people { display: flex; flex-wrap: wrap; gap: 4px; }
.blockeditor-board-person { display: inline-flex; align-items: center; gap: 6px; height: 28px; padding: 0 10px 0 4px; border: 1px solid var(--blockeditor-rule); border-radius: 14px; background: var(--blockeditor-pop-bg); color: var(--blockeditor-pop-ink); font: inherit; font-size: 13px; cursor: pointer; transition: background-color var(--blockeditor-fade), border-color var(--blockeditor-fade); }
.blockeditor-board-person.is-none { padding-inline-start: 10px; color: var(--blockeditor-pop-muted); }
.blockeditor-board-person:hover { background: var(--blockeditor-pop-hover); }
.blockeditor-board-person[aria-pressed="true"] { border-color: var(--blockeditor-accent); background: var(--blockeditor-selection); color: var(--blockeditor-pop-ink); }
.blockeditor-board-owner-input { width: 100%; height: 32px; box-sizing: border-box; padding: 0 8px; border: 1px solid var(--blockeditor-pop-line); border-radius: 7px; background: var(--blockeditor-pop-bg); color: var(--blockeditor-pop-ink); font: inherit; font-size: 14px; }
.blockeditor-board-owner-input::placeholder { color: var(--blockeditor-ink-faint); }
.blockeditor-board-face { flex: none; display: inline-grid; place-items: center; width: 20px; height: 20px; border-radius: 50%; background: var(--blockeditor-code-bg); box-shadow: inset 0 0 0 1px var(--blockeditor-rule); color: var(--blockeditor-ink-soft); font-size: 10.5px; font-weight: 600; line-height: 1; }
.blockeditor-scrim { display: none; }
.blockeditor-shell [hidden] { display: none !important; }
@media (max-width: 560px) {
  /* One fixed bar over the visual viewport, with the four controls a phone keeps.
     The bubble and the desktop-only tools stand down so nothing is doubled. */
  .blockeditor-dock { display: flex; position: fixed; left: 0; top: 8px; width: 204px; min-width: 204px; box-sizing: border-box; align-items: center; justify-content: center; gap: 4px; margin: 0; padding: 8px; z-index: 3; }
  .blockeditor-tool-desk { display: none; }
  .blockeditor-dock .blockeditor-tool-more { display: inline-flex; }
  .blockeditor-dock .blockeditor-tool { min-width: 44px; height: 44px; }
  .blockeditor-shell:has(.blockeditor-bubble:not([hidden])) .blockeditor-dock { visibility: visible; }
  .blockeditor-dock + .blockeditor-editor { padding-bottom: calc(72px + env(safe-area-inset-bottom)); }
  .blockeditor-plus { position: fixed; top: auto; left: 16px; bottom: calc(16px + env(safe-area-inset-bottom)); width: 44px; height: 44px; border-radius: 22px; }
  .blockeditor-grip { position: fixed; top: auto; left: 68px; bottom: calc(16px + var(--blockeditor-grip-lift, 0px) + env(safe-area-inset-bottom)); width: 44px; height: 44px; border-radius: 22px; opacity: 1; color: var(--blockeditor-chrome-ink-rest); background: var(--blockeditor-chrome-bg-rest); border: 1px solid var(--blockeditor-chrome-line-rest); }
  .blockeditor-bubble { display: none; }
  .blockeditor-scrim { display: block; position: fixed; inset: 0; z-index: 4; background: var(--blockeditor-scrim); }
  .blockeditor-menu { position: fixed; left: 0; top: 0; width: var(--blockeditor-pop-width, min(280px, calc(100vw - 16px))); max-height: min(70vh, var(--blockeditor-pop-max, 70vh)); overflow-y: auto; transform: translate(var(--blockeditor-pop-x, 0px), var(--blockeditor-pop-y, 0px)); z-index: 5; }
  .blockeditor-settings { position: fixed; left: 0; top: 0; transform: translate(var(--blockeditor-pop-x, 0px), var(--blockeditor-pop-y, 0px)); z-index: 5; }
  .blockeditor-menu-item { min-height: 48px; font-size: 16px; }
  .blockeditor-link { position: fixed; left: 0; right: 0; top: auto; bottom: 0; max-width: none; padding: 8px 16px calc(8px + env(safe-area-inset-bottom)); border-radius: 0; border-width: 1px 0 0; }
  .blockeditor-link-input { min-width: 0; height: 44px; font-size: 16px; }
  .blockeditor-link-btn, .blockeditor-upload-btn { min-width: 44px; height: 44px; }
  .blockeditor-image-frame { width: 100% !important; }
  .blockeditor-image-handle { display: none; }
  .blockeditor-table th button, .blockeditor-table-row-menu, .blockeditor-check-chip, .blockeditor-check-popup button, .blockeditor-menu-item { min-height: 44px; }
  .blockeditor-check-person { min-height: 44px; }
  .blockeditor-check-popup .blockeditor-check-quick-row { min-height: 44px; }
  .blockeditor-check-popup .blockeditor-check-cal-day { height: 40px; min-height: 40px; }
  .blockeditor-board-owner-input { font-size: 16px; height: 44px; }
  .blockeditor-check-search { font-size: 16px; height: 44px; }
  .blockeditor-check-popup .blockeditor-check-cal-btn { width: 44px; height: 44px; }
  .blockeditor-board-person { height: 44px; border-radius: 22px; }
}
@media (prefers-reduced-motion: reduce) {
  .blockeditor-dock, .blockeditor-plus, .blockeditor-grip, .blockeditor-toggle-button, .blockeditor-image-handle, .blockeditor-upload-bar > span, .blockeditor-board-person { transition: none; }
}
}
`,Zo=(o,e)=>(e&&e(o.tr.replaceSelectionWith(g.nodes.hard_break.create()).scrollIntoView()),!0),oe=(o,e,r)=>!e||!r?.editable||!Y(o,g.marks.underline)?!1:(import("../c-3S3647J6.js").then(({formatMark:t,refuse:i})=>t(r,"underline").then(d=>{d?.status!=="applied"&&i(r,d?.message??"That format isn\u2019t available here.")}),()=>null),!0),ee=({history:o=!0,undo:e,redo:r}={})=>({"Mod-z":o?S.undo:()=>(e?.(),!0),"Shift-Mod-z":o?S.redo:()=>(r?.(),!0),"Mod-y":o?S.redo:()=>(r?.(),!0),"Shift-Enter":Zo,"Mod-b":y.toggleMark(g.marks.strong),"Mod-i":y.toggleMark(g.marks.em),"Mod-u":oe}),re=(o,e,r)=>bo(null,r)?!1:X.undoInputRule(o,e&&(t=>e(Co(t)))),te={Backspace:y.chainCommands(re,so),"Mod-e":y.toggleMark(g.marks.code),"Mod-Shift-s":y.toggleMark(g.marks.strike)},ie=(o,e)=>globalThis.clay?.upload?.(o,e)??Promise.resolve({ok:!1,code:"unavailable",msg:"File uploads are not available in this editor"}),Io=new WeakMap,O=new h.PluginKey("lazyViews"),H=class{constructor(e,r){this.node=e,this.type=e.type,this.registry=r;let{dom:t,contentDOM:i}=U.DOMSerializer.renderSpec(document,e.type.spec.toDOM(e));this.dom=t,this.contentDOM=i??null}update(e){return e.type===this.type&&!this.registry.has(e.type.name)}};function de(){return new h.Plugin({key:O,state:{init:()=>m.DecorationSet.empty,apply(o){let e=o.getMeta(O);if(!e)return m.DecorationSet.empty;let r=[];return o.doc.forEach((t,i)=>{e.types.includes(t.type.name)&&r.push(m.Decoration.node(i,i+t.nodeSize,{},{lazyView:r.length+1}))}),m.DecorationSet.create(o.doc,r)}},props:{decorations:o=>O.getState(o)}})}function le({view:o,upload:e,toolHost:r,board:t=null,undo:i=null,redo:d=null,revise:c=null,highlight:b},s,u){let x=u.createFeature({view:o,upload:e,toolHost:r,board:t,undo:i,redo:d,revise:c,highlight:b}),z=Io.get(o),E=()=>{if(x.css&&L(`blockeditor-feature-${s}`,x.css,`blockeditor-feature-${s}`,["editor-ui"]),!x.nodeViews)return;let P=Object.keys(x.nodeViews);for(let v of P)z.set(v,x.nodeViews[v]);o.dispatch(o.state.tr.setMeta(O,{types:P}).setMeta("addToHistory",!1))},A=()=>{o.isDestroyed||(o.composing?setTimeout(A,25):E())};return o.composing?o.dom.addEventListener("compositionend",()=>setTimeout(A,25),{once:!0}):E(),x}function ce(o){return new h.Plugin({view(e){let r=new Set,t=()=>{for(let d of r)o.has(d)&&r.delete(d);e.state.doc.forEach(d=>{let c=Do(d);!c||o.has(c)||r.has(c)||o.ensure(c).catch(()=>{r.add(c)})})},i=e.state.doc;return t(),{update(){e.state.doc!==i&&(i=e.state.doc,t())}}}})}function ae({load:o=()=>Promise.reject(new Error("page: this editor has no command module")),context:e=()=>({}),reportError:r}={}){let t=new Map,i=new Set,d=null,c=null,b=!0,s=!1,u=0,x=a=>{let n=t.get(a);return n||(n=Promise.resolve(o(a)),t.set(a,n),n.catch(()=>{t.get(a)===n&&t.delete(a)})),n},z=()=>(c??=R({load:x,install:(a,n)=>n.createFeature(e(d)),reportError:r??((a,n)=>console.error(`clayjs: the ${n} command failed to load`,a))}),c),E=()=>{let a=u,n=d.state.selection;return{revision:a,selection:n,valid:()=>b&&!s&&!d.isDestroyed&&u===a&&d.state.selection.eq(n)}};function A(a,n){a.doc!==n.doc&&(u+=1);for(let p of i)p.lease.valid()||(i.delete(p),p.controller.abort({ok:!1,code:"stale-target"}))}async function P(a,n){if(!b||s||!d||d.isDestroyed)return{ok:!1,code:"not-editing"};let p={lease:E(),controller:new AbortController};i.add(p);try{let w=await z().ensure(a,{signal:p.controller.signal});return w?p.lease.valid()?await w.run(n):{ok:!1,code:"stale-target"}:{ok:!1,code:"not-editing"}}catch(w){return p.controller.signal.aborted?p.controller.signal.reason:{ok:!1,code:"unavailable",error:w}}finally{i.delete(p)}}function v(){if(b){b=!1;for(let a of i)a.controller.abort({ok:!1,code:"not-editing"});i.clear()}}function D(){v(),c?.dispose(),c=null}return{attach(a){d=a},observe:A,run:P,enter(){s||(b=!0)},quiesce:v,leave:D,dispose(){D(),s=!0,b=!1,t.clear()}}}var _=new WeakMap;function ne(o){return new h.Plugin({view:()=>({update:(e,r)=>o.observe(e.state,r)}),props:{handleTextInput(e,r,t,i){return i!=="@"||e.composing||!Object.hasOwn(T,"mentions")?!1:(e.dispatch(e.state.tr.insertText("@",r,t)),o.run("mentions",{from:r,to:r+1}),!0)}}})}function Ne(o,e,r){let t=o?_.get(o):null;return t?Object.hasOwn(T,e)?t.run(e,r):Promise.resolve({ok:!1,code:"unknown-command"}):Promise.resolve({ok:!1,code:"not-editing"})}function be(o){return new h.Plugin({view(e){let r=o?()=>o.tablesChanged():()=>mo.get(e)?.tablesChanged?.(),t=e.state.doc;return{update(i){i.state.doc!==t&&(t=i.state.doc,r())}}}})}function pe({upload:o=ie,highlight:e,history:r=!0,undo:t,redo:i,onAiTool:d=null,mediaNote:c=null,toolHost:b=null,features:s=null,commandHost:u=null}={}){return[to(),wo({onAiTool:d,mediaNote:c}),u?ne(u):null,vo(),be(b),ro(),uo(),Bo(),po(),ko(),Fo(),ho(),M.keymap(te),So,To,M.keymap(Po),M.keymap(ee({history:r,undo:t,redo:i})),M.keymap(y.baseKeymap),yo(),eo(),Mo(),no(),lo({upload:o}),Eo(),io({upload:o,features:s}),go(),Ao(),fo(),xo(),r?S.history():null,G.gapCursor(),de(),s?ce(s):null].filter(Boolean)}function ke(o,e={}){let r=o&&Object.keys(o).length?o:{[Q()]:{t:"paragraph",pos:J(),c:""}},t=e.doc??oo(g,r);return h.EditorState.create({doc:t,plugins:pe(e)})}function We(o){!o||o.isDestroyed||o.updateState(o.state)}function Ue(o,e,{quiesce:r=!1}={}){if(!o||o.isDestroyed)return!1;let t=e===!0;o.setProps({editable:()=>t});let i=_.get(o);return t?i?.enter():r?i?.quiesce():i?.leave(),t}function se(o,e){let r=e.target.closest?.('.blockeditor-li[data-kind="check"]');if(!r||!o.dom.contains(r))return!1;let t=r.getBoundingClientRect(),i=parseFloat(getComputedStyle(r).paddingInlineStart)||0,d=getComputedStyle(r).direction==="rtl"?t.right-e.clientX:e.clientX-t.left;if(d<0||d>i)return!1;let c=o.posAtDOM(r,0)-1,b=o.state.doc.nodeAt(c);return b?(e.preventDefault(),o.dispatch(o.state.tr.setNodeMarkup(c,null,{...b.attrs,checked:!b.attrs.checked})),!0):!1}function Ko(o,e){let r=parseFloat(o.defaultView?.getComputedStyle(o.documentElement).scrollPaddingTop);return Number.isFinite(r)?Math.max(e,r):e}function Xe(o,{blocks:e=null,onChange:r=null,upload:t,highlight:i,doc:d,dispatch:c,history:b=!0,undo:s,redo:u,editable:x=!0,toolHost:z=null,onAiTool:E=null,mediaNote:A=null,reviseTool:P=null,board:v=null,features:D=null,formatGate:a=null}={}){let n=o.ownerDocument;L(Oo,Qo,Oo,["editor-ui"]);let p=n.createElement("div");p.className="blockeditor-shell",p.setAttribute("clay","editor-ui"),p.setAttribute("sap-ignore",""),o.appendChild(p);let w=!1,k=null,j=D??R({load:l=>q[l].load(),install:(l,f)=>le({view:k,upload:t,toolHost:z,board:v,undo:s,redo:u,revise:P,highlight:i},l,f)}),F=ae({load:l=>T[l].load(),context:l=>({view:l,upload:t})}),K=new Map,N={};for(let l of Object.values(q))for(let f of l.views??l.types)N[f]=(B,Vo,qo,$o,Ho)=>{let W=K.get(B.type.name);return W?W(B,Vo,qo,$o,Ho):new H(B,K)};k=new m.EditorView(p,{state:ke(e,{upload:t,highlight:i,doc:d,history:b,undo:s,redo:u,onAiTool:E,mediaNote:A,toolHost:z,features:j,commandHost:F}),attributes:{class:"blockeditor-editor"},scrollMargin:{get top(){return Ko(n,16)},bottom:96,left:5,right:5},scrollThreshold:{get top(){return Ko(n,16)},bottom:96,left:0,right:0},editable:()=>x,nodeViews:N,handleDOMEvents:{mousedown:se,dragstart(l,f){let B=l.state.selection;return B instanceof h.NodeSelection||B instanceof zo?(f.preventDefault(),!0):!1}},dispatchTransaction(l){if(c){c(l);return}w&&l.docChanged&&!l.getMeta(V)&&S.closeHistory(l),l.docChanged&&(w=!!l.getMeta(V));let f=k.state.doc;k.updateState(k.state.apply(l)),r&&k.state.doc!==f&&r(Z(k.state.doc))}}),Io.set(k,K),ao.set(k,j),F.attach(k),_.set(k,F);let Lo=k.destroy.bind(k),C=null,I=!1,Ro=()=>(C||(C=Promise.resolve().then(()=>a()).catch(l=>{throw C=null,l})),C);return k.runFormatCommand=async l=>{if(I)return{status:"disposed",retry:!1};if(!a)return{status:"unsupported-host",code:"blockeditor-format-unsupported",retry:!1};let f=await Ro();return I?(f.dispose?.(),{status:"disposed",retry:!1}):f.run({apply:l})},k.destroy=()=>{I=!0,C&&C.then(l=>l?.dispose?.(),()=>{}),F.dispose(),j.dispose(),Lo()},k}export{Qo as PAGE_EDITOR_CSS,ke as blockeditorState,Xe as createPageEditor,le as installPageFeature,O as lazyViewsKey,de as lazyViewsPlugin,pe as pagePlugins,We as refreshMountedView,Ne as runPageCommand,Ue as setEditorActive};
