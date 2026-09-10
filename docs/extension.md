# The browser extension

A Chromium Manifest V3 extension for Brave, in [`extension/`](../extension).
It is the browser half of md-notes: it opens local markdown files in the app
instead of letting the browser render them as plain text, and — with the
clipper task — saves a page or a selection into the notes root.

This page covers what exists now: building it, loading it, the file URL
permission, the token, and every permission it asks for.

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

> **Not yet.** `mdn token` does not exist on `main` at the time of writing:
> the daemon's bearer token is requirement M3-R1, landing in this same
> milestone. Until it does, the daemon has no token to print and ignores the
> header, so everything in this page that needs the token — registering a new
> folder as a root, and clipping — cannot work yet, and the extension says so
> rather than failing quietly. What does work today is opening a file that is
> already inside a root the daemon serves.

**Test connection** asks the daemon for its roots and says what came back. If
a token is stored it also presents one the daemon cannot have issued, to find
out whether this daemon judges tokens at all, and only then reports yours as
accepted or rejected — a daemon that predates M3-R1 answers everything, so a
plain success would say nothing about your token.

What needs the token, and what does not:

| Action | Token | Works today |
|--------|-------|-------------|
| Reading the list of roots | not needed — a GET from the extension's background context carries no `Origin` header, so the daemon's guard lets it through | yes |
| Opening a file that is already inside a registered root | not needed | yes |
| Registering a new folder as a root | **needed** — the POST carries `Origin: chrome-extension://…`, which the daemon refuses without the token | not until M3-R1 |
| Clipping a page or a selection | **needed** | not until M3-R1 and M3-R2 |

So the extension works for notes already under a registered root before you
paste anything, and says what is missing the first time it needs to register
a folder.

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
   paths. **This step needs the token, so it does not work until M3-R1 lands**
   (see the box above); the badge and popup say exactly that when you try it.

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
| `cross-origin request refused: no token is stored` | The file is not in any registered root, and registering one needs the token. Run `mdn token` and paste it — or, until M3-R1 lands, register the folder with `mdn open DIR` instead. |
| `the daemon refused the extension's origin even with a token` | The daemon does not know about tokens yet: it predates M3-R1. `mdn open DIR` registers the folder in the meantime. |
| `the daemon rejected the token` | The token is wrong or has been rotated. `mdn token` prints the current one. |
| `path must be absolute`, `not a directory` | The daemon refused the folder; its own message is passed through. |

Whatever the reason, the page itself is untouched. Fix the cause and reload
the tab: the extension tries again on every load of a local markdown file,
including a reload of one that failed.

A `file:` URL naming another machine (`file://server/share/note.md`) is left
alone: it is not a path the daemon could register.

## Permissions, and why each one

The manifest asks for the least that makes the above work, plus the two the
clipper will need. Neither of those two produces an install-time warning or a
re-prompt, so declaring them now costs the user nothing and saves reloading
the extension mid-milestone; both are marked below. From
[`extension/public/manifest.json`](../extension/public/manifest.json):

| Permission | Why |
|------------|-----|
| `storage` | The daemon URL and token on the options page, and the per-tab status the popup reports. |
| `contextMenus` | Held for the clipper's right-click **Clip page** / **Clip selection** entries (M3-R3). Nothing in the file-URL work uses it yet. |
| `activeTab` | Held for the clipper: reading the page you are on, and only at the moment you invoke the extension on it (M3-R3). Nothing uses it yet. |
| `host_permissions: http://localhost:7337/*`, `http://127.0.0.1:7337/*` | Talking to the daemon. Chromium enforces the port, so this grants no access to any other service on your machine. |
| `host_permissions: file:///*` | Seeing that a tab has navigated to a local markdown file. Inert until you switch **Allow access to file URLs** on. |
| `optional_host_permissions: http://*/*`, `https://*/*` | Not granted at install. Only requested, with the browser's own prompt, if you set a daemon URL that is not the default — a different port, or a name reached over the tailnet. |

Deliberately **not** asked for:

- `tabs` and `webNavigation` — either would report every navigation in the
  browser. The file-URL intercept listens on `chrome.tabs.onUpdated`, which
  delivers a tab's URL on the strength of `file:///*` alone.
- `<all_urls>` — the extension never needs to reach an arbitrary site.
- Content scripts — none are registered, so no code of the extension's runs in
  a page unless you invoke it.
- Web-accessible resources — nothing in the extension is reachable from a web
  page.

## Testing it

Unit tests (`pnpm --dir extension test`, and part of `make check`) cover the
URL-to-path mapping, root matching, the app URL it builds, the daemon calls
and their failures, and the manifest's permission set.

There is also an end-to-end check that loads the built extension into headless
Chromium against a real daemon on a temporary root. It needs a Chromium binary
and a built `mdn`, so it is not part of `make check`; it skips itself, saying
which prerequisite is missing, when they are absent:

```
make build extension
PLAYWRIGHT_ROOT=/path/to/a/playwright/install pnpm --dir extension e2e
```
