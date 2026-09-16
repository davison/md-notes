# M5 — Create and delete notes, a deterministic watch test, browser checks in CI

Tracking issue: [#74](https://github.com/davison/md-notes/issues/74). Its five
implementation tasks are merged on `main` at
[`3a8f100`](https://github.com/davison/md-notes/commit/3a8f100).

## Goal and outcome

The milestone's goal, as stated on [#74](https://github.com/davison/md-notes/issues/74),
was to make md-notes complete as the operator's only notes application — notes created
and deleted from the web UI instead of from a shell or a file manager, adopting
[#73](https://github.com/davison/md-notes/issues/73) — and to harden the gate everything
else merges through: the timing-driven tests in `internal/watch` made deterministic
(adopting [#46](https://github.com/davison/md-notes/issues/46)) and the browser-level
checks that had lived in session scratchpads brought into a CI suite (adopting
[#72](https://github.com/davison/md-notes/issues/72)). The inbox clipper stayed later
work, as it has since M3.

So the milestone has two halves that do not touch each other. One is a capability the
operator asked for. The other is the merge gate itself: M5-R1 and M5-R4 buy no
user-visible behaviour at all, and are here because M4's record left both as open entries
in its gaps table — the flaky debounce test it had watched fail four times, and the
browser-level measurements QA called the largest untested surface in that milestone
([the M4 gaps table](4-polish-phone-e-ink-and-the-bundle.md#known-gaps-at-the-boundary)).

What shipped is an application that owns the whole life of a note except its name.

`POST` and `DELETE` joined `GET` and `PUT` on `/api/r/{slug}/source/{path...}` — the
note's own source resource, rather than a new noun — so there is one path grammar, one
confinement check and one `{code, error}` envelope for all four methods. Create opens the
file `O_CREATE|O_EXCL` through a handle on the resolved parent, so a name already held by
a file, a directory or a link is refused rather than overwritten, and missing parent
directories are made through the same escape checks the clipper uses. Delete `Lstat`s the
final component through that handle — not `Stat` — requires a regular, writable markdown
file, and calls `Remove` on that one name; `RemoveAll` appears nowhere in the package.
Both are admitted over the tailnet on the same rule as the save, and `POST /api/roots`
and `POST /api/clip` stay loopback-only.

In the web UI a **New note** control in the top bar opens a prompt that names the folder
the note will land in before anything is written; a bare title becomes `<title>.md`
there, a name containing `/` is a path under the root, a missing extension gains `.md`,
and anything the daemon refuses comes back into the prompt with the typed name still in
the box. A **Delete** at the right-hand end of the note bar opens a confirmation naming
the full path, says so when this tab's unsaved draft will go with it, and removes nothing
if it is cancelled by button, by `Escape` or by the backdrop. Both reach every open page
through the events stream as ordinary change batches; neither refetches the tree.

Under that, `internal/watch` grew an unexported clock seam, and the debounce tests now
move time themselves instead of racing it — the flake that had failed CI four times in
M4, three of them on branches carrying no Go at all, is gone by construction rather than
by a widened tolerance. And `ui/e2e` is 35 Playwright checks under Node's own test
runner, driven by `make e2e` in a CI job of its own, holding the pane rectangles, the
breakpoint, the drawer, the tag chip, the tap targets, the light override before first
paint, the no-motion flash, the second load's zero asset bytes and the create and delete
flows — the measurements that M4 could only put in comments on issues.

The system as it stands is described in [the introduction](../introduction.md);
[Creating and deleting notes](../introduction.md#creating-and-deleting-notes) and
[Creating and deleting a note](../introduction.md#creating-and-deleting-a-note) are the
sections this milestone added to it, and
[What holds these numbers](../introduction.md#what-holds-these-numbers) is the one it
rewrote.

Five implementation tasks delivered it, each through its own PR and review loop, and this
document is the sixth ([#79](https://github.com/davison/md-notes/issues/79)):

| Task | Requirements | PR | Merged as |
|------|--------------|----|-----------|
| [#76](https://github.com/davison/md-notes/issues/76) Daemon: create and delete endpoints for notes | M5-R2, M5-R3 | [#80](https://github.com/davison/md-notes/pull/80) | [`bb911de`](https://github.com/davison/md-notes/commit/bb911de) |
| [#75](https://github.com/davison/md-notes/issues/75) Make the watch package's timing tests deterministic | M5-R1 | [#81](https://github.com/davison/md-notes/pull/81) | [`98051aa`](https://github.com/davison/md-notes/commit/98051aa) |
| [#77](https://github.com/davison/md-notes/issues/77) UI: create and delete a note with a name prompt and a confirmation | M5-R2, M5-R3 | [#83](https://github.com/davison/md-notes/pull/83) | [`42ab168`](https://github.com/davison/md-notes/commit/42ab168) |
| [#85](https://github.com/davison/md-notes/issues/85) Fix task: a delete button that stays put, and New note in the top bar | M5-R2, M5-R3 | [#86](https://github.com/davison/md-notes/pull/86) | [`c55bf69`](https://github.com/davison/md-notes/commit/c55bf69) |
| [#78](https://github.com/davison/md-notes/issues/78) Browser-level checks in CI: a ui/e2e Playwright suite | M5-R4 | [#84](https://github.com/davison/md-notes/pull/84) | [`3a8f100`](https://github.com/davison/md-notes/commit/3a8f100) |

Three of the five adopted a backlog capture and closed it on merge:
[#46](https://github.com/davison/md-notes/issues/46) by #75,
[#73](https://github.com/davison/md-notes/issues/73) by #77, and
[#72](https://github.com/davison/md-notes/issues/72) by #78. The other two —
[#76](https://github.com/davison/md-notes/issues/76), the daemon half of the operator's
ask, and [#85](https://github.com/davison/md-notes/issues/85), the fix task — have no
capture behind them. The M4 record's gaps table lists #46 and #72 as open; they are
closed here, and that table is not edited, because a merged record is sealed.

**The shape of the milestone is the fix task.** #85 exists because the operator opened
the merged UI on 2026-09-16 and found two things no review and no test had
([#74](https://github.com/davison/md-notes/issues/74#issuecomment-5701985349)): the
delete button moved between view and edit mode, and the create control at the top of the
navigator's list scrolled out of sight. Both had passed a model review, a unit suite and
a browser suite, because both are about where a control *is* over time rather than about
what it does. This is the first milestone in which the operator's own use changed
delivered work inside the milestone rather than producing a capture for the next one, and
it is the first in which a requirement's own wording was superseded after delivery: M5-R2
says the create control is "in the navigator", and it is not. The requirement was
deliberately **not** edited; the supersession is recorded as a decision on the milestone
issue instead
([#74](https://github.com/davison/md-notes/issues/74#issuecomment-5702275665)), so that
the record shows the requirement changed after delivery and why.

Why a fix task rather than a capture for M6 is recorded in the same late scope decision
([#74](https://github.com/davison/md-notes/issues/74#issuecomment-5702824535)): both
findings concern behaviour M5-R2 and M5-R3 themselves deliver, and the rule is that an
operator or QA finding against an *open* requirement becomes a fix task, so QA verifies
the requirement as it will ship. The cost was M5-R2's supersession. **Rejected:**
deferring the findings and closing M5 with the control in the navigator — shipping a
milestone the operator had already said was wrong.

How the milestone was sequenced is recorded, but late. M4 opened with a coordinator
decision fixing scope, ordering and the open defaults
([#55](https://github.com/davison/md-notes/issues/55#issuecomment-5656096967)); M5 ran
without one, and the equivalent decision was written only after this record named its
absence as a gap ([#74](https://github.com/davison/md-notes/issues/74#issuecomment-5702824535)).
It says what the order was and why: one capture per task, with the daemon task #76 split
from the UI task #77 so the API could be reviewed on its own and the UI briefed from its
final shape; #75 and #76 in parallel as independent packages; #77 after #76 merged; #78
in two stages, the ported M4 checks at once and the create and delete flows after #77;
QA and #79 last. The task bodies say the same thing from the inside — #75's "Take this
first: every other task in the milestone merges through the gate it guards", #77's
"Depends on the daemon endpoints task", #78's "Depends on the UI task for the create and
delete flows" — and the merge times bear it out. What is worth keeping straight is that
the reasoning was reconstructed after the milestone rather than set before it; the record
is complete, the discipline was not.

No human decision gate (`cc:needs-decision`) was raised anywhere in this milestone — the
third of the five so far where none was, and here that is a gap rather than an absence of
occasion: one judgment call was flagged for the operator, put to them twice and never
answered, and was settled by a coordinator decision instead of by a gate
([#76](https://github.com/davison/md-notes/issues/76#issuecomment-5702824267)). See
[Where the record is silent](#where-the-record-is-silent).

## Requirement outcomes

The verdicts below are drawn from the independent QA comment on the milestone issue
([#74](https://github.com/davison/md-notes/issues/74#issuecomment-5702704529)), run
against a clean worktree of merged `main` at
[`3a8f100`](https://github.com/davison/md-notes/commit/3a8f100) — never the operator's
checkout — with every daemon on its own temporary root, `--config`, `--state` and
`--token-file` under QA's scratchpad, and the daemon on port 7337 and `~/.local/state/mdn`
untouched. The floor first: `make check` exit 0, `go test -race -count=50
./internal/watch/` ok in 88.9 s, and `make e2e` green three consecutive times at 35 tests
each. QA judged M5-R2 and M5-R3 with the top-bar decision
([#74](https://github.com/davison/md-notes/issues/74#issuecomment-5702275665)) in force
and re-verified both of the operator's findings as fixed. It raised three findings, none
blocking; the coordinator's disposition of them is at
[#74](https://github.com/davison/md-notes/issues/74#issuecomment-5702723351).

| ID | Requirement | Status |
|----|-------------|--------|
| M5-R1 | Deterministic watch tests: the quiet window driven by an injected clock or an explicit, argued tolerance, the package green at `-count=50` under `-race` locally and in CI, the run recorded on the PR | [Satisfied](https://github.com/davison/md-notes/issues/74#issuecomment-5702704529) — no finding; QA mutation-tested the suite and reproduced the old flake's conditions at `GOMAXPROCS=1` under load |
| M5-R2 | Create a note: an endpoint that refuses to overwrite, refuses empty, non-markdown and escaping names with the save's error shape, makes missing parents, is admitted over the tailnet like the save; a UI control that asks for a title or path before anything is written, opens the note in the editor, and reaches the navigator through the events stream in both layouts | [Satisfied](https://github.com/davison/md-notes/issues/74#issuecomment-5702704529) — one non-blocking finding, a too-long name answered `500`, captured as [#88](https://github.com/davison/md-notes/issues/88) |
| M5-R3 | Delete a note: a note-bar action behind a confirmation naming the file, cancelling removing nothing; an endpoint that deletes exactly one markdown file inside the root, refusing directories, symlinks, non-markdown targets and read-only files; the app leaving for the root's home, the navigator live, another tab's draft answered by the existing banner | [Satisfied](https://github.com/davison/md-notes/issues/74#issuecomment-5702704529) — no finding; the safety gate was read as well as exercised |
| M5-R4 | Browser-level checks in CI: a `ui/e2e` Playwright suite sharing the extension suite's dependency and runner, driven by `make e2e`, running in CI against the built daemon in a cached Chromium, porting M4's scratchpad checks and covering create and delete, deterministic over three consecutive CI runs with its per-run cost recorded | [Satisfied](https://github.com/davison/md-notes/issues/74#issuecomment-5702704529) — one cosmetic finding, a case name naming the superseded location, taken in this task |
| M5-R5 | Documentation and record: user documentation reflecting create and delete, the API table gaining the new endpoints, the roadmap row, and this record linking delivered work, decisions, gates and QA verdicts | [Not testable at the verdict](https://github.com/davison/md-notes/issues/74#issuecomment-5702704529), this task not having run — "verdict to be superseded once #79 lands". Everything QA listed as missing on `3a8f100` is this task's declared scope and is delivered in its pull request |

M5-R5 is the one row no verdict yet settles, and for the same reason M2-R6, M3-R7 and
M4-R8 were not settled: QA graded it against `main` as it stood before this task, and what
it graded as missing — the README's Editing paragraph, the introduction's two statements
and its API table, `docs/milestones/5-*.md` and the roadmap row — is the list this
document's pull request delivers. The closure gate on
[#74](https://github.com/davison/md-notes/issues/74) requires both that every requirement
verdict is satisfied *and* that the milestone document is merged, so this PR's merge is a
precondition of closure rather than the verdict itself.

**What QA did beyond the requirement text** is worth recording, because it is where the
confidence in this milestone actually comes from. On the create path: twenty concurrent
pairs of tabs racing for the same name produced **exactly one `201` and one `409` every
time**, which is `O_CREATE|O_EXCL` doing what its comment claims; a unicode title with
leading and trailing spaces trims and lands in the open note's folder; a title of only
`.md` is refused in the prompt with the typed text kept and nothing written; encoded NUL
and newline are `400`. On the delete path: a **non-empty** directory named `full.md` is
`422` with all three files beneath it still present; a note replaced on disk by a symlink
to a file outside the root is refused `403` with the outside file intact; a hardlink to an
outside file removes only the name inside the root; a note another process removed first
shows the daemon's refusal inside the dialog and keeps it open to be read. And QA read
`Store.Delete` as the gate requires rather than only exercising it, confirming that
`parent.Remove(base)` on one basename is the only call in the path that changes the
filesystem and that `grep -rn "RemoveAll" internal/ cmd/` is empty. Thirteen browser
probes of its own on the shipped harness — 320 px, the backdrop dismissal the suite does
not take, a vanished note, the delete control's `x` across view → edit → view at 1440 px
and at 320 px — none of which failed.

QA's three findings, and what was done with each
([#74](https://github.com/davison/md-notes/issues/74#issuecomment-5702723351)):

| Finding | Disposition |
|---------|-------------|
| A basename of about 300 characters is refused `500 io_error` and logged as a server fault, where the reader sees "could not read or save note" ([#76](https://github.com/davison/md-notes/issues/76#issuecomment-5702672668)) | Captured as [#88](https://github.com/davison/md-notes/issues/88). Not a breach of M5-R2: the requirement asks for "the same error shape as the source save", and `PUT` on the same name answers the same way. The boundary is the filesystem's `NAME_MAX` — a 252-character basename creates normally — and create is simply the first UI path that lets a reader provoke it. **Rejected:** a second fix task inside M5 for a one-line error mapping, which would hold the milestone open for something the next `internal/source` task can carry with [#82](https://github.com/davison/md-notes/issues/82) |
| `ui/e2e/create-delete.test.mjs`'s first case is still named "creates a note **from the navigator**" ([#78](https://github.com/davison/md-notes/issues/78#issuecomment-5702677509)), and `ui/src/root-view.test.tsx`'s prologue above the moved block still says "Creating from the navigator" ([#85](https://github.com/davison/md-notes/issues/85#issuecomment-5702680749)) — each contradicting a case directly beneath it | Folded into this task, the same pass that reconciles the top-bar wording everywhere else. Two strings, no behaviour change; they are the only code in this document's pull request |
| Two sharp edges in `ui/e2e`: the drawer's `Escape` effect attaches a frame after its element is in the page, the race the harness already blunts for dialogs with `dialogReady`, and the standing condition in the retired `Escape` decision — "rewrite the browser case if a document-level handler is ever put under a dialog again" — has no owner | Captured as [#89](https://github.com/davison/md-notes/issues/89) |

## Decisions

### The debounce is driven by a clock the test moves, not by a wider tolerance

M5-R1 offered two designs and #75 took the first, because the second cannot state the
property the test is for. `TestBurstIsOneBatch` asserts that a burst is *one* batch and
that nothing follows it; against a real clock that is true only while the whole burst
lands inside the quiet window, so widening the window does not make the test correct, it
makes it wrong less often. `internal/watch/watch.go` gained an unexported `clock`
interface (`Now`, `NewTimer`), a `timer` interface (`C`, `Stop`, `Reset`), a `wallClock`
that is `time.Now` and `time.NewTimer` and nothing else, and an unexported `withClock`
option; `loop` is the only caller
([#75](https://github.com/davison/md-notes/issues/75#issuecomment-5700996362)).

- **Trade-off:** the wall clock now reaches `loop` through an interface — one
  non-inlined `Now` per event and one allocation per watcher for the timer box. A
  watcher creates one timer in its lifetime and resets it thereafter, so the cost is a
  rounding error against an inotify event; the alternatives, a build tag or an exported
  knob on `New`, would have charged the daemon's API instead.
- **Rejected: widening the quiet window under `-race`.** It leaves the test asserting a
  property of the machine, it slows every run that is not failing, and it has no honest
  stopping point — the four recorded failures are scheduling delays of unknown size on a
  shared runner.
- **Rejected: converting the whole package to synthetic events.** The tests of watch
  placement, coverage, budget reclamation and directory rename are about what the kernel
  and ripgrep actually do; replacing those would delete coverage rather than stabilise
  it. They keep real events and one-sided waits with their bounds named. What was removed
  from them is five 20 ms "let the watches settle" sleeps — `New` places every watch
  before it returns, so they bought nothing — and one 400 ms sleep, now a poll on the
  condition it was waiting for.
- **Bought with the seam:** `maxWait`, that a stream which never goes quiet still yields
  a batch, had no test at all, because testing it meant sleeping for it.

### Create and delete are two more methods on the note's own resource

`/api/r/{slug}/source/{path...}` is already the note's source as a resource — `GET` reads
it, `PUT` replaces it — so `POST` creates and `DELETE` removes, and the daemon takes the
note's whole path rather than a title plus a folder in a body
([#76](https://github.com/davison/md-notes/issues/76#issuecomment-5700721960)). One path
grammar, one confinement check, one error envelope, and one thing for the tailnet
allow-list to reason about. M5-R2 already puts the `<title>.md`-in-a-folder rule in the
UI, so the endpoint gets the refusal set instead; nothing is lost in safety by letting
the UI compose the path, because the daemon refuses every unsafe name it is handed.

- **Trade-off:** a caller can still create a note whose name is awkward on another
  platform — a `\` or a `:` on Linux, a trailing space. Refusing those would mean the
  daemon deciding which filesystems the notes root may be copied to, which it does
  nowhere else.
- **Rejected:** a body of `{"folder", "name"}` with the daemon appending `.md`. It splits
  one path across a URL and a body and leaves the daemon with two ways to name the same
  file.

### Both verbs work in every registered root, `mdn open` ones included

`PUT` has always saved into every root in the registry, recent ones included. A root
opened with `mdn open` is a directory the operator pointed the daemon at on purpose, on a
single-user machine; a save can already rewrite any markdown file in it, so refusing to
create a sibling or remove one would be a narrower rule than the one already governing
the same bytes — and keeping the three write verbs on one rule keeps the tailnet
allow-list a statement about the `source` resource rather than about which slug is in the
URL ([#76](https://github.com/davison/md-notes/issues/76#issuecomment-5700724927)).

- **Trade-off:** `mdn open ~/some/project` therefore admits delete as well as save for
  the markdown under it, for as long as that root is registered. The confinement is
  unchanged — one regular markdown file inside the root, never a directory, never through
  a link — and `POST /api/roots` stays loopback-only, so nothing reachable over the
  tailnet can widen the set of roots this applies to.
- **Rejected:** restricting the two new verbs to `kind: notes`. It splits the write rules
  in two for no gain the operator asked for, and the UI would need to know which roots
  are writable before it could decide whether to show the controls.
- **This was the milestone's one ask-the-human point**, recorded as the default rather
  than taken silently and flagged to the coordinator to put to the operator. The operator
  never answered. The coordinator put it to them twice on 2026-09-16 with the reversal
  cost stated, and — after this record named the silence as a gap — recorded that the
  default stands as shipped on the coordinator's authority under the standing merge
  confirmation, until the operator says otherwise
  ([#76](https://github.com/davison/md-notes/issues/76#issuecomment-5702824267)). That is
  a record of the question having been asked, not an answer to it, and no
  `cc:needs-decision` gate was ever raised, which the same comment calls the stricter form
  this should have taken. Reversing it is `kind == notes` guards in the two handlers, their
  refusal tables, and the introduction's API and tailnet sections;
  `TestCreateAndDeleteInARootOpenedAtRuntime` pins the current behaviour, so a reversal
  breaks a test rather than passing quietly.

### The refusal table was amended by the review, not the code by the table

The model review of PR #80 found three unmapped refusals, all on paths the UI will never
compose. Two changed what the daemon answers, and the fix went to the resolver rather
than to the error map
([#76](https://github.com/davison/md-notes/issues/76#issuecomment-5701086842)). A
dangling symlink pointing out of the root had answered `409 exists` on create and
`422 unsupported_source` on delete, because the resolver calls a link with no target
*missing* rather than *outside*; its target is now read and checked lexically, which is
all there is to check when there is no real path, and it answers the `403 outside_root`
the table always promised. A path component that is an existing file was an unmapped
`500` and is now `404 not_found`. And a note under a directory reached by an *absolute*
symlink inside the root answered `500` on the two new verbs while `GET` and `PUT`
answered `200`, because create and delete handed an unresolved multi-component name to a
handle that may not traverse an absolute link; both now resolve the parent first, as the
read and save paths always have, so all four verbs agree. Fixed in `8ea407b`.

- **Trade-off:** `Root.Escapes`, the one new thing that answers lexically rather than by
  resolving, reads one link and no further. The re-review established the property that
  bounds it: it is only ever called on a path already being refused, so it can choose
  *which* refusal a caller gets and can never turn a refusal into a permit
  ([#80](https://github.com/davison/md-notes/pull/80#issuecomment-5701203689)).
- **Rejected:** mapping the symptoms in `sourceError` and leaving the resolver alone. It
  would have papered over the real fault — that the new verbs reached a note differently
  from the old ones — and left the four methods disagreeing about the same path.

### "The selected folder" is the folder of the open note, and the root when none is open

M5-R2 makes a bare title land in "the selected folder of the navigator", and the
navigator has no folder selection: its file rows are links and its directory rows only
expand and collapse, with expansion remembered per root, so several are open at once and
the last one clicked is as often being closed as opened. Inventing a selection out of
that would give the application a second notion of "where you are" that nothing on screen
marks, and the note it disagreed with would be the one the reader is looking at
([#77](https://github.com/davison/md-notes/issues/77#issuecomment-5701434013)). The
prompt names the folder it is about to use — `docs/deep`, or "the root of this folder" —
before anything is written, and a name containing `/` overrides it.

- **Trade-off:** creating a note in a folder you are not reading in means typing the
  folder into the name. That is the case the `/` rule exists for.
- **Rejected:** tracking the last directory the reader expanded. It is invisible, it
  survives navigation that has nothing to do with it, and "the last folder I opened" and
  "the folder I am reading in" diverge immediately.

### A deleted note's draft in this tab goes with it, and the app lands on the root's home

The confirmation gains a line — "This tab has unsaved changes to this note. They go with
it." — and confirming removes the file and drops the session. The alternative was
refusing the delete until the draft was saved, which means saving a note in order to
throw it away: the bytes reach disk for the length of one request and the reader does
work to destroy work. Nothing is lost silently, because the dialog says it before it
happens and cancelling is right there
([#77](https://github.com/davison/md-notes/issues/77#issuecomment-5701434216)).

Mechanically the session is dropped as soon as the daemon answers and **before** the
shell navigates, which cancels the scheduled save so nothing recreates the file a moment
later — and is why the delete path does not go through the conflict machinery at all. The
mirrored draft leaves `localStorage` only when the record there is this session's own,
the rule `Session.set` already applies on the clean path. In *another* tab nothing
changes: that tab learns from the events stream, its session finds a 404 under a draft,
and the existing deleted-on-disk banner answers with keep or copy. No second dialog.

M5-R3 says the app goes to "the parent folder or the root's home". This application has
no route for a folder — `/r/:slug/:note*` is a note or nothing — so those are one page,
`/r/{slug}/`, and the navigator arrives with the deleted note's folder still expanded.

- **Trade-off:** the wording of M5-R3 describes two destinations where the app has one.
  Adding a folder route to satisfy it would be a new view, not a delete.
- **Rejected:** refusing the delete under an unsaved draft; a second dialog in the other
  tab, which the existing banner already covers and which has a test in
  `note-pane.test.tsx`.

### Both prompts are the application's own dialog

Not `window.prompt` / `window.confirm`: they are the browser's chrome, with no
light-theme override, no no-motion setting and no 40 px tap targets, and the deciding one
is the refusal — `window.prompt` returns a string and forgets it, so showing the daemon's
"a note by that name already exists" would mean asking again with an empty box, which
makes M5-R2's "show the daemon's refusal in the prompt and let the user correct the name"
unimplementable. Not a native `<dialog>` either, which would give modality, the backdrop
and `Escape` free: jsdom, the environment the whole `ui/` suite runs in, implements no
`showModal`, so a native dialog could not be mounted in a unit test at all — and the
dialog logic is exactly what those tests are for
([#77](https://github.com/davison/md-notes/issues/77#issuecomment-5701434444)).

What was written instead is the shape the drawer and the settings popover already use:
`role="dialog" aria-modal="true"` with a backdrop, focus moved in on open, `Tab` cycling
inside, focus handed back to the opener, `Enter` confirming through the form's submit
button and `Escape` cancelling. One thing differs from the popovers already here:
`Escape` is answered on `window` in the capture phase and stopped there, where the drawer
and the editor listen on the *document*, so the topmost dialog answers alone.

- **Trade-off:** a dialog component the project maintains, against two the browser
  maintains. It is one file, and `<dialog>` is worth revisiting when jsdom implements
  `showModal`.
- **Rejected:** the browser's pair, and the native element, both above.
- **What became of the `Escape` rule** is its own decision, below: the browser-level
  check written to hold it could not survive #85.

### A dialog with a write in flight cannot be dismissed at all

The review of PR #83 found that the `busy` guard covered the buttons and not the other
three ways out: `Escape`, the backdrop and the `Tab` trap all ignored it, so a reader
could press `Escape` during a create, see the prompt close, and have the app navigate to
a note two and a half seconds later. Both shapes were on offer. The one taken is that
dismissal *loses*: while `busy`, `Escape` and the backdrop do nothing, which is what the
two disabled buttons already say
([#77](https://github.com/davison/md-notes/issues/77#issuecomment-5701747937)).

- **Trade-off:** the one way out of a modal that cannot be seen to be disabled is
  disabled. It is safe to refuse the exit because the window is one round trip to a
  daemon on loopback and the dialog closes itself the moment the answer arrives — gone on
  success, kept with the message on a refusal — and every rejection path sets `busy`
  false, so nothing can leave it stuck open.
- **Rejected:** dismissal wins and the in-flight write's navigation is cancelled. The
  request has been sent and the daemon acts on it either way, so "cancel" could only mean
  "do not route to the note you just made", leaving the reader with a file they cannot
  see they created.
- **Found with it:** disabling the control that holds focus drops focus to the body,
  outside the dialog, so the `Tab` trap never saw the keystroke and `Tab` walked into the
  page behind it. The dialog now takes focus itself for the length of the request and
  hands it back to the text box, since a refusal is a name to correct.

### The delete button's fixed place is the right-hand end, and the auto margin moves onto it

The operator's first finding. The bug was not the order of the note bar, which never
changed: it was that the element carrying `margin-left: auto` was `.save-status`, which
`SaveStatus` renders as `null` for a session that has never read its note — and a note
opened for *reading* is exactly that case, so the bar had no auto margin in it at all and
the button sat wherever the text before it ended. Entering the editor read the note,
`Saved` appeared carrying the margin, and the button went right with it; leaving the
editor does not unread the note, so it stayed. The fix puts the auto margin on the control
whose position must not move, which makes the position a property of that control rather
than of another element's presence
([#85](https://github.com/davison/md-notes/issues/85#issuecomment-5702098868)).

Measured in Chromium at 1280 px on `index.md`: before the fix the button's `x` was 368
unedited and 936 editing and after; after it, 936 in all three states.

- **Trade-off:** the left-hand end, beside the mode toggle, was the other way to make the
  position fixed. It pairs a destructive control with a frequently-tapped one at 41 px
  targets, and it is not where the button had been for any note the operator had edited.
- **Rejected:** changing the DOM order of the bar, which was never the fault.
- **Residual:** `.save-status` also *gained* `min-width: 0` so a long failure message
  shrinks rather than shoving its neighbours. The review measured what that buys and it
  is less than the record claims; see [Known gaps](#known-gaps-at-the-boundary).

### The create control moves to the top bar, and stays there in the drawer layout

The operator's second finding: the control lived at the top of the navigator's list and
scrolled out of sight on a long tree. Putting it back inside the drawer at narrow widths
would reproduce exactly that on a phone, where the list is longest relative to the
viewport, so it is in the top bar at both widths and the stylesheet changes its shape
rather than its home — a `+` with the label "New note" wide, a 2.75 rem square carrying
just the `+` below the breakpoint, beside the burger and the magnifier already drawn that
way. The accessible name is an `aria-label`, so it does not change when the label is
hidden ([#85](https://github.com/davison/md-notes/issues/85#issuecomment-5702099098)).

- **Trade-off, stated in advance rather than discovered later:** the drawer is modal. Its
  backdrop covers the whole top bar and its focus trap keeps `Tab` inside it, so with the
  drawer open the create control cannot be clicked or tabbed to — the same as the
  magnifier and the gear it now sits beside. Creating a note at narrow widths is "close
  the drawer, tap `+`". That is one tap more for someone already in the drawer, and one
  scroll less for everyone who is not.
- **Rejected:** keeping it in the navigator below the breakpoint, which is the finding
  itself, on the smallest screen.
- **And it contradicts M5-R2's own wording**, which says the control is in the navigator.
  The operator's finding supersedes the requirement, and the supersession is recorded as
  a decision on the milestone issue rather than by editing M5-R2
  ([#74](https://github.com/davison/md-notes/issues/74#issuecomment-5702275665)): editing
  the requirement in place would hide that it changed after delivery and why. The
  reviewer of #86 raised the contradiction and asked for exactly this
  ([#86](https://github.com/davison/md-notes/pull/86#issuecomment-5702263505), finding 4).

### Playwright is declared in `ui/`, and the four e2e files share one harness

Where the dependency lives was a deviation and is recorded below. The design half is that
`ui/e2e/harness.mjs` is the rig every file imports — Playwright resolution, a free port, a
`waitFor` poll, a `mkdtemp` fixture tree, a daemon on a temporary root with its own
`--config`, `--state` and `--token-file`, the device profiles — about seventy lines, where
four copies of it is how four files come to disagree about which port is free
([#78](https://github.com/davison/md-notes/issues/78#issuecomment-5701662110),
[#78](https://github.com/davison/md-notes/issues/78#issuecomment-5701859723)).

- **Trade-off:** `make ui-deps`, and so `make build`, now installs the Playwright driver
  — 707 ms and about 2 MB. The *browser* is not downloaded by it: pnpm 10 does not run
  the package's install script, so `pnpm --dir ui exec playwright install chromium` stays
  an explicit step, which is what lets CI cache it as its own artefact and keeps an
  ordinary `make build` from fetching 150 MB of Chromium.
- **Rejected:** both packages declaring it (two lockfile entries, two installations, and
  a version in one drifting from the version the other's cache key was computed from);
  a repository-root `package.json` (the repository has no root Node package and this was
  not the task to give it one).

### What makes a ported check deterministic: three choices, one shape

The suite asserts what the stylesheet promises and never what a dependency happens to say
today ([#78](https://github.com/davison/md-notes/issues/78#issuecomment-5701667426)).

1. **The device profiles are the suite's own literals, not `playwright.devices`.** The
   registry's viewports move between releases — Playwright 1.63 has Pixel 7 at 412x839
   where the M4 harness measured 412x792 — so a ported figure taken from it would fail on
   the next dependency bump for no regression at all.
2. **Each figure is asserted beside the rule it comes from.** M4's Pixel 7 numbers — a
   47 px bar over a 412x745 note, a 300x792 drawer — are pinned in a test of their own;
   every other profile asserts the relation instead (bar plus note equals the viewport,
   the three wide panes sum to the window, `min(20rem, 85vw)` at a 15 px root is 300). A
   viewport that moves fails the pinned test loudly rather than passing the relational
   ones on a coincidence. The 960 px breakpoint is walked at 959, 960 and 961.
3. **The light override is proved with the application bundle aborted.** "Before first
   paint" cannot be sampled after the fact without a race, so the check routes
   `**/assets/*.js` to `abort()` and only the inline script in `index.html` can have run.

- **Trade-off:** the profiles no longer track the real devices as Playwright's data does.
  They are the geometry the recorded numbers were taken at, which is what a ported check
  needs; a real device's new viewport is a decision to retake the numbers, not something
  a dependency bump should do silently.
- **Rejected:** a fixed pause anywhere (there is none in the suite; every wait is a
  condition); screenshot comparison (a pixel baseline per profile per theme, regenerated
  on every font or Chromium change, to answer questions `getBoundingClientRect` answers
  exactly); asserting the *first*-load byte count, which is a bundle size and would fail
  on every dependency change — the check asserts what M4-R7 actually promised, that the
  second load fetches none.

### `make e2e` is a CI job of its own and is not part of `make check`

The browser is a 150 MB download the Go and unit suites do not need, and `make check` is
what a developer runs before pushing. As a separate job the two report independently, a
Chromium cache miss never sits in front of the Go suite, and the milestone's "three
consecutive green runs" is a green square with its own history. The suite skips rather
than fails when the browser or the binary is missing
([#78](https://github.com/davison/md-notes/issues/78#issuecomment-5701667426)).

- **Trade-off:** a developer who runs only `make check` does not run the browser checks,
  so a stylesheet regression reaches CI rather than the working tree.
- **Measured:** three green runs of `dbf8e41` —
  [35133653370](https://github.com/davison/md-notes/actions/runs/35133653370),
  [35133658423](https://github.com/davison/md-notes/actions/runs/35133658423),
  [35133687685](https://github.com/davison/md-notes/actions/runs/35133687685) — 35 tests
  in each, the `make e2e` step 16 s, the suite itself 9.78 s / 10.08 s / 9.81 s, and the
  Chromium step 6-8 s on a cache hit against 15-27 s cold. The job is about a minute and
  a half end to end and the browser work in it is ten seconds.

### The `Escape` browser check cannot hold the propagation rule after #85, and says so

The review of PR #83 asked for a browser case proving that a dialog's `Escape` does not
also reach the layer below. It was written against the drawer and it had teeth: commenting
out `e.stopPropagation()` in `ui/src/dialog.tsx` failed that check and nothing else. Then
#85 moved the create control into the top bar, where the drawer's own backdrop makes it
inert, and the case was retargeted to the editor on the advice that the rule still
mattered there — and then measured, which is the step the previous round of this task had
taught the implementer not to skip. It does not
([#78](https://github.com/davison/md-notes/issues/78#issuecomment-5702378000)).

What was measured against the built daemon at `c55bf69`: with the drawer open,
`.topbar .new-note` is visible and `locator.click` times out, because the backdrop takes
the click, so no dialog can be opened over the drawer. Opening the prompt *closes* the
settings panel, through the panel's own documented pointerdown-outside path. And the
editor is not a third layer: its vim keymap listens on the editor's own element, which an
event dispatched at the dialog never reaches — commenting out `e.stopPropagation()` and
running the whole suite gives 35 tests, 35 passing.

- **Trade-off:** `stopPropagation` is now covered only by the unit test on #83. The
  decision records that as testing "that it is called rather than what it achieves"; the
  re-review of #84 corrected that in the safe direction —
  `ui/src/dialog.test.tsx:66` registers a capture-phase `document` listener and asserts it
  never fires, which is the propagation *effect*, so `make check` does catch the mutation
  ([#84](https://github.com/davison/md-notes/pull/84#issuecomment-5702513643)). If a
  future change puts a document-level `Escape` handler back underneath a dialog, the rule
  becomes observable again and the case should be rewritten against it; the comment in
  `ui/e2e/create-delete.test.mjs` says so where the next reader will ask.
- **What the case holds instead**, renamed to say so: a reader who opens the prompt over
  a half-typed note, changes their mind and presses `Escape` gets the prompt closed, the
  text intact and the editor still in insert mode.
- **Rejected:** deleting the case (the behaviour it does hold is worth holding, and an
  empty space would invite the same finding a third time); keeping the claim that it
  exercises the propagation rule (the suite would carry a false statement about itself —
  the exact thing the review of #84 had just found in the ripgrep diagnostic); and forcing
  the stacking with `click({ force: true })` through the backdrop, which a reader cannot
  do, so neither should the suite.

## Deviations

### #78 declared Playwright in `ui/package.json`, where the plan said `extension/`

The plan put the dependency in `extension/package.json` — the first e2e suite's own
package, and the second place its loader already looks — to keep `make build` off a
Playwright install. Task #77's `ui/e2e/create-delete.test.mjs`, which #78 inherits,
resolves Playwright from `[PLAYWRIGHT_ROOT, uiDir, repoRoot]` and does not look in
`extension/`, so a dependency declared there would have left that file skipping in CI for
want of an environment variable — the exact failure mode
[#72](https://github.com/davison/md-notes/issues/72) was filed for. Declaring it where
the suite lives is what makes `pnpm --dir ui e2e` work with nothing set. The extension's
three suites keep their dependency-free resolution and now find the same installation, so
`pnpm --dir extension e2e` no longer needs `PLAYWRIGHT_ROOT` either — which is the
"sharing the extension suite's Playwright dependency" half of M5-R4 discharged in the only
way available, the extension suite having had no declared dependency to share
([#78](https://github.com/davison/md-notes/issues/78#issuecomment-5701662110)).

### #78's stage-two file is #77's, folded onto the harness, not a new `notes.test.mjs`

The plan named a fourth file of its own. What landed is #77's `create-delete.test.mjs`
with its seventy-line prologue deleted and the harness imported in its place, every test
body untouched; the shared fixture gained `docs/guide.md`, the note in a folder those
checks read, and the daemon it starts now gets an explicit `--config`, which #77's file
did not pass — without it a run reads, and an `mdn open` would write, the developer's own
`~/.config/mdn`
([#78](https://github.com/davison/md-notes/issues/78#issuecomment-5701859723)).

### CI found two portability faults the plan could not have

`node --test e2e/` — the extension suite's form, and #77's — resolves its argument as a
*module path* on Node 24, the version CI pins, and dies with `MODULE_NOT_FOUND` before a
test runs; Node 26, which the task was written on, walks the directory. The script names
`e2e/*.test.mjs`, which every version understands. And the new job had no ripgrep, so the
daemon served an empty navigator and eleven checks failed thirty seconds apart on elements
that were never coming; the job installs it as `check` always has
([#78](https://github.com/davison/md-notes/issues/78#issuecomment-5701859723)). Two
differences between a developer machine and a two-core runner were fixed with them:
`return page.evaluate(...)` inside a `try` hands the promise back before the `finally`
runs, so a context was being closed under a read still in flight; and an exact-height
check pinned `.drawer-tab` at 41.25 px, where "Search & tags" wraps to two lines and 62 px
on a machine with different fonts.

### #85 is itself a deviation at milestone level

Two requirements were delivered, reviewed and merged, and then changed inside the same
milestone because the operator used the result
([#74](https://github.com/davison/md-notes/issues/74#issuecomment-5701985349)). The fix
task was opened rather than a capture filed for M6. Nothing on the record weighs that
choice; see [Where the record is silent](#where-the-record-is-silent).

## Corrections to the record itself

Three times a claim on the record was wrong about the code, and each was corrected as a
new comment rather than by editing the one it corrects, so the claim and its answer both
stay legible.

- **The sharpest item of the milestone: a diagnostic asserted working that had never been
  run.** A decision comment on #78 said "the harness now asks the daemon for the fixture
  tree before any browser opens, so the next person who forgets gets one sentence naming
  ripgrep instead of a wall of Playwright timeouts". The question was asked and the answer
  was thrown — but `startFixture` had already spawned the daemon, and a child process with
  its stdio pipes attached keeps the `node:test` worker's event loop alive, so the throw
  did not fail the file, it **hung** it: the message never flushed, the worker never
  exited, and an `mdn serve` and its `mkdtemp` tree were left behind. The review of PR #84
  reproduced four minutes of silence and a leaked daemon on port 33311
  ([#84](https://github.com/davison/md-notes/pull/84#issuecomment-5702105475), finding 1).
  So the commit that set out to replace eleven obscure thirty-second failures with one
  clear sentence made the failure mode worse, and the improvement was asserted on the
  issue without the case it was for ever being run. The implementer's own words:
  "a diagnostic is not verified by reading it"
  ([#78](https://github.com/davison/md-notes/issues/78#issuecomment-5702221318)). Fixed in
  `3928971` — the whole start sequence inside a `try` whose `catch` `SIGKILL`s the child
  and removes the tree before rethrowing — and measured with `rg` off `PATH`: the message
  printed once per file and the suite exited in 439 ms with nothing left behind.
- **And the device profiles were not "the viewports M4 measured".** The same decision
  claimed all four phone profiles came from M4. Two did not: the landscape pair was the
  portrait pair with its numbers swapped — 792x412 and 664x390 — where M4 measured
  **863x360** and **750x340** ([#55](https://github.com/davison/md-notes/issues/55), and
  PR [#69](https://github.com/davison/md-notes/pull/69)'s table). A browser's own chrome
  is a different height when the device is on its side, so a rotation was never the same
  profile. Nothing false was *asserted*, because only the relational checks run at those
  two sizes, but the record could not have been reconciled against #55. The real numbers
  are in `e425c2f`, and the correction says the first one's lesson applies to it too
  ([#78](https://github.com/davison/md-notes/issues/78#issuecomment-5702221318)).
- **A trade-off recorded larger than it is.** The decision retiring the `Escape` browser
  case says the propagation rule is left to a unit test that "tests that it is called
  rather than what it achieves". The re-review of #84 checked and found the unit test
  registers a capture-phase `document` listener and asserts it never fires, which is the
  effect and not the call, so `make check` does catch the mutation
  ([#84](https://github.com/davison/md-notes/pull/84#issuecomment-5702513643)). The record
  undersells its own coverage, which is the direction an error should point. This
  document is where that is corrected; the decision comment is not edited.

One further claim was flagged and **not** corrected: the decision on #85 and PR #86's body
both say `.save-status` *keeps* `min-width: 0`, where on `main` at
[`42ab168`](https://github.com/davison/md-notes/commit/42ab168) the rule was
`margin-left: auto` and nothing else — the declaration is new, not retained
([#86](https://github.com/davison/md-notes/pull/86#issuecomment-5702263505), finding 2).
It is corrected here rather than there.

## What the reviews changed

Every task was reviewed by an independent model session under the reviewer contract, in a
clean worktree, and every review ran the suites rather than reading them. Four of the five
approved on the first pass with non-blocking findings; one requested changes.

| PR | Verdict | What the review changed |
|----|---------|-------------------------|
| [#80](https://github.com/davison/md-notes/pull/80) | approve, then approve on the delta | Three non-blocking nits, all error mapping on paths the UI will never compose. All three taken in `8ea407b`, and the fix went to the resolver rather than the error map; the decision table on #76 was amended. The reviewer exercised the safety gate against the binary — 26 calls, then 40, with a whole-tree diff after each — and raced 1,800 create/delete calls against a directory being flipped to a symlink out of the root, which nothing escaped |
| [#81](https://github.com/davison/md-notes/pull/81) | approve, then approve on the delta | `TestEachChangeRestartsTheQuietWindow` did not catch the mutation it names: the reviewer's mutant passed it 50/50, because the absence check was a bare non-blocking receive that could not see a batch the immediately preceding `Advance` had caused. The answer was better than the suggestion — a barrier on each side of the check, so the whole class of missed absence goes, plus a third change so the contents discriminate as well as the timing. The mutant now fails 50 of 50. The header comment that overclaimed was narrowed |
| [#83](https://github.com/davison/md-notes/pull/83) | approve | The `busy` guard covered the buttons and not `Escape`, the backdrop or the `Tab` trap; the reviewer demonstrated it with the write verbs delayed 2.5 s. Fixed, with the decision above, and the `Tab` hole turned out to be one step earlier than the empty focusable list — disabling the focused control drops focus to the body. The review also asked for the browser case that #78 later could not keep |
| [#84](https://github.com/davison/md-notes/pull/84) | **request changes**, then approve | One blocking finding — the ripgrep diagnostic that hung instead of printing — and four non-blocking. All five fixed. `TAP_GROUPS` gained `.new-note`, `.modal button` and `.modal-name`; the last was measured **nowhere**, so a regression dropping the name box below the 40 px floor had been passing the suite. The reviewer then went looking for the teeth rather than taking them: a rule shrinking only `.modal-name` fails five checks, one per coarse profile |
| [#86](https://github.com/davison/md-notes/pull/86) | approve | Six non-blocking findings, and the one PR in this milestone whose approve was merged with no fix pass and no reply. Their disposition was recorded after the merge, and after this record named its absence as a gap ([#86](https://github.com/davison/md-notes/pull/86#issuecomment-5702824030)): finding 4 — that M5-R2's wording now contradicts the shipped UI — is the decision on [#74](https://github.com/davison/md-notes/issues/74#issuecomment-5702275665); findings 3 and 6 are captured as [#91](https://github.com/davison/md-notes/issues/91); finding 2 is a wrong sentence in the PR body that stays as written, corrected here and now on the PR as well; findings 1 and 5 are noted with no action — and finding 1 was found again by QA, on both files that carry the drift, so it is taken in this task's pull request after all. Finding 5's point survives: the new unit case passes unchanged against `main`, because the DOM order never changed and the bug was purely CSS, which leaves the browser check as the only thing between that bug and a repeat |

Every PR carries the operator's confirmation as a comment: "reviewed and accepted by
@davison as both author and operator (pure solo tier, SPEC §6) — no independent principal
exists in this project". Every role in this project is routed to the operator, so the
implementer, the reviewer and the operator are one person's identity and three clean-context
model sessions. The milestone's gate on
[#74](https://github.com/davison/md-notes/issues/74) makes that standing confirmation
explicit for M5: an approved PR merges without a per-PR wait.

## Captures adopted, and captures raised

Adopted and closed by this milestone:

| Capture | Adopted by | What it was |
|---------|-----------|-------------|
| [#46](https://github.com/davison/md-notes/issues/46) | [#75](https://github.com/davison/md-notes/issues/75) | `TestBurstIsOneBatch` flaky in CI — four recurrences inside M4, three of them on branches carrying no Go at all |
| [#73](https://github.com/davison/md-notes/issues/73) | [#77](https://github.com/davison/md-notes/issues/77) | Create and delete a note from the app |
| [#72](https://github.com/davison/md-notes/issues/72) | [#78](https://github.com/davison/md-notes/issues/78) | Browser-level checks for layout, e-ink settings and caching living only in scratchpads |

Raised by this milestone's reviews, for a later task to adopt:

| Capture | From | What it is |
|---------|------|-----------|
| [#82](https://github.com/davison/md-notes/issues/82) | the follow-up review of [#80](https://github.com/davison/md-notes/pull/80#issuecomment-5701203689) | Three residual findings, all on paths that are already refused, so none can turn a refusal into a permit: a dangling *directory* symlink out of the root answers `500 io_error` rather than `403 outside_root`; a dangling two-hop chain answers `409 exists` or `422 unsupported_source` rather than `403 outside_root`; and `TestEscapesIsLexical` carries a `true == false` literal |
| [#87](https://github.com/davison/md-notes/issues/87) | the review of [#86](https://github.com/davison/md-notes/pull/86#issuecomment-5702263505) | A vitest teardown flake in `ui/src/note-view.test.tsx`: the reviewer's first `make check` failed with an unhandled `ReferenceError: window is not defined` from a preact effect timer firing after jsdom teardown, in a file the PR did not touch; six later runs were clean. The same shape #46 was for the Go side |
| [#88](https://github.com/davison/md-notes/issues/88) | M5 QA, on [#76](https://github.com/davison/md-notes/issues/76#issuecomment-5702672668) | A basename at the filesystem's `NAME_MAX` is refused `500 io_error` and logged as a server fault, on the save path as well as on create. One `errors.Is(err, syscall.ENAMETOOLONG)` arm in the source error mapping answers `400 invalid_path` instead |
| [#89](https://github.com/davison/md-notes/issues/89) | M5 QA, on [#74](https://github.com/davison/md-notes/issues/74#issuecomment-5702704529) | Two edges in `ui/e2e`: a `drawerReady` helper beside `dialogReady`, the drawer's `Escape` effect attaching a frame late being the same race; and an owner for the standing condition left by the retired `Escape` decision |
| [#91](https://github.com/davison/md-notes/issues/91) | the review of [#86](https://github.com/davison/md-notes/pull/86#issuecomment-5702263505), findings 3 and 6, dispositioned at [#86](https://github.com/davison/md-notes/pull/86#issuecomment-5702824030) | The note bar still scrolls sideways under an unbreakable long failure message — 479 px in a 320 px bar after #86, against 558 px before, so improved and not fixed — and `ui/e2e/create-delete.test.mjs` passes a `name` field to `browser.newContext`, which Playwright ignores |

## Known gaps at the boundary

All three captures this milestone's tasks adopted are closed. What remains true and will
surprise someone who has not read this far:

| Gap | Where it is recorded |
|-----|----------------------|
| Three refusal codes are still diagnosis-only wrong on paths that are already refused — a dangling directory link out of the root, a dangling two-hop chain, and a `true == false` literal in a test table | [#82](https://github.com/davison/md-notes/issues/82) |
| `ui/src/note-view.test.tsx` can fail `make check` on a teardown race it does not own; six of seven runs were clean and the file was untouched by the PR that surfaced it | [#87](https://github.com/davison/md-notes/issues/87) |
| **Renaming a note is still not in the application at all.** M5 delivered two of the three verbs the README used to defer to other tools; renaming was never in scope and nothing in the milestone weighs it | [#74](https://github.com/davison/md-notes/issues/74), [the README](../../README.md) |
| The `Escape`-stops-propagation rule is held by unit tests only. It is genuinely held — the capture-phase `document` listener in `ui/src/dialog.test.tsx` asserts the effect, not the call — but no browser check covers it, because after #85 no layer a reader can reach sits under a dialog | [#78](https://github.com/davison/md-notes/issues/78#issuecomment-5702378000), [#84](https://github.com/davison/md-notes/pull/84#issuecomment-5702513643) |
| `ui/e2e/create-delete.test.mjs` hands `{ name, viewport, hasTouch, isMobile }` straight to `browser.newContext(layout)`; `name` is not a context option and Playwright tolerates it today | [#91](https://github.com/davison/md-notes/issues/91), from [#86](https://github.com/davison/md-notes/pull/86#issuecomment-5702263505) finding 6 |
| A too-long note name is refused `500 io_error` and logged as a server fault, on the save path as much as on create; the reader is told "could not read or save note" and not why. Not a breach of M5-R2, because the save answers the same way, but create is the first UI path that lets a reader reach it | [#88](https://github.com/davison/md-notes/issues/88), [#74](https://github.com/davison/md-notes/issues/74#issuecomment-5702723351) |
| The drawer's `Escape` effect attaches a frame after its element is in the page — the race `dialogReady` already blunts for dialogs. The shipped `layout.test.mjs` waits correctly; the next drawer check written will meet it | [#89](https://github.com/davison/md-notes/issues/89) |
| `min-width: 0` on `.save-status` does not settle what the record says it settles. Measured with an unbreakable long failure message: the note bar's `scrollWidth` at 320 px falls from 558 to 479, and at 1280 px the delete button's right edge is at 995 either way — outside the pane. There is no `overflow-wrap` or `overflow: hidden` on `.save-status`, so the box may shrink but its text does not. Pre-existing and improved rather than fixed | [#91](https://github.com/davison/md-notes/issues/91), from [#86](https://github.com/davison/md-notes/pull/86#issuecomment-5702263505) finding 3 |
| The new unit case in `ui/src/note-pane.test.tsx` for the delete button's position has no teeth of its own: it passes unchanged against `main`, because the DOM order never changed and the bug was purely CSS. The browser check is the only thing standing between that bug and a repeat | [#86](https://github.com/davison/md-notes/pull/86#issuecomment-5702263505), finding 5 |
| The e2e suite runs in Chromium only, and the iPhone 14 profile is its viewport in Chromium rather than in WebKit. Chromium is the one browser CI downloads, and it is what M4 measured in | [#78](https://github.com/davison/md-notes/issues/78#issuecomment-5701667426) |
| The suite's device profiles are frozen literals and deliberately no longer track Playwright's device registry, so a real device whose viewport changes will not be noticed by a dependency bump — that is a decision to retake the numbers | [#78](https://github.com/davison/md-notes/issues/78#issuecomment-5701667426) |
| `mdn open ~/some/project` now admits `DELETE` as well as `PUT` for the markdown under it, for as long as that root is registered, and over the tailnet as well as on loopback. The operator was asked twice and has not answered; the default stands on the coordinator's authority, and **no `cc:needs-decision` gate was raised**, which is the stricter form this should have taken | [#76](https://github.com/davison/md-notes/issues/76#issuecomment-5700724927), [#76](https://github.com/davison/md-notes/issues/76#issuecomment-5702824267) |
| M5-R2's text on [#74](https://github.com/davison/md-notes/issues/74) still says the create control is "in the navigator". It is in the top bar, by the operator's own finding, and the requirement is deliberately unedited | [#74](https://github.com/davison/md-notes/issues/74#issuecomment-5702275665) |

## Where the record is silent

- **Four of this milestone's silences were closed by writing them down after the fact,
  and that is itself the finding.** The scope and sequencing decision
  ([#74](https://github.com/davison/md-notes/issues/74#issuecomment-5702824535)), the
  standing of the unanswered ask-the-human point
  ([#76](https://github.com/davison/md-notes/issues/76#issuecomment-5702824267)) and the
  disposition of PR #86's six review findings
  ([#86](https://github.com/davison/md-notes/pull/86#issuecomment-5702824030)) were all
  written after this record's first draft named their absence. Each is now traceable and
  each is honest about being late. What none of them can undo is the order of events: the
  work was sequenced, a judgment call was shipped and six findings were merged past
  *before* the reasoning existed on the record, so nobody reviewing at the time could have
  read it. M4 wrote its scope decision before the work; M5 wrote it afterwards.
- **The ask-the-human point was never raised as a gate.** #76 flagged "create and delete
  accept any registered root" for the operator, the coordinator put it to them twice, and
  no answer came; the default stands on the coordinator's authority under the standing
  merge confirmation. No `cc:needs-decision` was raised at any point, which the decision
  itself names as the stricter form
  ([#76](https://github.com/davison/md-notes/issues/76#issuecomment-5702824267)). A
  judgment call the implementer explicitly wanted a human on was settled by silence and
  then ratified by the same identity that took it.
- **PR [#86](https://github.com/davison/md-notes/pull/86) merged with no fix pass and no
  reply to its review.** Every other PR in this milestone carries an implementer's reply
  naming what it did with each finding. This one carries the approve, the operator's
  confirmation and nothing else, and the disposition arrived after the merge. Two of the
  six are still true in the tree, captured as
  [#91](https://github.com/davison/md-notes/issues/91).
- **Nothing says what the create and delete flows cost the eager bundle beyond the
  measurement.** PR #83 records +1,107 bytes of brotli JavaScript (+7.0%) and +313 bytes
  of CSS (+9.5%) on a first load, reproduced to the byte by its reviewer, and notes the
  editor chunk is unchanged. No budget exists to weigh those against; M4's asset work set
  none, and this milestone did not either.
- **The operator has seen the UI and nothing else.** The operator opened the merged
  application on 2026-09-16 and produced the two findings that became #85. That is the
  only human judgement anywhere in the milestone: every other trade-off here was struck
  between an implementer, a reviewer and a QA session sharing one identity, under the
  standing merge confirmation on [#74](https://github.com/davison/md-notes/issues/74).
  The daemon's refusal set, the tailnet rule, the clock seam and the whole of the browser
  suite were judged by model sessions only — thoroughly, and by nobody who will be
  surprised by them in daily use.
