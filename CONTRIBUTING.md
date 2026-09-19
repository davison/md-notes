# Contributing

This page is for someone who has just cloned the repository: what to install,
what to run, how a change gets from an idea to a merged pull request, and how
a release is cut. Every command below was run on a Linux machine before it
was written down.

If you are an agent session rather than a person, read [AGENTS.md](AGENTS.md)
first — it points at the role contract you work under, and this page is the
part of it a human would also read.

## What you need

- **Go**, at the version `go.mod` names (1.27) or later.
- **pnpm**, version 10, and **Node** 24 — what CI uses.
- **ripgrep** (`rg`) on `PATH`. The daemon shells out to it for the
  navigator's tree, for search and for the tag panel, so it is a runtime
  dependency and not a build one. It says so when it is missing rather than
  quietly serving nothing: the daemon starts and answers the UI, logs
  `watch notes: ripgrep (rg) is not installed or not on PATH; watching every
  non-hidden directory`, and the tree, search and tags endpoints each answer
  `{"error":"ripgrep (rg) is not installed or not on PATH"}`. It is also what
  keeps gitignored and hidden files out of the tree and out of results, since
  ripgrep already knows the ignore rules.
- **Chromium**, for the two browser suites only. Playwright downloads its own
  build, and the download is a separate step from installing the package:
  `make ui-deps` installs the driver, and the driver then fetches the browser.
  So the two commands go together, in this order:

  ```
  make ui-deps
  pnpm --dir ui exec playwright install chromium
  ```

  The second on its own, in a clone that has never been built, exits 254 with
  `ERR_PNPM_RECURSIVE_EXEC_FIRST_FAIL  Command "playwright" not found`: there
  is no `ui/node_modules` for it to find the driver in yet.

  Do this before running either browser suite. Both drive a real browser
  against the real built daemon — which is why a browser is needed at all,
  rather than a DOM shim — and with the download missing they do not skip.
  They fail at the call that would have started the browser, with
  `Executable doesn't exist at …` and Playwright's own box telling you to
  install one: `make e2e` exits 2, `pnpm --dir extension e2e` exits 1.

## Building

```
make build      # the UI bundle and the static ./mdn binary
make extension  # the browser extension, into extension/dist, and a zip
make install    # installs what make build made; needs root (PREFIX=... to change)
make clean      # removes what build, extension and release produce
make distclean  # clean, and both node_modules trees as well
```

`make install` is a copy and nothing else. It has no prerequisite: it installs
the `./mdn` that `make build` already made and `contrib/mdn.service` beside it,
and refuses in one line, having copied nothing, if either is missing. So build
as yourself and install as root — never the other way round, which is what used
to run `pnpm install` and `go build` under `sudo` and leave root-owned files in
your checkout (#163):

```
make build
sudo make install
```

The two destinations are `/usr/bin/mdn` and
`/usr/lib/systemd/user/mdn.service`, which are the two paths the `.deb`
installs. **If the `md-notes` package is installed, do not:** both write
`/usr/bin/mdn`, so `make install` would overwrite the packaged binary behind
dpkg's back (`dpkg -V md-notes` then reports it modified) and a later
`apt remove` would delete your build. Use a prefix or a staging directory of
your own instead, which is also how to try an install without root:

```
make install DESTDIR=$PWD/dist/scratch  # ./dist/scratch/usr/..., gitignored
make install PREFIX=$PWD/dist/scratch   # ./dist/scratch/bin/mdn, gitignored
make install PREFIX=$HOME/.local        # ~/.local/bin/mdn, the old default
```

Installing does not start anything: the target's last lines are the
`systemctl --user daemon-reload` and `systemctl --user enable --now mdn` for
you to run, because `systemctl --user` under `sudo` is root's session and
cannot enable a user unit for you.

Under any prefix but `/usr`, `contrib/mdn.service` needs a drop-in pointing
`ExecStart` at the binary you installed, and under a prefix systemd does not
search — `~/.local` is one — the unit needs copying to
`~/.config/systemd/user/`; the unit's header has both.

`make clean` removes the binary, `ui/dist`, `extension/dist`, the extension zip
and `dist/`, and nothing else; it attempts all of them and names what it could
not remove rather than stopping at the first. `make distclean` also removes
`ui/node_modules` and `extension/node_modules`, so the next build re-runs
`pnpm install` and needs the network.

The UI is built into the binary, so `make build` builds both halves; there is
no separate step to remember. The version the binary reports comes from
`git describe`, so `mdn version` prints the commit it was built from on an
untagged tree, and a version number only on a tagged one. Nothing carries a
version literal that has to be bumped by hand;
[docs/releasing.md](docs/releasing.md) has the whole chain.

## Testing

```
make check   # vet, typecheck, Go tests, UI tests, extension tests, build
make vuln    # govulncheck over the Go graph, pnpm audit over both lockfiles
make e2e     # the UI's browser suite, in headless Chromium
```

`make check` is what CI's `check` job runs, and it is the one to run before
every commit. It is well under a minute.

`make vuln` is a step of the same CI job but deliberately not part of `make
check`: it asks the vulnerability database and the npm registry, so it can go
red without the tree having changed, and it fails when the network is away. A
red scan means something was published, not that the build broke.

`make e2e` builds the daemon and drives it through headless Chromium — the
phone layout, the drawer, the display settings, the tap targets, the asset
cache, creating and deleting a note, the navigator's two orders, removing a
root, the installable app and the service worker's cache name: eight suites,
54 tests. It is CI's second job.

The extension has a browser suite of its own, which CI does not run, because
it needs both the extension and the daemon built:

```
make build
make extension
pnpm --dir extension e2e
```

It loads the built extension into a real browser profile and clips into a
daemon it starts itself. With either of those two halves missing it skips and
names what is missing (`no mdn binary at …`) — worth knowing, because a suite
that skips still exits 0 and reads like a pass. The browser download is not
one of the halves it checks for: without it, this suite and `make e2e` both
fail at launch, as above.

## Running it while you work

The daemon takes its configuration from `~/.config/mdn/config.yml`, and the
flags override it, which is what you want against a scratch folder:

```
mkdir -p /tmp/mdn-dev/notes
./mdn serve --root /tmp/mdn-dev/notes --port 8819 \
    --token-file /tmp/mdn-dev/token --state /tmp/mdn-dev/roots.json
```

Then open `http://localhost:8819/` in a browser. The daemon binds the loopback
address only.

`--token-file` and `--state` are worth passing. The daemon writes a bearer
token on first start — what the extension presents to post a clip — and
remembers the folders registered with `mdn open`; both live under your home
directory by default, and pointing them at a scratch path is how a test run
stays out of the state your real daemon is using. Pass the same path when you
read the token back:

```
./mdn token --token-file /tmp/mdn-dev/token
```

Bare `mdn token` reads the token under your home directory instead — and
creates one there if there is none — so against a scratch daemon it prints a
token that daemon has never heard of.

The README's [Running](README.md#running) section describes the configuration
file, the tailnet host and the rest of the daemon's behaviour; there is no
need to repeat it here.

## Commit messages

The convention is the one the history uses — `git log --format='%s'` is the
best description of it:

```
feat(watch): report a refused directory as its own cause (#140)
fix(ui): say the daemon is not answering, not "Failed to fetch" (#118)
docs(release): say how a release is cut and what happens after the tag (#134)
test(render): show a note forging the scroll-to-line target (#139)
```

- `type(scope): lower-case summary`, with the task issue's number in
  parentheses at the end. The scope is optional and is usually the package or
  directory the change lives in.
- The types in use are `feat`, `fix`, `docs`, `test`, `refactor`, `chore`,
  `build`, `ci` and `perf`.
- The summary says what the commit does, in the imperative, and is not
  capitalised.
- One change per commit. A test that demonstrates a bug and the fix that
  closes it are two commits, in that order, so the record shows the test
  failing before it passed.
- When a model wrote the commit, it ends with a `Co-Authored-By:` trailer
  naming the model.

The release notes are generated from these subjects, so they are worth
writing well.

## How work flows here

This repository is run as a [CodeCrew](https://github.com/radiusred/gh-codecrew)
project. Coordination lives in GitHub issues and pull requests rather than in
a chat log, and the shape is the same whether a person or a model is doing the
work:

- **An issue first, then a plan, then a branch.** Every change of substance
  has a task issue — the exception below, for a typo or a dead link, is the
  only one. Before the first commit, the plan goes into the issue: what will
  change, which requirement it serves, and anything that needs a human to
  decide. A trivial change gets a trivial plan, not an absent one.
- **One pull request per task**, closing its issue with `Closes #N`.
- **A model review before merge.** The reviewer is a separate session with a
  clean context, reading the diff against the task and the requirement; its
  findings land as a comment on the pull request. Nobody reviews their own
  work, and nobody merges their own work unreviewed.
- **Findings out of scope become backlog captures.** Something noticed while
  working on something else does not grow the pull request. It becomes its
  own issue in the shape below, and a later task adopts it. This is why the
  diffs stay readable.
- **Decisions are recorded when they are made**, as a comment on the issue
  starting `**Decision:**`, with the alternatives that were rejected and the
  trade-off that was weighed. Three weeks later the comment is the only place
  that reasoning still exists.
- **A question that belongs to a person is raised, not guessed.** It goes on
  the issue as a gate — the `cc:needs-decision` label and a comment stating
  the question — and the work waits for a reply that begins
  `**Gate resolved:**`.
- **A refusal is a message, not an obstacle.** The tooling refuses a blocked
  step with `refused[CODE]: detail`. The code says what is missing; the answer
  is to supply it, never to route around the gate.

The milestone records under [docs/milestones/](docs/milestones/) are what this
produces: each one links the tasks, the decisions and the verdicts behind a
milestone.

## Reporting a bug, or proposing a change

Open an issue. Anyone is welcome to, and the shape that is most useful is the
one the backlog already uses:

- A sentence of context — what you were doing, and where it happened.
- **Want** — what should be true instead, and why it matters. Include what
  you observed and what you expected, precisely enough for someone else to
  reproduce it.
- **Shape** — how a fix might look, if you have a view. A rough size helps.
  This part is a suggestion, not a specification; the task that adopts the
  issue decides.

Open the issue before writing the code, for anything with a decision in it.
The plan, the recorded decisions and the review all hang off the issue, so a
change that skips it has nowhere to say why it was made, and the first
question it meets will be that one. An obvious typo, a dead link or a stale
line in the documentation is welcome as a pull request on its own — there is
nothing to plan, and an issue would only be a second place to read the same
diff.

An issue may sit in the backlog for a while before a milestone takes it up.
That is the backlog working as intended, not the issue being ignored.

## Cutting a release

A release is two acts, and the second one is a person.

1. Push a tag of the form `v<major>.<minor>.<patch>`. The release workflow
   runs CI over that commit, builds the static daemon for `linux/amd64` and
   `linux/arm64`, builds the extension zip, writes `SHA256SUMS`, checks that
   the tag, the binary and the extension manifest all say the same version,
   and leaves a **draft** GitHub Release carrying the lot.
2. Open the draft, read the generated notes, and press Publish.

Nothing downstream starts until that press, and it has to be a person:
GitHub does not start workflow runs from events caused by a workflow's own
token, so a Release the workflow published itself would leave every channel
silently unstarted.

Publishing emits `release: published`, and each publishing channel is a
workflow file of its own on that event, so one that fails can be re-run alone
from the Actions page without cutting another tag. The channels are the
Chrome Web Store, the Arch User Repository and a `.deb` — they are what the
current release milestone,
[#133](https://github.com/davison/md-notes/issues/133), adds, in
[#135](https://github.com/davison/md-notes/issues/135),
[#136](https://github.com/davison/md-notes/issues/136) and
[#137](https://github.com/davison/md-notes/issues/137). Until those land, a
published Release is the whole of it: the binaries, the extension zip and the
checksums on the release page.

`make release VERSION=v0.1.0` does locally everything the workflow's build
step does, so none of this has to be tried for the first time on a real tag.
[docs/releasing.md](docs/releasing.md) is the full account: where the version
comes from, what the tag runs, what the assets are called, how to re-run a
release, and what to check at the first one.

## Licence

[MIT](LICENSE). Contributions are made under the same licence.
