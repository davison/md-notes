import { createHash } from "node:crypto";
import { existsSync, readFileSync } from "node:fs";
import { join, resolve } from "node:path";
import { defineConfig } from "vite";
import { precompress } from "./vite.precompress.ts";

/**
 * The service worker, built on its own into the same dist/.
 *
 * It cannot be an entry of the main build, for the reason
 * `extension/vite.inject.config.ts` gives about the clipper's injected half:
 * one Rollup build has one output format, and a service worker wants a
 * *classic* script. A module worker (`register(..., {type: "module"})`) is
 * Chromium 91 and later; the Boox's stock browser is older than that on some
 * firmware, and a worker that fails to register there would take the offline
 * shell with it. Library mode in IIFE format is a classic script, so the
 * registration is the plain one that every browser with a worker at all
 * accepts.
 *
 * It also runs second because it reads the first pass's output: the precache
 * list below is the eager reading page, taken from the emitted index.html
 * rather than guessed.
 */
const here = import.meta.dirname;
const dist = resolve(here, "dist");

/**
 * The files the shell asks for before it can paint: the entry chunk, the
 * stylesheet, and any static import Vite preloads beside them. Reading them
 * out of the built index.html is the definition rather than an approximation
 * of it — whatever the shell references is what the browser fetches — and it
 * leaves the lazily imported editor chunk out, which is the point. The
 * editor is cached when it is first asked for, not when the app is installed.
 */
function eagerAssets(): string[] {
  const html = readFileSync(join(dist, "index.html"), "utf8");
  return [...new Set([...html.matchAll(/\/assets\/[A-Za-z0-9._-]+/g)].map((m) => m[0]))].sort();
}

/**
 * The shell, the manifest and the favicon: what the app needs to open with
 * nothing behind it. The PNG icons are not here on purpose — they are the
 * launcher's, taken once when the app is installed and kept by Android after
 * that, so precaching twenty kilobytes of them would buy an offline install
 * prompt that nobody can act on. They are still cached if they are ever
 * fetched, like any other file outside the hashed directory.
 */
const staticFiles = ["/index.html", "/manifest.webmanifest", "/icon.svg"];

if (!existsSync(join(dist, "index.html"))) {
  throw new Error("ui/dist/index.html is missing: the service worker pass runs after `vite build`");
}

const precache = [...staticFiles, ...eagerAssets()];

/**
 * The cache name, and so the whole cache's lifetime. It changes when any
 * precached file changes: the hashed assets say so in their names, and the
 * shell, the manifest and the icons are hashed here by content because
 * theirs never change. A new build is therefore a new cache, the old one is
 * deleted on activate, and nothing a previous build left behind can be
 * served by this one.
 */
const version = createHash("sha256");
for (const url of precache) {
  version.update(url);
  version.update(readFileSync(join(dist, url.replace(/^\//, ""))));
}
const cacheName = `mdn-${version.digest("hex").slice(0, 16)}`;

export default defineConfig({
  publicDir: false,
  define: {
    __MDN_CACHE__: JSON.stringify(cacheName),
    __MDN_PRECACHE__: JSON.stringify(precache),
  },
  plugins: [precompress(/^sw\.js$/)],
  build: {
    outDir: "dist",
    emptyOutDir: false,
    // Rollup reports the worker's own size; the main build already reported
    // the bundle, and a second table of one row reads as a second bundle.
    reportCompressedSize: false,
    lib: {
      entry: resolve(here, "src/sw.ts"),
      formats: ["iife"],
      name: "mdnServiceWorker",
      fileName: () => "sw.js",
    },
  },
});
