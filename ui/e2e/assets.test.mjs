/**
 * The caching check the asset task (#59) measured in a scratchpad harness
 * and #72 asked to be brought into CI: a second page load that fetches no
 * asset bytes at all.
 *
 * The bytes are Chromium's own, over CDP — `Network.loadingFinished`'s
 * `encodedDataLength` is what crossed the socket, compression included,
 * which is the figure #59 recorded and is not a number the page can be
 * asked for.
 *
 * Everything here is stated in bytes and in distinct URLs, never in rows,
 * because the number of rows stopped being ours to predict when the service
 * worker landed (#124). With the worker answering, Chromium asks for the
 * entry module script *twice* on some loads — two parser-initiated requests
 * from the same `<script>` tag, at two columns of the same source line: the
 * speculative preload and the module loader's own fetch, folded into one when
 * the browser's own cache answers and not folded when the worker does. Both
 * come back from the worker for **zero bytes**, so the property this file
 * exists to protect is untouched and only a count of rows notices. A
 * `deepEqual` over the raw path list noticed, and failed four full-suite runs
 * in nine on merged main while passing twenty-two in twenty-two on its own.
 *
 * Which load it lands on is a matter of how busy the machine is: idle, it is
 * the third, where nothing compared the list; under load it is the second,
 * where something did. That is the whole of the flake, and it is why the
 * second case below slows the CPU down on purpose rather than hoping. At
 * `1x` the duplicate lands on the third load every time and at `4x` and
 * above on the second, so the two cases together cover both interleavings
 * on every run instead of one of them by luck.
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

  /**
   * `cpu` is Chromium's own throttling multiplier. 1 is the machine as it is;
   * anything from 4 up is enough to make the worker answer the second load's
   * script twice, which is the interleaving a busy CI runner produces on its
   * own and the one the old assertions broke on.
   */
  for (const { how, cpu } of [
    { how: "on an idle machine", cpu: 1 },
    // Not "until the worker answers": it answers at 1x too, and the log line
    // from that case says so. What the throttling changes is whether Chromium
    // reports the module request twice, and this string is what a failure
    // prints.
    { how: "with the CPU slowed until the worker answers the second load's script twice", cpu: 8 },
  ]) {
    it(`fetches no asset bytes on a second page load, ${how}`, async () => {
      const { name, ...options } = DESKTOP;
      const context = await browser.newContext(options);
      try {
        const page = await context.newPage();
        const cdp = await context.newCDPSession(page);
        await cdp.send("Network.enable");
        if (cpu > 1) await cdp.send("Emulation.setCPUThrottlingRate", { rate: cpu });

        const responses = new Map();
        cdp.on("Network.responseReceived", (e) => {
          const header = (n) => e.response.headers[n] ?? e.response.headers[n.replace(/(^|-)(\w)/g, (m) => m.toUpperCase())];
          responses.set(e.requestId, {
            path: new URL(e.response.url).pathname,
            status: e.response.status,
            cacheControl: header("cache-control"),
            encoding: header("content-encoding"),
            // Who answered. Nothing is asserted on it — the wire cost is the
            // same either way — but a failure here should say whether the
            // browser's cache or the worker was in the way, rather than leaving
            // the next reader to instrument it as #124 had to.
            worker: e.response.fromServiceWorker === true,
            bytes: 0,
          });
        });
        cdp.on("Network.loadingFinished", (e) => {
          const r = responses.get(e.requestId);
          if (r) r.bytes = e.encodedDataLength;
        });

        /** The URLs a set of responses covers, each once, in a stable order. */
        const urls = (rows) => [...new Set(rows.map((r) => r.path))].sort();

        const drain = (label) => {
          const rows = [...responses.values()];
          responses.clear();
          const assets = rows.filter((r) => r.path.startsWith(ASSETS));
          const shell = rows.filter((r) => r.path.startsWith("/r/"));
          const total = assets.reduce((a, r) => a + r.bytes, 0);
          const shellBytes = shell.reduce((a, r) => a + r.bytes, 0);
          console.log(
            `# ${label}: ${urls(assets).length} assets over ${assets.length} responses` +
              `${assets.some((r) => r.worker) ? " (the worker answered)" : ""}, ${total} B on the wire;` +
              ` shell ${shellBytes} B over ${shell.length} response${shell.length === 1 ? "" : "s"}`,
          );
          return { assets, shell, total, shellBytes };
        };

        // A cold load: the assets cross the wire, compressed and immutable.
        // Nothing is cached yet and no worker is controlling this page — it
        // registers after `load`, which is after these requests — so every
        // asset here is a real transfer.
        await openNote(page, fixture.url("alpha.md"));
        const first = drain("first load");
        assert.ok(urls(first.assets).length >= 2, "the shell pulled its script and its stylesheet");
        assert.ok(first.total > 0, "a cold load fetches the assets");
        for (const asset of first.assets) {
          assert.equal(asset.status, 200);
          assert.equal(asset.cacheControl, "public, max-age=31536000, immutable", `${asset.path} is immutable`);
          assert.equal(asset.encoding, "br", `${asset.path} was served brotli-compressed`);
          assert.ok(asset.bytes > 0);
        }
        assert.ok(first.shell.length >= 1, "the shell itself was fetched");
        assert.equal(first.shell[0].cacheControl, "no-cache", "the shell itself is never cached blind");
        const shellBytes = first.shell[0].bytes;

        // A second page load in the same browser: the same asset URLs, and
        // nothing of them on the wire. Every response is checked, not their
        // sum — a total of zero could in principle be one response that was
        // fetched and another that was refunded, and per-response is what the
        // sentence "no asset bytes" actually means.
        await openNote(page, fixture.url("beta.md"));
        const second = drain("second load");
        for (const asset of second.assets) {
          assert.equal(asset.bytes, 0, `${asset.path} crossed the wire on a second load`);
        }
        assert.equal(second.total, 0, "a second page load fetches no asset bytes");
        assert.deepEqual(
          urls(second.assets),
          urls(first.assets),
          "and it is the same assets it did not fetch",
        );

        // The shell is `no-cache`, not `no-store`: asked for again at the same
        // URL it does not come back as a body. Without a worker that is a 304
        // against the ETag; with one the navigation is answered by the worker
        // and the network exchange it made behind that is not reported on this
        // page's session at all, so the figure is smaller still. Either way the
        // claim here is about bytes on the wire, and it is counted across
        // whatever responses the load produced rather than assuming one.
        //
        // That the shell is *fresh* rather than merely cheap is a different
        // promise and a different file: ui/e2e/pwa.test.mjs holds the worker to
        // network-first on the shell, which is what makes a rebuild arrive.
        await openNote(page, fixture.url("alpha.md"));
        const third = drain("third load, the first URL again");
        for (const asset of third.assets) {
          assert.equal(asset.bytes, 0, `${asset.path} crossed the wire on a third load`);
        }
        assert.equal(third.total, 0, "still no asset bytes");
        assert.deepEqual(urls(third.assets), urls(first.assets), "and still the same assets");
        assert.ok(third.shell.length >= 1, "the shell was asked for again");
        assert.ok(
          third.shellBytes < shellBytes / 2,
          `the shell came back in ${third.shellBytes} B against ${shellBytes} B of body`,
        );
      } finally {
        await context.close();
      }
    });
  }
});
