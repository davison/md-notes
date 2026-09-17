/**
 * The installable app (davison/md-notes#118, M7-R5): the web app manifest,
 * the service worker, and the four promises the worker makes.
 *
 * On installability this suite checks Chrome's own criteria item by item
 * rather than running Lighthouse. Lighthouse cannot answer the question any
 * more: its PWA category — which held `installable-manifest` and the
 * service-worker audits — was removed in Lighthouse 12, and nothing replaced
 * it. What is checked instead is the list Chrome documents as the conditions
 * for the install prompt, read back two ways: over HTTP, and out of the
 * browser's own parsed manifest through CDP `Page.getAppManifest`, whose
 * `errors` array is Chrome's verdict on the document rather than ours.
 */

import { after, before, describe, it } from "node:test";
import assert from "node:assert/strict";
import fs from "node:fs";
import { loadPlaywright, missingPrerequisite, startFixture, waitFor } from "./harness.mjs";

const playwright = loadPlaywright();
const blocker = missingPrerequisite(playwright);
if (blocker) console.log(`# skipped: ${blocker}`);

/** The sizes Android asks for, and the shape of the maskable variant. */
const ICONS = [
  { src: "/icon-192.png", size: 192, purpose: "any" },
  { src: "/icon-512.png", size: 512, purpose: "any" },
  { src: "/icon-maskable-512.png", size: 512, purpose: "maskable" },
];

/** A PNG's own dimensions, from the IHDR that begins every one of them. */
function pngSize(bytes) {
  const b = Buffer.from(bytes);
  assert.equal(b.subarray(1, 4).toString("ascii"), "PNG", "not a PNG");
  return { width: b.readUInt32BE(16), height: b.readUInt32BE(20) };
}

/** Resolves once the worker is active *and* has taken this page over. */
async function controlled(page) {
  await page.evaluate(() => navigator.serviceWorker.ready);
  await waitFor(
    () => page.evaluate(() => !!navigator.serviceWorker.controller),
    "the service worker to control the page",
  );
}

/** Every URL the worker has in its caches. */
function cachedURLs(page) {
  return page.evaluate(async () => {
    const urls = [];
    for (const name of await caches.keys()) {
      const cache = await caches.open(name);
      for (const request of await cache.keys()) urls.push(new URL(request.url).pathname);
    }
    return urls.sort();
  });
}

describe("the installable app", { skip: blocker ?? false }, () => {
  let fixture, browser, context, page;

  before(async () => {
    fixture = await startFixture("pwa");
    browser = await playwright.chromium.launch({ headless: true });
    // One context for the suite: a service worker belongs to a browser
    // profile, and registering it once is what the checks below are about.
    context = await browser.newContext();
    page = await context.newPage();
    await page.goto(`${fixture.origin}/`);
    await controlled(page);
  });

  after(async () => {
    await context?.close();
    await browser?.close();
    await fixture?.stop();
  });

  it("meets Chrome's installability criteria", async () => {
    // 1. A secure context. The tailnet route is HTTPS because `tailscale
    //    serve` terminates TLS; loopback is trusted without one.
    assert.equal(await page.evaluate(() => window.isSecureContext), true, "loopback is a secure context");

    // 2. The manifest, served as a manifest and never cached blind.
    const res = await fetch(`${fixture.origin}/manifest.webmanifest`);
    assert.equal(res.status, 200);
    assert.equal(res.headers.get("content-type"), "application/manifest+json");
    assert.equal(res.headers.get("cache-control"), "no-cache");

    // 3. Chrome's own reading of it: the parsed document and its complaints.
    const cdp = await context.newCDPSession(page);
    const parsed = await cdp.send("Page.getAppManifest");
    assert.ok(parsed.url.endsWith("/manifest.webmanifest"), `manifest URL ${parsed.url}`);
    assert.deepEqual(parsed.errors ?? [], [], "Chrome parsed the manifest without complaint");
    const manifest = JSON.parse(parsed.data);

    // 4. The fields the prompt requires.
    assert.equal(manifest.name, "MD Notes");
    assert.equal(manifest.short_name, "mdn");
    assert.equal(manifest.display, "standalone", "an installed window, not a browser tab");
    assert.equal(manifest.start_url, "/", "the app's home page");
    assert.equal(manifest.scope, "/", "so /r/{slug}/ routes stay inside the installed app");
    assert.match(manifest.theme_color, /^#[0-9a-f]{6}$/);
    assert.match(manifest.background_color, /^#[0-9a-f]{6}$/);

    // 5. The icons, fetched and measured rather than believed.
    for (const want of ICONS) {
      const declared = manifest.icons.find((i) => i.src.endsWith(want.src) && i.purpose === want.purpose);
      assert.ok(declared, `the manifest declares ${want.src} for ${want.purpose}`);
      assert.equal(declared.type, "image/png");
      assert.equal(declared.sizes, `${want.size}x${want.size}`);
      const icon = await fetch(`${fixture.origin}${want.src}`);
      assert.equal(icon.status, 200);
      assert.equal(icon.headers.get("content-type"), "image/png");
      const measured = pngSize(await icon.arrayBuffer());
      assert.deepEqual(measured, { width: want.size, height: want.size }, `${want.src} is square at ${want.size}`);
    }

    // 6. The page asks for the manifest with credentials, which is what makes
    //    it reachable over the tailnet at all.
    const link = await page.getAttribute('link[rel="manifest"]', "crossorigin");
    assert.equal(link, "use-credentials");

    // 7. A service worker with a fetch handler, controlling the start URL.
    const worker = await fetch(`${fixture.origin}/sw.js`);
    assert.equal(worker.headers.get("content-type"), "text/javascript; charset=utf-8");
    assert.equal(worker.headers.get("cache-control"), "no-cache", "a new build's worker is never masked");
    const state = await page.evaluate(async () => {
      const reg = await navigator.serviceWorker.ready;
      return { scope: new URL(reg.scope).pathname, state: reg.active?.state };
    });
    assert.deepEqual(state, { scope: "/", state: "activated" });
    console.log(`# installability: manifest, ${ICONS.length} icons, worker activated at scope ${state.scope}`);
  });

  it("caches the shell and the eager assets, and not the editor", async () => {
    const urls = await cachedURLs(page);
    assert.ok(urls.includes("/index.html"), "the shell is cached");
    assert.ok(urls.includes("/manifest.webmanifest"), "the manifest is cached");
    const assets = urls.filter((u) => u.startsWith("/assets/"));
    assert.ok(
      assets.some((u) => /^\/assets\/index-.*\.js$/.test(u)),
      `the entry chunk is cached: ${assets.join(", ")}`,
    );
    assert.ok(
      assets.some((u) => /^\/assets\/index-.*\.css$/.test(u)),
      "the stylesheet is cached",
    );
    assert.equal(
      assets.filter((u) => u.includes("/editor-")).length,
      0,
      "the editor chunk is not precached: it is still fetched the first time it is wanted",
    );
    console.log(`# precached: ${urls.length} files, ${assets.length} of them hashed assets`);
  });

  it("answers an API request from the network and never from a cache", async () => {
    const cdp = await context.newCDPSession(page);
    await cdp.send("Network.enable");
    const seen = [];
    cdp.on("Network.responseReceived", (e) => {
      seen.push({
        path: new URL(e.response.url).pathname,
        type: e.type,
        fromWorker: e.response.fromServiceWorker === true,
      });
    });

    await page.goto(fixture.url("alpha.md"));
    await page.waitForSelector(".note-body .markdown", { state: "attached" });
    await waitFor(
      () => seen.some((r) => r.path.endsWith("/events")),
      `the events stream to be requested: ${JSON.stringify(seen)}`,
    );

    const api = seen.filter((r) => r.path.startsWith("/api/"));
    assert.ok(api.length >= 3, `the page called the API: ${JSON.stringify(api)}`);
    for (const call of api) {
      assert.equal(call.fromWorker, false, `${call.path} was answered by the network, not the worker`);
    }
    // The control: the worker *is* in the way of everything else, so the
    // line above is a decision it took rather than a worker that is absent.
    const assets = seen.filter((r) => r.path.startsWith("/assets/"));
    assert.ok(assets.length > 0 && assets.every((r) => r.fromWorker), "the assets came from the worker");

    // Nothing under /api/ is in a cache, whatever the worker did with it.
    assert.deepEqual(
      (await cachedURLs(page)).filter((u) => u.startsWith("/api/")),
      [],
      "no API response was ever stored",
    );

    // And with the network gone, an API call fails rather than being served
    // a stale answer — which is what a save and the login depend on.
    await context.setOffline(true);
    try {
      assert.equal(
        await page.evaluate(() => fetch("/api/roots").then(() => "answered", () => "failed")),
        "failed",
      );
    } finally {
      await context.setOffline(false);
    }
  });

  it("prefers a rebuilt shell to its cached one, and falls back to it offline", async () => {
    await page.goto(`${fixture.origin}/`);
    await controlled(page);

    // A shell in the cache that is not the shell the daemon is serving: the
    // state a rebuild leaves behind, staged here because the daemon's bundle
    // is embedded in a binary this suite cannot rebuild under itself.
    const STALE = "the stale shell from an older build";
    await page.evaluate(async (stale) => {
      const name = (await caches.keys()).find((k) => k.startsWith("mdn-"));
      const cache = await caches.open(name);
      await cache.put(
        "/index.html",
        new Response(`<!doctype html><title>stale</title><body><p id="stale">${stale}</p>`, {
          headers: { "Content-Type": "text/html; charset=utf-8" },
        }),
      );
    }, STALE);

    // Online: the network wins. This is the M4 no-cache rule surviving the
    // worker — the shell is asked for every time it is opened.
    await page.reload();
    assert.equal(await page.locator("#stale").count(), 0, "the stale shell was not served");
    await page.waitForSelector("#app h1");

    // And the fresh one replaced it in the cache, so the fallback is never
    // older than the last load that succeeded.
    const cached = await page.evaluate(async () => {
      const name = (await caches.keys()).find((k) => k.startsWith("mdn-"));
      return (await (await caches.open(name)).match("/index.html", { ignoreVary: true })).text();
    });
    assert.ok(!cached.includes(STALE), "the cached shell was replaced by the one the daemon served");

    // Offline: the shell opens anyway, and the app says why it is empty
    // rather than the browser saying the site cannot be reached — on every
    // route the scope covers, not only the start URL. A note route is where
    // an installed app is most likely to be opened from: the last note read,
    // or a link followed into it.
    await page.goto(fixture.url("alpha.md"));
    await page.waitForSelector(".note-body .markdown", { state: "attached" });
    await context.setOffline(true);
    try {
      for (const [what, url] of [
        ["the roots page", `${fixture.origin}/`],
        ["a root", fixture.url("")],
        ["a note", fixture.url("alpha.md")],
        ["a note in a folder", fixture.url("docs/guide.md")],
      ]) {
        await page.goto(url);
        await page.waitForSelector("#app h1");
        const text = await page.locator("#app").textContent();
        assert.doesNotMatch(
          text,
          /Unknown root|No root named/,
          `${what} offline says the root does not exist, which is false and frightening`,
        );
        const error = await page.locator(".error").textContent();
        assert.match(error, /not answering/, `${what} offline names the daemon, not fetch`);
        console.log(`# offline ${what}: ${JSON.stringify(error)}`);
      }
      await page.goto(`${fixture.origin}/`);
      await page.waitForSelector("#app h1");

      // The last resort: offline with no cached shell at all, which is what
      // an install that never reached the network leaves behind. The worker
      // writes the page itself rather than letting the browser say the site
      // cannot be reached.
      await page.evaluate(async () => {
        const name = (await caches.keys()).find((k) => k.startsWith("mdn-"));
        await (await caches.open(name)).delete("/index.html", { ignoreVary: true });
      });
      await page.reload();
      assert.match(await page.locator("h1").textContent(), /unreachable/i);
      assert.match(await page.locator("p").first().textContent(), /mdn is not answering/);
      assert.equal(await page.locator("button").count(), 1, "and a way to try again");
    } finally {
      await context.setOffline(false);
    }
  });

  it("still delivers a change over the events stream with the worker in place", async () => {
    await page.goto(fixture.url("alpha.md"));
    await controlled(page);
    await page.waitForSelector(".note-body .markdown");

    const changed = "Edited on disk while the worker was installed.";
    fs.writeFileSync(fixture.file("alpha.md"), `---\ntags: [alpha, beta]\n---\n\n# Alpha\n\n${changed}\n`);
    await page.waitForFunction(
      (text) => document.querySelector(".note-body .markdown")?.textContent.includes(text),
      changed,
      { timeout: 20000 },
    );
  });
});
