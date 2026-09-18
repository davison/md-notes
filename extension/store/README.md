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

## Regenerating the screenshots

```
make build extension
node extension/store/screenshots.mjs
```

It needs a `mdn` binary, a built `extension/dist`, and Playwright's Chromium —
the one `make ui-deps` installs the driver for, which
`pnpm --dir ui exec playwright install chromium` downloads. `PLAYWRIGHT_ROOT`
names another installation; `MDN_BIN` another binary.

Every screenshot is the real thing: the script starts a daemon on a temporary
notes root, loads the built extension into Chromium through the same harness
`extension/e2e/` uses, clips a page it serves itself, opens the saved note in
the app, opens a local `.md` file through the file-URL intercept, and composes
each capture onto a captioned 1280×800 frame. Nothing is drawn by hand, so a
screenshot that no longer matches the product is one that fails to regenerate
rather than one that quietly misleads a stranger.

They are committed because the store needs the files and because a listing
nobody can rebuild is a listing that decays. Rerun the script when the UI moves,
and commit what comes out.
