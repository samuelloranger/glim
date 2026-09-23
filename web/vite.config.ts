import solid from "@solidjs/vite-plugin";
import { defineConfig } from "vite";

export default defineConfig({
  plugins: [solid()],
  // Served by glim under /_glim/; index.html itself is served at "/".
  base: "/_glim/",
  build: {
    outDir: "../internal/web/dist",
    emptyOutDir: true,
    // Never inline assets as data: URIs; the dashboard CSP only allows 'self' fonts.
    assetsInlineLimit: 0,
  },
  server: {
    proxy: { "/_glim/api": "http://127.0.0.1:8787" },
  },
});
