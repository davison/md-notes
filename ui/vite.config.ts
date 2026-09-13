/// <reference types="vitest/config" />
import { readdirSync, readFileSync, writeFileSync } from "node:fs";
import { join, resolve } from "node:path";
import { brotliCompressSync, constants, gzipSync } from "node:zlib";
import { defineConfig, type Plugin } from "vite";
import preact from "@preact/preset-vite";

/** What is worth compressing: everything the daemon serves as text. */
const compressible = /\.(css|html|js|json|map|mjs|svg|txt|webmanifest|xml)$/;

/**
 * Below this a compressed copy buys nothing worth the bytes it adds to the
 * binary: a gzip header and trailer alone are 18 bytes, and a payload this
 * small crosses the wire in the same packet either way.
 */
const minBytes = 1024;

/**
 * Writes `<file>.br` and `<file>.gz` beside each compressible build output,
 * so the daemon can answer `Accept-Encoding` from the embedded filesystem
 * without carrying a compressor. Node's own `zlib` does both codings, so
 * this costs no dependency; brotli runs at its maximum quality and gzip at
 * level 9 because a build pays for it once and every page load is repaid.
 * A copy that came out no smaller than its source is not written, and the
 * daemon falls back to the identity bytes when a sibling is missing.
 */
function precompress(): Plugin {
  let dir = "";
  return {
    name: "mdn-precompress",
    apply: "build",
    configResolved(config) {
      dir = resolve(config.root, config.build.outDir);
    },
    closeBundle() {
      for (const entry of readdirSync(dir, { recursive: true, withFileTypes: true })) {
        if (!entry.isFile() || !compressible.test(entry.name)) continue;
        const file = join(entry.parentPath, entry.name);
        const raw = readFileSync(file);
        if (raw.byteLength < minBytes) continue;
        const br = brotliCompressSync(raw, {
          params: {
            [constants.BROTLI_PARAM_QUALITY]: constants.BROTLI_MAX_QUALITY,
            [constants.BROTLI_PARAM_MODE]: constants.BROTLI_MODE_TEXT,
            [constants.BROTLI_PARAM_SIZE_HINT]: raw.byteLength,
          },
        });
        if (br.byteLength < raw.byteLength) writeFileSync(`${file}.br`, br);
        const gz = gzipSync(raw, { level: 9 });
        if (gz.byteLength < raw.byteLength) writeFileSync(`${file}.gz`, gz);
      }
    },
  };
}

export default defineConfig({
  plugins: [preact(), precompress()],
  build: { outDir: "dist", emptyOutDir: true },
  server: {
    // During development the Go daemon owns the API; proxy to it.
    proxy: { "/api": "http://127.0.0.1:7337" },
  },
  test: {
    environment: "jsdom",
    include: ["src/**/*.test.{ts,tsx}"],
  },
});
