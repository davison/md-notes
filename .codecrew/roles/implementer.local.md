<!--
.codecrew/roles/implementer.local.md — this project's extension to the implementer contract.
Loaded after .codecrew/roles/implementer.md, append-only, never a replacement: an extension
that contradicts its contract is a review finding. gh codecrew roles show implementer
prints the composition. Comments only, it adds nothing.

What belongs here, with worked examples (house style, repo conventions,
a platform's wake syntax, ids and tooling):
https://github.com/radiusred/gh-codecrew/blob/main/docs/extensions.md
Protocol: https://github.com/radiusred/gh-codecrew/blob/main/SPEC.md (section 7)
-->

## Commit types decide whether CI runs the tests (davison/md-notes#215)

`ci.yml` skips its `check` and `e2e` jobs for a push or pull request whose
every commit has a `docs` subject (scoped forms such as `docs(readme):`
included), and for nothing else. `chore` and every other type run them. The
subject is the only thing that decides: no path is looked at. So the type is a
statement to the CI, and you are careful with it:

- Use `docs` only for a change CI need not test: prose and documentation
  pages. A change that affects what the tests check, or where you would want
  the tests run anyway, takes its true type: `fix(...)`, or `feat`, `test`,
  `ci`, `build` or `refactor` where that is truer. That covers code (comments in source
  files included), tests, workflows, the Makefile, packaging, the man pages
  (`contrib/mdn.1`, `packaging/deb/mdn.1`), `go.mod` and `.gitignore`,
  whatever the change says about itself.
- Housekeeping stays `chore:`, exactly as SPEC section 4 and the contract
  above require. It always runs CI, so the two do not conflict.
- Every commit in the PR counts, not the last. One commit that is not `docs`
  runs the whole suite for the PR.
- A release never skips its checks, whatever the tagged commit's subject.
  That is `ci.yml`'s job, not yours; do not work around it.
