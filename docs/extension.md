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

**Test connection** asks the daemon for its roots and reports what came back.

What needs the token, and what does not:

| Action | Token |
|--------|-------|
| Reading the list of roots | not needed — a GET from the extension's background context carries no `Origin` header, so the daemon's guard lets it through |
| Opening a file that is already inside a registered root | not needed |
| Registering a new folder as a root | **needed** — the POST carries `Origin: chrome-extension://…`, which the daemon refuses without the token |
| Clipping a page or a selection | **needed** |

So the extension works for notes already under a registered root before you
paste anything, and tells you to paste the token the first time it needs to
register a folder. (The token itself, and `mdn token`, come with the daemon's
authentication work in this same milestone — requirement M3-R1.)

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
   paths.

When any of that fails the tab is left exactly as it was, showing the plain
text the browser was going to show anyway, and the toolbar icon gains a red
`!`. The popup says why:

| What you see | What happened |
|--------------|---------------|
| `daemon not reachable at http://localhost:7337` | Nothing is listening. Start `mdn serve`, or fix the daemon URL in the options. |
| `cross-origin request refused: no token is stored` | The file is not in any registered root, and registering one needs the token. Run `mdn token` and paste it. |
| `the daemon rejected the token` | The token is wrong or has been rotated. `mdn token` prints the current one. |
| `path must be absolute`, `not a directory` | The daemon refused the folder; its own message is passed through. |

A `file:` URL naming another machine (`file://server/share/note.md`) is left
alone: it is not a path the daemon could register.

## Permissions, and why each one

The manifest asks for the least that makes the above work. From
[`extension/public/manifest.json`](../extension/public/manifest.json):

| Permission | Why |
|------------|-----|
| `storage` | The daemon URL and token on the options page, and the per-tab status the popup reports. |
| `contextMenus` | The right-click **Clip page** / **Clip selection** entries. |
| `activeTab` | Reading the page you are on, only at the moment you invoke the extension on it. |
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
