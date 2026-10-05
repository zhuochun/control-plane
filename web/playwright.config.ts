import { defineConfig } from "@playwright/test";

export default defineConfig({
  testDir: "./tests",
  outputDir: process.env.AICP_TEST_BUNDLE
    ? process.env.AICP_TEST_BUNDLE + "/browser"
    : "./test-results",
  workers: 1,
  forbidOnly: true,
  timeout: 60_000,
  globalTimeout: 270_000,
  retries: 0,
  reporter: [["line"], ["./tests/report-reporter.ts"]],
  grepInvert:
    process.env.AICP_TEST_PROFILE === "renderer"
      ? undefined
      : /@profile:renderer/,
  use: {
    baseURL: "http://127.0.0.1:7331",
    trace: "retain-on-failure",
    viewport: { width: 1440, height: 1000 },
    locale: "en-US",
    timezoneId: "Asia/Singapore",
    actionTimeout: 10_000,
    navigationTimeout: 10_000,
  },
});
