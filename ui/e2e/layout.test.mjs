/**
 * The layout checks the phone-layout task (#61) measured in a scratchpad
 * harness and #72 asked to be brought into CI: the pane rectangles at the
 * four phone profiles and at a desktop control, the breakpoint itself, the
 * drawer's geometry and its four close paths, and the tag chip.
 *
 * The figures are the ones the M4 record states for the Pixel 7 profile —
 * a 47-pixel bar over a 412x745 note, a 300x792 drawer — and each of them
 * is asserted beside the rule it comes from, so a viewport that moves fails
 * on the rule rather than passing on a coincidence.
 */

import { after, before, describe, it } from "node:test";
import assert from "node:assert/strict";
import {
  BREAKPOINT,
  COARSE_DESKTOP,
  DESKTOP,
  MIDDLE,
  NOTE_COLUMN,
  PHONES,
  PIXEL_7,
  READING_WIDTH,
  TAP_TARGET,
  THREE_COLUMN,
  drawerReady,
  loadPlaywright,
  missingPrerequisite,
  openNote,
  rect,
  startFixture,
} from "./harness.mjs";

const playwright = loadPlaywright();
const blocker = missingPrerequisite(playwright);
if (blocker) console.log(`# skipped: ${blocker}`);

describe("the layout at phone widths and above", { skip: blocker ?? false }, () => {
  let fixture, browser;

  before(async () => {
    fixture = await startFixture("layout");
    browser = await playwright.chromium.launch({ headless: true });
  });

  after(async () => {
    await browser?.close();
    await fixture?.stop();
  });

  /**
   * A page in a fresh context, closed by the caller through the returned
   * handle. Anything the page throws is collected rather than raised from
   * the listener, and reported when the context closes.
   */
  const page = async (profile, extra = {}) => {
    const { name, ...options } = profile;
    const context = await browser.newContext({ ...options, ...extra });
    const p = await context.newPage();
    const thrown = [];
    p.on("pageerror", (e) => thrown.push(e.message));
    return [
      p,
      async () => {
        await context.close();
        if (thrown.length > 0) assert.fail(`${name}: the page threw ${thrown.join("; ")}`);
      },
    ];
  };

  for (const profile of PHONES) {
    it(`gives the note the viewport under a compact bar at ${profile.name}`, async () => {
      const [p, close] = await page(profile);
      try {
        const { width, height } = profile.viewport;
        await openNote(p, fixture.url("alpha.md"));

        const narrow = await p.evaluate(() => matchMedia("(max-width: 60rem)").matches);
        assert.equal(narrow, true, "the profile is below the breakpoint");

        const bar = await rect(p, ".topbar");
        const note = await rect(p, ".note");
        assert.equal(bar.width, width, "the bar spans the viewport");
        assert.equal(note.width, width, "the note spans the viewport");
        assert.equal(note.y, bar.height, "the note starts where the bar ends");
        // The rule: the bar and the note are the whole viewport, nothing else
        // in the column. #34's failure was a note squeezed to a content-sized
        // row's leftovers, which is what this number catches.
        assert.equal(bar.height + note.height, height, "the bar and the note fill the viewport");
        assert.ok(note.height > height * 0.85, `the note has ${note.height} of ${height}`);

        // The root path is the first label to go at this width.
        assert.equal(await p.evaluate(() => getComputedStyle(document.querySelector(".path")).display), "none");
        // Nothing overflows sideways: a top bar that pushes the page wider is
        // what the unsaved-draft clamp and the ellipsis rules are there for.
        assert.equal(await p.evaluate(() => document.documentElement.scrollWidth), width);

        // Edit mode is the other half of the requirement: the same rectangle.
        await p.click(".mode-toggle");
        await p.waitForSelector(".editor-body");
        const editing = await rect(p, ".note");
        assert.deepEqual({ w: editing.width, h: editing.height }, { w: note.width, h: note.height });
      } finally {
        await close();
      }
    });
  }

  it("matches the figures the M4 record states for the Pixel 7 profile", async () => {
    const [p, close] = await page(PIXEL_7);
    try {
      await openNote(p, fixture.url("alpha.md"));
      const bar = await rect(p, ".topbar");
      const note = await rect(p, ".note");
      const drawer = await rect(p, ".panes");
      assert.equal(Math.round(bar.height), 47, "the compact bar is 47 px");
      assert.deepEqual(
        { w: Math.round(note.width), h: Math.round(note.height) },
        { w: 412, h: 745 },
        "the note is 412x745",
      );
      // min(20rem, 85vw) against the app's 15 px root font: 300, not 350.
      assert.deepEqual(
        { w: drawer.width, h: drawer.height },
        { w: 300, h: 792 },
        "the drawer is 300x792",
      );
    } finally {
      await close();
    }
  });

  it("keeps the three-pane grid on a wide window", async () => {
    const [p, close] = await page(DESKTOP);
    try {
      await openNote(p, fixture.url("alpha.md"));
      const nav = await rect(p, ".nav");
      const note = await rect(p, ".note");
      const side = await rect(p, ".side");
      assert.equal(nav.x, 0);
      assert.equal(note.x, nav.width, "the note begins where the navigator ends");
      assert.equal(side.x, nav.width + note.width, "the side pane begins where the note ends");
      assert.equal(nav.width + note.width + side.width, DESKTOP.viewport.width, "the three panes are the window");
      // The wrapper is inert here: the panes are grid items of the shell.
      assert.equal(await p.evaluate(() => getComputedStyle(document.querySelector(".panes")).display), "contents");
      assert.notEqual(await p.evaluate(() => getComputedStyle(document.querySelector(".path")).display), "none");
      // None of the drawer's chrome is laid out, so none of it is focusable
      // or in the accessibility tree. Asked of the box rather than of the
      // `display` property: the tabs are `inline-block` inside a head the
      // media query has taken out, and it is the box that decides.
      for (const sel of [".drawer-toggle", ".drawer-head", ".drawer-tab", ".drawer-close", ".tag-chip"]) {
        assert.equal(
          await p.evaluate((s) => [...document.querySelectorAll(s)].every((e) => e.getClientRects().length === 0), sel),
          true,
          `${sel} is not laid out at a wide width`,
        );
      }
    } finally {
      await close();
    }
  });

  /**
   * The middle layout (M8-R10, davison/md-notes#156): above the drawer
   * breakpoint but below the width at which all three panes fit, the
   * search-and-tags pane is a second row under the navigator rather than a
   * column taken out of the note.
   *
   * The note is measured two ways here, because the requirement is about
   * both: the column, which is the whole window less the navigator — the
   * side pane costs it nothing — and the rendered article inside it, which
   * is at its 48rem reading width at this width and was 530 px before this
   * layout existed.
   */
  it("stacks the side pane under the navigator between the two breakpoints", async () => {
    const [p, close] = await page(MIDDLE);
    try {
      const { width } = MIDDLE.viewport;
      await openNote(p, fixture.url("projects/deep/nested.md"));
      assert.ok(width > BREAKPOINT && width < THREE_COLUMN, "the profile is between the two");

      const nav = await rect(p, ".nav");
      const side = await rect(p, ".side");
      const note = await rect(p, ".note");
      const article = await rect(p, ".note-article");

      assert.equal(nav.x, 0);
      assert.equal(side.x, 0, "the side pane is in the left column, not a column of its own");
      assert.equal(side.width, nav.width, "the two share the column's width");
      // Within a hundredth of a pixel: the two rows are fractions of the
      // space under the bar, and `rect` rounds each of them on its own.
      const meets = (a, b, what) => assert.ok(Math.abs(a - b) < 0.02, `${what}: ${a} against ${b}`);
      meets(side.y, nav.y + nav.height, "the side pane begins where the navigator ends");
      meets(nav.height + side.height, note.height, "the two fill the column beside the note");

      assert.equal(note.x, nav.width, "the note begins where the navigator ends");
      assert.equal(note.width, width - nav.width, "the side pane takes no width from the note");
      assert.ok(note.width >= NOTE_COLUMN, `the note column is ${note.width}, its reading width plus gutters is ${NOTE_COLUMN}`);
      assert.equal(article.width, READING_WIDTH, "the rendered note is at its reading width");

      // Each pane scrolls on its own, which is the point of two rows rather
      // than one scrolling box holding both.
      assert.deepEqual(
        await p.evaluate(() => [".nav", ".side"].map((s) => getComputedStyle(document.querySelector(s)).overflowY)),
        ["auto", "auto"],
      );

      // The drawer wrapper is inert above the breakpoint at this width too,
      // which is what `drawer.tsx` asks the element when a window crosses it.
      assert.equal(await p.evaluate(() => getComputedStyle(document.querySelector(".panes")).display), "contents");
    } finally {
      await close();
    }
  });

  it(`moves to three columns at ${THREE_COLUMN} CSS pixels and not at ${THREE_COLUMN - 1}`, async () => {
    const [p, close] = await page({ name: "resizable", viewport: { width: THREE_COLUMN, height: 900 } });
    try {
      await openNote(p, fixture.url("projects/deep/nested.md"));
      const at = async (width) => {
        await p.setViewportSize({ width, height: 900 });
        await p.waitForFunction((w) => document.documentElement.clientWidth === w, width);
        const [nav, note, side, article] = await Promise.all(
          [".nav", ".note", ".side", ".note-article"].map((s) => rect(p, s)),
        );
        return { nav, note, side, article };
      };

      // One pixel below it the three columns would have had to come out of
      // the note: 1289 - 16rem - 18rem is 779, a pixel under the 780 the
      // reading width and its gutters need. So the side pane is still a row.
      const under = await at(THREE_COLUMN - 1);
      assert.equal(under.side.x, 0, "the side pane is still under the navigator");
      assert.equal(under.note.width, THREE_COLUMN - 1 - under.nav.width);
      assert.equal(under.article.width, READING_WIDTH);

      // At it, all three fit and the note column is exactly its reading
      // width plus the 2rem `.note-body` pads it with either side.
      const over = await at(THREE_COLUMN);
      assert.equal(over.side.x, over.nav.width + over.note.width, "the side pane is a column again");
      assert.equal(over.nav.width + over.note.width + over.side.width, THREE_COLUMN, "the three panes are the window");
      assert.equal(over.note.width, NOTE_COLUMN, "the note column is exactly its reading width plus gutters");
      assert.equal(over.article.width, READING_WIDTH, "and the note itself is exactly at its reading width");

      // Wider still, the note column grows and the note stays at its
      // reading width: the max-width has been doing that all along.
      const wide = await at(DESKTOP.viewport.width);
      assert.equal(wide.article.width, READING_WIDTH);
      assert.ok(wide.note.width > NOTE_COLUMN);
    } finally {
      await close();
    }
  });

  it("switches layout at 960 CSS pixels and not at 961", async () => {
    const [p, close] = await page({ name: "resizable", viewport: { width: BREAKPOINT, height: 800 } });
    try {
      await openNote(p, fixture.url("alpha.md"));
      const layoutAt = async (width) => {
        await p.setViewportSize({ width, height: 800 });
        await p.waitForFunction(
          (w) => document.documentElement.clientWidth === w,
          width,
        );
        return p.evaluate(() => ({
          narrow: matchMedia("(max-width: 60rem)").matches,
          panes: getComputedStyle(document.querySelector(".panes")).display,
        }));
      };
      assert.deepEqual(await layoutAt(BREAKPOINT - 1), { narrow: true, panes: "flex" });
      assert.deepEqual(await layoutAt(BREAKPOINT), { narrow: true, panes: "flex" }, "60rem is inclusive");
      assert.deepEqual(await layoutAt(BREAKPOINT + 1), { narrow: false, panes: "contents" });
    } finally {
      await close();
    }
  });

  it("opens the drawer from both buttons and closes it four ways", async () => {
    const [p, close] = await page(PIXEL_7);
    try {
      await openNote(p, fixture.url("alpha.md"));
      const state = () =>
        p.evaluate(() => {
          const panes = document.querySelector(".panes");
          return {
            open: panes.classList.contains("open"),
            visibility: getComputedStyle(panes).visibility,
            role: panes.getAttribute("role"),
            modal: panes.getAttribute("aria-modal"),
            label: panes.getAttribute("aria-label"),
            tab: panes.getAttribute("data-tab"),
            backdrop: document.querySelectorAll(".drawer-backdrop").length,
            inside: panes.contains(document.activeElement),
            active: document.activeElement?.className ?? "",
            expanded: document.querySelector(".nav-toggle").getAttribute("aria-expanded"),
          };
        });

      // Closed, the drawer is out of the tab order and out of the
      // accessibility tree — not merely off screen.
      assert.deepEqual(await state(), {
        open: false, visibility: "hidden", role: null, modal: null, label: null,
        tab: "notes", backdrop: 0, inside: false, active: "", expanded: "false",
      });

      // Opening is complete when focus has moved in, which is also the
      // requirement; waiting on that rather than on the transition is what
      // keeps the close paths below off a race with the handlers' effects.
      // `drawerReady` is that wait, in the harness so the next drawer case
      // written does not have to know about the race (davison/md-notes#89).
      const open = async (selector, tab) => {
        await p.click(selector);
        await drawerReady(p);
        const s = await state();
        assert.equal(s.open, true);
        assert.equal(s.visibility, "visible");
        assert.equal(s.role, "dialog");
        assert.equal(s.modal, "true");
        assert.equal(s.label, "Navigator, search and tags");
        assert.equal(s.tab, tab);
        assert.equal(s.backdrop, 1);
      };
      const closed = async (opener) => {
        await p.waitForFunction(() => !document.querySelector(".panes").classList.contains("open"));
        // Focus goes back to the button that opened it.
        await p.waitForFunction((o) => document.activeElement?.classList.contains(o), opener);
        const s = await state();
        assert.equal(s.role, null, "the closed drawer is no longer a dialog");
        assert.equal(s.backdrop, 0);
        assert.equal(s.expanded, "false");
      };

      await open(".nav-toggle", "notes");
      await p.keyboard.press("Escape");
      await closed("nav-toggle");

      await open(".nav-toggle", "notes");
      await p.click(".drawer-backdrop", { position: { x: 380, y: 400 } });
      await closed("nav-toggle");

      await open(".nav-toggle", "notes");
      await p.click(".drawer-close");
      await closed("nav-toggle");

      // The magnifier opens the same drawer on the other tab, with the
      // search box focused — the reason for having opened it.
      await open(".find-toggle", "find");
      assert.equal(await p.evaluate(() => document.activeElement.getAttribute("type")), "search");
      await p.keyboard.press("Escape");
      await closed("find-toggle");

      // Selecting a note closes the drawer, which is covering the note the
      // tap just opened.
      await open(".nav-toggle", "notes");
      await p.click('.tree a.file[href*="beta.md"]');
      await closed("nav-toggle");
      assert.equal(new URL(p.url()).pathname, "/r/notes/beta.md");
      await p.waitForFunction(() => document.querySelector(".note-title")?.textContent === "Beta");
    } finally {
      await close();
    }
  });

  it("shows the active tag filter in the top bar at narrow widths only", async () => {
    const [p, close] = await page(PIXEL_7);
    try {
      await openNote(p, fixture.url("alpha.md", "?tag=alpha"));
      await p.waitForSelector(".tag-chip");
      const chip = await p.evaluate(() => {
        const c = document.querySelector(".tag-chip");
        return {
          label: c.getAttribute("aria-label"),
          text: c.textContent,
          href: c.getAttribute("href"),
          height: c.getBoundingClientRect().height,
        };
      });
      assert.equal(chip.label, "Clear the tag filter alpha");
      assert.equal(chip.text, "#alpha×");
      assert.equal(chip.href, "/r/notes/alpha.md", "the chip clears the filter, keeping the note");
      assert.ok(chip.height >= TAP_TARGET, `the chip is ${chip.height} px tall`);

      // The filter is doing something: the tree is the two tagged notes.
      const files = await p.evaluate(() => [...document.querySelectorAll(".tree a.file")].map((a) => a.textContent));
      assert.deepEqual(files, ["alpha.md", "beta.md"]);

      await p.click(".tag-chip");
      await p.waitForFunction(() => !document.querySelector(".tag-chip"));
      assert.equal(new URL(p.url()).search, "", "the filter is gone from the URL");
    } finally {
      await close();
    }
  });

  it("uses the tag panel's own clear link on a wide window, not the chip", async () => {
    const [p, close] = await page(DESKTOP);
    try {
      await openNote(p, fixture.url("alpha.md", "?tag=alpha"));
      await p.waitForSelector(".tag-clear");
      assert.equal(
        await p.evaluate(() => getComputedStyle(document.querySelector(".tag-chip")).display),
        "none",
        "the chip is not laid out beside a visible tag panel",
      );
      assert.equal(
        await p.evaluate(() => document.querySelector(".tag-clear").textContent),
        "Clear filter: alpha",
      );
    } finally {
      await close();
    }
  });

  it("gives the drawer the same geometry under a coarse pointer on a wide window", async () => {
    // The wide layout is the wide layout whatever is pointing at it: the
    // drawer's chrome stays unlaid-out, which is what makes the tap-target
    // rule in display.test.mjs a question about density and not about the
    // breakpoint.
    const [p, close] = await page(COARSE_DESKTOP);
    try {
      await openNote(p, fixture.url("alpha.md"));
      assert.equal(await p.evaluate(() => matchMedia("(pointer: coarse)").matches), true);
      assert.equal(await p.evaluate(() => getComputedStyle(document.querySelector(".panes")).display), "contents");
      const nav = await rect(p, ".nav");
      assert.ok(nav.width > 0 && nav.x === 0, "the navigator is still a pane, not a drawer");
    } finally {
      await close();
    }
  });
});
