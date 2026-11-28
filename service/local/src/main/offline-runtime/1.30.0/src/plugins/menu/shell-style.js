export const MENU_SAFETY = `@layer clay-menu-safety {
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
`;

export const MENU_SURFACE = { light: [0xFF, 0xFC, 0xF6], dark: [0x1A, 0x1D, 0x2C], mutedLight: [0x6B, 0x59, 0x42], mutedDark: [0xA3, 0xA9, 0xC2] };

export const SHELL_A = `.clay-menu-dock, .clay-menu-spots, .clay-seo {
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

`;

export const SHELL_B = `.clay-menu-goo { position: relative; display: inline-grid; place-items: center; box-sizing: border-box; margin: 0; padding: 0; cursor: pointer; }
.clay-menu-goo svg { display: block; }
.clay-menu-dot { position: absolute; top: -1px; right: -2px; width: 8px; height: 8px; border-radius: 50%; background: var(--bevel-brass); box-shadow: 0 0 0 2px var(--bevel-surface); }

.clay-menu :focus-visible, .clay-roster:focus-visible { outline: 2px solid var(--bevel-brass); outline-offset: 2px; }
.clay-menu-edit:focus-visible { outline: none; }
.clay-menu-edit:focus-visible .clay-menu-track { outline: 2px solid var(--bevel-brass); outline-offset: 2px; }

`;

export const SHELL_C = `/* Popovers: the menu, the people panel, the status note. Below the dock, above it in a bottom corner. */
.clay-menu-list, .clay-people, .clay-menu-note { position: absolute; z-index: 1; top: calc(100% + 8px); right: 0; box-sizing: border-box; margin: 0; color: var(--bevel-ink); text-align: left; }
.clay-people, .clay-menu-note { border: 1px solid var(--bevel-line-2); background: var(--bevel-surface); box-shadow: var(--clay-shadow); }

.clay-menu-list { width: max-content; max-width: calc(100vw - 24px); max-height: calc(100dvh - 72px); overflow-y: auto; overscroll-behavior: contain; }
.clay-menu-list[popover] { position: fixed; inset: auto; margin: 0; }
.clay-menu-list [role^="menuitem"] { box-sizing: border-box; display: flex; align-items: center; justify-content: space-between; gap: 12px; width: 100%; margin: 0; border: 0; border-radius: var(--clay-menu-item-radius, 0px); background: none; text-align: left; text-decoration: none; cursor: pointer; }
.clay-menu-list [role^="menuitem"].clay-menu-nested { justify-content: flex-start; gap: 7px; width: calc(100% - 18px); min-height: 29px; margin: 2px 4px 4px 14px; padding: 5px 9px; border: 1px solid var(--clay-menu-hr, var(--bevel-line)); border-radius: max(3px, var(--clay-menu-item-radius, 0px)); background: color-mix(in srgb, var(--clay-menu-hover-bg, var(--bevel-sunk)) 58%, var(--clay-menu-bg, var(--bevel-surface))); color: var(--clay-menu-muted, var(--bevel-ink-2)); }
.clay-menu-list [role^="menuitem"].clay-menu-nested::after { content: "›"; margin-left: auto; font-size: 13px; line-height: 1; opacity: .55; }
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

`;

export const SHELL_BASE = SHELL_A + SHELL_B + SHELL_C;

export const SHELL_FLOATING = `/* The dock: the roster (when a page opts in) and the capsule, fixed in one corner. */
.clay-menu-dock { position: fixed; z-index: 2147482000; top: max(12px, env(safe-area-inset-top)); right: max(12px, env(safe-area-inset-right)); }
.clay-menu-dock[data-corner$="left"] { right: auto; left: max(12px, env(safe-area-inset-left)); flex-direction: row-reverse; }
.clay-menu-dock[data-corner^="bottom"] { top: auto; bottom: max(12px, env(safe-area-inset-bottom)); }
.clay-menu-dock[data-corner$="left"] :is(.clay-menu-list, .clay-people, .clay-menu-note) { right: auto; left: 0; }
.clay-menu-dock[data-corner^="bottom"] :is(.clay-menu-list, .clay-people, .clay-menu-note) { top: auto; bottom: calc(100% + 8px); }
`;

export const MENU_SKIN = `.clay-menu { height: 34px; padding: 0 4px 0 2px; border: 1px solid var(--clay-menu-border, var(--bevel-line-3)); border-radius: var(--clay-menu-button-radius, 0px); background: var(--clay-menu-bg, var(--bevel-surface)); box-shadow: var(--clay-shadow); }
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
`;

// Last, so no skin can shrink a touch target or bring motion back.
export const MENU_TOUCH = `@media (pointer: coarse), (max-width: 680px) {
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
`;
