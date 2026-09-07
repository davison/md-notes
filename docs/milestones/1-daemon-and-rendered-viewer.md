# M1 — Daemon and rendered viewer over multiple roots

Tracking issue: [#1](https://github.com/davison/md-notes/issues/1). Its five
implementation tasks are merged on `main` at
[`70b9f4e`](https://github.com/davison/md-notes/commit/70b9f4e).

## Goal and outcome

The milestone's goal, as stated on [#1](https://github.com/davison/md-notes/issues/1),
was a local daemon serving a rendered, navigable, searchable view of the markdown
files in one or more folders on disk, updating live as files change — the reading
half of the notes app, and the whole of the MarkdownReader replacement. Editing,
clipping, and the browser extension were deferred to later milestones by the goal
itself.

What shipped is a single static Go binary, `mdn`, with a Preact UI embedded in it.
`mdn serve` runs a loopback-only daemon over a configured notes root; `mdn open DIR`
registers any other folder with the running daemon and opens the browser at it. For
each root the daemon serves a navigator built from ripgrep's file listing, notes
rendered server-side as sanitised GitHub-flavoured markdown, a literal search through
ripgrep with scroll-to-match, a tag panel that filters the navigator, and a
Server-Sent Events stream that pushes change batches to the open page. Every path a
request names is resolved inside a registered root or refused.

The system as it stands is described in [the introduction](../introduction.md).

Five implementation tasks delivered it, each through its own PR and review loop:

| Task | Requirements | PR |
|------|--------------|----|
| [#2](https://github.com/davison/md-notes/issues/2) Project skeleton: mdn binary, roots, confinement | M1-R1, M1-R2, M1-R8 | [#3](https://github.com/davison/md-notes/pull/3) |
| [#4](https://github.com/davison/md-notes/issues/4) Navigator: markdown-only tree from ripgrep | M1-R3 | [#6](https://github.com/davison/md-notes/pull/6) |
| [#5](https://github.com/davison/md-notes/issues/5) Rendering: GFM notes with frontmatter, link rewriting, and assets | M1-R4 | [#7](https://github.com/davison/md-notes/pull/7) |
| [#8](https://github.com/davison/md-notes/issues/8) Live update: watch roots and push changes to the browser | M1-R5 | [#9](https://github.com/davison/md-notes/pull/9) |
| [#11](https://github.com/davison/md-notes/issues/11) Search and tags | M1-R6, M1-R7 | [#12](https://github.com/davison/md-notes/pull/12) |

## Requirement outcomes

Every verdict below is drawn from the QA comment on the milestone issue
([#1](https://github.com/davison/md-notes/issues/1#issuecomment-5575193559)), which
was run against the binary `make build` produced from merged `main`, with the daemon
on a temporary config, state file and root and the UI driven headlessly in Chromium.

| ID | Requirement | Status |
|----|-------------|--------|
| M1-R1 | Single binary, `mdn serve`, configurable port, systemd unit example | [Satisfied](https://github.com/davison/md-notes/issues/1#issuecomment-5575193559) |
| M1-R2 | Permanent notes root, `mdn open DIR`, home page listing roots | [Satisfied](https://github.com/davison/md-notes/issues/1#issuecomment-5575193559) |
| M1-R3 | Navigator: markdown only, gitignore honoured, from ripgrep | [Satisfied](https://github.com/davison/md-notes/issues/1#issuecomment-5575193559) |
| M1-R4 | GFM rendering, frontmatter as metadata, link and asset rewriting | [Satisfied](https://github.com/davison/md-notes/issues/1#issuecomment-5575193559) — one low finding filed ([#14](https://github.com/davison/md-notes/issues/14)) |
| M1-R5 | Live update within about a second, no manual refresh | [Satisfied](https://github.com/davison/md-notes/issues/1#issuecomment-5575193559) — two low findings filed ([#13](https://github.com/davison/md-notes/issues/13), and the README correction made here) |
| M1-R6 | Search through ripgrep, with context, opening the note at the match | [Satisfied](https://github.com/davison/md-notes/issues/1#issuecomment-5575193559) |
| M1-R7 | Tags from frontmatter or hashtags, counted, filtering the navigator | [Satisfied](https://github.com/davison/md-notes/issues/1#issuecomment-5575193559) |
| M1-R8 | Loopback bind, path confinement, Origin rejection | [Satisfied](https://github.com/davison/md-notes/issues/1#issuecomment-5575193559), as scoped by the gate resolved on [#2](https://github.com/davison/md-notes/issues/2#issuecomment-5572874194) |

The three low findings were dispositioned by the operator, none of them blocking the
milestone ([#1](https://github.com/davison/md-notes/issues/1#issuecomment-5575933501));
see [Known gaps at the boundary](#known-gaps-at-the-boundary).

## Decisions

### Go, goldmark, fsnotify — and the name `mdn`

Recorded on [#1](https://github.com/davison/md-notes/issues/1#issuecomment-5572364167).

- **Trade-off:** a second language alongside the TypeScript UI, and Go's markdown
  ecosystem is smaller than Node's.
- **Rejected:** Bun or Node for the daemon. A static Go binary cross-compiles to ARM
  with no runtime, which keeps a later Termux or NAS deployment possible without
  extra work.

### Preact with Vite, embedded in the binary at build time

Recorded on [#1](https://github.com/davison/md-notes/issues/1#issuecomment-5572364167).

- **Trade-off:** less ecosystem than React, and contributors may be less familiar
  with it.
- **Rejected:** React, Svelte, or a larger framework. The UI is three panes with no
  routing complexity, so the smallest thing that gives components and state is
  enough.

QA confirmed the embedding is real rather than incidental: a binary copied out of
the worktree, with `ui/dist` emptied by `make clean` and the working directory set
elsewhere, still served `index.html` and both hashed asset files
([#1](https://github.com/davison/md-notes/issues/1#issuecomment-5575193559)).

### ripgrep instead of an index

Search, tag collection and the navigator listing all shell out to ripgrep rather
than maintaining an index. Recorded on
[#1](https://github.com/davison/md-notes/issues/1#issuecomment-5572364167).

- **Trade-off:** ripgrep becomes a runtime dependency, and every query re-scans the
  root.
- **Rejected:** an embedded index or database. The notes corpus is small, ripgrep
  over it is instant, and it honours gitignore for free, which matters for the
  code-project use case.

This decision propagated further than it first appears. It is why the navigator, the
search and the watch set all agree about what a root contains, and it is why two of
the milestone's sharper review findings existed at all: the navigator's include globs
overrode file-level gitignore rules
([#6, finding 1](https://github.com/davison/md-notes/pull/6#issuecomment-5573706952)),
and search's markdown type filter re-included hidden files
([#12, finding 1](https://github.com/davison/md-notes/pull/12#issuecomment-5574796558)).
Both were fixed by moving the extension filtering into Go and leaving ripgrep's own
rules untouched.

### No separate testing requirement

Recorded on [#1](https://github.com/davison/md-notes/issues/1#issuecomment-5572364167):
tests ride each PR per the implementer contract rather than forming a requirement of
their own. No trade-off or rejected alternative was recorded for this one.

### Symlinks that leave a root are rejected, for every root

The operator's answer to the ask-the-human point on the skeleton plan
([#2](https://github.com/davison/md-notes/issues/2#issuecomment-5572526403)).

- **Trade-off:** a symlinked notes directory would need the policy revisited. The
  operator's notes folder uses no symlinks.
- **Rejected:** a per-root exemption or a config switch, as there is no current need.

A symlink pointing back inside the root is served; QA confirmed both halves
([#1](https://github.com/davison/md-notes/issues/1#issuecomment-5575193559)).

### `mdn open` does not start the daemon

Also from [#2](https://github.com/davison/md-notes/issues/2#issuecomment-5572526403).
When the daemon is unreachable the CLI prints how to start it and exits non-zero.

- **Trade-off:** one more manual step on a machine without the systemd unit
  installed.
- **Rejected:** auto-starting a background daemon from `open`, which would leave an
  unmanaged process outside systemd.

### The single-user premise (gate raised and resolved)

The review of the skeleton PR observed that the daemon has no authentication of its
own — it binds to loopback, checks `Host` and `Origin`, and confines paths, but any
local process or local user can list roots, register folders, and read every file
under them through the API — and that this premise was stated nowhere in the
milestone or the task. It was raised as a decision gate rather than decided by the
implementer ([#2](https://github.com/davison/md-notes/issues/2#issuecomment-5572740350)).

Resolved by the operator
([#2](https://github.com/davison/md-notes/issues/2#issuecomment-5572874194)):
the premise is accepted for this milestone, the state file is kept private to the
user (`0600`), and the premise is stated in the README.

- **Trade-off:** another local user on the same machine could read the served folders
  through the API.
- **Rejected:** a per-boot token presented by the UI and CLI. Deferred until the
  browser extension needs an authentication path, at which point the same mechanism
  can serve both.

This is the scope within which M1-R8's verdict should be read. QA recorded the same
boundary explicitly: files the navigator and search hide — hidden and gitignored
files — remain readable by direct URL because they lie inside a registered root
([#1](https://github.com/davison/md-notes/issues/1#issuecomment-5575193559)). The
backlog capture for tailnet access ([#10](https://github.com/davison/md-notes/issues/10))
records that exposing the daemon beyond the machine stretches this premise to
everyone an ACL admits, and so should land with or after an authentication path.

### `.md` and `.markdown` count as markdown; hidden entries do not

Recorded on [#4](https://github.com/davison/md-notes/issues/4#issuecomment-5573615193).

- **Trade-off (extensions):** none of note; `.markdown` is rare but costs one extra
  glob. **Rejected:** `.md` only, or a configurable list, as nothing needs it yet.
- **Trade-off (hidden):** a note deliberately named with a leading dot is invisible.
  **Rejected:** passing `--hidden`, which would surface `.git`, `.obsidian`, and
  similar tool folders in every root.

QA confirmed `.markdown` is listed, `.mdown` is not, and upper-case `READ.MD` is
listed ([#1](https://github.com/davison/md-notes/issues/1#issuecomment-5575193559)).
The upper-case behaviour arrived through review finding 4 on
[#6](https://github.com/davison/md-notes/pull/6#issuecomment-5573706952), not through
the original decision.

### Rendered HTML is sanitised

The operator's answer to the rendering plan's ask-the-human point
([#5](https://github.com/davison/md-notes/issues/5#issuecomment-5573615027)):
bluemonday's user-generated-content policy, extended for task-list checkboxes,
heading IDs, footnotes and code classes.

- **Trade-off:** raw HTML in notes and clipped pages loses scripts, inline handlers,
  iframes, and embedded media.
- **Rejected:** rendering raw HTML as written, because clipped third-party pages
  would then run script on the app's own origin.

This is the decision that most of the milestone's security posture rests on, and it
is the one most heavily probed. The reviewer failed to get anything past the
sanitiser in about sixty attempts
([#7](https://github.com/davison/md-notes/pull/7#issuecomment-5573996511)), and QA
failed again over about fifty vectors against the merged binary
([#5](https://github.com/davison/md-notes/issues/5#issuecomment-5575184182)).

### A heading that supplies the title is removed from the body

Recorded on [#5](https://github.com/davison/md-notes/issues/5#issuecomment-5574025766),
in answer to the reviewer's finding that the title rendered twice in the common case.

- **Trade-off:** a note whose first H1 is not really its title loses that heading from
  the body; a frontmatter `title` keeps the H1 in place.
- **Rejected:** hiding the app's own title when it came from the H1, which would make
  the title bar inconsistent between notes with and without frontmatter.

The first implementation of this removed headings nested inside blockquotes and list
items — content loss, caught in re-review as N2 on
[#7](https://github.com/davison/md-notes/pull/7#issuecomment-5574074357) and narrowed
so that only a top-level H1 can be the title.

### A link that escapes the root is disarmed

Recorded on [#5](https://github.com/davison/md-notes/issues/5#issuecomment-5574025766).
Such a link keeps its text, gains a title saying why, and has no destination.

- **Trade-off:** the original relative path is no longer visible in the link.
- **Rejected:** leaving it relative, which the browser resolves against the app's
  route and lands on a dead page with a 200; and pointing it at the raw endpoint,
  which cannot express a path above the root.

### Server-Sent Events, not a WebSocket

Recorded on [#8](https://github.com/davison/md-notes/issues/8#issuecomment-5574155865).

- **Trade-off:** SSE is one-directional and text-only; anything the UI must send goes
  through ordinary requests, which is how the app already works.
- **Rejected:** a WebSocket, which would add a third-party dependency — Go's standard
  library has none — for bidirectional messaging nothing in this milestone needs.

The decision explicitly supersedes the milestone's opening wording, which had said
the daemon pushes changes over a WebSocket. The README carried that stale wording
until QA caught it
([#8](https://github.com/davison/md-notes/issues/8#issuecomment-5575181315)); it is
corrected in the same PR as this document.

### A lost batch becomes an empty batch

Recorded on [#8](https://github.com/davison/md-notes/issues/8#issuecomment-5574412848).
A batch lost in the watcher or the hub is delivered to the consumer as an empty
batch, which the UI treats as "refetch everything"; the watcher retries delivery on
its debounce interval.

- **Trade-off:** one full refetch after a stall rather than a precise list.
- **Rejected:** silently dropping, which left a stalled tab out of date until the next
  unrelated change.

Kernel-side loss was folded into the same mechanism later, as one of the deferred
observations that rode the search task
([#8](https://github.com/davison/md-notes/issues/8#issuecomment-5574635558)). QA
forced a genuine `fsnotify` queue overflow — 240,000 file creations in 2.08 s across
300 watched directories — and observed the empty-batch marker arrive as designed
([#8](https://github.com/davison/md-notes/issues/8#issuecomment-5575181315)).

### Six SSE connections per origin is accepted

Recorded on [#8](https://github.com/davison/md-notes/issues/8#issuecomment-5574412848).
Browsers cap plaintext HTTP/1.1 connections at six per origin and each open tab holds
one stream, so more than about six tabs would starve.

- **Trade-off:** a user with many tabs sees live update stall in the extra ones.
- **Rejected:** a WebSocket (same per-connection cost, plus a dependency) and a
  SharedWorker multiplexing one stream across tabs — the right fix if it ever
  matters, and one that does not change the server.

### Search is a literal, case-insensitive phrase

Recorded on [#11](https://github.com/davison/md-notes/issues/11#issuecomment-5574646763).

- **Trade-off:** no regex, and no "all these words anywhere in the file".
- **Rejected:** regex by default, which makes `.` and `(` surprise a keyword search;
  and multi-word AND, which needs per-file post-filtering and is easy to add later.

### Hashtag and frontmatter tag rules

Recorded on [#11](https://github.com/davison/md-notes/issues/11#issuecomment-5574646763).
An inline tag is `#` at the start of a line or after whitespace, followed by letters,
digits, `_`, `-` or `/`, containing at least one letter, outside fenced and inline
code, lower-cased. Frontmatter `tags` may be a list or a single string split on
commas and whitespace.

- **Trade-off:** `#123` is never a tag, and a tag inside a code span is ignored.
- **Rejected:** any `#word` anywhere, which turns headings, issue numbers and URL
  fragments into tags.

Review and re-review narrowed the rule twice more, in the same spirit: markdown link
destinations and bare URLs are blanked before matching
([#12, finding 5](https://github.com/davison/md-notes/pull/12#issuecomment-5574796558)),
and reference link definitions are skipped
([#12, R5](https://github.com/davison/md-notes/pull/12#issuecomment-5574887059)).

### Tags are computed per request, with no index

Recorded on [#11](https://github.com/davison/md-notes/issues/11#issuecomment-5574646763).

- **Trade-off:** each request re-reads the corpus; fine for a notes folder, slow for
  a huge code root, where tags are rarely wanted anyway.
- **Rejected:** a persisted index, which would need invalidation the watcher would
  have to drive.

### Deferring review observations rather than pushing them after approval

Three times, low-severity observations from a re-review were deferred to the next
task rather than pushed onto an approved branch: the navigator's partial-listing
warning and streaming
([#6](https://github.com/davison/md-notes/pull/6#issuecomment-5573781278)), the
rendering nit about a note wearing the app's layout classes
([#7](https://github.com/davison/md-notes/pull/7#issuecomment-5574123924)), and six
watcher observations from the live-update re-review
([#8](https://github.com/davison/md-notes/issues/8#issuecomment-5574635558)).

- **Trade-off, as recorded:** until the next task lands, an unreadable subfolder is
  omitted without a log line, a very large root buffers a few megabytes briefly, a
  note can give a code span a stray border, a kernel event overflow can leave a tab
  stale, and a batch of thousands of paths costs about a second of CPU.
- **Rejected:** another review round for changes that are small, low, and about to be
  touched again by the next task anyway.

Each deferral named the task that would carry it, and each was carried: the navigator
items landed in the live-update PR
([#9](https://github.com/davison/md-notes/pull/9)) and the watcher items in the
search PR ([#12](https://github.com/davison/md-notes/pull/12)). The one exception is
the class-leak nit, which QA later found to be wider than the single `nav` class it
was recorded as and which is now backlog issue
[#14](https://github.com/davison/md-notes/issues/14).

## Deviations

### Two flags not in the plan, and one commit folded into another

[#2](https://github.com/davison/md-notes/issues/2#issuecomment-5572611597).
`mdn serve --state FILE` and `mdn open --no-browser` were added so tests and the
smoke test can run the real binary against temporary paths without touching the
user's config or opening a browser. The serve wiring landed in the UI shell commit
rather than a commit of its own. Neither flag changes any requirement's meaning.

### The render package landed as one commit

[#5](https://github.com/davison/md-notes/issues/5#issuecomment-5573921263). The
plan's first three commits (render package, link rewriting, sanitisation) landed
together as one — on `main` this is
[`2a1aa17`](https://github.com/davison/md-notes/commit/2a1aa17) — because the
rewriting transformer and the sanitiser policy were inseparable from the package's
tests. Splitting after the fact would have produced commits that do not build or test
on their own, which the implementer judged worse for the record than one larger
commit.

### Raw HTML `<img>` and `<a>` keep their attributes unchanged

[#5](https://github.com/davison/md-notes/issues/5#issuecomment-5573921263). The
rewriter works on the markdown AST, and raw HTML blocks pass through it opaque, so
only markdown image and link syntax is resolved against the root. The clipper will
emit markdown image syntax, so this affects hand-written HTML only. Recorded as a
known gap rather than left as a surprise, and pinned by a test.

### `Render` takes a slug, not a root value

[#5](https://github.com/davison/md-notes/issues/5#issuecomment-5574025766). The
renderer needs only the slug for URLs and the note's directory for resolving links;
taking the root value would couple the package to the registry for no use. Recorded
against the reviewer's finding 9 on
[#7](https://github.com/davison/md-notes/pull/7#issuecomment-5573996511).

### The watch set is larger than "directories the navigator would list"

[#8](https://github.com/davison/md-notes/issues/8#issuecomment-5574412848), as
amended by [#8](https://github.com/davison/md-notes/issues/8#issuecomment-5574635558).
This is the milestone's most consequential deviation and the one most worth
understanding later.

The plan said the watched directory set would come from the same ripgrep listing the
navigator uses. What shipped watches every directory holding a file ripgrep lists,
their ancestors, and any subtree beneath those that contains no files at all. The
reasoning, as recorded: a directory that is empty at startup holds nothing ripgrep
can list, yet the first note created there must be seen; a file-less subtree has
nothing for an ignore rule to match, so watching it cannot pull in an ignored tree,
while a subtree that has files ripgrep does not list is ignored or hidden and stays
unwatched. A directory that appears at runtime recomputes the set from the whole
root, because ripgrep never applies ignore rules to a directory it is pointed at.

The rule arrived in two steps. The first version watched direct subdirectories
unconditionally, which the reviewer showed watched ignored trees
([#9, finding 6](https://github.com/davison/md-notes/pull/9#issuecomment-5574348901));
the fix made the fileless-subtree rule the actual criterion
([`dd961f3`](https://github.com/davison/md-notes/commit/dd961f3)). The amendment then
stated plainly what that implies and the earlier commit title
([`b80ab66`](https://github.com/davison/md-notes/commit/b80ab66)) had denied: a
directory whose only files are all ignored is *not* watched, and a note created there
is seen only when the directory next appears in a batch or the daemon restarts.

QA found a second door into the same hole — a directory whose only pre-existing file
is *hidden*, such as a `.gitkeep` placeholder, falls through both arms of the rule for
the same reason
([#8](https://github.com/davison/md-notes/issues/8#issuecomment-5575178838)) — and
measured the other end of the trade: on `/usr/share` registered as an ad-hoc root, 310
navigator directories against 5,790 inotify watches
([#8](https://github.com/davison/md-notes/issues/8#issuecomment-5575181315)). Both are
now [#13](https://github.com/davison/md-notes/issues/13).

### `tags.Collect` returns a sorted list, not a map

[#11](https://github.com/davison/md-notes/issues/11#issuecomment-5574834895). The
endpoint wants the sorted list with counts, so the package produces it directly: one
shape for both the package and the API, and the sort order is tested where it is
computed.

### Line anchors for blocks whose renderers discard attributes

[#11](https://github.com/davison/md-notes/issues/11#issuecomment-5574834895). Fenced
and indented code, raw HTML and rules get an empty `line-anchor` div placed before
them instead of a `data-line` attribute, because chroma and goldmark's code and HTML
renderers drop node attributes. The note view scrolls to the anchor and flashes the
block after it. Without this, R6's scroll-to-match landed on the wrong block for a
hit inside a code fence
([#12, finding 4](https://github.com/davison/md-notes/pull/12#issuecomment-5574796558))
— which, in a notes folder, is where a large share of hits live.

### Search excludes dotfiles with an explicit glob

[#11](https://github.com/davison/md-notes/issues/11#issuecomment-5574834895). The
plan was right that ripgrep's markdown *type* filter does not override ignore rules,
but wrong that it changes nothing else: it re-includes hidden files, so search
returned hits in files the navigator refuses to show
([#12, finding 1](https://github.com/davison/md-notes/pull/12#issuecomment-5574796558)).
Search now passes an exclusion glob for dotfiles, matching the navigator.

## What the reviews changed

Every PR went through review, at least one round of fixes, and a re-review; three
needed two rounds. The reviewer seat is routed to the same identity as the author
(pure solo tier), so each review is a comment rather than a formal approval, and the
operator confirmed each merge explicitly — for example
[#3](https://github.com/davison/md-notes/pull/3#issuecomment-5572876724).

The pattern worth keeping is that findings were confirmed by execution rather than by
reading, in both directions: the reviewer reproduced each defect against a running
daemon and then re-verified each fix the same way
([#9 re-review](https://github.com/davison/md-notes/pull/9#issuecomment-5574629818),
[#12 re-review](https://github.com/davison/md-notes/pull/12#issuecomment-5574960200)).
That caught two regressions introduced *by* fixes — the sanitiser class allow-list
silently stripping chroma's `c1`, `s1` and `s2` so comments and strings lost
highlighting in almost every language, and the H1-removal fix deleting nested
headings
([#7 re-review](https://github.com/davison/md-notes/pull/7#issuecomment-5574074357)).

Classes of defect the loop caught, by task:

- **Skeleton ([#3](https://github.com/davison/md-notes/pull/3#issuecomment-5572710294)):**
  `make check` could not run from a clean checkout and CI was red; `make clean`
  deleted a tracked file and broke the build; the raw endpoint was a same-origin HTML
  sink; `within()` was wrong when the root is `/`; slugs were re-derived at every
  start, so root URLs were neither stable nor unique. Twelve findings, all fixed
  ([response](https://github.com/davison/md-notes/pull/3#issuecomment-5572740050)),
  and a further round for two new ones, including a recents-cache problem that stopped
  the daemon serving the notes root at all
  ([#3](https://github.com/davison/md-notes/pull/3#issuecomment-5572799032)).
- **Navigator ([#6](https://github.com/davison/md-notes/pull/6#issuecomment-5573706952)):**
  include globs defeated file-level gitignore, so M1-R3 was not met as written; a
  backslash rewrite corrupted legal Linux filenames into phantom tree nodes; one
  unreadable directory took down the whole tree. Nine findings, all fixed. Finding 1
  was fixed by removing the include globs entirely and filtering extensions in Go,
  which preserved the recorded `.md`/`.markdown` decision instead of widening it to
  ripgrep's built-in type.
- **Rendering ([#7](https://github.com/davison/md-notes/pull/7#issuecomment-5573996511)):**
  an empty frontmatter block rendered as two rules in the body; the plan's premise
  about escaping links did not match reality; the title rendered twice. Ten findings,
  then four more in re-review.
- **Live update ([#9](https://github.com/davison/md-notes/pull/9#issuecomment-5574348901)):**
  `Dirs` mutated the map it was ranging over, so the watch set was non-deterministic;
  renaming a directory permanently stopped live update inside it; one 404 stuck on
  every note for the rest of the session; ignored trees were watched, which the plan,
  the PR body and the README all denied. Fifteen findings. Two structural fixes came
  out of it — reconciling the watch set at flush time with removals before additions,
  and making the fileless-subtree rule the criterion — and one finding was declined
  with a reason rather than fixed.
- **Search and tags ([#12](https://github.com/davison/md-notes/pull/12#issuecomment-5574796558)):**
  search read files the navigator hides; a single long line broke search for the whole
  root; clearing the search box after a truncated result threw. Ten findings, then six
  more in re-review, one of which was a regression the long-line fix introduced — a
  hang traded for an unbounded response, fixed by windowing a hit line to 300 units
  around its first match
  ([#12](https://github.com/davison/md-notes/pull/12#issuecomment-5574902087)).

## Known gaps at the boundary

The operator's disposition of the three low QA findings, recorded on
[#1](https://github.com/davison/md-notes/issues/1#issuecomment-5575933501):

| Finding | Disposition |
|---------|-------------|
| README says WebSocket, and misstates the watch rule | Corrected in this document's PR |
| Hidden-only directories unwatched; watch count on large roots | [#13](https://github.com/davison/md-notes/issues/13) |
| Short app classes reachable from note content | [#14](https://github.com/davison/md-notes/issues/14) |

- **Trade-off, as recorded:** two small live-update gaps and one cosmetic class leak
  remain in M1 as shipped.
- **Rejected:** a remedy task now, since every requirement's verdict is satisfied and
  the remaining items are refinements the editor milestone will touch anyway.

Also carried forward, from discussion rather than from a defect:
[#10](https://github.com/davison/md-notes/issues/10), remote access over the tailnet
via `tailscale serve`, which the issue records as needing to land with or after the
authentication path the browser extension will require.

Beyond those, three things are true of M1 as shipped and are not defects, but will
surprise someone who has not read this far:

- Symlinked files and directories inside a root are absent from the navigator and
  from search, which is consistent with the confinement policy but is documented
  nowhere else ([#1](https://github.com/davison/md-notes/issues/1#issuecomment-5575193559)).
- Hidden and gitignored files are readable by direct URL even though the navigator
  and search hide them, which the accepted single-user premise covers explicitly
  ([#2](https://github.com/davison/md-notes/issues/2#issuecomment-5572874194)).
- More than about six open tabs of the app will starve the extra ones of live update
  ([#8](https://github.com/davison/md-notes/issues/8#issuecomment-5574412848)).

## Where the record is silent

Two gaps, named rather than filled:

- **The milestone declared no gates.** The Gates section of
  [#1](https://github.com/davison/md-notes/issues/1) still holds its scaffolded
  placeholder, so nothing records what "done" was to mean beyond CI. In practice the
  QA verdict comment became that definition, and it was thorough — but that was never
  decided, only done.
- **Library choices below the founding decision were never recorded as decisions.**
  Go, goldmark and fsnotify are covered by
  [#1](https://github.com/davison/md-notes/issues/1#issuecomment-5572364167), and
  bluemonday by the sanitisation decision on
  [#5](https://github.com/davison/md-notes/issues/5#issuecomment-5573615027). Chroma
  (through `goldmark-highlighting`), `preact-iso` for routing, `yaml.v3` for
  frontmatter and config, and Vitest with Testing Library for the UI suite appear
  only inside task plans or in `go.mod` and `ui/package.json`. No trade-off and no
  rejected alternative is recorded for any of them; anyone revisiting one of those
  choices is starting from scratch.

A third, smaller silence: the default port `7337` appears in the skeleton plan on
[#2](https://github.com/davison/md-notes/issues/2) and nowhere else, with no
rationale.
