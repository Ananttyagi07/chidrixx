import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

// Built assets are embedded into the Go binary (see controlplane/embed.go)
// and served at "/" behind the same Basic Auth as everything else — no
// separate dev server/CORS story needed in production.
export default defineConfig({
  plugins: [react()],
  base: "/",
  build: {
    outDir: "dist",
    emptyOutDir: true,
  },
  server: {
    proxy: {
      // `npm run dev` proxies API calls to a locally running control
      // plane (`go run . -addr=:8090`) so the dashboard can be developed
      // without rebuilding the Go binary on every change.
      "/api": "http://localhost:8090",
    },
  },
});
