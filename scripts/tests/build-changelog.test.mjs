import { test } from 'node:test';
import assert from 'node:assert/strict';
import { build, parse, render } from '../build-changelog.mjs';

const SAMPLE = `# Changelog

Releases before 1.9.0 are elsewhere.

## [Unreleased]

### Added
- Not released yet.

## [1.2.0] - 2026-10-08

This release has an intro
that wraps.

### Added
- **A bold lead with \`<code> & more\`.** Then text
  that continues on the next line, and a [link](https://example.com/a?b=1).
- A loose item.

  Its second paragraph.

### Fixed

- A fix.

## [1.1.0] - 2026-09-01

Nothing changed in the app in this release.
`;

// The page lists released versions only, newest first, with each item's
// wrapped lines joined and a loose item's paragraphs kept apart.
test('the changelog page reads releases, groups, wrapped and loose items', () => {
  const releases = parse(SAMPLE);
  assert.deepEqual(releases.map(r => r.version), ['1.2.0', '1.1.0']);
  const [r] = releases;
  assert.deepEqual(r.intro, ['This release has an intro that wraps.']);
  assert.deepEqual(r.groups.map(g => g.name), ['Added', 'Fixed']);
  assert.equal(r.groups[0].items[0].length, 1);
  assert.match(r.groups[0].items[0][0], /Then text that continues on the next line/);
  assert.deepEqual(r.groups[0].items[1], ['A loose item.', 'Its second paragraph.']);
  assert.deepEqual(r.groups[1].items, [['A fix.']]);
  assert.deepEqual(releases[1].intro, ['Nothing changed in the app in this release.']);
});

// Every text node is escaped, inside code too, so a changelog line that names
// an HTML tag cannot become markup on htmlclay.com.
test('the changelog page escapes text and renders bold, code and links', () => {
  const html = render(parse(SAMPLE));
  assert.match(html, /<strong>A bold lead with <code>&lt;code&gt; &amp; more<\/code>\.<\/strong>/);
  assert.match(html, /<a href="https:\/\/example.com\/a\?b=1">link<\/a>/);
  assert.match(html, /<li><p>A loose item\.<\/p><p>Its second paragraph\.<\/p><\/li>/);
  assert.match(html, /<time class="release-date" datetime="2026-10-08">October 8, 2026<\/time>/);
  assert.doesNotMatch(html, /Not released yet/);
});

test('the template needs its one slot', () => {
  assert.throws(() => build(SAMPLE, '<main></main>'), /changelog:entries/);
  assert.match(build(SAMPLE, '<main><!-- changelog:entries --></main>'), /<main>\s*<article class="release" id="v1.2.0">/);
});

// A line the page cannot place fails while it is still unreleased, in CI's
// --check, instead of halfway through the release that copies it.
test('a line the page cannot place fails in [Unreleased] too', () => {
  const bad = SAMPLE.replace('- Not released yet.', '- Not released yet.\n* A star bullet.');
  assert.throws(() => parse(bad), /cannot place this line in Unreleased/);
});

// Changelog lines quote shell and regex text; none of it may reach the page as
// anything but text, and a link only ever goes to a web address.
test('a line quoting $\' or a script link stays text on the page', () => {
  const md = SAMPLE.replace('- A fix.', () => "- Quotes `$'\\n'` and $& and [a link](javascript:alert%281%29) and [**docs**](https://example.com/**x**).");
  const page = build(md, '<main><!-- changelog:entries --></main><footer>end</footer>');
  assert.equal(page.match(/<footer>/g).length, 1, 'the template was copied into the page');
  assert.ok(page.includes("<code>$'\\n'</code> and $&amp; and a link and"), page);
  assert.ok(!page.includes('javascript:'));
  assert.ok(page.includes('<a href="https://example.com/**x**"><strong>docs</strong></a>'), page);
});
