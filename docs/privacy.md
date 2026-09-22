# Privacy and the browser extension

What the md-notes browser extension in [`extension/`](../extension) does with
the pages you visit, what it sends and where, and what it keeps. It is written
for the person about to load it: an unpacked extension that asks for `file:///*`
and, optionally, for every http and https address deserves an account of what it
does with them. The daemon it talks to is a program you run on your own machine,
and what that does with your notes is your filesystem's business, not this
page's.

The short version: **the extension sends page content to one address, the one
you typed into its options, and keeps two settings, that address and your
token. Nothing goes anywhere else, and nothing goes to the author.**

Every claim below can be checked against the source, which is the authority for
all of them: the extension you load is `mdn-extension-<tag>.zip` from a release,
built from that tag, so the version of this page at the same tag is the one that
describes it.

## Who collects what

Nobody, and nothing.

There is no server behind this extension. There is no account, no sign-in, no
analytics, no telemetry, no crash reporting, no advertising identifier, no
third-party SDK, and no remote code: everything the extension runs is in the
zip you unpacked, and it loads no script from anywhere else and evaluates no
code it fetched. The author of md-notes receives nothing when you install it,
use it, or remove it, and has no way to.

## What leaves your browser, and where it goes

One destination: **the daemon URL you set on the extension's options page.**
That is `http://localhost:7337` by default — the daemon on your own machine —
and you may point it at another address, in practice your own machine reached
over your Tailscale network. The extension talks to that address and to no
other.

| When | What is sent | Where |
| --- | --- | --- |
| You save a clip of a page or a selection | The page's address, the title you saw in the popup, the content converted to markdown, and whether it was a page or a selection | Your daemon URL |
| A tab opens a local `.md` or `.markdown` file | A request for the list of registered folders; if the file is in none of them, its folder and its name, so the daemon can register that folder; then the tab itself goes to the note's address on the daemon | Your daemon URL |
| You press **Test connection** on the options page | A request for the list of registered folders, and — against a daemon on this machine — the same request with a made-up token the daemon cannot have issued, to learn whether it checks tokens at all | Your daemon URL |
| You press **Open the note** after a clip | A new tab at the note's address in the app | Your daemon URL |

A clip is prepared when you press **Clip page** or **Clip selection**, in the
popup or in the page's right-click menu, and it is **not sent until you press
Save to notes** — you see the title and the address it came from first, and
**Discard** throws it away. The extension reads a page's content only at the
moment you invoke a clip on that page, and only that page: it registers no
content scripts, so none of its code is running in the pages you browse.

Your token goes to that same daemon URL, as the `Authorization` header that
proves to your own daemon that the request is yours: on every request to a
daemon that is not on this machine, and to one that is, on the requests that
change something (registering a folder, saving a clip) and on the connection
test.

Nothing else leaves the browser. There is no fallback address, no "anonymous
usage statistics", and no second request made alongside the first.

## What is kept, and where

In the browser's own extension storage (`chrome.storage.local`), on this
computer:

- **`daemonUrl`** — the address you typed on the options page.
- **`token`** — the bearer token printed by `mdn token`, which you paste there.

That is the whole of it. This storage belongs to the extension and is readable
by no website and no other extension, and it is not the synced kind: the
extension uses no `chrome.storage.sync`, so the two settings stay on the
machine you typed them into.

Two more things are held in `chrome.storage.session`, which lives in memory and
is gone when the browser closes:

- the clip you have prepared but not yet saved — its address, title, markdown
  and kind — so that it survives the popup closing between **Clip** and
  **Save to notes**;
- per tab, the last thing the extension did there: the `file:` address it
  acted on, if any, and a one-line result — opened in the app, or why not —
  which is what the popup shows when the toolbar badge has something to say.
  It is dropped when the tab moves on to another page.

The extension keeps nothing else: no cookies of its own, no `localStorage`, no
IndexedDB, and no files. (The daemon's app, once a tab has been sent to it, is
a web page on your daemon's address like any other, and the login cookie it
sets over the tailnet is the daemon's.)

## Why the extension asks for what it asks for

Each permission, and the single thing it is for. The fuller account, written
for a sceptical reader, is
[Permissions, and why each one](extension.md#permissions-and-why-each-one).

- **`storage`** — to keep the two settings above, and the session values beside
  them.
- **`activeTab`** — to read the page you invoked a clip on, at the moment you
  invoke it, and no other page.
- **`scripting`** — to put the extractor and the markdown converter into that
  one tab, because converting a page needs a live DOM and a selection exists
  nowhere else.
- **`contextMenus`** — for the two **Clip … to md-notes** entries in the
  right-click menu.
- **`http://localhost:7337/*`, `http://127.0.0.1:7337/*`** — to reach the daemon
  at its default address.
- **`file:///*`** — to notice that a tab has navigated to a local markdown file,
  so it can be opened in the app instead of shown as plain text. This does
  nothing until you switch **Allow access to file URLs** on for the extension
  yourself, and it is used to read the address of such a tab, never the
  contents of your filesystem.
- **`http://*/*`, `https://*/*` (optional)** — not granted when you load the
  extension. Requested, with the browser's own prompt, only when you save a
  daemon URL that is not the default one, and then only for that one address,
  which is the only way the extension can be allowed to reach it. It does not
  make the extension visit anything else.

## What the extension does not do

- It does not sell or pass your data to anybody, because it sends your data to
  nobody but your own daemon.
- It does not read pages you have not asked it to clip.
- It does not watch your browsing: it asks for neither `tabs` nor
  `webNavigation`, so the browser shows it the address of a tab only where it
  holds a permission for that address — a local file, your daemon, or the one
  tab you invoked a clip on.
- It loads no remote code.

## If this changes

A change to what is sent or kept is a change to this page as well as to the
code, and both are in the repository's history: what this page said on any
date, and the commit that changed it, are public.

## Contact

Open an issue at
[github.com/davison/md-notes/issues](https://github.com/davison/md-notes/issues).
