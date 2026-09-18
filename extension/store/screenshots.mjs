/**
 * Regenerates the Chrome Web Store screenshots in `screenshots/`.
 *
 *     make build extension
 *     node extension/store/screenshots.mjs
 *
 * The store wants 1280×800 or 640×400; these are 1280×800. Every one of them
 * is the real extension doing the real thing — a daemon started here on a
 * temporary notes root, the built extension loaded into Chromium, a clip taken
 * from a page served here and saved for real — composed onto a captioned
 * 1280×800 frame. Nothing is mocked and nothing is drawn by hand, so a
 * screenshot that stops matching the product is a screenshot that fails to
 * regenerate rather than one that quietly lies in the listing.
 *
 * The harness is the extension's own e2e suite (`extension/e2e/`), which is
 * where the awkward parts come from: the unpacked extension id, the profile
 * preference that stands in for "Allow access to file URLs", and opening the
 * popup as a tab because a browser-action popup cannot be clicked from a
 * script. Playwright is `ui/package.json`'s, installed by `make ui-deps`;
 * `PLAYWRIGHT_ROOT` names another installation.
 *
 * Written for davison/md-notes#135 (M8-R2).
 */

import { createRequire } from "node:module";
import { execFileSync, spawn } from "node:child_process";
import { createServer } from "node:net";
import { createServer as createHttpServer } from "node:http";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import crypto from "node:crypto";
import { fileURLToPath } from "node:url";

const here = path.dirname(fileURLToPath(import.meta.url));
const extensionDir = path.resolve(here, "..");
const repoRoot = path.resolve(extensionDir, "..");
const dist = path.join(extensionDir, "dist");
const outDir = path.join(here, "screenshots");
const mdnBin = process.env.MDN_BIN ?? path.join(repoRoot, "mdn");

const WIDTH = 1280;
const HEIGHT = 800;

function loadPlaywright() {
  const roots = [process.env.PLAYWRIGHT_ROOT, extensionDir, path.join(repoRoot, "ui"), repoRoot].filter(
    Boolean,
  );
  for (const root of roots) {
    try {
      return createRequire(path.join(root, "noop.js"))("playwright");
    } catch {
      // try the next place
    }
  }
  return null;
}

function refuse(why) {
  console.error(`cannot take the screenshots: ${why}`);
  process.exit(1);
}

function freePort() {
  return new Promise((resolve, reject) => {
    const server = createServer();
    server.on("error", reject);
    server.listen(0, "127.0.0.1", () => {
      const { port } = server.address();
      server.close(() => resolve(port));
    });
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
  const playwright = loadPlaywright();
  if (playwright === null) refuse("playwright is not installed (set PLAYWRIGHT_ROOT)");
  if (!fs.existsSync(path.join(dist, "manifest.json"))) refuse("extension/dist is not built (make extension)");
  if (!fs.existsSync(mdnBin)) refuse(`no mdn binary at ${mdnBin} (make build, or set MDN_BIN)`);

  const tmp = fs.realpathSync(fs.mkdtempSync(path.join(os.tmpdir(), "mdn-store-shots-")));
  // The app's header shows the notes root's real path, so it is in every
  // screenshot that shows the app. A mkdtemp name there reads as a scratch
  // directory, which is not what a notes root is; a plain one under the
  // temporary directory reads as somebody's notes. Anything already sitting at
  // that name belongs to someone else, so this falls back rather than touching
  // it — the screenshot is then a little uglier and nothing is lost.
  const preferred = path.join(fs.realpathSync(os.tmpdir()), "notes");
  const notesDir = fs.existsSync(preferred) ? path.join(tmp, "notes") : preferred;
  const removeNotes = notesDir !== path.join(tmp, "notes");
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

  const port = await freePort();
  const appOrigin = `http://localhost:${port}`;
  const daemon = spawn(
    mdnBin,
    ["serve", "--root", notesDir, "--port", String(port), "--state", path.join(tmp, "state.json"), "--token-file", tokenFile],
    { stdio: ["ignore", "ignore", "inherit"] },
  );
  await waitFor(() => fetch(`${appOrigin}/api/roots`).then(() => true).catch(() => false), "the daemon to listen");

  const site = createHttpServer((_req, res) => {
    res.setHeader("content-type", "text/html; charset=utf-8");
    res.end(ARTICLE);
  });
  await new Promise((r) => site.listen(0, "127.0.0.1", r));
  const siteOrigin = `http://127.0.0.1:${site.address().port}`;
  const articleUrl = `${siteOrigin}/2026/what-a-note-is-for/`;

  // A copy of dist/, so the shipped manifest keeps its narrow host permission
  // while this run grants the ephemeral ports it needs. The article's origin
  // stands in for the `activeTab` grant a real click makes.
  const extDir = path.join(tmp, "ext");
  fs.cpSync(dist, extDir, { recursive: true });
  const manifestPath = path.join(extDir, "manifest.json");
  const manifest = JSON.parse(fs.readFileSync(manifestPath, "utf8"));
  manifest.host_permissions = [`http://localhost:${port}/*`, `http://127.0.0.1:${port}/*`, `${siteOrigin}/*`, "file:///*"];
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
    args: [`--disable-extensions-except=${extDir}`, `--load-extension=${extDir}`],
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
    if (removeNotes) fs.rmSync(notesDir, { recursive: true, force: true });
  }
}

await main();
