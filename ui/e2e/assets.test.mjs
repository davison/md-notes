/**
 * The caching check the asset task (#59) measured in a scratchpad harness
 * and #72 asked to be brought into CI: a second page load that fetches no
 * asset bytes at all.
 *
 * The bytes are Chromium's own, over CDP — `Network.loadingFinished`'s
 * `encodedDataLength` is what crossed the socket, compression included,
 * which is the figure #59 recorded and is not a number the page can be
 * asked for.
 */

import { after, before, describe, it } from "node:test";
import assert from "node:assert/strict";
import { DESKTOP, loadPlaywright, missingPrerequisite, openNote, startFixture } from "./harness.mjs";

const playwright = loadPlaywright();
const blocker = missingPrerequisite(playwright);
if (blocker) console.log(`# skipped: ${blocker}`);

/** Vite's hashed output directory, which is the one served immutable. */
const ASSETS = "/assets/";

describe("the embedded assets over the wire", { skip: blocker ?? false }, () => {
  let fixture, browser;

  before(async () => {
    fixture = await startFixture("assets");
    browser = await playwright.chromium.launch({ headless: true });
  });

  after(async () => {
    await browser?.close();
    await fixture?.stop();
  });

  it("fetches no asset bytes on a second page load", async () => {
    const { name, ...options } = DESKTOP;
    const context = await browser.newContext(options);
    try {
      const page = await context.newPage();
      const cdp = await context.newCDPSession(page);
      await cdp.send("Network.enable");

      const responses = new Map();
      cdp.on("Network.responseReceived", (e) => {
        const header = (n) => e.response.headers[n] ?? e.response.headers[n.replace(/(^|-)(\w)/g, (m) => m.toUpperCase())];
        responses.set(e.requestId, {
          path: new URL(e.response.url).pathname,
          status: e.response.status,
          cacheControl: header("cache-control"),
          encoding: header("content-encoding"),
          bytes: 0,
        });
      });
      cdp.on("Network.loadingFinished", (e) => {
        const r = responses.get(e.requestId);
        if (r) r.bytes = e.encodedDataLength;
      });

      const drain = (label) => {
        const rows = [...responses.values()];
        responses.clear();
        const assets = rows.filter((r) => r.path.startsWith(ASSETS));
        const shell = rows.filter((r) => r.path.startsWith("/r/"));
        const total = assets.reduce((a, r) => a + r.bytes, 0);
        console.log(
          `# ${label}: ${assets.length} assets, ${total} B on the wire; shell ${shell.map((r) => `${r.bytes} B`).join(", ")}`,
        );
        return { assets, shell, total };
      };

      // A cold load: the assets cross the wire, compressed and immutable.
      await openNote(page, fixture.url("alpha.md"));
      const first = drain("first load");
      assert.ok(first.assets.length >= 2, "the shell pulled its script and its stylesheet");
      assert.ok(first.total > 0, "a cold load fetches the assets");
      for (const asset of first.assets) {
        assert.equal(asset.status, 200);
        assert.equal(asset.cacheControl, "public, max-age=31536000, immutable", `${asset.path} is immutable`);
        assert.equal(asset.encoding, "br", `${asset.path} was served brotli-compressed`);
        assert.ok(asset.bytes > 0);
      }
      assert.equal(first.shell.length, 1);
      assert.equal(first.shell[0].cacheControl, "no-cache", "the shell itself is never cached blind");
      const shellBytes = first.shell[0].bytes;

      // A second page load in the same browser: the same asset URLs, and
      // nothing of them on the wire.
      await openNote(page, fixture.url("beta.md"));
      const second = drain("second load");
      assert.equal(second.total, 0, "a second page load fetches no asset bytes");
      assert.deepEqual(
        second.assets.map((a) => a.path).sort(),
        first.assets.map((a) => a.path).sort(),
        "and it is the same assets it did not fetch",
      );

      // The shell is `no-cache`, not `no-store`: asked for again at the same
      // URL it revalidates and comes back without a body.
      await openNote(page, fixture.url("alpha.md"));
      const third = drain("third load, the first URL again");
      assert.equal(third.total, 0, "still no asset bytes");
      assert.equal(third.shell.length, 1);
      assert.ok(
        third.shell[0].bytes < shellBytes / 2,
        `the shell revalidated in ${third.shell[0].bytes} B against ${shellBytes} B of body`,
      );
    } finally {
      await context.close();
    }
  });
});
