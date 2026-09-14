# M4 — Polish: mobile layout, e-ink, dark code colours, sync docs, tailnet extension, asset caching

Tracking issue: [#55](https://github.com/davison/md-notes/issues/55). Six of its seven
implementation tasks are merged on `main` at
[`3802e2f`](https://github.com/davison/md-notes/commit/3802e2f); the seventh,
[#62](https://github.com/davison/md-notes/issues/62), is in review on
[#70](https://github.com/davison/md-notes/pull/70) as this document is drafted, and its
entries are marked **awaiting #62** below.

## Goal and outcome

The milestone's goal, as stated on [#55](https://github.com/davison/md-notes/issues/55),
was to make the application solid on the devices the operator now uses it from, and to
fix what three milestones of use had surfaced: the three-pane shell on a phone, the web
UI on an e-ink tablet, the browser tab title, unreadable code colours in the dark scheme,
a user-facing account of the sync and offline workflow, the extension against a tailnet
daemon URL, and the size and cacheability of the embedded UI. The inbox clipper stayed
later work. This is the first milestone whose subject is the *experience* of what the
three before it built rather than a new capability, and six of its eight requirements
were raised as backlog captures by earlier reviews and QA rather than asked for from
scratch.

The coordinator fixed the scope, the sequencing and the open defaults in one decision on
the operator's instruction
([#55](https://github.com/davison/md-notes/issues/55#issuecomment-5656096967)): six
captures adopted one per task, the tab title added as a task of its own on the operator's
direct request with no capture behind it, and the M3 standing merge confirmation
([#35](https://github.com/davison/md-notes/issues/35#issuecomment-5632220363)) carried
over, so an approved PR merged without a per-PR wait. Three defaults the requirements
would otherwise have left open were written into them for the operator to overrule:
clipping over the tailnet stays refused, the editor chunk is lazy-loaded only if
measurements overturn the M2 decision that kept the toggle instant, and the home for
search and tags on a phone is the implementer's choice, recorded.

What shipped is a web UI that works on the devices it is read on, and a bundle that costs
what it should.

Below 960 pixels of window width the note takes the whole viewport under a compact top
bar in both modes, and the navigator and the search-and-tags pane become the two tabs of
one off-canvas drawer opened by a burger and a magnifier; the active tag filter follows
the reader out of the drawer as a chip in the top bar. The browser tab names the open
note — its rendered H1, its frontmatter title, or its file name — with a leading `•` for
unsaved work and `⚠` for a conflict, and it follows the note under an open editor as well
as under the rendered view. Both code colour schemes are now generated from chroma's one
`github` palette, toned per scheme against the background the browser actually paints, so
the class sets are equal by construction and every declared colour clears 4.5:1 in the
scheme it is drawn in; the dark scheme that shipped for three milestones was chroma's
fallback for a style name that does not exist. The daemon serves its embedded assets
compressed and cacheable — brotli and gzip copies written at build time, an immutable
year on the hashed names, `no-cache` and an `ETag` on `index.html` — and the editor is no
longer in the bundle a reader downloads: a first page load transfers about 17 KB where it
transferred 722 KB, and a second transfers none. The extension works against a daemon
reached over the tailnet, presenting the token on the roots listing whenever the daemon
URL is not loopback, and naming the two actions the tailnet allow-list refuses rather
than blaming the token for them. And [Sync and offline editing](../sync.md) is the page
this milestone added: the multi-device model, Syncthing setup, conflict files in the
navigator, Android, the tailnet as the connected alternative, and the one shape that does
not work.

**Awaiting #62.** The e-ink settings — the explicit light-theme override, the no-motion
setting and the larger tap targets — and the page `docs/e-ink.md` are #62's;
this paragraph, that page's link and the outcomes table are completed when it merges.

The system as it stands is described in [the introduction](../introduction.md);
[Sync and offline editing](../sync.md) is the page this milestone added, and
[On a phone](../introduction.md#on-a-phone) is the section it added to the other.

Seven implementation tasks delivered it, each through its own PR and review loop, and
this document is the eighth ([#63](https://github.com/davison/md-notes/issues/63)):

| Task | Requirements | PR |
|------|--------------|----|
| [#56](https://github.com/davison/md-notes/issues/56) Browser tab title follows the open note | M4-R2 | [#64](https://github.com/davison/md-notes/pull/64) |
| [#58](https://github.com/davison/md-notes/issues/58) Document the sync and offline workflow | M4-R4 | [#65](https://github.com/davison/md-notes/pull/65) |
| [#60](https://github.com/davison/md-notes/issues/60) Extension against a tailnet daemon URL | M4-R6 | [#66](https://github.com/davison/md-notes/pull/66) |
| [#57](https://github.com/davison/md-notes/issues/57) Dark-scheme code highlighting covers every token | M4-R3 | [#67](https://github.com/davison/md-notes/pull/67) |
| [#59](https://github.com/davison/md-notes/issues/59) Serve embedded assets cacheable and compressed | M4-R7 | [#68](https://github.com/davison/md-notes/pull/68) |
| [#61](https://github.com/davison/md-notes/issues/61) Phone layout: drawer navigator and a note pane that fills the screen | M4-R1 | [#69](https://github.com/davison/md-notes/pull/69) |
| [#62](https://github.com/davison/md-notes/issues/62) E-ink tablet: light override, no motion, larger targets | M4-R5 | [#70](https://github.com/davison/md-notes/pull/70) |

Six of the seven adopted a backlog capture and closed it on merge:
[#34](https://github.com/davison/md-notes/issues/34) by #61,
[#32](https://github.com/davison/md-notes/issues/32) by #59,
[#51](https://github.com/davison/md-notes/issues/51) by #60,
[#53](https://github.com/davison/md-notes/issues/53) by #58,
[#54](https://github.com/davison/md-notes/issues/54) by #57, and
[#52](https://github.com/davison/md-notes/issues/52) by #62. #56 is the one task with no
capture behind it. That is the milestone's shape: the backlog that three milestones of
reviews and QA had built up is where six of the eight requirements came from, and
[the M3 record's gaps table](3-clipper-authentication-and-tailnet.md#known-gaps-at-the-boundary)
is annotated with the four of them it had listed as open.

The sequencing came from the same coordinator decision: #56, #57, #58, #59 and #60 first
and in parallel, since they touch disjoint areas; #61 after #56, because both edit the
root view; #62 after #61; #63 last with QA. It held, and the cost of the parallelism was
two mechanical rebases rather than any design collision — #68 onto #56's edits to
`ui/src/note-pane.tsx`, which the reviewer rehearsed before asking for it
([#68](https://github.com/davison/md-notes/pull/68#issuecomment-5656500255)), and #69 onto
#59, after which the reviewer re-ran #56's tab title and #59's lazy editor at all four
phone profiles
([#69](https://github.com/davison/md-notes/pull/69#issuecomment-5657482484)).

No human decision gate (`cc:needs-decision`) was raised anywhere in this milestone — the
second of the four so far where none was. Every task's plan answered its own
ask-the-human section with "none", each time because the requirement itself, or the
coordinator's scope decision, had already fixed the answer.

## Requirement outcomes

**Awaiting QA.** Independent QA exercises all eight requirements against the built
daemon, UI and extension after [#62](https://github.com/davison/md-notes/issues/62)
merges, and its verdict comment on
[#55](https://github.com/davison/md-notes/issues/55) is what fills this table. Each row
below names what the verdict has to reach; none of them is a claim of this document.

| ID | Requirement | Status |
|----|-------------|--------|
| M4-R1 | Phone layout: the note under a compact top bar in both modes, the navigator as a drawer closing on selection, Escape and backdrop, search and tags reachable from the top bar, the filter visible, the root path gone, nothing overlapping at Pixel 7 and iPhone 14 in either orientation, scroll-to-line still working | _Awaiting QA_ |
| M4-R2 | Tab title: the open note's title, updating on navigation and live update; the root's slug with no note open; `MD Notes` elsewhere; the unsaved and conflict states as a leading marker | _Awaiting QA_ |
| M4-R3 | Dark-scheme code colours: a dark rule for every class the light palette defines, asserted by the generator, with contrast checked against the code background in both schemes, verified over Go, Python and shell fences | _Awaiting QA_ |
| M4-R4 | Sync and offline documentation: the multi-device model, Syncthing setup, offline editing, conflict files and how to resolve them, Android, the tailnet alternative and the one unsupported shape, linked from the README and the introduction | _Awaiting QA_ |
| M4-R5 | E-ink tablet: the light-theme override, no animations under the setting or reduced motion, 40-pixel tap targets, the editor usable with the on-screen keyboard, verified at the Boox viewport in both orientations | _Awaiting #62, then QA_ |
| M4-R6 | Extension against a tailnet daemon URL: opening a local file inside a registered root works, the two refused actions named as the allow-list's rather than the token's, Test connection reporting the true state, clipping still refused | _Awaiting QA_ |
| M4-R7 | Embedded assets: immutable long-lived caching on hashed names and `no-cache` on `index.html`, precompressed at build time and served compressed when accepted, a second page load fetching no asset bytes, and the eager JavaScript in view mode reduced or the M2 decision reaffirmed with measurements | _Awaiting QA_ |
| M4-R8 | Documentation and record: user documentation reflecting the delivered changes, the roadmap row, and this record | _Awaiting QA_ — see below |

M4-R8 is the row no verdict can settle on its own, for the same reason M2-R6 and M3-R7
could not: QA grades it against `main` as it stands *before* this task, so the
milestone-level half of it — the boundary claims, the roadmap row and this record — is
graded as missing however carefully it is then done. The closure gate on
[#55](https://github.com/davison/md-notes/issues/55) requires both that every requirement
verdict is satisfied *and* that the milestone document is merged, so the merge of this
task's PR is a precondition of closure rather than the verdict itself. QA's provisional
wording is recorded here when it arrives, with a superseding verdict expected after the
merge, exactly as
[M3-R7](3-clipper-authentication-and-tailnet.md#requirement-outcomes) records it.

The operator's own checks on the phone and on the Boox are recorded on
[#55](https://github.com/davison/md-notes/issues/55) when available and are deliberately
**not** closure gates; anything they find becomes a backlog capture or a remedy task. One
thing is explicitly waiting for them: whether an Android stylus digitiser reports itself
as a coarse pointer, which is the premise the tap-target rule's coverage rests on and
which no headless harness can settle
([#62](https://github.com/davison/md-notes/issues/62#issuecomment-5657572060)).

## Decisions

### The tab's markers are `•` and `⚠`, and they follow the session rather than the mode

`• ` (U+2022) for a note with unsaved work and `⚠ ` (U+26A0) for one in conflict, a
conflict outranking the plain unsaved states so the two never appear together. "Unsaved"
is the existing `hasUnsaved` predicate — `pending`, `saving` and `failed` — so a save in
flight and a save the daemon refused both keep the mark until the draft is on disk, and
the marker follows the *session*, not the pane's mode: a note left in the rendered view
with a draft outstanding still shows it, because the work is still unsaved whichever way
the note is being looked at
([#56](https://github.com/davison/md-notes/issues/56#issuecomment-5656129575)).

- **Trade-off:** two characters of tab width, and a glyph a bare-bones font may draw as a
  box. Both are single, widely supported code points with no variation selector, and the
  title behind them still reads, so a missing glyph costs recognition rather than meaning.
- **Rejected:** `*` and `!`, which read as part of a file name in a narrow tab; one
  marker for both states, which loses the distinction the requirement asks to be
  "reflected"; a trailing marker, which the tab truncates away exactly when the tab is
  narrow enough to need it.

### The note pane owns the title, and never derives it in the browser

The rendered view is the only component that reads the daemon's `title` for a note, so it
reports it up through an `onTitle` callback and the pane keeps it; the pane is keyed by
note in the root view, so the kept title cannot outlive the note it belongs to, and the
root view passes `null` on a render where a pane exists below it, meaning "not mine to
set". The alternative that was refused throughout is deriving the title in the browser
from the draft's first H1 and frontmatter, which forks `internal/render`'s rule —
goldmark's inline parsing and its top-level-heading rule — into a second implementation
that would have to be kept in step
([#56](https://github.com/davison/md-notes/issues/56#issuecomment-5656129674)).

- **Trade-off, as first recorded:** a note opened straight into the editor is titled by
  its file name until the rendered view next runs. This is the milestone's first
  correction: the sentence was true of the case it named and false of two it was written
  as though it covered, and the fix removed the trade-off rather than restating it — see
  [Corrections](#corrections-to-the-record-itself).
- **Rejected:** fetching the rendered note from the pane as well, which doubles a request
  for a string; deriving the title in the browser, above; hoisting the note fetch out of
  the rendered view into the pane, a larger change to a component the task had no other
  reason to touch.

### Both code colour schemes come from one palette, toned per scheme

The investigation found a cause underneath the one
[#54](https://github.com/davison/md-notes/issues/54) reported. `gencss` asked chroma for
`github-dark`; chroma v2.2.0 ships no style by that name and `styles.Get` answers an
unknown name with its fallback rather than with nothing, so the dark block has been
`swapoff` — a style nobody chose — since the generator was written. Of the 42 token types
`github` declares, swapoff defines 28 not at all, which is why 23 classes had no dark rule
and kept their light colours on `#1b1b1b`. Surveying every dark style chroma v2.2.0 ships
against `github`'s token set and the app's `--bg`, none clears both bars — the best
coverage comes with the worst contrast — and `github` itself puts 13 colours below 4.5:1
on the light code background, so the contrast half of M4-R3 bites in the light scheme too.
Once a coverage pass and a contrast pass exist, a second style earns nothing: toning the
one palette makes the class sets equal by construction and keeps a token the same hue in
both schemes ([#57](https://github.com/davison/md-notes/issues/57#issuecomment-5656179995)).

- **Trade-off:** the dark scheme's whole appearance changes, not only the 23 broken
  classes — bold cyan strings and yellow numbers are gone. That is more than #54 asked
  for. It was recorded rather than raised as a gate because the palette being replaced was
  never chosen by anyone: it is chroma's fallback for a name that does not exist. The
  light scheme changes only where `github` sat below AA, and — after the review — in one
  further place, `#008080` → `#006b6b`, an already-AA colour the AA floor had collapsed a
  distinction onto.
- **Rejected:** filling the 23 gaps with a hand-picked map, the shape #54 suggests (it
  fixes the symptom, leaves the generator asking for a style that does not exist, and
  leaves the sub-AA entries in both schemes); picking a dark style whose coverage is a
  superset of `github`'s (none exists in chroma v2.2.0); upgrading chroma for a real
  `github-dark` (it would still need both passes, and a dependency bump with five other
  tasks in flight); duplicating the `--bg` values in the generator with a test that they
  match `style.css`, rather than reading them out of it.

The rule the decision states — a colour's target is the contrast it holds against the
light page, floored at AA — turned out to measure neither visibility against a background
nor difference from another colour, and the review found both consequences. The three
rules that answer them — a cap at the contrast a colour can reach while still carrying its
own chroma, a minimum absolute luminance delta for a token's own background, and sRGB
distance rather than contrast ratio for keeping two colours apart — are the review's, and
are described under [What the reviews and QA changed](#what-the-reviews-and-qa-changed).
Two of the review's numbers in the survey above were wrong and are corrected below.

### The sync page's unsupported shape rests on a measured lost update

The page had to say why two daemons over one folder on one machine is unsupported. The
port clash is not the reason — give the second daemon its own `--port`, `--state` and
`--token-file` and it starts happily against the same folder. What is actually
unsupported is underneath: `internal/source.Store` holds one process mutex over reads and
saves and its revision tokens are session-local
([the M2 decision](https://github.com/davison/md-notes/issues/18#issuecomment-5576251236)),
so two daemons share neither and coordinate through nothing. Measured rather than
asserted, over 200 rounds of two concurrent saves against one note: none lost through a
single daemon, and 23 of 200 lost through two — later 19, 23 and 29 across three runs on
two machines, which is what the page states
([#58](https://github.com/davison/md-notes/issues/58#issuecomment-5656137446)).

- **Trade-off:** the page explains a race rather than stating a rule, and a reader who
  only wanted "don't do this" reads two paragraphs to learn why. Worth it, because the
  same reader is told correctly that two daemons on two *machines* over one Syncthing
  folder are fine, and the reason they differ — Syncthing standing between them, noticing
  the divergence and writing a conflict file — is the whole point of the page.
- **Rejected:** resting the claim on the port clash, which is not the problem and which a
  reader routes around in a minute; resting it on the source comment alone, when the shape
  can be run. The sequential case *is* caught — daemon B re-reads, mints a new token and
  returns 409 — so the page must not claim the second daemon never notices; it notices a
  change that has already landed and cannot notice one landing beside it.

### Everything the sync page says about Syncthing is attributed, and marked where it appears

The task forbade installing or configuring Syncthing, so the setup section is the one part
of the page no run backed. Rather than sprinkle hedges, the page marks the attributed
prose and says where it comes from, linking
[Syncthing's own documentation](https://docs.syncthing.net/); the sections either side —
what the daemon does with Syncthing's writes, conflict files in the navigator, the
unsupported shape — are checked against the binary and say so, so a reader can tell at a
glance which half was verified and which was read
([#58](https://github.com/davison/md-notes/issues/58#issuecomment-5656138457)).

- **Trade-off:** the setup instructions are less specific than a page that had actually
  run the wizard. They name the concepts — folder ID, the shared folder, the folder type,
  ignore patterns, an always-on peer — and point at the manual for the clicks.
- **Rejected:** writing step-by-step Syncthing instructions from memory, which would be
  the only unverified *and* unmarked prose on the page; omitting setup entirely, which
  [#53](https://github.com/davison/md-notes/issues/53) asks for by name.

The decision said "one marked section, and nothing else on the page depends on it". The
page ended up with two, and the review found the cost of the gap before the shape was
fixed — see [Deviations](#deviations).

### The build precompresses with Node's own zlib, and the daemon carries no compressor

A ~30-line Vite plugin over `node:zlib` writes `<file>.br` and `<file>.gz` beside every
build output over a kilobyte, at brotli quality 11 with the text mode and size hint and
gzip level 9. No new dependency; `//go:embed all:dist` picks the siblings up unchanged,
and a copy that came out no smaller than its source is not written
([#59](https://github.com/davison/md-notes/issues/59#issuecomment-5656189477)).

- **Trade-off:** the embedded bundle roughly doubles in file count and the binary grows
  by about 1.11–1.13 MiB (+7.1%), in exchange for a first page load that transfers about
  17 KB instead of 722 KB. `index.html` at 387 bytes is under the threshold and is served
  uncompressed, which is also what keeps the missing-sibling fallback on a path that runs
  in production rather than only in tests.
- **Rejected:** `vite-plugin-compression2` and its siblings (a dependency and a lockfile
  entry for what `zlib` already does in thirty lines); compressing on the fly in Go
  (`compress/gzip` at request time on every load, and no brotli in the standard library at
  all, so the better coding would need a third-party encoder in the binary); shipping only
  `.br` (the fallback would then be identity for any client without brotli, for a coding
  every HTTP client has had for twenty-five years).

One of this decision's two file counts was measured on the wrong tree and is corrected
below.

### The M2 decision that kept the editor eager is overturned, for the editor chunk only

M4-R7 asked for measurements before this was touched, and named the M2 decision on
[#19](https://github.com/davison/md-notes/issues/19) — `@codemirror/language-data`
statically imported so the toggle stays instant — as the thing to overturn or reaffirm.
`./editor` is now a dynamic `import()` taken on the first toggle; `language-data` itself
is untouched and still travels inside that chunk, so a note in any language still
highlights without a list to maintain. Headless Chromium against the built binary over
loopback, CDP `encodedDataLength` for the bytes and an in-page `MutationObserver` from the
keydown to the mounted `.cm-editor` for the timing, seven runs per cell: eager JavaScript
in view mode 710,620 B → 42,635 B, first-load asset bytes 208,355 B → 16,580 B, first
toggle 37.6 ms → 55.6 ms. Both conditions the requirement set were met, so the dynamic
import landed
([#59](https://github.com/davison/md-notes/issues/59#issuecomment-5656192500)).

- **Trade-off:** the first `Ctrl+E` of a page costs about 26 ms more and 209 KB rather
  than 11 KB crosses loopback to pay for it. Every later toggle in the page is unchanged —
  the module is held once it arrives — and view mode, which is how the app is mostly used
  and the only mode a phone or an e-ink tablet may ever see, stops paying for an editor it
  never opens. What made the M2 call right at the time was that lazy loading saved
  "130 KB gzipped once" against a first-toggle pause; precompression and the immutable
  cache moved both halves of that.
- **Rejected:** prefetching the chunk on idle after first paint (the toggle would stay at
  30 ms, but the bytes cross the wire on every cold load anyway, which is the cost the
  requirement asked to remove — and it would have made the measurement a fiction);
  `preact-iso`'s `lazy` with a Suspense boundary (it throws promises up to a router
  boundary this pane does not sit under); reaffirming the M2 decision, which the numbers
  do not support.

This is the first time this project has overturned a recorded decision from an earlier
milestone. The mechanism was the requirement itself: M4-R7 named the decision, named the
evidence that would overturn it, and put the measurement in front of the change.

### A name under the hashed assets directory that the bundle does not hold is a 404

Everything under `/assets/` is build output with a content hash in its name, so no
client-side route lives there and a name that does not resolve cannot be a route either.
The single-page shell fallback stays exactly as it was for every other path
([#59](https://github.com/davison/md-notes/issues/59#issuecomment-5656546472)).

- **Trade-off:** narrowing the fallback costs the theoretical case of a future
  client-side route under `/assets/` — there is none, and there should not be one — and
  buys the case the lazy editor makes real: after an upgrade under an open tab the chunk
  the page asks for is gone, and `200 text/html` turned that into a MIME-type error in
  the console rather than the one status that says what happened. It also stops the `.br`
  and `.gz` siblings being answered with the shell, so there is exactly one URL per asset.
- **Rejected:** leaving it (silent, and the failure the reviewer proved is the one case
  where a name genuinely cannot be right); 404ing every unknown path, which is the
  single-page fallback and would break every deep link the app has.

### Clipping over the tailnet stays refused, and the daemon is not widened

M4-R6 left this open and [#51](https://github.com/davison/md-notes/issues/51) named it as
the one thing to decide. It was decided *within* the requirement, per the requirement's
own default: no Go code is touched, `remoteAllowed` keeps `POST /api/clip` and
`POST /api/roots` off the tailnet allow-list exactly as
[#39 decided](https://github.com/davison/md-notes/issues/39#issuecomment-5632388601), and
the extension reports the refusal accurately instead of routing around it
([#60](https://github.com/davison/md-notes/issues/60#issuecomment-5656174981)).

- **Trade-off:** a clip taken on a laptop pointed at the tailnet daemon cannot be saved,
  and the remedy is to clip from a browser on the daemon's own machine or to point the
  extension back at loopback. The popup says so before the Clip button is pressed rather
  than after the save fails, so the cost is a sentence read. In exchange the credential
  that crosses the tailnet still cannot write a new file anywhere the daemon has not
  already been told to serve, which is the whole of #39's reasoning.
- **Rejected:** adding `POST /api/clip` to `remoteAllowed`. It is defensible on its own —
  a clip writes one file into an existing notes root, not an arbitrary path — but it is a
  security decision about what a tailnet credential may do, and a task whose requirement
  says the opposite is not where it should be taken. A "not yet" box in
  [the extension page](../extension.md#a-daemon-reached-over-the-tailnet) says it is a
  decision not taken rather than an oversight.
- **Rejected:** a separate "this daemon is remote" checkbox on the options page to drive
  the header. The daemon URL already carries the fact, and a second source of truth for it
  is a second thing to get wrong. `isLoopbackUrl` classifies the URL, and a URL that will
  not parse is treated as remote — the safe way round, since the cost of that mistake is a
  token travelling to the address the user typed, while the reverse is the silent `401`
  this task exists to end.

### The tailnet end-to-end suite runs over plain HTTP

The suite starts the daemon with `--tailnet-host <name>:<port>` and Chromium with
`--host-resolver-rules`, so every request carries the tailnet `Host` and lands on the
daemon's own loopback listener. The daemon's tailnet rule is keyed on the `Host` header
alone, so TLS changes nothing the extension or the guard decides
([#60](https://github.com/davison/md-notes/issues/60#issuecomment-5656176349)).

- **Trade-off:** the daemon's session cookie carries the `__Host-` prefix, so a browser
  will not accept it over plain HTTP. The redirected tab therefore lands on the daemon's
  login page rather than on the rendered note, and the test asserts the tab reached the
  note's address with the recorded status `opened` and no badge — not that the note
  rendered. That boundary is the right one: sending the tab to the right URL is the
  extension's job and logging in is the app's, and the suite's header comment says so.
  The reviewer then made the stronger assertion independently behind a TLS-terminating
  proxy, and the note itself renders
  ([#66](https://github.com/davison/md-notes/pull/66#issuecomment-5656253807)).
- **Rejected:** a Node HTTPS proxy with a generated self-signed certificate plus
  `--ignore-certificate-errors`, which buys the stronger assertion with an `openssl`
  prerequisite the other two suites do not have, for a claim about the *app's* login flow
  rather than about M4-R6.

### Search and tags become a second tab in the same drawer

The coordinator left the phone's home for search and tags to the implementer and recorded
it as such. [#34](https://github.com/davison/md-notes/issues/34) had put up three options:
a second tab in the drawer, a right-hand drawer of its own, or a bottom sheet. One drawer
means one modal, one focus trap, one Escape handler, one backdrop and one place where
"closes on selection" is decided; both panes stay mounted behind the tabs, so a search and
its results survive a trip to the tree and back, and the top bar's magnifier answers
"reachable from the top bar" in one tap
([#61](https://github.com/davison/md-notes/issues/61#issuecomment-5656475004)).

- **Trade-off:** search and the navigator cannot be seen at once, which on a 390 px screen
  they could not have been anyway. The real cost is the tab bar: two taps from the tree to
  the tag list where a right-hand drawer would have been one from the note. The magnifier
  buys back the common case.
- **Two further choices inside it.** Choosing a *tag* does not close the drawer, it moves
  to the Notes tab: "closes on selection" is about opening a note, since the drawer covers
  the note the tap just opened, whereas a tag's whole effect is on the tree, and closing
  would hide the result of the tap. And the drawer is `role="dialog"` with `aria-modal`
  rather than a native `<dialog>`: `showModal()` would give the trap and Escape for
  nothing, but its top-layer placement and `::backdrop` fight a slide-in transition, and
  jsdom's support is thin enough that the vitest cover would have to stub it.
- **Rejected:** a right-hand drawer (a second modal surface, a second set of close paths,
  and the question of what two open drawers mean, to buy one tap on the tag list); a
  bottom sheet (the phone keyboard takes the bottom of the screen, which is exactly where
  the search box would be); a `matchMedia` hook to decide what to render (a second copy of
  a threshold the stylesheet already owns, out of step with it on every resize frame);
  unmounting the panes when the drawer closes (it throws away the search query and its
  results on every open and close, where `visibility: hidden` does the same job).

The `matchMedia` rejection is the one decision of this milestone that two other tasks then
inherited: #62's settings boot script and #61's own breakpoint-crossing close both cite it,
and the second of them solved its problem by asking the element what the stylesheet made of
it rather than repeating `960` in JavaScript.

### **Awaiting #62** — the e-ink settings, and what the tap-target rule is keyed on

Two decisions on [#62](https://github.com/davison/md-notes/issues/62) —
[where the settings live and why the light override is the *absence* of the dark palette](https://github.com/davison/md-notes/issues/62#issuecomment-5657570182),
and
[the tap targets keyed on a coarse pointer rather than on width](https://github.com/davison/md-notes/issues/62#issuecomment-5657572060),
with
[a correction](https://github.com/davison/md-notes/issues/62#issuecomment-5657608807) to
the second's premise — are written up here when #70 merges.

### How the milestone was run

One decision belongs to the coordinator rather than to any task
([#55](https://github.com/davison/md-notes/issues/55#issuecomment-5656096967)): the scope
(six captures adopted one per task, the tab title added on the operator's direct request,
the remaining M2/M3 captures left open), the sequencing, the carried-over standing merge
confirmation, and three defaults fixed in the requirements for the operator to overrule.

- **Trade-off:** five concurrent tasks mean five review loops at once; the areas are
  disjoint enough that rebases should be trivial, and they were. The operator is present
  intermittently, so defaults are recorded rather than gated — at the risk of one of them
  being wrong. None of the three was taken on the default alone: M4-R7's named the
  evidence that would overturn it and the measurements overturned it, M4-R6's refusal was
  reaffirmed with the reasoning written down next to #39's rather than inherited in
  silence, and M4-R1's open choice was argued against the two alternatives
  [#34](https://github.com/davison/md-notes/issues/34) had put up.
- **Rejected:** folding the tab title into the phone-layout task (the operator asked for a
  task of its own, and it is a smaller deliverable that unblocks nothing).

## Deviations

### #60 edited eight lines of a file outside its brief

The brief gave #60 `extension/` and `docs/extension.md`. It also edited the paragraph in
`docs/introduction.md` carrying the one-sentence version of the claim the task falsifies —
"even opening a local file fails, because the extension asks for the roots list without
the token" — because leaving it would land a documented contradiction on `main` in the
same merge that fixes the thing it describes. The edit is confined to that paragraph and
re-points it at the extension page's new subsection
([#60](https://github.com/davison/md-notes/issues/60#issuecomment-5656176349)).

The M3 record's gap row saying the same thing was deliberately *not* touched: it is a
sealed record of what was true at M3, and
[#51](https://github.com/davison/md-notes/issues/51) is its gap row. That is the
[#40 decision on sealed records](https://github.com/davison/md-notes/issues/40) applied by
a task that had every reason to reach for the edit, and it is why the annotation at the
end of this milestone is an annotation rather than a rewrite.

### #61's drawer is a module the plan did not name

The plan put the drawer's state, the top bar's two buttons, Escape, the backdrop, the
focus handling and the Tab trap in `ui/src/root-view.tsx`. They landed in a new
`ui/src/drawer.tsx`, which the plan did not name; the review found it as finding 3.
`root-view.tsx` was 178 lines and already carried the root's data loading, the live-update
wiring, the tag filter, the tab title and the three panes, so adding a modal's whole
machinery would have made one file the answer to two unrelated questions — and the
drawer's rule, that the stylesheet decides what exists at which width and the module holds
only the state, needs a header comment that belongs at the top of its own file rather than
halfway down someone else's. Nothing about the requirement or the plan's substance
changed: the same behaviours, the same `.panes` wrapper, the same `display: contents` at
wide widths
([#61](https://github.com/davison/md-notes/issues/61#issuecomment-5657377326)).

The same comment records a second, smaller one. The plan said nothing about the
live-update notice, which sat inside the navigator and so moved behind the drawer. It is
now rendered twice, with the stylesheet showing the navigator's copy at wide widths and a
copy above the note at narrow ones. Duplicated markup was chosen because only one copy is
ever laid out — so only one is ever in the accessibility tree — and because moving the
single instance between two parents at a breakpoint would have needed the `matchMedia`
this task's own decision rejected. The cost is that a DOM query for the notice now finds
two nodes.

### #58's attribution ended up in two marked places, not one

The attribution decision said every Syncthing claim would sit "in one marked section, and
nothing else on the page depends on it". The review's finding 3 was right that the page
did not do that, and finding 1 showed the cost — the page's single factual error landed in
exactly the unmarked half the decision had promised would not exist. Moving the conflict
mechanics up into the setup section would have been worse for the reader: what a
`sync-conflict` file is belongs beside what the navigator does with it. So the shape
changed rather than the principle. "When two devices edit the same note" now opens with
its own attribution line, its Syncthing paragraphs are italicised so the boundary is
visible sentence by sentence, and the preamble says attribution appears per section
([#58](https://github.com/davison/md-notes/issues/58#issuecomment-5656269097)).

- **Trade-off:** two markers to keep honest instead of one, and a section that alternates
  between attributed and verified prose.
- **Rejected:** relocating the conflict mechanics into the setup section, a hundred lines
  from the behaviour they explain; leaving the preamble's "marked as such where it
  appears" to paper over four unmarked claims, which is what the review caught.

### #58's conflict-file device ID was a claim the manual does not support

Found by the implementer while citing the corrections, with no finding asking for it. The
page said the short device ID in a conflict name is "the device ID that produced the
losing copy". Syncthing's public documentation introduces `<modifiedBy>` without defining
it, and the source passes `file.ModifiedBy` — the *incoming* version's last modifier,
which is the copy that keeps its name. Rather than resolve a question the manual leaves
open, the page now calls it "a short device ID" and tells the reader not to read identity
off the file name
([#58](https://github.com/davison/md-notes/issues/58#issuecomment-5656267935)).

Tasks [#56](https://github.com/davison/md-notes/issues/56),
[#57](https://github.com/davison/md-notes/issues/57) and
[#59](https://github.com/davison/md-notes/issues/59) recorded no deviations from their
plans. **Awaiting #62** for its own.

## Corrections to the record itself

Six times a review finding was not about the code but about what the record said the code
did, and one of them is the sharpest item of the milestone. In every case the correction
was written as a new comment rather than by editing the one it corrects, so the finding
and its answer stay legible to whoever reads the issue next.

- **A trade-off written as though it covered cases it did not.** The decision on where the
  tab title is owned excused a note opened straight into the editor being titled by its
  file name, on the ground that "the tab is accurate rather than merely less specific".
  The review of [#64](https://github.com/davison/md-notes/pull/64#issuecomment-5656219635)
  found two cases that sentence did not excuse — a live update landing while the editor is
  open, and a conflict resolved with **Load the file** — in neither of which the tab falls
  back to the file name: it keeps the title of text that is no longer under the editor and
  no longer on disk, which is the tab naming a note something it is not called. The
  correction quotes the sentence, says what it should have said, and takes the more
  expensive fix rather than the cheap one the review offered: the pane now asks the daemon
  for the title itself whenever the text under the editor is replaced from outside it,
  which the session's `generation` counts, so the original trade-off disappears rather than
  being restated
  ([#56](https://github.com/davison/md-notes/issues/56#issuecomment-5656273163),
  [`9930d02`](https://github.com/davison/md-notes/commit/9930d02)).
- **And then the correction's own enumeration was wrong.** It said the generation is
  bumped by "a conflict resolved either way". Only one way bumps it: `loadFile()` does,
  `keepDraft()` does not, so after **Keep my draft** the tab holds the title the pane
  already had until the rendered view next runs. Round two of the review caught it and
  named it as the same shape as the sentence the first correction exists to fix — a
  justification that reads as covering a case it does not
  ([#56](https://github.com/davison/md-notes/issues/56#issuecomment-5656348152)). The
  behaviour is right and falls inside the residual the same comment records; it is the
  parenthetical list that was wrong.
- **Two numbers in the chroma survey.** "swapoff styles 28 fewer token types than
  `github`" is not what 28 counts: 28 is how many of `github`'s types swapoff does not
  define, which is what the survey table's own column header says. The count of entries
  swapoff *declares* is 16 to `github`'s 42. And the vulcan row read 14 sub-AA entries
  where the count is 13, by the same method that reproduces the other seven rows exactly.
  The reviewer checked both against the pinned chroma and was right on both; neither
  changes the decision or the code
  ([#57](https://github.com/davison/md-notes/issues/57#issuecomment-5656357050)). The
  sentence stood in three places — the plan, the decision comment and
  [`5451284`](https://github.com/davison/md-notes/commit/5451284) — and, round two found,
  in a fourth the correction had not named: the PR body, which is what `gh pr view` shows
  at merge.
- **A file count measured on the wrong tree.** The precompression decision's "from
  1,720,143 bytes in 124 files" is not `origin/main`'s `ui/dist`, which the reviewer
  measured at 117 files / 1,718,952 B. Both figures in that trade-off were measured on the
  *post-#59* UI tree, once with the plugin and once without it — the like-for-like pair for
  "what does precompression cost", but not what the wording implies. The 124 → 346 delta is
  the 111 `.br` and 111 `.gz` copies the reviewer independently counted, and the arithmetic
  self-verifies; nothing the decision turned on changes
  ([#59](https://github.com/davison/md-notes/issues/59#issuecomment-5656348068)).
- **A retry that could never retry, asserted as working in four places.** This is the
  milestone's sharpest record item.
  [`78b2a6e`](https://github.com/davison/md-notes/commit/78b2a6e)'s message — reproduced
  in the round-one reply and in `docs/introduction.md` — claimed that the **Try again**
  button on a failed editor chunk would refetch, "a module that never loaded was never
  registered, so the request is made again rather than replayed from a cache". That is the
  opposite of what browsers do: a module script whose fetch fails leaves a **null entry**
  in the module map, and every later `import()` of that URL rejects against that entry
  without issuing a request. The null entry *is* the registration. Round two of the review
  of [#68](https://github.com/davison/md-notes/pull/68#issuecomment-5656500255) proved it
  twice — once with the chunk answered by the single-page fallback, once with CDP failing
  the first request and passing every later one, the friendliest case a retry could have —
  and the one test covering the retry passed only because `vi.mock` re-invokes its factory
  on each import, so it was pinning a mock artefact rather than browser behaviour. The
  button is gone; the failure now offers a reload, which is the only thing that cures it,
  and that is verified end to end. All four places that carried the false claim were
  corrected
  ([#59](https://github.com/davison/md-notes/issues/59#issuecomment-5656545448),
  [`637302c`](https://github.com/davison/md-notes/commit/637302c)).
- **A PR body that contradicted its own diff, caught before the merge.** Round two of the
  review of [#67](https://github.com/davison/md-notes/pull/67#issuecomment-5656425743)
  blocked on the description alone, with no finding left against the code: it said
  `style.css` was unchanged when the PR changes it, said the light scheme changes only
  where `github` was below AA when the PR deliberately moves an already-AA colour across
  six classes, listed dark colours that three commits had superseded, described a `verify`
  rule that had been replaced by the one that fixed the blocking defect, and still carried
  the "28 fewer token types" sentence the correction had not reached. The reviewer said it
  would approve on the body alone, and round three checked all five against the diff. It is
  the only time in four milestones a review has blocked a merge on a PR description with
  nothing against the code.

**Awaiting #62** for the correction on its tap-target decision's premise.

One correction this milestone could not make, and which is therefore recorded here rather
than in the place it belongs. [#68](https://github.com/davison/md-notes/pull/68)'s
**Measurements** table is the pre-rebase run: it gives 16,580 B of assets on the first
load, a 42,635 B eager chunk and a 17,580,295 B binary, where the merged tree gives
17,049 B, 43,776 B and about 17,539,232 B — the bundle grew slightly with #56's and #57's
own work. Round three raised it as a nit
([#68](https://github.com/davison/md-notes/pull/68#issuecomment-5656631839)) and it was
not taken. The reply comment and `docs/introduction.md` carry the current figures, so the
record as a whole is honest; the body is what a reader meets first, and it now disagrees
with the page. The same round left two nits of the same kind unfixed in the tree, and they
are in [Known gaps](#known-gaps-at-the-boundary).

**A note on commit SHAs.** Every SHA quoted in a review comment or a reply on
[#64](https://github.com/davison/md-notes/pull/64)–[#69](https://github.com/davison/md-notes/pull/69)
is the one it had on the task branch, which the rebase merge rewrote. This document quotes
the SHAs on `main`. [#68](https://github.com/davison/md-notes/pull/68) was rewritten twice
— once by its own rebase onto `main` mid-review, after #56, #57 and #58 had landed, and
once by the merge — so four of
its round-one and round-two SHAs have two predecessors; both are given. The mapping, for
anyone following the comments into the history:

| On the branch | On `main` | What it was |
|---------------|-----------|-------------|
| `b3fa582` | [`0cf238f`](https://github.com/davison/md-notes/commit/0cf238f) | the static `<title>` before the bundle runs |
| `d9ff145` | [`7d0b1cf`](https://github.com/davison/md-notes/commit/7d0b1cf) | the tab title follows the open note |
| `c73a9df` | [`d438960`](https://github.com/davison/md-notes/commit/d438960) | one file-name fallback, not two |
| `5d4f62f` | [`6576aff`](https://github.com/davison/md-notes/commit/6576aff) | no root-slug flash on a note URL |
| `10150e7` | [`2f073c2`](https://github.com/davison/md-notes/commit/2f073c2) | a note that stops rendering gives up its title |
| `f3e84e2` | [`9930d02`](https://github.com/davison/md-notes/commit/9930d02) | the tab follows the note under an open editor |
| `71945dc` | [`17770f3`](https://github.com/davison/md-notes/commit/17770f3) | the editor and delete cases covered |
| `560fdea` | [`5055539`](https://github.com/davison/md-notes/commit/5055539) | the request the editor's title does not make |
| `7fb3d86` | [`d6d1be3`](https://github.com/davison/md-notes/commit/d6d1be3) | the sync and offline workflow page |
| `58980c1` | [`5320639`](https://github.com/davison/md-notes/commit/5320639) | the page linked from the README and the introduction |
| `d97a5a0` | [`99d103a`](https://github.com/davison/md-notes/commit/99d103a) | what Syncthing does with conflict copies |
| `7ef37b1` | [`9f9aac9`](https://github.com/davison/md-notes/commit/9f9aac9) | attribution and mechanisms |
| `612d1c0` | [`0a34b70`](https://github.com/davison/md-notes/commit/0a34b70) | pointers from Conflicts and Live update |
| `2a7362b` | [`dc8417e`](https://github.com/davison/md-notes/commit/dc8417e) | the last attribution gaps |
| `523572b` | [`8a83c94`](https://github.com/davison/md-notes/commit/8a83c94) | the token presented to a daemon off this machine |
| `3a4beec` | [`d18e30a`](https://github.com/davison/md-notes/commit/d18e30a) | the allow-list named rather than the token blamed |
| `595815e` | [`8568f9b`](https://github.com/davison/md-notes/commit/8568f9b) | Test connection off loopback |
| `d89ec1b` | [`7598653`](https://github.com/davison/md-notes/commit/7598653) | the tailnet e2e suite |
| `841615e` | [`acf9552`](https://github.com/davison/md-notes/commit/acf9552) | the extension against a tailnet daemon URL |
| `ddf2e41` | [`b26632f`](https://github.com/davison/md-notes/commit/b26632f) | the tailnet daemon URL is the `https` name |
| `d859c3f` | [`02d44fb`](https://github.com/davison/md-notes/commit/02d44fb) | a refusal attributed to the call refused |
| `4f26562` | [`d5be079`](https://github.com/davison/md-notes/commit/d5be079) | the second tailnet name earned, the daemon restored |
| `cb313ef` | [`5451284`](https://github.com/davison/md-notes/commit/5451284) | both schemes toned from one palette |
| `c8c15fc` | [`dc02186`](https://github.com/davison/md-notes/commit/dc02186) | visible tints, hues that do not wash out |
| `ef47d49` | [`88d158f`](https://github.com/davison/md-notes/commit/88d158f) | the `--bg` declarations gencss reads |
| `d9e1c41` | [`77df575`](https://github.com/davison/md-notes/commit/77df575) | what the Generic exemption exempts; `--fg` read |
| `cb39368` | [`5e60a16`](https://github.com/davison/md-notes/commit/5e60a16) | brotli and gzip copies at build time |
| `cc8921a` | [`745c734`](https://github.com/davison/md-notes/commit/745c734) | assets served cacheable and compressed |
| `3167cfd`, then `7f19ebd` | [`0e58f7b`](https://github.com/davison/md-notes/commit/0e58f7b) | the editor chunk on the first toggle |
| `f9c9877` | [`e837ceb`](https://github.com/davison/md-notes/commit/e837ceb) | the bundle paragraph rewritten |
| `5820441`, then `24ec354` | [`1df3311`](https://github.com/davison/md-notes/commit/1df3311) | scrub, type and hide what the asset path got wrong |
| `4e2f4f5`, then `6848e1a` | [`78b2a6e`](https://github.com/davison/md-notes/commit/78b2a6e) | a failed editor chunk surfaced |
| `07fa08f`, then `9472e7c` | [`dd951e1`](https://github.com/davison/md-notes/commit/dd951e1) | which toggle each timing describes |
| `1f8d613` | [`637302c`](https://github.com/davison/md-notes/commit/637302c) | the reload that works, not the retry that cannot |
| `1c4184c` | [`0190506`](https://github.com/davison/md-notes/commit/0190506) | one list of what the bundle holds, and a 404 |
| `5c841de` | [`59bba59`](https://github.com/davison/md-notes/commit/59bba59) | the CSS figure after the rebase |
| `5448dfb` | [`964f745`](https://github.com/davison/md-notes/commit/964f745) | the drawer navigator and a note that fills the screen |
| `cd1f059` | [`2bf7cc3`](https://github.com/davison/md-notes/commit/2bf7cc3) | the tag filter as a chip in the top bar |
| `93444da` | [`c7911db`](https://github.com/davison/md-notes/commit/c7911db) | the phone layout in the web UI section |
| `609edca` | [`217d845`](https://github.com/davison/md-notes/commit/217d845) | the unsaved deep draft clamped |
| `7825349` | [`113aeef`](https://github.com/davison/md-notes/commit/113aeef) | the drawer does not outlive its width |
| `302ab54` | [`3565433`](https://github.com/davison/md-notes/commit/3565433) | the live-update notice at phone widths |
| `a3acf5a` | [`3802e2f`](https://github.com/davison/md-notes/commit/3802e2f) | the breakpoint named in pixels |

## What the reviews and QA changed

Every PR went through at least two rounds; two went through three. The reviewer seat is
routed to the same identity as the author (pure solo tier), so each review is a comment
rather than a formal approval, and each merge carried the operator's standing confirmation
from [#35](https://github.com/davison/md-notes/issues/35#issuecomment-5632220363).

The habit the earlier milestones established held. Every review executed the code rather
than reading it, and the findings that mattered most were the ones a reproduction
produced: a tab measured through a `MutationObserver` while a file changed under an open
editor, a diff fence's luminance delta sampled off the painted page, twenty-eight timed
editor toggles against three separately built binaries, a module map defeated with CDP
request interception, a magnifier measured four pixels inside the viewport at an iPhone 14
width. Twelve findings blocked a merge across the six PRs; several of them are things the
author could not have found by reading their own diff, and of one — the retry that cannot
retry — the author says so in as many words.

- **The tab title ([#64](https://github.com/davison/md-notes/pull/64#issuecomment-5656219635)):**
  round one reproduced all fifteen of the PR's own browser checks independently and then
  found two blocking cases of the same mistake — the tab keeping a title the note no longer
  had, once when a live update lands under an open editor and once after a conflict
  resolved with **Load the file** — and pointed out that the recorded justification covered
  neither. Of the two smaller findings, one was a root slug flashing over a note's title
  with `/api/roots` delayed, and one was the browser's file-name fallback keeping the
  extension where the daemon's drops it. Round two
  ([#64](https://github.com/davison/md-notes/pull/64#issuecomment-5656332353)) verified all
  four by re-running its own fixtures, confirmed the added tests fail against the old code,
  measured the zero-request property the new trade-off rests on, and approved — leaving two
  non-blocking findings, one of which was that nothing *asserted* that property, so adding
  `revision` to one dependency list would have turned on a full render request per autosave
  with nothing failing. Both were taken.
- **The sync page ([#65](https://github.com/davison/md-notes/pull/65#issuecomment-5656228186)):**
  round one re-ran every md-notes claim on the page against a binary built from the branch,
  reproduced the two-daemon measurement at 29/200 against the PR's 23/200, and then checked
  the Syncthing half against Syncthing's published documentation and, where the
  documentation is silent, against its source. Two blocking findings came out of that.
  File versioning does not keep conflict copies out of the folder — versioning archives
  files superseded by *incoming* changes and is not on the conflict path at all, so a
  reader following the page's advice would have turned it on, kept getting
  `sync-conflict-` files, and gained a `.stversions` tree; and the setting that does
  suppress them, `maxConflicts: 0`, *deletes* the losing text rather than archiving it. The
  second was that the page never named the Syncthing folder type while its own Android
  framing — "read-mostly, with the occasional edit" — is exactly the phrasing that leads a
  reader to pick **Receive Only** on the phone, where local changes are not distributed and
  one tap reverts them. The implementer fetched each cited page and read the quoted text
  before touching the prose, and recorded the citations
  ([#58](https://github.com/davison/md-notes/issues/58#issuecomment-5656267935)). Round two
  ([#65](https://github.com/davison/md-notes/pull/65#issuecomment-5656324490)) re-fetched
  every one of them rather than take the citations on trust, re-read `moveForConflict` on
  Syncthing's current default branch, approved, and left four nits — of which the one that
  mattered was the last unmarked Syncthing claim on the page, the sentence that makes the
  two-machine shape feel safe, sitting three lines under a table of measurements where it
  reads as verified.
- **The extension over the tailnet ([#66](https://github.com/davison/md-notes/pull/66#issuecomment-5656253807)):**
  round one re-ran the whole surface behind a local TLS-terminating proxy — the rig #40's
  doc-synthesizer had used on PR #49 — so the assertion the implementer's suite had
  deliberately not made could be made independently: the redirected tab renders the note
  itself. It blocked on one finding, and it is the same class of defect the task existed to
  remove. The new tailnet subsection offered `http://laptop.ts.net:7337` as an alternative
  daemon address; the listener binds `127.0.0.1` and nothing else, so the daemon's own port
  is not reachable under the tailnet name at all, and a reader following the example would
  have configured an address with nothing listening. The clause came from generalising the
  extension's *own* e2e rig, which reaches a loopback listener under a tailnet `Host`
  because `--host-resolver-rules` puts it there — a thing only a test can do. Four of the
  five non-blocking findings were taken further than asked: a `loopback_only` misattributed
  to the registration was fixed rather than commented, and dead scaffolding for a second
  tailnet name was exercised rather than dropped. Round two
  ([#66](https://github.com/davison/md-notes/pull/66#issuecomment-5656309417)) put the
  attribution fix in front of a real browser with a stub daemon, appended a probe test to
  prove the test order was no longer load-bearing, and approved.
- **The code colours ([#67](https://github.com/davison/md-notes/pull/67#issuecomment-5656267135)):**
  three rounds, and the two blocking findings in round one are the milestone's clearest
  case of a measure being used for something it does not measure. The dark inserted-line
  diff tint was `#1a211a` on a `#1b1b1b` page — a luminance delta of 0.0029 — so a diff
  fence marked removed lines and not added ones, which is the `#191919` defect from #54
  under a different class name, blessed by a `verify` rule that compared the two schemes'
  contrast *ratios* to each other. And saturated dark hues washed to near-white, because
  moving a colour in HSL trades chroma for lightness: `NameTag` `#000080` became `#f2f2ff`,
  1.13:1 from the body text, so a YAML or HTML fence was a wall of near-white where it had
  been navy keys. A third, non-blocking, was the AA floor collapsing numbers onto variables
  in the *light* scheme, which was not broken. All three were fixed rather than recorded,
  and each fix came with a gate in the generator: a minimum absolute luminance delta for
  tints, a cap at the contrast a colour can reach while still carrying its own chroma, and
  sRGB distance — not contrast ratio — for keeping apart two colours the source palette
  tells apart. The review also asked for before-and-after screenshots of a dark fence set
  on the ground that these are taste calls in the operator's own daily reading surface and
  the honest way to close the gap is to let them see it; they are at
  [#67](https://github.com/davison/md-notes/pull/67#issuecomment-5656356154). Round two
  ([#67](https://github.com/davison/md-notes/pull/67#issuecomment-5656425743)) verified
  every fix by measurement, added an HTML fence of its own because HTML is the other
  language the wash-out flattened, said as a reader that the palette is now a pleasant
  daily surface — and blocked on the PR body alone. Round three
  ([#67](https://github.com/davison/md-notes/pull/67#issuecomment-5656479560)) checked the
  five contradictions against the diff, confirmed the refactor moved not one byte of the
  generated stylesheet, and approved.
- **The bundle ([#68](https://github.com/davison/md-notes/pull/68#issuecomment-5656297955)):**
  three rounds, every measurement reproduced against binaries the reviewer built from
  `origin/main`, from the branch, and from the branch with the import reverted, so the
  comparison isolated the import rather than the server. Round one blocked on a
  documentation figure that named the *rejected* build's first toggle as the shipped
  build's held-module case, understating it by about four times; and on a dynamic import
  with no rejection path, which left the pane on "Loading the editor…" for ever with an
  unhandled rejection escaping — reproduced by answering the editor chunk with the
  single-page fallback, which is exactly what happens when the daemon is upgraded under an
  open tab. Its five non-blocking findings are a catalogue of the edges a hand-written
  `Content-Length` on a compressed body opens: a range request over a compressed
  representation with no test, error paths that did not scrub `Content-Encoding`, `ETag`
  and a year-long immutable directive off a JSON 404, a `Content-Type` taken from the build
  machine's `/etc/mime.types`, and the precompressed siblings directly addressable and
  served undecodable. All five were fixed, and two were made reachable rather than asserted
  unreachable. Round two
  ([#68](https://github.com/davison/md-notes/pull/68#issuecomment-5656500255)) found the
  retry that cannot retry, above; it also found the build's list of compressible extensions
  and the server's list of known content types disagreeing, which became one list with a
  test that walks the real embedded bundle and fails on drift, and raised the unknown
  `/assets/` name that became this milestone's 404 decision. And it did something no other
  review round did: it rehearsed the certain rebase onto #56's merge, named both conflicts
  and the resolution, and confirmed the rebased tree typechecks and passes. Round three
  ([#68](https://github.com/davison/md-notes/pull/68#issuecomment-5656631839)) verified the
  reload end to end, checked the rebase had kept #56's behaviour by driving the tab title
  under an open editor against the running daemon, and approved. Its own absolute timings
  ran about 1.8× the recorded ones throughout; it said so, calibrated against a build of
  `origin/main` on the same host to show the factor was the environment, and held the prose
  to the *ratios* it turns on rather than to the numbers.
- **The phone layout ([#69](https://github.com/davison/md-notes/pull/69#issuecomment-5656598085)):**
  round one blocked on a state the PR's own sweep did not reach: an unsaved draft on a
  deeply-nested note is named in the compact top bar with nothing to clamp it, so the bar
  wrapped to two lines and pushed the magnifier off the screen — measured at 4.7 px of a
  41 px target inside an iPhone 14 viewport. M4-R1 requires search and tags to be reachable
  from the top bar, and in that state they are not; and it is exactly the case
  [#34](https://github.com/davison/md-notes/issues/34) was captured for, since a dropped
  SSH forward is what leaves an unsaved draft. The review supplied the four-line fix it had
  verified and asked for a test pinning the bar's contents as a *set*, so that an element
  added later without an answer in the narrow block fails a test rather than being found on
  a phone. Its other findings were a drawer whose `role="dialog"` and `aria-modal` outlived
  the breakpoint when a desktop window was dragged wide, the unrecorded module deviation,
  the live-update notice moving behind the drawer, and "below 60rem" being ambiguous in a
  stylesheet whose root font is 15 px while a media query's `rem` is 16. All five were
  taken, two of them further than asked. Round two
  ([#69](https://github.com/davison/md-notes/pull/69#issuecomment-5657482484)) reproduced
  the round-one state exactly, emptied the new rule in the live cascade to prove the clamp
  and not the fixture was doing the work, re-ran the wide layout against a build of the
  branch point in five states to byte-identical screenshots, and re-ran #56's tab title and
  #59's lazy editor at all four phone profiles after the rebase. It approved with two nits,
  neither taken.

Two findings changed something beyond their own PR. The review of #68 proving that a
failed module import can never be retried is the reason the pane offers a reload and
carries a comment saying why, and the reason `/assets/` answers 404 — a decision about the
server that exists because of what a browser does. And the review of #67 blocking on a
description with nothing against the code set a bar the rest of the milestone was held to:
round three of #68 and round two of #69 both audited their PR bodies against the diff, and
both found them stale.

**Awaiting QA.** Independent QA's verdicts and findings, and the coordinator's disposition
of them, are recorded here when they arrive.

## Known gaps at the boundary

The four M2 and M3 captures this milestone's tasks adopted —
[#32](https://github.com/davison/md-notes/issues/32),
[#34](https://github.com/davison/md-notes/issues/34),
[#51](https://github.com/davison/md-notes/issues/51) and
[#54](https://github.com/davison/md-notes/issues/54) — are closed, and
[the M3 record's gaps table](3-clipper-authentication-and-tailnet.md#known-gaps-at-the-boundary)
is annotated to say so. What remains true and will surprise someone who has not read this
far:

| Gap | Where it is recorded |
|-----|----------------------|
| A reader who edits their **own** H1 sees the tab follow it only when the rendered view next runs: their save does not replace the text under the editor, so the pane does not re-ask. **Keep my draft** is the same case. Closing it means forking `internal/render`'s title rule into the browser, which is refused | [#56](https://github.com/davison/md-notes/issues/56#issuecomment-5656273163), [#56](https://github.com/davison/md-notes/issues/56#issuecomment-5656348152) |
| The closest pair of colours the generator enforces is `LIMIT=42` — numbers against variables — at 0.0704 of sRGB distance against a 0.068 floor. Two shades of teal rather than two colours, which is what `github` itself offers there; the floor is 85% of the distance the source pair had, so it is "no worse than `github`" rather than comfortable | [#67](https://github.com/davison/md-notes/pull/67#issuecomment-5656425743) |
| The dark palette is new, not a fix to the old one: the scheme that shipped for three milestones was chroma's fallback for a style name that does not exist. `TestNamedStyleRefusesASubstitute` is written so that a chroma release adding a real `github-dark` fails with a message saying the test has lost its subject | [#57](https://github.com/davison/md-notes/issues/57#issuecomment-5656179995) |
| The Syncthing half of the sync page is read, not run — nothing here installs or configures Syncthing, and the page marks which half is which. Two daemons over one folder on one machine lose an update in between one round in seven and one in ten, silently, with both saves reporting success | [the sync page](../sync.md), [#58](https://github.com/davison/md-notes/issues/58#issuecomment-5656137446) |
| Syncthing's `maxConflicts: 0` deletes the losing text rather than archiving it, and file versioning does not keep conflict copies out of the folder at all. The page says so and tells the reader not to reach for either | [#65](https://github.com/davison/md-notes/pull/65#issuecomment-5656228186), finding 1 |
| The short device ID in a Syncthing conflict file name is not defined by Syncthing's manual, and the source passes the *incoming* version's last modifier — the copy that keeps its name. The page declines to say whose it is | [#58](https://github.com/davison/md-notes/issues/58#issuecomment-5656267935) |
| The binary is about 1.11–1.13 MiB larger (+7.1%) for the brotli and gzip copies it now embeds, and the first `Ctrl+E` of a page costs about 25–30 ms more than it did | [#59](https://github.com/davison/md-notes/issues/59#issuecomment-5656189477), [#59](https://github.com/davison/md-notes/issues/59#issuecomment-5656192500) |
| An editor chunk that fails to load can only be cured by reloading the page. No retry is possible: a module script whose fetch fails leaves a null entry in the browser's module map, and every later `import()` of that URL rejects against it without a request | [#59](https://github.com/davison/md-notes/issues/59#issuecomment-5656545448) |
| `TestUITypesCoverTheBundle` skips silently when `ui/dist` is empty, so a bare `go test ./...` does not run it; `make check` and `make test` are what make it mean anything | [#68](https://github.com/davison/md-notes/pull/68#issuecomment-5656631839) |
| `uiSibling`'s comment in `internal/server/server.go` still says the precompressed copies "fall through to the app like any other unknown path". Since the 404 decision they do not: every sibling the build writes is under `assets/` and 404s. The test that pins the behaviour was updated; the comment above it was not | [#68](https://github.com/davison/md-notes/pull/68#issuecomment-5656631839), nit 1 |
| `ui/src/style.css`'s narrow-breakpoint comment still does not say that a media query's `rem` is the initial 16 px, so `60rem` is 960 — the trap the same review had just removed from the introduction, one file over, while the next comment in the same block computes `rem` against the app's 15 px root | [#69](https://github.com/davison/md-notes/pull/69#issuecomment-5657482484), finding 7 |
| [#69](https://github.com/davison/md-notes/pull/69)'s Verification block is the round-one one, under a sentence saying it is not: it reports 153 checks and the pre-review count of new vitest cases, where the post-review sweep is 192 checks and the branch adds 15 UI tests in total | [#69](https://github.com/davison/md-notes/pull/69#issuecomment-5657482484), finding 6 |
| Clipping over the tailnet is refused by design, so a clip taken on a laptop pointed at the tailnet daemon cannot be saved; the popup still offers the Clip buttons under such a URL, under a line saying they are refused, because hiding them would make the popup lie about what the extension is for | [#60](https://github.com/davison/md-notes/issues/60#issuecomment-5656174981), [#66](https://github.com/davison/md-notes/pull/66#issuecomment-5656253807) |
| The tailnet end-to-end suite asserts that the redirected tab reached the note's address, not that the note rendered: over plain HTTP the `__Host-` session cookie cannot be set. The reviewer made the stronger assertion behind a TLS-terminating proxy; the suite in the repository cannot | [#60](https://github.com/davison/md-notes/issues/60#issuecomment-5656176349) |
| On a phone the navigator and the search-and-tags pane cannot be seen at once, and it is two taps from the tree to the tag list. The live-update notice is duplicated markup, one copy per width, so a DOM query for it finds two nodes | [#61](https://github.com/davison/md-notes/issues/61#issuecomment-5656475004), [#61](https://github.com/davison/md-notes/issues/61#issuecomment-5657377326) |
| `TestBurstIsOneBatch` in `internal/watch` is still flaky in CI. It recurred twice on 2026-09-13, once on a **documentation-only** commit, which is what rules out a regression and leaves the debounce race | [#46](https://github.com/davison/md-notes/issues/46) |

**Awaiting #62** for the e-ink gaps, including the one the record already knows it cannot
close: a device that reports a fine pointer and hover gets no enlarged tap targets at all,
and the failure mode is silent
([#62](https://github.com/davison/md-notes/issues/62#issuecomment-5657572060)).

Milestone two's and milestone three's own boundary notes still stand, minus the two this
milestone discharged — the bundle served uncompressed and uncacheable, and the extension
useless under a tailnet daemon URL. The `localStorage` draft mirror is still one record per
note shared by every tab, a directory whose only files are *ignored* is still not watched,
and the editor still cannot create, rename or delete a note
([the M2 record](2-editor-autosave-and-live-update.md#known-gaps-at-the-boundary),
[the M3 record](3-clipper-authentication-and-tailnet.md#known-gaps-at-the-boundary)).

Eight captures stay open across the three earlier milestones —
[#30](https://github.com/davison/md-notes/issues/30),
[#31](https://github.com/davison/md-notes/issues/31),
[#33](https://github.com/davison/md-notes/issues/33),
[#45](https://github.com/davison/md-notes/issues/45),
[#46](https://github.com/davison/md-notes/issues/46),
[#47](https://github.com/davison/md-notes/issues/47),
[#48](https://github.com/davison/md-notes/issues/48) and
[#50](https://github.com/davison/md-notes/issues/50) — and this milestone added **none**.
That is the first time, and it is worth saying why rather than treating it as a virtue: six
of its seven implementation tasks were captures being paid off, every blocking review
finding was fixed inside the PR that raised it rather than deferred, and the handful of
nits the reviews left on the table were judged too small to file. Whether that judgement
was right is visible in [Corrections](#corrections-to-the-record-itself) and in the three
rows above that a capture would have carried.

## Where the record is silent

- **Nothing says why the two settings the e-ink work adds are the only two.**
  **Awaiting #62.**
- **The palette's aesthetics were settled by two model readers.** The decision on toning
  both schemes from one palette is argued on coverage and contrast, which are measurable,
  and the three rules that came out of the review are argued on what a contrast ratio does
  not measure, which is also measurable. But the question the review itself raised — these
  are taste calls in the operator's own daily reading surface — was closed by the reviewer
  reading the fences and saying "yes, I would be happy to have this as a daily surface"
  ([#67](https://github.com/davison/md-notes/pull/67#issuecomment-5656425743)). The
  screenshots are on the record so the operator can disagree; nothing in the milestone
  records their having looked.
- **The 1 KB precompression threshold, the 0.02 tint floor and the 85% separation ratio
  are stated, not argued.** Each is a constant with a defensible story and none has a
  comment weighing it against a neighbouring value. The separation floor's reply says
  plainly what it buys — "no worse than `github`" — and that the palette regenerates if it
  reads badly in daily use, which is the closest any of the three comes to an argument.
- **The phone breakpoint is inherited, not chosen.** `60rem` was in the stylesheet before
  this milestone; #61 rebuilt what happens below it and #62 keyed a media feature beside
  it, and neither asked whether 960 px is the right place for the layout to change. The one
  thing the milestone did establish about it is that the number is ambiguous in a
  stylesheet with a 15 px root, and that trap is still in the stylesheet.
- **No requirement asked where the device sweeps belong.** #61's and #62's Playwright
  harnesses drove the built binary at real device profiles and produced the measurements
  both requirements turn on, and both deliberately live in the run's scratchpad rather than
  the repository, on the ground that the project carries no e2e dependency and adding one
  is not the task's business. That is the same answer M3 recorded for the extension's
  end-to-end suites, reached independently, twice, by tasks that had no reason to consult
  it — and the record still does not say where such a test belongs, or what it costs that
  the one measurement nobody can re-run is the one that proves the requirement.
- **The operator has still not seen any of this.** The standing merge confirmation carried
  over from M3, so every trade-off in this document was struck between an implementer and a
  reviewer sharing one identity. This milestone's subject is more exposed to that than the
  three before it: a colour palette, a phone layout and an e-ink reading surface are judged
  by a person using them, and the two checks that would judge them — the operator's own, on
  the phone and on the Boox — are explicitly not closure gates. The record cannot say what
  they will find.
