import { defineConfig } from "vite";

// O Tauri serve o frontend em dev por este porto fixo (ver `devUrl` no
// tauri.conf.json); `strictPort` garante que falha em vez de mudar de porto.
export default defineConfig({
  clearScreen: false,
  server: {
    port: 5173,
    strictPort: true,
  },
  build: {
    outDir: "dist",
    emptyOutDir: true,
    target: "es2021",
  },
});
