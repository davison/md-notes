/**
 * What the extension's browser suites need before they can run, and what
 * they do when something is missing — shared by all three, so the answer is
 * the same in each.
 *
 * Playwright is declared by `ui/package.json`, which `make ui-deps` installs
 * and `ui/e2e` shares; `PLAYWRIGHT_ROOT` names another installation. The
 * browser is a separate download, and it is probed for rather than found
 * missing by `launch` (davison/md-notes#153): the probe, the install command
 * it names, and the rule that CI fails where a developer's machine skips are
 * `ui/e2e/harness.mjs`'s, imported rather than restated, so the two suites
 * cannot drift apart on what "missing" means.
 */

import { createRequire } from "node:module";
import fs from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { gate, missingBrowser } from "../../ui/e2e/harness.mjs";

export { gate };

const here = path.dirname(fileURLToPath(import.meta.url));
const extensionDir = path.resolve(here, "..");
const repoRoot = path.resolve(extensionDir, "..");
const dist = path.join(extensionDir, "dist");
const mdnBin = process.env.MDN_BIN ?? path.join(repoRoot, "mdn");

export function loadPlaywright() {
  const roots = [process.env.PLAYWRIGHT_ROOT, extensionDir, path.join(repoRoot, "ui"), repoRoot].filter(Boolean);
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

/** Why the suite cannot run here, or null when it can. */
export function missingPrerequisite(playwright) {
  if (playwright === null) return "playwright is not installed (run make ui-deps, or set PLAYWRIGHT_ROOT)";
  const browser = missingBrowser(playwright);
  if (browser !== null) return browser;
  if (!fs.existsSync(path.join(dist, "manifest.json"))) return "extension/dist is not built (run make extension)";
  if (!fs.existsSync(path.join(dist, "clip-inject.js"))) return "extension/dist has no clip-inject.js (run make extension)";
  if (!fs.existsSync(mdnBin)) return `no mdn binary at ${mdnBin} (set MDN_BIN, or run make build)`;
  return null;
}
