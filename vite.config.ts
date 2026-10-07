import { defineConfig } from "vite";

export default defineConfig({
  server: {
    watch: {
      // Generated downloads/profiles are not source files. Watching a locked
      // Chromium .crdownload on Windows can otherwise terminate the dev server.
      // Native fixture trees are movable data, too. On Windows a watcher on a
      // descendant can deny directory rename even with DELETE sharing allowed.
      ignored: ["**/output/**", "**/.appdata/**", "**/.tools/**", "**/build/**", "**/wailsjs/**"],
    },
  },
});
