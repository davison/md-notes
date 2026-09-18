/**
 * Writes the build's version into `dist/manifest.json`.
 *
 * The last step of `pnpm build`, after Vite has copied `public/` across. It is
 * a step of its own rather than a Vite plugin because two Vite builds run in
 * sequence here (the main one and the injected clipper's), and a stamp that
 * depends on which of them copies the public directory, and when, is a stamp
 * that silently stops happening the day that changes.
 */

import fs from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { withVersion } from "./version.mjs";

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");
const target = path.join(root, "dist", "manifest.json");

if (!fs.existsSync(target)) {
  console.error(`nothing to stamp: ${target} does not exist (run the build first)`);
  process.exit(1);
}

const stamped = withVersion(JSON.parse(fs.readFileSync(target, "utf8")), process.env.MDN_VERSION);
fs.writeFileSync(target, `${JSON.stringify(stamped, null, 2)}\n`);
console.log(`dist/manifest.json: version ${stamped.version}`);
