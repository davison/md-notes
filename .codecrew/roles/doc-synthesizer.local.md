<!--
.codecrew/roles/doc-synthesizer.local.md — this project's extension to the doc-synthesizer contract.
Loaded after .codecrew/roles/doc-synthesizer.md, append-only, never a replacement: an extension
that contradicts its contract is a review finding. gh codecrew roles show doc-synthesizer
prints the composition. Comments only, it adds nothing.

What belongs here, with worked examples (house style, repo conventions,
a platform's wake syntax, ids and tooling):
https://github.com/radiusred/gh-codecrew/blob/main/docs/extensions.md
Protocol: https://github.com/radiusred/gh-codecrew/blob/main/SPEC.md (section 7)
-->

## Front-door documents — this hub

This hub's front door is the README (`README.md`), written for a
stranger who has never heard of md-notes, and
`docs/introduction.md`, the full account of
what the daemon and the app do. At every milestone boundary the record's PR
brings their claims into line with what the milestone delivered:

- **The README:** what md-notes does, the screenshots under `docs/images/`
  (`make screenshots` regenerates them), the install channels, and the
  documentation list. It names no release version: it links to the
  releases page, so a release never makes it stale.
- **The introduction:** its account of the daemon, roots, the HTTP API
  (endpoints, refusals and their codes), the web UI, editing, live update,
  the tailnet and confinement.

The guides the README's Documentation section links to are the front door's
second rank: `docs/install.md`, `docs/running.md`, `docs/extension.md`,
`docs/privacy.md`, `docs/sync.md`, `docs/e-ink.md`, `CONTRIBUTING.md`,
`docs/releasing.md`, and the man page `contrib/mdn.1`. Refresh one when the
milestone changed a claim it makes, such as a flag, a command, a path or a
count. Where the milestone changed none of these claims, leave it alone.
