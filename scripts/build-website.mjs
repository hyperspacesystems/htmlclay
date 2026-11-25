#!/usr/bin/env node
// Builds htmlclay.com's shared header/footer chrome, syncs starter apps from
// One HTML File (`../onehtmlfile/copy.json`), stamps content-hash `?v=<hash>`
// query strings onto local CSS, JS, SVG, and WebP assets so browsers never
// serve stale files from cache, and rebuilds both `website/changelog.html` and
// `website/changelog/index.html`.
//
// Pass `--check` to exit 1 if any committed website file is out of sync.

import { createHash } from 'node:crypto';
import { existsSync, mkdirSync, readdirSync, readFileSync, statSync, writeFileSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';
import { build as buildChangelog, escapeHTML } from './build-changelog.mjs';

const ROOT = join(dirname(fileURLToPath(import.meta.url)), '..');
const WEBSITE_DIR = join(ROOT, 'website');
const DEFAULT_ONEHTMLFILE_JSON = join(ROOT, '..', 'onehtmlfile', 'copy.json');

const PAGES = [
  { file: 'index.html', active: '/' },
  { file: 'features/index.html', active: '/features/' },
  { file: 'apps/index.html', active: '/apps/' },
  { file: 'security/index.html', active: '/security/' },
  { file: 'download/index.html', active: '/download/' },
  { file: 'changelog.template.html', active: '/changelog' },
];

const SHOT_FILE_BY_SLUG = {
  blockeditor: 'blockeditor.webp',
  journal: 'journal.webp',
  kanban: 'kanban.webp',
  'freelance-rate-calculator': 'calculator.webp',
  whatthefunc: 'whatthefunc.webp',
};

const DISPLAY_NAME_OVERRIDES = {
  'freelance-rate-calculator': 'Freelance Rate Calculator',
  whatthefunc: 'What The Func',
};

const hashCache = new Map();
function assetHash(webPath) {
  const clean = webPath.split('?')[0];
  if (hashCache.has(clean)) return hashCache.get(clean);
  const diskPath = join(WEBSITE_DIR, clean);
  if (!existsSync(diskPath) || !statSync(diskPath).isFile()) return null;
  const h = createHash('sha256').update(readFileSync(diskPath)).digest('hex').slice(0, 8);
  hashCache.set(clean, h);
  return h;
}

function versionedUrl(webPath) {
  const clean = webPath.split('?')[0];
  const h = assetHash(clean);
  return h ? `${clean}?v=${h}` : clean;
}

function computeSettingsImagesHash() {
  const v2Dir = join(WEBSITE_DIR, 'images', 'v2');
  if (!existsSync(v2Dir)) return '1';
  const h = createHash('sha256');
  const names = readdirSync(v2Dir).filter((n) => n.startsWith('settings-') && n.endsWith('.webp')).sort();
  for (const name of names) {
    h.update(name);
    h.update(readFileSync(join(v2Dir, name)));
  }
  return h.digest('hex').slice(0, 8);
}

function stampAssetVersions(html) {
  const ogHash = assetHash('/og.png');
  let stamped = html.replace(
    /\b(href|src|data-light-src|data-dark-src)="(\/(?:css|js|assets|images)\/[^"?#]+|\/favicon\.svg|\/sap\.min\.js)(?:\?v=[0-9a-f]+)?"/g,
    (whole, attr, cleanPath) => {
      const h = assetHash(cleanPath);
      return h ? `${attr}="${cleanPath}?v=${h}"` : whole;
    }
  );
  if (ogHash) {
    stamped = stamped.replace(
      /https:\/\/htmlclay\.com\/og\.png(?:\?v=[0-9a-zA-Z_-]+)?/g,
      `https://htmlclay.com/og.png?v=${ogHash}`
    );
  }
  return stamped;
}

function renderHeader(activePath) {
  const cur = (href) => (activePath === href ? ' aria-current="page"' : '');
  const markUrl = versionedUrl('/assets/htmlclay-mark.svg');
  return `  <header class="site-header">
    <nav class="sitenav" aria-label="Primary">
      <a class="brand" href="/"${cur('/')} aria-label="HTML Clay home">
        <img src="${markUrl}" alt="" width="27" height="30">
        <span>HTML Clay</span>
      </a>
      <div class="navlinks">
        <a href="/features/"${cur('/features/')}>Features</a>
        <a href="/apps/"${cur('/apps/')}>Apps</a>
        <a href="/security/"${cur('/security/')}>Security</a>
        <a href="/docs/">Docs</a>
        <a href="/changelog"${cur('/changelog')}>Changelog</a>
        <a class="github-clay" href="https://github.com/hyperspacesystems/htmlclay" target="_blank" rel="noopener" aria-label="View HTML Clay source on GitHub"><span class="github-bracket" aria-hidden="true">&lt;</span><span class="github-mascot" aria-hidden="true"><svg viewBox="0 0 16 16" focusable="false"><path d="M8 0C3.58 0 0 3.58 0 8c0 3.54 2.29 6.53 5.47 7.59.4.07.55-.17.55-.38 0-.19-.01-.82-.01-1.49-2.01.37-2.53-.49-2.69-.94-.09-.23-.48-.94-.82-1.13-.28-.15-.68-.52-.01-.53.63-.01 1.08.58 1.23.82.72 1.21 1.87.87 2.33.66.07-.52.28-.87.51-1.07-1.78-.2-3.64-.89-3.64-3.95 0-.87.31-1.59.82-2.15-.08-.2-.36-1.02.08-2.12 0 0 .67-.21 2.2.82.64-.18 1.32-.27 2-.27.68 0 1.36.09 2 .27 1.53-1.04 2.2-.82 2.2-.82.44 1.1.16 1.92.08 2.12.51.56.82 1.27.82 2.15 0 3.07-1.87 3.75-3.65 3.95.29.25.54.73.54 1.48 0 1.07-.01 1.93-.01 2.2 0 .21.15.46.55.38A8.01 8.01 0 0 0 16 8c0-4.42-3.58-8-8-8Z"/></svg></span><span class="github-word" aria-hidden="true">source</span><span class="github-bracket github-close" aria-hidden="true">/&gt;</span></a>
      </div>
      <a class="nav-cta" href="/download/"${cur('/download/')}>Download</a>
    </nav>
  </header>`;
}

function renderFooter(isHome) {
  const shaAttrs = isHome ? ' data-hv-goal="download" data-hv-where="index" data-hv-os="unknown"' : '';
  const markUrl = versionedUrl('/assets/htmlclay-mark.svg');
  return `  <footer class="site-footer">
    <div class="container">
      <div class="footer-top">
        <a class="footer-brand" href="/">
          <img src="${markUrl}" alt="" width="27" height="30">
          <span>HTML Clay</span>
        </a>
        <p class="footer-tagline">Software in a single HTML file, yours to keep and modify.</p>
      </div>
      <div class="footer-grid">
        <div class="footer-col">
          <h3>Product</h3>
          <ul>
            <li><a href="/features/">Features</a></li>
            <li><a href="/apps/">Starter Apps</a></li>
            <li><a href="/download/">Download</a></li>
            <li><a href="/changelog">Changelog</a></li>
          </ul>
        </div>
        <div class="footer-col">
          <h3>Documentation</h3>
          <ul>
            <li><a href="/docs/guide/quick-start/">Quick Start</a></li>
            <li><a href="/docs/features/">Features Reference</a></li>
            <li><a href="/docs/live-sync/">Live Sync</a></li>
            <li><a href="/docs/json-http/">JSON over HTTP</a></li>
            <li><a href="/docs/llms.txt">llms.txt</a></li>
          </ul>
        </div>
        <div class="footer-col">
          <h3>Trust &amp; Source</h3>
          <ul>
            <li><a href="/security/">Security &amp; Trust</a></li>
            <li><a href="https://github.com/hyperspacesystems/htmlclay">GitHub Source</a></li>
            <li><a href="https://download.htmlclay.com/SHA256SUMS"${shaAttrs}>SHA256SUMS</a></li>
            <li><a href="https://onehtmlfile.com/">Free MIT-0 Apps</a></li>
          </ul>
        </div>
        <div class="footer-col">
          <h3>Ecosystem</h3>
          <ul>
            <li><a href="https://hyperclay.com">Hyperclay</a></li>
            <li><a href="https://clayjs.com">ClayJS</a></li>
            <li><a href="https://hyperclaylocal.com">Hyperclay Local</a></li>
            <li><a href="https://onehtmlfile.com/">One HTML File</a></li>
          </ul>
        </div>
      </div>
    </div>
  </footer>`;
}

async function loadOneHtmlFileApps() {
  const jsonFlagIdx = process.argv.indexOf('--apps-json');
  const jsonPath = jsonFlagIdx >= 0 && process.argv[jsonFlagIdx + 1]
    ? process.argv[jsonFlagIdx + 1]
    : DEFAULT_ONEHTMLFILE_JSON;

  if (!existsSync(jsonPath)) return null;
  return normalizeAppsPayload(JSON.parse(readFileSync(jsonPath, 'utf8')));
}

function normalizeAppsPayload(raw) {
  // Reads `onehtmlfile/copy.json` (ordered by tabs, keeping `whatthefunc` last)
  const entries = [];
  if (Array.isArray(raw?.apps)) {
    for (const item of raw.apps) {
      if (item && item.live !== false) entries.push(item);
    }
  } else if (raw?.apps && typeof raw.apps === 'object') {
    const orderedSlugs = [];
    if (Array.isArray(raw.tabs)) {
      for (const tab of raw.tabs) {
        if (Array.isArray(tab.apps)) {
          for (const s of tab.apps) {
            if (!orderedSlugs.includes(s)) orderedSlugs.push(s);
          }
        }
      }
    }
    for (const s of Object.keys(raw.apps)) {
      if (!orderedSlugs.includes(s)) orderedSlugs.push(s);
    }
    for (const slug of orderedSlugs) {
      const app = raw.apps[slug];
      if (!app || app.live === false) continue;
      entries.push({ slug, ...app });
    }
  }

  // Keep What The Func last on both `/` and `/apps/` per David's preference
  const head = entries.filter((a) => a.slug !== 'whatthefunc');
  const tail = entries.filter((a) => a.slug === 'whatthefunc');
  return [...head, ...tail].map((app) => {
    const cleanUrl = String(app.url || '').replace(/\/?$/, '/');
    const base = cleanUrl.replace(/\/$/, '');
    const dlUrl = app.downloadFileUrl || `${base}/_/download?as=htmlclay&file=1`;
    const shotFile = SHOT_FILE_BY_SLUG[app.slug] || `${app.slug}.webp`;
    return {
      slug: app.slug,
      title: DISPLAY_NAME_OVERRIDES[app.slug] || app.name,
      url: cleanUrl,
      downloadUrl: dlUrl,
      shotSrc: versionedUrl(`/images/v2/apps/${shotFile}`),
      shotAlt: app.ogImageAlt || `${app.name} running as a single HTML file.`,
      cardBody: app.cardBody || app.metaDescription || '',
      whatItIs: app.whatItIs || app.cardBody || '',
      whoItIsFor: app.whoItIsFor || '',
    };
  });
}

function renderHomeAppsBlock(apps) {
  const cards = apps.map((app) => `          <article class="app-card">
            <div class="app-card-shot">
              <img src="${escapeHTML(app.shotSrc)}" alt="${escapeHTML(app.shotAlt)}" width="800" height="500" decoding="async">
            </div>
            <div class="app-card-body">
              <h3>${escapeHTML(app.title)}</h3>
              <p>${escapeHTML(app.cardBody)}</p>
              <div class="app-card-actions">
                <a class="btn btn-secondary btn-sm" href="${escapeHTML(app.url)}">Open live</a>
                <a href="${escapeHTML(app.downloadUrl)}">Download .htmlclay</a>
              </div>
            </div>
          </article>`);

  cards.push(`          <article class="card app-catalog-card">
            <div>
              <h3>Browse the full catalog</h3>
              <p>Explore more free, single-file MIT-0 apps on One HTML File, or grab a lightweight starter file to inspect and customize in your text editor.</p>
            </div>
            <div class="app-card-actions">
              <a class="btn btn-dark btn-sm" href="/apps/">Starter apps &amp; samples</a>
              <a href="https://onehtmlfile.com/">onehtmlfile.com &nearr;</a>
            </div>
          </article>`);

  return `<!-- onehtmlfile:home-apps:start -->\n${cards.join('\n\n')}\n          <!-- onehtmlfile:home-apps:end -->`;
}

function renderAppsPageBlock(apps) {
  const items = apps.map((app, idx) => {
    const loadingAttr = idx === 0 ? '' : ' loading="lazy"';
    const secondParagraph = app.whoItIsFor
      ? `\n              <p>${escapeHTML(app.whoItIsFor)} Download the <span class="filechip">.htmlclay</span> file to keep your data on disk and modify how it works anytime.</p>`
      : '';
    return `          <article class="app-showcase-item">
            <div class="app-showcase-shot">
              <img src="${escapeHTML(app.shotSrc)}" alt="${escapeHTML(app.shotAlt)}" width="800" height="500"${loadingAttr}>
            </div>
            <div class="app-showcase-copy">
              <h2>${escapeHTML(app.title)}</h2>
              <p>${escapeHTML(app.whatItIs)}</p>${secondParagraph}
              <div class="app-meta-row">
                <span class="kiln-pill kiln-pill-synced">MIT-0</span>
                <span class="filechip">Signed: ecosystem-2026-11</span>
              </div>
              <div class="app-card-actions">
                <a class="btn btn-amber btn-sm" href="${escapeHTML(app.downloadUrl)}">Download .htmlclay</a>
                <a class="btn btn-secondary btn-sm" href="${escapeHTML(app.url)}">Open live &nearr;</a>
              </div>
            </div>
          </article>`;
  });

  return `<!-- onehtmlfile:apps-page:start -->\n${items.join('\n\n')}\n          <!-- onehtmlfile:apps-page:end -->`;
}

function replaceMarked(html, startTag, endTag, replacement, fileLabel) {
  const startIdx = html.indexOf(startTag);
  const endIdx = html.indexOf(endTag);
  if (startIdx < 0 || endIdx < 0 || endIdx < startIdx) {
    throw new Error(`Missing ${startTag} ... ${endTag} markers in ${fileLabel}`);
  }
  return html.slice(0, startIdx) + replacement + html.slice(endIdx + endTag.length);
}

async function main() {
  const check = process.argv.includes('--check');
  let stale = false;

  // 1. Stamp dynamic Settings WebP cache-buster inside website/js/site.js first
  // so site.js's own hash reflects any Settings image updates.
  const siteJsPath = join(WEBSITE_DIR, 'js', 'site.js');
  if (existsSync(siteJsPath)) {
    const curJs = readFileSync(siteJsPath, 'utf8');
    const settingsVer = computeSettingsImagesHash();
    const nextJs = curJs.replace(
      /var SETTINGS_IMG_VER = "[^"]*";/,
      `var SETTINGS_IMG_VER = "${settingsVer}";`
    );
    if (nextJs !== curJs) {
      stale = true;
      if (!check) {
        writeFileSync(siteJsPath, nextJs, 'utf8');
        console.log('updated website/js/site.js (SETTINGS_IMG_VER)');
      } else {
        console.error('website/js/site.js SETTINGS_IMG_VER is stale: run node scripts/build-website.mjs');
      }
    }
  }

  hashCache.clear();
  const apps = await loadOneHtmlFileApps();

  for (const page of PAGES) {
    const fullPath = join(WEBSITE_DIR, page.file);
    const current = readFileSync(fullPath, 'utf8');
    let next = current;

    if (!/<header class="site-header">[\s\S]*?<\/header>/.test(next)) {
      throw new Error(`Missing <header class="site-header"> in ${page.file}`);
    }
    if (!/<footer class="site-footer">[\s\S]*?<\/footer>/.test(next)) {
      throw new Error(`Missing <footer class="site-footer"> in ${page.file}`);
    }

    next = next.replace(/^[ \t]*<header class="site-header">[\s\S]*?<\/header>/m, renderHeader(page.active));
    next = next.replace(/^[ \t]*<footer class="site-footer">[\s\S]*?<\/footer>/m, renderFooter(page.active === '/'));

    if (apps && page.file === 'index.html') {
      next = replaceMarked(next, '<!-- onehtmlfile:home-apps:start -->', '<!-- onehtmlfile:home-apps:end -->', renderHomeAppsBlock(apps), page.file);
    }
    if (apps && page.file === 'apps/index.html') {
      next = replaceMarked(next, '<!-- onehtmlfile:apps-page:start -->', '<!-- onehtmlfile:apps-page:end -->', renderAppsPageBlock(apps), page.file);
    }

    next = stampAssetVersions(next);

    if (next !== current) {
      stale = true;
      if (!check) {
        writeFileSync(fullPath, next, 'utf8');
        console.log(`updated website/${page.file}`);
      } else {
        console.error(`website/${page.file} is out of sync: run node scripts/build-website.mjs`);
      }
    }
  }

  // Rebuild changelog.html and changelog/index.html from CHANGELOG.md + changelog.template.html
  const changelogMd = readFileSync(join(ROOT, 'CHANGELOG.md'), 'utf8');
  const changelogTpl = readFileSync(join(WEBSITE_DIR, 'changelog.template.html'), 'utf8');
  const freshChangelog = buildChangelog(changelogMd, changelogTpl);

  for (const relOut of ['changelog.html', 'changelog/index.html']) {
    const outPath = join(WEBSITE_DIR, relOut);
    let cur = '';
    try {
      cur = readFileSync(outPath, 'utf8');
    } catch {}
    if (cur !== freshChangelog) {
      stale = true;
      if (!check) {
        mkdirSync(dirname(outPath), { recursive: true });
        writeFileSync(outPath, freshChangelog, 'utf8');
        console.log(`wrote website/${relOut}`);
      } else {
        console.error(`website/${relOut} is stale: run node scripts/build-website.mjs`);
      }
    }
  }

  if (check && stale) process.exit(1);
  if (check) console.log('website pages, asset hashes, One HTML File cards, and changelog are in sync');
}

main().catch((err) => {
  console.error(err.message || err);
  process.exit(1);
});
