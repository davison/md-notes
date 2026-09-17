# md-notes

A small local service that turns folders of markdown files into a notes
application in the browser, and doubles as a markdown viewer for any folder
on disk.

## Why

Notes should be plain markdown files in a folder hierarchy, editable with
any tool, synced by anything that syncs files. The application on top should
do a few things well and nothing else:

- Render notes properly, with a navigator pane that shows only markdown.
- Flip between the rendered view and a capable editor with one key.
- Save every edit automatically and reflect edits made elsewhere without a
  refresh.
- Find things by keyword, instantly.
- Accept clipped web pages and selections as markdown.
- Open any folder on disk, such as a code project, and browse its docs.

## Shape

The design as a whole. Milestone one built the daemon and the reading half of
the web UI, milestone two the editor, milestone three the browser extension
with the authentication it needed, milestone four made the result usable on
the devices it is read from and cheap on the wire, milestone five made
the app the only tool the notes need day to day — notes are created and
deleted in it, and a browser-level suite holds the result in CI — and
milestone six took up what five left behind: clipping works from another
tailnet node, the clipper converts the tables and code blocks it used to
mangle, the refusals say what is wrong, and the note bar and the
deleted-on-disk banner do what they promise. Milestone seven closed the way a
root could be registered by accident and left there, gave the navigator a
last-modified order beside its alphanumeric one, and made the UI an app a phone can
install from the tailnet name. The inbox is still ahead.

- **Daemon.** One static Go binary. Serves the web UI, watches one or more
  root folders, renders markdown server-side, shells out to ripgrep for
  search and tags, and pushes changes over Server-Sent Events. Binds to
  localhost only.
- **Web UI.** TypeScript. Navigator, rendered note, search panel, and a
  CodeMirror 6 editor with vim keybindings behind a single toggle. Three panes
  on a wide screen; below 960 pixels the note takes the whole viewport under a
  compact top bar and the two side panes become the tabs of one drawer. Notes
  are created from the top bar behind a name prompt and deleted from the note
  bar behind a confirmation. The browser tab names the note you have open. The
  bundle is embedded in the binary, served compressed and cached, and does not
  carry the editor to a page that is only reading. The tree is alphanumeric or
  most-recently-modified-first, at a toggle the browser remembers. A web app
  manifest and a service worker make it installable from Android over the
  tailnet, and the installed app opens with no daemon behind it and says so.
  A Playwright suite under
  `ui/e2e` drives the built daemon in a real browser as a CI job of its own,
  so the layout, the display settings, the tap targets, the asset cache, the
  create and delete flows, the navigator's two orders and removing a root are
  held by a check rather than by a
  measurement in a comment.
- **Browser extension.** Chromium Manifest V3. Clips a readable page or a
  selection as markdown and posts it to the daemon. Also intercepts local
  markdown file URLs so they open in the app. See
  [docs/extension.md](docs/extension.md).
- **Sync and mobile.** Out of scope for the daemon. Syncthing keeps the
  folder mirrored between machines and an Android phone, where any markdown
  editor reads the same files. An inbox file that turns URLs shared from the
  phone into proper clips when the folder next syncs to a machine running the
  daemon is planned, and is not built.
  [docs/sync.md](docs/sync.md) describes the whole
  arrangement: Syncthing setup, offline editing, conflict files in the
  navigator, Android with Markor, and the tailnet alternative.
- **E-ink.** The web UI has a light-theme override and a no-animation
  setting for a screen with no backlight, and tap targets that suit a
  stylus.
  [docs/e-ink.md](docs/e-ink.md) covers a Boox Note Air 3 and the two ways
  to reach your notes from one.

## Building

Requires Go and pnpm to build, and ripgrep (`rg`) on PATH at runtime: the
navigator, search and the tag panel all run through it, which is what keeps
gitignored and hidden files out of the tree and out of results.

```
make build      # builds the UI and the static ./mdn binary
make extension  # builds the browser extension to extension/dist and a zip
make check      # vet, typecheck, tests, build
make e2e        # browser checks for the UI, in headless Chromium
make install    # copies ./mdn to ~/.local/bin/mdn (PREFIX=... to change)
```

`make e2e` drives the built daemon through a real browser — the phone
layout, the drawer, the display settings, the tap targets, the asset cache,
and creating and deleting a note — and needs Chromium, which is a separate
download:

```
pnpm --dir ui exec playwright install chromium
```

Without it the suite skips rather than fails. It is not part of `make
check`, and it runs as its own CI job. The extension's own end-to-end
suites (`pnpm --dir extension e2e`) share the same Playwright installation
and additionally need `make extension`.

The commands below assume `~/.local/bin` is on your PATH; otherwise run
`./mdn` from the repository.

## Running

Create `~/.config/mdn/config.yml`:

```yaml
notes_root: /home/you/notes
port: 7337
max_watches: 8192
clips_dir: clips
```

Add `tailnet_host: laptop.tailnet-name.ts.net` to reach the daemon from
another node on your tailnet; see [Over the tailnet](#over-the-tailnet).

Then run the daemon and open the browser:

```
mdn serve
xdg-open http://localhost:7337/
```

`mdn serve --root DIR --port N` overrides the file. To run it under systemd
as a user service, see [contrib/mdn.service](contrib/mdn.service).

`mdn token` prints the daemon's bearer token — what a browser extension
presents to write a clipping — and `mdn token --rotate` replaces it, which
a running daemon picks up without a restart. The token file must be a
regular file, not a symlink; `--token-file FILE` moves it.

To browse the markdown in any other folder, such as a code project:

```
mdn open ~/projects/some-repo
```

That registers the folder with the running daemon, remembers it under
"Recent" on the home page, and opens the browser at it. `--no-browser`
prints the URL instead. The daemon must already be running.

A recent root is removed again from the home page — the **Remove** control
beside it, behind a confirmation naming the folder. Nothing leaves the disk,
and the configured notes root cannot be removed at all.

Search is a literal, case-insensitive phrase over the current root, run by
ripgrep, so gitignored and hidden files never match. Results show the
matching line with a line of context on either side, and selecting one
opens the note scrolled to the match. Tags come from a frontmatter list
(`tags: [a, b]`) or inline hashtags such as `#project/x`; the tag panel
lists them with counts and filters the navigator to the notes carrying one.

`Ctrl+E` flips the open note between the rendered view and a CodeMirror 6
editor with vim keybindings, and back. Edits save to the original file
automatically, one second after typing stops and immediately when you
switch view, move to another note, leave the window or type `:w` — the
note bar says whether the draft is saved, saving, failed or in conflict, and
the browser tab carries the same news as a leading `•` for unsaved work or
`⚠` for a conflict.
A refused save keeps the draft and offers a retry — a long reason is cut
with an ellipsis rather than widening the bar, and hovering it shows the
whole. A note changed or deleted on disk under an unsaved draft raises a
banner that keeps the draft until you say what to do with it; for a
deleted one, **Recreate the note** puts the file back with the draft in
it in a single confirmation.

Notes are created and deleted from the app as well as edited in it. **New
note**, in the top bar beside the home link, asks for a title or a path
before anything is written: a bare title becomes `<title>.md` in the folder
of the note you are reading, a name containing `/` is a path under the root,
and the new note opens in the editor. **Delete**, at the right-hand end of
the note bar, removes the open note only after a confirmation naming the
file; cancelling removes nothing. Both reach the navigator through the same
change stream as an edit made by any other tool. Renaming a note is still a
job for other tools.
[docs/introduction.md](docs/introduction.md#editing) has the detail.

Changes on disk show up in the browser without a refresh: the daemon watches
every registered root and streams change events to the page. The watched
directories are the ones holding a file ripgrep lists, the ones holding a
hidden file it lists — a `.gitkeep` placeholder, at any depth — their
ancestors, and any subtree below those holding no files at all. So gitignored
and hidden trees are not watched, and neither is a directory whose only files
are *ignored*: a note created there is seen when that directory next appears
in a change batch or when the daemon restarts.

In a folder with no ignore rules the watch set is much larger than the
navigator's tree — 5,814 inotify watches for `/usr/share`, whose notes live in
311 directories — so each root has a budget of `max_watches` directories, 8192
by default (`--max-watches N` on `mdn serve`, `0` for no budget). Watches go
first to the directories holding notes and to those where a first note could
appear, so a root over its budget loses the directories that hold files but no
note, and a directory that gains notes later takes a watch back from one of
them. When the budget is spent, or the kernel runs out of watches, the daemon
logs one line saying how much of the root is covered and the page shows a
notice above the navigator; the rest of the root stays live. To raise the
kernel's own limit (this needs root):

```
sudo sysctl fs.inotify.max_user_watches=524288
echo fs.inotify.max_user_watches=524288 | sudo tee /etc/sysctl.d/90-mdn.conf
```

The daemon listens on the loopback address only, refuses requests whose
Host or Origin is not its own, and never serves a path that resolves
outside a registered root, symlinks included. It holds one bearer token,
generated on first start and stored at `~/.local/state/mdn/token` with
mode `0600`; a request presenting it in an `Authorization` header is
accepted whatever its origin, which is how a browser extension writes to
the notes. Nothing else needs it, and the premise underneath is unchanged:
a single-user machine, where every local process already runs as the user
who owns the notes and can read that file anyway.

## Over the tailnet

`tailnet_host` in the configuration (or `--tailnet-host`) names one extra
`Host` the daemon answers to, so you can read your notes from another node
on your tailnet. The listener does not move: it still binds `127.0.0.1`,
and `tailscale serve` terminates TLS on the machine's own tailnet name and
proxies to it.

```yaml
tailnet_host: laptop.tailnet-name.ts.net
```

```
tailscale serve --bg 7337
```

Whatever terminates TLS in front must pass the original `Host` through
unchanged — that header is what tells the daemon a request came from the
tailnet and must prove itself. `tailscale serve` does; nginx's
`proxy_pass` does not unless you add `proxy_set_header Host $host;`. As a
backstop the daemon refuses a request that claims a loopback `Host` while
announcing a proxy — `Forwarded`, `Via`, `X-Forwarded-*`, `X-Real-IP` —
which catches most misconfigurations but not a bare `proxy_pass`, since
nginx alone sends none of those. The sentence above is the defence; the
backstop is not a substitute for it.

Everything under that name must authenticate. A browser is shown a login
page, pastes the token once and gets a session cookie scoped to that host
— `HttpOnly`, `Secure`, `SameSite=Strict` — and the whole UI, live update
included, works from there. An API client sends the `Authorization`
header. `mdn token --rotate` ends every session as well as every stored
token, which is how you revoke a device.

What a caller reaches over the tailnet is narrower than on loopback: the
UI's own API — the reads, creating, saving and deleting a note, the events
stream, search and tags — and `POST /api/clip` to a caller presenting the
token, but neither `POST /api/roots` nor `DELETE /api/roots/{slug}`.
Registering a folder is the step from
"read my notes" to "read any file on this machine", so it stays on the
machine, and so does unregistering one: changing the set of roots is a thing
that happens at the keyboard of the machine serving them. The clip does not: it writes one file into the notes root's clips
directory, at a name the daemon chooses, which is narrower than the note
save the tailnet already admits.

This is where the single-user premise stretches. On loopback the people
who can reach the daemon are the processes running as you. Under
`tailnet_host` they are whoever your **tailnet ACL admits to this node**,
and one of them holding the token can read and edit every root the daemon
serves — the notes root and every folder added with `mdn open`. Keep the
ACL as narrow as the notes deserve, and use `tailscale serve`, never
`tailscale funnel`, which would publish to the internet at large.
[docs/introduction.md](docs/introduction.md#reaching-the-daemon-over-the-tailnet)
has the details.

## Browser extension

`make extension` builds a Chromium Manifest V3 extension into
`extension/dist`, loadable unpacked in Brave from `brave://extensions` with
**Developer mode** on. Paste the token from `mdn token` into its options page
and it does two things.

**Clipping.** **Clip page** and **Clip selection**, from the toolbar button or
the page's right-click menu, turn the readable article — or just the selection
— into markdown and post it to the daemon. The popup shows the title, editable
before saving, and then a link that opens the new note in the app. A clip
lands under `clips/` in the notes root with `title`, `source`, `clipped` and
`tags: [clip]` above it, and appears in the navigator without a refresh. This
works with the daemon URL set to the `tailnet_host` name as well as to
loopback, so a browser on another tailnet node clips into the same notes;
registering a folder is the one action that stays on the daemon's machine.

**Opening local files.** Switch on **Allow access to file URLs** in its
details and a local `.md` or `.markdown` file opens in md-notes instead of
rendering as plain text: under the root that already contains it, or under its
own folder, which the extension registers as a new root — naming the file, so a
URL for a note that is not there registers nothing. The page is left
alone, with a red `!` on the toolbar icon saying why, when the daemon cannot be
reached, no token is stored, or the note the URL names does not exist.

[docs/extension.md](docs/extension.md) covers loading it, the token, clipping,
opening local files, and each permission it asks for and why.

## Status

Milestone one, the daemon and rendered viewer, is done: roots, confinement,
navigator, rendering, live update, search, and tags. Milestone two is done
too: the editor, autosave with explicit conflict handling, and the two
live-update and rendering follow-ups milestone one's QA left open. So is
milestone three: the bearer token and the clip endpoint, the extension that
clips a page or a selection and opens local markdown files, and reaching the
daemon from another node on the tailnet. And so is milestone four, which
worked through what three milestones of use had surfaced: the phone layout,
the browser tab title, code colours that are readable in the dark scheme, an
embedded bundle that is compressed, cached and no longer carries the editor
to a reader, the extension against a tailnet daemon URL, the display settings
an e-ink tablet needs, and [docs/sync.md](docs/sync.md), the account of how
the notes reach every device. Milestone five is done too: a note is created
and deleted from the app rather than from a shell or a file manager, with
`POST` and `DELETE` on the note's own source resource behind the same
confinement as the save; the `internal/watch` timing tests are driven by an
injected clock instead of the wall clock; and the browser-level checks that
lived in session scratchpads are a `ui/e2e` suite running in CI. Milestone
six took up the backlog that five raised: `POST /api/clip` joined the tailnet
allow-list, so the extension pointed at the `tailnet_host` URL clips into the
notes while registering a folder stays on the machine; the clipper writes a
headerless, spanning or nested table as markdown instead of leaving the
page's own HTML, keeps the caption and the filename line beside a code block
and drops only the chrome, and declines to squash a page laid out in a table
into one cell; a name the filesystem calls too long is answered `400
invalid_path` instead of `500`, and a dangling symlink chain out of the root
is answered `403 outside_root` on create and delete, followed as far as the
resolver itself follows one; and the note bar no longer scrolls sideways
under a long failure message, with the deleted-on-disk banner offering to
recreate the note from the draft in one step. It also took the two
test-suite sharp edges M5 left: the vitest teardown flake is gone at its
cause, and a browser check now holds the rule that a dialog's `Escape`
reaches nothing beneath it. Milestone seven took two asks of the operator's:
a `file:` URL for a markdown file that does not exist no longer registers its
directory as a permanent root — the daemon looks for the note before it
registers anything — and a root registered by accident is removed from the home
page rather than by editing the state file, both of them refused under the
tailnet name as registration always was; the navigator's tree can be ordered
by last modification, most recent first, at a toggle the browser remembers; and the
UI ships a web app manifest and a service worker, so Brave or Chrome on Android
offers to install it over the tailnet, the installed app opens in its own window,
and with the daemon unreachable it opens anyway and says so instead of showing the
browser's error page — [docs/sync.md](docs/sync.md#installing-it-on-the-phone) has
the steps.
[docs/introduction.md](docs/introduction.md) describes what the daemon does
today, [docs/extension.md](docs/extension.md) the extension,
[docs/e-ink.md](docs/e-ink.md) the e-ink tablet, and the milestone
records
([one](docs/milestones/1-daemon-and-rendered-viewer.md),
[two](docs/milestones/2-editor-autosave-and-live-update.md),
[three](docs/milestones/3-clipper-authentication-and-tailnet.md),
[four](docs/milestones/4-polish-phone-e-ink-and-the-bundle.md),
[five](docs/milestones/5-create-and-delete-notes.md),
[six](docs/milestones/6-tailnet-clipping-and-the-m5-backlog.md),
[seven](docs/milestones/7-roots-recency-and-the-installable-app.md))
record the decisions behind them. The inbox, which turns URLs shared from a
phone into clips, follows in a later milestone. Progress is tracked in
[ROADMAP.md](ROADMAP.md) and in the GitHub issues of this repository, which
is run as a [CodeCrew](https://github.com/radiusred/gh-codecrew) project.

## License

[MIT](LICENSE)
