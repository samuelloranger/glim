import { defineConfig, devices } from "@playwright/test";

export default defineConfig({
  testDir: "e2e",
  globalSetup: "./e2e/harness.ts",
  workers: 1,
  timeout: 30_000,
  use: { baseURL: "http://127.0.0.1:18799", trace: "retain-on-failure" },
  projects: [{ name: "chromium", use: { ...devices["Desktop Chrome"] } }],
});
