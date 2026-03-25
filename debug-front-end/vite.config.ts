import { defineConfig } from "vite";
import preact from "@preact/preset-vite";

export default defineConfig({
  plugins: [preact()],
  server: {
    host: "0.0.0.0",
    port: 5181,
    proxy: {
      "/api": "http://127.0.0.1:18081"
    }
  },
  preview: {
    host: "0.0.0.0",
    port: 4181
  }
});
