/**
 * End-to-end check of the file-URL intercept, against the built daemon and the
 * built extension loaded into headless Chromium.
 *
 * It is not part of `make check`: it needs a Chromium binary and a built `mdn`,
 * neither of which CI installs. Run it with
 *
 *     make build extension
 *     PLAYWRIGHT_ROOT=/path/to/a/playwright/install pnpm --dir extension e2e
 *
 * and it skips itself, loudly, when a prerequisite is missing.
 */

import { after, before, describe, it } from "node:test";
import assert from "node:assert/strict";
import { createRequire } from "node:module";
import { spawn } from "node:child_process";
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

const playwright = loadPlaywright();
const blocker = missingPrerequisite(playwright);

describe("file URL intercept", { skip: blocker ?? false }, () => {
  let tmp, notesDir, outsideDir, port, daemon, context, worker, extensionId, appOrigin;
  // A second origin, deliberately outside the extension's host permissions:
  // Chromium redacts a tab's URL there, which is the ordinary web's shape.
  let elsewhere, elsewhereOrigin;

  const startDaemon = async () => {
    daemon = spawn(
      mdnBin,
      [
        "serve",
        "--root", notesDir,
        "--port", String(port),
        "--state", path.join(tmp, "state.json"),
        // Its own token file: the default is the user's real one.
        "--token-file", path.join(tmp, "token"),
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

  before(async () => {
    tmp = fs.realpathSync(fs.mkdtempSync(path.join(os.tmpdir(), "mdn-ext-e2e-")));
    notesDir = path.join(tmp, "notes");
    outsideDir = path.join(tmp, "outside");
    fs.mkdirSync(path.join(notesDir, "deep"), { recursive: true });
    fs.mkdirSync(outsideDir, { recursive: true });
    fs.writeFileSync(path.join(notesDir, "deep", "a note.md"), "# A note\n\nfrom a file URL\n");
    fs.writeFileSync(path.join(outsideDir, "todo.md"), "# Todo\n");

    port = await freePort();
    appOrigin = `http://localhost:${port}`;
    await startDaemon();

    elsewhere = createHttpServer((_req, res) => {
      res.setHeader("content-type", "text/html");
      res.end("<h1>somewhere else</h1>");
    });
    await new Promise((r) => elsewhere.listen(0, "127.0.0.1", r));
    elsewhereOrigin = `http://127.0.0.1:${elsewhere.address().port}`;

    // Load a copy of dist/, so the shipped manifest keeps its narrow host
    // permission while the test grants the ephemeral port it needs.
    const extDir = path.join(tmp, "ext");
    fs.cpSync(dist, extDir, { recursive: true });
    const manifestPath = path.join(extDir, "manifest.json");
    const manifest = JSON.parse(fs.readFileSync(manifestPath, "utf8"));
    manifest.host_permissions = [
      `http://localhost:${port}/*`,
      `http://127.0.0.1:${port}/*`,
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
      args: [`--disable-extensions-except=${extDir}`, `--load-extension=${extDir}`],
    });
    worker =
      context.serviceWorkers()[0] ?? (await context.waitForEvent("serviceworker", { timeout: 20000 }));
    assert.equal(new URL(worker.url()).host, extensionId);
  });

  after(async () => {
    await context?.close();
    elsewhere?.close();
    daemon?.kill("SIGTERM");
    if (tmp !== undefined) fs.rmSync(tmp, { recursive: true, force: true });
  });

  it("has file URL access in this profile", async () => {
    const allowed = await worker.evaluate(
      () => new Promise((r) => chrome.extension.isAllowedFileSchemeAccess(r)),
    );
    assert.equal(allowed, true);
  });

  it("stores the daemon URL and token from the options page", async () => {
    const page = await context.newPage();
    await page.goto(`chrome-extension://${extensionId}/options.html`);
    await page.fill("#daemon-url", `${appOrigin}/`);
    await page.fill("#token", "  paste-me  ");
    await page.click("#save");
    await page.waitForSelector("#status.ok");
    assert.equal(await page.textContent("#status"), "saved");

    const stored = await worker.evaluate(() =>
      chrome.storage.local.get(["daemonUrl", "token"]),
    );
    assert.deepEqual(stored, { daemonUrl: appOrigin, token: "paste-me" });

    // and it survives a reload of the page
    await page.reload();
    assert.equal(await page.inputValue("#daemon-url"), appOrigin);
    assert.equal(await page.inputValue("#token"), "paste-me");
    await page.close();
  });

  it("tests the connection with the token, not around it", async () => {
    const page = await context.newPage();
    await page.goto(`chrome-extension://${extensionId}/options.html`);

    // No token: reachable, and honest that writing will be refused.
    await page.fill("#token", "");
    await page.click("#save");
    await page.waitForSelector("#status.ok");
    await page.click("#test");
    await page.waitForFunction(() => document.querySelector("#status").textContent !== "testing…");
    assert.match(await page.textContent("#status"), /daemon answered: 1 root; no token stored/);

    // A token the daemon will not accept must never be reported as accepted.
    // Today's daemon ignores the header entirely — a GET carrying it is not a
    // CORS request, so the Origin guard never sees one — which is why the
    // page asks first whether this daemon checks tokens at all. Once M3-R1
    // lands the same click reports the 401 instead.
    await page.fill("#token", "definitely-a-wrong-token");
    await page.click("#save");
    await page.waitForSelector("#status.ok");
    await page.click("#test");
    await page.waitForFunction(() => document.querySelector("#status").textContent !== "testing…");
    const message = await page.textContent("#status");
    assert.doesNotMatch(message, /token was accepted/);
    assert.match(message, /does not check tokens yet|rejected the token/);
    await page.close();
  });

  const noteFileUrl = () =>
    `file://${path.join(notesDir, "deep", "a note.md").split("/").map(encodeURIComponent).join("/")}`;

  it("opens a markdown file inside a registered root in the app", async () => {
    const page = await context.newPage();
    const fileUrl = noteFileUrl();
    await page.goto(fileUrl);
    await page.waitForURL(`${appOrigin}/r/notes/deep/a%20note.md`, { timeout: 15000 });
    await page.waitForSelector("text=from a file URL", { timeout: 15000 });
    assert.equal(await worker.evaluate(async () => {
      const [tab] = await chrome.tabs.query({ url: "http://localhost/*" });
      return chrome.action.getBadgeText({ tabId: tab.id });
    }), "");
    await page.close();
  });

  it("leaves a file outside every root alone, and says the token is missing", async () => {
    // The connection test above left a wrong token stored, and a wrong token
    // is a different complaint from no token at all. This is the no-token case.
    await worker.evaluate(() => chrome.storage.local.set({ token: "" }));
    const page = await context.newPage();
    const fileUrl = `file://${path.join(outsideDir, "todo.md")}`;
    await page.goto(fileUrl);
    const tabId = await waitFor(() => pageTabId(worker, fileUrl), "the tab to appear");
    const status = await waitFor(
      () => tabStatus(worker, tabId),
      "the extension to record why it could not register the root",
    );
    assert.equal(page.url(), fileUrl);
    assert.equal(status.kind, "origin_refused");
    assert.match(status.message, /mdn token|even with a token/);
    await page.close();
  });

  it("leaves the page alone and badges the tab when the daemon is down", async () => {
    await stopDaemon();
    const page = await context.newPage();
    const fileUrl = noteFileUrl();
    await page.goto(fileUrl);
    const tabId = await waitFor(() => pageTabId(worker, fileUrl), "the tab to appear");
    const status = await waitFor(() => tabStatus(worker, tabId), "the failure to be recorded");
    assert.equal(page.url(), fileUrl);
    assert.equal(status.kind, "unreachable");
    assert.equal(await worker.evaluate((id) => chrome.action.getBadgeText({ tabId: id }), tabId), "!");
    await page.close();
    await startDaemon();
  });

  it("retries on a reload, so starting the daemon and reloading is enough", async () => {
    // A reload carries no changeInfo.url, so the intercept has to read the
    // tab's own URL — without that, every remedy the popup suggests is
    // unreachable in the tab that failed.
    await stopDaemon();
    const page = await context.newPage();
    const fileUrl = noteFileUrl();
    await page.goto(fileUrl);
    const tabId = await waitFor(() => pageTabId(worker, fileUrl), "the tab to appear");
    const failure = await waitFor(() => tabStatus(worker, tabId), "the failure to be recorded");
    assert.equal(failure.kind, "unreachable");

    await startDaemon();
    await page.reload();
    await page.waitForURL(`${appOrigin}/r/notes/deep/a%20note.md`, { timeout: 15000 });
    await page.waitForSelector("text=from a file URL", { timeout: 15000 });
    const after = await tabStatus(worker, tabId);
    assert.equal(after.kind, "opened");
    assert.ok(after.at > failure.at, "the reload ran a fresh attempt");
    await page.close();
  });

  it("forgets a tab that has moved on to the ordinary web", async () => {
    // Chromium only fills in `tab.url` for origins the extension has
    // permission for, so a `loading` event with no URL at all *is* the signal
    // that this tab is somewhere the extension cannot see — and the record
    // from the note it used to show must not follow it there.
    const page = await context.newPage();
    await page.goto(noteFileUrl());
    await page.waitForURL(`${appOrigin}/r/notes/deep/a%20note.md`, { timeout: 15000 });
    const tabId = await waitFor(
      () => pageTabId(worker, `${appOrigin}/r/notes/deep/a%20note.md`),
      "the redirected tab",
    );
    assert.equal((await tabStatus(worker, tabId)).kind, "opened");

    await page.goto(elsewhereOrigin);
    await waitFor(
      async () => (await tabStatus(worker, tabId)) === null,
      "the record to be dropped on an origin the extension cannot see",
    );

    // and the popup on that page has nothing to say
    await page.goto(`chrome-extension://${extensionId}/popup.html`);
    await page.waitForSelector("#status");
    assert.equal(await page.textContent("#status"), "Nothing to report for this tab.");
    await page.close();
  });

  it("renders both popup states", async () => {
    const popupUrl = `chrome-extension://${extensionId}/popup.html`;
    const page = await context.newPage();
    await page.goto(popupUrl);

    // A tab the extension has never acted on has nothing to report.
    await page.waitForSelector("#status");
    assert.equal(await page.textContent("#status"), "Nothing to report for this tab.");
    assert.equal(await page.getAttribute("#status", "class"), "status");
    assert.match(await page.textContent("#daemon"), new RegExp(appOrigin));

    // The popup reads the record for the tab it is open over, so seeding this
    // tab's own record is what a real popup over a failed file page sees.
    const tabId = await page.evaluate(
      () => new Promise((resolve) => chrome.tabs.getCurrent((t) => resolve(t.id))),
    );
    await worker.evaluate(
      (id) =>
        chrome.storage.session.set({
          [`status:${id}`]: {
            kind: "unreachable",
            message: "daemon not reachable at http://localhost:7337",
            source: "file:///n/a.md",
            at: Date.now(),
          },
        }),
      tabId,
    );
    await page.reload();
    await page.waitForSelector("#status.error");
    assert.match(await page.textContent("#status"), /daemon not reachable/);
    await page.close();
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
