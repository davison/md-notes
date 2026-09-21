# M9 — Flowcharts, drawn by the daemon

Tracking issue: [#169](https://github.com/davison/md-notes/issues/169). Its
implementation tasks are merged on `main` at
[`c5ab85a`](https://github.com/davison/md-notes/commit/c5ab85a). The milestone was opened
at 18:51:29Z on 2026-09-21, and its last implementation task merged at 22:04:24Z the same
day.

Independent QA graded `main` at
[`6e735d5`](https://github.com/davison/md-notes/commit/6e735d5) and found M9-R2, M9-R3 and
M9-R4 satisfied. M9-R1 was **not** satisfied: a search hit below a diagram landed about two
screens from its target on a note's first open. M9-R5 was untestable, because this task had
not merged
([#169](https://github.com/davison/md-notes/issues/169#issuecomment-5767706977)). A fix task,
[#177](https://github.com/davison/md-notes/issues/177), was raised for M9-R1 on the
coordinator's disposition
([#169](https://github.com/davison/md-notes/issues/169#issuecomment-5767721928)), and QA's
superseding verdict on `c5ab85a` found M9-R1 satisfied
([#169](https://github.com/davison/md-notes/issues/169#issuecomment-5768247525)). M9-R5 is
delivered by the pull request that carries this record, and a superseding verdict is owed
after it merges.

## Goal and outcome

On 2026-09-21 the operator asked for mermaid to be vetted, so that the diagrams in the notes
could be drawn. The coordinator's scope decision records what the vetting found and what the
operator chose
([#169](https://github.com/davison/md-notes/issues/169#issuecomment-5765785737)):

- The mermaid-js library has 14 published advisories. Two of them, CVE-2025-54880 and
  CVE-2025-54881, are critical cross-site scripting under its default
  `securityLevel: "strict"`.
- In md-notes, any script on the app's origin has the whole API. It can read and write notes
  and register roots.
- Notes are not all trusted, because the clipper writes web content and a root can be any
  folder.
- Running mermaid in the page safely would need a sandboxed frame with an opaque origin. The
  operator judged that not worth it, and asked instead for a renderer of the project's own,
  drawn on the server and shown as an image.

Flowcharts were the only mermaid type in the operator's notes, so they are the whole scope.
Every other diagram type stays a code block, and comes back as a milestone when someone needs
one.

What a reader has now: a fenced `mermaid` block that is a `flowchart` or a `graph`, within a
documented subset, reads as a diagram. The daemon parses it into plain data, lays it out, and
writes an SVG built only from elements it chooses, with every piece of the note's text
escaped. The page shows that SVG through `<img>`, never inline, in the light, dark or e-ink
palette the page is using. The drawing follows the note on disk, and its box is reserved
before it loads, so a search hit below it lands where it should. A block outside the subset,
one over a bound or past the 2 s deadline, and every other diagram type all stay the code
block they were before, and nothing is ever half-drawn.
[Flowcharts](../introduction.md#flowcharts) is the reader's account of all of it.

### What shipped, in the order it merged

Each coordinator disposition names the head of its pull request as the merge point. Each
pull request landed on `main` by rebase, so the commit on `main` differs from the head the
review approved; both are given.

**A flowchart renderer in the daemon**
([#170](https://github.com/davison/md-notes/issues/170),
[PR #173](https://github.com/davison/md-notes/pull/173), approved at `95da1d4`, on `main` as
[`c1aacea`](https://github.com/davison/md-notes/commit/c1aacea)). This added a new package,
`internal/diagram`, with no dependency outside the standard library:

- a parser for the subset into a typed model, with every other construct refused and a
  reason given;
- a hand-written layered layout;
- an SVG writer with a fixed set of elements and attributes;
- the bounds and the render deadline;
- two fuzzers;
- one named test case for each published mermaid advisory, each shown to be inert.

It was not wired into anything; that was #171.

**Flowcharts in the reading view**
([#171](https://github.com/davison/md-notes/issues/171),
[PR #175](https://github.com/davison/md-notes/pull/175), approved at `c97b581`, on `main` as
[`6e735d5`](https://github.com/davison/md-notes/commit/6e735d5)):

- The note JSON gains a `diagrams` list, which comes from the markdown AST and never from the
  HTML.
- The reading view inserts an `<img>` in front of each listed block's code block.
- A new route, `GET /api/r/{slug}/diagram/{path...}?h=&theme=`, re-reads the note and finds
  the block by its hash.
- Drawings and refusals are cached, and draws take bounded slots.
- Every answer at a diagram URL carries the sandboxing CSP and `nosniff`.
- The palettes follow the settings, a live update reuses an unchanged image, and a failure
  falls back to the code block.

**Search hits land below diagrams**
([#177](https://github.com/davison/md-notes/issues/177),
[PR #181](https://github.com/davison/md-notes/pull/181), approved at `292dd95`, on `main` as
[`c5ab85a`](https://github.com/davison/md-notes/commit/c5ab85a)). This is the fix task for
QA's M9-R1 finding:

- The reading view asks for the note with `?sizes=1`.
- The daemon measures each diagram's natural size within a 150 ms budget and keeps the sizes
  by source hash.
- The page reserves each image's box before it loads.
- For a diagram the budget did not reach, the page holds the target in view as the image
  loads, until the reader scrolls, clicks or types.
- A block the layout refuses is taken out of the list, so it is code from the start.

**Documentation and record** ([#172](https://github.com/davison/md-notes/issues/172),
[PR #176](https://github.com/davison/md-notes/pull/176)): a Flowcharts section in the
introduction, the milestone in the README, `docs/e-ink.md` and `docs/sync.md`, the roadmap
row, and this record.

### What it cost the binary

M9-R4 asks for the size change to be measured and recorded. QA measured the whole milestone
against [`b287bbe`](https://github.com/davison/md-notes/commit/b287bbe), the commit it was
opened on, with the same flags, at `6e735d5`
([#169](https://github.com/davison/md-notes/issues/169#issuecomment-5767706977)):

| | `b287bbe` | `6e735d5` | delta |
|---|---|---|---|
| `make build`, amd64, UI embedded | 17,678,496 B | 17,911,968 B | **+233,472 B (+1.3%)** |
| arm64, UI embedded | 16,711,840 B | 16,908,448 B | **+196,608 B (+1.2%)** |
| Go only (empty `ui/dist`), amd64 | 14,700,704 B | 14,930,080 B | +229,376 B |
| Go only, arm64 | 13,697,184 B | 13,959,328 B | +262,144 B |

`internal/diagram` alone accounts for less than that. The layout decision measured it at
+176,128 B on amd64 and +131,072 B on arm64
([#170](https://github.com/davison/md-notes/issues/170#issuecomment-5766105515)). After the
round-one fixes, PR #173 re-measured it at +192,512 B and +131,072 B
([PR #173](https://github.com/davison/md-notes/pull/173#issuecomment-5766660958)). The rest
is #171's server and UI wiring. `6e735d5` predates #177, whose size reading and measuring
code is not in these figures; nobody measured the binary again after it. QA confirmed the
binary is statically linked, with `CGO_ENABLED=0` in its build information on both
architectures, and that `go.mod`, `go.sum`, both `package.json` files and both lockfiles are
byte-identical to `b287bbe`. The milestone brought in no third-party licence, so no packaging
file changed.

## Requirement outcomes

The verdicts are from the independent QA comments on the milestone issue. The first graded
`6e735d5` ([#169](https://github.com/davison/md-notes/issues/169#issuecomment-5767706977));
the second re-graded M9-R1 on `c5ab85a`
([#169](https://github.com/davison/md-notes/issues/169#issuecomment-5768247525)). QA ran the
built binary as scratch daemons, driven by headless Chromium and curl. The floor under both
verdicts: `make check` passed each time, and `make e2e` passed 71 of 71 tests on `6e735d5`
and 83 of 83 on `c5ab85a`.

| ID | Requirement | What the work established | QA |
|----|-------------|---------------------------|----|
| M9-R1 | A `flowchart` or `graph` block renders as a diagram in the reading view, for a documented subset: every direction, the common shapes, solid, dotted and thick edges with and without arrows and labels, chains and `&`, `subgraph`, comments. Anything else shows as code | 14 shapes, the link forms the plan lists (solid, dotted, thick and invisible, with arrow, circle and cross ends, both-ends forms and longer links), both label forms, four subgraph header forms, nesting, and links to a subgraph's border. A subgraph's own `direction` is accepted and not honoured, which is what mermaid does whenever a link crosses the border ([#170](https://github.com/davison/md-notes/issues/170#issuecomment-5766109427)). After the first verdict, each diagram's box is reserved before its image loads ([#177](https://github.com/davison/md-notes/issues/177#issuecomment-5768086074)) | **[Not satisfied](https://github.com/davison/md-notes/issues/169#issuecomment-5767706977)** on `6e735d5`: every form drew, but a search hit inside the 9th diagram of a note landed at 2802 px in a 900 px viewport, where the pre-M9 build centres it at 490 px ([#171](https://github.com/davison/md-notes/issues/171#issuecomment-5767691648)). **[Satisfied](https://github.com/davison/md-notes/issues/169#issuecomment-5768247525)** on `c5ab85a`: every hit probed stays centred for 6 s, cold notes included, at 1280×900 and 390×800; a reader's wheel or PageUp during loading is not fought; and the subset and fallbacks re-confirmed on the new binary |
| M9-R2 | Drawn by the daemon from a typed model into an SVG of its own elements, with source text escaped, shown through `<img>`; styling and markup directives not honoured; the parser fuzzed; size, counts and time bounded | Parsed into enums, indexes and label lines; the writer emits only `svg rect path polygon ellipse circle line text tspan` and writes no id, class, style, href, `url()`, marker or defs. Styling and `click` are skipped whole, and `%%{…}%%`, HTML and markdown labels are refused ([#170](https://github.com/davison/md-notes/issues/170#issuecomment-5766500829)). Delivered with `default-src 'none'; sandbox` and `nosniff` ([#171](https://github.com/davison/md-notes/issues/171#issuecomment-5767039874)) | [Satisfied](https://github.com/davison/md-notes/issues/169#issuecomment-5767706977): the advisory payloads were run through the running app, not just the package. Of 23 hostile blocks, 13 stayed code and 10 drew. All 30 drawings (10 blocks × 3 palettes) held only the elements `svg path polygon rect text tspan`, with payload text only as escaped character data, and no dialog and no off-origin request in Chromium. Opened directly, they had no active elements. Headers were on every status tried except one encoded dot-segment URL (observation 2, captured as [#179](https://github.com/davison/md-notes/issues/179)). Raw-HTML forgeries were not adopted. A 120-draw burst held the six slots. The fuzzers ran clean for 90 s and 60 s |
| M9-R3 | Legible in the light, dark and e-ink themes and following the setting; updated when the note changes on disk; falling back to code when a drawing cannot be fetched | Light and dark come from the page's own colours, and e-ink is black on white with 2 px lines. A test holds text to 4.5:1 and lines to 3:1. The light override gives the e-ink palette, accepted by the operator ([#171](https://github.com/davison/md-notes/issues/171#issuecomment-5767528754)). An unchanged diagram keeps its element across a live update. A failed image leaves the code block and is not asked for again on that page ([#171](https://github.com/davison/md-notes/issues/171#issuecomment-5767530708)) | [Satisfied](https://github.com/davison/md-notes/issues/169#issuecomment-5767706977): every drawing checked by eye in all three palettes, and a scheme change and the override each rewrote `src` on the same elements with no reload. A disk edit re-requested only the changed diagram. An aborted fetch, going offline, a killed daemon and a 422 each left code, and a 5 ms sampler never saw a broken image. Re-confirmed on `c5ab85a` with a 10 ms sampler ([#169](https://github.com/davison/md-notes/issues/169#issuecomment-5768247525)) |
| M9-R4 | One static binary with no cgo on amd64 and arm64; the size change measured and recorded; any third-party licence carried by the `.deb`, the AUR package and the README | No module added and no cgo. The size is under [What it cost the binary](#what-it-cost-the-binary). No licence came in; the Noto Sans advance widths are numbers only, with no glyph data ([#170](https://github.com/davison/md-notes/issues/170#issuecomment-5766109427)) | [Satisfied](https://github.com/davison/md-notes/issues/169#issuecomment-5767706977): `file` and `ldd` report a static binary, and `go version -m` reports `CGO_ENABLED=0` on both architectures. QA's Go-only baselines match #170's recorded ones byte for byte. `go list -deps ./internal/diagram` shows only the standard library |
| M9-R5 | The README and docs describe the supported subset and what falls back; the roadmap row; this record | This task; the pull request carrying this record is what delivers it. Stage one was reviewed and approved ([PR #176](https://github.com/davison/md-notes/pull/176#issuecomment-5767712105)) | [Untestable at the verdict](https://github.com/davison/md-notes/issues/169#issuecomment-5767706977): QA graded `main` before this task had merged, and nothing of it existed there. The re-grade repeated that ([#169](https://github.com/davison/md-notes/issues/169#issuecomment-5768247525)). A superseding verdict is owed once it merges |

## The human gates

None were raised. `gh codecrew status` reports "gates raised: none" for the milestone, and
no task plan listed a blocking ask-the-human point.

The operator still made four of the milestone's decisions, each recorded by the coordinator
when the operator gave it:

- the renderer is the project's own, drawn on the server, instead of mermaid in the page;
- flowcharts are the only diagram type in scope;
- the advisory vectors are proven inert by test;
- the light override gives the e-ink palette.

They are set out below with the others.

## Decisions

### A renderer of our own, on the server, shown as an image

This is the operator's decision, recorded by the coordinator at opening
([#169](https://github.com/davison/md-notes/issues/169#issuecomment-5765785737)). No diagram
source runs, styles or inserts anything in the browser. The daemon parses it, lays it out
and writes the SVG, and the page shows the result through `<img>`, which runs no script and
loads nothing.

**Rejected:** mermaid-js in the page. It is exposed through its advisories, and on this
origin a script has every note. **Rejected:** mermaid-js inside a sandboxed frame with an
opaque origin. It would be safe, and the operator judged it not worth the machinery.

### Flowcharts only

Also the operator's, in the same decision. Flowcharts are the only mermaid type in the
operator's notes. Every other type stays the code block it was, and becomes a milestone of
its own when someone needs it.

**Trade-off:** a sequence or class diagram in a note is still read as its source.

### The advisory vectors are tests, and each is shown to bite

The operator asked for this, and the coordinator made it part of #170's acceptance
([#170](https://github.com/davison/md-notes/issues/170#issuecomment-5765984674)):

- There is one named case for each mermaid advisory, with each payload adapted to flowchart
  syntax.
- Each case asserts either a refusal, with its reason, or an SVG that parses as XML, holds
  only allowlisted elements and attributes, and carries the payload only as escaped
  character data.
- The critical rows had to be shown failing against a deliberately unescaped writer, because
  "a test that would pass against a vulnerable writer proves nothing".
- CVE-2024-45801, the DOMPurify that mermaid bundles, has no counterpart, because no HTML
  sanitiser is in the SVG path. The test file says so rather than leaving the row silently
  missing.
- The delivery half, the route's headers, was assigned to #171.

### The layout is hand-written, not Graphviz in WebAssembly and not autog

The implementer prototyped all three on the same graphs
([#170](https://github.com/davison/md-notes/issues/170#issuecomment-5766105515)):

| | Graphviz wasm | nulab/autog | hand-written |
|---|---|---|---|
| binary delta, amd64 / arm64 | +6,537,216 / +6,291,456 B | +294,912 / +327,680 B | +176,128 / +131,072 B |
| start-up | ~450 ms with an empty `$TMPDIR`, and a 5.9 MB cache written from package `init` by every `mdn` subcommand | — | — |
| random 500 nodes / 1000 edges | 49.8 s | — | 1.75 s |
| stops at a deadline | no | no | yes, in every loop |
| subgraphs | yes | no | yes |
| licence | MIT + EPL-1.0 | MIT | ours |

**Trade-off:** the project owns about 1,100 lines of layout code and its quality. Graphviz's
`dot` still draws hard graphs better.

**Rejected:**
- *Graphviz wasm*, for the reasons in the table: a layout that cannot be interrupted, and an
  EPL-1.0 component to carry in both packages.
- *autog*. Its crashes on ordinary input include `fatal error: stack overflow`, which no
  `recover` can catch, so one hostile note could kill the daemon.

### Styling and `click` are skipped, not refused

`style`, `classDef`, `class`, `:::`, `linkStyle` and `click` are recognised by keyword and
skipped whole, and nothing of them enters the model
([#170](https://github.com/davison/md-notes/issues/170#issuecomment-5766109427)). Every other
construct outside the subset is refused, `%%{init}%%` included, because it can change the
layout and not just the colour.

**Trade-off:** a flowchart that used colour to mean something draws without that meaning.

**Rejected:** refusing them. That would send every styled flowchart back to a code block, and
guard against nothing that skipping does not already guard against.

The coordinator's brief for #170 had called refusal "the safer default". The coordinator
then recorded that skipping meets M9-R2's "not honoured"
([#170](https://github.com/davison/md-notes/issues/170#issuecomment-5766500829)). The
reviews then closed the ways a skipped statement could swallow what followed it. What
remains is the false refusals set out under
[Deviations and narrowings](#deviations-and-narrowings).

### The bounds

The limits are input 32 KiB, 200 nodes, 400 edges after `&` expansion, 50 subgraphs,
nesting depth 8, link length 8, 500 characters and 20 lines per label, 3,000 layout nodes,
and a 2 s render deadline
([#170](https://github.com/davison/md-notes/issues/170#issuecomment-5766109839)):

- The real flowcharts in the corpus render in 0.1 to 0.9 ms.
- The layout-node bound is the one that catches pathological long-link graphs, in under
  1 ms and before any layout work.
- The 2 s deadline leaves room for a host four to five times slower than the one measured.

**Trade-off:** a legitimate diagram over 200 nodes, or with links longer than `--------->`,
shows as code.

One timing in this decision was wrong and was corrected; see
[Corrections](#corrections-to-the-record-itself).

### The image is made outside the render pipeline

The daemon sends `diagrams: [{line, hash}]` beside the sanitised HTML. It takes the list
from the markdown AST, and only from fenced `mermaid` blocks that `diagram.Parse` accepts.
The reading view inserts the `<img>` in front of the matching line anchor, which `scrubRaw`
already makes impossible for a note to forge. The sanitiser and the note's HTML do not
change, and the `<pre>` stays behind the image as the fallback
([#171](https://github.com/davison/md-notes/issues/171#issuecomment-5766982824)).

**Trade-off:** the swap needs JavaScript. The reading view renders nothing without it anyway.

**Rejected:**
- An `<img>` or `data-diagram` emitted by goldmark and admitted narrowly by bluemonday. That
  needs a matching `scrubRaw` rule, which is the class of bug #144 was.
- The server inserting markup after sanitising. No pattern can tell the renderer's markup
  from the note's, as #31 showed.

### The light override gives the e-ink palette

The mapping ([#171](https://github.com/davison/md-notes/issues/171#issuecomment-5766983049)):

- Light override on → `eink`.
- Otherwise `prefers-color-scheme: dark` → `dark`.
- Otherwise `light`.

The app has no separate e-ink switch, and the override exists for the e-ink panel.

**Trade-off:** a desktop reader using the override gets heavier black lines.

**Rejected:**
- The override mapping to `light`, which gives the panel the greys it draws worst.
- Detecting e-ink with `(monochrome)` or `(update: slow)`, which the Boox browser is not
  known to report.
- `<picture>` sources.

The operator accepted the mapping: "it's acceptable, no separate e-ink theme needed"
([#171](https://github.com/davison/md-notes/issues/171#issuecomment-5767528754)).

### The route's delivery and caching

The route decision
([#171](https://github.com/davison/md-notes/issues/171#issuecomment-5767039874)):

- **Look-up.** Each request re-reads the note and looks the hash up among its blocks. The
  hash is a key among blocks the caller can already read, never a claim about a source. An
  unknown hash is `404`.
- **Headers.** Every answer carries `nosniff` and `default-src 'none'; sandbox`.
  `style-src 'unsafe-inline'` is left out, because the writer uses presentation attributes
  only.
- **Browser caching.** `Cache-Control: no-cache` with a strong ETag. `immutable` was rejected:
  the URL names the source, not the drawing, so an upgraded daemon's better drawing would stay
  hidden.
- **Server cache.** Drawings and refusals share an LRU of 8 MiB and 512 entries, so a hostile
  block costs its deadline once.
- **Concurrency.** At most `max(1, GOMAXPROCS/2)` draws run at once.

**Trade-off, as the decision states it:** a deadline refusal caused by a momentarily busy
machine stays refused until the entry is evicted or the daemon restarts. #177 lengthened
that; see below.

### Sizes are measured on request, within a budget, and kept by source

The first #177 decision drew every diagram when the note rendered, so that each box could be
reserved, and added no re-anchoring code
([#177](https://github.com/davison/md-notes/issues/177#issuecomment-5767799265)). The review
of [PR #181](https://github.com/davison/md-notes/pull/181#issuecomment-5767909888) measured
what that cost, and the decision was revised
([#177](https://github.com/davison/md-notes/issues/177#issuecomment-5768086074)):

- Only the reading view asks for sizes, with `?sizes=1`.
- Sizes and refusals are kept by source hash in their own LRU of 32,768 entries, and every
  draw fills it.
- The daemon measures for at most 150 ms, on at most two draw slots per request, under the
  request's context, and the page aborts its fetch when the reader leaves.
- For a diagram the budget did not reach, `holdInView` re-centres the target each time an
  image above it loads or fails, and lets go when the reader scrolls, clicks or types.

The revised decision measured the cost: a hit inside the 40th of 60 mostly unmeasured
diagrams lands at 491 px of a 900 px viewport with the hold, and at 9323 px without it.

**Trade-off:** on a first open of a note with many unfamiliar, slow diagrams, most of them go
out unmeasured and are placed in stages.

**Rejected:**
- A fixed count of blocks instead of a time budget, because a count bounds nothing when each
  block can take 2 s.
- Measuring in the background after the response, which is work nobody asked for and which
  outlives the reader.

### What the approvals left, and why

- **After PR #173**
  ([#170](https://github.com/davison/md-notes/issues/170#issuecomment-5766902241)):
  - Entity codes that decode to invisible characters are not dropped before the checks.
    Declined: a decoded entity is always text, and the output is escaped either way.
  - The false refusals of `var(--x)` and of single-quoted styling containing `--` or `-->`
    were handed to the docs.
  - So was the black flag that the England, Scotland and Wales emoji draw as.
- **After PR #175**
  ([#171](https://github.com/davison/md-notes/issues/171#issuecomment-5767530708)):
  - N4, a read that hangs on a network or FUSE mount holding a draw slot, is not changed.
  - N5, a failed diagram staying as code until the note is reopened, is not changed. The
    image's `error` event cannot tell a refusal from a network failure.
  - Both are stated in the docs.
- **After PR #181**
  ([#177](https://github.com/davison/md-notes/issues/177#issuecomment-5768152245)):
  - N1 accepted: a block slower than the budget is started again on each open, at about
    0.3 CPU-s per open, until the image route sizes it.
  - N2 captured as #182 and stated in the docs: a deadline refusal now lasts until restart.
  - N3 accepted: the 422 browser case depends on the machine's speed, and can only fail
    loudly.

## Deviations and narrowings

**Skipping styling departs from the brief's stated default.** It is recorded as a
disposition on #170 rather than as a gate, because neither the colours nor the link targets
reach the output. See the decision above.

**A statement that is skipped but holds a link is refused, and that refuses some real
styling.** The round-two review asked for the fix, because a skipped `style A --> B` was
dropping the rest of the diagram
([PR #173](https://github.com/davison/md-notes/pull/173#issuecomment-5766755158)). Round
three listed what the rule catches that it should not
([PR #173](https://github.com/davison/md-notes/pull/173#issuecomment-5766897000)):

- `style A fill:var(--accent)`;
- `classDef x font-family:'a--b'`;
- `click A 'https://x/a-->b'`, because only double quotes count as quotes.

Each falls back to code. Mermaid's grammar gives none of them a standard form, and the docs
say so.

**Subdivision flags draw as a black flag.** Stripping the tag characters U+E0000 to U+E007F
closes an invisible-character channel. The same characters spell the England, Scotland and
Wales flags, so those draw as 🏴. The reviewer judged this acceptable, and so did the
coordinator
([#170](https://github.com/davison/md-notes/issues/170#issuecomment-5766902241)).

**M9-R3's "falls back to the code block" is narrowed to a page.** A drawing that fails is not
asked for again on that page. It stays code until its source or the palette changes, or the
note is opened again, and that covers a transient network failure too
([#171](https://github.com/davison/md-notes/issues/171#issuecomment-5767530708)). This
removed a flicker that happened on every live update (N3 on
[PR #175](https://github.com/davison/md-notes/pull/175#issuecomment-5767392235)).

**Diagrams are not cached for offline use.** They are under `/api/`, which the service worker
never answers from its cache, and neither is the note they belong to. #171's plan recorded
this, and the docs state it.

**#177 added the re-anchoring its plan said it would add only "if the measurement shows it is
needed".** Reserved boxes alone were enough for measured diagrams. Once a budget left some
diagrams unmeasured, re-anchoring became the fix for those, and the revised decision records
it.

## What the reviews changed

Every implementation pull request was reviewed by a clean-context session under the reviewer
contract, with the reviewer seat routed to the operator. Each pull request merged under the
operator's standing confirmation, posted as an operator-confirmation comment on it. Each
blocking finding came with a test that failed before its fix.

**[PR #173](https://github.com/davison/md-notes/pull/173), three rounds.**
- **Round one** ([changes requested](https://github.com/davison/md-notes/pull/173#issuecomment-5766494612))
  found two half-drawn diagrams:
  - B1: a hex colour's `;` was read as the end of an entity code, so `style A fill:#fff;
    B-->C` swallowed `B-->C`;
  - B2: any statement whose first word began with `acc` was refused, `account --> B`
    included.

  The fix pass
  ([PR #173](https://github.com/davison/md-notes/pull/173#issuecomment-5766660958)) also:
  - refused a diagram with nothing to draw;
  - held the SVG validator's numbers to decimals;
  - stripped bidi and zero-width characters;
  - spread arrowheads that met at one point, and nested repeated self-loops;
  - strengthened the fuzzer's drawing check. It then found a real bug: in LR, a self-loop's
    label wider than its node ran off the image.
- **Round two** ([changes requested](https://github.com/davison/md-notes/pull/173#issuecomment-5766755158))
  found that `TestLayoutChecksContext` still passed with two of the layout's three context
  checks removed. The fix made each phase visible to the test, and the reply showed each
  removal failing. The same round moved stripping ahead of the checks, and added the rule
  that a skipped statement must not hold a link.
- **Round three** [approved](https://github.com/davison/md-notes/pull/173#issuecomment-5766897000).

**[PR #175](https://github.com/davison/md-notes/pull/175), three rounds.**
- **Round one** ([changes requested](https://github.com/davison/md-notes/pull/175#issuecomment-5767314701)),
  B1: every diagram request re-read and re-parsed the whole note outside the draw slots.
  - The review measured 200 requests for one already-cached diagram, 50 at a time, at
    23.8 s of CPU, about 11 of 12 cores and 495 MB, on a 100-diagram, 2.3 MB note.
  - The fix caches each note's list by path, size and modification time, and puts building
    a list in a draw slot. Re-measured on the implementer's note, 200 such requests fell
    from 23.5 s of CPU and 509 MB to under 0.1 s and 56 MB
    ([PR #175](https://github.com/davison/md-notes/pull/175#issuecomment-5767392235)).
  - The same pass put the headers on answers written before the handler, and made the page
    remember failed URLs.
- **Round two** ([changes requested](https://github.com/davison/md-notes/pull/175#issuecomment-5767454492)),
  B2: a panic while building a list kept its draw slot for good. After as many panics as
  there are slots, diagrams stop for every note until restart. The fix releases the slot in
  a `defer` and recovers the panic.
- **Round three** [approved](https://github.com/davison/md-notes/pull/175#issuecomment-5767526941).

**[PR #181](https://github.com/davison/md-notes/pull/181), two rounds.**
- **Round one** ([changes requested](https://github.com/davison/md-notes/pull/181#issuecomment-5767909888)),
  B1: the note endpoint laid out every diagram on every open. A note of 600 slow diagrams
  took 9.2 s and 45 s of CPU per open even when warm, because its drawings evicted each
  other from the 512-entry cache. Three concurrent opens took 23.6 s and 142 s of CPU.
  Another note's one small diagram waited 4.76 s behind them.
- **Round two** [approved](https://github.com/davison/md-notes/pull/181#issuecomment-5768146985)
  the budgeted design, and re-ran the reviewer's own table. The same note took 2.69 s cold
  with 3.0 s of CPU, and three concurrent opens took 2.73 s and 8.8 s of CPU, against the
  pre-#177 base's 2.7 s and 8 s.

**[PR #176](https://github.com/davison/md-notes/pull/176)**, this task. Stage one was
[approved](https://github.com/davison/md-notes/pull/176#issuecomment-5767712105) with one nit,
a line wrapped long, which is taken. Stage two, this record, is reviewed on the same pull
request.

## Corrections to the record itself

**The bounds decision's worst case.** The decision called a dense 200-node, 400-edge graph
at 366 ms "the worst case inside those counts". That graph was timed with the layout-node
bound lifted, and under `DefaultLimits` it is refused before any layout work. The real worst
case inside all the bounds, from a search of 300 random graphs, is 211 ms. So the 2 s
deadline is about ten times that, not five
([#170](https://github.com/davison/md-notes/issues/170#issuecomment-5766322910)).

**The route decision's concurrency claim.** The decision said a note with many expensive
blocks "must queue rather than take every core". As first built, only the draw was bounded.
It was corrected on the task when round one of PR #175 found this
([#171](https://github.com/davison/md-notes/issues/171#issuecomment-5767394237)).

**The first #177 decision.** It was superseded by a revision, not edited, after round one of
PR #181 measured it
([#177](https://github.com/davison/md-notes/issues/177#issuecomment-5768086074)).

**"Every answer at a diagram URL carries both headers."** The round-one reply on PR #175
([PR #175](https://github.com/davison/md-notes/pull/175#issuecomment-5767392235)) gave this
as the precise statement for the docs. QA found one encoded dot-segment URL that gets the
generic 404 without them, on `6e735d5` and again on `c5ab85a`
([#169](https://github.com/davison/md-notes/issues/169#issuecomment-5767706977),
observation 2). It is captured as #179, and the docs name the exception.

**The e2e count.** Stage one of this task wrote 71 checks. #177 made it 83, and the count
was re-run on the rebased branch.

## Captures adopted, and captures raised

M9 adopted no captures. [#168](https://github.com/davison/md-notes/issues/168), the manual
page missing from the AUR package and from `make install`, predates the milestone. The scope
decision left it to the operator, and it is not part of M9.

Raised by this milestone, for a later task to adopt:

| Capture | From | What it is |
|---------|------|-----------|
| [#174](https://github.com/davison/md-notes/issues/174) | the round-one review of [PR #173](https://github.com/davison/md-notes/pull/173#issuecomment-5766494612) | An edge into a nested subgraph is drawn through the subgraph's title band. It is the one drawing-quality finding that the fix pass left alone, as not cheap |
| [#178](https://github.com/davison/md-notes/issues/178) | M9 QA, observation 1 | A note with 20,000 small language-tagged code blocks takes about 78 s to render, on the pre-M9 build as well. The addendum from #177's fix pass ([#178](https://github.com/davison/md-notes/issues/178#issuecomment-5768099588)): each block in a language chroma has no lexer for, `mermaid` included, costs 2.5 to 3.5 ms to render, against about 0.15 ms for `go` |
| [#179](https://github.com/davison/md-notes/issues/179) | M9 QA, observation 2 | An encoded dot-segment under a diagram URL gets the generic 404 without `nosniff` or the CSP. The body is fixed text, and QA found no way to exploit it |
| [#180](https://github.com/davison/md-notes/issues/180) | M9 QA, observation 3 | An e-ink diagram's white background sits in a faint box on the light page's `#fbfbfa`. Cosmetic |
| [#182](https://github.com/davison/md-notes/issues/182) | N2 of the round-two review of [PR #181](https://github.com/davison/md-notes/pull/181#issuecomment-5768146985) | A diagram refused at the 2 s deadline on a merely busy host stays as code until the daemon restarts |

## Where the record is silent

**The operator's reasoning is recorded in the coordinator's words.** The vetting of mermaid,
the choice of a server-side renderer, flowcharts only, and the advisory tests are all written
up by the coordinator: "the operator judged that not worth it and asked instead". The
light-override acceptance is the one place with the operator's own words quoted. There is no
operator comment weighing the sandboxed-frame option against a renderer of our own. That it
was weighed is stated, and how is not.

**Whether the Noto Sans widths need an OFL notice.** The implementer believed not, because
they are numbers with no glyph data, and offered it as a one-line addition "if the operator
wants one"
([#170](https://github.com/davison/md-notes/issues/170#issuecomment-5766109427)). No answer is
recorded. No notice was added, so the question stands as the implementer left it.

**The exact URL #179 names.** QA's observation gives it as
`…/diagram/%2e%2e/outside/o.md`, with the start elided. Three spellings of that shape, sent
with `curl --path-as-is` to a scratch daemon built from this branch, reached the route and
were answered `403` with both headers, so the failing form differs in some way the record
does not show. Whoever takes #179 should ask QA for the full URL rather than reconstruct it.
