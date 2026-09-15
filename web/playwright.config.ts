import { defineConfig } from "@playwright/test";

const binary =
  process.env.AICP_TEST_BINARY ??
  (process.platform === "win32" ? "..\\dist\\aicp.exe" : "../dist/aicp");

export default defineConfig({
  testDir: "./tests",
  outputDir: "./test-results",
  reporter: "line",
  use: { baseURL: "http://127.0.0.1:7331", trace: "retain-on-failure" },
  webServer: {
    command: `"${binary}" serve --data-dir test-results/e2e-data`,
    url: "http://127.0.0.1:7331/healthz",
    reuseExistingServer: false,
  },
});
