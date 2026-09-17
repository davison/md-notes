/**
 * The navigator's order toggle (M7-R3, davison/md-notes#116), in the
 * browser: the two orders in the wide layout and in the drawer, a note
 * saved in the app and a note changed on disk both moving to the top with
 * no reload, and the choice surviving one.
 *
 * The ordering rules themselves are held by ui/src/order.test.ts, which
 * can state them in a dozen lines apiece. What only a browser can show is
 * that the times reach the page at all, that the live-update path keeps
 * them current, and that `localStorage` carries the choice across a load.
 */

import { after, before, beforeEach, describe, it } from "node:test";
import assert from "node:assert/strict";
import fs from "node:fs";
import {
  DESKTOP,
  PIXEL_7,
  TAP_TARGET,
  drawerReady,
  loadPlaywright,
  missingPrerequisite,
  startFixture,
  waitFor,
} from "./harness.mjs";

const playwright = loadPlaywright();
const blocker = missingPrerequisite(playwright);
if (blocker) console.log(`# skipped: ${blocker}`);

/**
 * The fixture's modification times, set rather than inherited from the
 * order the harness happened to write the files in. Every pair disagrees
 * with the alphabetical order, so a check that passes here is reading the
 * times and not the names:
 *
 *   folders, newest note beneath:  archive (Jun) · projects (May) · docs (Apr)
 *   files:                         beta (Mar) · index (Feb) · alpha (Jan)
 *
 * The alphabetical order is archive, docs, projects, alpha, beta, index.
 */
const TIMES = {
  "alpha.md": Date.UTC(2026, 0, 1),
  "index.md": Date.UTC(2026, 1, 1),
  "beta.md": Date.UTC(2026, 2, 1),
  "docs/guide.md": Date.UTC(2026, 3, 1),
  "projects/one.md": Date.UTC(2026, 4, 1),
  "projects/deep/nested.md": Date.UTC(2026, 4, 2),
  "archive/old.md": Date.UTC(2026, 5, 1),
};

const BY_NAME = ["archive", "docs", "projects", "alpha.md", "beta.md", "index.md"];
const BY_RECENCY = ["archive", "projects", "docs", "beta.md", "index.md", "alpha.md"];

/** The rows on screen, in the order they are laid out, twisty stripped. */
const rows = (page) =>
  page.evaluate(() =>
    [...document.querySelectorAll(".tree .dir, .tree .file")].map((e) =>
      e.textContent.replace(/^[▾▸]/, ""),
    ),
  );

/** Waits until the navigator shows exactly these rows, and says so when it never does. */
async function expectRows(page, want, what) {
  let last = null;
  try {
    await waitFor(async () => {
      last = await rows(page);
      return last.length === want.length && last.every((r, i) => r === want[i]);
    }, `${what}: ${want.join(", ")}`);
  } catch (e) {
    assert.deepEqual(last, want, `${what} (${e.message})`);
    throw e;
  }
}

describe("the navigator's order toggle", { skip: blocker ?? false }, () => {
  let fixture, browser;

  /**
   * The fixture is shared by the whole file — one daemon and one browser
   * are what the other suites cost too — and two of the cases below change
   * a note on purpose. Stamping the times back before each case is what
   * keeps the last case's premise from being the previous case's result;
   * the content a case wrote stays written, which nothing here reads.
   */
  const stampTimes = () => {
    for (const [rel, when] of Object.entries(TIMES)) {
      fs.utimesSync(fixture.file(rel), when / 1000, when / 1000);
    }
  };

  before(async () => {
    fixture = await startFixture("order");
    stampTimes();
    browser = await playwright.chromium.launch({ headless: true });
  });

  beforeEach(() => {
    if (fixture) stampTimes();
  });

  after(async () => {
    await browser?.close();
    await fixture?.stop();
  });

  /** A page in a fresh context — so a fresh `localStorage` — closed by the caller. */
  const page = async (profile) => {
    const { name, ...options } = profile;
    const context = await browser.newContext(options);
    const p = await context.newPage();
    const thrown = [];
    p.on("pageerror", (e) => thrown.push(e.message));
    return [
      p,
      async () => {
        await context.close();
        assert.deepEqual(thrown, [], `${name}: the page threw`);
      },
    ];
  };

  const toggle = (p) => p.getByRole("button", { name: "Recent first" });

  it("switches the wide layout between the two orders", async () => {
    const [p, close] = await page(DESKTOP);
    try {
      await p.goto(fixture.url());
      await p.locator(".tree").waitFor();

      await expectRows(p, BY_NAME, "the alphanumeric order, which is the default");
      assert.equal(await toggle(p).getAttribute("aria-pressed"), "false");

      await toggle(p).click();
      await expectRows(p, BY_RECENCY, "the recency order");
      assert.equal(await toggle(p).getAttribute("aria-pressed"), "true");

      await toggle(p).click();
      await expectRows(p, BY_NAME, "back to the alphanumeric order");
    } finally {
      await close();
    }
  });

  it("carries the choice across a reload, and across roots", async () => {
    const [p, close] = await page(DESKTOP);
    try {
      await p.goto(fixture.url());
      await p.locator(".tree").waitFor();
      await toggle(p).click();
      await expectRows(p, BY_RECENCY, "the recency order");

      await p.reload();
      await p.locator(".tree").waitFor();
      assert.equal(
        await toggle(p).getAttribute("aria-pressed"),
        "true",
        "the reloaded page forgot the order",
      );
      await expectRows(p, BY_RECENCY, "the recency order after a reload");

      // A note deep in the tree, opened cold: the choice is the browser's
      // rather than one pane's, so it is in force before anything is clicked.
      await p.goto(fixture.url("docs/guide.md"));
      await p.locator(".tree").waitFor();
      await assert.doesNotReject(() =>
        waitFor(async () => (await rows(p))[0] === "archive", "the recency order on another page"),
      );
    } finally {
      await close();
    }
  });

  it("moves a note saved in the app to the top with no reload", async () => {
    const [p, close] = await page(DESKTOP);
    try {
      // alpha.md is the oldest note in the fixture and the last row under
      // the recency order, which is what makes its arrival at the top a
      // result rather than a coincidence.
      await p.goto(fixture.url("alpha.md"));
      await p.locator(".note-bar").waitFor();
      await toggle(p).click();
      await expectRows(p, BY_RECENCY, "the recency order before the save");

      await p.getByRole("button", { name: "Edit" }).click();
      await p.locator(".cm-editor").waitFor();
      await p.locator(".cm-content").click();
      // Out of vim's normal mode before typing, the way the create-and-delete
      // suite does it.
      await p.keyboard.press("i");
      await p.waitForFunction(
        () => !document.querySelector(".cm-scroller").classList.contains("cm-vimMode"),
      );
      await p.keyboard.type("An edit made in the app.\n");
      await waitFor(
        () => p.locator(".save-status").textContent().then((t) => t === "Saved"),
        "the autosave to land",
      );

      // No reload anywhere in this case: the events stream reports the
      // path, the page refetches the tree, and the tree it gets back
      // carries the new time.
      await expectRows(
        p,
        ["archive", "projects", "docs", "alpha.md", "beta.md", "index.md"],
        "the saved note at the top of the notes, with no reload",
      );
    } finally {
      await close();
    }
  });

  it("moves a note changed on disk to the top with no reload", async () => {
    const [p, close] = await page(DESKTOP);
    try {
      await p.goto(fixture.url());
      await p.locator(".tree").waitFor();
      await toggle(p).click();
      await expectRows(p, BY_RECENCY, "the recency order before the write");

      // Written by something that is not this application — the case a
      // synced folder makes every day — so the only way the page can learn
      // of it is the watcher.
      fs.writeFileSync(fixture.file("docs/guide.md"), "# Guide\n\nRewritten on disk.\n");

      await expectRows(
        p,
        ["docs", "archive", "projects", "beta.md", "index.md", "alpha.md"],
        "the folder holding the externally changed note at the top, with no reload",
      );
    } finally {
      await close();
    }
  });

  it("offers the same control in the drawer, at a stylus-sized height", async () => {
    const [p, close] = await page(PIXEL_7);
    try {
      await p.goto(fixture.url());
      await p.locator(".nav-toggle").click();
      await drawerReady(p);

      const box = await toggle(p).boundingBox();
      assert.ok(
        box.height >= TAP_TARGET,
        `the order control is ${box.height}px tall in the drawer, under the ${TAP_TARGET}px floor`,
      );

      await toggle(p).click();
      await expectRows(p, BY_RECENCY, "the recency order in the drawer");

      // The drawer stays open: the order control is not a note link, and
      // closing the drawer would hide the tree the tap just reordered.
      assert.equal(
        await p.evaluate(() => document.querySelector(".panes").classList.contains("open")),
        true,
        "the drawer closed when the order was changed",
      );
    } finally {
      await close();
    }
  });
});
