import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import { resolve } from "node:path";
import { fileURLToPath } from "node:url";

const projectDirectory = fileURLToPath(new URL(".", import.meta.url));

export default defineConfig({
  plugins: [react()],
  build: { outDir: resolve(projectDirectory, "../backend/web/dist"), emptyOutDir: true },
  server: { proxy: { "/api": "http://localhost:8080", "/health": "http://localhost:8080", "/ready": "http://localhost:8080" } },
});
