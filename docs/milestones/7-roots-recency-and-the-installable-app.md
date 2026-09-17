# M7 — No root registered by accident, a navigator sorted by recency, and an installable app

Tracking issue: [#114](https://github.com/davison/md-notes/issues/114). Its three
implementation tasks are merged on `main` at
[`1407305`](https://github.com/davison/md-notes/commit/1407305).

Independent QA ran against that commit and found M7-R1, M7-R2, M7-R3 and M7-R5
satisfied, M7-R4 untestable until this record merges
([#114](https://github.com/davison/md-notes/issues/114#issuecomment-5719043677)). Its
one finding — a browser check that M7-R5's own worker had made flaky — became a fix task
inside the milestone, [#124](https://github.com/davison/md-notes/issues/124), merged at
[`0c049f4`](https://github.com/davison/md-notes/commit/0c049f4); this record is what is
left.

## Goal and outcome

The milestone came from two asks the operator made on 2026-09-17, and grew a third the
same afternoon. The first was the M3 QA capture
[#50](https://github.com/davison/md-notes/issues/50): opening a `file:` URL for a
markdown file that does not exist must not register its directory as a permanent root,
and a root registered by accident must be removable from the app rather than by editing
the state file. The second was a navigator sort toggle — alphanumeric as it always was,
or last-modified with the most recent at the top, remembered per browser. The third
arrived after the milestone was open, with neither implementation task yet at a pull
request: a web app manifest and a service worker, so the UI installs as a home-screen
app on Android
([#114](https://github.com/davison/md-notes/issues/114#issuecomment-5717938933)). The
inbox clipper — the last piece of the original plan — stayed later work, as it has since
M3.

### What #50 actually was

The operator asked whether #50 was serious from a security point of view. The
coordinator's answer, recorded before any task started, is the frame the whole milestone
is built on
([#114](https://github.com/davison/md-notes/issues/114#issuecomment-5717883455)):

> Real but bounded. The trigger is the user's own navigation to a `file:` URL (a web
> page cannot navigate to one), registration is loopback-only, and on a single-user
> machine every local process already reads and writes the same files. What the defect
> adds is persistence (a root nobody meant to register stays in the state file with no
> way to remove it from the app) and reach: a root registered by accident becomes
> readable and editable, markdown files only, by a token holder over the tailnet. That
> is a footgun with a security consequence rather than an exploitable hole, and the fix
> is the capture's own shape: verify before registering, and an unregister path.

That assessment set the scope: the fix is two halves, both of them in the capture, and
neither of them a change to who may reach what. It also set what the milestone could not
claim afterwards — nothing here narrows the tailnet credential; it stops the *set of
roots* growing by accident and gives the reader a way to shrink it.

### What shipped, in the order it merged

**The navigator has two orders.** `tree.Stat` pairs each listed path with its
modification time and `tree.Build` puts it on the file node as `modified`, Unix
milliseconds, `omitempty`; directories carry none. In the UI a toggle in the navigator's
own header — visible label **Recent first**, `aria-pressed` for the order in force —
switches the tree between the alphanumeric order the daemon has always produced and
last-modified with the newest note at the top: notes by their time within a folder,
folders by the newest note anywhere beneath them, folders still grouped before notes,
every tie broken by the same case-insensitive comparison the daemon sorts by. The order
is applied *after* the tag filter, so a filtered tree is ordered by the notes it is
showing; the control lives inside `Navigator`, so the wide pane and the drawer's
**Notes** tab get it from one place; the choice is one `localStorage` key for the
browser; and no sorting library entered the bundle
([#119](https://github.com/davison/md-notes/pull/119)).

**A registration can name the note it is for, and a recent root can be removed.**
`POST /api/roots` takes an optional `file` beside the `path`; with it the daemon
registers only once that path resolves inside the folder and is a regular markdown file
that exists, and otherwise answers `404 {"code":"not_found"}` naming the file, with
nothing appended to the registry and nothing written to the state file. `mdn open` sends
no `file` and is untouched. `DELETE /api/roots/{slug}` takes a recent root out of the
registry and the state file, answers `204`, stops its watcher and ends its open event
streams, and refuses three things: the configured notes root (`403 notes_root`), an
unknown slug or a second delete (`404 not_found`), and any caller under `tailnet_host`
(`403 loopback_only`). The extension sends the file with its registration and names the
refusal — "No such note: the daemon cannot find …, so … was not registered as a root" —
on the badge and in the popup as its own failure kind rather than as a token or
connection failure. The home page grew a **Remove** control on each recent root behind a
confirmation naming the folder and saying that nothing leaves the disk, and a tab open on
a removed root lands on the home page
([#121](https://github.com/davison/md-notes/pull/121)).

**The app installs, and opens with no daemon behind it.** `ui/public/manifest.webmanifest`
declares the name, `display: standalone`, `start_url: /`, `scope: /`, the colours and
icons at 192 and 512 with a maskable 512 beside them, and `ui/index.html` links it with
`crossorigin="use-credentials"` — without which a browser over the tailnet fetches the
manifest with no cookie, is handed the login page under a `401`, and concludes there is
nothing to install. `ui/src/sw.ts`, built to `/sw.js` by a second Vite pass, never
intercepts anything under `/api/` or `/login` or anything that is not a same-origin GET;
serves `/assets/**` cache-first because those names carry their content hash; and asks
the network first for the shell, the manifest and the icons, so a rebuilt shell is picked
up on the next open and the cache answers only when the network cannot. Offline, a
navigation is answered with the cached shell and the app says the daemon is not
answering — at the roots page and at every `/r/{slug}/…` route, the second of which took
the review to get right. The daemon gained one content type
(`.webmanifest → application/manifest+json`) and nothing else; the tailnet guard, the
session cookie and the Host and Origin checks are untouched, and
`TestTailnetGuardsTheInstallableAppsFiles` says so
([#120](https://github.com/davison/md-notes/pull/120)).

**What all of it cost the reading page.** The eager reading-page bundle — the shell, its
entry chunk and its stylesheet, brotli, as the daemon sends them — grew by 1,030 bytes
across the milestone, about 4.8 per cent:

| | pre-M7 ([`e0849dc`](https://github.com/davison/md-notes/commit/e0849dc)) | merged `main` ([`1407305`](https://github.com/davison/md-notes/commit/1407305)) |
|---|---:|---:|
| entry chunk | 17,472 B | 18,145 B |
| stylesheet | 3,629 B | 3,714 B |
| shell (`index.html`) | 574 B | 846 B |
| **total** | **21,675 B** | **22,705 B** |

Neither pull request's table says this: #119's and #120's were each measured on a branch
that did not carry the other's commits, so #120's "+403 B" describes a tree that was
never shipped. QA measured the merged figures
([#118](https://github.com/davison/md-notes/issues/118#issuecomment-5719027735)) and the
coordinator's disposition directs this record to cite them
([#114](https://github.com/davison/md-notes/issues/114#issuecomment-5719060998)). They
are reproduced here rather than copied: both trees were exported with `git archive` into
scratch directories, built with the same `pnpm --dir ui install --frozen-lockfile` and
`pnpm --dir ui build`, and the `.br` siblings the build writes were measured with `stat`
— every figure above matches QA's to the byte. Fetched after the page has loaded rather
than with it, merged `main` also carries `sw.js` at 1,133 B brotli and the manifest at
682 B, which is under the build's 1,024 B compression floor and so is its own wire size.
The editor chunk is still lazy and still absent from both the shell and the worker's
precache list.

The system as it stands is described in [the introduction](../introduction.md);
[Roots](../introduction.md#roots), [The web UI](../introduction.md#the-web-ui),
[On a phone](../introduction.md#on-a-phone),
[Installing the app](../introduction.md#installing-the-app),
[Display settings](../introduction.md#display-settings),
[The HTTP API](../introduction.md#the-http-api) and
[What is reachable under that name](../introduction.md#what-is-reachable-under-that-name-and-what-is-not)
are the sections this milestone rewrote, and
[the extension page's account of opening a local file](../extension.md#opening-a-local-markdown-file)
is the one it corrected.

| Task | Requirements | Adopts | PR | Merged as |
|------|--------------|--------|----|-----------|
| [#115](https://github.com/davison/md-notes/issues/115) Roots: refuse registration for a missing file, and unregister a recent root | M7-R1, M7-R2 | [#50](https://github.com/davison/md-notes/issues/50) | [#121](https://github.com/davison/md-notes/pull/121) | [`f9dfbd3`](https://github.com/davison/md-notes/commit/f9dfbd3) |
| [#116](https://github.com/davison/md-notes/issues/116) Navigator: toggle between alphanumeric and last-modified order | M7-R3 | — | [#119](https://github.com/davison/md-notes/pull/119) | [`50d3197`](https://github.com/davison/md-notes/commit/50d3197) |
| [#118](https://github.com/davison/md-notes/issues/118) Installable app: web app manifest and service worker | M7-R5 | — | [#120](https://github.com/davison/md-notes/pull/120) | [`1407305`](https://github.com/davison/md-notes/commit/1407305) |
| [#117](https://github.com/davison/md-notes/issues/117) Document M7 and synthesize its record | M7-R4 | — | this one | — |
| [#124](https://github.com/davison/md-notes/issues/124) Fix task: `assets.test.mjs` is flaky since the service worker landed | M7-R5 | — | [#126](https://github.com/davison/md-notes/pull/126) | [`0c049f4`](https://github.com/davison/md-notes/commit/0c049f4) |

**The three implementation tasks ran in parallel and did not meet.** #115 is the roots
registry, the server's roots handlers, the extension's open-file flow and the home page;
#116 is `internal/tree`, the navigator and a new `ui/src/order.ts`; #118 is the manifest,
the worker, a second Vite pass and the asset-serving table. The one file more than one of
them touched is `ui/src/api.ts`, and the reviewer of #119 checked the hunks were disjoint
— the `TreeNode` interface on one branch, the `send`/`fetch` wrappers on the other —
before either merged, and confirmed that the worker returns `"network"` for every path
under `/api/`, so the whole-tree refetch the order toggle leans on can never be answered
from a cache
([#119](https://github.com/davison/md-notes/pull/119#issuecomment-5718387766)).

**Both tasks wrote their tests first and showed them failing.** #116's new cases were run
against the unchanged code and the failures quoted
([#119](https://github.com/davison/md-notes/pull/119)); #115 posted a whole comment of
them, of which `TestAddRootRefusesAMissingFile` is the one worth reading twice — nine
spellings of "the note is not there", every one a `200` and a permanent root on `main`,
"which is #50 in a table"
([#121](https://github.com/davison/md-notes/pull/121#issuecomment-5718253173)).

## Requirement outcomes

The verdicts are from the independent QA comment on the milestone issue
([#114](https://github.com/davison/md-notes/issues/114#issuecomment-5719043677)), run
against a clean worktree of merged `main` at
[`1407305`](https://github.com/davison/md-notes/commit/1407305) — never the operator's
checkout — with scratch daemons on ports 8841–8845, their own roots, token files and
state files, and a TLS stand-in on 8844 in front of a made-up tailnet name, so the
daemon on 7337 and `~/.local/state/mdn` were untouched. The floor, which QA is explicit
is the floor and not the evidence: `make check` clean (Go, 291 UI tests, 212 extension
tests, both builds and the zip), `go test -race -count=5 ./internal/...` clean, the
extension's unit and e2e suites 212/212 and 28/28 three times each, and `make e2e` nine
times — five runs 53 of 53 and four runs 52 of 53, every failure the same case, which is
the finding below.

| ID | Requirement | What the work established | QA |
|----|-------------|---------------------------|----|
| M7-R1 | No root registered for a missing file: the daemon verifies before registering, nothing is written to the state file on a refusal, the extension names the refusal on the badge and popup, and registration stays loopback-only | The daemon shape was taken (an optional `file` on `POST /api/roots`); handler and registry tests read the state file after a missing file, a non-markdown file, a file outside the folder lexically and absolutely, a symlink out, a directory and the happy path in four spellings; an extension e2e case drives a real `file:` URL for a note that is not there in Chromium and asserts the roots listing and the state file unchanged; the reviewer re-ran the confinement matrix by hand, including percent-encoded and NUL-bearing spellings ([#121](https://github.com/davison/md-notes/pull/121#issuecomment-5718407586)) | [Satisfied](https://github.com/davison/md-notes/issues/114#issuecomment-5719043677) — and the case neither suite enumerates: a note that has never existed, in a folder that **is** already a root. The tab lands on the note's route and the app says it is not found, while the roots listing and the state file are byte-for-byte unchanged, because the `file` guards registering and not serving. In an *un*registered folder the extension leaves the tab on the browser's error page, records the refusal by name, sets the `!` badge and mentions no token; the real note beside it then registers the same folder and opens |
| M7-R2 | Unregister a recent root: `DELETE /api/roots/{slug}` from registry and state file, never the notes root, refused over the tailnet, a remove control with a confirmation naming the path, and a removed root's tabs landing on the home page | All of it, with two narrowings recorded on the milestone issue ([#114](https://github.com/davison/md-notes/issues/114#issuecomment-5718608764)): the route home holds for every root the daemon can *watch*, and the single `404 not_found` is about the note rather than the folder. The reviewer measured the refusal set, the state file after each case, and that no file leaves the disk | [Satisfied](https://github.com/davison/md-notes/issues/114#issuecomment-5719043677) — QA read the allow-list diff as well as exercising it (five lines of comment inside the `rest == "roots"` case and no code), aimed five path spellings at the guard's `path.Clean` under the tailnet name, ran two *concurrent* deletes of one slug for exactly one `204` and one `404`, and drove the watcher-guard race from #121's review against a 6,481-directory folder — the slug's stream afterwards carried the second folder's changes and not the first's |
| M7-R3 | Navigator sort toggle: both orders, folders by their newest note, persistence per browser, the tag-filtered tree and both layouts, times from the daemon staying current through the events stream, the 40 px target, e-ink and no-motion, and no sorting library in the eager bundle | All of it, driven in Chromium at 1280 px and on a Pixel 7 profile by the implementer and again by the reviewer against a scratch root whose times disagree with the alphabetical order at three levels of nesting; the tag-filtered case comes back in the reverse of the unfiltered recency order, which is the premise the folder-rank decision rests on ([#119](https://github.com/davison/md-notes/pull/119#issuecomment-5718387766)) | [Satisfied](https://github.com/davison/md-notes/issues/114#issuecomment-5719043677) — with five probes past the suite, of which two are worth carrying: the no-mtime rule met for real rather than in a fixture (a 5,000-note root under a delete-and-recreate loop gave 107 of 200 tree responses carrying at least one node with no `modified`, none a non-200, and the navigator sorts such a node last even when it is the newest file on disk), and a junk value in `mdn:nav:order` leaving the control unpressed and the tree alphanumeric rather than empty. The eager chunk contains no occurrence of `lodash`, `fast-sort`, `natsort`, `collator` or `Intl.Collator` |
| M7-R4 | Documentation and record: the refusal, the remove control, the sort toggle and the installable app, the introduction's API table and the extension page, the roadmap row, and this record | This task; its pull request is what delivers it | [Untestable at the verdict](https://github.com/davison/md-notes/issues/114#issuecomment-5719043677) — QA graded it against `main` as it stood before this task, where none of it exists yet; a superseding verdict is due once this merges. Two things it asked this record to carry are carried: the API table's `400` for a folder and `404 not_found` for a note, per the narrowing, and merged-`main` bundle figures rather than either branch's |
| M7-R5 | Installable app: manifest, service worker, install on Android over HTTPS, offline shell, `/api/` never intercepted, no stale shell after a rebuild, content types and `no-cache`, the tailnet checks unchanged, the eager bundle not regressed | Chrome's installability criteria checked item by item in headless Chromium — over HTTP and through CDP `Page.getAppManifest`, whose `errors` array is asserted empty — because Lighthouse's PWA category, which held those audits, was removed in Lighthouse 12; `fromServiceWorker` false on every `/api/**` response with the assets in the same trace true as a control, and no `/api/` URL in any cache; a poisoned shell cache losing to the daemon's shell while the network is up and answering when it is down; a real rebuild picked up in one reload with the old cache deleted on activation; and the offline walk over four routes that the first review's blocking finding produced. The reviewer re-ran all of it on a TLS stand-in for `tailscale serve` as well ([#120](https://github.com/davison/md-notes/pull/120#issuecomment-5718739359)) | [Satisfied](https://github.com/davison/md-notes/issues/114#issuecomment-5719043677), with a finding against the suite rather than the behaviour. QA ran the install path in a browser over a TLS stand-in, which nothing in the suites does: anonymous, a navigation gets the login page under `401` with no manifest link and no worker; through the login form the manifest parses with `errors: []`, the worker activates at scope `/`, and all six of the manifest, the worker and the four icons answer `401` anonymous and `200` with a session. A save offline is rejected outright with nothing written and no queue; nothing under `/api/` and no login page is in any cache after a real session on either origin; and a real rebuild is picked up on the next load with the old cache gone. The finding is that `ui/e2e/assets.test.mjs` has become flaky, fixed in [#124](https://github.com/davison/md-notes/issues/124) at [`0c049f4`](https://github.com/davison/md-notes/commit/0c049f4); a superseding verdict on this requirement is the coordinator's to ask for |

**QA's one finding is against a check, not against the app.** `ui/e2e/assets.test.mjs`
fails on roughly four runs in nine of the whole suite on merged `main`, always the same
assertion: the worker answers a third `/assets/index-*.js` response, and under the
suite's own load the worker's install and claim slide early enough that the extra
response lands on the *second* page load, where the case compares the **list** of asset
paths rather than only the byte total. The property the case exists to protect held in
every failing run — zero asset bytes on the wire, the extra response served from the
worker's cache rather than fetched — and the file passes 22 of 22 runs on its own, even
under eight spinning CPU hogs; it needs the rest of the suite beside it
([#118](https://github.com/davison/md-notes/issues/118#issuecomment-5719027735)). The
coordinator's disposition made it a fix task *inside* M7 rather than a capture, because
the milestone's own gates make the e2e job a merge gate for everything that follows and
a flaky gate would read as the next task's failure
([#114](https://github.com/davison/md-notes/issues/114#issuecomment-5719060998)).

**What the flake was.** [#124](https://github.com/davison/md-notes/issues/124) went back
to CDP before changing anything, and the extra row is neither the worker's precache fetch
nor an inner/outer pair: it is a **second request for the entry module from the same
`<script>` tag** — the preload scanner's and the module loader's own, reported at columns
70 and 79 of one source line, both `parser`-initiated, both answered by the worker out of
the disk cache for zero bytes. The browser's own cache folds those two; the worker does
not, which is why no duplicate appears until a worker is controlling the page. Where it
lands is load, not luck: at ×1 CPU throttling the duplicate arrives on the *third* load,
where the case asserted only a byte total; at ×4 and above it arrives on the *second*,
where the case compared the list of paths. That is QA's "4 of 9 in the full suite, 0 of
22 alone" exactly — the full suite is eight other browsers' worth of contention
([#126](https://github.com/davison/md-notes/pull/126)).

So the check now states the property rather than the shape of the trace: the `deepEqual`
is over *distinct* URLs, every asset response on the second and third loads is asserted
at zero bytes rather than zero in total — strictly stronger, since a total of zero could
be one row fetched and one refunded — the shell is counted in bytes rather than in rows,
and each row records who answered. And the interleaving that used to arrive by luck now
arrives on every run: the case is parameterised over Chromium's CPU throttling, ×1 and
×8, so both shapes are covered on any machine. The old assertions under the same ×8
throttling fail with QA's message verbatim, three times in three; the new case ran
**thirty consecutive full-suite runs green**, and the reviewer's own twelve runs saw
both interleavings on every one of them
([#126](https://github.com/davison/md-notes/pull/126),
[review](https://github.com/davison/md-notes/pull/126#issuecomment-5719422345)). #124
also took QA's second note — the tailnet guard test covered the manifest and the worker
but not the four icons — and the review turned that leg into something stronger than the
measurement it replaced: each of the six files is now asked for with its own
`Sec-Fetch-Dest`, so the refusal an image decoder meets is asserted to be the JSON `401`
rather than the login page
([#126](https://github.com/davison/md-notes/pull/126#issuecomment-5719486897)).

The review approved with two wording findings and took both on the way past: the case
title said the throttling makes "the worker answer the second load", where what it
changes is whether Chromium reports the module request *twice* — the worker answers at
×1 as well — and the guard test's anonymous leg asked for icons with navigation headers
while its comment reasoned about a browser fetching them on a path of its own. The same
review left `docs/introduction.md`'s copy of the first sentence alone on the ground that
this task owns the page, and this task has taken the reviewer's wording there
([review](https://github.com/davison/md-notes/pull/126#issuecomment-5719422345),
[#126](https://github.com/davison/md-notes/pull/126#issuecomment-5719486897)).

**What QA did beyond the requirement text** is, as in M6, where most of the confidence
comes from: the shipped suites were already green, and QA went looking for the shapes
they do not cover. A note that never existed inside a folder that is already a root; five
path spellings aimed at the tailnet guard's `path.Clean`; two concurrent deletes of one
slug; the watcher-guard race driven against a 6,481-directory folder; a 5,000-note root
under a delete-and-recreate loop to reach the missing-mtime rule for real; a note dated
three days in the future; junk in the `localStorage` key; and the whole install path in a
browser over a TLS stand-in, which no suite exercises. None of it found a defect in the
three merged features.

## Decisions

### The milestone's scope, and #50's severity, were settled before any task started

Recorded by the coordinator on the operator's instruction, at the top of the milestone
([#114](https://github.com/davison/md-notes/issues/114#issuecomment-5717883455)): one
task for #50's two halves, one for the sort toggle, one for the record; the backlog
captures [#111](https://github.com/davison/md-notes/issues/111),
[#112](https://github.com/davison/md-notes/issues/112),
[#107](https://github.com/davison/md-notes/issues/107),
[#108](https://github.com/davison/md-notes/issues/108) and
[#110](https://github.com/davison/md-notes/issues/110) stay in the backlog, none of them
touching the same code. The decision also fixed the defaults the requirements state, for
an implementer to overrule on the record rather than in silence: the daemon verifies
rather than the extension rolling back, folders keep grouping before notes and rank by
their newest note, the choice persists in `localStorage`, and modification times ride on
the tree response.

- **Trade-off:** fixing defaults in requirement text decides design questions before the
  person who will implement them has looked at the code. It is the shape M5 and M6
  settled on because the operator is away between batches, and each default is stated as
  overrulable on the record.
- **Rejected:** a milestone of #50 alone (the sort toggle is small and independent), and
  folding #111 or #112 in (unrelated code; the operator did not ask).

### M7-R5 was added to an open milestone, by the coordinator, on the operator's words

The operator asked for the installable app while the milestone was open and before either
implementation task had a pull request. Rather than open an M8 or defer it to a capture,
the coordinator added a fifth requirement and widened M7-R4 and the QA gate to cover it
([#114](https://github.com/davison/md-notes/issues/114#issuecomment-5717938933)). The
work is UI and asset-serving, disjoint from #115 and mostly from #116, so it fits as a
third parallel task; the tailnet route already terminates TLS, which is the secure
context an install needs.

- **Trade-off:** editing a milestone's requirements after creation is something the
  implementer contract forbids to implementers. The coordinator did it as the operator's
  primary session, on the operator's explicit words, and recorded it — so QA and this
  record judge five requirements and know the fifth arrived late.
- **Rejected:** a separate M8 (small, and the operator wanted it with this batch);
  deferring it to a capture.

### The daemon verifies, rather than the extension registering and rolling back

M7-R1 offered both shapes and left the choice to the task, which took the requirement's
own default and said why
([#115](https://github.com/davison/md-notes/issues/115#issuecomment-5718216788)). A
rollback has a window in which the root *is* registered and *is* in the state file; an
extension service worker killed in that window — which a browser may do whenever it likes
— or a daemon that goes away between the two calls leaves behind exactly the root the
check exists to prevent, which is #50. Verification before the first write has no such
window, and it is one request instead of three. The `file` is confined through the same
funnel every request path goes through — cleaned lexically, symlinks evaluated, the real
path compared against the root's real path — and whether the name is a *note* is decided
in the handler with `tree.IsMarkdown`, beside every other place that asks that question.

- **Trade-off:** `POST /api/roots` grows a second job. It is kept to one field, absent
  by default, so `mdn open`'s request is byte-for-byte what it was.
- **Rejected:** the extension registering and checking through the new slug, for the
  window above.

### One refusal for every way of failing — about the note

Missing, not markdown, not a regular file, outside the folder lexically or through a
symlink: all `404 {"code":"not_found"}` naming the file the caller sent. Telling them
apart would say more about what is outside the folder than about the folder, and the
caller is on loopback holding that path already
([#115](https://github.com/davison/md-notes/issues/115#issuecomment-5718216788)).

The review measured that the decision as first written was broader than what ships: a
registration naming a *folder* that is not there still answers
`400 {"error":"lstat …: no such file or directory"}`, the branch `POST /api/roots` has
always had. That was kept deliberately and the decision narrowed to match
([#115](https://github.com/davison/md-notes/issues/115#issuecomment-5718465965)): the
folder's `400` is `mdn open`'s diagnostic, printed to somebody standing at a terminal who
has just typed the path, where the OS's own sentence is the most useful thing there is;
the note's uniform `404` is what a program branches on. Both endpoints are loopback-only,
where anything that can reach the port can already `stat` the filesystem.

- **Trade-off:** two refusals for one request body, and a reader has to know which half
  of it they got wrong. The introduction's API table now says both.
- **Rejected:** distinct codes per failure mode (an existence oracle for paths inside a
  folder); and making the folder answer `404` too (it would take `mdn open`'s best
  diagnostic away for symmetry nobody needs).

### A root whose file is later deleted stays registered

The `file` is a condition on *registering*, not a lease on the root
([#115](https://github.com/davison/md-notes/issues/115#issuecomment-5718216788)). Once
the folder is a root, the note it was registered for can be deleted, renamed or emptied
and the folder goes on being served until somebody unregisters it — which M7-R2 is now
the way to do. Anything else would mean the daemon watching a file to decide whether to
keep serving a folder, dropping a root out from under an open tab for a reason the reader
never sees. The one existing rule that does drop a root is unchanged: a folder that has
*gone* when the daemon starts is dropped from the state file then.

- **Trade-off:** a root can outlive the reason it exists. The remove control is the
  answer, and it is one click.
- **Rejected:** treating the `file` as a lease.

### `403 notes_root` for deleting the configured notes root

403 rather than 404, because the root is there and the caller may see it — the honest
answer is "not this one", not "no such thing"; and rather than 409, because nothing about
the daemon's state would make the request succeed later. The code travels in the
`{code, error}` envelope the source and clip endpoints already use, so the home page and
the extension branch on a code rather than on a sentence; an unknown slug, and removing
the same root twice, are `404 not_found` in the same envelope
([#115](https://github.com/davison/md-notes/issues/115#issuecomment-5718216788)).

- **Trade-off:** `POST /api/roots` keeps its older `{error}`-only refusals (bad JSON,
  missing path, relative path, not a directory), so one endpoint answers in two shapes.
  Left alone rather than changed under a task that was not about them.
- **Rejected:** `404` (dishonest), `409` (implies a retry that cannot help).

### The route home for a removed root rides the event stream

The daemon ends the root's streams when it unregisters it (`watch.Hub` gained a `Close`),
so a stream does not sit on keepalives for a root that is no longer served; the browser
reconnects of its own accord and meets the `404` the unknown slug now gives. That closes
the stream in exactly the way a root whose watcher never started does, so the page asks
the roots listing before concluding anything: a slug that has gone routes to `/`, one
that is still there keeps the "live update is not available" notice
([#115](https://github.com/davison/md-notes/issues/115#issuecomment-5718216788)). The
reviewer checked the discrimination from the other side too: a daemon killed with
`SIGKILL` under an open tab leaves `readyState` at CONNECTING, so the listing is never
asked and the tab does not conclude a removal
([#121](https://github.com/davison/md-notes/pull/121#issuecomment-5718407586)).

- **Trade-off:** it holds only for roots with a stream — see the narrowing below.
- **Rejected:** a timer asking "have I been removed yet", judged a worse thing to own
  than a stale navigator on a root that already says live update is off.

### Nothing was added to the tailnet allow-list

`DELETE /api/roots/{slug}` is refused under the tailnet name by the default that covers
every endpoint nobody has considered under that heading: the list names `/api/roots` and
admits reads of it, and `/api/roots/{slug}` matches no case. The allow-list table in
`tailnet_test.go` gained the DELETE on its refused side, and `remoteAllowed` gained a
comment and no code. Registering with a `file` is refused there too — the field changes
what the daemon checks before it registers, not who may ask
([#115](https://github.com/davison/md-notes/issues/115#issuecomment-5718216788)). The
reviewer read the diff and exercised it against a daemon with `--tailnet-host`: the
DELETE and the `file`-bearing POST both `403 loopback_only`, `GET /api/roots` still
`200`, the state file unmoved after each
([#121](https://github.com/davison/md-notes/pull/121#issuecomment-5718407586)).

- **Trade-off:** the endpoint is invisible to the tailnet by default rather than by a
  statement, which is exactly why the introduction now names it in the table.
- **Rejected:** naming it in `remoteAllowed` (it would suggest the list is an inventory
  rather than an allow-list).

### The order control sits in the navigator's own header

A toggle above the tree, right-aligned, inside `Navigator` rather than in `RootView`
beside the tag filter — because the drawer is the same component at a narrow width, so
one control exists in both layouts without a second copy or a media query in script.
Above the tree rather than below it, because a long tree scrolls a control below it out
of sight, which is why **New note** moved to the top bar in #85. It renders even when the
tree is empty or the tag filter matched nothing, so a reader is never left holding a
filter with no way back to a familiar order
([#116](https://github.com/davison/md-notes/issues/116#issuecomment-5718168243)).

- **Trade-off:** the wide layout loses about 32 px from the top of the navigator and the
  drawer about 44 px, and the tag filter's chrome is in the other pane, so the two
  controls are not side by side.
- **Rejected:** the top bar (the only per-pane control there, and the bar is full at
  phone widths); the drawer head (exists only below the breakpoint); a `<select>` (a
  native picker on a phone, and a second idiom in an application whose switches are all
  `aria-pressed` buttons).

### The accessible name never changes; `aria-pressed` carries the order

The button's accessible name is its visible text, **Recent first**, in both states. A
name that swapped to "A to Z" would say what the button does next while the pressed state
said what is on now, and a screen reader reads the two together; a fixed name also keeps
WCAG 2.5.3 label-in-name true, which an `aria-label` over different visible text would
not ([#116](https://github.com/davison/md-notes/issues/116#issuecomment-5718168243)).

- **Trade-off:** a reader who does not use the pressed state has only the label to go
  on.
- **Rejected:** a label that swaps; an `aria-label` unlike the visible text. The review
  took the third string away as well — a constant `title` describing only one direction
  ([#119](https://github.com/davison/md-notes/pull/119#issuecomment-5718463895)).

### Folders are ranked in the UI, not in the daemon

The daemon sends `modified` on file nodes only; the navigator ranks a folder by the
newest note anywhere beneath it in one post-order pass. Under a tag filter the navigator
shows a subset of the notes and a folder must be ordered by the newest note *it is
showing* — an aggregate computed by the daemon would be right only while no filter was
on, which is to say wrong exactly when the filter is on
([#116](https://github.com/davison/md-notes/issues/116#issuecomment-5718168243)). The
review drove the filtered case and got `docs, archive`, the reverse of both the
alphabetical order and the unfiltered recency order — the premise holding from outside
([#119](https://github.com/davison/md-notes/pull/119#issuecomment-5718387766)).

- **Trade-off:** the ranking is recomputed in the browser on every tree load, and an
  empty folder (or one holding only unstattable notes) ranks last among folders.
- **Rejected:** a directory aggregate on the wire.

### A note with no readable modification time sorts last, and does not fail the listing

`tree.Stat` leaves such a node without `modified` and logs anything that is not a missing
file; between the ripgrep listing and the stat a note can be deleted or renamed, and a
sync landing mid-request is the everyday way to see it, so a tree that 500s for one
missing file is a navigator that empties itself. In the navigator the note sorts last
among its siblings and its folder last among folders holding nothing newer: "we do not
know when this changed" is not a claim that it changed just now
([#116](https://github.com/davison/md-notes/issues/116#issuecomment-5718168243)). The
reviewer reached the case for real with `chmod 0600` on a directory — ripgrep still lists
the note, `os.Stat` fails `EACCES` — and saw exactly that, one log line and no failure
([#119](https://github.com/davison/md-notes/pull/119#issuecomment-5718387766)).

- **Trade-off:** `omitempty` means a genuine epoch-zero mtime would be dropped and read
  as 0, which is the value that sorts last anyway.
- **Rejected:** failing the listing; logging every absent file.

### Live refresh is the whole-tree refetch that was already there

`RootView` already calls `loadTree()` on every change batch `affectsTree` accepts, and a
note saved in the app and a note written on disk are both such a batch, so the new times
arrive by a path that needed no new code. The alternative — patching the named paths'
times from the event with no fetch — cannot *replace* the refetch, because the events
stream names a path and not what happened to it: a create, a delete and a rename are
indistinguishable from a modify and the tree's shape still has to come from the daemon.
It would be an addition, and it would order the tree by the browser's clock until the
refetch reconciled it
([#116](https://github.com/davison/md-notes/issues/116#issuecomment-5718168243)).

What the refetch costs, measured over 50 runs on loopback, 25 directories, two passes:

| root | before, median | after, median | response before → after |
|------|---------------|---------------|--------------------------|
| 300 notes | 4.54 / 4.20 ms | 11.07 / 7.12 ms | 20,546 → 28,046 B |
| 5,000 notes | 13.68 / 11.94 ms | 32.84 / 29.73 ms | 321,346 → 446,346 B |

The stats are the smaller part — `Stat` benchmarks at 0.19–0.22 ms for 300 files and
3.6–4.3 ms for 5,000, against a ripgrep listing of 2.5–3.2 ms and 4.1–5.7 ms — and the
rest is the 25 bytes per note the field adds to the JSON, 125 kB on a 5,000-note root.
The reviewer reproduced the response sizes to the byte and got lower, noisier medians on
a busier machine, with the same shape and the same conclusion
([#119](https://github.com/davison/md-notes/pull/119#issuecomment-5718387766)).

- **Trade-off:** a 5,000-note root pays about 18 ms more per change batch, on a
  background fetch behind the daemon's 150 ms debounce rather than on an interaction
  path. The roots this application is for are the 300-note column.
- **Rejected:** making the times conditional on a query parameter — it halves the worst
  case but makes the tree endpoint's response shape depend on a client preference and
  forces a refetch on every toggle.

### Two smaller calls: milliseconds, and one storage key for the browser

Autosave fires a second after typing stops, so two saves inside one second are ordinary;
at second resolution the recency order would decide them alphabetically, which reads as
the toggle not working. And one `localStorage` key, `mdn:nav:order`, for the browser
rather than per root — M7-R3 says per browser, and a reader who wants recency wants it of
every root they open; expanded directories are the opposite case and stay keyed by root
in `mdn:nav:<slug>`. Both the read and the write are wrapped, so a browser that refuses
storage gets the alphanumeric default and keeps a change for the life of the page
([#116](https://github.com/davison/md-notes/issues/116#issuecomment-5718168243)).

### The worker does not intercept `/api/` at all, rather than passing it through

The routing policy is decided per class of request, and the first row is the one the rest
of the design hangs on: nothing under `/api/` reaches `respondWith`, so the request goes
to the network exactly as it would with no worker installed
([#118](https://github.com/davison/md-notes/issues/118#issuecomment-5718162299)). A
worker that passes an event stream *through* `respondWith` can still hold it; not
answering at all is the only shape that cannot. `/login` is on the same rule — tailnet
only, `no-store` at the daemon, and the one page whose staleness would lock a reader out
of their own notes — as is anything that is not a same-origin GET. `/assets/**` is
cache-first, because those names carry their content hash, which is the M4 immutable-year
rule restated where the worker can see it; the shell, the manifest and the icons are
network-first with the cache as the offline fallback, which is how the M4 `no-cache` rule
on `index.html` survives the worker.

- **Trade-off:** the offline app is the shell and the assets, never a note. Nothing
  about the notes is stored on the device, and the page says so.
- **Rejected:** intercepting `/api/` and passing it through, for the reason above. The
  reviewer measured what the choice costs when it is honoured — the events stream's
  disk-change latency is 167/167/166 ms with no worker and 174/166/167 ms with one — and
  confirmed by mutation that the rule is the load-bearing one: deleting the `/api/` line
  from `sw-policy.ts` fails three of the five browser checks
  ([#120](https://github.com/davison/md-notes/pull/120#issuecomment-5718549275)).

### What is precached is read out of the built shell, and the editor is not in it

The precache list is generated by the worker's own build pass from the built
`index.html` — the entry chunk, its static imports and its CSS, plus the shell, the
manifest and the SVG favicon — rather than written by hand, which is the definition of
"what the shell fetches before it paints". The lazily loaded editor chunk is left out and
cached only when it is first wanted, so installing the app does not pull 218 kB of
CodeMirror; the 192 and 512 PNG icons are left out too, because Android keeps its own
copy from the moment of install and an offline install prompt is not something anyone can
act on. The cache is named for a SHA-256 over that list and the bytes of the unhashed
files in it, so a rebuild is a new cache and `activate` deletes every `mdn-` cache that is
not the current one; there is no `skipWaiting`, so a tab running the previous build keeps
the worker that still holds that build's chunks, and `clients.claim()` on activate makes
the first visit controlled rather than the one after it
([#118](https://github.com/davison/md-notes/issues/118#issuecomment-5718162299)).

- **Trade-off:** the first `Ctrl+E` after an install is a network fetch, and offline it
  is not available at all until it has been taken once.
- **Rejected:** precaching the whole build.

### `start_url` and `scope` are both `/`, and the worker is served from the root

The requirement's default, kept, and for a reason the review later made concrete: the
scope is the whole origin so that `/r/{slug}/…` — every note, every root — is inside the
installed window, and a note opened from a link, a clip or the extension is not handed
back to the browser. The worker is served from `/sw.js` for the same reason: a worker's
default scope is its own directory, and one at the root is the only one that covers `/r/`
([#118](https://github.com/davison/md-notes/issues/118#issuecomment-5718162299)).

- **Trade-off:** a scope that wide is what made the offline behaviour of `/r/…` a
  requirement question rather than a detail — see B1 in
  [What the reviews changed](#what-the-reviews-changed).
- **Rejected:** a narrower scope.

### The manifest link carries `crossorigin="use-credentials"`

A manifest is fetched with credentials *omitted* by default, and a browser doing that
over the tailnet is handed the login page under a `401` and concludes there is nothing to
install. The attribute is the whole of what the tailnet made necessary: the guard, the
session cookie and the Host and Origin checks are untouched, and the manifest and the
worker are ordinary files in the bundle, refused to a caller with no session like every
other (`TestTailnetGuardsTheInstallableAppsFiles`)
([#118](https://github.com/davison/md-notes/issues/118#issuecomment-5718162299)).

The PR flagged one thing it had not verified — whether a browser sends the session cookie
when it downloads the manifest's *icons* — and the reviewer settled it rather than
deferring it: the two fetches are governed by different code, and only the manifest
document reads the attribute. The icons go through a path where `CredentialsMode::kInclude`
is hardcoded and `ManifestUseCredentials()` is never consulted, with `SiteForCookies`
taken from the document, so a same-origin icon carries the page's cookies exactly as an
ordinary `<img>` would; `__Host-mdn_session; Secure; SameSite=Strict` is not excluded on a
same-site request. Measured on a TLS stand-in for `tailscale serve`: the manifest parsed
with `errors: []`, `fetch("/icon-192.png", {credentials: "omit"})` `401` and
`{credentials: "same-origin"}` `200`, and a browser-initiated `<img>` loading 512×512. **No
tailnet exemption was needed and none was made**
([#120](https://github.com/davison/md-notes/pull/120#issuecomment-5718549275),
recorded as an addendum on
[#118](https://github.com/davison/md-notes/issues/118#issuecomment-5718162299)).

- **Trade-off:** one attribute in the shell that a reader of `index.html` cannot guess
  the purpose of. It carries a comment, and most of the shell's 274 added bytes are that
  comment.
- **Rejected:** widening what an unauthenticated caller may read under the tailnet name
  so that the manifest could be fetched without credentials. The milestone's gate
  forbids it, and the trace above shows nothing needed it.

### The worker is hand-written, and built as a classic script by a second Vite pass

Forty lines of routing table, with the policy itself a pure function in
`ui/src/sw-policy.ts` so the rules that matter — above all that nothing under `/api/` is
ever answered from a cache — are unit-tested without a browser as well as in one
([#118](https://github.com/davison/md-notes/issues/118#issuecomment-5718162299)).

- **Trade-off:** no Workbox. It would add a build dependency and a runtime larger than
  the thing it generates, and its defaults would have to be argued out of precaching the
  editor chunk.
- **Trade-off:** a second Vite pass (`ui/vite.sw.config.ts`, library mode, IIFE) rather
  than one build, because one Rollup build has one output format. It is the shape
  `extension/vite.inject.config.ts` already uses.
- **Rejected:** a module worker in one build. `register(…, {type: "module"})` is
  Chromium 91 and later, and the Boox's stock browser is older than that on some
  firmware — a registration that failed there would take the offline shell with it.
- **Rejected:** Lighthouse for the installability audit. Its PWA category, which held
  `installable-manifest` and the service-worker audits, was removed in Lighthouse 12 and
  nothing replaced it, so `ui/e2e/pwa.test.mjs` checks Chrome's documented criteria item
  by item instead and reads the browser's own verdict out of CDP `Page.getAppManifest`.
- **Rejected:** a manifest that changes colour with the scheme — it cannot;
  `theme_color` and `background_color` are one value each, so the manifest carries the
  light pair and the page's two `<meta name="theme-color" media="(prefers-color-scheme:
  …)">` tags carry both.

## Deviations and narrowings

### M7-R2's route home holds for every root the daemon can watch, not for every root

The requirement says "a removed root's tabs land on the home page rather than a dead
route". What ships is that, for every root with an event stream. A root whose `watch.New`
failed — the daemon logs "live update disabled for this root" — is answered `503` on its
*first* events connect, and `EventSource` does not retry a refused connection, so such a
tab has no stream to end when the root is removed and its one listing check ran while the
root was still there. It keeps its "live update is not available" notice and its stale
navigator until it next asks the daemon for something.

Found by the review
([#121](https://github.com/davison/md-notes/pull/121#issuecomment-5718407586), finding
2), left as it is with the reasoning at the site in `ui/src/events.ts` and on the task
issue ([#115](https://github.com/davison/md-notes/issues/115#issuecomment-5718465965)),
and then carried to the milestone issue at the re-reviewer's insistence, so that QA judges
M7-R2 against what the code does rather than against words it does not meet
([#114](https://github.com/davison/md-notes/issues/114#issuecomment-5718608764)). The
re-review's phrasing is the one worth keeping: it is "a narrowing of M7-R2, though, not
merely a note"
([#121](https://github.com/davison/md-notes/pull/121#issuecomment-5718597230)).

### The `404 not_found` is about the note; the folder keeps `mdn open`'s `400`

The same comment carries the second narrowing, for the same reason: the decision as first
written said "one refusal for every way of failing", which is broader than what ships.
Narrowed on the task issue and on the milestone issue, and the API table in
[the introduction](../introduction.md#the-http-api) now states both answers rather than
one.

### M7-R5's offline promise held at `start_url` before it held everywhere

The requirement asks that "the app opens offline to a page that says the daemon is
unreachable rather than a browser error". At the first review that was true of the roots
page and false of every `/r/{slug}/…` route, which said the root did not exist — and the
task's own decision to set `scope: "/"` is what made those routes the installed app's
business in the first place. Fixed in the fix pass rather than narrowed, with the check
that would have caught it added at the same time
([#120](https://github.com/davison/md-notes/pull/120#issuecomment-5718647776)). Recorded
here because the requirement was met on a reading of it that the task itself had already
made too narrow.

### #116's plan made the refresh strategy conditional on a measurement, and the measurement kept it

The plan said the whole-tree refetch stays "if the added stat cost is not material on a
5,000-note root", with patching in place as the fallback. The numbers above are what
settled it, and they are on the record with the rejected alternative rather than left as
a plan clause nobody returned to
([#116](https://github.com/davison/md-notes/issues/116#issuecomment-5718168243)).

### An absolute `file` spelled through a symlinked alias of the folder is refused

`hasFile` takes `filepath.Rel(root.Path, …)` before resolving symlinks, so
`{"path":"<real dir>","file":"<alias>/todo.md"}` is `404` although the note is really
there; the reverse spelling works, and it is the only shape anything that ships produces
(the extension sends a base name, `mdn open` sends no `file`). The behaviour was kept and
the doc comment corrected to say so, because the relative path goes on to `Resolve` and
the daemon keeps one confinement funnel rather than growing a second way of deciding what
is inside a root. Found by the review, accepted by the re-review as the call it would have
made ([#121](https://github.com/davison/md-notes/pull/121#issuecomment-5718507046),
[#121](https://github.com/davison/md-notes/pull/121#issuecomment-5718597230)).

### The documentation both tasks left stale was declared, not forgotten

#119 named the two pages its merge would falsify — the introduction's tree row and the
navigator, phone-layout and tap-target lists — and left them for this task on purpose, to
keep its branch out of #117's files; #121 did the same. The review of #119 recorded it as
"a tracked gap rather than an omission", and noted it so that this task "inherits a
written list"
([#119](https://github.com/davison/md-notes/pull/119#issuecomment-5718387766), finding
4). That list is where this task's sweep started, and [Where the record is
silent](#where-the-record-is-silent) says what the sweep found beyond it.

## What the reviews changed

All three pull requests were reviewed by a clean-context session under the reviewer
contract and merged under the operator's standing confirmation
([#121](https://github.com/davison/md-notes/pull/121#issuecomment-5718606175),
[#119](https://github.com/davison/md-notes/pull/119#issuecomment-5718471530),
[#120](https://github.com/davison/md-notes/pull/120#issuecomment-5718768663)). #119 and
#121 were approved with non-blocking findings; #120 was sent back twice, once for a
defect and once for a record. Every fix pass here touched more than wording, so each got
the focused re-review the M6 lesson asks for — and both re-reviews earned their place:
#121's found the residual now captured as
[#122](https://github.com/davison/md-notes/issues/122), and #120's found that the
decision comment the whole task rests on had been overwritten.

**The one real defect the milestone found was in code the milestone made reachable.**
`watchRoot`'s setup goroutine re-read `s.reg.Get(root.Slug)` before installing the hub and
watcher — the right idea, and correct for the removal case, but it asked only whether
*something* held that slug. M7-R2 made slugs freeable at runtime, so a slug that comes
back can name a different folder: register `/a/docs`, remove it while its setup is inside
`tree.Dirs`, register `/b/docs` — whose own `watchRoot` returns because `starting["docs"]`
is occupied — and the first goroutine then installs a watcher on `/a/docs` under the slug
`docs`. From then on the events stream under `docs` reports the wrong folder's changes.
No confinement is broken, because every refetch resolves through the registry against the
current root; it is wrong live update, not exposure
([#121](https://github.com/davison/md-notes/pull/121#issuecomment-5718407586), finding
1).

The fix asks about the root rather than the slug, and hands the slug's new owner to a
fresh `watchRoot` once the first setup's `starting` entry clears — the second half being
necessary rather than belt-and-braces, since without it the folder that took the freed
slug has no watcher at all. `TestWatcherSetupTargetsTheRootItStartedFor` reproduces the
four steps with the first setup held inside its directory listing, through an unexported
`listDirs` seam a test `Option` replaces; on the head before the fix it reports the stream
under `docs` carrying a file written into the folder the slug no longer names
([#121](https://github.com/davison/md-notes/pull/121#issuecomment-5718507046)). The
re-review tried three ways to break the hand-off — a removal during it, a third folder
taking the slug during it, and shutdown in the middle — and found it correct in all three,
with no goroutine left behind
([#121](https://github.com/davison/md-notes/pull/121#issuecomment-5718597230)).

**#119's review found a test that could not fail.** All nine cases in
`navigator.test.tsx`'s "order control" block passed with `sortTree` reduced to
`return tree`, including two whose names promise otherwise, because the fixture was one
directory and one file at each level and folders group before notes — so the two orders
were the same list everywhere the file could see
([#119](https://github.com/davison/md-notes/pull/119#issuecomment-5718387766), finding
1). The rules themselves were covered by `order.test.ts` and by the browser suite, both of
which the reviewer mutated to confirm, so it was a test-quality finding rather than a
coverage hole. The fix pass rebuilt the fixture with two folders and two notes at three
levels, times disagreeing with the alphabetical order at each, and remade the measurement:
under the same mutation the block goes from 0 of 9 failing to 6 of 9, the three survivors
being the three that should — the default order is the daemon's own, and two cases are
about the accessible name and an empty tree
([#119](https://github.com/davison/md-notes/pull/119#issuecomment-5718463895)).

The same pass removed a horizontal padding that had never applied — `.nav-order`'s base
rule is declared after the coarse-pointer block, so only `min-height` survived, which is
why the measured 41.25 px was right for the wrong reason — and dropped the tooltip
described above.

**What the reviews did not change is worth recording too.** Both reviewers rebuilt the
"before" side themselves rather than reading the numbers: #119's reproduced both bundle
figures to the byte and re-measured the tree endpoint on their own machine; #121's ran
`make check`, `go test -race -count=3`, `make e2e` and the extension e2e suite three times
each, deleted load-bearing guards to watch the tests fail, and confirmed
`closingIssuesReferences` carried the task issue alone.

**#120's blocking finding is the milestone's own scope decision coming back.** `scope: "/"`
was chosen so that `/r/{slug}/…` stays inside the installed window; the offline check
reloaded `start_url` only. So the reviewer walked the routes and found that offline —
worker active, network down, each route warmed first — `/` gave the unreachable line while
`/r/notes/`, `/r/notes/alpha.md` and `/r/notes/docs/guide.md` all said **Unknown root — No
root named `notes`**
([#120](https://github.com/davison/md-notes/pull/120#issuecomment-5718549275), B1). The
cause was one line in `ui/src/root-view.tsx`: a `listRoots()` that *failed* and one that
*succeeded and did not name this slug* both became `null`, and `null` renders "Unknown
root". The review's own words for why it blocks are the ones worth keeping — the reader
opens the installed app on the note they were last on and is told the root holding their
notes does not exist, which "reads as 'my notes are gone', where `docs/sync.md`'s own
paragraph promises 'Your notes are not there… nothing is lost'". It also made three
statements untrue at once: the introduction's, the PR body's and the decision comment's,
each of which said the app opens at the route asked for and says the daemon is not
answering.

The behaviour was created by the PR — before the worker, an offline `/r/…` navigation got
the browser's error page — and the fix separates the two cases, renders the same
unreachable line the roots page renders for a failed listing, and keeps `null` meaning
what it always meant for a daemon that answered. The browser check was written first and
watched fail on the previous head, and the offline half of `pwa.test.mjs` now walks four
routes rather than reloading the origin, asserting at each that the unreachable line is
there *and* that "Unknown root" is not
([#120](https://github.com/davison/md-notes/pull/120#issuecomment-5718647776)).

The same round's five non-blocking findings were all taken: the cache's content-hashed
name and the `activate` rotation were the one promise no test held — the reviewer replaced
the hash with a constant and every browser check and all 260 unit tests still passed — and
are now held by `ui/e2e/sw-build.test.mjs`, which runs the worker pass again with a byte
changed in the shell and back out; a maskable SVG that nothing referenced left the served
bundle; `/login` being the one client-side route the offline shell cannot open is written
down beside the rule that causes it; a bundle figure that read as brotli and was an
identity size was reworded; and two `api.ts` doc comments were brought in line with the
new network-failure wrapper.

**The re-review blocked on the record, not on the code**
([#120](https://github.com/davison/md-notes/pull/120#issuecomment-5718739359)): "The code
delta is right and I have nothing left on it… One thing blocks, and it is not code: the
decision record this PR cites no longer exists." See
[Corrections to the record itself](#corrections-to-the-record-itself). Its one
non-blocking observation — a roots-listing rejection from the slug a reader has left
landing on the slug they went to — was demonstrated in jsdom on the branch *and* on the
code before it, where the stuck message was the worse "Unknown root", so it is older than
the branch and is captured as
[#123](https://github.com/davison/md-notes/issues/123) rather than fixed in a PR that did
not cause it.

## Corrections to the record itself

**The decision comment for #118 was overwritten with a file path, and was empty for about
half an hour.** Appending the post-review addendum, the implementer edited the comment
with `gh api -X PATCH … -f body=@<path>`; `-f` sets a string field verbatim and only `-F`
expands `@`, so the API stored the path as the comment's text. The published decision went
from 6,211 characters to 116, and what stood at that URL was a scratchpad path
([#118](https://github.com/davison/md-notes/issues/118#issuecomment-5718760030)).

The re-review caught it, refused to repair it — "the record is the implementer's to
restore, not the reviewer's to write" — and named both routes back: the file the comment
now pointed at, and, if that were lost, GitHub's own `userContentEdits` on the comment
([#120](https://github.com/davison/md-notes/pull/120#issuecomment-5718739359), R1). It
also named what the loss cost while it stood: the PR body's citation resolved to a stray
path, the milestone's "evidence links resolve" gate failed, and merging would have closed
#118 with its decision gone. The comment was restored byte for byte from the file it came
from, and the correction states the lesson in the form that generalises — a comment body
long enough to be worth putting in a file is long enough that its edit should be read
back, which `gh api … --jq '.body | length'` does in one line.

This record cites that comment throughout. Everything it cites is in the restored body.

## Captures adopted, and captures raised

Adopted and closed by this milestone:

| Capture | Adopted by | What it was |
|---------|-----------|-------------|
| [#50](https://github.com/davison/md-notes/issues/50) | [#115](https://github.com/davison/md-notes/issues/115) | The file-URL intercept registering a permanent root for a file that does not exist, with no way to unregister one short of editing the state file. Raised by M3 QA ([#35](https://github.com/davison/md-notes/issues/35#issuecomment-5633486688), finding 1), dispositioned to the backlog then, and taken up here on the operator's ask. Closed by `gh codecrew task finish 115` ([#50](https://github.com/davison/md-notes/issues/50#issuecomment-5718607329)) |

Raised by this milestone's reviews and by its QA, for a later task to adopt — the
**From** cell says which, since one of the four came through the coordinator's
disposition of QA's report rather than through a review:

| Capture | From | What it is |
|---------|------|-----------|
| [#122](https://github.com/davison/md-notes/issues/122) | the re-review of [#121](https://github.com/davison/md-notes/pull/121#issuecomment-5718597230) | The watcher hand-off is spawned from one exit of the setup goroutine while `starting[slug]` is released on every exit, so a registration landing in the window between the guard's unlock and the deferred delete — or meeting the `watch.New` failure path — can still leave a registered root with no watcher. Milder than what was fixed: the page says "live update is not available", never a wrong folder's changes. Found by reading, not reproduced |
| [#123](https://github.com/davison/md-notes/issues/123) | the re-review of [#120](https://github.com/davison/md-notes/pull/120#issuecomment-5718739359), observation R2 | The roots-listing effect in `ui/src/root-view.tsx` has no cancellation guard, so a rejection from the slug a reader has just left can settle after they navigate and stick on the new slug — `rootError` is cleared only by a slug change. Demonstrated in jsdom on the branch and, with the two new lines reverted, on the code before it, where the stuck message was the worse "Unknown root". Older than the branch; the fix is the `cancelled` guard the very next effect in the same file already uses |
| [#125](https://github.com/davison/md-notes/issues/125) | M7 QA, through the coordinator's disposition ([#114](https://github.com/davison/md-notes/issues/114#issuecomment-5719060998)) | Removing the last recent root writes `{"recent": null}` to the state file rather than `{"recent": []}`. It reloads correctly and nothing depends on the shape; captured so nobody has to rediscover it |
| [#127](https://github.com/davison/md-notes/issues/127) | the review of [#126](https://github.com/davison/md-notes/pull/126#issuecomment-5719422345) | Nothing asserts that the editor chunk stays off a reading page. M4-R7 promised it and every bundle table since has reported it, but `ui/e2e/assets.test.mjs` never held it — old and new alike assert two *or more* assets on a cold load and compare the second load to the first, so an eager editor chunk would pass; the only assertion naming the chunk is about the worker's precache. True today, asserted nowhere for the page itself |

QA's other two items did not become captures. The flaky browser check became a fix task
inside the milestone, [#124](https://github.com/davison/md-notes/issues/124), merged at
[`0c049f4`](https://github.com/davison/md-notes/commit/0c049f4), which also took the
tailnet guard test's missing icons; and
[#122](https://github.com/davison/md-notes/issues/122)'s residual window, which QA's
probes did not reach, stays a capture as it was
([#114](https://github.com/davison/md-notes/issues/114#issuecomment-5719060998)).

## Known gaps at the boundary

| Gap | Where it is recorded |
|-----|----------------------|
| A root registered inside the watcher hand-off window, or one whose watcher failed to start, can be left registered with no watcher. The page says live update is not available; no wrong folder is ever streamed | [#122](https://github.com/davison/md-notes/issues/122) |
| A tab open on a root the daemon could not watch does **not** land on the home page when that root is removed. The route rides the event stream, and such a root has none | [#114](https://github.com/davison/md-notes/issues/114#issuecomment-5718608764), [#115](https://github.com/davison/md-notes/issues/115#issuecomment-5718465965), [the introduction](../introduction.md#roots) |
| A registration naming a folder that does not exist answers `400` with the OS's sentence, while one naming a note that does not exist answers `404 not_found`. Deliberate, and the two are for different callers | [#115](https://github.com/davison/md-notes/issues/115#issuecomment-5718465965) |
| An absolute `file` spelled through a symlinked alias of the folder is refused although the note is inside it. Unreachable from anything that ships | [#121](https://github.com/davison/md-notes/pull/121#issuecomment-5718507046) |
| A root whose registered note is later deleted stays a root. The remove control is the answer | [#115](https://github.com/davison/md-notes/issues/115#issuecomment-5718216788) |
| A 5,000-note root pays about 18 ms and 125 kB more per change batch for the modification times, whether or not the reader ever turns the recency order on. There is still no bundle or latency budget to weigh that against, as M5's and M6's records also said | [#116](https://github.com/davison/md-notes/issues/116#issuecomment-5718168243) |
| A genuine epoch-zero modification time is dropped by `omitempty` and read as "unknown", which sorts last. Harmless and unpinned | [#119](https://github.com/davison/md-notes/pull/119#issuecomment-5718387766) |
| Removing a root is in the app only. There is no `mdn` verb for it, and the state file remains the only other way | [the introduction](../introduction.md#the-daemon) |
| **Renaming a note is still not in the application at all.** Untouched by this milestone, as by M5 and M6 | [the README](../../README.md) |
| The M6 backlog is untouched: [#107](https://github.com/davison/md-notes/issues/107), [#108](https://github.com/davison/md-notes/issues/108), [#110](https://github.com/davison/md-notes/issues/110), [#111](https://github.com/davison/md-notes/issues/111) and [#112](https://github.com/davison/md-notes/issues/112) are all still open, by the scope decision | [#114](https://github.com/davison/md-notes/issues/114#issuecomment-5717883455) |
| A roots-listing rejection from a slug the reader has left can land on the slug they went to and stay there. Predates the installable app, which only made the message it leaves less alarming | [#123](https://github.com/davison/md-notes/issues/123) |
| **The Android install has never been performed.** Everything about it was exercised in headless Chromium, including on a TLS stand-in for `tailscale serve`; no phone and no Boox has run it. The one part headless Chromium cannot reach is Chrome's WebAPK minting server, which cannot resolve a `.ts.net` name and is expected to use the icon bytes Chrome uploads — a claim about a server whose source is not public | [#118](https://github.com/davison/md-notes/issues/118#issuecomment-5718162299), [docs/sync.md](../sync.md#installing-it-on-the-phone) |
| `/login` is the one client-side route the offline shell does not open. Right under the tailnet, where it is the daemon's own page; on loopback it is an ordinary route and the sole one the worker leaves to the browser's error page | [#120](https://github.com/davison/md-notes/pull/120#issuecomment-5718549275), N3 |
| The milestone costs the eager reading page 1,030 bytes brotli (21,675 → 22,705 B, +4.8 per cent), plus `sw.js` at 1,133 B and the manifest at 682 B fetched after the page has loaded. There is still no budget to weigh any of it against, which M5's and M6's records also said | [#118](https://github.com/davison/md-notes/issues/118#issuecomment-5719027735), and rebuilt for this record |
| Nothing asserts that the editor chunk stays off a reading page, in this milestone or in the four before it. The property holds; the check everybody has cited for it does not make the claim | [#127](https://github.com/davison/md-notes/issues/127), [#126](https://github.com/davison/md-notes/pull/126#issuecomment-5719422345) |
| The ×8 case covers the duplicate-request interleaving by construction rather than by assertion: nothing fails if Chromium ever folds the two requests and the case quietly stops covering it. Deliberate — an assertion on a browser quirk breaks on the day the quirk is fixed | [#126](https://github.com/davison/md-notes/pull/126#issuecomment-5719422345), F2 |
| Removing the last recent root leaves `{"recent": null}` in the state file. Cosmetic; it reloads correctly | [#125](https://github.com/davison/md-notes/issues/125) |
| Offline the app is the shell, never a note: nothing about the notes is stored on the device, so an installed app with no network opens and can show nothing | [the introduction](../introduction.md#installing-the-app) |

## Where the record is silent

- **The operator has still not used any of this.** Every trade-off in the three merged
  tasks was judged by an implementer, a reviewer and — when it runs — a QA session, all
  model sessions under one identity. The order toggle was driven on a Pixel 7 profile in
  headless Chromium and the remove control on a coarse-pointer desktop profile; neither
  has been touched on the operator's phone or on the Boox, and the milestone's headline
  feature is an app that installs on a phone nobody has installed it on. M6's record
  said the same, and the one M5 finding that reads most like a person's remains the
  model for what these sessions do not catch.
- **`docs/sync.md` carried a claim no task's file list would have caught.** It said a
  Syncthing conflict file "sorts next to the note it came from", which after #116 is
  only the alphanumeric order's answer. Nothing in either task's plan, PR body or review
  named that page; it was found by this task reading every page in `docs/` rather than
  the two the reviews handed over. M5's lesson was that a requirement's first clause is
  wider than its file list and M6's was that writing the sweep into a plan is not the
  same as running it — this is the third instance, and the sweep is now the only
  mechanism that has ever caught one.
- **The introduction's opening sentence had been stale since M6.** It said the page
  described the system "at the end of milestone five" and was still saying so after M6's
  own documentation task, whose record named documentation staleness as its closing
  lesson. Corrected here.
- **The `ui/e2e` figure in the introduction was stale for three milestones, and no task
  whose work changed it noticed.** The page said 35 checks from M4 until M7's last task;
  the suite ran 38 on `main` before this milestone and runs 54 now. The correction was
  made by [#124](https://github.com/davison/md-notes/issues/124) in
  [`a62ce62`](https://github.com/davison/md-notes/commit/a62ce62) — "The count had been
  35 since M4 and the suite is 54 checks; M7 added ten and this task one" — and this
  task only measured the figure it inherited. What none of the three milestones between
  had is anything that would fail when the number drifts: it is a count in prose, and
  every merge that adds a check makes it wrong again.
- **No `cc:needs-decision` gate was raised in this milestone, and neither implementation
  task had an ask-the-human point.** Both said so explicitly in their plans, on the
  ground that the requirements had already fixed the defaults and everything else was an
  implementation decision to be recorded. The one human decision the milestone contains
  — adding a fifth requirement to an open milestone — was taken by the coordinator on
  the operator's words in a session that is not on GitHub, and recorded after the fact
  ([#114](https://github.com/davison/md-notes/issues/114#issuecomment-5717938933)). That
  is the same shape M6's record named, and it stands here.
- **The severity assessment of #50 is the coordinator's, and nothing independent tested
  it.** Neither review nor this record re-derived the claim that the defect is bounded
  by the `file:` trigger and by loopback-only registration; QA's brief covers the code's
  behaviour, not the threat model. The assessment is recorded and reasoned
  ([#114](https://github.com/davison/md-notes/issues/114#issuecomment-5717883455)), and
  it is a single session's judgement.
- **A merged record said a check held something it did not.** PR #120's body said
  `assets.test.mjs` "still measures two assets on a cold reading-page load", which reads
  as the editor chunk being held off the reading page; the review of #126 found that
  neither the old case nor the new one makes that claim, and that no check in the tree
  does ([#126](https://github.com/davison/md-notes/pull/126#issuecomment-5719422345)).
  The property is true and has been true since M4 — what was never true is that anything
  would notice if it stopped being. Captured as
  [#127](https://github.com/davison/md-notes/issues/127); the sealed record it touches
  is not this one's to edit.
- **Nothing measured how long a root registered by accident had been possible to reach
  over the tailnet in practice.** #50 was raised in M3 QA and fixed in M7, four
  milestones later, on a daemon the operator runs. The record contains no statement
  about whether any such root ever existed on it.
- **A decision comment was destroyed by a tool flag and nothing but a review noticed.**
  The half hour in which #118's decision said only a scratchpad path is recorded
  ([#118](https://github.com/davison/md-notes/issues/118#issuecomment-5718760030)), and
  the re-review is what found it. Nothing in the protocol reads a comment back after
  writing it, and `gh codecrew milestone evidence` checks that a record's citations
  *resolve* — which this one did, to an empty decision. What the incident does not
  establish is how many other comments in this project's history were edited and never
  read back.
- **The milestone's requirements grew by 20% after it opened, and the record cannot say
  what that cost.** M7-R5 arrived with both implementation tasks already in flight, and
  the third task ran in parallel with the two that were already running. It merged last
  and needed two review rounds where the others needed one, which is consistent with
  being the largest task and equally consistent with being the one written under the
  most time pressure; nothing here distinguishes the two.

- **One of QA's observations reached GitHub only through a session.**
  [#125](https://github.com/davison/md-notes/issues/125) — the state file written as
  `{"recent": null}` after the last removal — first cited "observation 3" of the QA
  verdict, and the published verdict
  ([#114](https://github.com/davison/md-notes/issues/114#issuecomment-5719043677)) has
  no numbered observations and does not mention the shape at all. Its real source is the
  out-of-scope observations in QA's report to the coordinator, which only the
  coordinator's disposition
  ([#114](https://github.com/davison/md-notes/issues/114#issuecomment-5719060998))
  carries onto the record; the capture now says so, corrected after this record found
  the original citation did not resolve to what it named. The finding and the fix were
  never in doubt — what was missing is the path from the probe that found it to the
  issue that records it. That is the same class of gap as #118's decision comment
  overwritten by a flag, and as M5's decisions written down only after a record named
  their absence: a session holds something GitHub does not, and only somebody reading
  the record against its sources notices.
