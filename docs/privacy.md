# Privacy policy for the md-notes browser extension

This is the privacy policy the Chrome Web Store listing for **md-notes** points
at. It covers the browser extension in [`extension/`](../extension); the daemon
it talks to is software you run on your own machine, and what that does with
your notes is your filesystem's business, not this policy's.

The short version: **the extension sends page content to one address, the one
you typed into its options, and stores two things, the address and the token.
Nothing goes anywhere else, and nothing goes to the author.**

Last updated 2026-09-18, for the extension built from
[davison/md-notes](https://github.com/davison/md-notes). The authority for every
claim below is the source in that repository, which you can read.

## Who collects what

Nobody, and nothing.

There is no server behind this extension. There is no account, no sign-in, no
analytics, no telemetry, no crash reporting, no advertising identifier, no
third-party SDK, and no remote code: everything the extension runs is in the
package the store served you. The author of md-notes receives nothing when you
install it, use it, or uninstall it, and has no way to.

## What leaves your browser, and where it goes

One destination: **the daemon URL you set on the extension's options page.**
That is `http://localhost:7337` by default — a program on your own machine —
and you may point it at another address, in practice your own machine over a
Tailscale network. The extension talks to that address and to no other.

Three things go there, and only when you ask for them:

| When | What is sent | Where |
| --- | --- | --- |
| You clip a page or a selection | The page's address, its title, and its content converted to markdown | Your daemon URL |
| You open a local `.md` file in the app | The file's path, and the folder it is in if that folder is not yet registered | Your daemon URL |
| The extension needs to know where your notes are | Nothing but the request itself | Your daemon URL |

A clip is prepared when you press **Clip page** or **Clip selection**, and it is
**not sent until you press Save** — you see the title and the address it came
from first, and **Discard** throws it away. The extension reads the content of a
page only at the moment you invoke it on that page, and only that page: it
registers no content scripts, so no code of the extension's is running in the
pages you browse.

Your token is sent to that same daemon URL, as the `Authorization` header that
proves to your own daemon that the request is yours.

Nothing else leaves the browser. There is no fallback address, no "anonymous
usage statistics", and no second request made alongside the first.

## What is stored, and where

In the browser's own extension storage, on your computer:

- **`daemonUrl`** — the address you typed on the options page.
- **`token`** — the bearer token printed by `mdn token`, which you paste there.

That is the whole of it. Both live in `chrome.storage.local`, which belongs to
this extension and is readable by no website and no other extension. If you have
Chrome or Brave profile sync switched on, your browser may carry them between
your own devices; that is the browser's mechanism and its policy, and the
extension asks for no synced storage of its own.

Two further things are held in `chrome.storage.session`, which exists only until
the browser is closed and never reaches disk: the clip you have prepared but not
yet saved, and the one-line reason the toolbar badge is showing something, per
tab.

Nothing is stored anywhere else. There are no cookies, no `localStorage`, no
IndexedDB, no files written outside the browser.

## Why the extension asks for what it asks for

Each permission and the single thing it is for. The listing carries the same
justifications; the fuller account is in
[the extension documentation](extension.md#permissions-and-why-each-one).

- **`storage`** — to keep the two values above.
- **`activeTab`** — to read the page you invoked a clip on, at the moment you
  invoke it, and no other page.
- **`scripting`** — to put the extractor and the markdown converter into that
  one tab, because converting a page needs a live DOM and a selection exists
  nowhere else.
- **`contextMenus`** — to put the two **Clip … to md-notes** entries in the
  right-click menu.
- **`http://localhost:7337/*`, `http://127.0.0.1:7337/*`** — to reach the daemon
  at its default address.
- **`file:///*`** — to notice that a tab has navigated to a local markdown file,
  so it can be opened in the app instead of shown as plain text. This is inert
  until you switch **Allow access to file URLs** on for the extension yourself,
  and it is used to read the address of such a tab, never the contents of your
  filesystem.
- **`http://*/*`, `https://*/*` (optional)** — not granted at install. Requested,
  with the browser's own prompt, only if you set a daemon URL that is not the
  default one, which is the only way the extension can be allowed to reach the
  address you chose. Granting it lets the extension reach that address; it does
  not make the extension visit anything else, and it never has.

## What the extension does not do

- It does not sell or transfer your data to anybody, because it transfers your
  data to nobody.
- It does not use your data for anything unrelated to its single purpose:
  putting a web page into your own notes, and opening your own notes in your own
  app.
- It does not use your data to determine creditworthiness or for lending.
- It does not read pages you have not asked it to clip.
- It loads no remote code.

## Children

The extension is a tool for your own files. It is not directed at children, and
it collects nothing from anybody, of any age.

## If this changes

Any change to what is sent or stored is a change to this file, and this file is
in the repository's history: what it said on any date, and the commit that
changed it, are public. The store listing points at this page, so the version
you are reading is the one that describes the published extension.

## Contact

Open an issue at
[github.com/davison/md-notes/issues](https://github.com/davison/md-notes/issues).
