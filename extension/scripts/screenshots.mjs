/**
 * Takes the extension's screenshots, into `docs/images/extension/`.
 *
 *     make extension-screenshots
 *
 * Four 1280×800 frames, each a captioned capture of the real extension doing
 * the real thing — a daemon started here on a temporary notes root, the built
 * extension loaded into Chromium, a clip taken from a page served here and
 * saved for real, a local `.md` file opened through the file-URL intercept,
 * and the options page. Nothing is mocked and nothing is drawn by hand, so a
 * screenshot that stops matching the product is one that fails to regenerate
 * rather than one that quietly lies in the documentation.
 *
 * Written for the Chrome Web Store listing (davison/md-notes#135) and kept,
 * when that channel was withdrawn, for the documentation (#160, #195): the
 * README and docs/extension.md show these, and the UI they photograph moves.
 *
 * The awkward parts come from the extension's own e2e suites
 * (`extension/e2e/`): the unpacked extension id, the profile preference that
 * stands in for "Allow access to file URLs", and opening the popup as a tab
 * because a browser-action popup cannot be clicked from a script. Playwright
 * and its Chromium are the ones `ui/` installs (`make ui-deps`, then
 * `pnpm --dir ui exec playwright install chromium`); `PLAYWRIGHT_ROOT` names
 * another installation, `MDN_BIN` another binary.
 *
 * It needs **port 7337 free** and **no `<tmpdir>/notes`**, and refuses rather
 * than working around either, because both are rendered into the shots: the
 * daemon's address in the popup and on the options page, beside help text
 * that names 7337 as the default, and the notes root's path in the app. With
 * a daemon of your own on 7337, give the run a network namespace rather than
 * stopping it:
 *
 *     unshare --user --map-root-user --net -- \
 *       sh -c 'ip link set lo up; make extension-screenshots'
 *
 * `make extension-screenshots` then runs `oxipng -o max --strip safe` over
 * the four files: lossless, and about a third smaller. Running this script on
 * its own skips that step, so do it by hand before committing.
 *
 * Consecutive runs give the same pictures, and usually the same bytes. Two
 * things can still move them: the date, which the daemon names a clip for and
 * which is in the app's navigator in one shot, and a few antialiased pixels
 * along a border. Look at what changed before committing a regenerated shot.
 */

import { execFileSync, spawn } from "node:child_process";
import { createServer } from "node:net";
import { createServer as createHttpServer } from "node:http";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import crypto from "node:crypto";
import { fileURLToPath } from "node:url";
import { loadPlaywright, missingPrerequisite } from "../e2e/prerequisites.mjs";

const here = path.dirname(fileURLToPath(import.meta.url));
const extensionDir = path.resolve(here, "..");
const repoRoot = path.resolve(extensionDir, "..");
const dist = path.join(extensionDir, "dist");
const outDir = path.join(repoRoot, "docs", "images", "extension");
const mdnBin = process.env.MDN_BIN ?? path.join(repoRoot, "mdn");

const WIDTH = 1280;
const HEIGHT = 800;

/**
 * The port the daemon runs on here, which is its own default.
 *
 * Two of the shots render the daemon's address — the options page above its
 * own help text saying the default is `http://localhost:7337`, and the popup
 * in its header line — so an ephemeral port makes one screenshot contradict
 * itself in front of strangers and makes both of them differ on every run.
 *
 * It has to be the real port: the daemon answers only to a Host header naming
 * the port it was started with (`internal/server/server.go`, the loopback
 * guard), which is a defence against DNS rebinding and not one to work
 * around. So this refuses to run rather than rendering an address the app did
 * not produce. The `unshare` line in this file's header runs it without
 * stopping a daemon you already have on 7337.
 */
const DAEMON_PORT = 7337;

/**
 * The name the clipped page is served under.
 *
 * `.example` is reserved by RFC 2606 and can never be a real site, so nothing
 * is impersonated; Chromium is told to resolve it to the fixture server with
 * `--host-resolver-rules`, which maps the name below the URL layer, so the
 * address in the shot — and the `source:` the clip records — is
 * `http://theslowweb.example/...` with no port in it. The page is really
 * served, really clipped, and really saved; only the name is arranged.
 */
const SITE_HOST = "theslowweb.example";
const SITE_ORIGIN = `http://${SITE_HOST}`;

function refuse(why) {
  console.error(`cannot take the screenshots: ${why}`);
  process.exit(1);
}

/** Whether nothing is listening on `port` here. */
function portIsFree(port) {
  return new Promise((resolve) => {
    const server = createServer();
    server.on("error", () => resolve(false));
    server.listen(port, "127.0.0.1", () => server.close(() => resolve(true)));
  });
}

async function waitFor(predicate, what, timeoutMs = 20000) {
  const deadline = Date.now() + timeoutMs;
  for (;;) {
    const value = await predicate();
    if (value) return value;
    if (Date.now() > deadline) throw new Error(`timed out waiting for ${what}`);
    await new Promise((r) => setTimeout(r, 100));
  }
}

/** Chromium's id for an unpacked extension: sha256 of its path, hex mapped a-p. */
function unpackedExtensionId(dir) {
  return crypto
    .createHash("sha256")
    .update(dir)
    .digest("hex")
    .slice(0, 32)
    .replace(/[0-9a-f]/g, (c) => String.fromCharCode(97 + parseInt(c, 16)));
}

/** A page worth clipping: an article with the furniture Readability drops. */
const ARTICLE = `<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8" />
  <title>What a note is for — The Slow Web</title>
  <style>
    :root { color-scheme: light }
    body { margin: 0; font: 17px/1.65 Georgia, "Times New Roman", serif; color: #23201c; background: #fdfcfa }
    header { border-bottom: 1px solid #e6e1da; padding: 14px 0; font: 13px/1 system-ui, sans-serif; letter-spacing: .12em; text-transform: uppercase; color: #8a8175 }
    .wrap { max-width: 42rem; margin: 0 auto; padding: 0 2rem }
    h1 { font-size: 2.1rem; line-height: 1.15; margin: 2.2rem 0 .4rem; letter-spacing: -.01em }
    .byline { font: 14px/1 system-ui, sans-serif; color: #8a8175; margin: 0 0 2rem }
    h2 { font-size: 1.25rem; margin: 2rem 0 .6rem }
    blockquote { margin: 1.6rem 0; padding-left: 1.1rem; border-left: 3px solid #d8d1c7; color: #5d574e; font-style: italic }
    ul { padding-left: 1.2rem }
    footer { margin-top: 3rem; border-top: 1px solid #e6e1da; padding: 1rem 0 3rem; font: 13px/1.6 system-ui, sans-serif; color: #8a8175 }
    nav a { color: #8a8175; text-decoration: none; margin-right: 1.4rem }
  </style>
</head>
<body>
  <header><div class="wrap"><nav><a href="/">The Slow Web</a><a href="/archive">Archive</a><a href="/about">About</a></nav></div></header>
  <div class="wrap">
    <article>
      <h1>What a note is for</h1>
      <p class="byline">Ana Reyes · 14 March</p>
      <p>A note is not a filing cabinet and it is not a diary. It is the place you
        put the thing you will need in six months and cannot yet name. The test of
        a system for keeping them is not how neatly it files, but whether you still
        reach for it on the day you are busy and tired and have thirty seconds.</p>
      <h2>Three things worth keeping</h2>
      <ul>
        <li>The paragraph that changed your mind, with the address it came from.</li>
        <li>The command that worked, not the four that did not.</li>
        <li>The name of the person who told you, because you will want to ask them again.</li>
      </ul>
      <blockquote>The best time to write a note is while you still believe you will
        remember without one.</blockquote>
      <p>Everything else is housekeeping. A page you clip today is worth more as
        three sentences you can find than as a perfect archive you never open, and
        the difference between the two is almost always the number of steps between
        reading a thing and having kept it.</p>
      <p>Which is the whole argument for keeping your notes as plain files on a
        disk you own: there is no step at the end where something has to be
        exported, and no service between you and the sentence you wrote down.</p>
    </article>
    <footer>Comments (14) · Subscribe · Older: The case against the reading list</footer>
  </div>
</body>
</html>`;

/** The 1280×800 frame every screenshot is composed onto. */
function frame(caption, blurb, inner) {
  return `<!doctype html>
<html lang="en"><head><meta charset="utf-8" /><style>
  * { box-sizing: border-box }
  html, body { margin: 0; width: ${WIDTH}px; height: ${HEIGHT}px; overflow: hidden }
  body {
    background: linear-gradient(155deg, #f4f1ec 0%, #e8e3da 55%, #ded7cb 100%);
    font: 15px/1.5 system-ui, -apple-system, "Segoe UI", sans-serif;
    color: #2b2723;
    display: flex; flex-direction: column; align-items: center;
    padding: 34px 40px 40px;
  }
  h1 { font-size: 27px; line-height: 1.2; margin: 0 0 6px; letter-spacing: -.015em; font-weight: 650 }
  p.blurb { margin: 0 0 22px; font-size: 15.5px; color: #6a6157; max-width: 62ch; text-align: center }
  header { text-align: center }
  /* min-height:0 is what lets a flex item be *shorter* than its content, and
     so what makes the max-height below bite: without it a shot taller than the
     frame pushes its own bottom edge, rounded corner and shadow off the
     screenshot. */
  .stage { position: relative; flex: 1; min-height: 0; width: 100%; display: flex; align-items: flex-start; justify-content: center }
  .shot {
    max-height: 100%;
    border-radius: 9px; overflow: hidden; background: #fff;
    box-shadow: 0 1px 2px rgba(40,34,28,.18), 0 16px 40px -12px rgba(40,34,28,.42);
    border: 1px solid rgba(40,34,28,.12);
  }
  .shot img { display: block; max-width: 100%; max-height: 100%; width: auto; height: auto }
  .float { position: absolute; right: 6px; top: 26px }
</style></head>
<body>
  <header><h1>${caption}</h1><p class="blurb">${blurb}</p></header>
  <div class="stage">${inner}</div>
</body></html>`;
}

/**
 * One capture on the frame, at most `width` wide and never taller than the
 * frame leaves room for — the width is the intent, the height is the limit.
 */
const shot = (png, width, cls = "") =>
  `<div class="shot ${cls}" style="max-width:${width}px"><img src="data:image/png;base64,${png.toString(
    "base64",
  )}" /></div>`;

async function main() {
  // The e2e suites' own list of what has to be there, the browser download
  // included, so a missing piece is a sentence here too and not a stack trace.
  const playwright = loadPlaywright();
  const missing = missingPrerequisite(playwright);
  if (missing !== null) refuse(missing);
  if (!(await portIsFree(DAEMON_PORT))) {
    refuse(
      `port ${DAEMON_PORT} is in use. It is the daemon's default, and the shots render the ` +
        "address the extension is configured with, so they are taken on it or not at all. " +
        "Stop whatever is listening — most likely your own `mdn serve` — or run this in a " +
        "network namespace of its own; the header of extension/scripts/screenshots.mjs has the command.",
    );
  }

  const tmp = fs.realpathSync(fs.mkdtempSync(path.join(os.tmpdir(), "mdn-extension-shots-")));

  // The app's header shows the notes root's real path, so it is in every shot
  // that shows the app: it has to be a fixed one, and one that reads like
  // somebody's notes rather than like a scratch directory. Falling back to a
  // random name when this one is taken was the first version of this, and it
  // was wrong twice over — a run that died left the directory behind, and
  // every run after it then rendered a different random path into two of the
  // four shots. Deterministic or not at all, the same rule as the port.
  //
  // Every refusal above this line happens before anything is created, which is
  // the point of them being up there: a run that refused for a busy 7337 after
  // making this directory would leave it behind, and the README's own
  // `unshare` recovery would then refuse on the directory instead.
  const notesDir = path.join(fs.realpathSync(os.tmpdir()), "notes");
  if (fs.existsSync(notesDir)) {
    refuse(
      `${notesDir} already exists. It is where this script puts the notes root it renders ` +
        "into the screenshots, and it will not write over a directory it did not create. " +
        `If it is this script's own leftover from a run that died, \`rm -rf ${notesDir}\`.`,
    );
  }
  fs.mkdirSync(path.join(notesDir, "reading"), { recursive: true });
  fs.writeFileSync(
    path.join(notesDir, "index.md"),
    "# Notes\n\nEverything worth keeping, as files on a disk you own.\n",
  );
  fs.writeFileSync(
    path.join(notesDir, "reading", "on note-taking.md"),
    [
      "---",
      "title: On note-taking",
      "tags: [reading, method]",
      "---",
      "",
      "# On note-taking",
      "",
      "Opened from a `file:` URL in the file manager, and rendered here instead of",
      "as plain text in a browser tab.",
      "",
      "## What survives a year",
      "",
      "- The paragraph that changed my mind, with the address it came from.",
      "- The command that worked.",
      "- Who told me.",
      "",
      "> The best time to write a note is while you still believe you will remember",
      "> without one.",
      "",
      "```sh",
      "mdn serve          # the daemon behind the extension",
      "mdn token          # the token you paste into the options page",
      "```",
      "",
      "| Kept | Where |",
      "| --- | --- |",
      "| Clips from the browser | `clips/` |",
      "| Reading notes | `reading/` |",
      "",
    ].join("\n"),
  );

  const tokenFile = path.join(tmp, "token");
  const token = execFileSync(mdnBin, ["token", "--token-file", tokenFile], { encoding: "utf8" }).trim();

  const appOrigin = `http://localhost:${DAEMON_PORT}`;
  // An explicit configuration file, as the e2e harness passes, so your own
  // ~/.config/mdn is neither read nor written by a run.
  const configPath = path.join(tmp, "config.yaml");
  fs.writeFileSync(configPath, `notes_root: ${notesDir}\n`);
  const daemon = spawn(
    mdnBin,
    [
      "serve",
      "--config", configPath,
      "--root", notesDir,
      "--port", String(DAEMON_PORT),
      "--state", path.join(tmp, "state.json"),
      "--token-file", tokenFile,
    ],
    { stdio: ["ignore", "ignore", "inherit"] },
  );
  await waitFor(() => fetch(`${appOrigin}/api/roots`).then(() => true).catch(() => false), "the daemon to listen");

  // The fixture site keeps an ephemeral port, because nothing ever sees it:
  // Chromium is told below to resolve SITE_HOST to it, so every address in
  // the screenshots and in the clip's frontmatter is the portless name.
  const site = createHttpServer((_req, res) => {
    res.setHeader("content-type", "text/html; charset=utf-8");
    res.end(ARTICLE);
  });
  await new Promise((r) => site.listen(0, "127.0.0.1", r));
  const sitePort = site.address().port;
  const articleUrl = `${SITE_ORIGIN}/2026/what-a-note-is-for/`;

  // A copy of dist/, so the shipped manifest keeps its narrow host permission
  // while this run grants the addresses it needs. The article's origin stands
  // in for the `activeTab` grant a real click makes.
  const extDir = path.join(tmp, "ext");
  fs.cpSync(dist, extDir, { recursive: true });
  const manifestPath = path.join(extDir, "manifest.json");
  const manifest = JSON.parse(fs.readFileSync(manifestPath, "utf8"));
  manifest.host_permissions = [`${appOrigin}/*`, `${SITE_ORIGIN}/*`, "file:///*"];
  fs.writeFileSync(manifestPath, JSON.stringify(manifest, null, 2));

  const extensionId = unpackedExtensionId(extDir);
  const profile = path.join(tmp, "profile");
  fs.mkdirSync(path.join(profile, "Default"), { recursive: true });
  // "Allow access to file URLs" is a per-extension profile setting with no
  // command-line flag, so it is seeded into the profile.
  fs.writeFileSync(
    path.join(profile, "Default", "Preferences"),
    JSON.stringify({ extensions: { settings: { [extensionId]: { newAllowFileAccess: true } } } }),
  );

  const context = await playwright.chromium.launchPersistentContext(profile, {
    headless: true,
    channel: "chromium",
    viewport: { width: WIDTH, height: HEIGHT },
    args: [
      `--disable-extensions-except=${extDir}`,
      `--load-extension=${extDir}`,
      // The fixture's name resolved to the fixture's port, below the URL
      // layer, so the page is served here and addressed as itself.
      `--host-resolver-rules=MAP ${SITE_HOST} 127.0.0.1:${sitePort}`,
    ],
  });

  try {
    const worker =
      context.serviceWorkers()[0] ?? (await context.waitForEvent("serviceworker", { timeout: 30000 }));
    await worker.evaluate((s) => chrome.storage.local.set(s), { daemonUrl: appOrigin, token });

    fs.mkdirSync(outDir, { recursive: true });
    const write = async (name, html) => {
      const page = await context.newPage();
      await page.setViewportSize({ width: WIDTH, height: HEIGHT });
      await page.setContent(html);
      await page.waitForLoadState("load");
      const file = path.join(outDir, name);
      await page.screenshot({ path: file });
      await page.close();
      console.log(`${path.relative(repoRoot, file)}`);
    };

    // 1. The clip, mid-flight: the article behind, the popup holding it.
    const article = await context.newPage();
    // 1000×680 scaled to 880 wide is 598 tall, which fits under the caption
    // with room to spare: a shot taller than the frame is clipped by the
    // frame, and a web page clipped mid-paragraph loses its bottom corner.
    await article.setViewportSize({ width: 1000, height: 680 });
    await article.goto(articleUrl);
    const tabId = await waitFor(
      () =>
        worker.evaluate(async (u) => {
          const tabs = await chrome.tabs.query({});
          const tab = tabs.find((t) => t.url === u);
          return tab === undefined ? null : tab.id;
        }, articleUrl),
      "the article's tab id",
    );
    const articlePng = await article.screenshot();

    const popup = await context.newPage();
    await popup.setViewportSize({ width: 360, height: 420 });
    await popup.goto(`chrome-extension://${extensionId}/popup.html?tab=${tabId}`);
    await popup.waitForFunction(() => document.getElementById("daemon").textContent !== "");
    await popup.click("#clip-page");
    await popup.waitForSelector("#clip:not([hidden])");
    await popup.fill("#clip-title", "What a note is for");
    const popupPng = await popup.screenshot({ clip: { x: 0, y: 0, width: 360, height: 340 } });

    await write(
      "01-clip-a-page.png",
      frame(
        "Clip a web page into your own notes",
        "Readability keeps the article and drops the navigation; Turndown turns it into markdown. You see the title before anything is saved.",
        `${shot(articlePng, 880)}${shot(popupPng, 330, "float")}`,
      ),
    );

    // 2. The clip, saved: the daemon wrote a file, and the app is showing it.
    await popup.click("#clip-save");
    await popup.waitForSelector("#clip-status.ok");
    const noteHref = await popup.getAttribute("#clip-open", "href");
    if (noteHref === null) throw new Error("the popup offered no link to the saved note");
    await popup.close();
    await article.close();

    const saved = await context.newPage();
    await saved.setViewportSize({ width: 1180, height: 660 });
    await saved.goto(noteHref);
    await saved.waitForSelector("text=What a note is for", { timeout: 20000 });
    const savedPng = await saved.screenshot();
    await saved.close();
    await write(
      "02-saved-to-your-notes.png",
      frame(
        "It lands in your notes as a plain markdown file",
        "Under clips/, named for the date and the title, with the address it came from in the frontmatter. Your disk, your files, no account anywhere.",
        shot(savedPng, 1140),
      ),
    );

    // 3. A local markdown file, opened through the file-URL intercept.
    const fileUrl = `file://${path.join(notesDir, "reading", "on note-taking.md")}`;
    const local = await context.newPage();
    await local.setViewportSize({ width: 1180, height: 660 });
    await local.goto(fileUrl);
    await local.waitForURL((url) => url.origin === appOrigin, { timeout: 20000 });
    await local.waitForSelector("text=What survives a year", { timeout: 20000 });
    const localPng = await local.screenshot();
    await local.close();
    await write(
      "03-local-files-open-in-the-app.png",
      frame(
        "Local markdown files open in md-notes, not as plain text",
        "Navigate to a .md file from your file manager or the command line and the tab lands on that note, rendered, in the app on your own machine.",
        shot(localPng, 1140),
      ),
    );

    // 4. The whole of the configuration.
    const options = await context.newPage();
    await options.setViewportSize({ width: 860, height: 600 });
    await options.goto(`chrome-extension://${extensionId}/options.html`);
    await options.waitForFunction(() => document.getElementById("daemon-url").value !== "");
    await options.click("#test");
    await options.waitForFunction(
      () => document.querySelector("#status").textContent !== "testing…",
      null,
      { timeout: 20000 },
    );
    const optionsPng = await options.screenshot({ clip: { x: 0, y: 0, width: 860, height: 560 } });
    await options.close();
    await write(
      "04-one-address-one-token.png",
      frame(
        "Two settings, and nothing leaves the browser but this",
        "The address of your own daemon and the token it printed. There is no account, no server of ours, and no third party: page content goes to that address and nowhere else.",
        shot(optionsPng, 900),
      ),
    );
  } finally {
    await context.close();
    site.close();
    daemon.kill("SIGTERM");
    fs.rmSync(tmp, { recursive: true, force: true });
    fs.rmSync(notesDir, { recursive: true, force: true });
  }
}

await main();
