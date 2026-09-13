import { defineConfig } from "vite";
import { resolve } from "node:path";

// The clipper's in-page half, built on its own and added to the same dist/.
//
// It cannot be an entry of the main build: `chrome.scripting.executeScript`
// injects a *classic* script, so this file must carry Readability, Turndown
// and the conversion inside it with no `import` of its own and no `export` at
// the end. Library mode in IIFE format is exactly that, and it is a separate
// pass because one Rollup build has one output format.
export default defineConfig({
  build: {
    outDir: "dist",
    emptyOutDir: false,
    lib: {
      entry: resolve(import.meta.dirname, "src/inject/clip.ts"),
      formats: ["iife"],
      name: "mdNotesClip",
      fileName: () => "clip-inject.js",
    },
  },
});
