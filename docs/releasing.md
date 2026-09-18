# Cutting a release

A release is one act: pushing a tag. Everything else follows from it — the
version the binaries report, the version the extension manifest carries, the
assets on the release page, the notes, and the channels that publish
afterwards.

```
git tag -a v0.1.0 -m "v0.1.0"
git push origin v0.1.0
```

That is the whole procedure. The rest of this page is what happens next, and
how to be sure of it before you push.

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
4. **Publishes the GitHub Release** with `gh release create --generate-notes`,
   carrying every file in `dist/`. The notes are GitHub's, generated from the
   commits and pull requests since the previous tag, which is why commit
   subjects are worth writing well.

The assets are:

| asset | what it is |
| --- | --- |
| `mdn-<version>-linux-amd64` | the static daemon, x86-64 |
| `mdn-<version>-linux-arm64` | the static daemon, aarch64 |
| `mdn-extension-<version>.zip` | the unpacked extension, zipped |
| `SHA256SUMS` | checksums of the three above |

The binaries are bare rather than tarred: the licence and
`contrib/mdn.service` are in the tagged source tree, which is what the
packaging workflows check out.

## What happens after the tag

Publishing the Release emits `release: published`, and each publishing channel
is its own workflow file on that event:

| channel | workflow |
| --- | --- |
| Chrome Web Store | `.github/workflows/publish-webstore.yml` |
| Arch User Repository | `.github/workflows/publish-aur.yml` |
| Debian package | `.github/workflows/publish-deb.yml` |

One file each, rather than one workflow with three jobs, so a channel that
fails — a store review, an expired deploy key — can be re-run on its own from
the Actions page against the same release, without cutting another tag.

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
checks, builds and uploads `dist/` as a run artifact, and creates no Release.

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

## Versions

`v<major>.<minor>.<patch>`, and the tag carries the leading `v`. The first
release is `0.1.0`. The workflow's trigger and its version check both require
that shape, so a mistyped tag fails before it builds rather than publishing
something the channels cannot read.
