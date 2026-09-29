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
every commit has a `docs` or `chore` subject (scoped forms such as
`docs(readme):` and `chore(deps):` included), and for nothing else. The subject
is the only thing that decides: no path is looked at. So the type is a
statement to the CI, and you are careful with it:

- Use `docs` or `chore` only for a change that CI need not test: prose,
  comments, housekeeping that no test, build or package reads.
- If the change affects what the tests check, or you would want the tests run
  on it anyway, use `fix(...)`, or another type that is truer (`feat`, `test`,
  `build`, `ci`, `refactor`). That covers the man pages (`contrib/mdn.1`,
  `packaging/deb/mdn.1`), the Makefile, workflows, `go.mod`, `.gitignore`,
  packaging files, test files, and code, whatever the change says about itself.
- Every commit in the PR counts, not the last. One commit that is not `docs` or
  `chore` runs the whole suite for the PR.
- A release never skips its checks, whatever the tagged commit's subject.
  That is `ci.yml`'s job, not yours; do not work around it.
