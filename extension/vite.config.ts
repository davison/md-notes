/// <reference types="vitest/config" />
import { defineConfig } from "vite";
import { resolve } from "node:path";

// The extension is built as three entry points into a flat dist/ that Chromium
// loads unpacked: the MV3 service worker, the options page and the popup.
// public/ (the manifest and the icon) is copied verbatim.
export default defineConfig({
  build: {
    outDir: "dist",
    emptyOutDir: true,
    modulePreload: false,
    rollupOptions: {
      input: {
        background: resolve(import.meta.dirname, "src/background.ts"),
        options: resolve(import.meta.dirname, "options.html"),
        popup: resolve(import.meta.dirname, "popup.html"),
      },
      output: {
        // Chromium loads these by name from the manifest, so they must not be
        // hashed. Shared code goes to a hashed chunk the ES-module worker imports.
        entryFileNames: "[name].js",
        chunkFileNames: "chunk-[hash].js",
        assetFileNames: "[name][extname]",
      },
    },
  },
  test: {
    environment: "node",
    include: ["src/**/*.test.ts"],
  },
});
