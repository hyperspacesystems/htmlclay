# Managed trial control API

The protocol Hyperclay Local's adapter (`trial/go-document-host.cjs`) speaks to
the managed trial host. Everything here is what this repository implements; the
one client is Local's own Node code, and the Go host is the only writer of a
served root in the managed trial.

## Transport

- The control listener from G1: the `control` URL of the `READY` line. Its
  address is loopback and its port is ephemeral.
- Every request carries `Authorization: Bearer <secret>`; the secret is the 64
  lowercase hex characters the binary read from `--control-fd`. A request with
  no header, or a wrong secret, is `401` with `{"error":"unauthorized"}`.
- Request bodies are JSON (`Content-Type: application/json`). Answers are JSON,
  except the event stream, which is newline-delimited JSON.
- Bytes travel as `{ "encoding": "utf8" | "base64", "data": "..." }`. In a
  request, `utf8` means the caller sent a string and `base64` means it sent
  binary. In an answer, a value the lane holds as a string is `utf8`, and bytes
  are `utf8` when they are valid UTF-8 text without NUL and `base64` otherwise.
  A value a lane did not name is `null`.
- A request never carries a file path. A request names a root by the id the
  `READY` line gave it (`root-0`, `root-1`, ...) plus the logical name; Go
  resolves the canonical path itself (G3), and a name that resolves outside the
  root is refused.

## `POST /v1/commit`

```json
{ "opId": "…", "root": "root-0", "name": "index.html", "kind": "browser-save",
  "expected": { "type": "checksum", "value": "…" },
  "bytes": { "encoding": "utf8", "data": "<!doctype html>…" },
  "edit": false, "safety": false, "modifiedAt": 1762000000000,
  "pageRecord": { "v": 1, "rev": "…", "tag": "…" },
  "saveId": "…", "userDriven": true,
  "opHandle": "…", "headers": { "if-match": "…" }, "origin": "…",
  "captureId": "…" }
```

`kind` is one of G6's `KINDS`: `browser-save`, `data-api`, `restore`,
`guard-restore`, `sync-download`, `external-rebase`, `create`. `bytes`,
`expected`, `pageRecord`, `modifiedAt`, `saveId`, `userDriven`, `edit` and
`safety` are optional, and `userDriven` is a tri-state: absent means the save
names no provenance (Local's `ui-unknown`).

With a service attached, four more fields reach the write's consultation.
`opHandle` is the opId the service sent with this commit, echoed in every
consultation call so the service finds its own state by it. `headers` are the
request headers Local's lanes read, by lower-case name, sent with `admit`.
`origin` names the write's origin to the service when it is not the lane's own
(a checkpoint). `captureId` is the checkpoint capture the write stores, marked
saved on `settle`.

The request runs G6's `commit` with that kind under the document's lock and
answers Local's `CommitResult` plus the frame it produced:

```json
{ "outcome": "committed" | "refused" | "indeterminate",
  "reason": null | "changed" | "…",
  "etag": null | "…", "checksum": null | "…",
  "stored": { "encoding": "utf8", "data": "…" } | null,
  "content": { "encoding": "base64", "data": "…" } | null,
  "refusal": null | { "status": 409, "code": "…",
                      "body": { … } | null, "headers": { "…": "…" } },
  "opId": "…", "cursor": "<epoch>:<seq>" }
```

`opId` is the caller's, or one Go made up when the caller sent none. `checksum`
is the lane's own checksum when it computes one (`sync-download`) and `null`
where Local answers `null`. `refusal` is the refusal the JS service decided, as
it was decided: its `status`, its `code`, its `body` and the `headers` it
answers the client with. It is `null` for every write the service did not
refuse.

Refusals by the API itself, all `400` with `{"error": "…"}`:

- an unknown `kind`: `Unknown document write kind: <kind>`, Local's message;
- `"hooks"` present in the body at all: `hooks ended in M16`, Local's message
  (the native stage has no hook carrier and Local refuses `hooks` too);
- `kind: "checkpoint"`: `checkpoint needs rooms`, because the native stage has
  no rooms to capture. (`checkpoint` is a separate entry point in Local, not a
  commit kind.)

A name Go cannot resolve, or a lane that fails before writing, answers `400`;
a write whose bytes landed but whose directory entry may not survive still
answers `200` with `outcome: "indeterminate"`, never `committed`.

## `POST /v1/checkpoint`

```json
{ "opId": "…", "root": "root-0", "name": "index.html",
  "roomIdentity": "…", "generation": 12 | null }
```

Local's checkpoint, run by this host: the JS service's checkpoint scheduler
posts one for a dirty room, and Go does the write. A checkpoint needs the JS
service (404 `{"error":"checkpoint needs the service"}` without one), a page
document and a room identity (400 otherwise). Under the document's lock, after
refusing a barred document and reading the identity, the bytes and the
snapshot, Go asks the service to render the room with `POST
/v1/render-checkpoint`: the consultation's usual fields plus `roomIdentity`,
`generation` and `current`, with `opHandle` set to this opId and `lane` `save`
and `origin` `checkpoint`. There is no `claim` and no `captureId` in that call:
the room, its target and the admission claim are the service's, found by
`opHandle`.

A `render` answer runs the save lane with origin `checkpoint`,
`userDriven: false`, `saveId: ""` and `dataLossPrev` from the file it read,
through the same lock: `admit` and `settle` run as they do for any save. The
file's identity is checked again immediately before the rename, as Local's
checkpoint does; a file that changed since it was read answers `refused` with
`invalid-identity`, and nothing replaces it.

```json
{ "kind": "ok", "revision": 4 | null,
  "written": { "encoding": "utf8", "data": "…" } | null,
  "opId": "…", "cursor": "<epoch>:<seq>" } |
{ "kind": "clean" | "current", "reason": null | "…",
  "opId": "…", "cursor": null } |
{ "kind": "refused", "reason": "…" | "invalid-identity",
  "opId": "…", "cursor": null } |
{ "kind": "indeterminate", "reason": "…",
  "opId": "…", "cursor": "<epoch>:<seq>" }
```

`ok` reports the render's `revision` and the save's `written` bytes (`null`
unless the write committed), plus the opId and its frame's cursor, whatever the
save's outcome other than indeterminate. `clean`, `current` and `refused` are
the render's own answers passed through, with its `reason`. `refused` with
`invalid-identity` is the identity check before the rename.

A barred document, or a service that cannot be reached, answers `503` with
`{"reason":"unconfirmed","retryable":true}` or
`{"reason":"service-unavailable","retryable":true}` and a `Retry-After`, so the
service's checkpoint throws and its scheduler retries.

## `POST /v1/restructure`

```json
{ "opId": "…", "root": "root-0", "kind": "move",
  "steps": [ { "from": "a.html", "to": "b.html" } ] }
```

`kind` is one of G6b's `STRUCTURE_KINDS` (`move`, `trash`, `duplicate`,
`sync-move`, `sync-trash`) or one of the two workspace kinds: `folder`, whose
steps carry only `to` and each make one new directory (never its parents), and
`empty-trash`, which takes no steps and removes everything inside the root's
`.trash/`. A step may carry `"pageState": "move"` (kinds `move`, `sync-move`) or
`"pageState": "drop"` (kinds `trash`, `sync-trash`): after the moves, every
completed step's page state is moved or dropped (the folder form for a
directory) inside the same exclusive section, as Local's workspace command does
after a completed step. A restructure holds the process-wide structure gate, so
no document write and no other structure change runs until it answers. Go
resolves both names of every step and answers Local's `StructureResult`:

```json
{ "outcome": "committed" | "partial" | "failed",
  "completed": 1, "reason": null | "ENOENT" | "…",
  "pageState": null | "ENOENT" | "…",
  "opId": "…", "cursor": "<epoch>:<seq>" }
```

A restructure is not a document commit, so it appends no frame; `cursor` is
where the stream stands when it answers.

## `POST /v1/page-state`

```json
{ "opId": "…", "root": "root-0", "op": "write",
  "name": "a.html", "to": "b.html", "record": { "v": 1, "rev": "…", "tag": "…" } }
```

`op` is one of Local's host `pageState` operations, run over G4b's store:

- `write` takes `record` and writes the document's sidecar;
- `move` takes `to` and carries the sidecar to the new name;
- `moveDir` takes `to` and carries a folder's whole page-state mirror;
- `drop` removes the sidecar;
- `dropDir` removes a folder's mirror.

Each returns nothing in Local, so the answer body is `null` with status `200`.
Any other `op` is `400` with `Unknown page-state operation: <op>`. A `write`
holds the document's lock; `move`, `moveDir`, `drop` and `dropDir` hold the
structure gate.

## `POST /v1/snapshot/take`

```json
{ "root": "root-0", "name": "a.html" }
```

Local's `getAndClearSnapshot` (`server.js:187-194`): what the document owes the
platform on its next sync upload, read once through the relay's snapshot map
(G6a's store) and cleared, so a second upload cannot send the same working state
twice.

```json
{ "snapshot": { "html": "<!doctype html>…", "userDriven": true } } |
{ "snapshot": { "html": null, "userDriven": true } } |
{ "snapshot": null }
```

`html` is the last snapshot the document relayed through `POST /_/sync`, and
`null` when only a save has written the entry. `userDriven` is the provenance
bit the document's last save left, and it is absent when neither lane named one
(Local's `undefined`, the `ui-unknown` an upload reports). `snapshot` is `null`
when the document owes nothing: no entry at all, or an entry that names neither
half. An entry older than five minutes is dropped by the host's sweep before any
caller can take it.

## `GET /v1/read?root=root-0&name=a.html`

Local's `read` (`document-host.js:137-146`), taken under the document's lock:

```json
{ "exists": true,
  "checksum": "<Local's calculateChecksum of the file's bytes>",
  "bytes": { "encoding": "utf8", "data": "…" } }
```

A file that is not there answers `200` with `{"exists": false, "checksum": null,
"bytes": null}`; an unreadable one answers `500`.

The read holds the document's lock, so it never sees a sidecar written for bytes
not yet renamed into place. After `exists`, `checksum` and `bytes` it carries
`fileSha256`, `identity` (`dev`, `ino`, `mtimeNs` as decimal strings, `size` as a
number: Node's bigint `stat`, volume serial and file index on Windows), and, for a
page document whose bytes are UTF-8, `logical` (the document with its merge state
put back) and `sidecar` (the stored page-state record as the sidecar holds it
(`v`, `rev`, `tag`, `generation`, `prev`), or null); each is null when it does not
apply.

## `POST /v1/durable`

```json
{ "root": "root-0", "name": "a.html", "checksum": "<the bytes' checksum>" }
```

```json
{ "durable": true, "checksum": "<the checksum>" } |
{ "changed": true, "checksum": "<the current checksum>" | null }
```

Under the document's lock, if the file still holds the bytes `checksum` names, Go
fsyncs the file, its page-state sidecar and their directories; the JS service
uses it to prove a checkpoint durable after an `indeterminate` write. `changed`
answers when the file holds other bytes or is not there, and changes nothing.

## `GET /v1/op/<opId>`

```json
{ "state": "unknown" } |
{ "state": "running" } |
{ "state": "committed", "checksum": "…", "cursor": "<epoch>:<seq>" } |
{ "state": "refused",   "cursor": "<epoch>:<seq>" } |
{ "state": "indeterminate", "checksum": "…", "cursor": "<epoch>:<seq>" }
```

`unknown` is any opId this process never saw. A caller's opId is 16 to 64
lowercase hex (400 otherwise) and new to this process: a reused one is refused
`409 {"error":"op-reused"}` while its outcome is kept. With none, Go makes one.
Outcomes are kept for this process only: the last 10,000, or 10 minutes,
whichever is smaller. Commits and restructures and page-state operations are all
recorded; an operation that failed is recorded as `refused`.

## `GET /v1/events?since=<cursor>`

Newline-delimited JSON frames. A cursor is `<epoch>:<seq>`: the epoch is 16
random hex characters made once at process start, and `seq` counts frames from
1. An empty or absent `since` streams from now, replaying nothing.

Frames, in the order they happened:

```json
{ "type": "change", "root": "root-0", "event": "add" | "addDir" | "change" | "unlink" | "unlinkDir",
  "rel": "a.html", "cursor": "<epoch>:<seq>" }
{ "type": "commit", "root": "root-0", "kind": "browser-save", "rel": "a.html",
  "outcome": "committed" | "refused" | "indeterminate",
  "checksum": null | "…", "opId": "…", "cursor": "<epoch>:<seq>" }
{ "type": "gap", "cursor": "<epoch>:<seq>" }
```

- `change` frames are the watcher's observations of a served root.
- `commit` frames are every write that reached the commit core, from any route:
  `/v1/commit` with the caller's opId, and HTTP writes (`/_/save`, a version
  restore, a guard restore, and every later writer) with an opId Go made up. The
  frame's `checksum` is the lane's checksum when it computes one, else the etag
  it stamped.
- `gap` comes first, before any other frame, when `since` names another epoch
  (a different process) or a sequence older than the ring still holds. Its
  `cursor` is where the stream stands when the client reconnects, and the
  frames that follow start from the oldest one the ring still holds.

The ring keeps the last 10,000 frames. An idle stream writes `{}` every 25
seconds. The stream ends when the client disconnects or the process stops.

Every `change` and `commit` frame names its `root` (the READY line's id), and its
`rel` is relative to that root. The ring is one per process, so a client for one
root ignores frames whose `root` is another's.

## The watcher

Every root is polled every 100 ms (`SYNC_CONFIG.FILE_STABILIZATION`), and a path
is reported only after it has looked the same for 1000 ms, which is
`awaitWriteFinish` in Local's `root-observer.js`. The ignored paths are Local's
`IGNORED`: any leading-dot file or folder anywhere (`.hyperclay`, `.trash`,
`.DS_Store`), `node_modules`, `sites-versions`, `tailwindcss` and `Thumbs.db`.

A changed document is judged by the data-loss guard's outside-edit pass from
G7a, with external provenance, exactly as `root-observer.js` calls it: if the
bytes on disk are not the ones this host last committed there, the guard runs on
those bytes. A write this host made is therefore one `change` frame and no guard
pass; a document that loses data is one `change` frame and a guard event on
disk. The `kind: 'external'` live emission of Local's observer belongs to G9's
live hook, which the native stage does not attach.

## The full-host stage: the JS service

`--bootstrap-stdin` (instead of `--control-fd`, never both) reads one JSON line
from stdin before serving: `{"v":1,"secret":"<64 hex>","service":{"url":"http://127.0.0.1:<port>","secret":"<64 hex>","epoch":"<16 hex>"}|null}`.
The rest of stdin is unchanged: EOF still shuts the host down. READY gains
`"epoch"`, the cursor epoch. With a service attached, Go's own live hub
publishes nothing: every notification becomes a `notice` frame,
`{"type":"notice","root","rel","payload":<Local's notify payload>,"cursor"}`,
and saves are published by the service. A request carrying `X-Trial-Epoch`
must come from the attached service (`X-Trial-Peer-Epoch` is Go's epoch,
`X-Trial-Epoch` the service's), else `409 {"error":"epoch","epoch":"<Go's>"}`;
requests without it (the supervisor) are unchanged. `POST /v1/peer` replaces
the service's address after the supervisor restarts it. A page document Go
wrote without the service confirming it is listed by `GET /v1/unconfirmed`,
announced once by an `{"type":"unconfirmed","root","rel","cursor"}` frame, and
lifted by `POST /v1/reconciled {root, name}`.

A consultation (`consult.go`) sends `admit` with `current` (the `/v1/read`
snapshot read under the held lock), `versions` (the version files a stale-save
rebase reads) and the five lane headers, every one present; runs artifact
batches until an answer is not pending (at most 16); writes `safety-version`
beside the document's versions and `recovered-copy` under the named root's
`.hyperclay/recovered/`; and sends `settle` with the written file's `identity`.
A failed settle after the bytes landed marks the document unconfirmed and
retries `abort settle-failed`.

With a service attached, a save-lane write (`browser-save`, `data-api`) to a
page document asks `admit` before its first backup and stores the service's
`logical` and `file` (no `canonicalForWrite`, AD2 refusal or `splitWrite` of its
own), then `settle`s after the rename and the etag record and publishes nothing
itself. A service refusal is answered with its own status, body and headers; an
unreachable service or an unconfirmed document is
`503 {"reason":"service-unavailable"|"unconfirmed","retryable":true}` with
`Retry-After: 1`, and nothing is written.

The other write lanes consult the service the same way:

- a `replace` write (`restore`, `guard-restore`) to a page document asks `admit`
  with `edit` and `safety` and its request kind as `origin`, stores the service's
  `logical` and `file` (no normalization, no safety backup and no `splitWrite` of
  its own; a `safety-version` artifact is written before its own version backup),
  then `settle`s with sender `server-save`;
- an `external-rebase` write asks `admit` (the service recovers the room's
  unsaved edits as artifacts) with origin `external`, writes the answered bytes,
  then `settle`s with sender `file-watcher`;
- a `sync-download` write to a page document asks no `admit`, and `settle`s once
  the bytes and the mtime are on disk with origin `sync-download` and sender
  `sync-engine`; a failed settle answers `indeterminate`;
- a `create` write to a page document asks no `admit`, and `settle`s with origin
  `create` and sender `server-save`; a failed settle answers `indeterminate`.

The watcher's observations reach the service as `/v1/observe`: after its guard
pass, for every page-document `add`, `change` and `unlink` the watcher reports,
Go takes the document's lock, reads the `/v1/read` snapshot and sends `observe`
with that `current` and the event. Every reported event is sent, the ones its
own writes cause included: the service judges its own confirmed writes `none`
from the authority's stamp, so Go keeps no record of them. A `rebase` answer
runs the `external-rebase` lane under the same lock with `expected` the
snapshot's `checksum`, so an edit that landed meanwhile refuses `changed` and is
observed on its own next poll. An unconfirmed document is skipped (the service
reconciles it from a fresh read), and a failure is logged: the watcher has no
caller to answer.
