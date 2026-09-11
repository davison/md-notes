# M3 — Browser extension clipper, daemon authentication and tailnet access

Tracking issue: [#35](https://github.com/davison/md-notes/issues/35). Its four
implementation tasks are merged on `main` at
[`b6b0866`](https://github.com/davison/md-notes/commit/b6b0866).

## Goal and outcome

The milestone's goal, as stated on [#35](https://github.com/davison/md-notes/issues/35),
was a Chromium Manifest V3 extension for Brave that clips a readable page or a
selection into the notes root through the daemon, opens local markdown files in the
app, and the authentication the daemon needs to accept those writes and to be reached
from another node on the tailnet. The inbox clipper for URLs shared from a phone was
explicitly left for later.

The coordinator fixed the scope and the open defaults in one decision on the
operator's instruction, taken while the operator was abroad and checking in
infrequently
([#35](https://github.com/davison/md-notes/issues/35#issuecomment-5621679656)): the M2
follow-up backlog ([#30](https://github.com/davison/md-notes/issues/30)–[#34](https://github.com/davison/md-notes/issues/34))
waits for a later polish milestone; tailnet access
([#10](https://github.com/davison/md-notes/issues/10)) is adopted here rather than
later, because it needs the same token and designing authentication twice would be the
alternative; and the naming, frontmatter and credential defaults the requirements would
otherwise have left open were written into them, for the operator to overrule.

What shipped is the browser half of md-notes and the authentication under it. The
daemon holds one bearer token per installation, generated on first start, stored
`0600` beside the state file, printed and rotated by `mdn token`; a request presenting
it passes the Origin guard from any origin, and a rotation revokes the replaced token
on the next request without a restart. `POST /api/clip` turns `{url, title, markdown,
kind}` into a dated, slugged note under `clips/` in the notes root, with `title`,
`source`, `clipped` and `tags: [clip]` above the markdown written byte for byte, and
the new note reaches the navigator through the existing live update. The extension
clips a page or a selection from the toolbar and the page's context menu — Readability
then Turndown, converted in the page — shows the title in an editable popup, and links
to the created note; it also intercepts `file:` URLs ending in `.md` or `.markdown` and
opens them in the app, registering the file's folder as a root when no existing root
contains it. And the daemon answers one configured extra `Host` name for a
`tailscale serve` proxy, where nothing is served without proof of the token: a browser
logs in once and holds a host-scoped session cookie, an API client sends the header,
and what either reaches is narrowed to the UI's own API.

The system as it stands is described in [the introduction](../introduction.md);
[The browser extension](../extension.md) is the page this milestone added, and
[Reaching the daemon over the tailnet](../introduction.md#reaching-the-daemon-over-the-tailnet)
is the section it added to the other.

Four implementation tasks delivered it, each through its own PR and review loop, and
this document is the fifth ([#40](https://github.com/davison/md-notes/issues/40)):

| Task | Requirements | PR |
|------|--------------|----|
| [#36](https://github.com/davison/md-notes/issues/36) Bearer token and the clip endpoint | M3-R1, M3-R2 (and M3-R3's server half) | [#42](https://github.com/davison/md-notes/pull/42) |
| [#38](https://github.com/davison/md-notes/issues/38) Extension: open local markdown files, build and install | M3-R4, M3-R5 | [#41](https://github.com/davison/md-notes/pull/41) |
| [#37](https://github.com/davison/md-notes/issues/37) Extension: clip page and selection | M3-R3 | [#43](https://github.com/davison/md-notes/pull/43) |
| [#39](https://github.com/davison/md-notes/issues/39) Tailnet access: extra Host name, token login and session cookie | M3-R6 | [#44](https://github.com/davison/md-notes/pull/44) |

[#39](https://github.com/davison/md-notes/issues/39) formally adopted
[#10](https://github.com/davison/md-notes/issues/10), the tailnet capture milestone one
left open, and closed it on merge.

The sequencing came from the same coordinator decision:
[#36](https://github.com/davison/md-notes/issues/36) first with
[#38](https://github.com/davison/md-notes/issues/38) in parallel — the file-URL
intercept needs only the existing roots API, and that task owns the extension's
directory and build — then [#37](https://github.com/davison/md-notes/issues/37) on
#38's scaffold and #36's contract, then
[#39](https://github.com/davison/md-notes/issues/39) on #36's token. It held: the one
place the parallelism cost anything is recorded as a deviation on
[#37](https://github.com/davison/md-notes/issues/37#issuecomment-5632499768), where
#38's end-to-end suite had gone stale against the merged token daemon and #37 repaired
it rather than leaving a red suite for QA to rediscover.

Partway through, the operator made merge confirmation standing for the rest of the
milestone
([#35](https://github.com/davison/md-notes/issues/35#issuecomment-5632220363)): "just
go ahead and merge, I'll test it once it's available in the browser". From that point
the coordinator merged each task once its PR carried an approving model review and
green CI, rather than waiting for a check-in. No human decision gate
(`cc:needs-decision`) was raised anywhere in this milestone — the only one of the three
so far where none was.

## Requirement outcomes

The verdicts below are drawn from the independent QA comment on the milestone issue
([#35](https://github.com/davison/md-notes/issues/35#issuecomment-5633486688)), run
against the binary and the `extension/dist` a clean worktree of `main` at
[`b6b0866`](https://github.com/davison/md-notes/commit/b6b0866) produced — five
temporary daemons above port 8800 on their own configs, state files, token files and
roots, one of them behind a local TLS-terminating proxy, and headless Chromium driving
the built extension and the UI against QA's own fixture pages rather than the
repository's. It raised seven findings, none blocking; the coordinator's disposition
of them is at
[#35](https://github.com/davison/md-notes/issues/35#issuecomment-5633500584).

| ID | Requirement | Status |
|----|-------------|--------|
| M3-R1 | Token authentication: one bearer token per installation at `0600`, `mdn token` and `--rotate`, the Origin exemption, `401` for a wrong or revoked token | [Satisfied](https://github.com/davison/md-notes/issues/35#issuecomment-5633486688) |
| M3-R2 | Clip endpoint: a dated, slugged, deduplicated note under `clips_dir` with the documented frontmatter, confined to the notes root, appearing live | [Satisfied](https://github.com/davison/md-notes/issues/35#issuecomment-5633486688) — one low finding accepted |
| M3-R3 | Clipping from the browser: both entry points, Readability plus Turndown with GFM, the editable title, the success link and the named failures, the options page | [Satisfied](https://github.com/davison/md-notes/issues/35#issuecomment-5633486688) — three low findings accepted |
| M3-R4 | Opening local markdown in the app: the `file:` intercept, both open paths, the page left alone when the daemon is unreachable, the file URL permission documented | [Satisfied](https://github.com/davison/md-notes/issues/35#issuecomment-5633486688) — one finding filed as [#50](https://github.com/davison/md-notes/issues/50) |
| M3-R5 | Build and install: the extension in the repository, `make extension` producing both artefacts, typecheck and tests in `make check`, least permissions, the install documentation | [Satisfied](https://github.com/davison/md-notes/issues/35#issuecomment-5633486688) |
| M3-R6 | Tailnet access: one extra `Host` name, everything under it authenticated, the login page and session cookie, the UI and events stream end to end, loopback unchanged, the premise restated | [Satisfied](https://github.com/davison/md-notes/issues/35#issuecomment-5633486688) |
| M3-R7 | Documentation and record: the token, the clip endpoint, the extension, local files and tailnet access documented; the roadmap row; this record | [Not satisfied, provisionally](https://github.com/davison/md-notes/issues/35#issuecomment-5633486688), on the state of `main` before this task — a superseding verdict is outstanding, see below |

M3-R7 is the one row no verdict yet settles, and for the same reason M2-R6 was:
QA graded it against `main` as it stood before this task and said the verdict "should
be superseded once #40 merges". Everything it could check against behaviour it found
accurate in every particular it could provoke — the token file rules, rotation, the
CORS constraint, all five rows of the Refusals table, the clip request and response
shapes, the slug rules to the letter, and the whole tailnet section including the
forwarded-header table and the sentence about a non-443 `serve` port, which QA had to
rely on to build its proxy rig at all. What it graded as missing is the
milestone-level half — the boundary claims, the roadmap row, this record, and a
cross-reference from the introduction to
[the extension page](../extension.md) — every item of it this task's declared scope,
listed as finding 6 so it could be acted on directly. The closure gate on
[#35](https://github.com/davison/md-notes/issues/35) requires both that every
requirement verdict is satisfied *and* that the milestone document is merged, so the
merge of [#49](https://github.com/davison/md-notes/pull/49) is a precondition of
closure rather than the verdict itself; the coordinator's disposition says a
superseding M3-R7 verdict follows it
([#35](https://github.com/davison/md-notes/issues/35#issuecomment-5633500584)).

QA's seven findings, and what was done with each:

| Finding | Disposition |
|---------|-------------|
| 1 — a `file:` URL naming a markdown file that **does not exist** still registers its directory as a permanent root, so navigating to `file:///etc/no-such-note.md` registers `/etc` | Backlog, captured as [#50](https://github.com/davison/md-notes/issues/50) |
| 2 — Readability lifts the article's own `<h1>` out of a page clip | Accepted; the title survives in the frontmatter and as the app's heading, and a selection clip keeps its `<h1>`s |
| 3 — task-list checkboxes survive a selection clip but not a page clip | Accepted, with the record corrected and one sentence added to [the extension page](../extension.md#what-ends-up-in-the-note); see [Corrections](#corrections-to-the-record-itself) |
| 4 — a table with no header row is emitted as raw `<table>` HTML | Accepted; already captured as [#45](https://github.com/davison/md-notes/issues/45) from the review of [#43](https://github.com/davison/md-notes/pull/43) |
| 5 — the login throttle's total floor was not documented | Folded into this task: [the introduction](../introduction.md#a-browser-on-the-tailnet) now says that once the daemon has seen more than three failures in the window across every address, each further failure waits half a second whatever `X-Forwarded-For` claims, and that a key-varying caller therefore meets the delay and never the `429` |
| 6 — statements on `main` the merged work made false, and the milestone-level documentation not yet there | Folded into this task; it is the boundary refresh, the roadmap row, this record, and the introduction's link to the extension page |
| 7 — a `clips_dir` naming an existing regular file fails at clip time (`500 io_error`) rather than at startup | Accepted; nothing is overwritten and the confinement that matters holds, so it is a configuration mistake surfacing late rather than a hole |

- **Trade-off, as recorded in the disposition:** findings 2, 3, 4 and 7 put nothing at
  risk and none falls within a requirement's wording, so fixing them inside the
  milestone would extend it for cosmetic outcomes. Finding 1 is real and has a clear
  shape, but it needs a decision about what should happen when a navigation names a
  file that is not there — a silent no-op, a confirmation, or an existence check —
  rather than a patch inside a documentation task.
- **Rejected:** a remedy task inside this milestone;
  [#50](https://github.com/davison/md-notes/issues/50) records the shape for a later
  one to adopt.

The operator's own check of the extension in Brave is recorded on
[#35](https://github.com/davison/md-notes/issues/35) when they are back at a desk and
is deliberately **not** a closure gate; anything it finds becomes a backlog capture or
a remedy task. Two things neither the reviews nor QA could reach are waiting for it:
Chromium's own `activeTab` grant, which is given by a user gesture no harness can
perform, and the `chrome.permissions.request` prompt for a non-default daemon URL,
which headless Chromium blocks on
([#41](https://github.com/davison/md-notes/pull/41#issuecomment-5622167896)).

## Decisions

### The token lives in its own file, and a rotation reaches a running daemon by re-reading it

M3-R1 says the token is "stored with the state file" and leaves the exact file open. It
is `$XDG_STATE_HOME/mdn/token`, one line, `0600`, in a `0700` directory, beside
`roots.json` rather than inside it: `roots.json` is a cache the daemon is entitled to
discard on a parse error, and a token that vanished with it would silently lock the
extension out. `mdn token` creates it when it does not exist, so a client can be
configured before the daemon has ever run, and `--token-file` moves it for both
commands
([#36](https://github.com/davison/md-notes/issues/36#issuecomment-5621817420)).

Rotation is picked up by re-reading the file rather than by restarting the daemon —
`mdn token --rotate` takes effect on the next request, with no `systemctl --user
restart mdn` in the extension's setup instructions.

- **Trade-off:** two files in the state directory instead of one, and a reader has to
  know which. Against that, the secret has an obvious home, `mdn token` can create it
  without loading or rewriting the roots state, and the file mode is the whole of its
  protection rather than a property of a mixed-purpose document.
- **Rejected:** a `token` key inside `roots.json` (couples the secret's lifetime to a
  discardable cache, and every root add rewrites the secret); an environment variable or
  a key in `~/.config/mdn/config.yml` (not `0600`, and the file a user pastes into an
  issue); deriving the token from the machine (unrotatable); requiring a restart after
  `--rotate` (a footgun in the setup instructions); watching the file with fsnotify (a
  third watcher and a startup ordering problem for a file read in microseconds).

The *mechanism* of the re-read did not survive contact with the binary, and is the
milestone's first deviation — see
[Rotation had to revoke, not merely admit](#rotation-had-to-revoke-not-merely-admit).

### A bad `Authorization` header is a `401`, not a fall-back to the Origin rule

M3-R1 fixes the `401` for a wrong or revoked token; what it leaves open is whether a
bad token on a *same-origin* request should still be served. It is not. A stale token
in the extension's options page must be reported as a stale token, and silently serving
the request would hide the rotation the user just performed. Requests with no
`Authorization` header at all are untouched, so the web UI over loopback behaves
exactly as before, and the `Host` check is never waived by the token — DNS rebinding is
a separate attack, and a rebound page holds no token
([#36](https://github.com/davison/md-notes/issues/36#issuecomment-5621817420)).

- **Trade-off:** a client that sends a junk `Authorization` header for unrelated
  reasons is refused where it used to be served. Nothing in the project does, and the
  alternative makes "token rejected" undiagnosable.
- **Rejected:** ignoring an unparseable header and applying the Origin rule; accepting
  other schemes such as `Basic`, for which the daemon offers no credential.

### `201 {root, path}`, and `clips_dir` as a configuration key with no flag

M3-R2 asks for the created path to be returned. A bare path is not enough for M3-R3,
which has to show a link that opens the note in the app: app URLs are `/r/{slug}/{path}`
and the slug is derived from the root's basename and deduplicated, so the extension
cannot compute it. Returning both keeps the extension free of a second round trip to
`GET /api/roots`. `clips_dir` gets no `mdn serve` flag because, unlike `--root`,
`--port` and `--max-watches`, it names a place *inside* the notes root that should not
vary between one start and the next; `--root` already relocates it wholesale
([#36](https://github.com/davison/md-notes/issues/36#issuecomment-5621889774)).

- **Trade-off:** two fields where the requirement named one, and one key that cannot be
  overridden from the command line. A field the extension ignores costs nothing; a
  missing flag is one line to add if a use appears.
- **Rejected:** returning an absolute filesystem path (leaks the layout, and the app
  cannot navigate to it); returning a ready-made app URL (the daemon would have to know
  its own external origin, which is #39's problem, not this endpoint's).

### The slug folds Latin accents and falls back to `untitled`; the frontmatter carries exactly four keys

Folding U+00C0–U+00FF gives `cafe` rather than `caf` for a title like "Café" with no new
dependency; anything outside that becomes a separator, so a title in another script
yields the fallback, and the frontmatter — which keeps the title verbatim — is where
that title is still readable. The bound is 64 characters, cut back to the last hyphen so
a name never ends mid-word. `kind` is required and validated because the contract #37
builds against needs it fixed, and is deliberately *not* written into the note, because
M3-R2 enumerates the frontmatter and adding a key to that list would be a requirement
change ([#36](https://github.com/davison/md-notes/issues/36#issuecomment-5621889774)).

- **Trade-off:** a CJK page title yields `YYYY-MM-DD-untitled.md`, `-2`, `-3`, which is
  poor naming for a reader of the directory. Transliterating those scripts properly is a
  dependency and a large table; the frontmatter title and the `clip` tag make the note
  findable meanwhile.
- **Rejected:** `golang.org/x/text/unicode/norm` for a filename (a dependency for a
  cosmetic gain in a project with six); a slug taken from the URL's host when the title
  yields nothing (two rules where one will do); dropping `kind` from the request.

The mitigation this decision rests on — that the frontmatter keeps the title readable —
was found to be false for exactly the titles it was about, and the review that found it
is in [What the reviews changed](#what-the-reviews-and-qa-changed).

### The markdown is written byte for byte, and the note is created through a root handle

Verbatim is what M3-R2 says and what the source API already promises for edits; a clip
that came back subtly different from what the extension converted would be a bug that
only shows up in a diff. `O_EXCL` through an `os.Root` handle on the notes root is what
makes the `-2`, `-3` search correct under a concurrent clip and guarantees an existing
note is never overwritten — the endpoint creates and has no update path at all — and a
`clips_dir` that resolves out of the root is diagnosed as `outside_root` before anything
is written, re-checked after the directory is created in case a symlink raced in
([#36](https://github.com/davison/md-notes/issues/36#issuecomment-5621889774)).

- **Trade-off:** a clip whose markdown lacks a final newline produces a file without
  one. That is the conversion's business, not the daemon's — and the extension gives the
  markdown exactly one trailing newline in the one place that builds the request
  ([#37](https://github.com/davison/md-notes/issues/37#issuecomment-5632495099)).
- **Rejected:** appending a newline when one is missing (a small silent edit is still an
  edit); `MkdirAll` on an absolute path followed by `os.Create` (the confinement would
  then be lexical only, which a symlink defeats); reusing the source store's
  replace-by-rename (it exists to preserve an existing file's identity and mode, neither
  of which a new file has).

### The token grants the whole API, and #39 is where that is narrowed

Review finding 8 on [#42](https://github.com/davison/md-notes/pull/42#issuecomment-5622127832)
demonstrated the shape: with the token and any `Origin`, a caller can `POST /api/roots`
to register an arbitrary directory and then read any file beneath it through the raw
endpoint. That is the deliberate design of M3-R1 — one guard for the whole surface —
and on loopback it is contained: the token is `0600`, any local process that can read it
already runs as the user and can read the notes directly, and a hostile *page* cannot
use it at all because the preflight is refused. What changes under M3-R6 is not the
rule but its reachability, and narrowing it is a design choice belonging to the task
that makes the daemon reachable, with the tailnet's requirements in front of it
([#36](https://github.com/davison/md-notes/issues/36#issuecomment-5622222887)).

- **Trade-off:** M3-R6's work starts with a wider credential than it may want, and if
  #39 narrows it the extension's contract may shift once more before the milestone
  closes — cheap while #37 has not shipped. Against that, splitting the token here
  would invent two credentials before anything needs the second, and would have to
  guess at the login flow #39 designs.
- **Rejected:** confining the token to `POST /api/clip` in this task (M3-R1 says the
  guard, not the endpoint); minting a separate clip-only token now (two secrets, two
  rotations and two things to paste, for a boundary that only matters once the daemon is
  reachable); doing nothing and leaving it unwritten — the reviewer had to find it by
  trying, which is the wrong way for #39 to meet it.

This is the decision that shaped the milestone's second half: #39 opens by answering it.

### No CORS preflight is answered and no CORS header is ever sent

An `Authorization` header makes a cross-origin `fetch` non-simple, so a browser page
sends `OPTIONS` first; the guard refuses it like any other cross-origin request
(`403 {"code":"cross_origin"}`), and no successful response carries
`Access-Control-Allow-Origin` either. The reasoning is that a page which has merely
found the port must not be able to write to the notes even if it has somehow obtained
the token, and refusing the preflight is what keeps that true. It is also the
containment the token-scope decision above depends on
([#36](https://github.com/davison/md-notes/issues/36#issuecomment-5622353217)).
`TestGuardRefusesThePreflight` pins both the `403` and the absence of the header, so
relaxing it later is a deliberate act with a failing test in front of it.

- **Trade-off:** a caller CORS *does* govern fails with the browser's opaque CORS error
  rather than one of the daemon's documented codes, which is a poor diagnostic for
  anyone who tries it — mitigated only by saying so in the documentation.
- **Rejected:** answering the preflight for a token-bearing request (it would widen the
  write path from one exempt class of caller to any page that can obtain the token,
  which is the opposite of the boundary this milestone draws); a preflight answered only
  for `chrome-extension://` origins (an allow-list nothing verifies — any extension the
  user installs would qualify); a per-endpoint allow-list for the exempt origin (a
  second authorisation model beside the one M3-R1 fixes at the guard).

The settlement's *reason* named the wrong set of callers, and was corrected twice; see
[Corrections to the record itself](#corrections-to-the-record-itself). The boundary
itself, and every test of it, are unchanged.

### `tabs.onUpdated` for the file-URL intercept, and no `tabs` or `webNavigation`

Probed against headless Chromium before choosing: with `host_permissions: ["file:///*"]`
alone, `tabs.onUpdated` already delivers a file tab's URL. `webNavigation` would have
been the more obvious API but is a separately warned permission that exposes every
navigation in the browser, and `tabs` exposes every tab's URL
([#38](https://github.com/davison/md-notes/issues/38#issuecomment-5621771957)).

- **Trade-off:** `onUpdated` fires slightly later than
  `webNavigation.onBeforeNavigate`, so the file may have begun rendering as plain text
  before the redirect replaces it. The file is local and small, and the flicker costs
  less than two broad permissions.
- **Rejected:** `webNavigation` with a URL filter (broader permission for a marginally
  earlier hook); `declarativeNetRequest` (the redirect target depends on the daemon's
  answer, which a static rule cannot express); a content script matched on
  `file:///*.md`.

The same choice was re-examined when the review found that a reload never retried, and
held: the fix reads the listener's own `tab.url`, delivered under the same `file:///*`
permission, so the least-permission decision stands unchanged
([#38](https://github.com/davison/md-notes/issues/38#issuecomment-5622314618)).

### Host permission pinned to the daemon's default origin, with the rest optional

Chromium enforces the port in a host permission — probed with `http://localhost:7401/*`
granted, a fetch to `localhost:7402` fails and `chrome.permissions.contains` returns
false for it. A manifest cannot carry a configurable origin, so the static grant is the
documented default and anything else is an explicit, user-gestured grant from the
options page, which is also what M3-R6's tailnet name needs
([#38](https://github.com/davison/md-notes/issues/38#issuecomment-5621771957)).

- **Trade-off:** a user who moves the daemon's port meets a permission prompt on their
  first options save. That is the honest cost of not shipping `http://*/*` in the
  manifest.
- **Rejected:** `http://localhost/*` and `http://127.0.0.1/*` (any port on loopback —
  simpler, but strictly wider than the requirement asks for); `<all_urls>`.

### "Test connection" asks whether the daemon judges tokens at all

Sending the bearer header on the connection test was the review's ask, and it is
necessary but not sufficient: probed in Chromium, a GET carrying an `Authorization`
header from an extension context is still not a CORS request, so no `Origin` is attached
and a daemon predating M3-R1 returns `200` whatever the token says. The page therefore
presents a token the daemon cannot have issued first: a `200` to *that* means this
daemon does not check tokens and it says exactly that, a `401` means it does and the
real token is then judged on its own answer
([#38](https://github.com/davison/md-notes/issues/38#issuecomment-5622314618)).

- **Trade-off:** one extra request per click, and one deliberate failed authentication
  in the daemon's log — only on a button the user pressed.
- **Rejected:** sending the token and trusting a `200` (the defect, moved); a version
  check endpoint (does not exist, and would be a requirement change).

### The zip is written by the project, and `make check` runs the build

`zip` is not installed on the development machine and `make check` should not fail for
want of it. Node is already a build prerequisite, so eighty lines of the ZIP format buys
a packer with no new dependency and one that is deterministic — entries sorted,
timestamps fixed, so an unchanged build packs to an identical archive
([#41](https://github.com/davison/md-notes/pull/41#issuecomment-5621976721)).

- **Trade-off:** a hand-written archive writer is code the project now owns. It is
  verified: the emitted zip extracts byte-identical to `dist/` and the extracted tree
  loads into Chromium as an extension.
- **Rejected:** `zip -qrX` (a tool that is not everywhere, and non-deterministic without
  care); `bsdtar --format zip` (not on GitHub's runners by default); `python3 -m
  zipfile` (a third language in the build); an npm zip dependency.

The review noted that `make check` stopped at the build and never ran the packer; it now
runs the full `extension` target
([`a0fe52f`](https://github.com/davison/md-notes/commit/a0fe52f)).

### The conversion runs in the page, in the same injected bundle as the extraction

Readability needs a live document and a selection exists nowhere but in the page, so the
extraction was never in question; the conversion could have gone either way. It goes in
the page because Turndown parses HTML with `DOMParser` and an MV3 service worker has no
DOM at all — putting it in the worker means shipping a DOM implementation into the
worker to reparse HTML that was parsed a moment earlier in the page. Converting where
the DOM already is keeps what crosses the boundary small: a string of markdown rather
than a page of HTML
([#37](https://github.com/davison/md-notes/issues/37#issuecomment-5632495099)).

- **Trade-off:** the injected bundle is 49 kB of Readability and Turndown, injected into
  the tab being clipped each time. Against that the worker stays 6 kB, nothing is parsed
  twice, and the alternative's only real advantage — a page that cannot interfere with
  the conversion — is not available anyway, since the isolated world's converter reads
  the same DOM either way.
- **Rejected:** an offscreen document (a fourth context, the `offscreen` permission, and
  the HTML still has to be sent to it); a DOM shim in the worker (a dependency and a
  second HTML parse, to move code away from the data it reads); extracting in the worker
  (there is no document there).

### The injected code is an IIFE bundle publishing one function, called by a second `executeScript`

`chrome.scripting.executeScript({files})` injects a *classic* script, so the file may
contain no `import` and no `export`, and a serialized `func` cannot carry 49 kB of
libraries. That fixes the file as a self-contained IIFE, which one Rollup build cannot
emit alongside the ES-module worker and pages — hence a second Vite pass writing into
the same `dist/`. The second call exists because the result of a `files` injection is
the completion value of the script, whatever shape the bundler's wrapper leaves, while
`func`'s return value is defined by the API
([#37](https://github.com/davison/md-notes/issues/37#issuecomment-5632495099)).

- **Trade-off:** two `executeScript` round trips per clip instead of one, and a global
  name in the isolated world that must match on both sides — exported from one module
  and imported by both. Re-injecting on every clip also re-evaluates the bundle, at a
  few milliseconds, and means a reloaded extension never talks to a stale copy of itself.
- **Rejected:** depending on the IIFE's completion value (works today, and is a silent
  breakage the first time the bundler wraps its output differently); `world: "MAIN"`
  (the page could then see and replace the converter); registering a content script in
  the manifest (code in every page, a permission warning, and the opposite of
  `activeTab`).

### One pending clip at a time: the context menu prepares it, the popup saves it

A context-menu click has no popup of its own, and M3-R3 requires the title to be
editable before saving. So the menu click prepares the clip and then opens the popup
where the browser allows it (Chrome 127+), and badges the toolbar icon with a blue `1`
where it does not. Session storage rather than a worker variable, because an MV3 worker
is shut down between the two acts as a matter of routine; one record rather than one per
tab, because there is one popup. A failed save *keeps* the clip, so pasting a token or
starting the daemon and pressing Save again is the whole remedy
([#37](https://github.com/davison/md-notes/issues/37#issuecomment-5632495099)).

- **Trade-off:** clipping in one tab and then clipping in another before saving loses
  the first clip. The alternative is a queue with a picker in a 22 rem popup, for a case
  the single-user premise makes rare; the pending clip is dropped when its tab navigates
  away or closes, so it does not linger.
- **Rejected:** a per-tab pending clip (the popup would have to guess which tab's clip
  to show); saving straight from the context menu (the requirement asks for an editable
  title); keeping the clip in a worker variable (lost on every worker shutdown).

### Readability keeps classes and falls back to the body; two kinds of target are dropped

`keepClasses: true`, because Readability strips every class by default and a fenced code
block's language lives in one. When Readability declines a page altogether — a
dashboard, a search result, an index — the whole `<body>` is converted instead: a messy
clip is worth more than a refusal, and the user sees it in the popup before it is saved.
In the conversion a `javascript:` link keeps its text and loses the link, and a `data:`
image is dropped entirely: a clipped note is read later in an editor, a link that runs
code has no business travelling there, and an inlined image is often megabytes of base64
counting against the daemon's 8 MiB limit
([#37](https://github.com/davison/md-notes/issues/37#issuecomment-5632495099)).

- **Trade-off:** `keepClasses` leaves the page's class attributes in Readability's
  output, so a slightly larger string crosses between the page and the worker. Dropping
  a `data:` image loses an image that was genuinely inline. Both are recoverable by hand
  in the note; the alternatives are not.
- **Rejected:** `classesToPreserve` (exact class names only, enumerated forever); keeping
  `data:` images (a clip that fails the size limit for a spacer gif); refusing a page
  Readability declines (most pages worth clipping from a browser are not articles).

### The popup takes its target tab from `?tab=`, so the real popup can be driven end to end

A browser action popup cannot be clicked from Playwright, and `chrome.action.openPopup()`
does not produce a page a test can reach in headless Chromium. The alternatives were to
leave the popup's own buttons untested end to end, or to give the popup a way to be
opened as an ordinary tab against a named page; it takes the second. Nothing is reachable
that way that clicking the toolbar button would not also reach: `popup.html` is not a
web-accessible resource, so a page cannot open it
([#37](https://github.com/davison/md-notes/issues/37#issuecomment-5632499768)).

- **Trade-off:** a test affordance in shipped code, which is a smell. Weighed against a
  popup whose only coverage would be "it renders", and a requirement — "the popup shows
  the title, editable before saving" — that is about the popup and not about the worker
  behind it.
- **Rejected:** driving the worker's message API and asserting on storage (tests the
  plumbing, not the thing the requirement describes); a separate test-only HTML page
  importing `popup.ts` (a second document to keep in step, and still not the popup);
  waiting for `chrome.action.openPopup` to become drivable.

The end-to-end test also grants the fixture site's origin in *its copy* of the manifest,
standing in for the `activeTab` grant a user gesture would give; the shipped manifest is
untouched and the test asserts what it asks for. What that does not cover is Chromium's
own `activeTab` mechanism, which is left to the operator's check and to QA.

### Over the tailnet the credential reaches the UI's own API, as an allow-list

This is the narrowing the token-scope decision on #36 left to this task. Under the
tailnet name `GET /api/roots`, the per-root reads, `PUT …/source/…` and the UI bundle
are reachable; `POST /api/roots` and `POST /api/clip` answer
`403 {"code":"loopback_only"}`, and so does anything added later. Root registration is
the step from "read my notes" to "read any file on this machine", so it stays on the
machine; the clip endpoint is refused for a weaker reason — nothing off the machine
clips, because the extension's daemon URL is `http://localhost:7337` and it runs where
the notes are. An allow-list rather than two exclusions is the point of the shape: the
next endpoint anyone adds is loopback-only until somebody has thought about it under
this heading ([#39](https://github.com/davison/md-notes/issues/39#issuecomment-5632388601)).

- **Trade-off:** clipping into the notes from a browser on another device is not
  possible, and neither is registering a folder from the laptop. Both were already
  outside the milestone; if either is wanted it is one entry in the allow-list plus a
  key to turn it on, decided with that use in front of it rather than by default.
- **Rejected:** leaving the token whole over the network (it makes the login page a
  credential for the whole filesystem, which is what #36 asked this task to look at);
  refusing only `POST /api/roots` (the same reasoning applied inconsistently, and it
  leaves the next write endpoint exposed by default); a second, narrower token for the
  tailnet (two secrets, two rotations, and a login page that has to say which one it
  wants); allowing root registration with a configured directory allow-list (a second
  confinement model beside the one `roots.Registry` already enforces).

### A session is a random id in memory, tied to the token's generation, with no logout

`__Host-mdn_session` with `Path=/`, `HttpOnly`, `Secure`, `SameSite=Strict`, no
`Domain`, and a 30-day `Max-Age`. Scoping to the host is delegated to the browser rather
than asserted by the daemon: the `__Host-` prefix makes a browser refuse the cookie
altogether unless it is `Secure`, `Path=/` and carries no `Domain`, so it is bound to
the exact name that set it and cannot later be widened. Invalidation is the token
*generation* — a counter that moves when and only when the secret does — so
`mdn token --rotate` ends every session on the next request with no expiry sweep, no
second thing on disk and nothing for `--rotate` to remember to clear. The store holds at
most 64 and evicts the one nearest expiry
([#39](https://github.com/davison/md-notes/issues/39#issuecomment-5632395808)).

- **Trade-off:** restarting the daemon logs every device out, which on a machine running
  `contrib/mdn.service` means an upgrade does. That is visible and recoverable in
  seconds; the alternative is a persisted secret whose whole purpose is to survive
  something the user did deliberately. And a session cannot be ended from the browser
  that holds it.
- **Rejected:** a signed stateless cookie (nothing to store, but nothing to revoke
  either, and it makes the token the signing key for a bearer credential that leaves the
  machine); persisting sessions beside `roots.json` (a second secret at rest for one
  restart's convenience); an idle timeout on top of the absolute one; a per-request
  rolling renewal; storing the session id rather than its SHA-256.

### The CORS settlement is not reversed for the tailnet, and a cookie gets the strict Origin check

#36 handed this task the question explicitly: reverse the settlement knowingly, or
inherit it. It is inherited. The browser on the tailnet loads the UI from the daemon
itself, so every call the UI makes is same-origin and CORS never enters it; nothing in
M3-R6 needs a *foreign* page to reach the API, and the containment argument the
token-scope decision rests on therefore still holds. What the tailnet name adds is a
second way to authenticate, and the two get different origin rules: the bearer token
keeps its exemption, while a cookie-authenticated request must carry
`https://<tailnet_host>` exactly, if it carries an `Origin` at all — `SameSite=Strict`
is one attribute enforced by the client, and the daemon should not rely on it alone to
keep a foreign page from writing to the notes. The login POST gets the same check, so a
form on another site cannot seat this browser in a session somebody else chose
([#39](https://github.com/davison/md-notes/issues/39#issuecomment-5632396022)).

- **Trade-off:** a page on the tailnet that is *not* served by the daemon — some other
  tool on another node — cannot call the API from page context and will fail at the
  preflight with the browser's opaque CORS error. Same boundary, same poor diagnostic,
  now stated for the network case too.
- **Rejected:** answering the preflight for the tailnet origin (it would widen the write
  path to any page that obtains the token, and re-open the question #36 closed, for no
  caller that exists); exempting cookie-authenticated requests from the Origin check on
  the strength of `SameSite=Strict`; accepting `http://<tailnet_host>` as an origin (the
  cookie is `Secure` and would not be sent over it anyway, so accepting it would only
  mask a broken proxy).

### Failed logins are bounded, and the bound is about work and log volume

Three failures a minute from one caller are answered at once, the next few wait half a
second, past a dozen the daemon answers `429 {"code":"too_many_attempts"}` with
`Retry-After` until the window rolls, and a correct token clears the count. The limit is
not what stands between anyone and the notes — the token is 130 bits from
`crypto/rand`, and no rate limit changes that arithmetic. What is unbounded without one
is the work and the log volume a single caller on the tailnet can cause
([#39](https://github.com/davison/md-notes/issues/39#issuecomment-5632962985)).

- **Trade-off:** a hostile node can still make the daemon do a bounded amount of work
  and fill a minute's worth of log lines. In exchange there is no lockout state to get
  stuck in, no persistence, and no way for one caller to lock another out.
- **Rejected:** no limit at all with a sentence in the docs (the reviewer's own
  position, defensible on the arithmetic, but the log volume is a real cost and the code
  is thirty lines); a global limit across all callers (one hostile node could then lock
  the operator out of their own notes); a lockout that outlives the window or persists;
  keying on the peer address alone (under `tailscale serve` it is always the proxy, so
  every device would share one budget).

The decision as first written claimed a property the code did not have, and the
correction that followed is the sharpest record item of the milestone — see
[Corrections](#corrections-to-the-record-itself).

### How the milestone was run

Two decisions belong to the coordinator rather than to any task, and both were taken
because the operator was away.

**Scope and defaults**
([#35](https://github.com/davison/md-notes/issues/35#issuecomment-5621679656)). The
requirements fix the defaults a human would otherwise have been asked for: `clips/`
inside the notes root, `YYYY-MM-DD-<title-slug>.md` with a numeric suffix on collision,
the four frontmatter keys, one bearer token per installation presented in an
`Authorization` header and exchanged for a host-scoped cookie on the tailnet.

- **Trade-off:** fixing defaults in the requirements lets the seats build without a
  human gate while the operator is away, at the risk of one of them being wrong; each is
  a config key or a naming rule, cheap to change in a follow-up. Including
  [#10](https://github.com/davison/md-notes/issues/10) grows the milestone by one task
  but designs authentication once, as the capture asked.
- **Rejected:** deferring the token to keep the milestone smaller (the extension cannot
  write without it, and #10 would then need a second auth design); a polish milestone
  first (the operator chose functionality).

**Standing merge confirmation**
([#35](https://github.com/davison/md-notes/issues/35#issuecomment-5632220363)).

- **Trade-off:** delivery continues across the operator's absence; the operator gives up
  the per-PR look before merge, relying on the model review and QA gates, and on the
  record to catch up afterwards. Human gates are unaffected and still block.
- **Rejected:** holding every approved PR for an explicit confirmation, which stalled
  the milestone for a day at a time.

## Deviations

### Rotation had to revoke, not merely admit

The recorded mechanism — "a presented token that does not match is compared once more
against the file" — was wrong, and verifying against the built binary caught it.
Re-reading only on a *mismatch* admits the new token but never withdraws the old one:
the daemon's cached value still matches the old token, so it is served, and nothing ever
triggers the re-read. Rotating and then presenting the replaced token returned `200`.
M3-R1 requires a revoked token to be a `401`, so a rotation that does not revoke is not
a rotation. The mechanism became "the file is stat-ed on every authenticated request and
re-read when it is no longer the file the held value came from"
([#36](https://github.com/davison/md-notes/issues/36#issuecomment-5621928369), fixed in
[`bf61e79`](https://github.com/davison/md-notes/commit/bf61e79)).

The unit test that first covered this presented the new token *before* the old one and
so passed over the hole; the test added with the fix presents only the old one. The cost
changes from no I/O on the hot path to one `stat` per request carrying an
`Authorization` header — the web UI over loopback sends none, so ordinary browsing is
untouched.

### The file-URL intercept accepts a query string and a fragment, and refuses a backslash

The plan said the intercept condition included "no query"; the code drops the query and
the fragment instead of refusing, because neither is part of a local file's identity and
a `.md` link with `#section` is ordinary. It was written and tested that way from the
first commit and the plan was not updated. Separately, a decoded path segment containing
a backslash is refused, so a file whose name really contains one is left alone: the
derived directory is what `POST /api/roots` is asked to serve, and the segment rules
that keep a crafted URL out of that are worth more than an exotic but legal POSIX
filename ([#38](https://github.com/davison/md-notes/issues/38#issuecomment-5622314618)).
A third, smaller one: `isMarkdownPath` refuses a file whose whole base name is the
suffix — `.md` and `.markdown` are dotfiles the navigator would not list either
([#41](https://github.com/davison/md-notes/pull/41#issuecomment-5621976721)).

### #37 repaired #38's end-to-end suite

`extension/e2e/file-url.test.mjs` had gone stale against the merged token daemon in two
ways: it ran `mdn serve` with no `--token-file`, so the suite read and created the
*operator's real* token file rather than one in its temporary directory, and its "leaves
a file outside every root alone" case inherited a deliberately wrong token from the test
above it, which #36's daemon answers `401 unauthorized` where the old one answered
`403 cross_origin`. Both are one-line corrections in a suite the new one sits beside,
and leaving a red e2e for QA to rediscover would have been worse
([#37](https://github.com/davison/md-notes/issues/37#issuecomment-5632499768)). No
behaviour of the extension changed. Each later review checked
`~/.local/state/mdn/token` by checksum before and after its run.

Task [#39](https://github.com/davison/md-notes/issues/39) recorded no deviations from
its plan ([#44](https://github.com/davison/md-notes/pull/44)).

## Corrections to the record itself

Four times a review finding was not about the code but about what the record said the
code did. In three of the four the code was right and the words were wrong; in the
fourth the words described a protection the code did not have. QA added a fifth, below
them.

- **Who CORS actually governs.** The CORS settlement, and the Authentication section of
  `docs/introduction.md` that stated it, said the token's Origin exemption was usable
  only by an MV3 extension **service worker**, and that a popup or an options page would
  fail at the preflight. Exemption in an extension follows `host_permissions` and covers
  **every extension context**, its pages included; what CORS governs is ordinary web
  pages and, since Chrome 73, content scripts. The repository already depended on the
  true version — the options page's **Test connection** is exactly such a call — and the
  review of [#43](https://github.com/davison/md-notes/pull/43#issuecomment-5632642999)
  demonstrated it, issuing a `POST /api/clip` from `options.html` in the shipped build
  and watching the note land on disk. Corrected on
  [#36](https://github.com/davison/md-notes/issues/36#issuecomment-5632737929), and
  [amended](https://github.com/davison/md-notes/issues/36#issuecomment-5632949057) when
  round two found the same phrase in a second place the first correction had not named.
  The daemon's behaviour, `TestGuardRefusesThePreflight` and the containment argument
  are all untouched: that argument rests on a hostile *web page*, which is exactly the
  caller CORS does govern. What changed is the set of callers described as able to
  present the token — and the reason clipping belongs in the service worker, which is
  that the worker outlives the popup, not that a popup could not make the call
  ([`6bcf7e6`](https://github.com/davison/md-notes/commit/6bcf7e6) for the extension's
  copies, [`7d5921a`](https://github.com/davison/md-notes/commit/7d5921a) and
  [`b6b0866`](https://github.com/davison/md-notes/commit/b6b0866) for the daemon's).
  Routing it was deliberate: #43 fixed what it owned and recorded the rest rather than
  editing another task's document, and told
  [#39](https://github.com/davison/md-notes/issues/39#issuecomment-5632951469) directly,
  since #39 held that file on a branch at the time.
- **A throttle that claimed a floor it did not have.** The login-throttle decision
  accepted that its `X-Forwarded-For` key is spoofable and rested on one sentence: "the
  half-second delay — which applies whatever the key says — is the part that bounds a
  single connection's rate." It did not. Both tiers were keyed, so a caller varying the
  header reached neither, and round two of the review of
  [#44](https://github.com/davison/md-notes/pull/44#issuecomment-5633075275) measured
  it: twenty wrong logins with a varying address completed in **93 ms** with twenty
  refusal lines in the log, against 4,674 ms and a `429` for the same twenty with a
  constant one. The work and the log volume the decision exists to bound were not
  bounded. The throttle now also counts failures across every key, so once the daemon has
  seen more than the free tier's worth in the window in total, every further failure
  waits whatever key it claims; the total only ever delays and never refuses, because a
  refusal counted across all callers would hand anyone the ACL admits the ability to
  lock the operator out — which is what the original decision rejected a global limit
  for. The same twenty attempts then took **8,778 ms**, and the correct token still
  logged in in 5 ms in the middle of them
  ([#39](https://github.com/davison/md-notes/issues/39#issuecomment-5633145581),
  [`781237f`](https://github.com/davison/md-notes/commit/781237f)). The reviewer's
  position in both rounds was that no limit at all would have been defensible; what was
  not defensible was a decision comment claiming a bound the code did not enforce.
- **A decision that was said to be recorded and was not.** The reply to round one of
  [#42](https://github.com/davison/md-notes/pull/42) said the CORS settlement had been
  "recorded as a decision … on #36". It had not: the comment it pointed at was the
  *token-scope* decision, which mentions the preflight in its reasoning but whose
  rejected alternatives are about narrowing the token. Round two
  ([#42](https://github.com/davison/md-notes/pull/42#issuecomment-5622337215)) blocked on
  exactly that, and on the point that made it matter — the token-scope decision's
  containment argument *depends* on the preflight refusal, so with the settlement
  unrecorded, #39 could have reversed the boundary without ever meeting the reasoning it
  would be invalidating. The settlement was then recorded with that coupling stated in
  both directions
  ([#36](https://github.com/davison/md-notes/issues/36#issuecomment-5622353217)). This
  milestone's record is the thing that would have lost it.
- **A comment that claimed a cleanup the code did not do, twice.** The file-URL worker's
  comment said a tab moving on to another page had its badge *and* its record cleared.
  Round one of [#41](https://github.com/davison/md-notes/pull/41#issuecomment-5622167896)
  found the record survived; the fix made the comment a stronger claim, and round two
  ([#41](https://github.com/davison/md-notes/pull/41#issuecomment-5622474315)) measured
  that it was still false in the case that matters — `tab.url` is undefined for a tab on
  an origin the extension has no permission for, which is every ordinary site, so the
  cleanup ran only in a configuration no real user has. The third attempt used the
  absence of the URL as the signal, and pinned it with a test that fails without the
  branch. A third of the same kind, on the same PR family: the highlight rule's comment
  claimed to make "the test the GFM plugin's own rule makes", which it deliberately does
  not — the rule is looser on two counts, and looser is right
  ([#43](https://github.com/davison/md-notes/pull/43#issuecomment-5633053331), fixed as
  a comment in [`1e45740`](https://github.com/davison/md-notes/commit/1e45740)).

A fifth, found by QA rather than by a review, is a claim in a **merged** PR
description. [#43](https://github.com/davison/md-notes/pull/43)'s body lists task lists
among what the conversion keeps. That is true of a selection clip and false of a page
clip: Readability's sanitiser removes `input` elements before Turndown sees them, so a
task list inside an article comes through as ordinary bullets, while a selection over
the same markup gives `- [x]`. Reproduced against the built extension from this branch,
both ways in one run. The body is the merge commit's message and cannot be edited, and
the disposition on
[#35](https://github.com/davison/md-notes/issues/35#issuecomment-5633500584) says the
correction belongs here rather than in a rewritten history; the user-facing half is one
sentence in [the extension page](../extension.md#what-ends-up-in-the-note), which said
the same thing without the qualification.

One citation in the record is itself wrong, and is left as it stands rather than edited:
the tailnet CORS decision
([#39](https://github.com/davison/md-notes/issues/39#issuecomment-5632396022)) cites the
#36 settlement as comment `5622277793`, which does not exist; the settlement is
[5622353217](https://github.com/davison/md-notes/issues/36#issuecomment-5622353217). The
anchor is dead, but the issue URL around it resolves, so
`gh codecrew milestone evidence` cannot see it — which is worth knowing about what that
check does and does not prove.

**A note on commit SHAs.** Every SHA quoted in a review comment or a reply on
[#41](https://github.com/davison/md-notes/pull/41)–[#44](https://github.com/davison/md-notes/pull/44)
is the one it had on the task branch, which the rebase merge rewrote. This document
quotes the SHAs on `main`. The mapping, for anyone following the comments into the
history:

| On the branch | On `main` | What it was |
|---------------|-----------|-------------|
| `9b5a078` | [`bf61e79`](https://github.com/davison/md-notes/commit/bf61e79) | rotation revokes the replaced token |
| `b08c90e` | [`42e798a`](https://github.com/davison/md-notes/commit/42e798a) | the frontmatter title bounded by runes |
| `ef51af0` | [`21914a4`](https://github.com/davison/md-notes/commit/21914a4) | the README's authentication paragraph |
| `5d47d1d` | [`3fc1c5b`](https://github.com/davison/md-notes/commit/3fc1c5b) | a code on every guard refusal, and the CORS boundary |
| `d66f9da` | [`9d2979c`](https://github.com/davison/md-notes/commit/9d2979c) | the symlinked token file, the load race, the live repair |
| `3cd98ce` | [`34700c0`](https://github.com/davison/md-notes/commit/34700c0) | chmod the verified descriptor; name the symlink refusal |
| `e61127f` | [`0d8f74c`](https://github.com/davison/md-notes/commit/0d8f74c) | the encoded-separator traversal |
| `9535b38` | [`8463fd6`](https://github.com/davison/md-notes/commit/8463fd6) | the reload retry |
| `bfcf471` | [`77c8625`](https://github.com/davison/md-notes/commit/77c8625) | "Test connection" judges the token |
| `67983e5` | [`a0fe52f`](https://github.com/davison/md-notes/commit/a0fe52f) | the documentation findings, and the packer in `make check` |
| `6fbf1b8` | [`512bd4a`](https://github.com/davison/md-notes/commit/512bd4a) | the navigate-away cleanup |
| `2240db9` | [`a031d0e`](https://github.com/davison/md-notes/commit/a031d0e) | relative URLs against `document.baseURI` |
| `8e936c4` | [`f93ae31`](https://github.com/davison/md-notes/commit/f93ae31) | fenced code left as the page wrote it |
| `430c3f9` | [`ac17f81`](https://github.com/davison/md-notes/commit/ac17f81) | three conversion defects |
| `7031f46` | [`6bcf7e6`](https://github.com/davison/md-notes/commit/6bcf7e6) | why the clip call is the worker's |
| `19c9155` | [`dedb5ff`](https://github.com/davison/md-notes/commit/dedb5ff) | fences Turndown indented or quoted |
| `c1d1ad7` | [`0006907`](https://github.com/davison/md-notes/commit/0006907) | GitHub's real highlight markup |
| `2bb4a7d` | [`4c95df8`](https://github.com/davison/md-notes/commit/4c95df8) | only the `Host` decides which rule applies |
| `8ec425a` | [`2fc938e`](https://github.com/davison/md-notes/commit/2fc938e) | one read for the answer and the generation |
| `ef321d0` | [`0b7abdd`](https://github.com/davison/md-notes/commit/0b7abdd) | the login throttle |
| `398f543` | [`5e3d57d`](https://github.com/davison/md-notes/commit/5e3d57d) | what counts as a proxy announcing itself |
| `9e39ad2` | [`781237f`](https://github.com/davison/md-notes/commit/781237f) | the floor under the throttle |
| `5a9037b` | [`b6b0866`](https://github.com/davison/md-notes/commit/b6b0866) | the #36 amendment in the Confinement list |

## What the reviews and QA changed

All four PRs went through at least two rounds; three went through three. The reviewer
seat is routed to the same identity as the author (pure solo tier), so each review is a
comment rather than a formal approval, and the operator confirmed each merge — under the
standing confirmation from
[#35](https://github.com/davison/md-notes/issues/35#issuecomment-5632220363) for the
later ones.

The habit the earlier milestones established held, and hardened: every review executed
the code rather than reading it, and the findings that mattered most were the ones a
reproduction produced. Each of the blocking findings below was demonstrated against a
running binary or a real browser before it was written down — a crafted `file:` URL
registering `/etc` as a root, a `POST /api/clip` issued from the options page, twenty
timed logins, raw bytes down a socket — and several of them are things the author could
not have found by reading their own diff.

- **Token and clip endpoint
  ([#42](https://github.com/davison/md-notes/pull/42#issuecomment-5622127832)):** round
  one found nine, two of them blocking. A title over 300 bytes made of multibyte
  characters was cut mid-rune and landed in the frontmatter as a base64 `!!binary` blob
  — which defeats exactly the mitigation the slug decision rests on, since for a long
  CJK title the note gets `untitled` *and* an unreadable title. The README still said
  the daemon "has no authentication of its own", a sentence this PR is what made false.
  The other seven — the unusable Origin bypass under CORS, an error table that did not
  describe what a token-less extension request actually gets, a token file read through
  a symlink and chmodded through it, a rotation landing between load and stamp, a
  permission repair that never ran while the daemon did — were all fixed in the same PR
  rather than deferred. Round two
  ([#42](https://github.com/davison/md-notes/pull/42#issuecomment-5622337215)) verified
  all nine and then blocked on two *record* items: a decision the reply said had been
  recorded and had not, and a PR body four commits stale. Round three
  ([#42](https://github.com/davison/md-notes/pull/42#issuecomment-5622485105)) approved,
  having re-run every reproduction and confirmed each new test fails against the old
  code.
- **The extension's scaffold and file URLs
  ([#41](https://github.com/davison/md-notes/pull/41#issuecomment-5622167896)):** round
  one found twelve, two blocking, and the first is the milestone's sharpest security
  finding. `%2F` is not normalised by `new URL()`, and decoding the pathname turned it
  back into a separator, so a crafted `file:` URL could make the extension POST
  `/tmp/probe/nowhere/../../../../../../etc` to `POST /api/roots` — which the daemon
  cleans to `/etc`, registers and serves. The daemon's guarantee is that it never serves
  a path outside a *registered* root; this made the registration itself attacker-chosen.
  The fix decodes one segment at a time and refuses a decoded segment that is empty, `.`,
  `..`, or that contains a separator or a NUL, with the assembled path compared against
  its own normal form as a backstop. The second blocking finding was that a reload never
  re-triggered the intercept, so every remedy the documentation offered — start the
  daemon, fix the URL, paste the token — was unreachable in the tab that had failed. Of
  the rest, two were documentation claiming a command that did not exist yet
  (`mdn token`, which #36 had not merged), one was a "Test connection" that reported
  success for a wrong token, and one was a README heading that had captured the daemon's
  security paragraph into the extension section. Round two
  ([#41](https://github.com/davison/md-notes/pull/41#issuecomment-5622474315)) re-ran
  round one's exploit with eleven variants of its own, approved, and added three small
  findings, all taken.
- **Clipping ([#43](https://github.com/davison/md-notes/pull/43#issuecomment-5632642999)):**
  three rounds, and the two blocking findings in round one are both silent corruption of
  a reader's content. A `<base href>` was ignored, so *selection* clips resolved relative
  links against the wrong base and produced absolute URLs that point at pages which do
  not exist — the note looks right. And the conversion's final tidies ran over the whole
  document, fences included, so a clipped Python article silently lost PEP 8's two blank
  lines and any trailing whitespace inside its code. Round two
  ([#43](https://github.com/davison/md-notes/pull/43#issuecomment-5632892479)) found the
  fence fix incomplete in the shape that matters most — a fence Turndown has *indented*
  inside a list item or prefixed inside a blockquote was still not recognised, which is
  the dominant shape of a tutorial article — and the third attempt tracks the opening
  fence's blockquote depth rather than its exact prefix, because Turndown opens such a
  fence on the list marker's line and closes it on an indented one. Round three
  ([#43](https://github.com/davison/md-notes/pull/43#issuecomment-5633053331)) approved
  after 27 probes comparing the tidied output against Turndown's own, and left two items
  as a comment and a capture. The same review chain produced the CORS correction above,
  and two backlog captures.
- **Tailnet access ([#44](https://github.com/davison/md-notes/pull/44#issuecomment-5632825891)):**
  round one blocked on the boundary itself. The guard switched on `Host` alone, so a
  request the daemon saw with a loopback `Host` took the loopback rule and was served
  with no credential — reachable two ways: a proxy that rewrites `Host` (nginx's bare
  `proxy_pass` does), and an absolute-form request target, which Go uses to fill
  `r.Host`, demonstrated down a raw socket. Neither is reachable through
  `tailscale serve`, which passes the `Host` through; the point was that the entire
  authentication boundary rested on an internal property of another program that nothing
  here documented, tested or corroborated. Both halves were closed in the guard —
  absolute-form targets refused outright, and a loopback `Host` carrying proxy markers
  refused when a tailnet name is configured — with the demonstrations as tests, and the
  prose says plainly that whatever terminates TLS in front must pass the original `Host`
  through unchanged. Round two
  ([#44](https://github.com/davison/md-notes/pull/44#issuecomment-5633075275)) approved
  and found three follow-ups, of which one is the throttle correction above and one was
  that the marker list did not include the RFC 7239 `Forwarded` header, `Via` or
  `X-Real-IP` — so the backstop did not fire for several proxies that announce
  themselves, and the prose read as a guarantee while sitting under the one example it
  cannot cover. Round three
  ([#44](https://github.com/davison/md-notes/pull/44#issuecomment-5633227788)) approved
  and left one capture.

Two findings changed the shape of the milestone rather than a line of code. Review
finding 8 on [#42](https://github.com/davison/md-notes/pull/42#issuecomment-5622127832)
— register any directory with the token, then read every file under it — became the
token-scope decision, which handed #39 a stated question, which became the allow-list
that is now the tailnet's authorisation model. And round two of
[#42](https://github.com/davison/md-notes/pull/42#issuecomment-5622337215) blocking on a
missing decision comment is what put the CORS settlement on the record at all; without
it this document would not have found it, which is what the reviewer said at the time.

Independent QA then went at the merged whole
([#35](https://github.com/davison/md-notes/issues/35#issuecomment-5633486688)), and
found one real defect the four review loops and every suite had missed: **a `file:`
URL naming a markdown file that does not exist still registers its directory as a
permanent root**. Navigating to `file:///etc/no-such-note.md` registered `/etc`, wrote
it to the state file, and left every markdown file under it readable *and writable*
through the loopback editor, with no unregister endpoint to take it back. The gap is
an unstated precondition rather than a contradiction of M3-R4, which is why the verdict
holds — and QA bounded it carefully before recommending a capture: Chromium refuses a
web page's navigation to a `file:` URL, so the trigger is the user or a link in a page
already opened over `file://`. It is invisible to the suites on both sides of the seam,
because `open-file.test.ts` mocks the roots API and the end-to-end suite covers only
the already-registered path. Filed as [#50](https://github.com/davison/md-notes/issues/50).

Three of its other findings are conversion edges, invisible for one reason — no
fixture contained the shape. Readability lifts the article's own `<h1>` out of a page
clip, and strips task-list checkboxes from a page clip, which a *selection* clip keeps;
both are new. The third, a table with no header row surviving as raw `<table>` HTML, is
[#45](https://github.com/davison/md-notes/issues/45) found again from the outside,
which is some evidence that the capture is worth acting on. All three reproduce against
the built extension. QA named the suite gaps behind them in as many words: no test
contains a `<table>` without a `<thead>`, a task list inside an article, or an `<h1>`
inside a selection — and, separately, the register-a-new-root half of M3-R4, the half
that writes, has no end-to-end coverage at all, which is the gap finding 1 lives in.
None of them was judged a reason to hold anything.

QA also found the one behaviour this milestone's code has and its documentation did not
describe — the throttle's total floor — and the boundary statements this task then
fixed; both are folded into this task, and
[Requirement outcomes](#requirement-outcomes) says what each became.

## Known gaps at the boundary

Milestone one's tailnet capture ([#10](https://github.com/davison/md-notes/issues/10))
is closed by [#39](https://github.com/davison/md-notes/issues/39). What remains true and
will surprise someone who has not read this far:

| Gap | Where it is recorded |
|-----|----------------------|
| On loopback the token grants the **whole** API to whoever presents it, not the clip endpoint alone: a holder can register any directory as a root and read every file under it. That is by design, and contained by the premise and by the refused preflight | [#36](https://github.com/davison/md-notes/issues/36#issuecomment-5622222887) |
| Under `tailnet_host` the premise stretches from "processes running as you" to "whoever the tailnet ACL admits to this node", and one of them holding the token can read, search and edit every root the daemon serves — including folders added with `mdn open` that were only ever meant to be looked at locally | [the introduction](../introduction.md#the-premise-restated) |
| The throttle's floor delays each failed login but serialises nothing, so 40 parallel wrong logins complete in about half a second and write 40 log lines. The comment claims more than that | [#48](https://github.com/davison/md-notes/issues/48) |
| The throttle's per-caller key comes from `X-Forwarded-For`, which a client behind a proxy that appends can write. Accepted rather than solved: the peer address is always the proxy under `tailscale serve`, so keying on it would throttle every device together | [#39](https://github.com/davison/md-notes/issues/39#issuecomment-5632962985) |
| The proxy-marker backstop cannot catch a bare `proxy_pass`, which rewrites the `Host` and announces itself with none of the six headers. The documented requirement that the proxy preserve the `Host` is the defence; the backstop is not a substitute | [#44](https://github.com/davison/md-notes/pull/44#issuecomment-5633227788) |
| Sessions live in memory, so restarting the daemon logs every device out — on a machine running `contrib/mdn.service`, an upgrade does. There is no logout: rotation is the revocation | [#39](https://github.com/davison/md-notes/issues/39#issuecomment-5632395808) |
| A request target in absolute form is now refused on loopback too, which is the one respect in which loopback behaviour changed. Nothing but a raw socket or a forward-proxy client produces one | [#44](https://github.com/davison/md-notes/pull/44#issuecomment-5633075275) |
| A page title written entirely in a non-Latin script yields `YYYY-MM-DD-untitled.md`, `-2`, `-3` …; the title itself is in the frontmatter | [#36](https://github.com/davison/md-notes/issues/36#issuecomment-5621889774) |
| There is one pending clip at a time: clipping in one tab and then in another before saving loses the first | [#37](https://github.com/davison/md-notes/issues/37#issuecomment-5632495099) |
| A `data:` image is dropped from a clip and a `javascript:` link keeps its text but loses the link | [#37](https://github.com/davison/md-notes/issues/37#issuecomment-5632495099) |
| A `file:` URL naming a markdown file that **does not exist** still registers its directory as a permanent root, which then serves every markdown file under it to the loopback editor. There is no unregister endpoint; the state file is edited by hand | [#50](https://github.com/davison/md-notes/issues/50) |
| Readability lifts the article's own `<h1>` out of a page clip — it survives as the frontmatter title and as the app's heading — and strips task-list checkboxes, which a selection clip keeps | [#35](https://github.com/davison/md-notes/issues/35#issuecomment-5633486688), findings 2 and 3 |
| A clipped table with no header row survives as raw `<table>` HTML, because the GFM plugin declines it and Turndown keeps what it cannot convert | [#45](https://github.com/davison/md-notes/issues/45) |
| A `clips_dir` naming an existing regular file is accepted at startup and fails on the first clip with `500 {"code":"io_error"}`. Nothing is overwritten, and the escape cases are still caught at startup | [#35](https://github.com/davison/md-notes/issues/35#issuecomment-5633486688), finding 7 |
| Content after the `pre` inside a matched `div.highlight` — a caption, a filename strip — is dropped along with the copy button | [#47](https://github.com/davison/md-notes/issues/47) |
| A `file:` URL the path mapping refuses is left alone silently, with no badge and no popup message, like a URL that is not markdown at all | [the extension page](../extension.md#opening-a-local-markdown-file) |
| The mapping assumes POSIX paths, so a `file:///C:/…` URL maps to a path the daemon simply will not find | [#41](https://github.com/davison/md-notes/pull/41#issuecomment-5622167896) |
| Pointing the extension's daemon URL at a `tailnet_host` name leaves it with nothing that works: clipping and registering a folder are refused `loopback_only` by design, and opening a local file fails as well, because the roots listing the intercept depends on is sent without the token — which loopback allows and that name does not, so the badge reports a rejected token that is in fact correct | [#51](https://github.com/davison/md-notes/issues/51) |
| The popup carries a `?tab=` seam that exists for the end-to-end tests. It reaches nothing the toolbar button does not, and `popup.html` is not web-accessible | [#37](https://github.com/davison/md-notes/issues/37#issuecomment-5632499768) |
| Chromium's own `activeTab` grant and the `chrome.permissions.request` prompt are asserted rather than executed: no harness can perform the gesture, and headless Chromium blocks on the prompt | [#37](https://github.com/davison/md-notes/issues/37#issuecomment-5632499768), [#41](https://github.com/davison/md-notes/pull/41#issuecomment-5622167896) |
| `TestBurstIsOneBatch` in `internal/watch` fails intermittently in CI on filesystem timing, in code this milestone did not touch | [#46](https://github.com/davison/md-notes/issues/46) |
| The extension is loaded unpacked from this repository and is in no store; the inbox that turns URLs shared from a phone into clips is not built | [the extension page](../extension.md#the-daemon-url-and-the-token) |

Milestone two's own boundary notes still stand, minus the one this milestone
discharged: the `localStorage` draft mirror is one record per note shared by every tab,
a directory whose only files are *ignored* is still not watched, the editor cannot
create, rename or delete a note, and the UI bundle is served uncompressed and
uncacheable ([the M2 record](2-editor-autosave-and-live-update.md#known-gaps-at-the-boundary)).
The five M2 follow-up captures — [#30](https://github.com/davison/md-notes/issues/30),
[#31](https://github.com/davison/md-notes/issues/31),
[#32](https://github.com/davison/md-notes/issues/32),
[#33](https://github.com/davison/md-notes/issues/33) and
[#34](https://github.com/davison/md-notes/issues/34) — are still open, deferred by the
scope decision to a later polish milestone, and this milestone added five of its own —
[#45](https://github.com/davison/md-notes/issues/45),
[#46](https://github.com/davison/md-notes/issues/46),
[#47](https://github.com/davison/md-notes/issues/47) and
[#48](https://github.com/davison/md-notes/issues/48) from the review trail, and
[#50](https://github.com/davison/md-notes/issues/50) from QA.

## Where the record is silent

- **Readability and Turndown were named by the requirement, not chosen in the open.**
  M3-R3 says "extracts the readable article with Readability and converts it to
  GitHub-flavoured markdown with Turndown", so the two libraries at the heart of the
  clipper arrived as a requirement and no comment anywhere weighs them against an
  alternative. What *is* recorded is everything downstream of that choice — where they
  run, how they are bundled, which of their defaults are overridden and why
  ([#37](https://github.com/davison/md-notes/issues/37#issuecomment-5632495099)). This
  is the same silence milestone one recorded about chroma and `yaml.v3`, and milestone
  two about the vim implementation: the framework is named by the milestone, and the
  reason it was the right framework is nowhere.
- **Nothing says why the extension is Chromium-only.** The goal names Brave and the
  manifest is MV3; Firefox is not mentioned anywhere in the milestone, the tasks or the
  reviews — not as a rejected alternative, not as a deferral. It is almost certainly the
  operator's own browser deciding it, but the record does not say so.
- **The token's own shape was never argued.** Its length and alphabet come from
  `rand.Text()`, and the only place its strength is discussed at all is as an aside in
  the throttle decision ("130 bits from `crypto/rand`")
  ([#39](https://github.com/davison/md-notes/issues/39#issuecomment-5632962985)). No
  decision records the choice, and nothing says whether a user-visible format — grouped,
  prefixed, checksummed — was considered for something a person has to copy between a
  terminal and a browser.
- **The 30-day session and the 64-session cap are stated, not argued.** The session
  decision explains the mechanism and the trade-offs of holding sessions in memory in
  detail, and gives both numbers as given
  ([#39](https://github.com/davison/md-notes/issues/39#issuecomment-5632395808)).
- **No requirement asked where the seam between the daemon and the extension is tested.**
  Milestone one decided that tests ride each PR rather than forming a requirement of
  their own, and milestone two recorded that its blocking QA finding lived exactly in the
  composition of two well-tested halves. This milestone has end-to-end suites that do
  span the seam — they drive the built extension in headless Chromium against the built
  daemon — but they exist because two implementers chose to write them, not because
  anything asked for them, and they are deliberately outside `make check` because they
  need a Chromium binary. The record still does not say where such a test belongs.
- **The `?tab=` seam's cost was weighed once and never revisited.** It is argued
  carefully as a decision
  ([#37](https://github.com/davison/md-notes/issues/37#issuecomment-5632499768)), and no
  review returned to it; whether a test affordance in shipped code is acceptable in this
  project is therefore settled by one task's judgement rather than by a policy.
- **The operator saw none of this before it merged.** The standing confirmation
  ([#35](https://github.com/davison/md-notes/issues/35#issuecomment-5632220363)) is
  recorded honestly, including what it gives up, and the model reviews were unusually
  thorough — but every trade-off in this document was struck between an implementer and
  a reviewer sharing one identity, and the human whose notes these are has not yet
  loaded the extension into their own browser. The record cannot say what that check
  will find.
