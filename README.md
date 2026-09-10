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
the web UI, milestone two the editor; the extension and the inbox are still
ahead.

- **Daemon.** One static Go binary. Serves the web UI, watches one or more
  root folders, renders markdown server-side, shells out to ripgrep for
  search and tags, and pushes changes over Server-Sent Events. Binds to
  localhost only.
- **Web UI.** TypeScript. Navigator, rendered note, search panel, and a
  CodeMirror 6 editor with vim keybindings behind a single toggle.
- **Browser extension.** Chromium Manifest V3. Clips a readable page or a
  selection as markdown and posts it to the daemon. Also intercepts local
  markdown file URLs so they open in the app.
- **Sync and mobile.** Out of scope for the daemon. Syncthing keeps the
  folder mirrored between machines and an Android phone, where any markdown
  editor reads the same files. An inbox file lets URLs shared from the
  phone become proper clips when the folder next syncs to a machine running
  the daemon.

## Building

Requires Go and pnpm to build, and ripgrep (`rg`) on PATH at runtime: the
navigator, search and the tag panel all run through it, which is what keeps
gitignored and hidden files out of the tree and out of results.

```
make build      # builds the UI and the static ./mdn binary
make check      # vet, typecheck, tests, build
make install    # copies ./mdn to ~/.local/bin/mdn (PREFIX=... to change)
```

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

Then run the daemon and open the browser:

```
mdn serve
xdg-open http://localhost:7337/
```

`mdn serve --root DIR --port N` overrides the file. To run it under systemd
as a user service, see [contrib/mdn.service](contrib/mdn.service).

`mdn token` prints the daemon's bearer token — what a browser extension
presents to write a clipping — and `mdn token --rotate` replaces it, which
a running daemon picks up without a restart.

To browse the markdown in any other folder, such as a code project:

```
mdn open ~/projects/some-repo
```

That registers the folder with the running daemon, remembers it under
"Recent" on the home page, and opens the browser at it. `--no-browser`
prints the URL instead. The daemon must already be running.

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
note bar says whether the draft is saved, saving, failed or in conflict.
A refused save keeps the draft and offers a retry; a note changed or
deleted on disk under an unsaved draft raises a banner that keeps the
draft until you say what to do with it. Notes are edited in place:
creating, renaming and deleting them is still a job for other tools.
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

## Status

Milestone one, the daemon and rendered viewer, is done: roots, confinement,
navigator, rendering, live update, search, and tags. Milestone two is done
too: the editor, autosave with explicit conflict handling, and the two
live-update and rendering follow-ups milestone one's QA left open.
[docs/introduction.md](docs/introduction.md) describes what the daemon does
today, and the milestone records
([one](docs/milestones/1-daemon-and-rendered-viewer.md),
[two](docs/milestones/2-editor-autosave-and-live-update.md))
record the decisions behind it. The browser clipper and the inbox follow in
later milestones. Progress is tracked in
[ROADMAP.md](ROADMAP.md) and in the GitHub issues of this repository, which
is run as a [CodeCrew](https://github.com/radiusred/gh-codecrew) project.

## License

[MIT](LICENSE)
