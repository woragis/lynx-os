import { defineConfig, devices } from "@playwright/test";
import { resolve } from "node:path";

process.env.DATABASE_URL ??= `file:${resolve(".test/lynx-os-e2e.db")}`;
process.env.LYNX_ENCRYPTION_KEY ??= "d".repeat(64);
process.env.LYNX_GO_API_URL ??= "http://127.0.0.1:8182";
process.env.LYNX_GO_INTERNAL_URL ??= "http://127.0.0.1:8182";

export default defineConfig({
  testDir: "./tests/e2e",
  globalSetup: "./tests/e2e-global-setup.ts",
  fullyParallel: false,
  retries: 0,
  reporter: "list",
  use: {
    ...devices["Desktop Chrome"],
    baseURL: "http://127.0.0.1:3100",
    headless: true,
    launchOptions: process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH
      ? { executablePath: process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH }
      : {},
  },
  webServer: [
    {
      command: "node tests/e2e/auth-api.mjs",
      url: "http://127.0.0.1:8182/health/live",
      reuseExistingServer: !process.env.CI,
      timeout: 20_000,
    },
    {
      command: "npm run dev -- --hostname 127.0.0.1 --port 3100",
      url: "http://127.0.0.1:3100/login",
      reuseExistingServer: !process.env.CI,
      timeout: 120_000,
      env: {
        DATABASE_URL: process.env.DATABASE_URL,
        LYNX_ENCRYPTION_KEY: process.env.LYNX_ENCRYPTION_KEY,
        LYNX_GO_API_URL: process.env.LYNX_GO_API_URL,
        LYNX_GO_INTERNAL_URL: process.env.LYNX_GO_INTERNAL_URL,
      },
    },
  ],
});
