# Cutting a release

A release is two acts. Pushing a tag:

```
git tag -a v0.1.0 -m "v0.1.0"
git push origin v0.1.0
```

and then, when the workflow has finished, opening the **draft** Release it left
on the releases page, reading the notes it generated, and pressing Publish.

Everything in between follows from the tag — the version the binaries report,
the version the extension manifest carries, the assets, the notes. Everything
after follows from the publish: that press is what starts the workflows that
put the release on the Chrome Web Store, the AUR and a `.deb`.

That is the whole procedure. The rest of this page is what happens at each
step, and how to be sure of it before you push.

## Where the version comes from

There is one version in this repository and it is the tag. Nothing carries a
version literal that has to be bumped by hand.

- The `Makefile` derives `VERSION` from `git describe --tags --always --dirty`,
  or takes it as `make release VERSION=v0.1.0`.
- The daemon gets it at link time (`-ldflags -X main.version`), so
  `mdn version` prints it.
- The extension manifest gets it at build time:
  `extension/scripts/stamp-manifest.mjs` writes it into
  `extension/dist/manifest.json` from `MDN_VERSION`.
  `extension/public/manifest.json`, the committed one, carries no `version` key
  at all, because a literal there would be a second source and the second
  source is the one nobody remembers to bump.
- The Chrome Web Store takes only one to four dot-separated integers, so the
  manifest carries the tag normalised: `v0.1.0` becomes `0.1.0`. Anything that
  is not a clean release tag — an untagged build, a `-dirty` tree, a
  `v0.1.0-3-gabc1234` describe string — becomes `0.0.0`, which loads and is
  obviously not a release.
- `scripts/relcheck` then refuses the release if the tag, the binary and the
  manifest do not all say the same thing. It runs inside `make release`, before
  the checksums are written, so a broken version chain fails the build rather
  than shipping a release that lies about what it is.

## What the tag runs

`.github/workflows/release.yml`, on any tag matching `v<major>.<minor>.<patch>`:

1. **Checks the commit** by calling `ci.yml` itself, so a release runs exactly
   the checks every other commit runs — `make check` and the browser suite —
   and cannot drift from them.
2. **Builds** with `make release VERSION=<tag>`: the static daemon for
   `linux/amd64` and `linux/arm64` with `CGO_ENABLED=0`, the extension zip, and
   `SHA256SUMS` over all three, into `dist/`.
3. **Checks the versions agree**, as above.
4. **Drafts the GitHub Release** with `gh release create --draft
   --generate-notes --verify-tag`, carrying every file in `dist/`. The notes
   are GitHub's, generated from the commits and pull requests since the
   previous tag, which is why commit subjects are worth writing well. The run
   summary links the draft.

Nothing is published at that point, and nothing downstream has started.

The assets are:

| asset | what it is |
| --- | --- |
| `mdn-<version>-linux-amd64` | the static daemon, x86-64 |
| `mdn-<version>-linux-arm64` | the static daemon, aarch64 |
| `mdn-extension-<version>.zip` | the unpacked extension, zipped |
| `SHA256SUMS` | checksums of the three above |

`<version>` in those names is the tag verbatim, leading `v` and all:
`mdn-v0.1.0-linux-amd64`, `mdn-extension-v0.1.0.zip`. Only the places that
cannot take a `v` get the stripped form — the extension manifest, and an AUR
`pkgver` or a Debian version. A workflow downloading an asset by URL wants the
`v`; one writing a package version does not.

The binaries are bare rather than tarred: the licence and
`contrib/mdn.service` are in the tagged source tree, which is what the
packaging workflows check out.

## Publishing the draft

Open the draft from the releases page, read the generated notes — this is the
one moment anyone reads them before they are public — edit them if they need
it, and press Publish.

It has to be a person. A Release published by the workflow's own
`GITHUB_TOKEN` starts no workflow run: GitHub's rule is that *"events triggered
by the `GITHUB_TOKEN` will not create a new workflow run"*, with only
`workflow_dispatch` and `repository_dispatch` excepted. So a workflow that
published the Release itself would leave every channel silently unstarted. A
person pressing Publish is not that token, and the event fires.

The assets are already attached to the draft and stay attached through the
publish: `gh release create` uploads assets to a draft and then publishes it
even for an ordinary published release, which is the same transition. Their
download URLs are
`https://github.com/davison/md-notes/releases/download/<tag>/<asset>` — built
from the tag and the asset name, neither of which the publish changes — so the
URLs a channel workflow consumes are the ones on the page.

## What happens after the publish

Publishing emits `release: published`, and each publishing channel is its own
workflow file on that event:

| channel | workflow |
| --- | --- |
| Chrome Web Store | `.github/workflows/publish-webstore.yml` |
| Arch User Repository | `.github/workflows/publish-aur.yml` |
| Debian package | `.github/workflows/publish-deb.yml` |

One file each, rather than one workflow with three jobs, so a channel that
fails — a store review, an expired deploy key — can be re-run on its own from
the Actions page against the same release, without cutting another tag or
republishing anything.

## Proving it before you push

A tag is public and awkward to take back, so neither the build nor the workflow
has to be tried for the first time on a real one.

**The build, locally.** `make release VERSION=v0.1.0` does everything the
workflow's build step does, on your machine, and leaves `dist/` to look at:

```
make release VERSION=v0.1.0
./dist/mdn-v0.1.0-linux-amd64 version   # v0.1.0
cat dist/SHA256SUMS
```

The version check runs a binary it has just built, so `make release` wants a
host that can run one of its own targets — an `amd64` or `arm64` Linux machine,
which is also what CI is.

**The workflow, as a dry run.** The release workflow takes a
`workflow_dispatch` with a version input. Dispatched, it is a dry run: it
checks, builds and uploads `dist/` as a run artifact, and creates no Release —
not even a draft. Only a tag push drafts one.

```
gh workflow run release.yml -f version=v0.1.0
gh run watch
```

`--ref <branch>` runs it against a branch, but only once the workflow file is
on `main`: GitHub answers `HTTP 404: workflow release.yml not found on the
default branch` for a `workflow_dispatch` it cannot see there, whatever ref you
ask for. So the first dry run of a change to this workflow happens after it
merges and before the tag is pushed, and that is the point of the dry run
existing.

## When a release has to be re-run

`gh release create` has no update mode — no `--clobber`, nothing — and a tag
that already has a *published* Release is refused. What it does when a *draft*
already stands under that tag is not established here: a draft holds no tag
ref, so it may well leave a second draft rather than refuse. Either way the
move is the same, and it is the reason this is written as an instruction rather
than a prediction: delete the draft first, from the releases page or with
`gh release delete <tag>`, and only then re-run from the Actions page. Skip
that and the worst case is two drafts under one name to choose between.

Which mistake you are recovering from decides how far back you can go.

**The draft is still a draft.** Nothing is public and nothing downstream has
started, so this is free: delete the draft, and if the commit itself was wrong,
delete the tag and push it again at the right one. A draft Release, its assets
and its tag can all be deleted — the immutability that pins a release's tag and
assets applies only once it is published.

**It is already published.** Then the tag stays. It names the commit the assets
were built from, and someone may already have fetched it; deleting and
re-pushing it would build a different tree under a name that is already out
there. Delete the Release only, re-run, and publish the new draft. If a channel
workflow is what failed, do not touch the Release at all — re-run that channel's
workflow from the Actions page, which is why each has a file of its own.

## To settle at the first release

Two things on this page are read from `gh`'s documentation rather than measured,
because measuring them means creating a real Release. The first release is the
moment to look, and to correct this page in the same breath:

- **A re-run with a draft standing.** Does `gh release create` refuse, or does a
  second draft appear under the same tag? The instruction above is safe under
  either, but the sentence should say which.
- **A draft's asset URLs before publication.** The published form is
  `https://github.com/davison/md-notes/releases/download/<tag>/<asset>`, built
  from the tag and the asset name; what a draft serves in the meantime, and
  whether anything but the page's own links changes at the publish, was not
  observed. The channel workflows read those URLs.

## Versions

`v<major>.<minor>.<patch>`, and the tag carries the leading `v`. The first
release is `0.1.0`. The workflow's trigger and its version check both require
that shape, so a mistyped tag fails before it builds rather than publishing
something the channels cannot read.
