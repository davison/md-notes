# Introduction

md-notes is a local service that turns folders of markdown files into a notes
application in the browser. This page describes what exists and works today, at the
end of [milestone nine](milestones/9-flowcharts-drawn-by-the-daemon.md): the
daemon and the rendered viewer from
[milestone one](milestones/1-daemon-and-rendered-viewer.md), the editor from
[milestone two](milestones/2-editor-autosave-and-live-update.md), the browser half
from [milestone three](milestones/3-clipper-authentication-and-tailnet.md), the
polish milestone four put on all three, the create and delete verbs milestone
five added to them, the tailnet clipping and clipper repairs of
[milestone six](milestones/6-tailnet-clipping-and-the-m5-backlog.md), and what
milestone seven did to roots, to the navigator and to installing the app on a phone,
all of it carrying a version number and installable from a package since
[milestone eight](milestones/8-the-first-release.md), and the flowcharts milestone nine
draws.
Notes are created, edited and deleted in the app; renaming one is still done with other
tools.

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

Milestone five made the application the only tool the notes need day to day. A note
is created from the top bar behind a name prompt and deleted from the note bar
behind a confirmation ([Creating and deleting a note](#creating-and-deleting-a-note)),
over two new methods on the note's own source resource
([The HTTP API](#the-http-api)) that carry the same confinement, the same error
envelope and the same tailnet rule as the save that was already there. The
milestone also hardened the gate the rest of this is merged through: the debounce
tests in `internal/watch` are driven by a clock the test moves rather than by the
wall clock, and the browser-level checks that used to live in a session's scratchpad
are a suite in the repository that CI runs
([What holds these numbers](#what-holds-these-numbers)).

Milestone seven took up an accident and an ask. Opening a `file:` URL for a markdown
file that did not exist used to register its directory as a permanent root with no
way to remove it from the app; now a registration can name the note it is for and the
daemon registers nothing when that note is not there, a recent root is removed from
the home page, and both of those stay on the machine rather than crossing the tailnet
name ([Roots](#roots)). The navigator's tree, alphanumeric since milestone one, has a
**Recent first** toggle that puts the most recently modified note at the top
([The web UI](#the-web-ui)). And the UI ships a web app manifest and a service worker, so
a phone reaching the daemon over the tailnet offers to install it and the installed app
opens in its own window — and opens with no daemon behind it, saying so rather than
showing the browser's error page ([Installing the app](#installing-the-app)).

Milestone eight gave all of it a version and a way in.
[v0.1.0](https://github.com/davison/md-notes/releases/tag/v0.1.0) is cut from a tag,
and the daemon installs from the Arch User Repository or from a `.deb` rather than out
of a working copy; the browser extension is a zip on the same release page, loaded
unpacked, the Chrome Web Store channel having been withdrawn before the first release
([#135](https://github.com/davison/md-notes/issues/135#issuecomment-5744118645)). The
[README's installation section](../README.md#installing) has the commands and
[Cutting a release](releasing.md) the machinery. The same milestone took the things a
first public release should not carry: a note's own HTML can no longer wear the
renderer's structural classes or its `data-line` marker, so it cannot plant a decoy
scroll target ([The web UI](#the-web-ui)); a directory the daemon may not read is
reported as that rather than as the kernel's watch limit
([When coverage is limited](#when-coverage-is-limited)); every dependency is on its
current release or held back for a reason recorded in the file that holds it, with
`govulncheck` and `pnpm audit` in CI; and between the drawer breakpoint and the width
the three-column layout needs, the search and tag pane sits under the navigator so the
note column gets every pixel that is not the navigator — reaching the full reading width
from 1020 px up — with thin scrollbars in the theme's own colours throughout
([In a narrower window](#in-a-narrower-window)).

Milestone nine made a mermaid flowchart in a note read as a diagram. The daemon parses
the block, lays it out and writes the SVG itself, and the page shows it as an image in
front of the code block, in the light, dark or e-ink palette the page is using, redrawn
when the note changes on disk. Mermaid's own library was vetted and declined — it has
critical cross-site-scripting advisories under its strictest setting, and any script on
this app's origin can read and write every note — so no diagram source ever runs,
styles or inserts anything in the browser. A block outside the supported subset, one
the daemon refuses to draw, and every other mermaid diagram type stay the code block
they were ([Flowcharts](#flowcharts)).

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
mdn serve                run the daemon against the configured roots
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

`--root` can be given more than once, and every one is served:
`mdn serve --root ~/notes --root ~/projects` serves both, with `~/notes` as the notes
root and `~/projects` as a [permanent root](#roots). Any `--root` replaces
`notes_root` from the file completely rather than adding to it, just as `--port`
replaces `port`, so the command line alone says what is served and which folder is
first. The daemon refuses to start if two roots name the same folder, including
through a symlink. Every start logs each root it serves with its slug, kind and
path, and when `--root` has set aside roots the file names, it logs that too, naming
them:

```
--root replaced notes_root from /home/you/.config/mdn/config.yml (/home/you/notes)
serving notes (notes): /home/you/notes
serving projects (permanent): /home/you/projects
```

`mdn open DIR` takes `--config FILE`, `--port N`, and `--no-browser` to print the URL
instead of launching one. It resolves `DIR` against your working directory, POSTs it
to the running daemon and opens `/r/<slug>/`. If no daemon answers on the port it
prints how to start one and exits non-zero — it never starts a daemon itself. There
is no verb that undoes it: a folder registered this way is removed from the home
page, which is where a root registered by accident is removed as well
([Roots](#roots)).

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

`notes_root` also takes a list, which is how you name several permanent roots in
the file:

```yaml
notes_root:
  - /home/you/notes      # the notes root: clips land here
  - /home/you/projects   # a permanent root
```

The first entry is the notes root and the rest are permanent roots, in the order
given. Each one must be an existing directory, and two entries naming the same
folder stop the daemon at startup with an error naming both.

A missing file is not an error as long as `--root` supplies the notes root. The
default port is 7337 and the default watch budget 8192 directories per root; `0`
removes the budget, and a negative value is refused. The Live update section below
says what the budget buys. `clips_dir` is where [clips](#clipping-a-web-page) land,
relative to the notes root, default `clips`; an absolute path, or one climbing out
of the notes root, is refused at startup. `tailnet_host` is the one extra `Host`
name the daemon answers to, for requests a `tailscale serve` proxy forwards to the
loopback port; it is empty by default, and everything under it must authenticate.
See [Reaching the daemon over the tailnet](#reaching-the-daemon-over-the-tailnet).
`contrib/mdn.service` is a systemd user unit that runs `/usr/bin/mdn serve`, which
is where `make install`, the `.deb` and the AUR package all put the binary; all
three install the unit itself to `/usr/lib/systemd/user/mdn.service` as well — the
two packages install that file verbatim — and all three leave
`systemctl --user enable --now mdn` to you. `make install` copies what
`make build` already made and refuses if it is not there, so build first; a
per-user install points the unit elsewhere with a drop-in, as the unit's own
header describes.

ripgrep (`rg`) must be on `PATH` at runtime. It builds the navigator's file listing,
runs search, and decides which files the tag collector reads — which is how
gitignored and hidden files stay out of all three.

## Roots

A root is a folder the daemon serves, identified in URLs by a slug derived from its
basename and deduplicated with a numeric suffix. There are three kinds, and the kind
is the `kind` field of each root in `GET /api/roots`:

- The **notes root** (`notes`): the first entry in `notes_root`, or the first
  `--root`. It is permanent and always present, and it is where
  [clips](#clipping-a-web-page) land.
- **Permanent roots** (`permanent`): every configured root after the first, from
  `notes_root`'s list form or a repeated `--root`. Like the notes root they are the
  daemon's configuration. They are served from every start, never written to the
  state file, and cannot be removed from the home page. A folder that is in the
  state file as a recent root and is now configured is served once, as permanent,
  and the start logs it. Its entry stays in the state file, so a start without that
  configuration serves it as a recent root again, under its old slug. One
  configured root may sit inside another: each is served as a root of its own, as a
  nested `mdn open` is.
- **Recent roots** (`recent`), added by `mdn open` or by the browser extension when
  you open a local markdown file. They persist to the state file with their slugs,
  so a root's URL survives a restart. A recent root whose directory has since
  disappeared is not served, and leaves the state file once the daemon has bound its
  port; a start that fails leaves the state file exactly as it was. Any recent root
  can be removed from the home page.

Registering a path that is already a root returns the existing root rather than
duplicating it; the comparison is on the real path, so a symlinked alias resolves to
the same root. A directory nested inside an existing root can still be registered as
a root of its own.

**A registration that names a note the daemon cannot find registers nothing.** The
body of `POST /api/roots` takes an optional `file` beside the `path` — a note inside
the folder, relative to it or absolute and under it — and with it the daemon
registers the folder only once it has found that note: a regular markdown file that
resolves inside the folder through the same confinement every request path goes
through. Anything else is `404 {"code":"not_found"}` naming the file, with nothing
appended to the registry and nothing written to the state file. One answer covers
every way of failing, because the caller is on loopback and already holds the path it
asked about, and a finer answer would say more about what is *outside* the folder
than about the folder
([#115](https://github.com/davison/md-notes/issues/115#issuecomment-5718216788)). The
folder is the other question and keeps its own answer: a `path` that is not a
directory is still `400` with the filesystem's own sentence, which is the diagnostic
`mdn open` prints to somebody who has just typed it
([#115](https://github.com/davison/md-notes/issues/115#issuecomment-5718465965)).

The check is a condition on registering, not a lease on the root. Once the folder is
a root the note it was registered for can be deleted, renamed or emptied, and the
folder goes on being served until somebody unregisters it. `mdn open` sends no
`file`, so it is unaffected; the extension sends the note it was asked to open, which
is what stops a `file:` URL for a note that does not exist from registering its
directory for ever
([#50](https://github.com/davison/md-notes/issues/50)).

**Removing a recent root** is `DELETE /api/roots/{slug}`, and on the home page it is
the **Remove** control beside the root, behind a confirmation naming the folder. The
root leaves the registry and the state file at once. Nothing leaves the disk: the
folder and every note in it stay exactly as they are, and registering the folder
again brings the root back. When the last recent root is removed, the state file is
left as `{"recent": []}`. Three things are refused. The configured notes root gets
`403 {"code":"notes_root"}`, and a permanent root gets `403 {"code":"permanent_root"}`:
both are the daemon's configuration rather than registrations, and the next start
would put them back. A slug that is not registered gets `404 {"code":"not_found"}`,
which is also what removing the same root twice gets.

A tab left open on a root that has just been removed lands on the home page rather
than on a dead route. The daemon ends the root's event streams when it unregisters
it; the browser reconnects of its own accord, meets a `404` for the unknown slug, and
the page asks the roots listing before concluding anything — a slug that has gone
routes home, one that is still there keeps the "live update is not available" notice
described under [When coverage is limited](#when-coverage-is-limited). That route
rides on the stream, so it holds for every root the daemon can watch. The one root it
does not hold for is a root whose watcher never started at all: its first events
connect is refused, the browser does not retry a refused connection, and such a tab
keeps its notice and its stale navigator until it next asks the daemon for something
([#115](https://github.com/davison/md-notes/issues/115#issuecomment-5718465965)).

Both are loopback-only. Registering a root and unregistering one are refused under a
configured `tailnet_host` — see [What is reachable under that
name](#what-is-reachable-under-that-name-and-what-is-not).

The home page at `/` lists the notes root and the permanent roots under "Notes", in
the order configured, and every recent root under "Recent". Each links to its
three-pane view, and only the recent roots have the **Remove** control beside them.

## The HTTP API

All endpoints are on the loopback listener, and all of them are behind the guard
described under [Confinement](#confinement). Most of them are also reachable under a
configured `tailnet_host`, to an authenticated caller; the two that change the set of
roots — `POST /api/roots` and `DELETE /api/roots/{slug}` — are not, and
the [tailnet section](#reaching-the-daemon-over-the-tailnet) says why.

| Endpoint | What it does |
|----------|--------------|
| `GET /api/roots` | `{roots: [{slug, path, kind}]}`, `kind` being `notes`, `permanent` or `recent`; the notes root first, then the permanent roots in the order configured, then the recent roots. See [Roots](#roots) |
| `POST /api/roots` | Registers `{"path": "/absolute/dir"}` and returns the root. Relative paths are refused, and a path that is not a directory is `400` with the filesystem's own sentence. The optional `"file"` names a note inside that folder: with it the daemon registers only once it has found the note, and otherwise answers `404 {"code":"not_found"}` having written nothing. See [Roots](#roots) |
| `DELETE /api/roots/{slug}` | Unregisters a recent root and returns `204 No Content`; the root leaves the registry and the state file, and no file leaves the disk. `403 {"code":"notes_root"}` for the configured notes root, `403 {"code":"permanent_root"}` for a permanent root, `404 {"code":"not_found"}` for a slug that is not registered, and `403 {"code":"loopback_only"}` under a configured `tailnet_host`. See [Roots](#roots) |
| `GET /api/r/{slug}/tree` | The root's markdown tree as nested `{name, path, dir, children}`, each file node also carrying `modified`, its modification time in Unix milliseconds — absent on a directory, and on a file whose time the daemon could not read. See [the navigator's order](#the-web-ui) |
| `GET /api/r/{slug}/note/{path...}` | A rendered note as `{path, title, frontmatter, html, diagrams}`. `diagrams` lists the note's drawable flowcharts as `[{line, hash}]` — the `data-line` of the anchor before the block and a hash of its source — and is absent when there are none. With `?sizes=1` the daemon also measures them, within a 150 ms budget: each entry it reached gains the drawing's `width` and `height`, and a block the layout refuses is left out. Non-markdown paths are 404 here. See [Flowcharts](#flowcharts) |
| `GET /api/r/{slug}/diagram/{path...}?h=&theme=` | The SVG of the flowchart whose hash is `h` in that note, drawn in the `light`, `dark` or `eink` palette, as `image/svg+xml` with `Cache-Control: no-cache` and an `ETag`. `404` when the note holds no such block (it changed), `400` for a bad `theme` or `h`, `422` with the reason when the daemon refuses to draw it. Every answer carries `Content-Security-Policy: default-src 'none'; sandbox` and `X-Content-Type-Options: nosniff`. See [Flowcharts](#flowcharts) |
| `GET /api/r/{slug}/source/{path...}` | Existing UTF-8 markdown as `{source, revision}`; see [conditional saves](#conditional-saves) |
| `PUT /api/r/{slug}/source/{path...}` | Conditionally saves JSON `{source, revision}` and returns the saved `{source, revision}` |
| `POST /api/r/{slug}/source/{path...}` | Creates the note at `{path...}`, never overwriting. Optional JSON body `{"source": "..."}`; no body at all creates an empty note. `201` with a `Location` header and `{root, path, source, revision}`. Missing parent directories are created. See [Creating and deleting notes](#creating-and-deleting-notes) |
| `DELETE /api/r/{slug}/source/{path...}` | Removes exactly one regular markdown file inside the root. `204 No Content`. See [Creating and deleting notes](#creating-and-deleting-notes) |
| `GET /api/r/{slug}/raw/{path...}` | File bytes, for images and other assets. Served with `Content-Security-Policy: sandbox` and `X-Content-Type-Options: nosniff` |
| `GET /api/r/{slug}/search?q=` | `{hits, truncated}`; each hit is a path, line number, matching text with match offsets, and the lines either side |
| `GET /api/r/{slug}/tags` | `{tags: [{name, count, notes}]}`, sorted by count then name |
| `GET /api/r/{slug}/events` | A Server-Sent Events stream of change batches |
| `POST /api/clip` | Creates a note from `{url, title, markdown, kind}`; needs the token, on loopback and under `tailnet_host` alike. See [Clipping a web page](#clipping-a-web-page) |

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
| 400 | `invalid_body`, `invalid_path` | Correct malformed JSON or missing/invalid fields; `invalid_path` also answers a name the filesystem calls too long |
| 403 | `outside_root`, `permission_denied` | Retain the draft; check the path or file/directory permissions |
| 404 | `not_found`, `not_markdown` | Retain the draft; the note/root is missing or the path is not markdown |
| 409 | `conflict` | Retain the draft and fetch current source before choosing how to reconcile |
| 413 | `too_large` | Source or request exceeds the size limit |
| 415 | `invalid_body` | Send `Content-Type: application/json` |
| 422 | `unsupported_source` | The file is nonregular, contains invalid UTF-8, or is a chain of symlinks nothing will follow to the end |
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

### Creating and deleting notes

`POST` and `DELETE` on the note's own source resource. They are two more methods on
the URL `GET` reads and `PUT` replaces, rather than a new noun, so there is one path
grammar, one confinement check and one error envelope for all four
([#76](https://github.com/davison/md-notes/issues/76#issuecomment-5700721960)). Both
work in any root the daemon serves — the notes root, the permanent roots and every
folder added with `mdn open` — on the same terms as the save, which has always
written to all of them
([#76](https://github.com/davison/md-notes/issues/76#issuecomment-5700724927)).

`POST` takes the note's whole path under the root. It refuses to overwrite: the file
is opened `O_CREATE|O_EXCL` through a handle on the resolved parent directory, so a
name already held by a file, a directory or a link is refused rather than replaced.
Parent directories that do not exist are created, and a directory that could only be
made outside the root is refused as such. The `revision` in the `201` body is the one
a first save is checked against, so a client can open the new note in an editor
without a second request, and `path` is the daemon's cleaned form of the path — which
is what the client should route to, not the string it sent.

`DELETE` removes one regular markdown file and nothing else. The name is confined
lexically, the parent is resolved, and the final component is `Lstat`ed — not
`Stat`ed — through a handle on that parent, so a symlink is refused rather than
followed and the file that goes is always the file the caller named. Directories,
symlinks, non-markdown targets and read-only files are all refused, and `RemoveAll`
appears nowhere in the path.

Both refuse with the save's `{code, error}` body
([#76](https://github.com/davison/md-notes/issues/76#issuecomment-5701086842)):

| Condition | Status | Code |
|-----------|--------|------|
| Empty path (`POST /api/r/{slug}/source/`) | 400 | `invalid_path` |
| Any path component begins with `.`, including a file called `.md` | 400 | `invalid_path` |
| A control character anywhere in the path | 400 | `invalid_path` |
| A path component the filesystem calls too long — 255 bytes on the filesystems this runs on | 400 | `invalid_path` |
| The extension is not `.md` or `.markdown` | 404 | `not_markdown` |
| A path component exists but is not a directory (`hello.md/child.md`) | 404 | `not_found` |
| The note or the root does not exist (delete) | 404 | `not_found` |
| The path resolves outside the root, lexically or through a symlink — including a symlink with no target, or a chain of them as long as the resolver itself follows (see below), ending outside, and whether it stands where the note does or where one of its folders does | 403 | `outside_root` |
| The file is read-only (delete) | 403 | `permission_denied` |
| The target is a directory or a symlink inside the root (delete) | 422 | `unsupported_source` |
| The name is already taken, by a file, a directory or a link inside the root (create) | 409 | `exists` |

A name the filesystem will not take is the caller's mistake rather than the
daemon's fault, and is answered as one: the length a name may be belongs to the
filesystem — `NAME_MAX`, 255 bytes per component — so the kernel's refusal is
translated, not anticipated by a limit of the daemon's own
([#88](https://github.com/davison/md-notes/issues/88)).

A symlink with no target is followed lexically, hop by hop, because the first
name it gives may be inside the root and the second outside. The walk follows as
many links as the resolver behind every other check follows — 255, which is
`filepath.EvalSymlinks`'s own budget, and far past the 40 the kernel will resolve
in a single lookup — so every chain that reaches the daemon as a missing path is
answered by it. A chain longer than that nothing will follow to the end: the
resolver refuses it before the walk is reached, and the name is answered as one
held by something unresolvable — `409 exists` to create, because the name is
taken, and `422 unsupported_source` to delete, to read and to save, because what
holds it is nothing any of them can act on — never followed and never written
over. Nothing is created or removed
in any of these cases, and what the walk decides is only which refusal the caller
is shown ([#82](https://github.com/davison/md-notes/issues/82)).

`GET` and `PUT` on one of these dangling-link paths answer `404 not_found` rather
than `403 outside_root`: confinement there is the resolver's alone, and to it a
link with no target is simply missing. The split is deliberate — the requirement
that fixed these codes ([M6-R3](milestones/6-tailnet-clipping-and-the-m5-backlog.md))
is about create and delete, and widening the read and save paths is a behaviour
change no requirement asks for — and it is recorded with its
trade-off at
[#99](https://github.com/davison/md-notes/issues/99#issuecomment-5703933261).

Creating and deleting a note reaches every open page for that root through the
[events stream](#live-updates) as an ordinary change batch, so a navigator needs no
special handling for either.

The daemon takes a path and not a title plus a folder: the rule that turns a typed
title into `<title>.md` in a folder belongs to the client, and the daemon's job is to
refuse every unsafe name it is handed. [Creating and deleting a
note](#creating-and-deleting-a-note) is what the web UI composes with it.

### Clipping a web page

`POST /api/clip` creates a note from a web clipping. It is the only endpoint that
requires the [token](#authentication), and the only one that chooses the new note's
path itself rather than taking it from the caller. The client it exists for is the
browser extension, which [has its own page](extension.md) covering installation,
clipping and opening local markdown files. It is reachable under `tailnet_host` as
well as on loopback — with the token, which the session cookie does not substitute
for here; see
[What is reachable under that name](#what-is-reachable-under-that-name-and-what-is-not).

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
| 403 | `outside_root` | `clips_dir` resolves outside the notes root, a dangling symlink chain out of it included ([#82](https://github.com/davison/md-notes/issues/82)) |
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
{"watched": 8192, "unwatched": 4508, "budget": 8192,
 "overBudget": true, "failed": 0, "refused": 0, "limited": true}
```

`limited` is `unwatched > 0`; `overBudget` says the budget rather than an error is
the reason; `failed` counts directories the kernel's own watch pool refused —
`ENOSPC` from `inotify_add_watch`, meaning `fs.inotify.max_user_watches` is spent —
and nothing else; `refused` counts directories refused for any other reason, which no
limit answers; `budget` is the per-root maximum and is `0` when there is none, never
negative. Every frame carries all of those. `reason` is sent only when `refused` is
not zero, and holds the first such refusal in the words the operating system used, so
a root with one directory the daemon may not read sends:

```json
{"watched": 2, "unwatched": 1, "budget": 8192, "overBudget": false,
 "failed": 0, "refused": 1, "reason": "permission denied", "limited": true}
```

[When coverage is limited](#when-coverage-is-limited) says what the daemon reports for
each cause and what to do about it. A root with no watcher at all has no stream: the
endpoint answers 503, which an `EventSource` reports by closing for good rather than
reconnecting.

Unregistering a root ends the streams open on it, rather than leaving them on
keepalives for a root the daemon no longer serves; the reconnect that follows meets
the `404` an unknown slug gets, and [Roots](#roots) says what the page does with
that.

## The web UI

A Preact application, three panes per root on a wide screen, the same three in two
columns on a narrower window — see [In a narrower window](#in-a-narrower-window) — and a
note with a drawer on a phone — see [On a phone](#on-a-phone). The browser tab names
what is on screen; [The browser tab](#the-browser-tab) below says how. The panes are:

- **Navigator.** The markdown tree, with collapsible directories whose expansion is
  remembered per root in `localStorage`. The current note is highlighted and its
  ancestors are opened. Only markdown files and the directories containing them
  appear; `.md` and `.markdown` count, hidden entries do not, and everything ripgrep
  would ignore is absent. A **Recent first** toggle in the navigator's own header
  chooses the order: unpressed, the tree is the alphanumeric one it has always been;
  pressed, the most recently modified note is at the top — notes by their
  modification time within a folder, folders by the newest note anywhere beneath
  them, folders still grouped before notes, and every tie broken by the same
  case-insensitive name comparison the daemon sorts by. A note whose modification
  time could not be read sorts last among its siblings, and its folder last among
  folders holding nothing newer: "we do not know when this changed" is not a claim
  that it changed just now. The order applies to the tag-filtered tree, where a
  folder is ranked by the newest note it is *showing*, and to the same navigator in
  the drawer at narrow widths. The choice is kept in `localStorage` for the browser
  rather than per root — unlike the expanded directories, which are per root — and a
  toggle disturbs neither the expansion nor the selected note. The times ride on the
  [tree response](#the-http-api) and the tree is refetched on every change batch that
  can affect it, so a note saved in the app, or written on disk by something else,
  moves to the top without a reload
  ([#116](https://github.com/davison/md-notes/issues/116#issuecomment-5718168243)).
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
  application calls its own classes. HTML a note writes itself is held to less than that:
  those structural names, and the `data-line` marker the note view scrolls a search hit
  by, are taken off it before the sanitiser runs, so a note cannot plant a decoy scroll
  target for a search hit to land on
  ([#31](https://github.com/davison/md-notes/issues/31)). Relative links to markdown
  become in-app navigation; relative images and other assets are served from the raw
  endpoint; a link whose target escapes the root keeps its text but loses its destination
  and says why. A mermaid flowchart is drawn by the daemon and shown as an image in front
  of its code block — see [Flowcharts](#flowcharts). A bar above the note carries the
  mode, the save state, `Ctrl+E`, which flips the pane to the editor and back, and
  **Delete** at its right-hand end — see [Editing](#editing). The delete button's place
  is fixed: it is the far end of the bar from the mode toggle, in the rendered view and
  in the editor alike, and the `margin-left: auto` that puts it there is a property of
  the button rather than of the save status beside it, which is absent on a note that
  has only been read
  ([#85](https://github.com/davison/md-notes/issues/85#issuecomment-5702098868)).
- **Search and tags.** A debounced search box whose results group by file, showing
  the matching line with the match emphasised and a line of context either side.
  Selecting a hit opens the note with `?l=<line>` and scrolls to the block at that
  line, flashing it. Below it, the tag panel lists each tag with its count; selecting
  one sets `?tag=` and prunes the navigator to the notes carrying it, with a link to
  clear the filter.

Above the panes is one top bar at every width, carrying — left to right — the `mdn`
home link, **New note**, the root's slug and its path, and at the right the list of
other notes holding unsaved work, the search toggle and the **gear** for
[Display settings](#display-settings). The
create control is there rather than above the navigator's tree because a long tree
scrolled it out of sight, and it stays there below the narrow breakpoint as well,
where the stylesheet drops its label and leaves a square `+` beside the burger and
the magnifier ([#85](https://github.com/davison/md-notes/issues/85#issuecomment-5702099098)).

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

### Flowcharts

A fenced block whose info string is `mermaid` and whose diagram is a `flowchart` or a
`graph` reads as a drawing rather than as code. The daemon draws it, as an SVG, and the
page shows that SVG as an image in front of the code block, which it hides while the
image stands. Mermaid's own library is not used, here or anywhere
([#169](https://github.com/davison/md-notes/issues/169#issuecomment-5765785737)): the
flowchart is parsed, laid out and written by `internal/diagram`, a package of the
daemon's own. Other mermaid diagram types — sequence, class, state, Gantt and the rest
— stay code blocks until someone needs one.

**What is drawn.** A block is drawn when it uses only this subset of the flowchart
syntax:

- the header `flowchart` or `graph`, with a direction `TB`, `TD`, `BT`, `LR` or `RL`
  (`TB` if none), statements on their own lines or separated by `;`, so
  `graph TD; A-->B;` works;
- nodes, bare (`A`, a rectangle labelled `A`) or in one of fourteen shapes: `A[rect]`,
  `A(round)`, `A([stadium])`, `A[[subroutine]]`, `A[(cylinder)]`, `A((circle))`,
  `A(((double circle)))`, `A>asymmetric]`, `A{rhombus}`, `A{{hexagon}}`, the
  parallelograms `A[/…/]` and `A[\…\]`, and the trapezoids `A[/…\]` and `A[\…/]`;
- links: solid `---` and `-->`, dotted `-.-` and `-.->`, thick `===` and `==>`,
  invisible `~~~`; circle and cross ends (`--o`, `--x`); both ends (`<-->`, `o--o`,
  `x--x`); and longer links (`--->`, `-..->`), each extra character one more rank,
  up to eight ranks — `--------->` is the longest arrow drawn;
- link labels in either form, `A -- text --> B` (`-. text .->`, `== text ==>`) or
  `A -->|text| B`;
- chains (`A --> B --> C`) and groups (`A & B --> C & D`);
- `subgraph id`, `subgraph id [Title]`, `subgraph "Title"` and `subgraph Title with
  spaces`, nested, each closed by `end`, with links to and from a subgraph drawn to its
  border. A `direction` inside a subgraph is accepted and not honoured: the subgraph is
  drawn in the diagram's direction, which is what mermaid itself does whenever a link
  crosses the subgraph's border;
- labels, quoted (`A["text (with) brackets"]`) or not, with `<br>` as a line break and
  mermaid's entity codes (`#quot;`, `#35;`) as the characters they name; a long line
  wraps at a space;
- `%%` comments.

**How it is drawn.** Nodes are placed in ranks along the diagram's direction, like
mermaid's own layout. A link that points back against the flow runs in a straight lane
of its own outside the nodes: below the row in `LR` and `RL`, and to the right of the
column in `TB` and `BT`. It leaves and enters its nodes by the side that faces that
lane. A link from a node to itself is drawn on the other side, above the node or to
its left. Where the layout allows, the longest chain of links is drawn on one straight
line, and then each next-longest chain among the nodes that are left. A chain does not
continue through a node with more than two links out, or more than two in. A
subgraph's title sits at the place nearest the middle of its band that no link
crosses. In top-to-bottom and bottom-to-top diagrams, if there is no such place, the
subgraph is widened by the room the title lacks, up to twice. If that is not enough,
or the diagram runs across the page, the title stays in the middle with the link
through it ([#188](https://github.com/davison/md-notes/issues/188)).

**What is skipped.** `style`, `classDef`, `class`, `:::class`, `linkStyle` and `click`
are recognised and skipped whole, up to the end of their line or their `;`. The drawing
is in the theme's colours, and an image has nothing to click, so a styled flowchart
copied from elsewhere still draws — without whatever its colours meant
([#170](https://github.com/davison/md-notes/issues/170#issuecomment-5766109427)). One
edge of that is worth knowing: a skipped statement holding `--` or `-->` outside double
quotes is read as a link, and the whole diagram then shows as code. `fill:var(--x)`,
`font-family:'a--b'` in single quotes and a single-quoted `click` URL containing `-->`
all do it. Double quotes, or leaving the statement out, draws the diagram
([#170](https://github.com/davison/md-notes/issues/170#issuecomment-5766902241)).
Bidirectional-text controls and zero-width characters are dropped from labels, and so
are the tag characters that make the England, Scotland and Wales flags: those three
emoji draw as a plain black flag.

**What shows as code, and why.** Everything else stays the code block it would be
without this feature, and nothing is ever half-drawn:

- **another diagram type**, or a block that is not a flowchart at all;
- **a construct outside the subset**: `%%{init}%%` or any other `%%{…}%%` directive,
  front matter, `accTitle`, `accDescr` and `title`, `@{…}` node metadata, markdown
  labels, HTML in a label (anything but `<br>`), `fa:` icons, and a `direction` outside
  a subgraph. Each of these changes what is drawn, or passes styling or markup through,
  so it is refused rather than guessed at;
- **text that does not parse**, and a flowchart with nothing to draw;
- **a block over a bound.** A block is limited to 32 KiB of source, 200 nodes, 400
  links (counting each one an `&` expands to), 50 subgraphs nested at most 8 deep,
  links of at most 8 ranks, and 500 characters and 20 lines per label; and the working
  graph the layout builds — nodes, one point per rank each link crosses, and one per
  rank a subgraph spans — to 3,000. The flowcharts in real notes are nowhere near
  these; the reasons for each number are on
  [#170](https://github.com/davison/md-notes/issues/170#issuecomment-5766109839), with
  one of its timings [corrected](https://github.com/davison/md-notes/issues/170#issuecomment-5766322910);
- **a drawing that takes longer than 2 seconds**, which the layout checks for
  throughout and stops at.

Most of these are known when the note is rendered, and the page then asks for no image
at all. The two only drawing can find — the layout's size and the deadline — are found
later. The layout's size is usually found while the daemon answers for the note itself
(see *Sized before they load* below): a block the layout refuses is then taken out of
the list and shows as code from the start, with no image requested. The deadline, and
the size of a block the note's answer did not reach, are found when its image is asked
for, and the block goes back to code then. Either way the daemon remembers the refusal,
by the block's source, so a hostile block costs its two seconds once rather than at every view. The other side of
that is a block refused at the deadline only because the machine was busy at that
moment: it **stays code until the daemon restarts**, however often the note is opened
([#177](https://github.com/davison/md-notes/issues/177#issuecomment-5768152245),
captured as [#182](https://github.com/davison/md-notes/issues/182)).

**When the image cannot be fetched.** An image that fails for any reason — a refusal
while drawing, the note changed underneath it, the daemon stopped, the network gone —
takes itself out and leaves the code block, so there is never a broken-image icon. The
page does not ask for that drawing again: the block stays code, through live updates
too, until its source changes, the palette changes, or you **open the note again**,
which is how to retry after the network comes back. The page cannot tell a refusal from
a failed fetch, which is why it does not retry on its own
([#171](https://github.com/davison/md-notes/issues/171#issuecomment-5767530708)).
**Diagrams are not available offline**: drawings are under `/api/`, which the
[installed app's](#installing-the-app) worker never answers from its cache — and
neither is the note they belong to, so offline there is no rendered note to show one
in.

**Palettes.** There are three, and the image follows the page's own choice between
them, changing in place when the setting or the device's scheme changes:

| The page is | The diagram is drawn |
|-------------|----------------------|
| In the light scheme | In the light palette, the page's own colours |
| In the dark scheme | In the dark palette, the page's own colours |
| Under **Always use the light theme** | In the e-ink palette: black on white, 2-pixel lines, no greys |

The light override is the app's e-ink setting (see
[Display settings](#display-settings)), so it is the one signal that the screen may
have no backlight, and it gets the palette a panel draws best; on an ordinary screen
the only difference from the light palette is the weight of the lines. The operator
accepted that mapping, with no separate e-ink switch
([#171](https://github.com/davison/md-notes/issues/171#issuecomment-5767528754)). In
every palette text is held to 4.5:1 against what it sits on and lines to 3:1, by a
test.

**Live, sized and accessible.** A diagram follows its note on disk: an edit elsewhere
reaches the page through [live update](#live-update) and the changed diagram is drawn
again, while an unchanged one in the same note keeps its image and is not fetched
again. The image is drawn at its natural size, never scaled up, and shrinks to the
reading column when it is wider. Its `alt` text is the diagram's source, which is the
whole of what it says, in the syntax its author wrote. A search hit on the block
flashes the image. Images load lazily: one far below the part of the note on screen is
not fetched until you scroll towards it.

**Sized before they load.** So that a search hit or a line link lands on its target
rather than being pushed away as the drawings above it arrive, the reading view asks
for the note with each diagram's natural width and height, and the page reserves each
image's box before its drawing loads. The daemon learns a size by laying the block
out, and keeps it by the block's source, so an unchanged diagram is laid out once
however often its note is opened. It spends at most 150 ms, on two of its drawing
slots, measuring the blocks it has no size for yet, and then answers with what it has.
On the **first open of a note with many large diagrams**, some go out unmeasured and
are placed in stages as they load; the page re-centres the search hit or line each
time one above it arrives, and stops doing so as soon as you scroll, click or type.
Later opens find more of the sizes kept, until all of them are
([#177](https://github.com/davison/md-notes/issues/177#issuecomment-5768086074)).
Only the reading view asks for sizes: the editor, which fetches the note for its title
alone, costs the daemon no layout.

**Why this cannot run anything.** The daemon parses the block into plain data — node
shapes, links, label lines — and writes the SVG itself, from a fixed set of elements
and attributes, with every piece of the note's text escaped as character data; nothing
in it refers to anything, and no styling from the note reaches it. The page shows it
only through `<img>`, which runs no script and loads nothing, and never inline. And
every answer at a diagram URL, the refusals included, carries
`Content-Security-Policy: default-src 'none'; sandbox` and
`X-Content-Type-Options: nosniff`, so even a drawing opened directly as a page runs
nothing and is never sniffed into something that could. The attack payloads of
mermaid's published advisories are test cases in the package, each shown inert
([#170](https://github.com/davison/md-notes/issues/170#issuecomment-5765984674)).

**The cost to the daemon.** Drawings and refusals are kept in memory, up to 8 MiB and
512 of them; sizes and refusals, up to 32,768 of them, in a cache of their own; and a
note's list of flowcharts is kept by its size and modification time, so a page asking
for each of its diagrams costs a parse once. At most half the
machine's processors draw at once, and listing a note's flowcharts takes one of those
slots too, so a clipped note full of expensive blocks queues rather than taking every
core. One consequence: a note on a network or FUSE mount whose read hangs holds its
slot for as long as the read does, because a file read cannot be interrupted, and
other diagrams wait behind it if every slot is held. Rendering the note itself has the
same exposure
([#171](https://github.com/davison/md-notes/issues/171#issuecomment-5767530708)).

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

### In a narrower window

Between 961 and 1289 pixels of window width — a laptop screen, or a desktop window with
something else beside it — the three panes are two columns: the navigator with the
**Search & tags** pane underneath it on the left, each scrolling on its own, and the note
taking everything that is not the navigator. Nothing moves into a drawer and no control
changes; the side pane is a row rather than a column. 960 and below is the drawer, as
before.

1290 is where all three panes fit without any of it coming out of the note: the
navigator's 16rem, the note's 48rem reading width with the 2rem of padding either side of
it, and the side pane's 18rem, at the application's root font. From there the side pane is
a column on the right again — the layout that was there at every width above 960 before,
and where the note column was the one that paid: at 1100 pixels the note column was 590
pixels and the note inside it 530 of an intended 720.

One caveat if you are checking these figures against a window of your own. The reading
width is the note *column* less that 2rem of padding either side, and where the browser
draws a scrolling bar **in** the padding rather than over the text — Chromium on a desktop
does; a phone and macOS do not — the bar takes 10 more pixels of it. So the note first
reaches its full 720 at 1020 pixels with no bar drawn and at 1030 with one, and crossing
1290 costs it those 10 pixels back until 1300, the side pane having just taken its column.
The breakpoint is the layout's own arithmetic and leaves the bar out on purpose: a bar is
10 pixels in one browser, none in another, and absent altogether from a note too short to
scroll, so a breakpoint that assumed one would be wrong in all three places.

Below about 1020 pixels the note is still narrower than its reading width, because a
16rem navigator and 48rem of text do not fit in less than that. It gets every pixel that
is not the navigator, which is 270 more than it used to have.

### On a phone

Below 960 pixels of window width — a phone in either orientation, and a narrow
desktop window — the note takes the whole viewport under a compact top bar, in the
rendered view and in the editor alike. The two side panes move into one drawer:

- The **burger** at the left of the top bar opens the drawer on its **Notes** tab,
  which is the navigator, with the expanded directories, the tag filter and the
  **Recent first** toggle it has at any other width.
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
- **New note** stays in the top bar at these widths, as a square `+` between the
  `mdn` home link and the root's name, with the burger one place further left. It
  does not move into the drawer — the drawer is where a long list scrolled it out of
  sight in the first place. The drawer is modal, so while it is open the create
  control is behind its backdrop and outside its focus trap, like the magnifier and
  the gear: creating a note at these widths is "close the drawer, tap `+`", not "tap
  `+` from inside the drawer"
  ([#85](https://github.com/davison/md-notes/issues/85#issuecomment-5702099098)).

Search is a literal, case-insensitive phrase — what you type is what is matched.
Tags come from a frontmatter `tags` value (a list, or one string split on commas and
whitespace) and from inline hashtags: `#` at the start of a line or after whitespace,
followed by letters, digits, `_`, `-` or `/`, containing at least one letter, outside
fenced and inline code, lower-cased. Tags are collected per request, with no index.

### Installing the app

The UI ships a web app manifest and a service worker, so a browser reached over
HTTPS — the tailnet name behind `tailscale serve` — offers to install it, and
the installed app opens in its own window at the roots page with no browser bar.
`http://localhost:<port>` is a secure context too, so the same works on the
daemon's own machine. [Installing it on the
phone](sync.md#installing-it-on-the-phone) has the steps and what to expect of
it; this is what it is made of.

The manifest is `/manifest.webmanifest`: the name, the icons at 192 and 512
pixels and a maskable variant, `display: standalone`, `start_url: /` and
`scope: /`. The scope is the whole origin, so `/r/{slug}/…` — every note, every
root — is inside the installed window. The page asks for the manifest with
`crossorigin="use-credentials"`, without which a browser fetches it with no
cookie, is handed the tailnet login page, and concludes there is nothing to
install. The manifest carries one theme colour and one background colour
because that is all a manifest can carry; the page's two `theme-color` meta tags
carry the light and the dark one.

The service worker is `/sw.js`, registered after the page has loaded. What it
does with a request depends on what the request is for:

- **Nothing under `/api/`** is intercepted at all. The events stream is never
  buffered, a save reaches the daemon or fails, the login and the session
  cookie are never touched, and no API response is ever stored. Neither is
  `/login`, nor anything that is not a same-origin GET.
- **The hashed files under `/assets/`** are served from the cache when they are
  in it. Their names carry their content, so a cached one cannot be wrong.
- **The shell, the manifest and the icons** are fetched from the network first
  and cached as they arrive; the cache answers only when the network cannot.
  That is the `no-cache` rule above surviving the worker: a rebuilt shell is
  picked up the next time the app is opened, and a stale one is never served
  while the daemon is answering.

The cache is named after its own contents, so a new build is a new cache and
the old one is deleted whole when the new worker activates. The worker does not
push itself in front of a page that is already open: a tab running the previous
build keeps the previous worker, which is the only one still holding that
build's lazily loaded chunks.

With the daemon unreachable, a navigation is answered with the cached shell, so
the app opens at the route that was asked for and says the daemon is not
answering. The notes are not cached and are not available; nothing about the
notes is stored on the device.

### Display settings

The **gear** at the right of the top bar, at every width, opens a panel with two
switches. Both are kept in `localStorage`, so they are per device and per browser,
and neither is sent anywhere.

- **Always use the light theme.** The page uses the light palette whatever the
  device's `prefers-color-scheme` says. The syntax colouring in fenced code follows
  the same switch, so an overridden page is not left with dark-scheme tokens on a
  light background. It is applied by an inline script before the stylesheet paints,
  so overriding a dark device shows no frame of the dark scheme on load. Flowcharts
  follow it too, in the e-ink palette rather than the light one — see
  [Flowcharts](#flowcharts). Turning it off returns the page to the device's preference.
- **No animation.** No transitions, and no flash on the block a search hit scrolls
  to — the scroll itself still happens, centring the block. The same is true without
  the switch on a device that asks for reduced motion: `prefers-reduced-motion:
  reduce` is honoured on its own.

Tap targets are not a setting. Wherever the browser reports a coarse pointer or no
hover — a phone, a tablet, a stylus — or the window is below the narrow breakpoint,
every row and control that is tapped is at least 40 pixels tall: tree entries, the
navigator's **Recent first** toggle, tags
and the clear link, search hits and the search box, the drawer's tabs, the top bar's
buttons including **New note**, the note bar's including **Delete**, the frontmatter
disclosure, both dialogs' buttons, the create prompt's name box, and the home page's
**Remove** control on a recent root. Links *inside* a note are the
exception, and have to be — their size is the line of prose they sit in. Under a mouse
at a wide width the rows keep their compact density.

Both settings exist for an e-ink tablet, where a dark theme is grey on grey and
every animation is a slow visible repaint. [On an e-ink tablet](e-ink.md) covers
that device end to end, including the two ways to reach your notes from one.

### What holds these numbers

The figures in the two sections above are not only documented, they are measured on
every push. `make e2e` runs a suite of 83 checks under `ui/e2e` in headless Chromium
against the built daemon on a temporary root, and CI runs it as a job of its own: the
pane rectangles at four phone profiles and a desktop control, the 960-pixel
breakpoint walked at 959, 960 and 961, the 1290-pixel one walked at 1289 and 1290 —
the side pane under the navigator below it, a column of its own at it, and the note
exactly at its 720-pixel reading width either way, which is the figure for a browser
drawing no bar in the gutter, as the suite's own headless one does not — the drawer's
geometry and all four
of its close paths, the tag chip, the 40-pixel targets under a coarse pointer with the
mouse-driven window's density left alone, the scrollbars' computed width and colour on
the navigator, the note, the search pane and the editor's own scroller in both schemes,
under the light override and under a coarse pointer, with the thin bar itself measured
in a browser that draws one and its track colour read off the pane's edge pixels, the
light override applied with the
application bundle blocked — so nothing but the inline boot script can have applied
it — the flash suppressed by the setting and by `prefers-reduced-motion`, and a
second page load that fetches no asset bytes — that last one twice, once on the
machine as it is and once with the CPU slowed until Chromium reports the entry module
twice, because the bytes on the wire are the same either way and only the number of
responses is not. The same suite drives creating and
deleting a note end to end, in the wide layout and in the drawer layout: a name typed
into the prompt, a bare title landing in the open note's folder, a refusal corrected
in place, the new note reaching a second tab through the events stream, the delete
button holding one place across an edit, and a deletion that a cancelled confirmation
does not perform. It drives the navigator's two orders in both layouts — the choice
surviving a reload, a note saved in the app and a note rewritten on disk each moving
to the top with no reload — and the home page's **Remove** control: the confirmation
naming the folder, a cancelled removal that removes nothing, the files still on disk
afterwards, and a second tab landing on the home page when the root it was open on
goes. And it holds [the installable app](#installing-the-app) to Chrome's installability
criteria item by item — the manifest read back both over HTTP and out of the browser's
own parse, each icon measured from its own header, the worker activated at scope `/` —
together with the rules that matter about it: that no request under `/api/` is ever
answered from the worker's cache, that a rebuilt shell wins over the cached one while the
daemon is answering and the cached one answers when it is not, that the worker's cache is
named after its contents, and that the roots page and three `/r/` routes all say the
daemon is unreachable rather than that the root does not exist. It draws
[a flowchart](#flowcharts) in each of the three palettes, reading each palette's
background back out of the image, follows a change of the setting without a reload,
shrinks a wide diagram to the column, shows as code a block the layout refuses without
asking for an image, keeps the code block when the drawing cannot be fetched, redraws a
diagram edited on disk without fetching an unchanged one again, and opens a drawing
directly as a document to show it cannot run a script even with one spliced into it. At
desktop and Pixel 7 sizes, with every drawing held back until after the scroll, it
lands a line link centred on its target below twelve diagrams — inside the ninth, on a
block shown as code, on a paragraph, and on the paragraph when every drawing fails — and
inside and below sixty dense diagrams the daemon had no time to measure, one refused by
the layout at its image's request.
The viewports are the suite's own literals rather than Playwright's
device registry, whose numbers move between releases
([#78](https://github.com/davison/md-notes/issues/78#issuecomment-5701667426)). It
needs Chromium, which is a separate download; see the README's **Building** section.
It is not part of `make check`, which is what keeps a 150 MB browser off the ordinary
developer loop.

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
switching to the rendered view and back keeps the cursor position, the undo
history and the vim mode: a note left in insert mode comes back in insert mode.
The same holds wherever the editor is put back over the same text, including
after **Recreate the note** ([#112](https://github.com/davison/md-notes/issues/112)).

Every vim exit command saves, and none of them closes anything: `:w`, `:q`, `:q!`,
`:x` and `:wq` all mean "save now". There is no way to quit without saving, because
the editor never closes and autosave would have written the draft anyway.

Edits go to the original file through the [conditional save](#conditional-saves)
endpoint, so they are subject to the same root confinement and the same Host and
Origin guard as everything else, and the source is written back byte for byte —
frontmatter, hard tabs, trailing spaces, a byte-order mark and a missing final
newline all survive an edit untouched. The editor itself only ever edits: creating
and deleting a note are the two controls below, on their own endpoints, and the save
endpoint still has no create-on-missing path. Renaming a note is not in the
application at all.

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

Three files in the bundle are not part of that page load. `/manifest.webmanifest`
is served as `application/manifest+json`, `/sw.js` as JavaScript, and the icons as
PNG and SVG; none of them is under `/assets/`, so all of them carry `no-cache` and
an `ETag` like `index.html` does — a service worker that could be cached for a year
would keep a year-old shell with it. [Installing the app](#installing-the-app)
above says what the two of them do.

### Creating and deleting a note

**New note** sits in the top bar beside the `mdn` home link, at every width. It opens
a prompt that names the folder the note will land in and takes a title or a path;
nothing is written until it is confirmed.

- A name with no `/` is created in that folder. The folder is **the folder of the
  open note, and the root when no note is open** — the navigator has no folder
  selection of its own, only expansion, so the note you are reading is the one
  unambiguous statement of where you are
  ([#77](https://github.com/davison/md-notes/issues/77#issuecomment-5701434013)).
  The prompt names it before anything happens, as `docs/deep` or as "the root of
  this folder".
- A name containing `/` is a path under the root and ignores the folder.
- A final component with no markdown extension gains `.md`.
- Everything else is the daemon's to refuse, and its message appears in the prompt
  with the typed name still in the box, so correcting a name already taken is one
  edit rather than a retyped title.

On success the app routes to the path the daemon returned — its cleaned form, not
the string that was sent — and the pane opens in the **editor**, a note just created
being empty and named in order to write in it.

**Delete** sits at the right-hand end of the note bar, in the rendered view and in
the editor alike, and its place does not move between them. It opens a confirmation
naming the note's full path; cancelling by the button, by `Escape` or by the backdrop
sends nothing at all. When this tab holds unsaved changes to that note the
confirmation says they go with it, and confirming drops the session before the app
navigates, so no scheduled save can recreate the file behind the deletion
([#77](https://github.com/davison/md-notes/issues/77#issuecomment-5701434216)). The
app then goes to `/r/{slug}/`, the root's home, which is also the parent folder:
this application has no route for a folder. A draft of the same note in *another*
tab is answered by the existing [deleted-on-disk banner](#conflicts) and no second
dialog.

Both prompts are the application's own modal dialog rather than `window.prompt` and
`window.confirm`, which cannot show a refusal without losing what was typed and are
the browser's chrome rather than this page's, and rather than a native `<dialog>`,
which jsdom cannot mount in a unit test
([#77](https://github.com/davison/md-notes/issues/77#issuecomment-5701434444)). They
move focus in on opening, cycle `Tab` inside themselves, hand focus back to the
control that opened them, confirm on `Enter` and cancel on `Escape`. While a create
or a delete is in flight the dialog cannot be dismissed at all — `Escape` and the
backdrop do nothing, as the disabled buttons already say — because the request has
been sent and the daemon will act on it either way
([#77](https://github.com/davison/md-notes/issues/77#issuecomment-5701747937)).

Both changes reach the navigator through the [events stream](#live-update), in this
tab and in every other one, exactly as a file written by another tool does. Neither
path refetches the tree.

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
| `Deleted on disk` | The file was deleted while there were no unsaved edits; see [Conflicts](#conflicts) |

A failed save keeps the draft and shows the reason the daemon gave — a read-only
file reports `note or directory is not readable/writable`; the rest are the codes
in [Conditional saves](#conditional-saves). **Retry** sends it again, and so does
the next edit; nothing is discarded in between. A reason too long for the bar is cut
off with an ellipsis rather than widening it — the note bar never scrolls sideways,
at any width, and **Retry** keeps its place beside the message — and hovering the
message shows the whole of it. A save landing while the pane is in view mode
refreshes the rendered note.

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

A note **deleted** on disk under a draft raises a banner with **Recreate the note**,
**Copy draft** and **Discard draft**. The editor cannot save the draft back itself,
because the save API has no create-on-missing path, but **New note** can write it
again, and **Recreate the note** is that prompt opened with the lost note's path
already in the box and the draft as the new note's body: one confirmation puts the
file back and the editor carries on over it with nothing lost. It is still the
prompt — the name can be changed before it is confirmed, and a path taken again in
the meantime comes back as the ordinary "already exists" refusal, with the name kept
for correcting. **Copy draft** remains for a draft that is going somewhere else
entirely. The file recreated any other way — another tool, or **New note** under the
same name with different text — turns the conflict back into a changed one instead,
where **Keep my draft** writes the draft over it.

A note deleted on disk while its editor holds **no** unsaved edits is not a
conflict, since there is nothing of the reader's to protect. The bar reads
`Deleted on disk`, in the editor and in the rendered view, and a notice says the
note no longer exists. The text stays in the editor, because it may be the only
copy left, and **Recreate the note** is offered for it as above. Nothing is kept as
a draft: the page does not ask before closing and the tab shows no marker. When the
file comes back, whatever it holds, the note simply carries on from it and the
notice goes by itself. Typing into the note while it is gone starts a draft of a
note that does not exist, which is the deleted-on-disk conflict above
([#30](https://github.com/davison/md-notes/issues/30)).

A note deleted from *another* tab takes one of these two paths here: the banner when
this tab holds unsaved edits to it, `Deleted on disk` when it does not. Either way,
this is why deleting a note raises no second dialog in another tab
([#77](https://github.com/davison/md-notes/issues/77#issuecomment-5701434216)).

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
  naming the root, how many directories are covered, how many are not, why, and what
  to do about each cause;
- the root's event stream opens with a `status` event carrying the same numbers, and
  sends it again on the next keepalive tick after they change;
- the page shows a notice above the navigator whenever coverage is limited, naming
  whichever causes are in play and the remedy for each, so it is visible in the
  browser and not only in the daemon's log.

A third cause is not a limit at all. A directory the daemon may not read — a
subdirectory at mode `000`, say — cannot be watched however much of either pool is
left, so it is counted and named on its own, in the words the operating system used,
and no limit is offered for it. The log line reads `1 could not be watched:
permission denied (not the kernel limit — the daemon needs access to that
directory)`, the `status` event carries `"refused"` and `"reason"` beside `"failed"`,
and the notice ends `to cover them all, give the daemon access to the 1 directory it
could not watch (permission denied)`. The remedy is to make the directory readable —
`chmod` it, or change its owner — or to leave it out of the root; raising either
limit would change nothing. Anything else the filesystem refuses is reported the same
way, under whatever reason it gave.

A root whose watcher never started is the same story with nothing covered: its event
stream answers 503, and the page says live update is not available for that root and
that changes show up on reload. That notice is also the one place where removing a
root leaves a tab behind: with no stream to end, such a tab keeps the notice and its
tree instead of routing home — see [Roots](#roots).

None of this stops the daemon, and none of it disables live update for the rest of a
root. To cover a large root completely — where the budget or the kernel's pool is
what stands in the way — raise `max_watches` (or set it to `0` for no budget) and
raise the kernel's own limit to match:

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
| the per-root reads — `tree`, `note`, `diagram`, `source`, `raw`, `search`, `tags`, `events` | `DELETE /api/roots/{slug}` |
| | anything else under `/api/` |
| `PUT`, `POST` and `DELETE` on `/api/r/{slug}/source/{path…}` | |
| `POST /api/clip`, to a caller presenting the token | |
| the UI bundle and its client-side routes | |

Anything on the right answers `403 {"code":"loopback_only"}`. It is an allow-list,
not that one exclusion, so an endpoint added later is loopback-only until somebody
decides otherwise.

`POST /api/roots` is the one that matters: with it, a caller holding the credential
could register any directory on the machine and then read every file under it
through the raw endpoint. On loopback that is inside the premise below — anything
that can reach the port runs as you and can read those files anyway. Over the
tailnet it is not, so it stays on the machine. It is the only endpoint the allow-list
*names* on the right; everything else there is the default, an endpoint nobody has
considered under this heading yet — which is the difference between this list and
the one milestone three wrote.

`DELETE /api/roots/{slug}`, which milestone seven added, is one of those defaults
rather than a new exclusion: the allow-list admits reads of `/api/roots` and matches
nothing for a slug beneath it, so the endpoint was refused under the tailnet name
before it existed and nothing was added to the list to make it so
([#115](https://github.com/davison/md-notes/issues/115#issuecomment-5718216788)). It
is named in the table because a reader should not have to derive it. So milestone
seven widened nothing here: changing the set of roots — adding one or taking one
away — stays a thing that happens on the machine, and the `file` a registration may
now carry changes what the daemon checks before it registers, not who may ask.

`POST /api/clip` was on the right until milestone six. It was refused for a weaker
reason than root registration — not that a clip is dangerous, but that no write at
all crossed the name then, so nothing off the machine clipped
([#39](https://github.com/davison/md-notes/issues/39#issuecomment-5632388601)).
[M5-R2](milestones/5-create-and-delete-notes.md) removed that ground when it
admitted `POST /api/r/{slug}/source/{path…}`: a credential that can create a file at
any path inside any registered root is already wider than one that can add a file to
`clips_dir` at a name the *daemon* chooses. So the clip is admitted and the
extension pointed at the tailnet name clips into the notes
([#95](https://github.com/davison/md-notes/issues/95)). It still requires the bearer
token — a browser logged in at the tailnet name holds a session cookie, which has
never been enough for this endpoint — so in practice the caller is the extension,
holding the token it was given.

What that leaves the extension under a tailnet name is two of its three actions:
with the token pasted it clips, and it opens a local markdown file already inside a
registered root, because the roots listing it depends on carries the token whenever
the daemon URL is not loopback. Registering a folder meets the same `loopback_only`
as everybody else's, and the extension names that refusal rather than blaming the
token — see [the extension page](extension.md#a-daemon-reached-over-the-tailnet).
Reading notes from another device is still the UI's job in the browser there.

The three write methods on a note's source are admitted on one rule, because they are
one resource: a caller that can already replace a note's bytes is not meaningfully
restrained by being refused the right to create a sibling or remove one, and keeping
them together leaves the allow-list a statement about the `source` resource rather
than about which method is in the request
([#76](https://github.com/davison/md-notes/issues/76#issuecomment-5700724927)). That
applies to every registered root, `mdn open` ones included, for as long as they are
registered — but `POST /api/roots` stays loopback-only, so nothing reachable over the
tailnet can widen the set of roots it applies to.

So a remote device reads, searches, creates, edits, deletes and clips into the notes
the daemon already serves. It cannot add a root or remove one, and `mdn open` remains
a command for the daemon's own machine.

### The premise, restated

On loopback the daemon assumes a single-user machine: everything that can reach the
port already runs as the user who owns the notes. Under `tailnet_host` that is no
longer who is on the other end. The people and devices your **tailnet ACL admits to
this node** can reach the login page, and one of them holding the token can read,
search and edit every root the daemon serves — the notes root, every permanent root
and every folder added with `mdn open`, including any that was only ever meant to be
looked at locally — and add a clip to the notes root's clips directory. A second
`--root` or a `notes_root` list puts that whole folder within the token holder's
reach too.

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
  authenticated caller reaches is the UI's own API, plus `POST /api/clip` to a
  caller presenting the token: `POST /api/roots` and `DELETE /api/roots/{slug}` stay
  on the machine, and so does any endpoint added later until somebody decides
  otherwise.
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
the user who owns the notes — so any local process can list roots, register folders
and unregister them, and read files under them through the API without presenting
anything, and files the navigator and search hide are still readable by direct URL. That premise was raised
and accepted deliberately
([#2](https://github.com/davison/md-notes/issues/2#issuecomment-5572874194)). The
[bearer token](#authentication) does not change it: it exists so that a client which
cannot make a same-origin claim — the browser extension — can be told apart from a
web page that has merely found the port, and it is protected by the same `0600` the
state file has. Any local process running as the user can read the token file, and
is already inside the premise. Configuring `tailnet_host` does change it, and
[The premise, restated](#the-premise-restated) above says how.
