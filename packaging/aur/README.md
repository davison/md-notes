# The AUR package

`md-notes-bin` on the [Arch User Repository][aur]. It installs the `mdn`
binary a release publishes, the systemd user unit and the licence, and
depends on ripgrep:

```
paru -S md-notes-bin      # or any other AUR helper
```

It is a `-bin` package because it installs prebuilt deliverables, and the
submission guidelines require the suffix for those. `md-notes` — no suffix —
is reserved by that decision for a package that builds from source, which
nothing here provides yet; the `-bin` package `provides` and `conflicts` with
that name so the two could never be installed together.

## What is here

| file | what it is |
| --- | --- |
| `PKGBUILD`, `.SRCINFO` | rendered by `scripts/aurgen`, committed for review |
| `MAINTAINER` | the content of the `# Maintainer:` line, one line |
| `md-notes-bin.install` | what pacman prints after installing |
| `LICENSE` | 0BSD, the licence of the *package sources* |
| `aur.gitignore` | pushed to the AUR as `.gitignore` |
| `verify.sh` | build, lint, install and remove it in a container |

The AUR repository holds five files: `PKGBUILD`, `.SRCINFO`,
`md-notes-bin.install`, `LICENSE` and `.gitignore`. Nothing else, ever —
the `.gitignore` excludes everything and the workflow force-adds those five,
so a built package or a `src/` directory cannot land there by accident. It is
called `aur.gitignore` in this repository because a file named `.gitignore`
here would ignore *this* directory in md-notes itself.

`LICENSE` is the [0BSD text Arch asks for][0bsd] and licenses the packaging,
not the software. The software's licence is MIT, which is what the PKGBUILD's
`license=('MIT')` field names and what lands in
`/usr/share/licenses/md-notes-bin/LICENSE`.

## The committed PKGBUILD is a template

`pkgver=0.0.0` and every checksum `SKIP`: there is no release it describes.
It is committed so the package is reviewable in a diff, and it is what

```
go run ./scripts/aurgen -placeholder
```

writes. A test holds the two together, so **edit the renderer, not these
files** — a hand edit here would look like a change to the package and change
nothing at all, because the workflow renders its own copy from the release.

For a real release:

```
gh release download v0.1.0 --pattern SHA256SUMS
go run ./scripts/aurgen -version v0.1.0 -sums SHA256SUMS
```

The two binaries' checksums come from the release's own `SHA256SUMS`; the
licence's and the unit's are hashed from this checkout, because the release
carries bare binaries and neither of those two files. That is also why they
are fetched from `raw.githubusercontent.com` at the tag rather than from the
release.

## Verifying it

`verify.sh` does everything the release workflow does except push, in a
container, so it can be run on any machine with podman or docker:

```
podman run --rm -v "$PWD:/work:ro,Z" -w /work archlinux:latest \
    packaging/aur/verify.sh v0.1.0
```

It builds with `makepkg -s` as a non-root user, diffs `.SRCINFO` against what
`makepkg --printsrcinfo` writes from the rendered `PKGBUILD`, runs `namcap`
over both the `PKGBUILD` and the built package and fails on any `E:` line,
installs the package, checks `mdn version` against the tag and that the unit
starts `/usr/bin/mdn`, then removes it and checks nothing survived. It
refuses to run outside a container, because installing and removing packages
on a machine somebody uses is not a test.

Three `namcap` warnings are expected and none is a defect:

- *lacks FULL RELRO* and *lacks PIE* — the release binary is static, built
  with `CGO_ENABLED=0`; a source package built to the Go package guidelines
  would be hardened, and that is the price of the `-bin` road.
- *Dependency included, but may not be needed ('ripgrep')* — the daemon runs
  `rg` as a subprocess, which namcap cannot see in an ELF file. The
  dependency is real; search and tags stop working without it.

## What a release does

`.github/workflows/publish-aur.yml`, on `release: published` and nothing
else. It renders, runs `verify.sh` in `archlinux:latest`, refuses to go on if
the PKGBUILD still carries `SKIP` or the placeholder maintainer, then clones
`ssh://aur@aur.archlinux.org/md-notes-bin.git`, copies the five files in,
commits as the operator and pushes to `master`. It pushes nothing when the
content has not changed.

The deploy key is the repository secret **`AUR_SSH_PRIVATE_KEY`**: the
private half of a key pair made for this and nothing else, whose public half
is on the AUR account. The AUR's host key is pinned in the workflow
(`SHA256:RFzBCUItH9LZS0cKB5UE6ceAYhBD5C8GeOBip8Z11+4`) rather than taken from
an `ssh-keyscan`.

The AUR records the author of every commit and it cannot be changed after the
push, so the workflow sets `user.name` and `user.email` on the clone
explicitly. Nothing needs to be created on the AUR by hand: the first push to
an empty `md-notes-bin` repository creates it.

[aur]: https://aur.archlinux.org/packages/md-notes-bin
[0bsd]: https://gitlab.archlinux.org/archlinux/devtools/-/blob/master/data/LICENSE
