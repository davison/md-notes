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
put the release on the AUR and add the `.deb`s.

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
- The rule for an extension manifest's `version` is Chromium's, not a store's:
  *"Required value 'version' is missing or invalid. It must be between 1-4
  dot-separated integers each between 0 and 65536."* So the manifest carries the
  tag normalised: `v0.1.0` becomes `0.1.0`. Anything that is not a clean release
  tag — an untagged build, a `-dirty` tree, a `v0.1.0-3-gabc1234` describe
  string — becomes `0.0.0`, which loads and is obviously not a release.
- `scripts/relcheck` then refuses the release if the tag, the binary and the
  manifest do not all say the same thing. It runs inside `make release`, before
  the checksums are written, so a broken version chain fails the build rather
  than shipping a release that lies about what it is.

## What the tag runs

`.github/workflows/release.yml`, on any tag matching `v<major>.<minor>.<patch>`:

1. **Checks the commit** by calling `ci.yml` itself, so a release runs exactly
   the checks every other commit runs — `make check` and the browser suite —
   and cannot drift from them.
2. **Builds** with `make release VERSION=<tag>`, using the Go and the Node the
   tag pins (see [Checking a release](#checking-a-release)): the static daemon
   for `linux/amd64` and `linux/arm64` with `CGO_ENABLED=0`, the extension
   zip, and `SHA256SUMS` over all three, into `dist/`.
3. **Checks the versions agree**, as above.
4. **Drafts the GitHub Release** with `gh release create --draft
   --generate-notes --verify-tag`, carrying every file in `dist/`. The notes
   are GitHub's, generated from the commits and pull requests since the
   previous tag, which is why commit subjects are worth writing well. The run
   summary links the draft.

Nothing is published at that point, and nothing downstream has started.

A published release carries six assets. Four are the release workflow's, and
are on the draft before anyone sees it; the two `.deb`s are added afterwards by
`publish-deb.yml`, which runs on the publish:

| asset | what it is | put there by |
| --- | --- | --- |
| `mdn-<version>-linux-amd64` | the static daemon, x86-64 | `release.yml` |
| `mdn-<version>-linux-arm64` | the static daemon, aarch64 | `release.yml` |
| `mdn-extension-<version>.zip` | the unpacked extension, zipped | `release.yml` |
| `SHA256SUMS` | checksums of the three above | `release.yml` |
| `md-notes_<version without the v>_amd64.deb` | the Debian package, x86-64 | `publish-deb.yml` |
| `md-notes_<version without the v>_arm64.deb` | the Debian package, arm64 | `publish-deb.yml` |

`SHA256SUMS` covers the three files built beside it and nothing else: the
`.deb`s do not exist when it is written, and re-writing it afterwards would mean
a second version of a file people may already have fetched. Each `.deb` carries
the byte-identical binary from the release it was built for — `publish-deb.yml`
checks the downloaded binaries against `SHA256SUMS` before nfpm wraps them — so
the chain of custody runs through that file either way, and the packages'
own digests are printed in the `publish-deb` run summary.

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
| Arch User Repository | `.github/workflows/publish-aur.yml` |
| Debian package | `.github/workflows/publish-deb.yml` |

One file each, rather than one workflow with two jobs, so a channel that fails —
an expired deploy key, a container image that has moved — can be re-run on its
own from the Actions page against the same release, without cutting another tag
or republishing anything.

There were to have been three. The Chrome Web Store channel was withdrawn before
the first release — the reasoning is on
[#135](https://github.com/davison/md-notes/issues/135#issuecomment-5744118645) —
and the extension is distributed as the release's own
`mdn-extension-<version>.zip` instead, loaded unpacked. Nothing publishes it
anywhere else.

`publish-deb.yml` puts its two packages on the release with
`gh release upload --clobber`, so a re-run replaces what the previous attempt
uploaded rather than failing on a name that is already there. `--clobber`
deletes the existing asset *before* it uploads the new one, so an upload that
fails half way leaves neither: a re-run of the channel is the recovery, and
there is nothing else to undo.

## Checking a release

A release can be rebuilt from its tag to the same bytes. For v0.1.0 this was
measured, not assumed: two builds in one runner job, a third in another job, a
fourth on Ubuntu 26.04, and a local rebuild on Arch all reproduced the three
published SHA-256 sums exactly
([#193](https://github.com/davison/md-notes/issues/193#issuecomment-5773496854)).
It holds on one condition, which is **the toolchain**. The tag names it:

- **Go** is the `toolchain` line in `go.mod`, for example `toolchain go1.27.1`.
  It has to be upstream's build of that version. Arch's `go1.27.1-X:nodwarf5`
  is patched and makes different binaries at the same version number.
  `GOTOOLCHAIN=go1.27.1` fetches upstream's from the Go proxy, whatever Go you
  have installed. A release built before the pin (v0.1.0) records its Go in the
  binary: `go version -m mdn-v0.1.0-linux-amd64` prints `go1.27.1` on its first
  line.
- **Node** is `.node-version`. It has to be a nodejs.org build of that version.
  The daemon embeds `ui/dist`, which includes gzip copies of every asset, and
  the extension zip is deflated too. Both come from Node's own zlib, so a Node
  linked against a distribution's zlib (Arch's is one) changes the zip *and*
  both binaries while every `.js` file stays identical. v0.1.0 predates the
  pin and was built with Node 24.20.0.

pnpm's version doesn't matter: the lockfiles pin everything it installs, and
10.33 and 10.34 measured the same.

To check a release, clone it, check out its tag, and run the check below from
the checkout. It stops, naming what is missing, rather than building with
whatever Go and Node happen to be installed, because a build with the wrong
toolchain differs and would look like a release that doesn't reproduce.

```
git clone https://github.com/davison/md-notes && cd md-notes
git checkout v0.2.0                        # the tag you are checking

TAG=v0.2.0 NODE_DIR=$HOME/node bash -eu <<'CHECK'
go=${GO:-$(sed -n 's/^toolchain //p' go.mod)}
: "${go:?go.mod names no toolchain line: set GO (see below for v0.1.0)}"
node=${NODE_VERSION:-$(cat .node-version 2>/dev/null || true)}
: "${node:?there is no .node-version: set NODE_VERSION (see below for v0.1.0)}"
bin=$NODE_DIR/node-v$node-linux-x64/bin
if [ ! -x "$bin/node" ]; then
  echo "no Node $node in $bin: unpack https://nodejs.org/dist/v$node/ there" >&2
  exit 1
fi
export GOTOOLCHAIN=$go PATH=$bin:$PATH
make release VERSION="$TAG"
gh release download "$TAG" --pattern SHA256SUMS --output dist/published.sha256
diff dist/SHA256SUMS dist/published.sha256 && echo "$TAG reproduced"
CHECK
```

`NODE_DIR` is wherever you unpacked the nodejs.org tarball
(`node-v<version>-linux-x64`, or `-arm64`, adjusting `bin`).

**v0.1.0 predates both pins.** It was built with go1.27.1 and Node 24.20.0, so
check it with `TAG=v0.1.0 GO=go1.27.1 NODE_VERSION=24.20.0 NODE_DIR=… bash
-eu <<'CHECK'` and the same script. Node 24.21.0 reproduces it too. Without
`GO` and `NODE_VERSION`, the script stops at the first of the two lookups.

Rebuild in a clean checkout, and keep the output in `dist/`, which is
gitignored. The binaries carry a `vcs.modified` flag, so any untracked file
that isn't ignored (a `DIST=` of your own, say) flips it and changes both
binaries. They also carry a module version that Go derives from the tags it can
see. At a tag, that is the tag itself. At an untagged commit it is a
pseudo-version, which comes out differently in a full clone and in a shallow
one. So compare a dry run's artifact against a clone made the way CI makes
one: `git clone --depth 1 --branch <branch>`. `make release` wants an `amd64`
or `arm64` Linux host because it runs one of the binaries it builds; the runner
image makes no difference.

**If you can't rebuild**, check the assets against `SHA256SUMS` instead, which
is what every channel does. The packages carry the release's binaries
unchanged: the `/usr/bin/mdn` inside the `.deb` and the AUR package is the
release asset byte for byte, because `publish-deb.yml` checks the downloaded
binaries against `SHA256SUMS` before packaging them and the PKGBUILD's
`sha256sums_*` are copied out of the same file. So `sha256sum /usr/bin/mdn`
on an installed system should print the matching line of the release's
`SHA256SUMS`.

**Moving the pins** is a one-line change each, reviewed like any other. Bump
the `toolchain` line when a Go point release comes out. CI will prompt you:
`make vuln` scans the pinned standard library, so a Go security release turns
it red until the line moves. Bump `.node-version` when there's a reason to;
Node builds the bundle but doesn't ship in it. The runner image is pinned the
same way (`ubuntu-24.04` in every workflow; the reasons are at the top of
`ci.yml`'s jobs), and `scripts/workflows/pins_test.go` holds all three.

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
already stands under that tag is still not established: the first release did
not need a re-run, so nothing has exercised it (see
[What the first release showed](#what-the-first-release-showed)). A draft holds
no tag ref, so it may well leave a second draft rather than refuse. Either way the
move is the same, and it is the reason this is written as an instruction rather
than a prediction: delete the draft first, from the releases page or with
`gh release delete <tag>`, and only then re-run from the Actions page. Skip
that and the worst case is two drafts under one name to choose between.

Which mistake you are recovering from decides how far back you can go.

**The draft is still a draft.** Nothing is public and nothing downstream has
started, so this is free: delete the draft, and if the commit itself was wrong,
delete the tag and push it again at the right one. A draft Release, its assets
and its tag can all be deleted — the immutability that pins a release's tag and
assets applies only once it is published. A draft holds no tag ref of its own,
which the first release showed plainly: the run summary recorded the draft it
had created as
`https://github.com/davison/md-notes/releases/tag/untagged-ce73351a50e41ea92653`,
a generated name rather than `v0.1.0`, which is the address it answered to until
the operator pressed Publish.

**It is already published.** Then the tag stays. It names the commit the assets
were built from, and someone may already have fetched it; deleting and
re-pushing it would build a different tree under a name that is already out
there. Delete the Release only, re-run, and publish the new draft. If a channel
workflow is what failed, do not touch the Release at all — re-run that channel's
workflow from the Actions page, which is why each has a file of its own.

## What the first release showed

[v0.1.0](https://github.com/davison/md-notes/releases/tag/v0.1.0) was tagged on
2026-09-19. The tag's run
([35460122576](https://github.com/davison/md-notes/actions/runs/35460122576))
drafted the Release; the operator read the notes and published it at 18:11:59Z.
What that settled, and what it did not:

**Settled.**

- **The two acts work as written.** The publish fired `release: published`, and
  both channel workflows started two seconds later —
  [publish-aur](https://github.com/davison/md-notes/actions/runs/35460432264)
  and
  [publish-deb](https://github.com/davison/md-notes/actions/runs/35460432286).
  A person pressing Publish is what the design needs, and it is what happened.
- **The assets survive the publish, unchanged.** The four the workflow attached
  were created at 18:08:44Z, three minutes before the operator published, and
  the digests the release API reports for them are the ones `SHA256SUMS`
  attests to and the ones a download checks out against — measured by QA after
  the fact
  ([#133](https://github.com/davison/md-notes/issues/133#issuecomment-5744403052),
  observation 5). The two `.deb`s joined them from the channel run.
- **The published URL form is the one written above.** `makepkg` inside
  publish-aur's container fetched
  `https://github.com/davison/md-notes/releases/download/v0.1.0/mdn-v0.1.0-linux-amd64`
  from the PKGBUILD's `source_x86_64` and matched it against the checksum
  `SHA256SUMS` carried, so the form is measured rather than inferred.
- **A channel re-runs on its own, and `--clobber` behaves.** `publish-deb`
  succeeded on the publish and was then re-run against the same published
  release, deliberately rather than in recovery; the second attempt succeeded
  too, replacing both packages in place at 18:14:25Z while the four
  release-workflow assets went untouched. Afterwards the release API's digests
  for the two `.deb`s equal the SHA-256 of the files downloaded from the page,
  and both install. That is the re-run M8-R1 asks for, and the caveat above is
  now written from a measurement rather than from the manual.
- **The generated notes need a previous tag to be short.** With none, GitHub
  generated notes listing every pull request in the repository's history. That
  is one-off: the next release's notes span one tag to the next.

**Still not settled.** Both of these need a mistake, or a deliberate rehearsal,
that the first release did not supply:

- **A re-run with a draft standing.** `gh release create` ran once. Whether it
  refuses or leaves a second draft under the same tag is still unmeasured, and
  the instruction above is still written to be safe under either.
- **A draft's asset URLs before publication.** The draft stood for about three
  minutes and nobody fetched an asset from it, so what a draft serves is still
  unobserved — though the *page* address is now known, since the run summary
  recorded it as `untagged-ce73351a50e41ea92653` rather than the tag. Nothing
  depends on it: no channel workflow
  runs before the publish.

## Versions

`v<major>.<minor>.<patch>`, and the tag carries the leading `v`. The first
release is `0.1.0`. The workflow's trigger and its version check both require
that shape, so a mistyped tag fails before it builds rather than publishing
something the channels cannot read.
