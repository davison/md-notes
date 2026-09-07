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

## Status

Design stage. Nothing runnable yet. Progress is tracked in
[ROADMAP.md](ROADMAP.md) and in the GitHub issues of this repository, which
is run as a [CodeCrew](https://github.com/radiusred/gh-codecrew) project.

## License

[MIT](LICENSE)
