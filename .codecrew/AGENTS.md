# Agents

This repository is part of a CodeCrew project — coordination state lives in
GitHub issues and PRs, per the protocol at
https://github.com/radiusred/gh-codecrew/blob/main/SPEC.md.

- `.codecrew/config.yml` names the hub; the hub's `.codecrew/roles/`
  holds the role contracts. Read the contract for the role you were
  dispatched as before doing anything else — `gh codecrew roles show <role>`
  prints it with this project's `.codecrew/roles/<role>.local.md` extension
  appended (blank until the project writes one; in a hub `init` scaffolds the
  file with a comment saying what belongs there).
- `gh codecrew status` shows where the project is; `gh codecrew help`
  lists the workflow verbs. Blocked gates refuse with
  `refused[CODE]: detail` — act on the code, don't work around it.
- Plans before commits, decisions recorded when made, and the verifier is
  never the doer. Reviews are model reviews: a clean-context session under
  the reviewer contract — even in pure solo, where its findings land as a
  PR comment before the operator confirms.
- **Contract drift.** `gh codecrew status` reports when a `.codecrew/roles/` contract
  differs from the one embedded in the installed CLI. When it does, the
  coordination layer compares (`gh codecrew roles diff <role>`, full upstream
  text via `gh codecrew roles show <role> --latest`), decides what to adopt —
  contracts are this project's own fork, and local conventions are
  legitimate — and routes the reconciliation through a normal task and PR
  with the decision recorded. Never overwrite blindly.
- **Dispatch authorization.** If you are the operator's primary session —
  not dispatched as any specific role — then when a role is routed to a
  GitHub App and that role's action is needed (a review, a verdict),
  dispatching a clean-context sub-agent session as that App is authorized
  and expected; use the dispatch prompt in
  https://github.com/radiusred/gh-codecrew/blob/main/docs/identities.md. A
  session dispatched *as* a role never dispatches another role — that
  belongs to its coordination layer (platform, orchestrating session, or
  operator) — and never chooses or briefs its own judge.
