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

- **Daemon.** One static Go binary. Serves the web UI, watches one or more
  root folders, renders markdown server-side, shells out to ripgrep for
  search and tags, and pushes changes over a WebSocket. Binds to localhost
  only.
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
navigator is built from its file listing, which is also what keeps
gitignored and hidden files out of the tree.

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
```

Then run the daemon and open the browser:

```
mdn serve
xdg-open http://localhost:7337/
```

`mdn serve --root DIR --port N` overrides the file. To run it under systemd
as a user service, see [contrib/mdn.service](contrib/mdn.service).

To browse the markdown in any other folder, such as a code project:

```
mdn open ~/projects/some-repo
```

That registers the folder with the running daemon, remembers it under
"Recent" on the home page, and opens the browser at it. `--no-browser`
prints the URL instead. The daemon must already be running.

Changes on disk show up in the browser without a refresh: the daemon watches
every registered root and streams change events to the page. Only
directories the navigator would show are watched, so ignored and hidden
trees cost nothing. A very large root can still exceed the kernel's
inotify watch limit; the daemon logs one line saying how many directories
it could not watch and serves the root without live update for those. If
that happens, raise the limit (this needs root):

```
sudo sysctl fs.inotify.max_user_watches=524288
echo fs.inotify.max_user_watches=524288 | sudo tee /etc/sysctl.d/90-mdn.conf
```

The daemon listens on the loopback address only, refuses requests whose
Host or Origin is not its own, and never serves a path that resolves
outside a registered root, symlinks included. It has no authentication of
its own: it assumes a single-user machine, where every local process
already runs as the user who owns the notes.

## Status

Milestone one in progress: the daemon skeleton, roots, confinement, the
navigator, rendering, and live update are in place; search and tags follow. Progress is tracked in
[ROADMAP.md](ROADMAP.md) and in the GitHub issues of this repository, which
is run as a [CodeCrew](https://github.com/radiusred/gh-codecrew) project.

## License

[MIT](LICENSE)
