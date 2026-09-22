/**
 * Flowcharts in the reading view, in headless Chromium against the built
 * daemon (davison/md-notes#171): a fenced mermaid flowchart shows as the
 * daemon's drawing in each of the three palettes, anything the daemon will
 * not draw shows as the code block it always did, a live edit redraws only
 * what changed, and the drawing opened on its own as a document can run
 * nothing.
 *
 *     make e2e
 *     pnpm --dir ui e2e
 *
 * The rig is ./harness.mjs: a temporary root, its own daemon on a port the
 * kernel hands out, nothing of the operator's touched.
 */

import { after, before, describe, it } from "node:test";
import assert from "node:assert/strict";
import fs from "node:fs";
import { DESKTOP, MIDDLE, PHONES, PIXEL_7, loadPlaywright, missingPrerequisite, openNote, startFixture, waitFor } from "./harness.mjs";

const playwright = loadPlaywright();
const blocker = missingPrerequisite(playwright);
if (blocker) console.log(`# skipped: ${blocker}`);

const FLOW = "flowchart LR\n  A[Start] --> B{Ready?}\n  B -->|yes| C[Ship]\n  B -->|no| A\n";
const SEQUENCE = "sequenceDiagram\n  Alice->>Bob: Hello\n";
/** A long left-to-right chain: wider than the reading column. */
const WIDE = "flowchart LR\n  " + Array.from({ length: 14 }, (_, i) => `N${i}[Step number ${i}]`).join(" --> ") + "\n";

/**
 * The shape of the operator's note that #183 was found on: an eight-node
 * left-to-right chain with one side input and two links back. About 1900 px
 * wide at natural size, with 14 px labels.
 */
const LONG_LR = `flowchart LR
  A[Team notes and correspondence] --> B[Evidence store with provenance]
  R[Versioned policy rules] --> C[Eligibility and budget checks]
  B --> C
  C --> D[Case workflow and agent tasks]
  D --> E[Grounded application packet]
  E --> F[Team approval and authorised filing]
  F --> G[Receipt and reviewer response]
  G --> D
  G --> H[Award obligations and reporting]
  H --> B
`;
/** A chain a little wider than the 720 px column: it shrinks to fit without reaching the floor. */
const JUST_WIDE = "flowchart LR\n  " + Array.from({ length: 6 }, (_, i) => `M${i}[Middle ${i}]`).join(" --> ") + "\n";

/**
 * The smallest scale a diagram is shown at (the decision on
 * davison/md-notes#187): its 14 px labels never drawn below 12 px.
 */
const MIN_SCALE = 12 / 14;
const LABEL_PX = 14;

/** A note with the long chain near the top and a paragraph to land on well below it. */
const WIDE_NOTE = (() => {
  const lines = ["# Wide", "", "A paragraph above.", "", "```mermaid", ...LONG_LR.trimEnd().split("\n"), "```", ""];
  for (let i = 0; i < 30; i++) lines.push(`Filler paragraph ${i}.`, "");
  lines.push("The needle-word paragraph.", "");
  const paragraph = lines.length - 1;
  for (let i = 0; i < 40; i++) lines.push(`Trailing paragraph ${i}.`, "");
  return { text: lines.join("\n"), paragraph };
})();

/** The widths M10-R1 names: wide, middle and three phones. */
const WIDTHS = [
  DESKTOP,
  MIDDLE,
  PIXEL_7,
  PHONES[2],
  { name: "320 px phone", viewport: { width: 320, height: 640 }, deviceScaleFactor: 2, isMobile: true, hasTouch: true },
];

/** Parses, so the note lists it, and is refused by the layout's node bound. */
function layoutRefused() {
  const lines = ["flowchart TD"];
  for (let i = 0; i < 149; i++) lines.push(`A${i} --------> A${i + 1}`);
  for (let i = 0; i < 10; i++) lines.push(`A${149 - i} --------> A${i}`);
  return lines.join("\n") + "\n";
}

const fence = (src) => "```mermaid\n" + src + "```\n";

/**
 * A note of twelve tall flowcharts, then a block drawn as code, then prose:
 * the shape QA's regression was found on (davison/md-notes#177). The source
 * lines of three scroll-to-line targets are recorded as it is built.
 */
/**
 * More diagrams than the daemon's measuring budget covers (review of PR
 * #181): sixty dense graphs, each over ten milliseconds to lay out, then a
 * block the layout refuses, then the target paragraph. Most go out
 * unmeasured, the refused block among them, so the page meets both the
 * boxes it has to keep its place for and the route's 422.
 */
function manyNote(start) {
  let seed = start;
  const rand = (n) => {
    seed = (seed * 1103515245 + 12345) % 2147483648;
    return seed % n;
  };
  const lines = ["# Many", ""];
  const at = {};
  for (let d = 1; d <= 60; d++) {
    lines.push("```mermaid", "flowchart TD");
    const fenceLine = lines.length - 1;
    // Ten layers of twenty nodes, each linked to two in the next layer,
    // and forty links that skip a layer: 200 nodes and 400 links, the
    // node and link bounds, and wide ranks that are slow to order and
    // place. Random links between 80 nodes stopped being slow enough when
    // the layout got faster (review of PR #200, finding 1); these take
    // about as long now as those did then, and every one is drawn.
    for (let l = 0; l < 9; l++) {
      for (let w = 0; w < 20; w++) {
        for (let k = 0; k < 2; k++) lines.push(`  G${d}L${l}N${w} --> G${d}L${l + 1}N${rand(20)}`);
      }
    }
    for (let x = 0; x < 40; x++) {
      const l = rand(8);
      lines.push(`  G${d}L${l}N${rand(20)} --> G${d}L${l + 2}N${rand(20)}`);
    }
    if (d === 40) at.inDiagram = fenceLine + 3;
    lines.push("```", "");
  }
  // Its own names, so the daemon has not already refused it for another note.
  lines.push("```mermaid", ...layoutRefused().replaceAll("A", `R${start}x`).trimEnd().split("\n"), "```", "");
  lines.push("The needle-word paragraph.", "");
  at.paragraph = lines.length - 1;
  for (let i = 0; i < 40; i++) lines.push(`Trailing paragraph ${i}.`, "");
  return { text: lines.join("\n"), at };
}

/**
 * One such note per case, each with graphs of its own: the daemon keeps
 * every size it has measured, so a note another case has opened would no
 * longer be one it has to measure.
 */
const MANY = Array.from({ length: 4 }, (_, i) => manyNote(7 + i));

const FLOWS = (() => {
  const lines = ["# Flows", ""];
  const at = {};
  for (let d = 1; d <= 12; d++) {
    lines.push("```mermaid", "flowchart TD");
    const fenceLine = lines.length - 1;
    for (let n = 0; n < 5; n++) lines.push(`  D${d}N${n}[Diagram ${d} step ${n}] --> D${d}N${n + 1}[Diagram ${d} step ${n + 1}]`);
    if (d === 9) at.inDiagram = fenceLine + 3;
    lines.push("```", "");
  }
  lines.push("```mermaid", "sequenceDiagram", "  Alice->>Bob: Hello", "```", "");
  at.codeBlock = lines.length - 3;
  lines.push("Some prose between.", "");
  lines.push("The needle-word paragraph.", "");
  at.paragraph = lines.length - 1;
  for (let i = 0; i < 40; i++) lines.push(`Trailing paragraph ${i}.`, "");
  return { text: lines.join("\n"), at };
})();

/** The page backgrounds each palette draws, as the canvas reads them back. */
const BACKGROUND = { light: [251, 251, 250], dark: [27, 27, 27], eink: [255, 255, 255] };

describe("flowcharts in the reading view", { skip: blocker ?? false }, () => {
  let fixture, browser;

  before(async () => {
    fixture = await startFixture("diagram");
    const write = (rel, text) => fs.writeFileSync(fixture.file(rel), text);
    write("flow.md", "# Flow\n\nA diagram and one the daemon does not draw.\n\n" + fence(FLOW) + "\n" + fence(SEQUENCE));
    write("wide.md", "# Wide\n\n" + fence(WIDE));
    write("wide-lr.md", WIDE_NOTE.text);
    write("just-wide.md", "# Just wide\n\n" + fence(JUST_WIDE));
    write("refused.md", "# Refused\n\n" + fence(layoutRefused()));
    write("live.md", "# Live\n\n" + fence("graph TD; P-->Q\n") + "\n" + fence("graph TD; X-->Y\n"));
    write("flows.md", FLOWS.text);
    MANY.forEach((m, i) => write(`many-${i}.md`, m.text));
    write("unreachable.md", "# Unreachable\n\n" + fence(FLOW));
    browser = await playwright.chromium.launch({ headless: true });
  });

  after(async () => {
    await browser?.close();
    await fixture?.stop();
  });

  /**
   * A page with every diagram request recorded. Service workers are blocked
   * so every request is the page's own, where a route can see it.
   */
  const open = async ({ colorScheme = "light", settings = null } = {}) => {
    const ctx = await browser.newContext({ viewport: DESKTOP.viewport, colorScheme, serviceWorkers: "block" });
    if (settings !== null) {
      await ctx.addInitScript((s) => localStorage.setItem("mdn:settings", JSON.stringify(s)), settings);
    }
    const page = await ctx.newPage();
    const requests = [];
    const responses = [];
    page.on("request", (r) => {
      if (r.url().includes("/diagram/")) requests.push(r.url());
    });
    page.on("response", (r) => {
      if (r.url().includes("/diagram/")) responses.push({ url: r.url(), status: r.status() });
    });
    return { page, requests, responses, close: () => ctx.close() };
  };

  /** Waits for every diagram image in the note to have loaded. */
  const loaded = (page, count) =>
    page.waitForFunction(
      (n) => {
        const imgs = [...document.querySelectorAll(".markdown img.diagram")];
        return imgs.length === n && imgs.every((i) => i.complete && i.naturalWidth > 0);
      },
      count,
    );

  /** The colour of the drawing's top-left pixel: its palette's background. */
  const cornerPixel = (page) =>
    page.evaluate(() => {
      const img = document.querySelector(".markdown img.diagram");
      const c = document.createElement("canvas");
      c.width = img.naturalWidth;
      c.height = img.naturalHeight;
      const g = c.getContext("2d");
      g.drawImage(img, 0, 0);
      return [...g.getImageData(1, 1, 1, 1).data.slice(0, 3)];
    });

  for (const [palette, colorScheme, settings] of [
    ["light", "light", null],
    ["dark", "dark", null],
    // The light override is the e-ink setting, and wins over a dark device.
    ["eink", "dark", { light: true }],
  ]) {
    it(`draws a flowchart as an image in the ${palette} palette`, async () => {
      const { page, requests, close } = await open({ colorScheme, settings });
      try {
        await openNote(page, fixture.url("flow.md"));
        await loaded(page, 1);
        const seen = await page.evaluate(() => {
          const img = document.querySelector(".markdown img.diagram");
          const pres = [...document.querySelectorAll(".markdown pre")];
          return {
            src: img.getAttribute("src"),
            alt: img.alt,
            // The image stands in its box, the box right after the block's
            // line anchor and right before the code block.
            anchor: img.closest(".diagram-box")?.previousElementSibling?.className,
            codeHidden: getComputedStyle(img.closest(".diagram-box").nextElementSibling).display === "none",
            shown: pres.filter((p) => getComputedStyle(p).display !== "none").map((p) => p.textContent),
            width: img.getBoundingClientRect().width,
            natural: img.naturalWidth,
          };
        });
        assert.match(seen.src, new RegExp(`theme=${palette}$`));
        assert.equal(seen.alt, FLOW, "the source is the image's text alternative");
        assert.equal(seen.anchor, "line-anchor", "the image stands right after the block's line anchor");
        assert.equal(seen.codeHidden, true, "the flowchart's code block is hidden behind it");
        // The sequence diagram is outside the subset: its code block stays,
        // and nothing was ever asked of the daemon for it.
        assert.deepEqual(seen.shown, [SEQUENCE]);
        assert.equal(requests.length, 1, `one drawing requested: ${requests.join(", ")}`);
        // Drawn at its own size, not scaled up. The SVG's size is fractional
        // and naturalWidth is a whole number, so within a pixel.
        assert.ok(Math.abs(seen.width - seen.natural) < 1, `drawn at ${seen.width}, natural ${seen.natural}`);
        assert.deepEqual(await cornerPixel(page), BACKGROUND[palette]);
      } finally {
        await close();
      }
    });
  }

  it("follows a change of the setting without reloading the page", async () => {
    const { page, close } = await open({ colorScheme: "dark" });
    try {
      await openNote(page, fixture.url("flow.md"));
      await loaded(page, 1);
      assert.deepEqual(await cornerPixel(page), BACKGROUND.dark);
      await page.locator(".settings-toggle").click();
      await page.getByLabel("Always use the light theme").check();
      await page.waitForFunction(() => document.querySelector(".markdown img.diagram")?.getAttribute("src").endsWith("theme=eink"));
      await loaded(page, 1);
      assert.deepEqual(await cornerPixel(page), BACKGROUND.eink);
    } finally {
      await close();
    }
  });

  it("shrinks a diagram a little wider than the column to fit it", async () => {
    const { page, close } = await open();
    try {
      await openNote(page, fixture.url("just-wide.md"));
      await loaded(page, 1);
      const seen = await page.evaluate(() => {
        const img = document.querySelector(".markdown img.diagram");
        return {
          width: img.getBoundingClientRect().width,
          column: document.querySelector(".markdown").getBoundingClientRect().width,
          natural: img.naturalWidth,
        };
      });
      // The case is what it says: wider than the column, and not so wide
      // that fitting it would take it below the floor.
      assert.ok(seen.natural > seen.column, `the fixture is wider than the column (${seen.natural} > ${seen.column})`);
      assert.ok(seen.natural * MIN_SCALE <= seen.column, `the fixture fits above the floor (${seen.natural} x ${MIN_SCALE})`);
      assert.ok(Math.abs(seen.width - seen.column) < 1, `the image fits the column: ${seen.width} vs ${seen.column}`);
    } finally {
      await close();
    }
  });

  /**
   * Wide diagrams (davison/md-notes#183, #187). A diagram that would have to
   * shrink below the floor to fit is held at the floor, in a box of its own
   * that scrolls sideways, and the page does not.
   */
  const at = async (profile, url, { settings = null } = {}) => {
    const { name, ...options } = profile;
    const ctx = await browser.newContext({ ...options, colorScheme: "light", serviceWorkers: "block" });
    if (settings !== null) await ctx.addInitScript((s) => localStorage.setItem("mdn:settings", JSON.stringify(s)), settings);
    const page = await ctx.newPage();
    await openNote(page, url);
    return { page, close: () => ctx.close() };
  };

  for (const profile of WIDTHS) {
    it(`holds a wide diagram at the floor, scrolling in its own box, and never the page, at ${profile.name}`, async () => {
      const { page, close } = await at(profile, fixture.url("wide-lr.md"));
      try {
        await loaded(page, 1);
        const seen = await page.evaluate(() => {
          const img = document.querySelector(".markdown img.diagram");
          const md = document.querySelector(".markdown").getBoundingClientRect();
          const box = img.closest(".diagram-box");
          const r = (box ?? img).getBoundingClientRect();
          // Scrolled to its far end, the box moves nothing else.
          if (box) box.scrollLeft = box.scrollWidth;
          const html = document.documentElement;
          const main = document.querySelector("main");
          return {
            shown: img.getBoundingClientRect().width,
            natural: img.naturalWidth,
            box: box ? { left: r.left, right: r.right, scrolls: box.scrollWidth > box.clientWidth } : null,
            column: { left: md.left, right: md.right },
            viewport: html.clientWidth,
            rem: parseFloat(getComputedStyle(html).fontSize),
            page: { scrollWidth: html.scrollWidth, clientWidth: html.clientWidth, scrollX },
            pane: main ? { scrollWidth: main.scrollWidth, clientWidth: main.clientWidth } : null,
          };
        });
        const scale = seen.shown / seen.natural;
        assert.ok(seen.natural * MIN_SCALE > seen.column.right - seen.column.left, "the fixture is too wide to fit above the floor");
        assert.ok(
          LABEL_PX * scale >= LABEL_PX * MIN_SCALE - 0.01,
          `labels drawn at ${(LABEL_PX * scale).toFixed(1)} px (scale ${scale.toFixed(3)}), below the ${(LABEL_PX * MIN_SCALE).toFixed(1)} px floor`,
        );
        assert.ok(seen.box, "the diagram stands in a box of its own");
        assert.equal(seen.box.scrolls, true, "the box scrolls sideways");
        assert.ok(seen.box.left >= seen.column.left - 0.5 && seen.box.right <= seen.column.right + 0.5, `the box keeps to the column: ${JSON.stringify(seen)}`);
        assert.ok(seen.page.scrollWidth <= seen.page.clientWidth && seen.page.scrollX === 0, `the page scrolls sideways: ${JSON.stringify(seen.page)}`);
        if (seen.pane) assert.ok(seen.pane.scrollWidth <= seen.pane.clientWidth, `the note pane scrolls sideways: ${JSON.stringify(seen.pane)}`);
        // The phone layout keeps its gutter, 1rem either side.
        assert.ok(seen.column.left >= seen.rem - 0.5 && seen.viewport - seen.column.right >= seen.rem - 0.5, `the gutter: ${JSON.stringify(seen)}`);
      } finally {
        await close();
      }
    });
  }

  describe("the natural-size view", () => {
    /** The view's state: open or not, where focus is, the image's size, its palette. */
    const viewer = (page) =>
      page.evaluate(() => {
        const v = document.querySelector(".diagram-viewer");
        if (!v) return null;
        const img = v.querySelector("img");
        const close = v.querySelector(".diagram-viewer-close")?.getBoundingClientRect();
        return {
          role: v.getAttribute("role"),
          modal: v.getAttribute("aria-modal"),
          focusInside: v.contains(document.activeElement),
          width: img?.getBoundingClientRect().width,
          natural: img?.naturalWidth,
          loaded: !!img && img.complete && img.naturalWidth > 0,
          src: img?.getAttribute("src"),
          background: getComputedStyle(v).backgroundColor,
          close: close ? { width: close.width, height: close.height } : null,
          animations: document.getAnimations().length,
          pageScroll: document.documentElement.scrollWidth - document.documentElement.clientWidth,
        };
      });
    const opened = (page) => page.waitForFunction(() => {
      const img = document.querySelector(".diagram-viewer img");
      return !!img && img.complete && img.naturalWidth > 0 && document.querySelector(".diagram-viewer").contains(document.activeElement);
    });
    const focusedOpener = (page) => page.evaluate(() => document.activeElement?.matches(".diagram-open") ?? false);

    it("opens on a click at natural size, and Escape closes it and hands focus back", async () => {
      const { page, close } = await at(DESKTOP, fixture.url("wide-lr.md"));
      try {
        await loaded(page, 1);
        await page.locator(".markdown img.diagram").click();
        await opened(page);
        const seen = await viewer(page);
        assert.equal(seen.role, "dialog");
        assert.equal(seen.modal, "true");
        assert.ok(Math.abs(seen.width - seen.natural) < 1, `shown at ${seen.width}, natural ${seen.natural}`);
        assert.equal(seen.animations, 0, "nothing animates");
        assert.equal(seen.pageScroll, 0, "the page does not scroll sideways under it");
        await page.keyboard.press("Escape");
        await page.waitForFunction(() => !document.querySelector(".diagram-viewer"));
        assert.equal(await focusedOpener(page), true, "focus is back on the diagram");
      } finally {
        await close();
      }
    });

    it("opens from the keyboard, and keeps Tab inside while it is open", async () => {
      const { page, close } = await at(DESKTOP, fixture.url("wide-lr.md"));
      try {
        await loaded(page, 1);
        await page.locator(".markdown .diagram-open").focus();
        await page.keyboard.press("Enter");
        await opened(page);
        for (let i = 0; i < 4; i++) {
          await page.keyboard.press("Tab");
          assert.equal((await viewer(page)).focusInside, true, `focus left the view after ${i + 1} Tab presses`);
        }
        await page.keyboard.press("Shift+Tab");
        assert.equal((await viewer(page)).focusInside, true, "Shift+Tab left the view");
        await page.keyboard.press("Escape");
        await page.waitForFunction(() => !document.querySelector(".diagram-viewer"));
        assert.equal(await focusedOpener(page), true, "focus is back on the diagram");
        await page.keyboard.press(" ");
        await opened(page);
      } finally {
        await close();
      }
    });

    it("opens on a tap on a phone, and its close button is a tap target", async () => {
      const { page, close } = await at(PIXEL_7, fixture.url("wide-lr.md"));
      try {
        await loaded(page, 1);
        await page.locator(".markdown img.diagram").tap();
        await opened(page);
        const seen = await viewer(page);
        assert.ok(Math.abs(seen.width - seen.natural) < 1, `shown at ${seen.width}, natural ${seen.natural}`);
        assert.ok(seen.close.width >= 40 && seen.close.height >= 40, `the close button is ${JSON.stringify(seen.close)}`);
        assert.equal(seen.pageScroll, 0, "the page does not scroll sideways under it");
        await page.locator(".diagram-viewer-close").tap();
        await page.waitForFunction(() => !document.querySelector(".diagram-viewer"));
      } finally {
        await close();
      }
    });

    it("draws in the e-ink palette and with nothing animated when those settings are on", async () => {
      const { page, close } = await at(DESKTOP, fixture.url("wide-lr.md"), { settings: { light: true, noMotion: true } });
      try {
        await loaded(page, 1);
        await page.locator(".markdown img.diagram").click();
        await opened(page);
        const seen = await viewer(page);
        assert.match(seen.src, /theme=eink$/);
        assert.equal(seen.background, "rgb(255, 255, 255)");
        assert.equal(seen.animations, 0);
      } finally {
        await close();
      }
    });

    /**
     * The view follows the diagram as it is now, not as it was when it
     * opened (review of PR #202, finding 1). Each case edits a note of its
     * own, so none sees another's change.
     */
    const viewing = async (file, text, { profile = DESKTOP } = {}) => {
      fs.writeFileSync(fixture.file(file), text);
      const opened_ = await at(profile, fixture.url(file));
      await loaded(opened_.page, 1);
      await opened_.page.locator(".markdown img.diagram").click();
      await opened(opened_.page);
      return opened_;
    };
    const viewSrc = (page) => page.evaluate(() => document.querySelector(".diagram-viewer img")?.getAttribute("src") ?? null);
    const noteSrc = (page) => page.evaluate(() => document.querySelector(".markdown img.diagram")?.getAttribute("src") ?? null);

    it("names the action on the diagram's button, and describes it by its source", async () => {
      const { page, close } = await at(DESKTOP, fixture.url("wide-lr.md"));
      try {
        await loaded(page, 1);
        const button = page.getByRole("button", { name: "Open diagram at natural size" });
        assert.equal(await button.count(), 1);
        assert.equal(await button.getAttribute("aria-describedby"), await page.locator(".markdown img.diagram").getAttribute("id"));
      } finally {
        await close();
      }
    });

    it("follows a change of palette while it is open", async () => {
      const { page, close } = await viewing("view-palette.md", "# P\n\n" + fence(LONG_LR));
      try {
        assert.match(await viewSrc(page), /theme=light$/);
        await page.emulateMedia({ colorScheme: "dark" });
        await page.waitForFunction(() => document.querySelector(".markdown img.diagram")?.getAttribute("src").endsWith("theme=dark"));
        await waitFor(async () => (await viewSrc(page)) === (await noteSrc(page)), "the view to follow the note's image to the dark palette");
        await opened(page);
      } finally {
        await close();
      }
    });

    it("follows the diagram when it is edited on disk while open, and hands focus back to it", async () => {
      const { page, close } = await viewing("view-edit.md", "# E\n\n" + fence(LONG_LR) + "\nBelow.\n");
      try {
        const before = await viewSrc(page);
        fs.writeFileSync(fixture.file("view-edit.md"), "# E\n\n" + fence(LONG_LR + "  H --> Z[Archive]\n") + "\nBelow.\n");
        await waitFor(async () => {
          const [v, n] = [await viewSrc(page), await noteSrc(page)];
          return v !== before && v === n;
        }, "the view to show the edited drawing");
        // Still a modal: focus stayed inside it through the update, and Tab
        // cannot walk out to the page behind (review of PR #202, round two).
        await page.waitForTimeout(300);
        assert.equal((await viewer(page)).focusInside, true, "focus left the view when it followed the edit");
        for (let i = 0; i < 3; i++) {
          await page.keyboard.press("Tab");
          assert.equal((await viewer(page)).focusInside, true, `focus left the view after ${i + 1} Tab presses`);
        }
        await page.keyboard.press("Escape");
        await page.waitForFunction(() => !document.querySelector(".diagram-viewer"));
        assert.equal(await focusedOpener(page), true, "focus is on the edited diagram");
      } finally {
        await close();
      }
    });

    it("closes when the diagram is taken out of the note while open, with focus on the note's title", async () => {
      const { page, close } = await viewing("view-gone.md", "# G\n\n" + fence(LONG_LR) + "\nBelow.\n");
      try {
        fs.writeFileSync(fixture.file("view-gone.md"), "# G\n\nNo diagram now.\n\nBelow.\n");
        await page.waitForFunction(() => !document.querySelector(".diagram-viewer"));
        assert.equal(await page.evaluate(() => document.activeElement?.matches(".note-title") ?? false), true, "focus is on the title");
      } finally {
        await close();
      }
    });

    it("closes, rather than moving to another diagram, when the open one is deleted and another appended", async () => {
      const three = (a, b, c) => "# T\n\n" + [a, b, c].map((d) => fence(`graph LR; ${d}1 --> ${d}2\n`)).join("\n") + "\nBelow.\n";
      fs.writeFileSync(fixture.file("view-swap.md"), three("P", "Q", "R"));
      const { page, close } = await at(DESKTOP, fixture.url("view-swap.md"));
      try {
        await loaded(page, 3);
        await page.locator(".markdown img.diagram").nth(1).click();
        await opened(page);
        const opened_ = await viewSrc(page);
        // Q deleted and S appended: as many diagrams as before, and R, which
        // the reader did not open, now where Q was.
        fs.writeFileSync(fixture.file("view-swap.md"), three("P", "R", "S"));
        await page.waitForFunction(() => document.querySelectorAll(".markdown img.diagram").length === 3 && !document.querySelector(".diagram-viewer"), null, { timeout: 10000 })
          .catch(async () => assert.fail(`the view stayed open, showing ${await viewSrc(page)} (opened on ${opened_})`));
        assert.equal(await page.evaluate(() => document.activeElement?.matches(".note-title") ?? false), true, "focus is on the title");
      } finally {
        await close();
      }
    });

    it("closes when the whole note is deleted while open, with focus on the title the pane shows instead", async () => {
      const { page, close } = await viewing("view-deleted.md", "# D\n\n" + fence(LONG_LR));
      try {
        fs.unlinkSync(fixture.file("view-deleted.md"));
        await page.waitForFunction(() => !document.querySelector(".diagram-viewer"));
        await page.waitForTimeout(300);
        // The note's pane now says why there is no note, under its own title.
        const active = await page.evaluate(() => (document.activeElement?.matches(".note-title") ? "title" : document.activeElement?.tagName));
        assert.equal(active, "title", `focus fell to ${active}`);
      } finally {
        await close();
      }
    });

    it("closes when the diagram's drawing fails while open", async () => {
      const { page, close } = await viewing("view-fail.md", "# F\n\n" + fence(LONG_LR) + "\nBelow.\n");
      try {
        // Every drawing asked for from now on fails; a change of palette asks.
        await page.route("**/api/r/*/diagram/**", (route) => route.abort("internetdisconnected"));
        await page.emulateMedia({ colorScheme: "dark" });
        await page.waitForFunction(() => !document.querySelector(".markdown img.diagram"));
        await page.waitForFunction(() => !document.querySelector(".diagram-viewer"));
        assert.equal(await page.evaluate(() => document.activeElement?.matches(".note-title") ?? false), true, "focus is on the title");
      } finally {
        await close();
      }
    });
  });

  it("shows as code a block the layout refuses, without ever asking for an image", async () => {
    // The note endpoint draws each block to measure it (#177), so a refusal
    // by the layout is known before the page decorates: the block is not
    // listed, and it is code from the first frame.
    const { page, requests, close } = await open();
    try {
      await openNote(page, fixture.url("refused.md"));
      const seen = await page.evaluate(() => ({
        imgs: document.querySelectorAll(".markdown img").length,
        shown: getComputedStyle(document.querySelector(".markdown pre")).display !== "none",
      }));
      assert.deepEqual(seen, { imgs: 0, shown: true });
      fs.appendFileSync(fixture.file("refused.md"), "\nA paragraph added below.\n");
      await page.locator(".markdown p", { hasText: "A paragraph added below." }).waitFor();
      assert.equal(requests.length, 0, `a drawing was asked for: ${requests.join(", ")}`);
    } finally {
      await close();
    }
  });

  it("shows the code block, and no broken image, when the drawing cannot be fetched, and keeps it through a live update", async () => {
    const { page, requests, close } = await open();
    try {
      await page.route("**/api/r/*/diagram/**", (route) => route.abort("internetdisconnected"));
      await openNote(page, fixture.url("unreachable.md"));
      await waitFor(() => requests.length > 0, "the diagram request");
      await page.waitForFunction(
        () => [...document.querySelectorAll(".markdown pre")].every((p) => getComputedStyle(p).display !== "none"),
      );
      assert.equal(await page.locator(".markdown img").count(), 0);

      // A live update elsewhere in the note leaves the failed block as code:
      // it is not hidden again, and not asked for again (review of PR #175, N3).
      await page.evaluate(() => {
        window.__hidden = 0;
        new MutationObserver(() => {
          if (document.querySelector(".markdown pre.diagram-source")) window.__hidden++;
        }).observe(document.querySelector(".markdown"), { subtree: true, childList: true, attributes: true });
      });
      fs.appendFileSync(fixture.file("unreachable.md"), "\nA paragraph added below.\n");
      await page.locator(".markdown p", { hasText: "A paragraph added below." }).waitFor();
      assert.equal(await page.evaluate(() => window.__hidden), 0, "the code block was hidden again");
      assert.equal(requests.length, 1, `the failed drawing was asked for again: ${requests.join(", ")}`);
    } finally {
      await close();
    }
  });

  it("redraws a diagram edited on disk, and does not fetch an unchanged one again", async () => {
    const { page, requests, close } = await open();
    try {
      await openNote(page, fixture.url("live.md"));
      await loaded(page, 2);
      await page.evaluate(() => {
        document.querySelector(".markdown img.diagram").__kept = true;
      });
      const [first, second] = requests;
      assert.equal(requests.length, 2);

      fs.writeFileSync(
        fixture.file("live.md"),
        "# Live\n\nA paragraph added above.\n\n" + fence("graph TD; P-->Q\n") + "\n" + fence("graph TD; X-->Y-->Z\n"),
      );
      await page.waitForFunction(
        (old) => {
          const imgs = [...document.querySelectorAll(".markdown img.diagram")];
          return imgs.length === 2 && imgs[1].getAttribute("src") !== old && imgs[1].complete && imgs[1].naturalWidth > 0;
        },
        new URL(second).pathname + new URL(second).search,
      );
      const kept = await page.evaluate(() => document.querySelector(".markdown img.diagram").__kept === true);
      assert.equal(kept, true, "the unchanged diagram is the same element");
      assert.equal(requests.filter((u) => u === first).length, 1, `the unchanged diagram was fetched once: ${requests.join(", ")}`);
      assert.equal(requests.length, 3, `one new request, for the edited diagram: ${requests.join(", ")}`);
      assert.equal(await page.locator(".markdown p", { hasText: "A paragraph added above." }).count(), 1);
    } finally {
      await close();
    }
  });

  /**
   * Scroll-to-line with real layout (davison/md-notes#177). Every drawing is
   * held back until well after the page has scrolled, so the scroll always
   * meets the images before they have loaded, however fast the machine is;
   * `fail` then aborts them rather than letting them through. The target
   * must end in view once every image has settled, which the pre-M9 build
   * did and the first M9 build did not.
   */
  const landsOn = async (profile, line, { fail = false, file = "flows.md" } = {}) => {
    const { name, ...options } = profile;
    const ctx = await browser.newContext({ ...options, colorScheme: "light", serviceWorkers: "block" });
    try {
      const page = await ctx.newPage();
      const statuses = [];
      let listed = null;
      page.on("response", async (r) => {
        if (r.url().includes("/diagram/")) statuses.push(r.status());
        if (r.url().includes("/note/")) listed = (await r.json().catch(() => ({}))).diagrams ?? [];
      });
      await page.route("**/api/r/*/diagram/**", async (route) => {
        await new Promise((r) => setTimeout(r, 400));
        if (fail) await route.abort("internetdisconnected");
        else await route.continue();
      });
      await openNote(page, fixture.url(file, `?l=${line}`));
      await page.waitForFunction(() => document.querySelectorAll(".markdown pre").length > 0);
      // Settled: every image near the screen loaded, or taken away for its
      // code block. One far off stays unloaded, being lazy, and is not waited
      // for; it has no box to move anything by until the reader goes to it.
      await page.waitForFunction(
        () =>
          [...document.querySelectorAll(".markdown img.diagram")].every((i) => {
            const r = i.getBoundingClientRect();
            const near = r.bottom > -innerHeight && r.top < 2 * innerHeight;
            return !near || (i.complete && i.naturalWidth > 0);
          }) &&
          [...document.querySelectorAll(".markdown pre")].every(
            (p) => getComputedStyle(p).display !== "none" || !!p.previousElementSibling?.matches(".diagram-box"),
          ),
        null,
        { timeout: 20000 },
      );
      await page.waitForTimeout(300);
      const seen = await page.evaluate((l) => {
        let best = null;
        let bestLine = -1;
        for (const el of document.querySelectorAll(".markdown [data-line]")) {
          const n = Number(el.getAttribute("data-line"));
          if (n <= l && n >= bestLine) [best, bestLine] = [el, n];
        }
        // An anchor is empty; what the reader sees is the block after it.
        let shown = best.classList.contains("line-anchor")
          ? [best.nextElementSibling, best.nextElementSibling?.nextElementSibling].find(
              (e) => e && getComputedStyle(e).display !== "none",
            )
          : best;
        // A drawing stands in a box of its own.
        shown = shown.querySelector?.("img.diagram") ?? shown;
        const r = shown.getBoundingClientRect();
        // Each measured diagram's displayed scale, from its reserved box: a
        // lazy one far above the hit may not have loaded at all.
        const scales = [...document.querySelectorAll(".markdown img.diagram[width]")].map(
          (i) => i.getBoundingClientRect().width / Number(i.getAttribute("width")),
        );
        return { tag: shown.tagName, top: Math.round(r.top), bottom: Math.round(r.bottom), height: innerHeight, scales };
      }, line);
      const unmeasured = (listed ?? []).filter((d) => !d.width).length;
      return { ...seen, statuses, listed: listed?.length ?? 0, unmeasured };
    } finally {
      await ctx.close();
    }
  };

  for (const [p, profile] of [DESKTOP, PIXEL_7].entries()) {
    const many = (i) => ({ m: MANY[2 * p + i], file: `many-${2 * p + i}.md` });
    for (const [what, line, tag, opts] of [
      ["inside the ninth diagram", FLOWS.at.inDiagram, "IMG", {}],
      ["on a block shown as code below the diagrams", FLOWS.at.codeBlock, "PRE", {}],
      ["on a paragraph below the diagrams", FLOWS.at.paragraph, "P", {}],
      ["on a paragraph below diagrams that all fail to load", FLOWS.at.paragraph, "P", { fail: true }],
      ["inside a diagram the daemon had no time to measure", many(0).m.at.inDiagram, "IMG", { file: many(0).file, many: true }],
      ["below more diagrams than the daemon had time to measure", many(1).m.at.paragraph, "P", { file: many(1).file, many: true, refused: true }],
    ]) {
      it(`lands a hit ${what}, at ${profile.name}`, async () => {
        const seen = await landsOn(profile, line, opts);
        assert.equal(seen.tag, tag);
        if (opts.many) {
          // The case is what it says: the budget left most of the note's
          // diagrams without a size.
          assert.ok(seen.unmeasured >= 10, `only ${seen.unmeasured} of ${seen.listed} diagrams went out unmeasured`);
        }
        if (opts.refused) {
          // The block the layout refuses was not reached either, so the
          // route answered its image with 422 and the page fell back to its
          // code (review of PR #181, N1).
          assert.ok(seen.statuses.includes(422), `no 422 among ${seen.statuses.join(", ")}`);
        }
        // Centred, as the pre-M9 build centres it: the scroll puts the
        // block's start in the middle of the screen, and it stays there.
        assert.ok(
          Math.abs(seen.top - seen.height / 2) < seen.height / 4,
          `the target's top is at ${seen.top}px in a ${seen.height}px viewport: ${JSON.stringify(seen)}`,
        );
      });
    }
  }

  for (const profile of WIDTHS) {
    it(`lands a hit below a wide diagram held at the floor, at ${profile.name}`, async () => {
      const seen = await landsOn(profile, WIDE_NOTE.paragraph, { file: "wide-lr.md" });
      assert.equal(seen.tag, "P");
      // The case is what it says: the diagram above the hit is shown at the
      // floor, not shrunk to the column, so its box is the displayed size.
      assert.equal(seen.scales.length, 1, `the diagram was measured: ${JSON.stringify(seen)}`);
      assert.ok(seen.scales[0] >= MIN_SCALE - 0.001, `the diagram is shown at ${seen.scales[0].toFixed(3)}`);
      assert.ok(
        Math.abs(seen.top - seen.height / 2) < seen.height / 4,
        `the target's top is at ${seen.top}px in a ${seen.height}px viewport: ${JSON.stringify(seen)}`,
      );
    });
  }

  describe("the drawing opened as a document", () => {
    /** The URL the reading view uses for flow.md's diagram, taken from the page. */
    const diagramURL = async () => {
      const { page, close } = await open();
      try {
        await openNote(page, fixture.url("flow.md"));
        await loaded(page, 1);
        return new URL(await page.locator(".markdown img.diagram").getAttribute("src"), fixture.origin).href;
      } finally {
        await close();
      }
    };

    /**
     * Opens the diagram's URL as a top-level document, with a script spliced
     * into the daemon's real response — what a writer bug that let markup
     * through would serve. `keepCSP: false` drops the daemon's policy, which
     * is the control that shows the check can see a script run.
     */
    const openWithScript = async (url, { keepCSP }) => {
      const { page, close } = await open();
      await page.route(url, async (route) => {
        const res = await route.fetch();
        const headers = res.headers();
        if (!keepCSP) delete headers["content-security-policy"];
        const body = (await res.text()).replace(
          /<\/svg>\s*$/,
          '<script>document.documentElement.setAttribute("data-ran", "yes")</script></svg>',
        );
        await route.fulfill({ status: res.status(), headers, body });
      });
      await page.goto(url);
      await page.waitForLoadState("load");
      const ran = await page.evaluate(() => document.documentElement.getAttribute("data-ran"));
      return { ran, close };
    };

    it("draws, with no script in it", async () => {
      const url = await diagramURL();
      const { page, close } = await open();
      try {
        const res = await page.goto(url);
        assert.equal(res.headers()["content-type"], "image/svg+xml");
        assert.equal(res.headers()["content-security-policy"], "default-src 'none'; sandbox");
        assert.equal(res.headers()["x-content-type-options"], "nosniff");
        const seen = await page.evaluate(() => ({
          root: document.documentElement.localName,
          scripts: document.getElementsByTagName("script").length,
          shapes: document.querySelectorAll("rect, path, polygon").length,
        }));
        assert.deepEqual({ ...seen, shapes: seen.shapes > 0 }, { root: "svg", scripts: 0, shapes: true });
      } finally {
        await close();
      }
    });

    it("cannot run a script even if one were in it", async () => {
      const url = await diagramURL();
      const control = await openWithScript(url, { keepCSP: false });
      await control.close();
      assert.equal(control.ran, "yes", "without the policy the spliced script runs, so the check can see one");
      const guarded = await openWithScript(url, { keepCSP: true });
      await guarded.close();
      assert.equal(guarded.ran, null, "under the daemon's policy it does not");
    });
  });
});
