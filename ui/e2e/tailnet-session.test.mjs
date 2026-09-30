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
const COOKIE = "__Host-mdn_session";

describe("a tailnet login across daemon restarts", { skip: blocker ?? false }, () => {
  let tmp, notesDir, tokenFile, configPath, daemonPort, proxy, origin, host;
  let daemon = null;
  let browser, context, page;
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
      "-subj", `/CN=${NAME}`, "-addext", `subjectAltName=DNS:${NAME}`,
      "-keyout", path.join(tmp, "key.pem"), "-out", path.join(tmp, "cert.pem"),
    ], { stdio: "ignore" });

    daemonPort = await freePort();
    proxy = https.createServer(
      { key: fs.readFileSync(path.join(tmp, "key.pem")), cert: fs.readFileSync(path.join(tmp, "cert.pem")) },
      (req, res) => {
        const upstream = http.request(
          {
            host: "127.0.0.1",
            port: daemonPort,
            method: req.method,
            path: req.url,
            headers: { ...req.headers, "x-forwarded-proto": "https", "x-forwarded-for": "100.64.0.7" },
          },
          (r) => {
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

    try {
      await startDaemon();
      browser = await playwright.chromium.launch({
        headless: true,
        args: [`--host-resolver-rules=MAP ${NAME} 127.0.0.1`],
      });
      context = await browser.newContext({ ignoreHTTPSErrors: true });
      page = await context.newPage();
    } catch (e) {
      daemon?.kill("SIGKILL");
      proxy.close();
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
    if (tmp) fs.rmSync(tmp, { recursive: true, force: true });
  });

  it("keeps a login across a daemon restart, with the same cookie and no token", async () => {
    assert.deepEqual(await home(), { status: 401, login: true }, "a fresh browser is asked for the token");
    await logIn();
    const issued = await sessionCookie();
    assert.ok(issued, "the login set the session cookie");
    assert.equal(issued.httpOnly, true);
    assert.equal(issued.secure, true);
    assert.equal(issued.sameSite, "Strict");
    assert.equal(issued.path, "/");

    await stopDaemon();
    await startDaemon();

    assert.deepEqual(await home(), { status: 200, login: false }, "the app, not the login page, after the restart");
    assert.equal(await apiStatus(), 200, "the API accepts the session after the restart");
    assert.equal((await sessionCookie()).value, issued.value, "the cookie the browser kept, not a new login");
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
