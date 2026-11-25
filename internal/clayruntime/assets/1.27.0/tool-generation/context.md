# Page tool artifacts

A `page-tool/1` result contains an artifact and initial JSON state. An artifact has exactly six string fields: `title`, `html`, `css`, `javascript`, `fallbackHtml`, and `summary`. Its encoded JSON must fit 24,576 bytes. State must be plain JSON and fit 8,192 bytes. The saved page uses `fallbackHtml`; generated JavaScript runs only in an open editor or reader.

The script defines `mount(tool)` and may return a cleanup function. `tool` supplies `root`, `mode`, `state`, `on("change", fn)`, `on("tables", fn)`, `table(id)`, `tables()`, and tracked `schedule(fn, ms)`. State paths are arrays. A reader gets a local state copy. CSS is scoped to the tool block. Tool code runs with page authority and is not a sandbox.

## Countdown

```json
{
  "title": "Countdown",
  "html": "<strong class=\"count\"></strong>",
  "css": ".count { font-size: 2rem; }",
  "javascript": "function mount(tool) { const count = tool.root.querySelector('.count'); const draw = () => { count.textContent = String(tool.state.data.remaining ?? 0); }; const tick = () => { tool.state.set(['remaining'], Math.max(0, (tool.state.data.remaining ?? 0) - 1)); draw(); if (tool.state.data.remaining > 0) tool.schedule(tick, 1000); }; draw(); if (tool.state.data.remaining > 0) tool.schedule(tick, 1000); return tool.on('change', draw); }",
  "fallbackHtml": "<p>Countdown</p>",
  "summary": "Shows a countdown from the stored remaining seconds."
}
```

## Tally

```json
{
  "title": "Tally",
  "html": "<button type=\"button\">Add one</button><output></output>",
  "css": "button, output { font-size: 1.5rem; }",
  "javascript": "function mount(tool) { const button = tool.root.querySelector('button'); const output = tool.root.querySelector('output'); const draw = () => { output.textContent = String(tool.state.data.count ?? 0); }; const click = () => tool.state.add(['count'], 1); button.addEventListener('click', click); const off = tool.on('change', draw); draw(); return () => { button.removeEventListener('click', click); off(); }; }",
  "fallbackHtml": "<p>Tally</p>",
  "summary": "Counts button presses with the live store add operation."
}
```

## Big number from a table

```json
{
  "title": "Big number",
  "html": "<output class=\"number\"></output>",
  "css": ".number { font-size: 3rem; }",
  "javascript": "function mount(tool) { const output = tool.root.querySelector('output'); const draw = () => { const table = tool.table('revenue'); const values = table?.rows?.map(row => Number(row.cells?.amount)).filter(Number.isFinite) ?? []; output.textContent = String(values.reduce((sum, value) => sum + value, 0)); }; const off = tool.on('tables', draw); draw(); return off; }",
  "fallbackHtml": "<p>Revenue total</p>",
  "summary": "Shows the sum of the amount column in the revenue table."
}
```


# Page tool cookbook: larger examples

Four larger worked examples. Each block is a whole reply: the artifact and the state it starts with.

## Count from a table

```json
{
  "artifact": {
    "title": "Nights below freezing",
    "html": "<div class=\"frost\"><p class=\"lead\"><output class=\"big\">0</output> <span class=\"what\">nights below freezing</span></p><p class=\"detail\"></p><p class=\"note\"></p></div>",
    "css": ".frost { max-width: 26rem; display: grid; gap: 0.5rem; } p { margin: 0; } .big { font-size: 3rem; font-weight: 700; line-height: 1; font-variant-numeric: tabular-nums; } .detail, .note { font-size: 0.875em; } .note { opacity: 0.75; } p:empty { display: none; }",
    "javascript": "function mount(tool) { const find = (selector) => tool.root.querySelector(selector); const lead = find('.lead'), big = find('.big'), what = find('.what'), detail = find('.detail'), note = find('.note'); const plural = (n, one, many) => n + ' ' + (n === 1 ? one : many); const show = (n) => (Math.round(n * 10) / 10 || 0).toLocaleString(); const number = (value) => { const text = String(value ?? '').trim(); return text !== '' && Number.isFinite(Number(text)) ? Number(text) : null; }; const draw = () => { const table = tool.table('readings'); const rows = table?.rows ?? []; const key = (name) => table?.cols?.find((col) => String(col.name).trim().toLowerCase() === name)?.id ?? name; const nights = rows.map((row) => ({ night: String(row.cells?.[key('night')] ?? '').trim(), low: number(row.cells?.[key('low')]) })).filter((night) => night.low !== null); const skipped = rows.length - nights.length; const frosty = nights.filter((night) => night.low < 0).length; lead.hidden = !table; big.textContent = String(frosty); what.textContent = frosty === 1 ? 'night below freezing' : 'nights below freezing'; note.textContent = skipped ? plural(skipped, 'row', 'rows') + ' skipped: no number in the low column.' : ''; if (!table) detail.textContent = 'This page has no readings table to count from.'; else if (!rows.length) detail.textContent = 'The readings table has no rows yet.'; else if (!nights.length) detail.textContent = ''; else { const coldest = nights.reduce((a, b) => (b.low < a.low ? b : a)); const average = nights.reduce((sum, night) => sum + night.low, 0) / nights.length; detail.textContent = 'Out of ' + plural(nights.length, 'reading', 'readings') + '. Coldest ' + show(coldest.low) + ' °C' + (coldest.night ? ' on ' + coldest.night : '') + ', average ' + show(average) + ' °C.'; } }; const off = tool.on('tables', draw); draw(); return off; }",
    "fallbackHtml": "<p>Counts the nights below freezing in the readings table. Open this page to see the current count.</p>",
    "summary": "Counts the rows of the readings table whose low is under zero, with the coldest night and the average as detail. Says so when the table is missing, empty, or has rows without a number."
  },
  "state": {}
}
```

## List with add, edit, remove and undo

```json
{
  "artifact": {
    "title": "Meeting agenda",
    "html": "<div class=\"agenda\"><p><output class=\"big\">0</output> <span class=\"unit\">minutes planned</span></p><ol></ol><p class=\"empty\">No topics yet. Add the first one below.</p><p class=\"undo\" hidden><span></span> <button type=\"button\">Undo</button></p><form novalidate><label class=\"wide\"><span>Topic</span><input name=\"topic\" type=\"text\" maxlength=\"80\" autocomplete=\"off\" placeholder=\"Budget review\"></label><label><span>Minutes</span><input name=\"minutes\" type=\"text\" inputmode=\"numeric\" maxlength=\"4\" autocomplete=\"off\" placeholder=\"15\"></label><button type=\"submit\" class=\"primary\">Add topic</button><button type=\"button\" class=\"cancel\" hidden>Cancel</button><p class=\"error\" role=\"alert\"></p></form></div>",
    "css": ".agenda { max-width: 30rem; display: grid; gap: 0.75rem; } p, ol { margin: 0; padding: 0; } .big { font-size: 2.5rem; font-weight: 700; line-height: 1.1; font-variant-numeric: tabular-nums; overflow-wrap: anywhere; } li { display: flex; flex-wrap: wrap; align-items: center; gap: 0 0.5rem; border-top: 1px solid color-mix(in srgb, currentColor 20%, transparent); } .topic { flex: 1 1 9rem; overflow-wrap: anywhere; } .mins { font-variant-numeric: tabular-nums; } form { display: flex; flex-wrap: wrap; align-items: end; gap: 0.5rem; } label { flex: 0 1 6rem; display: grid; gap: 0.25rem; } label.wide { flex: 1 1 12rem; } label span, .empty, .undo { font-size: 0.875em; } input, button { font: inherit; color: inherit; min-height: 44px; box-sizing: border-box; border: 1px solid color-mix(in srgb, currentColor 40%, transparent); border-radius: 0.375rem; background: transparent; } input { width: 100%; padding: 0 0.625rem; font-size: max(1em, 16px); } input[aria-invalid] { border: 2px solid currentColor; } ::placeholder { color: inherit; opacity: 0.5; } button { min-width: 44px; padding: 0 0.875rem; cursor: pointer; } li button, .undo button { border-color: transparent; padding: 0 0.5rem; text-decoration: underline; } button.primary { background: color-mix(in srgb, currentColor 12%, transparent); border-color: currentColor; font-weight: 600; } :focus-visible { outline: 2px solid currentColor; outline-offset: 2px; } .error { flex-basis: 100%; font-weight: 600; } p:empty, ol:empty, [hidden] { display: none; }",
    "javascript": "function mount(tool) { const doc = tool.root.ownerDocument; const find = (selector) => tool.root.querySelector(selector); const big = find('.big'), unit = find('.unit'), list = find('ol'), empty = find('.empty'), undo = find('.undo'); const form = find('form'), topic = form.elements.topic, minutes = form.elements.minutes; const submit = find('.primary'), cancel = find('.cancel'), error = find('.error'); const stop = new AbortController(); const on = (target, type, handler) => target.addEventListener(type, handler, { signal: stop.signal }); const blank = { topic: '', minutes: '', id: '' }; let removed = null; let drawn = ''; let shown = {}; const read = () => { const data = tool.state.data || {}; const items = data.items || {}; const draft = { ...blank, ...data.draft }; const rows = Object.keys(items).map((id) => ({ id, ...items[id] })).sort((a, b) => a.at - b.at); return { items, rows, draft, editing: items[draft.id] ? draft.id : '' }; }; const put = (path, value) => { try { tool.state.set(path, value); return true; } catch (problem) { return false; } }; const el = (tag, className, text) => { const node = doc.createElement(tag); node.className = className; node.textContent = text; return node; }; const draw = () => { const now = read(); const total = now.rows.reduce((sum, row) => sum + row.minutes, 0); const count = now.rows.length; for (const input of [topic, minutes]) { const text = now.draft[input.name]; if (text !== shown[input.name] && input.value !== text) input.value = text; } shown = now.draft; submit.textContent = now.editing ? 'Save topic' : 'Add topic'; cancel.hidden = !now.editing; big.textContent = String(total); unit.textContent = (total === 1 ? 'minute' : 'minutes') + (count ? ' across ' + count + (count === 1 ? ' topic' : ' topics') : ' planned'); empty.hidden = count > 0; undo.hidden = !removed; undo.firstElementChild.textContent = removed ? 'Removed ' + removed.topic + '.' : ''; const signature = JSON.stringify(now.rows); if (signature === drawn) return; drawn = signature; list.textContent = ''; for (const row of now.rows) { const item = el('li', '', ''); const edit = el('button', 'edit', 'Edit'); const remove = el('button', 'remove', 'Remove'); item.dataset.id = row.id; edit.type = remove.type = 'button'; edit.setAttribute('aria-label', 'Edit ' + row.topic); remove.setAttribute('aria-label', 'Remove ' + row.topic); item.append(el('span', 'topic', row.topic), el('span', 'mins', row.minutes + ' min'), edit, remove); list.append(item); } }; const focusRow = (id, name) => (find('li[data-id=\"' + id + '\"] .' + name) || topic).focus(); const fail = (input, message) => { error.textContent = message; input.setAttribute('aria-invalid', 'true'); input.focus(); }; const full = () => fail(topic, 'This agenda is full. Remove a topic to make room.'); const clearError = () => { error.textContent = ''; topic.removeAttribute('aria-invalid'); minutes.removeAttribute('aria-invalid'); }; on(form, 'input', (event) => { clearError(); put(['draft', event.target.name], event.target.value); }); on(form, 'submit', (event) => { event.preventDefault(); clearError(); const now = read(); const name = topic.value.trim(); const length = minutes.value.trim(); if (!name) return fail(topic, 'Give the topic a name.'); if (!/^\\d+$/.test(length) || Number(length) < 1) return fail(minutes, 'Minutes must be a whole number above 0, such as 15.'); const id = now.editing || 't' + Date.now().toString(36) + Math.random().toString(36).slice(2, 5); if (!put(['items', id], { topic: name, minutes: Number(length), at: now.editing ? now.items[id].at : Date.now() })) return full(); removed = null; put(['draft'], blank); if (now.editing) focusRow(id, 'edit'); else topic.focus(); }); on(list, 'click', (event) => { const button = event.target.closest('button'); const id = button?.closest('li').dataset.id; const item = read().items[id]; if (!item) return; clearError(); if (button.className === 'edit') { if (!put(['draft'], { topic: item.topic, minutes: String(item.minutes), id })) return full(); topic.focus(); } else { removed = { id, topic: item.topic, minutes: item.minutes, at: item.at }; tool.state.delete(['items', id]); undo.lastElementChild.focus(); } }); on(undo.lastElementChild, 'click', () => { const { id, ...item } = removed; if (!put(['items', id], item)) return full(); removed = null; draw(); focusRow(id, 'edit'); }); on(cancel, 'click', () => { const id = read().editing; clearError(); put(['draft'], blank); focusRow(id, 'edit'); }); const off = tool.on('change', draw); draw(); return () => { stop.abort(); off(); }; }",
    "fallbackHtml": "<p>Meeting agenda with a time for each topic. Open this page in an editor to add or change topics.</p>",
    "summary": "Add topics with a length in minutes, edit or remove them, and see the total time. An empty name or a length that is not a whole number gets a message. A removed topic can be put back with Undo."
  },
  "state": {"items":{},"draft":{"topic":"","minutes":"","id":""}}
}
```

## Stopwatch that keeps time across a reload

```json
{
  "artifact": {
    "title": "Stopwatch",
    "html": "<div class=\"watch\"><p class=\"time\" role=\"timer\">0:00</p><p class=\"status\">Ready</p><div class=\"actions\"><button type=\"button\" class=\"primary\">Start</button><button type=\"button\" class=\"reset\" hidden>Reset</button></div><p class=\"undo\" hidden><span></span> <button type=\"button\">Undo</button></p></div>",
    "css": ".watch { max-width: 20rem; display: grid; gap: 0.5rem; } p { margin: 0; } .time { font-size: 3.5rem; font-weight: 700; line-height: 1; font-variant-numeric: tabular-nums; } .status, .undo { font-size: 0.875em; } .actions { display: flex; gap: 0.5rem; margin-top: 0.5rem; } button { font: inherit; color: inherit; min-width: 44px; min-height: 44px; padding: 0 1rem; border: 1px solid color-mix(in srgb, currentColor 40%, transparent); border-radius: 0.375rem; background: transparent; cursor: pointer; } button.primary { flex: 1; background: CanvasText; border-color: CanvasText; color: Canvas; font-weight: 600; } .undo button { border-color: transparent; padding: 0 0.5rem; text-decoration: underline; } :focus-visible { outline: 2px solid currentColor; outline-offset: 2px; } button.primary:focus-visible { outline-color: CanvasText; } [hidden] { display: none; }",
    "javascript": "function mount(tool) { const find = (selector) => tool.root.querySelector(selector); const time = find('.time'), status = find('.status'), toggle = find('.primary'), reset = find('.reset'), undo = find('.undo'); const stop = new AbortController(); const on = (target, type, handler) => target.addEventListener(type, handler, { signal: stop.signal }); let cleared = null; let waiting = false; const read = () => { const run = (tool.state.data || {}).run || {}; return { since: Number(run.since) || 0, banked: Number(run.banked) || 0 }; }; const elapsed = (run) => run.banked + (run.since ? Math.max(0, Date.now() - run.since) : 0); const clock = (ms) => { const seconds = Math.floor(ms / 1000); const two = (n) => String(n).padStart(2, '0'); const minutes = Math.floor(seconds / 60); return (minutes >= 60 ? Math.floor(minutes / 60) + ':' + two(minutes % 60) : minutes) + ':' + two(seconds % 60); }; const draw = () => { const run = read(); const ms = elapsed(run); time.textContent = clock(ms); toggle.textContent = run.since ? 'Pause' : ms ? 'Resume' : 'Start'; status.textContent = run.since ? 'Running since ' + new Date(run.since).toLocaleTimeString([], { hour: 'numeric', minute: '2-digit' }) : ms ? 'Paused' : 'Ready'; reset.hidden = !ms && !run.since; undo.hidden = !cleared; undo.firstElementChild.textContent = cleared ? 'Cleared ' + clock(elapsed(cleared)) + '.' : ''; if (run.since && !waiting) { waiting = true; tool.schedule(() => { waiting = false; draw(); }, 1000 - (ms % 1000)); } }; on(toggle, 'click', () => { const run = read(); cleared = null; tool.state.set(['run'], run.since ? { since: 0, banked: elapsed(run) } : { since: Date.now(), banked: run.banked }); }); on(reset, 'click', () => { cleared = read(); tool.state.set(['run'], { since: 0, banked: 0 }); undo.lastElementChild.focus(); }); on(undo.lastElementChild, 'click', () => { const run = cleared; cleared = null; tool.state.set(['run'], run); toggle.focus(); }); const off = tool.on('change', draw); draw(); return () => { stop.abort(); off(); }; }",
    "fallbackHtml": "<p>Stopwatch. Open this page in an editor to start, pause or reset it.</p>",
    "summary": "Start, pause and reset a stopwatch. It saves when it was started, not a ticking number, so the time stays right after a reload. A reset can be undone."
  },
  "state": {"run":{"since":0,"banked":0}}
}
```

## Calculator with messages for invalid input

```json
{
  "artifact": {
    "title": "Paint needed",
    "html": "<div class=\"paint\"><div class=\"fields\"><label><span>Wall length (m)</span><input name=\"length\" type=\"text\" inputmode=\"decimal\" maxlength=\"12\" autocomplete=\"off\" placeholder=\"14.5\"><small>Add up every wall you will paint.</small></label><label><span>Wall height (m)</span><input name=\"height\" type=\"text\" inputmode=\"decimal\" maxlength=\"12\" autocomplete=\"off\" placeholder=\"2.4\"><small>From the floor to the ceiling.</small></label><label><span>Coverage (m² per litre)</span><input name=\"cover\" type=\"text\" inputmode=\"decimal\" maxlength=\"12\" autocomplete=\"off\" placeholder=\"10\"><small>Printed on the tin.</small></label><div class=\"coats\" role=\"group\" aria-label=\"Coats\"><span>Coats</span><div><button type=\"button\" value=\"1\">1</button><button type=\"button\" value=\"2\">2</button><button type=\"button\" value=\"3\">3</button></div></div></div><div class=\"result\"><p class=\"main\"><output class=\"big\"></output> <span>litres of paint</span></p><p class=\"detail\"></p></div></div>",
    "css": ".paint { max-width: 28rem; display: grid; gap: 1rem; } .fields { display: grid; grid-template-columns: repeat(auto-fit, minmax(11rem, 1fr)); gap: 0.75rem; } label, .coats { display: grid; gap: 0.25rem; align-content: start; } span, small, .detail { font-size: 0.875em; } small { opacity: 0.75; } small.error { opacity: 1; font-weight: 600; } input, button { font: inherit; color: inherit; min-height: 44px; box-sizing: border-box; border: 1px solid color-mix(in srgb, currentColor 40%, transparent); border-radius: 0.375rem; background: transparent; } input { width: 100%; padding: 0 0.625rem; font-size: max(1em, 16px); } input[aria-invalid] { border: 2px solid currentColor; } ::placeholder { color: inherit; opacity: 0.5; } .coats div { display: flex; gap: 0.25rem; } button { flex: 1; cursor: pointer; } button[aria-pressed=\"true\"] { background: color-mix(in srgb, currentColor 12%, transparent); border-color: currentColor; font-weight: 600; } :focus-visible { outline: 2px solid currentColor; outline-offset: 2px; } .result { padding-top: 0.75rem; border-top: 1px solid color-mix(in srgb, currentColor 20%, transparent); } p { margin: 0; } .big { font-size: 2.5rem; font-weight: 700; line-height: 1.1; font-variant-numeric: tabular-nums; overflow-wrap: anywhere; } [hidden] { display: none; }",
    "javascript": "function mount(tool) { const find = (selector) => tool.root.querySelector(selector); const inputs = Array.from(tool.root.querySelectorAll('input')); const buttons = Array.from(tool.root.querySelectorAll('button')); const hints = inputs.map((input) => input.nextElementSibling.textContent); const main = find('.main'), big = find('.big'), detail = find('.detail'); const stop = new AbortController(); const on = (target, type, handler) => target.addEventListener(type, handler, { signal: stop.signal }); const show = (n) => (Math.round(n * 10) / 10).toLocaleString(); const check = (input, text) => { if (text === '') return { empty: true }; if (!Number.isFinite(Number(text))) return { error: 'Type a plain number, such as ' + input.placeholder + '.' }; if (Number(text) <= 0) return { error: 'This has to be more than 0.' }; return { value: Number(text) }; }; const draw = () => { const data = tool.state.data || {}; const coats = [1, 2, 3].includes(data.coats) ? data.coats : 2; const got = {}; let empty = 0, wrong = 0; inputs.forEach((input, index) => { const text = String(data[input.name] ?? ''); const result = got[input.name] = check(input, text.trim()); const small = input.nextElementSibling; if (input.value !== text) input.value = text; small.textContent = result.error || hints[index]; small.className = result.error ? 'error' : ''; if (result.error) input.setAttribute('aria-invalid', 'true'); else input.removeAttribute('aria-invalid'); wrong += result.error ? 1 : 0; empty += result.empty ? 1 : 0; }); for (const button of buttons) button.setAttribute('aria-pressed', String(Number(button.value) === coats)); main.hidden = wrong + empty > 0; if (wrong) detail.textContent = 'Fix the marked ' + (wrong === 1 ? 'field' : 'fields') + ' to see the amount.'; else if (empty) detail.textContent = 'Fill in every field to see how much paint to buy.'; else { const area = got.length.value * got.height.value; big.textContent = show(area * coats / got.cover.value); detail.textContent = show(area) + ' m² of wall, ' + coats + (coats === 1 ? ' coat' : ' coats') + ', ' + show(got.cover.value) + ' m² per litre.'; } }; on(tool.root, 'input', (event) => tool.state.set([event.target.name], event.target.value)); for (const button of buttons) on(button, 'click', () => tool.state.set(['coats'], Number(button.value))); const off = tool.on('change', draw); draw(); return () => { stop.abort(); off(); }; }",
    "fallbackHtml": "<p>Works out the litres of paint for a room from the wall length, wall height, number of coats and the paint's coverage.</p>",
    "summary": "Enter the wall length, height and coverage and pick the coats to see the litres of paint needed. A field that is not a number above 0 gets a message and no result is shown."
  },
  "state": {"length":"","height":"","cover":"10","coats":2}
}
```
