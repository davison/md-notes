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
import { DESKTOP, PIXEL_7, loadPlaywright, missingPrerequisite, openNote, startFixture, waitFor } from "./harness.mjs";

const playwright = loadPlaywright();
const blocker = missingPrerequisite(playwright);
if (blocker) console.log(`# skipped: ${blocker}`);

const FLOW = "flowchart LR\n  A[Start] --> B{Ready?}\n  B -->|yes| C[Ship]\n  B -->|no| A\n";
const SEQUENCE = "sequenceDiagram\n  Alice->>Bob: Hello\n";
/** A long left-to-right chain: wider than the reading column. */
const WIDE = "flowchart LR\n  " + Array.from({ length: 14 }, (_, i) => `N${i}[Step number ${i}]`).join(" --> ") + "\n";

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
    write("refused.md", "# Refused\n\n" + fence(layoutRefused()));
    write("live.md", "# Live\n\n" + fence("graph TD; P-->Q\n") + "\n" + fence("graph TD; X-->Y\n"));
    write("flows.md", FLOWS.text);
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
            anchor: img.previousElementSibling?.className,
            codeHidden: getComputedStyle(img.nextElementSibling).display === "none",
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

  it("shrinks a wide diagram to the reading column", async () => {
    const { page, close } = await open();
    try {
      await openNote(page, fixture.url("wide.md"));
      await loaded(page, 1);
      const seen = await page.evaluate(() => {
        const img = document.querySelector(".markdown img.diagram");
        return {
          width: img.getBoundingClientRect().width,
          column: document.querySelector(".markdown").getBoundingClientRect().width,
          natural: img.naturalWidth,
        };
      });
      assert.ok(seen.natural > seen.column, `the fixture is wider than the column (${seen.natural} > ${seen.column})`);
      assert.ok(Math.abs(seen.width - seen.column) < 1, `the image fits the column: ${seen.width} vs ${seen.column}`);
    } finally {
      await close();
    }
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
  const landsOn = async (profile, line, { fail = false } = {}) => {
    const { name, ...options } = profile;
    const ctx = await browser.newContext({ ...options, colorScheme: "light", serviceWorkers: "block" });
    try {
      const page = await ctx.newPage();
      await page.route("**/api/r/*/diagram/**", async (route) => {
        await new Promise((r) => setTimeout(r, 400));
        if (fail) await route.abort("internetdisconnected");
        else await route.continue();
      });
      await openNote(page, fixture.url("flows.md", `?l=${line}`));
      await page.waitForFunction(() => document.querySelectorAll(".markdown pre").length > 0);
      // Settled: every image loaded, or taken away for its code block.
      await page.waitForFunction(
        () =>
          [...document.querySelectorAll(".markdown img.diagram")].every((i) => i.complete && i.naturalWidth > 0) &&
          [...document.querySelectorAll(".markdown pre")].every(
            (p) => getComputedStyle(p).display !== "none" || p.previousElementSibling?.matches("img.diagram"),
          ),
        null,
        { timeout: 10000 },
      );
      await page.waitForTimeout(100);
      return await page.evaluate((l) => {
        let best = null;
        let bestLine = -1;
        for (const el of document.querySelectorAll(".markdown [data-line]")) {
          const n = Number(el.getAttribute("data-line"));
          if (n <= l && n >= bestLine) [best, bestLine] = [el, n];
        }
        // An anchor is empty; what the reader sees is the block after it.
        const shown = best.classList.contains("line-anchor")
          ? [best.nextElementSibling, best.nextElementSibling?.nextElementSibling].find(
              (e) => e && getComputedStyle(e).display !== "none",
            )
          : best;
        const r = shown.getBoundingClientRect();
        return { tag: shown.tagName, top: Math.round(r.top), bottom: Math.round(r.bottom), height: innerHeight };
      }, line);
    } finally {
      await ctx.close();
    }
  };

  for (const profile of [DESKTOP, PIXEL_7]) {
    for (const [what, line, tag, opts] of [
      ["inside the ninth diagram", FLOWS.at.inDiagram, "IMG", {}],
      ["on a block shown as code below the diagrams", FLOWS.at.codeBlock, "PRE", {}],
      ["on a paragraph below the diagrams", FLOWS.at.paragraph, "P", {}],
      ["on a paragraph below diagrams that all fail to load", FLOWS.at.paragraph, "P", { fail: true }],
    ]) {
      it(`lands a hit ${what}, at ${profile.name}`, async () => {
        const seen = await landsOn(profile, line, opts);
        assert.equal(seen.tag, tag);
        // Centred, as the pre-M9 build centres it: the scroll puts the
        // block's start in the middle of the screen, and it stays there.
        assert.ok(
          Math.abs(seen.top - seen.height / 2) < seen.height / 4,
          `the target's top is at ${seen.top}px in a ${seen.height}px viewport: ${JSON.stringify(seen)}`,
        );
      });
    }
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
