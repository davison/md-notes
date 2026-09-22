/**
 * Takes the app's screenshots, into `docs/images/app/`.
 *
 *     make screenshots
 *
 * The README shows these. Each is a capture of the real app: the built
 * `./mdn` serving a copy of `ui/scripts/demo-notes/` — a small notes folder
 * invented for the purpose, with headings, lists, a code block, tables and
 * two mermaid flowcharts — in headless Chromium. Nothing is mocked or drawn
 * by hand, so a screenshot that stops matching the product fails to
 * regenerate rather than quietly lying on the front page.
 *
 * Every shot comes in a light and a dark version, so the README can follow
 * the reader's own colour scheme with a `<picture>`:
 *
 * - `wide-{light,dark}.png`: the three panes at 1440×900 — the navigator,
 *   a note with a table and a code block, and a search with its hits;
 * - `phone-{light,dark}.png`: the same app on a 412×792 phone, at 2× so it
 *   stays sharp when the page shrinks it;
 * - `flowchart-{light,dark}.png`: a mermaid flowchart in the reading view,
 *   drawn by the daemon;
 * - `editor-{light,dark}.png`: the same note flipped to the editor.
 *
 * Nothing here touches your own daemon, configuration or state: the port
 * comes from the kernel, and the `--config`, `--state` and `--token-file`
 * are inside a `mkdtemp` this process removes. The notes root is the one
 * fixed path, `<tmpdir>/notes`, because the app's header shows the root's
 * path and a random one would read as a scratch directory and change on
 * every run. The extension's screenshot script uses the same path for the
 * same reason, so the two cannot run at once, and this refuses rather than
 * writing into a directory it did not create.
 *
 * Playwright and its Chromium are the ones `ui/` installs (`make ui-deps`,
 * then `pnpm --dir ui exec playwright install chromium`); `PLAYWRIGHT_ROOT`
 * names another installation, `MDN_BIN` another binary.
 *
 * `make screenshots` then runs `oxipng -o max --strip safe` over the files,
 * which is lossless. Running this script on its own skips that step, so do
 * it by hand before committing. Look at what changed before committing a
 * regenerated shot: a few antialiased pixels can move between runs.
 */

import { execFileSync, spawn } from "node:child_process";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { freePort, loadPlaywright, missingPrerequisite, repoRoot, mdnBin, waitFor } from "../e2e/harness.mjs";

const here = path.dirname(fileURLToPath(import.meta.url));
const demo = path.join(here, "demo-notes");
const outDir = path.join(repoRoot, "docs", "images", "app");

const WIDE = { viewport: { width: 1440, height: 900 } };
// Pixel 7 portrait, as the e2e suites measure it, at 2× rather than the
// device's 2.625: sharp enough when the README draws it a third of that
// width, and a much smaller file.
const PHONE = { viewport: { width: 412, height: 792 }, deviceScaleFactor: 2, isMobile: true, hasTouch: true };
// A window only as wide as the note needs, so the flowchart fills the frame.
const NOTE_ONLY = { viewport: { width: 1100, height: 760 } };

function refuse(why) {
  console.error(`cannot take the screenshots: ${why}`);
  process.exit(1);
}

async function main() {
  const playwright = loadPlaywright();
  const missing = missingPrerequisite(playwright);
  if (missing !== null) refuse(missing);

  const notesDir = path.join(fs.realpathSync(os.tmpdir()), "notes");
  if (fs.existsSync(notesDir)) {
    refuse(
      `${notesDir} already exists. It is where this script puts the notes root it renders ` +
        "into the screenshots, and it will not write over a directory it did not create. " +
        `If it is a leftover from a run that died, \`rm -rf ${notesDir}\`.`,
    );
  }
  // Everything from here on is undone in the `finally` below, whichever step
  // fails: a run that dies must say so, exit non-zero and leave neither the
  // daemon nor <tmpdir>/notes behind, or the next run refuses on the
  // directory and `make screenshots` goes on to squeeze the old pictures.
  const tmp = fs.realpathSync(fs.mkdtempSync(path.join(os.tmpdir(), "mdn-app-shots-")));
  let daemon = null;
  let exited = null;
  let browser = null;
  try {
    fs.cpSync(demo, notesDir, { recursive: true });

    // Every note the same age, so the navigator and anything that shows a time
    // come out the same from one run to the next.
    const when = new Date("2026-09-18T09:30:00Z");
    for (const entry of fs.readdirSync(notesDir, { recursive: true })) {
      fs.utimesSync(path.join(notesDir, entry), when, when);
    }

    const port = await freePort();
    const origin = `http://localhost:${port}`;
    const configPath = path.join(tmp, "config.yaml");
    fs.writeFileSync(configPath, `notes_root: ${notesDir}\n`);
    const tokenFile = path.join(tmp, "token");
    execFileSync(mdnBin, ["token", "--token-file", tokenFile]);
    daemon = spawn(
      mdnBin,
      [
        "serve",
        "--config", configPath,
        "--port", String(port),
        "--state", path.join(tmp, "state.json"),
        "--token-file", tokenFile,
      ],
      { stdio: ["ignore", "ignore", "inherit"] },
    );
    // Taken at spawn, so it settles even when the daemon has already gone by
    // the time anything awaits it: an `exit` listener added after the event
    // waits for ever, and node then drains its event loop and exits 0.
    exited = new Promise((resolve) => daemon.once("exit", (code, signal) => resolve({ code, signal })));
    // A binary that cannot be started at all emits `error` and no `exit`.
    let spawnError = null;
    daemon.once("error", (err) => {
      spawnError = err;
    });

    const running = () => spawnError === null && daemon.exitCode === null && daemon.signalCode === null;
    await waitFor(async () => {
      if (!running()) throw new Error(`the daemon ${spawnError ? `could not start: ${spawnError.message}` : `exited (${daemon.exitCode ?? daemon.signalCode})`} before it listened`);
      return fetch(`${origin}/api/roots`).then((r) => r.ok).catch(() => false);
    }, "the daemon to listen");

    browser = await playwright.chromium.launch();
    fs.mkdirSync(outDir, { recursive: true });

    /** A fresh page in `scheme`, with the three folders open in the navigator. */
    const open = async (profile, scheme, url) => {
      const context = await browser.newContext({ ...profile, colorScheme: scheme, serviceWorkers: "block" });
      await context.addInitScript(() => {
        localStorage.setItem("mdn:nav:notes", JSON.stringify(["home", "kitchen", "work"]));
      });
      const page = await context.newPage();
      await page.goto(`${origin}${url}`);
      await page.waitForSelector(".note-body .markdown", { state: "attached" });
      return [page, () => context.close()];
    };

    /** Every flowchart on the page drawn, and every image decoded. */
    const drawn = (page) =>
      page.waitForFunction(() => {
        const imgs = [...document.querySelectorAll(".markdown img.diagram")];
        return imgs.length > 0 && imgs.every((i) => i.complete && i.naturalWidth > 0);
      });

    const save = async (page, name) => {
      // Let the last layout and the fonts settle before the capture.
      await page.evaluate(() => document.fonts.ready);
      const file = path.join(outDir, name);
      await page.screenshot({ path: file });
      console.log(path.relative(repoRoot, file));
    };

    for (const scheme of ["light", "dark"]) {
      // The three panes: a note with a table and a code block, and a search.
      {
        const [page, close] = await open(WIDE, scheme, "/r/notes/home/backups.md");
        const search = page.getByRole("searchbox", { name: "Search notes" });
        await search.fill("weekend");
        await page.waitForSelector(".search .hit");
        await search.blur();
        await save(page, `wide-${scheme}.png`);
        await close();
      }

      // The phone: the same app, a note filling the screen.
      {
        const [page, close] = await open(PHONE, scheme, "/r/notes/kitchen/sourdough.md");
        await save(page, `phone-${scheme}.png`);
        await close();
      }

      // A flowchart in the reading view, scrolled to its heading.
      {
        const [page, close] = await open(NOTE_ONLY, scheme, "/r/notes/kitchen/sourdough.md");
        await drawn(page);
        await page.evaluate(() => {
          const heading = [...document.querySelectorAll(".markdown h2")].find((h) =>
            h.textContent.startsWith("How it goes wrong"),
          );
          heading.scrollIntoView({ block: "start" });
        });
        await save(page, `flowchart-${scheme}.png`);
        await close();
      }

      // The editor: the same note, one keystroke later.
      {
        const [page, close] = await open(WIDE, scheme, "/r/notes/home/backups.md");
        await page.keyboard.press("Control+e");
        await page.locator(".cm-editor").waitFor();
        await save(page, `editor-${scheme}.png`);
        await close();
      }
    }
    if (!running()) throw new Error(`the daemon exited (${daemon.exitCode ?? daemon.signalCode}) during the run`);
  } finally {
    if (browser !== null) await browser.close().catch(() => {});
    if (daemon !== null && daemon.pid !== undefined && daemon.exitCode === null && daemon.signalCode === null) {
      daemon.kill("SIGTERM");
      await Promise.race([exited, new Promise((r) => setTimeout(r, 5000))]);
      if (daemon.exitCode === null && daemon.signalCode === null) daemon.kill("SIGKILL");
    }
    fs.rmSync(tmp, { recursive: true, force: true });
    fs.rmSync(notesDir, { recursive: true, force: true });
  }
}

main().catch((err) => {
  console.error(err);
  process.exit(1);
});
