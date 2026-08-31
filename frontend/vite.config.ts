import { defineConfig } from "vitest/config";
import react from "@vitejs/plugin-react";
import { resolve } from "node:path";
import { fileURLToPath } from "node:url";

const projectDirectory = fileURLToPath(new URL(".", import.meta.url));

export default defineConfig({
  plugins: [react()],
  test: { environment: "jsdom", setupFiles: ["./src/test/setup.ts"] },
  build: { outDir: resolve(projectDirectory, "../web/dist"), emptyOutDir: true },
  server: { proxy: { "/api": "http://localhost:8001", "/health": "http://localhost:8001", "/ready": "http://localhost:8001" } },
});
