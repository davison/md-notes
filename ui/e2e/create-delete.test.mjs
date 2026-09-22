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
import { TAP_TARGET, dialogReady, gate, loadPlaywright, missingPrerequisite, startFixture, waitFor } from "./harness.mjs";

const playwright = loadPlaywright();
const blocker = gate(missingPrerequisite(playwright));

describe("creating and deleting a note in the browser", { skip: blocker ?? false }, () => {
  let fixture, origin, browser, context, page;

  const notePath = (p) => fixture.file(p);
  const exists = (p) => fs.existsSync(notePath(p));

  /** The dialog on screen, and the two ways out of it. */
  const modal = () => page.locator(".modal");
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
    await dialogReady(page);
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
    await dialogReady(page);
    assert.match(await modal().textContent(), /docs\/guide\.md/, "the confirmation names the file");
    await page.locator(".modal button", { hasText: "Cancel" }).click();
    await waitFor(() => modal().count().then((n) => n === 0), "the dialog to close");
    assert.equal(exists("docs/guide.md"), true, "cancelling removed nothing");

    await page.locator(".note-bar .delete-note").click();
    await dialogReady(page);
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
      // `name` is this suite's label for the layout, not one of Playwright's
      // context options; spread in whole it was silently ignored (#91).
      const { name, ...options } = layout;
      const ctx = await browser.newContext(options);
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

        assert.deepEqual(editing, unedited, `${name}: the button moved on entering the editor`);
        assert.deepEqual(afterwards, unedited, `${name}: the button moved on leaving the editor`);
        // And it is where it is meant to be: hard right, inside the bar.
        const bar = await view.locator(".note-bar").boundingBox();
        assert.ok(
          unedited.x + unedited.width > bar.x + bar.width - 40,
          `${name}: the button is not at the right-hand end (${unedited.x + unedited.width} of ${bar.x + bar.width})`,
        );
      } finally {
        await ctx.close();
      }
    }
  });

  it("never scrolls the note bar sideways under an unbreakable failure message", async () => {
    // #91: `min-width: 0` on the save status shrank its box and not its
    // text, so a message with nothing in it to break on pushed the bar out
    // to 479 px inside a 320 px pane and scrolled it sideways. The message
    // belongs to the daemon, so it is the daemon's answer that is replaced
    // here: one PUT refused with 400 characters of a single word.
    const long = "x".repeat(400);
    // A context per width rather than one page resized: the draft that
    // never lands is mirrored to localStorage, and a second visit under
    // the same storage would open in the editor on the recovered draft
    // rather than in the rendered view this starts from.
    for (const width of [320, 1280]) {
      const ctx = await browser.newContext({ viewport: { width, height: 900 } });
      try {
        const view = await ctx.newPage();
        await view.route("**/api/r/notes/source/**", (route) =>
          route.request().method() === "PUT"
            ? route.fulfill({
                status: 500,
                contentType: "application/json",
                body: JSON.stringify({ code: "io_error", error: long }),
              })
            : route.continue(),
        );

        await view.goto(`${origin}/r/notes/index.md`);
        await view.locator(".note-bar").waitFor();
        await view.getByRole("button", { name: "Edit" }).click();
        await view.locator(".cm-editor").waitFor();
        await view.locator(".save-status").waitFor();
        const del = view.locator(".note-bar .delete-note");
        const quiet = await del.boundingBox();

        await view.locator(".cm-content").click();
        await view.keyboard.press("i");
        await view.waitForFunction(() => !document.querySelector(".cm-scroller").classList.contains("cm-vimMode"));
        await view.keyboard.type("one word the daemon will not take");
        await view.locator(".save-status.error").waitFor();

        const bar = await view.evaluate(() => {
          const el = document.querySelector(".note-bar");
          return { scroll: el.scrollWidth, client: el.clientWidth, box: el.getBoundingClientRect().width };
        });
        assert.equal(
          bar.scroll,
          bar.client,
          `${width} px: the note bar scrolls sideways (${bar.scroll} inside ${bar.client})`,
        );
        assert.ok(bar.box <= width, `${width} px: the bar itself is ${bar.box}px wide`);
        // The message is cut rather than the bar widened, the whole of it
        // is still there to be read, and Retry is not what got cut off.
        assert.equal(
          await view.locator(".save-message").getAttribute("title"),
          `Save failed: ${long}. Draft kept.`,
          `${width} px: the full message is not in the title`,
        );
        const barBox = await view.locator(".note-bar").boundingBox();
        const retry = await view.getByRole("button", { name: "Retry" }).boundingBox();
        assert.ok(
          retry.x >= barBox.x - 0.5 && retry.x + retry.width <= barBox.x + barBox.width + 0.5,
          `${width} px: Retry is outside the bar (${retry.x}..${retry.x + retry.width} of ${barBox.width})`,
        );
        // And the delete button is where it was before the message arrived.
        assert.deepEqual(await del.boundingBox(), quiet, `${width} px: the delete button moved under the message`);
      } finally {
        await ctx.close();
      }
    }
  });

  it("recreates a note deleted under another tab's draft, from the banner", async () => {
    // The second tab's path (#92): the file goes while this tab holds a
    // draft the daemon has refused, the banner names New note, and its
    // control writes the draft back under the same name in one step.
    await page.goto(`${origin}/r/notes/`);
    await page.locator(".tree").waitFor();
    await page.getByRole("button", { name: "New note" }).click();
    await nameBox().fill("Vanishing");
    await confirmButton().click();
    await page.waitForURL(`${origin}/r/notes/Vanishing.md`);

    // The second tab is a phone, so the banner's new control is measured
    // against the 40 px floor where that rule applies. `display.test.mjs`
    // keeps `.conflict button` out of its tap-target sweep because a
    // conflict was not reachable from a browser; this one reaches it.
    const phone = await browser.newContext({ viewport: { width: 390, height: 844 }, hasTouch: true, isMobile: true });
    const other = await phone.newPage();
    try {
      // Every save from this tab is refused, so its draft is still unsaved
      // when the file goes — the deterministic form of "unsaved edits".
      await other.route("**/api/r/notes/source/**", (route) =>
        route.request().method() === "PUT"
          ? route.fulfill({
              status: 500,
              contentType: "application/json",
              body: JSON.stringify({ code: "io_error", error: "the daemon is not taking saves" }),
            })
          : route.continue(),
      );
      await other.goto(`${origin}/r/notes/Vanishing.md`);
      await other.locator(".note-bar").waitFor();
      await other.getByRole("button", { name: "Edit" }).click();
      await other.locator(".cm-editor").waitFor();
      await other.locator(".cm-content").click();
      await other.keyboard.press("i");
      await other.waitForFunction(() => !document.querySelector(".cm-scroller").classList.contains("cm-vimMode"));
      await other.keyboard.type("Draft that outlived the file");
      await other.locator(".save-status.error").waitFor();

      // The first tab deletes it out from under the second.
      await page.locator(".note-bar .delete-note").click();
      await confirmButton().click();
      await page.waitForURL(`${origin}/r/notes/`);
      assert.equal(exists("Vanishing.md"), false, "the file is gone");

      const banner = other.locator(".conflict");
      await banner.waitFor({ timeout: 15000 });
      assert.match(await banner.textContent(), /New note/, "the banner names the way back");
      assert.equal(await other.locator(".modal").count(), 0, "and raises no dialog of its own");
      const target = await other.getByRole("button", { name: "Recreate the note" }).boundingBox();
      assert.ok(target.height >= TAP_TARGET, `the recreate target is ${target.height}px tall`);

      // Cancelling the prompt keeps both the banner and the draft.
      await other.getByRole("button", { name: "Recreate the note" }).click();
      await dialogReady(other);
      assert.equal(await other.locator(".modal-name").inputValue(), "Vanishing.md", "the path is pre-filled");
      await other.locator(".modal button", { hasText: "Cancel" }).click();
      await waitFor(() => other.locator(".modal").count().then((n) => n === 0), "the prompt to close");
      assert.equal(exists("Vanishing.md"), false, "cancelling wrote nothing");
      assert.equal(await banner.count(), 1, "the banner is still up");
      assert.match(await other.locator(".cm-content").textContent(), /Draft that outlived the file/);

      // Confirming writes the draft back and ends the conflict in place.
      await other.getByRole("button", { name: "Recreate the note" }).click();
      await dialogReady(other);
      await other.locator(".modal button.primary").click();
      await waitFor(() => exists("Vanishing.md"), "the note to come back");
      assert.match(fs.readFileSync(notePath("Vanishing.md"), "utf8"), /Draft that outlived the file/);
      await waitFor(() => banner.count().then((n) => n === 0), "the banner to go");
      await other.locator(".cm-editor").waitFor();
      assert.match(await other.locator(".cm-content").textContent(), /Draft that outlived the file/);
      assert.equal(await other.locator(".save-status").textContent(), "Saved");
      assert.equal(other.url(), `${origin}/r/notes/Vanishing.md`, "and the tab is still on the note");

      // The reader was typing when the note vanished, and is typing again
      // now it is back (#112): still in insert mode, focus in the editor,
      // and the next key a character at the caret rather than a command.
      assert.equal(
        await other.evaluate(() => document.querySelector(".cm-scroller").classList.contains("cm-vimMode")),
        false,
        "the recreated note's editor is in insert mode",
      );
      await waitFor(
        () => other.evaluate(() => !!document.activeElement?.closest(".cm-content")),
        "focus to be back in the editor",
      );
      await other.keyboard.type("!");
      assert.match(await other.locator(".cm-content").textContent(), /Draft that outlived the file!/);
    } finally {
      await phone.close();
    }
  });

  it("tells a clean editor its note is gone, without a conflict, and carries on when it comes back", async () => {
    // #30: nothing typed, the file deleted on disk. Measured against a real
    // daemon and its change stream, because the unit suite's jsdom has been
    // blind to real-browser timing before.
    const rel = "Ghost.md";
    fs.writeFileSync(notePath(rel), "# Ghost\n\nFirst text.\n");
    const writes = [];
    const onRequest = (r) => {
      if (r.url().includes("/api/r/notes/source/") && r.method() !== "GET") writes.push(`${r.method()} ${r.url()}`);
    };
    page.on("request", onRequest);
    try {
      await page.goto(`${origin}/r/notes/${rel}`);
      await page.locator(".note-bar").waitFor();
      await page.getByRole("button", { name: "Edit" }).click();
      await page.locator(".cm-editor").waitFor();
      await waitFor(() => page.locator(".save-status").textContent().then((t) => t === "Saved"), "the editor to be clean");

      fs.rmSync(notePath(rel));
      await waitFor(
        () => page.locator(".save-status").textContent().then((t) => t === "Deleted on disk"),
        "the bar to say the note is gone",
      );
      assert.equal(await page.locator('.conflict[role="alert"]').count(), 0, "no conflict banner");
      assert.match(await page.locator(".gone-notice").textContent(), /no longer exists on disk/);
      assert.match(await page.locator(".cm-content").textContent(), /First text\./, "the text is still on screen");
      assert.doesNotMatch(await page.title(), /^[⚠•]/, "and the tab carries no unsaved marker");

      // View mode: the file as it is on disk, under the same bar.
      await page.getByRole("button", { name: "View" }).click();
      await page.locator(".note-body").waitFor();
      assert.equal(await page.locator(".save-status").textContent(), "Deleted on disk");
      assert.equal(await page.getByText("Conflict: draft kept").count(), 0);
      assert.equal(await page.locator(".gone-notice").count(), 1);
      await page.getByRole("button", { name: "Edit" }).click();
      await page.locator(".cm-editor").waitFor();

      // Back with other text: taken, with nothing to dismiss.
      fs.writeFileSync(notePath(rel), "# Ghost\n\nSecond text.\n");
      await waitFor(
        () => page.locator(".cm-content").textContent().then((t) => t.includes("Second text.")),
        "the editor to take the returning file",
      );
      await waitFor(() => page.locator(".save-status").textContent().then((t) => t === "Saved"), "the bar to say Saved");
      assert.equal(await page.locator(".gone-notice").count(), 0, "the notice has gone by itself");
      assert.equal(await page.locator(".conflict").count(), 0);

      // Gone again, and back with the very same bytes: still just the file.
      fs.rmSync(notePath(rel));
      await waitFor(
        () => page.locator(".save-status").textContent().then((t) => t === "Deleted on disk"),
        "the bar to say the note is gone again",
      );
      fs.writeFileSync(notePath(rel), "# Ghost\n\nSecond text.\n");
      await waitFor(() => page.locator(".save-status").textContent().then((t) => t === "Saved"), "the bar to say Saved again");
      assert.equal(await page.locator(".gone-notice").count(), 0);

      assert.deepEqual(writes, [], "the tab wrote nothing to the note at any point");
      assert.equal(fs.readFileSync(notePath(rel), "utf8"), "# Ghost\n\nSecond text.\n");
    } finally {
      page.off("request", onRequest);
      fs.rmSync(notePath(rel), { force: true });
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
    // call. That is the decision on #78; this case is the behaviour it kept
    // rather than the rule it retired. The rule itself is held by the unit
    // test on #83 and, since #101, by the case below.
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
    await dialogReady(page);
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

  it("lets no document-level listener see Escape while the prompt is open", async () => {
    // The premise the decision on #78 rests on, asserted rather than
    // remembered (davison/md-notes#89). That decision retired the browser
    // case for `stopPropagation` because the two layers the call exists to
    // outrank — the drawer and the settings panel — cannot be under a dialog
    // any more, and left a standing condition with no owner: rewrite the
    // case if a document-level Escape handler is ever put back underneath
    // one. This is that owner, and it needs no stacking a reader could not
    // reach: the note pane already registers a capture-phase `keydown`
    // listener on the document for Ctrl+E (ui/src/note-pane.tsx), which is
    // live under the prompt and sees every key the document is given.
    //
    // So the question is asked of the application as it ships: which
    // document- and window-level keydown listeners are *invoked* when
    // Escape is pressed? With the prompt open the answer must be the
    // dialog's window listener and nothing else. Remove
    // `e.stopPropagation()` from ui/src/dialog.tsx and the note pane's
    // listener is invoked too, and this fails.
    const probed = await browser.newContext();
    try {
      const p = await probed.newPage();
      // Wraps every keydown listener the page registers on `window` or
      // `document`, recording which target's listener actually ran. The
      // wrapper delegates with the `this` real dispatch would have given the
      // listener — the target it was registered on — and books each wrapper
      // under its phase, so the same function registered for capture and for
      // bubble keeps a removable wrapper apiece. `removeEventListener`
      // unwraps, so a listener that is taken off really is taken off.
      //
      // A listener registered as an object with `handleEvent` is left alone
      // and is invisible here. That under-reports rather than over-reports:
      // it can only hide a listener from the assertion below, never invent
      // one, so the control — which requires a document-level listener to be
      // seen — is what would fail first if the application ever moved to that
      // form.
      await p.addInitScript(() => {
        window.__escapeSeen = [];
        const phaseOf = (options) => (options === true || (options && options.capture) ? "capture" : "bubble");
        for (const [name, target] of [
          ["window", window],
          ["document", document],
        ]) {
          const add = EventTarget.prototype.addEventListener.bind(target);
          const remove = EventTarget.prototype.removeEventListener.bind(target);
          const wrapped = { capture: new Map(), bubble: new Map() };
          target.addEventListener = function (type, fn, options) {
            if (type !== "keydown" || typeof fn !== "function") return add(type, fn, options);
            const seen = (e) => {
              if (e.key === "Escape") window.__escapeSeen.push(name);
              return fn.call(target, e);
            };
            wrapped[phaseOf(options)].set(fn, seen);
            return add(type, seen, options);
          };
          target.removeEventListener = function (type, fn, options) {
            const book = wrapped[phaseOf(options)];
            const seen = type === "keydown" && book.get(fn);
            if (!seen) return remove(type, fn, options);
            book.delete(fn);
            return remove(type, seen, options);
          };
        }
      });

      await p.goto(`${origin}/r/notes/index.md`);
      await p.locator(".note-bar").waitFor();

      // The control, first: with no dialog open the key does reach a
      // document-level listener. Without this the assertion below would
      // pass just as well against a probe that records nothing.
      await p.evaluate(() => (window.__escapeSeen.length = 0));
      await p.keyboard.press("Escape");
      assert.deepEqual(
        await p.evaluate(() => window.__escapeSeen),
        ["document"],
        "with no dialog open, Escape reaches the note pane's document-level listener",
      );

      await p.getByRole("button", { name: "New note" }).click();
      await dialogReady(p);
      await p.evaluate(() => (window.__escapeSeen.length = 0));
      await p.keyboard.press("Escape");
      await waitFor(() => p.locator(".modal").count().then((n) => n === 0), "the prompt to close");
      assert.deepEqual(
        await p.evaluate(() => window.__escapeSeen),
        ["window"],
        "with the prompt open, only the dialog's own window listener sees Escape",
      );
    } finally {
      await probed.close();
    }
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
