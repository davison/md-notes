# M6 — Clipping over the tailnet, clipper conversion fixes, and the M5 backlog

Tracking issue: [#96](https://github.com/davison/md-notes/issues/96). Its five
implementation tasks are merged on `main` at
[`ed1d5f3`](https://github.com/davison/md-notes/commit/ed1d5f3).

## Goal and outcome

The milestone's goal, as stated on [#96](https://github.com/davison/md-notes/issues/96),
was to take up the backlog M5 raised rather than to add capability: admit clipping over
the tailnet now that create is admitted there
([#95](https://github.com/davison/md-notes/issues/95)); fix the two clipper conversion
defects captured in M3's reviews while the extension is in hand
([#45](https://github.com/davison/md-notes/issues/45),
[#47](https://github.com/davison/md-notes/issues/47)); map the refusal codes the create
and delete endpoints left as `500`s or wrong codes
([#82](https://github.com/davison/md-notes/issues/82),
[#88](https://github.com/davison/md-notes/issues/88)); finish the note bar and the
deleted-on-disk banner ([#91](https://github.com/davison/md-notes/issues/91),
[#92](https://github.com/davison/md-notes/issues/92)); and remove the two test-suite
sharp edges ([#87](https://github.com/davison/md-notes/issues/87),
[#89](https://github.com/davison/md-notes/issues/89)). The inbox clipper stayed later
work, as it has since M3.

So this is the first milestone whose subject is its predecessor's leavings. Of the nine
captures it adopts, six were raised by M5's own reviews and QA and two by M3's; the ninth
is a question the operator asked the day M5 closed. Nothing here is a new noun: one line
of allow-list, one converter, four refusal codes, one stylesheet rule, one button and two
test files that now own their own lifecycle.

What shipped, in the order it merged:

**Clipping works from another tailnet node.** `POST /api/clip` joined the tailnet
allow-list as a single `case` in `remoteAllowed` returning `r.Method ==
http.MethodPost`, so the extension pointed at the `tailnet_host` URL saves a clip into
the notes root's clips directory exactly as it does on loopback — byte-identical modulo
the `clipped` stamp, which is how the test measures "exactly" rather than describing it.
`POST /api/roots` is unchanged and still refused, and `GET`, `PUT` and `DELETE` on the
clip path are refused with everything else the allow-list has not considered. The clip
still requires the bearer token; the login session at the tailnet name is not a second
way in. The extension stopped naming clipping as refused and now names an *older daemon*
when it meets a `loopback_only` there.

**The clipper writes markdown where it used to leave the page's HTML.** A table with no
header row becomes a GFM table with a synthesised empty header row and every source row
kept as data; `colspan` and `rowspan` are laid out on a rectangle so the columns keep
meaning the same thing down the table; a nested table is flattened into the cell that
holds it. A table that is *not* carrying data — a page wrapped in one cell, Pygments'
line-number wrapper — is converted as ordinary markdown blocks instead, so a heading
stays a heading and a code block keeps its line breaks. Beside a fenced code block, a
caption or a filename line is kept and converted in the order the page wrote it, and
only chrome is dropped, wherever it sits inside the wrapper.

**Four wrong refusal codes, and a fifth nobody had noticed.** A basename the kernel calls
too long is answered `400 invalid_path` with a message naming the limit, on create, save
and delete. A dangling *directory* symlink out of the root and a dangling chain out of
the root answer `403 outside_root` on create and delete, however long the chain — the
walk follows as many links as the resolver itself does. And a chain nothing will follow
to the end — the kernel's `ELOOP`, `filepath.EvalSymlinks`'s own give-up, or the walk's
bound — is answered `422 unsupported_source` where each reached the error mapping
unrecognised and became a `500` logged as a server fault. Every one of these is on a path
that was already being refused: what changed is which refusal the caller is shown.

**The note bar never scrolls, and the banner puts the note back.** The save-status message
is a box of its own, clipped with an ellipsis, with the whole of it in a `title` and
**Retry** as its sibling rather than part of its text; measured against the old code with
a 400-character single word, the bar's `scrollWidth` falls from **2,832 px inside a 320 px
bar** to 320 of 320, and to 770 of 770 at a 1280 px window. The deleted-on-disk banner
names **New note** as the way back and offers **Recreate the note**, which opens the create
prompt with the lost note's path in the box and the orphaned draft as the body; one
confirmation puts the file back and the editor carries straight on over it, clean.

**The test suites own their own lifecycle, and two of their premises now trip a test.**
The vitest teardown flake M5's review met — `ReferenceError: window is not defined` from
a preact effect firing after jsdom is gone — was reproduced at 8 failures in 240
whole-suite runs, its cause measured rather than guessed, and fixed by the file
unmounting its last tree while the environment is still alive: `cleanup()` in `afterEach`
rather than `beforeEach`. Afterwards, 240 whole-suite runs and 200 runs of the file
alone, both at zero, and the reviewer's own 232 clean runs against 3 failures in 120 on
`main`. `ui/e2e/harness.mjs` gained `drawerReady`, with `dialogReady` moved in beside it
from the one `describe` block and the one hand-written copy it had lived in, and every
drawer and dialog site in the three e2e files now waits through them. And the
`stopPropagation` premise M5's record had to leave unowned — "no document-level handler
sees Escape while a dialog is open" — is asserted in Chromium against the application as
it ships: with `e.stopPropagation()` removed from `ui/src/dialog.tsx`, the suite is 38
tests and 37 passing, and the one failure is that case.

The system as it stands is described in [the introduction](../introduction.md);
[What is reachable under that name](../introduction.md#what-is-reachable-under-that-name-and-what-is-not),
[Creating and deleting notes](../introduction.md#creating-and-deleting-notes),
[Autosave and the save states](../introduction.md#autosave-and-the-save-states) and
[Conflicts](../introduction.md#conflicts) are the sections this milestone rewrote, and
[the extension page's conversion section](../extension.md#what-ends-up-in-the-note)
is the one it grew.

| Task | Requirements | Adopts | PR | Merged as |
|------|--------------|--------|----|-----------|
| [#97](https://github.com/davison/md-notes/issues/97) Admit clipping over the tailnet | M6-R1 | [#95](https://github.com/davison/md-notes/issues/95) | [#103](https://github.com/davison/md-notes/pull/103) | [`e562c9b`](https://github.com/davison/md-notes/commit/e562c9b) |
| [#98](https://github.com/davison/md-notes/issues/98) Clipper: headerless tables and captions beside code blocks | M6-R2 | [#45](https://github.com/davison/md-notes/issues/45), [#47](https://github.com/davison/md-notes/issues/47) | [#105](https://github.com/davison/md-notes/pull/105) | [`e2b5a47`](https://github.com/davison/md-notes/commit/e2b5a47) |
| [#99](https://github.com/davison/md-notes/issues/99) Refusal codes: too-long names and dangling links out of the root | M6-R3 | [#82](https://github.com/davison/md-notes/issues/82), [#88](https://github.com/davison/md-notes/issues/88) | [#104](https://github.com/davison/md-notes/pull/104) | [`915e715`](https://github.com/davison/md-notes/commit/915e715) |
| [#100](https://github.com/davison/md-notes/issues/100) UI: a note bar that never scrolls, and a banner that recreates the note | M6-R4 | [#91](https://github.com/davison/md-notes/issues/91), [#92](https://github.com/davison/md-notes/issues/92) | [#106](https://github.com/davison/md-notes/pull/106) | [`70f1986`](https://github.com/davison/md-notes/commit/70f1986) |
| [#101](https://github.com/davison/md-notes/issues/101) Test hygiene: the vitest teardown flake and two e2e harness edges | M6-R5 | [#87](https://github.com/davison/md-notes/issues/87), [#89](https://github.com/davison/md-notes/issues/89) | [#109](https://github.com/davison/md-notes/pull/109) | [`ed1d5f3`](https://github.com/davison/md-notes/commit/ed1d5f3) |
| [#102](https://github.com/davison/md-notes/issues/102) Document M6 and synthesize its record | M6-R6 | — | this one | — |

**The shape of the milestone is the review loop.** Every one of the five tasks needed a
fix pass and a re-review — four of the five reviews requested changes on the first pass
and the fifth requested them on the delta — and three of those later rounds found a real
defect **in the fix the previous round asked for**, not in the original work. #104's
first fix for a silently-abandoned symlink walk introduced a string match that put the
very defects the task exists to close back for any path whose *name* contained the words
`too many links`. #106's fix for a
caret that jumped to the top of the note introduced a silent whole-file line-ending
rewrite reachable with no conflict and no reader action. #105's fix for layout tables
being squashed into one cell took a single-column headerless table out of the
requirement's own stated default. Each was caught by the re-review that existed to check
the fix, which is the loop doing exactly what it is for — and it is also the strongest
thing this record has to say about how much of the milestone's confidence rests on that
one mechanism.

The fifth is the mirror image and worth naming as such: **PR #109's review found nothing
wrong with the code and two things wrong with the record**, and blocked on both — a count
of affected files that was off by eight, and a measurement that would not reproduce on the
reviewer's machine. Both were in comments already posted, so both were answered by
`**Correction:**` comments rather than by edits, and one of them changed the *work*: with
the correct count the reason for deferring a file evaporated, and the file was taken in the
fix pass. See [What the reviews changed](#what-the-reviews-changed).

**The scope decision was written before the work, not after it.** M5's record named its
own absence as a gap; here the coordinator posted scope, sequencing and the standing
confirmation on [#96](https://github.com/davison/md-notes/issues/96#issuecomment-5703779089)
at 20:05Z, before the first task started at 20:08Z. That is the one process finding M5
left that this milestone closed by doing rather than by writing down afterwards.

## Requirement outcomes

The verdicts below are drawn from the independent QA comment on the milestone issue
([#96](https://github.com/davison/md-notes/issues/96#issuecomment-5706600141)), run
against a clean worktree of merged `main` at
[`ed1d5f3`](https://github.com/davison/md-notes/commit/ed1d5f3) — never the operator's
checkout — in a throwaway root with its own token file, state file and port, leaving the
daemon on 7337 and `~/.local/state/mdn` untouched. The floor first, which QA is explicit
is the floor and not the evidence: `make check` green (14 Go packages, `ui` 249/249,
`extension` 205/205), `go test -race -count=5 ./internal/...` exit 0, `make e2e` 38/38
three consecutive times, `pnpm --dir extension e2e` 27/27, the whole `ui` vitest suite
60/60, and all five merge commits green on both CI jobs. QA raised two findings and four
observations, none blocking; the coordinator's disposition of them is at
[#96](https://github.com/davison/md-notes/issues/96#issuecomment-5706613081).

| ID | Requirement | Status |
|----|-------------|--------|
| M6-R1 | Clipping over the tailnet: the clip admitted and saved exactly as on loopback, `POST /api/roots` still refused `loopback_only` with its message unchanged, a handler test proving both, the extension no longer naming clipping as refused, and the introduction, the extension page and a dated annotation on the sealed M4 record saying what changed | [Satisfied](https://github.com/davison/md-notes/issues/96#issuecomment-5706600141) — QA read the allow-list diff as well as exercising it, and confirmed the milestone's gate holds by both. One documentation finding: two pages were still denying tailnet clipping on `main`, and the task's own plan had undertaken to re-check every other page at the end — naming the README among them — so the grep that would have found them was written down and not run ([#97](https://github.com/davison/md-notes/issues/97#issuecomment-5706541449)). Delivered in [#103](https://github.com/davison/md-notes/pull/103) |
| M6-R2 | Clipper conversion: a headerless table as a GFM table with a synthesised header, `colspan` and nested tables degraded to readable markdown, content beside a code block kept and only chrome dropped, six fixtures each shown failing against the current converter | [Satisfied](https://github.com/davison/md-notes/issues/96#issuecomment-5706600141) — twelve fixture shapes driven through the **real built extension** in Chromium against the built daemon, not the unit converter, and the flattened cells fed back through the daemon's own renderer. No finding. Delivered in [#105](https://github.com/davison/md-notes/pull/105) |
| M6-R3 | Refusal codes: a too-long basename `400 invalid_path` naming the limit on create and save; a dangling directory symlink and a dangling two-hop chain out of the root `403 outside_root` on create and delete; a row per case in the create and delete tables with the whole tree compared after every case; the introduction's caveat paragraph gone; the `true == false` literal gone | [Satisfied](https://github.com/davison/md-notes/issues/96#issuecomment-5706600141) — every row exercised on the exact byte and hop boundaries, with the tree inside *and* outside the root compared after every case. One observation, the 256-link create split, captured as [#111](https://github.com/davison/md-notes/issues/111). Delivered in [#104](https://github.com/davison/md-notes/pull/104) |
| M6-R4 | Note bar and banner: `scrollWidth == clientWidth` at 320 px and 1280 px under an unbreakable message of any length, the full text on hover or focus; the banner naming **New note** and offering a control that opens the create prompt pre-filled with the path and the draft; the stray `name` field gone from `browser.newContext` | [Satisfied](https://github.com/davison/md-notes/issues/96#issuecomment-5706600141) — twenty width-and-message combinations the shipped case does not use, and the recreated file compared byte for byte against the draft mirror. One finding, the vim mode after a recreate, captured as [#112](https://github.com/davison/md-notes/issues/112). Delivered in [#106](https://github.com/davison/md-notes/pull/106) |
| M6-R5 | Test hygiene: the vitest teardown flake reproduced, named and fixed at 200 consecutive runs; `drawerReady` in `ui/e2e/harness.mjs` used by every drawer case; the `stopPropagation` premise asserted or pinned with the #78 citation | [Satisfied](https://github.com/davison/md-notes/issues/96#issuecomment-5706600141) — both halves proved by mutation rather than by the green run: the flake reproduced once in 40 with the hook moved back, and the Escape probe discriminating with `stopPropagation` removed and the binary rebuilt. No finding. Delivered in [#109](https://github.com/davison/md-notes/pull/109) |
| M6-R6 | Documentation and record: user documentation reflecting every delivered change including the extension page, the introduction's tailnet and refusal sections and the sync page where it mentions clipping; the roadmap row; this record; the sealed M4 record carrying its annotation | [Untestable at the verdict](https://github.com/davison/md-notes/issues/96#issuecomment-5706600141), this task not having merged — "a superseding verdict is owed once #102 merges". The one clause already on `main`, the dated annotation on the sealed M4 record, QA found [in place and correct](4-polish-phone-e-ink-and-the-bundle.md#post-merge-annotation--2026-09-16) |

M6-R6 is the one row no verdict yet settles, and for the same reason M2-R6, M3-R7, M4-R8
and M5-R5 were not settled: QA graded it against `main` as it stood before this task, and
what it graded as missing — the documentation sweep, the roadmap row and this record — is
the list this document's pull request delivers. The closure gate on
[#96](https://github.com/davison/md-notes/issues/96) requires both that every requirement
verdict is satisfied *and* that the milestone document is merged, so this PR's merge is a
precondition of closure rather than the verdict itself.

**What QA did beyond the requirement text** is where the confidence in this milestone
actually comes from, because in four of the five requirements the shipped tests were
already green and QA went looking for the shapes they do not cover.

On the tailnet: seven path-normalisation shapes — `/api/clip/../roots`,
`/api/roots/../clip`, `/api//clip`, `/api/./clip`, `/api/clip/`, `/api/clip/%2e%2e/roots`
and `/api/%63lip` — reach root registration in none of them, with the roots listing
unchanged after all seven; and seven crafted clip *titles*, including `../../../etc/passwd`,
`/absolute/escape`, `..%2f..%2fout`, whitespace only, a script with no ASCII, and one of
500 characters, every one landing inside `clips/` at a name the daemon chose with nothing
created elsewhere in the root. A demonstrably live login session — `200` on
`GET /api/roots` with the same cookie — is refused `401` on the clip, which is the
widening [the decision](https://github.com/davison/md-notes/issues/97#issuecomment-5703912268)
explicitly declined.

On the clipper: twelve fixture shapes through the real built extension, and then the
flattened cells back through the daemon's own renderer, so the escaped pipe, the
double-backtick span and the synthesised empty header are shown *legible* and not merely
written. Seven of the twelve are QA's own, and two of them pin the recorded decisions from
the outside — `rowspan="0"` reading as one and `rowspan="99"` clamped to the table, and a
one-column headerless table of inline values staying a table, which is the amendment's own
claim.

On the refusals: the boundaries by the byte and by the hop rather than near them — 255
bytes creates `201` and 256 is `400`; chains of 17 and 255 links out of the root are `403`
on both verbs and 256 is answered by the resolver at `422`, which is exactly what the
`maxLinkHops = 255` decision predicts. Both spoofing shapes the fix in `2933ce3` exists
for — the one the first re-review's blocking finding produced — are clean: a note
literally named `too many links.md` is `403 outside_root`, and a root whose *directory
path* carries the phrase registers, reads and deletes like any
other. Nothing was created, removed or modified by any refusal, and the daemon logged no
`io_error`.

On the UI: twenty combinations the shipped case does not use — four messages (10,000
unbreakable characters, a sixty-segment path, 600 combining marks, a bidi override) across
five viewports — all with `scrollWidth` equal to `clientWidth`, the bar's height unchanged
and the delete button's box identical. The recreated file compared as strings against the
`mdn:draft:` mirror rather than by regex; one undo still reaching back across the recreate;
and the line-ending fix pinned in the browser in both directions.

On the tests: both halves proved by **mutation**. Moving `cleanup()` back into `beforeEach`
reproduced the flake once in 40 whole-suite runs under load — the same order as the 8-in-240
the task measured, which is what shows the hook and not the load is what stops it. Removing
`e.stopPropagation()` and rebuilding the binary left one e2e failure, and it is the new
case and nothing else.

QA's two findings and four observations, and what was done with each
([#96](https://github.com/davison/md-notes/issues/96#issuecomment-5706613081)):

| Finding or observation | Disposition |
|---------|-------------|
| **Finding, M6-R1.** The README's *Over the tailnet* paragraph and the introduction's **Confinement** list both still denied tailnet clipping on `main` at `ed1d5f3` — the second contradicting, eighty lines below it, the table in the same file that #103 had corrected ([#97](https://github.com/davison/md-notes/issues/97#issuecomment-5706541449)) | Not raised as a not-satisfied verdict: M6-R1's own documentation clauses are met, and both lines were already named in this task's plan and corrected on its branch by the stage-one sweep. QA filed it so that the M6-R6 verdict has something to be measured against, and **so that the record says the sweep was owned by the record task rather than by the task that made the claims false** — which is what the gaps table below records |
| **Finding, M6-R4.** A confirmed **Recreate the note** returns the reader to the editor in vim *normal* mode, where cancelling the same prompt keeps insert mode; the difference is the remount the adopted revision causes, not the dialog ([#100](https://github.com/davison/md-notes/issues/100#issuecomment-5706548457)) | Captured as [#112](https://github.com/davison/md-notes/issues/112). Not a breach: neither M6-R4 nor the plan promises the editing mode, and everything they do promise — byte identity, the caret's line and column, undo across the recreate, focus back in `.cm-content` — QA measured and found holding. It is reported *because* the task went to the trouble of keeping the caret and nothing says whether the mode was considered with it |
| **Observation 1, M6-R3.** A dangling chain past the resolver's budget answers create `409 exists` where delete, read and save all answer `422` — a second verb split, at the budget boundary, written down nowhere | Captured as [#111](https://github.com/davison/md-notes/issues/111). Confinement is unaffected; every answer is a refusal and the tree is unchanged |
| **Observation 2.** `docs/sync.md` still said clipping is "from a desktop browser only" | Folded into this task's sweep. The sentence now says that the browser need not be on the daemon's own machine |
| **Observation 3.** A `th`-labelled table flattens a code block | Already captured as [#107](https://github.com/davison/md-notes/issues/107), by the re-review that established the trade-off |
| **Observation 4.** A `title` is not keyboard-reachable either | Folded into [#108](https://github.com/davison/md-notes/issues/108), retitled to cover touch **and** keyboard. This is the half the decision on #100 believed it had covered; see [the decision below](#the-note-bars-failure-message-is-clipped-with-an-ellipsis-not-wrapped) |

## Decisions

### `POST /api/clip` joins the tailnet allow-list; `POST /api/roots` does not

The security decision of the milestone, and the one
[#60](https://github.com/davison/md-notes/issues/60#issuecomment-5656174981) declined to
take inside a requirement that said the opposite, naming it "a task of its own, with the
reasoning on the record next to #39's". That task is
[#97](https://github.com/davison/md-notes/issues/97) and the reasoning is at
[#97 (comment)](https://github.com/davison/md-notes/issues/97#issuecomment-5703912268).

What moved was not the reasoning but its ground.
[#39's allow-list decision](https://github.com/davison/md-notes/issues/39#issuecomment-5632388601)
refused two endpoints for two different reasons and said so at the time. `POST
/api/roots` was the one that mattered: with it a credential crossing the tailnet could
register any directory on the machine and read every file beneath it through the raw
endpoint. `POST /api/clip` was refused "for a different reason, and a weaker one" — not
that a clip is dangerous, but that **nothing off the machine called it**, M3-R3 having
fixed the extension's daemon URL at loopback. An endpoint reachable from off the machine
that nobody off the machine uses is surface for nothing, and #39 named the remedy in
advance: "one entry in the allow-list plus … a decision made with that use in front of
it".

M5-R2 removed the premise. It admitted `POST /api/r/{slug}/source/{path…}` under the
tailnet name on the reasoning that the three write methods on a note's source are one
resource ([#76](https://github.com/davison/md-notes/issues/76#issuecomment-5700724927)),
so since M5 the tailnet credential **can** create a file, at any path the caller names,
inside any registered root. What is left is a comparison and it runs one way: a clip
writes *one* file, into `clips_dir` inside the *notes* root, at a name the **daemon**
chooses from the date and a slug of the title, with the caller supplying no path at all.
That is strictly narrower than the create already admitted. And the use #39 said was
missing now exists, because the operator asked for it
([#95](https://github.com/davison/md-notes/issues/95)).

- **Trade-off:** the credential that crosses the tailnet can now add files to one
  directory of the notes root, where before it could only add them where it was told a
  path. In a notes tree that is a clips folder filling up, and the recovery is `DELETE`
  on the note, which the same credential already has.
- **Rejected: a configuration key to turn tailnet clipping on.** A default-off switch for
  a capability strictly inside one already on by default, at the cost of a fourth thing
  to set before the extension works from the laptop.
- **Rejected: admitting the login session as well.** `POST /api/clip` has required the
  bearer token since #39 — "nothing on the daemon's own origin needs to call it" — and
  the allow-list entry does not change that. Admitting the cookie would widen the
  *credential*, not just the endpoint, which is the one thing M6's gate forbids. A
  browser logged in at the tailnet name still cannot clip, and `TestTailnetAdmitsAClip`
  pins the `401`.
- **Rejected: deleting the extension's `loopback_only` message for clips.** A daemon
  older than the extension talking to it is a real state — the extension updates from the
  store, the daemon from `make install` — so the message stays and now names the daemon's
  age instead of asserting a rule that has stopped being one. It still never blames the
  token, which is the misdiagnosis [#51](https://github.com/davison/md-notes/issues/51)
  ended.

The reviewer read the premise as well as the diff, and recorded what bounds the widening:
`clip.Write` composes the name from the date and `Slug(title)`, which emits only `[a-z0-9]`
and single hyphens bounded to 64 — no separator a traversal could use — into `clipsDir`,
which is operator configuration and confined by `roots.ErrOutside`
([#103](https://github.com/davison/md-notes/pull/103#issuecomment-5704540805)).

### A headerless table gets a synthesised empty header row, and keeps every row as data

M6-R2 fixed this as the default and invited an overrule with reasoning; #98 adopted it
([#98 (comment)](https://github.com/davison/md-notes/issues/98#issuecomment-5703949650)).
[#45](https://github.com/davison/md-notes/issues/45) had left the choice open between
promoting the first row, synthesising an empty header, and converting the rows to a list.

- **Trade-off:** an empty header row is visible in an editor and renders as a thin empty
  band in the app. That is the price of GFM's rule that a table has a header; the
  alternative prices are a misrepresented row or a lost grid.
- **Rejected:** promoting the first row (asserts a header the page did not write); a list
  form (loses the columns).

### A spanning table stays a table, and a nested one is flattened into its cell

"Readable markdown" for `colspan` means the cells are laid out on a rectangle the way HTML
defines one: each cell placed at the first free column of its row, claiming the rectangle
its `colspan` and `rowspan` cover, the covered cells written empty, every row padded to
the widest. `rowspan` is handled the same way even though M6-R2 names only `colspan`,
because the two arrive together and a grid that understands one of them is not a grid.

"Readable markdown" for a nested table means the inner table is flattened into the cell
that holds it, its cells joined with ` / ` and its rows with `; `, because a GFM cell is
inline content only and a grid inside one is not available at any price.

- **Trade-off:** the ` / ` and `; ` separators are a convention a reader meets once. They
  are not ambiguous with cell text the way a comma would be, and the alternative — a
  `<br>`-joined cell — is HTML in the note again.
- **Rejected:** repeating a spanning value across the columns it covers (states a
  duplicate the page did not write); emitting the spanning row as prose between two
  tables (splits one table into three blocks); lifting the inner table out to stand after
  the outer one (separates it from the row it describes); leaving either as HTML (the
  defect being fixed).

### A table is written as a grid only when it is carrying data

Taken in answer to the first blocking finding on
[PR #105](https://github.com/davison/md-notes/pull/105#issuecomment-5704592150) and
recorded at [#98 (comment)](https://github.com/davison/md-notes/issues/98#issuecomment-5704683965).
The rule the decisions above took for *data* tables had been applied to every `<table>`,
which destroyed two shapes the previous behaviour had not: a page laid out in one cell
became one GFM cell holding a heading, two paragraphs and a flattened inner table, and
Pygments' line-number wrapper — the single most common code-block shape on documentation
sites — lost the code's line breaks irrecoverably.

The test, in order: `role="presentation"` or `role="none"` settles it as layout; a `th`
cell anywhere settles it the other way whatever else the cells hold; otherwise a table no
row of which has two or more cells is a wrapper; otherwise a cell holding a `pre` or an
`h1`–`h6` makes it layout. Anything that fails the test is converted as **ordinary
markdown blocks**, in the order the page wrote them — never as the page's own HTML.

- **Trade-off:** a genuine data table — one with `th` cells — that holds a code block in
  a cell still has that code flattened onto one line. Rule 2 is deliberate: a header row
  is the strongest signal a page gives. The residual is captured as
  [#107](https://github.com/davison/md-notes/issues/107). With a line-number wrapper the
  numbers now arrive as a small code block of their own above the code, which is noise,
  but nothing is lost and the code is intact. Both are in
  [the extension page](../extension.md#what-ends-up-in-the-note).
- **Rejected:** falling back to `keep`-as-HTML for non-data tables, which would answer the
  finding but re-open #45 for them; declining the grid whenever any cell holds block
  content of any kind, including two paragraphs, which would take a great many ordinary
  docs tables out of GFM; special-casing `table.highlighttable` by class, which is
  narrower than the defect — the same shape ships under other class names.

**Amended once.** Rule 3 took a *single-column headerless* table out of the grid, which is
the one place M6-R2's own stated default stopped holding; the re-review noticed the
consequence was unrecorded
([#105](https://github.com/davison/md-notes/pull/105#issuecomment-5704840025)). The
default was **restored rather than the exception recorded**
([#98 (comment)](https://github.com/davison/md-notes/issues/98#issuecomment-5704904320)):
a one-column table is now judged by what its cells hold — inline values keep it a grid,
a cell holding block content of any kind makes it the wrapper the rule was written for.
**Rejected:** recording the exception, which would have left the requirement's clearest
sentence untrue for a shape with no good reason to be special.

### Chrome beside a code block is a closed list, and everything else is content

A `<button>`, a `<clipboard-copy>` element, anything with `role="button"`, anything with
`aria-hidden="true"`, and anything whose class contains `clipboard` anywhere or `copy` as
a word. Everything else beside the `pre` is converted around the fence, in the order the
page wrote it.

- **Trade-off:** the class test is a judgement either way — too wide and a caption is
  lost, too narrow and a stray "Copy" lands in the note. `clipboard` matches loosely
  because GitHub's own container is `zeroclipboard-container`; `copy` has to be a word of
  its own so that only a class naming the control matches.

Two smaller choices recorded with them, because they change what a clip contains: a
`<caption>` is emitted as a paragraph above the table, which building the table from its
geometry would otherwise have dropped silently — the same defect shape as #47; and the
table's rows are **walked** rather than read from `HTMLTableElement.rows`, which in the
DOM the unit tests run under is a descendant search that hands a table its nested table's
rows.

### A too-long name is the kernel's refusal translated, not a length check of the daemon's own

`NAME_MAX` belongs to the filesystem — 255 bytes per component on every filesystem this
daemon is run on, but not a number the daemon is entitled to assert. A check written in
`internal/source` would either duplicate it wrongly or refuse a name some filesystem
would take, and would have to be kept in step with a limit it does not own. So create,
save and delete go on handing the name to the kernel, and one
`errors.Is(err, syscall.ENAMETOOLONG)` arm in the source error mapping turns its refusal
into `400 invalid_path`
([#99 (comment)](https://github.com/davison/md-notes/issues/99#issuecomment-5703933261)).

- **Trade-off:** the message names a limit the daemon has not verified for the filesystem
  in front of it. On a filesystem with a smaller `NAME_MAX` the number would be too
  generous while the refusal itself stayed correct. Worth it: the caller is told what to
  do about it, which "could not read or save note" never did.
- **Rejected:** capping the name box in the create prompt, which
  [#88](https://github.com/davison/md-notes/issues/88) offers as an option. The UI was not
  in that task's scope, and a cap in the box would hide the refusal rather than fix it —
  the API is reachable without the box.

### A dangling link chain is followed as far as the resolver itself follows one — 255 hops

The first form of this decision bounded the lexical walk at 16 hops and gave up in
silence past it, so a longer chain fell back to the wrong codes #82 reports. The review
of [PR #104](https://github.com/davison/md-notes/pull/104#issuecomment-5704545261) found
it and put the unsilenced window at 17–40, on the ground that the kernel resolves 40
links and refuses the 41st.

**That premise was true of the kernel and not of the path that decides this**, and the
correction is the more useful half of the exchange. `Root.Resolve` resolves through
`filepath.EvalSymlinks`, which walks a chain hop by hop *in user space* and never asks the
kernel for more than one link at a time, so a dangling chain never meets `ELOOP` at all.
Measured, and then verified independently by the reviewer rather than taken
([#104](https://github.com/davison/md-notes/pull/104#issuecomment-5704839649)):

| chain of dangling links | `filepath.EvalSymlinks` | `os.Stat` (one kernel lookup) |
| --- | --- | --- |
| 40 | `ENOENT` | `ENOENT` |
| 41 | `ENOENT` | `ELOOP` |
| 255 | `ENOENT` | `ELOOP` |
| 256 | `EvalSymlinks: too many links` | `ELOOP` |

`ENOENT` is what sends a caller to the lexical walk, so the silent window was **17–255**,
and a bound of 40 would have left 41–255 exactly as silent as 17–40 was. `maxLinkHops` is
therefore 255, the resolver's own budget, which makes the walk and the resolver agree by
construction: every chain that can arrive at the walk as a missing path is answered by it,
and a longer one is refused by the resolver before the walk is consulted. The bound still
ends a circular chain, which is the other reason it exists
([#99 (comment)](https://github.com/davison/md-notes/issues/99#issuecomment-5704685999)).

### A chain nothing will follow to the end is `422 unsupported_source`

Three different things mean "no one can follow this chain", and each reached the source
error mapping unrecognised and became `500 io_error` logged as a server fault: the
kernel's `ELOOP`; `filepath.EvalSymlinks` giving up on its own budget of 255 links; and
the new lexical walk exceeding `maxLinkHops`. All three are now one arm and one answer.
It is the code delete already gives a link it cannot act on, and it is true of every verb
— the name is held by something none of them can resolve, open, read or replace. No
answer that was not a `500` changes, so the amended decision table on
[#76](https://github.com/davison/md-notes/issues/76#issuecomment-5701086842) is untouched.

- **Trade-off:** one of the three is matched by the *text* of a stdlib error, because
  `EvalSymlinks` returns its give-up as a bare `errors.New` with no errno and no exported
  sentinel. If a Go release changes that message the case falls back to the `500` it was,
  which is why `TestSaveSourceUnfollowableChain` pins it — the test is the canary, and its
  comment says so.

**And the earlier decision was narrowed.** The first decision on #99 said "`Resolve` is
left alone; only the create and delete paths learn the new diagnosis", which was true when
it was written. Naming `EvalSymlinks`'s give-up put one branch inside `Resolve`, so `GET`
and `PUT` on a chain nothing will follow now answer `422` where they answered `500`. What
the earlier decision was protecting is intact — the *confinement* answer on the read and
save paths is untouched, and a dangling escaping link still answers them `404 not_found`
rather than `403 outside_root`. Both comments stand; the narrowing is recorded on the
later one rather than by editing the earlier.

### The read and save paths keep their `404`, so the four verbs disagree on one shape

The chain walk could have gone into `Root.Resolve`, which would have made `GET` and `PUT`
answer `403 outside_root` for a dangling escaping link as well. M6-R3 asks for the code on
create and delete, the amended decision table on #76 is the create/delete table, and
changing the read and save paths would be a behaviour change no requirement asks for.
`OpenDir` and `EnsureDir` consult the new diagnosis only *after* `Resolve` has already
refused with `os.ErrNotExist`, so nothing that resolves today resolves differently.

- **Trade-off:** the four verbs no longer agree on this one shape — create and delete say
  `403`, read and save say `404`. Recorded so the next task that touches the read path
  takes it up deliberately rather than discovers it, and — at the reviewer's request —
  written into [the introduction](../introduction.md#creating-and-deleting-notes) as well,
  where a reader is, rather than into a GitHub comment alone.

### The note bar's failure message is clipped with an ellipsis, not wrapped

M6-R4 allows either, and the two are not interchangeable here. Wrapping
(`overflow-wrap: anywhere`, which unlike `break-word` does shrink a flex item's
min-content width) also gets `scrollWidth === clientWidth`, but it grows the bar to two or
three lines under a long message — and the same requirement asks that the delete button
keep the fixed place #85 gave it, *measured*. A bar that changes height moves that button.
One line, clipped, is the only shape that satisfies both halves
([#100 (comment)](https://github.com/davison/md-notes/issues/100#issuecomment-5704672008)).

- **Trade-off:** a `title` is a pointer affordance. It is what the requirement names
  first, and the message is a *reason*, not a control — **Retry** is the control, and it
  is deliberately a sibling of the clipped box rather than part of its text, so it is
  never the thing that gets cut off and stays reachable by keyboard and by tap at every
  width. The failure is also readable in full in the daemon's log, and the DOM text is the
  complete message, so a screen reader reads all of it. What the decision weighs is the
  keyboard reader — and **it turns out not to have served that reader either**. The
  reviewer found the touch reader has no route to the `title` at 320 px, where 34 px of
  message is on screen; QA then found a keyboard-only reader has none, a `title` not being
  focusable. So the one affordance the trade-off rests on reaches neither, and the control
  the decision correctly kept reachable, **Retry**, is not the text. Captured as
  [#108](https://github.com/davison/md-notes/issues/108), retitled for both.
- **Rejected:** putting `overflow: hidden` on `.save-status` itself, which holds the Retry
  button as well as the message and would clip the button at exactly the widths where the
  message is long enough to matter; expanding the message in place on focus, which is the
  wrapping case again, with the bar's height changing under the reader's hands.

### **Recreate the note** stands beside **Copy draft**, and a retaken path is refused

The new control is the one-step way back into the same note: same path, same text, the
conflict over and the editor carrying on. **Copy draft** is kept unchanged because it
answers a different question — a draft that is going somewhere else entirely — and is the
escape hatch for the case below. Neither replaces the other
([#100 (comment)](https://github.com/davison/md-notes/issues/100#issuecomment-5704672175)).

A path that exists again is nothing special: the prompt sends the same `POST` as any other
new note and the daemon answers `409 exists`, shown beside the name it still holds, for
correcting. Deliberately **not** a create-or-overwrite: the file under that name is one
this tab knows nothing about — it may be what a sync just brought in — and overwriting it
silently to resolve a conflict about *not* losing text would be the same mistake in the
other direction.

### The session adopts the recreated file only when the draft matches byte for byte

`noteRecreated` resolves a *deleted* conflict to clean on the new revision. An ordinary
empty **New note** typed at a path some tab still holds an orphaned draft for does not
match, and is left to the change stream, which turns that tab's banner into the
changed-on-disk one with **Keep my draft** / **Load the file** / **Copy draft**. The draft
is what the banner exists to protect; a create that happens to collide with it must not
quietly stand in for the reader's answer. The reviewer exercised the guard in both
directions in a real browser, and confirmed that a draft edited between opening the prompt
and confirming fails the guard and falls through to the changed-on-disk banner rather than
to a silent overwrite ([#106](https://github.com/davison/md-notes/pull/106#issuecomment-5704943849)).

### A note erased whole and then retyped comes back LF, whatever ending it had

Recorded **after the merge**, from the final re-review's non-blocking finding
([#106](https://github.com/davison/md-notes/pull/106#issuecomment-5705241693)), so that
this record can cite it rather than leaving the next reader of `lineEnding("")` to find it
([#100 (comment)](https://github.com/davison/md-notes/issues/100#issuecomment-5705261754)).

The blocking fix that produced it stopped capturing a note's line ending when the editor
state is built and reads it from the session's draft as each edit is written back.
`lineEnding("")` is `"\n"`, so when the draft is momentarily empty the ending is read from
nothing. Only a *separate* delete and then type reaches it; selecting all and typing over
is one dispatch, and at that moment the draft still carries the note's endings.

[The introduction](../introduction.md#editing) promises exactly this much: a note that uses
one ending throughout keeps every byte through an edit. A note the reader has emptied has
no ending to keep. The previous answer was not a chosen rule either — it was the ending of
the text as it stood when the editor state was last built, which the reader cannot see and
which a remount could change under them. Neither is promised; the second is the one that
can be explained.

- **Trade-off:** a reader who empties a CRLF note and retypes it gets an LF file, and on a
  root under Syncthing that is a whole-file diff to every other device — the same shape as
  the defect the fix was for. The difference, and why it is taken: it follows a deliberate
  act by that reader on that note, rather than another device's rewrite landing behind
  their back with no conflict, no banner and nothing to decide.
- **Rejected:** keeping the built-in ending as a fallback for an empty draft — that is the
  second copy of the note's state whose going stale was the blocking finding, restored
  under a narrower name; falling back to the ending of `state.base`, which resurrects an
  ending from text the reader has just deleted and adds a third rule to explain.
- **Not pinned by a test, deliberately.** Pinning it would make a rule out of what is
  incidental to `lineEnding("")`. If a later task decides the empty-draft case should have
  an answer of its own, that decision — and a case for it — is where it belongs.

### The teardown flake is an effect flush that outlives the file, fixed in the file

Reproduced before anything was changed, which is what M6-R5 asks for and what makes the
cause a measurement rather than a guess: the file alone passed 200 runs, and the whole
`ui` suite failed 8 times in 240 (3.3%), every one of them the error M5's reviewer met, to
the character ([#101 (comment)](https://github.com/davison/md-notes/issues/101#issuecomment-5705629120)).

`@testing-library/preact` installs its own `afterEach(cleanup)` only when vitest injects
the test globals, and this project does not set `test.globals`, so the unmount is each
file's to make — and this file made it in `beforeEach`, which unmounts the *previous*
case's tree and leaves the last one mounted for the file's life. What is still queued
against it is a `useEffect` flush: the fetch a `NoteView` starts from its mount effect
resolves after the `act()` that rendered it returned, so its commit schedules through
preact's own `requestAnimationFrame`-raced-with-100 ms path rather than into act's
collector, and `act()` flushes only what its collector received. A probe on
`options.requestAnimationFrame` counted **one flush still queued when the last case
ends**. Normally it lands a frame later and does nothing; under eighteen jsdom
environments it lands after vitest has torn the environment down.

Moving `cleanup()` to `afterEach` makes that flush *harmless* rather than merely rarer —
`flushAfterPaintEffects` skips a component whose `_parentDom` is null, and unmounting
nulls it. The same hook restores `Element.prototype.scrollIntoView`, which two cases wrote
over and left. After: 240 whole-suite runs at zero, and the reviewer reproduced both arms
independently (3 failures in 120 on `main`, 232 clean runs across the two branch heads).

- **Rejected:** fake timers — the 1.5 s flash timer is cleared by its own effect cleanup
  and was never the leak, and the stack names the effect body, not the timer; reading
  `window` defensively in `NoteView`, which would hide a late effect rather than stop one,
  and would be a product change in a test-hygiene task; `--retry` on the vitest run, which
  is "a flake with the evidence deleted".

### `dialogReady` moves into the harness rather than `drawerReady` being written beside it

#89 and M6-R5 both say "a `drawerReady` helper beside `dialogReady`", which reads as though
`dialogReady` were already in the harness. It was not: it was a `const` inside one
`describe` in `create-delete.test.mjs`, and `display.test.mjs` carried the same wait
written out again with a nine-line comment. Adding only the new helper would have left the
pair across three files, which is the arrangement that produced the duplicate
([#101 (comment)](https://github.com/davison/md-notes/issues/101#issuecomment-5705889165)).

- **Small deviation, declared:** `drawerReady` asks for one thing the old inline waits did
  not — that `.panes` carries the `open` class as well as containing the active element.
  The panes element exists at every width, so "contains the active element" alone is also
  true of a wide window's navigator pane, and a caller asking `drawerReady` is asking about
  the drawer. Nothing any case asserts changed.
- **Noted with it:** two sites moved from Playwright's `waitForFunction` (30 s default) to
  the harness `waitFor` (20 s) — less headroom on a slow machine, for one wait with one
  explanation.

### The `stopPropagation` premise gets the browser check, and the comment as well

M6-R5 offered either. The check is the stronger, and M5's reason for retiring the old one
does not block it. That decision
([#78](https://github.com/davison/md-notes/issues/78#issuecomment-5702378000)) refused to
build a stacking a reader cannot reach, and all three of its observations still hold — but
the premise #89 asks to own is not "some layer's handler is outranked", it is "**no
document-level handler sees Escape while a dialog is open**", and the application already
registers a document-level capture-phase `keydown` listener that is live under the prompt:
the note pane's Ctrl+E toggle. It is not an Escape handler, but it is invoked for Escape
exactly when propagation reaches the document, which is the question
([#101 (comment)](https://github.com/davison/md-notes/issues/101#issuecomment-5705889343)).

So an init script wraps every `keydown` listener the page registers on `window` or
`document`, delegating and recording which target's listener ran: `["document"]` with no
dialog open — the control, without which the assertion would pass against a probe that
sees nothing — and `["window"]` with the prompt up. With `e.stopPropagation()` removed from
`ui/src/dialog.tsx` and the binary rebuilt, the suite is 38 tests and 37 passing, and the
sole failure is this case on `["window", "document"]`. The rule is now held in both
environments — jsdom against a listener the test registers, Chromium against one the
application registers — which is what #89 means by giving the standing condition an owner.

- **Trade-off:** the check observes listener *invocation*, through a wrapper the page would
  not otherwise have. It is one step away from pure black box — the price of asking a
  question about propagation rather than about pixels.
- **Rejected:** the comment-only option, which pins the reasoning but trips nothing; and
  forcing a real stacking, which #78 already rejected and which nothing here needs.
- **Taken with it, because it had become false:** the long comment in
  `create-delete.test.mjs` saying that commenting out `e.stopPropagation()` "fails nothing
  in this suite".

### Scope and sequencing, taken at the start

The coordinator's decision of 2026-09-16
([#96 (comment)](https://github.com/davison/md-notes/issues/96#issuecomment-5703779089))
grouped #95 and the six M5 captures by the code they touch into five tasks plus this
record, and rolled #45 and #47 in on an assessment rather than on the operator's premise:
#95 touches the extension's popup, options and e2e suite but *not* the conversion pipeline
in `extension/src/markdown.ts`, so "clipper code is being touched" was true of the package
and not of the converter. They were rolled in anyway because both are small,
extension-only, fixture-driven fixes from M3's reviews, the extension build and test
harness are exercised by the tailnet task regardless, and #45's one open question is fixed
as a default by M6-R2 — one task's cost against another milestone's visit to the same
package.

- **Not rolled in:** [#50](https://github.com/davison/md-notes/issues/50) (the file-URL
  intercept registering a root for a missing file), which is extension code but a
  behaviour change with its own decision to take; and #30, #31, #33, #48, unrelated to
  this milestone's subjects.
- **Rejected:** a milestone of #95 alone, which would leave six small captures to accrete;
  and folding the documentation into each task, which M5 showed leaves pages outside any
  task's file list.
- **Sequencing:** the five implementation tasks are independent and ran in parallel, pairs
  at a time to keep review loops tractable, with the two UI tasks rebasing in order
  because both edit `ui/e2e`. QA and this record last.

## Deviations

### #97's plan promised a session-cookie clip test; the decision rejected it

The plan for [#97](https://github.com/davison/md-notes/issues/97) said
`TestTailnetAdmitsAClip` would prove `201` "with the bearer token **and** with the session
cookie". The decision written before the code rejected that and the test pins `401`
instead. The reviewer recorded it plainly as "a deviation from a written plan, taken
deliberately, with the reasoning recorded on the task before the code landed — which is
the process working, not a finding"
([#103](https://github.com/davison/md-notes/pull/103#issuecomment-5704540805)).

### #99 changed a third endpoint's answer, and the plan scoped it to two

`EnsureDir` has one other caller, `internal/clip`. With a clips directory behind a dangling
symlink chain out of the notes root, `POST /api/clip` now answers `403 outside_root` where
it used to reach `MkdirAll` and an unmapped error. Strictly better, and it cannot widen
anything — the clip was refused before and is refused now — but the plan, the commit
messages and the PR body all scoped the change to create and delete. Found by the review
([#104](https://github.com/davison/md-notes/pull/104#issuecomment-5704545261), finding 2),
named in the PR body, and now in
[the introduction's clip refusal table](../introduction.md#clipping-a-web-page).

### #99's two test-coverage deviations, and a PR body that said there were none

The plan said `TestSourceErrorResponses` would "gain the too-long name on the save path"; a
separate `TestSaveSourceNameTooLong` was added instead, because that test's table is built
around mutating `hello.md` and the too-long case needs no fixture. The plan said
`internal/source` and `internal/roots` would both get unit coverage for the new walk; only
`internal/roots` did, with `internal/source`'s share carried by `internal/server`'s refusal
tables. Both are immaterial to what is covered — but the PR body said there were no
deviations, which the review corrected
([#104](https://github.com/davison/md-notes/pull/104#issuecomment-5704545261), finding 6).

### Two documentation corrections rode along in #97's docs commit

The clip section called `POST /api/clip` "the only endpoint that creates a file", which
M5-R2 had made false, and the extension page's connection-test example had backticks
nested inside a code span. Both were in pages the task was already rewriting, both were
declared in the PR body rather than left to be found, and the reviewer judged both as
belonging ([#103](https://github.com/davison/md-notes/pull/103#issuecomment-5704540805)).

## What the reviews changed

Every task was reviewed by an independent model session under the reviewer contract, in a
clean worktree, and every review ran the suites and drove the built artefacts rather than
reading them. Four of the five requested changes; the fifth approved and then requested
changes on the delta. **Every round after the first found a defect in the fix the previous
round had asked for** — except #109's, whose two blocking findings were both defects in
the *record* of work whose code the reviewer could not fault.

| PR | Verdict | What the review changed |
|----|---------|-------------------------|
| [#103](https://github.com/davison/md-notes/pull/103) | request changes, then approve | One blocking: the **"Not yet" box was half-deleted**, leaving two lines quoting the very decision the PR supersedes hanging off the end of a paragraph, on a page M6-R1 names explicitly — so the extension page told a reader, four paragraphs after telling them clipping works, that tailnet clipping is a future task nobody has decided. Three non-blocking: two test doc comments still asserting clipping is refused (the plan had grepped for *rows*, and comments are not rows), and a test computing the expected filename from its own clock, which could straddle midnight — fixed by taking the path from the daemon's own answer through one `clipPath` helper, which removed code rather than adding it. The reviewer exercised a scratch daemon under a made-up tailnet name across sixteen request shapes, including `POST /api/roots/../clip` and `POST /api/clip/../roots`, to confirm the guard's cleaner and the mux's agree |
| [#105](https://github.com/davison/md-notes/pull/105) | request changes, then approve with three items, then one more commit | Two blocking. **Presentational tables were being squashed into a cell** — a manual laid out in one `<td>` became a single GFM cell holding a heading, two paragraphs and a flattened data table, and Pygments' line-number wrapper lost the code's line breaks irrecoverably, both reproduced through the built bundle in Chromium. And **an overrunning `rowspan` invented empty rows** — `<td rowspan="5">` on a two-row table produced five, with the `100` ceiling in `span()` making 99 junk rows reachable from one attribute. Six non-blocking, five taken: chrome one level down (a code header bar leaked `main.goCopy`), `rowsOf` section ordering (which carried a real new bug — a `thead` written after the `tbody` read as headerless and its header row written out as data), a zero-cell row producing a delimiter with no dashes that goldmark renders as a paragraph of pipes, alignment lost under a synthesised header, and a missing `role="button"` in the docs. The re-review of the fix then found that the new data-table rule had taken a single-column headerless table out of M6-R2's own default, and that a comment blaming a DOM quirk described a cause neither DOM has — both closed in `c3facc4` |
| [#104](https://github.com/davison/md-notes/pull/104) | request changes, request changes, then approve | Round one blocking: **the refusal table's new row was false for a chain longer than 16 hops**, and the caveat paragraph that used to name such exceptions had been removed in the same commit titled "the refusal table is the whole account again" — a stated exception traded for an unstated one. Round two blocking, and the sharpest finding of the milestone: the fix's `strings.Contains(err.Error(), "too many links")` **fired on ordinary paths**, because every other error `EvalSymlinks` returns is an `*fs.PathError` that quotes the path — so a note or folder a reader named `too many links` answered `422` for every absent path, and, because `ErrTooManyLinks` is neither `os.ErrNotExist` nor `ErrOutside`, the new walk was skipped entirely and **#82's four wrong codes came back** on the exact shapes the task closes. With the phrase in the *root's* path, every absent path in that root answered `422` for every client. No test named such a path; ten rows now do. Five non-blocking, four taken |
| [#109](https://github.com/davison/md-notes/pull/109) | request changes, then approve | **The only review in the milestone whose blocking findings were both about the record and neither about the code** — "no change to any shipped file is needed", in the reviewer's words, and the verdict was still request changes. One: a count of nine affected test files that was one, with the census to prove it, which mattered because the *reason given for deferring the work* rested on the wrong number. Two: a "teeth" measurement of 6 failures in 6 runs that gave 0 in 12 on the reviewer's machine, with a reading of the call site explaining why it cannot fail there. Five non-blocking, three of them on the Escape probe's listener wrapper — `this`, per-phase bookkeeping, and the unwrapped `handleEvent` form — all taken. The reviewer reproduced the flake in both arms (3 failures in 120 on `main`, 232 clean runs on the branch), read `@testing-library/preact`'s auto-cleanup gate and preact's `_parentDom` check at source rather than taking the narrative, and rebuilt the binary with `stopPropagation` removed to confirm the new case is the only one in `ui/e2e` that fails |
| [#106](https://github.com/davison/md-notes/pull/106) | approve, then request changes on the delta, then approve | The first pass approved with four non-blocking, of which the useful one was that **the caret jumped to the top of the note after a successful recreate** — `recreated()` bumps `generation`, and the editor rebuilds its state on a generation change, which throws the selection and the scroll away for a draft the guard has just proved unchanged. The fix parked the editor state, which survived selection, scroll *and* undo — and introduced the blocking finding on the delta: **the parked state carried a line ending captured when it was built**, so a note whose file changed on disk in its endings only was silently rewritten whole on the next keystroke, with no conflict, no banner and no reader action, in an application whose notes root is expected to be under Syncthing. Fixed by removing the second copy rather than keeping two in step. The final re-review's one non-blocking finding is the empty-draft line-ending case, recorded as a decision after the merge |

Each PR carries the operator's confirmation as a comment: "reviewed and accepted by
@davison as both author and operator (pure solo tier, SPEC §6) — no independent principal
exists in this project". Every role in this project is routed to the operator, so the
implementer, the reviewer and the operator are one person's identity and three
clean-context model sessions, and the milestone's gate on
[#96](https://github.com/davison/md-notes/issues/96) makes that standing confirmation
explicit: an approved PR merges without a per-PR wait.

## Corrections to the record itself

Five times a claim on this milestone's record was wrong about the code, about a
measurement, or about the page it named. Four were corrected by a new comment rather than
by editing the one they correct, so the claim and its answer both stay legible; the fifth
is corrected here. **Two of the five were found by a review that found nothing wrong with
the code**, and one of them changed what the task did.

- **The `CELL_BLOCK` comment blamed a DOM quirk that does not exist.** It said the
  node-name walk was needed because the DOM the tests run under answers a `querySelector`
  *selector list* with the first element it finds whatever its name is. The re-review
  could not reproduce that, in either DOM, by any access path
  ([#105](https://github.com/davison/md-notes/pull/105#issuecomment-5704840025)). The
  failure seen during development was real and its cause is now pinned at the source:
  `@mixmark-io/domino` 2.2.0 implements `querySelector` as `select(selector, this)[0]`, so
  a miss returns `undefined`, never `null`, and the `!== null` the first draft used was
  true for every cell. A `!= null` comparison would have been correct; the node-name walk
  is kept because it sidesteps the comparison altogether and both DOMs agree on it
  ([#98 (comment)](https://github.com/davison/md-notes/issues/98#issuecomment-5704904320)).
  An earlier probe had written `?? null`, which converted the `undefined` and hid it —
  which is how the selector list came to be blamed.
- **The reviewer's own number was wrong, and the implementer corrected it with a
  measurement.** The first review of #104 put the silent window at 17–40 hops on the
  kernel's `ELOOP` boundary; the real window was 17–255, because a dangling chain is
  resolved in user space and never reaches the kernel's limit. The reviewer re-ran the
  probe independently rather than accepting the correction, and recorded that a bound of
  40 "would have fixed a third of what I asked for"
  ([#104](https://github.com/davison/md-notes/pull/104#issuecomment-5704839649)).

- **"Both are in `docs/extension.md`" was half true.** The data-table decision and the
  response that carried it both said the rule's two costs — a `th`-labelled table
  flattening a code block, and a line-number wrapper leaving its numbers as a small code
  block of their own — were stated on the extension page
  ([#98](https://github.com/davison/md-notes/issues/98#issuecomment-5704683965),
  [#105](https://github.com/davison/md-notes/pull/105#issuecomment-5704728581)). The page
  stated the second and left the first to be deduced from "a `th` cell settles it". This
  task added the missing clause and cited
  [#107](https://github.com/davison/md-notes/issues/107) beside it; the comments are not
  edited.

- **"Nine other `ui/src` test files share this shape" — the number was one.** The
  teardown decision deferred a sweep on the ground that it would be "an unreviewed
  ten-file diff inside a test-hygiene PR". The review of PR #109 counted the thirteen
  `ui/src/*.test.tsx` files against the *hook* each `cleanup()` call sits in, rather than
  against the call sites a grep returns, and found exactly one other file with the shape —
  `navigator.test.tsx`, which was also the only file in `ui/src` with no `afterEach` hook
  at all ([#109](https://github.com/davison/md-notes/pull/109#issuecomment-5706208251),
  finding 1). The correction does not re-justify the deferral on other grounds: with the
  number right, the stated reason does not survive, so **the deferral is withdrawn and the
  file is fixed in the same PR**, and it says that no capture should be opened from the old
  figure ([#101 (comment)](https://github.com/davison/md-notes/issues/101#issuecomment-5706269532)).
  It also records how the wrong number was written, which is the half that stops it
  recurring. The substantive claim beside it survived the recount and the reviewer checked
  it independently: `note-view` was still the only file that *could* fail, `Navigator`
  having no fetch and no effect that reaches a global.
- **A measurement that does not travel is not evidence.** The `drawerReady` decision led
  with 6 failures in 6 runs of the mutated harness under stated load; the reviewer got **0
  in 12** on their machine and explained the mechanism — in `open()` the click is followed
  by a round-trip and six assertions before any key is sent, which is far more than the
  frame the effect needs
  ([#109](https://github.com/davison/md-notes/pull/109#issuecomment-5706208251), finding 2).
  The re-measurement, interleaved A/B, reproduced 6/6 locally — and the figure was retired
  anyway, on the principle that a number another machine cannot get is not evidence
  ([#101 (comment)](https://github.com/davison/md-notes/issues/101#issuecomment-5706271793)).
  What replaces it is the reviewer's instrumentation, which does travel: counting the polls
  each `drawerReady` call needs on a quiet machine, all five calls in `layout.test.mjs`
  need a second 50 ms poll, on three consecutive runs, on both machines. The correction also
  volunteers a second error the finding had not caught — the failures were not "every one
  of them" the `closed()` timeout; a third of them were a focus assertion elsewhere.

**A process incident, recorded on the PR at the time.** PR #104's description was briefly
overwritten with another task's PR text at 21:23Z and restored a few minutes later: a
sibling session writing to the same scratchpad path replaced the file passed to
`--body-file`. Nothing merged in that window and `closingIssuesReferences` was `[99]`
before and after; the session moved its `gh` input files under a path of its own
([#104](https://github.com/davison/md-notes/pull/104#issuecomment-5704743621)). Running
five tasks in parallel is what makes this possible, and it is the first time the
parallelism has cost anything.

## Captures adopted, and captures raised

Adopted and closed by this milestone:

| Capture | Adopted by | What it was |
|---------|-----------|-------------|
| [#95](https://github.com/davison/md-notes/issues/95) | [#97](https://github.com/davison/md-notes/issues/97) | Admit `POST /api/clip` over the tailnet now that create is admitted there — the operator's question the day M5 closed |
| [#45](https://github.com/davison/md-notes/issues/45) | [#98](https://github.com/davison/md-notes/issues/98) | The clipper leaves a headerless table as raw HTML; from the review of PR #43, in M3, with the choice of what a headerless table becomes left open |
| [#47](https://github.com/davison/md-notes/issues/47) | [#98](https://github.com/davison/md-notes/issues/98) | The clipper drops captions beside a highlighted code block, taking them for chrome; from the round-three review of PR #43, in M3 |
| [#82](https://github.com/davison/md-notes/issues/82) | [#99](https://github.com/davison/md-notes/issues/99) | Three residual findings on paths already refused: a dangling *directory* symlink out of the root answering `500`, a dangling two-hop chain answering `409`/`422`, and a `true == false` literal in a test table |
| [#88](https://github.com/davison/md-notes/issues/88) | [#99](https://github.com/davison/md-notes/issues/99) | A too-long note name answered `500 io_error` and logged as a server fault, on the save path as much as on create; from M5 QA |
| [#91](https://github.com/davison/md-notes/issues/91) | [#100](https://github.com/davison/md-notes/issues/100) | The note bar still scrolling sideways under an unbreakable failure message — 479 px in a 320 px bar after M5's fix — and a stray `name` field passed to `browser.newContext` |
| [#92](https://github.com/davison/md-notes/issues/92) | [#100](https://github.com/davison/md-notes/issues/100) | The deleted-on-disk banner still telling the reader to recreate the file with another tool, where **New note** could already do it |
| [#87](https://github.com/davison/md-notes/issues/87) | [#101](https://github.com/davison/md-notes/issues/101) | A vitest teardown flake in `ui/src/note-view.test.tsx` — a preact effect firing after jsdom is torn down, met once by M5's reviewer in a file that PR did not touch. The same shape #46 was for the Go side |
| [#89](https://github.com/davison/md-notes/issues/89) | [#101](https://github.com/davison/md-notes/issues/101) | Two edges in `ui/e2e`: a `drawerReady` helper beside `dialogReady`, the drawer's `Escape` effect attaching a frame late being the same race `dialogReady` already blunts; and an owner for the standing condition the retired `Escape` decision left behind |

Raised by this milestone's reviews, for a later task to adopt:

| Capture | From | What it is |
|---------|------|-----------|
| [#107](https://github.com/davison/md-notes/issues/107) | the re-review of [#105](https://github.com/davison/md-notes/pull/105#issuecomment-5704840025) | A table using `th` cells as row labels with a `pre` in the value cell is classified a data table by the new rule — any `th` makes a grid — and its code is inlined into one GFM cell, losing the line breaks. The same shape *without* the `th` keeps the code. The trade-off is recorded on #98; this is the case it costs |
| [#108](https://github.com/davison/md-notes/issues/108) | the review of [#106](https://github.com/davison/md-notes/pull/106#issuecomment-5704943849), widened by [M6 QA](https://github.com/davison/md-notes/issues/96#issuecomment-5706600141) | The clipped save-status message is unreachable by touch **or** keyboard: 34 px of message at 320 px and 104 px at 390 px against 2,839 px of text, with the whole of it in a `title` a coarse pointer cannot raise and a keyboard cannot focus. M6-R4 asked for "hover *or* focus" and is met as written; between them the review and QA close both halves of the affordance the trade-off rested on |
| [#111](https://github.com/davison/md-notes/issues/111) | [M6 QA](https://github.com/davison/md-notes/issues/96#issuecomment-5706600141), observation 1 | A dangling chain past `filepath.EvalSymlinks`'s budget answers create `409 exists` where delete, read and save answer `422 unsupported_source` — a second verb split, at the budget boundary rather than at the dangling-escape shape the #99 decision records, and written down nowhere |
| [#112](https://github.com/davison/md-notes/issues/112) | [M6 QA](https://github.com/davison/md-notes/issues/100#issuecomment-5706548457) | A confirmed **Recreate the note** returns the editor in vim normal mode where cancelling the same prompt keeps insert mode. The reader was typing when the note vanished and is typing again when it returns; the remount the adopted revision causes is what changes it |
| [#110](https://github.com/davison/md-notes/issues/110) | [#101](https://github.com/davison/md-notes/issues/101), reworded after the review of [#109](https://github.com/davison/md-notes/pull/109#issuecomment-5706208251) | Install the UI test cleanup once, in a vitest setup file, rather than per test file. Every file under `ui/src` now cleans up in an `afterEach`, but nothing stops the next one from getting it wrong and the fix lives in thirteen places. Written first from the "nine other files" figure and rewritten from the census once that was corrected — the capture the review said should *not* be opened from the old number, opened from the right one |

QA's six items — the two findings and four observations tabled above — are all
dispositioned at
[#96](https://github.com/davison/md-notes/issues/96#issuecomment-5706613081): none blocks
a requirement, and each is small and either already in hand or a rider on a later task.
Four of them are the four rows above that cite QA; the other two are #107, already
captured by the re-review that established its trade-off, and `docs/sync.md`, folded into
this task's sweep. [#110](https://github.com/davison/md-notes/issues/110) is not among
QA's items at all — it came from the review of #109, through the correction that withdrew
the figure it was first written from.

## Known gaps at the boundary

All nine captures this milestone's tasks adopted are closed, and QA found no requirement
unsatisfied. What remains true and will surprise someone who has not read this far:

| Gap | Where it is recorded |
|-----|----------------------|
| A data table with `th` row labels and a code block in the value cell still has that code flattened onto one line. It is the deliberate residual of the data-table rule, not an oversight | [#107](https://github.com/davison/md-notes/issues/107), [#98](https://github.com/davison/md-notes/issues/98#issuecomment-5704683965) |
| The note bar's failure message is clipped to an ellipsis at phone widths, and the `title` holding the rest can be raised by neither a coarse pointer nor a keyboard. **Retry** is reachable by both, and the reason is not | [#108](https://github.com/davison/md-notes/issues/108) |
| A dangling chain past the resolver's budget answers create `409 exists` where the other three verbs answer `422`. The `403`/`404` split above is recorded; this one is at the budget boundary and was not | [#111](https://github.com/davison/md-notes/issues/111) |
| A confirmed **Recreate the note** returns the reader to the editor in vim normal mode, where cancelling the same prompt keeps insert mode | [#112](https://github.com/davison/md-notes/issues/112) |
| A second `<pre>` inside one highlight container loses the wrapper's language and is fenced unlabelled. Left with the reviewer's own assessment that it is rare: the only fix that keeps the container's language reachable is to synthesise a highlight class onto the throwaway element so the rule re-matches | [#105](https://github.com/davison/md-notes/pull/105#issuecomment-5704592150) finding 7, [#98](https://github.com/davison/md-notes/issues/98#issuecomment-5704683965) |
| A `rowspan` is clamped to the rows the *table* has, not the rows its *section* has, so it still carries into a second `<tbody>` where a browser would not. Rare, and left rather than changed unreviewed at the end of the loop | [#105](https://github.com/davison/md-notes/pull/105#issuecomment-5704840025), [#105](https://github.com/davison/md-notes/pull/105#issuecomment-5704940724) |
| A note erased whole and then retyped comes back LF whatever ending it had, and **no test pins it**. Deliberately recorded rather than pinned, because pinning would make a rule out of what is incidental to `lineEnding("")` | [#100](https://github.com/davison/md-notes/issues/100#issuecomment-5705261754) |
| `GET` and `PUT` answer `404 not_found` for a dangling escaping link where create and delete answer `403 outside_root`. The four verbs disagree on this one shape, deliberately, until a task takes the read path up | [#99](https://github.com/davison/md-notes/issues/99#issuecomment-5703933261), [the introduction](../introduction.md#creating-and-deleting-notes) |
| The too-long message names 255 bytes, a limit the daemon has not verified for the filesystem in front of it. On a filesystem with a smaller `NAME_MAX` the number would be too generous while the refusal stayed correct | [#99](https://github.com/davison/md-notes/issues/99#issuecomment-5703933261), [#104](https://github.com/davison/md-notes/pull/104#issuecomment-5704545261) finding 5 |
| One of the three "nothing can follow this chain" errors is recognised by the *text* of a stdlib error, gated on it wrapping no `*fs.PathError`. A Go release that rewords it drops that case back to a `500`; `TestSaveSourceUnfollowableChain` is the canary | [#99](https://github.com/davison/md-notes/issues/99#issuecomment-5704685999) |
| **Renaming a note is still not in the application at all.** Untouched by this milestone, as by M5 | [the README](../../README.md), [#74](https://github.com/davison/md-notes/issues/74) |
| Nothing installs the UI test cleanup for a file that forgets it. All thirteen `ui/src` test files now clean up in an `afterEach`, but each does it for itself, and `@testing-library/preact` will not install its own while this project injects no test globals | [#110](https://github.com/davison/md-notes/issues/110) |
| The Escape propagation check observes listener *invocation*, through a wrapper the page would not otherwise have — one step away from a pure black-box assertion, and the price of asking a question about propagation rather than about pixels | [#101](https://github.com/davison/md-notes/issues/101#issuecomment-5705889343) |
| Two pages carried a claim the merged tasks had made false, and were still carrying it on `main` when QA ran: the README's *Over the tailnet* paragraph and the introduction's *Confinement* list both said `POST /api/clip` stays on the machine, the second eighty lines below the table in its own file that says the opposite. Corrected by this task's sweep, not by the task whose merge made them false — whose plan had undertaken to re-check every other page at the end | [#97](https://github.com/davison/md-notes/issues/97#issuecomment-5706541449), this task's [#102](https://github.com/davison/md-notes/issues/102) |

QA found no gap beyond the two findings and four observations above, and no requirement
unsatisfied.

## Where the record is silent

- **The scope decision came first this time, and that is the one M5 gap this milestone
  closed by doing.** M5's record found that its sequencing, its unanswered ask-the-human
  point and a PR's review disposition were all written down only after the record named
  their absence. Here the scope, sequencing and standing-confirmation decision is
  timestamped 20:05Z on [#96](https://github.com/davison/md-notes/issues/96#issuecomment-5703779089),
  three minutes before the first task started, and every task's judgement calls were
  recorded on its issue before the code that implements them — which each reviewer checked
  and each said so.
- **No `cc:needs-decision` gate was raised, and the milestone's one security widening went
  through without one.** #97's plan says so explicitly: "the security decision this task
  takes is the one #95 and M6-R1 exist to take, and M6-R1 states the outcome; it is
  recorded as a decision, not raised as a gate". The coordinator recorded where the
  authority came from after this record named the absence
  ([#97 (comment)](https://github.com/davison/md-notes/issues/97#issuecomment-5705427981)):
  the operator asked on 2026-09-16 whether there was a good reason not to permit clipping
  over the tailnet or whether it had simply not been captured, #95 was written in that
  exchange, and the operator opened the milestone with "start M6 with #95 and the M5
  captures", to which M6-R1 was written. So the human decision preceded the requirement
  rather than arising inside it, which is why no gate was raised.
  **The gap stands anyway, in two halves.** The exchange lives in the coordinator's
  session and not on GitHub, so the comment is a report of it after the fact — the same
  shape as M5's late decisions, and the same thing this record says about them. And what
  the operator authorised was the *capability*; the reasoning that bounds it — the
  comparison with the create M5-R2 already admits, the rejection of the session cookie,
  and the judgement that a clips folder filling up is an acceptable cost — was struck
  entirely between model sessions sharing one identity, and no gate ever put any of it in
  front of a person.
- **The operator has not used any of this.** M5's shape was set by the operator opening the
  merged UI and finding two things no review and no test had. Nothing equivalent happened
  here: every trade-off in this milestone was judged by an implementer, a reviewer and a
  QA session, all model sessions under one identity, and the five tasks were exercised in a
  browser and against scratch daemons by those sessions alone. The note bar at 320 px, the
  recreate control on a phone and a clip taken over a real tailnet are all measured and
  none is *used* — and the one finding that reads most like a person's is QA's, that a
  reader returned to their note by **Recreate the note** has to press `i` before they can
  carry on typing.
- **Nothing weighs what the milestone cost the bundles.** `dist/clip-inject.js` grew 3,801
  bytes raw and about 1.24 kB gzipped (50,523 → 54,324 B) for the converter, and the eager
  reading-page bundle grew 332 bytes brotli and 24 bytes of CSS for the note bar and the
  banner; the editor chunk, which is off the reading path, grew 106 bytes. Every figure was
  reproduced to the byte by a reviewer. There is still no budget to weigh any of them
  against, which M5's record said too.
- **Five parallel task sessions shared one scratchpad, and one of them overwrote another's
  pull request description.** It was caught, restored within minutes and recorded on the
  PR. Nothing says what else that arrangement can reach.

- **Four of QA's six items are about what nothing promised.** The vim mode after a
  recreate, the create verb's answer at the resolver's budget, the `title` a keyboard
  cannot reach, and a code block inside a `th`-labelled cell are each a place where the
  requirement, the plan and the decisions are all silent and the code had to answer
  anyway. None is a breach and each is now captured — but taken together they are the
  measure of how much this milestone decided implicitly, in work whose explicit decisions
  were unusually well recorded.
- **And the documentation sweep found nothing until the task whose job it was ran it.**
  #97's plan promised a grep over every other page "so this does not repeat M5's
  `sync.md` failure"; the grep would have found the README line, and the introduction was
  not re-read as a whole after one of its sections was rewritten. Both survived a model
  review that checked the changed pages, a merge, and three more merges, and were still on
  `main` when QA ran the promised grep itself
  ([#97](https://github.com/davison/md-notes/issues/97#issuecomment-5706541449)). M5's
  lesson was that a requirement's first clause is wider than its file list; M6's is that
  writing the sweep into a plan is not the same as running it.
