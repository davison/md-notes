# Introduction

md-notes is a local service that turns folders of markdown files into a notes
application in the browser. This page describes what exists and works today, at the
end of [milestone one](milestones/1-daemon-and-rendered-viewer.md). The editor, the
browser clipper and the inbox are not built yet.

## The daemon

One static Go binary, `mdn`, with the web UI compiled into it. It listens on the
loopback address only and serves every root it knows about.

```
mdn serve                run the daemon against the configured notes root
mdn open DIR             register DIR with the running daemon and open it
mdn version              print the version
```

`mdn serve` takes `--config FILE` (default `~/.config/mdn/config.yml`),
`--root DIR` and `--port N` to override what the file says, and `--state FILE`
(default `~/.local/state/mdn/roots.json`) for where folders added with `mdn open`
are remembered. It stops cleanly on `SIGINT` and `SIGTERM`.

`mdn open DIR` takes `--config FILE`, `--port N`, and `--no-browser` to print the URL
instead of launching one. It resolves `DIR` against your working directory, POSTs it
to the running daemon and opens `/r/<slug>/`. If no daemon answers on the port it
prints how to start one and exits non-zero — it never starts a daemon itself.

The configuration file holds two keys:

```yaml
notes_root: /home/you/notes
port: 7337
```

A missing file is not an error as long as `--root` supplies the notes root. The
default port is 7337. `contrib/mdn.service` is a systemd user unit that runs
`mdn serve`.

ripgrep (`rg`) must be on `PATH` at runtime. It builds the navigator's file listing,
runs search, and decides which files the tag collector reads — which is how
gitignored and hidden files stay out of all three.

## Roots

A root is a folder the daemon serves, identified in URLs by a slug derived from its
basename and deduplicated with a numeric suffix. There are two kinds:

- The **notes root**, from `notes_root` or `--root`. It is permanent and always
  present.
- **Recent roots**, added by `mdn open`. They persist to the state file with their
  slugs, so a root's URL survives a restart. A recent root whose directory has since
  disappeared is dropped when the daemon next starts.

Registering a path that is already a root returns the existing root rather than
duplicating it; the comparison is on the real path, so a symlinked alias resolves to
the same root. A directory nested inside an existing root can still be registered as
a root of its own.

The home page at `/` lists the notes root under "Notes" and every recent root under
"Recent", each linking to its three-pane view.

## The HTTP API

All endpoints are on the loopback listener, and all of them are behind the guard
described under [Confinement](#confinement).

| Endpoint | What it does |
|----------|--------------|
| `GET /api/roots` | Every root as `{slug, path, kind}` |
| `POST /api/roots` | Registers `{"path": "/absolute/dir"}` and returns the root. Relative paths are refused |
| `GET /api/r/{slug}/tree` | The root's markdown tree as nested `{name, path, dir, children}` |
| `GET /api/r/{slug}/note/{path...}` | A rendered note as `{path, title, frontmatter, html}`. Non-markdown paths are 404 here |
| `GET /api/r/{slug}/raw/{path...}` | File bytes, for images and other assets. Served with `Content-Security-Policy: sandbox` and `X-Content-Type-Options: nosniff` |
| `GET /api/r/{slug}/search?q=` | `{hits, truncated}`; each hit is a path, line number, matching text with match offsets, and the lines either side |
| `GET /api/r/{slug}/tags` | `[{name, count, notes}]`, sorted by count then name |
| `GET /api/r/{slug}/events` | A Server-Sent Events stream of change batches |

Everything else serves the embedded UI bundle, falling back to `index.html` so
client-side routes such as `/r/notes/some/note.md` load.

The events stream opens with a `: connected` comment, sends `event: change` frames
whose data is `{"paths": [...]}`, and sends a keepalive comment every thirty seconds.
An empty batch means "refetch everything": it is what the daemon sends when a change
batch was lost, including on a kernel event queue overflow. `EventSource` reconnects
on its own, and the UI treats a reconnect as a full refresh too.

## The web UI

A Preact application, three panes per root:

- **Navigator.** The markdown tree, with collapsible directories whose expansion is
  remembered per root in `localStorage`. The current note is highlighted and its
  ancestors are opened. Only markdown files and the directories containing them
  appear; `.md` and `.markdown` count, hidden entries do not, and everything ripgrep
  would ignore is absent.
- **Note.** The rendered note: title, a collapsed metadata panel holding the
  frontmatter, and the body. Notes render server-side as GitHub-flavoured markdown —
  tables, task lists, strikethrough, autolinks, footnotes, and fenced code
  highlighted by chroma — then pass through a sanitiser, so a note cannot run script
  on the app's origin. Relative links to markdown become in-app navigation; relative
  images and other assets are served from the raw endpoint; a link whose target
  escapes the root keeps its text but loses its destination and says why.
- **Search and tags.** A debounced search box whose results group by file, showing
  the matching line with the match emphasised and a line of context either side.
  Selecting a hit opens the note with `?l=<line>` and scrolls to the block at that
  line, flashing it. Below it, the tag panel lists each tag with its count; selecting
  one sets `?tag=` and prunes the navigator to the notes carrying it, with a link to
  clear the filter.

Search is a literal, case-insensitive phrase — what you type is what is matched.
Tags come from a frontmatter `tags` value (a list, or one string split on commas and
whitespace) and from inline hashtags: `#` at the start of a line or after whitespace,
followed by letters, digits, `_`, `-` or `/`, containing at least one letter, outside
fenced and inline code, lower-cased. Tags are collected per request, with no index.

## Live update

The daemon watches every registered root with fsnotify and pushes change batches to
every open page for that root, so creating, modifying, deleting or renaming a file
shows up without a refresh. Events are debounced into one batch per root — 150 ms
of quiet, or one second at the outside. The page
refetches the tree on any batch that could change it, and refetches the open note
when the batch names its path or a directory above it.

The watched directory set is every directory holding a file ripgrep lists, plus their
ancestors, plus any subtree beneath those that contains no files at all. A directory
whose only files are hidden or ignored is therefore not watched: a note created in
one is seen when the directory next appears in a batch, or when the daemon restarts.
The trade-off and the two ways it bites are recorded in the milestone document and
tracked in [#13](https://github.com/davison/md-notes/issues/13).

Running out of inotify watches is logged with a count and does not stop the daemon;
the root is served without live update for the directories it could not watch.

## Confinement

- The listener binds `127.0.0.1` and nothing else.
- The `Host` header must be `localhost` or `127.0.0.1` with the daemon's port, which
  defeats DNS rebinding.
- An `Origin` header, if present, must be the daemon's own origin. A request with no
  `Origin`, such as the CLI, passes.
- Every path a request names is resolved through one function: it is cleaned and
  rejected if it leaves the root lexically, then symlinks are evaluated and it is
  rejected again if the real path leaves the root. A symlink pointing back inside the
  root is served.

The daemon has no authentication of its own. It assumes a single-user machine, where
every local process already runs as the user who owns the notes — so any local
process can list roots, register folders, and read files under them through the API,
and files the navigator and search hide are still readable by direct URL. That
premise was raised and accepted deliberately
([#2](https://github.com/davison/md-notes/issues/2#issuecomment-5572874194)); a token
is expected to arrive with the browser extension, which needs one anyway.
