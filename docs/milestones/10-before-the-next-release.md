# M10 — Before the next release: diagrams that read, the bugs that bite, a README for strangers

Tracking issue: [#186](https://github.com/davison/md-notes/issues/186). Its nine
implementation tasks are merged on `main` at
[`53e6bed`](https://github.com/davison/md-notes/commit/53e6bed). The milestone was opened at
08:16:22Z on 2026-09-22, on [`c4ab83e`](https://github.com/davison/md-notes/commit/c4ab83e),
and its last implementation task merged at 11:33:13Z the same day.

Independent QA graded M10-R1 to M10-R7 and M10-R9 on `main` at
[`b8e64b5`](https://github.com/davison/md-notes/commit/b8e64b5), and M10-R8 on `53e6bed`
after #194 merged. All nine were satisfied. M10-R10 was untestable, because this task had not
merged ([#186](https://github.com/davison/md-notes/issues/186#issuecomment-5775893446)). M10-R10
is delivered by the pull request that carries this record, and a superseding verdict is owed
after it merges.

The milestone ends ready to tag. Cutting the tag is the operator's, and it is not a
requirement ([#186](https://github.com/davison/md-notes/issues/186#issuecomment-5773313629)).

## Goal and outcome

On 2026-09-22 the operator asked for a pre-release milestone: the key bug fixes from the
backlog, documentation polish, and the diagram captures. The operator also asked for a new
capture, #185, to cut the README down to what the project is and why a person would care. The
coordinator's scope decision records the ask and the plan
([#186](https://github.com/davison/md-notes/issues/186#issuecomment-5773313629)). #185 carries
the operator's words as the coordinator relayed them: shorten it "to include only what the
project is and why a human user would care. Include the screenshots we have and talk about
the diagrams. Everything else from the README can be moved to new files in `/docs` or
`CONTRIBUTING.md` as appropriate"
([#185](https://github.com/davison/md-notes/issues/185)).

The milestone's goal, as #186 states it: flowcharts that stay legible and route cleanly, the
backlog bugs a user can actually hit, CI and packaging that survive the runner change due on
2026-10-19, and a README that tells a stranger what md-notes is, why they would want it and
what it looks like, with the reference material moved to `docs/` and CONTRIBUTING.md.

What a reader has now:

- **Flowcharts read and route better.** A wide diagram is never shown with labels under 12 px.
  Below that scale it scrolls in a box of its own, and any diagram opens at natural size on a
  click, a tap or Enter. A link that points back runs in a lane outside the nodes, the main
  chain is drawn straight, and a subgraph's title is kept clear of links. No palette draws a
  box behind the drawing. A drawing refused at the 2 s deadline on a busy host is tried again
  after a minute, backing off to an hour, instead of staying code until the daemon restarts.
- **A note with thousands of tagged code blocks renders in bounded time.** 20,000 `mermaid`
  blocks took 108.2 s through the daemon on `c4ab83e`, and take 0.20 s now
  ([PR #199](https://github.com/davison/md-notes/pull/199)).
- **A repeated `--root` is never half-ignored.** Every `--root` is served, `notes_root` takes a
  list, and the roots after the first are a new `permanent` kind that the home page cannot
  remove.
- **Three editor and navigator bugs are gone.** A clean editor whose file is deleted says
  `Deleted on disk`, not the unsaved-edits conflict. Recreate brings the vim mode back. A stale
  roots listing no longer lands on the root the reader moved to.
- **CI and packaging.** Every action is on its Node 24 major, and every job runs on
  `ubuntu-24.04`. The manual page is installed by the `.deb`, the AUR package and
  `make install`. The Go and Node that build a release are pinned in the tree, and
  [Checking a release](../releasing.md#checking-a-release) says how to rebuild a release to
  the same bytes.
- **Documentation.** The README is a front page for strangers, with generated screenshots.
  Installing and running are on pages of their own
  ([Installing md-notes](../install.md), [Running md-notes](../running.md)), and the
  extension has a [Privacy](../privacy.md) page and a justification for each permission. The
  browser suites skip without Chromium as documented, and fail under CI instead.

### What shipped, in the order it merged

Each pull request landed on `main` by rebase, so the commit on `main` differs from the head
the last review saw. Both are given.

| Task | Requirement | Adopts | Pull request | Approved at | On `main` as | Merged |
|------|-------------|--------|--------------|-------------|--------------|--------|
| [#191](https://github.com/davison/md-notes/issues/191) several permanent roots | M10-R5 | #162, #155, #125 | [PR #198](https://github.com/davison/md-notes/pull/198), three rounds | `a0cd48d` | [`a77e7f7`](https://github.com/davison/md-notes/commit/a77e7f7) | 08:57:24Z |
| [#193](https://github.com/davison/md-notes/issues/193) CI and packaging | M10-R7 | #161, #168, #167 | [PR #197](https://github.com/davison/md-notes/pull/197), two rounds | `a09b28a` | [`77d7cc7`](https://github.com/davison/md-notes/commit/77d7cc7) | 09:06:13Z |
| [#188](https://github.com/davison/md-notes/issues/188) flowchart routing | M10-R2 | #184, #174 | [PR #200](https://github.com/davison/md-notes/pull/200), two rounds | `200cd44` | [`8625a4e`](https://github.com/davison/md-notes/commit/8625a4e) | 09:41:06Z |
| [#192](https://github.com/davison/md-notes/issues/192) editor and navigator state | M10-R6 | #30, #112, #123 | [PR #201](https://github.com/davison/md-notes/pull/201), one round | `7247c5c` | [`ed08de1`](https://github.com/davison/md-notes/commit/ed08de1) | 10:09:35Z |
| [#190](https://github.com/davison/md-notes/issues/190) bounded render of tagged code | M10-R4 | #178 | [PR #199](https://github.com/davison/md-notes/pull/199), two rounds | `ea9714f` | [`bc641d6`](https://github.com/davison/md-notes/commit/bc641d6) | 10:13:09Z |
| [#187](https://github.com/davison/md-notes/issues/187) wide flowcharts | M10-R1 | #183 | [PR #202](https://github.com/davison/md-notes/pull/202), three rounds | `bdab2dd` | [`df276bd`](https://github.com/davison/md-notes/commit/df276bd) | 10:48:08Z |
| [#195](https://github.com/davison/md-notes/issues/195) privacy page, the Chromium claim | M10-R9 | #160, #153 | [PR #203](https://github.com/davison/md-notes/pull/203), two rounds | `d404a18` | [`4c394fe`](https://github.com/davison/md-notes/commit/4c394fe) | 10:54:17Z |
| [#189](https://github.com/davison/md-notes/issues/189) diagram edge cases | M10-R3 | #180, #182 | [PR #204](https://github.com/davison/md-notes/pull/204), two rounds | `54a2116` | [`b8e64b5`](https://github.com/davison/md-notes/commit/b8e64b5) | 11:25:47Z |
| [#194](https://github.com/davison/md-notes/issues/194) the README | M10-R8 | #185 | [PR #205](https://github.com/davison/md-notes/pull/205), two rounds | `11c439f` | [`53e6bed`](https://github.com/davison/md-notes/commit/53e6bed) | 11:33:13Z |
| [#196](https://github.com/davison/md-notes/issues/196) docs and this record | M10-R10 | — | [PR #206](https://github.com/davison/md-notes/pull/206) | | | |

Two pull requests merged with commits added after their last approval, which no later review
comment covers. On PR #201 these were the fixes for the approval's nits 1 and 2, including the
unit test that pins the Ctrl+E behaviour: `17fd406` and `0993e09` on the branch,
[`3787397`](https://github.com/davison/md-notes/commit/3787397) and
[`ed08de1`](https://github.com/davison/md-notes/commit/ed08de1) on `main`. On PR #203 they were
the fixes for the approval's three nits, and a change to CONTRIBUTING's suite count, to 109
tests, made after the rebase: `c086418` and `9ac5c03` on the branch,
[`e8d7477`](https://github.com/davison/md-notes/commit/e8d7477) and
[`4c394fe`](https://github.com/davison/md-notes/commit/4c394fe) on `main`
([PR #203](https://github.com/davison/md-notes/pull/203#issuecomment-5775211633)). That rebase
also resolved a conflict in `ui/e2e/diagram.test.mjs`, which no review saw either. The PR #203
approval said its first nit "needs no re-review"
([PR #203](https://github.com/davison/md-notes/pull/203#issuecomment-5775157856)).

## Requirement outcomes

The verdicts are from QA's comment on the milestone issue
([#186](https://github.com/davison/md-notes/issues/186#issuecomment-5775893446)). QA built each
tree in a clean worktree and ran its own daemons, each with a temporary `--config`, `--state`,
`--token-file` and root, on a random port. Its "before" control was a build of `c4ab83e`. The
floor under the verdicts: on `b8e64b5`, `make check` passed (Go, 344 UI and 226 extension unit
tests), `make e2e` passed 116 of 116, and `pnpm --dir extension e2e` passed 29 of 29.

| ID | Requirement | What the work established | QA |
|----|-------------|---------------------------|----|
| M10-R1 | A wide flowchart stays legible: a documented minimum display scale, its own horizontal scroll box below it, no sideways page scroll at any layout, a natural-size view, and a search hit below a diagram still lands | The floor is 12/14 of natural width, so the 14 px labels never show below 12 px ([#187](https://github.com/davison/md-notes/issues/187#issuecomment-5774372931)). The image stands in a `button` inside a `div.diagram-box` that scrolls sideways. The natural-size view is an overlay that follows palette changes and live edits ([PR #202](https://github.com/davison/md-notes/pull/202)) | [Satisfied](https://github.com/davison/md-notes/issues/186#issuecomment-5775893446): every wide diagram showed at 0.857 of natural width in its own box, and `scrollWidth` equalled `clientWidth` in all 16 note and width cases. The box kept within the phone gutter, including inside a list item and a blockquote. The view opened by Enter, Space, click and tap. A horizontal swipe scrolled the box without opening the view, and search hits landed centred |
| M10-R2 | Flowchart edges route cleanly: back edges clear of nodes, the main chain straight, an edge into a nested subgraph not drawn through its title | Back edges run in lanes outside the node band, a longest-chain pass straightens the chain, and titles are placed where no edge crosses them ([#188](https://github.com/davison/md-notes/issues/188#issuecomment-5773589715), [#188](https://github.com/davison/md-notes/issues/188#issuecomment-5773589968)). Held by geometric tests in all four directions | [Satisfied](https://github.com/davison/md-notes/issues/186#issuecomment-5775893446): QA drew nine flowcharts of its own. Every back edge ran in its own lane, with shorter lanes inside longer ones, and touched no node it does not connect to. In the `TD` case every chain node's `<text x>` was 129.6. No edge crossed a subgraph title, three deep |
| M10-R3 | An e-ink diagram sits on the page without a box, and a diagram refused at the deadline on a busy host is drawn once the host is idle, without a restart | Measurement moved the fix to the light and dark palettes ([#189](https://github.com/davison/md-notes/issues/189#issuecomment-5775179056)). A deadline refusal is cached with an expiry of 1 minute, doubling to 1 hour ([#189](https://github.com/davison/md-notes/issues/189#issuecomment-5775181549), revised at [#189](https://github.com/davison/md-notes/issues/189#issuecomment-5775456200)) | [Satisfied](https://github.com/davison/md-notes/issues/186#issuecomment-5775893446): pane and drawing pixels matched in every palette, at three widths, in the note and in the view. A real refusal, forced by pinning the daemon to one loaded core, answered `422` for the minute, and 70 s later the same daemon drew the diagram |
| M10-R4 | A note with thousands of language-tagged code blocks renders in bounded time, held by a test with a time or work bound that fails on today's code | There was no superlinear part: each block whose tag missed chroma's name tables cost about 3.5 ms in a glob search. The lookup is now made once per tag, at most 16 distinct tags per note, and never for a tag over 32 bytes ([#190](https://github.com/davison/md-notes/issues/190#issuecomment-5773673567), corrected at [#190](https://github.com/davison/md-notes/issues/190#issuecomment-5774618319)). `TestTaggedBlocksCostNoMoreThanHighlightedOnes` fails on `c4ab83e` | [Satisfied](https://github.com/davison/md-notes/issues/186#issuecomment-5775893446): 20,000 `mermaid` blocks in 0.15 s, where `c4ab83e` was still rendering at a 30 s timeout. `jungle` fences came out plain in 9 ms. Notes ending in unclosed constructs rendered byte-identical to `c4ab83e`, so the appended stand-in text leaks nothing |
| M10-R5 | A repeated `--root` is never half-ignored; several permanent roots on the command line and in the file, distinct from recent roots and not removable; the first-start log names the token file in use; the last removal writes an empty list | Any `--root` replaces `notes_root` whole. Duplicates stop the daemon, and nested roots are allowed. `DELETE` refuses a permanent root with `403 permanent_root`. A recent root that is now configured is hidden, not dropped, and nothing writes the state file before the bind ([#191](https://github.com/davison/md-notes/issues/191#issuecomment-5773596918)) | [Satisfied](https://github.com/davison/md-notes/issues/186#issuecomment-5775893446): QA checked the command-line and list forms, six ways a start is refused, the `DELETE` codes, the token hint with and without `--token-file`, and `{"recent": []}` through the API and the home page. A failed bind left `roots.json` byte for byte as it was |
| M10-R6 | A clean editing session is not put into the deleted-note conflict and recovers when its file returns; Recreate keeps the vim mode; a stale roots-listing rejection never lands on the root the reader moved to | A new session status, `gone`, which is not unsaved ([#192](https://github.com/davison/md-notes/issues/192#issuecomment-5773966327)). The vim mode is parked with the editor state ([#192](https://github.com/davison/md-notes/issues/192#issuecomment-5773977575)), and the Ctrl+E consequence went to a gate. The roots-listing effect gained a `cancelled` guard | [Satisfied](https://github.com/davison/md-notes/issues/186#issuecomment-5775893446): a clean delete showed `Deleted on disk` with no conflict, no tab marker and no unload prompt. Typing turned it into the conflict. Insert mode survived a `Ctrl+E` round trip. A delayed rejection for the old root showed nothing on `b8e64b5`, and on `c4ab83e` it showed the stale error |
| M10-R7 | Every action on a Node 24 major with no Node 20 warning; the runner pinned or verified against Ubuntu 26 before 2026-10-19; the manual page in the `.deb`, the AUR package and `make install`; `docs/releasing.md` says what a user can verify about a release's bytes, measured on the runner | Six actions moved to their Node 24 majors, and every job is pinned to `ubuntu-24.04` ([#193](https://github.com/davison/md-notes/issues/193#issuecomment-5773410249)). The page moved to `contrib/mdn.1` ([#193](https://github.com/davison/md-notes/issues/193#issuecomment-5773575022)). v0.1.0 rebuilds bit for bit on the runner, on 24.04 and 26.04 ([#193](https://github.com/davison/md-notes/issues/193#issuecomment-5773496854)), and the toolchain is pinned ([#193](https://github.com/davison/md-notes/issues/193#issuecomment-5773500114)) | [Satisfied](https://github.com/davison/md-notes/issues/186#issuecomment-5775893446): every action's `action.yml` declares `node24`, and both runs' annotations were empty. `make install DESTDIR=…` staged all three files. "Checking a release", followed exactly, printed `v0.1.0 reproduced`. The AUR `verify.sh` and the `.deb` `verify.sh` passed in containers |
| M10-R8 | The README says what md-notes is and why a person would want it, with screenshots including a flowchart, and talks about the diagrams; installing, running, the tailnet and building move to `docs/` and CONTRIBUTING.md with every inbound link resolving | README down from 536 lines to 160 on `53e6bed`, with generated screenshots in both schemes; `docs/install.md` and `docs/running.md` new; building merged into CONTRIBUTING ([#194](https://github.com/davison/md-notes/issues/194#issuecomment-5775350663), [#194](https://github.com/davison/md-notes/issues/194#issuecomment-5775350876)) | [Satisfied](https://github.com/davison/md-notes/issues/186#issuecomment-5775893446) on `53e6bed`: a link and anchor check over all 42 tracked `.md` files found nothing broken. 18 facts from the old sections were each found in a doc. The install page's commands worked, and a rerun of `make screenshots` matched the committed images apart from the root path and 5 editor pixels |
| M10-R9 | A privacy page and per-permission justifications for the extension, linked from its install section; the browser suites skip without Chromium as documented, or the docs say what happens | `docs/privacy.md` taken from PR #151's head and checked against `extension/src` ([#195](https://github.com/davison/md-notes/issues/195#issuecomment-5774801415)). Both suites skip without the browser, and every missing prerequisite fails under CI ([#195](https://github.com/davison/md-notes/issues/195#issuecomment-5774767049)) | [Satisfied](https://github.com/davison/md-notes/issues/186#issuecomment-5775893446): the privacy page checked against the source, the manifest and the built `dist`. With an empty browser path, six non-CI values of `CI` skipped with exit 0, and `CI=true` and `CI=1` failed |
| M10-R10 | Every change above described where a user or contributor looks for it; the roadmap row; this record | This task. Stage one was reviewed on [PR #206](https://github.com/davison/md-notes/pull/206#issuecomment-5775823962) and its findings were taken. QA's observation 1 was fixed here | [Untestable at the verdict](https://github.com/davison/md-notes/issues/186#issuecomment-5775893446): nothing of this task existed on `main`. A superseding verdict is owed once it merges |

## The human gates

Two gates were raised in the milestone, one on #190 and one on #192. Both were resolved as
option (a). No task plan listed a blocking ask-the-human point.

**Jungle, on #190** ([#190](https://github.com/davison/md-notes/issues/190#issuecomment-5773668362)),
raised at 08:47:10Z. While building #190's comparison corpus, the implementer found that
chroma v2.2.0's Jungle lexer never finishes. A block tagged `jungle` holding only `{` pins a
CPU core for good inside the note render, on `c4ab83e` too. That is an unbounded render, but not
the per-block lookup cost M10-R4 names, and chroma is held at v2.2.0 (#147). The question was
whether #190 should fold in a minimal fix, or whether the coordinator should capture it for
later. Two resolution comments followed. The first, at 08:57:50Z, reads in full "fix it now,
don't leave it" ([#190](https://github.com/davison/md-notes/issues/190#issuecomment-5773794459)).
The second, at 08:58:12Z, is recorded by the coordinator "on the operator's instruction, given
in session", with the operator's words as relayed: "option a on the jungle gate, fix it in
#190" ([#190](https://github.com/davison/md-notes/issues/190#issuecomment-5773798886)). Its
trade-off: the hang can be triggered from clipped web content, and the fix is a few lines in a
file PR #199 already changes. The cost is that `jungle` code loses its highlighting.

The fix keys on the resolved lexer, not the tag: `foo.jungle` reaches the same lexer, and the
review of PR #199 found it hung on 11 of 14 short samples, not only one-character ones
([PR #199](https://github.com/davison/md-notes/pull/199#issuecomment-5774053001),
[#190](https://github.com/davison/md-notes/issues/190#issuecomment-5774618925)).

**Ctrl+E and the vim mode, on #192**
([#192](https://github.com/davison/md-notes/issues/192#issuecomment-5774043141)), raised at
09:18:00Z. The fix for #112 parks the vim mode with the editor state. As a side effect, a
`Ctrl+E` round trip begun in insert mode now comes back in insert mode, where it used to come
back in normal mode. The options: (a), recommended, keep it, so that the mode follows every
park and restore as the caret does; or (b) restore the mode only after **Recreate the note**,
as vim does when you re-enter a buffer. The resolution, at 10:05:55Z, reads in full "(a) keep
insert mode" ([#192](https://github.com/davison/md-notes/issues/192#issuecomment-5774634547)).
A unit test now pins the insert-mode round trip, and it was shown to fail under the option (b)
condition ([#186](https://github.com/davison/md-notes/issues/186#issuecomment-5774688317)).

Both resolution comments on #190 and the one on #192 are posted under the operator's account,
which is also the account every seat in this project uses. Only the second #190 comment says
who gave the words and how. The record cannot tell whether the other two were typed by the
operator or relayed.

## Decisions

### Scope, and what was left out

The coordinator took a capture in if a user can hit it (#30, #112, #123, #162, #178), if it
has a date on it (#161), if a release is incomplete without it (#168, #167), or if it is the
README's own material (#160, #153). #155 and #125 were one-line riders on files #191 opens
anyway. Left out, each open for the operator to pull in
([#186](https://github.com/davison/md-notes/issues/186#issuecomment-5773313629)):

- #107, #108 and #111: real, but narrow edge cases in the clipper, the touch UI and the
  resolver;
- #110 and #127: test hygiene;
- #122 and #145: found by reading or by construction, both bounded, neither met in use;
- #149: palette ownership and a chroma upgrade. A colour change is not something to take just
  before a release.

The pre-flight on `c4ab83e` found `govulncheck` and `pnpm audit` clean, and only patch releases
outdated. They were not taken, and chroma stayed held by #147.

**The plan** was waves, so that no two tasks in flight touched the same files. Wave one was
#188, #190, #191 and #193, with #193 first because of its date. Wave two, after #188 merged,
was #187 and #192, then #189, which touches the same diagram CSS and cache as #187. Wave three
was #195, then #194, whose flowchart shot was to come after #187 and #188. QA and this record
came last.

**What ran** did not follow the plan in four places. The times are each task's **Started by**
comment and each pull request's merge:

| Task | Started | Merged | Against the plan |
|------|---------|--------|------------------|
| #191 | 08:21:39Z | 08:57:24Z | Started first and merged first, ahead of #193 |
| #190 | 08:24:53Z | 10:13:09Z | |
| #193 | 08:25:10Z | 09:06:13Z | |
| #188 | 08:25:36Z | 09:41:06Z | |
| #192 | 09:08:39Z | 10:09:35Z | Started 32 minutes before #188 merged |
| #187 | 09:45:01Z | 10:48:08Z | |
| #195 | 10:12:37Z | 10:54:17Z | Started while #187 was in review and before #189 had started |
| #189 | 10:49:58Z | 11:25:47Z | |
| #194 | 10:56:36Z | 11:33:13Z | Started before #189 merged |

The consequences are on the record. Because #191 merged before #193, the round-one review of
PR #197 found the manual page stale. #195 and #187 both changed the harness import line in
`ui/e2e/diagram.test.mjs`, and that was PR #203's only rebase conflict
([PR #203](https://github.com/davison/md-notes/pull/203#issuecomment-5775211633)). #194's
flowchart shots showed the #180 box that #189 was still fixing. No comment records a reason for
starting #192, #195 or #194 early; see
[Coordinator mistakes](#coordinator-mistakes-on-the-record).

### Wide diagrams: a floor of 12/14, a scroll box, an overlay

The floor is **12/14 of natural size** (about 0.857), so the 14 px labels never show below
12 px ([#187](https://github.com/davison/md-notes/issues/187#issuecomment-5774372931)). It was
set against measurement on `8625a4e`. Body text is 15 px, the smallest running text the app
sets is 12.75 px, and at DPR 1 labels at 0.9 and 0.8 of natural size read comfortably, 0.75
reads with effort and 0.6 does not. In the 720 px desktop column, a diagram up to 840 px wide
shrinks to fit. The operator's note, which showed at 38% (5.3 px labels), now shows at 1621 px
in its box.

**Trade-off:** more diagrams scroll, so less of a diagram is seen at once. The natural-size view
is where you see all of a wide one.

**Rejected:** 0.75, #183's example, which gives 10.5 px labels, smaller than any text the app
sets. Also 1.0, never shrinking, which would scroll a diagram for the sake of 2 px of text.

The natural-size view is an overlay rather than a new tab, since it needs no second route and
works the same on a phone ([#187](https://github.com/davison/md-notes/issues/187)).

### Flowchart routing inside the existing layout

Back edges and the straight chain are three small changes to the existing layered layout, not a
rewrite as Brandes–Köpf
([#188](https://github.com/davison/md-notes/issues/188#issuecomment-5773589715)):

- A reversed edge's dummies go after everything else in their rank, in a lane that keeps 24 px
  from any node. Shorter edges take the inner lanes.
- A back edge pulls on its end nodes with weight 0.25, down from 2. After relaxation, the
  longest path of forward edges is placed on one line where the constraints allow, then the
  next longest. A path stops at a node with more than two forward edges in or out.
- Clearance and straightness are asserted on fixtures in all four directions, not in
  `checkDrawing` for every input, because the routing does not guarantee clearance for
  arbitrary dense graphs.

**Trade-off:** lanes take room outside the node band. The operator's note in LR went from
1974 × 135 to 1974 × 171 px.

**Rejected:** Brandes–Köpf alignment. It would replace the separation-constraint positioning
that keeps subgraph boxes rectangular and disjoint.

Titles are held clear of edges on the title's own box, not its whole band
([#188](https://github.com/davison/md-notes/issues/188#issuecomment-5773589968)). In TB an edge
into a subgraph must cross the top border somewhere, so the requirement can only be about the
title text. Each title goes at the free place nearest the centre. In TB and BT, a subgraph
with no free place is widened, and the position pass runs again, at most twice. **Rejected:**
routing edges round the band into the subgraph's side, which needs a real obstacle router with
bends. The test found an existing bug: in LR and RL a title wider than its subgraph's contents
was drawn outside the box. It now widens the box.

### The box behind a drawing, and a retried deadline refusal

**The box** ([#189](https://github.com/davison/md-notes/issues/189#issuecomment-5775179056)).
Each palette's drawing background and edge-label fill is the colour of the surface the diagram
sits on, which is the note pane's `--pane`: `#ffffff` in light and e-ink, `#202020` in dark.
Light went from `#fbfbfa` and dark from `#1b1b1b`; e-ink already matched. **Rejected:** a
transparent background in every palette, because a drawing opened directly as a document would
then show the dark palette's light text on the browser's white canvas. How this departed from
#180 is under [Hand-offs to this record](#hand-offs-to-this-record).

**The retry** ([#189](https://github.com/davison/md-notes/issues/189#issuecomment-5775181549)).
`diagram.Refusal` gains `Deadline`, set only when the render's own deadline ended the layout.
A deadline refusal goes into the size cache with an expiry of 1 minute, doubling with each
consecutive refusal of the same source, up to 1 hour. Refusals that are properties of the
source (unsupported, syntax, a size or count limit) stay cached as before. **Rejected:** not
caching deadline refusals at all, which would cost 2 s of a draw slot on every open; a single
fixed expiry, which keeps paying for an always-slow block at the same rate forever; and
retrying only when the draw slots are idle, which says nothing about the CPU.

The review of PR #204 found that an expired refusal was re-measured by the note endpoint on
every open until the page asked for its image. The decision was revised: an expired refusal is
listed unmeasured, and only the image route draws it
([#189](https://github.com/davison/md-notes/issues/189#issuecomment-5775456200)). The revision
also corrected the figures: a block that is always too slow is drawn at 0, 1, 3, 7, 15 and 31
minutes, so **6 draws in its first hour**, the 7th at 63 minutes, then hourly, which is 29 in
the first day, then 24 a day, each per palette asked for. The approving review added that
"once per expiry" is really at most one per draw slot, if a burst of opens lands exactly at
the expiry; the coordinator accepted that as bounded and not worth a per-hash single flight
before the release ([#186](https://github.com/davison/md-notes/issues/186#issuecomment-5775587336)).

### Tagged code: intercept the lookup, cap it, and leave the drawing to the highlighter

The plan had the renderer own fenced blocks and reimplement the highlighting path. Instead,
`Render` works out each block's lexer first and hands the block to goldmark-highlighting
([#190](https://github.com/davison/md-notes/issues/190#issuecomment-5773673279)). The extension
also honours undocumented info-string attributes (`linenos`, `hl_lines`, `nohl` and others),
which a reimplementation would have had to copy. A tag in chroma's name or alias tables is
passed on as it is. Any other tag is looked up once per `Renderer`, and a lexer found that way
is handed to the extension under a quick name. **Rejected:** a cache alone, which does not bound
a note whose every tag is new (4,000 such blocks took 19.5 s on `c4ab83e`); registering aliases
in chroma's global registry, which mutates shared state; and upgrading chroma, held by #147.

A note has at most **16** distinct tags outside those tables looked up; a block with a later
one renders as plain code ([#190](https://github.com/davison/md-notes/issues/190#issuecomment-5773673567)).
The count includes remembered tags, so what a note renders as depends only on the note.
**Rejected:** a cap on highlighted blocks per note, since highlighting a known language is real
work, linear in the code (20,000 `go` blocks take 1.4 s), and a cap would un-highlight long
notes of real code.

The review then found that a lookup's cost grows with the tag's length, about 0.27 ms per byte,
so the cap bounded the number of lookups and not their cost. A tag outside the tables longer
than **32 bytes** is now never looked up
([#190](https://github.com/davison/md-notes/issues/190#issuecomment-5774618319)). The worst case
measured after that was 181 ms (16 distinct 32-byte tags). **Rejected:** 64 bytes, the review's
suggestion, at 325 ms for the same note.

### Several permanent roots

All seven decisions are on #191:

- The configured roots after the first are a new kind, `permanent`; the first keeps `notes`
  ([#191](https://github.com/davison/md-notes/issues/191#issuecomment-5773367920)). **Rejected:**
  `configured`, which does not tell the two apart, and `pinned`, which sounds like something a
  reader did in the UI.
- `DELETE` refuses a permanent root with `403 {"code":"permanent_root"}`, a sibling of
  `notes_root` ([#191](https://github.com/davison/md-notes/issues/191#issuecomment-5773368229)).
- Any `--root` replaces the file's `notes_root` whole, as `--port` replaces `port`
  ([#191](https://github.com/davison/md-notes/issues/191#issuecomment-5773368619)).
  **Rejected:** flags adding to the list, which puts two lists and an ordering rule in play. So a
  `--root` repeating a file entry needs no special case
  ([#191](https://github.com/davison/md-notes/issues/191#issuecomment-5773369097)).
- A duplicate in the configured list, by real path, stops the daemon with an error naming both
  entries ([#191](https://github.com/davison/md-notes/issues/191#issuecomment-5773369426)).
  **Rejected:** collapsing silently, or with a log line, because the log is where the original
  `--root` bug went unnoticed.
- A root inside another is allowed, as a nested `mdn open` already was
  ([#191](https://github.com/davison/md-notes/issues/191#issuecomment-5773369706)).
- **Amended after review:** a recent root that is now configured is hidden while it is
  configured, and its entry stays in `roots.json`. Starting the daemon writes nothing to the
  state file until the port is bound
  ([#191](https://github.com/davison/md-notes/issues/191#issuecomment-5773596918)). The first
  version dropped the entry at load, and the review measured a start that failed to bind
  emptying `roots.json`. **Rejected:** keeping the drop but moving it after the bind, which still
  loses the folder on the next start without the flag.

### A clean session whose file is deleted is `gone`

A new session status, `gone`, rather than a reworded conflict
([#192](https://github.com/davison/md-notes/issues/192#issuecomment-5773966327)). The text stays
in the editor, and nothing is treated as unsaved: no draft mirror, no unload prompt, no
`Unsaved:` entry and no tab marker. The file coming back is adopted as any clean change is, even
under its old revision. Typing turns it into the deleted conflict. **Recreate the note** is
offered in `gone` too. In view mode the rendered view keeps showing what is on disk, under a
`Deleted on disk` bar and a neutral notice. **Rejected:** keeping the conflict for clean drafts
with reworded text, which leaves `hasUnsaved` true for a session with nothing unsaved.
**Trade-off,** as corrected after review
([#192](https://github.com/davison/md-notes/issues/192#issuecomment-5774170446)): the text is lost
without asking only when the page itself goes, on a reload or a closed tab.

The vim mode is parked with the editor state and restored whenever that state is kept, not only
after Recreate ([#192](https://github.com/davison/md-notes/issues/192#issuecomment-5773977575)).
Its consequence for `Ctrl+E` went to the gate above.

### CI, packaging and what a release's bytes prove

- **The runner is pinned** to `ubuntu-24.04` in every job
  ([#193](https://github.com/davison/md-notes/issues/193#issuecomment-5773410249)). The publish
  workflows check out a tag and run when the release is published, so an image change in
  between can break a channel in a way only a new tag can fix. **Rejected:** staying on
  `ubuntu-latest` and verifying on 26.04, which clears this date but not the next bump. The
  26.04 build and lintian were measured anyway: both pass.
- **v0.1.0 rebuilds bit for bit** when the toolchain is right, on the runner (two builds in one
  job, a second job, and 26.04) and locally. What QA saw earlier was toolchain drift: Arch's
  patched Go, and Arch's Node, whose system zlib changes the gzipped assets and so both binaries
  and the zip ([#193](https://github.com/davison/md-notes/issues/193#issuecomment-5773496854)).
- **The toolchain is pinned in the tree:** `toolchain go1.27.1` in `go.mod`, and `.node-version`
  at 24.21.0 ([#193](https://github.com/davison/md-notes/issues/193#issuecomment-5773500114)).
  **Trade-off:** someone has to bump the pins. `make vuln` prompts for Go, and nothing prompts
  for Node. **Rejected:** documenting without pinning; removing the zlib dependence, which is a
  design change to the UI build; `-buildvcs=false`, which throws away the commit stamp; and
  `SOURCE_DATE_EPOCH` for the zip, which already reproduces.
- **The manual page** moved to `contrib/mdn.1`, and each route fills in its version and date
  ([#193](https://github.com/davison/md-notes/issues/193#issuecomment-5773575022)). `make build`
  renders it and `make install` only copies, so install still forks no git, go or pnpm.
  **Rejected:** having aurgen bake the publish date into the PKGBUILD, running `./mdn version` at
  install time, and installing an uncompressed page.

### The browser suites, the privacy page and the screenshots

- **#153 is made true** rather than withdrawn: both browser suites probe for the download and
  skip with one line naming the install command. Under CI **every** missing prerequisite fails,
  not only the browser, so CI's e2e job cannot go green by skipping
  ([#195](https://github.com/davison/md-notes/issues/195#issuecomment-5774767049)). **Rejected:** a
  runner script that prints one line for the whole run.
- **The privacy page** is taken from PR #151's head `e707c7f`, rewritten for an unpacked zip,
  and checked claim by claim against `extension/src`; the store listing is not taken as a file
  ([#195](https://github.com/davison/md-notes/issues/195#issuecomment-5774801415)). The page
  dropped the store-only material and one false claim, that profile sync may carry the
  settings, and corrected three others: when the token is sent to a loopback daemon, that a
  clip carries its kind, and what the per-tab record holds. The third correction, that the
  record is dropped when the tab moves on, was then found false after a worker restart by the
  first review of PR #203, and fixed in the code.
- **The extension screenshot generator** is taken, moved to `extension/scripts/`, and run by
  `make extension-screenshots` ([#195](https://github.com/davison/md-notes/issues/195#issuecomment-5774818699)).
  **Rejected:** a target that always runs inside `unshare`, because Ubuntu 24.04 restricts
  unprivileged user namespaces.
- **The README's sections** went to `docs/install.md`, `docs/running.md` and CONTRIBUTING, and
  the tailnet section was not moved because the introduction already covers it
  ([#194](https://github.com/davison/md-notes/issues/194#issuecomment-5775350663)).
  **Rejected:** folding Running into the introduction's reference section, which does not give a
  newcomer the order of steps.
- **The app's screenshots** are generated by `make screenshots` from an invented demo folder,
  in both schemes, through `<picture>`
  ([#194](https://github.com/davison/md-notes/issues/194#issuecomment-5775350876)). The #158
  layout shots were not reused, because of their fixture text.

## Deviations and narrowings

**#190 kept goldmark-highlighting** instead of owning fenced blocks, as its plan said
([#190](https://github.com/davison/md-notes/issues/190#issuecomment-5773673279)). See the
decision above.

**#189 changed the light and dark palettes, not e-ink,** against its plan, which followed
#180's premise ([#189](https://github.com/davison/md-notes/issues/189#issuecomment-5775179056)).
See [Hand-offs](#hand-offs-to-this-record).

**#187's view adds no e-ink rule of its own.** The plan said 2 px borders in e-ink. The app has
no e-ink CSS besides the palette tokens, and the view draws on `--pane`
([#187](https://github.com/davison/md-notes/issues/187#issuecomment-5774610378)).

**#189 took a scope addition**, the re-anchoring of a search hit when a measured diagram fails.
It was nit 2 of the first review of PR #202, and pre-existing
([#189](https://github.com/davison/md-notes/issues/189#issuecomment-5774861925)).

**The natural-size view can still follow the wrong diagram in one case.** In the code's words,
"the one case still followed wrongly is a diagram deleted while another new one lands at its
place, which is indistinguishable from an edit" (`followViewed` in `ui/src/diagrams.ts`).
Otherwise the view follows only a diagram new since the last check, or closes. The limit was
not captured
([#186](https://github.com/davison/md-notes/issues/186#issuecomment-5775146918)).

## What the reviews changed

Every implementation pull request was reviewed by a clean-context session under the reviewer
contract, with the reviewer seat routed to the operator. Each merged under the operator's
standing confirmation, posted as an operator-confirmation comment on it.

**[PR #198](https://github.com/davison/md-notes/pull/198), three rounds.**
- **Round one** ([changes requested](https://github.com/davison/md-notes/pull/198#issuecomment-5773550700)):
  three blocking findings. The start log named no roots, so the replace rule acted silently. A
  recent root moved to permanent was erased from `roots.json` even by a start that then failed
  to bind. Three statements, one in the tailnet threat model, left permanent roots out of what a
  token holder can reach. The nits: a null list entry dropped silently, long lines, and the
  extension typing a permanent root as `recent`.
- **Round two** ([changes requested](https://github.com/davison/md-notes/pull/198#issuecomment-5773664591)):
  a hidden entry's slug was not reserved, so it did not always come back "under its old slug".
  The nits: a race in the new failed-bind test, measured at 1 in 15 runs under `-race`, a hook
  comment, and one long line.
- **Round three** [approved](https://github.com/davison/md-notes/pull/198#issuecomment-5773785622).
  The reviewer showed the race's fix causally: 14 of 120 loaded runs failed without it, and 0 of
  120 and 0 of 300 with it.

Dispositions: [#186](https://github.com/davison/md-notes/issues/186#issuecomment-5773792610). The
extension's type is left for whichever task next touches the extension's roots code. The
watchers left open when `Serve` fails after a bind predate #191, and were not captured.

**[PR #197](https://github.com/davison/md-notes/pull/197), two rounds.**
- **Round one** ([changes requested](https://github.com/davison/md-notes/pull/197#issuecomment-5773809921)):
  the manual page was stale, because PR #198 had merged first and the page still described one
  `--root`. "Checking a release", run against v0.1.0 as written, silently built with the wrong
  toolchain, because v0.1.0 has neither pin. The nits: CONTRIBUTING did not mention the pins, one
  test passed against the old Makefile, the page spoke as the Debian package, and long lines.
- **Round two** [approved](https://github.com/davison/md-notes/pull/197#issuecomment-5773890501).
  The script now refuses when a pin is missing, and the reviewer ran it in a fresh clone.

**[PR #200](https://github.com/davison/md-notes/pull/200), two rounds.**
- **Round one** ([changes requested](https://github.com/davison/md-notes/pull/200#issuecomment-5773941252)):
  CI was red, because the faster layout let the 150 ms budget measure almost every diagram of an
  e2e fixture meant to go out unmeasured. Self-loops and back-edge ends were drawn on top of each
  other. Non-blocking: the reserved title strip was wider than a title needed (`pipeline` TB at
  457 px), the worst subgraph-heavy render time roughly doubled, lanes wove where spans
  interleaved, and the docs overclaimed.
- **Round two** [approved](https://github.com/davison/md-notes/pull/200#issuecomment-5774317213).
  Loops moved to the side away from the lanes, lanes are placed whole, the strip is only the
  missing room (`pipeline` TB at 326.4 px), and the fixture was made expensive again: 43 to 47 of
  61 diagrams went out unmeasured.

Dispositions: [#186](https://github.com/davison/md-notes/issues/186#issuecomment-5774325248).

**[PR #201](https://github.com/davison/md-notes/pull/201), one round.**
[Approved](https://github.com/davison/md-notes/pull/201#issuecomment-5774162580) with three nits:
a docs sentence about a note deleted from another tab was now true only with a draft; no test
pinned the Ctrl+E behaviour, so option (b) passed every suite; and the #30 decision's trade-off
overstated what a reader loses. All three were fixed after the approval
([#186](https://github.com/davison/md-notes/issues/186#issuecomment-5774688317)).

**[PR #199](https://github.com/davison/md-notes/pull/199), two rounds.**
- **Round one** ([changes requested](https://github.com/davison/md-notes/pull/199#issuecomment-5774053001)):
  a lexer whose quick name contains a space lost its highlighting, because goldmark cuts a tag at
  the first space. `mk`, `mak`, `GNUmakefile`, `sig` and `fun` were among the tags affected, so
  "byte-identical except `el`" was false. And the per-note bound depended on the tag's length:
  one 16 KB tag took 4.4 s, and 16 distinct 64 KB tags took 5 min 10 s. The nits: the docs'
  wording of the cap was narrower than the code, and the `el` decision would widen.
- **Round two** [approved](https://github.com/davison/md-notes/pull/199#issuecomment-5774716756),
  with two record-only nits, both posted as corrections on #190.

Dispositions: [#186](https://github.com/davison/md-notes/issues/186#issuecomment-5774731303).

**[PR #202](https://github.com/davison/md-notes/pull/202), three rounds.**
- **Round one** ([changes requested](https://github.com/davison/md-notes/pull/202#issuecomment-5774856964)):
  the natural-size view kept a stale copy of the drawing. A palette change left a light drawing
  on a dark pane, and an edit left the old drawing, with focus falling to `BODY` on close. The
  nits: a measured diagram that fails still moved a search hit (handed to #189), and the button's
  accessible name was the whole diagram source.
- **Round two** ([changes requested](https://github.com/davison/md-notes/pull/202#issuecomment-5775029763)):
  after an edit on disk the view stayed open with focus on the note's title behind it, in 5 runs
  of 5, so the modal's trap was broken. Non-blocking: in four same-count saves the view jumped to a
  diagram the reader had not opened. A nit: focus fell to `BODY` when the whole note was deleted.
- **Round three** [approved](https://github.com/davison/md-notes/pull/202#issuecomment-5775140580).

Dispositions: [#186](https://github.com/davison/md-notes/issues/186#issuecomment-5775146918).

**[PR #203](https://github.com/davison/md-notes/pull/203), two rounds.**
- **Round one** ([changes requested](https://github.com/davison/md-notes/pull/203#issuecomment-5774949425)):
  the privacy page said the per-tab record, which holds a `file:` URL, is dropped when the tab
  moves on. After a service-worker restart it was not. This was fixed in the code, with an e2e
  case that stops the worker. The nits: a stale README pointer, PNGs that oxipng made a third smaller at no cost,
  and a test header that claimed it never skips. The stale pointer was to a `README.md` in
  `extension/scripts/`, left from the store-era script.
- **Round two** [approved](https://github.com/davison/md-notes/pull/203#issuecomment-5775157856),
  with three nits, fixed before merge. The first was a sentence on the privacy page that was false
  in the safe direction: moving between notes in the app does drop the record.

Dispositions: [#186](https://github.com/davison/md-notes/issues/186#issuecomment-5775218047).

**[PR #204](https://github.com/davison/md-notes/pull/204), two rounds, both approvals.**
- **Round one** ([approved](https://github.com/davison/md-notes/pull/204#issuecomment-5775385486))
  with five nits and an observation. The hold waited on measured lazy images far above the hit. An
  expired refusal was re-measured on every open. The PR body's "7 in its first hour" was really 6.
  CONTRIBUTING's count needed updating, and a comment was stale. The observation: closing the view
  on a failed image jumped the note to the top. All were fixed in the PR.
- **Round two** [approved](https://github.com/davison/md-notes/pull/204#issuecomment-5775579242)
  with two nits, both accepted with no code change. The coordinator's disposition amends the
  retry decision for the first.

Dispositions: [#186](https://github.com/davison/md-notes/issues/186#issuecomment-5775587336).

**[PR #205](https://github.com/davison/md-notes/pull/205), two rounds.**
- **Round one** ([changes requested](https://github.com/davison/md-notes/pull/205#issuecomment-5775477268)):
  the flowchart shots showed the #180 box, because #189 had not merged. The screenshot script
  exited 0, silently, and left `/tmp/notes` behind when the daemon died. The nits: the wide shot's
  search pane opened on frontmatter, the `.gitignore` claim held only in a git repository, the
  Android claim held only over the tailnet, and the repeatability figure named one editor shot
  where both differ.
- **Round two** [approved](https://github.com/davison/md-notes/pull/205#issuecomment-5775677427),
  after the rebase over #204 and a retake, with one record-only nit, corrected on #194.

Dispositions: [#186](https://github.com/davison/md-notes/issues/186#issuecomment-5775688631).

**[PR #206](https://github.com/davison/md-notes/pull/206)**, this task. Stage one
[requested changes](https://github.com/davison/md-notes/pull/206#issuecomment-5775823962). It had
carried the three-differences hand-off over without checking it, and the reviewer measured the
fourth. Four nits: the vim-mode sentence did not say it is about Recreate, a misplaced sentence in
`docs/e-ink.md`, an unwrapped line, and `docs/sync.md` missing the `Deleted on disk` state. All
were taken ([PR #206](https://github.com/davison/md-notes/pull/206#issuecomment-5775879069)). This
record is reviewed on the same pull request.

## Hand-offs to this record

**The deadline headroom.** M9's record says the 2 s deadline is "about ten times" the worst case
inside the bounds, 211 ms on #170's reference host. After #188 that holds only for `slowest()`,
which is no longer the slowest input inside the bounds. On the approving review's corpus of 151
subgraph-heavy inputs, the worst case took 127.5 ms against 38.9 ms for `slowest()`, or 3.3×. On
the reference host that is **about 0.69 s, roughly 2.9× inside the deadline**
([PR #200](https://github.com/davison/md-notes/pull/200#issuecomment-5774317213),
[#186](https://github.com/davison/md-notes/issues/186#issuecomment-5774325248)). The implementer's
own figure was 0.72 s, about 2.8×, and the reviewer judged the two to agree. The implementer's
reply says its machine was shared and gives its figures as ±50%
([PR #200](https://github.com/davison/md-notes/pull/200#issuecomment-5774193175)). M9's record is sealed and stays as it is. This is the correction.

**#180 was reinterpreted after measurement.** #180, from M9 QA, said the e-ink drawing sat in a
faint white box on the light page's `--bg`. Measurement on `df276bd` showed that the reading
column is drawn on `--pane`, not `--bg`, at every width. The box was real in the **light**
palette (drawing 251 on a 255 pane) and the **dark** one (27 on 32), and absent in **e-ink** (255
on 255). Fixing what the capture named would have made e-ink wrong and left the other two
([#189](https://github.com/davison/md-notes/issues/189#issuecomment-5775179056)). The
coordinator's disposition records that all 18 setting combinations now match the pane
([#186](https://github.com/davison/md-notes/issues/186#issuecomment-5775587336)), and QA matched
pane and drawing pixels in every palette it tried.

**The phone gutter is 15 px.** It is 1rem, which is 15 px at the app's 15 px root font, not the
16 px #183 assumed. It was left unchanged, since it is outside M10-R1
([#186](https://github.com/davison/md-notes/issues/186#issuecomment-5775146918)). QA measured
the boxes at 15 to 397 px at 412 and 15 to 305 px at 320. PR #200's body also used 16 px gutters
when it said `pipeline` TB fits a 390 px phone.

**Four rendering differences from v0.1.0, not three.** The coordinator's disposition of PR #199
named three and called them "the only rendering differences from v0.1.0"
([#186](https://github.com/davison/md-notes/issues/186#issuecomment-5774731303)). The stage-one
review of this task measured a fourth, and the coordinator corrected the hand-off
([#186](https://github.com/davison/md-notes/issues/186#issuecomment-5775828263)). The four:

1. `el`, and file names ending `.el`, `.cl` or `.lisp`, now use the wrapper lexer that `elisp`
   and `cl` use, which colours some names as builtins and keywords
   ([#190](https://github.com/davison/md-notes/issues/190#issuecomment-5774618609)).
2. A tag outside chroma's names and aliases longer than 32 bytes renders as plain code, real
   file names included: `my-really-long-helm-values-file.yaml` (36 bytes) was highlighted as YAML
   at `c4ab83e` ([#190](https://github.com/davison/md-notes/issues/190#issuecomment-5774725145)).
3. A block whose tag is such a tag renders plain once 16 different ones have come before it in
   the note. Fences tagged `f1.yaml` to `f17.yaml` render the 17th plain, where v0.1.0 highlights
   it ([PR #206](https://github.com/davison/md-notes/pull/206#issuecomment-5775823962)).
4. A block whose resolved lexer is Jungle renders as plain code, where v0.1.0 never finished.

The 23,600-combination comparison behind #190's list built its tags from chroma's own names,
aliases and globs, one per note, so it could not meet the cap. #190's cap decision stated the
plain rendering past the cap, and the introduction said so in its code-fence paragraph.

## QA's observations, and what was done

QA's two observations were outside the verdicts
([#186](https://github.com/davison/md-notes/issues/186#issuecomment-5775904677)), and the
coordinator disposed of them
([#186](https://github.com/davison/md-notes/issues/186#issuecomment-5775913746)):

1. **The manual page said permanent roots are never written to `roots.json`.** A recent root
   that becomes configured keeps its hidden entry there, per the amended #191 decision. QA
   reproduced it. This was a documentation inaccuracy under M10-R10, and it is fixed in this
   task's pull request. The sentence was written in PR #197's round-one fix (`14950cd` on the
   branch, [`823f3ca`](https://github.com/davison/md-notes/commit/823f3ca) on `main`), and the
   round-two review passed it after comparing the page with the introduction point by point
   ([PR #197](https://github.com/davison/md-notes/pull/197#issuecomment-5773890501)). The
   introduction's "never written to the state file" had the same ambiguity, so the comparison
   inherited the error. Both pages now say that a configured recent root keeps its entry, and
   that it is served as configured.
2. **`make man` also fills the placeholders inside the page's own source comment.** Cosmetic,
   since `man` never shows comments. Not captured.

The verdict's R7 note said rendering v0.1.0's PKGBUILD by hand from `main` needs `-unit` and
`-license` pointed at the tag's files. The publish workflow renders from the tag, so nothing
changes.

## Coordinator mistakes on the record

**The three-differences hand-off.** Named above: the disposition of PR #199 called three
rendering differences the only ones. It had not been checked against #190's own cap decision.
The stage-one review of this task found the fourth
([#186](https://github.com/davison/md-notes/issues/186#issuecomment-5775828263)).

**A reviewer asked to approve with a gate open.** The coordinator asked the reviewer of PR #201
to judge both options of the Ctrl+E gate, and the reviewer approved while the gate was open. The
reviewer contract says not to approve on an open gate. The gate still blocked `task finish`
until the operator resolved it
([#186](https://github.com/davison/md-notes/issues/186#issuecomment-5774688317)).

**The README's flowchart shot was sequenced after #187 and #188 only.** The scope decision says
"The README takes its flowchart screenshot after #187 and #188 have merged"
([#186](https://github.com/davison/md-notes/issues/186#issuecomment-5773313629)), while #185 asks
for the shot "once the diagram tasks in this milestone have merged", and #189 is one of them.
#194 started at 10:56:36Z, before PR #204 merged at 11:25:47Z, and the review of PR #205 found
the #180 box in the committed shots. The fix was to retake them after #204 merged
([#194](https://github.com/davison/md-notes/issues/194#issuecomment-5775531308)).

**No disposition for PR #197.** Every other implementation pull request has a coordinator
disposition on #186. PR #197's has none, on #186 or on #193. Its four nits were all fixed in the
PR before the approval, so nothing was left to dispose of, but the record does not say so.

**Premises carried into briefs without measurement.** Two task goals repeated a capture's
premise that measurement then overturned. #190's goal said "find the superlinear cost", and its
plan found no superlinear part ([#190](https://github.com/davison/md-notes/issues/190)). #189's
title and goal named a box on e-ink, which was the one palette without one
([#189](https://github.com/davison/md-notes/issues/189)). Both were found and handled by the
task, and neither cost a review round, but both goals were written by the coordinator from the
capture's words.

**Tasks started ahead of the plan's waves.** #192 started before #188 merged, #195 while #187 was
in review and before #189 had started, and #194 before #189 merged. The table under
[Scope](#scope-and-what-was-left-out) has the times. No comment records the start as a
deliberate judgement. The early #195 cost a rebase conflict with #187, and the early #194 cost
a retake of its flowchart shots.

## Corrections to the record itself

**#190, the cap's worst case.** "The worst render pays 16 × ~3.5 ms ≈ 56 ms" did not hold, because
a lookup's cost grows with the tag's length. It was corrected, and the 32-byte limit added
([#190](https://github.com/davison/md-notes/issues/190#issuecomment-5774618319)).

**#190, the `el` decision.** It called `el` "the one output change". After round one it was
widened to the full list
([#190](https://github.com/davison/md-notes/issues/190#issuecomment-5774618609)). The widened
version said a `{nohl}` block is "plain either way", which was wrong: the unchanged combination is
`el{linenos=true}`, with no space
([#190](https://github.com/davison/md-notes/issues/190#issuecomment-5774724760)).

**#190, the length limit's reach.** It named `Caddyfile-directives{linenos=true}` as the one case
that loses highlighting. Any real file-name tag over 32 bytes does too
([#190](https://github.com/davison/md-notes/issues/190#issuecomment-5774725145)).

**#191, the duplicates decision.** Its "also decided" point dropped a recent root that becomes
configured. It was amended to hide it instead
([#191](https://github.com/davison/md-notes/issues/191#issuecomment-5773596918)).

**#192, the `gone` trade-off.** It said a reader who leaves a `gone` note loses the text without
being asked. Within the page the session outlives the pane, so only a reload or a closed tab loses
it ([#192](https://github.com/davison/md-notes/issues/192#issuecomment-5774170446)).

**#189, the retry figures.** "7 draws in its first hour … about 30 a day" became 6 within the first
hour and 29 in the first day, per palette
([#189](https://github.com/davison/md-notes/issues/189#issuecomment-5775456200)).

**#194, the screenshots decision.** The weight is 672 KiB (688,517 bytes), not 676 KiB, and the
flowchart pair is drawn after #189 as well as #187 and #188
([#194](https://github.com/davison/md-notes/issues/194#issuecomment-5775683129)).

**#186, the rendering differences.** Three became four
([#186](https://github.com/davison/md-notes/issues/186#issuecomment-5775828263)).

**#188, the titles decision, not corrected on the issue.** It gives `pipeline.mmd` in TB as
growing from 227 to 457 px, and `nested-title.mmd` in TB from 301 to 389 px. Round one's fix
reserved only the missing room, and the widths became 326.4 px and 371.8 px. PR #200's body and
the approving review give the new figures, but no correction was posted on #188 ([PR #200](https://github.com/davison/md-notes/pull/200)).

**The introduction's e2e count.** It said 83 browser checks, the count at the end of M9. This
task's stage one measured 116 on `53e6bed`, and QA's floor ran 116 of 116.

## Captures adopted, and captures raised

M10 adopted 18 captures, each closed by its task's `task finish`: #183; #184 and #174; #180 and
#182; #178; #162, #155 and #125; #30, #112 and #123; #161, #168 and #167; #185; #160 and #153.

It raised two. [#185](https://github.com/davison/md-notes/issues/185), the README, was raised by
the coordinator at the milestone's opening, at 08:15:49Z, 33 s before #186, and adopted by #194.
[#207](https://github.com/davison/md-notes/issues/207), search results repeating context lines
when two hits are close together, was raised after the record review. The first review of
PR #205 said it "may be worth a backlog capture"
([PR #205](https://github.com/davison/md-notes/pull/205#issuecomment-5775477268), finding 3), and
the disposition of that review did not mention it. The record review of this task found the gap
([PR #206](https://github.com/davison/md-notes/pull/206#issuecomment-5776193990), nit 10), and
the coordinator captured it late, outside M10
([#186](https://github.com/davison/md-notes/issues/186#issuecomment-5776203327)). Every other
review disposition and QA's observations ended "not captured" or fixed in the pull request. The
items left out of scope, listed under [Scope](#scope-and-what-was-left-out), stay open in the
backlog.

## Where the record is silent

**Who typed two of the three gate resolutions.** See [The human gates](#the-human-gates). Every
seat posts as the same account, and only the second #190 resolution says how the operator's words
reached it.

**The operator's ask is in the coordinator's words.** The milestone's scope, the triage and the
ask behind #185 are all written up by the coordinator, and the operator's words in #185 are
relayed. No requirement or decision on any M10 issue or pull request says it was written by the
operator directly. The operator-confirmation comment on each merged pull request is posted by
`task finish`.

**Two approvals did not see their last commits.** See the note under
[What shipped](#what-shipped-in-the-order-it-merged). QA's verdicts ran on `main` after both, so
the code is covered by QA and CI, but no review comment covers those commits.
