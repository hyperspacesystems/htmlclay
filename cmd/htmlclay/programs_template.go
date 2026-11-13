package main

import "html/template"

var programsTemplate = template.Must(template.New("programs").Parse(`<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Programs · HTML Clay</title>
<style nonce="{{.Nonce}}">
:root{color-scheme:light dark;--bg:#f6f5f2;--card:#fdfcfa;--ink:#24211c;--soft:#6c665c;--line:#e3dfd6;--warn:#fbf1dc;--warnline:#e9d3a3;--accent:#2f6fde;--accentink:#f7f9fc;--danger:#b4382c}
@media (prefers-color-scheme:dark){:root{--bg:#1c1b19;--card:#252421;--ink:#ece8e0;--soft:#a39d92;--line:#37352f;--warn:#3a3122;--warnline:#5c4a2a;--accent:#5b8ff0;--accentink:#0f1420;--danger:#e0786c}}
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
li .detail{display:block;color:var(--soft);font-size:12.5px;word-break:break-all}
form{margin:0}
button{font:inherit;border-radius:7px;padding:5px 12px;cursor:pointer;border:1px solid var(--line);background:var(--card);color:var(--ink)}
button.danger{color:var(--danger)}
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
<h1>Programs</h1>
<p class="lede">This link has expired. Open the Programs page again from the HTML Clay menu in your menu bar or system tray.</p>
{{else}}
<h1>Programs</h1>
<p class="lede">Programs on your computer that your documents may run, and which documents may run them. Removing access here takes effect right away. Giving access always asks you in an HTML Clay dialog.</p>
{{with .Done}}<p class="done" role="status">{{.}}</p>{{end}}

{{if .Missing}}
<h2>Needs attention</h2>
{{range .Missing}}
<section class="card missing">
  <div class="head"><span class="file">{{.File}}</span><span class="status">{{.Status}}</span></div>
  <div class="path">{{.Path}}</div>
  <p class="names">Documents ask for it as {{range $i, $r := .Registrations}}{{if $i}}, {{end}}“{{$r.Name}}”{{end}}.</p>
  {{if .Relink}}<p>HTML Clay found a file with the same name at <span class="path">{{.Relink}}</span>.</p>{{end}}
  <div class="actions">
    {{$id := (index .Registrations 0).ID}}
    <form method="post" action="/programs/action"><input type="hidden" name="csrf" value="{{$.CSRF}}"><input type="hidden" name="action" value="relink"><input type="hidden" name="program" value="{{$id}}"><button class="primary" type="submit">{{if .Relink}}Use the Found File…{{else}}Locate the File…{{end}}</button></form>
    <form method="post" action="/programs/action"><input type="hidden" name="csrf" value="{{$.CSRF}}"><input type="hidden" name="action" value="remove"><input type="hidden" name="program" value="{{$id}}"><button class="danger" type="submit">Remove from List</button></form>
  </div>
</section>
{{end}}
{{end}}

<h2>Programs</h2>
{{if not .Groups}}<p class="empty">No programs yet. When a document asks for one, HTML Clay shows a bar at the top of it to set one up.</p>{{end}}
{{range .Groups}}
<section class="card">
  <div class="head"><span class="file">{{.File}}</span><span class="status">{{.Status}}</span></div>
  <div class="path">{{.Path}}</div>
  <p class="names">Documents ask for it as {{range $i, $r := .Registrations}}{{if $i}}, {{end}}“{{$r.Name}}”{{end}}.</p>
  {{if .Grants}}
  <ul>
  {{range .Grants}}
    <li>
      <span class="who">{{.Label}}<span class="detail">{{.Detail}}</span></span>
      <form method="post" action="/programs/action"><input type="hidden" name="csrf" value="{{$.CSRF}}">{{if .Trusted}}<input type="hidden" name="action" value="revoke-trusted"><input type="hidden" name="program" value="{{.Program}}">{{else}}<input type="hidden" name="action" value="revoke-document"><input type="hidden" name="program" value="{{.Program}}"><input type="hidden" name="document" value="{{.Document}}"><input type="hidden" name="name" value="{{.Name}}">{{end}}<button class="danger" type="submit">Remove Access</button></form>
    </li>
  {{end}}
  </ul>
  {{else}}<p class="empty">No document may run it right now.</p>{{end}}
  <div class="actions">
    {{range .Registrations}}{{if not .AnyDocument}}<form method="post" action="/programs/action"><input type="hidden" name="csrf" value="{{$.CSRF}}"><input type="hidden" name="action" value="widen"><input type="hidden" name="program" value="{{.ID}}"><button type="submit">Allow “{{.Name}}” in Trusted Folders…</button></form>{{end}}{{end}}
    {{if .Grants}}<form method="post" action="/programs/action"><input type="hidden" name="csrf" value="{{$.CSRF}}"><input type="hidden" name="action" value="revoke-all"><input type="hidden" name="program" value="{{(index .Registrations 0).ID}}"><button class="danger" type="submit">Remove All Access</button></form>{{end}}
  </div>
</section>
{{end}}

{{if .Open}}
<h2>Open documents</h2>
<table>
{{range .Open}}<tr><td>{{.Document}}</td><td>“{{.Name}}”</td><td>{{.Status}}</td></tr>{{end}}
</table>
{{end}}
{{end}}
</main>
</body>
</html>
`))
