import { readFileSync, writeFileSync } from 'node:fs';

const [source, target, note] = process.argv.slice(2);
if (!source || !target || !note) {
  console.error('usage: node make-dashboard.mjs <kiln-final.html> <dashboard-preview.html> "<banner text>"');
  process.exit(2);
}

const html = readFileSync(source, 'utf8');
const cut = (from, to, start = 0) => {
  const a = html.indexOf(from, start);
  const b = html.indexOf(to, a + from.length);
  if (a < 0 || b < 0) {
    console.error(`could not find ${from} … ${to}`);
    process.exit(1);
  }
  return [a, b];
};

const [, headEnd] = cut('<head>', '<header class="page-top"');
const [sectionStart, sectionEnd] = cut('<section class="major-section" id="d">', '<section class="major-section', html.indexOf('<main>'));
const banner = `<p style="margin:0;padding:12px 40px;background:#2B303F;color:#F1F2F6;font:600 14px -apple-system,system-ui,sans-serif">${note}</p>`;
const page = `${html.slice(0, headEnd)}${banner}\n<main>\n${html.slice(sectionStart, sectionEnd)}</main>\n</body>\n</html>\n`;

const ids = [...page.matchAll(/id="(sD\d+)"/g)].map(m => m[1]);
const strays = [...page.matchAll(/id="(s[ABSRM]\d+)"/g)].map(m => m[1]);
if (ids.length < 11 || strays.length > 0) {
  console.error(`expected the 11 dashboard screens only, found ${ids.length} and strays ${strays.join(' ')}`);
  process.exit(1);
}
if (/https?:\/\/(?!www\.w3\.org)/.test(page.replace(/<a [^>]*href="https?:[^"]*"/g, ''))) {
  console.error('the page must not load anything from the network');
  process.exit(1);
}
writeFileSync(target, page);
console.log(`DASHBOARD OK ${ids.length} screens, ${page.length} bytes`);
