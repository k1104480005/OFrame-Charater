import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

// Wails v2 dev/build flow: `wails dev` starts this Vite server and injects
// /wails/runtime.js + /wails/ipc.js automatically; `wails build` embeds dist.
export default defineConfig({
  plugins: [react()],
  build: {
    outDir: "dist",
    // 本机 safe-delete 垫片对 vite 的 outDir 清理（rmSync force）会失败：
    // 关闭自动清理，构建前显式删除 dist（见 wails 打包流程），避免构建中断。
    emptyOutDir: false,
  },
  server: {
    host: "localhost",
    port: 5173,
    strictPort: true,
  },
});
