/**
 * What a browser suite does when the browser is not there
 * (davison/md-notes#153).
 *
 * The documentation promised a skip, and a clone with the Playwright package
 * installed but no browser downloaded got a stack trace out of
 * `browserType.launch` instead: `make e2e` exited 2. The harness now probes
 * for the download, skips with one line naming the command that fetches it,
 * and — under CI, where a skipped suite would be a green job that checked
 * nothing — fails instead.
 *
 * This file needs no browser: it runs one real suite in a child process whose
 * `PLAYWRIGHT_BROWSERS_PATH` is an empty directory, which is exactly a machine
 * that never ran `playwright install`. It does need the Playwright package,
 * and without it goes through `gate` like every other suite — a skip here, a
 * failure under CI.
 */

import { after, describe, it } from "node:test";
import assert from "node:assert/strict";
import { spawnSync } from "node:child_process";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import { INSTALL_CHROMIUM, gate, loadPlaywright, missingBrowser, underCI, uiDir } from "./harness.mjs";

const playwright = loadPlaywright();

const blocker = gate(playwright === null ? "playwright is not installed (run make ui-deps, or set PLAYWRIGHT_ROOT)" : null);

describe("a browser suite without the browser", { skip: blocker ?? false }, () => {
  const empty = fs.mkdtempSync(path.join(os.tmpdir(), "mdn-no-browsers-"));
  after(() => fs.rmSync(empty, { recursive: true, force: true }));

  /** Runs the installable-app suite with no browser download, as a clone would. */
  const run = (ci) => {
    const env = { ...process.env, PLAYWRIGHT_BROWSERS_PATH: empty };
    // The parent is a node:test worker; a child that inherits this variable
    // reports to it in the runner's own protocol rather than as a program.
    delete env.NODE_TEST_CONTEXT;
    delete env.CI;
    if (ci !== undefined) env.CI = ci;
    const result = spawnSync(process.execPath, ["--test", path.join(uiDir, "e2e", "pwa.test.mjs")], {
      cwd: uiDir,
      env,
      encoding: "utf8",
      timeout: 60000,
    });
    return { status: result.status, output: `${result.stdout}${result.stderr}` };
  };

  it("skips, exits 0, and names the command that downloads it", () => {
    const { status, output } = run(undefined);
    assert.equal(status, 0, output);
    assert.match(output, new RegExp(`# skipped: Playwright's Chromium is not downloaded \\(run ${INSTALL_CHROMIUM}\\)`));
    assert.doesNotMatch(output, /Executable doesn't exist/, "the probe answers before launch is tried");
  });

  it("fails under CI instead of skipping", () => {
    const { status, output } = run("true");
    assert.notEqual(status, 0, output);
    assert.match(output, /under CI an e2e suite fails rather than skipping/);
    assert.doesNotMatch(output, /# skipped:/);
  });

  it("finds nothing missing when the executable is there", () => {
    const present = { chromium: { executablePath: () => process.execPath } };
    const absent = { chromium: { executablePath: () => path.join(empty, "chrome") } };
    assert.equal(missingBrowser(present), null);
    assert.match(missingBrowser(absent), /not downloaded/);
  });
});

describe("what counts as CI", () => {
  it("is CI set to anything but empty, false or 0", () => {
    assert.equal(underCI({}), false);
    assert.equal(underCI({ CI: "" }), false);
    assert.equal(underCI({ CI: "false" }), false);
    assert.equal(underCI({ CI: "0" }), false);
    assert.equal(underCI({ CI: "true" }), true);
    assert.equal(underCI({ CI: "1" }), true);
  });
});
