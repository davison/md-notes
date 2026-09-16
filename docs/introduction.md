# Introduction

md-notes is a local service that turns folders of markdown files into a notes
application in the browser. This page describes what exists and works today, at the
end of [milestone four](milestones/4-polish-phone-e-ink-and-the-bundle.md): the
daemon and the rendered viewer from
[milestone one](milestones/1-daemon-and-rendered-viewer.md), the editor from
[milestone two](milestones/2-editor-autosave-and-live-update.md), the browser half
from [milestone three](milestones/3-clipper-authentication-and-tailnet.md), and the
polish milestone four put on all three. Notes are edited in place; creating,
renaming and deleting them is still done with other tools.

Milestone four is the one whose subject is how the rest of it is read rather than
what it can do. The web UI works on a phone, where the note takes the whole viewport
and the navigator, search and tags are the tabs of a drawer
([On a phone](#on-a-phone)); it works on an e-ink tablet, where a light-theme
override and a no-animation switch answer a screen with no backlight
([Display settings](#display-settings), [On an e-ink tablet](e-ink.md)); the browser
tab names the note you have open ([The browser tab](#the-browser-tab)); fenced code is
readable in the dark colour scheme
as well as the light one; the embedded bundle is compressed and cached and no longer
carries the editor to a page that is only reading ([Editing](#editing)); the browser
extension works against a daemon reached over the tailnet
([The browser extension](extension.md)); and
[Sync and offline editing](sync.md) is the page that says how the notes reach every
device.

The browser half is a Chromium extension that clips a readable page or a selection
into the notes root as markdown, and opens a local markdown file in the app instead
of leaving the browser to render it as plain text. The daemon's side of it is a
bearer token and a clip endpoint, both described below; the extension itself has its
own page, [The browser extension](extension.md). The same token is what lets the
daemon be reached from another node on the tailnet, behind `tailscale serve` and a
login page. The inbox, which turns URLs shared from a phone into clips when the
folder next syncs, is still later work.

Syncing is not the daemon's job at all: Syncthing mirrors the notes folder between
machines and an Android phone, and its writes reach the daemon as the ordinary
external changes that [live update](#live-update) and [conflicts](#conflicts)
describe below. [Sync and offline editing](sync.md) covers that arrangement —
Syncthing setup, editing offline with any editor or a second daemon, Syncthing's
conflict files in the navigator and how to resolve them, Android with Markor, and
when to use the tailnet instead.

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
client-side routes such as `/r/notes/some/note.md` load. Assets are served
compressed and cached — see [Editing](#editing) for what that costs and saves — and
the fallback `index.html` is, like the file itself, `no-cache` with an `ETag`.

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
endpoint alone: a holder can register a root and read files under it. That is true
over loopback, where anything that can reach the port already runs as you. Under
`tailnet_host` the same credential reaches less — see
[What is reachable under that name](#what-is-reachable-under-that-name-and-what-is-not).

The `Host` check is not waived by the token: DNS rebinding is a separate attack, and
a rebound page holds no token anyway.

**The caller must be one CORS does not govern.** An `Authorization` header makes a
cross-origin `fetch` non-simple, so a browser *page* sends `OPTIONS` first — and the
daemon refuses the preflight like any other cross-origin request and sends no
`Access-Control-Allow-Origin` on any response. That is deliberate, and it is the
boundary: a page that has merely found the port must not be able to write, even if
it has somehow read the token.

What that admits is **any context of a browser extension holding
`host_permissions`** for the daemon's origin — its service worker and its extension
pages, the popup and the options page alike, all of which are exempt from CORS and
never preflight. What it excludes is an ordinary web page and a content script;
since Chrome 73 a content script's requests are subject to CORS like a page's. So a
fetch from a web page fails at the preflight with the browser's opaque CORS error
whatever token it holds, while the extension's own options page can call the API to
test its token. (The clipping itself still belongs in the service worker, for a
different reason: the worker outlives the popup, which is destroyed the moment it
loses focus, and a context-menu clip has no popup open at all.)

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

Failed logins are bounded: three in a minute from one address are answered at once,
the next few wait half a second each, and past a dozen the daemon answers
`429 {"code":"too_many_attempts"}` with a `Retry-After` until the minute rolls. A
correct token clears the count, so being throttled never locks you out of your own
daemon. This is not what stands between anyone and the notes — the token is 130 bits
from `crypto/rand`, and guessing it is hopeless — it bounds how much work and how
many log lines one caller can cause.

The address those tiers count against comes from `X-Forwarded-For`, which a caller
behind a proxy that appends can write. Underneath them is a floor that key cannot
escape: once the daemon has seen more than three failures in the window **across
every address**, each further failure waits half a second whatever `X-Forwarded-For`
claims — including one arriving from an address the daemon has never seen. The floor
only ever delays and never refuses, because a refusal counted across all callers
would let anyone the ACL admits lock you out of your own notes. So a caller varying
the header meets the wait and never the `429`.

### Refusals

| Status | Code | What happened |
|--------|------|---------------|
| 403 | `bad_host` | The `Host` header is not `localhost` or `127.0.0.1` with the daemon's port, nor the configured `tailnet_host`; or a request announcing a proxy presented a loopback `Host`; or the request target was not in origin form |
| 403 | `cross_origin` | A foreign `Origin` and no token: present the token to write from another origin |
| 401 | `unauthorized` | An `Authorization` header that is not a valid `Bearer <token>` — a wrong token, a rotated-away one, or another scheme. Under `tailnet_host`, also a request that proved nothing at all |
| 403 | `loopback_only` | The endpoint is not reachable under `tailnet_host`. See [the tailnet section](#reaching-the-daemon-over-the-tailnet) |
| 405 | `method_not_allowed` | `/login` was reached with something other than `GET` or `POST` |

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
the [token](#authentication). The client it exists for is the browser extension,
which [has its own page](extension.md) covering installation, clipping and opening
local markdown files.

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

A Preact application, three panes per root on a wide screen and a note with a
drawer on a narrow one — see [On a phone](#on-a-phone) below. The browser tab names
what is on screen; [The browser tab](#the-browser-tab) below says how. The panes are:

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

The page follows the browser's own light or dark preference — unless the light-theme
override under [Display settings](#display-settings) is on — and so does the code in
a fence, which follows the override too. Both highlighting palettes are generated
from a single source palette by `internal/render/gencss`, each toned against the
background that scheme actually paints, so the two cover exactly the same set of
token classes and every colour the
stylesheet declares clears the 4.5:1 contrast the WCAG calls AA on the background it
is drawn on. The generator asserts both before it writes the file, and refuses a
stylesheet in which a class is styled in one scheme and not the other or carries a
background of its own in one scheme only, a colour falls below AA, a line-highlight or
diff tint is too close in luminance to the page to be seen, or two colours the source
palette tells apart have come together.

### The browser tab

The tab names what is on screen, so two windows on two roots are told apart in the tab
strip and a note with work still in the browser says so from there.

With a note open the tab takes the note's own title, by the same rule the note's heading
follows: the rendered `H1` if the note has one, otherwise the frontmatter `title`,
otherwise the file's own name with its extension dropped — `plain-name` for
`plain-name.md`. It follows the note through in-app navigation and through a
[live update](#live-update), in the rendered view and under an open editor alike: an
external rewrite that changes the heading changes the tab with it.

Everything else is named by what it is:

| What is open | The tab says |
|--------------|--------------|
| A note | The note's title, by the rule above |
| A root with no note open | The root's slug — `notes` for `/r/notes/` |
| The home page, a root that does not exist, any address that is not a route | `MD Notes` |

`MD Notes` is also what `index.html` itself says, ahead of the script that renders the
application, so a tab is never nameless while the page loads and stays sensible if the
bundle never runs at all.

An editing session adds a leading marker to the note's title:

| Marker | What it means |
|--------|---------------|
| `• ` | The draft is unsaved: waiting to be sent, being sent, or refused |
| `⚠ ` | The note changed or was deleted on disk under an unsaved draft — see [Conflicts](#conflicts) |

A conflict outranks the plain unsaved states, so the two never appear together and `⚠ `
replaces `• ` rather than joining it. The marker follows the *session*, not the pane's
mode, so a note left in the rendered view with a draft outstanding still carries it, and
both markers clear when the draft reaches disk or the conflict is resolved.

Two edges are worth knowing rather than meeting by surprise. A note **deleted** while it
is open keeps its file name on the tab — the pane says the note is gone, and the tab
still reads `doomed` rather than `MD Notes`, which is the more useful of the two when
several tabs are open. And a reader who edits their **own** `H1` sees the tab follow it
only when the rendered view next runs: their save does not replace the text under the
editor, so nothing asks the daemon what the note is called now. Both are recorded in
[the milestone four record](milestones/4-polish-phone-e-ink-and-the-bundle.md#known-gaps-at-the-boundary).

### On a phone

Below 960 pixels of window width — a phone in either orientation, and a narrow
desktop window — the note takes the whole viewport under a compact top bar, in the
rendered view and in the editor alike. The two side panes move into one drawer:

- The **burger** at the left of the top bar opens the drawer on its **Notes** tab,
  which is the navigator, with the expanded directories and the tag filter it has at
  any other width.
- The **magnifier** at the right opens the same drawer on its **Search & tags** tab,
  with the cursor already in the search box. Selecting a hit scrolls the note to the
  line as it does on a wide screen.
- The drawer closes when you choose a note or a search hit — it covers the note that
  the choice just opened — and on `Escape`, on the close button, and on a tap outside
  it. Focus moves into the drawer when it opens and back to the button that opened it
  when it closes. Choosing a **tag** instead moves to the Notes tab, since the filter
  prunes the tree the tab is showing.
- An active tag filter shows as a chip in the top bar, which names the tag and clears
  the filter when tapped: the tag panel that would otherwise say so is behind the
  drawer. The root's path leaves the top bar at these widths, being the longest and
  least useful of its labels on a phone, and the note a draft is unsaved in keeps as
  much of its name as fits.
- The [live update](#live-update) notice, which at wide widths sits at the top of the
  navigator, moves above the note here, where it is read without opening the drawer.

Search is a literal, case-insensitive phrase — what you type is what is matched.
Tags come from a frontmatter `tags` value (a list, or one string split on commas and
whitespace) and from inline hashtags: `#` at the start of a line or after whitespace,
followed by letters, digits, `_`, `-` or `/`, containing at least one letter, outside
fenced and inline code, lower-cased. Tags are collected per request, with no index.

### Display settings

The **gear** at the right of the top bar, at every width, opens a panel with two
switches. Both are kept in `localStorage`, so they are per device and per browser,
and neither is sent anywhere.

- **Always use the light theme.** The page uses the light palette whatever the
  device's `prefers-color-scheme` says. The syntax colouring in fenced code follows
  the same switch, so an overridden page is not left with dark-scheme tokens on a
  light background. It is applied by an inline script before the stylesheet paints,
  so overriding a dark device shows no frame of the dark scheme on load. Turning it
  off returns the page to the device's preference.
- **No animation.** No transitions, and no flash on the block a search hit scrolls
  to — the scroll itself still happens, centring the block. The same is true without
  the switch on a device that asks for reduced motion: `prefers-reduced-motion:
  reduce` is honoured on its own.

Tap targets are not a setting. Wherever the browser reports a coarse pointer or no
hover — a phone, a tablet, a stylus — or the window is below the narrow breakpoint,
every row and control that is tapped is at least 40 pixels tall: tree entries, tags
and the clear link, search hits and the search box, the drawer's tabs, the top bar's
buttons, the note bar's, and the frontmatter disclosure. Links *inside* a note are the
exception, and have to be — their size is the line of prose they sit in. Under a mouse
at a wide width the rows keep their compact density.

Both settings exist for an e-ink tablet, where a dark theme is grey on grey and
every animation is a slow visible repaint. [On an e-ink tablet](e-ink.md) covers
that device end to end, including the two ways to reach your notes from one.

### What holds these numbers

The figures in the two sections above are not only documented, they are measured on
every push. `make e2e` runs a suite under `ui/e2e` in headless Chromium against the
built daemon on a temporary root, and CI runs it as a job of its own: the pane
rectangles at four phone profiles and a desktop control, the 960-pixel breakpoint
walked at 959, 960 and 961, the drawer's geometry and all four of its close paths,
the tag chip, the 40-pixel targets under a coarse pointer with the mouse-driven
window's density left alone, the light override applied with the application bundle
blocked — so nothing but the inline boot script can have applied it — the flash
suppressed by the setting and by `prefers-reduced-motion`, and a second page load
that fetches no asset bytes. It needs Chromium, which is a separate download; see
the README's **Building** section.

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

The editor is part of the UI bundle embedded in the binary, but a page that is only
reading a note does not load it. Opening a note pulls one JavaScript chunk and one
stylesheet; CodeMirror and the table of languages it can highlight are an order of
magnitude larger and are fetched on the first `Ctrl+E` of that page, then kept for
every later toggle in it.

What that costs in time holds its shape across every run it has been measured on, even
where the absolute numbers differ by a factor of two. The first `Ctrl+E` of a page is the
expensive one; a second page with the chunk already in the browser's cache costs **the same
again**, because what the time buys is compiling CodeMirror and building the editor rather
than getting hold of it; and every later toggle *within* a page is several times quicker,
because the module is held. As of this milestone, on loopback, that is about 85 ms, about
85 ms and about 7 ms — two runs measured all three and agreed to within a millisecond, and
a third measured the held toggle five times quicker rather than twelve. Four samples of the
first figure run from 56 ms to 103 ms, two of them from the same environment, so the spread
is what else the machine was doing rather than which machine it was. That is wide enough
that the shape above is the part to rely on and any single number is the part to
re-measure; they are in
[the milestone four record](milestones/4-polish-phone-e-ink-and-the-bundle.md#known-gaps-at-the-boundary).

If the daemon is upgraded while a page is open, the chunk that page would ask for is no
longer in the bundle and the daemon answers 404: `Ctrl+E` then says the editor could not
be loaded and offers to reload, which is the only thing that cures it — a browser that
failed to fetch a module will not ask for that URL again, however healthy the network
becomes, but a reloaded page asks for whatever the current `index.html` names. The note
and any unsaved draft survive the reload. The per-language parsers for fenced code are
separate chunks again, one per language, fetched when a note containing such a block is
opened in the editor — not when the block is typed in.

What the daemon serves is compressed and cacheable. The build writes a brotli and a
gzip copy beside each asset, and the daemon serves whichever the request's
`Accept-Encoding` asks for — brotli first, gzip next, the plain file if the client
takes neither or the build could not shrink that file — always with
`Vary: Accept-Encoding`, the source file's `Content-Type`, and a `Content-Length`.
The hashed files under `/assets/` carry `Cache-Control: public, max-age=31536000,
immutable`: their names change when their content does, so a browser that has one
never asks for it again. `index.html` carries `no-cache`, which means revalidate
rather than do not store, and every response carries an `ETag` over the bytes actually
sent, so the revalidation is answered with a 304 and the page itself crosses the wire
only when it has changed. So a first load of a note transfers the bundle once and a
reload, a new tab, or a note URL opened directly transfers **no asset bytes at all** —
only the JSON for the note itself. That is the property worth remembering; the sizes
behind it move with every change to the UI and are not worth trusting from a page like
this one. As of this milestone, at
[`e37164c`](https://github.com/davison/md-notes/commit/e37164c), a cold load of a note
transferred **19,738 bytes** across two assets — a 50,657-byte JavaScript chunk and an
18,376-byte stylesheet, compressed to 16,189 and 3,549 on the wire — and the first
`Ctrl+E` pulled about 200 KB more, and more again on a note with several fenced languages,
since each one's parser is a chunk of its own: 197,808 bytes for a note with no fenced code,
209,076 with one `go` fence, 226,093 with three languages.
[The milestone four record](milestones/4-polish-phone-e-ink-and-the-bundle.md#corrections-to-the-record-itself)
carries the measurements, who took them, and how far they had already drifted inside one
milestone.

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

A write arriving from Syncthing lands here like any other external change, and is a
different thing from a Syncthing *conflict file*, which arrives as a new note beside
the old one. [Sync and offline editing](sync.md#when-two-devices-edit-the-same-note)
covers both and how to deal with each.

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

This is also how a synced folder stays current: the daemon does not know Syncthing
exists, and Syncthing's writes reach it as ordinary changes on disk. See
[Sync and offline editing](sync.md).

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
| `Host` | the browser, **passed through unchanged** | matched against `tailnet_host`; this is what makes the guard let the request through |
| `X-Forwarded-Proto: https` | `tailscale serve` | the login page refuses to set a `Secure` cookie the browser would discard, and says so, rather than looping |
| `X-Forwarded-For` | `tailscale serve` | the tailnet address in the daemon's log line for each login |
| `Tailscale-User-Login` and friends | `tailscale serve` | **nothing.** The daemon reads no identity header |

Those headers are trusted only because the listener is loopback: the only things
that can set them are the proxy and a process already running as you. The identity
headers are deliberately unused: the capture this work adopts
([#10](https://github.com/davison/md-notes/issues/10)) asked for one authentication
path designed once for both the browser extension and the tailnet, and that is the
token.

**Whatever terminates TLS in front must pass the original `Host` through
unchanged.** That header is the authentication boundary: it is what tells the daemon
a request came from the tailnet and must therefore prove itself, rather than from
the machine's own loopback, where it need not. `tailscale serve` does pass it
through, which is why the command above is the whole configuration. Not every proxy
does — nginx's `proxy_pass http://127.0.0.1:7337;` rewrites `Host` to the upstream
address unless you add `proxy_set_header Host $host;`, and a proxy set up that way
would present every remote request to the daemon as a local one.

There is a backstop under that, and it is worth being exact about what it catches.
With `tailnet_host` configured, a request that presents a loopback `Host` while
announcing that it came through a proxy — `Forwarded`, `Via`, `X-Forwarded-For`,
`X-Forwarded-Proto`, `X-Forwarded-Host` or `X-Real-IP`, none of which anything on
loopback sets — is refused `403 bad_host` with a message naming the cause. That
covers Caddy, Traefik, Apache's defaults and the usual nginx boilerplate, which all
say who they are. It does **not** cover the bare `proxy_pass` line above: nginx on
its own adds none of those headers, so a proxy configured with that line and nothing
else is indistinguishable from a local client, and no check here can save it. The
sentence in bold above is the defence; this is what catches the common mistakes
before they become unauthenticated access.

A request target in absolute form (`GET http://127.0.0.1:7337/api/roots HTTP/1.1`)
is refused outright, whatever is configured, because Go takes `Host` from the
target's authority when one is present and the target would otherwise choose the
rule. That one is a guarantee rather than a heuristic: origin form is the only form
a browser or a reverse proxy sends to an origin server.

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
by default, and it runs where the notes are. Pointing that URL at the tailnet name
buys exactly one of the extension's three actions: with the token pasted it opens a
local markdown file that is already inside a registered root, because the roots
listing it depends on carries the token whenever the daemon URL is not loopback.
Its clips and folder registrations meet the same `loopback_only` as everybody
else's, and the extension names that refusal rather than blaming the token — see
[the extension page](extension.md#a-daemon-reached-over-the-tailnet). Reading notes
from another device is still the UI's job in the browser there.

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
still a single secret shared by every client, which is the shape milestone three
fixed and did not go on to narrow.

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
  preflight is answered and no CORS header is ever sent, so a **web** page cannot use
  the token even if it has one, and neither can a content script. The exemption
  follows `host_permissions`: it is for the extension's own contexts, which CORS
  does not govern. Under `tailnet_host` the daemon's own origin is
  `https://<tailnet_host>` and only that, and a request authenticated by the session
  cookie gets the check too — `SameSite=Strict` is not left as the only thing
  between a foreign page and a write.
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
