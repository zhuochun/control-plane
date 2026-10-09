import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import { fileURLToPath } from "node:url";

export default defineConfig({
  plugins: [react()],
  resolve: {
    alias: {
      // Keep ELK's monolithic layout engine in a worker, off the UI thread.
      "elkjs/lib/elk.bundled.js": fileURLToPath(
        new URL("./src/elk-layout.ts", import.meta.url),
      ),
    },
  },
  build: {
    rolldownOptions: {
      output: {
        codeSplitting: {
          groups: [
            {
              name: "react",
              test: /node_modules[\\/](react|react-dom|scheduler)[\\/]/,
            },
            {
              name: "ui",
              test: /node_modules[\\/](@mui|@emotion|stylis)[\\/]/,
            },
            {
              name: "navigation",
              test: /node_modules[\\/](react-router|@tanstack)[\\/]/,
            },
            {
              name: "d3",
              test: /node_modules[\\/](d3[^\\/]*|internmap)[\\/]/,
            },
          ],
        },
      },
    },
  },
  server: {
    proxy: {
      "/api": {
        target: "http://127.0.0.1:7331",
        changeOrigin: true,
        headers: { Origin: "http://127.0.0.1:7331" },
      },
    },
  },
});
