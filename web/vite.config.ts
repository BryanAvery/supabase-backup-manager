import { defineConfig } from "vite";

export default defineConfig({
  base: "/app/",
  build: {
    target: "es2022",
    outDir: "dist",
    assetsDir: "assets",
    manifest: false,
    sourcemap: false,
    emptyOutDir: true,
    cssCodeSplit: false
  },
  server: {
    // Development preview is NOT an authenticated deployment.
    host: "127.0.0.1",
    strictPort: true,
    port: 5173
  }
});
