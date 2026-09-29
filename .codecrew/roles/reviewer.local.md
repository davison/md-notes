<!--
.codecrew/roles/reviewer.local.md — this project's extension to the reviewer contract.
Loaded after .codecrew/roles/reviewer.md, append-only, never a replacement: an extension
that contradicts its contract is a review finding. gh codecrew roles show reviewer
prints the composition. Comments only, it adds nothing.

What belongs here, with worked examples (house style, repo conventions,
a platform's wake syntax, ids and tooling):
https://github.com/radiusred/gh-codecrew/blob/main/docs/extensions.md
Protocol: https://github.com/radiusred/gh-codecrew/blob/main/SPEC.md (section 7)
-->

## Read every commit's type against its diff (davison/md-notes#215)

`ci.yml` skips its `check` and `e2e` jobs for a push or pull request whose
every commit has a `docs` subject, scoped forms included, and looks at nothing
else (the implementer's local extension says how the type is to be chosen).
`chore` and every other type run them. A wrong `docs` therefore ships untested
code, and only you will see it: CI will be green because it did not run.

- Read the type of **every** commit in the PR against what that commit changes,
  not the head commit and not the PR title.
- A `docs` commit that changes anything the tests check (code, comments in
  source files, tests, workflows, the Makefile, packaging, the man pages,
  `go.mod`, `.gitignore`), or where a test run is plainly wanted, is a
  **blocking finding**. It is fixed by rewording the commit to `fix(...)`, or
  to the truer type (`feat`, `test`, `build`, `ci`, `refactor`).
- A `docs` commit that touches only prose or documentation pages is correct:
  do not ask for it to be retyped.
- There is no rule against `chore` beyond the contract's own: housekeeping is
  `chore:` under SPEC section 4 and always runs CI.
- Where a PR's CI run shows `check` and `e2e` as skipped, say in the review
  that you confirmed each commit's type first.
