# On an e-ink tablet

An e-ink tablet is a good notes device and a bad web browser. The panel has no
backlight, so a dark theme is grey text on a grey-black page rather than the
crisp one a phone gives; a refresh is slow and visible, so an animation arrives
as a stutter or a smear rather than as motion; and a stylus is a blunter
instrument than a mouse, so a 27-pixel row in a tree is a row you miss.

The device this page was written for is a **Boox Note Air 3**: Android 12, a
1404x1872 panel at a device pixel ratio of 2 — which should be 702x936 CSS
pixels in portrait and 936x702 in landscape — a capacitive touchscreen, a Wacom
stylus, and a Chromium-based browser called NeoBrowser. Both of those are under
the application's 960-pixel breakpoint, so on those figures the tablet gets [the
narrow layout](introduction.md#on-a-phone): the note has the screen, and the
navigator and the search-and-tags pane are tabs of a drawer. Zoomed out, which
is the first thing a reader does on a panel this slow, it crosses into [the
two-column layout](introduction.md#in-a-narrower-window) — the navigator with
the search pane under it, and the note taking every pixel the navigator does
not. Past 1290 pixels, into the three-pane layout, takes a zoom of about 72%
or less in landscape and about 54% or less in portrait, those being 936 and
702 pixels divided by 1290. The figures are arithmetic from the panel, not a
reading taken off the device — see
[what has not been checked](#what-has-been-checked-and-what-has-not) — but nothing here
depends on them: the tap targets follow the pointer at any width, and the
layout follows the width whatever it turns out to be.

There are two ways to reach your notes from it, and they answer different
questions. Try both.

## What to turn on first, either way

The web UI has two settings for this device, behind the **gear** at the right of
the top bar. Both are remembered in the browser, per device, and both take
effect at once.

- **Always use the light theme.** The page uses the light palette whatever the
  device's colour-scheme preference says — which matters, because Android's
  night mode is a reasonable thing to leave on for a phone and the wrong way
  round for e-ink. The code colouring in fenced blocks follows the same switch,
  so a fence is not left in dark-scheme colours on a light page. The override is
  applied before the page paints, so you never see a frame of the scheme you
  overrode. Turning it off gives the device its preference back.
- **No animation.** No transitions anywhere, and no flash on the block a search
  hit scrolls to — the note still scrolls the hit to the middle of the screen,
  which is what tells you where it is. This is also on automatically when the
  device asks for reduced motion in its accessibility settings, so on a tablet
  that already does, the switch is belt and braces.

Tap targets do not need a setting. The application's own controls — tree
entries, the navigator's **Recent first** toggle, tags and the clear link,
search hits and the search box, the drawer's
tabs, the top bar's buttons including **New note**, the note bar's including
**Delete**, the frontmatter disclosure, the conflict banner's buttons including
**Recreate the note**, both dialogs' buttons, the create
prompt's name box and the home page's **Remove** control on a recent root — are
at least 40 pixels tall wherever the browser
reports a touch or stylus pointer, including the wide layout you get by zooming
out. A mouse keeps the compact rows. Links *inside* a note are the exception,
and have to be: their size is the line of prose they sit in, and a 40-pixel line
is not prose.

Scrollbars need no setting either. Every scrolling pane — the navigator, the
note, the search results, the editor and the drawer — draws a thin bar in the
palette's own colours rather than the browser's grey, in either scheme and
under the light override, with the thumb at least 3:1 against the pane behind
it so it is still there on a panel with no backlight. Thin, never hidden: a bar
you cannot see is a bar a stylus cannot catch.

Creating and deleting a note are stylus-sized for the same reason. **New note**
sits in the top bar at every width, so it is reachable without scrolling a long
tree, and both prompts are the application's own dialogs rather than the
browser's — which means they take the light override and the no-animation
setting like everything else on the page, where `window.prompt` and
`window.confirm` would not
([#77](https://github.com/davison/md-notes/issues/77#issuecomment-5701434444)).
[Creating and deleting a
note](introduction.md#creating-and-deleting-a-note) describes both.

## Route one: the browser, over the tailnet

There is one copy of the notes, on the machine running the daemon, and the
tablet reads and writes it over the network. Nothing is stored on the tablet,
so nothing on it can conflict.

What it needs:

1. **The daemon reachable on the tailnet.** `tailnet_host` in the daemon's
   config, `tailscale serve` in front of it, and the bearer token. [Reaching the
   daemon over the tailnet](introduction.md#reaching-the-daemon-over-the-tailnet)
   is the whole setup, and [what is reachable under that
   name](introduction.md#what-is-reachable-under-that-name-and-what-is-not) is
   what it narrows.
2. **Tailscale on the tablet.** The Android app, from the Play Store or as an
   APK if the tablet has no Play services. It takes the device's single VPN
   slot: Android allows one VPN at a time, so anything else you route — a work
   VPN, a filtering DNS app — is off while Tailscale is on. If the tablet is
   already holding that slot for something, this route is closed and route two
   is the answer.
3. **NeoBrowser**, the tablet's own browser, at the tailnet name. You log in
   once with the token; the session is a cookie, so it survives closing the tab.

Then the gear, the two settings, and you are reading.

If the tablet has Brave or Chrome on it as well, the UI can be installed as an
app in its own window rather than opened in a tab —
[Installing it on the phone](sync.md#installing-it-on-the-phone) is the same
procedure, and the same caveat applies: what works with the daemon unreachable
is the application, not the notes. NeoBrowser is a browser of its own and has
not been checked for the install prompt; on a tablet that route matters less
than on a phone, since there is no home screen habit to fit into.

## Route two: Syncthing-Fork and Markor

The tablet holds its own copy of the notes folder and edits it with a plain
markdown editor. No network at the moment of use, no daemon, no browser.

This is exactly the Android arrangement [Sync and offline
editing](sync.md#android) describes, and the tablet is an Android device like
any other: [Syncthing-Fork](https://github.com/Catfriend1/syncthing-android)
mirrors the folder, [Markor](https://github.com/gsantner/markor) opens it. The
folder is **Send & Receive**, so an edit made on the tablet reaches everything
else. Two devices editing the same note while apart produce a Syncthing conflict
file, which [the navigator shows and the conflict
section](sync.md#when-two-devices-edit-the-same-note) tells you how to resolve.

Markor is a better stylus-era editor than a browser text box, and it works with
the tablet's own keyboard and handwriting input. What it does not give you is
the rendered view, server-side search across the whole root, the tag panel or
live update.

## Which one

| | Browser over the tailnet | Syncthing-Fork and Markor |
|---|---|---|
| Works offline | No | Yes |
| Copy on the tablet | No | Yes |
| Can produce a conflict file | No | Yes |
| Rendered notes, search, tags | Yes | No |
| Needs the VPN slot | Yes | No |
| Editing | The app's editor, in the browser | Markor |

They are not exclusive, and running both is the arrangement worth aiming at: the
tablet syncs the folder so the notes are readable with the network down, and the
browser is there when you want search, the rendered view, or to be sure you are
looking at the one authoritative copy. The one shape to avoid is the one
[sync.md](sync.md#the-one-shape-that-does-not-work) already rules out, and it
does not arise here — the tablet never runs a daemon.

## What has been checked, and what has not

Everything above about the web UI was verified in headless Chromium at the
tablet's viewport in both orientations, with the device set to prefer dark: the
light override winning over that preference and taking the code colours with it,
no transition and no flash under the no-animation setting, every tap target at
40 pixels or more, and the editor still usable when the layout viewport shrinks
by the height of an on-screen keyboard.

Since milestone five that is no longer a measurement taken once: the `ui/e2e`
suite re-measures the light override, the suppressed flash and the tap targets
its own list names — the create control, the dialogs' own controls and the
navigator's order toggle included — on five coarse-pointer profiles on every
push, as a CI job of its own. Two of the controls listed above are measured
where they are raised instead, since no state that list walks has them on
screen: the conflict banner's buttons with a note deleted under an open draft,
and the home page's **Remove** control on one coarse profile. [What holds these
numbers](introduction.md#what-holds-these-numbers) says what it covers.

What has not been checked is the tablet. Nothing here has run on a Boox, and the
things that only hardware can answer are: **what viewport NeoBrowser actually
reports** — 702x936 is arithmetic from the panel and its pixel ratio, and a
browser is free to disagree, though as above nothing depends on it; whether
NeoBrowser reports a coarse pointer (if it reports a fine one, the tap targets
will not grow past the 960-pixel breakpoint, and that is a one-line fix);
whether the panel's own refresh modes leave ghosting the settings cannot help
with; whether the on-screen keyboard behaves as Chromium's
`interactive-widget=resizes-content` says it should; and whether the VPN slot is
free. The operator's check on the device is recorded on
[the milestone issue](https://github.com/davison/md-notes/issues/55) when it
happens.
