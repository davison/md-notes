/**
 * End-to-end check of the extension against a daemon reached under a tailnet
 * name, which is M4-R6 and the capture it adopts, #51 — and, since M6-R1,
 * the clip that suite once watched be refused.
 *
 * The name is simulated rather than real: the daemon is started with
 * `--tailnet-host <name>:<port>` and Chromium with
 * `--host-resolver-rules=MAP <name> 127.0.0.1`, so every request the browser
 * and the extension make carries `Host: <name>:<port>` and lands on the
 * daemon's own loopback listener — exactly the path a `tailscale serve` proxy
 * puts them on, minus the TLS it terminates. That is the rig the review of
 * PR #49 used to demonstrate the bug this suite now demonstrates is fixed;
 * plain HTTP keeps it to one prerequisite fewer (no certificate, no
 * `--ignore-certificate-errors`) and changes nothing the extension decides,
 * because the daemon's tailnet rule is keyed on the Host header alone.
 *
 * One consequence of the missing TLS is worth stating: the daemon's session
 * cookie carries the `__Host-` prefix and so is only ever set over https, so
 * the tab the intercept redirects lands on the daemon's login page rather
 * than on the rendered note. That is the app's business — reading notes from
 * another device means logging in there — and not the extension's, whose job
 * ends at sending the tab to the right URL. Which is what is asserted.
 *
 * Like the two suites beside it, this is not part of `make check`, and CI
 * does not run it:
 *
 *     make build extension
 *     pnpm --dir extension e2e
 *
 * `prerequisites.mjs` says where Playwright comes from, and it skips the
 * suite with one line naming what is missing — the browser download
 * included — or fails it under CI.
 */

import { after, before, describe, it } from "node:test";
import assert from "node:assert/strict";
import { execFileSync, spawn } from "node:child_process";
import { createServer } from "node:net";
import { createServer as createHttpServer } from "node:http";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import crypto from "node:crypto";
import { fileURLToPath } from "node:url";
import { gate, loadPlaywright, missingPrerequisite } from "./prerequisites.mjs";

const here = path.dirname(fileURLToPath(import.meta.url));
const extensionDir = path.resolve(here, "..");
const repoRoot = path.resolve(extensionDir, "..");
const dist = path.join(extensionDir, "dist");
const mdnBin = process.env.MDN_BIN ?? path.join(repoRoot, "mdn");

/**
 * The name the daemon answers to. `.test` is reserved by RFC 6761 and never
 * resolves, so the host-resolver rule is the only thing that can make this
 * address mean anything at all.
 */
const TAILNET_NAME = "mdn-e2e.tailnet.test";
/**
 * A second such name, resolving to the same listener, which the daemon is not
 * configured to answer to — a daemon URL naming the wrong node of the tailnet.
 */
const OTHER_NAME = "elsewhere.tailnet.test";

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

const ARTICLE = `<!doctype html>
<html lang="en">
<head><meta charset="utf-8"><title>A page worth clipping</title></head>
<body>
  <article>
    <h1>A page worth clipping</h1>
    <p>This paragraph exists so the extractor has enough prose in front of it to
      believe the page is an article at all, which is a precondition of the
      thing actually under test here: the clip the daemon saves under a
      tailnet name.</p>
    <p>A second paragraph, so the decision is not made on a knife edge, and the
      clip that reaches the daemon is a real one rather than an empty body the
      daemon might refuse for a different reason entirely.</p>
  </article>
</body>
</html>`;

const playwright = loadPlaywright();
const blocker = gate(missingPrerequisite(playwright));

describe("against a daemon under a tailnet name", { skip: blocker ?? false }, () => {
  let tmp, notesDir, outsideDir, tokenFile, token, port, daemon, context, extensionId;
  let tailnetOrigin, otherOrigin, site, siteOrigin, articleUrl;

  /** Starts the daemon answering to `host` as well as loopback. */
  const startDaemon = async (host) => {
    daemon = spawn(
      mdnBin,
      [
        "serve",
        "--root", notesDir,
        "--port", String(port),
        "--state", path.join(tmp, "state.json"),
        // Its own token file: the default is the user's real one.
        "--token-file", tokenFile,
        "--tailnet-host", host,
      ],
      { stdio: ["ignore", "pipe", "pipe"] },
    );
    await waitFor(
      () => fetch(`http://localhost:${port}/api/roots`).then(() => true).catch(() => false),
      "the daemon to listen",
    );
  };

  const stopDaemon = async () => {
    daemon?.kill("SIGTERM");
    await waitFor(
      () => fetch(`http://localhost:${port}/api/roots`).then(() => false).catch(() => true),
      "the daemon to stop",
    );
  };

  const worker = () => context.serviceWorkers()[0];
  const configure = (settings) =>
    worker().evaluate((s) => chrome.storage.local.set(s), settings);

  before(async () => {
    tmp = fs.realpathSync(fs.mkdtempSync(path.join(os.tmpdir(), "mdn-tailnet-e2e-")));
    notesDir = path.join(tmp, "notes");
    outsideDir = path.join(tmp, "outside");
    fs.mkdirSync(path.join(notesDir, "deep"), { recursive: true });
    fs.mkdirSync(outsideDir, { recursive: true });
    fs.writeFileSync(path.join(notesDir, "deep", "a note.md"), "# A note\n\nfrom a file URL\n");
    fs.writeFileSync(path.join(outsideDir, "todo.md"), "# Todo\n");

    tokenFile = path.join(tmp, "token");
    token = execFileSync(mdnBin, ["token", "--token-file", tokenFile], { encoding: "utf8" }).trim();
    assert.match(token, /^\S+$/, "mdn token printed a token");

    port = await freePort();
    tailnetOrigin = `http://${TAILNET_NAME}:${port}`;
    otherOrigin = `http://${OTHER_NAME}:${port}`;
    await startDaemon(`${TAILNET_NAME}:${port}`);

    site = createHttpServer((_req, res) => {
      res.setHeader("content-type", "text/html; charset=utf-8");
      res.end(ARTICLE);
    });
    await new Promise((r) => site.listen(0, "127.0.0.1", r));
    siteOrigin = `http://127.0.0.1:${site.address().port}`;
    articleUrl = `${siteOrigin}/posts/one/`;

    // A copy of dist/, so the shipped manifest keeps its narrow host
    // permission while the test grants the addresses it needs. The page's
    // origin stands in for the `activeTab` grant a real click makes.
    const extDir = path.join(tmp, "ext");
    fs.cpSync(dist, extDir, { recursive: true });
    const manifestPath = path.join(extDir, "manifest.json");
    const manifest = JSON.parse(fs.readFileSync(manifestPath, "utf8"));
    manifest.host_permissions = [
      `${tailnetOrigin}/*`,
      `${otherOrigin}/*`,
      `http://localhost:${port}/*`,
      `${siteOrigin}/*`,
      "file:///*",
    ];
    fs.writeFileSync(manifestPath, JSON.stringify(manifest, null, 2));

    // "Allow access to file URLs" is a per-extension profile setting; there is
    // no command line flag for it, so it is seeded into the profile.
    extensionId = unpackedExtensionId(extDir);
    const profile = path.join(tmp, "profile");
    fs.mkdirSync(path.join(profile, "Default"), { recursive: true });
    fs.writeFileSync(
      path.join(profile, "Default", "Preferences"),
      JSON.stringify({ extensions: { settings: { [extensionId]: { newAllowFileAccess: true } } } }),
    );

    context = await playwright.chromium.launchPersistentContext(profile, {
      headless: true,
      // Extensions need the full browser, not the headless shell.
      channel: "chromium",
      args: [
        `--disable-extensions-except=${extDir}`,
        `--load-extension=${extDir}`,
        `--host-resolver-rules=MAP ${TAILNET_NAME} 127.0.0.1, MAP ${OTHER_NAME} 127.0.0.1`,
      ],
    });
    const w =
      context.serviceWorkers()[0] ??
      (await context.waitForEvent("serviceworker", { timeout: 20000 }));
    assert.equal(new URL(w.url()).host, extensionId);
    await configure({ daemonUrl: tailnetOrigin, token });
  });

  after(async () => {
    await context?.close();
    site?.close();
    daemon?.kill("SIGTERM");
    if (tmp !== undefined) fs.rmSync(tmp, { recursive: true, force: true });
  });

  it("serves the roots listing only to a request carrying the token", async () => {
    // The pair the review of PR #49 measured, which is the whole mechanism:
    // the call the intercept makes has to be the authenticated one.
    const statuses = await worker().evaluate(async (origin) => {
      const stored = await chrome.storage.local.get(["token"]);
      const without = await fetch(`${origin}/api/roots`);
      const with_ = await fetch(`${origin}/api/roots`, {
        headers: { Authorization: `Bearer ${stored.token}` },
      });
      return { without: without.status, with: with_.status };
    }, tailnetOrigin);
    assert.deepEqual(statuses, { without: 401, with: 200 });
  });

  const noteFileUrl = () =>
    `file://${path.join(notesDir, "deep", "a note.md").split("/").map(encodeURIComponent).join("/")}`;

  it("opens a local file already inside a registered root", async () => {
    const page = await context.newPage();
    await page.goto(noteFileUrl());
    // The tab leaves the `file:` URL for the note's address on the tailnet
    // name. What the daemon then renders there depends on whether this
    // browser has logged in, which is the app's business and not this
    // extension's; sending the tab to the right place is.
    await page.waitForURL(`${tailnetOrigin}/r/notes/deep/a%20note.md`, { timeout: 15000 });
    const tabId = await waitFor(
      () => pageTabId(worker(), `${tailnetOrigin}/r/notes/deep/a%20note.md`),
      "the redirected tab",
    );
    const status = await tabStatus(worker(), tabId);
    assert.equal(status.kind, "opened");
    assert.match(status.message, new RegExp(`opened in md-notes at ${tailnetOrigin}`));
    assert.equal(
      await worker().evaluate((id) => chrome.action.getBadgeText({ tabId: id }), tabId),
      "",
    );
    await page.close();
  });

  it("names the allow-list, not the token, for a file outside every root", async () => {
    const page = await context.newPage();
    const fileUrl = `file://${path.join(outsideDir, "todo.md")}`;
    await page.goto(fileUrl);
    const tabId = await waitFor(() => pageTabId(worker(), fileUrl), "the tab to appear");
    const status = await waitFor(() => tabStatus(worker(), tabId), "the refusal to be recorded");
    assert.equal(page.url(), fileUrl, "the page itself is left alone");
    assert.equal(status.kind, "loopback_only");
    assert.match(status.message, /Registering a folder is refused over mdn-e2e\.tailnet\.test:\d+/);
    assert.match(status.message, /POST \/api\/roots/);
    assert.match(status.message, /mdn open DIR/);
    assert.doesNotMatch(status.message, /mdn token/);
    assert.doesNotMatch(status.message, /rejected the token/);

    // and the popup over that tab says the same thing
    const popup = await context.newPage();
    await popup.goto(`chrome-extension://${extensionId}/popup.html?tab=${tabId}`);
    await popup.waitForSelector("#status.error");
    assert.match(await popup.textContent("#status"), /Registering a folder is refused/);
    // The daemon line says what this daemon will not do before anything is
    // pressed, rather than leaving the Clip button to find out.
    assert.match(await popup.textContent("#daemon"), /over the tailnet/);
    const daemonLine = await popup.textContent("#daemon");
    assert.match(daemonLine, /registering a folder is refused/);
    // M6-R1: clipping is no longer named among the refusals, because it is
    // no longer one.
    assert.match(daemonLine, /clipping both work here/);
    assert.doesNotMatch(daemonLine, /clipping (?:is|are) refused/);
    await popup.close();
    await page.close();
  });

  it("saves a clip over the tailnet, into the clips directory", async () => {
    // M6-R1, and the case this suite used to make in the other direction:
    // the same rig, the same token, the daemon now admitting `POST
    // /api/clip` under the tailnet name.
    assert.deepEqual(fs.readdirSync(notesDir).sort(), ["deep"], "nothing clipped yet");
    const target = await context.newPage();
    await target.goto(articleUrl);
    const tabId = await waitFor(() => pageTabId(worker(), articleUrl), "the page to clip");

    const popup = await context.newPage();
    await popup.goto(`chrome-extension://${extensionId}/popup.html?tab=${tabId}`);
    await popup.click("#clip-page");
    await popup.waitForSelector("#clip-title:not([disabled])");
    await popup.click("#clip-save");
    await popup.waitForSelector("#clip-status.ok");
    const message = await popup.textContent("#clip-status");
    assert.match(message, /^Saved to clips\/\d{4}-\d{2}-\d{2}-a-page-worth-clipping\.md$/);
    assert.doesNotMatch(message, /refused/);
    // The link under the message opens the new note on the tailnet name,
    // where the clip now lives.
    assert.equal(
      await popup.getAttribute("#clip-open", "href"),
      `${tailnetOrigin}/r/notes/${message.replace("Saved to ", "")}`,
    );

    // And it is a file in the clips directory of the daemon's notes root,
    // exactly as a clip taken on loopback would be.
    assert.deepEqual(fs.readdirSync(notesDir).sort(), ["clips", "deep"]);
    const clips = fs.readdirSync(path.join(notesDir, "clips"));
    assert.equal(clips.length, 1, `one clip, got ${clips.join(", ")}`);
    const note = fs.readFileSync(path.join(notesDir, "clips", clips[0]), "utf8");
    assert.match(note, /^---\ntitle: A page worth clipping\n/);
    assert.ok(note.includes(`source: ${articleUrl}`), `the clip cites the page:\n${note}`);
    assert.match(note, /tags: \[clip\]/);
    // Readability lifts the page's own <h1> into the title, so the body is
    // the prose under it — the real conversion, not an empty file.
    assert.match(note, /---\n\nThis paragraph exists so the extractor/);
    assert.match(note, /A second paragraph, so the decision is not made on a knife edge/);
    await popup.close();
    await target.close();
  });

  it("still refuses to register a folder, and says so without blaming the token", async () => {
    // The half of the allow-list M6-R1 deliberately leaves alone: `POST
    // /api/roots` is the step from a network credential to any directory on
    // the machine, so it stays on the machine.
    const refusal = await worker().evaluate(async (origin) => {
      const stored = await chrome.storage.local.get(["token"]);
      const res = await fetch(`${origin}/api/roots`, {
        method: "POST",
        headers: { Authorization: `Bearer ${stored.token}`, "content-type": "application/json" },
        body: JSON.stringify({ path: "/etc" }),
      });
      return { status: res.status, body: await res.json() };
    }, tailnetOrigin);
    assert.equal(refusal.status, 403);
    assert.equal(refusal.body.code, "loopback_only");
    assert.match(refusal.body.error, /served on loopback only/);
  });

  it("tests the connection by authenticating first, and says what is refused", async () => {
    const page = await context.newPage();
    await page.goto(`chrome-extension://${extensionId}/options.html`);
    await page.fill("#daemon-url", tailnetOrigin);
    await page.fill("#token", token);
    await page.click("#test");
    await page.waitForFunction(() => document.querySelector("#status").textContent !== "testing…");
    const message = await page.textContent("#status");
    assert.equal(await page.getAttribute("#status", "class"), "status ok");
    assert.match(message, /daemon answered: 1 root, and the token was accepted/);
    assert.match(message, /opening a file already inside a registered root and clipping both work here/);
    assert.match(message, /registering a folder is refused/);
    assert.doesNotMatch(message, /clipping (?:is|are) refused/);
    assert.doesNotMatch(message, /rejected the token/);
    await page.close();
  });

  it("says an empty token is the problem rather than reporting a rejected one", async () => {
    const page = await context.newPage();
    await page.goto(`chrome-extension://${extensionId}/options.html`);
    await page.fill("#daemon-url", tailnetOrigin);
    await page.fill("#token", "");
    await page.click("#test");
    await page.waitForFunction(() => document.querySelector("#status").textContent !== "testing…");
    const message = await page.textContent("#status");
    assert.match(message, /no token stored/);
    assert.match(message, /serves\s+nothing without one/);
    assert.doesNotMatch(message, /rejected the token/);
    await page.close();
  });

  it("still reports a genuinely wrong token as a wrong token", async () => {
    const page = await context.newPage();
    await page.goto(`chrome-extension://${extensionId}/options.html`);
    await page.fill("#daemon-url", tailnetOrigin);
    await page.fill("#token", "definitely-not-the-token");
    await page.click("#test");
    await page.waitForFunction(() => document.querySelector("#status").textContent !== "testing…");
    assert.match(await page.textContent("#status"), /the daemon rejected the token/);
    await page.close();
  });

  it("blames the address when the daemon URL names another node of the tailnet", async () => {
    // The name resolves and the listener answers; the daemon simply is not
    // configured to be it. `403 bad_host`, which used to reach the user as an
    // unexplained "unexpected Host header".
    const page = await context.newPage();
    await page.goto(`chrome-extension://${extensionId}/options.html`);
    await page.fill("#daemon-url", otherOrigin);
    await page.fill("#token", token);
    await page.click("#test");
    await page.waitForFunction(() => document.querySelector("#status").textContent !== "testing…");
    const message = await page.textContent("#status");
    assert.match(message, /does not answer to elsewhere\.tailnet\.test:\d+/);
    assert.match(message, /unexpected Host header/);
    assert.match(message, /tailnet_host/);
    await page.close();
  });

  it("blames the port when the name carries one the daemon does not answer to", async () => {
    // A `tailnet_host` of the bare name while the browser reaches the daemon
    // under `<name>:<port>`: the same `403 bad_host` from the other direction,
    // and the case the docs warn about, since `tailnet_host` must carry a
    // non-443 port.
    await stopDaemon();
    await startDaemon(TAILNET_NAME);
    // Whatever this test asserts, the next one starts against a daemon on the
    // right name: the order of the suite should not be load-bearing.
    try {
      const page = await context.newPage();
      await page.goto(`chrome-extension://${extensionId}/options.html`);
      await page.fill("#daemon-url", tailnetOrigin);
      await page.fill("#token", token);
      await page.click("#test");
      await page.waitForFunction(
        () => document.querySelector("#status").textContent !== "testing…",
      );
      const message = await page.textContent("#status");
      assert.match(message, /does not answer to mdn-e2e\.tailnet\.test:\d+/);
      assert.match(message, /unexpected Host header/);
      assert.match(message, /tailnet_host/);
      await page.close();
    } finally {
      await stopDaemon();
      await startDaemon(`${TAILNET_NAME}:${port}`);
    }
  });
});

/** The Chromium tab id showing a URL, or null while it has not settled. */
function pageTabId(worker, url) {
  return worker.evaluate(async (u) => {
    const tabs = await chrome.tabs.query({});
    const tab = tabs.find((t) => t.url === u || t.pendingUrl === u);
    return tab === undefined ? null : tab.id;
  }, url);
}

/** What the extension last recorded about a tab, or null if nothing yet. */
function tabStatus(worker, tabId) {
  return worker.evaluate(
    (id) => chrome.storage.session.get([`status:${id}`]).then((s) => s[`status:${id}`] ?? null),
    tabId,
  );
}
