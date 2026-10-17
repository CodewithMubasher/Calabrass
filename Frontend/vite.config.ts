import { resolve } from "node:path"
import tailwindcss from "@tailwindcss/vite"
import react from "@vitejs/plugin-react"
import { defineConfig } from "vite"

// https://vite.dev/config/
export default defineConfig({
  plugins: [react(), tailwindcss()],
  resolve: {
    alias: {
      "@": resolve(import.meta.dirname, "./src"),
    },
  },
  server: {
    proxy: {
      // The Go backend. Keep the browser on a single origin so SSE and the
      // voice socket work without CORS preflight surprises.
      "/api": {
        target: "http://127.0.0.1:8787",
        changeOrigin: true,
        // Cal Live streams the microphone over this proxy.
        ws: true,
      },
    },
  },
})
