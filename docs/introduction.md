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
`--root DIR`, `--port N` and `--max-watches N` to override what the file says, and
`--state FILE` (default `~/.local/state/mdn/roots.json`) for where folders added with
`mdn open` are remembered. It stops cleanly on `SIGINT` and `SIGTERM`.

`mdn open DIR` takes `--config FILE`, `--port N`, and `--no-browser` to print the URL
instead of launching one. It resolves `DIR` against your working directory, POSTs it
to the running daemon and opens `/r/<slug>/`. If no daemon answers on the port it
prints how to start one and exits non-zero — it never starts a daemon itself.

The configuration file holds three keys:

```yaml
notes_root: /home/you/notes
port: 7337
max_watches: 8192
```

A missing file is not an error as long as `--root` supplies the notes root. The
default port is 7337 and the default watch budget 8192 directories per root; `0`
removes the budget, and a negative value is refused. The Live update section below
says what the budget buys. `contrib/mdn.service` is a systemd user unit that runs
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
| `GET /api/roots` | `{roots: [{slug, path, kind}]}` |
| `POST /api/roots` | Registers `{"path": "/absolute/dir"}` and returns the root. Relative paths are refused |
| `GET /api/r/{slug}/tree` | The root's markdown tree as nested `{name, path, dir, children}` |
| `GET /api/r/{slug}/note/{path...}` | A rendered note as `{path, title, frontmatter, html}`. Non-markdown paths are 404 here |
| `GET /api/r/{slug}/source/{path...}` | Existing UTF-8 markdown as `{source, revision}`; see [conditional saves](#conditional-saves) |
| `PUT /api/r/{slug}/source/{path...}` | Conditionally saves JSON `{source, revision}` and returns the saved `{source, revision}` |
| `GET /api/r/{slug}/raw/{path...}` | File bytes, for images and other assets. Served with `Content-Security-Policy: sandbox` and `X-Content-Type-Options: nosniff` |
| `GET /api/r/{slug}/search?q=` | `{hits, truncated}`; each hit is a path, line number, matching text with match offsets, and the lines either side |
| `GET /api/r/{slug}/tags` | `{tags: [{name, count, notes}]}`, sorted by count then name |
| `GET /api/r/{slug}/events` | A Server-Sent Events stream of change batches |

Everything else serves the embedded UI bundle, falling back to `index.html` so
client-side routes such as `/r/notes/some/note.md` load.

### Conditional saves

Read the source endpoint before editing, retain its opaque `revision`, then PUT the
complete new `source` with that revision and `Content-Type: application/json`.
An empty source string is valid; omitted or null source is not. A successful save
returns HTTP 200 and the new revision to use for the next save. Source responses
use `Cache-Control: no-store`. The existing rendered and raw endpoints are unchanged.

The API edits existing `.md` and `.markdown` regular files only, in any registered
root. It preserves frontmatter, whitespace, line endings, Unicode and trailing
newlines exactly as submitted; it never parses or rewrites the frontmatter.
Editable source must be valid UTF-8 and at most 8 MiB. Invalid UTF-8 and unpaired
JSON Unicode surrogate escapes are rejected. The encoded JSON request limit allows
six bytes per source byte plus 1 KiB for the revision and object syntax.

Source errors return JSON `{code, error}` with these statuses:

| Status | Code | Client action |
|--------|------|---------------|
| 400 | `invalid_body` | Correct malformed JSON or missing/invalid fields |
| 403 | `outside_root`, `permission_denied` | Retain the draft; check the path or file/directory permissions |
| 404 | `not_found`, `not_markdown` | Retain the draft; the note/root is missing or the path is not markdown |
| 409 | `conflict` | Retain the draft and fetch current source before choosing how to reconcile |
| 413 | `too_large` | Source or request exceeds the size limit |
| 415 | `invalid_body` | Send `Content-Type: application/json` |
| 422 | `unsupported_source` | The file is nonregular or contains invalid UTF-8 |
| 428 | `revision_required` | Read the source first and include its revision |
| 500 | `io_error` | Retain the draft and retry after checking disk/storage health |

The Host/Origin guard can also return 403 with the existing `{error}` body before
the source handler runs. A failed save never supplies a replacement revision or
instructs the client to discard its draft. A lost HTTP response can leave a save's
outcome unknown; fetch the source and compare it with the retained draft before
retrying. There is no force-save or create-on-missing option.

Revisions track file identity, content, modification time and permissions. They
remain stable while the observed file is unchanged, including across reads through
symlink aliases and overlapping registered roots. They are local to one daemon
session; a restart makes an old token conflict, so clients must re-read. The daemon
keeps one small revision record per canonical path accessed during that session.

Source reads and saves are serialized within the daemon. Saves check the revision,
write and sync a temporary file in the same directory, check the current file again,
then replace it by rename. Stale browser saves, detected external edits/replacements,
and deleted notes are rejected. **An uncoordinated external editor can still write
between the final revision check and replacement.** Ordinary filesystem rename is
not atomic compare-and-swap; external tools do not participate in the daemon lock.

Internal symlink aliases update their resolved target without replacing the alias;
symlinks outside the registered root are refused. Open directory handles confine
staging and replacement despite symlink changes. Read-only files are refused even
when the directory permits replacement. Replacement preserves the file's nine Unix
permission bits, but creates a new inode owned by the daemon user: hard-link identity,
ownership, ACLs, extended attributes and other extended metadata are not preserved.
The temporary file is synced before rename; the parent directory is not synced, so
the API does not promise rename durability across power loss.

### Live updates

The events stream opens with a `: connected` comment, sends `event: change` frames
whose data is `{"paths": [...]}`, and sends a keepalive comment every thirty seconds.
An empty batch means "refetch everything": it is what the daemon sends when a change
batch was lost, including on a kernel event queue overflow. `EventSource` reconnects
on its own, and the UI treats a reconnect as a full refresh too.

After the `: connected` comment the stream sends one `event: status` frame carrying
the root's watch coverage, and sends it again when the coverage has changed — on the
next keepalive tick, so up to thirty seconds later:

```json
{"watched": 8192, "unwatched": 4498, "budget": 8192,
 "overBudget": true, "failed": 0, "limited": true}
```

`limited` is `unwatched > 0`; `overBudget` says the budget rather than an error is
the reason; `failed` counts directories the kernel refused; `budget` is the per-root
maximum and is `0` when there is none, never negative. A root with no watcher at all
has no stream: the endpoint answers 503, which an `EventSource` reports by closing
for good rather than reconnecting.

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
  on the app's origin, nor dress itself in the application's own CSS. The classes a
  note may carry are a whitelist: any name in the reserved `mdn-` namespace, which
  only the syntax highlighter emits and no application stylesheet uses; the
  `language-` class goldmark gives a fence it could not tokenise; and by exact name
  the few structural classes a note's own footnotes and links carry. Every other
  class is stripped, so what a note can style does not depend on what the
  application calls its own classes. Relative links to markdown become in-app
  navigation; relative images and other assets are served from the raw endpoint; a
  link whose target escapes the root keeps its text but loses its destination and
  says why.
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

### Which directories are watched

The watched set has three parts, all of them found from one ripgrep listing:

- every directory holding a file ripgrep lists, and their ancestors;
- every directory holding a *placeholder* — a hidden file ripgrep lists, such as the
  `.gitkeep` that keeps an otherwise empty directory in a git repository — and their
  ancestors, at any depth;
- every subtree beneath those that holds no files at all.

The first note created in any of them is seen live, which is what a placeholder
directory exists for.

That listing is `rg --files --hidden --glob '!.*/'`: hidden files asked for, hidden
directories refused. Asking for hidden files does not disable ripgrep's ignore
rules, and that is the whole point — a `.gitkeep` no rule covers is named, while the
`.lock` files of a gitignored cache are not, so an ignored tree of dotfiles cannot
enter the set by looking empty. Refusing hidden directories costs nothing, because
they are never watched, and keeps the walk out of `.git` and `.cache`, which is
where the time would go: computing the watched set costs 6–9% more than the rule
this replaced, which asked ripgrep for no hidden files at all.

Ignored files are the limit of it. A directory whose only files are *ignored* holds
nothing ripgrep will name, hidden or not, so it is not watched, and a note created
there is seen when the directory next appears in a batch, or when the daemon
restarts. That is the remaining hole in live update's coverage of an ordinary root,
and it is the trade-off recorded in the
[milestone one document](milestones/1-daemon-and-rendered-viewer.md).

### The watch budget

The set grows with the tree, not with the notes in it, because a directory holding
files but no note can gain one later and is watched too. On a notes folder that
costs nothing; on a large ad-hoc root it is the dominant cost. Measured on one
machine with the budget removed, counting each directory by the group the listing
puts it in — the three groups below:

| root | watched | 1: holds a note, plus ancestors | 2: nothing to list | 3: files but no note |
| --- | --- | --- | --- | --- |
| a notes folder | 21 | 3 | 2 | 16 |
| `~/projects` | 1,614 | 642 | 30 | 942 |
| `/usr/share` | 5,814 | 311 | 706 | 4,797 |
| `/usr/lib` | 12,700 | 303 | 33 | 12,364 |
| `~/.cache` | 82,874 | 13,616 | 559 | 68,699 |

Group two is small because a directory that holds no file of its own but has
file-holding descendants is an ancestor, and ancestors go in the group of what they
lead to. QA measured `/usr/share` at 5,790 against 310 navigator directories before
this task ([#13](https://github.com/davison/md-notes/issues/13)); watching
hidden-only directories accounts for the difference.

An inotify watch costs about a kilobyte of
unswappable kernel memory and comes from
a per-user pool — `fs.inotify.max_user_watches`, commonly 524,288 — shared with
every editor, IDE and file manager the user is running. `~/.cache` alone would take
16% of that pool, some 83 MB, for one registered root.

So each root has a budget of `max_watches` directories, 8192 by default: enough to
cover `/usr/share` whole, a sixty-fourth of a typical desktop's pool, and equal to
the smallest limit still shipped, so one ad-hoc root cannot exhaust an unraised
system on its own. Watches are placed in priority order, so a root larger than its
budget keeps the ones worth most:

1. the root, every directory holding a markdown file, and their ancestors — the set
   the navigator lists, where notes change;
2. directories holding nothing the navigator would list, where a first note can
   appear that nothing else would report;
3. everything else — directories holding files but no note.

The third group is 83–97% of the watch set on every large root above — 58% on
`~/projects`, which is mostly notes — so it is what a spent budget gives up first.

The order holds after startup too. When a directory appears — or gains its first
note — on a root whose budget is already spent, the least valuable watch is released
to make room for it: a directory the listing no longer names, then the lowest group,
then the last path within that group. Directories of equal rank never displace each
other, so a settled root does not churn its watches, and the root's own watch is
never given up.

An unwatched directory hides nothing permanently: its changes still arrive when a
watched directory reports them, when the tree is refetched for another reason, and
when the daemon restarts.

### When coverage is limited

A spent budget and a kernel out of watches mean the same thing to a reader — part of
the root is not live — so they are one report:

- the daemon logs one line per root, whatever the number of directories behind it,
  naming the root, how many directories are covered, how many are not, why, and
  which limit to raise;
- the root's event stream opens with a `status` event carrying the same numbers, and
  sends it again on the next keepalive tick after they change;
- the page shows a notice above the navigator whenever coverage is limited, naming
  whichever limits are in play, so it is visible in the browser and not only in the
  daemon's log.

A root whose watcher never started is the same story with nothing covered: its event
stream answers 503, and the page says live update is not available for that root and
that changes show up on reload.

None of this stops the daemon, and none of it disables live update for the rest of a
root. To cover a large root completely, raise `max_watches` (or set it to `0` for no
budget) and raise the kernel's own limit to match:

```
sudo sysctl fs.inotify.max_user_watches=524288
echo fs.inotify.max_user_watches=524288 | sudo tee /etc/sysctl.d/90-mdn.conf
```

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
- The two halves of that meet at symlinks inside a root: ripgrep does not follow them,
  so a symlinked file or directory is in neither the navigator nor search, but one
  whose target is inside the root is still served by direct URL, and one whose target
  leaves the root is refused.

The daemon has no authentication of its own. It assumes a single-user machine, where
every local process already runs as the user who owns the notes — so any local
process can list roots, register folders, and read files under them through the API,
and files the navigator and search hide are still readable by direct URL. That
premise was raised and accepted deliberately
([#2](https://github.com/davison/md-notes/issues/2#issuecomment-5572874194)); a token
is expected to arrive with the browser extension, which needs one anyway.
