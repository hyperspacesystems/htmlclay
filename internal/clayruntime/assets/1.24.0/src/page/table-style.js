export const TABLE_CSS = `
.blockeditor-editor .blockeditor-table { position: relative; }
.blockeditor-table td { position: relative; }
.blockeditor-table th { position: relative; }
.blockeditor-table td.is-cell-selected { background: var(--blockeditor-block-selected); }
.blockeditor-editor.has-cell-rect .blockeditor-table td ::selection, .blockeditor-editor.has-cell-rect .blockeditor-table td::selection { background: transparent; }
.blockeditor-editor.has-cell-rect .blockeditor-table td { caret-color: transparent; }
.blockeditor-table table.is-sized { table-layout: fixed; min-width: 0; }
.blockeditor-table table.is-sized th, .blockeditor-table table.is-sized td { min-width: 0; overflow-wrap: anywhere; }
.blockeditor-table-resize { position: absolute; top: 0; bottom: 0; inset-inline-end: -4px; width: 8px; z-index: 2; cursor: col-resize; touch-action: none; }
.blockeditor-table-resize:hover, .blockeditor-table-resize:active, .blockeditor-table-resize:focus-visible { background: var(--blockeditor-accent); opacity: 0.4; }
.blockeditor-table-add-row, .blockeditor-table-add-col { position: absolute; z-index: 1; padding: 0; background: var(--blockeditor-chrome-bg-moved); color: var(--blockeditor-ink-soft); display: flex; align-items: center; justify-content: center; font: inherit; font-size: 14px; line-height: 1; cursor: pointer; opacity: 0; transition: opacity 120ms; }
.blockeditor-table-add-row { left: 0; right: 0; top: 100%; height: 16px; border-radius: 0 0 6px 6px; border: 1px solid var(--blockeditor-chrome-line-moved); border-top: 0; padding-bottom: 2px; box-sizing: border-box; }
.blockeditor-table-add-col { top: 0; bottom: 0; left: 100%; width: 16px; border-radius: 0 6px 6px 0; border: 1px solid var(--blockeditor-chrome-line-moved); border-left: 0; box-sizing: border-box; }
.blockeditor-table:hover > .blockeditor-table-add-row, .blockeditor-table:hover > .blockeditor-table-add-col, .blockeditor-table-add-row:focus-visible, .blockeditor-table-add-col:focus-visible { opacity: 1; }
.blockeditor-table-add-row:hover, .blockeditor-table-add-col:hover { background: var(--blockeditor-pop-hover); color: var(--blockeditor-ink); }
.blockeditor-table-date { position: absolute; inset-inline-start: 0; top: 100%; z-index: 3; font: inherit; color: var(--blockeditor-pop-ink); background: var(--blockeditor-pop-bg); border: 1px solid var(--blockeditor-pop-line); border-radius: 6px; padding: 4px 6px; box-shadow: var(--blockeditor-pop-shadow); }
@media (max-width: 560px) {
  .blockeditor-table-date { min-height: 44px; }
  .blockeditor-table-resize { display: none; }
  .blockeditor-table-add-row, .blockeditor-table-add-col { position: static; display: inline-block; width: 50%; height: 44px; border-radius: 0; opacity: 1; border-block-start: 1px solid var(--blockeditor-rule); }
  .blockeditor-table-add-row::after { content: " Row"; }
  .blockeditor-table-add-col::after { content: " Column"; }
}
`;
