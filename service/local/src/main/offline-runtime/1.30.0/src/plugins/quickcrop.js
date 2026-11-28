// clay.quickcrop: the vendored cropper, framed by ClayJS's own Bevel dialog.
//
// quickcrop draws the crop stage; how it is framed is an adapter's job (its
// `modal` option). Left to itself it would wrap themodal by reaching for themodal's
// classes, which ClayJS's modal no longer has, or fall back to a frame of its own
// that a page stylesheet can repaint. So every request that would have landed on
// either ('auto', or a themodal-shaped object such as clay.modal, which is what the
// CMS passes) is framed here instead. 'builtin' and a caller's own adapter pass
// through untouched. The stage itself is quickcrop's, and so is its geometry; its
// skin keeps a page stylesheet off it.
import vendorQuickcrop from "../vendor/quickcrop.vendor.js";
import { bevelDialog, dismissOf, keepFocusIn } from "../ui/bevel-dialog.js";
import { bevelButton } from "../ui/bevel-controls.js";
import { installSkin } from "../ui/vendor-skin.js";
import { CSS as SKIN, ROOTS as SKIN_ROOTS } from "../ui/skins/quickcrop.js";
import { resolveTheme } from "../ui/theme.js";
import { themed, partRules, markParts } from "../ui/theme-parts.js";
import { style } from "../lib/hostile-css.js";

const WIDTH = 844;

installSkin("quickcrop", SKIN, { roots: SKIN_ROOTS });

function isThemodal(m) {
  return !!m && typeof m === "object" &&
    typeof m.open === "function" && typeof m.close === "function" &&
    typeof m.onYes === "function" && "html" in m;
}

const TITLE = "Crop image";

// The frame for one crop, in the theme captured for that crop. Measuring and opening
// build the same frame, so the stage is sized for the padding, borders, heading and
// button the person will actually see.
function cropFrame(theme, confirmLabel, onConfirm = null) {
  const frame = bevelDialog({ zIndex: "2147483001", width: `${WIDTH}px`, closable: true, titled: true, theme, surface: "crop" });
  frame.heading.textContent = TITLE;
  frame.panel.setAttribute("aria-label", TITLE);
  const confirm = bevelButton(confirmLabel, {
    variant: "primary", onClick: onConfirm, theme,
    parts: ["dialog.button", "crop.button"], labelParts: ["dialog.buttonLabel", "crop.buttonLabel"],
  });
  frame.footer.append(confirm);
  return { ...frame, confirm };
}

// The stage is quickcrop's; its parts take only a page's explicit crop.* declarations,
// so dimming and crop lines keep their contrast unless the page asks otherwise.
function themeStage(stage, theme) {
  if (!themed(theme)) return;
  const paint = (el, names) => {
    if (!el) return;
    markParts(el, theme, names);
    style(el, partRules(theme, names, {}));
  };
  paint(stage, ["crop.stage"]);
  paint(stage.querySelector(":scope > img"), ["crop.image"]);
  paint(stage.querySelector(".qc-dim"), ["crop.dim"]);
  paint(stage.querySelector(".qc-box"), ["crop.selection"]);
  for (const corner of ["nw", "ne", "sw", "se"]) paint(stage.querySelector(`.qc-handle.qc-${corner}`), ["crop.handle", `crop.handle.${corner}`]);
}

// A themed frame is measured: a hidden copy is mounted for one layout read and removed,
// without touching focus or the scroll lock. The room left for the stage is the body's
// inner width, and the overlay's inner height less the empty panel. The exact Bevel
// frame keeps its fixed allowances.
function measure(theme, confirmLabel) {
  const { root, overlay, panel, body } = cropFrame(theme, confirmLabel);
  root.setAttribute("aria-hidden", "true");
  style(root, ["visibility:hidden", "pointer-events:none"]);
  document.body.append(root);
  try {
    const px = (value) => parseFloat(value) || 0;
    const o = getComputedStyle(overlay), b = getComputedStyle(body);
    const width = body.clientWidth - px(b.paddingLeft) - px(b.paddingRight);
    const height = overlay.clientHeight - px(o.paddingTop) - px(o.paddingBottom) - panel.offsetHeight;
    return { width: Math.max(1, Math.floor(width)), height: Math.max(1, Math.floor(height)) };
  } finally {
    root.remove();
  }
}

export function createCropAdapter({ theme = null, confirmLabel = "Crop" } = {}) {
  return {
    // Unthemed, the stage fits inside the panel by fixed allowances: the overlay's 16px
    // side inset, the panel's 1px border and the body's 22px padding a side; below it the
    // header, the footer and the overlay's top inset. Widths come from clientWidth, which
    // leaves out a classic scrollbar as the overlay does.
    fit() {
      if (themed(theme)) return measure(theme, confirmLabel);
      const width = document.documentElement.clientWidth;
      return {
        width: Math.max(120, Math.min(WIDTH, width - 32) - 2 - 44),
        height: Math.max(120, window.innerHeight - Math.min(96, window.innerHeight * 0.1) - 200),
      };
    },
    open({ content, confirmLabel: label, onConfirm, onCancel }) {
      const { root, overlay, panel, body, close, confirm } = cropFrame(theme, label, () => onConfirm());
      themeStage(content, theme);
      body.append(content);

      close.addEventListener("click", () => onCancel());
      panel.addEventListener("submit", (event) => event.preventDefault());
      let pressedOnBackdrop = false;
      overlay.addEventListener("mousedown", (event) => { pressedOnBackdrop = event.target === overlay; });
      overlay.addEventListener("click", (event) => {
        if (pressedOnBackdrop && event.target === overlay) onCancel();
        pressedOnBackdrop = false;
      });
      // Capture, and stopped there, so a dialog underneath (a CMS confirm through
      // clay.modal) does not take the same key as its own: Escape cancels this crop,
      // Tab stays inside it.
      const onKey = (event) => {
        if (event.key === "Tab") {
          event.stopPropagation();
          keepFocusIn(panel, event);
          return;
        }
        if (event.key !== "Escape") return;
        event.stopPropagation();
        onCancel();
      };
      document.addEventListener("keydown", onKey, true);

      // The page does not scroll behind the crop, as it did not under themodal. The
      // previous value comes back, so a modal underneath keeps its own lock.
      const overflow = document.body.style.overflow;
      document.body.style.overflow = "hidden";

      let done = false;
      const finish = () => {
        if (done) return;
        done = true;
        document.removeEventListener("keydown", onKey, true);
        document.body.style.overflow = overflow;
        root.remove();
      };
      // A modal opened on top supersedes this crop the way it supersedes itself: the
      // crop is cancelled, so its promise settles and the next crop can open.
      dismissOf.set(root, () => { finish(); onCancel(); });

      root.setAttribute("aria-hidden", "false");
      document.body.append(root);
      confirm.focus();

      return { close: finish };
    },
  };
}

// The exact Bevel frame, for callers that want ClayJS's crop without the page's theme.
export const bevelCropAdapter = createCropAdapter();

export function quickcrop(file, options = {}) {
  const modal = options.modal ?? "auto";
  const framed = modal === "auto" || isThemodal(modal);
  if (!framed) return vendorQuickcrop(file, options);
  const adapter = createCropAdapter({ theme: resolveTheme(), confirmLabel: options.labels?.confirm ?? "Crop" });
  return vendorQuickcrop(file, { ...options, modal: adapter });
}

export default quickcrop;
