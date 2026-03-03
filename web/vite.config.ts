import { defineConfig } from "vite";
import vue from "@vitejs/plugin-vue";
import { fileURLToPath, URL } from "node:url";

export default defineConfig({
  plugins: [vue()],
  resolve: {
    alias: {
      "@": fileURLToPath(new URL("./src", import.meta.url)),
    },
  },
  build: {
    outDir: "dist",
    emptyOutDir: true,
  },
  server: {
    port: 5173,
    proxy: {
      "/api": "http://localhost:8025",
      "/mock": "http://localhost:8025",
      "/v1": "http://localhost:8025",
      "/v2": "http://localhost:8025",
      "/v3": "http://localhost:8025",
      "/v4": "http://localhost:8025",
      "/v5": "http://localhost:8025",
    },
  },
});
