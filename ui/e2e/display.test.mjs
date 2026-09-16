/**
 * The e-ink display checks the settings task (#62) measured in a scratchpad
 * harness and #72 asked to be brought into CI: the light override applied
 * before the first paint, the scroll-to-line flash suppressed by the
 * setting and by the device preference, and the 40 px tap targets on a
 * coarse pointer with the mouse-driven window's density left alone.
 */

import { after, before, describe, it } from "node:test";
import assert from "node:assert/strict";
import {
  COARSE_DESKTOP,
  DESKTOP,
  NESTED_LINE,
  PHONES,
  PIXEL_7,
  TAP_TARGET,
  loadPlaywright,
  missingPrerequisite,
  openNote,
  startFixture,
} from "./harness.mjs";

const playwright = loadPlaywright();
const blocker = missingPrerequisite(playwright);
if (blocker) console.log(`# skipped: ${blocker}`);

/** The two palettes, as ui/src/style.css declares them. */
const LIGHT_BG = "rgb(251, 251, 250)";
const DARK_BG = "rgb(27, 27, 27)";

/** Every control the stylesheet's tap-target block names, by its selector. */
const TAP_GROUPS = [
  ".tree .dir",
  ".tree .file",
  ".tag",
  ".tag-clear",
  ".hit",
  ".search input",
  ".drawer-tab",
  ".settings-toggle",
  ".setting",
  ".shell .brand",
  ".tag-chip",
  ".note-bar button",
  ".metadata summary",
  // `.conflict button` is on the same list and is not reachable from a
  // browser without a save racing a change on disk; the Go suite covers the
  // conflict itself, and vitest covers the markup.
];

/** The laid-out heights of each group, skipping the ones this state has none of. */
function measure(page) {
  return page.evaluate((groups) => {
    const out = {};
    for (const sel of groups) {
      const heights = [...document.querySelectorAll(sel)]
        .filter((e) => e.getClientRects().length > 0)
        .map((e) => Math.round(e.getBoundingClientRect().height * 100) / 100);
      if (heights.length > 0) out[sel] = heights;
    }
    return out;
  }, TAP_GROUPS);
}

describe("the display settings and the tap targets", { skip: blocker ?? false }, () => {
  let fixture, browser;

  before(async () => {
    fixture = await startFixture("display");
    browser = await playwright.chromium.launch({ headless: true });
  });

  after(async () => {
    await browser?.close();
    await fixture?.stop();
  });

  const context = async (profile, extra = {}, settings = null) => {
    const { name, ...options } = profile;
    const ctx = await browser.newContext({ ...options, ...extra });
    if (settings !== null) {
      await ctx.addInitScript((s) => {
        try {
          localStorage.setItem("mdn:settings", JSON.stringify(s));
        } catch {
          // nothing to do: the check that follows will say so
        }
      }, settings);
    }
    const page = await ctx.newPage();
    return [page, () => ctx.close()];
  };

  it("serves the boot script ahead of anything that paints", async () => {
    const html = await fetch(fixture.url("alpha.md")).then((r) => r.text());
    const boot = html.indexOf('localStorage.getItem("mdn:settings")');
    const sheet = html.search(/<link[^>]+stylesheet/i);
    assert.ok(boot >= 0, "the inline boot script is in the served shell");
    assert.ok(sheet >= 0, "the shell links a stylesheet");
    assert.ok(boot < sheet, "the attributes are set before the stylesheet that reads them");
  });

  // The bundle is aborted, so nothing but that inline script can have run:
  // whatever the page is painting, it is painting it on the first paint.
  for (const [what, settings, theme, bg] of [
    ["overrides a dark device to light", { light: true }, "light", LIGHT_BG],
    ["leaves a dark device dark without the setting", {}, null, DARK_BG],
  ]) {
    it(`${what}, before the application has loaded`, async () => {
      const [page, close] = await context(DESKTOP, { colorScheme: "dark" }, settings);
      try {
        await page.route("**/assets/*.js", (route) => route.abort());
        await page.goto(fixture.url("alpha.md"));
        const seen = await page.evaluate(() => ({
          theme: document.documentElement.getAttribute("data-theme"),
          bg: getComputedStyle(document.body).backgroundColor,
          mounted: document.getElementById("app").childElementCount,
        }));
        assert.equal(seen.mounted, 0, "the application bundle did not run");
        assert.equal(seen.theme, theme);
        assert.equal(seen.bg, bg);
      } finally {
        await close();
      }
    });
  }

  it("sets no-motion before the application has loaded too", async () => {
    const [page, close] = await context(DESKTOP, {}, { noMotion: true });
    try {
      await page.route("**/assets/*.js", (route) => route.abort());
      await page.goto(fixture.url("alpha.md"));
      assert.equal(await page.evaluate(() => document.documentElement.getAttribute("data-motion")), "none");
    } finally {
      await close();
    }
  });

  /**
   * A scroll-to-line, with a record of every element that was ever given the
   * flash class. The observer is installed before the page's own scripts, so
   * a flash that is added and removed within the 1.5 s timer is still seen.
   */
  const flashesAt = async (profile, extra, settings, { expectFlash }) => {
    const [page, close] = await context(profile, extra, settings);
    try {
      await page.addInitScript(() => {
        window.__flashed = [];
        new MutationObserver((records) => {
          for (const r of records) {
            if (r.target.classList?.contains("flash")) window.__flashed.push(r.target.tagName);
          }
        }).observe(document, { subtree: true, attributes: true, attributeFilter: ["class"] });
      });
      await openNote(page, fixture.url("projects/deep/nested.md", `?l=${NESTED_LINE}`));
      await page.waitForSelector(`[data-line="${NESTED_LINE}"]`);
      if (expectFlash) {
        // Wait for the thing itself rather than for a moment by which it
        // ought to have happened: a check that samples for an absence is a
        // check that fails when the machine is busy.
        await page.waitForFunction(() => window.__flashed.length > 0);
      } else {
        // An absence has nothing to wait for, so wait for the decision
        // instead. The scroll and the flash are taken in the same effect,
        // one statement apart, so a note that has been scrolled is a note
        // whose flash has been decided; two frames after it, any mutation
        // record from that task has been delivered.
        await page.waitForFunction(
          (line) => document.querySelector(`[data-line="${line}"]`).closest("main").scrollTop > 0,
          NESTED_LINE,
        );
        await page.evaluate(
          () => new Promise((resolve) => requestAnimationFrame(() => requestAnimationFrame(resolve))),
        );
      }
      return page.evaluate(() => ({
        flashed: window.__flashed,
        motion: document.documentElement.getAttribute("data-motion"),
        scrolled: document.querySelector("main.note-body")?.scrollTop ?? 0,
      }));
    } finally {
      await close();
    }
  };

  it("flashes the line a search hit points at", async () => {
    const seen = await flashesAt(DESKTOP, {}, {}, { expectFlash: true });
    assert.deepEqual(seen.flashed, ["P"], "the paragraph at the requested line flashed");
    assert.equal(seen.motion, null);
  });

  it("draws no flash with the no-animation setting on", async () => {
    const seen = await flashesAt(DESKTOP, {}, { noMotion: true }, { expectFlash: false });
    assert.equal(seen.motion, "none");
    assert.deepEqual(seen.flashed, [], "the class is never added, not merely animated for 0s");
    assert.ok(seen.scrolled > 0, "the block is still scrolled to, which is not motion");
  });

  it("draws no flash when the device asks for reduced motion", async () => {
    const seen = await flashesAt(DESKTOP, { reducedMotion: "reduce" }, {}, { expectFlash: false });
    assert.equal(seen.motion, null, "the device preference is honoured without the setting");
    assert.deepEqual(seen.flashed, []);
    assert.ok(seen.scrolled > 0, "the block is still scrolled to, which is not motion");
  });

  /**
   * Walks the page through the three states that put every tapped control on
   * screen — the tree, the search and tag list, and the settings panel — and
   * returns every height measured on the way.
   */
  const tapTargets = async (profile, { drawer }) => {
    const [page, close] = await context(profile, {}, {});
    const found = {};
    const collect = async () => {
      for (const [sel, heights] of Object.entries(await measure(page))) {
        (found[sel] ??= []).push(...heights);
      }
    };
    const openDrawer = async (tab) => {
      if (!drawer) return;
      await page.click(tab === "find" ? ".find-toggle" : ".nav-toggle");
      await page.waitForFunction(() => document.querySelector(".panes").contains(document.activeElement));
    };
    const closeDrawer = async () => {
      if (!drawer) return;
      await page.keyboard.press("Escape");
      await page.waitForFunction(() => !document.querySelector(".panes").classList.contains("open"));
    };

    try {
      // The tree, with a directory expanded so a `.dir` row and the files
      // under it are both on screen — and the note itself, which carries the
      // frontmatter disclosure and the note bar.
      await openNote(page, fixture.url("alpha.md"));
      await page.waitForSelector(".metadata summary");
      await openDrawer("notes");
      const dirs = await page.locator(".tree button.dir").count();
      assert.ok(dirs > 0, "the fixture tree has directories to expand");
      await page.click(".tree button.dir >> nth=0");
      await page.waitForFunction(() => document.querySelectorAll(".tree .file").length > 3);
      await collect();
      await closeDrawer();

      // The search results and the tag list, with a filter on so the panel's
      // clear link and the top bar's chip are both there to be measured.
      await openNote(page, fixture.url("alpha.md", "?tag=alpha"));
      await openDrawer("find");
      await page.fill('input[type="search"]', "note");
      await page.waitForSelector(".hit", { state: "attached" });
      await page.waitForSelector(".tag-clear", { state: "attached" });
      await collect();
      await closeDrawer();

      // The settings panel.
      await page.click(".settings-toggle");
      await page.waitForSelector(".settings-panel");
      await collect();
      return found;
    } finally {
      await close();
    }
  };

  for (const profile of [...PHONES, COARSE_DESKTOP]) {
    const drawer = profile !== COARSE_DESKTOP;
    it(`keeps every tapped control at ${TAP_TARGET} px or more at ${profile.name}`, async () => {
      const found = await tapTargets(profile, { drawer });
      const required = TAP_GROUPS.filter((g) => drawer || (g !== ".drawer-tab" && g !== ".tag-chip"));
      for (const group of required) {
        assert.ok(found[group]?.length > 0, `nothing measured for ${group}`);
      }
      for (const [group, heights] of Object.entries(found)) {
        for (const height of heights) {
          assert.ok(height >= TAP_TARGET, `${group} is ${height} px, under the ${TAP_TARGET} px floor`);
        }
      }
    });
  }

  it("leaves a mouse-driven window its density", async () => {
    const found = await tapTargets(DESKTOP, { drawer: false });
    assert.ok(found[".tree .file"].length > 0, "the tree was measured");
    // The trade-off #62 recorded: 27 px rows under a mouse, 41 under a
    // finger. A rule that reached every width would have cost the navigator
    // a third of its notes.
    for (const height of found[".tree .file"]) {
      assert.equal(height, 27, "a tree row under a fine pointer is 27 px");
    }
    assert.ok(found[".tree .dir"].every((h) => h < TAP_TARGET));
  });

  it("measures the same 41.25 px the stylesheet's 2.75rem asks for", async () => {
    // 2.75rem against the application's 15 px root font. Stated once, here,
    // so the floor above is a floor and this is the number behind it.
    const found = await tapTargets(PIXEL_7, { drawer: true });
    for (const group of [".tree .file", ".tag", ".drawer-tab"]) {
      assert.ok(
        found[group].every((h) => h === 41.25),
        `${group} measured ${JSON.stringify(found[group])}`,
      );
    }
  });
});
