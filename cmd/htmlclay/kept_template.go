package main

import "html/template"

var keptTemplate = template.Must(template.New("kept").Parse(`<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Recovered changes · HTML Clay</title>
<style nonce="{{.Nonce}}">
:root{color-scheme:light dark;--bg:#F4F2EC;--card:#FCFBF7;--ink:#24211c;--soft:#6c665c;--line:#e3dfd6;--warn:#fbf1dc;--warnline:#e9d3a3;--accent:#2f6fde;--accentink:#f7f9fc;--danger:#C2352B;--dangerbg:#FBE9E7;--pill:#FBF0D9;--pillink:#8A5A00}
@media (prefers-color-scheme:dark){:root{--bg:#1F2128;--card:#282B34;--ink:#ece8e0;--soft:#a39d92;--line:#37352f;--warn:#3a3122;--warnline:#5c4a2a;--accent:#5b8ff0;--accentink:#0f1420;--danger:#F0857B;--dangerbg:#4A2420;--pill:#4A3B1A;--pillink:#F2C46B}}
*{box-sizing:border-box}
body{margin:0;background:var(--bg);color:var(--ink);font:14px/1.5 -apple-system,system-ui,"Segoe UI",sans-serif}
main{max-width:760px;margin:0 auto;padding:32px 16px 64px}
h1{font-size:22px;margin:0 0 4px}
h2{font-size:15px;margin:32px 0 10px}
p{margin:0 0 12px}
.lede{color:var(--soft)}
.done{background:var(--card);border:1px solid var(--line);border-radius:10px;padding:10px 14px;margin:16px 0}
.card{background:var(--card);border:1px solid var(--line);border-radius:12px;padding:16px;margin:0 0 12px}
.card.missing{background:var(--warn);border-color:var(--warnline)}
.head{display:flex;flex-wrap:wrap;align-items:baseline;gap:8px 12px;margin-bottom:6px}
.file{font-weight:600;font-size:15px}
.path{color:var(--soft);font-family:ui-monospace,Menlo,Consolas,monospace;font-size:12.5px;word-break:break-all}
.status{margin-left:auto;font-size:12.5px;color:var(--soft)}
.names{color:var(--soft);font-size:13px;margin-bottom:10px}
ul{list-style:none;margin:0;padding:0}
li{display:flex;flex-wrap:wrap;align-items:center;gap:6px 12px;padding:8px 0;border-top:1px solid var(--line)}
li .who{flex:1;min-width:200px}
li .detail{color:var(--soft);font-size:12.5px;word-break:break-all}
form{margin:0}
button{font:inherit;border-radius:7px;padding:5px 12px;cursor:pointer;border:1px solid var(--line);background:var(--card);color:var(--ink)}
button.danger{color:var(--danger)}
.btn{display:inline-block;padding:4px 12px;border-radius:6px;border:1px solid var(--line);text-decoration:none;color:var(--ink)}
.btn.danger{background:var(--dangerbg);color:var(--danger)}
.danger-link{color:var(--danger);text-decoration:none}
.danger-link:hover{text-decoration:underline}
.pill{border-radius:999px;padding:2px 8px;font-size:12.5px}
.pill.warn{background:var(--pill);color:var(--pillink)}
button.primary{background:var(--accent);border-color:var(--accent);color:var(--accentink)}
.actions{display:flex;flex-wrap:wrap;gap:8px;margin-top:12px}
.empty{color:var(--soft)}
table{width:100%;border-collapse:collapse;font-size:13px}
td{padding:7px 0;border-top:1px solid var(--line);vertical-align:top;word-break:break-all}
td+td{padding-left:12px}
</style>
</head>
<body>
<main>
{{if not .SignedIn}}
<h1>Recovered changes</h1>
<p class="lede">This link has expired. Open Recovered changes again from the HTML Clay menu.</p>
{{else if .Confirm}}
<h1>{{if and .All (gt .Count 1)}}Delete all {{.Count}} recovered changes?{{else}}Delete this recovered change?{{end}}</h1>
<section class="card">
  <div class="head"><span class="file">{{.File}}</span>{{if .Missing}}<span class="status pill warn">{{.Status}}</span>{{end}}</div>
  <div class="path">{{.Path}}</div>
  {{if .All}}<p class="names">{{.Count}} {{if eq .Count 1}}recovered change{{else}}recovered changes{{end}} · {{.Total}}</p>{{else}}<p class="names">{{.When}} · {{.Size}}</p>{{end}}
</section>
<p>{{if .All}}Once you delete these recovered changes, the edits saved inside them cannot be brought back. Earlier versions in this document's history stay untouched.{{else}}Once you delete this recovered change, the edits saved inside it cannot be brought back. Earlier versions in this document's history stay untouched.{{end}}</p>
<div class="actions">
<form method="post" action="/recovered/action"><input type="hidden" name="csrf" value="{{.CSRF}}"><input type="hidden" name="action" value="{{if .All}}delete-all{{else}}delete{{end}}"><input type="hidden" name="folder" value="{{.Folder}}">{{if .All}}{{range .Names}}<input type="hidden" name="name" value="{{.}}">{{end}}{{else}}<input type="hidden" name="name" value="{{.Name}}">{{end}}<button class="btn danger" type="submit">{{if and .All (gt .Count 1)}}Delete all {{.Count}} recovered changes{{else}}Delete{{end}}</button></form>
<a class="btn" href="/recovered">Cancel</a>
</div>
{{else}}
<h1>Recovered changes</h1>
<p class="lede">HTML Clay kept these copies before a change made outside a page removed data, or before you restored an earlier version. They stay on this computer until you delete them.</p>
{{if .Count}}<p>{{.Count}} {{if eq .Count 1}}recovered change{{else}}recovered changes{{end}} · {{.Total}}</p>{{end}}
{{with .Done}}<p class="done" role="status">{{.}}</p>{{end}}
{{if not .Documents}}<p class="empty">No recovered changes left.</p>{{end}}
{{range .Documents}}
<section class="card">
  <div class="head"><span class="file">{{.File}}</span>{{if .Missing}}<span class="status pill warn">{{.Status}}</span>{{end}}</div>
  <div class="path">{{.Path}}</div>
  <p class="names">{{.Count}} {{if eq .Count 1}}recovered change{{else}}recovered changes{{end}} · {{.Total}}</p>
  <ul>
  {{range .Rows}}
    <li>
      <span class="who">{{.When}} · <span class="detail">{{.Size}}</span></span>
      <form method="post" action="/recovered/action"><input type="hidden" name="csrf" value="{{$.CSRF}}"><input type="hidden" name="action" value="reveal"><input type="hidden" name="folder" value="{{.Folder}}"><input type="hidden" name="name" value="{{.Name}}"><button type="submit">{{$.RevealLabel}}</button></form>
      <a class="btn" href="{{.Confirm}}">Delete…</a>
    </li>
  {{end}}
  </ul>
  <div class="actions">
    <a class="danger-link" href="/recovered/confirm?folder={{.Folder}}&all=1">{{if eq .Count 1}}Delete this recovered change…{{else}}Delete all {{.Count}} recovered changes…{{end}}</a>
  </div>
</section>
{{end}}
{{end}}
</main>
</body>
</html>
`))
