import { readFileSync, writeFileSync } from 'node:fs';

const [source, target] = process.argv.slice(2);
if (!source || !target) {
  console.error('usage: node make-film.mjs <icon-anim-embed.html> <icon-film.html>');
  process.exit(2);
}

const edits = [
  ['once = calm,', 'once = true,'],
  ['window.iconAnim = api;', "window.iconAnim = api;\n  document.addEventListener('click', () => { if (!playing) api.replay(); });"],
];

let html = readFileSync(source, 'utf8');
for (const [from, to] of edits) {
  const count = html.split(from).length - 1;
  if (count !== 1) {
    console.error(`expected exactly one "${from}", found ${count}`);
    process.exit(1);
  }
  html = html.replace(from, to);
}
if (/https?:\/\/(?!www\.w3\.org)/.test(html)) {
  console.error('the film must not load anything from the network');
  process.exit(1);
}
writeFileSync(target, html);
console.log(`FILM OK ${html.length} bytes`);
