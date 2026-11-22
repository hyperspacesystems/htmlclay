#!/usr/bin/env node
// Regenerates htmlclay.com's 2x Retina screenshots into website/images/v2/.
//
// Default mode (`node scripts/capture-website-screenshots.mjs`):
//   1. Compiles a temporary offscreen SwiftUI ShotRenderer (`activationPolicy = .prohibited`,
//      no Dock icon, no browser) against `macos/Sources/PreviewCore` and `PreviewUI`.
//   2. Renders Light + Dark PNGs for `panel`, `request.found`, `welcome`, and every
//      `SettingsPane` in `SettingsPane.allCases` (`general`, `ai`, `folders`, `programs`,
//      `sync`, `updates`, `advanced`).
//   3. Normalizes all Settings panes to a uniform 2x WebP canvas (stretching only the empty
//      lower area inside the floating sidebar card above its bottom rounded cap so switching
//      tabs on htmlclay.com has 0px layout shift).
//   4. Synthesizes `website/images/v2/apps/blockeditor-dark.webp` from `blockeditor.webp`
//      using smooth Kiln dark-mode luminance mapping so Sunday dinner matches dark mode.
//
// Optional `--apps` flag (`node scripts/capture-website-screenshots.mjs --apps`):
//   Also recaptures the starter apps listed in `../onehtmlfile/copy.json` at 2x Retina
//   using a muted headless Chromium instance, hiding marketing intro strips and visitor bars.

import { execFileSync } from 'node:child_process';
import { existsSync, mkdirSync, mkdtempSync, readFileSync, rmSync, symlinkSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';

const ROOT = join(dirname(fileURLToPath(import.meta.url)), '..');
const MACOS_SOURCES = join(ROOT, 'macos', 'Sources');
const OUT_DIR = join(ROOT, 'website', 'images', 'v2');
const APPS_OUT_DIR = join(OUT_DIR, 'apps');
const includeApps = process.argv.includes('--apps');

mkdirSync(APPS_OUT_DIR, { recursive: true });

const workDir = mkdtempSync(join(tmpdir(), 'htmlclay-site-shots-'));
const rawPngDir = join(workDir, 'raw-png');
const pkgDir = join(workDir, 'ShotRenderer');
mkdirSync(join(pkgDir, 'Sources', 'ShotRenderer'), { recursive: true });
mkdirSync(rawPngDir, { recursive: true });

try {
  symlinkSync(join(MACOS_SOURCES, 'PreviewCore'), join(pkgDir, 'Sources', 'PreviewCore'));
  symlinkSync(join(MACOS_SOURCES, 'PreviewUI'), join(pkgDir, 'Sources', 'PreviewUI'));

  writeFileSync(
    join(pkgDir, 'Package.swift'),
    `// swift-tools-version: 5.10
import PackageDescription

let package = Package(
    name: "ShotRenderer",
    platforms: [.macOS(.v14)],
    targets: [
        .target(name: "PreviewCore", path: "Sources/PreviewCore"),
        .target(name: "PreviewUI", dependencies: ["PreviewCore"], path: "Sources/PreviewUI"),
        .executableTarget(name: "ShotRenderer", dependencies: ["PreviewCore", "PreviewUI"], path: "Sources/ShotRenderer"),
    ]
)
`
  );

  writeFileSync(
    join(pkgDir, 'Sources', 'ShotRenderer', 'main.swift'),
    `import AppKit
import SwiftUI
import PreviewCore
import PreviewUI

@MainActor
@main
struct ShotRenderer {
    static func main() {
        let app = NSApplication.shared
        app.setActivationPolicy(.prohibited)

        let outDir = URL(fileURLWithPath: CommandLine.arguments[1])
        try? FileManager.default.createDirectory(at: outDir, withIntermediateDirectories: true)

        var wanted: Set<String> = ["panel", "request.found", "welcome"]
        for pane in SettingsPane.allCases {
            wanted.insert("settings." + pane.rawValue)
        }

        let iconFilmPath = URL(fileURLWithPath: CommandLine.arguments[2])
        let iconFilm = try? String(contentsOf: iconFilmPath, encoding: .utf8)

        for (modeName, appearance) in [("light", NSAppearance.Name.aqua), ("dark", NSAppearance.Name.darkAqua)] {
            let nsApp = NSAppearance(named: appearance)!
            NSApp.appearance = nsApp
            let host = SampleHost(persona: .syncedWithTeams)
            let model = AppModel(host: host, initial: host.current)
            for screen in ScreenCatalog.all(model: model, film: iconFilm) where wanted.contains(screen.name) {
                let view = screen.view
                    .environment(\\.colorScheme, modeName == "dark" ? .dark : .light)
                let hosting = NSHostingView(rootView: view)
                hosting.appearance = nsApp
                hosting.layoutSubtreeIfNeeded()
                let fit = hosting.fittingSize
                let size: CGSize
                if screen.name.hasPrefix("settings.") {
                    size = CGSize(width: max(fit.width, 870), height: max(fit.height + 84, 580))
                } else {
                    size = fit
                }
                hosting.frame = NSRect(origin: .zero, size: size)
                hosting.layoutSubtreeIfNeeded()

                let scale: CGFloat = 2.0
                let pxW = Int(ceil(size.width * scale))
                let pxH = Int(ceil(size.height * scale))
                guard let rep = NSBitmapImageRep(
                    bitmapDataPlanes: nil,
                    pixelsWide: pxW,
                    pixelsHigh: pxH,
                    bitsPerSample: 8,
                    samplesPerPixel: 4,
                    hasAlpha: true,
                    isPlanar: false,
                    colorSpaceName: .deviceRGB,
                    bytesPerRow: 0,
                    bitsPerPixel: 0
                ) else { continue }
                rep.size = size
                let win = NSWindow(contentRect: NSRect(origin: .zero, size: size), styleMask: [.borderless], backing: .buffered, defer: false)
                win.appearance = nsApp
                win.contentView = hosting
                hosting.displayIfNeeded()
                hosting.cacheDisplay(in: hosting.bounds, to: rep)

                if let data = rep.representation(using: .png, properties: [:]) {
                    let slug = screen.name.replacingOccurrences(of: ".", with: "-")
                    let out = outDir.appendingPathComponent("\\(slug)-\\(modeName).png")
                    try? data.write(to: out)
                    print("RENDERED \\(out.lastPathComponent) \\(pxW)x\\(pxH)")
                }
            }
        }
    }
}
`
  );

  console.log('1/3 Rendering native macOS SwiftUI views offscreen (Light + Dark, 2x Retina)...');
  const iconFilmFile = join(ROOT, 'macos', 'Resources', 'icon-film.html');
  execFileSync('swift', ['run', '--disable-sandbox', '--package-path', pkgDir, 'ShotRenderer', rawPngDir, iconFilmFile], {
    stdio: 'inherit',
  });

  if (includeApps) {
    console.log('2/3 Recapturing live starter apps (--apps, muted headless browser)...');
    const captureAppsScript = join(workDir, 'capture-apps.mjs');
    const copyJsonPath = join(ROOT, '..', 'onehtmlfile', 'copy.json');
    const playwrightEntry = join(ROOT, '..', 'starter-templates', 'node_modules', 'playwright', 'index.mjs');
    writeFileSync(
      captureAppsScript,
      `import { chromium } from ${JSON.stringify(playwrightEntry)};
import { readFileSync } from 'node:fs';
import { join } from 'node:path';

const copy = JSON.parse(readFileSync(${JSON.stringify(copyJsonPath)}, 'utf8'));
const SLUG_TO_FILE = {
  blockeditor: 'blockeditor',
  journal: 'journal',
  kanban: 'kanban',
  'freelance-rate-calculator': 'calculator',
  whatthefunc: 'whatthefunc',
};
const browser = await chromium.launch({ headless: true, args: ['--mute-audio'] });
for (const [slug, fileKey] of Object.entries(SLUG_TO_FILE)) {
  const app = copy.apps[slug];
  if (!app?.url) continue;
  const ctx = await browser.newContext({ viewport: { width: 1280, height: 800 }, deviceScaleFactor: 2 });
  const page = await ctx.newPage();
  await page.goto(app.url, { waitUntil: 'networkidle', timeout: 30000 });
  await page.waitForTimeout(700);
  const hideIntro = page.getByText(/hide intro/i).first();
  if ((await hideIntro.count()) > 0 && (await hideIntro.isVisible().catch(() => false))) {
    await hideIntro.click();
    await page.waitForTimeout(450);
  }
  await page.evaluate(() => {
    function clean(root) {
      for (const el of root.querySelectorAll('*')) {
        if (el.shadowRoot) clean(el.shadowRoot);
        const st = getComputedStyle(el);
        const txt = el.textContent || '';
        if ((st.position === 'fixed' || st.position === 'sticky') &&
            (txt.includes('Your draft stays in this browser') || txt.includes('Download your copy') || txt.includes('Host and share online'))) {
          el.style.setProperty('display', 'none', 'important');
        }
      }
    }
    clean(document);
  });
  await page.waitForTimeout(250);
  await page.screenshot({ path: join(${JSON.stringify(rawPngDir)}, 'app-' + fileKey + '.png') });
  console.log('CAPTURED app-' + fileKey + '.png from ' + app.url);
  await ctx.close();
}
await browser.close();
`
    );
    execFileSync(process.execPath, [captureAppsScript], { stdio: 'inherit' });
  } else {
    console.log('2/3 Keeping existing starter app captures (pass --apps to recapture live URLs)...');
  }

  console.log('3/3 Normalizing canvases and encoding crisp WebP assets...');
  const pyScript = join(workDir, 'normalize.py');
  writeFileSync(
    pyScript,
    `import sys
from pathlib import Path
from PIL import Image

raw_dir = Path(sys.argv[1])
out_dir = Path(sys.argv[2])
include_apps = sys.argv[3] == "1"

settings_keys = ["general", "ai", "folders", "programs", "sync", "updates", "advanced"]
settings_imgs = {}
max_w, max_h = 1740, 1292
for key in settings_keys:
    for theme in ("light", "dark"):
        p = raw_dir / f"settings-{key}-{theme}.png"
        im = Image.open(p).convert("RGBA")
        settings_imgs[(key, theme)] = im
        max_w = max(max_w, im.size[0])
        max_h = max(max_h, im.size[1])

# Round height to an even integer for clean 2x scaling
if max_h % 2 == 1:
    max_h += 1

SIDEBAR_END_X = 460
BOTTOM_CAP_H = 44

for (key, theme), im in settings_imgs.items():
    w, h = im.size
    pane_bg = im.getpixel((w - 12, h - 12))
    out = Image.new("RGBA", (max_w, max_h), pane_bg)

    detail = im.crop((SIDEBAR_END_X, 0, w, h))
    out.paste(detail, (SIDEBAR_END_X, 0))

    sidebar_top = im.crop((0, 0, SIDEBAR_END_X, h - BOTTOM_CAP_H))
    out.paste(sidebar_top, (0, 0))

    if h < max_h:
        slice_y = h - BOTTOM_CAP_H - 10
        sidebar_slice = im.crop((0, slice_y, SIDEBAR_END_X, slice_y + 1))
        for y in range(h - BOTTOM_CAP_H, max_h - BOTTOM_CAP_H):
            out.paste(sidebar_slice, (0, y))

    sidebar_bottom = im.crop((0, h - BOTTOM_CAP_H, SIDEBAR_END_X, h))
    out.paste(sidebar_bottom, (0, max_h - BOTTOM_CAP_H))

    rgb = Image.new("RGB", out.size, pane_bg[:3])
    rgb.paste(out, mask=out.split()[3])
    dst = out_dir / f"settings-{key}-{theme}.webp"
    rgb.save(dst, "WEBP", quality=88, method=6)
    print(f"WROTE {dst.name:34s} {rgb.size[0]}x{rgb.size[1]} ({dst.stat().st_size / 1024:.1f} KB)")

for stem in ("panel-light", "panel-dark", "request-found-light", "request-found-dark"):
    src = raw_dir / f"{stem}.png"
    if not src.exists():
        continue
    im = Image.open(src).convert("RGBA")
    dst = out_dir / f"{stem}.webp"
    im.save(dst, "WEBP", quality=88, method=6)
    print(f"WROTE {dst.name:34s} {im.size[0]}x{im.size[1]} ({dst.stat().st_size / 1024:.1f} KB)")

if include_apps:
    for key in ("blockeditor", "journal", "kanban", "calculator", "whatthefunc"):
        src = raw_dir / f"app-{key}.png"
        if not src.exists():
            continue
        im = Image.open(src).convert("RGB")
        if im.size != (1600, 1000):
            im = im.resize((1600, 1000), Image.Resampling.LANCZOS)
        dst = out_dir / "apps" / f"{key}.webp"
        im.save(dst, "WEBP", quality=88, method=6)
        print(f"WROTE apps/{dst.name:29s} {im.size[0]}x{im.size[1]} ({dst.stat().st_size / 1024:.1f} KB)")

# Generate Kiln dark-mode variant for Sunday dinner (blockeditor-dark.webp)
light_block = out_dir / "apps" / "blockeditor.webp"
if light_block.exists():
    src_im = Image.open(light_block).convert("RGB")
    px = src_im.load()
    w, h = src_im.size
    dark_im = Image.new("RGB", (w, h))
    dpx = dark_im.load()
    # Map warm paper (#F7F6F2, lum ~245.6) -> Kiln dark canvas (#1C1E26)
    # floating toolbar (#FBFAF7, lum ~250.2) -> elevated dark surface (#252833)
    # ink (#23221F, lum ~34.0) -> crisp off-white ink (#EDEFF5)
    lut = {}
    for y in range(h):
        for x in range(w):
            rgb_key = px[x, y]
            mapped = lut.get(rgb_key)
            if mapped is None:
                r, g, b = rgb_key
                lum = 0.2126 * r + 0.7152 * g + 0.0722 * b
                if lum >= 245.5:
                    # Paper (#F7F6F2) up to floating toolbar (#FBFAF7)
                    u = min(1.0, (lum - 245.5) / 5.5)
                    nr = int(round(28 + u * 9))
                    ng = int(round(30 + u * 10))
                    nb = int(round(38 + u * 13))
                else:
                    # Ink (#23221F, lum ~34) through muted/borders up to paper (#F7F6F2, lum ~245.5)
                    t = max(0.0, min(1.0, (lum - 34.0) / (245.5 - 34.0)))
                    # Gently curve midtones so hairline borders and muted pills read crisply on dark
                    tc = t ** 0.82
                    nr = int(round(238 * (1.0 - tc) + 28 * tc))
                    ng = int(round(240 * (1.0 - tc) + 30 * tc))
                    nb = int(round(246 * (1.0 - tc) + 38 * tc))
                mapped = (max(18, min(246, nr)), max(20, min(248, ng)), max(26, min(250, nb)))
                lut[rgb_key] = mapped
            dpx[x, y] = mapped
    dark_dst = out_dir / "apps" / "blockeditor-dark.webp"
    dark_im.save(dark_dst, "WEBP", quality=90, method=6)
    print(f"WROTE apps/{dark_dst.name:29s} {dark_im.size[0]}x{dark_im.size[1]} ({dark_dst.stat().st_size / 1024:.1f} KB)")
`
  );

  execFileSync('python3', [pyScript, rawPngDir, OUT_DIR, includeApps ? '1' : '0'], {
    stdio: 'inherit',
  });

  // Remove obsolete settings-htmlclayFiles-* if present from older builds
  for (const stale of ['settings-htmlclayFiles-light.webp', 'settings-htmlclayFiles-dark.webp']) {
    const p = join(OUT_DIR, stale);
    if (existsSync(p)) rmSync(p);
  }

  console.log('Done! All website screenshots in website/images/v2/ are up to date.');
} finally {
  rmSync(workDir, { recursive: true, force: true });
}
