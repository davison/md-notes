# M13 — A home-screen shortcut that stays logged in

Tracking issue: [#239](https://github.com/davison/md-notes/issues/239). Its one implementation task,
[#240](https://github.com/davison/md-notes/issues/240), is merged on `main` at
[`8a27726`](https://github.com/davison/md-notes/commit/8a27726). The milestone was opened at
14:36:29Z on 2026-09-30, and the task merged at 15:14:02Z the same day. The task branch was cut from
[`8385614`](https://github.com/davison/md-notes/commit/8385614), the tree the plan's GET audit read
([#240](https://github.com/davison/md-notes/issues/240)).

Independent QA graded M13-R1 to M13-R3 on `main` at `8a27726`, and all three were satisfied
([#239](https://github.com/davison/md-notes/issues/239#issuecomment-5914576618)). M13-R4 was
untestable, because it includes the roadmap row and this record, which did not yet exist. The task's
share of M13-R4 was checked against QA's own measurements and found accurate. The pull request that
carries this record delivers the rest, and a superseding verdict is owed after it merges.

The milestone is to be released as v0.3.2. At the time of writing the tag does not exist; see [The
release](#the-release-v032-to-be-tagged).

## Goal and outcome

The milestone's goal, as #239 states it: a tailnet session cookie that a browser sends when md-notes
is opened from outside the site — a home-screen shortcut, a link from another app — so the e-ink
tablet stops asking for the token; with the reason for every refused tailnet request logged;
released as v0.3.2 ([#239](https://github.com/davison/md-notes/issues/239)).

It comes from one capture, [#238](https://github.com/davison/md-notes/issues/238), raised by the
coordinator at 12:20:16Z from the operator's own device check after installing v0.3.1: the check M12
left to him. On his Boox Note Air 3, whose NeoBrowser is Chromium-based, the home-screen shortcut
asked for the token on every open, while his phone stayed logged in. His reports, as the capture
quotes them, relayed from the coordinator session: "phone stays logged in, e-ink asks for token
every time", then "NeoBrowser, home-screen shortcut, asks on each new open, not on a same tab
reload", then, after typing the URL into NeoBrowser itself, "it's (a). Opens in the browser and asks
me to install the shortcut", meaning still logged in. The daemon log showed the tablet logging in at
13:11:56 and again at 13:13:32 with no restart between. The capture's diagnosis: the cookie was
stored and valid, but `SameSite=Strict` held it back on a navigation that starts outside the site,
and a shortcut's launch is one. It was not the M12 session work.

What a reader has now:

- **A home-screen shortcut, or a link from another app, opens the notes logged in.** The session
  cookie is `SameSite=Lax`.
- **Another site can start nothing that writes, and cannot read what it starts.** A cross-site form
  post, `fetch`, `iframe`, subresource or events stream arrives without the cookie. A
  cookie-authenticated request with a foreign `Origin` is still refused. Every request the daemon
  answers on `GET` is a read.
- **Every other protection is as it was in v0.3.1:** the `__Host-` prefix, `HttpOnly`, `Secure`,
  `Path=/`, no `Domain`, no token in the cookie, the 30-day idle limit, rotation ending every
  session, the login throttle and loopback.
- **Every tailnet request sent to the login page or refused is logged with its reason,** and every
  login with a coarse browser family. No secret reaches the log, a caller cannot forge a line, and
  the log takes at most thirty refusal lines a minute.
- **A device that opens the notes from a shortcut is asked for the token once after the upgrade to
  v0.3.2,** because it still holds a `Strict` cookie, and then stays logged in. A v0.3.1 cookie is
  still a good session, so nothing else logs in again.

### What shipped

| Task | Requirements | Adopts | Pull request | Approved at | Last head | On `main` as | Merged |
|------|--------------|--------|--------------|-------------|-----------|--------------|--------|
| [#240](https://github.com/davison/md-notes/issues/240) `SameSite=Lax` for the tailnet session, and log why a tailnet request was refused | M13-R1 to R3, M13-R4 (part) | #238 | [PR #241](https://github.com/davison/md-notes/pull/241), three rounds | `950f7ce` | `950f7ce` | [`6c22f91`](https://github.com/davison/md-notes/commit/6c22f91) to [`8a27726`](https://github.com/davison/md-notes/commit/8a27726), ten commits | 15:14:02Z |

The pull request landed by rebase, so the commits on `main` differ from the head the last review
saw, but the approval covered the last head: no commit came after it.

The commits are `fix`, `test` and `docs`, and none is `feat`: #240's plan lists `fix(server)`,
`test(e2e)`, `docs`, and `fix(man)` if the man page changed, and says "No `feat`"
([#240](https://github.com/davison/md-notes/issues/240)), and the release Decision calls the change
a fix ([#239](https://github.com/davison/md-notes/issues/239#issuecomment-5913477367)). Round one of
the review checked each commit's type against its diff ([PR
#241](https://github.com/davison/md-notes/pull/241#issuecomment-5913797034)).

## Requirement outcomes

The verdicts are from QA's comment on the milestone issue
([#239](https://github.com/davison/md-notes/issues/239#issuecomment-5914576618)). QA tested a clean
detached worktree of `8a27726` and a build of the `v0.3.1` tag for comparison, each daemon on a
temporary token file, state and root, behind a local TLS proxy modelled on the e2e harness, driven
by Chromium 153 with a second https site, `attacker.test`, and by raw requests. The floor under the
verdicts: `make check` passed, and `make e2e` passed 124 of 124, including the four tailnet-session
cases.

| ID | Requirement | What the work established | QA |
|----|-------------|---------------------------|----|
| M13-R1 | The cookie is `SameSite=Lax`: a top-level GET navigation from outside the site carries the session, while a cross-site POST, PUT, DELETE, fetch or iframe is never authenticated by it; the Origin check still refuses a foreign `Origin`; no GET changes state | `setSessionCookie` writes `Lax`, for a login and a reissue alike. `ui/e2e/tailnet-session.test.mjs` serves `attacker.test`, whose link carries the session and whose form posts, credentialed fetch and iframe arrive without it; against the v0.3.1 binary the link case fails. The GET audit is in the plan, and `TestEveryGETRouteHasBeenAuditedAsARead` pins the route list ([PR #241](https://github.com/davison/md-notes/pull/241)) | [Satisfied](https://github.com/davison/md-notes/issues/239#issuecomment-5914576618): 59 of 59 checks in Chromium. Links, `window.open`, a script navigation and a `rel=noreferrer` link from a `data:` page, the nearest emulation of a shortcut, carried the cookie; under v0.3.1 the last showed the login page, reproducing #238. Every cross-site write, fetch, frame, beacon and subresource arrived without it. 24 foreign-`Origin` requests with a valid cookie got 403. 116 GET and HEAD requests under `strace -f` wrote nothing |
| M13-R2 | Every tailnet request sent to the login page or refused is logged with its reason, and a login names the browser family, without any cookie value, token or other secret | `session.Verify` names why a value failed; the guard and `/login` add the rest; every line goes through one bounded, escaping logger ([#240](https://github.com/davison/md-notes/issues/240#issuecomment-5913636592), [addendum](https://github.com/davison/md-notes/issues/240#issuecomment-5913882742), [correction](https://github.com/davison/md-notes/issues/240#issuecomment-5914051956)) | [Satisfied](https://github.com/davison/md-notes/issues/239#issuecomment-5914576618): 40 refusal cases, each answered with its status and logged once. None of 29 secret values, nor the planted device names in the `User-Agent`, appeared. Control and separator characters were escaped on one line. A 3000-request flood over 200 parallel connections, each request with its own forwarded address, wrote 30 lines, and the next reported the other 2970 |
| M13-R3 | The other session protections are unchanged | The only edits to existing assertions are Strict → Lax ([PR #241](https://github.com/davison/md-notes/pull/241)) | [Satisfied](https://github.com/davison/md-notes/issues/239#issuecomment-5914576618): the same probe against v0.3.1 and this build differed only in `SameSite`, on login and reissue: the attributes, the ages from now to 30 days and 60 seconds, future skew, rotation, the throttle and ten loopback cases. A v0.3.1 cookie was accepted, and reissued as `Lax` |
| M13-R4 | Every doc that names `SameSite=Strict` says `Lax` and why; the roadmap row; this record | The task wrote the introduction's *A browser on the tailnet*, `docs/running.md` and `contrib/mdn.1`. This record's pull request adds the roadmap row, the record, and the corrections under [The front door](#the-front-door-in-this-pull-request) | [Untestable at the verdict](https://github.com/davison/md-notes/issues/239#issuecomment-5914576618): the roadmap row and the record did not exist. Outside the records, `git grep` found no `SameSite` `Strict` left, and the introduction's account of Lax, the upgrade and the refusal log matched QA's measurements. A superseding verdict is owed once this record merges |

## Decisions

### `SameSite=Lax`: the operator's decision, before the milestone opened

**The probe.** Before any milestone existed, the coordinator built a probe binary in a scratch
worktree: v0.3.1 with one line changed, `internal/server/tailnet.go:134` from
`http.SameSiteStrictMode` to `http.SameSiteLaxMode`, built with `make build
VERSION=v0.3.1-lax-probe`, and nothing committed. The operator ran it himself, stopping the `mdn`
unit and running `serve` over the same config and token file. On the tablet the shortcut first
showed the login page, since the old `Strict` cookie was withheld; the token he entered issued a
`Lax` cookie; and after closing the shortcut fully and opening it again, it stayed logged in. His
words, relayed from the coordinator session: "Lax works, the shortcut stays logged in"
([#238](https://github.com/davison/md-notes/issues/238#issuecomment-5913454699)). The probe comment
also warned of its residue: a `Lax` cookie stays good under the stock daemon until it is reissued as
`Strict`, so the shortcut could keep working for up to a day after he went back.

**The decision.** The operator's words, relayed from the coordinator session: "open a milestone for
#238 and release v0.3.2, Lax is approved". The scope Decision records them at 14:36:50Z, and says
itself that this time they were recorded when they were relayed, not at record time
([#239](https://github.com/davison/md-notes/issues/239#issuecomment-5913477367)). The Decision says
the choice was made before the milestone opened, after the probe; the probe comment was posted at
14:35:32Z and the milestone issue at 14:36:29Z. Because the operator had decided, the task raised no
gate for it. The plan and #240's own Decision both say `Lax` is not re-decided there
([#240](https://github.com/davison/md-notes/issues/240#issuecomment-5913636592)).

**What it supersedes.** The `Strict` attribute in the #39 cookie Decision
([#39](https://github.com/davison/md-notes/issues/39#issuecomment-5632395808)), the M3 design, and
the `SameSite=Strict` that M12-R4 named among the protections to keep
([#230](https://github.com/davison/md-notes/issues/230)). #238 had said both needed the operator's
decision.

**Trade-off accepted,** as the scope Decision states it: `Lax` sends the cookie on a top-level GET
navigation from another site. Every GET the daemon serves is a read, which the other site cannot
see, and every write stays protected: `Lax` never sends the cookie on a cross-site POST, PUT or
DELETE, and `guardTailnet` refuses a cookie-authenticated request with a foreign `Origin`. That
check is #39's second defence, put there so that `SameSite` would not be the only one
([#238](https://github.com/davison/md-notes/issues/238)).

The plan added why the change is safe in Chromium: the attribute is set explicitly, so Chromium's
"Lax-allowing-unsafe" two-minute window, which applies only to a cookie with no `SameSite`, does not
apply ([#240](https://github.com/davison/md-notes/issues/240)). The round-one review measured it:
every cross-site POST it sent, from 0.4 to 3.2 seconds after the login, arrived without the cookie
([PR #241](https://github.com/davison/md-notes/pull/241#issuecomment-5913797034)). The same review
found login CSRF unreachable: a cross-site login POST is refused on `Origin` before the form is
read, and `Lax` changes nothing there, since the login presents no cookie.

**The upgrade.** `SameSite` is not under the MAC, so a `Strict` cookie issued by v0.3.1 still
checks, and it is reissued as `Lax` once it is a day old, or at the next login. The plan said so,
and that "until that reissue, a home-screen launch can still ask once"
([#240](https://github.com/davison/md-notes/issues/240)). QA walked it in Chromium: after the
upgrade a typed URL still reached the app, the first shortcut-like launch showed the login page
once, and after one paste both that launch and a link from another site stayed logged in
([#239](https://github.com/davison/md-notes/issues/239#issuecomment-5914576618)).

**Rejected:** keeping `Strict` and telling users to install the app instead. #238 had offered it as
the alternative to weigh, since NeoBrowser offers to install the app and an installed app may launch
as same-site. The scope Decision rejects it because it depends on the browser, and it was untested
on the tablet. The task did not test it either: a real install through the DevTools protocol had
been refused behind the harness's self-signed certificate on #231, and `Lax` is the fix the operator
chose ([PR #241](https://github.com/davison/md-notes/pull/241)).

### Scope, merge authority and the release

One task, #240, adopts #238 and carries M13-R1 to R4: the `Lax` switch with its cross-site proofs,
the refusal-reason logging and the docs. The record is this housekeeping pull request. "Release
v0.3.2" is standing merge confirmation for the task and the record, as on M11 and M12, and anything
else only the operator can answer is raised as a checkpoint. The change is a fix, so the release is
a patch ([#239](https://github.com/davison/md-notes/issues/239#issuecomment-5913477367)). Unlike
M12's, the scope Decision lists no backlog items left out on purpose.

#238 asked for diagnostics too, "so the next report like this is readable from the host": why a
tailnet request was sent to the login page, and the login's user-agent family
([#238](https://github.com/davison/md-notes/issues/238)). That became M13-R2.

### The refusal log

The Decision on #240
([#240](https://github.com/davison/md-notes/issues/240#issuecomment-5913636592)), as amended after
two review rounds
([addendum](https://github.com/davison/md-notes/issues/240#issuecomment-5913882742),
[correction](https://github.com/davison/md-notes/issues/240#issuecomment-5914051956)):

- **Reasons come from the session code and the guard.** `session.Verify` returns `ErrMalformed`,
  which includes a v0.3.0 session id, `ErrSignature`, `ErrExpired` or `ErrFuture`; `session.Check`
  is `Verify` without the reason, and unchanged. The guard adds no session cookie, a bearer token
  that is not the current one, `cross_origin`, `loopback_only`, and, under a configured
  `tailnet_host`, `bad_host` with the `Host` the request came in under.
- **"Signed before the last rotation" is only knowable while the daemon holds the old token.**
  `token.Store` keeps in memory the token it held before the last rotation it saw, and a cookie
  whose MAC fails under the current key but checks under that one is logged as signed before the
  last rotation. It is used for the log line and never to accept a session. The earlier token was
  already in memory until the rotation replaced it, so nothing new goes on disk. After a restart, or
  for a token file replaced while the daemon was stopped, the line reads "bad signature (another
  token, a token replaced while the daemon was stopped, another tailnet_host, or altered)". The MAC
  binds host and key together, so a failed check cannot say which.
- **The lines.** `tailnet: <addr> <method> <path> sent to the login page: <reason>` for a
  navigation, `tailnet: <addr> <method> <path> refused (<status> <code>): <reason>` for anything
  else, and `tailnet login from <addr> (<browser> on <platform>)`. The address is the forwarded one,
  and the path is logged without its query. The browser family and platform come from a coarse parse
  of the `User-Agent` into a fixed vocabulary, and the string itself is never logged.
- **Never logged:** the cookie value, the token, a derived key, the query string or the
  `User-Agent`. A test asserts that none of them appears, in hex or base64 as well.
- **Everything the caller chose is escaped** (the addendum): the path, the forwarded address, the
  method, the `Host` and the `Origin`. Each is cut to a bound on a rune boundary, then every
  non-printable character is written as its Go escape, `strconv.Quote` without the outer quotes.
  That covers `\n`, `\r`, `\x1b`, U+0085 and U+2028, and backslashes and quotes are escaped too, so
  a caller cannot imitate an escape.
- **Every tailnet refusal is logged** (the addendum): at `/login` also the 405, the 403
  `cross_origin`, the 400 for an unreadable form, the 400 for a request not over https, named as the
  proxy sending no `X-Forwarded-Proto: https`, and the 429; in the Host guard also the `bad_host`
  for a forwarded request with a loopback `Host` and for an absolute-form target. Nothing is
  excluded, so the requirement kept its "every".
- **Bounded.** A line is written once a minute per forwarded address, answer and reason, and at most
  30 lines a minute in total, the wrong-token login line included (the correction). The address
  comes from `X-Forwarded-For`, which the caller writes, the throttle's weakness on #39, so the
  total cap is what bounds a caller who varies it. The first refusal after a capped minute writes `N
  tailnet refusals not logged in the minute from HH:MM:SS`: with the next refusal rather than on a
  timer, because "a timer would be a goroutine to stop for a count nobody is waiting on", and naming
  the minute makes a late line accurate (the addendum).

Measured against the built daemon, the log the Decision quotes:

```
tailnet: 100.64.0.5 GET / sent to the login page: no session cookie
tailnet: 100.64.0.5 GET /api/roots refused (401 unauthorized): not a session value
tailnet login from 100.64.0.6 (Android WebView on Android)
tailnet: 100.64.0.6 GET /r/notes/index.md sent to the login page: signed with the token before the last rotation
```

**Rejected:**

- Logging the full `User-Agent`: it is long, and it fingerprints the device beyond what the operator
  needs.
- A per-address limit alone: an address can be spoofed.
- Silently dropping over the cap: a flood would then look like quiet.
- Putting the host in clear in the cookie to name "wrong host": a format change that would log out
  every device at v0.3.2.
- Reporting the left-out count on a timer (the addendum, above).

### The GET audit

`Lax` lets another site start a top-level GET with the cookie, so M13-R1 requires that no GET the
daemon serves changes state. The plan audited every route answered on GET or HEAD under the tailnet
name, on `8385614` ([#240](https://github.com/davison/md-notes/issues/240)):

| Route | State it touches |
|-------|------------------|
| `GET /api/roots` | none |
| `GET /api/r/{slug}/tree` | the in-memory list cache |
| `GET /api/r/{slug}/note/{path}` | the in-memory size cache |
| `GET /api/r/{slug}/diagram/{path}` | the in-memory SVG cache |
| `GET /api/r/{slug}/source/{path}` | the in-memory revision record a later PUT is checked against |
| `GET /api/r/{slug}/raw/{path}` | none; answered with `Content-Security-Policy: sandbox`, so an HTML or SVG file opened top-level from another site runs no script |
| `GET /api/r/{slug}/events` | starts the root's watcher in memory |
| `GET /api/r/{slug}/search`, `/tags` | none |
| `GET /login`, `GET /` and client routes | none |

Nothing is written to disk and nothing a user can see changes. The only writers on disk are the root
registry's add and remove (loopback-only under the tailnet name), the state settled at startup, and
the PUT, POST and DELETE of source, clips and uploads. A page elsewhere that navigates the user to
one of these GETs gets a page shown to the user, which it cannot read. An `EventSource` is a
subresource, so a cross-site one goes without the cookie even with `withCredentials`.

The round-one review read every GET handler and agreed with the table ([PR
#241](https://github.com/davison/md-notes/pull/241#issuecomment-5913797034)). QA measured it
directly under `strace -f`, with 0 write-mode `openat` and 0 `rename`, `unlink` or `mkdir` calls.
**What holds it in the suite** is `TestEveryGETRouteHasBeenAuditedAsARead`, which pins the list of
GET routes, so a new one fails until it is added. It does not look inside a handler; see [QA's
findings](#qas-findings-and-what-was-done).

## What the reviews changed

The task's pull request was reviewed three times by a clean-context session under the reviewer
contract, with the reviewer seat routed to the operator.

- **Round one** ([changes
  requested](https://github.com/davison/md-notes/pull/241#issuecomment-5913797034)), at `942ee7c`.
  Two blocking findings:
  1. **Log injection through the request path.** The path was written as
     `path.Clean("/"+r.URL.Path)`, and `r.URL.Path` is percent-decoded, so `%0a` became a line break
     and `%1b` an ESC. The reviewer forged a whole line, a successful `tailnet login from …` in red,
     "the line an operator trusts most", from an unauthenticated request. A cross-site `<img>`
     reaches the same path, so any web page the user visits could have done it.
  2. **Seven tailnet refusals wrote nothing**, while R2 and three docs said every one was logged:
     five at `/login` and two `bad_host` forms in the Host guard.

  The nits: the GET-route test caught only one spelling of a new route (3); the introduction listed
  a typed URL among the navigations `Strict` withheld the cookie on, which the reviewer disproved by
  reverting to `Strict` (4); the docs left out the one shortcut ask after the upgrade that the plan
  admitted (5); and "not logged in the last minute" could be hours late (6). The reviewer also ran a
  cross-site matrix of its own in Chromium 153 and measured the explicit `Lax` against the
  two-minute window.
- **The fix pass** added `b107985` (escaping, and every refusal logged), `565e3fd` (the route test
  now parses every file of the package with `go/parser`), `83d0efc` (`test(session)`, a gofmt
  reorder of one import line) and `45bb114` (the docs for nits 4 to 6 and finding 2), and the
  addendum Decision. `TestTailnetLogCannotBeForged` failed on the old head with 4 lines, 2 of them
  forged; `TestEveryTailnetRefusalIsLogged` failed there for each of the seven new cases
  ([#240](https://github.com/davison/md-notes/issues/240#issuecomment-5913882742)).
- **Round two** ([changes
  requested](https://github.com/davison/md-notes/pull/241#issuecomment-5914022268)), at `45bb114`.
  One blocking finding: **the wrong-token login line was not bounded.** The addendum had kept it
  outside the cap "since the login throttle already bounds it", and that premise was false: the
  throttle delays one address's failures, and never trips for a caller who varies `X-Forwarded-For`.
  2000 wrong-token posts from 2000 addresses wrote 2000 lines in 5.7 seconds, and broke the
  introduction's "at most thirty lines a minute". Round one's findings were all closed. The reviewer
  pushed the escaping further, with invalid UTF-8, bidi and zero-width characters, 3000-digit
  addresses and long emoji runs, and slipped a new GET route past the parser test only as a method
  value, "contrived enough that I do not ask for it".
- **The correction** withdrew the addendum's premise, and `950f7ce` sent the line through the same
  logger, as `tailnet: <addr> POST /login refused (401): not the current token`. The bound test now
  sends 50 wrong logins from 50 addresses: 50 lines on the previous head, 30 now
  ([#240](https://github.com/davison/md-notes/issues/240#issuecomment-5914051956)).
- **Round three** [approved](https://github.com/davison/md-notes/pull/241#issuecomment-5914134224)
  at `950f7ce`, with no findings. The same flood wrote 30 lines, and the next wrong login reported
  the other 1970.

The pull request carries the operator-confirmation comment at 15:13:56Z, "reviewed and accepted by
@davison as both author and operator" ([PR
#241](https://github.com/davison/md-notes/pull/241#issuecomment-5914142314)), written by `task
finish 240 --operator-confirm` and resting on the standing merge confirmation in the scope Decision,
"release v0.3.2". The coordinator's attribution Decision records it, at record time
([#239](https://github.com/davison/md-notes/issues/239#issuecomment-5914771903)).

No coordinator disposition comment covers any of the reviews. The addendum and the correction answer
the findings, and the fix-pass commits the nits.

## A pull request GitHub would not link, and a commit that closed the task

On M12, no pull request could be linked to its task by the closing keyword, the operator linked one
by hand, and GitHub did not close the task at the merge either; the coordinator closed it by hand
and filed [radiusred/gh-codecrew#386](https://github.com/radiusred/gh-codecrew/issues/386). GitHub
reported no incident then. By M13 others had reported the same on GitHub's community forum, both on
2026-09-30:

- [community discussion 209162](https://github.com/orgs/community/discussions/209162): the web UI
  recognises `Closes #N` in a pull request, but `closingIssuesReferences` and
  `closedByPullRequestsReferences` are empty;
- [community discussion 209148](https://github.com/orgs/community/discussions/209148): a pull
  request GitHub lists as closing an issue, linked by hand from the Development sidebar, was merged
  into the default branch and the issue stayed open. A comment there, from another project, notes
  that a closing keyword in the commit message still closes the issue, and gives the workaround:
  "put `Closes owner/repo#N` in the squash commit message, or close the issue by hand".

The same happened on M13:

- **The keyword did not link.** PR #241 opened at 14:46:09Z with `Closes #240` as its first line.
  The `connected` event on #240 and #241 is at 14:54:36Z, and `closingIssuesReferences` on #241
  names #240. The operator made the link by hand from the Development sidebar and reported it in the
  coordinator session: "linked #241 to #240"; the coordinator then read `closingIssuesReferences` as
  `[240]` ([#239](https://github.com/davison/md-notes/issues/239#issuecomment-5914771903)). #240
  carries no Deviation for the missing link, as #231 did on M12.
- **The commit closed the task.** The fix pass's docs commit, `45bb114` on the branch and
  [`89c0b51`](https://github.com/davison/md-notes/commit/89c0b51) on `main`, ends its body with
  `Closes davison/md-notes#240`. The round-two review calls it "the stated workaround, not a
  finding" ([PR #241](https://github.com/davison/md-notes/pull/241#issuecomment-5914022268)). #241
  merged at 15:14:02Z, and #240's timeline shows it closed at 15:14:06Z by commit `89c0b51`. The
  pull request was rebase-merged, not squashed, and the keyword in one of the rebased commits was
  enough. Nothing had to be closed by hand.
- **Where the discussions and the workaround came from.** The operator asked the coordinator to look
  for reports: "Can you search to see if there's any open tickets about it with GH?", and the
  coordinator found 209162 and 209148. The operator found the workaround in 209148 and relayed it,
  quoting it: "Workaround: put Closes owner/repo#N in the squash commit message, or close the issue
  by hand." The coordinator asked the implementer to put `Closes davison/md-notes#240` in a commit
  body. Asked to check Settings → General → Issues → "Auto-close issues with merged linked pull
  requests", he answered "it's on", which rules out a repository setting as the cause
  ([#239](https://github.com/davison/md-notes/issues/239#issuecomment-5914771903)).

`task finish 240` ran at the merge, with the manual link in place, and closed the adopted capture
#238 at 15:14:06Z ([#238](https://github.com/davison/md-notes/issues/238#issuecomment-5914145123)).
radiusred/gh-codecrew#386, for `task finish` to check that the task closed, is still open.

## Deviations and corrections

**The coordinator's relays behind the link, the workaround and the auto-close setting were posted
late.** This record's Deviation on the milestone issue named the gap
([#239](https://github.com/davison/md-notes/issues/239#issuecomment-5914650687)), and the
coordinator's attribution Decision then recorded them at record time
([#239](https://github.com/davison/md-notes/issues/239#issuecomment-5914771903)); see [the section
above](#a-pull-request-github-would-not-link-and-a-commit-that-closed-the-task).

**The addendum's reason for leaving the wrong-token line unbounded was wrong.** Withdrawn by the
correction on #240, as above.

**The plan and the code differ in small ways.** The plan said `session.Check` would return a reason;
the code added `session.Verify` and left `Check` unchanged, as PR #241's body says. The Decision's
left-out line, "N tailnet refusals not logged in the last minute", became "in the minute from
HH:MM:SS" in the addendum.

**PR #241's body was not edited after the fix passes.** It still says the commits include "one
`docs` for the prose", where `main` has two; its R2 list names neither
`TestTailnetLogCannotBeForged` nor `TestEveryTailnetRefusalIsLogged`; and it says the upgrade "needs
nothing from users", which nit 5 qualified with the one shortcut ask. The round-two review also
noted that `83d0efc`, on `main` as [`da138d8`](https://github.com/davison/md-notes/commit/da138d8),
"gofmt the reason table", reorders one import line.

## QA's findings, and what was done

QA posted a suite-gap finding on #240 and two observations on #239 outside the verdicts
([#240](https://github.com/davison/md-notes/issues/240#issuecomment-5914564576),
[#239](https://github.com/davison/md-notes/issues/239#issuecomment-5914597383)), and the coordinator
disposed of them ([#239](https://github.com/davison/md-notes/issues/239#issuecomment-5914612638)).
None changed a verdict.

1. **Two promises the suite holds only in part.** `TestEveryGETRouteHasBeenAuditedAsARead` pins
   which GET routes exist, not that they never write: a GET handler that started writing would pass.
   And the e2e cross-site matrix is narrower than QA's: no PUT, DELETE, upload, note create,
   subresource, `window.open` or opaque-origin launch. QA also noted that "a Strict-era cookie still
   checks and is reissued Lax", a plan item, has no test of its own; it holds by construction, since
   the value carries no attribute. **Captured as
   [#242](https://github.com/davison/md-notes/issues/242)**, not a remedy task: QA measured the
   behaviour directly, and v0.3.2 is not at risk.
2. **Repeats removed by the dedupe are not counted.** The left-out line counts only what the
   30-a-minute cap dropped, so a device making hundreds of refused requests a minute looks like one
   making one. The introduction words it accurately. Folded into #242.
3. **Wrong-token logins from callers who vary `X-Forwarded-For` are slowed but never refused.** 600
   such logins all got 401 and none 429. No action: it is the weakness recorded on #39 and in
   `internal/server/refusals.go`, the throttle is byte-identical to v0.3.1's, and the 130-bit token
   is the defence. M13 bounded the log, not the attempts.

**Rejected:** a remedy task for the test gaps inside M13, which "would delay v0.3.2 for tests that
guard against changes nobody is making".

**A QA process slip.** During its run, QA briefly left a `pids.txt` in the operator's main checkout,
against its brief. QA found and deleted it, and the coordinator checked that the checkout is clean
on `main` at `8a27726`. The disposition records it "so the record can name it"
([#239](https://github.com/davison/md-notes/issues/239#issuecomment-5914612638)).

## The front door in this pull request

The task brought the introduction's *A browser on the tailnet*, `docs/running.md` and
`contrib/mdn.1` up to date, and every review and QA checked them. The claims M13 changed elsewhere
in the front door, brought into line with `8a27726`:

- **`docs/introduction.md`.** The opening now describes the project at the end of milestone
  thirteen, and a paragraph says what M13 changed. *What holds these numbers* counts 124 browser
  checks instead of 123, and says what the tailnet suite now drives from another site.
- **`CONTRIBUTING.md`.** The `make e2e` paragraph says the tailnet suite covers a login from another
  site, and counts 124 tests instead of 123; there are still twelve suites.

The README says the app works over the tailnet "behind a login" and makes no claim about the cookie.
No other guide names `SameSite`. `docs/e-ink.md` says that a NeoBrowser session "survives closing
the tab", which stays true. The same page says NeoBrowser "has not been checked for the install
prompt" and that a tablet has "no home screen habit to fit into". #238's report, that NeoBrowser
offers to install a shortcut and that the operator opens md-notes from one, contradicts both.
Neither is a claim M13 changed, so this pull request leaves them as they are, and the coordinator
captured the rewrite as [#244](https://github.com/davison/md-notes/issues/244).

## Captures adopted, and captures raised

M13 adopted one capture, [#238](https://github.com/davison/md-notes/issues/238), closed by `task
finish 240` when #241 merged
([#238](https://github.com/davison/md-notes/issues/238#issuecomment-5914145123)).

It raised two, both open:

- [#242](https://github.com/davison/md-notes/issues/242), at 15:40:49Z from QA's findings: a test
  that runs every audited GET route and fails on any write, the e2e cross-site matrix widened to
  QA's cases, and a repeat count for deduplicated refusals; "not release-blocking";
- [#244](https://github.com/davison/md-notes/issues/244), at 15:50:14Z from this record's front-door
  check: `docs/e-ink.md`'s NeoBrowser claims rewritten from #238's report, covering the shortcut,
  the install offer (untried) and the one token prompt after the upgrade.

No review proposed a capture. M12's captures [#235](https://github.com/davison/md-notes/issues/235)
and [#236](https://github.com/davison/md-notes/issues/236) stay open; M13 changed neither the
session format nor the installed-app path.

## The release: v0.3.2, to be tagged

The plan in the scope Decision
([#239](https://github.com/davison/md-notes/issues/239#issuecomment-5913477367)):

1. After `gh codecrew milestone close 13`, the coordinator pushes the annotated tag `v0.3.2` and
   adds any upgrade note the change needs to the drafted notes.
2. `release.yml` drafts the Release.
3. Publishing the draft stays the operator's, per the decision on #133.

It is a patch release because the change is a fix, and no M13 commit is `feat`. The introduction and
QA's upgrade walk name one thing a user meets at the upgrade: a device that opens the notes from a
home-screen shortcut is asked for the token once. When this record was written, `v0.3.1` was the
newest tag and `v0.3.2` did not exist.

## Where the record is silent

**Four of the operator's words were recorded late.** Every seat posts under the one account, so the
account alone does not say who typed a comment. The scope Decision recorded its attribution when the
words were relayed, as it says itself, where M12's came at record time. Four later relays were not
posted when they were said:

- "linked #241 to #240", behind the `connected` event at 14:54:36Z;
- the 209148 workaround, "put Closes owner/repo#N in the squash commit message, or close the issue
  by hand", behind `89c0b51`'s closing line;
- "Can you search to see if there's any open tickets about it with GH?", the ask that found the two
  discussions;
- "it's on", the repository's auto-close setting.

This record's Deviation on #239 named three of them, the link, the workaround and the auto-close
check ([#239](https://github.com/davison/md-notes/issues/239#issuecomment-5914650687)). The
coordinator's attribution Decision, posted at 15:50:12Z, records all four, the search ask included,
and that `task finish 240 --operator-confirm` wrote the operator confirmation on PR #241
([#239](https://github.com/davison/md-notes/issues/239#issuecomment-5914771903)).

**Relayed and recorded at the time:** the operator's device reports in #238, his "Lax works, the
shortcut stays logged in" on the probe, and "open a milestone for #238 and release v0.3.2, Lax is
approved". The session those words came from is not on GitHub, so they are quoted as the coordinator
gave them.
