import { defineConfig } from "vite";

export default defineConfig({
  server: {
    watch: {
      // Generated downloads/profiles are not source files. Watching a locked
      // Chromium .crdownload on Windows can otherwise terminate the dev server.
      ignored: ["**/output/**", "**/.tools/**", "**/build/**", "**/wailsjs/**"],
    },
  },
});
