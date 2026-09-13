/**
 * End-to-end check of the clipper, against the built daemon and the built
 * extension loaded into headless Chromium.
 *
 * Like the file-URL suite beside it, this is not part of `make check`: it
 * needs a Chromium binary and a built `mdn`, neither of which CI installs.
 *
 *     make build extension
 *     PLAYWRIGHT_ROOT=/path/to/a/playwright/install pnpm --dir extension e2e
 *
 * The clip is driven through the real popup document, opened as a tab with
 * `?tab=` naming the page to clip — a browser action popup cannot be clicked
 * from a test, and everything behind the click is the same code either way.
 */

import { after, before, describe, it } from "node:test";
import assert from "node:assert/strict";
import { createRequire } from "node:module";
import { execFileSync, spawn } from "node:child_process";
import { createServer } from "node:net";
import { createServer as createHttpServer } from "node:http";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import crypto from "node:crypto";
import { fileURLToPath } from "node:url";

const here = path.dirname(fileURLToPath(import.meta.url));
const extensionDir = path.resolve(here, "..");
const repoRoot = path.resolve(extensionDir, "..");
const dist = path.join(extensionDir, "dist");
const mdnBin = process.env.MDN_BIN ?? path.join(repoRoot, "mdn");

function loadPlaywright() {
  const roots = [process.env.PLAYWRIGHT_ROOT, extensionDir, repoRoot].filter(Boolean);
  for (const root of roots) {
    try {
      const require = createRequire(path.join(root, "noop.js"));
      return require("playwright");
    } catch {
      // try the next place
    }
  }
  return null;
}

function missingPrerequisite(playwright) {
  if (playwright === null) return "playwright is not installed (set PLAYWRIGHT_ROOT)";
  if (!fs.existsSync(path.join(dist, "manifest.json"))) return "extension/dist is not built";
  if (!fs.existsSync(path.join(dist, "clip-inject.js"))) return "extension/dist has no clip-inject.js";
  if (!fs.existsSync(mdnBin)) return `no mdn binary at ${mdnBin} (set MDN_BIN)`;
  return null;
}

/** Chromium's id for an unpacked extension: sha256 of its path, hex mapped a-p. */
function unpackedExtensionId(dir) {
  return crypto
    .createHash("sha256")
    .update(dir)
    .digest("hex")
    .slice(0, 32)
    .replace(/[0-9a-f]/g, (c) => String.fromCharCode(97 + parseInt(c, 16)));
}

function freePort() {
  return new Promise((resolve, reject) => {
    const server = createServer();
    server.on("error", reject);
    server.listen(0, "127.0.0.1", () => {
      const { port } = server.address();
      server.close(() => resolve(port));
    });
  });
}

async function waitFor(predicate, what, timeoutMs = 15000) {
  const deadline = Date.now() + timeoutMs;
  for (;;) {
    const value = await predicate();
    if (value) return value;
    if (Date.now() > deadline) throw new Error(`timed out waiting for ${what}`);
    await new Promise((r) => setTimeout(r, 100));
  }
}

/**
 * A page with everything the conversion has to keep, plus the furniture
 * Readability has to throw away.
 */
const ARTICLE = `<!doctype html>
<html lang="en">
<head><meta charset="utf-8"><title>The Cost of Abstraction — Example Blog</title></head>
<body>
  <nav><a href="/">Navigation that is not the article</a></nav>
  <article>
    <h1>The Cost of Abstraction</h1>
    <p>An <em>early</em> cost is <strong>indirection</strong>: every layer you add is a
      layer someone has to walk back down when the behaviour is not what the
      names promised. This paragraph exists mainly so that the extractor has
      enough prose in front of it to believe this page is an article at all,
      which is the thing being tested here.</p>
    <h2>Three kinds</h2>
    <ul>
      <li>Leaky
        <ul><li>at the seams</li><li>under load</li></ul>
      </li>
      <li>Expensive, in the sense that a reader pays for it every time.</li>
    </ul>
    <pre><code class="language-go">func main() {
\tprintln("hi")
}</code></pre>
    <table>
      <thead><tr><th>Layer</th><th>Cost</th></tr></thead>
      <tbody><tr><td>One</td><td>Low</td></tr><tr><td>Two</td><td>High</td></tr></tbody>
    </table>
    <p id="pick">A <a href="/other/post">relative link</a>, an image
      <img src="diagram.png" alt="A diagram">, and some ordinary words to select.</p>
    <p>A closing paragraph, so the article is comfortably long enough to be one
      and the extractor is not deciding on a knife edge. Abstraction is not
      free, and the bill arrives later than the benefit.</p>
  </article>
  <footer>Footer that is not the article</footer>
</body>
</html>`;

/**
 * The same article behind a `<base href>`, which is what relative URLs in the
 * markup resolve against — not the address the page was served from.
 */
const BASED_ARTICLE = ARTICLE.replace(
  '<head><meta charset="utf-8">',
  '<head><meta charset="utf-8"><base href="/deep/nested/">',
  // A root-relative href ignores a base; only a document-relative one shows
  // which URL the conversion resolved against.
).replace('href="/other/post"', 'href="rel/link"');

/**
 * Too little prose for Readability to call it an article, so the clipper
 * falls back to the whole body — the third path a `<base href>` has to reach.
 */
const THIN_PAGE = `<!doctype html>
<html lang="en">
<head><meta charset="utf-8"><base href="/deep/nested/"><title>A thin page</title></head>
<body>
  <nav>Navigation that is not an article</nav>
  <p>A <a href="rel/link">relative link</a>.</p>
</body>
</html>`;

const PAGES = {
  "/posts/abstraction/": ARTICLE,
  "/base/": BASED_ARTICLE,
  "/thin/": THIN_PAGE,
};

const playwright = loadPlaywright();
const blocker = missingPrerequisite(playwright);

describe("clipping a page and a selection", { skip: blocker ?? false }, () => {
  let tmp, notesDir, clipsDir, tokenFile, token, port, daemon, context, extensionId;
  let appOrigin, site, siteOrigin, articleUrl, basedUrl, thinUrl;

  const startDaemon = async () => {
    daemon = spawn(
      mdnBin,
      [
        "serve",
        "--root", notesDir,
        "--port", String(port),
        "--state", path.join(tmp, "state.json"),
        "--token-file", tokenFile,
      ],
      { stdio: ["ignore", "pipe", "pipe"] },
    );
    await waitFor(
      () => fetch(`${appOrigin}/api/roots`).then(() => true).catch(() => false),
      "the daemon to listen",
    );
  };

  const stopDaemon = async () => {
    daemon?.kill("SIGTERM");
    await waitFor(
      () => fetch(`${appOrigin}/api/roots`).then(() => false).catch(() => true),
      "the daemon to stop",
    );
  };

  /** The settings the extension is configured with for the next clip. */
  const configure = (worker, settings) =>
    worker.evaluate((s) => chrome.storage.local.set(s), settings);

  before(async () => {
    tmp = fs.realpathSync(fs.mkdtempSync(path.join(os.tmpdir(), "mdn-clip-e2e-")));
    notesDir = path.join(tmp, "notes");
    clipsDir = path.join(notesDir, "clips");
    fs.mkdirSync(notesDir, { recursive: true });
    fs.writeFileSync(path.join(notesDir, "index.md"), "# Notes\n");

    tokenFile = path.join(tmp, "token");
    token = execFileSync(mdnBin, ["token", "--token-file", tokenFile], { encoding: "utf8" }).trim();
    assert.match(token, /^\S+$/, "mdn token printed a token");

    port = await freePort();
    appOrigin = `http://localhost:${port}`;
    await startDaemon();

    site = createHttpServer((req, res) => {
      const { pathname } = new URL(req.url, "http://fixture.invalid");
      res.setHeader("content-type", "text/html; charset=utf-8");
      res.end(PAGES[pathname] ?? ARTICLE);
    });
    await new Promise((r) => site.listen(0, "127.0.0.1", r));
    siteOrigin = `http://127.0.0.1:${site.address().port}`;
    articleUrl = `${siteOrigin}/posts/abstraction/`;
    basedUrl = `${siteOrigin}/base/`;
    thinUrl = `${siteOrigin}/thin/`;

    // A copy of dist/, so the shipped manifest keeps its narrow host
    // permission while the test grants the two ephemeral ports it needs. The
    // page's origin stands in for the `activeTab` grant a real click makes,
    // which is the browser's mechanism rather than this extension's.
    const extDir = path.join(tmp, "ext");
    fs.cpSync(dist, extDir, { recursive: true });
    const manifestPath = path.join(extDir, "manifest.json");
    const manifest = JSON.parse(fs.readFileSync(manifestPath, "utf8"));
    manifest.host_permissions = [
      `http://localhost:${port}/*`,
      `http://127.0.0.1:${port}/*`,
      `${siteOrigin}/*`,
    ];
    fs.writeFileSync(manifestPath, JSON.stringify(manifest, null, 2));
    assert.ok(manifest.permissions.includes("scripting"), "the built manifest asks for scripting");

    extensionId = unpackedExtensionId(extDir);
    context = await playwright.chromium.launchPersistentContext(path.join(tmp, "profile"), {
      headless: true,
      // Extensions need the full browser, not the headless shell.
      channel: "chromium",
      args: [`--disable-extensions-except=${extDir}`, `--load-extension=${extDir}`],
    });
    const worker =
      context.serviceWorkers()[0] ??
      (await context.waitForEvent("serviceworker", { timeout: 20000 }));
    assert.equal(new URL(worker.url()).host, extensionId);
    await configure(worker, { daemonUrl: appOrigin, token });
  });

  after(async () => {
    await context?.close();
    site?.close();
    daemon?.kill("SIGTERM");
    if (tmp !== undefined) fs.rmSync(tmp, { recursive: true, force: true });
  });

  const worker = () => context.serviceWorkers()[0];

  /** The clips in the notes root, newest name last. */
  const clipFiles = () => (fs.existsSync(clipsDir) ? fs.readdirSync(clipsDir).sort() : []);

  /** The text of the one clip whose name contains `slug`. */
  const noteText = (slug) => {
    const name = clipFiles().find((f) => f.includes(slug));
    assert.ok(name !== undefined, `no clip named ${slug} in ${clipFiles().join(", ")}`);
    return fs.readFileSync(path.join(clipsDir, name), "utf8");
  };

  /** The body of that clip. */
  const readClip = (slug) => splitNote(noteText(slug))[1];

  /** Opens the popup as a tab, pointed at the tab it should clip. */
  async function openPopupFor(pageTab) {
    const id = await waitFor(() => tabIdShowing(pageTab.url()), "the page's tab id");
    const popup = await context.newPage();
    await popup.goto(`chrome-extension://${extensionId}/popup.html?tab=${id}`);
    // The popup is ready when it has read its settings; clicking before that
    // is a race the test should not be running.
    await popup.waitForFunction(() => document.getElementById("daemon").textContent !== "");
    return popup;
  }

  function tabIdShowing(url) {
    return worker().evaluate(async (u) => {
      const tabs = await chrome.tabs.query({});
      const tab = tabs.find((t) => t.url === u);
      return tab === undefined ? null : tab.id;
    }, url);
  }

  it("clips the readable article, not the furniture around it", async () => {
    const page = await context.newPage();
    await page.goto(articleUrl);
    const popup = await openPopupFor(page);

    await popup.click("#clip-page");
    await popup.waitForSelector("#clip:not([hidden])");
    assert.equal(await popup.textContent("#clip-kind"), "Page clip");
    assert.equal(await popup.textContent("#clip-source"), articleUrl);
    const offered = await popup.inputValue("#clip-title");
    assert.match(offered, /The Cost of Abstraction/);

    // The title is editable before saving.
    await popup.fill("#clip-title", "Abstraction, clipped whole");
    await popup.click("#clip-save");
    await popup.waitForSelector("#clip-status.ok");

    const files = clipFiles();
    assert.equal(files.length, 1, `one clip, got ${files.join(", ")}`);
    const name = files[0];
    assert.match(name, /^\d{4}-\d{2}-\d{2}-abstraction-clipped-whole\.md$/);
    assert.match(await popup.textContent("#clip-status"), new RegExp(`Saved to clips/${name}`));

    const note = fs.readFileSync(path.join(clipsDir, name), "utf8");
    const [frontmatter, body] = splitNote(note);
    assert.match(frontmatter, /^title: Abstraction, clipped whole$/m);
    assert.match(frontmatter, new RegExp(`^source: ${articleUrl}$`, "m"));
    assert.match(frontmatter, /^clipped: "\d{4}-\d{2}-\d{2}T/m);
    assert.match(frontmatter, /^tags: \[clip\]$/m);

    assert.match(body, /^## Three kinds$/m);
    assert.match(body, /-\s+Leaky/);
    assert.match(body, /\n {4}-\s+at the seams/);
    assert.ok(body.includes('```go\nfunc main() {\n\tprintln("hi")\n}\n```'), body);
    assert.match(body, /\| Layer \| Cost \|/);
    assert.match(body, /\| One \| Low \|/);
    assert.match(body, /\[relative link\]\(http:\/\/127\.0\.0\.1:\d+\/other\/post\)/);
    assert.match(body, /!\[A diagram\]\(http:\/\/127\.0\.0\.1:\d+\/posts\/abstraction\/diagram\.png\)/);
    assert.match(body, /_early_/);
    assert.match(body, /\*\*indirection\*\*/);
    assert.ok(body.endsWith("\n"), "the note ends with a newline");
    // Readability's work: the page's furniture is not in the note.
    assert.ok(!body.includes("Navigation that is not the article"), body);
    assert.ok(!body.includes("Footer that is not the article"), body);

    // The success link opens the note in the app.
    const href = await popup.getAttribute("#clip-open", "href");
    assert.equal(href, `${appOrigin}/r/notes/clips/${name}`);
    const opened = await context.newPage();
    await opened.goto(href);
    await opened.waitForSelector("text=Three kinds", { timeout: 15000 });
    await opened.close();
    await popup.close();
    await page.close();
  });

  it("clips only what is selected", async () => {
    const page = await context.newPage();
    await page.goto(articleUrl);
    await page.evaluate(() => {
      const range = document.createRange();
      range.selectNodeContents(document.getElementById("pick"));
      const selection = window.getSelection();
      selection.removeAllRanges();
      selection.addRange(range);
    });
    const popup = await openPopupFor(page);

    await popup.click("#clip-selection");
    await popup.waitForSelector("#clip:not([hidden])");
    assert.equal(await popup.textContent("#clip-kind"), "Selection clip");
    await popup.fill("#clip-title", "Just the selection");
    await popup.click("#clip-save");
    await popup.waitForSelector("#clip-status.ok");

    const name = clipFiles().find((f) => f.includes("just-the-selection"));
    assert.ok(name !== undefined, `no selection clip in ${clipFiles().join(", ")}`);
    const [frontmatter, body] = splitNote(fs.readFileSync(path.join(clipsDir, name), "utf8"));
    assert.match(frontmatter, /^title: Just the selection$/m);
    assert.match(body, /\[relative link\]\(http:\/\/127\.0\.0\.1:\d+\/other\/post\)/);
    assert.match(body, /some ordinary words to select/);
    // Nothing from outside the selection came with it.
    assert.ok(!body.includes("The Cost of Abstraction"), body);
    assert.ok(!body.includes("Three kinds"), body);
    await popup.close();
    await page.close();
  });

  it("resolves relative URLs against a base href, in all three paths", async () => {
    // A `<base href>` is what the markup's relative URLs resolve against; the
    // page's own address is not. Readability rewrites them itself, so a page
    // clip came out right by accident — a selection clip and the body
    // fallback did not, and an absolute URL that points at a page which does
    // not exist is worse than a relative one, because it looks right.
    const based = `${siteOrigin}/deep/nested/rel/link`;

    const page = await context.newPage();
    await page.goto(basedUrl);
    let popup = await openPopupFor(page);
    await popup.click("#clip-page");
    await popup.waitForSelector("#clip:not([hidden])");
    await popup.fill("#clip-title", "Based page");
    await popup.click("#clip-save");
    await popup.waitForSelector("#clip-status.ok");
    let body = readClip("based-page");
    assert.ok(body.includes(`[relative link](${based})`), body);
    await popup.close();

    // and the source is still where the clip came from, not the base
    const [frontmatter] = splitNote(noteText("based-page"));
    assert.match(frontmatter, new RegExp(`^source: ${basedUrl}$`, "m"));

    await page.evaluate(() => {
      const range = document.createRange();
      range.selectNodeContents(document.getElementById("pick"));
      const selection = window.getSelection();
      selection.removeAllRanges();
      selection.addRange(range);
    });
    popup = await openPopupFor(page);
    await popup.click("#clip-selection");
    await popup.waitForSelector("#clip:not([hidden])");
    await popup.fill("#clip-title", "Based selection");
    await popup.click("#clip-save");
    await popup.waitForSelector("#clip-status.ok");
    body = readClip("based-selection");
    assert.ok(body.includes(`[relative link](${based})`), body);
    assert.ok(body.includes(`![A diagram](${siteOrigin}/deep/nested/diagram.png)`), body);
    await popup.close();
    await page.close();

    // and the fallback, on a page too thin for Readability to accept
    const thin = await context.newPage();
    await thin.goto(thinUrl);
    popup = await openPopupFor(thin);
    await popup.click("#clip-page");
    await popup.waitForSelector("#clip:not([hidden])");
    await popup.fill("#clip-title", "Based fallback");
    await popup.click("#clip-save");
    await popup.waitForSelector("#clip-status.ok");
    body = readClip("based-fallback");
    assert.ok(body.includes(`[relative link](${based})`), body);
    // the whole body, furniture included: this really is the fallback
    assert.match(body, /Navigation that is not an article/);
    await popup.close();
    await thin.close();
  });

  it("says nothing is selected rather than clipping the page instead", async () => {
    const page = await context.newPage();
    await page.goto(articleUrl);
    const popup = await openPopupFor(page);
    await popup.click("#clip-selection");
    await popup.waitForSelector("#clip-status.error");
    assert.match(await popup.textContent("#clip-status"), /Nothing is selected/i);
    await popup.close();
    await page.close();
  });

  it("reports a rejected token, and keeps the clip for a second try", async () => {
    const before = clipFiles().length;
    await configure(worker(), { token: "not-the-token" });
    const page = await context.newPage();
    await page.goto(articleUrl);
    const popup = await openPopupFor(page);

    await popup.click("#clip-page");
    await popup.waitForSelector("#clip:not([hidden])");
    await popup.fill("#clip-title", "Rejected token");
    await popup.click("#clip-save");
    await popup.waitForSelector("#clip-status.error");
    const message = await popup.textContent("#clip-status");
    assert.match(message, /Token rejected/);
    assert.match(message, /mdn token/);
    assert.equal(await popup.isVisible("#clip-options"), true, "the way to fix it is offered");
    assert.equal(clipFiles().length, before, "nothing was written");

    // The clip is still there: paste the right token and press Save again.
    assert.equal(await popup.isVisible("#clip-title"), true);
    await configure(worker(), { token });
    await popup.click("#clip-save");
    await popup.waitForSelector("#clip-status.ok");
    assert.equal(clipFiles().length, before + 1);
    await popup.close();
    await page.close();
  });

  it("says no token is configured when none is", async () => {
    await configure(worker(), { token: "" });
    const page = await context.newPage();
    await page.goto(articleUrl);
    const popup = await openPopupFor(page);
    await popup.click("#clip-page");
    await popup.waitForSelector("#clip:not([hidden])");
    await popup.click("#clip-save");
    await popup.waitForSelector("#clip-status.error");
    assert.match(await popup.textContent("#clip-status"), /No token configured/);
    assert.equal(await popup.isVisible("#clip-options"), true);
    await popup.click("#clip-discard");
    await configure(worker(), { token });
    await popup.close();
    await page.close();
  });

  it("says the daemon is not reachable when it is down", async () => {
    await stopDaemon();
    const page = await context.newPage();
    await page.goto(articleUrl);
    const popup = await openPopupFor(page);
    await popup.click("#clip-page");
    await popup.waitForSelector("#clip:not([hidden])");
    await popup.fill("#clip-title", "Daemon down");
    await popup.click("#clip-save");
    await popup.waitForSelector("#clip-status.error");
    const message = await popup.textContent("#clip-status");
    assert.match(message, /not reachable at/);
    assert.match(message, new RegExp(appOrigin.replace(/[.*+?^${}()|[\]\\]/g, "\\$&")));
    await popup.close();
    await page.close();
    await startDaemon();
  });
});

/** A clipped note split into its frontmatter and its body. */
function splitNote(text) {
  const match = /^---\n([\s\S]*?)\n---\n\n([\s\S]*)$/.exec(text);
  assert.ok(match !== null, `note has no frontmatter:\n${text}`);
  return [match[1], match[2]];
}
