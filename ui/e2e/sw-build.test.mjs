/**
 * The service worker's cache name, which is the whole of its update story.
 *
 * `ui/vite.sw.config.ts` names the cache after a hash of what it precaches —
 * the URLs, and the bytes of the files whose names carry no hash of their own.
 * Everything else the worker does rests on that: `activate` deletes every
 * `mdn-` cache that is not the current one, so a rebuild's worker starts from
 * an empty cache and the previous build's shell and chunks go with the old
 * name. A constant there would be silent: the worker would keep serving a
 * cache filled by a build that no longer exists, and no browser check notices,
 * because within one build the behaviour is identical (the review of PR #120,
 * finding N1, made exactly that edit and watched all 43 checks pass).
 *
 * So this runs the build. A byte into the shell, the worker pass again, and
 * the name has to move; the byte back out, and it has to come back — a name
 * that is a function of the content rather than of the clock or a counter.
 */

import { after, before, describe, it } from "node:test";
import assert from "node:assert/strict";
import { execFileSync } from "node:child_process";
import fs from "node:fs";
import path from "node:path";
import { gate, repoRoot, uiDir } from "./harness.mjs";

const dist = path.join(uiDir, "dist");
const shell = path.join(dist, "index.html");
const worker = path.join(dist, "sw.js");

const blocker = gate(fs.existsSync(worker) ? null : `no built worker at ${worker} (run make ui)`);

/** Builds dist/sw.js again, against whatever dist currently holds. */
function buildWorker() {
  execFileSync("pnpm", ["--dir", "ui", "exec", "vite", "build", "--config", "vite.sw.config.ts"], {
    cwd: repoRoot,
    stdio: "pipe",
  });
}

/** The cache name the built worker will open, and what it precaches. */
function builtWorker() {
  const js = fs.readFileSync(worker, "utf8");
  const name = js.match(/mdn-[0-9a-f]{16}/);
  assert.ok(name, "the built worker names an mdn- cache");
  return { name: name[0], urls: [...js.matchAll(/`(\/[^`]*)`/g)].map((m) => m[1]) };
}

describe("the service worker's cache name", { skip: blocker ?? false }, () => {
  let original;

  before(() => {
    original = fs.readFileSync(shell);
  });

  after(() => {
    // Whatever happened, dist goes back to what `make ui` left, worker
    // included: the binary beside it embeds these bytes.
    fs.writeFileSync(shell, original);
    buildWorker();
  });

  it("moves when a precached file changes, and moves back when it changes back", () => {
    const before = builtWorker();
    assert.ok(before.urls.includes("/index.html"), `the shell is precached: ${before.urls}`);
    assert.ok(
      before.urls.some((u) => /^\/assets\/index-.*\.js$/.test(u)),
      `the entry chunk is precached: ${before.urls}`,
    );

    fs.writeFileSync(shell, Buffer.concat([original, Buffer.from("\n<!-- a rebuilt shell -->\n")]));
    buildWorker();
    const rebuilt = builtWorker();
    assert.notEqual(rebuilt.name, before.name, "a changed shell is a new cache");

    fs.writeFileSync(shell, original);
    buildWorker();
    assert.equal(builtWorker().name, before.name, "and the name is the content's, not a counter's");
  });
});
