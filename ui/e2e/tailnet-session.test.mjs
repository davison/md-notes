/**
 * A tailnet login that lasts until the token is rotated (davison/md-notes#231,
 * M12-R1 and M12-R2), in a real browser against the built daemon.
 *
 * The daemon runs with a `--tailnet-host`, behind a small TLS proxy in this
 * process that does what `tailscale serve` does: terminates https on the
 * tailnet name and forwards to the loopback listener with the Host kept and
 * `X-Forwarded-Proto: https` added. It has to be https: the session cookie is
 * `__Host-` and `Secure`, and a browser drops it over plain http. The
 * certificate is self-signed, made with `openssl` for the run, and Chromium
 * is told to accept it and to resolve the name to 127.0.0.1.
 *
 * Chromium logs in once through the login page; the daemon is then stopped
 * and started again over the same token file, and the same cookie is still a
 * session. A rotation — while running, and while stopped — ends it.
 *
 * The cookie is SameSite=Lax (davison/md-notes#240, M13-R1): a page on another
 * site, served by a second https server here, can send the browser to the app
 * with a link and the session goes with it, as a home-screen shortcut's launch
 * does; its form posts, credentialed fetch and iframe arrive without the
 * cookie. The proxy records, for each request marked `?via=`, whether the
 * browser sent the cookie, so those checks are about what the browser did and
 * not only about the answer.
 *
 * Needs `openssl` on PATH besides what every browser suite needs; without it
 * the suite skips with one line, and fails under CI, as `gate` does for the
 * rest. Touches nothing outside its own `mkdtemp`, and kills only its own
 * daemon, by its handle.
 */

import { after, before, describe, it } from "node:test";
import assert from "node:assert/strict";
import { execFileSync, spawn, spawnSync } from "node:child_process";
import fs from "node:fs";
import http from "node:http";
import https from "node:https";
import os from "node:os";
import path from "node:path";
import { freePort, gate, loadPlaywright, mdnBin, missingPrerequisite, waitFor } from "./harness.mjs";

const playwright = loadPlaywright();

function missingOpenSSL() {
  const probe = spawnSync("openssl", ["version"], { encoding: "utf8" });
  return probe.status === 0 ? null : "openssl is not on PATH (the tailnet session suite makes its certificate with it)";
}

const blocker = gate(missingPrerequisite(playwright) ?? missingOpenSSL());

const NAME = "mdn-session-e2e.tailnet.test";
/** Another site: its own registrable domain, so every request from it to NAME is cross-site. */
const ELSEWHERE = "attacker.test";
const COOKIE = "__Host-mdn_session";

describe("a tailnet login across daemon restarts", { skip: blocker ?? false }, () => {
  let tmp, notesDir, tokenFile, configPath, daemonPort, proxy, origin, host;
  let daemon = null;
  let browser, context, page, elsewhere, elsewhereOrigin;
  /** What the proxy saw of each request marked `?via=`: did the browser send the session? */
  const seen = new Map();
  const log = [];

  async function startDaemon() {
    daemon = spawn(
      mdnBin,
      [
        "serve",
        "--config", configPath,
        "--root", notesDir,
        "--port", String(daemonPort),
        "--state", path.join(tmp, "state.json"),
        "--token-file", tokenFile,
        "--tailnet-host", host,
      ],
      { stdio: ["ignore", "pipe", "pipe"] },
    );
    daemon.stdout.on("data", (d) => log.push(String(d)));
    daemon.stderr.on("data", (d) => log.push(String(d)));
    await waitFor(
      () => fetch(`http://127.0.0.1:${daemonPort}/api/roots`).then((r) => r.ok).catch(() => false),
      `the daemon to listen on ${daemonPort}:\n${log.join("")}`,
    );
  }

  async function stopDaemon() {
    if (!daemon) return;
    const exited = new Promise((r) => daemon.once("exit", r));
    daemon.kill("SIGTERM");
    await exited;
    daemon = null;
  }

  const rotate = () => execFileSync(mdnBin, ["token", "--rotate", "--token-file", tokenFile], { encoding: "utf8" });
  const currentToken = () => fs.readFileSync(tokenFile, "utf8").trim();

  async function sessionCookie() {
    return (await context.cookies(origin)).find((c) => c.name === COOKIE);
  }

  /** Loads the home page and says whether the daemon asked for the token. */
  async function home() {
    const response = await page.goto(`${origin}/`);
    const login = (await page.locator('input[name="token"]').count()) > 0;
    return { status: response.status(), login };
  }

  async function logIn() {
    await page.goto(`${origin}/`);
    await page.fill('input[name="token"]', currentToken());
    await Promise.all([page.waitForURL(`${origin}/`), page.click('button[type="submit"]')]);
    const shown = await home();
    assert.deepEqual(shown, { status: 200, login: false }, "the token logs in");
  }

  /** An API read with the browser's cookies, as the app would make it. */
  async function apiStatus() {
    const other = await context.newPage();
    try {
      return (await other.goto(`${origin}/api/roots`)).status();
    } finally {
      await other.close();
    }
  }

  before(async () => {
    tmp = fs.realpathSync(fs.mkdtempSync(path.join(os.tmpdir(), "mdn-ui-e2e-session-")));
    notesDir = path.join(tmp, "notes");
    fs.mkdirSync(notesDir, { recursive: true });
    fs.writeFileSync(path.join(notesDir, "index.md"), "# Notes\n\nThe root note.\n");
    tokenFile = path.join(tmp, "token");
    execFileSync(mdnBin, ["token", "--token-file", tokenFile], { encoding: "utf8" });
    configPath = path.join(tmp, "config.yaml");
    fs.writeFileSync(configPath, `notes_root: ${notesDir}\n`);

    execFileSync("openssl", [
      "req", "-x509", "-newkey", "ec", "-pkeyopt", "ec_paramgen_curve:prime256v1", "-nodes", "-days", "1",
      "-subj", `/CN=${NAME}`, "-addext", `subjectAltName=DNS:${NAME},DNS:${ELSEWHERE}`,
      "-keyout", path.join(tmp, "key.pem"), "-out", path.join(tmp, "cert.pem"),
    ], { stdio: "ignore" });

    daemonPort = await freePort();
    proxy = https.createServer(
      { key: fs.readFileSync(path.join(tmp, "key.pem")), cert: fs.readFileSync(path.join(tmp, "cert.pem")) },
      (req, res) => {
        const via = new URL(req.url, "https://x").searchParams.get("via");
        const carried = /(?:^|;\s*)__Host-mdn_session=/.test(req.headers.cookie ?? "");
        const upstream = http.request(
          {
            host: "127.0.0.1",
            port: daemonPort,
            method: req.method,
            path: req.url,
            headers: { ...req.headers, "x-forwarded-proto": "https", "x-forwarded-for": "100.64.0.7" },
          },
          (r) => {
            if (via) seen.set(via, { cookie: carried, status: r.statusCode, site: req.headers["sec-fetch-site"] });
            res.writeHead(r.statusCode, r.headers);
            r.pipe(res);
          },
        );
        upstream.on("error", () => {
          if (!res.headersSent) res.writeHead(502);
          res.end();
        });
        req.pipe(upstream);
      },
    );
    await new Promise((resolve) => proxy.listen(0, "127.0.0.1", resolve));
    host = `${NAME}:${proxy.address().port}`;
    origin = `https://${host}`;

    // A page on another site, reaching for the daemon every way a page can.
    elsewhere = https.createServer(
      { key: fs.readFileSync(path.join(tmp, "key.pem")), cert: fs.readFileSync(path.join(tmp, "cert.pem")) },
      (req, res) => {
        const pages = {
          "/link": `<a id="go" href="${origin}/?via=link">notes</a>`,
          "/form-clip": `<form id="f" method="post" action="${origin}/api/clip?via=form-clip"><input name="url" value="https://example.com"></form><script>f.submit()</script>`,
          "/form-login": `<form id="f" method="post" action="${origin}/login?via=form-login"><input name="token" value="x"></form><script>f.submit()</script>`,
          "/iframe": `<iframe src="${origin}/?via=iframe"></iframe>`,
          "/fetch": `<p>fetch</p>`,
        };
        res.writeHead(pages[req.url] ? 200 : 404, { "content-type": "text/html" });
        res.end(`<!doctype html><title>elsewhere</title>${pages[req.url] ?? ""}`);
      },
    );
    await new Promise((resolve) => elsewhere.listen(0, "127.0.0.1", resolve));
    elsewhereOrigin = `https://${ELSEWHERE}:${elsewhere.address().port}`;

    try {
      await startDaemon();
      browser = await playwright.chromium.launch({
        headless: true,
        args: [`--host-resolver-rules=MAP ${NAME} 127.0.0.1, MAP ${ELSEWHERE} 127.0.0.1`],
      });
      context = await browser.newContext({ ignoreHTTPSErrors: true });
      page = await context.newPage();
    } catch (e) {
      daemon?.kill("SIGKILL");
      proxy.close();
      elsewhere?.close();
      fs.rmSync(tmp, { recursive: true, force: true });
      throw e;
    }
  });

  after(async () => {
    await context?.close();
    await browser?.close();
    await stopDaemon();
    proxy?.closeAllConnections?.();
    proxy?.close();
    elsewhere?.closeAllConnections?.();
    elsewhere?.close();
    if (tmp) fs.rmSync(tmp, { recursive: true, force: true });
  });

  it("keeps a login across a daemon restart, with the same cookie and no token", async () => {
    assert.deepEqual(await home(), { status: 401, login: true }, "a fresh browser is asked for the token");
    await logIn();
    const issued = await sessionCookie();
    assert.ok(issued, "the login set the session cookie");
    assert.equal(issued.httpOnly, true);
    assert.equal(issued.secure, true);
    assert.equal(issued.sameSite, "Lax");
    assert.equal(issued.path, "/");

    await stopDaemon();
    await startDaemon();

    assert.deepEqual(await home(), { status: 200, login: false }, "the app, not the login page, after the restart");
    assert.equal(await apiStatus(), 200, "the API accepts the session after the restart");
    assert.equal((await sessionCookie()).value, issued.value, "the cookie the browser kept, not a new login");
  });

  it("carries the session on a navigation from another site, and on nothing else from it", async () => {
    if ((await home()).login) await logIn();
    const other = await context.newPage();
    try {
      // A link on another site: a top-level navigation, what a home-screen
      // shortcut or another app's link is. Under Lax the session goes with it.
      await other.goto(`${elsewhereOrigin}/link`);
      await Promise.all([other.waitForURL(`${origin}/?via=link`), other.click("#go")]);
      assert.equal(await other.locator('input[name="token"]').count(), 0, "the app, not the login page");
      assert.deepEqual(seen.get("link"), { cookie: true, status: 200, site: "cross-site" });

      // A cross-site form POST, to an endpoint the tailnet name admits and
      // to the login form: sent without the cookie, and refused.
      for (const via of ["form-clip", "form-login"]) {
        await other.goto(`${elsewhereOrigin}/${via}`);
        await waitFor(() => seen.has(via), `the ${via} post to reach the daemon`);
        const got = seen.get(via);
        assert.equal(got.cookie, false, `${via}: the cookie was sent on a cross-site POST`);
        assert.ok(got.status === 401 || got.status === 403, `${via}: status ${got.status}, want a refusal`);
      }

      // A credentialed fetch and an iframe from another site: no cookie.
      await other.goto(`${elsewhereOrigin}/fetch`);
      await other.evaluate(async (u) => {
        try {
          await fetch(u, { credentials: "include", mode: "no-cors" });
        } catch {
          // The answer is not the page's to read; the request is what counts.
        }
      }, `${origin}/api/roots?via=fetch`);
      await waitFor(() => seen.has("fetch"), "the fetch to reach the daemon");
      assert.deepEqual(seen.get("fetch"), { cookie: false, status: 401, site: "cross-site" });

      await other.goto(`${elsewhereOrigin}/iframe`);
      await waitFor(() => seen.has("iframe"), "the iframe to load");
      assert.equal(seen.get("iframe").cookie, false, "the cookie was sent to a cross-site iframe");
      assert.equal(seen.get("iframe").status, 401);
    } finally {
      await other.close();
    }

    // And were a cookie to arrive with a foreign Origin anyway, the daemon
    // refuses it: SameSite is not the only defence.
    const value = (await sessionCookie()).value;
    const status = await new Promise((resolve, reject) => {
      const req = http.request(
        {
          host: "127.0.0.1",
          port: daemonPort,
          method: "PUT",
          path: "/api/r/notes/source/index.md",
          headers: {
            host,
            "x-forwarded-proto": "https",
            origin: elsewhereOrigin,
            cookie: `${COOKIE}=${value}`,
            "content-type": "application/json",
          },
        },
        (res) => {
          res.resume();
          resolve(res.statusCode);
        },
      );
      req.on("error", reject);
      req.end("{}");
    });
    assert.equal(status, 403, "a cookie with a foreign Origin");
    assert.equal(fs.readFileSync(path.join(notesDir, "index.md"), "utf8"), "# Notes\n\nThe root note.\n", "the note is untouched");
  });

  it("ends the session when the token is rotated while the daemon runs", async () => {
    if ((await home()).login) await logIn();
    assert.equal(await apiStatus(), 200);
    rotate();
    assert.deepEqual(await home(), { status: 401, login: true }, "the login page after a rotation");
    assert.equal(await apiStatus(), 401);
  });

  it("ends the session when the token file is replaced while the daemon is stopped", async () => {
    await logIn();
    await stopDaemon();
    rotate();
    await startDaemon();
    assert.deepEqual(await home(), { status: 401, login: true }, "the login page after a rotation while stopped");
    assert.equal(await apiStatus(), 401);
  });
});
