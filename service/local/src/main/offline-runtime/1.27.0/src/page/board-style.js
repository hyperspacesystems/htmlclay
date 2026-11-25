export const BOARD_CSS = `
:where(article[data-blockeditor], .blockeditor-shell) { --blockeditor-board-lane: 274px; --blockeditor-board-gap: 14px; --blockeditor-board-radius: 8px; --blockeditor-board-panel: 360px; }
.blockeditor-editor .blockeditor-board { margin-block: 1.1em 1.5em; font-size: 15px; line-height: 1.4; white-space: normal; }
:is(.blockeditor-board, .blockeditor-board-panel) .i { flex: none; width: 16px; height: 16px; }
.blockeditor-board-scroll { position: relative; margin: -4px -4px 0; padding: 4px 4px 12px; overflow-x: auto; overscroll-behavior-x: contain; scrollbar-width: thin; scrollbar-color: var(--blockeditor-chrome-ink-rest) transparent; }
.blockeditor-board-lanes { display: flex; align-items: flex-start; gap: var(--blockeditor-board-gap); width: max-content; }
.blockeditor-board-lane { flex: 0 0 var(--blockeditor-board-lane); width: var(--blockeditor-board-lane); min-width: 0; }
.blockeditor-board-head { position: relative; display: flex; align-items: flex-start; gap: 8px; padding: 8px 0 7px; margin-bottom: 10px; border-bottom: 1px solid var(--blockeditor-rule); }
.blockeditor-board-name { min-width: 0; margin: 0; overflow: hidden; display: -webkit-box; -webkit-box-orient: vertical; -webkit-line-clamp: 2; color: var(--blockeditor-ink-strong); font: 600 14px/20px var(--blockeditor-font-heading); overflow-wrap: anywhere; }
.blockeditor-board-count { flex: none; color: var(--blockeditor-ink-soft); font-size: 14px; line-height: 20px; font-variant-numeric: tabular-nums; }
.blockeditor-board-icon { flex: none; display: inline-grid; place-items: center; width: 28px; height: 28px; margin-block: -4px; padding: 0; border: 0; border-radius: 6px; background: none; color: var(--blockeditor-ink-faint); cursor: pointer; transition: background-color var(--blockeditor-fade), color var(--blockeditor-fade); }
.blockeditor-board-icon:hover, .blockeditor-board-icon[aria-expanded="true"] { background: var(--blockeditor-pop-hover); color: var(--blockeditor-ink-soft); }
.blockeditor-board-icon:focus-visible { color: var(--blockeditor-ink-soft); }
.blockeditor-board-colmenu { margin-inline: auto -4px; }
.blockeditor-board-cards { display: flex; flex-direction: column; gap: 8px; margin: 0; padding: 0; list-style: none; }
.blockeditor-board-cards > li { position: relative; }
.blockeditor-board-card { display: block; width: 100%; margin: 0; padding: 10px 12px 11px; border: 1px solid var(--blockeditor-rule); border-radius: var(--blockeditor-board-radius); background: var(--blockeditor-pop-bg); color: var(--blockeditor-ink); font: inherit; text-align: start; cursor: pointer; transition: background-color var(--blockeditor-fade), border-color var(--blockeditor-fade); }
.blockeditor-board-card:hover { background-color: color-mix(in srgb, var(--blockeditor-pop-bg) 96%, var(--blockeditor-ink)); }
.blockeditor-board-card[aria-expanded="true"] { border-color: var(--blockeditor-accent); box-shadow: inset 0 0 0 1px var(--blockeditor-accent); }
.blockeditor-board-title { display: -webkit-box; -webkit-box-orient: vertical; -webkit-line-clamp: 4; overflow: hidden; font-size: 15px; font-weight: 500; line-height: 21px; overflow-wrap: anywhere; }
.blockeditor-board-meta { display: flex; align-items: center; gap: 8px; margin-top: 8px; color: var(--blockeditor-ink-soft); font-size: 13px; line-height: 20px; }
.blockeditor-board-owner { display: inline-flex; align-items: center; gap: 6px; min-width: 0; }
.blockeditor-board-owner-name { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.blockeditor-board-due { flex: none; display: inline-flex; align-items: center; gap: 6px; margin-inline-start: auto; white-space: nowrap; font-variant-numeric: tabular-nums; }
.blockeditor-board-due[data-due="today"] { color: var(--blockeditor-ink); font-weight: 500; }
.blockeditor-board-due[data-due="overdue"] { color: var(--blockeditor-ink); }
.blockeditor-board-due[data-due="overdue"]::before { content: ""; flex: none; width: 6px; height: 6px; border-radius: 50%; background: var(--blockeditor-danger); }
.blockeditor-board-sr { position: absolute; width: 1px; height: 1px; overflow: hidden; clip-path: inset(50%); white-space: nowrap; }
.blockeditor-board-add { display: flex; align-items: center; gap: 6px; width: 100%; min-height: 36px; margin-top: 6px; padding: 0 12px; border: 0; border-radius: 7px; background: none; color: var(--blockeditor-ink-soft); font: inherit; font-size: 14px; text-align: start; cursor: pointer; transition: background-color var(--blockeditor-fade); }
.blockeditor-board-add .i { width: 14px; height: 14px; color: var(--blockeditor-ink-faint); }
.blockeditor-board-add:hover { background: var(--blockeditor-pop-hover); }
.blockeditor-board-addcol { flex: none; display: inline-grid; place-items: center; width: 32px; height: 32px; margin-top: 2px; padding: 0; border: 1px solid var(--blockeditor-chrome-line-rest); border-radius: var(--blockeditor-chrome-radius); background: var(--blockeditor-chrome-bg-rest); color: var(--blockeditor-ink-faint); cursor: pointer; transition: color var(--blockeditor-fade), background-color var(--blockeditor-fade), border-color var(--blockeditor-fade); }
.blockeditor-board-addcol:hover { border-color: var(--blockeditor-chrome-line-moved); background: var(--blockeditor-chrome-bg-moved); color: var(--blockeditor-ink-soft); }
.blockeditor-board-addcol .i { width: 14px; height: 14px; }
.blockeditor-board-tray { margin-bottom: 18px; padding: 6px 12px 12px; border-radius: 10px; background: var(--blockeditor-callout-bg); }
.blockeditor-board-tray-head { display: flex; align-items: center; gap: 8px; min-height: 44px; margin-bottom: 4px; }
.blockeditor-board-tray .blockeditor-board-cards { display: grid; grid-template-columns: repeat(auto-fill, minmax(220px, 1fr)); gap: 8px; }
.blockeditor-board-select { position: relative; display: block; min-width: 0; }
.blockeditor-board-select > .i { position: absolute; inset-inline-end: 8px; top: 50%; margin-top: -8px; color: var(--blockeditor-ink-faint); pointer-events: none; }
.blockeditor-board-select select { appearance: none; -webkit-appearance: none; padding-inline-end: 32px; text-overflow: ellipsis; }
.blockeditor-board-move { margin-top: 6px; }
.blockeditor-board-move select { width: 100%; min-height: 32px; padding-inline-start: 10px; border: 1px solid var(--blockeditor-pop-line); border-radius: 7px; background: var(--blockeditor-pop-bg); color: var(--blockeditor-ink); font: inherit; font-size: 14px; cursor: pointer; }
.blockeditor-board-btn { display: inline-flex; align-items: center; justify-content: center; gap: 6px; min-height: 32px; padding: 0 12px 0 10px; border: 1px solid var(--blockeditor-pop-line); border-radius: 7px; background: none; color: var(--blockeditor-pop-ink); font: inherit; font-size: 14px; cursor: pointer; transition: background-color var(--blockeditor-fade), color var(--blockeditor-fade); }
.blockeditor-board-btn:hover { background: var(--blockeditor-pop-hover); }
.blockeditor-board-btn .i { width: 14px; height: 14px; color: var(--blockeditor-pop-muted); }
.blockeditor-board-btn:disabled { background: none; color: var(--blockeditor-ink-faint); cursor: default; }
.blockeditor-board-btn:disabled .i { color: var(--blockeditor-ink-faint); }
.blockeditor-board-restore { flex: none; margin-inline-start: auto; }
.blockeditor-board-deleted { background: none; padding: 0; }
.blockeditor-board-deleted-toggle[aria-expanded="false"] svg { transform: rotate(-90deg); }
.blockeditor-board-deleted-list { list-style: none; margin: 0; padding: 0 0 0 12px; }
.blockeditor-board-deleted-item { display: flex; align-items: center; gap: 8px; min-height: 44px; color: var(--blockeditor-ink-soft); }
.blockeditor-board :is(button, select, input, textarea):focus-visible, .blockeditor-board-panel :is(button, select, input, textarea):focus-visible { outline: 2px solid var(--blockeditor-accent); outline-offset: 2px; }
.blockeditor-board-pop { position: absolute; z-index: 6; padding: 6px; border: 1px solid var(--blockeditor-pop-line); border-radius: var(--blockeditor-pop-radius); background: var(--blockeditor-pop-bg); box-shadow: var(--blockeditor-pop-shadow); color: var(--blockeditor-pop-ink); font-size: 14px; line-height: 1.4; }
.blockeditor-board-menu { top: calc(100% - 2px); inset-inline-end: -6px; width: 208px; }
.blockeditor-board-option { display: flex; align-items: center; gap: 10px; width: 100%; min-height: 34px; padding: 4px 10px; border: 0; border-radius: 7px; background: none; color: inherit; font: inherit; text-align: start; cursor: pointer; }
.blockeditor-board-option:hover, .blockeditor-board-option[aria-selected="true"], .blockeditor-board-option[aria-pressed="true"] { background: var(--blockeditor-pop-hover); }
.blockeditor-board-option > .i { color: var(--blockeditor-pop-muted); }
.blockeditor-board-option.is-danger { color: var(--blockeditor-danger); }
.blockeditor-board-option.is-danger > .i { color: inherit; }
.blockeditor-board-option:disabled { color: var(--blockeditor-ink-faint); cursor: default; background: none; }
.blockeditor-board-option:disabled > .i { color: var(--blockeditor-ink-faint); }
.blockeditor-board-sep { height: 1px; margin: 4px 6px; background: var(--blockeditor-pop-line); }
.blockeditor-board-rename { flex: 1 1 auto; min-width: 0; height: 28px; margin: -4px 0 -4px -6px; padding: 0 6px; border: 0; border-radius: 6px; background: var(--blockeditor-pop-hover); color: var(--blockeditor-ink-strong); font: 600 14px/20px var(--blockeditor-font-heading); }
.blockeditor-board-new { display: block; width: 100%; min-height: 44px; margin-top: 8px; padding: 10px 12px; border: 1px solid var(--blockeditor-rule); border-radius: var(--blockeditor-board-radius); background: var(--blockeditor-pop-bg); color: var(--blockeditor-ink); font: 500 15px/21px var(--blockeditor-font); }
.blockeditor-board-new::placeholder, .blockeditor-board-rename::placeholder { color: var(--blockeditor-ink-faint); font-weight: 400; }
.blockeditor-board-card, .blockeditor-board-name, .blockeditor-board-count { -webkit-user-select: none; user-select: none; }
.blockeditor-editor[contenteditable="true"] .blockeditor-board-head[data-col] .blockeditor-board-name { cursor: grab; }
.blockeditor-board.is-dragging, .blockeditor-board.is-dragging * { cursor: grabbing; }
.blockeditor-board .is-drag-source { opacity: 0.45; }
.blockeditor-board-drop { height: 2px; margin-block: -5px; border-radius: 1px; background: var(--blockeditor-drop, var(--blockeditor-accent)); pointer-events: none; }
.blockeditor-board-drop.is-column { flex: 0 0 2px; align-self: stretch; width: 2px; height: auto; margin-block: 0; margin-inline: calc((var(--blockeditor-board-gap) + 2px) / -2); }
.blockeditor-board-ghost { position: fixed; z-index: 6; top: 0; left: 0; box-sizing: border-box; margin: 0; box-shadow: var(--blockeditor-pop-shadow); pointer-events: none; }
.blockeditor-board-lane.blockeditor-board-ghost { padding: 0 10px 10px; border-radius: var(--blockeditor-board-radius); background: var(--blockeditor-bg); }
.blockeditor-board-scrim { display: none; position: fixed; inset: 0; z-index: 4; background: var(--blockeditor-scrim); }
.blockeditor-board-panel { position: fixed; z-index: 5; inset-block: 0; inset-inline-end: 0; width: var(--blockeditor-board-panel); box-sizing: border-box; overflow-y: auto; padding: 18px 20px 24px; border-inline-start: 1px solid var(--blockeditor-pop-line); background: var(--blockeditor-pop-bg); color: var(--blockeditor-pop-ink); font-family: var(--blockeditor-font); font-size: 15px; line-height: 1.4; }
.blockeditor-shell.has-board-panel { padding-inline-end: calc(var(--blockeditor-board-panel) + 16px); }
.blockeditor-board-panel-top { display: flex; align-items: flex-start; gap: 8px; }
.blockeditor-board-panel-title { flex: 1 1 0; min-width: 0; margin: -4px 0 0 -8px; padding: 4px 8px; border: 0; border-radius: 7px; background: none; color: var(--blockeditor-ink-strong); font: 600 20px/28px var(--blockeditor-font-heading); field-sizing: content; resize: none; overflow: hidden; }
.blockeditor-board-panel-title:hover { background: var(--blockeditor-pop-hover); }
.blockeditor-board-close { width: 32px; height: 32px; margin: -2px -6px 0 0; color: var(--blockeditor-ink-soft); }
.blockeditor-board-fields { margin: 18px 0 0; border-top: 1px solid var(--blockeditor-pop-line); }
.blockeditor-board-field { position: relative; display: grid; grid-template-columns: 84px minmax(0, 1fr); align-items: center; column-gap: 8px; min-height: 44px; border-bottom: 1px solid var(--blockeditor-pop-line); }
.blockeditor-board-field.is-owner { align-items: start; padding-block: 8px; }
.blockeditor-board-field.is-owner .blockeditor-board-label { padding-top: 4px; }
.blockeditor-board-label { color: var(--blockeditor-pop-muted); font-size: 14px; }
.blockeditor-board-control { display: flex; align-items: center; gap: 8px; width: 100%; min-width: 0; min-height: 34px; margin: 0; padding: 0 8px; border: 0; border-radius: 7px; background: none; color: var(--blockeditor-pop-ink); font: inherit; font-size: 15px; text-align: start; cursor: pointer; transition: background-color var(--blockeditor-fade); }
.blockeditor-board-control:hover { background: var(--blockeditor-pop-hover); }
.blockeditor-board-control > .i:last-child { margin-inline-start: auto; color: var(--blockeditor-ink-faint); }
.blockeditor-board-control-text { min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.blockeditor-board-none { color: var(--blockeditor-ink-faint); }
.blockeditor-board-control .blockeditor-board-due { margin: 0; color: var(--blockeditor-pop-ink); font-weight: 400; }
.blockeditor-board-due-note { color: var(--blockeditor-pop-muted); font-size: 13px; }
.blockeditor-board-control .blockeditor-board-due + .blockeditor-board-due-note { margin-inline-start: auto; }
.blockeditor-board-control .blockeditor-board-due-note + .i { margin-inline-start: 0; }
.blockeditor-board-date { border: 1px solid var(--blockeditor-pop-line); cursor: text; }
.blockeditor-board-actions { display: flex; flex-wrap: wrap; align-items: center; gap: 8px; margin-top: 20px; }
.blockeditor-board-delete { margin-inline-start: auto; border-color: transparent; color: var(--blockeditor-pop-muted); }
.blockeditor-board-delete:hover { background: var(--blockeditor-pop-hover); color: var(--blockeditor-danger); }
.blockeditor-board-delete:hover .i { color: inherit; }
@media (max-width: 1039.98px) {
  .blockeditor-shell.has-board-panel { padding-inline-end: 16px; }
  .blockeditor-board-scrim { display: block; }
  .blockeditor-board-panel { inset: auto 0 0 0; width: auto; max-width: 640px; max-height: 85%; margin-inline: auto; padding: 14px 16px calc(16px + env(safe-area-inset-bottom)); border: 1px solid var(--blockeditor-pop-line); border-bottom: 0; border-radius: 16px 16px 0 0; box-shadow: var(--blockeditor-pop-shadow); }
  .blockeditor-board-close { width: 44px; height: 44px; margin: -8px -10px 0 0; }
  .blockeditor-board-panel-top { min-height: 36px; }
  .blockeditor-board-field { min-height: 48px; }
  .blockeditor-board-control { min-height: 44px; }
  .blockeditor-board-actions { margin-top: 16px; }
  .blockeditor-board-actions .blockeditor-board-btn { flex: 1 1 0; min-height: 44px; }
  .blockeditor-board-actions .blockeditor-board-delete { flex: 1 1 100%; margin-inline-start: 0; }
}
@media (max-width: 560px) {
  .blockeditor-board-scroll { scroll-snap-type: x proximity; scroll-padding-inline: 4px; }
  .blockeditor-board-lane { scroll-snap-align: start; }
  .blockeditor-board-head { padding: 12px 0 11px; }
  .blockeditor-board-head .blockeditor-board-icon { width: 44px; height: 44px; margin-block: -12px; }
  .blockeditor-board-colmenu { margin-inline-end: -12px; }
  .blockeditor-board-card { min-height: 44px; padding: 12px 14px 13px; }
  .blockeditor-board-add { min-height: 44px; padding-inline: 14px; }
  .blockeditor-board-addcol { width: 44px; height: 44px; margin-top: 0; }
  .blockeditor-board-tray-head { min-height: 48px; }
  .blockeditor-board-move select, .blockeditor-board-btn { min-height: 44px; }
  .blockeditor-board-move select, .blockeditor-board-new { font-size: 16px; }
  .blockeditor-board-option { min-height: 44px; }
  .blockeditor-board-rename { height: 44px; margin-block: -12px; font-size: 16px; }
  .blockeditor-board-panel { border-width: 1px 0 0; }
  .blockeditor-board-panel-title { margin-top: -8px; padding-block: 8px; }
  .blockeditor-board-control { font-size: 16px; }
  .blockeditor-board-label { font-size: 15px; }
}
@media (prefers-reduced-motion: reduce) {
  .blockeditor-board-icon, .blockeditor-board-card, .blockeditor-board-add, .blockeditor-board-addcol, .blockeditor-board-btn, .blockeditor-board-control { transition: none; }
}
`;
