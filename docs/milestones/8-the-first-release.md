# M8 — The first release: a tag, two daemon channels and a release page

Tracking issue: [#133](https://github.com/davison/md-notes/issues/133). Its
implementation tasks are merged on `main` at
[`2c64b22`](https://github.com/davison/md-notes/commit/2c64b22), which is also the
commit the `v0.1.0` tag names, so the tree that was reviewed and the tree the release
was built from are the same one.

The release itself is
[v0.1.0](https://github.com/davison/md-notes/releases/tag/v0.1.0), tagged by the
operator on 2026-09-19 and published by them at 18:11:59Z.

Independent QA ran against the published artefacts rather than a working tree and found
M8-R1, M8-R3, M8-R4, M8-R6, M8-R7, M8-R9 and M8-R10 satisfied, M8-R5 **not** satisfied on
two false claims in `CONTRIBUTING.md`, and M8-R8 untestable until this record merges
([#133](https://github.com/davison/md-notes/issues/133#issuecomment-5744412141)).
M8-R2 was struck before the release and carries no verdict; M8-R11 was added after it and
has its own task still open. Both of M8-R5's claims are corrected in the pull request
that carries this record, on the coordinator's disposition
([#133](https://github.com/davison/md-notes/issues/133#issuecomment-5744425314)).

## Goal and outcome

Seven milestones in, nothing had ever been tagged or published. The daemon was built
from source with `make install` and the extension was loaded unpacked out of a working
copy, which is a fine arrangement for the person who wrote it and no arrangement at all
for anybody else. The operator asked for three publishing channels — the extension on
the Chrome Web Store, the daemon on the Arch User Repository, and a `.deb` — each
updated as part of a release, plus a `CONTRIBUTING.md` and whichever backlog captures a
first public release should not ship with
([#133](https://github.com/davison/md-notes/issues/133#issuecomment-5733256158)).

None of the three could publish without a release, and no release had ever existed, so
the release workflow went first and the channels hung off its `release: published`
event. The milestone grew three requirements after it opened — M8-R9 (dependencies
current and scanned), M8-R10 (the middle-width layout and themed scrollbars) and M8-R11
(`make install` only installs) — and lost one: M8-R2, the Chrome Web Store, was struck
four hours before the tag.

What a stranger can do now that they could not before: install the daemon with
`paru -S md-notes-bin` on Arch or `apt install ./md-notes_0.1.0_amd64.deb` on Debian and
Ubuntu, take the browser extension as a zip from the release page, and check any of it
against `SHA256SUMS`. What they still cannot do is have the extension update itself,
which is what the withdrawn channel would have bought.

### What shipped, in the order it merged

**A watch failure is reported by its own cause** ([#140](https://github.com/davison/md-notes/issues/140),
[PR #142](https://github.com/davison/md-notes/pull/142), merged at
[`9140f61`](https://github.com/davison/md-notes/commit/9140f61)). A directory the daemon
may not read used to be counted as the kernel's watch budget and answered with a `sysctl`
that could not help. `watch.Coverage` gained `Refused` beside `Failed` and a `Reason`
carrying the operating system's own words; the log line, the `status` event and the
notice above the navigator each name the cause and offer access to that directory rather
than a limit to raise. Adopts [#33](https://github.com/davison/md-notes/issues/33), from
M2's QA.

**A note cannot forge the scroll anchor, and the throttle says what it does**
([#139](https://github.com/davison/md-notes/issues/139),
[PR #144](https://github.com/davison/md-notes/pull/144), merged at
[`b96b411`](https://github.com/davison/md-notes/commit/b96b411)). Raw HTML a note writes
is stripped of `data-line` and of the renderer's structural class names before the
sanitiser sees it, so a note cannot plant a decoy for a search hit to scroll to; the
login throttle's comment now describes the per-request floor it actually implements
rather than a daemon-wide gate it does not; and the README names the six proxy headers
the tailnet backstop checks. Adopts
[#31](https://github.com/davison/md-notes/issues/31) and
[#48](https://github.com/davison/md-notes/issues/48).

**A versioned release from a tag** ([#134](https://github.com/davison/md-notes/issues/134),
[PR #143](https://github.com/davison/md-notes/pull/143), merged at
[`e16013f`](https://github.com/davison/md-notes/commit/e16013f)). One version, and it is
the tag: the binary takes it at link time, the extension manifest is stamped at build
time from the same value and carries no committed literal, and `scripts/relcheck`
refuses a release whose three answers disagree. `make release VERSION=v0.1.0` builds
everything the release publishes, so the workflow's build step is one line calling it.
`.github/workflows/release.yml` checks the commit by *calling* `ci.yml`, builds, and
leaves a **draft** Release for a person to publish. Adopts
[#129](https://github.com/davison/md-notes/issues/129).

**Dependencies current and scanned** ([#147](https://github.com/davison/md-notes/issues/147),
[PR #148](https://github.com/davison/md-notes/pull/148), merged at
[`86e1c57`](https://github.com/davison/md-notes/commit/86e1c57)). `golang.org/x/net`
v0.26.0 → v0.59.0 takes the code past GO-2025-3595, which `govulncheck` reported as
reached through the sanitiser on every render; both lockfiles take their updates
including four majors; chroma is held at v2.2.0 with the reason in `go.mod`; and
`make vuln` runs as a step of CI's `check` job, so the next published vulnerability
fails a build rather than waiting to be noticed. Adopts
[#146](https://github.com/davison/md-notes/issues/146).

**A contributing guide** ([#138](https://github.com/davison/md-notes/issues/138),
[PR #150](https://github.com/davison/md-notes/pull/150), merged at
[`2ceade5`](https://github.com/davison/md-notes/commit/2ceade5)). Nine sections for
someone who has just cloned the repository, every command in it run before it was
written down, linking `docs/releasing.md`, the README and `docs/milestones/` rather than
restating them.

**A Debian package, and a system-wide `make install`**
([#137](https://github.com/davison/md-notes/issues/137),
[PR #154](https://github.com/davison/md-notes/pull/154), merged at
[`1940f58`](https://github.com/davison/md-notes/commit/1940f58)). `md-notes_<version>_<arch>.deb`
for amd64 and arm64, built by a pinned nfpm from the release's own binaries, carrying
`/usr/bin/mdn`, the unit, the licence, a manual page and a changelog, depending on
ripgrep, and uploaded to the published release by `publish-deb.yml`. Under the gate
resolution it also carried the shared change the AUR task needed: `contrib/mdn.service`
names `/usr/bin/mdn`, and `make install` defaults to `PREFIX=/usr`. Adopts
[#132](https://github.com/davison/md-notes/issues/132).

**The side pane under the navigator at middle widths, and themed scrollbars**
([#158](https://github.com/davison/md-notes/issues/158),
[PR #159](https://github.com/davison/md-notes/pull/159), merged at
[`ec328b3`](https://github.com/davison/md-notes/commit/ec328b3)). Between the 960 px
drawer breakpoint and 1290 px the search and tag pane sits under the navigator so the
note keeps its 48rem reading width; from 1290 px the three-column layout applies as
before. Every scrollbar in the application, the editor's scroller and the tailnet login
page included, is thin and drawn from the theme's palette in both schemes and under the
e-ink override. Adopts [#156](https://github.com/davison/md-notes/issues/156) and
[#157](https://github.com/davison/md-notes/issues/157).

**The AUR package** ([#136](https://github.com/davison/md-notes/issues/136),
[PR #152](https://github.com/davison/md-notes/pull/152), merged at
[`2c64b22`](https://github.com/davison/md-notes/commit/2c64b22)). `md-notes-bin`, a
`-bin` package installing the binaries a release publishes, its `PKGBUILD` and `.SRCINFO`
rendered by `scripts/aurgen` from the release's own `SHA256SUMS`, built, linted,
installed and removed in an `archlinux:latest` container before anything leaves the
runner, then pushed to the AUR by `publish-aur.yml`. Adopts
[#131](https://github.com/davison/md-notes/issues/131).

**This record and the documentation** ([#141](https://github.com/davison/md-notes/issues/141),
[PR #165](https://github.com/davison/md-notes/pull/165)).

### And then the release

The operator pushed `v0.1.0`. The release workflow
([run 35460122576](https://github.com/davison/md-notes/actions/runs/35460122576))
checked the commit, built both binaries and the extension zip, wrote `SHA256SUMS` and
drafted the Release. The operator read the generated notes and pressed Publish at
18:11:59Z. Two seconds later both channels started:
[publish-aur](https://github.com/davison/md-notes/actions/runs/35460432264) pushed
`md-notes-bin` 0.1.0-1 to the AUR, whose `master` is at `5e93e1b`, and
[publish-deb](https://github.com/davison/md-notes/actions/runs/35460432286) built and
uploaded both packages. The coordinator then re-ran `publish-deb` from the Actions API,
which is M8-R1's re-run check: attempt 2 succeeded and `--clobber` replaced both `.deb`
assets in place while the four release-workflow assets went untouched
([#133](https://github.com/davison/md-notes/issues/133#issuecomment-5744270873)).

Two things the release showed that nothing had predicted. The generated notes listed
every pull request in the repository's history, because there was no earlier tag to span
from — one-off, and the reason commit subjects are worth writing well. And the run
carried three "Node.js 20 is deprecated" warnings and three notices that `ubuntu-latest`
moves to Ubuntu 26 on 2026-10-19, neither affecting the release and both captured as
[#161](https://github.com/davison/md-notes/issues/161).

### The channel that did not ship

M8-R2 asked for the extension on the Chrome Web Store, published by a workflow on every
release. The operator registered the developer account, paid the fee, created the item —
its id is recorded on the task
([#135](https://github.com/davison/md-notes/issues/135#issuecomment-5742368826)) — and
worked the console path as far as minting a refresh token. There the requirement met
something no amount of reading had shown: a long-lived Web Store API token needs the
OAuth app's publishing status set to *In production*, and Google will not move an
*External* app out of *Testing* until its consent screen carries a home page and a
privacy policy on an **authorised domain** the operator proves they own through Search
Console. `github.com` cannot be one. The alternative, an app left in *Testing*, means a
refresh token that expires every seven days and a hand step before every release — which
is the thing the channel existed to remove.

So the channel was withdrawn in the negative, and M8-R2 struck rather than narrowed:
nothing of it is delivered, no listing, no submission, no workflow
([#135](https://github.com/davison/md-notes/issues/135#issuecomment-5744118645),
[#133](https://github.com/davison/md-notes/issues/133#issuecomment-5744125602)). Task
[#135](https://github.com/davison/md-notes/issues/135) closed as not planned,
[PR #151](https://github.com/davison/md-notes/pull/151) closed unmerged, and the
`CHROME_WEBSTORE_*` secrets came out of the repository. What the extension has instead is
the release workflow's own `mdn-extension-<tag>.zip`, downloaded, unzipped and loaded
unpacked; there is no auto-update, and that was accepted rather than overlooked. What the
closed pull request is worth keeping regardless of a store — the privacy statement, the
permission justifications and the screenshot generator — is captured as
[#160](https://github.com/davison/md-notes/issues/160).

The milestone's own goal statement still says "publishes the extension to the Chrome Web
Store". Per the sealed-record convention the issue body was not rewritten; the decision
comment is the correction, and this is the record of it.

## Requirement outcomes

The verdicts are from the independent QA comment on the milestone issue
([#133](https://github.com/davison/md-notes/issues/133#issuecomment-5744412141)), run
against the **published artefacts** rather than a working tree — the six assets
downloaded fresh, the AUR repository cloned, the two `.deb`s installed in containers, the
extension zip loaded unpacked into Chromium inside an unprivileged network namespace so
the manifest's own `http://localhost:7337` origin was free and the operator's daemon
unreachable. The floor, which QA is explicit is the floor and not the evidence, is that
all three suites are green on `2c64b22`: `make check` exit 0, `make e2e` 8 suites and 61
tests, the extension's suite 3 suites and 28 tests.

| ID | Requirement | What the work established | QA |
|----|-------------|---------------------------|----|
| M8-R1 | A versioned release from a tag: CI on the commit, both static binaries, the extension zip, `SHA256SUMS`, one version checked in three places, and channels on `release: published` re-runnable from the Actions page | The tag is the only version; the manifest carries no literal and `relcheck` refuses a mismatch; `make release` builds locally what the workflow builds; the Release is drafted for a person to publish, which is what makes the channel event fire at all ([#134](https://github.com/davison/md-notes/issues/134#issuecomment-5734230707)) | [Satisfied](https://github.com/davison/md-notes/issues/133#issuecomment-5744412141) — all six assets downloaded, `sha256sum -c` 3/3, `mdn version` `v0.1.0`, the arm64 binary read through its build metadata (`GOARCH=arm64`, `CGO_ENABLED=0`, `-trimpath`), `relcheck` made to fail on a forced mismatch, the `publish-deb` re-run exercised, and the release zip loaded unpacked into Chromium with both clip actions writing real notes |
| M8-R2 | The extension on the Chrome Web Store | **Struck** ([#133](https://github.com/davison/md-notes/issues/133#issuecomment-5744125602)). Nothing delivered; the reasoning is the gate resolution on [#135](https://github.com/davison/md-notes/issues/135#issuecomment-5744118645) | No verdict — the requirement was struck before QA ran |
| M8-R3 | The Arch User Repository: a named package, ripgrep, the unit, the licence, namcap clean, `PKGBUILD` in this repository, pushed by a workflow, verified from the published package | `md-notes-bin`, prebuilt, rendered from the release's own checksums and verified in a container before the push ([#136](https://github.com/davison/md-notes/issues/136#issuecomment-5734587464)) | [Satisfied](https://github.com/davison/md-notes/issues/133#issuecomment-5744412141), with one thing the environment forbade — QA cloned the AUR repository, found the pushed `PKGBUILD` and `.SRCINFO` **byte-identical** to a local render, built it with `makepkg`, and ran `namcap`, `pacman -U`, the unit and `pacman -R` in a container. An AUR helper needs root, which QA may not use; `makepkg` plus the `pacman -U` the helper itself ends with is as close as the host allows |
| M8-R4 | A `.deb` for amd64 and arm64 from the same version source, the three files, `Depends: ripgrep`, lintian clean, uploaded on release | nfpm 2.47.0 pinned and checksum-checked, one config for both architectures, three lintian findings fixed and one overridden with its justification installed in the package ([#137](https://github.com/davison/md-notes/issues/137#issuecomment-5734675114)) | [Satisfied](https://github.com/davison/md-notes/issues/133#issuecomment-5744412141) — installed in `debian:stable` and `ubuntu:24.04`, ripgrep pulled in, `systemd-analyze --user verify` exit 0, lintian exit 0 on both architectures on both distributions, `apt remove` leaving none of the 18 paths, and the packaged binary byte-identical to the release asset |
| M8-R5 | A contributing guide covering build, test, the browser suites, the commit convention, the CodeCrew flow, how to report a bug, and how a release is cut and what publishes after the tag | Nine sections, every command run before it was written ([#138](https://github.com/davison/md-notes/issues/138#issuecomment-5734490063)) | **[Not satisfied](https://github.com/davison/md-notes/issues/133#issuecomment-5744412141)** — every command still runs as written and all four documented failure modes reproduce, but two claims had been overtaken: the release section named the struck Chrome Web Store as one of three channels and said none had published when two had, and `make e2e` was given as 54 tests where it is 61 ([#138](https://github.com/davison/md-notes/issues/138#issuecomment-5744398417)). Both are corrected in the pull request carrying this record, on the coordinator's disposition ([#133](https://github.com/davison/md-notes/issues/133#issuecomment-5744425314)); a superseding verdict is owed after it merges |
| M8-R6 | Claims and confinement before strangers run it: no forged scroll target, a throttle comment that is true, a README that names the headers | The forgery needed `data-line`, not the class the capture named, so the scrub takes both before the sanitiser runs; the throttle comment was narrowed rather than the code serialised, because serialising converts a burst into a backlog ([#139](https://github.com/davison/md-notes/issues/139#issuecomment-5733355532)) | [Satisfied](https://github.com/davison/md-notes/issues/133#issuecomment-5744412141) — thirteen decoy spellings against the **downloaded release binary**, including the entity and hex forms, upper case, single quotes and a self-closing tag, all stripped while the renderer's own markers survive; the six proxy headers in the README match `proxyMarkers` in order |
| M8-R7 | A watch failure reported by its cause in the log, the status event and the notice, with the introduction naming the third cause | Three scalars on a comparable `Coverage` rather than a per-directory list, and the kernel limit named in the negative because the wrong remedy is what the old line gave ([#140](https://github.com/davison/md-notes/issues/140#issuecomment-5733373364)) | [Satisfied](https://github.com/davison/md-notes/issues/133#issuecomment-5744412141) — a mode-`000` subdirectory under the downloaded release binary, seen through all three surfaces at once: the log line with no `sysctl` in it, `"failed":0,"refused":1,"reason":"permission denied"` on the event stream, and the notice in Chromium, with the negative asserted too |
| M8-R8 | Documentation and record: the README's installation section, the extension and sync pages, the roadmap row, and this record | This task; the pull request carrying this record is what delivers it | [Untestable at the verdict](https://github.com/davison/md-notes/issues/133#issuecomment-5744412141) — QA graded it against `main` before this task had a pull request, where none of it exists. A superseding verdict is owed once it merges. The two things QA named for whoever wrote it — `docs/releasing.md`'s store row and its four-asset table — are both done |
| M8-R9 | No reachable known vulnerability, every direct dependency current or held back with a reason, lockfiles updated, scanners in CI | The vulnerable module was taken to its current release rather than to the first fixed one; chroma alone is held, with fifteen lines in `go.mod` saying which seven token types v2.27.0 stops styling ([#147](https://github.com/davison/md-notes/issues/147#issuecomment-5734015503)) | [Satisfied](https://github.com/davison/md-notes/issues/133#issuecomment-5744412141) — `make vuln` exit 0 on the tagged tree, and the **published arm64 binary's own build metadata** records `golang.org/x/net v0.59.0`, so the artefact on the page is the scanned one; the release run's `check` job shows the scan step succeeding |
| M8-R10 | The side pane under the navigator between the drawer breakpoint and the three-column width, the reading width kept, and thin themed scrollbars in both schemes and under the e-ink override | The threshold is 1290 px, computed from the stylesheet's own widths and confirmed either side of it; the scrollbar colours were chosen against WCAG 1.4.11 rather than by eye ([#158](https://github.com/davison/md-notes/issues/158#issuecomment-5742041075), [#158](https://github.com/davison/md-notes/issues/158#issuecomment-5742043518)) | [Satisfied](https://github.com/davison/md-notes/issues/133#issuecomment-5744412141) — measured against the release binary's own embedded UI, walking the viewport, and with Chromium launched **without** `--hide-scrollbars`, which the shipped suite cannot do for itself; the recorded table reproduces to the pixel, and the scrollbar properties compute on `.cm-scroller` in all four states including a coarse pointer |
| M8-R11 | `make install` only installs, and refuses when what it would install is not built; `make clean` only cleans | Added after the release, on the operator's first `sudo make install` of the v0.1.0 tree ([#133](https://github.com/davison/md-notes/issues/133#issuecomment-5744342213)) | No verdict in this pass by design; task [#164](https://github.com/davison/md-notes/issues/164) is open and gets its own when its pull request merges |

## The human gates

Four, each raised by the work that needed it and each resolved on the record before that
work went on.

**The release event, on the milestone** ([#133](https://github.com/davison/md-notes/issues/133#issuecomment-5733576397)).
The review of [PR #143](https://github.com/davison/md-notes/pull/143#issuecomment-5733563374)
found that GitHub does not start workflow runs from events caused by a workflow's own
`GITHUB_TOKEN`, so a Release the release workflow published would have fired nothing and
all three channels would have sat silent. Three ways out were offered: a draft the
operator publishes, a personal access token in a secret, or reusable workflows called
directly. The operator took the first —
*"option (a) I'll review and click-ops the release"*
([#133](https://github.com/davison/md-notes/issues/133#issuecomment-5734033208)) — which
costs one click per release and buys a human reading the generated notes before they are
public, with no new credential in the repository.

**The Chrome Web Store account and its credentials, on #135**
([#135](https://github.com/davison/md-notes/issues/135#issuecomment-5734417175), amended
at [#135](https://github.com/davison/md-notes/issues/135#issuecomment-5736558097) when
the client moved to the V2 API and needed a publisher ID the original question never
asked for). Resolved **in the negative** on 2026-09-19
([#135](https://github.com/davison/md-notes/issues/135#issuecomment-5744118645)): the
channel is withdrawn, for the reasons in [The channel that did not ship](#the-channel-that-did-not-ship).

**The AUR account, deploy key and maintainer line, on #136**
([#136](https://github.com/davison/md-notes/issues/136#issuecomment-5734468889)).
Resolved: *"private key added as a secret. Use name/email plain without approval
environment"*
([#136](https://github.com/davison/md-notes/issues/136#issuecomment-5734862923)). So
`packaging/aur/MAINTAINER` carries `Darren Davison <darren@davisononline.org>` unobfuscated
— the same identity the workflow authors the AUR commits as, which the AUR records and
cannot change afterwards — and no GitHub Environment approval stands in front of the push
([#136](https://github.com/davison/md-notes/issues/136#issuecomment-5734915594)).

**The unit's `ExecStart`, on #137** ([#137](https://github.com/davison/md-notes/issues/137#issuecomment-5734444178)).
M8-R4 asked the package to install `contrib/mdn.service` *and* for `systemd-analyze
verify` to accept it, and measurement showed both could not be true of the same file: the
committed unit ran `%h/.local/bin/mdn`, which a package user does not have. The gate laid
out four options with the measurements beside them. The operator answered **(b)**, and
widened it: *"The installer should work for a system wide install by default. Update the
`make install` target and remove the default PREFIX (or set it to `/usr`) … Update all
docs to match this decision"*
([#137](https://github.com/davison/md-notes/issues/137#issuecomment-5741912474)).

That answer is the one worth pausing on, because it dissolved the conflict rather than
working around it. With `make install` and both packages putting the binary in the same
place, the unit names one path, both of M8-R4's clauses come true, and there is nothing
left for the packaging to substitute. The AUR package's `package()` rewrite — and a
review finding about its fidelity — went away rather than being fixed
([#136](https://github.com/davison/md-notes/issues/136#issuecomment-5742177631)). The
coordinator sequenced it so that [PR #154](https://github.com/davison/md-notes/pull/154)
carried the shared change and [PR #152](https://github.com/davison/md-notes/pull/152)
rebased onto it
([#133](https://github.com/davison/md-notes/issues/133#issuecomment-5741920998)).

## Decisions

### There is one version, and it is the git tag

`extension/public/manifest.json` lost its `version` key rather than keeping a
placeholder. The build writes the version into `extension/dist/manifest.json` from
`MDN_VERSION`, which `make release` fills from the same `VERSION` the binary is linked
with, and `extension/src/manifest.test.ts` asserts the key stays absent so a literal
cannot come back unnoticed
([#134](https://github.com/davison/md-notes/issues/134#issuecomment-5733379505)).

**Rejected:** a placeholder literal such as `0.0.0` in the committed manifest. It would
have been a smaller diff and kept the committed manifest loadable on its own, at the cost
of being a second version that a build failing to stamp would ship silently — the exact
failure M8-R1 asks to be impossible. With the key absent, an unstamped manifest is one
Chromium refuses to load and one `relcheck` names.

A release tag is normalised for the manifest (`v0.1.0` → `0.1.0`) and anything that is
not a clean tag becomes `0.0.0`: numeric, so it loads, and obviously not a release. The
decision recorded the Chrome Web Store as the reason for the dotted-integer shape; with
no store the constraint survives its stated reason, because it is Chromium's own —
`chrome --pack-extension` over the release's extension with `v0.1.0` in its manifest
answers *"Required value 'version' is missing or invalid. It must be between 1-4
dot-separated integers each between 0 and 65536."*
`docs/releasing.md` now attributes it there
([#141](https://github.com/davison/md-notes/issues/141#issuecomment-5744380512)).

### The build is a `make` target and the workflow is thin

`make release VERSION=v0.1.0` produces everything the release publishes and runs the
version check before it writes the checksums; the workflow's build step is one line
calling it. So every step of a release can be proven on a laptop without spending a tag,
and a release that fails in CI fails the same way locally
([#134](https://github.com/davison/md-notes/issues/134#issuecomment-5733382357)).

Two riders from the same comment. **Only `linux/amd64` and `linux/arm64`** — the machine
the daemon runs on and a Raspberry Pi class peer. Darwin would cost nothing to add and
nothing in this project has ever been run on one; an untested asset on a release page is a
claim, so it waits for someone who wants it. **Bare binaries, not tarballs** — the licence
and `contrib/mdn.service` are in the tagged source tree, which is what both packaging
workflows check out anyway, so an archive would add a layer for them to unwrap and give a
downloading human a file to extract for no gain.

### The release checks its commit by calling CI, not by restating it

`release.yml` has `jobs.check.uses: ./.github/workflows/ci.yml`, and `ci.yml` gained a
`workflow_call` trigger. M8-R1 asks the release to check the commit "the way CI does";
copying CI's steps into a second file would satisfy that on the day it was written and
quietly stop satisfying it the first time CI changed
([#134](https://github.com/davison/md-notes/issues/134#issuecomment-5733385373)). The
same comment settles that the notes are GitHub's own, with no third-party action to pin
or audit, and that `workflow_dispatch` with a version input is always a dry run: same
checks, same build, artefact uploaded, no Release at all.

### A re-run deletes the Release first; the tag is never deleted

`gh release create` has no update mode and refuses a tag that already has a published
Release, so a run that died after the Release existed cannot simply be re-run. The
recovery is to delete the Release and re-run
([#134](https://github.com/davison/md-notes/issues/134#issuecomment-5733643578)).

**Rejected:** making the publish step idempotent, with `gh release delete --yes` or
`--clobber` in front of it. Either turns a re-run into something that silently replaces a
published release's assets — and a published release is the thing the channel workflows
have already been told about. It should take a human deleting it, which is one command
and a moment's thought about who has already downloaded what.

The tag stays in every recovery. It names the commit the assets were built from;
deleting and re-pushing it would build a different tree under a name someone may already
have fetched. That is written down because it is the tempting wrong move at exactly the
moment nobody is thinking clearly.

### The tag drafts; the operator publishes

Taken under the gate above, and not the seat's to have taken otherwise, since it changes
the shape of what M8-R1 delivers
([#134](https://github.com/davison/md-notes/issues/134#issuecomment-5734230707)).
`gh release create` gained `--draft`, keeping `--generate-notes` and `--verify-tag`.

**Rejected:** a personal access token or App token in a repository secret. It would have
been fully automatic from the tag push, at the cost of a credential with `contents: write`
to mint now and rotate within a year, for a project whose entire release cadence is one
person pushing a tag.

### The AUR package is `md-notes-bin`, because `mdn` is taken

Checked on 2026-09-18 and again immediately before the plan was written: `mdn` is taken
on the AUR by an unrelated project, `md-notes`, `md-notes-bin` and `mdnotes` are free, and
none of the four is in the official repositories
([#136](https://github.com/davison/md-notes/issues/136#issuecomment-5733549614),
[#136](https://github.com/davison/md-notes/issues/136#issuecomment-5734587464)). The
binary keeps the name `mdn`; the package cannot. The `.deb` took `md-notes` to match
([#137](https://github.com/davison/md-notes/issues/137#issuecomment-5733553184)).

**`-bin` rather than a source package**, for two reasons that are not preferences. The
submission guidelines *require* the suffix for a package built from prebuilt
deliverables, and M8-R3 asks for a package "templated on the release version and
checksum", which is the shape of a `-bin` package: the workflow reads the checksums out
of the release's own asset, so the package can only ever point at what was published. A
source package could not honour the guidelines from this tree anyway — the UI bundle is
embedded in the binary, so `PKGBUILD` would have to run `pnpm install` inside the build
and neither `node_modules` nor a pnpm store is vendored. `md-notes` — no suffix — is
reserved by the same decision for a source package that nothing here provides yet, and
the `-bin` package `provides` and `conflicts` with that name so the two could never be
installed together.

### The AUR push has no approval environment, and only `release: published` starts it

The checklist proposed a GitHub Environment with the operator as required reviewer; the
gate resolution turned it off
([#136](https://github.com/davison/md-notes/issues/136#issuecomment-5734597148)). The
publish click is already the human review point and is the only thing that starts the
workflow; behind it the run renders from the release's own checksums, builds, lints,
installs and removes the package in a container, and refuses outright to push a
`PKGBUILD` still carrying `SKIP` checksums or the placeholder maintainer line. A second
approval would be a second reading of a diff whose interesting half is derived rather
than written. **The trade-off is stated rather than hidden:** if a rendering ever goes
wrong in a way the container build does not catch, the AUR gets it without a second look,
and the job carries a comment naming the one line that would switch the environment on.

No `workflow_dispatch` on either channel. M8-R1 wants a channel re-runnable from the
Actions page without a new tag, and "Re-run jobs" on the run does exactly that against
the same release; a dispatch input would be a second way in that could name a release the
run was not started for. The first release proved the point the intended way, with the
`publish-deb` re-run.

### nfpm, pinned and checksum-checked

M8-R4 left the tool open. nfpm 2.47.0 was taken for one configuration serving both
architectures with no Debian toolchain on the runner, for control fields living in one
file a test can read, and because it is one static binary with no dependencies of its own
([#137](https://github.com/davison/md-notes/issues/137#issuecomment-5734674785)).

**The trade-off is real and was answered rather than accepted:** a third-party program
assembles the package a stranger installs as root, fetched over the network at run time.
So it is pinned to an exact version and its tarball checked against the digest goreleaser
publishes for that release before it is unpacked — which the review of PR #154 verified
independently by downloading the same tarball and the same `checksums.txt`.

Alongside it, the control fields and the lintian handling
([#137](https://github.com/davison/md-notes/issues/137#issuecomment-5734674955),
[#137](https://github.com/davison/md-notes/issues/137#issuecomment-5734675114)): three
findings fixed — a changelog and a manual page written rather than waved away — and one,
`statically-linked-binary`, overridden **with its justification installed in the package**
at `/usr/share/lintian/overrides/md-notes`, so anyone who runs lintian on it reads the
reason in the same output as the tag.

### chroma is held at v2.2.0, and the hold-back is the requirement's own provision

v2.27.0's `github` style stops colouring seven token types, which the M4 dark-scheme
guard names by hand; taking it would have meant editing that guard and quietly narrowing
a promise M4 made. The comparison was run both ways — fifteen languages rendered through
the renderer itself, and the generated stylesheet regenerated — and its HTML half is
genuinely better under v2.27.0, which is recorded rather than suppressed
([#147](https://github.com/davison/md-notes/issues/147#issuecomment-5734015503)). Twenty
five minor versions of lexer fixes are the price of seven coloured token types, and the
reason lives in `go.mod` beside the version, where the next `go get -u` will meet it.

A related question was raised by the review of PR #148 and answered rather than assumed:
this was **not** a human gate. The task's plan named a gate for *landing* an upgrade that
narrows an M4 promise; that branch was not taken, and M8-R9 writes the other one out — "or
the hold-back is recorded with a reason". A recorded hold-back is the requirement's own
provision, not a deviation from it
([#147](https://github.com/davison/md-notes/issues/147#issuecomment-5734295364)).

### The scan is its own step of the `check` job, not a prerequisite of `make check`

`make vuln` runs `govulncheck` and `pnpm audit`; CI runs it as a step after `make check`
([#147](https://github.com/davison/md-notes/issues/147#issuecomment-5734072606)). Folding
it into `make check` would put a check whose answer changes *without the tree changing* in
front of every local build, and would fail on a train. Kept separate, a red scan reads as
"something was published" rather than "the build broke".

### The scroll anchor is confined by scrubbing raw HTML, not by renaming the classes

Both shapes the capture offered, and M8-R6's first branch, read as though the forgery
needs the `line-anchor` class. It does not: `lineTarget` selects on `[data-line]` alone
and reads the class only to decide which element flashes. So the scrub takes `data-line`
and the structural names off note-authored HTML before the sanitiser runs, rather than
renaming the renderer's classes into an `mdn-` namespace across the stylesheet, the
renderer and their tests
([#139](https://github.com/davison/md-notes/issues/139#issuecomment-5733351023)).

**And the first version of that decision claimed more than the code delivered**, which is
recorded as a correction rather than edited away
([#139](https://github.com/davison/md-notes/issues/139#issuecomment-5733775405)). A fast
path decided whether a fragment was worth tokenising by scanning for the literal bytes of
those names — and an entity in the attribute value carries none of them, while the
tokeniser and the sanitiser behind it both decode it. `class="line&#45;anchor"` went
through untouched. The review of PR #144 measured it in the package and through the built
binary; the fast path was removed rather than patched. QA later aimed thirteen spellings
at it, entity and hex forms included, and none survived.

### The throttle's comment was narrowed; the code was not serialised

`loginDelay` claimed a caller "cannot run through attempts as fast as it can open
sockets", and a measurement from M3 showed 40 parallel wrong logins completing in 533 ms:
each request waits its 500 ms, but nothing queues them. The claim was about the daemon's
aggregate; the code bounds a request
([#139](https://github.com/davison/md-notes/issues/139#issuecomment-5733355532)).

**Rejected: a semaphore of one.** It would make the sentence true and the daemon worse. It
converts a burst into a backlog — 40 parallel failures stop being 533 ms of work and
become 20 seconds of held goroutines and file descriptors, and an attacker will not hang
up — and it reintroduces, in the time domain, the lockout the file already refuses to
create. The comment was made true instead, and says outright that nothing holds failures
in a queue.

### A third scalar on `Coverage`, not a per-directory error list

`watch.Coverage` is compared by value in two places, one of them on the daemon's per-tick
path, so a slice or map field would not compile without rewriting both into hand-written
comparisons. It gained `Refused int` and `Reason string`, and `Failed` narrowed to the one
refusal `fs.inotify.max_user_watches` answers
([#140](https://github.com/davison/md-notes/issues/140#issuecomment-5733373110)). The
package's standing choice against a list stands with it: one error stands for all of them,
so a root with hundreds of unwatchable directories logs one line and not hundreds.

The log line names the kernel limit **in the negative** — "not the kernel limit — the
daemon needs access to that directory" — deliberately, because the wrong remedy is what a
reader of the old line was given
([#140](https://github.com/davison/md-notes/issues/140#issuecomment-5733373364)). And no
`chmod` is offered: the daemon does not know whether the answer is a mode, an owner, an
ACL or leaving the directory out of the root
([#140](https://github.com/davison/md-notes/issues/140#issuecomment-5733373659)).

### The three-column threshold is 1290 px, written in pixels

Computed from the widths the stylesheet uses — a 16rem navigator, a 48rem reading width,
its 2rem gutters and an 18rem side pane, at the 15 px root font — and then confirmed in
Chromium at 1290 and at 1289
([#158](https://github.com/davison/md-notes/issues/158#issuecomment-5742041075)).

**The lesson in it is about `rem`, and it has caught this stylesheet before.** A media
query's `rem` is the *initial* 16 px, not the 15 px root the page sets — which is why the
existing `60rem` breakpoint is 960 px and not 900, a trap the M4 record already names.
86rem of application `rem` is not 86rem of media-query `rem`, so the query carries the
number the arithmetic comes to and a comment carries the sum.

Two things the same decision records as deliberate rather than overlooked. Between 961 and
1019 px the note column still cannot reach 780 px, and nothing can fix that without moving
either the drawer breakpoint or the reading width, which M8-R10 says stay. And the
threshold is the layout's arithmetic and does not include a scrollbar, so where a browser
reserves space for bars the rendered article is 710 px at exactly 1290
([#158](https://github.com/davison/md-notes/issues/158#issuecomment-5742203574)). QA
measured that independently against the release binary's own UI and reproduced the table
to the pixel
([#133](https://github.com/davison/md-notes/issues/133#issuecomment-5744403052),
observation 7). Folding a bar's width into a breakpoint would bake in a number that is
10 px in desktop Chromium and 0 on every platform with overlay scrollbars.

### The scrollbar colours were chosen for contrast, not by eye

One thumb-and-track pair per scheme, beside the existing palette tokens, each measured
against WCAG 1.4.11's 3:1 for a user-interface component — the e-ink panel with no
backlight being the device that most needs it. The track is a shade off `--pane` rather
than `--line`, because `--line` is the weight of a divider and too loud for a gutter that
runs the height of a pane
([#158](https://github.com/davison/md-notes/issues/158#issuecomment-5742043518)).

### The contributing guide explains the flow by obligation, not by verb

The CodeCrew section names no `gh codecrew` command and no role. A newcomer opening a pull
request never runs the CLI, and a list of verbs would read as "you need our tooling to
contribute", which is false. What they need is the six things the process asks of a
change, and the one piece of jargon they will actually meet — `refused[CODE]` — with the
instruction that matters: supply what it names, do not route around it
([#138](https://github.com/davison/md-notes/issues/138#issuecomment-5734494452)).

The guide's "open an issue first" line was softened after review, and the reason is worth
keeping. The original was a true statement of what the protocol requires of a *seat* — "no
task issue, no work" — extended to everyone, which is a policy about strangers that was
never recorded anywhere. It now asks for an issue for anything with a decision in it and
welcomes a typo or a dead link as a pull request on its own
([#138](https://github.com/davison/md-notes/issues/138#issuecomment-5734703429)).

### The store decisions, kept because they cost something to learn

The channel is gone, but three of its choices are the kind that will be faced again.

**The privacy policy was to be `docs/privacy.md`, linked at its blob URL**, because M8-R2
said "hosted from this repository" and this repository is public — GitHub already serves
the page rendered, at a stable address, with its history beside it. **Rejected:** GitHub
Pages, which would have meant a Pages source to configure and a second deployment that can
break independently of the thing it documents
([#135](https://github.com/davison/md-notes/issues/135#issuecomment-5734483710)). The
irony is that the gate's eventual answer turned on a privacy policy URL — on a domain the
operator owns, which is precisely what a blob URL is not.

**The publishing client was written here rather than taken from a package.** Three HTTP
calls with Node's own `fetch`, no dependency added, because this was the only job in the
repository holding credentials able to replace what every user of the extension has
installed — a third-party package in that job is a supply chain with write access to other
people's browsers
([#135](https://github.com/davison/md-notes/issues/135#issuecomment-5734581130)).

**It moved from the V1 API to V2 before it ever ran**, when a review found V1's own
reference announcing its retirement for October 2026. Doing it later would have meant
going back to the operator for a publisher ID after they had finished with the dashboard,
and the gate asking for four other values was still open
([#135](https://github.com/davison/md-notes/issues/135#issuecomment-5736561442)). The
migration was the right call on its own terms and bought nothing in the end, which is what
a withdrawn channel does to the work under it.

## Deviations and narrowings

**The scrollbar properties are declared on `*`, where the plan said `:root`.** The plan's
reason was that both properties inherit. Measured in the Chromium Playwright ships,
`scrollbar-color` does and `scrollbar-width` does not: on `:root` alone the root computes
`thin` and every pane computes `auto`, which is a full-width bar in the theme's colours.
Nothing the requirement says changes, so it was recorded as a deviation rather than raised
as a gate
([#158](https://github.com/davison/md-notes/issues/158#issuecomment-5742045723)).

**`CONTRIBUTING.md` gained `make release` and `make vuln`, which its plan had excluded.**
The release section could not explain that a tag need not be the first place a release is
tried without naming the command that proves it locally — and once the command was on the
page, the task's own rule applied to it, so it was run rather than cited. `make vuln`
landed on `main` from #147 while the task's commands were running; the branch was rebased
and every command re-run against the new base
([#138](https://github.com/davison/md-notes/issues/138#issuecomment-5734497750)).

**The release workflow's dry run cannot be exercised from a branch.** The plan said it
could. GitHub answers `HTTP 404: workflow release.yml not found on the default branch` for
a `workflow_dispatch` it cannot see there, whatever ref is asked for, so the first dry run
of a change to that workflow happens after it merges and before the tag is pushed
([#134](https://github.com/davison/md-notes/issues/134#issuecomment-5733385373)).

**`docs/releasing.md` had two things read from documentation rather than measured**, and
the first release settled one of them. The assets attached to the draft survive the
publish: they were created at 18:08:44Z, three minutes before the publish, and their
digests are the ones `SHA256SUMS` attests to and a download checks out against
([#133](https://github.com/davison/md-notes/issues/133#issuecomment-5744403052),
observation 5). What `gh release create` does when a draft already stands under the tag is
**still unmeasured** — the first release needed no re-run — and the page says so rather
than guessing.

**`SHA256SUMS` covers three files, not four.** The release note and this task's hand-off
both said four; the release workflow puts four *assets* on the draft and the fourth is
`SHA256SUMS` itself, which cannot carry its own digest. Found against the downloaded file
and corrected on both
([#141](https://github.com/davison/md-notes/issues/141#issuecomment-5744380512),
[#133](https://github.com/davison/md-notes/issues/133#issuecomment-5744388946)). The point
the hand-off was making is unchanged: the two `.deb`s are not covered by it.

**One recorded measurement is a tag stale.** The #137 decision records three
`I: spelling-error-in-binary` tags from lintian on `ubuntu:24.04`; QA measured two today,
`akS` having gone with a lintian data refresh. Nothing in the package changed and both
still exit 0
([#133](https://github.com/davison/md-notes/issues/133#issuecomment-5744403052),
observation 2).

## What the reviews changed

Every pull request was reviewed by a clean-context session under the reviewer contract and
merged under the operator's standing confirmation. Three of the reviews changed what the
milestone is, rather than tidying it.

**The review of [PR #143](https://github.com/davison/md-notes/pull/143#issuecomment-5733563374)
found that the whole milestone would not have run.** Its finding 1 is the `GITHUB_TOKEN`
rule: a Release created by the workflow starts no workflow run. Without it the tag would
have built, published, looked perfect, and every channel would have sat silent —
including M8-R1's own verification, which asks for a channel re-run and would have had no
run to re-run. That finding became the gate on #133 and the draft-plus-publish shape the
release now has.

**The review of [PR #144](https://github.com/davison/md-notes/pull/144#issuecomment-5733685846)
found the scrubber's fast path decoding entities**, which is the correction above: the
decision comment claimed a confinement the code did not have. The same review's remark
about the age of `golang.org/x/net` is what led the coordinator to run `govulncheck`, find
GO-2025-3595 reachable on every render, and add M8-R9 to an open milestone
([#133](https://github.com/davison/md-notes/issues/133#issuecomment-5733823680)). A note
about a dependency's age in a review of a sanitiser change turned into a requirement, a
task and a scanner in CI.

**The review of [PR #154](https://github.com/davison/md-notes/pull/154#issuecomment-5734885911)
measured two properties the pull request did not claim** and one caveat it had read rather
than measured. It verified that each package carries its own architecture's binary byte
for byte from the release, and that the build is reproducible under a fixed
`SOURCE_DATE_EPOCH` so a re-run re-uploads the same bytes; and it quoted
`gh release upload --help` on `--clobber` deleting existing assets *before* uploading, so
a failed upload loses both. That caveat was handed to this task and is now on
`docs/releasing.md` — written from a measurement rather than from the manual, since the
`publish-deb` re-run exercised it for real.

[PR #152](https://github.com/davison/md-notes/pull/152) took three review passes, the last
of them on the rebase that the #137 gate resolution forced, and each pass got a focused
re-review of the delta rather than a fresh read.
[PR #151](https://github.com/davison/md-notes/pull/151), the store work, was reviewed
twice and closed unmerged; its second review is the one that found the V1 API's
retirement, which produced a migration that was then withdrawn with the channel.

## Captures adopted, and captures raised

Adopted and closed by this milestone:

| Capture | Adopted by | What it was |
|---------|-----------|-------------|
| [#31](https://github.com/davison/md-notes/issues/31) | [#139](https://github.com/davison/md-notes/issues/139) | A note's own HTML could carry the renderer's scroll marker and take a search hit's scroll |
| [#33](https://github.com/davison/md-notes/issues/33) | [#140](https://github.com/davison/md-notes/issues/140) | A watch failure with any other cause was reported as the kernel limit, with a `sysctl` that could not help. From M2's QA |
| [#48](https://github.com/davison/md-notes/issues/48) | [#139](https://github.com/davison/md-notes/issues/139) | The login throttle's comment and a README line overclaimed |
| [#129](https://github.com/davison/md-notes/issues/129) | [#134](https://github.com/davison/md-notes/issues/134) | No release had ever been cut |
| [#131](https://github.com/davison/md-notes/issues/131) | [#136](https://github.com/davison/md-notes/issues/136) | No AUR package |
| [#132](https://github.com/davison/md-notes/issues/132) | [#137](https://github.com/davison/md-notes/issues/137) | No `.deb` |
| [#146](https://github.com/davison/md-notes/issues/146) | [#147](https://github.com/davison/md-notes/issues/147) | GO-2025-3595 reachable through the sanitiser, and nothing scanning dependencies |
| [#156](https://github.com/davison/md-notes/issues/156) | [#158](https://github.com/davison/md-notes/issues/158) | The side pane narrowing the note at middle widths |
| [#157](https://github.com/davison/md-notes/issues/157) | [#158](https://github.com/davison/md-notes/issues/158) | Browser-default scrollbars |
| [#163](https://github.com/davison/md-notes/issues/163) | [#164](https://github.com/davison/md-notes/issues/164) | `sudo make install` rebuilding everything as root. Open at the time of writing |

[#130](https://github.com/davison/md-notes/issues/130), the Chrome Web Store capture,
closed as not planned with the channel.

Raised by this milestone, for a later task to adopt:

| Capture | From | What it is |
|---------|------|-----------|
| [#145](https://github.com/davison/md-notes/issues/145) | the review of [PR #144](https://github.com/davison/md-notes/pull/144) | A note's raw HTML can erase the renderer's line anchors, or wear one — the residual beyond what #139 fixed |
| [#149](https://github.com/davison/md-notes/issues/149) | [#147](https://github.com/davison/md-notes/issues/147#issuecomment-5734015503) | Code colours are toned from chroma's own palette at generate time, so every chroma upgrade is a colour change. The structural reason the hold-back exists |
| [#153](https://github.com/davison/md-notes/issues/153) | the review of [PR #150](https://github.com/davison/md-notes/pull/150#issuecomment-5734635348) | The browser suites are documented in three places to skip without Chromium, and instead die in `browserType.launch`. `CONTRIBUTING.md` was corrected at the time; the `Makefile`, the harness header and the README still overclaim |
| [#155](https://github.com/davison/md-notes/issues/155) | the re-review of [PR #150](https://github.com/davison/md-notes/pull/150#issuecomment-5734787149) | The first-start log says `mdn token` even when the daemon was started with `--token-file` |
| [#160](https://github.com/davison/md-notes/issues/160) | the withdrawal of the store channel | The privacy statement, the permission justifications and the screenshot generator from the closed store pull request, which are worth having without a store — and, added to it by the coordinator after the review of [PR #165](https://github.com/davison/md-notes/pull/165#issuecomment-5744447868), the four places in code and workflow comments that still explain the version rule or the channel list by the store: `scripts/relcheck/main.go`, `.github/workflows/release.yml`, `extension/scripts/version.mjs` and `extension/scripts/version.test.mjs`. The prose is corrected; these are the same correction left half-made |
| [#161](https://github.com/davison/md-notes/issues/161) | the first release run | Every pinned action is on a Node 20 major, deprecated, and `ubuntu-latest` moves to Ubuntu 26 on 2026-10-19 |
| [#162](https://github.com/davison/md-notes/issues/162) | the operator | Several permanent roots on the command line and in the configuration file; a repeated `--root` is silently dropped today |
| [#167](https://github.com/davison/md-notes/issues/167) | M8 QA, observation 1 | A release is not bit-reproducible from its tag, and the documentation does not say what to verify instead. Nothing promised reproducibility; what a user can check is the asset's checksum, and the `-bin` package's own guarantee — that its `/usr/bin/mdn` is byte-identical to the release asset — does hold, verified in all three packages |

## Where the record is silent

Two things this record does not explain, because nobody wrote them down at the time.

**Why `0.1.0` and not `1.0.0`.** The version was set as a default at the milestone's
opening — "the version defaults to 0.1.0 unless the operator records otherwise on this
issue"
([#133](https://github.com/davison/md-notes/issues/133#issuecomment-5733256158)) — and
the operator never recorded otherwise, so it stood. There is no comment weighing the two;
the default was simply not overridden. Inferred from the absence, and stated here rather
than dressed as a decision.

**The operator's daemon stopped during the QA run**, and no session issued it a signal.
QA recorded the fact rather than guessing at a cause, including that its own kills were
confined to its own process tree
([#133](https://github.com/davison/md-notes/issues/133#issuecomment-5744403052),
observation 6). The coordinator's disposition is that nothing in the coordination layer
restarts a daemon that is the operator's
([#133](https://github.com/davison/md-notes/issues/133#issuecomment-5744425314)). Recorded
because an unexplained stop on the machine a milestone was verified on is exactly the kind
of thing a later reader would want to know was noticed.
