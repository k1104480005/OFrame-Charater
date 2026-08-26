import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

// Wails v2 dev/build flow: `wails dev` starts this Vite server and injects
// /wails/runtime.js + /wails/ipc.js automatically; `wails build` embeds dist.
export default defineConfig({
  plugins: [react()],
  build: {
    outDir: "dist",
    emptyOutDir: true,
  },
  server: {
    host: "localhost",
    port: 5173,
    strictPort: true,
  },
});
