# Running md-notes

From an installed `mdn` to your notes in a browser: the configuration file, the
first start, the systemd unit, the token, other folders and several roots. The
commands assume `mdn` is on your `PATH`; from a checkout, run `./mdn` instead.
[Installing md-notes](install.md) comes first, and
[the introduction](introduction.md) is the reference for everything this page
touches.

## The configuration file

Create `~/.config/mdn/config.yml`:

```yaml
notes_root: /home/you/notes
port: 7337
max_watches: 8192
clips_dir: clips
```

`notes_root` is the folder of notes, and the only key you need; the others are
shown at their defaults. `clips_dir` is where the browser extension's clips
land, relative to the notes root. `max_watches` is the live-update budget, under
[Large folders](#large-folders) below. A fifth key, `tailnet_host`, is for
[reaching it from another device](#from-another-device).

## Starting it

```
mdn serve
xdg-open http://localhost:7337/
```

The daemon listens on the loopback address only. Flags override the file for
one run: `mdn serve --root DIR --port N`, and `--config FILE` for another
file. [The daemon](introduction.md#the-daemon) lists every flag.

To have it start with your session, use the systemd user unit,
[contrib/mdn.service](../contrib/mdn.service). The AUR package and the `.deb`
install it for you, and so does `sudo make install`; either way you finish as
yourself, not as root:

```
systemctl --user daemon-reload
systemctl --user enable --now mdn
```

`journalctl --user -u mdn` shows what it logged: every root it serves, with its
kind and path, at each start.

## The token

The daemon holds one bearer token, generated on first start and stored at
`~/.local/state/mdn/token` with mode `0600`. It is what the browser extension
presents to write a clip, and what a browser on another tailnet node logs in
with. Nothing on the machine itself needs it: the app at `localhost` works
without one.

```
mdn token            # print it
mdn token --rotate   # replace it; a running daemon picks the new one up
```

Rotating it also ends every session a browser logged in with, which is how you
revoke a device. The token file must be a regular file, not a symlink, and
`--token-file FILE` moves it, on `mdn serve` and `mdn token` alike.

## Other folders

To browse the markdown in any other folder, such as a code project:

```
mdn open ~/projects/some-repo
```

That registers the folder with the running daemon, remembers it under "Recent"
on the home page, and opens the browser at it. `--no-browser` prints the URL
instead. The daemon must already be running; `mdn open` never starts one.

A recent root is removed from the home page, with the **Remove** control beside
it, behind a confirmation naming the folder. Nothing leaves the disk: the folder
and every note in it stay as they are, and `mdn open` brings it back.

## Several roots

A folder you want served at every start, rather than remembered from an
`mdn open`, is a **permanent root**. There are two ways to name one.

**In the file**, `notes_root` takes a list. The first entry is the notes root,
where clips land; every entry after it is a permanent root, in the order given:

```yaml
notes_root:
  - /home/you/notes      # the notes root
  - /home/you/projects   # a permanent root
```

**On the command line**, `--root` repeats, and every one is served, the first as
the notes root:

```
mdn serve --root ~/notes --root ~/projects
```

Any `--root` **replaces** the file's `notes_root` completely rather than adding
to it, as `--port` replaces `port`, so the command line alone says what is
served. The daemon logs the roots it set aside when it does this.

Permanent roots are listed under "Notes" on the home page, beside the notes
root, and like it they cannot be removed from the app: they are your
configuration, and the next start would put them back. Every root must be an
existing directory, and two roots naming the same folder, through a symlink or
not, stop the daemon at startup with an error naming both.
[Roots](introduction.md#roots) has the rest: slugs, how a recent root that is
now configured is served, and roots inside roots.

## Large folders

Changes on disk reach the browser without a refresh: the daemon watches every
root and streams change events to the page. A folder of notes costs a handful of
inotify watches. A large folder opened with `mdn open` can cost thousands —
`/usr/share` takes nearly 6,000 — so each root has a budget of `max_watches`
directories, 8192 by default, `0` for none. The watches go first to the
directories holding notes, so a root over its budget stays live where the notes
are.

When the budget or the kernel's own limit runs out, the daemon logs one line
saying how much of the root is covered, and the page shows a notice above the
navigator; the rest of the root stays live. To raise the kernel's limit (this
needs root):

```
sudo sysctl fs.inotify.max_user_watches=524288
echo fs.inotify.max_user_watches=524288 | sudo tee /etc/sysctl.d/90-mdn.conf
```

[Live update](introduction.md#live-update) says exactly which directories are
watched, and why.

## From another device

The daemon never listens beyond loopback, but it can be reached from another
node on your tailnet through `tailscale serve`, which terminates TLS on the
machine's tailnet name and proxies to the loopback port. Name that host in the
file,

```yaml
tailnet_host: laptop.tailnet-name.ts.net
```

and run `tailscale serve --bg 7337`. Everything under that name must
authenticate: a browser pastes the token once and gets a session cookie. Use
`tailscale serve`, never `tailscale funnel`, which would publish your notes to
the internet at large. Whoever your tailnet ACL admits to this machine can, with
the token, read and edit every root the daemon serves.

[Reaching the daemon over the tailnet](introduction.md#reaching-the-daemon-over-the-tailnet)
is the full account, and the one to read before you do it: what the proxy must
pass through, what is reachable under that name and what stays on the machine.
[Sync and offline editing](sync.md) covers the other way to reach your notes from
a phone, with Syncthing and no daemon on the phone at all.

## What it trusts

The daemon listens on the loopback address only, refuses a request whose `Host`
or `Origin` is not its own, and never serves a path that resolves outside a
registered root, symlinks included. A request carrying the token in an
`Authorization` header is accepted whatever its origin, which is how the
extension writes to the notes. The premise underneath is a single-user machine,
where every local process already runs as the user who owns the notes and could
read the token file anyway. [Confinement](introduction.md#confinement) and
[Authentication](introduction.md#authentication) have the detail.

## Using it

The app itself is in [The web UI](introduction.md#the-web-ui) and
[Editing](introduction.md#editing): the navigator, search and tags, `Ctrl+E` to
flip a note to the editor and back, autosave, creating and deleting notes, and
what happens when a note changes on disk under an unsaved draft. On a phone, see
[On a phone](introduction.md#on-a-phone) and
[Installing the app](introduction.md#installing-the-app); on an e-ink tablet,
[On an e-ink tablet](e-ink.md).
