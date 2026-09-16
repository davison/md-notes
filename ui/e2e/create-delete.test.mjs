/**
 * End-to-end checks of creating and deleting a note, against the built
 * daemon in headless Chromium (davison/md-notes#77), on the rig the rest of
 * ui/e2e shares (davison/md-notes#78).
 *
 *     make e2e
 *     pnpm --dir ui e2e
 *
 * `make build` first if you run the suite by hand: the daemon serves the
 * `ui/dist` embedded in the binary, so `pnpm --dir ui build` alone leaves
 * these checks reading yesterday's assets.
 *
 * The daemon under test gets a temporary root, its own config, state and
 * token files, and an ephemeral port the operating system hands out, so
 * nothing here touches a daemon you are running or the state under
 * ~/.local/state/mdn. See ./harness.mjs.
 */

import { after, before, describe, it } from "node:test";
import assert from "node:assert/strict";
import fs from "node:fs";
import { TAP_TARGET, loadPlaywright, missingPrerequisite, startFixture, waitFor } from "./harness.mjs";

const playwright = loadPlaywright();
const blocker = missingPrerequisite(playwright);
if (blocker) console.log(`# skipped: ${blocker}`);

describe("creating and deleting a note in the browser", { skip: blocker ?? false }, () => {
  let fixture, origin, browser, context, page;

  const notePath = (p) => fixture.file(p);
  const exists = (p) => fs.existsSync(notePath(p));

  /** The dialog on screen, and the two ways out of it. */
  const modal = () => page.locator(".modal");
  /**
   * Open and *ready*: the element is in the page a frame before the effect
   * that moves focus into it and subscribes to Escape has run, and a
   * keystroke sent in that frame reaches neither. Waiting for focus to
   * land is both the synchronisation and the check that it lands.
   */
  const dialogReady = (target = page) =>
    waitFor(
      () => target.evaluate(() => !!document.activeElement?.closest(".modal")),
      "focus to move into the dialog",
    );
  const confirmButton = () => page.locator(".modal button.primary");
  const nameBox = () => page.locator(".modal-name");

  before(async () => {
    fixture = await startFixture("create-delete");
    origin = fixture.origin;
    browser = await playwright.chromium.launch({ headless: true });
    context = await browser.newContext();
    page = await context.newPage();
  });

  after(async () => {
    await browser?.close();
    await fixture?.stop();
  });

  it("creates a note from the top bar, named in a prompt, and opens it in the editor", async () => {
    await page.goto(`${origin}/r/notes/`);
    await page.locator(".tree").waitFor();
    assert.equal(exists("Shopping list.md"), false);

    await page.getByRole("button", { name: "New note" }).click();
    await dialogReady();
    // Nothing is written by opening the prompt.
    assert.equal(exists("Shopping list.md"), false);

    // Typed, then confirmed from the keyboard: Enter is the confirm.
    await nameBox().fill("Shopping list");
    await page.keyboard.press("Enter");

    await page.waitForURL(`${origin}/r/notes/Shopping%20list.md`);
    await page.locator(".cm-editor").waitFor();
    assert.equal(exists("Shopping list.md"), true, "the file is on disk");
    assert.equal(fs.readFileSync(notePath("Shopping list.md"), "utf8"), "", "a new note is empty");
    assert.equal(await modal().count(), 0, "the prompt is gone");
  });

  it("puts a bare title in the open note's folder, and a name with a slash under the root", async () => {
    await page.goto(`${origin}/r/notes/docs/guide.md`);
    await page.locator(".note-bar").waitFor();

    await page.getByRole("button", { name: "New note" }).click();
    assert.equal(await page.locator(".modal-folder").textContent(), "docs");
    await nameBox().fill("Ideas");
    await confirmButton().click();
    await page.waitForURL(`${origin}/r/notes/docs/Ideas.md`);
    assert.equal(exists("docs/Ideas.md"), true);

    await page.getByRole("button", { name: "New note" }).click();
    await nameBox().fill("archive/2026/january.md");
    await confirmButton().click();
    await page.waitForURL(`${origin}/r/notes/archive/2026/january.md`);
    assert.equal(exists("archive/2026/january.md"), true, "missing parents are created");
  });

  it("shows the daemon's refusal in the prompt and keeps the name for correcting", async () => {
    await page.goto(`${origin}/r/notes/`);
    await page.locator(".tree").waitFor();
    const before = fs.readFileSync(notePath("index.md"), "utf8");

    await page.getByRole("button", { name: "New note" }).click();
    await nameBox().fill("index");
    await confirmButton().click();

    const alert = page.locator(".modal [role=alert]");
    await alert.waitFor();
    assert.match(await alert.textContent(), /already exists/);
    assert.equal(await nameBox().inputValue(), "index", "the typed name is still there");
    assert.equal(fs.readFileSync(notePath("index.md"), "utf8"), before, "the existing note is untouched");

    // Correcting the name from where it stands finishes the job.
    await nameBox().fill("index of things");
    await confirmButton().click();
    await page.waitForURL(`${origin}/r/notes/index%20of%20things.md`);
    assert.equal(exists("index of things.md"), true);
  });

  it("carries a creation and a deletion to another tab's navigator through the events stream", async () => {
    // A second page, opened before either change and never reloaded: what
    // it shows can only have come from the daemon's change batch.
    const watcher = await context.newPage();
    await watcher.goto(`${origin}/r/notes/`);
    await watcher.locator(".tree").waitFor();
    const link = (name) => watcher.locator(`.tree a.file`, { hasText: name });
    assert.equal(await link("Watched.md").count(), 0);

    await page.goto(`${origin}/r/notes/`);
    await page.locator(".tree").waitFor();
    await page.getByRole("button", { name: "New note" }).click();
    await nameBox().fill("Watched");
    await confirmButton().click();
    await page.waitForURL(`${origin}/r/notes/Watched.md`);

    await link("Watched.md").first().waitFor({ timeout: 10000 });

    await page.locator(".note-bar .delete-note").click();
    await confirmButton().click();
    await page.waitForURL(`${origin}/r/notes/`);
    await waitFor(() => link("Watched.md").count().then((n) => n === 0), "the other tab to lose the note");
    await watcher.close();
  });

  it("removes nothing when the confirmation is cancelled, by button or by Escape", async () => {
    await page.goto(`${origin}/r/notes/docs/guide.md`);
    await page.locator(".note-bar").waitFor();

    await page.locator(".note-bar .delete-note").click();
    await dialogReady();
    assert.match(await modal().textContent(), /docs\/guide\.md/, "the confirmation names the file");
    await page.locator(".modal button", { hasText: "Cancel" }).click();
    await waitFor(() => modal().count().then((n) => n === 0), "the dialog to close");
    assert.equal(exists("docs/guide.md"), true, "cancelling removed nothing");

    await page.locator(".note-bar .delete-note").click();
    await dialogReady();
    await page.keyboard.press("Escape");
    await waitFor(() => modal().count().then((n) => n === 0), "the dialog to close");
    assert.equal(exists("docs/guide.md"), true, "Escape removed nothing");
    assert.equal(page.url(), `${origin}/r/notes/docs/guide.md`, "and the note is still open");
  });

  it("deletes the open note after the confirmation and leaves it for the root's home", async () => {
    await page.goto(`${origin}/r/notes/docs/guide.md`);
    await page.locator(".note-bar").waitFor();

    await page.locator(".note-bar .delete-note").click();
    await confirmButton().click();

    await page.waitForURL(`${origin}/r/notes/`);
    assert.equal(exists("docs/guide.md"), false, "the file is gone");
    assert.equal(exists("docs/Ideas.md"), true, "and only that one file is gone");
    await waitFor(
      () => page.locator(".tree a.file", { hasText: "guide.md" }).count().then((n) => n === 0),
      "this tab's navigator to lose the note",
    );
  });

  it("keeps the delete button in one place across an edit, in both layouts", async () => {
    // The operator's finding on #74: on an unedited note the button sat at
    // the left of the bar, moved right on entering the editor and stayed
    // there on the way back. Its place must not be a function of whether
    // the session has read the note yet, so it is measured three times —
    // before, during and after an edit — at both layouts.
    const layouts = [
      { name: "wide", viewport: { width: 1280, height: 900 } },
      { name: "drawer", viewport: { width: 390, height: 844 }, hasTouch: true, isMobile: true },
    ];
    for (const layout of layouts) {
      const ctx = await browser.newContext(layout);
      try {
        const view = await ctx.newPage();
        await view.goto(`${origin}/r/notes/index.md`);
        await view.locator(".note-bar").waitFor();
        const del = view.locator(".note-bar .delete-note");

        const unedited = await del.boundingBox();
        await view.getByRole("button", { name: "Edit" }).click();
        await view.locator(".cm-editor").waitFor();
        const editing = await del.boundingBox();
        await view.getByRole("button", { name: "View" }).click();
        await view.locator(".note-body").waitFor();
        const afterwards = await del.boundingBox();

        assert.deepEqual(editing, unedited, `${layout.name}: the button moved on entering the editor`);
        assert.deepEqual(afterwards, unedited, `${layout.name}: the button moved on leaving the editor`);
        // And it is where it is meant to be: hard right, inside the bar.
        const bar = await view.locator(".note-bar").boundingBox();
        assert.ok(
          unedited.x + unedited.width > bar.x + bar.width - 40,
          `${layout.name}: the button is not at the right-hand end (${unedited.x + unedited.width} of ${bar.x + bar.width})`,
        );
      } finally {
        await ctx.close();
      }
    }
  });

  it("closes the prompt on Escape and leaves the editor under it untouched", async () => {
    // What this holds: a reader who opens the prompt over a half-typed note,
    // changes their mind and presses Escape gets the prompt closed and the
    // editor exactly as they left it — same text, still in insert mode.
    //
    // What it does *not* hold, and cannot: the dialog's `stopPropagation`.
    // The review of PR #83 left that for a browser, and it was held here
    // against the drawer until #85 moved the create control into the top
    // bar. Measured after that move: with the drawer open the top-bar
    // control is visible but not clickable (the drawer's backdrop takes the
    // click), and opening the prompt closes the settings panel (a
    // pointerdown outside it), so neither of the two other Escape handlers
    // in the application can be underneath a dialog any more. The editor is
    // not a third: with the prompt open the key lands on `.modal-name`, and
    // the editor's vim keymap is a listener on its own element, which a
    // keydown dispatched at the dialog never reaches with or without the
    // call. Commenting out `e.stopPropagation()` in ui/src/dialog.tsx fails
    // nothing in this suite, which is why this case does not claim to be
    // that check. The unit test on #83 is where that rule now lives; see the
    // decision on #78.
    await page.goto(`${origin}/r/notes/index.md`);
    await page.locator(".note-bar").waitFor();
    await page.getByRole("button", { name: "Edit" }).click();
    await page.locator(".cm-editor").waitFor();

    // Into insert mode, and typing. `cm-vimMode` on the scroller is how the
    // vim extension says the editor is in normal mode; inserting takes it
    // off, and that is the state this check is about.
    await page.locator(".cm-content").click();
    await page.keyboard.press("i");
    await page.waitForFunction(() => !document.querySelector(".cm-scroller").classList.contains("cm-vimMode"));
    await page.keyboard.type("half a wor");

    await page.locator(".new-note").click();
    await dialogReady();
    await page.keyboard.press("Escape");
    await waitFor(() => modal().count().then((n) => n === 0), "the prompt to close");

    assert.equal(
      await page.evaluate(() => document.querySelector(".cm-scroller").classList.contains("cm-vimMode")),
      false,
      "the editor under the prompt is still in insert mode",
    );
    assert.match(
      await page.locator(".cm-content").textContent(),
      /half a wor/,
      "and still holds what was typed",
    );

    // The control, so the check above is known to be about the dialog and
    // not about an editor that ignores Escape: closing the prompt hands
    // focus back to the button that opened it, so the editor is given it
    // again, and then the very same key does leave insert mode.
    await page.locator(".cm-content").click();
    await page.keyboard.press("Escape");
    await page.waitForFunction(() => document.querySelector(".cm-scroller").classList.contains("cm-vimMode"));
  });

  it("works in the drawer layout on a coarse pointer, with 40 px tap targets", async () => {
    // A phone: the narrow layout, a touch screen and no hover — the three
    // things the drawer and the tap-target block key off.
    const phone = await browser.newContext({
      viewport: { width: 390, height: 844 },
      hasTouch: true,
      isMobile: true,
    });
    try {
      const small = await phone.newPage();
      await small.goto(`${origin}/r/notes/index.md`);
      await small.locator(".note-bar").waitFor();

      // The delete action is in the note bar at this width too, and is a
      // 40 px target.
      const del = small.locator(".note-bar .delete-note");
      const delBox = await del.boundingBox();
      assert.ok(delBox.height >= TAP_TARGET, `the delete target is ${delBox.height}px tall`);

    // The create control is in the top bar at this width too — on screen
    // without opening the drawer, which is the point of #85 — and a square
    // the size of the burger beside it.
    const create = small.getByRole("button", { name: "New note" });
    assert.equal(await create.isVisible(), true, "the create control is visible with the drawer closed");
    assert.equal(await small.locator(".topbar .new-note").count(), 1, "and it is in the top bar");
    assert.equal(await small.locator(".nav .new-note").count(), 0, "and not in the navigator");
    const createBox = await create.boundingBox();
    assert.ok(createBox.height >= TAP_TARGET, `the create target is ${createBox.height}px tall`);
    assert.ok(createBox.width >= TAP_TARGET, `the create target is ${createBox.width}px wide`);
    // The label is dropped at this width; the accessible name is not.
    assert.equal(await small.locator(".new-note-label").isVisible(), false);

      await create.click();
      await dialogReady(small);
      const box = small.locator(".modal-name");
      // The dialog is over the drawer rather than inside it: it is wider than
      // the drawer, which is min(20rem, 85vw) = 331px here.
      const modalBox = await small.locator(".modal").boundingBox();
      assert.ok(modalBox.width > 331, `the dialog is ${modalBox.width}px wide`);
      for (const button of await small.locator(".modal button").all()) {
        const b = await button.boundingBox();
        assert.ok(b.height >= TAP_TARGET, `a dialog button is ${b.height}px tall`);
      }

      await box.fill("From the phone");
      await small.locator(".modal button.primary").click();
      await small.waitForURL(`${origin}/r/notes/From%20the%20phone.md`);
      assert.equal(exists("From the phone.md"), true);
      await small.locator(".cm-editor").waitFor();

      // And delete it again from here, to close the loop at this width.
      await small.locator(".note-bar .delete-note").click();
      await small.locator(".modal button.primary").click();
      await small.waitForURL(`${origin}/r/notes/`);
      assert.equal(exists("From the phone.md"), false);
    } finally {
      await phone.close();
    }
  });
});
