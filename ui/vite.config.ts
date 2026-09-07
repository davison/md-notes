import { defineConfig } from "vite";
import preact from "@preact/preset-vite";

export default defineConfig({
  plugins: [preact()],
  build: { outDir: "dist", emptyOutDir: true },
  server: {
    // During development the Go daemon owns the API; proxy to it.
    proxy: { "/api": "http://127.0.0.1:7337" },
  },
});
