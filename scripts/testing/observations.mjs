import assert from "node:assert/strict";
import { mkdirSync, writeFileSync } from "node:fs";
import path from "node:path";

export function compareObservation(actual, expected) {
  assert.deepStrictEqual(actual, expected);
}
export function validateJourney(actual, expected) {
  assert.deepStrictEqual(
    actual.steps.map((s) => s.step),
    Object.keys(expected),
    "Journey must execute every frozen step exactly once, in order",
  );
  assert.ok(
    actual.steps.every((s) => s.outcome === "pass"),
    "Every journey step must pass",
  );
  compareObservation(actual.observations, expected);
}
const cell = (value) => JSON.stringify(value).replaceAll("|", "\\|");

export class JourneyRecord {
  constructor(id, claim, expected, directory) {
    this.id = id;
    this.claim = claim;
    this.expected = expected;
    this.directory = directory;
    this.steps = [];
    this.actual = {};
    mkdirSync(directory, { recursive: true });
  }
  check(step, action, actual) {
    this.actual[step] = actual;
    let failure;
    try {
      if (!(step in this.expected))
        throw new Error(`No frozen expectation for ${this.id}/${step}`);
      compareObservation(actual, this.expected[step]);
    } catch (error) {
      failure = error;
    }
    this.steps.push({
      step,
      action,
      expected: this.expected[step],
      observed: actual,
      outcome: failure ? "fail" : "pass",
    });
    this.write();
    if (failure) throw failure;
  }
  async capture(target, name, options = { fullPage: true }) {
    await target.screenshot({
      ...options,
      path: path.join(this.directory, name),
    });
    this.screenshots ??= [];
    this.screenshots.push(name);
    this.write();
  }
  write() {
    writeFileSync(
      path.join(this.directory, "actual.json"),
      JSON.stringify(
        {
          case_id: this.id,
          claim: this.claim,
          observations: this.actual,
          steps: this.steps.map(({ step, outcome }) => ({ step, outcome })),
        },
        null,
        2,
      ) + "\n",
    );
    const pending = Object.keys(this.expected).filter(
      (step) => !(step in this.actual),
    );
    const lines = [
      `# ${this.id} — ${this.steps.some((s) => s.outcome === "fail") ? "FAIL" : pending.length ? "INCOMPLETE" : "PASS"}`,
      "",
      `Claim: ${this.claim}. Oracle version: 1. Synthetic fixtures; no external source or model judgment.`,
      "",
      "| Step | Actor / action | Expected | Observed | Outcome |",
      "| --- | --- | --- | --- | --- |",
      ...this.steps.map(
        (s) =>
          `| ${s.step} | ${s.action} | ${cell(s.expected)} | ${cell(s.observed)} | ${s.outcome} |`,
      ),
      ...pending.map(
        (step) =>
          `| ${step} | Not reached | ${cell(this.expected[step])} | — | not run |`,
      ),
      "",
      ...(this.screenshots ?? []).map(
        (name) => `![${this.id} ${name}](${name})`,
      ),
      "",
      "Screenshots are human-review evidence, not calibrated visual pass/fail baselines.",
    ];
    writeFileSync(
      path.join(this.directory, "report.md"),
      lines.join("\n") + "\n",
    );
  }
}
