/**
 * The rig the ui/e2e suites share: a temporary notes root, one `mdn serve`
 * over it, one headless Chromium, and the device profiles the checks are
 * made at.
 *
 * Playwright is resolved the way `extension/e2e` resolves it — an explicit
 * `PLAYWRIGHT_ROOT`, then the packages that might declare it — so the two
 * suites share one installation and one browser download. It is declared by
 * `ui/package.json`, which `make ui-deps` installs; the browser itself is a
 * separate download (`pnpm --dir ui exec playwright install chromium`), and
 * without it every test here skips rather than fails.
 *
 *     make e2e
 *     pnpm --dir ui e2e
 *     PLAYWRIGHT_ROOT=/elsewhere pnpm --dir ui e2e
 *
 * Nothing here touches the operator's daemon, configuration or state: the
 * root, the `--state` file, the `--token-file` and the `--config` are all
 * inside a `mkdtemp` this process removes, and the port comes from the
 * kernel.
 */

import { createRequire } from "node:module";
import { execFileSync, spawn } from "node:child_process";
import { createServer } from "node:net";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import { fileURLToPath } from "node:url";

const here = path.dirname(fileURLToPath(import.meta.url));
export const uiDir = path.resolve(here, "..");
export const repoRoot = path.resolve(uiDir, "..");
const extensionDir = path.join(repoRoot, "extension");
export const mdnBin = process.env.MDN_BIN ?? path.join(repoRoot, "mdn");

export function loadPlaywright() {
  const roots = [process.env.PLAYWRIGHT_ROOT, uiDir, extensionDir, repoRoot].filter(Boolean);
  for (const root of roots) {
    try {
      const require = createRequire(path.join(root, "noop.js"));
      return require("playwright");
    } catch {
      // try the next place
    }
  }
  return null;
}

/** Why the suite cannot run here, or null when it can. */
export function missingPrerequisite(playwright) {
  if (playwright === null) return "playwright is not installed (set PLAYWRIGHT_ROOT)";
  if (!fs.existsSync(mdnBin)) return `no mdn binary at ${mdnBin} (set MDN_BIN, or run make build)`;
  if (!fs.existsSync(path.join(uiDir, "dist", "index.html"))) return "ui/dist is not built (run make ui)";
  return null;
}

export function freePort() {
  return new Promise((resolve, reject) => {
    const server = createServer();
    server.on("error", reject);
    server.listen(0, "127.0.0.1", () => {
      const { port } = server.address();
      server.close(() => resolve(port));
    });
  });
}

/** Polls a condition rather than sleeping on a guess. */
export async function waitFor(predicate, what, timeoutMs = 20000) {
  const deadline = Date.now() + timeoutMs;
  for (;;) {
    const value = await predicate();
    if (value) return value;
    if (Date.now() > deadline) throw new Error(`timed out waiting for ${what}`);
    await new Promise((r) => setTimeout(r, 50));
  }
}

/**
 * The fixture tree. Small on purpose: three directories, a tag on two notes
 * so the filter has something to prune to, one long note whose paragraphs
 * carry the `data-line` markers the `?l=` checks aim at, and `docs/guide.md`
 * — a note in a folder, which is what the create prompt's "bare title lands
 * in the open note's folder" rule needs to be shown on.
 */
export const NESTED_PARAGRAPHS = 120;
/** The source line of the paragraph the scroll-to-line checks ask for. */
export const NESTED_LINE = 101;

function writeFixture(notesDir) {
  const write = (rel, text) => {
    const file = path.join(notesDir, rel);
    fs.mkdirSync(path.dirname(file), { recursive: true });
    fs.writeFileSync(file, text);
  };
  write("index.md", "# Notes\n\nThe root note.\n");
  write("alpha.md", "---\ntags: [alpha, beta]\n---\n\n# Alpha\n\nA note tagged alpha and beta.\n");
  write("beta.md", "---\ntags: [alpha]\n---\n\n# Beta\n\nAnother note tagged alpha.\n");
  write("projects/one.md", "# One\n\nThe first project note.\n");
  write("docs/guide.md", "# Guide\n\nA note in a folder.\n");
  write("archive/old.md", "# Old\n\nAn archived note.\n");
  const lines = ["# Nested", ""];
  for (let i = 1; i <= NESTED_PARAGRAPHS; i++) lines.push(`Paragraph ${i} of the nested note.`, "");
  write("projects/deep/nested.md", lines.join("\n"));
}

/**
 * A temporary root with a daemon over it. `stop()` kills the daemon by its
 * own handle — never by name — waits for the port to close, and removes the
 * tree.
 */
export async function startFixture(label) {
  const tmp = fs.realpathSync(fs.mkdtempSync(path.join(os.tmpdir(), `mdn-ui-e2e-${label}-`)));
  const notesDir = path.join(tmp, "notes");
  fs.mkdirSync(notesDir, { recursive: true });
  writeFixture(notesDir);

  const tokenFile = path.join(tmp, "token");
  execFileSync(mdnBin, ["token", "--token-file", tokenFile], { encoding: "utf8" });

  // An explicit configuration file, so a developer's own ~/.config/mdn is
  // neither read nor written by a test run.
  const configPath = path.join(tmp, "config.yaml");
  fs.writeFileSync(configPath, `notes_root: ${notesDir}\n`);

  const port = await freePort();
  const origin = `http://127.0.0.1:${port}`;
  const daemon = spawn(
    mdnBin,
    [
      "serve",
      "--config", configPath,
      "--root", notesDir,
      "--port", String(port),
      "--state", path.join(tmp, "state.json"),
      "--token-file", tokenFile,
    ],
    { stdio: ["ignore", "pipe", "pipe"] },
  );
  const log = [];
  daemon.stdout.on("data", (d) => log.push(String(d)));
  daemon.stderr.on("data", (d) => log.push(String(d)));
  daemon.on("exit", (code) => log.push(`daemon exited with ${code}\n`));

  // Everything between the spawn and the handle below has to hand the daemon
  // back if it throws. A child with its stdio pipes attached keeps the
  // node:test worker's event loop alive, so a bare `throw` here does not fail
  // the file — it hangs it, with the message computed and never flushed, and
  // leaves an `mdn serve` and a temporary tree behind. That is worse than the
  // failure the ripgrep check below was added to explain, and it is what the
  // review of PR #84 found. SIGKILL rather than SIGTERM: the run is over,
  // there is nothing to flush, and a graceful stop is one more thing that can
  // wedge and hang the worker all over again.
  try {
    await waitFor(
      () => fetch(`${origin}/api/roots`).then((r) => r.ok).catch(() => false),
      `the daemon to listen on ${port}:\n${log.join("")}`,
    );

    // The daemon lists a root through ripgrep. Without it the navigator is
    // empty and a dozen checks fail on a missing `.tree` row, each of them
    // 30 seconds of Playwright waiting for an element that was never coming;
    // asked here, the answer arrives once and says what is actually wrong.
    const tree = await fetch(`${origin}/api/r/notes/tree`);
    if (!tree.ok) {
      throw new Error(
        `the daemon cannot list the fixture root (HTTP ${tree.status}): ${(await tree.text()).trim()}\n` +
          "ripgrep (rg) on PATH is a runtime requirement of the daemon, not only of its tests.",
      );
    }
    const listed = await tree.json();
    if (!listed.children || listed.children.length === 0) {
      throw new Error(`the fixture root listed empty: ${JSON.stringify(listed)}`);
    }
  } catch (e) {
    daemon.kill("SIGKILL");
    fs.rmSync(tmp, { recursive: true, force: true });
    throw e;
  }

  return {
    tmp,
    notesDir,
    origin,
    // roots.slugify() takes the folder's basename, and the folder is "notes".
    slug: "notes",
    /** Where a note in the fixture lives on disk. */
    file: (rel) => path.join(notesDir, rel),
    /** The app's URL for a note. */
    url: (rel = "", query = "") => `${origin}/r/notes/${rel}${query}`,
    log: () => log.join(""),
    async stop() {
      daemon.kill("SIGTERM");
      await waitFor(
        () => fetch(`${origin}/api/roots`).then(() => false).catch(() => true),
        "the daemon to stop",
      );
      fs.rmSync(tmp, { recursive: true, force: true });
    },
  };
}

/**
 * The device profiles, spelled out rather than taken from
 * `playwright.devices`: that registry's viewports move between Playwright
 * releases — 1.63 has Pixel 7 at 412x839 where the M4 harness measured
 * 412x792 — and a ported figure that changes with a dependency bump is a
 * check that fails for no regression.
 *
 * All four are the viewports M4 measured, in headless Chromium, which is the
 * only browser CI downloads: 412x792 and 863x360 for the Pixel 7, 390x664 and
 * 750x340 for the iPhone 14 (davison/md-notes#55, and PR #69's table). The
 * landscape pair is not the portrait pair with its numbers swapped — the
 * browser's own chrome is a different height when the device is on its side —
 * which is why all four are written out rather than two of them derived.
 */
export const PHONES = [
  { name: "Pixel 7 portrait", viewport: { width: 412, height: 792 }, deviceScaleFactor: 2.625, isMobile: true, hasTouch: true },
  { name: "Pixel 7 landscape", viewport: { width: 863, height: 360 }, deviceScaleFactor: 2.625, isMobile: true, hasTouch: true },
  { name: "iPhone 14 portrait", viewport: { width: 390, height: 664 }, deviceScaleFactor: 3, isMobile: true, hasTouch: true },
  { name: "iPhone 14 landscape", viewport: { width: 750, height: 340 }, deviceScaleFactor: 3, isMobile: true, hasTouch: true },
];

/** The profile the M4 record's "On a phone" figures were measured at. */
export const PIXEL_7 = PHONES[0];

/** A mouse-driven window well above the breakpoint: the wide layout's control. */
export const DESKTOP = { name: "desktop", viewport: { width: 1440, height: 900 } };

/**
 * The same window under a stylus or a finger. `hasTouch` is what makes
 * Chromium report `(pointer: coarse)` and `(hover: none)`, which is the
 * clause the 40 px targets hang on above the narrow breakpoint (#62).
 */
export const COARSE_DESKTOP = { name: "desktop, coarse pointer", viewport: { width: 1440, height: 900 }, hasTouch: true };

/** The narrow breakpoint, in CSS pixels: 60rem against the initial 16 px font. */
export const BREAKPOINT = 960;

/** The tap-target floor the e-ink task set, in CSS pixels. */
export const TAP_TARGET = 40;

/** A rounded rectangle for an element, or null when it is not in the page. */
export function rect(page, selector) {
  return page.evaluate((s) => {
    const el = document.querySelector(s);
    if (!el) return null;
    const b = el.getBoundingClientRect();
    const round = (n) => Math.round(n * 100) / 100;
    return { width: round(b.width), height: round(b.height), x: round(b.x), y: round(b.y) };
  }, selector);
}

/** Opens a note and waits for it to be rendered rather than merely routed. */
export async function openNote(page, url) {
  await page.goto(url);
  await page.waitForSelector(".note-body .markdown", { state: "attached" });
  return page;
}
