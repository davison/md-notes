# M12 — A tailnet login that lasts until the token is rotated

Tracking issue: [#230](https://github.com/davison/md-notes/issues/230). Its one implementation task,
[#231](https://github.com/davison/md-notes/issues/231), is merged on `main` at
[`19823d0`](https://github.com/davison/md-notes/commit/19823d0). The milestone was opened at
10:09:06Z on 2026-09-30, and the task merged at 11:24:50Z the same day. The task branch was cut from
[`1c9698b`](https://github.com/davison/md-notes/commit/1c9698b), the v0.3.0 tree the plan measured
as "today" ([#231](https://github.com/davison/md-notes/issues/231)).

Independent QA graded M12-R1 to M12-R4 on `main` at `19823d0`, and all four were satisfied
([#230](https://github.com/davison/md-notes/issues/230#issuecomment-5910435811)). M12-R5 was
untestable, because it includes the roadmap row and this record, which did not yet exist. The parts
of M12-R5 that the task delivered were checked against QA's own measurements and found accurate. The
pull request that carries this record delivers the rest, and a superseding verdict is owed after it
merges.

The milestone is to be released as v0.3.1. At the time of writing the tag does not exist; see [The
release](#the-release-v031-to-be-tagged).

## Goal and outcome

The milestone's goal, as #230 states it: a device that has presented the token once on the tailnet
host stays logged in across daemon restarts, upgrades and reboots, and until the token is rotated,
without storing a second secret at rest and without giving up the cookie protections the tailnet
login was built with; released as v0.3.1 ([#230](https://github.com/davison/md-notes/issues/230)).

It comes from one capture, [#229](https://github.com/davison/md-notes/issues/229), raised by the
coordinator on the operator's ask. The operator's words, as the capture quotes them: "the
requirement to add the token in remote sessions is too inconvenient: I'm being asked for it way more
often than I expected. It's really awkward on the e-ink tablet and not much better an experience on
the phone. Once the token has been input once, I would only expect it to be required again if the
token is rotated on the server, but that's not the case at all." The capture measured why: sessions
were held in memory only, by the #39 design, so every start of the daemon logged out every device,
and the `mdn` unit on the operator's host had started five times since 2026-09-19. Sessions also
expired after a fixed 30 days however often the device was used.

The scope was the operator's, given in the coordinator session and relayed by the coordinator: "open
a milestone for #229 and run it through to a release of v0.3.1", as the scope Decision quotes it
([#230](https://github.com/davison/md-notes/issues/230#issuecomment-5909079844)). He typed "ov
v0.3.1", and the Decision corrected it to "of"
([#230](https://github.com/davison/md-notes/issues/230#issuecomment-5910571213)).

What a reader has now:

- **A tailnet login survives the daemon.** A restart, an upgrade to a newer build, a host reboot and
  closing the browser all leave a logged-in device logged in. The daemon keeps nothing about
  sessions, in memory or on disk.
- **Rotating the token logs every device out.** `mdn token --rotate`, or a token file replaced while
  the daemon was running or stopped, ends every session on its next request.
- **A session in use never expires.** A cookie a day old or more is reissued on a response the
  daemon serves. A device left unused for 30 days is logged out.
- **The cookie keeps every protection it had.** `__Host-mdn_session`, `Path=/`, no `Domain`,
  `HttpOnly`, `Secure`, `SameSite=Strict`, and never the token in it. The login throttle and
  loopback behave as they did in v0.3.0.
- **Every device logs in once more at the upgrade to v0.3.1,** and then stays logged in. v0.3.0's
  sessions were in memory, and the new daemon does not recognise them.

### What shipped

| Task | Requirements | Adopts | Pull request | Approved at | Last head | On `main` as | Merged |
|------|--------------|--------|--------------|-------------|-----------|--------------|--------|
| [#231](https://github.com/davison/md-notes/issues/231) tailnet sessions that survive restarts and end on rotation | M12-R1 to R4, M12-R5 (part) | #229 | [PR #233](https://github.com/davison/md-notes/pull/233), two rounds (round one on [PR #232](https://github.com/davison/md-notes/pull/232)) | `1211ba6` | `cf8d515` | [`7e13e56`](https://github.com/davison/md-notes/commit/7e13e56) to [`19823d0`](https://github.com/davison/md-notes/commit/19823d0), ten commits | 11:24:50Z |

The pull request landed by rebase, so the commits on `main` differ from the head the last review
saw. It merged with one commit added after the approval, which no later review comment covers: round
two's only nit, a reflow of the idle-limit bullet in the introduction. It is `cf8d515` on the branch
and [`19823d0`](https://github.com/davison/md-notes/commit/19823d0) on `main` ([PR
#233](https://github.com/davison/md-notes/pull/233#issuecomment-5909610270)).

The commits are `fix`, `test` and `docs`, and none is `feat`, as the plan and the release Decision
expected ([#230](https://github.com/davison/md-notes/issues/230#issuecomment-5909079844)). The man
page took `fix(man)` so that CI runs for it, under the commit-type rule M11 introduced.

## Requirement outcomes

The verdicts are from QA's comment on the milestone issue
([#230](https://github.com/davison/md-notes/issues/230#issuecomment-5910435811)). QA tested a clean
worktree of `19823d0` with a real `mdn serve --tailnet-host` behind a standalone Node TLS proxy,
driven by headless Chromium on a persistent profile and by raw https requests, with every daemon
under `strace -f`. It built four binaries: `main` twice (the second with a local-only version
string), the first fix commit `bbd2658`, and the `v0.3.0` tag. The floor under the verdicts: `make
check` passed, and `make e2e` passed 123 of 123, including the three new tailnet-login cases.

| ID | Requirement | What the work established | QA |
|----|-------------|---------------------------|----|
| M12-R1 | A session issued on the tailnet host survives a restart, an upgrade to a newer build and a host reboot, proven end to end against a real daemon restarted between requests | A stateless signed cookie checked against a key derived from the token file ([#231](https://github.com/davison/md-notes/issues/231#issuecomment-5909297037)). `TestTailnetSessionSurvivesARestart`, and `ui/e2e/tailnet-session.test.mjs` in Chromium behind TLS, which fails on the restart leg against the v0.3.0 binary ([PR #233](https://github.com/davison/md-notes/pull/233)) | [Satisfied](https://github.com/davison/md-notes/issues/230#issuecomment-5910435811): one `POST /login` for a chain of restarts of the same binary, an upgrade from `bbd2658` to `main`, a restart into a different binary of the same commit, and a simulated reboot that replaced the proxy process and relaunched the browser. v0.3.0 reproduced the #229 bug, and the upgrade from it logged out once |
| M12-R2 | Rotation ends every session on the next request, restarted or not, and a cookie can never be forged or replayed under a different token | Rotation changes the key, so an old cookie fails its MAC. The MAC covers the tailnet host name, and the check is strict about shape before `hmac.Equal`. Server tests for rotation while running, while stopped and across a restart, another token, another host and forgery | [Satisfied](https://github.com/davison/md-notes/issues/230#issuecomment-5910435811): 401 after `--rotate`, after the file was replaced while stopped, renamed while running and overwritten in place, with no reissue of a due cookie. Every tampered, re-MACed, other-token, other-host and hostile value was refused, against controls minted by an independent re-implementation that got 200. A restored earlier token revived its cookies, as the addendum Decision documents |
| M12-R3 | A session in use does not expire on a fixed date; the cookie is refreshed on use; what bounds a lost or borrowed device is documented; any bound dropped is the operator's call | Reissue at a day old, on served responses only; refused at 30 days from its issued-at. The operator chose the 30-day idle limit at the gate ([#231](https://github.com/davison/md-notes/issues/231#issuecomment-5909510463)) | [Satisfied](https://github.com/davison/md-notes/issues/230#issuecomment-5910435811): 200 with no cookie below 24 hours, 200 with a new cookie from 24 hours to just under 30 days, 401 past 30 days, and future skew at 5 minutes. Chromium kept the renewal. A refused request got no new cookie. The docs state the bound on a lost device |
| M12-R4 | The cookie keeps its protections, no secret beyond the token file is written to disk, and the throttle and loopback are unchanged | One function sets every cookie's attributes. Nothing in `--state` besides the token ([#231](https://github.com/davison/md-notes/issues/231#issuecomment-5909297037)) | [Satisfied](https://github.com/davison/md-notes/issues/230#issuecomment-5910435811): the attributes on the login and the reissue, and in Chromium's jar. The value carries no form of the token or the derived key. `strace` over 14 daemon lifetimes found no write but the first-start token file. The throttle's 25-login run and a 12-request loopback matrix matched v0.3.0 |
| M12-R5 | The introduction's Authentication section and the man page say how long a login lasts and what ends it; the roadmap row; this record | The task wrote the introduction's *A browser on the tailnet*, `docs/running.md` and `contrib/mdn.1`. This record's pull request adds the roadmap row, the record, and the corrections under [The front door](#the-front-door-in-this-pull-request) | [Untestable at the verdict](https://github.com/davison/md-notes/issues/230#issuecomment-5910435811): the roadmap row and the record did not exist. The parts that did matched QA's measurements. A superseding verdict is owed once this record merges |

## Decisions

### Scope, merge authority and the devices

One task, #231, carries M12-R1 to R5 and adopts #229, and the record is this housekeeping pull
request ([#230](https://github.com/davison/md-notes/issues/230#issuecomment-5909079844)). The
Decision takes "run it through to a release" as standing merge confirmation for the task and the
record, as on M11, while anything only the operator can decide is still raised as a
`cc:needs-decision` checkpoint. `govulncheck ./...` was clean at opening. **Left out on purpose:**
#107, #108, #110, #111, #122, #127, #145, #149, #207, #225, #226 and #227.

#229 asked for a measurement on the operator's own phone and e-ink tablet. No seat can drive those
devices, so the task measured desktop Chromium, including an app window where it could. The operator
checks his devices after installing v0.3.1, and anything that finds becomes a capture, not a
requirement of this milestone.

### A stateless signed cookie, not a persisted store

#229's Shape offered (a) stateless cookies, (b) a persisted session store in `--state`, or (c)
something else. The plan chose (a), and the Decision recorded it when the code landed
([#231](https://github.com/davison/md-notes/issues/231#issuecomment-5909297037)):

- **The value** is `v1.<issued-at, unix seconds>.<16 random bytes>.<HMAC-SHA256>`, in the cookie
  `__Host-mdn_session`.
- **The MAC** covers the version, the tailnet host name, the issued-at and the id. Its key is
  `HMAC-SHA256(token, "mdn session key v1")`, derived from the token the daemon already holds
  (`token.Store.Derive`).
- **The check** refuses a wrong field count, a non-canonical issued-at, an id or MAC of the wrong
  decoded length and non-canonical base64, all before the MAC comparison, which is `hmac.Equal`. It
  then refuses an issued-at more than 5 minutes in the future, or 30 days or more in the past.
- **Rotation is by construction.** A new token is a new key, so every cookie signed under the old
  one fails, on the next request, whether or not the daemon restarted. There is no store, no
  generation and no sweep.
- **The login race is kept closed.** `AuthenticateDerive` checks the presented token and derives the
  key from one read of it, so a rotation landing mid-login cannot sign a cookie under a token the
  browser never presented. The token's generation counter is gone.

Measured end to end in headless Chromium behind a local TLS proxy standing in for `tailscale serve`,
the v0.3.0 tree showed the login page after every restart, a browser relaunch and an app window, and
the branch showed the app for all three, while both refused after a rotation. `session.Check` costs
1.1 µs per call.

**This reverses two rejections in the #39 session Decision**
([#39](https://github.com/davison/md-notes/issues/39#issuecomment-5632395808)), the M3 design that
held sessions in memory. The #231 Decision answers each of #39's objections:

- *A signed stateless cookie "makes the token the signing key for a bearer credential that leaves
  the machine".* The key is derived from the token, not the token, and the token cannot be recovered
  from the cookie.
- *"Nothing to revoke short of rotating."* That was already true of the #39 design, which chose no
  logout and named rotation as the revocation that matters.
- *"A per-request rolling renewal."* A reissue happens at most once a day, and M12-R3 requires
  refresh on use.
- *"Persisting sessions beside `roots.json`."* Still rejected.

#39 also rejected "an idle timeout on top of the absolute one". The #231 Decision does not mention
it. M12 does not add an idle timeout on top: it replaces the absolute limit with an idle one, so a
session still has one lifetime to explain. That reading is this record's, not a recorded one.

**Trade-off:**

- Logging in again no longer deletes the session the browser held. The superseded value stays good
  until its own idle limit, though the browser has already overwritten it. The review of PR #232
  measured it and judged that no extra gate was needed: #39's "a credential nothing will present
  should not stay live" is weakened from "at once" to "within the idle limit" ([PR
  #232](https://github.com/davison/md-notes/pull/232#issuecomment-5909481856)).
- The store's cap of 64 sessions goes, since there is no store to grow. The review noted that the
  cap bounded memory, not access, and that failed logins are still throttled.

**Rejected:**

- (b), a persisted store in `--state`: a second secret at rest, which M12-R4 leaves to the operator,
  and it still needs a rotation check.
- The token itself as the HMAC key: the derivation costs nothing and keeps the key and the token
  apart.
- Leaving the host out of the MAC: a cookie issued under one `tailnet_host` would then check under
  another name holding the same token.

### Refresh, and the 30-day idle limit

The plan reissues a cookie once it is a day old, with a full `Max-Age`, so a device in use gets at
most one `Set-Cookie` a day rather than one per response, which was #39's objection to rolling
renewal. The idle limit is both the age at which a cookie is refused and its `Max-Age`, so the
browser and the daemon agree. Only a response the guard serves carries a reissue: a request the
Origin check or the allow-list refuses keeps its old cookie
([#231](https://github.com/davison/md-notes/issues/231#issuecomment-5909297037)).

Two things #39 relied on are dropped: the 30-day absolute bound, since a device in use never lapses
now, and "logging in again deletes the session the browser held", as above
([#231](https://github.com/davison/md-notes/issues/231)). No "log out other devices" action was
proposed. The first of the two went to the operator as the task's one gate.

## The human gate

**What bounds a session that is not being used, on #231**
([#231](https://github.com/davison/md-notes/issues/231#issuecomment-5909185722)), raised at
10:14:29Z. Dropping the 30-day absolute bound took away what #39 relied on for a lost or borrowed
device. The options were:

- (A), recommended: a 30-day idle limit. A device used at least once a month never asks for the
  token again, a browser used once and left behind lapses within 30 days, and a device someone else
  keeps using stays in until rotation, which was also true in v0.3.0 for up to 30 days;
- (B) a 400-day idle limit, Chromium's cap on a cookie's lifetime: in practice only rotation ends a
  session, matching the operator's words on #229, and a borrowed browser stays logged in for about
  13 months unless he rotates;
- (C) a 90-day idle limit, for an e-ink tablet that can sit unused for weeks.

The gate held only the constant and the sentence in the docs that names it, and the rest of the task
went ahead with A. The resolution, at 10:37:20Z, reads in full "expire after 30 days idle"
([#231](https://github.com/davison/md-notes/issues/231#issuecomment-5909510463)), and the
`cc:needs-decision` label came off at 10:37:30Z. The first review of PR #232 had counted the
unresolved gate as its one blocking finding, and checked that `session.Idle` is the only place the
value lives in code, so the answer needed no code change ([PR
#232](https://github.com/davison/md-notes/pull/232#issuecomment-5909481856), finding 1).

Every seat in this project posts under the one account, so the account alone does not say who typed
a comment. The coordinator's attribution Decision records that the operator typed the resolution and
removed the label himself, and confirmed both in the coordinator session: "#231 resolved: option A"
([#230](https://github.com/davison/md-notes/issues/230#issuecomment-5910571213)). That Decision was
posted late; see [Where the record is silent](#where-the-record-is-silent).

**What "30 days idle" means exactly.** A session is reissued only once it is a day old, so the 30
days run from the last reissue, not the last request, and a device used and then left lapses between
29 and 30 days after its last use. The introduction says "30 days from the last reissue, not from
the first login", and the man page rounds it to "after 30 days idle". QA raised it as an
observation, and the coordinator accepted the rounding for a man page
([#230](https://github.com/davison/md-notes/issues/230#issuecomment-5910459560), item 1). A commit
subject on `main` says the 30 days are counted "from the last use"
([`7d54096`](https://github.com/davison/md-notes/commit/7d54096)); the text it committed says "from
the last reissue".

## The addendum: a restored token, and a refreshed id

Two nits from the first review of PR #232 became an addendum to the stateless-session Decision
([#231](https://github.com/davison/md-notes/issues/231#issuecomment-5909500808); review nits 2 and 5
at [PR #232](https://github.com/davison/md-notes/pull/232#issuecomment-5909481856)).

**Writing an earlier token back revives its sessions.** A cookie is good while the token it was
signed under is in force. If an earlier token is written back, for example by restoring `--state`
from a backup after a rotation made to revoke a device, the sessions signed with it are good again,
up to their idle limit. The reviewer measured it: 401 after the rotation, 200 once the old token was
back. v0.3.0 differed: its generation counter moved on every change of value, so within one process
a restored token brought no session back. The Decision accepts this rather than add state: anyone
holding the restored token can log in anyway, and preventing it would need a record of past tokens,
a second thing at rest. "Rotation ends every session" holds as long as the rotated-away token is not
restored, and the introduction's rotation bullet says so
([`3efc805`](https://github.com/davison/md-notes/commit/3efc805)). QA reproduced it, and found that
a further rotation ended the revived session again.

**A refreshed cookie gets a new random id.** The plan's refresh bullet said "same id, new
issued-at"; the code draws a new id, and the addendum corrects the plan. Nothing reads the id: it
exists only so that two values issued in the same second differ, and it is under the MAC. Keeping it
would mean parsing it out of the presented cookie and passing it back into `Issue`, an extra path
with no effect. With a new id, a reissue is the same call a login makes. The code was unchanged.

## Three pull requests, and a task GitHub would not link or close

The protocol's `task finish` merges only a pull request that GitHub reports as closing the task
(`closingIssuesReferences`). For #231, GitHub never made that link from the closing keyword, and did
not close the task at the merge.

- **[PR #232](https://github.com/davison/md-notes/pull/232)**, opened at 10:22:48Z from the task's
  branch, got no `connected` event on #231, and `closingIssuesReferences` stayed empty. The first
  review guessed at an owner-qualified `Closes davison/md-notes#231` as the cause (finding 6). The
  body was re-saved, the title edited, the pull request closed and reopened and a `Fixes` keyword
  tried, with no link, and its first line now reads `Closes #231`
  ([#231](https://github.com/davison/md-notes/issues/231#issuecomment-5909597289)). It was closed at
  10:39:56Z, with its round-one review left standing ([PR
  #232](https://github.com/davison/md-notes/pull/232#issuecomment-5909545150)).
- **[PR #233](https://github.com/davison/md-notes/pull/233)**, a new pull request from the same
  branch, opened at 10:39:58Z. It did not link either. The round-two review confirmed that its first
  line was `Closes #231`, so the qualified form had not been the cause ([PR
  #233](https://github.com/davison/md-notes/pull/233#issuecomment-5909610270)).
- **[PR #234](https://github.com/davison/md-notes/pull/234)**, from a fresh branch name,
  `task/231-sessions`, opened at 10:41:59Z to test whether the linked branch was the cause. It made
  no difference, and it was closed at 10:43:22Z and its branch deleted ([PR
  #234](https://github.com/davison/md-notes/pull/234#issuecomment-5909592765)).

The Deviation on #231, at 10:43:42Z, records all three and that `task finish` refused `NO_PR` as a
result. GitHub reported no incident, and M11's PR #219 had linked normally the day before. The work
continued on #233, and the operator was asked to link it by hand from the Development sidebar, with
GitHub Support as the next step if that failed too
([#231](https://github.com/davison/md-notes/issues/231#issuecomment-5909597289)).

**The manual link.** #231's timeline shows a `connected` event at 11:23:22Z, and
`closingIssuesReferences` on #233 now names #231. The operator made the link himself from the
Development sidebar, and confirmed it in the coordinator session: "linked #233 to #231. Is it a
github bug?" ([#230](https://github.com/davison/md-notes/issues/230#issuecomment-5910571213)). The
upstream capture says the same
([radiusred/gh-codecrew#386](https://github.com/radiusred/gh-codecrew/issues/386)). From about
10:44Z to 11:23Z nothing else happened on #231: after the Deviation, the coordinator asked the
operator in the session to link #233 by hand, and the task waited on that until the `connected`
event ([#230](https://github.com/davison/md-notes/issues/230#issuecomment-5910571213)). `task finish
231 --operator-confirm` then passed its gate, posted the operator-confirmation comment at 11:24:46Z
([PR #233](https://github.com/davison/md-notes/pull/233#issuecomment-5910187914)), merged #233 at
11:24:50Z as `19823d0`, and closed the adopted capture #229
([#229](https://github.com/davison/md-notes/issues/229#issuecomment-5910189341)).

**The hand-close.** GitHub did not close #231 at the merge, even with the manual link in place, and
`task finish` reported that the task "closes via its closing keyword" without checking. The
coordinator noticed by reading the issue's state, and closed #231 by hand at 11:25:09Z with a
comment naming the pull request and the merge
([#231](https://github.com/davison/md-notes/issues/231#issuecomment-5910193024)).

**The upstream capture.** On the operator's ask in the coordinator session, "yes, file the upstream
capture for checking that a close happened"
([#230](https://github.com/davison/md-notes/issues/230#issuecomment-5910571213)), the coordinator
filed [radiusred/gh-codecrew#386](https://github.com/radiusred/gh-codecrew/issues/386) at 11:28:23Z.
It asks that `task finish` read the task's state back after a merge, and close it itself if GitHub
has not, as it already closes adopted captures. It notes that
[radiusred/gh-codecrew#318](https://github.com/radiusred/gh-codecrew/issues/318) covers the case
before a merge and cannot catch this one. It is open.

## What the reviews changed

The task's pull request was reviewed by a clean-context session under the reviewer contract, with
the reviewer seat routed to the operator. It merged under the operator's standing confirmation, and
its operator-confirmation comment was written by `gh codecrew task finish 231 --operator-confirm`,
resting on the standing merge confirmation in the scope Decision, not typed by the operator
([#230](https://github.com/davison/md-notes/issues/230#issuecomment-5909079844), [PR
#233](https://github.com/davison/md-notes/pull/233#issuecomment-5910187914),
[#230](https://github.com/davison/md-notes/issues/230#issuecomment-5910571213)).

- **Round one, on PR #232** ([changes
  requested](https://github.com/davison/md-notes/pull/232#issuecomment-5909481856)), at `5f8bf1c`.
  One blocking finding, procedural: the gate was unresolved. The nits were a restored token reviving
  its sessions (2); the id-length check untested, the one survivor of seven mutations (3); the
  v0.3.1 logout missing from `docs/running.md`, which said an upgrade does not log out, and the idle
  limit named in more places than the PR body listed (4); the unrecorded new id on refresh (5); and
  the missing link (6). The reviewer attacked the MAC with hostile values signed with the real key,
  measured rotation, host binding, refresh and the cookie attributes, ran the e2e suite against the
  v0.3.0 binary to watch it fail, and measured the implementer's Chromium table again. No capture
  was proposed.
- **The fix pass** added, on the branch, `829fd6c` (three id-length cases), `3621f42` and `1211ba6`
  (the introduction and `docs/running.md`) and `ecf9baf` (the man page), and the addendum Decision
  answered nits 2 and 5.
- **Round two, on PR #233**
  [approved](https://github.com/davison/md-notes/pull/233#issuecomment-5909610270), at `1211ba6`,
  after reverting the id-length check and watching all three new cases fail. The coordinator told
  the reviewer to leave finding 6 out of the verdict, because the operator was handling it. One nit,
  a line in the introduction ending early, was fixed in `cf8d515` after the approval.

No coordinator disposition comment covers either review. The addendum Decision and PR #233's body
answer the items that were not code.

## Deviations and corrections

**#231: no pull request could be linked by its closing keyword, and the task was closed by hand.**
See [Three pull requests](#three-pull-requests-and-a-task-github-would-not-link-or-close)
([#231](https://github.com/davison/md-notes/issues/231#issuecomment-5909597289),
[#231](https://github.com/davison/md-notes/issues/231#issuecomment-5910193024)).

**#231's plan said a refreshed cookie keeps its id.** Corrected by the addendum Decision, as above.
#231's plan still says "same id".

**PR #233's body still says the commits include "one `docs` for the prose".** There are four `docs`
commits on `main` by the end, from the fix pass and the reflow. The body was not edited for them.

## QA's findings, and what was done

QA posted three observations and a suite-gap finding outside the verdicts
([#230](https://github.com/davison/md-notes/issues/230#issuecomment-5910442763),
[#231](https://github.com/davison/md-notes/issues/231#issuecomment-5910421875)), and the coordinator
disposed of them ([#230](https://github.com/davison/md-notes/issues/230#issuecomment-5910459560)):

1. **The idle limit is 29 to 30 days after the last use.** No change; see [The human
   gate](#the-human-gate).
2. **The man page does not mention the one extra login at v0.3.1.** No change. The page stays free
   of versions, as round one on #232 accepted. The introduction and `docs/running.md` carry it, and
   the v0.3.1 release notes are to say it.
3. **Two promises no test holds.** Changing the key purpose in `internal/session` from `"mdn session
   key v1"` to `"mdn session key v1x"` passes every test, so nothing pins a cookie across builds:
   shipped, that edit would log every device out at the next upgrade. And nothing asserts that
   `--state` holds only the token file. Captured as
   [#235](https://github.com/davison/md-notes/issues/235), not a remedy task: the behaviour is
   correct and was measured directly, and v0.3.1 ships no further change to the session format.
4. **The operator's devices, and a real app install.** The phone and tablet check is the operator's
   after he installs v0.3.1. A real desktop install through the DevTools protocol fails behind the
   harness's self-signed certificate (`kNotValidManifestForWebApp`), so the installed-app leg was
   approximated with an `--app` window on the same profile; captured as
   [#236](https://github.com/davison/md-notes/issues/236).

**Rejected:** a remedy task for item 3 inside M12, which would delay v0.3.1 to guard against a
regression no pending change introduces.

## The front door in this pull request

The task brought the introduction's *A browser on the tailnet*, `docs/running.md` and
`contrib/mdn.1` up to date, and both reviews and QA checked them. The claims M12 changed elsewhere
in the front door, brought into line with `19823d0`:

- **`docs/introduction.md`.** The opening now describes the project at the end of milestone twelve,
  and a paragraph says what M12 changed. *What holds these numbers* counts 123 browser checks
  instead of 120, and says what the new suite drives and that it needs `openssl`.
- **`CONTRIBUTING.md`.** *What you need* lists `openssl` for the tailnet session suite, and the
  `make e2e` paragraph names that suite and counts twelve suites and 123 tests instead of eleven and
  120.

The README makes no claim about how long a login lasts, and no other guide makes a claim M12
changed. The e-ink and sync guides say a session is a cookie that survives closing the tab, which
stays true.

## Captures adopted, and captures raised

M12 adopted one capture, [#229](https://github.com/davison/md-notes/issues/229), closed by `task
finish 231` when #233 merged
([#229](https://github.com/davison/md-notes/issues/229#issuecomment-5910189341)).

It raised two, both at 11:41Z from QA's findings, both open:

- [#235](https://github.com/davison/md-notes/issues/235), a known-answer vector for the session
  format and a test that `--state` holds only the token file, best landed before the next change to
  the session code;
- [#236](https://github.com/davison/md-notes/issues/236), an e2e test that installs the app for real
  and checks it stays logged in.

It also raised one upstream,
[radiusred/gh-codecrew#386](https://github.com/radiusred/gh-codecrew/issues/386), for `task finish`
to check that the task closed. No review proposed a capture. The items left out of scope, listed
under [Scope](#scope-merge-authority-and-the-devices), stay open in the backlog.

## The release: v0.3.1, to be tagged

The plan in the release Decision
([#230](https://github.com/davison/md-notes/issues/230#issuecomment-5909079844)):

1. After `gh codecrew milestone close 12`, the coordinator pushes the annotated tag `v0.3.1` from
   `main`, on the operator's instruction.
2. `release.yml` drafts the Release.
3. Publishing the draft stays the operator's, per the decision on #133: a person reviews the notes
   and presses Publish, which starts the AUR and `.deb` channels.

It is a patch release because the change fixes behaviour and adds no feature surface, and no M12
commit is `feat`. The release notes are to say that every device logs in once more after the upgrade
([#230](https://github.com/davison/md-notes/issues/230#issuecomment-5910459560), item 2). When this
record was written, `v0.3.0` was the newest tag and `v0.3.1` did not exist. After the release, the
operator checks his phone and e-ink tablet, per the scope Decision.

## Where the record is silent

**The operator's words were attributed late.** Every seat posts under the one account, so the
account alone does not say who typed a comment. The coordinator's Decision saying where the
operator's words came from was posted at 11:48:29Z, after QA and at record time, and says itself
that it "should have been posted as the words were relayed"
([#230](https://github.com/davison/md-notes/issues/230#issuecomment-5910571213)). On M11 the same
comment also came after QA
([#213](https://github.com/davison/md-notes/issues/213#issuecomment-5900867536)). The M12 Decision
records:

- **Typed by the operator on GitHub:** the gate resolution on #231 and the label's removal, and the
  manual link of #233 to #231. He confirmed each in the coordinator session;
- **Relayed by the coordinator from the session:** the capture request behind #229; the scope, typed
  "ov v0.3.1" and quoted by the scope Decision with "of"; and "yes, file the upstream capture for
  checking that a close happened", behind radiusred/gh-codecrew#386;
- **Posted by `gh codecrew`:** the operator-confirmation comment on PR #233.

The session those words came from is not on GitHub, so the relayed words are quoted as the
coordinator gave them.

**One approval did not see its last commit.** See [What shipped](#what-shipped). It was the
approving review's own nit, and QA's verdicts ran on `main` after it.
