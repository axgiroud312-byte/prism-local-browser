import { defineConfig } from "@playwright/test";

export default defineConfig({
  testDir: "./tests/ui",
  workers: 1,
  fullyParallel: false,
  timeout: 30_000,
  expect: { timeout: 5_000 },
  outputDir: "output/goal/T01/ui",
  reporter: [["list"], ["html", { outputFolder: "output/goal/T01/report", open: "never" }]],
  use: {
    baseURL: "http://127.0.0.1:5183",
    viewport: { width: 1440, height: 1000 },
    screenshot: "only-on-failure",
    trace: "retain-on-failure",
  },
  webServer: {
    command: "npm run dev -- --port 5183 --strictPort",
    url: "http://127.0.0.1:5183",
    reuseExistingServer: false,
    timeout: 30_000,
  },
});
