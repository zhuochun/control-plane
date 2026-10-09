import { test as base, expect } from "@playwright/test";
import { OwnedServer } from "../../scripts/testing/owned-server.mjs";
import { existsSync, readFileSync, writeFileSync } from "node:fs";
import path from "node:path";
import { root } from "../../scripts/testing/build.mjs";
import { validateJourney } from "../../scripts/testing/observations.mjs";

// Focused system/UI cases share one owned worker server. Journey cases use the
// test-scoped variant below: each starts with a new database and process set.
export const test = base.extend<{}, { ownedServer: OwnedServer }>({
  ownedServer: [
    async ({}, use, workerInfo) => {
      const server = new OwnedServer();
      try {
        await server.start();
        await use(server);
      } finally {
        await server.close({
          diagnostics: workerInfo.project.outputDir + "/ui-processes",
        });
      }
    },
    { scope: "worker", auto: true },
  ],
});

export const journey = base.extend<{ ownedServer: OwnedServer }>({
  ownedServer: [
    async ({ browser }, use, testInfo) => {
      const server = new OwnedServer();
      try {
        await server.start();
        await use(server);
      } finally {
        const report = testInfo.outputPath("journey/report.md");
        let completionFailure;
        if (testInfo.status === "passed") {
          try {
            const actual = JSON.parse(
              readFileSync(testInfo.outputPath("journey/actual.json"), "utf8"),
            );
            if (!/^[a-z0-9-]+$/.test(actual.case_id))
              throw new Error("Invalid journey case ID");
            validateJourney(
              actual,
              JSON.parse(
                readFileSync(
                  path.join(
                    root,
                    "tests/e2e/contracts",
                    actual.case_id + ".json",
                  ),
                  "utf8",
                ),
              ),
            );
          } catch (error) {
            completionFailure = error;
          }
        }
        try {
          await server.close({
            preserve:
              !!completionFailure ||
              testInfo.status !== testInfo.expectedStatus,
            diagnostics: testInfo.outputPath("raw"),
          });
        } catch (error) {
          completionFailure ??= error;
        }
        const execution = {
          status: completionFailure ? "failed" : testInfo.status,
          browser: testInfo.project.use.browserName ?? "chromium",
          browser_version: browser.version(),
          viewport: testInfo.project.use.viewport,
          locale: testInfo.project.use.locale,
          timezone: testInfo.project.use.timezoneId,
        };
        writeFileSync(
          testInfo.outputPath("execution.json"),
          JSON.stringify(execution, null, 2),
        );
        if (existsSync(report))
          writeFileSync(
            report,
            `Overall execution: **${execution.status}**.\n\n` +
              readFileSync(report, "utf8"),
          );
        if (completionFailure) throw completionFailure;
      }
    },
    { auto: true },
  ],
});
export { expect };

// Input lifecycle cases need a fresh capture boundary, without a journey golden.
export const isolated = base.extend<{ ownedServer: OwnedServer }>({
  ownedServer: [async ({}, use, info) => {
    const server = new OwnedServer();
    try { await server.start(); await use(server); }
    finally { await server.close({ diagnostics: info.outputPath("input-processes") }); }
  }, { auto: true }],
});
