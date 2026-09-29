# M11 — Images in the editor, and CI that leaves text-only changes alone

Tracking issue: [#213](https://github.com/davison/md-notes/issues/213). Its two implementation
tasks are merged on `main` at [`623758e`](https://github.com/davison/md-notes/commit/623758e).
The milestone was opened at 18:19:51Z on 2026-09-29, and its last implementation task merged
at 22:43:52Z the same day. Both task branches were cut from
[`855ac05`](https://github.com/davison/md-notes/commit/855ac05), which QA used as its "before"
control ([#213](https://github.com/davison/md-notes/issues/213#issuecomment-5900745421)).

Independent QA graded M11-R1, M11-R2 and M11-R3 on `main` at `623758e`. All three were
satisfied, R3 under the docs-only reading the coordinator recorded before QA ran
([#213](https://github.com/davison/md-notes/issues/213#issuecomment-5900454804)). M11-R4 was
untestable, because it includes the roadmap row and this record, which did not yet exist
([#213](https://github.com/davison/md-notes/issues/213#issuecomment-5900745421)). The parts of
M11-R4 that the tasks delivered were checked and found accurate. The pull request that carries
this record delivers the rest, and a superseding verdict is owed after it merges.

## Goal and outcome

The milestone's goal, as #213 states it: an image pasted from the clipboard or dragged in from
a file manager lands in the root's `_resources` directory and is linked from the note, safely
and without ever replacing a file; and CI stops running the test suites for pushes and pull
requests made only of docs and chore commits, while a release always runs every check
([#213](https://github.com/davison/md-notes/issues/213)).

The scope was the operator's, given in the coordinator session and relayed by the
coordinator: "open a milestone for #208 and #212"
([#213](https://github.com/davison/md-notes/issues/213#issuecomment-5900867536)). The scope Decision records it as "exactly two captures"
([#213](https://github.com/davison/md-notes/issues/213#issuecomment-5897063989)). Both captures
carry the operator's ask as the coordinator relayed it. [#212](https://github.com/davison/md-notes/issues/212):
"allow an image to be pasted from the clipboard or dragged into the editor from a file
manager. In both cases, the image should be copied to the `_resources` directory and referenced
in there by the markdown." [#208](https://github.com/davison/md-notes/issues/208): "skip tests
on `docs:` and `chore:` commits. They should still run on a release".

What a reader has now:

- **Paste or drop an image into the editor.** The file is copied into the one `_resources`
  directory at the top of the root and linked from the note with a relative link that climbs
  to it (`../../_resources/x.png` from two folders down). A paste is named by its content, and
  a drop keeps its own name, reduced to a safe character set. No existing file is ever
  replaced, and a file of the same name holding the same bytes is reused. PNG, JPEG, GIF, WebP
  and SVG are accepted, judged by content, up to 16 MiB. An uploaded SVG's script cannot run
  from the app's origin. One undo removes every link from a paste or drop, and the files stay.
  A refused or failed upload inserts nothing and says why. A paste or drop with no image in it
  behaves as it did before.
- **An upload that outlives its editor is not lost.** If the reader goes to the reading view
  mid-upload, the link goes into the draft and autosave writes it. If the note is reloaded from
  disk with other text first, nothing is inserted, and the message names the saved file.
- **CI skips its test jobs for docs-only changes.** A push or pull request whose every commit
  has a `docs` subject, scoped and `!` forms included, skips `check` and `e2e`, and they show as
  skipped. One commit of any other type runs both, and that includes `chore`. A release, a
  `workflow_call` and a manual run always run every check. A gate that fails, or cannot be
  built, skips nothing.
- **The commit type is now a rule for two seats.** The implementer's and the reviewer's
  project extensions say when `docs` is the right type, and the reviewer treats a `docs` commit
  that changes anything the tests check as a blocking finding.

### What shipped, in the order it merged

Each pull request landed on `main` by rebase, so the commit on `main` differs from the head
the last review saw. Both are given.

| Task | Requirement | Adopts | Pull request | Approved at | Last head | On `main` as | Merged |
|------|-------------|--------|--------------|-------------|-----------|--------------|--------|
| [#214](https://github.com/davison/md-notes/issues/214) images in the editor | M11-R1, M11-R2, M11-R4 (part) | #212, and #220 by Decision | [PR #219](https://github.com/davison/md-notes/pull/219), two rounds | `909a6a1` | `0115d53` | [`e3ca09d`](https://github.com/davison/md-notes/commit/e3ca09d) | 20:03:49Z |
| [#215](https://github.com/davison/md-notes/issues/215) CI skips docs-only changes | M11-R3, M11-R4 (part) | #208 | [PR #218](https://github.com/davison/md-notes/pull/218), two rounds | `c2d04dd` | `28d93d7` | [`623758e`](https://github.com/davison/md-notes/commit/623758e) | 22:43:52Z |

Both pull requests merged with one commit added after the approval, which no later review
comment covers. Each was taken from its approving review's nit list. On PR #219 it was round
two's only nit, a test failure message that named the wrong error: `0115d53` on the branch, [`e3ca09d`](https://github.com/davison/md-notes/commit/e3ca09d)
on `main` ([PR #219](https://github.com/davison/md-notes/pull/219#issuecomment-5897671117)). On
PR #218 it was round two's nit 2, a one-word change to a role file: `refactor` added to the
implementer rule's list of types, so that it matches the reviewer's: `28d93d7` on the branch,
[`623758e`](https://github.com/davison/md-notes/commit/623758e) on `main`
([PR #218](https://github.com/davison/md-notes/pull/218#issuecomment-5900450086)).

The two tasks touched no file in common, and both were started in parallel, #215 at 19:24:28Z
and #214 at 19:26:22Z ([#215](https://github.com/davison/md-notes/issues/215#issuecomment-5897098703),
[#214](https://github.com/davison/md-notes/issues/214#issuecomment-5897128107)). #214 merged 22
minutes after its pull request opened. #215 waited on its second gate from 19:48:58Z to
22:32:24Z.

## Requirement outcomes

The verdicts are from QA's comment on the milestone issue
([#213](https://github.com/davison/md-notes/issues/213#issuecomment-5900745421)). QA tested a
detached worktree of `623758e` with its own daemons, each with a temporary `--root`, `--state`,
`--token-file` and `--config`, and a second daemon built from `855ac05` for the "behaves as
today" checks. Pastes were real clipboard pastes, and drops were real browser drags of files
from disk through CDP, not synthetic DOM events. The floor under the verdicts: `make check`
passed (Go, 371 UI and 226 extension unit tests), and the `ui/e2e` suite passed 120 of 120.

| ID | Requirement | What the work established | QA |
|----|-------------|---------------------------|----|
| M11-R1 | A pasted or dropped image is copied into the root's top-level `_resources`, never replacing a file, and the note gets a relative link at the cursor or drop point that renders from any depth; several dropped files give one link each; one undo removes the inserted link | `POST /api/r/{slug}/resources`, a store that creates every candidate name with `O_CREATE\|O_EXCL`, and editor handlers that insert every link in one transaction marked `isolateHistory` ([#214](https://github.com/davison/md-notes/issues/214), [PR #219](https://github.com/davison/md-notes/pull/219)). The renderer needed no change, and a test pins its rewrite from depths 0 to 3 | [Satisfied](https://github.com/davison/md-notes/issues/213#issuecomment-5900745421): a paste at the root and two folders down linked one hash-named file. A three-file drop beside an existing `shot.png` made `shot-2.png` and left the original's hash unchanged. One vim `u` removed all three links and kept the files, and the links rendered from depths 0 to 2 |
| M11-R2 | Upload is confined and safe: a documented set of types checked by content, a documented size cap, writes only inside `_resources` behind the same checks as every other write, no script from the UI's origin, a message and no insert on failure, and non-image paste or drop unchanged | Types by magic bytes, and SVG by namespace; a 16 MiB cap; `_resources` opened through the root handle; the tailnet admits the POST behind the session and Origin rule; the raw route's existing `Content-Security-Policy: sandbox` and `nosniff` neutralise an SVG ([#214](https://github.com/davison/md-notes/issues/214#issuecomment-5897126264), [#214](https://github.com/davison/md-notes/issues/214#issuecomment-5897126600)) | [Satisfied](https://github.com/davison/md-notes/issues/213#issuecomment-5900745421): five types accepted and eleven kinds of impostor refused with 415. 16 MiB accepted and one byte more refused with 413. Five traversal names all landed inside `_resources`. 20 racing uploads under one name got 20 names. The guard outcome matched a note create in all 20 conditions. An SVG with a `<script>` ran nothing, opened directly or in an `<iframe>`. Non-image pastes and drops were identical to `855ac05` |
| M11-R3 | CI skips its test jobs for a push or pull request whose commits are all docs (or chore) commits, under the rule the plan settles; one commit of any other type runs everything; a release, a `workflow_call` or a manual run always runs every check, held by a workflow test | Subject only, narrowed to `docs` only by the operator's second gate resolution ([#215](https://github.com/davison/md-notes/issues/215#issuecomment-5900356151)), and carried as docs-only by Decision ([#213](https://github.com/davison/md-notes/issues/213#issuecomment-5900454804)). A `gate` job runs `scripts/cigate`, and `scripts/workflows/ci_release_path_test.go` holds the wiring ([PR #218](https://github.com/davison/md-notes/pull/218)) | [Satisfied](https://github.com/davison/md-notes/issues/213#issuecomment-5900745421), judged docs-only: on scratch pull requests and branches, a docs-only PR and a docs-only push skipped `check` and `e2e`, and one `chore:` commit, a mixed PR, a manual run and a `workflow_call` ran them. The workflow test failed under ten mutations of the release path, and one it missed is capture #226 |
| M11-R4 | Every change described where a user or contributor looks for it (the introduction's editor section, the man page, CONTRIBUTING and `docs/releasing.md`), the roadmap row, and this record | The tasks wrote the introduction's *Pasting and dropping images* and *Uploading an image*, the man page's DESCRIPTION and FILES entries, CONTRIBUTING's Testing and Commit messages, and `docs/releasing.md` step 1. This record's pull request adds the roadmap row, the record, and the corrections under [The front door](#the-front-door-in-this-pull-request) | [Untestable at the verdict](https://github.com/davison/md-notes/issues/213#issuecomment-5900745421): the roadmap row and the record did not exist. The parts that did were checked against QA's own measurements and found accurate. A superseding verdict is owed once this record merges |

## The human gates

Two gates were raised in the milestone, both on #215. #214's plan listed no ask-the-human
points, and recorded its three open choices as Decisions instead
([#214](https://github.com/davison/md-notes/issues/214#issuecomment-5897125631)).

**Which rule skips the tests, on #215**
([#215](https://github.com/davison/md-notes/issues/215#issuecomment-5897096721)), raised at
19:24:20Z. The plan measured the rule against the 575 commits on `main`. 220 were `docs` or
`chore`, scoped forms included, and 39 of those (18%) touched something outside `*.md`,
`docs/` and images. 27 of the 39 touched a file a test, a build or a package consumes: the man
pages, the Makefile, workflows including `release.yml`, packaging files, `go.mod`, and test
files ([#215](https://github.com/davison/md-notes/issues/215)). The options were:

- (A) subject only, as asked: skips all 220, including the 39;
- (B) subject plus a path check: skip only when every commit is `docs` or `chore` and every
  changed file is `*.md`, `docs/**` or an image, which skips 181 of 220 and misses none;
- (C) `paths-ignore` alone: no convention, and the whole run disappears rather than showing
  skipped jobs.

The recommendation was B. The resolution, at 19:35:04Z, reads in full
([#215](https://github.com/davison/md-notes/issues/215#issuecomment-5897261868)):

> Option A. The implementer should be careful about the use of conventional commits. This is a
> local preference so the local role file for that seat should include clear instructions on
> the use of them. If the change impacts tests, or it's desirable to run tests anyway, it should
> be a `fix():` commit
>
> Option B fails for files like `.gitignore`

**Option A against SPEC §4, on #215**
([#215](https://github.com/davison/md-notes/issues/215#issuecomment-5897482847)), raised at
19:48:58Z from finding 2 of the first review of PR #218
([PR #218](https://github.com/davison/md-notes/pull/218#issuecomment-5897472508)). SPEC §4 and
the implementer contract require housekeeping (a dependency bump, a formatter's or linter's fix,
`roles sync`, `migrate`) to be a `chore:` commit. Under option A every `chore:` skipped CI, so a
`go.mod` bump or a formatter's rewrite of code would merge untested. The new reviewer rule, which
made a `chore` touching tested files a blocking finding, would have blocked exactly the commits
the protocol mandates, and a local role file cannot override the contract. The options:

- (a), recommended: skip on `docs:` only, and `chore:` always runs the tests. Still subject only,
  at a small cost: 7 of the 220 past `docs`/`chore` commits were `chore`;
- (b) keep `chore:` skipping, but run the tests for named housekeeping scopes such as
  `chore(deps)`, which fails for a tool that writes a plain `chore:`;
- (c) keep A, and exempt SPEC §4 housekeeping from the reviewer rule, accepting that dependency
  bumps and formatter fixes skip CI.

The resolution, at 22:32:24Z, reads in full "(a) skip on docs only"
([#215](https://github.com/davison/md-notes/issues/215#issuecomment-5900356151)).

**How A was narrowed to docs-only.** The second resolution narrowed the rule, not the intent:
still subject only, with no path list, but `chore` now runs the tests. The fix landed as
`044b536` and `c2d04dd` on the branch, and the round-two review fed `chore:` and `chore(deps):`
subjects to the built `cigate` and got `run=true` for both
([PR #218](https://github.com/davison/md-notes/pull/218#issuecomment-5900450086)). That review's
nit 1 found that M11-R3's text still said "docs or chore commits", and that #215's Goal still
described the old rule. The coordinator then recorded that M11-R3 is carried as docs-only, and
QA judged it that way
([#213](https://github.com/davison/md-notes/issues/213#issuecomment-5900454804)). #215's Goal and
its ask-the-human lines were edited to say so; the milestone body was left as written, with that
Decision as its annotation.

Every seat in this project posts under the one account, so the account alone does not say who
typed a comment. The coordinator recorded afterwards that the operator typed both resolution
comments himself on GitHub, and removed the `cc:needs-decision` label each time, and that he
confirmed each in the coordinator session: "#215 resolved", and "forgot to submit the comment.
There now (option a)" ([#213](https://github.com/davison/md-notes/issues/213#issuecomment-5900867536)).

## Decisions

### Scope, sequencing and merge authority

The operator asked for #212 and #208, carried by #214 (M11-R1, R2 and R4) and #215 (M11-R3 and
R4), with M11-R4's docs split between them and the record left to this housekeeping pull request
([#213](https://github.com/davison/md-notes/issues/213#issuecomment-5897063989)). The two tasks
touch disjoint files, so both were dispatched at once. The coordinator records that the operator
said to run the loop without stopping unless a `cc:needs-decision` gate blocks, and took that as
a standing merge confirmation for the milestone's tasks and its record. That instruction was
given in the coordinator session and relayed by the coordinator ([#213](https://github.com/davison/md-notes/issues/213#issuecomment-5900867536)).

At opening, `govulncheck` was clean, and `pnpm outdated` in `ui/` showed six patch releases,
none security-relevant, none in scope. None was taken. **Left out on purpose:** #107, #108, #110,
#111, #122, #127, #145, #149 and #207. **Rejected:** folding in #207 (search context repeats),
because it is a separate area and the operator named only two captures.

### Naming an uploaded image

A paste, which has no useful name, is named `<first 32 hex digits of SHA-256>.<ext>`. A drop
keeps its base name, reduced to `[A-Za-z0-9._-]`, with the daemon's extension for the sniffed
type, and takes `-2`, `-3` and so on when the name is taken by a different file. A candidate name
already holding exactly the same bytes is reused and nothing is written
([#214](https://github.com/davison/md-notes/issues/214#issuecomment-5897125963)). Both shapes
match what `~/notes/_resources` already holds: 32-hex Joplin names and original names like
`20231007_190002.jpg`. Hashing gives a paste a stable name and removes the duplicate when the
same screenshot is pasted twice.

**Trade-off:** a dropped file's non-ASCII characters are lost, though the alt text keeps the
original name.

**Rejected:** a hash for every file, which throws away the name the operator uses to find
photos; timestamps for pastes, which clash within a second and store a repeated image twice;
and verbatim names, which need escaping in the link and normalise differently on the phone and
the desktop.

Reuse holds only at the name a drop would take. The same image dropped under another name is
copied again, which the first review of PR #219 measured, and the man page was corrected to say
so ([PR #219](https://github.com/davison/md-notes/pull/219#issuecomment-5897496416), finding 5).

### SVG is allowed, and neutralised by the raw route's headers

SVG is on the list, checked by content: valid UTF-8 whose first element is `<svg>` in the SVG
namespace ([#214](https://github.com/davison/md-notes/issues/214#issuecomment-5897126264)). The
raw route already serves every file with `Content-Security-Policy: sandbox` and
`X-Content-Type-Options: nosniff`. In the reading view an SVG is an `<img>`, where its script never
runs, and opened directly the sandbox gives it an opaque origin with scripts disabled. The notes
already hold one `.svg`, so leaving SVG off would take away something they already do, and the
protection is the one every raw file already gets.

**Rejected:** leaving SVG off; and `Content-Disposition: attachment` for SVG, which breaks
opening the image in a new tab and adds nothing the sandbox does not give.

The first review of PR #219 judged this a sound engineering default rather than an operator's
call. It also noted that an SVG that declares entities is refused, because `encoding/xml` is
strict, so some Illustrator exports would be refused
([PR #219](https://github.com/davison/md-notes/pull/219#issuecomment-5897496416)).

### The upload is reachable over the tailnet

Under the tailnet name, `remoteAllowed` admits `POST /api/r/{slug}/resources` alongside the
source writes, behind the same session-cookie and Origin rule
([#214](https://github.com/davison/md-notes/issues/214#issuecomment-5897126600)). The upload is
narrower than a source create: the daemon picks the directory and the name. **Rejected:**
loopback only, which would make paste work in the desktop editor and fail on the phone over the
same session.

### An upload that outlives its editor, and capture #220

The plan said nothing about an upload that finishes after its editor is gone. The implementer's
first report raised capture [#220](https://github.com/davison/md-notes/issues/220) for the
reading-view case, and the first review of PR #219 found the related case of an editor rebuilt
mid-upload, which lost the link and left *Adding the image…* on screen for good
([PR #219](https://github.com/davison/md-notes/pull/219#issuecomment-5897496416), finding 1). The
fix is recorded as a Deviation on #214
([#214](https://github.com/davison/md-notes/issues/214#issuecomment-5897588261)):

- a reader who switched to the reading view gets the link in the parked editor state and the
  draft, autosave writes it, and one undo removes it when the editor reopens;
- a draft reloaded from disk with different text meanwhile gets nothing, and the alert names
  the saved file and says to paste or drop it again.

The round-two review checked both through the real paths, and in a real browser for the
reading-view case, and said #220 was covered
([PR #219](https://github.com/davison/md-notes/pull/219#issuecomment-5897671117)). #220 was not
under #214's `## Adopts`, because it was filed after #214 was created, so `task finish` did not
close it. The coordinator closed it as delivered by #214
([#213](https://github.com/davison/md-notes/issues/213#issuecomment-5897721916)). **Rejected:**
leaving #220 open for a later milestone, since its Want is met as written.

### The skip rule: subject only, then docs only

The rule came through the two gates above. How #215 carried the first resolution
([#215](https://github.com/davison/md-notes/issues/215#issuecomment-5897269136)):

- CI skips `check` and `e2e` when every commit in the push or pull request qualifies by its
  subject, with no path check. `workflow_call` and `workflow_dispatch` always run everything.
- The commit type carries the weight, so the rule is written into
  `.codecrew/roles/implementer.local.md`, this project's append-only extension, and into
  CONTRIBUTING and `docs/releasing.md` for human contributors.
- **Rejected:** B, a path allow-list that misses `.gitignore`-style files (the operator's call);
  C, `paths-ignore`, which has the same weakness and makes the run vanish instead of showing
  skipped jobs; and putting the rule in `implementer.md`, which is CodeCrew's contract text and
  would become a fork that `roles sync` refuses.

A matching reviewer check was first "not taken", and then added on the operator's ask, given in
the coordinator session and relayed by the coordinator as "add the reviewer check too"
([#215](https://github.com/davison/md-notes/issues/215#issuecomment-5897288081),
[#213](https://github.com/davison/md-notes/issues/213#issuecomment-5900867536)). The reviewer
reads every commit's type against its diff, and the two files are worded from the same source so
they cannot drift.

After the second resolution, the rule is `docs` only. `implementer.local.md` says housekeeping
"stays `chore:`, exactly as SPEC section 4 and the contract above require. It always runs CI",
and `reviewer.local.md` says there is "no rule against `chore` beyond the contract's own"
([PR #218](https://github.com/davison/md-notes/pull/218#issuecomment-5900450086)).

**M11-R3 is carried as docs-only**
([#213](https://github.com/davison/md-notes/issues/213#issuecomment-5900454804)). **Rejected:**
editing R3's text in the milestone body, which is not where changes to scope are recorded; and
striking R3 for a new requirement, since the intent is unchanged, only narrowed.

### How the gate tells a release

These choices are in #215's plan and PR #218, not in Decision comments
([#215](https://github.com/davison/md-notes/issues/215), [PR #218](https://github.com/davison/md-notes/pull/218)):

- `scripts/cigate` answers `run=false` only when the workflow is `ci` itself, on a branch, on
  `push` or `pull_request`, with at least one subject and every subject `docs`. Under
  `workflow_call` GitHub reports the caller's workflow name and event, so the workflow and the
  ref type, not the event alone, tell a release. The first review of PR #218 confirmed this
  against GitHub's documentation for reusable workflows.
- Every commit decides, not only the head. Push commits come from `compare/<before>...<after>`,
  not `github.event.commits`, which is capped at 20. Any failure to list them runs everything.
- The test jobs `need` the gate, so a skipped job reports as skipped, not missing. `main`'s
  ruleset has no required status checks, and `task finish` counts a `skipping` check as done.

Two parts of this were changed by the first review of PR #218
([PR #218](https://github.com/davison/md-notes/pull/218#issuecomment-5897472508)):

- **The gate fails open.** The plan gated the tests on `needs.gate.outputs.run == 'true'`. The
  step ran without `pipefail`, so a `cigate` that failed to build left the gate green and the
  output empty, and both test jobs skipped, on a pull request and under a release. The jobs now
  run under `!cancelled() && needs.gate.outputs.run != 'false'`, and the step has
  `shell: bash`. The implementer showed it on GitHub with a scratch PR whose `cigate` would not
  build.
- **Pull request commits come from `compare` too,** with `--paginate`, because `pulls/N/commits`
  lists at most 250. The round-two review showed that a stale base can only add commits, and so
  only turn a skip into a run.

## Deviations and narrowings

**#214: an upload that outlives its editor.** See the decision above
([#214](https://github.com/davison/md-notes/issues/214#issuecomment-5897588261)). PR #219's body
still says "There were no deviations from the plan"; it was written before round one, and the
round-one section added below it describes the fix without retracting that line
([PR #219](https://github.com/davison/md-notes/pull/219)).

**#214 took a capture it did not list.** #220 was delivered under #214 without being in its
`## Adopts`, and was closed by Decision
([#213](https://github.com/davison/md-notes/issues/213#issuecomment-5897721916)).

**#215 was narrowed from docs and chore to docs only.** See [The human gates](#the-human-gates).
It is recorded as a gate resolution and a Decision, not as a Deviation.

**#215's plan text still describes two superseded details.** Its Design and Tests sections say
the test jobs run on `needs.gate.outputs.run == 'true'`, and that PR commits come from
`pulls/N/commits` ([#215](https://github.com/davison/md-notes/issues/215)). Both were changed
after the first review of PR #218, as above. The change is recorded in PR #218's body and in the
round-two review, but no Deviation was posted on #215, and the plan was not edited for it.

**Two history artefacts keep the old rule's words.** PR #218's title, and the commit that
introduced the gate ([`eed22ae`](https://github.com/davison/md-notes/commit/eed22ae)), say
"docs and chore" and "every commit is docs or chore". The merged code skips on `docs` only.

## What the reviews changed

Every implementation pull request was reviewed by a clean-context session under the reviewer
contract, with the reviewer seat routed to the operator. Each merged under the operator's standing
confirmation. Its operator-confirmation comment was posted by `gh codecrew task finish
--operator-confirm`, resting on that standing confirmation ([#213](https://github.com/davison/md-notes/issues/213#issuecomment-5900867536))
([PR #219](https://github.com/davison/md-notes/pull/219#issuecomment-5897717165),
[PR #218](https://github.com/davison/md-notes/pull/218#issuecomment-5900489850)).

**[PR #219](https://github.com/davison/md-notes/pull/219), two rounds.**

- **Round one** ([changes requested](https://github.com/davison/md-notes/pull/219#issuecomment-5897496416)),
  at `69e783e`: one blocking finding. An editor rebuilt while an upload was in flight lost the
  link without a message, and left *Adding the image…* on screen. The nits: no test pinned
  `isolateHistory` (the test typed before the paste, where CodeMirror never joins events); the
  non-image drop test asserted nothing; `IsRegular()` in the reuse check was unpinned; the man
  page overstated reuse for drops; and a dangling `_resources` link inside the root answered
  `500 io_error` about a note. The reviewer also proved the SVG sandbox in a real browser, ran a
  `_resources` swap race over 400 uploads with nothing written outside, and checked auth parity.
- The fix pass retyped the man-page commit as `feat(docs)` under #215's new commit-type rule,
  and force-pushed the branch with a lease
  ([PR #219](https://github.com/davison/md-notes/pull/219)).
- **Round two** [approved](https://github.com/davison/md-notes/pull/219#issuecomment-5897671117),
  at `909a6a1`, after reverting each fix and watching its test fail. One nit: a failure message
  named the wrong error. The review also checked every commit's type against the new rule, and
  found none typed `docs` or `chore`.

**[PR #218](https://github.com/davison/md-notes/pull/218), two rounds.**

- **Round one** ([changes requested](https://github.com/davison/md-notes/pull/218#issuecomment-5897472508)),
  at `e6114fc`: two blocking findings. A gate step that failed skipped the tests and showed green,
  on a pull request and under a release. The role rules contradicted SPEC §4 on housekeeping,
  which became the second gate. The nits: two comments named a test file that does not exist, and
  a pull request with more than 250 commits was decided on its first 250.
- **Round two** [approved](https://github.com/davison/md-notes/pull/218#issuecomment-5900450086),
  at `c2d04dd`, with every round-one finding resolved and two nits: M11-R3's text still said
  "docs or chore" (answered by the coordinator's Decision on #213), and the two role files listed
  different "truer" types (fixed in `28d93d7`).

No coordinator disposition comment covers either review; the fixes are in each PR body's
round-one section, and the #220 and M11-R3 Decisions on #213 answer the items that were not code.

## QA's observations, and what was done

QA's three observations were outside the verdicts
([#213](https://github.com/davison/md-notes/issues/213#issuecomment-5900745611)), and the
coordinator disposed of them
([#213](https://github.com/davison/md-notes/issues/213#issuecomment-5900761005)):

1. **A flaky e2e test that predates M11.** "recreates a note deleted under another tab's draft,
   from the banner" failed once on a manual run of a branch that differed from `main` only by a
   markdown file, and passed on rerun. Because `release.yml` calls `ci.yml`, a flake there can
   turn a release red. Captured as [#225](https://github.com/davison/md-notes/issues/225).
2. **The release-path workflow test checks for substrings.** Appending `-workflow ci -ref-type
   branch` after the real flags passes the test, and Go's `flag` takes the last value, so
   `cigate` would answer `run=false` for a release. A real release is safe today, because a new
   tag's `before` is all zeros, so no subjects are listed and everything runs. QA's other ten
   mutations were caught. Captured as [#226](https://github.com/davison/md-notes/issues/226), not
   a remedy task, since the gap is in the test's strength, not the behaviour.
3. **An inserted link runs into the text beside it.** A drop at the start of a line gave
   `![shot](../../_resources/shot-2.png)gamma`, and a paste at the end of a line gave
   `first line![](_resources/….png)`. The introduction said several dropped files give "one link
   each, one per line", which is true between the links but not for the last one. The behaviour
   change is captured as [#227](https://github.com/davison/md-notes/issues/227). The wording is
   corrected in this record's pull request: the introduction now says the links are separated by
   line breaks and that nothing is added before the first or after the last, so a link lands
   beside any text at that point.

**Rejected:** remedy tasks for 2 or 3 inside M11, since neither fails a requirement and the
operator's scope was the two captures.

## The front door in this pull request

The claims M11 changed in the front-door documents, brought into line with `623758e`:

- **`docs/introduction.md`.** The opening now describes the project at the end of milestone
  eleven, and a paragraph says what M11 added. *Pasting and dropping images* no longer says the
  links go "one per line" (QA's observation 3). *What holds these numbers* said the suites run
  on "every push"; a docs-only push or pull request now skips them. Its count of browser checks
  went from 116 to 120, the count QA ran.
- **`README.md`.** The editor bullet says an image can be pasted or dropped into it.
- **`CONTRIBUTING.md`.** The `make e2e` paragraph lists the images suite, and its count went
  from ten suites and 116 tests to eleven and 120.

`contrib/mdn.1`, CONTRIBUTING's Testing and Commit messages sections, and `docs/releasing.md`
were brought up to date by the tasks themselves, and QA checked them. No other guide makes a
claim M11 changed.

## Captures adopted, and captures raised

M11 adopted two captures, each closed by its task's `task finish`:
[#212](https://github.com/davison/md-notes/issues/212#issuecomment-5897718766) by #214 and
[#208](https://github.com/davison/md-notes/issues/208#issuecomment-5900491021) by #215.

It raised four:

- [#220](https://github.com/davison/md-notes/issues/220), an upload that finishes after the
  editor closes, raised at 19:42:29Z from the #214 implementer's report, delivered by #214 and
  closed by Decision;
- [#225](https://github.com/davison/md-notes/issues/225), the flaky create-delete e2e test;
- [#226](https://github.com/davison/md-notes/issues/226), the release-path test and a repeated
  `cigate` flag;
- [#227](https://github.com/davison/md-notes/issues/227), an inserted image link running into the
  text beside it.

The last three were raised at 23:05Z from QA's observations and stay open. No review of either
pull request proposed a capture. The items left out of scope, listed under
[Scope](#scope-sequencing-and-merge-authority), stay open in the backlog.

## Corrections to the record itself

**#214, the naming trade-off.** Its example gave `Café.png` → `Caf-.png`. The first review of
PR #219 measured `Caf.png` and called the Decision text "slightly off"
([PR #219](https://github.com/davison/md-notes/pull/219#issuecomment-5897496416)). The
correction on #214 says a run of disallowed characters at the end of the stem is trimmed rather
than replaced, and that the rule is otherwise as stated
([#214](https://github.com/davison/md-notes/issues/214#issuecomment-5900867770)).

**Where the operator's words came from.** Posted after QA, for this record
([#213](https://github.com/davison/md-notes/issues/213#issuecomment-5900867536)): see
[The human gates](#the-human-gates) and [Where the record is silent](#where-the-record-is-silent).

## Where the record is silent

**The operator's session words have no source on the trail.** The coordinator's attribution
comment says which words the operator typed on GitHub (both #215 gate resolutions and the label
removals) and which it relayed from the session: the scope, the standing merge confirmation, "add
the reviewer check too", and the capture request behind #212 ([#213](https://github.com/davison/md-notes/issues/213#issuecomment-5900867536)). The relayed words
are quoted as the coordinator gave them; the session they came from is not on GitHub. #208's
words, from 2026-09-22, are relayed in the capture and the comment does not cover them.

**Two approvals did not see their last commits.** See the note under
[What shipped](#what-shipped-in-the-order-it-merged). Both commits were fixes for the approving
review's own nits, and QA's verdicts ran on `main` after both.
