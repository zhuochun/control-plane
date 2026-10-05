import type { Reporter, TestCase, TestResult } from "@playwright/test/reporter";
import { mkdirSync, writeFileSync } from "node:fs";
import path from "node:path";

export default class Outcomes implements Reporter {
  private outcomes: { id: string; status: string; duration_seconds: number }[] =
    [];
  onTestEnd(test: TestCase, result: TestResult) {
    const id = test.tags
      .map((tag) => tag.replace(/^@/, ""))
      .find((tag) => tag.startsWith("case:"))
      ?.slice(5);
    if (!id) throw new Error(`Case lacks identity: ${test.title}`);
    this.outcomes.push({
      id,
      status: result.status,
      duration_seconds: result.duration / 1000,
    });
  }
  onEnd() {
    if (!process.env.AICP_TEST_BUNDLE) return;
    mkdirSync(process.env.AICP_TEST_BUNDLE, { recursive: true });
    writeFileSync(
      path.join(process.env.AICP_TEST_BUNDLE, "browser-outcomes.json"),
      JSON.stringify(this.outcomes, null, 2),
    );
  }
}
