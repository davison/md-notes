/**
 * End-to-end check of pasting and dropping images into the editor
 * (davison/md-notes#214, M11-R1): against the built daemon in headless
 * Chromium, a paste and a two-file drop in a note two folders down put the
 * files in the root's `_resources`, write links that climb to it into the
 * note on disk, and render as loaded images in the reading view.
 *
 *     make e2e
 *     pnpm --dir ui e2e
 *
 * The clipboard and the file manager are not reachable from a headless
 * browser, so the events are the ones they would deliver, built in the page:
 * a ClipboardEvent whose DataTransfer holds a PNG File, and a DragEvent whose
 * DataTransfer holds two. The PNGs are drawn on a canvas, so they are real
 * images the daemon's content check and the browser's decoder both accept.
 *
 * Nothing here touches a daemon you are running: see ./harness.mjs.
 */

import { after, before, describe, it } from "node:test";
import assert from "node:assert/strict";
import fs from "node:fs";
import path from "node:path";
import { gate, loadPlaywright, missingPrerequisite, startFixture, waitFor } from "./harness.mjs";

const playwright = loadPlaywright();
const blocker = gate(missingPrerequisite(playwright));

const NOTE = "projects/deep/nested.md";

/**
 * Builds PNG files in the page, one per colour, and dispatches `kind` —
 * "paste" or "drop" — carrying them at the editor. A drop is aimed at the
 * middle of the given line's element.
 */
async function deliver(page, kind, files, line = 0) {
  await page.evaluate(
    async ({ kind, files, line }) => {
      const dt = new DataTransfer();
      for (const { name, colour } of files) {
        const canvas = document.createElement("canvas");
        canvas.width = 6;
        canvas.height = 4;
        const g = canvas.getContext("2d");
        g.fillStyle = colour;
        g.fillRect(0, 0, 6, 4);
        const blob = await new Promise((r) => canvas.toBlob(r, "image/png"));
        dt.items.add(new File([blob], name, { type: "image/png" }));
      }
      const content = document.querySelector(".cm-content");
      if (kind === "paste") {
        content.dispatchEvent(new ClipboardEvent("paste", { clipboardData: dt, bubbles: true, cancelable: true }));
      } else {
        const box = content.querySelectorAll(".cm-line")[line].getBoundingClientRect();
        const at = { clientX: box.left + 4, clientY: box.top + box.height / 2 };
        for (const type of ["dragenter", "dragover", "drop"]) {
          content.dispatchEvent(new DragEvent(type, { dataTransfer: dt, bubbles: true, cancelable: true, ...at }));
        }
      }
    },
    { kind, files, line },
  );
}

describe("pasting and dropping images into the editor", { skip: blocker ?? false }, () => {
  let fixture, browser, page;
  const source = () => fs.readFileSync(fixture.file(NOTE), "utf8");
  const resources = () => {
    const dir = path.join(fixture.notesDir, "_resources");
    return fs.existsSync(dir) ? fs.readdirSync(dir).sort() : [];
  };

  before(async () => {
    fixture = await startFixture("images");
    browser = await playwright.chromium.launch({ headless: true });
    page = await (await browser.newContext()).newPage();
  });

  after(async () => {
    await browser?.close();
    await fixture?.stop();
  });

  it("uploads a pasted image and links it from a note two folders down", async () => {
    assert.deepEqual(resources(), [], "the fixture starts with no _resources");
    await page.goto(fixture.url(NOTE));
    await page.getByRole("button", { name: "Edit" }).click();
    await page.locator(".cm-content").click();

    await deliver(page, "paste", [{ name: "image.png", colour: "#c00" }]);

    const pasted = await waitFor(() => resources().find((n) => /^[0-9a-f]{32}\.png$/.test(n)), "the pasted file on disk");
    const link = `![](../../_resources/${pasted})`;
    await waitFor(() => source().includes(link), `autosave to write ${link}`);
    assert.equal(await page.locator("[role=alert]").count(), 0, "no failure notice");
  });

  it("links each dropped file at the drop point, named for the file", async () => {
    // The third line of the editor is "Paragraph 1 of the nested note.".
    await deliver(
      page,
      "drop",
      [
        { name: "Blue square.png", colour: "#00c" },
        { name: "green.png", colour: "#0c0" },
      ],
      2,
    );
    await waitFor(() => resources().includes("Blue-square.png") && resources().includes("green.png"), "both dropped files on disk");
    const links = "![Blue square](../../_resources/Blue-square.png)\n![green](../../_resources/green.png)";
    await waitFor(() => source().includes(links), "autosave to write both links");
    // Aimed at the start of the line, so the links go in just before it.
    assert.ok(source().includes(`\n\n${links}Paragraph 1 of the nested note.\n`), "the links went where they were dropped");
    assert.equal(resources().length, 3);
  });

  it("shows every image loaded in the reading view", async () => {
    await page.getByRole("button", { name: "View" }).click();
    const images = page.locator(".note-body img");
    await waitFor(async () => (await images.count()) === 3, "three images in the reading view");
    const loaded = await waitFor(
      () =>
        images.evaluateAll((els) => {
          const all = els.map((el) => ({ src: el.getAttribute("src"), ok: el.complete && el.naturalWidth > 0 }));
          return all.every((i) => i.ok) ? all : null;
        }),
      "the images to load",
    );
    for (const { src } of loaded) assert.match(src, /^\/api\/r\/notes\/raw\/_resources\/[^/]+\.png$/);
    assert.equal(loaded[0].ok && (await images.first().evaluate((el) => el.naturalWidth)), 6, "drawn at the canvas's size");
  });

  it("leaves _resources out of the navigator", async () => {
    const tree = await page.locator(".tree").textContent();
    assert.doesNotMatch(tree, /_resources/);
  });
});
