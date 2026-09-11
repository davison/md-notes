# The browser extension

A Chromium Manifest V3 extension for Brave, in [`extension/`](../extension).
It is the browser half of md-notes: it opens local markdown files in the app
instead of letting the browser render them as plain text, and it clips a web
page or a selection into the notes root as markdown.

This page covers building it, loading it, the file URL permission, the token,
clipping, and every permission it asks for.

## Building

The extension is built from the repository, alongside the daemon:

```
make extension
```

That produces two things:

- `extension/dist/` — the unpacked extension, which is what you load into a
  browser during normal use.
- `extension/mdn-extension.zip` — the same tree zipped, for copying to another
  machine or uploading.

`make check` typechecks the extension, runs its unit tests and builds it, so a
broken extension fails CI like anything else.

## Loading it in Brave

Brave is Chromium, so the Chrome flow applies unchanged.

1. Open `brave://extensions`.
2. Turn on **Developer mode** (top right).
3. Click **Load unpacked** and choose `extension/dist` from this repository.
4. The extension appears as **md-notes**. Pin it to the toolbar if you want
   the badge visible.

Rebuilding (`make extension`) rewrites `extension/dist` in place; press the
reload arrow on the extension's card afterwards to pick the new build up.

The same steps work in Chrome and Chromium at `chrome://extensions`.

## Allowing access to file URLs

Opening a local markdown file needs one permission the browser will not grant
from the manifest: extensions cannot see `file:` URLs unless the user says so
per extension.

1. On `brave://extensions`, click **Details** on md-notes.
2. Turn on **Allow access to file URLs**.

Without it the extension is simply quiet: navigating to a `.md` file renders
plain text as before, and nothing is registered with the daemon. The options
page says which of the two states you are in.

## The daemon URL and the token

Open the extension's **Options** (from the popup, or **Details → Extension
options**):

- **Daemon URL** — `http://localhost:7337` by default, which is the daemon's
  own default port. If your daemon listens somewhere else, type it here; the
  browser asks for permission for that address when you save, because the
  manifest only grants the default (see below).
- **Token** — the installation's bearer token, printed by `mdn token`. Paste
  it and save.

> **Not yet.** The extension is loaded unpacked from this repository; it is
> not in any store. The inbox clipper for URLs shared from a phone is later
> work and is not here. Everything else on this page works against a daemon
> built from `main`.

The daemon URL may name a daemon reached over the tailnet rather than one on this
machine, and the browser will ask for permission for that address — but **nothing
the extension does works under such a name today**, whatever token is pasted.

- Clipping and registering a new folder are served on loopback only, so both are
  refused with `loopback_only` under a `tailnet_host` name. That is deliberate; see
  [What is reachable under that name](introduction.md#what-is-reachable-under-that-name-and-what-is-not).
- Opening a local file fails too, including one already inside a registered root.
  The extension asks the daemon for its roots *without* the token — a GET from its
  background context carries no `Origin`, so on loopback the guard lets it through
  and the intercept works with nothing pasted — and under `tailnet_host` nothing is
  served unauthenticated, so that request is `401` and the attempt ends there. The
  badge and the popup then say **the daemon rejected the token**, which is
  misleading: the token is fine, it was simply never sent.
- **Test connection** below fails there too, with that same message and for that
  same reason: its first question — is a daemon there at all? — is the same
  unauthenticated roots read, so it never reaches the read that carries your token.
  No token makes it succeed against such a name, and an empty one gives the same
  answer. So nothing in the extension tells a tailnet daemon URL apart from a token
  that is genuinely wrong.

Reading your notes from another device is the app's job, not the extension's: open
`https://<tailnet_host>/` in the browser there and log in. Leave the extension's
daemon URL pointing at the daemon on this machine.
[#51](https://github.com/davison/md-notes/issues/51) is the capture for making the
extension work against a tailnet name.

**Test connection** asks the daemon for its roots and says what came back. If
a token is stored it also presents one the daemon cannot have issued, to find
out whether this daemon judges tokens at all, and only then reports yours as
accepted or rejected — a daemon old enough to predate the token answers
everything, so a plain success would say nothing about your token.

What needs the token, and what does not:

| Action | Token | Works |
|--------|-------|-------|
| Reading the list of roots | not needed — a GET from the extension's background context carries no `Origin` header, so the daemon's guard lets it through | yes |
| Opening a file that is already inside a registered root | not needed | yes |
| Registering a new folder as a root | **needed** — the POST carries `Origin: chrome-extension://…`, which the daemon refuses without the token | yes |
| Clipping a page or a selection | **needed** — `POST /api/clip` requires it | yes |

So the extension works for notes already under a registered root before you
paste anything, and says what is missing the first time it needs to register
a folder.

## Clipping a page or a selection

Two ways in, both doing the same thing:

- **The toolbar button.** Click **md-notes** and choose **Clip page** or
  **Clip selection**.
- **The page's right-click menu.** **Clip page to md-notes** is there on any
  page; **Clip selection to md-notes** appears when you right-click a
  selection. A menu click prepares the clip and opens the popup over it. On a
  browser too old for an extension to open its own popup, the toolbar icon
  gains a blue **1** instead — click it and the clip is waiting.

Either way the popup then shows what it has: whether it is a page clip or a
selection clip, the address it came from, and the title in a field you can
edit. **Save to notes** writes it; **Discard** throws it away. Nothing is
written until you press Save.

On success the popup names the note and offers **Open the note**, which opens
`/r/<root>/<path>` in the app on your configured daemon. On failure it says
what went wrong and, where the remedy is a setting, offers **Options**. The
clip is kept either way, so you can start the daemon or paste a token and
press **Save to notes** again without finding the page a second time.

### What ends up in the note

**Clip page** runs Mozilla's Readability over the page first, so the note is
the article rather than the navigation, the sidebar and the footer. On a page
Readability declines — a dashboard, a search result, an index — the whole
`<body>` is converted instead, which is worth more than a refusal. Readability
drops the page's own `<h1>` when it is the article's title; the title is in the
frontmatter, and it is the one the popup offers you to edit.

**Clip selection** converts exactly what is selected, and nothing around it.

The conversion is Turndown with its GFM plugin:

- ATX headings (`#`, `##`), `-` bullets, nested and ordered lists;
- inline links, and fenced code with its language where the page named one in
  a `language-…`, `lang-…` or `data-lang` attribute;
- GFM tables, strikethrough and task lists;
- `**bold**`, `_italic_`, `` `code` `` and block quotes.

Two edges of that list are worth knowing. Task-list checkboxes survive a
**selection** clip as `- [x]`, but not a page clip: Readability's sanitiser
removes the `input` elements before the conversion sees them, so the items come
through as ordinary bullets. And a table with **no header row** is left as the
page's own `<table>` HTML, because GFM has no way to write one — the app renders
it, but the file is not markdown at that point
([#45](https://github.com/davison/md-notes/issues/45)).

Every link and image is made **absolute against the page's own URL**, so a note
still points at something once it has left the browser. Three deliberate
omissions: a `javascript:` link keeps its text and loses the link; a `data:`
image is dropped rather than carrying base64 into the note; `script`, `style`
and `noscript` never appear.

Where the note lands, what it is called and what its frontmatter says are the
daemon's doing, not the extension's — see
[Clipping a web page](introduction.md#clipping-a-web-page). In short: under
`clips/` in the notes root, named for the date and a slug of the title, with
`title`, `source`, `clipped` and `tags: [clip]` above the markdown.

### Where the work happens

Worth knowing, because it is what the permissions below are for:

1. The extension injects the extractor and the converter into the tab you are
   clipping, and they run there — Readability and Turndown both need a live
   DOM, and a selection exists nowhere else. This needs `scripting`, and
   access to that one tab, which is what `activeTab` grants at the moment you
   invoke the extension on it.
2. The daemon call is made from the extension's **service worker**, never
   from the popup — not because the popup could not make it, but because the
   worker owns the clip and outlives the popup. A popup is destroyed the
   moment it loses focus, which would take an in-flight save with it, and a
   clip taken from the right-click menu has no popup open at that point at
   all. Keeping the call in one context also keeps the token in one context.

   CORS exemption in an extension follows `host_permissions`, and it covers
   every **extension context** — the service worker and extension pages such
   as the popup and the options page alike. (The options page's **Test
   connection** is exactly such a call.) What it does *not* cover is a
   **content script** or an ordinary **web page**: those are governed by
   CORS, the daemon answers no preflight and sends no
   `Access-Control-Allow-Origin`, so a page that has merely found the port
   cannot write to your notes even if it has somehow obtained the token. That
   is the boundary the daemon draws, and the extension's own documents are on
   the inside of it.

### When a clip fails

| What you see | What happened |
|--------------|---------------|
| `No token configured.` | Nothing is pasted on the options page, or the daemon refused the extension's origin because no token was presented. Run `mdn token` and paste it. |
| `Token rejected: …` | The daemon has a different token. `mdn token` prints the current one; `mdn token --rotate` changed it. |
| `Daemon not reachable at …` | Nothing is listening there. Start `mdn serve`, or correct the daemon URL. |
| `This page cannot be clipped (…)` | Chromium does not let an extension run in browser pages (`chrome://`, `brave://`), the extensions gallery or the PDF viewer. |
| `Nothing is selected on this page.` | **Clip selection** with no selection. |
| `There is no text on this page to clip.` | The page converted to nothing at all. |
| The daemon's own words, such as `markdown is too large` | The clip endpoint refused it; its message is passed through unchanged. |

## Opening a local markdown file

With file URL access on and the daemon running, navigating to a `file:` URL
ending in `.md` or `.markdown` — from a file manager, a terminal's
`xdg-open`, or a link — opens that note in md-notes instead:

1. The extension maps the `file:` URL to its absolute path.
2. It asks the daemon for its roots. If one of them already contains the file,
   the tab goes to that note under that root: `/r/<slug>/<path in the root>`.
   Where two roots overlap, the deepest wins.
3. Otherwise it registers the file's own directory as a new root — the same
   thing `mdn open DIR` does — and goes to the note there. Registering a
   directory the daemon already serves under another name hands back the
   existing root rather than duplicating it, because the daemon compares real
   paths. **This step needs the token**; without one the page is left alone
   and the badge and popup say so.

The mapping in step 1 assumes POSIX paths — `file:///home/you/notes/a.md`
becomes `/home/you/notes/a.md`. That is the daemon's world: it runs on Linux
and takes absolute POSIX directories. A `file:` URL from another kind of
filesystem, such as `file:///C:/Users/me/note.md`, maps to a path the daemon
will simply not find, and the failure is the daemon's "no such file".

For the same reason the mapping is strict about what a path may contain. A
percent escape that decodes into a separator, a `.` or `..` segment, or a NUL
is refused outright rather than resolved, because the directory this step
derives is the one the daemon is asked to serve, and a crafted URL must not
get to choose it. (A `//` run is collapsed rather than refused: it names the
same file and escapes nothing.) One consequence worth knowing: a file whose
name contains a literal backslash is left alone.

A URL the mapping refuses is treated exactly like a URL that is not markdown
at all — the page is left alone and nothing is said, no badge and no popup
message. The failures below are the ones that happen *after* a URL has been
accepted.

When any of that fails the tab is left exactly as it was, showing the plain
text the browser was going to show anyway, and the toolbar icon gains a red
`!`. The popup says why:

| What you see | What happened |
|--------------|---------------|
| `daemon not reachable at http://localhost:7337` | Nothing is listening. Start `mdn serve`, or fix the daemon URL in the options. |
| `cross-origin request refused: no token is stored` | The file is not in any registered root, and registering one needs the token. Run `mdn token` and paste it — or register the folder with `mdn open DIR` instead. |
| `the daemon refused the extension's origin even with a token` | The daemon does not know about tokens: it was built before the token existed. Rebuild it, or use `mdn open DIR`. |
| `the daemon rejected the token` | The token is wrong or has been rotated — `mdn token` prints the current one. It also appears, misleadingly, when the daemon URL names a `tailnet_host`, where the roots call is refused for carrying no token at all ([#51](https://github.com/davison/md-notes/issues/51)). |
| `path must be absolute`, `not a directory` | The daemon refused the folder; its own message is passed through. |

Whatever the reason, the page itself is untouched. Fix the cause and reload
the tab: the extension tries again on every load of a local markdown file,
including a reload of one that failed.

A `file:` URL naming another machine (`file://server/share/note.md`) is left
alone: it is not a path the daemon could register.

## Permissions, and why each one

The manifest asks for the least that makes the above work. From
[`extension/public/manifest.json`](../extension/public/manifest.json):

| Permission | Why |
|------------|-----|
| `storage` | The daemon URL and token on the options page, the per-tab status the popup reports, and the clip waiting to be saved. |
| `contextMenus` | The two **Clip … to md-notes** entries in the page's right-click menu. |
| `activeTab` | Reading the page you are clipping — one tab, granted at the moment you invoke the extension on it, and gone again when that tab navigates. It is why the extension can read the page you asked it to clip and no other. |
| `scripting` | Putting the extractor and the markdown converter into that tab. `activeTab` says *which* page may be read; `scripting` is what allows code to be run in it at all. |
| `host_permissions: http://localhost:7337/*`, `http://127.0.0.1:7337/*` | Talking to the daemon. Chromium enforces the port, so this grants no access to any other service on your machine. |
| `host_permissions: file:///*` | Seeing that a tab has navigated to a local markdown file. Inert until you switch **Allow access to file URLs** on. |
| `optional_host_permissions: http://*/*`, `https://*/*` | Not granted at install. Only requested, with the browser's own prompt, if you set a daemon URL that is not the default — a different port, or a name reached over the tailnet — though no extension action works under a tailnet name today ([#51](https://github.com/davison/md-notes/issues/51); see above). |

Deliberately **not** asked for:

- `tabs` and `webNavigation` — either would report every navigation in the
  browser. The file-URL intercept listens on `chrome.tabs.onUpdated`, which
  delivers a tab's URL on the strength of `file:///*` alone.
- `<all_urls>` — the extension never needs to reach an arbitrary site.
- Content scripts — none are registered, so no code of the extension's runs in
  a page unless you invoke a clip on it, and then only in that tab.
- Web-accessible resources — nothing in the extension is reachable from a web
  page.

## Testing it

Unit tests (`pnpm --dir extension test`, and part of `make check`) cover the
URL-to-path mapping, root matching, the app URL it builds, the daemon calls
and their failures, the HTML-to-markdown conversion on fixtures, the clip
request built from an extraction, every failure the popup has to tell apart,
and the manifest's permission set.

There are also two end-to-end checks that load the built extension into
headless Chromium against a real daemon on a temporary root — one for the
file-URL intercept, one for clipping a page and a selection from a local
static page. They need a Chromium binary and a built `mdn`, so they are not
part of `make check`; they skip themselves, saying which prerequisite is
missing, when those are absent:

```
make build extension
PLAYWRIGHT_ROOT=/path/to/a/playwright/install pnpm --dir extension e2e
```
