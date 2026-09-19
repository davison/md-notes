# The Chrome Web Store listing, in the repository

The Web Store API can upload a package and submit it for review. It cannot
create the item and it cannot set a single listing field — the name, the
description, the screenshots, the single-purpose statement, the permission
justifications and the privacy declarations are all typed into the developer
dashboard by a person. So they live here, and the dashboard is filled in from
here.

| file | what it is |
| --- | --- |
| [`listing.md`](listing.md) | every field of the listing, as text to paste: the store listing tab, the privacy tab, and what to say if the review pushes back |
| [`screenshots/`](screenshots) | the four 1280×800 screenshots the listing carries, in the order they appear |
| [`screenshots.mjs`](screenshots.mjs) | the script that produces them |
| [`../../docs/privacy.md`](../../docs/privacy.md) | the privacy policy the listing's **Privacy policy URL** field points at, served at `https://github.com/davison/md-notes/blob/main/docs/privacy.md` |

The extension package itself is not here: it is
`mdn-extension-<version>.zip` on each GitHub Release, built from the tag, and
`.github/workflows/publish-webstore.yml` uploads it when the release is
published. See [`docs/releasing.md`](../../docs/releasing.md).

## Filling in the dashboard

Once, by hand, when the item is created — and again for whichever fields change.
`listing.md` is in dashboard order; each fenced block is one field, to paste
whole.

The **Package** tab is the workflow's, not yours: uploading a zip there by hand
is how the item is first created, and after that every version arrives from a
release.

## What the operator holds, and what is here

The listing is filled in by hand from this directory. What cannot be here is
the account: the Chrome Web Store developer account, the OAuth client, and the
two identifiers the V2 API addresses the item by. Those are the operator's, and
`.github/workflows/publish-webstore.yml` reads them under exactly these names:

| name | kind | what |
| --- | --- | --- |
| `CHROME_WEBSTORE_CLIENT_ID` | secret | OAuth client id |
| `CHROME_WEBSTORE_CLIENT_SECRET` | secret | OAuth client secret |
| `CHROME_WEBSTORE_REFRESH_TOKEN` | secret | OAuth refresh token |
| `CHROME_WEBSTORE_PUBLISHER_ID` | variable | the publisher, from the dashboard's **Publisher > Settings** |
| `CHROME_WEBSTORE_ITEM_ID` | variable | the item, the last segment of its store URL — `cefjbjkkddbahpcfapmfechniahpdgaj`, set |

The two identifiers are variables rather than secrets: neither grants anything
without the three secrets, and a secret is masked out of the run log at exactly
the line that would say which item was uploaded to. davison/md-notes#135 carries
how each one is made, and which of them are in place: as of 2026-09-19 the item
exists and its id is set, and the publisher id and the three secrets are not
there yet, so the workflow's preflight will refuse by name until they are.

The listing is at
<https://chromewebstore.google.com/detail/cefjbjkkddbahpcfapmfechniahpdgaj>,
which the store redirects to a URL carrying a slug of the listing's name. The id
is the half that does not change; link that form.

## Regenerating the screenshots

```
make build extension
node extension/store/screenshots.mjs
```

It needs a `mdn` binary, a built `extension/dist`, and Playwright's Chromium —
the one `make ui-deps` installs the driver for, which
`pnpm --dir ui exec playwright install chromium` downloads. `PLAYWRIGHT_ROOT`
names another installation; `MDN_BIN` another binary.

It also needs **port 7337 free** and **no `/tmp/notes`**, and refuses rather
than working around either. Both are rendered into the shots — the daemon's
address into the popup and the options page, the notes root's path into the
app's header — so a run that quietly used something else would produce a
listing screenshot showing an address that contradicts the help text beside it,
which is how the first version of these got committed. If you have your own
daemon on 7337, give the run a network namespace of its own rather than
stopping it:

```
unshare --user --map-root-user --net -- \
  sh -c 'ip link set lo up; node extension/store/screenshots.mjs'
```

Every screenshot is the real thing: the script starts a daemon on a temporary
notes root, loads the built extension into Chromium through the same harness
`extension/e2e/` uses, clips a page it serves itself, opens the saved note in
the app, opens a local `.md` file through the file-URL intercept, and composes
each capture onto a captioned 1280×800 frame. The page it clips is served here
too, under the reserved name `theslowweb.example`, which Chromium is told to
resolve to it — so the address in the shot is a name rather than a port, and
impersonates nothing. Nothing is drawn by hand, so a screenshot that no longer
matches the product is one that fails to regenerate rather than one that quietly
misleads a stranger.

Consecutive runs produce the same screenshots, and almost always the same
bytes — nothing in them is a port, a temporary path or a random name any more.
Two things can still move them. The date: the daemon names a clip for the day it
was taken, and that name is in the app's sidebar in one of the shots. And a
handful of antialiased pixels: one run here came back four rows and seven pixels
different from the one before it, along a border, with the content identical.
So compare what changed before committing a regenerated shot, rather than
assuming a diff means the UI moved — and do not treat a byte-identical result
as something the script guarantees.

They are committed because the store needs the files and because a listing
nobody can rebuild is a listing that decays. Rerun the script when the UI moves,
and commit what comes out.
