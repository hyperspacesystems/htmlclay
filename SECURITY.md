# Security

HTML Clay has two different security boundaries. Keep them separate:

1. **The HTML Clay server limits what a page can read and save through HTML Clay.** A page normally changes only itself. A trusted folder is the exception: every HTML Clay file in it can read named files in the folder and can acquire the save capability for every other HTML Clay file in it.
2. **A registered helper program runs as you.** It has your access to files, processes, environment variables, and the network. HTML Clay decides whether to start the program. It does not sandbox the program or constrain what the program does after it starts.

A document is not the program being approved. A document asks for a helper by a short name. You select the program when it is registered, and the stored document decision points to that program's local ID. HTML Clay does not let the document supply an executable path or command line, but the document supplies the request on the program's standard input. The program must treat every request as hostile input.

## Registered helper programs

A document declares helpers in `<meta name="htmlclay-helper">` elements near the start of its `<head>`. HTML Clay reads at most the first 512 KiB and accepts at most eight valid, distinct names. Later declarations are silently ignored.

The decision usually happens when HTML Clay receives a direct file open, before that page's script runs. For every name with no stored decision, HTML Clay raises one native dialog. If a name is still unanswered when the page sends its first `wire/request` for it, the dispatcher parks the request and asks the app to raise the same dialog for that one name. That covers a dialog that was closed, a file picker that was cancelled, and a document reached by a link or a reload. A `wire/describe` never asks, and neither does a malformed request. Requests for the same name share one dialog. After any dialog for the document that granted nothing, the page cannot raise another for one minute. After a Deny, the page cannot raise one at all until the next direct open. A denied helper call fails immediately, without a dialog.

A linked `.htmlclay` file opened inside a trusted folder registers without a prompt. It gets the helpers its existing allows already cover, and a name without one is asked about the first time the page uses it.

A document nobody vouches for (see "Trust follows the code" below) shows its warning first. Until the warning is answered with Allow, no stored allow starts a program for it, not even one for any document.

The dialog offers two affirmative choices. Neither is a one-time grant:

- **"Allow for This Document"** allows the selected program for that one document, across reloads and restarts.
- **"Allow for Any Document"** allows the selected program for every document that declares the same helper name. It is not limited to one folder.

The tray's row for a program turns any-document access back off and forgets the documents allowed to use it.

The prompt shows, for each name, the path of the program that would run, and marks a registered path that no longer exists as missing. A name with no registered program says so, and an Allow opens a file picker to choose one. When several registered programs share a name, the prompt lists every path and an Allow opens the file picker to choose which one runs; picking a file already registered under that name reuses its registration. HTML Clay never chooses between them itself: if more than one of them is allowed for any document, the name stays undecided until a dialog asks. The saved decision identifies one concrete program by its local ID.

A refusal is never stored. Deny refuses the helpers for the rest of this open, including across tray changes, navigation and the page's own requests, and the next direct open asks again. Deny is the keyboard default on macOS and Windows, so Return is a Deny there, and with zenity, Escape and closing the dialog are too. Everything else grants nothing and stores nothing: Escape and closing the dialog elsewhere, the 120 second timeout, a cancelled file picker, a missing dialog tool and any dialog error. The page's next request asks again a minute later. Return and Escape never grant access. Refusals stored by 1.9.x are dropped when HTML Clay starts; if exactly one program with that name is allowed for any document, a document that had refused it now uses it.

### What approval gives the program

For each request, HTML Clay starts the registered path with the document's folder as its working directory. It passes the full request as JSON on standard input, inherits the app's environment, and supplies the document path and request ID in environment variables. On macOS and Linux it also supplies a login shell `PATH` so ordinary script interpreters can be found.

The five minute execution deadline, the structured output parser, and the result size limits protect HTML Clay from an unbounded request or malformed response. They do not restrict the program's operating system access. A helper can, among other things:

- Read, create, change, or delete any file your account can access, including files outside the document folder and files HTML Clay itself refuses to serve.
- List directories, start other programs, and leave descendant processes running.
- Use the network and send away anything it can read.
- Read or change HTML Clay's `config.json`, including its program registry and permission decisions. The file is mode 0600, but the helper runs as the same account that owns it.

HTML Clay permits up to eight concurrent helper requests per document. This is not an app wide limit. Several documents can each run eight children, so the limit is not resource isolation.

Register only programs you trust with your account. The security boundary inside a helper is what that program does with hostile standard input. Validate the operation, types, paths, sizes, and every other value before using them. HTML Clay validates the helper's output protocol. It cannot validate the meaning of the input for the program.

HTML Clay stores the selected program path, not a hash or a pinned file identity. Replacing the program at that path, or changing the target behind a selected symlink, changes what runs without another prompt. Whoever can update that program controls the code future requests execute.

### Who can invoke an approved program

The dispatcher accepts only names declared by that document and allowed in its stored decisions. Helper wire sends and subscriptions also require that document's save token. Another HTML Clay origin and a local process without the token are refused.

Files in one trusted folder share an origin. A hostile file in that folder can open an approved sibling in a visible top level tab, read the sibling's save token through their shared origin, and use that token to reach the sibling's approved helper. Document specific approval therefore does not isolate a helper from hostile files in the same trusted folder.

When a document has a helper dispatcher, both live sync relay addresses refuse page supplied updates to that target, on both relay lanes. This prevents a sibling from silently pushing a script into an already open, helper enabled tab. It does not stop the visible navigation and token path above, and it does not limit the helper after execution starts.

### The programs folder

With `htmlclay developer-mode on` or `htmlclay programs enable`, HTML Clay also runs programs from `~/htmlclay/programs/` without registering them. A page's helper name `search` finds `~/htmlclay/programs/search`, or the same name with one of `.sh`, `.py`, `.mjs`, `.js`, `.rb` or `.pl` (`.exe`, `.cmd` or `.bat` on Windows). Exactly one file may match: two matches count as none. On macOS and Linux the file must be executable. A file that carries a download mark never runs, and neither does a script interpreter placed or linked there under a program's name.

A program in the folder answers your own documents and signed ecosystem apps with no prompt. Any other document reaches it only when one of the program's lines in its first 4 KiB reads `htmlclay-callers: any`, and then only after that document's warning or team question was answered with Allow for this document. Once it starts, a folder program has the same access as a registered program.

`htmlclay programs disable` turns the folder off. Turning developer mode off turns it off too, unless `htmlclay programs enable` turned it on separately.

### Removing or revoking a program

Removing a program or its document permissions stops future dispatch. HTML Clay advances the document's helper generation, rejects late output, and cancels work it is tracking.

That cancellation is not containment. The runner signals only the direct child process and waits for it. Descendants can survive, including descendants that keep output pipes open. Process group cleanup, if added, would be tidying rather than a security boundary because a program running as the user can escape the group or arrange work elsewhere.

Removing or revoking a program does not undo writes, recover data already sent away, or stop work that escaped the tracked process. It also cannot protect the registry from that program, because the registry is owned by the same account.

Removing a program leaves every document decision that pointed at it in place. Each one reads as undecided while the program is gone, so the next open asks again, and restoring the same program ID makes those decisions grant once more. Registering a new program under the same display name does not inherit them: registration mints a new ID, and the stored decisions name the old one.

## AI editing

AI editing is built in and on by default. A page that loads ClayJS sends the selected element, the selected text and your request. HTML Clay starts an agent CLI on this computer and sends the agent's reply back to the page, which shows it until you click Keep or Revert. It needs no `<meta name="htmlclay-helper">` declaration and no approval prompt, and it never takes the document's handler slot. Turn it off with **AI Editing** in the tray menu. Turning it off cancels any edit that is running.

- **The agent runs with no tools.** Claude Code (the default) and Fable run with `--tools ''`, `--strict-mcp-config` and one turn, in an empty scratch folder. Codex runs in a read-only sandbox with its shell, app, browser, plugin and image features disabled, in an empty scratch folder. agy is refused, because it can read files without asking and has no switch to stop that. An engine you define yourself under `aiEdit.engines` in `config.json` runs exactly the command you wrote, as you, and HTML Clay does not restrict it. It runs in the document's folder only for a document that may attach other files (below), and in an empty scratch folder for any other.
- **What leaves the machine.** The agent CLI sends its prompt to its model provider: the element's HTML, the selected text, your request, any file you named with `@name.ext`, and with `@page` the whole saved document. A `@name.ext` file must resolve, after symlinks, to a regular file inside the document's folder. At most 8 files, 256 KiB each and 1 MiB in total. Only a document you made or trusted, or a signed ecosystem app, may attach other files: any other document's request carrying `@name.ext` is refused before the agent starts.
- **Documents nobody vouches for ask first.** A downloaded document, a document whose signature does not match its code, or a local one HTML Clay never recorded as yours shows one warning before it can read its folder or use AI editing, built-in programs or helper programs. The warning names the folder the page could reach. Allow for this document lets this version of its code run from then on, and a change to its code asks again. Trust it like a file I made records it as yours. Don't allow is remembered until `htmlclay untrust <file>`; closing the warning without an answer stores nothing. A document in a team folder asks once for each change a teammate makes to its code; editing its data never asks.
- **Who can ask.** A request needs the document's `Save-Token`, the same capability that saves the document. One AI edit runs per document at a time, for at most five minutes.
- **What comes back.** The reply goes to the page and nowhere else. The page refuses a reply that adds a script, an inline event handler, a `javascript:` URL, `srcdoc` or an embedding element, and nothing is saved until you click Keep.

## What HTML Clay itself protects

These protections apply to requests handled by HTML Clay. They are not a sandbox around registered programs.

- **Only files you choose can be saved through HTML Clay.** That choice takes two forms: a file you open yourself, and any HTML Clay file inside a folder you have trusted. Everything else, including a file reached by a link, an iframe, or a typed URL, is served read only and never gets a save token.
- **A page can only read through HTML Clay inside the folder it was opened from.** A request outside that folder pauses and asks you, in a dialog HTML Clay draws itself, to widen reads to one named folder.
- **Each project gets its own origin.** HTML Clay listens on one local port and serves each trusted folder at its own hostname, such as `htmlclay1.localhost`. Each folder containing an opened loose file, and each loose file in your home folder, also gets its own hostname. A loose document nobody vouches for gets a hostname of its own, shared with no other file, and opening it grants no read access to its folder. Inside a trusted folder, a downloaded document, or one whose signature does not count, gets a hostname of its own the same way, so its page cannot read the folder. When another hostname reaches such a document as a file, in a frame or through a link, it serves the page sandboxed: its scripts do not run and it has no origin to act as. The same holds for a downloaded file of any other type a browser shows as a page, such as XML. The browser therefore treats unrelated projects as different sites, with separate storage. In a served folder, a stranger's page is served with its scripts off until you allow it.
- **Trust follows the code, not the data.** HTML Clay hashes a document's code set: its scripts, event handlers, embeds and script URLs, never its data. Editing data never changes whose document it is. A document is yours when you made it in HTML Clay, answered Trust it like a file I made, ran htmlclay trust on it, or had it before this version (opened, or unchanged in a folder you trusted) with no download mark. Any document with exactly the same code as one you trusted with Trust it like a file I made or htmlclay trust counts as yours too. With `htmlclay developer-mode on`, any local file with no download mark is yours when HTML Clay first sees it. A signed ecosystem app is trusted by its signature. A signature counts only when every script the page loads from another site carries an `integrity` hash, or comes from `https://clayjs.com` or `https://hyperclay.com`. A document whose signature does not count is treated as a stranger's, even with developer mode on. HTML Clay records whether a document carried a download mark the first time it saw it, and its saves keep the mark.
- **Save tokens ride only on real navigations.** A silent background `fetch()` of another file receives no save token. Getting a sibling's token requires a visible top level document navigation.
- **The read only banner cannot select another file.** Its single use code resolves on the server to the exact file that was served. The code expires after ten minutes and dies on first use.
- **Always refused, trusted or not:** anything outside your home folder, dotfiles and dot-directories such as `.env`, `.git`, and `.ssh`, HTML Clay's own settings and version history, and directory listings.
- **Other websites cannot drive the server.** It listens only on loopback, validates the `Host` header, rejects cross site browser requests, and requires same origin browser attestation on routes that save, restore, upload, relay live updates, or change folder permissions.
- **Permission prompts are native.** Page content cannot draw, restyle, or click them.
- **Reads are judged by the file actually opened.** HTML Clay checks the real path reported for the open handle, so swapping a symlink during a request cannot redirect the read into its own settings or version history.
- **A refused read reveals no target path.** Out of scope denials use one fixed response and are decided before checking whether the requested file exists.
- **JSON data routes use the same read checks.** The `?data=` and `/_/api/` routes cannot reach a file an ordinary request could not, never supply a save token, and strip any token that was written to disk.
- **JSON writes are content only and follow save rules.** `POST /_/api/` writes only to a file HTML Clay could already save: one you opened, or an HTML Clay file in a trusted folder. A browser page must present that file's own save token, so a silent background `fetch()` still cannot write a sibling. The write cannot add script: event handlers, `javascript:`, `vbscript:` and `data:` URLs, HTML through `@innerHTML` or `@outerHTML`, and anything inside `<script>`, `<style>` or `<template>` are refused, and a refused write writes nothing. A local program needs no token, as it could already write the file directly.

## Trusted folders

A trusted folder is HTML Clay's durable write grant. Every HTML Clay file in it, now or later, can become editable without another prompt and can follow links to other HTML Clay files in the folder.

Plainly: **one hostile HTML file inside a trusted folder can rewrite every other HTML Clay file in that folder.** It can open each sibling in a visible tab, read that page's save token through the shared origin, and overwrite it. A silent background fetch still receives no token, so this path is visible, but it is possible.

Version history records saves made through HTML Clay and normally keeps at least the last 20 versions plus everything from the last 60 days. It helps recover from page based overwrites. It is not protection from a helper, which can change or delete the version store with the user's file permissions.

Untrusting a folder closes that origin, revokes registrations created only through the trust, closes their live streams, and rechecks any later save. Files you opened yourself remain open on another origin because opening a file was a separate decision. Stored helper decisions for those files remain separate from folder trust.

Each trusted folder is normally pinned to the folder identity reported by the operating system. If the folder is deleted, replaced, or changed into a link to another folder, the entry stops granting and appears as missing or replaced. If the filesystem cannot supply a stable identity, HTML Clay falls back to treating the stored path as the identity.

## Known limitations

1. **A trusted folder trusts everything in it.** A hostile file can read that tree, send data away, overwrite sibling HTML Clay files, and reach a sibling's approved helper after acquiring its token through a visible navigation.
2. **A page can steer a read permission prompt.** It chooses the requested path. Read the full path before allowing it. Page steered trust refuses your main personal folders and everything inside them. You can still choose a folder deliberately from the tray.
3. **The Windows prompts still need native verification.** They are covered by tests and fail closed when a dialog process fails, but they have not run on Windows for this release. The Linux zenity and kdialog prompts were exercised under Xvfb.
4. **Mixed capitalization across mounted filesystems is not fully handled.** HTML Clay detects the rule used by the home filesystem. A mounted filesystem with the opposite rule can still be compared incorrectly.
5. **Unicode equivalent folder names can briefly produce duplicate trusted entries.** Load normalization merges entries that the operating system reports as the same directory, but two equivalent spellings added during one run can coexist until restart. Remove every duplicate row if you see one.
6. **kdialog shows program and folder choices as a list.** With kdialog and no zenity, the permission prompts are radiolists whose first row is Deny, and program management's first row is No change, because kdialog's Return always picks the highlighted row.
7. **Hard links are not a boundary against a local attacker.** Someone who can already create hard links inside your home folder can link files into an allowed tree. That attacker already has local file access.
8. **Inside a folder you allowed or trusted, a page can tell which files exist.** That follows from read access.
9. **A hostile page can cause repeated prompts for invented folders.** Allowing an invented folder grants nothing. Denying suppresses that branch for the session, but the page can invent other names.
10. **A fresh helper subscription may not recover a missed outcome.** Terminal result retention is bounded. Do not automatically repeat a helper request merely because its result was lost, since the program may already have completed side effects.
11. **A stranger's document that reaches a trusted folder without a download mark shares the folder's origin.** Inside a trusted folder, HTML Clay gives a stranger's file its own origin only when it carries a download mark or a signature that does not count. A file fetched with `curl`, unpacked by a tool that drops the mark, or copied from a USB drive carries no mark, so its page can read the folder like your own files. It still shows the warning before AI editing or programs run. Keep files from other people out of trusted folders.
12. **Documents opened before this version count as yours if they carry no download mark.** Earlier versions erased the mark on every save, so a download that an older version saved may have lost its mark. Run `htmlclay untrust <file>` for one you did not make.

## Reporting a problem

Email david@storylog.com with what you found and how to reproduce it. Please do not open a public issue for a security problem until it is fixed.
