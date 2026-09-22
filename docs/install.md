# Installing md-notes

The daemon ships on two channels, the Arch User Repository and a `.deb`, and the
browser extension is a zip on the same release page. The download commands below
use a `VERSION` variable, so they stay right from one release to the next. Set it
to the [latest release](https://github.com/davison/md-notes/releases/latest)'s
number, without the leading `v`, by asking GitHub for it:

```
VERSION=$(curl -fsSL https://api.github.com/repos/davison/md-notes/releases/latest \
  | sed -n 's/.*"tag_name": *"v\([^"]*\)".*/\1/p')
echo "$VERSION"
```

or by hand (`VERSION=0.1.0`) for a particular one from
[the releases page](https://github.com/davison/md-notes/releases). These are
bash and zsh commands; in fish, set it with `set VERSION 0.1.0` (or `set VERSION
(…)` around the same pipeline), and the commands below then work unchanged.

Whichever way you install it, the daemon needs **ripgrep** (`rg`) on `PATH` at
runtime. It builds the navigator's tree and runs search and tags, and it is what
keeps gitignored and hidden files out of all three. Both packages pull it in.

Once it is installed, [Running md-notes](running.md) takes you from the first
configuration file to a daemon that starts with your session.

## Arch, from the AUR

[`md-notes-bin`](https://aur.archlinux.org/packages/md-notes-bin) installs the
released binary, the systemd user unit and the licence, and depends on ripgrep.
From the release after v0.1.0 it installs the manual page as well
([#168](https://github.com/davison/md-notes/issues/168)).

```
paru -S md-notes-bin      # or any other AUR helper
```

It is a `-bin` package because it installs a prebuilt binary rather than
compiling one, and the AUR asks for the suffix when it does.
[packaging/aur/README.md](../packaging/aur/README.md) has the rest.

## Debian and Ubuntu, from the release page

Every release carries a `.deb` for amd64 and arm64:

```
curl -fsSLO https://github.com/davison/md-notes/releases/download/v$VERSION/md-notes_"$VERSION"_amd64.deb
sudo apt install ./md-notes_"$VERSION"_amd64.deb
```

The leading `./` is what tells apt the argument is a file rather than the name of
a package in a repository. It installs `/usr/bin/mdn`, the manual page, the unit
as `/usr/lib/systemd/user/mdn.service` and the licence, and pulls ripgrep in. No
apt repository is hosted, so an upgrade is those lines again, with `VERSION` set
to the later release. On arm64, put `arm64` where the file name says `amd64`.

## Starting it

Either package leaves the daemon to be started as your own user, not as root,
once `~/.config/mdn/config.yml` exists — [Running md-notes](running.md#the-configuration-file)
says what goes in it:

```
systemctl --user enable --now mdn
```

## The browser extension

There is no store listing. Download `mdn-extension-v$VERSION.zip`, unzip it into a
folder of its own — the zip has no top-level directory of its own, so unzipping
it where you stand scatters a dozen files — and load that folder unpacked:

```
curl -fsSLO https://github.com/davison/md-notes/releases/download/v$VERSION/mdn-extension-v$VERSION.zip
unzip -d mdn-extension-v$VERSION mdn-extension-v$VERSION.zip
```

Then `brave://extensions` (or `chrome://extensions`), **Developer mode** on,
**Load unpacked**, and choose that folder. Nothing updates it: a later release is
a later zip, loaded the same way.

Before you load it, [Privacy](privacy.md) says what it sends, where, and what it
keeps. [The browser extension](extension.md) covers the token, clipping, opening
local files, and every permission it asks for.

## Checking what you downloaded

`SHA256SUMS` on the release page covers the three files the release workflow
built — the two binaries and the extension zip:

```
curl -fsSLO https://github.com/davison/md-notes/releases/download/v$VERSION/SHA256SUMS
sha256sum --ignore-missing -c SHA256SUMS
```

The two `.deb`s are uploaded after the release is published, by a workflow of
their own, and are not in that file. [Checking a
release](releasing.md#checking-a-release) goes further, to what a release's bytes
prove about the commit they were built from.

## The bare binary

The daemon can also be taken bare: `mdn-v$VERSION-linux-amd64` and
`mdn-v$VERSION-linux-arm64` on the release page are the static binary, needing only
ripgrep on `PATH` and a configuration file. Put it on your `PATH` as `mdn`; the
unit, [contrib/mdn.service](../contrib/mdn.service), expects it at `/usr/bin/mdn`,
and its header says how to point it elsewhere.

## From source

Building it yourself — `make build`, then `sudo make install` — is the developer
route, and [CONTRIBUTING.md](../CONTRIBUTING.md#building) has it: what to install
first, what each target makes, and why not to `make install` over a packaged
`mdn`.
