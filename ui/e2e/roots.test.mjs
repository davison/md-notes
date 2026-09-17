/**
 * End-to-end checks of unregistering a root from the home page (M7-R2,
 * adopting davison/md-notes#50), against the built daemon in headless
 * Chromium, on the rig the rest of ui/e2e shares.
 *
 *     make e2e
 *     pnpm --dir ui e2e
 *
 * `make build` first if you run the suite by hand: the daemon serves the
 * `ui/dist` embedded in the binary.
 *
 * The daemon under test gets a temporary root, its own config, state and
 * token files, and an ephemeral port, so nothing here touches a daemon you
 * are running or the state under ~/.local/state/mdn. See ./harness.mjs.
 */

import { after, before, describe, it } from "node:test";
import assert from "node:assert/strict";
import fs from "node:fs";
import path from "node:path";
import {
  COARSE_DESKTOP,
  TAP_TARGET,
  dialogReady,
  loadPlaywright,
  missingPrerequisite,
  rect,
  startFixture,
  waitFor,
} from "./harness.mjs";

const playwright = loadPlaywright();
const blocker = missingPrerequisite(playwright);
if (blocker) console.log(`# skipped: ${blocker}`);

describe("unregistering a root in the browser", { skip: blocker ?? false }, () => {
  let fixture, origin, browser, context, page, scratch;

  /** Registers a folder the way `mdn open` does: a loopback POST, no token. */
  const register = async (dir) => {
    const res = await fetch(`${origin}/api/roots`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ path: dir }),
    });
    // One read of the body: it is the answer on success and the complaint
    // on failure, and a Response can only be read once.
    const body = await res.text();
    assert.equal(res.status, 200, body);
    return JSON.parse(body);
  };

  const stateFile = () => {
    const file = path.join(fixture.tmp, "state.json");
    return fs.existsSync(file) ? fs.readFileSync(file, "utf8") : "";
  };

  before(async () => {
    fixture = await startFixture("roots");
    origin = fixture.origin;
    scratch = path.join(fixture.tmp, "scratch");
    fs.mkdirSync(scratch, { recursive: true });
    fs.writeFileSync(path.join(scratch, "todo.md"), "# Todo\n\nA note in a folder added by hand.\n");
    browser = await playwright.chromium.launch();
    context = await browser.newContext(COARSE_DESKTOP);
    page = await context.newPage();
  });

  after(async () => {
    await context?.close();
    await browser?.close();
    await fixture?.stop();
  });

  it("offers a remove control on a recent root, sized for a finger", async () => {
    await register(scratch);
    await page.goto(`${origin}/`);
    await page.waitForSelector(".roots");

    const control = page.locator('[aria-label="Remove scratch"]');
    await control.waitFor();
    // The notes root is the daemon's configuration and has no control.
    assert.equal(await page.locator('[aria-label="Remove notes"]').count(), 0);
    const box = await rect(page, '[aria-label="Remove scratch"]');
    assert.ok(
      box.height >= TAP_TARGET,
      `the remove control is ${box.height} CSS px tall, below the ${TAP_TARGET} px floor`,
    );
  });

  it("asks before it removes, naming the folder, and takes no for an answer", async () => {
    await page.click('[aria-label="Remove scratch"]');
    await dialogReady(page);
    const asked = await page.locator(".modal").textContent();
    assert.ok(asked.includes(scratch), `the prompt does not name the folder: ${asked}`);
    assert.match(asked, /nothing is removed from disk/i);

    await page.click(".modal button:not(.primary)");
    await page.waitForSelector(".modal", { state: "detached" });
    assert.ok(stateFile().includes(scratch), "a cancelled prompt unregistered the root");
    assert.equal(await page.locator('[aria-label="Remove scratch"]').count(), 1);
  });

  it("unregisters the root on yes, and leaves every file on disk", async () => {
    await page.click('[aria-label="Remove scratch"]');
    await dialogReady(page);
    await page.click(".modal button.primary");

    await waitFor(
      () => page.locator('[aria-label="Remove scratch"]').count().then((n) => n === 0),
      "the root to leave the home page",
    );
    // The daemon agrees, on both of its records.
    const listed = await fetch(`${origin}/api/roots`).then((r) => r.json());
    assert.deepEqual(
      listed.roots.map((r) => r.slug),
      ["notes"],
    );
    assert.ok(!stateFile().includes(scratch), `the state file still holds it: ${stateFile()}`);
    // Unregistering is not deleting.
    assert.ok(fs.existsSync(path.join(scratch, "todo.md")), "the note went with the root");
  });

  it("sends a tab open on a removed root back to the home page", async () => {
    // The whole chain, which no unit test can hold: the daemon ends the
    // root's event stream, the browser's own reconnect meets the 404 the
    // unknown slug now gives, and the page asks the roots listing before
    // concluding the root has gone rather than that live update is off.
    const root = await register(scratch);
    const open = await context.newPage();
    await open.goto(`${origin}/r/${root.slug}/`);
    await open.waitForSelector(".shell");

    await page.goto(`${origin}/`);
    await page.click(`[aria-label="Remove ${root.slug}"]`);
    await dialogReady(page);
    await page.click(".modal button.primary");
    await waitFor(
      () => page.locator(`[aria-label="Remove ${root.slug}"]`).count().then((n) => n === 0),
      "the root to leave the home page",
    );

    await open.waitForURL(`${origin}/`, { timeout: 30000 });
    await open.waitForSelector(".roots");
    await open.close();
  });
});
