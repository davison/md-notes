# M2 — Editor, autosave and live-update follow-ups

Tracking issue: [#17](https://github.com/davison/md-notes/issues/17). Its five
implementation tasks — four planned, and one opened by the QA verdict — are merged on
`main` at [`619de06`](https://github.com/davison/md-notes/commit/619de06).

## Goal and outcome

The milestone's goal, as stated on [#17](https://github.com/davison/md-notes/issues/17),
was to turn the rendered viewer into a local markdown editor with reliable autosave
and explicit conflict handling, and to deliver the two follow-ups milestone one's QA
left open. The operator's approval fixed the scope in the same breath: editing
*existing* notes across registered roots, with note creation, deletion and renaming
out of it, and browser clipping, inbox processing and authenticated remote access
([#10](https://github.com/davison/md-notes/issues/10)) left for later
([#17](https://github.com/davison/md-notes/issues/17#issuecomment-5576208722)).

What shipped is an editor over the same notes the viewer already rendered. The
daemon gained a versioned source endpoint — read `{source, revision}`, PUT the
complete new source with that revision — whose saves are serialized and whose stale
revisions are refused. The page gained a CodeMirror 6 editor with vim keybindings
behind a single `Ctrl+E` toggle, autosave one second after typing stops, a save
state in the note bar, and a conflict banner that keeps the draft whatever happens
to the file underneath it. Alongside those, a directory whose only files are hidden
placeholders is now watched, every root's watch set is bounded by a budget spent in
priority order with limited coverage reported in the browser, and rendered note
content can no longer wear the application's own CSS classes. One thing shipped
because QA stopped the milestone to get it: the editor keeps a note's line endings
through an edit, which the first four tasks did not
([#28](https://github.com/davison/md-notes/issues/28)).

The system as it stands is described in [the introduction](../introduction.md);
[Editing](../introduction.md#editing) is the part this milestone added.

Four implementation tasks delivered it, each through its own PR and review loop; a
fifth was opened by QA's verdict and merged before the milestone closed, and this
document is the sixth ([#22](https://github.com/davison/md-notes/issues/22)):

| Task | Requirements | PR |
|------|--------------|----|
| [#18](https://github.com/davison/md-notes/issues/18) Versioned markdown reads and safe conditional saves | M2-R2, M2-R3 (daemon side) | [#23](https://github.com/davison/md-notes/pull/23) |
| [#19](https://github.com/davison/md-notes/issues/19) CodeMirror editor, autosave and conflict handling | M2-R1, M2-R2, M2-R3 | [#24](https://github.com/davison/md-notes/pull/24) |
| [#20](https://github.com/davison/md-notes/issues/20) Watch hidden-only directories and define large-root resource policy | M2-R4 | [#26](https://github.com/davison/md-notes/pull/26) |
| [#21](https://github.com/davison/md-notes/issues/21) Isolate application CSS from rendered note classes | M2-R5 | [#25](https://github.com/davison/md-notes/pull/25) |
| [#28](https://github.com/davison/md-notes/issues/28) Preserve the note's line endings in the editor | M2-R2 (QA finding 1) | [#29](https://github.com/davison/md-notes/pull/29) |

[#20](https://github.com/davison/md-notes/issues/20) and
[#21](https://github.com/davison/md-notes/issues/21) formally adopted the two low QA
findings milestone one dispositioned to the backlog —
[#13](https://github.com/davison/md-notes/issues/13) and
[#14](https://github.com/davison/md-notes/issues/14) — and closed them on merge.

## Requirement outcomes

The verdicts below are drawn from the independent QA comment on the milestone issue
([#17](https://github.com/davison/md-notes/issues/17#issuecomment-5589684484)), run
against the binary `make build` produced from merged `main` at
[`4c12d24`](https://github.com/davison/md-notes/commit/4c12d24) — three daemons on
temporary configs, state files and roots, direct exercise of the source API, and the
UI driven headlessly in Chromium with two browser contexts for the two-session case.
It raised four findings; the operator's disposition of them is at
[#17](https://github.com/davison/md-notes/issues/17#issuecomment-5591886325). One
was blocking, and M2-R2's row below comes instead from the re-verification run
against `main` at [`619de06`](https://github.com/davison/md-notes/commit/619de06)
once it was fixed
([#17](https://github.com/davison/md-notes/issues/17#issuecomment-5592034687)),
which supersedes that row of the first verdict and no other.

| ID | Requirement | Status |
|----|-------------|--------|
| M2-R1 | CodeMirror editor with vim keybindings, one view/edit shortcut that does not steal typing | [Satisfied](https://github.com/davison/md-notes/issues/17#issuecomment-5589684484) |
| M2-R2 | Autosave to the original file preserving frontmatter and source text, states shown, failed saves retained and retryable, writes confined and guarded | [Not satisfied at `4c12d24`](https://github.com/davison/md-notes/issues/17#issuecomment-5589684484) on finding 1; [satisfied at `619de06`](https://github.com/davison/md-notes/issues/17#issuecomment-5592034687) after [#28](https://github.com/davison/md-notes/issues/28) / [#29](https://github.com/davison/md-notes/pull/29) |
| M2-R3 | Serialized saves, stale revisions refused, drafts never discarded by a conflict, visible conflict states | [Satisfied](https://github.com/davison/md-notes/issues/17#issuecomment-5589684484) — one low finding filed ([#30](https://github.com/davison/md-notes/issues/30)) |
| M2-R4 | First note in a hidden-only directory seen live; documented, justified watch policy and clear reporting | [Satisfied](https://github.com/davison/md-notes/issues/17#issuecomment-5589684484) |
| M2-R5 | Note content cannot apply the application's layout classes; highlighting still works | [Satisfied](https://github.com/davison/md-notes/issues/17#issuecomment-5589684484) — one low finding filed ([#31](https://github.com/davison/md-notes/issues/31)) |
| M2-R6 | Documentation explains editing, autosave, conflicts and watch limitations; roadmap and this record | [Provisional](https://github.com/davison/md-notes/issues/17#issuecomment-5589684484) on the state of `main` before this task, and satisfied on the merge of [#27](https://github.com/davison/md-notes/pull/27) under the milestone's closure gate ([#17](https://github.com/davison/md-notes/issues/17)) |

M2-R2's verdict is the one the milestone turned on. QA found that the editor rewrote
a note's CRLF or lone-CR line endings to LF on the first keystroke — 48 bytes and
nine CRLF became 40 bytes and none — against M2-R2's "preserving frontmatter and
source text" and against what
[the introduction](../introduction.md#conditional-saves) already claimed. The daemon
was not at fault: a PUT of the exact bytes a GET returned reproduced the file with an
identical md5, so the loss was entirely in the editor, and both sides' suites passed
because neither contained a carriage return. The operator ruled it blocking and it
was fixed as [#28](https://github.com/davison/md-notes/issues/28); see
[Line endings are restored at the editor's boundary](#line-endings-are-restored-at-the-editors-boundary).
The re-verification byte-compared fifteen fixtures through the editor: every
pure-ending note — CRLF with frontmatter, lone CR, LF, a BOM, hard tabs, trailing
spaces, no final newline, Unicode and an empty file — came back as its original bytes
with exactly the typed character added, opening the editor and toggling back without
typing left all fifteen md5-identical, and the five mixed fixtures came back uniform
in the ending the rule picks, both tie directions included
([#17](https://github.com/davison/md-notes/issues/17#issuecomment-5592034687)).

Every finding across the two QA passes, and what was done with it
([#17](https://github.com/davison/md-notes/issues/17#issuecomment-5591886325) for the
first four):

| Finding | Disposition |
|---------|-------------|
| 1 — the editor rewrites CRLF and lone-CR line endings on the first keystroke | Blocking; fixed by [#28](https://github.com/davison/md-notes/issues/28) / [#29](https://github.com/davison/md-notes/pull/29) |
| 2 — a *clean* session enters a deleted-note conflict claiming unsaved edits | Backlog, captured as [#30](https://github.com/davison/md-notes/issues/30) |
| 3 — statements on `main` the merged work made false | Folded into [#22](https://github.com/davison/md-notes/issues/22), this document's task |
| A (re-verification) — the mixed-endings normalisation was still undocumented on `main` | Confirmed as landing in [#22](https://github.com/davison/md-notes/issues/22), not a separate issue: it is the line-endings paragraph in [Editing](../introduction.md#editing), added by [#27](https://github.com/davison/md-notes/pull/27) and recorded as a deviation from this task's plan ([#22](https://github.com/davison/md-notes/issues/22#issuecomment-5589950252)) |
| 4 — note content can forge `line-anchor` and redirect a search hit's scroll target | Backlog, captured as [#31](https://github.com/davison/md-notes/issues/31), with the same observation from the [#21](https://github.com/davison/md-notes/issues/21) review |

- **Trade-off, as recorded:** findings 2 and 4 put no data at risk and neither falls
  within a requirement's wording, so fixing them now would extend the milestone for
  cosmetic outcomes.
- **Rejected:** a remedy task inside this milestone; both captures record a clear
  shape for a later task to adopt.

## Decisions

### Saves are conditional on a revision, and serialized in the daemon

The requirement that became M2-R3 originally read as a disk-write guarantee. The
implementer raised it as a decision gate before writing any code, because the
guarantee as written could not be met without a storage or locking protocol every
external editor and Syncthing would also have to join
([#18](https://github.com/davison/md-notes/issues/18#issuecomment-5576229165)). The
operator resolved it in favour of serialized daemon saves plus a final revision
check immediately before replacement, with the residual race documented
([#18](https://github.com/davison/md-notes/issues/18#issuecomment-5576240416)), and
amended M2-R3's text to match
([#17](https://github.com/davison/md-notes/issues/17#issuecomment-5576240669)).

- **Trade-off:** notes stay ordinary markdown files that any tool can edit; the
  daemon reliably refuses a competing browser save and a detected external change,
  but an uncoordinated external editor can still write between the final check and
  the rename.
- **Rejected:** claiming atomic compare-and-swap against arbitrary external writers.

This is the boundary M2-R3's verdict should be read within, and it is stated in the
API documentation rather than left in the record
([the introduction](../introduction.md#conditional-saves)).

### The source API's shape: opaque revisions, one lock, hard limits

Recorded on [#18](https://github.com/davison/md-notes/issues/18#issuecomment-5576251236)
and, after implementation,
[#18](https://github.com/davison/md-notes/issues/18#issuecomment-5576315867). A
revision is an opaque token backed by file identity, content digest, modification
time and permissions; it is local to one daemon session, so a restart makes every
old token conflict and clients must re-read. One store lock covers reads and saves
across every root and alias. Editable source is bounded at 8 MiB and must be valid
UTF-8.

- **Trade-off:** one small revision record per canonical path accessed, and source
  operations serialized, which is straightforward correctness for a single-user
  local daemon. Directory-handle-confined staging and same-directory replacement
  preserve the nine Unix permission bits but create a new inode, so hard-link
  identity, ownership, ACLs and extended attributes are not preserved; the staging
  file is synced, the parent directory is not, so rename durability across power
  loss is not promised.
- **Rejected:** content-only tokens, which miss a same-content file replacement;
  unlocked check-then-write sequences, which let two browser tabs both win; and
  silently normalising non-UTF-8 source instead of refusing it.

### `Ctrl+E` is the view/edit toggle

Recorded on [#19](https://github.com/davison/md-notes/issues/19#issuecomment-5576722312).
Handled once at document level in the capture phase, with the note bar's button as
the mouse equivalent.

- **Trade-off:** a modifier chord never fires on plain typing, which is what M2-R1
  asks for; it behaves identically in both modes and from the navigator or the
  search box; and it is not one of Chromium's reserved shortcuts, so
  `preventDefault` keeps it away from the omnibox. The cost is vim's own `Ctrl+E`
  (scroll one line), which the editor shadows — almost nothing in a wrapped-line
  notes editor.
- **Rejected:** `Escape`, which belongs to vim; a bare key such as `e` or `i`, which
  would steal typing; and a two-key chord, which is harder to hit for a flip that
  happens constantly.

Its companion, recorded later on
[#19](https://github.com/davison/md-notes/issues/19#issuecomment-5576865994):
`indentWithTab` stays bound, so `Tab` indents rather than moving focus. A vim editor
that loses focus on `Tab` is unusable, and `Ctrl+E` is already the way out.

### Editing state lives outside the component tree

Recorded on [#19](https://github.com/davison/md-notes/issues/19#issuecomment-5576722446).
One session per note in `ui/src/session.ts`, holding the base `{source, revision}`,
the draft, the status and the parked CodeMirror state; dirty drafts are additionally
mirrored to `localStorage`, best effort, and recovered on the next open.

- **Trade-off:** this is what makes M2-R3's "navigation and mode switching do not
  silently lose pending edits" hold — navigating away flushes, and anything that
  does not land stays in the store, is listed in the top bar and reopens in the
  editor. The storage mirror extends that across a reload or a crash, at one write
  per keystroke; notes are small, and sessions are never evicted for the same
  reason.
- **Rejected:** mode or draft in the URL, where a reload would still lose the draft;
  and keeping the draft in the editor component, where a navigation would drop it.

### A conflict is about content, not about revisions

Recorded on [#19](https://github.com/davison/md-notes/issues/19#issuecomment-5576722553).
A conflict is entered only when the file's content differs from the base under a
dirty, in-flight or failed draft. A new revision over identical content — a `touch`,
a permission change, a same-bytes rewrite, the daemon's own event for our save — is
adopted silently, and a save refused with 409 for such a revision is resent once
against the new one.

- **Trade-off:** the revisions from [#18](https://github.com/davison/md-notes/issues/18)
  deliberately track identity, mtime and permissions as well as content, which is
  right for the daemon but would raise conflicts a user cannot see the point of.
  Comparing content on the client keeps conflicts meaningful while every save still
  goes through the daemon's conditional check. The single resend bounds the cost of
  a file being touched repeatedly: a second 409 stays "Save failed" with Retry.
- **Rejected:** treating every revision change as a conflict — noisy, and confirmed
  in headless testing where a `chmod` alone produced one — and unlimited automatic
  resends, which loop if an external process keeps touching the file.

### A deleted note keeps its draft, and the editor cannot recreate it

Recorded on [#19](https://github.com/davison/md-notes/issues/19#issuecomment-5576722702).
The banner for a deleted note offers **Copy draft** and **Discard draft** only; if
the file reappears the conflict becomes a "changed" one and **Keep my draft** saves
over it. A clean document whose file is deleted is kept rather than dropped.

- **Trade-off:** the save API has no create-on-missing path, so an option to
  recreate the file would be an option that cannot work; saying so in the banner is
  more honest. Keeping the text of a clean document that vanished costs nothing and
  may be the only copy left.
- **Rejected:** extending the save API with a create path. That would reopen
  [#18](https://github.com/davison/md-notes/issues/18)'s decision, and creating
  notes belongs with a task that decides where and how new notes are made.

### Autosave at one second, with flush points that do not wait

Recorded on [#19](https://github.com/davison/md-notes/issues/19#issuecomment-5576722865).
A save is sent one second after typing stops; a mode switch, navigating away, losing
window focus, `:w`/`:wq`, and page unload flush immediately, the last with a
`keepalive` request plus the browser's leave prompt when anything is still unsaved.

- **Trade-off:** one second keeps a save from landing mid-word, which matters
  because every save is a full-file write and a live-update event, while the flush
  points mean nothing waits on the timer when it matters. The unload prompt cannot
  be avoided honestly: `keepalive` sends the save but the page cannot learn whether
  it landed, so the draft stays in storage and is reconciled on the next open.
  Browsers cap a `keepalive` body at 64 KiB, so a larger draft's final save is
  refused and recovered from storage instead
  ([#19](https://github.com/davison/md-notes/issues/19#issuecomment-5576865994)).
- **Rejected:** saving on every keystroke — churn on disk and in the watcher — and a
  longer interval, which loses more on a crash.

An addendum on the same comment thread settled what the vim commands mean, and it is
the one behaviour a vim user is most likely to assume wrongly: `:q`, `:q!`, `:x` and
`:wq` all **save**, because the editor never closes and autosave would have written
the draft anyway
([#19](https://github.com/davison/md-notes/issues/19#issuecomment-5576919511)).

### Line endings are restored at the editor's boundary

Recorded on [#28](https://github.com/davison/md-notes/issues/28#issuecomment-5589728523),
the task QA's blocking finding opened. CodeMirror splits a document on any line
ending and joins with LF, and the editor handed that text straight to the session, so
one keystroke rewrote every ending in a CRLF or lone-CR note. The fix keeps
CodeMirror's default splitting and converts at the boundary: when the editor hands
its document to the session it rejoins with the ending the note had when the state
was built.

- **Trade-off:** setting `EditorState.lineSeparator` instead would make CodeMirror
  split only on the configured separator, so a pasted or typed `\n` inside a CRLF
  note would become literal content within a line rather than a line break — and
  `doc.toString()` joins with LF regardless, so only `sliceDoc()` honours the facet,
  a trap the reviewer confirmed. Converting at the boundary keeps every editing
  operation in the editor's native LF form and makes the exact-bytes guarantee hold
  for the shapes that occur in practice: pure LF, pure CRLF and pure CR. What is
  given up is byte preservation of a note that *already* mixes endings, which is
  normalised to its dominant one on the first edit.
- **Rejected:** the `lineSeparator` facet, for the paste and mixed-content behaviour
  above; and doing nothing in the editor while narrowing the documentation to a
  daemon-only guarantee, which is what QA found misleading in the first place.

The rule the code applies is *dominance by count*, ties to LF and then to CRLF, so a
stray carriage return never decides a whole file. That is not what the decision first
said — it described a precedence order, "CRLF if the note contains one, else a lone
CR, else LF" — and round one of the review of
[#29](https://github.com/davison/md-notes/pull/29#issuecomment-5589824871) showed the
difference matters: a mostly-LF note containing one CRLF, or one stray CR, would have
been converted wholesale. The by-count rule is recorded as part of the deviation
([#28](https://github.com/davison/md-notes/issues/28#issuecomment-5589841459)), and
the deviation is what the code and the user documentation follow.

Two things the fix repaired beyond the finding, both measured by the reviewer against
the built binary
([#29](https://github.com/davison/md-notes/pull/29#issuecomment-5589824871)):
`Enter` now inserts the note's own ending rather than an LF, and an undo back to the
original text now reaches *clean* — on `main` the LF-normalised draft no longer
equalled `base.source`, so undoing to where you started still saved a rewritten file.

### The storage mirror is one record per note, shared by every tab

Recorded on [#19](https://github.com/davison/md-notes/issues/19#issuecomment-5576865725)
in answer to a review finding, and extended by an addendum
([#19](https://github.com/davison/md-notes/issues/19#issuecomment-5576919511)). A
session removes only a record it wrote itself or recovered, so another tab settling
to clean no longer erases it — but two tabs editing one note still overwrite each
other's record, and a second tab opening a note whose first tab holds a live dirty
draft adopts that draft from storage and autosaves it within a second.

- **Trade-off:** two tabs editing one note is already the stale-save conflict path,
  where the second save is refused and its draft kept in memory. The in-memory
  session, the top-bar list and the unload prompt are the guarantee against loss;
  the mirror is crash and reload recovery, and making it two-tab-safe as well is
  more machinery than the corner warrants.
- **Rejected:** per-tab keys with recovery of other tabs' records, which would
  silently save a live tab's draft from a second tab; and per-tab keys recovered
  only by the same tab via `sessionStorage`, which loses recovery after a deliberate
  close.

### `@codemirror/language-data` ships whole, statically imported

Recorded on [#19](https://github.com/davison/md-notes/issues/19#issuecomment-5576865448)
after the reviewer measured the cost on
[#24](https://github.com/davison/md-notes/pull/24): `ui/dist` grows from 38 KB to
1.72 MB across 115 files, nearly all of them lazily loaded per-language parsers, and
the eager entry bundle from 29 KB to 406 KB (11 KB to 143 KB gzipped), all embedded
in the `mdn` binary.

- **Trade-off:** one of the project's stated uses is opening a code project and
  reading its docs, where fenced blocks in many languages are the norm; the rendered
  view already highlights them all through chroma, and the editor should not be the
  poorer half. The eager cost is paid once per page load over loopback and then
  cached; the per-language chunks cost nothing until a block of that language is
  edited, and a static import keeps the toggle instant, which is the point of a
  one-key flip.
- **Rejected:** a hand-picked language list, which guesses wrong for the
  code-project case and saves little of the eager bundle (that is CodeMirror
  itself); and lazy-loading the editor chunk, which buys a first-toggle pause on a
  local application to save 130 KB gzipped once.

Recorded as a decision rather than raised as a gate, because the plan named
`language-data` and the reviewer flagged the size as a trade-off to record, not as a
defect.

### A directory whose only files are hidden is watched; one whose only files are ignored is not

Recorded on [#20](https://github.com/davison/md-notes/issues/20#issuecomment-5577085446),
then reshaped twice by review and once by a decision gate — see
[the nested-placeholder gate](#the-nested-placeholder-gate-and-one-listing-rather-than-two)
below for where it ended up.

The two cases look identical from outside: a `.gitkeep` placeholder and a
gitignored file both leave a directory that ripgrep lists nothing from. They cost
differently to tell apart. Hidden files can be asked for in the listing that already
runs; deciding a file is *ignored* means asking ripgrep about it, and treating "not
listed" as "absent" would make `node_modules` look empty and pull a whole ignored
tree under watch — exactly the bug
[`dd961f3`](https://github.com/davison/md-notes/commit/dd961f3) fixed in milestone
one.

- **Trade-off:** the hidden-only hole from
  [#13](https://github.com/davison/md-notes/issues/13) closes; the ignored-only hole
  recorded on [#8](https://github.com/davison/md-notes/issues/8) stays open, and is
  now stated plainly in the introduction and the README instead of implied.
- **Rejected:** running ripgrep per candidate directory to distinguish ignored from
  absent — one process per directory probed while listing a root, seconds on
  `/usr/share`, for a case a user meets only when they gitignore the folder they
  then write notes into.

### A per-root watch budget, spent in priority order

Recorded on [#20](https://github.com/davison/md-notes/issues/20#issuecomment-5577087558),
with the measurements taken on one machine before any change. The policy is
`max_watches` — 8192 per root by default, `--max-watches N` to override, `0` for no
budget — spent in priority order: (1) the root, directories holding a markdown file,
and their ancestors; (2) directories holding nothing the navigator would list; (3)
the rest, directories holding files but no note. Group three is 83–97% of the watch
set on every large root, so it is what a spent budget gives up first
([the numbers as the code finally counts them](../introduction.md#the-watch-budget)).

- **Trade-off:** 8192 is a judgment call. An inotify watch costs about a kilobyte of
  unswappable kernel memory from a per-user pool commonly set to 524,288 and shared
  with every editor and file manager the user is running. 8192 covers `/usr/share`
  whole, is a sixty-fourth of that pool, and matches the smallest
  `fs.inotify.max_user_watches` still shipped, so one ad-hoc root cannot exhaust an
  unraised system by itself — at the cost of `/usr/lib` and `~/.cache` being only
  partly live out of the box, which is what the reporting exists to say.
- **Rejected:** watching only directories that already hold markdown. It would cut
  `/usr/share` from 5,790 watches to 311 with no budget needed, but it silently
  drops the case where the first note appears in a folder of images or PDFs — a
  coverage regression for every root, to fix a problem only large ones have.
- **Rejected:** no budget at all, with the growth documented and the kernel limit
  left to the user. That is roughly milestone one's behaviour, and it leaves one
  registered root able to take 16% of the machine's watches (`~/.cache`, about 87 MB
  of kernel memory) without saying so.

The order also applies after startup, which was a review finding rather than the
first cut: on a root whose budget is spent, a directory that outranks a watched one
takes its watch
([#20](https://github.com/davison/md-notes/issues/20#issuecomment-5577328490)).

- **Trade-off:** a watch can now be given up as well as placed, which opens a
  live-update gap for the displaced directory — the same gap every unwatched
  directory already has, moved to one the policy values less. Directories of equal
  rank never displace each other, so a settled root does not churn, and the root's
  own watch is never released.
- **Rejected:** documenting that the order applies to placement only, which the
  reviewer offered and which is honest, but would make "restart the daemon" the
  remedy for a notes directory going dark on a daemon meant to run under systemd;
  and raising the budget automatically when a high-priority directory cannot be
  placed, which makes the documented limit not a limit.

### Limited coverage is reported in the browser, not only in the log

Recorded on [#20](https://github.com/davison/md-notes/issues/20#issuecomment-5577089080).
A `status` event on the root's SSE stream carries the coverage, and a notice above
the navigator shows it.

- **Trade-off:** M2-R4 asks for "clear reporting when coverage is limited". The
  daemon's log alone would have been the smaller change and is where the existing
  inotify-exhaustion warning already goes, but the person who notices a note failing
  to appear is looking at the browser, not at `journalctl`. The stream sends the
  status on connect and again only when it changes, so an idle root costs one frame.
- **Rejected:** a separate `/api/r/{slug}/status` endpoint. Coverage belongs to the
  same thing the events stream already represents — whether this root is live — and
  a second endpoint would need its own polling to notice a runtime change.

A root whose watcher never started is the same story with nothing covered: it has no
hub, its stream answers 503, an `EventSource` treats that as a permanent close, and
the page says live update is not available for that root
([#20](https://github.com/davison/md-notes/issues/20#issuecomment-5577330689)).
Registering a hub for such a root so the stream stays open and reports zero was
rejected: it would hold open streams that can never carry an event, and 503 is the
honest answer to "is this root live".

The same comment records the sentinel: zero means "no budget" at every layer, and
"unset" is the file key being absent or the flag not being given. The shipped code
had `0` meaning "use the default" in configuration and "no cap" in the watcher, so
`max_watches: 0` could not ask for what the option plainly gives. An exported
`watch.NoBudget = -1` used in all three layers — which the reviewer suggested — was
rejected for leaving `max_watches: 0` silently meaning 8192, a value a user can
write and will not get.

### The nested-placeholder gate, and one listing rather than two

The narrowing that fixed the ignored-dotfile-tree finding (below, under
[Deviations](#deviations)) left a gap: a placeholder tree more than one level deep
was watched at no level at all. Whether to close it in code or accept it as a
documented limitation was a human decision, so it was raised as a gate rather than
decided by the implementer, with three options and their measured costs
([#20](https://github.com/davison/md-notes/issues/20#issuecomment-5577543916)).

The operator resolved it as option (b)
([#20](https://github.com/davison/md-notes/issues/20#issuecomment-5587683689)): list
hidden files ignore-aware, so a placeholder no ignore rule covers makes its
directory watchable at any depth and the special case in the walker goes away.

- **Trade-off, as resolved:** ripgrep runs over the listing path with `--hidden`,
  and hidden files enter a code path that previously ignored them, for a rule that
  means what M2-R4 says rather than a documented exception resembling the
  ignored-only one but not identical to it.
- **Rejected:** (a) accepting the gap as documented, which leaves M2-R4's sentence
  true only of the single-level shape; and (c) probing the rejected candidate's own
  top level, which buys one level for a rule harder to explain than either.

Implementing it, the shape changed: the resolution named a *second* ripgrep
invocation, and one carries both answers, because `rg --files --hidden --glob '!.*/'`
is a strict superset of `rg --files`
([#20](https://github.com/davison/md-notes/issues/20#issuecomment-5587850848)). The
cost figure in the gate — 2–20% — had been measured at the shell writing to
`/dev/null`; read through a pipe and scanned, one listing of `/usr/share` costs
215 ms in process against the 44 ms the shell showed, so a second listing would have
very nearly doubled `tree.Dirs` (+69–80%) instead of the band the operator accepted.
One listing costs 5–7%. The watch sets are identical on `/usr/share`, `/usr/lib`,
`~/projects` and this repository.

- **Trade-off:** it departs from the letter of the resolution, and the two-listing
  shape remains a two-line change if its separation is wanted.
- **Rejected:** shipping the literal two invocations and documenting the real cost —
  the record would carry a figure the operator accepted and a cost the code actually
  pays, for no behavioural difference; and filtering the second listing with an
  include glob such as `--glob '.*'`, which overrides ignore rules for the files it
  matches, the one property the whole option depends on *not* having.

### Rendered note content may carry only classes in a reserved namespace

Recorded on [#21](https://github.com/davison/md-notes/issues/21#issuecomment-5577001077),
adopting [#14](https://github.com/davison/md-notes/issues/14). Chroma emits its token
classes under `mdn-` via `chromahtml.ClassPrefix`, `internal/render/gencss` takes the
prefix from the renderer so markup and stylesheet cannot drift, and the sanitiser
admits `mdn-<lowercase alnum>` and nothing else on `pre`, `code` and `span`. The
note's own structural classes — goldmark's footnote classes, and this package's
`outside-root` and `line-anchor` — keep their exact-match allowance; they were never
the leak.

- **Trade-off:** the application's stylesheets now carry one naming rule — no class
  may begin with `mdn-` — which someone can break, so `TestAppClassesAreUnreachable`
  walks `ui/src` for every stylesheet the application writes, extracts every class
  named in a selector, and asserts each is stripped from note content on every
  element the policy allows a class on. Run against the old pattern it reports
  `nav`, `hit`, `tag`, `ctx` and `ln` surviving — which is exactly
  [#14](https://github.com/davison/md-notes/issues/14)'s finding, so the test is not
  vacuous. The cost is a generated `chroma.css` that is app-specific rather than
  stock, and slightly noisier rendered HTML.
- **Rejected:** prefixing or renaming the application's own short layout classes, or
  scoping every layout rule under `.shell` (the shape
  [#14](https://github.com/davison/md-notes/issues/14) first offered). Both fix
  today's collisions and leave the next one to whoever next names a class `ctx`; the
  guarantee should not depend on the application's naming at all. Also rejected:
  moving `outside-root` and `line-anchor` into the reserved namespace, since they are
  note classes rather than application layout classes and renaming them would change
  nothing about what a note can do.

Because the guarantee is about the namespace rather than about a list of names, the
editor's `cm-*` classes — which have shared the document with a rendered note since
[#19](https://github.com/davison/md-notes/issues/19) — are covered by the same
construction, and so is whatever the application adds next.

## Deviations

### A nest of placeholder directories, excluded and then restored

[#20](https://github.com/davison/md-notes/issues/20#issuecomment-5577540623). The
plan's test list said "a directory whose only file is `.gitkeep` is in the set; a
nest of them is too". The first implementation did cover the nest, by exempting
hidden files at every depth of the candidate's walk — and the model review of
[#26](https://github.com/davison/md-notes/pull/26#issuecomment-5577212242) showed
that rule also admitted a gitignored subtree whose files happen to be dotfiles: 202
watched directories where `main` watched one. Narrowing the exemption to the
candidate's own top level was the fix
([#20](https://github.com/davison/md-notes/issues/20#issuecomment-5577326649),
[`035a379`](https://github.com/davison/md-notes/commit/035a379)); losing the nest was
its cost, and `TestDirs` asserted the exclusion.

- **Rejected at that point:** amending the documentation instead, which would have
  made "respecting ignored subtrees" mean "except when the ignored files are hidden"
   — the class of bug
  [`dd961f3`](https://github.com/davison/md-notes/commit/dd961f3) fixed, re-entering
  by the back door.
- **Rejected at that point:** a second ripgrep pass with `--hidden`, on a cost
  measured — wrongly, as it turned out — at 3.5× on `~/projects`.

The gate resolution then restored the nest by exactly that route, measured properly
([`37c46e9`](https://github.com/davison/md-notes/commit/37c46e9)), so the deviation
is closed rather than carried: `emptySubtree`'s special case is gone and a file of
any kind disqualifies a subtree again, which is milestone one's rule.

### The `lineSeparator` facet, and a minority lone CR

[#28](https://github.com/davison/md-notes/issues/28#issuecomment-5589841459). Two
departures from that task's plan. Step 1 said the editor would configure CodeMirror's
`lineSeparator` facet; the implementation converts at the editor's boundary instead,
for the reasons in the decision above. Step 2 said a lone `\r` inside a CRLF note
would be preserved as content; it is not — CodeMirror treats it as a line break, so
it becomes the note's dominant ending on the first edit, like any other minority
ending.

Recorded because the PR body had claimed no deviations, which round one of the review
found untrue
([#29](https://github.com/davison/md-notes/pull/29#issuecomment-5589824871)).

### `config.Resolve` takes an `Overrides` struct

[#20](https://github.com/davison/md-notes/issues/20#issuecomment-5577089080). It
touches six existing call sites in the config tests, against
`Resolve("cfg.yml", dir, 0, 0)` at every future call site, where neither zero is
readable and a third override would make it worse.

## Corrections to the record itself

Three times, a review finding was not about the code but about what the record said
the code did. They are worth keeping, because in each case the tests were right and
the prose was wrong — twice in five places at once.

- **The narrowed guard's real cost.** The decision comment, the introduction, the
  `emptySubtree` comment, the PR body and a commit message all said a placeholder
  nest was "watched no further than its first level". The guard actually rejected
  the whole candidate the moment any file lay below its top level, so such a nest
  was watched at *no* level — including its own. The follow-up review of
  [#26](https://github.com/davison/md-notes/pull/26#issuecomment-5577482079) measured
  it; the correction carries the five measured layouts and two tests pin them, one
  of which pins that the gap was *silent* in all three reporting channels
  ([#20](https://github.com/davison/md-notes/issues/20#issuecomment-5577540406)).
- **"The more accurate of the two."** The one-listing decision said its single
  divergence from the two-listing shape — the group a directory lands in when its
  only listed file is a dotfile an explicit `!` rule un-ignores — put that directory
  in the better group. True of a `.gitkeep`; false when the un-ignored dotfile is
  *markdown*, where the navigator shows the note and the directory was nonetheless
  ranked below every note-holding one, first to lose its watch. Fixed by classifying
  markdown before hiddenness
  ([`4c12d24`](https://github.com/davison/md-notes/commit/4c12d24)), with the
  reviewer's root as the test
  ([#20](https://github.com/davison/md-notes/issues/20#issuecomment-5588186363)).
- **User documentation that did not exist yet.**
  [#28](https://github.com/davison/md-notes/issues/28#issuecomment-5589728523)'s
  decision justified normalising a mixed-endings note partly on the ground that "the
  record and the user documentation state" it. The record did; nothing on `main` did,
  because the introduction's Editing section was still unwritten in this task. Round
  one of the review of [#29](https://github.com/davison/md-notes/pull/29#issuecomment-5589824871)
  caught it, and the correction
  ([#28](https://github.com/davison/md-notes/issues/28#issuecomment-5589841653))
  restates the rule and hands the sentence to
  [#22](https://github.com/davison/md-notes/issues/22) — where it is now the
  line-endings paragraph in [Editing](../introduction.md#editing), recorded as a
  deviation from this task's own plan
  ([#22](https://github.com/davison/md-notes/issues/22#issuecomment-5589950252)).

A note for anyone following those comments into the history: they name the commits
by the SHAs they had on the task branch, which the rebase merge rewrote. `f676bd9` is
[`035a379`](https://github.com/davison/md-notes/commit/035a379) on `main`, `a71ced8`
is [`6e5fd7f`](https://github.com/davison/md-notes/commit/6e5fd7f), `816078d` is
[`fe4338b`](https://github.com/davison/md-notes/commit/fe4338b), `0fe99ed` is
[`e6c1d4d`](https://github.com/davison/md-notes/commit/e6c1d4d), `af977e3` is
[`37c46e9`](https://github.com/davison/md-notes/commit/37c46e9), `e07bfe4` is
[`b7f259b`](https://github.com/davison/md-notes/commit/b7f259b), and `56774b2` is
[`4c12d24`](https://github.com/davison/md-notes/commit/4c12d24).

## What the reviews and QA changed

Every PR went through review, at least one round of fixes, and a re-review;
[#26](https://github.com/davison/md-notes/pull/26) took three reviews and three
rounds of fixes, with a decision gate in the middle of them. The reviewer
seat is routed to the same identity as the author (pure solo tier), so each review is
a comment rather than a formal approval, and the operator confirmed each merge
explicitly — for example
[#23](https://github.com/davison/md-notes/pull/23#issuecomment-5576486486).

The habit milestone one established held: findings were confirmed by execution
rather than by reading, in both directions.

- **Source API ([#23](https://github.com/davison/md-notes/pull/23#issuecomment-5576330055)):**
  recommended approval with no blocking findings, after independently re-running the
  race suite and inspecting the revision comparisons, staging cleanup, symlink
  handling and request limits. The only milestone task whose review found nothing to
  fix.
- **Editor ([#24](https://github.com/davison/md-notes/pull/24#issuecomment-5576824396)):**
  two blocking items and several nits. `:q` and `:x` were undefined, so a vim user's
  most likely "done" command did nothing; the storage record was cleared by any
  session reaching clean, so a second tab settling erased the first tab's draft; and
  a claimed test for change batches reaching an editing session did not exist. All
  fixed ([`ad5e616`](https://github.com/davison/md-notes/commit/ad5e616),
  [`c3d23cc`](https://github.com/davison/md-notes/commit/c3d23cc)), and the reviewer
  re-verified each against the built binary
  ([#24](https://github.com/davison/md-notes/pull/24#issuecomment-5576915199)). The
  bundle size and the mirror's shared slot became decisions rather than fixes.
- **Class isolation ([#25](https://github.com/davison/md-notes/pull/25#issuecomment-5577086494)):**
  the guard test read one hardcoded stylesheet, so a stylesheet added later would
  not be scanned, and the element list it swept was asserted rather than derived from
  the policy. Both fixed by construction — the test now walks `ui/src` for every
  application stylesheet, and the policy and the test read the same two element
  slices — and the reviewer verified the walk by dropping scratch stylesheets into
  the tree and watching it fail
  ([#25](https://github.com/davison/md-notes/pull/25#issuecomment-5577141711)).
- **Watch coverage ([#26](https://github.com/davison/md-notes/pull/26#issuecomment-5577212242)):**
  the longest loop of the milestone. Round one: the hidden-file exemption admitted a
  gitignored dotfile tree (202 directories against one), and the priority order held
  at startup only, so a directory that gained notes on a full root stayed dark while
  a lower-ranked one kept its watch. Round two
  ([#26](https://github.com/davison/md-notes/pull/26#issuecomment-5577482079))
  blocked on the record rather than the code, and also found that a deletion did not
  hand its budget back and that `Coverage.Watched` counted directories the current
  set no longer named
  ([`e6c1d4d`](https://github.com/davison/md-notes/commit/e6c1d4d)). The last review,
  after the gate resolution was implemented
  ([#26](https://github.com/davison/md-notes/pull/26#issuecomment-5588139809)), found
  the un-ignored hidden *note* mis-ranked. Every reproduction became a test.

- **Line endings ([#29](https://github.com/davison/md-notes/pull/29#issuecomment-5589824871)):**
  the fix detected the ending by the first form present rather than the most
  frequent, so a mostly-LF note containing one CRLF — or one stray CR — would have
  been converted wholesale. The reviewer measured both cases; the rule became
  dominance by count, ties to LF, and both cases are now tests. The same review found
  the PR body claiming no deviations when there were two, and the decision citing
  user documentation that did not exist. Approved on round two
  ([#29](https://github.com/davison/md-notes/pull/29#issuecomment-5589936438)).

Two of those findings changed the shape of the milestone rather than a line of code:
the hidden-versus-ignored narrowing produced the decision gate on the nested
placeholder, and the storage-mirror finding produced the shared-slot limitation this
document records above.

Independent QA then found what four review loops had not, and it is the sharpest
lesson of the milestone: the editor rewrote every line ending in a CRLF or lone-CR
note on the first keystroke
([#17](https://github.com/davison/md-notes/issues/17#issuecomment-5589684484)). Both
sides of the seam were tested and both suites were green — `internal/source` and
`internal/server` each round-trip CRLF, and the UI suite had no carriage return in it
at all — so the defect lived exactly where neither side looked, in the composition.
QA also named the two suite gaps behind its findings, and both are now closed by the
tasks that answered them: [#29](https://github.com/davison/md-notes/pull/29) added
the first UI tests containing a carriage return, and
[#30](https://github.com/davison/md-notes/issues/30) carries the missing
clean-session deletion case.

## Known gaps at the boundary

Milestone one's two backlog captures are closed:
[#13](https://github.com/davison/md-notes/issues/13) by
[#20](https://github.com/davison/md-notes/issues/20) and
[#14](https://github.com/davison/md-notes/issues/14) by
[#21](https://github.com/davison/md-notes/issues/21). What remains true and will
surprise someone who has not read this far:

| Gap | Where it is recorded |
|-----|----------------------|
| An uncoordinated external editor can write between the daemon's final revision check and the rename | [gate resolution](https://github.com/davison/md-notes/issues/18#issuecomment-5576240416) |
| Replacement creates a new inode: hard links, ownership, ACLs and extended attributes are not preserved, and rename durability across power loss is not promised | [#18](https://github.com/davison/md-notes/issues/18#issuecomment-5576251236) |
| A revision is local to one daemon session; a restart makes every open editor's next save conflict until it re-reads | [#18](https://github.com/davison/md-notes/issues/18#issuecomment-5576251236) |
| The `localStorage` draft mirror is one record per note shared by every tab, so two tabs editing one note overwrite each other's record | [#19](https://github.com/davison/md-notes/issues/19#issuecomment-5576865725) |
| A draft over 64 KiB is not sent by the unload flush; it is recovered from storage on the next open | [#19](https://github.com/davison/md-notes/issues/19#issuecomment-5576865994) |
| The editor cannot create, rename or delete a note, and cannot write a draft back to a deleted one | [#19](https://github.com/davison/md-notes/issues/19#issuecomment-5576722702) |
| A directory whose only files are *ignored* is still not watched — milestone one's remaining live-update hole, unchanged | [#20](https://github.com/davison/md-notes/issues/20#issuecomment-5577085446) |
| A root over its budget leaves its lowest-priority directories unwatched, and a reclaimed watch opens the same gap for the directory that loses it | [#20](https://github.com/davison/md-notes/issues/20#issuecomment-5577328490) |
| A coverage change reaches the browser on the next keepalive tick, so up to thirty seconds late | [the introduction](../introduction.md#when-coverage-is-limited) |
| A note can still write `mdn-` classes and give its own text the syntax highlighter's colours — the bound of the reserved namespace, not a leak out of it | [#25](https://github.com/davison/md-notes/pull/25) |
| A note that *mixes* line endings is normalised to its dominant one on the first edit; a note using one ending throughout keeps every byte | [#28](https://github.com/davison/md-notes/issues/28#issuecomment-5589841459) |
| A *clean* editor session on a note deleted on disk enters a conflict claiming unsaved edits, and stays there until dismissed. Nothing is at risk — the "draft" is the file's own text | [#30](https://github.com/davison/md-notes/issues/30) |
| Note content can emit `class="line-anchor" data-line="N"` and so plant a decoy scroll target for a search hit's `?l=`. `line-anchor` is a note class by design, so M2-R5 is unaffected and the behaviour predates this milestone | [#31](https://github.com/davison/md-notes/issues/31) |

The last two are QA's findings 2 and 4, carried as backlog captures by the operator's
disposition
([#17](https://github.com/davison/md-notes/issues/17#issuecomment-5591886325)) rather
than fixed inside the milestone.

Milestone one's own boundary notes still stand: more than about six open tabs starve
the extra ones of live update, symlinked files inside a root are absent from the
navigator and from search while remaining servable by direct URL, and the daemon has
no authentication of its own
([the M1 record](1-daemon-and-rendered-viewer.md#known-gaps-at-the-boundary)).
[#10](https://github.com/davison/md-notes/issues/10), remote access over the tailnet,
is still waiting on the authentication path the browser extension will need.

## Where the record is silent

- **The vim and markdown libraries were never decided in the open.**
  `@replit/codemirror-vim`, `@codemirror/lang-markdown` and the rest of the
  CodeMirror packages appear in [#19](https://github.com/davison/md-notes/issues/19)'s
  plan and in `ui/package.json`, and only `@codemirror/language-data` was ever
  weighed as a choice with a cost
  ([#19](https://github.com/davison/md-notes/issues/19#issuecomment-5576865448)).
  CodeMirror 6 itself was named by the milestone
  ([#17](https://github.com/davison/md-notes/issues/17#issuecomment-5576208722)), so
  the framework is accounted for; the vim implementation under it is not. This is the
  same silence milestone one recorded about chroma, `preact-iso` and `yaml.v3`, and
  it has not been closed.
- **Nothing records why the conflict banner offers no merge.** Every conflict
  resolution is all-or-nothing — keep the draft, take the file, or copy the draft out
  and reconcile by hand. A side-by-side or three-way view was never proposed,
  rejected or deferred anywhere in the record; it simply never came up.
- **The one-second autosave delay is recorded; the 64 KiB and 8 MiB numbers are the
  platform's.** The delay has a decision behind it
  ([#19](https://github.com/davison/md-notes/issues/19#issuecomment-5576722865)); the
  `keepalive` cap is the browsers' and the 8 MiB source bound is
  [#18](https://github.com/davison/md-notes/issues/18#issuecomment-5576251236)'s,
  where it is stated but not argued.
- **The review rounds on [#26](https://github.com/davison/md-notes/pull/26) are
  misnumbered.** Its three review comments are labelled as an unnumbered first, a
  "round two" and a "round four"; nothing is labelled round three, and the PR body's
  "Review round three" section is the author's reply to the round-two review. The
  counting drifted between author and reviewer part-way through; no review appears to
  be missing from the thread.
- **Two suite judgements were accepted rather than closed.** The re-verification
  records that no UI test carries a carriage return all the way through the save path
  onto disk, and that the generation-rebuild test uses LF only, so re-detection of a
  note's ending after `Load the file` or an external rewrite is pinned by no test —
  both behaviours were verified in the browser instead, and QA recommended accepting
  them on the record rather than opening an issue
  ([#17](https://github.com/davison/md-notes/issues/17#issuecomment-5592034687)).
- **Nothing asked for a test across the daemon/UI seam.** Milestone one recorded the
  decision that tests ride each PR rather than forming a requirement of their own
  ([#1](https://github.com/davison/md-notes/issues/1#issuecomment-5572364167)), and
  this milestone inherited it without revisiting. QA's blocking finding lived exactly
  in the composition of two well-tested halves, and no requirement, plan or review
  had asked where a test spanning them would live. The record still does not say.
- **The gate's cost figure was accepted on a measurement that was wrong.** The
  operator resolved the nested-placeholder gate on "2 to 20 percent", a number
  measured at the shell rather than in the daemon
  ([#20](https://github.com/davison/md-notes/issues/20#issuecomment-5587683689)). The
  implementer found and corrected it while building
  ([#20](https://github.com/davison/md-notes/issues/20#issuecomment-5587850848)), and
  the shipped shape costs 5–7% — inside the band either way. What the record does not
  contain is any check that a decision gate's numbers were measured the way the code
  will run them, before the operator answers.
