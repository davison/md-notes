# Introduction

md-notes is a local service that turns folders of markdown files into a notes
application in the browser. This page describes what exists and works today, at the
end of [milestone two](milestones/2-editor-autosave-and-live-update.md): the daemon
and the rendered viewer from
[milestone one](milestones/1-daemon-and-rendered-viewer.md), and now an editor over
the same notes. Notes are edited in place; creating, renaming and deleting them is
still done with other tools. The daemon side of the browser clipper is now here too
— a bearer token and a clip endpoint — but the extension that uses them, and the
inbox, are not built yet. The daemon can also be reached from another node on the
tailnet, behind `tailscale serve` and a login page.

## The daemon

One static Go binary, `mdn`, with the web UI compiled into it. It listens on the
loopback address only and serves every root it knows about.

```
mdn serve                run the daemon against the configured notes root
mdn open DIR             register DIR with the running daemon and open it
mdn token                print the bearer token (--rotate replaces it)
mdn version              print the version
```

`mdn serve` takes `--config FILE` (default `~/.config/mdn/config.yml`),
`--root DIR`, `--port N`, `--max-watches N` and `--tailnet-host NAME` to override what
the file says, `--state FILE` (default `~/.local/state/mdn/roots.json`) for where
folders added with `mdn open` are remembered, and `--token-file FILE` (default
`~/.local/state/mdn/token`) for the bearer token. It stops cleanly on `SIGINT` and
`SIGTERM`.

`mdn open DIR` takes `--config FILE`, `--port N`, and `--no-browser` to print the URL
instead of launching one. It resolves `DIR` against your working directory, POSTs it
to the running daemon and opens `/r/<slug>/`. If no daemon answers on the port it
prints how to start one and exits non-zero — it never starts a daemon itself.

`mdn token` prints the daemon's bearer token, creating it if the daemon has not run
yet, and takes `--token-file FILE` and `--rotate`. See
[Authentication](#authentication).

The configuration file holds five keys:

```yaml
notes_root: /home/you/notes
port: 7337
max_watches: 8192
clips_dir: clips
tailnet_host: laptop.tailnet-name.ts.net
```

A missing file is not an error as long as `--root` supplies the notes root. The
default port is 7337 and the default watch budget 8192 directories per root; `0`
removes the budget, and a negative value is refused. The Live update section below
says what the budget buys. `clips_dir` is where [clips](#clipping-a-web-page) land,
relative to the notes root, default `clips`; an absolute path, or one climbing out
of the notes root, is refused at startup. `tailnet_host` is the one extra `Host`
name the daemon answers to, for requests a `tailscale serve` proxy forwards to the
loopback port; it is empty by default, and everything under it must authenticate.
See [Reaching the daemon over the tailnet](#reaching-the-daemon-over-the-tailnet).
`contrib/mdn.service` is a systemd user unit that runs `mdn serve`.

ripgrep (`rg`) must be on `PATH` at runtime. It builds the navigator's file listing,
runs search, and decides which files the tag collector reads — which is how
gitignored and hidden files stay out of all three.

## Roots

A root is a folder the daemon serves, identified in URLs by a slug derived from its
basename and deduplicated with a numeric suffix. There are two kinds:

- The **notes root**, from `notes_root` or `--root`. It is permanent and always
  present.
- **Recent roots**, added by `mdn open`. They persist to the state file with their
  slugs, so a root's URL survives a restart. A recent root whose directory has since
  disappeared is dropped when the daemon next starts.

Registering a path that is already a root returns the existing root rather than
duplicating it; the comparison is on the real path, so a symlinked alias resolves to
the same root. A directory nested inside an existing root can still be registered as
a root of its own.

The home page at `/` lists the notes root under "Notes" and every recent root under
"Recent", each linking to its three-pane view.

## The HTTP API

All endpoints are on the loopback listener, and all of them are behind the guard
described under [Confinement](#confinement). Most of them are also reachable under a
configured `tailnet_host`, to an authenticated caller; `POST /api/roots` and
`POST /api/clip` are not, and the
[tailnet section](#reaching-the-daemon-over-the-tailnet) says why.

| Endpoint | What it does |
|----------|--------------|
| `GET /api/roots` | `{roots: [{slug, path, kind}]}` |
| `POST /api/roots` | Registers `{"path": "/absolute/dir"}` and returns the root. Relative paths are refused |
| `GET /api/r/{slug}/tree` | The root's markdown tree as nested `{name, path, dir, children}` |
| `GET /api/r/{slug}/note/{path...}` | A rendered note as `{path, title, frontmatter, html}`. Non-markdown paths are 404 here |
| `GET /api/r/{slug}/source/{path...}` | Existing UTF-8 markdown as `{source, revision}`; see [conditional saves](#conditional-saves) |
| `PUT /api/r/{slug}/source/{path...}` | Conditionally saves JSON `{source, revision}` and returns the saved `{source, revision}` |
| `GET /api/r/{slug}/raw/{path...}` | File bytes, for images and other assets. Served with `Content-Security-Policy: sandbox` and `X-Content-Type-Options: nosniff` |
| `GET /api/r/{slug}/search?q=` | `{hits, truncated}`; each hit is a path, line number, matching text with match offsets, and the lines either side |
| `GET /api/r/{slug}/tags` | `{tags: [{name, count, notes}]}`, sorted by count then name |
| `GET /api/r/{slug}/events` | A Server-Sent Events stream of change batches |
| `POST /api/clip` | Creates a note from `{url, title, markdown, kind}`; needs the token. See [Clipping a web page](#clipping-a-web-page) |

Everything else serves the embedded UI bundle, falling back to `index.html` so
client-side routes such as `/r/notes/some/note.md` load.

### Authentication

The daemon holds one bearer token per installation. It is generated the first time
`mdn serve` runs and stored, one line, at `~/.local/state/mdn/token`
(`$XDG_STATE_HOME/mdn/token`) with mode `0600` — beside the state file and kept as
private as it is. `mdn token` prints it, creating it if the daemon has not run yet,
so a client can be set up before the daemon is ever started:

```
$ mdn token
NGTBQLVJHJQOIYMIZH5UMRB2XK
$ mdn token --rotate
DK5T2ZHQ4WQKX3BYA7CJEUPMSN
```

`--rotate` replaces the token and prints the new one. A running daemon needs no
restart: it notices the file has been replaced and, from the next request, accepts
the new token and refuses the old one. Both commands take `--token-file FILE`, which
must name the same file the daemon was given.

The token file must be a **regular file**, not a symlink — reading it repairs its
permissions to `0600`, and that must not reach a file you did not nominate as the
token. A path that is a symlink is refused, and the daemon says so and stops rather
than following it; `mdn token --rotate` on that path replaces the link with a
regular file. If you keep state under version control, point `--token-file`
somewhere else rather than linking this one in — it is a secret, and it is the one
state file that should not be copied between machines.

A client presents it in an `Authorization` header:

```
Authorization: Bearer NGTBQLVJHJQOIYMIZH5UMRB2XK
```

What the token buys is the [Origin](#confinement) check: a request carrying it is
served whatever its `Origin`, which is how the browser extension writes from its
`chrome-extension://` origin. It buys nothing else — every endpoint that was
reachable without it still is, over loopback, exactly as before, and the web UI
sends no `Authorization` header at all. `POST /api/clip` is the one endpoint that
*requires* the token, because nothing on the daemon's own origin needs to call it.
Note that the token opens the *whole* API to the origin presenting it, not the clip
endpoint alone: a holder can register a root and read files under it.

The `Host` check is not waived by the token: DNS rebinding is a separate attack, and
a rebound page holds no token anyway.

**The caller must be one CORS does not govern.** An `Authorization` header makes a
cross-origin `fetch` non-simple, so a browser *page* sends `OPTIONS` first — and the
daemon refuses the preflight like any other cross-origin request and sends no
`Access-Control-Allow-Origin` on any response. That is deliberate: the intended
client is a Manifest V3 extension **service worker** holding `host_permissions` for
the daemon's origin, which is exempt from CORS and never preflights. A fetch from a
content script, a popup document or an ordinary web page therefore fails at the
preflight with the browser's opaque CORS error, whatever token it holds, and that is
the boundary — a page that has merely found the port must not be able to write, even
if it has somehow read the token. An extension does its clipping from the service
worker.

#### A browser on the tailnet

When `tailnet_host` is configured, a browser reaching the daemon under that name
cannot put the token in a header on every request, so it presents it once instead.
An unauthenticated navigation is answered with a login page — one self-contained
document, no script and no asset to fetch — which posts the token to `/login` and
gets back a cookie:

```
Set-Cookie: __Host-mdn_session=…; Path=/; Max-Age=2592000; HttpOnly; Secure; SameSite=Strict
```

The `__Host-` prefix makes the browser itself refuse the cookie unless it is
`Secure`, `Path=/` and carries no `Domain`, so it is bound to the one name that set
it. The value is a random session id, never the token; the daemon keeps only its
SHA-256 and the token *generation* the session was minted from, so `mdn token
--rotate` logs every device out on the next request, and so does restarting the
daemon. A session otherwise lasts 30 days.

`/login` exists only under `tailnet_host`. Over loopback it is an ordinary
client-side route and serves the UI, as it always did.

Authentication is checked when a request arrives, so an events stream already open
is not cut off by a rotation: it ends when the page reloads, when the daemon
restarts, or when the connection does. It carries the paths that changed and no
file content.

An unauthenticated request that is *not* a navigation — a `fetch`, the events
stream, anything under `/api/` — gets `401 {"code":"unauthorized"}` rather than a
login page it cannot read. An API client should send the `Authorization` header and
never see the page at all.

### Refusals

| Status | Code | What happened |
|--------|------|---------------|
| 403 | `bad_host` | The `Host` header is not `localhost` or `127.0.0.1` with the daemon's port |
| 403 | `cross_origin` | A foreign `Origin` and no token: present the token to write from another origin |
| 401 | `unauthorized` | An `Authorization` header that is not a valid `Bearer <token>` — a wrong token, a rotated-away one, or another scheme. Under `tailnet_host`, also a request that proved nothing at all |
| 403 | `loopback_only` | The endpoint is not reachable under `tailnet_host`. See [the tailnet section](#reaching-the-daemon-over-the-tailnet) |

The guard answers before any handler runs, and its refusals carry the same
`{code, error}` envelope and `Cache-Control: no-store` as the handlers below, so one
client handles both. The two a client tells apart in practice are `cross_origin` —
no token configured yet — and `unauthorized` — a token that is not the current one;
a stale token is never quietly treated as no token. The `401` carries a
`WWW-Authenticate: Bearer` challenge for the sake of generic HTTP clients; there is
no interactive login behind it.

### Conditional saves

Read the source endpoint before editing, retain its opaque `revision`, then PUT the
complete new `source` with that revision and `Content-Type: application/json`.
An empty source string is valid; omitted or null source is not. A successful save
returns HTTP 200 and the new revision to use for the next save. Source responses
use `Cache-Control: no-store`. The existing rendered and raw endpoints are unchanged.

The API edits existing `.md` and `.markdown` regular files only, in any registered
root. It preserves frontmatter, whitespace, line endings, Unicode and trailing
newlines exactly as submitted; it never parses or rewrites the frontmatter.
Editable source must be valid UTF-8 and at most 8 MiB. Invalid UTF-8 and unpaired
JSON Unicode surrogate escapes are rejected. The encoded JSON request limit allows
six bytes per source byte plus 1 KiB for the revision and object syntax.

Source errors return JSON `{code, error}` with these statuses:

| Status | Code | Client action |
|--------|------|---------------|
| 400 | `invalid_body` | Correct malformed JSON or missing/invalid fields |
| 403 | `outside_root`, `permission_denied` | Retain the draft; check the path or file/directory permissions |
| 404 | `not_found`, `not_markdown` | Retain the draft; the note/root is missing or the path is not markdown |
| 409 | `conflict` | Retain the draft and fetch current source before choosing how to reconcile |
| 413 | `too_large` | Source or request exceeds the size limit |
| 415 | `invalid_body` | Send `Content-Type: application/json` |
| 422 | `unsupported_source` | The file is nonregular or contains invalid UTF-8 |
| 428 | `revision_required` | Read the source first and include its revision |
| 500 | `io_error` | Retain the draft and retry after checking disk/storage health |

The Host/Origin guard can also return 403 with the existing `{error}` body before
the source handler runs. A failed save never supplies a replacement revision or
instructs the client to discard its draft. A lost HTTP response can leave a save's
outcome unknown; fetch the source and compare it with the retained draft before
retrying. There is no force-save or create-on-missing option.

Revisions track file identity, content, modification time and permissions. They
remain stable while the observed file is unchanged, including across reads through
symlink aliases and overlapping registered roots. They are local to one daemon
session; a restart makes an old token conflict, so clients must re-read. The daemon
keeps one small revision record per canonical path accessed during that session.

Source reads and saves are serialized within the daemon. Saves check the revision,
write and sync a temporary file in the same directory, check the current file again,
then replace it by rename. Stale browser saves, detected external edits/replacements,
and deleted notes are rejected. **An uncoordinated external editor can still write
between the final revision check and replacement.** Ordinary filesystem rename is
not atomic compare-and-swap; external tools do not participate in the daemon lock.

Internal symlink aliases update their resolved target without replacing the alias;
symlinks outside the registered root are refused. Open directory handles confine
staging and replacement despite symlink changes. Read-only files are refused even
when the directory permits replacement. Replacement preserves the file's nine Unix
permission bits, but creates a new inode owned by the daemon user: hard-link identity,
ownership, ACLs, extended attributes and other extended metadata are not preserved.
The temporary file is synced before rename; the parent directory is not synced, so
the API does not promise rename durability across power loss.

### Clipping a web page

`POST /api/clip` creates a note from a web clipping. It is the only endpoint that
creates a file rather than reading or replacing one, and the only one that requires
the [token](#authentication).

```
POST /api/clip
Authorization: Bearer <token>
Content-Type: application/json

{"url": "https://example.com/article",
 "title": "The Cost of Abstraction",
 "markdown": "# Heading\n\nBody text.\n",
 "kind": "page"}
```

`url`, `markdown` and `kind` are required; `kind` is `page` or `selection`; `title`
may be empty or absent. The response is `201` with the notes root's slug and the
path of the created note relative to that root, which together are the app URL
`/r/{root}/{path}`:

```json
{"root": "notes", "path": "clips/2026-09-10-the-cost-of-abstraction.md"}
```

The note lands under `clips_dir` inside the **notes** root — never a recent root —
and the directory is created if it is missing. The name is the clip's date and a
slug of its title, with `-2`, `-3` … appended if that name is taken, so an existing
note is never overwritten:

```
clips/2026-09-10-the-cost-of-abstraction.md
clips/2026-09-10-the-cost-of-abstraction-2.md
```

The slug is lower case and ASCII: accented Latin letters fold to the letter (`Café`
→ `cafe`), every other run of characters becomes a single hyphen, and the result is
trimmed to 64 characters at a hyphen. A title that yields nothing — an empty title,
or one written entirely in another script — gives `untitled`; the title itself is
still in the frontmatter.

The file is the frontmatter, a blank line, and the markdown byte for byte:

```
---
title: The Cost of Abstraction
source: https://example.com/article
clipped: "2026-09-10T14:05:00+01:00"
tags: [clip]
---

# Heading

Body text.
```

`clipped` is RFC 3339 in the daemon's local time zone, and the note is created
`0644` less the daemon's umask, like any other file it would write. The title is
carried as it was sent, with two exceptions: runs of whitespace, newlines included,
collapse to single spaces, and a title longer than 300 characters is cut to 300 — a
`<title>` that is a paragraph would otherwise make the header unreadable. Nothing is
added to the markdown — not even a trailing newline — and nothing is normalised. The new note
reaches every open page through the [events stream](#live-updates) like any other
new file; the very first clip of an installation may report the new `clips`
directory rather than the note itself, which refetches the tree just the same.

Clip errors use the same `{code, error}` envelope as
[conditional saves](#conditional-saves):

| Status | Code | Meaning |
|--------|------|---------|
| 400 | `invalid_body` | Malformed JSON, a missing or empty field, a relative `url`, or a `kind` that is neither `page` nor `selection` |
| 401 | `unauthorized` | An `Authorization` header that is not the current token |
| 403 | `cross_origin` | A foreign `Origin` and no `Authorization` header at all — the [guard](#refusals) answers, and the handler never runs |
| 403 | `outside_root` | `clips_dir` resolves outside the notes root |
| 403 | `permission_denied` | The clips directory is not writable |
| 409 | `conflict` | The dated name and every suffix are taken |
| 413 | `too_large` | The markdown or the request exceeds the size limit |
| 415 | `invalid_body` | Send `Content-Type: application/json` |
| 500 | `io_error` | The note could not be written |

The limits are the source API's: the markdown must be valid UTF-8 and at most 8 MiB,
unpaired JSON Unicode surrogate escapes are rejected, and the encoded request may be
six bytes per markdown byte plus 64 KiB for the URL, the title and the object
syntax. Responses carry `Cache-Control: no-store`.

### Live updates

The events stream opens with a `: connected` comment, sends `event: change` frames
whose data is `{"paths": [...]}`, and sends a keepalive comment every thirty seconds.
An empty batch means "refetch everything": it is what the daemon sends when a change
batch was lost, including on a kernel event queue overflow. `EventSource` reconnects
on its own, and the UI treats a reconnect as a full refresh too.

After the `: connected` comment the stream sends one `event: status` frame carrying
the root's watch coverage, and sends it again when the coverage has changed — on the
next keepalive tick, so up to thirty seconds later:

```json
{"watched": 8192, "unwatched": 4498, "budget": 8192,
 "overBudget": true, "failed": 0, "limited": true}
```

`limited` is `unwatched > 0`; `overBudget` says the budget rather than an error is
the reason; `failed` counts directories the kernel refused; `budget` is the per-root
maximum and is `0` when there is none, never negative. A root with no watcher at all
has no stream: the endpoint answers 503, which an `EventSource` reports by closing
for good rather than reconnecting.

## The web UI

A Preact application, three panes per root:

- **Navigator.** The markdown tree, with collapsible directories whose expansion is
  remembered per root in `localStorage`. The current note is highlighted and its
  ancestors are opened. Only markdown files and the directories containing them
  appear; `.md` and `.markdown` count, hidden entries do not, and everything ripgrep
  would ignore is absent.
- **Note.** The rendered note: title, a collapsed metadata panel holding the
  frontmatter, and the body. Notes render server-side as GitHub-flavoured markdown —
  tables, task lists, strikethrough, autolinks, footnotes, and fenced code
  highlighted by chroma — then pass through a sanitiser, so a note cannot run script
  on the app's origin, nor dress itself in the application's own CSS. The classes a
  note may carry are a whitelist: any name in the reserved `mdn-` namespace, which
  only the syntax highlighter emits and no application stylesheet uses; the
  `language-` class goldmark gives a fence it could not tokenise; and by exact name
  the few structural classes a note's own footnotes and links carry. Every other
  class is stripped, so what a note can style does not depend on what the
  application calls its own classes. Relative links to markdown become in-app
  navigation; relative images and other assets are served from the raw endpoint; a
  link whose target escapes the root keeps its text but loses its destination and
  says why. A bar above the note carries the mode, the save state, and `Ctrl+E`,
  which flips the pane to the editor and back — see [Editing](#editing).
- **Search and tags.** A debounced search box whose results group by file, showing
  the matching line with the match emphasised and a line of context either side.
  Selecting a hit opens the note with `?l=<line>` and scrolls to the block at that
  line, flashing it. Below it, the tag panel lists each tag with its count; selecting
  one sets `?tag=` and prunes the navigator to the notes carrying it, with a link to
  clear the filter.

Search is a literal, case-insensitive phrase — what you type is what is matched.
Tags come from a frontmatter `tags` value (a list, or one string split on commas and
whitespace) and from inline hashtags: `#` at the start of a line or after whitespace,
followed by letters, digits, `_`, `-` or `/`, containing at least one letter, outside
fenced and inline code, lower-cased. Tags are collected per request, with no index.

## Editing

Any note the daemon serves can be edited in place. `Ctrl+E` flips the note pane
between the rendered view and the editor, and back; the **Edit** / **View** button
in the note bar does the same by mouse. The key is handled once for the whole page
in the capture phase, so it works from the navigator and the search box as well as
from the editor, and it reaches the editor ahead of vim's own `Ctrl+E`, whose
scroll-one-line it shadows. Being a chord, it never fires on ordinary typing.

The editor is CodeMirror 6 with vim keybindings, markdown highlighting — including
fenced blocks in their own language — line wrapping and undo history. `Tab` indents
rather than moving focus, so `Ctrl+E` is the way out by keyboard and the mode button
the way out by mouse. The editor's state is parked on the note between flips, so
switching to the rendered view and back keeps the cursor position and the undo
history.

Every vim exit command saves, and none of them closes anything: `:w`, `:q`, `:q!`,
`:x` and `:wq` all mean "save now". There is no way to quit without saving, because
the editor never closes and autosave would have written the draft anyway.

Edits go to the original file through the [conditional save](#conditional-saves)
endpoint, so they are subject to the same root confinement and the same Host and
Origin guard as everything else, and the source is written back byte for byte —
frontmatter, hard tabs, trailing spaces, a byte-order mark and a missing final
newline all survive an edit untouched. The editor edits existing notes only: it
cannot create, rename or delete one, and the save API has no create-on-missing path.

Line endings survive too, with one qualification. CodeMirror splits a document on
any of the three endings and joins with LF, so the editor rejoins its text with the
ending the note itself uses most — CRLF, a lone CR, or LF, a tie going to LF, or to
CRLF when LF is not in the tie. A note that uses one ending throughout therefore
keeps every byte through an edit; a note that already *mixes* endings comes back
uniform in its dominant one the first time it is edited.

The editor is part of the UI bundle embedded in the binary, and most of it loads
whether or not you open it. The page pulls two JavaScript chunks — about 710 KB
together, being CodeMirror itself and the table of languages it can highlight — plus
11 KB of CSS, all of it before `Ctrl+E` is ever pressed. The daemon sets no
`Content-Encoding`, so that is what crosses loopback rather than a compressed third
of it, and no `Cache-Control`, `ETag` or `Last-Modified` either, so nothing is
cached: every page load fetches all of it again, and a conditional request is
answered with the whole file rather than a 304. Moving between notes does not repay
it — navigation inside the app is client-side and fetches only the note — but a
reload, a new tab, or a note URL opened directly does. Over loopback it costs
milliseconds. The per-language parsers for fenced code are separate chunks, one per
language, fetched when a note containing such a block is opened in the editor — not
when the block is typed in.

### Autosave and the save states

A save is sent one second after typing stops. Anything that means "I am done for
now" sends it immediately instead: switching to the rendered view, navigating to
another note, the window losing focus, the tab being hidden, `:w` and its
relatives, and closing the page.

The note bar shows where the draft stands:

| State | What it means |
|-------|---------------|
| `Saved` | The draft matches the file on disk |
| `Unsaved changes` | The draft differs and a save is scheduled |
| `Saving…` | A save is in flight |
| `Save failed: <reason>. Draft kept.`, with a **Retry** button | The daemon refused the save; the draft is untouched |
| `Conflict: draft kept` | The file moved on under the draft; see [Conflicts](#conflicts) |

A failed save keeps the draft and shows the reason the daemon gave — a read-only
file reports `note or directory is not readable/writable`; the rest are the codes
in [Conditional saves](#conditional-saves). **Retry** sends it again, and so does
the next edit; nothing is discarded in between. A save landing while the pane is in
view mode refreshes the rendered note.

### Conflicts

A conflict is raised when the file's *content* differs from what the draft was made
against, and only under a draft that is unsaved, in flight or failed. A new revision
over identical content — a `touch`, a permission change, a same-bytes rewrite, or
the daemon's own event for the save just made — is adopted silently, and a save the
daemon refused for such a revision is sent again, once, against the new one. A clean
note simply follows the file: it refreshes from disk in both the rendered view and
the editor.

A note **changed** on disk under a draft raises a banner offering three ways out:

- **Keep my draft** saves the draft over the file as it now is;
- **Load the file** drops the draft and takes the file's text;
- **Copy draft** puts the draft on the clipboard, to reconcile by hand.

Autosave stops while the conflict stands, the draft stays in the editor, and
switching to View shows the file as it is on disk with the banner and the draft
still there.

A note **deleted** on disk under a draft raises a banner with **Copy draft** and
**Discard draft** only. The editor cannot recreate the file, because the save API
has no create-on-missing path; recreating it with another tool turns the conflict
back into a changed one, where **Keep my draft** writes the draft over it. A clean
note whose file is deleted is kept the same way rather than dropped, since the text
on screen may be the only copy left.

### Drafts that outlive the page

Editing state lives outside the note pane, one session per note, so nothing is
dropped by a mode switch or by navigating away — both flush first, and anything that
does not land stays in its session. The top bar lists every *other* note holding
unsaved work, as links; following one opens that note straight into the editor with
its draft.

Closing the page with unsaved work sends a final save and asks the browser's "leave
site?" question. That last save is a `keepalive` request: the page cannot learn
whether it landed, and browsers cap such a request at 64 KiB of body, so a larger
draft's final save is refused outright.

For both cases an unsaved draft is also mirrored to `localStorage`, and the next
open reconciles it against the file: identical to it, the draft is dropped as
already saved; made against the file's current revision, it is restored and saved a
second later; made against an older one, it opens as a conflict. A note with a draft
waiting — in memory or in storage — opens in the editor rather than the rendered
view.

The mirror is one record per note, **shared by every tab on this origin**. It is
crash and reload recovery, not a second copy per tab: two tabs editing the same note
overwrite each other's record, and a second tab opening a note whose first tab holds
an unsaved draft adopts that draft from storage and saves it within a second.
Nothing is lost — the in-memory session, the top-bar list and the leave prompt are
the guarantee against that — and the two tabs converge on the saved text, the later
save of the two refused as stale and kept as a conflict.

## Live update

The daemon watches every registered root with fsnotify and pushes change batches to
every open page for that root, so creating, modifying, deleting or renaming a file
shows up without a refresh. Events are debounced into one batch per root — 150 ms
of quiet, or one second at the outside. The page
refetches the tree on any batch that could change it, and refetches the open note
when the batch names its path or a directory above it.

### Which directories are watched

The watched set has three parts, all of them found from one ripgrep listing:

- every directory holding a file ripgrep lists, and their ancestors;
- every directory holding a *placeholder* — a hidden file ripgrep lists, such as the
  `.gitkeep` that keeps an otherwise empty directory in a git repository — and their
  ancestors, at any depth;
- every subtree beneath those that holds no files at all.

The first note created in any of them is seen live, which is what a placeholder
directory exists for.

That listing is `rg --files --hidden --glob '!.*/'`: hidden files asked for, hidden
directories refused. Asking for hidden files does not disable ripgrep's ignore
rules, and that is the whole point — a `.gitkeep` no rule covers is named, while the
`.lock` files of a gitignored cache are not, so an ignored tree of dotfiles cannot
enter the set by looking empty. Refusing hidden directories costs nothing, because
they are never watched, and keeps the walk out of `.git` and `.cache`, which is
where the time would go: computing the watched set costs 6–9% more than the rule
this replaced, which asked ripgrep for no hidden files at all.

Ignored files are the limit of it. A directory whose only files are *ignored* holds
nothing ripgrep will name, hidden or not, so it is not watched, and a note created
there is seen when the directory next appears in a batch, or when the daemon
restarts. That is the remaining hole in live update's coverage of an ordinary root,
and it is the trade-off recorded in the
[milestone one document](milestones/1-daemon-and-rendered-viewer.md).

### The watch budget

The set grows with the tree, not with the notes in it, because a directory holding
files but no note can gain one later and is watched too. On a notes folder that
costs nothing; on a large ad-hoc root it is the dominant cost. Measured on one
machine with the budget removed, counting each directory by the group the listing
puts it in — the three groups below:

| root | watched | 1: holds a note, plus ancestors | 2: nothing to list | 3: files but no note |
| --- | --- | --- | --- | --- |
| a notes folder | 21 | 3 | 2 | 16 |
| `~/projects` | 1,614 | 642 | 30 | 942 |
| `/usr/share` | 5,814 | 311 | 706 | 4,797 |
| `/usr/lib` | 12,700 | 303 | 33 | 12,364 |
| `~/.cache` | 82,874 | 13,616 | 559 | 68,699 |

Group two is small because a directory that holds no file of its own but has
file-holding descendants is an ancestor, and ancestors go in the group of what they
lead to. QA measured `/usr/share` at 5,790 against 310 navigator directories before
this task ([#13](https://github.com/davison/md-notes/issues/13)); watching
hidden-only directories accounts for the difference.

An inotify watch costs about a kilobyte of
unswappable kernel memory and comes from
a per-user pool — `fs.inotify.max_user_watches`, commonly 524,288 — shared with
every editor, IDE and file manager the user is running. `~/.cache` alone would take
16% of that pool, some 83 MB, for one registered root.

So each root has a budget of `max_watches` directories, 8192 by default: enough to
cover `/usr/share` whole, a sixty-fourth of a typical desktop's pool, and equal to
the smallest limit still shipped, so one ad-hoc root cannot exhaust an unraised
system on its own. Watches are placed in priority order, so a root larger than its
budget keeps the ones worth most:

1. the root, every directory holding a markdown file, and their ancestors — the set
   the navigator lists, where notes change;
2. directories holding nothing the navigator would list, where a first note can
   appear that nothing else would report;
3. everything else — directories holding files but no note.

The third group is 83–97% of the watch set on every large root above — 58% on
`~/projects`, which is mostly notes — so it is what a spent budget gives up first.

The order holds after startup too. When a directory appears — or gains its first
note — on a root whose budget is already spent, the least valuable watch is released
to make room for it: a directory the listing no longer names, then the lowest group,
then the last path within that group. Directories of equal rank never displace each
other, so a settled root does not churn its watches, and the root's own watch is
never given up.

An unwatched directory hides nothing permanently: its changes still arrive when a
watched directory reports them, when the tree is refetched for another reason, and
when the daemon restarts.

### When coverage is limited

A spent budget and a kernel out of watches mean the same thing to a reader — part of
the root is not live — so they are one report:

- the daemon logs one line per root, whatever the number of directories behind it,
  naming the root, how many directories are covered, how many are not, why, and
  which limit to raise;
- the root's event stream opens with a `status` event carrying the same numbers, and
  sends it again on the next keepalive tick after they change;
- the page shows a notice above the navigator whenever coverage is limited, naming
  whichever limits are in play, so it is visible in the browser and not only in the
  daemon's log.

A root whose watcher never started is the same story with nothing covered: its event
stream answers 503, and the page says live update is not available for that root and
that changes show up on reload.

None of this stops the daemon, and none of it disables live update for the rest of a
root. To cover a large root completely, raise `max_watches` (or set it to `0` for no
budget) and raise the kernel's own limit to match:

```
sudo sysctl fs.inotify.max_user_watches=524288
echo fs.inotify.max_user_watches=524288 | sudo tee /etc/sysctl.d/90-mdn.conf
```

## Reaching the daemon over the tailnet

The daemon still binds `127.0.0.1` and nothing else. To read your notes from
another node on your tailnet, put `tailscale serve` in front: it terminates TLS on
the machine's own tailnet name and proxies to the loopback port, and the tailnet
ACL decides who may reach it.

First tell the daemon the name it will be reached by, in `~/.config/mdn/config.yml`:

```yaml
tailnet_host: laptop.tailnet-name.ts.net
```

or `mdn serve --tailnet-host laptop.tailnet-name.ts.net`. The name is your machine's
MagicDNS name; `tailscale status --json | jq -r .Self.DNSName` prints it with a
trailing dot, which you may keep or drop — the daemon stores it without, since that
is what a browser puts in the `Host`. A name carrying a scheme, a path or a port
that is not a number is refused at startup rather than left never to match, and so
is a loopback name — loopback already works and is deliberately left alone.

Then, on the machine running the daemon, with **HTTPS Certificates** and **MagicDNS**
enabled for the tailnet in the Tailscale admin console:

```
tailscale serve --bg 7337
```

That is the whole command. It prints the URL it is serving and the configuration
persists across restarts. `tailscale serve status` shows what is set up and
`tailscale serve --https=443 off` takes it down again. (This is the `serve` syntax
of recent Tailscale releases; an older one spells the same thing differently, and
`tailscale serve --help` will say how.)

Use `serve`, **not** `tailscale funnel`: funnel publishes to the whole internet,
where the single-user premise below does not hold at all and one bearer token is the
only thing between a stranger and your notes.

Serving on a port other than 443 works — `tailscale serve --bg --https=8443 7337` —
but the browser then sends `laptop.tailnet-name.ts.net:8443` as the `Host`, so
`tailnet_host` must carry the port too.

### What the proxy forwards, and what the daemon does with it

| Header | Set by | Used for |
|--------|--------|----------|
| `Host` | the browser, passed through | matched against `tailnet_host`; this is what makes the guard let the request through |
| `X-Forwarded-Proto: https` | `tailscale serve` | the login page refuses to set a `Secure` cookie the browser would discard, and says so, rather than looping |
| `X-Forwarded-For` | `tailscale serve` | the tailnet address in the daemon's log line for each login |
| `Tailscale-User-Login` and friends | `tailscale serve` | **nothing.** The daemon reads no identity header |

Those headers are trusted only because the listener is loopback: the only things
that can set them are the proxy and a process already running as you. The identity
headers are deliberately unused: the capture this work adopts
([#10](https://github.com/davison/md-notes/issues/10)) asked for one authentication
path designed once for both the browser extension and the tailnet, and that is the
token.

### What is reachable under that name, and what is not

Everything under `tailnet_host` must authenticate: the `Authorization` header for an
API client, the [login page and session cookie](#a-browser-on-the-tailnet) for a
browser. Nothing is served without one.

What an authenticated caller reaches is the UI's own API and nothing else:

| Reachable | Not reachable |
|-----------|---------------|
| `GET /api/roots` | `POST /api/roots` |
| the per-root reads — `tree`, `note`, `source`, `raw`, `search`, `tags`, `events` | `POST /api/clip` |
| `PUT /api/r/{slug}/source/{path…}` | anything else under `/api/` |
| the UI bundle and its client-side routes | |

Anything on the right answers `403 {"code":"loopback_only"}`. It is an allow-list,
not those two exclusions, so an endpoint added later is loopback-only until somebody
decides otherwise.

`POST /api/roots` is the one that matters: with it, a caller holding the credential
could register any directory on the machine and then read every file under it
through the raw endpoint. On loopback that is inside the premise below — anything
that can reach the port runs as you and can read those files anyway. Over the
tailnet it is not, so it stays on the machine. `POST /api/clip` is refused because
nothing off the machine clips: the extension's daemon URL is `http://localhost:7337`
and it runs where the notes are.

So a remote device reads, searches, and edits the notes the daemon already serves.
It cannot add a root, and `mdn open` remains a command for the daemon's own machine.

### The premise, restated

On loopback the daemon assumes a single-user machine: everything that can reach the
port already runs as the user who owns the notes. Under `tailnet_host` that is no
longer who is on the other end. The people and devices your **tailnet ACL admits to
this node** can reach the login page, and one of them holding the token can read,
search and edit every root the daemon serves — the notes root and every folder added
with `mdn open`, including any that was only ever meant to be looked at locally.

So: keep the ACL as narrow as the notes deserve, ideally to your own devices; use
`serve` rather than `funnel`; and treat `mdn token --rotate` as the way to revoke a
device, since it ends every session and every stored token at once. The token is
still a single secret shared by every client, which is the shape M3-R1 fixed and
this milestone does not change.

## Confinement

- The listener binds `127.0.0.1` and nothing else, whether or not `tailnet_host` is
  configured. Reach from the tailnet comes from a proxy in front, never from a
  second listener.
- The `Host` header must be `localhost` or `127.0.0.1` with the daemon's port, which
  defeats DNS rebinding, or the configured `tailnet_host`. The allow-list grows by
  that one name and no other.
- An `Origin` header, if present, must be the daemon's own origin — unless the
  request carries the [bearer token](#authentication), which is accepted from any
  origin. A request with no `Origin`, such as the CLI, passes. No `OPTIONS`
  preflight is answered and no CORS header is ever sent, so a browser *page* cannot
  use the token even if it has one; the exemption is for an extension service
  worker, which CORS does not govern. Under `tailnet_host` the daemon's own origin
  is `https://<tailnet_host>` and only that, and a request authenticated by the
  session cookie gets the check too — `SameSite=Strict` is not left as the only
  thing between a foreign page and a write.
- Under `tailnet_host` nothing at all is served unauthenticated, and what an
  authenticated caller reaches is the UI's own API: `POST /api/roots` and
  `POST /api/clip` stay on the machine, and so does any endpoint added later until
  somebody decides otherwise.
- Every path a request names is resolved through one function: it is cleaned and
  rejected if it leaves the root lexically, then symlinks are evaluated and it is
  rejected again if the real path leaves the root. A symlink pointing back inside the
  root is served.
- The clip endpoint composes no absolute path: it creates the directory and the note
  through a directory handle opened on the notes root, so a `clips_dir` that is a
  symlink out of the root is refused rather than followed, and it creates
  exclusively, so an existing note is never overwritten.
- The two halves of that meet at symlinks inside a root: ripgrep does not follow them,
  so a symlinked file or directory is in neither the navigator nor search, but one
  whose target is inside the root is still served by direct URL, and one whose target
  leaves the root is refused.

The daemon assumes a single-user machine, where every local process already runs as
the user who owns the notes — so any local process can list roots, register folders,
and read files under them through the API without presenting anything, and files the
navigator and search hide are still readable by direct URL. That premise was raised
and accepted deliberately
([#2](https://github.com/davison/md-notes/issues/2#issuecomment-5572874194)). The
[bearer token](#authentication) does not change it: it exists so that a client which
cannot make a same-origin claim — the browser extension — can be told apart from a
web page that has merely found the port, and it is protected by the same `0600` the
state file has. Any local process running as the user can read the token file, and
is already inside the premise. Configuring `tailnet_host` does change it, and
[The premise, restated](#the-premise-restated) above says how.
