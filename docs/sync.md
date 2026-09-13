# Sync and offline editing

Notes are plain markdown files in a folder. That is the whole design, and it
is what makes the multi-device story simple: md-notes never syncs anything.
[Syncthing](https://syncthing.net/) mirrors the folder between your machines
and your phone, and the daemon treats what Syncthing writes exactly as it
treats an edit from `vim` in another terminal — because it cannot tell the
difference, and does not try to.

This page describes the arrangement: how the pieces fit, how to set Syncthing
up, what offline editing looks like, what happens when two devices edit the
same note, how Android fits in, when to use the tailnet instead, and the one
shape that does not work.

Everything below about *md-notes* was checked against the built daemon, and
[Verified against the daemon](#verified-against-the-daemon) lists what was run.
Everything about *Syncthing* is summarised from
[Syncthing's own documentation](https://docs.syncthing.net/) and is marked as
such where it appears; nothing here installs or configures Syncthing for you.

## The arrangement

```
   laptop                     desktop (mdn serve)            phone
   ~/notes  <--- Syncthing --->  ~/notes  <--- Syncthing --->  Notes/
   any editor                    mdn + any editor             Markor
```

Three claims hold the arrangement together.

**The daemon serves and edits files; it does not sync them.** `mdn serve`
watches one or more root folders, renders the markdown in them, and writes
edits back to the original files in place. It has no notion of a remote, a
peer or a revision history. Nothing about it needs to know that a folder is
shared.

**Syncthing mirrors the folder.** It is a separate program with its own
daemon, watching the same directory and reconciling it with the other devices
that share it. It writes files; the notes daemon reads and writes the same
files.

**Anything may edit the notes.** md-notes is one editor among several. Markor
on the phone, `vim` on the laptop, a script that appends to a log — all of them
are writing markdown into a folder, which is the only interface any of them
share.

So the daemon's [live update](introduction.md#live-update) and its
[conflict handling](introduction.md#conflicts) are not sync features that had
to be built. They exist because the files can change under the daemon at any
time, and Syncthing's writes are simply one more way that happens. A note
Syncthing rewrites while you are reading it refreshes in the browser without a
reload. A note it rewrites while you have an unsaved draft open raises the
same banner an external editor would.

The notes daemon runs on whichever machines you want the browser UI on — one
is normal. Syncthing runs on every device, including the ones with no daemon
at all.

## Setting Syncthing up

*This section is summarised from* [*Syncthing's documentation*](https://docs.syncthing.net/)
*rather than verified here. Follow the manual for the current UI; what is below
is what the folder needs to look like for md-notes' sake.*

Syncthing pairs devices by device ID and shares folders between them. The
essentials:

1. **Install Syncthing** on each machine and, on Android, from the
   [Syncthing-Fork](https://github.com/Catfriend1/syncthing-android) app.
   Syncthing's web GUI lives at `http://localhost:8384` on a machine.
2. **Introduce the devices to each other.** Each has a device ID; add the
   others' IDs under **Remote Devices**. Devices on a tailnet can reach each
   other by their tailnet names, and Syncthing's own relays and discovery work
   without one.
3. **Share the notes folder.** Add your notes root — the directory
   `notes_root` names in `~/.config/mdn/config.yml` — as a Syncthing folder,
   give it a **folder ID** you will recognise, and share it with the other
   devices. They pick their own local path for it; the folder ID is what makes
   them the same folder.
4. **Keep one peer always on** if you want a laptop and a phone that are rarely
   awake together to converge. Sync itself is peer-to-peer, so two devices
   exchange files only while both are running. A machine that is always up — or a
   Syncthing binary on whatever box you already leave on, reached over SSH to
   configure — closes that gap. It does not need the notes daemon; it only
   needs to hold a copy.

### Ignore patterns

Syncthing reads a `.stignore` file at the top of the shared folder. Two things
are worth putting in it:

```
// editor scratch files
(?d)*.swp
(?d)*~
(?d).DS_Store
```

Syncthing's own bookkeeping does not need ignoring — it never syncs `.stfolder`,
`.stignore` or `.stversions` — and neither does md-notes'. The daemon stages a
save as a temporary file in the note's own directory and renames it into place,
so the staging file exists for the length of one save; a `(?d).mdn-save-*` line
costs nothing if you would rather Syncthing never saw one at all.

Do **not** ignore `clips/`. Clips from the browser extension are ordinary
markdown files in an ordinary directory under the notes root, and they sync
like every other note.

### What Syncthing leaves in the folder, and what the navigator does with it

Syncthing puts three things in a shared folder: a `.stfolder` marker directory,
your `.stignore` file, and — if you turn file versioning on — a `.stversions`
archive of superseded copies. Transfers in flight are staged in hidden
`.syncthing.*.tmp` files.

None of them appear in the navigator. The tree is built from a ripgrep listing
that skips hidden entries, so anything whose name begins with a dot is out
before the markdown filter is reached; and the watcher is built from the same
listing, so a `.stversions` archive is not merely hidden but uncounted — 300
version directories added to a root still left the daemon reporting
`{"watched":3,"unwatched":0}`. A large version archive costs you disk, not
[watch budget](introduction.md#the-watch-budget).

## Editing offline

Nothing in md-notes needs the network. The daemon binds to loopback; the only
thing a network absence stops is Syncthing catching up with the other devices,
which it does when it can.

**On a machine with no daemon** — a laptop on a train — edit the files with
whatever you like. Syncthing propagates them when the machine is next online
alongside a peer. This is the common case and it needs no thought.

**On a machine with its own daemon** — a laptop you want the browser UI on —
run `mdn serve` against the synced folder and use the application normally. Two
daemons on two machines over one Syncthing folder is a supported shape: each
serves its own local copy of the files, each watches its own copy, and neither
knows the other exists. Syncthing reconciles the two copies afterwards.

That last sentence is where the interesting case lives.

## When two devices edit the same note

Syncthing propagates whole files; it does not merge them. If a note is edited
on two devices before they next sync, Syncthing keeps both: the newer file wins
the original name, and the one it displaced is renamed beside it.

```
readme.md
readme.sync-conflict-20260913-101010-ABCDEFG.md
```

The parts are the date, the time and the first characters of the device ID that
produced the losing copy. Syncthing's manual has the rules for which side is
renamed.

**The navigator shows a conflict file as an ordinary note**, because that is
what it is: a markdown file in a root the daemon serves. It sorts next to the
note it came from, renders, is searchable, and can be opened in the editor and
edited. Two versions of a note with the same `# Heading` show the same title in
the note pane, so the file name in the navigator is what tells them apart.

### Resolving one

1. Open both — the note and its `sync-conflict-` sibling — and read the
   difference.
2. Edit the survivor into the shape you want, in md-notes or any editor. If the
   conflict copy is the one you want to keep, copy its text into the real note
   rather than renaming the file: md-notes edits notes in place and cannot
   create, rename or delete one
   ([Editing](introduction.md#editing)).
3. Delete the conflict file with a file manager, a shell, or Markor on the
   phone. The navigator drops it as soon as it goes.

Deleting it on one device deletes it everywhere once the devices sync, which is
what you want.

If conflict files keep appearing, the usual cause is two devices editing the
same note during a stretch when they could not see each other. Syncthing's
docs describe file versioning, which keeps the displaced copies in
`.stversions` instead of beside the note, if you would rather the navigator
never showed them.

### What the daemon does with a write that arrives while you are looking

The behaviour is the same as for any external editor, and it depends on whether
you have unsaved work:

| What is open | What happens when Syncthing rewrites the file |
|--------------|-----------------------------------------------|
| The rendered view | It refreshes to the new text |
| The editor, with no unsaved changes | The editor's text is replaced with the file's |
| The editor, with an unsaved draft | A conflict banner: **Keep my draft**, **Load the file**, **Copy draft** |

A draft is never dropped for you. [Conflicts](introduction.md#conflicts)
describes the three buttons and the deleted-file case, where only **Copy draft**
and **Discard draft** are offered because the save API cannot recreate a file.

Note that this is a *second* kind of conflict, separate from Syncthing's. The
banner is the daemon noticing that the file on disk moved under your draft.
Syncthing's conflict file is Syncthing noticing that two devices diverged. You
can meet either alone, or both at once — a `sync-conflict-` file appearing in
the navigator while the banner is up over the note it came from.

## Android

Read-mostly, with the occasional edit, over the same folder:

- [**Syncthing-Fork**](https://github.com/Catfriend1/syncthing-android) — the
  maintained Android build of Syncthing. It holds a local copy of the notes
  folder and syncs it like any other device.
- [**Markor**](https://github.com/gsantner/markor) — a markdown editor that
  opens a directory of files. Point it at the synced folder and it reads and
  writes the same notes.

There is no md-notes application on Android. The phone gets plain files and a
plain editor, which is the point of keeping notes as plain files; the daemon and
its browser UI stay on the machines.

**Clipping from the phone is not built yet.** The plan is an inbox file of
URLs, shared to from the phone's share sheet, which the daemon turns into
proper clips when the folder next syncs to a machine running it. That is a
later milestone; today the browser extension clips from a desktop browser only
([The browser extension](extension.md)).

## The tailnet, and when to use it instead

If the device you are on can reach the daemon's machine, you do not need a
local copy at all: `tailnet_host` lets the daemon answer to one extra host name
behind `tailscale serve`, and the whole UI — reading, editing, search, live
update — works from another node on the tailnet after a token login. The
README's [Over the tailnet](../README.md#over-the-tailnet) section and
[the introduction](introduction.md#reaching-the-daemon-over-the-tailnet) cover
the setup and what it narrows.

The two routes answer different questions, and this project uses both:

| | Syncthing | Tailnet |
|---|---|---|
| Needs connectivity | Only to converge; editing never does | Yes, at the moment of use |
| Copy on the device | Yes | No |
| Can conflict | Yes — see above | No; there is one copy |
| Any editor | Yes | No; the daemon's UI |

The tailnet is the better route when you want one authoritative copy and you
have the network for it. It is not a replacement for sync on a phone, where the
VPN slot is often taken by something else and the notes should be readable
anyway. Syncthing is the floor; the tailnet is the convenience on top.

## The one shape that does not work

**Two daemons, two machines, one Syncthing folder: fine.** Covered above.

**Two daemons over one folder on one machine: don't.** Not because they refuse
to start — the second one only complains about the port, and giving it its own
`--port`, `--state` and `--token-file` starts it happily against the same
directory — but because of what the pair then fails to do.

A single daemon serializes saves: one lock covers every read and save across
every root, so two browser tabs editing the same note cannot both win. The
later save is refused as stale and kept as a conflict. That guarantee is a
process mutex, and the revision tokens it hands out are local to one daemon
session. Two daemons share neither. They coordinate through the filesystem and
nothing else, and an ordinary filesystem offers no conditional rename against a
competing writer.

Most of the time that still looks fine, because each daemon re-reads the file
immediately before writing: a save carrying a revision the *other* daemon has
already superseded is caught and refused with a conflict. What is not caught is
two saves landing at once — both read the same bytes, both check, both rename,
and one of them silently replaces the other with each side told its save
succeeded. Two hundred rounds of exactly that, one save through each daemon:

| Shape | Rounds where both saves reported success and one was lost |
|-------|-----------------------------------------------------------|
| Two clients, one daemon | 0 of 200 |
| Two clients, one daemon each, same folder | 23 of 200 |

Across two machines the same race exists and does not matter, because Syncthing
is standing between the copies: it sees the divergence and writes a conflict
file, so the losing text is on disk under a name you can find. On one machine
there is no Syncthing between the two daemons — they are writing to the same
inode — so the losing text is simply gone.

If you want the UI twice on one machine, open a second browser tab. It is the
same daemon, and the lock is doing its job.

## Verified against the daemon

These claims were checked against the daemon built from this repository, on
temporary roots, with headless Chromium where a browser was needed:

- A `readme.sync-conflict-20260913-101010-ABCDEFG.md` file placed in a served
  root is listed in the navigator beside `readme.md`, renders in the note pane,
  is found by search, and saves through the source API like any other note.
- Deleting that conflict file takes it out of the navigator with no reload.
- A note and a directory created in the root by another process appear in the
  navigator with no reload.
- A note open in the editor with no unsaved changes follows an external
  rewrite, and no banner is raised.
- The same note with an unsaved draft raises `Conflict: draft kept` and the
  three-button banner instead.
- `.stfolder`, `.stignore`, `.stversions` and a `.syncthing.*.tmp` staging file
  in the root are all absent from the navigator, while `clips/` is present.
- 300 directories under `.stversions` left the watch report at
  `{"watched":3,"unwatched":0,"budget":8192,"overBudget":false,"failed":0,"limited":false}`.
- Two concurrent saves through one daemon lost no update in 200 rounds; through
  two daemons over one folder, 23 of 200 rounds lost one.

The Syncthing setup instructions are the exception: they are summarised from
Syncthing's documentation, as the section itself says.
