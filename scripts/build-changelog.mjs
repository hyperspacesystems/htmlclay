#!/usr/bin/env node
// Writes website/changelog.html (served at htmlclay.com/changelog) from
// CHANGELOG.md, filling the <!-- changelog:entries --> slot in
// website/changelog.template.html. CHANGELOG.md is the source; the page is
// committed. --check exits 1 when the committed page is stale.
//
// The Markdown it reads is the changelog's own: "## [version] - date" releases
// ([Unreleased] is skipped), intro paragraphs before the first "###", "###"
// groups, "- " items continued by indented lines, an indented paragraph after a
// blank line inside an item, and inline **bold**, `code` and [links](url).

import { readFileSync, writeFileSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';

const SLOT = '<!-- changelog:entries -->';
const MONTHS = ['January', 'February', 'March', 'April', 'May', 'June', 'July', 'August', 'September', 'October', 'November', 'December'];

export function escapeHTML(text) {
  return text.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;');
}

const bold = html => html.replace(/\*\*(.+?)\*\*/g, '<strong>$1</strong>');

export function inline(text) {
  const codes = [];
  let html = text.replace(/`([^`]+)`/g, (_, code) => {
    codes.push(code);
    return `\u0000${codes.length - 1}\u0000`;
  });
  html = escapeHTML(html);
  // A link is set aside like code, so bold never reaches into its address, and
  // only web, site and anchor addresses become links; any other shows as text.
  const links = [];
  html = html.replace(/\[([^\]]+)\]\(([^)\s]+)\)/g, (whole, label, url) => {
    if (!/^(?:https?:\/\/|\/|#)/.test(url)) return label;
    links.push(`<a href="${url}">${bold(label)}</a>`);
    return `\u0001${links.length - 1}\u0001`;
  });
  html = bold(html);
  html = html.replace(/\u0001(\d+)\u0001/g, (_, i) => links[Number(i)]);
  return html.replace(/\u0000(\d+)\u0000/g, (_, i) => `<code>${escapeHTML(codes[Number(i)])}</code>`);
}

function longDate(iso) {
  const [year, month, day] = iso.split('-').map(Number);
  return `${MONTHS[month - 1]} ${day}, ${year}`;
}

// parse returns the released versions, newest first as the file lists them:
// [{version, date, intro: [paragraph], groups: [{name, items: [[paragraph]]}]}].
// [Unreleased] is read by the same rules and left out, so a line the page could
// not place fails CI before a release copies it into a released section.
export function parse(markdown) {
  const releases = [];
  let release = null;
  let group = null;
  let item = null;
  let paragraph = null;
  let blank = false;
  const close = () => {
    paragraph = null;
  };
  for (const raw of markdown.split(/\r?\n/)) {
    const line = raw.trimEnd();
    const heading = /^## \[([^\]]+)\](?: - (\d{4}-\d{2}-\d{2}))?/.exec(line);
    if (heading) {
      close();
      group = item = null;
      release = { version: heading[1], date: heading[2] ?? '', intro: [], groups: [] };
      if (heading[1] !== 'Unreleased') releases.push(release);
      blank = false;
      continue;
    }
    if (!release) continue;
    if (line === '') {
      blank = true;
      close();
      continue;
    }
    const section = /^### (.+)$/.exec(line);
    if (section) {
      group = { name: section[1].trim(), items: [] };
      release.groups.push(group);
      item = null;
      close();
      blank = false;
      continue;
    }
    const bullet = /^- (.*)$/.exec(line);
    if (bullet && group) {
      paragraph = [bullet[1]];
      item = [paragraph];
      group.items.push(item);
      blank = false;
      continue;
    }
    const indented = /^\s{2,}(\S.*)$/.exec(line);
    if (indented && item) {
      if (blank || paragraph === null) {
        paragraph = [indented[1]];
        item.push(paragraph);
      } else {
        paragraph.push(indented[1]);
      }
      blank = false;
      continue;
    }
    if (group === null) {
      if (blank || paragraph === null) {
        paragraph = [line.trim()];
        release.intro.push(paragraph);
      } else {
        paragraph.push(line.trim());
      }
      blank = false;
      continue;
    }
    throw new Error(`CHANGELOG.md: cannot place this line in ${release.version}: ${JSON.stringify(line)}`);
  }
  return releases.map(r => ({
    ...r,
    intro: r.intro.map(p => p.join(' ')),
    groups: r.groups.map(g => ({ name: g.name, items: g.items.map(it => it.map(p => p.join(' '))) })),
  }));
}

export function render(releases) {
  const blocks = releases.map(r => {
    const out = [`      <article class="release" id="v${escapeHTML(r.version)}">`];
    if (r.date) out.push(`        <time class="release-date" datetime="${r.date}">${longDate(r.date)}</time>`);
    out.push('        <div class="release-notes">');
    out.push(`          <h2><a href="#v${escapeHTML(r.version)}">${escapeHTML(r.version)}</a></h2>`);
    for (const p of r.intro) out.push(`          <p>${inline(p)}</p>`);
    for (const g of r.groups) {
      out.push(`          <h3>${escapeHTML(g.name)}</h3>`);
      out.push('          <ul>');
      for (const it of g.items) {
        const body = it.length === 1 ? inline(it[0]) : it.map(p => `<p>${inline(p)}</p>`).join('');
        out.push(`            <li>${body}</li>`);
      }
      out.push('          </ul>');
    }
    out.push('        </div>');
    out.push('      </article>');
    return out.join('\n');
  });
  return blocks.join('\n\n');
}

export function build(markdown, template) {
  const parts = template.split(SLOT);
  if (parts.length !== 2) throw new Error(`the template needs exactly one ${SLOT}`);
  // Joined, not String.replace: a changelog line holding $' or $& would be read
  // as a replacement pattern and copy parts of the template into the page.
  return parts[0] + render(parse(markdown)) + parts[1];
}

function main() {
  const root = join(dirname(fileURLToPath(import.meta.url)), '..');
  const page = join(root, 'website', 'changelog.html');
  const fresh = build(readFileSync(join(root, 'CHANGELOG.md'), 'utf8'), readFileSync(join(root, 'website', 'changelog.template.html'), 'utf8'));
  if (process.argv.includes('--check')) {
    let current = '';
    try {
      current = readFileSync(page, 'utf8');
    } catch {}
    if (current !== fresh) {
      console.error('website/changelog.html is stale: run node scripts/build-changelog.mjs');
      process.exit(1);
    }
    console.log('website/changelog.html matches CHANGELOG.md');
    return;
  }
  writeFileSync(page, fresh);
  console.log('wrote website/changelog.html');
}

if (process.argv[1] && fileURLToPath(import.meta.url) === process.argv[1]) main();
